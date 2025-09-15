package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/tools"
)

// WorkoutRecommendationParams represents the parameters for the generate_workout_recommendation function.
type WorkoutRecommendationParams struct {
	WorkoutType        string   `json:"workout_type"`
	DurationMinutes    *int     `json:"duration_minutes,omitempty"`
	EquipmentAvailable []string `json:"equipment_available,omitempty"`
	AvoidMuscleGroups  []string `json:"avoid_muscle_groups,omitempty"`
}

// WorkoutRecommendationResult represents a generated workout recommendation.
type WorkoutRecommendationResult struct {
	WorkoutType       string                `json:"workout_type"`
	EstimatedDuration int                   `json:"estimated_duration"`
	Exercises         []RecommendedExercise `json:"exercises"`
	Notes             []string              `json:"notes,omitempty"`
	WarmupExercises   []RecommendedExercise `json:"warmup_exercises,omitempty"`
	CooldownTips      []string              `json:"cooldown_tips,omitempty"`
}

// RecommendedExercise represents an exercise in a workout recommendation.
type RecommendedExercise struct {
	ExerciseName      string   `json:"exercise_name"`
	Sets              int      `json:"sets"`
	MinReps           int      `json:"min_reps"`
	MaxReps           int      `json:"max_reps"`
	RecommendedWeight *float64 `json:"recommended_weight,omitempty"`
	RestSeconds       int      `json:"rest_seconds"`
	Notes             string   `json:"notes,omitempty"`
	MuscleGroups      []string `json:"muscle_groups"`
}

// exerciseWithHistory represents an exercise with user's historical performance.
type exerciseWithHistory struct {
	ID              int
	Name            string
	Category        string
	ExerciseType    string
	MuscleGroups    []string
	LastPerformed   *string
	AverageWeight   *float64
	PersonalRecord  *float64
	TotalSessions   int
	EquipmentNeeded string
}

// WorkoutRecommendationTool provides intelligent workout recommendations based on user history.
type WorkoutRecommendationTool struct {
	db              *sqlite.Database
	logger          *slog.Logger
	secureQueryTool *tools.SecureQueryTool
}

// NewWorkoutRecommendationTool creates a new WorkoutRecommendationTool instance.
func NewWorkoutRecommendationTool(db *sqlite.Database, logger *slog.Logger) *WorkoutRecommendationTool {
	return &WorkoutRecommendationTool{
		db:              db,
		logger:          logger,
		secureQueryTool: tools.NewSecureQueryTool(db.ReadOnly),
	}
}

// ValidWorkoutTypes defines all valid workout types.
var ValidWorkoutTypes = map[string]bool{
	"strength":    true,
	"hypertrophy": true,
	"endurance":   true,
	"recovery":    true,
	"full_body":   true,
	"upper_body":  true,
	"lower_body":  true,
	"push":        true,
	"pull":        true,
	"legs":        true,
}

// ValidEquipmentTypes defines all valid equipment types.
var ValidEquipmentTypes = map[string]bool{
	"barbell":          true,
	"dumbbells":        true,
	"kettlebell":       true,
	"resistance_bands": true,
	"pull_up_bar":      true,
	"cables":           true,
	"machines":         true,
	"bodyweight_only":  true,
}

// GenerateRecommendation generates a workout recommendation based on user's history and preferences.
func (t *WorkoutRecommendationTool) GenerateRecommendation(ctx context.Context, params WorkoutRecommendationParams) (*WorkoutRecommendationResult, error) {
	// Get user ID from context
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return nil, errors.New("user not authenticated")
	}

	// Validate input parameters
	if err := t.validateParams(params); err != nil {
		return nil, fmt.Errorf("parameter validation failed: %w", err)
	}

	startTime := time.Now()

	// Get user's exercise history for personalization
	exerciseHistory, err := t.getUserExerciseHistory(ctx, userID)
	if err != nil {
		t.logger.WarnContext(ctx, "Failed to get user exercise history",
			"user_id", userID,
			"error", err)
		// Continue with empty history - still provide recommendations
		exerciseHistory = []exerciseWithHistory{}
	}

	// Generate the workout based on type and constraints
	recommendation, err := t.buildWorkoutRecommendation(ctx, params, exerciseHistory, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to build workout recommendation: %w", err)
	}

	executionTime := time.Since(startTime)
	t.logger.InfoContext(ctx, "Workout recommendation generated successfully",
		"user_id", userID,
		"workout_type", params.WorkoutType,
		"exercise_count", len(recommendation.Exercises),
		"execution_time_ms", executionTime.Milliseconds())

	return recommendation, nil
}

