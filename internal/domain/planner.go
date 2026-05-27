package domain

import (
	"errors"
	"fmt"
	"maps"
	"time"
)

const (
	minutesLong   = 90
	minutesMedium = 60

	exercisesLong              = 4
	exercisesLongHypertrophy   = 5
	exercisesMedium            = 3
	exercisesMediumHypertrophy = 4
	exercisesShort             = 2

	numPeriodizationTypes = 2

	hoursPerDay = 24
)

// errNoExercisesForCategory is returned by PlanDay (and wrapped by Plan) when
// the exercise pool contains nothing compatible with the derived category.
var errNoExercisesForCategory = errors.New("no exercises available for day category")

// Planner holds the static inputs needed to plan a full week of workouts.
type Planner struct {
	Prefs     Preferences
	Exercises []Exercise
	Targets   []MuscleGroupTarget
}

// NewPlanner creates a Planner over the supplied inputs.
func NewPlanner(prefs Preferences, exercises []Exercise, targets []MuscleGroupTarget) *Planner {
	return &Planner{
		Prefs:     prefs,
		Exercises: exercises,
		Targets:   targets,
	}
}

// Plan generates a WeekPlan for the week beginning on startingDate. The returned
// plan always has 7 Session slots indexed by day-offset from startingDate
// (slot i corresponds to startingDate.AddDate(0, 0, i)). Scheduled workout days
// are populated with full content; rest days carry an empty Session{Date: ...}
// with no Slots. Returns an error if startingDate is not a Monday, if no
// workout days are scheduled, or if a scheduled day has no compatible exercises.
func (wp *Planner) Plan(startingDate time.Time) (WeekPlan, error) {
	if startingDate.Weekday() != time.Monday {
		return WeekPlan{}, fmt.Errorf("startingDate must be a Monday, got %s", startingDate.Weekday())
	}

	result := WeekPlan{
		Monday:   startingDate,
		Sessions: [7]Session{},
	}
	for i := range 7 {
		result.Sessions[i] = Session{ //nolint:exhaustruct // Rest-day placeholder; no slots or periodization.
			Date: startingDate.AddDate(0, 0, i),
		}
	}

	var workoutDays []time.Time
	for i := range 7 {
		day := startingDate.AddDate(0, 0, i)
		if wp.Prefs.IsWorkoutDay(day.Weekday()) {
			workoutDays = append(workoutDays, day)
		}
	}
	if len(workoutDays) == 0 {
		return WeekPlan{}, errors.New("no workout days scheduled in preferences")
	}

	for _, day := range workoutDays {
		cat := wp.determineCategory(day)
		if !wp.hasExercisesForCategory(cat) {
			return WeekPlan{}, fmt.Errorf("%w: %s day (%s)", errNoExercisesForCategory, cat, day.Weekday())
		}
	}

	firstPT := wp.firstSessionPeriodizationType(startingDate)
	isDeload := IsDeloadWeek(
		startingDate, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled,
	)

	weekUsedExercises := map[int]bool{}
	load := map[string]float64{}
	for i, day := range workoutDays {
		pt := nextPeriodizationType(firstPT, i)
		if isDeload {
			pt = PeriodizationHypertrophy
		}
		n := exercisesPerSession(wp.Prefs, day.Weekday(), pt, isDeload)
		slots := wp.selectExercisesForDayWithPeriodization(
			wp.determineCategory(day), n, pt, isDeload, weekUsedExercises, load,
		)
		dayOffset := int(day.Sub(startingDate).Hours() / hoursPerDay)
		result.Sessions[dayOffset] = Session{ //nolint:exhaustruct // DifficultyRating/StartedAt/CompletedAt start zero.
			Date:              day,
			PeriodizationType: pt,
			IsDeload:          isDeload,
			Slots:             slots,
		}
	}

	return result, nil
}

