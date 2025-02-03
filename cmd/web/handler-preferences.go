package main

import (
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/workout"
	"log/slog"
	"net/http"
)

type preferencesTemplateData struct {
	BaseTemplateData
	Preferences workout.WorkoutPreferences
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
		Preferences:      prefs,
	}

	app.render(w, r, http.StatusOK, "preferences", data)
}

func (app *application) preferencesPOST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, errors.Wrap(err, "parse form"))
		return
	}

	// Parse form data into WorkoutPreferences
	// Form values will be present only if checkbox was checked
	prefs := workout.WorkoutPreferences{
		Monday:    r.Form.Get("monday") == "true",
		Tuesday:   r.Form.Get("tuesday") == "true",
		Wednesday: r.Form.Get("wednesday") == "true",
		Thursday:  r.Form.Get("thursday") == "true",
		Friday:    r.Form.Get("friday") == "true",
		Saturday:  r.Form.Get("saturday") == "true",
		Sunday:    r.Form.Get("sunday") == "true",
	}

	// Save preferences using workout service
	if err := app.workoutService.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, errors.Wrap(err, "save user preferences",
			slog.Any("preferences", prefs)))
		return
	}

	// Redirect back to preferences page
	http.Redirect(w, r, "/preferences", http.StatusSeeOther)
}
