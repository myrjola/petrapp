# Shim-Aware `serverError` for POST Failures

**Date:** 2026-05-24
**Status:** Draft
**Scope:** `app.serverError`, the JS stack-navigator shim's non-200 handling, and the dedicated `/error` page. Follow-up to [2026-05-18 Error UX Conventions](2026-05-18-error-ux-conventions-design.md).

## Problem

The 2026-05-18 error-UX convention introduced `app.userError` to surface
in-flight POST failures via flash + banner + redirect. It correctly covered
`domain.ValidationError` (class A) and unified the system-error path (class
C) into the same banner UX when handlers were migrated. Class C is still
described in that doc as "silent failure when handlers stay on `serverError`,"
with the assumption that opportunistic migration drains the gap over time.

The latent issue: even *with* full migration, `serverError` remains on the
table for handlers as the "true full-page failure" escape hatch, and it is
the **only** path `recoverPanic` and a handful of middleware errors can
take. On any of those paths, today's behavior is:

1. POST hits `serverError` → server returns 500 + `error.gohtml` body.
2. JS shim (`ui/static/main.js:213-227`) sees non-200 → calls `location.reload()`.
3. CSP / Trusted Types forbid the shim from rendering the response body in
   place, so the carefully-rendered `error.gohtml` is never seen.
