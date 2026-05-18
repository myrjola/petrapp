package domain

import (
	"errors"
	"testing"
	"time"
)

// planDayExercises returns a small pool with Upper, Lower, and FullBody coverage
// across distinct primary muscles so PlanDay's non-conflict selection has room.
// FullBody is listed first so FullBody-day selection (greedy first-pick) includes it,
// making sess.WorkoutType() return CategoryFullBody reliably in tests.
func planDayExercises() []Exercise {
	intPtr := func(v int) *int { return &v }
	return []Exercise{
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 6, Name: "Plank", Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Core"}, SecondaryMuscleGroups: nil,
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 1, Name: "Bench Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: []string{"Triceps"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 2, Name: "Row", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Upper Back"}, SecondaryMuscleGroups: []string{"Biceps"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 3, Name: "Overhead Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: []string{"Triceps"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 4, Name: "Squat", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads"}, SecondaryMuscleGroups: []string{"Glutes"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 5, Name: "Deadlift", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings"}, SecondaryMuscleGroups: []string{"Glutes"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
	}
}

func newPlanDayPlanner(t *testing.T, p Preferences) *Planner {
	t.Helper()
	return NewPlanner(p, planDayExercises(), nil)
}

func TestPlanner_PlanDay_IsolatedDateDefaultsToFullBody(t *testing.T) {
	// Empty prefs → isolated date → adjacency rule yields CategoryFullBody.
	wp := newPlanDayPlanner(t, Preferences{}) //nolint:exhaustruct // all zero on purpose.
	wed := date(monday2026Date(), 2)

	sess, err := wp.PlanDay(wed, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if sess.WorkoutType() != CategoryFullBody {
		t.Errorf("WorkoutType = %s, want full body", sess.WorkoutType())
	}
	// Default exercise count for unscheduled day = exercisesMedium = 3.
	if len(sess.ExerciseSets) != exercisesMedium {
		t.Errorf("ExerciseSets count = %d, want %d", len(sess.ExerciseSets), exercisesMedium)
	}
}

func TestPlanner_PlanDay_AdjacencyToScheduledDayPicksUpperOrLower(t *testing.T) {
	// Prefs: Tue scheduled. For Mon (yesterday is Sun=off, tomorrow is Tue=on)
	// the adjacency rule yields CategoryLower.
	wp := newPlanDayPlanner(t, prefs(time.Tuesday))
	mon := monday2026Date()

	sess, err := wp.PlanDay(mon, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if sess.WorkoutType() != CategoryLower {
		t.Errorf("WorkoutType = %s, want lower (today on, tomorrow on)", sess.WorkoutType())
	}
}

func TestPlanner_PlanDay_PeriodizationMatchesWeeklyPlannerForScheduledDate(t *testing.T) {
	// Mon, Wed, Fri scheduled. Plan(monday) assigns periodization by workoutDays index:
	// Mon=idx0 first, Wed=idx1 second, Fri=idx2 first. PlanDay must agree for each.
	p := prefs(time.Monday, time.Wednesday, time.Friday)
	wp := newPlanDayPlanner(t, p)
	mon := monday2026Date()

	weekly, err := wp.Plan(mon)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, want := range weekly {
		var got Session
		got, err = wp.PlanDay(want.Date, nil)
		if err != nil {
			t.Fatalf("PlanDay(%s): %v", want.Date.Weekday(), err)
		}
		if got.PeriodizationType != want.PeriodizationType {
			t.Errorf("PlanDay(%s) PeriodizationType = %s, want %s (matches weekly planner)",
				want.Date.Weekday(), got.PeriodizationType, want.PeriodizationType)
		}
	}
}

func TestPlanner_PlanDay_AvoidsUsedExercises(t *testing.T) {
	// Force the upper-only pool by picking Tue with Mon and Wed scheduled.
	// Then mark exercises 1,2 as used; only id 3 (Overhead Press) remains.
	p := prefs(time.Monday, time.Wednesday)
	wp := newPlanDayPlanner(t, p)
	tue := date(monday2026Date(), 1) // unscheduled Tuesday between two on days
	used := map[int]bool{1: true, 2: true}

	sess, err := wp.PlanDay(tue, used)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	for _, es := range sess.ExerciseSets {
		if used[es.Exercise.ID] {
			t.Errorf("PlanDay returned used exercise id=%d", es.Exercise.ID)
		}
	}
}

func TestPlanner_PlanDay_UsesPrefsExerciseCountWhenScheduled(t *testing.T) {
	// Long-day prefs (90 min) on Wednesday yields exercisesLong (4).
	p := Preferences{WednesdayMinutes: 90} //nolint:exhaustruct // only Wednesday duration is relevant to this test.
	wp := newPlanDayPlanner(t, p)
	wed := date(monday2026Date(), 2)

	sess, err := wp.PlanDay(wed, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.ExerciseSets) != exercisesLong {
		t.Errorf("ExerciseSets count = %d, want %d (long day)", len(sess.ExerciseSets), exercisesLong)
	}
}

func TestPlanner_PlanDay_EmptyCategoryPoolReturnsError(t *testing.T) {
	// Pool contains only Upper and FullBody. Pick a day whose category is Lower:
	// adjacency requires "yesterday is workout day" → Upper. So we need Lower:
	// today is on, tomorrow is on (gives Lower) → schedule Mon+Tue, ask for Mon.
	// Remove all Lower exercises from the pool to trigger the error.
	all := planDayExercises()
	noLower := make([]Exercise, 0, len(all))
	for _, ex := range all {
		if ex.Category != CategoryLower {
			noLower = append(noLower, ex)
		}
	}
	wp := NewPlanner(prefs(time.Monday, time.Tuesday), noLower, nil)
	mon := monday2026Date()

	_, err := wp.PlanDay(mon, nil)
	if err == nil {
		t.Fatal("PlanDay must error when category pool is empty")
	}
	if !errors.Is(err, errNoExercisesForCategory) {
		t.Errorf("err = %v, want wrap of errNoExercisesForCategory", err)
	}
}
