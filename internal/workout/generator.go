// Package workout provides functionality to generate personalized workout sessions.
package workout

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"time"
)

// Type represents the focus of a workout session.
type Type string

// Workout type constants.
const (
	WorkoutTypeStrength    Type = "strength"
	WorkoutTypeHypertrophy Type = "hypertrophy"
	WorkoutTypeEndurance   Type = "endurance"
)

// FeedbackLevel represents user feedback on workout difficulty.
type FeedbackLevel int

// Feedback rating constants.
const (
	FeedbackTooEasy      FeedbackLevel = 1
	FeedbackOptimalLow   FeedbackLevel = 2
	FeedbackOptimalMid   FeedbackLevel = 3
	FeedbackOptimalHigh  FeedbackLevel = 4
	FeedbackTooDifficult FeedbackLevel = 5
)

// Progression model constants.
const (
	// Exercise selection constants.
	ContinuityPercentage       = 0.8
	DefaultExercisesPerWorkout = 5
	MinCompoundMovementMuscles = 2

	// Weight progression constants.
	StandardWeightIncrementKg = 2.5
	LargerWeightIncrementKg   = 5.0
	WeightReductionFactor     = 0.1
	MaxConsecutiveCompletions = 2

	// Workout type weight adjustment factors.
	HypertrophyToEnduranceWeightFactor = 0.8
	StrengthToHypertrophyWeightFactor  = 0.85
	EnduranceToStrengthWeightFactor    = 1.3

	// User experience classification.
	BeginnerPeriodDays = 90

	// Default values.
	DefaultReps     = 8
	MaxStandardSets = 3

	// Rep ranges for different workout types.
	HypertrophyMinReps = 8
	HypertrophyMaxReps = 12
	EnduranceMinReps   = 12
	EnduranceMaxReps   = 15
	StrengthMinReps    = 3
	StrengthMaxReps    = 6
)

// DateExerciseMap maps dates to completed workout sessions.
type DateExerciseMap map[time.Time]*sessionAggregate

// generator generates workout sessions.
type generator struct {
	// preferences provided by the user
	preferences Preferences
	// history of workouts from previous 6 months.
	history []sessionAggregate
	// pool of available exercises.
	pool []Exercise
	// cached index for faster date lookups
	dateHistory DateExerciseMap
}

// newGenerator constructs a workout generator.
func newGenerator(prefs Preferences, history []sessionAggregate, pool []Exercise) (*generator, error) {
	if err := validateGeneratorInputs(pool); err != nil {
		return nil, err
	}

	dateHistory := buildDateHistoryIndex(history)

	return &generator{
		preferences: prefs,
		history:     history,
		pool:        pool,
		dateHistory: dateHistory,
	}, nil
}

// validateGeneratorInputs validates the inputs for generator creation.
func validateGeneratorInputs(pool []Exercise) error {
	if len(pool) == 0 {
		return errors.New("exercise pool cannot be empty")
	}
	return nil
}

// buildDateHistoryIndex creates a date-indexed map of completed sessions.
func buildDateHistoryIndex(history []sessionAggregate) DateExerciseMap {
	dateHistory := make(DateExerciseMap)
	for i, session := range history {
		if session.CompletedAt.IsZero() {
			continue
		}

		sessionDate := normalizeDate(session.Date)
		dateHistory[sessionDate] = &history[i]
	}
	return dateHistory
}

// normalizeDate normalizes a date to midnight UTC.
func normalizeDate(t time.Time) time.Time {
	return time.Date(
		t.Year(), t.Month(), t.Day(),
		0, 0, 0, 0, time.UTC,
	)
}

// Generate generates a new workout session for the given time.
func (g *generator) Generate(t time.Time) (sessionAggregate, error) {
	if err := validateWorkoutDate(t); err != nil {
		return sessionAggregate{}, err
	}

	category := g.determineWorkoutCategory(t)
	exerciseSets, err := g.selectExercises(t, category)
	if err != nil {
		return sessionAggregate{}, fmt.Errorf("failed to select exercises: %w", err)
	}

	return g.createWorkoutSession(t, exerciseSets), nil
}

