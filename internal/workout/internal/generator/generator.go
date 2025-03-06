// Package generator provides functionality to generate personalized workout sessions.
package generator

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/myrjola/petrapp/internal/workout"
)

// Generator generates workout sessions.
type Generator struct {
	// preferences provided by the user
	preferences workout.Preferences
	// history of workouts from previous 3 months.
	history []workout.Session
	// pool of available exercises.
	pool []workout.Exercise
}

// NewGenerator constructs a workout generator.
func NewGenerator(preferences workout.Preferences, history []workout.Session, pool []workout.Exercise) *Generator {
	return &Generator{
		preferences: preferences,
		history:     history,
		pool:        pool,
	}
}

// Generate generates a new workout session for the given time.
func (g *Generator) Generate(t time.Time) (workout.Session, error) {
	// Determine workout category
	category := g.determineWorkoutCategory(t)

	// Select exercises
	exerciseSets, err := g.selectExercises(t, category)
	if err != nil {
		return workout.Session{}, fmt.Errorf("failed to select exercises: %w", err)
	}

	// Create the workout session
	session := workout.Session{
		WorkoutDate:  t,
		ExerciseSets: exerciseSets,
		Status:       workout.StatusPlanned,
	}

	return session, nil
}

// determineWorkoutCategory decides what type of workout to create based on history and preferences.
func (g *Generator) determineWorkoutCategory(t time.Time) workout.Category {
	// Check if today is a planned workout day
	if !g.isWorkoutDay(t) {
		return workout.CategoryFullBody // Default to full body if today is not a workout day
	}

	// Check if tomorrow is a planned workout
	tomorrow := t.AddDate(0, 0, 1)
	if g.isWorkoutDay(tomorrow) {
		return workout.CategoryLower
	}

	// Check if yesterday was a workout
	yesterday := t.AddDate(0, 0, -1)
	if g.wasWorkoutDay(yesterday) {
		return workout.CategoryUpper
	}

	// Default to full body
	return workout.CategoryFullBody
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
	for _, session := range g.history {
		if session.WorkoutDate.Year() == t.Year() &&
			session.WorkoutDate.Month() == t.Month() &&
			session.WorkoutDate.Day() == t.Day() &&
			session.Status == workout.StatusDone {
			return true
		}
	}
	return false
}

// selectExercises selects appropriate exercises for the workout.
func (g *Generator) selectExercises(t time.Time, category workout.Category) ([]workout.ExerciseSet, error) {
	// Filter the exercise pool by category
	filteredPool := g.filterExercisesByCategory(category)
	if len(filteredPool) == 0 {
		return nil, fmt.Errorf("no exercises found for category: %s", category)
	}

	// Find the last workout on the same weekday for exercise continuity
	lastSameWeekdayWorkout := g.findLastSameWeekdayWorkout(t)

	// Number of exercises to select (adjust as needed)
	const numExercises = 5

	// Select exercises with continuity in mind
	selectedExercises := g.selectExercisesWithContinuity(filteredPool, lastSameWeekdayWorkout, numExercises)

	// Create exercise sets with appropriate sets and reps
	exerciseSets := make([]workout.ExerciseSet, 0, len(selectedExercises))
	for _, exercise := range selectedExercises {
		sets := g.determineSetsRepsWeight(exercise, lastSameWeekdayWorkout)
		exerciseSets = append(exerciseSets, workout.ExerciseSet{
			Exercise: exercise,
			Sets:     sets,
		})
	}

	return exerciseSets, nil
}

// filterExercisesByCategory returns exercises that match the given category.
func (g *Generator) filterExercisesByCategory(category workout.Category) []workout.Exercise {
	var filtered []workout.Exercise
	for _, exercise := range g.pool {
		if exercise.Category == category {
			filtered = append(filtered, exercise)
		}
	}
	return filtered
}

