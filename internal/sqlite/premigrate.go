package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// preMigrateExerciseSetTarget rewrites the legacy exercise_sets shape
// (min_reps/max_reps/completed_reps) into the collapsed target_value/
// completed_value shape, dropping historical Plank rows whose reps would be
// nonsensical to reinterpret as seconds. Idempotent and safe on a fresh DB.
//
// Delete this file, its call site in NewDatabase, the test in
// migrate_internal_test.go, and the legacy schema constant once it has run
// in production.
func (db *Database) preMigrateExerciseSetTarget(ctx context.Context) error {
	// Already migrated? (target_value column exists)
	var hasTarget int
	err := db.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('exercise_sets') WHERE name = 'target_value'`,
	).Scan(&hasTarget)
	if err != nil {
		return fmt.Errorf("pragma_table_info exercise_sets: %w", err)
	}
	if hasTarget > 0 {
		return nil
	}

	// Fresh DB? (no exercise_sets table at all)
	var hasTable int
	err = db.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='exercise_sets'`,
	).Scan(&hasTable)
	if err != nil {
		return fmt.Errorf("sqlite_master exercise_sets: %w", err)
	}
	if hasTable == 0 {
		return nil
	}

	if _, err = db.ReadWrite.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	tx, err := db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = fmt.Errorf("%w; rollback: %w", err, rbErr)
			}
		}
	}()

	stmts := []string{
		`CREATE TABLE exercise_sets_new (
			workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
			set_number          INTEGER NOT NULL CHECK (set_number > 0),
			weight_kg           REAL,
			target_value        INTEGER NOT NULL CHECK (target_value > 0),
			completed_value     INTEGER CHECK (completed_value IS NULL OR completed_value >= 0),
			completed_at        TEXT CHECK (completed_at IS NULL OR
			                                STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
			signal              TEXT CHECK (signal IS NULL OR signal IN ('too_heavy', 'on_target', 'too_light')),
			PRIMARY KEY (workout_exercise_id, set_number)
		) WITHOUT ROWID, STRICT`,
		`INSERT INTO exercise_sets_new
			(workout_exercise_id, set_number, weight_kg, target_value,
			 completed_value, completed_at, signal)
		 SELECT s.workout_exercise_id, s.set_number, s.weight_kg,
		        s.max_reps, s.completed_reps, s.completed_at, s.signal
		   FROM exercise_sets s
		   JOIN workout_exercise wx ON wx.id = s.workout_exercise_id
		   JOIN exercises e         ON e.id = wx.exercise_id
		  WHERE e.name <> 'Plank'`,
		`DROP TABLE exercise_sets`,
		`ALTER TABLE exercise_sets_new RENAME TO exercise_sets`,
	}
	for _, q := range stmts {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("premigrate exercise_sets: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
