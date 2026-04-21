package weekplanner

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"
)

// monday2026 is 2026-01-05, a known Monday.
var monday2026 = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

func date(base time.Time, offsetDays int) time.Time {
	return base.AddDate(0, 0, offsetDays)
}

func prefs(days ...time.Weekday) Preferences {
	p := Preferences{}
	for _, d := range days {
		switch d {
		case time.Monday:
			p.MondayMinutes = 60
		case time.Tuesday:
			p.TuesdayMinutes = 60
		case time.Wednesday:
			p.WednesdayMinutes = 60
		case time.Thursday:
			p.ThursdayMinutes = 60
		case time.Friday:
			p.FridayMinutes = 60
		case time.Saturday:
			p.SaturdayMinutes = 60
		case time.Sunday:
			p.SundayMinutes = 60
		}
	}
	return p
}

func TestDetermineCategory(t *testing.T) {
	tests := []struct {
		name     string
		prefs    Preferences
		date     time.Time
		expected Category
	}{
		{
			name:     "isolated day is full body",
			prefs:    prefs(time.Monday, time.Wednesday, time.Friday),
			date:     monday2026, // Mon: tomorrow=Tue not workout, yesterday=Sun not workout
			expected: CategoryFullBody,
		},
		{
			name:     "first of consecutive days is lower",
			prefs:    prefs(time.Monday, time.Tuesday),
			date:     monday2026, // Mon: tomorrow=Tue is workout
			expected: CategoryLower,
		},
		{
			name:     "second of consecutive days is upper",
			prefs:    prefs(time.Monday, time.Tuesday),
			date:     date(monday2026, 1), // Tue: yesterday=Mon was workout
			expected: CategoryUpper,
		},
		{
			name:     "week wrap: Sunday before Monday is lower",
			prefs:    prefs(time.Sunday, time.Monday, time.Tuesday),
			date:     date(monday2026, 6), // Sun (next week context doesn't matter — prefs wrap)
			expected: CategoryLower,       // Sun: today=workout, tomorrow=Mon=workout
		},
		{
			name:     "week wrap: Monday after Sunday is upper",
			prefs:    prefs(time.Sunday, time.Monday),
			date:     monday2026, // Mon: yesterday=Sun=workout
			expected: CategoryUpper,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wp := NewWeeklyPlanner(tt.prefs, nil, nil)
			got := wp.determineCategory(tt.date)
			if got != tt.expected {
				t.Errorf("determineCategory(%s) = %s, want %s", tt.date.Weekday(), got, tt.expected)
			}
		})
	}
}

func TestFirstSessionPeriodizationType(t *testing.T) {
	// Mon/Wed/Fri at 60 min = 3 exercises each = 9 exercises/week.
	p := prefs(time.Monday, time.Wednesday, time.Friday)
	wp := NewWeeklyPlanner(p, nil, nil)

	// Verify formula: (weeksSinceEpoch * exercisesPerWeek) % 2.
	// For any two Mondays 2 weeks apart the periodization must differ.
	monday1 := monday2026                  // week N
	monday2 := monday2026.AddDate(0, 0, 7) // week N+1

	pt1 := wp.firstSessionPeriodizationType(monday1)
	pt2 := wp.firstSessionPeriodizationType(monday2)

	if pt1 == pt2 {
		t.Errorf("consecutive weeks with odd exercisesPerWeek must alternate: both got %v", pt1)
	}

	// Verify determinism: same date always returns the same value.
	if wp.firstSessionPeriodizationType(monday1) != pt1 {
		t.Error("firstSessionPeriodizationType is not deterministic")
	}
}

