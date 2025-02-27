package main

import (
	"fmt"
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/workout"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type exerciseSetTemplateData struct {
	BaseTemplateData
	Date                 time.Time
	ExerciseSet          workout.ExerciseSet
	FirstIncompleteIndex int
	EditingIndex         int  // Index of the set being edited
	IsEditing            bool // Whether we're in edit mode
}

func getFirstIncompleteIndex(sets []workout.Set) int {
	for i, set := range sets {
		if set.CompletedReps == nil {
			return i
		}
	}
	return len(sets)
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

	// Check if edit parameter is provided
	editingIndex := -1
	isEditing := false
	editIndexStr := r.URL.Query().Get("edit")
	if editIndexStr != "" {
		if idx, err := strconv.Atoi(editIndexStr); err == nil {
			editingIndex = idx
			isEditing = true
		}
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
		BaseTemplateData:     newBaseTemplateData(r),
		Date:                 date,
		ExerciseSet:          exerciseSet,
		FirstIncompleteIndex: getFirstIncompleteIndex(exerciseSet.Sets),
		EditingIndex:         editingIndex,
		IsEditing:            isEditing,
	}

	app.render(w, r, http.StatusOK, "exerciseset", data)
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

	// Parse form for weight and reps
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

	// First update the weight if it has changed
	if err := app.workoutService.UpdateSetWeight(r.Context(), date, exerciseID, setIndex, weight); err != nil {
		app.serverError(w, r, errors.Wrap(err, "UPDATE set weight"))
		return
	}

	// Then mark the set as completed with the given reps
	if err := app.workoutService.CompleteSet(r.Context(), date, exerciseID, setIndex, reps); err != nil {
		app.serverError(w, r, errors.Wrap(err, "complete set"))
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "completed set",
		slog.String("date", dateStr),
		slog.Int("exercise_id", exerciseID),
		slog.Int("set_index", setIndex),
		slog.Float64("weight", weight),
		slog.Int("reps", reps))

	// Redirect to the clean URL (without the edit query parameter)
	redirectURL := fmt.Sprintf("/workouts/%s/exercises/%d", date.Format("2006-01-02"), exerciseID)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (app *application) exerciseSetUpdatePOST(w http.ResponseWriter, r *http.Request) {
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

	// Parse form for weight and reps
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

	// First update the weight
	if err := app.workoutService.UpdateSetWeight(r.Context(), date, exerciseID, setIndex, weight); err != nil {
		app.serverError(w, r, errors.Wrap(err, "update set weight"))
		return
	}

	// Then update the completed reps
	if err := app.workoutService.UpdateCompletedReps(r.Context(), date, exerciseID, setIndex, reps); err != nil {
		app.serverError(w, r, errors.Wrap(err, "update completed reps"))
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "updated set",
		slog.String("date", dateStr),
		slog.Int("exercise_id", exerciseID),
		slog.Int("set_index", setIndex),
		slog.Float64("weight", weight),
		slog.Int("reps", reps))

	// Redirect to the clean URL (without the edit query parameter)
	redirectURL := fmt.Sprintf("/workouts/%s/exercises/%d", date.Format("2006-01-02"), exerciseID)
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}
