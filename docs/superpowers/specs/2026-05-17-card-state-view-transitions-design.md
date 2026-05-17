# Card-state view transitions on bfcache replace

Status: draft
Owner: Martin
Created: 2026-05-17

Supersedes [`2026-05-16-card-state-view-transitions-design.md`](2026-05-16-card-state-view-transitions-design.md)
(and the matching plan at `docs/superpowers/plans/2026-05-16-card-state-view-transitions.md`).
A PoC built against the original spec validated the visual design but disproved
its load-bearing mechanic — `@view-transition { navigation: auto }` does not
fire on the `navigation.reload()` triggered from inside `pageshow`. This spec
re-grounds on a `navigation.navigate(..., { history: 'replace' })` mechanism
that does work, with a small cache-buster trick to keep Chromium from
reclassifying the same-URL navigate as a reload when deferred.

## Goal

Unchanged from the prior spec. When the user returns to `/workouts/{date}`
after changing an exercise's state on the exerciseset page, the affected card
should *visibly transform* into its new skin instead of snapping. Specifically:
the card's color crossfades from idle/started/completed to its new color, and
a newly-completed card's strikethrough line draws across the exercise name
from left to right.

Cards whose state did not change stay still. The forward navigation
(workout list → exerciseset) keeps its existing title morph (`exercise-title-{ID}`
in `ui/templates/pages/workout/workout.gohtml:343-389`) — out of scope here.

## What the PoC taught us

Findings from the `/dev/transitions-poc` exploration (no longer in the repo;
referenced here for posterity):

1. **`navigation.reload()` from inside `pageshow` does not get a cross-doc VT
   in Chromium.** Even with `@view-transition { navigation: auto }` set, the
   `pagereveal` for that reload reports `event.viewTransition === null`.
   The prior spec assumed this would work; it does not. This is the
   single change that invalidates the prior approach.
2. **`navigation.navigate(location.href, { history: 'replace' })` from
   inside `pageshow` does get a cross-doc VT** (`pagereveal` reports
   `event.viewTransition` non-null, `navType === 'replace'`). But firing
   synchronously from `pageshow` aborts the in-flight backward traverse VT
   the browser was running for the bfcache restore, so the user sees no
   slide-right and the experience is jarring.
3. **Firing the navigate from `pagereveal` *after* `await
   e.viewTransition.finished`** (so the slide-right has a chance to play)
   causes Chromium to reclassify a same-URL `navigate(_, { history: 'replace' })`
   as a `'reload'` — and reloads, per (1), don't get cross-doc VT.
4. **Cache-buster query trick:** firing
   `navigation.navigate(location.pathname + '?_inv=1', { history: 'replace' })`
   after the await gives Chromium a URL that differs from the current
   document's URL, so it stays classified as `'replace'` and gets a fresh
   cross-doc VT. The query is stripped from the address bar on arrival via
   `history.replaceState`. The server ignores the unknown query.
5. The global `pagereveal` handler in `ui/static/main.js:237` calls
   `e.viewTransition.skipTransition()` for both `'replace'` and `'reload'`
   navigation types. That suppresses the replace VT we want. Narrow the
   condition to `'reload'` only.
6. The global `pageshow` handler in `ui/static/main.js:298` fires
   `navigation.reload()` on bfcache-cookie-mismatch for every page in the
   app. It would preempt our page-local replace flow. The workout page
   opts out via a body data attribute that the global handler checks.

## Architecture

### Control flow (page-local, in `workout.gohtml`)

A `<script {{ nonce }}>` block at the bottom of the exercise list:

1. **At parse time**, before any event fires, mark the body as page-local:
   `document.body.dataset.bfcacheHandler = 'page-local'`. This is read by
   `main.js`'s global pageshow handler to skip its reload.
