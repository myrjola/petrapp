package domain_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

//nolint:exhaustruct // Test exercises omit unused fields (ID, Name, ExerciseType, content).
func TestSwapSimilarityScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current domain.Exercise
		other   domain.Exercise
		want    int
	}{
		{
			name: "identical primary, secondary, and category",
			current: domain.Exercise{
				Category:              domain.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: domain.Exercise{
				Category:              domain.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			want: 12, // 4*2 + 1 + 3.
		},
		{
			name: "one primary muscle in common, same category",
			current: domain.Exercise{
				Category:            domain.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: domain.Exercise{
				Category:            domain.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			want: 7, // 4 + 3.
		},
		{
			name: "current primary matches candidate secondary, same category",
			current: domain.Exercise{
				Category:            domain.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: domain.Exercise{
				Category:              domain.CategoryUpper,
				SecondaryMuscleGroups: []string{"Chest"},
			},
			want: 5, // 2 + 3.
		},
		{
			name: "current secondary matches candidate primary, same category",
			current: domain.Exercise{
				Category:              domain.CategoryUpper,
				SecondaryMuscleGroups: []string{"Chest"},
			},
			other: domain.Exercise{
				Category:            domain.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			want: 5, // 2 + 3.
		},
		{
			name: "secondary↔secondary match, same category",
			current: domain.Exercise{
				Category:              domain.CategoryUpper,
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: domain.Exercise{
				Category:              domain.CategoryUpper,
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			want: 4, // 1 + 3.
		},
		{
			name: "disjoint muscles, same category",
			current: domain.Exercise{
				Category:            domain.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: domain.Exercise{
				Category:            domain.CategoryUpper,
				PrimaryMuscleGroups: []string{"Biceps"},
			},
			want: 3,
		},
		{
			name: "disjoint muscles, different category",
			current: domain.Exercise{
				Category:            domain.CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: domain.Exercise{
				Category:            domain.CategoryLower,
				PrimaryMuscleGroups: []string{"Quads"},
			},
			want: 0,
		},
		{
			name:    "empty slices, different category",
			current: domain.Exercise{Category: domain.CategoryUpper},
			other:   domain.Exercise{Category: domain.CategoryLower},
			want:    0,
		},
		{
			name:    "empty slices, same category",
			current: domain.Exercise{Category: domain.CategoryUpper},
			other:   domain.Exercise{Category: domain.CategoryUpper},
			want:    3,
		},
		{
			name: "spec example: Bench Press vs Incline Press",
			current: domain.Exercise{
				Category:              domain.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: domain.Exercise{
				Category:              domain.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Shoulders"},
				SecondaryMuscleGroups: []string{"Triceps"},
			},
			want: 11, // 4 + 2 + 2 + 3.
		},
		{
			name: "spec example: Bench Press vs Push-Ups",
			current: domain.Exercise{
				Category:              domain.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: domain.Exercise{
				Category:              domain.CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest"},
				SecondaryMuscleGroups: []string{"Triceps", "Shoulders"},
			},
			want: 10, // 4 + 2 + 1 + 3.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.SwapSimilarityScore(tt.current, tt.other); got != tt.want {
				t.Errorf("SwapSimilarityScore(current, other) = %d, want %d", got, tt.want)
			}
		})
	}
}

//nolint:exhaustruct // Test exercises omit unused fields (ID, Name, ExerciseType, content).
func TestSwapSimilarityScore_isSymmetric(t *testing.T) {
	t.Parallel()

	a := domain.Exercise{
		Category:              domain.CategoryUpper,
		PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
		SecondaryMuscleGroups: []string{"Shoulders"},
	}
	b := domain.Exercise{
		Category:              domain.CategoryUpper,
		PrimaryMuscleGroups:   []string{"Chest", "Shoulders"},
		SecondaryMuscleGroups: []string{"Triceps"},
	}

	ab := domain.SwapSimilarityScore(a, b)
	ba := domain.SwapSimilarityScore(b, a)
	if ab != ba {
		t.Errorf("score asymmetric: SwapSimilarityScore(a,b) = %d, SwapSimilarityScore(b,a) = %d", ab, ba)
	}
}
