package tools_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	_ "github.com/mattn/go-sqlite3"

	"github.com/myrjola/petrapp/internal/chatbot/tools"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// setupTestDatabase creates an in-memory SQLite database with workout schema and test data.
func setupTestDatabase(t *testing.T) *sqlite.Database {
	t.Helper()

	// Create a temporary database file for testing
	tempFile, err := os.CreateTemp("", "test_workout_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tempFile.Close()

	dbPath := tempFile.Name()
	t.Cleanup(func() {
		os.Remove(dbPath)
	})

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Errorf("failed to close database: %v", closeErr)
		}
	})

	// Create test schema (simplified version of the actual schema)
	schema := `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			display_name TEXT NOT NULL,
			created TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ')),
			updated TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
		);

		CREATE TABLE exercises (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			category TEXT NOT NULL CHECK (category IN ('full_body', 'upper', 'lower')),
			exercise_type TEXT NOT NULL DEFAULT 'weighted' CHECK (exercise_type IN ('weighted', 'bodyweight'))
		);

		CREATE TABLE workout_sessions (
			user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			workout_date TEXT NOT NULL CHECK (DATE(workout_date, '+0 days') IS workout_date),
			difficulty_rating INTEGER CHECK (difficulty_rating BETWEEN 1 AND 5),
			started_at TEXT CHECK (started_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', started_at) = started_at),
			completed_at TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
			PRIMARY KEY (user_id, workout_date)
		) WITHOUT ROWID;

		CREATE TABLE exercise_sets (
			workout_user_id INTEGER NOT NULL,
			workout_date TEXT NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
			exercise_id INTEGER NOT NULL,
			set_number INTEGER NOT NULL CHECK (set_number > 0),
			weight_kg REAL CHECK (weight_kg IS NULL OR weight_kg >= 0),
			min_reps INTEGER NOT NULL CHECK (min_reps > 0),
			max_reps INTEGER NOT NULL CHECK (max_reps >= min_reps),
			completed_reps INTEGER,
			completed_at TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
			PRIMARY KEY (workout_user_id, workout_date, exercise_id, set_number),
			FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
			FOREIGN KEY (exercise_id) REFERENCES exercises (id)
		) WITHOUT ROWID;

		CREATE TABLE conversations (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			title TEXT,
			created_at TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ')),
			updated_at TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ')),
			is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
		);

		CREATE TABLE muscle_groups (
			name TEXT NOT NULL UNIQUE PRIMARY KEY
		) WITHOUT ROWID;

		CREATE TABLE exercise_muscle_groups (
			exercise_id INTEGER NOT NULL REFERENCES exercises (id) ON DELETE CASCADE,
			muscle_group_name TEXT NOT NULL REFERENCES muscle_groups (name) ON DELETE CASCADE,
			is_primary INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),
			PRIMARY KEY (exercise_id, muscle_group_name)
		) WITHOUT ROWID;
	`

	if _, schemaErr := db.Exec(schema); schemaErr != nil {
		t.Fatalf("failed to create test schema: %v", schemaErr)
	}

	// Insert test data
	testData := `
		INSERT INTO users (id, display_name) VALUES
			(1, 'John Doe'),
			(2, 'Jane Smith');

		INSERT INTO exercises (id, name, category, exercise_type) VALUES
			(1, 'Bench Press', 'upper', 'weighted'),
			(2, 'Squat', 'lower', 'weighted'),
			(3, 'Deadlift', 'full_body', 'weighted'),
			(4, 'Push-ups', 'upper', 'bodyweight');

		INSERT INTO muscle_groups (name) VALUES
			('Chest'), ('Legs'), ('Back'), ('Shoulders');

		INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary) VALUES
			(1, 'Chest', 1),
			(1, 'Shoulders', 0),
			(2, 'Legs', 1),
			(3, 'Back', 1),
			(3, 'Legs', 0),
			(4, 'Chest', 1);

		INSERT INTO workout_sessions (user_id, workout_date, difficulty_rating, started_at, completed_at) VALUES
			(1, '2024-01-01', 3, '2024-01-01T10:00:00.000Z', '2024-01-01T11:00:00.000Z'),
			(1, '2024-01-02', 4, '2024-01-02T10:00:00.000Z', '2024-01-02T11:30:00.000Z'),
			(2, '2024-01-01', 2, '2024-01-01T14:00:00.000Z', '2024-01-01T15:00:00.000Z'),
			(2, '2024-01-03', 3, '2024-01-03T09:00:00.000Z', NULL);

		INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps, completed_at) VALUES
			(1, '2024-01-01', 1, 1, 100.0, 8, 12, 10, '2024-01-01T10:15:00.000Z'),
			(1, '2024-01-01', 1, 2, 100.0, 8, 12, 8, '2024-01-01T10:20:00.000Z'),
			(1, '2024-01-01', 2, 1, 140.0, 5, 8, 6, '2024-01-01T10:30:00.000Z'),
			(1, '2024-01-02', 3, 1, 180.0, 3, 5, 3, '2024-01-02T10:15:00.000Z'),
			(2, '2024-01-01', 1, 1, 80.0, 10, 15, 12, '2024-01-01T14:15:00.000Z'),
			(2, '2024-01-01', 4, 1, NULL, 15, 20, 18, '2024-01-01T14:25:00.000Z');

		INSERT INTO conversations (id, user_id, title, created_at) VALUES
			(1, 1, 'My first workout chat', '2024-01-01T12:00:00.000Z'),
			(2, 2, 'Training questions', '2024-01-02T10:00:00.000Z'),
			(3, 1, 'Progress tracking', '2024-01-03T08:00:00.000Z');
	`

	if _, dataErr := db.Exec(testData); dataErr != nil {
		t.Fatalf("failed to insert test data: %v", dataErr)
	}

	// For tests, we need to make sure both connections share the same in-memory database
	// Using the same connection for both ReadWrite and ReadOnly in tests
	sqliteDB := &sqlite.Database{
		ReadWrite: db,
		ReadOnly:  db,
	}

	return sqliteDB
}

