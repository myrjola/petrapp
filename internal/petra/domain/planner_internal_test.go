package domain

import (
	"fmt"
	"testing"
	"time"
)

// monday2026Date returns 2026-01-05, a known Monday.
func monday2026Date() time.Time {
	return time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
}

func date(base time.Time, offsetDays int) time.Time {
	return base.AddDate(0, 0, offsetDays)
}

func prefs(days ...time.Weekday) Preferences {
	p := Preferences{} //nolint:exhaustruct // RestNotificationsEnabled irrelevant to planner tests.
	for _, d := range days {
		p.Minutes[d] = minutesMedium
	}
	return p
}

func TestDetermineCategory(t *testing.T) {
	t.Parallel()

	monday := monday2026Date()
	tests := []struct {
		name     string
		prefs    Preferences
		date     time.Time
		expected Category
	}{
		{
			name:     "isolated day is full body",
			prefs:    prefs(time.Monday, time.Wednesday, time.Friday),
			date:     monday, // Mon: tomorrow=Tue not workout, yesterday=Sun not workout
			expected: CategoryFullBody,
		},
		{
			name:     "first of consecutive days is lower",
			prefs:    prefs(time.Monday, time.Tuesday),
			date:     monday, // Mon: tomorrow=Tue is workout
			expected: CategoryLower,
		},
		{
			name:     "second of consecutive days is upper",
			prefs:    prefs(time.Monday, time.Tuesday),
			date:     date(monday, 1), // Tue: yesterday=Mon was workout
			expected: CategoryUpper,
		},
		{
			name:     "week wrap: Sunday before Monday is lower",
			prefs:    prefs(time.Sunday, time.Monday, time.Tuesday),
			date:     date(monday, 6), // Sun (next week context doesn't matter — prefs wrap)
			expected: CategoryLower,   // Sun: today=workout, tomorrow=Mon=workout
		},
		{
			name:     "week wrap: Monday after Sunday is upper",
			prefs:    prefs(time.Sunday, time.Monday),
			date:     monday, // Mon: yesterday=Sun=workout
			expected: CategoryUpper,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			wp := NewPlanner(tt.prefs, nil, nil)
			got := wp.determineCategory(tt.date)
			if got != tt.expected {
				t.Errorf("determineCategory(%s) = %s, want %s", tt.date.Weekday(), got, tt.expected)
			}
		})
	}
}

func TestFirstSessionPeriodizationType(t *testing.T) {
	t.Parallel()

	t.Run("consecutive weeks alternate for odd exercise count", func(t *testing.T) {
		t.Parallel()
		// Mon/Wed/Fri at 60 min = 3 exercises each = 9 exercises/week (odd).
		p := prefs(time.Monday, time.Wednesday, time.Friday)
		wp := NewPlanner(p, nil, nil)

		monday1 := monday2026Date()                  // week N
		monday2 := monday2026Date().AddDate(0, 0, 7) // week N+1

		pt1 := wp.firstSessionPeriodizationType(monday1)
		pt2 := wp.firstSessionPeriodizationType(monday2)

		if pt1 == pt2 {
			t.Errorf("consecutive weeks must alternate: both got %v", pt1)
		}
	})

	t.Run("consecutive weeks alternate for even exercise count", func(t *testing.T) {
		t.Parallel()
		// Mon/Wed at 60 min = 3 exercises each = 6 exercises/week (even).
		p := prefs(time.Monday, time.Wednesday)
		wp := NewPlanner(p, nil, nil)

		monday1 := monday2026Date()
		monday2 := monday2026Date().AddDate(0, 0, 7)

		pt1 := wp.firstSessionPeriodizationType(monday1)
		pt2 := wp.firstSessionPeriodizationType(monday2)

		if pt1 == pt2 {
			t.Errorf("consecutive weeks must alternate even for even exercise count: both got %v", pt1)
		}
	})

	t.Run("determinism", func(t *testing.T) {
		t.Parallel()
		p := prefs(time.Monday, time.Wednesday, time.Friday)
		wp := NewPlanner(p, nil, nil)

		monday1 := monday2026Date()
		pt1 := wp.firstSessionPeriodizationType(monday1)
		if wp.firstSessionPeriodizationType(monday1) != pt1 {
			t.Error("firstSessionPeriodizationType is not deterministic")
		}
	})
}

func minimalExercises() []Exercise {
	return []Exercise{
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 1, Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads", "Glutes"}, SecondaryMuscleGroups: nil,
			DefaultStartingSeconds: nil,
			RepMin:                 new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 2, Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings"}, SecondaryMuscleGroups: nil,
			DefaultStartingSeconds: nil,
			RepMin:                 new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest", "Triceps", "Shoulders"}, SecondaryMuscleGroups: nil,
			DefaultStartingSeconds: nil,
			RepMin:                 new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 4, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Lats", "Upper Back"}, SecondaryMuscleGroups: nil,
			DefaultStartingSeconds: nil,
			RepMin:                 new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 5, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Biceps"}, SecondaryMuscleGroups: nil,
			DefaultStartingSeconds: nil,
			RepMin:                 new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 6, Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings", "Glutes"}, SecondaryMuscleGroups: nil,
			DefaultStartingSeconds: nil,
			RepMin:                 new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 7, Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			DefaultStartingSeconds: nil,
			RepMin:                 new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 8, Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads"}, SecondaryMuscleGroups: nil,
			DefaultStartingSeconds: nil,
			RepMin:                 new(5), RepMax: new(10)},
	}
}

func minimalTargets() []MuscleGroupTarget {
	return []MuscleGroupTarget{
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Shoulders", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Triceps", MinSets: 8, MaxSets: 16},
		{MuscleGroupName: "Biceps", MinSets: 8, MaxSets: 16},
		{MuscleGroupName: "Upper Back", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Lats", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Quads", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Hamstrings", MinSets: 8, MaxSets: 16},
		{MuscleGroupName: "Glutes", MinSets: 8, MaxSets: 16},
	}
}

