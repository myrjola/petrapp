package domain

import "time"

// LatestStartingSet captures the weight of the most recent completed first
// set for an exercise along with the periodization type of the session it
// came from. PeriodizationType is empty when no history exists.
type LatestStartingSet struct {
	WeightKg          float64
	PeriodizationType PeriodizationType
}

// ExerciseSetHistory bundles a date with the sets recorded for one exercise
// on that date. Returned by repositories from history-style queries
// (e.g. ListSetsForExerciseSince).
type ExerciseSetHistory struct {
	Date time.Time
	Sets []Set
}
