package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func newTestExerciseFor(t *testing.T) domain.Exercise {
	t.Helper()
	return domain.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds nil for non-time_based.
		Name:                  "Test_Repo_Bench_Press_Sessions",
		Category:              domain.CategoryUpper,
		ExerciseType:          domain.ExerciseTypeWeighted,
		DescriptionMarkdown:   "# Test_Repo_Bench_Press_Sessions",
		PrimaryMuscleGroups:   []string{"Chest"},
		SecondaryMuscleGroups: []string{"Triceps"},
		RepMin:                new(5),
		RepMax:                new(10),
	}
}

func TestSessionRepository_GetMissingReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	missing := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := repos.Sessions.Get(ctx, missing)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound, got %v", err)
	}
}

func TestSessionRepository_GetHydratesExercise(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}

	monday := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero by design.
		Date:              monday,
		PeriodizationType: domain.PeriodizationStrength,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // ID assigned by DB; WarmupCompletedAt nil.
				Exercise: exercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	wp := domain.WeekPlan{Monday: monday} //nolint:exhaustruct // Sessions initialised below.
	for i := range 7 {
		//nolint:exhaustruct // rest-day placeholder; only Date is meaningful.
		wp.Sessions[i] = domain.Session{Date: monday.AddDate(0, 0, i)}
	}
	wp.Sessions[0] = sess
	if err = repos.WeekPlans.Create(ctx, wp); err != nil {
		t.Fatalf("WeekPlans.Create: %v", err)
	}

	got, err := repos.Sessions.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Slots) != 1 {
		t.Fatalf("want 1 ExerciseSlot, got %d", len(got.Slots))
	}
	hydrated := got.Slots[0].Exercise
	if hydrated.ID != exercise.ID {
		t.Errorf("Exercise.ID: want %d, got %d", exercise.ID, hydrated.ID)
	}
	if hydrated.Name != "Test_Repo_Bench_Press_Sessions" {
		t.Errorf("Exercise.Name: want Test_Repo_Bench_Press_Sessions, got %q", hydrated.Name)
	}
	if len(hydrated.PrimaryMuscleGroups) != 1 || hydrated.PrimaryMuscleGroups[0] != "Chest" {
		t.Errorf("Exercise.PrimaryMuscleGroups: want [Chest], got %v", hydrated.PrimaryMuscleGroups)
	}
}

func TestSessionRepository_ListHydratesEverySession(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	monday := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	earlier := monday                // Monday.
	later := monday.AddDate(0, 0, 2) // Wednesday — same week so a single WeekPlans.Create suffices.
	mkSession := func(day time.Time) domain.Session {
		return domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero.
			Date:              day,
			PeriodizationType: domain.PeriodizationStrength,
			Slots: []domain.ExerciseSlot{
				{ //nolint:exhaustruct // ID assigned by DB; WarmupCompletedAt nil.
					Exercise: exercise,
					Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
				},
			},
		}
	}
	wp := domain.WeekPlan{Monday: monday} //nolint:exhaustruct // Sessions initialised below.
	for i := range 7 {
		//nolint:exhaustruct // rest-day placeholder; only Date is meaningful.
		wp.Sessions[i] = domain.Session{Date: monday.AddDate(0, 0, i)}
	}
	wp.Sessions[0] = mkSession(earlier)
	wp.Sessions[2] = mkSession(later)
	if err = repos.WeekPlans.Create(ctx, wp); err != nil {
		t.Fatalf("WeekPlans.Create: %v", err)
	}

	got, err := repos.Sessions.List(ctx, earlier)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(got))
	}
	// Newest first.
	if !got[0].Date.Equal(later) || !got[1].Date.Equal(earlier) {
		t.Fatalf("want dates [%s, %s], got [%s, %s]",
			later.Format(time.DateOnly), earlier.Format(time.DateOnly),
			got[0].Date.Format(time.DateOnly), got[1].Date.Format(time.DateOnly))
	}
	// The batched query must hydrate every session, not just the first.
	for _, sess := range got {
		label := sess.Date.Format(time.DateOnly)
		if len(sess.Slots) != 1 {
			t.Fatalf("session %s: want 1 ExerciseSlot, got %d", label, len(sess.Slots))
		}
		ex := sess.Slots[0].Exercise
		if ex.ID != exercise.ID || ex.Name != exercise.Name {
			t.Errorf("session %s: exercise not hydrated: %+v", label, ex)
		}
		if len(ex.PrimaryMuscleGroups) != 1 || ex.PrimaryMuscleGroups[0] != "Chest" {
			t.Errorf("session %s: PrimaryMuscleGroups = %v, want [Chest]", label, ex.PrimaryMuscleGroups)
		}
		if len(sess.Slots[0].Sets) != 1 || sess.Slots[0].Sets[0].TargetValue != 5 {
			t.Errorf("session %s: sets not hydrated: %+v", label, sess.Slots[0].Sets)
		}
	}
}

