# Templates & CSS — UI Layer

Reference for Go templates, CSS architecture, and component conventions in
`cmd/petra/ui/templates/` and `cmd/petra/ui/static/`.

## What lives here

- **Page templates** (`pages/`) and **shared components** (`components/`) —
  Go `html/template` markup.
- **CSS** — colocated scoped `<style>` blocks plus the global `cmd/petra/ui/static/main.css`.
- **Static assets** (`cmd/petra/ui/static/`) — `main.js`, `webauthn.js`, icons.

## What does NOT live here

- Handler-side data preparation and per-template data structs — `cmd/petra/`
  (see the [web-layer README](../../README.md)).
- Business rules and display derivations on domain types — `internal/petra/domain/`.

## Design Language

The visual identity is an *editorial training logbook* — warm, restrained,
low-density. **Before any visual or CSS work, read
[`../design-system.md`](../design-system.md)** — the full design
language, the design-token catalogue, and the colour / motion / active-state
conventions live there.

Non-negotiables that hold even for non-visual changes:

- **Warm-only palette**, system font stacks only — **no `@font-face`, no
  web-font CDN requests**.
- **State is never colour-only** — every state also carries a shape, line, or
  wash signal (WCAG SC 1.4.1).
- **No `:hover` / `cursor: pointer`** — the app is mobile-first; style
  `:active` and `:focus-visible` instead.

## Copy register

All user-facing copy targets the **average gym user** — someone who has
never read training literature. The canonical term-to-label mapping (and the
terms that must never appear in copy: MEV/MRV, session goal, hypertrophy,
fractional set) lives in the root [CONTEXT.md](../../../../CONTEXT.md) under
"UI register". When a template needs a new label for a domain concept, check
that table first; if the concept isn't mapped yet, add the mapping there in
the same change.

## Template Structure

### Template Organization

- Page templates are organized in `cmd/petra/ui/templates/pages/{pageName}/` folders
- Each page template defines a `{{ define "page" }}` block
- All pages extend the base template which provides the HTML structure
- Include gotype comments at the top: `{{- /*gotype: github.com/myrjola/petrapp/cmd/petra.TemplateDataType*/ -}}`. This is read by the JetBrains Go Template plugin to give type-aware completion inside the template. Keep the fully-qualified struct path in sync when you rename the Go type — nothing will fail to compile if it drifts, but IDE hints will silently go stale
- Reusable components live in `cmd/petra/ui/templates/components/` and are available to every page automatically — see "Shared Components" below

### JavaScript in Templates

**ALWAYS prefer inline scripts in templates over static JavaScript files.**

- Include JavaScript directly in template files using `<script {{ $.Nonce }}>` tags
- Inline scripts provide better developer experience (no cache busting needed)
- Static files in `cmd/petra/ui/static/` are cached with fingerprinted filenames for performance
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

- Component templates live in `cmd/petra/ui/templates/components/*.gohtml`
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
  the a11y wiring. Dot structs live in `cmd/petra/components.go`.
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

- `back-link` — "← Back" anchor wired to the Navigation API. Dot:
  `cmd/petra.BackLinkData` (`Href`, `Nonce`); construct via the `backLink`
  template helper that builds it from the page's `$.Nonce`.
  ```gohtml
  {{ template "back-link" (backLink "/" $.Nonce) }}
  {{ template "back-link" (backLink (printf "/workouts/%s" (.Date.Format "2006-01-02")) $.Nonce) }}
  ```
- `banner` — server-message display (flash errors, notices). Dot:
  `cmd/petra.BannerData` (`Variant` ∈ `error`/`success`/`info`, `Message`).
  Renders nothing when `Message` is empty.
- `page-header` — a page's single `<h1>` with optional subtitle. Dot:
  `cmd/petra.PageHeaderData` (`Title`, `Subtitle`). Render meta/badges as
  siblings after it, not inside it.
- `field` — a labelled single text input; guarantees the `<label for>` ↔
  `<input id>` binding and `aria-describedby` → hint wiring. Dot:
  `cmd/petra.FieldData`. Covers `<input>` only — `<select>`, `<textarea>` and
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
new component, and assert it in `cmd/petra/handler-styleguide_test.go`.

### Styling Components