// findLastSameWeekdayWorkout finds the most recent workout on the same weekday.
func (g *Generator) findLastSameWeekdayWorkout(t time.Time) *workout.Session {
	targetWeekday := t.Weekday()

	var mostRecent *workout.Session
	var mostRecentDate time.Time

	for i, session := range g.history {
		if session.WorkoutDate.Weekday() == targetWeekday &&
			(mostRecent == nil || session.WorkoutDate.After(mostRecentDate)) {
			mostRecent = &g.history[i]
			mostRecentDate = session.WorkoutDate
		}
	}

	return mostRecent
}

// selectExercisesWithContinuity selects exercises with ~80% continuity from previous week.
func (g *Generator) selectExercisesWithContinuity(
	pool []workout.Exercise,
	lastWorkout *workout.Session,
	count int,
) []workout.Exercise {
	if count <= 0 {
		return []workout.Exercise{}
	}

	// If no previous workout, just select random exercises
	if lastWorkout == nil {
		return g.selectRandomExercises(pool, count)
	}

	// Calculate how many exercises to keep from previous workout (~80%)
	continuityCount := int(math.Ceil(float64(count) * 0.8))
	if continuityCount > count {
		continuityCount = count
	}

	// Extract exercises from previous workout
	previousExercises := make([]workout.Exercise, 0, len(lastWorkout.ExerciseSets))
	for _, es := range lastWorkout.ExerciseSets {
		previousExercises = append(previousExercises, es.Exercise)
	}

	// Check if we have enough exercises in previous workout
	if len(previousExercises) < continuityCount {
		continuityCount = len(previousExercises)
	}

	// Select exercises for continuity
	selected := make([]workout.Exercise, 0, count)

	// First, prioritize compound movements from previous workout
	for _, ex := range previousExercises {
		if len(selected) >= continuityCount {
			break
		}
		if isCompoundMovement(ex) && !containsExercise(selected, ex) {
			selected = append(selected, ex)
		}
	}

	// If we still need more for continuity, add non-compound movements
	for _, ex := range previousExercises {
		if len(selected) >= continuityCount {
			break
		}
		if !isCompoundMovement(ex) && !containsExercise(selected, ex) {
			selected = append(selected, ex)
		}
	}

	// Fill the rest with exercises not in the previous workout
	remainingCount := count - len(selected)
	if remainingCount > 0 {
		remainingPool := filterOutExercises(pool, selected)
		if len(remainingPool) == 0 {
			// If somehow we don't have any remaining exercises, use original pool
			remainingPool = pool
		}

		// Get random exercises from remaining pool
		randomExercises := g.selectRandomExercises(remainingPool, remainingCount)
		selected = append(selected, randomExercises...)
	}

	return selected
}

// selectRandomExercises selects random exercises from the pool.
func (g *Generator) selectRandomExercises(pool []workout.Exercise, count int) []workout.Exercise {
	if count <= 0 {
		return []workout.Exercise{}
	}

	if count >= len(pool) {
		return pool // Return all if we need more than available
	}

	// Create a copy of the pool to shuffle
	shuffled := make([]workout.Exercise, len(pool))
	copy(shuffled, pool)

	// Shuffle the pool
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	// Return the first 'count' exercises
	return shuffled[:count]
}

// isCompoundMovement determines if an exercise is a compound movement.
func isCompoundMovement(ex workout.Exercise) bool {
	return len(ex.PrimaryMuscleGroups) >= 2
}

// containsExercise checks if an exercise is in a slice.
func containsExercise(exercises []workout.Exercise, target workout.Exercise) bool {
	for _, ex := range exercises {
		if ex.ID == target.ID {
			return true
		}
	}
	return false
}

// filterOutExercises removes specified exercises from a pool.
func filterOutExercises(pool, toFilter []workout.Exercise) []workout.Exercise {
	var filtered []workout.Exercise
	for _, ex := range pool {
		if !containsExercise(toFilter, ex) {
			filtered = append(filtered, ex)
		}
	}
	return filtered
}

