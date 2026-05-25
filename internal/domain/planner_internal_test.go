package domain

import (
	"fmt"
	"math/rand/v2"
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
	p := Preferences{ //nolint:exhaustruct // RestNotificationsEnabled irrelevant to planner tests.
		MondayMinutes:    0,
		TuesdayMinutes:   0,
		WednesdayMinutes: 0,
		ThursdayMinutes:  0,
		FridayMinutes:    0,
		SaturdayMinutes:  0,
		SundayMinutes:    0,
	}
	for _, d := range days {
		switch d {
		case time.Monday:
			p.MondayMinutes = minutesMedium
		case time.Tuesday:
			p.TuesdayMinutes = minutesMedium
		case time.Wednesday:
			p.WednesdayMinutes = minutesMedium
		case time.Thursday:
			p.ThursdayMinutes = minutesMedium
		case time.Friday:
			p.FridayMinutes = minutesMedium
		case time.Saturday:
			p.SaturdayMinutes = minutesMedium
		case time.Sunday:
			p.SundayMinutes = minutesMedium
		}
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
		{MuscleGroupName: "Chest", WeeklySetTarget: 10},
		{MuscleGroupName: "Shoulders", WeeklySetTarget: 10},
		{MuscleGroupName: "Triceps", WeeklySetTarget: 8},
		{MuscleGroupName: "Biceps", WeeklySetTarget: 8},
		{MuscleGroupName: "Upper Back", WeeklySetTarget: 10},
		{MuscleGroupName: "Lats", WeeklySetTarget: 10},
		{MuscleGroupName: "Quads", WeeklySetTarget: 10},
		{MuscleGroupName: "Hamstrings", WeeklySetTarget: 8},
		{MuscleGroupName: "Glutes", WeeklySetTarget: 8},
	}
}

func TestAllocateMuscleGroups(t *testing.T) {
	t.Parallel()

	// Mon(Lower), Tue(Upper), Thu(Full Body) schedule.
	monday := monday2026Date()
	p := prefs(time.Monday, time.Tuesday, time.Thursday)
	wp := NewPlanner(p, minimalExercises(), minimalTargets())

	mon := monday          // Lower
	tue := date(monday, 1) // Upper
	thu := date(monday, 3) // Full Body

	workoutDays := []time.Time{mon, tue, thu}
	categories := map[time.Time]Category{
		mon: CategoryLower,
		tue: CategoryUpper,
		thu: CategoryFullBody,
	}

	alloc := wp.allocateMuscleGroups(workoutDays, categories)

	// Lower muscle groups (Quads, Hamstrings, Glutes) must appear on Mon (Lower
	// compatible) and/or Thu (Full Body compatible), never on Tue (Upper only).
	for _, mg := range []string{"Quads", "Hamstrings", "Glutes"} {
		for _, assignedMG := range alloc[tue] {
			if assignedMG == mg {
				t.Errorf("lower muscle group %q must not be assigned to Upper day", mg)
			}
		}
	}

	// Upper muscle groups must not appear on Mon (Lower only).
	for _, mg := range []string{"Chest", "Shoulders", "Triceps", "Biceps", "Upper Back", "Lats"} {
		for _, assignedMG := range alloc[mon] {
			if assignedMG == mg {
				t.Errorf("upper muscle group %q must not be assigned to Lower day", mg)
			}
		}
	}

	// Every tracked muscle group must appear in at least 1 day's allocation.
	allGroups := make(map[string]bool)
	for _, groups := range alloc {
		for _, g := range groups {
			allGroups[g] = true
		}
	}
	for _, target := range minimalTargets() {
		if !allGroups[target.MuscleGroupName] {
			t.Errorf("muscle group %q not assigned to any day", target.MuscleGroupName)
		}
	}
}

