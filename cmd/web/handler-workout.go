package main

import (
	"fmt"
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/workout"
	"net/http"
	"strconv"
	"time"
)

type workoutTemplateData struct {
	BaseTemplateData
	Date    time.Time
	Session workout.Session
}

type workoutCompletionTemplateData struct {
	BaseTemplateData
	Date         time.Time
	Difficulties []difficultyOption
}

type difficultyOption struct {
	Value int
	Label string
}

func (app *application) workoutCompletionGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := workoutCompletionTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Date:             date,
		Difficulties: []difficultyOption{
			{Value: 1, Label: "Too easy"},
			{Value: 2, Label: "I could do more"},
			{Value: 3, Label: "Just right"},
			{Value: 4, Label: "Very hard"},
			{Value: 5, Label: "Impossible"},
		},
	}

	app.render(w, r, http.StatusOK, "workout-completion", data)
}

func (app *application) workoutCompletePOST(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// First mark the workout as completed
	if err = app.workoutService.CompleteSession(r.Context(), date); err != nil {
		app.serverError(w, r, err)
		return
	}

	// Redirect to the completion form
	http.Redirect(w, r, fmt.Sprintf("/workouts/%s/complete", dateStr), http.StatusSeeOther)
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

func (app *application) workoutFeedbackPOST(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse difficulty from URL path
	difficultyStr := r.PathValue("difficulty")
	difficulty, err := strconv.Atoi(difficultyStr)
	if err != nil {
		app.serverError(w, r, errors.Wrap(err, "parse difficulty rating"))
		return
	}

	// Save the feedback
	if err = app.workoutService.SaveFeedback(r.Context(), date, difficulty); err != nil {
		app.serverError(w, r, err)
		return
	}

	// Redirect back to the home page
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
