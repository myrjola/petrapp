# Web Layer — HTTP Handlers & Routing

Reference for HTTP handlers, routing, middleware, error handling, navigation,
and testing in `cmd/petra/`.

## What lives here

- **HTTP handlers** — methods on the `application` struct — plus routing
  (`routes.go`) and middleware.
- **Per-template data structs** embedding `BaseTemplateData`, and the
  handler-side data transformation that feeds them.
- **Request/response concerns** — form parsing, CSRF, sessions, flash
  plumbing, redirects, and the error→HTTP mapping (`serverError` / `userError`).
- **Component dot structs** (`components.go`).
- **The app's UI** — templates and static assets under `cmd/petra/ui/`
  (see the [templates README](ui/templates/README.md) and
  [design system](ui/design-system.md)).

## What does NOT live here

- Business rules and aggregate methods — `internal/petra/domain/`.
- Cross-aggregate orchestration and external integrations — `internal/petra/service/`.
- SQL queries and persistence — `internal/petra/repository/`.
- Template markup and CSS — `cmd/petra/ui/templates/`.

## Runtime layout

- `cmd/petra/ui` (templates and static assets) is baked into the binary with
  `//go:embed` (see `assets.go`), so production is a single self-contained
  executable — no `ui/` directory on disk, no `PETRAPP_TEMPLATE_PATH`. In dev
  (`FLY_APP_NAME` unset) the same files are read live from disk via `os.DirFS`,
  so editing a template or asset and refreshing is the whole dev loop — the
  embedded copy is ignored. `uiFilesystems` picks the source per mode.
- Static assets are content-fingerprinted at startup, not by a build-time
  `sed`. `buildAssetManifest` walks `ui/static`, SHA-256s each file, and the
  `asset` template func emits hashed URLs (`{{ asset "/main.css" }}` →
  `/main.<hash>.css`). `manifest.json` is processed in memory so its icon srcs
  are hashed too; `manifest.json`'s own hash reflects the rewritten bytes.
  `sw.js` and `robots.txt` stay at their plain well-known URLs.
- The file server (`handler-fileserver.go`) strips the hash off a request path
  and serves the current bytes, so a stale hashed URL never 404s. Caching keys
  on the path shape: an exact current-hash match is `immutable`; a plain or
  stale path is `must-revalidate` (so non-fingerprinted files can't go stale for
  a year); dev is always `no-store`, which is why hot reload survives even
  though the emitted URL still carries a (startup) hash.

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

Handlers are also the **register seam**: domain code speaks the training
literature's terms, UI copy speaks plain gym English (see "UI register" in
the root [CONTEXT.md](../../CONTEXT.md)). Label maps like the difficulty
options below are where that translation happens — keep domain identifiers
out of user-facing strings.

- Filter collections in handlers (e.g., remove already-selected exercises)
- Transform enums to display-friendly structures with labels
- Compute derived values and format data before template rendering
- Create maps for lookups to avoid complex template logic
- **Don't recompute domain rules.** Handlers may format primitives and shape data, but any value that depends on multiple domain fields must come from a method on the domain type. If you find yourself writing `if exercise.X && session.Y { ... }` in a handler, move it to `internal/petra/domain/`.

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
- Template name corresponds to folder in `cmd/petra/ui/templates/pages/{template-name}/`
- Always provide appropriate HTTP status code (200, 404, etc.)
- Pass structured data, not maps or raw values

## Form Handling

### Form Processing Pattern

Parse, build a domain value, hand it to the service. Validation lives on the
domain type (`Exercise.Validate()` and friends). A form whose fields each need
their own message returns a `domain.FieldErrors` (keyed by HTML field name); a
single page-level message returns a `domain.ValidationError` — see "User-facing
validation errors" below for the plumbing. The handler hands the service error
to `userError`, which routes `FieldErrors` and `ValidationError` to the form and
everything else to `serverError`; the handler never inspects the error type
itself.

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
  app.userError(w, r, err, editPath)
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
| `app.userError(w, r, err, safeURL)` | Any failure on an in-flight POST | `FieldErrors` → stash per-field messages + submitted values, `redirect(safeURL)`; `ValidationError` → `putFlashError(ve.Message)` + `redirect(safeURL)`; anything else → delegates to `serverError` |
| `app.serverError(w, r, err)` | Catastrophic failures: panics, template render errors, escape hatch when no safe URL fits | Logs. On a shim POST navigates to `/error?from=<sanitised>`; on GET / non-shim renders `error.gohtml` with 500 |

#### `serverError` on POST navigates to `/error`

