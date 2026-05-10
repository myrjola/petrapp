package domain_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func Test_BuildPlannedSets(t *testing.T) {
	intPtr := func(i int) *int { return &i }

	cases := []struct {
		name          string
		exercise      domain.Exercise
		periodization domain.PeriodizationType
		wantTargetVal int
		wantSetCount  int
	}{
		{
			name: "weighted Strength: low end of window, 4 sets, nil weight (allocated by buildSetsForAdd)",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       intPtr(5),
				RepMax:       intPtr(10),
			},
			periodization: domain.PeriodizationStrength,
			wantTargetVal: 5,
			wantSetCount:  4, // reps <= 5 → 4 sets
		},
		{
			name: "weighted Hypertrophy: high end, 3 sets, nil weight (allocated by buildSetsForAdd)",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       intPtr(5),
				RepMax:       intPtr(10),
			},
			periodization: domain.PeriodizationHypertrophy,
			wantTargetVal: 10,
			wantSetCount:  3, // 6-10 → 3 sets
		},
		{
			name: "weighted Hypertrophy: high-rep window, 3 sets, nil weight (allocated by buildSetsForAdd)",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       intPtr(8),
				RepMax:       intPtr(12),
			},
			periodization: domain.PeriodizationHypertrophy,
			wantTargetVal: 12,
			wantSetCount:  3, // >= 11 → 3 sets
		},
		{
			name: "assisted exercise: nil weight pointer (allocated by buildSetsForAdd)",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeAssisted,
				RepMin:       intPtr(5),
				RepMax:       intPtr(10),
			},
			periodization: domain.PeriodizationStrength,
			wantTargetVal: 5,
			wantSetCount:  4,
		},
		{
			name: "bodyweight exercise: nil weight",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeBodyweight,
				RepMin:       intPtr(8),
				RepMax:       intPtr(12),
			},
			periodization: domain.PeriodizationStrength,
			wantTargetVal: 8,
			wantSetCount:  3, // 6-10 → 3 sets
		},
		{
			name: "time_based exercise: nil weight, 3 sets",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType:           domain.ExerciseTypeTime,
				DefaultStartingSeconds: intPtr(45),
			},
			periodization: domain.PeriodizationStrength,
			wantTargetVal: 45,
			wantSetCount:  3, // fixed set count for time-based exercises
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.BuildPlannedSets(tc.exercise, tc.periodization)
			if len(got) != tc.wantSetCount {
				t.Fatalf("len = %d, want %d", len(got), tc.wantSetCount)
			}
			for i, s := range got {
				if s.TargetValue != tc.wantTargetVal {
					t.Errorf("set[%d].TargetValue = %d, want %d", i, s.TargetValue, tc.wantTargetVal)
				}
				if s.WeightKg != nil {
					t.Errorf("set[%d].WeightKg = %v, want nil (allocation done by callers)", i, *s.WeightKg)
				}
				if s.CompletedValue != nil {
					t.Errorf("set[%d].CompletedValue = %v, want nil", i, *s.CompletedValue)
				}
				if s.CompletedAt != nil {
					t.Errorf("set[%d].CompletedAt = %v, want nil", i, *s.CompletedAt)
				}
				if s.Signal != nil {
					t.Errorf("set[%d].Signal = %v, want nil", i, *s.Signal)
				}
			}
		})
	}
}
