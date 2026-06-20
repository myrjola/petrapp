package service_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/platform/contexthelpers"
)

// Test_PreviousPerformance verifies the "Last time" data source: it returns the
// most recent prior session for an exercise, skips the queried date's own
// session, and reports ok=false when there is no history.
func Test_PreviousPerformance(t *testing.T) {
	t.Parallel()

	ctx, svc, db := setupTestServiceWithDB(t)
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var exerciseID int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Deadlift'`).Scan(&exerciseID); err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	prior, err := time.Parse("2006-01-02", "2026-01-10")
	if err != nil {
		t.Fatalf("parse prior date: %v", err)
	}
	current, err := time.Parse("2006-01-02", "2026-01-17")
	if err != nil {
		t.Fatalf("parse current date: %v", err)
	}

	// No history yet → ok=false, no error.
	if _, ok, perr := svc.PreviousPerformance(ctx, current, exerciseID); perr != nil || ok {
		t.Fatalf("expected no history before seeding, got ok=%v err=%v", ok, perr)
	}

	// Seed a completed prior session for the same exercise.
	priorStr := prior.Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), STRFTIME('%Y-%m-%dT%H:%M:%fZ'))`,
		userID, priorStr); err != nil {
		t.Fatalf("insert prior session: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_slots (workout_user_id, workout_date, position, exercise_id, warmup_completed_at)
		 VALUES (?, ?, 0, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))`,
		userID, priorStr, exerciseID); err != nil {
		t.Fatalf("insert prior slot: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets
		   (workout_user_id, workout_date, position, set_number,
		    weight_kg, target_value, completed_value, completed_at, signal)
		 VALUES (?, ?, 0, 1, 100.0, 5, 5, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'on_target')`,
		userID, priorStr); err != nil {
		t.Fatalf("insert prior set: %v", err)
	}

	// A later date sees the prior session.
	got, ok, err := svc.PreviousPerformance(ctx, current, exerciseID)
	if err != nil {
		t.Fatalf("PreviousPerformance: %v", err)
	}
	if !ok {
		t.Fatal("expected prior session to be found")
	}
	if !got.Date.Equal(prior) {
		t.Errorf("Date = %v, want %v", got.Date, prior)
	}
	if len(got.Sets) != 1 || got.Sets[0].CompletedValue == nil || *got.Sets[0].CompletedValue != 5 {
		t.Errorf("unexpected sets: %+v", got.Sets)
	}

	// Querying the prior date itself must skip that day's own session.
	if _, ok, _ = svc.PreviousPerformance(ctx, prior, exerciseID); ok {
		t.Error("PreviousPerformance(prior) must skip the session recorded on that same date")
	}
}
