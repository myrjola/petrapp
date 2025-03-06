package workout

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"log/slog"
	"math"
	"time"
)

// Service handles the business logic for workout management.
type Service struct {
	repo   *sqliteRepository
	logger *slog.Logger
}

// NewService creates a new workout service with SQLite repository.
func NewService(db *sqlite.Database, logger *slog.Logger) *Service {
	return &Service{
		repo:   newSQLiteRepository(db, logger),
		logger: logger,
	}
}

// GetUserPreferences retrieves the workout preferences for a user.
func (s *Service) GetUserPreferences(ctx context.Context) (Preferences, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	prefs, err := s.repo.getUserPreferences(ctx, userID)
	if err != nil {
		return Preferences{}, fmt.Errorf("get user preferences: %w", err)
	}
	return prefs, nil
}

// SaveUserPreferences saves the workout preferences for a user.
func (s *Service) SaveUserPreferences(ctx context.Context, prefs Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if err := s.repo.saveUserPreferences(ctx, userID, prefs); err != nil {
		return fmt.Errorf("save user preferences: %w", err)
	}
	return nil
}

// generateWorkout creates a new workout plan based on user preferences and history.
func (s *Service) generateWorkout(ctx context.Context, date time.Time) (Session, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	s.logger.LogAttrs(ctx, slog.LevelInfo, "generating workout",
		slog.Time("date", date),
		slog.String("user_id", hex.EncodeToString(userID)))

	// Step 1: Determine workout type (upper/lower/full body)
	category, err := s.determineWorkoutType(ctx, date)
	if err != nil {
		return Session{}, fmt.Errorf("determine workout type: %w", err)
	}

	s.logger.LogAttrs(ctx, slog.LevelDebug, "workout type determined",
		slog.String("category", string(category)))

	// Step 2: Select appropriate exercises with balanced muscle groups
	plannedExercises, err := s.selectExercisesForWorkout(ctx, category, date)
	if err != nil {
		return Session{}, fmt.Errorf("select exercises: %w", err)
	}

	s.logger.LogAttrs(ctx, slog.LevelDebug, "exercise selection complete",
		slog.Int("exercise_count", len(plannedExercises)))

	// Step 3: Convert to Session format with ExerciseSets
	var exerciseSets = make([]ExerciseSet, len(plannedExercises))
	for i, pe := range plannedExercises {
		var sets []Set
		for _, ps := range pe.sets {
			sets = append(sets, Set{
				WeightKg:      ps.weightKg,
				MinReps:       ps.targetMinReps,
				MaxReps:       ps.targetMaxReps,
				CompletedReps: nil, // Not completed yet
			})
		}

		exerciseSets[i] = ExerciseSet{
			Exercise: pe.exercise,
			Sets:     sets,
		}
	}

	// Step 4: Create the complete Session object
	session := Session{
		WorkoutDate:      date,
		DifficultyRating: nil, // Not rated yet
		StartedAt:        nil, // Not started yet
		CompletedAt:      nil, // Not completed yet
		ExerciseSets:     exerciseSets,
		Status:           StatusPlanned,
	}

	// Log information about the generated workout
	exerciseNames := make([]string, len(exerciseSets))
	for i, es := range exerciseSets {
		exerciseNames[i] = es.Exercise.Name
	}

	s.logger.LogAttrs(ctx, slog.LevelInfo, "workout generated successfully",
		slog.Time("date", date),
		slog.String("category", string(category)),
		slog.Any("exercises", exerciseNames))

	return session, nil
}

