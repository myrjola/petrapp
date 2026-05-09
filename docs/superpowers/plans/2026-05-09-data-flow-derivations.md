# Data-flow Derivations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the hardcoded `"6-10"` rep-range display in `formatTarget`, codify the rule that display values depending on multiple domain attributes must come from the domain type rather than be reconstructed in handlers, and pin the new behavior with a small regression test.

**Architecture:** Two commits. First commit: pin the new `formatTarget` behavior with a focused unit test, collapse the function to its `IsTimed`-only branch, drop the now-unused `session` parameter from `formatTarget` and `prepareSetsDisplay`, update the single call site. Second commit: append the rule to both relevant `CLAUDE.md` files.

**Tech Stack:** Go (stdlib `testing`), no new dependencies, no schema changes, no template changes.

**Spec:** `docs/superpowers/specs/2026-05-09-data-flow-derivations-design.md`

---

## File Structure

| File | Change |
|---|---|
| `cmd/web/handler-exerciseset.go` | Collapse `formatTarget` to a two-branch function (`IsTimed` → `"Ns"`, else → integer). Drop the `session workout.Session` parameter from both `formatTarget` and `prepareSetsDisplay`. Update the single `prepareSetsDisplay` call site at the bottom of `exerciseSetGET`. |
| `cmd/web/handler-exerciseset_test.go` | Append a small table-test for `formatTarget` pinning: time-based returns `"Ns"`, non-timed returns the integer regardless of any prior periodization branching. |
| `internal/workout/CLAUDE.md` | Append a subsection "Display derivations belong on domain types" at the end of "Common Patterns and Anti-Patterns". |
| `cmd/web/CLAUDE.md` | Append a "Don't recompute domain rules" bullet to "Prefer Handler-Side Processing". |

No SQL schema changes. No new dependencies. No template changes. The `internal/weekplanner` and `internal/exerciseprogression` packages are unaffected (they continue to consume `Session.PeriodizationType` and `Exercise.RepMin/RepMax` directly).

---

## Task 1: Pin behavior, collapse the hypertrophy branch

**Files:**
- Modify: `cmd/web/handler-exerciseset.go:17-31` (the `formatTarget` function)
- Modify: `cmd/web/handler-exerciseset.go:55-80` (the `prepareSetsDisplay` function — drop `session` parameter)
- Modify: `cmd/web/handler-exerciseset.go:168` (the single `prepareSetsDisplay` call site)
- Modify: `cmd/web/handler-exerciseset.go:33-39` (the `setDisplay` doc comment that mentions `"6-10"`)
- Test: `cmd/web/handler-exerciseset_test.go` (append at the end of the file)

- [ ] **Step 1: Confirm the baseline build is green**

Run: `make test`
Expected: PASS — establishes a clean starting point so any test failures introduced in step 2 are clearly attributable to this change.

- [ ] **Step 2: Append the failing unit test**

Append the following to the end of `cmd/web/handler-exerciseset_test.go`. The test exercises the new signature (`formatTarget(exercise, target)`) so it will fail to compile against the current 3-argument function — that compile failure is the "red" for this TDD cycle.

