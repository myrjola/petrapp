package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/tools"
)

// PatternAnalysisParams represents the parameters for the analyze_workout_pattern function.
type PatternAnalysisParams struct {
	AnalysisType string `json:"analysis_type"`
	LookbackDays int    `json:"lookback_days"`
}

// PatternAnalysisResult represents the result of workout pattern analysis.
type PatternAnalysisResult struct {
	AnalysisType    string                 `json:"analysis_type"`
	LookbackDays    int                    `json:"lookback_days"`
	Summary         string                 `json:"summary"`
	Insights        []string               `json:"insights"`
	Recommendations []string               `json:"recommendations"`
	MetricsData     map[string]interface{} `json:"metrics_data"`
	Score           *float64               `json:"score,omitempty"`
	UserID          int                    `json:"user_id,omitempty"`
}

// WorkoutPatternTool provides pattern analysis functionality for workout data.
type WorkoutPatternTool struct {
	db              *sqlite.Database
	logger          *slog.Logger
	secureQueryTool *tools.SecureQueryTool
}

// NewWorkoutPatternTool creates a new WorkoutPatternTool instance.
func NewWorkoutPatternTool(db *sqlite.Database, logger *slog.Logger) *WorkoutPatternTool {
	return &WorkoutPatternTool{
		db:              db,
		logger:          logger,
		secureQueryTool: tools.NewSecureQueryTool(db.ReadOnly),
	}
}

// AnalyzePattern analyzes patterns in user's workout history.
// This function ensures that:
// 1. Only authenticated users can access their own data
// 2. All database queries are securely executed with user isolation
// 3. Pattern analysis is meaningful and actionable
func (t *WorkoutPatternTool) AnalyzePattern(ctx context.Context, params PatternAnalysisParams) (*PatternAnalysisResult, error) {
	// Get user ID from context
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return nil, errors.New("user not authenticated")
	}

	// Validate parameters
	if err := t.validateParams(params); err != nil {
		return nil, fmt.Errorf("parameter validation failed: %w", err)
	}

	// Calculate the analysis period
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -params.LookbackDays)

	// Perform the specific pattern analysis
	var result *PatternAnalysisResult
	var err error

	switch params.AnalysisType {
	case "consistency":
		result, err = t.analyzeConsistency(ctx, userID, startDate, endDate, params.LookbackDays)
	case "progressive_overload":
		result, err = t.analyzeProgressiveOverload(ctx, userID, startDate, endDate, params.LookbackDays)
	case "muscle_balance":
		result, err = t.analyzeMuscleBalance(ctx, userID, startDate, endDate, params.LookbackDays)
	case "recovery_time":
		result, err = t.analyzeRecoveryTime(ctx, userID, startDate, endDate, params.LookbackDays)
	case "plateau_detection":
		result, err = t.analyzePlateauDetection(ctx, userID, startDate, endDate, params.LookbackDays)
	case "workout_variety":
		result, err = t.analyzeWorkoutVariety(ctx, userID, startDate, endDate, params.LookbackDays)
	default:
		return nil, fmt.Errorf("unsupported analysis type: %s", params.AnalysisType)
	}

	if err != nil {
		t.logger.ErrorContext(ctx, "Pattern analysis failed",
			"user_id", userID,
			"analysis_type", params.AnalysisType,
			"lookback_days", params.LookbackDays,
			"error", err)
		return nil, fmt.Errorf("pattern analysis failed: %w", err)
	}

	t.logger.InfoContext(ctx, "Pattern analysis completed successfully",
		"user_id", userID,
		"analysis_type", params.AnalysisType,
		"lookback_days", params.LookbackDays)

	return result, nil
}

