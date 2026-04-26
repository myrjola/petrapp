package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
	"github.com/myrjola/petrapp/internal/workout"
)

type setDisplay struct {
	Set    workout.Set
	RepStr string // Formatted rep string (e.g. "8" or "6-8")
	Number int    // 1-based set number for display.
}

type exerciseSetTemplateData struct {
	BaseTemplateData
	Date                 time.Time
	ExerciseSet          workout.ExerciseSet
	SetsDisplay          []setDisplay // Enhanced set data with formatted rep strings
	FirstIncompleteIndex int
	EditingIndex         int                           // Index of the set being edited
	IsEditing            bool                          // Whether we're in edit mode
	LastCompletedAt      *time.Time                    // Timestamp of most recently completed set
	CurrentSetTarget     exerciseprogression.SetTarget // Recommended weight and reps from progression
}

func formatRepRange(minReps, maxReps int) string {
	if minReps == maxReps {
		return strconv.Itoa(minReps)
	}
	return fmt.Sprintf("%d-%d", minReps, maxReps)
}

func prepareSetsDisplay(sets []workout.Set) []setDisplay {
	displays := make([]setDisplay, len(sets))
	for i, set := range sets {
		displays[i] = setDisplay{
			Set:    set,
			RepStr: formatRepRange(set.MinReps, set.MaxReps),
			Number: i + 1,
		}
	}
	return displays
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
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	// Parse exercise ID from URL path
	exerciseID, ok := app.parseExerciseIDParam(w, r)
	if !ok {
		return
	}

	// Check if edit parameter is provided
	editingIndex := -1
	isEditing := false
	editIndexStr := r.URL.Query().Get("edit")
	if editIndexStr != "" {
		var idx int
		var err error
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
		app.notFound(w, r)
		return
	}

	var currentSetTarget exerciseprogression.SetTarget
	if exerciseSet.Exercise.ExerciseType == workout.ExerciseTypeWeighted {
		progression, progressionErr := app.workoutService.BuildProgression(r.Context(), date, exerciseID)
		if progressionErr != nil {
			app.serverError(w, r, progressionErr)
			return
		}
		currentSetTarget = progression.CurrentSet()
	}

	data := exerciseSetTemplateData{
		BaseTemplateData:     newBaseTemplateData(r),
		Date:                 date,
		ExerciseSet:          exerciseSet,
		SetsDisplay:          prepareSetsDisplay(exerciseSet.Sets),
		FirstIncompleteIndex: getFirstIncompleteIndex(exerciseSet.Sets),
		EditingIndex:         editingIndex,
		IsEditing:            isEditing,
		LastCompletedAt:      getLastCompletedAt(exerciseSet.Sets),
		CurrentSetTarget:     currentSetTarget,
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

// recordWeightedSetCompletion handles parsing and persisting a weighted set completion from form data.
func (app *application) recordWeightedSetCompletion(
	w http.ResponseWriter, r *http.Request,
	date time.Time, exerciseID, setIndex int, dateStr string,
) bool {
	weightStr := strings.Replace(r.PostForm.Get("weight"), ",", ".", 1)
	weight, err := strconv.ParseFloat(weightStr, 64)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("parse weight: %w", err))
		return false
	}

	signal := workout.Signal(r.PostForm.Get("signal"))

	var reps int
	if signal == workout.SignalTooHeavy {
		reps, err = strconv.Atoi(r.PostForm.Get("reps"))
		if err != nil {
			app.serverError(w, r, fmt.Errorf("parse reps: %w", err))
			return false
		}
	} else {
		reps, err = strconv.Atoi(r.PostForm.Get("target_reps"))
		if err != nil {
			app.serverError(w, r, fmt.Errorf("parse target_reps: %w", err))
			return false
		}
	}

	err = app.workoutService.RecordSetCompletion(r.Context(), date, exerciseID, setIndex, signal, weight, reps)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("record set completion: %w", err))
		return false
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "recorded set completion",
		slog.String("date", dateStr),
		slog.Int("exercise_id", exerciseID),
		slog.Int("set_index", setIndex),
		slog.String("signal", string(signal)),
		slog.Float64("weight", weight),
		slog.Int("reps", reps))
	return true
}

func (app *application) exerciseSetUpdatePOST(w http.ResponseWriter, r *http.Request) {
	date, exerciseID, setIndex, dateStr, err := app.parseExerciseSetURLParams(r)
	if err != nil {
		app.notFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	session, err := app.workoutService.GetSession(r.Context(), date)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	exercise, found := app.findExerciseInSession(&session, exerciseID)
	if !found {
		app.notFound(w, r)
		return
	}

	if exercise.ExerciseType == workout.ExerciseTypeWeighted {
		if !app.recordWeightedSetCompletion(w, r, date, exerciseID, setIndex, dateStr) {
			return
		}
	} else {
		_, reps, parseErr := app.parseWeightAndReps(r, exercise)
		if parseErr != nil {
			app.serverError(w, r, parseErr)
			return
		}
		if err = app.workoutService.UpdateCompletedReps(r.Context(), date, exerciseID, setIndex, reps); err != nil {
			app.serverError(w, r, fmt.Errorf("update completed reps: %w", err))
			return
		}
	}

	app.redirectAfterPOST(w, r, fmt.Sprintf("/workouts/%s/exercises/%d", date.Format("2006-01-02"), exerciseID), "")
}

func (app *application) exerciseSetWarmupCompletePOST(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	// Parse exercise ID from URL path
	exerciseID, ok := app.parseExerciseIDParam(w, r)
	if !ok {
		return
	}

	// Mark warmup as complete
	if err := app.workoutService.MarkWarmupComplete(r.Context(), date, exerciseID); err != nil {
		app.serverError(w, r, fmt.Errorf("mark warmup complete: %w", err))
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "warmup completed",
		slog.String("date", date.Format("2006-01-02")),
		slog.Int("exercise_id", exerciseID))

	// Redirect back to the exercise set page
	redirectURL := fmt.Sprintf("/workouts/%s/exercises/%d", date.Format("2006-01-02"), exerciseID)
	app.redirectAfterPOST(w, r, redirectURL, "")
}
