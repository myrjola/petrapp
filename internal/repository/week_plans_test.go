package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// seedScheduledSession inserts a workout_sessions row + one workout_exercise
// (the Deadlift seed exercise) for the authenticated user on date, plus one
// exercise_sets row. Used by WeekPlan tests to populate a scheduled day.
func seedScheduledSession(ctx context.Context, t *testing.T, db *sqlite.Database, date time.Time) {
	t.Helper()
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := date.Format("2006-01-02")
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, periodization_type, is_deload)
		 VALUES (?, ?, 'strength', 0) ON CONFLICT DO NOTHING`,
		userID, dateStr); err != nil {
		t.Fatalf("insert session %s: %v", dateStr, err)
	}
	var exerciseID int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Deadlift'`).Scan(&exerciseID); err != nil {
		t.Fatalf("fetch Deadlift: %v", err)
	}
	var weID int
	if err := db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		 VALUES (?, ?, ?) RETURNING id`,
		userID, dateStr, exerciseID).Scan(&weID); err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, target_value, weight_kg)
		 VALUES (?, 1, 5, 60.0)`, weID); err != nil {
		t.Fatalf("insert exercise_set: %v", err)
	}
}

func TestWeekPlanRepository_Get_ReturnsErrNotFoundForEmptyWeek(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)
	_, err := repos.WeekPlans.Get(ctx, time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestWeekPlanRepository_Get_HydratesScheduledDays(t *testing.T) {
	t.Parallel()
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	seedScheduledSession(ctx, t, db, monday)
	seedScheduledSession(ctx, t, db, monday.AddDate(0, 0, 2)) // Wednesday.

	wp, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !wp.Monday.Equal(monday) {
		t.Errorf("Monday: got %v, want %v", wp.Monday, monday)
	}
	if len(wp.Sessions[0].ExerciseSets) == 0 {
		t.Error("Monday should be scheduled")
	}
	if len(wp.Sessions[1].ExerciseSets) != 0 {
		t.Error("Tuesday should be a rest day (empty)")
	}
	if !wp.Sessions[1].Date.Equal(monday.AddDate(0, 0, 1)) {
		t.Errorf("Tuesday rest day date: got %v", wp.Sessions[1].Date)
	}
	if len(wp.Sessions[2].ExerciseSets) == 0 {
		t.Error("Wednesday should be scheduled")
	}
}
