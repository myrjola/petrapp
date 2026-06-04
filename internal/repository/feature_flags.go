package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

type sqliteFeatureFlagRepository struct {
	baseRepository
}

func newSQLiteFeatureFlagRepository(db *sqlitekit.Database) *sqliteFeatureFlagRepository {
	return &sqliteFeatureFlagRepository{baseRepository: newBaseRepository(db)}
}

func (r *sqliteFeatureFlagRepository) Get(
	ctx context.Context, name domain.FeatureFlagName,
) (domain.FeatureFlag, error) {
	var flag domain.FeatureFlag
	var enabled int
	var nameStr string

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT name, enabled
		FROM feature_flags
		WHERE name = ?`, string(name)).Scan(&nameStr, &enabled)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.FeatureFlag{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("query feature flag %s: %w", name, err)
	}

	flag.Name = domain.FeatureFlagName(nameStr)
	flag.Enabled = enabled == 1
	return flag, nil
}

func (r *sqliteFeatureFlagRepository) Set(ctx context.Context, flag domain.FeatureFlag) error {
	enabled := 0
	if flag.Enabled {
		enabled = 1
	}

	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO feature_flags (name, enabled)
		VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET enabled = excluded.enabled`,
		string(flag.Name), enabled); err != nil {
		return fmt.Errorf("save feature flag %s: %w", flag.Name, err)
	}
	return nil
}

func (r *sqliteFeatureFlagRepository) List(ctx context.Context) (_ []domain.FeatureFlag, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT name, enabled
		FROM feature_flags
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query feature flags: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var flags []domain.FeatureFlag
	for rows.Next() {
		var flag domain.FeatureFlag
		var enabled int
		var nameStr string
		if scanErr := rows.Scan(&nameStr, &enabled); scanErr != nil {
			return nil, fmt.Errorf("scan feature flag: %w", scanErr)
		}
		flag.Name = domain.FeatureFlagName(nameStr)
		flag.Enabled = enabled == 1
		flags = append(flags, flag)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feature flags: %w", err)
	}
	return flags, nil
}
