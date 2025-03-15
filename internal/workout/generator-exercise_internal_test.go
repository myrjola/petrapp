package workout

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestExerciseGenerator_Generate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	muscleGroups := []string{"quadriceps", "glutes", "hamstrings", "calves", "core"}
	eg := newExerciseGenerator(openaiAPIKey, muscleGroups)

	// Test successful generation
	t.Run("Successful generation", func(t *testing.T) {
		exercise, err := eg.Generate(t.Context(), "Squat")

		if err != nil {
			t.Fatalf("Failed to generate exercise: %v", err)
		}

		if got, want := exercise.Name, "Squat"; got != want {
			t.Errorf("Got exercise name %q, want %q", got, want)
		}

		if got, want := exercise.Category, Category("lower"); got != want {
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
