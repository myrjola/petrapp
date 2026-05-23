package repository_test

import (
	"errors"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestFeatureFlagRepository_GetMissingReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	_, err := repos.FeatureFlags.Get(ctx, domain.FeatureFlagName("nonexistent_flag"))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound, got %v", err)
	}
}

func TestFeatureFlagRepository_SetThenGetRoundTrip(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	want := domain.FeatureFlag{Name: domain.FeatureFlagName("experimental_x"), Enabled: true}
	if err := repos.FeatureFlags.Set(ctx, want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := repos.FeatureFlags.Get(ctx, domain.FeatureFlagName("experimental_x"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("round-trip: want %+v, got %+v", want, got)
	}
}

func TestFeatureFlagRepository_SetUpsertsExisting(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	flagX := domain.FeatureFlag{Name: domain.FeatureFlagName("x"), Enabled: true}
	if err := repos.FeatureFlags.Set(ctx, flagX); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	flagX.Enabled = false
	if err := repos.FeatureFlags.Set(ctx, flagX); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	got, err := repos.FeatureFlags.Get(ctx, domain.FeatureFlagName("x"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Enabled {
		t.Errorf("expected upsert to disable flag, got Enabled=true")
	}
}

func TestFeatureFlagRepository_ListSortedByName(t *testing.T) {
	t.Parallel()

	ctx, repos := setupTestRepos(t)

	for _, name := range []string{"zebra", "apple", "mango"} {
		flag := domain.FeatureFlag{Name: domain.FeatureFlagName(name), Enabled: true}
		if err := repos.FeatureFlags.Set(ctx, flag); err != nil {
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
		if inserted[string(f.Name)] {
			filtered = append(filtered, string(f.Name))
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
