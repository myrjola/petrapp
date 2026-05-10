# Planning Data-Flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Funnel all `[]workout.Set` prescription construction through one helper (`buildPlannedSets`) so PlanWeek, AddExercise, and SwapExercise stop being three independently-derived paths; move rep-vs-second formatting onto `Exercise` so the handler has no input from which to reconstruct what the rule produced.

**Architecture:** Two pure additions land first (`Exercise.FormatSetValue` / `SetValueUnit` and `buildPlannedSets`), each with unit tests. Then four wiring changes consume them: replace `createDefaultSets` with the free function (Add/Swap path), collapse PlanWeek's inner loops via a lookup map, replace handler-side `formatTarget` with method calls, and delete the obsolete handler-side primitive and its test.

**Tech Stack:** Go (stdlib `testing`), no new dependencies, no schema changes, no template changes, no migrations.

**Spec:** `docs/superpowers/specs/2026-05-10-planning-data-flow-design.md`

---

## File Structure

| File | Change |
|---|---|
| `internal/workout/models.go` | Add `Exercise.FormatSetValue(value int) string` and `Exercise.SetValueUnit() string` methods. Add `strconv` import (`fmt` already present). |
| `internal/workout/models_test.go` | New file (`package workout_test`). Table-driven tests for the two new methods. |
| `internal/workout/planning.go` | New file. Holds the `buildPlannedSets(exercise Exercise, periodization PeriodizationType) []Set` free function. |
| `internal/workout/planning_internal_test.go` | New file (`package workout`). Table-driven tests for `buildPlannedSets`. |
| `internal/workout/service.go` | (a) `createDefaultSets` deletes; `buildSetsForAdd` calls `buildPlannedSets` directly. (b) `PlanWeek`'s outer loop builds an `exerciseByID` lookup map, then collapses the two inner loops into one `buildPlannedSets` call per exercise. |
| `cmd/web/handler-exerciseset.go` | Delete `formatTarget` (lines 17–25). Update `prepareSetsDisplay` to call `exercise.FormatSetValue` / `exercise.SetValueUnit`. |
| `cmd/web/handler-exerciseset_test.go` | Delete `Test_formatTarget` (the function it tests no longer exists). Existing rendered-HTML assertions stay unchanged because `FormatSetValue` produces the same strings. |

No SQL schema changes. No new dependencies. No template changes. The `internal/weekplanner` and `internal/exerciseprogression` packages are unaffected (they continue to do exactly what they do today).

---

## Task 1: Add `Exercise.FormatSetValue` and `Exercise.SetValueUnit`

Pure addition. No callers yet. Tests + methods land together; no other code depends on this task.

**Files:**
- Modify: `internal/workout/models.go` (append two methods, add `strconv` import)
- Create: `internal/workout/models_test.go`

- [ ] **Step 1: Confirm baseline build is green**

Run: `make test`
Expected: PASS — establishes a clean starting point so any test failure introduced in Step 2 is clearly attributable.

- [ ] **Step 2: Create the failing test file**

Create `internal/workout/models_test.go` with the following content. The tests reference methods that don't exist yet — the build will fail.

```go
package workout_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/workout"
)

func Test_Exercise_FormatSetValue(t *testing.T) {
	mkExercise := func(typ workout.ExerciseType) workout.Exercise {
		return workout.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise workout.Exercise
		value    int
		want     string
	}{
		{"weighted formats as integer", mkExercise(workout.ExerciseTypeWeighted), 8, "8"},
		{"bodyweight formats as integer", mkExercise(workout.ExerciseTypeBodyweight), 12, "12"},
		{"assisted formats as integer", mkExercise(workout.ExerciseTypeAssisted), 5, "5"},
		{"time_based formats as seconds", mkExercise(workout.ExerciseTypeTime), 30, "30s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.exercise.FormatSetValue(tc.value)
			if got != tc.want {
				t.Errorf("Exercise{%s}.FormatSetValue(%d) = %q, want %q",
					tc.exercise.ExerciseType, tc.value, got, tc.want)
			}
		})
	}
}

func Test_Exercise_SetValueUnit(t *testing.T) {
	mkExercise := func(typ workout.ExerciseType) workout.Exercise {
		return workout.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise workout.Exercise
		want     string
	}{
		{"weighted is reps", mkExercise(workout.ExerciseTypeWeighted), "reps"},
		{"bodyweight is reps", mkExercise(workout.ExerciseTypeBodyweight), "reps"},
		{"assisted is reps", mkExercise(workout.ExerciseTypeAssisted), "reps"},
		{"time_based is seconds", mkExercise(workout.ExerciseTypeTime), "seconds"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.exercise.SetValueUnit()
			if got != tc.want {
				t.Errorf("Exercise{%s}.SetValueUnit() = %q, want %q",
					tc.exercise.ExerciseType, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test -v ./internal/workout/ -run "Test_Exercise_FormatSetValue|Test_Exercise_SetValueUnit"`
