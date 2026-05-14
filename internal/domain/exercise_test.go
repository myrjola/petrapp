package domain_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func Test_Exercise_FormatSetValue(t *testing.T) {
	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		value    int
		want     string
	}{
		{"weighted formats as integer", mkExercise(domain.ExerciseTypeWeighted), 8, "8"},
		{"bodyweight formats as integer", mkExercise(domain.ExerciseTypeBodyweight), 12, "12"},
		{"assisted formats as integer", mkExercise(domain.ExerciseTypeAssisted), 5, "5"},
		{"time_based formats as seconds", mkExercise(domain.ExerciseTypeTime), 30, "30s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.exercise.FormatSetValue(tc.value)
			if got != tc.want {
				t.Errorf("Exercise{%s}.FormatSetValue(%d) = %q, want %q",
					tc.exercise.ExerciseType, tc.value, got, tc.want)
			}
		})
	}
}

func Test_Exercise_HasWeight(t *testing.T) {
	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		want     bool
	}{
		{"weighted has weight", mkExercise(domain.ExerciseTypeWeighted), true},
		{"assisted has weight", mkExercise(domain.ExerciseTypeAssisted), true},
		{"bodyweight has no weight", mkExercise(domain.ExerciseTypeBodyweight), false},
		{"time_based has no weight", mkExercise(domain.ExerciseTypeTime), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.exercise.HasWeight()
			if got != tc.want {
				t.Errorf("Exercise{%s}.HasWeight() = %v, want %v",
					tc.exercise.ExerciseType, got, tc.want)
			}
		})
	}
}

func Test_Category_IsValid(t *testing.T) {
	cases := []struct {
		name string
		c    domain.Category
		want bool
	}{
		{"full_body", domain.CategoryFullBody, true},
		{"upper", domain.CategoryUpper, true},
		{"lower", domain.CategoryLower, true},
		{"empty", domain.Category(""), false},
		{"unknown", domain.Category("garbage"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.IsValid(); got != tc.want {
				t.Errorf("Category(%q).IsValid() = %v, want %v", tc.c, got, tc.want)
			}
		})
	}
}

func Test_ExerciseType_IsValid(t *testing.T) {
	cases := []struct {
		name string
		et   domain.ExerciseType
		want bool
	}{
		{"weighted", domain.ExerciseTypeWeighted, true},
		{"bodyweight", domain.ExerciseTypeBodyweight, true},
		{"assisted", domain.ExerciseTypeAssisted, true},
		{"time_based", domain.ExerciseTypeTime, true},
		{"empty", domain.ExerciseType(""), false},
		{"unknown", domain.ExerciseType("garbage"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.et.IsValid(); got != tc.want {
				t.Errorf("ExerciseType(%q).IsValid() = %v, want %v", tc.et, got, tc.want)
			}
		})
	}
}

func Test_Exercise_EncodeFormWeight(t *testing.T) {
	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		input    float64
		assisted bool
		want     float64
	}{
		{"weighted ignores assisted flag", mkExercise(domain.ExerciseTypeWeighted), 50, true, 50},
		{"weighted unchecked", mkExercise(domain.ExerciseTypeWeighted), 50, false, 50},
		{"assisted with flag negates", mkExercise(domain.ExerciseTypeAssisted), 20, true, -20},
		{"assisted without flag keeps input", mkExercise(domain.ExerciseTypeAssisted), 20, false, 20},
		{"assisted with flag is idempotent on negative input", mkExercise(domain.ExerciseTypeAssisted), -20, true, -20},
		{"bodyweight ignores flag", mkExercise(domain.ExerciseTypeBodyweight), 0, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.exercise.EncodeFormWeight(tc.input, tc.assisted)
			if got != tc.want {
				t.Errorf("Exercise{%s}.EncodeFormWeight(%v, %v) = %v, want %v",
					tc.exercise.ExerciseType, tc.input, tc.assisted, got, tc.want)
			}
		})
	}
}

func Test_Exercise_FormatSetDescription(t *testing.T) {
	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}
	weight := func(v float64) *float64 { return &v }
	completed := func(v int) *int { return &v }
	mkSet := func(w *float64, c *int) domain.Set {
		return domain.Set{
			WeightKg:       w,
			TargetValue:    0,
			CompletedValue: c,
			CompletedAt:    nil,
			Signal:         nil,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		set      domain.Set
		want     string
	}{
		{"weighted with weight and reps", mkExercise(domain.ExerciseTypeWeighted),
			mkSet(weight(10), completed(8)), "8x10.0kg"},
		{"weighted missing weight", mkExercise(domain.ExerciseTypeWeighted),
			mkSet(nil, completed(8)), ""},
		{"weighted missing completed", mkExercise(domain.ExerciseTypeWeighted),
			mkSet(weight(10), nil), ""},
		{"assisted preserves negative weight", mkExercise(domain.ExerciseTypeAssisted),
			mkSet(weight(-5), completed(12)), "12x-5.0kg"},
		{"bodyweight reps", mkExercise(domain.ExerciseTypeBodyweight),
			mkSet(nil, completed(15)), "15 reps"},
		{"bodyweight missing completed", mkExercise(domain.ExerciseTypeBodyweight),
			mkSet(nil, nil), ""},
		{"time_based seconds", mkExercise(domain.ExerciseTypeTime),
			mkSet(nil, completed(30)), "30s"},
		{"time_based missing completed", mkExercise(domain.ExerciseTypeTime),
			mkSet(nil, nil), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.exercise.FormatSetDescription(tc.set)
			if got != tc.want {
				t.Errorf("Exercise{%s}.FormatSetDescription(...) = %q, want %q",
					tc.exercise.ExerciseType, got, tc.want)
			}
		})
	}
}

func Test_Category_Label(t *testing.T) {
	cases := []struct {
		name     string
		category domain.Category
		want     string
	}{
		{"upper", domain.CategoryUpper, "Upper Body"},
		{"lower", domain.CategoryLower, "Lower Body"},
		{"full body", domain.CategoryFullBody, "Full Body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.category.Label(); got != tc.want {
				t.Errorf("Label() = %q, want %q", got, tc.want)
			}
		})
	}
}

func Test_SetTarget_AbsWeightKg(t *testing.T) {
	cases := []struct {
		name string
		t    domain.SetTarget
		want float64
	}{
		{"positive weight", domain.SetTarget{WeightKg: 50, TargetReps: 0}, 50},
		{"negative weight (assisted convention)", domain.SetTarget{WeightKg: -10, TargetReps: 0}, 10},
		{"zero weight", domain.SetTarget{WeightKg: 0, TargetReps: 0}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.t.AbsWeightKg(); got != tc.want {
				t.Errorf("SetTarget{%v}.AbsWeightKg() = %v, want %v", tc.t.WeightKg, got, tc.want)
			}
		})
	}
}

func Test_Exercise_SetValueUnit(t *testing.T) {
	mkExercise := func(typ domain.ExerciseType) domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise domain.Exercise
		want     string
	}{
		{"weighted is reps", mkExercise(domain.ExerciseTypeWeighted), "reps"},
		{"bodyweight is reps", mkExercise(domain.ExerciseTypeBodyweight), "reps"},
		{"assisted is reps", mkExercise(domain.ExerciseTypeAssisted), "reps"},
		{"time_based is seconds", mkExercise(domain.ExerciseTypeTime), "seconds"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.exercise.SetValueUnit()
			if got != tc.want {
				t.Errorf("Exercise{%s}.SetValueUnit() = %q, want %q",
					tc.exercise.ExerciseType, got, tc.want)
			}
		})
	}
}
