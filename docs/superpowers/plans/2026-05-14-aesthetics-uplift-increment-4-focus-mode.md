# Aesthetics Uplift — Increment 4: Exercise-Set / Focus Mode — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the in-workout Focus mode — the dark `--stone-9` active-set panel with oversized weight × reps numerals, an Ember-promoted "log set" CTA, and dark-panel form widgets — while warming the surrounding warmup banner, exercise header and non-active set surfaces in the exerciseset templates fully onto the Stone palette.

**Architecture:** Increments 1–3 retuned the token layer and the home / schedule / workout-overview pages. This increment is the marquee Focus-mode work, scoped to the four templates under `ui/templates/pages/exerciseset/`. The handler already exposes `setDisplay.IsActive` (see `cmd/web/handler-exerciseset.go` and `Test_computeSetActive`), so no Go change is needed for activation. The active-set treatment is layered onto the existing `.exercise-set.active` `.card` element via new scoped CSS plus a minimal markup change — wrapping the numeric portion of each `.weight` / `.reps` target span in a `<span class="value">` so the digits can be sized independently of the unit suffix without flattening the `Text()`-based test assertions. Every existing DOM hook the test suite relies on (`.exercise-set`, `.exercise-set.completed`, `.exercise-set.completed .edit-button`, `.weight`, `.reps`, `.deload-banner`, `.rest-chip[data-rest-end-at-ms]`, `[data-rest-time]`, `button.signal-btn`, `button[name='signal']`, `input[name='weight']`, `button:contains('Done!')`, `button:contains('Mark Warmup Complete')`) survives intact.

**Tech Stack:** CSS (custom properties, `@layer`, `@scope`, CSS nesting, `color-mix(in oklab, …)` for the Ember and Info hover darkens), Go `html/template`, Go (`cmd/web` handler + `goquery`-based e2e tests).

**Spec:** `docs/superpowers/specs/2026-05-14-aesthetics-uplift-design.md` (Increment 4 — Exercise-set / Focus mode).

---

## File Structure

### Modified files

| File | Change |
|---|---|
| `ui/templates/pages/exerciseset/warmup.gohtml` | Warm `--white` (button text, check-icon text) onto `--stone-0`, the `--sky-7` hover onto a darken-of-`--color-info` via `color-mix`, and the `--lime-3` status-pill border onto `--color-success`. |
| `ui/templates/pages/exerciseset/exercise-header.gohtml` | Warm the Info / Swap action-button `--sky-1` / `--lime-1` hover holdouts onto darken-of-semantic via `color-mix`. |
| `ui/templates/pages/exerciseset/sets-container.gohtml` | Warm the **non-active** holdouts (completed-set status-icon `--white`, edit-button hover `--gray-1` / `--gray-4`, deload-banner `--sky-*`, `.active` outer ring `--sky-1`), then add the new **active-set Focus panel**: dark `--stone-9` surface, oversized `--font-mono` `.weight / .reps .value` numerals on a 2-column grid (stacking on `<380px`), Ember-promoted `.on-target-btn`, dark-panel form inputs (`--stone-8` recessed wells, Ember focus ring), Ember-solid `.submit-button` for deload-week and bodyweight single-button variants, and a `<span class="value">` markup wrap on the five target-weight / target-reps spans so the digit-vs-unit sizing works without breaking `.Find(".weight").Text()` / `.Find(".reps").Text()` assertions. |

### Out of scope (untouched)

- `ui/templates/pages/exerciseset/exerciseset.gohtml` — the page shell only references semantic tokens (`--color-surface`, `--color-text-primary`, `--color-border`) and the fixed-position `.timer` chip; a confirming `grep` shows zero raw-ramp / `--gray-*` / `--white` holdouts. Leave it alone; the view-transition wiring (`view-transition-name: exercise-title-{{ .ExerciseSet.ID }}` on the `exercise-header` `.exercise-title` h1) is in `exercise-header.gohtml` and is not edited either.
- `ui/static/main.css`, shared components (`.btn` / `button`, `.card`, `.badge`, `back-link`, `page-header`, `banner`, `field`) — already on the Stone system from Increment 1. Every shared component used by these four pages (the back-link, the inherited Clay-4 `button` styling on submit buttons, the `.card` shell of `.exercise-set`) shifts automatically.
- All other `pages/**` templates — their `--gray-*` call sites keep warming via the Increment 1 aliases until their own increment (Increment 5).
- The Go handler (`cmd/web/handler-exerciseset.go`) — active-set state is already computed (`computeSetActive`, exposed as `setDisplay.IsActive`). No Go change in this increment.
- The handler tests in `cmd/web/handler-exerciseset_test.go` — every selector the suite reads survives this increment intact; **no test edits are made**.

---

## Token mapping reference

The single source of truth for every substitution in this increment. All target tokens are defined in `ui/static/main.css` (verified present after Increment 1: `--stone-0..10`, `--clay-0..6`, `--ember`, the warm `--shadow-1/2/3`, `--font-mono`, `--font-size-fluid-3`, `--color-text-primary` / `-secondary` / `-muted`, `--color-success` / `-bg`, `--color-warning` / `-bg`, `--color-error` / `-bg`, `--color-info` / `-bg`, `--color-surface-elevated`, `--color-surface-active`, `--color-surface-completed`, `--color-border`, `--color-border-focus`).

**`--white` → `--stone-0`:** every existing call site is "white text on a colored surface" — `.warmup-complete-button` text on info, `.warmup-status .check` check-mark on success, `.status-icon` check-mark on success, `.submit-button` text on the previously-success-green button. All move to `--stone-0` (a warm off-white), keeping contrast against the darker fill below.

**Raw-ramp hover darkens → `color-mix(in oklab, <semantic> XX%, black|white)`:** the semantic tokens (`--color-info`, `--color-info-bg`, `--color-success-bg`) have no darker companion tokens and the spec does not introduce them. `color-mix(in oklab, …)` is a 2023-baseline CSS feature (Chrome 111+, Firefox 113+, Safari 16.2+), well within the project's no-fonts/no-deps constraint and adds no token. **This is a new pattern in this codebase** — the only color-mix call sites in the project are introduced by this increment. Each call is a one-liner inside a `:hover` rule.

