package service_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_ResolveWeeklySchedule_GeneratesFullWeekOnFirstLoad(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	if len(sessions) != 7 {
		t.Fatalf("want 7 sessions (one per day), got %d", len(sessions))
	}

	// Scheduled days (Mon=0, Wed=2, Fri=4) must have exercises.
	for _, i := range []int{0, 2, 4} {
		if len(sessions[i].ExerciseSets) == 0 {
			t.Errorf("sessions[%d] (%s) must have exercise sets", i, sessions[i].Date.Weekday())
		}
	}

	// Rest days must be empty sessions.
	for _, i := range []int{1, 3, 5, 6} {
		if len(sessions[i].ExerciseSets) != 0 {
			t.Errorf("sessions[%d] (%s) must be empty (rest day)", i, sessions[i].Date.Weekday())
		}
	}
}

func Test_ResolveWeeklySchedule_DoesNotRegenerateExistingSessions(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	sessions1, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("first ResolveWeeklySchedule: %v", err)
	}

	sessions2, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("second ResolveWeeklySchedule: %v", err)
	}

	// Same scheduled days must have the same exercise IDs on both calls.
	for _, i := range []int{0, 2, 4} {
		ids1 := extractExerciseIDs(sessions1[i])
		ids2 := extractExerciseIDs(sessions2[i])
		if !slices.Equal(ids1, ids2) {
			t.Errorf("sessions[%d] exercise IDs changed on second call: %v → %v", i, ids1, ids2)
		}
	}
}

func Test_GetSession_ReturnsErrNotFoundForUnplannedDate(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	// Generate this week's plan.
	if _, err := svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Request a date in a different week.
	nextWeekTuesday := time.Now().AddDate(0, 0, 14)
	_, err := svc.GetSession(ctx, nextWeekTuesday)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want ErrNotFound for unplanned date, got %v", err)
	}
}

func Test_RegenerateWeeklyPlanIfUnstarted_RegeneratesFromEmptyWeek(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t) // Mon, Wed, Fri at 60 min — no sessions created yet

	// Call directly without seeding via ResolveWeeklySchedule first.
	if err := svc.RegenerateWeeklyPlanIfUnstarted(ctx); err != nil {
		t.Fatalf("RegenerateWeeklyPlanIfUnstarted on empty week: %v", err)
	}

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after empty-week regenerate: %v", err)
	}
	// Mon=0, Wed=2, Fri=4 must have exercises.
	for _, i := range []int{0, 2, 4} {
		if len(sessions[i].ExerciseSets) == 0 {
			t.Errorf("sessions[%d] (%s) must have exercise sets", i, sessions[i].Date.Weekday())
		}
	}
}

func Test_RegenerateWeeklyPlanIfUnstarted_RegeneratesWhenNoWorkoutStarted(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t) // Mon, Wed, Fri at 60 min

	// Generate the initial plan.
	if _, err := svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Change to Tue, Thu, Sat at 45 min.
	if err := svc.SaveUserPreferences(ctx, domain.Preferences{ //nolint:exhaustruct // Rest days intentionally omitted.
		TuesdayMinutes:  45,
		ThursdayMinutes: 45,
		SaturdayMinutes: 45,
	}); err != nil {
		t.Fatalf("save preferences: %v", err)
	}

	if err := svc.RegenerateWeeklyPlanIfUnstarted(ctx); err != nil {
		t.Fatalf("RegenerateWeeklyPlanIfUnstarted: %v", err)
	}

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after regenerate: %v", err)
	}

	// Tue=1, Thu=3, Sat=5 must have exercises; Mon=0, Wed=2, Fri=4, Sun=6 must be rest.
	for _, i := range []int{1, 3, 5} {
		if len(sessions[i].ExerciseSets) == 0 {
			t.Errorf("sessions[%d] (%s) must have exercise sets after preference change", i, sessions[i].Date.Weekday())
		}
	}
	for _, i := range []int{0, 2, 4, 6} {
		if len(sessions[i].ExerciseSets) != 0 {
			t.Errorf("sessions[%d] (%s) must be a rest day after preference change", i, sessions[i].Date.Weekday())
		}
	}
}

