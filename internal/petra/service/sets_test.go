package service_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/petra/repository"
	"github.com/myrjola/petrapp/internal/petra/service"
	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/contexthelpers"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func Test_RecordSetCompletion(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + repository.SchemaSQL,
		Fixtures:     repository.FixturesSQL,
		Logger:       logger,
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("rsc-user"), "RSC User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Deadlift is pre-seeded by fixtures.sql; fetch its ID directly.
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Deadlift'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(
		ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, started_at) VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))",
		userID,
		today,
	)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	const pos = 0
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id) VALUES (?, ?, ?, ?)",
		userID, today, pos, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number,
		 weight_kg, target_value)
		 VALUES (?, ?, ?, 1, 100.0, 5)`,
		userID, today, pos)
	if err != nil {
		t.Fatalf("insert set: %v", err)
	}

	svc := service.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", today)

	weight := 102.5
	sig := domain.SignalOnTarget
	if err = svc.RecordSet(ctx, date, pos, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	sess, err := svc.GetSession(ctx, date)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var es *domain.ExerciseSlot
	for i := range sess.Slots {
		if sess.Slots[i].Exercise.ID == exerciseID {
			es = &sess.Slots[i]
			break
		}
	}
	if es == nil {
		t.Fatal("exercise not found in session")
	}

	set := es.Sets[0]
	if set.Signal == nil || *set.Signal != domain.SignalOnTarget {
		t.Errorf("signal: want on_target, got %v", set.Signal)
	}
	if set.WeightKg == nil || *set.WeightKg != 102.5 {
		t.Errorf("weight: want 102.5, got %v", set.WeightKg)
	}
	if set.CompletedValue == nil || *set.CompletedValue != 5 {
		t.Errorf("completed value: want 5, got %v", set.CompletedValue)
	}
	if set.CompletedAt == nil {
		t.Error("completed_at: want non-nil")
	}
}

// fakeScheduler captures Schedule/Cancel calls in test.
type fakeScheduler struct {
	mu        sync.Mutex
	scheduled []domain.ScheduledPush
	cancels   []fakeSlotCancel
	workout   []fakeWorkoutCancel
}

type fakeSlotCancel struct {
	userID int
	date   time.Time
	pos    int
}

type fakeWorkoutCancel struct {
	userID int
	date   time.Time
}

func (f *fakeScheduler) Schedule(_ context.Context, push domain.ScheduledPush) error {
	f.mu.Lock()
	f.scheduled = append(f.scheduled, push)
	f.mu.Unlock()
	return nil
}

func (f *fakeScheduler) Cancel(_ context.Context, userID int, date time.Time, pos int) error {
	f.mu.Lock()
	f.cancels = append(f.cancels, fakeSlotCancel{userID: userID, date: date, pos: pos})
	f.mu.Unlock()
	return nil
}

func (f *fakeScheduler) CancelForWorkout(_ context.Context, userID int, date time.Time) error {
	f.mu.Lock()
	f.workout = append(f.workout, fakeWorkoutCancel{userID: userID, date: date})
	f.mu.Unlock()
	return nil
}

// erroringScheduler returns the configured error from Schedule and behaves
// like a normal fake for Cancel/CancelForWorkout. Used to exercise the
// rest-push failure-logging path.
type erroringScheduler struct {
	scheduleErr error
}

func (e *erroringScheduler) Schedule(_ context.Context, _ domain.ScheduledPush) error {
	return e.scheduleErr
}
func (e *erroringScheduler) Cancel(_ context.Context, _ int, _ time.Time, _ int) error { return nil }
func (e *erroringScheduler) CancelForWorkout(_ context.Context, _ int, _ time.Time) error {
	return nil
}

func Test_RecordSet_SchedulesRestPush(t *testing.T) {
	t.Parallel()

	ctx, db, userID, pos := setupSessionForRecordSet(t)
	today := time.Now().Format("2006-01-02")
	// Seed a second incomplete set so the just-completed one isn't the last.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, weight_kg, target_value)
		 VALUES (?, ?, ?, 2, 100.0, 5)`, userID, today, pos,
	); err != nil {
		t.Fatalf("seed second set: %v", err)
	}
	// Seed a push subscription so the precondition holds.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/1', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	// Make sure rest_notifications_enabled defaults to true: explicitly set the row.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct // Slice fields zero-initialised by design.
	svc := service.NewService(db, testkit.NewLogger(testkit.NewWriter(t)), "").
		WithScheduler(fake)

	weight := 100.0
	date := time.Now().UTC().Truncate(24 * time.Hour)
	sig := domain.SignalOnTarget
	if err := svc.RecordSet(ctx, date, pos, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 1 {
		t.Fatalf("Schedule calls = %d, want 1", len(fake.scheduled))
	}
	if fake.scheduled[0].Position != pos {
		t.Errorf("Position = %d, want %d", fake.scheduled[0].Position, pos)
	}
	if !fake.scheduled[0].WorkoutDate.Equal(date) {
		t.Errorf("WorkoutDate = %s, want %s", fake.scheduled[0].WorkoutDate, date)
	}
}

