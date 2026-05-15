# Aesthetics Uplift — Increment 3: Workout Overview — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Polish the workout overview screen — the exercise list, the per-exercise status tints, the add-exercise affordance, and the sticky "complete workout" bar — so it fully realizes the warm "Stone" visual direction.

**Architecture:** Increment 1 already retuned the token layer (Stone/Clay/Ember ramps, re-pointed semantic tokens, `--gray-*` aliased onto Stone, warm-tinted `--shadow-*`). This increment is per-page CSS polish only, scoped to the three `<style>` blocks in `ui/templates/pages/workout/workout.gohtml`: the `.workout-meta` summary, the `.exercise-list` (rows + add-exercise affordance), and the sticky `.complete-workout` bar. Each block's raw-ramp holdouts (`--sky/lime/yellow/red-*`) are re-pointed onto the warm semantic tokens, its `--gray-*` references renamed to `--stone-*` / semantic, and the `--white` surface plus hardcoded pure-black `box-shadow` on the sticky bar are warmed. No DOM structure, no class names, no Go code changes — so the existing `goquery` / Playwright DOM tests stay green untouched.

**Tech Stack:** CSS (custom properties, `@layer`, `@scope`), Go `html/template`, Go (`cmd/web` handler + `goquery`-based e2e tests).

**Spec:** `docs/superpowers/specs/2026-05-14-aesthetics-uplift-design.md` (Increment 3).

---

## File Structure

### Modified files

| File | Change |
|---|---|
| `ui/templates/pages/workout/workout.gohtml` | Re-point the `.workout-meta` progress-summary text, the `.exercise-list` row states (base / hover / active / completed / started) and the add-exercise affordance, and the sticky `.complete-workout` bar's surface + shadow — off the raw `--gray/lime/yellow-*` ramps and `--white` onto warm semantic / Stone / Clay tokens and the warm shadow tint. |

### Out of scope (untouched)

- `ui/static/main.css`, shared components (`.badge`, `page-header`, `banner`, `button`/`.btn`) — already on the Stone system from Increment 1. The status badge on this page renders via the shared `.badge badge--{{ .StatusVariant }}` class and the page header via the `page-header` partial; neither has a scoped style in `workout.gohtml`, so both shift automatically and need no edit here.
- `ui/templates/pages/exerciseset/` (`exerciseset.gohtml`, `sets-container.gohtml`, `exercise-header.gohtml`, `warmup.gohtml`) — that is Increment 4 (Focus mode). Leave it alone.
- Every other `pages/**` template — their `--gray-*` call sites keep warming automatically via the Increment 1 aliases until their own increment.
- All Go handlers and tests — no DOM or class-name changes are made, so no test edits are needed.

---

## Token mapping reference

The single source of truth for every substitution in this increment. All target tokens are defined in `ui/static/main.css` (verified present after Increment 1: `--color-text-secondary`, `--color-text-muted`, `--color-text-primary`, `--color-success` / `-bg`, `--color-warning` / `-bg`, `--color-surface-elevated`, `--stone-1`, `--clay-1` / `--clay-2` / `--clay-6` all exist).

**Raw-ramp holdouts → warm semantic / Clay tokens:**

| Old raw token(s) | Meaning | New token |
|---|---|---|
| `--lime-2` / `--lime-9` (exercise row: completed) | success | `--color-success-bg` / `--color-success` |
| `--yellow-2` / `--yellow-11` (exercise row: started) | warning / in-progress | `--color-warning-bg` / `--color-warning` |
| `--lime-2` / `--lime-9` / `--lime-3` (add-exercise affordance) | additive action | `--clay-1` / `--clay-6` / `--clay-2` (hover) |

**`--gray-N` → semantic / `--stone-N`:**

| Old | New | Rationale |
|---|---|---|
| `--gray-7` (`.progress-summary` text) | `--color-text-secondary` | secondary text |
| `--gray-5` (`.exercise` base text — struck-through, de-emphasised) | `--color-text-muted` | muted text |
| `--gray-1` (`a.exercise:hover` background) | `--stone-1` | non-semantic warm surface tint |
| `--gray-9` (`.exercise.active` text) | `--color-text-primary` | primary text |

**`--white` and hardcoded shadow:**

