package chatbot_test

import (
	"context"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/chatbot"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// Contract test for analyze_workout_pattern LLM function
// This test MUST fail initially as the function is not yet implemented
func TestWorkoutPatternTool_AnalyzePattern(t *testing.T) {
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

	// Create comprehensive test data for pattern analysis
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'Chest exercise'),
		(2, 'Squat', 'lower', 'weighted', 'Leg exercise'),
		(3, 'Deadlift', 'full_body', 'weighted', 'Full body exercise'),
		(4, 'Overhead Press', 'upper', 'weighted', 'Shoulder exercise'),
		(5, 'Barbell Row', 'upper', 'weighted', 'Back exercise'),
		(6, 'Pull-ups', 'upper', 'bodyweight', 'Back exercise'),
		(7, 'Lunges', 'lower', 'weighted', 'Leg exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO muscle_groups (name) VALUES
		('Chest'), ('Triceps'), ('Shoulders'), ('Quads'), ('Glutes'), ('Hamstrings'),
		('Back'), ('Biceps'), ('Calves'), ('Core')
	`)
	if err != nil {
		t.Fatalf("Failed to insert muscle groups: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary) VALUES
		(1, 'Chest', 1), (1, 'Triceps', 0), (1, 'Shoulders', 0),
		(2, 'Quads', 1), (2, 'Glutes', 1), (2, 'Hamstrings', 0),
		(3, 'Back', 1), (3, 'Hamstrings', 1), (3, 'Glutes', 0),
		(4, 'Shoulders', 1), (4, 'Triceps', 0), (4, 'Core', 0),
		(5, 'Back', 1), (5, 'Biceps', 0),
		(6, 'Back', 1), (6, 'Biceps', 0),
		(7, 'Quads', 1), (7, 'Glutes', 1), (7, 'Core', 0)
	`)
	if err != nil {
		t.Fatalf("Failed to insert exercise muscle groups: %v", err)
	}

	// Insert workout history spanning different time periods for pattern analysis
	baseDate := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)

	// Create consistent workout pattern - 3x per week for 8 weeks
	for week := 0; week < 8; week++ {
		for day := 0; day < 3; day++ {
			workoutDate := baseDate.AddDate(0, 0, week*7+day*2) // Mon, Wed, Fri pattern
			difficultyRating := 3 + (week % 3)                  // Varying difficulty: 3, 4, 5, 3, 4, 5...

			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at, difficulty_rating) VALUES
				(?, ?, ?, ?, ?)
			`, userID, workoutDate.Format("2006-01-02"), workoutDate.Format(time.RFC3339),
				workoutDate.Add(time.Hour).Format(time.RFC3339), difficultyRating)
			if err != nil {
				t.Fatalf("Failed to insert workout session for week %d, day %d: %v", week, day, err)
			}
		}
	}

	// Insert exercises with progressive overload pattern
	workoutDates := []string{}
	for week := 0; week < 8; week++ {
		for day := 0; day < 3; day++ {
			workoutDate := baseDate.AddDate(0, 0, week*7+day*2)
			workoutDates = append(workoutDates, workoutDate.Format("2006-01-02"))
		}
	}

	// Create workout split pattern: Push/Pull/Legs
	for i, date := range workoutDates {
		dayType := i % 3 // 0=Push, 1=Pull, 2=Legs

		switch dayType {
		case 0: // Push day
			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
				(?, ?, 1), (?, ?, 4) -- Bench Press, Overhead Press
			`, userID, date, userID, date)
		case 1: // Pull day
			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
				(?, ?, 5), (?, ?, 6) -- Barbell Row, Pull-ups
			`, userID, date, userID, date)
		case 2: // Legs day
			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
				(?, ?, 2), (?, ?, 3), (?, ?, 7) -- Squat, Deadlift, Lunges
			`, userID, date, userID, date, userID, date)
		}

		if err != nil {
			t.Fatalf("Failed to insert workout exercises for date %s: %v", date, err)
		}
	}

	// Insert exercise sets with progressive overload
	for weekNum, date := range workoutDates {
		week := weekNum / 3 // Which week we're in
		dayType := weekNum % 3

		switch dayType {
		case 0: // Push day - Bench Press progression
			baseWeight := 80.0 + float64(week)*2.5 // Progressive overload
			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
				(?, ?, 1, 1, ?, 8, 10, 10),
				(?, ?, 1, 2, ?, 8, 10, 9),
				(?, ?, 1, 3, ?, 8, 10, 8),
				(?, ?, 4, 1, ?, 8, 10, 10),
				(?, ?, 4, 2, ?, 8, 10, 9)
			`, userID, date, baseWeight, userID, date, baseWeight, userID, date, baseWeight,
				userID, date, baseWeight*0.6, userID, date, baseWeight*0.6) // OHP lighter
		case 1: // Pull day
			baseWeight := 70.0 + float64(week)*2.0
			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
				(?, ?, 5, 1, ?, 8, 10, 10),
				(?, ?, 5, 2, ?, 8, 10, 8),
				(?, ?, 6, 1, 0, 8, 12, 10),
				(?, ?, 6, 2, 0, 8, 12, 9)
			`, userID, date, baseWeight, userID, date, baseWeight, userID, date, userID, date)
		case 2: // Legs day
			squatWeight := 100.0 + float64(week)*5.0
			deadliftWeight := 120.0 + float64(week)*5.0
			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
				(?, ?, 2, 1, ?, 5, 8, 8),
				(?, ?, 2, 2, ?, 5, 8, 7),
				(?, ?, 2, 3, ?, 5, 8, 6),
				(?, ?, 3, 1, ?, 3, 5, 5),
				(?, ?, 3, 2, ?, 3, 5, 4),
				(?, ?, 7, 1, 40.0, 10, 12, 12),
				(?, ?, 7, 2, 40.0, 10, 12, 11)
			`, userID, date, squatWeight, userID, date, squatWeight, userID, date, squatWeight,
				userID, date, deadliftWeight, userID, date, deadliftWeight, userID, date, userID, date)
		}

		if err != nil {
			t.Fatalf("Failed to insert exercise sets for date %s: %v", date, err)
		}
	}

	// Insert some recent missed workouts to test consistency analysis
	missedDate1 := baseDate.AddDate(0, 0, 60) // 2 weeks ago
	missedDate2 := baseDate.AddDate(0, 0, 62) // 10 days ago
	// These dates have no workout_sessions, creating gaps in the pattern

	// Create chatbot service
	service := chatbot.NewService(db, logger, "test-api-key")

	// Test cases for different pattern analysis types
	testCases := []struct {
		name         string
		analysisType string
		lookbackDays int
		expectError  bool
		description  string
	}{
		{
			name:         "Consistency analysis - 30 days",
			analysisType: "consistency",
			lookbackDays: 30,
			description:  "Should analyze workout consistency over past 30 days",
			expectError:  false,
		},
		{
			name:         "Consistency analysis - 90 days",
			analysisType: "consistency",
			lookbackDays: 90,
			description:  "Should analyze workout consistency over past 90 days",
			expectError:  false,
		},
		{
			name:         "Progressive overload analysis",
			analysisType: "progressive_overload",
			lookbackDays: 60,
			description:  "Should analyze progressive overload patterns",
			expectError:  false,
		},
		{
			name:         "Muscle balance analysis",
			analysisType: "muscle_balance",
			lookbackDays: 45,
			description:  "Should analyze muscle group balance in training",
			expectError:  false,
		},
		{
			name:         "Recovery time analysis",
			analysisType: "recovery_time",
			lookbackDays: 30,
			description:  "Should analyze rest periods between muscle group training",
			expectError:  false,
		},
		{
			name:         "Plateau detection",
			analysisType: "plateau_detection",
			lookbackDays: 60,
			description:  "Should detect performance plateaus in exercises",
			expectError:  false,
		},
		{
			name:         "Workout variety analysis",
			analysisType: "workout_variety",
			lookbackDays: 45,
			description:  "Should analyze variety in exercise selection",
			expectError:  false,
		},
		{
			name:         "Short lookback period",
			analysisType: "consistency",
			lookbackDays: 7,
			description:  "Should handle minimum lookback period",
			expectError:  false,
		},
		{
			name:         "Maximum lookback period",
			analysisType: "progressive_overload",
			lookbackDays: 365,
			description:  "Should handle maximum lookback period",
			expectError:  false,
		},
		{
			name:         "Invalid analysis type",
			analysisType: "invalid_analysis",
			lookbackDays: 30,
			description:  "Should reject invalid analysis type",
			expectError:  true,
		},
		{
			name:         "Empty analysis type",
			analysisType: "",
			lookbackDays: 30,
			description:  "Should reject empty analysis type",
			expectError:  true,
		},
		{
			name:         "Lookback period too short",
			analysisType: "consistency",
			lookbackDays: 5,
			description:  "Should reject lookback period below minimum",
			expectError:  true,
		},
		{
			name:         "Lookback period too long",
			analysisType: "consistency",
			lookbackDays: 400,
			description:  "Should reject lookback period above maximum",
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set user context
			userCtx := context.WithValue(ctx, "user_id", userID)

			// This will fail because WorkoutPatternTool doesn't exist yet
			// That's expected for TDD - we write the test first, then implement
			tool := service.GetWorkoutPatternTool()
			if tool == nil {
				t.Skip("WorkoutPatternTool not implemented yet (expected for TDD)")
			}

			request := chatbot.WorkoutPatternRequest{
				AnalysisType: tc.analysisType,
				LookbackDays: tc.lookbackDays,
			}

			result, err := tool.AnalyzePattern(userCtx, request)

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
					if result.AnalysisType != tc.analysisType {
						t.Errorf("expected analysis type %s, got %s", tc.analysisType, result.AnalysisType)
					}
					if result.LookbackDays != tc.lookbackDays {
						t.Errorf("expected lookback days %d, got %d", tc.lookbackDays, result.LookbackDays)
					}
					if result.Summary == "" {
						t.Error("expected non-empty summary")
					}
					if len(result.Insights) == 0 {
						t.Error("expected insights in result")
					}
					if len(result.Recommendations) == 0 {
						t.Error("expected recommendations in result")
					}
					if result.MetricsData == nil {
						t.Error("expected metrics data in result")
					}

					// Validate specific analysis types
					switch tc.analysisType {
					case "consistency":
						// Should have consistency metrics
						if _, hasFrequency := result.MetricsData["workout_frequency"]; !hasFrequency {
							t.Error("consistency analysis should include workout frequency")
						}
						if result.Score == nil {
							t.Error("consistency analysis should include a score")
						}
					case "progressive_overload":
						// Should have progression metrics
						if _, hasProgression := result.MetricsData["progression_rate"]; !hasProgression {
							t.Error("progressive overload analysis should include progression rate")
						}
					case "muscle_balance":
						// Should have muscle group distribution
						if _, hasBalance := result.MetricsData["muscle_distribution"]; !hasBalance {
							t.Error("muscle balance analysis should include muscle distribution")
						}
					case "recovery_time":
						// Should have recovery metrics
						if _, hasRecovery := result.MetricsData["average_recovery_time"]; !hasRecovery {
							t.Error("recovery analysis should include average recovery time")
						}
					case "plateau_detection":
						// Should have plateau information
						if _, hasPlateaus := result.MetricsData["detected_plateaus"]; !hasPlateaus {
							t.Error("plateau detection should include detected plateaus")
						}
					case "workout_variety":
						// Should have variety metrics
						if _, hasVariety := result.MetricsData["exercise_variety_score"]; !hasVariety {
							t.Error("workout variety analysis should include variety score")
						}
					}
				}
			}
		})
	}
}

// Test specific pattern analysis scenarios with known data
func TestWorkoutPatternTool_SpecificPatterns(t *testing.T) {
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

	// Setup minimal data for specific test scenarios
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'Chest exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert exercise: %v", err)
	}

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetWorkoutPatternTool()
	if tool == nil {
		t.Skip("WorkoutPatternTool not implemented yet (expected for TDD)")
	}

	// Test scenario with no workout data
	t.Run("No workout history", func(t *testing.T) {
		userCtx := context.WithValue(ctx, "user_id", userID)
		result, err := tool.AnalyzePattern(userCtx, chatbot.WorkoutPatternRequest{
			AnalysisType: "consistency",
			LookbackDays: 30,
		})

		if err != nil {
			t.Errorf("unexpected error for no data scenario: %v", err)
		}
		if result != nil {
			// Should handle empty data gracefully
			if result.Summary == "" {
				t.Error("expected summary even with no data")
			}
			if len(result.Recommendations) == 0 {
				t.Error("expected recommendations for user with no history")
			}
		}
	})
}

// Test that pattern analysis properly isolates user data
func TestWorkoutPatternTool_UserIsolation(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert two test users with different patterns
	var user1ID, user2ID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("consistent-user"), "Consistent User").Scan(&user1ID)
	if err != nil {
		t.Fatalf("Failed to insert user1: %v", err)
	}

	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("inconsistent-user"), "Inconsistent User").Scan(&user2ID)
	if err != nil {
		t.Fatalf("Failed to insert user2: %v", err)
	}

	// Setup exercise
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'Chest exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert exercise: %v", err)
	}

	// User1 has consistent workout pattern - every other day for past 20 days
	baseDate := time.Now().AddDate(0, 0, -20)
	for i := 0; i < 10; i++ { // 10 workouts over 20 days = very consistent
		workoutDate := baseDate.AddDate(0, 0, i*2)
		_, err = db.ReadWrite.ExecContext(ctx, `
			INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at, difficulty_rating) VALUES
			(?, ?, ?, ?, 4)
		`, user1ID, workoutDate.Format("2006-01-02"), workoutDate.Format(time.RFC3339),
			workoutDate.Add(time.Hour).Format(time.RFC3339))
		if err != nil {
			t.Fatalf("Failed to insert consistent workout: %v", err)
		}

		_, err = db.ReadWrite.ExecContext(ctx, `
			INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
			(?, ?, 1)
		`, user1ID, workoutDate.Format("2006-01-02"))
		if err != nil {
			t.Fatalf("Failed to insert workout exercise: %v", err)
		}

		_, err = db.ReadWrite.ExecContext(ctx, `
			INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
			(?, ?, 1, 1, 80.0, 8, 10, 10)
		`, user1ID, workoutDate.Format("2006-01-02"))
		if err != nil {
			t.Fatalf("Failed to insert exercise set: %v", err)
		}
	}

	// User2 has inconsistent pattern - only 3 workouts in past 20 days
	inconsistentDates := []int{-18, -12, -3} // Sporadic workouts
	for _, dayOffset := range inconsistentDates {
		workoutDate := time.Now().AddDate(0, 0, dayOffset)
		_, err = db.ReadWrite.ExecContext(ctx, `
			INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at, difficulty_rating) VALUES
			(?, ?, ?, ?, 3)
		`, user2ID, workoutDate.Format("2006-01-02"), workoutDate.Format(time.RFC3339),
			workoutDate.Add(time.Hour).Format(time.RFC3339))
		if err != nil {
			t.Fatalf("Failed to insert inconsistent workout: %v", err)
		}

		_, err = db.ReadWrite.ExecContext(ctx, `
			INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
			(?, ?, 1)
		`, user2ID, workoutDate.Format("2006-01-02"))
		if err != nil {
			t.Fatalf("Failed to insert workout exercise: %v", err)
		}

		_, err = db.ReadWrite.ExecContext(ctx, `
			INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
			(?, ?, 1, 1, 75.0, 8, 10, 8)
		`, user2ID, workoutDate.Format("2006-01-02"))
		if err != nil {
			t.Fatalf("Failed to insert exercise set: %v", err)
		}
	}

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetWorkoutPatternTool()
	if tool == nil {
		t.Skip("WorkoutPatternTool not implemented yet (expected for TDD)")
	}

	// Analyze consistency for both users
	user1Ctx := context.WithValue(ctx, "user_id", user1ID)
	result1, err := tool.AnalyzePattern(user1Ctx, chatbot.WorkoutPatternRequest{
		AnalysisType: "consistency",
		LookbackDays: 30,
	})

	user2Ctx := context.WithValue(ctx, "user_id", user2ID)
	result2, err := tool.AnalyzePattern(user2Ctx, chatbot.WorkoutPatternRequest{
		AnalysisType: "consistency",
		LookbackDays: 30,
	})

	if err != nil {
		t.Errorf("unexpected errors: %v", err)
	}

	// Results should be different - consistent user should have better consistency score
	if result1 != nil && result2 != nil {
		if result1.Score != nil && result2.Score != nil {
			if *result1.Score <= *result2.Score {
				t.Errorf("expected consistent user (score: %f) to have higher consistency than inconsistent user (score: %f)",
					*result1.Score, *result2.Score)
			}
		}

		// Verify that data is properly isolated
		if workoutCount1, ok := result1.MetricsData["workout_count"]; ok {
			if workoutCount2, ok := result2.MetricsData["workout_count"]; ok {
				// User1 should have 10 workouts, User2 should have 3 workouts
				if workoutCount1 == workoutCount2 {
					t.Error("workout counts should be different for users with different patterns")
				}
			}
		}
	}
}

// Test pattern analysis security and validation
func TestWorkoutPatternTool_SecurityValidation(t *testing.T) {
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
	tool := service.GetWorkoutPatternTool()
	if tool == nil {
		t.Skip("WorkoutPatternTool not implemented yet (expected for TDD)")
	}

	// Test without user context - should fail safely
	t.Run("Missing user context", func(t *testing.T) {
		_, err := tool.AnalyzePattern(ctx, chatbot.WorkoutPatternRequest{
			AnalysisType: "consistency",
			LookbackDays: 30,
		})
		if err == nil {
			t.Error("expected error when user context is missing")
		}
	})

	maliciousInputs := []struct {
		name    string
		request chatbot.WorkoutPatternRequest
	}{
		{
			"Script injection in analysis type",
			chatbot.WorkoutPatternRequest{
				AnalysisType: "<script>alert('xss')</script>",
				LookbackDays: 30,
			},
		},
		{
			"SQL injection in analysis type",
			chatbot.WorkoutPatternRequest{
				AnalysisType: "'; DROP TABLE workout_sessions; --",
				LookbackDays: 30,
			},
		},
		{
			"Extremely large lookback days",
			chatbot.WorkoutPatternRequest{
				AnalysisType: "consistency",
				LookbackDays: 999999,
			},
		},
		{
			"Negative lookback days",
			chatbot.WorkoutPatternRequest{
				AnalysisType: "consistency",
				LookbackDays: -30,
			},
		},
	}

	for _, tc := range maliciousInputs {
		t.Run(tc.name, func(t *testing.T) {
			userCtx := context.WithValue(ctx, "user_id", userID)
			result, err := tool.AnalyzePattern(userCtx, tc.request)

			// Should either return an error or safely handle the input
			// Should never cause a panic or database corruption
			if err == nil && result != nil {
				// If it succeeds, the result should be safe
				if result.AnalysisType == tc.request.AnalysisType {
					t.Errorf("potentially unsafe input was processed as-is: %s", tc.request.AnalysisType)
				}
			}
		})
	}
}
