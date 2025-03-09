// Package generator provides functionality to generate personalized workout sessions.
package workout

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
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
type DateExerciseMap map[time.Time]*Session

// Generator generates workout sessions.
type Generator struct {
	// preferences provided by the user
	preferences Preferences
	// history of workouts from previous 6 months.
	history []Session
	// pool of available exercises.
	pool []Exercise
	// cached index for faster date lookups
	dateHistory DateExerciseMap
}

// NewGenerator constructs a workout generator.
func NewGenerator(prefs Preferences, history []Session, pool []Exercise) (*Generator, error) {
	// Validate inputs
	if len(pool) == 0 {
		return nil, errors.New("exercise pool cannot be empty")
	}

	dateHistory := make(DateExerciseMap)
	for i, session := range history {
		// Only consider completed workouts
		if session.CompletedAt.IsZero() {
			continue
		}

		// Index by date
		sessionDate := time.Date(
			session.Date.Year(),
			session.Date.Month(),
			session.Date.Day(),
			0, 0, 0, 0, time.UTC,
		)
		dateHistory[sessionDate] = &history[i]
	}

	// Create generator
	g := &Generator{
		preferences: prefs,
		history:     history,
		pool:        pool,
		dateHistory: dateHistory,
	}

	return g, nil
}

// Generate generates a new workout session for the given time.
func (g *Generator) Generate(t time.Time) (Session, error) {
	// Validate input
	if t.IsZero() {
		return Session{}, errors.New("workout date cannot be zero")
	}

	// Determine workout category
	category := g.determineWorkoutCategory(t)

	// Select exercises
	exerciseSets, err := g.selectExercises(t, category)
	if err != nil {
		return Session{}, fmt.Errorf("failed to select exercises: %w", err)
	}

	// Create the workout session
	session := Session{
		Date:             t,
		ExerciseSets:     exerciseSets,
		DifficultyRating: nil,
		StartedAt:        time.Time{},
		CompletedAt:      time.Time{},
	}

	return session, nil
}

