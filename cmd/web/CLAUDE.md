# Web Layer Guidelines - HTTP Handlers & Routing

Guidelines for working with HTTP handlers, routing, middleware, and web server components in `cmd/web/`.

## What lives here

- **HTTP handlers** — methods on the `application` struct — plus routing
  (`routes.go`) and middleware.
- **Per-template data structs** embedding `BaseTemplateData`, and the
  handler-side data transformation that feeds them.
- **Request/response concerns** — form parsing, CSRF, sessions, flash
  plumbing, redirects, and the error→HTTP mapping (`serverError` / `userError`).
- **Component dot structs** (`components.go`).

## What does NOT live here

- Business rules and aggregate methods — `internal/domain/`.
- Cross-aggregate orchestration and external integrations — `internal/service/`.
- SQL queries and persistence — `internal/repository/`.
- Template markup and CSS — `ui/templates/`.

## Handler Structure

### Handler Method Pattern

- Handlers are methods on the `application` struct:
  `func (app *application) handlerName(w http.ResponseWriter, r *http.Request)`
- All dependencies are available through the `app` struct (services, templates, logger, etc.)
- Use descriptive handler names that indicate HTTP method: `handlerGET`, `handlerPOST`

### URL Parameter Extraction

- Use `r.PathValue("paramName")` to extract URL path parameters
- Always validate extracted parameters (dates, IDs) with proper error handling
- Return `http.NotFound(w, r)` for invalid parameters

### Template Data Structures

- Create dedicated structs for each template that embed `BaseTemplateData`
- Transform all data in handlers before passing to templates - avoid complex template logic
- Use `newBaseTemplateData(r)` to populate the embed from request context

`BaseTemplateData` (defined in `templates.go`) exposes two fields that every page template can read:

- `Authenticated bool` — set from `contexthelpers.IsAuthenticated(r.Context())`
- `IsAdmin bool` — set from `contexthelpers.IsAdmin(r.Context())`

The CSP nonce and CSRF plumbing do **not** travel on this struct — the nonce is injected as a template function (`{{ nonce }}`) via `contextTemplateFuncs` in `handlers.go`, and CSRF is handled in middleware. Anything that renders outside `app.render(...)` needs to wire those in explicitly.

Example:

```go
type workoutTemplateData struct {
  BaseTemplateData
  Date    time.Time
  Session domain.Session
}
```

## Data Transformation Patterns

### Prefer Handler-Side Processing

- Filter collections in handlers (e.g., remove already-selected exercises)
- Transform enums to display-friendly structures with labels
- Compute derived values and format data before template rendering
- Create maps for lookups to avoid complex template logic
- **Don't recompute domain rules.** Handlers may format primitives and shape data, but any value that depends on multiple domain fields must come from a method on the domain type. If you find yourself writing `if exercise.X && session.Y { ... }` in a handler, move it to `internal/domain/`.

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

Parse, build a domain value, hand it to the service. Validation lives on the
domain type (`Exercise.Validate()` and friends) and surfaces as a
`domain.ValidationError` — see "User-facing validation errors" below for the
flash/redirect plumbing. The handler's only job is to detect `ValidationError`
with `errors.As` and route it to the form; everything else is a
`serverError`.

```go
if err = r.ParseForm(); err != nil {
  app.serverError(w, r, fmt.Errorf("parse form: %w", err))
  return
}

exercise := domain.Exercise{
  ID:       id,
  Name:     r.PostForm.Get("name"),
  Category: domain.Category(r.PostForm.Get("category")),
  // ...
}

editPath := fmt.Sprintf("/admin/exercises/%d", id)
if err = app.service.UpdateExercise(r.Context(), exercise); err != nil {
  var ve domain.ValidationError
  if errors.As(err, &ve) {
    app.putFlashError(r.Context(), ve.Message)
    redirect(w, r, editPath)
    return
  }
  app.serverError(w, r, err)
  return
}

redirect(w, r, "/admin/exercises")
```

`r.PostForm.Get` returns `""` for missing fields — let the domain's
`Validate()` reject empties, don't pre-check with `serverError`. Use
`strconv` (or `optionalInt`-style helpers) for numeric conversions; reach
for `serverError` only on parser/IO failures the user cannot fix.

## Error Handling

### Three terminal calls

Every POST handler ends in exactly one of:

| Call | When | Effect |
|---|---|---|
| `redirect(w, r, url)` | Success | 200 + `X-Location` (stacknav) or 303 (plain); client navigates |
| `app.userError(w, r, err, safeURL)` | Any user-visible failure on an in-flight action | Routes by error type, calls `putFlashError`, then `redirect(w, r, safeURL)` |
| `app.serverError(w, r, err)` | True full-page failure (template render, broken session, no safe URL exists) | Logs and renders `error.gohtml` 500. Rare. |

`userError` is the single helper for *both* `domain.ValidationError` and unexpected
system errors on inline actions. It dispatches on the error type:

- `var ve domain.ValidationError; errors.As(err, &ve)` → flash with `ve.Message` verbatim.
- Otherwise → log the underlying error at ERROR and flash a generic message.

Then it writes the flash and redirects to `safeURL`. The form's GET handler
pops the flash with `app.popFlashError(...)` and renders the `banner`
component as today — see the worked example in `workoutGET` /
`workoutAddExercisePOST`.

#### `safeURL` is mandatory, must pop + render the flash

The call site must pass a URL that is known to render successfully AND whose
handler pops + renders the flash banner. Today that means: `/workouts/{date}`
(both success and not-found branches), `/schedule`, `/admin/exercises`,
`/admin/exercises/{id}`. If you need a new target, plumb a `Flash BannerData`
field through its template data struct, render `{{ template "banner" .Flash }}`
in the template, and pop with `app.popFlashError(r.Context())` in the handler.