- **Colocate styles with markup.** Emit a `<style {{ .Nonce }}>` block as a sibling immediately preceding the component root (not inside it — `<style>` is metadata content and is non-conforming inside interactive content like `<a>`, `<button>`, or form controls)
- Use `@scope (<selector>) { :scope { ... } }` with a class or data attribute that uniquely identifies the component root. Example: `@scope (.back-link) { :scope { ... } }`
- Pages that render their own elements matching the same selector will still override the component's rules via their own unlayered inline `<style>` blocks — unlayered wins by cascade proximity / source order
- The whole component — markup and styles — lives in one file. Delete the file to delete the feature; nothing to hunt down in `main.css`
- If a component is rendered many times on the same page and the duplicated `<style>` tag becomes a real cost, hoist the rule to `main.css` under `@layer components` (measure first; gzip makes duplicate inline `<style>` blocks cheap)
- Only add rules to `main.css` if they need to apply to markup outside the component — otherwise keep them local
- **Promotion rule ([ADR 0007](../../../docs/adr/0007-inline-scoped-page-css.md)).** Inline is for genuinely page-unique layout and template-emitted dynamic values. A pattern reused in **≥3 templates** (overline, display title, gradient divider…) graduates to a class/utility in `main.css`. Any inline literal that duplicates a token is a bug at *any* count — use the token (e.g. the serif display stack is `var(--font-serif)`, never a hardcoded `Charter, …` chain). "First used here" is not "specific to here."

## Available Template Functions

### Security helpers (data-driven)

CSP nonces are passed through data, not via a template function:

- In page templates and `base.gohtml`, reference `{{ $.Nonce }}` — `$` reaches the page-data root even from inside `{{ range }}` / `{{ with }}` blocks.
- In component partials, the component's dot struct carries a `Nonce template.HTMLAttr` field; reference it as `{{ .Nonce }}`. Callers populate it from their surrounding `BaseTemplateData.Nonce`.

Markdown rendering happens in the handler — pre-render to `template.HTML` and pass it on the data struct. There is no `mdToHTML` template function.

### Important Security Requirements

- **All `<style>` and `<script>` tags must carry their `Nonce` (`{{ .Nonce }}` inside a component, `{{ $.Nonce }}` inside a page)** so the CSP nonce-allowlist accepts them.
- **Never use inline `style="..."` attributes on elements.** The CSP `style-src` directive uses a nonce, and once a nonce is present in CSP Level 3 the `'unsafe-inline'` keyword is ignored even for style attributes — the browser silently drops the rule. Nonces apply to `<style>` elements, not to attributes; there is no way to "nonce" an inline attribute.
  - **For dynamic CSS values driven by template data** (e.g. one rule per token, or a value that depends on a handler-prepared list): emit the rules from inside a nonce'd `<style>` block by ranging over the data, then reference them from the markup as plain class names. See `cmd/petra/ui/templates/pages/styleguide/styleguide.gohtml` for a worked example — it generates `.bg-{token}`, `.w-{token}`, `.fs-{token}` etc. inside `<style {{ $.Nonce }}>` and the markup just uses the class.
  - **For one-off dynamic values** (e.g. a unique `view-transition-name` per row): same approach — emit a single rule inside a `<style {{ $.Nonce }}>` adjacent to the element, scoped via `@scope` or a unique class/data-attribute.

Example:

```gohtml
<style {{ $.Nonce }}>
    /* CSS here */
</style>

<script {{ $.Nonce }}>
    /* JS here */
</script>
```

## JavaScript & CSP (Trusted Types)

The CSP set in `cmd/petra/middleware.go` includes `require-trusted-types-for 'script'`. The browser then rejects raw strings passed to "script-loading sinks" — they must be a `TrustedHTML`, `TrustedScript`, or `TrustedScriptURL` value produced by a `TrustedTypePolicy`. Forget this and the call throws `TypeError: This assignment requires a TrustedScriptURL` (or `TrustedHTML` / `TrustedScript`).

### Sinks to avoid

| Category | Examples |
| --- | --- |
| HTML sinks | `el.innerHTML`, `el.outerHTML`, `el.insertAdjacentHTML`, `document.write`, `Range.createContextualFragment` |
| Script sinks | `eval`, `new Function`, `setTimeout(stringArg, ...)`, `setInterval(stringArg, ...)` |
| Script-URL sinks | `script.src = …`, `new Worker(url)`, `new SharedWorker(url)`, `importScripts(url)`, `navigator.serviceWorker.register(url)` |

### Build DOM with nodes, not HTML strings

The CSP would block string-to-HTML conversion anyway, and there's no reason to assemble markup at runtime when the server already renders templates. Patterns to prefer:

- **Text into an existing element** — set `textContent` (or `innerText`). Both are inert to markup. Existing examples: `errorEl.textContent = msg` in `cmd/petra/ui/templates/pages/preferences/preferences.gohtml`, `timeEl.textContent = m + ':' + …` in `cmd/petra/ui/templates/pages/exerciseset/sets-container.gohtml`.
- **Build new nodes** — `document.createElement(tag)`, set properties via setters (`el.className`, `el.dataset.x`, `el.href`), append with `el.append(child, ' text ', otherChild)`. Plain strings passed to `append` become text nodes, not parsed HTML.
- **Repeat server-rendered markup client-side** — declare an inert `<template id="…">` block in the page template (the `<template>` element is parsed but its contents aren't rendered), then `template.content.cloneNode(true)` in JS and fill in the clone with `textContent` / property setters. The markup still lives in the server template, which means it's still typechecked by gotype comments and styled by the page's `<style>` block.
- **Show/hide** — toggle classes (`.classList.add('hidden')` / `.remove('hidden')`); don't swap in pre-built HTML strings.

### Script-URL sinks need a Trusted Types policy

When you genuinely have to load a script (a service worker, a Worker), define a `TrustedTypePolicy` whose `createScriptURL` callback whitelists the exact URLs and feed all URLs through it. The policy is the one chokepoint where the allowlist lives, so a code review only has to audit that callback.

The service-worker URL already goes through one such policy. Don't call `navigator.serviceWorker.register('/sw.js')` directly — call `registerServiceWorker()` from `cmd/petra/ui/static/main.js`, which routes through the `sw-loader` policy. If you need to load a new script URL, **add it to that policy's allowlist** rather than creating a second policy: a policy name can only be created once per realm, and the CSP has no `trusted-types … 'allow-duplicates'` directive, so `createPolicy('sw-loader', …)` a second time throws.

Dynamic `import()` of bare specifiers resolved through the `<script type="importmap">` (e.g. `await import("webauthn")`) is the existing pattern for code-split modules; it works because import-map keys aren't treated as URL sinks. Stick with it for new modules — register them in the importmap in `cmd/petra/ui/templates/base.gohtml`.

### Inline scripts are still preferred

The "JavaScript in Templates" guidance above is unchanged. The nonce on `<script {{ $.Nonce }}>` authorizes the inline JS itself; Trusted Types is a separate runtime constraint on what that JS does. Inline scripts must follow the same DOM-construction and script-URL rules — there's no relaxation for being inline.

### Client-only error surface (`#js-flash`)

`base.gohtml` renders a hidden `<div id="js-flash" role="alert" hidden>` above
the page content. The Stack Navigator shim populates it via `textContent` on
`fetch` failure (offline / DNS / CORS) — the standard "live region whose text
changes" pattern, announced by screen readers without focus moves. Use
`textContent` only (CSP / Trusted Types blocks `innerHTML`). The skeleton is
*not* for server-originating errors: those flow through `app.userError` →
flash + redirect → server-rendered `banner` component on the next GET — see
"Error Handling" in the [web-layer README](../../README.md). Keep the
client surface as a true last resort.

## CSS Architecture and Scoping

### Scoped CSS Pattern

- Use `@scope` at-rules for page-specific component styles
- Place scoped styles directly in template files; reference `{{ $.Nonce }}` (or `{{ .Nonce }}` in components) on the `<style>` tag
- Avoid global CSS classes for page-specific styling
- Only add to `main.css` if truly global and reusable

### Scoped CSS Example

```gohtml
<div class="exercise-list">
    <style {{ $.Nonce }}>
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
in the [web-layer README](../../README.md) under "Data Transformation
Patterns"; the domain-rule escalation lives in
[`internal/petra/domain/README.md`](../../../../internal/petra/domain/README.md)
under "Display derivations belong on domain types."

Inline `printf` for one-off URL construction is idiomatic:
`{{ printf "/workouts/%s/exercises/%d" (.Date.Format "2006-01-02") .Exercise.ID }}`.
Pre-build URLs in the handler only when the same URL appears in several places
on the page or the path depends on non-trivial logic.

When a template fails to render: missing function ⇒ check `templateFuncs`
in `cmd/petra/handlers.go` and consider moving the logic to data preparation; nil
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
real HTML and assert against goquery selections — see "Testing with e2etest"
in the [web-layer README](../../README.md) for the canonical patterns. When
authoring markup: prefer semantic elements, give forms descriptive `action`
attributes, and keep button/heading text stable so selectors stay resilient.
