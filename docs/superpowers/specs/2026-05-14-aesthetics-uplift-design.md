# Aesthetics & UX Uplift — Design Spec

**Date:** 2026-05-14
**Status:** Approved (brainstorm), ready for implementation planning

## Goal

Take PetraApp's visual identity from "functional but generic" to a distinctive,
polished product. The frontend foundation (design tokens, layout primitives,
shared components, `/dev/styleguide`) already exists and is sound — this work
applies a deliberate **design direction** on top of it rather than rebuilding the
plumbing.

The approach is **visual-led**: a chosen visual direction drives the work, and
UX improvements (notably the in-workout "Focus mode") come along with it.

## Decisions locked in brainstorm

| Question | Decision |
| --- | --- |
| Primary gap | Both visual and UX, **visual-led** |
| Design direction | **Stone** — warm, organic, friendly. Sand/clay palette playing on "Petra" = stone. An approachable coach, not a drill sergeant. |
| In-workout screen | **Focus mode** — the active set becomes a dark, high-contrast panel with oversized numbers. Borrows glanceability into the otherwise-calm Stone system. |
| Typography | **System fonts, refined** — `system-ui` everywhere, used with discipline (weights, scale, letter-spacing). No custom webfonts. Zero load cost, no CSP changes, no Dockerfile impact. The current serif body face is dropped. |
| Rollout staging | **Foundation-first, then traffic-ordered** — repaint the token + component layer so the whole app shifts at once, then polish pages in usage order. |

## The Stone design language

### Palette

Three new ramps join `@layer props` in `ui/static/main.css`:

**Stone** — warm neutral ramp for surfaces, borders, and text. Replaces the cool
`--gray-*` role.

```
--stone-0  #faf7f2   --stone-4  #c3b5a0   --stone-8  #3d342b
--stone-1  #f4efe6   --stone-5  #9b8e7d   --stone-9  #2e2620
--stone-2  #ece4d9   --stone-6  #766b5c   --stone-10 #1c1813
--stone-3  #ddd2c1   --stone-7  #574e42
```

**Clay** — primary accent for buttons, links, and active state. Replaces
`--sky-*` as the *action* color.

```
--clay-0  #fbf1e8   --clay-3  #c98a55   --clay-6  #6b3c22
--clay-1  #f0d9c4   --clay-4  #a8643c
--clay-2  #e0b48a   --clay-5  #8a4f2e
```

**Ember + warmed functional colors** — `--ember #e08a4c` is the single bright
accent, reserved for the Focus-mode "log set" CTA on the dark panel. The
functional colors are warmed to sit in the palette: success `#7d8a4a`, warning
`#d99a2b`, error `#c0532f`, info `#5b7a8a`. (Exact functional values may be
tuned during the component pass; the hexes above are the target direction.)

### Token strategy

The key move that makes "foundation-first" cheap:

- **Retune `--gray-0..10` hex values in place** to the Stone ramp — *same token
  names, new values*. Every `--gray-*` call site across the scoped `<style>`
  blocks warms automatically with a tiny diff. This is not a backwards-compat
  shim; it is the palette's values changing.
- Add `--clay-0..6` and `--ember` as new tokens.
- Re-point the existing semantic tokens (`--color-surface`,
  `--color-surface-elevated`, `--color-surface-active`,
  `--color-surface-completed`, `--color-border`, `--color-border-focus`,
  `--color-text-primary/secondary/muted`, `--color-success`, `--color-info`,
  and their `-bg` variants) onto the Stone/Clay ramps.
- `--sky-*`, `--lime-*`, `--yellow-*`, `--red-*` stay for now; they are warmed
  during the component pass (Increment 1) and their call sites cleaned up as
  each page is polished.
- Soften radii slightly (cards a touch rounder) and warm-tint the `--shadow-*`
  values (currently pure-black rgba → warm-brown rgba).

### Typography

