package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// setupTestRepos creates an in-memory database, inserts a test user, and
// returns the authenticated context plus a populated *Repositories.
func setupTestRepos(t *testing.T) (context.Context, *repository.Repositories) {
	t.Helper()
	ctx, _, repos := setupTestReposWithDB(t)
	return ctx, repos
}

// setupTestReposWithDB is like setupTestRepos but also returns the *sqlite.Database
// so callers can seed fixtures directly. Use only in tests that need it.
func setupTestReposWithDB(t *testing.T) (context.Context, *sqlite.Database, *repository.Repositories) {
	t.Helper()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user"), "Test User").Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	return ctx, db, repository.New(db, logger)
}

// seedWorkoutExercise inserts a workout_session and workout_exercise row for the
// authenticated user and returns the workout_exercise.id.
func seedWorkoutExercise(ctx context.Context, t *testing.T, db *sqlite.Database) int {
	t.Helper()
	userID := contexthelpers.AuthenticatedUserID(ctx)
	today := time.Now().Format("2006-01-02")
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)
		 ON CONFLICT DO NOTHING`,
		userID, today,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var exerciseID int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Deadlift'`,
	).Scan(&exerciseID); err != nil {
		t.Fatalf("fetch deadlift: %v", err)
	}
	var weID int
	if err := db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		 VALUES (?, ?, ?) RETURNING id`,
		userID, today, exerciseID,
	).Scan(&weID); err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	return weID
}
