package service

import (
	"context"
	"fmt"

	"github.com/myrjola/petrapp/internal/domain"
)

// UpsertPushSubscription inserts or updates the authenticated user's push
// subscription identified by endpoint.
func (s *Service) UpsertPushSubscription(
	ctx context.Context, sub domain.PushSubscription,
) (domain.PushSubscription, error) {
	stored, err := s.repos.PushSubscriptions.Insert(ctx, sub)
	if err != nil {
		return domain.PushSubscription{}, fmt.Errorf("insert push subscription: %w", err)
	}
	return stored, nil
}

// DeletePushSubscription removes the authenticated user's subscription
// identified by endpoint. Empty endpoint deletes all subscriptions for the
// user.
func (s *Service) DeletePushSubscription(ctx context.Context, endpoint string) error {
	if endpoint == "" {
		subs, err := s.repos.PushSubscriptions.ListByUser(ctx)
		if err != nil {
			return fmt.Errorf("list push subscriptions: %w", err)
		}
		for _, sub := range subs {
			if err = s.repos.PushSubscriptions.DeleteByID(ctx, sub.ID); err != nil {
				return fmt.Errorf("delete push subscription: %w", err)
			}
		}
		return nil
	}
	if err := s.repos.PushSubscriptions.DeleteByEndpoint(ctx, endpoint); err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	return nil
}

// CountPushSubscriptions returns the number of subscribed devices for the
// authenticated user.
func (s *Service) CountPushSubscriptions(ctx context.Context) (int, error) {
	n, err := s.repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		return 0, fmt.Errorf("count push subscriptions: %w", err)
	}
	return n, nil
}
