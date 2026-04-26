package sqlite

import (
	"context"
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

// legacyWorkoutSchema reproduces the workout_exercise / exercise_sets shape that
// existed before the workout_exercise stable-id migration. Used to seed a
// realistic "production" database state for preMigrateWorkoutExerciseStableID.
const legacyWorkoutSchema = `
CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    webauthn_user_id BLOB NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    created TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ')),
    updated TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ')),
    is_admin INTEGER NOT NULL DEFAULT 0
) STRICT;

CREATE TABLE exercises (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    category TEXT NOT NULL,
    exercise_type TEXT NOT NULL DEFAULT 'weighted',
    description_markdown TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE workout_sessions (
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workout_date TEXT NOT NULL,
    difficulty_rating INTEGER,
    started_at TEXT,
    completed_at TEXT,
    periodization_type TEXT NOT NULL DEFAULT 'strength',
    PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;

CREATE TABLE workout_exercise (
    workout_user_id INTEGER NOT NULL,
    workout_date TEXT NOT NULL,
    exercise_id INTEGER NOT NULL,
    warmup_completed_at TEXT,
    PRIMARY KEY (workout_user_id, workout_date, exercise_id),
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) WITHOUT ROWID, STRICT;

CREATE TABLE exercise_sets (
    workout_user_id INTEGER NOT NULL,
    workout_date TEXT NOT NULL,
    exercise_id INTEGER NOT NULL,
    set_number INTEGER NOT NULL,
    weight_kg REAL,
    min_reps INTEGER NOT NULL,
    max_reps INTEGER NOT NULL,
    completed_reps INTEGER,
    completed_at TEXT,
    signal TEXT,
    PRIMARY KEY (workout_user_id, workout_date, exercise_id, set_number),
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) WITHOUT ROWID, STRICT;

CREATE INDEX exercise_sets_user_exercise_date_idx
    ON exercise_sets (workout_user_id, exercise_id, workout_date, set_number);
`

func TestDatabase_preMigrateWorkoutExerciseStableID(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := connect(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("close: %v", closeErr)
		}
	}()

	// Seed legacy schema and data: one workout_exercise with warmup completed
	// (matches sets) and one exercise without a workout_exercise row at all
	// (warmup never started, but sets exist).
	if _, err = db.ReadWrite.ExecContext(ctx, legacyWorkoutSchema); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	seed := `
		INSERT INTO users (id, webauthn_user_id, display_name) VALUES (1, X'01', 'Test');
		INSERT INTO exercises (id, name, category) VALUES (10, 'Bench', 'upper'), (20, 'Squat', 'lower');
		INSERT INTO workout_sessions (user_id, workout_date) VALUES (1, '2026-04-20');
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id, warmup_completed_at)
		    VALUES (1, '2026-04-20', 10, '2026-04-20T10:00:00.000Z');
		INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, min_reps, max_reps)
		    VALUES (1, '2026-04-20', 10, 1, 8, 12),
		           (1, '2026-04-20', 10, 2, 8, 12),
		           (1, '2026-04-20', 20, 1, 5, 5);
	`
	if _, err = db.ReadWrite.ExecContext(ctx, seed); err != nil {
		t.Fatalf("seed legacy data: %v", err)
	}

	if err = db.preMigrateWorkoutExerciseStableID(ctx); err != nil {
		t.Fatalf("preMigrate: %v", err)
	}

	assertPostMigrationState(ctx, t, db)

	// Idempotence: second call must be a no-op.
	if err = db.preMigrateWorkoutExerciseStableID(ctx); err != nil {
		t.Fatalf("preMigrate (idempotent call): %v", err)
	}
	assertPostMigrationState(ctx, t, db)

	// Run the declarative migrator on top to confirm it accepts the rewritten
	// schema and does not destroy data.
	if err = db.migrateTo(ctx, schemaDefinition); err != nil {
		t.Fatalf("migrateTo after preMigrate: %v", err)
	}
	assertPostMigrationState(ctx, t, db)
}

func assertPostMigrationState(ctx context.Context, t *testing.T, db *Database) {
	t.Helper()

	var setCount int
	if err := db.ReadWrite.QueryRowContext(ctx, `SELECT COUNT(*) FROM exercise_sets`).Scan(&setCount); err != nil {
		t.Fatalf("count exercise_sets: %v", err)
	}
	if setCount != 3 {
		t.Errorf("expected 3 exercise_sets rows, got %d", setCount)
	}

	var weCount int
	if err := db.ReadWrite.QueryRowContext(ctx, `SELECT COUNT(*) FROM workout_exercise`).Scan(&weCount); err != nil {
		t.Fatalf("count workout_exercise: %v", err)
	}
	if weCount != 2 {
		t.Errorf("expected 2 workout_exercise rows (one synthesized for the no-warmup exercise), got %d", weCount)
	}

	var orphans int
	if err := db.ReadWrite.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM exercise_sets es
		LEFT JOIN workout_exercise we ON we.id = es.workout_exercise_id
		WHERE we.id IS NULL`).Scan(&orphans); err != nil {
		t.Fatalf("orphan check: %v", err)
	}
	if orphans != 0 {
		t.Errorf("expected 0 orphan exercise_sets rows, got %d", orphans)
	}

	// The originally warmup-completed exercise (id=10) keeps its timestamp.
	var warmupCount int
	if err := db.ReadWrite.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM workout_exercise
		WHERE exercise_id = 10 AND warmup_completed_at = '2026-04-20T10:00:00.000Z'`).Scan(&warmupCount); err != nil {
		t.Fatalf("warmup check: %v", err)
	}
	if warmupCount != 1 {
		t.Errorf("expected warmup timestamp preserved on exercise 10, got %d matching rows", warmupCount)
	}
}
