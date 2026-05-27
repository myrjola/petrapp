# Exercise Data-Quality Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix dead links and redundant rep-guidance text in exercise descriptions, both at the generator (prompt + URL validation) and in existing prod data (offline backfill tool that emits a SQL one-shot).

**Architecture:** Three concerns: (a) prompt-only changes in `internal/service/exercise_generation.go` to stop generating bad content; (b) a new `validateResourceURLs` method on `exerciseGenerator` that HEAD-checks URLs returned by the web-search pass; (c) a new one-shot `cmd/exercise-content-fixup/` Go program that reads a local SQLite snapshot, applies pure-function transforms to each `exercises.description_markdown`, and emits an `UPDATE` SQL file for review and deploy.

**Tech Stack:** Go, SQLite (mattn/go-sqlite3), `net/http` for HEAD checks, `httptest` for tests, OpenAI Go SDK (already wired). No new dependencies.

**Spec:** [`docs/superpowers/specs/2026-05-27-exercise-data-quality-design.md`](../specs/2026-05-27-exercise-data-quality-design.md)

---

## File Structure

**Generator changes (Task 1–4):**
- Modify: `internal/service/exercise_generation.go`
- Modify: `internal/service/exercise_generation_internal_test.go`

**Backfill tool (Task 5–9):**
- Create: `cmd/exercise-content-fixup/main.go`
- Create: `cmd/exercise-content-fixup/transforms.go`
- Create: `cmd/exercise-content-fixup/transforms_test.go`
- Create: `cmd/exercise-content-fixup/linkcheck.go`

**Manual deliverables (Task 10 — produced by running the tool, not coded):**
- Create: `docs/2026-05-27-exercise-content-cleanup.sql`
- Create: `docs/2026-05-27-exercise-content-cleanup.md`
- Modify: `internal/sqlite/fixtures.sql`

---

### Task 1: Update prompt — drop rep guidance, add stabilizer rule, remove placeholder URLs

**Files:**
- Modify: `internal/service/exercise_generation.go:143-193` (`baseExercisePrompt`)
- Modify: `internal/service/exercise_generation_internal_test.go`

The prompt currently asks for an "Optional step 5 with repetition guidance" and emits a `## Resources` block with `https://example.com/...` placeholders. After this task: the structure template has only `## Instructions` and `## Common Mistakes`; explicit rules forbid rep/set/duration mentions and require stabilizers to be excluded from muscle groups.

- [ ] **Step 1: Write failing prompt assertions**

Add to `internal/service/exercise_generation_internal_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service -run TestExerciseGenerator_PromptDataQualityRules`
Expected: FAIL on the four substring checks (`example.com`, `repetition guidance`, `## Resources` all present in the current prompt; `stabilizer` and `rep counts` are absent).

- [ ] **Step 3: Update `baseExercisePrompt`**

Edit `internal/service/exercise_generation.go:143-193`. Replace the entire function body with:

```go
func (eg *exerciseGenerator) baseExercisePrompt(name string) string {
	return fmt.Sprintf(`Generate a detailed exercise for "%s".

The response must strictly follow this JSON structure:
{
  "id": -1,
  "name": "%s",
  "category": "CATEGORY",
  "exercise_type": "EXERCISE_TYPE",
  "default_starting_seconds": 30,
  "description_markdown": "MARKDOWN_DESCRIPTION",
  "primary_muscle_groups": ["PRIMARY_MUSCLE_GROUP1", "PRIMARY_MUSCLE_GROUP2"],
  "secondary_muscle_groups": ["SECONDARY_MUSCLE_GROUP1", "SECONDARY_MUSCLE_GROUP2"]
}

For "category", use one of: "full_body", "upper", "lower"
For "exercise_type", use one of: "weighted", "bodyweight", "assisted", "time_based"
  - Use "time_based" for isometric holds and timed exercises (planks, wall sits, dead hangs, etc.)
  - Use "weighted" for exercises performed with external load
  - Use "bodyweight" for exercises performed against gravity alone
  - Use "assisted" for exercises that reduce bodyweight (assisted pull-ups, etc.)
For "default_starting_seconds", set a reasonable beginner duration in seconds (e.g. 20-45)
when exercise_type is "time_based"; otherwise set it to null.
For "muscle_groups", use only from this list: %s

Muscle-group rule: only credit a muscle as primary or secondary if it performs a
working contraction (concentric or eccentric load). Pure isometric stabilizers
(e.g. the lats during a push-up, the upper back during a bench press, the core
during an overhead press) do not count and must be omitted.

The "description_markdown" must follow this exact structure:

## Instructions
1. [Step 1 with clear form guidance]
2. [Step 2 with positioning details]
3. [Step 3 with movement description]
4. [Optional step 4 with breathing/tempo guidance]

## Common Mistakes
- [Mistake 1: explanation of error and correction]
- [Mistake 2: explanation of error and correction]
- [Mistake 3: explanation of error and correction]
- [Optional Mistake 4: explanation of error and correction]

Description content rules:
- Do not include rep counts, set counts, weights, or durations anywhere in the
  description. The app tracks rep and set targets separately and shows them to
  the user. Mentions like "perform 8-12 reps", "do 3 sets", or "hold for 30
  seconds" must not appear.
- Do not include a "## Resources" section. Tutorial links are added by a
  follow-up search step and appended automatically.

Instructions must be clear, concise, and focus on proper form using simple language for beginners.
Include relevant safety considerations. The entire description should be 150-200 words.

Return only the valid JSON object with no additional text or explanation.`,
		name, name, strings.Join(eg.muscleGroups, ", "))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/service -run TestExerciseGenerator_PromptDataQualityRules`
Expected: PASS.

Also run the existing prompt schema test to confirm no regression:

Run: `go test -v ./internal/service -run TestExerciseGenerator_PromptCoversSchema`
Expected: PASS (the schema enum values `weighted`, `bodyweight`, `assisted`, `time_based` and `default_starting_seconds` all still appear in the new prompt).

- [ ] **Step 5: Commit**

```bash
git add internal/service/exercise_generation.go internal/service/exercise_generation_internal_test.go
git commit -m "feat(service): drop rep guidance and placeholders from exercise prompt"
```

