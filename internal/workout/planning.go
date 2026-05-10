package workout

// buildPlannedSets returns the persisted set slice for an exercise
// prescribed in a session of the given periodization. Single source of
// truth for "what target value and set count does this exercise get
// when first added to a session" — used by PlanWeek and buildSetsForAdd.
//
// WeightKg is left nil. The PlanWeek path keeps it nil so progression
// resolves the starting weight lazily at read time
// (see weekplanner.Exercise's "StartingWeightKg is intentionally
// absent" note). The AddExercise/SwapExercise path (buildSetsForAdd)
// post-processes the returned slice to allocate an empty weight
// pointer for weighted/assisted exercises and seed it from history
// when available.
func buildPlannedSets(exercise Exercise, periodization PeriodizationType) []Set {
	targetValue, n := deriveSchemeForExercise(exercise, periodization)
	sets := make([]Set, n)
	for i := range sets {
		sets[i] = Set{ //nolint:exhaustruct // WeightKg, CompletedValue, CompletedAt, Signal start nil.
			TargetValue: targetValue,
		}
	}
	return sets
}
