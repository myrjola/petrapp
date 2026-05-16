package domain

import "time"

// Signal is the user's perceived effort after completing a set.
type Signal string

const (
	SignalTooHeavy Signal = "too_heavy"
	SignalOnTarget Signal = "on_target"
	SignalTooLight Signal = "too_light"
)

// Label returns a human-readable display string for the signal.
// Returns "" for SignalOnTarget so the UI can hide the badge in the expected case.
func (s Signal) Label() string {
	switch s {
	case SignalTooHeavy:
		return "too heavy"
	case SignalTooLight:
		return "too light"
	case SignalOnTarget:
		return ""
	default:
		return ""
	}
}

// Glyph returns a single-character direction indicator for the signal
// (↓ for too-heavy, ↑ for too-light). Empty for SignalOnTarget.
func (s Signal) Glyph() string {
	switch s {
	case SignalTooHeavy:
		return "↓"
	case SignalTooLight:
		return "↑"
	case SignalOnTarget:
		return ""
	default:
		return ""
	}
}

// Set represents a single set of an exercise with target and actual performance.
type Set struct {
	WeightKg       *float64   // Nullable for bodyweight and time_based exercises.
	TargetValue    int        // Reps or seconds; unit derived from the parent exercise type.
	CompletedValue *int       // Same unit as TargetValue; nil until the set is completed.
	CompletedAt    *time.Time // Nullable timestamp when set was completed.
	Signal         *Signal    // Nullable; nil until the set is completed.
}
