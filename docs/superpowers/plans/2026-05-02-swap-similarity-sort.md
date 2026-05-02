# Swap Exercise Similarity Sort — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sort the Swap Exercise list by muscle-group overlap with the current exercise, so the most relevant alternatives appear first.

**Architecture:** Add a pure scoring function `workout.SwapSimilarityScore(current, candidate Exercise) int` in a new file `internal/workout/swap.go`. Modify `workoutSwapExerciseGET` in `cmd/web/handler-workout.go` to sort the already-filtered candidate list by that score (descending), with name as a stable tie-break. No template, schema, or data changes.

**Tech Stack:** Go 1.22+, stdlib `sort.SliceStable`, existing `goquery`-based handler tests in `cmd/web/handler-exerciseset_test.go`.

**Spec:** [`docs/superpowers/specs/2026-05-02-swap-similarity-sort-design.md`](../specs/2026-05-02-swap-similarity-sort-design.md)

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/workout/swap.go` | Create | Pure `SwapSimilarityScore` function and the small `countShared` helper |
| `internal/workout/swap_test.go` | Create | Table-driven unit tests for the score formula |
| `cmd/web/handler-workout.go` | Modify | Add `sort` import; sort `compatibleExercises` after the existing filter loop |
| `cmd/web/handler-exerciseset_test.go` | Modify | Append a new e2e test that asserts the rendered list is in score-descending order |

`swap.go` is a self-contained unit — pure function, no I/O, no service dependencies. It does not belong in `service.go` (no `Service` receiver, no DB access) or `models.go` (it's behavior over models, not a model definition).

---

## Task 1: Pure scoring function with unit tests

**Files:**
- Create: `internal/workout/swap_test.go`
- Create: `internal/workout/swap.go`

- [ ] **Step 1: Write the failing unit tests**

Create `internal/workout/swap_test.go` with table-driven tests covering each weight independently, the category bonus, empty slices, and symmetry:

```go
package workout

import "testing"

