# Aesthetics Uplift — Increment 4.1: Focus-mode Layout Polish — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tighten the Increment 4 Focus-mode layout in response to live-device feedback: give the exercise title its own row at all widths, relocate the workout-elapsed timer from a fixed-position viewport overlay into the header's action cluster (eliminating the title-overlap), put the Weight + Actual Reps inputs side-by-side on the active panel, and drop the redundant `reps` suffix from the active oversized `.reps` display where the column header already says REPS.

**Architecture:** Pure scoped-CSS + minimal-markup-restructure increment. Two templates change: `ui/templates/pages/exerciseset/exercise-header.gohtml` (header layout + timer integration) and `ui/templates/pages/exerciseset/sets-container.gohtml` (form-row pair + suffix drop). One template loses one line: `ui/templates/pages/exerciseset/exerciseset.gohtml` (the `<div class="timer">` moves out into the header) plus the related `.timer` scoped style block. The handler and tests are not touched. The view-transition wiring on `.exercise-title` is preserved. Every DOM selector the test suite reads (`.exercise-set`, `.exercise-set.completed`, `.weight`, `.reps`, `button[name='signal']`, `input[name='weight']`, `input[name='reps']`, `button:contains('Done!')`, `button:contains('Mark Warmup Complete')`, `#workout-timer` if anywhere — verified not used by tests) survives.

**Tech Stack:** CSS (custom properties, `@layer`, `@scope`, CSS nesting), Go `html/template`, Go (`cmd/web` handler + `goquery`-based e2e tests — unchanged).

**Spec:** the Focus-mode section of `docs/superpowers/specs/2026-05-14-aesthetics-uplift-design.md`. The spec defined the dark `--stone-9` panel, oversized numerals, Ember CTA and "restyled timer" target; this increment refines the layout the previous increment shipped after live-device review identified the title-clip and the vertical-stacked-inputs as friction points.

---

## File Structure

### Modified files

| File | Change |
|---|---|
| `ui/templates/pages/exerciseset/exercise-header.gohtml` | Restructure the header to a two-row layout at all widths — Row 1: Back link · workout timer · Info/Swap actions; Row 2: full-width h1 title. The `<div id="workout-timer">` markup and its scoped `.timer` rule relocate from `exerciseset.gohtml` into this file, re-styled as an inline pill (no `position: fixed`). |
| `ui/templates/pages/exerciseset/exerciseset.gohtml` | Remove the `<div class="timer" id="workout-timer">` markup and the scoped `.timer` rule — both move into `exercise-header.gohtml`. The `initializeTimer()` JS script stays; it targets `getElementById('workout-timer')` and the DOM id is unchanged. |
| `ui/templates/pages/exerciseset/sets-container.gohtml` | Wrap the active weighted/assisted Weight `.input-field` + Actual Reps `.input-field` in a `<div class="set-form-row">` so they render side-by-side; the assisted-field stays as a full-width row positioned **between** the input-pair and the signal-buttons (semantic: enter values, confirm the weight-meaning, then signal). Drop the trailing ` reps` suffix on the active row's `.reps` span in the "Recommended weight" branch (the column header already says REPS; keep ` kg` on `.weight` because label and unit differ). Add the scoped CSS for `.set-form-row`. |

### Out of scope (untouched)

- `ui/templates/pages/exerciseset/warmup.gohtml` — fully polished in Increment 4 Task 1; no holdouts.
- All other `pages/**` templates — Increment 5/6 territory.
- `ui/static/main.css`, shared components — already on the Stone system.
- Go handler (`cmd/web/handler-exerciseset.go`) and its tests — no Go change. The handler still exposes `setDisplay.IsActive`; the template still consumes it.
- The view-transition wiring (`view-transition-name: exercise-title-<id>` on `.exercise-title`) — preserved across the header restructure.
- The two oversized-display columns themselves (the dark `--stone-9` panel surface, `--font-size-fluid-3` mono digits, pseudo-element labels) — Increment 4 work, unchanged.
- Time-based exercises' oversized display — currently renders only a single `.reps` column in the 2-column grid (the other column sits empty), and the column header reads `Reps` even though the content is seconds. Both are real defects, but the user did not call them out and they are out of scope for this increment. Flag for a future Increment 4.2 if needed.

---

## Sequencing rationale

Four tasks. Tasks 1 and 2 are markup-and-CSS structural changes (header restructure + timer relocation; form-row pair + assisted-row repositioning); Task 3 is a one-line text drop with no CSS impact; Task 4 is the full-suite gate. Tasks are independent — Task 1 touches the header file pair, Task 2 touches the sets-container, Task 3 touches one branch in the sets-container. They could be executed in any order; the chosen order (header → form-row → suffix drop → verify) lets each task be reviewed against a stable predecessor.

