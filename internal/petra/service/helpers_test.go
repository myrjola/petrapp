package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/petra/repository"
	"github.com/myrjola/petrapp/internal/petra/service"
	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/contexthelpers"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func setupTestService(t *testing.T) (context.Context, *service.Service) {
	t.Helper()
	ctx, svc, _ := setupTestServiceWithDB(t)
	return ctx, svc
}

// setupTestServiceWithDB is like setupTestService but additionally returns the
// *sqlitekit.Database handle so tests that need to seed prior workout history
// (e.g. completed hypertrophy sessions to satisfy GetDeloadStartingWeight) can
// run direct SQL against the same in-memory database the service uses.
func setupTestServiceWithDB(t *testing.T) (context.Context, *service.Service, *sqlitekit.Database) {
	t.Helper()
	ctx := t.Context()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + repository.SchemaSQL,
		Fixtures:     repository.FixturesSQL,
		Logger:       logger,
		Premigration: nil,
	})
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
	svc := service.NewService(db, logger, "")
	if err = svc.SaveUserPreferences(ctx, domain.Preferences{ //nolint:exhaustruct // Rest days intentionally omitted.
		Minutes: [7]int{
			time.Monday:    60,
			time.Wednesday: 60,
			time.Friday:    60,
		},
	}); err != nil {
		t.Fatalf("save preferences: %v", err)
	}
	return ctx, svc, db
}

func extractExerciseIDs(session domain.Session) []int {
	ids := make([]int, len(session.Slots))
	for i, es := range session.Slots {
		ids[i] = es.Exercise.ID
	}
	return ids
}

// Helper function to count exercise sets for a given exercise.
func countExerciseSets(t *testing.T, db *sqlitekit.Database, exerciseID int) (int, error) {
	t.Helper()
	var count int
	err := db.ReadOnly.QueryRow(
		`SELECT COUNT(*) FROM exercise_sets es
		 JOIN workout_exercises we
		    ON we.workout_user_id = es.workout_user_id
		   AND we.workout_date    = es.workout_date
		   AND we.position        = es.position
		 WHERE we.exercise_id = ?`,
		exerciseID,
	).Scan(&count)
	return count, err
}

// Try to insert a muscle group, ignoring if it already exists.
func tryInsertMuscleGroup(ctx context.Context, t *testing.T, db *sqlitekit.Database, name string) error {
	t.Helper()
	_, err := db.ReadWrite.ExecContext(ctx, "INSERT INTO muscle_groups (name) VALUES (?)", name)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		// Muscle group already exists, which is fine
		return nil
	}
	return err
}

// Helper function to create a test exercise.
func createTestExercise(ctx context.Context, t *testing.T, db *sqlitekit.Database, name, category string) (int, error) {
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
func countExerciseSetsForWorkout(ctx context.Context, t *testing.T, svc *service.Service, date time.Time) (int, error) {
	t.Helper()
	session, err := svc.GetSession(ctx, date)
	if err != nil {
		return 0, err
	}

	// Count total sets across all exercises
	totalSets := 0
	for _, exerciseSlot := range session.Slots {
		totalSets += len(exerciseSlot.Sets)
	}

	return totalSets, nil
}

// Helper function to check if an exercise exists in a workout.
func exerciseExistsInWorkout(
	ctx context.Context,
	t *testing.T,
	svc *service.Service,
	date time.Time,
	exerciseID int,
) (bool, error) {
	t.Helper()
	session, err := svc.GetSession(ctx, date)
	if err != nil {
		return false, err
	}

	// Check if any exercise set has the specified exercise ID
	for _, exerciseSlot := range session.Slots {
		if exerciseSlot.Exercise.ID == exerciseID {
			return true, nil
		}
	}

	return false, nil
}

// Helper function to check if a workout exists for a date.
func workoutExistsForDate(ctx context.Context, t *testing.T, svc *service.Service, date time.Time) (bool, error) {
	t.Helper()
	_, err := svc.GetSession(ctx, date)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
