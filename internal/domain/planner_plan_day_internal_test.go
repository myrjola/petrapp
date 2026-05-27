package domain

import (
	"errors"
	"testing"
	"time"
)

// planDayExercises returns a small pool with Upper, Lower, and FullBody coverage
// across distinct primary muscles so PlanDay's non-conflict selection has room.
// The FullBody Plank carries the lowest ID so the lowest-id tie-break picks it
// first on FullBody days with empty targets, making sess.WorkoutType() return
// CategoryFullBody reliably in tests.
func planDayExercises() []Exercise {
	intPtr := func(v int) *int { return &v }
	return []Exercise{
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 1, Name: "Plank", Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Core"}, SecondaryMuscleGroups: nil,
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 2, Name: "Bench Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: []string{"Triceps"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 3, Name: "Row", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Upper Back"}, SecondaryMuscleGroups: []string{"Biceps"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 4, Name: "Overhead Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: []string{"Triceps"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 5, Name: "Squat", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads"}, SecondaryMuscleGroups: []string{"Glutes"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 6, Name: "Deadlift", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings"}, SecondaryMuscleGroups: []string{"Glutes"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
	}
}

func newPlanDayPlanner(t *testing.T, p Preferences) *Planner {
	t.Helper()
	return NewPlanner(p, planDayExercises(), nil)
}

