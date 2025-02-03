package workout

import (
	"time"
)

// Category represents the type of exercise.
type Category string

const (
	CategoryFullBody Category = "full_body"
	CategoryUpper    Category = "upper"
	CategoryLower    Category = "lower"
)

// Exercise represents a single exercise type, e.g. Squat, Bench Press, etc.
type Exercise struct {
	ID       int
	Name     string
	Category Category
}

// Set represents a single set of an exercise with target and actual performance.
type Set struct {
	WeightKg         float64
	AdjustedWeightKg float64
	MinReps          int
	MaxReps          int
	CompletedReps    *int
}

// ExerciseSet groups all sets for a specific exercise in a workout.
type ExerciseSet struct {
	Exercise Exercise
	Sets     []Set
}

// WorkoutStatus represents the state of a workout for a specific day.
type WorkoutStatus string

const (
	WorkoutStatusDone    WorkoutStatus = "Done"
	WorkoutStatusSkipped WorkoutStatus = "Skipped"
	WorkoutStatusRest    WorkoutStatus = "Rest day"
	WorkoutStatusPlanned WorkoutStatus = "Planned"
)

// WorkoutSession represents a complete workout session including all exercises and their sets.
type WorkoutSession struct {
	WorkoutDate      time.Time
	DifficultyRating *int
	StartedAt        *time.Time
	CompletedAt      *time.Time
	ExerciseSets     []ExerciseSet
	Status           WorkoutStatus
}

// WorkoutPreferences stores which days of the week a user wants to work out.
type WorkoutPreferences struct {
	Monday    bool
	Tuesday   bool
	Wednesday bool
	Thursday  bool
	Friday    bool
	Saturday  bool
	Sunday    bool
}
