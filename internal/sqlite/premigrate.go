package sqlite

import (
	"context"
	"fmt"
	"log/slog"
)

// preMigrateRepWindows adds the rep_min / rep_max columns to the exercises
// table (if missing) and backfills sensible values for every existing row.
//
// The declarative migrator at migrate.go:184 cannot do this on its own: it
// rebuilds the table by INSERT ... SELECT, which would put NULL into the new
// columns and trip the table-level CHECK that requires non-NULL rep_min /
// rep_max for non-time-based exercises.
//
// Idempotent: short-circuits if the columns already exist. Safe to run on
// every boot — also safe on a fresh database where the exercises table
// doesn't yet exist (returns early).
//
// Once production has booted past this premigration, this file, the call
// site in NewDatabase, and the matching test in migrate_internal_test.go
// should be deleted in a follow-up commit.
func (db *Database) preMigrateRepWindows(ctx context.Context) error {
	// Short-circuit if the exercises table doesn't exist (fresh DB; the
	// declarative migrator will create it from schema.sql with the new
	// columns already present).
	var exercisesExists int
	if err := db.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'exercises'`).
		Scan(&exercisesExists); err != nil {
		return fmt.Errorf("check exercises table existence: %w", err)
	}
	if exercisesExists == 0 {
		return nil
	}

	// Short-circuit if the columns already exist (we've run before).
	var hasRepMin int
	if err := db.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('exercises') WHERE name = 'rep_min'`).
		Scan(&hasRepMin); err != nil {
		return fmt.Errorf("check rep_min existence: %w", err)
	}
	if hasRepMin > 0 {
		return nil
	}

	db.logger.LogAttrs(ctx, slog.LevelInfo, "premigrating rep_min/rep_max")

	// FK off (consistent with CLAUDE.md guidance, even though no FK changes
	// here — keeps the boundary safe and matches existing premigration patterns).
	if _, err := db.ReadWrite.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	tx, err := db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Add the columns (no CHECK at this stage — ALTER TABLE ADD COLUMN
	// can't carry table-level CHECKs anyway; the declarative migrator
	// rebuilds the table afterwards and applies the constraints from
	// schema.sql).
	if _, err = tx.ExecContext(ctx,
		`ALTER TABLE exercises ADD COLUMN rep_min INTEGER`); err != nil {
		return fmt.Errorf("add rep_min column: %w", err)
	}
	if _, err = tx.ExecContext(ctx,
		`ALTER TABLE exercises ADD COLUMN rep_max INTEGER`); err != nil {
		return fmt.Errorf("add rep_max column: %w", err)
	}

	// Backfill known production exercises with their family-correct windows.
	// IDs not listed here (any future exercise added between this PR and the
	// next premigration deploy) get the catch-all (5, 10) default below.
	type backfillRow struct {
		id     int
		repMin int
		repMax int
	}
	knownRows := []backfillRow{
		{1, 3, 6},    // Deadlift
		{2, 5, 10},   // Bench Press
		{3, 8, 12},   // Tricep Pushdown
		{4, 8, 12},   // Dumbbell Biceps Curl
		{5, 10, 20},  // Lateral Raise
		{6, 5, 10},   // Dumbbell Shoulder Press
		{7, 5, 10},   // Dumbbell Bench Press
		{8, 8, 12},   // Cable Fly
		{9, 5, 10},   // Pulldown
		{10, 5, 10},  // Pulldown, Reverse Grip
		{11, 5, 10},  // Seated Cable Row
		{12, 5, 10},  // One-Arm Dumbell Row
		{13, 8, 15},  // Abdominal Machine Crunch
		{14, 5, 10},  // Leg Press
		{15, 8, 12},  // Leg Extension
		{16, 8, 12},  // Leg Curl
		{17, 10, 20}, // Calf Raise
		{18, 8, 20},  // Back Extension
		{19, 5, 10},  // Push-up
		{20, 8, 15},  // Ab Wheel Rollout
		// 21 = Plank — time_based, leave NULL
		{22, 5, 10},  // Incline Dumbbell Bench Press
		{23, 8, 20},  // Romanian Deadlift
		{24, 5, 10},  // Assisted Pull-Up
		{25, 8, 12},  // Hip Abductor
		{26, 8, 12},  // Hip Adductor
		{27, 8, 15},  // Rotary Torso
		{28, 10, 20}, // Seated Calf Raise
		{29, 3, 6},   // Squat
		{30, 8, 12},  // Pec Fly
		{31, 3, 6},   // Smith Machine Squat
	}
	for _, row := range knownRows {
		if _, err = tx.ExecContext(ctx,
			`UPDATE exercises SET rep_min = ?, rep_max = ? WHERE id = ?`,
			row.repMin, row.repMax, row.id); err != nil {
			return fmt.Errorf("backfill exercise %d: %w", row.id, err)
		}
	}

	// Catch-all: any non-time-based row not covered above gets sensible defaults.
	// This protects against future exercises added between this PR and the
	// next premigration deploy.
	if _, err = tx.ExecContext(ctx, `
		UPDATE exercises
		SET rep_min = 5, rep_max = 10
		WHERE rep_min IS NULL
		  AND exercise_type <> 'time_based'`); err != nil {
		return fmt.Errorf("backfill catch-all: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}
