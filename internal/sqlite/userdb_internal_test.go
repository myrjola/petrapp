package sqlite

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/myrjola/petrapp/internal/testhelpers"
)

func TestDatabase_createUserDB(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		userID         int
		setupSchema    string
		setupData      []string
		expectedTables []string
		expectedCounts map[string]int
		wantErr        bool
	}{
		{
			name:   "simple user export",
			userID: 1,
			setupSchema: `
				CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
				CREATE TABLE workouts (id INTEGER PRIMARY KEY, user_id INTEGER, name TEXT, FOREIGN KEY (user_id) REFERENCES users(id));
			`,
			setupData: []string{
				"INSERT INTO users (id, name) VALUES (1, 'John Doe')",
				"INSERT INTO users (id, name) VALUES (2, 'Jane Smith')",
				"INSERT INTO workouts (id, user_id, name) VALUES (1, 1, 'Morning Run')",
				"INSERT INTO workouts (id, user_id, name) VALUES (2, 1, 'Evening Gym')",
				"INSERT INTO workouts (id, user_id, name) VALUES (3, 2, 'Yoga Session')",
			},
			expectedTables: []string{"users", "workouts"},
			expectedCounts: map[string]int{
				"users":    1,
				"workouts": 2,
			},
			wantErr: false,
		},
		{
			name:   "user with no data",
			userID: 999,
			setupSchema: `
				CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
				CREATE TABLE workouts (id INTEGER PRIMARY KEY, user_id INTEGER, name TEXT, FOREIGN KEY (user_id) REFERENCES users(id));
			`,
			setupData: []string{
				"INSERT INTO users (id, name) VALUES (1, 'John Doe')",
				"INSERT INTO workouts (id, user_id, name) VALUES (1, 1, 'Morning Run')",
			},
			expectedTables: []string{"users", "workouts"},
			expectedCounts: map[string]int{
				"users":    0,
				"workouts": 0,
			},
			wantErr: false,
		},
		{
			name:   "complex schema with multiple related tables",
			userID: 1,
			setupSchema: `
				CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT);
				CREATE TABLE workouts (date TEXT, user_id INTEGER, PRIMARY KEY (date, user_id), FOREIGN KEY (user_id) REFERENCES users(id)) WITHOUT ROWID;
				CREATE TABLE exercises (id INTEGER PRIMARY KEY, name TEXT);
				CREATE TABLE exercise_sets (workout_date TEXT, workout_user_id INTEGER, exercise_id INTEGER, PRIMARY KEY (workout_date, workout_user_id, exercise_id), FOREIGN KEY (workout_date, workout_user_id) REFERENCES workouts(date, user_id), FOREIGN KEY (exercise_id) REFERENCES exercises(id)) WITHOUT ROWID;
				CREATE TABLE user_settings (user_id INTEGER PRIMARY KEY, theme TEXT, FOREIGN KEY (user_id) REFERENCES users(id));
			`,
			setupData: []string{
				"INSERT INTO users (id, name, email) VALUES (1, 'John Doe', 'john@example.com')",
				"INSERT INTO users (id, name, email) VALUES (2, 'Jane Smith', 'jane@example.com')",
				"INSERT INTO workouts (date, user_id) VALUES ('2024-01-01', 1)",
				"INSERT INTO workouts (date, user_id) VALUES ('2024-01-02', 2)",
				"INSERT INTO exercises (id, name) VALUES (1, 'Push-ups')",
				"INSERT INTO exercises (id, name) VALUES (2, 'Bench Press')",
				"INSERT INTO exercises (id, name) VALUES (3, 'Pull-ups')",
				"INSERT INTO exercise_sets (workout_date, workout_user_id, exercise_id) VALUES ('2024-01-01', 1,  1)",
				"INSERT INTO exercise_sets (workout_date, workout_user_id, exercise_id) VALUES ('2024-01-02', 2,  2)",
				"INSERT INTO user_settings (user_id, theme) VALUES (1, 'dark')",
				"INSERT INTO user_settings (user_id, theme) VALUES (2, 'light')",
			},
			expectedTables: []string{"users", "workouts", "exercises", "exercise_sets", "user_settings"},
			expectedCounts: map[string]int{
				"users":         1,
				"workouts":      1,
				"exercises":     3,
				"exercise_sets": 1,
				"user_settings": 1,
			},
			wantErr: false,
		},
		{
			name:   "no users table",
			userID: 1,
			setupSchema: `
				CREATE TABLE workouts (id INTEGER PRIMARY KEY, user_id INTEGER, name TEXT);
			`,
			setupData: []string{
				"INSERT INTO workouts (id, user_id, name) VALUES (1, 1, 'Morning Run')",
				"INSERT INTO workouts (id, user_id, name) VALUES (2, 1, 'Evening Gym')",
				"INSERT INTO workouts (id, user_id, name) VALUES (3, 2, 'Yoga Session')",
			},
			expectedTables: []string{},
			wantErr:        true,
		},
		{
			name:   "unrelated tables are not exported",
			userID: 1,
			setupSchema: `
				CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT);
				CREATE TABLE feature_flags (id INTEGER PRIMARY KEY, enabled INTEGER);
			`,
			setupData: []string{
				"INSERT INTO users (id, name) VALUES (1, 'John Doe')",
			},
			expectedTables: []string{"users"},
			expectedCounts: map[string]int{
				"users": 1,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

			// Create main database
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

			// Set up schema
			_, err = db.ReadWrite.ExecContext(ctx, tt.setupSchema)
			if err != nil {
				t.Fatalf("Failed to create schema: %v", err)
			}

			// Insert test data
			for _, dataSQL := range tt.setupData {
				_, err = db.ReadWrite.ExecContext(ctx, dataSQL)
				if err != nil {
					t.Fatalf("Failed to insert test data: %v", err)
				}
			}

			// Create temporary directory for export
			tempDir := t.TempDir()

			// Call createUserDB
			dbPath, err := db.createUserDB(ctx, tt.userID, tempDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("createUserDB() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// Verify the exported database file exists
			if _, err := os.Stat(dbPath); os.IsNotExist(err) {
				t.Errorf("Exported database file does not exist at %s", dbPath)
				return
			}

			// Open the exported database and verify its contents
			exportedDB, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("Failed to open exported database: %v", err)
			}
			defer exportedDB.Close()

			// Verify that only expected tables exist
			rows, err := exportedDB.QueryContext(ctx, "SELECT name FROM sqlite_schema WHERE type = 'table' AND name != 'sqlite_stat1'")
			if err != nil {
				t.Fatalf("Failed to query tables: %v", err)
			}
			defer rows.Close()

			var actualTables []string
			for rows.Next() {
				var tableName string
				if err := rows.Scan(&tableName); err != nil {
					t.Fatalf("Failed to scan table name: %v", err)
				}
				actualTables = append(actualTables, tableName)
			}

			// Check that actual tables match expected tables
			if len(actualTables) != len(tt.expectedTables) {
				t.Errorf("Table count mismatch: got %d tables %v, want %d tables %v", len(actualTables), actualTables, len(tt.expectedTables), tt.expectedTables)
			}

			expectedTableSet := make(map[string]bool)
			for _, table := range tt.expectedTables {
				expectedTableSet[table] = true
			}

			for _, table := range actualTables {
				if !expectedTableSet[table] {
					t.Errorf("Unexpected table found: %s", table)
				}
			}

			// Verify expected tables exist and have correct row counts
			for _, tableName := range tt.expectedTables {
				var count int
				query := "SELECT COUNT(*) FROM " + tableName
				err = exportedDB.QueryRowContext(ctx, query).Scan(&count)
				if err != nil {
					t.Errorf("Failed to query table %s: %v", tableName, err)
					continue
				}

				expectedCount, ok := tt.expectedCounts[tableName]
				if !ok {
					t.Errorf("Missing expected count for table %s", tableName)
					continue
				}

				if count != expectedCount {
					t.Errorf("Table %s: got %d rows, want %d rows", tableName, count, expectedCount)
				}
			}
		})
	}
}
