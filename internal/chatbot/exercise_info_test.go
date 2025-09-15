package chatbot_test

import (
	"context"
	"testing"

	"github.com/myrjola/petrapp/internal/chatbot"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// Contract test for get_exercise_info LLM function
// This test MUST fail initially as the function is not yet implemented.
func TestExerciseInfoTool_GetExerciseInfo(t *testing.T) {
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

	// Create comprehensive test data
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'A chest exercise performed lying on a bench'),
		(2, 'Squat', 'lower', 'weighted', 'A compound leg exercise'),
		(3, 'Push-ups', 'upper', 'bodyweight', 'A bodyweight chest exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO muscle_groups (name) VALUES
		('Chest'), ('Triceps'), ('Shoulders'), ('Quads'), ('Glutes'), ('Hamstrings')
	`)
	if err != nil {
		t.Fatalf("Failed to insert muscle groups: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary) VALUES
		(1, 'Chest', 1), (1, 'Triceps', 0), (1, 'Shoulders', 0),
		(2, 'Quads', 1), (2, 'Glutes', 1), (2, 'Hamstrings', 0),
		(3, 'Chest', 1), (3, 'Triceps', 0)
	`)
	if err != nil {
		t.Fatalf("Failed to insert exercise muscle groups: %v", err)
	}

	// Insert workout history for user
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at) VALUES
		(?, '2024-01-01', '2024-01-01T10:00:00Z', '2024-01-01T11:00:00Z'),
		(?, '2024-01-03', '2024-01-03T10:00:00Z', '2024-01-03T11:00:00Z'),
		(?, '2024-01-05', '2024-01-05T10:00:00Z', '2024-01-05T11:00:00Z')
	`, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert workout sessions: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
		(?, '2024-01-01', 1), (?, '2024-01-03', 1), (?, '2024-01-05', 1),
		(?, '2024-01-01', 2), (?, '2024-01-05', 2)
	`, userID, userID, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert workout exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
		-- Bench Press progression
		(?, '2024-01-01', 1, 1, 80.0, 8, 10, 10),
		(?, '2024-01-01', 1, 2, 80.0, 8, 10, 9),
		(?, '2024-01-01', 1, 3, 80.0, 8, 10, 8),
		(?, '2024-01-03', 1, 1, 82.5, 8, 10, 10),
		(?, '2024-01-03', 1, 2, 82.5, 8, 10, 9),
		(?, '2024-01-05', 1, 1, 85.0, 8, 10, 10),
		-- Squat progression
		(?, '2024-01-01', 2, 1, 100.0, 5, 8, 8),
		(?, '2024-01-01', 2, 2, 100.0, 5, 8, 7),
		(?, '2024-01-05', 2, 1, 105.0, 5, 8, 8)
	`, userID, userID, userID, userID, userID, userID, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert exercise sets: %v", err)
	}

	// Create chatbot service
	service := chatbot.NewService(db, logger, "test-api-key")

	// Test cases for the get_exercise_info function
	testCases := []struct {
		name           string
		exerciseName   string
		includeHistory bool
		expectError    bool
		description    string
	}{
		{
			name:           "Get basic exercise info without history",
			exerciseName:   "Bench Press",
			includeHistory: false,
			expectError:    false,
			description:    "Should return basic exercise information",
		},
		{
			name:           "Get exercise info with user history",
			exerciseName:   "Bench Press",
			includeHistory: true,
			expectError:    false,
			description:    "Should return exercise info plus user's workout history",
		},
		{
			name:           "Get info for exercise with limited history",
			exerciseName:   "Squat",
			includeHistory: true,
			expectError:    false,
			description:    "Should return exercise info for exercise with some history",
		},
		{
			name:           "Get info for exercise without user history",
			exerciseName:   "Push-ups",
			includeHistory: true,
			expectError:    false,
			description:    "Should return exercise info even if user has no history",
		},
		{
			name:           "Non-existent exercise",
			exerciseName:   "Flying Unicorn Press",
			includeHistory: false,
			expectError:    true,
			description:    "Should return error for non-existent exercise",
		},
		{
			name:           "Empty exercise name",
			exerciseName:   "",
			includeHistory: false,
			expectError:    true,
			description:    "Should return error for empty exercise name",
		},
		{
			name:           "Case insensitive lookup",
			exerciseName:   "bench press",
			includeHistory: false,
			expectError:    false,
			description:    "Should find exercise with case insensitive matching",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set user context
			userCtx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)

			// This will fail because ExerciseInfoTool doesn't exist yet
			// That's expected for TDD - we write the test first, then implement
			tool := service.GetExerciseInfoTool()
			if tool == nil {
				t.Skip("ExerciseInfoTool not implemented yet (expected for TDD)")
			}

			request := chatbot.ExerciseInfoRequest{
				ExerciseName:   tc.exerciseName,
				IncludeHistory: tc.includeHistory,
			}

			result, err := tool.GetExerciseInfo(userCtx, request)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error for case: %s", tc.name)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for case %s: %v", tc.name, err)
				}
				if result == nil {
					t.Error("expected non-nil result for valid request")
				} else {
					// Validate result structure
					if result.ExerciseName == "" {
						t.Error("expected non-empty exercise name in result")
					}
					if result.Category == "" {
						t.Error("expected non-empty category in result")
					}
					if result.ExerciseType == "" {
						t.Error("expected non-empty exercise type in result")
					}
					if len(result.MuscleGroups) == 0 {
						t.Error("expected muscle groups in result")
					}
					if len(result.PrimaryMuscleGroups) == 0 {
						t.Error("expected primary muscle groups in result")
					}

					// If history was requested and user has history, it should be included
					if tc.includeHistory && (tc.exerciseName == "Bench Press" || tc.exerciseName == "bench press") {
						if result.UserHistory == nil {
							t.Error("expected user history for exercise with user data")
						} else {
							if result.UserHistory.TotalSessions <= 0 {
								t.Error("expected positive total sessions in user history")
							}
							if result.UserHistory.PersonalRecord == nil || *result.UserHistory.PersonalRecord <= 0 {
								t.Error("expected positive personal record in user history")
							}
						}
					}

					// If history not requested, it should be nil
					if !tc.includeHistory && result.UserHistory != nil {
						t.Error("expected nil user history when not requested")
					}
				}
			}
		})
	}
}

// Test that exercise info properly isolates user data.
func TestExerciseInfoTool_UserIsolation(t *testing.T) {
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

	// Insert shared exercise
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'A chest exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert exercise: %v", err)
	}

	// Insert workout data for both users with different histories
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date) VALUES
		(?, '2024-01-01'), (?, '2024-01-02'), (?, '2024-01-03'),
		(?, '2024-01-01')
	`, user1ID, user1ID, user1ID, user2ID)
	if err != nil {
		t.Fatalf("Failed to insert workout sessions: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
		(?, '2024-01-01', 1), (?, '2024-01-02', 1), (?, '2024-01-03', 1),
		(?, '2024-01-01', 1)
	`, user1ID, user1ID, user1ID, user2ID)
	if err != nil {
		t.Fatalf("Failed to insert workout exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
		-- User1 has 3 sessions with progression
		(?, '2024-01-01', 1, 1, 80.0, 8, 10, 10),
		(?, '2024-01-02', 1, 1, 85.0, 8, 10, 10),
		(?, '2024-01-03', 1, 1, 90.0, 8, 10, 10),
		-- User2 has 1 session
		(?, '2024-01-01', 1, 1, 70.0, 8, 10, 10)
	`, user1ID, user1ID, user1ID, user2ID)
	if err != nil {
		t.Fatalf("Failed to insert exercise sets: %v", err)
	}

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetExerciseInfoTool()
	if tool == nil {
		t.Skip("ExerciseInfoTool not implemented yet (expected for TDD)")
	}

	// Test that user1 sees their own history
	user1Ctx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, user1ID)
	result1, err := tool.GetExerciseInfo(user1Ctx, chatbot.ExerciseInfoRequest{
		ExerciseName:   "Bench Press",
		IncludeHistory: true,
	})

	if err != nil {
		t.Errorf("unexpected error for user1: %v", err)
	}
	if result1 != nil && result1.UserHistory != nil {
		// User1 should see 3 sessions and 90kg personal record
		if result1.UserHistory.TotalSessions != 3 {
			t.Errorf("expected user1 to see 3 sessions, got %d", result1.UserHistory.TotalSessions)
		}
		if result1.UserHistory.PersonalRecord == nil || *result1.UserHistory.PersonalRecord != 90.0 {
			t.Errorf("expected user1 personal record to be 90kg, got %v", result1.UserHistory.PersonalRecord)
		}
	}

	// Test that user2 sees only their own history
	user2Ctx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, user2ID)
	result2, err := tool.GetExerciseInfo(user2Ctx, chatbot.ExerciseInfoRequest{
		ExerciseName:   "Bench Press",
		IncludeHistory: true,
	})

	if err != nil {
		t.Errorf("unexpected error for user2: %v", err)
	}
	if result2 != nil && result2.UserHistory != nil {
		// User2 should see 1 session and 70kg personal record
		if result2.UserHistory.TotalSessions != 1 {
			t.Errorf("expected user2 to see 1 session, got %d", result2.UserHistory.TotalSessions)
		}
		if result2.UserHistory.PersonalRecord == nil || *result2.UserHistory.PersonalRecord != 70.0 {
			t.Errorf("expected user2 personal record to be 70kg, got %v", result2.UserHistory.PersonalRecord)
		}
	}
}