| Old raw token / value | Use | New value |
|---|---|---|
| `var(--white)` | warmup button text, warmup-status check, sets-container `.status-icon`, sets-container `.submit-button` text | `var(--stone-0)` |
| `var(--sky-7)` | `.warmup-complete-button:hover` background | `color-mix(in oklab, var(--color-info) 85%, black)` |
| `var(--lime-3)` | `.warmup-status` border | `var(--color-success)` |
| `var(--sky-1)` | exercise-header `.info-button:hover` background | `color-mix(in oklab, var(--color-info-bg) 90%, black)` |
| `var(--lime-1)` | exercise-header `.swap-button:hover` background | `color-mix(in oklab, var(--color-success-bg) 90%, black)` |
| `var(--gray-1)` | sets-container `.edit-button:hover` background | `var(--stone-1)` |
| `var(--gray-4)` | sets-container `.edit-button:hover` border | `var(--stone-4)` |
| `var(--sky-1)` | sets-container `.exercise-set.active` outer ring (replaced; see Task 4) | `var(--shadow-2)` (the active panel is the dark `--stone-9` panel; the outer ring is replaced by an elevated shadow, **not** a sky-tinted glow) |
| `var(--sky-1)` (line ~135) | sets-container `.input-field input:focus` ring (rewritten in Task 5 as part of dark-panel inputs) | `rgb(224 138 76 / 0.25)` (Ember at 25% alpha — matches the `--ember: #e08a4c` hex; raw `box-shadow` is permitted by `ui/templates/CLAUDE.md` for focus rings) |
| `var(--red-5)` | sets-container `.input-field input:user-invalid` border | `var(--color-error)` |
| `var(--red-1)` / `--red-8` / `--red-3` / `--red-2` / `--red-5` | sets-container `.too-heavy-btn` (rewritten in Task 5 onto dark-panel ghost-button treatment — transparent fill, `--stone-7` border, `--stone-3` text) | transparent fill, `var(--stone-3)` text, `var(--stone-7)` border; hover lifts to `var(--stone-8)` bg; active adds a `var(--color-warning)` 3px shadow ring |
| `var(--lime-2)` / `--lime-4` / `--lime-5` / `--lime-7` | sets-container `.on-target-btn` / `.submit-button` (rewritten in Task 5 as the Ember-promoted primary CTA) | `var(--ember)` solid background, `var(--stone-10)` text, hover `color-mix(in oklab, var(--ember) 90%, white)`, active `color-mix(in oklab, var(--ember) 85%, black)` with a `var(--clay-2)` 3px ring |
| `var(--sky-1)` / `--sky-2` / `--sky-3` / `--sky-5` | sets-container `.too-light-btn` (rewritten in Task 5 — same ghost treatment as `.too-heavy-btn` with a `var(--color-info)` active ring) | transparent fill, `var(--stone-3)` text, `var(--stone-7)` border; hover `var(--stone-8)`; active adds `var(--color-info)` ring |
| `var(--sky-1)` / `--sky-10` / `--sky-7` | sets-container `.deload-banner` background / text / border-left | `var(--color-info-bg)` / `var(--color-info)` / `var(--color-info)` |

**Note on Ember at 25% alpha.** The Ember-on-stone-9 focus ring needs a colored glow with alpha. CSS variables in alpha contexts work as `color-mix(in oklab, var(--ember), transparent 75%)`, but for the box-shadow we keep it simple with the literal `rgb(224 138 76 / 0.25)` — the exact alpha-25 version of the `--ember: #e08a4c` hex. This matches the existing pattern in `workout.gohtml` where the sticky-bar shadow uses a literal `rgb(120 90 60 / 0.10)` rather than expanding `--shadow-1` (since the geometry differs from the token). The Ember focus ring is the same "the token doesn't carry alpha" situation.

**Note on `color-mix` as a new pattern.** Increments 1–3 used only the existing token surface. This increment introduces `color-mix(in oklab, …)` for three hover-darken cases (the warmup-banner button, both exercise-header action buttons) plus the active-set panel's Ember hover/active states. The alternative — defining `--color-info-hover`, `--color-info-bg-hover`, `--color-success-bg-hover`, `--ember-hover`, `--ember-active` tokens — is over-engineering for five call sites. `color-mix(in oklab, …)` is a CSS-built-in, requires no token, no JS, no dependency, and is the standard 2026 pattern for a single hover darken. It is introduced once in this increment and can be promoted to tokens later if reuse warrants it.

---

## Sequencing rationale

Six tasks decomposed bottom-up by risk. The three warm-pass tasks (1, 2, 3) handle low-risk one-line substitutions in three separate files, building confidence and proving the grep gate. Task 4 is the marquee — it introduces the dark `--stone-9` panel shell and the oversized-numeral typography, plus the markup change to add the `<span class="value">` wrapper. Task 5 polishes the form widgets *inside* the dark panel (inputs, signal buttons with the Ember-promoted middle button, the deload single-button case). Task 6 is the full-suite gate.

The split between Tasks 4 and 5 is reviewable-unit driven: Task 4 changes markup *and* establishes the panel; if a regression slips through it's caught early. Task 5 only touches CSS — pure-paint changes inside the panel that's already verified.

**Verification shape.** Same as Increments 1–3: scoped-style changes have no unit test (CSS has no compiler). Each task is verified by (a) `go test ./cmd/web/ -run <RenderTest> -v` against the touched render path; (b) a `grep` gate proving no raw-ramp / `--gray-*` / `--white` holdout survives in the touched file; (c) a described visual check (the user cannot drive a browser in this environment — perform the automated checks and explicitly report which described visual checks were *not* performed rather than claiming them). Task 6 runs the full `make ci`.

---

## Task 1: Warmup banner & status — warm onto Stone

**Files:**
- Modify: `ui/templates/pages/exerciseset/warmup.gohtml`

The warmup banner is the page's pre-warmup CTA and the post-warmup confirmation pill. Both already use the warmed semantic info / success tokens; only four hardcoded raw tokens remain. The button stays info-framed (per the design proposal — info is the right semantic; it's "you need to do this first", not a primary action against the rest of the app's Clay buttons).

- [ ] **Step 1: Re-point the `.warmup-complete-button` text and hover**

In `ui/templates/pages/exerciseset/warmup.gohtml`, in the `@scope` block, replace:

```css
                .warmup-complete-button {
                    padding: var(--size-3) var(--size-6);
                    background: var(--color-info);
                    color: var(--white);
                    border: none;
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-2);
                    font-weight: var(--font-weight-6);
                    cursor: pointer;
                    transition: background-color 0.2s ease, transform 0.1s ease;
                    min-width: 200px;

                    &:hover {
                        background: var(--sky-7);
                        transform: translateY(-1px);
                    }
```

with:

```css
                .warmup-complete-button {
                    padding: var(--size-3) var(--size-6);
                    background: var(--color-info);
                    color: var(--stone-0);
                    border: none;
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-2);
                    font-weight: var(--font-weight-6);
                    cursor: pointer;
                    transition: background-color 0.2s ease, transform 0.1s ease;
                    min-width: 200px;

                    &:hover {
                        background: color-mix(in oklab, var(--color-info) 85%, black);
                        transform: translateY(-1px);
                    }
```

- [ ] **Step 2: Re-point the `.warmup-status` border and the inner `.check` text**

In the same file, in the same `@scope` block, replace:

```css
            .warmup-status {
                display: flex;
                align-items: center;
                justify-content: center;
                gap: var(--size-2);
                padding: var(--size-2) var(--size-4);
                border-radius: var(--radius-round);
                background: var(--color-success-bg);
                color: var(--color-success);
                border: 1px solid var(--lime-3);
                font-size: var(--font-size-1);
                font-weight: var(--font-weight-6);
                align-self: center;

                .check {
                    display: inline-flex;
                    align-items: center;
                    justify-content: center;
                    width: 1.25rem;
                    height: 1.25rem;
                    border-radius: var(--radius-round);
                    background: var(--color-success);
                    color: var(--white);
                    font-size: var(--font-size-0);
                    font-weight: var(--font-weight-7);
                }
            }
```

with:

```css
            .warmup-status {
                display: flex;
                align-items: center;
                justify-content: center;
                gap: var(--size-2);
                padding: var(--size-2) var(--size-4);
                border-radius: var(--radius-round);
                background: var(--color-success-bg);
                color: var(--color-success);
                border: 1px solid var(--color-success);
                font-size: var(--font-size-1);
                font-weight: var(--font-weight-6);
                align-self: center;

                .check {
                    display: inline-flex;
                    align-items: center;
                    justify-content: center;
                    width: 1.25rem;
                    height: 1.25rem;
                    border-radius: var(--radius-round);
                    background: var(--color-success);
                    color: var(--stone-0);
                    font-size: var(--font-size-0);
                    font-weight: var(--font-weight-7);
                }
            }
```

- [ ] **Step 3: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain in the file**

Run:
```bash
grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/exerciseset/warmup.gohtml
```
Expected: no output (exit status 1).

- [ ] **Step 4: Verify the template still renders end-to-end**

Run: `go test ./cmd/web/ -run Test_application_exerciseSet -v`
Expected: PASS. (This test renders the warmup banner pre-completion and the warmup-status pill post-completion via `button:contains('Mark Warmup Complete')`.)

