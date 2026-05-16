# Exercise Set page redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pull the active-workout screen onto the workout-overview aesthetic (serif title + mono overline + warm surfaces, semantic warning/success states), give the set list a stable grid so numbers stack, move the active card off the dark slab onto a warm amber wash with a left rule strip, and replace raw signal enum text with human strings — all without touching the signal submit contract or any domain/service rule.

**Architecture:** Bottom-up per the project's standard layer-outward workflow. Two new methods on `domain.Signal` produce display strings. The handler grows two pure-derivation fields (`CurrentSetNumber`, `TotalSetCount`) and `setDisplay` grows two pre-rendered signal strings; no domain or service change. The three exerciseset templates (`exercise-header`, `warmup`, `sets-container`) get rewritten with scoped CSS that mirrors the workout-overview language; `exerciseset.gohtml` drops the now-unused elapsed-timer script. Test selectors that hard-code old copy or class names get updated alongside the markup that produces them.

**Tech Stack:** Go 1.22+, `html/template` with `@scope` scoped CSS in `<style {{ nonce }}>` blocks, goquery-driven `e2etest` handler tests, Playwright for full-flow tests.

**Spec:** `docs/superpowers/specs/2026-05-16-exerciseset-page-redesign-design.md`

---

## File Structure

| File | Action | Responsibility |
| --- | --- | --- |
| `internal/domain/set.go` | Modify | Add `Signal.Label()` and `Signal.Glyph()` methods. |
| `internal/domain/set_test.go` | Create | Cover Label/Glyph mapping for all three signal values. |
| `cmd/web/handler-exerciseset.go` | Modify | Add `CurrentSetNumber`, `TotalSetCount` to template data; add `SignalLabel`, `SignalGlyph` to `setDisplay`; populate them. |
| `cmd/web/handler-exerciseset_test.go` | Modify | Update copy ("Mark Warmup Complete" → "Mark done"), update class selectors (`.edit-link` → `.set-edit`, `.weight`/`.reps` → `.set-weight`/`.set-reps`, `.set-info` selectors). |
| `cmd/web/playwright_test.go` | Modify | Update "Mark Warmup Complete" button-name selectors to "Mark done". |
| `ui/templates/pages/exerciseset/exercise-header.gohtml` | Rewrite | Mono overline → serif title → quiet nav row. View-transition name preserved. |
| `ui/templates/pages/exerciseset/exerciseset.gohtml` | Modify | Remove `initializeTimer()` function and call site. Keep autofocus-on-reps script. |
| `ui/templates/pages/exerciseset/warmup.gohtml` | Rewrite | Slim row + single ghost-small button (incomplete) or mono one-liner (complete). |
| `ui/templates/pages/exerciseset/sets-container.gohtml` | Rewrite | New per-row grid, warm-wash active card with left rule strip, relocated rest chip, relabelled signal buttons, reworded question. |

Each file change is self-contained. The plan commits at task boundaries so a reviewer can bisect.

---

## Task 1: Add display methods on `domain.Signal`

**Files:**
- Modify: `internal/domain/set.go`
- Create: `internal/domain/set_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/domain/set_test.go`:

```go
package domain_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestSignal_Label(t *testing.T) {
	tests := []struct {
		signal domain.Signal
		want   string
	}{
		{domain.SignalTooHeavy, "too heavy"},
		{domain.SignalTooLight, "too light"},
		{domain.SignalOnTarget, ""},
		{domain.Signal("unknown"), ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.signal), func(t *testing.T) {
			if got := tt.signal.Label(); got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSignal_Glyph(t *testing.T) {
	tests := []struct {
		signal domain.Signal
		want   string
	}{
		{domain.SignalTooHeavy, "↓"},
		{domain.SignalTooLight, "↑"},
		{domain.SignalOnTarget, ""},
		{domain.Signal("unknown"), ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.signal), func(t *testing.T) {
			if got := tt.signal.Glyph(); got != tt.want {
				t.Errorf("Glyph() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/domain -run TestSignal_`
Expected: FAIL — `signal.Label undefined` and `signal.Glyph undefined`.

- [ ] **Step 3: Implement the methods**

Edit `internal/domain/set.go` — add after the const block:

```go
// Label returns a human-readable display string for the signal.
// Returns "" for SignalOnTarget so the UI can hide the badge in the expected case.
func (s Signal) Label() string {
	switch s {
	case SignalTooHeavy:
		return "too heavy"
	case SignalTooLight:
		return "too light"
	case SignalOnTarget:
		return ""
	default:
		return ""
	}
}

// Glyph returns a single-character direction indicator for the signal
// (↓ for too-heavy, ↑ for too-light). Empty for SignalOnTarget.
func (s Signal) Glyph() string {
	switch s {
	case SignalTooHeavy:
		return "↓"
	case SignalTooLight:
		return "↑"
	case SignalOnTarget:
		return ""
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/domain -run TestSignal_`
Expected: PASS, both subtests green for all four cases.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/set.go internal/domain/set_test.go
git commit -m "$(cat <<'EOF'
Add Signal.Label and Signal.Glyph display methods

Returns human-readable text ("too heavy" / "too light") and a direction
glyph (↓ / ↑) for the Signal enum so templates don't render raw enum
strings. SignalOnTarget returns "" so the UI can hide the badge in the
expected case.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Extend handler template data

**Files:**
- Modify: `cmd/web/handler-exerciseset.go`

The handler additions are pure additive derivations of existing data. There is no test for them in isolation — they're verified by Task 4/5/6's template tests. We keep this task small so it commits cleanly.

- [ ] **Step 1: Extend `setDisplay` with signal display strings**

Edit `cmd/web/handler-exerciseset.go`, find the `setDisplay` struct (around line 15) and add the two fields:

```go
type setDisplay struct {
	Set          domain.Set
	TargetStr    string // Pre-formatted target string (e.g. "5", "30s").
	CompletedStr string // Pre-formatted completed string, same unit as TargetStr.
	Unit         string // "reps" or "seconds" — for input labels.
	Number       int    // 1-based set number for display.
	IsActive     bool   // Whether this row renders the completion form.
	SignalLabel  string // Human label ("too heavy"/"too light"/""). "" hides the badge.
	SignalGlyph  string // Direction glyph ("↓"/"↑"/""). Empty when Label is empty.
}
```

- [ ] **Step 2: Populate the new fields in `prepareSetsDisplay`**

In the same file, in `prepareSetsDisplay`, modify the loop body so each `setDisplay` carries the strings:

```go
func prepareSetsDisplay(exercise domain.Exercise, sets []domain.Set) []setDisplay {
	unit := exercise.SetValueUnit()
	displays := make([]setDisplay, len(sets))
	for i, set := range sets {
		completedStr := ""
		if set.CompletedValue != nil {
			completedStr = exercise.FormatSetValue(*set.CompletedValue)
		}
		signalLabel, signalGlyph := "", ""
		if set.Signal != nil {
			signalLabel = set.Signal.Label()
			signalGlyph = set.Signal.Glyph()
		}
		displays[i] = setDisplay{
			Set:          set,
			TargetStr:    exercise.FormatSetValue(set.TargetValue),
			CompletedStr: completedStr,
			Unit:         unit,
			Number:       i + 1,
			IsActive:     false, // Populated by the caller after firstIncompleteIndex is known.
			SignalLabel:  signalLabel,
			SignalGlyph:  signalGlyph,
		}
	}
	return displays
}
```