The stack-navigator JS shim (`cmd/petra/ui/static/main.js`) intercepts every form
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

`userError` routes by error type (first match wins):

- `errors.As(err, &fe)` matching `domain.FieldErrors` → stash the per-field
  messages **and** the submitted form values (`r.PostForm`, populated by
  `parseForm` before the service call) via `app.putFormError(...)`, then
  `redirect(safeURL)`. The form's GET handler pops them with
  `app.popFormError(...)` and re-renders each control with its own `Error`,
  the user's submitted `Value` (so edits survive the bounce), and an
  `error-summary` component listing every error as a link to its field. This is
  the field-level path — use it for forms whose fields each need a message.
- `errors.As(err, &ve)` matching `domain.ValidationError` → flash with
  `ve.Message` verbatim and `redirect(safeURL)`. The form's GET handler
  pops the flash with `app.popFlash(...)`, reads the `.Message` field,
  and renders the `banner` component. This is the page-level path — one
  message, no field anchoring (service business errors use it).
- Anything else → delegate to `serverError`. The non-validation case
  produces the catastrophic-failure UX (shim navigation to `/error`,
  or inline 500 for non-shim clients). `userError` does not
  flash a generic "Couldn't complete that action" message any more —
  banner UX is reserved for failures the user can act on by adjusting
  their input.

On a `FieldErrors` bounce render the **error-summary** (which is `Live`,
focus-on-load) and leave the page-top `banner` empty; on a `ValidationError`
bounce render the `banner` (`Live`) and an empty summary. Exactly one element
takes focus per load, so they never contend — see "Banner announcement & focus".

`safeURL` is consulted on both validation branches. It must point at a GET
handler known to render successfully AND that pops + renders the flash banner
(and, for field errors, the form-error payload). See the requirements below.

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

#### `userError` vs a handler-local guard

There is exactly one way to route a *service* error: pass it to `userError`.
Do not hand-roll the `errors.As(&fe / &ve) { … ; redirect } / serverError`
block inline — that is what `userError` is. The only `errors.As` for
`FieldErrors` / `ValidationError` in the package lives inside `userError` itself.

A handler-local guard is different and stays inline: when the *handler* rejects
input before any service call (e.g. `prefs.IsEmpty()` in `scheduleGET`/
`preferencesGET`), there is no error value to route, so it flashes the fixed
message directly with `putFlashError` (or `putFlashErrorWithAnchor` for a
panel-anchored banner, which `userError` does not express) and redirects. Reach
for `userError` only when a service/domain call returned an `error`.

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
  verbatim. Example: `internal/petra/service/exercises.go:AddExercise` returns
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
- `auth.AuthenticateMiddleware` → injected
  `InternalErrorHandler` (wired to `app.serverError` in `main.go`).
  DB lookup failures land on `/error` instead of producing a silent
  500.

The injection point on `auth` keeps the package unaware of
the stack-navigator wire protocol. If you add another middleware in
that package, route its internal errors through `h.internalError` and
they will inherit the same UX.

### Client-side error surface (`#js-flash`)

`#js-flash` is the JS shim's last-resort surface for client-only `fetch`
failures (offline / DNS / CORS) — **not** for server-originating messages.
Any server response (2xx or not) drives navigation or reload through the wire
protocol, so the `userError` → flash → redirect path stays canonical for
server messages. Markup, accessibility, and Trusted-Types detail live in
[`ui/templates/README.md`](ui/templates/README.md) under "Client-only
error surface".

### Banner announcement & focus (`BannerData.Live`)

The server-rendered flash path is a *full document load* (flash → redirect →
the shim's same-URL `location.replace`). A live region (`role="alert"` /
`role="status"`) that is already present in a freshly-parsed document is **not**
announced by screen readers — assistive tech only narrates changes made *after*
the region is registered. So a server banner that just sits in the initial HTML
is silent.

`BannerData.Live` opts a banner into a small nonce'd enhancement in the `banner`
component that fixes this on load:

- **error** → the banner is `tabindex="-1"` and receives `focus()`, which both
  announces the message (assertive) and scrolls it into view. This is the
  GOV.UK error-summary focus-on-load pattern.
- **success / info** → the text is re-asserted into the `role="status"` region
  (cleared, then set on the next frame) so it announces, *without* stealing
  focus — confirmations don't demand action.

Set `Live: true` on banners built from a popped session flash (a real
just-happened action). Leave it `false` for static reference galleries
(styleguide, the `/dev/error-ux` variant catalog) — multiple live banners on one
page would fight over focus. The `#js-flash` client path needs none of this: its
region ships empty and is populated later, so the mutation announces naturally
(it only adds `scrollIntoView` for sighted users). Exercise every path live at
`/dev/error-ux` (dev mode).