// createAuthenticatedContext creates a context with authenticated user ID.
func createAuthenticatedContext(userID int) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	return ctx
}

// createUnauthenticatedContext creates a context without authentication.
func createUnauthenticatedContext() context.Context {
	return context.Background()
}

func TestWorkoutDataQueryTool_QueryWorkoutData_ValidQuery(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)
	params := tools.QueryWorkoutDataParams{
		Query:       "SELECT workout_date, difficulty_rating FROM workout_sessions ORDER BY workout_date",
		Description: "Get all workout sessions for user",
	}

	result, err := tool.QueryWorkoutData(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedColumns := []string{"workout_date", "difficulty_rating"}
	if diff := cmp.Diff(expectedColumns, result.Columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}

	if result.RowCount != 2 {
		t.Errorf("expected 2 rows for user 1, got %d", result.RowCount)
	}

	if result.UserID != 1 {
		t.Errorf("expected user_id 1, got %d", result.UserID)
	}

	if result.Description != "Get all workout sessions for user" {
		t.Errorf("expected description to match, got %s", result.Description)
	}
}

func TestWorkoutDataQueryTool_QueryWorkoutData_UnauthenticatedUser(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createUnauthenticatedContext()
	params := tools.QueryWorkoutDataParams{
		Query:       "SELECT * FROM workout_sessions",
		Description: "Get workout sessions",
	}

	_, err := tool.QueryWorkoutData(ctx, params)
	if err == nil {
		t.Error("expected error for unauthenticated user")
	}

	expectedError := "user not authenticated"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("expected error to contain '%s', got %v", expectedError, err)
	}
}

func TestWorkoutDataQueryTool_QueryWorkoutData_UserIsolation(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	// Test query for user 1
	ctx1 := createAuthenticatedContext(1)
	params := tools.QueryWorkoutDataParams{
		Query:       "SELECT COUNT(*) as workout_count FROM workout_sessions",
		Description: "Count workout sessions",
	}

	result1, err := tool.QueryWorkoutData(ctx1, params)
	if err != nil {
		t.Fatalf("expected no error for user 1, got %v", err)
	}

	if result1.RowCount != 1 || len(result1.Rows) != 1 {
		t.Fatalf("expected 1 result row, got %d rows", result1.RowCount)
	}

	count1 := result1.Rows[0][0].(int64)
	if count1 != 2 {
		t.Errorf("expected 2 workouts for user 1, got %d", count1)
	}

	// Test same query for user 2 using the same database instance
	ctx2 := createAuthenticatedContext(2)
	result2, err := tool.QueryWorkoutData(ctx2, params)
	if err != nil {
		t.Fatalf("expected no error for user 2, got %v", err)
	}

	count2 := result2.Rows[0][0].(int64)
	if count2 != 2 {
		t.Errorf("expected 2 workouts for user 2, got %d", count2)
	}

	// Verify different users get different data
	if result1.UserID == result2.UserID {
		t.Error("expected different user IDs in results")
	}
}

