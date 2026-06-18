package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/platform/contexthelpers"
)

// exerciseInfoTemplateData contains data for the exercise info template.
type exerciseInfoTemplateData struct {
	BaseTemplateData

	Date           time.Time
	Header         PageHeaderData
	Position       int
	Exercise       domain.Exercise
	IsAdmin        bool
	ProgressPoints []ExerciseProgressDataPoint
}

// exerciseInfoGET handles GET requests to view exercise information.
func (app *application) exerciseInfoGET(w http.ResponseWriter, r *http.Request) {
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		app.notFound(w, r)
		return
	}

	pos, err := strconv.Atoi(r.PathValue("position"))
	if err != nil || pos < 0 {
		app.notFound(w, r)
		return
	}

	// Resolve the slot to its current exercise via the workout session.
	session, err := app.service.GetSession(r.Context(), date)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			app.notFound(w, r)
			return
		}
		app.serverError(w, r, err)
		return
	}
	if pos >= len(session.Slots) {
		app.notFound(w, r)
		return
	}
	exercise := session.Slots[pos].Exercise

	// Fetch the progress data.
	progressData, err := app.generateExerciseProgressData(r.Context(), date, exercise)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("generate exercise progress data: %w", err))
		return
	}

	// Check if the user is admin.
	isAdmin := contexthelpers.IsAdmin(r.Context())

	base := newBaseTemplateData(r)
	data := exerciseInfoTemplateData{
		BaseTemplateData: base,
		Date:             date,
		Header: PageHeaderData{
			Title:    exercise.Name,
			Subtitle: "",
			Nonce:    base.Nonce,
		},
		Position:       pos,
		Exercise:       exercise,
		IsAdmin:        isAdmin,
		ProgressPoints: progressData,
	}

	app.render(w, r, http.StatusOK, "exercise-info", data)
}

// ExerciseProgressDataPoint represents a single data point for the exercise chart.
type ExerciseProgressDataPoint struct {
	// Date of the exercise session.
	Date time.Time
	// SetDescriptions is a list of sets formatted as "8x10kg".
	SetDescriptions []string
}

// generateExerciseProgressData creates a chart dataset for exercise progress tracking.
func (app *application) generateExerciseProgressData(
	ctx context.Context, currentDate time.Time, exercise domain.Exercise) ([]ExerciseProgressDataPoint, error) {
	// Get historical data for the past 5 years.
	fiveYearsAgo := currentDate.AddDate(-5, 0, 0)
	progress, err := app.service.GetExerciseSetsForExerciseSince(ctx, exercise.ID, fiveYearsAgo)
	if err != nil {
		return nil, fmt.Errorf("failed to get exercise sets: %w", err)
	}

	dataPoints := make([]ExerciseProgressDataPoint, len(progress.Entries))
	for i, entry := range progress.Entries {
		var setDescriptions []string
		for _, set := range entry.Sets {
			if desc := progress.Exercise.FormatSetDescription(set); desc != "" {
				setDescriptions = append(setDescriptions, desc)
			}
		}
		dataPoints[i] = ExerciseProgressDataPoint{
			Date:            entry.Date,
			SetDescriptions: setDescriptions,
		}
	}

	return dataPoints, nil
}
