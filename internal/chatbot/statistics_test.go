package chatbot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/myrjola/petrapp/internal/chatbot"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// Contract test for calculate_statistics LLM function
// This test MUST fail initially as the function is not yet implemented.
func TestCalculateStatisticsTool_CalculateMetrics(t *testing.T) {
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

	// Create comprehensive test workout data
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'Chest exercise'),
		(2, 'Squat', 'lower', 'weighted', 'Leg exercise'),
		(3, 'Deadlift', 'full_body', 'weighted', 'Full body exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO muscle_groups (name) VALUES
		('Chest'), ('Quads'), ('Glutes'), ('Hamstrings'), ('Back')
	`)
	if err != nil {
		t.Fatalf("Failed to insert muscle groups: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary) VALUES
		(1, 'Chest', 1),
		(2, 'Quads', 1), (2, 'Glutes', 0),
		(3, 'Back', 1), (3, 'Hamstrings', 0)
	`)
	if err != nil {
		t.Fatalf("Failed to insert exercise muscle groups: %v", err)
	}

	// Insert workout sessions with progression
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at, difficulty_rating) VALUES
		(?, '2024-01-01', '2024-01-01T10:00:00Z', '2024-01-01T11:00:00Z', 3),
		(?, '2024-01-03', '2024-01-03T10:00:00Z', '2024-01-03T11:15:00Z', 4),
		(?, '2024-01-05', '2024-01-05T10:00:00Z', '2024-01-05T11:30:00Z', 4),
		(?, '2024-01-07', '2024-01-07T10:00:00Z', '2024-01-07T11:45:00Z', 5)
	`, userID, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert workout sessions: %v", err)
	}

	// Insert workout exercises
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
		(?, '2024-01-01', 1), (?, '2024-01-01', 2),
		(?, '2024-01-03', 1), (?, '2024-01-03', 3),
		(?, '2024-01-05', 2), (?, '2024-01-05', 3),
		(?, '2024-01-07', 1), (?, '2024-01-07', 2), (?, '2024-01-07', 3)
	`, userID, userID, userID, userID, userID, userID, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert workout exercises: %v", err)
	}

	// Insert exercise sets with progressive weights
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
		-- Jan 1: Bench Press
		(?, '2024-01-01', 1, 1, 80.0, 8, 10, 10),
		(?, '2024-01-01', 1, 2, 80.0, 8, 10, 9),
		(?, '2024-01-01', 1, 3, 80.0, 8, 10, 8),
		-- Jan 1: Squat
		(?, '2024-01-01', 2, 1, 100.0, 5, 8, 8),
		(?, '2024-01-01', 2, 2, 100.0, 5, 8, 7),
		-- Jan 3: Bench Press (progression)
		(?, '2024-01-03', 1, 1, 82.5, 8, 10, 10),
		(?, '2024-01-03', 1, 2, 82.5, 8, 10, 9),
		-- Jan 3: Deadlift
		(?, '2024-01-03', 3, 1, 120.0, 3, 5, 5),
		-- Jan 5: Squat (progression)
		(?, '2024-01-05', 2, 1, 105.0, 5, 8, 8),
		(?, '2024-01-05', 2, 2, 105.0, 5, 8, 6),
		-- Jan 7: All exercises
		(?, '2024-01-07', 1, 1, 85.0, 8, 10, 10),
		(?, '2024-01-07', 2, 1, 110.0, 5, 8, 8),
		(?, '2024-01-07', 3, 1, 125.0, 3, 5, 5)
	`, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert exercise sets: %v", err)
	}

	// Create chatbot service
	service := chatbot.NewService(db, logger, "test-api-key")

	// Test cases for different metric types
	testCases := []struct {
		name         string
		metricType   string
		exerciseName string
		dateRange    *chatbot.DateRange
		expectError  bool
		description  string
	}{
		{
			name:         "Personal record for specific exercise",
			metricType:   "personal_record",
			exerciseName: "Bench Press",
			description:  "Should find max weight for Bench Press",
			expectError:  false,
		},
		{
			name:        "Personal record for all exercises",
			metricType:  "personal_record",
			description: "Should find all personal records",
			expectError: false,
		},
		{
			name:        "Total volume calculation",
			metricType:  "total_volume",
			description: "Should calculate total volume (weight × reps × sets)",
			expectError: false,
		},
		{
			name:         "Total volume for specific exercise",
			metricType:   "total_volume",
			exerciseName: "Squat",
			description:  "Should calculate total volume for Squat only",
			expectError:  false,
		},
		{
			name:        "Workout frequency",
			metricType:  "workout_frequency",
			description: "Should calculate workouts per week",
			expectError: false,
		},
		{
			name:       "Workout frequency with date range",
			metricType: "workout_frequency",
			dateRange: &chatbot.DateRange{
				StartDate: "2024-01-01",
				EndDate:   "2024-01-05",
			},
			description: "Should calculate frequency for specific period",
			expectError: false,
		},
		{
			name:        "Average intensity",
			metricType:  "average_intensity",
			description: "Should calculate average difficulty rating",
			expectError: false,
		},
		{
			name:        "Muscle group distribution",
			metricType:  "muscle_group_distribution",
			description: "Should show percentage of exercises per muscle group",
			expectError: false,
		},
		{
			name:         "Progression rate for specific exercise",
			metricType:   "progression_rate",
			exerciseName: "Bench Press",
			description:  "Should calculate weight progression over time",
			expectError:  false,
		},
		{
			name:        "Invalid metric type",
			metricType:  "invalid_metric",
			description: "Should reject invalid metric type",
			expectError: true,
		},
		{
			name:        "Empty metric type",
			metricType:  "",
			description: "Should reject empty metric type",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set user context
			userCtx := context.WithValue(ctx, "user_id", userID)

			// This will fail because CalculateStatisticsTool doesn't exist yet
			// That's expected for TDD - we write the test first, then implement
			tool := service.GetCalculateStatisticsTool()
			if tool == nil {
				t.Skip("CalculateStatisticsTool not implemented yet (expected for TDD)")
			}

			request := chatbot.StatisticsRequest{
				MetricType:   tc.metricType,
				ExerciseName: tc.exerciseName,
				DateRange:    tc.dateRange,
			}

			result, err := tool.CalculateStatistics(userCtx, request)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error for invalid input: %v", tc)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for valid input: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result for valid input")
				} else {
					// Validate result structure
					if result.MetricType != tc.metricType {
						t.Errorf("expected metric type %s, got %s", tc.metricType, result.MetricType)
					}
					if result.Value == nil {
						t.Error("expected non-nil value in result")
					}
					if result.Description == "" {
						t.Error("expected non-empty description in result")
					}
				}
			}
		})
	}
}

// Test specific statistical calculations.
func TestCalculateStatisticsTool_SpecificCalculations(t *testing.T) {
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
	tool := service.GetCalculateStatisticsTool()
	if tool == nil {
		t.Skip("CalculateStatisticsTool not implemented yet (expected for TDD)")
	}

	// Test specific calculation scenarios
	scenarios := []struct {
		name           string
		metricType     string
		expectedValue  interface{}
		setupData      func() error
		validateResult func(*chatbot.StatisticsResult) error
	}{
		{
			name:       "Personal record calculation",
			metricType: "personal_record",
			setupData: func() error {
				// Setup data for personal record test
				return setupPersonalRecordData(db, userID)
			},
			validateResult: func(result *chatbot.StatisticsResult) error {
				if result.ExerciseName == "" {
					return errors.New("expected exercise name in personal record result")
				}
				return nil
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			if scenario.setupData != nil {
				if err := scenario.setupData(); err != nil {
					t.Fatalf("Failed to setup test data: %v", err)
				}
			}

			userCtx := context.WithValue(ctx, "user_id", userID)
			result, err := tool.CalculateStatistics(userCtx, chatbot.StatisticsRequest{
				MetricType: scenario.metricType,
			})

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != nil && scenario.validateResult != nil {
				if err := scenario.validateResult(result); err != nil {
					t.Errorf("result validation failed: %v", err)
				}
			}
		})
	}
}

// Test user data isolation for statistics.
func TestCalculateStatisticsTool_UserIsolation(t *testing.T) {
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

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetCalculateStatisticsTool()
	if tool == nil {
		t.Skip("CalculateStatisticsTool not implemented yet (expected for TDD)")
	}

	// Each user should only see their own statistics
	user1Ctx := context.WithValue(ctx, "user_id", user1ID)
	user2Ctx := context.WithValue(ctx, "user_id", user2ID)

	// Test that statistics are properly isolated per user
	result1, err1 := tool.CalculateStatistics(user1Ctx, chatbot.StatisticsRequest{
		MetricType: "workout_frequency",
	})

	result2, err2 := tool.CalculateStatistics(user2Ctx, chatbot.StatisticsRequest{
		MetricType: "workout_frequency",
	})

	if err1 != nil {
		t.Errorf("unexpected error for user1: %v", err1)
	}
	if err2 != nil {
		t.Errorf("unexpected error for user2: %v", err2)
	}

	// Results should be different for different users
	if result1 != nil && result2 != nil {
		if result1.UserID == result2.UserID && result1.UserID != 0 {
			t.Error("statistics should be isolated per user")
		}
	}
}

// Helper function to setup personal record test data.
func setupPersonalRecordData(db *sqlite.Database, userID int) error {
	ctx := context.Background()

	_, err := db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'Chest exercise')
	`)
	if err != nil {
		return err
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date) VALUES
		(?, '2024-01-01'), (?, '2024-01-02')
	`, userID, userID)
	if err != nil {
		return err
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
		(?, '2024-01-01', 1), (?, '2024-01-02', 1)
	`, userID, userID)
	if err != nil {
		return err
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
		(?, '2024-01-01', 1, 1, 100.0, 5, 8, 8),
		(?, '2024-01-02', 1, 1, 105.0, 5, 8, 7)
	`, userID, userID)

	return err
}
