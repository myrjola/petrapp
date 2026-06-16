package domain

import "testing"

// goalForWeek's half-integer quantisation is a numeric invariant with no public
// seam: scoreCandidate sums per-muscle-group rewards by ranging over a map
// (Go randomises map-iteration order), and float addition is not associative,
// so the only thing keeping an exercise's score identical run-to-run — and thus
// keeping Plan's output reproducible and pickBestExerciseIdx's lowest-id
// tie-break meaningful — is that every term is an exact multiple of 0.5. The
// reward magnitudes are already half-integers; goalForWeek is where a fractional
// ramp progress (e.g. 1/3 → 13.333…) would otherwise leak a non-half-integer
// segment boundary into segmentReward. Plan_Deterministic cannot catch a
// regression here (one binary picks the same map order twice by luck), so the
// invariant is pinned directly. The scoring *ordering* is a separate matter and
// is asserted behaviourally through PlanDay in planner_test.go.

// goalForWeek must always return a multiple of 0.5 — see the file comment for
// why exactness, not the ramp value itself, is the contract under test.
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
