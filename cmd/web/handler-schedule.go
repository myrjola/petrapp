package main

import (
	"fmt"
	"net/http"
)

type scheduleTemplateData struct {
	BaseTemplateData
	Header          PageHeaderData
	Weekdays        []weekdayPreference
	DurationOptions []workoutDurationOption
	Flash           BannerData
}

func (app *application) scheduleGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefs, err := app.service.GetUserPreferences(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}

	data := scheduleTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Header: PageHeaderData{
			Title:    "Set Up Your Schedule",
			Subtitle: "Choose which days you'll be going to the gym",
		},
		Weekdays:        preferencesToWeekdays(prefs),
		DurationOptions: getWorkoutDurationOptions(),
		Flash:           BannerData{Variant: "error", Message: app.popFlashError(ctx)},
	}

	app.render(w, r, http.StatusOK, "schedule", data)
}

func (app *application) schedulePOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}

	prefs, err := app.service.GetUserPreferences(r.Context())
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}
	prefs.MondayMinutes = parseMinutes(r.Form.Get("monday_minutes"))
	prefs.TuesdayMinutes = parseMinutes(r.Form.Get("tuesday_minutes"))
	prefs.WednesdayMinutes = parseMinutes(r.Form.Get("wednesday_minutes"))
	prefs.ThursdayMinutes = parseMinutes(r.Form.Get("thursday_minutes"))
	prefs.FridayMinutes = parseMinutes(r.Form.Get("friday_minutes"))
	prefs.SaturdayMinutes = parseMinutes(r.Form.Get("saturday_minutes"))
	prefs.SundayMinutes = parseMinutes(r.Form.Get("sunday_minutes"))

	if prefs.IsEmpty() {
		app.putFlashError(r.Context(), "Please schedule at least one workout day.")
		redirect(w, r, "/schedule")
		return
	}

	if err = app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
		return
	}

	redirect(w, r, "/")
}