// determineWorkoutCategory decides what type of workout to create based on history and preferences.
func (g *Generator) determineWorkoutCategory(t time.Time) Category {
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
func (g *Generator) isWorkoutDay(t time.Time) bool {
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
func (g *Generator) wasWorkoutDay(t time.Time) bool {
	normalizedDate := time.Date(
		t.Year(), t.Month(), t.Day(),
		0, 0, 0, 0, time.UTC,
	)
	_, exists := g.dateHistory[normalizedDate]
	return exists
}

// selectExercises selects appropriate exercises for the workout.
func (g *Generator) selectExercises(t time.Time, category Category) ([]ExerciseSet, error) {
	// Filter the exercise pool by category
	filteredPool := g.filterExercisesByCategory(category)
	if len(filteredPool) == 0 {
		return nil, fmt.Errorf("no exercises found for category: %s", category)
	}

	// Find the last workout on the same weekday for exercise continuity
	lastSameWeekdayWorkout := g.findLastSameWeekdayWorkout(t)

	// Select exercises with continuity in mind
	selectedExercises := g.selectExercisesWithContinuity(filteredPool, lastSameWeekdayWorkout, DefaultExercisesPerWorkout)
	if len(selectedExercises) == 0 {
		return nil, fmt.Errorf("failed to select exercises for category: %s", category)
	}

	// Create exercise sets with appropriate sets and reps
	exerciseSets := make([]ExerciseSet, 0, len(selectedExercises))
	for _, exercise := range selectedExercises {
		sets := g.determineSetsRepsWeight(exercise)
		exerciseSets = append(exerciseSets, ExerciseSet{
			Exercise: exercise,
			Sets:     sets,
		})
	}

	return exerciseSets, nil
}

// filterExercisesByCategory returns exercises that match the given category.
func (g *Generator) filterExercisesByCategory(category Category) []Exercise {
	var filtered []Exercise
	for _, exercise := range g.pool {
		if exercise.Category == category {
			filtered = append(filtered, exercise)
		}
	}
	return filtered
}

// findLastSameWeekdayWorkout finds the most recent workout on the same weekday.
func (g *Generator) findLastSameWeekdayWorkout(t time.Time) *Session {
	targetWeekday := t.Weekday()

	var mostRecent *Session
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
func (g *Generator) selectExercisesWithContinuity(
	pool []Exercise,
	lastWorkout *Session,
	count int,
) []Exercise {
	// Handle edge cases
	if count <= 0 {
		return []Exercise{}
	}

	// If no previous workout, just select random exercises
	if lastWorkout == nil {
		return g.selectRandomExercises(pool, count)
	}

	// Calculate the number of exercises to keep
	continuitySets := g.calculateContinuitySets(lastWorkout, count)
	newSets := g.calculateNewSets(count, continuitySets)

	// The result will consist of continued exercises and new exercises
	selected := make([]Exercise, 0, count)
	selected = append(selected, continuitySets...)

	// Add random new exercises if needed
	if newSets > 0 {
		remainingPool := filterOutExercises(pool, selected)
		if len(remainingPool) == 0 {
			// If somehow we don't have any remaining exercises, use original pool
			remainingPool = pool
		}

		// Get random exercises from remaining pool
		randomExercises := g.selectRandomExercises(remainingPool, newSets)
		selected = append(selected, randomExercises...)
	}

	return selected
}

// calculateContinuitySets selects exercises to continue from previous workout.
func (g *Generator) calculateContinuitySets(lastWorkout *Session, totalCount int) []Exercise {
	if lastWorkout == nil {
		return []Exercise{}
	}

	// Calculate how many exercises to keep from previous workout
	continuityCount := int(math.Ceil(float64(totalCount) * ContinuityPercentage))
	if continuityCount > totalCount {
		continuityCount = totalCount
	}

	// Extract exercises from previous workout
	previousExercises := make([]Exercise, 0, len(lastWorkout.ExerciseSets))
	for _, es := range lastWorkout.ExerciseSets {
		previousExercises = append(previousExercises, es.Exercise)
	}

	// Check if we have enough exercises in previous workout
	if len(previousExercises) < continuityCount {
		continuityCount = len(previousExercises)
	}

	// Selected exercises for continuity
	selected := make([]Exercise, 0, continuityCount)

	// First, prioritize compound movements
	selected = g.addPrioritizedExercises(previousExercises, selected, continuityCount, true)

	// Then add non-compound movements if needed
	selected = g.addPrioritizedExercises(previousExercises, selected, continuityCount, false)

	return selected
}

// addPrioritizedExercises adds exercises based on priority (compound or non-compound).
func (g *Generator) addPrioritizedExercises(
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

// calculateNewSets determines how many new exercises to add.
func (g *Generator) calculateNewSets(totalCount int, continuitySets []Exercise) int {
	return totalCount - len(continuitySets)
}

// selectRandomExercises selects random exercises from the pool.
func (g *Generator) selectRandomExercises(pool []Exercise, count int) []Exercise {
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
func (g *Generator) determineSetsRepsWeight(exercise Exercise) []Set {
	// Find most recent occurrence of this exercise in workout history
	lastExerciseSet := g.findMostRecentExerciseSet(exercise.ID)

	// No history - start with default sets and reps
	if lastExerciseSet == nil {
		return createDefaultSets()
	}

	// Apply user feedback if available
	feedback := g.getMostRecentFeedback(exercise.ID)

	// Check if we have feedback to override progression
	if feedback != nil {
		// For "too easy", increase weight directly
		if *feedback == int(FeedbackTooEasy) {
			return increaseWeight(lastExerciseSet.Sets, LargerWeightIncrementKg)
		}
	}

	// Has history - determine progression
	sets := g.progressSets(*lastExerciseSet)

	// Apply other feedback types if available
	if feedback != nil && *feedback != int(FeedbackTooEasy) {
		sets = g.integrateUserFeedback(sets, feedback)
	}

	return sets
}

// createDefaultSets creates a default set of exercises for beginners.
func createDefaultSets() []Set {
	return []Set{
		{WeightKg: 0, MinReps: DefaultReps, MaxReps: DefaultReps, CompletedReps: nil},
		{WeightKg: 0, MinReps: DefaultReps, MaxReps: DefaultReps, CompletedReps: nil},
		{WeightKg: 0, MinReps: DefaultReps, MaxReps: DefaultReps, CompletedReps: nil},
	}
}

// findMostRecentExerciseSet finds the most recent performance of a specific exercise.
func (g *Generator) findMostRecentExerciseSet(exerciseID int) *ExerciseSet {
	var mostRecent *ExerciseSet
	var mostRecentDate time.Time

	for _, session := range g.history {
		// Only consider completed workouts
		if session.CompletedAt.IsZero() {
			continue
		}

		// Look for the exercise in this session
		for i, es := range session.ExerciseSets {
			if es.Exercise.ID == exerciseID {
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
func (g *Generator) isBeginnerUser() bool {
	if len(g.history) == 0 {
		return true // No history, assume beginner
	}

	// Sort history by date
	sortedHistory := make([]Session, len(g.history))
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
func (g *Generator) progressSets(lastExerciseSet ExerciseSet) []Set {
	if g.isBeginnerUser() {
		return g.progressSetsLinear(lastExerciseSet)
	}
	return g.progressSetsUndulating(lastExerciseSet)
}

// progressSetsLinear implements linear progression for beginners.
func (g *Generator) progressSetsLinear(lastExerciseSet ExerciseSet) []Set {
	// Check exercise completion status
	completionStatus := g.evaluateSetCompletion(lastExerciseSet.Sets)

	switch completionStatus {
	case "not_completed":
		// If not completed yet, keep the same sets
		return copySetWithoutCompletion(lastExerciseSet.Sets)
	case "failed":
		// If any set failed, reduce weight
		return reduceWeight(lastExerciseSet.Sets, WeightReductionFactor)
	case "completed_max":
		// If all sets completed at max, increase weight
		return increaseWeight(lastExerciseSet.Sets, StandardWeightIncrementKg)
	default:
		// Partial completion, keep weight the same
		return copySetWithoutCompletion(lastExerciseSet.Sets)
	}
}

// evaluateSetCompletion determines the completion status of a set of exercises.
func (g *Generator) evaluateSetCompletion(sets []Set) string {
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
func (g *Generator) progressSetsUndulating(lastExerciseSet ExerciseSet) []Set {
	// Determine current workout type
	currentType := determineWorkoutType(lastExerciseSet.Sets)

	// Check if all sets were completed at maximum reps
	allCompletedAtMax := allSetsCompletedAtMax(lastExerciseSet.Sets)

	// If not completed at max, maintain current program
	if !allCompletedAtMax {
		return copySetWithoutCompletion(lastExerciseSet.Sets)
	}

	// Now handle the case where all sets were completed at max
	// Count consecutive workouts with all sets at max reps
	consecutiveMaxCompletions := g.countConsecutiveMaxCompletions(lastExerciseSet)

	// Progress to next workout type if completed at max for consecutive workouts
	if consecutiveMaxCompletions >= MaxConsecutiveCompletions {
		// Get weight from the first set (they should all be the same weight)
		weight := 0.0
		if len(lastExerciseSet.Sets) > 0 {
			weight = lastExerciseSet.Sets[0].WeightKg
		}
		return createSetsForWorkoutType(getNextWorkoutType(currentType), weight)
	}

	// Just increase weight within current workout type
	return increaseWeight(lastExerciseSet.Sets, StandardWeightIncrementKg)
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
func (g *Generator) countConsecutiveMaxCompletions(lastExerciseSet ExerciseSet) int {
	// Initial check - if current workout isn't at max, return 0
	if !allSetsCompletedAtMax(lastExerciseSet.Sets) {
		return 0
	}

	count := 1 // Current workout counts as one
	exerciseID := lastExerciseSet.Exercise.ID

	// Look at previous workouts for this exercise
	for i := len(g.history) - 1; i >= 0; i-- {
		session := g.history[i]
		for _, es := range session.ExerciseSets {
			if es.Exercise.ID == exerciseID {
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
	// Apply weight adjustment based on workout type transition
	var adjustedWeight float64
	var minReps, maxReps int

	switch workoutType {
	case WorkoutTypeStrength:
		adjustedWeight = baseWeight * EnduranceToStrengthWeightFactor
		minReps = StrengthMinReps
		maxReps = StrengthMaxReps
	case WorkoutTypeHypertrophy:
		adjustedWeight = baseWeight * StrengthToHypertrophyWeightFactor
		minReps = HypertrophyMinReps
		maxReps = HypertrophyMaxReps
	case WorkoutTypeEndurance:
		adjustedWeight = baseWeight * HypertrophyToEnduranceWeightFactor
		minReps = EnduranceMinReps
		maxReps = EnduranceMaxReps
	default:
		// Default to hypertrophy
		adjustedWeight = baseWeight
		minReps = HypertrophyMinReps
		maxReps = HypertrophyMaxReps
	}

	// Create the sets with the calculated parameters
	return []Set{
		{WeightKg: adjustedWeight, MinReps: minReps, MaxReps: maxReps, CompletedReps: nil},
		{WeightKg: adjustedWeight, MinReps: minReps, MaxReps: maxReps, CompletedReps: nil},
		{WeightKg: adjustedWeight, MinReps: minReps, MaxReps: maxReps, CompletedReps: nil},
	}
}

// integrateUserFeedback adjusts workout intensity based on user feedback.
func (g *Generator) integrateUserFeedback(sets []Set, feedback *int) []Set {
	if feedback == nil {
		return sets // No feedback, no adjustment
	}

	switch FeedbackLevel(*feedback) {
	case FeedbackTooEasy: // Too easy
		// Increase intensity more aggressively
		return increaseWeight(sets, LargerWeightIncrementKg)
	case FeedbackTooDifficult: // Too difficult
		// Reduce volume or intensity
		if len(sets) > MaxStandardSets {
			// Reduce volume by removing a set
			return sets[:len(sets)-1]
		}
		// Reduce intensity by reducing weight
		return reduceWeight(sets, WeightReductionFactor)
	case FeedbackOptimalLow:
		return increaseWeight(sets, StandardWeightIncrementKg)
	case FeedbackOptimalMid:
		return increaseWeight(sets, StandardWeightIncrementKg)
	case FeedbackOptimalHigh:
		return increaseWeight(sets, StandardWeightIncrementKg)
	default: // 2-4 (optimal challenge)
		// For optimal challenge, make a small increase
		return increaseWeight(sets, StandardWeightIncrementKg)
	}
}

// getMostRecentFeedback gets the most recent feedback for a session containing the specified exercise.
func (g *Generator) getMostRecentFeedback(exerciseID int) *int {
	var mostRecentSession *Session
	var mostRecentDate time.Time

	for i, session := range g.history {
		// Only consider sessions with feedback
		if session.DifficultyRating == nil {
			continue
		}

		// Check if this session contains the exercise
		containsExercise := false
		for _, es := range session.ExerciseSets {
			if es.Exercise.ID == exerciseID {
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
	newSets := make([]Set, len(sets))
	for i, set := range sets {
		newSets[i] = Set{
			WeightKg:      set.WeightKg,
			MinReps:       set.MinReps,
			MaxReps:       set.MaxReps,
			CompletedReps: nil, // Reset completion
		}
	}
	return newSets
}

// reduceWeight reduces the weight by a percentage.
func reduceWeight(sets []Set, percentage float64) []Set {
	newSets := make([]Set, len(sets))
	for i, set := range sets {
		reduction := set.WeightKg * percentage
		newWeight := set.WeightKg - reduction
		if newWeight < 0 {
			newWeight = 0
		}

		newSets[i] = Set{
			WeightKg:      newWeight,
			MinReps:       set.MinReps,
			MaxReps:       set.MaxReps,
			CompletedReps: nil, // Reset completion
		}
	}
	return newSets
}

// increaseWeight increases the weight by a fixed amount.
func increaseWeight(sets []Set, increment float64) []Set {
	newSets := make([]Set, len(sets))
	for i, set := range sets {
		newWeight := set.WeightKg + increment

		newSets[i] = Set{
			WeightKg:      newWeight,
			MinReps:       set.MinReps,
			MaxReps:       set.MaxReps,
			CompletedReps: nil, // Reset completion
		}
	}
	return newSets
}
