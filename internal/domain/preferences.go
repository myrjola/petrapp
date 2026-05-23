package domain

import "time"

// Preferences stores how long a user wants to work out each day of the week.
// A value of 0 means rest day; any positive integer means workout day with
// that duration in minutes.
type Preferences struct {
	MondayMinutes            int
	TuesdayMinutes           int
	WednesdayMinutes         int
	ThursdayMinutes          int
	FridayMinutes            int
	SaturdayMinutes          int
	SundayMinutes            int
	RestNotificationsEnabled bool
	DeloadEnabled            bool
	MesocycleLength          int
	MesocycleAnchor          time.Time
}

func (p Preferences) Monday() bool    { return p.MondayMinutes > 0 }
func (p Preferences) Tuesday() bool   { return p.TuesdayMinutes > 0 }
func (p Preferences) Wednesday() bool { return p.WednesdayMinutes > 0 }
func (p Preferences) Thursday() bool  { return p.ThursdayMinutes > 0 }
func (p Preferences) Friday() bool    { return p.FridayMinutes > 0 }
func (p Preferences) Saturday() bool  { return p.SaturdayMinutes > 0 }
func (p Preferences) Sunday() bool    { return p.SundayMinutes > 0 }

// IsEmpty reports whether no workout days are scheduled.
func (p Preferences) IsEmpty() bool {
	for d := time.Sunday; d <= time.Saturday; d++ {
		if p.IsWorkoutDay(d) {
			return false
		}
	}
	return true
}

// MinutesForDay returns the planned workout duration in minutes for the
// given weekday. Returns 0 for rest days.
func (p Preferences) MinutesForDay(weekday time.Weekday) int {
	switch weekday {
	case time.Monday:
		return p.MondayMinutes
	case time.Tuesday:
		return p.TuesdayMinutes
	case time.Wednesday:
		return p.WednesdayMinutes
	case time.Thursday:
		return p.ThursdayMinutes
	case time.Friday:
		return p.FridayMinutes
	case time.Saturday:
		return p.SaturdayMinutes
	case time.Sunday:
		return p.SundayMinutes
	default:
		return 0
	}
}

// IsWorkoutDay reports whether the given weekday is a workout day.
func (p Preferences) IsWorkoutDay(weekday time.Weekday) bool {
	return p.MinutesForDay(weekday) > 0
}