// validateWorkoutDate validates the workout date.
func validateWorkoutDate(t time.Time) error {
	if t.IsZero() {
		return errors.New("workout date cannot be zero")
	}
	return nil
}

// createWorkoutSession creates a new workout session.
func (g *generator) createWorkoutSession(date time.Time, exerciseSets []exerciseSetAggregate) sessionAggregate {
	return sessionAggregate{
		Date:             date,
		ExerciseSets:     exerciseSets,
		DifficultyRating: nil,
		StartedAt:        time.Time{},
		CompletedAt:      time.Time{},
	}
}

// determineWorkoutCategory decides what type of workout to create based on history and preferences.
func (g *generator) determineWorkoutCategory(t time.Time) Category {
	// Check if today is a planned workout day
	if !g.isWorkoutDay(t) {
		return CategoryFullBody // Default to full body if today is not a workout day
	}

	// Check if tomorrow is a planned workout
	tomorrow := t.AddDate(0, 0, 1)
	if g.isWorkoutDay(tomorrow) {
		return CategoryLower
	}

	// Check if yesterday was a workout
	yesterday := t.AddDate(0, 0, -1)
	if g.wasWorkoutDay(yesterday) {
		return CategoryUpper
	}

	// Default to full body
	return CategoryFullBody
}

// isWorkoutDay checks if the given day is a planned workout day according to preferences.
func (g *generator) isWorkoutDay(t time.Time) bool {
	switch t.Weekday() {
	case time.Monday:
		return g.preferences.Monday
	case time.Tuesday:
		return g.preferences.Tuesday
	case time.Wednesday:
		return g.preferences.Wednesday
	case time.Thursday:
		return g.preferences.Thursday
	case time.Friday:
		return g.preferences.Friday
	case time.Saturday:
		return g.preferences.Saturday
	case time.Sunday:
		return g.preferences.Sunday
	default:
		return false
	}
}

// wasWorkoutDay checks if there was a completed workout on the given day.
func (g *generator) wasWorkoutDay(t time.Time) bool {
	normalizedDate := normalizeDate(t)
	_, exists := g.dateHistory[normalizedDate]
	return exists
}

// selectExercises selects appropriate exercises for the workout.
func (g *generator) selectExercises(t time.Time, category Category) ([]exerciseSetAggregate, error) {
	filteredPool := g.filterExercisesByCategory(category)
	if len(filteredPool) == 0 {
		return nil, fmt.Errorf("no exercises found for category: %s", category)
	}

	selectedExercises, err := g.selectExercisesForWorkout(t, filteredPool)
	if err != nil {
		return nil, fmt.Errorf("failed to select exercises for category %s: %w", category, err)
	}

	return g.createExerciseSets(selectedExercises), nil
}

// selectExercisesForWorkout selects exercises with continuity consideration.
func (g *generator) selectExercisesForWorkout(t time.Time, pool []Exercise) ([]Exercise, error) {
	lastSameWeekdayWorkout := g.findLastSameWeekdayWorkout(t)
	selectedExercises := g.selectExercisesWithContinuity(pool, lastSameWeekdayWorkout, DefaultExercisesPerWorkout)

	if len(selectedExercises) == 0 {
		return nil, errors.New("no exercises could be selected")
	}

	return selectedExercises, nil
}

// createExerciseSets creates exercise sets with appropriate parameters.
func (g *generator) createExerciseSets(exercises []Exercise) []exerciseSetAggregate {
	exerciseSets := make([]exerciseSetAggregate, 0, len(exercises))
	for _, exercise := range exercises {
		sets := g.determineSetsRepsWeight(exercise)
		exerciseSets = append(exerciseSets, exerciseSetAggregate{
			ExerciseID: exercise.ID,
			Sets:       sets,
		})
	}
	return exerciseSets
}

// filterExercisesByCategory returns exercises that match the given category. If category is full body, it matches all.
func (g *generator) filterExercisesByCategory(category Category) []Exercise {
	var filtered []Exercise
	for _, exercise := range g.pool {
		if category == CategoryFullBody || exercise.Category == category {
			filtered = append(filtered, exercise)
		}
	}
	return filtered
}