func TestPlanner_PlanDay_IsolatedDateDefaultsToFullBody(t *testing.T) {
	t.Parallel()

	// Empty prefs → isolated date → adjacency rule yields CategoryFullBody.
	wp := newPlanDayPlanner(t, Preferences{}) //nolint:exhaustruct // all zero on purpose.
	wed := date(monday2026Date(), 2)

	sess, err := wp.PlanDay(wed, nil, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if sess.WorkoutType() != CategoryFullBody {
		t.Errorf("WorkoutType = %s, want full body", sess.WorkoutType())
	}
	// Default exercise count for unscheduled day = exercisesMedium = 3.
	if len(sess.Slots) != exercisesMedium {
		t.Errorf("Slots count = %d, want %d", len(sess.Slots), exercisesMedium)
	}
}

func TestPlanner_PlanDay_AdjacencyToScheduledTomorrowPicksLower(t *testing.T) {
	t.Parallel()

	// Prefs: Tue scheduled. For Mon (yesterday is Sun=off, tomorrow is Tue=on)
	// the adjacency rule yields CategoryLower — Lower whenever tomorrow is
	// scheduled, regardless of today.
	wp := newPlanDayPlanner(t, prefs(time.Tuesday))
	mon := monday2026Date()

	sess, err := wp.PlanDay(mon, nil, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if sess.WorkoutType() != CategoryLower {
		t.Errorf("WorkoutType = %s, want lower (tomorrow is scheduled)", sess.WorkoutType())
	}
}

func TestPlanner_PlanDay_PeriodizationMatchesWeeklyPlannerForScheduledDate(t *testing.T) {
	t.Parallel()

	// Mon, Wed, Fri scheduled. Plan(monday) assigns periodization by workoutDays index:
	// Mon=idx0 first, Wed=idx1 second, Fri=idx2 first. PlanDay must agree for each.
	p := prefs(time.Monday, time.Wednesday, time.Friday)
	wp := newPlanDayPlanner(t, p)
	mon := monday2026Date()

	weekly, err := wp.Plan(mon)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for i := range weekly.Sessions {
		want := weekly.Sessions[i]
		if len(want.Slots) == 0 {
			continue
		}
		var got Session
		got, err = wp.PlanDay(want.Date, nil, nil)
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
	t.Parallel()

	// Force the upper-only pool by picking Tue with Mon and Wed scheduled.
	// Mark exercises 1,2 as used; the planner must not return them. The
	// selector mutates the supplied used-set with the IDs it picks, so we
	// snapshot the pre-call IDs before checking.
	p := prefs(time.Monday, time.Wednesday)
	wp := newPlanDayPlanner(t, p)
	tue := date(monday2026Date(), 1) // unscheduled Tuesday between two on days
	used := map[int]bool{1: true, 2: true}
	preUsed := map[int]bool{}
	for id := range used {
		preUsed[id] = true
	}

	sess, err := wp.PlanDay(tue, used, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	for _, es := range sess.Slots {
		if preUsed[es.Exercise.ID] {
			t.Errorf("PlanDay returned used exercise id=%d", es.Exercise.ID)
		}
	}
}

func TestPlanner_PlanDay_UsesPrefsExerciseCountWhenScheduled(t *testing.T) {
	t.Parallel()

	// Long-day prefs (90 min) on Wednesday yields exercisesLong (4).
	p := Preferences{ //nolint:exhaustruct // only Wednesday duration is relevant to this test.
		Minutes: [7]int{time.Wednesday: 90},
	}
	wp := newPlanDayPlanner(t, p)
	wed := date(monday2026Date(), 2)

	sess, err := wp.PlanDay(wed, nil, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.Slots) != exercisesLong {
		t.Errorf("Slots count = %d, want %d (long day)", len(sess.Slots), exercisesLong)
	}
}

func TestPlanner_PlanDay_EmptyCategoryPoolReturnsError(t *testing.T) {
	t.Parallel()

	// Pool contains only Upper and FullBody. Pick a day whose category is Lower:
	// tomorrow is on (gives Lower) → schedule Mon+Tue, ask for Mon.
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

	_, err := wp.PlanDay(mon, nil, nil)
	if err == nil {
		t.Fatal("PlanDay must error when category pool is empty")
	}
	if !errors.Is(err, errNoExercisesForCategory) {
		t.Errorf("err = %v, want wrap of errNoExercisesForCategory", err)
	}
}

func TestPlanner_PlanDay_PeriodizationMatchesWeeklyPlannerForSundaySchedule(t *testing.T) {
	t.Parallel()

	// Mon+Sun schedule. Plan assigns Mon→idx0, Sun→idx1. PlanDay must agree
	// for Sunday — regression test for the Sunday=0 weekday-arithmetic bug.
	p := prefs(time.Monday, time.Sunday)
	wp := newPlanDayPlanner(t, p)
	mon := monday2026Date()
	sun := date(mon, 6)

	weekly, err := wp.Plan(mon)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var planSunPT PeriodizationType
	for i := range weekly.Sessions {
		s := weekly.Sessions[i]
		if s.Date.Equal(sun) {
			planSunPT = s.PeriodizationType
		}
	}

	got, err := wp.PlanDay(sun, nil, nil)
	if err != nil {
		t.Fatalf("PlanDay(Sunday): %v", err)
	}
	if got.PeriodizationType != planSunPT {
		t.Errorf("Sunday PeriodizationType = %s, want %s (matches weekly planner)",
			got.PeriodizationType, planSunPT)
	}
}

func TestPlanner_PlanDay_AvoidsAlreadyLoadedMuscleGroup(t *testing.T) {
	t.Parallel()
	// Pool: one Shoulders-primary exercise, one Chest-primary exercise.
	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
	}
	targets := []MuscleGroupTarget{
		{MuscleGroupName: "Shoulders", WeeklySetTarget: 10},
		{MuscleGroupName: "Chest", WeeklySetTarget: 10},
	}
	// Tuesday scheduled so category=FullBody (isolated day).
	p := Preferences{} //nolint:exhaustruct // Other prefs irrelevant.
	p.Minutes[time.Tuesday] = 60
	wp := NewPlanner(p, exercises, targets)

	weekLoad := map[string]float64{"Shoulders": 10} // Already at target.
	sess, err := wp.PlanDay(time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), nil, weekLoad)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.Slots) == 0 {
		t.Fatalf("no slots picked")
	}
	if sess.Slots[0].Exercise.ID != 2 {
		t.Errorf("first pick is exercise %d; expected exercise 2 (Chest, under target)", sess.Slots[0].Exercise.ID)
	}
}
