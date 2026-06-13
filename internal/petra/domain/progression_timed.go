package domain

import (
	"math"
)

// TimedConfig is provided once when starting a timed exercise execution.
type TimedConfig struct {
	StartingSeconds int // seconds; caller-derived from history, may be user-overridden
}

const (
	timedSnapSeconds   = 5
	timedFloorSeconds  = 5
	timedDecrFraction  = 0.10
	timedStepShort     = 5  // < 60s
	timedStepMid       = 10 // 60–119s
	timedStepLong      = 15 // ≥ 120s
	timedMidThreshold  = 60
	timedLongThreshold = 120
)

// TimedProgression manages duration progression for one timed exercise execution.
type TimedProgression struct {
	config    TimedConfig
	completed []SetResult
}

// NewTimedProgression creates a TimedProgression for a new exercise execution.
func NewTimedProgression(c TimedConfig) *TimedProgression {
	return &TimedProgression{config: c, completed: nil}
}

// NewTimedProgressionFromHistory reconstructs a TimedProgression from sets already completed in this session.
func NewTimedProgressionFromHistory(c TimedConfig, completed []SetResult) *TimedProgression {
	p := NewTimedProgression(c)
	p.completed = make([]SetResult, len(completed))
	copy(p.completed, completed)
	return p
}

// CurrentSet returns the recommended target for the next hold. TargetValue
// carries the seconds goal; WeightKg stays zero (timed holds have no load).
func (p *TimedProgression) CurrentSet() SetTarget {
	if len(p.completed) == 0 {
		return SetTarget{WeightKg: 0, TargetValue: p.config.StartingSeconds}
	}
	last := p.completed[len(p.completed)-1]
	return SetTarget{WeightKg: 0, TargetValue: adjustedSeconds(last)}
}

// RecordCompletion records what actually happened and advances internal state.
func (p *TimedProgression) RecordCompletion(r SetResult) {
	p.completed = append(p.completed, r)
}

// SetsCompleted returns the number of holds recorded so far.
func (p *TimedProgression) SetsCompleted() int {
	return len(p.completed)
}

func adjustedSeconds(last SetResult) int {
	step := timedIncrement(last.ActualValue)
	switch last.Signal {
	case SignalTooLight:
		return snap5(last.ActualValue + step)
	case SignalTooHeavy:
		decrement := step
		if pct := snap5(int(math.Round(float64(last.ActualValue) * timedDecrFraction))); pct > decrement {
			decrement = pct
		}
		next := max(last.ActualValue-decrement, timedFloorSeconds)
		return snap5(next)
	case SignalOnTarget:
		return last.ActualValue
	default:
		// Unknown signal: degrade gracefully to no adjustment. See adjustedWeight.
		return last.ActualValue
	}
}

func timedIncrement(current int) int {
	switch {
	case current < timedMidThreshold:
		return timedStepShort
	case current < timedLongThreshold:
		return timedStepMid
	default:
		return timedStepLong
	}
}

func snap5(seconds int) int {
	return int(math.Round(float64(seconds)/float64(timedSnapSeconds))) * timedSnapSeconds
}
