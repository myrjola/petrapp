package repository_test

import (
	"errors"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestFeatureFlagRepository_GetMissingReturnsErrNotFound(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	_, err := repos.FeatureFlags.Get(ctx, "nonexistent_flag")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound, got %v", err)
	}
}

func TestFeatureFlagRepository_SetThenGetRoundTrip(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	want := domain.FeatureFlag{Name: "experimental_x", Enabled: true}
	if err := repos.FeatureFlags.Set(ctx, want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := repos.FeatureFlags.Get(ctx, "experimental_x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("round-trip: want %+v, got %+v", want, got)
	}
}

func TestFeatureFlagRepository_SetUpsertsExisting(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	if err := repos.FeatureFlags.Set(ctx, domain.FeatureFlag{Name: "x", Enabled: true}); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	if err := repos.FeatureFlags.Set(ctx, domain.FeatureFlag{Name: "x", Enabled: false}); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	got, err := repos.FeatureFlags.Get(ctx, "x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Enabled {
		t.Errorf("expected upsert to disable flag, got Enabled=true")
	}
}

func TestFeatureFlagRepository_ListSortedByName(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	for _, name := range []string{"zebra", "apple", "mango"} {
		if err := repos.FeatureFlags.Set(ctx, domain.FeatureFlag{Name: name, Enabled: true}); err != nil {
			t.Fatalf("Set %s: %v", name, err)
		}
	}
	got, err := repos.FeatureFlags.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Extract only the names we inserted, verifying they appear in sorted order
	// among all returned flags (the DB may contain fixture flags as well).
	inserted := map[string]bool{"apple": true, "mango": true, "zebra": true}
	var filtered []string
	for _, f := range got {
		if inserted[f.Name] {
			filtered = append(filtered, f.Name)
		}
	}
	want := []string{"apple", "mango", "zebra"}
	if len(filtered) != len(want) {
		t.Fatalf("List: inserted flags found %v, want %v", filtered, want)
	}
	for i, w := range want {
		if filtered[i] != w {
			t.Errorf("List order[%d]: want %q, got %q", i, w, filtered[i])
		}
	}
}
