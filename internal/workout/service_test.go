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

func Test_WeeklyMuscleGroupVolume_AggregatesPrimaryAndSecondary(t *testing.T) {
	ctx, svc := setupTestService(t)

	// Two synthetic exercises sharing a secondary muscle group so we can verify
	// both the primary (1.0/set) and secondary (0.5/set) weights and that
	// contributions accumulate across exercises.
	bench := workout.Exercise{ //nolint:exhaustruct // ID, Description etc. unused by the aggregator.
		Name:                  "Bench",
		PrimaryMuscleGroups:   []string{"Chest"},
		SecondaryMuscleGroups: []string{"Triceps", "Shoulders"},
	}
	dip := workout.Exercise{ //nolint:exhaustruct // ID, Description etc. unused by the aggregator.
		Name:                  "Dip",
		PrimaryMuscleGroups:   []string{"Triceps"},
		SecondaryMuscleGroups: []string{"Chest"},
	}

	completed := time.Now().UTC()
	completedSet := workout.Set{ //nolint:exhaustruct // Value and weight are not relevant for volume.
		TargetValue: 12,
		CompletedAt: &completed,
	}
	plannedSet := workout.Set{ //nolint:exhaustruct // CompletedAt nil → planned but not completed.
		TargetValue: 12,
	}

	benchSet := workout.ExerciseSet{ //nolint:exhaustruct // ID + WarmupCompletedAt are repository-managed.
		Exercise: bench,
		Sets:     []workout.Set{completedSet, completedSet, plannedSet},
	}
	dipSet := workout.ExerciseSet{ //nolint:exhaustruct // ID + WarmupCompletedAt are repository-managed.
		Exercise: dip,
		Sets:     []workout.Set{plannedSet, plannedSet},
	}
	sessions := []workout.Session{
		{ //nolint:exhaustruct // Date and timestamps are not relevant for the volume aggregator.
			ExerciseSets: []workout.ExerciseSet{benchSet, dipSet},
		},
	}

	got, err := svc.WeeklyMuscleGroupVolume(ctx, sessions)
	if err != nil {
		t.Fatalf("WeeklyMuscleGroupVolume: %v", err)
	}

	byName := make(map[string]workout.MuscleGroupVolume, len(got))
	for _, v := range got {
		byName[v.Name] = v
	}

	// Chest: primary on bench (3 sets) + secondary on dip (2 sets * 0.5)
	//        = 3 planned, 2 completed (bench had 2 completed); plus 1.0 secondary completed = 0
	// Bench: 3 planned (2 completed) primary  → planned 3.0, completed 2.0.
	// Dip: 2 planned secondary on chest      → planned 1.0, completed 0.0.
	// Total chest: planned 4.0, completed 2.0.
	if v := byName["Chest"]; v.PlannedLoad != 4.0 || v.CompletedLoad != 2.0 {
		t.Errorf("Chest: want planned=4.0 completed=2.0, got planned=%v completed=%v", v.PlannedLoad, v.CompletedLoad)
	}

	// Triceps: secondary on bench (3 sets * 0.5 = 1.5 planned, 2*0.5 = 1.0 completed)
	//          + primary on dip (2 sets planned, 0 completed).
	// Total: planned 3.5, completed 1.0.
	if v := byName["Triceps"]; v.PlannedLoad != 3.5 || v.CompletedLoad != 1.0 {
		t.Errorf("Triceps: want planned=3.5 completed=1.0, got planned=%v completed=%v",
			v.PlannedLoad, v.CompletedLoad)
	}

	// Shoulders: secondary on bench only (3 sets * 0.5 = 1.5 planned, 2 * 0.5 = 1.0 completed).
	if v := byName["Shoulders"]; v.PlannedLoad != 1.5 || v.CompletedLoad != 1.0 {
		t.Errorf("Shoulders: want planned=1.5 completed=1.0, got planned=%v completed=%v",
			v.PlannedLoad, v.CompletedLoad)
	}

	// Untouched group must appear with zero load (UI shows it as a flat bar).
	if v, ok := byName["Calves"]; !ok || v.PlannedLoad != 0 || v.CompletedLoad != 0 {
		t.Errorf("Calves: want zero-load entry, got %#v (present=%v)", v, ok)
	}

	// Targets are joined from muscle_group_weekly_targets seed (Chest=10, Calves not seeded).
	if v := byName["Chest"]; v.TargetSets != 10 {
		t.Errorf("Chest target: want 10, got %d", v.TargetSets)
	}
	if v := byName["Calves"]; v.TargetSets != 0 {
		t.Errorf("Calves target: want 0 (no seed), got %d", v.TargetSets)
	}

	// Result must list every muscle group exactly once, in alphabetical order.
	allNames, err := svc.ListMuscleGroups(ctx)
	if err != nil {
		t.Fatalf("ListMuscleGroups: %v", err)
	}
	if len(got) != len(allNames) {
		t.Errorf("result count: want %d (all groups), got %d", len(allNames), len(got))
	}
	for i, v := range got {
		if v.Name != allNames[i] {
			t.Errorf("result[%d].Name: want %q, got %q", i, allNames[i], v.Name)
		}
	}
}

