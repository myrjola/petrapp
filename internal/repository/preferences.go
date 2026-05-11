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

type sqlitePreferencesRepository struct {
	baseRepository
}

func newSQLitePreferencesRepository(db *sqlite.Database) *sqlitePreferencesRepository {
	return &sqlitePreferencesRepository{baseRepository: newBaseRepository(db)}
}

// Get returns the authenticated user's weekly schedule preferences. When no
// row exists yet the all-zero (all rest days) Preferences value is returned —
// this mirrors the previous workout package behaviour and keeps first-time
// users on a clean slate without a special "missing" sentinel.
func (r *sqlitePreferencesRepository) Get(ctx context.Context) (domain.Preferences, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var prefs domain.Preferences
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
		       friday_minutes, saturday_minutes, sunday_minutes,
		       rest_notifications_enabled
		FROM workout_preferences
		WHERE user_id = ?`, userID).Scan(
		&prefs.MondayMinutes, &prefs.TuesdayMinutes, &prefs.WednesdayMinutes, &prefs.ThursdayMinutes,
		&prefs.FridayMinutes, &prefs.SaturdayMinutes, &prefs.SundayMinutes,
		&prefs.RestNotificationsEnabled,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.Preferences{ //nolint:exhaustruct // Weekday minutes zero by design.
			RestNotificationsEnabled: true,
		}, nil
	}
	if err != nil {
		return domain.Preferences{}, fmt.Errorf("query workout preferences: %w", err)
	}
	return prefs, nil
}

// Set upserts the authenticated user's weekly schedule preferences.
func (r *sqlitePreferencesRepository) Set(ctx context.Context, prefs domain.Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_preferences (
			user_id, monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
			friday_minutes, saturday_minutes, sunday_minutes, rest_notifications_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			monday_minutes = excluded.monday_minutes,
			tuesday_minutes = excluded.tuesday_minutes,
			wednesday_minutes = excluded.wednesday_minutes,
			thursday_minutes = excluded.thursday_minutes,
			friday_minutes = excluded.friday_minutes,
			saturday_minutes = excluded.saturday_minutes,
			sunday_minutes = excluded.sunday_minutes,
			rest_notifications_enabled = excluded.rest_notifications_enabled`,
		userID,
		prefs.MondayMinutes, prefs.TuesdayMinutes, prefs.WednesdayMinutes, prefs.ThursdayMinutes,
		prefs.FridayMinutes, prefs.SaturdayMinutes, prefs.SundayMinutes,
		prefs.RestNotificationsEnabled,
	); err != nil {
		return fmt.Errorf("save workout preferences: %w", err)
	}
	return nil
}