// validateParams validates the input parameters for workout recommendation.
func (t *WorkoutRecommendationTool) validateParams(params WorkoutRecommendationParams) error {
	// Validate workout type
	if params.WorkoutType == "" {
		return errors.New("workout_type is required")
	}

	if !ValidWorkoutTypes[params.WorkoutType] {
		return fmt.Errorf("invalid workout_type: %s", params.WorkoutType)
	}

	// Validate duration
	if params.DurationMinutes != nil {
		duration := *params.DurationMinutes
		if duration < 15 || duration > 180 {
			return fmt.Errorf("duration_minutes must be between 15 and 180, got: %d", duration)
		}
	}

	// Validate equipment
	for _, equipment := range params.EquipmentAvailable {
		if !ValidEquipmentTypes[equipment] {
			return fmt.Errorf("invalid equipment type: %s", equipment)
		}
	}

	return nil
}

// getUserExerciseHistory retrieves the user's exercise history for personalization.
func (t *WorkoutRecommendationTool) getUserExerciseHistory(ctx context.Context, userID int) ([]exerciseWithHistory, error) {
	query := fmt.Sprintf(`
		SELECT
			e.id,
			e.name,
			e.category,
			e.exercise_type,
			MAX(ws.workout_date) as last_performed,
			AVG(es.weight_kg) as average_weight,
			MAX(es.weight_kg) as personal_record,
			COUNT(DISTINCT ws.workout_date) as total_sessions
		FROM exercises e
		LEFT JOIN workout_exercise we ON we.exercise_id = e.id AND we.workout_user_id = %d
		LEFT JOIN workout_sessions ws ON ws.user_id = we.workout_user_id AND ws.workout_date = we.workout_date
		LEFT JOIN exercise_sets es ON es.workout_user_id = we.workout_user_id
			AND es.workout_date = we.workout_date
			AND es.exercise_id = we.exercise_id
			AND es.completed_reps IS NOT NULL
		GROUP BY e.id, e.name, e.category, e.exercise_type
		ORDER BY total_sessions DESC, last_performed DESC`, userID)

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	var exercises []exerciseWithHistory
	for _, row := range result.Rows {
		if len(row) < 8 {
			continue
		}

		exercise := exerciseWithHistory{}

		if id, ok := row[0].(int64); ok {
			exercise.ID = int(id)
		}
		if name, ok := row[1].(string); ok {
			exercise.Name = name
		}
		if category, ok := row[2].(string); ok {
			exercise.Category = category
		}
		if exerciseType, ok := row[3].(string); ok {
			exercise.ExerciseType = exerciseType
		}
		if row[4] != nil {
			if lastPerformed, ok := row[4].(string); ok {
				exercise.LastPerformed = &lastPerformed
			}
		}
		if row[5] != nil {
			if avgWeight, ok := row[5].(float64); ok {
				exercise.AverageWeight = &avgWeight
			}
		}
		if row[6] != nil {
			if pr, ok := row[6].(float64); ok {
				exercise.PersonalRecord = &pr
			}
		}
		if totalSessions, ok := row[7].(int64); ok {
			exercise.TotalSessions = int(totalSessions)
		}

		// Get muscle groups for this exercise
		muscleGroups, err := t.getExerciseMuscleGroups(ctx, exercise.ID)
		if err != nil {
			t.logger.WarnContext(ctx, "Failed to get muscle groups for exercise",
				"exercise_id", exercise.ID,
				"error", err)
		} else {
			exercise.MuscleGroups = muscleGroups
		}

		// Infer equipment needed from exercise name and type
		exercise.EquipmentNeeded = t.inferEquipmentNeeded(exercise.Name, exercise.ExerciseType)

		exercises = append(exercises, exercise)
	}

	return exercises, nil
}

