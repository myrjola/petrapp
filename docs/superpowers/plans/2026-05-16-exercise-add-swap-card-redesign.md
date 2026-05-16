# Exercise Add/Swap Card Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the inverted button hierarchy and flat metadata on the
Add Exercise / Swap Exercise card lists, and extract the duplicated
muscle-group pill and info-dialog styles into reusable global components.

**Architecture:** Two new global component classes go into
`ui/static/main.css` under `@layer components`: `.muscle-chip` (with a
`--primary` variant) and `.sheet-dialog`. The Add and Swap page templates
rewrite their card markup to use chips for category/muscle metadata,
`.cluster` to lay out a primary `.btn .btn--quiet` next to a secondary
`.btn .btn--ghost .btn--sm`, and the new `.sheet-dialog` for the per-card
info modal. The Exercise Info page migrates its local muscle-group pill
rules over to `.muscle-chip` for one consistent visual vocabulary.

**Tech Stack:** Go `html/template`, server-side rendered `.gohtml` pages,
CSS with `@scope` + `@layer`, goquery in handler tests.

**Spec:** `docs/superpowers/specs/2026-05-16-exercise-add-swap-card-redesign.md`

---

## File Structure

| File | Role |
| --- | --- |
| `ui/static/main.css` | Source of truth for global components — gains `.muscle-chip*` and `.sheet-dialog*` rules. |
| `ui/templates/pages/styleguide/styleguide.gohtml` | Living catalog — gets two new sections demonstrating the new classes. |
| `cmd/web/handler-styleguide_test.go` | Assertions that lock the styleguide entries in place. |
| `ui/templates/pages/exercise-info/exercise-info.gohtml` | Migrates `.muscle-group` / `.muscle-group.primary` → `.muscle-chip` / `.muscle-chip--primary`; drops the local rules. |
| `cmd/web/handler-exercise-info_test.go` | Updates the selector assertion to `.muscle-chip`. |
| `ui/templates/pages/exercise-add/exercise-add.gohtml` | Rewrites the card: chips for metadata, cluster of primary + ghost buttons, `<dialog class="sheet-dialog">`, no per-card inline styles. |
| `ui/templates/pages/exercise-swap/exercise-swap.gohtml` | Same rewrite, `Swap to this exercise` label, `/swap` action. |
| `cmd/web/handler-workout_test.go` | Adds structural assertions for the new Add card (one new test). |
| `cmd/web/handler-exerciseset_test.go` | Adds structural assertions for the new Swap card (one new test). |
| `ui/templates/CLAUDE.md` | Documents the two new class-components. |

No Go source under `internal/`. No handler signatures, route handlers, or template data shapes change.

## Selectors that MUST keep working (existing tests depend on them)

Don't drop or rename these in the rewrites:

- `.exercise-option` — the per-exercise card container.
- `.exercise-option .exercise-name` — the title element.
- `input[name='exercise_id']` (Add) and `input[name='new_exercise_id']` (Swap) — hidden inputs inside the submit form.
- The form `action` attributes `/workouts/{date}/add-exercise` and `/workouts/{date}/exercises/{id}/swap`.

(`cmd/web/handler-workout_test.go:183`, `:204`, `:232`; `cmd/web/handler-exerciseset_test.go:442`, `:463`, `:491`, `:711-:712`.)

`.muscle-group` on the Exercise Info page is the one selector we're intentionally renaming — Task 2 updates the test in lockstep.

---

### Task 1: Add `.muscle-chip` global component + styleguide entry + tests

**Files:**
- Modify: `ui/static/main.css` (append a new rule under `@layer components`)
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml` (insert a new `<section>` after the existing "Badges" section, before "Card")
- Modify: `cmd/web/handler-styleguide_test.go`

- [ ] **Step 1: Write the failing test additions**

Append the following two assertion blocks to `Test_application_styleguide` in `cmd/web/handler-styleguide_test.go`, after the existing "Badge and card" block (currently ending at line 57):

```go
	// Muscle-chip — pill used by exercise-info / -add / -swap pages.
	if doc.Find("h2:contains('Muscle chip')").Length() == 0 {
		t.Error("expected a 'Muscle chip' section on the styleguide")
	}
	if doc.Find(".muscle-chip").Length() == 0 {
		t.Error("expected a .muscle-chip example on the styleguide")
	}
	if doc.Find(".muscle-chip.muscle-chip--primary").Length() == 0 {
		t.Error("expected a .muscle-chip--primary example on the styleguide")
	}
