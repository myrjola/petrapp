package main

import (
	"fmt"
	"log/slog"
	"net/http"
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