func TestSelectExercises_CategoryFilter(t *testing.T) {
	t.Parallel()

	p := prefs(time.Tuesday)
	wp := NewPlanner(p, minimalExercises(), minimalTargets())

	t.Run("lower day only selects lower exercises", func(t *testing.T) {
		t.Parallel()
		load := map[string]float64{}
		used := map[int]bool{}
		slots := wp.selectExercisesForDayWithPeriodization(
			CategoryLower, 2, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
		)
		if len(slots) != 2 {
			t.Fatalf("want 2 slots, got %d", len(slots))
		}
		for _, s := range slots {
			ex := findExercise(wp.Exercises, s.Exercise.ID)
			if ex.Category != CategoryLower {
				t.Errorf("lower day got exercise with category %s", ex.Category)
			}
		}
	})

	t.Run("upper day only selects upper exercises", func(t *testing.T) {
		t.Parallel()
		load := map[string]float64{}
		used := map[int]bool{}
		slots := wp.selectExercisesForDayWithPeriodization(
			CategoryUpper, 2, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
		)
		for _, s := range slots {
			ex := findExercise(wp.Exercises, s.Exercise.ID)
			if ex.Category != CategoryUpper {
				t.Errorf("upper day got exercise with category %s", ex.Category)
			}
		}
	})

	t.Run("full body day can select any category", func(t *testing.T) {
		t.Parallel()
		load := map[string]float64{}
		used := map[int]bool{}
		slots := wp.selectExercisesForDayWithPeriodization(
			CategoryFullBody, 3, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
		)
		seen := map[Category]bool{}
		for _, s := range slots {
			ex := findExercise(wp.Exercises, s.Exercise.ID)
			seen[ex.Category] = true
		}
		if !seen[CategoryLower] || !seen[CategoryUpper] {
			t.Error("full body day should draw from multiple categories with targets across both")
		}
	})
}

func TestSelectExercises_SessionDiversity(t *testing.T) {
	t.Parallel()

	t.Run("no primary muscle group repeats within a session", func(t *testing.T) {
		t.Parallel()
		// Three Chest-primary exercises in the pool; we ask for 3 slots.
		// Only one can be picked (no primary overlap); the other 2 must come
		// from non-Chest primaries (Triceps-only exercise).
		exercises := []Exercise{
			{ //nolint:exhaustruct // Test exercises omit display fields.
				ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
				RepMin: new(5), RepMax: new(10),
			},
			{ //nolint:exhaustruct // Test exercises omit display fields.
				ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest", "Triceps"}, SecondaryMuscleGroups: nil,
				RepMin: new(5), RepMax: new(10),
			},
			{ //nolint:exhaustruct // Test exercises omit display fields.
				ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Triceps"}, SecondaryMuscleGroups: nil,
				RepMin: new(5), RepMax: new(10),
			},
		}
		wp := NewPlanner(prefs(time.Tuesday), exercises, []MuscleGroupTarget{
			{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
			{MuscleGroupName: "Triceps", MinSets: 8, MaxSets: 16},
		})
		load := map[string]float64{}
		used := map[int]bool{}
		slots := wp.selectExercisesForDayWithPeriodization(
			CategoryUpper, 3, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
		)

		seenPrimary := map[string]bool{}
		for _, s := range slots {
			ex := findExercise(exercises, s.Exercise.ID)
			for _, mg := range ex.PrimaryMuscleGroups {
				if seenPrimary[mg] {
					t.Errorf("primary muscle group %q appears in two picks in the same session", mg)
				}
				seenPrimary[mg] = true
			}
		}
	})
}

func TestSelectExercises_WeekUsedExclusion(t *testing.T) {
	t.Parallel()

	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
	}
	wp := NewPlanner(prefs(time.Tuesday), exercises, []MuscleGroupTarget{
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Shoulders", MinSets: 10, MaxSets: 20},
	})
	load := map[string]float64{}
	used := map[int]bool{1: true} // Exercise 1 was used earlier in the week.

	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 1, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
	)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	if slots[0].Exercise.ID == 1 {
		t.Errorf("week-used exercise was picked anyway")
	}
}

func TestSelectExercises_TargetAwarePrefersUnderloadedMG(t *testing.T) {
	t.Parallel()
	// Pool has two equally-eligible exercises. Chest is at zero load,
	// Shoulders already at target. The Chest exercise must win.
	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
	}
	wp := NewPlanner(prefs(time.Tuesday), exercises, []MuscleGroupTarget{
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Shoulders", MinSets: 10, MaxSets: 20},
	})
	load := map[string]float64{"Shoulders": 10}
	used := map[int]bool{}
	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 1, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
	)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	if slots[0].Exercise.ID != 2 {
		t.Errorf("picked exercise %d (Shoulders); expected exercise 2 (Chest, under target)", slots[0].Exercise.ID)
	}
}

func TestSelectExercises_FallsBackToLowestIDWhenScoresEqual(t *testing.T) {
	t.Parallel()
	// Empty targets: every candidate scores 0. Picker must return the
	// lowest-id eligible candidate deterministically.
	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 7, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
	}
	wp := NewPlanner(prefs(time.Tuesday), exercises, nil)
	load := map[string]float64{}
	used := map[int]bool{}
	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 1, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
	)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	if slots[0].Exercise.ID != 3 {
		t.Errorf("got exercise %d; expected exercise 3 (lowest id among ties)", slots[0].Exercise.ID)
	}
}

