# Frontend Foundation Design

**Date:** 2026-05-14
**Status:** Approved — ready for implementation planning

## Goal

Establish a more maintainable frontend foundation: a settled component
vocabulary, a forms/validation convention, and template-logic discipline. This
is structure and conventions work — aesthetic polish comes later via the
frontend-design plugin and depends on this foundation existing.

### Constraints (unchanged by this work)

- No-build frontend — templates and static assets load from the filesystem at
  runtime; editing a template and refreshing is the whole dev loop.
- Mobile-only, very constrained user flows (filling in gym workouts).
- Strict CSP with nonces + `require-trusted-types-for 'script'`.
- Global JS features in `ui/static/main.js` — the stack navigator (intercepts
  form POSTs, replays as fetch, drives History) and navigation feedback
  (loading bar + button text swap) — must keep working untouched.
- Existing design-token system (open-props subset) and `@layer reset, props,
  layout, components` cascade are kept.

### Out of scope

- App shell / landmarks (`base.gohtml` skeleton, nav, skip-link). The app is
  mobile-only with constrained flows; this is not where the maintainability
  pain is.
- Visual / aesthetic design — deferred to the frontend-design plugin.
- Per-field server-side validation errors and submitted-value preservation.
- Any change to the stack navigator or the flash mechanism.

## Core principle — guarantee-based split

A component's delivery mechanism is decided by what it must *guarantee*:

| Delivery | For | Why |
|---|---|---|
| **Go partial** in `ui/templates/components/*.gohtml`, colocated `@scope` `<style>` | Pieces that enforce accessibility/structure: `field`, `banner`, `page-header`, `back-link` | Caller can't forget `<label for>` / `aria-describedby`; leaf-ish, so no children-slot problem; appears 1–few times per page, so no `<style>` duplication cost |
| **CSS class** in `main.css @layer components` | Pure paint on a semantic element: `button`/`.btn`, `badge`, `card` | Composes freely (a card holds anything); zero per-render cost; can appear many times per page |
| **Inline scoped `<style>`** | Genuinely page-specific composition | Documented escape hatch — unchanged from today |
| **Layout primitive class** in `@layer layout` | `.stack`, `.cluster`, `.grid-auto`, `.center` | Kills the single biggest source of inline-`<style>` duplication |

### Why not the alternatives

- **Classless / Pure-CSS-style:** breaks down at the first variant, doesn't
  touch the app's hard components (sets-container, progress-chart,
  muscle-balance), and pulling in a framework fights the existing open-props
  tokens and `@layer` cascade.
- **Partials for everything (incl. card/list):** Go templates have no clean
  children-slot mechanism; wrapper components degenerate into
  `card-open`/`card-close` pairs or many params.
- **CSS classes for everything (incl. form fields):** loses the accessibility
  enforcement that a `field` partial gives — `<label for>` binding and
  `aria-describedby` wiring become author discipline every time.
- **Full utility-class set (Tailwind-like):** class soup; a large hand-rolled
  set really wants a JIT build, which conflicts with the no-build constraint.

## CSS structure

The existing `@layer reset, props, layout, components` cascade is kept.
`reset` and `props` are untouched. This work *adds* to:

- `@layer layout` — the 4 layout primitives.
- `@layer components` — `.badge`, `.card` (`button`/`.btn` already there).

`main.css` grows modestly. Nothing that should stay page-scoped is moved out of
templates.

## Component inventory (settled)

### Partials — `ui/templates/components/`

Each is one file with a colocated `<style {{ nonce }}>` `@scope` block, per the
existing `back-link` pattern.

| Component | Dot (input) | Guarantee / behaviour |
|---|---|---|
| `banner` | `{ Variant, Message }` — `Variant` ∈ `error` / `success` / `info`; renders nothing when `Message` is empty | `role="alert"` for `error`, `role="status"` for `success`/`info`. Replaces the ad-hoc `<p class="error-message">` rendering |
| `page-header` | `{ Title, Subtitle }` — `Subtitle` optional | Emits exactly one `<h1>` with consistent heading style. Pages render any meta/badges as siblings *after* the header (avoids the children-slot problem) |
| `field` | `{ Label, Name, Type, Value, Required, Hint, Min, Max, Step, Pattern }` | Binds `<label for>` ↔ input `id`, wires `aria-describedby` → hint, passes native-validation attributes through to the `<input>`. Covers the common single-`<input>` case only |

`back-link` — exists, unchanged.

**`field` limitation (documented):** `field` covers text-ish `<input>` fields.
`<select>`, `<textarea>`, and checkbox/radio groups do not fit a single dot
shape and stay as inline markup for now. If real duplication emerges later,
a sibling partial can be added — not part of this work.

### Class-components — `main.css @layer components`

