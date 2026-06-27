package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/myrjola/petrapp/internal/petra/domain"
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
	AdminNav  AdminNavData
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
	AdminNav               AdminNavData
	Exercise               domain.Exercise
	NameField              FieldData
	SecondsField           FieldData
	RepMinField            FieldData
	RepMaxField            FieldData
	CategoryOptions        []selectOption
	TypeOptions            []selectOption
	PrimaryMuscleOptions   []MuscleGroupOption
	SecondaryMuscleOptions []MuscleGroupOption
	// Structured content rendered as line-delimited textarea bodies: one
	// instruction/mistake per line, resources as "Title | URL" per line.
	InstructionsText   string
	CommonMistakesText string
	ResourcesText      string
}

// adminExercisesGET handles GET requests to the exercise admin page.
func (app *application) adminExercisesGET(w http.ResponseWriter, r *http.Request) {
	exercises, err := app.service.ListExercises(r.Context())
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

	base := newBaseTemplateData(r)
	flash := app.popFlash(r.Context())
	data := exerciseAdminTemplateData{
		BaseTemplateData: base,
		Header: PageHeaderData{
			Title:    "Exercise Administration",
			Subtitle: "",
			Nonce:    base.Nonce,
		},
		Flash: BannerData{
			Variant: flash.Variant,
			Message: flash.Message,
			Nonce:   base.Nonce,
		},
		AdminNav: AdminNavData{
			Active: adminSectionExercises,
			Nonce:  base.Nonce,
		},
		Exercises: rows,
		NameField: FieldData{ //nolint:exhaustruct // labelled text input; native-validation attrs unused here.
			Label:    "Exercise Name",
			Name:     "name",
			Type:     inputTypeText,
			Required: true,
			Hint:     "e.g., Bench Press, Deadlift, Squat",
			Nonce:    base.Nonce,
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

	base := newBaseTemplateData(r)
	flash := app.popFlash(r.Context())
	data := exerciseEditTemplateData{
		BaseTemplateData: base,
		Header: PageHeaderData{
			Title:    fmt.Sprintf("Edit Exercise: %s", exercise.Name),
			Subtitle: "",
			Nonce:    base.Nonce,
		},
		Flash: BannerData{
			Variant: flash.Variant,
			Message: flash.Message,
			Nonce:   base.Nonce,
		},
		AdminNav: AdminNavData{
			Active: adminSectionExercises,
			Nonce:  base.Nonce,
		},
		Exercise: exercise,
		NameField: FieldData{ //nolint:exhaustruct // labelled text input; native-validation attrs unused here.
			Label:    "Name",
			Name:     "name",
			Type:     inputTypeText,
			Value:    exercise.Name,
			Required: true,
			Nonce:    base.Nonce,
		},
		SecondsField: FieldData{ //nolint:exhaustruct // labelled number input; Max/Step/Pattern unused here.
			Label:    "Default Starting Seconds",
			Name:     "default_starting_seconds",
			Type:     inputTypeNumber,
			Value:    defaultSecondsValue,
			Required: exercise.IsTimed(),
			Hint:     "Number of seconds to hold on the first set for new users.",
			Min:      "1",
			Nonce:    base.Nonce,
		},
		RepMinField: FieldData{ //nolint:exhaustruct // labelled number input; Hint/Step/Pattern unused here.
			Label:    "Min Reps",
			Name:     "rep_min",
			Type:     inputTypeNumber,
			Value:    repMinValue,
			Required: !exercise.IsTimed(),
			Min:      "1",
			Max:      "50",
			Nonce:    base.Nonce,
		},
		RepMaxField: FieldData{ //nolint:exhaustruct // labelled number input; Hint/Step/Pattern unused here.
			Label:    "Max Reps",
			Name:     "rep_max",
			Type:     inputTypeNumber,
			Value:    repMaxValue,
			Required: !exercise.IsTimed(),
			Min:      "1",
			Max:      "50",
			Nonce:    base.Nonce,
		},
		CategoryOptions:        buildCategoryOptions(exercise.Category),
		TypeOptions:            buildTypeOptions(exercise.ExerciseType),
		PrimaryMuscleOptions:   buildMuscleGroupOptions(muscleGroups, exercise.PrimaryMuscleGroups),
		SecondaryMuscleOptions: buildMuscleGroupOptions(muscleGroups, exercise.SecondaryMuscleGroups),
		InstructionsText:       strings.Join(exercise.Instructions, "\n"),
		CommonMistakesText:     strings.Join(exercise.CommonMistakes, "\n"),
		ResourcesText:          formatResourcesText(exercise.Resources),
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

	if !app.parseForm(w, r, largeMaxFormSize) {
		return
	}

	// Time-based exercises carry a starting-seconds value; every other type
	// carries a rep range. The handler reads the type-appropriate fields;
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
		Instructions:           splitLines(r.PostForm.Get("instructions")),
		CommonMistakes:         splitLines(r.PostForm.Get("common_mistakes")),
		Resources:              parseResourcesText(r.PostForm.Get("resources")),
		PrimaryMuscleGroups:    r.PostForm["primary_muscles"],
		SecondaryMuscleGroups:  r.PostForm["secondary_muscles"],
		DefaultStartingSeconds: defaultStartingSeconds,
		RepMin:                 repMin,
		RepMax:                 repMax,
	}

	editPath := fmt.Sprintf("/admin/exercises/%d", id)
	if err = app.service.UpdateExercise(r.Context(), exercise); err != nil {
		app.userError(w, r, err, editPath)
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "updated exercise",
		slog.Int("id", id),
		slog.String("name", exercise.Name))

	redirect(w, r, "/admin/exercises")
}

// adminExerciseGeneratePOST handles POST requests to generate a new exercise.
func (app *application) adminExerciseGeneratePOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}

	name := r.PostForm.Get("name")
	exercise, err := app.service.GenerateExercise(r.Context(), name)
	if err != nil {
		app.userError(w, r, err, "/admin/exercises")
		return
	}

	redirect(w, r, fmt.Sprintf("/admin/exercises/%d", exercise.ID))
}

// splitLines splits a textarea body into trimmed, non-empty lines — the
// deterministic transform that turns one-item-per-line authoring into a
// structured slice. It tolerates CRLF and blank lines.
func splitLines(raw string) []string {
	var out []string
	for line := range strings.SplitSeq(raw, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// parseResourcesText parses the resources textarea — one "Title | URL" per
// line — into domain.Resource values. Lines without a separator, or missing
// either side, are dropped; Exercise.Validate is the backstop for what remains.
func parseResourcesText(raw string) []domain.Resource {
	var out []domain.Resource
	for _, line := range splitLines(raw) {
		title, url, ok := strings.Cut(line, "|")
		if !ok {
			continue
		}
		title, url = strings.TrimSpace(title), strings.TrimSpace(url)
		if title == "" || url == "" {
			continue
		}
		out = append(out, domain.Resource{Title: title, URL: url})
	}
	return out
}

// formatResourcesText renders resources back into the "Title | URL" per-line
// form the edit textarea expects, the inverse of parseResourcesText.
func formatResourcesText(resources []domain.Resource) string {
	lines := make([]string, len(resources))
	for i, res := range resources {
		lines[i] = res.Title + " | " + res.URL
	}
	return strings.Join(lines, "\n")
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