func TestWorkoutDataQueryTool_QueryWorkoutData_ExerciseSetsWithJoin(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)
	params := tools.QueryWorkoutDataParams{
		Query:       "SELECT e.name, es.weight_kg, es.completed_reps FROM exercise_sets es JOIN exercises e ON es.exercise_id = e.id ORDER BY es.workout_date, es.set_number",
		Description: "Get exercise sets with exercise names",
	}

	result, err := tool.QueryWorkoutData(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedColumns := []string{"name", "weight_kg", "completed_reps"}
	if diff := cmp.Diff(expectedColumns, result.Columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}

	// User 1 should have 4 exercise sets based on test data
	if result.RowCount != 4 {
		t.Errorf("expected 4 rows for user 1's exercise sets, got %d", result.RowCount)
	}

	// Verify data content
	if len(result.Rows) > 0 {
		firstRow := result.Rows[0]
		exerciseName := firstRow[0].(string)
		if exerciseName != "Bench Press" {
			t.Errorf("expected first exercise to be 'Bench Press', got %s", exerciseName)
		}
	}
}

func TestWorkoutDataQueryTool_QueryWorkoutData_ComplexJoinQuery(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)
	params := tools.QueryWorkoutDataParams{
		Query: `
			SELECT
				e.name AS exercise_name,
				mg.name AS muscle_group,
				MAX(es.weight_kg) AS max_weight,
				COUNT(es.set_number) AS total_sets
			FROM exercise_sets es
			JOIN exercises e ON es.exercise_id = e.id
			JOIN exercise_muscle_groups emg ON e.id = emg.exercise_id
			JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
			WHERE emg.is_primary = 1
			GROUP BY e.name, mg.name
			ORDER BY max_weight DESC
		`,
		Description: "Get max weights and set counts by exercise and primary muscle group",
	}

	result, err := tool.QueryWorkoutData(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expectedColumns := []string{"exercise_name", "muscle_group", "max_weight", "total_sets"}
	if diff := cmp.Diff(expectedColumns, result.Columns); diff != "" {
		t.Errorf("columns mismatch (-want +got):\n%s", diff)
	}

	if result.RowCount == 0 {
		t.Error("expected at least one result row")
	}
}

func TestWorkoutDataQueryTool_QueryWorkoutData_ForbiddenQueries(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)

	forbiddenQueries := []struct {
		query string
		desc  string
	}{
		{"INSERT INTO users (display_name) VALUES ('hacker')", "INSERT statement"},
		{"UPDATE workout_sessions SET difficulty_rating = 5", "UPDATE statement"},
		{"DELETE FROM exercise_sets WHERE workout_user_id = 1", "DELETE statement"},
		{"DROP TABLE users", "DROP statement"},
		{"CREATE TABLE malicious (id INTEGER)", "CREATE statement"},
		{"ATTACH DATABASE 'evil.db' AS evil", "ATTACH DATABASE"},
		{"PRAGMA table_info(users)", "PRAGMA statement"},
		{"TRUNCATE users", "TRUNCATE statement"},
		{"ALTER TABLE users ADD COLUMN evil TEXT", "ALTER statement"},
	}

	for _, tc := range forbiddenQueries {
		t.Run(tc.desc, func(t *testing.T) {
			params := tools.QueryWorkoutDataParams{
				Query:       tc.query,
				Description: "Malicious query test",
			}

			_, err := tool.QueryWorkoutData(ctx, params)
			if err == nil {
				t.Errorf("expected query to be forbidden: %s", tc.query)
			}

			if !strings.Contains(err.Error(), "validation failed") {
				t.Errorf("expected validation error, got %v", err)
			}
		})
	}
}

func TestWorkoutDataQueryTool_QueryWorkoutData_ExistingUserIDFilter(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)
	params := tools.QueryWorkoutDataParams{
		Query:       "SELECT COUNT(*) FROM workout_sessions WHERE user_id = 999", // Wrong user ID
		Description: "Query with hardcoded user_id",
	}

	result, err := tool.QueryWorkoutData(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// The tool should have replaced user_id = 999 with user_id = 1
	if result.RowCount != 1 {
		t.Error("expected 1 result row")
	}

	count := result.Rows[0][0].(int64)
	if count != 2 {
		t.Errorf("expected 2 workouts for user 1 (user_id should have been replaced), got %d", count)
	}
}

