package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

// signalFormField is both the POST form field name and the slog attribute key
// for the signal value (too-heavy/too-light) attached to a recorded set.
const signalFormField = "signal"

type setDisplay struct {
	Set          domain.Set
	TargetStr    string // Pre-formatted target string (e.g. "5", "30s").
	CompletedStr string // Pre-formatted completed string, same unit as TargetStr.
	Unit         string // "reps" or "seconds" — for input labels.
	Number       int    // 1-based set number for display.
	IsActive     bool   // Whether this row renders the completion form.
	SignalLabel  string // Human label ("too heavy"/"too light"/""). "" hides the badge.
	SignalGlyph  string // Direction glyph ("↓"/"↑"/""). Empty when Label is empty.
}

type exerciseSetTemplateData struct {
	BaseTemplateData

	Date                  time.Time
	Position              int // Slot's 0-based index in Session.Slots, used for URL building and view-transition names.
	ExerciseSlot          domain.ExerciseSlot
	SetsDisplay           []setDisplay // Enhanced set data with formatted rep strings
	FirstIncompleteIndex  int
	EditingIndex          int              // Index of the set being edited
	IsEditing             bool             // Whether we're in edit mode
	IsDeload              bool             // Whether this session is a deload week.
	CurrentSetTarget      domain.SetTarget // Recommended weight and reps from progression
	CurrentSetTimedTarget int              // Recommended seconds for time_based exercises; 0 for others.
	AbsCurrentWeight      float64          // |CurrentSetTarget.WeightKg|, for assisted form input
	RestEndAtMs           int64            // 0 when no rest chip should be shown.
	CurrentSetNumber      int              // 1-based number of the first incomplete set, clamped to TotalSetCount when all done.
	TotalSetCount         int              // len(ExerciseSlot.Sets), for the "Set N of M" overline.
}

func prepareSetsDisplay(exercise domain.Exercise, sets []domain.Set) []setDisplay {
	unit := exercise.SetValueUnit()
	displays := make([]setDisplay, len(sets))
	for i, set := range sets {
		completedStr := ""
		if set.CompletedValue != nil {
			completedStr = exercise.FormatSetValue(*set.CompletedValue)
		}
		signalLabel, signalGlyph := "", ""
		if set.Signal != nil {
			signalLabel = set.Signal.Label()
			signalGlyph = set.Signal.Glyph()
		}
		displays[i] = setDisplay{
			Set:          set,
			TargetStr:    exercise.FormatSetValue(set.TargetValue),
			CompletedStr: completedStr,
			Unit:         unit,
			Number:       i + 1,
			IsActive:     false, // Populated by the caller after firstIncompleteIndex is known.
			SignalLabel:  signalLabel,
			SignalGlyph:  signalGlyph,
		}
	}
	return displays
}

func getFirstIncompleteIndex(sets []domain.Set) int {
	for i, set := range sets {
		if set.CompletedValue == nil {
			return i
		}
	}
	return len(sets)
}

// buildProgressionTargets returns the current set target and timed target seconds for the
// given exercise on the given date. Both are zero-valued when not applicable.
func (app *application) buildProgressionTargets(
	r *http.Request,
	date time.Time,
	exercise domain.Exercise,
) (domain.SetTarget, int, error) {
	var currentSetTarget domain.SetTarget
	var currentSetTimedTarget int
	switch exercise.ExerciseType {
	case domain.ExerciseTypeWeighted, domain.ExerciseTypeAssisted:
		progression, err := app.service.BuildProgression(r.Context(), date, exercise.ID)
		if err != nil {
			return domain.SetTarget{}, 0, fmt.Errorf("build progression: %w", err)
		}
		currentSetTarget = progression.CurrentSet()
	case domain.ExerciseTypeTime:
		progression, err := app.service.BuildTimedProgression(r.Context(), date, exercise.ID)
		if err != nil {
			return domain.SetTarget{}, 0, fmt.Errorf("build timed progression: %w", err)
		}
		currentSetTimedTarget = progression.CurrentSet().TargetSeconds
	case domain.ExerciseTypeBodyweight:
		// No progression engine for bodyweight — uses the stored target as-is.
	}
	return currentSetTarget, currentSetTimedTarget, nil
}

// computeSetActive reports whether a set row should render its completion form.
// A row is active when the warmup is done and it is either the first incomplete
// set or the set explicitly being edited. This is a multi-field derived value, so
// it lives in the handler rather than the template.
func computeSetActive(
	warmupComplete, completed bool,
	index, firstIncompleteIndex, editingIndex int,
	isEditing bool,
) bool {
	if !warmupComplete {
		return false
	}
	isCurrentTarget := !completed && firstIncompleteIndex == index
	isEditingThis := isEditing && editingIndex == index
	return isCurrentTarget || isEditingThis
}