func TestSelectExercisesForDay(t *testing.T) {
	t.Parallel()

	p := prefs(time.Monday, time.Tuesday, time.Thursday)
	wp := NewPlanner(p, minimalExercises(), minimalTargets())
	wp.rng = rand.New(rand.NewPCG(42, 0)) // fixed seed for determinism

	t.Run("lower day only selects lower exercises", func(t *testing.T) {
		t.Parallel()
		sets := wp.selectExercisesForDay(CategoryLower, []string{"Quads", "Hamstrings"}, 2)
		if len(sets) != 2 {
			t.Fatalf("want 2 exercise sets, got %d", len(sets))
		}
		for _, es := range sets {
			ex := findExercise(wp.Exercises, es.Exercise.ID)
			if ex.Category != CategoryLower {
				t.Errorf("lower day got exercise with category %s", ex.Category)
			}
		}
	})

	t.Run("upper day only selects upper exercises", func(t *testing.T) {
		t.Parallel()
		sets := wp.selectExercisesForDay(CategoryUpper, []string{"Chest", "Lats"}, 2)
		for _, es := range sets {
			ex := findExercise(wp.Exercises, es.Exercise.ID)
			if ex.Category != CategoryUpper {
				t.Errorf("upper day got exercise with category %s", ex.Category)
			}
		}
	})

	t.Run("full body day can select any category", func(t *testing.T) {
		t.Parallel()
		sets := wp.selectExercisesForDay(CategoryFullBody, []string{"Hamstrings", "Chest"}, 3)
		categorySet := make(map[Category]bool)
		for _, es := range sets {
			ex := findExercise(wp.Exercises, es.Exercise.ID)
			categorySet[ex.Category] = true
		}
		// With Hamstrings and Chest as priorities, expect both lower and upper exercises selected.
		if !categorySet[CategoryLower] || !categorySet[CategoryUpper] {
			t.Error("full body day should draw from multiple categories when priorities span both")
		}
	})

	t.Run("rep-based exercise set count comes from DeriveScheme", func(t *testing.T) {
		t.Parallel()
		sets := wp.selectExercisesForDay(CategoryUpper, []string{"Chest"}, 1)
		if len(sets) != 1 {
			t.Fatalf("want 1 exercise set, got %d", len(sets))
		}
		// With Strength + window 5-10, DeriveScheme returns 4 sets (reps=5 ≤ 5).
		expectedSets := DeriveScheme(5, 10, PeriodizationStrength, false).TargetSets
		if len(sets[0].Sets) != expectedSets {
			t.Errorf("want %d sets, got %d", expectedSets, len(sets[0].Sets))
		}
	})

	t.Run("strength periodization sets correct target value", func(t *testing.T) {
		t.Parallel()
		sets := wp.selectExercisesForDay(CategoryUpper, nil, 1)
		expectedReps := DeriveScheme(5, 10, PeriodizationStrength, false).TargetReps
		for _, s := range sets[0].Sets {
			if s.TargetValue != expectedReps {
				t.Errorf("strength set: want TargetValue=%d, got %d", expectedReps, s.TargetValue)
			}
		}
	})
}

