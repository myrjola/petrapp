package repository_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/myrjola/petrapp/internal/petra/repository"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

// legacyTargetsSchema reproduces the pre-Phase-B shape of the only tables the
// premigration touches (schema.sql no longer contains the weekly_sets_target
// column). Kept here so the test can build a database in the pre-migration
// state via the public NewDatabase API.
const legacyTargetsSchema = `
CREATE TABLE muscle_groups (name TEXT PRIMARY KEY) WITHOUT ROWID, STRICT;
CREATE TABLE muscle_group_weekly_targets
(
    muscle_group_name   TEXT    PRIMARY KEY REFERENCES muscle_groups (name) ON DELETE CASCADE,
    weekly_sets_target  INTEGER NOT NULL CHECK (weekly_sets_target > 0)
) WITHOUT ROWID, STRICT;`

func newLegacyDB(t *testing.T) (context.Context, *sqlitekit.Database) {
	t.Helper()
	ctx := t.Context()
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       legacyTargetsSchema,
		Fixtures:     "",
		Logger:       slog.New(slog.DiscardHandler),
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("NewDatabase(legacy): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO muscle_groups (name) VALUES ('Chest'), ('Shoulders');
		INSERT INTO muscle_group_weekly_targets (muscle_group_name, weekly_sets_target)
		VALUES ('Chest', 10), ('Shoulders', 6);`)
	if err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}
	return ctx, db
}

func TestPreMigrateMuscleTargets_RewritesLegacyTableAndIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx, db := newLegacyDB(t)

	if err := repository.PreMigrateMuscleTargets(ctx, db); err != nil {
		t.Fatalf("PreMigrateMuscleTargets: %v", err)
	}

	// Legacy column is gone; min_sets carries the old value; max_sets is a
	// valid placeholder (>= min_sets) that fixtures will overwrite on boot.
	var legacyCols int
	if err := db.ReadWrite.QueryRowContext(ctx,
		"SELECT count(*) FROM pragma_table_info('muscle_group_weekly_targets') WHERE name = 'weekly_sets_target'").
		Scan(&legacyCols); err != nil {
		t.Fatalf("pragma: %v", err)
	}
	if legacyCols != 0 {
		t.Errorf("weekly_sets_target column still present after premigration")
	}

	var minSets, maxSets int
	if err := db.ReadWrite.QueryRowContext(ctx,
		"SELECT min_sets, max_sets FROM muscle_group_weekly_targets WHERE muscle_group_name = 'Chest'").
		Scan(&minSets, &maxSets); err != nil {
		t.Fatalf("read Chest: %v", err)
	}
	if minSets != 10 || maxSets < minSets {
		t.Errorf("Chest after premigration: min=%d max=%d, want min=10 max>=10", minSets, maxSets)
	}

	// Idempotent: a second run on the already-migrated table is a no-op.
	if err := repository.PreMigrateMuscleTargets(ctx, db); err != nil {
		t.Fatalf("PreMigrateMuscleTargets (second run): %v", err)
	}
	var rowCount int
	if err := db.ReadWrite.QueryRowContext(ctx,
		"SELECT count(*) FROM muscle_group_weekly_targets").Scan(&rowCount); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rowCount != 2 {
		t.Errorf("row count after second run = %d, want 2", rowCount)
	}
}

func TestPreMigrateMuscleTargets_FreshDatabaseIsNoOp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	// A database with no muscle_group_weekly_targets table at all (premigration
	// runs before migrate on a fresh boot).
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       "CREATE TABLE unrelated (id INTEGER PRIMARY KEY) STRICT;",
		Fixtures:     "",
		Logger:       slog.New(slog.DiscardHandler),
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err = repository.PreMigrateMuscleTargets(ctx, db); err != nil {
		t.Fatalf("PreMigrateMuscleTargets on fresh DB should be a no-op, got %v", err)
	}
}
