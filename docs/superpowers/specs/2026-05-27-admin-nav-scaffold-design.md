# Admin nav: preferences entry + sub-nav scaffold

## Motivation

Admins today have no discoverable path to admin tooling. The two existing
admin pages (`/admin/exercises`, `/admin/feature-flags`) are reachable only
by typing the URL, and once on one admin page there is no link to the
other — admins navigate by retyping or by going home and re-typing.

The `users.is_admin` flag is already exposed on every page via
`BaseTemplateData.IsAdmin` (`internal/contexthelpers/getters.go:43`), and
the `mustAdmin()` middleware (`cmd/web/middleware.go:222`) already gates
the admin routes. What's missing is the navigation surface: a way in
(from preferences) and a way to move between sections once inside.

Two related gaps, one change:

1. **No entry point.** Preferences is the natural "settings" surface and
   is open to every logged-in user, but it has no admin-only entry.
2. **No movement between admin sections.** Each admin page is standalone
   with no shared chrome. As a third or fourth admin section appears,
   this gets worse.

## Approach

**Two-part change:**

1. Add an admin-only panel to the preferences page that links to `/admin`.
2. Add `/admin` as a redirect to `/admin/exercises`, plus a shared
   `admin-nav` template partial included near the top of every admin
   page so admins can jump between sections in one click.

Sub-nav (horizontal tab bar) over sidebar: the rest of the app is
mobile-first with no global nav chrome, and we have only two sections
today. Tabs match the existing visual language (the `page-header`
component, `.stack` panels) and wrap gracefully on narrow viewports.

Partial template over a new layout template: PetrApp has no nested
layouts today — every page owns its `<main>` starting from `base.gohtml`.
Introducing a layout system for two admin pages is heavier than the
benefit. A partial included at the top of each admin page achieves the
same result with zero framework changes.

## Preferences entry

Add a new panel to `ui/templates/pages/preferences/preferences.gohtml`,
placed between the existing `04 Your account` panel and the `danger-zone`
section. Rendered only when `BaseTemplateData.IsAdmin` is true. The panel
follows the same `panel-eyebrow` / `panel-title` / `panel-blurb` shape as
every other panel on the page so it inherits the existing scoped CSS,
gets the same entry animation (`panel-rise`), and reads as part of the
stack rather than a bolted-on addition:

```gohtml
{{ if $.IsAdmin }}
    <section class="panel" aria-labelledby="admin-title">
        <header class="panel-head">
            <span class="panel-eyebrow"><span class="panel-eyebrow-num">05</span> Admin</span>
            <h2 class="panel-title" id="admin-title">Admin tools</h2>
            <p class="panel-blurb">Manage exercises and feature flags.</p>
        </header>
        <div class="panel-actions">
            <a href="/admin" class="btn btn--ghost btn--block">Open admin</a>
        </div>
    </section>
{{ end }}
```

The `IsAdmin` field is on `BaseTemplateData`, which `preferencesTemplateData`
embeds — reachable from inside the page template as `$.IsAdmin` (the page
data root). No handler changes are needed.

Position rationale: admin tooling is not a personal preference, so it
sits visually separated from the schedule/notifications/recovery stack
and after "Your account" (which closes out personal data management).
Placing it before the danger-zone keeps the destructive Delete action at
the very bottom where it is today. For non-admins the panel is omitted
entirely, so they see panels 01–04 followed by the danger-zone, exactly
as today.

Numbering choice: panel `05` for admins, gap (no 05) for non-admins. The
numbers are visual ordinals (`panel-eyebrow-num`), not stable IDs — a
non-admin user simply sees four numbered panels, an admin sees five.
Renumbering existing panels conditionally would create churn for the
much more common non-admin case; keeping the existing numbers stable
and appending `05` for admins is the lighter change.

Linking to `/admin` (not `/admin/exercises`) keeps the preferences entry
stable when admin sections come and go. The redirect handles the
selection of the default landing section.

## `/admin` landing redirect

New route under the existing `app.mustAdminStack()` middleware in
`cmd/web/routes.go`, alongside the other `/admin/*` routes:

```go
mux.Handle("GET /admin", app.mustAdminStack(http.HandlerFunc(app.adminGET)))
```

Handler in `cmd/web/handler-admin.go` (new file, mirrors the
single-purpose-handler shape used elsewhere):

```go
func (app *application) adminGET(w http.ResponseWriter, r *http.Request) {
    redirect(w, r, "/admin/exercises")
}
```

`redirect` is the existing helper that emits the right response for both
the stack-navigator shim path and the native 303 path. No template, no
template data, no flash plumbing.

Choosing `/admin/exercises` as the default landing section: it's the
more frequently used surface today (exercise edits happen routinely;
feature-flag toggles are rare). Easy to revisit later by changing one
string.

## Sub-nav partial

