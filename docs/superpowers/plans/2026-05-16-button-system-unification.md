# Button System Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace ~15 bespoke `.{x}-button` classes across the app with a small semantic-variant system on `.btn`, fix the preferences-page contrast bug, and add a `:disabled` state + 44 px touch target everywhere.

**Architecture:** Add 5 variants (`primary` default / `--quiet` / `--ghost` / `--danger` / `--focus`) and 2 modifiers (`--sm` / `--block`) to the existing `.btn` rule in `main.css` `@layer components`. Then per page, delete the bespoke class from the colocated `<style>` block and switch the markup to `.btn .btn--variant`. Layout-only rules (margin, alignment) stay in the page block.

**Tech Stack:** Go html/template, plain CSS with `@layer components`/`@scope`, goquery-driven handler tests via `internal/e2etest`. No build step — templates and static assets load from the filesystem at runtime.

**Spec:** [`docs/superpowers/specs/2026-05-16-button-system-unification-design.md`](../specs/2026-05-16-button-system-unification-design.md)

---

## File Structure

**Modified files:**

| File | What changes | Why |
|---|---|---|
| `ui/static/main.css` | Add 5 variants, 2 modifiers, `:disabled` rule. Update base `.btn` `min-height` (2.75 rem) + `border-radius` (`--radius-2`) | Single source of truth for button look |
| `ui/templates/pages/styleguide/styleguide.gohtml` | Replace the 3-button "Buttons" block with a full variant catalog | Living catalog — every variant visible & testable |
| `cmd/web/handler-styleguide_test.go` | Extend assertions to cover the new variant headings | Lock the catalog into the test |
| `ui/templates/pages/preferences/preferences.gohtml` | Remove 6 bespoke classes + their CSS, switch markup to `.btn .btn--variant`. Fix the `select` focus colour | The page the user pointed at |
| `ui/templates/pages/exerciseset/sets-container.gohtml` | Replace `.submit-button` and `.edit-button` | Workout-flow CTAs |
| `cmd/web/handler-exerciseset_test.go` | Update edit-link selector | `.edit-button` class goes away |
| `ui/templates/pages/exerciseset/warmup.gohtml` | Replace `.warmup-complete-button` with `.btn .btn--focus .btn--block` (ember) | Unify with in-set CTA |
| `ui/templates/pages/exercise-add/exercise-add.gohtml` | Replace `.add-button` | List-item action |
| `ui/templates/pages/exercise-swap/exercise-swap.gohtml` | Replace `.swap-button` | List-item action |
| `ui/templates/pages/workout/workout.gohtml` | Replace `.add-exercise-button` | Quiet anchor |
| `cmd/web/playwright_test.go` | Update `a.add-exercise-button` selector (2 sites) | Class goes away |
| `ui/templates/pages/home/schedule.gohtml` | Replace `.menu-button a` styling | Header chip |
| `ui/templates/pages/schedule/schedule.gohtml` | Replace `.save-button` + align `select` focus colour | Setup page CTA |
| `ui/templates/pages/not-found/not-found.gohtml` | Replace `.home-link`/`.back-link` | 404 actions |
| `ui/templates/pages/error/error.gohtml` | Replace `.home-link` | Error actions |
| `ui/templates/pages/workout-not-found/workout-not-found.gohtml` | Replace `.primary-button` | 404 action |
| `ui/templates/CLAUDE.md` | Update the "Current components" list to mention the new variants | Doc convention |

**Created files:** none.

**No changes:** Go handler signatures, routing, services, domain types, the base `aria-busy` loading state in `main.js`, the stack navigator, the `.signal-btn` segmented control, the `.timed-runner-cancel` micro-link.

---

## Task 1 — Extend `main.css` with the variant system

**Files:**
- Modify: `ui/static/main.css` (the `button, .btn { … }` rule inside `@layer components`, currently at lines 265–329)

- [ ] **Step 1: Read the current `.btn` rule**

Read `ui/static/main.css` lines 260–330 so you understand the existing rule, its hover/active/focus/aria-busy behaviour, and the `@layer components` boundary you're adding inside. **Don't** remove the existing transition / hover / focus-visible / aria-busy / active-press rules — only add to them.

- [ ] **Step 2: Bump base `.btn` `min-height` and `border-radius`**

Edit the existing `button, .btn { … }` declaration to add two new properties **and** change the `border-radius`. Find this:

```css
button, .btn {
    position: relative;
    display: inline-flex;
    border-radius: var(--radius-3);
    background-color: var(--clay-4);
    color: var(--color-surface-elevated);
    font-family: var(--font-sans);
    font-size: var(--font-size-1);
    font-weight: var(--font-weight-7);
    letter-spacing: var(--font-letterspacing-3);
    --vertical-padding: 2.5em;
    padding: var(--size-2) var(--vertical-padding);
    border: none;
    white-space: nowrap;
    text-decoration: none;
    transition: background-color var(--duration-2) var(--ease-out-quiet), transform var(--duration-1) var(--ease-out-quiet);
```

Change `border-radius: var(--radius-3);` to `border-radius: var(--radius-2);` and add `min-height: 2.75rem;` and `align-items: center;` and `justify-content: center;` to the same block (so anchors-as-buttons centre their text and the touch target reaches 44 px regardless of label length). The block now begins:

```css
button, .btn {
    position: relative;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-height: 2.75rem;
    border-radius: var(--radius-2);
    background-color: var(--clay-4);
    /* … rest unchanged … */
```

