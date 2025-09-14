package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/tools"
)

// StatisticsParams represents the parameters for the calculate_statistics function.
type StatisticsParams struct {
	MetricType   string     `json:"metric_type"`
	ExerciseName string     `json:"exercise_name,omitempty"`
	DateRange    *DateRange `json:"date_range,omitempty"`
}

// DateRange represents a date range for statistical analysis.
type DateRange struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// StatisticsResult represents the result of calculating statistics.
type StatisticsResult struct {
	MetricType   string      `json:"metric_type"`
	ExerciseName string      `json:"exercise_name,omitempty"`
	Value        interface{} `json:"value"`
	Description  string      `json:"description"`
	UserID       int         `json:"user_id,omitempty"`
}

// StatisticsTool provides statistical analysis functionality for workout data.
type StatisticsTool struct {
	db              *sqlite.Database
	logger          *slog.Logger
	secureQueryTool *tools.SecureQueryTool
}

// NewStatisticsTool creates a new StatisticsTool instance.
func NewStatisticsTool(db *sqlite.Database, logger *slog.Logger) *StatisticsTool {
	return &StatisticsTool{
		db:              db,
		logger:          logger,
		secureQueryTool: tools.NewSecureQueryTool(db.ReadOnly),
	}
}

// CalculateStatistics calculates statistical metrics from workout data.
// This function ensures that:
// 1. Only authenticated users can access their own data
// 2. All database queries are securely executed with user isolation
// 3. Statistical calculations are accurate and meaningful
func (t *StatisticsTool) CalculateStatistics(ctx context.Context, params StatisticsParams) (*StatisticsResult, error) {
	// Get user ID from context
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return nil, errors.New("user not authenticated")
	}

	// Validate metric type
	if err := t.validateMetricType(params.MetricType); err != nil {
		return nil, fmt.Errorf("metric validation failed: %w", err)
	}

	// Calculate the specific metric
	var result *StatisticsResult
	var err error

	switch params.MetricType {
	case "personal_record":
		result, err = t.calculatePersonalRecord(ctx, userID, params.ExerciseName, params.DateRange)
	case "total_volume":
		result, err = t.calculateTotalVolume(ctx, userID, params.ExerciseName, params.DateRange)
	case "workout_frequency":
		result, err = t.calculateWorkoutFrequency(ctx, userID, params.DateRange)
	case "average_intensity":
		result, err = t.calculateAverageIntensity(ctx, userID, params.DateRange)
	case "muscle_group_distribution":
		result, err = t.calculateMuscleGroupDistribution(ctx, userID, params.DateRange)
	case "progression_rate":
		result, err = t.calculateProgressionRate(ctx, userID, params.ExerciseName, params.DateRange)
	default:
		return nil, fmt.Errorf("unsupported metric type: %s", params.MetricType)
	}

	if err != nil {
		t.logger.ErrorContext(ctx, "Statistic calculation failed",
			"user_id", userID,
			"metric_type", params.MetricType,
			"exercise_name", params.ExerciseName,
			"error", err)
		return nil, fmt.Errorf("statistic calculation failed: %w", err)
	}

	t.logger.InfoContext(ctx, "Statistic calculated successfully",
		"user_id", userID,
		"metric_type", params.MetricType,
		"exercise_name", params.ExerciseName)

	return result, nil
}

// validateMetricType ensures the metric type is supported.
func (t *StatisticsTool) validateMetricType(metricType string) error {
	validMetrics := []string{
		"personal_record",
		"total_volume",
		"workout_frequency",
		"average_intensity",
		"muscle_group_distribution",
		"progression_rate",
	}

	for _, valid := range validMetrics {
		if metricType == valid {
			return nil
		}
	}

	return fmt.Errorf("invalid metric type: %s", metricType)
}

