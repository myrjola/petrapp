package workout_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"github.com/myrjola/petrapp/internal/workout"
)

func setupTestService(t *testing.T) (context.Context, *workout.Service) {
	t.Helper()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Set preferences: Mon, Wed, Fri at 60 min.
	svc := workout.NewService(db, logger, "")
	if err = svc.SaveUserPreferences(ctx, workout.Preferences{ //nolint:exhaustruct // Rest days intentionally omitted.
		MondayMinutes:    60,
		WednesdayMinutes: 60,
		FridayMinutes:    60,
	}); err != nil {
		t.Fatalf("save preferences: %v", err)
	}
	return ctx, svc
}

func Test_ResolveWeeklySchedule_GeneratesFullWeekOnFirstLoad(t *testing.T) {
	ctx, svc := setupTestService(t)

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	if len(sessions) != 7 {
		t.Fatalf("want 7 sessions (one per day), got %d", len(sessions))
	}

	// Scheduled days (Mon=0, Wed=2, Fri=4) must have exercises.
	for _, i := range []int{0, 2, 4} {
		if len(sessions[i].ExerciseSets) == 0 {
			t.Errorf("sessions[%d] (%s) must have exercise sets", i, sessions[i].Date.Weekday())
		}
	}

	// Rest days must be empty sessions.
	for _, i := range []int{1, 3, 5, 6} {
		if len(sessions[i].ExerciseSets) != 0 {
			t.Errorf("sessions[%d] (%s) must be empty (rest day)", i, sessions[i].Date.Weekday())
		}
	}
}

func Test_ResolveWeeklySchedule_DoesNotRegenerateExistingSessions(t *testing.T) {
	ctx, svc := setupTestService(t)

	sessions1, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("first ResolveWeeklySchedule: %v", err)
	}

	sessions2, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("second ResolveWeeklySchedule: %v", err)
	}

	// Same scheduled days must have the same exercise IDs on both calls.
	for _, i := range []int{0, 2, 4} {
		ids1 := extractExerciseIDs(sessions1[i])
		ids2 := extractExerciseIDs(sessions2[i])
		if !slices.Equal(ids1, ids2) {
			t.Errorf("sessions[%d] exercise IDs changed on second call: %v → %v", i, ids1, ids2)
		}
	}
}

func Test_GetSession_ReturnsErrNotFoundForUnplannedDate(t *testing.T) {
	ctx, svc := setupTestService(t)

	// Generate this week's plan.
	if _, err := svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Request a date in a different week.
	nextWeekTuesday := time.Now().AddDate(0, 0, 14)
	_, err := svc.GetSession(ctx, nextWeekTuesday)
	if !errors.Is(err, workout.ErrNotFound) {
		t.Errorf("want ErrNotFound for unplanned date, got %v", err)
	}
}

func Test_RegenerateWeeklyPlanIfUnstarted_RegeneratesFromEmptyWeek(t *testing.T) {
	ctx, svc := setupTestService(t) // Mon, Wed, Fri at 60 min — no sessions created yet

	// Call directly without seeding via ResolveWeeklySchedule first.
	if err := svc.RegenerateWeeklyPlanIfUnstarted(ctx); err != nil {
		t.Fatalf("RegenerateWeeklyPlanIfUnstarted on empty week: %v", err)
	}

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after empty-week regenerate: %v", err)
	}
	// Mon=0, Wed=2, Fri=4 must have exercises.
	for _, i := range []int{0, 2, 4} {
		if len(sessions[i].ExerciseSets) == 0 {
			t.Errorf("sessions[%d] (%s) must have exercise sets", i, sessions[i].Date.Weekday())
		}
	}
}

func Test_RegenerateWeeklyPlanIfUnstarted_RegeneratesWhenNoWorkoutStarted(t *testing.T) {
	ctx, svc := setupTestService(t) // Mon, Wed, Fri at 60 min

	// Generate the initial plan.
	if _, err := svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Change to Tue, Thu, Sat at 45 min.
	if err := svc.SaveUserPreferences(ctx, workout.Preferences{ //nolint:exhaustruct // Rest days intentionally omitted.
		TuesdayMinutes:  45,
		ThursdayMinutes: 45,
		SaturdayMinutes: 45,
	}); err != nil {
		t.Fatalf("save preferences: %v", err)
	}

	if err := svc.RegenerateWeeklyPlanIfUnstarted(ctx); err != nil {
		t.Fatalf("RegenerateWeeklyPlanIfUnstarted: %v", err)
	}

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after regenerate: %v", err)
	}

	// Tue=1, Thu=3, Sat=5 must have exercises; Mon=0, Wed=2, Fri=4, Sun=6 must be rest.
	for _, i := range []int{1, 3, 5} {
		if len(sessions[i].ExerciseSets) == 0 {
			t.Errorf("sessions[%d] (%s) must have exercise sets after preference change", i, sessions[i].Date.Weekday())
		}
	}
	for _, i := range []int{0, 2, 4, 6} {
		if len(sessions[i].ExerciseSets) != 0 {
			t.Errorf("sessions[%d] (%s) must be a rest day after preference change", i, sessions[i].Date.Weekday())
		}
	}
}