| Old | New | Rationale |
|---|---|---|
| `var(--white)` (`.complete-workout` sticky-bar background) | `var(--color-surface-elevated)` | elevated surface, consistent with Increment 1's `--color-surface-elevated: var(--stone-0)` |
| `0 -1px 2px 0 rgb(0 0 0 / 0.05)` (`.complete-workout` sticky-bar shadow) | `0 -1px 2px 0 rgb(120 90 60 / 0.10)` | warm-brown rgba tint at the same alpha Increment 1 used for `--shadow-*`; geometry kept (upward `-1px` offset) — see note below |

**Note on the sticky-bar shadow.** Increment 1 warmed the `--shadow-1/2/3` tokens by swapping `rgb(0 0 0 / …)` for `rgb(120 90 60 / …)`. Those tokens all cast *downward* (positive y-offset). The `.complete-workout` bar is `position: sticky; bottom: 0` and needs an *upward* shadow rising over the scrolling content above it, so a literal `var(--shadow-1)` would cast the shadow the wrong way. `ui/templates/CLAUDE.md` permits a raw `box-shadow` for cases the elevation tokens do not cover. The faithful, consistent move is therefore to keep the upward `0 -1px 2px 0` geometry and warm only the colour to `rgb(120 90 60 / 0.10)` — the exact tint and alpha Increment 1 applied to `--shadow-1`.

---

## Sequencing rationale

The single file has three independent scoped `<style>` blocks. They are decomposed into three implementation tasks by component group, ordered top-to-bottom as they appear on the page: (1) the workout-meta summary plus the exercise-list row states, (2) the add-exercise affordance, (3) the sticky complete-workout bar. Task 1 bundles the meta summary with the exercise rows because the meta summary is a single one-line colour swap and shares the "exercise list region" of the page. Task 4 is the full-suite gate.

**A note on verification:** CSS token/colour changes have no unit test — there is no compiler or linter for scoped `<style>` blocks. Each task is verified by (a) `go test ./cmd/web/ -run Test_application_addWorkout -v`, which registers a user, enables a weekday, starts a workout, and lands on `/workouts/<date>` — rendering `workout.gohtml` (page header, status badge, exercise list, add-exercise affordance, sticky complete-workout bar) end-to-end through `goquery`; a template typo or broken `@scope` block fails here; and (b) a `grep` gate proving no raw-ramp / `--gray-*` / `--white` holdout survives in the touched file; and (c) a described visual check. Task 4 runs the full `make ci`. This is the appropriate verification shape for a per-page palette repaint; **do not invent fake unit tests for hex values.** The user cannot drive a browser in this environment — perform the automated checks and explicitly report which described visual checks were not performed rather than claiming them.

---

## Task 1: Workout meta summary + exercise-list row states

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml`

- [ ] **Step 1: Re-point the `.workout-meta` progress-summary text**

In `ui/templates/pages/workout/workout.gohtml`, in the `@scope (.workout-meta)` block, replace:

```css
                @scope (.workout-meta) {
                    .progress-summary {
                        color: var(--gray-7);
                        font-size: var(--font-size-1);
                    }
                }
```

with:

```css
                @scope (.workout-meta) {
                    .progress-summary {
                        color: var(--color-text-secondary);
                        font-size: var(--font-size-1);
                    }
                }
```

- [ ] **Step 2: Re-point the exercise-list base, hover and active row states**

In the same file, in the `@scope (.exercise-list)` block, replace:

```css
                    .exercise {
                        display: flex;
                        align-items: center;
                        text-decoration: line-through;
                        color: var(--gray-5);
                    }

                    a.exercise {
                        text-decoration: none;
                        transition: background-color 0.2s;
                    }

                    a.exercise:hover {
                        background: var(--gray-1);
                    }

                    .exercise.active {
                        text-decoration: none;
                        color: var(--gray-9);
                        font-weight: var(--font-weight-6);
                    }
```

with:

```css
                    .exercise {
                        display: flex;
                        align-items: center;
                        text-decoration: line-through;
                        color: var(--color-text-muted);
                    }

                    a.exercise {
                        text-decoration: none;
                        transition: background-color 0.2s;
                    }

                    a.exercise:hover {
                        background: var(--stone-1);
                    }

                    .exercise.active {
                        text-decoration: none;
                        color: var(--color-text-primary);
                        font-weight: var(--font-weight-6);
                    }