// findLastSameWeekdayWorkout finds the most recent workout on the same weekday.
func (g *generator) findLastSameWeekdayWorkout(t time.Time) *sessionAggregate {
	targetWeekday := t.Weekday()

	var mostRecent *sessionAggregate
	var mostRecentDate time.Time

	for _, session := range g.dateHistory {
		if session.Date.Weekday() == targetWeekday &&
			(mostRecent == nil || session.Date.After(mostRecentDate)) {
			mostRecent = session
			mostRecentDate = session.Date
		}
	}

	return mostRecent
}

// selectExercisesWithContinuity selects exercises with ~80% continuity from previous week.
func (g *generator) selectExercisesWithContinuity(
	pool []Exercise,
	lastWorkout *sessionAggregate,
	count int,
) []Exercise {
	if count <= 0 {
		return []Exercise{}
	}

	if lastWorkout == nil {
		return g.selectRandomExercises(pool, count)
	}

	return g.combineExercisesWithContinuity(pool, lastWorkout, count)
}

// combineExercisesWithContinuity combines continued and new exercises.
func (g *generator) combineExercisesWithContinuity(
	pool []Exercise,
	lastWorkout *sessionAggregate,
	count int,
) []Exercise {
	continuitySets := g.calculateContinuitySets(lastWorkout, count)
	newSets := count - len(continuitySets)

	selected := make([]Exercise, 0, count)
	selected = append(selected, continuitySets...)

	if newSets > 0 {
		newExercises := g.selectNewExercises(pool, selected, newSets)
		selected = append(selected, newExercises...)
	}

	return selected
}

// selectNewExercises selects new exercises not already in the selection.
func (g *generator) selectNewExercises(pool, selected []Exercise, count int) []Exercise {
	remainingPool := filterOutExercises(pool, selected)
	if len(remainingPool) == 0 {
		remainingPool = pool
	}
	return g.selectRandomExercises(remainingPool, count)
}

// calculateContinuitySets selects exercises to continue from previous workout.
func (g *generator) calculateContinuitySets(lastWorkout *sessionAggregate, totalCount int) []Exercise {
	if lastWorkout == nil {
		return []Exercise{}
	}

	continuityCount := g.calculateContinuityCount(totalCount)
	previousExercises, err := g.extractPreviousExercises(lastWorkout)
	if err != nil {
		// Log error and return empty slice instead of panicking
		return []Exercise{}
	}

	return g.selectContinuityExercises(previousExercises, continuityCount)
}

// calculateContinuityCount calculates how many exercises to continue.
func (g *generator) calculateContinuityCount(totalCount int) int {
	continuityCount := int(math.Ceil(float64(totalCount) * ContinuityPercentage))
	if continuityCount > totalCount {
		continuityCount = totalCount
	}
	return continuityCount
}

// extractPreviousExercises extracts exercises from the previous workout.
func (g *generator) extractPreviousExercises(lastWorkout *sessionAggregate) ([]Exercise, error) {
	previousExercises := make([]Exercise, 0, len(lastWorkout.ExerciseSets))
	for _, es := range lastWorkout.ExerciseSets {
		idx := slices.IndexFunc(g.pool, func(exercise Exercise) bool {
			return exercise.ID == es.ExerciseID
		})
		if idx == -1 {
			return nil, fmt.Errorf("exercise %d not found in pool", es.ExerciseID)
		}
		previousExercises = append(previousExercises, g.pool[idx])
	}
	return previousExercises, nil
}

// selectContinuityExercises selects exercises for continuity with prioritization.
func (g *generator) selectContinuityExercises(previousExercises []Exercise, continuityCount int) []Exercise {
	if len(previousExercises) < continuityCount {
		continuityCount = len(previousExercises)
	}

	selected := make([]Exercise, 0, continuityCount)
	selected = g.addPrioritizedExercises(previousExercises, selected, continuityCount, true)
	selected = g.addPrioritizedExercises(previousExercises, selected, continuityCount, false)
	return selected
}

