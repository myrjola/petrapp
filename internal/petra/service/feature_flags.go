package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

// maintenanceCache memoises the maintenance_mode feature flag. Every HTTP
// request consults this via Service.IsMaintenanceModeEnabled in middleware;
// before caching, that was one DB read per request and showed up as the
// largest single share of read-pool wait time under load.
//
// A zero ttl disables the cache (every read goes straight to the database).
// Tests that mutate feature_flags via raw SQL leave the TTL at zero so they
// observe writes immediately. Production wires it to a few seconds via
// Service.WithMaintenanceCacheTTL — long enough to absorb steady-state
// request volume, short enough that an admin toggle propagates without
// requiring a manual cache flush. Service.SetFeatureFlag invalidates the
// entry eagerly anyway, so the TTL is just an upper bound on staleness for
// writes that arrive through some other path (which today doesn't exist).
type maintenanceCache struct {
	state atomic.Pointer[maintenanceState]
	ttl   time.Duration
}

type maintenanceState struct {
	enabled bool
	expires time.Time
}

func newMaintenanceCache() *maintenanceCache {
	return &maintenanceCache{state: atomic.Pointer[maintenanceState]{}, ttl: 0}
}

// load returns the cached value if caching is enabled and the entry has not
// expired. ok=false means the caller must re-read from the database.
//
// Safety: the atomic.Pointer.Load gives a self-consistent snapshot — the
// captured *maintenanceState cannot mutate. A concurrent store or
// invalidate that runs between Load and the time check is irrelevant: this
// caller is allowed to use a snapshot up to ttl old, by definition of the
// cache.
func (c *maintenanceCache) load() (bool, bool) {
	if c.ttl <= 0 {
		return false, false
	}
	s := c.state.Load()
	if s == nil || time.Now().After(s.expires) {
		return false, false
	}
	return s.enabled, true
}

func (c *maintenanceCache) store(enabled bool) {
	if c.ttl <= 0 {
		return
	}
	c.state.Store(&maintenanceState{enabled: enabled, expires: time.Now().Add(c.ttl)})
}

func (c *maintenanceCache) invalidate() {
	c.state.Store(nil)
}

// WithMaintenanceCacheTTL returns a copy of the service wired to memoise the
// maintenance_mode flag for the given duration. Production passes a few
// seconds; tests leave it unset (zero) so raw-SQL mutations are observed
// immediately.
func (s *Service) WithMaintenanceCacheTTL(ttl time.Duration) *Service {
	cp := *s
	cp.maintenanceCache = &maintenanceCache{state: atomic.Pointer[maintenanceState]{}, ttl: ttl}
	return &cp
}

// GetFeatureFlag retrieves a feature flag by name.
func (s *Service) GetFeatureFlag(ctx context.Context, name domain.FeatureFlagName) (domain.FeatureFlag, error) {
	flag, err := s.repos.FeatureFlags.Get(ctx, name)
	if err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("get feature flag %s: %w", name, err)
	}
	return flag, nil
}

// IsMaintenanceModeEnabled checks if maintenance mode is enabled. Cached in
// process when the maintenance cache TTL is positive; see maintenanceCache.
func (s *Service) IsMaintenanceModeEnabled(ctx context.Context) bool {
	if enabled, ok := s.maintenanceCache.load(); ok {
		return enabled
	}
	flag, err := s.repos.FeatureFlags.Get(ctx, domain.FeatureFlagMaintenanceMode)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Missing row is the steady state — cache it as "disabled" so we
			// don't keep paying for a no-rows lookup on every request.
			s.maintenanceCache.store(false)
			return false
		}
		// If we can't check the flag, assume maintenance is disabled for safety.
		// Don't poison the cache with a guess — leave it empty so the next
		// request retries the read.
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to check maintenance mode flag", slog.Any("error", err))
		return false
	}
	s.maintenanceCache.store(flag.Enabled)
	return flag.Enabled
}

// ListFeatureFlags retrieves all feature flags.
func (s *Service) ListFeatureFlags(ctx context.Context) ([]domain.FeatureFlag, error) {
	flags, err := s.repos.FeatureFlags.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feature flags: %w", err)
	}
	return flags, nil
}

// SetFeatureFlag updates or creates a feature flag.
func (s *Service) SetFeatureFlag(ctx context.Context, flag domain.FeatureFlag) error {
	if err := s.repos.FeatureFlags.Set(ctx, flag); err != nil {
		return fmt.Errorf("set feature flag %s: %w", flag.Name, err)
	}
	if flag.Name == domain.FeatureFlagMaintenanceMode {
		s.maintenanceCache.invalidate()
	}
	return nil
}
