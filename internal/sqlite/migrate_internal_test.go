package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"github.com/myrjola/petrapp/internal/testhelpers"
)

func TestDatabase_migrate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		schemaDefinitions []string
		testQueries       []string
		wantErr           bool
	}{
		{
			name:              "empty schema",
			schemaDefinitions: []string{""},
			testQueries:       []string{"SELECT * FROM sqlite_master"},
			wantErr:           false,
		},
		{
			name:              "create table",
			schemaDefinitions: []string{"CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)"},
			testQueries: []string{
				"INSERT INTO test (name) VALUES ('test')",
				"SELECT * FROM test",
			},
			wantErr: false,
		},
		{
			name: "drop table",
			schemaDefinitions: []string{
				"CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)",
				"", // drop table
			},
			testQueries: []string{"INSERT INTO test (name) VALUES ('test')"},
			wantErr:     true,
		},
		{
			name: "add column",
			schemaDefinitions: []string{
				"CREATE TABLE test (id INTEGER PRIMARY KEY)",
				"CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)",
			},
			testQueries: []string{"INSERT INTO test (name) VALUES ('test')"},
			wantErr:     false,
		},
		{
			name: "remove column",
			schemaDefinitions: []string{
				"CREATE TABLE test (id INTEGER PRIMARY KEY)",
				"CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)",
				"CREATE TABLE test (id INTEGER PRIMARY KEY)",
			},
			testQueries: []string{"INSERT INTO test (name) VALUES ('test')"},
			wantErr:     true,
		},
		{
			name: "create index",
			schemaDefinitions: []string{
				"CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT); CREATE INDEX test_name ON test (name)",
			},
			testQueries: []string{"DROP INDEX test_name"},
			wantErr:     false,
		},
		{
			name: "drop index",
			schemaDefinitions: []string{
				"CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT); CREATE INDEX test_name ON test (name)",
				"CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)",
			},
			testQueries: []string{"DROP INDEX test_name"},
			wantErr:     true,
		},
		{
			name: "update index",
			schemaDefinitions: []string{
				"CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT); CREATE INDEX test_name ON test (name)",
				"CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT); CREATE INDEX test_name ON test (id, name)",
			},
			testQueries: []string{"DROP INDEX test_name"},
			wantErr:     false,
		},
		{
			name: "create trigger",
			schemaDefinitions: []string{
				`CREATE TABLE test ( id   INTEGER PRIMARY KEY, name TEXT );
                 CREATE TRIGGER test_trigger AFTER INSERT ON test BEGIN SELECT RAISE ( FAIL, 'fail' ); END;`,
			},
			testQueries: []string{"INSERT INTO test (name) VALUES ('test')"},
			wantErr:     true,
		},
		{
			name: "delete trigger",
			schemaDefinitions: []string{
				`CREATE TABLE test ( id   INTEGER PRIMARY KEY, name TEXT );
                 CREATE TRIGGER test_trigger AFTER INSERT ON test BEGIN SELECT RAISE ( FAIL, 'fail' ); END;`,
				"CREATE TABLE test ( id   INTEGER PRIMARY KEY, name TEXT )",
			},
			testQueries: []string{"INSERT INTO test (name) VALUES ('test')"},
			wantErr:     false,
		},
		{
			name: "update trigger",
			schemaDefinitions: []string{
				`CREATE TABLE test ( id   INTEGER PRIMARY KEY, name TEXT );
                 CREATE TRIGGER test_trigger AFTER INSERT ON test BEGIN SELECT RAISE ( FAIL, 'fail' ); END;`,
				`CREATE TABLE test ( id   INTEGER PRIMARY KEY, name TEXT );
                 CREATE TRIGGER test_trigger AFTER INSERT ON test BEGIN SELECT 1; END;`,
			},
			testQueries: []string{"INSERT INTO test (name) VALUES ('test')"},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
			db, err := connect(ctx, ":memory:", logger)
			if err != nil {
				t.Fatalf("Failed to connect to database: %v", err)
			}
			defer func(db *Database) {
				err = db.Close()
				if err != nil {
					t.Errorf("Failed to close database: %v", err)
				}
			}(db)

			for _, schemaDefinition := range tt.schemaDefinitions {
				logger.LogAttrs(ctx, slog.LevelInfo, "migrating", slog.String("schema", schemaDefinition))
				err = db.migrateTo(ctx, schemaDefinition)
				if err != nil {
					t.Fatalf("Failed to migrate: %v", err)
				}
			}

			for _, query := range tt.testQueries {
				logger.LogAttrs(ctx, slog.LevelInfo, "executing", slog.String("query", query))
				_, err = db.ReadWrite.ExecContext(ctx, query)
				if tt.wantErr && err == nil {
					t.Errorf("Expected error for query %q, but got none", query)
				}
				if !tt.wantErr && err != nil {
					t.Errorf("Unexpected error for query %q: %v", query, err)
				}
			}
		})
	}
}

