# Aesthetics Uplift — Increment 2: Home & Schedule — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Polish the highest-traffic landing surface — the home/schedule page and its components — so it fully realizes the warm "Stone" visual direction.

**Architecture:** Increment 1 already retuned the token layer (Stone/Clay/Ember ramps, re-pointed semantic tokens, `--gray-*` aliased onto Stone). This increment is per-page CSS polish only: each of the five home-page templates has its scoped `<style>` blocks cleaned up, its raw-ramp holdouts (`--sky/lime/yellow/red-*`) re-pointed onto the warm semantic / Stone / Clay tokens, and its `--gray-*` references renamed to `--stone-*`. No DOM structure, no class names, no Go code changes — so the existing `goquery` / Playwright DOM tests stay green untouched.

**Tech Stack:** CSS (custom properties, `@layer`, `@scope`), Go `html/template`, Go (`cmd/web` handler + `goquery`-based e2e tests).

**Spec:** `docs/superpowers/specs/2026-05-14-aesthetics-uplift-design.md` (Increment 2).

---

## File Structure

### Modified files

| File | Change |
|---|---|
| `ui/templates/pages/home/day-cards.gohtml` | Re-point the `.day-card` and `.status-indicator` per-status tints (`--sky/lime/yellow/red-*`) onto the warm semantic tokens; rename `--gray-*` → `--stone-*` / semantic. |
| `ui/templates/pages/home/progress-bar.gohtml` | Re-point the track / fill / text colours onto Stone + warm semantic tokens. |
| `ui/templates/pages/home/muscle-balance.gohtml` | Re-point the status bar/legend colours (`--yellow/lime/red/sky-6`) onto the warm semantic tokens; rename `--gray-*` → `--stone-*` / semantic. |
| `ui/templates/pages/home/schedule.gohtml` | Re-point the `.menu-button` chrome and the deload `.week-chip` (`--white`, `--gray-*`, `--sky-*`) onto Stone / Clay / semantic tokens. |
| `ui/templates/pages/home/unauthenticated.gohtml` | Rename the `--gray-*` references in the landing copy / footer onto Stone / semantic tokens. |

### Out of scope (untouched)

- `ui/templates/pages/schedule/schedule.gohtml` — that is the **schedule editor**, which the spec assigns to Increment 5 ("schedule editor" under Secondary pages). Increment 2's "schedule.gohtml" is the home-page weekly view, `pages/home/schedule.gohtml`.
- `ui/static/main.css`, shared components, every other `pages/**` template — their `--gray-*` call sites keep warming automatically via the Increment 1 aliases until their own increment.
- All Go handlers and tests — no DOM or class-name changes are made, so no test edits are needed.

---

## Token mapping reference

The single source of truth for every substitution in this increment. All target tokens are defined in `ui/static/main.css` (verified present after Increment 1).

**Raw-ramp holdouts → warm semantic tokens** (status semantics):

| Old raw token(s) | Meaning | New token |
|---|---|---|
| `--sky-*` (today / active highlight) | active / "today" | `--color-surface-active`, `--clay-3` border, `--clay-1` / `--clay-6` badge |
| `--lime-*` (completed, on-target) | success | `--color-success`, `--color-success-bg` |
| `--yellow-*` (in progress, under-target) | warning / in-progress | `--color-warning`, `--color-warning-bg` |
| `--red-*` (past-incomplete, over-target) | error / over | `--color-error`, `--color-error-bg` |
| `--sky-6` (no-target, informational) | info | `--color-info` |

**`--gray-N` → `--stone-N`** literal rename, except where a semantic token is the clearer fit:

| Old | New |
|---|---|
| `--gray-0` | `--stone-0` |
| `--gray-1` | `--stone-1` |
| `--gray-2` | `--stone-2` |
| `--gray-3` | `--color-border` (when used as a border) / `--stone-3` (otherwise) |
| `--gray-4` | `--stone-4` |
| `--gray-6` (muted text) | `--color-text-secondary` |
| `--gray-6` (non-text, e.g. dashed outline) | `--stone-6` |
| `--gray-7` (secondary text) | `--color-text-secondary` |
| `--gray-7` (non-text) | `--stone-7` |
| `--gray-8` (text) | `--color-text-primary` |
| `--gray-9` (text) | `--color-text-primary` |
| `--gray-9` (non-text, e.g. tick mark) | `--stone-9` |
| `--white` (surface) | `--color-surface-elevated` |

---