func TestSelectExercises_TimeBasedExerciseGetsThreeSets(t *testing.T) {
	t.Parallel()
	plank := Exercise{ //nolint:exhaustruct // Test exercises omit display fields.
		ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeTime,
		PrimaryMuscleGroups: []string{"Abs"}, SecondaryMuscleGroups: nil,
		DefaultStartingSeconds: new(30),
	}
	wp := NewPlanner(prefs(time.Tuesday), []Exercise{plank}, []MuscleGroupTarget{
		{MuscleGroupName: "Abs", MinSets: 4, MaxSets: 8},
	})
	load := map[string]float64{}
	used := map[int]bool{}
	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 1, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
	)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	if len(slots[0].Sets) != defaultTimedSets {
		t.Errorf("time-based slot has %d sets, want %d", len(slots[0].Sets), defaultTimedSets)
	}
	for _, s := range slots[0].Sets {
		if s.TargetValue != 30 {
			t.Errorf("target seconds = %d, want 30", s.TargetValue)
		}
	}
}

func TestSelectExercises_WeightedExerciseSetCountMatchesDeriveScheme(t *testing.T) {
	t.Parallel()
	// A weighted exercise picked via selectExercisesForDayWithPeriodization
	// should carry the set count and per-set target reps that
	// DeriveScheme produces for the same exercise + periodization.
	bench := Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
		PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
		RepMin: new(5), RepMax: new(10),
	}
	wp := NewPlanner(prefs(time.Tuesday), []Exercise{bench}, []MuscleGroupTarget{
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
	})
	load := map[string]float64{}
	used := map[int]bool{}
	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 1, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
	)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	wantReps, wantSets := deriveSchemeForExercise(bench, PeriodizationStrength, false, 4)
	if len(slots[0].Sets) != wantSets {
		t.Errorf("set count = %d, want %d", len(slots[0].Sets), wantSets)
	}
	for i, s := range slots[0].Sets {
		if s.TargetValue != wantReps {
			t.Errorf("set %d target reps = %d, want %d", i, s.TargetValue, wantReps)
		}
	}
}

func TestSelectExercises_GracefulDegradationWhenAllSharePrimaryMG(t *testing.T) {
	t.Parallel()
	// Pool: three exercises all primary on Chest. The session asks for 3
	// slots but only the first pick can satisfy the no-primary-overlap
	// rule; the loop must stop early and return one slot (graceful
	// degradation), not loop forever or panic.
	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
	}
	wp := NewPlanner(prefs(time.Tuesday), exercises, []MuscleGroupTarget{
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
	})
	load := map[string]float64{}
	used := map[int]bool{}
	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 3, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, used, load,
	)
	if len(slots) != 1 {
		t.Errorf("want 1 slot (graceful degradation under primary-overlap exhaustion), got %d", len(slots))
	}
}

func TestPlan_DoesNotRepeatExercisesAcrossDays(t *testing.T) {
	t.Parallel()
	exercises := minimalExercises()
	targets := minimalTargets()
	monday := monday2026Date()
	p := prefs(time.Monday, time.Tuesday, time.Thursday)
	wp := NewPlanner(p, exercises, targets)

	plan, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	used := map[int]bool{}
	for i := range plan.Sessions {
		for _, slot := range plan.Sessions[i].Slots {
			if used[slot.Exercise.ID] {
				t.Errorf("exercise %d appears in two sessions across the week", slot.Exercise.ID)
			}
			used[slot.Exercise.ID] = true
		}
	}
}

func findExercise(exercises []Exercise, id int) Exercise {
	for _, ex := range exercises {
		if ex.ID == id {
			return ex
		}
	}
	panic(fmt.Sprintf("exercise %d not found", id))
}

func TestPlanner_DeloadWeekForcesHypertrophyAndReducesSets(t *testing.T) {
	t.Parallel()

	// Anchor on the same Monday we'll plan: week 0 of length 4 would NOT be a
	// deload (we want length-1 → 3, so plan on a date that is anchor + 21 days).
	anchor := time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC) // Monday
	planMonday := anchor.AddDate(0, 0, 21)                         // week 3 of 4 → deload

	prefs := Preferences{ //nolint:exhaustruct // RestNotificationsEnabled and other UI prefs irrelevant.
		Minutes:         [7]int{time.Monday: 60, time.Tuesday: 60},
		DeloadEnabled:   true,
		MesocycleLength: 4,
		MesocycleAnchor: anchor,
	}
	// Use a fixture exercise list with rep windows that produce 3 normal sets
	// (mid rep band 8–12 with Hypertrophy → DeriveScheme gives 3 sets).
	repMin, repMax := 8, 12
	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID:                  1,
			Name:                "Bench Press",
			Category:            CategoryUpper,
			ExerciseType:        ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"chest"},
			RepMin:              &repMin,
			RepMax:              &repMax,
		},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID:                  2,
			Name:                "Squat",
			Category:            CategoryLower,
			ExerciseType:        ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"quads"},
			RepMin:              &repMin,
			RepMax:              &repMax,
		},
	}
	targets := []MuscleGroupTarget{
		{MuscleGroupName: "chest", MinSets: 6, MaxSets: 12},
		{MuscleGroupName: "quads", MinSets: 6, MaxSets: 12},
	}
	wp := NewPlanner(prefs, exercises, targets)
	plan, err := wp.Plan(planMonday)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var sessions []Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].Slots) > 0 {
			sessions = append(sessions, plan.Sessions[i])
		}
	}
	if len(sessions) == 0 {
		t.Fatal("expected at least one session")
	}
	for _, s := range sessions {
		if !s.IsDeload {
			t.Errorf("session %s IsDeload = false, want true", s.Date.Format("2006-01-02"))
		}
		if s.PeriodizationType != PeriodizationHypertrophy {
			t.Errorf(
				"session %s PeriodizationType = %s, want hypertrophy",
				s.Date.Format("2006-01-02"),
				s.PeriodizationType,
			)
		}
		for _, es := range s.Slots {
			// Normal high-rep band has 3 sets. Deload drops to 2.
			if len(es.Sets) != 2 {
				t.Errorf("session %s, exercise %s: %d sets, want 2 (deload drops one set)",
					s.Date.Format("2006-01-02"), es.Exercise.Name, len(es.Sets))
			}
			for _, set := range es.Sets {
				if set.TargetValue != 12 {
					t.Errorf("set TargetValue = %d, want 12 (repMax for hypertrophy)", set.TargetValue)
				}
			}
		}
	}
}

