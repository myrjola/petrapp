package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_UpdateExercise_PreservesExerciseSets(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	// Insert a user first
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create workout service
	svc := service.NewService(db, logger, "")

	// Insert necessary muscle groups
	for _, group := range []string{"Quads", "Glutes", "Hamstrings", "Core"} {
		if err = tryInsertMuscleGroup(ctx, t, db, group); err != nil {
			t.Fatalf("Failed to insert muscle group: %v", err)
		}
	}

	// 1. Create a test exercise directly in the database
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Test Exercise", "lower", "Test description", 5, 10)
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
		(workout_exercise_id, set_number, weight_kg, target_value)
		VALUES (?, ?, ?, ?)`,
		weID, 1, 50.0, 12)
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

	repMin, repMax := 5, 10
	updatedExercise := domain.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds not needed for this test.
		ID:                    exerciseID,
		Name:                  "Updated Test Exercise",
		Category:              domain.CategoryLower,
		ExerciseType:          domain.ExerciseTypeWeighted,
		DescriptionMarkdown:   "Updated test description",
		PrimaryMuscleGroups:   []string{"Quads", "Glutes"},
		SecondaryMuscleGroups: []string{"Hamstrings", "Core"},
		RepMin:                &repMin,
		RepMax:                &repMax,
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

func Test_UpdateExercise_RejectsInvalidExercise(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	svc := service.NewService(db, logger, "")

	invalid := domain.Exercise{ //nolint:exhaustruct // intentionally invalid: empty name.
		ID:           1,
		Category:     domain.CategoryUpper,
		ExerciseType: domain.ExerciseTypeWeighted,
	}
	err = svc.UpdateExercise(ctx, invalid)
	if err == nil {
		t.Fatal("UpdateExercise() = nil, want ValidationError")
	}
	var ve domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("UpdateExercise() error is not a ValidationError: %v", err)
	}
	if ve.Message != "Name is required." {
		t.Errorf("message = %q, want %q", ve.Message, "Name is required.")
	}
}

func Test_GenerateExercise_RejectsEmptyName(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	svc := service.NewService(db, logger, "")

	_, err = svc.GenerateExercise(ctx, "")
	if err == nil {
		t.Fatal("GenerateExercise(\"\") = nil, want ValidationError")
	}
	var ve domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("GenerateExercise(\"\") error is not a ValidationError: %v", err)
	}
}

// Test_AddExercise tests adding a new exercise to a workout.
func Test_AddExercise(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

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
	svc := service.NewService(db, logger, "")

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
		(workout_exercise_id, set_number, weight_kg, target_value)
		VALUES (?, ?, ?, ?)`,
		weID1, 1, 50.0, 12)
	if err != nil {
		t.Fatalf("Failed to insert exercise set: %v", err)
	}

	// Create a context with the user ID for service calls
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Test adding a new exercise
	t.Run("Add exercise to existing workout", func(t *testing.T) {
		t.Parallel()

		// Count exercise sets before adding
		var countBefore int
		var errCount error
		countBefore, errCount = countExerciseSetsForWorkout(ctx, t, svc, today)
		if errCount != nil {
			t.Fatalf("Failed to count exercise sets before update: %v", errCount)
		}

		// Add exercise 2 to the workout. AddExercise returns the
		// workout_exercise.id of the new slot so handlers can redirect
		// straight to the new exercise's detail page.
		newSlotID, errAdd := svc.AddExercise(ctx, today, exercise2ID)
		if errAdd != nil {
			t.Fatalf("Failed to add exercise to workout: %v", errAdd)
		}
		if newSlotID == 0 {
			t.Errorf("expected non-zero new slot ID, got 0")
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

		// Verify the returned slot ID belongs to the slot we just added —
		// same workout_exercise.id, mapped to exercise 2.
		got, errGet := svc.GetSession(ctx, today)
		if errGet != nil {
			t.Fatalf("GetSession after add: %v", errGet)
		}
		var foundSlot bool
		for _, es := range got.ExerciseSets {
			if es.ID == newSlotID {
				if es.Exercise.ID != exercise2ID {
					t.Errorf("slot %d has exercise %d, want %d", newSlotID, es.Exercise.ID, exercise2ID)
				}
				foundSlot = true
				break
			}
		}
		if !foundSlot {
			t.Errorf("returned slot ID %d not present in session", newSlotID)
		}
	})

	// Test adding an exercise that's already in the workout
	t.Run("Add duplicate exercise to workout", func(t *testing.T) {
		t.Parallel()

		// Try to add exercise 1 which is already in the workout
		_, errAdd := svc.AddExercise(ctx, today, exercise1ID)
		if errAdd == nil {
			t.Error("Expected error when adding duplicate exercise, but got nil")
		}
	})

	// Test adding an exercise to a non-existent workout (should return a
	// user-facing ValidationError).
	t.Run("Add exercise to non-existent workout", func(t *testing.T) {
		t.Parallel()

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

		// Add exercise to the non-existent workout - should fail with a
		// ValidationError carrying a user-facing message.
		_, errAdd := svc.AddExercise(ctx, futureDate, exercise1ID)
		if errAdd == nil {
			t.Fatal("Expected error when adding exercise to non-existent workout, but got nil")
		}
		var ve domain.ValidationError
		if !errors.As(errAdd, &ve) {
			t.Fatalf("Expected ValidationError, got %T: %v", errAdd, errAdd)
		}
		wantMsg := "This day has no planned workout. Schedule one from the home page first."
		if ve.Message != wantMsg {
			t.Errorf("ValidationError.Message = %q, want %q", ve.Message, wantMsg)
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

// Test_AddExercise_UsesMostRecentHistoricalWeight verifies findHistoricalSets
// returns the most recent prior session's sets, not the oldest match.
func Test_AddExercise_UsesMostRecentHistoricalWeight(t *testing.T) {
	t.Parallel()

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
		[]byte("history-user"), "History User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := service.NewService(db, logger, "")

	for _, group := range []string{"Quads", "Glutes", "Hamstrings", "Core"} {
		if err = tryInsertMuscleGroup(ctx, t, db, group); err != nil {
			t.Fatalf("insert muscle group: %v", err)
		}
	}
	exerciseID, err := createTestExercise(ctx, t, db, "Squat", "lower")
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	today := time.Now()
	insertHistoricalSession := func(daysAgo int, weight float64) {
		t.Helper()
		dateStr := today.AddDate(0, 0, -daysAgo).Format("2006-01-02")
		if _, err = db.ReadWrite.ExecContext(ctx,
			"INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)",
			userID, dateStr); err != nil {
			t.Fatalf("insert session %d days ago: %v", daysAgo, err)
		}
		var weID int
		if err = db.ReadWrite.QueryRowContext(ctx,
			`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
			 VALUES (?, ?, ?) RETURNING id`,
			userID, dateStr, exerciseID).Scan(&weID); err != nil {
			t.Fatalf("insert workout_exercise %d days ago: %v", daysAgo, err)
		}
		if _, err = db.ReadWrite.ExecContext(ctx,
			`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
			 VALUES (?, 1, ?, 12)`,
			weID, weight); err != nil {
			t.Fatalf("insert exercise_set %d days ago: %v", daysAgo, err)
		}
	}

	// Older session at 60kg, newer session at 80kg.
	insertHistoricalSession(56, 60.0)
	insertHistoricalSession(7, 80.0)

	// Create today's workout session (empty — AddExercise requires it to exist).
	if _, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)",
		userID, today.Format("2006-01-02")); err != nil {
		t.Fatalf("insert today's session: %v", err)
	}

	if _, err = svc.AddExercise(ctx, today, exerciseID); err != nil {
		t.Fatalf("AddExercise: %v", err)
	}

	session, err := svc.GetSession(ctx, today)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var seededWeight *float64
	for _, es := range session.ExerciseSets {
		if es.Exercise.ID == exerciseID && len(es.Sets) > 0 {
			seededWeight = es.Sets[0].WeightKg
			break
		}
	}
	if seededWeight == nil {
		t.Fatalf("expected exercise %d to be seeded with weight, got nil", exerciseID)
	}
	if *seededWeight != 80.0 {
		t.Errorf("expected most recent historical weight 80kg, got %v", *seededWeight)
	}
}

// Test_AddExercise_TimeBased_NoHistory_SeedsDefaultStartingSeconds verifies
// that adding a time-based exercise with no usable history produces three
// sets pre-seeded with the exercise's DefaultStartingSeconds rather than
// the rep-based defaultTargetValue or zero sets.
//
// Regression: the time_based premigration in PR #87 dropped historical
// exercise_sets rows for plank but kept the workout_exercise slots. The
// resulting empty (but non-nil) history slice slipped past the
// `historicalSets != nil` check and persisted zero sets, so the user saw
// a blank exercise page when adding or swapping to plank.
func Test_AddExercise_TimeBased_NoHistory_SeedsDefaultStartingSeconds(t *testing.T) {
	t.Parallel()

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
		[]byte("plank-user"), "Plank User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := service.NewService(db, logger, "")

	if err = tryInsertMuscleGroup(ctx, t, db, "Core"); err != nil {
		t.Fatalf("insert muscle group: %v", err)
	}

	const startingSeconds = 30
	var plankID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO exercises (name, category, exercise_type, default_starting_seconds, description_markdown)
		 VALUES (?, 'full_body', 'time_based', ?, '') RETURNING id`,
		"TestPlankAdd", startingSeconds).Scan(&plankID)
	if err != nil {
		t.Fatalf("insert plank exercise: %v", err)
	}

	today := time.Now()
	dateStr := today.Format("2006-01-02")

	// Simulate the post-premigration state: a historical workout_exercise slot
	// for plank exists but its exercise_sets rows were dropped.
	historicalDate := today.AddDate(0, 0, -7).Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)",
		userID, historicalDate); err != nil {
		t.Fatalf("insert historical session: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?)`,
		userID, historicalDate, plankID); err != nil {
		t.Fatalf("insert orphaned workout_exercise: %v", err)
	}

	// Today's empty session.
	if _, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)",
		userID, dateStr); err != nil {
		t.Fatalf("insert today's session: %v", err)
	}

	if _, err = svc.AddExercise(ctx, today, plankID); err != nil {
		t.Fatalf("AddExercise: %v", err)
	}

	session, err := svc.GetSession(ctx, today)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var plankSets []domain.Set
	for _, es := range session.ExerciseSets {
		if es.Exercise.ID == plankID {
			plankSets = es.Sets
			break
		}
	}
	if len(plankSets) != 3 {
		t.Fatalf("expected 3 plank sets seeded, got %d", len(plankSets))
	}
	for i, set := range plankSets {
		if set.TargetValue != startingSeconds {
			t.Errorf("set %d TargetValue: want %d, got %d", i, startingSeconds, set.TargetValue)
		}
		if set.WeightKg != nil {
			t.Errorf("set %d WeightKg: want nil for time_based, got %v", i, *set.WeightKg)
		}
	}
}

