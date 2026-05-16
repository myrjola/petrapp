# Button System Unification

**Date:** 2026-05-16
**Status:** Approved direction — ready for implementation planning

## Goal

Make the app's buttons consistently styled and accessible. Today there is one
`.btn` rule in `main.css` plus ~15 near-duplicate bespoke button classes
sprinkled across page templates. The preferences page is the worst offender
visually and has a real contrast bug. This work centralises every button into a
small semantic-variant system and retrofits every existing call site.

### Constraints (unchanged)

- No build step — templates load from the filesystem at runtime.
- Strict CSP with nonces + Trusted Types.
- Existing `@layer reset, props, layout, components` cascade is kept.
- Stone/Clay palette, existing design tokens.
- `/dev/styleguide` is the living component catalog (and asserted in
  `cmd/web/handler-styleguide_test.go`).

### Out of scope

- The `.signal-btn` segmented control in the working-set form (it's a custom
  segmented choice, not a button).
- The `.timed-runner-cancel` micro-link inside the timed-runner card (lives
  inside a coloured card; styled to match the card chrome).
- Any new tokens — design tokens stay as-is.
- Any change to button JS behaviour (the `aria-busy` loading state from
  `main.js` keeps working unchanged).

## Diagnosis

### Visual drift

| Spot | Current class | Notes |
|---|---|---|
| Preferences — Save Schedule | `.save-button` | clay-4, `--radius-2`, weight-6, full-width |
| Preferences — Save deload settings | bare `<button>` (base `.btn`) | clay-4, `--radius-3`, weight-7 — *different shape from Save Schedule right above it* |
| Preferences — Restart cycle | bare `<button>`, has `disabled` attr | identical appearance whether enabled or not |
| Preferences — Download My Data | `.export-button` (an `<a>`) | clay-4, smaller padding & weight |
| Preferences — Log out | `.logout-button` | transparent + stone border — only used here |
| Preferences — Delete my data | `.delete-button` | error-red |
| Preferences — Enable rest notifications | `.push-button` | **`background: var(--color-surface)` = `--stone-1` = same as the page** — button blends into the page |
| Preferences — Disable on this device | `.push-button.danger` | error text on near-white + 30 % opacity error border |
| Workout set — Done! / Start timer | `.submit-button` | ember, uppercase, weight-8 |
| Workout warmup — Complete warmup | `.warmup-complete-button` | `--color-info` blue |
| Exercise swap — Swap to this | `.swap-button` | `--color-success-bg` pale-green pill |
| Exercise add — Add this exercise | `.add-button` | identical to `.swap-button` |
| Workout — Add an exercise (link) | `.add-exercise-button` | clay-1 chip |
| Home — preferences cog | `.menu-button a` | stone-0 + border + shadow chip |
| Workout edit chip | `.edit-button` | tiny stone-0 chip |
| not-found / error / workout-not-found | `.btn .home-link`, `.btn .back-link`, `.primary-button` | extend `.btn` then recolour |

### Accessibility issues

1. **Contrast — preferences rest-notifications card.** `.push-button` background
   equals the page background, the button is nearly invisible. The danger
   variant is error-coloured text on near-white. Visible in the user-supplied
   screenshot.
2. **No `:disabled` state.** Base `button`/`.btn` has no rule for disabled
   buttons. `Restart cycle from next Monday` (which renders with the `disabled`
   attribute when planned deloads are off) looks identical to its enabled state.
3. **Touch targets.** `.delete-button`, `.export-button`, `.logout-button`,
   `.push-button` resolve to ≈ 36–40 px tall — below WCAG 2.5.5 (44 × 44).
4. **Focus-colour drift.** The `select` inside the preferences schedule form
   focuses *green* (`--color-success`), buttons focus *clay-3*. Two visual
   languages on the same form.
5. **Radius drift.** Base `.btn` uses `--radius-3` (1 rem) but most call sites
   override to `--radius-2` (5 px). Whichever class wins on a given page
   determines the shape — inconsistent across pages.

## Design

### Variant set (added to `main.css` `@layer components`)

Five intent variants, two modifiers. Variants are mutually exclusive; modifiers
compose with any variant.