func Test_RegenerateWeeklyPlanIfUnstarted_SkipsRegenerateWhenWorkoutStarted(t *testing.T) {
	ctx, svc := setupTestService(t) // Mon, Wed, Fri at 60 min

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Start the first scheduled workout (Monday, index 0).
	if err = svc.StartSession(ctx, sessions[0].Date); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Change preferences to Tue, Thu, Sat.
	if err = svc.SaveUserPreferences(ctx, workout.Preferences{ //nolint:exhaustruct // Rest days intentionally omitted.
		TuesdayMinutes:  45,
		ThursdayMinutes: 45,
		SaturdayMinutes: 45,
	}); err != nil {
		t.Fatalf("save preferences: %v", err)
	}

	if err = svc.RegenerateWeeklyPlanIfUnstarted(ctx); err != nil {
		t.Fatalf("RegenerateWeeklyPlanIfUnstarted: %v", err)
	}

	sessions2, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after skip: %v", err)
	}

	// Monday (index 0) must still have exercises — the original plan was kept.
	if len(sessions2[0].ExerciseSets) == 0 {
		t.Error("sessions2[0] (Monday) must still have exercise sets; workout was already started")
	}

	// Tuesday must still be a rest day — the new preferences were not applied.
	if len(sessions2[1].ExerciseSets) != 0 {
		t.Error("sessions2[1] (Tuesday) must remain a rest day; new preferences must not be applied")
	}
}

func extractExerciseIDs(session workout.Session) []int {
	ids := make([]int, len(session.ExerciseSets))
	for i, es := range session.ExerciseSets {
		ids[i] = es.Exercise.ID
	}
	return ids
}

func Test_UpdateExercise_PreservesExerciseSets(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func(db *sqlite.Database) {
		_ = db.Close()
	}(db)

	// Insert a user first
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
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

	// Insert workout_exercise slot and one set hanging off it.
	var weID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id`,
		userID, dateStr, exerciseID).Scan(&weID)
	if err != nil {
		t.Fatalf("Failed to insert workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets
		(workout_exercise_id, set_number, weight_kg, min_reps, max_reps)
		VALUES (?, ?, ?, ?, ?)`,
		weID, 1, 50.0, 8, 12)
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
		ExerciseType:          workout.ExerciseTypeWeighted,
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
		`SELECT COUNT(*) FROM exercise_sets es
		 JOIN workout_exercise we ON we.id = es.workout_exercise_id
		 WHERE we.exercise_id = ?`,
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
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func(db *sqlite.Database) {
		_ = db.Close()
	}(db)

	// Create a test user ID
	webauthnUserID := []byte("test-user-id")

	// Insert a user first
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		webauthnUserID, "Test User").Scan(&userID)
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

	// Insert workout_exercise slot for exercise 1 with one set.
	var weID1 int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id`,
		userID, dateStr, exercise1ID).Scan(&weID1)
	if err != nil {
		t.Fatalf("Failed to insert workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets
		(workout_exercise_id, set_number, weight_kg, min_reps, max_reps)
		VALUES (?, ?, ?, ?, ?)`,
		weID1, 1, 50.0, 8, 12)
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

func Test_GenerateWorkout_PeriodizationTypeAlternatesAcrossSessions(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := workout.NewService(db, logger, "")

	// Save preferences with Mon, Wed, Fri as workout days.
	if err = svc.SaveUserPreferences(ctx, workout.Preferences{ //nolint:exhaustruct // Rest days intentionally omitted.
		MondayMinutes:    60,
		WednesdayMinutes: 60,
		FridayMinutes:    60,
	}); err != nil {
		t.Fatalf("save preferences: %v", err)
	}

	// Generate this week's plan and collect periodization types for all 3 workout days.
	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Collect periodization types for scheduled days (Mon=0, Wed=2, Fri=4).
	scheduledIndices := []int{0, 2, 4}
	types := make([]workout.PeriodizationType, len(scheduledIndices))
	for j, i := range scheduledIndices {
		types[j] = sessions[i].PeriodizationType
	}

	// Each consecutive session must alternate periodization type.
	for i := 1; i < len(types); i++ {
		if types[i] == types[i-1] {
			t.Errorf("sessions[%d] and sessions[%d] have the same periodization type %q; want alternating",
				i-1, i, types[i])
		}
	}
}