```

- [ ] **Step 3: Re-point the completed and started exercise-row tints**

In the same `@scope (.exercise-list)` block, replace:

```css
                    .exercise.completed {
                        background-color: var(--lime-2);
                        color: var(--lime-9);
                    }

                    .exercise.started {
                        background-color: var(--yellow-2);
                        color: var(--yellow-11);
                    }
```

with:

```css
                    .exercise.completed {
                        background-color: var(--color-success-bg);
                        color: var(--color-success);
                    }

                    .exercise.started {
                        background-color: var(--color-warning-bg);
                        color: var(--color-warning);
                    }
```

- [ ] **Step 4: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_addWorkout -v`
Expected: PASS (this starts a workout and lands on `/workouts/<date>`, rendering `workout.gohtml` end-to-end; a template typo or broken `@scope` block fails here).

- [ ] **Step 5: Visual check (describe; cannot drive a browser here)**

Intended result on `make dev` → a started workout at `/workouts/<date>`: the "N exercises" meta summary is the secondary stone text colour; exercise rows that are not yet done are struck-through in muted stone; the active exercise row is primary-stone, un-struck, semibold; a completed exercise row sits on a sage `--color-success-bg` fill with `--color-success` text; a started-but-unfinished row sits on an amber `--color-warning-bg` fill with `--color-warning` text; hovering a row gives a warm `--stone-1` sand tint. No cool grey/green/yellow Tailwind hues remain in this region. Report this as "not visually verified — describe only".

- [ ] **Step 6: Confirm no raw-ramp or `--gray-*` tokens remain in the meta/exercise-list blocks**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-' ui/templates/pages/workout/workout.gohtml`
Expected: one remaining match — the `.add-exercise-button` `--lime-*` uses (lines ~76, 77, 86), which Task 2 handles. The `.workout-meta` and exercise-row holdouts must all be gone. (After Task 2 this command returns nothing.)

- [ ] **Step 7: Commit**

```bash
git add ui/templates/pages/workout/workout.gohtml
git commit -m "$(cat <<'EOF'
Warm the workout meta and exercise-list rows onto the Stone palette

Re-points the progress-summary text, the exercise-row base/hover/active
states and the completed/started per-status tints from the raw
--gray/lime/yellow ramps onto the warm semantic and Stone tokens.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add-exercise affordance

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml`

The `.add-exercise-button` is an additive action affordance, currently a soft green (`--lime-*`) pill. The Stone spec designates **Clay** as the action accent ("primary accent for buttons, links, and active state"). It is re-pointed onto the soft Clay pair — `--clay-1` background / `--clay-6` text, darkening to `--clay-2` on hover — the same soft-Clay treatment Increment 2 used for the "today" badge and the deload week chip. It deliberately stays a soft pill rather than the solid `--clay-4` of the primary `.btn`, so it reads as a secondary action and does not compete with the sticky "Complete workout" button.

- [ ] **Step 1: Re-point the `.add-exercise-button` onto Clay**

In `ui/templates/pages/workout/workout.gohtml`, in the `@scope (.exercise-list)` block, replace:

```css
                    .add-exercise-button {
                        background-color: var(--lime-2);
                        color: var(--lime-9);
                        border-radius: var(--radius-2);
                        padding: var(--size-2) var(--size-3);
                        font-weight: var(--font-weight-6);
                        text-decoration: none;
                        display: inline-block;
                    }

                    .add-exercise-button:hover {
                        background-color: var(--lime-3);
                    }
```

with:

```css
                    .add-exercise-button {
                        background-color: var(--clay-1);
                        color: var(--clay-6);
                        border-radius: var(--radius-2);
                        padding: var(--size-2) var(--size-3);
                        font-weight: var(--font-weight-6);
                        text-decoration: none;
                        display: inline-block;
                    }

                    .add-exercise-button:hover {
                        background-color: var(--clay-2);
                    }
```

