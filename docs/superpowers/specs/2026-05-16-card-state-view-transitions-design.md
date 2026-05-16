# Card-state view transitions on bfcache reload

Status: draft
Owner: Martin
Created: 2026-05-16

## Goal

When the user returns to the workout list after changing an exercise's state
on the exerciseset page, the affected card should *visibly transform* into
its new skin instead of snapping. Specifically: the card's color crossfades
from idle/started/completed to its new color, and a newly-completed card's
strikethrough line draws across the exercise name from left to right.

This only happens on the post-bfcache reload. Cards whose state did not
change stay still. The forward navigation (workout list → exerciseset) keeps
its existing title morph (`exercise-title-{ID}` in
`ui/templates/pages/workout/workout.gohtml:343-389`) — out of scope here.

## The mechanic this hangs on

The app already has a bfcache invalidation path
(`ui/static/main.js:298-319`): on `pageshow` with `event.persisted`, if the
`inv_bfcache` cookie no longer matches the rendered `<meta>` token, the page
fires `navigation.reload()`. That reload is a cross-document navigation —
old document = bfcached (stale) workout list, new document = freshly
rendered (current) workout list — and per the View Transitions Level 2 spec
under `@view-transition { navigation: auto }` (already set in
`ui/static/main.css:580-582`), the browser captures snapshots from both and
runs `pageswap` / `pagereveal` lifecycle around it.

Because both documents render the same workout-list template against the
same exercise set, element matching is trivial: each card with the same
`view-transition-name` pairs up automatically. Color and strike state differ
between snapshots — that difference *is* the animation.

## Visual specification

### Per-card crossfade

Each `.exercise` anchor in the list gets a unique
`view-transition-name: ex-card-{ID}`. With paired names, the browser
captures the old card pixels (e.g. amber) and the new card pixels (e.g.
green), then runs the default `::view-transition-old/new` crossfade. Because
the card's geometry is identical between the two documents (same layout,
same content shape), the morph is a pure color/content crossfade, not a
shape interpolation.

Duration: inherits `var(--duration-4)` from the existing
`::view-transition-group(page)` rule (`main.css:592-594`); no override
needed at the per-card level.

Cards whose state didn't change produce identical old/new snapshots; the
crossfade is visually a no-op. Browser cost is one extra named element per
card per transition — acceptable at the ~3–8 cards-per-list scale.

### Strikethrough draw

Today the strikethrough is `text-decoration: line-through` on
`.exercise.completed .exercise-name` (`workout.gohtml:243-247`). View
transitions snapshot DOM as bitmap, so `text-decoration` would just fade in
with the rest of the card crossfade — no "draw" effect.

Replace with a pseudo-element line:

