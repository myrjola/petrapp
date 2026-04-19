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

func TestCurrentSet_SignalAdjustment(t *testing.T) {
	const startWeight = 100.0

	tests := []struct {
		name       string
		signal     exerciseprogression.Signal
		wantWeight float64
	}{
		{
			name:       "TooLight increases by 2.5kg",
			signal:     exerciseprogression.SignalTooLight,
			wantWeight: 102.5,
		},
		{
			name:       "TooHeavy decreases by 10 percent rounded to 0.5kg",
			signal:     exerciseprogression.SignalTooHeavy,
			wantWeight: 90.0, // 100 * 0.9 = 90.0
		},
		{
			name:       "OnTarget keeps same weight",
			signal:     exerciseprogression.SignalOnTarget,
			wantWeight: startWeight,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := exerciseprogression.New(exerciseprogression.Config{
				Type:           exerciseprogression.Hypertrophy,
				StartingWeight: startWeight,
			})
			p.RecordCompletion(exerciseprogression.SetResult{
				ActualReps: 8,
				Signal:     tt.signal,
				WeightKg:   startWeight,
			})
			got := p.CurrentSet()
			if got.WeightKg != tt.wantWeight {
				t.Errorf("WeightKg = %v, want %v", got.WeightKg, tt.wantWeight)
			}
		})
	}
}

func TestCurrentSet_TooHeavyRounding(t *testing.T) {
	// 23kg * 0.9 = 20.7kg → rounds to 20.5
	p := exerciseprogression.New(exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 23.0,
	})
	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 5,
		Signal:     exerciseprogression.SignalTooHeavy,
		WeightKg:   23.0,
	})
	got := p.CurrentSet()
	if got.WeightKg != 20.5 {
		t.Errorf("WeightKg = %v, want 20.5", got.WeightKg)
	}
}
