package workout_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/workout"
)

//nolint:exhaustruct // Test exercises omit unused fields (ID, Name, ExerciseType, DescriptionMarkdown).
func TestSwapSimilarityScore(t *testing.T) {
	tests := []struct {
		name    string
		current workout.Exercise
		other   workout.Exercise
		want    int
	}{
		{
			name: "identical primary, secondary, and category",
			current: workout.Exercise{
				Category:              workout.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: workout.Exercise{
				Category:              workout.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			want: 12, // 4*2 + 1 + 3.
		},
		{
			name: "one primary muscle in common, same category",
			current: workout.Exercise{
				Category:            workout.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: workout.Exercise{
				Category:            workout.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			want: 7, // 4 + 3.
		},
		{
			name: "current primary matches candidate secondary, same category",
			current: workout.Exercise{
				Category:            workout.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: workout.Exercise{
				Category:              workout.CategoryUpper,
				SecondaryMuscleGroups: []string{"Chest"},
			},
			want: 5, // 2 + 3.
		},
		{
			name: "current secondary matches candidate primary, same category",
			current: workout.Exercise{
				Category:              workout.CategoryUpper,
				SecondaryMuscleGroups: []string{"Chest"},
			},
			other: workout.Exercise{
				Category:            workout.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			want: 5, // 2 + 3.
		},
		{
			name: "secondary↔secondary match, same category",
			current: workout.Exercise{
				Category:              workout.CategoryUpper,
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: workout.Exercise{
				Category:              workout.CategoryUpper,
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			want: 4, // 1 + 3.
		},
		{
			name: "disjoint muscles, same category",
			current: workout.Exercise{
				Category:            workout.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: workout.Exercise{
				Category:            workout.CategoryUpper,
				PrimaryMuscleGroups: []string{"Biceps"},
			},
			want: 3,
		},
		{
			name: "disjoint muscles, different category",
			current: workout.Exercise{
				Category:            workout.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: workout.Exercise{
				Category:            workout.CategoryLower,
				PrimaryMuscleGroups: []string{"Quads"},
			},
			want: 0,
		},
		{
			name:    "empty slices, different category",
			current: workout.Exercise{Category: workout.CategoryUpper},
			other:   workout.Exercise{Category: workout.CategoryLower},
			want:    0,
		},
		{
			name:    "empty slices, same category",
			current: workout.Exercise{Category: workout.CategoryUpper},
			other:   workout.Exercise{Category: workout.CategoryUpper},
			want:    3,
		},
		{
			name: "spec example: Bench Press vs Incline Press",
			current: workout.Exercise{
				Category:              workout.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: workout.Exercise{
				Category:              workout.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Shoulders"},
				SecondaryMuscleGroups: []string{"Triceps"},
			},
			want: 11, // 4 + 2 + 2 + 3.
		},
		{
			name: "spec example: Bench Press vs Push-Ups",
			current: workout.Exercise{
				Category:              workout.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: workout.Exercise{
				Category:              workout.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest"},
				SecondaryMuscleGroups: []string{"Triceps", "Shoulders"},
			},
			want: 10, // 4 + 2 + 1 + 3.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workout.SwapSimilarityScore(tt.current, tt.other); got != tt.want {
				t.Errorf("SwapSimilarityScore(current, other) = %d, want %d", got, tt.want)
			}
		})
	}
}

//nolint:exhaustruct // Test exercises omit unused fields (ID, Name, ExerciseType, DescriptionMarkdown).
func TestSwapSimilarityScore_isSymmetric(t *testing.T) {
	a := workout.Exercise{
		Category:              workout.CategoryUpper,
		PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
		SecondaryMuscleGroups: []string{"Shoulders"},
	}
	b := workout.Exercise{
		Category:              workout.CategoryUpper,
		PrimaryMuscleGroups:   []string{"Chest", "Shoulders"},
		SecondaryMuscleGroups: []string{"Triceps"},
	}

	ab := workout.SwapSimilarityScore(a, b)
	ba := workout.SwapSimilarityScore(b, a)
	if ab != ba {
		t.Errorf("score asymmetric: SwapSimilarityScore(a,b) = %d, SwapSimilarityScore(b,a) = %d", ab, ba)
	}
}
