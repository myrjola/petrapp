# WeekPlan aggregate: removing the per-user mutex via correct aggregate boundaries

## Background

`internal/service/service.go` carries a per-user `sync.Map` of
`*sync.Mutex` (`userLocks`) used by `RegenerateWeeklyPlanIfUnstarted` to
serialise a `list → check-started → delete-week → generate → insert-batch`
sequence across multiple transactions. The doc-comment is honest about
the design's limits:

> Multi-process deployments would need a different scheme (advisory lock
> or single-row sentinel); today's deployment is single-machine.

The mutex protects `Regenerate` against itself but **does not** protect
it against `StartSession`. The classic check-then-act race still exists:
`Regenerate` reads "no session started," `StartSession` commits a
started session, `Regenerate` deletes the week including the
just-started row. `ResolveWeeklySchedule`'s self-heal masks this after
the fact.

The mutex is a symptom of a wrong aggregate boundary. Today the only
aggregate is `Session` (one day). The Planner's output (`[]Session`)
and the cross-week operations (`RegenerateWeeklyPlanIfUnstarted`,
`StartDeloadNow`, `RestartMesocycleAnchor`) all manipulate the *week*,
but the week has no aggregate root — the operations iterate over
per-day `Session.Update` closures and need an out-of-band lock to look
correct.

## Goal

Introduce `WeekPlan` as the aggregate root for everything that scopes
to a calendar week: planning, periodization, deload flips, and
per-session lifecycle + per-set logging. Collapse the cross-week
service methods into single `WeekPlanRepository.Update` closures.
Delete `userLocks`, `userMutex`, and the `sync.Map` field.

After this work:

- `RegenerateWeeklyPlanIfUnstarted` is one atomic transaction. The
  `Regenerate` vs `Regenerate` AND `Regenerate` vs `StartSession`
  races are both closed by `BEGIN IMMEDIATE` at the connection layer.
- No in-process mutex; correctness comes from the DB.
- Service methods that mutate a session become one-line closures over
  `WeekPlanRepository.Update`.
- The "self-heal via `ResolveWeeklySchedule`" defence-in-depth
  remains for the cold-start case (first request of a new week).

## Why one aggregate, not two

A natural-sounding alternative is `WeekPlan` (plan structure +
lifecycle) + `ExerciseLog` (per slot-per-day actuals). It was
rejected because:

- **No true cross-aggregate invariant.** Plan ops only touch the
  plan; log ops only touch logs; lifecycle touches the plan.
  `Swap` is the one cross-axis operation, and "wipe actuals when the
  slot's exercise changes" is a row-rewrite consequence, not a
  protected invariant.
- **Two aggregates over one table is a smell.** `exercise_sets`
  carries both target columns (`target_value`, `weight_kg`) and
  actual columns (`completed_value`, `completed_at`, `signal`).
  Splitting these into different aggregate boundaries cuts across a
  natural seam — the schema author already concluded they belong
  together.
- **Lock cost is irrelevant at this scale.** Single user, single
  device. A week-scoped write lock holds for ~5 ms.
- **Read views are unaffected.** Aggregates are write boundaries; the
  workout page can join across whatever it needs via flat SQL.

If planning and logging ever genuinely diverge (a coach plans for a
trainee; multi-device concurrent logging), revisit. Not today.

## The aggregate

```text
WeekPlan
├── UserID, Monday
├── PeriodizationType   // per-week (research: weekly undulation; lifted from per-day denormalisation)
└── Sessions [7]Session
    Session
    ├── Date
    ├── StartedAt, CompletedAt
    ├── DifficultyRating *int
    ├── IsDeload bool         // per-day; StartDeloadNow flips remaining days only
    └── ExerciseSets []ExerciseSet
        ExerciseSet
        ├── ID                // stable slot ID, repo-assigned
        ├── Exercise
        ├── WarmupCompletedAt *time.Time
        └── Sets []Set        // {TargetValue, WeightKg, CompletedValue, CompletedAt, Signal}
```

