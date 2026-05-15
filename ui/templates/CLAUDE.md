# Template & CSS Guidelines - UI Layer

Guidelines for working with Go templates, CSS architecture, and design systems in `ui/templates/` and `ui/static/`.

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

**Class-components** (`main.css @layer components`): `.btn`/`button`,
`.badge` (+ `--success`/`--warning`/`--neutral`/`--info`), `.card`.

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
- **Gray colors**: `--gray-0` through `--gray-10` — transitional aliases onto Stone.
  Don't reach for these in new code; use `--stone-*`. Existing `--gray-*` call sites
  are renamed to `--stone-*` as each page is polished.
- **Raw ramps** (`--sky-*`, `--lime-*`, `--yellow-*`, `--red-*`) — legacy; still defined
  but being replaced by the semantic tokens above. Don't introduce new uses.

#### Border & Typography

- **Radius**: `--radius-1` through `--radius-6`, `--radius-round`
- **Border sizes**: `--border-size-1` through `--border-size-5`
- **Shadows (elevation)**: `--shadow-1` (subtle), `--shadow-2` (card), `--shadow-3` (raised). Use these instead of inline `box-shadow` values; reach for raw `box-shadow` only for color-tinted glows or focus rings.
- **Font weights**: `--font-weight-1` through `--font-weight-9`
- **Font sizes**: `--font-size-00` (0.5rem) and `--font-size-0` through `--font-size-8`
- **Fluid font sizes**: `--font-size-fluid-0` through `--font-size-fluid-3` (responsive via `clamp()`)

### Color Usage Patterns

- Use proper contrast ratios by pairing light backgrounds with dark text
- Map semantic intentions to the semantic tokens (e.g., success state → `--color-success-bg` background, `--color-success` text; neutral surface → `--color-surface` / `--stone-*`)
- **NEVER use undefined color tokens** - always verify they exist in main.css first

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