| Class | Markup | Notes |
|---|---|---|
| `.btn` / `button` | exists | Documented in the styleguide. Variants left to emerge (YAGNI) |
| `.badge` + `.badge--success` / `--warning` / `--neutral` / `--info` | `<span class="badge badge--success">` | Workout statuses map onto semantic variants: completed → `success`, in-progress → `warning`, not-started → `neutral` |
| `.card` | `.card` on `<article>` / `<a>` / `<li>` | Surface + radius + shadow + padding. Composes freely with any children |

### Layout primitives — `main.css @layer layout`

| Class | Does |
|---|---|
| `.stack` | Vertical flex, default `gap: var(--size-4)` |
| `.cluster` | Horizontal wrap flex, default `gap: var(--size-2)` |
| `.grid-auto` | `auto-fill` responsive grid |
| `.center` | Readable-column: `max-width` + `margin-inline: auto` |

Each primitive has one sensible default gap/width. A different value is treated
as page-specific → an inline scoped `<style>` override on that element (the
escape hatch). This keeps the primitive set tiny rather than breeding
`.stack-tight` / `.stack-loose` variants.

The **styleguide page** (`ui/templates/pages/styleguide/`) becomes the living
catalog of every item above.

## Forms & validation

Three layers. No change to the stack-navigator wire protocol or the flash
mechanism.

1. **Client UX → native HTML validation.** The `field` partial passes
   `required`, `type`, `min` / `max` / `step`, `pattern` through to the
   `<input>`. The browser handles inline field-level UX. Accepted tradeoff:
   native validation's accessibility is imperfect; this bets on platform
   evolution rather than building a custom per-field error system now.
2. **Server validation → domain/service returns typed errors.** Validation
   rules move out of handlers into domain/service methods, surfaced as a typed
   `ValidationError` (following the project's "Error" suffix convention).
   Reusable and testable. `handler-admin-exercises.go`'s inline checks are the
   migration target (during that page's own later migration).
3. **Server error display → `banner`.** The handler catches the typed error,
   calls `putFlashError` with a user-facing message, and redirects to the form
   — the existing pattern the stack navigator already handles. The form's GET
   handler pops the flash and renders it through the `banner` component.

No structured payload, no value preservation, no per-field server errors —
a single banner channel. This largely *formalizes* what
`handler-admin-exercises.go` and `handler-schedule.go` already do, plus moves
the rules into the domain layer and routes display through `banner` instead of
ad-hoc markup.

## Template-logic discipline

Principle (already stated in `CLAUDE.md`, currently violated): derived values
come from the handler (formatting / shaping) or the domain (multi-field rules)
— never computed in templates.

- `workout.gohtml` is the known offender: it computes the workout-type string
  and per-set completion state in-template. Workout-type → a domain method
  (it is a multi-field domain rule). Per-set completion state →
  handler-prepared data or a domain method.
- This becomes a per-page migration checklist item. `workout.gohtml` is fixed
  now, as part of the reference-page work below.

## Rollout

### Reference page: `workout.gohtml`

`workout.gohtml` is migrated fully as the worked exemplar. It exercises most of
the foundation: the template-logic offense (workout-type, set state), status
badges, cards (exercise-list items), layout duplication (`flex` / `gap`
repeated inline), and a form (complete workout) with the `banner` slot wired
for flash errors.

The typed `ValidationError` infrastructure is established and unit-tested as
part of this work even though the reference page's own form has little to
validate — it is foundational, and the first form-heavy page to migrate
(`admin-exercises`) will consume it without further design work.

### Incremental migration

The remaining ~27 page templates are **not** converted in this effort. They
migrate opportunistically as they are touched. The deliverable is the
foundation plus the conventions that govern it — not a fully converted
codebase.

## Deliverables

1. `main.css` — 4 layout primitives in `@layer layout`; `.badge` and `.card`
   in `@layer components`.
2. `ui/templates/components/banner.gohtml`, `page-header.gohtml`,
   `field.gohtml` — partials with colocated scoped styles.
3. Domain/service typed `ValidationError` infrastructure, established and
   unit-tested. (Moving specific validation rules — e.g. `admin-exercises` —
   into the domain layer follows later, during each page's own migration.)
4. `workout.gohtml` migrated; its derived-state logic moved to domain/handler.
5. Styleguide page updated to catalog every component and layout primitive.
6. Conventions updated:
   - `ui/templates/CLAUDE.md` — the guarantee-based split rule, the component
     inventory, and "when to use what".
   - `cmd/web/CLAUDE.md` — the validation-error flow (typed domain error →
     `putFlashError` → redirect → `banner`).

## Testing

- Existing Playwright and e2e tests covering `workout.gohtml` must stay green.
  Selectors are updated to be resilient where the DOM changes, per the
  `CLAUDE.md` test-compatibility guidance.
- The styleguide page rendering exercises every component, giving them smoke
  coverage.
- Domain `ValidationError` paths get unit tests.
