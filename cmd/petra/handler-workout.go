package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

type workoutTemplateData struct {
	BaseTemplateData

	Date            time.Time
	WorkoutTypeName string
	StatusLabel     string
	StatusVariant   string
	FinishNote      string
	Exercises       []workoutExerciseView
	CompletedCount  int
	TotalCount      int
	ProgressPercent int
	ProgressState   string
	Flash           BannerData
}

// workoutExerciseView is the per-exercise row rendered on the workout overview.
// Pre-shaped in the handler so the template can range without arithmetic.
type workoutExerciseView struct {
	Position          int // 0-based slot index in Session.Slots — used in /exercises/{position} URLs and the view-transition name.
	Index             int // 1-based position label ("01", "02", …)
	Name              string
	ExerciseType      domain.ExerciseType // exposed as data-exercise-type for Playwright flows that need a specific form shape.
	State             domain.ExerciseSlotState
	SetCount          int
	CompletedSetCount int
	TargetText        string
	SubLine           string
	Dots              []workoutExerciseDot
	RestEndAtMs       int64 // 0 when no rest chip should be shown for this row.
}

// workoutExerciseDot represents one set's done/not-done state for the dot indicator.
type workoutExerciseDot struct {
	Done bool
}

type workoutCompletionTemplateData struct {
	BaseTemplateData

	Date         time.Time
	Difficulties []difficultyOption
}

type workoutNotFoundTemplateData struct {
	BaseTemplateData

	Date   time.Time
	Header PageHeaderData
	Flash  BannerData
}

// newWorkoutNotFoundTemplateData builds the data for the workout-not-found page.
func newWorkoutNotFoundTemplateData(
	r *http.Request, date time.Time, flashMessage string,
) workoutNotFoundTemplateData {
	base := newBaseTemplateData(r)
	return workoutNotFoundTemplateData{
		BaseTemplateData: base,
		Date:             date,
		Header: PageHeaderData{
			Title:    "Not in This Week's Plan",
			Subtitle: "",
			Nonce:    base.Nonce,
		},
		Flash: BannerData{
			Variant: BannerVariantError,
			Message: flashMessage,
			Live:    true,
			Nonce:   base.Nonce,
		},
	}
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
	date, ok := app.parseDateParam(w, r)
	if !ok {
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
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	// First mark the workout as completed
	if err := app.service.CompleteSession(r.Context(), date); err != nil {
		app.userError(w, r, err, fmt.Sprintf("/workouts/%s", date.Format("2006-01-02")))
		return
	}

	// Redirect to the completion form
	redirect(w, r, fmt.Sprintf("/workouts/%s/complete", date.Format("2006-01-02")))
}

func (app *application) workoutStartPOST(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	// Start the workout session
	if err := app.service.StartSession(r.Context(), date); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			flash := app.popFlash(r.Context())
			data := newWorkoutNotFoundTemplateData(r, date, flash.Message)
			app.render(w, r, http.StatusNotFound, "workout-not-found", data)
			return
		}
		app.serverError(w, r, err)
		return
	}

	// Redirect to the workout page
	redirect(w, r, fmt.Sprintf("/workouts/%s", date.Format("2006-01-02")))
}

func (app *application) workoutGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	// Fetch a workout session for the date
	session, err := app.service.GetSession(r.Context(), date)
	if err != nil {
		// Check if the workout doesn't exist
		if errors.Is(err, domain.ErrNotFound) {
			flash := app.popFlash(r.Context())
			data := newWorkoutNotFoundTemplateData(r, date, flash.Message)
			app.render(w, r, http.StatusNotFound, "workout-not-found", data)
			return
		}
		app.serverError(w, r, err)
		return
	}

	flash := app.popFlash(r.Context())
	data := newWorkoutTemplateData(r, date, session, flash.Message)

	app.render(w, r, http.StatusOK, "workout", data)
}

// percentBase is the multiplier for converting a 0..1 ratio into a 0..100 percent.
const percentBase = 100