- [ ] **Step 3: Add the `:disabled` rule**

Inside `@layer components`, immediately after the existing `button, .btn { … }` block (before the `.sr-only` rule), add:

```css
button:disabled,
.btn:disabled,
.btn[aria-disabled="true"] {
    opacity: 0.55;
    cursor: not-allowed;
}

button:disabled:hover,
.btn:disabled:hover,
.btn[aria-disabled="true"]:hover {
    background-color: var(--clay-4);
    transform: none;
}
```

- [ ] **Step 4: Add the five variant rules**

Immediately after the `:disabled` rule (still inside `@layer components`), add:

```css
.btn--quiet {
    background-color: var(--color-success-bg);
    color: var(--color-success);
}

.btn--quiet:hover {
    background-color: color-mix(in oklab, var(--color-success-bg) 92%, black);
}

.btn--ghost {
    background-color: var(--color-surface-elevated);
    color: var(--color-text-primary);
    border: var(--border-size-1) solid var(--color-border);
}

.btn--ghost:hover {
    background-color: var(--stone-1);
    border-color: var(--stone-4);
}

.btn--danger {
    background-color: var(--color-error);
    color: var(--color-surface-elevated);
}

.btn--danger:hover {
    background-color: color-mix(in oklab, var(--color-error) 88%, black);
}

.btn--danger:active {
    background-color: color-mix(in oklab, var(--color-error) 78%, black);
}

.btn--focus {
    background-color: var(--ember);
    color: var(--stone-10);
    font-weight: var(--font-weight-8);
    text-transform: uppercase;
}

.btn--focus:hover {
    background-color: color-mix(in oklab, var(--ember) 90%, white);
}

.btn--focus:active {
    background-color: color-mix(in oklab, var(--ember) 85%, black);
}

.btn--focus:focus-visible {
    outline: 3px solid var(--ember);
}
```

- [ ] **Step 5: Add the two modifier rules**

Immediately after the variant rules, add:

```css
.btn--block {
    width: 100%;
}

.btn--sm {
    min-height: 2rem;
    padding: var(--size-1) var(--size-3);
    font-size: var(--font-size-0);
    font-weight: var(--font-weight-5);
    letter-spacing: var(--font-letterspacing-1);
    text-transform: none;
}
```

- [ ] **Step 6: Run the full test suite**

Run: `make test`
Expected: PASS. No existing handler test asserts the *visual* rules, so they should all continue to pass — the only failure mode here is a CSS syntax error breaking template parse (which we'd see as a panic).

- [ ] **Step 7: Manually verify in browser**

Run: `make init && go run ./cmd/web/` (or whatever the project's dev command is — check the `Makefile`'s default target if unsure). Open `http://localhost:8080/dev/styleguide`. The existing "Buttons" section (Default / Anchor as button / Loading) should render — the Default button is now slightly more rounded (it picked up `--radius-2`) and a touch taller. No layout breakage anywhere.

- [ ] **Step 8: Commit**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Add semantic button variants + :disabled state to main.css

Introduces .btn--quiet/--ghost/--danger/--focus variants plus --sm/--block
modifiers that compose with any variant. Bumps base .btn to a 44 px min
touch target, normalises border-radius to --radius-2, and adds an opacity
+ no-op-hover :disabled rule. Per-page bespoke button classes (preferences,
sets-container, warmup, …) are still in place; the next commits delete them
and switch markup to .btn .btn--variant.
EOF
)"
```

---

## Task 2 — Extend `/dev/styleguide` with a "Button variants" catalog

**Files:**
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml` (the existing "Buttons" `<h3>` block under the `<h2>Components</h2>` section)
- Modify: `cmd/web/handler-styleguide_test.go` (add new assertions)

- [ ] **Step 1: Write failing test assertions first**

Open `cmd/web/handler-styleguide_test.go`. Find the existing `Test_application_styleguide` function. Immediately before its closing `}`, append:

```go
	// Button variants — every variant + modifier renders on the styleguide.
	if doc.Find("h3:contains('Button variants')").Length() == 0 {
		t.Error("expected a 'Button variants' section on the styleguide")
	}
	for _, cls := range []string{
		".btn",
		".btn.btn--quiet",
		".btn.btn--ghost",
		".btn.btn--danger",
		".btn.btn--focus",
		".btn.btn--sm",
		".btn.btn--block",
	} {
		if doc.Find(cls).Length() == 0 {
			t.Errorf("expected a %s example on the styleguide", cls)
		}
	}
	// Disabled and aria-busy examples too, so the rule visibly covers them.
	if doc.Find(".btn:disabled, button.btn[disabled]").Length() == 0 {
		t.Error("expected a disabled button example on the styleguide")
	}
	if doc.Find(`.btn[aria-busy="true"]`).Length() == 0 {
		t.Error("expected an aria-busy button example on the styleguide")
	}
```

- [ ] **Step 2: Run the test, see it fail**

Run: `go test -v ./cmd/web/ -run Test_application_styleguide`
Expected: FAIL. Multiple `expected a .btn.btn--quiet example` etc. errors.

- [ ] **Step 3: Replace the styleguide "Buttons" block with the full catalog**

Open `ui/templates/pages/styleguide/styleguide.gohtml`. Find the block at lines ~399–404:

```gohtml
            <h3>Buttons</h3>
            <div class="component-sample">
                <button type="button">Default</button>
                <a class="btn" href="#">Anchor as button</a>
                <button type="button" aria-busy="true">Loading</button>
            </div>
```

Replace it with:

```gohtml
            <h3>Button variants</h3>
            <div class="component-sample">
                <button type="button" class="btn">Primary</button>
                <button type="button" class="btn btn--quiet">Quiet</button>
                <button type="button" class="btn btn--ghost">Ghost</button>
                <button type="button" class="btn btn--danger">Danger</button>
                <button type="button" class="btn btn--focus">Focus</button>
                <a class="btn" href="#">Anchor as primary</a>
            </div>

            <h3>Button modifiers</h3>
            <div class="component-sample">
                <button type="button" class="btn btn--sm">Small primary</button>
                <button type="button" class="btn btn--ghost btn--sm">Small ghost</button>
                <button type="button" class="btn btn--danger btn--sm">Small danger</button>
            </div>
            <div class="component-sample">
                <button type="button" class="btn btn--block">Block primary</button>
            </div>

            <h3>Button states</h3>
            <div class="component-sample">
                <button type="button" class="btn" disabled>Disabled</button>
                <button type="button" class="btn btn--danger" disabled>Disabled danger</button>
                <button type="button" class="btn" aria-busy="true">Loading</button>
            </div>
```

- [ ] **Step 4: Run the test, see it pass**

Run: `go test -v ./cmd/web/ -run Test_application_styleguide`
Expected: PASS.

- [ ] **Step 5: Visually verify**

Reload `http://localhost:8080/dev/styleguide` in the browser. Scroll to the new Button-variants, Button-modifiers, and Button-states sections. Confirm:
- Five distinct primary/quiet/ghost/danger/focus appearances.
- Small variants are visibly smaller (≈ 32 px tall) but still readable.
- The block button stretches to the column width.
- The disabled buttons read as muted (≈ 55 % opacity) and don't change on hover.
- The aria-busy button shows the spinner.

- [ ] **Step 6: Run `make lint-fix` + `make test` for the package**

Run: `make lint-fix && go test ./cmd/web/`
Expected: PASS, no lint changes that need review.

- [ ] **Step 7: Commit**

```bash
git add ui/templates/pages/styleguide/styleguide.gohtml cmd/web/handler-styleguide_test.go
git commit -m "$(cat <<'EOF'
Add button variants & states to /dev/styleguide

Replaces the three-button placeholder with the full catalog: every variant,
every modifier, plus :disabled and aria-busy states. Handler test now
asserts each variant/modifier renders so the catalog can't silently drift
out of sync with main.css.
EOF
)"
```

---

## Task 3 — Migrate the preferences page

**Files:**
- Modify: `ui/templates/pages/preferences/preferences.gohtml`

This is the largest single migration because the page has six bespoke button classes and the user-reported visual issues. Work the page in three commits so each is small and reversible.

### Task 3a — Schedule form, Export, Delete, Logout buttons

- [ ] **Step 1: Switch the `Save Schedule` markup**

In `preferences.gohtml`, find line 230:

```gohtml
            <button type="submit" class="save-button">Save Schedule</button>
```

Replace with:

```gohtml
            <button type="submit" class="btn btn--block">Save Schedule</button>
```

- [ ] **Step 2: Switch the `Download My Data` anchor**

Find line 572:

```gohtml
            <a href="/preferences/export-data" class="export-button" download>
                Download My Data
            </a>
```

Replace with:

```gohtml
            <a href="/preferences/export-data" class="btn" download>
                Download My Data
            </a>
```

- [ ] **Step 3: Switch the `Delete my data` button**

Find line 582:

```gohtml
                <button type="submit" class="delete-button">
                    Delete my data
                </button>
```

Replace with:

```gohtml
                <button type="submit" class="btn btn--danger">
                    Delete my data
                </button>
```

- [ ] **Step 4: Switch the `Log out` button**

Find line 590:

```gohtml
                <button type="submit" class="logout-button">
                    Log out
                </button>
```

Replace with:

```gohtml
                <button type="submit" class="btn btn--ghost btn--sm">
                    Log out
                </button>
```

- [ ] **Step 5: Delete the four bespoke CSS rules**

Still in `preferences.gohtml`, in the top `<style>` block (lines 5–202), delete the entire rules for `.save-button` (lines 74–93), `.export-button` (lines 140–158), `.delete-button` (lines 182–200), and `.logout-button` (lines 103–118). Leave `.footer-section` (lines 95–101), `.data-export-section` layout-only rules (lines 120–138), `.danger-zone` layout rules (lines 160–180), and the form/fieldset/weekday/select rules alone.

For `.data-export-section` keep only its layout properties — remove `.data-export-section h2` and `.data-export-section p` only if their values match the page defaults; if they tune typography (font-size/colour), leave them.

After this edit, `grep -n 'save-button\|export-button\|delete-button\|logout-button' ui/templates/pages/preferences/preferences.gohtml` must print nothing.

- [ ] **Step 6: Run the preferences test**

Run: `go test -v ./cmd/web/ -run 'Test_application_preferences|Test_application_deleteUser|Test_application_exportUserData'`
Expected: PASS. The test uses `button:contains('Delete my data')` and `.danger-zone` selectors which still work.

- [ ] **Step 7: Manually verify the page**

Reload `http://localhost:8080/preferences` and check:
- `Save Schedule` is a full-width clay-4 primary, same as before.
- `Download My Data` reads as a small clay primary (not a full-width chunk).
- `Delete my data` is a red-filled button, weight 7, not the old weight-5.
- `Log out` is a small ghost chip in the footer.
- Keyboard tab order: every focused element shows the same clay-3 outline.

