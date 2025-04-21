package main

import (
	"fmt"
	"github.com/myrjola/petrapp/internal/workout"
	"log/slog"
	"net/http"
)

const TRUE = "true"

type weekdayPreference struct {
	ID      string // lowercase ID for form field name
	Name    string // Display name
	Checked bool
}

type preferencesTemplateData struct {
	BaseTemplateData
	Weekdays []weekdayPreference
}

func preferencesToWeekdays(prefs workout.Preferences) []weekdayPreference {
	return []weekdayPreference{
		{ID: "monday", Name: "Monday", Checked: prefs.Monday},
		{ID: "tuesday", Name: "Tuesday", Checked: prefs.Tuesday},
		{ID: "wednesday", Name: "Wednesday", Checked: prefs.Wednesday},
		{ID: "thursday", Name: "Thursday", Checked: prefs.Thursday},
		{ID: "friday", Name: "Friday", Checked: prefs.Friday},
		{ID: "saturday", Name: "Saturday", Checked: prefs.Saturday},
		{ID: "sunday", Name: "Sunday", Checked: prefs.Sunday},
	}
}

func weekdaysToPreferences(r *http.Request) workout.Preferences {
	return workout.Preferences{
		Monday:    r.Form.Get("monday") == TRUE,
		Tuesday:   r.Form.Get("tuesday") == TRUE,
		Wednesday: r.Form.Get("wednesday") == TRUE,
		Thursday:  r.Form.Get("thursday") == TRUE,
		Friday:    r.Form.Get("friday") == TRUE,
		Saturday:  r.Form.Get("saturday") == TRUE,
		Sunday:    r.Form.Get("sunday") == TRUE,
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