func TestPlanner_NonDeloadWeekUnchanged(t *testing.T) {
	t.Parallel()

	anchor := time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC)
	planMonday := anchor.AddDate(0, 0, 7) // week 1 → not a deload

	p := Preferences{ //nolint:exhaustruct // RestNotificationsEnabled and other UI prefs irrelevant.
		Minutes:         [7]int{time.Monday: 60},
		DeloadEnabled:   true,
		MesocycleLength: 4,
		MesocycleAnchor: anchor,
	}
	repMin, repMax := 8, 12
	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 1, Name: "Bench", Category: CategoryFullBody,
			ExerciseType:        ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"chest"},
			RepMin:              &repMin, RepMax: &repMax,
		},
	}
	targets := []MuscleGroupTarget{{MuscleGroupName: "chest", MinSets: 3, MaxSets: 6}}
	wp := NewPlanner(p, exercises, targets)
	plan, err := wp.Plan(planMonday)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var sessions []Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].Slots) > 0 {
			sessions = append(sessions, plan.Sessions[i])
		}
	}
	for _, s := range sessions {
		if s.IsDeload {
			t.Errorf("session %s IsDeload = true, want false (week 1 is not deload)", s.Date.Format("2006-01-02"))
		}
	}
}

func TestPlan(t *testing.T) {
	t.Parallel()

	monday := monday2026Date()
	exercises := minimalExercises()
	targets := minimalTargets()

	t.Run("returns error for non-Monday start date", func(t *testing.T) {
		t.Parallel()
		p := prefs(time.Monday, time.Wednesday)
		wp := NewPlanner(p, exercises, targets)
		_, err := wp.Plan(date(monday, 1)) // Tuesday.
		if err == nil {
			t.Error("want error for non-Monday start date, got nil")
		}
	})

	t.Run("returns error when no workout days scheduled", func(t *testing.T) {
		t.Parallel()
		wp := NewPlanner(prefs(), exercises, targets)
		_, err := wp.Plan(monday)
		if err == nil {
			t.Error("want error when no workout days scheduled, got nil")
		}
	})

	t.Run("returns one session per scheduled day", func(t *testing.T) {
		t.Parallel()
		p := prefs(time.Monday, time.Wednesday, time.Friday)
		wp := NewPlanner(p, exercises, targets)

		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		var sessions []Session
		for i := range plan.Sessions {
			if len(plan.Sessions[i].Slots) > 0 {
				sessions = append(sessions, plan.Sessions[i])
			}
		}
		if len(sessions) != 3 {
			t.Fatalf("want 3 sessions, got %d", len(sessions))
		}
	})

	t.Run("session dates match scheduled weekdays", func(t *testing.T) {
		t.Parallel()
		p := prefs(time.Monday, time.Wednesday, time.Friday)
		wp := NewPlanner(p, exercises, targets)

		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		var sessions []Session
		for i := range plan.Sessions {
			if len(plan.Sessions[i].Slots) > 0 {
				sessions = append(sessions, plan.Sessions[i])
			}
		}
		expected := []time.Weekday{time.Monday, time.Wednesday, time.Friday}
		for i, sess := range sessions {
			if sess.Date.Weekday() != expected[i] {
				t.Errorf("session %d: want %s, got %s", i, expected[i], sess.Date.Weekday())
			}
		}
	})

	t.Run("each session has correct exercise count for duration", func(t *testing.T) {
		t.Parallel()
		// 60 min: strength → 3 exercises, hypertrophy → 4 exercises.
		p := prefs(time.Monday, time.Wednesday)
		wp := NewPlanner(p, exercises, targets)

		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		var sessions []Session
		for i := range plan.Sessions {
			if len(plan.Sessions[i].Slots) > 0 {
				sessions = append(sessions, plan.Sessions[i])
			}
		}
		for _, sess := range sessions {
			want := exercisesMedium
			if sess.PeriodizationType == PeriodizationHypertrophy && !sess.IsDeload {
				want = exercisesMediumHypertrophy
			}
			if len(sess.Slots) != want {
				t.Errorf("60-min %s session: want %d exercises, got %d",
					sess.PeriodizationType, want, len(sess.Slots))
			}
		}
	})

	t.Run("consecutive sessions alternate periodization", func(t *testing.T) {
		t.Parallel()
		p := prefs(time.Monday, time.Tuesday)
		wp := NewPlanner(p, exercises, targets)

		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		var sessions []Session
		for i := range plan.Sessions {
			if len(plan.Sessions[i].Slots) > 0 {
				sessions = append(sessions, plan.Sessions[i])
			}
		}
		if len(sessions) < 2 {
			t.Fatal("need at least 2 sessions to test alternation")
		}
		if sessions[0].PeriodizationType == sessions[1].PeriodizationType {
			t.Error("consecutive sessions must have different periodization types")
		}
	})
}

func TestMondayOf_UsesLocalCalendarAnchoredToUTC(t *testing.T) {
	t.Parallel()

	helsinki, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Fatalf("load Europe/Helsinki: %v", err)
	}

	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			// Sunday 00:32 EEST: previously Truncate(24h) rolled the result
			// back into Sunday in local time.
			name: "early-morning Sunday in EEST returns previous Monday",
			in:   time.Date(2026, 5, 3, 0, 32, 41, 0, helsinki),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Monday after midnight EEST returns same Monday",
			in:   time.Date(2026, 5, 4, 0, 32, 0, 0, helsinki),
			want: time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Monday afternoon UTC returns same Monday",
			in:   time.Date(2026, 4, 27, 14, 0, 0, 0, time.UTC),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Tuesday 02:00 EEST returns same week's Monday",
			in:   time.Date(2026, 4, 28, 2, 0, 0, 0, helsinki),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Saturday late evening UTC returns same week's Monday",
			in:   time.Date(2026, 5, 2, 23, 59, 0, 0, time.UTC),
			want: time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MondayOf(tt.in)
			if !got.Equal(tt.want) {
				t.Errorf("MondayOf(%s) = %s, want %s", tt.in, got, tt.want)
			}
			if got.Weekday() != time.Monday {
				t.Errorf("MondayOf(%s).Weekday() = %s, want Monday", tt.in, got.Weekday())
			}
		})
	}
}