// Test_SwapExercise_ToTimeBased_NoHistory_SeedsDefaultStartingSeconds verifies
// that swapping into a time-based exercise (e.g. plank) without usable
// history produces sets pre-seeded with DefaultStartingSeconds, not the
// previous exercise's rep target. Regression for the same orphaned-history
// case as Test_AddExercise_TimeBased_NoHistory_SeedsDefaultStartingSeconds.
func Test_SwapExercise_ToTimeBased_NoHistory_SeedsDefaultStartingSeconds(t *testing.T) {
	t.Parallel()

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
		[]byte("swap-user"), "Swap User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := service.NewService(db, logger, "")

	for _, group := range []string{"Core", "Quads"} {
		if err = tryInsertMuscleGroup(ctx, t, db, group); err != nil {
			t.Fatalf("insert muscle group: %v", err)
		}
	}

	squatID, err := createTestExercise(ctx, t, db, "Squat", "lower")
	if err != nil {
		t.Fatalf("create squat: %v", err)
	}

	const startingSeconds = 30
	var plankID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO exercises (name, category, exercise_type, default_starting_seconds, description_markdown)
		 VALUES (?, 'full_body', 'time_based', ?, '') RETURNING id`,
		"TestPlankSwap", startingSeconds).Scan(&plankID)
	if err != nil {
		t.Fatalf("insert plank: %v", err)
	}

	today := time.Now()
	dateStr := today.Format("2006-01-02")

	// Simulate orphaned historical plank row from the premigration.
	historicalDate := today.AddDate(0, 0, -7).Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)",
		userID, historicalDate); err != nil {
		t.Fatalf("insert historical session: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?)`,
		userID, historicalDate, plankID); err != nil {
		t.Fatalf("insert orphaned workout_exercise: %v", err)
	}

	// Today's session with squat occupying a slot.
	if _, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)",
		userID, dateStr); err != nil {
		t.Fatalf("insert today's session: %v", err)
	}
	var squatSlotID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		 VALUES (?, ?, ?) RETURNING id`,
		userID, dateStr, squatID).Scan(&squatSlotID)
	if err != nil {
		t.Fatalf("insert squat slot: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if _, err = db.ReadWrite.ExecContext(ctx,
			`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
			 VALUES (?, ?, ?, ?)`,
			squatSlotID, i, 60.0, 5); err != nil {
			t.Fatalf("insert squat set: %v", err)
		}
	}

	if err = svc.SwapExercise(ctx, today, squatSlotID, plankID); err != nil {
		t.Fatalf("SwapExercise: %v", err)
	}

	session, err := svc.GetSession(ctx, today)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var plankSets []domain.Set
	for _, es := range session.ExerciseSets {
		if es.ID == squatSlotID {
			if es.Exercise.ID != plankID {
				t.Fatalf("slot %d still maps to exercise %d, want %d", es.ID, es.Exercise.ID, plankID)
			}
			plankSets = es.Sets
			break
		}
	}
	if len(plankSets) != 3 {
		t.Fatalf("expected 3 plank sets, got %d", len(plankSets))
	}
	for i, set := range plankSets {
		if set.TargetValue != startingSeconds {
			t.Errorf("set %d TargetValue: want %d, got %d", i, startingSeconds, set.TargetValue)
		}
		if set.WeightKg != nil {
			t.Errorf("set %d WeightKg: want nil for time_based, got %v", i, *set.WeightKg)
		}
	}
}

// Test_AddExercise_DerivesTargetValueFromPeriodization is a regression test for
// PR #89: AddExercise used to produce target_value=8 (the old repsHypertrophy
// constant) for every new exercise regardless of the session's periodization and
// the exercise's per-exercise rep window.
//
// For Deadlift (rep_min=3, rep_max=6):
//   - Hypertrophy → DeriveScheme(3, 6, Hypertrophy).TargetReps == 6, TargetSets == 3
//   - Strength    → DeriveScheme(3, 6, Strength).TargetReps    == 3, TargetSets == 4
func Test_AddExercise_DerivesTargetValueFromPeriodization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		periodization   string
		wantTargetValue int
		wantSetCount    int
	}{
		{"hypertrophy", "hypertrophy", 6, 3},
		{"strength", "strength", 3, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each subtest builds its own in-memory database so the parallel
			// runs don't contend on the same SQLite handle.
			t.Parallel()

			ctx := t.Context()
			logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
			db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
			if err != nil {
				t.Fatalf("create db: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })

			var userID int
			err = db.ReadWrite.QueryRowContext(ctx,
				"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
				[]byte("derive-user-"+tt.periodization), "Derive User").Scan(&userID)
			if err != nil {
				t.Fatalf("insert user: %v", err)
			}
			ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
			ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

			svc := service.NewService(db, logger, "")

			// Deadlift-like exercise: rep_min=3, rep_max=6.
			var deadliftID int
			err = db.ReadWrite.QueryRowContext(ctx,
				`INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max)
				 VALUES (?, 'lower', '', 3, 6) RETURNING id`,
				"Test Deadlift Derive").Scan(&deadliftID)
			if err != nil {
				t.Fatalf("insert deadlift: %v", err)
			}

			sessionDate := time.Now()
			sessionDateStr := sessionDate.Format("2006-01-02")

			if _, err = db.ReadWrite.ExecContext(ctx,
				"INSERT INTO workout_sessions (user_id, workout_date, periodization_type) VALUES (?, ?, ?)",
				userID, sessionDateStr, tt.periodization); err != nil {
				t.Fatalf("insert session: %v", err)
			}

			if _, err = svc.AddExercise(ctx, sessionDate, deadliftID); err != nil {
				t.Fatalf("AddExercise: %v", err)
			}

			session, err := svc.GetSession(ctx, sessionDate)
			if err != nil {
				t.Fatalf("GetSession: %v", err)
			}

			var sets []domain.Set
			for _, es := range session.ExerciseSets {
				if es.Exercise.ID == deadliftID {
					sets = es.Sets
					break
				}
			}
			if len(sets) != tt.wantSetCount {
				t.Errorf("%s: want %d sets, got %d", tt.name, tt.wantSetCount, len(sets))
			}
			for i, s := range sets {
				if s.TargetValue != tt.wantTargetValue {
					t.Errorf("%s set[%d] TargetValue: want %d, got %d",
						tt.name, i, tt.wantTargetValue, s.TargetValue)
				}
			}
		})
	}
}

// Test_ReplaceExerciseInSession_DerivesTargetValueFromPeriodization is a regression
// test for PR #89: SwapExercise used to copy TargetValue verbatim from the most
// recent historical session, which spread the wrong rep count across periodization
// boundaries (e.g. a Calf Raise swapped in on a Strength week kept 20 reps from
// a Hypertrophy-week history instead of getting the correct 10 reps).
//
// For Deadlift (rep_min=3, rep_max=6):
//   - Hypertrophy swap → TargetValue == 6, TargetSets == 3
//   - Historical weight (80kg) is preserved; periodization-wrong TargetValue (3) is overridden.
func Test_ReplaceExerciseInSession_DerivesTargetValueFromPeriodization(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("swap-derive-user"), "Swap Derive User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := service.NewService(db, logger, "")

	if err = tryInsertMuscleGroup(ctx, t, db, "Quads"); err != nil {
		t.Fatalf("insert muscle group: %v", err)
	}

	// Squat occupies today's slot.
	squatID, err := createTestExercise(ctx, t, db, "Squat Swap Derive", "lower")
	if err != nil {
		t.Fatalf("create squat: %v", err)
	}

	// Deadlift-like exercise: rep_min=3, rep_max=6.
	var deadliftID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max)
		 VALUES (?, 'lower', '', 3, 6) RETURNING id`,
		"Test Deadlift Swap Derive").Scan(&deadliftID)
	if err != nil {
		t.Fatalf("insert deadlift: %v", err)
	}

	today := time.Now()
	dateStr := today.Format("2006-01-02")

	// Historical strength session 7 days ago: deadlift with 3 reps at 80kg.
	// After the swap onto a hypertrophy session, the weight (80kg) should be
	// preserved but TargetValue should become 6 (hypertrophy target), not 3.
	histDateStr := today.AddDate(0, 0, -7).Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, periodization_type) VALUES (?, ?, 'strength')",
		userID, histDateStr); err != nil {
		t.Fatalf("insert hist session: %v", err)
	}
	var weHistID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id`,
		userID, histDateStr, deadliftID).Scan(&weHistID)
	if err != nil {
		t.Fatalf("insert hist workout_exercise: %v", err)
	}
	for i := 1; i <= 4; i++ {
		if _, err = db.ReadWrite.ExecContext(ctx,
			`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
			 VALUES (?, ?, 80.0, 3)`,
			weHistID, i); err != nil {
			t.Fatalf("insert hist set %d: %v", i, err)
		}
	}

	// Today's hypertrophy session with squat in slot.
	if _, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, periodization_type) VALUES (?, ?, 'hypertrophy')",
		userID, dateStr); err != nil {
		t.Fatalf("insert today's session: %v", err)
	}
	var squatSlotID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?) RETURNING id`,
		userID, dateStr, squatID).Scan(&squatSlotID)
	if err != nil {
		t.Fatalf("insert squat slot: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if _, err = db.ReadWrite.ExecContext(ctx,
			`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
			 VALUES (?, ?, 60.0, 10)`,
			squatSlotID, i); err != nil {
			t.Fatalf("insert squat set %d: %v", i, err)
		}
	}

	// Swap squat → deadlift on the hypertrophy session.
	if err = svc.SwapExercise(ctx, today, squatSlotID, deadliftID); err != nil {
		t.Fatalf("SwapExercise: %v", err)
	}

	session, err := svc.GetSession(ctx, today)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var sets []domain.Set
	for _, es := range session.ExerciseSets {
		if es.ID == squatSlotID {
			if es.Exercise.ID != deadliftID {
				t.Fatalf("slot still maps to exercise %d, want deadlift %d", es.Exercise.ID, deadliftID)
			}
			sets = es.Sets
			break
		}
	}

	// Hypertrophy: DeriveScheme(3, 6, Hypertrophy) → 6 reps, 3 sets.
	const wantTargetValue = 6
	const wantSetCount = 3
	if len(sets) != wantSetCount {
		t.Errorf("want %d sets, got %d", wantSetCount, len(sets))
	}
	for i, s := range sets {
		if s.TargetValue != wantTargetValue {
			t.Errorf("set[%d] TargetValue: want %d (hypertrophy), got %d", i, wantTargetValue, s.TargetValue)
		}
		// Historical weight (80kg from the strength session) must be preserved.
		if s.WeightKg == nil || *s.WeightKg != 80.0 {
			var w float64
			if s.WeightKg != nil {
				w = *s.WeightKg
			}
			t.Errorf("set[%d] WeightKg: want 80.0 (from strength history), got %v", i, w)
		}
	}
}