// calculatePersonalRecord finds the maximum weight lifted for an exercise.
func (t *StatisticsTool) calculatePersonalRecord(ctx context.Context, userID int, exerciseName string, dateRange *DateRange) (*StatisticsResult, error) {
	var query string
	var description string

	if exerciseName != "" {
		// Personal record for specific exercise
		query = fmt.Sprintf(`
			SELECT e.name, MAX(es.weight_kg) as max_weight
			FROM exercise_sets es
			JOIN exercises e ON es.exercise_id = e.id
			WHERE es.workout_user_id = %d
			AND e.name = '%s'
			AND es.weight_kg IS NOT NULL
			AND es.completed_reps IS NOT NULL`, userID, exerciseName)

		if dateRange != nil && dateRange.StartDate != "" && dateRange.EndDate != "" {
			query += fmt.Sprintf(" AND es.workout_date BETWEEN '%s' AND '%s'", dateRange.StartDate, dateRange.EndDate)
		}

		query += " GROUP BY e.name"
		description = fmt.Sprintf("Personal record for %s", exerciseName)
	} else {
		// All personal records
		query = fmt.Sprintf(`
			SELECT e.name, MAX(es.weight_kg) as max_weight
			FROM exercise_sets es
			JOIN exercises e ON es.exercise_id = e.id
			WHERE es.workout_user_id = %d
			AND es.weight_kg IS NOT NULL
			AND es.completed_reps IS NOT NULL`, userID)

		if dateRange != nil && dateRange.StartDate != "" && dateRange.EndDate != "" {
			query += fmt.Sprintf(" AND es.workout_date BETWEEN '%s' AND '%s'", dateRange.StartDate, dateRange.EndDate)
		}

		query += " GROUP BY e.name ORDER BY max_weight DESC"
		description = "All personal records"
	}

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query personal records: %w", err)
	}

	var value interface{}
	if exerciseName != "" && len(result.Rows) > 0 {
		// Single exercise - return just the weight
		if len(result.Rows[0]) >= 2 {
			value = result.Rows[0][1]
		} else {
			value = 0.0
		}
	} else {
		// Multiple exercises - return all records
		records := make(map[string]interface{})
		for _, row := range result.Rows {
			if len(row) >= 2 {
				records[row[0].(string)] = row[1]
			}
		}
		value = records
	}

	return &StatisticsResult{
		MetricType:   "personal_record",
		ExerciseName: exerciseName,
		Value:        value,
		Description:  description,
		UserID:       userID,
	}, nil
}

// calculateTotalVolume calculates total volume (weight × reps × sets).
func (t *StatisticsTool) calculateTotalVolume(ctx context.Context, userID int, exerciseName string, dateRange *DateRange) (*StatisticsResult, error) {
	var query string
	var description string

	if exerciseName != "" {
		query = fmt.Sprintf(`
			SELECT COALESCE(SUM(es.weight_kg * es.completed_reps), 0) as total_volume
			FROM exercise_sets es
			JOIN exercises e ON es.exercise_id = e.id
			WHERE es.workout_user_id = %d
			AND e.name = '%s'
			AND es.weight_kg IS NOT NULL
			AND es.completed_reps IS NOT NULL`, userID, exerciseName)
		description = fmt.Sprintf("Total volume for %s", exerciseName)
	} else {
		query = fmt.Sprintf(`
			SELECT COALESCE(SUM(es.weight_kg * es.completed_reps), 0) as total_volume
			FROM exercise_sets es
			WHERE es.workout_user_id = %d
			AND es.weight_kg IS NOT NULL
			AND es.completed_reps IS NOT NULL`, userID)
		description = "Total volume across all exercises"
	}

	if dateRange != nil && dateRange.StartDate != "" && dateRange.EndDate != "" {
		query += fmt.Sprintf(" AND es.workout_date BETWEEN '%s' AND '%s'", dateRange.StartDate, dateRange.EndDate)
		description += fmt.Sprintf(" from %s to %s", dateRange.StartDate, dateRange.EndDate)
	}

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query total volume: %w", err)
	}

	var volume float64
	if len(result.Rows) > 0 && len(result.Rows[0]) > 0 {
		if v, ok := result.Rows[0][0].(float64); ok {
			volume = v
		}
	}

	return &StatisticsResult{
		MetricType:   "total_volume",
		ExerciseName: exerciseName,
		Value:        volume,
		Description:  description,
		UserID:       userID,
	}, nil
}

