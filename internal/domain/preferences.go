package domain

import "time"

// Preferences stores how long a user wants to work out each day of the week.
// Minutes is indexed by time.Weekday (Sunday=0 … Saturday=6); a value of 0
// means rest day, any positive integer means workout day with that duration
// in minutes.
type Preferences struct {
	Minutes                  [7]int
	RestNotificationsEnabled bool
	DeloadEnabled            bool
	MesocycleLength          int
	MesocycleAnchor          time.Time
}

// IsEmpty reports whether no workout days are scheduled.
func (p Preferences) IsEmpty() bool {
	for _, m := range p.Minutes {
		if m > 0 {
			return false
		}
	}
	return true
}

// MinutesForDay returns the planned workout duration in minutes for the
// given weekday. Returns 0 for rest days.
func (p Preferences) MinutesForDay(weekday time.Weekday) int {
	return p.Minutes[weekday]
}

// IsWorkoutDay reports whether the given weekday is a workout day.
func (p Preferences) IsWorkoutDay(weekday time.Weekday) bool {
	return p.MinutesForDay(weekday) > 0
}
