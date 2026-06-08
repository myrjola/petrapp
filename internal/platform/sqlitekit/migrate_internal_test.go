package sqlitekit

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/myrjola/petrapp/internal/platform/testkit"
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
			logger := testkit.NewLogger(testkit.NewWriter(t))
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

func TestNewDatabase_PremigrationRunsBeforeMigrate(t *testing.T) {
	t.Parallel()

	var called, ranBeforeMigrate bool
	cfg := Config{
		URL:      ":memory:",
		Schema:   "CREATE TABLE widgets (id INTEGER PRIMARY KEY) STRICT;",
		Fixtures: "",
		Logger:   slog.New(slog.DiscardHandler),
		Premigration: func(ctx context.Context, db *Database) error {
			called = true
			// At premigration time the declarative migrate has not run yet,
			// so the schema table must not exist.
			var n int
			if err := db.ReadWrite.QueryRowContext(ctx,
				"SELECT count(*) FROM sqlite_master WHERE type='table' AND name='widgets'").Scan(&n); err != nil {
				return err
			}
			ranBeforeMigrate = n == 0
			return nil
		},
	}

	db, err := NewDatabase(t.Context(), cfg)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if !called {
		t.Error("Premigration was not called")
	}
	if !ranBeforeMigrate {
		t.Error("Premigration ran after migrate (widgets table already existed)")
	}
}

func TestNewDatabase_PremigrationErrorPropagates(t *testing.T) {
	t.Parallel()

	cfg := Config{
		URL:      ":memory:",
		Schema:   "CREATE TABLE widgets (id INTEGER PRIMARY KEY) STRICT;",
		Fixtures: "",
		Logger:   slog.New(slog.DiscardHandler),
		Premigration: func(context.Context, *Database) error {
			return errors.New("boom")
		},
	}
	if _, err := NewDatabase(t.Context(), cfg); err == nil {
		t.Fatal("expected NewDatabase to fail when premigration errors")
	}
}
