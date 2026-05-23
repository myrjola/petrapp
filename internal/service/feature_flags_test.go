package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// newCachedFeatureFlagService returns a Service backed by a fresh in-memory
// database with maintenance caching enabled. Unlike setupTestService it
// doesn't seed preferences or a user — the feature-flag path doesn't need
// either and seeding only adds noise to assertions.
func newCachedFeatureFlagService(
	t *testing.T,
	ttl time.Duration,
) (context.Context, *sqlite.Database, *service.Service) {
	t.Helper()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := service.NewService(db, logger, "").WithMaintenanceCacheTTL(ttl)
	return ctx, db, svc
}

// Test_Service_IsMaintenanceModeEnabled_cachesAcrossReads pins the production
// fast path: once the flag has been loaded, subsequent calls are answered
// from the cache and the database row can change without the service noticing
// until the entry expires. We assert that by mutating feature_flags via raw
// SQL (bypassing SetFeatureFlag's invalidate hook) and checking the cached
// answer stays the same.
func Test_Service_IsMaintenanceModeEnabled_cachesAcrossReads(t *testing.T) {
	t.Parallel()

	ctx, db, svc := newCachedFeatureFlagService(t, time.Minute)

	if svc.IsMaintenanceModeEnabled(ctx) {
		t.Fatalf("expected maintenance disabled on a fresh DB, got enabled")
	}

	if _, err := db.ReadWrite.ExecContext(ctx,
		"UPDATE feature_flags SET enabled = 1 WHERE name = 'maintenance_mode'"); err != nil {
		t.Fatalf("enable maintenance via raw SQL: %v", err)
	}

	// Cache still holds the previous "disabled" answer because the write
	// went around SetFeatureFlag.
	if svc.IsMaintenanceModeEnabled(ctx) {
		t.Errorf("expected stale cached answer (disabled) immediately after raw SQL write")
	}
}

// Test_Service_SetFeatureFlag_invalidatesMaintenanceCache pins the inverse
// guarantee: writes that go through SetFeatureFlag drop the cache entry so
// the very next read observes the new value.
func Test_Service_SetFeatureFlag_invalidatesMaintenanceCache(t *testing.T) {
	t.Parallel()

	ctx, _, svc := newCachedFeatureFlagService(t, time.Minute)

	// Prime the cache with "disabled".
	if svc.IsMaintenanceModeEnabled(ctx) {
		t.Fatalf("expected maintenance disabled on a fresh DB")
	}

	maintenanceFlag := domain.FeatureFlag{Name: domain.FeatureFlagMaintenanceMode, Enabled: true}
	if err := svc.SetFeatureFlag(ctx, maintenanceFlag); err != nil {
		t.Fatalf("SetFeatureFlag: %v", err)
	}

	if !svc.IsMaintenanceModeEnabled(ctx) {
		t.Errorf("expected fresh DB read after SetFeatureFlag invalidated the cache")
	}
}

// Test_Service_IsMaintenanceModeEnabled_ttlExpiry pins that the cached entry
// is dropped once its TTL has elapsed, so an out-of-band toggle eventually
// propagates without manual invalidation.
func Test_Service_IsMaintenanceModeEnabled_ttlExpiry(t *testing.T) {
	t.Parallel()

	ctx, db, svc := newCachedFeatureFlagService(t, 10*time.Millisecond)

	if svc.IsMaintenanceModeEnabled(ctx) {
		t.Fatalf("expected maintenance disabled on a fresh DB")
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		"UPDATE feature_flags SET enabled = 1 WHERE name = 'maintenance_mode'"); err != nil {
		t.Fatalf("enable maintenance via raw SQL: %v", err)
	}

	// Wait past the TTL and the next read must re-hit the database.
	time.Sleep(50 * time.Millisecond)
	if !svc.IsMaintenanceModeEnabled(ctx) {
		t.Errorf("expected re-read after TTL expiry to observe enabled maintenance")
	}
}

// Test_Service_IsMaintenanceModeEnabled_zeroTTLDisablesCache pins the
// default-construction behaviour: without WithMaintenanceCacheTTL, every read
// goes straight to the database. This is what the e2e tests rely on so they
// can flip the flag via raw SQL without an invalidation hook.
func Test_Service_IsMaintenanceModeEnabled_zeroTTLDisablesCache(t *testing.T) {
	t.Parallel()

	ctx, db, svc := newCachedFeatureFlagService(t, 0)

	if svc.IsMaintenanceModeEnabled(ctx) {
		t.Fatalf("expected maintenance disabled on a fresh DB")
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		"UPDATE feature_flags SET enabled = 1 WHERE name = 'maintenance_mode'"); err != nil {
		t.Fatalf("enable maintenance via raw SQL: %v", err)
	}
	if !svc.IsMaintenanceModeEnabled(ctx) {
		t.Errorf("expected raw-SQL write to be observed immediately with caching off")
	}
}
