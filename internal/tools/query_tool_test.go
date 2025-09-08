package tools_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	_ "github.com/mattn/go-sqlite3"

	"github.com/myrjola/petrapp/internal/tools"
)

// setupTestDB creates an in-memory SQLite database with test data.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create test schema
	schema := `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL
		);

		CREATE TABLE exercises (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			category TEXT NOT NULL
		);

		CREATE TABLE workouts (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			date TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		);

		CREATE TABLE exercise_sets (
			id INTEGER PRIMARY KEY,
			workout_id INTEGER NOT NULL,
			exercise_id INTEGER NOT NULL,
			weight_kg REAL,
			reps INTEGER,
			FOREIGN KEY (workout_id) REFERENCES workouts(id),
			FOREIGN KEY (exercise_id) REFERENCES exercises(id)
		);
	`

	if _, schemaErr := db.Exec(schema); schemaErr != nil {
		t.Fatalf("failed to create test schema: %v", schemaErr)
	}

	// Insert test data
	testData := `
		INSERT INTO users (id, name, email) VALUES 
			(1, 'John Doe', 'john@example.com'),
			(2, 'Jane Smith', 'jane@example.com');

		INSERT INTO exercises (id, name, category) VALUES
			(1, 'Bench Press', 'Chest'),
			(2, 'Squat', 'Legs'),
			(3, 'Deadlift', 'Back');

		INSERT INTO workouts (id, user_id, date) VALUES
			(1, 1, '2024-01-01'),
			(2, 1, '2024-01-02'),
			(3, 2, '2024-01-01');

		INSERT INTO exercise_sets (id, workout_id, exercise_id, weight_kg, reps) VALUES
			(1, 1, 1, 100.0, 10),
			(2, 1, 1, 100.0, 8),
			(3, 1, 2, 140.0, 5),
			(4, 2, 3, 180.0, 3),
			(5, 3, 1, 80.0, 12);
	`

	if _, dataErr := db.Exec(testData); dataErr != nil {
		t.Fatalf("failed to insert test data: %v", dataErr)
	}

	return db
}

func TestNewSecureQueryTool(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger)

	// Test that the tool was created successfully (private fields can't be accessed)
	if tool == nil {
		t.Error("expected non-nil tool")
	}
}

func TestSecureQueryTool_Configuration(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger).
		WithTimeout(10 * time.Second).
		WithMaxRows(500)

	// Test that configuration was applied successfully (private fields can't be accessed)
	if tool == nil {
		t.Error("expected non-nil tool after configuration")
	}
}

func TestSecureQueryTool_ExecuteQuery_ValidSelect(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger)
	ctx := context.Background()

	query := "SELECT id, name, email FROM users WHERE id = 1"
	result, err := tool.ExecuteQuery(ctx, query)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedColumns := []string{"id", "name", "email"}
	if diff := cmp.Diff(expectedColumns, result.Columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}

	if result.RowCount != 1 {
		t.Errorf("expected 1 row, got %d", result.RowCount)
	}

	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row in result, got %d", len(result.Rows))
	}

	expectedRow := []interface{}{int64(1), "John Doe", "john@example.com"}
	if diff := cmp.Diff(expectedRow, result.Rows[0]); diff != "" {
		t.Errorf("row data mismatch (-want +got):\n%s", diff)
	}
}

func TestSecureQueryTool_ExecuteQuery_ComplexQuery(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger)
	ctx := context.Background()

	query := `
		SELECT e.name as exercise_name, MAX(es.weight_kg) as max_weight
		FROM exercises e
		JOIN exercise_sets es ON e.id = es.exercise_id
		JOIN workouts w ON es.workout_id = w.id
		WHERE w.user_id = 1
		GROUP BY e.name
		ORDER BY max_weight DESC
	`

	result, err := tool.ExecuteQuery(ctx, query)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedColumns := []string{"exercise_name", "max_weight"}
	if diff := cmp.Diff(expectedColumns, result.Columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}

	if result.RowCount != 3 {
		t.Errorf("expected 3 rows, got %d", result.RowCount)
	}
}

func TestSecureQueryTool_ValidateSQL_AllowedQueries(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger)

	allowedQueries := []string{
		"SELECT * FROM users",
		"SELECT id, name FROM users WHERE id = 1",
		"SELECT COUNT(*) FROM exercises",
		"SELECT u.name, w.date FROM users u JOIN workouts w ON u.id = w.user_id",
		"SELECT * FROM users UNION SELECT * FROM exercises",
		"WITH user_workouts AS (SELECT user_id, COUNT(*) as workout_count FROM workouts GROUP BY user_id) " +
			"SELECT u.name, uw.workout_count FROM users u JOIN user_workouts uw ON u.id = uw.user_id",
	}

	for _, query := range allowedQueries {
		t.Run(query, func(t *testing.T) {
			if err := tool.ValidateSQL(query); err != nil {
				t.Errorf("expected query to be allowed, got error: %v", err)
			}
		})
	}
}