## Redirects and Navigation

Why the app is an MPA with a navigation shim at all — rather than an SPA or
htmx-style partial swaps — is recorded in
[ADR 0002](../../docs/adr/0002-stack-navigator-mpa-enhancement.md).

### The stack-navigator wire protocol

The JS shim (`ui/static/main.js`, gated on `'navigation' in window`)
intercepts form POSTs and replays them via `fetch`. Server and client speak a
small header protocol; without JS (or without the Navigation API) forms submit
natively and the server falls back to plain POST-Redirect-GET 303s.

| Direction | Header | Required | Meaning |
|---|---|---|---|
| Request | `X-Requested-With: stacknav` | set by the shim's POST fetch | This POST follows the header protocol; server replies `200` instead of `303` |
| Response | `X-Location: <path>` | yes, on 200 | Where the client navigates — both successes and validation errors (flash + redirect-to-form) |
| Response | `X-Replace-Url: true` | optional | Replace the current history entry instead of pop-or-push (the form page is disposable) |

Any non-200 response makes the shim `location.reload()` to surface state.
The client's history strategy is **pop-or-push**: same-URL targets (and
`X-Replace-Url`) replace in place; otherwise it walks the backward history
stack for a URL match and traverses to the most recent one; otherwise it
pushes a new entry. Validation errors cannot render in place because the CSP
(`require-trusted-types-for 'script'`) blocks string-to-HTML injection — hence
flash + redirect-to-form.

### Using redirect() and redirectReplace()

Two helpers cover all redirect needs. Both negotiate the wire protocol when the request carries `X-Requested-With: stacknav`, and fall through to a plain 303 See Other otherwise. Non-POST callers transparently use the 303 path because they don't carry the header.

- **`redirect(w, r, "/path")`** — default. Use for almost all redirects: POST results, GET-handler bounces, auth gates, validation re-renders via flash + redirect-to-form. The client behavior is "pop-or-push": traverse to the URL if it's already in the backward history stack, otherwise push a new entry. Same-URL submits (target equals the current URL — set updates, warmup completion, validation errors that re-render the form) are auto-detected by the client and become a replace; the helper itself stays simple.
- **`redirectReplace(w, r, "/path")`** — opt-in. Use when the originating page should be erased from history on submit. Today's only call site is `workoutAddExercisePOST`, which redirects to the new exercise's detail page and replaces `/add-exercise`. Reach for this when the form page only exists to submit (a picker, an editor that disappears on save) and you don't want it left behind in the back-button stack.

Redirect paths may include a `#fragment` to land the user at a specific
section after a form submit (e.g. `redirect(w, r, "/preferences#deload-title")`).
The JS shim's `popOrPushTo` replace branch guarantees a real cross-
document fetch via a `bf_inv` cache-bust query param it injects, then
strips after parse — so any flash banner the handler sets is rendered
on the next GET and native scroll-to-anchor fires on the new document.
Handlers don't think about same-document semantics; emit the redirect
with the fragment you want and the shim does the rest.

## Testing with e2etest

The `e2etest` package (`internal/e2etest/`) provides a real HTTP server bound to a random port plus a `Client` with cookie jar and form helpers. Handler tests use it instead of `httptest` so session middleware, CSRF, and the full middleware stack are exercised.

### End-to-End Testing Pattern

```go
server, err := e2etest.StartServer(ctx, testkit.NewWriter(t), testLookupEnv, run)
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

`cmd/petra/playwright_test.go` drives real Chromium against a live server to
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
at `/`. Petra is a mobile-first PWA — don't introduce desktop
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
class rename in `cmd/petra/ui/templates/` shouldn't break tests.

### Helpers for repeated flows

Common flows live as helpers at the bottom of `playwright_test.go`:

- `registerAndWaitSchedule(t, page, serverURL)` — register a new user, land on `/schedule`.
- `selectAndSubmitSchedule(t, page, serverURL, days)` — fill the given weekdays for 1 hour, submit, wait for `/`.
- `todayAndTomorrowWeekdays()` / `allWeekdays()` — day-name helpers for the schedule form.
- `addWeightedExerciseToWorkout(t, page, workoutURL)` — fallback when the workout has no weighted/assisted exercise (either zero exercises, or only bodyweight/time_based were scheduled). Picks via the picker card's `data-exercise-type` attribute.
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
PWDEBUG=1 go test -count 1 -v -run Test_playwright_smoketest ./cmd/petra/
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