- [ ] **Step 2: Confirm no raw-ramp or `--gray-*` tokens remain anywhere in the file**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-' ui/templates/pages/workout/workout.gohtml`
Expected: no output (exit status 1) — every raw-ramp and `--gray-*` holdout in the file is now gone. (`--white` is still present in the `.complete-workout` block; Task 3 handles it.)

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_addWorkout -v`
Expected: PASS (the add-exercise affordance renders on `/workouts/<date>`, and the test also navigates the add-exercise flow).

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `/workouts/<date>`: the "Add Exercise" pill is a soft clay fill (`--clay-1`) with dark-clay text (`--clay-6`), darkening to `--clay-2` on hover — a calm secondary action, not the bright green it was, and visually subordinate to the solid-clay "Complete workout" button below. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/workout/workout.gohtml
git commit -m "$(cat <<'EOF'
Warm the workout add-exercise affordance onto Clay

Re-points the add-exercise pill from the raw --lime ramp onto the soft
Clay action accent, consistent with the Stone direction's designated
action colour.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Sticky complete-workout bar

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml`

The sticky bar's background is the legacy `--white`; its `box-shadow` is a hardcoded pure-black rgba. The background moves onto `--color-surface-elevated` (the elevated-surface semantic token, `var(--stone-0)` after Increment 1). The shadow keeps its upward `0 -1px 2px 0` geometry — no `--shadow-*` token casts upward — and only its colour is warmed to `rgb(120 90 60 / 0.10)`, the exact tint and alpha Increment 1 applied to `--shadow-1`. See the "Note on the sticky-bar shadow" in the Token mapping reference.

- [ ] **Step 1: Re-point the sticky-bar surface and warm its shadow**

In `ui/templates/pages/workout/workout.gohtml`, in the `@scope (.complete-workout)` block, replace:

```css
                @scope (.complete-workout) {
                    :scope {
                        padding: var(--size-4);
                        position: sticky;
                        bottom: 0;
                        background: var(--white);
                        box-shadow: 0 -1px 2px 0 rgb(0 0 0 / 0.05);
                    }
```

with:

```css
                @scope (.complete-workout) {
                    :scope {
                        padding: var(--size-4);
                        position: sticky;
                        bottom: 0;
                        background: var(--color-surface-elevated);
                        box-shadow: 0 -1px 2px 0 rgb(120 90 60 / 0.10);
                    }
```

(The `button { ... }` rule below in the same block uses only `--font-size-2` and `--size-3` — no colour tokens — leave it unchanged. The button itself is the shared `button` class-component, already clay from Increment 1.)

- [ ] **Step 2: Confirm no raw-ramp, `--gray-*`, or `--white` tokens remain anywhere in the file**

Run: `grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/workout/workout.gohtml`
Expected: no output (exit status 1) — the entire file is now free of raw-ramp, `--gray-*`, and `--white` holdouts.

- [ ] **Step 3: Verify the template still renders**

Run: `go test ./cmd/web/ -run Test_application_addWorkout -v`
Expected: PASS (the sticky `.complete-workout` bar with its "Complete workout" submit button renders on `/workouts/<date>`).

- [ ] **Step 4: Visual check (describe; cannot drive a browser here)**

Intended result on `/workouts/<date>`: the sticky bar pinned to the bottom sits on the warm elevated stone surface (not pure white) with a soft warm-brown upward shadow lifting it off the scrolling content above; the "Complete workout" button inside it is the solid clay primary button from Increment 1. Report as "not visually verified — describe only".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/workout/workout.gohtml
git commit -m "$(cat <<'EOF'
Warm the sticky complete-workout bar onto the Stone palette

Moves the sticky-bar surface onto --color-surface-elevated and warms
its hardcoded pure-black box-shadow to the warm-brown tint Increment 1
applied to the elevation tokens, keeping the upward geometry the
shadow tokens do not cover.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Full-suite verification and visual QA pass

**Files:** none (verification only)

- [ ] **Step 1: Confirm the file is fully repainted**

Run:
```bash
grep -nE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)' ui/templates/pages/workout/workout.gohtml
```
Expected: no output (exit status 1) — every raw-ramp, `--gray-*`, and `--white` holdout in `workout.gohtml` is gone.

- [ ] **Step 2: Run the full CI suite**

Run: `make ci`
Expected: `init`, `build`, `lint-fix`, `test`, `sec` all pass. If `lint-fix` makes formatting changes, review them, `git add` them, and commit with message `Apply lint-fix formatting`.

- [ ] **Step 3: Visual QA across the workout overview page**