// getExerciseMuscleGroups retrieves muscle groups for an exercise.
func (t *WorkoutRecommendationTool) getExerciseMuscleGroups(ctx context.Context, exerciseID int) ([]string, error) {
	query := fmt.Sprintf(`
		SELECT emg.muscle_group_name
		FROM exercise_muscle_groups emg
		WHERE emg.exercise_id = %d
		ORDER BY emg.is_primary DESC, emg.muscle_group_name ASC`, exerciseID)

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	var muscleGroups []string
	for _, row := range result.Rows {
		if len(row) > 0 {
			if muscleGroup, ok := row[0].(string); ok {
				muscleGroups = append(muscleGroups, muscleGroup)
			}
		}
	}

	return muscleGroups, nil
}

// inferEquipmentNeeded infers equipment needed based on exercise name and type.
func (t *WorkoutRecommendationTool) inferEquipmentNeeded(exerciseName, exerciseType string) string {
	lowerName := strings.ToLower(exerciseName)

	if exerciseType == "bodyweight" {
		if strings.Contains(lowerName, "pull-up") || strings.Contains(lowerName, "chin-up") {
			return "pull_up_bar"
		}
		return "bodyweight_only"
	}

	// Infer equipment from exercise name
	switch {
	case strings.Contains(lowerName, "barbell") || strings.Contains(lowerName, "deadlift") ||
		strings.Contains(lowerName, "bench press") && !strings.Contains(lowerName, "dumbbell"):
		return "barbell"
	case strings.Contains(lowerName, "dumbbell"):
		return "dumbbells"
	case strings.Contains(lowerName, "kettlebell"):
		return "kettlebell"
	case strings.Contains(lowerName, "cable") || strings.Contains(lowerName, "pulldown") ||
		strings.Contains(lowerName, "row") && strings.Contains(lowerName, "seated"):
		return "cables"
	case strings.Contains(lowerName, "machine") || strings.Contains(lowerName, "leg press") ||
		strings.Contains(lowerName, "leg extension") || strings.Contains(lowerName, "leg curl"):
		return "machines"
	case strings.Contains(lowerName, "resistance band"):
		return "resistance_bands"
	default:
		return "dumbbells" // Default assumption for weighted exercises
	}
}

// buildWorkoutRecommendation builds the complete workout recommendation.
func (t *WorkoutRecommendationTool) buildWorkoutRecommendation(ctx context.Context, params WorkoutRecommendationParams, history []exerciseWithHistory, userID int) (*WorkoutRecommendationResult, error) {
	// Filter exercises based on constraints
	filteredExercises := t.filterExercises(history, params)

	// Select exercises based on workout type
	selectedExercises := t.selectExercisesForWorkout(params.WorkoutType, filteredExercises)

	// Determine target duration
	targetDuration := 60 // Default 60 minutes
	if params.DurationMinutes != nil {
		targetDuration = *params.DurationMinutes
	}

	// Build recommended exercises with sets/reps/weights
	recommendedExercises := t.buildRecommendedExercises(selectedExercises, params.WorkoutType, targetDuration)

	// Calculate estimated duration
	estimatedDuration := t.calculateEstimatedDuration(recommendedExercises)

	// Generate notes and tips
	notes := t.generateWorkoutNotes(params.WorkoutType, len(selectedExercises))
	cooldownTips := t.generateCooldownTips(params.WorkoutType)

	return &WorkoutRecommendationResult{
		WorkoutType:       params.WorkoutType,
		EstimatedDuration: estimatedDuration,
		Exercises:         recommendedExercises,
		Notes:             notes,
		CooldownTips:      cooldownTips,
	}, nil
}