func TestWorkoutDataQueryTool_QueryWorkoutData_EmptyQuery(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)
	params := tools.QueryWorkoutDataParams{
		Query:       "",
		Description: "Empty query test",
	}

	_, err := tool.QueryWorkoutData(ctx, params)
	if err == nil {
		t.Error("expected error for empty query")
	}

	if !strings.Contains(err.Error(), "empty query") {
		t.Errorf("expected 'empty query' error, got %v", err)
	}
}

func TestWorkoutDataQueryTool_QueryWorkoutData_WhitespaceOnlyQuery(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)
	params := tools.QueryWorkoutDataParams{
		Query:       "   \n\t  ",
		Description: "Whitespace only query",
	}

	_, err := tool.QueryWorkoutData(ctx, params)
	if err == nil {
		t.Error("expected error for whitespace-only query")
	}

	if !strings.Contains(err.Error(), "empty query") {
		t.Errorf("expected 'empty query' error, got %v", err)
	}
}

func TestWorkoutDataQueryTool_ToOpenAIFunction(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	function := tool.ToOpenAIFunction()

	// Verify function structure
	if function["type"] != "function" {
		t.Errorf("expected type 'function', got %v", function["type"])
	}

	funcDef, ok := function["function"].(map[string]interface{})
	if !ok {
		t.Fatal("expected function definition to be a map")
	}

	if funcDef["name"] != "query_workout_data" {
		t.Errorf("expected name 'query_workout_data', got %v", funcDef["name"])
	}

	if funcDef["description"] == "" {
		t.Error("expected non-empty description")
	}

	// Verify parameters structure
	params, ok := funcDef["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("expected parameters to be a map")
	}

	properties, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	// Check required parameters exist
	if _, exists := properties["query"]; !exists {
		t.Error("expected 'query' parameter to exist")
	}

	if _, exists := properties["description"]; !exists {
		t.Error("expected 'description' parameter to exist")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("expected required to be a string slice")
	}

	expectedRequired := []string{"query", "description"}
	if diff := cmp.Diff(expectedRequired, required); diff != "" {
		t.Errorf("required parameters mismatch (-want +got):\n%s", diff)
	}
}

func TestWorkoutDataQueryTool_ExecuteFunction(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)

	// Test valid function execution
	arguments := `{"query": "SELECT COUNT(*) as count FROM workout_sessions", "description": "Count user workouts"}`
	resultJSON, err := tool.ExecuteFunction(ctx, "query_workout_data", arguments)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var result tools.QueryWorkoutDataResult
	if unmarshalErr := json.Unmarshal([]byte(resultJSON), &result); unmarshalErr != nil {
		t.Fatalf("failed to unmarshal result: %v", unmarshalErr)
	}

	if result.UserID != 1 {
		t.Errorf("expected user_id 1, got %d", result.UserID)
	}

	if result.RowCount != 1 {
		t.Errorf("expected 1 row, got %d", result.RowCount)
	}
}

func TestWorkoutDataQueryTool_ExecuteFunction_UnsupportedFunction(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)

	_, err := tool.ExecuteFunction(ctx, "unsupported_function", "{}")
	if err == nil {
		t.Error("expected error for unsupported function")
	}

	if !strings.Contains(err.Error(), "unsupported function") {
		t.Errorf("expected 'unsupported function' error, got %v", err)
	}
}

func TestWorkoutDataQueryTool_ExecuteFunction_InvalidArguments(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)

	_, err := tool.ExecuteFunction(ctx, "query_workout_data", "invalid json")
	if err == nil {
		t.Error("expected error for invalid JSON arguments")
	}

	if !strings.Contains(err.Error(), "failed to parse arguments") {
		t.Errorf("expected 'failed to parse arguments' error, got %v", err)
	}
}

func TestWorkoutDataQueryTool_QueryWorkoutData_ConversationsTable(t *testing.T) {
	db := setupTestDatabase(t)
	logger := slog.Default()
	tool := tools.NewWorkoutDataQueryTool(db, logger)

	ctx := createAuthenticatedContext(1)
	params := tools.QueryWorkoutDataParams{
		Query:       "SELECT id, title FROM conversations ORDER BY created_at",
		Description: "Get user conversations",
	}

	result, err := tool.QueryWorkoutData(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// User 1 should have 2 conversations
	if result.RowCount != 2 {
		t.Errorf("expected 2 conversations for user 1, got %d", result.RowCount)
	}

	if len(result.Rows) > 0 {
		firstTitle := result.Rows[0][1].(string)
		if firstTitle != "My first workout chat" {
			t.Errorf("expected first conversation title 'My first workout chat', got %s", firstTitle)
		}
	}
}
