package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/tools"
)

// ExerciseInfoParams represents the parameters for the get_exercise_info function.
type ExerciseInfoParams struct {
	ExerciseName   string `json:"exercise_name"`
	IncludeHistory bool   `json:"include_history"`
}

// ExerciseInfoResult represents detailed information about an exercise.
type ExerciseInfoResult struct {
	ExerciseName        string           `json:"exercise_name"`
	Category            string           `json:"category"`
	ExerciseType        string           `json:"exercise_type"`
	Description         string           `json:"description"`
	MuscleGroups        []string         `json:"muscle_groups"`
	PrimaryMuscleGroups []string         `json:"primary_muscle_groups"`
	UserHistory         *ExerciseHistory `json:"user_history,omitempty"`
}

// ExerciseHistory represents user's history with a specific exercise.
type ExerciseHistory struct {
	FirstPerformed *string  `json:"first_performed"`
	LastPerformed  *string  `json:"last_performed"`
	TotalSessions  int      `json:"total_sessions"`
	PersonalRecord *float64 `json:"personal_record"`
	AverageWeight  *float64 `json:"average_weight"`
	TotalVolume    float64  `json:"total_volume"`
}

// ExerciseInfoTool provides exercise information retrieval with optional user history.
type ExerciseInfoTool struct {
	db              *sqlite.Database
	logger          *slog.Logger
	secureQueryTool *tools.SecureQueryTool
}

// NewExerciseInfoTool creates a new ExerciseInfoTool instance.
func NewExerciseInfoTool(db *sqlite.Database, logger *slog.Logger) *ExerciseInfoTool {
	return &ExerciseInfoTool{
		db:              db,
		logger:          logger,
		secureQueryTool: tools.NewSecureQueryTool(db.ReadOnly),
	}
}

// GetExerciseInfo retrieves detailed information about a specific exercise.
// This function ensures that:
// 1. Exercise information is retrieved from the database
// 2. Muscle groups are correctly categorized as primary/secondary
// 3. User history is included if requested and user is authenticated
// 4. All queries are secured and user-isolated
func (t *ExerciseInfoTool) GetExerciseInfo(ctx context.Context, params ExerciseInfoParams) (*ExerciseInfoResult, error) {
	// Validate input
	if strings.TrimSpace(params.ExerciseName) == "" {
		return nil, errors.New("exercise name cannot be empty")
	}

	// Get exercise basic information
	exercise, err := t.getExerciseDetails(ctx, params.ExerciseName)
	if err != nil {
		return nil, fmt.Errorf("failed to get exercise details: %w", err)
	}

	// Get muscle groups for the exercise
	muscleGroups, primaryMuscleGroups, err := t.getExerciseMuscleGroups(ctx, exercise.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get muscle groups: %w", err)
	}

	result := &ExerciseInfoResult{
		ExerciseName:        exercise.Name,
		Category:            exercise.Category,
		ExerciseType:        exercise.ExerciseType,
		Description:         exercise.Description,
		MuscleGroups:        muscleGroups,
		PrimaryMuscleGroups: primaryMuscleGroups,
	}

	// Include user history if requested
	if params.IncludeHistory {
		userID := contexthelpers.AuthenticatedUserID(ctx)
		if userID == 0 {
			return nil, errors.New("user not authenticated")
		}

		history, err := t.getUserExerciseHistory(ctx, exercise.ID, userID)
		if err != nil {
			// Log the error but don't fail the request - user might not have history
			t.logger.WarnContext(ctx, "Failed to get user exercise history",
				"exercise_id", exercise.ID,
				"user_id", userID,
				"error", err)
		} else {
			result.UserHistory = history
		}
	}

	return result, nil
}

// exerciseDetails represents the basic exercise information from database.
type exerciseDetails struct {
	ID           int
	Name         string
	Category     string
	ExerciseType string
	Description  string
}

// getExerciseDetails retrieves basic exercise information from the database.
func (t *ExerciseInfoTool) getExerciseDetails(ctx context.Context, exerciseName string) (*exerciseDetails, error) {
	query := `
		SELECT id, name, category, exercise_type, description_markdown
		FROM exercises
		WHERE LOWER(name) = LOWER(?)
		LIMIT 1`

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	if result.RowCount == 0 {
		return nil, fmt.Errorf("exercise '%s' not found", exerciseName)
	}

	row := result.Rows[0]
	if len(row) < 5 {
		return nil, errors.New("invalid query result structure")
	}

	// Convert interface{} values to appropriate types
	id, ok := row[0].(int64)
	if !ok {
		return nil, errors.New("invalid exercise ID type")
	}

	name, ok := row[1].(string)
	if !ok {
		return nil, errors.New("invalid exercise name type")
	}

	category, ok := row[2].(string)
	if !ok {
		return nil, errors.New("invalid exercise category type")
	}

	exerciseType, ok := row[3].(string)
	if !ok {
		return nil, errors.New("invalid exercise type")
	}

	description, ok := row[4].(string)
	if !ok {
		return nil, errors.New("invalid exercise description type")
	}

	return &exerciseDetails{
		ID:           int(id),
		Name:         name,
		Category:     category,
		ExerciseType: exerciseType,
		Description:  description,
	}, nil
}