// addPrioritizedExercises adds exercises based on priority (compound or non-compound).
func (g *generator) addPrioritizedExercises(
	sourceExercises []Exercise,
	selected []Exercise,
	targetCount int,
	compoundOnly bool,
) []Exercise {
	for _, ex := range sourceExercises {
		if len(selected) >= targetCount {
			break
		}

		isCompound := isCompoundMovement(ex)
		if (compoundOnly && isCompound) || (!compoundOnly && !isCompound) {
			if !containsExercise(selected, ex) {
				selected = append(selected, ex)
			}
		}
	}

	return selected
}

// selectRandomExercises selects random exercises from the pool.
func (g *generator) selectRandomExercises(pool []Exercise, count int) []Exercise {
	// Handle edge cases
	if count <= 0 {
		return []Exercise{}
	}

	if count >= len(pool) {
		return pool // Return all if we need more than available
	}

	// Create a copy of the pool to shuffle
	shuffled := make([]Exercise, len(pool))
	copy(shuffled, pool)

	// Shuffle the pool
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// Return the first 'count' exercises
	return shuffled[:count]
}

// isCompoundMovement determines if an exercise is a compound movement.
func isCompoundMovement(ex Exercise) bool {
	return len(ex.PrimaryMuscleGroups) >= MinCompoundMovementMuscles
}

// containsExercise checks if an exercise is in a slice.
func containsExercise(exercises []Exercise, target Exercise) bool {
	for _, ex := range exercises {
		if ex.ID == target.ID {
			return true
		}
	}
	return false
}

// filterOutExercises removes specified exercises from a pool.
func filterOutExercises(pool, toFilter []Exercise) []Exercise {
	var filtered []Exercise
	for _, ex := range pool {
		if !containsExercise(toFilter, ex) {
			filtered = append(filtered, ex)
		}
	}
	return filtered
}

// determineSetsRepsWeight determines sets, reps, and weights for an exercise.
func (g *generator) determineSetsRepsWeight(exercise Exercise) []Set {
	lastExerciseSet := g.findMostRecentExerciseSet(exercise.ID)
	if lastExerciseSet == nil {
		return createDefaultSets(exercise.ExerciseType)
	}

	feedback := g.getMostRecentFeedback(exercise.ID)
	return g.createProgressiveSets(*lastExerciseSet, feedback, exercise.ExerciseType)
}

// createProgressiveSets creates sets with progression based on history and feedback.
func (g *generator) createProgressiveSets(lastExerciseSet exerciseSetAggregate, feedback *int, exerciseType ExerciseType) []Set {
	if feedback != nil && *feedback == int(FeedbackTooEasy) {
		if exerciseType == ExerciseTypeBodyweight {
			return increaseReps(lastExerciseSet.Sets, 2)
		}
		return increaseWeight(lastExerciseSet.Sets, LargerWeightIncrementKg)
	}

	sets := g.progressSets(lastExerciseSet, exerciseType)
	if feedback != nil && *feedback != int(FeedbackTooEasy) {
		sets = g.integrateUserFeedback(sets, feedback, exerciseType)
	}

	return sets
}

// createDefaultSets creates a default set of exercises for beginners with 8 reps.
func createDefaultSets(exerciseType ExerciseType) []Set {
	var weight *float64
	if exerciseType == ExerciseTypeWeighted {
		weight = &[]float64{0}[0] // Starting weight of 0 for weighted exercises
	}
	// For bodyweight exercises, weight remains nil

	return []Set{
		{WeightKg: weight, MinReps: DefaultReps, MaxReps: DefaultReps, CompletedReps: nil},
		{WeightKg: weight, MinReps: DefaultReps, MaxReps: DefaultReps, CompletedReps: nil},
		{WeightKg: weight, MinReps: DefaultReps, MaxReps: DefaultReps, CompletedReps: nil},
	}
}