func TestSelectExercisesForDaySessionDiversity(t *testing.T) {
	t.Parallel()

	t.Run("no primary muscle group overlap within session", func(t *testing.T) {
		t.Parallel()
		// Exercise pool: multiple exercises that could target overlapping muscles.
		exercises := []Exercise{
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: []string{"Triceps"},
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: []string{"Shoulders"},
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Shoulders", "Triceps"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 4, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Triceps"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
		}

		p := prefs(time.Tuesday) // 3 exercises
		wp := NewPlanner(p, exercises, nil)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		// Request 3 exercises with priority Chest, Shoulders, Triceps.
		// Expected: one exercise per primary muscle group, no overlaps.
		sets := wp.selectExercisesForDayWithPeriodization(
			CategoryUpper,
			[]string{"Chest", "Shoulders", "Triceps"},
			3,
			PeriodizationStrength,
			false,
			make(map[int]bool), // Empty week-used set.
		)

		if len(sets) < 2 {
			t.Fatalf("want at least 2 exercises, got %d", len(sets))
		}

		// Collect all primary muscle groups across selected exercises.
		seenPrimary := make(map[string]bool)
		for _, es := range sets {
			ex := findExercise(exercises, es.Exercise.ID)
			for _, mg := range ex.PrimaryMuscleGroups {
				if seenPrimary[mg] {
					t.Errorf("primary muscle group %q appears in multiple exercises in the same session", mg)
				}
				seenPrimary[mg] = true
			}
		}
	})

	t.Run("skip priority muscle group when no non-conflicting exercise available", func(t *testing.T) {
		t.Parallel()
		// Exercise pool: all Chest exercises have overlapping primary muscles.
		exercises := []Exercise{
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest", "Triceps"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest", "Shoulders"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
		}

		p := prefs(time.Tuesday) // 3 exercises
		wp := NewPlanner(p, exercises, nil)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		// Request 3 exercises, but only 1 non-overlapping is available.
		// Expected: graceful degradation — select 1 exercise covering Chest.
		sets := wp.selectExercisesForDayWithPeriodization(
			CategoryUpper,
			[]string{"Chest", "Shoulders", "Triceps"}, // Three priorities, but can't all be satisfied.
			3,
			PeriodizationStrength,
			false,
			make(map[int]bool),
		)

		if len(sets) == 0 {
			t.Fatalf("want at least 1 exercise, got 0")
		}

		// Should select 1 exercise (Chest), then can't add more without overlap.
		if len(sets) > 1 {
			// Check that no primary muscle groups repeat.
			seenPrimary := make(map[string]bool)
			for _, es := range sets {
				ex := findExercise(exercises, es.Exercise.ID)
				for _, mg := range ex.PrimaryMuscleGroups {
					if seenPrimary[mg] {
						t.Errorf(
							"primary muscle group %q appears twice; expected graceful degradation to 1 exercise",
							mg,
						)
					}
					seenPrimary[mg] = true
				}
			}
		}
	})
}

func TestSelectExercisesForDayWeekDeduplication(t *testing.T) {
	t.Parallel()

	t.Run("exercise used earlier in week is skipped", func(t *testing.T) {
		t.Parallel()
		exercises := []Exercise{
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Triceps"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
		}

		p := prefs(time.Tuesday)
		wp := NewPlanner(p, exercises, nil)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		// Simulate that exercise 1 was already used earlier in the week.
		weekUsedExercises := map[int]bool{1: true}

		// Request exercises with Chest priority, but exercise 1 (Chest) is already used.
		// Expected: select exercise 2 (Shoulders) or 3 (Triceps) instead.
		sets := wp.selectExercisesForDayWithPeriodization(
			CategoryUpper,
			[]string{"Chest"},
			1,
			PeriodizationStrength,
			false,
			weekUsedExercises,
		)

		if len(sets) == 0 {
			t.Fatalf("want 1 exercise, got 0")
		}

		selectedID := sets[0].Exercise.ID
		if selectedID == 1 {
			t.Errorf("exercise 1 was already used this week; expected a different exercise, got %d", selectedID)
		}
	})

	t.Run("plan() does not repeat exercises across days", func(t *testing.T) {
		t.Parallel()
		exercises := minimalExercises() // Use existing test fixture.
		targets := minimalTargets()

		monday := monday2026Date()
		p := prefs(time.Monday, time.Tuesday, time.Thursday)
		wp := NewPlanner(p, exercises, targets)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}
		var sessions []Session
		for i := range plan.Sessions {
			if len(plan.Sessions[i].ExerciseSets) > 0 {
				sessions = append(sessions, plan.Sessions[i])
			}
		}

		// Collect all exercise IDs across all sessions.
		usedExercises := make(map[int]bool)
		for _, session := range sessions {
			for _, es := range session.ExerciseSets {
				if usedExercises[es.Exercise.ID] {
					t.Errorf("exercise %d appears in multiple sessions across the week", es.Exercise.ID)
				}
				usedExercises[es.Exercise.ID] = true
			}
		}

		// Verify that we have more than one session (to make the test meaningful).
		if len(sessions) < 2 {
			t.Logf("note: only %d session(s) planned, test less meaningful", len(sessions))
		}
	})
}