// filterExercises filters exercises based on equipment and muscle group constraints.
func (t *WorkoutRecommendationTool) filterExercises(exercises []exerciseWithHistory, params WorkoutRecommendationParams) []exerciseWithHistory {
	var filtered []exerciseWithHistory

	// Create equipment map for quick lookup
	equipmentMap := make(map[string]bool)
	for _, equipment := range params.EquipmentAvailable {
		equipmentMap[equipment] = true
	}

	// Create avoided muscle groups map
	avoidedMuscles := make(map[string]bool)
	for _, muscle := range params.AvoidMuscleGroups {
		avoidedMuscles[muscle] = true
	}

	for _, exercise := range exercises {
		// Filter by equipment if specified
		if len(params.EquipmentAvailable) > 0 {
			if !equipmentMap[exercise.EquipmentNeeded] {
				continue
			}
		}

		// Filter by avoided muscle groups
		shouldAvoid := false
		for _, muscleGroup := range exercise.MuscleGroups {
			if avoidedMuscles[muscleGroup] {
				shouldAvoid = true
				break
			}
		}
		if shouldAvoid {
			continue
		}

		filtered = append(filtered, exercise)
	}

	return filtered
}

// selectExercisesForWorkout selects appropriate exercises based on workout type.
func (t *WorkoutRecommendationTool) selectExercisesForWorkout(workoutType string, exercises []exerciseWithHistory) []exerciseWithHistory {
	var selected []exerciseWithHistory

	switch workoutType {
	case "strength":
		selected = t.selectForStrength(exercises)
	case "hypertrophy":
		selected = t.selectForHypertrophy(exercises)
	case "endurance":
		selected = t.selectForEndurance(exercises)
	case "recovery":
		selected = t.selectForRecovery(exercises)
	case "full_body":
		selected = t.selectForFullBody(exercises)
	case "upper_body":
		selected = t.selectForUpperBody(exercises)
	case "lower_body":
		selected = t.selectForLowerBody(exercises)
	case "push":
		selected = t.selectForPush(exercises)
	case "pull":
		selected = t.selectForPull(exercises)
	case "legs":
		selected = t.selectForLegs(exercises)
	default:
		selected = t.selectForFullBody(exercises) // Fallback
	}

	return selected
}

// selectForStrength selects exercises optimal for strength training.
func (t *WorkoutRecommendationTool) selectForStrength(exercises []exerciseWithHistory) []exerciseWithHistory {
	// Prioritize compound movements and exercises with user history
	var selected []exerciseWithHistory

	// Compound movements for strength
	compoundExercises := []string{
		"Deadlift", "Bench Press", "Squat", "Pulldown", "Seated Cable Row",
	}

	// Add compound exercises first
	for _, compoundName := range compoundExercises {
		for _, exercise := range exercises {
			if exercise.Name == compoundName {
				selected = append(selected, exercise)
				break
			}
		}
	}

	// Add accessories if we need more exercises
	for _, exercise := range exercises {
		if len(selected) >= 6 {
			break
		}

		// Skip if already selected
		isAlreadySelected := false
		for _, sel := range selected {
			if sel.ID == exercise.ID {
				isAlreadySelected = true
				break
			}
		}
		if isAlreadySelected {
			continue
		}

		// Prioritize exercises with user experience
		if exercise.TotalSessions > 0 {
			selected = append(selected, exercise)
		}
	}

	return selected
}

// selectForHypertrophy selects exercises optimal for muscle growth.
func (t *WorkoutRecommendationTool) selectForHypertrophy(exercises []exerciseWithHistory) []exerciseWithHistory {
	// Balance compound and isolation movements
	var selected []exerciseWithHistory

	// Sort by total sessions to prioritize familiar exercises
	sort.Slice(exercises, func(i, j int) bool {
		return exercises[i].TotalSessions > exercises[j].TotalSessions
	})

	// Select up to 8 exercises for hypertrophy volume
	for i, exercise := range exercises {
		if i >= 8 {
			break
		}
		selected = append(selected, exercise)
	}

	return selected
}

