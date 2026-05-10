package repository_test

import (
	"context"
	"testing"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// setupTestRepos creates an in-memory database, inserts a test user, and
// returns the authenticated context plus a populated *Repositories.
func setupTestRepos(t *testing.T) (context.Context, *repository.Repositories) {
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

	return ctx, repository.New(db, logger)
}
