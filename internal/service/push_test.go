package service_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestDeletePushSubscription_EmptyEndpointRemovesAll(t *testing.T) {
	t.Parallel()
	ctx, svc := setupTestService(t)

	for _, ep := range []string{"https://x/a", "https://x/b", "https://x/c"} {
		sub := domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt assigned by repo.
			Endpoint: ep,
			P256dh:   "p",
			Auth:     "a",
		}
		if _, err := svc.UpsertPushSubscription(ctx, sub); err != nil {
			t.Fatalf("UpsertPushSubscription %s: %v", ep, err)
		}
	}

	if err := svc.DeletePushSubscription(ctx, ""); err != nil {
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
