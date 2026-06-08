package repository

import (
	"context"
	"fmt"

	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

// preMigrateMuscleTargetsStmts rebuilds muscle_group_weekly_targets from the
// pre-Phase-B shape (single weekly_sets_target column) into the floor/ceiling
// shape declared in schema.sql, preserving the existing value as min_sets and
// seeding max_sets = min_sets as a valid placeholder. fixtures.sql overwrites
// both columns with the authoritative per-group values on the same boot. The
// CREATE body MUST stay textually identical to schema.sql's
// muscle_group_weekly_targets definition so the declarative migrator sees no
// diff afterward.
//
//nolint:gochecknoglobals // immutable SQL script; inlining blows the funlen budget.
var preMigrateMuscleTargetsStmts = []string{
	`CREATE TABLE muscle_group_weekly_targets_new
(
    muscle_group_name   TEXT    PRIMARY KEY REFERENCES muscle_groups (name) ON DELETE CASCADE,
    min_sets            INTEGER NOT NULL CHECK (min_sets > 0),
    max_sets            INTEGER NOT NULL CHECK (max_sets >= min_sets)
) WITHOUT ROWID, STRICT`,
	`INSERT INTO muscle_group_weekly_targets_new (muscle_group_name, min_sets, max_sets)
	 SELECT muscle_group_name, weekly_sets_target, weekly_sets_target
	 FROM muscle_group_weekly_targets`,
	`DROP TABLE muscle_group_weekly_targets`,
	`ALTER TABLE muscle_group_weekly_targets_new RENAME TO muscle_group_weekly_targets`,
}

// PreMigrateMuscleTargets rewrites the legacy muscle_group_weekly_targets table
// into the floor/ceiling shape before the declarative migrate runs (the
// migrator copies only same-named columns, so it cannot rename
// weekly_sets_target or populate the new NOT NULL columns and would crash on a
// populated table). It is idempotent: it short-circuits when the legacy column
// is absent — true both on a fresh boot (the table does not exist yet) and on
// an already-migrated database. Wire it into sqlitekit.Config.Premigration.
//
// Delete this function, its wiring in cmd/petra and cmd/migratetest, and its
// test once production has booted past it (there is no version table; the only
// signal it is no longer needed is that prod is on the new schema).
func PreMigrateMuscleTargets(ctx context.Context, db *sqlitekit.Database) (err error) {
	var legacyCols int
	if err = db.ReadWrite.QueryRowContext(ctx,
		"SELECT count(*) FROM pragma_table_info('muscle_group_weekly_targets') WHERE name = 'weekly_sets_target'").
		Scan(&legacyCols); err != nil {
		return fmt.Errorf("detect legacy targets column: %w", err)
	}
	if legacyCols == 0 {
		return nil // fresh or already migrated.
	}

	// PRAGMA foreign_keys cannot be toggled inside a transaction; set it on the
	// single ReadWrite connection first. migrateTo re-enables FKs in its defer.
	if _, err = db.ReadWrite.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	tx, err := db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin premigration tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				err = fmt.Errorf("%w; rollback: %w", err, rbErr)
			}
		}
	}()

	for _, stmt := range preMigrateMuscleTargetsStmts {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("premigrate muscle targets (%.40s…): %w", stmt, err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit premigration: %w", err)
	}
	return nil
}