- Drop the serif body face: the `body` rule in `@layer reset` switches from
  `--font-serif` to `--font-sans` (`system-ui`).
- Headings stay `system-ui`. The personality comes from a **tightened,
  deliberate type scale and weight usage**, not from a typeface.
- No new font files → `Dockerfile` fingerprinting and the importmap are
  untouched.

### Focus mode (in-workout)

The exercise-set screen (`ui/templates/pages/exerciseset/`) gains a two-tier
visual system:

- Completed and upcoming sets render as calm Stone cards (consistent with the
  rest of the app).
- The **active set** renders as a dark `--stone-9` panel with oversized numbers
  (weight × reps), a clear `--ember` "LOG SET" CTA, and high contrast — built to
  glance at mid-set with sweaty hands.

The handler already computes active-set state (see commits "Compute exercise-set
active state in the handler", "Migrate sets-container to .card and
handler-derived active state"), so this is primarily a scoped-CSS + markup
restructure with minimal Go change.

## Architecture — how it threads into the codebase

1. **Token layer** — all changes land in `main.css` `@layer props` (new ramps,
   retuned `--gray-*`, re-pointed semantic tokens) and `@layer reset` (body
   font). No new files.
2. **Shared components** — `button`/`.btn`, `.card`, `.badge` (+ variants) in
   `main.css` `@layer components`; the `page-header`, `back-link`, `banner`,
   `field` partials in `ui/templates/components/*.gohtml` (colocated `@scope`
   styles). `/dev/styleguide` is updated alongside, and
   `cmd/web/handler-styleguide_test.go` stays green — the regression net for the
   component layer.
3. **Per-page polish** — each page's scoped `<style>` blocks are cleaned up in
   traffic order. As a page is touched, its raw-token holdouts are fixed and
   `--gray-*` references are renamed to `--stone-*`. The `--gray-*` name retires
   organically by the end of Increment 5 — no big-bang rename commit.
4. **Focus mode** — scoped to `pages/exerciseset/` (`exerciseset.gohtml`,
   `sets-container.gohtml`, `exercise-header.gohtml`, `warmup.gohtml`).
5. **Motion** — within-page microinteractions layered on top of the existing
   view transitions, all gated behind `prefers-reduced-motion` (an established
   pattern in `main.css`).

## Constraints & non-goals

- **No new fonts, no new dependencies.** Dockerfile fingerprinting untouched.
- **CSP unaffected** — all styles stay nonce'd inline `<style>` blocks or
  `main.css`; no new external resources.
- **Test compatibility** — DOM-structure and scoped-style changes risk
  Playwright / `goquery` DOM-selector tests. Follow the CLAUDE.md selector
  guidance (specific selectors, unique identifiers); update selectors as
  structure changes. Recent history shows this is a live concern ("Fix stale
  Playwright back-link selector after component swap").
- **Every increment ends green on `make ci`** and is independently shippable.
- **Not in scope:** dark mode as a user setting (dark is used purposefully for
  Focus-mode surfaces only, not as a global theme), custom typefaces, new
  features or flows beyond Focus mode, backend/domain changes beyond what Focus
  mode's markup needs.

## Increment plan

One design spec → one implementation plan with six phases. Each ends green on
`make ci`.

### Increment 1 — Token & component repaint (foundation)

Retune `--gray-*` to Stone, add Clay + Ember, re-point semantic tokens, soften
radii, warm shadows, drop the serif body face. Restyle `button`/`.btn`, `.card`,
`.badge`, and the four partials. Update `/dev/styleguide` and keep
`handler-styleguide_test.go` green.

**Outcome:** the entire app reads as Stone, coherently, in one step.

### Increment 2 — Home & schedule

`day-cards.gohtml`, `schedule.gohtml`, `muscle-balance.gohtml`, the week chip,
`progress-bar.gohtml`, `unauthenticated.gohtml`. Clean scoped styles, fix
raw-token holdouts, rename `--gray-*` → `--stone-*` on these files.

**Outcome:** the highest-traffic landing surface fully realized.

### Increment 3 — Workout overview

`workout.gohtml` — exercise list, status badge, add-exercise affordance, the
sticky "complete workout" bar.

**Outcome:** the workout overview screen polished.

### Increment 4 — Exercise-set / Focus mode (marquee)

`exerciseset.gohtml`, `sets-container.gohtml`, `exercise-header.gohtml`,
`warmup.gohtml`. Build the dark `--stone-9` active-set panel, oversized numbers,
`--ember` CTA, restyled timer.

**Outcome:** the in-workout experience — the screen actually used during a
workout — transformed.

### Increment 5 — Secondary pages

`exercise-info.gohtml` + `progress-chart.gohtml`, `exercise-swap.gohtml`,
`exercise-add.gohtml`, `workout-completion.gohtml`, `preferences.gohtml`,
schedule editor, `privacy.gohtml`, admin pages, `error.gohtml` /
`not-found.gohtml` / `workout-not-found.gohtml` / `maintenance.gohtml`.

**Outcome:** every surface is Stone; the `--gray-*` token name is fully retired.

### Increment 6 — Motion & microinteraction pass

Button press, set-logged confirmation, card hover/active, polish on the existing
view transitions. All gated behind `prefers-reduced-motion`.

**Outcome:** the app feels alive, not just looks better.

## Success criteria

- The app no longer looks like a default open-props / bootstrapped template — a
  first-time viewer reads a deliberate "Stone" identity.
- The in-workout Focus mode makes the active set unmistakable and glanceable.
- No regressions: `make ci` green after every increment; existing e2e/DOM tests
  pass (with selector updates where structure legitimately changed).
- No new runtime dependencies, no CSP changes, no Dockerfile changes.

---

## Increment 6 — Design notes (2026-05-16)

Appendix produced during the Increment 6 brainstorm pass. Pins the design
language and concrete values that "Motion & microinteraction pass" was
otherwise leaving for the implementer to invent. Lives here rather than as a
separate spec because it is small and continuous with the rest of the Stone
work.

### Design language

Stone is **calm and confident** — warm sand, deliberate clay. Motion has to
match: **quiet, brief, confident; never bouncy.** Think "polite
acknowledgement", not "hey look at me." Springs, overshoots, and celebration
flourishes are out of character and explicitly out of scope. Subtle, not
invisible: a press at `scale(0.97)` is felt; at `0.99` it is not.

Concretely:

- **Durations** sit on a 80 / 160 / 240 / 320 ms scale. No motion crosses
  350 ms; press feedback fires under 100 ms.
- **Easing** is `cubic-bezier(0.2, 0, 0, 1)` — a quiet ease-out that
  decelerates into rest without overshoot. One token, used everywhere.
- **Reduced motion** drops every transform and collapses the view transition to
  instant. Colour swaps remain (colour is paint, not motion).

### Token additions — `ui/static/main.css` `@layer props`

To stop scattering literal `0.2s ease` strings across the 26 existing
`transition:` sites — and to give *new* rules in this increment a token system
they can reach for — add five motion tokens:

```css
/* Motion */
--ease-out-quiet: cubic-bezier(0.2, 0, 0, 1);

--duration-1: 80ms;   /* press depress */
--duration-2: 160ms;  /* hover, colour */
--duration-3: 240ms;  /* card lift, surface swap */
--duration-4: 320ms;  /* view-transition page */
```

Scope-wise: the tokens are introduced for the new rules in this increment and
for the rules we already touch. **A full sweep of the existing 26 scattered
`0.2s ease` call sites onto the new tokens is explicitly *not* in this
increment** — that would balloon scope into a token-pass-3 effort. Existing
inline values keep working; new code uses the tokens.

### Microinteraction targets

#### 1. Global button press (`button`, `.btn`)

Today, `button` in `main.css @layer components` has a `:hover` rule that swaps
`background-color` instantly, and no `:active` state at all. The single
biggest "alive" win in the app is here: every primary CTA suddenly feels
reactive.

```css
button, .btn {
    transition:
        background-color var(--duration-2) var(--ease-out-quiet),
        transform var(--duration-1) var(--ease-out-quiet);
    /* …existing rules… */

    &:active:not([aria-busy="true"]) {
        transform: scale(0.97);
    }
}
```

Why `0.97`, not `0.95` or `0.99`: `0.95` is a chunky tap (game-button), `0.99`
is invisible. `0.97` is the calmest press that still registers — measured by
sticking a button on a styleguide test page and tapping until it felt right.

Why guard `:not([aria-busy="true"])`: pressed-while-loading should not scale —
the button already has a spinner overlay and `cursor: wait`; scaling on top of
that reads as a stuck animation.

Where this *doesn't* apply: scoped buttons that already define their own
`transform` on `:hover` / `:active` (the warmup-complete button, the
Focus-mode `.submit-button`, `.signal-btn`) win by source order and keep their
existing motion grammar. The global rule applies to the ~60% of clickable
surface that has no scoped override.

Reduced-motion: drop the `transform` rule via the existing `@media
(prefers-reduced-motion: reduce)` block at the bottom of `main.css`. The
colour change still fires — that is paint, not motion.

#### 2. Card hover-lift (`a.card`, `.card` inside `<a>`)

Today `.card` is static — no `:hover`. The home day-cards, the workout
exercise-list rows, and a handful of other surfaces all wrap a `.card` in an
`<a>`. They are interactive but motionless.

Add a hover-lift, scoped to the interactive case:

```css
a.card, a > .card {
    transition:
        transform var(--duration-3) var(--ease-out-quiet),
        box-shadow var(--duration-3) var(--ease-out-quiet),
        border-color var(--duration-2) var(--ease-out-quiet);

    &:hover {
        transform: translateY(-1px);
        box-shadow: var(--shadow-2);
    }

    &:active {
        transform: translateY(0);
        box-shadow: var(--shadow-1);
    }
}
```

Why **1px** lift and not the conventional 2px: Stone is calm. 2px reads
"click me!"; 1px reads "I'm responsive." The press settles the card back to
its resting elevation — completing the gesture.

Reduced-motion: drop the `transform`s; the shadow swap still fires.

#### 3. Set-logged confirmation — the rest-chip "Ready" bloom

Considered and rejected: a per-`.exercise-set` fade-in on the freshly-completed
row. Two problems killed it. First, the page reloads via the stack-navigator
wire protocol (200 + X-Location) and the view-transition system would not
fire reliably for that flow. Second, applying `animation: fade-in` to
`.exercise-set.completed` would fire on every page render — so the initial
page load (with three already-completed sets) would have everything
fade-in at once, which reads as broken.

The earned moment is elsewhere: when the **rest timer expires**, the
`.rest-chip` flips from `--color-info-bg` (calm blue-grey) to
`--color-success-bg` (warm sage) via a class swap. Today this snaps. Add a
240 ms colour transition so the chip *blooms* into "Ready" — a brief, quiet
"your rest is done" cue. This is the moment the user is waiting for; the
animation rewards the wait.

```css
@scope (.rest-chip) {
    :scope {
        /* …existing rules… */
        transition:
            background-color var(--duration-3) var(--ease-out-quiet),
            color var(--duration-3) var(--ease-out-quiet);
    }
}
```

No reduced-motion guard needed — colour, not motion.

#### 4. View-transition polish

Today (`main.css` lines 428–465): forward navigation = old `shrink` (scale to
0.8, opacity to 0.5) + new `slide-in` from 100vw; backward mirrors. Hardcoded
`animation-duration: 0.3s` on `::view-transition-group(page)`.

Two tunings:

- The `shrink` keyframe sits at `scale: 0.8; opacity: 0.5` — the 0.5 opacity
  reads as a "ghost hovering behind the new page", and 0.8 scale is dramatic.
  Soften to `scale: 0.96; opacity: 0` — the old page recedes a fraction
  *and* cleanly fades out, instead of half-fading then snapping.
- Replace the literal `0.3s` with `var(--duration-4)` (320 ms). Same feel,
  consistent token.

```css
@keyframes shrink {
    to {
        scale: 0.96;
        opacity: 0;
    }
}

::view-transition-group(page) {
    animation-duration: var(--duration-4);
}
```

Reduced-motion: CSS view transitions are **not** automatically gated by the
browser on `prefers-reduced-motion`. Add a rule to the existing reduced-motion
block that collapses the transition to instant — the page snapshot mechanism
still runs, but no animation plays:

```css
@media (prefers-reduced-motion: reduce) {
    /* …existing rules… */
    ::view-transition-group(page),
    ::view-transition-old(page),
    ::view-transition-new(page) {
        animation-duration: 0.001ms !important;
    }
}
```

`0.001ms` rather than `0s` so the `animationend` event still fires for any
listener — defensive against listeners we may add later.

### Out of scope

- **Refactoring the 26 scattered `transition: ... 0.2s ease` sites** onto the
  new motion tokens. Pure cleanup; not motion design. A future increment can
  sweep if it becomes worth it.
- **Celebration animations** — confetti, sparkles, badge pops. Wrong tone for
  Stone. The Focus-mode Ember CTA is the only "bright accent" the design
  language allows.
- **Page-load fade-ins, scroll animations, parallax, spring physics.**
- **JS-driven motion.** Everything in this increment is CSS.
- **Touching the Focus-mode `.submit-button` / `.signal-btn` / `.on-target-btn`
  motion.** They already have a tuned motion grammar inside the dark panel
  (`translateY(-1px)` + outline shadow on active); the global button press
  rule does not reach them because their scoped rules win on source order.
- **The warmup-complete button motion.** Same reason — already tuned.
- **The `back-link` and other already-transitioned surfaces.** Already feel
  right; no value churning them onto tokens in this increment.
- **The loading-bar shimmer.** Already exists, already reduced-motion-aware.
- **A new pattern in `ui/templates/CLAUDE.md` for motion tokens.** Defer until
  scoped blocks actually reach for the tokens — likely zero in this increment.
  The `main.css` rule is the documentation. (If a second site needs the same
  press scale in this increment, promote it then.)

### Implementation files (preview for the plan)

| File | Change |
|---|---|
| `ui/static/main.css` | Add five motion tokens to `@layer props`; add the global button-press rule to `button, .btn`; add the `a.card` / `a > .card` hover-lift rule; soften the `shrink` keyframe and swap the `0.3s` literal to `var(--duration-4)`; extend the `@media (prefers-reduced-motion: reduce)` block to drop the button-press transform, the card-lift transform, and collapse the view transition to instant. |
| `ui/templates/pages/exerciseset/sets-container.gohtml` | Add the `transition: background-color / color` rule to `.rest-chip :scope` so the `.ready` swap blooms. |

Two files. The plan will break each into reviewable sub-steps with grep gates
and per-touch render tests, mirroring the Increment 5 shape.

### Visual-check anchors

The user cannot drive a browser in this environment; per-task visual checks
are described, not performed. The anchors a reviewer should hit manually:

- `/dev/styleguide` — hover and press every button variant; confirm scale
  feels acknowledging-not-bouncy.
- `/` — hover/press a day-card; confirm the 1px lift is felt.
- `/workouts/<today>` — hover an exercise-list row card.
- Mid-workout — wait through a rest period; confirm the chip blooms from
  blue-grey to sage.
- Navigate forward/back between pages; confirm the shrink feels lighter than
  before.
- Toggle OS-level reduced-motion; confirm presses and lifts collapse to
  colour/shadow swaps and view transitions become instant.