// PlanDay generates one Session for date, suitable for ad-hoc workouts on
// days outside the weekly plan (extra workouts, or days added mid-week
// after Plan(monday) already ran). weekUsedExerciseIDs is the set of
// exercise IDs already used in other sessions this week; weekLoad is
// the running per-MG weighted load from those sessions (built by the
// caller via WeeklyPlannedLoad).
//
// weekLoad is copied internally before scoring this day's picks, so
// the caller's map is not mutated. weekUsedExerciseIDs is NOT copied —
// the selection loop adds this day's picked IDs to it so a single
// shared map can be threaded through multiple PlanDay calls in the
// same week. Pass a fresh map if you don't want this side effect.
//
// Returns errNoExercisesForCategory (wrapped) if the derived category
// has no compatible exercises.
func (wp *Planner) PlanDay(
	date time.Time,
	weekUsedExerciseIDs map[int]bool,
	weekLoad map[string]float64,
) (Session, error) {
	category := wp.determineCategory(date)
	if !wp.hasExercisesForCategory(category) {
		return Session{}, fmt.Errorf(
			"%w: %s day (%s)", errNoExercisesForCategory, category, date.Weekday(),
		)
	}

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
	monday := MondayOf(date)
	firstPT := wp.firstSessionPeriodizationType(monday)
	pt := nextPeriodizationType(firstPT, idx)

	isDeload := IsDeloadWeek(
		monday, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled,
	)
	if isDeload {
		pt = PeriodizationHypertrophy
	}

	n := exercisesPerSession(wp.Prefs, date.Weekday(), pt, isDeload)
	if n == 0 {
		n = exercisesMedium
		if pt == PeriodizationHypertrophy && !isDeload {
			n = exercisesMediumHypertrophy
		}
	}

	used := weekUsedExerciseIDs
	if used == nil {
		used = map[int]bool{}
	}
	load := make(map[string]float64, len(weekLoad))
	maps.Copy(load, weekLoad)
	slots := wp.selectExercisesForDayWithPeriodization(category, n, pt, isDeload, used, load)

	return Session{ //nolint:exhaustruct // DifficultyRating/StartedAt/CompletedAt start zero.
		Date:              date,
		PeriodizationType: pt,
		IsDeload:          isDeload,
		Slots:             slots,
	}, nil
}

