package main

import (
	"fmt"
	"github.com/myrjola/petrapp/internal/workout"
	"net/http"
	"time"
)

type workoutTemplateData struct {
	BaseTemplateData
	Date    time.Time
	Session workout.Session
}

func (app *application) workoutStartPOST(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Start the workout session
	if err := app.workoutService.StartSession(r.Context(), date); err != nil {
		app.serverError(w, r, err)
		return
	}

	// Redirect to the workout page
	http.Redirect(w, r, fmt.Sprintf("/workouts/%s", dateStr), http.StatusSeeOther)
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
