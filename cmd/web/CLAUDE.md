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

`BaseTemplateData` (defined in `templates.go`) exposes four fields that every page template can read:

- `Authenticated bool` — set from `contexthelpers.IsAuthenticated(r.Context())`
- `IsAdmin bool` — set from `contexthelpers.IsAdmin(r.Context())`
- `InvalidationToken string` — set from the `inv_bfcache` cookie
- `Nonce template.HTMLAttr` — the CSP nonce, pre-formatted as `nonce="<value>"`. Reference as `{{ $.Nonce }}` in page templates and `{{ .Nonce }}` in components (whose dot structs each carry their own `Nonce` field).

CSRF protection is handled in middleware and does not appear on this struct.

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
| `app.userError(w, r, err, safeURL)` | Any failure on an in-flight POST | `ValidationError` → `putFlashError(ve.Message)` + `redirect(safeURL)`; anything else → delegates to `serverError` |
| `app.serverError(w, r, err)` | Catastrophic failures: panics, template render errors, escape hatch when no safe URL fits | Logs. On a shim POST navigates to `/error?from=<sanitised>`; on GET / non-shim renders `error.gohtml` with 500 |

#### `serverError` on POST navigates to `/error`

The stack-navigator JS shim (`ui/static/main.js`) intercepts every form
POST, replays it via `fetch`, and on any non-200 response calls
`location.reload()` — CSP / Trusted Types block injecting the response
body in place. Historically that meant `serverError` on a POST produced
a silent failure: reload landed on the form page with no flash, no
banner.

Today `serverError` detects the `X-Requested-With: stacknav` header and,
on the shim path, replies `200 + X-Location: /error[?from=…]` instead
of a 500 body. The shim navigates to `/error`, which renders a
catastrophic-failure page with a "← Back" link (when the originating
path was same-origin) and Go Home. On a curl / no-JS request the same
helper falls through to the inline 500 + `error.gohtml` body — the
browser renders it directly.

Use `serverError` when there is no safe URL to flash + redirect to:
panics caught by `recoverPanic`, template render failures, and the
escape hatches inside helpers like `parseForm`. Use `userError` for
in-flight POST failures where validation needs a banner on a known-good
GET handler — it routes the validation case to the banner and
delegates everything else back to `serverError`.

#### `userError` semantics

`userError` routes by error type:

- `errors.As(err, &ve)` matching `domain.ValidationError` → flash with
  `ve.Message` verbatim and `redirect(safeURL)`. The form's GET handler
  pops the flash with `app.popFlash(...)`, reads the `.Message` field,
  and renders the `banner` component.
- Anything else → delegate to `serverError`. The non-validation case
  produces the catastrophic-failure UX (shim navigation to `/error`,
  or inline 500 for non-shim clients). `userError` does not
  flash a generic "Couldn't complete that action" message any more —
  banner UX is reserved for failures the user can act on by adjusting
  their input.

`safeURL` is only consulted on the validation branch. It must point at
a GET handler known to render successfully AND that pops + renders the
flash banner. See the `safeURL` requirements below.

#### `safeURL` is mandatory, must pop + render the flash

The call site must pass a URL that is known to render successfully AND whose
handler pops + renders the flash banner. Today that means: `/workouts/{date}`
(both success and not-found branches), `/schedule`, `/admin/exercises`,
`/admin/exercises/{id}`. If you need a new target, plumb a `Flash BannerData`
field through its template data struct, render `{{ template "banner" .Flash }}`
in the template, and pop with `app.popFlash(r.Context())` (reading the
`.Message` field) in the handler.

**Do not** default `safeURL` to `r.Referer()` (unreliable on direct POSTs,
easily forged) or to the request URL (wrong for action endpoints like
`POST /workouts/{date}/start`, which is POST-only and would 405 on a GET).
Pointing `safeURL` at an action endpoint or another broken handler will
produce a redirect loop.

#### Existing handlers may still use inline `errors.As(&ve)` boilerplate

> Go-forward convention for new and migrating handlers. Existing
> handlers predate `userError` and may still use the inline
> `errors.As(&ve) { putFlashError(ve.Message); redirect(formURL) }`
> + trailing `app.serverError(w, r, err)` pattern. Under the
> shim-aware `serverError`, that inline pattern is now
> *functionally equivalent* to calling `userError` — both route
> `ValidationError` to the banner and non-validation to the
> catastrophic-failure page. Migrate opportunistically when next
> touching the handler; there is no UX gap remaining.

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

### Middleware error paths

Three middleware paths used to silently 500 (or 401) on the JS-shim
path. They now route through the shim-aware helpers:

- `recoverPanic` → `app.serverError`. Panics navigate to `/error` on
  shim POSTs and render the 500 inline otherwise.
- `mustAdmin` → `redirect(w, r, "/")`. Non-admins are bounced to home;
  the 401 status code has been retired.
- `webauthnhandler.AuthenticateMiddleware` → injected
  `InternalErrorHandler` (wired to `app.serverError` in `main.go`).
  DB lookup failures land on `/error` instead of producing a silent
  500.

The injection point on `webauthnhandler` keeps the package unaware of
the stack-navigator wire protocol. If you add another middleware in
that package, route its internal errors through `h.internalError` and
they will inherit the same UX.

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

Redirect paths may include a `#fragment` to land the user at a specific
section after a form submit (e.g. `redirect(w, r, "/preferences#deload-title")`).
The JS shim's `popOrPushTo` replace branch guarantees a real cross-
document fetch via a `bf_inv` cache-bust query param it injects, then
strips after parse — so any flash banner the handler sets is rendered
on the next GET and native scroll-to-anchor fires on the new document.
Handlers don't think about same-document semantics; emit the redirect
with the fragment you want and the shim does the rest. See
`docs/superpowers/specs/2026-05-26-stack-navigator-replace-force-fresh-design.md`.

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

