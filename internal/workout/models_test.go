package workout_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/workout"
)

func Test_Exercise_FormatSetValue(t *testing.T) {
	mkExercise := func(typ workout.ExerciseType) workout.Exercise {
		return workout.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise workout.Exercise
		value    int
		want     string
	}{
		{"weighted formats as integer", mkExercise(workout.ExerciseTypeWeighted), 8, "8"},
		{"bodyweight formats as integer", mkExercise(workout.ExerciseTypeBodyweight), 12, "12"},
		{"assisted formats as integer", mkExercise(workout.ExerciseTypeAssisted), 5, "5"},
		{"time_based formats as seconds", mkExercise(workout.ExerciseTypeTime), 30, "30s"},
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
	mkExercise := func(typ workout.ExerciseType) workout.Exercise {
		return workout.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise workout.Exercise
		want     string
	}{
		{"weighted is reps", mkExercise(workout.ExerciseTypeWeighted), "reps"},
		{"bodyweight is reps", mkExercise(workout.ExerciseTypeBodyweight), "reps"},
		{"assisted is reps", mkExercise(workout.ExerciseTypeAssisted), "reps"},
		{"time_based is seconds", mkExercise(workout.ExerciseTypeTime), "seconds"},
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
