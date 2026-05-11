package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqlitePushSubscriptionRepository struct {
	baseRepository
}

func newSQLitePushSubscriptionRepository(db *sqlite.Database) *sqlitePushSubscriptionRepository {
	return &sqlitePushSubscriptionRepository{baseRepository: newBaseRepository(db)}
}

// Insert upserts a push subscription keyed by endpoint. Duplicates are not
// possible (UNIQUE constraint); a second registration with the same endpoint
// rebinds keys to the authenticated user — useful when iOS rotates the auth
// secret on its own.
func (r *sqlitePushSubscriptionRepository) Insert(
	ctx context.Context, sub domain.PushSubscription,
) (domain.PushSubscription, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var createdAt sql.NullString
	err := r.db.ReadWrite.QueryRowContext(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET
		    user_id = excluded.user_id,
		    p256dh  = excluded.p256dh,
		    auth    = excluded.auth
		RETURNING id, created_at`,
		userID, sub.Endpoint, sub.P256dh, sub.Auth,
	).Scan(&sub.ID, &createdAt)
	if err != nil {
		return domain.PushSubscription{}, fmt.Errorf("insert push subscription: %w", err)
	}
	if sub.CreatedAt, err = parseTimestamp(createdAt); err != nil {
		return domain.PushSubscription{}, fmt.Errorf("parse push subscription created_at: %w", err)
	}
	sub.UserID = userID
	return sub, nil
}

func (r *sqlitePushSubscriptionRepository) DeleteByEndpoint(
	ctx context.Context, endpoint string,
) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE user_id = ? AND endpoint = ?`,
		userID, endpoint,
	); err != nil {
		return fmt.Errorf("delete push subscription by endpoint: %w", err)
	}
	return nil
}

// DeleteByID removes a subscription by its primary key without checking
// ownership. Intended for the push sender to prune endpoints rejected with
// 410 Gone / 404 Not Found from the push service; user-initiated deletes
// must go through DeleteByEndpoint, which scopes by the authenticated user.
func (r *sqlitePushSubscriptionRepository) DeleteByID(ctx context.Context, id int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("delete push subscription by id: %w", err)
	}
	return nil
}

func (r *sqlitePushSubscriptionRepository) ListByUser(ctx context.Context) (_ []domain.PushSubscription, err error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, user_id, endpoint, p256dh, auth, created_at
		FROM push_subscriptions
		WHERE user_id = ?
		ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query push subscriptions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var subs []domain.PushSubscription
	for rows.Next() {
		var sub domain.PushSubscription
		var createdAt sql.NullString
		if err = rows.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth, &createdAt); err != nil {
			return nil, fmt.Errorf("scan push subscription: %w", err)
		}
		if sub.CreatedAt, err = parseTimestamp(createdAt); err != nil {
			return nil, fmt.Errorf("parse push subscription created_at: %w", err)
		}
		subs = append(subs, sub)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return subs, nil
}

func (r *sqlitePushSubscriptionRepository) CountByUser(ctx context.Context) (int, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	var count int
	if err := r.db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM push_subscriptions WHERE user_id = ?`, userID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count push subscriptions: %w", err)
	}
	return count, nil
}
