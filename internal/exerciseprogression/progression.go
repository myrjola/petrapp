// Package exerciseprogression manages set-to-set weight progression for a single
// weighted exercise execution using RIR-based auto-regulation.
package exerciseprogression

import (
	"fmt"
	"math"
)

// Signal is the user's perceived effort after completing a set.
type Signal int

const (
	SignalUnknown  Signal = iota // zero value; must not be used
	SignalTooHeavy               // failed to complete target reps
	SignalOnTarget               // completed reps with ~1-2 in reserve
	SignalTooLight               // completed reps with 2+ in reserve
)

// PeriodizationType determines the fixed rep target for the exercise.
type PeriodizationType int

const (
	Strength    PeriodizationType = iota // 5 reps
	Hypertrophy                          // 8 reps
	Endurance                            // 15 reps
)

// Config is provided once when starting an exercise execution.
type Config struct {
	Type           PeriodizationType
	StartingWeight float64 // kg; caller-derived from history, may be user-overridden
}

// SetTarget is what the package recommends for the upcoming set.
type SetTarget struct {
	WeightKg   float64
	TargetReps int
}

// SetResult is recorded by the caller after the user completes a set.
type SetResult struct {
	ActualReps int
	Signal     Signal
	WeightKg   float64 // weight actually used; may differ from recommendation if user overrode
}

const (
	repsStrength    = 5
	repsHypertrophy = 8
	repsEndurance   = 15

	weightIncrementKg     = 2.5
	weightDecrementFactor = 0.10
)

// Progression manages set-to-set weight progression for one exercise execution.
type Progression struct {
	config    Config
	completed []SetResult
}

// New creates a Progression for a new exercise execution.
func New(config Config) *Progression {
	return &Progression{config: config}
}

// NewFromHistory reconstructs a Progression from sets already completed in this session.
func NewFromHistory(config Config, completed []SetResult) *Progression {
	p := New(config)
	p.completed = make([]SetResult, len(completed))
	copy(p.completed, completed)
	return p
}

// CurrentSet returns the recommended target for the next set.
func (p *Progression) CurrentSet() SetTarget {
	reps := targetReps(p.config.Type)
	if len(p.completed) == 0 {
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

func targetReps(t PeriodizationType) int {
	switch t {
	case Strength:
		return repsStrength
	case Hypertrophy:
		return repsHypertrophy
	case Endurance:
		return repsEndurance
	default:
		panic(fmt.Sprintf("exerciseprogression: unknown PeriodizationType %d", t))
	}
}

func adjustedWeight(last SetResult) float64 {
	switch last.Signal {
	case SignalTooLight:
		return last.WeightKg + weightIncrementKg
	case SignalTooHeavy:
		decreased := last.WeightKg * (1 - weightDecrementFactor)
		return roundToHalf(decreased)
	default: // SignalOnTarget
		return last.WeightKg
	}
}

func roundToHalf(kg float64) float64 {
	return math.Round(kg/0.5) * 0.5
}