**Verification shape:** unchanged from Increments 1–4: per-task render test (`go test ./cmd/web/ -run Test_application_exerciseSet -v -count=1`) plus full `make ci` at the end. CSS layout changes have no compiler; selector preservation is verified by the existing handler test suite passing without test edits. Visual QA is described but not browser-driven — the user can verify on `make dev` after merge.

---

## Task 1: Restructure exercise-header — title on its own row + integrate workout timer

**Files:**
- Modify: `ui/templates/pages/exerciseset/exercise-header.gohtml`
- Modify: `ui/templates/pages/exerciseset/exerciseset.gohtml`

The current header is a single-row 3-column grid (`auto 1fr auto`) on desktop, with the action cluster wrapping to a second row on `<480px`. The h1 title shares its row with the Back link, and the `<div class="timer">` floats `position: fixed` at the viewport's top-right — overlapping the title text on mobile.

After Task 1:
- Row 1: `← Back` (left) · workout-timer pill · Info/Swap actions (right) — all at all widths.
- Row 2: full-width centered `<h1>` exercise title.

The timer moves out of fixed-position. It becomes an inline child of `.header-actions` (or sibling, depending on a11y reading-order preference — see below). The `id="workout-timer"` stays, so `initializeTimer()` keeps working unchanged.

### Step 1: Add the timer markup to the header and remove it from the page shell

In `ui/templates/pages/exerciseset/exercise-header.gohtml`, find:

```gohtml
        {{ template "back-link" (printf "/workouts/%s" (.Date.Format "2006-01-02")) }}
        <h1 class="exercise-title"
            data-workout-exercise-id="{{ .ExerciseSet.ID }}">{{ .ExerciseSet.Exercise.Name }}</h1>
        <div class="header-actions cluster">
            <a href="/workouts/{{ .Date.Format "2006-01-02" }}/exercises/{{ .ExerciseSet.ID }}/info"
               class="action-button info-button" aria-label="View exercise information">Info</a>
            <a href="/workouts/{{ .Date.Format "2006-01-02" }}/exercises/{{ .ExerciseSet.ID }}/swap"
               class="action-button swap-button" aria-label="Swap exercise">Swap</a>
        </div>
    </header>
```

Replace with:

```gohtml
        {{ template "back-link" (printf "/workouts/%s" (.Date.Format "2006-01-02")) }}
        <div class="timer" id="workout-timer" aria-live="polite" aria-label="Workout timer">0:00</div>
        <div class="header-actions cluster">
            <a href="/workouts/{{ .Date.Format "2006-01-02" }}/exercises/{{ .ExerciseSet.ID }}/info"
               class="action-button info-button" aria-label="View exercise information">Info</a>
            <a href="/workouts/{{ .Date.Format "2006-01-02" }}/exercises/{{ .ExerciseSet.ID }}/swap"
               class="action-button swap-button" aria-label="Swap exercise">Swap</a>
        </div>
        <h1 class="exercise-title"
            data-workout-exercise-id="{{ .ExerciseSet.ID }}">{{ .ExerciseSet.Exercise.Name }}</h1>
    </header>
```

(The DOM order changes: Back · Timer · Actions · Title. The h1 stays inside the header. The data-attributes and view-transition-name on the h1 stay intact. The `#workout-timer` id stays intact.)

### Step 2: Restructure the header layout to a two-row grid at all widths

In the same file, replace the entire `<style {{ nonce }}>` block:

```gohtml
        <style {{ nonce }}>
            @scope {
                :scope {
                    display: grid;
                    grid-template-columns: auto 1fr auto;
                    align-items: center;
                    gap: var(--size-3);
                }

                .exercise-title {
                    font-size: var(--font-size-4);
                    font-weight: var(--font-weight-7);
                    color: var(--color-text-primary);
                    text-align: center;
                    margin: 0;
                    view-transition-name: exercise-title-{{ .ExerciseSet.ID }};
                }

                .header-actions {
                    justify-content: flex-end;
                }

                .action-button {
                    padding: var(--size-2) var(--size-3);
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-0);
                    font-weight: var(--font-weight-6);
                    text-decoration: none;
                    transition: background-color 0.2s ease;

                    &:focus-visible {
                        outline: 2px solid var(--color-border-focus);
                        outline-offset: 2px;
                    }

                    &.info-button {
                        background: var(--color-info-bg);
                        color: var(--color-info);

                        &:hover {
                            background: color-mix(in oklab, var(--color-info-bg) 90%, black);
                        }
                    }

                    &.swap-button {
                        background: var(--color-success-bg);
                        color: var(--color-success);

                        &:hover {
                            background: color-mix(in oklab, var(--color-success-bg) 90%, black);
                        }
                    }
                }

                @media (max-width: 480px) {
                    :scope {
                        grid-template-columns: auto 1fr;
                        row-gap: var(--size-2);
                    }

                    .header-actions {
                        grid-column: 1 / -1;
                        justify-content: center;
                    }
                }
            }
        </style>
```

