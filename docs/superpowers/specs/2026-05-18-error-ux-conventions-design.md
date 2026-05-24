# Error UX Conventions

**Date:** 2026-05-18
**Status:** Approved (design phase)

**Class-C update:** the "silent failure when handlers stay on `serverError`"
note in this design has been superseded by
[2026-05-24 Shim-Aware `serverError`](2026-05-24-servererror-shim-aware-design.md).
Class C now navigates to `/error` instead of producing a silent reload.

**Scope:** Server-side and JavaScript error surfacing across the MPA, with WCAG 2.2 AA accessibility.

## Problem

Today there is one user-visible error surface (the `banner` component, rendered
from a session flash on the next GET) and one path that reaches it:
`domain.ValidationError` is caught with `errors.As`, the message is stuffed
into the session via `putFlashError`, and the handler redirects back to the
form URL. The form's GET handler pops the flash, and the template renders the
banner with `role="alert"`.

Anything that is **not** a `ValidationError` goes through `app.serverError`,
which logs the error and renders the generic full-page `error.gohtml`. From
the JavaScript shim's point of view (`ui/static/main.js`), any non-200
response triggers `location.reload()` — and any thrown `fetch` triggers the
same.

This convention works for validation. It does not fit three other error
classes the app keeps producing:

| Class | Example | Today's behavior | UX outcome |
|---|---|---|---|
| **B. Expected business error** | Add exercise on unplanned day | Handler wraps `ErrNotFound` and calls `serverError` → 500 → JS reloads | Button "does nothing" — reload to same page, no banner |
| **C. Unexpected server error** | DB hiccup mid-POST | `serverError` → 500 → JS reloads | Same. Silent failure from the user's POV. |
| **D. Network failure** | Offline / fetch throws | JS `catch` → `location.reload()` | If offline, reload also fails; user lands on the browser's offline page |

Mechanical root cause for B and C: `serverError` never writes to the flash,
so the reload that JS triggers reads no flash and shows no banner. The
convention is fine; the gap is that it only fires for `ValidationError`.

## Goal

A single, unified error-surfacing convention that covers all four error
classes (validation, expected business error, unexpected server error,
client-side network failure), keeps the MPA / Trusted-Types architecture
intact, and stays inside the existing `banner` component as the single
visual surface.

## Non-goals

- A toast / snackbar component.
- In-place HTML injection of server-rendered error fragments (CSP / Trusted
  Types blocks this).
- A structured per-field server-side error channel. Native HTML5 validation
  handles client-side field UX; a single page-level banner handles
  server-side messages.
- Optimistic UI, retry queues, or offline replay.
- A global error path for pure-GET 500s. Those continue to render
  `error.gohtml` because there is no in-flight user action to flash to.
- A mass rewrite of existing handlers. Go-forward convention only; touched
  handlers migrate opportunistically.

## Design

### Three terminal calls

Every POST handler ends in exactly one of:

| Call | When | Effect |
|---|---|---|
| `redirect(w, r, url)` | Success | 200 + `X-Location` (stacknav) or 303 (plain); client navigates |
| `app.userError(w, r, err, safeURL)` | Any user-visible failure on an in-flight action | Routes by error type, calls `putFlashError`, then `redirect(w, r, safeURL)` |
| `app.serverError(w, r, err)` | True full-page failure (template render, broken session, no safe URL exists) | Logs and renders `error.gohtml` 500. Rare. |

### `userError` semantics

`userError` is the single new helper. Its message-routing rule:

```go
func (app *application) userError(
    w http.ResponseWriter, r *http.Request, err error, safeURL string,
) {
    var ve domain.ValidationError
    var msg string
    if errors.As(err, &ve) {
        msg = ve.Message
    } else {
        app.logger.LogAttrs(r.Context(), slog.LevelError,
            "user-facing server error", slog.Any("error", err))
        msg = "Couldn't complete that action. Please try again."
    }
    app.putFlashError(r.Context(), msg)
    redirect(w, r, safeURL)
}
```