---

### Task 2: Add `validateResourceURLs` method with HEAD checks

**Files:**
- Modify: `internal/service/exercise_generation.go` (add field on `exerciseGenerator`, new method)
- Modify: `internal/service/exercise_generation_internal_test.go` (new test)

The method takes a slice of `domain.Resource`, HEAD-checks each URL with a 5s timeout, and returns the filtered slice. Failures are logged at debug level; the method never errors.

- [ ] **Step 1: Write the failing test**

Add to `internal/service/exercise_generation_internal_test.go`:

```go
// TestExerciseGenerator_validateResourceURLs spins up an httptest.Server with
// handlers covering the response classes we care about: 200, 301→200, 404,
// 500, and a slow handler that exceeds the client timeout. Only 200 and the
// redirect chain that ends in 200 should survive.
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
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	eg := newExerciseGenerator("dummy-key", nil, testhelpers.NewLogger(testhelpers.NewWriter(t)))
	// Override the default client timeout so the slow handler trips it
	// inside the test budget.
	eg.httpClient = &http.Client{Timeout: 200 * time.Millisecond}

	in := []domain.Resource{
		{Title: "OK", URL: srv.URL + "/ok"},
		{Title: "Redirect", URL: srv.URL + "/redirect"},
		{Title: "NotFound", URL: srv.URL + "/notfound"},
		{Title: "Boom", URL: srv.URL + "/boom"},
		{Title: "Slow", URL: srv.URL + "/slow"},
	}

	got := eg.validateResourceURLs(t.Context(), in)

	wantTitles := map[string]bool{"OK": true, "Redirect": true}
	if len(got) != len(wantTitles) {
		t.Fatalf("got %d surviving resources, want %d: %#v", len(got), len(wantTitles), got)
	}
	for _, r := range got {
		if !wantTitles[r.Title] {
			t.Errorf("unexpected survivor %q (%s)", r.Title, r.URL)
		}
	}
}
```

Add the imports `"net/http"`, `"net/http/httptest"`, and `"time"` at the top of the test file if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service -run TestExerciseGenerator_validateResourceURLs`
Expected: FAIL — `eg.httpClient undefined` and `eg.validateResourceURLs undefined`.

- [ ] **Step 3: Add the field and method**

In `internal/service/exercise_generation.go`, at the top of the file add to the imports:

```go
"net/http"
"time"
```

Modify the `exerciseGenerator` struct (around line 104):

```go
// exerciseGenerator generates exercises using OpenAI API.
type exerciseGenerator struct {
	client       openai.Client
	httpClient   *http.Client
	logger       *slog.Logger
	muscleGroups []string
}
```

Modify `newExerciseGenerator` (around line 111) to initialize `httpClient`:

```go
func newExerciseGenerator(openaiAPIKey string, muscleGroups []string, logger *slog.Logger) *exerciseGenerator {
	client := openai.NewClient(option.WithAPIKey(openaiAPIKey))
	return &exerciseGenerator{
		client:       client,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		logger:       logger,
		muscleGroups: muscleGroups,
	}
}
```

Add the new method, placed just below `enhanceWithWebSearch`:

```go
// validateResourceURLs HEAD-checks each resource URL with the generator's
// http client and returns the subset whose final response is 2xx or 3xx.
// Failures (network errors, timeouts, 4xx, 5xx) are logged at debug level
// and the resource is silently dropped. Best-effort: never returns an error.
func (eg *exerciseGenerator) validateResourceURLs(
	ctx context.Context,
	resources []domain.Resource,
) []domain.Resource {
	alive := make([]domain.Resource, 0, len(resources))
	for _, r := range resources {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, r.URL, nil)
		if err != nil {
			eg.logger.LogAttrs(ctx, slog.LevelDebug, "skip resource: bad URL",
				slog.String("url", r.URL), slog.Any("error", err))
			continue
		}
		resp, err := eg.httpClient.Do(req)
		if err != nil {
			eg.logger.LogAttrs(ctx, slog.LevelDebug, "skip resource: request failed",
				slog.String("url", r.URL), slog.Any("error", err))
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			eg.logger.LogAttrs(ctx, slog.LevelDebug, "skip resource: bad status",
				slog.String("url", r.URL), slog.Int("status", resp.StatusCode))
			continue
		}
		alive = append(alive, r)
	}
	return alive
}
```

Note: `http.Client.Do` follows 3xx redirects by default up to 10 hops, so a 301→200 chain ends with `StatusCode == 200` and survives the `>= 400` filter.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/service -run TestExerciseGenerator_validateResourceURLs`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/exercise_generation.go internal/service/exercise_generation_internal_test.go
git commit -m "feat(service): HEAD-check resource URLs before injecting into description"
```

---

### Task 3: Fix `updateResourcesInDescription` to handle empty resources

**Files:**
- Modify: `internal/service/exercise_generation.go:311-348` (`updateResourcesInDescription`)
- Modify: `internal/service/exercise_generation_internal_test.go`

The current helper's `case strings.HasPrefix(line, "## Resources"):` branch appends the header line unconditionally, so calling it with an empty `resources` slice on input that already has a `## Resources` section emits an orphan header followed by no items. Fix: when resources is empty, drop the section heading too.

- [ ] **Step 1: Write the failing test**

Add to `internal/service/exercise_generation_internal_test.go` (place inside the existing `TestExerciseGenerator_updateResourcesInDescription` function as a new subtest):

```go
	t.Run("empty resources drops existing Resources section", func(t *testing.T) {
		t.Parallel()

		input := "## Instructions\n1. Step one\n\n## Resources\n" +
			"- [Old video](https://example.com/video)\n" +
			"- [Old guide](https://example.com/guide)\n"
		got := eg.updateResourcesInDescription(input, nil)

		if strings.Contains(got, "## Resources") {
			t.Errorf("orphan ## Resources heading left behind; got:\n%s", got)
		}
		if strings.Contains(got, "https://example.com/") {
			t.Errorf("placeholder URLs leaked through; got:\n%s", got)
		}
		if !strings.Contains(got, "## Instructions") {
			t.Errorf("Instructions section was dropped; got:\n%s", got)
		}
	})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service -run TestExerciseGenerator_updateResourcesInDescription`
