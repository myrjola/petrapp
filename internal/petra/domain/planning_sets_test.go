package domain_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func Test_BuildPlannedSets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		exercise      domain.Exercise
		periodization domain.PeriodizationType
		weekSets      int
		wantTargetVal int
		wantSetCount  int
	}{
		{
			name: "weighted Strength: low end of window, week-driven set count, nil weight (BuildSetsForAdd allocates)",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       new(5),
				RepMax:       new(10),
			},
			periodization: domain.PeriodizationStrength,
			weekSets:      4,
			wantTargetVal: 5,
			wantSetCount:  4, // set count comes from weekSets, not the rep band
		},
		{
			name: "weighted Hypertrophy: high end, week-driven set count, nil weight (BuildSetsForAdd allocates)",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       new(5),
				RepMax:       new(10),
			},
			periodization: domain.PeriodizationHypertrophy,
			weekSets:      3,
			wantTargetVal: 10,
			wantSetCount:  3, // set count comes from weekSets
		},
		{
			name: "weighted Hypertrophy: high-rep window, week-driven set count, nil weight (BuildSetsForAdd allocates)",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       new(8),
				RepMax:       new(12),
			},
			periodization: domain.PeriodizationHypertrophy,
			weekSets:      3,
			wantTargetVal: 12,
			wantSetCount:  3, // set count comes from weekSets
		},
		{
			name: "assisted exercise: nil weight pointer (BuildSetsForAdd allocates)",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeAssisted,
				RepMin:       new(5),
				RepMax:       new(10),
			},
			periodization: domain.PeriodizationStrength,
			weekSets:      4,
			wantTargetVal: 5,
			wantSetCount:  4,
		},
		{
			name: "bodyweight exercise: nil weight",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType: domain.ExerciseTypeBodyweight,
				RepMin:       new(8),
				RepMax:       new(12),
			},
			periodization: domain.PeriodizationStrength,
			weekSets:      3,
			wantTargetVal: 8,
			wantSetCount:  3, // set count comes from weekSets
		},
		{
			name: "time_based exercise: nil weight, fixed 3 sets, ignores weekSets",
			exercise: domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildPlannedSets are set.
				ExerciseType:           domain.ExerciseTypeTime,
				DefaultStartingSeconds: new(45),
			},
			periodization: domain.PeriodizationStrength,
			weekSets:      5,
			wantTargetVal: 45,
			wantSetCount:  3, // fixed set count for time-based exercises (ignores weekSets)
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.BuildPlannedSets(tc.exercise, tc.periodization, false, tc.weekSets)
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

