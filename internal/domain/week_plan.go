package domain

import (
	"time"
)

// WeekPlan is the aggregate root for one calendar week of a user's training.
// It owns seven Session values indexed by day-of-week (0 = Monday). Rest days
// carry an empty Session{Date: ...} with no ExerciseSets.
//
// All cross-week operations (regenerate, deload flip, mesocycle restart) are
// methods on *WeekPlan and are atomic when invoked inside a
// WeekPlanRepository.Update closure.
type WeekPlan struct {
	Monday   time.Time
	Sessions [7]Session
}

// PeriodizationType returns the week-wide periodization style. Every scheduled
// session shares the same value (enforced by the planner and by the repo).
// Returns the zero value when the week has no scheduled sessions.
func (wp *WeekPlan) PeriodizationType() PeriodizationType {
	for i := range wp.Sessions {
		if len(wp.Sessions[i].ExerciseSets) > 0 {
			return wp.Sessions[i].PeriodizationType
		}
	}
	return ""
}

// SessionOn returns a pointer to the session for date, or nil if date falls
// outside this WeekPlan's week. The returned pointer aliases into wp.Sessions
// so dispatchers can mutate in place.
func (wp *WeekPlan) SessionOn(date time.Time) *Session {
	d := StartOfDay(date)
	for i := range wp.Sessions {
		if wp.Sessions[i].Date.Equal(d) {
			return &wp.Sessions[i]
		}
	}
	return nil
}

// AnyStarted reports whether any session in the week has StartedAt set.
func (wp *WeekPlan) AnyStarted() bool {
	for i := range wp.Sessions {
		if !wp.Sessions[i].StartedAt.IsZero() {
			return true
		}
	}
	return false
}

// IsDeloadWeek reports whether every scheduled session is a deload session.
// Rest days are ignored. Returns false when the week has no scheduled sessions.
func (wp *WeekPlan) IsDeloadWeek() bool {
	scheduled := 0
	deload := 0
	for i := range wp.Sessions {
		if len(wp.Sessions[i].ExerciseSets) == 0 {
			continue
		}
		scheduled++
		if wp.Sessions[i].IsDeload {
			deload++
		}
	}
	return scheduled > 0 && scheduled == deload
}

// Replace replaces the plan with newPlan, preserving the Monday. Used by
// RegenerateIfUnstarted; callers normally don't invoke this directly.
func (wp *WeekPlan) Replace(newPlan WeekPlan) {
	wp.Sessions = newPlan.Sessions
}

// FlipDeloadFromToday sets IsDeload=true on every non-completed session whose
// Date is on or after today. Past sessions and completed sessions are left
// untouched. Idempotent.
func (wp *WeekPlan) FlipDeloadFromToday(today time.Time) error {
	t := StartOfDay(today)
	for i := range wp.Sessions {
		s := &wp.Sessions[i]
		if s.Date.Before(t) {
			continue
		}
		if s.Status() == SessionCompleted {
			continue
		}
		if err := s.SwitchToDeload(); err != nil {
			return err
		}
	}
	return nil
}

// ClearDeloadFromToday sets IsDeload=false on every non-completed session whose
// Date is on or after today. Counterpart to FlipDeloadFromToday. Idempotent.
func (wp *WeekPlan) ClearDeloadFromToday(today time.Time) error {
	t := StartOfDay(today)
	for i := range wp.Sessions {
		s := &wp.Sessions[i]
		if s.Date.Before(t) {
			continue
		}
		if s.Status() == SessionCompleted {
			continue
		}
		if err := s.ClearDeload(); err != nil {
			return err
		}
	}
	return nil
}
