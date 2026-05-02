package workout

import (
	"encoding/json"
	"fmt"
	"time"
)

// Category represents the type of exercise.
type Category string

const (
	CategoryFullBody Category = "full_body"
	CategoryUpper    Category = "upper"
	CategoryLower    Category = "lower"
)

// ExerciseType represents whether an exercise uses weights or bodyweight.
type ExerciseType string

const (
	ExerciseTypeWeighted   ExerciseType = "weighted"
	ExerciseTypeBodyweight ExerciseType = "bodyweight"
	ExerciseTypeAssisted   ExerciseType = "assisted"
)

// PeriodizationType determines the fixed rep target for all exercises in a session.
type PeriodizationType string

const (
	PeriodizationStrength    PeriodizationType = "strength"
	PeriodizationHypertrophy PeriodizationType = "hypertrophy"
)

// Signal is the user's perceived effort after completing a set.
type Signal string

const (
	SignalTooHeavy Signal = "too_heavy"
	SignalOnTarget Signal = "on_target"
	SignalTooLight Signal = "too_light"
)

// Exercise represents a single exercise type, e.g. Squat, Bench Press, etc.
type Exercise struct {
	ID                    int          `json:"id"`
	Name                  string       `json:"name"`
	Category              Category     `json:"category"`
	ExerciseType          ExerciseType `json:"exercise_type"`
	DescriptionMarkdown   string       `json:"description_markdown"`
	PrimaryMuscleGroups   []string     `json:"primary_muscle_groups"`
	SecondaryMuscleGroups []string     `json:"secondary_muscle_groups"`
}

// Resource represents a learning resource for an exercise.
type Resource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

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
				"description": "Type of exercise: weighted, bodyweight, or assisted",
				"enum":        []string{"weighted", "bodyweight", "assisted"},
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

// Set represents a single set of an exercise with target and actual performance.
type Set struct {
	WeightKg      *float64 // Nullable for bodyweight exercises
	MinReps       int
	MaxReps       int
	CompletedReps *int
	CompletedAt   *time.Time // Nullable timestamp when set was completed
	Signal        *Signal    // Nullable; nil until the set is completed
}

// ExerciseSet groups all sets for a specific exercise in a workout.
type ExerciseSet struct {
	// ID is the stable identifier of this exercise slot within the workout. It survives
	// swapping the exercise to a different one, which is what keeps URLs stable.
	ID                int
	Exercise          Exercise
	Sets              []Set
	WarmupCompletedAt *time.Time // Nullable timestamp when warmup for this exercise was completed
}

// ExerciseProgressEntry represents the sets performed for an exercise on a specific date.
type ExerciseProgressEntry struct {
	Date time.Time
	Sets []Set
}

// ExerciseProgress represents an exercise with its historical performance data across sessions.
type ExerciseProgress struct {
	Exercise Exercise
	Entries  []ExerciseProgressEntry
}

// Session represents a complete workout session including all exercises and their sets.
type Session struct {
	Date              time.Time
	DifficultyRating  *int
	StartedAt         time.Time
	CompletedAt       time.Time
	ExerciseSets      []ExerciseSet
	PeriodizationType PeriodizationType
}

// Preferences stores how long a user wants to work out each day of the week.
// A value of 0 means rest day (equivalent to false), any other value means workout day (equivalent to true).
type Preferences struct {
	MondayMinutes    int
	TuesdayMinutes   int
	WednesdayMinutes int
	ThursdayMinutes  int
	FridayMinutes    int
	SaturdayMinutes  int
	SundayMinutes    int
}

// Helper methods to check if a day is a workout day (equivalent to the old boolean logic).
func (p Preferences) Monday() bool    { return p.MondayMinutes > 0 }
func (p Preferences) Tuesday() bool   { return p.TuesdayMinutes > 0 }
func (p Preferences) Wednesday() bool { return p.WednesdayMinutes > 0 }
func (p Preferences) Thursday() bool  { return p.ThursdayMinutes > 0 }
func (p Preferences) Friday() bool    { return p.FridayMinutes > 0 }
func (p Preferences) Saturday() bool  { return p.SaturdayMinutes > 0 }
func (p Preferences) Sunday() bool    { return p.SundayMinutes > 0 }

// IsEmpty returns true if no workout days are scheduled (all days are rest days).
func (p Preferences) IsEmpty() bool {
	return p.MondayMinutes == 0 && p.TuesdayMinutes == 0 && p.WednesdayMinutes == 0 &&
		p.ThursdayMinutes == 0 && p.FridayMinutes == 0 && p.SaturdayMinutes == 0 &&
		p.SundayMinutes == 0
}

// FeatureFlag represents a feature flag that can toggle application features.
type FeatureFlag struct {
	Name    string
	Enabled bool
}

// MuscleGroupTarget stores the minimum weekly set target for a tracked muscle group.
type MuscleGroupTarget struct {
	MuscleGroupName string
	WeeklySetTarget int
}

// MuscleGroupVolume captures the weekly weighted set load for a single muscle group.
// Each set in the plan contributes to every muscle group it touches: PrimarySetWeight
// for primaries and SecondarySetWeight for secondaries. Completed counts only sets
// that have a CompletedAt timestamp; Planned counts every set in the weekly plan and
// is therefore always >= Completed. TargetSets is 0 for muscle groups that don't
// have a row in muscle_group_weekly_targets.
type MuscleGroupVolume struct {
	Name          string
	CompletedLoad float64
	PlannedLoad   float64
	TargetSets    int
}

// MuscleGroupRegion is a coarse anatomical grouping used by UI layers to arrange
// the per-muscle-group bars into push/pull/legs/core sections.
type MuscleGroupRegion string

const (
	RegionUpperPush MuscleGroupRegion = "Upper Push"
	RegionUpperPull MuscleGroupRegion = "Upper Pull"
	RegionLegs      MuscleGroupRegion = "Legs"
	RegionCore      MuscleGroupRegion = "Core"
	RegionOther     MuscleGroupRegion = "Other"
)

// RegionFor classifies a muscle group name into its anatomical region. Names that
// aren't recognised fall through to RegionOther so newly added muscle groups still
// render even before this map is updated.
func RegionFor(muscleGroupName string) MuscleGroupRegion {
	switch muscleGroupName {
	case "Chest", "Shoulders", "Triceps":
		return RegionUpperPush
	case "Upper Back", "Lats", "Biceps", "Traps", "Forearms":
		return RegionUpperPull
	case "Quads", "Hamstrings", "Glutes", "Calves", "Hip Flexors", "Adductors":
		return RegionLegs
	case "Abs", "Obliques", "Lower Back":
		return RegionCore
	default:
		return RegionOther
	}
}
