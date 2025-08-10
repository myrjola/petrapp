package workout

import (
	"context"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// sqlitePreferencesRepository implements preferencesRepository.
type sqlitePreferencesRepository struct {
	baseRepository
}

// newSQLitePreferenceRepository creates a new SQLite preferences repository.
func newSQLitePreferenceRepository(db *sqlite.Database) *sqlitePreferencesRepository {
	return &sqlitePreferencesRepository{
		baseRepository: newBaseRepository(db),
	}
}

// GetUserPreferences retrieves the workout preferences for a user.
func (r *sqlitePreferencesRepository) Get(ctx context.Context) (Preferences, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var prefs Preferences
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes, 
		       friday_minutes, saturday_minutes, sunday_minutes
		FROM workout_preferences 
		WHERE user_id = ?`, userID).Scan(
		&prefs.MondayMinutes, &prefs.TuesdayMinutes, &prefs.WednesdayMinutes, &prefs.ThursdayMinutes,
		&prefs.FridayMinutes, &prefs.SaturdayMinutes, &prefs.SundayMinutes,
	)

	if errors.Is(err, ErrNotFound) {
		// If no preferences are found, return default preferences (all rest days)
		return Preferences{
			MondayMinutes:    0,
			TuesdayMinutes:   0,
			WednesdayMinutes: 0,
			ThursdayMinutes:  0,
			FridayMinutes:    0,
			SaturdayMinutes:  0,
			SundayMinutes:    0,
		}, nil
	}

	if err != nil {
		return Preferences{}, fmt.Errorf("query workout preferences: %w", err)
	}

	return prefs, nil
}

// Set saves the workout preferences for a user.
func (r *sqlitePreferencesRepository) Set(ctx context.Context, prefs Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	_, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_preferences (
			user_id, monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
			friday_minutes, saturday_minutes, sunday_minutes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			monday_minutes = excluded.monday_minutes,
			tuesday_minutes = excluded.tuesday_minutes,
			wednesday_minutes = excluded.wednesday_minutes,
			thursday_minutes = excluded.thursday_minutes,
			friday_minutes = excluded.friday_minutes,
			saturday_minutes = excluded.saturday_minutes,
			sunday_minutes = excluded.sunday_minutes`,
		userID,
		prefs.MondayMinutes, prefs.TuesdayMinutes, prefs.WednesdayMinutes, prefs.ThursdayMinutes,
		prefs.FridayMinutes, prefs.SaturdayMinutes, prefs.SundayMinutes,
	)

	if err != nil {
		return fmt.Errorf("save workout preferences: %w", err)
	}

	return nil
}
