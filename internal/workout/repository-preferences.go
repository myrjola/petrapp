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

// newSQLitePreferencesRepository creates a new SQLite preferences repository.
func newSQLitePreferencesRepository(db *sqlite.Database) *sqlitePreferencesRepository {
	return &sqlitePreferencesRepository{
		baseRepository: newBaseRepository(db),
	}
}

// GetUserPreferences retrieves the workout preferences for a user.
func (r *sqlitePreferencesRepository) Get(ctx context.Context) (Preferences, error) {
	var prefs Preferences
	userID := contexthelpers.AuthenticatedUserID(ctx)
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT monday, tuesday, wednesday, thursday, friday, saturday, sunday 
		FROM workout_preferences 
		WHERE user_id = ?`, userID).Scan(
		&prefs.Monday,
		&prefs.Tuesday,
		&prefs.Wednesday,
		&prefs.Thursday,
		&prefs.Friday,
		&prefs.Saturday,
		&prefs.Sunday,
	)

	if errors.Is(err, ErrNotFound) {
		// If no preferences are found, return default preferences
		return Preferences{
			Monday:    false,
			Tuesday:   false,
			Wednesday: false,
			Thursday:  false,
			Friday:    false,
			Saturday:  false,
			Sunday:    false,
		}, nil
	}

	if err != nil {
		return Preferences{}, fmt.Errorf("query workout preferences: %w", err)
	}

	return prefs, nil
}

// SaveUserPreferences saves the workout preferences for a user.
func (r *sqlitePreferencesRepository) Set(ctx context.Context, prefs Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	_, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_preferences (
			user_id, monday, tuesday, wednesday, thursday, friday, saturday, sunday
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			monday = excluded.monday,
			tuesday = excluded.tuesday,
			wednesday = excluded.wednesday,
			thursday = excluded.thursday,
			friday = excluded.friday,
			saturday = excluded.saturday,
			sunday = excluded.sunday`,
		userID,
		prefs.Monday,
		prefs.Tuesday,
		prefs.Wednesday,
		prefs.Thursday,
		prefs.Friday,
		prefs.Saturday,
		prefs.Sunday,
	)

	if err != nil {
		return fmt.Errorf("save workout preferences: %w", err)
	}

	return nil
}