Expected: FAIL on the new "empty resources" subtest — current code leaves the `## Resources` header line in the output.

- [ ] **Step 3: Fix the helper**

In `internal/service/exercise_generation.go`, replace the body of `updateResourcesInDescription` with:

```go
func (eg *exerciseGenerator) updateResourcesInDescription(
	markdown string,
	resources []domain.Resource,
) string {
	lines := strings.Split(markdown, "\n")
	var result []string
	inResourcesSection := false

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "## Resources"):
			inResourcesSection = true
			if len(resources) == 0 {
				continue
			}
			result = append(result, line)
			for _, res := range resources {
				result = append(result, fmt.Sprintf("- [%s](%s)", res.Title, res.URL))
			}
		case inResourcesSection && strings.HasPrefix(line, "##"):
			inResourcesSection = false
			result = append(result, line)
		case !inResourcesSection:
			result = append(result, line)
		}
	}

	// If no Resources section was present and we have resources to add, append one.
	if !inResourcesSection && len(resources) > 0 && !containsResourcesHeading(result) {
		result = append(result, "\n## Resources")
		for _, res := range resources {
			result = append(result, fmt.Sprintf("- [%s](%s)", res.Title, res.URL))
		}
	}

	return strings.Join(result, "\n")
}

// containsResourcesHeading reports whether any line in result already starts
// with "## Resources". Used by updateResourcesInDescription to avoid emitting
// a duplicate section when the input already had one and it was replaced.
func containsResourcesHeading(lines []string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "## Resources") {
			return true
		}
	}
	return false
}
```

