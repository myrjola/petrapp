package domain

import (
	"math"
)

// Config is provided once when starting an exercise execution.
// RepMin/RepMax describe the exercise's per-session rep range — the
// progression uses DeriveScheme on each CurrentSet() call to know what reps
// to recommend for the next set under the session's goal.
type Config struct {
	Type           SessionGoal
	RepMin         int
	RepMax         int
	StartingWeight float64 // kg; caller-derived from history, may be user-overridden
	IsDeload       bool
}

// SetTarget is what the package recommends for the upcoming set.
type SetTarget struct {
	WeightKg   float64
	TargetReps int
}

// AbsWeightKg returns the unsigned magnitude of WeightKg. Assisted exercises
// store weight as a negative number to mark assistance use; the form input
// and any other display surface that wants the bare magnitude calls this.
func (s SetTarget) AbsWeightKg() float64 { return math.Abs(s.WeightKg) }

// SetResult is recorded by the caller after the user completes a set.
type SetResult struct {
	ActualReps int
	Signal     Signal
	WeightKg   float64 // weight actually used; may differ from recommendation if user overrode
}

const (
	// dumbbellThresholdKg is the boundary below which loads progress in 1kg
	// steps (matching real-world fixed-weight dumbbell sets) and at/above
	// which they revert to the standard 2.5kg plate increment.
	dumbbellThresholdKg = 10.0

	weightIncrementKgLow  = 1.0
	weightIncrementKgHigh = 2.5
	weightDecrementFactor = 0.10
)

// Progression manages set-to-set weight progression for one exercise execution.
type Progression struct {
	config    Config
	completed []SetResult
}

// NewProgression creates a Progression for a new exercise execution.
func NewProgression(config Config) *Progression {
	return &Progression{config: config, completed: nil}
}

// NewProgressionFromHistory reconstructs a Progression from sets already completed in this session.
func NewProgressionFromHistory(config Config, completed []SetResult) *Progression {
	p := NewProgression(config)
	p.completed = make([]SetResult, len(completed))
	copy(p.completed, completed)
	return p
}

// CurrentSet returns the recommended target for the next set.
//
// Deload sessions hold the seeded starting weight for the first set but then
// carry whatever weight the user actually lifted on the previous set forward
// — no autoregulation, just the override. This lets a one-time correction
// (e.g. dropping a seeded 61 kg to 60 kg because that's what the rack offers)
// propagate to the remaining sets without forcing the user to re-enter it.
func (p *Progression) CurrentSet() SetTarget {
	reps := DeriveScheme(p.config.RepMin, p.config.RepMax, p.config.Type, p.config.IsDeload).TargetReps
	if len(p.completed) == 0 {
		return SetTarget{WeightKg: p.config.StartingWeight, TargetReps: reps}
	}
	last := p.completed[len(p.completed)-1]
	if p.config.IsDeload {
		return SetTarget{WeightKg: last.WeightKg, TargetReps: reps}
	}
	weight := adjustedWeight(last)
	return SetTarget{WeightKg: weight, TargetReps: reps}
}

// RecordCompletion records what actually happened and advances internal state.
func (p *Progression) RecordCompletion(result SetResult) {
	p.completed = append(p.completed, result)
}

// SetsCompleted returns the number of sets recorded so far.
func (p *Progression) SetsCompleted() int {
	return len(p.completed)
}

func adjustedWeight(last SetResult) float64 {
	switch last.Signal {
	case SignalTooLight:
		return snapWeight(last.WeightKg + incrementFor(last.WeightKg))
	case SignalTooHeavy:
		increment := incrementFor(last.WeightKg)
		decrement := math.Max(increment, math.Abs(last.WeightKg)*weightDecrementFactor)
		return snapWeight(last.WeightKg - decrement)
	case SignalOnTarget:
		return last.WeightKg
	default:
		// Unknown signal: degrade gracefully to no adjustment rather than
		// crashing. Signal is DB- and request-validated, so this is a
		// should-be-unreachable safety net.
		return last.WeightKg
	}
}

// incrementFor returns the load step appropriate for the given weight: 1kg
// inside the dumbbell range (|w| < 10kg), 2.5kg otherwise.
func incrementFor(weight float64) float64 {
	if math.Abs(weight) < dumbbellThresholdKg {
		return weightIncrementKgLow
	}
	return weightIncrementKgHigh
}

// snapWeight rounds a kilo value to the nearest realisable load: 1kg in the
// dumbbell range (|w| < 10kg), 0.5kg above. User overrides may sit off-grid,
// so each per-set adjustment is snapped before being recommended.
func snapWeight(kg float64) float64 {
	if math.Abs(kg) < dumbbellThresholdKg {
		return math.Round(kg)
	}
	const halfKg = 0.5
	return math.Round(kg/halfKg) * halfKg
}

// DeloadSeedWeight applies a deload reduction to a working weight, returning
// a definitely-loadable seed for the deload week's first set under the
// commonly-stocked plate set (1, 2.5, 5 kg) — which can't hit 0.5 kg
// precision — and rounded toward the easier of the two adjacent whole-kg
// loads so the deload is genuinely easier than the working weight without
// the user having to round a 60.75-kg recommendation by hand.
//
// factor is a fraction in (0, 1] expressing the deload's relative intensity
// against the working load. For weighted exercises (positive workingKg) the
// load is scaled down and floored: 75 × 0.9 = 67.5 → 67. For assisted
// exercises (negative workingKg, where |w| is machine assistance and a
// larger magnitude means an easier lift) the magnitude is scaled UP by
// 1/factor and rounded UP in magnitude, so -50 with factor 0.9 becomes
// -ceil(50 / 0.9) = -ceil(55.56) = -56 — more assistance than the working
// set, never less.
//
// Returns 0 for a zero working weight (no qualifying history).
func DeloadSeedWeight(workingKg, factor float64) float64 {
	if workingKg < 0 {
		return -math.Ceil(math.Abs(workingKg) / factor)
	}
	return math.Floor(workingKg * factor)
}