func Test_BuildSetsForAdd(t *testing.T) {
	t.Parallel()

	weightPtr := func(w float64) *float64 { return &w }

	weighted := domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildSetsForAdd are set.
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       new(5),
		RepMax:       new(10),
	}
	assisted := domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildSetsForAdd are set.
		ExerciseType: domain.ExerciseTypeAssisted,
		RepMin:       new(5),
		RepMax:       new(10),
	}
	bodyweight := domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildSetsForAdd are set.
		ExerciseType: domain.ExerciseTypeBodyweight,
		RepMin:       new(8),
		RepMax:       new(12),
	}
	timeBased := domain.Exercise{ //nolint:exhaustruct // Only fields read by BuildSetsForAdd are set.
		ExerciseType:           domain.ExerciseTypeTime,
		DefaultStartingSeconds: new(45),
	}

	t.Run("weighted with no history allocates zero-valued weight pointer", func(t *testing.T) {
		t.Parallel()
		sets := domain.BuildSetsForAdd(weighted, domain.PeriodizationStrength, false, 4, nil)
		if len(sets) != 4 {
			t.Fatalf("len = %d, want 4", len(sets))
		}
		for i, s := range sets {
			if s.WeightKg == nil {
				t.Errorf("set[%d].WeightKg = nil, want allocated", i)
			} else if *s.WeightKg != 0 {
				t.Errorf("set[%d].WeightKg = %v, want 0", i, *s.WeightKg)
			}
		}
	})

	t.Run("weighted seeds from most recent non-nil historical weight", func(t *testing.T) {
		t.Parallel()
		history := []domain.Set{
			{WeightKg: weightPtr(60), TargetValue: 0, CompletedValue: nil, CompletedAt: nil, Signal: nil},
			{WeightKg: weightPtr(62.5), TargetValue: 0, CompletedValue: nil, CompletedAt: nil, Signal: nil},
			{WeightKg: nil, TargetValue: 0, CompletedValue: nil, CompletedAt: nil, Signal: nil}, // never recorded
		}
		sets := domain.BuildSetsForAdd(weighted, domain.PeriodizationHypertrophy, false, 4, history)
		for i, s := range sets {
			if s.WeightKg == nil || *s.WeightKg != 62.5 {
				t.Errorf("set[%d].WeightKg = %v, want 62.5", i, s.WeightKg)
			}
		}
	})

	t.Run("weighted with history of all-nil weights allocates zero", func(t *testing.T) {
		t.Parallel()
		history := []domain.Set{
			{WeightKg: nil, TargetValue: 0, CompletedValue: nil, CompletedAt: nil, Signal: nil},
			{WeightKg: nil, TargetValue: 0, CompletedValue: nil, CompletedAt: nil, Signal: nil},
		}
		sets := domain.BuildSetsForAdd(weighted, domain.PeriodizationStrength, false, 4, history)
		for i, s := range sets {
			if s.WeightKg == nil || *s.WeightKg != 0 {
				t.Errorf("set[%d].WeightKg = %v, want 0", i, s.WeightKg)
			}
		}
	})

	t.Run("assisted preserves negative seed weight", func(t *testing.T) {
		t.Parallel()
		history := []domain.Set{
			{WeightKg: weightPtr(-20), TargetValue: 0, CompletedValue: nil, CompletedAt: nil, Signal: nil},
		}
		sets := domain.BuildSetsForAdd(assisted, domain.PeriodizationStrength, false, 4, history)
		for i, s := range sets {
			if s.WeightKg == nil || *s.WeightKg != -20 {
				t.Errorf("set[%d].WeightKg = %v, want -20", i, s.WeightKg)
			}
		}
	})

	t.Run("bodyweight leaves WeightKg nil regardless of history", func(t *testing.T) {
		t.Parallel()
		history := []domain.Set{
			{WeightKg: weightPtr(100), TargetValue: 0, CompletedValue: nil, CompletedAt: nil, Signal: nil},
		}
		sets := domain.BuildSetsForAdd(bodyweight, domain.PeriodizationStrength, false, 4, history)
		for i, s := range sets {
			if s.WeightKg != nil {
				t.Errorf("set[%d].WeightKg = %v, want nil", i, *s.WeightKg)
			}
		}
	})

	t.Run("time-based leaves WeightKg nil regardless of history", func(t *testing.T) {
		t.Parallel()
		history := []domain.Set{
			{WeightKg: weightPtr(100), TargetValue: 0, CompletedValue: nil, CompletedAt: nil, Signal: nil},
		}
		sets := domain.BuildSetsForAdd(timeBased, domain.PeriodizationStrength, false, 4, history)
		for i, s := range sets {
			if s.WeightKg != nil {
				t.Errorf("set[%d].WeightKg = %v, want nil", i, *s.WeightKg)
			}
		}
	})

	t.Run("each set gets independent weight pointer", func(t *testing.T) {
		t.Parallel()
		history := []domain.Set{
			{WeightKg: weightPtr(80), TargetValue: 0, CompletedValue: nil, CompletedAt: nil, Signal: nil},
		}
		sets := domain.BuildSetsForAdd(weighted, domain.PeriodizationStrength, false, 4, history)
		if len(sets) < 2 {
			t.Fatalf("need at least 2 sets to verify pointer independence, got %d", len(sets))
		}
		*sets[0].WeightKg = 999
		if *sets[1].WeightKg != 80 {
			t.Errorf("mutating set[0].WeightKg leaked into set[1]: got %v, want 80", *sets[1].WeightKg)
		}
	})
}

func TestBuildPlannedSets_Deload(t *testing.T) {
	t.Parallel()

	ex := domain.Exercise{ //nolint:exhaustruct // Only the planning fields are read.
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       new(8),
		RepMax:       new(12),
	}
	got := domain.BuildPlannedSets(ex, domain.PeriodizationStrength, true, 3)
	// weekSets=3; deload drops one set to 2 (floor).
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (deload drops to 2 (floor))", len(got))
	}
	for i, s := range got {
		if s.TargetValue != 12 {
			t.Errorf("set %d TargetValue = %d, want 12 (deload forces repMax)", i, s.TargetValue)
		}
	}
}