### Lifted fields, kept fields

- **`PeriodizationType` → WeekPlan.** Today on `Session`, but
  `Planner.firstSessionPeriodizationType` alternates week-to-week and
  every session in a week shares the value. This matches the
  Renaissance Periodization framework PetrApp's vocabulary already
  uses (weekly undulating periodization). Storage stays denormalised
  per the decision below; the WeekPlan domain method writes the same
  value into every `Session.PeriodizationType` field.
- **`IsDeload` stays per-day.** Justified by `StartDeloadNow`, which
  flips only days `>= today` to avoid retroactively relabeling
  already-performed sessions. Week-level "is this a deload week?"
  is derivable via `WeekPlan.IsDeloadWeek()` (all scheduled days
  carry `IsDeload=true`) or — for next-week generation — computed
  from `(MesocycleAnchor, monday, cycleLen)`.

### Methods

Cross-week (atomic on WeekPlan):
- `AnyStarted() bool`
- `RegenerateIfUnstarted(now, planner) error` — no-op when
  `AnyStarted()`; otherwise replaces `*wp` with `planner.Plan(monday)`.
- `FlipDeloadFromToday(today time.Time) error` — sets `IsDeload=true`
  on every non-completed session with `Date >= today`.
- `ClearDeloadFromToday(today time.Time) error` — counterpart.
- `IsDeloadWeek() bool` — derived display helper.

Per-day dispatchers (navigate to the right `Session`, delegate to a
helper method or to the existing logic on the nested `Session` struct):
- `Start(date, now)`, `Complete(date, now)`, `SetDifficulty(date, rating)`
- `MarkWarmupComplete(date, slotID, now)`
- `RecordSet(date, slotID, setIdx, signal, weight, value, now)`
- `UpdateCompletedValue(date, slotID, setIdx, value, now)`
- `UpdateSetWeight(date, slotID, setIdx, weight)`
- `Swap(date, slotID, newEx, sets)`
- `AddExercise(date, ex, sets)`

The nested `Session` struct keeps its internals (`findSlot`,
`setAt`, etc.) so each dispatcher is a one-liner.

## Persistence

**No schema change.** WeekPlan is a logical aggregate over the
existing three tables:

| Table | Holds | Per-week count |
|---|---|---|
| `workout_sessions` | One row per scheduled `Session` | up to 7 |
| `workout_exercise` | One row per `ExerciseSet` slot | ~5 × scheduled days |
| `exercise_sets` | One row per `Set` | ~3 × slots |

`PeriodizationType` keeps its existing `workout_sessions.periodization_type`
column; the WeekPlan repo writes the same value to every day's row
when persisting (invariant enforced at the repo boundary). A
`weekly_plans (user_id, monday, periodization_type)` parent table was
considered and rejected — it would add a premigration for ~0
functional benefit.

**Lazy materialisation matches today.** No `workout_sessions` row exists
for a week until the user triggers planning. Rest days have no row.
The repo synthesises empty `Session{Date: ...}` for non-scheduled
dates so `WeekPlan.Sessions` is always length 7 in memory.

**Load** (`WeekPlanRepository.Get(ctx, monday)`):
3 read queries — sessions for `monday..sunday`, the existing
`loadExerciseSetsSince` join over the week's slots, batched
muscle-group hydration. Same shape as `SessionRepository.List(monday)`
today.

**Save** (`WeekPlanRepository.Update` commit path):
`BEGIN IMMEDIATE` → `DELETE FROM workout_sessions WHERE user_id=? AND
workout_date BETWEEN ? AND ?` (CASCADE clears `workout_exercise` and
`exercise_sets`) → re-insert all sessions, slots
(`INSERT ... RETURNING id` to preserve slot IDs), sets → `COMMIT`.
Worst case ~150 rows × single fsync ≈ 5 ms. Per-set logging hits this
~30 times per workout → ~150 ms cumulative, imperceptible.

