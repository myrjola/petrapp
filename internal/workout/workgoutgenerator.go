package workout

import (
	"time"
)

// workoutGenerator handles workout generation and progression logic.
type workoutGenerator struct {
	// exercisePool contains all available exercises from the database
	exercisePool []Exercise
	// userHistory contains the user's previous workout performance
	userHistory []completedWorkout
	// preferences contains user preferences for workout days
	preferences Preferences
}

// workoutPlan represents a generated workout before it's saved as a Session.
type workoutPlan struct {
	// date is the scheduled date for this workout
	date time.Time
	// category indicates whether this is an upper, lower, or full body workout
	category Category
	// exercises contains the exercises selected for this workout with their planned sets
	exercises []plannedExercise
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

// exerciseHistory represents performance history for a specific exercise.
type exerciseHistory struct {
	// exercise contains the exercise details
	exercise Exercise
	// lastPerformed is the date when this exercise was last done
	lastPerformed time.Time
	// performanceData contains historical set performance data
	performanceData []setPerformance
}

// setPerformance tracks the performance of a set.
type setPerformance struct {
	// date is when this set was performed
	date time.Time
	// weightKg is the weight used
	weightKg float64
	// targetReps is how many reps were planned
	targetReps int
	// completedReps is how many reps were actually completed
	completedReps int
}

// completedWorkout represents a finished workout session.
type completedWorkout struct {
	// date when this workout was completed
	date time.Time
	// category indicates whether this was an upper, lower, or full body workout.
	category Category
	// difficultyRating is the user's feedback on difficulty (1-5).
	difficultyRating int
	// completedExercises contains the exercises performed with their results.
	completedExercises []completedExercise
}

// completedExercise represents a finished exercise with performance data.
type completedExercise struct {
	// exercise contains the exercise details.
	exercise Exercise
	// sets contains the performance data for each set.
	sets []setPerformance
}

// workoutSplit determines the type of workout for each training day.
type workoutSplit struct {
	// scheduledDays contains the weekdays when workouts are scheduled.
	scheduledDays []time.Weekday
	// lastWorkoutCategory is the category of the previous workout.
	lastWorkoutCategory Category
	// consecutiveDay indicates if this workout follows another with no rest day.
	consecutiveDay bool
}

// exerciseSelectionCriteria guides exercise selection for a workout.
type exerciseSelectionCriteria struct {
	// category is the workout type, e.g., upper, lower, and full_body.
	category Category
	// recentExercises maps exercises to when they were last performed.
	recentExercises map[string]time.Time
	// requiredMuscleGroups indicates muscle groups that should be worked.
	requiredMuscleGroups []string
	// excludeExercises contains exercises to avoid in this workout.
	excludeExercises []Exercise
}

// muscleGroupBalance tracks balance across multiple workouts.
type muscleGroupBalance struct {
	// primary tracks how often primary muscle groups are trained.
	primary map[string]int
	// secondary tracks how often secondary muscle groups are trained.
	secondary map[string]int
}

// progressionModel determines how to progress weights and reps.
type progressionModel interface {
	// CalculateNextWorkload determines the next set of weights and reps
	// based on exercise history and user feedback.
	calculateNextWorkload(history exerciseHistory, lastFeedback int) ([]plannedSet, error)
}

// linearProgressionModel implements simple progressive overload.
type linearProgressionModel struct {
	// weightIncrementPercent is how much to increase weight (e.g., 0.05 for 5%).
	weightIncrementPercent float64
	// minWeightIncrement is the smallest possible weight increment (e.g., 2.5kg).
	minWeightIncrement float64
	// repProgressionThreshold is reps needed before increasing weight.
	repProgressionThreshold int
}
