package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/workout"
)

// ExerciseProgressDataPoint represents a single data point for the exercise chart.
type ExerciseProgressDataPoint struct {
	MaxWeight float64 `json:"max_weight"`
	Date      string  `json:"date"`
	Sets      string  `json:"sets"`
}

// processSessionData extracts exercise data from a session and calculates metrics.
func processSessionData(session workout.Session, exerciseID int) (ExerciseProgressDataPoint, bool) {
	dateStr := session.Date.Format("2006-01-02")

	// Find the exercise in this session
	var exerciseData *workout.ExerciseSet
	for _, es := range session.ExerciseSets {
		if es.Exercise.ID == exerciseID {
			exerciseData = &es
			break
		}
	}

	if exerciseData == nil {
		return ExerciseProgressDataPoint{
			MaxWeight: 0,
			Date:      "",
			Sets:      "",
		}, false
	}

	// Calculate metrics and build tooltip for this session
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
	if sessionMaxWeight > 0 && len(setDescriptions) > 0 {
		return ExerciseProgressDataPoint{
			MaxWeight: sessionMaxWeight,
			Date:      dateStr,
			Sets:      strings.Join(setDescriptions, "<br>"),
		}, true
	}

	return ExerciseProgressDataPoint{
		MaxWeight: 0,
		Date:      "",
		Sets:      "",
	}, false
}

// ExerciseProgressDataset represents the dataset for the exercise progress chart.
type ExerciseProgressDataset struct {
	ExerciseName string                      `json:"exercise_name"`
	DataPoints   []ExerciseProgressDataPoint `json:"data_points"`
}

// generateExerciseProgressData creates chart dataset for exercise progress tracking.
func (app *application) generateExerciseProgressData(
	ctx context.Context, currentDate time.Time, exercise workout.Exercise) (ExerciseProgressDataset, error) {
	// Get historical data for the past 5 years
	fiveYearsAgo := currentDate.AddDate(-5, 0, 0)
	sessions, err := app.workoutService.GetSessionsWithExerciseSince(ctx, exercise.ID, fiveYearsAgo)
	if err != nil {
		return ExerciseProgressDataset{}, fmt.Errorf("failed to get sessions: %w", err)
	}

	// Process data for chart
	var dataPoints []ExerciseProgressDataPoint

	for _, session := range sessions {
		if dataPoint, hasData := processSessionData(session, exercise.ID); hasData {
			dataPoints = append(dataPoints, dataPoint)
		}
	}

	return ExerciseProgressDataset{
		ExerciseName: exercise.Name,
		DataPoints:   dataPoints,
	}, nil
}

// exerciseProgressChart handles requests for exercise progress chart data.
func (app *application) exerciseProgressChart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse date and exercise ID from URL
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.Error(w, "Invalid date format", http.StatusBadRequest)
		return
	}

	exerciseIDStr := r.PathValue("exerciseID")
	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		http.Error(w, "Invalid exercise ID", http.StatusBadRequest)
		return
	}

	// Get exercise information
	exercise, err := app.workoutService.GetExercise(r.Context(), exerciseID)
	if err != nil {
		http.Error(w, "Exercise not found", http.StatusNotFound)
		return
	}

	// Generate dataset for the exercise
	dataset, err := app.generateExerciseProgressData(r.Context(), date, exercise)
	if err != nil {
		http.Error(w, "Failed to generate chart data", http.StatusInternalServerError)
		return
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(dataset)
	if err != nil {
		http.Error(w, "Failed to generate JSON", http.StatusInternalServerError)
		return
	}

	// Write the JSON response
	_, err = w.Write(jsonData)
	if err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		return
	}
}
