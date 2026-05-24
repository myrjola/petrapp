package domain

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"
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

// errNoExercisesForCategory is returned by PlanDay (and wrapped by Plan) when
// the exercise pool contains nothing compatible with the derived category.
var errNoExercisesForCategory = errors.New("no exercises available for day category")

// Planner holds the static inputs needed to plan a full week of workouts.
type Planner struct {
	Prefs     Preferences
	Exercises []Exercise
	Targets   []MuscleGroupTarget
	rng       *rand.Rand
}

// NewPlanner creates a Planner with a randomly seeded RNG.
func NewPlanner(prefs Preferences, exercises []Exercise, targets []MuscleGroupTarget) *Planner {
	// Non-cryptographic randomness is intentional for exercise selection.
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)) //nolint:gosec // not security-sensitive
	return &Planner{
		Prefs:     prefs,
		Exercises: exercises,
		Targets:   targets,
		rng:       rng,
	}
}

// Plan generates one Session per scheduled workout day for the week beginning on
// startingDate. Returns an error if startingDate is not a Monday, if no workout days are
// scheduled, or if a scheduled day has no compatible exercises.
func (wp *Planner) Plan(startingDate time.Time) ([]Session, error) {
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
			return nil, fmt.Errorf("%w: %s day (%s)", errNoExercisesForCategory, cat, day.Weekday())
		}
		categories[day] = cat
	}

	// Phase 2: allocate muscle group slots across days.
	dayMuscleGroups := wp.allocateMuscleGroups(workoutDays, categories)

	// Determine periodization type for first session.
	firstPT := wp.firstSessionPeriodizationType(startingDate)
	isDeload := IsDeloadWeek(startingDate, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled)

	// Phase 3: select exercises and build sessions.
	weekUsedExercises := make(map[int]bool)
	sessions := make([]Session, len(workoutDays))
	for i, day := range workoutDays {
		pt := nextPeriodizationType(firstPT, i)
		if isDeload {
			pt = PeriodizationHypertrophy
		}
		n := exercisesPerSession(wp.Prefs, day.Weekday())
		exerciseSets := wp.selectExercisesForDayWithPeriodization(
			categories[day],
			dayMuscleGroups[day],
			n,
			pt,
			isDeload,
			weekUsedExercises,
		)

		// Record which exercises were used this week.
		for _, es := range exerciseSets {
			weekUsedExercises[es.Exercise.ID] = true
		}

		sessions[i] = Session{ //nolint:exhaustruct // DifficultyRating, StartedAt, CompletedAt start zero.
			Date:              day,
			PeriodizationType: pt,
			IsDeload:          isDeload,
			ExerciseSets:      exerciseSets,
		}
	}

	return sessions, nil
}

// PlanDay generates one Session for date, suitable for ad-hoc workouts on
// days outside the weekly plan (extra workouts, or days added mid-week after
// Plan(monday) already ran). weekUsedExerciseIDs is the set of exercise IDs
// already used in other sessions this week; the planner avoids repeating
// them when possible. Returns errNoExercisesForCategory (wrapped) if the
// derived category has no compatible exercises.
func (wp *Planner) PlanDay(date time.Time, weekUsedExerciseIDs map[int]bool) (Session, error) {
	category := wp.determineCategory(date)
	if !wp.hasExercisesForCategory(category) {
		return Session{}, fmt.Errorf(
			"%w: %s day (%s)", errNoExercisesForCategory, category, date.Weekday())
	}

	// Exercise count: from prefs if the day is scheduled, otherwise medium.
	n := exercisesPerSession(wp.Prefs, date.Weekday())
	if n == 0 {
		n = exercisesMedium
	}

	// Periodization: replicate the weekly planner's per-day alternation.
	// Count scheduled prefs days strictly before date.Weekday() in Mon-first
	// week order. Iterating Mon..Sat explicitly (rather than as an int range)
	// handles Sunday correctly: time.Sunday = 0 < time.Monday = 1, so an int
	// range would never count anything for a Sunday date.
	idx := 0
	target := date.Weekday()
	for _, d := range []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday,
	} {
		if d == target {
			break
		}
		if wp.Prefs.IsWorkoutDay(d) {
			idx++
		}
	}
	// Sunday never matches any d above, so it falls through with the full count
	// of scheduled Mon..Sat days — exactly the index workoutDays[i==len-1] would
	// have produced for it.
	monday := MondayOf(date)
	firstPT := wp.firstSessionPeriodizationType(monday)
	pt := nextPeriodizationType(firstPT, idx)

	isDeload := IsDeloadWeek(monday, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled)
	if isDeload {
		pt = PeriodizationHypertrophy
	}

	used := weekUsedExerciseIDs
	if used == nil {
		used = make(map[int]bool)
	}
	exerciseSets := wp.selectExercisesForDayWithPeriodization(
		category, nil, n, pt, isDeload, used,
	)

	return Session{ //nolint:exhaustruct // DifficultyRating, StartedAt, CompletedAt start zero.
		Date:              date,
		PeriodizationType: pt,
		IsDeload:          isDeload,
		ExerciseSets:      exerciseSets,
	}, nil
}