- [ ] **Step 5: Visual check (describe; cannot drive a browser here)**

Intended result on `make dev` → starting a new workout's first exercise: pre-warmup the banner is a warm-info card (info-bg surface, dark info border), the "Mark Warmup Complete" button sits in solid `--color-info` with warm off-white (`--stone-0`) text, hovering darkens it via `color-mix` (no hue shift to the legacy saturated `--sky-7`); post-warmup the status pill is a warm-success sage with a `--color-success` border (not the cool `--lime-3` it used) and the `✓` check renders warm off-white on success-green. Report this as "not visually verified — describe only".

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/exerciseset/warmup.gohtml
git commit -m "$(cat <<'EOF'
Warm the exerciseset warmup banner onto the Stone palette

Re-points the warmup-complete-button text from --white to --stone-0,
its --sky-7 hover to a darken-of-info via color-mix(in oklab, …), the
warmup-status pill border from --lime-3 to the warmed --color-success,
and the inner check text from --white to --stone-0.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Exercise header — warm the info / swap action-button hovers

**Files:**
- Modify: `ui/templates/pages/exerciseset/exercise-header.gohtml`

The exercise header is a three-column grid (back-link, h1, action cluster). The h1 carries the `view-transition-name: exercise-title-<id>` pairing the workout-overview navigation. Only the two action-button `:hover` rules carry raw tokens. The base info / success backgrounds already use the warmed semantic tokens.

- [ ] **Step 1: Re-point the `.info-button:hover` and `.swap-button:hover` backgrounds**

In `ui/templates/pages/exerciseset/exercise-header.gohtml`, in the `@scope` block, replace:

```css
                    &.info-button {
                        background: var(--color-info-bg);
                        color: var(--color-info);

                        &:hover {
                            background: var(--sky-1);
                        }
                    }

                    &.swap-button {
                        background: var(--color-success-bg);
                        color: var(--color-success);

                        &:hover {
                            background: var(--lime-1);
                        }
                    }
```

with:

```css
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
```

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain in the file**

Run:
```bash
grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/exerciseset/exercise-header.gohtml
```
Expected: no output (exit status 1).

- [ ] **Step 3: Verify the template still renders end-to-end**

Run: `go test ./cmd/web/ -run Test_application_exerciseSet -v`
Expected: PASS. (The Info and Swap action buttons render on `/workouts/<date>/exercises/<id>`; a typo or broken `@scope` fails at the document-parse stage.)

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `make dev` → exercise-set page: the Info button stays a soft warm-info pill, the Swap button stays a soft warm-success pill; hovering either visibly darkens the surface (10% black mix in oklab) without shifting hue away from the semantic; the back-link, h1 view-transition wiring and right-column cluster layout are unchanged. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exerciseset/exercise-header.gohtml
git commit -m "$(cat <<'EOF'
Warm the exerciseset header action-button hovers onto color-mix

Re-points the Info button's --sky-1 hover and the Swap button's
--lime-1 hover onto darken-of-semantic via color-mix(in oklab, …),
keeping each button's semantic (info / success) hue family.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Sets container — warm the non-active surfaces

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`

The sets-container has three concerns: the calm-Stone surfaces shared by all `.exercise-set` rows (the shell, the completed treatment, the universal `.set-info` layout and `.edit-button`), the active-set Focus panel (Tasks 4 + 5), and the standalone `.deload-banner` + `.rest-chip` siblings. This task handles only the non-active raw-token holdouts: the completed-set `.status-icon` `--white`, the `.edit-button` hover `--gray-1` / `--gray-4`, the `.deload-banner` `--sky-*` family, and the legacy `--sky-1` outer ring on `.exercise-set.active` (the outer ring is being replaced — the active-set shell shifts to `--stone-9` + `--shadow-2` in Task 4; cleaning it here keeps the `grep` gates monotonic).

- [ ] **Step 1: Re-point the completed-set `.status-icon` check text and the `.edit-button:hover`**

In `ui/templates/pages/exerciseset/sets-container.gohtml`, in the first `@scope` block (the one containing `:scope`, `.exercise-set`, etc.), replace:

```css
                        .status-icon {
                            display: inline-flex;
                            align-items: center;
                            justify-content: center;
                            width: 1.5rem;
                            height: 1.5rem;
                            border-radius: var(--radius-round);
                            background: var(--color-success);
                            color: var(--white);
                            font-size: var(--font-size-0);
                            font-weight: var(--font-weight-7);
                            flex-shrink: 0;
                        }

                        .edit-button {
                            margin-left: auto;
                            padding: var(--size-1) var(--size-3);
                            background: var(--color-surface-elevated);
                            border: 1px solid var(--color-border);
                            border-radius: var(--radius-2);
                            font-size: var(--font-size-0);
                            font-weight: var(--font-weight-5);
                            cursor: pointer;
                            text-decoration: none;
                            color: var(--color-text-secondary);
                            transition: background-color 0.2s ease, border-color 0.2s ease;

                            &:hover {
                                background: var(--gray-1);
                                border-color: var(--gray-4);
                            }

                            &:focus-visible {
                                outline: 2px solid var(--color-border-focus);
                                outline-offset: 2px;
                            }
                        }
```

with:

```css
                        .status-icon {
                            display: inline-flex;
                            align-items: center;
                            justify-content: center;
                            width: 1.5rem;
                            height: 1.5rem;
                            border-radius: var(--radius-round);
                            background: var(--color-success);
                            color: var(--stone-0);
                            font-size: var(--font-size-0);
                            font-weight: var(--font-weight-7);
                            flex-shrink: 0;
                        }

                        .edit-button {
                            margin-left: auto;
                            padding: var(--size-1) var(--size-3);
                            background: var(--color-surface-elevated);
                            border: 1px solid var(--color-border);
                            border-radius: var(--radius-2);
                            font-size: var(--font-size-0);
                            font-weight: var(--font-weight-5);
                            cursor: pointer;
                            text-decoration: none;
                            color: var(--color-text-secondary);
                            transition: background-color 0.2s ease, border-color 0.2s ease;

                            &:hover {
                                background: var(--stone-1);
                                border-color: var(--stone-4);
                            }

                            &:focus-visible {
                                outline: 2px solid var(--color-border-focus);
                                outline-offset: 2px;
                            }
                        }
```

- [ ] **Step 2: Replace the `.exercise-set.active` outer ring with an elevated warm shadow**

The legacy outer ring was a cool `--sky-1` glow that fades against the Stone palette and disappears entirely against the dark panel coming in Task 4. Replace it with `--shadow-2` (the warm-tinted card elevation token from Increment 1) so the active row reads as **raised** above the surrounding Stone cards — the same elevation language Increment 1 established for cards generally.

In the same `@scope` block, replace:

```css
                    &.active {
                        background: var(--color-surface-active);
                        border-color: var(--color-info);
                        box-shadow: 0 0 0 2px var(--sky-1);
                    }
```

with (this is a transitional value — Task 4 rewrites the `&.active` block to the dark panel; this step removes the `--sky-1` token now so the grep gate on this file does not depend on Task 4):

```css
                    &.active {
                        background: var(--color-surface-active);
                        border-color: var(--color-info);
                        box-shadow: var(--shadow-2);
                    }
```

- [ ] **Step 3: Re-point the `.deload-banner` raw `--sky-*` tokens onto the semantic info tokens**

In the same file, in the second `@scope` block (`@scope (.deload-banner) { ... }`), replace:

```css
        <style {{ nonce }}>
            @scope (.deload-banner) {
                :scope {
                    margin: var(--size-3) 0;
                    padding: var(--size-3) var(--size-4);
                    background: var(--sky-1);
                    color: var(--sky-10);
                    border-left: var(--border-size-3) solid var(--sky-7);
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-1);
                }
            }
        </style>