// validateParams ensures the parameters are valid.
func (t *WorkoutPatternTool) validateParams(params PatternAnalysisParams) error {
	validAnalysisTypes := []string{
		"consistency",
		"progressive_overload",
		"muscle_balance",
		"recovery_time",
		"plateau_detection",
		"workout_variety",
	}

	// Validate analysis type
	valid := false
	for _, validType := range validAnalysisTypes {
		if params.AnalysisType == validType {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid analysis type: %s", params.AnalysisType)
	}

	// Validate lookback days
	if params.LookbackDays < 7 {
		return errors.New("lookback days must be at least 7")
	}
	if params.LookbackDays > 365 {
		return errors.New("lookback days cannot exceed 365")
	}

	return nil
}

// analyzeConsistency analyzes workout consistency patterns.
func (t *WorkoutPatternTool) analyzeConsistency(ctx context.Context, userID int, startDate, endDate time.Time, lookbackDays int) (*PatternAnalysisResult, error) {
	// Query workout frequency
	query := fmt.Sprintf(`
		SELECT COUNT(DISTINCT workout_date) as workout_count,
		       COUNT(DISTINCT strftime('%%W-%%Y', workout_date)) as weeks_with_workouts,
		       MIN(workout_date) as first_workout,
		       MAX(workout_date) as last_workout
		FROM workout_sessions
		WHERE user_id = %d
		AND workout_date BETWEEN '%s' AND '%s'
		AND completed_at IS NOT NULL`,
		userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query workout frequency: %w", err)
	}

	var workoutCount, weeksWithWorkouts int64
	var firstWorkout, lastWorkout string

	if len(result.Rows) > 0 && len(result.Rows[0]) >= 4 {
		if count, ok := result.Rows[0][0].(int64); ok {
			workoutCount = count
		}
		if weeks, ok := result.Rows[0][1].(int64); ok {
			weeksWithWorkouts = weeks
		}
		if first, ok := result.Rows[0][2].(string); ok {
			firstWorkout = first
		}
		if last, ok := result.Rows[0][3].(string); ok {
			lastWorkout = last
		}
	}

	// Calculate consistency metrics
	totalWeeks := float64(lookbackDays) / 7
	workoutFrequency := float64(workoutCount) / totalWeeks
	weeklyConsistency := float64(weeksWithWorkouts) / totalWeeks * 100

	// Calculate consistency score (0-100)
	consistencyScore := math.Min(100, weeklyConsistency)

	// Generate insights and recommendations
	insights := []string{}
	recommendations := []string{}

	if workoutCount == 0 {
		insights = append(insights, "No completed workouts found in the analysis period.")
		recommendations = append(recommendations, "Start with 2-3 workouts per week to build a consistent routine.")
		recommendations = append(recommendations, "Choose activities you enjoy to increase adherence.")
	} else {
		insights = append(insights, fmt.Sprintf("Completed %d workouts over %d days (%.1f workouts per week).", workoutCount, lookbackDays, workoutFrequency))
		insights = append(insights, fmt.Sprintf("Had workouts in %d out of %.0f weeks (%.1f%% weekly consistency).", weeksWithWorkouts, totalWeeks, weeklyConsistency))

		if consistencyScore >= 80 {
			insights = append(insights, "Excellent workout consistency!")
			recommendations = append(recommendations, "Maintain your current routine while gradually increasing intensity.")
		} else if consistencyScore >= 60 {
			insights = append(insights, "Good consistency with room for improvement.")
			recommendations = append(recommendations, "Try to maintain at least 3 workouts per week.")
			recommendations = append(recommendations, "Consider scheduling workouts at the same time each day.")
		} else if consistencyScore >= 40 {
			insights = append(insights, "Moderate consistency - focus on building routine.")
			recommendations = append(recommendations, "Start with 2 workouts per week and gradually increase.")
			recommendations = append(recommendations, "Set specific workout days and times to build a habit.")
		} else {
			insights = append(insights, "Low consistency indicates difficulty maintaining routine.")
			recommendations = append(recommendations, "Start with just 1-2 workouts per week to build the habit.")
			recommendations = append(recommendations, "Consider shorter workouts (15-30 minutes) to reduce barriers.")
		}

		// Check for gaps in workout schedule
		if workoutCount > 0 && firstWorkout != "" && lastWorkout != "" {
			last, _ := time.Parse("2006-01-02", lastWorkout)
			daysSinceLastWorkout := int(endDate.Sub(last).Hours() / 24)

			if daysSinceLastWorkout > 7 {
				insights = append(insights, fmt.Sprintf("Last workout was %d days ago - longer gap than usual.", daysSinceLastWorkout))
				recommendations = append(recommendations, "Schedule your next workout soon to maintain momentum.")
			}
		}
	}

	summary := fmt.Sprintf("Workout consistency analysis over %d days shows %.1f%% weekly consistency with %.1f workouts per week.",
		lookbackDays, weeklyConsistency, workoutFrequency)

	metricsData := map[string]interface{}{
		"workout_count":       workoutCount,
		"workout_frequency":   workoutFrequency,
		"weekly_consistency":  weeklyConsistency,
		"weeks_with_workouts": weeksWithWorkouts,
		"total_weeks":         totalWeeks,
		"days_since_last":     0,
	}

	if lastWorkout != "" {
		last, _ := time.Parse("2006-01-02", lastWorkout)
		daysSince := int(endDate.Sub(last).Hours() / 24)
		metricsData["days_since_last"] = daysSince
	}

	return &PatternAnalysisResult{
		AnalysisType:    "consistency",
		LookbackDays:    lookbackDays,
		Summary:         summary,
		Insights:        insights,
		Recommendations: recommendations,
		MetricsData:     metricsData,
		Score:           &consistencyScore,
		UserID:          userID,
	}, nil
}

// analyzeProgressiveOverload analyzes progressive overload patterns.
func (t *WorkoutPatternTool) analyzeProgressiveOverload(ctx context.Context, userID int, startDate, endDate time.Time, lookbackDays int) (*PatternAnalysisResult, error) {
	// Query progression data for major exercises
	query := fmt.Sprintf(`
		SELECT e.name,
		       es.workout_date,
		       MAX(es.weight_kg) as max_weight,
		       AVG(es.weight_kg) as avg_weight
		FROM exercise_sets es
		JOIN exercises e ON es.exercise_id = e.id
		WHERE es.workout_user_id = %d
		AND es.workout_date BETWEEN '%s' AND '%s'
		AND es.weight_kg IS NOT NULL
		AND es.completed_reps IS NOT NULL
		AND e.exercise_type = 'weighted'
		GROUP BY e.name, es.workout_date
		ORDER BY e.name, es.workout_date`,
		userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query progression data: %w", err)
	}

	// Group data by exercise
	exerciseData := make(map[string][]map[string]interface{})
	for _, row := range result.Rows {
		if len(row) >= 4 {
			exerciseName := row[0].(string)
			workoutDate := row[1].(string)
			maxWeight := row[2]
			avgWeight := row[3]

			if exerciseData[exerciseName] == nil {
				exerciseData[exerciseName] = []map[string]interface{}{}
			}

			exerciseData[exerciseName] = append(exerciseData[exerciseName], map[string]interface{}{
				"date":       workoutDate,
				"max_weight": maxWeight,
				"avg_weight": avgWeight,
			})
		}
	}

	// Analyze progression for each exercise
	progressingExercises := 0
	stagnantExercises := 0
	decreasingExercises := 0
	insights := []string{}
	recommendations := []string{}
	progressionData := make(map[string]interface{})

	if len(exerciseData) == 0 {
		insights = append(insights, "No weighted exercise data found in the analysis period.")
		recommendations = append(recommendations, "Start tracking weights to monitor progressive overload.")
		recommendations = append(recommendations, "Focus on gradually increasing weight, reps, or sets over time.")
	} else {
		for exerciseName, data := range exerciseData {
			if len(data) >= 2 {
				// Calculate progression rate
				firstSession := data[0]
				lastSession := data[len(data)-1]

				firstWeight, _ := firstSession["max_weight"].(float64)
				lastWeight, _ := lastSession["max_weight"].(float64)

				firstDate, _ := time.Parse("2006-01-02", firstSession["date"].(string))
				lastDate, _ := time.Parse("2006-01-02", lastSession["date"].(string))

				daysDiff := lastDate.Sub(firstDate).Hours() / 24
				if daysDiff > 0 {
					progressionRate := (lastWeight - firstWeight) / daysDiff * 7 // Per week

					progressionData[exerciseName] = map[string]interface{}{
						"first_weight":     firstWeight,
						"last_weight":      lastWeight,
						"progression_rate": progressionRate,
						"total_sessions":   len(data),
					}

					if progressionRate > 0.5 {
						progressingExercises++
					} else if progressionRate > -0.5 {
						stagnantExercises++
					} else {
						decreasingExercises++
					}
				}
			}
		}

		totalExercises := progressingExercises + stagnantExercises + decreasingExercises

		insights = append(insights, fmt.Sprintf("Analyzed %d exercises for progressive overload patterns.", totalExercises))
		insights = append(insights, fmt.Sprintf("%d exercises showing progression, %d stagnant, %d decreasing.", progressingExercises, stagnantExercises, decreasingExercises))

		if progressingExercises > stagnantExercises+decreasingExercises {
			insights = append(insights, "Good overall progression in most exercises!")
			recommendations = append(recommendations, "Continue current progression strategy.")
		} else {
			insights = append(insights, "Many exercises showing plateau or decline.")
			recommendations = append(recommendations, "Consider deload weeks followed by small weight increases.")
			recommendations = append(recommendations, "Try different rep ranges or exercise variations.")
		}

		if stagnantExercises > 0 {
			recommendations = append(recommendations, "For stagnant exercises, try increasing reps before adding weight.")
		}
	}

	summary := fmt.Sprintf("Progressive overload analysis shows %d progressing, %d stagnant, and %d decreasing exercises over %d days.",
		progressingExercises, stagnantExercises, decreasingExercises, lookbackDays)

	progressionScore := 0.0
	if progressingExercises+stagnantExercises+decreasingExercises > 0 {
		progressionScore = float64(progressingExercises) / float64(progressingExercises+stagnantExercises+decreasingExercises) * 100
	}

	metricsData := map[string]interface{}{
		"progression_rate":      progressionScore,
		"progressing_exercises": progressingExercises,
		"stagnant_exercises":    stagnantExercises,
		"decreasing_exercises":  decreasingExercises,
		"exercise_data":         progressionData,
	}

	return &PatternAnalysisResult{
		AnalysisType:    "progressive_overload",
		LookbackDays:    lookbackDays,
		Summary:         summary,
		Insights:        insights,
		Recommendations: recommendations,
		MetricsData:     metricsData,
		Score:           &progressionScore,
		UserID:          userID,
	}, nil
}

// analyzeMuscleBalance analyzes muscle group balance in training.
func (t *WorkoutPatternTool) analyzeMuscleBalance(ctx context.Context, userID int, startDate, endDate time.Time, lookbackDays int) (*PatternAnalysisResult, error) {
	// Query muscle group distribution
	query := fmt.Sprintf(`
		SELECT mg.name as muscle_group,
		       COUNT(*) as exercise_count,
		       COUNT(DISTINCT we.workout_date) as session_count
		FROM workout_exercise we
		JOIN exercise_muscle_groups emg ON we.exercise_id = emg.exercise_id
		JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
		WHERE we.workout_user_id = %d
		AND we.workout_date BETWEEN '%s' AND '%s'
		AND emg.is_primary = 1
		GROUP BY mg.name
		ORDER BY exercise_count DESC`,
		userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query muscle group distribution: %w", err)
	}

	muscleData := make(map[string]map[string]interface{})
	totalExercises := 0

	for _, row := range result.Rows {
		if len(row) >= 3 {
			muscleGroup := row[0].(string)
			exerciseCount, _ := row[1].(int64)
			sessionCount, _ := row[2].(int64)

			muscleData[muscleGroup] = map[string]interface{}{
				"exercise_count": exerciseCount,
				"session_count":  sessionCount,
			}
			totalExercises += int(exerciseCount)
		}
	}

	insights := []string{}
	recommendations := []string{}

	if totalExercises == 0 {
		insights = append(insights, "No exercise data found for muscle group analysis.")
		recommendations = append(recommendations, "Start tracking exercises with muscle group information.")
		recommendations = append(recommendations, "Aim for balanced training across all major muscle groups.")
	} else {
		// Calculate percentages and identify imbalances
		musclePercentages := make(map[string]float64)
		for muscle, data := range muscleData {
			count := data["exercise_count"].(int64)
			percentage := float64(count) / float64(totalExercises) * 100
			musclePercentages[muscle] = percentage
		}

		insights = append(insights, fmt.Sprintf("Analyzed %d exercises across %d muscle groups.", totalExercises, len(muscleData)))

		// Find highest and lowest trained muscle groups
		var maxMuscle, minMuscle string
		var maxPercentage, minPercentage float64 = 0, 100

		for muscle, percentage := range musclePercentages {
			if percentage > maxPercentage {
				maxMuscle = muscle
				maxPercentage = percentage
			}
			if percentage < minPercentage {
				minMuscle = muscle
				minPercentage = percentage
			}
		}

		if maxMuscle != "" && minMuscle != "" {
			insights = append(insights, fmt.Sprintf("Most trained: %s (%.1f%%), Least trained: %s (%.1f%%).", maxMuscle, maxPercentage, minMuscle, minPercentage))

			// Check for significant imbalances
			imbalanceRatio := maxPercentage / minPercentage
			if imbalanceRatio > 3 {
				insights = append(insights, "Significant muscle group imbalance detected.")
				recommendations = append(recommendations, fmt.Sprintf("Increase training frequency for %s to improve balance.", minMuscle))
				recommendations = append(recommendations, "Consider reducing volume for overdeveloped muscle groups.")
			} else if imbalanceRatio > 2 {
				insights = append(insights, "Moderate muscle group imbalance present.")
				recommendations = append(recommendations, fmt.Sprintf("Add more exercises targeting %s.", minMuscle))
			} else {
				insights = append(insights, "Good overall muscle group balance.")
				recommendations = append(recommendations, "Maintain current balance while focusing on progressive overload.")
			}
		}

		// Check for missing major muscle groups
		majorMuscleGroups := []string{"Chest", "Back", "Shoulders", "Quads", "Hamstrings", "Glutes"}
		missingGroups := []string{}
		for _, group := range majorMuscleGroups {
			if _, exists := muscleData[group]; !exists {
				missingGroups = append(missingGroups, group)
			}
		}

		if len(missingGroups) > 0 {
			insights = append(insights, fmt.Sprintf("Missing training for: %s.", strings.Join(missingGroups, ", ")))
			recommendations = append(recommendations, "Add exercises targeting all major muscle groups.")
		}
	}

	// Calculate balance score
	balanceScore := 0.0
	if len(muscleData) > 0 {
		// Calculate coefficient of variation (lower is better for balance)
		var values []float64
		for _, data := range muscleData {
			count := data["exercise_count"].(int64)
			values = append(values, float64(count))
		}

		if len(values) > 1 {
			mean := 0.0
			for _, v := range values {
				mean += v
			}
			mean /= float64(len(values))

			variance := 0.0
			for _, v := range values {
				variance += (v - mean) * (v - mean)
			}
			variance /= float64(len(values))

			stdDev := math.Sqrt(variance)
			cv := stdDev / mean

			// Convert CV to balance score (0-100, higher is better)
			balanceScore = math.Max(0, 100-cv*50)
		}
	}

	summary := fmt.Sprintf("Muscle balance analysis shows training distributed across %d muscle groups with %.1f balance score.",
		len(muscleData), balanceScore)

	metricsData := map[string]interface{}{
		"muscle_distribution": muscleData,
		"total_exercises":     totalExercises,
		"muscle_groups_count": len(muscleData),
		"balance_score":       balanceScore,
	}

	return &PatternAnalysisResult{
		AnalysisType:    "muscle_balance",
		LookbackDays:    lookbackDays,
		Summary:         summary,
		Insights:        insights,
		Recommendations: recommendations,
		MetricsData:     metricsData,
		Score:           &balanceScore,
		UserID:          userID,
	}, nil
}

// analyzeRecoveryTime analyzes rest periods between muscle group training.
func (t *WorkoutPatternTool) analyzeRecoveryTime(ctx context.Context, userID int, startDate, endDate time.Time, lookbackDays int) (*PatternAnalysisResult, error) {
	// Query workout dates with muscle groups
	query := fmt.Sprintf(`
		SELECT DISTINCT we.workout_date, mg.name as muscle_group
		FROM workout_exercise we
		JOIN exercise_muscle_groups emg ON we.exercise_id = emg.exercise_id
		JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
		WHERE we.workout_user_id = %d
		AND we.workout_date BETWEEN '%s' AND '%s'
		AND emg.is_primary = 1
		ORDER BY mg.name, we.workout_date`,
		userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query recovery data: %w", err)
	}

	// Group by muscle group and calculate recovery times
	muscleWorkouts := make(map[string][]time.Time)
	for _, row := range result.Rows {
		if len(row) >= 2 {
			workoutDate := row[0].(string)
			muscleGroup := row[1].(string)

			date, parseErr := time.Parse("2006-01-02", workoutDate)
			if parseErr == nil {
				muscleWorkouts[muscleGroup] = append(muscleWorkouts[muscleGroup], date)
			}
		}
	}

	insights := []string{}
	recommendations := []string{}
	recoveryData := make(map[string]interface{})

	if len(muscleWorkouts) == 0 {
		insights = append(insights, "No muscle group workout data found for recovery analysis.")
		recommendations = append(recommendations, "Track exercises with muscle group information to analyze recovery patterns.")
		recommendations = append(recommendations, "Generally allow 48-72 hours recovery between training the same muscle groups.")
	} else {
		totalRecoveryTimes := []float64{}
		muscleRecoveryStats := make(map[string]map[string]interface{})

		for muscle, workouts := range muscleWorkouts {
			if len(workouts) >= 2 {
				recoveryTimes := []float64{}

				// Sort workout dates
				for i := 0; i < len(workouts)-1; i++ {
					for j := i + 1; j < len(workouts); j++ {
						if workouts[i].After(workouts[j]) {
							workouts[i], workouts[j] = workouts[j], workouts[i]
						}
					}
				}

				// Calculate recovery times between consecutive workouts
				for i := 1; i < len(workouts); i++ {
					recoveryHours := workouts[i].Sub(workouts[i-1]).Hours()
					recoveryDays := recoveryHours / 24
					recoveryTimes = append(recoveryTimes, recoveryDays)
					totalRecoveryTimes = append(totalRecoveryTimes, recoveryDays)
				}

				if len(recoveryTimes) > 0 {
					// Calculate average recovery time for this muscle group
					avgRecovery := 0.0
					minRecovery := recoveryTimes[0]
					maxRecovery := recoveryTimes[0]

					for _, recovery := range recoveryTimes {
						avgRecovery += recovery
						if recovery < minRecovery {
							minRecovery = recovery
						}
						if recovery > maxRecovery {
							maxRecovery = recovery
						}
					}
					avgRecovery /= float64(len(recoveryTimes))

					muscleRecoveryStats[muscle] = map[string]interface{}{
						"average_recovery_days": avgRecovery,
						"min_recovery_days":     minRecovery,
						"max_recovery_days":     maxRecovery,
						"workout_count":         len(workouts),
					}
				}
			}
		}

		// Calculate overall recovery statistics
		if len(totalRecoveryTimes) > 0 {
			avgOverallRecovery := 0.0
			for _, recovery := range totalRecoveryTimes {
				avgOverallRecovery += recovery
			}
			avgOverallRecovery /= float64(len(totalRecoveryTimes))

			insights = append(insights, fmt.Sprintf("Analyzed recovery patterns for %d muscle groups.", len(muscleRecoveryStats)))
			insights = append(insights, fmt.Sprintf("Average recovery time between muscle group training: %.1f days.", avgOverallRecovery))

			// Analyze recovery patterns
			shortRecoveries := 0
			optimalRecoveries := 0
			longRecoveries := 0

			for _, recovery := range totalRecoveryTimes {
				if recovery < 1 {
					shortRecoveries++
				} else if recovery <= 3 {
					optimalRecoveries++
				} else {
					longRecoveries++
				}
			}

			insights = append(insights, fmt.Sprintf("Recovery distribution: %d short (<1 day), %d optimal (1-3 days), %d long (>3 days).", shortRecoveries, optimalRecoveries, longRecoveries))

			if shortRecoveries > optimalRecoveries {
				insights = append(insights, "Many short recovery periods detected - possible overtraining risk.")
				recommendations = append(recommendations, "Allow at least 48 hours between training the same muscle groups.")
				recommendations = append(recommendations, "Consider spacing out workouts more or alternating muscle groups.")
			} else if longRecoveries > optimalRecoveries {
				insights = append(insights, "Many long recovery periods - muscle groups may be undertrained.")
				recommendations = append(recommendations, "Increase training frequency for better muscle development.")
			} else {
				insights = append(insights, "Good overall recovery pattern.")
				recommendations = append(recommendations, "Maintain current recovery schedule while monitoring for fatigue.")
			}

			// Calculate recovery score
			recoveryScore := float64(optimalRecoveries) / float64(len(totalRecoveryTimes)) * 100

			recoveryData = map[string]interface{}{
				"average_recovery_time": avgOverallRecovery,
				"muscle_recovery_stats": muscleRecoveryStats,
				"short_recoveries":      shortRecoveries,
				"optimal_recoveries":    optimalRecoveries,
				"long_recoveries":       longRecoveries,
				"recovery_score":        recoveryScore,
			}
		}
	}

	summary := fmt.Sprintf("Recovery time analysis over %d days shows patterns for %d muscle groups.",
		lookbackDays, len(muscleWorkouts))

	var recoveryScore *float64
	if data, exists := recoveryData["recovery_score"]; exists {
		if score, ok := data.(float64); ok {
			recoveryScore = &score
		}
	}

	return &PatternAnalysisResult{
		AnalysisType:    "recovery_time",
		LookbackDays:    lookbackDays,
		Summary:         summary,
		Insights:        insights,
		Recommendations: recommendations,
		MetricsData:     recoveryData,
		Score:           recoveryScore,
		UserID:          userID,
	}, nil
}

// analyzePlateauDetection detects performance plateaus in exercises.
func (t *WorkoutPatternTool) analyzePlateauDetection(ctx context.Context, userID int, startDate, endDate time.Time, lookbackDays int) (*PatternAnalysisResult, error) {
	// Query exercise performance over time
	query := fmt.Sprintf(`
		SELECT e.name,
		       es.workout_date,
		       MAX(es.weight_kg) as max_weight,
		       MAX(es.completed_reps) as max_reps,
		       (MAX(es.weight_kg) * MAX(es.completed_reps)) as max_volume
		FROM exercise_sets es
		JOIN exercises e ON es.exercise_id = e.id
		WHERE es.workout_user_id = %d
		AND es.workout_date BETWEEN '%s' AND '%s'
		AND es.weight_kg IS NOT NULL
		AND es.completed_reps IS NOT NULL
		AND e.exercise_type = 'weighted'
		GROUP BY e.name, es.workout_date
		HAVING COUNT(*) >= 1
		ORDER BY e.name, es.workout_date`,
		userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query plateau data: %w", err)
	}

	// Group by exercise and analyze trends
	exerciseData := make(map[string][]map[string]interface{})
	for _, row := range result.Rows {
		if len(row) >= 5 {
			exerciseName := row[0].(string)
			workoutDate := row[1].(string)
			maxWeight := row[2]
			maxReps := row[3]
			maxVolume := row[4]

			if exerciseData[exerciseName] == nil {
				exerciseData[exerciseName] = []map[string]interface{}{}
			}

			exerciseData[exerciseName] = append(exerciseData[exerciseName], map[string]interface{}{
				"date":       workoutDate,
				"max_weight": maxWeight,
				"max_reps":   maxReps,
				"max_volume": maxVolume,
			})
		}
	}

	insights := []string{}
	recommendations := []string{}
	plateauData := make(map[string]interface{})
	detectedPlateaus := []string{}

	if len(exerciseData) == 0 {
		insights = append(insights, "No weighted exercise data found for plateau detection.")
		recommendations = append(recommendations, "Track weights and reps consistently to detect plateaus.")
		recommendations = append(recommendations, "Monitor progress over several weeks to identify stagnation.")
	} else {
		plateauCount := 0
		progressingCount := 0

		for exerciseName, sessions := range exerciseData {
			if len(sessions) >= 4 { // Need at least 4 sessions for plateau detection
				// Analyze recent performance vs earlier performance
				mid := len(sessions) / 2
				earlierSessions := sessions[:mid]
				recentSessions := sessions[mid:]

				// Calculate average performance for each period
				earlierAvgWeight := 0.0
				recentAvgWeight := 0.0

				for _, session := range earlierSessions {
					if weight, ok := session["max_weight"].(float64); ok {
						earlierAvgWeight += weight
					}
				}
				earlierAvgWeight /= float64(len(earlierSessions))

				for _, session := range recentSessions {
					if weight, ok := session["max_weight"].(float64); ok {
						recentAvgWeight += weight
					}
				}
				recentAvgWeight /= float64(len(recentSessions))

				// Detect plateau (less than 2.5% improvement)
				improvementPercentage := ((recentAvgWeight - earlierAvgWeight) / earlierAvgWeight) * 100

				if improvementPercentage < 2.5 && improvementPercentage > -2.5 {
					plateauCount++
					detectedPlateaus = append(detectedPlateaus, exerciseName)
					plateauData[exerciseName] = map[string]interface{}{
						"status":                 "plateau",
						"earlier_avg_weight":     earlierAvgWeight,
						"recent_avg_weight":      recentAvgWeight,
						"improvement_percentage": improvementPercentage,
						"sessions_analyzed":      len(sessions),
					}
				} else {
					progressingCount++
					plateauData[exerciseName] = map[string]interface{}{
						"status":                 "progressing",
						"earlier_avg_weight":     earlierAvgWeight,
						"recent_avg_weight":      recentAvgWeight,
						"improvement_percentage": improvementPercentage,
						"sessions_analyzed":      len(sessions),
					}
				}
			}
		}

		totalAnalyzed := plateauCount + progressingCount
		insights = append(insights, fmt.Sprintf("Analyzed %d exercises for plateau patterns.", totalAnalyzed))

		if plateauCount > 0 {
			insights = append(insights, fmt.Sprintf("Detected plateaus in %d exercises: %s.", plateauCount, strings.Join(detectedPlateaus, ", ")))
			recommendations = append(recommendations, "For plateaued exercises, try deloading to 90% of current weight for 1-2 weeks.")
			recommendations = append(recommendations, "Consider changing rep ranges or exercise variations.")
			recommendations = append(recommendations, "Ensure adequate nutrition and recovery for continued progress.")
		}

		if progressingCount > 0 {
			insights = append(insights, fmt.Sprintf("%d exercises showing continued progression.", progressingCount))
			recommendations = append(recommendations, "Maintain current progression strategy for advancing exercises.")
		}

		if plateauCount == 0 && progressingCount > 0 {
			insights = append(insights, "No plateaus detected - good consistent progress!")
			recommendations = append(recommendations, "Continue current training approach.")
		}

		// Calculate plateau score (lower is better)
		plateauScore := 0.0
		if totalAnalyzed > 0 {
			plateauScore = float64(progressingCount) / float64(totalAnalyzed) * 100
		}

		plateauData["detected_plateaus"] = detectedPlateaus
		plateauData["plateau_count"] = plateauCount
		plateauData["progressing_count"] = progressingCount
		plateauData["plateau_score"] = plateauScore
	}

	summary := fmt.Sprintf("Plateau detection analysis found %d plateaued exercises out of %d analyzed over %d days.",
		len(detectedPlateaus), len(exerciseData), lookbackDays)

	var plateauScore *float64
	if data, exists := plateauData["plateau_score"]; exists {
		if score, ok := data.(float64); ok {
			plateauScore = &score
		}
	}

	return &PatternAnalysisResult{
		AnalysisType:    "plateau_detection",
		LookbackDays:    lookbackDays,
		Summary:         summary,
		Insights:        insights,
		Recommendations: recommendations,
		MetricsData:     plateauData,
		Score:           plateauScore,
		UserID:          userID,
	}, nil
}

// analyzeWorkoutVariety analyzes variety in exercise selection.
func (t *WorkoutPatternTool) analyzeWorkoutVariety(ctx context.Context, userID int, startDate, endDate time.Time, lookbackDays int) (*PatternAnalysisResult, error) {
	// Query exercise variety data
	query := fmt.Sprintf(`
		SELECT e.name,
		       e.category,
		       e.exercise_type,
		       COUNT(DISTINCT we.workout_date) as session_count,
		       COUNT(*) as total_occurrences
		FROM workout_exercise we
		JOIN exercises e ON we.exercise_id = e.id
		WHERE we.workout_user_id = %d
		AND we.workout_date BETWEEN '%s' AND '%s'
		GROUP BY e.name, e.category, e.exercise_type
		ORDER BY session_count DESC`,
		userID, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	result, err := t.secureQueryTool.ExecuteQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query variety data: %w", err)
	}

	exercises := []map[string]interface{}{}
	categories := make(map[string]int)
	exerciseTypes := make(map[string]int)
	totalSessions := 0

	for _, row := range result.Rows {
		if len(row) >= 5 {
			exerciseName := row[0].(string)
			category := row[1].(string)
			exerciseType := row[2].(string)
			sessionCount, _ := row[3].(int64)
			totalOccurrences, _ := row[4].(int64)

			exercises = append(exercises, map[string]interface{}{
				"name":              exerciseName,
				"category":          category,
				"exercise_type":     exerciseType,
				"session_count":     sessionCount,
				"total_occurrences": totalOccurrences,
			})

			categories[category]++
			exerciseTypes[exerciseType]++
			if int(sessionCount) > totalSessions {
				totalSessions = int(sessionCount)
			}
		}
	}

	insights := []string{}
	recommendations := []string{}

	if len(exercises) == 0 {
		insights = append(insights, "No exercise data found for variety analysis.")
		recommendations = append(recommendations, "Start incorporating a variety of exercises into your routine.")
		recommendations = append(recommendations, "Mix compound and isolation exercises for balanced development.")
	} else {
		uniqueExercises := len(exercises)
		insights = append(insights, fmt.Sprintf("Performed %d unique exercises over %d days.", uniqueExercises, lookbackDays))

		// Analyze category distribution
		categoryCount := len(categories)
		insights = append(insights, fmt.Sprintf("Exercise categories used: %d (Upper: %d, Lower: %d, Full Body: %d).",
			categoryCount,
			categories["upper"],
			categories["lower"],
			categories["full_body"]))

		// Analyze exercise type distribution
		weightedCount := exerciseTypes["weighted"]
		bodyweightCount := exerciseTypes["bodyweight"]
		insights = append(insights, fmt.Sprintf("Exercise types: %d weighted, %d bodyweight exercises.", weightedCount, bodyweightCount))

		// Calculate variety score
		varietyScore := 0.0

		// Factor 1: Number of unique exercises relative to workout frequency
		if totalSessions > 0 {
			exerciseToSessionRatio := float64(uniqueExercises) / float64(totalSessions)
			varietyScore += math.Min(25, exerciseToSessionRatio*100) // Max 25 points
		}

		// Factor 2: Category balance
		if categoryCount >= 2 {
			varietyScore += 25 // 25 points for multiple categories
		}

		// Factor 3: Exercise type variety
		if weightedCount > 0 && bodyweightCount > 0 {
			varietyScore += 25 // 25 points for mixed exercise types
		}

		// Factor 4: Even distribution (avoid overuse of single exercises)
		overusedExercises := 0
		underusedExercises := 0

		for _, exercise := range exercises {
			sessionCount := exercise["session_count"].(int64)
			percentage := float64(sessionCount) / float64(totalSessions) * 100

			if percentage > 80 {
				overusedExercises++
			} else if percentage < 20 && totalSessions >= 5 {
				underusedExercises++
			}
		}

		if overusedExercises == 0 {
			varietyScore += 25 // 25 points for no overused exercises
		}

		// Generate insights based on variety patterns
		if varietyScore >= 80 {
			insights = append(insights, "Excellent exercise variety!")
			recommendations = append(recommendations, "Maintain current variety while focusing on progressive overload.")
		} else if varietyScore >= 60 {
			insights = append(insights, "Good exercise variety with room for improvement.")
		} else {
			insights = append(insights, "Limited exercise variety detected.")
		}

		if categoryCount < 2 {
			recommendations = append(recommendations, "Add exercises from different categories (upper, lower, full body).")
		}

		if weightedCount == 0 {
			recommendations = append(recommendations, "Consider adding weighted exercises for strength development.")
		} else if bodyweightCount == 0 {
			recommendations = append(recommendations, "Add bodyweight exercises for functional movement patterns.")
		}

		if overusedExercises > 0 {
			recommendations = append(recommendations, "Reduce frequency of overused exercises and add alternatives.")
		}

		if uniqueExercises < 5 && totalSessions >= 10 {
			recommendations = append(recommendations, "Expand exercise selection to include more movement patterns.")
		}
	}

	summary := fmt.Sprintf("Workout variety analysis shows %d unique exercises across %d categories with %.1f variety score over %d days.",
		len(exercises), len(categories), 0.0, lookbackDays)

	// Update summary with actual variety score
	varietyScore := 0.0
	if len(exercises) > 0 {
		// Recalculate for summary
		if totalSessions > 0 {
			exerciseToSessionRatio := float64(len(exercises)) / float64(totalSessions)
			varietyScore += math.Min(25, exerciseToSessionRatio*100)
		}
		if len(categories) >= 2 {
			varietyScore += 25
		}
		if exerciseTypes["weighted"] > 0 && exerciseTypes["bodyweight"] > 0 {
			varietyScore += 25
		}
		varietyScore += 25 // Assume good distribution for summary
	}

	summary = fmt.Sprintf("Workout variety analysis shows %d unique exercises across %d categories with %.1f variety score over %d days.",
		len(exercises), len(categories), varietyScore, lookbackDays)

	metricsData := map[string]interface{}{
		"exercise_variety_score":     varietyScore,
		"unique_exercises":           len(exercises),
		"category_distribution":      categories,
		"exercise_type_distribution": exerciseTypes,
		"exercise_details":           exercises,
		"total_sessions":             totalSessions,
	}

	return &PatternAnalysisResult{
		AnalysisType:    "workout_variety",
		LookbackDays:    lookbackDays,
		Summary:         summary,
		Insights:        insights,
		Recommendations: recommendations,
		MetricsData:     metricsData,
		Score:           &varietyScore,
		UserID:          userID,
	}, nil
}

// ToOpenAIFunction returns the OpenAI function definition for analyze_workout_pattern.
func (t *WorkoutPatternTool) ToOpenAIFunction() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "analyze_workout_pattern",
			"description": "Analyze patterns in user's workout history",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"analysis_type": map[string]interface{}{
						"type": "string",
						"enum": []string{
							"consistency",
							"progressive_overload",
							"muscle_balance",
							"recovery_time",
							"plateau_detection",
							"workout_variety",
						},
						"description": "Type of pattern analysis to perform",
					},
					"lookback_days": map[string]interface{}{
						"type":        "integer",
						"description": "Number of days to look back for analysis",
						"default":     30,
						"minimum":     7,
						"maximum":     365,
					},
				},
				"required": []string{"analysis_type"},
			},
		},
	}
}

// ExecuteFunction executes the analyze_workout_pattern function with the given parameters.
// This method is compatible with OpenAI function calling.
func (t *WorkoutPatternTool) ExecuteFunction(ctx context.Context, functionName string, argumentsJSON string) (string, error) {
	if functionName != "analyze_workout_pattern" {
		return "", fmt.Errorf("unsupported function: %s", functionName)
	}

	var params PatternAnalysisParams
	if err := json.Unmarshal([]byte(argumentsJSON), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Set default lookback days if not provided
	if params.LookbackDays == 0 {
		params.LookbackDays = 30
	}

	result, err := t.AnalyzePattern(ctx, params)
	if err != nil {
		return "", err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}