**Slot ID stability.** Same trick `Session.Update` uses today —
pre-existing `ExerciseSet.ID` values are passed back into
`INSERT ... RETURNING id` so URLs and `scheduled_push.workout_exercise_id`
FKs continue to resolve.

## Repository contract

```go
type WeekPlanRepository interface {
    // Get returns the (lazily-materialised) week. Sessions are length 7;
    // non-scheduled dates carry an empty Session{Date: ...}. Returns
    // ErrNotFound when no workout_sessions row exists for the week yet.
    Get(ctx context.Context, monday time.Time) (WeekPlan, error)

    // Update loads the week, runs fn under a single transaction, and
    // commits the diff on nil. Sentinel errors from domain methods
    // propagate unchanged. Closure pattern lifted from SessionRepository.Update.
    Update(ctx context.Context, monday time.Time, fn func(*WeekPlan) error) error

    // Create persists a freshly-planned week. Returns ErrAlreadyExists
    // (wrapped) on PK conflict so concurrent first-time generation races
    // can recover by re-reading.
    Create(ctx context.Context, plan WeekPlan) error
}
```

`SessionRepository` **write surface** (`Update`, `Create`, `CreateBatch`,
`DeleteWeek`) is deleted in step 6 of the rollout.

**Read surface stays.** `internal/service/reporting.go` calls
`Sessions.List(since)` (multi-week aggregation over volume) and
`Sessions.ListSetsForExerciseSince(...)` (cross-week progression
history). Routing those through `WeekPlanRepository.Get` would
require loading 10+ WeekPlans to compute one volume rollup — wasteful
when a flat join already exists. Keep `Sessions.Get(date)`,
`Sessions.List(since)`, and `Sessions.ListSetsForExerciseSince` as
read-only methods; delete the writes. Optional follow-up: rename the
interface to `SessionView` to make the read-only-ness explicit; out
of scope here.

The one per-day caller (`ResolveWeeklySchedule` itself, plus per-day
handlers like `GET /workouts/{date}`) migrates to
`WeekPlanRepository.Get(MondayOf(date))` + `plan.SessionOn(date)`.
GDPR export already touches `*sqlite.Database` directly and is
unaffected.

## Service layer changes

| Today | After |
|---|---|
| `s.userLocks *sync.Map`, `s.userMutex(...)`, `mu.Lock/Unlock` | **Deleted.** `WeekPlanRepository.Update` is the atomicity boundary. |
| `RegenerateWeeklyPlanIfUnstarted` (list + mutex + delete + generate + create-batch, ~30 lines) | One `WeekPlans.Update` closure: `if wp.AnyStarted() { return nil }; *wp = planner.Plan(...); return nil` |
| `StartDeloadNow` (list + per-session Update loop + prefs save) | One `WeekPlans.Update` closure for the flip + the existing prefs save (two txs, but only the flip needs atomicity) |
| `RestartMesocycleAnchor` (list + per-session Update loop + prefs save) | Symmetric to `StartDeloadNow`. |
| `StartSession`, `CompleteSession`, `RecordSet`, `MarkWarmupComplete`, `UpdateSetWeight`, `UpdateCompletedValue`, `SwapExercise`, `AddExercise`, `SaveFeedback` | Each becomes a one-line `WeekPlans.Update(MondayOf(date), fn)` where `fn` calls the matching WeekPlan dispatcher. |
| `ResolveWeeklySchedule` (list + lazy `generateWeeklyPlan` + per-day `Sessions.Get` loop) | `WeekPlans.Get`; on `ErrNotFound`, plan + `Create`; tolerate `ErrAlreadyExists` and re-read. |

The `nextMonday` / `MondayOf` helpers and the existing `domain.Planner`
keep their current shapes; only `Planner.Plan` changes its return type
from `[]domain.Session` to `domain.WeekPlan`. `Planner.PlanDay`
continues to return a single `domain.Session` for the ad-hoc path.

