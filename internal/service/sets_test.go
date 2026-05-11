package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_RecordSetCompletion(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

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
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, started_at) VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))",
		userID, today)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var weID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id",
		userID, today, exerciseID).Scan(&weID)
	if err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number,
		 weight_kg, target_value)
		 VALUES (?, 1, 100.0, 5)`,
		weID)
	if err != nil {
		t.Fatalf("insert set: %v", err)
	}

	svc := service.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", today)

	weight := 102.5
	if err = svc.RecordSet(ctx, date, weID, 0, domain.SignalOnTarget, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	sess, err := svc.GetSession(ctx, date)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var es *domain.ExerciseSet
	for i := range sess.ExerciseSets {
		if sess.ExerciseSets[i].Exercise.ID == exerciseID {
			es = &sess.ExerciseSets[i]
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
	cancels   []int
	workout   []fakeWorkoutCancel
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

func (f *fakeScheduler) Cancel(_ context.Context, weID int) error {
	f.mu.Lock()
	f.cancels = append(f.cancels, weID)
	f.mu.Unlock()
	return nil
}

func (f *fakeScheduler) CancelForWorkout(_ context.Context, userID int, date time.Time) error {
	f.mu.Lock()
	f.workout = append(f.workout, fakeWorkoutCancel{userID: userID, date: date})
	f.mu.Unlock()
	return nil
}

func Test_RecordSet_SchedulesRestPush(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	// Seed a second incomplete set so the just-completed one isn't the last.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 2, 100.0, 5)`, weID,
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
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	weight := 100.0
	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.RecordSet(ctx, date, weID, 0, domain.SignalOnTarget, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 1 {
		t.Fatalf("Schedule calls = %d, want 1", len(fake.scheduled))
	}
	if fake.scheduled[0].WorkoutExerciseID != weID {
		t.Errorf("WorkoutExerciseID = %d, want %d", fake.scheduled[0].WorkoutExerciseID, weID)
	}
}

func Test_RecordSet_LastSetDoesNotSchedule(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/last', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	fake := &fakeScheduler{} //nolint:exhaustruct // Slice fields zero-initialised by design.
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	weight := 100.0
	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.RecordSet(ctx, date, weID, 0, domain.SignalOnTarget, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 0 {
		t.Errorf("Schedule calls = %d, want 0 (last set should not schedule)", len(fake.scheduled))
	}
}

// setupSessionForRecordSet builds a workout session with one weighted exercise
// and one planned set, returning everything the scheduling tests need.
func setupSessionForRecordSet(t *testing.T) (context.Context, *sqlite.Database, int, int) {
	t.Helper()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
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
	var weID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		 VALUES (?, ?, ?) RETURNING id`,
		userID, today, exerciseID,
	).Scan(&weID); err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 1, 100.0, 5)`, weID,
	); err != nil {
		t.Fatalf("insert set: %v", err)
	}
	return ctx, db, userID, weID
}
