# Add Exercise — Name Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port the swap page's `?q=` name filter to `/workouts/{date}/add-exercise` so users can narrow a long exercise list by typing a substring.

**Architecture:** In-handler `strings.Contains` filter on a lowercased name (mirrors `workoutSwapExerciseGET`), `Query` field threaded through `exerciseAddTemplateData` to a copied search form and `.no-results` empty state in the template. No service-layer changes, no shared component, no JavaScript.

**Tech Stack:** Go stdlib (`strings`, `net/http`), `html/template`, `goquery` for e2e DOM assertions, `e2etest` package for the test server.

**Reference spec:** `docs/superpowers/specs/2026-05-02-exercise-add-search-design.md`

**Reference implementation (port source):**
- Handler: `cmd/web/handler-workout.go:166-236` (`workoutSwapExerciseGET`) and `:277-285` (`exerciseSwapTemplateData`)
- Template: `ui/templates/pages/exercise-swap/exercise-swap.gohtml` lines `94-125` (search section) and `189-279` (list with `{{ if … }} … {{ else }} … <p class="no-results"> … </p> {{ end }}`)
- Test: `cmd/web/handler-exerciseset_test.go:394-501` (`Test_application_workoutSwapExercise_search_filters_by_name`)

---

## File Structure

- **Modify** `cmd/web/handler-workout.go` — add `Query` to `exerciseAddTemplateData`; in `workoutAddExerciseGET` read `?q=`, lowercase, filter by name in the existing loop.
- **Modify** `ui/templates/pages/exercise-add/exercise-add.gohtml` — add search section (form + scoped CSS) and the `{{ if … }} … {{ else }} <p class="no-results">…</p> {{ end }}` empty-state branch around the list. Add a `.no-results` rule into the existing `available-exercises` scoped style.
- **Modify** `cmd/web/handler-workout_test.go` — add `Test_application_workoutAddExercise_search_filters_by_name`. May need new imports (`net/url`, `strings`).

No new files. The existing `Test_application_addWorkout` keeps working unchanged because it doesn't pass `?q=`.

---

## Task 1: Add e2e test for the name filter (failing)

**Files:**
- Modify: `cmd/web/handler-workout_test.go` (currently imports only `net/http`, `testing`, `time`, `goquery`, `e2etest`, `testhelpers`)

- [ ] **Step 1: Write the failing test**

Append this function at the end of `cmd/web/handler-workout_test.go` (after `Test_application_workoutNotFound`). The structure mirrors `Test_application_workoutSwapExercise_search_filters_by_name` in `handler-exerciseset_test.go:394-501`.

You'll also need to add imports for `net/url` and `strings` to the import block at the top of the file.

```go
// Test_application_workoutAddExercise_search_filters_by_name verifies that the
// add-exercise page filters available exercises by name substring
// (case-insensitive) when ?q= is set, echoes the query into the search input,
// and renders an empty state when nothing matches.
func Test_application_workoutAddExercise_search_filters_by_name(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	formData := map[string]string{time.Now().Weekday().String(): "60"}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("Start workout: %v", err)
	}

	// Collect the unfiltered set of available exercise names so the test is
	// data-driven against fixtures rather than coupled to specific names.
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise"); err != nil {
		t.Fatalf("Get add-exercise page: %v", err)
	}
	var allNames []string
	doc.Find(".exercise-option .exercise-name").Each(func(_ int, s *goquery.Selection) {
		allNames = append(allNames, strings.TrimSpace(s.Text()))
	})
	if len(allNames) < 2 {
		t.Fatalf("Need at least 2 available exercises to exercise the filter, got %d", len(allNames))
	}

	// Pick the first word of the first exercise name as the search substring.
	// Mixed case to verify case-insensitivity.
	firstWord := strings.Fields(allNames[0])[0]
	if len(firstWord) < 3 {
		t.Fatalf("First word too short to be a useful filter: %q", firstWord)
	}
	needle := strings.ToUpper(firstWord)

	// Filtered query should return a (non-empty) subset, all containing the
	// substring case-insensitively.
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise?q="+url.QueryEscape(needle)); err != nil {
		t.Fatalf("Get add-exercise page with query: %v", err)
	}
	var filteredNames []string
	doc.Find(".exercise-option .exercise-name").Each(func(_ int, s *goquery.Selection) {
		filteredNames = append(filteredNames, strings.TrimSpace(s.Text()))
	})
	if len(filteredNames) == 0 {
		t.Fatalf("Expected at least one match for %q, got none", needle)
	}
	if len(filteredNames) > len(allNames) {
		t.Errorf("Filtered list (%d) larger than unfiltered (%d)", len(filteredNames), len(allNames))
	}
	needleLower := strings.ToLower(needle)
	for _, name := range filteredNames {
		if !strings.Contains(strings.ToLower(name), needleLower) {
			t.Errorf("Result %q does not contain %q", name, needle)
		}
	}

	// Search input must echo the query so reloading preserves state.
	gotQuery, _ := doc.Find("input[name='q']").Attr("value")
	if gotQuery != needle {
		t.Errorf("Search input value = %q, want %q", gotQuery, needle)
	}

	// A query that matches nothing must render the empty-state copy and no
	// add forms.
	noMatch := "zzznotreal"
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise?q="+url.QueryEscape(noMatch)); err != nil {
		t.Fatalf("Get add-exercise page with no-match query: %v", err)
	}
	if doc.Find(".exercise-option").Length() != 0 {
		t.Error("Expected zero exercise options when query matches nothing")
	}
	emptyState := doc.Find(".no-results")
	if emptyState.Length() == 0 {
		t.Fatal("Expected .no-results empty state when query matches nothing")
	}
	if !strings.Contains(emptyState.Text(), noMatch) {
		t.Errorf("Empty state %q should echo the query %q", strings.TrimSpace(emptyState.Text()), noMatch)
	}
}
```

