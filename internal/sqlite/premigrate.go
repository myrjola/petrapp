package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
)

// preMigrateWorkoutExerciseStableID rewrites legacy workout_exercise / exercise_sets
// tables (composite PK, exercise_id-keyed sets) into the new shape with a surrogate
// workout_exercise.id and exercise_sets.workout_exercise_id FK before the declarative
// migrator runs. The declarative migrator cannot infer how to populate the new id
// column or rewrite the FK, so it would rebuild the tables empty.
//
// The function is idempotent: it detects the new shape via pragma_table_info and
// returns immediately on a fresh database or one that has already been migrated.
//
// Once production has run this migration, the function and its call site can be removed.
func (db *Database) preMigrateWorkoutExerciseStableID(ctx context.Context) (err error) {
	migrated, err := db.workoutExerciseStableIDAlreadyMigrated(ctx)
	if err != nil {
		return fmt.Errorf("check migration state: %w", err)
	}
	if migrated {
		return nil
	}

	db.logger.LogAttrs(ctx, slog.LevelInfo, "pre-migrating workout_exercise to stable IDs")

	// Foreign keys must be off for the table swap; the declarative migrator
	// re-enables them in its own defer.
	if _, err = db.ReadWrite.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	tx, err := db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback: %w", rollbackErr))
		}
	}()

	for _, stmt := range preMigrateWorkoutExerciseStatements {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// workoutExerciseStableIDAlreadyMigrated returns true when either the database is
// fresh (no exercise_sets table yet) or the table already has the new
// workout_exercise_id column.
func (db *Database) workoutExerciseStableIDAlreadyMigrated(ctx context.Context) (bool, error) {
	var hasColumn int
	err := db.ReadWrite.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pragma_table_info('exercise_sets')
		WHERE name = 'workout_exercise_id'`).Scan(&hasColumn)
	if err != nil {
		return false, fmt.Errorf("query pragma_table_info: %w", err)
	}
	if hasColumn > 0 {
		return true, nil
	}

	var tableCount int
	err = db.ReadWrite.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'exercise_sets'`).Scan(&tableCount)
	if err != nil {
		return false, fmt.Errorf("query sqlite_master: %w", err)
	}
	// Fresh database: nothing to migrate, declarative migrator will create the new shape.
	return tableCount == 0, nil
}

// preMigrateWorkoutExerciseStatements rewrites the legacy tables to the new shape.
// The old workout_exercise rows only existed when warmup was completed, so we
// UNION in (user, date, exercise_id) tuples found in exercise_sets to ensure
// every set has a matching workout_exercise row before re-keying the FK.
//
//nolint:gochecknoglobals // statements are constant migration data.
var preMigrateWorkoutExerciseStatements = []string{
	`CREATE TABLE workout_exercise_new
	(
		id                  INTEGER PRIMARY KEY,
		workout_user_id     INTEGER NOT NULL,
		workout_date        TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
		exercise_id         INTEGER NOT NULL,
		warmup_completed_at TEXT CHECK (warmup_completed_at IS NULL OR
		                                STRFTIME('%Y-%m-%dT%H:%M:%fZ', warmup_completed_at) = warmup_completed_at),

		UNIQUE (workout_user_id, workout_date, exercise_id),
		FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
		FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
	) STRICT`,

	`INSERT INTO workout_exercise_new (workout_user_id, workout_date, exercise_id, warmup_completed_at)
	 SELECT workout_user_id, workout_date, exercise_id, warmup_completed_at FROM workout_exercise
	 UNION
	 SELECT DISTINCT es.workout_user_id, es.workout_date, es.exercise_id, NULL
	 FROM exercise_sets es
	 WHERE NOT EXISTS (
	     SELECT 1 FROM workout_exercise we
	     WHERE we.workout_user_id = es.workout_user_id
	       AND we.workout_date    = es.workout_date
	       AND we.exercise_id     = es.exercise_id
	 )`,

	`CREATE TABLE exercise_sets_new
	(
		workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise_new (id) ON DELETE CASCADE,
		set_number          INTEGER NOT NULL CHECK (set_number > 0),
		weight_kg           REAL CHECK (weight_kg IS NULL OR weight_kg >= 0),
		min_reps            INTEGER NOT NULL CHECK (min_reps > 0),
		max_reps            INTEGER NOT NULL CHECK (max_reps >= min_reps),
		completed_reps      INTEGER CHECK (completed_reps IS NULL OR completed_reps >= 0),
		completed_at        TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
		signal              TEXT CHECK (signal IS NULL OR signal IN ('too_heavy', 'on_target', 'too_light')),

		PRIMARY KEY (workout_exercise_id, set_number)
	) WITHOUT ROWID, STRICT`,

	`INSERT INTO exercise_sets_new (workout_exercise_id, set_number, weight_kg, min_reps,
	                                 max_reps, completed_reps, completed_at, signal)
	 SELECT we.id, es.set_number, es.weight_kg, es.min_reps, es.max_reps,
	        es.completed_reps, es.completed_at, es.signal
	 FROM exercise_sets es
	 JOIN workout_exercise_new we
	   ON we.workout_user_id = es.workout_user_id
	  AND we.workout_date    = es.workout_date
	  AND we.exercise_id     = es.exercise_id`,

	`DROP TABLE exercise_sets`,
	`DROP TABLE workout_exercise`,
	`ALTER TABLE workout_exercise_new RENAME TO workout_exercise`,
	`ALTER TABLE exercise_sets_new RENAME TO exercise_sets`,
}
