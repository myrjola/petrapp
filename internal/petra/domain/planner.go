package domain

import (
	"errors"
	"fmt"
	"maps"
	"math"
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

	numSessionGoals = 2

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

	// Pre-fill all 7 slots with rest-day placeholders. Scheduled days
	// overwrite their slot below; the rest carry only their Date so
	// WeekPlan.SessionOn returns a valid pointer for every day of the week.
	result := WeekPlan{
		Monday:   startingDate,
		Sessions: [7]Session{},
	}
	for i := range 7 {
		result.Sessions[i] = Session{ //nolint:exhaustruct // Rest-day placeholder; no slots or goal.
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

	firstPT := wp.firstSessionGoal(startingDate)
	isDeload := IsDeloadWeek(
		startingDate, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled,
	)
	wv := weekVolumeFor(startingDate, wp.Prefs)

	weekUsedExercises := map[int]bool{}
	volume := map[string]float64{}
	for i, day := range workoutDays {
		pt := nextSessionGoal(firstPT, i)
		if isDeload {
			pt = SessionGoalHypertrophy
		}
		n := exercisesPerSession(wp.Prefs, day.Weekday(), pt, isDeload)
		slots := wp.selectExercisesForDayWithGoal(
			wp.determineCategory(day), n, pt, isDeload, wv, weekUsedExercises, volume,
		)
		dayOffset := int(day.Sub(startingDate).Hours() / hoursPerDay)
		result.Sessions[dayOffset] = Session{ //nolint:exhaustruct // DifficultyRating/StartedAt/CompletedAt start zero.
			Date:     day,
			Goal:     pt,
			IsDeload: isDeload,
			Slots:    slots,
		}
	}

	return result, nil
}

// PlanDay generates one Session for date, suitable for ad-hoc workouts on
// days outside the weekly plan (extra workouts, or days added mid-week
// after Plan(monday) already ran). weekUsedExerciseIDs is the set of
// exercise IDs already used in other sessions this week; weekLoad is
// the running per-MG volume from those sessions (built by the
// caller via WeeklyPlannedVolume).
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

	// Count scheduled prefs days strictly before date.Weekday() in Mon-first
	// week order. Iterating Mon..Sat explicitly (rather than as an int range)
	// handles Sunday correctly: time.Sunday = 0 < time.Monday = 1, so an int
	// range would never count anything for a Sunday date. Sunday falls
	// through the loop with the full Mon..Sat count — exactly the index
	// workoutDays[i==len-1] would have produced for it in Plan.
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
	firstPT := wp.firstSessionGoal(monday)
	pt := nextSessionGoal(firstPT, idx)

	isDeload := IsDeloadWeek(
		monday, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled,
	)
	if isDeload {
		pt = SessionGoalHypertrophy
	}
	wv := weekVolumeFor(monday, wp.Prefs)

	n := exercisesPerSession(wp.Prefs, date.Weekday(), pt, isDeload)
	if n == 0 {
		n = exercisesMedium
		if pt == SessionGoalHypertrophy && !isDeload {
			n = exercisesMediumHypertrophy
		}
	}

	used := weekUsedExerciseIDs
	if used == nil {
		used = map[int]bool{}
	}
	volume := make(map[string]float64, len(weekLoad))
	maps.Copy(volume, weekLoad)
	slots := wp.selectExercisesForDayWithGoal(category, n, pt, isDeload, wv, used, volume)

	return Session{ //nolint:exhaustruct // DifficultyRating/StartedAt/CompletedAt start zero.
		Date:     date,
		Goal:     pt,
		IsDeload: isDeload,
		Slots:    slots,
	}, nil
}

// exercisesPerSession returns how many exercises to include based on session
// duration and goal. Hypertrophy non-deload sessions of >= 60 min
// get one extra exercise to use the working-set time budget more fully;
// strength and deload sessions keep their base counts.
func exercisesPerSession(prefs Preferences, weekday time.Weekday, pt SessionGoal, isDeload bool) int {
	hyperBonus := pt == SessionGoalHypertrophy && !isDeload
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

// firstSessionGoal derives the session goal for the first session of the
// week deterministically from the start date and preferences — no DB query needed.
func (wp *Planner) firstSessionGoal(startingDate time.Time) SessionGoal {
	const secondsPerWeek = 7 * 24 * 3600
	weeksSinceEpoch := startingDate.Unix() / secondsPerWeek
	if weeksSinceEpoch%2 == 0 {
		return SessionGoalStrength
	}
	return SessionGoalHypertrophy
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

// selectExercisesForDayWithGoal picks up to n category-compatible
// exercises for a session, mutating volume with each pick's primary
// (PrimarySetFraction) and secondary (SecondarySetFraction) contributions
// and marking each picked exercise's ID in weekUsedExercises so later
// days in the same week skip it. The chosen exercise on every slot is
// the one that maximises scoreCandidate against the current volume and
// the planner's Targets, with the lowest exercise ID winning ties.
// Within a session, exercises whose primary MGs overlap with already
// selected primaries are skipped (no two chest-primary picks in one
// session). When no eligible candidate remains, selection stops early
// (graceful degradation: the session may have fewer than n slots).
func (wp *Planner) selectExercisesForDayWithGoal(
	category Category,
	n int,
	pt SessionGoal,
	isDeload bool,
	wv weekVolume,
	weekUsedExercises map[int]bool,
	volume map[string]float64,
) []ExerciseSlot {
	targets := make(map[string]MuscleGroupTarget, len(wp.Targets))
	for _, t := range wp.Targets {
		targets[t.MuscleGroupName] = t
	}

	selectedPrimaryMGs := make(map[string]bool)
	selected := make([]ExerciseSlot, 0, n)

	for len(selected) < n {
		bestIdx := wp.pickBestExerciseIdx(
			category,
			pt,
			isDeload,
			wv,
			selectedPrimaryMGs,
			weekUsedExercises,
			volume,
			targets,
		)
		if bestIdx < 0 {
			break
		}
		ex := wp.Exercises[bestIdx]
		slot := buildPlannedExerciseSlot(ex, pt, isDeload, wv.sets)
		selected = append(selected, slot)
		for _, mg := range ex.PrimaryMuscleGroups {
			selectedPrimaryMGs[mg] = true
		}
		weekUsedExercises[ex.ID] = true
		applyVolume(volume, ex, float64(len(slot.Sets)))
	}

	return selected
}

// pickBestExerciseIdx returns the index into wp.Exercises of the exercise that
// maximises scoreCandidate among candidates that are category-compatible,
// not already used this week, and don't share a primary MG with selectedPrimaryMGs.
// Ties are broken by lowest exercise ID. Returns -1 if no candidate qualifies.
func (wp *Planner) pickBestExerciseIdx(
	category Category,
	pt SessionGoal,
	isDeload bool,
	wv weekVolume,
	selectedPrimaryMGs map[string]bool,
	weekUsedExercises map[int]bool,
	volume map[string]float64,
	targets map[string]MuscleGroupTarget,
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
		score := scoreCandidate(ex, pt, isDeload, wv, volume, targets)
		// Exact float equality is safe here: scores are derived from
		// integer targets, integer set counts, and fixed half-integer
		// weights (PrimarySetFraction, SecondarySetFraction), so ties round-trip
		// cleanly through IEEE 754.
		if bestIdx < 0 || score > bestScore ||
			(score == bestScore && ex.ID < wp.Exercises[bestIdx].ID) {
			bestIdx = i
			bestScore = score
		}
	}
	return bestIdx
}

// applyVolume accumulates the per-set MG contribution from ex into volume:
// PrimarySetFraction per primary MG, SecondarySetFraction per secondary, scaled
// by nSets. Mutates volume in place.
func applyVolume(volume map[string]float64, ex Exercise, nSets float64) {
	for _, mg := range ex.PrimaryMuscleGroups {
		volume[mg] += nSets * PrimarySetFraction
	}
	for _, mg := range ex.SecondaryMuscleGroups {
		volume[mg] += nSets * SecondarySetFraction
	}
}

// buildPlannedExerciseSlot creates an ExerciseSlot for one exercise using
// BuildPlannedSets as the single source of truth for set prescription.
func buildPlannedExerciseSlot(ex Exercise, pt SessionGoal, isDeload bool, weekSets int) ExerciseSlot {
	return ExerciseSlot{ //nolint:exhaustruct // WarmupCompletedAt nil.
		Exercise: ex,
		Sets:     BuildPlannedSets(ex, pt, isDeload, weekSets),
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

// nextSessionGoal cycles between SessionGoalStrength and SessionGoalHypertrophy.
// It uses index-based alternation: even indices get the first type, odd indices get the second.
func nextSessionGoal(first SessionGoal, idx int) SessionGoal {
	if idx%numSessionGoals == 0 {
		return first
	}
	if first == SessionGoalStrength {
		return SessionGoalHypertrophy
	}
	return SessionGoalStrength
}

// weekVolume captures the mesocycle-week-derived inputs to one planned session:
// sets is the base per-exercise working-set count for the week (pre-deload),
// progress is the ramp position in [0,1] used to lerp each muscle's scoring goal
// from MinSets toward MaxSets. Both are constant across the days of a calendar
// week, so the planner resolves them once per Plan/PlanDay call.
type weekVolume struct {
	sets     int
	progress float64
}

// goalQuantisationFactor is the denominator used to snap goalForWeek output to
// the nearest 0.5: multiply by this factor, round, then divide back.
const goalQuantisationFactor = 2.0

// goalForWeek lerps a muscle's weekly scoring goal from its floor (MinSets)
// toward its ceiling (MaxSets) by the mesocycle ramp progress, quantised to the
// nearest 0.5. Quantisation keeps every segmentReward term on multiples of 0.5
// so scores stay exact in IEEE-754 and pickBestExerciseIdx's tie-break holds.
// progress 0 → MinSets (the Phase B static floor); progress 1 → MaxSets.
func goalForWeek(t MuscleGroupTarget, progress float64) float64 {
	raw := float64(t.MinSets) + (float64(t.MaxSets)-float64(t.MinSets))*progress
	return math.Round(raw*goalQuantisationFactor) / goalQuantisationFactor
}

// weekVolumeFor resolves the week's volume context from the session date and the
// user's mesocycle preferences. A zero anchor / disabled deload yields progress 0
// and the base set count — the Phase B behaviour.
func weekVolumeFor(date time.Time, prefs Preferences) weekVolume {
	return weekVolume{
		sets:     SetsForWeek(date, prefs.MesocycleAnchor, prefs.MesocycleLength, prefs.DeloadEnabled),
		progress: MesocycleRampProgress(date, prefs.MesocycleAnchor, prefs.MesocycleLength, prefs.DeloadEnabled),
	}
}

// Piecewise per-set marginal reward for weekly volume. Below a muscle's floor
// (MinSets) each added set is worth belowGoalSetReward; from the floor up to
// the ceiling (MaxSets) the dose-response still pays but less
// (aboveGoalSetReward); past the ceiling each set is penalised
// (overMaxSetPenalty) so volume spreads to muscles with remaining headroom.
// The invariant the planner relies on is the ordering
// belowGoalSetReward > aboveGoalSetReward > 0 > overMaxSetPenalty; the exact
// magnitudes are tuning knobs. All three are multiples of 0.5 so scores stay
// exact in IEEE-754 (the tie-break in pickBestExerciseIdx depends on this).
const (
	belowGoalSetReward = 3.0
	aboveGoalSetReward = 1.0
	overMaxSetPenalty  = -2.0
)

// segmentReward returns the marginal balance reward for adding `added` weighted
// sets to a muscle currently at `volume`, given its floor `goal` (MinSets) and
// ceiling `maxSets`. It integrates the piecewise per-set rate over
// [volume, volume+added], so a pick that straddles segments is credited
// proportionally to how much of it lands in each.
func segmentReward(volume, added, goal, maxSets float64) float64 {
	end := volume + added
	below := overlapLength(volume, end, 0, goal)
	within := overlapLength(volume, end, goal, maxSets)
	over := math.Max(0, end-math.Max(volume, maxSets))
	return below*belowGoalSetReward + within*aboveGoalSetReward + over*overMaxSetPenalty
}

// overlapLength returns the length of the intersection of [lo, hi] and
// [segLo, segHi], clamped at 0.
func overlapLength(lo, hi, segLo, segHi float64) float64 {
	return math.Max(0, math.Min(hi, segHi)-math.Max(lo, segLo))
}

// scoreCandidate returns the weekly-volume balance reward from adding ex to a
// session: the sum, over the exercise's targeted muscle groups, of the
// piecewise per-set reward (see segmentReward) for the volume it would add.
// Sets below a muscle's floor earn the steepest reward, sets between floor and
// ceiling a smaller positive reward, and sets past the ceiling a penalty, so
// the planner drives every muscle toward its floor and then keeps paying with
// diminishing returns up to its ceiling. Tag-only muscle groups (no target
// row) contribute nothing. Set count comes from the same
// deriveSchemeForExercise the planner uses to persist sets (the mesocycle
// week's count, wv.sets), so deload set reduction and the week-driven set-count
// ramp are reflected automatically. The goal ramps from MinSets toward MaxSets
// across the block via goalForWeek(t, wv.progress).
func scoreCandidate(
	ex Exercise,
	pt SessionGoal,
	isDeload bool,
	wv weekVolume,
	volume map[string]float64,
	targets map[string]MuscleGroupTarget,
) float64 {
	_, nSets := deriveSchemeForExercise(ex, pt, isDeload, wv.sets)
	n := float64(nSets)
	contrib := make(map[string]float64, len(ex.PrimaryMuscleGroups)+len(ex.SecondaryMuscleGroups))
	for _, mg := range ex.PrimaryMuscleGroups {
		contrib[mg] += n * PrimarySetFraction
	}
	for _, mg := range ex.SecondaryMuscleGroups {
		contrib[mg] += n * SecondarySetFraction
	}
	var score float64
	for mg, added := range contrib {
		t, ok := targets[mg]
		if !ok {
			continue // tag-only group: no target row, contributes nothing.
		}
		score += segmentReward(volume[mg], added, goalForWeek(t, wv.progress), float64(t.MaxSets))
	}
	return score
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