// exercisesPerSession returns how many exercises to include based on session duration.
func exercisesPerSession(prefs Preferences, weekday time.Weekday) int {
	switch minutes := prefs.MinutesForDay(weekday); {
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

// determineCategory returns the workout category for a given date using the adjacency rule.
// Uses preference-based weekday checks so week boundaries wrap naturally through date arithmetic:
// Sunday's "tomorrow" is Monday, Monday's "yesterday" is Sunday.
// Lower is chosen when tomorrow is a workout day (whether today is scheduled or ad-hoc), so that
// the following session can use Upper-body exercises while the legs recover. Upper is chosen when
// yesterday was a workout day. Otherwise FullBody.
func (wp *Planner) determineCategory(date time.Time) Category {
	tomorrow := date.AddDate(0, 0, 1).Weekday()
	yesterday := date.AddDate(0, 0, -1).Weekday()

	if wp.Prefs.IsWorkoutDay(tomorrow) {
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
func (wp *Planner) exercisesPerWeek() int {
	total := 0
	for _, wd := range []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday, time.Sunday,
	} {
		total += exercisesPerSession(wp.Prefs, wd)
	}
	return total
}

// firstSessionPeriodizationType derives the periodization type for the first session of the
// week deterministically from the start date and preferences — no DB query needed.
func (wp *Planner) firstSessionPeriodizationType(startingDate time.Time) PeriodizationType {
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
func (wp *Planner) hasCategoryExerciseForMuscleGroup(dayCategory Category, muscleGroup string) bool {
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
func (wp *Planner) allocateMuscleGroups(
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
			if wp.hasCategoryExerciseForMuscleGroup(categories[day], target.MuscleGroupName) {
				valid = append(valid, day)
			}
		}
		entries[i] = mgEntry{name: target.MuscleGroupName, validDays: valid}
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
func (wp *Planner) findBestExerciseInPool(
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

// selectExercisesForDay picks n exercises for a day via category-filtered, score-based
// greedy selection. Uses Strength periodization by default.
func (wp *Planner) selectExercisesForDay(
	category Category,
	priorityMuscleGroups []string,
	n int,
) []ExerciseSet {
	return wp.selectExercisesForDayWithPeriodization(
		category,
		priorityMuscleGroups,
		n,
		PeriodizationStrength,
		false,
		make(map[int]bool),
	)
}

func (wp *Planner) selectExercisesForDayWithPeriodization(
	category Category,
	priorityMuscleGroups []string,
	n int,
	pt PeriodizationType,
	isDeload bool,
	weekUsedExercises map[int]bool,
) []ExerciseSet {
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

	// Build ExerciseSets. Time-based exercises use their own
	// DefaultStartingSeconds with a fixed set count; rep-based exercises
	// derive their full prescription from the per-exercise window via DeriveScheme.
	result := make([]ExerciseSet, len(selected))
	for i, ex := range selected {
		result[i] = buildPlannedExerciseSet(ex, pt, isDeload)
	}
	return result
}

// buildPlannedExerciseSet creates an ExerciseSet for one exercise using
// BuildPlannedSets as the single source of truth for set prescription.
func buildPlannedExerciseSet(ex Exercise, pt PeriodizationType, isDeload bool) ExerciseSet {
	return ExerciseSet{ //nolint:exhaustruct // ID auto-assigned at insert; WarmupCompletedAt nil.
		Exercise: ex,
		Sets:     BuildPlannedSets(ex, pt, isDeload),
	}
}

// hasExercisesForCategory reports whether the exercise pool contains at least one
// exercise compatible with the given day category.
func (wp *Planner) hasExercisesForCategory(category Category) bool {
	for _, ex := range wp.Exercises {
		if isCategoryCompatible(ex.Category, category) {
			return true
		}
	}
	return false
}

// nextPeriodizationType cycles between PeriodizationStrength and PeriodizationHypertrophy.
// It uses index-based alternation: even indices get the first type, odd indices get the second.
func nextPeriodizationType(first PeriodizationType, idx int) PeriodizationType {
	if idx%numPeriodizationTypes == 0 {
		return first
	}
	if first == PeriodizationStrength {
		return PeriodizationHypertrophy
	}
	return PeriodizationStrength
}

// MondayOf returns the Monday of the week containing date, at 00:00 UTC.
// The calendar date is taken from date's own location so the user's local
// week boundary is preserved, but the result is anchored to UTC so it
// compares cleanly against session dates loaded from the database (which
// time.Parse always returns in UTC). time.Truncate is unsafe here because
// it rounds to UTC-midnight boundaries from an absolute instant, which can
// roll local-timezone times back into the previous calendar day.
func MondayOf(date time.Time) time.Time {
	y, m, d := date.Date()
	offset := int(time.Monday - date.Weekday())
	if offset > 0 {
		offset = -6
	}
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, offset)
}

// StartOfDay returns the UTC midnight of date's calendar day. Mirrors
// MondayOf's UTC-anchored-but-calendar-date-from-local behaviour so the
// result compares cleanly against session dates loaded from the database
// (which time.Parse always returns in UTC).
func StartOfDay(date time.Time) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