4. Reload lands on the page the form lived on. That page's GET pops nothing
   (`serverError` didn't write the flash) — banner is empty.
5. User experience: button "did nothing." Same silent-fail bug as the
   pre-migration class-C case.

This affects:

- **`recoverPanic`** (`cmd/web/middleware.go:182`) — every uncaught handler
  panic.
- **`workoutCompletePOST`** and any other un-migrated handler that falls
  through to `serverError` on the non-`ValidationError` branch.
- The `render` helper itself (`cmd/web/handlers.go:154`) when a template
  fails to render after a POST.
- Indirectly: `webauthnhandler.AuthenticateMiddleware`
  (`internal/webauthnhandler/middleware.go:32,44`) and `mustAdmin`
  (`cmd/web/middleware.go:208`), both of which use `http.Error` and produce
  the same silent-reload UX.

There is also a missing affordance for the user even when the banner *does*
work: a "soft try-again on the same page" banner trivialises an unexpected
catastrophic failure. A panic and a `ValidationError` produce visually
identical UX, which misrepresents the failure class.

Pure-GET 500s are unaffected — the browser renders `error.gohtml` directly,
no shim involvement.

## Goal

Make `serverError` produce a visible, distinct UX on POST regardless of
whether the JS shim intercepted the response. Preserve the design split
between predictable (validation, in-context banner) and unexpected
(catastrophic, dedicated error page) failures. Stay inside the existing
wire protocol — no new headers, no new client signals.

## Non-goals

- Replaying the original POST automatically. Form data is not preserved
  across the error page navigation, and "retry by re-submission" interacts
  poorly with non-idempotent actions, CSP, and Trusted Types.
- A toast / snackbar surface. The banner remains the soft-failure surface;
  the error page remains the catastrophic-failure surface.
- Reworking `recoverPanic`'s recover-after-partial-write story.
  `recoverPanic` will benefit from the new shim-aware `serverError`
  transparently, but the underlying constraint (headers may already be
  written when the panic fires) is unchanged.
- A new wire protocol header. `X-Location` already exists and is sufficient.
- Migrating the `webauthnhandler` / `mustAdmin` `http.Error` paths in this
  pass. They get the same treatment in a follow-up once the helper shape
  is settled.

## Design

### `serverError` becomes shim-aware

```go
func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
    app.logger.LogAttrs(r.Context(), slog.LevelError, "server error", slog.Any("error", err))

    if r.Header.Get("X-Requested-With") == "stacknav" {
        // Drive the shim's "navigate on 200 + X-Location" path to /error.
        // The page the form lived on is preserved in ?from so the error page
        // can offer a "← Back" affordance with context.
        target := "/error"
        if from := r.Referer(); from != "" {
            if u, parseErr := url.Parse(from); parseErr == nil && u.Host == r.Host {
                target = "/error?from=" + url.QueryEscape(u.Path)
            }
        }
        w.Header().Set("X-Location", target)
        w.WriteHeader(http.StatusOK)
        return
    }

    // Non-JS clients (curl, no-JS browsers, direct GET 500s) keep the
    // current behavior: render error.gohtml inline with a 500 status.
    app.render(w, r, http.StatusInternalServerError, "error", nil)
}
```

`Referer` is used here *only as a UX hint for the back link* — not as a
trust boundary. The query parameter is rejected on the GET side if it
isn't a same-origin path beginning with `/`. The catastrophic-error UX is
correct even when `?from=` is missing.

### A real `GET /error` route

`app.errorGET` renders `error.gohtml` against `BaseTemplateData` plus an
optional sanitised `From` field. Returns 200 (the page itself is the
expected response — not a 500). Wired through `withoutMaintenanceModeStack`
so it works mid-maintenance and pre-auth alike — the error page must be
reachable from any state the rest of the app can land in.

```go
type errorTemplateData struct {
    BaseTemplateData
    From string // sanitised same-origin path, or "" if not provided / invalid
}

func (app *application) errorGET(w http.ResponseWriter, r *http.Request) {
    from := r.URL.Query().Get("from")
    if from == "" || !strings.HasPrefix(from, "/") || strings.Contains(from, "//") {
        from = ""
    }
    app.render(w, r, http.StatusOK, "error", errorTemplateData{
        BaseTemplateData: newBaseTemplateData(r),
        From:             from,
    })
}
```

Sanitisation rules:
- Must start with `/`.
- Must not contain `//` (defends against protocol-relative URLs like
  `//evil.example.com/foo`).
- Anything else falls back to empty `From`; the page still renders.

### `error.gohtml` retry-affordance change

Today the page has a Retry button that calls `location.reload()` and a Go
Home anchor. From `/error`, `location.reload()` just GETs `/error` again —
not a retry in any meaningful sense. Replace the Retry button with a
context-aware "← Back" link:

- When `From` is set: anchor pointing at `From` ("← Back to where you were").
- When `From` is empty: anchor omitted; Go Home is the only action.

This deliberately does **not** auto-resubmit the original POST. The user
chooses to retry by clicking the page button on the originating screen.
That preserves intent for non-idempotent actions (no surprise double-submit
of "Finish workout") and avoids the CSP / Trusted-Types complications of
replaying a POST from JS.

### `userError` narrows to predictable failures

Today `userError` flashes a generic message and redirects on any non-
`ValidationError`. With the shim-aware `serverError` in place, the
catastrophic path is owned by `serverError`. `userError` should delegate
the non-validation case to keep one source of truth for "this was
unexpected":

```go
func (app *application) userError(
    w http.ResponseWriter, r *http.Request, err error, safeURL string,
) {
    var ve domain.ValidationError
    if errors.As(err, &ve) {
        app.putFlashError(r.Context(), ve.Message)
        redirect(w, r, safeURL)
        return
    }
    // Non-validation = unexpected = catastrophic UX.
    app.serverError(w, r, err)
}
```

This is a behaviour change for migrated handlers: a DB hiccup mid-POST
now lands the user on `/error` instead of showing a "Couldn't complete
that action" banner on the safe URL. That is the *intended* shift — the
two failure classes deserve different UX, and conflating them via the
banner was a compromise of the original 2026-05-18 design under the
assumption that the shim could not be made smarter. It can.

### Wire protocol — unchanged

The existing protocol already says:
- `200 + X-Location: <path>` → shim navigates to `<path>` (pop-or-push).
- `200 + X-Location + X-Replace-Url: true` → navigate with replace.
- Any other status → `location.reload()`.

`serverError` on a stacknav POST now produces the first form, targeting
`/error[?from=…]`. No new headers, no new client states.

### What the user sees

Concrete walkthroughs for the affected scenarios:

| Scenario | Before | After |
|---|---|---|
| `POST /workouts/X/complete` before `/start` | Silent reload, no banner | Navigation to `/error?from=/workouts/X`. Error page with "← Back to your workout" + Go Home. |
| Handler panics mid-POST | Silent reload | Same as above — `recoverPanic` runs `serverError`, which now navigates. |
| `POST /admin/exercises` returns DB error | (Migrated handler) banner on `/admin/exercises` | Navigation to `/error?from=/admin/exercises`. |
| `POST /admin/exercises` returns `ValidationError` | Banner on `/admin/exercises` | Unchanged — `userError` validation path is untouched. |
| `GET /something/broken` 500s | Browser renders `error.gohtml` | Unchanged — non-shim path renders 500 inline. |
| `POST /workouts/X/complete` from `curl` | 500 + `error.gohtml` body | Unchanged — no `X-Requested-With: stacknav` header. |

### Accessibility

The error page already meets `role` and contrast requirements (`<h1>` for
the title, `.btn` for the actions, page-level layout). The "← Back" link
is a standard anchor; no live-region behaviour is needed because the user
navigated to this page (not been notified of an inline state change).

### Logging

`serverError` continues to log the underlying error at ERROR level *before*
emitting the navigation. Trace ID and request context are preserved as
today. The shim-navigation path adds nothing to the log line; observability
is unchanged.

## Migration

1. Land the `serverError` shim-aware change + `GET /error` route + template
   change in one commit. No handler changes are required for the basic
   improvement to take effect — `recoverPanic` and every unmigrated
   `serverError` POST call site immediately gain the new UX.
2. Land the `userError` narrowing in a follow-up commit. This is a
   behaviour change for migrated handlers (banner → error page on
   non-validation errors) and warrants its own review.
3. `workoutCompletePOST` and any other handler still doing inline
   `errors.As(&ve) { … } serverError(err)` migrate to `userError` over
   time. Under the new contract, the inline branch and the trailing
   `serverError` produce the same UX as `userError` does internally, so
   migration is a code-tidiness exercise rather than a UX fix.
4. `webauthnhandler.AuthenticateMiddleware` and `mustAdmin` switch their
   `http.Error` calls to the equivalent of `serverError` (via a thin
   adapter or by exposing the helper). Out of scope for this design;
   tracked separately.

## Risks

- **`Referer` is unreliable.** Direct POSTs (e.g. extensions, server-side
  callers) may omit it, and the spec allows it to be stripped by certain
  client configurations. Mitigated by the `?from=` being a UX hint only —
  the error page renders correctly without it.
- **`/error` becomes a high-visibility surface.** Any future change to its
  layout, copy, or wiring is a UX-critical change. Keep the page
  intentionally minimal: title, one explanatory sentence, two clear
  actions.
- **`userError` narrowing changes behaviour for migrated handlers.**
  Existing tests that assert "banner appears on safe URL after DB-error
  scenario" will break. Update them to assert navigation to `/error`
  instead. Class A (`ValidationError`) tests are untouched.
- **Maintenance-mode interaction.** `/error` is wired outside
  `maintenanceMode`, so an error during maintenance still produces the
  error page (not the maintenance page). Acceptable: an error is more
  specific than maintenance, and the user can still navigate home.
- **`recoverPanic` post-partial-write.** If the panic fired after the
  handler had already written headers, the `X-Location` write will fail
  silently and the shim falls through to reload. Same floor as today;
  not a regression.

## Test plan

- `e2etest` scenario: register, schedule today, POST `/workouts/{today}/complete`
  without `/start`, assert response is `200 + X-Location: /error?from=/workouts/{today}`,
  then GET that URL and assert the error page renders with a back link to
  the workout.
- `e2etest` scenario: same as above but submitted by `curl` semantics
  (no `X-Requested-With` header) — assert response is `500 + error.gohtml`
  body.
- `e2etest` scenario: `GET /error` directly (no `?from=`) — assert page
  renders with Go Home only.
- `e2etest` scenario: `GET /error?from=//evil.example.com` — assert the
  back link is omitted (sanitisation works).
- `e2etest` scenario: `GET /error?from=/workouts/2026-05-24` — assert the
  back link points at the workout.
- Unit test on a `recoverPanic`-wrapped handler that panics inside a
  POST — assert the shim path produces the navigation header.
- Existing `userError`-validation tests must still pass unchanged.
- Pre-existing tests that asserted "banner on safe URL after server
  error" need to be updated to assert navigation to `/error`.

## File touch-list

- `cmd/web/helpers.go` — `serverError` body, `userError` body.
- `cmd/web/routes.go` — register `GET /error`.
- `cmd/web/handler-error.go` (new) — `errorGET` handler + template data
  struct.
- `ui/templates/pages/error/error.gohtml` — replace Retry button with
  context-aware Back link.
- `cmd/web/CLAUDE.md` — update the "Error Handling" section to describe
  the new `serverError`/`userError` split (the prior change in this
  branch already prepared the ground; this finalises it).
- `docs/superpowers/specs/2026-05-18-error-ux-conventions-design.md` —
  add a short "Superseded by" note pointing here for the class-C path.
- `cmd/web/handler-workout_test.go`, `cmd/web/handler-admin-exercises_test.go`,
  and any other test that asserts banner-on-safe-URL after a non-validation
  error — update assertions to expect navigation to `/error`.