```

with:

```css
        <style {{ nonce }}>
            @scope (.deload-banner) {
                :scope {
                    margin: var(--size-3) 0;
                    padding: var(--size-3) var(--size-4);
                    background: var(--color-info-bg);
                    color: var(--color-info);
                    border-left: var(--border-size-3) solid var(--color-info);
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-1);
                }
            }
        </style>
```

- [ ] **Step 4: Confirm the only remaining raw-ramp / `--gray-*` / `--white` holdouts in the file live inside the `.exercise-set { ... }` active-form sub-tree (Tasks 4 + 5)**

Run:
```bash
grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/exerciseset/sets-container.gohtml
```
Expected: lines remaining are the seven holdouts that live inside the `.exercise-set` `.input-field`, `.signal-btn` (and its three button-variant blocks) and `.submit-button` rules — i.e. the form widgets that only render inside `&.active`. These are: the `.input-field input:focus` `box-shadow` `--sky-1`, the `.input-field input:user-invalid` `--red-5`, every `--red-*` line inside `.too-heavy-btn`, every `--lime-*` line inside `.on-target-btn`, every `--sky-*` line inside `.too-light-btn`, the `.submit-button` `--white` text and `--lime-7` hover. **No `--gray-*`, no `--white` outside `.submit-button`, no `--sky-1` outside `.input-field input:focus`.** Task 5 retires these.

- [ ] **Step 5: Verify the template still renders end-to-end**

Run: `go test ./cmd/web/ -run Test_application_exerciseSet -v && go test ./cmd/web/ -run TestExerciseSetGET_DeloadHidesSignalButtons -v`
Expected: both PASS. The first navigates to a completed set, asserts `.exercise-set.completed .edit-button` exists, exercises `.weight` / `.reps` text on the completed and edited rows. The second renders the `.deload-banner` and asserts `button.signal-btn` is absent (no signal buttons on a deload week).

- [ ] **Step 6: Visual check (describe; cannot drive a browser here)**

Intended result on `make dev` → an exercise with at least one completed set: the completed-set check-icon renders warm off-white (`--stone-0`) on success-green — no cool blue or cool white showing through; hovering the Edit button gives a warm sand tint (`--stone-1` bg, `--stone-4` border) instead of the cool grey it had; the active set's outer ring (the soft-rest visual that says "you are here") is now a warm-brown elevated shadow lifting the card off the page rather than the legacy cool sky-blue halo; on a deload week the deload-banner sits on a warm-info beige (`--color-info-bg`) with a dark-info text and left-rule (no saturated sky-blue). Report as "not visually verified — describe only".

- [ ] **Step 7: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "$(cat <<'EOF'
Warm the exerciseset non-active surfaces onto Stone

Re-points the completed-set status-icon check text from --white to
--stone-0, the edit-button hover from --gray-1/--gray-4 to
--stone-1/--stone-4, the deload-banner --sky-1/--sky-10/--sky-7 onto
the warmed semantic info tokens, and replaces the active-set's --sky-1
outer ring with the warm-tinted --shadow-2 elevation token. The
remaining --sky/lime/red/white holdouts in this file all live inside
the active-set form widgets — handled in the Focus-panel task.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Active-set Focus panel — structure + oversized numerals

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`

This is the marquee. Two concerns:

1. **Markup:** wrap the digit portion of the five target-weight / target-reps spans in a `<span class="value">` so the digits can be sized at `--font-size-fluid-3` independently of the unit suffix ("kg", "reps", "s") which stays calm. The wrapper is added to all five rendering branches that show a *target* (active and upcoming and pre-warmup) so the markup is uniform across rows; only the active row will get the oversized CSS treatment. Test impact: `.Find(".weight").Text()` / `.Find(".reps").Text()` continue to return the full concatenated text (e.g. "22.5 kg", "5 reps", "12 reps", "30s") because `goquery`'s `Text()` flattens descendants. **Zero test edits needed.**

2. **CSS:** add the `&.active { ... }` Focus-panel surface and the `&.active:not(.completed) .set-info { ... }` oversized-numeral grid. The dark panel is `--stone-9` background with `--stone-1` text and `--stone-7` border; the warm `--shadow-2` is kept from Task 3's transitional value. The oversized layout is a 2-column grid (`1fr 1fr`) of stacked `[label / big-number]` columns, stacking to a single column at `<380px`. Labels are pseudo-element `::before` (`content: "Weight"` / `content: "Reps"`) so no template change is needed to render them; pseudo-elements are inert to test selectors.

Task 5 handles the form widgets inside the panel (inputs, signal buttons, submit button, assisted field, signal-group legend).

- [ ] **Step 1: Wrap the digit portion of the five target spans in `<span class="value">`**

In `ui/templates/pages/exerciseset/sets-container.gohtml`, in the `range` block over `.SetsDisplay`, locate the five target-rendering branches and apply these five replacements. **Order of replacements is the textual order they appear in the file.**

Replacement 1 (weighted/assisted, active row — "Recommended weight" branch):

Replace:
```gohtml
                        {{ else if and $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
                            <span class="weight" aria-label="Recommended weight">{{ formatFloat $.CurrentSetTarget.WeightKg }} kg</span>
                            <span class="reps" aria-label="Target reps">{{ $.CurrentSetTarget.TargetReps }} reps</span>
```
with:
```gohtml
                        {{ else if and $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
                            <span class="weight" aria-label="Recommended weight"><span class="value">{{ formatFloat $.CurrentSetTarget.WeightKg }}</span> kg</span>
                            <span class="reps" aria-label="Target reps"><span class="value">{{ $.CurrentSetTarget.TargetReps }}</span> reps</span>
```

Replacement 2 (weighted/assisted, upcoming / pre-warmup row — "Target weight" branch):

Replace:
```gohtml
                        {{ else }}
                            <span class="weight" aria-label="Target weight">{{ formatFloat $.CurrentSetTarget.WeightKg }} kg</span>
                            <span class="reps" aria-label="Target reps">{{ $.CurrentSetTarget.TargetReps }} reps</span>
                        {{ end }}
                    {{ else }}
```
with:
```gohtml
                        {{ else }}
                            <span class="weight" aria-label="Target weight"><span class="value">{{ formatFloat $.CurrentSetTarget.WeightKg }}</span> kg</span>
                            <span class="reps" aria-label="Target reps"><span class="value">{{ $.CurrentSetTarget.TargetReps }}</span> reps</span>
                        {{ end }}
                    {{ else }}
```

Replacement 3 (time_based, active row — "Recommended target" branch):

Replace:
```gohtml
                        {{ else if and (eq $.ExerciseSet.Exercise.ExerciseType "time_based") $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
                            <span class="reps" aria-label="Recommended target">{{ $.CurrentSetTimedTarget }}s</span>
```
with:
```gohtml
                        {{ else if and (eq $.ExerciseSet.Exercise.ExerciseType "time_based") $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
                            <span class="reps" aria-label="Recommended target"><span class="value">{{ $.CurrentSetTimedTarget }}</span>s</span>
```

Replacement 4 (bodyweight / generic non-weighted, upcoming / pre-warmup — "Target value" branch):

Replace:
```gohtml
                        {{ else }}
                            <span class="reps" aria-label="Target value">{{ $setDisplay.TargetStr }} {{ $setDisplay.Unit }}</span>
                        {{ end }}
                    {{ end }}
                </div>
```
with:
```gohtml
                        {{ else }}
                            <span class="reps" aria-label="Target value"><span class="value">{{ $setDisplay.TargetStr }}</span> {{ $setDisplay.Unit }}</span>
                        {{ end }}
                    {{ end }}
                </div>
```