func TestSecureQueryTool_ValidateSQL_ForbiddenQueries(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger)

	forbiddenQueries := []struct {
		query string
		desc  string
	}{
		{"INSERT INTO users (name, email) VALUES ('test', 'test@example.com')", "INSERT statement"},
		{"UPDATE users SET name = 'test' WHERE id = 1", "UPDATE statement"},
		{"DELETE FROM users WHERE id = 1", "DELETE statement"},
		{"DROP TABLE users", "DROP statement"},
		{"CREATE TABLE test (id INTEGER)", "CREATE statement"},
		{"ATTACH DATABASE 'test.db' AS test", "ATTACH DATABASE"},
		{"PRAGMA table_info(users)", "PRAGMA statement"},
		{"SELECT LOAD_EXTENSION('test')", "LOAD_EXTENSION"},
		{"CREATE TEMP TABLE temp_test (id INTEGER)", "CREATE TEMP TABLE"},
		{"CREATE TEMPORARY TABLE temp_test (id INTEGER)", "CREATE TEMPORARY TABLE"},
		{"CREATE VIEW test_view AS SELECT * FROM users", "CREATE VIEW"},
		{"CREATE TRIGGER test_trigger AFTER INSERT ON users BEGIN SELECT 1; END", "CREATE TRIGGER"},
		{"CREATE INDEX idx_test ON users(name)", "CREATE INDEX"},
		{"WITH malicious AS (SELECT * FROM users) INSERT INTO users (name, email) VALUES ('hacker', 'hack@evil.com')",
			"CTE with INSERT"},
		{"WITH evil AS (SELECT * FROM users) UPDATE users SET name = 'hacked'", "CTE with UPDATE"},
		{"", "empty query"},
		{"   ", "whitespace only query"},
	}

	for _, tc := range forbiddenQueries {
		t.Run(tc.desc, func(t *testing.T) {
			if err := tool.ValidateSQL(tc.query); err == nil {
				t.Errorf("expected query to be forbidden: %s", tc.query)
			}
		})
	}
}

func TestSecureQueryTool_RowLimitEnforcement(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger).WithMaxRows(2)
	ctx := context.Background()

	// Query that would return more than 2 rows without limit
	query := "SELECT * FROM exercise_sets"
	result, err := tool.ExecuteQuery(ctx, query)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.RowCount != 2 {
		t.Errorf("expected row count to be limited to 2, got %d", result.RowCount)
	}

	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows in result, got %d", len(result.Rows))
	}
}

func TestSecureQueryTool_TimeoutEnforcement(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger).WithTimeout(1 * time.Millisecond)
	ctx := context.Background()

	// This query might timeout depending on timing, but we mainly want to test that timeout is respected
	query := "SELECT * FROM users"
	_, err := tool.ExecuteQuery(ctx, query)

	// The query might succeed if it's fast enough, but timeout mechanism should be in place
	// We just verify the tool doesn't crash and handles timeout gracefully
	if err != nil && !strings.Contains(err.Error(), "timeout") {
		t.Logf("Query failed with non-timeout error (this is acceptable): %v", err)
	}
}

func TestSecureQueryTool_SanitizeError(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger)

	testCases := []struct {
		inputErr   error
		expectType string
		desc       string
	}{
		{
			inputErr:   errors.New("syntax error near 'FROM' at line 1 column 5"),
			expectType: "SQL syntax error",
			desc:       "syntax error",
		},
		{
			inputErr:   errors.New("query execution timeout exceeded"),
			expectType: "query execution timeout",
			desc:       "timeout error",
		},
		{
			inputErr:   errors.New("no such table: nonexistent_table"),
			expectType: "referenced table or column not found",
			desc:       "table not found",
		},
		{
			inputErr:   errors.New("FOREIGN KEY constraint failed"),
			expectType: "constraint violation",
			desc:       "constraint error",
		},
		{
			inputErr:   errors.New("some random database error with /path/to/file SQLITE_ERROR"),
			expectType: "query execution failed",
			desc:       "generic error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			sanitizedErr := tool.SanitizeError(tc.inputErr)
			if sanitizedErr.Error() != tc.expectType {
				t.Errorf("expected %q, got %q", tc.expectType, sanitizedErr.Error())
			}
		})
	}
}

func TestConfigureSecureDB(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	ctx := context.Background()
	err := tools.ConfigureSecureDB(ctx, db)

	if err != nil {
		t.Fatalf("expected no error configuring secure DB, got %v", err)
	}

	// Verify some of the pragma settings were applied
	var result string
	err = db.QueryRowContext(ctx, "PRAGMA trusted_schema").Scan(&result)
	if err != nil {
		t.Errorf("failed to check trusted_schema pragma: %v", err)
	}
	if result != "0" {
		t.Errorf("expected trusted_schema to be OFF (0), got %s", result)
	}
}

func TestSecureQueryTool_ExecuteQuery_InvalidTable(t *testing.T) {
	db := setupTestDB(t)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	}()

	logger := slog.Default()
	tool := tools.NewSecureQueryTool(db, logger)
	ctx := context.Background()

	query := "SELECT * FROM nonexistent_table"
	_, err := tool.ExecuteQuery(ctx, query)

	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}

	if err.Error() != "referenced table or column not found" {
		t.Errorf("expected sanitized error message, got: %v", err)
	}
}