## Sequencing rationale

The five files are independent — each is a self-contained scoped-style cleanup, verifiable on its own. They are ordered by visual prominence on the authenticated home page (day cards → progress bar → muscle balance → page shell) with the unauthenticated landing last. Task 6 is the full-suite gate.

**A note on verification:** CSS token/colour changes have no unit test — there is no compiler or linter for scoped `<style>` blocks. Each task is verified by (a) `go test ./cmd/web/ -run Test_application_home -v`, which renders the home page (authenticated schedule view *and* the unauthenticated landing) end-to-end through `goquery` — a template typo or broken `@scope` block fails here; and (b) a described visual check. Task 6 runs the full `make ci`. This is the appropriate verification shape for a per-page palette repaint; **do not invent fake unit tests for hex values.** The user cannot drive a browser in this environment — perform the automated checks and explicitly report which described visual checks were not performed rather than claiming them.

---

## Task 1: Day cards — warm the per-status tints

**Files:**
- Modify: `ui/templates/pages/home/day-cards.gohtml`

- [ ] **Step 1: Re-point the `.day-card` per-status block**

In `ui/templates/pages/home/day-cards.gohtml`, replace the `.day-card` rule's status branches:

```css
                &[data-status="today"] {
                    border-color: var(--sky-6);
                    background: var(--sky-0);
                    box-shadow: 0 2px 8px var(--sky-2);
                }

                &[data-status="completed"] {
                    border-color: var(--lime-6);
                    background: var(--lime-0);
                }

                &[data-status="in_progress"] {
                    border-color: var(--yellow-6);
                    background: var(--yellow-0);
                }

                &[data-status="past-incomplete"] {
                    border-color: var(--red-6);
                    background: var(--red-1);
                }

                &[data-status="unscheduled"] {
                    border-color: var(--gray-4);
                    background: var(--gray-1);
                    border-style: dashed;
                    border-width: 1px;
                    opacity: 0.7;
                }

                &[data-status="upcoming"] {
                    border-color: var(--gray-3);
                    background: var(--gray-0);
                }
```

with:

```css
                &[data-status="today"] {
                    border-color: var(--clay-3);
                    background: var(--color-surface-active);
                    box-shadow: var(--shadow-2);
                }

                &[data-status="completed"] {
                    border-color: var(--color-success);
                    background: var(--color-success-bg);
                }

                &[data-status="in_progress"] {
                    border-color: var(--color-warning);
                    background: var(--color-warning-bg);
                }

                &[data-status="past-incomplete"] {
                    border-color: var(--color-error);
                    background: var(--color-error-bg);
                }

                &[data-status="unscheduled"] {
                    border-color: var(--stone-4);
                    background: var(--stone-1);
                    border-style: dashed;
                    border-width: 1px;
                    opacity: 0.7;
                }

                &[data-status="upcoming"] {
                    border-color: var(--color-border);
                    background: var(--color-surface-elevated);
                }
```

- [ ] **Step 2: Re-point `.day-date` and the `.status-indicator` per-status block**

In the same file, replace:

```css
            .day-date {
                color: var(--gray-6);
                font-size: var(--font-size-0);
            }
```

with:

```css
            .day-date {
                color: var(--color-text-secondary);
                font-size: var(--font-size-0);
            }
```

Then replace the `.status-indicator` status branches:

```css
                &[data-status="completed"] {
                    background: var(--lime-2);
                    color: var(--lime-9);
                }

                &[data-status="in_progress"] {
                    background: var(--yellow-2);
                    color: var(--yellow-11);
                }

                &[data-status="upcoming"] {
                    background: var(--gray-3);
                    color: var(--gray-9);
                }

                &[data-status="past-incomplete"] {
                    background: var(--red-2);
                    color: var(--red-9);
                }

                &[data-status="unscheduled"] {
                    background: var(--gray-2);
                    color: var(--gray-9);
                    border: 1px dashed var(--gray-5);
                }

                &[data-status="today"] {
                    background: var(--sky-2);
                    color: var(--sky-9);
                    font-weight: var(--font-weight-6);
                }
```

with:

```css
                &[data-status="completed"] {
                    background: var(--color-success-bg);
                    color: var(--color-success);
                }

                &[data-status="in_progress"] {
                    background: var(--color-warning-bg);
                    color: var(--color-warning);
                }

                &[data-status="upcoming"] {
                    background: var(--stone-2);
                    color: var(--color-text-primary);
                }

                &[data-status="past-incomplete"] {
                    background: var(--color-error-bg);
                    color: var(--color-error);
                }

                &[data-status="unscheduled"] {
                    background: var(--stone-2);
                    color: var(--color-text-primary);
                    border: 1px dashed var(--stone-5);
                }

                &[data-status="today"] {
                    background: var(--clay-1);
                    color: var(--clay-6);
                    font-weight: var(--font-weight-6);
                }
```

- [ ] **Step 3: Confirm no raw-ramp or `--gray-*` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-' ui/templates/pages/home/day-cards.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 4: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS (this renders the authenticated home page including `day-cards`; a template typo or broken `@scope` block fails here).

- [ ] **Step 5: Visual check (describe; cannot drive a browser here)**

Intended result on `make dev` → `/`: the day cards read in the warm palette — "today" is a soft clay-tinted card with a clay border, "completed" sage-green, "in progress" amber, "past-incomplete" a dusty red-brown, "unscheduled" a dashed warm-sand card, "upcoming" a plain elevated stone card. The status-indicator badges match. No cool blue/green/yellow Tailwind hues remain. Report this as "not visually verified — describe only".

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/home/day-cards.gohtml
git commit -m "$(cat <<'EOF'
Warm the day-card status tints onto the Stone palette

Re-points the per-status card and badge tints from the raw
--sky/lime/yellow/red ramps onto the warm semantic tokens, and
renames the remaining --gray-* references to Stone/semantic.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Progress bar — warm the track, fill and text

**Files:**
- Modify: `ui/templates/pages/home/progress-bar.gohtml`

- [ ] **Step 1: Re-point the progress-bar colours**

In `ui/templates/pages/home/progress-bar.gohtml`, replace:

```css
                    .progress-bar {
                        flex: 1;
                        height: 8px;
                        background: var(--gray-3);
                        border-radius: var(--radius-round);
                        overflow: hidden;
                    }

                    .progress-fill {
                        width: {{ .ProgressPercent }}%;
                        height: 100%;
                        background: {{ if eq .Status "completed" }}var(--lime-6){{ else }}var(--yellow-6){{ end }};
                    }

                    .progress-text {
                        font-size: var(--font-size-0);
                        color: var(--gray-6);
                        min-width: fit-content;
                    }
```

with:

```css
                    .progress-bar {
                        flex: 1;
                        height: 8px;
                        background: var(--stone-3);
                        border-radius: var(--radius-round);
                        overflow: hidden;
                    }

                    .progress-fill {
                        width: {{ .ProgressPercent }}%;
                        height: 100%;
                        background: {{ if eq .Status "completed" }}var(--color-success){{ else }}var(--color-warning){{ end }};
                    }

                    .progress-text {
                        font-size: var(--font-size-0);
                        color: var(--color-text-secondary);
                        min-width: fit-content;
                    }
```

- [ ] **Step 2: Confirm no raw-ramp or `--gray-*` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-' ui/templates/pages/home/progress-bar.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS (the progress bar renders inside the day cards on the home page).

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `/`: the progress track is a warm stone grey; the fill is sage-green for a completed day and amber for an in-progress day; the "N/M sets" text is the secondary stone text colour. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/home/progress-bar.gohtml
git commit -m "$(cat <<'EOF'
Warm the day-card progress bar onto the Stone palette

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Muscle balance — warm the bars, legend and text

**Files:**
- Modify: `ui/templates/pages/home/muscle-balance.gohtml`

- [ ] **Step 1: Re-point the text and surface colours**

In `ui/templates/pages/home/muscle-balance.gohtml`, apply these substitutions inside the `@scope (.muscle-balance)` block (each old value occurs once unless noted):

- `.region-heading` `color: var(--gray-7);` → `color: var(--color-text-secondary);`
- `.label` `color: var(--gray-9);` → `color: var(--color-text-primary);`
- `.bar` `background: var(--gray-2);` → `background: var(--stone-2);`
- `.bar-planned` `border: 1px dashed var(--gray-6);` → `border: 1px dashed var(--stone-6);`
- `.target-mark` `background: var(--gray-9);` → `background: var(--stone-9);`
- `.legend` `color: var(--gray-9);` → `color: var(--color-text-primary);`
- `.legend > summary` `color: var(--gray-7);` → `color: var(--color-text-secondary);`
- `.legend > summary:hover` `background: var(--gray-2);` → `background: var(--stone-2);`
- `.legend > summary:hover` `color: var(--gray-9);` → `color: var(--color-text-primary);`
- `.legend-body` `background: var(--gray-1);` → `background: var(--stone-1);`
- `.legend-sample` `background: var(--gray-2);` → `background: var(--stone-2);`
- `.legend-sample-planned` `border: 1px dashed var(--gray-6);` → `border: 1px dashed var(--stone-6);`
- `.legend-sample-target` `background: var(--gray-9);` → `background: var(--stone-9);`
- `.counts` `color: var(--gray-7);` → `color: var(--color-text-secondary);`
- `.counts .target` `color: var(--gray-6);` → `color: var(--color-text-secondary);`

