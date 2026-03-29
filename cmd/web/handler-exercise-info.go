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

// renderMarkdownToHTML converts Markdown string to HTML.
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

// processEntryData extracts chart metrics from a single exercise progress entry.
// For weighted exercises Progress is the max weight lifted; for bodyweight it is the max reps completed.
func processEntryData(entry workout.ExerciseProgressEntry, typ workout.ExerciseType) ExerciseProgressDataPoint {
	var progress float64
	var setDescriptions []string

	for _, set := range entry.Sets {
		reps := *set.CompletedReps // service guarantees CompletedReps != nil

		switch typ {
		case workout.ExerciseTypeWeighted:
			if set.WeightKg != nil {
				weight := *set.WeightKg
				if weight > progress {
					progress = weight
				}
				setDescriptions = append(setDescriptions, fmt.Sprintf("%dx%.1fkg", reps, weight))
			}
		case workout.ExerciseTypeAssisted:
			if set.WeightKg != nil {
				weight := *set.WeightKg
				// For assisted exercises, less negative (closer to 0) is more progress.
				if weight > progress {
					progress = weight
				}
				setDescriptions = append(setDescriptions, fmt.Sprintf("%dx%.1fkg", reps, weight))
			}
		case workout.ExerciseTypeBodyweight:
			if float64(reps) > progress {
				progress = float64(reps)
			}
			setDescriptions = append(setDescriptions, fmt.Sprintf("%d reps", reps))
		}
	}

	return ExerciseProgressDataPoint{
		Progress:        progress,
		Date:            entry.Date,
		SetDescriptions: setDescriptions,
	}
}

// generateExerciseProgressData creates a chart dataset for exercise progress tracking.
func (app *application) generateExerciseProgressData(
	ctx context.Context, currentDate time.Time, exercise workout.Exercise) ([]ExerciseProgressDataPoint, error) {
	// Get historical data for the past 5 years.
	fiveYearsAgo := currentDate.AddDate(-5, 0, 0)
	progress, err := app.workoutService.GetExerciseSetsForExerciseSince(ctx, exercise.ID, fiveYearsAgo)
	if err != nil {
		return nil, fmt.Errorf("failed to get exercise sets: %w", err)
	}

	dataPoints := make([]ExerciseProgressDataPoint, len(progress.Entries))
	for i, entry := range progress.Entries {
		dataPoints[i] = processEntryData(entry, progress.Exercise.ExerciseType)
	}

	return dataPoints, nil
}