```

- [ ] **Step 2: Run the test to verify it fails**

```
go test -v ./cmd/web -run Test_application_styleguide
```

Expected: FAIL with three error lines about the missing `Muscle chip` heading and `.muscle-chip` / `.muscle-chip--primary` examples.

- [ ] **Step 3: Add the CSS rules to `main.css`**

Open `ui/static/main.css`. Inside `@layer components { ... }`, after the `.badge--info` rule (currently ending around line 459) and before the closing brace of `@layer components`, insert:

```css
    .muscle-chip {
        display: inline-flex;
        align-items: center;
        padding: var(--size-1) var(--size-2);
        border-radius: var(--radius-2);
        font-size: var(--font-size-0);
        font-weight: var(--font-weight-6);
        background: var(--stone-2);
        color: var(--stone-8);
    }

    .muscle-chip--primary {
        background: var(--clay-1);
        color: var(--clay-6);
    }
```

- [ ] **Step 4: Add the styleguide entry**

Open `ui/templates/pages/styleguide/styleguide.gohtml`. Between the existing `<section>` for "Badges" (ends at line 358) and the `<section>` for "Card" (starts at line 360), insert:

```gohtml
        <section>
            <h2>Muscle chip</h2>
            <div class="component-sample">
                <span class="muscle-chip">Triceps</span>
                <span class="muscle-chip muscle-chip--primary">Chest</span>
                <span class="muscle-chip muscle-chip--primary">Shoulders</span>
            </div>
        </section>
```

- [ ] **Step 5: Run the styleguide test to verify it passes**

```
go test -v ./cmd/web -run Test_application_styleguide
```

Expected: PASS.

- [ ] **Step 6: Run the full test suite to confirm nothing regressed**

```
make test
```

Expected: PASS.

- [ ] **Step 7: Lint**

```
make lint-fix
```

Expected: no changes (or only formatting), exit code 0.

- [ ] **Step 8: Commit**

```bash
git add ui/static/main.css ui/templates/pages/styleguide/styleguide.gohtml cmd/web/handler-styleguide_test.go
git commit -m "Add .muscle-chip global component class

Extracts the clay-1/clay-6 primary-muscle pill style (defined locally in
exercise-info today) into a global component class so exercise-add and
-swap can use the same vocabulary."
```

---

### Task 2: Migrate `exercise-info` to use `.muscle-chip`

**Files:**
- Modify: `ui/templates/pages/exercise-info/exercise-info.gohtml`
- Modify: `cmd/web/handler-exercise-info_test.go:91`

- [ ] **Step 1: Update the test selector**

In `cmd/web/handler-exercise-info_test.go`, change the existing block at line 90-93:

```go
		// Check for muscle groups
		if doc.Find(".muscle-group").Length() == 0 {
			t.Error("Expected to find muscle groups")
		}
```

to:

```go
		// Check for muscle groups (rendered as .muscle-chip elements).
		if doc.Find(".muscle-chip").Length() == 0 {
			t.Error("Expected to find muscle groups")
		}
```

- [ ] **Step 2: Run the test to verify it fails**

```
go test -v ./cmd/web -run TestExerciseInfo
```

(If that doesn't match, find the actual test name with `grep -n "^func Test" cmd/web/handler-exercise-info_test.go` and use it.)

Expected: FAIL — "Expected to find muscle groups" because the template still emits `.muscle-group`.

- [ ] **Step 3: Migrate the template**

Open `ui/templates/pages/exercise-info/exercise-info.gohtml`. Make three changes:

(a) Inside the `@scope (.exercise-info)` block, **delete** these rule sets (currently lines 55-66):

```css
                        .muscle-group {
                            background: var(--stone-1);
                            border-radius: var(--radius-2);
                            padding: var(--size-1) var(--size-2);
                            font-size: var(--font-size-1);
                            color: var(--color-text-secondary);
                        }

                        .primary {
                            background: var(--clay-1);
                            color: var(--clay-6);
                        }
