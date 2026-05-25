package sqlite

import (
	"log/slog"
	"reflect"
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

func TestDatabase_preMigrateWorkoutPositions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Drop the live schema's workout-related tables; we re-seed the legacy shape
	// to exercise the premigration end-to-end. PRAGMA foreign_keys must be off
	// while we tear down the live schema; otherwise the cascade graph rejects
	// drops of tables still referenced by remaining tables.
	if _, err = db.ReadWrite.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS scheduled_pushes`,
		`DROP TABLE IF EXISTS exercise_sets`,
		`DROP TABLE IF EXISTS workout_exercises`,
		`DROP TABLE IF EXISTS workout_exercise`,
		`DROP TABLE IF EXISTS workout_sessions`,
		`DROP TABLE IF EXISTS exercise_muscle_groups`,
		`DROP TABLE IF EXISTS exercises`,
		`DROP TABLE IF EXISTS push_subscriptions`,
		`DROP TABLE IF EXISTS workout_preferences`,
		`DROP TABLE IF EXISTS credentials`,
		`DROP TABLE IF EXISTS users`,
	} {
		if _, execErr := db.ReadWrite.ExecContext(ctx, stmt); execErr != nil {
			t.Fatalf("teardown %q: %v", stmt, execErr)
		}
	}

	legacy := `
	CREATE TABLE users (id INTEGER PRIMARY KEY) STRICT;
	CREATE TABLE exercises (id INTEGER PRIMARY KEY, name TEXT NOT NULL) STRICT;
	CREATE TABLE workout_sessions (
		user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
		workout_date TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
		PRIMARY KEY (user_id, workout_date)
	) WITHOUT ROWID, STRICT;
	CREATE TABLE workout_exercise (
		id                  INTEGER PRIMARY KEY,
		workout_user_id     INTEGER NOT NULL,
		workout_date        TEXT    NOT NULL,
		exercise_id         INTEGER NOT NULL,
		warmup_completed_at TEXT,
		UNIQUE (workout_user_id, workout_date, exercise_id),
		FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
		FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
	) STRICT;
	CREATE TABLE exercise_sets (
		workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
		set_number          INTEGER NOT NULL CHECK (set_number > 0),
		weight_kg           REAL,
		target_value        INTEGER NOT NULL CHECK (target_value > 0),
		completed_value     INTEGER,
		completed_at        TEXT,
		signal              TEXT,
		PRIMARY KEY (workout_exercise_id, set_number)
	) WITHOUT ROWID, STRICT;
	CREATE TABLE scheduled_pushes (
		id                  INTEGER PRIMARY KEY,
		user_id             INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
		workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
		fire_at             TEXT    NOT NULL,
		payload             TEXT    NOT NULL,
		created_at          TEXT    NOT NULL
	) STRICT;
	`
	if _, err = db.ReadWrite.ExecContext(ctx, legacy); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	seed := []string{
		`INSERT INTO users (id) VALUES (1)`,
		`INSERT INTO exercises (id, name) VALUES (10, 'bench'), (20, 'row'), (30, 'squat'), (40, 'plank')`,
		`INSERT INTO workout_sessions (user_id, workout_date) VALUES
			(1, '2026-05-04'),
			(1, '2026-05-05'),
			(1, '2026-05-06')`,
		`INSERT INTO workout_exercise (id, workout_user_id, workout_date, exercise_id) VALUES
			(5,  1, '2026-05-04', 10),
			(12, 1, '2026-05-04', 20),
			(47, 1, '2026-05-04', 30),
			(50, 1, '2026-05-05', 40)`,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, target_value, completed_value, completed_at) VALUES
			(5,  1, 5, NULL, NULL),
			(5,  2, 5, 5,    '2026-05-04T09:00:00.000Z'),
			(12, 1, 8, NULL, NULL),
			(47, 1, 6, NULL, NULL),
			(50, 1, 30, NULL, NULL)`,
		`INSERT INTO scheduled_pushes (id, user_id, workout_exercise_id, fire_at, payload, created_at) VALUES
			(1, 1, 12, '2026-05-04T09:30:00.000Z', '{}', '2026-05-04T09:00:00.000Z')`,
	}
	for _, stmt := range seed {
		if _, err = db.ReadWrite.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed: %q: %v", stmt, err)
		}
	}

	if err = db.preMigrateWorkoutPositions(ctx); err != nil {
		t.Fatalf("preMigrateWorkoutPositions: %v", err)
	}

	type row struct {
		date     string
		position int
		exercise int
	}
	rows, err := db.ReadOnly.QueryContext(ctx,
		`SELECT workout_date, position, exercise_id FROM workout_exercises
		 WHERE workout_user_id = 1 ORDER BY workout_date, position`)
	if err != nil {
		t.Fatalf("query workout_exercises: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var got []row
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.date, &r.position, &r.exercise); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	want := []row{
		{"2026-05-04", 0, 10},
		{"2026-05-04", 1, 20},
		{"2026-05-04", 2, 30},
		{"2026-05-05", 0, 40},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workout_exercises rows = %#v, want %#v", got, want)
	}

	var setsCount int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM exercise_sets`).Scan(&setsCount); err != nil {
		t.Fatalf("count exercise_sets: %v", err)
	}
	if setsCount != 5 {
		t.Fatalf("exercise_sets count = %d, want 5", setsCount)
	}

	var pushPos int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT position FROM scheduled_pushes WHERE id = 1`).Scan(&pushPos); err != nil {
		t.Fatalf("scheduled_pushes lookup: %v", err)
	}
	if pushPos != 1 {
		t.Fatalf("scheduled_pushes.position = %d, want 1", pushPos)
	}

	if err = db.preMigrateWorkoutPositions(ctx); err != nil {
		t.Fatalf("idempotent re-run: %v", err)
	}

	// The migrateTo no-op assertion is enabled in Task 14, once schema.sql is updated
	// to the post-premigration shape. Until then, the declarative migrator sees a
	// shape mismatch (workout_exercises exists but schema.sql still has workout_exercise).
	_ = schemaDefinition
}
