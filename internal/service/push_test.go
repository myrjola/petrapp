package service_test

import (
	"context"
	"testing"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_DeletePushSubscription_EmptyEndpoint_RemovesAll(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("del-all-user"), "Del All User",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := service.NewService(db, logger, "")

	for _, ep := range []string{"https://x/a", "https://x/b", "https://x/c"} {
		//nolint:exhaustruct // ID/UserID/CreatedAt assigned by repo.
		sub := domain.PushSubscription{
			Endpoint: ep, P256dh: "p", Auth: "a",
		}
		if _, err = svc.UpsertPushSubscription(ctx, sub); err != nil {
			t.Fatalf("UpsertPushSubscription %s: %v", ep, err)
		}
	}

	if err = svc.DeletePushSubscription(ctx, ""); err != nil {
		t.Fatalf("DeletePushSubscription(empty): %v", err)
	}

	count, err := svc.CountPushSubscriptions(ctx)
	if err != nil {
		t.Fatalf("CountPushSubscriptions: %v", err)
	}
	if count != 0 {
		t.Errorf("after delete-all: count = %d, want 0", count)
	}
}