func Test_ListSwapCandidates_ExcludesSessionExercises(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	plan, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	sessions := plan.Sessions[:]
	var (
		session     domain.Session
		workoutDate time.Time
		found       bool
	)
	for _, s := range sessions {
		if len(s.ExerciseSets) > 0 {
			session, workoutDate, found = s, s.Date, true
			break
		}
	}
	if !found {
		t.Fatal("no workout day with exercises found in this week")
	}
	weID := session.ExerciseSets[0].ID

	current, candidates, err := svc.ListSwapCandidates(ctx, workoutDate, weID, "")
	if err != nil {
		t.Fatalf("ListSwapCandidates: %v", err)
	}
	if current.ID != session.ExerciseSets[0].Exercise.ID {
		t.Errorf("current.ID = %d, want %d", current.ID, session.ExerciseSets[0].Exercise.ID)
	}

	sessionIDs := make(map[int]bool, len(session.ExerciseSets))
	for _, es := range session.ExerciseSets {
		sessionIDs[es.Exercise.ID] = true
	}
	for _, c := range candidates {
		if sessionIDs[c.ID] {
			t.Errorf("candidate %q (id=%d) is already used by the session", c.Name, c.ID)
		}
	}
	if len(candidates) == 0 {
		t.Error("got 0 candidates; seed pool should leave at least one swap option after exclusions")
	}

	for i := 1; i < len(candidates); i++ {
		prev := domain.SwapSimilarityScore(current, candidates[i-1])
		cur := domain.SwapSimilarityScore(current, candidates[i])
		if cur > prev {
			t.Errorf("candidates not sorted by similarity desc at index %d: prev=%d cur=%d", i, prev, cur)
			break
		}
	}
}

