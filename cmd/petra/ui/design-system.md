# PetrApp Design System

The visual identity, design-token catalogue, and colour / motion / active-state
conventions for the UI. Companion to
[`templates/README.md`](templates/README.md) — read this before any
visual or CSS work.

File paths below are relative to this directory (`cmd/petra/ui/`).

## Design Language

The app's visual identity is an *editorial training logbook*, not a fitness
SaaS. Closest references: field notebooks, atelier signage, small-press
editorial design. Restrained, warm, intentional. Density is low — every
element earns its space.

### Identity rules

- **Warm-only palette.** Stone neutrals + Clay terracotta accent + Ember
  one-off pop. No cool grays anywhere; shadows are warm-tinted
  (`rgb(120 90 60 / .10)`), never neutral black. The semantic-token
  catalogue is documented under "Color System" below. **Clay-5 is the
  AA-safe small-text accent** (overlines, links, the primary button
  surface, ~5.7–6.1:1 on stone); **Clay-4 is reserved for large display
  (≥18.66px/700) or non-text fills** (progress bars, accent dots, the
  select chevron) — it measures ~4.0–4.3:1 and fails AA for small text.
- **Three type voices, system stacks only — zero web fonts.** Display =
  serif (Charter → Iowan Old Style → Georgia → ui-serif), tight tracking
  `-0.025em`, line-height `0.95`, weight 6. Body = `system-ui`. Mono =
  the developer-font waterfall in `--font-monospace-code`, and it does
  real work: overlines, dates, counters, status labels are all mono. Mono
  is the "annotation" voice and a signature of the system.
- **No `@font-face` rules. No font CDN requests.** Cross-platform
  rendering variance is intentional — the system aims for *native craft*
  per OS, not pixel-identical rendering. Don't propose a Google-fonted
  look or introduce a custom face.
- **Signature page-header motif.** Uppercase mono overline in
  `--clay-5` → serif display title (`clamp(2.25rem, 9vw, 3rem)`,
  line-height `0.95`, tracking `-0.025em`) → meta row. The overline ends
  with a 1px hairline fading right via
  `linear-gradient(to right, var(--stone-3), transparent)`. Worked
  example in `templates/pages/workout/workout.gohtml` and
  `templates/pages/exerciseset/exercise-header.gohtml`. Reuse this
  rhythm; don't reinvent it.
- **State is never colour-only.** Every state carries a non-colour
  signal too: in-progress = amber wash *plus* a 3px left-edge tab;
  completed = sage wash *plus* a drawn strike-through; default =
  elevated paper. Required by WCAG SC 1.4.1 and by the visual identity.
- **Motion: one curve, sparingly.** `cubic-bezier(0.2, 0, 0, 1)`
  (`--ease-out-quiet`), `80–320ms`. The View Transitions API is
  first-class — title morphs list ↔ detail, strike-through draws on
  completion. No springs, no bounces, no scattered micro-interactions.

### Anti-patterns

Do not introduce: Inter / Space Grotesk / generic geometric sans; web
fonts via Google or any CDN; purple gradients or neon accents;
dashboard-style multi-column layouts or sidebars; card-on-card density;
saturated status colours (use the muted sage / amber / terracotta
washes); neutral-black drop shadows; springy or bouncy motion.

### The one thing to keep

Serif title + mono overline + fading hairline on a paper-warm surface,
with terracotta as the only saturated voice. That's the brand.

## Design System Usage

### CSS Custom Properties (Design Tokens)

Always verify these exist in `static/main.css` before using:

#### Spacing System

- `--size-1` through `--size-15` (0.25rem to 30rem)
- Use for margins, padding, gaps: `margin: var(--size-4)`

#### Color System

The app is on the warm "Stone" palette. **Reach for semantic tokens first**, then
the Stone/Clay ramps; the raw Tailwind-style ramps are legacy holdouts being
retired page-by-page.

- **Semantic colors** (prefer these): `--color-surface`, `--color-surface-elevated`,
  `--color-surface-active`, `--color-surface-completed`, `--color-border`,
  `--color-border-focus`, `--color-text-primary` / `-secondary`,
  `--color-success` / `-bg`, `--color-warning` / `-bg`, `--color-error` / `-bg`,
  `--color-info` / `-bg`. `--color-text-muted` (stone-5) is **retired** — it
  fails AA for text; use `--color-text-secondary` (stone-6) for secondary
  copy, or `--stone-7` for small mono labels.
