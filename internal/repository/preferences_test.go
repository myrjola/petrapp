package repository_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestPreferencesRepository_GetEmptyReturnsZeroValue(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get on empty: %v", err)
	}
	want := domain.Preferences{ //nolint:exhaustruct // Weekday minutes still zero by design.
		RestNotificationsEnabled: true,
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
	ctx, repos := setupTestRepos(t)

	want := domain.Preferences{ //nolint:exhaustruct // Untouched days stay zero.
		MondayMinutes:    60,
		WednesdayMinutes: 45,
		FridayMinutes:    90,
	}
	if err := repos.Preferences.Set(ctx, want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("round-trip: want %+v, got %+v", want, got)
	}
}

func TestPreferencesRepository_SetUpdatesExisting(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	if err := repos.Preferences.Set(ctx, domain.Preferences{ //nolint:exhaustruct // First write.
		MondayMinutes: 45,
	}); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	updated := domain.Preferences{ //nolint:exhaustruct // Second write — Monday changes, others stay zero.
		MondayMinutes:  90,
		TuesdayMinutes: 45,
	}
	if err := repos.Preferences.Set(ctx, updated); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != updated {
		t.Errorf("after upsert: want %+v, got %+v", updated, got)
	}
}