```

(b) Replace the Primary muscle groups markup (currently lines 89-96):

```gohtml
            <div class="section stack">
                <div class="section-title">Primary Muscle Groups</div>
                <div class="muscle-groups cluster">
                    {{ range .Exercise.PrimaryMuscleGroups }}
                        <span class="muscle-group primary">{{ . }}</span>
                    {{ end }}
                </div>
            </div>
```

with:

```gohtml
            <div class="section stack">
                <div class="section-title">Primary Muscle Groups</div>
                <div class="muscle-groups cluster">
                    {{ range .Exercise.PrimaryMuscleGroups }}
                        <span class="muscle-chip muscle-chip--primary">{{ . }}</span>
                    {{ end }}
                </div>
            </div>
```

(c) Replace the Secondary muscle groups markup (currently lines 99-105):

```gohtml
                <div class="section stack">
                    <div class="section-title">Secondary Muscle Groups</div>
                    <div class="muscle-groups cluster">
                        {{ range .Exercise.SecondaryMuscleGroups }}
                            <span class="muscle-group">{{ . }}</span>
                        {{ end }}
                    </div>
                </div>
```

with:

```gohtml
                <div class="section stack">
                    <div class="section-title">Secondary Muscle Groups</div>
                    <div class="muscle-groups cluster">
                        {{ range .Exercise.SecondaryMuscleGroups }}
                            <span class="muscle-chip">{{ . }}</span>
                        {{ end }}
                    </div>
                </div>
```

- [ ] **Step 4: Run the test to verify it passes**

```
go test -v ./cmd/web -run TestExerciseInfo
```

Expected: PASS.

- [ ] **Step 5: Run the full test suite**

```
make test
```

Expected: PASS.

- [ ] **Step 6: Lint**

```
make lint-fix
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add ui/templates/pages/exercise-info/exercise-info.gohtml cmd/web/handler-exercise-info_test.go
git commit -m "Migrate exercise-info muscle pills to .muscle-chip

Drops the local .muscle-group / .primary scoped rules in favour of the
global .muscle-chip / .muscle-chip--primary classes. Updates the handler
test to match."
```

---

### Task 3: Add `.sheet-dialog` global component + styleguide entry + test

**Files:**
- Modify: `ui/static/main.css`
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml`
- Modify: `cmd/web/handler-styleguide_test.go`

- [ ] **Step 1: Write the failing test additions**

Append to `Test_application_styleguide` in `cmd/web/handler-styleguide_test.go`, after the muscle-chip block from Task 1:

```go
	// Sheet-dialog — slide-up modal used by exercise-add / -swap.
	if doc.Find("h2:contains('Sheet dialog')").Length() == 0 {
		t.Error("expected a 'Sheet dialog' section on the styleguide")
	}
	if doc.Find("dialog.sheet-dialog").Length() == 0 {
		t.Error("expected a dialog.sheet-dialog example on the styleguide")
	}
	if doc.Find("dialog.sheet-dialog .sheet-dialog__close").Length() == 0 {
		t.Error("expected the sheet-dialog example to render a .sheet-dialog__close row")
	}
```

- [ ] **Step 2: Run the test to verify it fails**

```
go test -v ./cmd/web -run Test_application_styleguide
```

Expected: FAIL on the three new assertions.

- [ ] **Step 3: Add the CSS rules**

In `ui/static/main.css`, inside `@layer components { ... }`, after the `.muscle-chip--primary` rule added in Task 1, append:

```css
    .sheet-dialog {
        padding: var(--size-3);
        border: none;
        position: fixed;
        inset: 0;
        transform: translateY(100%);
        animation: sheet-dialog-slide-up 0.3s ease-in-out forwards;
        transition-behavior: allow-discrete;
    }

    .sheet-dialog::backdrop {
        background-color: rgba(0, 0, 0, 0.5);
        animation: fade-in 0.3s ease-out forwards;
    }

    .sheet-dialog__close {
        display: flex;
        justify-content: flex-end;
        margin-bottom: var(--size-3);
    }
```

