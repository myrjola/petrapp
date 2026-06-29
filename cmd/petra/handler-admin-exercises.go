package main

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

// Exercise-form field names. These MUST match the keys
// domain.Exercise.Validate attaches messages to and the name attributes the
// form components render, so a per-field error reaches the right input. Defined
// once here as the single source of truth on the cmd/petra side.
const (
	exFieldName             = "name"
	exFieldCategory         = "category"
	exFieldType             = "exercise_type"
	exFieldStartingSeconds  = "default_starting_seconds"
	exFieldRepMin           = "rep_min"
	exFieldRepMax           = "rep_max"
	exFieldPrimaryMuscles   = "primary_muscles"
	exFieldSecondaryMuscles = "secondary_muscles"
	exFieldInstructions     = "instructions"
	exFieldCommonMistakes   = "common_mistakes"
	exFieldResources        = "resources"
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

// exerciseEditTemplateData contains data for the exercise edit template.
type exerciseEditTemplateData struct {
	BaseTemplateData

	Header       PageHeaderData
	Flash        BannerData       // business-error banner; empty on a field bounce
	ErrorSummary ErrorSummaryData // field-error summary; empty otherwise
	AdminNav     AdminNavData
	Exercise     domain.Exercise
	NameField    FieldData
	SecondsField FieldData
	RepMinField  FieldData
	RepMaxField  FieldData
	// Selects and line-delimited textareas (one instruction/mistake per line,
	// resources as "Title | URL" per line) rendered through shared components.
	CategorySelect        SelectData
	TypeSelect            SelectData
	PrimaryMuscleSelect   SelectData
	SecondaryMuscleSelect SelectData
	InstructionsField     TextareaData
	CommonMistakesField   TextareaData
	ResourcesField        TextareaData
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
			Live:    true,
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

	base := newBaseTemplateData(r)
	flash := app.popFlash(r.Context())
	// On a validation bounce, fep carries the user's submitted values + the
	// per-field messages; fep.value/multi fall back to the DB state otherwise.
	fep := app.popFormError(r.Context())
	data := newExerciseEditData(base, exercise, muscleGroups, flash, fep)

	app.render(w, r, http.StatusOK, "admin-exercise-edit", data)
}

// newExerciseEditData assembles the edit-page view model. Every control's value
// is the user's submitted value when fep is a validation bounce, else the DB
// state; each control's Error comes from fep.Fields keyed by its form name. The
// page-top banner carries only a business-error flash (empty on a field bounce,
// where the error-summary owns focus instead).
func newExerciseEditData(
	base BaseTemplateData, exercise domain.Exercise, muscleGroups []string,
	flash flashEntry, fep formErrorPayload,
) exerciseEditTemplateData {
	secondsValue := ""
	if exercise.DefaultStartingSeconds != nil {
		secondsValue = strconv.Itoa(*exercise.DefaultStartingSeconds)
	}
	repMinValue := ""
	if exercise.RepMin != nil {
		repMinValue = strconv.Itoa(*exercise.RepMin)
	}
	repMaxValue := ""
	if exercise.RepMax != nil {
		repMaxValue = strconv.Itoa(*exercise.RepMax)
	}

	return exerciseEditTemplateData{
		BaseTemplateData: base,
		Header: PageHeaderData{
			Title:    fmt.Sprintf("Edit Exercise: %s", exercise.Name),
			Subtitle: "",
			Nonce:    base.Nonce,
		},
		Flash: BannerData{
			Variant: flash.Variant,
			Message: flash.Message,
			Live:    true,
			Nonce:   base.Nonce,
		},
		ErrorSummary: buildExerciseErrorSummary(fep, base.Nonce),
		AdminNav: AdminNavData{
			Active: adminSectionExercises,
			Nonce:  base.Nonce,
		},
		Exercise: exercise,
		NameField: FieldData{ //nolint:exhaustruct // labelled text input; native-validation attrs unused here.
			Label:    "Name",
			Name:     exFieldName,
			Type:     inputTypeText,
			Value:    fep.value(exFieldName, exercise.Name),
			Error:    fep.Fields[exFieldName],
			Required: true,
			Nonce:    base.Nonce,
		},
		SecondsField: FieldData{ //nolint:exhaustruct // labelled number input; Max/Step/Pattern unused here.
			Label:    "Default Starting Seconds",
			Name:     exFieldStartingSeconds,
			Type:     inputTypeNumber,
			Value:    fep.value(exFieldStartingSeconds, secondsValue),
			Error:    fep.Fields[exFieldStartingSeconds],
			Required: exercise.IsTimed(),
			Hint:     "Number of seconds to hold on the first set for new users.",
			Min:      "1",
			Nonce:    base.Nonce,
		},
		RepMinField: FieldData{ //nolint:exhaustruct // labelled number input; Step/Pattern unused here.
			Label:    "Min Reps",
			Name:     exFieldRepMin,
			Type:     inputTypeNumber,
			Value:    fep.value(exFieldRepMin, repMinValue),
			Error:    fep.Fields[exFieldRepMin],
			Required: !exercise.IsTimed(),
			Min:      "1",
			Max:      "50",
			Nonce:    base.Nonce,
		},
		RepMaxField: FieldData{ //nolint:exhaustruct // labelled number input; Step/Pattern unused here.
			Label:    "Max Reps",
			Name:     exFieldRepMax,
			Type:     inputTypeNumber,
			Value:    fep.value(exFieldRepMax, repMaxValue),
			Error:    fep.Fields[exFieldRepMax],
			Required: !exercise.IsTimed(),
			Min:      "1",
			Max:      "50",
			Nonce:    base.Nonce,
		},
		CategorySelect: buildCategorySelect(
			fep.value(exFieldCategory, string(exercise.Category)), fep.Fields[exFieldCategory], base.Nonce),
		TypeSelect: buildTypeSelect(
			fep.value(exFieldType, string(exercise.ExerciseType)), fep.Fields[exFieldType], base.Nonce),
		PrimaryMuscleSelect: buildMuscleSelect(exFieldPrimaryMuscles, "Primary Muscle Groups",
			muscleGroups, fep.multi(exFieldPrimaryMuscles, exercise.PrimaryMuscleGroups),
			true, fep.Fields[exFieldPrimaryMuscles], base.Nonce),
		SecondaryMuscleSelect: buildMuscleSelect(exFieldSecondaryMuscles, "Secondary Muscle Groups",
			muscleGroups, fep.multi(exFieldSecondaryMuscles, exercise.SecondaryMuscleGroups),
			false, fep.Fields[exFieldSecondaryMuscles], base.Nonce),
		InstructionsField: TextareaData{
			Label: "Instructions", Name: exFieldInstructions,
			Value: fep.value(exFieldInstructions, strings.Join(exercise.Instructions, "\n")),
			Rows:  "8", Hint: "One step per line.", Error: fep.Fields[exFieldInstructions], Nonce: base.Nonce,
		},
		CommonMistakesField: TextareaData{
			Label: "Common Mistakes", Name: exFieldCommonMistakes,
			Value: fep.value(exFieldCommonMistakes, strings.Join(exercise.CommonMistakes, "\n")),
			Rows:  "6", Hint: "One mistake per line.", Error: fep.Fields[exFieldCommonMistakes], Nonce: base.Nonce,
		},
		ResourcesField: TextareaData{
			Label: "Resources", Name: exFieldResources,
			Value: fep.value(exFieldResources, formatResourcesText(exercise.Resources)),
			Rows:  "4", Hint: "One per line, as Title | https://example.com",
			Error: fep.Fields[exFieldResources], Nonce: base.Nonce,
		},
	}
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
	exerciseType := domain.ExerciseType(r.PostForm.Get(exFieldType))
	var defaultStartingSeconds, repMin, repMax *int
	if exerciseType == domain.ExerciseTypeTime {
		defaultStartingSeconds = optionalInt(r.PostForm.Get(exFieldStartingSeconds))
	} else {
		repMin = optionalInt(r.PostForm.Get(exFieldRepMin))
		repMax = optionalInt(r.PostForm.Get(exFieldRepMax))
	}

	exercise := domain.Exercise{
		ID:                     id,
		Name:                   r.PostForm.Get(exFieldName),
		Category:               domain.Category(r.PostForm.Get(exFieldCategory)),
		ExerciseType:           exerciseType,
		Instructions:           splitLines(r.PostForm.Get(exFieldInstructions)),
		CommonMistakes:         splitLines(r.PostForm.Get(exFieldCommonMistakes)),
		Resources:              parseResourcesText(r.PostForm.Get(exFieldResources)),
		PrimaryMuscleGroups:    r.PostForm[exFieldPrimaryMuscles],
		SecondaryMuscleGroups:  r.PostForm[exFieldSecondaryMuscles],
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

// buildCategorySelect wraps the category options in a SelectData, marking the
// current value selected and attaching any validation error.
func buildCategorySelect(current, errMsg string, nonce template.HTMLAttr) SelectData {
	return SelectData{
		Label:    "Category",
		Name:     "category",
		Options:  buildCategoryOptions(domain.Category(current)),
		Multiple: false,
		Required: true,
		Hint:     "",
		Error:    errMsg,
		Nonce:    nonce,
	}
}

// buildTypeSelect wraps the exercise-type options in a SelectData. The form's
// toggle script keys off this select's value to show/hide the seconds and
// rep-range fields, so it keeps id == name.
func buildTypeSelect(current, errMsg string, nonce template.HTMLAttr) SelectData {
	return SelectData{
		Label:    "Exercise Type",
		Name:     "exercise_type",
		Options:  buildTypeOptions(domain.ExerciseType(current)),
		Multiple: false,
		Required: true,
		Hint:     "",
		Error:    errMsg,
		Nonce:    nonce,
	}
}

// buildMuscleSelect builds a multi-select of every muscle group, marking those
// in selected. Used for both the primary (required) and secondary lists.
func buildMuscleSelect(
	name, label string, groups, selected []string, required bool, errMsg string, nonce template.HTMLAttr,
) SelectData {
	options := make([]selectOption, len(groups))
	for i, group := range groups {
		options[i] = selectOption{
			Value:    group,
			Label:    group,
			Selected: slices.Contains(selected, group),
		}
	}
	return SelectData{
		Label:    label,
		Name:     name,
		Options:  options,
		Multiple: true,
		Required: required,
		Hint:     "Hold Ctrl/Cmd to select multiple.",
		Error:    errMsg,
		Nonce:    nonce,
	}
}

// buildExerciseErrorSummary turns a popped form-error payload into the
// error-summary dot. Items follow the form's field order so the summary reads
// top-to-bottom like the page; a message already listed is skipped so the
// rep-range error (attached to both rep_min and rep_max) appears once. Returns
// the zero value (renders nothing) when there is no payload.
func buildExerciseErrorSummary(fep formErrorPayload, nonce template.HTMLAttr) ErrorSummaryData {
	fieldOrder := []string{
		exFieldName, exFieldCategory, exFieldType, exFieldStartingSeconds,
		exFieldRepMin, exFieldRepMax, exFieldPrimaryMuscles, exFieldSecondaryMuscles,
		exFieldInstructions, exFieldCommonMistakes, exFieldResources,
	}
	var items []ErrorSummaryItem
	seen := make(map[string]bool)
	for _, name := range fieldOrder {
		msg, ok := fep.Fields[name]
		if !ok || seen[msg] {
			continue
		}
		seen[msg] = true
		items = append(items, ErrorSummaryItem{Anchor: name, Message: msg})
	}
	var form []string
	if fep.FormMessage != "" {
		form = []string{fep.FormMessage}
	}
	return ErrorSummaryData{
		Title: "",
		Items: items,
		Form:  form,
		Live:  fep.has(),
		Nonce: nonce,
	}
}
