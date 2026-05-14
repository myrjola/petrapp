package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/myrjola/petrapp/internal/domain"
)

// adminExerciseRow is one row of the exercise admin table, with display
// values (category label, comma-joined muscle lists) prepared by the handler.
type adminExerciseRow struct {
	ID               int
	Name             string
	CategoryLabel    string
	PrimaryMuscles   string
	SecondaryMuscles string
}

// exerciseAdminTemplateData contains data for the exercise admin template.
type exerciseAdminTemplateData struct {
	BaseTemplateData
	Header    PageHeaderData
	Flash     BannerData
	Exercises []adminExerciseRow
	NameField FieldData
}

// MuscleGroupOption represents a muscle group with a selection state.
type MuscleGroupOption struct {
	Name     string
	Selected bool
}

// selectOption is one <option> of a single-select dropdown, with its
// selected state resolved by the handler.
type selectOption struct {
	Value    string
	Label    string
	Selected bool
}

// exerciseEditTemplateData contains data for the exercise edit template.
type exerciseEditTemplateData struct {
	BaseTemplateData
	Header                 PageHeaderData
	Flash                  BannerData
	Exercise               domain.Exercise
	NameField              FieldData
	SecondsField           FieldData
	RepMinField            FieldData
	RepMaxField            FieldData
	CategoryOptions        []selectOption
	TypeOptions            []selectOption
	PrimaryMuscleOptions   []MuscleGroupOption
	SecondaryMuscleOptions []MuscleGroupOption
}

// adminExercisesGET handles GET requests to the exercise admin page.
func (app *application) adminExercisesGET(w http.ResponseWriter, r *http.Request) {
	exercises, err := app.service.List(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	rows := make([]adminExerciseRow, 0, len(exercises))
	for _, ex := range exercises {
		rows = append(rows, adminExerciseRow{
			ID:               ex.ID,
			Name:             ex.Name,
			CategoryLabel:    ex.Category.Label(),
			PrimaryMuscles:   strings.Join(ex.PrimaryMuscleGroups, ", "),
			SecondaryMuscles: strings.Join(ex.SecondaryMuscleGroups, ", "),
		})
	}

	data := exerciseAdminTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Header:           PageHeaderData{Title: "Exercise Administration", Subtitle: ""},
		Flash:            BannerData{Variant: "error", Message: app.popFlashError(r.Context())},
		Exercises:        rows,
		NameField: FieldData{ //nolint:exhaustruct // labelled text input; native-validation attrs unused here.
			Label:    "Exercise Name",
			Name:     "name",
			Type:     "text",
			Required: true,
			Hint:     "e.g., Bench Press, Deadlift, Squat",
		},
	}

	app.render(w, r, http.StatusOK, "admin-exercises", data)
}

// adminExerciseEditGET handles GET requests to the exercise edit page.
func (app *application) adminExerciseEditGET(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		app.notFound(w, r)
		return
	}

	exercise, err := app.service.GetExercise(r.Context(), id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	muscleGroups, err := app.service.ListMuscleGroups(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
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
		BaseTemplateData: newBaseTemplateData(r),
		Header:           PageHeaderData{Title: fmt.Sprintf("Edit Exercise: %s", exercise.Name), Subtitle: ""},
		Flash:            BannerData{Variant: "error", Message: app.popFlashError(r.Context())},
		Exercise:         exercise,
		NameField: FieldData{ //nolint:exhaustruct // labelled text input; native-validation attrs unused here.
			Label:    "Name",
			Name:     "name",
			Type:     "text",
			Value:    exercise.Name,
			Required: true,
		},
		SecondsField: FieldData{ //nolint:exhaustruct // labelled number input; Max/Step/Pattern unused here.
			Label:    "Default Starting Seconds",
			Name:     "default_starting_seconds",
			Type:     "number",
			Value:    defaultSecondsValue,
			Required: exercise.IsTimed(),
			Hint:     "Number of seconds to hold on the first set for new users.",
			Min:      "1",
		},
		RepMinField: FieldData{ //nolint:exhaustruct // labelled number input; Hint/Step/Pattern unused here.
			Label:    "Min Reps",
			Name:     "rep_min",
			Type:     "number",
			Value:    repMinValue,
			Required: !exercise.IsTimed(),
			Min:      "1",
			Max:      "50",
		},
		RepMaxField: FieldData{ //nolint:exhaustruct // labelled number input; Hint/Step/Pattern unused here.
			Label:    "Max Reps",
			Name:     "rep_max",
			Type:     "number",
			Value:    repMaxValue,
			Required: !exercise.IsTimed(),
			Min:      "1",
			Max:      "50",
		},
		CategoryOptions:        buildCategoryOptions(exercise.Category),
		TypeOptions:            buildTypeOptions(exercise.ExerciseType),
		PrimaryMuscleOptions:   buildMuscleGroupOptions(muscleGroups, exercise.PrimaryMuscleGroups),
		SecondaryMuscleOptions: buildMuscleGroupOptions(muscleGroups, exercise.SecondaryMuscleGroups),
	}

	app.render(w, r, http.StatusOK, "admin-exercise-edit", data)
}