```go
func Test_formatTarget(t *testing.T) {
	mkExercise := func(typ workout.ExerciseType) workout.Exercise {
		// Only ExerciseType matters for formatTarget; everything else can stay
		// zero. nolint suppression matches the rest of this file.
		return workout.Exercise{ //nolint:exhaustruct // Only ExerciseType is read.
			ExerciseType: typ,
		}
	}

	cases := []struct {
		name     string
		exercise workout.Exercise
		target   int
		want     string
	}{
		{
			name:     "time_based formats as seconds",
			exercise: mkExercise(workout.ExerciseTypeTime),
			target:   30,
			want:     "30s",
		},
		{
			name:     "weighted formats as integer",
			exercise: mkExercise(workout.ExerciseTypeWeighted),
			target:   8,
			want:     "8",
		},
		{
			name:     "bodyweight formats as integer",
			exercise: mkExercise(workout.ExerciseTypeBodyweight),
			target:   12,
			want:     "12",
		},
		{
			name:     "assisted formats as integer",
			exercise: mkExercise(workout.ExerciseTypeAssisted),
			target:   5,
			want:     "5",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatTarget(tc.exercise, tc.target)
			if got != tc.want {
				t.Errorf("formatTarget(%v, %d) = %q, want %q",
					tc.exercise.ExerciseType, tc.target, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run the new test to verify it fails**

Run: `go test -v ./cmd/web/ -run Test_formatTarget`
Expected: FAIL — compile error along the lines of `not enough arguments in call to formatTarget` because `formatTarget` currently takes three parameters (`exercise`, `session`, `target`). This confirms the test is exercising the post-fix signature.

- [ ] **Step 4: Collapse `formatTarget` and drop its `session` parameter**

In `cmd/web/handler-exerciseset.go`, replace the existing `formatTarget` function (lines 17-31) with the following:

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

The hypertrophy branch, the misleading comment about "preserves the legacy 6-10 rep range UX", and the `session` parameter all delete in this step.

- [ ] **Step 5: Update the `setDisplay` doc comment**

In `cmd/web/handler-exerciseset.go`, update the `TargetStr` field doc on `setDisplay` (currently around line 35) so it no longer references `"6-10"`:

```go
type setDisplay struct {
	Set          workout.Set
	TargetStr    string // Pre-formatted target string (e.g. "5", "30s").
	CompletedStr string // Pre-formatted completed string, same unit as TargetStr.
	Unit         string // "reps" or "seconds" — for input labels.
	Number       int    // 1-based set number for display.
}
```

- [ ] **Step 6: Drop the `session` parameter from `prepareSetsDisplay`**

In `cmd/web/handler-exerciseset.go`, replace the existing `prepareSetsDisplay` function (lines 55-80) with the following. The body keeps the same per-set formatting; only the parameter list and the inner call to `formatTarget` change.

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

- [ ] **Step 7: Update the single `prepareSetsDisplay` call site**

In `cmd/web/handler-exerciseset.go` at line 168 (inside `exerciseSetGET`), replace the call:

```go
SetsDisplay:           prepareSetsDisplay(exerciseSet.Exercise, session, exerciseSet.Sets),
```

with:

```go
SetsDisplay:           prepareSetsDisplay(exerciseSet.Exercise, exerciseSet.Sets),
```

If `grep` shows any other `prepareSetsDisplay(` call sites you missed, update them the same way (drop the second argument). Confirm with:

Run: `grep -n "prepareSetsDisplay(" cmd/web/`
Expected: a single `prepareSetsDisplay(exerciseSet.Exercise, exerciseSet.Sets)` call at the previous line 168 plus the function definition itself.

- [ ] **Step 8: Run the new test to verify it passes**

Run: `go test -v ./cmd/web/ -run Test_formatTarget`
Expected: PASS — all four subtests succeed.

- [ ] **Step 9: Run the full test suite**

Run: `make test`
Expected: PASS. If any pre-existing test fails, it's likely a fixture that asserted on the legacy `"6-10"` string in the rendered HTML; in that case, update the assertion to expect the integer target value (e.g. the planner's emitted single integer) and re-run.

- [ ] **Step 10: Run lint**

Run: `make lint-fix`
Expected: clean. The `nolint:exhaustruct` directive on the test's `mkExercise` helper matches the convention used elsewhere in this test file when only a subset of `Exercise`'s fields are relevant.

- [ ] **Step 11: Commit**

```bash
git add cmd/web/handler-exerciseset.go cmd/web/handler-exerciseset_test.go
git commit -m "$(cat <<'EOF'
Collapse formatTarget hypertrophy branch; pin new display contract

Removes the hardcoded "6-10" rep-range display for hypertrophy sessions.
The display now always shows the planner's target integer (or "Ns" for
time-based exercises). Drops the now-unused session parameter from
formatTarget and prepareSetsDisplay.