This collapses today's `errors.As(&ve) { putFlashError(ve.Message);
redirect(formURL) }` boilerplate into one call, and makes the unexpected
case behave the same way (flash + redirect) instead of falling through to a
full-page 500.

#### `safeURL` is explicit per handler

Required argument. No defaulting to `r.Referer()` (unreliable on direct
POSTs and easily forged) or to the request URL (wrong for action endpoints
like `POST /workouts/.../complete`, which would 404 on GET). The call site
knows where to land the user; pass it.

#### Generic message for system errors

One message: `"Couldn't complete that action. Please try again."` We do not
leak underlying causes ("DB error", "timeout") because the user cannot act
on the distinction, and any nuance we want to expose later belongs in a
sentinel `ValidationError` (which becomes class B).

### Wire protocol — unchanged

Still 200 + `X-Location` for stacknav, 303 for plain. No new headers. The
flash mechanism does all the work; the JS shim does not need to learn
anything new for the server-error path.

This preserves the property that **any non-2xx → `location.reload()` is
correct** at the shim layer, because the server's flash will surface on the
reloaded page. The shim's non-2xx branch stays as the safety net for
unexpected failures that escape `userError` (a panic, a 502 from the proxy,
etc.).

### JavaScript network-failure path

Today: `fetch` throws → `location.reload()`. If the user is offline, the
reload also fails.

New: `base.gohtml` carries a static skeleton above `<main>`:

```gohtml
<div id="js-flash" class="banner banner--error" role="alert" hidden></div>
```

Co-located `<style {{ nonce }}>` block uses the existing banner tokens
(`--color-error`, `--color-error-bg`, `--radius-2`). The element is *always*
present in the DOM but hidden by the `hidden` attribute; `role="alert"` is
an implicit `aria-live="assertive"`, so populating `textContent` and toggling
`hidden = false` triggers a live-region announcement across NVDA / JAWS /
VoiceOver.

In `main.js`, the `catch` block becomes:

```js
} catch (_) {
    const flash = document.getElementById('js-flash')
    if (flash) {
        flash.textContent =
            'Connection lost. Check your network and try again.'
        flash.hidden = false
    }
    clearLoad()
    return
}
```

CSP-clean: `textContent` is inert to markup, no Trusted-Types policy needed.

#### Clearing the client flash

- On a successful subsequent navigation, the new document destroys the
  element naturally.
- For in-place success after a prior client-only failure, we hide the
  banner at the top of `submitForm` (before issuing the fetch):
  `flash.hidden = true; flash.textContent = ''`.

#### Scope

`#js-flash` exists only for client-only failure (network throw, bad CORS,
opaque-redirect). For any server response — even non-200 — the shim still
reloads; the server's flash carries the message. This keeps the client
banner as a true last-resort surface.

### Domain-layer: user-displayable business errors

Class B errors (logically-not-allowed actions that today get wrapped into
opaque `fmt.Errorf` and surface as 500s) reuse `domain.ValidationError`. No
new error type is needed; `ValidationError` already carries the
"user-safe message" semantics, and `userError` already routes it correctly.

Worked example — `internal/service/exercises.go` `AddExercise`:

```go
session, err := s.repos.Sessions.Get(ctx, date)
if errors.Is(err, domain.ErrNotFound) {
    return 0, domain.ValidationError{
        Message: "This day has no planned workout. " +
            "Schedule one from the home page first.",
    }
}
if err != nil {
    return 0, fmt.Errorf("get session %s: %w", date, err)
}
```

Convention going forward:

- If a service-layer failure has a message the user can act on, return
  `domain.ValidationError{Message: "..."}`.
- If it's a system fault the user cannot fix, return the wrapped underlying
  error.

### Handler change — worked example

`cmd/web/handler-workout.go` `workoutAddExercisePOST`:

```go
newWorkoutExerciseID, err := app.service.AddExercise(r.Context(), date, exerciseID)
if err != nil {
    workoutURL := fmt.Sprintf("/workouts/%s", date.Format("2006-01-02"))
    app.userError(w, r, err, workoutURL)
    return
}
```

No `errors.As` switch in the handler. `userError` does the routing. The
safe URL is explicit — on failure, the user lands on the workout day page
with the banner explaining the situation.

### Accessibility

- `banner` (server-rendered, post-redirect path): unchanged. Already uses
  `role="alert"` for the error variant; the element is new in the DOM on
  the next page render, so screen readers announce it.
- `#js-flash` (client-only path): pre-existing element with `role="alert"`.
  Populating `textContent` and revealing it is a recognised live-region
  pattern — assistive tech announces the new content.
- Focus is not moved on either surface. The banner appears above `<main>`
  and is announced; we do not steal focus from whatever the user was
  interacting with (consistent with WCAG 2.4.3 focus order — programmatic
  focus changes after a user action are disorienting when not requested).
- Touch targets and contrast: the banner already uses
  `--color-error` / `--color-error-bg`, which are documented as a "suspect
  pairing" in `ui/templates/CLAUDE.md` (Accessibility → Colour contrast).
  Verify the measured ratio when adding the pairing matrix entry; tighten
  to `color-mix(... black)` if needed. No new tokens introduced.
- Reduced motion and forced colours: no new motion or graphical signal
  introduced. The banner inherits existing styles.

### CLAUDE.md updates

