package domain

import "time"

// Signal is the user's perceived effort after completing a set.
type Signal string

const (
	SignalTooHeavy Signal = "too_heavy"
	SignalOnTarget Signal = "on_target"
	SignalTooLight Signal = "too_light"
)

// Set represents a single set of an exercise with target and actual performance.
type Set struct {
	WeightKg       *float64   // Nullable for bodyweight and time_based exercises.
	TargetValue    int        // Reps or seconds; unit derived from the parent exercise type.
	CompletedValue *int       // Same unit as TargetValue; nil until the set is completed.
	CompletedAt    *time.Time // Nullable timestamp when set was completed.
	Signal         *Signal    // Nullable; nil until the set is completed.
}