func Test_RegenerateWeeklyPlanIfUnstarted_SkipsRegenerateWhenWorkoutStarted(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t) // Mon, Wed, Fri at 60 min

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Start the first scheduled workout (Monday, index 0).
	if err = svc.StartSession(ctx, sessions[0].Date); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Change preferences to Tue, Thu, Sat.
	if err = svc.SaveUserPreferences(ctx, domain.Preferences{ //nolint:exhaustruct // Rest days intentionally omitted.
		TuesdayMinutes:  45,
		ThursdayMinutes: 45,
		SaturdayMinutes: 45,
	}); err != nil {
		t.Fatalf("save preferences: %v", err)
	}

	if err = svc.RegenerateWeeklyPlanIfUnstarted(ctx); err != nil {
		t.Fatalf("RegenerateWeeklyPlanIfUnstarted: %v", err)
	}

	sessions2, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after skip: %v", err)
	}

	// Monday (index 0) must still have exercises — the original plan was kept.
	if len(sessions2[0].ExerciseSets) == 0 {
		t.Error("sessions2[0] (Monday) must still have exercise sets; workout was already started")
	}

	// Tuesday must still be a rest day — the new preferences were not applied.
	if len(sessions2[1].ExerciseSets) != 0 {
		t.Error("sessions2[1] (Tuesday) must remain a rest day; new preferences must not be applied")
	}
}

func Test_CompleteSession_CancelsPendingPushes(t *testing.T) {
	t.Parallel()

	ctx, db, userID, _ := setupSessionForRecordSet(t)
	fake := &fakeScheduler{} //nolint:exhaustruct // Slice fields zero-initialised by design.
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	today := time.Now().UTC().Truncate(24 * time.Hour)
	// CompleteSession requires StartedAt to be set, which setupSessionForRecordSet
	// already does via the INSERT INTO workout_sessions ... started_at clause.
	if err := svc.CompleteSession(ctx, today); err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.workout) != 1 {
		t.Fatalf("CancelForWorkout calls = %d, want 1", len(fake.workout))
	}
	if fake.workout[0].userID != userID {
		t.Errorf("CancelForWorkout userID = %d, want %d", fake.workout[0].userID, userID)
	}
	if !fake.workout[0].date.Equal(today) {
		t.Errorf("CancelForWorkout date = %v, want %v", fake.workout[0].date, today)
	}
}

// Test_CompleteSession_UnstartedSession_AutoStartsAndCompletes covers the
// retroactive-finish flow: a user navigates to a past scheduled workout that
// they performed in real life but never marked started in the app, and clicks
// "Finish workout". CompleteSession must succeed by auto-starting the session
// inside the same transaction. Before 2026-05-24 this returned ErrNotStarted
// and the handler routed the user to /error — see the prod logs referenced in
// the fix commit.
func Test_CompleteSession_UnstartedSession_AutoStartsAndCompletes(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t) // Mon, Wed, Fri at 60 min

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Pick a scheduled workout (Monday, index 0) and complete it WITHOUT
	// calling StartSession first — the production "session not started" path.
	date := sessions[0].Date

	if err = svc.CompleteSession(ctx, date); err != nil {
		t.Fatalf("CompleteSession on unstarted session: %v", err)
	}

	sess, err := svc.GetSession(ctx, date)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.StartedAt.IsZero() {
		t.Error("StartedAt is zero after auto-start; CompleteSession should have started the session")
	}
	if sess.CompletedAt.IsZero() {
		t.Error("CompletedAt is zero; CompleteSession should have completed the session")
	}
	if got := sess.Status(); got != domain.SessionCompleted {
		t.Errorf("Status = %q, want %q", got, domain.SessionCompleted)
	}
}

func Test_StartSession_CreatesAdHocSessionForUnscheduledToday(t *testing.T) {
	t.Parallel()

	// setupTestService sets Mon/Wed/Fri preferences. Pick a day this week
	// the user has not scheduled (a Tuesday) and verify StartSession both
	// creates the session and marks it started.
	ctx, svc := setupTestService(t)

	// Ensure the week is generated first so usedExerciseIDs is populated.
	weekSessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	monday := weekSessions[0].Date
	tue := monday.AddDate(0, 0, 1)

	if err = svc.StartSession(ctx, tue); err != nil {
		t.Fatalf("StartSession on unscheduled Tuesday: %v", err)
	}

	sess, err := svc.GetSession(ctx, tue)
	if err != nil {
		t.Fatalf("GetSession after ad-hoc Start: %v", err)
	}
	if sess.StartedAt.IsZero() {
		t.Error("StartedAt is zero — Start did not mark the session")
	}
	if len(sess.ExerciseSets) == 0 {
		t.Error("ad-hoc session has no exercises — PlanDay or persistence failed")
	}
}