// adminExerciseUpdatePOST handles POST requests to update an exercise.
func (app *application) adminExerciseUpdatePOST(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		app.notFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, largeMaxFormSize)
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	// Time-based exercises carry a starting-seconds value; every other type
	// carries a rep window. The handler reads the type-appropriate fields;
	// Exercise.Validate enforces that the populated fields are valid.
	exerciseType := domain.ExerciseType(r.PostForm.Get("exercise_type"))
	var defaultStartingSeconds, repMin, repMax *int
	if exerciseType == domain.ExerciseTypeTime {
		defaultStartingSeconds = optionalInt(r.PostForm.Get("default_starting_seconds"))
	} else {
		repMin = optionalInt(r.PostForm.Get("rep_min"))
		repMax = optionalInt(r.PostForm.Get("rep_max"))
	}

	exercise := domain.Exercise{
		ID:                     id,
		Name:                   r.PostForm.Get("name"),
		Category:               domain.Category(r.PostForm.Get("category")),
		ExerciseType:           exerciseType,
		DescriptionMarkdown:    r.PostForm.Get("description"),
		PrimaryMuscleGroups:    r.PostForm["primary_muscles"],
		SecondaryMuscleGroups:  r.PostForm["secondary_muscles"],
		DefaultStartingSeconds: defaultStartingSeconds,
		RepMin:                 repMin,
		RepMax:                 repMax,
	}

	editPath := fmt.Sprintf("/admin/exercises/%d", id)
	if err = app.service.UpdateExercise(r.Context(), exercise); err != nil {
		var ve domain.ValidationError
		if errors.As(err, &ve) {
			app.putFlashError(r.Context(), ve.Message)
			redirect(w, r, editPath)
			return
		}
		app.serverError(w, r, err)
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "updated exercise",
		slog.Int("id", id),
		slog.String("name", exercise.Name))

	redirect(w, r, "/admin/exercises")
}

// adminExerciseGeneratePOST handles POST requests to generate a new exercise.
func (app *application) adminExerciseGeneratePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, err)
		return
	}

	name := r.PostForm.Get("name")
	exercise, err := app.service.GenerateExercise(r.Context(), name)
	if err != nil {
		var ve domain.ValidationError
		if errors.As(err, &ve) {
			app.putFlashError(r.Context(), ve.Message)
			redirect(w, r, "/admin/exercises")
			return
		}
		app.serverError(w, r, err)
		return
	}

	redirect(w, r, fmt.Sprintf("/admin/exercises/%d", exercise.ID))
}

// optionalInt parses a form field into an *int, returning nil for an empty or
// unparseable value. Native HTML validation guards the client; Exercise.Validate
// is the backstop for malformed input that reaches the server anyway.
func optionalInt(raw string) *int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &n
}

// buildCategoryOptions builds the category <select> options, marking the
// exercise's current category as selected.
func buildCategoryOptions(current domain.Category) []selectOption {
	return []selectOption{
		{
			Value:    string(domain.CategoryFullBody),
			Label:    domain.CategoryFullBody.Label(),
			Selected: current == domain.CategoryFullBody,
		},
		{
			Value:    string(domain.CategoryUpper),
			Label:    domain.CategoryUpper.Label(),
			Selected: current == domain.CategoryUpper,
		},
		{
			Value:    string(domain.CategoryLower),
			Label:    domain.CategoryLower.Label(),
			Selected: current == domain.CategoryLower,
		},
	}
}

// buildTypeOptions builds the exercise-type <select> options, marking the
// exercise's current type as selected.
func buildTypeOptions(current domain.ExerciseType) []selectOption {
	return []selectOption{
		{
			Value:    string(domain.ExerciseTypeWeighted),
			Label:    "Weighted",
			Selected: current == domain.ExerciseTypeWeighted,
		},
		{
			Value:    string(domain.ExerciseTypeBodyweight),
			Label:    "Bodyweight",
			Selected: current == domain.ExerciseTypeBodyweight,
		},
		{
			Value:    string(domain.ExerciseTypeAssisted),
			Label:    "Assisted",
			Selected: current == domain.ExerciseTypeAssisted,
		},
		{
			Value:    string(domain.ExerciseTypeTime),
			Label:    "Time-based",
			Selected: current == domain.ExerciseTypeTime,
		},
	}
}

// buildMuscleGroupOptions pairs every muscle group with whether it appears in
// the selected list, for rendering a <select multiple>.
func buildMuscleGroupOptions(groups, selected []string) []MuscleGroupOption {
	options := make([]MuscleGroupOption, len(groups))
	for i, group := range groups {
		options[i] = MuscleGroupOption{
			Name:     group,
			Selected: slices.Contains(selected, group),
		}
	}
	return options
}