// newWorkoutTemplateData shapes a fetched session into the per-row view structs
// the workout-overview template renders. All derivations (progress, status,
// finish-note copy, sub-line) happen here so the template stays declarative.
func newWorkoutTemplateData(
	r *http.Request,
	date time.Time,
	session domain.Session,
	flashMessage string,
) workoutTemplateData {
	var statusLabel, statusVariant string
	switch session.Status() {
	case domain.SessionCompleted:
		statusLabel, statusVariant = "Completed", "success"
	case domain.SessionInProgress:
		statusLabel, statusVariant = "In progress", "warning"
	case domain.SessionNotStarted:
		statusLabel, statusVariant = "Ready", "neutral"
	}

	completed := session.CompletedExerciseCount()
	total := len(session.Slots)
	progressPercent := 0
	progressState := ""
	if total > 0 {
		progressPercent = completed * percentBase / total
		switch {
		case completed == total:
			progressState = "completed"
		case completed > 0:
			progressState = "in-progress"
		}
	}

	exerciseViews := make([]workoutExerciseView, 0, total)
	for i, es := range session.Slots {
		exerciseViews = append(
			exerciseViews,
			newWorkoutExerciseView(i, es, session.Goal, session.IsDeload),
		)
	}

	base := newBaseTemplateData(r)
	return workoutTemplateData{
		BaseTemplateData: base,
		Date:             date,
		WorkoutTypeName:  session.WorkoutType().Label(),
		StatusLabel:      statusLabel,
		StatusVariant:    statusVariant,
		FinishNote:       finishNoteFor(session.IncompleteExerciseCount()),
		Exercises:        exerciseViews,
		CompletedCount:   completed,
		TotalCount:       total,
		ProgressPercent:  progressPercent,
		ProgressState:    progressState,
		Flash: BannerData{
			Variant: BannerVariantError,
			Message: flashMessage,
			Live:    true,
			Nonce:   base.Nonce,
		},
	}
}

// finishNoteFor returns the copy shown under the Finish-workout button.
// It encourages early finish when slots remain and celebrates a clean sweep.
func finishNoteFor(incomplete int) string {
	switch incomplete {
	case 0:
		return "You rock!"
	case 1:
		return "1 exercise remains · you can finish anyway"
	default:
		return fmt.Sprintf("%d exercises remain · you can finish anyway", incomplete)
	}
}

// newWorkoutExerciseView shapes one ExerciseSlot into a workoutExerciseView,
// including the sub-line copy and the per-set dot indicator. pos is the
// 0-based slot index in Session.Slots.
func newWorkoutExerciseView(
	pos int, es domain.ExerciseSlot, pt domain.SessionGoal, isDeload bool,
) workoutExerciseView {
	dots := make([]workoutExerciseDot, len(es.Sets))
	for j, s := range es.Sets {
		dots[j] = workoutExerciseDot{Done: s.CompletedAt != nil}
	}
	completedSets := es.CompletedSetCount()
	var subLine string
	switch {
	case len(es.Sets) == 0:
		subLine = "no sets planned"
	case completedSets == 0:
		subLine = fmt.Sprintf("%d sets", len(es.Sets))
		if target := es.Exercise.TargetRangeText(); target != "" {
			subLine = subLine + " · " + target
		}
	default:
		subLine = fmt.Sprintf("%d / %d sets done", completedSets, len(es.Sets))
	}
	var restEndAtMs int64
	if restEnd, ok := es.RestEndAt(pt, isDeload); ok {
		restEndAtMs = restEnd.UnixMilli()
	}
	return workoutExerciseView{
		Position:          pos,
		Index:             pos + 1,
		Name:              es.Exercise.Name,
		ExerciseType:      es.Exercise.ExerciseType,
		State:             es.CompletionState(),
		SetCount:          len(es.Sets),
		CompletedSetCount: completedSets,
		TargetText:        es.Exercise.TargetRangeText(),
		SubLine:           subLine,
		Dots:              dots,
		RestEndAtMs:       restEndAtMs,
	}
}

func (app *application) workoutFeedbackPOST(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	// Parse difficulty from URL path.
	difficultyStr := r.PathValue("difficulty")
	difficulty, err := strconv.Atoi(difficultyStr)
	if err != nil {
		app.notFound(w, r)
		return
	}

	// Save the feedback. An out-of-range rating is a malformed URL, not a
	// server fault, so surface it as a 404 like the parse failure above.
	if err = app.service.SaveFeedback(r.Context(), date, difficulty); err != nil {
		if errors.Is(err, domain.ErrInvalidDifficultyRating) {
			app.notFound(w, r)
			return
		}
		app.serverError(w, r, err)
		return
	}

	// Redirect back to the home page
	redirect(w, r, "/")
}

