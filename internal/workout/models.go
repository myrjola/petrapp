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

// Exercise represents a single exercise type, e.g. Squat, Bench Press, etc.
type Exercise struct {
	ID                    int      `json:"id"`
	Name                  string   `json:"name"`
	Category              Category `json:"category"`
	DescriptionMarkdown   string   `json:"description_markdown"`
	PrimaryMuscleGroups   []string `json:"primary_muscle_groups"`
	SecondaryMuscleGroups []string `json:"secondary_muscle_groups"`
}

type exerciseJSONSchema struct {
	muscleGroups []string
}

func (ejs exerciseJSONSchema) MarshalJSON() ([]byte, error) {
	// encode the muscle groups into the JSON schema
	muscleGroupsJSON, err := json.Marshal(ejs.muscleGroups)
	if err != nil {
		return nil, fmt.Errorf("marshal muscle groups: %w", err)
	}

	schema := map[string]interface{}{
		"type": "object",
		"required": []string{
			"id",
			"name",
			"category",
			"description_markdown",
			"primary_muscle_groups",
			"secondary_muscle_groups",
		},
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type":        "integer",
				"description": "Unique identifier for the exercise, leave as -1 for new exercises",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Name of the exercise",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "Category of the exercise",
				"enum":        []string{"full_body", "upper", "lower"},
			},
			"description_markdown": map[string]interface{}{
				"type":        "string",
				"description": "Markdown description of the exercise",
			},
			"primary_muscle_groups": map[string]interface{}{
				"type":        "array",
				"description": "Primary muscle groups targeted by the exercise",
				"items": map[string]interface{}{
					"type": "string",
					"enum": ejs.muscleGroups,
				},
			},
			"secondary_muscle_groups": map[string]interface{}{
				"type":        "array",
				"description": "Secondary muscle groups targeted by the exercise",
				"items": map[string]interface{}{
					"type": "string",
					"enum": ejs.muscleGroups,
				},
			},
		},
		"additionalProperties": false,
	}
	return json.Marshal(schema)
}

// Set represents a single set of an exercise with target and actual performance.
type Set struct {
	WeightKg      float64
	MinReps       int
	MaxReps       int
	CompletedReps *int
}

// ExerciseSet groups all sets for a specific exercise in a workout.
type ExerciseSet struct {
	Exercise Exercise
	Sets     []Set
}

// Session represents a complete workout session including all exercises and their sets.
type Session struct {
	Date             time.Time
	DifficultyRating *int
	StartedAt        time.Time
	CompletedAt      time.Time
	ExerciseSets     []ExerciseSet
}

// Preferences stores which days of the week a user wants to work out.
type Preferences struct {
	Monday    bool
	Tuesday   bool
	Wednesday bool
	Thursday  bool
	Friday    bool
	Saturday  bool
	Sunday    bool
}
