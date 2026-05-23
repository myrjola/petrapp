package service_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_ResolveWeeklySchedule_GeneratesFullWeekOnFirstLoad(t *testing.T) {
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

func Test_StartSession_CreatesAdHocSessionForUnscheduledToday(t *testing.T) {
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
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

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
