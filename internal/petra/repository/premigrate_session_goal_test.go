package repository_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/petra/repository"
	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

// legacySessionGoalSchemaSQL reproduces the product schema as it stood before
// the workout_sessions.periodization_type -> session_goal column rename. The
// rename was pure (no data reshaping), so deriving it from the live schema by
// reversing the name keeps the fixture in sync. Delete together with
// PreMigrateSessionGoal once production has booted past the rename.
func legacySessionGoalSchemaSQL() string {
	return strings.ReplaceAll(repository.SchemaSQL, "session_goal", "periodization_type")
}

// seedLegacySession inserts a user and a workout_sessions row carrying a
// non-default periodization_type so the rename can be observed to preserve it.
func seedLegacySession(ctx context.Context, t *testing.T, db *sqlitekit.Database) {
	t.Helper()
	stmts := []string{
		`INSERT INTO users (webauthn_user_id, display_name) VALUES (X'01', 'Test User')`,
		`INSERT INTO workout_sessions (user_id, workout_date, periodization_type)
		 VALUES (1, '2026-06-10', 'hypertrophy')`,
	}
	for _, stmt := range stmts {
		if _, err := db.ReadWrite.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed legacy session (%q): %v", stmt, err)
		}
	}
}

func TestPreMigrateSessionGoal_renamesAndPreservesData(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + legacySessionGoalSchemaSQL(),
		Fixtures:     "",
		Logger:       testkit.NewLogger(testkit.NewWriter(t)),
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create legacy database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	seedLegacySession(ctx, t, db)

	if err = repository.PreMigrateSessionGoal(ctx, db); err != nil {
		t.Fatalf("premigration: %v", err)
	}

	// The old column is gone and the value rode along under the new name.
	var legacyCount int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('workout_sessions') WHERE name = 'periodization_type'`,
	).Scan(&legacyCount); err != nil {
		t.Fatalf("count legacy column: %v", err)
	}
	if legacyCount != 0 {
		t.Errorf("periodization_type column still present after rename")
	}
	var goal string
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT session_goal FROM workout_sessions WHERE user_id = 1 AND workout_date = '2026-06-10'`,
	).Scan(&goal); err != nil {
		t.Fatalf("read session_goal: %v", err)
	}
	if goal != "hypertrophy" {
		t.Errorf("session_goal = %q, want hypertrophy", goal)
	}

	// Idempotent: a second run is a no-op (the guard short-circuits).
	if err = repository.PreMigrateSessionGoal(ctx, db); err != nil {
		t.Fatalf("second premigration run: %v", err)
	}
}

func TestPreMigrateSessionGoal_fullMigratePathPreservesData(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dbPath := filepath.Join(t.TempDir(), "petra.db")
	logger := testkit.NewLogger(testkit.NewWriter(t))

	// Build the database in the legacy shape and seed a hypertrophy session.
	legacy, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          dbPath,
		Schema:       auth.SchemaSQL + "\n" + legacySessionGoalSchemaSQL(),
		Fixtures:     "",
		Logger:       logger,
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create legacy file database: %v", err)
	}
	t.Cleanup(func() { _ = legacy.Close() })
	seedLegacySession(ctx, t, legacy)
	if err = legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	// Reopen with the real schema + premigration: the production boot path.
	migrated, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          dbPath,
		Schema:       auth.SchemaSQL + "\n" + repository.SchemaSQL,
		Fixtures:     repository.FixturesSQL,
		Logger:       logger,
		Premigration: repository.PreMigrateSessionGoal,
	})
	if err != nil {
		t.Fatalf("reopen with real schema + premigration: %v", err)
	}
	t.Cleanup(func() { _ = migrated.Close() })

	var goal string
	if err = migrated.ReadOnly.QueryRowContext(ctx,
		`SELECT session_goal FROM workout_sessions WHERE user_id = 1 AND workout_date = '2026-06-10'`,
	).Scan(&goal); err != nil {
		t.Fatalf("read session_goal after full migrate: %v", err)
	}
	if goal != "hypertrophy" {
		t.Errorf("session_goal = %q, want hypertrophy", goal)
	}
}

func TestPreMigrateSessionGoal_freshDatabaseIsNoOp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	// A database already on the real schema has session_goal and no
	// periodization_type; the premigration must be a clean no-op.
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + repository.SchemaSQL,
		Fixtures:     repository.FixturesSQL,
		Logger:       testkit.NewLogger(testkit.NewWriter(t)),
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create fresh database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err = repository.PreMigrateSessionGoal(ctx, db); err != nil {
		t.Fatalf("premigration on fresh database: %v", err)
	}

	var goalColumnCount int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('workout_sessions') WHERE name = 'session_goal'`,
	).Scan(&goalColumnCount); err != nil {
		t.Fatalf("count session_goal column: %v", err)
	}
	if goalColumnCount != 1 {
		t.Errorf("session_goal column present = %d, want 1", goalColumnCount)
	}
}