// findMostRecentExerciseSet finds the most recent performance of a specific exercise.
func (g *generator) findMostRecentExerciseSet(exerciseID int) *exerciseSetAggregate {
	var mostRecent *exerciseSetAggregate
	var mostRecentDate time.Time

	for _, session := range g.history {
		// Only consider completed workouts
		if session.CompletedAt.IsZero() {
			continue
		}

		// Look for the exercise in this session
		for i, es := range session.ExerciseSets {
			if es.ExerciseID == exerciseID {
				if mostRecent == nil || session.Date.After(mostRecentDate) {
					mostRecent = &session.ExerciseSets[i]
					mostRecentDate = session.Date
				}
				break // Found the exercise in this session, move to next session
			}
		}
	}

	return mostRecent
}

// isBeginnerUser determines if a user is a beginner (1-3 months of training).
func (g *generator) isBeginnerUser() bool {
	if len(g.history) == 0 {
		return true // No history, assume beginner
	}

	// Sort history by date
	sortedHistory := make([]sessionAggregate, len(g.history))
	copy(sortedHistory, g.history)
	sort.Slice(sortedHistory, func(i, j int) bool {
		return sortedHistory[i].Date.Before(sortedHistory[j].Date)
	})

	// Find the oldest workout
	oldestWorkout := sortedHistory[0]

	// Calculate duration since oldest workout
	duration := time.Since(oldestWorkout.Date)

	// Check if it's less than the beginner period
	return duration < BeginnerPeriodDays*24*time.Hour
}

// progressSets determines the new sets based on the previous performance.
func (g *generator) progressSets(lastExerciseSet exerciseSetAggregate, exerciseType ExerciseType) []Set {
	if g.isBeginnerUser() {
		return g.progressSetsLinear(lastExerciseSet, exerciseType)
	}
	return g.progressSetsUndulating(lastExerciseSet, exerciseType)
}

// progressSetsLinear implements linear progression for beginners.
func (g *generator) progressSetsLinear(lastExerciseSet exerciseSetAggregate, exerciseType ExerciseType) []Set {
	// Check exercise completion status
	completionStatus := g.evaluateSetCompletion(lastExerciseSet.Sets)

	switch completionStatus {
	case "not_completed":
		// If not completed yet, keep the same sets
		return copySetWithoutCompletion(lastExerciseSet.Sets)
	case "failed":
		// If any set failed, reduce difficulty
		if exerciseType == ExerciseTypeBodyweight {
			return g.reduceBodyweightDifficulty(lastExerciseSet.Sets)
		}
		return reduceWeight(lastExerciseSet.Sets, WeightReductionFactor)
	case "completed_max":
		// If all sets completed at max, increase difficulty
		if exerciseType == ExerciseTypeBodyweight {
			return g.increaseBodyweightDifficulty(lastExerciseSet.Sets)
		}
		return increaseWeight(lastExerciseSet.Sets, StandardWeightIncrementKg)
	default:
		// Partial completion, keep difficulty the same
		return copySetWithoutCompletion(lastExerciseSet.Sets)
	}
}

// evaluateSetCompletion determines the completion status of a set of exercises.
func (g *generator) evaluateSetCompletion(sets []Set) string {
	allCompleted := true
	anyFailed := false

	for _, set := range sets {
		if set.CompletedReps == nil {
			return "not_completed"
		}

		if *set.CompletedReps < set.MinReps {
			anyFailed = true
		} else if *set.CompletedReps < set.MaxReps {
			allCompleted = false
		}
	}

	if anyFailed {
		return "failed"
	} else if allCompleted {
		return "completed_max"
	}

	return "completed_partial"
}

// progressSetsUndulating implements undulating periodization for experienced users.
func (g *generator) progressSetsUndulating(lastExerciseSet exerciseSetAggregate, exerciseType ExerciseType) []Set {
	currentType := determineWorkoutType(lastExerciseSet.Sets)
	allCompletedAtMax := allSetsCompletedAtMax(lastExerciseSet.Sets)

	if !allCompletedAtMax {
		return copySetWithoutCompletion(lastExerciseSet.Sets)
	}

	return g.handleMaxCompletionProgression(lastExerciseSet, currentType, exerciseType)
}