// determineSetsRepsWeight determines sets, reps, and weights for an exercise.
func (g *Generator) determineSetsRepsWeight(
	exercise workout.Exercise,
	lastWorkout *workout.Session,
) []workout.Set {
	// Find if this exercise has history
	var lastExerciseSet *workout.ExerciseSet
	if lastWorkout != nil {
		for i, es := range lastWorkout.ExerciseSets {
			if es.Exercise.ID == exercise.ID {
				lastExerciseSet = &lastWorkout.ExerciseSets[i]
				break
			}
		}
	}

	// No history - start with 3 sets of 8 reps
	if lastExerciseSet == nil {
		return []workout.Set{
			{
				WeightKg:         0,
				AdjustedWeightKg: 0,
				MinReps:          8,
				MaxReps:          8,
				CompletedReps:    nil,
			},
			{
				WeightKg:         0,
				AdjustedWeightKg: 0,
				MinReps:          8,
				MaxReps:          8,
				CompletedReps:    nil,
			},
			{
				WeightKg:         0,
				AdjustedWeightKg: 0,
				MinReps:          8,
				MaxReps:          8,
				CompletedReps:    nil,
			},
		}
	}

	// Has history - determine progression
	sets := g.progressSets(*lastExerciseSet)

	// Apply user feedback if available
	feedback := g.getMostRecentFeedback(exercise.ID)
	if feedback != nil {
		sets = g.integrateUserFeedback(sets, feedback)
	}

	return sets
}

// isBeginnerUser determines if a user is a beginner (1-3 months of training).
func (g *Generator) isBeginnerUser() bool {
	if len(g.history) == 0 {
		return true // No history, assume beginner
	}

	// Sort history by date
	sortedHistory := make([]workout.Session, len(g.history))
	copy(sortedHistory, g.history)
	sort.Slice(sortedHistory, func(i, j int) bool {
		return sortedHistory[i].WorkoutDate.Before(sortedHistory[j].WorkoutDate)
	})

	// Find the oldest workout
	oldestWorkout := sortedHistory[0]

	// Calculate duration since oldest workout
	duration := time.Since(oldestWorkout.WorkoutDate)

	// Check if it's less than 3 months
	return duration < 90*24*time.Hour
}

// progressSets determines the new sets based on the previous performance.
func (g *Generator) progressSets(lastExerciseSet workout.ExerciseSet) []workout.Set {
	if g.isBeginnerUser() {
		return g.progressSetsLinear(lastExerciseSet)
	}
	return g.progressSetsUndulating(lastExerciseSet)
}

// progressSetsLinear implements linear progression for beginners.
func (g *Generator) progressSetsLinear(lastExerciseSet workout.ExerciseSet) []workout.Set {
	// Check if all sets were completed
	allCompleted := true
	anyFailed := false

	for _, set := range lastExerciseSet.Sets {
		if set.CompletedReps == nil {
			// If not completed yet, keep the same sets
			return copySetWithoutCompletion(lastExerciseSet.Sets)
		}

		if *set.CompletedReps < set.MinReps {
			anyFailed = true
		} else if *set.CompletedReps < set.MaxReps {
			allCompleted = false
		}
	}

	// Determine if we should increase, maintain, or decrease weight
	if anyFailed {
		// If any set failed, reduce weight by 5-10%
		return reduceWeight(lastExerciseSet.Sets, 0.1)
	} else if allCompleted {
		// If all sets completed successfully, increase weight
		return increaseWeight(lastExerciseSet.Sets, 2.5) // Add 2.5kg
	}
	// If sets partially complete, keep weight the same
	return copySetWithoutCompletion(lastExerciseSet.Sets)
}