The backdrop reuses the existing global `@keyframes fade-in` (already defined at `ui/static/main.css:526`). The slide-up animation is new — `@keyframes` can't live inside `@layer`, so add it **outside** `@layer components`, alongside the other globals (e.g. immediately after `@keyframes fade-out` around line 536):

```css
@keyframes sheet-dialog-slide-up {
    from { transform: translateY(100%); }
    to   { transform: translateY(0); }
}
```

- [ ] **Step 4: Add the styleguide entry**

In `ui/templates/pages/styleguide/styleguide.gohtml`, immediately after the "Muscle chip" `<section>` added in Task 1, insert:

```gohtml
        <section>
            <h2>Sheet dialog</h2>
            <div class="component-sample">
                <button type="button" class="btn btn--ghost btn--sm">
                    <script {{ nonce }}>
                      me().addEventListener('click', () => {
                        document.getElementById('styleguide-sheet-dialog').showModal()
                      })
                    </script>
                    Open sheet
                </button>
                <dialog id="styleguide-sheet-dialog" class="sheet-dialog">
                    <form method="dialog" class="sheet-dialog__close">
                        <button class="btn btn--ghost btn--sm">Close</button>
                    </form>
                    <p>Example sheet-dialog content. Slides up from the bottom on open.</p>
                </dialog>
            </div>
        </section>
```

- [ ] **Step 5: Run the styleguide test to verify it passes**

```
go test -v ./cmd/web -run Test_application_styleguide
```

Expected: PASS.

- [ ] **Step 6: Run the full test suite**

```
make test
```

Expected: PASS.

- [ ] **Step 7: Lint**

```
make lint-fix
```

Expected: clean.

- [ ] **Step 8: Commit**

```bash
git add ui/static/main.css ui/templates/pages/styleguide/styleguide.gohtml cmd/web/handler-styleguide_test.go
git commit -m "Add .sheet-dialog global component class

Lifts the slide-up sheet dialog CSS out of exercise-add and exercise-swap
(where it was duplicated once per exercise) into main.css. The two page
templates will switch over in the next two tasks."
```

---

### Task 4: Rewrite the Add Exercise card

**Files:**
- Modify: `ui/templates/pages/exercise-add/exercise-add.gohtml`
- Modify: `cmd/web/handler-workout_test.go`

- [ ] **Step 1: Write a failing structural test**

