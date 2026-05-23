# Rest-push policy — design

## Problem

The current Web Push scheduler (`internal/service/sets.go::maybeSchedulePush`,
backed by `internal/notification/scheduler.go`) has two defects and one
missing trigger:

1. **Wrong "last set" gate.** The schedule decision uses
   `sess.HasIncompleteSets()` — a session-wide predicate. When the user
   completes the last set of one exercise while *other* exercises still have
   work to do, a push gets scheduled for the just-finished exercise with a
   nonsense payload (`Time for set 4 of 3 — Squat`).

2. **No cancel when a slot becomes fully complete.** Once every set in an
   exercise is done, any pending push for that exercise should be dropped.
   Today nothing actively cancels — we rely on the bug above to keep
   scheduling, which masks the issue but produces wrong payloads.

3. **No notification after warmup completion.** `MarkWarmupComplete` records
   the timestamp but never schedules a push. The user wants a notification
   for set 1 the same way they get one between sets.

The scheduling logic also lives entirely inside the service layer
(`maybeSchedulePush`) and mixes pure rules (when to schedule, what time, what
payload) with I/O (preferences, subscriptions, persistence). Extracting the
pure rule makes it testable and lets the service stay focused on
orchestration.

## Goals

- After warmup-complete on a slot with incomplete sets → schedule a push for
  the first incomplete set.
- After completing a non-last set in a slot → schedule a push for the next
  incomplete set.
- After completing the last set of a slot → cancel any pending push for that
  slot.
- Multiple slots being worked in rotation (power sets) → each slot has its
  own independent timer, identical to today's behavior.
- Batch completion of multiple sets in the same slot → last write wins (one
  pending timer per slot, identical to today's behavior via the
  `UNIQUE(workout_exercise_id)` index).
- Editing already-completed sets (`UpdateCompletedValue`, `UpdateSetWeight`)
  → no scheduling, no cancellation. Bookkeeping is not progress.
- Re-clicking a trigger that has already fired (re-recording a complete set,
  re-clicking warmup-complete) → no-op.

## Non-goals

- Configurable warmup-to-set-1 delay. We reuse `RestSecondsFor` for both
  triggers; if a different value is wanted later, this is the seam to add
  it.
- Cross-device notification suppression (multiple subscribed devices still
  all receive each push — unchanged).
- Snoozing or dismissing notifications from the app.
- Migrating away from `internal/notification/` to a new package. The
  scheduler/sender/idle-monitor split stays.

## Design

### Pure rule in `internal/domain/rest_push.go` (new)

A pure helper takes the post-mutation slot and decides what the scheduler
should do. No I/O, no service dependencies.

```go
package domain

type RestPushAction int

const (
    RestPushActionNoOp RestPushAction = iota
    RestPushActionSchedule
    RestPushActionCancel
)

type RestPushPayload struct {
    Title         string
    Body          string
    ExerciseName  string
    NextSetNumber int
    SetsTotal     int
}

type RestPushDecision struct {
    Action  RestPushAction
    FireAt  time.Time       // only set when Action == Schedule
    Payload RestPushPayload // only set when Action == Schedule
}

// PlanRestPush inspects the slot after a state change and decides what the
// push scheduler should do. completedAt is the moment the mutation
// happened — used as the rest-clock zero point.
func PlanRestPush(
    slot ExerciseSet,
    periodization PeriodizationType,
    isDeload bool,
    completedAt time.Time,
) RestPushDecision
```

The rule:

1. Find the first set in `slot.Sets` whose `CompletedAt == nil`.
2. None found → `RestPushActionCancel`. Every set in the slot is done.
3. `RestSecondsFor(slot.Exercise, periodization, isDeload) <= 0` →
   `RestPushActionNoOp`. No rest defined for this exercise.
4. Otherwise → `RestPushActionSchedule` with:
   - `FireAt = completedAt + RestSecondsFor(...)`
   - `Payload.NextSetNumber = index of first incomplete + 1`
   - `Payload.SetsTotal = len(slot.Sets)`
   - `Payload.Title = "Rest over"`
   - `Payload.Body = fmt.Sprintf("Time for set %d of %d — %s", NextSetNumber, SetsTotal, slot.Exercise.Name)`
   - `Payload.ExerciseName = slot.Exercise.Name`

This is the same set of fields the current implementation marshals into JSON,
preserved byte-for-byte so the service worker (`ui/static/sw.js`) keeps
working unchanged.

The rule is uniform across the two triggers (warmup-complete and
set-complete) because "first incomplete set" answers both questions. The
caller decides whether to invoke; the rule itself doesn't branch on the
trigger.