func TestPreMigrateRepWindows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const legacyExercisesSchema = `
CREATE TABLE exercises (
    id                       INTEGER PRIMARY KEY,
    name                     TEXT    NOT NULL UNIQUE CHECK (LENGTH(name) < 124),
    category                 TEXT    NOT NULL CHECK (category IN ('full_body', 'upper', 'lower')),
    exercise_type            TEXT    NOT NULL DEFAULT 'weighted'
                             CHECK (exercise_type IN ('weighted', 'bodyweight', 'assisted', 'time_based')),
    description_markdown     TEXT    NOT NULL DEFAULT '' CHECK (LENGTH(description_markdown) < 20000),
    default_starting_seconds INTEGER CHECK (default_starting_seconds IS NULL OR default_starting_seconds > 0),
    CHECK (exercise_type <> 'time_based' OR default_starting_seconds IS NOT NULL)
) STRICT;
`
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := connect(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("new test database: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close database: %v", closeErr)
		}
	}()

	// Build the legacy state: drop whatever the helper created and recreate
	// just the exercises table in its pre-rep_min shape, with realistic rows.
	if _, err = db.ReadWrite.ExecContext(ctx, `DROP TABLE IF EXISTS exercises`); err != nil {
		t.Fatalf("drop exercises: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx, legacyExercisesSchema); err != nil {
		t.Fatalf("create legacy exercises: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, default_starting_seconds) VALUES
		(1, 'Deadlift', 'full_body', 'weighted', NULL),
		(21, 'Plank', 'upper', 'time_based', 30),
		(99, 'Mystery Lift', 'upper', 'weighted', NULL)`); err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	// Premigration must succeed and populate columns.
	if err = db.preMigrateRepWindows(ctx); err != nil {
		t.Fatalf("preMigrateRepWindows: %v", err)
	}

	// Verify each row is correctly populated.
	checkRow := func(id int, wantMin, wantMax any) {
		t.Helper()
		var gotMin, gotMax sql.NullInt64
		if err = db.ReadOnly.QueryRowContext(ctx,
			`SELECT rep_min, rep_max FROM exercises WHERE id = ?`, id).
			Scan(&gotMin, &gotMax); err != nil {
			t.Fatalf("query id %d: %v", id, err)
		}
		switch v := wantMin.(type) {
		case int:
			if !gotMin.Valid || gotMin.Int64 != int64(v) {
				t.Errorf("id %d rep_min: want %d, got %v", id, v, gotMin)
			}
		case nil:
			if gotMin.Valid {
				t.Errorf("id %d rep_min: want NULL, got %d", id, gotMin.Int64)
			}
		}
		switch v := wantMax.(type) {
		case int:
			if !gotMax.Valid || gotMax.Int64 != int64(v) {
				t.Errorf("id %d rep_max: want %d, got %v", id, v, gotMax)
			}
		case nil:
			if gotMax.Valid {
				t.Errorf("id %d rep_max: want NULL, got %d", id, gotMax.Int64)
			}
		}
	}
	checkRow(1, 3, 6)      // Deadlift, known row
	checkRow(21, nil, nil) // Plank, time_based — must remain NULL
	checkRow(99, 5, 10)    // Mystery Lift, unknown ID — catch-all default

	// Idempotence: running again is a no-op.
	if err = db.preMigrateRepWindows(ctx); err != nil {
		t.Fatalf("preMigrateRepWindows (second run): %v", err)
	}
	checkRow(1, 3, 6)
	checkRow(99, 5, 10)

	// After the premigration, the declarative migrator should accept the
	// state without further changes.
	if err = db.migrateTo(ctx, schemaDefinition); err != nil {
		t.Fatalf("migrateTo after premigration: %v", err)
	}
}
