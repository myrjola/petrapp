package service_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func Test_SaveUserPreferences_SnapsAnchorOnEnable(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	// Anchor starts zero. Enable deload and confirm anchor lands on a Monday >= today.
	prefs := domain.Preferences{ //nolint:exhaustruct // only deload-related fields exercised
		MondayMinutes:   60,
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
		MondayMinutes:   60,
		DeloadEnabled:   true,
		MesocycleLength: 5,
		MesocycleAnchor: existing,
	}
	if err := svc.SaveUserPreferences(ctx, first); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Toggle a non-deload field; anchor should not move.
	first.MondayMinutes = 90
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

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after restart: %v", err)
	}

	today := domain.StartOfDay(time.Now())
	for i, s := range sessions {
		if len(s.ExerciseSets) == 0 {
			continue
		}
		if !s.Date.Before(today) && s.IsDeload {
			t.Errorf("session[%d] (%s) should be cleared after restart, still IsDeload",
				i, s.Date.Weekday())
		}
	}
}