The change: the `## Resources` case branch now does `continue` (skipping both the header append and the resource-line emission) when `resources` is empty. The dropped-line branch — original resource list items inside the section — collapses from the previous redundant `if !strings.HasPrefix(line, "- [") || !inResourcesSection` to a plain `case !inResourcesSection:` (which fall-through correctly drops anything while `inResourcesSection` is true). The `containsResourcesHeading` guard prevents the trailing append branch from duplicating a section we already emitted via the in-loop branch.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/service -run TestExerciseGenerator_updateResourcesInDescription`
Expected: PASS on all three subtests (existing two + new one).

- [ ] **Step 5: Commit**

```bash
git add internal/service/exercise_generation.go internal/service/exercise_generation_internal_test.go
git commit -m "fix(service): drop ## Resources heading when no resources are provided"
```

---

### Task 4: Wire `validateResourceURLs` into `enhanceWithWebSearch`

**Files:**
- Modify: `internal/service/exercise_generation.go:253-309` (`enhanceWithWebSearch`)
- Modify: `internal/service/exercise_generation_internal_test.go`

After parsing the web-search JSON, run the resources through `validateResourceURLs` before injecting them. If zero survive, the description gets no Resources section (`updateResourcesInDescription` skips when `len(resources) == 0`, fixed in Task 3).

- [ ] **Step 1: Write the failing test**

Add to `internal/service/exercise_generation_internal_test.go`:

```go
// TestExerciseGenerator_enhanceWithWebSearch_validatesURLs is a focused unit
// test on the integration between Pass 2's JSON parsing and the URL
// validator. We exercise validateResourceURLs and updateResourcesInDescription
// directly with a representative mix and assert that only live URLs land in
// the final markdown — covering the wiring without mocking the OpenAI client.
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

	eg := newExerciseGenerator("dummy-key", nil, testhelpers.NewLogger(testhelpers.NewWriter(t)))
	eg.httpClient = &http.Client{Timeout: 200 * time.Millisecond}

	parsed := []domain.Resource{
		{Title: "Live", URL: srv.URL + "/live"},
		{Title: "Dead", URL: srv.URL + "/dead"},
	}
	alive := eg.validateResourceURLs(t.Context(), parsed)

	desc := "## Instructions\n1. Step one\n\n## Common Mistakes\n- Bad form\n"
	got := eg.updateResourcesInDescription(desc, alive)

	if !strings.Contains(got, "[Live]") {
		t.Errorf("live URL missing from output; got:\n%s", got)
	}
	if strings.Contains(got, "[Dead]") {
		t.Errorf("dead URL leaked through; got:\n%s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it passes (no production change needed yet — this test exercises Task 2 + Task 3 wiring)**

Run: `go test -v ./internal/service -run TestExerciseGenerator_enhanceWithWebSearch_validatesURLs`
Expected: PASS (the building blocks already exist).

- [ ] **Step 3: Wire `validateResourceURLs` into `enhanceWithWebSearch`**

In `internal/service/exercise_generation.go`, modify `enhanceWithWebSearch`. Replace the block starting at the `// Update description with real URLs if found` comment with:

```go
	// Validate URLs before injecting them: drop dead links so the
	// description never ships with broken Resources entries.
	alive := eg.validateResourceURLs(ctx, resourceResponse.Resources)
	if len(alive) == 0 && len(resourceResponse.Resources) > 0 {
		eg.logger.LogAttrs(ctx, slog.LevelInfo, "dropped all resource URLs",
			slog.String("exercise", exercise.Name),
			slog.Int("returned", len(resourceResponse.Resources)))
	}
	exercise.DescriptionMarkdown = eg.updateResourcesInDescription(
		exercise.DescriptionMarkdown,
		alive,
	)

	return nil
}
```

(The original `if len(resourceResponse.Resources) > 0` guard is no longer needed because `updateResourcesInDescription` itself now handles the empty-slice case correctly.)

- [ ] **Step 4: Run the full service test suite**

Run: `go test ./internal/service -count=1`
Expected: PASS. No live OpenAI test runs in short mode by default; if the env happens to have `OPENAI_API_KEY` set the live test runs and should still pass — but the live test isn't required.

- [ ] **Step 5: Commit**

```bash
git add internal/service/exercise_generation.go internal/service/exercise_generation_internal_test.go
git commit -m "feat(service): validate web-search URLs before injecting into description"
```

---

### Task 5: Scaffold the backfill tool — `cmd/exercise-content-fixup/main.go`

**Files:**
- Create: `cmd/exercise-content-fixup/main.go`

A minimal skeleton: flag parsing (`-db`, `-out`), open SQLite read-only, iterate `exercises.id, name, description_markdown`, no transforms yet — just print row count to stdout. Subsequent tasks fill in transforms and SQL emission.

- [ ] **Step 1: Write the skeleton**

Create `cmd/exercise-content-fixup/main.go`:

```go
// Package main implements a one-shot tool that cleans dead links and
// redundant rep-guidance text from exercises.description_markdown in a
// SQLite snapshot of the production database. It is read-only against the
// input database; the deliverable is a SQL UPDATE file that a human
// reviews before applying to production via `make fly-sql-write`.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	var (
		dbPath  = flag.String("db", "", "Path to the SQLite snapshot to read (required)")
		outPath = flag.String("out", "", "Path to write the generated UPDATE SQL (required)")
	)
	flag.Parse()

	if *dbPath == "" || *outPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*dbPath, *outPath); err != nil {
		log.Fatalf("exercise-content-fixup: %v", err)
	}
}

func run(dbPath, outPath string) error {
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query("SELECT id, name, description_markdown FROM exercises ORDER BY id")
	if err != nil {
		return fmt.Errorf("select exercises: %w", err)
	}
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		var (
			id   int
			name string
			desc string
		)
		if err = rows.Scan(&id, &name, &desc); err != nil {
			return fmt.Errorf("scan row: %w", err)
		}
		count++
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("iterate rows: %w", err)
	}

	fmt.Printf("scanned %d exercises\n", count)

	// TODO(task 9): emit transformed UPDATEs to outPath.
	_ = outPath

	return nil
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./cmd/exercise-content-fixup`
Expected: builds clean, produces a `./exercise-content-fixup` binary in the working directory.

- [ ] **Step 3: Smoke-test against the dev DB**

The repo already has `petrapp.sqlite3` at the root from `make init` runs. If absent, run `make init` first.

Run: `./exercise-content-fixup -db petrapp.sqlite3 -out /tmp/test.sql`
Expected: prints `scanned N exercises` where N matches the row count in fixtures (around 38).

Clean up the binary: `rm exercise-content-fixup`.

- [ ] **Step 4: Commit**

```bash
git add cmd/exercise-content-fixup/main.go
git commit -m "feat(cmd): scaffold exercise-content-fixup tool"
```

---

### Task 6: Implement `StripDeadResourceLinks` transform

**Files:**
- Create: `cmd/exercise-content-fixup/transforms.go`
- Create: `cmd/exercise-content-fixup/transforms_test.go`

Pure function: takes a description string and a `map[string]bool` of URLs known to be alive. Drops list items whose URL is dead or matches the placeholder prefix `https://example.com/`. If zero list items survive in the `## Resources` section, drops the section heading too.

- [ ] **Step 1: Write the failing test**

Create `cmd/exercise-content-fixup/transforms_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestStripDeadResourceLinks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		alive     map[string]bool
		wantHas   []string
		wantNotIn []string
	}{
		{
			name: "drops example.com placeholders unconditionally",
			input: "## Instructions\n1. Step\n\n## Resources\n" +
				"- [Video](https://example.com/v)\n" +
				"- [Guide](https://example.com/g)\n",
			alive:     map[string]bool{},
			wantNotIn: []string{"## Resources", "example.com"},
			wantHas:   []string{"## Instructions"},
		},
		{
			name: "drops dead URLs, keeps live",
			input: "## Instructions\n1. Step\n\n## Resources\n" +
				"- [Live](https://live.example.org/a)\n" +
				"- [Dead](https://dead.example.org/b)\n",
			alive:     map[string]bool{"https://live.example.org/a": true},
			wantHas:   []string{"## Resources", "[Live]", "## Instructions"},
			wantNotIn: []string{"[Dead]"},
		},
		{
			name: "drops Resources heading when nothing survives",
			input: "## Instructions\n1. Step\n\n## Resources\n" +
				"- [A](https://dead.example.org/a)\n" +
				"- [B](https://dead.example.org/b)\n",
			alive:     map[string]bool{},
			wantNotIn: []string{"## Resources", "[A]", "[B]"},
			wantHas:   []string{"## Instructions"},
		},
		{
			name: "no Resources section is a no-op",
			input: "## Instructions\n1. Step\n\n## Common Mistakes\n- Bad form\n",
			alive: map[string]bool{},
			wantHas: []string{"## Instructions", "## Common Mistakes", "- Bad form"},
			wantNotIn: []string{"## Resources"},
		},
		{
			name: "Resources at end of file with no trailing section",
			input: "## Instructions\n1. Step\n\n## Resources\n" +
				"- [Live](https://live.example.org/a)\n",
			alive:   map[string]bool{"https://live.example.org/a": true},
			wantHas: []string{"## Resources", "[Live]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := StripDeadResourceLinks(tt.input, tt.alive)
			for _, want := range tt.wantHas {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q; got:\n%s", want, got)
				}
			}
			for _, notWant := range tt.wantNotIn {
				if strings.Contains(got, notWant) {
					t.Errorf("found %q but should be absent; got:\n%s", notWant, got)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/exercise-content-fixup -run TestStripDeadResourceLinks`
Expected: FAIL — `StripDeadResourceLinks` undefined.

- [ ] **Step 3: Implement the transform**

Create `cmd/exercise-content-fixup/transforms.go`:

```go
package main

import (
	"regexp"
	"strings"
)

// resourceLinkPattern matches a markdown list item of the form
// `- [Title](https://...)`. The URL is captured in group 1.
var resourceLinkPattern = regexp.MustCompile(`^\s*-\s*\[[^\]]*\]\((https?://[^\s)]+)\)\s*$`)

// placeholderURLPrefix marks URLs the AI generator emits as unfilled
// placeholders. They are dead by definition; never HEAD-check them.
const placeholderURLPrefix = "https://example.com/"

// StripDeadResourceLinks removes resource list items whose URLs are not in
// aliveURLs (or whose URLs match the placeholder prefix). If no list items
// survive in the ## Resources section, the heading is also removed.
//
// The caller is responsible for pre-populating aliveURLs by HEAD-checking
// every URL it cares about; transforms.go has no network dependency.
func StripDeadResourceLinks(desc string, aliveURLs map[string]bool) string {
	lines := strings.Split(desc, "\n")
	var (
		out                []string
		inResources        bool
		resourcesHeaderIdx = -1
		survivors          int
	)

	for _, line := range lines {
		if strings.HasPrefix(line, "## Resources") {
			inResources = true
			resourcesHeaderIdx = len(out)
			out = append(out, line)
			continue
		}
		if inResources && strings.HasPrefix(line, "##") {
			inResources = false
			if survivors == 0 {
				out = dropResourcesHeader(out, resourcesHeaderIdx)
			}
			resourcesHeaderIdx = -1
			out = append(out, line)
			continue
		}
		if inResources {
			m := resourceLinkPattern.FindStringSubmatch(line)
			if m == nil {
				// Non-link content inside the section (blank lines, prose).
				// Keep it — we only filter list items.
				out = append(out, line)
				continue
			}
			url := m[1]
			if strings.HasPrefix(url, placeholderURLPrefix) {
				continue
			}
			if !aliveURLs[url] {
				continue
			}
			out = append(out, line)
			survivors++
			continue
		}
		out = append(out, line)
	}

	// Resources at EOF with no trailing section header to flush.
	if inResources && survivors == 0 {
		out = dropResourcesHeader(out, resourcesHeaderIdx)
	}

	return strings.Join(out, "\n")
}

// dropResourcesHeader removes the "## Resources" line at idx and any
// immediately-trailing blank lines that were the separator before the
// (now empty) section content. Returns the trimmed slice.
func dropResourcesHeader(out []string, idx int) []string {
	if idx < 0 || idx >= len(out) {
		return out
	}
	// Drop the header itself.
	out = append(out[:idx], out[idx+1:]...)
	// Drop one trailing blank line if present (visual separator).
	if idx > 0 && idx <= len(out) && idx-1 < len(out) && out[idx-1] == "" {
		out = append(out[:idx-1], out[idx:]...)
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./cmd/exercise-content-fixup -run TestStripDeadResourceLinks`
Expected: PASS on all five subtests.

- [ ] **Step 5: Commit**

```bash
git add cmd/exercise-content-fixup/transforms.go cmd/exercise-content-fixup/transforms_test.go
git commit -m "feat(cmd): add StripDeadResourceLinks transform"
```

---

### Task 7: Implement `StripRepGuidanceLines` transform

**Files:**
- Modify: `cmd/exercise-content-fixup/transforms.go`
- Modify: `cmd/exercise-content-fixup/transforms_test.go`

Strip list items in the `## Instructions` ordered list (and any list item anywhere in the description, defensively) whose text matches rep / set / duration patterns. Keep negative cases like "Take 2 deep breaths" or "3-second tempo" working.

- [ ] **Step 1: Write the failing test**

Append to `cmd/exercise-content-fixup/transforms_test.go`:

```go
func TestStripRepGuidanceLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantHas   []string
		wantNotIn []string
	}{
		{
			name: "drops 'perform 8-12 reps' from Instructions",
			input: "## Instructions\n" +
				"1. Set up the bar.\n" +
				"2. Lower with control.\n" +
				"3. Perform 8-12 reps.\n",
			wantHas:   []string{"Set up the bar", "Lower with control"},
			wantNotIn: []string{"Perform 8-12 reps", "8-12 reps"},
		},
		{
			name: "drops 'complete 3 sets'",
			input: "## Instructions\n1. Step one.\n2. Complete 3 sets of the movement.\n",
			wantHas:   []string{"Step one"},
			wantNotIn: []string{"Complete 3 sets"},
		},
		{
			name: "drops 'hold for 30 seconds'",
			input: "## Instructions\n1. Set up.\n2. Hold for 30 seconds at the bottom.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Hold for 30 seconds"},
		},
		{
			name: "drops 'do 10 repetitions'",
			input: "## Instructions\n1. Set up.\n2. Do 10 repetitions per side.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"Do 10 repetitions"},
		},
		{
			name:    "keeps 'Take 2 deep breaths' — not a rep mention",
			input:   "## Instructions\n1. Take 2 deep breaths before lifting.\n",
			wantHas: []string{"Take 2 deep breaths"},
		},
		{
			name:    "keeps '3-second tempo' — not a rep mention",
			input:   "## Instructions\n1. Lower at a 3-second tempo.\n",
			wantHas: []string{"3-second tempo"},
		},
		{
			name:      "drops bare 'repetition guidance' from literal template leak",
			input:     "## Instructions\n1. Set up.\n2. Optional step 5 with repetition guidance.\n",
			wantHas:   []string{"Set up"},
			wantNotIn: []string{"repetition guidance"},
		},
		{
			name: "leaves Common Mistakes alone (would be irregular hit otherwise)",
			input: "## Instructions\n1. Set up.\n\n## Common Mistakes\n" +
				"- Doing 50 reps at once: pace yourself.\n",
			wantHas: []string{"Doing 50 reps at once"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := StripRepGuidanceLines(tt.input)
			for _, want := range tt.wantHas {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q; got:\n%s", want, got)
				}
			}
			for _, notWant := range tt.wantNotIn {
				if strings.Contains(got, notWant) {
					t.Errorf("found %q but should be absent; got:\n%s", notWant, got)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/exercise-content-fixup -run TestStripRepGuidanceLines`
Expected: FAIL — `StripRepGuidanceLines` undefined.

- [ ] **Step 3: Implement the transform**

Append to `cmd/exercise-content-fixup/transforms.go`:

```go
// Patterns that indicate rep / set / duration mentions that don't belong in
// description text (the app tracks these separately on the exercise).
//
// Each pattern is intentionally narrow: it must match phrases that mean
// "do N reps / sets / seconds" and must NOT match "Take 2 deep breaths",
// "3-second tempo", or rep mentions inside Common Mistakes prose like
// "Doing 50 reps at once: pace yourself" (the colon + advice marks it as
// a mistake to correct, not a target — keeping it is fine).
var repGuidancePatterns = []*regexp.Regexp{
	// "perform 8-12 reps", "perform 10 repetitions", "perform 8 times"
	regexp.MustCompile(`(?i)\bperform\s+\d+(?:\s*[-–]\s*\d+)?\s*(?:reps?|repetitions?|times)\b`),
	// "do 8-12 reps", "do 10 repetitions"
	regexp.MustCompile(`(?i)\bdo\s+\d+(?:\s*[-–]\s*\d+)?\s*(?:reps?|repetitions?)\b`),
	// "complete 3 sets"
	regexp.MustCompile(`(?i)\bcomplete\s+\d+(?:\s*[-–]\s*\d+)?\s*sets?\b`),
	// "8-12 reps" / "8 reps" / "3 sets" at the start of an instruction step
	regexp.MustCompile(`(?i)^\s*\d+\.\s*\d+(?:\s*[-–]\s*\d+)?\s*(?:reps?|repetitions?|sets?)\b`),
	// "hold for 30 seconds", "hold for 5s"
	regexp.MustCompile(`(?i)\bhold\s+for\s+\d+\s*(?:seconds?|s)\b`),
	// Literal template-leak phrase
	regexp.MustCompile(`(?i)\brepetition guidance\b`),
}

// orderedListItemPattern matches a markdown ordered-list item like "5. text".
var orderedListItemPattern = regexp.MustCompile(`^\s*\d+\.\s+`)

// StripRepGuidanceLines drops ordered-list items in the ## Instructions
// section whose text matches any repGuidancePatterns entry. Other sections
// (## Common Mistakes, ## Resources) are passed through unchanged — rep
// mentions there describe errors to avoid, not targets to hit.
func StripRepGuidanceLines(desc string) string {
	lines := strings.Split(desc, "\n")
	out := make([]string, 0, len(lines))
	inInstructions := false

	for _, line := range lines {
		if strings.HasPrefix(line, "## Instructions") {
			inInstructions = true
			out = append(out, line)
			continue
		}
		if inInstructions && strings.HasPrefix(line, "##") {
			inInstructions = false
		}

		if inInstructions && orderedListItemPattern.MatchString(line) && matchesRepGuidance(line) {
			continue
		}
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

func matchesRepGuidance(line string) bool {
	for _, p := range repGuidancePatterns {
		if p.MatchString(line) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./cmd/exercise-content-fixup -run TestStripRepGuidanceLines`
Expected: PASS on all eight subtests.

- [ ] **Step 5: Commit**

```bash
git add cmd/exercise-content-fixup/transforms.go cmd/exercise-content-fixup/transforms_test.go
git commit -m "feat(cmd): add StripRepGuidanceLines transform"
```

---

### Task 8: Implement the link checker for the backfill tool

**Files:**
- Create: `cmd/exercise-content-fixup/linkcheck.go`
- Modify: `cmd/exercise-content-fixup/transforms_test.go` (cover regex extraction helper)

Provide `ExtractResourceURLs(desc string) []string` so the main loop can collect every URL across all exercises before HEAD-checking, deduplicating to one network call per unique URL. The checker itself is `CheckURLs(ctx context.Context, urls []string) map[string]bool` — same 5s timeout, same 2xx/3xx-pass logic as the runtime validator.

- [ ] **Step 1: Add the URL-extractor test**

Append to `cmd/exercise-content-fixup/transforms_test.go`:

```go
func TestExtractResourceURLs(t *testing.T) {
	t.Parallel()

	desc := "## Instructions\n1. Set up.\n\n## Resources\n" +
		"- [A](https://a.example.org/x)\n" +
		"- [B](http://b.example.org/y)\n" +
		"- [C without link]\n" +
		"- [D](https://a.example.org/x)\n" // duplicate

	got := ExtractResourceURLs(desc)

	want := []string{
		"https://a.example.org/x",
		"http://b.example.org/y",
		"https://a.example.org/x", // ExtractResourceURLs preserves duplicates; caller dedupes
	}
	if len(got) != len(want) {
		t.Fatalf("got %d URLs, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("URL[%d] = %q, want %q", i, got[i], w)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/exercise-content-fixup -run TestExtractResourceURLs`
Expected: FAIL — `ExtractResourceURLs` undefined.

- [ ] **Step 3: Implement extractor + checker**

Append the extractor to `cmd/exercise-content-fixup/transforms.go`:

```go
// ExtractResourceURLs returns every URL referenced by a markdown resource
// list item (`- [Title](URL)`) anywhere in desc, in the order they appear.
// Duplicates are preserved so the caller can decide on deduplication.
func ExtractResourceURLs(desc string) []string {
	var urls []string
	for _, line := range strings.Split(desc, "\n") {
		m := resourceLinkPattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		urls = append(urls, m[1])
	}
	return urls
}
```

Create `cmd/exercise-content-fixup/linkcheck.go`:

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CheckURLs HEAD-requests every URL with a 5s per-request timeout and
// returns a map from URL to alive-status (true iff the final response is
// 2xx or 3xx). Placeholder URLs are skipped — they are always treated as
// dead.
//
// Network failures, timeouts, and 4xx/5xx responses all map to false.
// Progress is printed to stdout so a human watching the run can see what's
// happening on a list of ~150 URLs.
func CheckURLs(ctx context.Context, urls []string) map[string]bool {
	client := &http.Client{Timeout: 5 * time.Second}
	results := make(map[string]bool, len(urls))

	for _, u := range urls {
		if _, seen := results[u]; seen {
			continue
		}
		if strings.HasPrefix(u, placeholderURLPrefix) {
			results[u] = false
			fmt.Printf("  [dead]  %s  (placeholder)\n", u)
			continue
		}
		alive, reason := headCheck(ctx, client, u)
		results[u] = alive
		if alive {
			fmt.Printf("  [live]  %s\n", u)
		} else {
			fmt.Printf("  [dead]  %s  (%s)\n", u, reason)
		}
	}
	return results
}

func headCheck(ctx context.Context, client *http.Client, url string) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, fmt.Sprintf("bad url: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err.Error()
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false, fmt.Sprintf("status %d", resp.StatusCode)
	}
	return true, ""
}
```

- [ ] **Step 4: Run the extractor test**

Run: `go test -v ./cmd/exercise-content-fixup -run TestExtractResourceURLs`
Expected: PASS.

Run the full package suite to confirm nothing regressed:

Run: `go test ./cmd/exercise-content-fixup -count=1`
Expected: PASS on all transforms tests.

- [ ] **Step 5: Commit**

```bash
git add cmd/exercise-content-fixup/transforms.go cmd/exercise-content-fixup/transforms_test.go cmd/exercise-content-fixup/linkcheck.go
git commit -m "feat(cmd): add URL extractor and HEAD-check link checker"
```

---

### Task 9: Wire main.go to apply transforms and emit SQL

**Files:**
- Modify: `cmd/exercise-content-fixup/main.go`

Replace the row-counting skeleton with the real pipeline: collect URLs, HEAD-check them, apply transforms, emit `UPDATE` statements for changed rows, print a summary.

- [ ] **Step 1: Replace `run` with the full pipeline**

Replace the body of `cmd/exercise-content-fixup/main.go` with:

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	"flag"
	"log"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	var (
		dbPath  = flag.String("db", "", "Path to the SQLite snapshot to read (required)")
		outPath = flag.String("out", "", "Path to write the generated UPDATE SQL (required)")
	)
	flag.Parse()

	if *dbPath == "" || *outPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(context.Background(), *dbPath, *outPath); err != nil {
		log.Fatalf("exercise-content-fixup: %v", err)
	}
}

type exerciseRow struct {
	id   int
	name string
	desc string
}

func run(ctx context.Context, dbPath, outPath string) error {
	db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer func() { _ = db.Close() }()

	rows, err := readExercises(ctx, db)
	if err != nil {
		return err
	}

	// Collect every URL across every exercise, dedupe, HEAD-check.
	allURLs := collectURLs(rows)
	fmt.Printf("checking %d unique URLs...\n", len(allURLs))
	alive := CheckURLs(ctx, allURLs)

	updates := transformAll(rows, alive)

	if err = writeSQL(outPath, updates); err != nil {
		return fmt.Errorf("write sql: %w", err)
	}

	fmt.Printf("\nsummary: scanned %d exercises, %d modified, output → %s\n",
		len(rows), len(updates), outPath)
	return nil
}

func readExercises(ctx context.Context, db *sql.DB) ([]exerciseRow, error) {
	rs, err := db.QueryContext(ctx,
		"SELECT id, name, description_markdown FROM exercises ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("select exercises: %w", err)
	}
	defer func() { _ = rs.Close() }()

	var out []exerciseRow
	for rs.Next() {
		var r exerciseRow
		if err = rs.Scan(&r.id, &r.name, &r.desc); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		out = append(out, r)
	}
	if err = rs.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return out, nil
}

func collectURLs(rows []exerciseRow) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, r := range rows {
		for _, u := range ExtractResourceURLs(r.desc) {
			if seen[u] {
				continue
			}
			seen[u] = true
			unique = append(unique, u)
		}
	}
	return unique
}

type updateRow struct {
	id      int
	name    string
	newDesc string
}

func transformAll(rows []exerciseRow, alive map[string]bool) []updateRow {
	var updates []updateRow
	for _, r := range rows {
		newDesc := StripDeadResourceLinks(r.desc, alive)
		newDesc = StripRepGuidanceLines(newDesc)
		if newDesc == r.desc {
			continue
		}
		fmt.Printf("\n--- exercise %d (%s) ---\n", r.id, r.name)
		fmt.Println(diffSummary(r.desc, newDesc))
		updates = append(updates, updateRow{id: r.id, name: r.name, newDesc: newDesc})
	}
	return updates
}

// diffSummary produces a tiny human-readable change report — line counts
// before/after and the first three dropped lines so a reviewer can spot
// obviously wrong transforms before shipping the SQL.
func diffSummary(before, after string) string {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	dropped := diffLines(beforeLines, afterLines)
	if len(dropped) == 0 {
		return "(no line drops — markdown structure changed in place)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %d lines → %d lines, dropped:\n", len(beforeLines), len(afterLines))
	for i, l := range dropped {
		if i == 3 {
			fmt.Fprintf(&b, "    ... and %d more\n", len(dropped)-3)
			break
		}
		fmt.Fprintf(&b, "    - %s\n", strings.TrimSpace(l))
	}
	return b.String()
}

func diffLines(before, after []string) []string {
	idx := make(map[string]int, len(after))
	for _, l := range after {
		idx[l]++
	}
	var dropped []string
	for _, l := range before {
		if idx[l] > 0 {
			idx[l]--
			continue
		}
		dropped = append(dropped, l)
	}
	return dropped
}

func writeSQL(path string, updates []updateRow) error {
	var b strings.Builder
	b.WriteString("-- Generated by cmd/exercise-content-fixup.\n")
	b.WriteString("-- Apply via: make fly-sql-write SCRIPT=docs/<this-file>\n")
	b.WriteString("BEGIN TRANSACTION;\n\n")
	for _, u := range updates {
		fmt.Fprintf(&b, "-- %d: %s\n", u.id, u.name)
		fmt.Fprintf(&b, "UPDATE exercises SET description_markdown = '%s' WHERE id = %d;\n\n",
			sqlEscape(u.newDesc), u.id)
	}
	b.WriteString("COMMIT;\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// sqlEscape doubles single quotes for safe inlining of a literal into a
// SQLite UPDATE statement. The input is description markdown sourced from
// our own DB — it does not contain injection-grade payloads but does
// frequently contain apostrophes (e.g. "don't").
func sqlEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./cmd/exercise-content-fixup`
Expected: builds clean.

- [ ] **Step 3: Smoke-test against the dev DB**

If `petrapp.sqlite3` is absent, run `make init`. Then:

Run: `./exercise-content-fixup -db petrapp.sqlite3 -out /tmp/fixup-test.sql 2>&1 | head -60`
Expected:
- Output begins with `checking N unique URLs...` where N is some count.
- For each URL: `[live]` or `[dead]` line.
- For each modified exercise: a `--- exercise <id> (<name>) ---` block with a small diff summary.
- A final `summary: scanned <N> exercises, <M> modified, output → /tmp/fixup-test.sql` line.

Inspect `/tmp/fixup-test.sql`:

Run: `head -20 /tmp/fixup-test.sql`
Expected: a `BEGIN TRANSACTION` header followed by `-- <id>: <name>` comments and `UPDATE exercises SET description_markdown = ... WHERE id = N;` statements.

Clean up: `rm exercise-content-fixup /tmp/fixup-test.sql`.

- [ ] **Step 4: Confirm `make lint-fix` is clean**

Run: `make lint-fix`
Expected: clean exit. If `funlen` flags `run` or `transformAll`, split the function further; the 100-line limit is enforced (see `CLAUDE.md`).

- [ ] **Step 5: Run all tests**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/exercise-content-fixup/main.go
git commit -m "feat(cmd): wire transforms and emit UPDATE SQL"
```

---

### Task 10: Ship the cleanup — manual rollout

This is the deploy step, not new code. It produces three artifacts: the prod SQL one-shot, the post-mortem doc, and the synced fixtures.sql.

- [ ] **Step 1: Deploy the generator changes to production**

The generator changes from Tasks 1–4 must reach production *before* the snapshot is taken — otherwise the snapshot picks up freshly-generated bad data while we're trying to clean up.

Confirm the commits from Tasks 1–4 have landed on `main` and the deploy has rolled out. Check with:

Run: `git log --oneline origin/main | head -10`

Expected: commits from this plan visible. Then check the live deploy via whatever the team's deploy verification step is (likely `fly status`; consult the `fly-ops` skill).

- [ ] **Step 2: Take a prod snapshot**

Consult the `fly-ops` skill (`Skill` tool) for the canonical read-only snapshot flow against the Fly Machine. The output is a `.sqlite3` file on the local filesystem; record the path.

- [ ] **Step 3: Run the tool**

```bash
go build ./cmd/exercise-content-fixup
./exercise-content-fixup \
    -db <path-to-snapshot.sqlite3> \
    -out docs/2026-05-27-exercise-content-cleanup.sql \
    2>&1 | tee /tmp/fixup-run.log
```

- [ ] **Step 4: Review the diff log and the SQL**

Read `/tmp/fixup-run.log` end-to-end. For each `--- exercise N (name) ---` block, sanity-check that the dropped lines look correct. Open `docs/2026-05-27-exercise-content-cleanup.sql` and skim a few `UPDATE` statements end-to-end.

If anything looks wrong, do NOT ship the SQL. Fix the transform regex in `cmd/exercise-content-fixup/transforms.go`, rerun, review again.

- [ ] **Step 5: Sync `internal/sqlite/fixtures.sql`**

Apply the same content changes by hand to `internal/sqlite/fixtures.sql`. For each modified exercise in the diff log:

- Find the matching `INSERT INTO exercises ... VALUES (id, 'name', ...)` row in `fixtures.sql`.
- Edit the `description_markdown` literal to match the cleaned version. Pay attention to escaped single quotes (`''` in SQL).

Run: `make test`
Expected: PASS (fixtures are re-applied on every boot; any syntax error here would fail tests).

- [ ] **Step 6: Write the post-mortem**

Create `docs/2026-05-27-exercise-content-cleanup.md` modeled on `docs/2026-05-13-exercise-cleanup.md`:

```markdown
# Exercise content cleanup — 2026-05-27

## Context

Companion to the generator fixes in [spec](superpowers/specs/2026-05-27-exercise-data-quality-design.md).
The new prompt and URL validator stop new exercises from shipping with
dead links and redundant rep guidance, but the existing N exercises in
production still have the old content. This script applies the same
cleanup transforms to the live rows.

## What this script changes

Generated by `cmd/exercise-content-fixup`. Per modified exercise:

- Resource list items pointing to URLs that 404, time out, or use the
  `https://example.com/` placeholder prefix were removed.
- Resource sections that lost all their links had the `## Resources`
  heading removed too.
- Ordered-list items in `## Instructions` that mentioned rep counts,
  set counts, or hold durations were removed.

## Run

\`\`\`
make fly-sql-write SCRIPT=docs/2026-05-27-exercise-content-cleanup.sql FLY_APP=petra
\`\`\`

The make target snapshots the live DB to `/data/snapshots/` before
applying the script.
```

(Replace `N` with the actual modified count from the run log.)

- [ ] **Step 7: Commit**

```bash
git add docs/2026-05-27-exercise-content-cleanup.sql \
        docs/2026-05-27-exercise-content-cleanup.md \
        internal/sqlite/fixtures.sql
git commit -m "fix(exercises): clean dead links and rep guidance from existing rows"
```

Then push to `main` for review.

- [ ] **Step 8: Apply the SQL to prod**

After the commit is merged:

```bash
make fly-sql-write SCRIPT=docs/2026-05-27-exercise-content-cleanup.sql FLY_APP=petra
```

Verify by spot-checking a known-affected exercise via the exercise-info page in production.

- [ ] **Step 9: Clean up**

Delete the local snapshot file and the `exercise-content-fixup` binary. Leave the `cmd/exercise-content-fixup/` source in the repo — future link-rot sweeps rerun it.

---

## Self-Review Notes

- **Spec coverage:** Every section of the spec maps to tasks:
  - "Prompt: drop rep guidance, add stabilizer rule, remove placeholder URLs" → Task 1.
  - "Pass 2: validate URLs before injecting them" → Task 2 + Task 4.
  - "updateResourcesInDescription" empty-resources fix → Task 3.
  - Backfill `StripDeadResourceLinks` → Task 6.
  - Backfill `StripRepGuidanceLines` → Task 7.
  - Backfill HEAD-checker + URL extractor → Task 8.
  - Backfill CLI + SQL emission → Tasks 5 + 9.
  - Workflow + fixtures sync + post-mortem → Task 10.
  - Testing requirements (httptest URL validator, prompt assertions, table-driven transform tests) → Tasks 1, 2, 6, 7.
- **Live OpenAI test extension** (spec mentions adding a rep-mention regex assertion to the live test). Deliberately not in a dedicated task — adding it is a one-line follow-up the engineer can do during Task 1's test edit if convenient, but the gated nature of the live test (skipped without `OPENAI_API_KEY`) makes it low-value for routine CI. If desired, fold it into Task 1 Step 1 alongside the other assertions.
