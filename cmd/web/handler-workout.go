package main

import (
	"github.com/myrjola/petrapp/internal/workout"
	"net/http"
	"time"
)

type workoutTemplateData struct {
	BaseTemplateData
	Date    time.Time
	Session workout.Session
}

func (app *application) workoutGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Fetch workout session for the date
	session, err := app.workoutService.GetSession(r.Context(), date)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := workoutTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Date:             date,
		Session:          session,
	}

	app.render(w, r, http.StatusOK, "workout", data)
}