- [ ] **Step 8: Commit**

```bash
git add ui/templates/pages/preferences/preferences.gohtml
git commit -m "$(cat <<'EOF'
Switch preferences Save/Export/Delete/Logout to .btn variants

Removes the four bespoke .{save,export,delete,logout}-button classes from
preferences.gohtml and uses the centralised .btn variants instead. No
visual change to Save Schedule or Delete (which already mirrored the
primary/danger colours); Download My Data and Log out pick up the
consistent shape + 44 px touch target as a side effect.
EOF
)"
```

### Task 3b — Rest notifications buttons

- [ ] **Step 1: Switch the enable button**

In `preferences.gohtml`, find line 328:

```gohtml
                    <button type="button" class="push-button enable-btn hidden">Enable rest notifications</button>
```

Replace with:

```gohtml
                    <button type="button" class="btn btn--ghost enable-btn hidden">Enable rest notifications</button>
```

- [ ] **Step 2: Switch the disable button**

Find line 329:

```gohtml
                    <button type="button" class="push-button danger disable-btn hidden">Disable on this device</button>
```

Replace with:

```gohtml
                    <button type="button" class="btn btn--danger btn--sm disable-btn hidden">Disable on this device</button>
```

- [ ] **Step 3: Delete the bespoke `.push-button` rules**

In the `@scope (.rest-notifications-section)` block (around lines 234–303), delete the rules for `.push-button` (lines 280–287) and `.push-button.danger` (lines 289–292). Leave `:scope`, `h2`, `.status`, `.install-steps`, `.toggle-row`, `.error`, `.hidden` alone — those are layout/state rules, not button look.

After this edit, `grep -n 'push-button' ui/templates/pages/preferences/preferences.gohtml` must print nothing.

- [ ] **Step 4: Verify the JS still finds the buttons**

The page's JS uses `root.querySelector('.enable-btn')` and `.disable-btn` (around lines 375–376). The marker classes `enable-btn` / `disable-btn` survived the markup edit, so the JS keeps working. Read lines 370–380 to confirm those selectors are unchanged.

- [ ] **Step 5: Run the preferences test**

Run: `go test -v ./cmd/web/ -run TestPreferences`
Expected: PASS.

- [ ] **Step 6: Manually verify**

Reload `/preferences` on a device or simulator where the rest-notifications card is visible (the screenshot showed a desktop view with the card visible because the user-agent was iOS or a PWA install). Confirm:
- `Enable rest notifications` is now a *visible* ghost button with a stone-0 background that pops off the page stone-1 background — no longer blending in.
- `Disable on this device` is a small red-filled chip — unambiguously destructive.
- Clicking Enable still triggers the permission prompt (JS still finds the button via `.enable-btn`).

- [ ] **Step 7: Commit**

```bash
git add ui/templates/pages/preferences/preferences.gohtml
git commit -m "$(cat <<'EOF'
Fix rest-notifications buttons blending into the preferences page

The .push-button class used background: var(--color-surface), which equals
--stone-1, the same as the page background — the button was nearly
invisible (see user-supplied screenshot). Switches the enable button to
.btn .btn--ghost (surface-elevated background, real border) and the
disable button to .btn .btn--danger .btn--sm (a small red chip that reads
as destructive without dominating the card).
EOF
)"
```

### Task 3c — Deload section + select focus colour

- [ ] **Step 1: Switch the deload `Save` button to explicit primary block**

In `preferences.gohtml`, find line 554:

```gohtml
                <button type="submit">Save deload settings</button>
```

Replace with:

```gohtml
                <button type="submit" class="btn btn--block">Save deload settings</button>
```

- [ ] **Step 2: Switch the `Restart cycle` button to ghost**

Find line 562:

```gohtml
                <button type="submit" {{ if not .DeloadEnabled }}disabled{{ end }}>
                    Restart cycle from next Monday
                </button>
```

Replace with:

```gohtml
                <button type="submit" class="btn btn--ghost" {{ if not .DeloadEnabled }}disabled{{ end }}>
                    Restart cycle from next Monday
                </button>
```

The `disabled` attribute now picks up the new `.btn:disabled` opacity/cursor rule from Task 1 — the button visibly looks disabled when planned deloads are off.

- [ ] **Step 3: Align the preference-page `select` focus colour with buttons**

The schedule form's `select` focuses green; everything else focuses clay. Find the `select { … &:focus { outline: 2px solid var(--color-success); … } }` rule inside `.weekday-item .workout-duration` (lines 56–70). Change the `:focus` outline from `var(--color-success)` to `var(--color-border-focus)` so it matches the global button focus colour:

```css
                        &:focus {
                            outline: 2px solid var(--color-border-focus);
                            outline-offset: 2px;
                        }
```

- [ ] **Step 4: Run the preferences test**

Run: `go test -v ./cmd/web/ -run TestPreferences`
Expected: PASS.

- [ ] **Step 5: Manually verify**

Reload `/preferences`:
- `Save deload settings` now matches `Save Schedule` in size and shape (both `.btn .btn--block`).
- `Restart cycle from next Monday` is a ghost button (white background, border). When the `Enable planned deloads` checkbox is **unchecked**, the button visibly fades to ≈ 55 % opacity and the cursor goes `not-allowed`.
- Tab into any weekday `select` — its outline is now clay-3, matching every button on the page.

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/preferences/preferences.gohtml
git commit -m "$(cat <<'EOF'
Unify deload buttons + select focus colour on preferences

