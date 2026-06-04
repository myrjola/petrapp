package repository_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func TestPreferencesRepository_GetEmptyReturnsZeroValue(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get on empty: %v", err)
	}
	want := domain.Preferences{ //nolint:exhaustruct // Weekday minutes still zero by design.
		RestNotificationsEnabled: true,
		MesocycleLength:          5,
	}
	if got != want {
		t.Errorf("empty Get: want %+v, got %+v", want, got)
	}
}

func TestPreferences_RestNotificationsEnabled_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	// Default for first-time users is true.
	prefs, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !prefs.RestNotificationsEnabled {
		t.Errorf("default RestNotificationsEnabled = false, want true")
	}

	// Flip to false and confirm.
	prefs.RestNotificationsEnabled = false
	if err = repos.Preferences.Set(ctx, prefs); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got.RestNotificationsEnabled {
		t.Errorf("after Set false, got true")
	}
}

func TestPreferencesRepository_SetThenGetRoundTrip(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	set := domain.Preferences{ //nolint:exhaustruct // Untouched days stay zero.
		Minutes: [7]int{
			time.Monday:    60,
			time.Wednesday: 45,
			time.Friday:    90,
		},
	}
	if err := repos.Preferences.Set(ctx, set); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// MesocycleLength defaults to 5 when not explicitly set.
	want := set
	want.MesocycleLength = 5
	if got != want {
		t.Errorf("round-trip: want %+v, got %+v", want, got)
	}
}

func TestPreferencesRepository_SetUpdatesExisting(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	if err := repos.Preferences.Set(ctx, domain.Preferences{ //nolint:exhaustruct // First write.
		Minutes: [7]int{time.Monday: 45},
	}); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	updated := domain.Preferences{ //nolint:exhaustruct // Second write — Monday changes, others stay zero.
		Minutes: [7]int{time.Monday: 90, time.Tuesday: 45},
	}
	if err := repos.Preferences.Set(ctx, updated); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// MesocycleLength defaults to 5 when not explicitly set.
	want := updated
	want.MesocycleLength = 5
	if got != want {
		t.Errorf("after upsert: want %+v, got %+v", want, got)
	}
}

func TestPreferencesRepository_DeloadFields(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	prefs := domain.Preferences{ //nolint:exhaustruct // only deload fields are exercised here
		DeloadEnabled:   true,
		MesocycleLength: 4,
		MesocycleAnchor: anchor,
	}
	if err := repos.Preferences.Set(ctx, prefs); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.DeloadEnabled {
		t.Error("DeloadEnabled = false, want true")
	}
	if got.MesocycleLength != 4 {
		t.Errorf("MesocycleLength = %d, want 4", got.MesocycleLength)
	}
	if !got.MesocycleAnchor.Equal(anchor) {
		t.Errorf("MesocycleAnchor = %s, want %s", got.MesocycleAnchor, anchor)
	}
}