// getExerciseMuscleGroups retrieves muscle groups for an exercise, separated by primary/secondary.
func (t *ExerciseInfoTool) getExerciseMuscleGroups(ctx context.Context, exerciseID int) ([]string, []string, error) {
	query := `
		SELECT emg.muscle_group_name, emg.is_primary
		FROM exercise_muscle_groups emg
		WHERE emg.exercise_id = ?
		ORDER BY emg.is_primary DESC, emg.muscle_group_name ASC`

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("query execution failed: %w", err)
	}

	var allMuscleGroups []string
	var primaryMuscleGroups []string

	for _, row := range result.Rows {
		if len(row) < 2 {
			continue
		}

		muscleGroupName, ok := row[0].(string)
		if !ok {
			continue
		}

		isPrimary, ok := row[1].(int64)
		if !ok {
			continue
		}

		allMuscleGroups = append(allMuscleGroups, muscleGroupName)
		if isPrimary == 1 {
			primaryMuscleGroups = append(primaryMuscleGroups, muscleGroupName)
		}
	}

	return allMuscleGroups, primaryMuscleGroups, nil
}

// getUserExerciseHistory retrieves user's workout history for a specific exercise.
func (t *ExerciseInfoTool) getUserExerciseHistory(ctx context.Context, exerciseID, userID int) (*ExerciseHistory, error) {
	// Query to get comprehensive exercise history
	query := `
		SELECT
			MIN(ws.workout_date) as first_performed,
			MAX(ws.workout_date) as last_performed,
			COUNT(DISTINCT ws.workout_date) as total_sessions,
			MAX(es.weight_kg) as personal_record,
			AVG(es.weight_kg) as average_weight,
			SUM(COALESCE(es.weight_kg, 0) * COALESCE(es.completed_reps, 0)) as total_volume
		FROM workout_sessions ws
		JOIN workout_exercise we ON we.workout_user_id = ws.user_id AND we.workout_date = ws.workout_date
		JOIN exercise_sets es ON es.workout_user_id = we.workout_user_id
			AND es.workout_date = we.workout_date
			AND es.exercise_id = we.exercise_id
		WHERE ws.user_id = ? AND we.exercise_id = ? AND es.completed_reps IS NOT NULL`

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	if result.RowCount == 0 {
		// User has no history with this exercise
		return &ExerciseHistory{
			TotalSessions: 0,
			TotalVolume:   0,
		}, nil
	}

	row := result.Rows[0]
	if len(row) < 6 {
		return nil, errors.New("invalid query result structure")
	}

	history := &ExerciseHistory{}

	// First performed (can be null)
	if row[0] != nil {
		if firstPerformed, ok := row[0].(string); ok {
			history.FirstPerformed = &firstPerformed
		}
	}

	// Last performed (can be null)
	if row[1] != nil {
		if lastPerformed, ok := row[1].(string); ok {
			history.LastPerformed = &lastPerformed
		}
	}

	// Total sessions
	if totalSessions, ok := row[2].(int64); ok {
		history.TotalSessions = int(totalSessions)
	}

	// Personal record (can be null)
	if row[3] != nil {
		if pr, ok := row[3].(float64); ok {
			history.PersonalRecord = &pr
		}
	}

	// Average weight (can be null)
	if row[4] != nil {
		if avgWeight, ok := row[4].(float64); ok {
			history.AverageWeight = &avgWeight
		}
	}

	// Total volume
	if totalVolume, ok := row[5].(float64); ok {
		history.TotalVolume = totalVolume
	}

	return history, nil
}

// ToOpenAIFunction returns the OpenAI function definition for get_exercise_info.
func (t *ExerciseInfoTool) ToOpenAIFunction() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "get_exercise_info",
			"description": "Retrieve detailed information about a specific exercise",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"exercise_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the exercise to look up",
					},
					"include_history": map[string]interface{}{
						"type":        "boolean",
						"description": "Whether to include user's history with this exercise",
						"default":     false,
					},
				},
				"required": []string{"exercise_name"},
			},
		},
	}
}

// ExecuteFunction executes the get_exercise_info function with the given parameters.
// This method is compatible with OpenAI function calling.
func (t *ExerciseInfoTool) ExecuteFunction(ctx context.Context, functionName string, argumentsJSON string) (string, error) {
	if functionName != "get_exercise_info" {
		return "", fmt.Errorf("unsupported function: %s", functionName)
	}

	var params ExerciseInfoParams
	if err := json.Unmarshal([]byte(argumentsJSON), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	result, err := t.GetExerciseInfo(ctx, params)
	if err != nil {
		return "", err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}