func Test_GetStartingWeight(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("sw-user"), "SW User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown) VALUES (?, ?, ?)",
		"Squat", "lower", "desc")
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Squat'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := workout.NewService(db, logger, "")

	today := time.Now()

	// No history: expect 0.
	got, err := svc.GetStartingWeight(ctx, exerciseID, today, workout.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight no history: %v", err)
	}
	if got != 0 {
		t.Errorf("no history: want 0, got %v", got)
	}

	// Insert a completed strength session 7 days ago. Set 1 ramps up from 95kg
	// (too_light), set 2 lands on 100kg (on_target), set 3 fails at 105kg
	// (too_heavy). The latest *successful* set is set 2 at 100kg.
	dateStr := today.AddDate(0, 0, -7).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, completed_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'strength')`,
		userID, dateStr)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var weHistID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id`,
		userID, dateStr, exerciseID).Scan(&weHistID)
	if err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number,
		 weight_kg, min_reps, max_reps, completed_reps, signal)
		 VALUES (?, 1, 95.0, 5, 5, 5, 'too_light'),
		        (?, 2, 100.0, 5, 5, 5, 'on_target'),
		        (?, 3, 105.0, 5, 5, 3, 'too_heavy')`,
		weHistID, weHistID, weHistID)
	if err != nil {
		t.Fatalf("insert sets: %v", err)
	}

	// Same periodization (strength → strength): the latest successful set (set 2 at
	// 100kg) carries over unchanged, ignoring the failed set 3.
	got, err = svc.GetStartingWeight(ctx, exerciseID, today, workout.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight with history: %v", err)
	}
	if got != 100.0 {
		t.Errorf("strength → strength: want 100.0, got %v", got)
	}

	// Cross-periodization (strength 5 reps → hypertrophy 8 reps): Epley conversion
	// 100 * (1 + 5/30) / (1 + 8/30) ≈ 92.1, rounded to 0.5 = 92.0.
	got, err = svc.GetStartingWeight(ctx, exerciseID, today, workout.PeriodizationHypertrophy)
	if err != nil {
		t.Fatalf("GetStartingWeight cross-periodization: %v", err)
	}
	if got != 92.0 {
		t.Errorf("strength → hypertrophy: want 92.0, got %v", got)
	}

	// Insert today's session with different set weights. The starting weight must
	// remain anchored to the historical session, regardless of today's sets.
	todayStr := today.Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, started_at) VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))",
		userID, todayStr)
	if err != nil {
		t.Fatalf("insert today's session: %v", err)
	}
	var weTodayID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id`,
		userID, todayStr, exerciseID).Scan(&weTodayID)
	if err != nil {
		t.Fatalf("insert today's workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number,
		 weight_kg, min_reps, max_reps, completed_reps, signal)
		 VALUES (?, 1, 75.0, 5, 5, 5, 'too_light'),
		        (?, 2, 80.0, 5, 5, 5, 'on_target')`,
		weTodayID, weTodayID)
	if err != nil {
		t.Fatalf("insert today's sets: %v", err)
	}

	got, err = svc.GetStartingWeight(ctx, exerciseID, today, workout.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight ignoring today: %v", err)
	}
	if got != 100.0 {
		t.Errorf("today ignored: want 100.0, got %v", got)
	}

	// Insert a more recent strength session 3 days ago where every set was
	// too_heavy. GetStartingWeight must skip it and fall back to the 7-days-ago
	// session's latest successful set (100kg).
	failDateStr := today.AddDate(0, 0, -3).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, completed_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'strength')`,
		userID, failDateStr)
	if err != nil {
		t.Fatalf("insert fail session: %v", err)
	}
	var weFailID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id`,
		userID, failDateStr, exerciseID).Scan(&weFailID)
	if err != nil {
		t.Fatalf("insert fail workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number,
		 weight_kg, min_reps, max_reps, completed_reps, signal)
		 VALUES (?, 1, 110.0, 5, 5, 3, 'too_heavy'),
		        (?, 2, 110.0, 5, 5, 2, 'too_heavy')`,
		weFailID, weFailID)
	if err != nil {
		t.Fatalf("insert fail sets: %v", err)
	}

	got, err = svc.GetStartingWeight(ctx, exerciseID, today, workout.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight fallback: %v", err)
	}
	if got != 100.0 {
		t.Errorf("fallback past too_heavy session: want 100.0, got %v", got)
	}
}

