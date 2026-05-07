package exerciseprogression

import (
	"fmt"
	"math"
)

// TimedConfig is provided once when starting a timed exercise execution.
type TimedConfig struct {
	StartingSeconds int // seconds; caller-derived from history, may be user-overridden
}

// TimedSetTarget is what the package recommends for the upcoming hold.
type TimedSetTarget struct {
	TargetSeconds int
}

// TimedSetResult is recorded by the caller after the user completes a hold.
type TimedSetResult struct {
	ActualSeconds int
	Signal        Signal
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
	completed []TimedSetResult
}

// NewTimed creates a TimedProgression for a new exercise execution.
func NewTimed(c TimedConfig) *TimedProgression {
	return &TimedProgression{config: c, completed: nil}
}

// NewTimedFromHistory reconstructs a TimedProgression from sets already completed in this session.
func NewTimedFromHistory(c TimedConfig, completed []TimedSetResult) *TimedProgression {
	p := NewTimed(c)
	p.completed = make([]TimedSetResult, len(completed))
	copy(p.completed, completed)
	return p
}

// CurrentSet returns the recommended target for the next hold.
func (p *TimedProgression) CurrentSet() TimedSetTarget {
	if len(p.completed) == 0 {
		return TimedSetTarget{TargetSeconds: p.config.StartingSeconds}
	}
	last := p.completed[len(p.completed)-1]
	return TimedSetTarget{TargetSeconds: adjustedSeconds(last)}
}

// RecordCompletion records what actually happened and advances internal state.
func (p *TimedProgression) RecordCompletion(r TimedSetResult) {
	p.completed = append(p.completed, r)
}

// SetsCompleted returns the number of holds recorded so far.
func (p *TimedProgression) SetsCompleted() int {
	return len(p.completed)
}

func adjustedSeconds(last TimedSetResult) int {
	step := timedIncrement(last.ActualSeconds)
	switch last.Signal {
	case SignalTooLight:
		return snap5(last.ActualSeconds + step)
	case SignalTooHeavy:
		decrement := step
		if pct := snap5(int(math.Round(float64(last.ActualSeconds) * timedDecrFraction))); pct > decrement {
			decrement = pct
		}
		next := max(last.ActualSeconds-decrement, timedFloorSeconds)
		return snap5(next)
	case SignalOnTarget:
		return last.ActualSeconds
	case SignalUnknown:
		panic("exerciseprogression: TimedSetResult must not use SignalUnknown")
	}
	panic(fmt.Sprintf("exerciseprogression: unhandled Signal %d", last.Signal))
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