func TestSelectExercisesForDayGracefulDegradation(t *testing.T) {
	t.Parallel()

	t.Run("returns fewer exercises if constraints can't be fully satisfied", func(t *testing.T) {
		t.Parallel()
		// Exercise pool: only 2 non-overlapping exercises available.
		exercises := []Exercise{
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
			{ //nolint:exhaustruct // Test exercises omit unused display fields.
				ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
				DefaultStartingSeconds: nil, RepMin: new(5), RepMax: new(10)},
		}

		p := prefs(time.Tuesday) // Requests 3 exercises.
		wp := NewPlanner(p, exercises, nil)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		// Request 3 exercises, but only 2 non-overlapping available.
		// Expected: plan succeeds with 2 exercises, no error.
		sets := wp.selectExercisesForDayWithPeriodization(
			CategoryUpper,
			[]string{"Chest", "Shoulders", "Triceps"},
			3,
			PeriodizationStrength,
			false,
			make(map[int]bool),
		)

		if len(sets) != 2 {
			t.Errorf("want 2 exercises (graceful degradation), got %d", len(sets))
		}

		// Verify the 2 selected have no overlapping primary muscles.
		seenPrimary := make(map[string]bool)
		for _, es := range sets {
			ex := findExercise(exercises, es.Exercise.ID)
			for _, mg := range ex.PrimaryMuscleGroups {
				if seenPrimary[mg] {
					t.Errorf("primary muscle group %q appears twice", mg)
				}
				seenPrimary[mg] = true
			}
		}
	})
}

