# Preferences: preserve scroll on submit, add per-panel flash

## Motivation

Every form on `/preferences` currently lands the user at the top of the next
page — either home (`POST /preferences`) or `/preferences` itself (the deload
actions and the notifications toggle). For the Recovery panel that means a
~700 px scroll jump away from the controls the user was just touching, with
no signal that the mutation succeeded.

Two related problems, one pattern:

1. **Scroll loss on submit.** The mutation succeeds but the user has to
   scroll back to verify, and on the Restart-cycle action there is nothing
   visible to verify against until next Monday.
2. **No success feedback.** The codebase has `putFlashError` and the
   `banner` component already renders a `success` variant, but no handler
   ever sets a success flash. Every mutation is silent on success.

The canonical Post-Redirect-Get pattern says: redirect to a page that shows
the result, and surface a flash banner so the user knows the action
landed. We adopt that pattern here in a way that generalizes to the rest of
the app.

## Approach

**Pattern A: hash-fragment in the redirect target, flash banner inside the
owning panel.**

Each form's handler picks a redirect URL that includes a fragment
identifier for the panel that owns the form (e.g. `/preferences#deload-title`).
The browser snaps that panel into view on both the stack-navigator path
(`navigation.navigate(target)` honors fragments) and the native 303 path.
Each mutation also writes a success flash carrying the same anchor; the
GET handler routes the flash into a per-panel slot rendered inside the
matching panel, so the message lands where the user is looking after the
snap.

Snap-to-panel is intentional over pixel-precise restoration: the panel's
state has just changed (a new `.anchor-note` appeared, the disabled
buttons just enabled), and putting the heading at the top of the viewport
makes the change visible. Pixel-precise scroll restoration is a different
problem reserved for the sets-editing flow and is out of scope here.

## Redirect targets

| Form | Method + path | Redirects to |
|---|---|---|
| Save week (schedule) | `POST /preferences/schedule` (new) | `/` |
| Save recovery settings | `POST /preferences/deload` (new) | `/preferences#deload-title` |
| Notifications toggle | `POST /preferences/rest-notifications-toggle` | `/preferences#notif-title` |
| Start deload this week | `POST /preferences/mesocycle/start-deload-now` | `/preferences#deload-title` |
| Restart cycle next Monday | `POST /preferences/mesocycle/restart` | `/preferences#deload-title` |
| Delete account | `POST /preferences/delete-user` | `/` |

The two no-fragment redirects keep going to `/`: the schedule save's effect
is on the home page (the regenerated week), and the deleted-account user
no longer exists.

## Split `POST /preferences` into two routes

`POST /preferences` currently serves **both** the Save-week form and the
Save-recovery-settings form. The handler can't tell them apart without a
discriminator, and we now need different redirect targets per form. Split
into two single-purpose handlers:

- `POST /preferences/schedule` → new `preferencesScheduleSavePOST`. Owns
  the seven `<weekday>_minutes` fields. Redirects to `/` on success.
- `POST /preferences/deload` → new `preferencesDeloadSavePOST`. Owns the
  `deload_enabled` checkbox and the `mesocycle_length` select. Redirects
  to `/preferences#deload-title` on success.

Each handler does its own read-modify-write on `domain.Preferences`,
mirroring the existing pattern in `preferencesRestNotificationsTogglePOST`.
Neither handler touches fields outside its panel's concern — the
`Save-week` form no longer carries hidden deload fields and the
`Save-recovery` form no longer carries hidden weekday fields.

The current shared handler `preferencesPOST` is removed. The route
`POST /preferences` is unregistered.