func Test_WeeklyMuscleGroupVolume_EmptyWeek(t *testing.T) {
	ctx, svc := setupTestService(t)

	got, err := svc.WeeklyMuscleGroupVolume(ctx, nil)
	if err != nil {
		t.Fatalf("WeeklyMuscleGroupVolume on nil sessions: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("want one entry per muscle group even when sessions are empty, got 0")
	}
	for _, v := range got {
		if v.PlannedLoad != 0 || v.CompletedLoad != 0 {
			t.Errorf("%s: want zero load on empty week, got planned=%v completed=%v",
				v.Name, v.PlannedLoad, v.CompletedLoad)
		}
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
	updatedExercise := workout.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds not needed for this test.
		ID:                    exerciseID,
		Name:                  "Updated Test Exercise",
		Category:              workout.CategoryLower,
		ExerciseType:          workout.ExerciseTypeWeighted,
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
		var newSlotID int
		newSlotID, err = svc.AddExercise(ctx, today, exercise2ID)
		if err != nil {
			t.Fatalf("Failed to add exercise to workout: %v", err)
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
		// Try to add exercise 1 which is already in the workout
		_, err = svc.AddExercise(ctx, today, exercise1ID)
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
		_, err = svc.AddExercise(ctx, futureDate, exercise1ID)
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

// Test_AddExercise_UsesMostRecentHistoricalWeight verifies findHistoricalSets
// returns the most recent prior session's sets, not the oldest match.
func Test_AddExercise_UsesMostRecentHistoricalWeight(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("history-user"), "History User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := workout.NewService(db, logger, "")

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
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("plank-user"), "Plank User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := workout.NewService(db, logger, "")

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

	var plankSets []workout.Set
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
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("swap-user"), "Swap User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := workout.NewService(db, logger, "")

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

	var plankSets []workout.Set
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

// Helper function to create a test exercise.
func createTestExercise(ctx context.Context, t *testing.T, db *sqlite.Database, name, category string) (int, error) {
	t.Helper()
	_, err := db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		name, category, "Test description", 5, 10)
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
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Squat", "lower", "desc", 5, 8)
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
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, 1, 95.0, 5, 5, 'too_light'),
		        (?, 2, 100.0, 5, 5, 'on_target'),
		        (?, 3, 105.0, 5, 3, 'too_heavy')`,
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
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, 1, 75.0, 5, 5, 'too_light'),
		        (?, 2, 80.0, 5, 5, 'on_target')`,
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
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, 1, 110.0, 5, 3, 'too_heavy'),
		        (?, 2, 110.0, 5, 2, 'too_heavy')`,
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

// Test_GetStartingWeight_Assisted covers the assisted-exercise (negative weight)
// flow across periodization changes: an on-target -50 kg x5 strength set must
// translate into a more negative weight when the next session is hypertrophy
// (8 reps), since more reps require more machine assistance for the same
// relative intensity.
func Test_GetStartingWeight_Assisted(t *testing.T) {
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
		[]byte("sw-assisted-user"), "SW Assisted User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Assisted Test Exercise", "upper", "desc", 5, 8)
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx,
		"SELECT id FROM exercises WHERE name = 'Assisted Test Exercise'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := workout.NewService(db, logger, "")

	today := time.Now()

	// Insert a completed strength session 7 days ago at -50 kg x5, on target.
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
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, 1, -50.0, 5, 5, 'on_target')`,
		weHistID)
	if err != nil {
		t.Fatalf("insert sets: %v", err)
	}

	// Same periodization: -50 kg carries over unchanged.
	got, err := svc.GetStartingWeight(ctx, exerciseID, today, workout.PeriodizationStrength)
	if err != nil {
		t.Fatalf("GetStartingWeight strength→strength: %v", err)
	}
	if got != -50.0 {
		t.Errorf("assisted strength → strength: want -50.0, got %v", got)
	}

	// Cross-periodization (strength 5 reps → hypertrophy 8 reps): more reps
	// require more assistance, so the recommendation must be more negative.
	// -50 * (1 + 8/30) / (1 + 5/30) ≈ -54.29 → snaps to -54.5.
	got, err = svc.GetStartingWeight(ctx, exerciseID, today, workout.PeriodizationHypertrophy)
	if err != nil {
		t.Fatalf("GetStartingWeight strength→hypertrophy: %v", err)
	}
	if got != -54.5 {
		t.Errorf("assisted strength → hypertrophy: want -54.5, got %v", got)
	}
}

func Test_GetStartingSeconds(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("ts-user"), "TS User").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Insert a time_based exercise with default 30s.
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (name, category, exercise_type, default_starting_seconds, description_markdown)
		VALUES (?, ?, ?, ?, ?)`,
		"Test Plank", "upper", "time_based", 30, ""); err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	if err = db.ReadOnly.QueryRowContext(ctx,
		"SELECT id FROM exercises WHERE name = 'Test Plank'").Scan(&exerciseID); err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := workout.NewService(db, logger, "")
	today := time.Now()

	// Case 1: no history → fallback to default_starting_seconds.
	got, err := svc.GetStartingSeconds(ctx, exerciseID, today)
	if err != nil {
		t.Fatalf("no history: %v", err)
	}
	if got != 30 {
		t.Errorf("no history: want 30, got %d", got)
	}

	// Case 2: seed a successful session 2 days ago.
	twoDaysAgo := today.AddDate(0, 0, -2).Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, periodization_type)
		VALUES (?, ?, 'strength')`, userID, twoDaysAgo); err != nil {
		t.Fatalf("insert session 1: %v", err)
	}
	var weID1 int
	if err = db.ReadWrite.QueryRowContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		VALUES (?, ?, ?) RETURNING id`,
		userID, twoDaysAgo, exerciseID).Scan(&weID1); err != nil {
		t.Fatalf("insert workout_exercise 1: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets
			(workout_exercise_id, set_number, target_value, completed_value, completed_at, signal)
		VALUES (?, 1, 40, 40, '2026-05-05T12:00:00.000Z', 'on_target')`, weID1); err != nil {
		t.Fatalf("insert set 1: %v", err)
	}

	got, err = svc.GetStartingSeconds(ctx, exerciseID, today)
	if err != nil {
		t.Fatalf("with history: %v", err)
	}
	if got != 40 {
		t.Errorf("with history: want 40, got %d", got)
	}

	// Case 3: more recent too_heavy session should be skipped.
	oneDayAgo := today.AddDate(0, 0, -1).Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, periodization_type)
		VALUES (?, ?, 'strength')`, userID, oneDayAgo); err != nil {
		t.Fatalf("insert session 2: %v", err)
	}
	var weID2 int
	if err = db.ReadWrite.QueryRowContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		VALUES (?, ?, ?) RETURNING id`,
		userID, oneDayAgo, exerciseID).Scan(&weID2); err != nil {
		t.Fatalf("insert workout_exercise 2: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_sets
			(workout_exercise_id, set_number, target_value, completed_value, completed_at, signal)
		VALUES (?, 1, 50, 50, '2026-05-06T12:00:00.000Z', 'too_heavy')`, weID2); err != nil {
		t.Fatalf("insert set 2: %v", err)
	}

	got, err = svc.GetStartingSeconds(ctx, exerciseID, today)
	if err != nil {
		t.Fatalf("skip too_heavy: %v", err)
	}
	if got != 40 {
		t.Errorf("skip too_heavy: want 40 (older successful), got %d", got)
	}
}

func Test_BuildTimedProgression(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("btp-user"), "BTP User").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (name, category, exercise_type, default_starting_seconds, description_markdown)
		VALUES (?, ?, ?, ?, ?)`,
		"Test Plank BTP", "upper", "time_based", 30, ""); err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	if err = db.ReadOnly.QueryRowContext(ctx,
		"SELECT id FROM exercises WHERE name = 'Test Plank BTP'").Scan(&exerciseID); err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := workout.NewService(db, logger, "")

	today := time.Now().Format("2006-01-02")
	todayTime, _ := time.Parse("2006-01-02", today)

	// Seed today's session with the exercise but no completed sets yet.
	if _, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, periodization_type)
		VALUES (?, ?, 'strength')`, userID, today); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var weID int
	if err = db.ReadWrite.QueryRowContext(ctx, `
		INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		VALUES (?, ?, ?) RETURNING id`,
		userID, today, exerciseID).Scan(&weID); err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	// Seed three planned sets with target_value=30, no completion yet.
	for i := 1; i <= 3; i++ {
		if _, err = db.ReadWrite.ExecContext(ctx, `
			INSERT INTO exercise_sets (workout_exercise_id, set_number, target_value)
			VALUES (?, ?, 30)`, weID, i); err != nil {
			t.Fatalf("insert set %d: %v", i, err)
		}
	}

	// Case 1: no completed sets in this session → first set returns starting seconds (default 30).
	progression, err := svc.BuildTimedProgression(ctx, todayTime, exerciseID)
	if err != nil {
		t.Fatalf("BuildTimedProgression no completion: %v", err)
	}
	if got := progression.CurrentSet().TargetSeconds; got != 30 {
		t.Errorf("first set: got %d, want 30 (default)", got)
	}
	if got := progression.SetsCompleted(); got != 0 {
		t.Errorf("first set: SetsCompleted = %d, want 0", got)
	}

	// Case 2: complete set 1 with too_light → second set should be 35s.
	if _, err = db.ReadWrite.ExecContext(ctx, `
		UPDATE exercise_sets
		SET completed_value = 30, signal = 'too_light'
		WHERE workout_exercise_id = ? AND set_number = 1`, weID); err != nil {
		t.Fatalf("update set 1: %v", err)
	}

	progression, err = svc.BuildTimedProgression(ctx, todayTime, exerciseID)
	if err != nil {
		t.Fatalf("BuildTimedProgression after set 1: %v", err)
	}
	if got := progression.CurrentSet().TargetSeconds; got != 35 {
		t.Errorf("after too_light: got %d, want 35", got)
	}
	if got := progression.SetsCompleted(); got != 1 {
		t.Errorf("after set 1: SetsCompleted = %d, want 1", got)
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
		 weight_kg, target_value)
		 VALUES (?, 1, 100.0, 5)`,
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
	if set.CompletedValue == nil || *set.CompletedValue != 5 {
		t.Errorf("completed value: want 5, got %v", set.CompletedValue)
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
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"OHP", "upper", "desc", 5, 8)
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
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 1, 40.0, 8), (?, 2, 40.0, 8), (?, 3, 40.0, 8)`,
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

	// Rebuild: next set should be 0 + 1 = 1 kg (1kg increment in dumbbell range).
	prog, err = svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression after set 1: %v", err)
	}
	target = prog.CurrentSet()
	if target.WeightKg != 1.0 {
		t.Errorf("second set weight: want 1.0, got %v", target.WeightKg)
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
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Squat", "lower", "desc", 5, 8)
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
		 weight_kg, target_value, completed_value, signal)
		 VALUES (?, 1, 100.0, 5, 5, 'on_target')`,
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

