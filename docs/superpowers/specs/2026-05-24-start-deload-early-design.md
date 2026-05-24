# Start Deload Early — Design

**Date:** 2026-05-24
**Status:** Draft for review
**Type:** Feature design

## Motivation

The planned-deload feature ([deload-periodization-design](2026-05-12-deload-periodization-design.md))
schedules a recovery week at the end of every mesocycle, but the cadence is
fixed. When the user needs recovery off-schedule — after sickness, poor
sleep, or unexpected fatigue — there is no in-app way to switch the current
week to deload mode. The existing "Restart cycle next Monday" button only
snaps the anchor: it starts the new block on an *accumulation* week, which
is the opposite of what a sick user needs.

This design adds a button that flips the current week to deload behaviour
immediately, and extends the existing Restart button to act as its undo.

## Approach summary

A new **"Start deload this week"** button in the Preferences → Recovery
panel posts to `/preferences/mesocycle/start-deload-now`. The handler
calls `Service.StartDeloadNow`, which flips `Session.IsDeload = true` on
each current-week session that is *not yet fully completed and dated
today or later*, then snaps `MesocycleAnchor = nextMonday(...)` so the
following Monday begins a fresh accumulation cycle.

No per-set weight re-seeding is needed. The displayed weight
recommendation is derived live by `Service.BuildProgression`, which
already branches on `sess.IsDeload` to call `GetDeloadStartingWeight`
vs `GetStartingWeight`. Flipping the flag is sufficient.

The existing "Restart cycle next Monday" button is extended to also
clear `IsDeload` on the same forward-looking, non-completed current-week
sessions, so an accidental "Start deload" press is fully reversible.

## Why no per-set mutation

`internal/domain/planning_sets.go:47` (`BuildPlannedSets`) explicitly
leaves `WeightKg = nil` on persisted sets; the docstring calls out
`WeightKg == nil` as the "never recorded" sentinel.
`internal/service/sessions.go:139` (`seedDeloadWeights`) only writes
`WeightKg` at *plan generation time* for sessions that the planner
already marked deload. For mid-week conversion, the next render of the
exercise calls `BuildProgression`
(`internal/service/progression.go:91`), which derives the recommended
weight from `sess.IsDeload` plus history — no read of incomplete-set
`WeightKg`. Flipping the session flag is the entire mutation.

Completed sets in a partially-completed today session retain their
recorded `WeightKg` (the user already lifted those values).

## Why no mutex

`RegenerateWeeklyPlanIfUnstarted` (`internal/service/sessions.go:27`)
uses a per-user mutex to close a check-then-act gap between its
`DeleteWeek` and `generateWeeklyPlan` calls — without a single
enclosing transaction, two concurrent callers could both pass the
"no started session" check and race on the destructive
delete+generate. The comment on that method documents this as a
workaround for the absence of a cross-aggregate transactional
pattern.

`StartDeloadNow` has no comparable gap:

1. `repos.Sessions.List` is a pure-read snapshot used only to
   enumerate candidate dates.
2. Each `repos.Sessions.Update(ctx, date, closure)` is already
   transactional per session; the closure re-reads the latest state
   and the body re-checks `Status() != SessionCompleted` before
   mutating, so the only meaningful race (session completed between
   List and Update) is handled inside the transaction.
3. `repos.Preferences.Set` is a single write; `MesocycleAnchor =
   nextMonday(time.Now().UTC())` is deterministic for any few-second
   window, so concurrent writes converge.

A double-press is idempotent (second `SwitchToDeload` is a no-op).
The small window in which some sessions are flipped but others aren't
(e.g. network blip mid-loop, anchor not yet snapped) is benign: re-press
converges; without re-press, unflipped sessions show normal
recommendations and the next-week anchor change is independent. Real
cross-aggregate atomicity would need a `WithTx` pattern the codebase
does not have today, and adding it for one feature would be out of
proportion to the risk.

## Aggregate methods

Two trivially-bodied methods on `*domain.Session` in
`internal/domain/session.go`:

```go
// SwitchToDeload marks the session as a deload session going forward.
// Sets recorded prior to this call retain their stored values; the next
// progression recommendation will be derived from GetDeloadStartingWeight
// rather than GetStartingWeight. Idempotent.
func (s *Session) SwitchToDeload() error {
    s.IsDeload = true
    return nil
}

// ClearDeload marks the session as a non-deload session going forward.
// Counterpart to SwitchToDeload; used by RestartMesocycleAnchor to undo
// an ad-hoc early deload. Idempotent.
func (s *Session) ClearDeload() error {
    s.IsDeload = false
    return nil
}
```

The bodies are trivial but the named methods match the codebase's
"aggregate methods enforce invariants" pattern and give future
invariants a home (e.g. "cannot deload a fully-completed session"
could move here later).

