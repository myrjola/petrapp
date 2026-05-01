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

func TestCurrentSet_OverridePropagates(t *testing.T) {
	// Recommended set 1 = 100kg. User overrides to 95kg and signals OnTarget.
	// Set 2 recommendation must be 95kg (from actual), not 100kg.
	p := exerciseprogression.New(exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 100.0,
	})
	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 8,
		Signal:     exerciseprogression.SignalOnTarget,
		WeightKg:   95.0, // user lifted less than recommended
	})
	got := p.CurrentSet()
	if got.WeightKg != 95.0 {
		t.Errorf("WeightKg = %v, want 95.0 (override weight)", got.WeightKg)
	}
}

func TestCurrentSet_OverrideThenTooLight(t *testing.T) {
	// User overrides set 2 to 90kg and signals TooLight.
	// Set 3 must be 90 + 2.5 = 92.5kg.
	p := exerciseprogression.New(exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 100.0,
	})
	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 8,
		Signal:     exerciseprogression.SignalOnTarget,
		WeightKg:   100.0,
	})
	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 8,
		Signal:     exerciseprogression.SignalTooLight,
		WeightKg:   90.0, // user overrode set 2 down to 90kg
	})
	got := p.CurrentSet()
	if got.WeightKg != 92.5 {
		t.Errorf("WeightKg = %v, want 92.5", got.WeightKg)
	}
}

func TestNewFromHistory_MatchesReplay(t *testing.T) {
	config := exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 80.0,
	}
	results := []exerciseprogression.SetResult{
		{ActualReps: 8, Signal: exerciseprogression.SignalTooLight, WeightKg: 80.0},
		{ActualReps: 8, Signal: exerciseprogression.SignalOnTarget, WeightKg: 82.5},
	}

	// Build via replay.
	replay := exerciseprogression.New(config)
	for _, r := range results {
		replay.RecordCompletion(r)
	}

	// Build via NewFromHistory.
	history := exerciseprogression.NewFromHistory(config, results)

	replayTarget := replay.CurrentSet()
	historyTarget := history.CurrentSet()

	if replayTarget != historyTarget {
		t.Errorf("NewFromHistory CurrentSet = %+v, want %+v", historyTarget, replayTarget)
	}
	if history.SetsCompleted() != replay.SetsCompleted() {
		t.Errorf("SetsCompleted = %d, want %d", history.SetsCompleted(), replay.SetsCompleted())
	}
}

func TestNewFromHistory_EmptySliceEqualsNew(t *testing.T) {
	config := exerciseprogression.Config{
		Type:           exerciseprogression.Strength,
		StartingWeight: 60.0,
	}
	fresh := exerciseprogression.New(config)
	fromEmpty := exerciseprogression.NewFromHistory(config, nil)

	if fresh.CurrentSet() != fromEmpty.CurrentSet() {
		t.Errorf("CurrentSet mismatch: fresh=%+v history=%+v", fresh.CurrentSet(), fromEmpty.CurrentSet())
	}
}

func TestSetsCompleted(t *testing.T) {
	p := exerciseprogression.New(exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 60.0,
	})

	if p.SetsCompleted() != 0 {
		t.Errorf("SetsCompleted before any sets = %d, want 0", p.SetsCompleted())
	}

	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 8,
		Signal:     exerciseprogression.SignalOnTarget,
		WeightKg:   60.0,
	})
	if p.SetsCompleted() != 1 {
		t.Errorf("SetsCompleted after 1 set = %d, want 1", p.SetsCompleted())
	}

	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 8,
		Signal:     exerciseprogression.SignalTooLight,
		WeightKg:   60.0,
	})
	if p.SetsCompleted() != 2 {
		t.Errorf("SetsCompleted after 2 sets = %d, want 2", p.SetsCompleted())
	}
}

func TestAdjustedWeight_AssistedAndZeroBoundary(t *testing.T) {
	tests := []struct {
		name       string
		lastWeight float64
		signal     exerciseprogression.Signal
		wantWeight float64
	}{
		{
			name:       "negative TooHeavy goes further negative (more assistance)",
			lastWeight: -20.0,
			signal:     exerciseprogression.SignalTooHeavy,
			wantWeight: -22.5,
		},
		{
			name:       "zero TooHeavy goes negative (zero boundary fixed)",
			lastWeight: 0.0,
			signal:     exerciseprogression.SignalTooHeavy,
			wantWeight: -2.5,
		},
		{
			name:       "negative TooLight goes less negative (less assistance)",
			lastWeight: -20.0,
			signal:     exerciseprogression.SignalTooLight,
			wantWeight: -17.5,
		},
		{
			name:       "negative OnTarget holds steady",
			lastWeight: -20.0,
			signal:     exerciseprogression.SignalOnTarget,
			wantWeight: -20.0,
		},
		{
			name:       "low positive TooHeavy uses 2.5kg minimum decrement",
			lastWeight: 10.0,
			signal:     exerciseprogression.SignalTooHeavy,
			wantWeight: 7.5,
		},
		{
			name:       "high positive TooHeavy uses percentage (regression)",
			lastWeight: 100.0,
			signal:     exerciseprogression.SignalTooHeavy,
			wantWeight: 90.0,
		},
		{
			name:       "mid positive TooHeavy uses percentage (regression)",
			lastWeight: 50.0,
			signal:     exerciseprogression.SignalTooHeavy,
			wantWeight: 45.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := exerciseprogression.NewFromHistory(
				exerciseprogression.Config{
					Type:           exerciseprogression.Strength,
					StartingWeight: 0,
				},
				[]exerciseprogression.SetResult{
					{ActualReps: 5, Signal: tt.signal, WeightKg: tt.lastWeight},
				},
			)
			got := p.CurrentSet().WeightKg
			if got != tt.wantWeight {
				t.Errorf("WeightKg = %v, want %v", got, tt.wantWeight)
			}
		})
	}
}