- [ ] **Step 2: Re-point the status colours**

In the same file, replace:

```css
                    .row[data-status="under"] .bar-completed,
                    .legend-swatch[data-status="under"] { background: var(--yellow-6); }
                    .row[data-status="on-target"] .bar-completed,
                    .legend-swatch[data-status="on-target"] { background: var(--lime-6); }
                    .row[data-status="over"] .bar-completed,
                    .legend-swatch[data-status="over"] { background: var(--red-6); }
                    .row[data-status="no-target"] .bar-completed,
                    .legend-swatch[data-status="no-target"] { background: var(--sky-6); }
```

with:

```css
                    .row[data-status="under"] .bar-completed,
                    .legend-swatch[data-status="under"] { background: var(--color-warning); }
                    .row[data-status="on-target"] .bar-completed,
                    .legend-swatch[data-status="on-target"] { background: var(--color-success); }
                    .row[data-status="over"] .bar-completed,
                    .legend-swatch[data-status="over"] { background: var(--color-error); }
                    .row[data-status="no-target"] .bar-completed,
                    .legend-swatch[data-status="no-target"] { background: var(--color-info); }
```

Then replace the legend sample fill:

```css
                    .legend-sample-completed {
                        position: absolute;
                        inset-block: 0;
                        inset-inline-start: 0;
                        width: 60%;
                        background: var(--lime-6);
                        border-radius: var(--radius-round);
                    }
```

with:

```css
                    .legend-sample-completed {
                        position: absolute;
                        inset-block: 0;
                        inset-inline-start: 0;
                        width: 60%;
                        background: var(--color-success);
                        border-radius: var(--radius-round);
                    }
```

- [ ] **Step 3: Confirm no raw-ramp or `--gray-*` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-' ui/templates/pages/home/muscle-balance.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 4: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS (the test asserts `section.muscle-balance` renders and sits after `.weekly-schedule` — class names are unchanged, so it stays green).

- [ ] **Step 5: Visual check (describe; cannot drive a browser here)**

Intended result on `/`: the muscle-balance bars use the warm functional colours — amber for under-target, sage for on-target, dusty red-brown for over, dusty blue for no-target. The legend swatches match. Bar tracks, the legend body and the dashed planned outlines are warm stone; headings and counts are stone text colours. The legend `<details>` still expands. Report as "not visually verified — describe only".

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/home/muscle-balance.gohtml
git commit -m "$(cat <<'EOF'
Warm the muscle-balance bars and legend onto the Stone palette

Re-points the per-status bar/legend colours onto the warm semantic
tokens and renames the --gray-* surface and text references to
Stone/semantic.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Home page shell — menu button and deload week chip

**Files:**
- Modify: `ui/templates/pages/home/schedule.gohtml`

- [ ] **Step 1: Re-point the `.menu-button` chrome**

In `ui/templates/pages/home/schedule.gohtml`, replace:

```css
                .menu-button a {
                    display: inline-flex;
                    align-items: center;
                    padding: var(--size-2) var(--size-3);
                    background: var(--white);
                    color: var(--gray-9);
                    text-decoration: none;
                    border-radius: var(--radius-2);
                    border: 1px solid var(--gray-3);
                    font-weight: var(--font-weight-5);
                    font-size: var(--font-size-1);
                    transition: all 0.2s ease;
                    box-shadow: var(--shadow-1);
                }

                .menu-button a:hover {
                    background: var(--gray-0);
                    border-color: var(--gray-4);
                    box-shadow: var(--shadow-3);
                    transform: translateY(-1px);
                }
```

with:

```css
                .menu-button a {
                    display: inline-flex;
                    align-items: center;
                    padding: var(--size-2) var(--size-3);
                    background: var(--color-surface-elevated);
                    color: var(--color-text-primary);
                    text-decoration: none;
                    border-radius: var(--radius-2);
                    border: 1px solid var(--color-border);
                    font-weight: var(--font-weight-5);
                    font-size: var(--font-size-1);
                    transition: all 0.2s ease;
                    box-shadow: var(--shadow-1);
                }

                .menu-button a:hover {
                    background: var(--stone-1);
                    border-color: var(--stone-4);
                    box-shadow: var(--shadow-3);
                    transform: translateY(-1px);
                }
```

(The `.menu-button a:active` rule below uses only `--shadow-1` and `transform` — leave it unchanged.)

- [ ] **Step 2: Re-point the deload `.week-chip`**

In the same file, replace:

```css
                @scope (.week-chip) {
                    :scope {
                        display: inline-block;
                        padding: var(--size-1) var(--size-2);
                        border-radius: var(--radius-2);
                        background: var(--gray-1);
                        color: var(--gray-9);
                        font-size: var(--font-size-0);

                        &.week-chip--deload {
                            background: var(--sky-2);
                            color: var(--sky-10);
                            font-weight: var(--font-weight-7);
                        }
                    }
                }
```

with:

```css
                @scope (.week-chip) {
                    :scope {
                        display: inline-block;
                        padding: var(--size-1) var(--size-2);
                        border-radius: var(--radius-2);
                        background: var(--stone-1);
                        color: var(--color-text-primary);
                        font-size: var(--font-size-0);

                        &.week-chip--deload {
                            background: var(--clay-1);
                            color: var(--clay-6);
                            font-weight: var(--font-weight-7);
                        }
                    }
                }
```

- [ ] **Step 3: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/home/schedule.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 4: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS.

- [ ] **Step 5: Visual check (describe; cannot drive a browser here)**

Intended result on `/`: the "Menu" link reads as a calm stone-bordered chip on an elevated surface, darkening to a warm sand on hover. When a deload week is active (feature-flag dependent), the week chip's deload variant is a soft clay fill with dark-clay text instead of the old cool blue. Report as "not visually verified — describe only"; note the deload chip only appears when `DeloadEnabled` and is week-dependent.

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/home/schedule.gohtml
git commit -m "$(cat <<'EOF'
Warm the home menu button and deload week chip onto Stone

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Unauthenticated landing — warm the copy and footer

**Files:**
- Modify: `ui/templates/pages/home/unauthenticated.gohtml`

- [ ] **Step 1: Re-point the tagline paragraph colour**

In `ui/templates/pages/home/unauthenticated.gohtml`, replace:

```css
                                > p {
                                    font-size: var(--font-size-3);
                                    line-height: var(--font-lineheight-3);
                                    color: var(--gray-7);
                                }
```

with:

```css
                                > p {
                                    font-size: var(--font-size-3);
                                    line-height: var(--font-lineheight-3);
                                    color: var(--color-text-secondary);
                                }
```

- [ ] **Step 2: Re-point the footer colours**

In the same file, replace:

```css
                :scope {
                    margin-top: var(--size-8);
                    padding-block: var(--size-4);
                    text-align: center;
                    border-top: 1px solid var(--gray-3);
                }

                a {
                    color: var(--gray-6);
                    text-decoration: none;
                    font-size: var(--font-size-1);
                }

                a:hover {
                    color: var(--gray-8);
                    text-decoration: underline;
                }
```

with:

```css
                :scope {
                    margin-top: var(--size-8);
                    padding-block: var(--size-4);
                    text-align: center;
                    border-top: 1px solid var(--color-border);
                }

                a {
                    color: var(--color-text-secondary);
                    text-decoration: none;
                    font-size: var(--font-size-1);
                }

                a:hover {
                    color: var(--color-text-primary);
                    text-decoration: underline;
                }
```

- [ ] **Step 3: Confirm no raw-ramp or `--gray-*` tokens remain**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-' ui/templates/pages/home/unauthenticated.gohtml`
Expected: no output (exit status 1).