// handleMaxCompletionProgression handles progression when all sets completed at max.
func (g *generator) handleMaxCompletionProgression(lastExerciseSet exerciseSetAggregate, currentType Type, exerciseType ExerciseType) []Set {
	consecutiveMaxCompletions := g.countConsecutiveMaxCompletions(lastExerciseSet)

	if consecutiveMaxCompletions >= MaxConsecutiveCompletions {
		if exerciseType == ExerciseTypeBodyweight {
			// For bodyweight, switch workout types with rep adjustments
			return createSetsForBodyweightWorkoutType(getNextWorkoutType(currentType))
		}
		weight := g.extractWeightFromSets(lastExerciseSet.Sets)
		return createSetsForWorkoutType(getNextWorkoutType(currentType), weight)
	}

	if exerciseType == ExerciseTypeBodyweight {
		return g.increaseBodyweightDifficulty(lastExerciseSet.Sets)
	}
	return increaseWeight(lastExerciseSet.Sets, StandardWeightIncrementKg)
}

// extractWeightFromSets extracts weight from the first set.
func (g *generator) extractWeightFromSets(sets []Set) float64 {
	if len(sets) > 0 && sets[0].WeightKg != nil {
		return *sets[0].WeightKg
	}
	return 0.0
}

// getNextWorkoutType determines the next workout type in the cycle.
func getNextWorkoutType(currentType Type) Type {
	switch currentType {
	case WorkoutTypeStrength:
		return WorkoutTypeHypertrophy
	case WorkoutTypeHypertrophy:
		return WorkoutTypeEndurance
	case WorkoutTypeEndurance:
		return WorkoutTypeStrength
	default:
		return WorkoutTypeHypertrophy
	}
}

// countConsecutiveMaxCompletions counts how many consecutive workouts had all sets completed at max reps.
func (g *generator) countConsecutiveMaxCompletions(lastExerciseSet exerciseSetAggregate) int {
	// Initial check - if current workout isn't at max, return 0
	if !allSetsCompletedAtMax(lastExerciseSet.Sets) {
		return 0
	}

	count := 1 // Current workout counts as one
	exerciseID := lastExerciseSet.ExerciseID

	// Look at previous workouts for this exercise
	for i := len(g.history) - 1; i >= 0; i-- {
		session := g.history[i]
		for _, es := range session.ExerciseSets {
			if es.ExerciseID == exerciseID {
				if allSetsCompletedAtMax(es.Sets) {
					count++
					if count >= MaxConsecutiveCompletions {
						return count
					}
				} else {
					// Break at first non-max completion
					return count
				}
			}
		}
	}

	return count
}

// determineWorkoutType identifies the type of workout based on rep ranges.
func determineWorkoutType(sets []Set) Type {
	if len(sets) == 0 {
		return WorkoutTypeHypertrophy // Default
	}

	// Check the rep range of the first set
	minReps := sets[0].MinReps
	maxReps := sets[0].MaxReps

	switch {
	case minReps >= StrengthMinReps && maxReps <= StrengthMaxReps:
		return WorkoutTypeStrength
	case minReps >= HypertrophyMinReps && maxReps <= HypertrophyMaxReps:
		return WorkoutTypeHypertrophy
	case minReps >= EnduranceMinReps && maxReps <= EnduranceMaxReps:
		return WorkoutTypeEndurance
	default:
		return WorkoutTypeHypertrophy
	}
}

// allSetsCompletedAtMax checks if all sets were completed at maximum reps.
func allSetsCompletedAtMax(sets []Set) bool {
	for _, set := range sets {
		if set.CompletedReps == nil || *set.CompletedReps < set.MaxReps {
			return false
		}
	}
	return true
}

// createSetsForWorkoutType creates sets for a specific workout type with adjusted weight.
func createSetsForWorkoutType(workoutType Type, baseWeight float64) []Set {
	params := getWorkoutTypeParameters(workoutType, baseWeight)
	return createStandardSets(params.weight, params.minReps, params.maxReps)
}

// createSetsForBodyweightWorkoutType creates sets for bodyweight exercises with workout type variations.
func createSetsForBodyweightWorkoutType(workoutType Type) []Set {
	var minReps, maxReps int
	switch workoutType {
	case WorkoutTypeStrength:
		minReps, maxReps = 3, 6
	case WorkoutTypeHypertrophy:
		minReps, maxReps = 8, 12
	case WorkoutTypeEndurance:
		minReps, maxReps = 12, 15
	default:
		minReps, maxReps = 8, 12
	}
	return createStandardBodyweightSets(minReps, maxReps)
}

