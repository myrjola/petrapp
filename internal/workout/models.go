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

	return []byte(fmt.Sprintf(`{
		  "type": "object",
		  "required": [
			"id",
			"name",
			"category",
			"description_markdown",
			"primary_muscle_groups",
			"secondary_muscle_groups"
		  ],
		  "properties": {
			"id": {
			  "type": "integer",
			  "description": "Unique identifier for the exercise, leave as -1 for new exercises"
			},
			"name": {
			  "type": "string",
			  "description": "Name of the exercise"
			},
			"category": {
			  "type": "string",
			  "description": "Category of the exercise",
			  "enum": ["full_body", "upper", "lower"]
			},
			"description_markdown": {
			  "type": "string",
			  "description": "Markdown description of the exercise"
			},
			"primary_muscle_groups": {
			  "type": "array",
			  "description": "Primary muscle groups targeted by the exercise",
			  "items": {
				"type": "string",
				"enum": %s
			  }
			},
			"secondary_muscle_groups": {
			  "type": "array",
			  "description": "Secondary muscle groups targeted by the exercise",
			  "items": {
				"type": "string",
				"enum": %s
			  }
			}
		  },
		  "additionalProperties": false
		}`, muscleGroupsJSON, muscleGroupsJSON)), nil
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
