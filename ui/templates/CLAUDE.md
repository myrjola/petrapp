# Template & CSS Guidelines - UI Layer

Guidelines for working with Go templates, CSS architecture, and design systems in `ui/templates/` and `ui/static/`.

## Template Structure

### Template Organization
- Page templates are organized in `/ui/templates/pages/{pageName}/` folders
- Each page template defines a `{{ define "page" }}` block
- All pages extend the base template which provides the HTML structure
- Include gotype comments at the top: `{{- /*gotype: github.com/myrjola/petrapp/cmd/web.TemplateDataType*/ -}}`

### Template Rendering Flow
- Handlers call `app.render(w, r, statusCode, "template-name", data)`
- Template name corresponds to folder name in `pages/`
- Base template wraps page content and provides shared HTML structure
- Page-specific templates focus only on content within `<main>`

## Available Template Functions

### Security Functions (Always Available)
- `{{ nonce }}` - CSP nonce attribute for style/script tags (required for CSP compliance)
- `{{ csrf }}` - CSRF token input field for forms (required for state-changing operations)
- `{{ mdToHTML "markdown content" }}` - Convert markdown to HTML

### Important Security Requirements
- **NEVER use inline styles without nonce** - all `<style>` tags must include `{{ nonce }}`
- **ALL forms must include `{{ csrf }}`** for CSRF protection
- Use `{{ nonce }}` for any `<script>` tags as well

Example:
```gohtml
<style {{ nonce }}>
    /* CSS here */
</style>

<form method="post">
    {{ csrf }}
    <!-- form fields -->
</form>
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
- **Font weights**: `--font-weight-1` through `--font-weight-9`
- **Font sizes**: `--font-size-0` through `--font-size-8`

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