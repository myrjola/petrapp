package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/repository"
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

func TestWeekPlanRepository_Update_CommitsOnNil(t *testing.T) {
	t.Parallel()
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	seedScheduledSession(ctx, t, db, monday)
	err := repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		return wp.Start(monday, time.Now().UTC())
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	reloaded, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if reloaded.Sessions[0].StartedAt.IsZero() {
		t.Error("Start should have persisted")
	}
}

func TestWeekPlanRepository_Update_RollsBackOnError(t *testing.T) {
	t.Parallel()
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	seedScheduledSession(ctx, t, db, monday)
	sentinel := errors.New("rollback me")
	err := repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		_ = wp.Start(monday, time.Now().UTC())
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Update: got %v, want sentinel", err)
	}
	reloaded, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get after rollback: %v", err)
	}
	if !reloaded.Sessions[0].StartedAt.IsZero() {
		t.Error("rollback should have left StartedAt unset")
	}
}

func TestWeekPlanRepository_Update_PreservesSlotIDs(t *testing.T) {
	t.Parallel()
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	seedScheduledSession(ctx, t, db, monday)

	wp, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(wp.Sessions[0].ExerciseSets) == 0 {
		t.Fatalf("seed should have produced a slot")
	}
	originalSlotID := wp.Sessions[0].ExerciseSets[0].ID

	err = repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		return wp.Start(monday, time.Now().UTC())
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	reloaded, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if reloaded.Sessions[0].ExerciseSets[0].ID != originalSlotID {
		t.Errorf(
			"slot ID changed: got %d, want %d",
			reloaded.Sessions[0].ExerciseSets[0].ID, originalSlotID,
		)
	}
}

// buildPlanWithOneSlot constructs a minimal in-memory WeekPlan with one
// scheduled session on monday containing the Deadlift exercise + one set.
func buildPlanWithOneSlot(
	ctx context.Context, t *testing.T, repos *repository.Repositories, monday time.Time,
) domain.WeekPlan {
	t.Helper()
	exs, err := repos.Exercises.List(ctx)
	if err != nil {
		t.Fatalf("list exercises: %v", err)
	}
	var deadlift domain.Exercise
	for _, e := range exs {
		if e.Name == "Deadlift" {
			deadlift = e
			break
		}
	}
	if deadlift.ID == 0 {
		t.Fatalf("Deadlift seed exercise not found")
	}
	w := 60.0
	plan := domain.WeekPlan{Monday: monday} //nolint:exhaustruct // Sessions filled below.
	for i := range 7 {
		plan.Sessions[i] = domain.Session{Date: monday.AddDate(0, 0, i)} //nolint:exhaustruct // rest-day placeholder.
	}
	plan.Sessions[0] = domain.Session{ //nolint:exhaustruct // Day-zero state only.
		Date:              monday,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned by repo; warmup nil.
				Exercise: deadlift,
				Sets: []domain.Set{
					{TargetValue: 5, WeightKg: &w}, //nolint:exhaustruct // CompletedValue/At/Signal start nil.
				},
			},
		},
	}
	return plan
}

func TestWeekPlanRepository_Create_PersistsWeek(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	plan := buildPlanWithOneSlot(ctx, t, repos, monday)
	if err := repos.WeekPlans.Create(ctx, plan); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Sessions[0].ExerciseSets) == 0 {
		t.Error("session not persisted")
	}
}

func TestWeekPlanRepository_Create_ReturnsErrAlreadyExistsOnConflict(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	plan := buildPlanWithOneSlot(ctx, t, repos, monday)
	if err := repos.WeekPlans.Create(ctx, plan); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	err := repos.WeekPlans.Create(ctx, plan)
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("Create second: got %v, want ErrAlreadyExists", err)
	}
}
