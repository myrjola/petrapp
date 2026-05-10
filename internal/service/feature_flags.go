package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/myrjola/petrapp/internal/domain"
)

// GetFeatureFlag retrieves a feature flag by name.
func (s *Service) GetFeatureFlag(ctx context.Context, name string) (domain.FeatureFlag, error) {
	flag, err := s.repos.FeatureFlags.Get(ctx, name)
	if err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("get feature flag %s: %w", name, err)
	}
	return flag, nil
}

// IsMaintenanceModeEnabled checks if maintenance mode is enabled.
func (s *Service) IsMaintenanceModeEnabled(ctx context.Context) bool {
	flag, err := s.repos.FeatureFlags.Get(ctx, "maintenance_mode")
	if err != nil {
		// If we can't check the flag, assume maintenance is disabled for safety.
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to check maintenance mode flag", slog.Any("error", err))
		return false
	}
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
	return nil
}