### Service wiring in `internal/service/`

A new private helper in `sets.go`:

```go
func (s *Service) applyRestPushDecision(
    ctx context.Context,
    userID, workoutExerciseID int,
    slot domain.ExerciseSet,
    periodization domain.PeriodizationType,
    isDeload bool,
    completedAt time.Time,
)
```

The helper:

1. Short-circuit `s.scheduler == nil`.
2. `decision := domain.PlanRestPush(slot, periodization, isDeload, completedAt)`.
3. Branch on `decision.Action`:
   - `NoOp` → return.
   - `Cancel` → `s.scheduler.Cancel(ctx, workoutExerciseID)`; log warn on
     error, do not propagate (the user-facing mutation already committed).
   - `Schedule` → check `prefs.RestNotificationsEnabled` and
     `PushSubscriptions.CountByUser(ctx) > 0`. If either is false, return.
     Otherwise marshal `decision.Payload` + `decision.FireAt.UnixMilli()`
     into the JSON shape consumed by `sw.js`, build
     `domain.ScheduledPush{UserID, WorkoutExerciseID, FireAt, Payload}`,
     and call `s.scheduler.Schedule(ctx, push)`. Log warn on any I/O
     failure; do not propagate.

The preferences/subscription gate stays out of the pure rule — those are
boundary checks, not part of the decision logic, and they require I/O.

#### Call sites

**`sets.go::RecordSet`** — replace the existing `maybeSchedulePush` call.

Inside the `Sessions.Update` closure:

```go
var (
    wasComplete    bool
    postSlot       domain.ExerciseSet
    postSlotOK     bool
    periodization  domain.PeriodizationType
    sessionDeload  bool
)

err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
    if slot, ok := sess.Slot(workoutExerciseID); ok && setIndex >= 0 && setIndex < len(slot.Sets) {
        wasComplete = slot.Sets[setIndex].CompletedAt != nil
    }
    periodization = sess.PeriodizationType
    sessionDeload = sess.IsDeload

    if recErr := sess.RecordSet(workoutExerciseID, setIndex, signal, weightKg, completedValue, now); recErr != nil {
        return recErr //nolint:wrapcheck
    }
    postSlot, postSlotOK = sess.Slot(workoutExerciseID)
    return nil
})
if err != nil {
    return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
}

if !wasComplete && postSlotOK {
    userID := contexthelpers.AuthenticatedUserID(ctx)
    s.applyRestPushDecision(ctx, userID, workoutExerciseID, postSlot, periodization, sessionDeload, now)
}
```

The `!wasComplete` guard prevents re-recording an already-complete set from
firing a new decision. Combined with the policy returning `Cancel` when the
slot has just been fully completed, this gives the desired behavior:

| Pre-state           | Action this call          | Decision   |
|---------------------|---------------------------|------------|
| Set was incomplete  | Becomes the new last done | `Cancel`   |
| Set was incomplete  | Other sets still pending  | `Schedule` |
| Set was complete    | Re-record (no transition) | no call    |

**`sessions.go::MarkWarmupComplete`** — extend with the same pattern.
`now` is captured once at the top of the method (as `RecordSet` already
does) so the same instant is used for the domain mutation and the
`FireAt` computation.

```go
now := time.Now().UTC()
var (
    wasComplete   bool
    postSlot      domain.ExerciseSet
    postSlotOK    bool
    periodization domain.PeriodizationType
    sessionDeload bool
)

err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
    if slot, ok := sess.Slot(workoutExerciseID); ok {
        wasComplete = slot.WarmupCompletedAt != nil
    }
    periodization = sess.PeriodizationType
    sessionDeload = sess.IsDeload

    if mErr := sess.MarkWarmupComplete(workoutExerciseID, now); mErr != nil {
        return mErr //nolint:wrapcheck
    }
    postSlot, postSlotOK = sess.Slot(workoutExerciseID)
    return nil
})
if err != nil {
    return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
}

if !wasComplete && postSlotOK {
    userID := contexthelpers.AuthenticatedUserID(ctx)
    s.applyRestPushDecision(ctx, userID, workoutExerciseID, postSlot, periodization, sessionDeload, now)
}
```

#### Non-trigger paths

These mutate slot state but **do not** call `applyRestPushDecision`:

- `UpdateCompletedValue` — edits the recorded value on an already-complete
  set. The set was already complete before the call and stays complete
  after; no transition.
- `UpdateSetWeight` — edits the weight on a set. Doesn't change completion
  state.
- `SwapExerciseInSlot` — already calls `scheduler.Cancel` separately (see
  `exercises.go`). The slot's sets are wiped on swap, so the policy would
  see no completed sets and return `Schedule` for set 1 — wrong, because
  the user hasn't started the new exercise.