## Service orchestration

### `StartDeloadNow`

In `internal/service/sessions.go`:

```go
// StartDeloadNow flips IsDeload to true on every current-week session
// dated today or later that is not already fully completed, then snaps
// the mesocycle anchor to next Monday. Used when the user needs
// recovery off-schedule (e.g. returning from sickness). Undone by
// RestartMesocycleAnchor.
func (s *Service) StartDeloadNow(ctx context.Context) error {
    monday := domain.MondayOf(time.Now())
    today := domain.StartOfDay(time.Now())

    sessions, err := s.repos.Sessions.List(ctx, monday)
    if err != nil {
        return fmt.Errorf("list sessions for current week: %w", err)
    }

    for _, sess := range sessions {
        if sess.Date.Before(today) {
            continue
        }
        err = s.repos.Sessions.Update(ctx, sess.Date, func(latest *domain.Session) error {
            if latest.Status() == domain.SessionCompleted {
                return nil
            }
            return latest.SwitchToDeload()
        })
        if err != nil {
            return fmt.Errorf("flip deload for %s: %w", sess.Date.Format(time.DateOnly), err)
        }
    }

    prefs, err := s.repos.Preferences.Get(ctx)
    if err != nil {
        return fmt.Errorf("get preferences: %w", err)
    }
    prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
    if err = s.repos.Preferences.Set(ctx, prefs); err != nil {
        return fmt.Errorf("save preferences: %w", err)
    }
    return nil
}
```

`domain.StartOfDay` is a new helper in `internal/domain/planner.go`
alongside the existing `MondayOf`:

```go
// StartOfDay returns the UTC midnight of date's calendar day. Mirrors
// MondayOf's UTC-anchored-but-calendar-date-from-local behaviour so the
// result compares cleanly against session dates loaded from the
// database.
func StartOfDay(date time.Time) time.Time {
    y, m, d := date.Date()
    return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
```

### Extended `RestartMesocycleAnchor`

In `internal/service/service.go`, extend the existing method to also
clear current-week IsDeload on forward-looking, non-completed sessions:

```go
func (s *Service) RestartMesocycleAnchor(ctx context.Context) error {
    monday := domain.MondayOf(time.Now())
    today := domain.StartOfDay(time.Now())

    sessions, err := s.repos.Sessions.List(ctx, monday)
    if err != nil {
        return fmt.Errorf("list sessions for current week: %w", err)
    }
    for _, sess := range sessions {
        if sess.Date.Before(today) {
            continue
        }
        err = s.repos.Sessions.Update(ctx, sess.Date, func(latest *domain.Session) error {
            if latest.Status() == domain.SessionCompleted {
                return nil
            }
            return latest.ClearDeload()
        })
        if err != nil {
            return fmt.Errorf("clear deload for %s: %w", sess.Date.Format(time.DateOnly), err)
        }
    }

    prefs, err := s.repos.Preferences.Get(ctx)
    if err != nil {
        return fmt.Errorf("get preferences: %w", err)
    }
    prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
    if err = s.repos.Preferences.Set(ctx, prefs); err != nil {
        return fmt.Errorf("save preferences: %w", err)
    }
    return nil
}
```

### Edge case: Restart during a natural-cadence deload week