- `cmd/web/CLAUDE.md`: replace the "User-facing validation errors" section
  with "User-facing errors". Document the three terminal calls
  (`redirect`, `userError`, `serverError`), the `userError` semantics
  (validation vs system message routing), and the `safeURL` rule. Keep the
  go-forward language — existing handlers are not in scope.
- `ui/templates/CLAUDE.md`: add a one-paragraph note under the JavaScript
  section that `#js-flash` is the client-only error surface and must be
  populated via `textContent` (not `innerHTML`).

## Migration

- **In this PR / plan**:
  - Add `app.userError` to `cmd/web/helpers.go`.
  - Add `#js-flash` skeleton to `ui/templates/base.gohtml` with co-located
    styles.
  - Update `submitForm` in `ui/static/main.js` to use `#js-flash` on the
    `catch` branch and to clear it at the top of each submit. **Bump the
    fingerprinted filename** so the new shim is served (per `Dockerfile`
    md5 rewrite — editing `main.js` in dev is fine, but the deploy needs
    the cache-bust).
  - Migrate the "add exercise on unplanned day" path:
    `internal/service/exercises.go` returns `ValidationError` for the
    missing-session case; `workoutAddExercisePOST` uses `userError` with
    the workout URL as `safeURL`.
  - Update both CLAUDE.md files.
  - Regression test in `cmd/web/handler-workout_test.go`: submit the add
    form on an unplanned date, follow the redirect, assert the banner text
    and `role="alert"` selector on the workout page.

- **Out of scope, deliberate**:
  - Migrating other handlers from the `errors.As(&ve)` boilerplate to
    `userError`. They keep working; they migrate when next touched.
  - JS-layer test for the network-failure path. The change is small and
    stable; document a manual smoke step (devtools "Offline" toggle,
    submit a form, expect the `#js-flash` to appear).
  - A toast component, a per-field error channel, or any change to
    `error.gohtml`.

## Risks

- **`role="alert"` on a hidden element**: some screen readers in the past
  had inconsistent behavior when `aria-live` regions started hidden. The
  modern spec is clear that `hidden` plus `role="alert"` is fine and the
  element becomes announceable when revealed; verify with NVDA + VoiceOver
  during implementation. Fallback if needed: render the element without
  `hidden`, keep it empty, and rely on `:empty { display: none; }` in the
  scoped style (CSS-only hide, no `hidden` attribute interaction).
- **Stale flash leaking across navigations**: `popFlashError` is a "pop"
  (read + delete), so a flash written by `userError` is consumed by the
  next GET. Risk is low; covered by existing session-manager behavior.
- **`safeURL` redirect loops**: a handler that redirects to a GET handler
  which itself fails would loop. Discipline: `safeURL` should always
  point to a page known to render successfully (a stable list / detail
  page, never an action endpoint). Document in `cmd/web/CLAUDE.md`.
- **Visual banner pairing contrast**: the `--color-error` /
  `--color-error-bg` pairing is flagged as "suspect" in
  `ui/templates/CLAUDE.md`. The pairing matrix entry in `/dev/styleguide`
  needs a measured ratio; tighten the foreground if it lands under 4.5:1.

## Test plan

1. **Unit / handler test**: `workoutAddExercisePOST` on an unplanned date
   redirects to `/workouts/{date}` with the banner text present and
   `role="alert"` on the banner element.
2. **Unit / handler test**: existing `ValidationError` flows continue to
   produce the same banner on the form URL (`userError` is a drop-in
   replacement for the old `errors.As` boilerplate).
3. **Manual smoke (documented)**:
   - Devtools "Offline" → submit any form → `#js-flash` appears with the
     network-loss message; page stays in place; user can re-enable network
     and retry.
   - Trigger a non-2xx response (kill the service mid-POST, or stub a
     500) → JS reloads → server-rendered banner appears with the generic
     message.
4. **Accessibility**: NVDA + VoiceOver announce the banner on the
   server-rendered path and announce the `#js-flash` on the
   network-failure path.

## File touch-list

- `cmd/web/helpers.go` — add `userError`.
- `cmd/web/handler-workout.go` — migrate `workoutAddExercisePOST` to
  `userError`.
- `internal/service/exercises.go` — return `ValidationError` from
  `AddExercise` for the missing-session case.
- `ui/templates/base.gohtml` — add `#js-flash` skeleton with co-located
  scoped style.
- `ui/static/main.js` — populate `#js-flash` in `submitForm`'s `catch`;
  clear it at the top of each submit. (No manual cache-bust needed —
  `main.js` is md5-fingerprinted at Docker build time.)
- `cmd/web/CLAUDE.md` — rewrite "User-facing validation errors" section.
- `ui/templates/CLAUDE.md` — add `#js-flash` note under JavaScript.
- `cmd/web/handler-workout_test.go` — regression test for unplanned-day
  add-exercise.