The empty-schedule validation (`prefs.IsEmpty()` → flash error → redirect
to form) moves to the new `preferencesScheduleSavePOST` only — it's a
constraint on the schedule form's input. The deload form cannot violate it
(it doesn't touch weekday minutes).

## CSS scroll-margin

Browsers snap the targeted element flush against the viewport top, which
visually crowds the heading. Add `scroll-margin-top: var(--size-5)` to the
panel headings inside the page's scoped `<style>` block:

```css
.panel-title { scroll-margin-top: var(--size-5); }
```

`scroll-margin-top` has been supported in iOS Safari since 14.5 — no
progressive enhancement needed.

The page's existing `panel-rise` entry animation translates each panel
upward 6 px. That animation runs once on page reveal and does not
interfere with fragment snap (the browser scrolls after layout settles).

## Flash plumbing extension

Generalize the session-backed flash from "error string only" to a typed
entry. Today the helper in `cmd/web/helpers.go` reads/writes a bare string
at session key `flash_error`. Replace it with:

```go
type flashEntry struct {
    Variant BannerVariant // success | info | error
    Message string
    Anchor  string        // panel id; empty = page-top slot
}
```

Stored under a new session key (`flash`) so any in-flight session at
deploy time silently retires the old `flash_error` orphan. Helpers:

```go
func (app *application) putFlash(ctx context.Context, variant BannerVariant, msg, anchor string)
func (app *application) putFlashSuccess(ctx context.Context, msg, anchor string)
func (app *application) putFlashError(ctx context.Context, msg string)                 // existing call sites
func (app *application) putFlashErrorWithAnchor(ctx context.Context, msg, anchor string)
func (app *application) popFlash(ctx context.Context) flashEntry                       // empty struct ⇒ nothing to show
```

`putFlashError(msg)` keeps its signature so the existing call sites
elsewhere in the codebase don't change. It writes `Anchor: ""` (page-top
slot). New code that wants an anchor calls `putFlashErrorWithAnchor`.
`popFlashError(ctx) string` is removed; pages that currently call it
switch to `popFlash(ctx)` and read the `Message` field.

`BannerData` and the `banner` component are unchanged.

## Per-panel banner rendering

`preferencesTemplateData` gains a single field:

```go
FlashByPanel map[string]BannerData
```

`preferencesGET` pops the flash once and routes the entry into one of two
slots based on its anchor:

```go
entry := app.popFlash(ctx)
data.FlashByPanel = map[string]BannerData{}
if entry.Message != "" {
    bd := BannerData{Variant: entry.Variant, Message: entry.Message, Nonce: base.Nonce}
    if entry.Anchor == "" {
        data.Flash = bd                            // page-top slot
    } else {
        data.FlashByPanel[entry.Anchor] = bd      // per-panel slot
    }
}
```

Pages that haven't adopted per-panel slots simply keep using the existing
`Flash BannerData` field on their template data and call `popFlash` to
drive it (reading `entry.Message`). Empty anchor on the popped entry
means "page-top slot"; non-empty anchor on a page that doesn't render
`FlashByPanel` means the flash is dropped — acceptable while migration
is opportunistic, since handlers that set an anchor today are by
definition targeting pages that DO render per-panel slots.

In `ui/templates/pages/preferences/preferences.gohtml`, each panel renders
its banner immediately under the `<header class="panel-head">`:

```gohtml
{{ template "banner" (index $.FlashByPanel "deload-title") }}
```

The `banner` component renders nothing when `Message` is empty, so panels
without a current flash emit nothing. Including the template call on every
panel keeps the markup symmetric and means future flash anchors require no
template change.

The existing page-top `{{ template "banner" .Flash }}` near the back-link
stays as the default slot for empty-anchor flashes.

## Handler wiring

Every mutating handler on this page picks an anchor at write time and
passes it consistently through both the flash and the redirect:

```go
const deloadAnchor = "deload-title"

func (app *application) preferencesDeloadSavePOST(w http.ResponseWriter, r *http.Request) {
    if !app.parseForm(w, r, defaultMaxFormSize) { return }
    prefs, err := app.service.GetUserPreferences(r.Context())
    if err != nil {
        app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
        return
    }
    prefs.DeloadEnabled = r.Form.Get("deload_enabled") == "on"
    prefs.MesocycleLength = parseMesocycleLength(r.Form.Get("mesocycle_length"))
    if err = app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
        app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
        return
    }
    app.putFlashSuccess(r.Context(), "Recovery settings saved.", deloadAnchor)
    redirect(w, r, "/preferences#"+deloadAnchor)
}
```

Same shape for the other handlers:

- `preferencesScheduleSavePOST` — no flash, redirect to `/`. The
  regenerated week shown on home is the feedback; setting a flash that
  the home page wouldn't render would leak into the next
  `/preferences` visit.