// selectForEndurance prioritizes bodyweight and high-rep exercises.
func (t *WorkoutRecommendationTool) selectForEndurance(exercises []exerciseWithHistory) []exerciseWithHistory {
	var selected []exerciseWithHistory

	// Prioritize bodyweight exercises
	for _, exercise := range exercises {
		if exercise.ExerciseType == "bodyweight" {
			selected = append(selected, exercise)
		}
	}

	// Add light weight exercises if needed
	for _, exercise := range exercises {
		if len(selected) >= 6 {
			break
		}

		// Skip if already selected
		isAlreadySelected := false
		for _, sel := range selected {
			if sel.ID == exercise.ID {
				isAlreadySelected = true
				break
			}
		}
		if !isAlreadySelected && exercise.ExerciseType == "weighted" {
			selected = append(selected, exercise)
		}
	}

	return selected
}

// selectForRecovery selects light, mobility-focused exercises.
func (t *WorkoutRecommendationTool) selectForRecovery(exercises []exerciseWithHistory) []exerciseWithHistory {
	var selected []exerciseWithHistory

	// Prioritize bodyweight and light exercises
	recoveryExercises := []string{
		"Plank", "Push-up", "Back Extension", "Calf Raise",
	}

	for _, recoveryName := range recoveryExercises {
		for _, exercise := range exercises {
			if exercise.Name == recoveryName {
				selected = append(selected, exercise)
				break
			}
		}
	}

	// Limit to 4 exercises for recovery
	if len(selected) > 4 {
		selected = selected[:4]
	}

	return selected
}

// selectForFullBody selects exercises that target all major muscle groups.
func (t *WorkoutRecommendationTool) selectForFullBody(exercises []exerciseWithHistory) []exerciseWithHistory {
	var selected []exerciseWithHistory

	// Target muscle groups to cover
	targetGroups := map[string]bool{
		"Chest": false, "Back": false, "Legs": false, "Shoulders": false, "Arms": false,
	}

	for _, exercise := range exercises {
		if len(selected) >= 8 {
			break
		}

		// Check if this exercise covers needed muscle groups
		coversNeeded := false
		for _, muscleGroup := range exercise.MuscleGroups {
			switch muscleGroup {
			case "Chest":
				if !targetGroups["Chest"] {
					targetGroups["Chest"] = true
					coversNeeded = true
				}
			case "Upper Back", "Lats":
				if !targetGroups["Back"] {
					targetGroups["Back"] = true
					coversNeeded = true
				}
			case "Quads", "Hamstrings", "Glutes":
				if !targetGroups["Legs"] {
					targetGroups["Legs"] = true
					coversNeeded = true
				}
			case "Shoulders":
				if !targetGroups["Shoulders"] {
					targetGroups["Shoulders"] = true
					coversNeeded = true
				}
			case "Biceps", "Triceps":
				if !targetGroups["Arms"] {
					targetGroups["Arms"] = true
					coversNeeded = true
				}
			}
		}

		if coversNeeded || exercise.TotalSessions > 0 {
			selected = append(selected, exercise)
		}
	}

	return selected
}

// selectForUpperBody selects exercises targeting upper body muscles.
func (t *WorkoutRecommendationTool) selectForUpperBody(exercises []exerciseWithHistory) []exerciseWithHistory {
	var selected []exerciseWithHistory

	for _, exercise := range exercises {
		if exercise.Category == "upper" || exercise.Category == "full_body" {
			selected = append(selected, exercise)
		}
		if len(selected) >= 8 {
			break
		}
	}

	return selected
}

// selectForLowerBody selects exercises targeting lower body muscles.
func (t *WorkoutRecommendationTool) selectForLowerBody(exercises []exerciseWithHistory) []exerciseWithHistory {
	var selected []exerciseWithHistory

	for _, exercise := range exercises {
		if exercise.Category == "lower" || exercise.Category == "full_body" {
			selected = append(selected, exercise)
		}
		if len(selected) >= 6 {
			break
		}
	}

	return selected
}

// selectForPush selects pushing movement exercises.
func (t *WorkoutRecommendationTool) selectForPush(exercises []exerciseWithHistory) []exerciseWithHistory {
	var selected []exerciseWithHistory

	pushExercises := []string{
		"Bench Press", "Dumbbell Bench Press", "Push-up", "Dumbbell Shoulder Press",
		"Tricep Pushdown", "Lateral Raise", "Cable Fly",
	}

	for _, pushName := range pushExercises {
		for _, exercise := range exercises {
			if exercise.Name == pushName {
				selected = append(selected, exercise)
				break
			}
		}
	}

	return selected
}