// Test_AddExercise_DerivesTargetValueFromPeriodization is a regression test for
// PR #89: AddExercise used to produce target_value=8 (the old repsHypertrophy
// constant) for every new exercise regardless of the session's periodization and
// the exercise's per-exercise rep window.
//
// For Deadlift (rep_min=3, rep_max=6):
//   - Hypertrophy → DeriveScheme(3, 6, Hypertrophy).TargetReps == 6, TargetSets == 3
//   - Strength    → DeriveScheme(3, 6, Strength).TargetReps    == 3, TargetSets == 4
func Test_AddExercise_DerivesTargetValueFromPeriodization(t *testing.T) {
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
		[]byte("derive-user"), "Derive User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := workout.NewService(db, logger, "")

	// Deadlift-like exercise: rep_min=3, rep_max=6.
	var deadliftID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max)
		 VALUES (?, 'lower', '', 3, 6) RETURNING id`,
		"Test Deadlift Derive").Scan(&deadliftID)
	if err != nil {
		t.Fatalf("insert deadlift: %v", err)
	}

	today := time.Now()
	dateStr := today.Format("2006-01-02")

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
			// Create a fresh dated session for each sub-test using distinct dates.
			sessionDate := today.AddDate(0, 0, 0)
			sessionDateStr := dateStr
			if tt.periodization == "strength" {
				sessionDate = today.AddDate(0, 0, 1)
				sessionDateStr = sessionDate.Format("2006-01-02")
			}

			if _, err = db.ReadWrite.ExecContext(ctx,
				"INSERT INTO workout_sessions (user_id, workout_date, periodization_type) VALUES (?, ?, ?)",
				userID, sessionDateStr, tt.periodization); err != nil {
				t.Fatalf("insert session: %v", err)
			}

			if _, err = svc.AddExercise(ctx, sessionDate, deadliftID); err != nil {
				t.Fatalf("AddExercise: %v", err)
			}

			session, errGet := svc.GetSession(ctx, sessionDate)
			if errGet != nil {
				t.Fatalf("GetSession: %v", errGet)
			}

			var sets []workout.Set
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
		[]byte("swap-derive-user"), "Swap Derive User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := workout.NewService(db, logger, "")

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

	var sets []workout.Set
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

// Test_BuildProgression_CurrentSetUsesDeriveScheme is a regression test for the bug
// where Progression.CurrentSet() returned TargetReps from the legacy TargetReps()
// function (hardcoded 5/8/15) rather than from DeriveScheme on the exercise's
// per-session rep window. A Deadlift (rep_min=3, rep_max=6) on a hypertrophy
// session must produce CurrentSet().TargetReps == 6 (repMax), not 8 (the old
// hypertrophy constant). Before this fix the workout UI displayed "8 reps" even
// though the planner had persisted target_value=6.
func Test_BuildProgression_CurrentSetUsesDeriveScheme(t *testing.T) {
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
		[]byte("ds-user"), "DS User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	// Deadlift-like exercise: rep_min=3, rep_max=6.
	var exerciseID int
	err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max)
		 VALUES (?, 'lower', '', 3, 6) RETURNING id`,
		"Test Deadlift DS").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}

	svc := workout.NewService(db, logger, "")

	today := time.Now().Format("2006-01-02")

	// Hypertrophy session today.
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'hypertrophy')`,
		userID, today)
	if err != nil {
		t.Fatalf("insert hypertrophy session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?)",
		userID, today, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}

	date, _ := time.Parse("2006-01-02", today)

	// Hypertrophy: DeriveScheme(3, 6, Hypertrophy).TargetReps == 6 (repMax).
	// Before the fix this returned 8 (the legacy TargetReps hypertrophy constant).
	prog, err := svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression hypertrophy: %v", err)
	}
	if got := prog.CurrentSet().TargetReps; got != 6 {
		t.Errorf("hypertrophy CurrentSet().TargetReps: want 6, got %d (legacy bug returned 8)", got)
	}

	// Strength session: DeriveScheme(3, 6, Strength).TargetReps == 3 (repMin).
	strengthDay := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'strength')`,
		userID, strengthDay)
	if err != nil {
		t.Fatalf("insert strength session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?)",
		userID, strengthDay, exerciseID)
	if err != nil {
		t.Fatalf("insert strength workout_exercise: %v", err)
	}

	strengthDate, _ := time.Parse("2006-01-02", strengthDay)
	prog, err = svc.BuildProgression(ctx, strengthDate, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression strength: %v", err)
	}
	if got := prog.CurrentSet().TargetReps; got != 3 {
		t.Errorf("strength CurrentSet().TargetReps: want 3, got %d", got)
	}
}
