package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func monday() time.Time {
	return time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
}

func newWeekPlan() domain.WeekPlan {
	return domain.WeekPlan{ //nolint:exhaustruct // Sessions zero-valued; tests assign per-day as needed.
		Monday: monday(),
	}
}

func sessionOn(offset int, started bool, completed bool, isDeload bool) domain.Session {
	d := monday().AddDate(0, 0, offset)
	s := domain.Session{ //nolint:exhaustruct // test scaffolding.
		Date:     d,
		IsDeload: isDeload,
	}
	if started {
		s.StartedAt = d.Add(8 * time.Hour)
	}
	if completed {
		s.CompletedAt = d.Add(9 * time.Hour)
	}
	s.ExerciseSets = []domain.ExerciseSet{{ //nolint:exhaustruct // test scaffolding.
		ID:   1,
		Sets: []domain.Set{{}}, //nolint:exhaustruct // test scaffolding.
	}}
	s.PeriodizationType = domain.PeriodizationStrength
	return s
}

func TestWeekPlan_SessionOn(t *testing.T) {
	t.Parallel()
	wp := newWeekPlan()
	wp.Sessions[2] = sessionOn(2, false, false, false)
	got := wp.SessionOn(monday().AddDate(0, 0, 2))
	if got == nil {
		t.Fatalf("expected session, got nil")
	}
	if !got.Date.Equal(monday().AddDate(0, 0, 2)) {
		t.Errorf("unexpected date: %v", got.Date)
	}
	if wp.SessionOn(monday().AddDate(0, 0, 8)) != nil {
		t.Error("expected nil for out-of-week date")
	}
}

func TestWeekPlan_AnyStarted(t *testing.T) {
	t.Parallel()
	wp := newWeekPlan()
	if wp.AnyStarted() {
		t.Error("empty week should not report started")
	}
	wp.Sessions[3] = sessionOn(3, true, false, false)
	if !wp.AnyStarted() {
		t.Error("week with one started session should report started")
	}
}

func TestWeekPlan_IsDeloadWeek(t *testing.T) {
	t.Parallel()
	wp := newWeekPlan()
	wp.Sessions[0] = sessionOn(0, false, false, true)
	wp.Sessions[2] = sessionOn(2, false, false, true)
	if !wp.IsDeloadWeek() {
		t.Error("all scheduled deload should report IsDeloadWeek=true")
	}
	wp.Sessions[2].IsDeload = false
	if wp.IsDeloadWeek() {
		t.Error("mixed deload state should report false")
	}
	empty := newWeekPlan()
	if empty.IsDeloadWeek() {
		t.Error("empty week should report false")
	}
}

func TestWeekPlan_FlipDeloadFromToday(t *testing.T) {
	t.Parallel()
	wp := newWeekPlan()
	wp.Sessions[0] = sessionOn(0, true, true, false)   // Mon: completed.
	wp.Sessions[2] = sessionOn(2, true, false, false)  // Wed: started, not completed.
	wp.Sessions[4] = sessionOn(4, false, false, false) // Fri: not started.

	today := monday().AddDate(0, 0, 2) // Wednesday.
	if err := wp.FlipDeloadFromToday(today); err != nil {
		t.Fatalf("FlipDeloadFromToday: %v", err)
	}

	if wp.Sessions[0].IsDeload {
		t.Error("past completed session should be untouched")
	}
	if !wp.Sessions[2].IsDeload {
		t.Error("today's non-completed session should flip")
	}
	if !wp.Sessions[4].IsDeload {
		t.Error("future session should flip")
	}
}

func TestWeekPlan_ClearDeloadFromToday(t *testing.T) {
	t.Parallel()
	wp := newWeekPlan()
	wp.Sessions[0] = sessionOn(0, true, true, true)
	wp.Sessions[2] = sessionOn(2, false, false, true)
	wp.Sessions[4] = sessionOn(4, false, false, true)

	if err := wp.ClearDeloadFromToday(monday().AddDate(0, 0, 2)); err != nil {
		t.Fatalf("ClearDeloadFromToday: %v", err)
	}
	if !wp.Sessions[0].IsDeload {
		t.Error("past completed should keep IsDeload")
	}
	if wp.Sessions[2].IsDeload || wp.Sessions[4].IsDeload {
		t.Error("today and future should be cleared")
	}
}