With:

```gohtml
        <style {{ nonce }}>
            @scope {
                :scope {
                    display: grid;
                    grid-template-columns: auto 1fr auto;
                    align-items: center;
                    gap: var(--size-2) var(--size-3);
                }

                .exercise-title {
                    grid-column: 1 / -1;
                    font-size: var(--font-size-4);
                    font-weight: var(--font-weight-7);
                    color: var(--color-text-primary);
                    text-align: center;
                    margin: 0;
                    view-transition-name: exercise-title-{{ .ExerciseSet.ID }};
                }

                .timer {
                    justify-self: center;
                    background: var(--color-surface);
                    color: var(--color-text-primary);
                    padding: var(--size-1) var(--size-3);
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-1);
                    font-weight: var(--font-weight-6);
                    font-family: var(--font-mono);
                    border: 1px solid var(--color-border);
                    min-width: 4rem;
                    text-align: center;
                }

                .header-actions {
                    justify-content: flex-end;
                }

                .action-button {
                    padding: var(--size-2) var(--size-3);
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-0);
                    font-weight: var(--font-weight-6);
                    text-decoration: none;
                    transition: background-color 0.2s ease;

                    &:focus-visible {
                        outline: 2px solid var(--color-border-focus);
                        outline-offset: 2px;
                    }

                    &.info-button {
                        background: var(--color-info-bg);
                        color: var(--color-info);

                        &:hover {
                            background: color-mix(in oklab, var(--color-info-bg) 90%, black);
                        }
                    }

                    &.swap-button {
                        background: var(--color-success-bg);
                        color: var(--color-success);

                        &:hover {
                            background: color-mix(in oklab, var(--color-success-bg) 90%, black);
                        }
                    }
                }
            }
        </style>
```

Notes on this block:

- The grid is now `auto 1fr auto` — column 1 is the Back link (auto), column 2 is the timer pill (1fr — flex space, centered via `.timer { justify-self: center; }`), column 3 is the action cluster (auto, right-aligned). The h1 spans all three columns via `grid-column: 1 / -1` on Row 2.
- The DOM order placed in Step 1 (Back · Timer · Actions · Title) maps to grid auto-flow: cells 1, 2, 3 fill Row 1 left-to-right (Back, Timer, Actions); the fourth cell with `grid-column: 1 / -1` starts a new row and spans full width (Title).
- The `@media (max-width: 480px)` block is removed entirely. The single two-row layout works at every width — on narrow viewports the Back/Timer/Actions row stays compact (Back link is short text, Timer pill is `~5rem`, Actions cluster is small pills) and the h1 wraps naturally on Row 2 if needed.
- The `.timer` rule no longer uses `position: fixed`, `top:`, `right:`, or `isolation: isolate`. It becomes an in-flow inline pill. Padding tightens slightly (`var(--size-1) var(--size-3)` rather than `var(--size-2) var(--size-3)`) so the pill height matches the action-button row height (both rules above ride at `var(--font-size-0..1)`).
- All other tokens (`--color-surface`, `--color-text-primary`, `--color-border`, `--font-mono`, `--font-size-1`, `--font-weight-6`, `--radius-2`, `--size-1/2/3`) are unchanged from the previous timer styling; only the positioning is new.

### Step 3: Remove the timer markup and scoped style from the page shell

In `ui/templates/pages/exerciseset/exerciseset.gohtml`, find:

```gohtml
        <style {{ nonce }}>
            @scope {
                :scope {
                    gap: var(--size-5);
                    max-width: 600px;
                    margin: var(--size-4) auto;
                    padding: var(--size-4);

                    @media (max-width: 768px) {
                        margin: var(--size-2) auto;
                        padding: var(--size-3);
                        gap: var(--size-4);
                    }
                }

                .timer {
                    position: fixed;
                    top: var(--size-2);
                    right: var(--size-2);
                    isolation: isolate;
                    background: var(--color-surface);
                    color: var(--color-text-primary);
                    padding: var(--size-2) var(--size-3);
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-1);
                    font-weight: var(--font-weight-6);
                    font-family: var(--font-mono);
                    border: 1px solid var(--color-border);
                    min-width: 4rem;
                    text-align: center;
                }
            }
        </style>
```

Replace with (the `.timer` rule is gone — only the page-container `:scope` rule remains):