func Test_RecordSetCompletion(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("rsc-user"), "RSC User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Deadlift is pre-seeded by fixtures.sql; fetch its ID directly.
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Deadlift'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, started_at) VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))",
		userID, today)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var weID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id",
		userID, today, exerciseID).Scan(&weID)
	if err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number,
		 weight_kg, min_reps, max_reps)
		 VALUES (?, 1, 100.0, 5, 5)`,
		weID)
	if err != nil {
		t.Fatalf("insert set: %v", err)
	}

	svc := workout.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", today)

	if err = svc.RecordSetCompletion(ctx, date, weID, 0, workout.SignalOnTarget, 102.5, 5); err != nil {
		t.Fatalf("RecordSetCompletion: %v", err)
	}

	sess, err := svc.GetSession(ctx, date)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var es *workout.ExerciseSet
	for i := range sess.ExerciseSets {
		if sess.ExerciseSets[i].Exercise.ID == exerciseID {
			es = &sess.ExerciseSets[i]
			break
		}
	}
	if es == nil {
		t.Fatal("exercise not found in session")
	}

	set := es.Sets[0]
	if set.Signal == nil || *set.Signal != workout.SignalOnTarget {
		t.Errorf("signal: want on_target, got %v", set.Signal)
	}
	if set.WeightKg == nil || *set.WeightKg != 102.5 {
		t.Errorf("weight: want 102.5, got %v", set.WeightKg)
	}
	if set.CompletedReps == nil || *set.CompletedReps != 5 {
		t.Errorf("reps: want 5, got %v", set.CompletedReps)
	}
	if set.CompletedAt == nil {
		t.Error("completed_at: want non-nil")
	}
}

func Test_BuildProgression(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("bp-user"), "BP User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown) VALUES (?, ?, ?)",
		"OHP", "upper", "desc")
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'OHP'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	// Hypertrophy session (1 completed before this one).
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'hypertrophy')`,
		userID, today)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var weID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id",
		userID, today, exerciseID).Scan(&weID)
	if err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, min_reps, max_reps)
		 VALUES (?, 1, 40.0, 8, 8), (?, 2, 40.0, 8, 8), (?, 3, 40.0, 8, 8)`,
		weID, weID, weID)
	if err != nil {
		t.Fatalf("insert sets: %v", err)
	}

	svc := workout.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", today)

	// No history: starting weight 0, target 8 reps (hypertrophy).
	prog, err := svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression: %v", err)
	}
	target := prog.CurrentSet()
	if target.WeightKg != 0 {
		t.Errorf("first set weight: want 0, got %v", target.WeightKg)
	}
	if target.TargetReps != 8 {
		t.Errorf("first set reps: want 8, got %v", target.TargetReps)
	}

	// Record set 0 as TooLight at 0kg.
	if err = svc.RecordSetCompletion(ctx, date, weID, 0, workout.SignalTooLight, 0, 8); err != nil {
		t.Fatalf("RecordSetCompletion: %v", err)
	}

	// Rebuild: next set should be 0 + 2.5 = 2.5 kg.
	prog, err = svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression after set 1: %v", err)
	}
	target = prog.CurrentSet()
	if target.WeightKg != 2.5 {
		t.Errorf("second set weight: want 2.5, got %v", target.WeightKg)
	}
}

func Test_BuildProgression_CrossPeriodizationConversion(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("bp-x-user"), "BPX User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown) VALUES (?, ?, ?)",
		"Squat", "lower", "desc")
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Squat'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	// Prior strength session 7 days ago: completed first set 100 kg x 5 on target.
	prevStr := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, completed_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'strength')`,
		userID, prevStr)
	if err != nil {
		t.Fatalf("insert prev session: %v", err)
	}
	var wePrevID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id",
		userID, prevStr, exerciseID).Scan(&wePrevID)
	if err != nil {
		t.Fatalf("insert prev workout_exercise: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number,
		 weight_kg, min_reps, max_reps, completed_reps, signal)
		 VALUES (?, 1, 100.0, 5, 5, 5, 'on_target')`,
		wePrevID)
	if err != nil {
		t.Fatalf("insert prev set: %v", err)
	}

	// New hypertrophy session today.
	todayStr := time.Now().Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'hypertrophy')`,
		userID, todayStr)
	if err != nil {
		t.Fatalf("insert today session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?)",
		userID, todayStr, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}

	svc := workout.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", todayStr)

	prog, err := svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression: %v", err)
	}
	target := prog.CurrentSet()
	// Strength 100kg x5 → Hypertrophy 8 reps via Epley:
	// 100 * (1 + 5/30) / (1 + 8/30) ≈ 92.105, rounded to 0.5 = 92.0.
	if target.WeightKg != 92.0 {
		t.Errorf("first set weight: want 92.0, got %v", target.WeightKg)
	}
	if target.TargetReps != 8 {
		t.Errorf("first set reps: want 8, got %v", target.TargetReps)
	}
}