Update the import block in `cmd/web/handler-workout_test.go` so it reads:

```go
import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_workoutAddExercise_search_filters_by_name`

Expected: FAIL. The page won't have an `input[name='q']` (so `gotQuery` will be empty) and the no-match query will return the full unfiltered list (so `.exercise-option` count won't be zero and there's no `.no-results` element). The exact failure surfaces depend on order — likely the empty-state branch first.

- [ ] **Step 3: Commit the failing test**

```bash
git add cmd/web/handler-workout_test.go
git commit -m "test(workout): cover name-search filter on add-exercise page"
```

Note: committing a known-failing test is intentional here — it pins the contract before implementation. The next task makes it pass.

---

## Task 2: Wire `?q=` filter through the handler

**Files:**
- Modify: `cmd/web/handler-workout.go` (specifically `workoutAddExerciseGET` at lines 295-338 and `exerciseAddTemplateData` at lines 287-292)

- [ ] **Step 1: Add `Query` field to the template data struct**

In `cmd/web/handler-workout.go`, locate `exerciseAddTemplateData` (around lines 287-292) and add the `Query` field. The existing struct:

```go
// exerciseAddTemplateData contains data for the exercise add template.
type exerciseAddTemplateData struct {
	BaseTemplateData
	Date      time.Time
	Exercises []workout.Exercise
}
```

becomes:

```go
// exerciseAddTemplateData contains data for the exercise add template.
type exerciseAddTemplateData struct {
	BaseTemplateData
	Date      time.Time
	Exercises []workout.Exercise
	Query     string
}
```

- [ ] **Step 2: Read and apply the query in the handler**

Replace the body of `workoutAddExerciseGET` (lines 295-338) with the version below. The diff vs current: read `query` early, lowercase it once, skip non-matching names inside the existing filter loop, and pass `Query` to the template.

```go
// workoutAddExerciseGET handles GET requests to show available exercises for adding.
func (app *application) workoutAddExerciseGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	date, ok := app.parseDateParam(w, r)
	if !ok {
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// Get the current workout session to see which exercises are already included
	session, err := app.workoutService.GetSession(r.Context(), date)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Create a map of exercise IDs that are already in the workout
	existingExerciseIDs := make(map[int]bool)
	for _, exerciseSet := range session.ExerciseSets {
		existingExerciseIDs[exerciseSet.Exercise.ID] = true
	}

	// Get all exercises
	allExercises, err := app.workoutService.List(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	queryLower := strings.ToLower(query)
	var availableExercises []workout.Exercise
	for _, exercise := range allExercises {
		if existingExerciseIDs[exercise.ID] {
			continue
		}
		if queryLower != "" && !strings.Contains(strings.ToLower(exercise.Name), queryLower) {
			continue
		}
		availableExercises = append(availableExercises, exercise)
	}

	// Prepare template data
	data := exerciseAddTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Date:             date,
		Exercises:        availableExercises, // Use filtered exercises instead of all exercises
		Query:            query,
	}

	app.render(w, r, http.StatusOK, "exercise-add", data)
}
```

