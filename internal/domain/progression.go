package domain

import (
	"math"
)

// Config is provided once when starting an exercise execution.
// RepMin/RepMax describe the exercise's per-session rep window — the
// progression uses DeriveScheme on each CurrentSet() call to know what reps
// to recommend for the next set under the session's periodization.
type Config struct {
	Type           PeriodizationType
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
func (p *Progression) CurrentSet() SetTarget {
	reps := DeriveScheme(p.config.RepMin, p.config.RepMax, p.config.Type, p.config.IsDeload).TargetReps
	if p.config.IsDeload || len(p.completed) == 0 {
		return SetTarget{WeightKg: p.config.StartingWeight, TargetReps: reps}
	}
	last := p.completed[len(p.completed)-1]
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

// SnapWeightKg rounds a kilo value to the nearest realisable load: 1kg
// inside the dumbbell range (|w| < 10kg), 0.5kg above. Exposed for service
// callers that derive weights outside the per-set progression flow.
func SnapWeightKg(kg float64) float64 {
	if math.Abs(kg) < dumbbellThresholdKg {
		return math.Round(kg)
	}
	const halfKg = 0.5
	return math.Round(kg/halfKg) * halfKg
}

// snapWeight rounds to the nearest realisable load: 1kg in the dumbbell
// range, 0.5kg above. User overrides may sit off-grid, so each adjustment
// is snapped before being recommended.
func snapWeight(kg float64) float64 { return SnapWeightKg(kg) }