func Test_StartOfDay_TruncatesToUTCMidnight(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "UTC noon collapses to UTC midnight",
			in:   time.Date(2026, 5, 24, 12, 30, 0, 0, time.UTC),
			want: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "UTC midnight is fixed point",
			in:   time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Local late-evening uses local calendar date",
			in:   time.Date(2026, 5, 24, 23, 30, 0, 0, loc),
			want: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := StartOfDay(tc.in)
			if !got.Equal(tc.want) {
				t.Errorf("StartOfDay(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func Test_exercisesPerSession_PeriodizationAware(t *testing.T) {
	t.Parallel()

	// Build a Preferences value where each weekday carries a different minutes
	// value, so the test can pick a weekday to control the minutes input.
	p := Preferences{ //nolint:exhaustruct // RestNotificationsEnabled and mesocycle fields irrelevant.
		Minutes: [7]int{
			time.Monday:    minutesLong,   // 90
			time.Tuesday:   minutesMedium, // 60
			time.Wednesday: 45,
		},
	}

	tests := []struct {
		name     string
		weekday  time.Weekday
		pt       PeriodizationType
		isDeload bool
		want     int
	}{
		{"90 strength non-deload", time.Monday, PeriodizationStrength, false, exercisesLong},
		{"90 hypertrophy non-deload", time.Monday, PeriodizationHypertrophy, false, exercisesLongHypertrophy},
		{"90 hypertrophy deload", time.Monday, PeriodizationHypertrophy, true, exercisesLong},
		{"60 strength non-deload", time.Tuesday, PeriodizationStrength, false, exercisesMedium},
		{"60 hypertrophy non-deload", time.Tuesday, PeriodizationHypertrophy, false, exercisesMediumHypertrophy},
		{"60 hypertrophy deload", time.Tuesday, PeriodizationHypertrophy, true, exercisesMedium},
		{"45 strength non-deload", time.Wednesday, PeriodizationStrength, false, exercisesShort},
		{"45 hypertrophy non-deload", time.Wednesday, PeriodizationHypertrophy, false, exercisesShort},
		{"45 hypertrophy deload", time.Wednesday, PeriodizationHypertrophy, true, exercisesShort},
		{"0 strength", time.Thursday, PeriodizationStrength, false, 0},
		{"0 hypertrophy", time.Thursday, PeriodizationHypertrophy, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := exercisesPerSession(p, tt.weekday, tt.pt, tt.isDeload)
			if got != tt.want {
				t.Errorf("exercisesPerSession(weekday=%s, pt=%s, deload=%v) = %d, want %d",
					tt.weekday, tt.pt, tt.isDeload, got, tt.want)
			}
		})
	}
}

func Test_scoreCandidate(t *testing.T) {
	t.Parallel()

	bench := Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID:                    1,
		Category:              CategoryUpper,
		ExerciseType:          ExerciseTypeWeighted,
		PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
		SecondaryMuscleGroups: []string{"Shoulders"},
		RepMin:                new(5), RepMax: new(10),
	}

	targets := map[string]MuscleGroupTarget{
		"Chest":     {MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
		"Triceps":   {MuscleGroupName: "Triceps", MinSets: 8, MaxSets: 16},
		"Shoulders": {MuscleGroupName: "Shoulders", MinSets: 10, MaxSets: 20},
	}

	t.Run("positive when pulling under-target MGs up", func(t *testing.T) {
		t.Parallel()
		// Empty load: every targeted MG at full deficit.
		load := map[string]float64{}
		// Strength + 5-10 window: reps=5, sets=4 (DeriveScheme low band).
		// contrib: Chest=4, Triceps=4, Shoulders=2 (secondary).
		score := scoreCandidate(bench, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, load, targets)
		// Chest:    segmentReward(0, 4, 10, 20) = 4*below(3)         = 12.
		// Triceps:  segmentReward(0, 4, 8, 16)  = 4*below(3)         = 12.
		// Shoulders:segmentReward(0, 2, 10, 20) = 2*below(3)         =  6.
		// Total = 12 + 12 + 6 = 30.
		if score != 30 {
			t.Errorf("score = %v, want 30", score)
		}
	})

	t.Run("positive but lower when MGs are at floor", func(t *testing.T) {
		t.Parallel()
		// MGs already at their floor: adding more sets earns aboveGoalSetReward.
		load := map[string]float64{"Chest": 10, "Triceps": 8, "Shoulders": 10}
		score := scoreCandidate(bench, PeriodizationStrength, false, weekVolume{sets: 4, progress: 0}, load, targets)
		// Chest:    segmentReward(10, 4, 10, 20) = 4*above(1)  =  4.
		// Triceps:  segmentReward(8, 4, 8, 16)   = 4*above(1)  =  4.
		// Shoulders:segmentReward(10, 2, 10, 20)  = 2*above(1) =  2.
		// Total = 4 + 4 + 2 = 10.
		if score != 10 {
			t.Errorf("score = %v, want 10", score)
		}
	})

	t.Run("zero when no targeted MG is touched", func(t *testing.T) {
		t.Parallel()
		calfRaise := Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    99,
			Category:              CategoryLower,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Calves"},
			SecondaryMuscleGroups: nil,
			RepMin:                new(10), RepMax: new(20),
		}
		load := map[string]float64{}
		score := scoreCandidate(
			calfRaise,
			PeriodizationStrength,
			false,
			weekVolume{sets: 4, progress: 0},
			load,
			targets,
		)
		if score != 0 {
			t.Errorf("score = %v, want 0", score)
		}
	})

	t.Run("deload reduces set count", func(t *testing.T) {
		t.Parallel()
		load := map[string]float64{}
		// Strength + deload + 5-10 window: reps=10 (deload forces hypertrophy),
		// base sets = 3 (mid band, 6 <= reps <= 10), deload drops to 2.
		// contrib: Chest=2, Triceps=2, Shoulders=1 (secondary).
		score := scoreCandidate(bench, PeriodizationStrength, true, weekVolume{sets: 3, progress: 0}, load, targets)
		// Chest:    segmentReward(0, 2, 10, 20) = 2*below(3) = 6.
		// Triceps:  segmentReward(0, 2, 8, 16)  = 2*below(3) = 6.
		// Shoulders:segmentReward(0, 1, 10, 20) = 1*below(3) = 3.
		// Total = 6 + 6 + 3 = 15.
		if score != 15 {
			t.Errorf("score = %v, want 15", score)
		}
	})
}