```gohtml
        <style {{ nonce }}>
            @scope {
                :scope {
                    gap: var(--size-5);
                    max-width: 600px;
                    margin: var(--size-4) auto;
                    padding: var(--size-4);

                    @media (max-width: 768px) {
                        margin: var(--size-2) auto;
                        padding: var(--size-3);
                        gap: var(--size-4);
                    }
                }
            }
        </style>
```

Then in the same file, find and remove the `<div class="timer">` at the bottom of `<main>`:

Find:

```gohtml
        {{ template "exercise-header" . }}
        {{ template "warmup" . }}
        {{ template "sets-container" . }}
        <div class="timer" id="workout-timer" aria-live="polite" aria-label="Workout timer">0:00</div>
    </main>
```

Replace with:

```gohtml
        {{ template "exercise-header" . }}
        {{ template "warmup" . }}
        {{ template "sets-container" . }}
    </main>
```

The `<script>` block above `{{ template "exercise-header" . }}` stays unchanged — `initializeTimer()` still calls `getElementById('workout-timer')` and the element is still in the DOM, just under a different parent.

### Step 4: Render-test gate

Run: `go test ./cmd/web/ -run Test_application_exerciseSet -v -count=1`

Expected: all four matching tests PASS:
- `Test_application_exerciseSet`
- `Test_application_exerciseSet_swap_preserves_url_and_drops_completed_sets`
- `Test_application_exerciseSet_nonexistent_exercise_returns_custom_404`
- `Test_application_exerciseSet_assisted_storage`

(None of these query the `.timer` or `#workout-timer` element, but they all render `exerciseset` end-to-end through `goquery` — a template syntax error in either touched file would fail at the document-parse stage.)

### Step 5: Commit

After the gate is green:

```bash
git add ui/templates/pages/exerciseset/exercise-header.gohtml \
        ui/templates/pages/exerciseset/exerciseset.gohtml
git commit -m "$(cat <<'EOF'
Move the exercise title onto its own row and inline the workout timer

The h1 was sharing its row with the Back link, and the workout-elapsed
timer was a fixed-position viewport overlay that clipped the title on
mobile. Restructure the header to a two-row grid at all widths — Row 1
holds Back, the timer pill, and the Info/Swap actions; Row 2 holds the
full-width h1. The .timer markup and styles relocate from the page
shell into the header; #workout-timer stays so initializeTimer() works
unchanged.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Form-row pair for weighted/assisted — Weight + Actual Reps side-by-side

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`

The active weighted/assisted form currently stacks `<div class="input-field">` Weight, optional `<div class="input-field assisted-field">`, and `<div class="input-field">` Actual Reps as three vertical rows. Wrap the two main `.input-field`s (Weight + Reps) in a new `<div class="set-form-row">` styled as a 2-column grid so they sit side-by-side. The assisted-field moves to a position **between** the input-row and the signal/submit group — semantic order is "enter weight, enter reps, confirm whether it was assisted (modifies the weight's meaning), submit".

Time-based and bodyweight forms have a single input and are unaffected.

### Step 1: Restructure the weighted/assisted form markup

In `ui/templates/pages/exerciseset/sets-container.gohtml`, find:

```gohtml
                    {{ if or (eq $.ExerciseSet.Exercise.ExerciseType "weighted") (eq $.ExerciseSet.Exercise.ExerciseType "assisted") }}
                        <form method="post"
                              action="/workouts/{{ $.Date.Format "2006-01-02" }}/exercises/{{ $.ExerciseSet.ID }}/sets/{{ $index }}/update"
                              id="form-{{ $index }}"
                              class="set-form"
                              aria-label="Complete current set">
                            <div class="input-field">
                                <label for="weight-{{ $index }}">Weight (kg)</label>
                                <input
                                        id="weight-{{ $index }}"
                                        inputmode="decimal"
                                        pattern="[0-9,\.]*"
                                        name="weight"
                                        value="{{ formatFloat $.AbsCurrentWeight }}"
                                        step="0.5"
                                        required
                                        aria-describedby="weight-help-{{ $index }}"
                                >
                                <div id="weight-help-{{ $index }}" class="sr-only">Enter weight in kilograms</div>
                            </div>
                            {{ if eq $.ExerciseSet.Exercise.ExerciseType "assisted" }}
                            <div class="input-field assisted-field">
                                <label for="assisted-{{ $index }}">
                                    <input type="checkbox" id="assisted-{{ $index }}" name="assisted"
                                           {{ if lt $.CurrentSetTarget.WeightKg 0.0 }}checked{{ end }}>
                                    Assisted (band/machine)
                                </label>
                                <details>
                                    <summary>What's this?</summary>
                                    <p>Check this when you used a band or machine to make the exercise easier.
                                       Leave it unchecked if you added weight (e.g. with a belt).</p>
                                </details>
                            </div>
                            {{ end }}
                            <div class="input-field">
                                <label for="reps-{{ $index }}">Actual reps</label>
                                <input
                                        id="reps-{{ $index }}"
                                        inputmode="numeric"
                                        pattern="[0-9]*"
                                        name="reps"
                                        value="{{ $.CurrentSetTarget.TargetReps }}"
                                        required
                                        class="reps-input"
                                        aria-describedby="reps-help-{{ $index }}"
                                >
                                <div id="reps-help-{{ $index }}" class="sr-only">Enter actual repetitions completed</div>
                            </div>
```