func minimalExercises() []Exercise {
	return []Exercise{
		{ID: 1, Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads", "Glutes"}},
		{ID: 2, Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings"}},
		{ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest", "Triceps", "Shoulders"}},
		{ID: 4, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Lats", "Upper Back"}},
		{ID: 5, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Biceps"}},
		{ID: 6, Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings", "Glutes"}},
	}
}

func minimalTargets() []MuscleGroupTarget {
	return []MuscleGroupTarget{
		{Name: "Chest", WeeklySetTarget: 10},
		{Name: "Shoulders", WeeklySetTarget: 10},
		{Name: "Triceps", WeeklySetTarget: 8},
		{Name: "Biceps", WeeklySetTarget: 8},
		{Name: "Upper Back", WeeklySetTarget: 10},
		{Name: "Lats", WeeklySetTarget: 10},
		{Name: "Quads", WeeklySetTarget: 10},
		{Name: "Hamstrings", WeeklySetTarget: 8},
		{Name: "Glutes", WeeklySetTarget: 8},
	}
}

func TestAllocateMuscleGroups(t *testing.T) {
	// Mon(Lower), Tue(Upper), Thu(Full Body) schedule.
	p := prefs(time.Monday, time.Tuesday, time.Thursday)
	wp := NewWeeklyPlanner(p, minimalExercises(), minimalTargets())

	mon := monday2026          // Lower
	tue := date(monday2026, 1) // Upper
	thu := date(monday2026, 3) // Full Body

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
		if !allGroups[target.Name] {
			t.Errorf("muscle group %q not assigned to any day", target.Name)
		}
	}
}

func TestSelectExercisesForDay(t *testing.T) {
	p := prefs(time.Monday, time.Tuesday, time.Thursday)
	wp := NewWeeklyPlanner(p, minimalExercises(), minimalTargets())
	wp.rng = rand.New(rand.NewPCG(42, 0)) // fixed seed for determinism

	t.Run("lower day only selects lower exercises", func(t *testing.T) {
		sets := wp.selectExercisesForDay(CategoryLower, []string{"Quads", "Hamstrings"}, 2)
		if len(sets) != 2 {
			t.Fatalf("want 2 exercise sets, got %d", len(sets))
		}
		for _, es := range sets {
			ex := findExercise(wp.Exercises, es.ExerciseID)
			if ex.Category != CategoryLower {
				t.Errorf("lower day got exercise with category %s", ex.Category)
			}
		}
	})

	t.Run("upper day only selects upper exercises", func(t *testing.T) {
		sets := wp.selectExercisesForDay(CategoryUpper, []string{"Chest", "Lats"}, 2)
		for _, es := range sets {
			ex := findExercise(wp.Exercises, es.ExerciseID)
			if ex.Category != CategoryUpper {
				t.Errorf("upper day got exercise with category %s", ex.Category)
			}
		}
	})

	t.Run("full body day can select any category", func(t *testing.T) {
		sets := wp.selectExercisesForDay(CategoryFullBody, []string{"Hamstrings", "Chest"}, 3)
		categories := make(map[Category]bool)
		for _, es := range sets {
			ex := findExercise(wp.Exercises, es.ExerciseID)
			categories[ex.Category] = true
		}
		// With Hamstrings and Chest as priorities, expect both lower and upper exercises selected.
		if !categories[CategoryLower] || !categories[CategoryUpper] {
			t.Error("full body day should draw from multiple categories when priorities span both")
		}
	})

	t.Run("each exercise set has setsPerExercise sets", func(t *testing.T) {
		sets := wp.selectExercisesForDay(CategoryUpper, []string{"Chest"}, 1)
		if len(sets) != 1 {
			t.Fatalf("want 1 exercise set, got %d", len(sets))
		}
		if len(sets[0].Sets) != setsPerExercise {
			t.Errorf("want %d sets, got %d", setsPerExercise, len(sets[0].Sets))
		}
	})

	t.Run("strength periodization sets correct rep range", func(t *testing.T) {
		sets := wp.selectExercisesForDay(CategoryUpper, nil, 1)
		for _, s := range sets[0].Sets {
			if s.MinReps != minRepsStrength || s.MaxReps != maxRepsStrength {
				t.Errorf("strength set: want min=%d max=%d, got min=%d max=%d",
					minRepsStrength, maxRepsStrength, s.MinReps, s.MaxReps)
			}
		}
	})
}

func findExercise(exercises []Exercise, id int) Exercise {
	for _, ex := range exercises {
		if ex.ID == id {
			return ex
		}
	}
	panic(fmt.Sprintf("exercise %d not found", id))
}

func TestPlan(t *testing.T) {
	exercises := minimalExercises()
	targets := minimalTargets()

	t.Run("returns error for non-Monday start date", func(t *testing.T) {
		p := prefs(time.Monday, time.Wednesday)
		wp := NewWeeklyPlanner(p, exercises, targets)
		_, err := wp.Plan(date(monday2026, 1)) // Tuesday
		if err == nil {
			t.Error("want error for non-Monday start date, got nil")
		}
	})

	t.Run("returns error when no workout days scheduled", func(t *testing.T) {
		wp := NewWeeklyPlanner(Preferences{}, exercises, targets)
		_, err := wp.Plan(monday2026)
		if err == nil {
			t.Error("want error when no workout days scheduled, got nil")
		}
	})

	t.Run("returns one session per scheduled day", func(t *testing.T) {
		p := prefs(time.Monday, time.Wednesday, time.Friday)
		wp := NewWeeklyPlanner(p, exercises, targets)
		wp.rng = rand.New(rand.NewPCG(1, 0))

		sessions, err := wp.Plan(monday2026)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		if len(sessions) != 3 {
			t.Fatalf("want 3 sessions, got %d", len(sessions))
		}
	})

	t.Run("session dates match scheduled weekdays", func(t *testing.T) {
		p := prefs(time.Monday, time.Wednesday, time.Friday)
		wp := NewWeeklyPlanner(p, exercises, targets)
		wp.rng = rand.New(rand.NewPCG(1, 0))

		sessions, err := wp.Plan(monday2026)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		expected := []time.Weekday{time.Monday, time.Wednesday, time.Friday}
		for i, sess := range sessions {
			if sess.Date.Weekday() != expected[i] {
				t.Errorf("session %d: want %s, got %s", i, expected[i], sess.Date.Weekday())
			}
		}
	})

	t.Run("each session has correct exercise count for duration", func(t *testing.T) {
		// 60 min → 3 exercises
		p := prefs(time.Monday, time.Wednesday)
		wp := NewWeeklyPlanner(p, exercises, targets)
		wp.rng = rand.New(rand.NewPCG(2, 0))

		sessions, err := wp.Plan(monday2026)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		for _, sess := range sessions {
			if len(sess.ExerciseSets) != 3 {
				t.Errorf("60-min session: want 3 exercises, got %d", len(sess.ExerciseSets))
			}
		}
	})

	t.Run("consecutive sessions alternate periodization", func(t *testing.T) {
		p := prefs(time.Monday, time.Tuesday)
		wp := NewWeeklyPlanner(p, exercises, targets)
		wp.rng = rand.New(rand.NewPCG(3, 0))

		sessions, err := wp.Plan(monday2026)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		if len(sessions) < 2 {
			t.Fatal("need at least 2 sessions to test alternation")
		}
		if sessions[0].PeriodizationType == sessions[1].PeriodizationType {
			t.Error("consecutive sessions must have different periodization types")
		}
	})
}