func Test_StartSession_CreatesNewlyScheduledMidWeekDay(t *testing.T) {
	t.Parallel()

	// Mon/Wed/Fri prefs, start Monday, then change prefs to add Tuesday
	// — RegenerateWeeklyPlanIfUnstarted will skip because Monday is started.
	// StartSession on Tuesday must still succeed by creating the session.
	ctx, svc := setupTestService(t)

	weekSessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	monday := weekSessions[0].Date
	tue := monday.AddDate(0, 0, 1)

	if err = svc.StartSession(ctx, monday); err != nil {
		t.Fatalf("StartSession Monday: %v", err)
	}

	if err = svc.SaveUserPreferences(ctx, domain.Preferences{ //nolint:exhaustruct // Rest days omitted.
		MondayMinutes:    60,
		TuesdayMinutes:   60,
		WednesdayMinutes: 60,
		FridayMinutes:    60,
	}); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	// RegenerateWeeklyPlanIfUnstarted is a no-op now (Monday is started).
	if err = svc.RegenerateWeeklyPlanIfUnstarted(ctx); err != nil {
		t.Fatalf("RegenerateWeeklyPlanIfUnstarted: %v", err)
	}

	if err = svc.StartSession(ctx, tue); err != nil {
		t.Fatalf("StartSession Tuesday after schedule change: %v", err)
	}

	sess, err := svc.GetSession(ctx, tue)
	if err != nil {
		t.Fatalf("GetSession Tuesday: %v", err)
	}
	if sess.StartedAt.IsZero() {
		t.Error("Tuesday StartedAt is zero")
	}
	if len(sess.ExerciseSets) == 0 {
		t.Error("Tuesday session has no exercises")
	}
}

func Test_StartSession_DoubleStartIsIdempotent(t *testing.T) {
	t.Parallel()

	// Two StartSession calls on the same unscheduled date must both succeed
	// and leave exactly one started session. Simulates the lazy-create race
	// via sequential calls (the second's Create returns ErrAlreadyExists and
	// the Update is idempotent via ErrAlreadyStarted).
	ctx, svc := setupTestService(t)

	weekSessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	tue := weekSessions[0].Date.AddDate(0, 0, 1)

	if err = svc.StartSession(ctx, tue); err != nil {
		t.Fatalf("first StartSession: %v", err)
	}
	if err = svc.StartSession(ctx, tue); err != nil {
		t.Fatalf("second StartSession (must be idempotent): %v", err)
	}

	sess, err := svc.GetSession(ctx, tue)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.StartedAt.IsZero() {
		t.Error("StartedAt is zero after two Start calls")
	}
}

func Test_GenerateWorkout_PeriodizationTypeAlternatesAcrossSessions(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := service.NewService(db, logger, "")

	// Save preferences with Mon, Wed, Fri as workout days.
	if err = svc.SaveUserPreferences(ctx, domain.Preferences{ //nolint:exhaustruct // Rest days intentionally omitted.
		MondayMinutes:    60,
		WednesdayMinutes: 60,
		FridayMinutes:    60,
	}); err != nil {
		t.Fatalf("save preferences: %v", err)
	}

	// Generate this week's plan and collect periodization types for all 3 workout days.
	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Collect periodization types for scheduled days (Mon=0, Wed=2, Fri=4).
	scheduledIndices := []int{0, 2, 4}
	types := make([]domain.PeriodizationType, len(scheduledIndices))
	for j, i := range scheduledIndices {
		types[j] = sessions[i].PeriodizationType
	}

	// Each consecutive session must alternate periodization type.
	for i := 1; i < len(types); i++ {
		if types[i] == types[i-1] {
			t.Errorf("sessions[%d] and sessions[%d] have the same periodization type %q; want alternating",
				i-1, i, types[i])
		}
	}
}

func Test_MarkWarmupComplete_SchedulesPushForFirstSet(t *testing.T) {
	t.Parallel()

	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/warmup', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct // Slice fields zero-initialised by design.
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 1 {
		t.Fatalf("Schedule calls = %d, want 1", len(fake.scheduled))
	}
	if fake.scheduled[0].WorkoutExerciseID != weID {
		t.Errorf("WorkoutExerciseID = %d, want %d", fake.scheduled[0].WorkoutExerciseID, weID)
	}
	if !strings.Contains(fake.scheduled[0].Payload, `"set_number":1`) {
		t.Errorf("payload = %q, want set_number=1", fake.scheduled[0].Payload)
	}
}

