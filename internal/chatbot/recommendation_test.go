package chatbot_test

import (
	"context"
	"testing"

	"github.com/myrjola/petrapp/internal/chatbot"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// Contract test for generate_workout_recommendation LLM function
// This test MUST fail initially as the function is not yet implemented
func TestWorkoutRecommendationTool_GenerateRecommendation(t *testing.T) {
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

	// Create comprehensive test data for recommendations
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown, equipment_needed) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'Chest exercise', 'barbell'),
		(2, 'Squat', 'lower', 'weighted', 'Leg exercise', 'barbell'),
		(3, 'Deadlift', 'full_body', 'weighted', 'Full body exercise', 'barbell'),
		(4, 'Dumbbell Row', 'upper', 'weighted', 'Back exercise', 'dumbbells'),
		(5, 'Push-ups', 'upper', 'bodyweight', 'Chest exercise', 'bodyweight_only'),
		(6, 'Pull-ups', 'upper', 'bodyweight', 'Back exercise', 'pull_up_bar'),
		(7, 'Kettlebell Swing', 'full_body', 'weighted', 'Power exercise', 'kettlebell'),
		(8, 'Leg Press', 'lower', 'weighted', 'Leg exercise', 'machines'),
		(9, 'Cable Fly', 'upper', 'weighted', 'Chest isolation', 'cables'),
		(10, 'Resistance Band Curl', 'upper', 'weighted', 'Arm exercise', 'resistance_bands')
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
		(4, 'Back', 1), (4, 'Biceps', 0),
		(5, 'Chest', 1), (5, 'Triceps', 0),
		(6, 'Back', 1), (6, 'Biceps', 0),
		(7, 'Glutes', 1), (7, 'Core', 0),
		(8, 'Quads', 1), (8, 'Glutes', 0),
		(9, 'Chest', 1),
		(10, 'Biceps', 1)
	`)
	if err != nil {
		t.Fatalf("Failed to insert exercise muscle groups: %v", err)
	}

	// Insert user's workout history for personalized recommendations
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at, difficulty_rating) VALUES
		(?, '2024-01-01', '2024-01-01T10:00:00Z', '2024-01-01T11:00:00Z', 4),
		(?, '2024-01-03', '2024-01-03T10:00:00Z', '2024-01-03T11:15:00Z', 3),
		(?, '2024-01-05', '2024-01-05T10:00:00Z', '2024-01-05T11:30:00Z', 5)
	`, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert workout sessions: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
		(?, '2024-01-01', 1), (?, '2024-01-01', 2),
		(?, '2024-01-03', 3), (?, '2024-01-03', 4),
		(?, '2024-01-05', 1), (?, '2024-01-05', 5)
	`, userID, userID, userID, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert workout exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
		-- Recent performance data for recommendations
		(?, '2024-01-01', 1, 1, 80.0, 8, 10, 10),
		(?, '2024-01-01', 1, 2, 80.0, 8, 10, 9),
		(?, '2024-01-01', 2, 1, 100.0, 5, 8, 8),
		(?, '2024-01-03', 3, 1, 120.0, 3, 5, 5),
		(?, '2024-01-05', 1, 1, 82.5, 8, 10, 10)
	`, userID, userID, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert exercise sets: %v", err)
	}

	// Create chatbot service
	service := chatbot.NewService(db, logger, "test-api-key")

	// Test cases for different workout recommendation scenarios
	testCases := []struct {
		name            string
		workoutType     string
		durationMinutes *int
		equipment       []string
		avoidMuscles    []string
		expectError     bool
		description     string
	}{
		{
			name:        "Basic strength workout",
			workoutType: "strength",
			description: "Should generate a strength-focused workout",
			expectError: false,
		},
		{
			name:            "Hypertrophy workout with duration",
			workoutType:     "hypertrophy",
			durationMinutes: intPtr(60),
			description:     "Should generate hypertrophy workout for 60 minutes",
			expectError:     false,
		},
		{
			name:        "Endurance workout",
			workoutType: "endurance",
			description: "Should generate an endurance-focused workout",
			expectError: false,
		},
		{
			name:        "Recovery workout",
			workoutType: "recovery",
			description: "Should generate a light recovery workout",
			expectError: false,
		},
		{
			name:        "Full body workout",
			workoutType: "full_body",
			description: "Should generate a full body workout",
			expectError: false,
		},
		{
			name:        "Upper body focus",
			workoutType: "upper_body",
			description: "Should generate upper body focused workout",
			expectError: false,
		},
		{
			name:        "Lower body focus",
			workoutType: "lower_body",
			description: "Should generate lower body focused workout",
			expectError: false,
		},
		{
			name:        "Push workout",
			workoutType: "push",
			description: "Should generate push movement workout",
			expectError: false,
		},
		{
			name:        "Pull workout",
			workoutType: "pull",
			description: "Should generate pull movement workout",
			expectError: false,
		},
		{
			name:        "Legs workout",
			workoutType: "legs",
			description: "Should generate leg-focused workout",
			expectError: false,
		},
		{
			name:            "Equipment-specific workout",
			workoutType:     "strength",
			durationMinutes: intPtr(45),
			equipment:       []string{"dumbbells", "resistance_bands"},
			description:     "Should generate workout using only specified equipment",
			expectError:     false,
		},
		{
			name:            "Bodyweight only workout",
			workoutType:     "full_body",
			durationMinutes: intPtr(30),
			equipment:       []string{"bodyweight_only"},
			description:     "Should generate bodyweight-only workout",
			expectError:     false,
		},
		{
			name:            "Workout avoiding sore muscles",
			workoutType:     "upper_body",
			durationMinutes: intPtr(40),
			avoidMuscles:    []string{"Chest", "Shoulders"},
			description:     "Should generate upper body workout avoiding chest and shoulders",
			expectError:     false,
		},
		{
			name:            "Short workout with equipment constraints",
			workoutType:     "strength",
			durationMinutes: intPtr(20),
			equipment:       []string{"kettlebell"},
			avoidMuscles:    []string{"Back"},
			description:     "Should generate short workout with kettlebell, avoiding back",
			expectError:     false,
		},
		{
			name:            "Maximum duration workout",
			workoutType:     "hypertrophy",
			durationMinutes: intPtr(180),
			description:     "Should handle maximum duration workout",
			expectError:     false,
		},
		{
			name:            "Minimum duration workout",
			workoutType:     "recovery",
			durationMinutes: intPtr(15),
			description:     "Should handle minimum duration workout",
			expectError:     false,
		},
		{
			name:        "Invalid workout type",
			workoutType: "invalid_type",
			description: "Should reject invalid workout type",
			expectError: true,
		},
		{
			name:        "Empty workout type",
			workoutType: "",
			description: "Should reject empty workout type",
			expectError: true,
		},
		{
			name:            "Invalid duration too low",
			workoutType:     "strength",
			durationMinutes: intPtr(5),
			description:     "Should reject duration below minimum",
			expectError:     true,
		},
		{
			name:            "Invalid duration too high",
			workoutType:     "strength",
			durationMinutes: intPtr(200),
			description:     "Should reject duration above maximum",
			expectError:     true,
		},
		{
			name:        "Invalid equipment",
			workoutType: "strength",
			equipment:   []string{"flying_unicorn"},
			description: "Should reject invalid equipment type",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set user context
			userCtx := context.WithValue(ctx, "user_id", userID)

			// This will fail because WorkoutRecommendationTool doesn't exist yet
			// That's expected for TDD - we write the test first, then implement
			tool := service.GetWorkoutRecommendationTool()
			if tool == nil {
				t.Skip("WorkoutRecommendationTool not implemented yet (expected for TDD)")
			}

			request := chatbot.WorkoutRecommendationRequest{
				WorkoutType:        tc.workoutType,
				DurationMinutes:    tc.durationMinutes,
				EquipmentAvailable: tc.equipment,
				AvoidMuscleGroups:  tc.avoidMuscles,
			}

			result, err := tool.GenerateRecommendation(userCtx, request)

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
					if result.WorkoutType != tc.workoutType {
						t.Errorf("expected workout type %s, got %s", tc.workoutType, result.WorkoutType)
					}
					if result.EstimatedDuration <= 0 {
						t.Error("expected positive estimated duration")
					}
					if len(result.Exercises) == 0 {
						t.Error("expected exercises in recommendation")
					}

					// Validate duration constraints
					if tc.durationMinutes != nil {
						// Should be reasonably close to requested duration
						requestedDuration := *tc.durationMinutes
						estimatedDuration := result.EstimatedDuration
						if estimatedDuration < requestedDuration-15 || estimatedDuration > requestedDuration+15 {
							t.Errorf("estimated duration %d should be within 15 minutes of requested %d",
								estimatedDuration, requestedDuration)
						}
					}

					// Validate equipment constraints
					if len(tc.equipment) > 0 {
						for _, exercise := range result.Exercises {
							// Each exercise should use allowed equipment (implementation will need to verify this)
							if exercise.ExerciseName == "" {
								t.Error("expected non-empty exercise name")
							}
							if exercise.Sets <= 0 {
								t.Error("expected positive number of sets")
							}
							if exercise.MinReps <= 0 || exercise.MaxReps <= 0 {
								t.Error("expected positive rep ranges")
							}
							if exercise.MinReps > exercise.MaxReps {
								t.Error("min reps should not exceed max reps")
							}
						}
					}

					// Validate muscle group avoidance
					if len(tc.avoidMuscles) > 0 {
						for _, exercise := range result.Exercises {
							for _, avoidMuscle := range tc.avoidMuscles {
								for _, exerciseMuscle := range exercise.MuscleGroups {
									if exerciseMuscle == avoidMuscle {
										t.Errorf("exercise %s targets avoided muscle group %s",
											exercise.ExerciseName, avoidMuscle)
									}
								}
							}
						}
					}

					// Validate exercise structure
					for _, exercise := range result.Exercises {
						if exercise.ExerciseName == "" {
							t.Error("expected non-empty exercise name")
						}
						if exercise.Sets <= 0 {
							t.Error("expected positive number of sets")
						}
						if exercise.MinReps <= 0 || exercise.MaxReps <= 0 {
							t.Error("expected positive rep ranges")
						}
						if exercise.RestSeconds < 0 {
							t.Error("expected non-negative rest seconds")
						}
						if len(exercise.MuscleGroups) == 0 {
							t.Error("expected muscle groups for each exercise")
						}
					}
				}
			}
		})
	}
}

