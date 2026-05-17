# Card-state view transitions on bfcache replace — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the user returns to `/workouts/{date}` after changing an exercise's state on the exerciseset page, the affected card crossfades to its new color and (if newly completed) a strikethrough line draws across the exercise name. All other cards stay still.

**Architecture:** Page-local `<script {{ nonce }}>` in `workout.gohtml` opts out of `main.js`'s global bfcache reload (via a body data attribute), awaits the backward traverse view transition, then fires `navigation.navigate(location.pathname + '?_inv=1', { history: 'replace' })`. The cache-buster query keeps Chromium from reclassifying the same-URL navigate as a `'reload'` (which wouldn't get cross-doc VT). On arrival, the script strips the query via `history.replaceState`. A custom `'state-refresh'` view-transition type lets the page-local CSS suppress the global page slide and run per-card animations (color crossfade + strike-line draw) without affecting other transitions.

**Tech Stack:** Go html/template, plain CSS with `@scope` and `<style {{ nonce }}>` colocation, cross-document View Transitions (CSS `@view-transition`, JS Navigation API), no build step.

**Spec:** [`docs/superpowers/specs/2026-05-17-card-state-view-transitions-design.md`](../specs/2026-05-17-card-state-view-transitions-design.md)

---

## File Structure

**Modified files:**

| File | What changes | Why |
|---|---|---|
| `ui/static/main.js` | Add a 1-line opt-out check in the `pageshow` handler (line 298) right after `clearLoad()`; narrow the `pagereveal` `skipTransition` guard (line 242) from `(replace \|\| reload)` to `reload` only | Allows the workout page to handle its own bfcache invalidation; allows replace navigations to participate in cross-doc VT |
| `ui/templates/pages/workout/workout.gohtml` | Rewrite `.exercise.completed .exercise-name` strikethrough as an `::after` pseudo-line; add a dynamic `<style {{ nonce }}>` block emitting per-card and per-strike view-transition names, type-gated animation overrides, and reduced-motion rules; add a `<script {{ nonce }}>` block for the bfcache-handler opt-out + cache-buster replace flow + URL cleanup | All page-local visual + behavior changes; emitting per-card rules needs `.Exercises` data |

**Created files:** none.

**No changes:** Go handlers, services, domain types, `main.css`, any other template.

---

## Working-tree note for the implementer

The worktree may contain uncommitted PoC files from the design phase
(`cmd/web/handler-transitions-poc.go`, modified `cmd/web/handler-home.go`,
`cmd/web/routes.go`, `ui/templates/pages/home/schedule.gohtml`, and the
`ui/templates/pages/transitions-poc-{list,detail}/` folders). These are
*not* part of this plan. Don't stage, modify, or commit them. They'll be
cleaned up separately. Use `git add <specific-file>` rather than `git add
-A` or `git commit -a` so they stay unstaged.

---

## Pre-flight checks before implementation

These are smoke tests, not architectural decisions. The PoC validated the
load-bearing mechanic (replace navigation with cache-buster gets cross-doc
VT after await), so this pre-flight is light.

- [ ] **Confirm the modified main.js path doesn't regress existing animations.**

