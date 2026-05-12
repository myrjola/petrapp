package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// defaultMesocycleLengthWeeks is the default mesocycle length in weeks, matching the SQL column default.
const defaultMesocycleLengthWeeks = 5

type sqlitePreferencesRepository struct {
	baseRepository
}

func newSQLitePreferencesRepository(db *sqlite.Database) *sqlitePreferencesRepository {
	return &sqlitePreferencesRepository{baseRepository: newBaseRepository(db)}
}

// Get returns the authenticated user's weekly schedule preferences. When no
// row exists yet the weekday minutes default to zero (all rest days),
// RestNotificationsEnabled defaults to true, and MesocycleLength defaults
// to 5, matching the SQL column defaults.
func (r *sqlitePreferencesRepository) Get(ctx context.Context) (domain.Preferences, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var (
		prefs     domain.Preferences
		anchorStr sql.NullString
	)
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
		       friday_minutes, saturday_minutes, sunday_minutes,
		       rest_notifications_enabled,
		       deload_enabled, mesocycle_length, mesocycle_anchor
		FROM workout_preferences
		WHERE user_id = ?`, userID).Scan(
		&prefs.MondayMinutes, &prefs.TuesdayMinutes, &prefs.WednesdayMinutes, &prefs.ThursdayMinutes,
		&prefs.FridayMinutes, &prefs.SaturdayMinutes, &prefs.SundayMinutes,
		&prefs.RestNotificationsEnabled,
		&prefs.DeloadEnabled, &prefs.MesocycleLength, &anchorStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.Preferences{ //nolint:exhaustruct // Weekday minutes zero by design.
			RestNotificationsEnabled: true,
			MesocycleLength:          defaultMesocycleLengthWeeks,
		}, nil
	}
	if err != nil {
		return domain.Preferences{}, fmt.Errorf("query workout preferences: %w", err)
	}
	if anchorStr.Valid {
		anchor, parseErr := time.Parse(dateFormat, anchorStr.String)
		if parseErr != nil {
			return domain.Preferences{}, fmt.Errorf("parse mesocycle_anchor: %w", parseErr)
		}
		prefs.MesocycleAnchor = anchor
	}
	return prefs, nil
}

// Set upserts the authenticated user's weekly schedule preferences.
func (r *sqlitePreferencesRepository) Set(ctx context.Context, prefs domain.Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var anchorStr sql.NullString
	if !prefs.MesocycleAnchor.IsZero() {
		anchorStr = sql.NullString{Valid: true, String: formatDate(prefs.MesocycleAnchor)}
	}
	length := prefs.MesocycleLength
	if length == 0 {
		length = 5
	}
	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_preferences (
			user_id, monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
			friday_minutes, saturday_minutes, sunday_minutes, rest_notifications_enabled,
			deload_enabled, mesocycle_length, mesocycle_anchor
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			monday_minutes = excluded.monday_minutes,
			tuesday_minutes = excluded.tuesday_minutes,
			wednesday_minutes = excluded.wednesday_minutes,
			thursday_minutes = excluded.thursday_minutes,
			friday_minutes = excluded.friday_minutes,
			saturday_minutes = excluded.saturday_minutes,
			sunday_minutes = excluded.sunday_minutes,
			rest_notifications_enabled = excluded.rest_notifications_enabled,
			deload_enabled = excluded.deload_enabled,
			mesocycle_length = excluded.mesocycle_length,
			mesocycle_anchor = excluded.mesocycle_anchor`,
		userID,
		prefs.MondayMinutes, prefs.TuesdayMinutes, prefs.WednesdayMinutes, prefs.ThursdayMinutes,
		prefs.FridayMinutes, prefs.SaturdayMinutes, prefs.SundayMinutes,
		prefs.RestNotificationsEnabled,
		prefs.DeloadEnabled, length, anchorStr,
	); err != nil {
		return fmt.Errorf("save workout preferences: %w", err)
	}
	return nil
}