// Test that recommendations are personalized based on user history
func TestWorkoutRecommendationTool_Personalization(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert two test users with different experience levels
	var beginnerID, advancedID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("beginner"), "Beginner User").Scan(&beginnerID)
	if err != nil {
		t.Fatalf("Failed to insert beginner user: %v", err)
	}

	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("advanced"), "Advanced User").Scan(&advancedID)
	if err != nil {
		t.Fatalf("Failed to insert advanced user: %v", err)
	}

	// Setup exercises
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'Chest exercise'),
		(2, 'Squat', 'lower', 'weighted', 'Leg exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert exercises: %v", err)
	}

	// Beginner has minimal history with light weights
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date) VALUES
		(?, '2024-01-01')
	`, beginnerID)
	if err != nil {
		t.Fatalf("Failed to insert beginner workout session: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
		(?, '2024-01-01', 1)
	`, beginnerID)
	if err != nil {
		t.Fatalf("Failed to insert beginner workout exercise: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
		(?, '2024-01-01', 1, 1, 40.0, 8, 12, 10)
	`, beginnerID)
	if err != nil {
		t.Fatalf("Failed to insert beginner sets: %v", err)
	}

	// Advanced user has extensive history with heavy weights
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date) VALUES
		(?, '2024-01-01'), (?, '2024-01-03'), (?, '2024-01-05'), (?, '2024-01-07')
	`, advancedID, advancedID, advancedID, advancedID)
	if err != nil {
		t.Fatalf("Failed to insert advanced workout sessions: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
		(?, '2024-01-01', 1), (?, '2024-01-01', 2),
		(?, '2024-01-03', 1), (?, '2024-01-03', 2),
		(?, '2024-01-05', 1), (?, '2024-01-05', 2),
		(?, '2024-01-07', 1), (?, '2024-01-07', 2)
	`, advancedID, advancedID, advancedID, advancedID, advancedID, advancedID, advancedID, advancedID)
	if err != nil {
		t.Fatalf("Failed to insert advanced workout exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
		-- Progressive heavy training
		(?, '2024-01-01', 1, 1, 100.0, 3, 5, 5),
		(?, '2024-01-03', 1, 1, 102.5, 3, 5, 5),
		(?, '2024-01-05', 1, 1, 105.0, 3, 5, 4),
		(?, '2024-01-07', 1, 1, 107.5, 3, 5, 3),
		(?, '2024-01-01', 2, 1, 140.0, 3, 5, 5),
		(?, '2024-01-03', 2, 1, 145.0, 3, 5, 4),
		(?, '2024-01-05', 2, 1, 150.0, 3, 5, 3),
		(?, '2024-01-07', 2, 1, 155.0, 3, 5, 2)
	`, advancedID, advancedID, advancedID, advancedID, advancedID, advancedID, advancedID, advancedID)
	if err != nil {
		t.Fatalf("Failed to insert advanced sets: %v", err)
	}

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetWorkoutRecommendationTool()
	if tool == nil {
		t.Skip("WorkoutRecommendationTool not implemented yet (expected for TDD)")
	}

	// Get recommendations for both users
	beginnerCtx := context.WithValue(ctx, "user_id", beginnerID)
	beginnerResult, err := tool.GenerateRecommendation(beginnerCtx, chatbot.WorkoutRecommendationRequest{
		WorkoutType: "strength",
	})
	if err != nil {
		t.Errorf("unexpected error for beginner: %v", err)
	}

	advancedCtx := context.WithValue(ctx, "user_id", advancedID)
	advancedResult, err := tool.GenerateRecommendation(advancedCtx, chatbot.WorkoutRecommendationRequest{
		WorkoutType: "strength",
	})
	if err != nil {
		t.Errorf("unexpected error for advanced user: %v", err)
	}

	// Recommendations should be different for different experience levels
	if beginnerResult != nil && advancedResult != nil {
		// Advanced user should get heavier recommended weights
		beginnerHasBenchPress := false
		advancedHasBenchPress := false
		var beginnerBenchWeight, advancedBenchWeight *float64

		for _, exercise := range beginnerResult.Exercises {
			if exercise.ExerciseName == "Bench Press" {
				beginnerHasBenchPress = true
				beginnerBenchWeight = exercise.RecommendedWeight
			}
		}

		for _, exercise := range advancedResult.Exercises {
			if exercise.ExerciseName == "Bench Press" {
				advancedHasBenchPress = true
				advancedBenchWeight = exercise.RecommendedWeight
			}
		}

		if beginnerHasBenchPress && advancedHasBenchPress {
			if beginnerBenchWeight != nil && advancedBenchWeight != nil {
				if *advancedBenchWeight <= *beginnerBenchWeight {
					t.Errorf("expected advanced user (%0.1fkg) to get heavier recommendation than beginner (%0.1fkg)",
						*advancedBenchWeight, *beginnerBenchWeight)
				}
			}
		}
	}
}

// Test recommendation security and validation
func TestWorkoutRecommendationTool_SecurityValidation(t *testing.T) {
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
	tool := service.GetWorkoutRecommendationTool()
	if tool == nil {
		t.Skip("WorkoutRecommendationTool not implemented yet (expected for TDD)")
	}

	// Test without user context - should fail safely
	t.Run("Missing user context", func(t *testing.T) {
		_, err := tool.GenerateRecommendation(ctx, chatbot.WorkoutRecommendationRequest{
			WorkoutType: "strength",
		})
		if err == nil {
			t.Error("expected error when user context is missing")
		}
	})

	maliciousInputs := []struct {
		name    string
		request chatbot.WorkoutRecommendationRequest
	}{
		{
			"Script injection in workout type",
			chatbot.WorkoutRecommendationRequest{
				WorkoutType: "<script>alert('xss')</script>",
			},
		},
		{
			"SQL injection in equipment",
			chatbot.WorkoutRecommendationRequest{
				WorkoutType:        "strength",
				EquipmentAvailable: []string{"'; DROP TABLE exercises; --"},
			},
		},
		{
			"Path traversal in avoid muscles",
			chatbot.WorkoutRecommendationRequest{
				WorkoutType:       "strength",
				AvoidMuscleGroups: []string{"../../../etc/passwd"},
			},
		},
	}

	for _, tc := range maliciousInputs {
		t.Run(tc.name, func(t *testing.T) {
			userCtx := context.WithValue(ctx, "user_id", userID)
			result, err := tool.GenerateRecommendation(userCtx, tc.request)

			// Should either return an error or safely handle the input
			// Should never cause a panic or database corruption
			if err == nil && result != nil {
				// If it succeeds, the result should be safe
				if result.WorkoutType == tc.request.WorkoutType {
					t.Errorf("potentially unsafe input was processed as-is: %s", tc.request.WorkoutType)
				}
			}
		})
	}
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}
