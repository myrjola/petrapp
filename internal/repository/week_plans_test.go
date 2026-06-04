package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/platform/contexthelpers"
	"github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// seedScheduledSession inserts a workout_sessions row + one workout_exercises
// row at position 0 (the Deadlift seed exercise) for the authenticated user
// on date, plus one exercise_sets row. Used by WeekPlan tests to populate a
// scheduled day.
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
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
		 VALUES (?, ?, 0, ?)`,
		userID, dateStr, exerciseID); err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, target_value, weight_kg)
		 VALUES (?, ?, 0, 1, 5, 60.0)`, userID, dateStr); err != nil {
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
	if len(wp.Sessions[0].Slots) == 0 {
		t.Error("Monday should be scheduled")
	}
	if len(wp.Sessions[1].Slots) != 0 {
		t.Error("Tuesday should be a rest day (empty)")
	}
	if !wp.Sessions[1].Date.Equal(monday.AddDate(0, 0, 1)) {
		t.Errorf("Tuesday rest day date: got %v", wp.Sessions[1].Date)
	}
	if len(wp.Sessions[2].Slots) == 0 {
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

func TestWeekPlanRepository_Update_PreservesSlotPositions(t *testing.T) {
	t.Parallel()
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	seedScheduledSession(ctx, t, db, monday)

	wp, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(wp.Sessions[0].Slots) == 0 {
		t.Fatalf("seed should have produced a slot")
	}
	originalExerciseID := wp.Sessions[0].Slots[0].Exercise.ID

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
	if len(reloaded.Sessions[0].Slots) != 1 {
		t.Fatalf(
			"slot count changed: got %d, want 1",
			len(reloaded.Sessions[0].Slots),
		)
	}
	// The slot's identity is its position (array index 0) and Exercise.ID.
	// Update with a no-op closure must preserve both.
	if reloaded.Sessions[0].Slots[0].Exercise.ID != originalExerciseID {
		t.Errorf(
			"slot 0 exercise changed: got %d, want %d",
			reloaded.Sessions[0].Slots[0].Exercise.ID, originalExerciseID,
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
		Slots: []domain.ExerciseSlot{
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
	if len(got.Sessions[0].Slots) == 0 {
		t.Error("session not persisted")
	}
}

func TestWeekPlanRepository_Update_AddsNewSessionAlongsideExisting(t *testing.T) {
	t.Parallel()
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	tuesday := monday.AddDate(0, 0, 1)
	// Seed Tuesday only — gives that session a slot at position 0.
	seedScheduledSession(ctx, t, db, tuesday)

	// Inside Update: add a brand-new Monday session. Each slot is keyed by
	// (workout_user_id, workout_date, position), so Monday's new position-0
	// slot can never collide with Tuesday's existing position-0 slot.
	err := repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		// Find an exercise to use for the new Monday slot — copy Tuesday's.
		var newSlotExercise domain.Exercise
		for _, slot := range wp.Sessions[1].Slots {
			newSlotExercise = slot.Exercise
			break
		}
		if newSlotExercise.ID == 0 {
			t.Fatal("Tuesday seed should have a slot")
		}
		wp.Sessions[0] = domain.Session{ //nolint:exhaustruct // Day-zero state only.
			Date:              monday,
			PeriodizationType: domain.PeriodizationStrength,
			Slots: []domain.ExerciseSlot{{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: newSlotExercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // CompletedValue etc. nil.
			}},
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Update should succeed adding new session: %v", err)
	}
	reloaded, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(reloaded.Sessions[0].Slots) != 1 {
		t.Errorf("Monday should have 1 slot, got %d", len(reloaded.Sessions[0].Slots))
	}
	if len(reloaded.Sessions[1].Slots) != 1 {
		t.Errorf("Tuesday should still have 1 slot, got %d", len(reloaded.Sessions[1].Slots))
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

// TestWeekPlanRepository_RoundtripsScheduledDayWithNoSlots covers the case
// where the planner schedules a day (so PeriodizationType is set) but the
// per-week exercise pool is exhausted and the session ends up with zero slots.
// The row must persist through Create, hydrate through Get with its
// PeriodizationType intact, and survive a subsequent Start through Update —
// otherwise Start re-inserts a row with empty PeriodizationType and trips the
// workout_sessions CHECK constraint.
func TestWeekPlanRepository_RoundtripsScheduledDayWithNoSlots(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	wednesday := monday.AddDate(0, 0, 2)
	plan := domain.WeekPlan{Monday: monday} //nolint:exhaustruct // Sessions filled below.
	for i := range 7 {
		plan.Sessions[i] = domain.Session{Date: monday.AddDate(0, 0, i)} //nolint:exhaustruct // rest-day.
	}
	// Scheduled Wednesday with no exercise slots — what the planner produces
	// when the week's pool has already been spent on earlier days.
	plan.Sessions[2] = domain.Session{ //nolint:exhaustruct // Slots intentionally empty.
		Date:              wednesday,
		PeriodizationType: domain.PeriodizationStrength,
	}
	if err := repos.WeekPlans.Create(ctx, plan); err != nil {
		t.Fatalf("Create: %v", err)
	}

	reloaded, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if reloaded.Sessions[2].PeriodizationType != domain.PeriodizationStrength {
		t.Errorf("Wednesday PeriodizationType after Get = %q, want %q",
			reloaded.Sessions[2].PeriodizationType, domain.PeriodizationStrength)
	}

	if err = repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		return wp.Start(wednesday, time.Now().UTC())
	}); err != nil {
		t.Fatalf("Update Start: %v", err)
	}

	reloaded, err = repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get after Start: %v", err)
	}
	if reloaded.Sessions[2].StartedAt.IsZero() {
		t.Error("Wednesday Start did not persist")
	}
	if reloaded.Sessions[2].PeriodizationType != domain.PeriodizationStrength {
		t.Errorf("Wednesday PeriodizationType after Start = %q, want %q",
			reloaded.Sessions[2].PeriodizationType, domain.PeriodizationStrength)
	}
}