func Test_MarkWarmupComplete_NoSubscriptions_DoesNotSchedule(t *testing.T) {
	t.Parallel()

	ctx, db, _, weID := setupSessionForRecordSet(t)
	fake := &fakeScheduler{} //nolint:exhaustruct // Slice fields zero-initialised by design.
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 0 {
		t.Errorf("Schedule calls = %d, want 0 (no subscriptions)", len(fake.scheduled))
	}
}

func Test_MarkWarmupComplete_AfterFirstSetComplete_SchedulesSet2(t *testing.T) {
	t.Parallel()

	ctx, db, userID, weID := setupSessionForRecordSet(t)
	// Seed a second set so set 2 exists for the schedule target.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 2, 100.0, 5)`, weID,
	); err != nil {
		t.Fatalf("seed second set: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/warmup-after-set1', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct // Slice fields zero-initialised by design.
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	weight := 100.0
	sig := domain.SignalOnTarget
	// Complete set 1 first.
	if err := svc.RecordSet(ctx, date, weID, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}
	// Now click warmup-complete (out-of-order user behavior, but legal).
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 2 {
		t.Fatalf("Schedule calls = %d, want 2 (one for set 1 complete, one for warmup)", len(fake.scheduled))
	}
	if !strings.Contains(fake.scheduled[1].Payload, `"set_number":2`) {
		t.Errorf("second payload = %q, want set_number=2 (warmup plans for first incomplete)",
			fake.scheduled[1].Payload)
	}
}

func Test_MarkWarmupComplete_AllSetsComplete_CancelsAndDoesNotSchedule(t *testing.T) {
	t.Parallel()

	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/warmup-done', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct // Slice fields zero-initialised by design.
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	weight := 100.0
	sig := domain.SignalOnTarget
	// Complete the only set, then call warmup-complete on an exhausted slot.
	if err := svc.RecordSet(ctx, date, weID, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}
	fake.mu.Lock()
	preScheduleCount := len(fake.scheduled)
	preCancelCount := len(fake.cancels)
	fake.mu.Unlock()

	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != preScheduleCount {
		t.Errorf("Schedule calls after warmup = %d, want %d (no schedule when all done)",
			len(fake.scheduled), preScheduleCount)
	}
	// Warmup on an all-done slot triggers a Cancel from the policy.
	if len(fake.cancels) != preCancelCount+1 {
		t.Errorf("Cancel calls after warmup = %d, want %d", len(fake.cancels), preCancelCount+1)
	}
}

func Test_MarkWarmupComplete_AlreadyDone_DoesNotReschedule(t *testing.T) {
	t.Parallel()

	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/warmup-dup', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct // Slice fields zero-initialised by design.
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("first MarkWarmupComplete: %v", err)
	}
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("second MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 1 {
		t.Errorf("Schedule calls = %d, want 1 (second call is a no-op)", len(fake.scheduled))
	}
}

func Test_RegenerateWeeklyPlanIfUnstarted_ConcurrentCallsSerialized(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	const goroutines = 8
	var (
		wg   sync.WaitGroup
		errs = make(chan error, goroutines)
	)
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			errs <- svc.RegenerateWeeklyPlanIfUnstarted(ctx)
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("RegenerateWeeklyPlanIfUnstarted: %v", err)
		}
	}

	// Verify the week was regenerated cleanly. Use the repo directly since
	// Service has no public ListSessions wrapper.
	monday := domain.MondayOf(time.Now())
	sunday := monday.AddDate(0, 0, 6)
	allSessions, err := svc.Repos().Sessions.List(ctx, monday)
	if err != nil {
		t.Fatalf("Sessions.List: %v", err)
	}
	thisWeek := 0
	withExercises := 0
	for _, s := range allSessions {
		if !s.Date.After(sunday) {
			thisWeek++
			if len(s.ExerciseSets) > 0 {
				withExercises++
			}
		}
	}
	if withExercises != 3 {
		t.Errorf("after concurrent regenerate: got %d sessions with exercises, want 3 (Mon/Wed/Fri)", withExercises)
	}
}