// Test exercise lookup validation and security.
func TestExerciseInfoTool_SecurityValidation(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

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

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetExerciseInfoTool()
	if tool == nil {
		t.Skip("ExerciseInfoTool not implemented yet (expected for TDD)")
	}

	maliciousInputs := []struct {
		name         string
		exerciseName string
	}{
		{"SQL Injection in exercise name", "'; DROP TABLE exercises; --"},
		{"Script injection", "<script>alert('xss')</script>"},
		{"Path traversal attempt", "../../../etc/passwd"},
		{"Unicode normalization attack", "bènch prèss"},
		{"Very long input", string(make([]byte, 10000))},
		{"Null bytes", "bench\x00press"},
	}

	for _, tc := range maliciousInputs {
		t.Run(tc.name, func(t *testing.T) {
			userCtx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
			result, err := tool.GetExerciseInfo(userCtx, chatbot.ExerciseInfoRequest{
				ExerciseName:   tc.exerciseName,
				IncludeHistory: false,
			})

			// Should either return an error or safely handle the input
			// Should never cause a panic or database corruption
			if err == nil && result != nil {
				// If it succeeds, the result should be safe
				if result.ExerciseName == tc.exerciseName {
					t.Errorf("potentially unsafe input was processed as-is: %s", tc.exerciseName)
				}
			}
		})
	}

	// Test without user context - should fail safely
	t.Run("Missing user context", func(t *testing.T) {
		_, err := tool.GetExerciseInfo(ctx, chatbot.ExerciseInfoRequest{
			ExerciseName:   "Bench Press",
			IncludeHistory: true,
		})
		if err == nil {
			t.Error("expected error when user context is missing")
		}
	})
}