After Task 1 (the main.js tweaks), open the running app, navigate around
between the existing pages (workout list ↔ exerciseset, schedule ↔ home,
etc.) and confirm the existing forward/backward slide animations still play
the same way. The change narrows `skipTransition` from `(replace || reload)`
to `(reload)` — replace navigations on other pages would now animate
(previously they snapped). If any such animation looks wrong, file it as a
follow-up and proceed (this plan's scope is the workout page only).

- [ ] **(Optional) Re-verify the cache-buster trick on `/workouts/{date}` after Task 6.**

Add a temporary `console.log` in the page-local pagereveal handler. Trigger
the full flow (complete a set, back out). Confirm two `pagereveal` events:
first with `navType: 'traverse'` and `hasVT: true` (the bfcache restore),
then with `navType: 'replace'` and `hasVT: true` (our replace navigation).
Remove the log before committing Task 6.

If the second pagereveal reports `navType: 'reload'`, the cache-buster query
isn't differentiating the URL enough for Chromium. Verify
`location.pathname + '?_inv=1'` is the call site target, not `location.href`
(which already contains `?_inv=1` on the second call and looks identical to
current). Escalate if it persists.

---

## Task 1 — main.js: opt-out check + narrow `skipTransition`

**Files:**
- Modify: `ui/static/main.js` (line 237-251 — the `pagereveal` handler; line 298-322 — the `pageshow` handler)

The global `pagereveal` handler currently skips view transitions for both
`replace` and `reload` navigation types. Replace needs to be allowed through
so the workout page's cross-doc VT can run. The global `pageshow` handler
currently fires `navigation.reload()` on every page when the bfcache cookie
mismatches; pages that handle their own bfcache invalidation need a way to
opt out so they don't get preempted.

- [ ] **Step 1: Narrow the `skipTransition` guard from `(replace || reload)` to `(reload)`**

Find the `pagereveal` handler at lines 237-252:

```js
window.addEventListener('pagereveal', (e) => {
    if (!e.viewTransition) return
    if (!('navigation' in window)) return
    const act = navigation.activation
    if (!act) return
    if (act.navigationType === 'replace' || act.navigationType === 'reload') {
        e.viewTransition.skipTransition()
        return
    }
    let dir = 'forward'
    if (act.navigationType === 'traverse' && act.from && act.entry) {
        dir = act.entry.index < act.from.index ? 'backward' : 'forward'
    }
    e.viewTransition.types.add(dir)
})
```

Replace the condition block (lines 242-245) with:

```js
    if (act.navigationType === 'reload') {
        // Reload-triggered cross-doc VT: never animate (would replay the page
        // slide for "same page, just fresher data" — disorienting). Replace
        // navigations are allowed through so pages that opt into a replace-
        // driven bfcache flow (see workout.gohtml) can animate state changes.
        e.viewTransition.skipTransition()
        return
    }
```

- [ ] **Step 2: Add the opt-out check in the `pageshow` handler**

Find the `pageshow` handler at lines 298-323:

```js
window.addEventListener('pageshow', (event) => {
    if (event.persisted) {
        // Clear any in-flight navigation feedback that was captured in the
        // bfcache snapshot — the navigation that triggered it has long
        // since resolved (we are reading this snapshot from the cache).
        clearLoad()

        // Reload if the invalidation cookie has changed since this page was rendered.
        // ...
    }
})
```

Right after the `clearLoad()` call, insert:

```js
        // Pages that handle their own bfcache invalidation opt out here so
        // their custom flow (e.g. replace navigation for cross-doc view
        // transitions) isn't preempted by our global reload.
        if (document.body.dataset.bfcacheHandler === 'page-local') return
```

So the handler becomes:

```js
window.addEventListener('pageshow', (event) => {
    if (event.persisted) {
        // Clear any in-flight navigation feedback that was captured in the
        // bfcache snapshot — the navigation that triggered it has long
        // since resolved (we are reading this snapshot from the cache).
        clearLoad()

        // Pages that handle their own bfcache invalidation opt out here so
        // their custom flow (e.g. replace navigation for cross-doc view
        // transitions) isn't preempted by our global reload.
        if (document.body.dataset.bfcacheHandler === 'page-local') return

        // Reload if the invalidation cookie has changed since this page was rendered.
        // The render-time value is baked into a <meta> tag; a mismatch means a POST
        // ran while we were in bfcache and our state may be stale.
        const meta = document.querySelector('meta[name="invalidation-token"]')
        const rendered = meta ? meta.content : ''
        const m = document.cookie.match(/(?:^|;\s*)inv_bfcache=([^;]+)/)
        const current = m ? m[1] : ''
        if (rendered !== current) {
            if ('navigation' in window) {
                navigation.reload()
            } else {
                location.reload()
            }
        }
    }
})
```

- [ ] **Step 3: Run the test suite to make sure nothing broke**

```
make test
```

Expected: all tests PASS. The main.js change is JS-only; Go tests assert on
markup/handlers, not on JS. If anything fails, read the failure and
investigate.

- [ ] **Step 4: Run lint-fix**

```
make lint-fix
```

Expected: clean (no diff). main.js isn't part of golangci-lint's scope, so
this is mostly verifying nothing else regressed.

- [ ] **Step 5: Visually verify no regression**

Open the running app (or start the dev server: `make dev` → opens browser
on a random port). Navigate around: workout list ↔ exerciseset, schedule ↔
home. The existing forward/backward page slide animations should still
play. Specifically:
- Forward to exerciseset: page slides in from right
- Back to workout list: page slides in from left
- Same-URL form submits (e.g. set update on exerciseset): page should now
  *crossfade* instead of snapping (this is a subtle improvement from
  removing `replace` from the skip list). Acceptable.

- [ ] **Step 6: Commit**

```
git add ui/static/main.js
git commit -m "Allow replace navigations to opt into cross-doc view transitions"
```

---

## Task 2 — workout.gohtml: replace strike-through with an `::after` pseudo-line

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml` (`.exercise-name` rule at lines 181-186; `.exercise.completed .exercise-name` rule at lines 243-247, inside the `@scope (.exercise-list)` block)

The strike line must render at full width on cold loads so any reload (or
any reveal where the card was already completed) still shows it. It uses a
semantic success color with a transparency tint, and a `forced-colors:
active` fallback. The line gets its own `view-transition-name` later (Task
4); this task just lays down the structure.

- [ ] **Step 1: Make `.exercise-name` a positioning context**

Find the existing `.exercise-name` rule at lines 181-186:

```css
.exercise-name {
    font-weight: var(--font-weight-6);
    font-size: var(--font-size-2);
    color: var(--color-text-primary);
    line-height: 1.2;
}
```

Add `position: relative;` so the future `::after` line can be absolutely
positioned against it:

```css
.exercise-name {
    font-weight: var(--font-weight-6);
    font-size: var(--font-size-2);
    color: var(--color-text-primary);
    line-height: 1.2;
    position: relative;
}
```

- [ ] **Step 2: Replace `text-decoration: line-through` with the `::after` line**

Find the `.exercise.completed .exercise-name` rule at lines 243-247:

```css
.exercise.completed .exercise-name {
    color: var(--color-success);
    text-decoration: line-through;
    text-decoration-color: color-mix(in oklab, var(--color-success) 40%, transparent);
}
```

Replace with:

```css
.exercise.completed .exercise-name {
    color: var(--color-success);
}

.exercise.completed .exercise-name::after {
    content: "";
    position: absolute;
    left: 0;
    right: 0;
    top: 50%;
    height: 1.5px;
    background: color-mix(in oklab, var(--color-success) 50%, transparent);
    transform: scaleX(1);
    transform-origin: left center;
    pointer-events: none;
}

@media (forced-colors: active) {
    .exercise.completed .exercise-name::after {
        background: CanvasText;
        forced-color-adjust: none;
    }
}
```

The default `transform: scaleX(1)` keeps the line drawn on cold loads. The
`transform-origin: left center` is set here as well as in the per-strike
view-transition rules (Task 4) so a transform applied during the transition
animates from the same anchor.

- [ ] **Step 3: Run handler tests to confirm nothing assertion-level broke**

```
go test ./cmd/web -run TestWorkout -count=1
```

Expected: PASS. The structural change only affects pseudo-element styling;
no `.exercise-name` text content, class names, or attributes change. If
anything fails, read the failure and investigate.

- [ ] **Step 4: Visually verify cold render**

Load `/workouts/<date>` for a day with at least one completed exercise
(hard reload, Ctrl+Shift+R, to bypass any view-transition path). The
completed card should show a thin success-green line straight across the
exercise name, vertically centered with the text. If it looks misaligned,
tweak `top: 50%` (some fonts will want `top: 55%` or a small `margin-top`).
The visual goal is "line through the x-height," matching the prior
`text-decoration: line-through` rendering.

- [ ] **Step 5: Commit**

```
git add ui/templates/pages/workout/workout.gohtml
git commit -m "Replace exercise-name strike with an ::after line"
```

---

## Task 3 — workout.gohtml: per-card `view-transition-name` for every exercise

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml` (new dynamic `<style {{ nonce }}>` block adjacent to the exercise list)

Every card gets a unique transition name. The browser pairs old and new
snapshots by name across any cross-doc navigation. During the bfcache→
replace flow (added in Task 6), cards whose state is unchanged produce
identical snapshots and crossfade no-op; cards whose state changed crossfade
their pixels.

- [ ] **Step 1: Add a dynamic style block before the exercise list**

Find the existing `<style {{ nonce }}>` block ending at line 317 (the
closing `}` of `@scope (.exercise-list)` followed by `</style>`). Immediately
*after* that closing `</style>`, before the `{{ range .Exercises }}` loop at
line 319, insert a new style block:

```gohtml
<style {{ nonce }}>
    {{ range .Exercises }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] { view-transition-name: ex-card-{{ .ID }}; }
    {{ end }}
</style>
```

Each rule pins one card's transition name to its existing
`data-workout-exercise-id` attribute (already set at line 322 — no markup
change needed). The rule lives at root scope (not inside `@scope`) so
`view-transition-name` applies regardless of selector nesting.

- [ ] **Step 2: Verify rendered HTML contains the per-card rules**

Start the dev server (`make dev`), load `/workouts/<date>`, view source.
Confirm the new `<style>` block contains one line per exercise, e.g.:

```
.exercise[data-workout-exercise-id="42"] { view-transition-name: ex-card-42; }
.exercise[data-workout-exercise-id="43"] { view-transition-name: ex-card-43; }
```

Rule count = `len(.Exercises)`.

- [ ] **Step 3: Verify the transition captures via DevTools (optional but recommended)**

Open DevTools → Animations panel. Navigate from `/workouts/<date>` forward
to an exerciseset page (which fires the existing title morph). The
Animations panel should show entries like `::view-transition-group(ex-card-N)`
alongside `::view-transition-group(page)`. If you see only the `page` group
and no `ex-card-*` entries, the names aren't being captured — recheck the
HTML output from Step 2 and confirm the `<style>` block sits *outside* the
`@scope (.exercise-list)` block. (Inside `@scope`, the selector
specificity changes and may not apply as expected.)

- [ ] **Step 4: Visual verification deferred**

The per-card crossfade isn't visible yet — it only activates during the
bfcache→replace flow added in Task 6. Don't try to verify it here.

- [ ] **Step 5: Commit**

```
git add ui/templates/pages/workout/workout.gohtml
git commit -m "Name exercise cards for cross-doc view transitions"
```

---

## Task 4 — workout.gohtml: per-strike `view-transition-name`, draw animation, page-slide suppression

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml` (extend the dynamic `<style {{ nonce }}>` block from Task 3)

The strike line gets a per-card name on completed cards only. During the
bfcache→replace flow, the old snapshot (pre-POST: card was not yet
completed) has no `::after` line; the new snapshot (post-POST: card now
completed) has one. The browser runs the default "new element appears"
treatment — we override with a `scaleX(0)→scaleX(1)` draw. Both the strike
draw and the page-slide suppression are wrapped in
`html:active-view-transition-type(state-refresh)` so they only fire for our
custom replace transition (the type is added in Task 6), not for the
backward traverse or regular forward navigations.

- [ ] **Step 1: Extend the dynamic style block**

Replace the entire `<style {{ nonce }}>` block you added in Task 3 with:

```gohtml
<style {{ nonce }}>
    {{ range .Exercises }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] { view-transition-name: ex-card-{{ .ID }}; }
    {{ if eq .State "completed" }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] .exercise-name::after { view-transition-name: ex-strike-{{ .ID }}; }
    {{ end }}
    {{ end }}

    html:active-view-transition-type(state-refresh) {
        /* Suppress the global page slide so per-card snapshots dominate. */
        &::view-transition-old(page),
        &::view-transition-new(page) {
            animation: none;
        }
        /* Strike draw on newly-completed cards (asymmetric: line exists in
           new snapshot but not old). */
        {{ range .Exercises }}{{ if eq .State "completed" }}
        &::view-transition-new(ex-strike-{{ .ID }}) {
            animation: strike-draw 320ms cubic-bezier(.2, .7, .3, 1) both;
            transform-origin: left center;
        }
        {{ end }}{{ end }}
    }

    @keyframes strike-draw {
        from { transform: scaleX(0); }
        to   { transform: scaleX(1); }
    }
</style>
```

Notes on the template:
- `.State` is `domain.ExerciseSetState` (a named string type). Go's template
  `eq` compares string-kind values regardless of the named type, so
  `eq .State "completed"` works the same as `eq $.ExerciseSet.Exercise.ExerciseType "assisted"`
  already does in `ui/templates/pages/exerciseset/sets-container.gohtml:476`.
- The `&::view-transition-...` nested selectors are CSS Nesting (works in
  every browser that supports cross-doc VT).

- [ ] **Step 2: Verify the strike's name shows up only on completed cards**

Reload `/workouts/<date>` and view source. For each completed card, confirm
two rules per card:

```
.exercise[data-workout-exercise-id="42"] { view-transition-name: ex-card-42; }
.exercise[data-workout-exercise-id="42"] .exercise-name::after { view-transition-name: ex-strike-42; }
```

Non-completed cards should have only the `ex-card-{ID}` rule, not the strike
rule. Also confirm the `html:active-view-transition-type(state-refresh)`
block contains one `&::view-transition-new(ex-strike-{ID})` rule per
completed card, and the `@keyframes strike-draw` block is present exactly
once.

- [ ] **Step 3: Visual verification deferred to Task 6**

The strike-draw animation isn't visible until the bfcache→replace flow
activates and adds the `state-refresh` type. Skip visual verification here.

- [ ] **Step 4: Commit**

```
git add ui/templates/pages/workout/workout.gohtml
git commit -m "Add strike-draw animation gated on state-refresh transition type"
```

---

## Task 5 — workout.gohtml: reduced-motion override for the strike draw

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml` (extend the dynamic `<style {{ nonce }}>` block from Task 4)

`main.css:670-674` already collapses the `page`-level transition to ~1ms
under `prefers-reduced-motion: reduce`. The `strike-draw` keyframe needs
the same treatment so the line snaps instead of drawing. Per-card color
crossfades use the UA-stylesheet default fade animations which already
respect reduced motion, so they don't need explicit overrides.

- [ ] **Step 1: Add the `@media (prefers-reduced-motion: reduce)` block**

Inside the same `<style {{ nonce }}>` block from Task 4, after the
`@keyframes strike-draw` declaration and before `</style>`, add:

```gohtml
    @media (prefers-reduced-motion: reduce) {
        html:active-view-transition-type(state-refresh) {
            {{ range .Exercises }}{{ if eq .State "completed" }}
            &::view-transition-new(ex-strike-{{ .ID }}) {
                animation-duration: 0.001ms;
            }
            {{ end }}{{ end }}
        }
    }
```

The full block now reads:

```gohtml
<style {{ nonce }}>
    {{ range .Exercises }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] { view-transition-name: ex-card-{{ .ID }}; }
    {{ if eq .State "completed" }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] .exercise-name::after { view-transition-name: ex-strike-{{ .ID }}; }
    {{ end }}
    {{ end }}

    html:active-view-transition-type(state-refresh) {
        &::view-transition-old(page),
        &::view-transition-new(page) {
            animation: none;
        }
        {{ range .Exercises }}{{ if eq .State "completed" }}
        &::view-transition-new(ex-strike-{{ .ID }}) {
            animation: strike-draw 320ms cubic-bezier(.2, .7, .3, 1) both;
            transform-origin: left center;
        }
        {{ end }}{{ end }}
    }

    @keyframes strike-draw {
        from { transform: scaleX(0); }
        to   { transform: scaleX(1); }
    }

    @media (prefers-reduced-motion: reduce) {
        html:active-view-transition-type(state-refresh) {
            {{ range .Exercises }}{{ if eq .State "completed" }}
            &::view-transition-new(ex-strike-{{ .ID }}) {
                animation-duration: 0.001ms;
            }
            {{ end }}{{ end }}
        }
    }
</style>
```

- [ ] **Step 2: Visual verification deferred to Task 6**

Same as Task 4: the reduced-motion override only matters during the
`state-refresh` transition which isn't active yet. Verify in Task 6.

- [ ] **Step 3: Commit**

```
git add ui/templates/pages/workout/workout.gohtml
git commit -m "Honor prefers-reduced-motion for the strike draw"
```

---

## Task 6 — workout.gohtml: bfcache-handler script (opt-out, await-then-replace, URL cleanup, type swap)

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml` (new `<script {{ nonce }}>` block)

This is the behavioral wire-up. The script sets the opt-out flag at parse
time, listens for `pagereveal`, handles the URL cleanup on arrival, swaps
the navigation type to `state-refresh`, and (when the bfcache token is
stale) awaits the backward traverse VT and then fires the replace
navigation with the cache-buster query.

- [ ] **Step 1: Add the bfcache-handler script**

Find the existing `<script {{ nonce }}>` block at lines 343-389 (the
title-morph script). You can either extend that block or add a new block
adjacent to it. Adding adjacent is cleaner (the two scripts handle
unrelated concerns — title morph vs. bfcache).

Add the following `<script {{ nonce }}>` block immediately before the
closing `</div>` at line 390 (so it sits inside the exercise-list container,
adjacent to the title-morph script):

```gohtml
<script {{ nonce }}>
    // Opt out of main.js's global bfcache reload (ui/static/main.js:300+);
    // we handle invalidation here with a replace navigation that participates
    // in cross-document view transitions. The dataset write happens at parse
    // time, before pageshow fires.
    document.body.dataset.bfcacheHandler = 'page-local'

    // Cache-buster query used to keep navigation.navigate(SAME_URL, ...) from
    // being reclassified as 'reload' by Chromium when fired from a deferred
    // context (after `await viewTransition.finished`). Stripped on arrival.
    const STATE_REFRESH_QUERY = '?_inv=1'

    window.addEventListener('pagereveal', async (e) => {
        if (e.viewTransition && navigation.activation?.navigationType === 'replace') {
            // Swap the global 'forward' type added by main.js for our custom
            // type so the page-slide suppression and strike-draw rules apply
            // only to this transition, not to regular forward navigations.
            e.viewTransition.types.delete('forward')
            e.viewTransition.types.add('state-refresh')
        }

        // Clean the cache-buster from the URL on the arrival pagereveal.
        if (location.search === STATE_REFRESH_QUERY) {
            history.replaceState(null, '', location.pathname)
        }

        const meta = document.querySelector('meta[name="invalidation-token"]')
        const rendered = meta ? meta.content : ''
        const m = document.cookie.match(/(?:^|;\s*)inv_bfcache=([^;]+)/)
        const current = m ? m[1] : ''
        if (rendered === current) return

        if (e.viewTransition) {
            try { await e.viewTransition.finished } catch (_) { /* skipped */ }
        }

        const target = location.pathname + STATE_REFRESH_QUERY
        if ('navigation' in window) {
            navigation.navigate(target, { history: 'replace' })
        } else {
            location.replace(target)
        }
    })
</script>
```

- [ ] **Step 2: Verify rendered HTML**

Reload `/workouts/<date>` and view source. Confirm the new `<script>` block
sits immediately before the closing `</div>` of the exercise list (next to
the existing title-morph script). The opt-out line `document.body.dataset.bfcacheHandler = 'page-local'` should be the first
non-comment line inside the script.

- [ ] **Step 3: Verify the opt-out is reachable from main.js**

Open DevTools console on `/workouts/<date>`. Type:

```js
document.body.dataset.bfcacheHandler
```

Expected output: `'page-local'`.

- [ ] **Step 4: Full flow — verify the bfcache→replace transition runs**

Start (or refresh) the dev server. In a Chromium-based browser with DevTools
open:

1. Log in and navigate to `/workouts/<date>` for today, with an in-progress
   exercise (sets started but not all done).
2. Open Console; you should not see any errors.
3. Optional: add a temporary `console.log('[pagereveal]', { hasVT:
   !!e.viewTransition, navType: navigation.activation?.navigationType })`
   at the top of the pagereveal handler to trace.
4. Tap the in-progress card → exerciseset page.
5. Mark the last remaining set done (the exercise flips to `completed`).
6. Tap the back link (or use the back button).

Expected: backward slide-right plays first (the existing `backward`
page-level animation from main.css), then the just-completed card crossfades
amber→green and a thin green line draws across the exercise name from left
to right over ~320ms. Other completed-from-before cards do not redraw
(identical snapshots crossfade no-op).

If you added the trace log, you should see two `[pagereveal]` events:
- `{ hasVT: true, navType: 'traverse' }` (the bfcache restore)
- `{ hasVT: true, navType: 'replace' }` (our replace navigation)

If the second pagereveal shows `navType: 'reload'`, the cache-buster trick
isn't working — recheck the call site uses `location.pathname +
STATE_REFRESH_QUERY` (not `location.href`).

If the strike just fades in instead of drawing, the `state-refresh` type
isn't being added before the browser starts animating. Check the
`e.viewTransition.types.add('state-refresh')` call runs before any `await`
in the handler.

**Remove any temporary `console.log` before committing.**

- [ ] **Step 5: Verify the address bar cleans up**

After the bfcache→replace flow completes (from Step 4), the address bar
should show `/workouts/<date>` (no lingering `?_inv=1`). The brief flash
during the replace is expected.

- [ ] **Step 6: Verify reduced-motion behavior**

DevTools → Rendering panel → "Emulate CSS media feature
`prefers-reduced-motion`" → `reduce`. Repeat the bfcache→replace repro from
Step 4. The card colors should snap (existing page-level reduced-motion
override handles this) and the strike line should appear without any
visible draw — instantly at full width. Compare against the same flow with
reduced motion off (the strike should draw smoothly).

- [ ] **Step 7: Commit**

```
git add ui/templates/pages/workout/workout.gohtml
git commit -m "Animate exercise-card state changes via bfcache replace"
```

---

## Task 7 — Run the spec's full test plan and lint

**Files:** none modified (verification only). If issues turn up, file
follow-up tasks rather than fixing in-line — each fix should be its own
commit with its own justification.

- [ ] **Step 1: Manual test matrix (from the spec)**

Run through each case in [`spec § Test plan`](../specs/2026-05-17-card-state-view-transitions-design.md#test-plan).
Expected outcomes:

1. *Complete an exercise's last set, navigate back.* Backward slide plays
   first, then completed card crossfades amber/cream → green and the
   strike line draws.
2. *Re-open an already-completed exercise and back out without changes.*
   Cookie unchanged → no replace fires → instant return, no animation.
3. *Start (but don't complete) sets on a different card; back out.* That
   card crossfades cream → amber. No strike (still not completed).
4. *Repeat (1) with `prefers-reduced-motion: reduce`.* Strike appears
   without the draw; card colors snap; backward slide already collapses
   via the global rule at main.css:670-674.
5. *Cold-load `/workouts/<date>` with mixed states.* Strike lines render
   at full width on completed cards. No animation on first paint.
6. *Cold-load `/workouts/<date>?_inv=1` directly.* Address bar cleans to
   `/workouts/<date>` within one frame; no animation; no errors.
7. *After the bfcache→replace flow completes, address bar shows
   `/workouts/<date>` (no lingering `?_inv=1`).* Brief flash during the
   replace is expected.

For each, note pass/fail. Any failure is a spec-violation regression —
investigate before claiming done.

- [ ] **Step 2: Run `make lint-fix` and `make test`**

```
make lint-fix && make test
```

Expected: clean lint output (no diff), all tests pass. The template change
has no Go signature impact, so a clean test pass plus clean lint is
sufficient. main.js isn't part of golangci-lint's scope.

- [ ] **Step 3: Final commit (only if Step 2 produced changes)**

If `make lint-fix` modified anything, stage only those changes (not the
unrelated PoC files in the working tree) and commit:

```
git add -u
git status   # confirm only intended files are staged; if any PoC files
             # are also staged, unstage them with `git restore --staged <path>`
git commit -m "Lint-fix"
```

Otherwise no commit.

- [ ] **Step 4: Mark plan complete**

In your subagent/inline-execution status, report: plan complete, N commits,
links to commits, brief one-line summary of verified outcomes from Step 1.

---

## Notes for the implementer

- **Don't break the existing title morph.** The `pageswap`/`pagereveal`
  script at lines 343-389 is unrelated to this change — leave it alone. It
  handles a different transition (workout list ↔ exerciseset page, title
  element). The bfcache-handler script added in Task 6 rides on a different
  navigation (`replace`, same URL).
- **Don't add `view-transition-name` to elements via inline `style=`
  attributes.** The CSP nonce mode disables inline styles entirely (see
  `ui/templates/CLAUDE.md`). Always emit via a nonce'd `<style>` block.
- **Don't mass-rename `data-workout-exercise-id`.** It's already used by
  the title-morph script (lines 359, 379). The pattern is
  `data-workout-exercise-id="{{ .ID }}"`; reuse, don't replace.
- **The worktree may have uncommitted PoC files.** See the "Working-tree
  note" above. Use `git add <specific-file>` rather than `git add -A`.
- **Test on a real device too if possible.** The bfcache path behaves
  slightly differently on iOS Safari and mobile Chromium than on desktop
  Chromium. The graceful fallback for any browser that doesn't run the
  cross-doc transition is "page swaps without animation" — same as today's
  reload behavior.
