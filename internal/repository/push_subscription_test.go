package repository_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestPushSubscriptions_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	sub := domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt populated by Insert.
		Endpoint: "https://web.push.apple.com/foo",
		P256dh:   "BPa-abc",
		Auth:     "auth-xyz",
	}
	got, err := repos.PushSubscriptions.Insert(ctx, sub)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got.ID == 0 {
		t.Errorf("Insert returned ID 0")
	}

	subs, err := repos.PushSubscriptions.ListByUser(ctx)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("ListByUser returned %d, want 1", len(subs))
	}
	if subs[0].Endpoint != sub.Endpoint {
		t.Errorf("endpoint = %q, want %q", subs[0].Endpoint, sub.Endpoint)
	}

	count, err := repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		t.Fatalf("CountByUser: %v", err)
	}
	if count != 1 {
		t.Errorf("CountByUser = %d, want 1", count)
	}

	if err = repos.PushSubscriptions.DeleteByEndpoint(ctx, sub.Endpoint); err != nil {
		t.Fatalf("DeleteByEndpoint: %v", err)
	}

	count, _ = repos.PushSubscriptions.CountByUser(ctx)
	if count != 0 {
		t.Errorf("after delete: count = %d, want 0", count)
	}
}

func TestPushSubscriptions_InsertReplacesByEndpoint(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	sub := domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt populated by Insert.
		Endpoint: "https://fcm.googleapis.com/wp/abc",
		P256dh:   "old",
		Auth:     "old",
	}
	if _, err := repos.PushSubscriptions.Insert(ctx, sub); err != nil {
		t.Fatalf("first Insert: %v", err)
	}
	sub.P256dh = "new"
	sub.Auth = "new"
	if _, err := repos.PushSubscriptions.Insert(ctx, sub); err != nil {
		t.Fatalf("second Insert: %v", err)
	}
	subs, _ := repos.PushSubscriptions.ListByUser(ctx)
	if len(subs) != 1 {
		t.Fatalf("got %d rows, want 1", len(subs))
	}
	if subs[0].P256dh != "new" {
		t.Errorf("P256dh = %q, want updated value", subs[0].P256dh)
	}
}

func TestPushSubscriptions_DeleteAllByUser(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	endpoints := []string{
		"https://web.push.apple.com/a",
		"https://web.push.apple.com/b",
		"https://fcm.googleapis.com/wp/c",
	}
	for _, ep := range endpoints {
		sub := domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt populated by Insert.
			Endpoint: ep,
			P256dh:   "p",
			Auth:     "a",
		}
		if _, err := repos.PushSubscriptions.Insert(ctx, sub); err != nil {
			t.Fatalf("Insert %s: %v", ep, err)
		}
	}
	count, err := repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		t.Fatalf("CountByUser before: %v", err)
	}
	if count != len(endpoints) {
		t.Fatalf("CountByUser before = %d, want %d", count, len(endpoints))
	}

	if err = repos.PushSubscriptions.DeleteAllByUser(ctx); err != nil {
		t.Fatalf("DeleteAllByUser: %v", err)
	}

	count, err = repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		t.Fatalf("CountByUser after: %v", err)
	}
	if count != 0 {
		t.Errorf("after DeleteAllByUser: CountByUser = %d, want 0", count)
	}
}
