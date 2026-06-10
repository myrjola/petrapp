package domain_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func TestDeriveScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		repMin   int
		repMax   int
		goal     domain.SessionGoal
		wantReps int
		wantRest int
	}{
		// Heavy spinal-load compound (3-6 window).
		{"deadlift strength", 3, 6, domain.SessionGoalStrength, 3, 180},
		{"deadlift hypertrophy", 3, 6, domain.SessionGoalHypertrophy, 6, 150},

		// Non-spinal compound (5-10 window).
		{"bench strength", 5, 10, domain.SessionGoalStrength, 5, 180},
		{"bench hypertrophy", 5, 10, domain.SessionGoalHypertrophy, 10, 150},

		// Lumbar-stress accessory (8-20 window).
		{"back ext strength", 8, 20, domain.SessionGoalStrength, 8, 150},
		{"back ext hypertrophy", 8, 20, domain.SessionGoalHypertrophy, 20, 90},

		// Isolation, large muscle (8-12 window).
		{"bicep curl strength", 8, 12, domain.SessionGoalStrength, 8, 150},
		{"bicep curl hypertrophy", 8, 12, domain.SessionGoalHypertrophy, 12, 90},

		// Isolation, small/slow muscle (10-20 window).
		{"calf strength", 10, 20, domain.SessionGoalStrength, 10, 150},
		{"calf hypertrophy", 10, 20, domain.SessionGoalHypertrophy, 20, 90},

		// Bucket boundaries.
		{"reps=5 (top of low bucket)", 5, 5, domain.SessionGoalStrength, 5, 180},
		{"reps=6 (start of mid bucket)", 6, 6, domain.SessionGoalStrength, 6, 150},
		{"reps=10 (top of mid bucket)", 10, 10, domain.SessionGoalStrength, 10, 150},
		{"reps=11 (start of high bucket)", 11, 11, domain.SessionGoalStrength, 11, 90},

		// Single-value window: same output regardless of goal.
		{"single 5 strength", 5, 5, domain.SessionGoalStrength, 5, 180},
		{"single 5 hypertrophy", 5, 5, domain.SessionGoalHypertrophy, 5, 180},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.DeriveScheme(tt.repMin, tt.repMax, tt.goal, false)
			if got.TargetReps != tt.wantReps {
				t.Errorf("TargetReps: want %d, got %d", tt.wantReps, got.TargetReps)
			}
			if got.RestSeconds != tt.wantRest {
				t.Errorf("RestSeconds: want %d, got %d", tt.wantRest, got.RestSeconds)
			}
		})
	}
}

func TestDeriveSchemePanicOnUnknownSessionGoal(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown SessionGoal")
		}
	}()
	_ = domain.DeriveScheme(5, 10, domain.SessionGoal("unknown"), false)
}

func TestRestSecondsFor(t *testing.T) {
	t.Parallel()

	repMin5, repMax5 := 5, 5
	repMin6, repMax10 := 6, 10
	repMin12, repMax15 := 12, 15
	startSecs := 30

	tests := []struct {
		name string
		ex   domain.Exercise
		pt   domain.SessionGoal
		want int
	}{
		{
			name: "weighted strength 5 reps to 180s",
			ex: domain.Exercise{ //nolint:exhaustruct // Only fields read by RestSecondsFor are set.
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       &repMin5, RepMax: &repMax5,
			},
			pt:   domain.SessionGoalStrength,
			want: 180,
		},
		{
			name: "weighted hypertrophy 10 reps to 150s",
			ex: domain.Exercise{ //nolint:exhaustruct // Only fields read by RestSecondsFor are set.
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       &repMin6, RepMax: &repMax10,
			},
			pt:   domain.SessionGoalHypertrophy,
			want: 150,
		},
		{
			name: "weighted hypertrophy 15 reps to 90s",
			ex: domain.Exercise{ //nolint:exhaustruct // Only fields read by RestSecondsFor are set.
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       &repMin12, RepMax: &repMax15,
			},
			pt:   domain.SessionGoalHypertrophy,
			want: 90,
		},
		{
			name: "time-based exercise to 0 (no scheduling)",
			ex: domain.Exercise{ //nolint:exhaustruct // Only fields read by RestSecondsFor are set.
				ExerciseType:           domain.ExerciseTypeTime,
				DefaultStartingSeconds: &startSecs,
			},
			pt:   domain.SessionGoalStrength,
			want: 0,
		},
		{
			name: "rep-based with nil rep range to 0 (defensive)",
			ex: domain.Exercise{ //nolint:exhaustruct // Only fields read by RestSecondsFor are set.
				ExerciseType: domain.ExerciseTypeWeighted,
			},
			pt:   domain.SessionGoalStrength,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := domain.RestSecondsFor(tt.ex, tt.pt, false)
			if got != tt.want {
				t.Errorf("RestSecondsFor() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDeriveScheme_Deload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		repMin, repMax  int
		goal            domain.SessionGoal
		wantTargetReps  int
		wantRestSeconds int
	}{
		{"low rep range, deload still targets repMax", 3, 5, domain.SessionGoalStrength, 5, 180},
		{"mid rep range, deload still targets repMax", 6, 10, domain.SessionGoalStrength, 10, 150},
		{"high rep range, deload still targets repMax", 12, 15, domain.SessionGoalHypertrophy, 15, 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.DeriveScheme(tt.repMin, tt.repMax, tt.goal, true)
			if got.TargetReps != tt.wantTargetReps {
				t.Errorf("TargetReps = %d, want %d (deload always uses repMax)", got.TargetReps, tt.wantTargetReps)
			}
			if got.RestSeconds != tt.wantRestSeconds {
				t.Errorf(
					"RestSeconds = %d, want %d (unchanged from hypertrophy mapping)",
					got.RestSeconds,
					tt.wantRestSeconds,
				)
			}
		})
	}
}

func TestRestSecondsFor_Deload(t *testing.T) {
	t.Parallel()

	ex := domain.Exercise{ //nolint:exhaustruct // Only fields read by RestSecondsFor are set.
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       new(8),
		RepMax:       new(12),
	}
	if got := domain.RestSecondsFor(ex, domain.SessionGoalStrength, true); got != 90 {
		t.Errorf("RestSecondsFor deload = %d, want 90 (hypertrophy mapping)", got)
	}
}

func Test_deloadSets(t *testing.T) {
	t.Parallel()

	// deloadSets is internal; exercise it through BuildPlannedSets, which now
	// reduces the week's base set count by one (floored at deloadSetFloor=2) on
	// a deload. SessionGoal no longer affects the count.
	ex := domain.Exercise{ //nolint:exhaustruct // Only planning fields read.
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       new(6),
		RepMax:       new(10),
	}
	tests := []struct {
		name     string
		weekSets int
		want     int
	}{
		{"base 3 drops to 2", 3, 2},
		{"peak 4 drops to 3", 4, 3},
		{"already at floor stays 2", 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.BuildPlannedSets(ex, domain.SessionGoalStrength, true, tt.weekSets)
			if len(got) != tt.want {
				t.Errorf("deload set count = %d, want %d", len(got), tt.want)
			}
		})
	}
}
