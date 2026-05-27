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

// TestExerciseGenerator_PromptCoversSchema asserts the prompt instructs the AI
// on every exercise_type the schema enum accepts, and mentions the
// default_starting_seconds field. Without this, the AI silently never produces
// (e.g.) time_based exercises because the natural-language instructions
// exclude that option.
func TestExerciseGenerator_PromptCoversSchema(t *testing.T) {
	t.Parallel()

	muscleGroups := []string{"quadriceps"}
	eg := newExerciseGenerator("dummy-key", muscleGroups, testhelpers.NewLogger(testhelpers.NewWriter(t)))
	prompt := eg.baseExercisePrompt("Plank")

	raw, err := exerciseJSONSchema{muscleGroups: muscleGroups}.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err = json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}

	for _, exerciseType := range schema.Properties["exercise_type"].Enum {
		if !strings.Contains(prompt, exerciseType) {
			t.Errorf("prompt does not mention exercise_type %q from schema enum", exerciseType)
		}
	}
	if !strings.Contains(prompt, "default_starting_seconds") {
		t.Errorf("prompt does not mention default_starting_seconds field")
	}
}

// TestExerciseJSONSchema_StrictMode asserts the OpenAI strict-mode contract:
// every key in `properties` must also appear in `required`. Violating this
// returns a 400 from the chat completions API and silently drops every
// generated exercise into the placeholder fallback path.
func TestExerciseJSONSchema_StrictMode(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
		t.Parallel()

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

// TestExerciseGenerator_GenerateEmptyName asserts the cheap pre-check fires
// before any API call — empty input never reaches OpenAI.
func TestExerciseGenerator_GenerateEmptyName(t *testing.T) {
	t.Parallel()

	eg := newExerciseGenerator("dummy-key", []string{"quadriceps"},
		testhelpers.NewLogger(testhelpers.NewWriter(t)))
	if _, err := eg.Generate(t.Context(), ""); err == nil {
		t.Fatal("Generate(\"\") returned nil error, want non-nil")
	}
}

func TestExerciseGenerator_validateMuscleGroups(t *testing.T) {
	t.Parallel()

	eg := newExerciseGenerator("dummy-key", []string{"quadriceps", "glutes"},
		testhelpers.NewLogger(testhelpers.NewWriter(t)))
	tests := []struct {
		name    string
		input   []string
		wantErr bool
	}{
		{name: "empty is allowed", input: nil, wantErr: false},
		{name: "all valid", input: []string{"quadriceps", "glutes"}, wantErr: false},
		{name: "one invalid rejects", input: []string{"quadriceps", "biceps"}, wantErr: true},
		{name: "case sensitive", input: []string{"Quadriceps"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := eg.validateMuscleGroups(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateMuscleGroups(%v) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestExerciseGenerator_updateResourcesInDescription(t *testing.T) {
	t.Parallel()

	eg := newExerciseGenerator("dummy-key", nil, testhelpers.NewLogger(testhelpers.NewWriter(t)))
	resources := []domain.Resource{
		{Title: "Real video", URL: "https://youtube.com/real"},
		{Title: "Real guide", URL: "https://exrx.net/real"},
	}

	t.Run("replaces existing Resources section", func(t *testing.T) {
		t.Parallel()

		input := "## Instructions\n1. Step one\n\n## Resources\n" +
			"- [Old video](https://example.com/video)\n" +
			"- [Old guide](https://example.com/guide)\n"
		got := eg.updateResourcesInDescription(input, resources)

		if !strings.Contains(got, "[Real video](https://youtube.com/real)") {
			t.Errorf("missing real video link; got:\n%s", got)
		}
		if strings.Contains(got, "https://example.com/video") {
			t.Errorf("placeholder URL leaked through; got:\n%s", got)
		}
		if !strings.Contains(got, "## Instructions") {
			t.Errorf("Instructions section was dropped; got:\n%s", got)
		}
	})

	t.Run("appends Resources section when missing", func(t *testing.T) {
		t.Parallel()

		input := "## Instructions\n1. Step one\n"
		got := eg.updateResourcesInDescription(input, resources)

		if !strings.Contains(got, "## Resources") {
			t.Errorf("Resources section not appended; got:\n%s", got)
		}
		if !strings.Contains(got, "[Real guide](https://exrx.net/real)") {
			t.Errorf("real guide link not appended; got:\n%s", got)
		}
	})
}

// TestExerciseGenerator_PromptDataQualityRules asserts the prompt instructs
// the AI to (a) omit rep counts from the description text — those are
// tracked separately on the exercise — and (b) credit only working
// muscles, not stabilizers. Also asserts the Pass-1 template no longer
// emits the ## Resources block with example.com placeholders.
func TestExerciseGenerator_PromptDataQualityRules(t *testing.T) {
	t.Parallel()

	eg := newExerciseGenerator("dummy-key", []string{"Chest"},
		testhelpers.NewLogger(testhelpers.NewWriter(t)))
	prompt := eg.baseExercisePrompt("Bench Press")

	if strings.Contains(prompt, "example.com") {
		t.Errorf("prompt contains example.com placeholder URLs; Pass 2 should append " +
			"the Resources section only when web search returns valid URLs")
	}
	if strings.Contains(prompt, "repetition guidance") {
		t.Errorf("prompt still asks for 'repetition guidance' step")
	}
	if strings.Contains(prompt, "## Resources") {
		t.Errorf("prompt's Pass-1 structure template still contains a ## Resources block")
	}
	if !strings.Contains(prompt, "stabilizer") {
		t.Errorf("prompt is missing the stabilizer-exclusion rule for muscle groups")
	}
	// Spot-check the rep-rule wording so accidental edits that remove it
	// are caught.
	if !strings.Contains(prompt, "rep counts") {
		t.Errorf("prompt is missing the 'do not include rep counts' rule")
	}
}

func TestCreateMinimalExercise(t *testing.T) {
	t.Parallel()

	ex := createMinimalExercise("Goblet Squat")

	if ex.Name != "Goblet Squat" {
		t.Errorf("Name = %q, want %q", ex.Name, "Goblet Squat")
	}
	if ex.ID != -1 {
		t.Errorf("ID = %d, want -1 (sentinel for unsaved)", ex.ID)
	}
	if ex.Category != domain.CategoryFullBody {
		t.Errorf("Category = %q, want %q", ex.Category, domain.CategoryFullBody)
	}
	if ex.ExerciseType != domain.ExerciseTypeWeighted {
		t.Errorf("ExerciseType = %q, want %q", ex.ExerciseType, domain.ExerciseTypeWeighted)
	}
	if ex.RepMin == nil || ex.RepMax == nil || *ex.RepMin != 5 || *ex.RepMax != 10 {
		t.Errorf("rep range = (%v, %v), want (5, 10) so DB CHECK passes for non-time_based",
			ex.RepMin, ex.RepMax)
	}
	if !strings.Contains(ex.DescriptionMarkdown, "Goblet Squat") {
		t.Errorf("description missing exercise name; got %q", ex.DescriptionMarkdown)
	}
}
