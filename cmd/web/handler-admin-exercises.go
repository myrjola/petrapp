package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"

	"github.com/myrjola/petrapp/internal/domain"
)

// exerciseAdminTemplateData contains data for the exercise admin template.
type exerciseAdminTemplateData struct {
	BaseTemplateData
	Exercises    []domain.Exercise
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
	Exercise                    domain.Exercise
	PrimaryMuscleOptions        []MuscleGroupOption
	SecondaryMuscleOptions      []MuscleGroupOption
	ValidationError             string
	DefaultStartingSecondsValue string
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
		app.notFound(w, r)
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

	defaultSecondsValue := ""
	if exercise.DefaultStartingSeconds != nil {
		defaultSecondsValue = strconv.Itoa(*exercise.DefaultStartingSeconds)
	}

	data := exerciseEditTemplateData{
		BaseTemplateData:            newBaseTemplateData(r),
		Exercise:                    exercise,
		PrimaryMuscleOptions:        primaryMuscleOptions,
		SecondaryMuscleOptions:      secondaryMuscleOptions,
		ValidationError:             app.popFlashError(r.Context()),
		DefaultStartingSecondsValue: defaultSecondsValue,
	}

	app.render(w, r, http.StatusOK, "admin-exercise-edit", data)
}

// adminExerciseUpdatePOST handles POST requests to update an exercise.
//
//nolint:funlen // linear form-validation work; splitting would harm readability for a 1-stmt threshold miss
func (app *application) adminExerciseUpdatePOST(w http.ResponseWriter, r *http.Request) {
	// Get exercise ID from URL
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		app.notFound(w, r)
		return
	}

	// Parse form
	r.Body = http.MaxBytesReader(w, r.Body, largeMaxFormSize)
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	// Extract form data
	name := r.PostForm.Get("name")
	category := domain.Category(r.PostForm.Get("category"))
	exerciseType := domain.ExerciseType(r.PostForm.Get("exercise_type"))
	description := r.PostForm.Get("description")
	primaryMuscles := r.PostForm["primary_muscles"]
	secondaryMuscles := r.PostForm["secondary_muscles"]

	// Validate form data. These are user input errors, not server errors —
	// surface them via the flash + redirect pattern instead of a 500.
	editPath := fmt.Sprintf("/admin/exercises/%d", id)
	if name == "" {
		app.putFlashError(r.Context(), "Name is required.")
		redirect(w, r, editPath)
		return
	}

	if category != domain.CategoryFullBody && category != domain.CategoryUpper && category != domain.CategoryLower {
		app.putFlashError(r.Context(), "Category must be one of full body, upper, or lower.")
		redirect(w, r, editPath)
		return
	}

	if exerciseType != domain.ExerciseTypeWeighted &&
		exerciseType != domain.ExerciseTypeBodyweight &&
		exerciseType != domain.ExerciseTypeAssisted &&
		exerciseType != domain.ExerciseTypeTime {
		app.putFlashError(r.Context(), "Exercise type must be weighted, bodyweight, assisted, or time_based.")
		redirect(w, r, editPath)
		return
	}

	var defaultStartingSeconds *int
	if exerciseType == domain.ExerciseTypeTime {
		raw := r.PostForm.Get("default_starting_seconds")
		n, atoiErr := strconv.Atoi(raw)
		if atoiErr != nil || n <= 0 {
			app.putFlashError(r.Context(), "Default starting seconds must be a positive integer for time-based exercises.")
			redirect(w, r, editPath)
			return
		}
		defaultStartingSeconds = &n
	}

	if len(primaryMuscles) == 0 {
		app.putFlashError(r.Context(), "At least one primary muscle group is required.")
		redirect(w, r, editPath)
		return
	}

	repMin, repMax, err := app.preserveRepWindow(r.Context(), id, exerciseType)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Create exercise object.
	exercise := domain.Exercise{
		ID:                     id,
		Name:                   name,
		Category:               category,
		ExerciseType:           exerciseType,
		DescriptionMarkdown:    description,
		PrimaryMuscleGroups:    primaryMuscles,
		SecondaryMuscleGroups:  secondaryMuscles,
		DefaultStartingSeconds: defaultStartingSeconds,
		RepMin:                 repMin,
		RepMax:                 repMax,
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
	redirect(w, r, "/admin/exercises")
}

// adminExerciseGeneratePOST handles POST requests to generate a new exercise.
func (app *application) adminExerciseGeneratePOST(w http.ResponseWriter, r *http.Request) {
	// Parse form.
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, err)
		return
	}

	// Extract exercise name from form.
	name := r.PostForm.Get("name")
	if name == "" {
		app.serverError(w, r, nil)
		return
	}

	// Generate the exercise.
	exercise, err := app.workoutService.GenerateExercise(r.Context(), name)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Redirect to the newly created exercise.
	redirect(w, r, fmt.Sprintf("/admin/exercises/%d", exercise.ID))
}

// preserveRepWindow returns the rep_min / rep_max for an existing exercise so
// that an admin update doesn't NULL them out. The admin edit form does not
// surface these fields yet, so preservation is the only path. Returns
// (nil, nil, nil) for time-based exercises, which don't carry a rep window.
func (app *application) preserveRepWindow(
	ctx context.Context, id int, exerciseType domain.ExerciseType,
) (*int, *int, error) {
	if exerciseType == domain.ExerciseTypeTime {
		return nil, nil, nil
	}
	existing, err := app.workoutService.GetExercise(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("get exercise for rep window: %w", err)
	}
	return existing.RepMin, existing.RepMax, nil
}
