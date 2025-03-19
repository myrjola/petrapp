package main

import (
	"github.com/myrjola/petrapp/internal/workout"
	"net/http"
	"strconv"
	"time"
)

// exerciseInfoTemplateData contains data for the exercise info template.
type exerciseInfoTemplateData struct {
	BaseTemplateData
	Date     time.Time
	Exercise workout.Exercise
}

// exerciseInfoGET handles GET requests to view exercise information.
func (app *application) exerciseInfoGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse exercise ID from URL path
	exerciseIDStr := r.PathValue("exerciseID")
	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get the exercise
	exercise, err := app.workoutService.GetExercise(r.Context(), exerciseID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := exerciseInfoTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Date:             date,
		Exercise:         exercise,
	}

	app.render(w, r, http.StatusOK, "exercise-info", data)
}
