package service_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func Test_SaveUserPreferences_SnapsAnchorOnEnable(t *testing.T) {
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