// progressSetsUndulating implements undulating periodization for experienced users.
func (g *Generator) progressSetsUndulating(lastExerciseSet workout.ExerciseSet) []workout.Set {
	// Determine the current type of workout based on rep ranges
	currentType := determineWorkoutType(lastExerciseSet.Sets)

	// Check if all sets were completed at maximum reps
	allCompletedAtMax := allSetsCompletedAtMax(lastExerciseSet.Sets)

	// Count consecutive workouts with all sets at max reps
	consecutiveMaxCompletions := 0
	if allCompletedAtMax {
		consecutiveMaxCompletions = 1 // Current workout counts as one
		exerciseID := lastExerciseSet.Exercise.ID

		// Look at previous workouts for this exercise
		for i := len(g.history) - 1; i >= 0; i-- {
			session := g.history[i]
			for _, es := range session.ExerciseSets {
				if es.Exercise.ID == exerciseID {
					if allSetsCompletedAtMax(es.Sets) {
						consecutiveMaxCompletions++
						if consecutiveMaxCompletions >= 2 {
							break
						}
					} else {
						// Break at first non-max completion
						break
					}
				}
			}
			if consecutiveMaxCompletions >= 2 {
				break
			}
		}
	}

	// Progress to next workout type if completed at max for 2 consecutive workouts
	if consecutiveMaxCompletions >= 2 {
		return createSetsForNextWorkoutType(currentType, lastExerciseSet.Sets[0].WeightKg)
	}

	// Otherwise, keep same workout type with adjusted weight
	if allCompletedAtMax {
		// If all sets completed at max, increase weight slightly
		return increaseWeight(lastExerciseSet.Sets, 2.5)
	}
	// Otherwise keep the same weight and rep ranges
	return copySetWithoutCompletion(lastExerciseSet.Sets)
}

// determineWorkoutType identifies the type of workout based on rep ranges.
func determineWorkoutType(sets []workout.Set) string {
	if len(sets) == 0 {
		return "hypertrophy" // Default
	}

	// Check the rep range of the first set
	minReps := sets[0].MinReps
	maxReps := sets[0].MaxReps

	if minReps >= 3 && maxReps <= 6 {
		return "strength"
	} else if minReps >= 8 && maxReps <= 12 {
		return "hypertrophy"
	} else if minReps >= 12 && maxReps <= 15 {
		return "endurance"
	}

	return "hypertrophy" // Default
}

// allSetsCompletedAtMax checks if all sets were completed at maximum reps.
func allSetsCompletedAtMax(sets []workout.Set) bool {
	for _, set := range sets {
		if set.CompletedReps == nil || *set.CompletedReps < set.MaxReps {
			return false
		}
	}
	return true
}

// createSetsForNextWorkoutType creates sets for the next workout type in the cycle.
func createSetsForNextWorkoutType(currentType string, lastWeight float64) []workout.Set {
	switch currentType {
	case "strength":
		// Progress from strength to hypertrophy
		return []workout.Set{
			{WeightKg: lastWeight * 0.8, AdjustedWeightKg: lastWeight * 0.8, MinReps: 8, MaxReps: 12},
			{WeightKg: lastWeight * 0.8, AdjustedWeightKg: lastWeight * 0.8, MinReps: 8, MaxReps: 12},
			{WeightKg: lastWeight * 0.8, AdjustedWeightKg: lastWeight * 0.8, MinReps: 8, MaxReps: 12},
		}
	case "hypertrophy":
		// Progress from hypertrophy to endurance
		return []workout.Set{
			{WeightKg: lastWeight * 0.7, AdjustedWeightKg: lastWeight * 0.7, MinReps: 12, MaxReps: 15},
			{WeightKg: lastWeight * 0.7, AdjustedWeightKg: lastWeight * 0.7, MinReps: 12, MaxReps: 15},
			{WeightKg: lastWeight * 0.7, AdjustedWeightKg: lastWeight * 0.7, MinReps: 12, MaxReps: 15},
		}
	case "endurance":
		// Progress from endurance to strength
		return []workout.Set{
			{WeightKg: lastWeight * 1.3, AdjustedWeightKg: lastWeight * 1.3, MinReps: 3, MaxReps: 6},
			{WeightKg: lastWeight * 1.3, AdjustedWeightKg: lastWeight * 1.3, MinReps: 3, MaxReps: 6},
			{WeightKg: lastWeight * 1.3, AdjustedWeightKg: lastWeight * 1.3, MinReps: 3, MaxReps: 6},
		}
	default:
		// Default to hypertrophy
		return []workout.Set{
			{WeightKg: lastWeight, AdjustedWeightKg: lastWeight, MinReps: 8, MaxReps: 12},
			{WeightKg: lastWeight, AdjustedWeightKg: lastWeight, MinReps: 8, MaxReps: 12},
			{WeightKg: lastWeight, AdjustedWeightKg: lastWeight, MinReps: 8, MaxReps: 12},
		}
	}
}

