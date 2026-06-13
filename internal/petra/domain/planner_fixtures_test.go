package domain_test

import (
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

// Shared fixtures for the planner's behavioural suite. These construct
// Planner inputs entirely from the package's exported surface, so the suite
// in planner_test.go exercises Plan/PlanDay the way real callers do.

// monday2026Date returns 2026-01-05, a known Monday.
func monday2026Date() time.Time {
	return time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
}

// date offsets base by offsetDays — used to name weekdays relative to a Monday.
func date(base time.Time, offsetDays int) time.Time {
	return base.AddDate(0, 0, offsetDays)
}

// prefs schedules the given weekdays at 60 minutes (the medium duration that
// yields exercisesMedium / exercisesMediumHypertrophy).
func prefs(days ...time.Weekday) domain.Preferences {
	p := domain.Preferences{} //nolint:exhaustruct // Only Minutes matters to planner tests.
	for _, d := range days {
		p.Minutes[d] = 60
	}
	return p
}

// prefs90 schedules the given weekdays at 90 minutes (the long duration that
// yields exercisesLong / exercisesLongHypertrophy).
func prefs90(days ...time.Weekday) domain.Preferences {
	p := domain.Preferences{} //nolint:exhaustruct // Only Minutes matters to planner tests.
	for _, d := range days {
		p.Minutes[d] = 90
	}
	return p
}

// minimalExercises is a compact pool spanning Lower, Upper, and FullBody with
// distinct primary muscles, enough for diversity and balance behaviour.
func minimalExercises() []domain.Exercise {
	return []domain.Exercise{
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 1, Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads", "Glutes"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 2, Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 3, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest", "Triceps", "Shoulders"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 4, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Lats", "Upper Back"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 5, Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Biceps"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 6, Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings", "Glutes"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 7, Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 8, Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10)},
	}
}

func minimalTargets() []domain.MuscleGroupTarget {
	return []domain.MuscleGroupTarget{
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

// planDayExercises is a pool with Upper, Lower, and FullBody coverage across
// distinct primary muscles so PlanDay's non-conflict selection has room. The
// FullBody Plank carries the lowest ID, so with empty targets the lowest-id
// tie-break picks it first on FullBody days, making WorkoutType() report
// CategoryFullBody reliably.
func planDayExercises() []domain.Exercise {
	return []domain.Exercise{
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 1, Name: "Plank", Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Core"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 2, Name: "Bench Press", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: []string{"Triceps"},
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 3, Name: "Row", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Upper Back"}, SecondaryMuscleGroups: []string{"Biceps"},
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 4, Name: "Overhead Press", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: []string{"Triceps"},
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 5, Name: "Squat", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads"}, SecondaryMuscleGroups: []string{"Glutes"},
			RepMin: new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercises omit unused display fields.
			ID: 6, Name: "Deadlift", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings"}, SecondaryMuscleGroups: []string{"Glutes"},
			RepMin: new(5), RepMax: new(10)},
	}
}

// seedExercises mirrors the 39 exercises in internal/repository/fixtures.sql
// verbatim (IDs, categories, types, rep ranges, and primary/secondary muscle
// groups all match). It keeps the balance regression test pure-domain while
// exercising the algorithm against the actual seed users start with. Update
// both this helper and fixtures.sql together when seed exercises change.
func seedExercises() []domain.Exercise {
	return []domain.Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Name: "Deadlift", Category: domain.CategoryFullBody, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Hamstrings", "Lower Back"},
			SecondaryMuscleGroups: []string{"Forearms", "Lats", "Quads", "Traps", "Upper Back"},
			RepMin:                new(3), RepMax: new(6)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Name: "Bench Press", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
			SecondaryMuscleGroups: []string{"Abs", "Forearms", "Shoulders"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 3, Name: "Tricep Pushdown", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Triceps"},
			SecondaryMuscleGroups: []string{"Shoulders"},
			RepMin:                new(8), RepMax: new(12)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    4,
			Name:                  "Dumbbell Biceps Curl",
			Category:              domain.CategoryUpper,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Biceps"},
			SecondaryMuscleGroups: []string{"Forearms"},
			RepMin:                new(8),
			RepMax:                new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 5, Name: "Lateral Raise", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{"Traps", "Upper Back"},
			RepMin:                new(10), RepMax: new(20)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    6,
			Name:                  "Dumbbell Shoulder Press",
			Category:              domain.CategoryUpper,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{"Triceps", "Upper Back"},
			RepMin:                new(5),
			RepMax:                new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    7,
			Name:                  "Dumbbell Bench Press",
			Category:              domain.CategoryUpper,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest"},
			SecondaryMuscleGroups: []string{"Shoulders", "Triceps"},
			RepMin:                new(5),
			RepMax:                new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 8, Name: "Cable Fly", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest"},
			SecondaryMuscleGroups: []string{"Shoulders", "Triceps"},
			RepMin:                new(8), RepMax: new(12)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 9, Name: "Pulldown", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Shoulders"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    10,
			Name:                  "Pulldown, Reverse Grip",
			Category:              domain.CategoryUpper,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Biceps", "Lats"},
			SecondaryMuscleGroups: []string{"Forearms", "Upper Back"},
			RepMin:                new(5),
			RepMax:                new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 11, Name: "Seated Cable Row", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Lower Back"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    12,
			Name:                  "One-Arm Dumbbell Row",
			Category:              domain.CategoryUpper,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Forearms"},
			RepMin:                new(5),
			RepMax:                new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    13,
			Name:                  "Abdominal Machine Crunch",
			Category:              domain.CategoryUpper,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Abs"},
			SecondaryMuscleGroups: []string{"Obliques"},
			RepMin:                new(8),
			RepMax:                new(15),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 14, Name: "Leg Press", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Calves", "Hamstrings"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 15, Name: "Leg Extension", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Quads"},
			SecondaryMuscleGroups: []string{"Hip Flexors"},
			RepMin:                new(8), RepMax: new(12)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 16, Name: "Leg Curl", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Hamstrings"},
			SecondaryMuscleGroups: []string{"Calves"},
			RepMin:                new(8), RepMax: new(12)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 17, Name: "Calf Raise", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Calves"},
			SecondaryMuscleGroups: []string{"Quads"},
			RepMin:                new(10), RepMax: new(20)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 18, Name: "Back Extension", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lower Back"},
			SecondaryMuscleGroups: []string{"Glutes", "Hamstrings"},
			RepMin:                new(8), RepMax: new(20)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 19, Name: "Push-Up", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
			SecondaryMuscleGroups: []string{"Abs", "Forearms", "Shoulders", "Upper Back"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    20,
			Name:                  "Ab Wheel Rollout",
			Category:              domain.CategoryUpper,
			ExerciseType:          domain.ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Abs", "Obliques"},
			SecondaryMuscleGroups: []string{"Calves", "Glutes", "Hamstrings", "Lats", "Quads", "Shoulders"},
			RepMin:                new(8),
			RepMax:                new(15),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 21, Name: "Plank", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Abs"},
			SecondaryMuscleGroups: []string{"Glutes", "Hip Flexors", "Lower Back", "Obliques", "Quads", "Shoulders"},
			RepMin:                new(8), RepMax: new(15)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    22,
			Name:                  "Incline Dumbbell Bench Press",
			Category:              domain.CategoryUpper,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest"},
			SecondaryMuscleGroups: []string{"Shoulders", "Triceps", "Upper Back"},
			RepMin:                new(5),
			RepMax:                new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    23,
			Name:                  "Romanian Deadlift",
			Category:              domain.CategoryLower,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Hamstrings"},
			SecondaryMuscleGroups: []string{"Lower Back"},
			RepMin:                new(8),
			RepMax:                new(20),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 24, Name: "Assisted Pull-Up", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeAssisted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Forearms"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 25, Name: "Hip Abductor", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes"},
			SecondaryMuscleGroups: nil,
			RepMin:                new(8), RepMax: new(12)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 26, Name: "Hip Adductor", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Adductors"},
			SecondaryMuscleGroups: []string{"Glutes"},
			RepMin:                new(8), RepMax: new(12)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 27, Name: "Rotary Torso", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Obliques"},
			SecondaryMuscleGroups: []string{"Abs"},
			RepMin:                new(8), RepMax: new(15)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    28,
			Name:                  "Seated Calf Raise",
			Category:              domain.CategoryLower,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Calves"},
			SecondaryMuscleGroups: nil,
			RepMin:                new(10),
			RepMax:                new(20),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 29, Name: "Squat", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Hamstrings", "Lower Back"},
			RepMin:                new(3), RepMax: new(6)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 30, Name: "Pec Fly", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest"},
			SecondaryMuscleGroups: []string{"Shoulders"},
			RepMin:                new(8), RepMax: new(12)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    31,
			Name:                  "Smith Machine Squat",
			Category:              domain.CategoryLower,
			ExerciseType:          domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Abs", "Hamstrings"},
			RepMin:                new(3),
			RepMax:                new(6),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 32, Name: "Overhead Press", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders", "Triceps"},
			SecondaryMuscleGroups: []string{"Abs", "Upper Back"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 33, Name: "Barbell Row", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Lower Back"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 34, Name: "Face Pull", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders", "Upper Back"},
			SecondaryMuscleGroups: []string{"Traps", "Triceps"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 35, Name: "Hip Thrust", Category: domain.CategoryLower, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes"},
			SecondaryMuscleGroups: []string{"Hamstrings", "Quads"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    36,
			Name:                  "Bulgarian Split Squat",
			Category:              domain.CategoryLower,
			ExerciseType:          domain.ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Abs", "Hamstrings"},
			RepMin:                new(5),
			RepMax:                new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 37, Name: "Hammer Curl", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Biceps", "Forearms"},
			SecondaryMuscleGroups: []string{"Shoulders", "Triceps"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 38, Name: "Skull Crusher", Category: domain.CategoryUpper, ExerciseType: domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Triceps"},
			SecondaryMuscleGroups: []string{"Forearms", "Shoulders"},
			RepMin:                new(5), RepMax: new(10)},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    39,
			Name:                  "Hanging Leg Raise",
			Category:              domain.CategoryUpper,
			ExerciseType:          domain.ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Abs", "Hip Flexors"},
			SecondaryMuscleGroups: []string{"Forearms", "Obliques"},
			RepMin:                new(5),
			RepMax:                new(10),
		},
	}
}

func seedTargets() []domain.MuscleGroupTarget {
	return []domain.MuscleGroupTarget{
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

// planSessions returns the populated (workout-day) sessions of a week plan,
// in day order.
func planSessions(plan domain.WeekPlan) []domain.Session {
	var ss []domain.Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].Slots) > 0 {
			ss = append(ss, plan.Sessions[i])
		}
	}
	return ss
}

// slotIDs returns the exercise IDs of a session's slots, in order.
func slotIDs(s domain.Session) []int {
	ids := make([]int, len(s.Slots))
	for i, slot := range s.Slots {
		ids[i] = slot.Exercise.ID
	}
	return ids
}
