# Migrate `home` and `exerciseset` onto the Frontend Foundation

**Date:** 2026-05-14
**Status:** Approved — ready for implementation planning

## Goal

Bring the last two unmigrated page templates — `home` and `exerciseset` — onto
the frontend foundation established in
[`2026-05-14-frontend-foundation-design.md`](2026-05-14-frontend-foundation-design.md).
These two were deliberately deferred: both are partial-heavy, JS-heavy, and
visualization-heavy, and the mechanical migration checklist only superficially
touches them.

"Migrate" here means: adopt the foundation primitives (`page-header`,
`back-link`, `.card`, `.badge`, `.stack`/`.cluster`/`.grid-auto`/`.center`)
*where they genuinely fit*, fix any template-logic-discipline offenses, and
leave the genuinely page-specific visualizations and interactive components as
page-scoped markup. It is structure and conventions work — no aesthetic
redesign.

### Constraints (unchanged)

- No-build frontend; templates load from the filesystem at runtime.
- Strict CSP with nonces + `require-trusted-types-for 'script'` — every
  `<style>`/`<script>` keeps `{{ nonce }}`; dynamic CSS values stay inside
  nonced `<style>` blocks, never inline `style=""` attributes.
- The global stack navigator and navigation feedback in `main.js` stay
  untouched.
- Existing Playwright (`cmd/web/playwright_test.go`) and e2e handler tests must
  stay green. Selectors get updated only where the DOM genuinely changes, and
  toward more resilient selectors.

### Out of scope

- Aesthetic / visual redesign of either page.
- App shell / landmarks.
- Any new shared component or new `.badge` variant (see "Decisions" below).
- Refactoring the `sets-container` form-branching logic beyond the one
  template-logic fix called out below.

## Page composition

**`home`** — `home.gohtml` routes on `Authenticated`:
- `unauthenticated.gohtml` — landing hero (logo, h1 "Petra", tagline, Sign
  in/Register forms, footer) + decorative `backdrops.gohtml`.
- `schedule.gohtml` — authenticated view: a `Menu` link header, an optional
  deload week-chip, the `.weekly-schedule` list (`day-cards.gohtml` →
  `progress-bar.gohtml` per day), and `muscle-balance.gohtml`.

**`exerciseset`** — `exerciseset.gohtml` is the page shell (a `<main>`, a fixed
timer, two page-specific `<script>` blocks) wrapping `exercise-header.gohtml`,
`warmup.gohtml`, and `sets-container.gohtml`.

## What maps to foundation primitives