- [ ] **Step 3: Extend `exerciseSetTemplateData` with set-count fields**

In the same file, add to `exerciseSetTemplateData`:

```go
type exerciseSetTemplateData struct {
	BaseTemplateData
	Date                  time.Time
	ExerciseSet           domain.ExerciseSet
	SetsDisplay           []setDisplay // Enhanced set data with formatted rep strings
	FirstIncompleteIndex  int
	EditingIndex          int              // Index of the set being edited
	IsEditing             bool             // Whether we're in edit mode
	IsDeload              bool             // Whether this session is a deload week.
	LastCompletedAt       *time.Time       // Timestamp of most recently completed set
	CurrentSetTarget      domain.SetTarget // Recommended weight and reps from progression
	CurrentSetTimedTarget int              // Recommended seconds for time_based exercises; 0 for others.
	AbsCurrentWeight      float64          // |CurrentSetTarget.WeightKg|, for assisted form input
	RestEndAtMs           int64            // 0 when no rest chip should be shown.
	CurrentSetNumber      int              // 1-based number of the first incomplete set (or len+1 when all done).
	TotalSetCount         int              // len(ExerciseSet.Sets), for the "Set N of M" overline.
}
```

- [ ] **Step 4: Populate the new fields where `data` is constructed**

In the same file, find the `data := exerciseSetTemplateData{...}` literal around line 183 and add the two fields. The exact construction site is the only spot where `FirstIncompleteIndex` and `SetsDisplay` are computed, so both new values are trivial. Locate the literal and append:

```go
	data := exerciseSetTemplateData{
		BaseTemplateData:      newBaseTemplateData(r),
		Date:                  date,
		ExerciseSet:           exerciseSet,
		SetsDisplay:           prepareSetsDisplay(exerciseSet.Exercise, exerciseSet.Sets),
		FirstIncompleteIndex:  getFirstIncompleteIndex(exerciseSet.Sets),
		// ...existing fields preserved...
		CurrentSetNumber:      getFirstIncompleteIndex(exerciseSet.Sets) + 1,
		TotalSetCount:         len(exerciseSet.Sets),
	}
```

Note: read the file first to copy the existing literal verbatim — don't paraphrase. Keep all existing fields in their original order, append the two new ones at the end.

- [ ] **Step 5: Run the build to verify compilation**

Run: `go build ./...`
Expected: clean build, no errors.

- [ ] **Step 6: Run the existing exerciseset tests to verify no regression**

Run: `go test ./cmd/web -run TestExerciseSet -count=1`
Expected: PASS — additive changes can't break existing assertions.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-exerciseset.go
git commit -m "$(cat <<'EOF'
Extend exerciseset template data for redesigned page

Adds CurrentSetNumber and TotalSetCount on exerciseSetTemplateData for
the new "Set N of M" mono overline, and SignalLabel/SignalGlyph on
setDisplay so templates render the human signal strings produced by
the new Signal.Label/Glyph methods instead of reaching into the enum.

All additive; no field renames or removals. Templates pick the new
fields up in follow-up commits.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Rewrite `exercise-header.gohtml` as a serif brief

**Files:**
- Modify: `ui/templates/pages/exerciseset/exercise-header.gohtml`
- Modify: `ui/templates/pages/exerciseset/exerciseset.gohtml`

This task does the header rewrite and drops the now-unused `initializeTimer()` JS from the parent template in the same commit (the elapsed timer is removed; nothing else calls the function).

- [ ] **Step 1: Rewrite the header template**

Replace the entire contents of `ui/templates/pages/exerciseset/exercise-header.gohtml` with:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.exerciseSetTemplateData*/ -}}