If the user presses Restart during a *planned* deload week (the
mesocycle's last week, naturally), the rule fires uniformly and
un-flips IsDeload on current+future non-completed sessions. Those
sessions had `set.WeightKg` pre-seeded to the deload weight by
`seedDeloadWeights` at plan time. `BuildProgression` doesn't read
incomplete-set `WeightKg` for its recommendation, so it routes
through `GetStartingWeight` (normal progression) and the displayed
weight switches to the normal recommendation. The stale per-set
`WeightKg` values stay nil-or-seeded but are not surfaced as the
recommendation.

This is an acceptable corner: pressing Restart during a planned
deload week is rare, and the behaviour matches the button's name
("start fresh now").

## HTTP layer

### Route

In `cmd/web/routes.go`, register alongside the existing
`/preferences/mesocycle/restart`:

```go
mux.Handle("POST /preferences/mesocycle/start-deload-now",
    app.mustSessionStack(http.HandlerFunc(app.preferencesStartDeloadNowPOST)))
```

(Matches the middleware stack used by every other `/preferences/*`
route in `routes.go`.)

### Handler

In `cmd/web/handler-preferences.go`:

```go
func (app *application) preferencesStartDeloadNowPOST(w http.ResponseWriter, r *http.Request) {
    if !app.parseForm(w, r, defaultMaxFormSize) {
        return
    }
    if err := app.service.StartDeloadNow(r.Context()); err != nil {
        app.serverError(w, r, fmt.Errorf("start deload now: %w", err))
        return
    }
    redirect(w, r, "/preferences")
}
```

No validation case; the action is parameter-free. Failures route
through `serverError` (catastrophic UX is appropriate — the user has
no input to correct).

## UI

In `ui/templates/pages/preferences/preferences.gohtml`, add a new
form to the Recovery panel between the existing cycle-length form and
the Restart-cycle form:

```gohtml
<form method="post" action="/preferences/mesocycle/start-deload-now" class="panel-actions">
    <button type="submit" class="btn btn--ghost btn--block"
            {{ if not .DeloadEnabled }}disabled{{ end }}>
        Start deload this week
    </button>
</form>
```

No confirmation dialog; gated by `DeloadEnabled` (mirrors the
existing Restart button's gating). Existing Restart button keeps its
copy and position; users discover the undo path through it.

## Testing

### Domain

In `internal/domain/session_test.go`:

- `Test_Session_SwitchToDeload` — sets `IsDeload`; idempotent on
  repeat call; does not touch any other field.
- `Test_Session_ClearDeload` — clears `IsDeload`; idempotent on
  repeat call from an already-cleared state.

### Service

In `internal/service/sessions_test.go`:

- `Test_StartDeloadNow_FlipsTodayAndFutureNonCompleted` — Mon Wed Fri
  schedule, time fixed to Wednesday morning; press button; assert
  Wednesday + Friday flipped, Monday untouched.
- `Test_StartDeloadNow_SkipsCompletedToday` — today fully completed
  before press; today not flipped, future days flipped.
- `Test_StartDeloadNow_PartiallyCompletedTodayFlips` — today started
  with one set completed and one set incomplete; today flipped (the
  remaining set will get the deload recommendation via
  `BuildProgression`).
- `Test_StartDeloadNow_SnapsAnchorToNextMonday` — anchor after press
  equals `nextMonday(now)`.
- `Test_StartDeloadNow_Idempotent` — pressing twice produces the
  same state as pressing once.
- `Test_StartDeloadNow_BuildProgressionUsesDeloadWeight` — after
  press, `BuildProgression` for an affected session returns a
  `CurrentSetTarget` whose weight equals `GetDeloadStartingWeight`.

In `internal/service/service_test.go`:

- `Test_RestartMesocycleAnchor_ClearsCurrentWeekDeload` — flip via
  `StartDeloadNow`, then `RestartMesocycleAnchor`, then assert
  forward-looking non-completed current-week sessions have
  `IsDeload == false`.
- `Test_RestartMesocycleAnchor_LeavesCompletedSessionsAlone` —
  natural-cadence deload week, today fully completed; press
  Restart; today's session stays as-is (we only walk non-completed).

### HTTP

In `cmd/web/handler-preferences_test.go`:

- `Test_PreferencesStartDeloadNowPOST_FlipsAndRedirects` — submit
  the new form via the e2etest client; assert redirect to
  `/preferences` and a subsequent `GET /workouts/<future-date>`
  shows the deload weight recommendation.
- `Test_PreferencesStartDeloadNowPOST_DisabledWhenDeloadOff` —
  assert the button renders with `disabled` attribute when
  `DeloadEnabled == false`.

## File touches

- `internal/domain/session.go` — `SwitchToDeload`, `ClearDeload`
  methods.
- `internal/domain/session_test.go` — aggregate-method tests.
- `internal/domain/` (where `MondayOf` lives) — `StartOfDay` helper
  if not already present.
- `internal/service/sessions.go` — `StartDeloadNow` method.
- `internal/service/sessions_test.go` — service tests.
- `internal/service/service.go` — extend
  `RestartMesocycleAnchor`.
- `internal/service/service_test.go` — extended-restart tests.
- `cmd/web/handler-preferences.go` —
  `preferencesStartDeloadNowPOST` handler.
- `cmd/web/handler-preferences_test.go` — handler tests.
- `cmd/web/routes.go` — `POST /preferences/mesocycle/start-deload-now`
  route registration.
- `ui/templates/pages/preferences/preferences.gohtml` — new form in
  Recovery panel.

## Out of scope

- A dedicated "Undo: resume normal week" button. The extended
  Restart button covers the undo case symmetrically.
- Cross-aggregate transactional pattern (`WithTx`) for atomic
  multi-session + preferences writes. Per-session transactions plus
  idempotency are sufficient for this feature's risk profile.
- Surfacing the button on the home/schedule page. Recovery controls
  stay together in Preferences.
- Auto-enabling `DeloadEnabled` on press. The button is gated by the
  existing preference; users who want recovery must first opt into
  deload.
- Mid-week conversion that retroactively re-characterises a
  partially-completed *past* session (before today) as a deload
  session. The intent is forward-looking only.