| Class | Used for | Replaces |
|---|---|---|
| `.btn` (default = primary) | Page CTA — Save, Submit, Generate, Sign in, Download | `.save-button`, `.export-button`, `.primary-button`, plus bare `<button>` callers that already inherit `.btn` |
| `.btn--quiet` | List-item action — "Add this exercise", "Swap to this" | `.add-button`, `.swap-button`, `.add-exercise-button` |
| `.btn--ghost` | Secondary / reversible — "Log out", "Enable rest notifications", header chip | `.logout-button`, `.push-button` (enable), `.menu-button a`, `.edit-button` |
| `.btn--danger` | Destructive — "Delete my data", "Disable on this device" | `.delete-button`, `.push-button.danger` |
| `.btn--focus` | The single in-set CTA in workout mode | `.submit-button` (ember), `.warmup-complete-button` (currently blue → unified to ember per user decision) |
| `.btn--block` *(modifier)* | `width: 100%` | full-width form CTAs |
| `.btn--sm` *(modifier)* | small chip, ≈ 32 px tall | inline edit / footer "log out" / disable-on-device |

### Base `.btn` changes (in `main.css`)

```css
button, .btn {
    /* existing rules… plus: */
    min-height: 2.75rem;              /* WCAG 2.5.5 — 44 px touch target */
    border-radius: var(--radius-2);   /* was --radius-3; matches every override */
}

button:disabled, .btn:disabled, .btn[aria-disabled="true"] {
    opacity: 0.55;
    cursor: not-allowed;
}
button:disabled:hover, .btn:disabled:hover {
    background-color: var(--clay-4);   /* cancel hover for disabled */
}
```

The existing `aria-busy="true"` loading state, focus-visible outline, and active
press-transform are unchanged.

### Variant rules

```css
.btn--quiet {
    background: var(--color-success-bg);
    color: var(--color-success);
}
.btn--quiet:hover { background: color-mix(in oklab, var(--color-success-bg) 92%, black); }

.btn--ghost {
    background: var(--color-surface-elevated);   /* stone-0 — pops off the page */
    color: var(--color-text-primary);
    border: var(--border-size-1) solid var(--color-border);
}
.btn--ghost:hover {
    background: var(--stone-1);
    border-color: var(--stone-4);
}

.btn--danger {
    background: var(--color-error);
    color: var(--color-surface-elevated);
}
.btn--danger:hover { background: color-mix(in oklab, var(--color-error) 88%, black); }
.btn--danger:active { background: color-mix(in oklab, var(--color-error) 78%, black); }

.btn--focus {
    background: var(--ember);
    color: var(--stone-10);
    font-weight: var(--font-weight-8);
    text-transform: uppercase;
    letter-spacing: var(--font-letterspacing-3);
}
.btn--focus:hover { background: color-mix(in oklab, var(--ember) 90%, white); }
.btn--focus:active { background: color-mix(in oklab, var(--ember) 85%, black); }
.btn--focus:focus-visible { outline: 3px solid var(--ember); }
```

### Modifier rules

```css
.btn--block { width: 100%; }

.btn--sm {
    min-height: 2rem;                 /* 32 px — accepted exception to 44 px for tertiary chips */
    padding: var(--size-1) var(--size-3);
    font-size: var(--font-size-0);
    font-weight: var(--font-weight-5);
    letter-spacing: var(--font-letterspacing-1);
    text-transform: none;
}
```

`--sm` deliberately keeps the variant's colour but normalises size + weight, so
`.btn .btn--ghost .btn--sm` (the log-out / disable / edit chip) and
`.btn .btn--quiet .btn--sm` look right.

### Migration

Pages get their bespoke `.{x}-button` classes deleted from their colocated
`<style>` block; the markup switches to `.btn .btn--…`. Layout-only rules
(margin, alignment, sizing relative to a parent) stay in the page block where
they belong.

