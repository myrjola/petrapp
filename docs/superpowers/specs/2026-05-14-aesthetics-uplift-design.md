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