func Test_ListSwapCandidates_FiltersByQuery(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)
	plan, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	sessions := plan.Sessions[:]
	var (
		session     domain.Session
		workoutDate time.Time
		found       bool
	)
	for _, s := range sessions {
		if len(s.ExerciseSets) > 0 {
			session, workoutDate, found = s, s.Date, true
			break
		}
	}
	if !found {
		t.Fatal("no workout day with exercises found")
	}
	weID := session.ExerciseSets[0].ID

	var candidates []domain.Exercise
	_, candidates, err = svc.ListSwapCandidates(ctx, workoutDate, weID, "zzzzzzz")
	if err != nil {
		t.Fatalf("ListSwapCandidates(no-match): %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("query 'zzzzzzz' returned %d candidates; want 0", len(candidates))
	}

	var all []domain.Exercise
	_, all, err = svc.ListSwapCandidates(ctx, workoutDate, weID, "")
	if err != nil {
		t.Fatalf("ListSwapCandidates(unfiltered): %v", err)
	}
	var eFiltered []domain.Exercise
	_, eFiltered, err = svc.ListSwapCandidates(ctx, workoutDate, weID, "e")
	if err != nil {
		t.Fatalf("ListSwapCandidates('e'): %v", err)
	}
	if len(eFiltered) > len(all) {
		t.Errorf("'e'-filtered = %d, unfiltered = %d - filter cannot grow the set", len(eFiltered), len(all))
	}
	for _, c := range eFiltered {
		if !strings.Contains(strings.ToLower(c.Name), "e") {
			t.Errorf("'e'-filtered candidate %q does not contain 'e'", c.Name)
		}
	}
}