func Test_Plan_HypertrophyDaysGetExtraExerciseInMixedWeek(t *testing.T) {
	t.Parallel()

	// Anchor on a strength-first Monday so the alternation is deterministic.
	monday := monday2026Date()
	pl := &Planner{} //nolint:exhaustruct // only firstSessionPeriodizationType is used.
	if pl.firstSessionPeriodizationType(monday) != PeriodizationStrength {
		monday = monday.AddDate(0, 0, 7)
	}

	// Two 60-min days with no adjacent workout day so both are CategoryFullBody
	// (per determineCategory). FullBody days accept all exercise categories,
	// so the planner can draw freely from the 8-exercise minimalExercises()
	// pool to fill 3 + 4 = 7 unique slots.
	// Strength-first alternation on a 2-day week → [strength, hypertrophy] →
	// [exercisesMedium=3, exercisesMediumHypertrophy=4] under the new bump rule.
	p := Preferences{ //nolint:exhaustruct // RestNotificationsEnabled and mesocycle fields irrelevant.
		Minutes: [7]int{
			time.Monday:   minutesMedium,
			time.Thursday: minutesMedium,
		},
	}
	wp := NewPlanner(p, minimalExercises(), minimalTargets())

	plan, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	var sessions []Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].Slots) > 0 {
			sessions = append(sessions, plan.Sessions[i])
		}
	}
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}

	wantCount := []int{exercisesMedium, exercisesMediumHypertrophy}
	wantPT := []PeriodizationType{PeriodizationStrength, PeriodizationHypertrophy}
	for i, sess := range sessions {
		if sess.PeriodizationType != wantPT[i] {
			t.Errorf("session %d periodization: want %s, got %s", i, wantPT[i], sess.PeriodizationType)
		}
		if got := len(sess.Slots); got != wantCount[i] {
			t.Errorf("session %d (%s) exercise count: want %d, got %d",
				i, sess.PeriodizationType, wantCount[i], got)
		}
	}
}