// integrateUserFeedback adjusts workout intensity based on user feedback.
func (g *Generator) integrateUserFeedback(sets []workout.Set, feedback *int) []workout.Set {
	if feedback == nil {
		return sets // No feedback, no adjustment
	}

	switch *feedback {
	case 1: // Too easy
		// Increase intensity more aggressively
		return increaseWeight(sets, 5.0) // Add 5kg
	case 5: // Too difficult
		// Reduce volume or intensity
		if len(sets) > 3 {
			// Reduce volume by removing a set
			return sets[:len(sets)-1]
		}
		// Reduce intensity by reducing weight
		return reduceWeight(sets, 0.1)
	default: // 2-4 (optimal challenge)
		// Follow standard progression
		return sets
	}
}

// getMostRecentFeedback gets the most recent feedback for a session containing the specified exercise.
func (g *Generator) getMostRecentFeedback(exerciseID int) *int {
	var mostRecentSession *workout.Session
	var mostRecentDate time.Time

	for i, session := range g.history {
		// Check if this session contains the exercise and has a difficulty rating
		containsExercise := false
		for _, es := range session.ExerciseSets {
			if es.Exercise.ID == exerciseID {
				containsExercise = true
				break
			}
		}

		if containsExercise && session.DifficultyRating != nil &&
			(mostRecentSession == nil || session.WorkoutDate.After(mostRecentDate)) {
			mostRecentSession = &g.history[i]
			mostRecentDate = session.WorkoutDate
		}
	}

	if mostRecentSession != nil {
		return mostRecentSession.DifficultyRating
	}

	return nil
}

// copySetWithoutCompletion creates a copy of sets without completed reps.
func copySetWithoutCompletion(sets []workout.Set) []workout.Set {
	newSets := make([]workout.Set, len(sets))
	for i, set := range sets {
		newSets[i] = workout.Set{
			WeightKg:         set.WeightKg,
			AdjustedWeightKg: set.AdjustedWeightKg,
			MinReps:          set.MinReps,
			MaxReps:          set.MaxReps,
			CompletedReps:    nil, // Reset completion
		}
	}
	return newSets
}

// reduceWeight reduces the weight by a percentage.
func reduceWeight(sets []workout.Set, percentage float64) []workout.Set {
	newSets := make([]workout.Set, len(sets))
	for i, set := range sets {
		reduction := set.WeightKg * percentage
		newWeight := set.WeightKg - reduction
		if newWeight < 0 {
			newWeight = 0
		}

		newSets[i] = workout.Set{
			WeightKg:         newWeight,
			AdjustedWeightKg: newWeight,
			MinReps:          set.MinReps,
			MaxReps:          set.MaxReps,
			CompletedReps:    nil, // Reset completion
		}
	}
	return newSets
}

// increaseWeight increases the weight by a fixed amount.
func increaseWeight(sets []workout.Set, increment float64) []workout.Set {
	newSets := make([]workout.Set, len(sets))
	for i, set := range sets {
		newWeight := set.WeightKg + increment

		newSets[i] = workout.Set{
			WeightKg:         newWeight,
			AdjustedWeightKg: newWeight,
			MinReps:          set.MinReps,
			MaxReps:          set.MaxReps,
			CompletedReps:    nil, // Reset completion
		}
	}
	return newSets
}