// calculateWorkoutFrequency calculates workouts per week.
func (t *StatisticsTool) calculateWorkoutFrequency(ctx context.Context, userID int, dateRange *DateRange) (*StatisticsResult, error) {
	query := fmt.Sprintf(`
		SELECT COUNT(DISTINCT workout_date) as workout_count,
		       MIN(workout_date) as start_date,
		       MAX(workout_date) as end_date
		FROM workout_sessions
		WHERE user_id = %d
		AND completed_at IS NOT NULL`, userID)

	description := "Workout frequency (workouts per week)"

	if dateRange != nil && dateRange.StartDate != "" && dateRange.EndDate != "" {
		query += fmt.Sprintf(" AND workout_date BETWEEN '%s' AND '%s'", dateRange.StartDate, dateRange.EndDate)
		description += fmt.Sprintf(" from %s to %s", dateRange.StartDate, dateRange.EndDate)
	}

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query workout frequency: %w", err)
	}

	var frequency float64
	if len(result.Rows) > 0 && len(result.Rows[0]) >= 3 {
		workoutCount := result.Rows[0][0]
		startDate := result.Rows[0][1]
		endDate := result.Rows[0][2]

		if count, ok := workoutCount.(int64); ok && count > 0 {
			// Calculate the number of weeks between start and end date
			if startStr, ok := startDate.(string); ok {
				if endStr, ok := endDate.(string); ok {
					start, parseErr1 := time.Parse("2006-01-02", startStr)
					end, parseErr2 := time.Parse("2006-01-02", endStr)
					if parseErr1 == nil && parseErr2 == nil {
						days := end.Sub(start).Hours() / 24
						weeks := math.Max(1, days/7) // At least 1 week
						frequency = float64(count) / weeks
					}
				}
			}
		}
	}

	return &StatisticsResult{
		MetricType:  "workout_frequency",
		Value:       frequency,
		Description: description,
		UserID:      userID,
	}, nil
}

// calculateAverageIntensity calculates average difficulty rating of workouts.
func (t *StatisticsTool) calculateAverageIntensity(ctx context.Context, userID int, dateRange *DateRange) (*StatisticsResult, error) {
	query := fmt.Sprintf(`
		SELECT AVG(CAST(difficulty_rating as REAL)) as avg_intensity
		FROM workout_sessions
		WHERE user_id = %d
		AND difficulty_rating IS NOT NULL
		AND completed_at IS NOT NULL`, userID)

	description := "Average workout intensity (difficulty rating)"

	if dateRange != nil && dateRange.StartDate != "" && dateRange.EndDate != "" {
		query += fmt.Sprintf(" AND workout_date BETWEEN '%s' AND '%s'", dateRange.StartDate, dateRange.EndDate)
		description += fmt.Sprintf(" from %s to %s", dateRange.StartDate, dateRange.EndDate)
	}

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query average intensity: %w", err)
	}

	var intensity float64
	if len(result.Rows) > 0 && len(result.Rows[0]) > 0 {
		if v, ok := result.Rows[0][0].(float64); ok {
			intensity = v
		}
	}

	return &StatisticsResult{
		MetricType:  "average_intensity",
		Value:       intensity,
		Description: description,
		UserID:      userID,
	}, nil
}

// calculateMuscleGroupDistribution calculates percentage of exercises per muscle group.
func (t *StatisticsTool) calculateMuscleGroupDistribution(ctx context.Context, userID int, dateRange *DateRange) (*StatisticsResult, error) {
	var subqueryDateFilter, mainQueryDateFilter string
	if dateRange != nil && dateRange.StartDate != "" && dateRange.EndDate != "" {
		subqueryDateFilter = fmt.Sprintf(" AND we2.workout_date BETWEEN '%s' AND '%s'", dateRange.StartDate, dateRange.EndDate)
		mainQueryDateFilter = fmt.Sprintf(" AND we.workout_date BETWEEN '%s' AND '%s'", dateRange.StartDate, dateRange.EndDate)
	}

	query := fmt.Sprintf(`
		SELECT mg.name as muscle_group,
		       COUNT(*) as exercise_count,
		       ROUND(COUNT(*) * 100.0 / (SELECT COUNT(*)
		                                 FROM workout_exercise we2
		                                 JOIN exercise_muscle_groups emg2 ON we2.exercise_id = emg2.exercise_id
		                                 WHERE we2.workout_user_id = %d
		                                 AND emg2.is_primary = 1%s), 2) as percentage
		FROM workout_exercise we
		JOIN exercise_muscle_groups emg ON we.exercise_id = emg.exercise_id
		JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
		WHERE we.workout_user_id = %d
		AND emg.is_primary = 1%s
		GROUP BY mg.name ORDER BY percentage DESC`, userID, subqueryDateFilter, userID, mainQueryDateFilter)

	description := "Muscle group distribution (percentage of exercises)"
	if dateRange != nil && dateRange.StartDate != "" && dateRange.EndDate != "" {
		description += fmt.Sprintf(" from %s to %s", dateRange.StartDate, dateRange.EndDate)
	}

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query muscle group distribution: %w", err)
	}

	distribution := make(map[string]interface{})
	for _, row := range result.Rows {
		if len(row) >= 3 {
			muscleGroup := row[0].(string)
			percentage := row[2]
			distribution[muscleGroup] = percentage
		}
	}

	return &StatisticsResult{
		MetricType:  "muscle_group_distribution",
		Value:       distribution,
		Description: description,
		UserID:      userID,
	}, nil
}