The previous branch reimplemented a rep-window rule that lived correctly
in the planner via Exercise.RepMin/RepMax + DeriveScheme, and silently
diverged once per-exercise windows landed (PR #89). RIR is the chosen
auto-regulation signal; surfacing a window string at set-display time is
no longer needed.

Adds a small table-test pinning formatTarget's contract so the bug class
cannot regress.

Spec: docs/superpowers/specs/2026-05-09-data-flow-derivations-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Document the rule in CLAUDE.md

**Files:**
- Modify: `internal/workout/CLAUDE.md` (insert after line 242 — the end of the existing `### ❌ Anti-Patterns` block, before `## Error Handling Strategies`)
- Modify: `cmd/web/CLAUDE.md` (extend the bullet list under `### Prefer Handler-Side Processing` at lines 47-50)

- [ ] **Step 1: Append the "Display derivations" subsection to `internal/workout/CLAUDE.md`**

In `internal/workout/CLAUDE.md`, insert the following block immediately after the existing `### ❌ Anti-Patterns` list (after line 242, before the `## Error Handling Strategies` heading on line 244). This adds a new H3 subsection within the existing "Common Patterns and Anti-Patterns" H2.

```markdown
### Display derivations belong on domain types

Any value that depends on multiple domain attributes, or that encodes a business rule, lives as a method on the domain type that owns the rule (`Exercise.IsTimed()` is the canonical example). Handlers may format primitives (`%d`, `%.1fkg`, `time.Format`) and shape data into per-page template structs, but they may not branch on multiple domain fields to compute a value.

**Test:** if changing the rule would force edits in two or more files outside `internal/workout/`, it is a domain method. The 2026-05-09 hypertrophy-window incident traces back to `formatTarget` in `cmd/web/` reimplementing a rule that already lived (correctly) in the planner.
```

Use the `Edit` tool. The unique anchor for the edit is the closing line of the anti-patterns list immediately followed by the next H2:

- old_string:
  ```
  - Don't mix domain and repository aggregate types

  ## Error Handling Strategies
  ```

- new_string:
  ```
  - Don't mix domain and repository aggregate types

  ### Display derivations belong on domain types

  Any value that depends on multiple domain attributes, or that encodes a business rule, lives as a method on the domain type that owns the rule (`Exercise.IsTimed()` is the canonical example). Handlers may format primitives (`%d`, `%.1fkg`, `time.Format`) and shape data into per-page template structs, but they may not branch on multiple domain fields to compute a value.

  **Test:** if changing the rule would force edits in two or more files outside `internal/workout/`, it is a domain method. The 2026-05-09 hypertrophy-window incident traces back to `formatTarget` in `cmd/web/` reimplementing a rule that already lived (correctly) in the planner.

  ## Error Handling Strategies
  ```

- [ ] **Step 2: Append the "Don't recompute domain rules" bullet to `cmd/web/CLAUDE.md`**

In `cmd/web/CLAUDE.md`, extend the bullet list under `### Prefer Handler-Side Processing` (currently lines 47-50). The existing list ends with "Create maps for lookups to avoid complex template logic". Append one new bullet after it.

Use the `Edit` tool:

- old_string:
  ```
  - Filter collections in handlers (e.g., remove already-selected exercises)
  - Transform enums to display-friendly structures with labels
  - Compute derived values and format data before template rendering
  - Create maps for lookups to avoid complex template logic
  ```

- new_string:
  ```
  - Filter collections in handlers (e.g., remove already-selected exercises)
  - Transform enums to display-friendly structures with labels
  - Compute derived values and format data before template rendering
  - Create maps for lookups to avoid complex template logic
  - **Don't recompute domain rules.** Handlers may format primitives and shape data, but any value that depends on multiple domain fields must come from a method on the domain type. If you find yourself writing `if exercise.X && session.Y { ... }` in a handler, move it to `internal/workout/`.
  ```

- [ ] **Step 3: Verify both files render as intended**

Run: `grep -n "Display derivations belong" internal/workout/CLAUDE.md`
Expected: a single line of output showing the new H3 heading.

Run: `grep -n "Don't recompute domain rules" cmd/web/CLAUDE.md`
Expected: a single bullet line.

- [ ] **Step 4: Commit the doc updates**

```bash
git add internal/workout/CLAUDE.md cmd/web/CLAUDE.md
git commit -m "$(cat <<'EOF'
Document display-derivations rule in CLAUDE.md

Adds a new H3 subsection in internal/workout/CLAUDE.md and a bullet in
cmd/web/CLAUDE.md codifying the rule that values depending on multiple
domain attributes must live as methods on the domain type, not be
reconstructed in handlers. Captures the lesson from the 2026-05-09
hypertrophy-window incident.

Spec: docs/superpowers/specs/2026-05-09-data-flow-derivations-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Final verification

- [ ] **Step 1: Run the full CI suite**

Run: `make ci`
Expected: PASS across init, build, lint, test, sec.

- [ ] **Step 2: Inspect the diff log**

Run: `git log --oneline main..HEAD`
Expected: two commits — the code/test commit from Task 1, then the docs commit from Task 2.

- [ ] **Step 3: Confirm no remaining `"6-10"` display string anywhere**

Run: `grep -rn '"6-10"' cmd/web/ ui/ internal/workout/ 2>/dev/null`
Expected: no output. (The `internal/weekplanner` enum-doc comment and the `internal/exerciseprogression/scheme.go` comment that mention `6-10` are about set/rest mapping calibration, not user-facing display, and are intentionally left alone — they describe planner behavior.)

- [ ] **Step 4: Smoke-test one workout flow manually**

Boot the dev server: `make dev`. Open a workout with a hypertrophy session in the browser. Confirm the per-set target now shows the planner's integer (e.g. `8`) instead of the legacy `6-10`. Confirm a time-based exercise still shows `Ns`. Stop the dev server with Ctrl-C when done.
