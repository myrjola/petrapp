package sqlite

import (
	"context"
	"fmt"
)

// preMigrateWorkoutPositionsStmts is the ordered rewrite sequence applied by
// preMigrateWorkoutPositions. Kept as a package-level value so the method body
// stays under the funlen limit.
//
//nolint:gochecknoglobals // immutable SQL script; pulling it inline blows the funlen budget.
var preMigrateWorkoutPositionsStmts = []string{
	`CREATE TABLE workout_exercises (
		workout_user_id     INTEGER NOT NULL,
		workout_date        TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
		position            INTEGER NOT NULL CHECK (position >= 0),
		exercise_id         INTEGER NOT NULL,
		warmup_completed_at TEXT CHECK (warmup_completed_at IS NULL OR
			STRFTIME('%Y-%m-%dT%H:%M:%fZ', warmup_completed_at) = warmup_completed_at),
		PRIMARY KEY (workout_user_id, workout_date, position),
		UNIQUE (workout_user_id, workout_date, exercise_id),
		FOREIGN KEY (workout_user_id, workout_date)
			REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
		FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
	) WITHOUT ROWID, STRICT`,

	`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id, warmup_completed_at)
	 SELECT workout_user_id, workout_date,
	        ROW_NUMBER() OVER (PARTITION BY workout_user_id, workout_date ORDER BY id) - 1,
	        exercise_id, warmup_completed_at
	 FROM workout_exercise`,

	`CREATE TABLE exercise_sets_new (
		workout_user_id INTEGER NOT NULL,
		workout_date    TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
		position        INTEGER NOT NULL,
		set_number      INTEGER NOT NULL CHECK (set_number > 0),
		weight_kg       REAL,
		target_value    INTEGER NOT NULL CHECK (target_value > 0),
		completed_value INTEGER CHECK (completed_value IS NULL OR completed_value >= 0),
		completed_at    TEXT CHECK (completed_at IS NULL OR
			STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
		signal          TEXT CHECK (signal IS NULL OR signal IN ('too_heavy', 'on_target', 'too_light')),
		PRIMARY KEY (workout_user_id, workout_date, position, set_number),
		FOREIGN KEY (workout_user_id, workout_date, position)
			REFERENCES workout_exercises (workout_user_id, workout_date, position) ON DELETE CASCADE
	) WITHOUT ROWID, STRICT`,

	`INSERT INTO exercise_sets_new (
		workout_user_id, workout_date, position, set_number,
		weight_kg, target_value, completed_value, completed_at, signal
	 )
	 SELECT new_we.workout_user_id, new_we.workout_date, new_we.position,
	        es.set_number, es.weight_kg, es.target_value, es.completed_value, es.completed_at, es.signal
	 FROM exercise_sets es
	 JOIN workout_exercise   old_we ON old_we.id = es.workout_exercise_id
	 JOIN workout_exercises  new_we
	      ON new_we.workout_user_id = old_we.workout_user_id
	     AND new_we.workout_date    = old_we.workout_date
	     AND new_we.exercise_id     = old_we.exercise_id`,

	`CREATE TABLE scheduled_pushes_new (
		id              INTEGER PRIMARY KEY,
		workout_user_id INTEGER NOT NULL,
		workout_date    TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
		position        INTEGER NOT NULL,
		fire_at         TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', fire_at) = fire_at),
		payload         TEXT    NOT NULL CHECK (LENGTH(payload) < 2048),
		created_at      TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
			CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at),
		FOREIGN KEY (workout_user_id, workout_date, position)
			REFERENCES workout_exercises (workout_user_id, workout_date, position) ON DELETE CASCADE
	) STRICT`,

	`INSERT INTO scheduled_pushes_new (
		id, workout_user_id, workout_date, position, fire_at, payload, created_at
	 )
	 SELECT sp.id,
	        new_we.workout_user_id, new_we.workout_date, new_we.position,
	        sp.fire_at, sp.payload, sp.created_at
	 FROM scheduled_pushes sp
	 JOIN workout_exercise  old_we ON old_we.id = sp.workout_exercise_id
	 JOIN workout_exercises new_we
	      ON new_we.workout_user_id = old_we.workout_user_id
	     AND new_we.workout_date    = old_we.workout_date
	     AND new_we.exercise_id     = old_we.exercise_id`,

	`DROP TABLE scheduled_pushes`,
	`DROP TABLE exercise_sets`,
	`DROP TABLE workout_exercise`,
	`ALTER TABLE exercise_sets_new    RENAME TO exercise_sets`,
	`ALTER TABLE scheduled_pushes_new RENAME TO scheduled_pushes`,

	`CREATE INDEX workout_exercises_user_exercise_date_idx
	    ON workout_exercises (workout_user_id, exercise_id, workout_date)`,
	`CREATE UNIQUE INDEX scheduled_pushes_slot_uidx
	    ON scheduled_pushes (workout_user_id, workout_date, position)`,
	`CREATE INDEX scheduled_pushes_fire_at ON scheduled_pushes (fire_at)`,
}

// preMigrateWorkoutPositions rewrites the legacy workout_exercise table
// (surrogate id PK) into workout_exercises (composite PK keyed by
// workout_user_id, workout_date, position), re-keying exercise_sets and
// scheduled_pushes onto the composite. Idempotent: returns nil immediately on
// fresh or already-migrated databases. Runs before migrateTo so the
// declarative migrator sees a database that matches schema.sql.
func (d *Database) preMigrateWorkoutPositions(ctx context.Context) error {
	// Detection: if the legacy singular table is gone, there is nothing to do.
	// This covers fresh DBs (no workout_exercise yet — migrateTo will create
	// workout_exercises directly from schema.sql) and already-migrated DBs
	// (the legacy table was dropped in a previous run).
	var present int
	if err := d.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('workout_exercise')`,
	).Scan(&present); err != nil {
		return fmt.Errorf("detect legacy workout_exercise: %w", err)
	}
	if present == 0 {
		return nil
	}

	if _, err := d.ReadWrite.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	tx, err := d.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin premigration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for i, stmt := range preMigrateWorkoutPositionsStmts {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("premigration stmt %d: %w", i, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit premigration: %w", err)
	}
	return nil
}
