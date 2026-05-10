package workout

import (
	"encoding/json"
	"fmt"

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

// exerciseJSONSchema and its MarshalJSON method follow — generator-exercise.go
// consumes the unexported type.

type exerciseJSONSchema struct {
	muscleGroups []string
}

func (ejs exerciseJSONSchema) MarshalJSON() ([]byte, error) {
	schema := map[string]any{
		"type": "object",
		"required": []string{
			"id",
			"name",
			"category",
			"exercise_type",
			"description_markdown",
			"primary_muscle_groups",
			"secondary_muscle_groups",
		},
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "integer",
				"description": "Unique identifier for the exercise, leave as -1 for new exercises",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Name of the exercise",
			},
			"category": map[string]any{
				"type":        "string",
				"description": "Category of the exercise",
				"enum":        []string{"full_body", "upper", "lower"},
			},
			"exercise_type": map[string]any{
				"type":        "string",
				"description": "Type of exercise: weighted, bodyweight, assisted, or time_based",
				"enum":        []string{"weighted", "bodyweight", "assisted", "time_based"},
			},
			"default_starting_seconds": map[string]any{
				"type":        "integer",
				"description": "Default starting seconds for time_based exercises; omit for other types",
			},
			"description_markdown": map[string]any{
				"type":        "string",
				"description": "Markdown description of the exercise",
			},
			"primary_muscle_groups": map[string]any{
				"type":        "array",
				"description": "Primary muscle groups targeted by the exercise",
				"items": map[string]any{
					"type": "string",
					"enum": ejs.muscleGroups,
				},
			},
			"secondary_muscle_groups": map[string]any{
				"type":        "array",
				"description": "Secondary muscle groups targeted by the exercise",
				"items": map[string]any{
					"type": "string",
					"enum": ejs.muscleGroups,
				},
			},
		},
		"additionalProperties": false,
	}
	result, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal exercise schema: %w", err)
	}
	return result, nil
}
