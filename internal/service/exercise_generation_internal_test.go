package service

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// TestExerciseJSONSchema_StrictMode asserts the OpenAI strict-mode contract:
// every key in `properties` must also appear in `required`. Violating this
// returns a 400 from the chat completions API and silently drops every
// generated exercise into the placeholder fallback path.
func TestExerciseJSONSchema_StrictMode(t *testing.T) {
	raw, err := exerciseJSONSchema{muscleGroups: []string{"quadriceps"}}.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}

	var parsed struct {
		Required   []string       `json:"required"`
		Properties map[string]any `json:"properties"`
	}
	if err = json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	required := make(map[string]struct{}, len(parsed.Required))
	for _, k := range parsed.Required {
		required[k] = struct{}{}
	}
	for prop := range parsed.Properties {
		if _, ok := required[prop]; !ok {
			t.Errorf("property %q is missing from 'required' (OpenAI strict mode rejects this with 400)", prop)
		}
	}
}

func TestExerciseGenerator_Generate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	muscleGroups := []string{"quadriceps", "glutes", "hamstrings", "calves", "core"}
	eg := newExerciseGenerator(openaiAPIKey, muscleGroups, testhelpers.NewLogger(testhelpers.NewWriter(t)))

	t.Run("Successful generation", func(t *testing.T) {
		exercise, err := eg.Generate(t.Context(), "Squat")

		if err != nil {
			t.Fatalf("Failed to generate exercise: %v", err)
		}

		if got, want := exercise.Name, "Squat"; got != want {
			t.Errorf("Got exercise name %q, want %q", got, want)
		}

		if got, want := exercise.Category, domain.Category("lower"); got != want {
			t.Errorf("Got exercise category %q, want %q", got, want)
		}

		if !strings.Contains(exercise.DescriptionMarkdown, "Squat") {
			t.Errorf("No 'Squat' in description %s", exercise.DescriptionMarkdown)
		}

		if !slices.Contains(exercise.PrimaryMuscleGroups, "quadriceps") {
			t.Errorf("Primary muscle groups %v does not contain 'quadriceps'", exercise.PrimaryMuscleGroups)
		}

		if !slices.Contains(exercise.SecondaryMuscleGroups, "core") {
			t.Errorf("Secondary muscle groups %v does not contain 'core'", exercise.SecondaryMuscleGroups)
		}
	})
}
