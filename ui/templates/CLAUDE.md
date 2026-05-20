# Template & CSS Guidelines - UI Layer

Guidelines for working with Go templates, CSS architecture, and design systems in `ui/templates/` and `ui/static/`.

## Design Language

The app's visual identity is an *editorial training logbook*, not a fitness
SaaS. Closest references: field notebooks, atelier signage, small-press
editorial design. Restrained, warm, intentional. Density is low — every
element earns its space.

### Identity rules

- **Warm-only palette.** Stone neutrals + Clay terracotta accent + Ember
  one-off pop. No cool grays anywhere; shadows are warm-tinted
  (`rgb(120 90 60 / .10)`), never neutral black. The semantic-token
  catalogue is documented under "Color System" below.
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
  `--clay-4` → serif display title (`clamp(2.25rem, 9vw, 3rem)`,
  line-height `0.95`, tracking `-0.025em`) → meta row. The overline ends
  with a 1px hairline fading right via
  `linear-gradient(to right, var(--stone-3), transparent)`. Worked
  example in `pages/workout/workout.gohtml` and
  `pages/exerciseset/exercise-header.gohtml`. Reuse this rhythm; don't
  reinvent it.
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

## Template Structure

### Template Organization

- Page templates are organized in `/ui/templates/pages/{pageName}/` folders
- Each page template defines a `{{ define "page" }}` block
- All pages extend the base template which provides the HTML structure
- Include gotype comments at the top: `{{- /*gotype: github.com/myrjola/petrapp/cmd/web.TemplateDataType*/ -}}`. This is read by the JetBrains Go Template plugin to give type-aware completion inside the template. Keep the fully-qualified struct path in sync when you rename the Go type — nothing will fail to compile if it drifts, but IDE hints will silently go stale
- Reusable components live in `/ui/templates/components/` and are available to every page automatically — see "Shared Components" below

### JavaScript in Templates

**ALWAYS prefer inline scripts in templates over static JavaScript files.**

- Include JavaScript directly in template files using `<script {{ nonce }}>` tags
- Inline scripts provide better developer experience (no cache busting needed)
- Static files in `/ui/static/` are cached with fingerprinted filenames for performance
- Changing static files requires renaming them to bust the cache
- Inline scripts update immediately when templates are reloaded

**When to use inline scripts:**

- Page-specific JavaScript logic
- Scripts that benefit from template context or dynamic values
- Any JavaScript that may need frequent updates during development

**When to use static files:**

- Large third-party libraries (e.g., echarts, webauthn)
- Scripts that rarely change and benefit from long-term caching

### Template Rendering Flow

- Handlers call `app.render(w, r, statusCode, "template-name", data)`
- Template name corresponds to folder name in `pages/`
- Base template wraps page content and provides shared HTML structure
- Page-specific templates focus only on content within `<main>`

## Shared Components

### Where Components Live

- Component templates live in `/ui/templates/components/*.gohtml`
- Every file in this folder is parsed alongside every page, so any `{{ define "component-name" }}` block defined here is callable from any page via `{{ template "component-name" <data> }}`
- One component per file; the filename should match the defined template name (e.g. `back-link.gohtml` defines `back-link`)
- Keep the dot (`.`) passed to a component minimal — a string, a small struct — not the whole page data

### When to Add a Component

- **Extract only when real duplication exists.** Three nearly-identical sites is the threshold. Two is borderline; one is premature.
- Prefer small, presentational components (anchors, buttons, banners) over wrappers that try to capture layout
- If two candidate usages differ in more than trivial attributes (label wording, icon presence), consider whether they're really the same component or just look similar

### Component delivery — the guarantee-based split

A component's delivery mechanism is decided by what it must *guarantee*:

- **Go partial** (`components/*.gohtml`, colocated `@scope` `<style>`) — for
  pieces that enforce accessibility or structure: `field`, `banner`,
  `page-header`, `back-link`. The caller passes a small dot and cannot forget
  the a11y wiring. Dot structs live in `cmd/web/components.go`.
- **CSS class** (`main.css @layer components`) — for pure-paint pieces on a
  semantic element: `button`/`.btn`, `.badge`, `.card`. These compose freely
  and have zero per-render cost.
