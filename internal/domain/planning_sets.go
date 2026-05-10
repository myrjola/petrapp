package domain

// defaultTargetValue is the fallback target value (reps) when no history is
// available.
const defaultTargetValue = 8

// defaultTimedSets is the fixed set count for time-based exercises, matching
// the planner's timeBasedSets constant.
const defaultTimedSets = 3

// deriveSchemeForExercise returns the per-set target reps and total set
// count for an exercise within a session of the given periodization. For
// time-based exercises, uses DefaultStartingSeconds and a fixed set count of
// defaultTimedSets. For rep-based exercises, returns DeriveScheme values.
func deriveSchemeForExercise(ex Exercise, pt PeriodizationType) (int, int) {
	if ex.IsTimed() {
		if ex.DefaultStartingSeconds != nil {
			return *ex.DefaultStartingSeconds, defaultTimedSets
		}
		// Defensive: time_based exercises must have DefaultStartingSeconds per the
		// schema CHECK, but fall back gracefully rather than panicking.
		return defaultTargetValue, defaultTimedSets
	}
	if ex.RepMin == nil || ex.RepMax == nil {
		// Defensive: non-time_based exercises must have rep_min/rep_max per the
		// schema CHECK; fall back to old defaults if a fixture invariant is violated.
		return defaultTargetValue, defaultTimedSets
	}
	scheme := DeriveScheme(*ex.RepMin, *ex.RepMax, pt)
	return scheme.TargetReps, scheme.TargetSets
}

// BuildPlannedSets returns the persisted set slice for an exercise prescribed
// in a session of the given periodization. Single source of truth for "what
// target value and set count does this exercise get when first added to a
// session".
//
// WeightKg is left nil. The planner persists sets in this shape so that an
// untouched set is distinguishable from one with a recorded weight of zero —
// downstream code (notably BuildSetsForAdd's seed-weight lookup) relies on
// `WeightKg == nil` meaning "never recorded".
func BuildPlannedSets(exercise Exercise, periodization PeriodizationType) []Set {
	targetValue, n := deriveSchemeForExercise(exercise, periodization)
	sets := make([]Set, n)
	for i := range sets {
		sets[i] = Set{ //nolint:exhaustruct // WeightKg, CompletedValue, CompletedAt, Signal start nil.
			TargetValue: targetValue,
		}
	}
	return sets
}

// BuildSetsForAdd produces the Set slice for an exercise being added to or
// swapping into an existing session. The session's periodization always
// dictates TargetValue and TargetSets — a Deadlift added in a Strength week
// gets 3 reps × 4 sets, not whatever the historical session had.
//
// HasWeight exercises always get an allocated WeightKg pointer so the per-set
// form has a non-nil binding target. When historicalSets contains a non-nil
// WeightKg, the most recent one seeds every new set so the user's progression
// isn't lost just because the prescription changed; otherwise the seed is 0.
// Bodyweight and time-based exercises stay nil.
func BuildSetsForAdd(exercise Exercise, periodization PeriodizationType, historicalSets []Set) []Set {
	sets := BuildPlannedSets(exercise, periodization)
	if !exercise.HasWeight() {
		return sets
	}
	var seedWeight float64
	for i := len(historicalSets) - 1; i >= 0; i-- {
		if historicalSets[i].WeightKg != nil {
			seedWeight = *historicalSets[i].WeightKg
			break
		}
	}
	for i := range sets {
		w := seedWeight
		sets[i].WeightKg = &w
	}
	return sets
}
