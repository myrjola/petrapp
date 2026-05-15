# Aesthetics Uplift — Increment 5: Secondary Pages — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sweep every remaining `pages/**` template onto the warm "Stone" palette and fully retire the transitional `--gray-*` alias token names from the codebase.

**Architecture:** Increments 1–4.1 retuned the token layer (Stone/Clay/Ember ramps, re-pointed semantic tokens, `--gray-*` aliased onto Stone) and polished home/schedule, workout overview, and the in-workout Focus mode. This increment is per-page CSS polish only: each remaining secondary page has its scoped `<style>` blocks cleaned up, its raw-ramp holdouts (`--sky/lime/yellow/red-*` and `--white`) re-pointed onto warm semantic / Stone / Clay tokens, and its `--gray-*` references renamed to `--stone-*` or the appropriate semantic token. The final task deletes the `--gray-*` alias definitions from `ui/static/main.css` once no template references them. No DOM structure, no class names, no Go code changes — so the existing `goquery` / Playwright DOM tests stay green untouched.

**Tech Stack:** CSS (custom properties, `@layer`, `@scope`), Go `html/template`, Go (`cmd/web` handler + `goquery`-based e2e tests).

**Spec:** `docs/superpowers/specs/2026-05-14-aesthetics-uplift-design.md` (Increment 5).

---

## File Structure

### Modified files

| File | Holdouts | Change |
|---|---:|---|
| `ui/templates/pages/preferences/preferences.gohtml` | 21 | Save/export buttons (`--sky-*` + `--white`) → Clay; logout-button hover (`--gray-*`) → Stone; danger-zone surface (`--red-1/4`) → `--color-error-bg`/border; delete-button (`--red-11/12` + `--white`) → `--color-error` / `color-mix` darken; push-button error states (`--red-*`) → `--color-error`; deload-section surface (`--gray-0/7`) → `--color-surface` / `--color-text-secondary`. |
| `ui/templates/pages/exercise-info/exercise-info.gohtml` | 10 | Section title + muscle-group chips (`--gray-*`) → `--color-text-*` / `--stone-*`; primary muscle-group accent (`--sky-1/8`) → Clay; category badge (`--lime-1/8`) → `--color-success-bg` / `--color-success`; admin edit link (`--yellow-2/3/8`) → Clay (admin affordance, not a warning). |
| `ui/templates/pages/exercise-swap/exercise-swap.gohtml` | 10 | Current-exercise surface (`--gray-1/5/6`) → `--stone-1` / `--color-text-secondary`; search input (`--gray-3`, `--white`) → `--color-border` / `--color-surface-elevated`; swap-button (`--lime-2/3/9`) → `--color-success-bg` / `color-mix` hover / `--color-success`. |
| `ui/templates/pages/exercise-add/exercise-add.gohtml` | 7 | Search input (`--gray-3`, `--white`) → `--color-border` / `--color-surface-elevated`; option-details / no-results muted text (`--gray-6`) → `--color-text-secondary`; add-button (`--lime-2/3/9`) → `--color-success-bg` / `color-mix` hover / `--color-success`. |
| `ui/templates/pages/workout-completion/workout-completion.gohtml` | 7 | "How did it feel today?" subhead (`--gray-7`) → `--color-text-secondary`; difficulty buttons (`--white`/`--gray-3/9`) → elevated surface with stone border and clay-accent hover (no Ember — reserved for Focus-mode CTAs per spec). |
| `ui/templates/pages/schedule/schedule.gohtml` (the schedule **editor**, not the home weekly view) | 4 | "Start Tracking" save-button (`--sky-6/7/8` + `--white`) → Clay primary. |
| `ui/templates/pages/privacy/privacy.gohtml` | 6 | Headings / copy / list items (`--gray-7/8`) → `--color-text-*`; inline `<code>` (`--gray-1`) → `--stone-1`; "← Back to Home" link (`--sky-6/7`) → Clay. |
| `ui/templates/pages/error/error.gohtml` | 5 | Container background (`--gray-0`) → `--color-surface`; title / message text (`--gray-9/6`) → `--color-text-*`; "Go Home" button (`--gray-6/7`) → stone-bordered surface (the `.btn` base provides clay theming). |
| `ui/templates/pages/not-found/not-found.gohtml` | 8 | Container background (`--gray-0`) → `--color-surface`; title / subtitle / message text (`--gray-9/7/6`) → `--color-text-*`; "Go Home" primary (`--sky-6/7`) → Clay; "Go Back" secondary (`--gray-6/7`) → stone-bordered surface. |
| `ui/templates/pages/workout-not-found/workout-not-found.gohtml` | 4 | Body copy (`--gray-7`) → `--color-text-secondary`; "Back to Home" primary (`--lime-6/7` + `--white`) → Clay primary. |
| `ui/templates/pages/maintenance/maintenance.gohtml` | 9 | Container surface (`--gray-0`) → `--color-surface`; title / body text (`--gray-9/6`) → `--color-text-*`; subtitle + icon (`--yellow-5/7/9`) → `--color-warning`; notice band (`--yellow-1/3/11`) → `--color-warning-bg` / `--color-warning`. |
| `ui/templates/pages/styleguide/styleguide.gohtml` | 7 | Scale-row divider + shadow-grid container (`--gray-1/7`) → `--stone-1` / `--color-text-secondary`; scale-bar fill (`--sky-5`) → `--clay-3`; radius-sample (`--sky-2/9`) → `--clay-1/6`; shadow-sample card (`--white`) → `--color-surface-elevated`. These are the dev styleguide's *page-styling* holdouts; they have nothing to do with the page's role of displaying tokens (those samples are generated from `.ColorTokens` data — see lines 158-170 — and remain unchanged). |
| `ui/static/main.css` | — | Final task: delete the `--gray-0..10` alias definitions (lines ~152-164) and the accompanying explanatory comment, once `grep -rn 'var(--gray-' ui/templates/` returns nothing. |

### Out of scope (untouched)

- All admin pages (`pages/admin-exercises/`, `pages/admin-exercise-edit/`, `pages/admin-feature-flags/`) — verified clean in the survey (no `--sky/lime/yellow/red/gray-*` or `--white` holdouts).
- `ui/templates/pages/exercise-info/progress-chart.gohtml` — has no scoped `<style>` block, so no palette work needed; it inherits the existing `<table>` styling.
- `pages/exerciseset/*` Focus-mode templates — Increment 4 / 4.1 territory. (The time-based oversized-numerals defect noted in the briefing is **not** folded into this increment — no exerciseset file is touched here, and a separate Increment 4.2 is the right place for it.)
- `pages/home/*`, `pages/workout/*` — already polished by Increments 2 and 3.
- All Go handlers and tests — no DOM or class-name changes are made, so no test edits are needed.

---

## Token mapping reference

The single source of truth for every substitution in this increment. All target tokens are defined in `ui/static/main.css` and proven by Increment 2's plan; verified still-present in the post-Increment-4.1 codebase.

**Raw-ramp holdouts → warm semantic tokens** (status semantics):

