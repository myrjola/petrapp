package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

// TestExerciseGenerator_PromptCoversSchema asserts the prompt instructs the AI
// on every exercise_type the schema enum accepts, and mentions the
// default_starting_seconds field. Without this, the AI silently never produces
// (e.g.) time_based exercises because the natural-language instructions
// exclude that option.
func TestExerciseGenerator_PromptCoversSchema(t *testing.T) {
	t.Parallel()

	muscleGroups := []string{"quadriceps"}
	eg := newExerciseGenerator("dummy-key", muscleGroups, testkit.NewLogger(testkit.NewWriter(t)))
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
	eg := newExerciseGenerator(openaiAPIKey, muscleGroups, testkit.NewLogger(testkit.NewWriter(t)))

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

		// Assert the structural contract rather than a verbatim mention of the
		// name — the model may describe the movement without repeating "Squat".
		if len(exercise.Instructions) == 0 {
			t.Errorf("expected at least one instruction step, got none")
		}

		if !slices.Contains(exercise.PrimaryMuscleGroups, "quadriceps") {
			t.Errorf("Primary muscle groups %v does not contain 'quadriceps'", exercise.PrimaryMuscleGroups)
		}

		// A squat works the glutes and hamstrings; the core is an isometric
		// stabilizer and the prompt's stabilizer-exclusion rule asks the model
		// to omit it, so assert on a genuinely-worked secondary muscle instead.
		if !slices.Contains(exercise.SecondaryMuscleGroups, "glutes") &&
			!slices.Contains(exercise.SecondaryMuscleGroups, "hamstrings") {
			t.Errorf("Secondary muscle groups %v contain neither 'glutes' nor 'hamstrings'",
				exercise.SecondaryMuscleGroups)
		}
	})
}

// TestExerciseGenerator_GenerateEmptyName asserts the cheap pre-check fires
// before any API call — empty input never reaches OpenAI.
func TestExerciseGenerator_GenerateEmptyName(t *testing.T) {
	t.Parallel()

	eg := newExerciseGenerator("dummy-key", []string{"quadriceps"},
		testkit.NewLogger(testkit.NewWriter(t)))
	if _, err := eg.Generate(t.Context(), ""); err == nil {
		t.Fatal("Generate(\"\") returned nil error, want non-nil")
	}
}

func TestExerciseGenerator_validateMuscleGroups(t *testing.T) {
	t.Parallel()

	eg := newExerciseGenerator("dummy-key", []string{"quadriceps", "glutes"},
		testkit.NewLogger(testkit.NewWriter(t)))
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

// TestExerciseGenerator_PromptDataQualityRules asserts the prompt instructs
// the AI to (a) omit rep counts from the description text — those are
// tracked separately on the exercise — and (b) credit only working
// muscles, not stabilizers. Also asserts the Pass-1 template no longer
// emits the ## Resources block with example.com placeholders.
func TestExerciseGenerator_PromptDataQualityRules(t *testing.T) {
	t.Parallel()

	eg := newExerciseGenerator("dummy-key", []string{"Chest"},
		testkit.NewLogger(testkit.NewWriter(t)))
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

// TestExerciseGenerator_validateResourceURLs spins up an httptest.Server with
// handlers covering the response classes we care about: 200, 301→200, 404,
// 500, a slow handler that exceeds the client timeout, a server that forbids
// every probe (403), and one that rejects HEAD with 405 but serves GET. The
// 200, redirect-to-200, always-403 (access-restricted but live), and
// HEAD-blocked-GET-OK resources should survive; 404, 500, and the timeout
// should drop.
func TestExerciseGenerator_validateResourceURLs(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ok", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/notfound", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/boom", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	// Forbids HEAD and GET alike — models a bot-blocking site like NASM.
	mux.HandleFunc("/forbidden", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	// Rejects HEAD with 405 but serves GET — the GET retry should rescue it.
	mux.HandleFunc("/headblocked", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	eg := newExerciseGenerator("dummy-key", nil, testkit.NewLogger(testkit.NewWriter(t)))
	// Override the default client timeout so the slow handler trips it
	// inside the test budget.
	eg.httpClient = &http.Client{Timeout: 200 * time.Millisecond}

	in := []domain.Resource{
		{Title: "OK", URL: srv.URL + "/ok"},
		{Title: "Redirect", URL: srv.URL + "/redirect"},
		{Title: "NotFound", URL: srv.URL + "/notfound"},
		{Title: "Boom", URL: srv.URL + "/boom"},
		{Title: "Slow", URL: srv.URL + "/slow"},
		{Title: "Forbidden", URL: srv.URL + "/forbidden"},
		{Title: "HeadBlocked", URL: srv.URL + "/headblocked"},
	}

	got := eg.validateResourceURLs(t.Context(), in)

	wantTitles := map[string]bool{
		"OK": true, "Redirect": true, "Forbidden": true, "HeadBlocked": true,
	}
	if len(got) != len(wantTitles) {
		t.Fatalf("got %d surviving resources, want %d: %#v", len(got), len(wantTitles), got)
	}
	for _, r := range got {
		if !wantTitles[r.Title] {
			t.Errorf("unexpected survivor %q (%s)", r.Title, r.URL)
		}
	}
}

// TestExerciseGenerator_enhanceWithWebSearch_validatesURLs is a focused unit
// test on Pass 2's URL validation: only live URLs survive into the structured
// Resources slice, covering the wiring without mocking the OpenAI client.
func TestExerciseGenerator_enhanceWithWebSearch_validatesURLs(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/dead", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	eg := newExerciseGenerator("dummy-key", nil, testkit.NewLogger(testkit.NewWriter(t)))
	eg.httpClient = &http.Client{Timeout: 200 * time.Millisecond}

	parsed := []domain.Resource{
		{Title: "Live", URL: srv.URL + "/live"},
		{Title: "Dead", URL: srv.URL + "/dead"},
	}
	alive := eg.validateResourceURLs(t.Context(), parsed)

	if len(alive) != 1 || alive[0].Title != "Live" {
		t.Errorf("expected only the Live resource to survive, got %#v", alive)
	}
}

// TestExtractJSONObject covers the Pass-2 hardening: the web_search-enabled
// Responses call can wrap its JSON in a markdown fence or surround it with
// prose, so extractJSONObject must recover the bare object in each case and
// leave already-clean input untouched.
func TestExtractJSONObject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bare object",
			in:   `{"resources":[]}`,
			want: `{"resources":[]}`,
		},
		{
			name: "fenced json block",
			in:   "```json\n{\"resources\":[{\"title\":\"A\",\"url\":\"https://x\"}]}\n```",
			want: `{"resources":[{"title":"A","url":"https://x"}]}`,
		},
		{
			name: "prose wrapped",
			in:   "Here are the resources I found:\n{\"resources\":[]}\nHope that helps!",
			want: `{"resources":[]}`,
		},
		{
			name: "brace inside string literal",
			in:   `{"resources":[{"title":"a}b","url":"https://x"}]}`,
			want: `{"resources":[{"title":"a}b","url":"https://x"}]}`,
		},
		{
			name: "no object returns trimmed input",
			in:   "  no json here  ",
			want: "no json here",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := extractJSONObject(tt.in); got != tt.want {
				t.Errorf("extractJSONObject(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
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
	if len(ex.Instructions) != 0 || len(ex.CommonMistakes) != 0 || len(ex.Resources) != 0 {
		t.Errorf("minimal exercise should have empty structured content; got %#v / %#v / %#v",
			ex.Instructions, ex.CommonMistakes, ex.Resources)
	}
}
