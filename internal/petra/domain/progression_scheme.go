package domain

import "fmt"

const (
	// Rep boundaries for set/rest derivation.
	repBoundaryLowToMid  = 5
	repBoundaryMidToHigh = 10

	// Set and rest values for each rep range.
	restLow  = 180 // seconds
	restMid  = 150 // seconds
	restHigh = 90  // seconds

	// deloadSetFloor is the minimum set count a deload prescription will return.
	// Preserves at least two working sets per exercise so deload still functions
	// as training rather than a single confirmation set.
	deloadSetFloor = 2
)

// Scheme is the per-exercise rep + rest prescription for one planned session,
// computed from a per-exercise rep range (repMin, repMax) and the session's
// SessionGoal. Set count is no longer part of Scheme — since Phase D the
// mesocycle week drives set count (see SetsForWeek / BuildPlannedSets), not the
// goal-derived rep band.
type Scheme struct {
	TargetReps  int
	RestSeconds int
}

// DeriveScheme returns the rep target and inter-set rest for one exercise given
// its rep range, the session goal, and whether the session is a
// deload. Pure: same inputs → same output, no DB, no clock. Set count is NOT
// returned — since Phase D it is a function of the mesocycle week, not the rep
// band (see SetsForWeek / deriveSchemeForExercise).
//
// Reps:
//
//	Strength    → repMin (low end of the range)
//	Hypertrophy → repMax (high end of the range)
//	Deload      → repMax always (forces hypertrophy target regardless of p)
//
// Rest is derived from the resulting rep target:
//
//	reps ≤ 5  → 180s rest  (heavy work, full ATP-PCr recovery)
//	reps 6-10 → 150s rest  (moderate; longer rest improves hypertrophy in trained lifters per Schoenfeld 2016)
//	reps ≥ 11 → 90s rest   (lighter; rest shortens)
func DeriveScheme(repMin, repMax int, p SessionGoal, isDeload bool) Scheme {
	if isDeload {
		// Deload forces hypertrophy targets (repMax) regardless of incoming p.
		p = SessionGoalHypertrophy
	}

	var reps int
	switch p {
	case SessionGoalStrength:
		reps = repMin
	case SessionGoalHypertrophy:
		reps = repMax
	default:
		panic(fmt.Sprintf("domain: unknown SessionGoal %q", p))
	}

	var rest int
	switch {
	case reps <= repBoundaryLowToMid:
		rest = restLow
	case reps <= repBoundaryMidToHigh:
		rest = restMid
	default:
		rest = restHigh
	}

	return Scheme{TargetReps: reps, RestSeconds: rest}
}

// deloadSets reduces the week's base set count by one, floored at deloadSetFloor,
// for ~25-33% volume reduction on a deload week.
func deloadSets(normalSets int) int {
	reduced := normalSets - 1
	if reduced < deloadSetFloor {
		return deloadSetFloor
	}
	return reduced
}

// RestSecondsFor returns the inter-set rest in seconds for the given exercise
// under the session's goal. Returns 0 for time-based exercises and
// for exercises with missing rep ranges — service code treats 0 as "no
// rest scheduling".
func RestSecondsFor(ex Exercise, pt SessionGoal, isDeload bool) int {
	if ex.IsTimed() {
		return 0
	}
	if ex.RepMin == nil || ex.RepMax == nil {
		return 0
	}
	return DeriveScheme(*ex.RepMin, *ex.RepMax, pt, isDeload).RestSeconds
}