func TestSelectExercisesForDay_TimeBasedTarget(t *testing.T) {
	t.Parallel()

	starting := 30
	plank := Exercise{ //nolint:exhaustruct // Test exercise omits unused display fields.
		ID:                     21,
		Category:               CategoryUpper,
		ExerciseType:           ExerciseTypeTime,
		PrimaryMuscleGroups:    []string{"Abs"},
		SecondaryMuscleGroups:  nil,
		DefaultStartingSeconds: &starting,
		RepMin:                 nil,
		RepMax:                 nil,
	}

	wp := &Planner{
		Prefs: Preferences{ //nolint:exhaustruct // RestNotificationsEnabled irrelevant to planner tests.
			MondayMinutes:    60,
			TuesdayMinutes:   0,
			WednesdayMinutes: 0,
			ThursdayMinutes:  0,
			FridayMinutes:    0,
			SaturdayMinutes:  0,
			SundayMinutes:    0,
		},
		Exercises: []Exercise{plank},
		Targets:   []MuscleGroupTarget{{MuscleGroupName: "Abs", WeeklySetTarget: 8}},
		rng:       nil,
	}

	sets := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper,
		[]string{"Abs"},
		1,
		PeriodizationStrength,
		false,
		map[int]bool{},
	)

	if len(sets) != 1 {
		t.Fatalf("got %d ExerciseSets, want 1", len(sets))
	}
	if sets[0].Exercise.ID != plank.ID {
		t.Fatalf("got exerciseID %d, want %d", sets[0].Exercise.ID, plank.ID)
	}
	if len(sets[0].Sets) != defaultTimedSets {
		t.Fatalf("got %d sets, want %d", len(sets[0].Sets), defaultTimedSets)
	}
	for i, s := range sets[0].Sets {
		if s.TargetValue != 30 {
			t.Errorf("set %d: TargetValue = %d, want 30 (DefaultStartingSeconds)", i, s.TargetValue)
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

func TestPlanner_DeloadWeekForcesHypertrophyAndHalvesSets(t *testing.T) {
	t.Parallel()

	// Anchor on the same Monday we'll plan: week 0 of length 4 would NOT be a
	// deload (we want length-1 → 3, so plan on a date that is anchor + 21 days).
	anchor := time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC) // Monday
	planMonday := anchor.AddDate(0, 0, 21)                         // week 3 of 4 → deload

	prefs := Preferences{ //nolint:exhaustruct // RestNotificationsEnabled and other UI prefs irrelevant.
		MondayMinutes:   60,
		TuesdayMinutes:  60,
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
		{MuscleGroupName: "chest", WeeklySetTarget: 6},
		{MuscleGroupName: "quads", WeeklySetTarget: 6},
	}
	wp := NewPlanner(prefs, exercises, targets)
	plan, err := wp.Plan(planMonday)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var sessions []Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].ExerciseSets) > 0 {
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
		for _, es := range s.ExerciseSets {
			// Normal mid-rep band has 3 sets. Deload halves to 2.
			if len(es.Sets) != 2 {
				t.Errorf("session %s, exercise %s: %d sets, want 2 (deload halves)",
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
		MondayMinutes:   60,
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
	targets := []MuscleGroupTarget{{MuscleGroupName: "chest", WeeklySetTarget: 3}}
	wp := NewPlanner(p, exercises, targets)
	plan, err := wp.Plan(planMonday)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var sessions []Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].ExerciseSets) > 0 {
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
		wp.rng = rand.New(rand.NewPCG(1, 0))

		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		var sessions []Session
		for i := range plan.Sessions {
			if len(plan.Sessions[i].ExerciseSets) > 0 {
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
		wp.rng = rand.New(rand.NewPCG(1, 0))

		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		var sessions []Session
		for i := range plan.Sessions {
			if len(plan.Sessions[i].ExerciseSets) > 0 {
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
		wp.rng = rand.New(rand.NewPCG(2, 0))

		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		var sessions []Session
		for i := range plan.Sessions {
			if len(plan.Sessions[i].ExerciseSets) > 0 {
				sessions = append(sessions, plan.Sessions[i])
			}
		}
		for _, sess := range sessions {
			want := exercisesMedium
			if sess.PeriodizationType == PeriodizationHypertrophy && !sess.IsDeload {
				want = exercisesMediumHypertrophy
			}
			if len(sess.ExerciseSets) != want {
				t.Errorf("60-min %s session: want %d exercises, got %d",
					sess.PeriodizationType, want, len(sess.ExerciseSets))
			}
		}
	})

	t.Run("consecutive sessions alternate periodization", func(t *testing.T) {
		t.Parallel()
		p := prefs(time.Monday, time.Tuesday)
		wp := NewPlanner(p, exercises, targets)
		wp.rng = rand.New(rand.NewPCG(3, 0))

		plan, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		var sessions []Session
		for i := range plan.Sessions {
			if len(plan.Sessions[i].ExerciseSets) > 0 {
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
		MondayMinutes:    minutesLong,   // 90
		TuesdayMinutes:   minutesMedium, // 60
		WednesdayMinutes: 45,
		ThursdayMinutes:  0,
		FridayMinutes:    0,
		SaturdayMinutes:  0,
		SundayMinutes:    0,
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
		MondayMinutes:    minutesMedium,
		TuesdayMinutes:   0,
		WednesdayMinutes: 0,
		ThursdayMinutes:  minutesMedium,
		FridayMinutes:    0,
		SaturdayMinutes:  0,
		SundayMinutes:    0,
	}
	wp := NewPlanner(p, minimalExercises(), minimalTargets())
	wp.rng = rand.New(rand.NewPCG(7, 0))

	plan, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	var sessions []Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].ExerciseSets) > 0 {
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
		if got := len(sess.ExerciseSets); got != wantCount[i] {
			t.Errorf("session %d (%s) exercise count: want %d, got %d",
				i, sess.PeriodizationType, wantCount[i], got)
		}
	}
}