Run: `make dev`, then register, enable today's weekday in `/preferences`, start a workout, and eyeball `/workouts/<date>`:
- The status badge (shared `.badge` component) and page header read in the warm palette.
- The "N exercises" meta summary is secondary stone text.
- Exercise rows: not-done rows struck-through in muted stone; the active row primary-stone semibold; a completed row on sage; a started row on amber; hover gives a warm sand tint.
- The "Add Exercise" pill is a soft clay action affordance.
- The sticky "Complete workout" bar sits on the warm elevated stone surface with a soft warm upward shadow; its button is solid clay.

Expected: the entire workout overview surface reads as deliberate warm "Stone" — no element is left on a cool grey/green/yellow/white Tailwind-bright surface. If a browser cannot be driven in this environment, state explicitly that the visual QA was not performed and that verification rests on the `grep` checks, the `Test_application_addWorkout` render test, and `make ci`.

- [ ] **Step 4: Final confirmation**

Confirm `git status` is clean and `git log --oneline` shows the Task 1-3 commits (plus any lint-fix commit). Increment 3 is complete: the workout overview screen fully realizes the Stone direction.

---

## Self-review notes

- **Spec coverage:** Increment 3 of the spec calls for polishing `workout.gohtml` — "exercise list, status badge, add-exercise affordance, the sticky 'complete workout' bar." Exercise list → Task 1 (rows + states) plus the `.workout-meta` summary. Status badge → renders via the shared `.badge badge--{{ .StatusVariant }}` class already warmed in Increment 1; it has no scoped style in this file, so it shifts automatically and correctly needs no edit — noted in "Out of scope". Add-exercise affordance → Task 2. Sticky complete-workout bar → Task 3. All covered; the Task 2/3 `grep` gates and Task 4 Step 1 prove no holdout survives.
- **Add-exercise affordance colour choice:** the spec does not pin an exact token for it. It is an additive *action* affordance, and the spec designates Clay as the action accent. Soft Clay (`--clay-1` / `--clay-6`, `--clay-2` hover) matches the soft-Clay treatment Increment 2 already established for the "today" badge and deload chip, and deliberately stays a soft pill rather than the solid `--clay-4` primary `.btn` so it reads as secondary to the "Complete workout" button. This is a defensible application of the locked spec direction, not a new design decision.
- **Sticky-bar shadow:** the spec note says to re-point hardcoded shadows onto the warm shadow token. The `--shadow-*` tokens all cast downward; the sticky bar at `bottom: 0` needs an upward shadow. `ui/templates/CLAUDE.md` permits a raw `box-shadow` where the elevation tokens do not fit. The faithful, consistent move — kept in the plan — is to preserve the upward `0 -1px 2px 0` geometry and warm only the colour to `rgb(120 90 60 / 0.10)`, the exact tint/alpha Increment 1 applied to `--shadow-1`. Documented in the Token mapping reference and Task 3's preamble.
- **No DOM changes:** every step swaps only CSS custom-property values inside the existing scoped `<style>` blocks. No class names, element structure, button text, `data-*` attributes, or the inline view-transition `<script>` change — so the `goquery` workout tests (`a.exercise` counts, "Add Exercise" heading) and the Playwright flow stay green with zero test edits. This matches the Increment 1-2 verification shape.
- **Token choices:** semantic tokens (`--color-text-secondary`, `--color-text-muted`, `--color-text-primary`, `--color-success` / `-bg`, `--color-warning` / `-bg`, `--color-surface-elevated`) are preferred wherever the use has a clear semantic meaning, per `ui/templates/CLAUDE.md`. Literal `--stone-1` is used only for the non-semantic hover surface tint, and `--clay-1/2/6` for the action affordance — matching the Increment 1-2 convention.
- **Verification shape:** no fake unit tests for hex values — CSS has no compiler. Verification is the per-file `grep` gate + the `Test_application_addWorkout` end-to-end render test + `make ci` + described (and honestly caveated) visual checks. This mirrors the Increment 1-2 plans.
- **Out of scope (per the spec's traffic-ordered staging):** `pages/exerciseset/` Focus mode (Increment 4), all other `pages/**` and the motion pass (Increments 5-6). Their `--gray-*` call sites keep warming via the Increment 1 aliases until their own increment.
</content>
</invoke>