// ResolveWeeklySchedule retrieves the workout schedule for a week.
func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]Session, error) {
	//nolint:godox // temporary todo
	// TODO: Implement weekly schedule retrieval
	// This should:
	// 1. Get all sessions for the week
	// 2. Fill in rest days and planned workouts based on preferences
	// 3. Return complete 7-day schedule
	workouts := make([]Session, 7) //nolint:mnd // 7 days in a week

	// Get the current date
	now := time.Now()

	// Calculate the current week's Monday
	// Weekday() returns the day of the week with Sunday as 0
	// We need to adjust this to 1-based with Monday as 1
	offset := int(time.Monday - now.Weekday())
	if offset > 0 {
		offset = -6 //nolint:mnd // If today is Sunday, adjust the offset to get last Monday
	}
	monday := now.AddDate(0, 0, offset)

	// Generate dates from Monday to Sunday
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		workout, err := s.generateWorkout(ctx, day)
		if err != nil {
			return nil, fmt.Errorf("generate workout: %w", err)
		}
		workouts[i] = workout
	}

	return workouts, nil
}

// GetSession retrieves a workout session for a specific date.
func (s *Service) GetSession(ctx context.Context, date time.Time) (Session, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	session, err := s.repo.getSession(ctx, userID, date)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// If no session exists, generate a new one
			return s.generateWorkout(ctx, date)
		}
		return Session{}, fmt.Errorf("get session: %w", err)
	}

	return session, nil
}

// StartSession starts a new workout session or returns an error if one already exists.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	if err := s.repo.startSession(ctx, userID, date); err != nil {
		return fmt.Errorf("start session: %w", err)
	}

	// Generate workout if it doesn't exist
	session, err := s.generateWorkout(ctx, date)
	if err != nil {
		return fmt.Errorf("generate workout: %w", err)
	}

	// Save the generated exercise sets to the database
	if err = s.repo.saveExerciseSets(ctx, userID, date, session.ExerciseSets); err != nil {
		return fmt.Errorf("save exercise sets: %w", err)
	}

	return nil
}

// CompleteSession marks a workout session as completed.
func (s *Service) CompleteSession(ctx context.Context, date time.Time) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if err := s.repo.completeSession(ctx, userID, date); err != nil {
		return fmt.Errorf("complete session: %w", err)
	}
	return nil
}

// SaveFeedback saves the difficulty rating for a completed workout session.
func (s *Service) SaveFeedback(ctx context.Context, date time.Time, difficulty int) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if err := s.repo.saveFeedback(ctx, userID, date, difficulty); err != nil {
		return fmt.Errorf("save feedback: %w", err)
	}
	return nil
}

// UpdateSetWeight updates the weight for a specific set in a workout.
func (s *Service) UpdateSetWeight(
	ctx context.Context,
	date time.Time,
	exerciseID int,
	setIndex int,
	newWeight float64,
) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if err := s.repo.updateSetWeight(ctx, userID, date, exerciseID, setIndex, newWeight); err != nil {
		return fmt.Errorf("set weight: %w", err)
	}
	return nil
}

// UpdateCompletedReps updates a previously completed set with new rep count.
func (s *Service) UpdateCompletedReps(
	ctx context.Context,
	date time.Time,
	exerciseID int,
	setIndex int,
	completedReps int,
) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if err := s.repo.updateCompletedReps(ctx, userID, date, exerciseID, setIndex, completedReps); err != nil {
		return fmt.Errorf("update completed reps: %w", err)
	}
	return nil
}

