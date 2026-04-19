package exerciseprogression_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
)

func TestCurrentSet_FirstSet(t *testing.T) {
	tests := []struct {
		name           string
		periodization  exerciseprogression.PeriodizationType
		startingWeight float64
		wantReps       int
		wantWeight     float64
	}{
		{
			name:           "strength returns 5 reps",
			periodization:  exerciseprogression.Strength,
			startingWeight: 80.0,
			wantReps:       5,
			wantWeight:     80.0,
		},
		{
			name:           "hypertrophy returns 8 reps",
			periodization:  exerciseprogression.Hypertrophy,
			startingWeight: 60.0,
			wantReps:       8,
			wantWeight:     60.0,
		},
		{
			name:           "endurance returns 15 reps",
			periodization:  exerciseprogression.Endurance,
			startingWeight: 40.0,
			wantReps:       15,
			wantWeight:     40.0,
		},
		{
			name:           "zero starting weight is returned as-is",
			periodization:  exerciseprogression.Hypertrophy,
			startingWeight: 0.0,
			wantReps:       8,
			wantWeight:     0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := exerciseprogression.New(exerciseprogression.Config{
				Type:           tt.periodization,
				StartingWeight: tt.startingWeight,
			})
			got := p.CurrentSet()
			if got.TargetReps != tt.wantReps {
				t.Errorf("TargetReps = %d, want %d", got.TargetReps, tt.wantReps)
			}
			if got.WeightKg != tt.wantWeight {
				t.Errorf("WeightKg = %v, want %v", got.WeightKg, tt.wantWeight)
			}
		})
	}
}