{{ define "exercise-header" }}
    <header class="exercise-brief">
        <style {{ nonce }}>
            @scope (.exercise-brief) {
                :scope {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-3);
                    padding-top: var(--size-3);
                }

                .brief-overline {
                    font-family: var(--font-mono);
                    font-size: var(--font-size-0);
                    letter-spacing: var(--font-letterspacing-3);
                    text-transform: uppercase;
                    color: var(--clay-4);
                    font-weight: var(--font-weight-6);
                    display: flex;
                    align-items: center;
                    gap: var(--size-2);
                }

                .brief-overline::after {
                    content: "";
                    flex: 1;
                    height: 1px;
                    background: linear-gradient(to right, var(--stone-3), transparent);
                }

                .brief-overline .dot {
                    color: var(--stone-4);
                }

                .exercise-title {
                    font-family: Charter, "Iowan Old Style", Georgia, ui-serif, serif;
                    font-weight: var(--font-weight-6);
                    font-size: clamp(2.25rem, 9vw, 3rem);
                    line-height: 0.95;
                    letter-spacing: -0.025em;
                    color: var(--color-text-primary);
                    margin: 0;
                    view-transition-name: exercise-title-{{ .ExerciseSet.ID }};
                }

                .brief-nav {
                    display: flex;
                    align-items: center;
                    justify-content: space-between;
                    gap: var(--size-3);
                    flex-wrap: wrap;
                }

                .brief-actions {
                    display: flex;
                    align-items: center;
                    gap: var(--size-2);
                }

                .brief-action {
                    display: inline-flex;
                    align-items: center;
                    padding: var(--size-1) var(--size-2);
                    color: var(--color-text-secondary);
                    font-weight: var(--font-weight-6);
                    font-size: var(--font-size-1);
                    text-decoration: none;
                    border-radius: var(--radius-2);
                    transition: color var(--duration-2) var(--ease-out-quiet),
                                background var(--duration-2) var(--ease-out-quiet);
                }

                .brief-action:hover {
                    color: var(--color-text-primary);
                    background: var(--color-surface-elevated);
                }

                .brief-action:focus-visible {
                    outline: 2px solid var(--color-border-focus);
                    outline-offset: 2px;
                }
            }
        </style>

        <div class="brief-overline">
            <span>{{ .Date.Format "Mon · Jan 2" }}</span>
            <span class="dot">·</span>
            <span>Set {{ .CurrentSetNumber }} of {{ .TotalSetCount }}</span>
        </div>

        <h1 class="exercise-title"
            data-workout-exercise-id="{{ .ExerciseSet.ID }}">{{ .ExerciseSet.Exercise.Name }}</h1>

        <div class="brief-nav">
            {{ template "back-link" (printf "/workouts/%s" (.Date.Format "2006-01-02")) }}
            <div class="brief-actions">
                <a class="brief-action"
                   href="/workouts/{{ .Date.Format "2006-01-02" }}/exercises/{{ .ExerciseSet.ID }}/info"
                   aria-label="View exercise information">Info</a>
                <a class="brief-action"
                   href="/workouts/{{ .Date.Format "2006-01-02" }}/exercises/{{ .ExerciseSet.ID }}/swap"
                   aria-label="Swap exercise">Swap</a>
            </div>
        </div>
    </header>
{{ end }}
```

- [ ] **Step 2: Drop the elapsed-timer script from the page template**

Edit `ui/templates/pages/exerciseset/exerciseset.gohtml`. Remove the entire `initializeTimer()` function definition (everything from `function initializeTimer() {` through its closing brace, inclusive) and the `initializeTimer()` call inside the `DOMContentLoaded` listener. Keep the reps-autofocus block intact.

After the edit, the `<script {{ nonce }}>` block should look like:

```gohtml
        <script {{ nonce }}>
          document.addEventListener("DOMContentLoaded", function () {
            const form = document.getElementById(`form-{{ $.EditingIndex }}`)
            if (form) {
              const repsInput = form.querySelector('.reps-input')
              if (repsInput) {
                setTimeout(() => {
                  repsInput.focus()
                  repsInput.select()
                }, 100)
              }
            }
          })
        </script>
```

- [ ] **Step 3: Build to verify the page still compiles**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 4: Run the existing tests to verify nothing breaks**

Run: `go test ./cmd/web -run TestExerciseSet -count=1`
Expected: PASS — the header's existing test selectors (back link, page heading, etc.) still resolve since the markup keeps semantic equivalents (`<h1>`, the back-link partial). If a test fails because it asserts on `.timer` or the elapsed-timer markup, update the test in this task — that selector no longer applies.

If any tests fail, fix them now: search for `workout-timer`, `.timer`, `info-button`, `swap-button` in `cmd/web/handler-exerciseset_test.go` and `cmd/web/playwright_test.go`. Update each to the new class (`.brief-action.info` / `.brief-action.swap` if needed, or simply remove the assertion if it was decorative). Re-run the tests until green.

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exerciseset/exercise-header.gohtml \
        ui/templates/pages/exerciseset/exerciseset.gohtml \
        cmd/web/handler-exerciseset_test.go cmd/web/playwright_test.go
# the last two only if Step 4 required edits
git commit -m "$(cat <<'EOF'
Rewrite exerciseset header as a serif training brief

Mono date + "Set N of M" overline, Charter-serif exercise title, and a
quiet nav row with the back link on the left and Info/Swap as
inheriting text-buttons on the right. Drops the bordered elapsed-timer
chrome (and the initializeTimer JS), matching the workout overview's
visual language. View-transition-name is preserved so the page-to-page
title slide still works.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Rewrite `warmup.gohtml` as a slim row

**Files:**
- Modify: `ui/templates/pages/exerciseset/warmup.gohtml`
- Modify: `cmd/web/handler-exerciseset_test.go`
- Modify: `cmd/web/playwright_test.go`

- [ ] **Step 1: Rewrite the warmup template**

Replace the entire contents of `ui/templates/pages/exerciseset/warmup.gohtml` with:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.exerciseSetTemplateData*/ -}}

{{ define "warmup" }}
    {{ if not .ExerciseSet.WarmupCompletedAt }}
        <div class="warmup-row" role="region" aria-label="Warmup">
            <style {{ nonce }}>
                @scope (.warmup-row) {
                    :scope {
                        display: flex;
                        align-items: center;
                        gap: var(--size-3);
                        padding: var(--size-3);
                        border-radius: var(--radius-3);
                        background: var(--color-surface-elevated);
                        box-shadow: var(--shadow-1);
                        flex-wrap: wrap;
                    }

                    .warmup-label {
                        font-family: var(--font-mono);
                        font-size: var(--font-size-0);
                        letter-spacing: var(--font-letterspacing-3);
                        text-transform: uppercase;
                        font-weight: var(--font-weight-7);
                        color: var(--color-text-primary);
                    }

                    .warmup-hint {
                        color: var(--color-text-secondary);
                        font-size: var(--font-size-1);
                        flex: 1;
                        min-width: 10rem;
                    }

                    form {
                        margin: 0;
                        margin-left: auto;
                    }
                }
            </style>
            <span class="warmup-label">Warm up</span>
            <span class="warmup-hint">a few easy reps to prime</span>
            <form method="post"
                  action="/workouts/{{ .Date.Format "2006-01-02" }}/exercises/{{ .ExerciseSet.ID }}/warmup/complete">
                <button type="submit" class="btn btn--quiet btn--sm">Mark done</button>
            </form>
        </div>
    {{ else }}
        <div class="warmup-done" role="status">
            <style {{ nonce }}>
                @scope (.warmup-done) {
                    :scope {
                        font-family: var(--font-mono);
                        font-size: var(--font-size-0);
                        letter-spacing: var(--font-letterspacing-3);
                        text-transform: uppercase;
                        color: var(--color-success);
                        font-weight: var(--font-weight-6);
                        padding: var(--size-1) 0;
                    }
                }
            </style>
            ✓ warmup complete
        </div>
    {{ end }}
{{ end }}
```

- [ ] **Step 2: Update handler test selectors that reference the old warmup button copy**

In `cmd/web/handler-exerciseset_test.go`, replace every occurrence of `button:contains('Mark Warmup Complete')` with `button:contains('Mark done')`. There are 4 occurrences (lines 125, 320, 980, 1123 at time of writing).

Easiest via Edit's `replace_all`:

```
old_string: button:contains('Mark Warmup Complete')
new_string: button:contains('Mark done')
```

- [ ] **Step 3: Update Playwright test selectors that reference the old warmup button copy**

In `cmd/web/playwright_test.go`, replace every occurrence of `Mark Warmup Complete` with `Mark done`. There are 3 occurrences (lines 202–203, 506, 814–816).

```
old_string: Mark Warmup Complete
new_string: Mark done
```

Note: this is the accessible button name used by `page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "..."})`. The button's `aria-label` defaults to its text content, so updating the visible text is sufficient.

- [ ] **Step 4: Run handler tests**

Run: `go test ./cmd/web -run TestExerciseSet -count=1`
Expected: PASS.

If a test fails because it looked for the old warmup banner styling (`.warmup-banner`, `.warmup-message`, `.warmup-status`, `.warmup-description`), grep for those class names in `cmd/web/handler-exerciseset_test.go` and update — the markup no longer carries them.

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exerciseset/warmup.gohtml \
        cmd/web/handler-exerciseset_test.go cmd/web/playwright_test.go
git commit -m "$(cat <<'EOF'
Collapse warmup banner into a slim row

Replaces the boxed banner with a single-row callout sized like a
planned set: mono "Warm up" overline, one-line hint, quiet small
"Mark done" button on the right. The bordered green status pill
becomes a one-line mono confirmation when the warmup is complete.

Updates the handler and Playwright tests that asserted on the old
button copy ("Mark Warmup Complete" → "Mark done").

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Rewrite `sets-container.gohtml` (the bulk of the visual change)

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`
- Modify: `cmd/web/handler-exerciseset_test.go`

This is the largest task. It replaces the per-row markup with a stable grid, moves the active card off the dark slab onto the warm amber wash, relocates the rest chip into the active card, relabels the signal buttons, and reworks the question copy. The form submit shape (`action`, `name="signal" value="..."`, weight/reps `name`/`id`) is preserved so the handler is untouched.

- [ ] **Step 1: Rewrite the template**

Replace the entire contents of `ui/templates/pages/exerciseset/sets-container.gohtml` with the following. This is long; copy it verbatim:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.exerciseSetTemplateData*/ -}}

{{ define "sets-container" }}
    <div class="sets-container{{ if not .ExerciseSet.WarmupCompletedAt }} disabled{{ end }}"
         aria-label="Exercise sets"{{ if not .ExerciseSet.WarmupCompletedAt }} aria-disabled="true"{{ end }}>
        <style {{ nonce }}>
            @scope (.sets-container) {
                :scope {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-2);
                }

                :scope.disabled {
                    opacity: 0.6;
                    pointer-events: none;
                }

                /* ---------- Planned & Done rows (same grid, two skins) ---------- */
                .exercise-set {
                    display: grid;
                    grid-template-columns: minmax(3rem, auto) 1fr auto;
                    align-items: baseline;
                    gap: var(--size-3);
                    padding: var(--size-3);
                    border-radius: var(--radius-3);
                    background: var(--color-surface-elevated);
                    box-shadow: var(--shadow-1);
                    transition: background var(--duration-2) var(--ease-out-quiet);
                }

                .set-index {
                    font-family: var(--font-mono);
                    font-size: var(--font-size-0);
                    letter-spacing: var(--font-letterspacing-3);
                    text-transform: uppercase;
                    color: var(--color-text-secondary);
                    font-weight: var(--font-weight-6);
                }

                .set-figures {
                    font-family: Charter, "Iowan Old Style", Georgia, ui-serif, serif;
                    font-size: var(--font-size-2);
                    color: var(--color-text-primary);
                    font-variant-numeric: tabular-nums;
                    display: inline-flex;
                    align-items: baseline;
                    gap: var(--size-2);
                }

                .set-figures .sep {
                    color: var(--stone-4);
                }

                .set-trailing {
                    display: inline-flex;
                    align-items: center;
                    gap: var(--size-3);
                    font-size: var(--font-size-0);
                }

                .set-status {
                    color: var(--color-success);
                    font-weight: var(--font-weight-7);
                }

                .set-signal {
                    font-family: var(--font-mono);
                    letter-spacing: var(--font-letterspacing-1);
                    text-transform: uppercase;
                    color: var(--color-success);
                    font-weight: var(--font-weight-6);
                }

                .set-edit {
                    color: var(--color-success);
                    text-decoration: none;
                    font-weight: var(--font-weight-6);
                }

                .set-edit:hover {
                    text-decoration: underline;
                    text-underline-offset: 0.2em;
                }

                .set-edit:focus-visible {
                    outline: 2px solid var(--color-border-focus);
                    outline-offset: 2px;
                    border-radius: var(--radius-1);
                }

                /* Done state — warm success wash; figures keep serif. */
                .exercise-set.completed {
                    background: var(--color-success-bg);
                }

                .exercise-set.completed .set-index,
                .exercise-set.completed .set-figures {
                    color: var(--color-success);
                }

                /* ---------- Active card — warm amber wash, hero numbers, in-card rest chip ---------- */
                .exercise-set.active {
                    display: block;
                    background: var(--color-warning-bg);
                    box-shadow: var(--shadow-2);
                    padding: var(--size-4);
                    position: relative;
                }

                .exercise-set.active::before {
                    content: "";
                    position: absolute;
                    left: 0;
                    top: var(--size-3);
                    bottom: var(--size-3);
                    width: 3px;
                    background: var(--color-warning);
                    border-radius: 0 3px 3px 0;
                }

                .exercise-set.active .active-head {
                    display: flex;
                    align-items: baseline;
                    justify-content: space-between;
                    gap: var(--size-3);
                    margin-bottom: var(--size-3);
                }

                .exercise-set.active .active-head .set-index {
                    color: var(--color-warning);
                    font-weight: var(--font-weight-7);
                }

                .exercise-set.active .active-hero {
                    display: flex;
                    align-items: baseline;
                    justify-content: center;
                    gap: var(--size-3);
                    font-family: Charter, "Iowan Old Style", Georgia, ui-serif, serif;
                    font-size: var(--font-size-fluid-3);
                    color: var(--color-text-primary);
                    font-variant-numeric: tabular-nums;
                    line-height: 1;
                    padding: var(--size-3) 0 var(--size-4);
                }

                .exercise-set.active .active-hero .sep {
                    color: var(--stone-5);
                    font-size: 0.8em;
                }

                .exercise-set.active .active-hero .unit {
                    font-family: var(--font-mono);
                    font-size: var(--font-size-1);
                    text-transform: uppercase;
                    letter-spacing: var(--font-letterspacing-3);
                    color: var(--color-text-secondary);
                    margin-left: var(--size-1);
                }

                /* Form ------------------------------------------------------------ */
                .exercise-set.active .set-form,
                .exercise-set.active .bodyweight-form {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-4);
                }

                .exercise-set.active .bodyweight-form {
                    flex-direction: row;
                    align-items: end;
                    flex-wrap: wrap;
                }

                .exercise-set.active .bodyweight-form .input-field {
                    flex: 0 1 8rem;
                }

                .exercise-set.active .set-form-row {
                    display: grid;
                    grid-template-columns: 1fr 1fr;
                    gap: var(--size-3);
                }

                .exercise-set.active .input-field {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-1);
                }

                .exercise-set.active .input-field label {
                    font-size: var(--font-size-0);
                    color: var(--color-text-secondary);
                    font-weight: var(--font-weight-6);
                    text-transform: uppercase;
                    letter-spacing: var(--font-letterspacing-3);
                }

                .exercise-set.active .input-field input {
                    width: 100%;
                    padding: var(--size-2) var(--size-3);
                    border: 2px solid var(--stone-3);
                    border-radius: var(--radius-2);
                    text-align: center;
                    font-size: var(--font-size-3);
                    font-weight: var(--font-weight-7);
                    font-family: var(--font-mono);
                    background: var(--color-surface);
                    color: var(--color-text-primary);
                    transition: border-color 0.2s ease, box-shadow 0.2s ease;
                }

                .exercise-set.active .input-field input:focus {
                    outline: none;
                    border-color: var(--ember);
                    box-shadow: 0 0 0 3px rgb(224 138 76 / 0.25);
                }

                .exercise-set.active .input-field input:user-invalid {
                    border-color: var(--color-error);
                }

                .exercise-set.active .timed-form .input-field input {
                    width: 6rem;
                }

                .exercise-set.active .assisted-field {
                    display: flex;
                    flex-direction: row;
                    align-items: center;
                    gap: var(--size-2);
                    flex-wrap: wrap;
                }

                .exercise-set.active .assisted-field label {
                    display: flex;
                    align-items: center;
                    gap: var(--size-2);
                    font-size: var(--font-size-1);
                    color: var(--color-text-primary);
                    font-weight: var(--font-weight-4);
                    cursor: pointer;
                    text-transform: none;
                    letter-spacing: normal;
                }

                .exercise-set.active .assisted-field input[type="checkbox"] {
                    width: auto;
                    height: 1.25rem;
                    margin: 0;
                    padding: 0;
                    border: revert;
                    border-radius: revert;
                    background: revert;
                    text-align: revert;
                    font-size: revert;
                    box-shadow: none;
                }

                .exercise-set.active .assisted-field details {
                    font-size: var(--font-size-0);
                    color: var(--color-text-secondary);
                }

                .exercise-set.active .assisted-field details summary {
                    cursor: pointer;
                    color: var(--ember);
                }

                .exercise-set.active .assisted-field details summary:focus-visible {
                    outline: 3px solid var(--ember);
                    outline-offset: 2px;
                }

                /* Signal group + buttons ----------------------------------------- */
                .exercise-set.active .signal-group {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-2);
                    border: none;
                    padding: 0;
                    margin: 0;
                    min-width: 0;
                }

                .exercise-set.active .signal-group > legend {
                    font-size: var(--font-size-1);
                    font-weight: var(--font-weight-6);
                    color: var(--color-text-primary);
                    padding: 0;
                    margin-bottom: var(--size-1);
                }

                .exercise-set.active .signal-buttons {
                    display: grid;
                    grid-template-columns: 1fr 1.5fr 1fr;
                    gap: var(--size-2);
                }

                @media (max-width: 380px) {
                    .exercise-set.active .signal-buttons {
                        grid-template-columns: 1fr;
                    }
                }

                .exercise-set.active .signal-btn {
                    display: inline-flex;
                    align-items: center;
                    justify-content: center;
                    text-align: center;
                    padding: var(--size-3) var(--size-2);
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-7);
                    font-size: var(--font-size-1);
                    cursor: pointer;
                    border: 2px solid transparent;
                    min-height: 3rem;
                    transition: background-color 0.2s ease, box-shadow 0.2s ease;
                }

                .exercise-set.active .signal-btn:focus-visible {
                    outline: 3px solid var(--ember);
                    outline-offset: 2px;
                }

                .exercise-set.active .signal-btn.too-heavy-btn {
                    background: transparent;
                    color: var(--color-text-secondary);
                    border-color: var(--stone-3);
                }

                .exercise-set.active .signal-btn.too-heavy-btn:hover {
                    background: var(--color-surface);
                }

                .exercise-set.active .signal-btn.too-heavy-btn:active {
                    background: var(--color-surface);
                    box-shadow: 0 0 0 3px var(--color-warning);
                }

                .exercise-set.active .signal-btn.on-target-btn {
                    background: var(--ember);
                    color: var(--stone-10);
                    border-color: var(--ember);
                    font-size: var(--font-size-2);
                    font-weight: var(--font-weight-8);
                    min-height: 3.5rem;
                    text-transform: uppercase;
                    letter-spacing: var(--font-letterspacing-3);
                }

                .exercise-set.active .signal-btn.on-target-btn:hover {
                    background: color-mix(in oklab, var(--ember) 90%, white);
                }

                .exercise-set.active .signal-btn.on-target-btn:active {
                    background: color-mix(in oklab, var(--ember) 85%, black);
                    box-shadow: 0 0 0 3px var(--clay-2);
                }

                .exercise-set.active .signal-btn.too-light-btn {
                    background: transparent;
                    color: var(--color-text-secondary);
                    border-color: var(--stone-3);
                }

                .exercise-set.active .signal-btn.too-light-btn:hover {
                    background: var(--color-surface);
                }

                .exercise-set.active .signal-btn.too-light-btn:active {
                    background: var(--color-surface);
                    box-shadow: 0 0 0 3px var(--color-info);
                }

                /* In-card rest chip --------------------------------------------- */
                .exercise-set.active .rest-chip {
                    display: inline-flex;
                    align-items: center;
                    gap: var(--size-2);
                    padding: var(--size-1) var(--size-3);
                    border-radius: var(--radius-round);
                    background: var(--color-info-bg);
                    color: var(--color-info);
                    font-weight: var(--font-weight-6);
                    font-size: var(--font-size-0);
                    transition: background-color var(--duration-3) var(--ease-out-quiet),
                                color var(--duration-3) var(--ease-out-quiet);
                }

                .exercise-set.active .rest-chip.ready {
                    background: var(--color-success-bg);
                    color: var(--color-success);
                }
            }
        </style>

        {{ if .IsDeload }}
        <style {{ nonce }}>
            @scope (.deload-banner) {
                :scope {
                    margin: 0;
                    padding: var(--size-3) var(--size-4);
                    background: var(--color-info-bg);
                    color: var(--color-info);
                    border-left: var(--border-size-3) solid var(--color-info);
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-1);
                }
            }
        </style>
        <div class="deload-banner" role="status">
            Deload week — lighter loads, same weight every set. Just hit your reps and rest.
            These sets don't influence future progression.
        </div>
        {{ end }}

        {{ range $index, $setDisplay := .SetsDisplay }}
            {{ $set := $setDisplay.Set }}
            {{ $isActive := $setDisplay.IsActive }}
            <div class="exercise-set{{ if $set.CompletedValue }} completed{{ end }}{{ if $isActive }} active{{ end }}"
                 role="group"
                 {{ if $isActive }}aria-current="step"{{ end }}
                 aria-label="{{ if $set.CompletedValue }}Completed set{{ else if $isActive }}Current set{{ else }}Upcoming set{{ end }}">

                {{ if $isActive }}
                    {{ /* Active card ---------------------------------- */ }}
                    <div class="active-head">
                        <span class="set-index">Set {{ $setDisplay.Number }}</span>
                        {{ if gt $.RestEndAtMs 0 }}
                            <div class="rest-chip" data-rest-end-at-ms="{{ $.RestEndAtMs }}" aria-live="polite">
                                <span>Rest</span>
                                <span data-rest-time>—:—</span>
                            </div>
                        {{ end }}
                    </div>

                    {{ if or (eq $.ExerciseSet.Exercise.ExerciseType "weighted") (eq $.ExerciseSet.Exercise.ExerciseType "assisted") }}
                        <div class="active-hero">
                            <span>{{ formatFloat $.CurrentSetTarget.WeightKg }}<span class="unit">kg</span></span>
                            <span class="sep">×</span>
                            <span>{{ $.CurrentSetTarget.TargetReps }}<span class="unit">reps</span></span>
                        </div>
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
                                    >
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
                                    >
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
                            {{ if $.IsDeload }}
                                <button type="submit" class="btn btn--focus btn--block" aria-label="Complete set">Done!</button>
                            {{ else }}
                                <fieldset class="signal-group">
                                    <legend>How did {{ $.CurrentSetTarget.TargetReps }} reps feel?</legend>
                                    <div class="signal-buttons">
                                        <button type="submit" name="signal" value="too_heavy"
                                                class="signal-btn too-heavy-btn"
                                                aria-label="Too heavy — failed to reach target reps">too heavy</button>
                                        <button type="submit" name="signal" value="on_target"
                                                class="signal-btn on-target-btn"
                                                aria-label="Barely reached target reps">BARELY</button>
                                        <button type="submit" name="signal" value="too_light"
                                                class="signal-btn too-light-btn"
                                                aria-label="Easy — could have done more reps">easy</button>
                                    </div>
                                </fieldset>
                            {{ end }}
                        </form>
                    {{ else if eq $.ExerciseSet.Exercise.ExerciseType "time_based" }}
                        <div class="active-hero">
                            <span>{{ $.CurrentSetTimedTarget }}<span class="unit">sec</span></span>
                        </div>
                        <div class="timed-runner" data-timed-runner
                             data-target-seconds="{{ $.CurrentSetTimedTarget }}"
                             hidden>
                            <div class="timed-runner-display" data-phase="idle">
                                <span class="timed-runner-label" data-label aria-live="polite">Get ready</span>
                                <span class="timed-runner-time" data-time aria-hidden="true">0:10</span>
                            </div>
                            <button type="button" class="timed-runner-cancel" data-cancel>Cancel</button>
                        </div>
                        <button type="button" class="btn btn--focus btn--block timed-runner-start"
                                data-timed-runner-start hidden>Start timer</button>
                        <style {{ nonce }}>
                            @scope (.timed-runner) {
                                :scope {
                                    display: flex;
                                    flex-direction: column;
                                    align-items: center;
                                    gap: var(--size-3);
                                    padding: var(--size-4);
                                    margin-bottom: var(--size-3);
                                    background: var(--color-info-bg);
                                    color: var(--color-info);
                                    border-radius: var(--radius-3);
                                }
                                .timed-runner-display {
                                    display: flex;
                                    flex-direction: column;
                                    align-items: center;
                                    gap: var(--size-1);
                                }
                                .timed-runner-label {
                                    font-size: var(--font-size-0);
                                    text-transform: uppercase;
                                    letter-spacing: var(--font-letterspacing-3);
                                    font-weight: var(--font-weight-7);
                                }
                                .timed-runner-time {
                                    font-family: var(--font-mono);
                                    font-size: var(--font-size-7);
                                    font-weight: var(--font-weight-8);
                                    line-height: 1;
                                    font-variant-numeric: tabular-nums;
                                }
                                .timed-runner-display[data-phase="exercise"] {
                                    color: var(--color-success);
                                }
                                .timed-runner-cancel {
                                    padding: var(--size-1) var(--size-3);
                                    background: transparent;
                                    color: inherit;
                                    border: var(--border-size-1) solid currentColor;
                                    border-radius: var(--radius-2);
                                    font-size: var(--font-size-0);
                                    text-transform: uppercase;
                                    letter-spacing: var(--font-letterspacing-3);
                                    cursor: pointer;
                                }
                            }
                        </style>
                        <script {{ nonce }}>
                            (() => {
                                const root = document.querySelector('[data-timed-runner]');
                                const startBtn = document.querySelector('[data-timed-runner-start]');
                                if (!root || !startBtn) return;

                                const targetSeconds = parseInt(root.dataset.targetSeconds, 10);
                                if (!Number.isFinite(targetSeconds) || targetSeconds <= 0) return;

                                const form = document.querySelector('.set-form.timed-form');
                                if (!form) return;

                                const display = root.querySelector('.timed-runner-display');
                                const labelEl = root.querySelector('[data-label]');
                                const timeEl = root.querySelector('[data-time]');
                                const cancelBtn = root.querySelector('[data-cancel]');

                                const PREP_SECONDS = 10;
                                let intervalId = null;
                                let phase = 'idle';
                                let deadline = 0;
                                let lastWhole = -1;
                                let audioCtx = null;
                                let wakeLock = null;

                                startBtn.hidden = false;

                                function fmt(seconds) {
                                    const s = Math.max(0, Math.ceil(seconds));
                                    const m = Math.floor(s / 60);
                                    const r = s % 60;
                                    return m + ':' + String(r).padStart(2, '0');
                                }

                                function ensureAudio() {
                                    const Ctor = window.AudioContext || window.webkitAudioContext;
                                    if (!Ctor) return null;
                                    if (!audioCtx) audioCtx = new Ctor();
                                    if (audioCtx.state === 'suspended') audioCtx.resume();
                                    return audioCtx;
                                }

                                function playBeep(freq, ms) {
                                    const ctx = ensureAudio();
                                    if (!ctx) return;
                                    const t0 = ctx.currentTime;
                                    const t1 = t0 + ms / 1000;
                                    const osc = ctx.createOscillator();
                                    const gain = ctx.createGain();
                                    osc.type = 'sine';
                                    osc.frequency.value = freq;
                                    gain.gain.setValueAtTime(0, t0);
                                    gain.gain.linearRampToValueAtTime(0.3, t0 + 0.01);
                                    gain.gain.setValueAtTime(0.3, Math.max(t0 + 0.01, t1 - 0.01));
                                    gain.gain.linearRampToValueAtTime(0, t1);
                                    osc.connect(gain).connect(ctx.destination);
                                    osc.start(t0);
                                    osc.stop(t1);
                                }

                                async function acquireWakeLock() {
                                    if (!('wakeLock' in navigator)) return;
                                    try {
                                        const sentinel = await navigator.wakeLock.request('screen');
                                        if (phase === 'idle') {
                                            sentinel.release().catch((err) => console.debug('wake lock release failed', err));
                                            return;
                                        }
                                        sentinel.addEventListener('release', () => {
                                            if (wakeLock === sentinel) wakeLock = null;
                                        });
                                        wakeLock = sentinel;
                                    } catch (err) {
                                        console.debug('wake lock request failed', err);
                                    }
                                }

                                function releaseWakeLock() {
                                    if (!wakeLock) return;
                                    const sentinel = wakeLock;
                                    wakeLock = null;
                                    sentinel.release().catch((err) => console.debug('wake lock release failed', err));
                                }

                                function tick() {
                                    const remainingMs = deadline - Date.now();
                                    const remainingSec = remainingMs / 1000;
                                    timeEl.textContent = fmt(remainingSec);
                                    if (phase === 'prep') {
                                        const whole = Math.ceil(remainingSec);
                                        if (whole !== lastWhole && whole >= 1 && whole <= 3) {
                                            playBeep(800, 120);
                                        }
                                        lastWhole = whole;
                                        if (remainingMs <= 0) beginExercise();
                                    } else if (phase === 'exercise') {
                                        if (remainingMs <= 0) finishExercise();
                                    }
                                }

                                function beginPrep() {
                                    ensureAudio();
                                    acquireWakeLock();
                                    form.hidden = true;
                                    startBtn.hidden = true;
                                    root.hidden = false;
                                    labelEl.textContent = 'Get ready';
                                    display.dataset.phase = 'prep';
                                    phase = 'prep';
                                    deadline = Date.now() + PREP_SECONDS * 1000;
                                    lastWhole = -1;
                                    timeEl.textContent = fmt(PREP_SECONDS);
                                    intervalId = setInterval(tick, 100);
                                }

                                function beginExercise() {
                                    playBeep(1200, 250);
                                    labelEl.textContent = 'Hold';
                                    display.dataset.phase = 'exercise';
                                    phase = 'exercise';
                                    deadline = Date.now() + targetSeconds * 1000;
                                    timeEl.textContent = fmt(targetSeconds);
                                }

                                function finishExercise() {
                                    playBeep(440, 600);
                                    teardown();
                                }

                                function teardown() {
                                    if (intervalId !== null) {
                                        clearInterval(intervalId);
                                        intervalId = null;
                                    }
                                    phase = 'idle';
                                    releaseWakeLock();
                                    root.hidden = true;
                                    display.dataset.phase = 'idle';
                                    form.hidden = false;
                                    startBtn.hidden = false;
                                }

                                startBtn.addEventListener('click', beginPrep);
                                cancelBtn.addEventListener('click', teardown);

                                document.addEventListener('visibilitychange', () => {
                                    if (document.visibilityState === 'visible'
                                            && phase !== 'idle' && !wakeLock) {
                                        acquireWakeLock();
                                    }
                                });

                                window.addEventListener('beforeunload', releaseWakeLock);
                            })();
                        </script>
                        <form method="post"
                              action="/workouts/{{ $.Date.Format "2006-01-02" }}/exercises/{{ $.ExerciseSet.ID }}/sets/{{ $index }}/update"
                              id="form-{{ $index }}"
                              class="set-form timed-form"
                              aria-label="Complete current hold">
                            <div class="input-field">
                                <label for="completed-value-{{ $index }}">Seconds held</label>
                                <input
                                        id="completed-value-{{ $index }}"
                                        inputmode="numeric"
                                        pattern="[0-9]*"
                                        name="completed_value"
                                        value="{{ $.CurrentSetTimedTarget }}"
                                        required
                                        class="reps-input"
                                >
                            </div>
                            {{ if $.IsDeload }}
                                <button type="submit" class="btn btn--focus btn--block" aria-label="Complete set">Done!</button>
                            {{ else }}
                                <fieldset class="signal-group">
                                    <legend>How did {{ $.CurrentSetTimedTarget }}s feel?</legend>
                                    <div class="signal-buttons">
                                        <button type="submit" name="signal" value="too_heavy"
                                                class="signal-btn too-heavy-btn"
                                                aria-label="Too heavy — failed to reach the target hold">too heavy</button>
                                        <button type="submit" name="signal" value="on_target"
                                                class="signal-btn on-target-btn"
                                                aria-label="Barely reached target hold">BARELY</button>
                                        <button type="submit" name="signal" value="too_light"
                                                class="signal-btn too-light-btn"
                                                aria-label="Easy — could have held longer">easy</button>
                                    </div>
                                </fieldset>
                            {{ end }}
                        </form>
                    {{ else }}
                        <div class="active-hero">
                            <span>{{ $setDisplay.TargetStr }}<span class="unit">{{ $setDisplay.Unit }}</span></span>
                        </div>
                        <form method="post"
                              action="/workouts/{{ $.Date.Format "2006-01-02" }}/exercises/{{ $.ExerciseSet.ID }}/sets/{{ $index }}/update"
                              id="form-{{ $index }}"
                              class="set-form bodyweight-form"
                              aria-label="Complete current set">
                            <div class="input-field">
                                <label for="completed-value-{{ $index }}">{{ $setDisplay.Unit }}</label>
                                <input
                                        id="completed-value-{{ $index }}"
                                        inputmode="numeric"
                                        pattern="[0-9]*"
                                        name="completed_value"
                                        placeholder="{{ $setDisplay.TargetStr }}"
                                        {{ if $set.CompletedValue }}value="{{ $setDisplay.CompletedStr }}"{{ end }}
                                        required
                                        {{ if $set.CompletedValue }}disabled{{ end }}
                                        class="reps-input"
                                >
                            </div>
                            <button type="submit" class="btn btn--focus btn--block" aria-label="Complete set">Done!</button>
                        </form>
                    {{ end }}
                {{ else }}
                    {{ /* Planned & Done rows ------------------------------ */ }}
                    <span class="set-index">Set {{ $setDisplay.Number }}</span>
                    <span class="set-figures">
                        {{ if or (eq $.ExerciseSet.Exercise.ExerciseType "weighted") (eq $.ExerciseSet.Exercise.ExerciseType "assisted") }}
                            {{ if $set.CompletedValue }}
                                <span class="set-weight">{{ formatFloat $set.WeightKg }} kg</span>
                                <span class="sep">·</span>
                                <span class="set-reps">{{ $setDisplay.CompletedStr }} {{ $setDisplay.Unit }}</span>
                            {{ else }}
                                <span class="set-weight">{{ formatFloat $.CurrentSetTarget.WeightKg }} kg</span>
                                <span class="sep">·</span>
                                <span class="set-reps">{{ $.CurrentSetTarget.TargetReps }}</span>
                            {{ end }}
                        {{ else }}
                            {{ if $set.CompletedValue }}
                                <span class="set-reps">{{ $setDisplay.CompletedStr }} {{ $setDisplay.Unit }}</span>
                            {{ else }}
                                <span class="set-reps">{{ $setDisplay.TargetStr }} {{ $setDisplay.Unit }}</span>
                            {{ end }}
                        {{ end }}
                    </span>
                    {{ if $set.CompletedValue }}
                        <span class="set-trailing">
                            <span class="set-status" aria-hidden="true">✓</span>
                            {{ if $setDisplay.SignalLabel }}
                                <span class="set-signal" aria-label="Signal">{{ $setDisplay.SignalGlyph }} {{ $setDisplay.SignalLabel }}</span>
                            {{ end }}
                            <a href="?edit={{ $index }}" class="set-edit" aria-label="Edit set {{ $setDisplay.Number }}">edit</a>
                        </span>
                    {{ end }}
                {{ end }}
            </div>

            {{ if $isActive }}
                {{ if gt $.RestEndAtMs 0 }}
                <script {{ nonce }}>
                    (() => {
                        const chips = document.querySelectorAll('.exercise-set.active .rest-chip[data-rest-end-at-ms]');
                        if (chips.length === 0) return;
                        const tick = () => {
                            const now = Date.now();
                            chips.forEach((chip) => {
                                const endAt = parseInt(chip.dataset.restEndAtMs, 10);
                                const remaining = Math.max(0, endAt - now);
                                const timeEl = chip.querySelector('[data-rest-time]');
                                if (remaining === 0) {
                                    chip.classList.add('ready');
                                    if (timeEl) timeEl.textContent = 'Ready';
                                } else {
                                    const totalSec = Math.ceil(remaining / 1000);
                                    const m = Math.floor(totalSec / 60);
                                    const s = totalSec % 60;
                                    if (timeEl) timeEl.textContent = m + ':' + String(s).padStart(2, '0');
                                }
                            });
                        };
                        tick();
                        setInterval(tick, 250);
                    })();
                </script>
                {{ end }}
            {{ end }}
        {{ end }}
    </div>
{{ end }}
```

Notes on what changed structurally:

- The previous top-of-list `.rest-chip` block (lines 354–404 of the original) is removed. The rest chip now renders inside the active card's `.active-head`. The driving script moves alongside it.
- The per-row markup splits into two paths: the active card has its own structure (`.active-head`, `.active-hero`, the form), while planned/done rows share a `[set-index | set-figures | set-trailing]` grid.
- `.exercise-set` wrapper class is preserved (plus `.completed` and `.active` modifiers) so the existing test selectors keep working at the wrapper level.
- Inner class renames: `.weight` → `.set-weight`, `.reps` → `.set-reps`, `.edit-link` → `.set-edit`. The old `.set-info` wrapper is gone.

- [ ] **Step 2: Update handler test selectors that referenced the old inner class names**

In `cmd/web/handler-exerciseset_test.go`:

```
old: .exercise-set.completed .edit-link
new: .exercise-set.completed .set-edit
```

```
old: .exercise-set.completed .weight
new: .exercise-set.completed .set-weight
```

```
old: .exercise-set.completed .reps
new: .exercise-set.completed .set-reps
```

```
old: .exercise-set.active .set-info
new: .exercise-set.active
```

(The `.set-info` wrapper no longer exists; the active card's structure is flatter. Update the test logic if it indexed into `.set-info` for further selection — verify it still does what it intended by reading the surrounding 10–20 lines.)

- [ ] **Step 3: Build to verify**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 4: Run handler tests**

Run: `go test ./cmd/web -count=1`
Expected: PASS. Pay attention to:
- `TestExerciseSetGET_*` family — set-row markup assertions.
- `TestExerciseSetGET_DeloadHidesSignalButtons` (line 1029) — checks `button.signal-btn` count; class name is preserved, should still pass.
- The Plank time-based active-card test (line 1299) — uses `.exercise-set.active .set-info`; update per Step 2.

If a test fails on a selector that wasn't covered above, grep for the failing selector in the test file, find the matching old markup, and update either the selector (preferred) or restore the class name in the template (only if the class was load-bearing for something other than CSS — likely never).

- [ ] **Step 5: Run Playwright tests**

Run: `go test ./cmd/web -run TestPlaywright -count=1 -timeout=10m`
Expected: PASS. The flow tests submit the signal buttons by class (`.too-heavy-btn`, etc.) — class names are preserved in this redesign.

If a Playwright test fails because of a copy assertion ("Did you reach", "Could do more", "No"), update the assertion to the new copy ("How did", "easy", "too heavy") in the same step.

- [ ] **Step 6: Manual visual sanity check**

Start the dev server (`make init` if first time, then `go run ./cmd/web`) and open `/workouts/<today>/exercises/<id>` in a mobile-width browser window (resize to ~380px). Verify:
- Serif title is prominent; mono overline reads "Set N of M".
- Warmup row is slim; "Mark done" button is on the right.
- Planned set numbers stack vertically — same indent for each row's `Set N` label.
- Active card is warm amber-tinted, with an amber rule strip on the left edge.
- Rest chip (if applicable) sits in the top-right of the active card header.
- Signal buttons read "too heavy / BARELY / easy" with the BARELY in ember orange.
- Completed sets show "✓" + `↑ too light` (or no badge for on-target) + "edit" link in the success green.
- Edit link on a completed set, when clicked, opens the edit form correctly.

- [ ] **Step 7: Run lint-fix**

Run: `make lint-fix`
Expected: no diagnostics, or only auto-fixed ones.

- [ ] **Step 8: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml \
        cmd/web/handler-exerciseset_test.go
git commit -m "$(cat <<'EOF'
Redesign exerciseset set list and active card

Replaces the per-row flex/grid mishmash with a stable
[index | figures | trailing] grid for planned and done rows, so set
numbers and weight/reps stack vertically. Active card moves off the
dark slab onto the warm warning wash with a left amber rule strip,
matching the workout overview's in-progress treatment. Hero numbers
use Charter serif with tabular figures.

Signal buttons relabelled to "too heavy / BARELY / easy" with a
"How did N reps feel?" legend; the on-target case no longer renders
a badge in completed rows, and the too-heavy / too-light badges use
Signal.Label/Glyph. Edit affordance becomes a quiet text link in the
success colour.

The rest chip moves out of the page-level position into the active
card's header. Form action and signal value attributes are unchanged
so the handler accepts the existing submit contract.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Full validation pass

**Files:**
- None new. This task is the gate before declaring the work done.

- [ ] **Step 1: Full test suite**

Run: `make test`
Expected: PASS — all packages, race detector clean, no test-shuffle order dependencies.

- [ ] **Step 2: Lint**

Run: `make lint-fix`
Expected: no diagnostics.

- [ ] **Step 3: Build the binary**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 4: Run the deploy CI gate locally**

Run: `make ci`
Expected: PASS.

- [ ] **Step 5: Manual workflow walkthrough**

Start the server, sign in, navigate to today's workout, and complete the full flow on one exercise:
- Click into an exercise from the workout overview (view-transition should still slide the title).
- See the new serif brief header + slim warmup row.
- Click "Mark done" on the warmup. Confirm it transitions to "✓ warmup complete" line.
- Submit set 1 with "BARELY". Confirm it appears as completed with green "✓ edit", no badge.
- Submit set 2 with "easy". Confirm it appears with "↑ too light" badge plus the edit link.
- Submit set 3 with "too heavy". Confirm "↓ too heavy" badge appears.
- Click "edit" on set 2. Confirm the edit form opens with set 2's values pre-filled.
- Resubmit set 2 with different reps. Confirm the row updates.
- Confirm the rest chip appears inside the active card after submitting.
- Back link returns to workout overview without breaking the view transition.

If any step misbehaves, fix in a follow-up commit rather than amending — the chain stays bisectable.

- [ ] **Step 6: Visual diff on a phone**

Open the deployed dev URL on a real phone (or a 350-414px browser viewport). Compare side-by-side with the workout overview — the two pages should now feel like one app.

- [ ] **Step 7: Final commit (only if Steps 1–6 surfaced anything)**

If everything passes cleanly, this task is just verification — no commit needed. If fixes were needed, they should each have their own commit per the bisect-friendly convention.

---

## Self-Review

Cross-checked against `docs/superpowers/specs/2026-05-16-exerciseset-page-redesign-design.md`:

- **Spec §Design / Header** → Task 3 (rewrite header).
- **Spec §Design / Warmup** → Task 4 (rewrite warmup) + warmup tests in same task.
- **Spec §Design / Set list — one layout, three skins** → Task 5, planned/done rows grid + completed wash.
- **Spec §Design / Active card — warm, not black** → Task 5, active card block + amber rule strip + relocated rest chip.
- **Spec §Design / Time-based and bodyweight active cards** → Task 5, both branches preserved with new warm treatment.
- **Spec §Design / Signal labels — display mapping** → Task 1 (domain methods) + Task 2 (setDisplay fields) + Task 5 (template usage).
- **Spec §Design / Handler additions** → Task 2 (`CurrentSetNumber`, `TotalSetCount`, `SignalLabel`, `SignalGlyph`).
- **Spec §Files touched** → All seven files appear in Tasks 1–5; the test files (handler + Playwright) appear in the tasks that introduce the breaking changes.
- **Spec §Testing** → Task 5 runs handler + Playwright; Task 6 is the `make test` / `make lint-fix` gate; manual phone check at the end.

No placeholders. No "TBD" / "implement later" / "similar to". Code blocks are complete for every step. Class-name renames (`.edit-link` → `.set-edit`, `.weight` → `.set-weight`, `.reps` → `.set-reps`, `.set-info` → flat active markup) are introduced in Task 5 and updated in the same task's test edits. Button copy renames (`Mark Warmup Complete` → `Mark done`, "Could do more" → "easy", "No" → "too heavy") are introduced in Tasks 4 and 5 respectively and the corresponding test selectors updated alongside.