New template `ui/templates/components/admin-nav.gohtml`. Follows the
project's partial conventions (see `ui/templates/CLAUDE.md` "Shared
Components"): one component per file, sibling `<style>` block carrying
the nonce, `@scope` rule keyed to a class on the root, gotype comment
at the top for IDE type-completion.

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.AdminNavData*/ -}}
{{ define "admin-nav" }}
<style {{ .Nonce }}>
    @scope (.admin-nav) {
        :scope {
            display: flex;
            flex-wrap: wrap;
            gap: var(--size-2);
            margin-block-end: var(--size-4);
            list-style: none;
            padding-inline-start: 0;
        }

        .admin-nav__tab {
            position: relative;
            display: inline-flex;
            align-items: center;
            min-height: 3rem; /* 48 px touch target */
            padding-inline: var(--size-3);
            padding-block: var(--size-2);
            border-radius: var(--radius-2);
            background: var(--color-surface-elevated);
            color: var(--color-text-secondary);
            text-decoration: none;
            font-weight: 500;
        }

        .admin-nav__tab[aria-current="page"] {
            background: var(--color-surface-active);
            color: var(--color-text-primary);
            font-weight: 600;
        }

        .admin-nav__tab[aria-current="page"]::after {
            /* Non-colour state signal (SC 1.4.1): underline accent on active tab. */
            content: "";
            position: absolute;
            inset-inline: var(--size-3);
            inset-block-end: -2px;
            block-size: 2px;
            background: var(--color-text-primary);
        }

        .admin-nav__tab:focus-visible {
            outline: 2px solid var(--color-text-primary);
            outline-offset: 2px;
        }
    }
</style>
<nav class="admin-nav" aria-label="Admin sections">
    <a class="admin-nav__tab" href="/admin/exercises"
       {{ if eq .Active "exercises" }}aria-current="page"{{ end }}>
        Exercises
    </a>
    <a class="admin-nav__tab" href="/admin/feature-flags"
       {{ if eq .Active "feature-flags" }}aria-current="page"{{ end }}>
        Feature Flags
    </a>
</nav>
{{ end }}
```

Data contract, added to `cmd/web/components.go` (the documented home for
component dot-structs):

```go
type AdminNavData struct {
    Active string            // "exercises" | "feature-flags"
    Nonce  template.HTMLAttr // matches the existing component pattern (see BackLinkData, BannerData)
}

