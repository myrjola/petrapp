package domain

import "slices"

// defaultTargetValue is the fallback target value (reps) when no history is
// available.
const defaultTargetValue = 8

// defaultTimedSets is the fixed set count for time-based exercises, matching
// the planner's timeBasedSets constant.
const defaultTimedSets = 3

// deriveSchemeForExercise returns the per-set target value and total set count
// for an exercise in a session of the given goal. Reps/seconds come
// from the goal (DeriveScheme / DefaultStartingSeconds); the set count
// is weekSets (the mesocycle week's count, see Preferences.SetCountFor), reduced by one on a
// deload (floored at deloadSetFloor). Timed exercises keep a fixed set count
// (defaultTimedSets) and ignore weekSets — their per-session volume does not
// ramp.
func deriveSchemeForExercise(ex Exercise, pt SessionGoal, isDeload bool, weekSets int) (int, int) {
	if ex.IsTimed() {
		sets := defaultTimedSets
		if isDeload {
			sets = deloadSets(sets)
		}
		if ex.DefaultStartingSeconds != nil {
			return *ex.DefaultStartingSeconds, sets
		}
		// Defensive: time_based exercises must have DefaultStartingSeconds per the
		// schema CHECK, but fall back gracefully rather than panicking.
		return defaultTargetValue, sets
	}

	sets := weekSets
	if isDeload {
		sets = deloadSets(weekSets)
	}
	if ex.RepMin == nil || ex.RepMax == nil {
		// Defensive: non-time_based exercises must have rep_min/rep_max per the
		// schema CHECK; fall back to a sane target value if a fixture invariant
		// is violated, but still honour the week-driven set count.
		return defaultTargetValue, sets
	}
	return DeriveScheme(*ex.RepMin, *ex.RepMax, pt, isDeload).TargetReps, sets
}

// BuildPlannedSets returns the persisted set slice for an exercise prescribed
// in a session of the given goal. Single source of truth for "what
// target value and set count does this exercise get when first added to a
// session".
//
// WeightKg is left nil. The planner persists sets in this shape so that an
// untouched set is distinguishable from one with a recorded weight of zero —
// downstream code (notably BuildSetsForAdd's seed-weight lookup) relies on
// `WeightKg == nil` meaning "never recorded".
//
// isDeload drops one set from weekSets (floored at 2) and targets repMax.
func BuildPlannedSets(exercise Exercise, goal SessionGoal, isDeload bool, weekSets int) []Set {
	targetValue, n := deriveSchemeForExercise(exercise, goal, isDeload, weekSets)
	sets := make([]Set, n)
	for i := range sets {
		sets[i] = Set{ //nolint:exhaustruct // WeightKg, CompletedValue, CompletedAt, Signal start nil.
			TargetValue: targetValue,
		}
	}
	return sets
}

// BuildSetsForAdd produces the Set slice for an exercise being added to or
// swapping into an existing session. The session's goal dictates the
// rep/seconds TargetValue while the set count comes from the mesocycle week
// (weekSets, deload-reduced) — a Deadlift added in a Strength week gets the
// strength rep target and the week's set count, not whatever the historical
// session had.
//
// HasWeight exercises always get an allocated WeightKg pointer so the per-set
// form has a non-nil binding target. When historicalSets contains a non-nil
// WeightKg, the most recent one seeds every new set so the user's progression
// isn't lost just because the prescription changed; otherwise the seed is 0.
// Bodyweight and time-based exercises stay nil.
//
// isDeload drops one set (floored at 2) and targets repMax (see BuildPlannedSets).
func BuildSetsForAdd(
	exercise Exercise,
	goal SessionGoal,
	isDeload bool,
	weekSets int,
	historicalSets []Set,
) []Set {
	sets := BuildPlannedSets(exercise, goal, isDeload, weekSets)
	if !exercise.HasWeight() {
		return sets
	}
	var seedWeight float64
	for _, v := range slices.Backward(historicalSets) {
		if v.WeightKg != nil {
			seedWeight = *v.WeightKg
			break
		}
	}
	for i := range sets {
		w := seedWeight
		sets[i].WeightKg = &w
	}
	return sets
}
