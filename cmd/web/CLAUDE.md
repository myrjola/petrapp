# Web Layer Guidelines - HTTP Handlers & Routing

Guidelines for working with HTTP handlers, routing, middleware, and web server components in `cmd/web/`.

## Handler Structure

### Handler Method Pattern
- Handlers are methods on the `application` struct: `func (app *application) handlerName(w http.ResponseWriter, r *http.Request)`
- All dependencies are available through the `app` struct (services, templates, logger, etc.)
- Use descriptive handler names that indicate HTTP method: `handlerGET`, `handlerPOST`

### URL Parameter Extraction
- Use `r.PathValue("paramName")` to extract URL path parameters
- Always validate extracted parameters (dates, IDs) with proper error handling
- Return `http.NotFound(w, r)` for invalid parameters

### Template Data Structures
- Create dedicated structs for each template that embed `BaseTemplateData`
- Transform all data in handlers before passing to templates - avoid complex template logic
- Use `newBaseTemplateData(r)` to create base data with CSRF tokens and security context

Example:
```go
type workoutTemplateData struct {
    BaseTemplateData
    Date    time.Time
    Session workout.Session
}
```

## Data Transformation Patterns

### Prefer Handler-Side Processing
- Filter collections in handlers (e.g., remove already-selected exercises)
- Transform enums to display-friendly structures with labels
- Compute derived values and format data before template rendering
- Create maps for lookups to avoid complex template logic

Example:
```go
// Transform enum to options with labels in handler
Difficulties: []difficultyOption{
    {Value: difficultyTooEasy, Label: "Too easy"},
    {Value: difficultyICouldDoMore, Label: "I could do more"},
    // ...
}
```

## Template Rendering

### Using app.render()
- Use `app.render(w, r, statusCode, "template-name", data)` for all template rendering
- Template name corresponds to folder in `/ui/templates/pages/{template-name}/`
- Always provide appropriate HTTP status code (200, 404, etc.)
- Pass structured data, not maps or raw values

## Form Handling

### Form Processing Pattern
```go
// Parse form data
if err = r.ParseForm(); err != nil {
    app.serverError(w, r, fmt.Errorf("parse form: %w", err))
    return
}

// Extract form fields
fieldValue := r.PostForm.Get("field_name")
if fieldValue == "" {
    app.serverError(w, r, errors.New("field not provided"))
    return
}
```

### Validation and Conversion
- Validate required form fields immediately after parsing
- Use `strconv` functions for type conversion with proper error handling
- Call service layer methods for business logic validation

## Error Handling

### Error Response Patterns
- Use `app.serverError(w, r, err)` for 500-level errors (logs and renders error page)
- Use `http.NotFound(w, r)` for 404 errors
- Use specific HTTP status codes when rendering templates (404 for not found pages)
- Always wrap errors with context using `fmt.Errorf("operation description: %w", err)`

### Service Layer Error Handling
- Check for specific business errors using `errors.Is(err, workout.ErrNotFound)`
- Handle business errors with appropriate user-facing responses
- Let service layer handle business validation, handlers handle HTTP concerns

## Redirects and Navigation

### Using redirect() Helper
- Use `redirect(w, r, "/path")` function for all redirects
- Redirect after successful POST operations (POST-redirect-GET pattern)
- Use appropriate redirect paths that match routing patterns

## Testing with e2etest

### End-to-End Testing Pattern
```go
server, err := e2etest.StartServer(ctx, testhelpers.NewWriter(t), testLookupEnv, run)
if err != nil {
    t.Fatalf("Failed to start server: %v", err)
}
client := server.Client()
```

### DOM Testing with goquery
- Use specific selectors that are resilient to UI changes
- Look for unique identifiers like button text, headings, or CSS classes
- Use `.FilterFunction()` for complex selection logic instead of generic selectors
- Test both success and error scenarios with appropriate HTTP status codes

Example:
```go
// Good: specific, resilient selector
form := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
    return s.Find("button:contains('Create Workout')").Length() > 0
}).First()

// Good: checking for specific content
if doc.Find("h1:contains('Add Exercise')").Length() == 0 {
    t.Error("Expected to find 'Add Exercise' heading")
}
```

### Form Testing Patterns
- Use `client.SubmitForm(ctx, doc, "/path", formData)` for form submissions
- Test form validation by submitting invalid data
- Verify redirects after successful form submissions
- Check that dynamic content updates correctly (e.g., exercise counts)

## Service Layer Integration

### Calling Service Methods
- All business logic goes through service layer (`app.workoutService`, etc.)
- Pass request context to service methods: `app.workoutService.Method(r.Context(), params)`
- Handle service errors appropriately (business errors vs system errors)
- Don't implement business logic in handlers - delegate to services

### Context Propagation
- Always pass `r.Context()` to service layer methods
- Use context for cancellation, timeouts, and request-scoped values
- Don't create new contexts in handlers unless specifically needed

## Routing and URL Patterns

### URL Structure
- Use RESTful patterns where appropriate
- Date parameters use format: `/workouts/{date}` (YYYY-MM-DD format)
- Nested resources: `/workouts/{date}/exercises/{exerciseID}`
- Action endpoints: `/workouts/{date}/complete`, `/workouts/{date}/start`

### Route Registration
- Group related routes together in routes.go
- Use descriptive route patterns that map to handler names
- Include HTTP method constraints in routing configuration