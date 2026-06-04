package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/platform/contexthelpers"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
	"github.com/myrjola/petrapp/internal/repository"
)

// setupTestRepos creates an in-memory database, inserts a test user, and
// returns the authenticated context plus a populated *Repositories.
func setupTestRepos(t *testing.T) (context.Context, *repository.Repositories) {
	t.Helper()
	ctx, _, repos := setupTestReposWithDB(t)
	return ctx, repos
}

// setupTestReposWithDB is like setupTestRepos but also returns the *sqlitekit.Database
// so callers can seed fixtures directly. Use only in tests that need it.
func setupTestReposWithDB(t *testing.T) (context.Context, *sqlitekit.Database, *repository.Repositories) {
	t.Helper()
	ctx := t.Context()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:      ":memory:",
		Schema:   repository.SchemaSQL,
		Fixtures: repository.FixturesSQL,
		Logger:   logger,
	})
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

	return ctx, db, repository.New(db)
}

// seedWorkoutExerciseSlot inserts a workout_session and workout_exercises row at
// position 0 for the authenticated user and returns the workout date and the
// slot position (always 0). Callers that need additional slots can insert
// further rows at incrementing positions.
func seedWorkoutExerciseSlot(
	ctx context.Context, t *testing.T, db *sqlitekit.Database,
) (time.Time, int) {
	t.Helper()
	userID := contexthelpers.AuthenticatedUserID(ctx)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	todayStr := today.Format("2006-01-02")
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)
		 ON CONFLICT DO NOTHING`,
		userID, todayStr,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var exerciseID int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Deadlift'`,
	).Scan(&exerciseID); err != nil {
		t.Fatalf("fetch deadlift: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
		 VALUES (?, ?, 0, ?)`,
		userID, todayStr, exerciseID,
	); err != nil {
		t.Fatalf("insert workout_exercises: %v", err)
	}
	return today, 0
}
