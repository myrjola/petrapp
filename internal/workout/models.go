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
	ID                    int
	Name                  string
	Category              Category
	DescriptionMarkdown   string
	PrimaryMuscleGroups   []string
	SecondaryMuscleGroups []string
}

// Set represents a single set of an exercise with target and actual performance.
type Set struct {
	WeightKg      float64
	MinReps       int
	MaxReps       int
	CompletedReps *int
}

// ExerciseSet groups all sets for a specific exercise in a workout.
type ExerciseSet struct {
	Exercise Exercise
	Sets     []Set
}

// Session represents a complete workout session including all exercises and their sets.
type Session struct {
	Date             time.Time
	DifficultyRating *int
	StartedAt        time.Time
	CompletedAt      time.Time
	ExerciseSets     []ExerciseSet
}

// Preferences stores which days of the week a user wants to work out.
type Preferences struct {
	Monday    bool
	Tuesday   bool
	Wednesday bool
	Thursday  bool
	Friday    bool
	Saturday  bool
	Sunday    bool
}