func (app *application) exerciseSetGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	pos, ok := app.parsePositionParam(w, r)
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
	session, err := app.service.GetSession(r.Context(), date)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if pos >= len(session.Slots) {
		app.notFound(w, r)
		return
	}
	exerciseSlot := session.Slots[pos]

	currentSetTarget, currentSetTimedTarget, err := app.buildProgressionTargets(r, date, exerciseSlot.Exercise)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Render the chip whenever a rest is active for this slot — the chip
	// stays put past expiration so a user rotating through other exercises
	// (power sets) and returning later still sees the "Ready" state instead
	// of nothing. The on-screen JS flips elapsed deadlines to "Ready" itself.
	var restEndAtMs int64
	if restEnd, restActive := exerciseSlot.RestEndAt(session.PeriodizationType, session.IsDeload); restActive {
		restEndAtMs = restEnd.UnixMilli()
	}

	data := exerciseSetTemplateData{
		BaseTemplateData:      newBaseTemplateData(r),
		Date:                  date,
		Position:              pos,
		ExerciseSlot:          exerciseSlot,
		SetsDisplay:           prepareSetsDisplay(exerciseSlot.Exercise, exerciseSlot.Sets),
		FirstIncompleteIndex:  getFirstIncompleteIndex(exerciseSlot.Sets),
		EditingIndex:          editingIndex,
		IsEditing:             isEditing,
		IsDeload:              session.IsDeload,
		CurrentSetTarget:      currentSetTarget,
		CurrentSetTimedTarget: currentSetTimedTarget,
		AbsCurrentWeight:      currentSetTarget.AbsWeightKg(),
		RestEndAtMs:           restEndAtMs,
		CurrentSetNumber:      min(getFirstIncompleteIndex(exerciseSlot.Sets)+1, len(exerciseSlot.Sets)),
		TotalSetCount:         len(exerciseSlot.Sets),
	}

	for i := range data.SetsDisplay {
		data.SetsDisplay[i].IsActive = computeSetActive(
			exerciseSlot.WarmupCompletedAt != nil,
			data.SetsDisplay[i].Set.CompletedValue != nil,
			i,
			data.FirstIncompleteIndex,
			data.EditingIndex,
			data.IsEditing,
		)
	}

	app.render(w, r, http.StatusOK, "exerciseset", data)
}

// exerciseSetParams bundles the URL params for exercise set operations.
// Position is the slot's 0-based index in Session.Slots, taken from
// the URL.
type exerciseSetParams struct {
	Date     time.Time
	Position int
	SetIndex int
}

// parseExerciseSetURLParams extracts and validates URL parameters for exercise set operations.
func (app *application) parseExerciseSetURLParams(r *http.Request) (exerciseSetParams, error) {
	date, err := time.Parse("2006-01-02", r.PathValue("date"))
	if err != nil {
		return exerciseSetParams{}, fmt.Errorf("parse date: %w", err)
	}

	pos, err := strconv.Atoi(r.PathValue("position"))
	if err != nil {
		return exerciseSetParams{}, fmt.Errorf("parse position: %w", err)
	}
	if pos < 0 {
		return exerciseSetParams{}, fmt.Errorf("position %d: must be >= 0", pos)
	}

	setIndex, err := strconv.Atoi(r.PathValue("setIndex"))
	if err != nil {
		return exerciseSetParams{}, fmt.Errorf("parse set index: %w", err)
	}

	return exerciseSetParams{
		Date:     date,
		Position: pos,
		SetIndex: setIndex,
	}, nil
}

// recordSetCompletionWithWeight handles parsing and persisting a weighted or assisted set completion from form data.
func (app *application) recordSetCompletionWithWeight(
	w http.ResponseWriter, r *http.Request,
	params exerciseSetParams,
	exercise domain.Exercise,
) bool {
	weightStr := strings.Replace(r.PostForm.Get("weight"), ",", ".", 1)
	weight, err := strconv.ParseFloat(weightStr, 64)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("parse weight: %w", err))
		return false
	}

	weight = exercise.EncodeFormWeight(weight, r.PostForm.Get("assisted") != "")

	var signal *domain.Signal
	if raw := r.PostForm.Get(signalFormField); raw != "" {
		s := domain.Signal(raw)
		signal = &s
	}

	reps, err := strconv.Atoi(r.PostForm.Get("reps"))
	if err != nil {
		app.serverError(w, r, fmt.Errorf("parse reps: %w", err))
		return false
	}

	err = app.service.RecordSet(
		r.Context(), params.Date, params.Position, params.SetIndex, signal, &weight, reps)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("record set completion: %w", err))
		return false
	}

	signalStr := ""
	if signal != nil {
		signalStr = string(*signal)
	}
	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "recorded set completion",
		slog.String("date", params.Date.Format("2006-01-02")),
		slog.Int("position", params.Position),
		slog.Int("set_index", params.SetIndex),
		slog.String(signalFormField, signalStr),
		slog.Float64("weight", weight),
		slog.Int("reps", reps))
	return true
}

