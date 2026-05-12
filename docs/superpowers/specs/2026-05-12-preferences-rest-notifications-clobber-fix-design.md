# Fix: weekday-schedule POST silently disables rest notifications

## Motivation

Shipped in the rest-timer series. Any authenticated user who submits **either**
weekday-schedule form (the onboarding `/schedule` form **or** the
`/preferences` weekly-schedule form) has their `rest_notifications_enabled`
column overwritten to `false`, because:

- Both `cmd/web/handler-preferences.go:preferencesPOST` and
  `cmd/web/handler-schedule.go:schedulePOST` constructed a fresh
  `domain.Preferences{}` via the shared `weekdaysToPreferences(r)` helper,
  leaving `RestNotificationsEnabled` at Go's zero-value `false`.
- `internal/repository/preferences.go:68` upserts `rest_notifications_enabled
  = excluded.rest_notifications_enabled` unconditionally.

The `//nolint:exhaustruct // RestNotificationsEnabled handled by separate
endpoint.` on the helper was exactly the lint hiding the bug. The comment
is false — no separate endpoint protects the value across these code paths.
`exhaustruct` was correctly flagging that the partial struct literal would
lose data.

The `/schedule` clobber is the more impactful path: it runs during
onboarding, before a user can reach `/preferences` to subscribe, so every
new user lost the column's default-true value silently. The `/preferences`
clobber re-fires later whenever a subscribed user updates their weekly
schedule.

## Fix

Replace the `weekdaysToPreferences(r)` helper with a read-modify-write
inside both `preferencesPOST` and `schedulePOST`, mirroring
`preferencesRestNotificationsTogglePOST` (`handler-preferences.go:209-225`).
Example (the schedule version keeps its existing `IsEmpty()` flash-error
branch unchanged; both handlers follow the same shape):

```go
prefs, err := app.service.GetUserPreferences(r.Context())
if err != nil { … }
prefs.MondayMinutes    = parseMinutes(r.Form.Get("monday_minutes"))
prefs.TuesdayMinutes   = parseMinutes(r.Form.Get("tuesday_minutes"))
prefs.WednesdayMinutes = parseMinutes(r.Form.Get("wednesday_minutes"))
prefs.ThursdayMinutes  = parseMinutes(r.Form.Get("thursday_minutes"))
prefs.FridayMinutes    = parseMinutes(r.Form.Get("friday_minutes"))
prefs.SaturdayMinutes  = parseMinutes(r.Form.Get("saturday_minutes"))
prefs.SundayMinutes    = parseMinutes(r.Form.Get("sunday_minutes"))
if err := app.service.SaveUserPreferences(r.Context(), prefs); err != nil { … }
```

Delete `weekdaysToPreferences` entirely — its two callers each inline the
assignments. Remove the `nolint:exhaustruct` along with it. Field-by-field
assignment makes the bug unrepresentable: a future column added to
`domain.Preferences` cannot land in either upsert without being assigned
somewhere.

`preferencesToWeekdays` (the inverse, used by `preferencesGET` and
`scheduleGET`) is untouched.

## Regression tests

Two regression tests, one per handler, both using the existing `e2etest`
pattern:

- `Test_application_preferencesPOST_preservesRestNotificationsEnabled`
  in `cmd/web/handler-preferences_test.go`.
- `Test_application_schedulePOST_preservesRestNotificationsEnabled`
  in `cmd/web/handler-schedule_test.go` (new file — matches the
  project's per-handler test-file convention).

Both follow the same shape:

1. Register a fresh user (defaults `RestNotificationsEnabled = true`).
2. GET `/preferences`, assert the "rest notifications enabled" checkbox
   starts checked (premise check).
3. POST the relevant form with a valid weekday selection.
4. Re-fetch `/preferences` and assert the checkbox is **still** checked.

The form-roundtrip assertion is more faithful to the bug report than a
service-level read.

## Incidental fix

Running `make ci` surfaced a pre-existing `ireturn` lint failure
introduced in commit `9299e02 Wire notification.Scheduler, Sender,
IdleMonitor into main`. `service.Service.ScheduledPushRepo()` returned the
`repository.ScheduledPushRepository` interface, which `ireturn` flags
correctly — but returning the interface is the *right* shape because the
SQLite implementations are intentionally unexported per
`internal/repository/CLAUDE.md`. The feature shipped with a green CI only
because the lint cache masked it.

Replace the narrow accessor with a broader concrete-pointer accessor that
satisfies the lint without a `nolint` directive or restructuring
`NewService`:

```go
func (s *Service) Repos() *repository.Repositories {
    return s.repos
}
```

`*repository.Repositories` is a concrete struct pointer, so `ireturn` is
happy. `main.go` updates from `baseService.ScheduledPushRepo()` to
`baseService.Repos().ScheduledPushes`. The accessor's docstring keeps the
"only intended for main.go" intent that made the original method narrow.

This is the smallest refactor that makes `make ci` pass cleanly without
touching the 20+ `service.NewService(...)` call sites in tests.

## Out of scope

- The other findings from the post-ship review (scheduler races, SW
  deep-link, chip placement, Sender doc lie, VAPID guard, env-var rename,
  test gaps). Each is its own follow-up.
- Splitting `PreferencesRepository.Set` into column-scoped writes. The
  field-by-field assignment in the handler is enough; column-scoped repo
  methods are a larger refactor and deferred.

## Acceptance

- A user can submit any weekday schedule combination and the
  `rest_notifications_enabled` value they previously set is preserved.
- `make ci` passes — including with `exhaustruct` enabled and no new
  `nolint` directives.
- The new test fails on `main` before the fix and passes after.