func Test_StartDeloadNow_FlipsTodayAndFutureNonCompletedSessions(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t) // Mon/Wed/Fri 60 min

	// Enable deload so the button is permissible; anchor it to this week's
	// Monday so the planner treats this week as accumulation week 0 (not a
	// natural deload week).
	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}

	// Materialise the week's sessions.
	if _, err = svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Sanity: no session should be a natural-cadence deload yet.
	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule (re-list): %v", err)
	}
	for i, s := range sessions {
		if s.IsDeload {
			t.Fatalf("session[%d] (%s) unexpectedly already deload", i, s.Date.Weekday())
		}
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow: %v", err)
	}

	sessions, err = svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after StartDeloadNow: %v", err)
	}

	today := domain.StartOfDay(time.Now())
	for i, s := range sessions {
		if len(s.ExerciseSets) == 0 {
			continue // rest day
		}
		isForwardLooking := !s.Date.Before(today)
		if isForwardLooking && !s.IsDeload {
			t.Errorf("session[%d] (%s, %s) should be deload (today or later, not completed)",
				i, s.Date.Weekday(), s.Date.Format(time.DateOnly))
		}
		if !isForwardLooking && s.IsDeload {
			t.Errorf("session[%d] (%s, %s) should NOT be deload (past)",
				i, s.Date.Weekday(), s.Date.Format(time.DateOnly))
		}
	}
}

func Test_StartDeloadNow_SnapsAnchorToNextMonday(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	if _, err = svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow: %v", err)
	}

	got, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences after StartDeloadNow: %v", err)
	}
	if !got.MesocycleAnchor.After(monday) {
		t.Errorf("MesocycleAnchor = %v; want a Monday strictly after %v",
			got.MesocycleAnchor, monday)
	}
	if got.MesocycleAnchor.Weekday() != time.Monday {
		t.Errorf("MesocycleAnchor weekday = %v; want Monday", got.MesocycleAnchor.Weekday())
	}
	if !got.MesocycleAnchor.Equal(monday.AddDate(0, 0, 7)) {
		t.Errorf("MesocycleAnchor = %v; want %v (next Monday)",
			got.MesocycleAnchor, monday.AddDate(0, 0, 7))
	}
}

func Test_StartDeloadNow_Idempotent(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	if _, err = svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow first call: %v", err)
	}
	first, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after first: %v", err)
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow second call: %v", err)
	}
	second, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after second: %v", err)
	}

	for i := range first {
		if first[i].IsDeload != second[i].IsDeload {
			t.Errorf("session[%d] IsDeload flipped between calls: %v -> %v",
				i, first[i].IsDeload, second[i].IsDeload)
		}
	}
}

// Test_StartDeloadNow_SkipsCompletedToday covers the orchestrator closure's
// Status() != SessionCompleted re-check — the spec's central race-avoidance
// argument. Between List returning a snapshot and Update running, a concurrent
// caller may have completed the session; the closure must not flip it then.
//
// Determinism: setupTestService uses a Mon/Wed/Fri schedule. The test needs
// (a) today to be a scheduled workout day so we can complete it, and (b) at
// least one scheduled workout strictly after today to prove the loop kept
// going. Today is Friday or weekend → no future scheduled workout this week
// → t.Skip.
func Test_StartDeloadNow_SkipsCompletedToday(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t) // Mon/Wed/Fri 60 min

	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	today := domain.StartOfDay(time.Now())
	todayIdx := -1
	futureWorkoutDays := 0
	for i, s := range sessions {
		if s.Date.Equal(today) && len(s.ExerciseSets) > 0 {
			todayIdx = i
		}
		if s.Date.After(today) && len(s.ExerciseSets) > 0 {
			futureWorkoutDays++
		}
	}
	if todayIdx == -1 {
		t.Skip("today is a rest day in Mon/Wed/Fri schedule; cannot complete a non-existent session")
	}
	if futureWorkoutDays == 0 {
		t.Skip("no scheduled workout strictly after today this week; cannot prove loop ran past today")
	}

	// Fully complete today's session — CompleteSession auto-starts if needed
	// (see Test_CompleteSession_UnstartedSession_AutoStartsAndCompletes) and
	// sets CompletedAt, which is what flips Status() to SessionCompleted.
	if err = svc.CompleteSession(ctx, today); err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow: %v", err)
	}

	sessions, err = svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after StartDeloadNow: %v", err)
	}

	// Today must remain non-deload — the closure's Status() re-check saw
	// SessionCompleted and returned nil without calling SwitchToDeload.
	if sessions[todayIdx].IsDeload {
		t.Errorf("today's session (%s) IsDeload = true; closure must skip completed sessions",
			sessions[todayIdx].Date.Weekday())
	}

	// At least one future scheduled session must have flipped — proves the
	// loop kept iterating past the completed today rather than aborting.
	flippedFuture := 0
	for _, s := range sessions {
		if s.Date.After(today) && len(s.ExerciseSets) > 0 && s.IsDeload {
			flippedFuture++
		}
	}
	if flippedFuture == 0 {
		t.Errorf("no future scheduled session flipped to deload; loop must continue past completed today")
	}
}
