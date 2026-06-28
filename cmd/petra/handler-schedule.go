package main

import (
	"fmt"
	"net/http"
	"time"
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

	base := newBaseTemplateData(r)
	flash := app.popFlash(ctx)
	data := scheduleTemplateData{
		BaseTemplateData: base,
		Header: PageHeaderData{
			Title:    "Set Up Your Schedule",
			Subtitle: "Choose which days you'll be going to the gym",
			Nonce:    base.Nonce,
		},
		Weekdays:        preferencesToWeekdays(prefs),
		DurationOptions: getWorkoutDurationOptions(),
		Flash: BannerData{
			Variant: flash.Variant,
			Message: flash.Message,
			Live:    true,
			Nonce:   base.Nonce,
		},
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
	prefs.Minutes[time.Monday] = parseMinutes(r.Form.Get("monday_minutes"))
	prefs.Minutes[time.Tuesday] = parseMinutes(r.Form.Get("tuesday_minutes"))
	prefs.Minutes[time.Wednesday] = parseMinutes(r.Form.Get("wednesday_minutes"))
	prefs.Minutes[time.Thursday] = parseMinutes(r.Form.Get("thursday_minutes"))
	prefs.Minutes[time.Friday] = parseMinutes(r.Form.Get("friday_minutes"))
	prefs.Minutes[time.Saturday] = parseMinutes(r.Form.Get("saturday_minutes"))
	prefs.Minutes[time.Sunday] = parseMinutes(r.Form.Get("sunday_minutes"))

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