// exercisesPerSession returns how many exercises to include based on session
// duration and periodization. Hypertrophy non-deload sessions of >= 60 min
// get one extra exercise to use the working-set time budget more fully;
// strength and deload sessions keep their base counts.
func exercisesPerSession(prefs Preferences, weekday time.Weekday, pt PeriodizationType, isDeload bool) int {
	hyperBonus := pt == PeriodizationHypertrophy && !isDeload
	switch minutes := prefs.MinutesForDay(weekday); {
	case minutes >= minutesLong:
		if hyperBonus {
			return exercisesLongHypertrophy
		}
		return exercisesLong
	case minutes >= minutesMedium:
		if hyperBonus {
			return exercisesMediumHypertrophy
		}
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

// selectExercisesForDayWithPeriodization picks up to n category-compatible
// exercises for a session, mutating load with each pick's primary
// (PrimarySetWeight) and secondary (SecondarySetWeight) contributions
// and marking each picked exercise's ID in weekUsedExercises so later
// days in the same week skip it. The chosen exercise on every slot is
// the one that maximises scoreCandidate against the current load and
// the planner's Targets, with the lowest exercise ID winning ties.
// Within a session, exercises whose primary MGs overlap with already
// selected primaries are skipped (no two chest-primary picks in one
// session). When no eligible candidate remains, selection stops early
// (graceful degradation: the session may have fewer than n slots).
func (wp *Planner) selectExercisesForDayWithPeriodization(
	category Category,
	n int,
	pt PeriodizationType,
	isDeload bool,
	weekUsedExercises map[int]bool,
	load map[string]float64,
) []ExerciseSlot {
	targets := make(map[string]int, len(wp.Targets))
	for _, t := range wp.Targets {
		targets[t.MuscleGroupName] = t.WeeklySetTarget
	}

	selectedPrimaryMGs := make(map[string]bool)
	selected := make([]ExerciseSlot, 0, n)

	for len(selected) < n {
		bestIdx := wp.pickBestExerciseIdx(category, pt, isDeload, selectedPrimaryMGs, weekUsedExercises, load, targets)
		if bestIdx < 0 {
			break
		}
		ex := wp.Exercises[bestIdx]
		slot := buildPlannedExerciseSlot(ex, pt, isDeload)
		selected = append(selected, slot)
		for _, mg := range ex.PrimaryMuscleGroups {
			selectedPrimaryMGs[mg] = true
		}
		weekUsedExercises[ex.ID] = true
		applyLoad(load, ex, float64(len(slot.Sets)))
	}

	return selected
}

// pickBestExerciseIdx returns the index into wp.Exercises of the exercise that
// maximises scoreCandidate among candidates that are category-compatible,
// not already used this week, and don't share a primary MG with selectedPrimaryMGs.
// Ties are broken by lowest exercise ID. Returns -1 if no candidate qualifies.
func (wp *Planner) pickBestExerciseIdx(
	category Category,
	pt PeriodizationType,
	isDeload bool,
	selectedPrimaryMGs map[string]bool,
	weekUsedExercises map[int]bool,
	load map[string]float64,
	targets map[string]int,
) int {
	bestIdx := -1
	bestScore := 0.0
	for i := range wp.Exercises {
		ex := wp.Exercises[i]
		if !isCategoryCompatible(ex.Category, category) ||
			weekUsedExercises[ex.ID] ||
			primaryMuscleGroupsOverlap(ex, selectedPrimaryMGs) {
			continue
		}
		score := scoreCandidate(ex, pt, isDeload, load, targets)
		// Exact float equality is safe here: scores are derived from
		// integer targets, integer set counts, and fixed half-integer
		// weights (PrimarySetWeight, SecondarySetWeight), so ties round-trip
		// cleanly through IEEE 754.
		if bestIdx < 0 || score > bestScore ||
			(score == bestScore && ex.ID < wp.Exercises[bestIdx].ID) {
			bestIdx = i
			bestScore = score
		}
	}
	return bestIdx
}

// applyLoad accumulates the per-set MG contribution from ex into load:
// PrimarySetWeight per primary MG, SecondarySetWeight per secondary, scaled
// by nSets. Mutates load in place.
func applyLoad(load map[string]float64, ex Exercise, nSets float64) {
	for _, mg := range ex.PrimaryMuscleGroups {
		load[mg] += nSets * PrimarySetWeight
	}
	for _, mg := range ex.SecondaryMuscleGroups {
		load[mg] += nSets * SecondarySetWeight
	}
}

// buildPlannedExerciseSlot creates an ExerciseSlot for one exercise using
// BuildPlannedSets as the single source of truth for set prescription.
func buildPlannedExerciseSlot(ex Exercise, pt PeriodizationType, isDeload bool) ExerciseSlot {
	return ExerciseSlot{ //nolint:exhaustruct // WarmupCompletedAt nil.
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

// scoreCandidate returns the gain in target-balance from adding ex to a
// session: positive when the exercise pulls the running per-MG load
// closer to its target, negative when it pushes an MG further from
// target. An MG that is already over target contributes negatively
// proportional to how far it would overshoot — picks that re-load an
// already-saturated MG score lower than picks that touch fresh ground.
// The metric is the change in the sum of squared distances over
// targeted muscle groups; untargeted MGs are ignored. Set count
// comes from the same deriveSchemeForExercise the planner uses to
// persist sets, so deload halving and periodization-driven set-count
// shifts are reflected automatically.
func scoreCandidate(
	ex Exercise,
	pt PeriodizationType,
	isDeload bool,
	load map[string]float64,
	targets map[string]int,
) float64 {
	_, nSets := deriveSchemeForExercise(ex, pt, isDeload)
	n := float64(nSets)
	contrib := make(map[string]float64, len(ex.PrimaryMuscleGroups)+len(ex.SecondaryMuscleGroups))
	for _, mg := range ex.PrimaryMuscleGroups {
		contrib[mg] += n * PrimarySetWeight
	}
	for _, mg := range ex.SecondaryMuscleGroups {
		contrib[mg] += n * SecondarySetWeight
	}
	var delta float64
	for mg, target := range targets {
		before := float64(target) - load[mg]
		after := before - contrib[mg]
		delta += before*before - after*after
	}
	return delta
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
