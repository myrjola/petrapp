package sqlite

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/myrjola/petrapp/internal/testhelpers"
)

const legacyExerciseSetsSchema = `
CREATE TABLE workout_sessions (
	user_id            INTEGER NOT NULL,
	workout_date       TEXT    NOT NULL,
	difficulty_rating  INTEGER,
	started_at         TEXT,
	completed_at       TEXT,
	periodization_type TEXT NOT NULL DEFAULT 'strength',
	PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;

CREATE TABLE workout_exercise (
	id                  INTEGER PRIMARY KEY,
	workout_user_id     INTEGER NOT NULL,
	workout_date        TEXT    NOT NULL,
	exercise_id         INTEGER NOT NULL,
	warmup_completed_at TEXT
) STRICT;

CREATE TABLE exercises (
	id                   INTEGER PRIMARY KEY,
	name                 TEXT NOT NULL UNIQUE,
	category             TEXT NOT NULL,
	exercise_type        TEXT NOT NULL DEFAULT 'weighted',
	description_markdown TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE exercise_sets (
	workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
	set_number          INTEGER NOT NULL CHECK (set_number > 0),
	weight_kg           REAL,
	min_reps            INTEGER NOT NULL CHECK (min_reps > 0),
	max_reps            INTEGER NOT NULL CHECK (max_reps >= min_reps),
	completed_reps      INTEGER CHECK (completed_reps IS NULL OR completed_reps >= 0),
	completed_at        TEXT,
	signal              TEXT,
	PRIMARY KEY (workout_exercise_id, set_number)
) WITHOUT ROWID, STRICT;
`

func TestPreMigrateExerciseSetTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := connect(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err = db.ReadWrite.ExecContext(ctx, legacyExerciseSetsSchema); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}
	// Legacy seed: hypertrophy bench (min=6, max=10, completed=8) and a plank row.
	seed := "INSERT INTO exercises (id, name, category, exercise_type) VALUES " +
		"(1, 'Bench Press', 'upper', 'weighted'), (2, 'Plank', 'upper', 'bodyweight');" +
		"INSERT INTO workout_sessions (user_id, workout_date, periodization_type) VALUES " +
		"(1, '2026-04-01', 'hypertrophy');" +
		"INSERT INTO workout_exercise (id, workout_user_id, workout_date, exercise_id) VALUES " +
		"(10, 1, '2026-04-01', 1), (11, 1, '2026-04-01', 2);" +
		"INSERT INTO exercise_sets " +
		"(workout_exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps, signal) VALUES " +
		"(10, 1, 60.0, 6, 10, 8, 'on_target'), (11, 1, NULL, 5, 5, 5, 'on_target');"
	if _, err = db.ReadWrite.ExecContext(ctx, seed); err != nil {
		t.Fatalf("seed legacy data: %v", err)
	}

	if err = db.preMigrateExerciseSetTarget(ctx); err != nil {
		t.Fatalf("preMigrate: %v", err)
	}

	// Bench row: target_value = max_reps = 10, completed_value = 8.
	var target, completed int
	if err = db.ReadWrite.QueryRowContext(ctx,
		`SELECT target_value, completed_value FROM exercise_sets WHERE workout_exercise_id = 10`,
	).Scan(&target, &completed); err != nil {
		t.Fatalf("read bench row: %v", err)
	}
	if target != 10 || completed != 8 {
		t.Errorf("bench row: target=%d completed=%d, want 10 and 8", target, completed)
	}

	// Plank row dropped.
	var plankCount int
	if err = db.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM exercise_sets WHERE workout_exercise_id = 11`,
	).Scan(&plankCount); err != nil {
		t.Fatalf("count plank rows: %v", err)
	}
	if plankCount != 0 {
		t.Errorf("plank rows after migration = %d, want 0", plankCount)
	}

	// Idempotent: second call is a no-op and does not error.
	if err = db.preMigrateExerciseSetTarget(ctx); err != nil {
		t.Fatalf("preMigrate idempotent call: %v", err)
	}

	// Declarative migrator must accept the new schema as no-op for exercise_sets.
	if err = db.migrateTo(ctx, schemaDefinition); err != nil {
		t.Fatalf("migrateTo after premigration: %v", err)
	}
}

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
