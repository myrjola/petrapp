package main

import (
	"errors"
	"fmt"
	"github.com/myrjola/petrapp/internal/workout"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
)

// exerciseAdminTemplateData contains data for the exercise admin template.
type exerciseAdminTemplateData struct {
	BaseTemplateData
	Exercises    []workout.Exercise
	MuscleGroups []string
}

// MuscleGroupOption represents a muscle group with a selection state.
type MuscleGroupOption struct {
	Name     string
	Selected bool
}

// exerciseEditTemplateData contains data for the exercise edit template.
type exerciseEditTemplateData struct {
	BaseTemplateData
	Exercise               workout.Exercise
	PrimaryMuscleOptions   []MuscleGroupOption
	SecondaryMuscleOptions []MuscleGroupOption
}

// adminExercisesGET handles GET requests to the exercise admin page.
func (app *application) adminExercisesGET(w http.ResponseWriter, r *http.Request) {
	// Get all exercises from the workout service
	exercises, err := app.workoutService.List(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Get all muscle groups
	muscleGroups, err := app.workoutService.ListMuscleGroups(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := exerciseAdminTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Exercises:        exercises,
		MuscleGroups:     muscleGroups,
	}

	app.render(w, r, http.StatusOK, "admin-exercises", data)
}

// adminExerciseEditGET handles GET requests to the exercise edit page.
func (app *application) adminExerciseEditGET(w http.ResponseWriter, r *http.Request) {
	// Get exercise ID from URL
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get exercise from workout service
	exercise, err := app.workoutService.GetExercise(r.Context(), id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Get all muscle groups
	muscleGroups, err := app.workoutService.ListMuscleGroups(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Create primary muscle options
	primaryMuscleOptions := make([]MuscleGroupOption, len(muscleGroups))
	for i, group := range muscleGroups {
		primaryMuscleOptions[i] = MuscleGroupOption{
			Name:     group,
			Selected: slices.Contains(exercise.PrimaryMuscleGroups, group),
		}
	}

	// Create secondary muscle options
	secondaryMuscleOptions := make([]MuscleGroupOption, len(muscleGroups))
	for i, group := range muscleGroups {
		secondaryMuscleOptions[i] = MuscleGroupOption{
			Name:     group,
			Selected: slices.Contains(exercise.SecondaryMuscleGroups, group),
		}
	}

	data := exerciseEditTemplateData{
		BaseTemplateData:       newBaseTemplateData(r),
		Exercise:               exercise,
		PrimaryMuscleOptions:   primaryMuscleOptions,
		SecondaryMuscleOptions: secondaryMuscleOptions,
	}

	app.render(w, r, http.StatusOK, "admin-exercise-edit", data)
}

// adminExerciseUpdatePOST handles POST requests to update an exercise.
func (app *application) adminExerciseUpdatePOST(w http.ResponseWriter, r *http.Request) {
	// Get exercise ID from URL
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse form
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	// Extract form data
	name := r.PostForm.Get("name")
	category := workout.Category(r.PostForm.Get("category"))
	description := r.PostForm.Get("description")
	primaryMuscles := r.PostForm["primary_muscles"]
	secondaryMuscles := r.PostForm["secondary_muscles"]

	// Validate form data
	if name == "" {
		app.serverError(w, r, errors.New("name is required"))
		return
	}

	if category != workout.CategoryFullBody && category != workout.CategoryUpper && category != workout.CategoryLower {
		app.serverError(w, r, errors.New("invalid category"))
		return
	}

	if len(primaryMuscles) == 0 {
		app.serverError(w, r, errors.New("primary muscle groups are required"))
		return
	}

	// Create exercise object
	exercise := workout.Exercise{
		ID:                    id,
		Name:                  name,
		Category:              category,
		DescriptionMarkdown:   description,
		PrimaryMuscleGroups:   primaryMuscles,
		SecondaryMuscleGroups: secondaryMuscles,
	}

	// Update exercise
	if err = app.workoutService.UpdateExercise(r.Context(), exercise); err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "updated exercise",
		slog.Int("id", id),
		slog.String("name", name))

	// Redirect to exercise list
	http.Redirect(w, r, "/admin/exercises", http.StatusSeeOther)
}

// adminExerciseCreatePOST handles POST requests to create a new exercise.
func (app *application) adminExerciseCreatePOST(w http.ResponseWriter, r *http.Request) {
	// Parse form
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	// Extract form data
	name := r.PostForm.Get("name")
	category := workout.Category(r.PostForm.Get("category"))
	description := r.PostForm.Get("description")
	primaryMuscles := r.PostForm["primary_muscles"]
	secondaryMuscles := r.PostForm["secondary_muscles"]

	// Validate form data
	if name == "" {
		app.serverError(w, r, errors.New("name is required"))
		return
	}

	if category != workout.CategoryFullBody && category != workout.CategoryUpper && category != workout.CategoryLower {
		app.serverError(w, r, errors.New("invalid category"))
		return
	}

	if len(primaryMuscles) == 0 {
		app.serverError(w, r, errors.New("primary muscle groups are required"))
		return
	}

	// Create exercise object
	exercise := workout.Exercise{
		ID:                    0,
		Name:                  name,
		Category:              category,
		DescriptionMarkdown:   description,
		PrimaryMuscleGroups:   primaryMuscles,
		SecondaryMuscleGroups: secondaryMuscles,
	}

	// Create exercise
	if err := app.workoutService.CreateExercise(r.Context(), exercise); err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "created exercise",
		slog.String("name", name))

	// Redirect to exercise list
	http.Redirect(w, r, "/admin/exercises", http.StatusSeeOther)
}
