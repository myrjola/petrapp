# Fix: weekday-schedule POST silently disables rest notifications

## Motivation

Shipped in the rest-timer series. Any authenticated user who submits the
weekday schedule form at `/preferences` has their `rest_notifications_enabled`
column overwritten to `false`, because:

- `cmd/web/handler-preferences.go:121` constructs a fresh `domain.Preferences{}`
  via `weekdaysToPreferences(r)`, leaving `RestNotificationsEnabled` at Go's
  zero-value `false`.
- `internal/repository/preferences.go:68` upserts `rest_notifications_enabled
  = excluded.rest_notifications_enabled` unconditionally.

The `//nolint:exhaustruct // RestNotificationsEnabled handled by separate
endpoint.` at `handler-preferences.go:78` is exactly the lint hiding the
bug. The comment is false — no separate endpoint protects the value across
this code path. `exhaustruct` was correctly flagging that the partial struct
literal would lose data.

## Fix

Replace the `weekdaysToPreferences(r)` helper with a read-modify-write in
`preferencesPOST`, mirroring `preferencesRestNotificationsTogglePOST`
(`handler-preferences.go:209-225`):

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

Delete `weekdaysToPreferences` entirely — it had one caller. Remove the
`nolint:exhaustruct` along with it. Field-by-field assignment makes the bug
unrepresentable: a future column added to `domain.Preferences` cannot land
in the upsert without being assigned somewhere.

`preferencesToWeekdays` (the inverse, used by `preferencesGET`) is
untouched.

## Regression test

Add to `cmd/web/handler-preferences_test.go` using the existing `e2etest`
pattern:

1. Register a fresh user (defaults `RestNotificationsEnabled = true`).
2. POST `/preferences` with a non-zero weekday schedule (use
   `client.SubmitForm` against the rendered form).
3. GET `/preferences`. Assert the "rest notifications enabled" checkbox is
   still checked.

The form-roundtrip assertion is more faithful to the bug report than a
service-level read.

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