func TestSessionRepository_RoundTripIsDeload(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	monday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // only IsDeload round-trip is exercised
		Date:              monday,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          true,
	}
	wp := domain.WeekPlan{Monday: monday} //nolint:exhaustruct // Sessions initialised below.
	for i := range 7 {
		//nolint:exhaustruct // rest-day placeholder; only Date is meaningful.
		wp.Sessions[i] = domain.Session{Date: monday.AddDate(0, 0, i)}
	}
	wp.Sessions[0] = sess
	if err := repos.WeekPlans.Create(ctx, wp); err != nil {
		t.Fatalf("WeekPlans.Create: %v", err)
	}
	got, err := repos.Sessions.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.IsDeload {
		t.Error("IsDeload = false, want true")
	}
}

func TestSessionRepository_StartingWeight_SkipsDeloadSessions(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}

	mondayNormal := time.Date(2026, time.April, 27, 0, 0, 0, 0, time.UTC)
	mondayDeload := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	weight100 := 100.0
	weight90 := 90.0
	onTarget := domain.SignalOnTarget
	completedAt := time.Date(2026, time.April, 27, 10, 0, 0, 0, time.UTC)

	normal := domain.Session{ //nolint:exhaustruct // only fields relevant to deload skip test
		Date:              mondayNormal,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          false,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // ID and WarmupCompletedAt not needed for round-trip test
				Exercise: exercise,
				Sets: []domain.Set{
					{
						TargetValue:    10,
						WeightKg:       &weight100,
						CompletedValue: new(10),
						CompletedAt:    &completedAt,
						Signal:         &onTarget,
					},
				},
			},
		},
	}
	deload := domain.Session{ //nolint:exhaustruct // only fields relevant to deload skip test
		Date:              mondayDeload,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          true,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // ID and WarmupCompletedAt not needed for round-trip test
				Exercise: exercise,
				Sets: []domain.Set{
					{
						TargetValue:    10,
						WeightKg:       &weight90,
						CompletedValue: new(10),
						CompletedAt:    &completedAt,
						Signal:         &onTarget,
					},
				},
			},
		},
	}

	// Two distinct weeks → two Create calls.
	for _, wkSess := range []struct {
		monday time.Time
		sess   domain.Session
	}{{mondayNormal, normal}, {mondayDeload, deload}} {
		wp := domain.WeekPlan{Monday: wkSess.monday} //nolint:exhaustruct // Sessions initialised below.
		for i := range 7 {
			//nolint:exhaustruct // rest-day placeholder; only Date is meaningful.
			wp.Sessions[i] = domain.Session{Date: wkSess.monday.AddDate(0, 0, i)}
		}
		wp.Sessions[0] = wkSess.sess
		if err = repos.WeekPlans.Create(ctx, wp); err != nil {
			t.Fatalf("WeekPlans.Create %s: %v", wkSess.monday.Format(time.DateOnly), err)
		}
	}

	got, err := repos.Sessions.GetLatestStartingWeightBefore(ctx, exercise.ID, mondayDeload.AddDate(0, 0, 7))
	if err != nil {
		t.Fatalf("GetLatestStartingWeightBefore: %v", err)
	}
	if got.WeightKg != 100.0 {
		t.Errorf("WeightKg = %v, want 100.0 (deload session must be excluded)", got.WeightKg)
	}
}

func TestGetLatestSuccessfulSecondsBefore_NoRows_ReturnsNotFound(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	// No exercise_sets seeded for exercise 99999 → must surface ErrNotFound.
	_, err := repos.Sessions.GetLatestSuccessfulSecondsBefore(ctx, 99999, time.Now())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want domain.ErrNotFound", err)
	}
}
