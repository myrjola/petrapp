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
)

// Scheme is the per-exercise prescription for one planned session: the rep
// target, set count, and inter-set rest. Computed from a per-exercise rep
// window (repMin, repMax) and the session's PeriodizationType.
type Scheme struct {
	TargetReps  int
	TargetSets  int
	RestSeconds int
}

// DeriveScheme returns the prescription for one exercise given its rep window
// and the session periodization. Pure: same inputs → same output, no DB, no
// clock.
//
// Reps:
//
//	Strength    → repMin (low end of the window)
//	Hypertrophy → repMax (high end of the window)
//
// Sets and rest are derived from the resulting rep target:
//
//	reps ≤ 5  → 4 sets, 180s rest  (heavy work, more sets, full ATP-PCr recovery)
//	reps 6-10 → 3 sets, 150s rest  (moderate; longer rest improves hypertrophy in trained lifters per Schoenfeld 2016)
//	reps ≥ 11 → 3 sets, 90s rest   (lighter; volume kept up, rest shortens)
func DeriveScheme(repMin, repMax int, p PeriodizationType) Scheme {
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

	return Scheme{TargetReps: reps, TargetSets: sets, RestSeconds: rest}
}
