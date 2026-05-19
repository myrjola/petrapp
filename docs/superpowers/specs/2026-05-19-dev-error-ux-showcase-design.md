# `/dev/error-ux` Showcase

**Date:** 2026-05-19
**Status:** Approved (design phase)
**Scope:** A dev-only page that demonstrates the four error-surfacing classes
documented in `2026-05-18-error-ux-conventions-design.md`, plus
front-page discovery links to the dev routes.

## Problem

The error UX conventions (`userError`, `safeURL`, `#js-flash`, full-page
`error.gohtml`) are written down but have no living catalog. Today, the
only way to confirm a banner's text or styling is to grep for a flow that
exercises it and trigger that flow manually — which is friction for both
design tweaks (does the new `--color-error` pairing meet 3:1?) and
onboarding (what does class C actually look like in this app?).

The styleguide page (`/dev/styleguide`) already shows the *static* banner
variants but not the *live* paths that produce them. The
flash-then-redirect mechanic, the generic-message routing, and the
client-only `#js-flash` are invisible to a reader of the styleguide.

A secondary problem: the existing dev route is undiscoverable. You have
to know the URL.

## Goal

A single dev-only page with one card per error class, each card
containing a control that actually triggers the path end-to-end so the
resulting banner is real, not a screenshot. Plus a small
front-page block that lists the dev routes when `app.devMode` is true.

## Non-goals

- Triggering a real network failure from the page. The client-only
  surface is demonstrated by a JS-only "simulate" button that populates
  `#js-flash` directly — same code path as `main.js`'s `catch` block,
  not a true network outage.
- A toast, a per-field error channel, or any change to the underlying
  convention. This page only *exercises* what already exists.
- Per-handler migration auditing. The page demonstrates the
  convention, not which handlers conform to it.
- Authentication. The dev routes inherit `sessionStack` (the same as
  `/dev/styleguide`) and are gated inside the handler on
  `app.devMode`. Prod returns 404.

## Design

### Routes

| Route | Method | Notes |
|---|---|---|
| `/dev/error-ux` | GET | Renders the page. Pops + renders any flash via `banner`. Returns 404 outside dev mode. |
| `/dev/error-ux/trigger` | POST | Form field `kind ∈ {validation, business, system}` dispatches to `userError`. `safeURL` is always `/dev/error-ux`. |
| `/dev/error-ux/server-error` | GET | Calls `app.serverError` so the user lands on `error.gohtml`. Demonstrates class E. |

All three are gated on `app.devMode` at the top of the handler
(`http.NotFound` otherwise). Routes are wired in `cmd/web/routes.go`
under the same comment block as `/dev/styleguide`.

### Page sections (one card each)

1. **A. Server-side validation error.**
   Form with a single button "Trigger validation error". POST →
   handler returns `domain.ValidationError{Message: "Name must be 1–50
   characters."}` → `userError` flashes verbatim → redirect to
   `/dev/error-ux` → banner appears with `role="alert"`.