## Rollout — single PR, ordered commits

Each commit must leave `make test` green so the wide refactor bisects
cleanly.

1. **Domain.** Add `WeekPlan` with all methods. Lift `PeriodizationType`
   field to `WeekPlan`. Keep `Session` as the per-day nested struct
   (no rename). No callers yet.
2. **Planner.** Change `Planner.Plan(monday)` return type to
   `WeekPlan`. Fix planner internal tests.
3. **Repository.** Add `WeekPlanRepository` ({`Get`, `Update`, `Create`}).
   Reuse `scanExerciseSetRows`, `loadExerciseSetsSince`. Wire into
   `Repositories` struct. Round-trip + closure-rollback tests.
4. **Service migration** (one method per commit). Read-only first
   (`ResolveWeeklySchedule`), then per-day mutations (`StartSession`,
   `CompleteSession`, `RecordSet`, `MarkWarmupComplete`,
   `UpdateSetWeight`, `UpdateCompletedValue`, `SwapExercise`,
   `AddExercise`, `SaveFeedback`), then cross-week
   (`RegenerateWeeklyPlanIfUnstarted`, `StartDeloadNow`,
   `RestartMesocycleAnchor`). Service tests must pass after each.
5. **Delete the mutex.** Remove `userLocks`, `userMutex`, the
   `sync.Map` field, the `sync` import from `service.go`. Update doc
   comments to reflect the new atomicity story.
6. **Delete `SessionRepository` writes.** Drop `Update`, `Create`,
   `CreateBatch`, `DeleteWeek` from the interface and the SQLite
   implementation. Keep the read methods (`Get`, `List`,
   `ListSetsForExerciseSince`) — `reporting.go` needs them.

`make ci` is the gate at the end.

## Testing

- **Domain — `internal/domain/week_plan_test.go` (new).** Per-day
  delegation correctness + cross-week invariants. Key cases:
  `RegenerateIfUnstarted` is a no-op when any session has `StartedAt`;
  `FlipDeloadFromToday` skips past days; `IsDeloadWeek` reflects all
  scheduled days; `PeriodizationType` change propagates to every
  `Session.PeriodizationType` on persist.
- **Repository — `internal/repository/week_plan_test.go` (new).**
  Round-trip persistence; `Update` closure commits on nil and rolls
  back on returned error; slot ID stability through full-week
  rewrite (load, mutate one set, save, reload, assert slot IDs
  unchanged); lazy materialisation (rest days return empty
  `Session{Date: ...}`); `Create` → `ErrAlreadyExists` on PK conflict.
- **Service.** Existing assertions in `sessions_test.go`, `sets_test.go`,
  `feature_flags_test.go` continue to hold — public signatures don't
  change. **Add one race test** in `sessions_test.go` exercising
  concurrent `RegenerateWeeklyPlanIfUnstarted` + `StartSession`
  goroutines that today's mutex does *not* serialise. Proves the
  redesign closes the documented race window. Without this, the
  spec's main correctness claim is unverified.
- **Handler tests.** Unchanged; verified by passing `make ci`.

## Out of scope

- **Mesocycle anchor flexibility.** Loosening `Preferences.MesocycleAnchor`
  so it can land mid-week (currently snapped to Monday by
  `SaveUserPreferences` and `nextMonday`) is a separate, independent
  change. WeekPlan stays Monday-anchored regardless. Track separately
  if there's appetite.
- **Schema refactor.** No new tables; `PeriodizationType` stays
  denormalised on `workout_sessions`. The `weekly_plans` parent-row
  option was considered and rejected above.
- **Read-side splits / CQRS.** Reporting, history, GDPR export
  continue to use flat SQL. No view layer / projection introduced.
- **Renaming `ExerciseSet` → `Slot`** or similar terminology cleanups.
  Out of scope to keep diff bounded.