In `cmd/web/handler-workout_test.go`, find the existing test that exercises `/add-exercise` rendering — the block starting around line 179 that calls `client.GetDoc(ctx, "/workouts/"+today+"/add-exercise")`. After the existing assertions in that test, add (before the test function's closing brace):

```go
	// Lock in the new card structure.
	card := doc.Find(".exercise-option").First()
	if card.Length() == 0 {
		t.Fatal("expected at least one .exercise-option on the add page")
	}
	if card.Find(".badge").Length() == 0 {
		t.Error("expected category badge inside the exercise card")
	}
	if card.Find(".muscle-chip.muscle-chip--primary").Length() == 0 {
		t.Error("expected at least one .muscle-chip--primary in the card")
	}
	if card.Find(".actions .btn.btn--quiet[type='submit']").Length() == 0 {
		t.Error("expected the primary Add action as .btn.btn--quiet submit inside .actions")
	}
	if card.Find(".actions .btn.btn--ghost.btn--sm").Length() == 0 {
		t.Error("expected the secondary Info action as .btn.btn--ghost.btn--sm inside .actions")
	}
	if card.Find("dialog.sheet-dialog").Length() == 0 {
		t.Error("expected the per-card info dialog as dialog.sheet-dialog")
	}
```

If your editor is unsure which test to extend, search the file for the literal string `add-exercise page filters available exercises by name substring` (around line 144) — the test that follows is the right one.

- [ ] **Step 2: Run the test to verify it fails**

```
go test -v ./cmd/web -run TestWorkout
```

(Or use the exact test name from the surrounding `func TestXxx`.)

Expected: FAIL on the new assertions because the current template still uses the old structure.

- [ ] **Step 3: Rewrite the card markup**

Open `ui/templates/pages/exercise-add/exercise-add.gohtml`. **Replace the entire `<section class="available-exercises">` block** (currently lines 43-168) with:

```gohtml
        <section class="available-exercises">
            <style {{ nonce }}>
                @scope (.available-exercises) {
                    :scope {
                        h2 {
                            font-size: var(--font-size-2);
                            margin-bottom: var(--size-3);
                        }

                        .exercises-list {
                            gap: var(--size-3);
                        }

                        .exercise-option {
                            gap: var(--size-3);
                        }

                        .exercise-name {
                            font-weight: var(--font-weight-6);
                        }

                        .actions {
                            align-items: stretch;
                        }

                        .actions .add-form {
                            flex: 1;
                        }

                        .no-results {
                            color: var(--color-text-secondary);
                            padding: var(--size-4) 0;
                            text-align: center;
                        }
                    }
                }
            </style>
            <h2>Choose Exercise to Add</h2>

            <div class="exercises-list stack">
                {{ if .Exercises }}
                {{ range .Exercises }}
                    <div class="exercise-option card stack">
                        <div class="exercise-name">{{ .Name }}</div>
                        <div class="meta cluster">
                            <span class="badge">{{ .Category.Label }}</span>
                            {{ range .PrimaryMuscleGroups }}
                                <span class="muscle-chip muscle-chip--primary">{{ . }}</span>
                            {{ end }}
                        </div>
                        <div class="actions cluster">
                            <form method="post"
                                  action="/workouts/{{ $.Date.Format "2006-01-02" }}/add-exercise"
                                  class="add-form">
                                <input type="hidden" name="exercise_id" value="{{ .ID }}">
                                <button type="submit" class="btn btn--quiet btn--block">Add this exercise</button>
                            </form>
                            <button type="button" class="btn btn--ghost btn--sm info-trigger">
                                <script {{ nonce }}>
                                  me().addEventListener('click', () => {
                                    document.getElementById('dialog-exercise-{{ .ID }}').showModal()
                                  })
                                </script>
                                Info
                            </button>
                        </div>
                        <dialog id="dialog-exercise-{{ .ID }}" class="sheet-dialog">
                            <form method="dialog" class="sheet-dialog__close">
                                <button class="btn btn--ghost btn--sm">Close</button>
                            </form>
                            {{ mdToHTML .DescriptionMarkdown }}
                        </dialog>
                    </div>
                {{ end }}
                {{ else }}
                    <p class="no-results">
                        {{ if .Query }}No exercises match &ldquo;{{ .Query }}&rdquo;.{{ else }}No exercises available to add.{{ end }}
                    </p>
                {{ end }}
            </div>
        </section>
```

Key things this removes:
- The per-card `<style {{ nonce }}>` block that duplicated the sheet-dialog CSS for every exercise.
- The old `.option-details` / `.category` / `.muscle-groups` flat-text metadata.

Key things this preserves:
- `.exercise-option`, `.exercise-option .exercise-name`, `input[name='exercise_id']`, the form `action`.

- [ ] **Step 4: Run the test to verify it passes**

```
go test -v ./cmd/web -run TestWorkout
```

Expected: PASS.

- [ ] **Step 5: Run the full test suite**

```
make test
```

Expected: PASS — including the other Add-page tests (filter-by-query, no-match) that depend on `.exercise-option .exercise-name`.

- [ ] **Step 6: Lint**

```
make lint-fix
```

Expected: clean.

- [ ] **Step 7: Manual smoke**

Start the dev server (`go run ./cmd/web` or however you're running locally), open `/workouts/<today>/add-exercise` in a narrow browser (DevTools mobile viewport), verify:

- The card title sits on its own line.
- Category + primary muscle group(s) render as chips below the title.
- The green "Add this exercise" button is the visually dominant action and fills most of the action row.
- The "Info" button is small + bordered on the right.
- Clicking "Info" slides up the description sheet; "Close" dismisses it.

- [ ] **Step 8: Commit**

```bash
git add ui/templates/pages/exercise-add/exercise-add.gohtml cmd/web/handler-workout_test.go
git commit -m "Redesign add-exercise card

Replaces the inline grey-text metadata with category + muscle-chip rows,
puts the primary Add action next to a secondary Info ghost button in a
single cluster, and swaps the per-card inline sheet-dialog CSS for the
new global .sheet-dialog class."
```

---

### Task 5: Rewrite the Swap Exercise card

**Files:**
- Modify: `ui/templates/pages/exercise-swap/exercise-swap.gohtml`
- Modify: `cmd/web/handler-exerciseset_test.go`

- [ ] **Step 1: Write a failing structural test**

In `cmd/web/handler-exerciseset_test.go`, find the existing test that exercises the `/swap` page rendering (the test containing the block around line 438: `client.GetDoc(ctx, slotURL+"/swap")` followed by ranging over `.exercise-option .exercise-name`). After its existing assertions and before the function's closing brace, add:

```go
	// Lock in the new swap-card structure.
	swapCard := doc.Find(".exercise-option").First()
	if swapCard.Length() == 0 {
		t.Fatal("expected at least one .exercise-option on the swap page")
	}
	if swapCard.Find(".badge").Length() == 0 {
		t.Error("expected category badge inside the swap card")
	}
	if swapCard.Find(".muscle-chip.muscle-chip--primary").Length() == 0 {
		t.Error("expected at least one .muscle-chip--primary in the swap card")
	}
	if swapCard.Find(".actions .btn.btn--quiet[type='submit']").Length() == 0 {
		t.Error("expected the primary Swap action as .btn.btn--quiet submit inside .actions")
	}
	if swapCard.Find(".actions .btn.btn--ghost.btn--sm").Length() == 0 {
		t.Error("expected the secondary Info action as .btn.btn--ghost.btn--sm inside .actions")
	}
	if swapCard.Find("dialog.sheet-dialog").Length() == 0 {
		t.Error("expected the per-card info dialog as dialog.sheet-dialog")
	}
```

- [ ] **Step 2: Run the test to verify it fails**

```
go test -v ./cmd/web -run TestExerciseSet
```

(Or the exact test name from the surrounding `func TestXxx` — `grep -n "^func Test" cmd/web/handler-exerciseset_test.go` if needed.)

Expected: FAIL on the new assertions.

- [ ] **Step 3: Rewrite the card markup**

Open `ui/templates/pages/exercise-swap/exercise-swap.gohtml`. **Replace the entire `<section class="alternative-exercises">` block** (currently lines 82-208) with:

```gohtml
        <section class="alternative-exercises">
            <style {{ nonce }}>
                @scope (.alternative-exercises) {
                    :scope {
                        h2 {
                            font-size: var(--font-size-2);
                            margin-bottom: var(--size-3);
                        }

                        .alternatives-list {
                            gap: var(--size-3);
                        }

                        .exercise-option {
                            gap: var(--size-3);
                        }

                        .exercise-name {
                            font-weight: var(--font-weight-6);
                        }

                        .actions {
                            align-items: stretch;
                        }

                        .actions .swap-form {
                            flex: 1;
                        }

                        .no-results {
                            color: var(--color-text-secondary);
                            padding: var(--size-4) 0;
                            text-align: center;
                        }
                    }
                }
            </style>
            <h2>Choose Alternative Exercise</h2>

            <div class="alternatives-list stack">
                {{ if .CompatibleExercises }}
                {{ range .CompatibleExercises }}
                    <div class="exercise-option card stack">
                        <div class="exercise-name">{{ .Name }}</div>
                        <div class="meta cluster">
                            <span class="badge">{{ .Category.Label }}</span>
                            {{ range .PrimaryMuscleGroups }}
                                <span class="muscle-chip muscle-chip--primary">{{ . }}</span>
                            {{ end }}
                        </div>
                        <div class="actions cluster">
                            <form method="post"
                                  action="/workouts/{{ $.Date.Format "2006-01-02" }}/exercises/{{ $.WorkoutExerciseID }}/swap"
                                  class="swap-form">
                                <input type="hidden" name="new_exercise_id" value="{{ .ID }}">
                                <button type="submit" class="btn btn--quiet btn--block">Swap to this exercise</button>
                            </form>
                            <button type="button" class="btn btn--ghost btn--sm info-trigger">
                                <script {{ nonce }}>
                                  me().addEventListener('click', () => {
                                    document.getElementById('dialog-exercise-{{ .ID }}').showModal()
                                  })
                                </script>
                                Info
                            </button>
                        </div>
                        <dialog id="dialog-exercise-{{ .ID }}" class="sheet-dialog">
                            <form method="dialog" class="sheet-dialog__close">
                                <button class="btn btn--ghost btn--sm">Close</button>
                            </form>
                            {{ mdToHTML .DescriptionMarkdown }}
                        </dialog>
                    </div>
                {{ end }}
                {{ else }}
                    <p class="no-results">
                        {{ if .Query }}No exercises match &ldquo;{{ .Query }}&rdquo;.{{ else }}No alternative exercises available.{{ end }}
                    </p>
                {{ end }}
            </div>
        </section>
```

The existing `.current-exercise` section above (lines 17-54) is **unchanged** — it's the "Current Exercise" summary at the top of the swap page and not part of this redesign.

- [ ] **Step 4: Run the test to verify it passes**

```
go test -v ./cmd/web -run TestExerciseSet
```

Expected: PASS.

- [ ] **Step 5: Run the full test suite**

```
make test
```

Expected: PASS — including the ordering test around line 711 that ranges over `.exercise-option` and reads `input[name='new_exercise_id']`.

- [ ] **Step 6: Lint**

```
make lint-fix
```

Expected: clean.

- [ ] **Step 7: Manual smoke**

Open `/workouts/<today>/exercises/<id>/swap` in a narrow browser. Verify:

- "Current Exercise" summary unchanged at the top.
- Alternative cards match the visual style from Task 4.
- The "Swap to this exercise" button dominates; Info button sits compact on the right.
- Info sheet slides up; Close dismisses.

- [ ] **Step 8: Commit**

```bash
git add ui/templates/pages/exercise-swap/exercise-swap.gohtml cmd/web/handler-exerciseset_test.go
git commit -m "Redesign swap-exercise card

Mirror of the add-exercise redesign: category + muscle chips, primary
.btn--quiet alongside a secondary .btn--ghost--sm Info in a cluster, and
the global .sheet-dialog class for the per-card info modal."
```

---

### Task 6: Document the two new component classes

**Files:**
- Modify: `ui/templates/CLAUDE.md`

- [ ] **Step 1: Add the new classes to the component catalog**

Open `ui/templates/CLAUDE.md`. In the "Current components" section, find the "**Class-components** (`main.css @layer components`):" bullet list (the one containing `.btn`/`button`, `.badge`, `.card`). Add two new bullets after `.card`:

```markdown
- `.muscle-chip` (+ `--primary`) — pill for muscle-group labels on the
  exercise-info / -add / -swap pages. `--primary` uses the Clay accent
  for primary-muscle emphasis; the base uses Stone for secondary muscles.
- `.sheet-dialog` (+ `__close` row) — slide-up modal sheet used for
  per-card exercise info on the Add and Swap pages. The companion
  `.sheet-dialog__close` element pins the close button to the top-right.
```

- [ ] **Step 2: Verify no test or lint regressions (docs-only change, should be no-op)**

```
make test && make lint-fix
```

Expected: PASS, clean.

- [ ] **Step 3: Commit**

```bash
git add ui/templates/CLAUDE.md
git commit -m "Document .muscle-chip and .sheet-dialog class-components

Add the two new global classes introduced by the exercise add/swap card
redesign to the components catalog in ui/templates/CLAUDE.md."
```

---

## Final verification

After all tasks land, run the full CI pipeline once locally:

```
make ci
```

Expected: PASS end to end.

Manual review on a narrow viewport: open `/dev/styleguide`, scroll to the new "Muscle chip" and "Sheet dialog" sections, then open both `/workouts/<today>/add-exercise` and `/workouts/<today>/exercises/<id>/swap` and confirm the card hierarchy reads as intended (primary green action dominant, Info quietly available, metadata legible as chips).