| File | Before | After |
|---|---|---|
| `preferences/preferences.gohtml` | `.save-button` | `.btn .btn--block` |
| | `.export-button` | `.btn` (still an `<a>` — `.btn` already handles anchor) |
| | `.logout-button` | `.btn .btn--ghost .btn--sm` |
| | `.delete-button` | `.btn .btn--danger` |
| | `.push-button` (enable) | `.btn .btn--ghost` |
| | `.push-button.danger` (disable) | `.btn .btn--danger .btn--sm` |
| | bare `<button>` Save deload | `.btn .btn--block` |
| | bare `<button>` Restart cycle | `.btn .btn--ghost` |
| | inline `select { outline: 2px solid var(--color-success); }` | use base `--color-border-focus` (clay-3) — focus colour aligns with buttons |
| `exerciseset/sets-container.gohtml` | `.submit-button` | `.btn .btn--focus .btn--block` |
| | `.edit-button` | `.btn .btn--ghost .btn--sm` |
| `exerciseset/warmup.gohtml` | `.warmup-complete-button` | `.btn .btn--focus .btn--block` (ember per user decision) |
| `exercise-add/exercise-add.gohtml` | `.add-button` | `.btn .btn--quiet .btn--block` |
| `exercise-swap/exercise-swap.gohtml` | `.swap-button` | `.btn .btn--quiet .btn--block` |
| `workout/workout.gohtml` | `.add-exercise-button` | `.btn .btn--quiet` |
| `home/schedule.gohtml` | `.menu-button a` | `.btn .btn--ghost .btn--sm` (the cog stays a header chip via layout-only rules) |
| `home/day-cards.gohtml` | `.btn` (anchor) | unchanged — already on the system |
| `not-found/not-found.gohtml` | `.btn .home-link`, `.btn .back-link` | `.btn` and `.btn .btn--ghost` |
| `error/error.gohtml` | `.btn .home-link` | `.btn` |
| `workout-not-found/workout-not-found.gohtml` | `.primary-button` | `.btn` |
| `schedule/schedule.gohtml` | `.save-button` | `.btn .btn--block` |

Bare `<button type="submit">` callers that already get the base `.btn` look —
`admin-exercise-edit`, `admin-exercises`, `admin-feature-flags`,
`workout-completion`, `unauthenticated`, `exercise-add` (Search),
`exercise-swap` (Search), `workout` (Complete workout), `exercise-add`
(dialog Close), `exercise-swap` (dialog Close) — keep working unchanged because
the base rule still selects them. Explicit `.btn .btn--block` is added where
they're intended as the page CTA.

### Styleguide additions (`/dev/styleguide`)

A new "Button variants" section in `pages/styleguide/styleguide.gohtml`
showing:

- One row per variant (primary, quiet, ghost, danger, focus).
- For each variant: idle / hover-visible / focus-visible-equivalent /
  `:disabled` / `aria-busy="true"` / `.btn--sm` / `.btn--block`.
- An anchor (`<a class="btn">`) alongside a `<button class="btn">` to show the
  shared rendering.

`handler-styleguide_test.go` already asserts the page renders 200 + has a
"Buttons" heading; extend the assertions to check the new variant headings exist
(see Validation below).

### Accessibility wins delivered

- Every button ≥ 44 px tall (modifier `.btn--sm` is the explicit, documented
  exception for tertiary chips at 32 px — sized small but still well above the
  default `:active` target the user is pointing at).
- Every button has a visible `:disabled` style.
- The "Enable rest notifications" / "Disable on this device" buttons render on
  surface-elevated background — they pop off the page.
- Focus ring colour is the same (`--color-border-focus` = clay-3) on buttons
  *and* on the schedule `select`.
- Danger semantics are visual: red fill, not low-contrast red text.

## Validation

- `make ci` — full pipeline (build, lint-fix, test, sec) must pass.
- `cmd/web/handler-styleguide_test.go` extended to assert each new variant
  heading renders.
- Manual smoke pass in browser:
  - `/preferences` — every button distinguishable from the page background, the
    danger zone reads as destructive, `Restart cycle` looks disabled when
    deloads are off.
  - `/workouts/<today>` — `Complete workout` and `Add an exercise` look right.
  - `/workouts/<today>/exercises/<id>` — in-set `Done!` and `Start timer` retain
    the ember focus-mode look.
  - `/dev/styleguide` — every variant row visible and well-distinguished.
- Keyboard: tab through preferences, every focused control has the same clay-3
  outline.

## Implementation order

1. **CSS first.** Add variants + modifiers + `:disabled` rule + base `min-height`
   and `border-radius` change to `main.css`. Extend `/dev/styleguide` to show
   the new variants and extend `handler-styleguide_test.go` to assert their
   headings. Verify the styleguide page visually.
2. **Preferences page.** This is the worst-looking surface and the one the user
   pointed at. Convert all six bespoke classes and remove their CSS. Verify the
   page visually.
3. **Workout-flow pages.** `sets-container`, `warmup`, `exercise-add`,
   `exercise-swap`, `workout` — these touch the daily user path; verify each in
   the browser before moving on.
4. **Remaining surfaces.** Home, schedule, not-found, error, workout-not-found.
5. **Final sweep.** Grep for `-button` class names that should be gone; remove
   any leftover dead CSS.

Each step is independently shippable.
