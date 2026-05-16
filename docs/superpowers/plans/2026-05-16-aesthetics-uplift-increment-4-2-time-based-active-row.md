# Aesthetics Uplift — Increment 4.2: Time-based Active-row Oversized Display — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the two defects on the Focus-mode oversized active row when the exercise is time-based: render a single centered column (instead of an off-centre cell in a 2-col grid) and label it "Time" (instead of the misleading "Reps"). The same single-column fix incidentally corrects active bodyweight rows, whose label was already correct.

**Architecture:** Single template file changes (`ui/templates/pages/exerciseset/sets-container.gohtml`): one markup-branch rewrite (rename `.reps` → `.time`, drop trailing `s` suffix, refine `aria-label`) plus four edits to the existing scoped `<style>` block (extend three label/typography selectors with `.time`, add a `.time::before` pseudo-element label, add a `:not(:has(.weight))` rule that drops the active grid to one column when no `.weight` cell renders). One new render test in `cmd/web/handler-exerciseset_test.go` mirroring the assisted-storage test pattern locks the layout in. **First `:has()` use in the codebase** — this is intentional and called out in the spec.

**Tech Stack:** Go `html/template` (`gohtml` scoped `@scope` `<style>` blocks), CSS (nesting, `@scope`, `:has()`, custom properties), Go (`cmd/web` handler tests with `goquery` + `e2etest`).

**Spec:** `docs/superpowers/specs/2026-05-15-aesthetics-uplift-increment-4-2-time-based-active-row-design.md`

---

## File Structure

### Modified files

| File | Change |
|---|---|
| `ui/templates/pages/exerciseset/sets-container.gohtml` | (a) Markup: in the time-based active branch (currently lines 477–478) rename `class="reps"` → `class="time"`, change `aria-label="Recommended target"` → `aria-label="Target time in seconds"`, drop the trailing ` s` text node so `.value` renders the bare integer. (b) CSS inside the existing `<style {{ nonce }}>` block, specifically inside the `&.active { &:not(.completed) .set-info { … } }` rule: add `&:not(:has(.weight)) { grid-template-columns: 1fr; }`; add `.time` to the `.weight, .reps { … }` label-block selector and to the `.weight .value, .reps .value { … }` oversized-numeral selector; add `.time::before { content: "Time"; }`. |
| `cmd/web/handler-exerciseset_test.go` | Add `Test_application_exerciseSet_time_based_active_oversized_layout` mirroring the `Test_application_exerciseSet_assisted_storage` setup (register, preferences, start workout, then `INSERT INTO workout_exercise` pointing at the seeded `Plank` exercise with `warmup_completed_at` and one placeholder `exercise_sets` row). Assertions: `.set-info .time` exists, `.value` text is `"30"` (no trailing `s`), `.weight` is absent inside the active `.set-info`. |

### Out of scope (untouched)

- `cmd/web/handler-exerciseset.go` — no new field, no data-shape change. Handler already exposes `$.ExerciseSet.Exercise.ExerciseType` and `$.CurrentSetTimedTarget`.
- `internal/domain/exercise.go` — `Exercise.IsTimed()`, `SetValueUnit()`, `FormatSetValue()` exist and stay as-is.
- `ui/templates/pages/exerciseset/exerciseset.gohtml`, `exercise-header.gohtml`, `warmup.gohtml` — untouched.
- `ui/static/main.css` — untouched.
- Completed-row and upcoming-row markup for time-based (the pre-existing `"30s seconds"` redundancy at lines 475 and 480) — pre-existing, separate from the active-row defect; spec lists this out of scope.
- The weighted/assisted active row, the time-based form, the bodyweight form, the warmup completion, the rest chip, the workout timer.

---

## Sequencing rationale

Three tasks, ordered to **prove the test catches the defect first**, then fix it, then gate.

- **Task 1 (test-first):** Add the failing render test for the time-based active oversized layout. Verify it fails *against current main* — column-1 layout via `.reps` not `.time`, with the trailing `s`. This proves the test exercises the right code path and would catch a regression.
- **Task 2 (fix):** Apply the markup + CSS changes to `sets-container.gohtml` in a single commit. The Task 1 test now passes; the existing nine exerciseset tests also still pass (verified by the full-suite gate at the end).
- **Task 3 (full-suite verification):** `make ci` + the cross-file palette grep gate + the full exerciseset test set. Same verification shape as Increments 1–4 / 4.1.

The choice to do **two commits** (test, then fix) is deliberate TDD: a reviewer browsing the increment can see the test by itself and convince themselves it expresses the right contract, before reading the diff that makes it pass.

