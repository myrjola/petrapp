package main

import (
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/workout"
	"log/slog"
	"net/http"
)

type weekdayPreference struct {
	ID      string // lowercase ID for form field name
	Name    string // Display name
	Checked bool
}

type preferencesTemplateData struct {
	BaseTemplateData
	Weekdays []weekdayPreference
}

func preferencesToWeekdays(prefs workout.WorkoutPreferences) []weekdayPreference {
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

func weekdaysToPreferences(r *http.Request) workout.WorkoutPreferences {
	return workout.WorkoutPreferences{
		Monday:    r.Form.Get("monday") == "true",
		Tuesday:   r.Form.Get("tuesday") == "true",
		Wednesday: r.Form.Get("wednesday") == "true",
		Thursday:  r.Form.Get("thursday") == "true",
		Friday:    r.Form.Get("friday") == "true",
		Saturday:  r.Form.Get("saturday") == "true",
		Sunday:    r.Form.Get("sunday") == "true",
	}
}

func (app *application) preferencesGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefs, err := app.workoutService.GetUserPreferences(ctx)
	if err != nil {
		app.serverError(w, r, errors.Wrap(err, "get user preferences"))
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
		app.serverError(w, r, errors.Wrap(err, "parse form"))
		return
	}

	prefs := weekdaysToPreferences(r)

	if err := app.workoutService.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, errors.Wrap(err, "save user preferences",
			slog.Any("preferences", prefs)))
		return
	}

	http.Redirect(w, r, "/preferences", http.StatusSeeOther)
}
