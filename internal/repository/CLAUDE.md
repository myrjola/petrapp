# Repository — SQLite Persistence

The `internal/repository` package is the canonical persistence layer for
the workout bounded context. It depends on `internal/domain`,
`internal/sqlite`, and `internal/contexthelpers` only — no HTTP, no
template logic, no business orchestration.

## What lives here

- **SQLite implementations** of the seven repository contracts:
  `SessionRepository`, `ExerciseRepository`, `PreferencesRepository`,
  `FeatureFlagRepository`, `MuscleGroupTargetRepository`,
  `PushSubscriptionRepository`, `ScheduledPushRepository`. Implementations
  are unexported (`sqliteSessionRepository`, etc.).
- **The `Repositories` composite struct** plus the single public
  constructor `New(db *sqlite.Database, logger *slog.Logger) *Repositories`
  that wires everything together (notably injecting `ExerciseRepository`
  into `SessionRepository` for hydration).
- **Shared helpers** in `shared.go`: `parseTimestamp`, `formatTimestamp`,
  `formatDate`, the `baseRepository` mixin, and the timestamp/date format
  constants.

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

`SessionRepository.Update` and `ExerciseRepository.Update` accept a
closure `func(*domain.X) error` that runs inside an open transaction:

- The repo loads the aggregate (hydrating exercise data for sessions),
  runs the closure, persists the result on `nil`, rolls back on error.
- The closure expresses business invariants by calling aggregate methods
  on `domain.Session` (e.g. `sess.Start(now)`); domain sentinels
  (`ErrAlreadyStarted`, `ErrSlotNotFound`, etc.) propagate to the caller
  unchanged so service-layer code can `errors.Is` against them.
- Returning a non-domain error rolls back too — the repo doesn't try to
  classify; service code decides whether the error is fatal.

## Diff strategy: delete-and-reinsert

`SessionRepository.Update` persists by deleting the
`workout_sessions` row inside the tx (CASCADE clears
`workout_exercise` and `exercise_sets`) and re-inserting the entire
session. Pre-existing `ExerciseSet.ID` values are passed back into
`INSERT ... RETURNING id` so URL-stable slot IDs survive the cycle. New
slots (ID == 0) get auto-assigned IDs. For PetrApp's data sizes (a
handful of exercises × a handful of sets per session) the cost is
negligible and the simplicity is worth the trade.

## Hydration policy

`SessionRepository.Get` and `List` always populate `ExerciseSet.Exercise`
inline: the base exercise columns are joined in via
`workout_exercise.exercise_id → exercises.id`, and primary/secondary
muscle groups are fetched in a single follow-up query keyed by the
deduped exercise IDs of the session. A session with N exercise slots
costs two read queries (the sets+exercise join plus the batched muscle
group fetch) rather than the prior 1 + 2N. Callers receive a
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