2. **B. Expected business error.**
   Same path, different copy ("This day has no planned workout. Schedule
   one from the home page first.") — pulled from the worked example in
   the conventions doc so the message is recognisable.

3. **C. Unexpected system error.**
   POST returns `fmt.Errorf("simulated system fault: %w", io.ErrUnexpectedEOF)`.
   `userError` logs at ERROR and flashes the generic
   "Couldn't complete that action. Please try again." Demonstrates that
   the user sees a banner instead of a silent reload.

4. **D. Client-side network failure (simulated).**
   Button labelled "Simulate network failure". `<script {{ nonce }}>`
   attaches a `click` handler that populates `#js-flash` with the
   message used by `main.js` ("Connection lost. Check your network and
   try again.") and toggles `hidden = false`. The code is the same
   pattern the shim uses; documented inline that real failures take the
   same path.

5. **E. Full-page server error.**
   `<a href="/dev/error-ux/server-error">` — a navigation, not a form
   POST, because `serverError` does not redirect. Card text explains
   the trade-off: no flash, no banner, just `error.gohtml` 500. Use
   only when no safe URL exists.

6. **F. Banner variants reference.**
   Static rendering of `error`, `success`, `info` via the `banner`
   component. Mirrors the styleguide section so the dev page is
   self-contained.

### Template structure

New page `ui/templates/pages/error-ux/error-ux.gohtml`. Mirrors
`styleguide.gohtml`: single `{{ define "page" }}` block, scoped `<style {{ nonce }}>`
emitting a card layout (`.cards`, `.card` reusing tokens), each card a
labelled section.

Pop the flash in the handler and pass it as `Flash BannerData` on the
template data; render `{{ template "banner" .Flash }}` at the top.

### Data plumbing

```go
type errorUXTemplateData struct {
    BaseTemplateData
    Flash BannerData
}
```

The trigger handler reads `r.PostForm.Get("kind")` and switches on the
known values:

```go
switch kind {
case "validation":
    err = domain.ValidationError{Message: "Name must be 1–50 characters."}
case "business":
    err = domain.ValidationError{Message: "This day has no planned workout. Schedule one from the home page first."}
case "system":
    err = fmt.Errorf("simulated system fault: %w", io.ErrUnexpectedEOF)
default:
    http.NotFound(w, r); return
}
app.userError(w, r, err, "/dev/error-ux")
```

Unknown `kind` returns 404 — there is no graceful UX path for an
unrecognised demo trigger.

### Front-page discovery

`homeTemplateData` gains `DevMode bool`, populated from `app.devMode`
in `home`. Two templates show the section when true:

- `schedule.gohtml`: insert a short "Dev tools" link group inside
  `week-eyebrow`, alongside the existing Settings link. Treatment matches
  the eyebrow's mono / muted typography so it disappears into the
  editorial chrome rather than competing with content.
- `unauthenticated.gohtml`: the footer (next to "Privacy") grows a Dev
  group with the same two links.

Both surfaces show `Styleguide` and `Error UX` as the only two link
labels. New dev pages should add themselves here as they appear.

The choice to add to `homeTemplateData` rather than `BaseTemplateData`
is deliberate: only the home pages need it today, and the convention
in this codebase is to plumb data through page-specific structs unless
a value is genuinely cross-cutting (auth, CSP nonce). When the third
dev page appears, the call to widen scope can be made then.

### CSP / Trusted Types

- The simulated-network-failure button uses an inline `<script {{ nonce }}>` that
  sets `textContent` and toggles a property — no `innerHTML`, no
  script-loading sink. CSP-clean by construction.
- All scoped styles co-located with markup as `<style {{ nonce }}>`.

### Accessibility

- The page-top `banner` is server-rendered on each redirected GET;
  screen readers announce it as the new document enters because the
  element appears with `role="alert"`.
- The simulated-failure button populates the existing `#js-flash` in
  `base.gohtml`, which is the same live region used by the real shim.
- Each card has a `<h2>` and a single primary action; tab order is
  natural document order.
- Focus is not moved on banner appearance — same rule as the
  conventions doc (WCAG 2.4.3, no programmatic focus changes after a
  user action).

## Testing

Handler tests in `cmd/web/handler-error-ux_test.go` (matching
`handler-styleguide_test.go`):

1. `GET /dev/error-ux` returns 200 and contains an h2 for each of the
   four cards (validation, business, system, network).
2. `GET /dev/error-ux` returns 404 when `devMode == false` (same
   shape as the styleguide gating test).
3. `POST /dev/error-ux/trigger` with `kind=validation` → 303 or
   stacknav 200 → follow → page contains the validation message in a
   `.banner--error` element.
4. `POST /dev/error-ux/trigger` with `kind=system` → follow → page
   contains the generic system-error message in a `.banner--error`.
5. `POST /dev/error-ux/trigger` with unknown `kind` → 404.

Home-page tests in `cmd/web/handler-home_test.go`:

6. Authenticated home in dev mode renders an anchor to `/dev/error-ux`.
7. Authenticated home in non-dev mode does not.

The network-failure JS path is exercised by manual smoke (devtools
"Offline" toggle, submit any form → existing behavior already covered
by the conventions doc's smoke checklist).

## Risks

- **Dev route leak to prod.** The handler-side `if !app.devMode { http.NotFound(w, r); return }` is the only gate. Identical to the styleguide's
  existing approach, which has been in prod since
  `2026-04-22-exercise-selection-improvements`; mechanism is trusted.
- **Trigger endpoint as ambient capability.** Even though the page is
  dev-only, the POST endpoint is registered on the public mux. The
  `devMode` check runs first; outside dev, the POST is also 404.
- **Front-page clutter in screenshots / reviews.** The dev links only
  render when `app.devMode == true`; production HTML is byte-identical.

## File touch-list

- `cmd/web/handler-error-ux.go` — new. `devErrorUXGET`,
  `devErrorUXTriggerPOST`, `devErrorUXServerErrorGET`.
- `cmd/web/handler-error-ux_test.go` — new. Render + gating +
  trigger-paths tests.
- `cmd/web/routes.go` — register the three routes alongside
  `/dev/styleguide`.
- `cmd/web/handler-home.go` — populate `DevMode` on
  `homeTemplateData`.
- `cmd/web/handler-home_test.go` — assert presence/absence of dev
  links by mode.
- `ui/templates/pages/error-ux/error-ux.gohtml` — new page template.
- `ui/templates/pages/home/schedule.gohtml` — eyebrow dev block.
- `ui/templates/pages/home/unauthenticated.gohtml` — footer dev
  block.