---

## Task 1: Add the failing render test for time-based active oversized layout

**Files:**
- Modify: `cmd/web/handler-exerciseset_test.go` (append a new test function at end-of-file)

This test must FAIL on current `main` (proving the defect is real and the test catches it) and pass once Task 2 lands.

### Step 1: Append the new test function

- [ ] **Step 1.1: Open the test file and append the new function**

Append the following to `cmd/web/handler-exerciseset_test.go` (after `Test_computeSetActive` and its closing `}` at the bottom of the file). It uses the same imports already present at top of file (`database/sql`, `net/http`, `strconv`, `strings`, `testing`, `time`, `goquery`, `domain`, `e2etest`, `testhelpers`) — no new imports needed.

```go
// Test_application_exerciseSet_time_based_active_oversized_layout verifies that
// the active oversized treatment for a time-based exercise (Plank) renders as a
// single-column .time cell (not the two-column .weight + .reps grid used for
// weighted exercises) and that the column-header label reads "Time" via the
// .time::before pseudo-element. Locks Increment 4.2's :has(.weight)-driven
// single-column switch in against regression.
func Test_application_exerciseSet_time_based_active_oversized_layout(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Set preferences and start a workout for today.
	formData := map[string]string{time.Now().Weekday().String(): "60"}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("submit preferences: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if _, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("start workout: %v", err)
	}

	// Look up the seeded "Plank" id (set to ExerciseTypeTime by fixtures.sql)
	// and attach a workout_exercise slot for it on today's session with the
	// warmup already complete, so the active oversized row renders.
	db := server.DB()
	var plankID int
	if err = db.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Plank'`).Scan(&plankID); err != nil {
		t.Fatalf("get Plank id: %v", err)
	}

	var slotID int
	if err = db.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id,
            warmup_completed_at)
         SELECT user_id, workout_date, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ')
         FROM workout_sessions WHERE workout_date = ?
         RETURNING id`, plankID, today).Scan(&slotID); err != nil {
		t.Fatalf("insert plank slot: %v", err)
	}

	// Seed a single placeholder set (target_value = 30 seconds) so the form has
	// a row to render. weight_kg is unused for time-based but the column is NOT NULL.
	if _, err = db.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number,
            weight_kg, target_value)
         VALUES (?, 1, 0.0, 30)`, slotID); err != nil {
		t.Fatalf("insert plank set: %v", err)
	}

	slotPath := "/workouts/" + today + "/exercises/" + strconv.Itoa(slotID)
	if doc, err = client.GetDoc(ctx, slotPath); err != nil {
		t.Fatalf("get exercise set page: %v", err)
	}

	activeRow := doc.Find(".exercise-set.active .set-info").First()
	if activeRow.Length() == 0 {
		t.Fatalf("expected an active .set-info row on the Plank page")
	}

	timeCell := activeRow.Find(".time")
	if timeCell.Length() == 0 {
		t.Errorf("expected .time cell on active time-based row, got none")
	}
	if v := strings.TrimSpace(timeCell.Find(".value").Text()); v != "30" {
		t.Errorf("active .time .value = %q, want %q (no trailing 's' suffix)", v, "30")
	}

	// Single-column condition: no .weight cell renders alongside .time, which
	// is what triggers the :not(:has(.weight)) → 1-col layout switch.
	if activeRow.Find(".weight").Length() != 0 {
		t.Errorf("active time-based row should not render a .weight cell")
	}
}
```

### Step 2: Run the new test against current main — it MUST fail

- [ ] **Step 2.1: Run the new test**

Run:
```bash
go test ./cmd/web/ -run Test_application_exerciseSet_time_based_active_oversized_layout -v -count=1
```

Expected: **FAIL** with two error lines:
1. `active .time .value = "" "" want "30" (no trailing 's' suffix)` — because `.time` doesn't exist yet; `Find(".time")` returns empty, and `Find(".value").Text()` on an empty selection returns `""`.
2. (Possibly also) `expected .time cell on active time-based row, got none` — same root cause.

The `.weight` assertion (`activeRow.Find(".weight").Length() != 0`) is expected to **pass** even on current main, because the current time-based active branch doesn't render `.weight` either — it just renders `.reps`. That's fine; the assertion is forward-looking.

This proves: the test path correctly reaches the time-based active oversized branch (otherwise `activeRow` would be empty and we'd see `expected an active .set-info row`), and the contract the test expresses is not yet satisfied.

If `activeRow.Length() == 0` (the `t.Fatalf` fires), STOP — something about the setup (Plank seed, workout_exercise insert, warmup-complete timestamp) is wrong, and Task 2 won't fix it. Re-read `Test_application_exerciseSet_assisted_storage` lines 783–815 and copy the pattern more faithfully.

### Step 3: Commit the failing test

- [ ] **Step 3.1: Stage and commit**

```bash
git add cmd/web/handler-exerciseset_test.go
git commit -m "$(cat <<'EOF'
Add failing render test for time-based active oversized layout

