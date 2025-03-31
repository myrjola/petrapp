package workout_test

import (
	"context"
	"errors"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/workout"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func Test_UpdateExercise_PreservesExerciseSets(t *testing.T) {
	// Setup context
	ctx := t.Context()

	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		AddSource:   false,
		ReplaceAttr: nil,
	}))

	// Setup test database
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Create a test user ID
	userID := []byte("test-user-id")

	// Insert a user first
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO users (id, display_name) VALUES (?, ?)",
		userID, "Test User")
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create workout service
	svc := workout.NewService(db, logger, "")

	// Insert necessary muscle groups
	for _, group := range []string{"Quads", "Glutes", "Hamstrings", "Core"} {
		if err = tryInsertMuscleGroup(ctx, t, db, group); err != nil {
			t.Fatalf("Failed to insert muscle group: %v", err)
		}
	}

	// 1. Create a test exercise directly in the database
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown) VALUES (?, ?, ?)",
		"Test Exercise", "lower", "Test description")
	if err != nil {
		t.Fatalf("Failed to insert exercise: %v", err)
	}

	// Get the exercise ID
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx,
		"SELECT id FROM exercises WHERE name = ?", "Test Exercise").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("Failed to get exercise ID: %v", err)
	}

	// Insert exercise muscle groups
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary) VALUES (?, ?, ?)",
		exerciseID, "Quads", 1)
	if err != nil {
		t.Fatalf("Failed to insert primary muscle group: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary) VALUES (?, ?, ?)",
		exerciseID, "Hamstrings", 0)
	if err != nil {
		t.Fatalf("Failed to insert secondary muscle group: %v", err)
	}

	// 2. Create a workout session
	today := time.Now()
	dateStr := today.Format("2006-01-02")

	// Insert workout session
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)",
		userID, dateStr)
	if err != nil {
		t.Fatalf("Failed to insert workout session: %v", err)
	}

	// Insert exercise set
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets 
		(workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, dateStr, exerciseID, 1, 50.0, 8, 12)
	if err != nil {
		t.Fatalf("Failed to insert exercise set: %v", err)
	}

	// 3. Verify exercise set exists
	countBefore, err := countExerciseSets(t, db, exerciseID)
	if err != nil {
		t.Fatalf("Failed to count exercise sets before update: %v", err)
	}
	if countBefore == 0 {
		t.Fatal("Expected at least one exercise set before update")
	}

	// 4. Update the exercise
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	updatedExercise := workout.Exercise{
		ID:                    exerciseID,
		Name:                  "Updated Test Exercise",
		Category:              workout.CategoryLower,
		DescriptionMarkdown:   "Updated test description",
		PrimaryMuscleGroups:   []string{"Quads", "Glutes"},
		SecondaryMuscleGroups: []string{"Hamstrings", "Core"},
	}

	err = svc.UpdateExercise(ctx, updatedExercise)
	if err != nil {
		t.Fatalf("Failed to update exercise: %v", err)
	}

	// 5. Verify exercise sets still exist after update
	countAfter, err := countExerciseSets(t, db, exerciseID)
	if err != nil {
		t.Fatalf("Failed to count exercise sets after update: %v", err)
	}

	if countAfter == 0 {
		t.Error("BUG DETECTED: All exercise sets were deleted when editing the exercise")
	}

	if countAfter != countBefore {
		t.Errorf("Expected %d exercise sets after update, got %d", countBefore, countAfter)
	}
}

// Helper function to count exercise sets for a given exercise.
func countExerciseSets(t *testing.T, db *sqlite.Database, exerciseID int) (int, error) {
	t.Helper()
	var count int
	err := db.ReadOnly.QueryRow(
		"SELECT COUNT(*) FROM exercise_sets WHERE exercise_id = ?",
		exerciseID,
	).Scan(&count)
	return count, err
}

// Try to insert a muscle group, ignoring if it already exists.
func tryInsertMuscleGroup(ctx context.Context, t *testing.T, db *sqlite.Database, name string) error {
	t.Helper()
	_, err := db.ReadWrite.ExecContext(ctx, "INSERT INTO muscle_groups (name) VALUES (?)", name)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		// Muscle group already exists, which is fine
		return nil
	}
	return err
}