Save deload settings and Restart cycle were rendering with the bare-button
default style — a different shape and weight from the .btn .btn--block
Save Schedule sitting right above them. Both now declare their intent
explicitly: Save = block primary, Restart = ghost (and the disabled state
is finally visible thanks to the new .btn:disabled rule).

The weekday-duration select also no longer focuses green while every
button on the page focuses clay — one focus colour everywhere.
EOF
)"
```

---

## Task 4 — Migrate `sets-container.gohtml` (submit-button, edit-button)

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`
- Modify: `cmd/web/handler-exerciseset_test.go` (line 196 selector)

- [ ] **Step 1: Switch the three `submit-button` call sites**

In `sets-container.gohtml`, find the three lines that read:

```gohtml
                                <button type="submit" class="submit-button" aria-label="Complete set">Done!</button>
```

(at line 545, 797, 835 — they are identical). Replace each with:

```gohtml
                                <button type="submit" class="btn btn--focus btn--block" aria-label="Complete set">Done!</button>
```

- [ ] **Step 2: Switch the `Start timer` button**

Find line 573:

```gohtml
                        <button type="button" class="submit-button timed-runner-start"
                                data-timed-runner-start hidden>Start timer</button>
```

Replace with:

```gohtml
                        <button type="button" class="btn btn--focus btn--block timed-runner-start"
                                data-timed-runner-start hidden>Start timer</button>
```

The marker class `timed-runner-start` is kept so the JS keeps finding the button.

- [ ] **Step 3: Switch the two `edit-button` anchors**

Find lines 472 and 484:

```gohtml
                            <a href="?edit={{ $index }}" class="edit-button" aria-label="Edit set {{ $setDisplay.Number }}">Edit</a>
```

Replace each with:

```gohtml
                            <a href="?edit={{ $index }}" class="btn btn--ghost btn--sm edit-link" aria-label="Edit set {{ $setDisplay.Number }}">Edit</a>
```

The new `edit-link` marker class lets the test target the edit anchor without depending on the visual class.

- [ ] **Step 4: Delete the bespoke `.submit-button` and `.edit-button` CSS rules**

In the page's `<style>` blocks, find `.submit-button { … }` (around line 349) and `.edit-button { … }` (around line 125). Delete both rule bodies entirely (including their hover/active/focus pseudo-rules).

After this edit:

```bash
grep -n 'submit-button\|edit-button' ui/templates/pages/exerciseset/sets-container.gohtml
```

must print nothing.

- [ ] **Step 5: Update the failing test selector**

Open `cmd/web/handler-exerciseset_test.go` line 196. Change:

```go
	editLink := doc.Find(".exercise-set.completed .edit-button").First()
```

to:

```go
	editLink := doc.Find(".exercise-set.completed .edit-link").First()
```

- [ ] **Step 6: Run the exerciseset tests**

Run: `go test ./cmd/web/`
Expected: PASS.

- [ ] **Step 7: Manually verify**

Open `http://localhost:8080/workouts/<today>/exercises/<first-exercise-id>` (start a workout if needed). Confirm:
- `Done!` is a full-width ember button, same look as before (since `.submit-button` already used ember + uppercase).
- `Start timer` on a time-based set is the same ember full-width button.
- `Edit` anchors on completed sets are small ghost chips in the top-right of each row.

- [ ] **Step 8: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml cmd/web/handler-exerciseset_test.go
git commit -m "$(cat <<'EOF'
Migrate sets-container Done/Start-timer/Edit to .btn variants

Replaces .submit-button (Done!, Start timer — ember focus-mode CTA) with
.btn .btn--focus .btn--block and .edit-button with .btn .btn--ghost
.btn--sm + a stable edit-link marker class for the test selector. Drops
both bespoke rules from the page's <style> block.
EOF
)"
```

---

## Task 5 — Migrate `warmup.gohtml` (unify to ember)

**Files:**
- Modify: `ui/templates/pages/exerciseset/warmup.gohtml`

- [ ] **Step 1: Switch the warmup-complete button**

In `warmup.gohtml`, find line 97:

```gohtml
                <button type="submit" class="warmup-complete-button">
                    ✓ Mark Warmup Complete
                </button>
```

Replace with:

```gohtml
                <button type="submit" class="btn btn--focus btn--block">
                    ✓ Mark Warmup Complete
                </button>
```

- [ ] **Step 2: Delete the bespoke rule**

In the `<style>` block, find `.warmup-complete-button { … }` (lines 30–55) and delete the entire rule including its `:hover`, `:active`, and `:focus-visible` children. The surrounding `.warmup-banner`, `.warmup-message`, `.warmup-description`, and `.warmup-status` rules stay.

After this edit:

```bash
grep -n 'warmup-complete-button' ui/templates/pages/exerciseset/warmup.gohtml
```

must print nothing.

- [ ] **Step 3: Run the exerciseset tests (warmup is exercised by them)**

Run: `go test ./cmd/web/`
Expected: PASS. The warmup test selector in `handler-exerciseset_test.go` uses `button:contains('Mark Warmup Complete')` — text-based, resilient.

- [ ] **Step 4: Manually verify**

Open the workout-set page for any exercise on a day where the warmup hasn't been completed yet. The button is now ember (orange), uppercase, full-width — matching the in-set `Done!` styling. The blue `.warmup-banner` background around it stays blue (it's a region styling, not the button).

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exerciseset/warmup.gohtml
git commit -m "$(cat <<'EOF'
Unify warmup CTA with in-set CTA — both ember .btn--focus

The warmup-complete button used --color-info blue, while the in-set Done!
used --ember. Switching to .btn .btn--focus .btn--block gives the workout
flow one unmistakable CTA colour (per the design spec). The blue warmup
region background stays — only the button colour changes.
EOF
)"
```

