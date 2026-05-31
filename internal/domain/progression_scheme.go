package domain

import "fmt"

const (
	// Rep boundaries for set/rest derivation.
	repBoundaryLowToMid  = 5
	repBoundaryMidToHigh = 10

	// Set and rest values for each rep range.
	setsLow  = 4
	restLow  = 180 // seconds
	setsMid  = 3
	restMid  = 150 // seconds
	setsHigh = 3
	restHigh = 90 // seconds

	// deloadSetFloor is the minimum set count a deload prescription will return.
	// Preserves at least two working sets per exercise so deload still functions
	// as training rather than a single confirmation set.
	deloadSetFloor = 2
)

// Scheme is the per-exercise prescription for one planned session: the rep
// target, set count, and inter-set rest. Computed from a per-exercise rep
// window (repMin, repMax) and the session's PeriodizationType.
type Scheme struct {
	TargetReps  int
	TargetSets  int
	RestSeconds int
}

// DeriveScheme returns the prescription for one exercise given its rep window,
// the session periodization, and whether the session is a deload. Pure: same
// inputs → same output, no DB, no clock.
//
// Reps:
//
//	Strength    → repMin (low end of the window)
//	Hypertrophy → repMax (high end of the window)
//	Deload      → repMax always (forces hypertrophy target regardless of p)
//
// Sets and rest are derived from the resulting rep target:
//
//	reps ≤ 5  → 4 sets, 180s rest  (heavy work, more sets, full ATP-PCr recovery)
//	reps 6-10 → 3 sets, 150s rest  (moderate; longer rest improves hypertrophy in trained lifters per Schoenfeld 2016)
//	reps ≥ 11 → 3 sets, 90s rest   (lighter; volume kept up, rest shortens)
//
// During deload the set count drops one set (floored at 2); rest stays at the hypertrophy
// mapping for the resulting rep band.
func DeriveScheme(repMin, repMax int, p PeriodizationType, isDeload bool) Scheme {
	if isDeload {
		// Deload forces hypertrophy targets regardless of incoming p, then
		// drops one set (floored at 2). Rest stays at the hypertrophy
		// mapping for the resulting rep band.
		p = PeriodizationHypertrophy
	}

	var reps int
	switch p {
	case PeriodizationStrength:
		reps = repMin
	case PeriodizationHypertrophy:
		reps = repMax
	default:
		panic(fmt.Sprintf("domain: unknown PeriodizationType %q", p))
	}

	var sets, rest int
	switch {
	case reps <= repBoundaryLowToMid:
		sets, rest = setsLow, restLow
	case reps <= repBoundaryMidToHigh:
		sets, rest = setsMid, restMid
	default:
		sets, rest = setsHigh, restHigh
	}

	if isDeload {
		sets = deloadSets(sets)
	}

	return Scheme{TargetReps: reps, TargetSets: sets, RestSeconds: rest}
}

// deloadSets reduces the normal set count by one, floored at deloadSetFloor.
// Targets ~25-33% volume reduction while preserving the Strength-vs-Hypertrophy
// set-count distinction (Strength 4 → 3, Hypertrophy 3 → 2).
func deloadSets(normalSets int) int {
	reduced := normalSets - 1
	if reduced < deloadSetFloor {
		return deloadSetFloor
	}
	return reduced
}

// RestSecondsFor returns the inter-set rest in seconds for the given exercise
// under the session's periodization. Returns 0 for time-based exercises and
// for exercises with missing rep windows — service code treats 0 as "no
// rest scheduling".
func RestSecondsFor(ex Exercise, pt PeriodizationType, isDeload bool) int {
	if ex.IsTimed() {
		return 0
	}
	if ex.RepMin == nil || ex.RepMax == nil {
		return 0
	}
	return DeriveScheme(*ex.RepMin, *ex.RepMax, pt, isDeload).RestSeconds
}