Replace with (the Weight + Reps `.input-field`s are wrapped in `.set-form-row`; the assisted-field moves out from between them to AFTER the row, before the signal/submit):

```gohtml
                    {{ if or (eq $.ExerciseSet.Exercise.ExerciseType "weighted") (eq $.ExerciseSet.Exercise.ExerciseType "assisted") }}
                        <form method="post"
                              action="/workouts/{{ $.Date.Format "2006-01-02" }}/exercises/{{ $.ExerciseSet.ID }}/sets/{{ $index }}/update"
                              id="form-{{ $index }}"
                              class="set-form"
                              aria-label="Complete current set">
                            <div class="set-form-row">
                                <div class="input-field">
                                    <label for="weight-{{ $index }}">Weight (kg)</label>
                                    <input
                                            id="weight-{{ $index }}"
                                            inputmode="decimal"
                                            pattern="[0-9,\.]*"
                                            name="weight"
                                            value="{{ formatFloat $.AbsCurrentWeight }}"
                                            step="0.5"
                                            required
                                            aria-describedby="weight-help-{{ $index }}"
                                    >
                                    <div id="weight-help-{{ $index }}" class="sr-only">Enter weight in kilograms</div>
                                </div>
                                <div class="input-field">
                                    <label for="reps-{{ $index }}">Actual reps</label>
                                    <input
                                            id="reps-{{ $index }}"
                                            inputmode="numeric"
                                            pattern="[0-9]*"
                                            name="reps"
                                            value="{{ $.CurrentSetTarget.TargetReps }}"
                                            required
                                            class="reps-input"
                                            aria-describedby="reps-help-{{ $index }}"
                                    >
                                    <div id="reps-help-{{ $index }}" class="sr-only">Enter actual repetitions completed</div>
                                </div>
                            </div>
                            {{ if eq $.ExerciseSet.Exercise.ExerciseType "assisted" }}
                            <div class="input-field assisted-field">
                                <label for="assisted-{{ $index }}">
                                    <input type="checkbox" id="assisted-{{ $index }}" name="assisted"
                                           {{ if lt $.CurrentSetTarget.WeightKg 0.0 }}checked{{ end }}>
                                    Assisted (band/machine)
                                </label>
                                <details>
                                    <summary>What's this?</summary>
                                    <p>Check this when you used a band or machine to make the exercise easier.
                                       Leave it unchecked if you added weight (e.g. with a belt).</p>
                                </details>
                            </div>
                            {{ end }}
```

The trailing part of the form (the `{{ if $.IsDeload }}` Done button or the `{{ else }}` signal-group `<fieldset>`) stays unchanged.

Notes:
- The Weight and Actual Reps `.input-field`s keep their classes, IDs, `name` attributes, `aria-describedby`, `class="reps-input"`, the `.sr-only` helper divs — everything the form-submission / focus / a11y plumbing depends on. The only change is they're now inside a wrapper `<div class="set-form-row">`.
- The `<input name="weight">`, `<input name="reps">`, `<button name="signal">`, `class="reps-input"` selectors used by the test suite (`Test_application_exerciseSet`, `Test_application_exerciseSet_assisted_storage`) are all preserved.
- The `<script>` block in `exerciseset.gohtml` queries `form.querySelector('.reps-input')` on mount — the `.reps-input` class stays on the reps input, so focus-on-load continues to work.
- For an **assisted** exercise: the previous order was [Weight] · [Assisted-checkbox] · [Reps]. The new order is [Weight | Reps] · [Assisted-checkbox]. Semantically the assisted checkbox is about the meaning of the weight value; placing it between the inputs (current) keeps it visually adjacent to the weight, but the layout splits the side-by-side flow. Placing it AFTER the inputs (new) trades that proximity for a cleaner inputs row, and is justified because the assisted choice is a "before you submit, also tell us..." modifier — semantically a refinement step rather than an input-time choice.

### Step 2: Add the `.set-form-row` scoped CSS