// workoutTypeParams holds parameters for a workout type.
type workoutTypeParams struct {
	weight  float64
	minReps int
	maxReps int
}

// getWorkoutTypeParameters returns parameters for a specific workout type.
func getWorkoutTypeParameters(workoutType Type, baseWeight float64) workoutTypeParams {
	switch workoutType {
	case WorkoutTypeStrength:
		return workoutTypeParams{
			weight:  baseWeight * EnduranceToStrengthWeightFactor,
			minReps: StrengthMinReps,
			maxReps: StrengthMaxReps,
		}
	case WorkoutTypeHypertrophy:
		return workoutTypeParams{
			weight:  baseWeight * StrengthToHypertrophyWeightFactor,
			minReps: HypertrophyMinReps,
			maxReps: HypertrophyMaxReps,
		}
	case WorkoutTypeEndurance:
		return workoutTypeParams{
			weight:  baseWeight * HypertrophyToEnduranceWeightFactor,
			minReps: EnduranceMinReps,
			maxReps: EnduranceMaxReps,
		}
	default:
		return workoutTypeParams{
			weight:  baseWeight,
			minReps: HypertrophyMinReps,
			maxReps: HypertrophyMaxReps,
		}
	}
}

// createStandardSets creates a standard set of 3 sets with given parameters.
func createStandardSets(weight float64, minReps, maxReps int) []Set {
	weightPtr := &weight
	set := Set{
		WeightKg:      weightPtr,
		MinReps:       minReps,
		MaxReps:       maxReps,
		CompletedReps: nil,
	}
	return []Set{set, set, set}
}

// createStandardBodyweightSets creates a standard set of 3 bodyweight sets.
func createStandardBodyweightSets(minReps, maxReps int) []Set {
	set := Set{
		WeightKg:      nil, // No weight for bodyweight exercises
		MinReps:       minReps,
		MaxReps:       maxReps,
		CompletedReps: nil,
	}
	return []Set{set, set, set}
}

// integrateUserFeedback adjusts workout intensity based on user feedback.
func (g *generator) integrateUserFeedback(sets []Set, feedback *int, exerciseType ExerciseType) []Set {
	if feedback == nil {
		return sets
	}

	feedbackLevel := FeedbackLevel(*feedback)
	switch feedbackLevel {
	case FeedbackTooEasy:
		if exerciseType == ExerciseTypeBodyweight {
			return g.increaseBodyweightDifficulty(sets)
		}
		return increaseWeight(sets, LargerWeightIncrementKg)
	case FeedbackTooDifficult:
		return g.reduceDifficulty(sets, exerciseType)
	case FeedbackOptimalLow, FeedbackOptimalMid, FeedbackOptimalHigh:
		if exerciseType == ExerciseTypeBodyweight {
			return g.increaseBodyweightDifficulty(sets)
		}
		return increaseWeight(sets, StandardWeightIncrementKg)
	default:
		if exerciseType == ExerciseTypeBodyweight {
			return g.increaseBodyweightDifficulty(sets)
		}
		return increaseWeight(sets, StandardWeightIncrementKg)
	}
}

// reduceDifficulty reduces workout difficulty by volume or intensity.
func (g *generator) reduceDifficulty(sets []Set, exerciseType ExerciseType) []Set {
	if exerciseType == ExerciseTypeBodyweight {
		return g.reduceBodyweightDifficulty(sets)
	}
	if len(sets) > MaxStandardSets {
		return sets[:len(sets)-1] // Reduce volume
	}
	return reduceWeight(sets, WeightReductionFactor) // Reduce intensity
}

