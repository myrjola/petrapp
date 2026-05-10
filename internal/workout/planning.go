package workout

// buildPlannedSets returns the persisted set slice for an exercise
// prescribed in a session of the given periodization. Single source of
// truth for "what sets does this exercise get when first added to a
// session" — used by PlanWeek, AddExercise, and SwapExercise.
//
// For weighted/assisted exercises with prior history, callers may
// post-process the returned slice to seed WeightKg from the latest
// completed set (see buildSetsForAdd). This function deliberately does
// not load history; it produces a clean prescription.
func buildPlannedSets(exercise Exercise, periodization PeriodizationType) []Set {
	targetValue, n := deriveSchemeForExercise(exercise, periodization)
	sets := make([]Set, n)
	for i := range sets {
		var weight *float64
		if !exercise.IsTimed() && exercise.ExerciseType != ExerciseTypeBodyweight {
			weight = new(float64)
		}
		sets[i] = Set{ //nolint:exhaustruct // CompletedValue, CompletedAt, Signal start nil.
			WeightKg:    weight,
			TargetValue: targetValue,
		}
	}
	return sets
}
