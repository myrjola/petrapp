package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

type sqliteMuscleGroupTargetRepository struct {
	baseRepository
}

func newSQLiteMuscleGroupTargetRepository(db *sqlitekit.Database) *sqliteMuscleGroupTargetRepository {
	return &sqliteMuscleGroupTargetRepository{baseRepository: newBaseRepository(db)}
}

// List returns all configured weekly volume range targets, ordered by muscle-group
// name. The targets table is seeded by migrations and is not user-editable.
func (r *sqliteMuscleGroupTargetRepository) List(ctx context.Context) (_ []domain.MuscleGroupTarget, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT muscle_group_name, min_sets, max_sets
		FROM muscle_group_weekly_targets
		ORDER BY muscle_group_name`)
	if err != nil {
		return nil, fmt.Errorf("query muscle group targets: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var targets []domain.MuscleGroupTarget
	for rows.Next() {
		var t domain.MuscleGroupTarget
		if err = rows.Scan(&t.MuscleGroupName, &t.MinSets, &t.MaxSets); err != nil {
			return nil, fmt.Errorf("scan muscle group target: %w", err)
		}
		targets = append(targets, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return targets, nil
}
