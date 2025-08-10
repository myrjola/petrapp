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

	// We need to query both booleans and minutes from the database
	var mondayBool, tuesdayBool, wednesdayBool, thursdayBool, fridayBool, saturdayBool, sundayBool bool
	var mondayMinutes, tuesdayMinutes, wednesdayMinutes, thursdayMinutes, fridayMinutes, saturdayMinutes, sundayMinutes int

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT monday, tuesday, wednesday, thursday, friday, saturday, sunday,
		       monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes, 
		       friday_minutes, saturday_minutes, sunday_minutes
		FROM workout_preferences 
		WHERE user_id = ?`, userID).Scan(
		&mondayBool, &tuesdayBool, &wednesdayBool, &thursdayBool,
		&fridayBool, &saturdayBool, &sundayBool,
		&mondayMinutes, &tuesdayMinutes, &wednesdayMinutes, &thursdayMinutes,
		&fridayMinutes, &saturdayMinutes, &sundayMinutes,
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

	// Convert database values to domain model
	// Priority: if minutes is set and > 0, use it; otherwise convert boolean to minutes
	prefs := Preferences{
		MondayMinutes:    convertToMinutes(mondayBool, mondayMinutes),
		TuesdayMinutes:   convertToMinutes(tuesdayBool, tuesdayMinutes),
		WednesdayMinutes: convertToMinutes(wednesdayBool, wednesdayMinutes),
		ThursdayMinutes:  convertToMinutes(thursdayBool, thursdayMinutes),
		FridayMinutes:    convertToMinutes(fridayBool, fridayMinutes),
		SaturdayMinutes:  convertToMinutes(saturdayBool, saturdayMinutes),
		SundayMinutes:    convertToMinutes(sundayBool, sundayMinutes),
	}

	return prefs, nil
}

// convertToMinutes converts database boolean/minutes to domain minutes.
func convertToMinutes(dayBool bool, dayMinutes int) int {
	// If minutes is already set and > 0, use it
	if dayMinutes > 0 {
		return dayMinutes
	}
	// Otherwise, convert boolean: true -> 60 minutes, false -> 0 minutes
	if dayBool {
		return 60
	}
	return 0
}

// Set saves the workout preferences for a user.
func (r *sqlitePreferencesRepository) Set(ctx context.Context, prefs Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	// Convert minutes to booleans for database storage
	mondayBool := prefs.MondayMinutes > 0
	tuesdayBool := prefs.TuesdayMinutes > 0
	wednesdayBool := prefs.WednesdayMinutes > 0
	thursdayBool := prefs.ThursdayMinutes > 0
	fridayBool := prefs.FridayMinutes > 0
	saturdayBool := prefs.SaturdayMinutes > 0
	sundayBool := prefs.SundayMinutes > 0

	_, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_preferences (
			user_id, monday, tuesday, wednesday, thursday, friday, saturday, sunday,
			monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
			friday_minutes, saturday_minutes, sunday_minutes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			monday = excluded.monday,
			tuesday = excluded.tuesday,
			wednesday = excluded.wednesday,
			thursday = excluded.thursday,
			friday = excluded.friday,
			saturday = excluded.saturday,
			sunday = excluded.sunday,
			monday_minutes = excluded.monday_minutes,
			tuesday_minutes = excluded.tuesday_minutes,
			wednesday_minutes = excluded.wednesday_minutes,
			thursday_minutes = excluded.thursday_minutes,
			friday_minutes = excluded.friday_minutes,
			saturday_minutes = excluded.saturday_minutes,
			sunday_minutes = excluded.sunday_minutes`,
		userID,
		mondayBool, tuesdayBool, wednesdayBool, thursdayBool,
		fridayBool, saturdayBool, sundayBool,
		prefs.MondayMinutes, prefs.TuesdayMinutes, prefs.WednesdayMinutes, prefs.ThursdayMinutes,
		prefs.FridayMinutes, prefs.SaturdayMinutes, prefs.SundayMinutes,
	)

	if err != nil {
		return fmt.Errorf("save workout preferences: %w", err)
	}

	return nil
}