Expected: FAIL with a build error like `ex.FormatSetValue undefined` and `ex.SetValueUnit undefined`. This confirms the tests exercise the post-fix surface.

- [ ] **Step 4: Add the `strconv` import to `models.go`**

In `internal/workout/models.go`, the current import block (lines 3–7) is:

```go
import (
	"encoding/json"
	"fmt"
	"time"
)
```

Replace with:

```go
import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)
```

- [ ] **Step 5: Append the two methods to `models.go`**

In `internal/workout/models.go`, the existing `IsTimed` method is at line 60:

```go
// IsTimed returns true if this exercise uses duration targets instead of rep counts.
func (e Exercise) IsTimed() bool { return e.ExerciseType == ExerciseTypeTime }
```

Insert the following two methods immediately after it (before the `// Resource represents...` comment):

```go
// FormatSetValue returns the user-visible string for a set's target or
// completed value. Reps render as "%d"; seconds render as "%ds". The unit
// choice is driven by ExerciseType — display layers must call this rather
// than reconstruct the formatting from periodization or any other field.
func (e Exercise) FormatSetValue(value int) string {
	if e.IsTimed() {
		return fmt.Sprintf("%ds", value)
	}
	return strconv.Itoa(value)
}

// SetValueUnit returns the input-label unit for a set value: "reps" or
// "seconds". Used by handlers when rendering input form labels.
func (e Exercise) SetValueUnit() string {
	if e.IsTimed() {
		return "seconds"
	}
	return "reps"
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test -v ./internal/workout/ -run "Test_Exercise_FormatSetValue|Test_Exercise_SetValueUnit"`
Expected: PASS — all 8 subtests succeed.

- [ ] **Step 7: Run the full workout-package tests**

Run: `go test ./internal/workout/...`
Expected: PASS — adding methods doesn't break existing tests.

- [ ] **Step 8: Commit**

```bash
git add internal/workout/models.go internal/workout/models_test.go
git commit -m "$(cat <<'EOF'
Add Exercise.FormatSetValue and Exercise.SetValueUnit

Domain methods that own rep-vs-second display formatting and the
"reps"/"seconds" unit label. Pure addition — no callers yet. Handlers
get switched to these methods in a later commit, replacing the
handler-side formatTarget primitive.

Spec: docs/superpowers/specs/2026-05-10-planning-data-flow-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `buildPlannedSets` (the funnel)

Pure addition. No callers yet. Tests + function land together; later tasks wire it into PlanWeek and Add/Swap.

**Files:**
- Create: `internal/workout/planning.go`
- Create: `internal/workout/planning_internal_test.go`

- [ ] **Step 1: Create the failing test file**

Create `internal/workout/planning_internal_test.go`:

```go
package workout

import (
	"testing"
)