---

## Task 6 — Migrate `exercise-add.gohtml`

**Files:**
- Modify: `ui/templates/pages/exercise-add/exercise-add.gohtml`

- [ ] **Step 1: Switch the add button**

In `exercise-add.gohtml`, find line 109:

```gohtml
                            <button type="submit" class="add-button">Add this exercise</button>
```

Replace with:

```gohtml
                            <button type="submit" class="btn btn--quiet btn--block">Add this exercise</button>
```

- [ ] **Step 2: Delete the bespoke rule**

In the page's `<style>` block, find `.add-button { … }` (the rule from "background-color: var(--color-success-bg)" through its `:hover` close brace). Delete the entire rule.

After this edit:

```bash
grep -n 'add-button' ui/templates/pages/exercise-add/exercise-add.gohtml
```

must print nothing.

- [ ] **Step 3: Run the relevant test**

Run: `go test ./cmd/web/`
Expected: PASS. The relevant tests are `Test_application_workoutAddExercise_search_filters_by_name` in `handler-workout_test.go`.

- [ ] **Step 4: Manually verify**

Open the add-exercise page (workout overview → Add an exercise). Confirm each result row's `Add this exercise` button is a full-width pale-green pill (same colour as before; the shape and 44 px height come from `.btn`).

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exercise-add/exercise-add.gohtml
git commit -m "$(cat <<'EOF'
Migrate exercise-add Add-this-exercise to .btn .btn--quiet

The bespoke .add-button was visually identical to .swap-button on the
exercise-swap page — both are list-item actions on a search result. Both
become .btn .btn--quiet .btn--block in the central system.
EOF
)"
```

---

## Task 7 — Migrate `exercise-swap.gohtml`

**Files:**
- Modify: `ui/templates/pages/exercise-swap/exercise-swap.gohtml`

- [ ] **Step 1: Switch the swap button**

In `exercise-swap.gohtml`, find line 149:

```gohtml
                            <button type="submit" class="swap-button">Swap to this exercise</button>
```

Replace with:

```gohtml
                            <button type="submit" class="btn btn--quiet btn--block">Swap to this exercise</button>
```

- [ ] **Step 2: Delete the bespoke rule**

In the page's `<style>` block, find `.swap-button { … }` and delete the entire rule (including `:hover`).

After this edit:

```bash
grep -n 'swap-button' ui/templates/pages/exercise-swap/exercise-swap.gohtml
```

must print nothing.

- [ ] **Step 3: Run the swap test**

Run: `go test ./cmd/web/`
Expected: PASS. Relevant tests are `Test_application_workoutSwapExercise_*` in `handler-exerciseset_test.go`.

- [ ] **Step 4: Manually verify**

Open a swap page (workout overview → tap an exercise → Swap). Each result row's `Swap to this exercise` button is full-width pale-green, matching the add-exercise page's look.

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exercise-swap/exercise-swap.gohtml
git commit -m "$(cat <<'EOF'
Migrate exercise-swap Swap-to-this to .btn .btn--quiet

Symmetric with exercise-add — both list-item actions now share one rule.
EOF
)"
```

---

## Task 8 — Migrate `workout.gohtml` (add-exercise-button) + playwright test

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml`
- Modify: `cmd/web/playwright_test.go` (lines 638, 728)

- [ ] **Step 1: Switch the add-exercise anchor**

In `workout.gohtml`, find line 148:

```gohtml
                <a href="/workouts/{{ .Date.Format "2006-01-02" }}/add-exercise" class="add-exercise-button">
```

Replace the class with the variant and a stable marker:

```gohtml
                <a href="/workouts/{{ .Date.Format "2006-01-02" }}/add-exercise" class="btn btn--quiet add-exercise-link">
```

- [ ] **Step 2: Delete the bespoke rule**

In the page's `<style>` block (around lines 110–125), delete the `.add-exercise-button { … }` and `.add-exercise-button:hover { … }` rules.

After this edit:

```bash
grep -n 'add-exercise-button' ui/templates/pages/workout/workout.gohtml
```

must print nothing.

- [ ] **Step 3: Update the two playwright selectors**

Open `cmd/web/playwright_test.go`. At line 638:

```go
	addExerciseLink := page.Locator("a.add-exercise-button")
```

Change to:

```go
	addExerciseLink := page.Locator("a.add-exercise-link")
