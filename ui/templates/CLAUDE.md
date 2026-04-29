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

### Current Components

- `back-link` — canonical "← Back" anchor wired into the Navigation API via `data-back-button`. Takes an href string as the dot.
  ```gohtml
  {{ template "back-link" "/" }}
  {{ template "back-link" (printf "/workouts/%s" (.Date.Format "2006-01-02")) }}
  ```
  Self-contained: styles live in the component file as a scoped `<style {{ nonce }}>` block.

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
                    background: var(--gray-1);
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

- **Gray colors**: `--gray-0` through `--gray-10` (light to dark)
- **Success colors**: `--lime-0` through `--lime-10` (light to dark)
- **Warning colors**: `--yellow-0` through `--yellow-12`
- **Error colors**: `--red-0` through `--red-12`
- **Info/Accent colors**: `--sky-0` through `--sky-10`
- **Semantic colors**: `--color-success`, `--color-success-bg`, etc.

#### Border & Typography

- **Radius**: `--radius-1` through `--radius-6`, `--radius-round`
- **Border sizes**: `--border-size-1` through `--border-size-5`
- **Shadows (elevation)**: `--shadow-1` (subtle), `--shadow-2` (card), `--shadow-3` (raised). Use these instead of inline `box-shadow` values; reach for raw `box-shadow` only for color-tinted glows or focus rings.
- **Font weights**: `--font-weight-1` through `--font-weight-9`
- **Font sizes**: `--font-size-00` (0.5rem) and `--font-size-0` through `--font-size-8`
- **Fluid font sizes**: `--font-size-fluid-0` through `--font-size-fluid-3` (responsive via `clamp()`)

### Color Usage Patterns

- Use proper contrast ratios by pairing light backgrounds with dark text
- Map semantic intentions to available tokens (e.g., success state → `--lime-2` background, `--lime-9` text)
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

### Data-First Approach

- **STRONGLY PREFER preparing data in Go handlers** rather than complex template logic
- Transform indices, format dates, compute derived values in handlers before passing to templates
- Use simple iteration and display in templates - avoid complex conditionals
- When templates need computed values, transform the data structure in the handler

### Template Logic Guidelines

- Keep templates simple and logic-free
- Use range loops for iteration, basic if/else for display logic
- For complex formatting or calculations, prepare data in handlers
- Avoid nested template logic - flatten data structures in Go code instead

### URL Construction

- Inline `printf` for URL formatting is fine and idiomatic here: `{{ printf "/workouts/%s/exercises/%d" (.Date.Format "2006-01-02") .Exercise.ID }}`
- Only pre-build URLs in the handler when the same URL is used in several places on the page, or the path depends on non-trivial logic

### Common Transformation Examples

```go
// Handler: Transform enum to display options
Difficulties: []difficultyOption{
{Value: difficultyTooEasy, Label: "Too easy"},
{Value: difficultyICouldDoMore, Label: "I could do more"},
}

// Handler: Filter and prepare collections
var availableExercises []workout.Exercise
for _, exercise := range allExercises {
if !existingExerciseIDs[exercise.ID] {
availableExercises = append(availableExercises, exercise)
}
}
```

## Error Recovery Patterns

### When Templates Fail to Render

1. **Check missing template functions** - verify functions exist in handlers.go
2. **Fix data preparation** - don't add template complexity, transform data in handlers
3. **Validate template syntax** - check for unclosed blocks, typos in function names
4. **Check data structure mismatches** - ensure template expects correct data types

### Common Template Errors

- "function not defined" → Check if logic belongs in handler data preparation
- "nil pointer" → Validate data structure in handler before passing to template
- "unexpected token" → Check template syntax, especially in scoped CSS blocks

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

### DOM Structure for Tests

- Use semantic HTML elements and meaningful CSS classes
- Include data attributes for test targeting when needed
- Ensure form actions and button text are descriptive for test reliability
- Structure templates so e2e tests can find elements consistently

### Test-Friendly Patterns

- Use unique button text and headings for test selectors
- Include CSS classes that won't change frequently
- Structure forms with clear action attributes and consistent field names
