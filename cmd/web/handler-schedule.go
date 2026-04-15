package main

import (
	"fmt"
	"net/http"
)

type scheduleTemplateData struct {
	BaseTemplateData
	Weekdays        []weekdayPreference
	DurationOptions []workoutDurationOption
	ValidationError string
}

func (app *application) scheduleGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefs, err := app.workoutService.GetUserPreferences(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}

	data := scheduleTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Weekdays:         preferencesToWeekdays(prefs),
		DurationOptions:  getWorkoutDurationOptions(),
		ValidationError:  "",
	}

	app.render(w, r, http.StatusOK, "schedule", data)
}

func (app *application) schedulePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	prefs := weekdaysToPreferences(r)

	if prefs.IsEmpty() {
		data := scheduleTemplateData{
			BaseTemplateData: newBaseTemplateData(r),
			Weekdays:         preferencesToWeekdays(prefs),
			DurationOptions:  getWorkoutDurationOptions(),
			ValidationError:  "Please schedule at least one workout day.",
		}
		app.render(w, r, http.StatusUnprocessableEntity, "schedule", data)
		return
	}

	if err := app.workoutService.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
		return
	}

	redirect(w, r, "/")
}