## End-to-end testing with Playwright

`cmd/web/playwright_test.go` drives real Chromium against a live server to
cover flows that exercise the JS shim — stack-navigator behavior, bfcache
staleness, WebAuthn auth. The Go binding is
`github.com/playwright-community/playwright-go`. Tests skip under
`testing.Short()` (so `go test -short ./...` stays fast) and run in
parallel under `make ci`.

### Shared setup

`setupPlaywrightPage(t, allowedConsoleErrors...)` installs Chromium once
(serialized by `sync.OnceValue` to avoid ETXTBSY in parallel runs), boots
the app server via `e2etest.StartServer`, launches a browser context at a
390x844 mobile viewport with `prefers-reduced-motion: reduce` emulated,
wires a virtual WebAuthn authenticator via CDP, and returns a page parked
at `/`. Petrapp is a mobile-first PWA — don't introduce desktop
dimensions without a reason that's worth documenting in the test.

The CSS honors `prefers-reduced-motion`: view transitions collapse to
0.001ms, transforms drop, the loading-bar animation stops. Playwright's
actionability checks wait on those transitions; emulating reduced motion
keeps tests fast and stable. Don't override it unless you're specifically
testing motion-sensitive behavior.

### Selectors: role > data attribute > CSS class

Prefer accessibility-API locators that match how a user finds the element:

```go
page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Start Tracking"})
page.GetByLabel("Monday")
page.GetByText("Recovery settings saved.")
```

`Name` is a case-insensitive substring against the accessible name — so
a button with `aria-label="Too heavy — failed to reach target reps"`
matches `Name: "Too heavy"`. Pass a `*regexp.Regexp` for finer control.

Reach for data attributes (`a[data-back-button]`, `a[data-workout-exercise-id]`)
when a stable contract attribute exists *specifically* as a test/JS hook —
those won't move under a styling refactor. Use bare CSS classes
(`button.too-heavy-btn`, `a.add-exercise-link`) only as a last resort; a
class rename in `ui/templates/` shouldn't break tests.

### Helpers for repeated flows

Common flows live as helpers at the bottom of `playwright_test.go`:

- `registerAndWaitSchedule(t, page, serverURL)` — register a new user, land on `/schedule`.
- `selectAndSubmitSchedule(t, page, serverURL, days)` — fill the given weekdays for 1 hour, submit, wait for `/`.
- `todayAndTomorrowWeekdays()` / `allWeekdays()` — day-name helpers for the schedule form.
- `addExerciseToWorkout(t, page, workoutURL)` — fallback when the weekly planner exhausts its exercise pool.
- `dumpNavDiagnostics(t, page, where, wantURL)` — log Navigation API state for URL-mismatch fatals.

Add a helper when the third caller appears — earlier than that is
premature.

### Console errors fail the test

`setupPlaywrightPage` collects browser console errors and fails the test
in a `t.Cleanup` if any aren't allowlisted. Pass known-benign substrings
as variadic args:

```go
page, serverURL := setupPlaywrightPage(t, "ResizeObserver loop")
```

If a test starts failing on a console error you didn't introduce, fix the
underlying JS bug rather than expanding the allowlist. Uncaught JS
exceptions surface separately through `pageerror` and always fail.

### Failure screenshots

When a test fails, `setupPlaywrightPage`'s cleanup writes a full-page PNG
to `os.TempDir()` and logs the path:

```
playwright_test.go:136: failure screenshot: /tmp/playwright-failure-Test_playwright_smoketest-20260527-204631.png
```

The file persists past the test run. In CI, surface `os.TempDir()` as an
artifact upload path to capture these automatically.

### Web-first assertions for state that needs retry

`playwright.NewPlaywrightAssertions()` returns matchers that auto-retry
against a deadline — use them for anything that becomes true after a
navigation, fetch, or DOM update settles. Compare these two forms:

```go
// Brittle: snapshot read, no retry. Flakes if the attribute clears
// after the next paint.
busy, _ := btn.GetAttribute("aria-busy")
if busy == "true" { t.Errorf("...") }

// Web-first: retries until the timeout. Survives any brief moment
// the busy flag is still in the act of clearing.
assertions := playwright.NewPlaywrightAssertions()
assertions.Locator(btn).Not().ToHaveAttribute("aria-busy", "true")
```

`ToHaveURL`, `ToBeVisible`, `ToBeInViewport`, `ToHaveText` follow the
same pattern. Imperative checks after a `WaitForURL` are fine — the
wait already settled the state — but anything that races with a JS
side-effect should go through web-first assertions.

### Same-URL navigations and DOM-settle waits

The stack-navigator auto-replaces same-URL submits, so `WaitForURL` can
return against a stale DOM (URL never changed). Wait on a post-submit
DOM marker instead:

```go
completedBefore, _ := completedSets.Count()
// ...submit set...
completedSets.Nth(completedBefore).WaitFor()   // new set rendered
```

The `Test_playwright_smoketest` set loop and `Test_playwright_stacknav`
Flow 1 both use this pattern; copy it whenever a redirect lands at the
same URL the form was submitted from.

### Debugging

Set `PWDEBUG=1` to run headed with no default timeouts and the
Playwright Inspector wired up; `page.Pause()` in a test will drop into
the inspector at that point.

```
PWDEBUG=1 go test -count 1 -v -run Test_playwright_smoketest ./cmd/web/
```

Run a single test for fast iteration. For URL-mismatch failures, prefer
`dumpNavDiagnostics(t, page, where, wantURL)` over ad-hoc logging — it
dumps the Navigation API entry stack, current load state, and current
heading in one line.

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