const (
    adminSectionExercises    = "exercises"
    adminSectionFeatureFlags = "feature-flags"
)
```

Notes on the choices above:

- **`min-height: 3rem`** — `.btn` enforces 48 px; the tabs adopt the
  same minimum to satisfy the project's 48×48 touch-target rule. No
  `::before` expansion needed because the visual itself already clears
  the threshold.
- **Active state ships with both colour and shape** — the `::after`
  underline is the SC 1.4.1 non-colour signal. Colour alone (`color`,
  `background`, `font-weight`) would fail the project's "state is never
  colour-only" non-negotiable.
- **`aria-current="page"`** for the active tab — standard ARIA value
  for current-page indication in nav landmarks. `sets-container.gohtml`
  uses `aria-current="step"` for its row-of-sets affordance; `"page"`
  is the right value here because the tab points at a different URL.
- **No `:hover` rule** — project convention is mobile-first; style
  `:active` / `:focus-visible` instead.
- **Colour tokens** are the real ones in `ui/static/main.css`:
  `--color-surface-elevated` for inactive tab fill,
  `--color-surface-active` for active tab fill,
  `--color-text-secondary` → `--color-text-primary` for the text-color
  shift. The pairing matrix in `/dev/styleguide` should be checked when
  wiring this up — if any pairing isn't already validated, either pick
  a validated alternative or get the new pairing measured and added.
- **`Active` is a plain string** — two values today, used only in a
  template `eq` comparison. If a third section lands and the
  comparison logic spreads, promote to a typed const. Explicitly YAGNI
  now.

## Wiring admin pages into the scaffold

Two admin pages exist today. Each gets:

1. An `AdminNav AdminNavData` field added to its existing template-data
   struct.
2. Its GET handler populates `AdminNav.Active` with the section
   constant and `AdminNav.Nonce` from the base template data.
3. The template includes `{{ template "admin-nav" .AdminNav }}` near the
   top of `<main>`, immediately after the `page-header` call.

### Exercises

- File: `cmd/web/handler-admin-exercises.go`
- Affected handlers: `adminExercisesGET`, `adminExerciseEditGET` — both
  set `Active: "exercises"`.
- Templates:
  - `ui/templates/pages/admin-exercises/admin-exercises.gohtml` (the list)
  - `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml` (the edit form)

  Both templates include the partial with the same active value.

### Feature flags

- File: `cmd/web/handler-admin-feature-flags.go`
- Affected handler: `adminFeatureFlagsGET` sets `Active: "feature-flags"`.
- Template: `ui/templates/pages/admin-feature-flags/admin-feature-flags.gohtml`.

`adminFeatureFlagTogglePOST` is mutating only and redirects back to
`/admin/feature-flags` — no template, no `AdminNav` needed.

## Styleguide entry

Per `ui/templates/CLAUDE.md`: "The `/dev/styleguide` page is the living
catalog — add an entry there for any new component, and assert it in
`cmd/web/handler-styleguide_test.go`."

Add an `admin-nav` section to `ui/templates/pages/styleguide/styleguide.gohtml`
showing both states side by side — one render with `Active: "exercises"`,
one with `Active: "feature-flags"` — so the visual diff between active
and inactive tabs is verifiable at a glance. Add the corresponding
assertion in `cmd/web/handler-styleguide_test.go` that the styleguide
page contains the rendered `admin-nav` markup.

## Routing summary

| Path | Handler | Notes |
|---|---|---|
| `GET /admin` | `adminGET` (new) | 302/303 → `/admin/exercises` |
| `GET /admin/exercises` | `adminExercisesGET` (existing) | sets `Active: "exercises"` |
| `GET /admin/exercises/{id}` | `adminExerciseEditGET` (existing) | sets `Active: "exercises"` |
| `POST /admin/exercises/{id}` | unchanged | mutating, no template |
| `POST /admin/exercises/generate` | unchanged | mutating, no template |
| `GET /admin/feature-flags` | `adminFeatureFlagsGET` (existing) | sets `Active: "feature-flags"` |
| `POST /admin/feature-flags/{name}/toggle` | unchanged | mutating, no template |

All routes already sit (or are added) inside `app.mustAdminStack()`. No
authorization changes.

## Testing

Extend `cmd/web/handler-preferences_test.go`:

- Admin user: `GET /preferences` returns a document containing a link
  with `href="/admin"` inside a panel labelled "Admin".
- Non-admin user: `GET /preferences` does **not** contain any link to
  `/admin`. Use the existing e2etest user-factory helpers (an
  is-admin variant exists; if not, add one — see
  `internal/repository/users.go` for the underlying boolean).

Extend `cmd/web/handler-admin-exercises_test.go` (or create alongside
the existing admin handler tests, following the file layout of
`handler-admin-feature-flags_test.go` if present):

- `GET /admin/exercises` renders the sub-nav with a link whose
  `aria-current="page"` matches the Exercises entry.
- `GET /admin/exercises/{id}` (edit form) likewise renders the sub-nav
  with Exercises marked active.

Extend `cmd/web/handler-admin-feature-flags_test.go`:

- `GET /admin/feature-flags` renders the sub-nav with Feature Flags
  marked active.

New `cmd/web/handler-admin_test.go`:

- `GET /admin` as admin returns a redirect with `Location:
  /admin/exercises` (use the shim-header pattern from
  `handler-workout_test.go:540` for the `X-Location` assertion).
- `GET /admin` as non-admin returns 403/redirect to `/forbidden` —
  inherited from `mustAdmin()`, asserted explicitly to lock in the
  authorization surface.

Use the goquery selectors documented in `cmd/web/CLAUDE.md` under
"Testing with e2etest" — look up the sub-nav by its `aria-label`, then
assert the descendant links and their `aria-current` attribute.

## Acceptance

- Admin sees an "Admin" panel (numbered `05`) on `/preferences`, between
  the existing "Your account" panel and the danger-zone section, with a
  link to `/admin`. Non-admin sees no such panel and no link to any
  `/admin/*` URL on `/preferences`.
- `GET /admin` redirects to `/admin/exercises` for admins; returns the
  same forbidden response as other admin routes for non-admins.
- `/admin/exercises`, `/admin/exercises/{id}`, and `/admin/feature-flags`
  each render the horizontal sub-nav immediately after the page header,
  with the matching tab marked `aria-current="page"` and visually
  distinguished by both colour shift AND the underline accent.
- The sub-nav wraps gracefully at 320 px viewport width (no horizontal
  scroll, no overflow); each tab clears the 48 px touch-target rule.
- `/dev/styleguide` renders the `admin-nav` component in both active
  states, and the styleguide handler test asserts its presence.
- No regression on existing admin functionality: exercise edits, exercise
  generation, and feature-flag toggles continue to work and redirect as
  they do today.
- `make ci` passes.

## Out of scope

- An admin dashboard / overview at `/admin`. It's a redirect today;
  promote to a real page when there's content to show.
- Promoting/demoting admin users through the UI. Still a direct SQL
  change.
- A typed enum for `Active`. String comparison until a 3rd section
  forces the question.
- A global page-header admin badge or any admin-visible signal on
  non-admin pages. The preferences entry is the only discoverability
  surface in this change.
- Migrating admin pages to a nested-layout system. The partial-include
  approach is sufficient for two-to-four sections.