// determineWorkoutType decides what type of workout to generate based on user preferences and training history.
func (s *Service) determineWorkoutType(ctx context.Context, date time.Time) (Category, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	// 1. Check if tomorrow is a preferred workout day.
	tomorrow := date.AddDate(0, 0, 1)
	tomorrowWeekday := tomorrow.Weekday()

	preferences, err := s.repo.getUserPreferences(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("get user preferences: %w", err)
	}

	isTomorrowWorkoutDay := false
	switch tomorrowWeekday {
	case time.Monday:
		isTomorrowWorkoutDay = preferences.Monday
	case time.Tuesday:
		isTomorrowWorkoutDay = preferences.Tuesday
	case time.Wednesday:
		isTomorrowWorkoutDay = preferences.Wednesday
	case time.Thursday:
		isTomorrowWorkoutDay = preferences.Thursday
	case time.Friday:
		isTomorrowWorkoutDay = preferences.Friday
	case time.Saturday:
		isTomorrowWorkoutDay = preferences.Saturday
	case time.Sunday:
		isTomorrowWorkoutDay = preferences.Sunday
	}

	// Start with lower body workout.
	if isTomorrowWorkoutDay {
		return CategoryLower, nil
	}

	yesterday := date.AddDate(0, 0, -1)
	_, err = s.repo.getSession(ctx, userID, yesterday)
	if errors.Is(err, sql.ErrNoRows) {
		// If no session exists, default to full body workout
		return CategoryFullBody, nil
	}

	// End with upper body workout with the assumption that yesterday was lower body.
	return CategoryUpper, nil
}

// selectExercisesForWorkout chooses appropriate exercises with balanced muscle groups.
func (s *Service) selectExercisesForWorkout(
	ctx context.Context,
	category Category,
	date time.Time,
) ([]plannedExercise, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	// 1. Get exercise target count for this category
	targetCount := s.getTargetExerciseCount(category)

	// 2. Get candidate exercises for this category
	candidateExercises, err := s.getCandidateExercises(ctx, category)
	if err != nil {
		return nil, err
	}

	// 3. Categorize exercises by recency
	recentList, nonRecentList, err := s.categorizeByRecency(ctx, userID, candidateExercises, date)
	if err != nil {
		return nil, err
	}

	// 4. Select exercises using our selection strategy
	selectedExercises := s.selectExercisesWithStrategy(category, nonRecentList, recentList, targetCount)

	// 5. Convert to planned exercises with appropriate sets
	return s.createPlannedExercises(ctx, userID, selectedExercises)
}

// getTargetExerciseCount returns the ideal number of exercises for a workout category.
func (s *Service) getTargetExerciseCount(category Category) int {
	fullBodySets := 6
	upperLowerSets := 5
	targetCounts := map[Category]int{
		CategoryFullBody: fullBodySets,
		CategoryUpper:    upperLowerSets,
		CategoryLower:    upperLowerSets,
	}
	return targetCounts[category]
}

// getCandidateExercises retrieves and filters exercises suitable for the given category.
func (s *Service) getCandidateExercises(ctx context.Context, category Category) ([]Exercise, error) {
	allExercises, err := s.repo.getExercisesByCategory(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("get exercises by category: %w", err)
	}

	var candidateExercises []Exercise
	for _, ex := range allExercises {
		if category == CategoryFullBody || ex.Category == category {
			candidateExercises = append(candidateExercises, ex)
		}
	}

	if len(candidateExercises) == 0 {
		return nil, fmt.Errorf("no exercises found for category %s", category)
	}

	return candidateExercises, nil
}

// categorizeByRecency divides exercises into recent and non-recent lists.
func (s *Service) categorizeByRecency(
	ctx context.Context,
	userID []byte,
	exercises []Exercise,
	date time.Time,
) ([]Exercise, []Exercise, error) {
	lookbackPeriod := date.AddDate(0, 0, -14) // 2 weeks ago

	recentExercises, err := s.repo.getRecentExercises(ctx, userID, lookbackPeriod)
	if err != nil {
		return nil, nil, fmt.Errorf("get recent exercises: %w", err)
	}

	var recentList, nonRecentList []Exercise
	for _, ex := range exercises {
		_, isRecent := recentExercises[ex.ID]
		if isRecent {
			recentList = append(recentList, ex)
		} else {
			nonRecentList = append(nonRecentList, ex)
		}
	}

	return recentList, nonRecentList, nil
}