```

At line 728:

```go
	if err = page.Locator("a.add-exercise-button").Click(); err != nil {
```

Change to:

```go
	if err = page.Locator("a.add-exercise-link").Click(); err != nil {
```

- [ ] **Step 4: Run the workout test (the non-playwright one — playwright requires a browser binary)**

Run: `go test ./cmd/web/`
Expected: PASS. Relevant tests are `Test_application_addWorkout`, `Test_application_workoutAddExercise_*`, `Test_application_workoutNotFound`.

- [ ] **Step 5: Manually verify**

Open `http://localhost:8080/workouts/<today>`. The `Add an exercise` link is a small pale-green pill, same colour family as before but now consistent with the swap/add pages.

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/workout/workout.gohtml cmd/web/playwright_test.go
git commit -m "$(cat <<'EOF'
Migrate workout add-exercise anchor to .btn .btn--quiet

Replaces the bespoke .add-exercise-button (clay-1 chip) with the standard
quiet variant — visually the same family, but the shape and touch target
now match every other list-item action. The playwright selectors switch
to a stable .add-exercise-link marker class so they don't depend on the
visual classes.
EOF
)"
```

---

## Task 9 — Migrate `home/schedule.gohtml` (menu-button)

**Files:**
- Modify: `ui/templates/pages/home/schedule.gohtml`

- [ ] **Step 1: Switch the anchor markup**

In `home/schedule.gohtml`, find lines 50–54:

```gohtml
        <header class="menu-button">
            <a href="/preferences">
                Menu
            </a>
        </header>
```

Replace with:

```gohtml
        <header class="menu-button">
            <a href="/preferences" class="btn btn--ghost btn--sm">
                Menu
            </a>
        </header>
```

The `header.menu-button` wrapper keeps its layout role (`margin-left: auto`); the anchor picks up the central ghost-small look.

- [ ] **Step 2: Delete the bespoke anchor styles**

In the page's `<style>` block, delete three consecutive rules: `.menu-button a { … }` (lines 15–28), `.menu-button a:hover { … }` (lines 30–35), `.menu-button a:active { … }` (lines 37–40). Keep `.menu-button { margin-left: auto; }` (lines 11–13) — that's layout, not appearance.

After this edit, `grep -n '\.menu-button a' ui/templates/pages/home/schedule.gohtml` must print nothing.

- [ ] **Step 3: Run the home test**

Run: `go test -v ./cmd/web/ -run Test_application_home`
Expected: PASS.

- [ ] **Step 4: Manually verify**

Open `http://localhost:8080/`. The `Menu` link in the header keeps its position (still right-aligned) and now looks like a consistent ghost-small chip.

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/home/schedule.gohtml
git commit -m "$(cat <<'EOF'
Migrate home header Menu link to .btn .btn--ghost .btn--sm

Drops the bespoke .menu-button a appearance and uses the central ghost
variant. The header.menu-button wrapper keeps its layout role
(margin-left: auto) — only the anchor's visual style moves to the system.
EOF
)"
```

---

## Task 10 — Migrate `schedule/schedule.gohtml` (initial setup page)

**Files:**
- Modify: `ui/templates/pages/schedule/schedule.gohtml`

- [ ] **Step 1: Switch the save button**

In `schedule.gohtml`, find line 127:

```gohtml
            <button type="submit" class="save-button">Start Tracking</button>
```

Replace with:

```gohtml
            <button type="submit" class="btn btn--block">Start Tracking</button>
```

- [ ] **Step 2: Align the select focus colour with buttons**

Find the `select { … &:focus { outline: 2px solid var(--color-success); … } }` rule inside `.weekday-item .workout-duration` (lines 60–74). Change:

```css
                        &:focus {
                            outline: 2px solid var(--color-success);
                            outline-offset: 2px;
                        }
```

to:

```css
                        &:focus {
                            outline: 2px solid var(--color-border-focus);
                            outline-offset: 2px;
                        }
```

- [ ] **Step 3: Delete the bespoke `.save-button` rule**

In the page's `<style>` block, find `.save-button { … }` (lines 78–97) and delete the entire rule including `:hover` and `:active`.

After this edit, `grep -n 'save-button' ui/templates/pages/schedule/schedule.gohtml` must print nothing.

- [ ] **Step 4: Run the schedule tests**

Run: `go test ./cmd/web/`
Expected: PASS. Relevant test is `Test_application_schedulePOST_preservesRestNotificationsEnabled` in `handler-schedule_test.go`.

- [ ] **Step 5: Manually verify**

Open `/schedule` (you may need to use a new account or temporarily clear your weekly schedule). `Start Tracking` is full-width clay-4 primary, matching the preferences `Save Schedule`. Tab into a `select` — outline is clay-3.

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/schedule/schedule.gohtml
git commit -m "$(cat <<'EOF'
Migrate schedule setup Start-Tracking + select focus colour

Mirrors the preferences fix: .save-button → .btn .btn--block, and the
weekday-duration select focus colour aligns with buttons (clay-3 instead
of green).
EOF
)"
```

---

## Task 11 — Migrate `not-found`, `error`, `workout-not-found` pages

**Files:**
- Modify: `ui/templates/pages/not-found/not-found.gohtml`
- Modify: `ui/templates/pages/error/error.gohtml`
- Modify: `ui/templates/pages/workout-not-found/workout-not-found.gohtml`

- [ ] **Step 1: not-found — switch the two action buttons**

In `not-found.gohtml`, find lines 79–80:

```gohtml
                <a href="/" class="btn home-link">Go Home</a>
                <button type="button" class="btn back-link">
```

Replace with:

```gohtml
                <a href="/" class="btn">Go Home</a>
                <button type="button" class="btn btn--ghost">
```

Then in the page's `<style>` block (lines 50–66), delete the `.home-link { … }` and `.back-link { … }` rules entirely.

After this edit: `grep -n 'home-link\|back-link' ui/templates/pages/not-found/not-found.gohtml` must print nothing.

- [ ] **Step 2: error.gohtml — switch the Retry button + Go Home link**

In `error.gohtml`, find lines 59–67:

```gohtml
                <button type="button">
                    <span>Retry</span>
                    <script {{ nonce }}>
                      document.currentScript.parentElement.addEventListener('click', function () {
                        location.reload()
                      })
                    </script>
                </button>
                <a href="/" class="btn home-link">Go Home</a>
```