// selectForPull selects pulling movement exercises.
func (t *WorkoutRecommendationTool) selectForPull(exercises []exerciseWithHistory) []exerciseWithHistory {
	var selected []exerciseWithHistory

	pullExercises := []string{
		"Pulldown", "Pulldown, Reverse Grip", "Seated Cable Row", "One-Arm Dumbell Row",
		"Dumbbell Biceps Curl", "Deadlift",
	}

	for _, pullName := range pullExercises {
		for _, exercise := range exercises {
			if exercise.Name == pullName {
				selected = append(selected, exercise)
				break
			}
		}
	}

	return selected
}

// selectForLegs selects leg-focused exercises.
func (t *WorkoutRecommendationTool) selectForLegs(exercises []exerciseWithHistory) []exerciseWithHistory {
	var selected []exerciseWithHistory

	legExercises := []string{
		"Leg Press", "Leg Extension", "Leg Curl", "Calf Raise", "Deadlift",
	}

	for _, legName := range legExercises {
		for _, exercise := range exercises {
			if exercise.Name == legName {
				selected = append(selected, exercise)
				break
			}
		}
	}

	return selected
}

// buildRecommendedExercises creates recommended exercises with sets, reps, and weights.
func (t *WorkoutRecommendationTool) buildRecommendedExercises(exercises []exerciseWithHistory, workoutType string, targetDuration int) []RecommendedExercise {
	var recommended []RecommendedExercise

	for _, exercise := range exercises {
		recExercise := RecommendedExercise{
			ExerciseName: exercise.Name,
			MuscleGroups: exercise.MuscleGroups,
		}

		// Set parameters based on workout type
		switch workoutType {
		case "strength":
			recExercise.Sets = 5
			recExercise.MinReps = 3
			recExercise.MaxReps = 5
			recExercise.RestSeconds = 180
		case "hypertrophy":
			recExercise.Sets = 4
			recExercise.MinReps = 8
			recExercise.MaxReps = 12
			recExercise.RestSeconds = 90
		case "endurance":
			recExercise.Sets = 3
			recExercise.MinReps = 15
			recExercise.MaxReps = 20
			recExercise.RestSeconds = 60
		case "recovery":
			recExercise.Sets = 2
			recExercise.MinReps = 10
			recExercise.MaxReps = 15
			recExercise.RestSeconds = 45
		default:
			recExercise.Sets = 3
			recExercise.MinReps = 8
			recExercise.MaxReps = 12
			recExercise.RestSeconds = 75
		}

		// Adjust for bodyweight exercises
		if exercise.ExerciseType == "bodyweight" {
			recExercise.RestSeconds = int(float64(recExercise.RestSeconds) * 0.75)
		}

		// Calculate recommended weight based on user history
		if exercise.ExerciseType == "weighted" && exercise.PersonalRecord != nil {
			var intensity float64
			switch workoutType {
			case "strength":
				intensity = 0.85 // 85% of PR for strength
			case "hypertrophy":
				intensity = 0.75 // 75% of PR for hypertrophy
			case "endurance":
				intensity = 0.60 // 60% of PR for endurance
			case "recovery":
				intensity = 0.50 // 50% of PR for recovery
			default:
				intensity = 0.70 // 70% default
			}

			recommendedWeight := *exercise.PersonalRecord * intensity
			recExercise.RecommendedWeight = &recommendedWeight
		} else if exercise.AverageWeight != nil {
			// Use average weight as baseline if no PR
			recExercise.RecommendedWeight = exercise.AverageWeight
		}

		// Add personalized notes
		if exercise.TotalSessions == 0 {
			recExercise.Notes = "New exercise - start with lighter weight to learn proper form"
		} else if exercise.LastPerformed != nil {
			recExercise.Notes = fmt.Sprintf("Last performed: %s", *exercise.LastPerformed)
		}

		recommended = append(recommended, recExercise)
	}

	return recommended
}

