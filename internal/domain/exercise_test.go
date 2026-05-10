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