func TestSwapSimilarityScore(t *testing.T) {
	tests := []struct {
		name    string
		current Exercise
		other   Exercise
		want    int
	}{
		{
			name: "identical primary, secondary, and category",
			current: Exercise{
				Category:              CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: Exercise{
				Category:              CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			want: 4*2 + 1 + 3, // 12
		},
		{
			name: "one primary muscle in common, same category",
			current: Exercise{
				Category:            CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: Exercise{
				Category:            CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			want: 4 + 3, // 7
		},
		{
			name: "current primary matches candidate secondary, same category",
			current: Exercise{
				Category:            CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: Exercise{
				Category:              CategoryUpper,
				SecondaryMuscleGroups: []string{"Chest"},
			},
			want: 2 + 3, // 5
		},
		{
			name: "current secondary matches candidate primary, same category",
			current: Exercise{
				Category:              CategoryUpper,
				SecondaryMuscleGroups: []string{"Chest"},
			},
			other: Exercise{
				Category:            CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			want: 2 + 3, // 5
		},
		{
			name: "secondary↔secondary match, same category",
			current: Exercise{
				Category:              CategoryUpper,
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: Exercise{
				Category:              CategoryUpper,
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			want: 1 + 3, // 4
		},
		{
			name: "disjoint muscles, same category",
			current: Exercise{
				Category:            CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: Exercise{
				Category:            CategoryUpper,
				PrimaryMuscleGroups: []string{"Biceps"},
			},
			want: 3,
		},
		{
			name: "disjoint muscles, different category",
			current: Exercise{
				Category:            CategoryUpper,
				PrimaryMuscleGroups: []string{"Chest"},
			},
			other: Exercise{
				Category:            CategoryLower,
				PrimaryMuscleGroups: []string{"Quads"},
			},
			want: 0,
		},
		{
			name:    "empty slices, different category",
			current: Exercise{Category: CategoryUpper},
			other:   Exercise{Category: CategoryLower},
			want:    0,
		},
		{
			name:    "empty slices, same category",
			current: Exercise{Category: CategoryUpper},
			other:   Exercise{Category: CategoryUpper},
			want:    3,
		},
		{
			name: "spec example: Bench Press vs Incline Press",
			current: Exercise{
				Category:              CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: Exercise{
				Category:              CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Shoulders"},
				SecondaryMuscleGroups: []string{"Triceps"},
			},
			want: 4 + 2 + 2 + 3, // 11
		},
		{
			name: "spec example: Bench Press vs Push-Ups",
			current: Exercise{
				Category:              CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
				SecondaryMuscleGroups: []string{"Shoulders"},
			},
			other: Exercise{
				Category:              CategoryUpper,
				PrimaryMuscleGroups:   []string{"Chest"},
				SecondaryMuscleGroups: []string{"Triceps", "Shoulders"},
			},
			want: 4 + 2 + 1 + 3, // 10
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SwapSimilarityScore(tt.current, tt.other); got != tt.want {
				t.Errorf("SwapSimilarityScore(current, other) = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSwapSimilarityScore_isSymmetric(t *testing.T) {
	a := Exercise{
		Category:              CategoryUpper,
		PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
		SecondaryMuscleGroups: []string{"Shoulders"},
	}
	b := Exercise{
		Category:              CategoryUpper,
		PrimaryMuscleGroups:   []string{"Chest", "Shoulders"},
		SecondaryMuscleGroups: []string{"Triceps"},
	}

	ab := SwapSimilarityScore(a, b)
	ba := SwapSimilarityScore(b, a)
	if ab != ba {
		t.Errorf("score asymmetric: SwapSimilarityScore(a,b) = %d, SwapSimilarityScore(b,a) = %d", ab, ba)
	}
}
```

- [ ] **Step 2: Run unit tests to verify they fail**

Run: `go test ./internal/workout/ -run TestSwapSimilarityScore -v`

Expected output (or similar): build failure or `undefined: SwapSimilarityScore`.

- [ ] **Step 3: Implement `SwapSimilarityScore`**

Create `internal/workout/swap.go`:

```go
package workout

// SwapSimilarityScore returns a non-negative integer describing how similar
// candidate is to current for the purposes of swapping one workout exercise
// for another. Higher means a better candidate.
//
// Weights:
//   - primary ∩ primary:     +4 per shared muscle
//   - primary ∩ secondary:   +2 per shared muscle (counted in both directions)
//   - secondary ∩ secondary: +1 per shared muscle
//   - same category:         +3 flat bonus
//
// The function is pure and symmetric: SwapSimilarityScore(a, b) ==
// SwapSimilarityScore(b, a).
func SwapSimilarityScore(current, candidate Exercise) int {
	score := 0
	score += 4 * countShared(current.PrimaryMuscleGroups, candidate.PrimaryMuscleGroups)
	score += 2 * countShared(current.PrimaryMuscleGroups, candidate.SecondaryMuscleGroups)
	score += 2 * countShared(current.SecondaryMuscleGroups, candidate.PrimaryMuscleGroups)
	score += 1 * countShared(current.SecondaryMuscleGroups, candidate.SecondaryMuscleGroups)
	if current.Category == candidate.Category {
		score += 3
	}
	return score
}

// countShared returns the number of strings appearing in both a and b.
// Inputs are treated as sets — duplicates within a single slice are not
// double-counted.
func countShared(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, m := range a {
		set[m] = struct{}{}
	}
	n := 0
	seen := make(map[string]struct{}, len(b))
	for _, m := range b {
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		if _, ok := set[m]; ok {
			n++
		}
	}
	return n
}
```

- [ ] **Step 4: Run unit tests to verify they pass**

Run: `go test ./internal/workout/ -run TestSwapSimilarityScore -v`

Expected: all subtests PASS, including `TestSwapSimilarityScore_isSymmetric`.

- [ ] **Step 5: Lint**

Run: `make lint-fix`

Expected: no errors. If lint flags a comment without a period or an unused parameter, fix inline before continuing.

- [ ] **Step 6: Commit**

```bash
git add internal/workout/swap.go internal/workout/swap_test.go
git commit -m "$(cat <<'EOF'
feat(workout): add SwapSimilarityScore for ranking swap candidates

Pure function that scores two exercises by muscle-group overlap (primary
weighted 4×, primary↔secondary 2×, secondary↔secondary 1×) plus a
+3 same-category bonus. Used by the swap exercise handler to order the
candidate list.
EOF
)"
```

---

## Task 2: Sort the swap candidate list in the handler

**Files:**
- Modify: `cmd/web/handler-workout.go` (imports + `workoutSwapExerciseGET`)
- Modify: `cmd/web/handler-exerciseset_test.go` (append new test)

- [ ] **Step 1: Write the failing handler test**

Append the following test to `cmd/web/handler-exerciseset_test.go`. It builds an in-memory map of `(exercise id → workout.Exercise)` by querying the test server's database directly via `server.DB()` (which already exists — see `internal/e2etest/server.go`). It then walks the rendered swap page, recovers each option's exercise ID from the hidden form input, computes the expected score order via `workout.SwapSimilarityScore`, and asserts the rendered order matches.

The test imports `"sort"` and `"strconv"`; check the existing import block at the top of `handler-exerciseset_test.go` and add whichever are missing. `workout`, `goquery`, `e2etest`, and `testhelpers` are already imported in that file.

```go
// Test_application_workoutSwapExercise_sorts_by_similarity verifies that the
// swap page renders compatible exercises in descending order of
// SwapSimilarityScore against the current slot's exercise, with alphabetical
// tie-breaks. Reads muscle-group data from the test DB so the assertion
// tracks fixture changes automatically.
func Test_application_workoutSwapExercise_sorts_by_similarity(t *testing.T) {
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

	var slotURL string
	doc.Find("a.exercise").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			if href, exists := s.Attr("href"); exists {
				slotURL = href
			}
		}
	})
	if slotURL == "" {
		t.Fatal("No exercise found on workout page")
	}

	if doc, err = client.GetDoc(ctx, slotURL+"/swap"); err != nil {
		t.Fatalf("Get swap page: %v", err)
	}

	// Recover the current exercise's name from the rendered Current Exercise
	// section so we can identify it in the DB load below.
	currentName := strings.TrimSpace(doc.Find(".current-exercise .name").Text())
	if currentName == "" {
		t.Fatal("Could not locate current exercise name on swap page")
	}

	// Load every exercise plus its muscle groups directly from the test
	// database. We build minimal workout.Exercise values populated with just
	// the fields SwapSimilarityScore reads (Category, PrimaryMuscleGroups,
	// SecondaryMuscleGroups, plus ID/Name for lookup and tie-breaks).
	db := server.DB()
	rows, err := db.QueryContext(ctx, `
		SELECT e.id, e.name, e.category, emg.muscle_group_name, emg.is_primary
		FROM exercises e
		LEFT JOIN exercise_muscle_groups emg ON emg.exercise_id = e.id
		ORDER BY e.id`)
	if err != nil {
		t.Fatalf("Query exercises: %v", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			t.Errorf("Close rows: %v", cerr)
		}
	}()
	byID := make(map[int]workout.Exercise)
	byName := make(map[string]workout.Exercise)
	for rows.Next() {
		var (
			id        int
			name      string
			category  string
			muscle    sql.NullString
			isPrimary sql.NullBool
		)
		if err = rows.Scan(&id, &name, &category, &muscle, &isPrimary); err != nil {
			t.Fatalf("Scan exercise row: %v", err)
		}
		ex, ok := byID[id]
		if !ok {
			ex = workout.Exercise{
				ID:       id,
				Name:     name,
				Category: workout.Category(category),
			}
		}
		if muscle.Valid {
			if isPrimary.Valid && isPrimary.Bool {
				ex.PrimaryMuscleGroups = append(ex.PrimaryMuscleGroups, muscle.String)
			} else {
				ex.SecondaryMuscleGroups = append(ex.SecondaryMuscleGroups, muscle.String)
			}
		}
		byID[id] = ex
		byName[name] = ex
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("Iterate exercise rows: %v", err)
	}
	current, ok := byName[currentName]
	if !ok {
		t.Fatalf("Current exercise %q not found in DB", currentName)
	}

	// Walk rendered options in DOM order, capturing (id, name).
	type rendered struct {
		id   int
		name string
	}
	var renderedOpts []rendered
	doc.Find(".exercise-option").Each(func(_ int, s *goquery.Selection) {
		idStr, _ := s.Find("input[name='new_exercise_id']").Attr("value")
		id, convErr := strconv.Atoi(idStr)
		if convErr != nil {
			return
		}
		name := strings.TrimSpace(s.Find(".exercise-name").Text())
		renderedOpts = append(renderedOpts, rendered{id: id, name: name})
	})
	if len(renderedOpts) < 2 {
		t.Fatalf("Need at least 2 rendered options to assert ordering, got %d", len(renderedOpts))
	}

	// Build expected order: same set of ids, sorted by score desc then name asc.
	expected := make([]rendered, len(renderedOpts))
	copy(expected, renderedOpts)
	sort.SliceStable(expected, func(i, j int) bool {
		si := workout.SwapSimilarityScore(current, byID[expected[i].id])
		sj := workout.SwapSimilarityScore(current, byID[expected[j].id])
		if si != sj {
			return si > sj
		}
		return expected[i].name < expected[j].name
	})

	for i := range renderedOpts {
		if renderedOpts[i].id != expected[i].id {
			gotNames := make([]string, len(renderedOpts))
			wantNames := make([]string, len(expected))
			for k := range renderedOpts {
				gotNames[k] = renderedOpts[k].name
				wantNames[k] = expected[k].name
			}
			t.Errorf("Rendered order does not match score-sorted order.\n got: %v\nwant: %v",
				gotNames, wantNames)
			return
		}
	}
}
```

Imports to verify/add at the top of `handler-exerciseset_test.go`: `"database/sql"` (for `sql.NullString`/`sql.NullBool`), `"sort"`, `"strconv"`. Run `goimports` (or rely on `make lint-fix`) to settle the import block.

No new method on `e2etest.Server` is needed — `server.DB() *sql.DB` already exists at `internal/e2etest/server.go:123`.

- [ ] **Step 2: Run handler test to verify it fails**

Run: `go test ./cmd/web/ -run Test_application_workoutSwapExercise_sorts_by_similarity -v`

Expected: FAIL with a message like `Rendered order does not match score-sorted order.` (because the handler isn't sorting yet, the rendered order is whatever the service returns and very unlikely to coincide with score-sorted order across all fixture exercises).

If the test fails for a *different* reason (compile error, missing accessor, missing import), fix that before continuing — the test must fail specifically on the assertion.

- [ ] **Step 3: Sort `compatibleExercises` in the handler**

In `cmd/web/handler-workout.go`:

1. Add `"sort"` to the import block at the top of the file (alphabetical order between `"net/http"` and `"strconv"`):

```go
import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/workout"
)
```

2. In `workoutSwapExerciseGET`, after the existing filter loop and before the `data := exerciseSwapTemplateData{...}` assignment (currently around line 215), insert the sort:

```go
sort.SliceStable(compatibleExercises, func(i, j int) bool {
	si := workout.SwapSimilarityScore(currentSlot.Exercise, compatibleExercises[i])
	sj := workout.SwapSimilarityScore(currentSlot.Exercise, compatibleExercises[j])
	if si != sj {
		return si > sj
	}
	return compatibleExercises[i].Name < compatibleExercises[j].Name
})
```

The full modified handler segment, for clarity:

```go
	queryLower := strings.ToLower(query)
	var compatibleExercises []workout.Exercise
	for _, exercise := range allExercises {
		if exercise.ID == currentSlot.Exercise.ID || existingExerciseIDs[exercise.ID] {
			continue
		}
		if queryLower != "" && !strings.Contains(strings.ToLower(exercise.Name), queryLower) {
			continue
		}
		compatibleExercises = append(compatibleExercises, exercise)
	}

	sort.SliceStable(compatibleExercises, func(i, j int) bool {
		si := workout.SwapSimilarityScore(currentSlot.Exercise, compatibleExercises[i])
		sj := workout.SwapSimilarityScore(currentSlot.Exercise, compatibleExercises[j])
		if si != sj {
			return si > sj
		}
		return compatibleExercises[i].Name < compatibleExercises[j].Name
	})

	data := exerciseSwapTemplateData{
		BaseTemplateData:    newBaseTemplateData(r),
		Date:                date,
		WorkoutExerciseID:   workoutExerciseID,
		CurrentExercise:     currentSlot.Exercise,
		CompatibleExercises: compatibleExercises,
		Query:               query,
	}
```

- [ ] **Step 4: Run the new handler test to verify it passes**

Run: `go test ./cmd/web/ -run Test_application_workoutSwapExercise_sorts_by_similarity -v`

Expected: PASS.

- [ ] **Step 5: Run pre-existing swap tests to confirm no regression**

Run: `go test ./cmd/web/ -run Test_application_workoutSwapExercise -v`

Expected: all swap tests pass — both the existing `Test_application_workoutSwapExercise_search_filters_by_name` (search filter must still work) and the existing swap-completion test that follows the workflow.

- [ ] **Step 6: Run the full test suite**

Run: `make test`

Expected: all packages pass. Pay attention to `cmd/web` and `internal/workout`. If anything else broke, investigate — sorting is the only behavior change, so unrelated failures are surprising.

- [ ] **Step 7: Lint**

Run: `make lint-fix`

Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add cmd/web/handler-workout.go cmd/web/handler-exerciseset_test.go
git commit -m "$(cat <<'EOF'
feat(swap): sort candidates by muscle-group similarity

Order the swap exercise list by SwapSimilarityScore (descending) with
alphabetical tie-break, so users see the closest variety match first
when they can't perform the planned exercise. Filter behavior and
search are unchanged.
EOF
)"
```

---

## Out of scope

This plan does not touch:

- The swap **POST** handler (`workoutSwapExercisePOST`) — exercise replacement and historical-set lookup are unchanged.
- The exercise-swap template (`ui/templates/pages/exercise-swap/exercise-swap.gohtml`) — the silent-sort UX means no badges, sections, highlighting, or layout changes.
- The `exerciseSwapTemplateData` struct — no new fields needed.
- Database schema or fixtures — `Exercise.PrimaryMuscleGroups` and `Exercise.SecondaryMuscleGroups` already carry everything the score needs.
- The week planner's muscle-group target logic in `internal/weekplanner/` — the swap score is independent of weekly volume planning.

If, while implementing, you discover that a same-category-bonus weight of `3` puts a different-category exercise with strong primary overlap below a same-category exercise with weak secondary overlap and that feels wrong, do **not** retune the weights inside this PR — file it as a follow-up. The constants live in one place and are designed to be tweaked once we've used the feature for a while.