Locks the contract for Increment 4.2's fix: the active oversized row on
a time-based exercise (Plank in the seed fixtures) must render a .time
cell with the bare integer seconds (no 's' suffix) and no .weight cell
alongside it. The latter is what makes the :not(:has(.weight)) CSS rule
in the next commit drop the grid to one column.

Fails on current main: the markup still emits .reps with a trailing "s",
not .time, so .Find(".time") returns empty.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Apply the markup + CSS fix to sets-container.gohtml

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`

One template edit, one CSS edit, both in the same file. After this commit Task 1's test passes; all existing exerciseset tests still pass (verified by Task 3).

### Step 1: Edit the time-based active markup branch

- [ ] **Step 1.1: Find the time-based active branch and apply the markup change**

In `ui/templates/pages/exerciseset/sets-container.gohtml`, find these two lines (around line 477–478 in the current `main` revision; the surrounding `{{ else if ... }}` is unique on the page so the edit is unambiguous):

```gohtml
                        {{ else if and (eq $.ExerciseSet.Exercise.ExerciseType "time_based") $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
                            <span class="reps" aria-label="Recommended target"><span class="value">{{ $.CurrentSetTimedTarget }}</span>s</span>
```

Replace with:

```gohtml
                        {{ else if and (eq $.ExerciseSet.Exercise.ExerciseType "time_based") $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
                            <span class="time" aria-label="Target time in seconds"><span class="value">{{ $.CurrentSetTimedTarget }}</span></span>
```

Three deltas on the inner `<span>`:
- `class="reps"` → `class="time"`.
- `aria-label="Recommended target"` → `aria-label="Target time in seconds"`.
- The trailing ` s` text node after the inner `</span>` is removed (the closing `</span>` of the outer span now follows the inner span immediately).

The `{{ else if }}` line and the line above/below it are unchanged. Do not touch the active bodyweight branch (the `{{ else }}` at line ≈479 that renders `<span class="reps">...`); its `class="reps"` is correct.

### Step 2: Edit the scoped CSS — four small additions

The four CSS edits all live inside the same nested rule:
`&.active { &:not(.completed) .set-info { … } }` (around lines 39–79 in the current `main` revision).

- [ ] **Step 2.1: Add the `:has()`-driven single-column rule**

Find this rule inside the scoped `<style>` block (it is the opening of `&:not(.completed) .set-info`):

```css
                        &:not(.completed) .set-info {
                            display: grid;
                            grid-template-columns: 1fr 1fr;
                            gap: var(--size-4);
                            align-items: center;
                            justify-items: center;
                            padding: var(--size-2) 0;

                            @media (max-width: 380px) {
                                grid-template-columns: 1fr;
                                gap: var(--size-3);
                            }
```

Insert the new `:not(:has(.weight))` rule between `padding: var(--size-2) 0;` and the `@media` block:

```css
                        &:not(.completed) .set-info {
                            display: grid;
                            grid-template-columns: 1fr 1fr;
                            gap: var(--size-4);
                            align-items: center;
                            justify-items: center;
                            padding: var(--size-2) 0;

                            &:not(:has(.weight)) {
                                grid-template-columns: 1fr;
                            }

                            @media (max-width: 380px) {
                                grid-template-columns: 1fr;
                                gap: var(--size-3);
                            }
```

This is the first `:has()` use in the codebase — the spec calls this out for reviewers. Browser support is universal (current Chrome/Edge/Safari/Firefox); no fallback needed.

- [ ] **Step 2.2: Extend the label-block selector to include `.time`**

In the same rule, find:

```css
                            .weight,
                            .reps {
                                display: flex;
                                flex-direction: column;
                                align-items: center;
                                gap: var(--size-1);
                                color: var(--stone-4);
                                font-size: var(--font-size-0);
                                font-weight: var(--font-weight-6);
                                text-transform: uppercase;
                                letter-spacing: var(--font-letterspacing-3);
                            }
```

Replace with (add `.time` as a third selector):

```css
                            .weight,
                            .reps,
                            .time {
                                display: flex;
                                flex-direction: column;
                                align-items: center;
                                gap: var(--size-1);
                                color: var(--stone-4);
                                font-size: var(--font-size-0);
                                font-weight: var(--font-weight-6);
                                text-transform: uppercase;
                                letter-spacing: var(--font-letterspacing-3);
                            }
```

- [ ] **Step 2.3: Add the `.time::before` pseudo-element label**

Immediately after the label-block selector above, find:

```css
                            .weight::before { content: "Weight"; }
                            .reps::before { content: "Reps"; }
```

Replace with (append a third line):

```css
                            .weight::before { content: "Weight"; }
                            .reps::before { content: "Reps"; }
                            .time::before { content: "Time"; }
```

- [ ] **Step 2.4: Extend the oversized-numeral selector to include `.time .value`**

Find:

```css
                            .weight .value,
                            .reps .value {
                                display: block;
                                font-size: var(--font-size-fluid-3);
                                font-family: var(--font-mono);
                                font-weight: var(--font-weight-7);
                                color: var(--stone-0);
                                line-height: 1;
                                letter-spacing: -0.02em;
                                text-transform: none;
                            }
```

Replace with (add `.time .value` as a third selector):

```css
                            .weight .value,
                            .reps .value,
                            .time .value {
                                display: block;
                                font-size: var(--font-size-fluid-3);
                                font-family: var(--font-mono);
                                font-weight: var(--font-weight-7);
                                color: var(--stone-0);
                                line-height: 1;
                                letter-spacing: -0.02em;
                                text-transform: none;
                            }
```

### Step 3: Verify the new test now passes

- [ ] **Step 3.1: Run the Task 1 test**

Run:
```bash
go test ./cmd/web/ -run Test_application_exerciseSet_time_based_active_oversized_layout -v -count=1
```

Expected: **PASS**. The `.time` cell renders, `.value` text equals `"30"`, and no `.weight` cell is present inside the active `.set-info`.

If it fails on the `.time .value` assertion with `""`, the markup change in Step 1 didn't apply correctly — re-read `sets-container.gohtml` at the time-based active branch and confirm the class is now `time`, not `reps`.

If it fails on the `.weight` assertion (`should not render a .weight cell`), the test selector is wrong — but this assertion should pass on both old and new code because the time-based active branch has never emitted `.weight`. If you see this, stop and investigate.

### Step 4: Run the broader exerciseset suite to confirm no regression

- [ ] **Step 4.1: Run the four sibling exerciseset tests**

Run:
```bash
go test ./cmd/web/ -run 'Test_application_exerciseSet|Test_application_exerciseSet_swap_preserves_url_and_drops_completed_sets|Test_application_exerciseSet_nonexistent_exercise_returns_custom_404|Test_application_exerciseSet_assisted_storage' -v -count=1
```

Expected: all four PASS. None of them assert on `.time` or on the time-based active branch text content, so the rename should be invisible to them. The shared `.weight, .reps, .time { … }` label-block rule still styles `.weight` and `.reps` exactly as before (additive change).

### Step 5: Commit

- [ ] **Step 5.1: Stage and commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "$(cat <<'EOF'
Fix time-based active oversized row: single column + Time label

The Focus-mode active oversized row on a time-based exercise rendered
as a single .reps cell auto-placed into column 1 of a 2-col grid (off
centre, column 2 empty), with the ::before pseudo-element header
reading "Reps" while the content was seconds.

Rename the time-based active span to .time, drop the trailing "s"
suffix (Increment 4.1 pattern — the column header is the unit), and
sharpen aria-label to "Target time in seconds". Inside the existing
&.active &:not(.completed) .set-info rule, add the .time class to the
label-block and oversized-numeral selectors, add a .time::before with
content:"Time", and add &:not(:has(.weight)) { grid-template-columns:
1fr; } so the active grid drops to one column whenever no .weight
cell renders. This covers both time-based (.time only) and active
bodyweight (.reps only) rows; weighted/assisted stays 2-col because
.weight is present.

First :has() use in the codebase — see the spec for context.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Full-suite verification

**Files:** none (verification only)

### Step 1: Cross-file palette grep gate

- [ ] **Step 1.1: Run the codebase-wide raw-token grep**

Run:
```bash
grep -rnE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)|var\(--black\)' ui/templates/ ui/static/main.css ; echo "exit:$?"
```

Expected: no matching lines and `exit:1` (grep returns 1 when nothing matches). This proves Increment 4.2 hasn't re-introduced any retired raw tokens.

### Step 2: Full exerciseset test set

- [ ] **Step 2.1: Run every exerciseset-touching test (the existing nine plus the new one)**

Run:
```bash
go test ./cmd/web/ -run 'ExerciseSet|exerciseSet|computeSetActive' -v -count=1
```

Expected: ten tests PASS:
- `Test_application_exerciseSet`
- `Test_application_exerciseSet_swap_preserves_url_and_drops_completed_sets`
- `Test_application_exerciseSet_nonexistent_exercise_returns_custom_404`
- `Test_application_workoutSwapExercise_search_filters_by_name`
- `Test_application_workoutSwapExercise_sorts_by_similarity`
- `Test_application_exerciseSet_assisted_storage`
- `Test_application_exerciseSet_time_based_active_oversized_layout` (new in this increment)
- `Test_ExerciseSet_RestChipAfterCompletedSet`
- `TestExerciseSetGET_DeloadHidesSignalButtons`
- `Test_computeSetActive` (with its six sub-tests)

### Step 3: Full `make ci`

- [ ] **Step 3.1: Run the project CI gate**

Run:
```bash
make ci
```

Expected: `init` + `build` + `lint-fix` (must report `0 issues.`) + `test` (every package OK) + `sec` (`No vulnerabilities found.`).

If `lint-fix` makes formatting-only changes (rare for `.gohtml` / `_test.go`), stage and commit them with `Apply lint-fix formatting` as the message. Do not amend the Task 2 commit.

### Step 4: Visual QA — note explicitly

- [ ] **Step 4.1: State the visual-QA caveat in your end-of-increment report**

Visual QA on `make dev` is NOT performed by this plan. The verification rests on the render test (`Test_application_exerciseSet_time_based_active_oversized_layout`), the cross-file grep gate, and the full `make ci`. Before merging, the developer running this plan should `make dev`, navigate to a Plank workout on a test account, complete the warmup, and confirm:
- The active oversized row renders as a single centred column.
- The column header reads **TIME** (uppercase via `text-transform: uppercase`).
- The oversized numeral is the bare integer (e.g. **30**), no trailing **s**.
- The signal form below (`No / Barely / Could do more`) is unchanged.

Do not claim the visual checks happened. If you don't run them, say so.

### Step 5: Final commit log

- [ ] **Step 5.1: Confirm the increment shape**

Run:
```bash
git log --oneline origin/main..HEAD
```

Expected: exactly two Increment 4.2 commits (plus an optional lint-fix commit), in this order:
1. `Add failing render test for time-based active oversized layout`
2. `Fix time-based active oversized row: single column + Time label`

---

## Self-review notes

- **Spec coverage:** every part of the spec maps to a task. Markup change (spec "Markup change") → Task 2 Step 1. CSS changes (spec "CSS changes" — four edits) → Task 2 Steps 2.1–2.4. New render test (spec "Test coverage") → Task 1. `:has()` first-use note (spec "Note on `:has()`") → mentioned in Task 2 Step 2.1 and commit message. Out-of-scope notes (completed/upcoming redundancy, min:sec format, handler/domain changes) → File Structure "Out of scope" + not addressed in tasks.
- **No placeholders:** every code block is concrete, every command is exact, no TBDs.
- **Type consistency:** the new class name is `time` everywhere — markup `class="time"` (Task 2 Step 1), CSS selectors `.time` / `.time .value` / `.time::before` (Steps 2.2–2.4), test assertion `activeRow.Find(".time")` (Task 1). The label text `"Time"` is consistent in `.time::before { content: "Time"; }` and in the Step 4 visual-QA "column header reads **TIME**" (uppercased visually via the inherited `text-transform: uppercase` from the label-block rule). The aria-label string `"Target time in seconds"` appears in Task 2 Step 1 only; the test does not assert on it.
- **No `:has()` second-site work:** the spec explicitly defers any "modifier class extraction" or "`:has()` recipe in CLAUDE.md" to a future increment if a second site appears. Don't preemptively refactor.
- **No handler change:** confirmed `$.ExerciseSet.Exercise.ExerciseType` and `$.CurrentSetTimedTarget` are already on `exerciseSetTemplateData` (see `cmd/web/handler-exerciseset.go` lines 27 and 35). Domain helpers `Exercise.IsTimed()`, `Exercise.SetValueUnit()`, `Exercise.FormatSetValue()` exist but are not consumed by this increment; they stay available for future cleanups.
- **Verification shape unchanged:** matches Increments 1–4 / 4.1 (cross-file grep + targeted test gate at task boundaries + full `make ci` at the end + explicit "no visual QA" disclaimer).
- **TDD ordering:** Task 1 commits a failing test against current `main` and Task 2 makes it pass. Two commits, not one — reviewer can read the test contract independently of the implementation diff.