(The "Completed reps" / "Completed value" / "Weight" spans rendered when `$set.CompletedValue` is truthy are deliberately *not* wrapped — completed-set rows stay calm Stone in edit mode and the test reads `.exercise-set.completed .reps`'s exact text "12 reps" / "5 reps".)

- [ ] **Step 2: Replace the `.exercise-set.active` block with the Focus-panel surface and the oversized-numeral grid**

In the same file, in the first `@scope` block, locate the current `&.active { ... }` rule (modified in Task 3 Step 2 to use `--shadow-2`) and replace:

```css
                    &.active {
                        background: var(--color-surface-active);
                        border-color: var(--color-info);
                        box-shadow: var(--shadow-2);
                    }
```

with the Focus-panel block:

```css
                    &.active {
                        background: var(--stone-9);
                        color: var(--stone-1);
                        border-color: var(--stone-7);
                        box-shadow: var(--shadow-2);
                        padding: var(--size-4);

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

                            .weight::before { content: "Weight"; }
                            .reps::before { content: "Reps"; }

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
                        }
                    }
```

**Notes on this block.**
- `padding: var(--size-4)` on the active card overrides the `.card` shell's `--size-3` so the dark panel has more interior breathing room around the oversized numerals.
- `&:not(.completed) .set-info { ... }` deliberately excludes edit-mode rows. When the user is editing a completed set (`?edit=N`), the row is `.exercise-set.active.completed`; the panel chrome stays dark (the outer `&.active { background, color, border, shadow, padding }` applies) but the `.set-info` keeps the calm flex layout the unscoped rule provides — because what's rendered is the *completed value* the user is editing, and showing that at a 3.5rem scale would be misleading.
- The pseudo-element `::before` labels are inert to `goquery` and `Find(".weight").Text()` does not include `::before` content. The visible "WEIGHT" / "REPS" labels live in CSS only — no template change is needed.
- `font-family: var(--font-mono)` on the `.value` digits keeps column widths stable across reps changes (a 5 and a 12 should not jump the layout).
- `--font-size-fluid-3: clamp(2rem, 9vw, 3.5rem)` is the existing fluid scale token — already defined in Increment 1's token surface (line 134 of `main.css`).

- [ ] **Step 3: Confirm the file still parses and the active-set markup change does not regress the test suite**

Run: `go test ./cmd/web/ -run Test_application_exerciseSet -v && go test ./cmd/web/ -run Test_ExerciseSet_RestChipAfterCompletedSet -v`
Expected: both PASS. The first exercises the active-set `.weight` / `.reps` text (the active row's "Recommended weight" branch now contains the `<span class="value">` wrapper; `Text()` flattens and returns the same concatenated string as before, e.g. `"22.5 kg"`). The second triggers the rest-chip render after a set submission.

The post-completion test asserts `setReps == "12 reps"` on the **completed-set** row (line 263 of `handler-exerciseset_test.go`); completed-set rows are *not* wrapped, so the assertion stays exact-match.

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `make dev` → an exercise with the warmup complete and at least one upcoming weighted set: the active-set card swaps to a dark stone (`#2e2620`) panel with off-white text; two oversized columns dominate the panel — a "Weight" label in tiny uppercase-warmstone (`--stone-4`) above the big monospace digits "22.5" in warm off-white, and a matching "Reps" / "5" pair to its right, separated by visual breathing room; below the numerals the existing form widgets render (still in their pre-Task-5 colors — Task 5 reworks them); above and below the dark panel the completed and upcoming sets remain calm Stone cards (no change). On mobile under 380px the two columns stack to one. The dark panel sits raised on the warm `--shadow-2`. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "$(cat <<'EOF'
Add the Focus-mode dark panel and oversized numerals

Introduces the marquee Focus-mode treatment for the active set:
a --stone-9 dark panel with --stone-1 text, sitting elevated on the
warm --shadow-2; oversized mono numerals on a two-column grid
(stacking under 380px) for the target weight and reps; tiny uppercase
pseudo-element "Weight" / "Reps" labels above each digit. The five
target-value spans gain a <span class="value"> wrapper so digits and
unit suffix can be sized independently — goquery's Text() flattens
descendants so existing assertions on .weight / .reps text stay exact.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Active-set Focus panel — form widgets & Ember CTA

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`

The dark panel from Task 4 still hosts the un-warmed `.input-field input`, `.signal-btn` (and its three variants), `.submit-button`, `.assisted-field` and `.signal-group` rules — these have raw `--red/lime/sky-*` and `--white` holdouts. This task reworks those form widgets so they read correctly on the dark panel: inputs become recessed `--stone-8` wells with `--stone-0` mono digits; the signal buttons become a two-tier system (ghost buttons for `.too-heavy-btn` / `.too-light-btn`, Ember-solid primary for `.on-target-btn`); the `.submit-button` (deload-week and bodyweight single-button cases) goes Ember solid; the assisted field and signal-group legend pick up dark-panel text colors.

All of these rules nest under `&.active { ... }` so they only apply on the dark panel. The non-active `.exercise-set` paths render no form (no `{{ if $isActive }}` branch), so there is no calm-panel variant to maintain.

- [ ] **Step 1: Re-point the form widget block under `&.active` and retire the input-focus / signal-btn / submit-button raw tokens**

In `ui/templates/pages/exerciseset/sets-container.gohtml`, in the first `@scope` block, locate the chain of rules **after** `.set-info` and **before** the closing `}` of `.exercise-set`. These are:

- `.set-form` (lines ~102-108)
- `.input-field` (lines ~110-142, including `label`, `input`, `&:focus`, `&:user-invalid`)
- `.assisted-field` (lines ~144-188)
- `.signal-group` (lines ~190-206)
- `.signal-buttons` (lines ~208-216)
- `.signal-btn` (lines ~218-260, including `&.too-heavy-btn`, `&.on-target-btn`, `&.too-light-btn`)
- `.submit-button` (lines ~262-286)
- `.bodyweight-form` (lines ~288-295)

Currently they sit at the top level of `.exercise-set { ... }`. Move them all under a new `&.active { ... }` nested block — co-located with the `&.active` rule that Task 4 added — and rewrite their colors for the dark panel.

Replace the entire block (from `.set-form {` through the closing `}` of `.bodyweight-form`, lines ~102-295 of the file):

```css
                    .set-form {
                        display: flex;
                        flex-direction: column;
                        gap: var(--size-4);
                        padding-top: var(--size-2);
                        border-top: 1px solid var(--color-border);
                    }

                    .input-field {
                        display: flex;
                        flex-direction: column;
                        gap: var(--size-1);

                        label {
                            font-size: var(--font-size-0);
                            color: var(--color-text-secondary);
                            font-weight: var(--font-weight-5);
                        }

                        input {
                            width: 6rem;
                            padding: var(--size-2) var(--size-3);
                            border: 2px solid var(--color-border);
                            border-radius: var(--radius-2);
                            text-align: center;
                            font-size: var(--font-size-2);
                            font-weight: var(--font-weight-6);
                            background: var(--color-surface-elevated);
                            transition: border-color 0.2s ease, box-shadow 0.2s ease;

                            &:focus {
                                outline: none;
                                border-color: var(--color-border-focus);
                                box-shadow: 0 0 0 3px var(--sky-1);
                            }

                            &:user-invalid {
                                border-color: var(--red-5);
                            }
                        }
                    }

                    .assisted-field {
                        display: flex;
                        flex-direction: row;
                        align-items: center;
                        gap: var(--size-2);
                        flex-wrap: wrap;

                        label {
                            display: flex;
                            align-items: center;
                            gap: var(--size-2);
                            font-size: var(--font-size-1);
                            color: var(--color-text-primary);
                            font-weight: var(--font-weight-4);
                            cursor: pointer;
                        }

                        input[type="checkbox"] {
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

                        details {
                            font-size: var(--font-size-0);
                            color: var(--color-text-secondary);

                            summary {
                                cursor: pointer;
                                color: var(--color-info);

                                &:focus-visible {
                                    outline: 3px solid var(--color-border-focus);
                                    outline-offset: 2px;
                                }
                            }
                        }
                    }

                    .signal-group {
                        display: flex;
                        flex-direction: column;
                        gap: var(--size-2);
                        border: none;
                        padding: 0;
                        margin: 0;
                        min-width: 0;

                        > legend {
                            font-size: var(--font-size-1);
                            font-weight: var(--font-weight-6);
                            color: var(--color-text-primary);
                            padding: 0;
                            margin-bottom: var(--size-1);
                        }
                    }

                    .signal-buttons {
                        display: grid;
                        grid-template-columns: repeat(3, 1fr);
                        gap: var(--size-2);

                        @media (max-width: 380px) {
                            grid-template-columns: 1fr;
                        }
                    }

                    .signal-btn {
                        display: inline-flex;
                        align-items: center;
                        justify-content: center;
                        text-align: center;
                        padding: var(--size-3) var(--size-2);
                        border-radius: var(--radius-2);
                        font-weight: var(--font-weight-6);
                        font-size: var(--font-size-1);
                        cursor: pointer;
                        border: 2px solid transparent;
                        min-height: 3rem;
                        transition: background-color 0.2s ease, box-shadow 0.2s ease;

                        &:focus-visible {
                            outline: 3px solid var(--color-border-focus);
                            outline-offset: 2px;
                        }

                        &.too-heavy-btn {
                            background: var(--red-1);
                            color: var(--red-8);
                            border-color: var(--red-3);
                            &:hover { background: var(--red-2); }
                            &:active { background: var(--red-2); box-shadow: 0 0 0 3px var(--red-5); }
                        }

                        &.on-target-btn {
                            background: var(--color-success-bg);
                            color: var(--color-success);
                            border-color: var(--lime-4);
                            &:hover { background: var(--lime-2); }
                            &:active { background: var(--lime-2); box-shadow: 0 0 0 3px var(--lime-5); }
                        }

                        &.too-light-btn {
                            background: var(--color-info-bg);
                            color: var(--color-info);
                            border-color: var(--sky-3);
                            &:hover { background: var(--sky-2); }
                            &:active { background: var(--sky-2); box-shadow: 0 0 0 3px var(--sky-5); }
                        }
                    }

                    .submit-button {
                        padding: var(--size-3) var(--size-5);
                        background: var(--color-success);
                        color: var(--white);
                        border: none;
                        border-radius: var(--radius-2);
                        font-weight: var(--font-weight-6);
                        font-size: var(--font-size-1);
                        cursor: pointer;
                        transition: background-color 0.2s ease, transform 0.1s ease;

                        &:hover {
                            background: var(--lime-7);
                            transform: translateY(-1px);
                        }

                        &:active {
                            transform: translateY(0);
                        }

                        &:focus-visible {
                            outline: 3px solid var(--color-border-focus);
                            outline-offset: 2px;
                        }
                    }

                    .bodyweight-form {
                        display: flex;
                        gap: var(--size-3);
                        align-items: end;
                        flex-wrap: wrap;
                        padding-top: var(--size-2);
                        border-top: 1px solid var(--color-border);
                    }
```

with — note the closing `}` is the new `&.active` block's closing brace, which needs to come *before* the existing `.exercise-set` closing `}`:

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

                                &:focus {
                                    outline: none;
                                    border-color: var(--ember);
                                    box-shadow: 0 0 0 3px rgb(224 138 76 / 0.25);
                                }

                                &:user-invalid {
                                    border-color: var(--color-error);
                                }
                            }
                        }

                        .assisted-field {
                            display: flex;
                            flex-direction: row;
                            align-items: center;
                            gap: var(--size-2);
                            flex-wrap: wrap;

                            label {
                                display: flex;
                                align-items: center;
                                gap: var(--size-2);
                                font-size: var(--font-size-1);
                                color: var(--stone-1);
                                font-weight: var(--font-weight-4);
                                cursor: pointer;
                            }

                            input[type="checkbox"] {
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

                            details {
                                font-size: var(--font-size-0);
                                color: var(--stone-3);

                                summary {
                                    cursor: pointer;
                                    color: var(--ember);

                                    &:focus-visible {
                                        outline: 3px solid var(--color-border-focus);
                                        outline-offset: 2px;
                                    }
                                }
                            }
                        }

                        .signal-group {
                            display: flex;
                            flex-direction: column;
                            gap: var(--size-2);
                            border: none;
                            padding: 0;
                            margin: 0;
                            min-width: 0;

                            > legend {
                                font-size: var(--font-size-1);
                                font-weight: var(--font-weight-6);
                                color: var(--stone-0);
                                padding: 0;
                                margin-bottom: var(--size-1);
                            }
                        }

                        .signal-buttons {
                            display: grid;
                            grid-template-columns: 1fr 1.5fr 1fr;
                            gap: var(--size-2);

                            @media (max-width: 380px) {
                                grid-template-columns: 1fr;
                            }
                        }

                        .signal-btn {
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

                            &:focus-visible {
                                outline: 3px solid var(--ember);
                                outline-offset: 2px;
                            }

                            &.too-heavy-btn {
                                background: transparent;
                                color: var(--stone-3);
                                border-color: var(--stone-7);
                                &:hover { background: var(--stone-8); }
                                &:active { background: var(--stone-8); box-shadow: 0 0 0 3px var(--color-warning); }
                            }

                            &.on-target-btn {
                                background: var(--ember);
                                color: var(--stone-10);
                                border-color: var(--ember);
                                font-size: var(--font-size-2);
                                font-weight: var(--font-weight-8);
                                min-height: 3.5rem;
                                text-transform: uppercase;
                                letter-spacing: var(--font-letterspacing-3);
                                &:hover { background: color-mix(in oklab, var(--ember) 90%, white); }
                                &:active { background: color-mix(in oklab, var(--ember) 85%, black); box-shadow: 0 0 0 3px var(--clay-2); }
                            }

                            &.too-light-btn {
                                background: transparent;
                                color: var(--stone-3);
                                border-color: var(--stone-7);
                                &:hover { background: var(--stone-8); }
                                &:active { background: var(--stone-8); box-shadow: 0 0 0 3px var(--color-info); }
                            }
                        }

                        .submit-button {
                            padding: var(--size-3) var(--size-5);
                            background: var(--ember);
                            color: var(--stone-10);
                            border: none;
                            border-radius: var(--radius-2);
                            font-weight: var(--font-weight-8);
                            font-size: var(--font-size-2);
                            text-transform: uppercase;
                            letter-spacing: var(--font-letterspacing-3);
                            cursor: pointer;
                            min-height: 3.5rem;
                            transition: background-color 0.2s ease, transform 0.1s ease;

                            &:hover {
                                background: color-mix(in oklab, var(--ember) 90%, white);
                                transform: translateY(-1px);
                            }

                            &:active {
                                background: color-mix(in oklab, var(--ember) 85%, black);
                                transform: translateY(0);
                            }

                            &:focus-visible {
                                outline: 3px solid var(--ember);
                                outline-offset: 2px;
                            }
                        }
                    }
```