`strings` is already imported by this file (line 9), no import change needed.

- [ ] **Step 3: Build to confirm the handler compiles**

Run: `go build ./cmd/web`

Expected: no output (success).

- [ ] **Step 4: Run the e2e test — it should still fail on UI assertions**

Run: `go test -v ./cmd/web -run Test_application_workoutAddExercise_search_filters_by_name`

Expected: FAIL. The handler now filters correctly so a `?q=` request returns the right slice, but the template doesn't render `<input name="q">` or `.no-results` yet. The assertion that fails first is `gotQuery != needle` (empty string vs the needle) or the `.no-results` length check.

- [ ] **Step 5: Commit the handler change**

```bash
git add cmd/web/handler-workout.go
git commit -m "feat(workout): filter add-exercise list by ?q= name substring"
```

---

## Task 3: Add search form and empty state to the template

**Files:**
- Modify: `ui/templates/pages/exercise-add/exercise-add.gohtml`

- [ ] **Step 1: Insert the search section between the header and the available-exercises section**

Open `ui/templates/pages/exercise-add/exercise-add.gohtml`. The current file has:
- `<header>` with the back-link and `<h1>Add Exercise</h1>` (lines 16-47)
- Immediately followed by `<section class="available-exercises">` (line 49)

Between them — i.e. after the closing `</header>` on line 47 and before `<section class="available-exercises">` on line 49 — insert this block. It is a verbatim copy of the search section in `ui/templates/pages/exercise-swap/exercise-swap.gohtml` lines 94-125:

```gohtml
        <section class="exercise-search">
            <style {{ nonce }}>
                @scope {
                    :scope {
                        form {
                            display: flex;
                            gap: var(--size-2);
                        }

                        input[type="search"] {
                            flex: 1;
                            padding: var(--size-2) var(--size-3);
                            border: 1px solid var(--gray-3);
                            border-radius: var(--radius-2);
                            font-size: var(--font-size-1);
                            background: var(--white);
                        }
                    }
                }
            </style>
            <form method="get" role="search">
                <label for="exercise-search-q" class="sr-only">Search exercises</label>
                <input id="exercise-search-q"
                       type="search"
                       name="q"
                       value="{{ .Query }}"
                       placeholder="Search exercises…"
                       autocomplete="off"
                       inputmode="search">
                <button type="submit">Search</button>
            </form>
        </section>
```

- [ ] **Step 2: Add the `.no-results` rule to the available-exercises scoped style**

In the same file, find the scoped `<style {{ nonce }}>` block inside `<section class="available-exercises">` (originally lines 50-102 — after the insert from Step 1, the line numbers will shift down; locate it by content). Inside the `:scope { … }` block, after the existing `.add-button` rule and before the closing `}` of `:scope`, add:

```css
                        .no-results {
                            color: var(--gray-6);
                            padding: var(--size-4) 0;
                            text-align: center;
                        }
```

For reference, the current `.add-button` rule ends like this:

```css
                        .add-button {
                            background-color: var(--lime-2);
                            color: var(--lime-9);
                            border: none;
                            border-radius: var(--radius-2);
                            padding: var(--size-2) var(--size-3);
                            font-weight: var(--font-weight-6);
                            cursor: pointer;
                            width: 100%;

                            &:hover {
                                background-color: var(--lime-3);
                            }
                        }
```

The new `.no-results` rule goes immediately after that closing `}`, inside the same `:scope { … }`.

- [ ] **Step 3: Wrap the exercise list in an if/else with the empty state**

In the same file, find the `<div class="exercises-list">` block (originally starting at line 105 in the unmodified file; line numbers shift after Steps 1-2). The block contains a single `{{ range .Exercises }} … {{ end }}` with a `<div class="exercise-option">` body. The body itself is **not changed by this step** — only the framing around it.

Make exactly two edits:

**Edit A** — find this single line (~16 spaces of indent) and replace it with two lines:

Find:
```gohtml
                {{ range .Exercises }}
```