- [ ] **Step 4: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_home -v`
Expected: PASS (the test loads the unauthenticated home page and asserts the "Sign in" / "Register" buttons render — button text is unchanged, so it stays green).

- [ ] **Step 5: Visual check (describe; cannot drive a browser here)**

Intended result on `/` while logged out: the "Personal trainer in your pocket." tagline is the secondary stone text colour, the footer rule is a warm stone border, and the "Privacy & Security" link is secondary-stone, darkening to primary on hover. The Sign in / Register buttons are already clay from Increment 1. Report as "not visually verified — describe only".

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/home/unauthenticated.gohtml
git commit -m "$(cat <<'EOF'
Warm the unauthenticated landing copy and footer onto Stone

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Full-suite verification and visual QA pass

**Files:** none (verification only)

- [ ] **Step 1: Confirm the increment's file list is fully repainted**

Run:
```bash
grep -rnE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' \
  ui/templates/pages/home/day-cards.gohtml \
  ui/templates/pages/home/progress-bar.gohtml \
  ui/templates/pages/home/muscle-balance.gohtml \
  ui/templates/pages/home/schedule.gohtml \
  ui/templates/pages/home/unauthenticated.gohtml
```
Expected: no output (exit status 1) — every raw-ramp and `--gray-*` / `--white` holdout in the increment's five files is gone.

- [ ] **Step 2: Run the full CI suite**

Run: `make ci`
Expected: `init`, `build`, `lint-fix`, `test`, `sec` all pass. If `lint-fix` makes formatting changes, review them, `git add` them, and commit with message `Apply lint-fix formatting`.

- [ ] **Step 3: Visual QA across the real home page**

Run: `make dev`, then open and eyeball:
- `/` while logged out — warm landing copy, warm footer, clay buttons.
- `/` while authenticated — the page background is warm stone, day cards use the warm per-status tints, the progress bars and muscle-balance bars use the warm functional colours, the "Menu" button is a calm stone chip.

Expected: the entire home/schedule surface reads as deliberate warm "Stone" — no element is left on a cool gray/blue/Tailwind-bright surface. If a browser cannot be driven in this environment, state explicitly that the visual QA was not performed and that verification rests on the `grep` checks, the `Test_application_home` render test, and `make ci`.

- [ ] **Step 4: Final confirmation**

Confirm `git status` is clean and `git log --oneline` shows the Task 1-5 commits (plus any lint-fix commit). Increment 2 is complete: the highest-traffic landing surface fully realizes the Stone direction.

---

## Self-review notes

- **Spec coverage:** Increment 2 of the spec calls for polishing `day-cards.gohtml` (Task 1), `schedule.gohtml` (Task 4), `muscle-balance.gohtml` (Task 3), the week chip (Task 4 — it lives in `home/schedule.gohtml`), `progress-bar.gohtml` (Task 2), `unauthenticated.gohtml` (Task 5). "Clean scoped styles, fix raw-token holdouts, rename `--gray-*` → `--stone-*` on these files" — every task does exactly this, and Task 1/3 Step 3 + Task 6 Step 1 `grep` gates prove no holdout survives. All covered.
- **"schedule.gohtml" disambiguation:** there are two — `pages/home/schedule.gohtml` (the home weekly view, in scope) and `pages/schedule/schedule.gohtml` (the schedule *editor*). The spec's Increment 5 explicitly lists "schedule editor" under Secondary pages, so Increment 2's "schedule.gohtml" is the home one. The editor is left untouched.
- **No DOM changes:** every task swaps only CSS custom-property values inside scoped `<style>` blocks. No class names, element structure, button text, or `data-*` attributes change — so the `goquery` home tests (`section.muscle-balance`, `.weekly-schedule` ordering, "Sign in"/"Register" button counts) and the Playwright flow stay green with zero test edits. This matches the Increment 1 verification shape.
- **Token choices:** semantic tokens (`--color-success`, `--color-text-secondary`, etc.) are preferred over literal `--stone-N` wherever the use has a clear semantic meaning, per `ui/templates/CLAUDE.md` ("reach for semantic tokens first"). Literal `--stone-N` is used only for ramp-position-specific, non-semantic uses (e.g. a dashed outline, a tick mark, a bar track). The "today" status maps to Clay because Increment 1 re-pointed `--color-surface-active` onto `--clay-0` and Clay is the spec's designated active-state accent.
- **Verification shape:** no fake unit tests for hex values — CSS has no compiler. Verification is the per-file `grep` gate + the `Test_application_home` end-to-end render test + `make ci` + described (and honestly caveated) visual checks. This mirrors Increment 1's plan.
- **Out of scope (per the spec's traffic-ordered staging):** `pages/schedule/schedule.gohtml` (Increment 5), `workout.gohtml` (Increment 3), `exerciseset/` Focus mode (Increment 4), all other `pages/**` and the motion pass. Their `--gray-*` call sites keep warming via the Increment 1 aliases until their own increment.
