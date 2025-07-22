package main

import (
	"errors"
	"fmt"
	"github.com/myrjola/petrapp/internal/workout"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type exerciseSetTemplateData struct {
	BaseTemplateData
	Date                 time.Time
	ExerciseSet          workout.ExerciseSet
	FirstIncompleteIndex int
	EditingIndex         int        // Index of the set being edited
	IsEditing            bool       // Whether we're in edit mode
	LastCompletedAt      *time.Time // Timestamp of most recently completed set
}

func getFirstIncompleteIndex(sets []workout.Set) int {
	for i, set := range sets {
		if set.CompletedReps == nil {
			return i
		}
	}
	return len(sets)
}

func getLastCompletedAt(sets []workout.Set) *time.Time {
	var latest *time.Time
	for _, set := range sets {
		if set.CompletedAt != nil {
			if latest == nil || set.CompletedAt.After(*latest) {
				latest = set.CompletedAt
			}
		}
	}
	return latest
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
		var idx int
		if idx, err = strconv.Atoi(editIndexStr); err == nil {
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
		LastCompletedAt:      getLastCompletedAt(exerciseSet.Sets),
	}

	app.render(w, r, http.StatusOK, "exerciseset", data)
}

// parseExerciseSetURLParams extracts and validates URL parameters for exercise set operations.
func (app *application) parseExerciseSetURLParams(r *http.Request) (time.Time, int, int, string, error) {
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, 0, 0, "", fmt.Errorf("parse date: %w", err)
	}

	exerciseIDStr := r.PathValue("exerciseID")
	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		return time.Time{}, 0, 0, "", fmt.Errorf("parse exercise ID: %w", err)
	}

	setIndexStr := r.PathValue("setIndex")
	setIndex, err := strconv.Atoi(setIndexStr)
	if err != nil {
		return time.Time{}, 0, 0, "", fmt.Errorf("parse set index: %w", err)
	}

	return date, exerciseID, setIndex, dateStr, nil
}

// findExerciseInSession finds an exercise by ID in the given session.
func (app *application) findExerciseInSession(session *workout.Session, exerciseID int) (workout.Exercise, bool) {
	for _, es := range session.ExerciseSets {
		if es.Exercise.ID == exerciseID {
			return es.Exercise, true
		}
	}
	return workout.Exercise{
		ID:                    0,
		Name:                  "",
		Category:              "",
		ExerciseType:          "",
		DescriptionMarkdown:   "",
		PrimaryMuscleGroups:   nil,
		SecondaryMuscleGroups: nil,
	}, false
}

// parseWeightAndReps extracts weight and reps from form data based on exercise type.
func (app *application) parseWeightAndReps(r *http.Request, exercise workout.Exercise) (float64, int, error) {
	var weight float64
	if exercise.ExerciseType == workout.ExerciseTypeWeighted {
		weightStr := r.PostForm.Get("weight")
		if weightStr == "" {
			return 0, 0, errors.New("weight not provided for weighted exercise")
		}
		// Replace comma with dot for decimal numbers.
		weightStr = strings.Replace(weightStr, ",", ".", 1)

		var err error
		weight, err = strconv.ParseFloat(weightStr, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("parse weight: %w", err)
		}
	}

	repsStr := r.PostForm.Get("reps")
	if repsStr == "" {
		return 0, 0, errors.New("reps not provided")
	}

	reps, err := strconv.Atoi(repsStr)
	if err != nil {
		return 0, 0, fmt.Errorf("parse reps: %w", err)
	}

	return weight, reps, nil
}

func (app *application) exerciseSetUpdatePOST(w http.ResponseWriter, r *http.Request) {
	// Parse URL parameters
	date, exerciseID, setIndex, dateStr, err := app.parseExerciseSetURLParams(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse form for weight and reps
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	// Get the exercise to check if it's bodyweight or weighted
	session, err := app.workoutService.GetSession(r.Context(), date)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	exercise, found := app.findExerciseInSession(&session, exerciseID)
	if !found {
		http.NotFound(w, r)
		return
	}

	// Parse weight and reps from form
	weight, reps, err := app.parseWeightAndReps(r, exercise)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Update weight only for weighted exercises
	if exercise.ExerciseType == workout.ExerciseTypeWeighted {
		if err = app.workoutService.UpdateSetWeight(r.Context(), date, exerciseID, setIndex, weight); err != nil {
			app.serverError(w, r, fmt.Errorf("update weight: %w", err))
			return
		}
	}

	// Update the completed reps
	if err = app.workoutService.UpdateCompletedReps(r.Context(), date, exerciseID, setIndex, reps); err != nil {
		app.serverError(w, r, fmt.Errorf("update completed reps: %w", err))
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
	redirect(w, r, redirectURL)
}

func (app *application) exerciseSetWarmupCompletePOST(w http.ResponseWriter, r *http.Request) {
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

	// Mark warmup as complete
	if err = app.workoutService.MarkWarmupComplete(r.Context(), date, exerciseID); err != nil {
		app.serverError(w, r, fmt.Errorf("mark warmup complete: %w", err))
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "warmup completed",
		slog.String("date", dateStr),
		slog.Int("exercise_id", exerciseID))

	// Redirect back to the exercise set page
	redirectURL := fmt.Sprintf("/workouts/%s/exercises/%d", date.Format("2006-01-02"), exerciseID)
	redirect(w, r, redirectURL)
}