- **Layout primitive class** (`main.css @layer layout`) — `.stack`, `.cluster`,
  `.grid-auto`, `.center`. Reach for these before writing
  `display:flex; gap:…` in a scoped block.
- **Inline scoped `<style>`** — the escape hatch for genuinely page-specific
  composition. Still fine; just not the first reach for layout or for the
  pieces above.

### Primitives first, bespoke when warranted

Before writing `display: flex` or `display: grid` in a scoped `<style>`, ask:
would another page reasonably want this layout? If yes, reach for `.stack` /
`.cluster` / `.grid-auto` / `.center` — an extra `gap` override on a primitive
is still cheaper than the next page reinventing the same flex column. Scoped
`<style>` remains the documented escape hatch for genuinely page-specific
composition (asymmetric hero grids, editorial landing layouts), but treat it
as escalation, not the default. The signal isn't admin-vs-user — it's reuse
potential and change frequency: high-churn pages (`workout.gohtml`,
`sets-container.gohtml`, admin lists) pay the duplication cost every time
they're touched, so the bar to escape the primitives is highest there.

### Current components

**Partials** (call via `{{ template "name" <dot> }}`):

- `back-link` — "← Back" anchor wired to the Navigation API. Dot: an href
  string.
  ```gohtml
  {{ template "back-link" "/" }}
  {{ template "back-link" (printf "/workouts/%s" (.Date.Format "2006-01-02")) }}
  ```
- `banner` — server-message display (flash errors, notices). Dot:
  `cmd/web.BannerData` (`Variant` ∈ `error`/`success`/`info`, `Message`).
  Renders nothing when `Message` is empty.
- `page-header` — a page's single `<h1>` with optional subtitle. Dot:
  `cmd/web.PageHeaderData` (`Title`, `Subtitle`). Render meta/badges as
  siblings after it, not inside it.
- `field` — a labelled single text input; guarantees the `<label for>` ↔
  `<input id>` binding and `aria-describedby` → hint wiring. Dot:
  `cmd/web.FieldData`. Covers `<input>` only — `<select>`, `<textarea>` and
  checkbox/radio groups stay as inline markup.

**Class-components** (`main.css @layer components`):

- `.btn`/`button` — primary (default) + variants `.btn--quiet`,
  `.btn--ghost`, `.btn--danger`, `.btn--focus`; modifiers `.btn--sm`,
  `.btn--block`. Variants are mutually exclusive; modifiers compose with
  any variant. The base rule guarantees a 44 px min touch target and a
  visible `:disabled` state. Every variant is in `/dev/styleguide`.
- `.badge` (+ `--success`/`--warning`/`--neutral`/`--info`).
- `.card`.
- `.muscle-chip` (+ `--primary`) — pill for muscle-group labels on the
  exercise-info / -add / -swap pages. `--primary` uses the Clay accent
  for primary-muscle emphasis; the base uses Stone for secondary muscles.
- `.sheet-dialog` (+ `__close` row) — slide-up modal sheet used for
  per-card exercise info on the Add and Swap pages. The companion
  `.sheet-dialog__close` element pins the close button to the top-right.

**Layout primitives** (`main.css @layer layout`): `.stack`, `.cluster`,
`.grid-auto`, `.center`.

The `/dev/styleguide` page is the living catalog — add an entry there for any
new component, and assert it in `cmd/web/handler-styleguide_test.go`.

### Styling Components

- **Colocate styles with markup.** Emit a `<style {{ nonce }}>` block as a sibling immediately preceding the component root (not inside it — `<style>` is metadata content and is non-conforming inside interactive content like `<a>`, `<button>`, or form controls)
- Use `@scope (<selector>) { :scope { ... } }` with a class or data attribute that uniquely identifies the component root. Example: `@scope (.back-link) { :scope { ... } }`
- Pages that render their own elements matching the same selector will still override the component's rules via their own unlayered inline `<style>` blocks — unlayered wins by cascade proximity / source order
- The whole component — markup and styles — lives in one file. Delete the file to delete the feature; nothing to hunt down in `main.css`
- If a component is rendered many times on the same page and the duplicated `<style>` tag becomes a real cost, hoist the rule to `main.css` under `@layer components` (measure first; gzip makes duplicate inline `<style>` blocks cheap)
- Only add rules to `main.css` if they need to apply to markup outside the component — otherwise keep them local