// recordBodyweightSetCompletion handles parsing and persisting a bodyweight set
// completion from form data. Time-based sets go through recordTimedSetCompletion.
func (app *application) recordBodyweightSetCompletion(
	w http.ResponseWriter, r *http.Request,
	params exerciseSetParams,
) bool {
	completedValueStr := r.PostForm.Get("completed_value")
	if completedValueStr == "" {
		app.serverError(w, r, errors.New("completed_value not provided"))
		return false
	}
	completedValue, err := strconv.Atoi(completedValueStr)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("parse completed_value: %w", err))
		return false
	}
	if err = app.service.UpdateCompletedValue(
		r.Context(), params.Date, params.Position, params.SetIndex, completedValue); err != nil {
		app.serverError(w, r, fmt.Errorf("update completed value: %w", err))
		return false
	}
	return true
}

// recordTimedSetCompletion handles parsing and persisting a time-based set
// completion: completed_seconds + signal.
func (app *application) recordTimedSetCompletion(
	w http.ResponseWriter, r *http.Request,
	params exerciseSetParams,
) bool {
	completedValueStr := r.PostForm.Get("completed_value")
	if completedValueStr == "" {
		app.serverError(w, r, errors.New("completed_value not provided"))
		return false
	}
	completedSeconds, err := strconv.Atoi(completedValueStr)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("parse completed_value: %w", err))
		return false
	}

	var signal *domain.Signal
	if raw := r.PostForm.Get(signalFormField); raw != "" {
		s := domain.Signal(raw)
		signal = &s
	}

	if err = app.service.RecordSet(
		r.Context(),
		params.Date,
		params.Position,
		params.SetIndex,
		signal,
		nil,
		completedSeconds,
	); err != nil {
		app.serverError(w, r, fmt.Errorf("record timed set completion: %w", err))
		return false
	}

	signalStr := ""
	if signal != nil {
		signalStr = string(*signal)
	}
	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "recorded timed set completion",
		slog.String("date", params.Date.Format("2006-01-02")),
		slog.Int("position", params.Position),
		slog.Int("set_index", params.SetIndex),
		slog.String(signalFormField, signalStr),
		slog.Int("completed_seconds", completedSeconds))
	return true
}

func (app *application) exerciseSetUpdatePOST(w http.ResponseWriter, r *http.Request) {
	params, err := app.parseExerciseSetURLParams(r)
	if err != nil {
		app.notFound(w, r)
		return
	}

	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}

	session, err := app.service.GetSession(r.Context(), params.Date)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if params.Position >= len(session.Slots) {
		app.notFound(w, r)
		return
	}
	exerciseSlot := session.Slots[params.Position]
	exercise := exerciseSlot.Exercise

	switch exercise.ExerciseType {
	case domain.ExerciseTypeWeighted, domain.ExerciseTypeAssisted:
		if !app.recordSetCompletionWithWeight(w, r, params, exercise) {
			return
		}
	case domain.ExerciseTypeTime:
		if !app.recordTimedSetCompletion(w, r, params) {
			return
		}
	case domain.ExerciseTypeBodyweight:
		if !app.recordBodyweightSetCompletion(w, r, params) {
			return
		}
	}

	redirect(w, r, fmt.Sprintf("/workouts/%s/exercises/%d",
		params.Date.Format("2006-01-02"), params.Position))
}

func (app *application) exerciseSetWarmupCompletePOST(w http.ResponseWriter, r *http.Request) {
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	pos, ok := app.parsePositionParam(w, r)
	if !ok {
		return
	}

	if err := app.service.MarkWarmupComplete(r.Context(), date, pos); err != nil {
		app.serverError(w, r, fmt.Errorf("mark warmup complete: %w", err))
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "warmup completed",
		slog.String("date", date.Format("2006-01-02")),
		slog.Int("position", pos))

	redirect(w, r, fmt.Sprintf("/workouts/%s/exercises/%d", date.Format("2006-01-02"), pos))
}
