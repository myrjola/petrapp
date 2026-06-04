package service_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func Test_SaveUserPreferences_SnapsAnchorOnEnable(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	// Anchor starts zero. Enable deload and confirm anchor lands on a Monday >= today.
	prefs := domain.Preferences{ //nolint:exhaustruct // only deload-related fields exercised
		Minutes:         [7]int{time.Monday: 60},
		DeloadEnabled:   true,
		MesocycleLength: 5,
	}
	if err := svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	got, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	if got.MesocycleAnchor.IsZero() {
		t.Fatal("MesocycleAnchor was not set on enable")
	}
	if got.MesocycleAnchor.Weekday() != time.Monday {
		t.Errorf("MesocycleAnchor weekday = %s, want Monday", got.MesocycleAnchor.Weekday())
	}
	if got.MesocycleAnchor.Before(time.Now().UTC().Truncate(24 * time.Hour)) {
		t.Errorf("MesocycleAnchor = %s is in the past", got.MesocycleAnchor)
	}
}

func Test_SaveUserPreferences_NoSnapWhenAnchorAlreadySet(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	existing := time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC)
	// First enable with an explicit anchor.
	first := domain.Preferences{ //nolint:exhaustruct // only deload-related fields exercised
		Minutes:         [7]int{time.Monday: 60},
		DeloadEnabled:   true,
		MesocycleLength: 5,
		MesocycleAnchor: existing,
	}
	if err := svc.SaveUserPreferences(ctx, first); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Toggle a non-deload field; anchor should not move.
	first.Minutes[time.Monday] = 90
	if err := svc.SaveUserPreferences(ctx, first); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.MesocycleAnchor.Equal(existing) {
		t.Errorf("MesocycleAnchor = %s, want %s", got.MesocycleAnchor, existing)
	}
}

func Test_RestartMesocycleAnchor_ClearsCurrentWeekDeloadAfterStartDeloadNow(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	if _, err = svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow: %v", err)
	}
	if err = svc.RestartMesocycleAnchor(ctx); err != nil {
		t.Fatalf("RestartMesocycleAnchor: %v", err)
	}

	plan, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after restart: %v", err)
	}
	sessions := plan.Sessions[:]

	today := domain.StartOfDay(time.Now())
	// Precondition: the assertion loop only checks sessions dated today or
	// later. On weekdays like Sunday every Mon/Wed/Fri session in the current
	// week is already in the past, so the loop body never runs and the test
	// would silently pass. Count forward-looking workout days and skip with a
	// clear message when there are none.
	forwardLookingCount := 0
	for _, s := range sessions {
		if len(s.Slots) == 0 {
			continue
		}
		if !s.Date.Before(today) {
			forwardLookingCount++
		}
	}
	if forwardLookingCount == 0 {
		t.Skip("no forward-looking Mon/Wed/Fri session this week (weekday-sensitive); " +
			"cannot prove RestartMesocycleAnchor cleared current-week deload flips")
	}

	for i, s := range sessions {
		if len(s.Slots) == 0 {
			continue
		}
		if !s.Date.Before(today) && s.IsDeload {
			t.Errorf("session[%d] (%s) should be cleared after restart, still IsDeload",
				i, s.Date.Weekday())
		}
	}
}

// Test_RestartMesocycleAnchor_LeavesCompletedSessionsAlone covers the
// closure's Status() == SessionCompleted short-circuit inside
// RestartMesocycleAnchor: a natural-cadence deload week session that is
// already fully completed before the restart must remain IsDeload == true,
// because the closure returns nil without calling ClearDeload.
//
// Determinism: setupTestService uses Mon/Wed/Fri. The test needs today to be
// a scheduled workout day so we can complete it; otherwise we t.Skip.
func Test_RestartMesocycleAnchor_LeavesCompletedSessionsAlone(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t) // Mon/Wed/Fri 60 min

	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	// Anchor 4 weeks before this Monday so that with MesocycleLength == 5,
	// WeekInBlock for this week is 4 == length-1 — IsDeloadWeek is true and
	// the planner marks every scheduled session in the current week IsDeload.
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday.AddDate(0, 0, -7*4)
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}

	plan, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	sessions := plan.Sessions[:]

	today := domain.StartOfDay(time.Now())
	todayIdx := -1
	for i, s := range sessions {
		if s.Date.Equal(today) {
			todayIdx = i
			break
		}
	}
	if todayIdx == -1 {
		t.Fatalf("today (%s) not found in weekly schedule", today.Weekday())
	}
	todaySess := sessions[todayIdx]
	if len(todaySess.Slots) == 0 {
		t.Skip("today is a rest day in Mon/Wed/Fri schedule; cannot complete a non-existent session")
	}
	if !todaySess.IsDeload {
		t.Fatalf("precondition: today's session (%s) IsDeload = false; "+
			"expected natural-cadence deload week", today.Weekday())
	}

	// Fully complete today's session — CompleteSession auto-starts if needed
	// (see Test_CompleteSession_UnstartedSession_AutoStartsAndCompletes) and
	// sets CompletedAt, which is what flips Status() to SessionCompleted.
	if err = svc.CompleteSession(ctx, today); err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}

	if err = svc.RestartMesocycleAnchor(ctx); err != nil {
		t.Fatalf("RestartMesocycleAnchor: %v", err)
	}

	plan, err = svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after RestartMesocycleAnchor: %v", err)
	}
	sessions = plan.Sessions[:]

	// Today must remain IsDeload == true — the closure's Status() re-check
	// saw SessionCompleted and returned nil without calling ClearDeload.
	if !sessions[todayIdx].IsDeload {
		t.Errorf("today's session (%s) IsDeload = false; closure must skip completed sessions",
			sessions[todayIdx].Date.Weekday())
	}
}
