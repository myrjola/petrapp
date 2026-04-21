package weekplanner

import (
	"math/rand/v2"
	"slices"
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

// exercisesPerWeek sums the exercise count across all scheduled days.
func (wp *WeeklyPlanner) exercisesPerWeek() int {
	total := 0
	for _, wd := range []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday, time.Sunday,
	} {
		total += wp.Prefs.ExercisesPerSession(wd)
	}
	return total
}

// firstSessionPeriodizationType derives the periodization type for the first session of the
// week deterministically from the start date and preferences — no DB query needed.
func (wp *WeeklyPlanner) firstSessionPeriodizationType(startingDate time.Time) PeriodizationType {
	const secondsPerWeek = 7 * 24 * 3600
	weeksSinceEpoch := startingDate.Unix() / secondsPerWeek
	epw := int64(wp.exercisesPerWeek())
	if epw == 0 {
		return PeriodizationStrength
	}
	if (weeksSinceEpoch*epw)%2 == 0 {
		return PeriodizationStrength
	}
	return PeriodizationHypertrophy
}

// isCategoryCompatible reports whether an exercise of exerciseCategory can be
// used on a day with dayCategory.
//   - Full Body days accept all exercise categories.
//   - Upper/Lower days only accept their matching exercise category.
func isCategoryCompatible(exerciseCategory, dayCategory Category) bool {
	if dayCategory == CategoryFullBody {
		return true
	}
	return exerciseCategory == dayCategory
}

// hasCategoryExerciseForMuscleGroup reports whether the pool contains at least
// one exercise compatible with dayCategory whose primary muscles include muscleGroup.
func (wp *WeeklyPlanner) hasCategoryExerciseForMuscleGroup(dayCategory Category, muscleGroup string) bool {
	for _, ex := range wp.Exercises {
		if !isCategoryCompatible(ex.Category, dayCategory) {
			continue
		}
		for _, mg := range ex.PrimaryMuscleGroups {
			if mg == muscleGroup {
				return true
			}
		}
	}
	return false
}

// allocateMuscleGroups assigns each tracked muscle group to up to 2 workout days
// using a most-constrained-first greedy algorithm. A muscle group is valid for a
// day if at least one compatible exercise targets it as a primary muscle.
func (wp *WeeklyPlanner) allocateMuscleGroups(
	workoutDays []time.Time,
	categories map[time.Time]Category,
) map[time.Time][]string {
	// Build valid-day lists for each muscle group.
	type mgEntry struct {
		name      string
		validDays []time.Time
	}
	entries := make([]mgEntry, len(wp.Targets))
	for i, target := range wp.Targets {
		var valid []time.Time
		for _, day := range workoutDays {
			if wp.hasCategoryExerciseForMuscleGroup(categories[day], target.Name) {
				valid = append(valid, day)
			}
		}
		entries[i] = mgEntry{name: target.Name, validDays: valid}
	}

	// Sort ascending by number of valid days (most constrained first).
	// Alphabetical name as tiebreaker for determinism.
	slices.SortFunc(entries, func(a, b mgEntry) int {
		if len(a.validDays) != len(b.validDays) {
			return len(a.validDays) - len(b.validDays)
		}
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		return 0
	})

	assignmentCount := make(map[time.Time]int)
	result := make(map[time.Time][]string)

	for _, entry := range entries {
		if len(entry.validDays) == 0 {
			continue
		}

		// Sort valid days by current assignment count (least loaded first).
		sortedDays := slices.Clone(entry.validDays)
		slices.SortFunc(sortedDays, func(a, b time.Time) int {
			return assignmentCount[a] - assignmentCount[b]
		})

		// Assign to up to 2 days.
		limit := min(2, len(sortedDays))
		for i := range limit {
			day := sortedDays[i]
			result[day] = append(result[day], entry.name)
			assignmentCount[day]++
		}
	}

	return result
}

// setsForPeriodization returns MinReps/MaxReps for a PlannedSet based on periodization type.
func setsForPeriodization(pt PeriodizationType) (minReps, maxReps int) {
	if pt == PeriodizationStrength {
		return minRepsStrength, maxRepsStrength
	}
	return minRepsHypertrophy, maxRepsHypertrophy
}

// scoreExercise returns how many of the priority muscle groups the exercise covers
// via primary muscle groups and are not yet satisfied.
func scoreExercise(ex Exercise, priority []string, satisfied map[string]bool) int {
	score := 0
	for _, mg := range ex.PrimaryMuscleGroups {
		for _, p := range priority {
			if mg == p && !satisfied[mg] {
				score++
			}
		}
	}
	return score
}

// selectExercisesForDay picks n exercises for a day via category-filtered, score-based
// greedy selection. Uses Strength periodization by default.
func (wp *WeeklyPlanner) selectExercisesForDay(
	category Category,
	priorityMuscleGroups []string,
	n int,
) []PlannedExerciseSet {
	return wp.selectExercisesForDayWithPeriodization(category, priorityMuscleGroups, n, PeriodizationStrength)
}

func (wp *WeeklyPlanner) selectExercisesForDayWithPeriodization(
	category Category,
	priorityMuscleGroups []string,
	n int,
	pt PeriodizationType,
) []PlannedExerciseSet {
	// Filter exercise pool by category compatibility.
	pool := make([]Exercise, 0, len(wp.Exercises))
	for _, ex := range wp.Exercises {
		if isCategoryCompatible(ex.Category, category) {
			pool = append(pool, ex)
		}
	}

	satisfied := make(map[string]bool)
	var selected []Exercise

	for len(selected) < n && len(pool) > 0 {
		// Find best score among remaining pool.
		bestScore := -1
		for _, ex := range pool {
			if s := scoreExercise(ex, priorityMuscleGroups, satisfied); s > bestScore {
				bestScore = s
			}
		}

		// Collect all exercises with best score.
		var candidates []int
		for i, ex := range pool {
			if scoreExercise(ex, priorityMuscleGroups, satisfied) == bestScore {
				candidates = append(candidates, i)
			}
		}

		// Pick one at random from best candidates.
		chosen := candidates[wp.rng.IntN(len(candidates))]
		ex := pool[chosen]
		selected = append(selected, ex)

		// Mark primary muscle groups satisfied.
		for _, mg := range ex.PrimaryMuscleGroups {
			satisfied[mg] = true
		}

		// Remove chosen from pool.
		pool = append(pool[:chosen], pool[chosen+1:]...)
	}

	// Build PlannedExerciseSets.
	minR, maxR := setsForPeriodization(pt)
	sets := make([]PlannedSet, setsPerExercise)
	for i := range sets {
		sets[i] = PlannedSet{MinReps: minR, MaxReps: maxR}
	}

	result := make([]PlannedExerciseSet, len(selected))
	for i, ex := range selected {
		result[i] = PlannedExerciseSet{
			ExerciseID: ex.ID,
			Sets:       slices.Clone(sets),
		}
	}
	return result
}
