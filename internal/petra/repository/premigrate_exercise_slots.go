package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

// PreMigrateExerciseSlots renames the legacy workout_exercises table to
// exercise_slots before the declarative migrator runs. The migrator is
// structural only and would read the rename as a drop + create, losing data,
// so this native ALTER TABLE RENAME runs first. Modern SQLite (legacy_alter_table
// off, the default) auto-rewrites the foreign keys in the child tables
// (exercise_sets, scheduled_pushes) to the new name, and the migrator then
// reconciles the renamed index. RENAME does not revalidate foreign keys, so no
// PRAGMA foreign_keys = OFF is needed.
//
// It is idempotent and safe on a fresh database: if workout_exercises is
// absent (already renamed, or a brand-new database) it returns nil.
//
// Delete this function, its wiring in cmd/petra and cmd/migratetest, and its
// test once production has booted past the rename.
func PreMigrateExerciseSlots(ctx context.Context, db *sqlitekit.Database) error {
	// A transaction pins a single pooled connection for the check and the
	// rename, and rolls back automatically on any early return or error.
	tx, err := db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var name string
	err = tx.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'workout_exercises'`,
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // Already renamed, or a fresh database — nothing to do.
	}
	if err != nil {
		return fmt.Errorf("check for legacy workout_exercises table: %w", err)
	}

	if _, err = tx.ExecContext(ctx,
		`ALTER TABLE workout_exercises RENAME TO exercise_slots`,
	); err != nil {
		return fmt.Errorf("rename workout_exercises to exercise_slots: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit exercise_slots rename: %w", err)
	}
	return nil
}