// selectExercisesWithStrategy implements the core selection algorithm.
func (s *Service) selectExercisesWithStrategy(
	category Category,
	nonRecentExercises []Exercise,
	recentExercises []Exercise,
	targetCount int,
) []Exercise {
	// Step 1: Get target muscle groups for this category
	targetMuscleGroups := s.getTargetMuscleGroups(category)

	// Step 2: Select by muscle groups first
	selectedExercises := s.selectByMuscleGroups(targetMuscleGroups, nonRecentExercises, targetCount)

	// Step 3: Fill any remaining slots
	return s.fillRemainingSlots(selectedExercises, nonRecentExercises, recentExercises, targetCount)
}

// getTargetMuscleGroups returns the primary muscle groups to target based on workout category.
func (s *Service) getTargetMuscleGroups(category Category) []string {
	switch category {
	case CategoryUpper:
		return []string{"Chest", "Back", "Shoulders", "Biceps", "Triceps"}
	case CategoryLower:
		return []string{"Quadriceps", "Hamstrings", "Glutes", "Calves"}
	case CategoryFullBody:
		return []string{"Chest", "Back", "Shoulders", "Quadriceps", "Hamstrings", "Glutes"}
	}
	return []string{}
}

// selectByMuscleGroups selects exercises to cover targeted muscle groups.
func (s *Service) selectByMuscleGroups(
	targetMuscleGroups []string,
	candidates []Exercise,
	targetCount int,
) []Exercise {
	selectedExercises := []Exercise{}
	selectedIDs := make(map[int]bool)
	selectedMuscleGroups := make(map[string]bool)

	for _, muscleGroup := range targetMuscleGroups {
		if len(selectedExercises) >= targetCount {
			break
		}

		for _, ex := range candidates {
			// Skip if already selected
			if selectedIDs[ex.ID] {
				continue
			}

			// Check if this exercise targets the muscle group
			if s.exerciseTargetsMuscleGroup(ex, muscleGroup) {
				selectedExercises = append(selectedExercises, ex)
				selectedIDs[ex.ID] = true
				selectedMuscleGroups[muscleGroup] = true
				break
			}
		}
	}

	return selectedExercises
}

// exerciseTargetsMuscleGroup checks if an exercise primarily targets a muscle group.
func (s *Service) exerciseTargetsMuscleGroup(exercise Exercise, muscleGroup string) bool {
	for _, group := range exercise.PrimaryMuscleGroups {
		if group == muscleGroup {
			return true
		}
	}
	return false
}

// fillRemainingSlots adds more exercises to reach the target count.
func (s *Service) fillRemainingSlots(
	selected []Exercise,
	nonRecentCandidates []Exercise,
	recentCandidates []Exercise,
	targetCount int,
) []Exercise {
	// Create a copy of selected exercises
	var result []Exercise
	copy(result, selected)

	// Create a map for quick lookup
	selectedIDs := make(map[int]bool)
	for _, ex := range result {
		selectedIDs[ex.ID] = true
	}

	// Helper to filter and select additional exercises
	addMore := func(candidates []Exercise, count int) {
		added := 0
		for _, ex := range candidates {
			if added >= count {
				break
			}

			if !selectedIDs[ex.ID] {
				result = append(result, ex)
				selectedIDs[ex.ID] = true
				added++
			}
		}
	}

	// First try to add non-recent exercises
	remainingCount := targetCount - len(result)
	if remainingCount > 0 {
		addMore(nonRecentCandidates, remainingCount)
	}

	// If needed, add recent exercises
	remainingCount = targetCount - len(result)
	if remainingCount > 0 {
		addMore(recentCandidates, remainingCount)
	}

	return result
}

// createPlannedExercises adds appropriate sets to each selected exercise.
func (s *Service) createPlannedExercises(
	ctx context.Context,
	userID []byte,
	selectedExercises []Exercise,
) ([]plannedExercise, error) {
	var plannedExercises = make([]plannedExercise, len(selectedExercises))
	for i, ex := range selectedExercises {
		history, err := s.repo.getExercisePerformanceHistory(ctx, userID, ex)
		if err != nil {
			return nil, fmt.Errorf("get exercise history for %s: %w", ex.Name, err)
		}

		plannedExercises[i] = plannedExercise{
			exercise: ex,
			sets:     s.generateSetsForExercise(history),
		}
	}
	return plannedExercises, nil
}

