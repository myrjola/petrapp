package workout

import (
	"context"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteMuscleGroupTargetRepository struct {
	baseRepository
}

func newSQLiteMuscleGroupTargetRepository(db *sqlite.Database) *sqliteMuscleGroupTargetRepository {
	return &sqliteMuscleGroupTargetRepository{baseRepository: newBaseRepository(db)}
}

func (r *sqliteMuscleGroupTargetRepository) List(ctx context.Context) (_ []MuscleGroupTarget, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT muscle_group_name, weekly_sets_target
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

	var targets []MuscleGroupTarget
	for rows.Next() {
		var t MuscleGroupTarget
		if err = rows.Scan(&t.MuscleGroupName, &t.WeeklySetTarget); err != nil {
			return nil, fmt.Errorf("scan muscle group target: %w", err)
		}
		targets = append(targets, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return targets, nil
}