// workoutSwapExerciseGET handles GET requests to show available exercises for swapping.
func (app *application) workoutSwapExerciseGET(w http.ResponseWriter, r *http.Request) {
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	pos, ok := app.parsePositionParam(w, r)
	if !ok {
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	current, candidates, err := app.service.ListSwapCandidates(r.Context(), date, pos, query)
	if err != nil {
		if errors.Is(err, domain.ErrSlotNotFound) {
			app.notFound(w, r)
			return
		}
		app.serverError(w, r, err)
		return
	}

	base := newBaseTemplateData(r)
	dateStr := date.Format("2006-01-02")
	cards := make([]ExerciseResultCardData, 0, len(candidates))
	for _, ex := range candidates {
		cards = append(cards, ExerciseResultCardData{
			Exercise:    ex,
			FormAction:  fmt.Sprintf("/workouts/%s/exercises/%d/swap", dateStr, pos),
			FieldName:   "new_exercise_id",
			ButtonLabel: "Swap to this exercise",
			Nonce:       base.Nonce,
		})
	}

	data := exerciseSwapTemplateData{
		BaseTemplateData: base,
		Date:             date,
		Header:           PageHeaderData{Title: "Swap Exercise", Subtitle: "", Nonce: base.Nonce},
		Position:         pos,
		CurrentExercise:  current,
		Cards:            cards,
		Query:            query,
	}

	app.render(w, r, http.StatusOK, "exercise-swap", data)
}

// workoutSwapExercisePOST handles POST requests to swap an exercise.
func (app *application) workoutSwapExercisePOST(w http.ResponseWriter, r *http.Request) {
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	pos, ok := app.parsePositionParam(w, r)
	if !ok {
		return
	}

	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}

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

	if err = app.service.SwapExercise(r.Context(), date, pos, newExerciseID); err != nil {
		app.serverError(w, r, err)
		return
	}

	redirect(w, r, fmt.Sprintf("/workouts/%s/exercises/%d", date.Format("2006-01-02"), pos))
}

// exerciseSwapTemplateData contains data for the exercise swap template.
type exerciseSwapTemplateData struct {
	BaseTemplateData

	Date            time.Time
	Header          PageHeaderData
	Position        int
	CurrentExercise domain.Exercise
	Cards           []ExerciseResultCardData
	Query           string
}

// exerciseAddTemplateData contains data for the exercise add template.
type exerciseAddTemplateData struct {
	BaseTemplateData

	Date   time.Time
	Header PageHeaderData
	Cards  []ExerciseResultCardData
	Query  string
}

// workoutAddExerciseGET handles GET requests to show available exercises for adding.
func (app *application) workoutAddExerciseGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// Get the current workout session to see which exercises are already included
	session, err := app.service.GetSession(r.Context(), date)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Create a map of exercise IDs that are already in the workout
	existingExerciseIDs := make(map[int]bool)
	for _, exerciseSlot := range session.Slots {
		existingExerciseIDs[exerciseSlot.Exercise.ID] = true
	}

	// Get all exercises
	allExercises, err := app.service.ListExercises(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	queryLower := strings.ToLower(query)
	var availableExercises []domain.Exercise
	for _, exercise := range allExercises {
		if existingExerciseIDs[exercise.ID] {
			continue
		}
		if queryLower != "" && !strings.Contains(strings.ToLower(exercise.Name), queryLower) {
			continue
		}
		availableExercises = append(availableExercises, exercise)
	}

	base := newBaseTemplateData(r)
	dateStr := date.Format("2006-01-02")
	cards := make([]ExerciseResultCardData, 0, len(availableExercises))
	for _, ex := range availableExercises {
		cards = append(cards, ExerciseResultCardData{
			Exercise:    ex,
			FormAction:  fmt.Sprintf("/workouts/%s/add-exercise", dateStr),
			FieldName:   "exercise_id",
			ButtonLabel: "Add this exercise",
			Nonce:       base.Nonce,
		})
	}

	data := exerciseAddTemplateData{
		BaseTemplateData: base,
		Date:             date,
		Header: PageHeaderData{
			Title:    "Add Exercise",
			Subtitle: "",
			Nonce:    base.Nonce,
		},
		Cards: cards,
		Query: query,
	}

	app.render(w, r, http.StatusOK, "exercise-add", data)
}

// workoutAddExercisePOST handles POST requests to add an exercise to a workout.
func (app *application) workoutAddExercisePOST(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	// Parse form
	if !app.parseForm(w, r, defaultMaxFormSize) {
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

	// Add exercise to the workout and capture the new slot's position so we
	// can land the user straight on the new exercise's detail page.
	newPos, err := app.service.AddExercise(r.Context(), date, exerciseID)
	if err != nil {
		workoutURL := fmt.Sprintf("/workouts/%s", date.Format("2006-01-02"))
		app.userError(w, r, err, workoutURL)
		return
	}

	// Replace /add-exercise with the new exercise's detail page so back
	// goes to the workout overview rather than the picker.
	redirectReplace(w, r, fmt.Sprintf("/workouts/%s/exercises/%d",
		date.Format("2006-01-02"), newPos))
}
