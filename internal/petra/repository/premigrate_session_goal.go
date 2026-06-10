package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

// PreMigrateSessionGoal renames the legacy workout_sessions.periodization_type
// column to session_goal before the declarative migrator runs. The migrator is
// structural only and would read the rename as a drop + add, losing data, so
// this native ALTER TABLE RENAME COLUMN runs first. Modern SQLite rewrites the
// column's CHECK constraint reference along with the rename, and RENAME does
// not revalidate foreign keys, so no PRAGMA foreign_keys = OFF is needed.
//
// It is idempotent and safe on a fresh database: if periodization_type is
// absent (already renamed, or a brand-new database) it returns nil.
//
// Delete this function, its wiring in cmd/petra and cmd/migratetest, and its
// test once production has booted past the rename.
func PreMigrateSessionGoal(ctx context.Context, db *sqlitekit.Database) error {
	// A transaction pins a single pooled connection for the check and the
	// rename, and rolls back automatically on any early return or error.
	tx, err := db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var name string
	err = tx.QueryRowContext(ctx,
		`SELECT name FROM pragma_table_info('workout_sessions') WHERE name = 'periodization_type'`,
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // Already renamed, or a fresh database — nothing to do.
	}
	if err != nil {
		return fmt.Errorf("check for legacy periodization_type column: %w", err)
	}

	if _, err = tx.ExecContext(ctx,
		`ALTER TABLE workout_sessions RENAME COLUMN periodization_type TO session_goal`,
	); err != nil {
		return fmt.Errorf("rename periodization_type to session_goal: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit session_goal rename: %w", err)
	}
	return nil
}