**Notes on this block.**
- The previous-task `&.active { ... }` block (the dark panel + oversized numerals) and this `&.active { ... }` block both nest under `.exercise-set`. CSS rules allow this — the two `&.active` blocks are siblings, both selecting the same `.exercise-set.active` and cascading by source order. Alternatively the implementer may consolidate them into a single `&.active { ... }` block; functionally identical.
- `.signal-buttons` grid changes from `repeat(3, 1fr)` to `1fr 1.5fr 1fr` so the middle Ember "Barely" button reads visually dominant (1.5x the width of the flanking buttons). On `<380px` it stacks to one column and the order is preserved: too_heavy → on_target → too_light. The on-target sits between the other two — the same accessibility order as before.
- `.on-target-btn` gets `min-height: 3.5rem`, `font-size-2`, `font-weight-8`, uppercase / wide-letterspaced — the visual "LOG SET — barely" CTA. The flanking buttons stay `min-height: 3rem`.
- `.submit-button` (deload-week and bodyweight cases — single button, no signal row) gets the Ember solid treatment with the same uppercase / oversized characteristics so the deload-week "Done!" reads as a primary CTA on the dark panel. `TestExerciseSetGET_DeloadHidesSignalButtons` asserts `button:contains('Done!')` exists; the new button selector is `.submit-button` so the `:contains` text match continues to work.
- The Ember-active state's `box-shadow: 0 0 0 3px var(--clay-2)` provides a warm focus halo for the press feedback — softer than the `--ember` itself, an in-palette member of the Clay family.
- The `.bodyweight-form`'s extra `flex-direction: row` override is kept so single-input bodyweight rows lay out as `[input][button]` side-by-side rather than stacked.
- The previous `.signal-btn`'s `border: 2px solid transparent` is dropped — the new ghost-button variants set `border-color` explicitly to `--stone-7`, so a transparent default is meaningless.

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain in the file**

