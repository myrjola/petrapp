package main

import (
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
	RepMinValue                 string
	RepMaxValue                 string
}

// adminExercisesGET handles GET requests to the exercise admin page.
func (app *application) adminExercisesGET(w http.ResponseWriter, r *http.Request) {
	// Get all exercises from the workout service
	exercises, err := app.service.List(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Get all muscle groups
	muscleGroups, err := app.service.ListMuscleGroups(r.Context())
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
	exercise, err := app.service.GetExercise(r.Context(), id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Get all muscle groups
	muscleGroups, err := app.service.ListMuscleGroups(r.Context())
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
	repMinValue := ""
	if exercise.RepMin != nil {
		repMinValue = strconv.Itoa(*exercise.RepMin)
	}
	repMaxValue := ""
	if exercise.RepMax != nil {
		repMaxValue = strconv.Itoa(*exercise.RepMax)
	}

	data := exerciseEditTemplateData{
		BaseTemplateData:            newBaseTemplateData(r),
		Exercise:                    exercise,
		PrimaryMuscleOptions:        primaryMuscleOptions,
		SecondaryMuscleOptions:      secondaryMuscleOptions,
		ValidationError:             app.popFlashError(r.Context()),
		DefaultStartingSecondsValue: defaultSecondsValue,
		RepMinValue:                 repMinValue,
		RepMaxValue:                 repMaxValue,
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

	if !category.IsValid() {
		app.putFlashError(r.Context(), "Category must be one of full body, upper, or lower.")
		redirect(w, r, editPath)
		return
	}

	if !exerciseType.IsValid() {
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

	repMin, repMax, repErr := parseRepWindow(r.PostForm.Get("rep_min"), r.PostForm.Get("rep_max"), exerciseType)
	if repErr != "" {
		app.putFlashError(r.Context(), repErr)
		redirect(w, r, editPath)
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
	if err = app.service.UpdateExercise(r.Context(), exercise); err != nil {
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
	exercise, err := app.service.GenerateExercise(r.Context(), name)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Redirect to the newly created exercise.
	redirect(w, r, fmt.Sprintf("/admin/exercises/%d", exercise.ID))
}

// parseRepWindow validates and converts the rep_min/rep_max form fields.
// Time-based exercises don't carry a rep window — both inputs are dropped
// and the function returns (nil, nil, ""). For every other exercise type
// both values must be present, parse as ints in [1, 50], and satisfy
// min <= max. On validation failure the returned string is non-empty and
// suitable to surface as a flash error.
func parseRepWindow(rawMin, rawMax string, exerciseType domain.ExerciseType) (*int, *int, string) {
	if exerciseType == domain.ExerciseTypeTime {
		return nil, nil, ""
	}
	const repBoundsError = "Min and max reps must be whole numbers between 1 and 50."
	minVal, err := strconv.Atoi(rawMin)
	if err != nil || minVal < 1 || minVal > 50 {
		return nil, nil, repBoundsError
	}
	maxVal, err := strconv.Atoi(rawMax)
	if err != nil || maxVal < 1 || maxVal > 50 {
		return nil, nil, repBoundsError
	}
	if minVal > maxVal {
		return nil, nil, "Min reps must be less than or equal to max reps."
	}
	return &minVal, &maxVal, ""
}
