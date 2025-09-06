package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/myrjola/petrapp/internal/workout"
)

const (
	RestDayMinutes        = 0
	FortyFiveMinutes      = 45
	OneHourMinutes        = 60
	OneAndHalfHourMinutes = 90
)

type weekdayPreference struct {
	ID      string // lowercase ID for form field name
	Name    string // Display name
	Minutes int    // Selected workout duration in minutes
}

type workoutDurationOption struct {
	Value int    // Minutes value
	Label string // Display label
}

type preferencesTemplateData struct {
	BaseTemplateData
	Weekdays        []weekdayPreference
	DurationOptions []workoutDurationOption
}

func getWorkoutDurationOptions() []workoutDurationOption {
	return []workoutDurationOption{
		{Value: RestDayMinutes, Label: "Rest day"},
		{Value: FortyFiveMinutes, Label: "45 minutes"},
		{Value: OneHourMinutes, Label: "1 hour"},
		{Value: OneAndHalfHourMinutes, Label: "1.5 hours"},
	}
}

func preferencesToWeekdays(prefs workout.Preferences) []weekdayPreference {
	return []weekdayPreference{
		{ID: "monday", Name: "Monday", Minutes: prefs.MondayMinutes},
		{ID: "tuesday", Name: "Tuesday", Minutes: prefs.TuesdayMinutes},
		{ID: "wednesday", Name: "Wednesday", Minutes: prefs.WednesdayMinutes},
		{ID: "thursday", Name: "Thursday", Minutes: prefs.ThursdayMinutes},
		{ID: "friday", Name: "Friday", Minutes: prefs.FridayMinutes},
		{ID: "saturday", Name: "Saturday", Minutes: prefs.SaturdayMinutes},
		{ID: "sunday", Name: "Sunday", Minutes: prefs.SundayMinutes},
	}
}

func parseMinutes(value string) int {
	minutes, err := strconv.Atoi(value)
	if err != nil {
		return 0 // Default to rest day if parsing fails
	}
	// Validate against allowed values
	switch minutes {
	case RestDayMinutes, FortyFiveMinutes, OneHourMinutes, OneAndHalfHourMinutes:
		return minutes
	default:
		return RestDayMinutes // Default to rest day for invalid values
	}
}

func weekdaysToPreferences(r *http.Request) workout.Preferences {
	return workout.Preferences{
		MondayMinutes:    parseMinutes(r.Form.Get("monday_minutes")),
		TuesdayMinutes:   parseMinutes(r.Form.Get("tuesday_minutes")),
		WednesdayMinutes: parseMinutes(r.Form.Get("wednesday_minutes")),
		ThursdayMinutes:  parseMinutes(r.Form.Get("thursday_minutes")),
		FridayMinutes:    parseMinutes(r.Form.Get("friday_minutes")),
		SaturdayMinutes:  parseMinutes(r.Form.Get("saturday_minutes")),
		SundayMinutes:    parseMinutes(r.Form.Get("sunday_minutes")),
	}
}

func (app *application) preferencesGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefs, err := app.workoutService.GetUserPreferences(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}

	data := preferencesTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Weekdays:         preferencesToWeekdays(prefs),
		DurationOptions:  getWorkoutDurationOptions(),
	}

	app.render(w, r, http.StatusOK, "preferences", data)
}

func (app *application) preferencesPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	prefs := weekdaysToPreferences(r)

	if err := app.workoutService.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
		app.logger.LogAttrs(r.Context(), slog.LevelDebug, "preferences details", slog.Any("preferences", prefs))
		return
	}

	redirect(w, r, "/")
}

func (app *application) deleteUserPOST(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Delete the user and all their data
	if err := app.webAuthnHandler.DeleteUser(ctx); err != nil {
		app.serverError(w, r, fmt.Errorf("delete user: %w", err))
		return
	}

	// Log the user out by clearing the session and redirect to home
	if err := app.webAuthnHandler.Logout(ctx); err != nil {
		app.serverError(w, r, fmt.Errorf("logout after user deletion: %w", err))
		return
	}

	redirect(w, r, "/")
}

func (app *application) exportUserDataGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Create the user database export
	exportPath, err := app.workoutService.ExportUserData(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("export user data: %w", err))
		return
	}

	// Clean up the temporary file when done
	defer func() {
		if removeErr := os.Remove(exportPath); removeErr != nil {
			app.logger.LogAttrs(ctx, slog.LevelWarn, "failed to remove temporary export file",
				slog.String("path", exportPath), slog.Any("error", removeErr))
		}
	}()

	// Open the file for reading
	file, err := os.Open(exportPath)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("open export file: %w", err))
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			app.logger.LogAttrs(ctx, slog.LevelWarn, "failed to close export file",
				slog.String("path", exportPath), slog.Any("error", closeErr))
		}
	}()

	// Set headers for file download
	filename := filepath.Base(exportPath)
	w.Header().Set("Content-Type", "application/x-sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Stream the file to the client
	_, err = io.Copy(w, file)
	if err != nil {
		app.logger.LogAttrs(ctx, slog.LevelError, "failed to stream export file to client",
			slog.String("path", exportPath), slog.Any("error", err))
		return
	}
}
