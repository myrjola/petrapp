package workout

import (
	"github.com/myrjola/petrapp/internal/domain"
)

// Backward-compat aliases. The canonical types live in internal/domain;
// these aliases let handlers and existing tests continue to import "workout"
// while the multi-phase rearchitecture is in flight. They will be removed in
// Phase 4.

type Category = domain.Category

const (
	CategoryFullBody = domain.CategoryFullBody
	CategoryUpper    = domain.CategoryUpper
	CategoryLower    = domain.CategoryLower
)

type ExerciseType = domain.ExerciseType

const (
	ExerciseTypeWeighted   = domain.ExerciseTypeWeighted
	ExerciseTypeBodyweight = domain.ExerciseTypeBodyweight
	ExerciseTypeAssisted   = domain.ExerciseTypeAssisted
	ExerciseTypeTime       = domain.ExerciseTypeTime
)

type PeriodizationType = domain.PeriodizationType

const (
	PeriodizationStrength    = domain.PeriodizationStrength
	PeriodizationHypertrophy = domain.PeriodizationHypertrophy
)

type Signal = domain.Signal

const (
	SignalTooHeavy = domain.SignalTooHeavy
	SignalOnTarget = domain.SignalOnTarget
	SignalTooLight = domain.SignalTooLight
)

type (
	Exercise              = domain.Exercise
	Resource              = domain.Resource
	Set                   = domain.Set
	ExerciseSet           = domain.ExerciseSet
	Session               = domain.Session
	ExerciseProgress      = domain.ExerciseProgress
	ExerciseProgressEntry = domain.ExerciseProgressEntry
	Preferences           = domain.Preferences
	FeatureFlag           = domain.FeatureFlag
	MuscleGroupTarget     = domain.MuscleGroupTarget
	MuscleGroupVolume     = domain.MuscleGroupVolume
	MuscleGroupRegion     = domain.MuscleGroupRegion
)

const (
	RegionUpperPush = domain.RegionUpperPush
	RegionUpperPull = domain.RegionUpperPull
	RegionLegs      = domain.RegionLegs
	RegionCore      = domain.RegionCore
	RegionOther     = domain.RegionOther
)

func RegionFor(name string) MuscleGroupRegion { return domain.RegionFor(name) }

// ErrNotFound is re-exported from internal/domain for the duration of the
// rearchitecture. Phase 4 will retire this alias along with the rest of the
// workout package.
var ErrNotFound = domain.ErrNotFound

// SwapSimilarityScore is re-exported from internal/domain. Handlers call
// workout.SwapSimilarityScore today; that import path keeps working through
// this phase.
func SwapSimilarityScore(current, candidate Exercise) int {
	return domain.SwapSimilarityScore(current, candidate)
}