// Test_AddExercise tests adding a new exercise to a workout.
func Test_AddExercise(t *testing.T) {
	// Setup context
	ctx := t.Context()

	// Setup logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		AddSource:   false,
		ReplaceAttr: nil,
	}))

	// Setup test database
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Create a test user ID
	userID := []byte("test-user-id")

	// Insert a user first
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO users (id, display_name) VALUES (?, ?)",
		userID, "Test User")
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create workout service
	svc := workout.NewService(db, logger, "")

	// Insert necessary muscle groups
	for _, group := range []string{"Quads", "Glutes", "Hamstrings", "Core"} {
		if err = tryInsertMuscleGroup(ctx, t, db, group); err != nil {
			t.Fatalf("Failed to insert muscle group: %v", err)
		}
	}

	// Create test exercises
	exercise1ID, err := createTestExercise(ctx, t, db, "Test Exercise 1", "lower")
	if err != nil {
		t.Fatalf("Failed to create test exercise 1: %v", err)
	}

	exercise2ID, err := createTestExercise(ctx, t, db, "Test Exercise 2", "upper")
	if err != nil {
		t.Fatalf("Failed to create test exercise 2: %v", err)
	}

	// Create a workout session with exercise 1
	today := time.Now()
	dateStr := today.Format("2006-01-02")

	// Insert workout session
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)",
		userID, dateStr)
	if err != nil {
		t.Fatalf("Failed to insert workout session: %v", err)
	}

	// Insert exercise set for exercise 1
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets 
		(workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, dateStr, exercise1ID, 1, 50.0, 8, 12)
	if err != nil {
		t.Fatalf("Failed to insert exercise set: %v", err)
	}

	// Create a context with the user ID for service calls
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Test adding a new exercise
	t.Run("Add exercise to existing workout", func(t *testing.T) {
		// Count exercise sets before adding
		var countBefore int
		var errCount error
		countBefore, errCount = countExerciseSetsForWorkout(ctx, t, svc, today)
		if errCount != nil {
			t.Fatalf("Failed to count exercise sets before update: %v", errCount)
		}

		// Add exercise 2 to the workout
		err = svc.AddExercise(ctx, today, exercise2ID)
		if err != nil {
			t.Fatalf("Failed to add exercise to workout: %v", err)
		}

		// Count exercise sets after adding
		var countAfter int
		countAfter, errCount = countExerciseSetsForWorkout(ctx, t, svc, today)
		if errCount != nil {
			t.Fatalf("Failed to count exercise sets after update: %v", errCount)
		}

		// We expect more exercise sets after adding
		if countAfter <= countBefore {
			t.Errorf("Expected more exercise sets after adding an exercise, but got %d before and %d after",
				countBefore, countAfter)
		}

		// Verify the added exercise exists in the workout
		var exists bool
		var errExists error
		exists, errExists = exerciseExistsInWorkout(ctx, t, svc, today, exercise2ID)
		if errExists != nil {
			t.Fatalf("Failed to check if exercise exists in workout: %v", errExists)
		}
		if !exists {
			t.Error("Exercise was not added to the workout")
		}
	})

	// Test adding an exercise that's already in the workout
	t.Run("Add duplicate exercise to workout", func(t *testing.T) {
		// Try to add exercise 1 which is already in the workout
		err = svc.AddExercise(ctx, today, exercise1ID)
		if err == nil {
			t.Error("Expected error when adding duplicate exercise, but got nil")
		}
	})

	// Test adding an exercise to a non-existent workout (should return an error)
	t.Run("Add exercise to non-existent workout", func(t *testing.T) {
		// Set a future date for a workout that doesn't exist yet
		futureDate := today.AddDate(0, 0, 7) // 1 week in the future

		// Verify the workout doesn't exist yet
		var existsCheck bool
		var errExists error
		existsCheck, errExists = workoutExistsForDate(ctx, t, svc, futureDate)
		if errExists != nil {
			t.Fatalf("Failed to check if workout exists: %v", errExists)
		}
		if existsCheck {
			t.Fatalf("Workout already exists for future date, can't test error case")
		}

		// Add exercise to the non-existent workout - should fail
		err = svc.AddExercise(ctx, futureDate, exercise1ID)
		if err == nil {
			t.Error("Expected error when adding exercise to non-existent workout, but got nil")
		}

		// Verify workout was NOT created
		existsCheck, errExists = workoutExistsForDate(ctx, t, svc, futureDate)
		if errExists != nil {
			t.Fatalf("Failed to check if workout was created: %v", errExists)
		}
		if existsCheck {
			t.Error("Workout was created for future date when it should not have been")
		}
	})
}

// Helper function to create a test exercise.
func createTestExercise(ctx context.Context, t *testing.T, db *sqlite.Database, name, category string) (int, error) {
	t.Helper()
	_, err := db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown) VALUES (?, ?, ?)",
		name, category, "Test description")
	if err != nil {
		return 0, err
	}

	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx,
		"SELECT id FROM exercises WHERE name = ?", name).Scan(&exerciseID)
	if err != nil {
		return 0, err
	}

	// Insert exercise muscle groups
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary) VALUES (?, ?, ?)",
		exerciseID, "Quads", 1)
	if err != nil {
		return 0, err
	}

	return exerciseID, nil
}

// Helper function to count exercise sets for a specific workout date.
func countExerciseSetsForWorkout(ctx context.Context, t *testing.T, svc *workout.Service, date time.Time) (int, error) {
	t.Helper()
	session, err := svc.GetSession(ctx, date)
	if err != nil {
		return 0, err
	}

	// Count total sets across all exercises
	totalSets := 0
	for _, exerciseSet := range session.ExerciseSets {
		totalSets += len(exerciseSet.Sets)
	}

	return totalSets, nil
}

// Helper function to check if an exercise exists in a workout.
func exerciseExistsInWorkout(
	ctx context.Context,
	t *testing.T,
	svc *workout.Service,
	date time.Time,
	exerciseID int,
) (bool, error) {
	t.Helper()
	session, err := svc.GetSession(ctx, date)
	if err != nil {
		return false, err
	}

	// Check if any exercise set has the specified exercise ID
	for _, exerciseSet := range session.ExerciseSets {
		if exerciseSet.Exercise.ID == exerciseID {
			return true, nil
		}
	}

	return false, nil
}

// Helper function to check if a workout exists for a date.
func workoutExistsForDate(ctx context.Context, t *testing.T, svc *workout.Service, date time.Time) (bool, error) {
	t.Helper()
	_, err := svc.GetSession(ctx, date)
	if err != nil {
		if errors.Is(err, workout.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
