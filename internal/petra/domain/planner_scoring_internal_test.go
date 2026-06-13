package domain

import "testing"

// The planner's candidate scoring is a private implementation detail tested for
// its behaviour through Plan/PlanDay in planner_test.go (package domain_test).
// These two in-package tests pin the only pure-function invariants the planner
// relies on — properties that are real contracts (not tunable magnitudes) and
// invisible through Plan's output, so they cannot be asserted behaviourally.

// segmentReward's marginal per-set reward must order below-floor > between
// floor-and-ceiling > 0 > past-ceiling. The exact magnitudes are tuning knobs;
// this ordering is the contract pickBestExerciseIdx depends on to drive every
// muscle toward its floor, then keep paying with diminishing returns up to its
// ceiling, then penalise excess.
func Test_segmentReward_OrdersBelowAboveOverMax(t *testing.T) {
	t.Parallel()

	const goal, maxSets = 10.0, 20.0
	below := segmentReward(0, 1, goal, maxSets)  // one set entirely below the floor
	above := segmentReward(10, 1, goal, maxSets) // one set between floor and ceiling
	over := segmentReward(20, 1, goal, maxSets)  // one set entirely past the ceiling

	if !(below > above && above > 0 && 0 > over) {
		t.Errorf("want below > above > 0 > over; got below=%v above=%v over=%v", below, above, over)
	}
}

// goalForWeek must always return a multiple of 0.5. Quantisation keeps every
// segmentReward term on half-integer values so scores stay exact in IEEE-754
// and pickBestExerciseIdx's lowest-id tie-break holds.
func Test_goalForWeek(t *testing.T) {
	t.Parallel()

	chest := MuscleGroupTarget{MuscleGroupName: "Chest", MinSets: 10, MaxSets: 20}
	tests := []struct {
		name     string
		progress float64
		want     float64
	}{
		{"progress 0 → MinSets exactly", 0.0, 10.0},
		{"progress 1 → MaxSets exactly", 1.0, 20.0},
		{"progress 0.5 → midpoint", 0.5, 15.0},
		{"fractional lerp quantised to nearest 0.5", 1.0 / 3.0, 13.5}, // raw 13.333 → 13.5.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := goalForWeek(chest, tt.progress)
			if got != tt.want {
				t.Errorf("goalForWeek(progress=%v) = %v, want %v", tt.progress, got, tt.want)
			}
			// The 0.5-multiple invariant: int64 truncation suffices to check.
			if twice := got * 2; twice != float64(int64(twice)) {
				t.Errorf("goalForWeek(progress=%v) = %v is not a multiple of 0.5", tt.progress, got)
			}
		})
	}
}