Replace with:
```gohtml
                {{ if .Exercises }}
                {{ range .Exercises }}
```

**Edit B** — find the `{{ end }}` that closes the `range` (the one immediately before `</div>` of `exercises-list`) and replace it with a `{{ end }}` followed by the `{{ else }}` branch and the outer `{{ end }}`.

Find (this is the second-to-last line of the `<div class="exercises-list">` block, just before `            </div>`):
```gohtml
                {{ end }}
```

Replace with:
```gohtml
                {{ end }}
                {{ else }}
                    <p class="no-results">
                        {{ if .Query }}No exercises match &ldquo;{{ .Query }}&rdquo;.{{ else }}No exercises available to add.{{ end }}
                    </p>
                {{ end }}
```

If your editor finds multiple `                {{ end }}` matches inside the file, use surrounding context to disambiguate — the target one is the `{{ end }}` whose next non-blank line is `            </div>` (closing the `exercises-list` div), located near the bottom of the file just before `        </section>`.

After both edits, the framing around the unchanged `<div class="exercise-option">…</div>` body looks like:

```gohtml
            <div class="exercises-list">
                {{ if .Exercises }}
                {{ range .Exercises }}
                    <div class="exercise-option">
                        <!-- unchanged: name, details, post form, dialog, info button -->
                    </div>
                {{ end }}
                {{ else }}
                    <p class="no-results">
                        {{ if .Query }}No exercises match &ldquo;{{ .Query }}&rdquo;.{{ else }}No exercises available to add.{{ end }}
                    </p>
                {{ end }}
            </div>
```

- [ ] **Step 4: Run the e2e test — should now pass**

Run: `go test -v ./cmd/web -run Test_application_workoutAddExercise_search_filters_by_name`

Expected: PASS.

- [ ] **Step 5: Run the full test suite to confirm no regressions**

Run: `make test`

Expected: all tests pass. Pay particular attention to `Test_application_addWorkout` (in `handler-workout_test.go`) — it submits `/workouts/{today}/add-exercise` without `?q=` and clicks the first add button, so it must still work. If it fails, the most likely cause is that the wrapping `{{ if }}/{{ else }}/{{ end }}` was placed incorrectly and the `<form method="post">` inside `.exercise-option` was lost.

- [ ] **Step 6: Run lint to catch style issues**

Run: `make lint-fix`

Expected: no errors.

- [ ] **Step 7: Commit the template change**

```bash
git add ui/templates/pages/exercise-add/exercise-add.gohtml
git commit -m "feat(workout): render search box and empty state on add-exercise page"
```

---

## Task 4: Final verification

- [ ] **Step 1: Run the full CI pipeline**

Run: `make ci`

Expected: init, build, lint, test, sec all pass.

- [ ] **Step 2: Manual smoke (optional, recommended)**

Start the dev server (`make run` or whatever the project's dev entry point is — check the Makefile if unsure), register, set today's weekday in preferences, start a workout, navigate to "Add Exercise":

1. Verify the search input renders above the exercise list.
2. Type a substring matching at least one exercise (e.g. "press") and click Search. URL becomes `?q=press`, list narrows, input keeps the value.
3. Type `zzznotreal`, click Search. URL becomes `?q=zzznotreal`, list shows the empty-state paragraph quoting the query, no exercise cards.
4. Clear the input, click Search. Full list returns.
5. With a real query and at least one match, click an Add button — exercise is added, redirect to the workout page works (this exercises the unchanged POST path).

- [ ] **Step 3: No commit needed** if `make ci` passed — Tasks 1-3 already committed the change. If `make lint-fix` made any auto-fixes, commit them now:

```bash
git status
# if any files changed:
git add <files>
git commit -m "chore: lint fixes"
```

---

## Out of scope (do not implement)

- Similarity sort (the swap page also has this from commit `b0777d7`; it does not apply to add — there's no current exercise to compare against).
- A shared search component or `filterExercisesByName` helper. Threshold for extraction in this codebase is three call sites (per `ui/templates/CLAUDE.md`); this is the second.
- Service-layer changes — filtering stays handler-side.
- JavaScript live-navigation enhancement. The swap page tried this and reverted in PR #79's second commit.
- Any change to `workoutAddExercisePOST` or `workoutService.AddExercise`.