**Do not** default `safeURL` to `r.Referer()` (unreliable on direct POSTs,
easily forged) or to the request URL (wrong for action endpoints like
`POST /workouts/{date}/start`, which is POST-only and would 405 on a GET).
Pointing `safeURL` at an action endpoint or another broken handler will
produce a redirect loop.

#### Existing handlers may still use inline `errors.As(&ve)` boilerplate

> Go-forward convention for new and migrating handlers. Existing handlers
> predate `userError` and may still use the inline `errors.As(&ve) {
> putFlashError(ve.Message); redirect(formURL) }` pattern — that's fine,
> functionally equivalent, and they migrate opportunistically when next
> touched. Don't expect every form handler to call `userError` today.

#### Other patterns

- `http.NotFound(w, r)` (or `app.notFound(w, r)`) for 404s. Path-param
  parsers like `parseDateParam` call `notFound` for you on parse failure.
- `app.render(w, r, http.StatusNotFound, "workout-not-found", data)` for
  domain-level "no such resource" pages that want richer copy than the
  generic 404. These pages also pop + render the flash so `userError`
  redirects to them surface the message.

### Service Layer Error Handling

- Check for specific business errors using `errors.Is(err, domain.ErrNotFound)`.
- For user-actionable business failures, return a `domain.ValidationError`
  from the service / domain layer so `userError` can surface the message
  verbatim. Example: `internal/service/exercises.go:AddExercise` returns
  `domain.ValidationError{Message: "This day has no planned workout..."}`
  on the missing-session branch.
- For system faults the user cannot fix, return the wrapped underlying
  error and let `userError` log it and show the generic message.
- Let the service layer handle business validation; handlers handle HTTP
  concerns.

### Client-side error surface (`#js-flash`)

`#js-flash` is the JS shim's last-resort surface for client-only `fetch`
failures (offline / DNS / CORS) — **not** for server-originating messages.
Any server response (2xx or not) drives navigation or reload through the wire
protocol, so the `userError` → flash → redirect path stays canonical for
server messages. Markup, accessibility, and Trusted-Types detail live in
[`ui/templates/CLAUDE.md`](../../ui/templates/CLAUDE.md) under "Client-only
error surface".

## Redirects and Navigation

### Using redirect() and redirectReplace()

Two helpers cover all redirect needs. Both negotiate the stack-navigator wire protocol when the request carries `X-Requested-With: stacknav` (set by the JS shim's POST fetch), and fall through to a plain 303 See Other otherwise. Non-POST callers transparently use the 303 path because they don't carry the header.

- **`redirect(w, r, "/path")`** — default. Use for almost all redirects: POST results, GET-handler bounces, auth gates, validation re-renders via flash + redirect-to-form. The client behavior is "pop-or-push": traverse to the URL if it's already in the backward history stack, otherwise push a new entry. Same-URL submits (target equals the current URL — set updates, warmup completion, validation errors that re-render the form) are auto-detected by the client and become a replace; the helper itself stays simple.
- **`redirectReplace(w, r, "/path")`** — opt-in. Use when the originating page should be erased from history on submit. Today's only call site is `workoutAddExercisePOST`, which redirects to the new exercise's detail page and replaces `/add-exercise`. Reach for this when the form page only exists to submit (a picker, an editor that disappears on save) and you don't want it left behind in the back-button stack.

The client treats every 200 response with `X-Location` as a navigation; an additional `X-Replace-Url: true` header (set by `redirectReplace`) flips the strategy from pop-or-push to replace.

See `docs/superpowers/specs/2026-05-03-stack-navigator-push-default-design.md` for the wire protocol, per-flow behavior, and rationale.

## Testing with e2etest

The `e2etest` package (`internal/e2etest/`) provides a real HTTP server bound to a random port plus a `Client` with cookie jar and form helpers. Handler tests use it instead of `httptest` so session middleware, CSRF, and the full middleware stack are exercised.

### End-to-End Testing Pattern

```go
server, err := e2etest.StartServer(ctx, testhelpers.NewWriter(t), testLookupEnv, run)
if err != nil {
  t.Fatalf("Failed to start server: %v", err)
}
client := server.Client()
```

Key entry points:

- `e2etest.StartServer(ctx, out, lookupEnv, runFn) (*Server, error)` — `server.go`
- `(*Client).SubmitForm(ctx, doc, actionPath, fields) (*goquery.Document, error)` — `client.go`; resolves the form by `action`, fills matching labels/fields, submits, returns the parsed response

### DOM Testing with goquery

- Use specific selectors that are resilient to UI changes
- Look for unique identifiers like button text, headings, or CSS classes
- Use `.FilterFunction()` for complex selection logic instead of generic selectors
- Test both success and error scenarios with appropriate HTTP status codes

Example:

```go
// Good: specific, resilient selector
form := doc.Find("form").FilterFunction(func (_ int, s *goquery.Selection) bool {
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

- All business logic goes through service layer (`app.service`, etc.)
- Pass request context to service methods: `app.service.Method(r.Context(), params)`
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

- Routing uses the stdlib `http.ServeMux` with Go 1.22+ method+pattern syntax (e.g. `mux.Handle("GET /workouts/{date}", ...)`). No third-party router.
- Path parameters are read with `r.PathValue("name")` inside the handler
- Group related routes together in `routes.go`
- Use descriptive route patterns that map to handler names
- Include HTTP method constraints in the pattern (`GET`, `POST`, etc.) — ServeMux returns 405 automatically for mismatches