// calculateProgressionRate calculates weight progression over time for an exercise.
func (t *StatisticsTool) calculateProgressionRate(ctx context.Context, userID int, exerciseName string, dateRange *DateRange) (*StatisticsResult, error) {
	if exerciseName == "" {
		return nil, errors.New("exercise name is required for progression rate calculation")
	}

	query := fmt.Sprintf(`
		SELECT es.workout_date, AVG(es.weight_kg) as avg_weight
		FROM exercise_sets es
		JOIN exercises e ON es.exercise_id = e.id
		WHERE es.workout_user_id = %d
		AND e.name = '%s'
		AND es.weight_kg IS NOT NULL
		AND es.completed_reps IS NOT NULL`, userID, exerciseName)

	if dateRange != nil && dateRange.StartDate != "" && dateRange.EndDate != "" {
		query += fmt.Sprintf(" AND es.workout_date BETWEEN '%s' AND '%s'", dateRange.StartDate, dateRange.EndDate)
	}

	query += " GROUP BY es.workout_date ORDER BY es.workout_date"

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query progression rate: %w", err)
	}

	var progressionRate float64
	var progressionData []map[string]interface{}

	if len(result.Rows) >= 2 {
		// Calculate progression rate between first and last workout
		firstWeight := result.Rows[0][1].(float64)
		lastWeight := result.Rows[len(result.Rows)-1][1].(float64)

		firstDate, _ := time.Parse("2006-01-02", result.Rows[0][0].(string))
		lastDate, _ := time.Parse("2006-01-02", result.Rows[len(result.Rows)-1][0].(string))

		daysDiff := lastDate.Sub(firstDate).Hours() / 24
		if daysDiff > 0 {
			progressionRate = (lastWeight - firstWeight) / daysDiff * 7 // Rate per week
		}

		// Build progression data points
		for _, row := range result.Rows {
			if len(row) >= 2 {
				progressionData = append(progressionData, map[string]interface{}{
					"date":   row[0],
					"weight": row[1],
				})
			}
		}
	}

	description := fmt.Sprintf("Progression rate for %s (kg per week)", exerciseName)
	if dateRange != nil && dateRange.StartDate != "" && dateRange.EndDate != "" {
		description += fmt.Sprintf(" from %s to %s", dateRange.StartDate, dateRange.EndDate)
	}

	value := map[string]interface{}{
		"rate_per_week": progressionRate,
		"data_points":   progressionData,
	}

	return &StatisticsResult{
		MetricType:   "progression_rate",
		ExerciseName: exerciseName,
		Value:        value,
		Description:  description,
		UserID:       userID,
	}, nil
}

// ToOpenAIFunction returns the OpenAI function definition for calculate_statistics.
func (t *StatisticsTool) ToOpenAIFunction() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "calculate_statistics",
			"description": "Calculate statistical metrics from workout data",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"metric_type": map[string]interface{}{
						"type": "string",
						"enum": []string{
							"personal_record",
							"total_volume",
							"workout_frequency",
							"average_intensity",
							"muscle_group_distribution",
							"progression_rate",
						},
						"description": "Type of statistical metric to calculate",
					},
					"exercise_name": map[string]interface{}{
						"type":        "string",
						"description": "Optional: specific exercise to analyze",
					},
					"date_range": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"start_date": map[string]interface{}{
								"type":        "string",
								"format":      "date",
								"description": "Start date for analysis (YYYY-MM-DD)",
							},
							"end_date": map[string]interface{}{
								"type":        "string",
								"format":      "date",
								"description": "End date for analysis (YYYY-MM-DD)",
							},
						},
						"description": "Optional date range for the calculation",
					},
				},
				"required": []string{"metric_type"},
			},
		},
	}
}

// ExecuteFunction executes the calculate_statistics function with the given parameters.
// This method is compatible with OpenAI function calling.
func (t *StatisticsTool) ExecuteFunction(ctx context.Context, functionName string, argumentsJSON string) (string, error) {
	if functionName != "calculate_statistics" {
		return "", fmt.Errorf("unsupported function: %s", functionName)
	}

	var params StatisticsParams
	if err := json.Unmarshal([]byte(argumentsJSON), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	result, err := t.CalculateStatistics(ctx, params)
	if err != nil {
		return "", err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}
