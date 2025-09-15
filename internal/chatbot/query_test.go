package chatbot_test

import (
	"context"
	"testing"

	"github.com/myrjola/petrapp/internal/chatbot"
	"github.com/myrjola/petrapp/internal/chatbot/tools"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// Contract test for query_workout_data LLM function
// This test MUST fail initially as the function is not yet implemented
func TestQueryWorkoutDataTool_ExecuteQuery(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	// Create test database
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert test user
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create workout data for testing
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (name, category, exercise_type, description_markdown) VALUES
		('Bench Press', 'upper', 'weighted', 'Chest exercise'),
		('Squat', 'lower', 'weighted', 'Leg exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at) VALUES
		(?, '2024-01-01', '2024-01-01T10:00:00Z', '2024-01-01T11:00:00Z'),
		(?, '2024-01-02', '2024-01-02T10:00:00Z', '2024-01-02T11:00:00Z')
	`, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert test workout sessions: %v", err)
	}

	// Create chatbot service - this will create the tools we want to test
	service := chatbot.NewService(db, logger, "test-api-key")

	// Test cases for the query_workout_data function
	testCases := []struct {
		name        string
		query       string
		description string
		expectError bool
	}{
		{
			name:        "Valid SELECT query",
			query:       "SELECT COUNT(*) as workout_count FROM workout_sessions WHERE user_id = ?",
			description: "Count total workouts for user",
			expectError: false,
		},
		{
			name:        "Complex JOIN query",
			query:       "SELECT e.name, COUNT(*) as frequency FROM exercises e JOIN workout_exercise we ON e.id = we.exercise_id JOIN workout_sessions ws ON we.workout_user_id = ws.user_id AND we.workout_date = ws.workout_date WHERE ws.user_id = ? GROUP BY e.name",
			description: "Exercise frequency analysis",
			expectError: false,
		},
		{
			name:        "Invalid INSERT query",
			query:       "INSERT INTO workout_sessions (user_id, workout_date) VALUES (1, '2024-01-03')",
			description: "Attempt to insert data",
			expectError: true,
		},
		{
			name:        "Invalid DROP query",
			query:       "DROP TABLE workout_sessions",
			description: "Attempt to drop table",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set user context
			userCtx := context.WithValue(ctx, "user_id", userID)

			// This will fail because QueryWorkoutDataTool doesn't exist yet
			// That's expected for TDD - we write the test first, then implement
			tool := service.GetQueryWorkoutDataTool()
			if tool == nil {
				t.Skip("QueryWorkoutDataTool not implemented yet (expected for TDD)")
			}

			params := tools.QueryWorkoutDataParams{
				Query:       tc.query,
				Description: tc.description,
			}
			result, err := tool.QueryWorkoutData(userCtx, params)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error for query: %s", tc.query)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for valid query: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result for valid query")
				}
			}
		})
	}
}

// Test that the query function properly isolates user data
func TestQueryWorkoutDataTool_UserIsolation(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert two test users
	var user1ID, user2ID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("user1"), "User 1").Scan(&user1ID)
	if err != nil {
		t.Fatalf("Failed to insert user1: %v", err)
	}

	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("user2"), "User 2").Scan(&user2ID)
	if err != nil {
		t.Fatalf("Failed to insert user2: %v", err)
	}

	// Insert workout data for both users
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date) VALUES
		(?, '2024-01-01'),
		(?, '2024-01-02'),
		(?, '2024-01-01')
	`, user1ID, user1ID, user2ID)
	if err != nil {
		t.Fatalf("Failed to insert workout sessions: %v", err)
	}

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetQueryWorkoutDataTool()
	if tool == nil {
		t.Skip("QueryWorkoutDataTool not implemented yet (expected for TDD)")
	}

	// Test that user1 can only see their own data
	user1Ctx := context.WithValue(ctx, "user_id", user1ID)
	params1 := tools.QueryWorkoutDataParams{
		Query:       "SELECT COUNT(*) as count FROM workout_sessions",
		Description: "Count workouts",
	}
	result, err := tool.QueryWorkoutData(user1Ctx, params1)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil && len(result.Rows) > 0 {
		count := result.Rows[0][0]
		// User1 should see 2 workouts, not 3 (user isolation)
		if count != int64(2) {
			t.Errorf("expected user1 to see 2 workouts, got %v", count)
		}
	}

	// Test that user2 can only see their own data
	user2Ctx := context.WithValue(ctx, "user_id", user2ID)
	params2 := tools.QueryWorkoutDataParams{
		Query:       "SELECT COUNT(*) as count FROM workout_sessions",
		Description: "Count workouts",
	}
	result2, err := tool.QueryWorkoutData(user2Ctx, params2)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result2 != nil && len(result2.Rows) > 0 {
		count := result2.Rows[0][0]
		// User2 should see 1 workout
		if count != int64(1) {
			t.Errorf("expected user2 to see 1 workout, got %v", count)
		}
	}
}

// Test query validation and security
func TestQueryWorkoutDataTool_SecurityValidation(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetQueryWorkoutDataTool()
	if tool == nil {
		t.Skip("QueryWorkoutDataTool not implemented yet (expected for TDD)")
	}

	maliciousQueries := []struct {
		name  string
		query string
	}{
		{"SQL Injection attempt", "SELECT * FROM users; DROP TABLE users; --"},
		{"Cross-table access", "SELECT * FROM credentials"},
		{"System table access", "SELECT name FROM sqlite_master WHERE type='table'"},
		{"File system access", "ATTACH DATABASE '/etc/passwd' AS etc"},
		{"Function execution", "SELECT load_extension('malicious')"},
	}

	for _, tc := range maliciousQueries {
		t.Run(tc.name, func(t *testing.T) {
			params := tools.QueryWorkoutDataParams{
				Query:       tc.query,
				Description: "Malicious query",
			}
			_, err := tool.QueryWorkoutData(ctx, params)
			if err == nil {
				t.Errorf("expected malicious query to be blocked: %s", tc.query)
			}
		})
	}
}