## Available Template Functions

### Security Functions (Always Available)

- `{{ nonce }}` - CSP nonce attribute for style/script tags (required for CSP compliance)
- `{{ mdToHTML "markdown content" }}` - Convert markdown to HTML

### Important Security Requirements

- **All `<style>` and `<script>` tags must carry `{{ nonce }}`** so the CSP nonce-allowlist accepts them.
- **Never use inline `style="..."` attributes on elements.** The CSP `style-src` directive uses a nonce, and once a nonce is present in CSP Level 3 the `'unsafe-inline'` keyword is ignored even for style attributes — the browser silently drops the rule. Nonces apply to `<style>` elements, not to attributes; there is no way to "nonce" an inline attribute.
  - **For dynamic CSS values driven by template data** (e.g. one rule per token, or a value that depends on a handler-prepared list): emit the rules from inside a nonce'd `<style>` block by ranging over the data, then reference them from the markup as plain class names. See `ui/templates/pages/styleguide/styleguide.gohtml` for a worked example — it generates `.bg-{token}`, `.w-{token}`, `.fs-{token}` etc. inside `<style {{ nonce }}>` and the markup just uses the class.
  - **For one-off dynamic values** (e.g. a unique `view-transition-name` per row): same approach — emit a single rule inside a `<style {{ nonce }}>` adjacent to the element, scoped via `@scope` or a unique class/data-attribute.

Example:

```gohtml
<style {{ nonce }}>
    /* CSS here */
</style>

<script {{ nonce }}>
    /* JS here */
</script>
```

## JavaScript & CSP (Trusted Types)

The CSP set in `cmd/web/middleware.go` includes `require-trusted-types-for 'script'`. The browser then rejects raw strings passed to "script-loading sinks" — they must be a `TrustedHTML`, `TrustedScript`, or `TrustedScriptURL` value produced by a `TrustedTypePolicy`. Forget this and the call throws `TypeError: This assignment requires a TrustedScriptURL` (or `TrustedHTML` / `TrustedScript`).

### Sinks to avoid

| Category | Examples |
| --- | --- |
| HTML sinks | `el.innerHTML`, `el.outerHTML`, `el.insertAdjacentHTML`, `document.write`, `Range.createContextualFragment` |
| Script sinks | `eval`, `new Function`, `setTimeout(stringArg, ...)`, `setInterval(stringArg, ...)` |
| Script-URL sinks | `script.src = …`, `new Worker(url)`, `new SharedWorker(url)`, `importScripts(url)`, `navigator.serviceWorker.register(url)` |

### Build DOM with nodes, not HTML strings

The CSP would block string-to-HTML conversion anyway, and there's no reason to assemble markup at runtime when the server already renders templates. Patterns to prefer:

- **Text into an existing element** — set `textContent` (or `innerText`). Both are inert to markup. Existing examples: `errorEl.textContent = msg` in `ui/templates/pages/preferences/preferences.gohtml`, `timeEl.textContent = m + ':' + …` in `ui/templates/pages/exerciseset/sets-container.gohtml`.
- **Build new nodes** — `document.createElement(tag)`, set properties via setters (`el.className`, `el.dataset.x`, `el.href`), append with `el.append(child, ' text ', otherChild)`. Plain strings passed to `append` become text nodes, not parsed HTML.
- **Repeat server-rendered markup client-side** — declare an inert `<template id="…">` block in the page template (the `<template>` element is parsed but its contents aren't rendered), then `template.content.cloneNode(true)` in JS and fill in the clone with `textContent` / property setters. The markup still lives in the server template, which means it's still typechecked by gotype comments and styled by the page's `<style>` block.
- **Show/hide** — toggle classes (`.classList.add('hidden')` / `.remove('hidden')`); don't swap in pre-built HTML strings.

### Script-URL sinks need a Trusted Types policy

When you genuinely have to load a script (a service worker, a Worker), define a `TrustedTypePolicy` whose `createScriptURL` callback whitelists the exact URLs and feed all URLs through it. The policy is the one chokepoint where the allowlist lives, so a code review only has to audit that callback.

The service-worker URL already goes through one such policy. Don't call `navigator.serviceWorker.register('/sw.js')` directly — call `registerServiceWorker()` from `ui/static/main.js`, which routes through the `sw-loader` policy. If you need to load a new script URL, **add it to that policy's allowlist** rather than creating a second policy: a policy name can only be created once per realm, and the CSP has no `trusted-types … 'allow-duplicates'` directive, so `createPolicy('sw-loader', …)` a second time throws.

Dynamic `import()` of bare specifiers resolved through the `<script type="importmap">` (e.g. `await import("webauthn")`) is the existing pattern for code-split modules; it works because import-map keys aren't treated as URL sinks. Stick with it for new modules — register them in the importmap in `ui/templates/base.gohtml`.

### Inline scripts are still preferred

The "JavaScript in Templates" guidance above is unchanged. The nonce on `<script {{ nonce }}>` authorizes the inline JS itself; Trusted Types is a separate runtime constraint on what that JS does. Inline scripts must follow the same DOM-construction and script-URL rules — there's no relaxation for being inline.

### Client-only error surface (`#js-flash`)

`base.gohtml` renders a hidden `<div id="js-flash" role="alert" hidden>` above
the page content. The Stack Navigator shim populates it via `textContent` on
`fetch` failure (offline / DNS / CORS) — the standard "live region whose text
changes" pattern, announced by screen readers without focus moves. Use
`textContent` only (CSP / Trusted Types blocks `innerHTML`). The skeleton is
*not* for server-originating errors: those flow through `app.userError` →
flash + redirect → server-rendered `banner` component on the next GET. Keep
the client surface as a true last resort.

## CSS Architecture and Scoping

### Scoped CSS Pattern

- Use `@scope` at-rules for page-specific component styles
- Place scoped styles directly in template files with nonce attribute
- Avoid global CSS classes for page-specific styling
- Only add to `main.css` if truly global and reusable

### Scoped CSS Example

```gohtml
<div class="exercise-list">
    <style {{ nonce }}>
        @scope {
            :scope {
                display: flex;
                flex-direction: column;
                gap: var(--size-3);
                
                .exercise {
                    padding: var(--size-3);
                    background: var(--color-surface-elevated);
                }
            }
        }
    </style>
    <!-- HTML content -->
</div>
```

### Multiple Scoped Sections

- Use multiple `@scope` blocks within the same template for different components
- Each scope block applies only to its parent container
- Scopes don't interfere with each other or global styles

## Design System Usage

### CSS Custom Properties (Design Tokens)

Always verify these exist in `main.css` before using:

#### Spacing System

- `--size-1` through `--size-15` (0.25rem to 30rem)
- Use for margins, padding, gaps: `margin: var(--size-4)`

#### Color System

The app is on the warm "Stone" palette. **Reach for semantic tokens first**, then
the Stone/Clay ramps; the raw Tailwind-style ramps are legacy holdouts being
retired page-by-page.

- **Semantic colors** (prefer these): `--color-surface`, `--color-surface-elevated`,
  `--color-surface-active`, `--color-surface-completed`, `--color-border`,
  `--color-border-focus`, `--color-text-primary` / `-secondary` / `-muted`,
  `--color-success` / `-bg`, `--color-warning` / `-bg`, `--color-error` / `-bg`,
  `--color-info` / `-bg`
- **Stone**: `--stone-0` through `--stone-10` (warm neutral ramp — surfaces, borders, text)
- **Clay**: `--clay-0` through `--clay-6` (primary accent — buttons, links, active state)
- **Ember**: `--ember` (single bright accent, reserved for Focus-mode CTAs)
- **Raw ramps** (`--sky-*`, `--lime-*`, `--yellow-*`, `--red-*`) — legacy; still defined
  but being replaced by the semantic tokens above. Don't introduce new uses.

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
- **NEVER use undefined color tokens** - always verify they exist in main.css first

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
| Mid (saturated, e.g. `--color-error`)           | `78%, black` | `.btn--danger` in `ui/static/main.css`       |
| Bright accent (`--ember`)                       | `85%, black` | Focus-mode CTA in `sets-container.gohtml`    |

If you reach for `color-mix(... var(--clay-N) NN%, black)` you're
probably reinventing `var(--clay-N+1)` — use the ramp step instead.

## CSS Layer System

The project uses CSS layers defined in main.css:

```css
@layer reset, props, layout, components;
```

### Layer Guidelines

- **reset**: Base CSS reset - don't modify
- **props**: Design tokens and custom properties - verify before using
- **layout**: Page-level layout styles - rare additions
- **components**: Global reusable components - add only truly global styles

## Accessibility

Target: **WCAG 2.2 AA + 48×48 touch targets** (the stricter Material
guideline, above WCAG 2.2's 24×24 SC 2.5.8 minimum).

### Touch targets

The canonical pattern is **make the whole semantic container the trigger**
— a list row, a card, a panel header — with small visual ornaments
inside. This removes the touch-target question for the common case
(`workout.gohtml` exercise rows are ~70 px tall; the 7-px dots inside
are `aria-hidden` ornaments).

When a small, standalone trigger is unavoidable (icon buttons, inline
text actions like the "Info" / "Swap" header links), keep the visual
small and **expand the hit area with an invisible `::before`**:

```css
.brief-action {
  position: relative;     /* visual stays small */
}
.brief-action::before {
  content: "";
  position: absolute;
  inset: 50% 50% 50% 50%;
  min-width: 48px;
  min-height: 48px;
  translate: -50% -50%;
}
```

The pseudo inherits pointer events from the parent link/button — no
`pointer-events` override needed. **Never enlarge the visual chrome to
satisfy the rule**; that breaks the editorial restraint.

`.btn` base enforces `min-height: 3rem` (48 px). `.btn--sm` (32 px) and
any icon-only button must use the `::before` expansion above to clear
the same target.

Densely-packed controls (rare) may rely on WCAG 2.2's spacing
exception: visuals < 48 px are acceptable when no other target's
*centre* falls within a 48 px clearance circle. Document any such
exception in `/dev/styleguide`.

### Colour contrast

Contrast is a property of **token pairs**, not tokens. Maintain a
pairing matrix in `/dev/styleguide` that names every legal
text-on-surface combination with its measured ratio (AA: 4.5:1 body,
3:1 large text ≥ 18 pt or 14 pt bold). Pairings not in the matrix are
forbidden by convention.

Suspect pairings to audit before reuse:

- `--color-text-muted` (`--stone-5`) on `--color-surface` — likely
  borderline for body; reserve for large text or darken to `--stone-6`.
- `--color-text-secondary` (`--stone-6`) on `--color-surface` — verify.
- `--clay-4` overlines on `--stone-1` — verify; may need `--clay-5` for
  body-sized usage.
- Status text on its matching wash (`--color-success` on
  `--color-success-bg`, warning on `-bg`, error on `-bg`) — these were
  tuned muted; need empirical numbers before adding new usages.

**Non-text contrast (SC 1.4.11) must clear 3:1**: form borders, focus
rings, card boundaries, dividers, the strike-through line on completed
cards. The current `.card` border (`--stone-3` on `--stone-1`) is below
3:1 — when a card relies on its border for separation, use `--stone-4`
or heavier; cards that read clearly via elevation + interior content
can drop the border entirely.

### Other standing requirements

- **Information not by colour alone (SC 1.4.1)**: every state ships
  with a shape / line / wash signal in addition to colour. See the
  "Design Language" section.
- **Reduced motion** (`prefers-reduced-motion: reduce`) is wired across
  `main.css`; any new motion must add its own reduced-motion fallback.
- **Forced colours** (`@media (forced-colors: active)`) is handled for
  the strike-through and the loading bar; new graphical signals that
  carry meaning (dots, bars, tabs) must add forced-colours fallbacks
  too — see the `CanvasText` swap in `workout.gohtml`.
- **Focus rings**: prefer `outline-offset: 2px` with a high-contrast
  colour (`--stone-9` or a two-tone `outline` + `box-shadow`). Don't
  rely on a light-tone outline against a similarly-toned background.

## Text overflow & localization-readiness

Layouts must survive text longer than today's English strings. The app is
not localized yet, but German / Finnish / Russian routinely run 30–50%
longer — a layout validated only against compact English WILL break on
translation. Build the resilience in now.

### Let flex/grid children shrink

Flex and grid items default to `min-width: auto` (their min-content width)
and refuse to shrink below their longest word. This is the single most
common cause of text bleeding past a container.

- Use `minmax(0, 1fr)` instead of a bare `1fr` for grid tracks that should
  yield. A bare `1fr` is `minmax(auto, 1fr)` — it will not shrink.
- Put `min-width: 0` on text-bearing flex children.
- Apply `justify-self: end` / `margin-inline-start: auto` only *after* the
  element can shrink — otherwise it pushes itself off the edge.

### Give every text node one overflow strategy

Pick exactly one, deliberately:

- **Wrap** (default, preferred) — no fixed height; if the row holds
  competing items, give the row `flex-wrap: wrap` so the lower-priority
  item drops to its own line.
- **Break** unbreakable tokens (long compounds, URLs) — `overflow-wrap:
  anywhere`. Use `anywhere`, not `break-word`: only `anywhere` shrinks
  min-content, so only `anywhere` also fixes grid/flex track sizing. Add
  `hyphens: auto` for prose.
- **Truncate** — `text-overflow: ellipsis` (inert without `overflow:
  hidden` plus `white-space: nowrap` on the same element) or
  `-webkit-line-clamp`, only for secondary, repeating text whose full
  value is reachable elsewhere (a `title`, an `aria-label`, a detail
  view). Never silently truncate a primary label.

### Don't let two flexible elements fight for one row

Two elements that both want horizontal space will collide. Either let the
row wrap, or let the lower-priority element move to its own line. No fixed
`width` / `height` on text containers — `min-*` plus padding is fine; a
size fitted to an English label breaks in other languages.

### Test for growth

Budget roughly +40% length. Check new components at 320px width with a
deliberately long label, not just the English string. Keep source labels
short — but a short label is never a substitute for a layout that
survives a long one.

Worked example: `pages/home/day-cards.gohtml` — the day name and action
share a `flex-wrap: wrap` row (`.day-headline`), so a long action drops
to its own line instead of bleeding past the card.

## Template Data Preparation

Templates should range, format primitives, and conditionally render — nothing
more. Anything that branches on multiple fields, builds a collection, or
derives a value belongs in the handler. The full rule + worked examples live
in [`cmd/web/CLAUDE.md`](../../cmd/web/CLAUDE.md) under "Data Transformation
Patterns"; the domain-rule escalation lives in
[`internal/domain/CLAUDE.md`](../../internal/domain/CLAUDE.md) under "Display
derivations belong on domain types."

Inline `printf` for one-off URL construction is idiomatic:
`{{ printf "/workouts/%s/exercises/%d" (.Date.Format "2006-01-02") .Exercise.ID }}`.
Pre-build URLs in the handler only when the same URL appears in several places
on the page or the path depends on non-trivial logic.

When a template fails to render: missing function ⇒ check `contextTemplateFuncs`
in `cmd/web/handlers.go` and consider moving the logic to data preparation; nil
pointer ⇒ validate the data shape in the handler; unexpected-token ⇒ check
scoped CSS blocks for unclosed braces.

## View Transitions and Progressive Enhancement

### CSS View Transitions

- Use `view-transition-name` for smooth transitions between pages
- Generate unique transition names using template data (e.g., `exercise-title-{{ .Exercise.ID }}`)
- Applied within scoped CSS for specific components

### Progressive Enhancement

- Templates work without JavaScript
- JavaScript enhances UX but isn't required for functionality
- Use semantic HTML that works without styles or scripts

## Template Testing Considerations

Templates are exercised through `e2etest`-driven handler tests that submit
real HTML and assert against goquery selections — see
[`cmd/web/CLAUDE.md`](../../cmd/web/CLAUDE.md) "Testing with e2etest" for the
canonical patterns. When authoring markup: prefer semantic elements, give
forms descriptive `action` attributes, and keep button/heading text stable so
selectors stay resilient.