Replace with:

```gohtml
                <button type="button" class="btn btn--ghost">
                    <span>Retry</span>
                    <script {{ nonce }}>
                      document.currentScript.parentElement.addEventListener('click', function () {
                        location.reload()
                      })
                    </script>
                </button>
                <a href="/" class="btn">Go Home</a>
```

`Go Home` is the primary action (clay), `Retry` is the reversible secondary (ghost). In the page's `<style>` block, delete the `.home-link { … }` rule (lines 40–47).

After this edit, `grep -n 'home-link' ui/templates/pages/error/error.gohtml` must print nothing.

- [ ] **Step 3: workout-not-found — switch the primary button**

In `workout-not-found.gohtml`, find line 56:

```gohtml
            <a href="/" class="primary-button">Back to Home</a>
```

Replace with:

```gohtml
            <a href="/" class="btn">Back to Home</a>
```

Then in the page's `<style>` block (around line 29 onwards), delete the `.primary-button { … }` rule entirely.

After this edit: `grep -n 'primary-button' ui/templates/pages/workout-not-found/workout-not-found.gohtml` must print nothing.

- [ ] **Step 4: Run the not-found tests**

Run: `go test ./cmd/web/`
Expected: PASS. Relevant tests are `Test_application_notFound`, `Test_application_notFound_template_content`, `Test_application_workoutNotFound`.

- [ ] **Step 5: Manually verify**

Visit a bad URL like `/foo` (renders not-found) and `/workouts/9999-12-31` (renders workout-not-found). The action buttons keep their visual hierarchy — primary clay button is "Go Home" / "Back to Home"; the Go Back button on not-found is now an explicit ghost.

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/not-found/not-found.gohtml ui/templates/pages/error/error.gohtml ui/templates/pages/workout-not-found/workout-not-found.gohtml
git commit -m "$(cat <<'EOF'
Migrate 404/error pages to .btn variants

Replaces .home-link / .back-link / .primary-button (which extended .btn
then recoloured it) with .btn and .btn .btn--ghost. The visual hierarchy
(primary CTA + secondary Go Back) is now explicit in the markup.
EOF
)"
```

---

## Task 12 — Final sweep + ci

- [ ] **Step 1: Grep for any leftover bespoke button class names**

Run:

```bash
grep -rn -E '(save-button|delete-button|export-button|logout-button|push-button|submit-button|warmup-complete-button|swap-button|add-button|primary-button|menu-button a|add-exercise-button|edit-button|home-link|back-link)' ui/templates/ cmd/web/
```

Expected: only matches inside `cmd/web/*_test.go` (the playwright comments mentioning `back-link` / `data-back-button` are about the navigation back-link **partial component** at `ui/templates/components/back-link.gohtml`, which is unrelated to the button classes — leave those alone). If `ui/templates/...` lines show, finish the migration there. The `internal/e2etest/client.go` comment mentioning `submit-button` is about HTML submit semantics, not a CSS class — leave it.

- [ ] **Step 2: Update `ui/templates/CLAUDE.md` button-component note**

Open `ui/templates/CLAUDE.md`. Find the "Current components" section (around lines 76–104). In the "Class-components" paragraph at the bottom, replace:

```markdown
**Class-components** (`main.css @layer components`): `.btn`/`button`,
`.badge` (+ `--success`/`--warning`/`--neutral`/`--info`), `.card`.
```

with:

```markdown
**Class-components** (`main.css @layer components`):

- `.btn`/`button` — primary (default) + variants `.btn--quiet`,
  `.btn--ghost`, `.btn--danger`, `.btn--focus`; modifiers `.btn--sm`,
  `.btn--block`. Variants are mutually exclusive; modifiers compose with
  any variant. The base rule guarantees a 44 px min touch target and a
  visible `:disabled` state. Every variant is in `/dev/styleguide`.
- `.badge` (+ `--success`/`--warning`/`--neutral`/`--info`).
- `.card`.
```

- [ ] **Step 3: Run full ci pipeline**

Run: `make ci`
Expected: PASS — init + build + lint-fix + test + sec.

- [ ] **Step 4: Final manual walkthrough**

Visit in order, confirm visually:
- `/dev/styleguide` — all variants/modifiers/states render distinctly.
- `/preferences` — every button consistent, rest-notifications buttons visible, danger zone reads destructive, deload Restart looks disabled when planned deloads off.
- `/` (home) — preferences cog chip.
- `/schedule` — Start Tracking button + clay select focus.
- `/workouts/<today>` — Add an exercise pill, Complete workout primary.
- `/workouts/<today>/exercises/<id>` — Done! / Start timer ember CTA, Edit chip on completed sets, warmup ember.
- `/workouts/<today>/add-exercise` — Add this exercise pill.
- A swap page — Swap to this pill.
- A 404 (`/foo`) — Go Home + Go Back.

- [ ] **Step 5: Commit the doc + sweep**

```bash
git add ui/templates/CLAUDE.md
git commit -m "$(cat <<'EOF'
Document the button variant system in ui/templates/CLAUDE.md

Updates the Current Components note to list the .btn variants and
modifiers, the 44 px touch target guarantee, and the :disabled state, so
the convention is discoverable from the template-layer guide.
EOF
)"
```

- [ ] **Step 6: Confirm working tree is clean**

Run: `git status`
Expected: `nothing to commit, working tree clean`. The branch is ahead of `origin/main` by 12 commits (one for each Task / Task-subpart above plus the original spec commit).
