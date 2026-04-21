package weekplanner

import (
	"testing"
	"time"
)

// monday2026 is 2026-01-05, a known Monday.
var monday2026 = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

func date(base time.Time, offsetDays int) time.Time {
	return base.AddDate(0, 0, offsetDays)
}

func prefs(days ...time.Weekday) Preferences {
	p := Preferences{}
	for _, d := range days {
		switch d {
		case time.Monday:
			p.MondayMinutes = 60
		case time.Tuesday:
			p.TuesdayMinutes = 60
		case time.Wednesday:
			p.WednesdayMinutes = 60
		case time.Thursday:
			p.ThursdayMinutes = 60
		case time.Friday:
			p.FridayMinutes = 60
		case time.Saturday:
			p.SaturdayMinutes = 60
		case time.Sunday:
			p.SundayMinutes = 60
		}
	}
	return p
}

func TestDetermineCategory(t *testing.T) {
	tests := []struct {
		name     string
		prefs    Preferences
		date     time.Time
		expected Category
	}{
		{
			name:     "isolated day is full body",
			prefs:    prefs(time.Monday, time.Wednesday, time.Friday),
			date:     monday2026, // Mon: tomorrow=Tue not workout, yesterday=Sun not workout
			expected: CategoryFullBody,
		},
		{
			name:     "first of consecutive days is lower",
			prefs:    prefs(time.Monday, time.Tuesday),
			date:     monday2026, // Mon: tomorrow=Tue is workout
			expected: CategoryLower,
		},
		{
			name:     "second of consecutive days is upper",
			prefs:    prefs(time.Monday, time.Tuesday),
			date:     date(monday2026, 1), // Tue: yesterday=Mon was workout
			expected: CategoryUpper,
		},
		{
			name:     "week wrap: Sunday before Monday is lower",
			prefs:    prefs(time.Sunday, time.Monday, time.Tuesday),
			date:     date(monday2026, 6), // Sun (next week context doesn't matter — prefs wrap)
			expected: CategoryLower,       // Sun: today=workout, tomorrow=Mon=workout
		},
		{
			name:     "week wrap: Monday after Sunday is upper",
			prefs:    prefs(time.Sunday, time.Monday),
			date:     monday2026, // Mon: yesterday=Sun=workout
			expected: CategoryUpper,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wp := NewWeeklyPlanner(tt.prefs, nil, nil)
			got := wp.determineCategory(tt.date)
			if got != tt.expected {
				t.Errorf("determineCategory(%s) = %s, want %s", tt.date.Weekday(), got, tt.expected)
			}
		})
	}
}