// getMostRecentFeedback gets the most recent feedback for a session containing the specified exercise.
func (g *generator) getMostRecentFeedback(exerciseID int) *int {
	var mostRecentSession *sessionAggregate
	var mostRecentDate time.Time

	for i, session := range g.history {
		// Only consider sessions with feedback
		if session.DifficultyRating == nil {
			continue
		}

		// Check if this session contains the exercise
		containsExercise := false
		for _, es := range session.ExerciseSets {
			if es.ExerciseID == exerciseID {
				containsExercise = true
				break
			}
		}

		if containsExercise &&
			(mostRecentSession == nil || session.Date.After(mostRecentDate)) {
			mostRecentSession = &g.history[i]
			mostRecentDate = session.Date
		}
	}

	if mostRecentSession != nil {
		return mostRecentSession.DifficultyRating
	}

	return nil
}

// copySetWithoutCompletion creates a copy of sets without completed reps.
func copySetWithoutCompletion(sets []Set) []Set {
	return transformSets(sets, func(set Set) Set {
		return Set{
			WeightKg:      set.WeightKg,
			MinReps:       set.MinReps,
			MaxReps:       set.MaxReps,
			CompletedReps: nil,
		}
	})
}

// reduceWeight reduces the weight by a percentage.
func reduceWeight(sets []Set, percentage float64) []Set {
	return transformSets(sets, func(set Set) Set {
		var newWeight *float64
		if set.WeightKg != nil {
			reduction := *set.WeightKg * percentage
			weightValue := math.Max(0, *set.WeightKg-reduction)
			newWeight = &weightValue
		}
		return Set{
			WeightKg:      newWeight,
			MinReps:       set.MinReps,
			MaxReps:       set.MaxReps,
			CompletedReps: nil,
		}
	})
}

// increaseWeight increases the weight by a fixed amount.
func increaseWeight(sets []Set, increment float64) []Set {
	return transformSets(sets, func(set Set) Set {
		var newWeight *float64
		if set.WeightKg != nil {
			newWeight = &[]float64{*set.WeightKg + increment}[0]
		}
		return Set{
			WeightKg:      newWeight,
			MinReps:       set.MinReps,
			MaxReps:       set.MaxReps,
			CompletedReps: nil,
		}
	})
}

// increaseReps increases the rep range for bodyweight exercises.
func increaseReps(sets []Set, increment int) []Set {
	return transformSets(sets, func(set Set) Set {
		return Set{
			WeightKg:      set.WeightKg, // Keep weight nil for bodyweight
			MinReps:       set.MinReps + increment,
			MaxReps:       set.MaxReps + increment,
			CompletedReps: nil,
		}
	})
}

// increaseBodyweightDifficulty increases difficulty for bodyweight exercises.
func (g *generator) increaseBodyweightDifficulty(sets []Set) []Set {
	// First try increasing reps up to 15
	if len(sets) > 0 && sets[0].MaxReps < 15 {
		return increaseReps(sets, 2)
	}
	// If already at high reps, add a set (up to 5 sets max)
	if len(sets) < 5 {
		newSet := Set{
			WeightKg:      nil, // Bodyweight, no weight
			MinReps:       8,   // Reset to base reps for new set
			MaxReps:       8,
			CompletedReps: nil,
		}
		return append(sets, newSet)
	}
	// If already at max sets and reps, keep same difficulty
	return copySetWithoutCompletion(sets)
}

// reduceBodyweightDifficulty reduces difficulty for bodyweight exercises.
func (g *generator) reduceBodyweightDifficulty(sets []Set) []Set {
	// First try reducing reps (minimum 5)
	if len(sets) > 0 && sets[0].MinReps > 5 {
		return transformSets(sets, func(set Set) Set {
			return Set{
				WeightKg:      set.WeightKg,
				MinReps:       set.MinReps - 2,
				MaxReps:       set.MaxReps - 2,
				CompletedReps: nil,
			}
		})
	}
	// If at minimum reps, reduce sets (minimum 2 sets)
	if len(sets) > 2 {
		return sets[:len(sets)-1]
	}
	// If already at minimum, keep same difficulty
	return copySetWithoutCompletion(sets)
}

// transformSets applies a transformation function to all sets.
func transformSets(sets []Set, transform func(Set) Set) []Set {
	newSets := make([]Set, len(sets))
	for i, set := range sets {
		newSets[i] = transform(set)
	}
	return newSets
}
