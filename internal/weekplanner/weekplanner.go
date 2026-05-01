package weekplanner

import (
	"errors"
	"fmt"
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
	ExerciseTypeAssisted   ExerciseType = "assisted"
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

const (
	minutesLong   = 90
	minutesMedium = 60

	exercisesLong   = 4
	exercisesMedium = 3
	exercisesShort  = 2

	maxMuscleGroupDaysPerWeek = 2
	numPeriodizationTypes     = 2
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
	case minutes >= minutesLong:
		return exercisesLong
	case minutes >= minutesMedium:
		return exercisesMedium
	case minutes > 0:
		return exercisesShort
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
	// Non-cryptographic randomness is intentional for exercise selection.
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)) //nolint:gosec // not security-sensitive
	return &WeeklyPlanner{
		Prefs:     prefs,
		Exercises: exercises,
		Targets:   targets,
		rng:       rng,
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
//
//nolint:unused // kept for future extensibility.
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
	if weeksSinceEpoch%2 == 0 {
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
		if slices.Contains(ex.PrimaryMuscleGroups, muscleGroup) {
			return true
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

		// Assign to up to maxMuscleGroupDaysPerWeek days.
		limit := min(maxMuscleGroupDaysPerWeek, len(sortedDays))
		for i := range limit {
			day := sortedDays[i]
			result[day] = append(result[day], entry.name)
			assignmentCount[day]++
		}
	}

	return result
}

// setsForPeriodization returns MinReps/MaxReps for a PlannedSet based on periodization type.
func setsForPeriodization(pt PeriodizationType) (int, int) {
	if pt == PeriodizationStrength {
		return minRepsStrength, maxRepsStrength
	}
	return minRepsHypertrophy, maxRepsHypertrophy
}

// primaryMuscleGroupsOverlap returns true if any of the exercise's primary muscle groups
// are already in the selectedPrimaryMuscles set.
func primaryMuscleGroupsOverlap(ex Exercise, selectedPrimaryMuscles map[string]bool) bool {
	for _, mg := range ex.PrimaryMuscleGroups {
		if selectedPrimaryMuscles[mg] {
			return true
		}
	}
	return false
}

// selectExercisesForDay picks n exercises for a day via category-filtered, score-based
// scoreExerciseForPriority scores an exercise by how many unsatisfied priority muscle groups it covers.
func scoreExerciseForPriority(ex Exercise, priorityMuscleGroups []string, satisfied map[string]bool) int {
	score := 0
	for _, mg := range ex.PrimaryMuscleGroups {
		if slices.Contains(priorityMuscleGroups, mg) && !satisfied[mg] {
			score++
		}
	}
	return score
}

// findBestExerciseInPool finds the highest-scoring exercise from the pool that doesn't conflict.
// Returns the index in pool, or -1 if no suitable exercise found.
func (wp *WeeklyPlanner) findBestExerciseInPool(
	pool []Exercise,
	priorityMuscleGroups []string,
	selectedPrimaryMuscles map[string]bool,
	weekUsedExercises map[int]bool,
) int {
	bestScore := -1
	bestIdx := -1

	for i := range pool {
		if weekUsedExercises[pool[i].ID] || primaryMuscleGroupsOverlap(pool[i], selectedPrimaryMuscles) {
			continue
		}

		score := scoreExerciseForPriority(pool[i], priorityMuscleGroups, selectedPrimaryMuscles)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return bestIdx
}

// selectAndRemoveFromPool selects an exercise from the pool and removes it.
func selectAndRemoveFromPool(pool *[]Exercise, idx int, selectedPrimaryMuscles map[string]bool) Exercise {
	ex := (*pool)[idx]
	for _, mg := range ex.PrimaryMuscleGroups {
		selectedPrimaryMuscles[mg] = true
	}
	*pool = append((*pool)[:idx], (*pool)[idx+1:]...)
	return ex
}

// greedy selection. Uses Strength periodization by default.
func (wp *WeeklyPlanner) selectExercisesForDay(
	category Category,
	priorityMuscleGroups []string,
	n int,
) []PlannedExerciseSet {
	return wp.selectExercisesForDayWithPeriodization(
		category,
		priorityMuscleGroups,
		n,
		PeriodizationStrength,
		make(map[int]bool),
	)
}

func (wp *WeeklyPlanner) selectExercisesForDayWithPeriodization(
	category Category,
	priorityMuscleGroups []string,
	n int,
	pt PeriodizationType,
	weekUsedExercises map[int]bool,
) []PlannedExerciseSet {
	// Filter exercise pool by category compatibility.
	pool := make([]Exercise, 0, len(wp.Exercises))
	for _, ex := range wp.Exercises {
		if isCategoryCompatible(ex.Category, category) {
			pool = append(pool, ex)
		}
	}

	selectedPrimaryMuscles := make(map[string]bool)
	var selected []Exercise

	// Phase A: Try to satisfy each priority muscle group with a non-conflicting exercise.
	for _, priorityMG := range priorityMuscleGroups {
		if len(selected) >= n || selectedPrimaryMuscles[priorityMG] {
			continue
		}

		bestIdx := wp.findBestExerciseInPool(pool, priorityMuscleGroups, selectedPrimaryMuscles, weekUsedExercises)
		if bestIdx >= 0 {
			ex := selectAndRemoveFromPool(&pool, bestIdx, selectedPrimaryMuscles)
			selected = append(selected, ex)
		}
	}

	// Phase B: Fill remaining slots with any non-conflicting exercise from the pool.
	for len(selected) < n && len(pool) > 0 {
		bestIdx := wp.findBestExerciseInPool(pool, priorityMuscleGroups, selectedPrimaryMuscles, weekUsedExercises)
		if bestIdx < 0 {
			break
		}

		ex := selectAndRemoveFromPool(&pool, bestIdx, selectedPrimaryMuscles)
		selected = append(selected, ex)
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

// hasExercisesForCategory reports whether the exercise pool contains at least one
// exercise compatible with the given day category.
func (wp *WeeklyPlanner) hasExercisesForCategory(category Category) bool {
	for _, ex := range wp.Exercises {
		if isCategoryCompatible(ex.Category, category) {
			return true
		}
	}
	return false
}

// Plan generates one PlannedSession per scheduled workout day for the week beginning on
// startingDate. Returns an error if startingDate is not a Monday, if no workout days are
// scheduled, or if a scheduled day has no compatible exercises.
func (wp *WeeklyPlanner) Plan(startingDate time.Time) ([]PlannedSession, error) {
	if startingDate.Weekday() != time.Monday {
		return nil, fmt.Errorf("startingDate must be a Monday, got %s", startingDate.Weekday())
	}

	// Collect scheduled workout days Mon–Sun.
	var workoutDays []time.Time
	for i := range 7 {
		day := startingDate.AddDate(0, 0, i)
		if wp.Prefs.IsWorkoutDay(day.Weekday()) {
			workoutDays = append(workoutDays, day)
		}
	}
	if len(workoutDays) == 0 {
		return nil, errors.New("no workout days scheduled in preferences")
	}

	// Phase 1: determine category for each scheduled day.
	categories := make(map[time.Time]Category, len(workoutDays))
	for _, day := range workoutDays {
		cat := wp.determineCategory(day)
		if !wp.hasExercisesForCategory(cat) {
			return nil, fmt.Errorf("no exercises available for %s day (%s)", cat, day.Weekday())
		}
		categories[day] = cat
	}

	// Phase 2: allocate muscle group slots across days.
	dayMuscleGroups := wp.allocateMuscleGroups(workoutDays, categories)

	// Determine periodization type for first session.
	firstPT := wp.firstSessionPeriodizationType(startingDate)

	// Phase 3: select exercises and build sessions.
	weekUsedExercises := make(map[int]bool)
	sessions := make([]PlannedSession, len(workoutDays))
	for i, day := range workoutDays {
		pt := PeriodizationType((int(firstPT) + i) % numPeriodizationTypes)
		n := wp.Prefs.ExercisesPerSession(day.Weekday())
		exerciseSets := wp.selectExercisesForDayWithPeriodization(
			categories[day],
			dayMuscleGroups[day],
			n,
			pt,
			weekUsedExercises,
		)

		// Record which exercises were used this week.
		for _, es := range exerciseSets {
			weekUsedExercises[es.ExerciseID] = true
		}

		sessions[i] = PlannedSession{
			Date:              day,
			Category:          categories[day],
			PeriodizationType: pt,
			ExerciseSets:      exerciseSets,
		}
	}

	return sessions, nil
}
