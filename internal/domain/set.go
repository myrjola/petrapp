package domain

// Signal is the user's perceived effort after completing a set.
type Signal string

const (
	SignalTooHeavy Signal = "too_heavy"
	SignalOnTarget Signal = "on_target"
	SignalTooLight Signal = "too_light"
)
