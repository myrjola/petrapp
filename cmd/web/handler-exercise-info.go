package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/workout"
	"github.com/yuin/goldmark"
)

// exerciseInfoTemplateData contains data for the exercise info template.
type exerciseInfoTemplateData struct {
	BaseTemplateData
	Date           time.Time
	Exercise       workout.Exercise
	IsAdmin        bool
	ProgressPoints []ExerciseProgressDataPoint
}

// exerciseInfoGET handles GET requests to view exercise information.
func (app *application) exerciseInfoGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		app.notFound(w, r)
		return
	}

	// Parse exercise ID from URL path
	exerciseIDStr := r.PathValue("exerciseID")
	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		app.notFound(w, r)
		return
	}

	// Get the exercise.
	exercise, err := app.workoutService.GetExercise(r.Context(), exerciseID)
	if err != nil {
		if errors.Is(err, workout.ErrNotFound) {
			app.notFound(w, r)
			return
		}
		app.serverError(w, r, err)
		return
	}

	// Fetch the progress data.
	progressData, err := app.generateExerciseProgressData(r.Context(), date, exercise)
	if err != nil {
		http.Error(w, "Failed to generate chart data", http.StatusInternalServerError)
		return
	}

	// Check if the user is admin.
	isAdmin := contexthelpers.IsAdmin(r.Context())

	data := exerciseInfoTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Date:             date,
		Exercise:         exercise,
		IsAdmin:          isAdmin,
		ProgressPoints:   progressData,
	}

	app.render(w, r, http.StatusOK, "exercise-info", data)
}

// renderMarkdownToHTML converts markdown string to HTML.
func (app *application) renderMarkdownToHTML(ctx context.Context, markdown string) template.HTML {
	md := goldmark.New()

	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		app.logger.LogAttrs(ctx, slog.LevelError, "failed to render markdown",
			slog.Any("error", err))
		return "<p>Error rendering markdown content.</p>"
	}

	// Returning as template.HTML tells Go this is safe HTML that doesn't need escaping
	return template.HTML(buf.String()) //nolint:gosec // we trust the markdown renderer
}

// ExerciseProgressDataPoint represents a single data point for the exercise chart.
type ExerciseProgressDataPoint struct {
	// Progress is a numerical value for the y-axis on the line chart. In most cases, it represents the max weight.
	Progress float64
	// Date of the exercise session.
	Date time.Time
	// SetDescriptions is a list of sets formatted as "8x10kg".
	SetDescriptions []string
}

// processSessionData extracts exercise data from a session and calculates metrics.
func processSessionData(session workout.Session, exerciseID int) (ExerciseProgressDataPoint, bool) {
	// Find the exercise in this session
	var exerciseData *workout.ExerciseSet
	for _, es := range session.ExerciseSets {
		if es.Exercise.ID == exerciseID {
			exerciseData = &es
			break
		}
	}
	if exerciseData == nil {
		return ExerciseProgressDataPoint{}, false
	}

	var sessionMaxWeight float64
	var setDescriptions []string

	for _, set := range exerciseData.Sets {
		if set.WeightKg != nil && set.CompletedReps != nil {
			weight := *set.WeightKg
			reps := *set.CompletedReps

			if weight > sessionMaxWeight {
				sessionMaxWeight = weight
			}

			// Format as "8x10kg"
			setDescriptions = append(setDescriptions, fmt.Sprintf("%dx%.1fkg", reps, weight))
		}
	}

	// Only include sessions with actual data
	if len(setDescriptions) == 0 {
		return ExerciseProgressDataPoint{
			Progress:        0,
			Date:            time.Time{},
			SetDescriptions: nil,
		}, false
	}

	return ExerciseProgressDataPoint{
		Progress:        sessionMaxWeight, // TODO: Handle other types of progress than max weight.
		Date:            session.Date,
		SetDescriptions: setDescriptions,
	}, true
}

// generateExerciseProgressData creates a chart dataset for exercise progress tracking.
func (app *application) generateExerciseProgressData(
	ctx context.Context, currentDate time.Time, exercise workout.Exercise) ([]ExerciseProgressDataPoint, error) {
	// Get historical data for the past 5 years
	fiveYearsAgo := currentDate.AddDate(-5, 0, 0)
	sessions, err := app.workoutService.GetSessionsWithExerciseSince(ctx, exercise.ID, fiveYearsAgo)
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}

	// Process data for the chart.
	var dataPoints []ExerciseProgressDataPoint

	for _, session := range sessions {
		if dataPoint, hasData := processSessionData(session, exercise.ID); hasData {
			dataPoints = append(dataPoints, dataPoint)
		}
	}

	return dataPoints, nil
}