func Test_buildPlannedSets(t *testing.T) {
	intPtr := func(i int) *int { return &i }

	cases := []struct {
		name           string
		exercise       Exercise
		periodization  PeriodizationType
		wantTargetVal  int
		wantSetCount   int
		wantWeightNil  bool // true means WeightKg should be nil; false means non-nil empty pointer
	}{
		{
			name: "weighted Strength: low end of window, 4 sets, weight pointer present",
			exercise: Exercise{ //nolint:exhaustruct // Only fields read by buildPlannedSets are set.
				ExerciseType: ExerciseTypeWeighted,
				RepMin:       intPtr(5),
				RepMax:       intPtr(10),
			},
			periodization: PeriodizationStrength,
			wantTargetVal: 5,
			wantSetCount:  4, // reps <= 5 → 4 sets
			wantWeightNil: false,
		},
		{
			name: "weighted Hypertrophy: high end, 3 sets",
			exercise: Exercise{ //nolint:exhaustruct
				ExerciseType: ExerciseTypeWeighted,
				RepMin:       intPtr(5),
				RepMax:       intPtr(10),
			},
			periodization: PeriodizationHypertrophy,
			wantTargetVal: 10,
			wantSetCount:  3, // 6-10 → 3 sets
			wantWeightNil: false,
		},
		{
			name: "weighted Hypertrophy: high-rep window, 3 sets",
			exercise: Exercise{ //nolint:exhaustruct
				ExerciseType: ExerciseTypeWeighted,
				RepMin:       intPtr(8),
				RepMax:       intPtr(12),
			},
			periodization: PeriodizationHypertrophy,
			wantTargetVal: 12,
			wantSetCount:  3, // >= 11 → 3 sets
			wantWeightNil: false,
		},
		{
			name: "assisted exercise: weight pointer present",
			exercise: Exercise{ //nolint:exhaustruct
				ExerciseType: ExerciseTypeAssisted,
				RepMin:       intPtr(5),
				RepMax:       intPtr(10),
			},
			periodization: PeriodizationStrength,
			wantTargetVal: 5,
			wantSetCount:  4,
			wantWeightNil: false,
		},
		{
			name: "bodyweight exercise: nil weight",
			exercise: Exercise{ //nolint:exhaustruct
				ExerciseType: ExerciseTypeBodyweight,
				RepMin:       intPtr(8),
				RepMax:       intPtr(12),
			},
			periodization: PeriodizationStrength,
			wantTargetVal: 8,
			wantSetCount:  3, // 6-10 → 3 sets
			wantWeightNil: true,
		},
		{
			name: "time_based exercise: nil weight, defaultTimedSets count",
			exercise: Exercise{ //nolint:exhaustruct
				ExerciseType:           ExerciseTypeTime,
				DefaultStartingSeconds: intPtr(45),
			},
			periodization: PeriodizationStrength,
			wantTargetVal: 45,
			wantSetCount:  defaultTimedSets,
			wantWeightNil: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPlannedSets(tc.exercise, tc.periodization)
			if len(got) != tc.wantSetCount {
				t.Fatalf("len = %d, want %d", len(got), tc.wantSetCount)
			}
			for i, s := range got {
				if s.TargetValue != tc.wantTargetVal {
					t.Errorf("set[%d].TargetValue = %d, want %d", i, s.TargetValue, tc.wantTargetVal)
				}
				if tc.wantWeightNil && s.WeightKg != nil {
					t.Errorf("set[%d].WeightKg = %v, want nil", i, *s.WeightKg)
				}
				if !tc.wantWeightNil && s.WeightKg == nil {
					t.Errorf("set[%d].WeightKg = nil, want non-nil pointer", i)
				}
				if s.CompletedValue != nil {
					t.Errorf("set[%d].CompletedValue = %v, want nil", i, *s.CompletedValue)
				}
				if s.CompletedAt != nil {
					t.Errorf("set[%d].CompletedAt = %v, want nil", i, *s.CompletedAt)
				}
				if s.Signal != nil {
					t.Errorf("set[%d].Signal = %v, want nil", i, *s.Signal)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./internal/workout/ -run Test_buildPlannedSets`
Expected: FAIL with `undefined: buildPlannedSets`. Confirms the test exercises a function that doesn't exist yet.

- [ ] **Step 3: Create `planning.go` with the funnel**

Create `internal/workout/planning.go`:

```go
package workout

// buildPlannedSets returns the persisted set slice for an exercise
// prescribed in a session of the given periodization. Single source of
// truth for "what sets does this exercise get when first added to a
// session" — used by PlanWeek, AddExercise, and SwapExercise.
//
// For weighted/assisted exercises with prior history, callers may
// post-process the returned slice to seed WeightKg from the latest
// completed set (see buildSetsForAdd). This function deliberately does
// not load history; it produces a clean prescription.
func buildPlannedSets(exercise Exercise, periodization PeriodizationType) []Set {
	targetValue, n := deriveSchemeForExercise(exercise, periodization)
	sets := make([]Set, n)
	for i := range sets {
		var weight *float64
		if !exercise.IsTimed() && exercise.ExerciseType != ExerciseTypeBodyweight {
			weight = new(float64)
		}
		sets[i] = Set{ //nolint:exhaustruct // CompletedValue, CompletedAt, Signal start nil.
			WeightKg:    weight,
			TargetValue: targetValue,
		}
	}
	return sets
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -v ./internal/workout/ -run Test_buildPlannedSets`
Expected: PASS — all 6 subtests succeed.

- [ ] **Step 5: Run the full workout-package tests**

Run: `go test ./internal/workout/...`
Expected: PASS — `buildPlannedSets` has no callers yet, so existing tests are unaffected.

- [ ] **Step 6: Commit**

```bash
git add internal/workout/planning.go internal/workout/planning_internal_test.go
git commit -m "$(cat <<'EOF'
Add buildPlannedSets funnel for prescription Set construction

New free function in the workout package that builds the []Set slice
for an exercise prescribed in a session of the given periodization.
Pure addition — no callers yet. Subsequent commits wire it in for the
AddExercise/SwapExercise path (replacing createDefaultSets) and for
PlanWeek (replacing the manual unwrap of weekplanner.PlannedSet).

Single source of truth for the question "what sets does this exercise
get when first added to a session?" so the three insert paths can no
longer drift independently the way PR #89 had to fix.

Spec: docs/superpowers/specs/2026-05-10-planning-data-flow-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Wire `buildPlannedSets` into the AddExercise/SwapExercise path

Replace `createDefaultSets` with a direct `buildPlannedSets` call from `buildSetsForAdd`. Existing PR #89 regression tests pin the behavior.

**Files:**
- Modify: `internal/workout/service.go` (delete `createDefaultSets` at lines 1083–1105, update `buildSetsForAdd` at line 1117)

- [ ] **Step 1: Verify the existing PR #89 regression tests are green**

Run: `go test -v ./internal/workout/ -run "Test_AddExercise_DerivesTargetValueFromPeriodization|Test_ReplaceExerciseInSession_DerivesTargetValueFromPeriodization"`
Expected: PASS — these tests pin the behavior we must preserve.

- [ ] **Step 2: Update `buildSetsForAdd` to call `buildPlannedSets` directly**

In `internal/workout/service.go`, the current `buildSetsForAdd` function (around line 1116) starts with:

```go
func (s *Service) buildSetsForAdd(ex Exercise, pt PeriodizationType, historicalSets []Set) []Set {
	sets := s.createDefaultSets(ex, pt)
	if len(historicalSets) == 0 {
		return sets
	}
```

Replace `s.createDefaultSets(ex, pt)` with `buildPlannedSets(ex, pt)`:

```go
func (s *Service) buildSetsForAdd(ex Exercise, pt PeriodizationType, historicalSets []Set) []Set {
	sets := buildPlannedSets(ex, pt)
	if len(historicalSets) == 0 {
		return sets
	}
```

The rest of `buildSetsForAdd` (the historical-weight seeding logic) is unchanged.

- [ ] **Step 3: Delete `createDefaultSets`**

In `internal/workout/service.go`, the current `createDefaultSets` function spans lines 1083–1105 (the doc-comment header at line 1083 begins with `// createDefaultSets returns N empty sets seeded with...`). Delete the entire function — its body is now subsumed by `buildPlannedSets`.

After deletion, `deriveSchemeForExercise` (the function immediately above it) becomes the sole remaining helper of its kind in `service.go`. It is still called — by `buildPlannedSets` in `planning.go`. Keep it where it is.

- [ ] **Step 4: Run the regression tests to verify behavior is preserved**

Run: `go test -v ./internal/workout/ -run "Test_AddExercise_DerivesTargetValueFromPeriodization|Test_ReplaceExerciseInSession_DerivesTargetValueFromPeriodization"`
Expected: PASS — `buildPlannedSets` produces the same output as the deleted `createDefaultSets` did.

- [ ] **Step 5: Run the full workout-package tests**

Run: `go test ./internal/workout/...`
Expected: PASS.

- [ ] **Step 6: Run lint**

Run: `make lint-fix`
Expected: clean. (Watch for "method `createDefaultSets` is unused" if any reference was missed; if so, find and update it.)

- [ ] **Step 7: Confirm `createDefaultSets` is gone**

Run: `grep -n "createDefaultSets" internal/workout/`
Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add internal/workout/service.go
git commit -m "$(cat <<'EOF'
Route AddExercise/SwapExercise through buildPlannedSets

Replaces createDefaultSets (which had a single caller) with a direct
call from buildSetsForAdd to the buildPlannedSets funnel introduced in
the previous commit. Behavior unchanged — buildPlannedSets is the
funnel form of createDefaultSets, with the same body now reused by
PlanWeek in the next commit.

PR #89 regression tests
(Test_AddExercise_DerivesTargetValueFromPeriodization,
Test_ReplaceExerciseInSession_DerivesTargetValueFromPeriodization)
continue to pass — they pin the exact target value and set count we
now produce via the shared funnel.

Spec: docs/superpowers/specs/2026-05-10-planning-data-flow-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire `buildPlannedSets` into the PlanWeek path

Build a lookup map once before the outer loop, collapse the two inner loops into one `buildPlannedSets` call. Existing PlanWeek tests pin the behavior.

**Files:**
- Modify: `internal/workout/service.go` (the `PlanWeek` body, lines ~190–220)

- [ ] **Step 1: Verify baseline PlanWeek tests are green**

Run: `go test -v ./internal/workout/ -run "PlanWeek|Plan_"`
Expected: PASS — these tests pin the PlanWeek behavior we must preserve.

(If grep shows no test names matching `PlanWeek|Plan_`, fall back to the broader run in Step 5 — there is no specific regression test we need to single out here.)

- [ ] **Step 2: Replace PlanWeek's inner per-exercise construction**

In `internal/workout/service.go`, locate the block currently spanning lines ~190–220 (immediately after `planner.Plan(monday)` returns). The current code is:

```go
sessionAggrs := make([]sessionAggregate, len(plannedSessions))
for i, ps := range plannedSessions {
	periodType := PeriodizationStrength
	if ps.PeriodizationType == weekplanner.PeriodizationHypertrophy {
		periodType = PeriodizationHypertrophy
	}

	exerciseSets := make([]exerciseSetAggregate, len(ps.ExerciseSets))
	for j, pes := range ps.ExerciseSets {
		sets := make([]Set, len(pes.Sets))
		for k, planSet := range pes.Sets {
			// RestSeconds is computed by DeriveScheme and carried in PlannedSet but not yet
			// persisted on Set — rest UI is out of scope for the rep-scheme spec.
			sets[k] = Set{ //nolint:exhaustruct // WeightKg, CompletedValue, CompletedAt, Signal start nil.
				TargetValue: planSet.TargetValue,
			}
		}
		exerciseSets[j] = exerciseSetAggregate{ //nolint:exhaustruct // ID is auto-assigned, WarmupCompletedAt starts nil.
			ExerciseID: pes.ExerciseID,
			Sets:       sets,
		}
	}

	sessionAggrs[i] = sessionAggregate{ //nolint:exhaustruct // DifficultyRating, StartedAt, CompletedAt start zero.
		Date:              ps.Date,
		PeriodizationType: periodType,
		ExerciseSets:      exerciseSets,
	}
}
```

Replace it with the following. Note: `exercises` is the `[]workout.Exercise` slice already in scope (it was used to build `wpExercises` ~30 lines earlier in the same function).

```go
exerciseByID := make(map[int]Exercise, len(exercises))
for _, ex := range exercises {
	exerciseByID[ex.ID] = ex
}

sessionAggrs := make([]sessionAggregate, len(plannedSessions))
for i, ps := range plannedSessions {
	periodType := PeriodizationStrength
	if ps.PeriodizationType == weekplanner.PeriodizationHypertrophy {
		periodType = PeriodizationHypertrophy
	}

	exerciseSets := make([]exerciseSetAggregate, len(ps.ExerciseSets))
	for j, pes := range ps.ExerciseSets {
		exerciseSets[j] = exerciseSetAggregate{ //nolint:exhaustruct // ID is auto-assigned, WarmupCompletedAt starts nil.
			ExerciseID: pes.ExerciseID,
			Sets:       buildPlannedSets(exerciseByID[pes.ExerciseID], periodType),
		}
	}

	sessionAggrs[i] = sessionAggregate{ //nolint:exhaustruct // DifficultyRating, StartedAt, CompletedAt start zero.
		Date:              ps.Date,
		PeriodizationType: periodType,
		ExerciseSets:      exerciseSets,
	}
}
```

The two inner loops collapse to one. The `RestSeconds` comment goes away with the deleted code.

- [ ] **Step 3: Run the full workout-package tests**

Run: `go test ./internal/workout/...`
Expected: PASS — `buildPlannedSets` re-derives the target value via `DeriveScheme`, which is the same pure function the planner used to produce `planSet.TargetValue`. Output is identical.

- [ ] **Step 4: Run the full test suite (handler + planner integration)**

Run: `make test`
Expected: PASS — handler tests and any integration tests that exercise the Plan→GetSession path see the same TargetValue as before.

If a test fails with a TargetValue mismatch, the most likely cause is a fixture exercise with `RepMin`/`RepMax` that produces a different `Scheme` than the planner happened to emit. Inspect the fixture and the failing assertion; the funnel's output is authoritative.

- [ ] **Step 5: Run lint**

Run: `make lint-fix`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/workout/service.go
git commit -m "$(cat <<'EOF'
Route PlanWeek through buildPlannedSets

PlanWeek's inner loop no longer unwraps weekplanner.PlannedSet to
build []Set rows. Instead, an exerciseByID lookup map is built once
and buildPlannedSets is called per exercise. The two inner loops
collapse into one.

Behavior is unchanged because buildPlannedSets re-derives via
exerciseprogression.DeriveScheme — the same pure function the planner
used to produce planSet.TargetValue. Both paths now derive at the
same point, so they cannot drift.

The weekplanner package continues to compute PlannedSet for its
internal muscle-volume calculations; service.go simply stops
consulting PlannedSet.TargetValue.

Spec: docs/superpowers/specs/2026-05-10-planning-data-flow-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Switch handler to `Exercise` methods, delete `formatTarget`

Replace handler-side `formatTarget` with `exercise.FormatSetValue`. Collapse the inline `IsTimed()` branches in `prepareSetsDisplay`. Delete the obsolete handler-side helper and its unit test.

**Files:**
- Modify: `cmd/web/handler-exerciseset.go` (delete `formatTarget` at lines 17–25, update `prepareSetsDisplay` at lines 49–74)
- Modify: `cmd/web/handler-exerciseset_test.go` (delete `Test_formatTarget` at lines ~904–953)

- [ ] **Step 1: Verify baseline tests pass**

Run: `make test`
Expected: PASS — establishes the green starting point. The HTML strings rendered today are what we'll keep producing after this change.

- [ ] **Step 2: Replace `prepareSetsDisplay` with the method-call version**

In `cmd/web/handler-exerciseset.go`, the current `prepareSetsDisplay` function (lines 49–74) is:

```go
func prepareSetsDisplay(exercise workout.Exercise, sets []workout.Set) []setDisplay {
	unit := "reps"
	if exercise.IsTimed() {
		unit = "seconds"
	}
	displays := make([]setDisplay, len(sets))
	for i, set := range sets {
		targetStr := formatTarget(exercise, set.TargetValue)
		completedStr := ""
		if set.CompletedValue != nil {
			if exercise.IsTimed() {
				completedStr = fmt.Sprintf("%ds", *set.CompletedValue)
			} else {
				completedStr = strconv.Itoa(*set.CompletedValue)
			}
		}
		displays[i] = setDisplay{
			Set:          set,
			TargetStr:    targetStr,
			CompletedStr: completedStr,
			Unit:         unit,
			Number:       i + 1,
		}
	}
	return displays
}
```

Replace with:

```go
func prepareSetsDisplay(exercise workout.Exercise, sets []workout.Set) []setDisplay {
	unit := exercise.SetValueUnit()
	displays := make([]setDisplay, len(sets))
	for i, set := range sets {
		completedStr := ""
		if set.CompletedValue != nil {
			completedStr = exercise.FormatSetValue(*set.CompletedValue)
		}
		displays[i] = setDisplay{
			Set:          set,
			TargetStr:    exercise.FormatSetValue(set.TargetValue),
			CompletedStr: completedStr,
			Unit:         unit,
			Number:       i + 1,
		}
	}
	return displays
}
```

Three `IsTimed()` branches collapse to three method calls. `fmt`/`strconv` remain used elsewhere in the file (form-parsing helpers below) — do not remove the imports.

- [ ] **Step 3: Delete `formatTarget` from `handler-exerciseset.go`**

In `cmd/web/handler-exerciseset.go`, the current `formatTarget` function (lines 17–25) is:

```go
// formatTarget returns the display string for a set target.
// For timed exercises it appends "s" (e.g. "30s").
// For rep-based exercises it returns the planner's target integer.
func formatTarget(exercise workout.Exercise, target int) string {
	if exercise.IsTimed() {
		return fmt.Sprintf("%ds", target)
	}
	return strconv.Itoa(target)
}
```

Delete the entire function including its doc comment.

- [ ] **Step 4: Delete `Test_formatTarget` from the handler test file**

In `cmd/web/handler-exerciseset_test.go`, the current `Test_formatTarget` function starts at line 904 with `func Test_formatTarget(t *testing.T) {` and runs to its closing `}` (around line 953). Delete the entire function. Its coverage now lives in `Test_Exercise_FormatSetValue` in the workout package (added in Task 1).

- [ ] **Step 5: Run the handler-package tests**

Run: `go test ./cmd/web/...`
Expected: PASS — rendered HTML strings are unchanged because `FormatSetValue` produces the same output as the deleted `formatTarget` did.

If a handler test fails with an unexpected target string (e.g. integer where the test asserted on something else), the most likely cause is a fixture mismatch from earlier work — inspect the assertion and reconcile with the planner's actual output.

- [ ] **Step 6: Run the full test suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 7: Run lint**

Run: `make lint-fix`
Expected: clean. If lint complains about unused `fmt` or `strconv` imports in `handler-exerciseset.go`, double-check — they should remain used by the form-parsing helpers further down the file.

- [ ] **Step 8: Confirm no remaining references to `formatTarget`**

Run: `grep -rn "formatTarget" cmd/web/`
Expected: no output.

- [ ] **Step 9: Commit**

```bash
git add cmd/web/handler-exerciseset.go cmd/web/handler-exerciseset_test.go
git commit -m "$(cat <<'EOF'
Switch handler to Exercise.FormatSetValue / SetValueUnit

prepareSetsDisplay now calls exercise.FormatSetValue (for both target
and completed values) and exercise.SetValueUnit (for the input
label). Three inline IsTimed() branches collapse to method calls on
the domain type that owns the rule.

formatTarget in cmd/web/handler-exerciseset.go is deleted entirely;
its coverage moved to Test_Exercise_FormatSetValue in the workout
package. The handler now has no input from which to reconstruct what
the planner produced — every displayed value reads from a stored
field on Set and is formatted by the domain.

Spec: docs/superpowers/specs/2026-05-10-planning-data-flow-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Final verification

Run the full CI suite, hit the acceptance grep checks from the spec, and smoke-test one workout flow in the browser.

- [ ] **Step 1: Run the full CI suite**

Run: `make ci`
Expected: PASS across init, build, lint, test, sec.

- [ ] **Step 2: Acceptance grep — `formatTarget` is gone from `cmd/web/`**

Run: `grep -rn "formatTarget" cmd/web/`
Expected: no output.

- [ ] **Step 3: Acceptance grep — `DeriveScheme` callers in the workout package**

Run: `grep -rn "DeriveScheme" internal/workout/`
Expected: a single caller — `internal/workout/service.go` inside `deriveSchemeForExercise`. (The `planning.go` funnel does not call `DeriveScheme` directly; it calls `deriveSchemeForExercise`.) No call from `PlanWeek`, `AddExercise`, `SwapExercise`, or anywhere else.

- [ ] **Step 4: Inspect the commit log**

Run: `git log --oneline main..HEAD` (or `git log --oneline -10` if you're already on main)
Expected: five commits, in this order:
1. Add Exercise.FormatSetValue and Exercise.SetValueUnit
2. Add buildPlannedSets funnel for prescription Set construction
3. Route AddExercise/SwapExercise through buildPlannedSets
4. Route PlanWeek through buildPlannedSets
5. Switch handler to Exercise.FormatSetValue / SetValueUnit

- [ ] **Step 5: Smoke-test one workout flow in the browser**

Boot the dev server: `make dev`. Open a workout day in the browser. Verify, for each exercise type that's present:

- **Weighted exercise (e.g. bench press)**: per-set target shows the planner's integer (e.g. `8`); unit label is `reps`; completed string after recording shows the integer.
- **Bodyweight exercise (e.g. pull-up)**: per-set target shows an integer; unit label is `reps`.
- **Time-based exercise (e.g. plank)**: per-set target shows `Ns` (e.g. `45s`); unit label is `seconds`; completed string after recording shows `Ns`.
- **Assisted exercise** (if present in your fixtures): per-set target shows an integer; unit label is `reps`; the assisted weight input still works correctly (this path goes through `recordSetCompletionWithWeight`, untouched by this change).

Stop the dev server with Ctrl-C when done. If any flow renders a target that doesn't match the planner's prescription, check the corresponding `exerciseByID[pes.ExerciseID]` lookup in PlanWeek — a missing or mismatched exercise would cause the funnel to receive a zero-value `Exercise` and produce defensive defaults.
