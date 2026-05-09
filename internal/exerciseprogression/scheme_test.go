package exerciseprogression_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
)

func TestDeriveScheme(t *testing.T) {
	tests := []struct {
		name          string
		repMin        int
		repMax        int
		periodization exerciseprogression.PeriodizationType
		wantReps      int
		wantSets      int
		wantRest      int
	}{
		// Heavy spinal-load compound (3-6 window).
		{"deadlift strength", 3, 6, exerciseprogression.Strength, 3, 4, 180},
		{"deadlift hypertrophy", 3, 6, exerciseprogression.Hypertrophy, 6, 3, 150},

		// Non-spinal compound (5-10 window).
		{"bench strength", 5, 10, exerciseprogression.Strength, 5, 4, 180},
		{"bench hypertrophy", 5, 10, exerciseprogression.Hypertrophy, 10, 3, 150},

		// Lumbar-stress accessory (8-20 window).
		{"back ext strength", 8, 20, exerciseprogression.Strength, 8, 3, 150},
		{"back ext hypertrophy", 8, 20, exerciseprogression.Hypertrophy, 20, 3, 90},

		// Isolation, large muscle (8-12 window).
		{"bicep curl strength", 8, 12, exerciseprogression.Strength, 8, 3, 150},
		{"bicep curl hypertrophy", 8, 12, exerciseprogression.Hypertrophy, 12, 3, 90},

		// Isolation, small/slow muscle (10-20 window).
		{"calf strength", 10, 20, exerciseprogression.Strength, 10, 3, 150},
		{"calf hypertrophy", 10, 20, exerciseprogression.Hypertrophy, 20, 3, 90},

		// Bucket boundaries.
		{"reps=5 (top of low bucket)", 5, 5, exerciseprogression.Strength, 5, 4, 180},
		{"reps=6 (start of mid bucket)", 6, 6, exerciseprogression.Strength, 6, 3, 150},
		{"reps=10 (top of mid bucket)", 10, 10, exerciseprogression.Strength, 10, 3, 150},
		{"reps=11 (start of high bucket)", 11, 11, exerciseprogression.Strength, 11, 3, 90},

		// Single-value window: same output regardless of periodization.
		{"single 5 strength", 5, 5, exerciseprogression.Strength, 5, 4, 180},
		{"single 5 hypertrophy", 5, 5, exerciseprogression.Hypertrophy, 5, 4, 180},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exerciseprogression.DeriveScheme(tt.repMin, tt.repMax, tt.periodization)
			if got.TargetReps != tt.wantReps {
				t.Errorf("TargetReps: want %d, got %d", tt.wantReps, got.TargetReps)
			}
			if got.TargetSets != tt.wantSets {
				t.Errorf("TargetSets: want %d, got %d", tt.wantSets, got.TargetSets)
			}
			if got.RestSeconds != tt.wantRest {
				t.Errorf("RestSeconds: want %d, got %d", tt.wantRest, got.RestSeconds)
			}
		})
	}
}

func TestDeriveSchemePanicOnUnknownPeriodization(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown PeriodizationType")
		}
	}()
	_ = exerciseprogression.DeriveScheme(5, 10, exerciseprogression.PeriodizationType(99))
}
