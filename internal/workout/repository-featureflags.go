package workout

import (
	"context"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/sqlite"
)

// sqliteFeatureFlagRepository implements featureFlagRepository.
type sqliteFeatureFlagRepository struct {
	baseRepository
}

// newSQLiteFeatureFlagRepository creates a new SQLite feature flag repository.
func newSQLiteFeatureFlagRepository(db *sqlite.Database) *sqliteFeatureFlagRepository {
	return &sqliteFeatureFlagRepository{
		baseRepository: newBaseRepository(db),
	}
}

// Get retrieves a feature flag by name.
func (r *sqliteFeatureFlagRepository) Get(ctx context.Context, name string) (FeatureFlag, error) {
	var flag FeatureFlag
	var enabled int

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT name, enabled
		FROM feature_flags 
		WHERE name = ?`, name).Scan(&flag.Name, &enabled)

	if errors.Is(err, ErrNotFound) {
		return FeatureFlag{}, ErrNotFound
	}

	if err != nil {
		return FeatureFlag{}, fmt.Errorf("query feature flag %s: %w", name, err)
	}

	flag.Enabled = enabled == 1
	return flag, nil
}

// Set updates or creates a feature flag.
func (r *sqliteFeatureFlagRepository) Set(ctx context.Context, flag FeatureFlag) error {
	enabled := 0
	if flag.Enabled {
		enabled = 1
	}

	_, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO feature_flags (name, enabled) 
		VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET enabled = excluded.enabled`,
		flag.Name, enabled)

	if err != nil {
		return fmt.Errorf("save feature flag %s: %w", flag.Name, err)
	}

	return nil
}

// List retrieves all feature flags.
func (r *sqliteFeatureFlagRepository) List(ctx context.Context) ([]FeatureFlag, error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT name, enabled
		FROM feature_flags
		ORDER BY name`)

	if err != nil {
		return nil, fmt.Errorf("query feature flags: %w", err)
	}
	defer rows.Close()

	var flags []FeatureFlag
	for rows.Next() {
		var flag FeatureFlag
		var enabled int

		scanErr := rows.Scan(&flag.Name, &enabled)
		if scanErr != nil {
			return nil, fmt.Errorf("scan feature flag: %w", scanErr)
		}

		flag.Enabled = enabled == 1
		flags = append(flags, flag)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feature flags: %w", err)
	}

	return flags, nil
}