Inside the second `&.active { … }` block (the form-widgets block from Increment 4 Task 5), the existing combined rule for `.set-form, .bodyweight-form` already defines the form's vertical flex layout. The new `.set-form-row` is a child container that arranges Weight + Reps in two columns.

In `ui/templates/pages/exerciseset/sets-container.gohtml`, find this part of the second `&.active { … }` block (it's the first rule inside that block — `.set-form, .bodyweight-form`):

```css
                    &.active {
                        .set-form,
                        .bodyweight-form {
                            display: flex;
                            flex-direction: column;
                            gap: var(--size-4);
                            padding-top: var(--size-3);
                            border-top: 1px solid var(--stone-7);
                        }

                        .bodyweight-form {
                            flex-direction: row;
                            align-items: end;
                            flex-wrap: wrap;
                        }
```

Replace with (insert a new `.set-form-row` rule between `.bodyweight-form` and the `.input-field` rule that follows):

```css
                    &.active {
                        .set-form,
                        .bodyweight-form {
                            display: flex;
                            flex-direction: column;
                            gap: var(--size-4);
                            padding-top: var(--size-3);
                            border-top: 1px solid var(--stone-7);
                        }

                        .bodyweight-form {
                            flex-direction: row;
                            align-items: end;
                            flex-wrap: wrap;
                        }

                        .set-form-row {
                            display: grid;
                            grid-template-columns: 1fr 1fr;
                            gap: var(--size-3);
                        }
```

(The `.input-field` rule and everything below it stays unchanged.)

### Step 3: Let the form inputs fill their column

The current `.input-field input` rule sets `width: 6rem`, which is too narrow for the column the input now sits in (each grid column is roughly `(panel-width - size-3 gap) / 2`, much wider than 6rem). Update the rule so the input fills the column width while keeping the digit-centered, glanceable feel:

In `ui/templates/pages/exerciseset/sets-container.gohtml`, in the second `&.active { … }` block, find:

```css
                        .input-field {
                            display: flex;
                            flex-direction: column;
                            gap: var(--size-1);

                            label {
                                font-size: var(--font-size-0);
                                color: var(--stone-3);
                                font-weight: var(--font-weight-6);
                                text-transform: uppercase;
                                letter-spacing: var(--font-letterspacing-3);
                            }

                            input {
                                width: 6rem;
                                padding: var(--size-2) var(--size-3);
                                border: 2px solid var(--stone-7);
                                border-radius: var(--radius-2);
                                text-align: center;
                                font-size: var(--font-size-3);
                                font-weight: var(--font-weight-7);
                                font-family: var(--font-mono);
                                background: var(--stone-8);
                                color: var(--stone-0);
                                transition: border-color 0.2s ease, box-shadow 0.2s ease;
```

Replace with:

```css
                        .input-field {
                            display: flex;
                            flex-direction: column;
                            gap: var(--size-1);

                            label {
                                font-size: var(--font-size-0);
                                color: var(--stone-3);
                                font-weight: var(--font-weight-6);
                                text-transform: uppercase;
                                letter-spacing: var(--font-letterspacing-3);
                            }

                            input {
                                width: 100%;
                                padding: var(--size-2) var(--size-3);
                                border: 2px solid var(--stone-7);
                                border-radius: var(--radius-2);
                                text-align: center;
                                font-size: var(--font-size-3);
                                font-weight: var(--font-weight-7);
                                font-family: var(--font-mono);
                                background: var(--stone-8);
                                color: var(--stone-0);
                                transition: border-color 0.2s ease, box-shadow 0.2s ease;
```

(One change: `width: 6rem` → `width: 100%`. Everything else in the `input` rule is unchanged.)

This makes the input grow to fill its container's width:
- Inside `.set-form-row` (weighted/assisted Weight + Reps): each input fills its grid column (~50% of the panel interior width minus the gap).
- Inside `.bodyweight-form` (bodyweight single-input row): the input fills available row space; but the `.bodyweight-form` rule sets `flex-direction: row; align-items: end; flex-wrap: wrap`, and the single `<input>` shares the row with the Done button. With `width: 100%` the input would consume all flex-space — undesirable. To prevent that, add a flex sizing constraint on bodyweight inputs (see below).
- Inside the time-based form (also `.set-form` with a single `.input-field`): single column, full width is appropriate.

For the bodyweight case, add a constraint so the input doesn't push the button to a wrap row at typical viewport widths. In the same second `&.active { … }` block, find:

```css
                        .bodyweight-form {
                            flex-direction: row;
                            align-items: end;
                            flex-wrap: wrap;
                        }
```

Replace with:

```css
                        .bodyweight-form {
                            flex-direction: row;
                            align-items: end;
                            flex-wrap: wrap;
                        }

                        .bodyweight-form .input-field {
                            flex: 0 1 8rem;
                        }

                        .bodyweight-form .input-field input {
                            width: 100%;
                        }
```

(`flex: 0 1 8rem` gives the bodyweight input-field a target width of 8rem; the `width: 100%` on the input inside makes the input fill that 8rem. The Done submit-button continues to share the row and wrap below if needed.)

### Step 4: Render-test gate

Run: `go test ./cmd/web/ -run 'Test_application_exerciseSet|Test_application_exerciseSet_assisted_storage|TestExerciseSetGET_DeloadHidesSignalButtons|Test_ExerciseSet_RestChipAfterCompletedSet' -v -count=1`

Expected: every matching test PASSES. `Test_application_exerciseSet_assisted_storage` is the most critical — it exercises the assisted form three times (with and without the checkbox) and asserts that the submitted weight is stored correctly. The new layout puts the assisted-field on a different row from Weight, but the form's submission semantics (the `<input name="weight">`, `<input name="assisted">`, `<input name="reps">`, signal `<button name="signal">` ) are unchanged, so submission stays valid.

### Step 5: Commit

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "$(cat <<'EOF'
Lay Weight and Actual Reps side-by-side in the active set form

The weighted/assisted active form was stacking Weight, optional
Assisted checkbox, and Actual Reps as three vertical rows — too much
vertical space inside the dark panel. Wrap Weight + Actual Reps in a
.set-form-row 1fr/1fr grid so they sit side-by-side, and move the
assisted-field to a full-width row between the input pair and the
signal-buttons (semantic: enter values, confirm the weight-meaning,
then signal). Inputs grow to fill their column via width:100%; the
bodyweight single-input form keeps an 8rem cap so the input does not
push the Done button to a wrap row.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Drop the redundant `reps` suffix on the active oversized display

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`

The active row's oversized `.reps` column renders `REPS / 8 / reps` — the column header is "REPS" (via the `::before { content: "Reps"; }` pseudo-element on the `.reps` span), the big digit is 8, and the trailing text-node suffix is " reps". The header and the suffix are the same word; drop the suffix on the active row only.

For `.weight` the column header "WEIGHT" and the suffix "kg" differ, so the suffix carries information — keep it.

The change is template-only and scoped to the "Recommended weight" branch (the active weighted/assisted row). Upcoming and completed rows are not affected.

### Step 1: Drop the trailing ` reps` text on the active .reps span

In `ui/templates/pages/exerciseset/sets-container.gohtml`, find:

```gohtml
                        {{ else if and $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
                            <span class="weight" aria-label="Recommended weight"><span class="value">{{ formatFloat $.CurrentSetTarget.WeightKg }}</span> kg</span>
                            <span class="reps" aria-label="Target reps"><span class="value">{{ $.CurrentSetTarget.TargetReps }}</span> reps</span>
```

Replace with:

```gohtml
                        {{ else if and $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
                            <span class="weight" aria-label="Recommended weight"><span class="value">{{ formatFloat $.CurrentSetTarget.WeightKg }}</span> kg</span>
                            <span class="reps" aria-label="Target reps"><span class="value">{{ $.CurrentSetTarget.TargetReps }}</span></span>
```

(One change: drop the trailing ` reps` text node from inside the active `.reps` span. The `.weight` span and its " kg" suffix stay unchanged.)

The `aria-label="Target reps"` remains, so screen-readers continue to read the value as a rep count. The `.value` digit is centered inside the `.reps` column under the "REPS" `::before` label, giving a clean `REPS / 8` glance.

### Step 2: Render-test gate

Run: `go test ./cmd/web/ -run Test_application_exerciseSet -v -count=1`

Expected: all four matching tests PASS. The "Recommended weight" branch only renders on the active weighted/assisted row pre-submit. Tests query `.exercise-set.completed .reps` text (a different DOM path — completed rows are rendered through a different template branch that this change does not touch) and assert on values like `"12 reps"` — unaffected.

The active row's text now reads `"8"` instead of `"8 reps"` when concatenated via `goquery.Text()`. No test asserts on the active row's `.reps` text content (only existence via `.Length() > 0`).

### Step 3: Commit

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "$(cat <<'EOF'
Drop the redundant "reps" suffix on the active oversized display

The active row's oversized .reps column was rendering "REPS / 8 / reps" —
the pseudo-element column header and the trailing suffix were the same
word. Drop the suffix on the active "Recommended weight" branch only;
.weight keeps its " kg" suffix because the column header "WEIGHT" and
the unit "KG" carry different information. Upcoming and completed rows
are unaffected.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Full-suite verification

**Files:** none (verification only)

### Step 1: Cross-file grep gate is still clean

Increment 4 cleared the raw-ramp / `--gray-*` / `--white` holdouts across all four exerciseset templates. Increment 4.1 must not reintroduce any.

Run:
```bash
grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/exerciseset/*.gohtml
```

Expected: no output (exit status 1).

### Step 2: Full exerciseset test suite

Run:
```bash
go test ./cmd/web/ -run 'ExerciseSet|exerciseSet|computeSetActive' -v -count=1
```

Expected: all nine matching tests PASS:
- `Test_application_exerciseSet`
- `Test_application_exerciseSet_swap_preserves_url_and_drops_completed_sets`
- `Test_application_exerciseSet_nonexistent_exercise_returns_custom_404`
- `Test_application_workoutSwapExercise_search_filters_by_name`
- `Test_application_workoutSwapExercise_sorts_by_similarity`
- `Test_application_exerciseSet_assisted_storage`
- `Test_ExerciseSet_RestChipAfterCompletedSet`
- `TestExerciseSetGET_DeloadHidesSignalButtons`
- `Test_computeSetActive` (with its six sub-tests)

### Step 3: Full CI suite

Run: `make ci`

Expected: `init`, `build`, `lint-fix`, `test`, `sec` all pass. If `lint-fix` makes formatting changes, stage and commit them with message `Apply lint-fix formatting` and re-run.

### Step 4: Visual QA NOT performed

State explicitly: "Visual QA on `make dev` not performed in this environment. Verification rests on the per-task render tests (all 9 exerciseset tests PASS), the cross-file grep gate (silent), and `make ci` (PASS)." Do not claim the visual checks happened.

### Step 5: Final commit log

Run:
```bash
git log --oneline main..HEAD
```

Expected: three Increment 4.1 commits (plus optional lint-fix), in order:
1. `Move the exercise title onto its own row and inline the workout timer`
2. `Lay Weight and Actual Reps side-by-side in the active set form`
3. `Drop the redundant "reps" suffix on the active oversized display`

---

## Self-review notes

- **Scope:** four tasks, three template files touched (`exercise-header.gohtml`, `exerciseset.gohtml`, `sets-container.gohtml`), zero Go files, zero test files. Same shape as Increment 4 — selectors / aria / IDs preserved; the handler-tests-as-contract design holds.
- **Timer relocation:** the `#workout-timer` element keeps its DOM id and the `initializeTimer()` script in `exerciseset.gohtml` continues to populate it. The `<script>` block doesn't move because it depends on template data (`{{ if .LastCompletedAt }}` etc.) available at the page level. The timer DOM element being parented under `<header>` rather than `<main>` is semantically fine — it's an aria-live region with an explicit label, and screen-readers will announce updates regardless of which element wraps it.
- **DOM order vs visual order in the new header:** the DOM order is Back, Timer, Actions, Title. CSS grid lays them out as Row 1 [Back · Timer · Actions] (left-to-right by grid auto-flow) and Row 2 [Title] (full-width via `grid-column: 1 / -1`). Screen-reader reading order matches DOM order, which puts the timer announcement before the action buttons and before the title — slight reorder from the original Back → Title → Actions. A11y-acceptable: the timer is an `aria-live="polite"` region and won't interrupt; the title is read after, which preserves the workflow "I am on the exercise screen for X".
- **Assisted-field relocation:** the previous order put the assisted checkbox between Weight and Reps in DOM order, visually adjacent to the weight value (logical given the checkbox modifies the weight's sign). The new order puts the assisted checkbox AFTER the input row, which trades visual adjacency for a cleaner side-by-side input layout. The semantic relationship is preserved by the inline `<details><summary>What's this?</summary></details>` explanation, which the user can expand to recover context. Test still passes (`Test_application_exerciseSet_assisted_storage` exercises the submission logic, not the visual layout).
- **`width: 100%` on inputs:** the previous `width: 6rem` was a fixed pill width that worked when each input had its own row but is too narrow for a half-column. Switching to `width: 100%` lets the input fill its column for weighted/assisted (now 1/2 of the panel interior, ~120-150px on mobile) and the time-based single-column (full panel interior, ~280-330px). The bodyweight case overrides with a flex-basis cap (`flex: 0 1 8rem`) so the input doesn't push the Done button to a wrap row at typical viewport widths.
- **No new tokens, no new `color-mix` call sites, no Go change.** Pure layout-and-markup polish.
- **Time-based oversized display defect noted out-of-scope:** the time-based active row renders the oversized treatment with only one column populated and the column label reads `Reps` (wrong for seconds). User didn't call this out; flag for a future Increment 4.2.
- **Verification shape unchanged:** per-task render test + final cross-file grep + full `make ci`. Same as Increments 1-4.