- **Stone**: `--stone-0` through `--stone-10` (warm neutral ramp — surfaces, borders, text)
- **Clay**: `--clay-0` through `--clay-6` (primary accent — buttons, links, active state)
- **Ember**: `--ember` (single bright accent, reserved for Focus-mode CTAs)

#### Border & Typography

- **Radius**: `--radius-1` through `--radius-6`, `--radius-round`
- **Border sizes**: `--border-size-1` through `--border-size-5`
- **Shadows (elevation)**: `--shadow-1` (subtle), `--shadow-2` (card), `--shadow-3` (raised). Use these instead of inline `box-shadow` values; reach for raw `box-shadow` only for color-tinted glows or focus rings.
- **Font families**: `--font-sans` (system-ui), `--font-serif` (Charter
  → Iowan Old Style → Georgia → ui-serif), `--font-mono` (the developer
  waterfall in `--font-monospace-code`). Three roles — display serif,
  body sans, mono annotation — documented under "Design Language"
  above. Zero `@font-face` rules; do not add web-font loading.
- **Font weights**: `--font-weight-1` through `--font-weight-9`
- **Font sizes**: `--font-size-00` (0.5rem) and `--font-size-0` through `--font-size-8`
- **Fluid font sizes**: `--font-size-fluid-0` through `--font-size-fluid-3` (responsive via `clamp()`)

### Color Usage Patterns

- Use proper contrast ratios by pairing light backgrounds with dark text
- Map semantic intentions to the semantic tokens (e.g., success state → `--color-success-bg` background, `--color-success` text; neutral surface → `--color-surface` / `--stone-*`)
- **NEVER use undefined color tokens** - always verify they exist in `static/main.css` first

### No `:hover` — mobile-first

The app is mobile-first. `:hover` styles are deliberately omitted because
on touch devices a tap triggers hover and the state sticks until the next
interaction (the "sticky hover" bug). The same reasoning rules out
`cursor: pointer` and other mouse-only declarations — they have no effect
on touch and add no value to the mobile experience.

Reserve interactive-state styling for `:active` (fires on touch and mouse
alike) and `:focus-visible` (keyboard). Don't reintroduce `:hover`,
`cursor: pointer`, `@media (hover: hover)` gates, or `mouseenter` /
`mouseleave` listeners.

### Focus rings — outline, never a glow

Focus-visible is a **2px `--color-border-focus` (clay-4) outline at 2px
offset** — clay-4 clears the 3:1 non-text floor (4.3:1 on stone-0). The
offset is applied once globally (`:focus-visible { outline-offset: 2px }`
in `static/main.css`); per-component rules only set the outline colour/width
via the token. There is **no glow token** — don't reach for a
`box-shadow` focus ring.

### Active-state darkening — named-ramp step vs `color-mix`

There are two ways to nudge a token a shade darker for `:active`:
a named ramp step (e.g. `var(--clay-6)` to depress a `--clay-4` base), or
`color-mix(in oklab, var(--BASE) NN%, black)`. The decision is mechanical:

- **Use the ramp step when one exists.** Clay primaries depress on
  `var(--clay-6)`. Terse, responds to ramp retunes, no perceptual math at
  the call site.
- **Use `color-mix(... black)` when no darker ramp step exists.** Applies
  to `--color-error`, `--color-info`, `--color-info-bg`,
  `--color-success-bg`, and `--ember` — none have a `*-darker` companion,
  so mixing toward black is the way.

#### `color-mix` percentage convention

Picked perceptually in oklab — lighter bases need less black to register
the state change, so the percentages slide toward 100%:

| Base lightness                                  | Active       | Example                                      |
|-------------------------------------------------|--------------|----------------------------------------------|
| Mid (saturated, e.g. `--color-error`)           | `78%, black` | `.btn--danger` in `static/main.css`       |
| Bright accent (`--ember`)                       | `85%, black` | Focus-mode CTA in `sets-container.gohtml`    |

If you reach for `color-mix(... var(--clay-N) NN%, black)` you're
probably reinventing `var(--clay-N+1)` — use the ramp step instead.