- `preferencesRestNotificationsTogglePOST` — anchor `"notif-title"`,
  message `"Rest pings enabled."` or `"Rest pings disabled."` chosen
  by the resulting `prefs.RestNotificationsEnabled`.
- `preferencesStartDeloadNowPOST` — anchor `"deload-title"`, message
  `"Deload started for this week."`.
- `preferencesRestartMesocyclePOST` — anchor `"deload-title"`, message
  `"Cycle will restart next Monday."`.

The existing empty-schedule validation in the new
`preferencesScheduleSavePOST` becomes:

```go
if prefs.IsEmpty() {
    app.putFlashErrorWithAnchor(r.Context(),
        "Please schedule at least one workout day.",
        "schedule-title")
    redirect(w, r, "/preferences#schedule-title")
    return
}
```

## Stack-navigator: no changes

`navigation.navigate('/preferences#deload-title')` honors the fragment on
push, replace, and traverse paths. `X-Location` carries the path verbatim
through the shim. Same-URL submit auto-replace still fires — the path
component is unchanged, so submitting from `/preferences` and being
redirected to `/preferences#deload-title` is detected as same-URL and
becomes a replace, which is the right history shape (the form submit
shouldn't push a duplicate entry).

The browser preserves fragments per history entry, so the back button
returns the user to whatever fragment they had on that entry.

## Acceptance

- Submitting any Recovery-panel form lands the user with the `<h2
  id="deload-title">` heading visible at the top of the viewport.
- Each mutation surfaces a success banner inside the panel that owns the
  form, with the message text per the table above.
- The empty-schedule validation flash appears inside the schedule panel,
  not at page top.
- `POST /preferences` no longer routes. `POST /preferences/schedule` and
  `POST /preferences/deload` carry the split.
- Schedule save still redirects to `/`. Account delete still redirects to
  `/`. No other form on the page redirects away from `/preferences`.
- `make ci` passes.

## Testing

Extend `cmd/web/handler-preferences_test.go`:

- For each form, assert the shim path returns `200` with
  `X-Location: /preferences#<expected-anchor>` (no follow). Use the
  shim-header pattern from `handler-workout_test.go:540`.
- For each form, follow the redirect and assert the matching panel
  contains a banner with the expected variant and message text. Resilient
  selector: look up the panel by `aria-labelledby`, then assert the
  `.banner` descendant.
- Empty-schedule POST to `/preferences/schedule`: assert the error banner
  lives inside the schedule panel, not the page-top slot.
- Existing `Test_application_preferencesPOST_preservesRestNotificationsEnabled`
  moves to target `POST /preferences/schedule` (the new home of the
  weekday fields) and the assertion shape is unchanged.

No new view-transition or scroll-position tests — JSDOM-like environments
can't observe scroll behavior reliably, and the fragment-snap is browser
behavior we trust.

## Generalization beyond preferences

The pattern any handler can adopt for the same UX:

1. Give the destination's panel/section a stable `id`.
2. `app.putFlashSuccess(ctx, msg, anchor)` before `redirect(w, r, path
   + "#" + anchor)`.
3. In the destination template, render `{{ template "banner" (index
   $.FlashByPanel "<anchor>") }}` inside each panel. Add a
   `FlashByPanel map[string]BannerData` field to that page's template data
   and pop the flash once in its GET handler.

The session payload, the typed flash helpers, and the banner component
are all shared. Each page that adopts it adds one template-data field and
one template call per panel; no shim or repository work.

## Out of scope

- Pixel-precise scroll restoration. Deferred to a separate spec when the
  sets-editing flow (`cmd/web/handler-exerciseset.go` /
  `ui/templates/pages/exerciseset/sets-container.gohtml`) needs it. That
  page edits many items in a long list and snap-to-anchor is the wrong
  shape there.
- Migrating other pages' flash usage to the new per-panel slot. They
  continue to use the page-top slot via the empty-anchor path. Migration
  is opportunistic, per page.
- Visual / motion polish on the snap itself (smooth-scroll, etc.). The
  default jump matches the rest of the app.