2. **On every `pagereveal`:**
   1. If the navigation is `'replace'` and has a `viewTransition`, swap the
      `'forward'` type that `main.js` just added for `'state-refresh'`. This
      lets us scope the page-level animation suppression and per-card draw
      keyframes to *only* the state-refresh transition without affecting
      regular forward navigations to the workout page from elsewhere.
   2. If `location.search === '?_inv=1'`, strip it via
      `history.replaceState(null, '', location.pathname)`. Runs on the
      arrival pagereveal of the replace navigation.
   3. Read the rendered invalidation token (from
      `<meta name="invalidation-token">`) and the current `inv_bfcache`
      cookie. If they match, return early — no work to do.
   4. If `e.viewTransition` exists (the backward traverse VT from the
      bfcache restore), `await e.viewTransition.finished` so the slide-right
      plays out before we fire the replace. Wrap in a try/catch since the
      promise rejects on skip.
   5. Call `navigation.navigate(location.pathname + '?_inv=1',
      { history: 'replace' })`. Fall back to
      `location.replace(location.pathname + '?_inv=1')` for browsers
      without the Navigation API.

The user perceives: backward swipe/click → page slides right (browser-native
backward traverse animation) → completed card crossfades color and strike line
draws across the name. Cards that didn't change stay perfectly still.

### `main.js` changes (two small tweaks)

1. **`pageshow` handler (line 298):** opt-out check right after `clearLoad()`:
   ```js
   if (document.body.dataset.bfcacheHandler === 'page-local') return
   ```
   Pages that handle their own bfcache invalidation get to do so without
   competition from the global reload.
2. **`pagereveal` handler (line 237):** narrow the `skipTransition` guard
   from `(replace || reload)` to `reload` only:
   ```js
   if (act.navigationType === 'reload') {
       e.viewTransition.skipTransition()
       return
   }
   ```
   Replace navigations are allowed through so the cross-doc VT can animate.

## Visual specification

### Strike line (replaces `text-decoration: line-through`)

The original spec's design, intact:

```css
.exercise-name { position: relative; }

.exercise.completed .exercise-name {
    color: var(--color-success);
}
.exercise.completed .exercise-name::after {
    content: "";
    position: absolute; left: 0; right: 0; top: 50%;
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

The default `scaleX(1)` keeps the line drawn on cold loads — the animation
only runs during the `state-refresh` transition when the card newly transitions
into the completed state.

### Per-card view-transition names (dynamic block, emitted by ranging over `.Exercises`)

```gohtml
<style {{ nonce }}>
    {{ range .Exercises }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] {
        view-transition-name: ex-card-{{ .ID }};
    }
    {{ if eq .State "completed" }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] .exercise-name::after {
        view-transition-name: ex-strike-{{ .ID }};
    }
    {{ end }}
    {{ end }}

    html:active-view-transition-type(state-refresh) {
        /* Suppress the global page slide so per-card snapshots dominate. */
        &::view-transition-old(page),
        &::view-transition-new(page) {
            animation: none;
        }
        /* Strike draw on newly-completed cards. */
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

### Why the per-card animations are type-gated

`view-transition-name` declarations apply to *every* navigation, so a per-card
name participates in both the backward traverse VT (where it appears as a
"new element" because the exerciseset page snapshot doesn't have it) and our
state-refresh VT. To avoid the per-card defaults firing during the traverse
slide, the `::view-transition-new(ex-strike-N)` animation override is wrapped
in `html:active-view-transition-type(state-refresh)` — so it runs only when
our custom type is active.

The per-card color crossfade (the `ex-card-{ID}` pair) is *not* explicitly
animated; we rely on the default `::view-transition-old/new` UA-stylesheet
crossfade. During backward traverse the card snapshot fades in on top of the
page slide — a small visual artifact accepted in the PoC and confirmed
acceptable here. If the artifact ever bothers us, the fix is symmetric to the
strike override:
```css
html:active-view-transition-type(backward) {
    &::view-transition-new(ex-card-N), … { animation: none; }
}
```

## Files touched

| File | Change |
|---|---|
| `ui/templates/pages/workout/workout.gohtml` | Replace `text-decoration: line-through` rule with strike-line `::after`. Add new dynamic `<style {{ nonce }}>` block emitting per-card and per-strike VT names, type-gated animation overrides, and reduced-motion rules. Add bfcache→replace handler — either as a new `<script {{ nonce }}>` block or by extending the existing one at lines 343-389 (the title-morph script); implementer's choice. |
| `ui/static/main.js` | `pageshow` opt-out (1 line); narrow `pagereveal` skip condition (3 chars: `replace ||` removed). |

No Go changes. No new files.

## Pre-flight checks before implementation

Smoke-test items, lighter than the prior spec because the PoC already burned
through the load-bearing ones. Each is a 1-minute sanity check.

1. **`document.body.dataset.bfcacheHandler` is readable in main.js's
   pageshow handler at the time it runs.** It is set synchronously by the
   page-local script at parse time, before pageshow fires. Verify by adding
   a temporary `console.log` in main.js's pageshow handler.
2. **The page-local pagereveal listener registers in time to fire on the
   first pagereveal.** It does — pagereveal fires after parse, and inline
   scripts register synchronously during parse.
3. **`navigation.navigate(_, { history: 'replace' })` with a cache-buster
   query still gets `navType === 'replace'` and `hasVT === true` when fired
   from `pagereveal` after `await viewTransition.finished` on `/workouts/{date}`.**
   PoC confirmed this on `/dev/transitions-poc/exercise-sets`; the workout
   list is the same per-page setup so this should hold, but reverify on the
   actual route during implementation.

If (3) fails on `/workouts/{date}` specifically (it didn't fail on the PoC,
but Chromium navigation behavior is subtle), the fallback is the synchronous
fire from `pageshow` — losing the traverse-then-replace sequencing but
keeping the per-card animations. We'd document this regression in the spec
and ship anyway. No further unblocking is needed.

## No-JS / pre-VT browser fallback

- **No JS:** bfcache invalidation handler doesn't run; the bfcached stale
  page stays visible. Same as today. Not our problem here.
- **VT-unsupported browser (Safari pre-cross-doc-VT):** the replace
  navigation still happens (we have the `location.replace` fallback for
  no-Navigation-API browsers; modern Safari has the Navigation API and
  `navigate({ history: 'replace' })`). The cross-doc VT just doesn't run,
  so the page swaps without animation. No regression vs. today's reload
  behavior.
- **Strike line renders correctly on cold loads in all browsers** because
  the `::after` defaults to `scaleX(1)`. Same as the prior spec.

## Out of scope

- Forward-direction card-to-brief morph (the "shared card" idea from
  brainstorming option A). Could layer on later — the per-card
  `view-transition-name` machinery built here is a prerequisite — but a
  separate motion language; keeping this round focused on the bfcache
  reveal moment.
- Animating the dot indicators (`.exercise-dot`) independently. They're
  covered by the per-card crossfade already.
- The left amber rail's color shift on started → completed. Also covered
  by the per-card crossfade.
- Any change to the title morph between workout list and exerciseset
  pages. Existing behavior preserved.
- Restoring the forward history entry that the replace prunes. Accepted
  trade-off; users rarely forward-traverse on mobile, and re-tapping the
  card creates a fresh push if they want to.
- Generalizing the bfcache→replace pattern to other pages. The
  `data-bfcache-handler="page-local"` opt-out is shaped to allow it later,
  but no other page is in scope today.

## Test plan

Manual:

1. Complete an exercise's last set on the exerciseset page; tap back.
   Expected: backward slide plays first, then the completed card crossfades
   amber/cream → green and the strike line draws.
2. Re-open an already-completed exercise and back out without changing
   anything. Cookie unchanged → no replace fires → instant return, no
   animation.
3. Start (but don't complete) sets on a different card; back out. That card
   crossfades cream → amber. No strike (still not completed).
4. With `prefers-reduced-motion: reduce`, repeat (1). Strike appears without
   the draw; card colors snap; backward slide already collapsed by the
   existing global reduced-motion rule at `main.css:670-674`.
5. Cold-load `/workouts/<date>` with a mix of states. Strike lines render at
   full width; no animations on first paint.
6. Cold-load `/workouts/<date>?_inv=1` directly (simulating a shared
   cache-busted URL). Address bar cleans to `/workouts/<date>` within one
   frame; no animation; no errors.
7. After the bfcache→replace flow completes, address bar shows
   `/workouts/<date>` (no lingering `?_inv=1`). Brief flash during the
   replace is expected.

Automated coverage is not applicable — view transitions are not asserted in
the existing `e2etest` flow and adding a Playwright check just for animation
timing would be brittle. The structural change (strike `::after` replacing
`text-decoration`, per-card VT names) is implicitly covered by existing
handler tests asserting card state classes; nothing else needs an assertion.