- `CompleteSession` — already calls `scheduler.CancelForWorkout`.

The rule: only call `applyRestPushDecision` when a mutation transitions the
slot toward completion (warmup `nil → set`, or a set `incomplete → complete`).
A regression test guards this for `UpdateCompletedValue` and
`UpdateSetWeight`.

### What stays unchanged

- `internal/notification/scheduler.go` — `Schedule` / `Cancel` /
  `CancelForWorkout` / `Reload` / `PendingCount`. The UNIQUE-by-
  `workout_exercise_id` index in `scheduled_pushes` still gives us per-slot
  dedup for free.
- `internal/notification/sender.go` — VAPID send, 410/404 → prune.
- `internal/notification/idle_monitor.go` — scale-to-zero gating.
- `ui/static/sw.js` — payload shape on the wire is preserved.
- Database schema — no migrations.

## Edge cases (accepted, not fixed)

1. **Cancel-vs-fire race.** If a timer is already inside `Scheduler.fire()`
   when `Cancel` runs, the dispatch goes through anyway — `timer.Stop()`
   doesn't unwind an already-running callback. The user may see one stale
   "Time for set N of N" notification immediately after marking the last
   set complete. The 60s Web Push TTL caps the damage. Fixing this would
   require a slot-state re-check inside `fire()`, more complexity than the
   rare race warrants.

2. **Out-of-order completions** (set 3 recorded before set 2 for any
   reason). `PlanRestPush` returns based on "first incomplete," so the
   payload announces set 2. Correct by construction.

3. **Re-clicking warmup-complete after sets have started.** The
   `!wasComplete` guard suppresses the push decision. The underlying domain
   mutation (`WarmupCompletedAt = &now`) still runs, preserving today's
   behavior — only the rest-push side is gated.

## Testing

### `internal/domain/rest_push_test.go` (new, table-driven, pure unit)

| Case                                                         | Expect            |
|--------------------------------------------------------------|-------------------|
| Empty slot                                                   | `Cancel`          |
| All sets complete                                            | `Cancel`          |
| First incomplete = set 1 (warmup just done, no sets started) | `Schedule`, set 1 |
| Mid-exercise: sets 1–2 done, 3 pending                       | `Schedule`, set 3 |
| Last incomplete just completed                               | `Cancel`          |
| `RestSecondsFor` returns 0                                   | `NoOp`            |
| Deload session uses deload rest                              | `Schedule`        |
| Payload `Title`/`Body` byte-equal to current marshalled form | matches           |

The last row guards the wire-format contract with `sw.js`.

### `internal/service/sets_test.go` (extend, real DB + `fakeScheduler`)

- **Regression:** completing the last set of slot A1 while slot A2 still has
  incomplete sets → fake records exactly one `Cancel(A1)`, zero `Schedule`.
  Today this scenario produces a `Schedule` with a "set 4 of 3" payload.
- **Power-set rotation:** completing set 1 of A1 then set 1 of A2 → two
  `Schedule` calls with distinct `workoutExerciseID`s, no `Cancel`. Already
  works today; the assertion locks it in.
- **Edit-completed-set invariant:** record set 1, snapshot fake-scheduler
  call count, then call `UpdateCompletedValue` and `UpdateSetWeight` on that
  set → call count unchanged. Guards future contributors from adding
  scheduling to those paths.
- **Re-record completed set:** record set 1, then call `RecordSet` again on
  the same set with new values → second call does not produce a new
  `Schedule` or `Cancel`. Guards the `!wasComplete` check.

### `internal/service/sessions_test.go` (extend)

- `MarkWarmupComplete` with no subscriptions → fake records zero
  `Schedule`/`Cancel`.
- `MarkWarmupComplete` first time, sets exist and are all incomplete → one
  `Schedule` with `NextSetNumber = 1`.
- `MarkWarmupComplete` when set 1 already complete → one `Schedule` with
  `NextSetNumber = 2`. (Intentional — the rule is uniform; warmup-complete
  always plans for the first *incomplete* set.)
- `MarkWarmupComplete` clicked twice in a row → second call produces no
  additional scheduler interaction (the `!wasComplete` guard).
- `MarkWarmupComplete` on a slot where every set is already complete → one
  `Cancel`, no `Schedule`.

## Migration

None. No schema changes, no feature flags, no data backfill. Any pre-existing
"Time for set N+1 of N" pending push survives until it fires (60s TTL on the
Web Push side) or the user finishes the session and `CompleteSession` calls
`CancelForWorkout`.