// calculateEstimatedDuration estimates workout duration based on exercises.
func (t *WorkoutRecommendationTool) calculateEstimatedDuration(exercises []RecommendedExercise) int {
	totalTime := 0

	for _, exercise := range exercises {
		// Exercise time = (sets * average_reps * 3_seconds) + (sets * rest_time)
		avgReps := (exercise.MinReps + exercise.MaxReps) / 2
		exerciseTime := (exercise.Sets * avgReps * 3) + (exercise.Sets * exercise.RestSeconds)
		totalTime += exerciseTime
	}

	// Add warmup and cooldown time
	totalTime += 10 * 60 // 10 minutes warmup/cooldown

	// Convert to minutes and round up
	return int(math.Ceil(float64(totalTime) / 60.0))
}

// generateWorkoutNotes generates helpful notes for the workout.
func (t *WorkoutRecommendationTool) generateWorkoutNotes(workoutType string, exerciseCount int) []string {
	var notes []string

	switch workoutType {
	case "strength":
		notes = append(notes, "Focus on progressive overload - gradually increase weight each week")
		notes = append(notes, "Take full rest periods between sets for maximum strength gains")
	case "hypertrophy":
		notes = append(notes, "Focus on controlled movements and time under tension")
		notes = append(notes, "Aim for muscle fatigue in the target rep range")
	case "endurance":
		notes = append(notes, "Keep rest periods short to maintain elevated heart rate")
		notes = append(notes, "Focus on maintaining good form even when fatigued")
	case "recovery":
		notes = append(notes, "Use lighter weights and focus on movement quality")
		notes = append(notes, "This workout should feel restorative, not exhausting")
	}

	if exerciseCount > 6 {
		notes = append(notes, "Consider splitting this workout across multiple sessions if time is limited")
	}

	notes = append(notes, "Always warm up properly before starting your workout")
	notes = append(notes, "Listen to your body and adjust weights as needed")

	return notes
}

// generateCooldownTips generates cooldown recommendations.
func (t *WorkoutRecommendationTool) generateCooldownTips(workoutType string) []string {
	tips := []string{
		"Spend 5-10 minutes doing light cardio to gradually lower heart rate",
		"Perform static stretches holding each for 20-30 seconds",
		"Focus on stretching the muscle groups you worked today",
		"Stay hydrated and consider a protein-rich snack within 30 minutes",
	}

	if workoutType == "strength" {
		tips = append(tips, "Consider a gentle massage or foam rolling for muscle recovery")
	}

	return tips
}

// ToOpenAIFunction returns the OpenAI function definition for generate_workout_recommendation.
func (t *WorkoutRecommendationTool) ToOpenAIFunction() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "generate_workout_recommendation",
			"description": "Generate a workout recommendation based on user's history and goals",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"workout_type": map[string]interface{}{
						"type": "string",
						"enum": []string{
							"strength", "hypertrophy", "endurance", "recovery",
							"full_body", "upper_body", "lower_body", "push", "pull", "legs",
						},
						"description": "Type of workout to recommend",
					},
					"duration_minutes": map[string]interface{}{
						"type":        "integer",
						"description": "Desired workout duration in minutes",
						"minimum":     15,
						"maximum":     180,
					},
					"equipment_available": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
							"enum": []string{
								"barbell", "dumbbells", "kettlebell", "resistance_bands",
								"pull_up_bar", "cables", "machines", "bodyweight_only",
							},
						},
						"description": "Available equipment for the workout",
					},
					"avoid_muscle_groups": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "Muscle groups to avoid (e.g., due to soreness)",
					},
				},
				"required": []string{"workout_type"},
			},
		},
	}
}

// ExecuteFunction executes the generate_workout_recommendation function with the given parameters.
// This method is compatible with OpenAI function calling.
func (t *WorkoutRecommendationTool) ExecuteFunction(ctx context.Context, functionName string, argumentsJSON string) (string, error) {
	if functionName != "generate_workout_recommendation" {
		return "", fmt.Errorf("unsupported function: %s", functionName)
	}

	var params WorkoutRecommendationParams
	if err := json.Unmarshal([]byte(argumentsJSON), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	result, err := t.GenerateRecommendation(ctx, params)
	if err != nil {
		return "", err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}