| Old raw token(s) | Meaning | New token |
|---|---|---|
| `--sky-6/7/8` (primary action button bg) | primary action | `--clay-4` base, `color-mix(in oklab, var(--clay-4) 88%, black)` hover, `color-mix(in oklab, var(--clay-4) 78%, black)` active |
| `--sky-1` / `--sky-8` (primary muscle accent) | primary accent | `--clay-1` / `--clay-6` |
| `--sky-2/9` (sample chip) | accent chip | `--clay-1/6` |
| `--sky-5` (visualization bar) | accent fill | `--clay-3` |
| `--sky-6/7` (link) | link / hover | `--clay-4` / `color-mix(in oklab, var(--clay-4) 88%, black)` |
| `--lime-1/8`, `--lime-2/9` | success / category badge | `--color-success-bg` / `--color-success` |
| `--lime-3` (hover on success surface) | success hover | `color-mix(in oklab, var(--color-success-bg) 92%, black)` |
| `--lime-6/7` + `--white` (success primary CTA) | "primary" CTA on workout-not-found | `--clay-4` / `color-mix(...)` (this CTA was using lime as a *primary*, not as success; the Clay treatment matches the rest of the app's primary buttons) |
| `--yellow-2/3/8` (admin edit chip) | admin/info chip | `--clay-1/6` (and `color-mix(in oklab, var(--clay-1) 92%, black)` hover) |
| `--yellow-5/7/9` (maintenance subtitle & icon) | warning text/icon | `--color-warning` |
| `--yellow-1/3/11` (maintenance notice surface) | warning surface | `--color-warning-bg` / `--color-warning` (border) / `--color-warning` (text) |
| `--red-1/4` (danger-zone surface) | error/destructive surface | `--color-error-bg` / `--color-error` border |
| `--red-11/12` + `--white` (delete button) | destructive primary | `--color-error` base, `color-mix(in oklab, var(--color-error) 88%, black)` hover/active, `--color-surface-elevated` text |
| `--red-3/8` (push-button danger + error message) | error text/border | `--color-error` / `color-mix(in oklab, var(--color-error) 30%, transparent)` |

**`--gray-N` → Stone / semantic** literal rename, except where a semantic token is the clearer fit:

| Old | New |
|---|---|
| `--gray-0` | `--color-surface` (when used as a page bg) / `--stone-0` (otherwise) |
| `--gray-1` | `--stone-1` (surface) / `--color-text-primary` only via direct mention |
| `--gray-3` | `--color-border` (when used as a border) / `--stone-3` (otherwise) |
| `--gray-4` | `--stone-4` |
| `--gray-5` (muted text) | `--color-text-secondary` |
| `--gray-6` (muted text) | `--color-text-secondary` |
| `--gray-6` (non-text bg) | `--stone-6` |
| `--gray-7` (text) | `--color-text-secondary` |
| `--gray-8` (text) | `--color-text-primary` |
| `--gray-9` (text) | `--color-text-primary` |
| `--white` (surface) | `--color-surface-elevated` |
| `--white` (text on coloured button) | `--color-surface-elevated` (kept for contrast on Clay/Error) |

---

## Sequencing rationale

The twelve files are independent — each is a self-contained scoped-style cleanup, verifiable on its own. Tasks 1–12 are ordered by visual prominence and risk: highest-traffic first (`preferences` is the largest file *and* has the destructive-action surface that warrants the most careful review), then exercise-detail pages, then the smaller error/empty-state pages, finishing with the dev-only `styleguide`. Task 13 deletes the `--gray-*` aliases once Task 12 has guaranteed no template references them. Task 14 is the full-suite gate.

**A note on verification:** CSS token/colour changes have no unit test — there is no compiler or linter for scoped `<style>` blocks. Each task is verified by (a) a per-file `grep` gate that fails if any raw-ramp / `--white` / `--gray-*` holdout remains, (b) a per-task render test against a handler test that exercises the touched template (or `go test ./cmd/web/ -count=1` when no dedicated test exists — templates are parsed per-request, so a route hit catches any template syntax error introduced by the edit; the broader run gives the same protection), and (c) a described visual check. Task 14 runs the full `make ci`. The user cannot drive a browser in this environment — perform the automated checks and explicitly report which described visual checks were not performed rather than claiming them.

---

## Task 1: Preferences — primary buttons, logout, danger zone, deload panel

**Files:**
- Modify: `ui/templates/pages/preferences/preferences.gohtml`

This is the largest task — 21 holdouts spanning four scoped blocks. Work top-to-bottom through the file.

- [ ] **Step 1: Re-point the `.save-button` (primary save) and `.export-button` (primary action) onto Clay**

In `ui/templates/pages/preferences/preferences.gohtml`, replace:

```css
                .save-button {
                    width: 100%;
                    padding: var(--size-4) var(--size-6);
                    background: var(--sky-6);
                    color: var(--white);
                    border: none;
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-6);
                    font-size: var(--font-size-2);
                    cursor: pointer;
                    transition: background-color 0.2s ease;

                    &:hover {
                        background: var(--sky-7);
                    }

                    &:active {
                        background: var(--sky-8);
                    }
                }
```

with:

```css
                .save-button {
                    width: 100%;
                    padding: var(--size-4) var(--size-6);
                    background: var(--clay-4);
                    color: var(--color-surface-elevated);
                    border: none;
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-6);
                    font-size: var(--font-size-2);
                    cursor: pointer;
                    transition: background-color 0.2s ease;

                    &:hover {
                        background: color-mix(in oklab, var(--clay-4) 88%, black);
                    }

                    &:active {
                        background: color-mix(in oklab, var(--clay-4) 78%, black);
                    }
                }
```

Then replace:

```css
                .export-button {
                    display: inline-block;
                    padding: var(--size-3) var(--size-5);
                    background: var(--sky-6);
                    color: var(--white);
                    text-decoration: none;
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-5);
                    font-size: var(--font-size-1);
                    transition: background-color 0.2s ease;

                    &:hover {
                        background: var(--sky-7);
                    }

                    &:active {
                        background: var(--sky-8);
                    }
                }
```

with:

```css
                .export-button {
                    display: inline-block;
                    padding: var(--size-3) var(--size-5);
                    background: var(--clay-4);
                    color: var(--color-surface-elevated);
                    text-decoration: none;
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-5);
                    font-size: var(--font-size-1);
                    transition: background-color 0.2s ease;

                    &:hover {
                        background: color-mix(in oklab, var(--clay-4) 88%, black);
                    }

                    &:active {
                        background: color-mix(in oklab, var(--clay-4) 78%, black);
                    }
                }
```

- [ ] **Step 2: Re-point the `.logout-button` hover onto Stone**

In the same file, replace:

```css
                    &:hover {
                        background: var(--gray-1);
                        color: var(--color-text-primary);
                        border-color: var(--gray-4);
                    }
```

(the `.logout-button` hover block) with:

```css
                    &:hover {
                        background: var(--stone-1);
                        color: var(--color-text-primary);
                        border-color: var(--stone-4);
                    }
```

- [ ] **Step 3: Re-point the `.danger-zone` surface and `.delete-button` onto `--color-error`**

In the same file, replace:

```css
                .danger-zone {
                    margin-top: var(--size-8);
                    padding: var(--size-6);
                    background: var(--red-1);
                    border: var(--border-size-1) solid var(--red-4);
                    border-radius: var(--radius-3);
                }
```

with:

```css
                .danger-zone {
                    margin-top: var(--size-8);
                    padding: var(--size-6);
                    background: var(--color-error-bg);
                    border: var(--border-size-1) solid var(--color-error);
                    border-radius: var(--radius-3);
                }
```

Then replace:

```css
                .delete-button {
                    padding: var(--size-3) var(--size-5);
                    background: var(--red-11);
                    color: var(--white);
                    border: none;
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-5);
                    font-size: var(--font-size-1);
                    cursor: pointer;
                    transition: background-color 0.2s ease;

                    &:hover {
                        background: var(--red-12);
                    }

                    &:active {
                        background: var(--red-12);
                    }
                }
```

with:

```css
                .delete-button {
                    padding: var(--size-3) var(--size-5);
                    background: var(--color-error);
                    color: var(--color-surface-elevated);
                    border: none;
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-5);
                    font-size: var(--font-size-1);
                    cursor: pointer;
                    transition: background-color 0.2s ease;

                    &:hover {
                        background: color-mix(in oklab, var(--color-error) 88%, black);
                    }

                    &:active {
                        background: color-mix(in oklab, var(--color-error) 78%, black);
                    }
                }
```

- [ ] **Step 4: Re-point the rest-notifications `.push-button.danger` and `.error` onto `--color-error`**

In the same file (inside the `@scope (.rest-notifications-section)` block), replace:

```css
                    .push-button.danger {
                        color: var(--red-8);
                        border-color: var(--red-3);
                    }

                    .error {
                        color: var(--red-8);
                        font-size: var(--font-size-1);
                        margin-top: var(--size-2);
                    }
```

with:

```css
                    .push-button.danger {
                        color: var(--color-error);
                        border-color: color-mix(in oklab, var(--color-error) 30%, transparent);
                    }

                    .error {
                        color: var(--color-error);
                        font-size: var(--font-size-1);
                        margin-top: var(--size-2);
                    }
```

- [ ] **Step 5: Re-point the `.deload-section` surface onto semantic tokens**

In the same file (inside the `@scope (.deload-section)` block), replace:

```css
                    :scope {
                        margin-top: var(--size-5);
                        padding: var(--size-4);
                        background: var(--gray-0);
                        border-radius: var(--radius-2);

                        h2 {
                            margin-top: 0;
                        }

                        .deload-controls {
                            display: flex;
                            flex-direction: column;
                            gap: var(--size-3);
                        }

                        .anchor-line {
                            font-size: var(--font-size-0);
                            color: var(--gray-7);
                        }
                    }
```

with:

```css
                    :scope {
                        margin-top: var(--size-5);
                        padding: var(--size-4);
                        background: var(--color-surface);
                        border-radius: var(--radius-2);

                        h2 {
                            margin-top: 0;
                        }

                        .deload-controls {
                            display: flex;
                            flex-direction: column;
                            gap: var(--size-3);
                        }

                        .anchor-line {
                            font-size: var(--font-size-0);
                            color: var(--color-text-secondary);
                        }
                    }
```

- [ ] **Step 6: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/preferences/preferences.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 7: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_preferences -v -count=1`
Expected: PASS (this exercises the `/preferences` GET handler and renders the full template).

- [ ] **Step 8: Visual check (describe; cannot drive a browser here)**

Intended result on `make dev` → `/preferences`: "Save Schedule" and "Download My Data" buttons are warm clay-orange primaries that darken on hover/press. The "Log out" footer button stays as a calm stone-bordered secondary that warms to soft sand on hover. The Danger Zone reads as a soft red-brown surface; "Delete my data" is a saturated warm-red primary. The Rest-notifications "Disable" button is a clay-bordered chip with red-tinted text. The Deload section sits on a warm-sand background; the cycle anchor line is secondary stone. No cool blue/red `--red-1*` Tailwind-bright tones remain. Report as "not visually verified — describe only".

- [ ] **Step 9: Commit**

```bash
git add ui/templates/pages/preferences/preferences.gohtml
git commit -m "$(cat <<'EOF'
Warm preferences page onto Stone — Clay primaries, semantic destructive

Re-points the save/export primaries onto Clay, swaps the cool-blue
hover/active darkens for color-mix oklab darkens, retunes the
logout-button hover, danger-zone, delete-button and rest-notifications
error states onto --color-error, and warms the deload-section surface.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Exercise info — section titles, muscle-group chips, category badge, admin edit

**Files:**
- Modify: `ui/templates/pages/exercise-info/exercise-info.gohtml`

- [ ] **Step 1: Re-point the page-level `.admin-edit` chip onto Clay**

In `ui/templates/pages/exercise-info/exercise-info.gohtml`, replace:

```css
                .admin-edit {
                    align-self: flex-start;
                    background: var(--yellow-2);
                    color: var(--yellow-8);
                    text-decoration: none;
                    padding: var(--size-1) var(--size-3);
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-0);
                    font-weight: var(--font-weight-6);
                }

                .admin-edit:hover {
                    background: var(--yellow-3);
                }
```

with:

```css
                .admin-edit {
                    align-self: flex-start;
                    background: var(--clay-1);
                    color: var(--clay-6);
                    text-decoration: none;
                    padding: var(--size-1) var(--size-3);
                    border-radius: var(--radius-2);
                    font-size: var(--font-size-0);
                    font-weight: var(--font-weight-6);
                }

                .admin-edit:hover {
                    background: color-mix(in oklab, var(--clay-1) 92%, black);
                }
```

(Rationale: this is an admin affordance — "you can edit this" — not a warning. Clay is the spec's designated action accent and matches the same treatment used for the `--gray-*` admin chips elsewhere.)

- [ ] **Step 2: Re-point the `.exercise-info` section titles, muscle-group chips, and primary accent**

In the same file (inside the `@scope (.exercise-info)` block), replace:

```css
                        .section-title {
                            font-weight: var(--font-weight-6);
                            color: var(--gray-8);
                            font-size: var(--font-size-2);
                        }

                        .muscle-group {
                            background: var(--gray-1);
                            border-radius: var(--radius-2);
                            padding: var(--size-1) var(--size-2);
                            font-size: var(--font-size-1);
                            color: var(--gray-7);
                        }

                        .primary {
                            background: var(--sky-1);
                            color: var(--sky-8);
                        }

                        .category-badge {
                            display: inline-flex;
                            align-items: center;
                            justify-content: center;
                            background: var(--lime-1);
                            color: var(--lime-8);
                            border-radius: var(--radius-2);
                            padding: var(--size-1) var(--size-2);
                            font-size: var(--font-size-1);
                            font-weight: var(--font-weight-5);
                            align-self: flex-start;
                        }
```

with:

```css
                        .section-title {
                            font-weight: var(--font-weight-6);
                            color: var(--color-text-primary);
                            font-size: var(--font-size-2);
                        }

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

                        .category-badge {
                            display: inline-flex;
                            align-items: center;
                            justify-content: center;
                            background: var(--color-success-bg);
                            color: var(--color-success);
                            border-radius: var(--radius-2);
                            padding: var(--size-1) var(--size-2);
                            font-size: var(--font-size-1);
                            font-weight: var(--font-weight-5);
                            align-self: flex-start;
                        }
```

- [ ] **Step 3: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/exercise-info/exercise-info.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 4: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_exerciseInfo -v -count=1`
Expected: PASS.

- [ ] **Step 5: Visual check (describe; cannot drive a browser here)**

Intended result on `/workouts/<date>/exercises/<id>/info`: the Category badge reads as a sage-green pill; the Primary muscle-group chips read as soft clay-tinted pills; secondary muscle groups stay as plain warm-stone chips. Section titles are primary stone. The admin "Edit Exercise" chip (when logged in as admin) is a soft clay-tinted pill that darkens on hover. Report as "not visually verified — describe only".

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/exercise-info/exercise-info.gohtml
git commit -m "$(cat <<'EOF'
Warm exercise-info chips and admin edit affordance onto Stone

Re-points the section titles and muscle-group chips onto Stone/text
semantic tokens, the primary muscle-group accent onto Clay, the
category badge onto --color-success, and the admin edit chip onto
Clay (admin affordance, not a warning).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Exercise swap — current-exercise card, search, swap button

**Files:**
- Modify: `ui/templates/pages/exercise-swap/exercise-swap.gohtml`

- [ ] **Step 1: Re-point the `.current-exercise` surface**

In `ui/templates/pages/exercise-swap/exercise-swap.gohtml`, replace:

```css
                    :scope {
                        background: var(--gray-1);
                        padding: var(--size-3);
                        border-radius: var(--radius-2);

                        h2 {
                            font-size: var(--font-size-2);
                            margin-bottom: var(--size-2);
                        }

                        .exercise-details {
                            gap: var(--size-1);
                        }

                        .category {
                            font-size: var(--font-size-0);
                            color: var(--gray-5);
                        }

                        .muscle-groups {
                            font-size: var(--font-size-0);
                            color: var(--gray-6);
                        }
                    }
```

with:

```css
                    :scope {
                        background: var(--stone-1);
                        padding: var(--size-3);
                        border-radius: var(--radius-2);

                        h2 {
                            font-size: var(--font-size-2);
                            margin-bottom: var(--size-2);
                        }

                        .exercise-details {
                            gap: var(--size-1);
                        }

                        .category {
                            font-size: var(--font-size-0);
                            color: var(--color-text-secondary);
                        }

                        .muscle-groups {
                            font-size: var(--font-size-0);
                            color: var(--color-text-secondary);
                        }
                    }
```

- [ ] **Step 2: Re-point the search input chrome**

In the same file (inside `@scope (.exercise-search)`), replace:

```css
                    input[type="search"] {
                        flex: 1;
                        padding: var(--size-2) var(--size-3);
                        border: 1px solid var(--gray-3);
                        border-radius: var(--radius-2);
                        font-size: var(--font-size-1);
                        background: var(--white);
                    }
```

with:

```css
                    input[type="search"] {
                        flex: 1;
                        padding: var(--size-2) var(--size-3);
                        border: 1px solid var(--color-border);
                        border-radius: var(--radius-2);
                        font-size: var(--font-size-1);
                        background: var(--color-surface-elevated);
                    }
```

- [ ] **Step 3: Re-point the `.alternative-exercises` muted text, swap button, no-results**

In the same file (inside `@scope (.alternative-exercises)`), replace:

```css
                        .option-details {
                            font-size: var(--font-size-0);
                            color: var(--gray-6);
                            margin-bottom: var(--size-3);
                        }

                        .category {
                            margin-bottom: var(--size-1);
                        }

                        .swap-button {
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

                        .no-results {
                            color: var(--gray-6);
                            padding: var(--size-4) 0;
                            text-align: center;
                        }
```

with:

```css
                        .option-details {
                            font-size: var(--font-size-0);
                            color: var(--color-text-secondary);
                            margin-bottom: var(--size-3);
                        }

                        .category {
                            margin-bottom: var(--size-1);
                        }

                        .swap-button {
                            background-color: var(--color-success-bg);
                            color: var(--color-success);
                            border: none;
                            border-radius: var(--radius-2);
                            padding: var(--size-2) var(--size-3);
                            font-weight: var(--font-weight-6);
                            cursor: pointer;
                            width: 100%;

                            &:hover {
                                background-color: color-mix(in oklab, var(--color-success-bg) 92%, black);
                            }
                        }

                        .no-results {
                            color: var(--color-text-secondary);
                            padding: var(--size-4) 0;
                            text-align: center;
                        }
```

- [ ] **Step 4: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/exercise-swap/exercise-swap.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 5: Verify the template still renders**

Run: `go test ./cmd/web/ -count=1`
Expected: PASS for the whole `cmd/web` package (no dedicated `exercise-swap` test exists — the package run exercises the routes via Playwright + handler tests, and per-request template parsing means any syntax breakage would fail there).

- [ ] **Step 6: Visual check (describe; cannot drive a browser here)**

Intended result on `/workouts/<date>/exercises/<id>/swap`: the current-exercise summary sits on a warm-sand surface with secondary stone text. The search input has a stone border on an elevated stone-cream background. Each alternative exercise's "Swap to this exercise" button reads as a soft sage-green pill (success-bg) that darkens on hover. The "No exercises match…" placeholder is secondary stone. Report as "not visually verified — describe only".

- [ ] **Step 7: Commit**

```bash
git add ui/templates/pages/exercise-swap/exercise-swap.gohtml
git commit -m "$(cat <<'EOF'
Warm exercise-swap page onto Stone — sage swap action, stone search

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Exercise add — search, add button, no-results

**Files:**
- Modify: `ui/templates/pages/exercise-add/exercise-add.gohtml`

- [ ] **Step 1: Re-point the search input chrome**

In `ui/templates/pages/exercise-add/exercise-add.gohtml`, replace:

```css
                    input[type="search"] {
                        flex: 1;
                        padding: var(--size-2) var(--size-3);
                        border: 1px solid var(--gray-3);
                        border-radius: var(--radius-2);
                        font-size: var(--font-size-1);
                        background: var(--white);
                    }
```

with:

```css
                    input[type="search"] {
                        flex: 1;
                        padding: var(--size-2) var(--size-3);
                        border: 1px solid var(--color-border);
                        border-radius: var(--radius-2);
                        font-size: var(--font-size-1);
                        background: var(--color-surface-elevated);
                    }
```

- [ ] **Step 2: Re-point the `.available-exercises` muted text, add button, no-results**

In the same file (inside `@scope (.available-exercises)`), replace:

```css
                        .option-details {
                            font-size: var(--font-size-0);
                            color: var(--gray-6);
                            margin-bottom: var(--size-3);
                        }

                        .category {
                            margin-bottom: var(--size-1);
                        }

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

                        .no-results {
                            color: var(--gray-6);
                            padding: var(--size-4) 0;
                            text-align: center;
                        }
```

with:

```css
                        .option-details {
                            font-size: var(--font-size-0);
                            color: var(--color-text-secondary);
                            margin-bottom: var(--size-3);
                        }

                        .category {
                            margin-bottom: var(--size-1);
                        }

                        .add-button {
                            background-color: var(--color-success-bg);
                            color: var(--color-success);
                            border: none;
                            border-radius: var(--radius-2);
                            padding: var(--size-2) var(--size-3);
                            font-weight: var(--font-weight-6);
                            cursor: pointer;
                            width: 100%;

                            &:hover {
                                background-color: color-mix(in oklab, var(--color-success-bg) 92%, black);
                            }
                        }

                        .no-results {
                            color: var(--color-text-secondary);
                            padding: var(--size-4) 0;
                            text-align: center;
                        }
```

- [ ] **Step 3: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/exercise-add/exercise-add.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 4: Verify the template still renders**

Run: `go test ./cmd/web/ -count=1`
Expected: PASS for the whole `cmd/web` package.

- [ ] **Step 5: Visual check (describe; cannot drive a browser here)**

Intended result on `/workouts/<date>/add-exercise`: identical treatment to `exercise-swap` — stone-bordered search, sage-green "Add this exercise" buttons, secondary-stone muted text. Report as "not visually verified — describe only".

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/exercise-add/exercise-add.gohtml
git commit -m "$(cat <<'EOF'
Warm exercise-add page onto Stone — sage add action, stone search

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Workout completion — subhead and difficulty buttons

**Files:**
- Modify: `ui/templates/pages/workout-completion/workout-completion.gohtml`

The "Well done!" heading already reads warmly from the base typography (Increment 1) — no extra celebratory accent needed; keep this a calm post-workout moment rather than introducing a second bright accent (Ember is reserved for the Focus-mode "LOG SET" CTA per spec, and adding a second Ember consumer would dilute the signal). The substitutions below warm the subhead and the difficulty rating buttons onto Stone with a soft Clay hover.

- [ ] **Step 1: Re-point the `.completion-heading` subhead**

In `ui/templates/pages/workout-completion/workout-completion.gohtml`, replace:

```css
                        > h2 {
                            font-size: var(--font-size-4);
                            font-weight: var(--font-weight-4);
                            color: var(--gray-7);
                        }
```

with:

```css
                        > h2 {
                            font-size: var(--font-size-4);
                            font-weight: var(--font-weight-4);
                            color: var(--color-text-secondary);
                        }
```

- [ ] **Step 2: Re-point the `.difficulty-choices` buttons onto Stone with a Clay hover**

In the same file (inside `@scope (.difficulty-choices)`), replace:

```css
                        button {
                            width: 100%;
                            justify-content: center;
                            padding: var(--size-4);
                            font-size: var(--font-size-3);
                            background-color: var(--white);
                            color: var(--gray-9);
                            border: var(--border-size-1) solid var(--gray-3);

                            &:hover {
                                background-color: var(--sky-1);
                                border-color: var(--sky-7);
                                color: var(--sky-9);
                            }
                        }
```

with:

```css
                        button {
                            width: 100%;
                            justify-content: center;
                            padding: var(--size-4);
                            font-size: var(--font-size-3);
                            background-color: var(--color-surface-elevated);
                            color: var(--color-text-primary);
                            border: var(--border-size-1) solid var(--color-border);

                            &:hover {
                                background-color: var(--clay-1);
                                border-color: var(--clay-3);
                                color: var(--clay-6);
                            }
                        }
```

- [ ] **Step 3: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/workout-completion/workout-completion.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 4: Verify the template still renders**

Run: `go test ./cmd/web/ -count=1`
Expected: PASS for the whole `cmd/web` package.

- [ ] **Step 5: Visual check (describe; cannot drive a browser here)**

Intended result on `/workouts/<date>` after completing the last set, redirecting to the completion form: "Well done!" reads as a large primary heading; "How did it feel today?" is secondary stone. Each difficulty button reads as a calm stone-cream pill with a primary stone label; hovering warms the surface to soft Clay with a darker clay border and clay-text label. No cool blue hover remains. Report as "not visually verified — describe only".

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/workout-completion/workout-completion.gohtml
git commit -m "$(cat <<'EOF'
Warm workout-completion page onto Stone — Clay-tinted button hover

Re-points the subhead onto --color-text-secondary and the difficulty
rating buttons onto an elevated stone surface with a soft Clay hover.
Keeps the page a calm post-workout moment — no second bright accent
(Ember stays reserved for the Focus-mode LOG SET CTA per spec).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Schedule editor — primary save button

**Files:**
- Modify: `ui/templates/pages/schedule/schedule.gohtml`

Note: this is the schedule **editor** (the standalone setup page at `/schedule`, used by new users to set their weekly workout duration), *not* the home-page weekly view at `pages/home/schedule.gohtml`, which Increment 2 already handled.

- [ ] **Step 1: Re-point the `.save-button` (primary "Start Tracking" CTA) onto Clay**

In `ui/templates/pages/schedule/schedule.gohtml`, replace:

```css
                .save-button {
                    width: 100%;
                    padding: var(--size-4) var(--size-6);
                    background: var(--sky-6);
                    color: var(--white);
                    border: none;
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-6);
                    font-size: var(--font-size-2);
                    cursor: pointer;
                    transition: background-color 0.2s ease;

                    &:hover {
                        background: var(--sky-7);
                    }

                    &:active {
                        background: var(--sky-8);
                    }
                }
```

with:

```css
                .save-button {
                    width: 100%;
                    padding: var(--size-4) var(--size-6);
                    background: var(--clay-4);
                    color: var(--color-surface-elevated);
                    border: none;
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-6);
                    font-size: var(--font-size-2);
                    cursor: pointer;
                    transition: background-color 0.2s ease;

                    &:hover {
                        background: color-mix(in oklab, var(--clay-4) 88%, black);
                    }

                    &:active {
                        background: color-mix(in oklab, var(--clay-4) 78%, black);
                    }
                }
```

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/schedule/schedule.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_schedulePOST_preservesRestNotificationsEnabled -v -count=1`
Expected: PASS (this test POSTs to `/schedule` and exercises the redirect path that renders this template on errors).

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `/schedule` (the setup page shown to a new user before they have a saved schedule): the duration-per-day list looks like the rest of the warm app; the "Start Tracking" CTA is a full-width clay-orange primary that darkens on hover/press. Report as "not visually verified — describe only" — this page is normally only seen on first signup.

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/schedule/schedule.gohtml
git commit -m "$(cat <<'EOF'
Warm schedule editor "Start Tracking" CTA onto Clay

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Privacy — headings, body, code, back link

**Files:**
- Modify: `ui/templates/pages/privacy/privacy.gohtml`

- [ ] **Step 1: Re-point headings, paragraphs, list items, code, and back-link**

In `ui/templates/pages/privacy/privacy.gohtml`, replace the page-scope block:

```css
                h2 {
                    font-size: var(--font-size-4);
                    font-weight: var(--font-weight-3);
                    margin-top: var(--size-6);
                    margin-bottom: var(--size-4);
                    color: var(--gray-8);
                }

                p {
                    margin-bottom: var(--size-4);
                    color: var(--gray-7);
                }

                ul {
                    margin-bottom: var(--size-4);
                    padding-left: var(--size-5);
                }

                li {
                    margin-bottom: var(--size-2);
                    color: var(--gray-7);
                }

                code {
                    background: var(--gray-1);
                    padding: var(--size-1) var(--size-2);
                    border-radius: var(--radius-2);
                    font-family: monospace;
                    font-size: var(--font-size-1);
                }

                .back-link {
                    display: inline-block;
                    margin-bottom: var(--size-4);
                    color: var(--sky-6);
                    text-decoration: underline;
                }

                .back-link:hover {
                    color: var(--sky-7);
                }
```

with:

```css
                h2 {
                    font-size: var(--font-size-4);
                    font-weight: var(--font-weight-3);
                    margin-top: var(--size-6);
                    margin-bottom: var(--size-4);
                    color: var(--color-text-primary);
                }

                p {
                    margin-bottom: var(--size-4);
                    color: var(--color-text-secondary);
                }

                ul {
                    margin-bottom: var(--size-4);
                    padding-left: var(--size-5);
                }

                li {
                    margin-bottom: var(--size-2);
                    color: var(--color-text-secondary);
                }

                code {
                    background: var(--stone-1);
                    padding: var(--size-1) var(--size-2);
                    border-radius: var(--radius-2);
                    font-family: monospace;
                    font-size: var(--font-size-1);
                }

                .back-link {
                    display: inline-block;
                    margin-bottom: var(--size-4);
                    color: var(--clay-4);
                    text-decoration: underline;
                }

                .back-link:hover {
                    color: color-mix(in oklab, var(--clay-4) 88%, black);
                }
```

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/privacy/privacy.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -count=1`
Expected: PASS.

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `/privacy`: section headings are primary stone, body copy and bullets are secondary stone, inline `<code>` reads on a warm-sand chip. The "← Back to Home" link is clay-orange, darkening on hover. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/privacy/privacy.gohtml
git commit -m "$(cat <<'EOF'
Warm privacy page typography and back link onto Stone/Clay

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Generic error page — container, text, button

**Files:**
- Modify: `ui/templates/pages/error/error.gohtml`

- [ ] **Step 1: Re-point the error-page surface and text onto Stone**

In `ui/templates/pages/error/error.gohtml`, replace:

```css
            .error-container {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                min-height: 100vh;
                padding: var(--size-4);
                text-align: center;
                background: var(--gray-0);
            }

            .error-content {
                max-width: var(--size-15);
                padding: var(--size-8) var(--size-6);
            }

            .error-title {
                font-size: var(--font-size-6);
                font-weight: var(--font-weight-7);
                color: var(--gray-9);
                margin-bottom: var(--size-4);
                line-height: var(--font-lineheight-1);
            }

            .error-message {
                font-size: var(--font-size-2);
                color: var(--gray-6);
                margin-bottom: var(--size-6);
                line-height: var(--font-lineheight-3);
            }

            .error-actions {
                gap: var(--size-3);
                justify-content: center;
            }

            .home-link {
                background-color: var(--gray-6);
                text-decoration: none;

                &:hover {
                    background-color: var(--gray-7);
                }
            }
```

with:

```css
            .error-container {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                min-height: 100vh;
                padding: var(--size-4);
                text-align: center;
                background: var(--color-surface);
            }

            .error-content {
                max-width: var(--size-15);
                padding: var(--size-8) var(--size-6);
            }

            .error-title {
                font-size: var(--font-size-6);
                font-weight: var(--font-weight-7);
                color: var(--color-text-primary);
                margin-bottom: var(--size-4);
                line-height: var(--font-lineheight-1);
            }

            .error-message {
                font-size: var(--font-size-2);
                color: var(--color-text-secondary);
                margin-bottom: var(--size-6);
                line-height: var(--font-lineheight-3);
            }

            .error-actions {
                gap: var(--size-3);
                justify-content: center;
            }

            .home-link {
                background-color: var(--stone-6);
                text-decoration: none;

                &:hover {
                    background-color: var(--stone-7);
                }
            }
```

(Rationale: the "Go Home" link sits next to a "Retry" `<button>` that inherits the Clay-themed `.btn` base, so making `.home-link` a secondary stone-toned button avoids two identical primaries on the same row. The `.btn` class on the anchor still provides the base shape; the override here only changes the `background-color`.)

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/error/error.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -count=1`
Expected: PASS.

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on a 500 error: full-screen warm-stone surface with a centred card, primary-stone title "Something went wrong", secondary-stone message, a Retry primary button (Clay from `.btn`) sitting next to a "Go Home" stone-toned secondary. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/error/error.gohtml
git commit -m "$(cat <<'EOF'
Warm generic error page onto Stone — secondary Go Home next to Retry

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: 404 not-found — container, text, primary and secondary buttons

**Files:**
- Modify: `ui/templates/pages/not-found/not-found.gohtml`

- [ ] **Step 1: Re-point the not-found surface, text, and buttons**

In `ui/templates/pages/not-found/not-found.gohtml`, replace:

```css
            .not-found-container {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                min-height: 100vh;
                padding: var(--size-4);
                text-align: center;
                background: var(--gray-0);
            }

            .not-found-content {
                max-width: var(--size-15);
                padding: var(--size-8) var(--size-6);
            }

            .not-found-title {
                font-size: var(--font-size-6);
                font-weight: var(--font-weight-7);
                color: var(--gray-9);
                margin-bottom: var(--size-2);
                line-height: var(--font-lineheight-1);
            }

            .not-found-subtitle {
                font-size: var(--font-size-4);
                font-weight: var(--font-weight-5);
                color: var(--gray-7);
                margin-bottom: var(--size-4);
                line-height: var(--font-lineheight-1);
            }

            .not-found-message {
                font-size: var(--font-size-2);
                color: var(--gray-6);
                margin-bottom: var(--size-6);
                line-height: var(--font-lineheight-3);
            }

            .not-found-actions {
                gap: var(--size-3);
                justify-content: center;
            }

            .home-link {
                background-color: var(--sky-6);
                text-decoration: none;

                &:hover {
                    background-color: var(--sky-7);
                }
            }

            .back-link {
                background-color: var(--gray-6);
                text-decoration: none;

                &:hover {
                    background-color: var(--gray-7);
                }
            }
```

with:

```css
            .not-found-container {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                min-height: 100vh;
                padding: var(--size-4);
                text-align: center;
                background: var(--color-surface);
            }

            .not-found-content {
                max-width: var(--size-15);
                padding: var(--size-8) var(--size-6);
            }

            .not-found-title {
                font-size: var(--font-size-6);
                font-weight: var(--font-weight-7);
                color: var(--color-text-primary);
                margin-bottom: var(--size-2);
                line-height: var(--font-lineheight-1);
            }

            .not-found-subtitle {
                font-size: var(--font-size-4);
                font-weight: var(--font-weight-5);
                color: var(--color-text-secondary);
                margin-bottom: var(--size-4);
                line-height: var(--font-lineheight-1);
            }

            .not-found-message {
                font-size: var(--font-size-2);
                color: var(--color-text-secondary);
                margin-bottom: var(--size-6);
                line-height: var(--font-lineheight-3);
            }

            .not-found-actions {
                gap: var(--size-3);
                justify-content: center;
            }

            .home-link {
                background-color: var(--clay-4);
                text-decoration: none;

                &:hover {
                    background-color: color-mix(in oklab, var(--clay-4) 88%, black);
                }
            }

            .back-link {
                background-color: var(--stone-6);
                text-decoration: none;

                &:hover {
                    background-color: var(--stone-7);
                }
            }
```

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/not-found/not-found.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_notFound -v -count=1`
Expected: PASS.

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on a 404: warm-stone full-screen surface; "404" title in primary stone; "Page Not Found" subtitle and message in secondary stone; "Go Home" primary in clay-orange (with `color-mix` darken on hover); "Go Back" secondary in stone-toned grey-brown. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/not-found/not-found.gohtml
git commit -m "$(cat <<'EOF'
Warm 404 not-found page onto Stone — Clay primary, stone secondary

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Workout-not-found — copy and primary CTA

**Files:**
- Modify: `ui/templates/pages/workout-not-found/workout-not-found.gohtml`

- [ ] **Step 1: Re-point body copy and the "Back to Home" primary onto Stone/Clay**

In `ui/templates/pages/workout-not-found/workout-not-found.gohtml`, replace:

```css
                .message p {
                    color: var(--gray-7);
                    font-size: var(--font-size-1);
                    line-height: 1.6;
                }

                .actions {
                    width: 100%;
                    max-width: 200px;
                }

                .primary-button {
                    background-color: var(--lime-6);
                    color: var(--white);
                    border: none;
                    border-radius: var(--radius-2);
                    padding: var(--size-3) var(--size-4);
                    font-weight: var(--font-weight-6);
                    cursor: pointer;
                    text-decoration: none;
                    display: inline-block;
                    font-size: var(--font-size-1);

                    &:hover {
                        background-color: var(--lime-7);
                    }
                }
```

with:

```css
                .message p {
                    color: var(--color-text-secondary);
                    font-size: var(--font-size-1);
                    line-height: 1.6;
                }

                .actions {
                    width: 100%;
                    max-width: 200px;
                }

                .primary-button {
                    background-color: var(--clay-4);
                    color: var(--color-surface-elevated);
                    border: none;
                    border-radius: var(--radius-2);
                    padding: var(--size-3) var(--size-4);
                    font-weight: var(--font-weight-6);
                    cursor: pointer;
                    text-decoration: none;
                    display: inline-block;
                    font-size: var(--font-size-1);

                    &:hover {
                        background-color: color-mix(in oklab, var(--clay-4) 88%, black);
                    }
                }
```

(Rationale: the existing `--lime-6/7` was being used as a *primary* CTA — "Back to Home" — not as success state semantics. Clay matches every other primary in the app.)

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/workout-not-found/workout-not-found.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_workoutNotFound -v -count=1`
Expected: PASS.

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `/workouts/<date>` for an off-schedule date: the explanatory paragraph reads in secondary stone; the "Back to Home" anchor is a clay-orange primary that darkens on hover. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/workout-not-found/workout-not-found.gohtml
git commit -m "$(cat <<'EOF'
Warm workout-not-found primary CTA onto Clay; stone body copy

The lime CTA was being used as a primary, not as success semantics —
Clay matches every other primary in the app.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Maintenance mode — surface, text, warning band

**Files:**
- Modify: `ui/templates/pages/maintenance/maintenance.gohtml`

- [ ] **Step 1: Re-point the maintenance surface, headings, and warning band**

In `ui/templates/pages/maintenance/maintenance.gohtml`, replace:

```css
            .maintenance-container {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                min-height: 100vh;
                padding: var(--size-4);
                text-align: center;
                background: var(--gray-0);
            }

            .maintenance-content {
                max-width: var(--size-15);
                padding: var(--size-8) var(--size-6);
                border-color: var(--yellow-5);
                box-shadow: var(--shadow-2);
            }

            .maintenance-icon {
                font-size: var(--font-size-8);
                color: var(--yellow-7);
                margin-bottom: var(--size-4);
                line-height: 1;
            }

            .maintenance-title {
                font-size: var(--font-size-6);
                font-weight: var(--font-weight-7);
                color: var(--gray-9);
                margin-bottom: var(--size-2);
                line-height: var(--font-lineheight-1);
            }

            .maintenance-subtitle {
                font-size: var(--font-size-4);
                font-weight: var(--font-weight-5);
                color: var(--yellow-9);
                margin-bottom: var(--size-4);
                line-height: var(--font-lineheight-1);
            }

            .maintenance-message {
                font-size: var(--font-size-2);
                color: var(--gray-6);
                margin-bottom: var(--size-6);
                line-height: var(--font-lineheight-3);
            }

            .maintenance-notice {
                padding: var(--size-3);
                background: var(--yellow-1);
                border: var(--border-size-1) solid var(--yellow-3);
                border-radius: var(--radius-2);
                font-size: var(--font-size-1);
                color: var(--yellow-11);
                margin-top: var(--size-4);
            }
```

with:

```css
            .maintenance-container {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                min-height: 100vh;
                padding: var(--size-4);
                text-align: center;
                background: var(--color-surface);
            }

            .maintenance-content {
                max-width: var(--size-15);
                padding: var(--size-8) var(--size-6);
                border-color: var(--color-warning);
                box-shadow: var(--shadow-2);
            }

            .maintenance-icon {
                font-size: var(--font-size-8);
                color: var(--color-warning);
                margin-bottom: var(--size-4);
                line-height: 1;
            }

            .maintenance-title {
                font-size: var(--font-size-6);
                font-weight: var(--font-weight-7);
                color: var(--color-text-primary);
                margin-bottom: var(--size-2);
                line-height: var(--font-lineheight-1);
            }

            .maintenance-subtitle {
                font-size: var(--font-size-4);
                font-weight: var(--font-weight-5);
                color: var(--color-warning);
                margin-bottom: var(--size-4);
                line-height: var(--font-lineheight-1);
            }

            .maintenance-message {
                font-size: var(--font-size-2);
                color: var(--color-text-secondary);
                margin-bottom: var(--size-6);
                line-height: var(--font-lineheight-3);
            }

            .maintenance-notice {
                padding: var(--size-3);
                background: var(--color-warning-bg);
                border: var(--border-size-1) solid var(--color-warning);
                border-radius: var(--radius-2);
                font-size: var(--font-size-1);
                color: var(--color-warning);
                margin-top: var(--size-4);
            }
```

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/maintenance/maintenance.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_maintenanceMode_integration -v -count=1`
Expected: PASS.

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result when the maintenance feature flag is on: full-screen warm-stone surface; centred card with a warm-amber `--color-warning` border accent; "Maintenance Mode" title in primary stone; "System Under Maintenance" subtitle in warm amber; body copy in secondary stone; the bottom notice band reads in `--color-warning-bg` with amber border and amber text. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/maintenance/maintenance.gohtml
git commit -m "$(cat <<'EOF'
Warm maintenance page onto Stone — --color-warning amber accent

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Styleguide — dev-styling holdouts

**Files:**
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml`

The 7 holdouts in this file are all *page-styling* — the dividers, accent bars, and shadow-grid container for the dev styleguide page itself. They have nothing to do with the page's role of *displaying* tokens (the color, size, font-size, font-weight, and radius swatches are generated declaratively from `.ColorTokens` / `.Sizes` / etc. data ranges starting at line 158 and remain unchanged).

- [ ] **Step 1: Re-point the dev styleguide's page-styling holdouts**

In `ui/templates/pages/styleguide/styleguide.gohtml`, replace:

```css
                .scale-row {
                    display: grid;
                    grid-template-columns: 6rem 1fr;
                    align-items: center;
                    gap: var(--size-3);
                    padding: var(--size-2) 0;
                    border-bottom: var(--border-size-1) solid var(--gray-1);
                }
```

with:

```css
                .scale-row {
                    display: grid;
                    grid-template-columns: 6rem 1fr;
                    align-items: center;
                    gap: var(--size-3);
                    padding: var(--size-2) 0;
                    border-bottom: var(--border-size-1) solid var(--stone-1);
                }
```

Then replace:

```css
                .scale-bar {
                    height: var(--size-3);
                    background: var(--sky-5);
                    border-radius: var(--radius-1);
                }
```

with:

```css
                .scale-bar {
                    height: var(--size-3);
                    background: var(--clay-3);
                    border-radius: var(--radius-1);
                }
```

Then replace:

```css
                .radius-sample {
                    background: var(--sky-2);
                    height: var(--size-9);
                    display: flex;
                    align-items: end;
                    justify-content: center;
                    padding: var(--size-2);
                    font-family: var(--font-mono);
                    font-size: var(--font-size-0);
                    color: var(--sky-9);
                }
```

with:

```css
                .radius-sample {
                    background: var(--clay-1);
                    height: var(--size-9);
                    display: flex;
                    align-items: end;
                    justify-content: center;
                    padding: var(--size-2);
                    font-family: var(--font-mono);
                    font-size: var(--font-size-0);
                    color: var(--clay-6);
                }
```

Then replace:

```css
                .shadow-grid {
                    display: grid;
                    grid-template-columns: repeat(auto-fill, minmax(10rem, 1fr));
                    gap: var(--size-5);
                    padding: var(--size-3);
                    background: var(--gray-1);
                    border-radius: var(--radius-2);
                }

                .shadow-sample {
                    background: var(--white);
                    border-radius: var(--radius-2);
                    height: var(--size-9);
                    display: flex;
                    align-items: end;
                    justify-content: center;
                    padding: var(--size-2);
                    font-family: var(--font-mono);
                    font-size: var(--font-size-0);
                    color: var(--gray-7);
                }
```

with:

```css
                .shadow-grid {
                    display: grid;
                    grid-template-columns: repeat(auto-fill, minmax(10rem, 1fr));
                    gap: var(--size-5);
                    padding: var(--size-3);
                    background: var(--stone-1);
                    border-radius: var(--radius-2);
                }

                .shadow-sample {
                    background: var(--color-surface-elevated);
                    border-radius: var(--radius-2);
                    height: var(--size-9);
                    display: flex;
                    align-items: end;
                    justify-content: center;
                    padding: var(--size-2);
                    font-family: var(--font-mono);
                    font-size: var(--font-size-0);
                    color: var(--color-text-secondary);
                }
```

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/styleguide/styleguide.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v -count=1`
Expected: PASS (the test asserts structural elements like sample classes; it does not assert specific hex/token values, so the swap is invisible to it).

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `/dev/styleguide`: the spacing-scale rows have warm-sand dividers and clay-tinted size bars; the radius samples sit on soft-clay chips with dark-clay labels; the shadows section sits on a warm-stone background, with each shadow sample reading as a cream-coloured card with a secondary-stone label. The color-token swatch grid (`.ColorTokens` data) is unchanged and continues to show every defined token. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/styleguide/styleguide.gohtml
git commit -m "$(cat <<'EOF'
Warm /dev/styleguide page-styling onto Stone — Clay accents, stone dividers

Re-points the page's own chrome (scale-row dividers, scale-bar fill,
radius-sample chip, shadow-grid container, shadow-sample card) onto
Stone/Clay semantic tokens. The dynamically-generated color swatch
grid is unchanged.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Retire the `--gray-*` alias definitions from `main.css`

**Files:**
- Modify: `ui/static/main.css`

- [ ] **Step 1: Confirm no template still references `--gray-*`**

Run: `grep -rn 'var(--gray-' ui/templates/`
Expected: no output (exit status 1) — Tasks 1-12 have renamed every call site.

If this fails: do **not** delete the aliases. Re-run the per-file `grep` gates from earlier tasks against the file(s) still listed, fix the missed references, commit, then retry this step.

- [ ] **Step 2: Delete the alias block from `main.css`**

In `ui/static/main.css`, locate the `@layer props` block containing the `--gray-*` aliases — currently around lines 151-164:

```css
        /*
         * Transitional --gray-* aliases pointing onto Stone. New code should reach for
         * --stone-* or the semantic --color-text-* / --color-surface tokens; these are
         * renamed to --stone-* as pages are polished; the --gray-* names are
         * scheduled to retire at the end of Increment 5.
         */
        --gray-0: var(--stone-0);
        --gray-1: var(--stone-1);
        --gray-2: var(--stone-2);
        --gray-3: var(--stone-3);
        --gray-4: var(--stone-4);
        --gray-5: var(--stone-5);
        --gray-6: var(--stone-6);
        --gray-7: var(--stone-7);
        --gray-8: var(--stone-8);
        --gray-9: var(--stone-9);
        --gray-10: var(--stone-10);
```

Delete the entire block (comment + 11 declarations) so that `--gray-*` is no longer a defined custom property anywhere in the codebase. Re-check that line numbers haven't drifted since the survey — `grep -n 'gray-' ui/static/main.css` should now return zero matches.

- [ ] **Step 3: Update `ui/templates/CLAUDE.md` to retire the alias mention**

In `ui/templates/CLAUDE.md`, find the "Color System" subsection's `--gray-*` bullet — currently:

```markdown
- **Gray colors**: `--gray-0` through `--gray-10` — transitional aliases onto Stone.
  Don't reach for these in new code; use `--stone-*`. Existing `--gray-*` call sites
  are renamed to `--stone-*` as each page is polished.
```

Delete that bullet entirely. The Stone, Clay, Ember, and semantic-token bullets above it are now the sole guidance.

- [ ] **Step 4: Confirm no source file still references `--gray-*`**

Run:
```bash
grep -rn 'gray-' ui/static/ ui/templates/ cmd/ internal/ docs/superpowers/specs/ docs/superpowers/plans/2026-05-15* 2>/dev/null
```
Expected: only matches inside `docs/superpowers/specs/` and `docs/superpowers/plans/` (which document the historical alias) — no matches in any `ui/`, `cmd/`, or `internal/` source file.

Also run: `grep -rn '\-\-gray\-' ui/static/main.css`
Expected: no output (exit status 1).

- [ ] **Step 5: Build to confirm nothing depends on the alias**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 6: Render-check that the visual outcome is unchanged**

Run: `go test ./cmd/web/ -count=1`
Expected: PASS — all templates still render; any rule that *used* `--gray-*` was renamed in Tasks 1-12 and the alias deletion is now a no-op for the rendered output. (Browser would show identical pixels; we cannot verify that visually here.)

- [ ] **Step 7: Commit**

```bash
git add ui/static/main.css ui/templates/CLAUDE.md
git commit -m "$(cat <<'EOF'
Retire the --gray-* transitional aliases from main.css

Every call site across pages/** templates has been renamed to
--stone-* or the appropriate semantic token across Increments 1-5.
This removes the alias definitions and the CLAUDE.md guidance bullet
referencing them, completing the Stone palette rollout.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Full-suite verification and visual QA pass

**Files:** none (verification only)

- [ ] **Step 1: Confirm the increment's file list is fully repainted**

Run:
```bash
grep -rnE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' \
  ui/templates/pages/preferences/preferences.gohtml \
  ui/templates/pages/exercise-info/exercise-info.gohtml \
  ui/templates/pages/exercise-swap/exercise-swap.gohtml \
  ui/templates/pages/exercise-add/exercise-add.gohtml \
  ui/templates/pages/workout-completion/workout-completion.gohtml \
  ui/templates/pages/schedule/schedule.gohtml \
  ui/templates/pages/privacy/privacy.gohtml \
  ui/templates/pages/error/error.gohtml \
  ui/templates/pages/not-found/not-found.gohtml \
  ui/templates/pages/workout-not-found/workout-not-found.gohtml \
  ui/templates/pages/maintenance/maintenance.gohtml \
  ui/templates/pages/styleguide/styleguide.gohtml
```
Expected: no output (exit status 1).

- [ ] **Step 2: Confirm the whole `ui/` tree has zero raw-ramp / `--gray-*` / `--white` holdouts**

Run:
```bash
grep -rnE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/ ui/static/main.css
```
Expected: no output (exit status 1) — every page is Stone, and the `--gray-*` alias names are gone from both `@layer props` and every call site.

- [ ] **Step 3: Run the full CI suite**

Run: `make ci`
Expected: `init`, `build`, `lint-fix`, `test`, `sec` all pass. If `lint-fix` makes formatting changes, review them, `git add` them, and commit with message `Apply lint-fix formatting`.

- [ ] **Step 4: Visual QA across the touched pages**

Run: `make dev`, then open and eyeball:
- `/preferences` — warm clay save/export primaries, stone logout, red destructive zone, warm-amber notice band.
- `/workouts/<date>/exercises/<id>/info` — clay primary muscle accent, sage category, clay admin edit.
- `/workouts/<date>/exercises/<id>/swap` and `/workouts/<date>/add-exercise` — sage action buttons, stone search.
- Any completed workout's completion screen — clay-tinted hover on difficulty buttons.
- `/schedule` (logged out / new user) — clay primary "Start Tracking".
- `/privacy` — primary-stone headings, secondary-stone body, clay back-link.
- A nonexistent path → 404 — warm-stone surface, clay primary, stone secondary.
- An off-schedule workout URL → workout-not-found — clay "Back to Home".
- A 500 path (or temporarily force one) → generic error — warm-stone surface, stone secondary "Go Home".
- Maintenance mode (flag on) → `/maintenance` — warm-stone surface, amber `--color-warning` accents.
- `/dev/styleguide` — clay-accent scale bars, warm-stone shadow grid.

Expected: every page reads as deliberate Stone. If a browser cannot be driven in this environment, state explicitly that the visual QA was not performed and that verification rests on the per-task `grep` gates, the per-task render tests, and `make ci`.

- [ ] **Step 5: Final confirmation**

Confirm `git status` is clean and `git log --oneline` shows the Task 1-13 commits (plus any lint-fix commit). Increment 5 is complete: every `pages/**` template is on Stone, and the `--gray-*` transitional alias is fully retired.

---

## Self-review notes

- **Spec coverage:** Increment 5 of the spec lists `exercise-info.gohtml` + `progress-chart.gohtml` (Task 2 covers exercise-info; `progress-chart.gohtml` has no scoped `<style>` and needs no work — documented in "Out of scope"), `exercise-swap.gohtml` (Task 3), `exercise-add.gohtml` (Task 4), `workout-completion.gohtml` (Task 5), `preferences.gohtml` (Task 1), schedule editor (Task 6 — `pages/schedule/schedule.gohtml`, disambiguated from `pages/home/schedule.gohtml` which Increment 2 covered), `privacy.gohtml` (Task 7), admin pages (verified clean in survey — none touched), `error.gohtml` / `not-found.gohtml` / `workout-not-found.gohtml` / `maintenance.gohtml` (Tasks 8-11), and the spec's stated outcome "the `--gray-*` token name is fully retired" (Task 13). The styleguide (Task 12) is not listed by name but is required for the `--gray-*` retirement to succeed. All covered.
- **Placeholder scan:** every step contains explicit before/after CSS or an exact command. No "TBD", no "similar to Task N", no unspecified validation.
- **Token consistency:** every substitution uses tokens already proven present in `main.css` after Increment 1 — `--clay-1..6`, `--stone-0..10`, `--color-surface`, `--color-surface-elevated`, `--color-border`, `--color-text-primary/secondary`, `--color-success/-bg`, `--color-warning/-bg`, `--color-error/-bg`. The `color-mix(in oklab, ...)` pattern matches the Increment 4 / 4.1 convention (~7 prior sites; this increment adds ~10 more, pushing the total past the ~10-site threshold from the lessons section).
- **The `color-mix` convention** is now used in this plan ~10 times across 5 buttons (save/export/delete/clay-primary CTA / Clay-bordered admin chip / link hover). Per the briefing's lessons section, that crosses the threshold for documenting in `ui/templates/CLAUDE.md` under "Color Usage Patterns". A small follow-up note: this plan does *not* touch CLAUDE.md mid-increment (apart from Task 13 retiring the `--gray-*` bullet); a separate cleanup PR is the natural next step *after* Increment 5 merges, mentioned here so the user can decide whether to fold it into Increment 6 or schedule a small standalone doc PR.
- **Verification shape:** matches Increments 2-4 — per-file `grep` gate + per-task render test + described visual check + `make ci` at the end. No fake unit tests for hex values.
- **No DOM changes:** every task swaps only CSS custom-property values (and the `--gray-*` alias deletion) inside scoped `<style>` blocks. No class names, element structure, button text, `data-*` attributes, or template syntax change — so existing `goquery` / Playwright DOM tests stay green untouched.
- **Out of scope (deferred):** the Increment 4 time-based oversized-numerals defect noted in the briefing is **not** folded in here — no `pages/exerciseset/*` file is touched. A separate Increment 4.2 is the right scope.
- **Worktree + subagent CWD:** as noted in the briefing's lessons, when this plan is executed via subagent-driven-development each subagent prompt MUST open with an explicit `cd /home/martin/petrapp/.claude/worktrees/<name>` and verification (pwd, `git rev-parse --show-toplevel`, `git branch --show-current`, `git log -1 --oneline`) — subagents do not inherit the EnterWorktree CWD and have committed on `main` by mistake in a prior increment. The driver session is responsible for adding that block to every per-task prompt; the plan itself does not need to repeat it inside each task.