Run:
```bash
grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/exerciseset/sets-container.gohtml
```
Expected: no output (exit status 1) — every raw-ramp, `--gray-*`, and `--white` holdout in `sets-container.gohtml` is gone.

- [ ] **Step 3: Verify the template still renders end-to-end and all four exerciseset tests pass**

Run: `go test ./cmd/web/ -run 'Test_application_exerciseSet$|Test_application_exerciseSet_assisted_storage|Test_ExerciseSet_RestChipAfterCompletedSet|TestExerciseSetGET_DeloadHidesSignalButtons|Test_computeSetActive' -v`
Expected: all PASS. Specifically: `Test_application_exerciseSet` exercises the weighted-set form (`button[name='signal']`, `input[name='weight']`, `input[name='reps']`); `Test_application_exerciseSet_assisted_storage` exercises the assisted checkbox; `Test_ExerciseSet_RestChipAfterCompletedSet` exercises the rest chip render after a set submission; `TestExerciseSetGET_DeloadHidesSignalButtons` exercises the deload-week `button:contains('Done!')` rendering with `button.signal-btn` absent.

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `make dev` → the active set on a non-deload week, weighted exercise, after warmup is complete: the dark panel from Task 4 now hosts recessed dark-stone inputs (`--stone-8` background, `--stone-7` border) with mono off-white digits at `--font-size-3`; the "Weight (kg)" / "Actual reps" labels above each input render in tiny uppercase `--stone-3` text — a quiet, glanceable field labelling system; below the inputs the "Did you reach N reps?" legend reads `--stone-0` strong; below it the three signal buttons render two-tier — "No" and "Could do more" as transparent ghost buttons with `--stone-7` outlines and `--stone-3` text, and the central "Barely" button as a bold solid-`--ember` Ember pill with `--stone-10` (near-black) text at 1.5× the flanking width and 3.5rem height, the unmistakable middle, set apart in `--font-weight-8` uppercase; on a deload-week or bodyweight exercise the single "Done!" button takes the same Ember-solid uppercase treatment; the Assisted checkbox (when present) is unchanged shape-wise but its label is now `--stone-1`, the `<details>` "What's this?" summary is `--ember`; focusing any input draws a 3px Ember-alpha-25 halo around it. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "$(cat <<'EOF'
Polish the Focus-mode form widgets onto the dark panel

Nests the .set-form / .bodyweight-form / .input-field / .assisted-field
/ .signal-group / .signal-buttons / .signal-btn / .submit-button rules
under &.active so they only paint on the dark Focus-mode panel.
The recessed-well inputs go --stone-8 / --stone-0 mono; the signal
buttons become a two-tier system with --ember promoted on the middle
.on-target-btn (1.5x width, --font-weight-8, uppercase) and the
flanking buttons as --stone-7 ghost outlines; the single-button
.submit-button case (deload week + bodyweight) takes the same Ember
solid treatment. Introduces color-mix(in oklab, …) for the Ember
hover / active darkens. Test selectors (.signal-btn, button[name=
'signal'], button:contains('Done!'), input[name='weight']) are all
preserved; no Go change.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Full-suite verification

**Files:** none (verification only)

- [ ] **Step 1: Confirm all four exerciseset templates are fully repainted**

Run:
```bash
grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/exerciseset/*.gohtml
```
Expected: no output (exit status 1) — every raw-ramp, `--gray-*`, and `--white` holdout across `exerciseset.gohtml`, `sets-container.gohtml`, `exercise-header.gohtml`, `warmup.gohtml` is gone.

- [ ] **Step 2: Run the exerciseset handler tests in full**

Run: `go test ./cmd/web/ -run 'ExerciseSet|exerciseSet|computeSetActive' -v`
Expected: every test in `cmd/web/handler-exerciseset_test.go` PASSES. Specifically: `Test_application_exerciseSet`, `Test_application_exerciseSet_swap_preserves_url_and_drops_completed_sets`, `Test_application_workoutSwapExercise_search_filters_by_name`, `Test_application_exerciseSet_nonexistent_exercise_returns_custom_404`, `Test_application_workoutSwapExercise_sorts_by_similarity`, `Test_application_exerciseSet_assisted_storage`, `Test_ExerciseSet_RestChipAfterCompletedSet`, `TestExerciseSetGET_DeloadHidesSignalButtons`, `Test_computeSetActive`.

- [ ] **Step 3: Run the full CI suite**

Run: `make ci`
Expected: `init`, `build`, `lint-fix`, `test`, `sec` all pass. If `lint-fix` makes formatting changes, review them, `git add` them, and commit with message `Apply lint-fix formatting`.

- [ ] **Step 4: Visual QA across the exerciseset page (describe; cannot drive a browser here)**

Run: `make dev`, register, enable today's weekday in `/preferences`, start a workout, and walk through one exercise:

1. Navigate to the first exercise. The page reads as warm Stone: header h1, info/swap buttons in warm-info/warm-success pills.
2. Pre-warmup: the warmup banner is a warm-info card; the "Mark Warmup Complete" button is solid info with off-white text and a `color-mix` hover darken.
3. Mark warmup complete. The warmup-status pill renders warm-success with a sage `--color-success` border and a `--stone-0` checkmark.
4. The first set's card becomes the dark `--stone-9` Focus panel: two oversized mono digit columns ("WEIGHT 22.5 / REPS 5") with uppercase `--stone-4` labels above. The panel sits elevated on `--shadow-2`.
5. Inside the dark panel: recessed `--stone-8` input wells with `--stone-0` mono digits, tiny uppercase `--stone-3` labels above each input. Focusing an input shows a 3px Ember-alpha-25 halo.
6. The three signal buttons render two-tier: ghost "No" and "Could do more" on the left and right, oversized solid `--ember` "Barely" in the middle (1.5x width, uppercase, weight-8).
7. Submit the active set with "Barely" / `on_target`. After redirect: the completed-set card reads as calm Stone — sage `--color-surface-completed` background, `--color-success` border, off-white `--stone-0` checkmark on success-green status-icon, calm `.weight` / `.reps` text, an Edit button with `--stone-1` / `--stone-4` warm hover.
8. The rest chip renders above the next active set in warm-info colors; after its TTL it switches to warm-success.
9. Below the active set, upcoming sets remain calm Stone cards.