// seedExercises mirrors the 39 exercises in internal/repository/fixtures.sql
// verbatim (IDs, categories, types, rep windows, and primary/secondary
// muscle groups all match). It keeps the regression test pure-domain while
// exercising the algorithm against the actual seed users start with.
// Update both this helper and fixtures.sql together when seed exercises
// change.
func seedExercises() []Exercise {
	return []Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Name: "Deadlift", Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Hamstrings", "Lower Back"},
			SecondaryMuscleGroups: []string{"Forearms", "Lats", "Quads", "Traps", "Upper Back"},
			RepMin:                new(3), RepMax: new(6),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Name: "Bench Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
			SecondaryMuscleGroups: []string{"Abs", "Forearms", "Shoulders"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 3, Name: "Tricep Pushdown", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Triceps"},
			SecondaryMuscleGroups: []string{"Shoulders"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 4, Name: "Dumbbell Biceps Curl", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Biceps"},
			SecondaryMuscleGroups: []string{"Forearms"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 5, Name: "Lateral Raise", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{"Traps", "Upper Back"},
			RepMin:                new(10), RepMax: new(20),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 6, Name: "Dumbbell Shoulder Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{"Triceps", "Upper Back"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 7, Name: "Dumbbell Bench Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest"},
			SecondaryMuscleGroups: []string{"Shoulders", "Triceps"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 8, Name: "Cable Fly", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest"},
			SecondaryMuscleGroups: []string{"Shoulders", "Triceps"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 9, Name: "Pulldown", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Shoulders"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 10, Name: "Pulldown, Reverse Grip", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Biceps", "Lats"},
			SecondaryMuscleGroups: []string{"Forearms", "Upper Back"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 11, Name: "Seated Cable Row", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Lower Back"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 12, Name: "One-Arm Dumbbell Row", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Forearms"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 13, Name: "Abdominal Machine Crunch", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Abs"},
			SecondaryMuscleGroups: []string{"Obliques"},
			RepMin:                new(8), RepMax: new(15),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 14, Name: "Leg Press", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Calves", "Hamstrings"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 15, Name: "Leg Extension", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Quads"},
			SecondaryMuscleGroups: []string{"Hip Flexors"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 16, Name: "Leg Curl", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Hamstrings"},
			SecondaryMuscleGroups: []string{"Calves"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 17, Name: "Calf Raise", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Calves"},
			SecondaryMuscleGroups: []string{"Quads"},
			RepMin:                new(10), RepMax: new(20),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 18, Name: "Back Extension", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lower Back"},
			SecondaryMuscleGroups: []string{"Glutes", "Hamstrings"},
			RepMin:                new(8), RepMax: new(20),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 19, Name: "Push-Up", Category: CategoryUpper, ExerciseType: ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
			SecondaryMuscleGroups: []string{"Abs", "Forearms", "Shoulders", "Upper Back"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 20, Name: "Ab Wheel Rollout", Category: CategoryUpper, ExerciseType: ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Abs", "Obliques"},
			SecondaryMuscleGroups: []string{"Calves", "Glutes", "Hamstrings", "Lats", "Quads", "Shoulders"},
			RepMin:                new(8), RepMax: new(15),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 21, Name: "Plank", Category: CategoryUpper, ExerciseType: ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Abs"},
			SecondaryMuscleGroups: []string{"Glutes", "Hip Flexors", "Lower Back", "Obliques", "Quads", "Shoulders"},
			RepMin:                new(8), RepMax: new(15),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 22, Name: "Incline Dumbbell Bench Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest"},
			SecondaryMuscleGroups: []string{"Shoulders", "Triceps", "Upper Back"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 23, Name: "Romanian Deadlift", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Hamstrings"},
			SecondaryMuscleGroups: []string{"Lower Back"},
			RepMin:                new(8), RepMax: new(20),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 24, Name: "Assisted Pull-Up", Category: CategoryUpper, ExerciseType: ExerciseTypeAssisted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Forearms"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 25, Name: "Hip Abductor", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes"},
			SecondaryMuscleGroups: nil,
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 26, Name: "Hip Adductor", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Adductors"},
			SecondaryMuscleGroups: []string{"Glutes"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 27, Name: "Rotary Torso", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Obliques"},
			SecondaryMuscleGroups: []string{"Abs"},
			RepMin:                new(8), RepMax: new(15),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 28, Name: "Seated Calf Raise", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Calves"},
			SecondaryMuscleGroups: nil,
			RepMin:                new(10), RepMax: new(20),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 29, Name: "Squat", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Hamstrings", "Lower Back"},
			RepMin:                new(3), RepMax: new(6),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 30, Name: "Pec Fly", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest"},
			SecondaryMuscleGroups: []string{"Shoulders"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 31, Name: "Smith Machine Squat", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Abs", "Hamstrings"},
			RepMin:                new(3), RepMax: new(6),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 32, Name: "Overhead Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders", "Triceps"},
			SecondaryMuscleGroups: []string{"Abs", "Upper Back"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 33, Name: "Barbell Row", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Lower Back"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 34, Name: "Face Pull", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders", "Upper Back"},
			SecondaryMuscleGroups: []string{"Traps", "Triceps"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 35, Name: "Hip Thrust", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes"},
			SecondaryMuscleGroups: []string{"Hamstrings", "Quads"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 36, Name: "Bulgarian Split Squat", Category: CategoryLower, ExerciseType: ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Abs", "Hamstrings"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 37, Name: "Hammer Curl", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Biceps", "Forearms"},
			SecondaryMuscleGroups: []string{"Shoulders", "Triceps"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 38, Name: "Skull Crusher", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Triceps"},
			SecondaryMuscleGroups: []string{"Forearms", "Shoulders"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 39, Name: "Hanging Leg Raise", Category: CategoryUpper, ExerciseType: ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Abs", "Hip Flexors"},
			SecondaryMuscleGroups: []string{"Forearms", "Obliques"},
			RepMin:                new(5), RepMax: new(10),
		},
	}
}

func seedTargets() []MuscleGroupTarget {
	return []MuscleGroupTarget{
		{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Shoulders", MinSets: 6, MaxSets: 12},
		{MuscleGroupName: "Triceps", MinSets: 8, MaxSets: 16},
		{MuscleGroupName: "Biceps", MinSets: 8, MaxSets: 16},
		{MuscleGroupName: "Upper Back", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Lats", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Quads", MinSets: 10, MaxSets: 20},
		{MuscleGroupName: "Hamstrings", MinSets: 8, MaxSets: 18},
		{MuscleGroupName: "Glutes", MinSets: 8, MaxSets: 16},
	}
}

// prefs90 returns prefs with the given weekdays scheduled at 90 minutes
// (the size that triggers exercisesLong / exercisesLongHypertrophy).
func prefs90(days ...time.Weekday) Preferences {
	p := Preferences{} //nolint:exhaustruct // Other prefs irrelevant to this test.
	for _, d := range days {
		p.Minutes[d] = 90
	}
	return p
}

func TestPlan_TargetAwareBalanceUnderSeedExercises(t *testing.T) {
	t.Parallel()
	// Tue/Thu/Sat 90 min, all FullBody. This is user 24's current schedule;
	// under the old algorithm Shoulders/Triceps/Upper Back ballooned and
	// Chest/Quads sat under target.
	p := prefs90(time.Tuesday, time.Thursday, time.Saturday)
	wp := NewPlanner(p, seedExercises(), seedTargets())

	plan, err := wp.Plan(monday2026Date())
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	load := WeeklyPlannedLoad(planSessions(plan))
	for _, target := range seedTargets() {
		l := load[target.MuscleGroupName]
		t.Logf("%s planned %.1f / floor %d ceiling %d", target.MuscleGroupName, l, target.MinSets, target.MaxSets)
	}

	// Range-model bounds: every targeted MG should reach at least 0.7x its
	// floor (regression protection — Chest must not sit starved at 8 as it did
	// under the old algorithm) and must not exceed its ceiling by more than a
	// small slack (one exercise ≈ 3-4 sets can overshoot a ceiling on its
	// final placement before the over-MaxSets penalty steers the next pick).
	const ceilingSlack = 4.0
	for _, target := range seedTargets() {
		l := load[target.MuscleGroupName]
		lower := 0.7 * float64(target.MinSets)
		if l < lower {
			t.Errorf("%s planned %.1f is below 0.7x floor (%v)", target.MuscleGroupName, l, lower)
		}
		if l > float64(target.MaxSets)+ceilingSlack {
			t.Errorf("%s planned %.1f exceeds ceiling %d + slack %v",
				target.MuscleGroupName, l, target.MaxSets, ceilingSlack)
		}
	}
}

func Test_segmentReward_OrdersBelowAboveOverMax(t *testing.T) {
	t.Parallel()

	const goal, maxSets = 10.0, 20.0
	below := segmentReward(0, 1, goal, maxSets)  // one set entirely below the floor
	above := segmentReward(10, 1, goal, maxSets) // one set between floor and ceiling
	over := segmentReward(20, 1, goal, maxSets)  // one set entirely past the ceiling

	if !(below > above && above > 0 && 0 > over) {
		t.Errorf("want below > above > 0 > over; got below=%v above=%v over=%v", below, above, over)
	}
}

func Test_scoreCandidate_TagOnlyGroupContributesNothing(t *testing.T) {
	t.Parallel()

	repMin, repMax := 8, 12
	// Exercise whose only primary MG ("Traps") has no target row.
	tagOnly := Exercise{ //nolint:exhaustruct // test exercise omits display fields.
		ID: 1, Name: "Shrug", Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
		PrimaryMuscleGroups: []string{"Traps"}, RepMin: &repMin, RepMax: &repMax,
	}
	targets := map[string]MuscleGroupTarget{"Chest": {MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20}}

	got := scoreCandidate(
		tagOnly,
		PeriodizationHypertrophy,
		false,
		weekVolume{sets: 4, progress: 0},
		map[string]float64{},
		targets,
	)
	if got != 0 {
		t.Errorf("tag-only exercise scored %v, want 0", got)
	}
}

func Test_scoreCandidate_OverMaxPickLosesToFreshMuscle(t *testing.T) {
	t.Parallel()

	repMin, repMax := 8, 12
	fresh := Exercise{ //nolint:exhaustruct // test exercise omits display fields.
		ID: 1, Name: "Fresh", Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
		PrimaryMuscleGroups: []string{"Chest"}, RepMin: &repMin, RepMax: &repMax,
	}
	saturated := Exercise{ //nolint:exhaustruct // test exercise omits display fields.
		ID: 2, Name: "Saturated", Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
		PrimaryMuscleGroups: []string{"Biceps"}, RepMin: &repMin, RepMax: &repMax,
	}
	targets := map[string]MuscleGroupTarget{
		"Chest":  {MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
		"Biceps": {MuscleGroupName: "Biceps", MinSets: 8, MaxSets: 16},
	}
	// Biceps already well past its ceiling; Chest at zero.
	load := map[string]float64{"Biceps": 30}

	freshScore := scoreCandidate(
		fresh,
		PeriodizationHypertrophy,
		false,
		weekVolume{sets: 4, progress: 0},
		load,
		targets,
	)
	satScore := scoreCandidate(
		saturated,
		PeriodizationHypertrophy,
		false,
		weekVolume{sets: 4, progress: 0},
		load,
		targets,
	)
	if !(freshScore > satScore) {
		t.Errorf("fresh-muscle pick (%v) should beat over-ceiling pick (%v)", freshScore, satScore)
	}
	if satScore >= 0 {
		t.Errorf("over-ceiling pick should score negative, got %v", satScore)
	}
}

func planSessions(plan WeekPlan) []Session {
	var ss []Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].Slots) > 0 {
			ss = append(ss, plan.Sessions[i])
		}
	}
	return ss
}

func TestPlan_DeterministicAcrossRuns(t *testing.T) {
	t.Parallel()
	// Same inputs twice → byte-equal Sessions output. Guards against map
	// iteration order leaking into selection.
	p := prefs90(time.Tuesday, time.Thursday, time.Saturday)
	wp := NewPlanner(p, seedExercises(), seedTargets())

	monday := monday2026Date()
	planA, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan A failed: %v", err)
	}
	planB, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan B failed: %v", err)
	}

	for i := range 7 {
		if got, want := slotIDs(planA.Sessions[i]), slotIDs(planB.Sessions[i]); !sliceEqInt(got, want) {
			t.Errorf("day %d differs: A=%v B=%v", i, got, want)
		}
	}
}

func slotIDs(s Session) []int {
	ids := make([]int, len(s.Slots))
	for i, slot := range s.Slots {
		ids[i] = slot.Exercise.ID
	}
	return ids
}

func sliceEqInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func Test_goalForWeek(t *testing.T) {
	t.Parallel()

	chest := MuscleGroupTarget{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20}

	tests := []struct {
		name     string
		progress float64
		want     float64
	}{
		{"progress 0 → MinSets exactly", 0.0, 10.0},
		{"progress 1 → MaxSets exactly", 1.0, 20.0},
		{"progress 0.5 → midpoint", 0.5, 15.0},
		{"fractional lerp quantised to nearest 0.5", 1.0 / 3.0, 13.5}, // raw 13.333 → 13.5.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := goalForWeek(chest, tt.progress)
			if got != tt.want {
				t.Errorf("goalForWeek(progress=%v) = %v, want %v", tt.progress, got, tt.want)
			}
			// Tie-break invariant: the goal must always be a multiple of 0.5.
			// (No math import needed — int64 truncation suffices for the check.)
			if twice := got * 2; twice != float64(int64(twice)) {
				t.Errorf("goalForWeek(progress=%v) = %v is not a multiple of 0.5", tt.progress, got)
			}
		})
	}
}

func Test_scoreCandidate_GoalRampsWithProgress(t *testing.T) {
	t.Parallel()

	// A muscle sitting above its floor but below its ceiling: at progress 0 the
	// goal is the floor (sets earn the smaller above-goal reward); at progress 1
	// the goal has risen past the current load (sets earn the steeper below-goal
	// reward). So the same pick scores strictly higher later in the block.
	bench := Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
		PrimaryMuscleGroups: []string{"Chest"}, RepMin: new(8), RepMax: new(12),
	}
	targets := map[string]MuscleGroupTarget{
		"Chest": {MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20},
	}
	load := map[string]float64{"Chest": 12} // above floor (10), below ceiling (20).

	early := scoreCandidate(bench, PeriodizationHypertrophy, false, weekVolume{sets: 4, progress: 0}, load, targets)
	late := scoreCandidate(bench, PeriodizationHypertrophy, false, weekVolume{sets: 4, progress: 1}, load, targets)
	if !(late > early) {
		t.Errorf("ramped goal should score higher late in block: early=%v late=%v", early, late)
	}
}
