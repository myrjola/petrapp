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
// WeightKg is left nil. Callers that need to seed a starting weight (e.g.
// AddExercise / SwapExercise paths in service) post-process the slice.
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
