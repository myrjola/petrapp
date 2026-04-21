package weekplanner

import (
	"math/rand/v2"
	"time"
)

// Category is the workout focus for a session.
type Category string

const (
	CategoryFullBody Category = "full_body"
	CategoryUpper    Category = "upper"
	CategoryLower    Category = "lower"
)

// ExerciseType distinguishes weighted from bodyweight exercises.
type ExerciseType string

const (
	ExerciseTypeWeighted   ExerciseType = "weighted"
	ExerciseTypeBodyweight ExerciseType = "bodyweight"
)

// PeriodizationType controls rep targets for the session.
type PeriodizationType int

const (
	PeriodizationStrength    PeriodizationType = 0 // 5 reps
	PeriodizationHypertrophy PeriodizationType = 1 // 6-10 reps
)

const (
	setsPerExercise    = 3
	minRepsStrength    = 5
	maxRepsStrength    = 5
	minRepsHypertrophy = 6
	maxRepsHypertrophy = 10
)

// Preferences describes which days are workout days and their duration in minutes.
// A value of 0 means rest day; 45, 60, or 90 means workout day.
type Preferences struct {
	MondayMinutes    int
	TuesdayMinutes   int
	WednesdayMinutes int
	ThursdayMinutes  int
	FridayMinutes    int
	SaturdayMinutes  int
	SundayMinutes    int
}

func (p Preferences) minutesForDay(weekday time.Weekday) int {
	switch weekday {
	case time.Monday:
		return p.MondayMinutes
	case time.Tuesday:
		return p.TuesdayMinutes
	case time.Wednesday:
		return p.WednesdayMinutes
	case time.Thursday:
		return p.ThursdayMinutes
	case time.Friday:
		return p.FridayMinutes
	case time.Saturday:
		return p.SaturdayMinutes
	case time.Sunday:
		return p.SundayMinutes
	default:
		return 0
	}
}

// IsWorkoutDay returns true if the given weekday has a non-zero duration in preferences.
func (p Preferences) IsWorkoutDay(weekday time.Weekday) bool {
	return p.minutesForDay(weekday) > 0
}

// ExercisesPerSession returns how many exercises to include based on session duration.
func (p Preferences) ExercisesPerSession(weekday time.Weekday) int {
	switch minutes := p.minutesForDay(weekday); {
	case minutes >= 90:
		return 4
	case minutes >= 60:
		return 3
	case minutes > 0:
		return 2
	default:
		return 0
	}
}

// Exercise is a dependency-free representation of an exercise for planning.
// StartingWeightKg is intentionally absent — resolved lazily by exerciseprogression.
type Exercise struct {
	ID                    int
	Category              Category
	ExerciseType          ExerciseType
	PrimaryMuscleGroups   []string
	SecondaryMuscleGroups []string
}

// MuscleGroupTarget holds the minimum weekly set target for a tracked muscle group.
type MuscleGroupTarget struct {
	Name            string
	WeeklySetTarget int
}

// PlannedSession is the output of Plan() for a single workout day.
type PlannedSession struct {
	Date              time.Time
	Category          Category
	PeriodizationType PeriodizationType
	ExerciseSets      []PlannedExerciseSet
}

// PlannedExerciseSet groups the planned sets for one exercise.
type PlannedExerciseSet struct {
	ExerciseID int
	Sets       []PlannedSet
}

// PlannedSet holds rep targets only; WeightKg is always nil at plan time.
type PlannedSet struct {
	MinReps int
	MaxReps int
}

// WeeklyPlanner holds the static inputs needed to plan a full week of workouts.
type WeeklyPlanner struct {
	Prefs     Preferences
	Exercises []Exercise
	Targets   []MuscleGroupTarget
	rng       *rand.Rand
}

// NewWeeklyPlanner creates a WeeklyPlanner with a randomly seeded RNG.
func NewWeeklyPlanner(prefs Preferences, exercises []Exercise, targets []MuscleGroupTarget) *WeeklyPlanner {
	return &WeeklyPlanner{
		Prefs:     prefs,
		Exercises: exercises,
		Targets:   targets,
		rng:       rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)),
	}
}

// determineCategory returns the workout category for a given date using the adjacency rule.
// Uses preference-based weekday checks so week boundaries wrap naturally through date arithmetic:
// Sunday's "tomorrow" is Monday, Monday's "yesterday" is Sunday.
func (wp *WeeklyPlanner) determineCategory(date time.Time) Category {
	today := date.Weekday()
	tomorrow := date.AddDate(0, 0, 1).Weekday()
	yesterday := date.AddDate(0, 0, -1).Weekday()

	if wp.Prefs.IsWorkoutDay(today) && wp.Prefs.IsWorkoutDay(tomorrow) {
		return CategoryLower
	}
	if wp.Prefs.IsWorkoutDay(yesterday) {
		return CategoryUpper
	}
	return CategoryFullBody
}
