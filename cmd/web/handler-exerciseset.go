package main

import (
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/workout"
	"net/http"
	"strconv"
	"time"
)

type exerciseSetTemplateData struct {
	BaseTemplateData
	Date        time.Time
	ExerciseSet workout.ExerciseSet
}

func (app *application) exerciseSetGET(w http.ResponseWriter, r *http.Request) {
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

	// Get workout session
	session, err := app.workoutService.GetSession(r.Context(), date)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Find matching exercise set
	var exerciseSet workout.ExerciseSet
	for _, es := range session.ExerciseSets {
		if es.Exercise.ID == exerciseID {
			exerciseSet = es
			break
		}
	}
	if exerciseSet.Exercise.ID == 0 {
		http.NotFound(w, r)
		return
	}

	data := exerciseSetTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Date:             date,
		ExerciseSet:      exerciseSet,
	}

	app.render(w, r, http.StatusOK, "exerciseset", data)
}

func (app *application) exerciseSetEditPOST(w http.ResponseWriter, r *http.Request) {
	// Parse URL parameters
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	exerciseIDStr := r.PathValue("exerciseID")
	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	setIndexStr := r.PathValue("setIndex")
	setIndex, err := strconv.Atoi(setIndexStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse form for weight update
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, errors.Wrap(err, "parse form"))
		return
	}

	weightStr := r.PostForm.Get("weight")
	if weightStr == "" {
		app.serverError(w, r, errors.New("weight not provided"))
		return
	}

	weight, err := strconv.ParseFloat(weightStr, 64)
	if err != nil {
		app.serverError(w, r, errors.Wrap(err, "parse weight"))
		return
	}

	// Update the weight
	if err := app.workoutService.UpdateSetWeight(r.Context(), date, exerciseID, setIndex, weight); err != nil {
		app.serverError(w, r, errors.Wrap(err, "UPDATE set weight"))
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

func (app *application) exerciseSetDonePOST(w http.ResponseWriter, r *http.Request) {
	// Parse URL parameters
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	exerciseIDStr := r.PathValue("exerciseID")
	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	setIndexStr := r.PathValue("setIndex")
	setIndex, err := strconv.Atoi(setIndexStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse form for completed reps
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, errors.Wrap(err, "parse form"))
		return
	}

	repsStr := r.PostForm.Get("reps")
	if repsStr == "" {
		app.serverError(w, r, errors.New("reps not provided"))
		return
	}

	reps, err := strconv.Atoi(repsStr)
	if err != nil {
		app.serverError(w, r, errors.Wrap(err, "parse reps"))
		return
	}

	// Mark the set as completed
	if err := app.workoutService.CompleteSet(r.Context(), date, exerciseID, setIndex, reps); err != nil {
		app.serverError(w, r, errors.Wrap(err, "complete set"))
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}