- `.exercise-name` becomes `position: relative`.
- A `::after` line is positioned at the text's vertical midline (`top: 50%;
  transform: translateY(-50%)`), `width: 100%`, `height: 1.5px`,
  `background: color-mix(in oklab, var(--color-success) 50%, transparent)`.
- The line is only rendered when the card is `.completed`. Default state:
  `transform: scaleX(1)` (so cold-loads and reloads where the card was
  *already* completed show a fully-drawn line).

Give the line its own named transition: `view-transition-name:
ex-strike-{ID}` on the `::after`. Because the line exists in the new
snapshot but *not* the old (the card was un-completed before the reload),
the browser runs the default "new element" path, which we override:

```css
::view-transition-new(ex-strike-{ID}) {
    animation: strike-draw 320ms cubic-bezier(.2, .7, .3, 1) both;
    transform-origin: left center;
}
@keyframes strike-draw {
    from { transform: scaleX(0); }
    to   { transform: scaleX(1); }
}
```

Asymmetry handles selectivity automatically:

- *Newly completed* card → strike line in new, not in old → custom draw
  runs.
- *Already completed* before and after → line in both → identical-snapshot
  crossfade, no replay.
- *Un-completed* (rare; e.g. user deleted a logged set) → line in old, not
  in new → default "old element fade out". No special handling.

### Reduced motion

Existing rule at `main.css:670-674` already collapses view-transition
animations to ~1ms under `prefers-reduced-motion: reduce`. The
`strike-draw` keyframe needs a matching override:

```css
@media (prefers-reduced-motion: reduce) {
    ::view-transition-new(ex-strike-*) { animation-duration: 0.001ms; }
}
```

(Using the wildcard selector form: if browser support is uneven, we list
named selectors emitted from the template loop alongside the per-card
`view-transition-name` rules.)

### Forced colors

The strike line uses a semantic color via `color-mix`; under
`forced-colors: active` it should fall back to `LinkText` or `CanvasText`
so the line remains visible. Add a rule in the colocated `<style>`.

## Files touched

- `ui/templates/pages/workout/workout.gohtml` — per-card
  `view-transition-name` declarations emitted inside the existing
  `<style {{ nonce }}>` block by ranging over `.Exercises`; rewrite of the
  `.exercise.completed` strikethrough to use a `::after` line with its own
  named transition; addition of the `strike-draw` keyframe and reduced-
  motion override. The existing `pageswap`/`pagereveal` script is
  unchanged — it remains responsible for the title morph only.
- (No JS changes.) Cross-doc transitions fire automatically; no
  `pageswap`/`pagereveal` coordination is needed for the cards because
  both documents are workout-list renders with matching DOM shape.
- (No Go changes.) Exercise IDs are already in the template data
  (`workout.gohtml:322` uses `.ID`).
- (No `main.css` changes.) All new rules are scoped to `.exercise-list`
  and live in the page template's `<style>`.

## Pre-flight checks before implementation

These are smoke tests, not architectural decisions — list them in the
implementation plan as the first verification step.

1. **`navigation: auto` covers reload navigations in production Chromium.**
   Log inside `pageswap`/`pagereveal` listeners and force the bfcache→reload
   path (POST something, navigate away, come back). Confirm the listeners
   fire and `e.viewTransition` is non-null. If a Chromium release we
   support omits reload from `auto`, the fallback is wrapping the reload
   in a manual same-document transition before invoking it — but the spec
   and current Chromium ship the auto behavior, so this is verification
   only.
2. **`view-transition-name` on `::after` pseudo-elements is honored.** Per
   spec, pseudo-elements participate in view transitions; Chromium
   supports it. Verify in the actual app by inspecting
   `::view-transition` pseudo-tree in DevTools during the transition.
3. **Identical-snapshot crossfades are visually a no-op.** Confirm by
   completing one card and reloading: the other cards should not flicker.
   If browsers ever draw a fade even for identical pixels, we'd need to
   gate `view-transition-name` to only the changed card by diffing in the
   `pageswap` handler. Not expected.

If (1) fails on a target browser, the design degrades to a hard reload
(no transition) — same as today. No regression.

## No-JS / pre-VT browser fallback

- No JS: bfcache invalidation reload doesn't fire (handler doesn't run), so
  the stale page is what the user sees. Pre-existing behavior; not our
  problem to solve here.
- VT-unsupported browser (Safari pre-cross-doc-VT): reload is a hard
  document swap; the new state appears instantly. No regression vs. today.
- Strike line still renders correctly on cold loads in all browsers because
  the `::after` defaults to `scaleX(1)`.

## Out of scope

- Forward-direction card-to-brief morph (the "shared card" idea from
  brainstorming option A). Could layer on later — the per-card
  `view-transition-name` machinery built here is a prerequisite — but a
  separate motion language; keeping this round focused on the bfcache
  reload moment.
- Animating the dot indicators (`.exercise-dot`) independently. They're
  covered by the per-card crossfade already.
- The left amber rail's color shift on started → completed. Also covered
  by the per-card crossfade.
- Any change to the title morph between workout list and exerciseset
  pages. Existing behavior preserved.

## Test plan

Manual:

1. Complete an exercise's sets on the exerciseset page; navigate back.
   Bfcache restores stale list → cookie mismatch → reload fires →
   completed card crossfades amber → green and the strike line draws.
2. Re-open the same card (already completed) and back out without
   changing anything. No state change → cookie unchanged → no reload, or
   reload with identical state → no visible animation.
3. Start (but don't complete) sets on a different card; back out. That
   card crossfades cream → amber.
4. With `prefers-reduced-motion: reduce`, repeat (1). Strike line appears
   without the draw; card colors snap.
5. Cold load on `/workouts/<date>` with a mix of states. Strike lines
   render at full width; no animations on first paint.

Automated coverage isn't really applicable here — view transitions are
not asserted in the existing `e2etest` flow and adding a Playwright check
just for animation timing would be brittle. The structural change
(strike-line `::after` replacing `text-decoration`) is implicitly covered
by existing handler tests asserting card state classes; nothing else
needs an assertion.