Expected: the entire exercise-set surface reads as deliberate warm "Stone" with the Focus-mode dark panel as the single high-contrast moment. No element is left on a cool grey/green/yellow/sky-blue Tailwind-bright surface; the single bright accent is `--ember`, used only for the active-set primary CTA (and its press / focus halos). If a browser cannot be driven in this environment, state explicitly that the visual QA was not performed and that verification rests on the `grep` checks, the handler test suite, and `make ci`.

- [ ] **Step 5: Final confirmation**

Confirm `git status` is clean and `git log --oneline` shows the Task 1-5 commits (plus any lint-fix commit). Increment 4 is complete: the in-workout Focus mode is built; the four exerciseset templates fully realize the Stone direction.

---

## Self-review notes

- **Spec coverage:** Increment 4 of the spec calls for `exerciseset.gohtml`, `sets-container.gohtml`, `exercise-header.gohtml`, `warmup.gohtml`, and "the dark `--stone-9` active-set panel, oversized numbers, `--ember` CTA, restyled timer." Coverage:
  - `warmup.gohtml` → Task 1.
  - `exercise-header.gohtml` → Task 2.
  - `sets-container.gohtml` → Tasks 3 (non-active surfaces) + 4 (active-panel structure + oversized numbers) + 5 (active-panel form widgets, Ember CTA).
  - `exerciseset.gohtml` → out of scope by inspection — its scoped styles already use only semantic tokens and have zero raw-ramp / `--gray-*` / `--white` holdouts.
  - **"Restyled timer"** — the spec's wording is ambiguous between (a) the top-right floating `.timer` workout-elapsed-time chip in `exerciseset.gohtml` and (b) the `.rest-chip` inside `sets-container.gohtml` that counts down between sets. (a) already uses only semantic tokens (`--color-surface`, `--color-text-primary`, `--color-border`, `--color-border-focus`, `--font-mono`) — no warming needed; (b) likewise (`--color-info-bg` / `--color-info`, switching to `--color-success-bg` / `--color-success` on `.ready`). Both are already on the Stone-warmed semantic surface. The plan does not introduce further changes to either, since there is no raw-token holdout and the design proposal explicitly held them in place to avoid out-of-scope motion work (Increment 6's territory). Documented in the "Out of scope" section.
  - **"Oversized numbers"** → Task 4 Step 2: the `&.active:not(.completed) .set-info` 2-column grid with `--font-size-fluid-3` mono `.value` digits.
  - **"Dark `--stone-9` panel"** → Task 4 Step 2: `&.active { background: var(--stone-9); color: var(--stone-1); ... }`.
  - **"`--ember` CTA"** → Task 5 Step 1: `.on-target-btn` solid `--ember`, the `.submit-button` for deload-week and bodyweight single-button cases solid `--ember`. This is the first consumer of `--ember` (added in Increment 1 but uncalled until now).

- **Why the `.value` markup wrapper:** the oversized "22.5 kg" / "5 reps" treatment needs the digits at a 3.5rem fluid scale and the unit suffix at a smaller calm scale. The minimum-impact restructure is a `<span class="value">` wrap around the digit portion of each target span. Tests reading `.weight` / `.reps` use `goquery`'s `Text()` which flattens descendants — `setReps := doc.Find(".exercise-set.completed .reps").First().Text()` returns `"12 reps"` whether the digit is wrapped or not. Completed-set spans are intentionally not wrapped (their styling stays calm Stone) so the test's exact-match `setReps != "12 reps"` continues to compare against unwrapped text. **Zero test edits are needed across this entire increment.**

- **Why the markup change is in Task 4 not Task 3:** keeping the `.value` wrap colocated with the CSS that consumes it makes the marquee task self-contained and reviewable. A subagent assigned Task 4 sees one cohesive unit: the dark panel surface, the oversized-numerals CSS, and the markup that enables them.

- **Why CSS nesting + `&.active`:** the form widgets only render when `$isActive` is true (the template gates them inside `{{ if $isActive }}`). Top-level `.exercise-set` rules for `.input-field`, `.signal-btn`, `.submit-button` etc. would in principle apply to non-active rows too, but in practice never paint anything (no descendant exists to style). Nesting them under `&.active` makes the dark-panel intent explicit, lets them reference `--stone-8` / `--ember` / etc. freely without worrying about non-active fallout, and shrinks the rule's specificity surface to exactly the case it serves.

- **Why introduce `color-mix(in oklab, …)`:** five hover/active darken cases need a "darker version of an existing semantic token" — three on calm-Stone surfaces (Task 1's `--color-info` button hover, Task 2's two action-button hovers) and two inside the dark panel (Task 5's Ember hover & active). The codebase has no `--color-info-hover` / `--ember-hover` tokens and the spec does not introduce them; adding tokens for five one-line call sites is over-engineering. `color-mix(in oklab, …)` is a 2023-baseline CSS feature (well within the project's no-fonts/no-deps constraint), adds no runtime cost, and is the standard 2026 pattern. The increment introduces it in one place and the same pattern is reused; future increments can promote any reused mix to a token if pressure builds.

- **`--ember` alpha-25 focus ring as literal RGB:** `box-shadow: 0 0 0 3px rgb(224 138 76 / 0.25)` — the digit triplet matches `--ember: #e08a4c`. Same pattern Increment 3 used for the `.complete-workout` sticky-bar shadow (`rgb(120 90 60 / 0.10)`), and `ui/templates/CLAUDE.md` explicitly permits raw `box-shadow` for color-tinted glows / focus rings. Adding an `--ember-focus-ring` token for one call site is over-engineering on the same logic as above.

- **`box-shadow: var(--shadow-2)` on the active card** (Task 3 Step 2, kept by Task 4 Step 2): the warm-tinted `--shadow-2` token from Increment 1 (`0 2px 8px 0 rgb(120 90 60 / 0.12), 0 1px 2px -1px rgb(120 90 60 / 0.10)`) gives the dark panel the same elevation language as the `.card` shell, which the panel already sits inside. The `.card` base sets `box-shadow: var(--shadow-1)`; the `&.active` override lifts it one step to `--shadow-2` — a calibrated "raised" affordance that says "you are here".

- **No DOM changes that break tests:** the markup change is purely additive — `<span class="value">` wraps the digit portion of five existing spans without altering the spans' class names, attributes, or text content. Every `goquery` selector the test suite relies on (`.exercise-set`, `.exercise-set.completed`, `.exercise-set.completed .edit-button`, `.weight`, `.reps`, `.weight`'s parent text, `.deload-banner`, `.rest-chip[data-rest-end-at-ms]`, `[data-rest-time]`, `button.signal-btn`, `button[name='signal']`, `input[name='weight']`, `input[name='reps']`, `button:contains('Done!')`, `button:contains('Mark Warmup Complete')`, `form` presence, `h1` presence) remains intact. The view-transition wiring (`view-transition-name: exercise-title-<id>` on the `.exercise-title` h1) is not touched. `data-workout-exercise-id` on the h1 is not touched. **The handler is not touched.**

- **Verification shape:** no fake unit tests for hex values — CSS has no compiler. Verification is per-file `grep` gates + per-task render tests against the touched render path + full `make ci` at the end + described (and honestly caveated) visual checks. This mirrors the Increment 1-3 plans exactly.

- **Out of scope (per the spec's traffic-ordered staging):** all other `pages/**` and the motion pass (Increments 5-6). Their `--gray-*` call sites keep warming via the Increment 1 aliases until their own increment.
