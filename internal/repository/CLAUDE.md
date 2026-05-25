# Repository — SQLite Persistence

The `internal/repository` package is the canonical persistence layer for
the workout bounded context. It depends on `internal/domain`,
`internal/sqlite`, and `internal/contexthelpers` only — no HTTP, no
template logic, no business orchestration.

## What lives here

- **SQLite implementations** of the eight repository contracts:
  `WeekPlanRepository`, `SessionRepository`, `ExerciseRepository`,
  `PreferencesRepository`, `FeatureFlagRepository`,
  `MuscleGroupTargetRepository`, `PushSubscriptionRepository`,
  `ScheduledPushRepository`. Implementations are unexported
  (`sqliteWeekPlanRepository`, `sqliteSessionRepository`, etc.).
  `WeekPlanRepository` owns workout writes at week scope; `SessionRepository`
  is read-only (see "Update closure contract" below).
- **The `Repositories` composite struct** plus the single public
  constructor `New(db *sqlite.Database) *Repositories` that wires every
  repository together.
- **Shared helpers** in `shared.go`:
  - Format primitives — `parseTimestamp`, `formatTimestamp`, `formatDate`,
    the `queryer` interface, the `baseRepository` mixin, and the
    timestamp/date format constants.
  - `fetchMuscleGroupsByExerciseID` — batched muscle-group hydration
    shared by the exercise and session reads.
  - Session-shaped persistence on `baseRepository` (write helpers in
    `shared.go`, read helpers in `sessions.go`): `insertSessionRowInTx`,
    `saveOneSlotInTx`, `saveExerciseSetsInTx` (per-session two-pass:
    explicit-ID slots before auto-ID slots), `insertSessionInTx` (the
    composite of row + sets), `deleteWeekInTx`, plus read-side
    `listSessionRows`, `listSessionRowsBetween`, and
    `loadExerciseSetsSince`. Shared by `WeekPlanRepository.Update`'s
    three-pass reinsert and the `SessionRepository` reads. If a third
    repository starts using them, consider splitting into a dedicated
    `session_persistence.go`.

## What does NOT live here

- Business rules — those live as aggregate methods on `domain.Session`
  (or as pure functions in the domain package).
- HTTP handlers, template shaping, response serialisation — `cmd/web`.
- Service orchestration, AI exercise generation, GDPR export, anything
  that combines multiple aggregates or external systems —
  `internal/service`.
- Tests of business behaviour — those belong in `internal/domain` (pure
  unit) or `internal/service` (orchestration/e2e). Repository tests
  cover repository-shape contracts: round-trip persistence, error
  translation, slot-ID stability across `Update`.

## Update closure contract

`WeekPlanRepository.Update` and `ExerciseRepository.Update` accept a
closure `func(*domain.X) error` that runs inside an open transaction:

- The repo loads the aggregate (hydrating exercise data for sessions),
  runs the closure, persists the result on `nil`, rolls back on error.
- The closure expresses business invariants by calling aggregate methods
  on `domain.WeekPlan` / `domain.Session` (e.g. `wp.Start(date, now)`);
  domain sentinels (`ErrAlreadyStarted`, `ErrSlotNotFound`, etc.)
  propagate to the caller unchanged so service-layer code can
  `errors.Is` against them.
- Returning a non-domain error rolls back too — the repo doesn't try to
  classify; service code decides whether the error is fatal.

`SessionRepository` is read-only — every write that touches workout
data goes through `WeekPlanRepository.Update`, which holds the
transactional boundary at week scope.

## Diff strategy: delete-and-reinsert

`WeekPlanRepository.Update` persists by deleting every
`workout_sessions` row in `[monday, monday+6]` inside the tx (CASCADE
clears `workout_exercise` and `exercise_sets`) and re-inserting the
sessions across three passes — session rows, explicit-ID slots, then
auto-assign slots — so SQLite's rowid auto-assignment never collides
with a preserved `workout_exercise.id`. Pre-existing `ExerciseSet.ID`
values are passed back into `INSERT ... RETURNING id` so URL-stable
slot IDs survive the cycle. New slots (ID == 0) get auto-assigned IDs.
For PetrApp's data sizes (a handful of exercises × a handful of sets
per session) the cost is negligible and the simplicity is worth the
trade.

## Hydration policy

`SessionRepository.Get` and `List` always populate `ExerciseSet.Exercise`
inline: the base exercise columns are joined in via
`workout_exercise.exercise_id → exercises.id`, and primary/secondary
muscle groups are fetched in a single follow-up query keyed by the
deduped exercise IDs. `Get` costs two read queries on top of the session
row (the sets+exercise join plus the batched muscle-group fetch).
`List` stays flat regardless of how many sessions it returns: one query
for the session rows, one batched sets+exercise join over the whole date
range, and one muscle-group query — three queries total, not the prior
1 + 2N per session (see `loadExerciseSetsSince`). Callers receive a
"fully hydrated" `domain.Session` and never need to enrich it themselves.

## ErrNotFound translation at the boundary

Every read method translates `sql.ErrNoRows` to `domain.ErrNotFound`
explicitly. Callers `errors.Is(err, domain.ErrNotFound)` to detect
missing rows; they never see `sql.ErrNoRows` directly. This is the
symbolic move that ends "persistence leaks into the domain": the domain
sentinel has no SQL ancestry.

## Adding a new repository

1. Declare the interface in `internal/repository/repository.go`.
2. Add the SQLite implementation in a new file (e.g. `widgets.go`) with
   an unexported struct `sqliteWidgetRepository` and an unexported
   constructor `newSQLiteWidgetRepository`.
3. Wire the new repo into `Repositories` and `New`.
4. Add round-trip tests in `widgets_test.go` (external `package
   repository_test`) using the `setupTestRepos` helper.

## Testing

Repository tests live in `package repository_test` (external) and run
against a real in-memory SQLite database — no mocks. They verify the
SQL-shaped contract, not the business rule.

- **Use `setupTestRepos`** (in `helpers_test.go`) to get a fresh
  database and the wired `*Repositories`. `seedWorkoutExercise` is a
  reusable fixture for tests that need a session + exercise row.
- **What to assert:**
  - Round-trip persistence — write a domain value, read it back,
    confirm the round-trip preserves every field including hydration.
  - Error translation at the boundary — missing rows surface as
    `domain.ErrNotFound` via `errors.Is`, never as `sql.ErrNoRows`.
  - Update-closure semantics — on `nil` from the closure the change
    persists; on a returned error the transaction rolls back and the
    on-disk state is unchanged.
  - Slot-ID stability — pre-existing `ExerciseSet.ID` values survive the
    delete-and-reinsert cycle inside `WeekPlanRepository.Update`.
- **What NOT to test here:** business rules and aggregate invariants
  (those live in `internal/domain/`), and end-to-end orchestration
  across multiple aggregates (that lives in `internal/service/`).

See `exercises_test.go` for a tight worked example covering all four
assertions above.