// generateSetsForExercise creates sets with appropriate progression based on history.
func (s *Service) generateSetsForExercise(history exerciseHistory) []plannedSet {
	// Default configuration
	defaultWeight := 20.0 // Default starting weight (kg)
	if history.exercise.Category == CategoryLower {
		defaultWeight = 30.0 // Heavier for lower body
	}
	defaultSets := 3       // Default number of sets
	defaultMinReps := 8    // Default minimum reps
	defaultMaxReps := 12   // Default maximum reps
	weightIncrement := 2.5 // Minimum weight increment (kg)

	// Create planned sets.
	var sets = make([]plannedSet, defaultSets)

	// If no history or incomplete history, use defaults.
	if len(history.performanceData) == 0 {
		for i := range defaultSets {
			sets[i] = plannedSet{
				weightKg:      defaultWeight,
				targetMinReps: defaultMinReps,
				targetMaxReps: defaultMaxReps,
				isWarmup:      i == 0, // First set is warmup
			}
		}
		return sets
	}

	// Calculate average weight and completion success from history.
	var totalWeight float64
	var totalCompletedReps, totalTargetReps, completedSets int

	for _, perf := range history.performanceData {
		totalWeight += perf.weightKg
		totalTargetReps += perf.targetReps

		// Only count completed sets.
		if perf.completedReps > 0 {
			totalCompletedReps += perf.completedReps
			completedSets++
		}
	}

	// Determine base weight for this exercise.
	var baseWeight float64
	if completedSets > 0 {
		averageWeight := totalWeight / float64(len(history.performanceData))
		averageCompletion := float64(totalCompletedReps) / float64(totalTargetReps)

		// Apply progression logic.
		weightIncreaseCompletion := .9
		noWeightIncreaseCompletion := .7
		switch {
		case averageCompletion >= weightIncreaseCompletion:
			baseWeight = averageWeight + weightIncrement
		case averageCompletion >= noWeightIncreaseCompletion:
			baseWeight = averageWeight
		default:
			weightReductionFactor := .95
			baseWeight = math.Max(averageWeight*weightReductionFactor, averageWeight-weightIncrement)
		}
	} else {
		// No completed sets, use the historical weight
		baseWeight = totalWeight / float64(len(history.performanceData))
	}

	// Ensure minimum weight
	if baseWeight < 1.0 {
		baseWeight = defaultWeight
	}

	// Generate sets with the determined weight
	for i := range defaultSets {
		// Warmup set uses lighter weight
		setWeight := baseWeight
		if i == 0 { // First set is warmup
			warmupProportion := .8
			setWeight = math.Max(baseWeight*warmupProportion, baseWeight-weightIncrement)
		}

		// Round to nearest weight increment
		setWeight = math.Round(setWeight/weightIncrement) * weightIncrement

		sets = append(sets, plannedSet{
			weightKg:      setWeight,
			targetMinReps: defaultMinReps,
			targetMaxReps: defaultMaxReps,
			isWarmup:      i == 0,
		})
	}

	return sets
}

// plannedExercise represents an exercise selected for a workout.
type plannedExercise struct {
	// exercise contains the exercise details (name, category, etc.)
	exercise Exercise
	// sets contains the planned sets for this exercise
	sets []plannedSet
}

// plannedSet represents a single set to be performed.
type plannedSet struct {
	// weightKg is the weight in kilograms for this set
	weightKg float64
	// targetMinReps is the minimum number of reps to aim for
	targetMinReps int
	// targetMaxReps is the maximum number of reps to aim for
	targetMaxReps int
	// isWarmup indicates whether this is a warmup set (lower weight)
	isWarmup bool
}