| Site | Change |
|---|---|
| `schedule.gohtml` authenticated `<main>` | Add a `page-header` ("This Week") — the authenticated home currently has **no `<h1>`**; this fixes the heading hierarchy (it otherwise jumps straight to the `muscle-balance` h2). `.stack` for the main column; `.weekly-schedule` keeps its class, gains `.stack` (gap override stays scoped). |
| `day-cards.gohtml` `.day-card` | Add `.card` for the base surface (border/radius/padding/shadow/background); the per-`data-status` border-colour/background overrides stay in the scoped `<style>`. `.day-card` keeps its class and `data-status` attribute. `.workout-actions` → `.cluster`. Drop the dead `btn-primary`/`btn-secondary` classes (undefined in `main.css`; the `<button>` element is already styled). |
| `muscle-balance.gohtml` `section.muscle-balance` | Add `.card` for the surface; the inner `display:flex;flex-direction:column` column → `.stack`, `.region` → `.stack` (gap override scoped). Everything else — the bars, legend, rows, per-`data-status` colours, and the dynamic per-`data-slug` widths emitted from the nonced `<style>` — stays page-specific. |
| `exercise-header.gohtml` back link | Replace the hand-rolled `<a data-back-button class="back-link">…</a>` and its duplicated scoped `.back-link` CSS with the existing `back-link` component: `{{ template "back-link" (printf "/workouts/%s" (.Date.Format "2006-01-02")) }}`. The component renders the same `class="back-link"` + `data-back-button` + `← Back` structure. Its `<style>` sibling is `display:none` so it does not become a grid item. `.header-actions` → `.cluster`. |
| `exerciseset.gohtml` `<main>` | `.stack` for the column; the page-specific `max-width: 600px` + margin + responsive padding stay in the scoped `<style>` (600px ≠ `.center`'s 32rem). |
| `sets-container.gohtml` `.exercise-set` | Add `.card` for the base surface; the `.completed`/`.active` variant overrides stay scoped. `.exercise-set` keeps its class. |
| `unauthenticated.gohtml` | Light touch: the button row → `.cluster` (gap override scoped); the repeated `display:flex;flex-direction:column` content wrappers → `.stack` where the gap matches or an override is acceptable. The hero h1/logo/tagline markup, the footer, and `backdrops.gohtml` stay page-specific — this is a full-viewport hero, same treatment as the `not-found`/`error`/`maintenance` heroes in commit 567f424. |

## Template-logic discipline fix

`sets-container.gohtml` computes per-set active state in-template:

```
{{ $isActive := and $.ExerciseSet.WarmupCompletedAt
    (or (and (not $set.CompletedValue) (eq $.FirstIncompleteIndex $index))
        (and $.IsEditing (eq $index $.EditingIndex))) }}
```

This is a multi-field derived value — exactly the offense the foundation spec
calls out (and that `workout.gohtml` fixed for its per-set state). Move it to
the handler: add an `IsActive bool` field to the `setDisplay` struct in
`cmd/web/handler-exerciseset.go` and compute it in `prepareSetsDisplay` (the
handler already has every input: `WarmupCompletedAt`, `CompletedValue`,
`FirstIncompleteIndex`, `IsEditing`, `EditingIndex`). The template then reads
`$setDisplay.IsActive` directly.

`prepareSetsDisplay` currently takes `(exercise, sets)`; it will need the extra
inputs threaded through (or the active-state computation can live in
`exerciseSetGET` after `prepareSetsDisplay` returns, then be assigned onto each
`setDisplay`). The implementation plan picks the cleaner of the two.

No other in-template logic on either page rises to an offense: `home`'s
handler already does all data shaping (`toDays`, `toMuscleBalance`, status
computation); the remaining `{{ if eq .Status … }}` conditionals in
`day-cards`/`progress-bar` are display-only branching, which is allowed.

## Decisions (things deliberately *not* done)

- **No new shared component.** `progress-bar` is used on one page; the landing
  hero is one-off. Neither clears the "three nearly-identical sites" bar.
- **No new `.badge` variant.** `day-cards`' `.status-indicator` needs six
  status colours (including a red "past-incomplete" and a dashed
  "unscheduled") against `.badge`'s deliberately-fixed four
  (success/warning/neutral/info). Mapping it onto `.badge` loses fidelity, and
  the status colouring is tightly coupled to the `.day-card`'s own per-status
  border/background. `.status-indicator` stays page-specific. Extending
  `.badge` is a foundation change that deserves its own design.
- **`.warmup-banner` and `.deload-banner` are not the `banner` component.**
  The `banner` component is for server flash messages (`BannerData{Variant,
  Message}` popped from a flash). `.warmup-banner` is a rich call-to-action
  (heading + description + form); `.deload-banner` is a persistent contextual
  notice with a distinct left-border callout style. Both stay page-specific;
  the class-name overlap is incidental.
- **`field` component is not used in `sets-container`.** Its set-form inputs
  carry `inputmode`, `pattern`, JS-target classes (`reps-input`), custom ids
  distinct from `name`, and `.sr-only` hint divs — well outside `field`'s
  documented "common single-`<input>` case". They stay as inline markup.
- **No styleguide or `CLAUDE.md` change.** No new components or conventions are
  introduced; the foundation docs already cover everything used here.

## Test impact

Selectors the tests depend on — all **preserved**:

- `home`: `section.muscle-balance`, `.weekly-schedule`, `.muscle-balance`,
  `.row`, `.row[data-slug="…"]`, `.row[data-status="…"]`, `.target-mark`, and
  the `main`-direct-child ordering of `.weekly-schedule` before
  `.muscle-balance` (`handler-home_test.go`); `button:contains('Sign in')` /
  `button:contains('Register')` and the Playwright `GetByRole("button", Name:
  "Register"/"Sign in")` (`handler-home_test.go`, `playwright_test.go`).
- `exerciseset`: `a.exercise` (on the workout page, unaffected), `h1`,
  `.exercise-set`, `.exercise-set.completed`, `.exercise-set.active`,
  `.weight`, `.reps`, `.edit-button`, `.rest-chip[data-rest-end-at-ms]`,
  `.deload-banner`, `.warmup-status`, `button[name='signal']`,
  `button.signal-btn`, `button:contains('Mark Warmup Complete')`,
  `button:contains('Done!')`, `input[name='weight']`
  (`handler-exerciseset_test.go`, `playwright_test.go`).

Adding `.card` to `.day-card` / `.exercise-set` / `section.muscle-balance` is
additive — the existing class stays, so every selector above keeps matching.
Adding a `page-header` to authenticated `home` adds a `<main>` child but the
ordering test compares the *relative* positions of `.weekly-schedule` and
`.muscle-balance`, which is unaffected. The `back-link` component swap in
`exercise-header` preserves `class="back-link"` and `data-back-button`.

No test selector change is anticipated. If the implementation forces one, it
must move *toward* a more resilient selector, per `CLAUDE.md`.

## Deliverables

1. `home` migrated: `schedule.gohtml` (`page-header`, `.stack`),
   `day-cards.gohtml` (`.card`, `.cluster`, dead classes dropped),
   `progress-bar.gohtml` (layout primitives where they fit),
   `muscle-balance.gohtml` (`.card`, `.stack`), `unauthenticated.gohtml`
   (`.stack`/`.cluster`, hero kept). `handler-home.go` gains a `Header
   PageHeaderData` field.
2. `exerciseset` migrated: `exercise-header.gohtml` (`back-link` component,
   `.cluster`), `exerciseset.gohtml` (`.stack`), `warmup.gohtml` (light
   touch), `sets-container.gohtml` (`.card` on `.exercise-set`).
3. `sets-container`'s `$isActive` moved to `setDisplay.IsActive` in
   `cmd/web/handler-exerciseset.go`.
4. `make ci` green; Playwright and e2e handler tests for both pages passing.

## Testing

- `make ci` (init, build, lint, test, sec).
- `go test ./cmd/web/ -run 'Test_application_home|Test_application_exerciseSet|TestExerciseSetGET_DeloadHidesSignalButtons|Test_ExerciseSet_RestChipAfterCompletedSet'`
  for the focused page coverage.
- Playwright `playwright_test.go` exercises both pages end-to-end.