func Test_RecordSet_LastSetDoesNotSchedule(t *testing.T) {
	t.Parallel()

	ctx, db, userID, pos := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/last', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	fake := &fakeScheduler{} //nolint:exhaustruct // Slice fields zero-initialised by design.
	svc := service.NewService(db, testkit.NewLogger(testkit.NewWriter(t)), "").
		WithScheduler(fake)

	weight := 100.0
	date := time.Now().UTC().Truncate(24 * time.Hour)
	sig2 := domain.SignalOnTarget
	if err := svc.RecordSet(ctx, date, pos, 0, &sig2, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 0 {
		t.Errorf("Schedule calls = %d, want 0 (last set should not schedule)", len(fake.scheduled))
	}
}

// Test_RecordSet_LastSetOfSlotWhileOtherSlotsIncomplete_Cancels is the
// regression for a defect where finishing the last set of one exercise
// scheduled a "Time for set N+1 of N" push because the gate was session-wide
// instead of per-slot. After the fix the policy returns Cancel for the
// just-finished slot.
func Test_RecordSet_LastSetOfSlotWhileOtherSlotsIncomplete_Cancels(t *testing.T) {
	t.Parallel()

	ctx, db, userID, pos := setupSessionForRecordSet(t)
	// Seed a SECOND exercise slot with one incomplete set so the session
	// still has work after finishing the first slot's only set.
	var otherExerciseID int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Bench Press'`,
	).Scan(&otherExerciseID); err != nil {
		t.Fatalf("get bench press id: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	const otherPos = 1
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
		 VALUES (?, ?, ?, ?)`,
		userID, today, otherPos, otherExerciseID,
	); err != nil {
		t.Fatalf("insert second workout_exercises: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, weight_kg, target_value)
		 VALUES (?, ?, ?, 1, 60.0, 5)`, userID, today, otherPos,
	); err != nil {
		t.Fatalf("insert other slot set: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/cancel', 'p', 'a')`, userID,
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
	svc := service.NewService(db, testkit.NewLogger(testkit.NewWriter(t)), "").
		WithScheduler(fake)

	// Complete the only set in the first slot (pos).
	weight := 100.0
	date := time.Now().UTC().Truncate(24 * time.Hour)
	sig := domain.SignalOnTarget
	if err := svc.RecordSet(ctx, date, pos, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 0 {
		t.Errorf("Schedule calls = %d, want 0 (slot fully complete must not schedule)", len(fake.scheduled))
	}
	if len(fake.cancels) != 1 {
		t.Fatalf("Cancel calls = %d, want 1", len(fake.cancels))
	}
	if fake.cancels[0].pos != pos {
		t.Errorf("Cancel target pos = %d, want %d", fake.cancels[0].pos, pos)
	}
	if !fake.cancels[0].date.Equal(date) {
		t.Errorf("Cancel target date = %s, want %s", fake.cancels[0].date, date)
	}
}

// Test_UpdateCompletedValue_DoesNotTouchScheduler locks in that editing the
// recorded value on an already-complete set is bookkeeping, not progress —
// no Schedule and no Cancel calls should reach the scheduler.
func Test_UpdateCompletedValue_DoesNotTouchScheduler(t *testing.T) {
	t.Parallel()

	ctx, db, userID, pos := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/edit', 'p', 'a')`, userID,
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
	svc := service.NewService(db, testkit.NewLogger(testkit.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	weight := 100.0
	sig := domain.SignalOnTarget
	if err := svc.RecordSet(ctx, date, pos, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet (seed completion): %v", err)
	}

	fake.mu.Lock()
	scheduledBefore := len(fake.scheduled)
	cancelsBefore := len(fake.cancels)
	fake.mu.Unlock()

	if err := svc.UpdateCompletedValue(ctx, date, pos, 0, 6); err != nil {
		t.Fatalf("UpdateCompletedValue: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != scheduledBefore {
		t.Errorf("Schedule calls after edit = %d, want %d", len(fake.scheduled), scheduledBefore)
	}
	if len(fake.cancels) != cancelsBefore {
		t.Errorf("Cancel calls after edit = %d, want %d", len(fake.cancels), cancelsBefore)
	}
}

// Test_UpdateSetWeight_DoesNotTouchScheduler locks in the same invariant
// for weight edits.
func Test_UpdateSetWeight_DoesNotTouchScheduler(t *testing.T) {
	t.Parallel()

	ctx, db, userID, pos := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/weight-edit', 'p', 'a')`, userID,
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
	svc := service.NewService(db, testkit.NewLogger(testkit.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.UpdateSetWeight(ctx, date, pos, 0, 105.0); err != nil {
		t.Fatalf("UpdateSetWeight: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 0 {
		t.Errorf("Schedule calls = %d, want 0 (weight edit is not a trigger)", len(fake.scheduled))
	}
	if len(fake.cancels) != 0 {
		t.Errorf("Cancel calls = %d, want 0 (weight edit is not a trigger)", len(fake.cancels))
	}
}

// Test_RecordSet_RerecordCompletedSet_DoesNotReschedule locks in the
// !wasComplete guard: re-recording an already-complete set produces no
// new scheduler interactions.
func Test_RecordSet_RerecordCompletedSet_DoesNotReschedule(t *testing.T) {
	t.Parallel()

	ctx, db, userID, pos := setupSessionForRecordSet(t)
	today := time.Now().Format("2006-01-02")
	// Seed a second set so the first completion schedules a push.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, weight_kg, target_value)
		 VALUES (?, ?, ?, 2, 100.0, 5)`, userID, today, pos,
	); err != nil {
		t.Fatalf("seed second set: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/rerecord', 'p', 'a')`, userID,
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
	svc := service.NewService(db, testkit.NewLogger(testkit.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	weight := 100.0
	sig := domain.SignalOnTarget
	if err := svc.RecordSet(ctx, date, pos, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet (first): %v", err)
	}

	fake.mu.Lock()
	if len(fake.scheduled) != 1 {
		fake.mu.Unlock()
		t.Fatalf("after first RecordSet, scheduled = %d, want 1", len(fake.scheduled))
	}
	fake.mu.Unlock()

	// Re-record the same set with a different value. wasComplete is true now,
	// so the policy must not be re-invoked.
	if err := svc.RecordSet(ctx, date, pos, 0, &sig, &weight, 6); err != nil {
		t.Fatalf("RecordSet (re-record): %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 1 {
		t.Errorf("Schedule calls after re-record = %d, want 1", len(fake.scheduled))
	}
	if len(fake.cancels) != 0 {
		t.Errorf("Cancel calls after re-record = %d, want 0", len(fake.cancels))
	}
}

// setupSessionForRecordSet builds a workout session with one weighted exercise
// and one planned set, returning everything the scheduling tests need. The
// returned int is the slot's position (always 0 here — single-slot setup).
func setupSessionForRecordSet(t *testing.T) (context.Context, *sqlitekit.Database, int, int) {
	t.Helper()
	ctx := t.Context()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + repository.SchemaSQL,
		Fixtures:     repository.FixturesSQL,
		Logger:       logger,
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("rs-user"), "RS User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	var exerciseID int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Deadlift'`,
	).Scan(&exerciseID); err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))`,
		userID, today,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	const pos = 0
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
		 VALUES (?, ?, ?, ?)`,
		userID, today, pos, exerciseID,
	); err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, weight_kg, target_value)
		 VALUES (?, ?, ?, 1, 100.0, 5)`, userID, today, pos,
	); err != nil {
		t.Fatalf("insert set: %v", err)
	}
	return ctx, db, userID, pos
}

func Test_RecordSet_FailedSchedule_LogsUserAndPosition(t *testing.T) {
	t.Parallel()

	ctx, db, userID, pos := setupSessionForRecordSet(t)
	today := time.Now().Format("2006-01-02")
	// Seed an incomplete second set so the just-completed one isn't the last,
	// which means PlanRestPush returns Schedule (not Cancel).
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, weight_kg, target_value)
		 VALUES (?, ?, ?, 2, 100.0, 5)`, userID, today, pos,
	); err != nil {
		t.Fatalf("seed second set: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/fail', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	var logBuf bytes.Buffer
	//nolint:exhaustruct // AddSource/ReplaceAttr intentionally zero.
	handlerOpts := &slog.HandlerOptions{Level: slog.LevelDebug}
	logger := slog.New(slog.NewTextHandler(&logBuf, handlerOpts))
	failer := &erroringScheduler{scheduleErr: errors.New("boom: push backend down")}
	svc := service.NewService(db, logger, "").WithScheduler(failer)

	weight := 100.0
	sig := domain.SignalOnTarget
	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.RecordSet(ctx, date, pos, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	out := logBuf.String()
	wantUser := fmt.Sprintf("user_id=%d", userID)
	wantPos := fmt.Sprintf("position=%d", pos)
	if !strings.Contains(out, wantUser) {
		t.Errorf("log missing %q; got:\n%s", wantUser, out)
	}
	if !strings.Contains(out, wantPos) {
		t.Errorf("log missing %q; got:\n%s", wantPos, out)
	}
	if !strings.Contains(out, "rest push: schedule failed") {
		t.Errorf("log missing expected message; got:\n%s", out)
	}
}
