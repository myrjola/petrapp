package main

import (
	"errors"
	"fmt"
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

const (
	difficultyTooEasy = iota + 1
	difficultyICouldDoMore
	difficultyJustRight
	difficultyVeryHard
	difficultyImpossible
)

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
			{Value: difficultyTooEasy, Label: "Too easy"},
			{Value: difficultyICouldDoMore, Label: "I could do more"},
			{Value: difficultyJustRight, Label: "Just right"},
			{Value: difficultyVeryHard, Label: "Very hard"},
			{Value: difficultyImpossible, Label: "Impossible"},
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
	if err = app.workoutService.StartSession(r.Context(), date); err != nil {
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
		app.serverError(w, r, fmt.Errorf("parse difficulty rating: %w", err))
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

// workoutSwapExerciseGET handles GET requests to show available exercises for swapping.
func (app *application) workoutSwapExerciseGET(w http.ResponseWriter, r *http.Request) {
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

	// Get current exercise
	currentExercise, err := app.workoutService.GetExercise(r.Context(), exerciseID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Get compatible exercises
	compatibleExercises, err := app.workoutService.FindCompatibleExercises(r.Context(), exerciseID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Prepare template data
	data := exerciseSwapTemplateData{
		BaseTemplateData:    newBaseTemplateData(r),
		Date:                date,
		CurrentExercise:     currentExercise,
		CompatibleExercises: compatibleExercises,
	}

	app.render(w, r, http.StatusOK, "exercise-swap", data)
}

// workoutSwapExercisePOST handles POST requests to swap an exercise.
func (app *application) workoutSwapExercisePOST(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse current exercise ID from URL path
	exerciseIDStr := r.PathValue("exerciseID")
	currentExerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse form
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	// Get new exercise ID from form
	newExerciseIDStr := r.PostForm.Get("new_exercise_id")
	if newExerciseIDStr == "" {
		app.serverError(w, r, errors.New("new exercise ID not provided"))
		return
	}

	newExerciseID, err := strconv.Atoi(newExerciseIDStr)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("parse new exercise ID: %w", err))
		return
	}

	// Swap exercise
	if err = app.workoutService.SwapExercise(r.Context(), date, currentExerciseID, newExerciseID); err != nil {
		app.serverError(w, r, err)
		return
	}

	// Redirect to exercise set page with the new exercise
	http.Redirect(w, r,
		fmt.Sprintf("/workouts/%s/exercises/%d", date.Format("2006-01-02"), newExerciseID), http.StatusSeeOther)
}

// exerciseSwapTemplateData contains data for the exercise swap template.
type exerciseSwapTemplateData struct {
	BaseTemplateData
	Date                time.Time
	CurrentExercise     workout.Exercise
	CompatibleExercises []workout.Exercise
}

// exerciseAddTemplateData contains data for the exercise add template.
type exerciseAddTemplateData struct {
	BaseTemplateData
	Date      time.Time
	Exercises []workout.Exercise
}

// workoutAddExerciseGET handles GET requests to show available exercises for adding.
func (app *application) workoutAddExerciseGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get all exercises
	exercises, err := app.workoutService.List(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Prepare template data
	data := exerciseAddTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Date:             date,
		Exercises:        exercises,
	}

	app.render(w, r, http.StatusOK, "exercise-add", data)
}

// workoutAddExercisePOST handles POST requests to add an exercise to a workout.
func (app *application) workoutAddExercisePOST(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse form
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	// Get exercise ID from form
	exerciseIDStr := r.PostForm.Get("exercise_id")
	if exerciseIDStr == "" {
		app.serverError(w, r, errors.New("exercise ID not provided"))
		return
	}

	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("parse exercise ID: %w", err))
		return
	}

	// Add exercise to workout
	if err = app.workoutService.AddExercise(r.Context(), date, exerciseID); err != nil {
		app.serverError(w, r, err)
		return
	}

	// Redirect to workout page
	http.Redirect(w, r, fmt.Sprintf("/workouts/%s", date.Format("2006-01-02")), http.StatusSeeOther)
}
