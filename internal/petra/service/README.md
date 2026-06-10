# Service — Orchestration Layer

The `internal/petra/service` package coordinates work across `internal/petra/domain`
and `internal/petra/repository`, exposes the API consumed by `cmd/petra` HTTP
handlers, and owns the small set of integrations that don't fit either
of the lower layers (OpenAI exercise generation, GDPR export).

It depends on `internal/petra/domain`, `internal/petra/repository`, `internal/platform/sqlitekit`
(only for the `*sqlitekit.Database` handle that GDPR export passes through),
and `internal/platform/contexthelpers`. It does NOT depend on `cmd/petra` or
`cmd/petra/ui/templates`.

## What lives here

- **`Service` struct + `NewService` constructor.** One monolithic struct
  that the web app references as `app.service` (`*service.Service`).
- **Session orchestration** (`sessions.go`): start/complete/feedback,
  weekly plan generation, schedule resolution. Week-start arithmetic
  uses `domain.MondayOf`.
- **Set mutations** (`sets.go`): all `WeekPlans.Update`-via-aggregate
  calls that change recorded set data.
- **Exercise CRUD + slot ops** (`exercises.go`): list/get/update,
  `AddExercise`, `SwapExercise`, plus the historical-set lookup
  helpers used by both.
- **Progression construction** (`progression.go`): build
  `domain.Progression` and `domain.TimedProgression` values from a
  session's recorded sets and the rep/seconds cross-period conversion
  helpers.
- **Reporting** (`reporting.go`): read-only aggregations across
  sessions and muscle groups.
- **Feature flags** (`feature_flags.go`): thin passthroughs to the
  feature-flag repository plus the `IsMaintenanceModeEnabled`
  fail-safe.
- **AI exercise generation** (`exercise_generation.go`): the OpenAI
  client wrapper, the JSON-schema helper, the AI-or-fallback decision
  tree, and the wrapping `GenerateExercise` service method that
  persists the result.
- **GDPR export** (`export.go`): `ExportUserData` — the only method
  that touches `*sqlitekit.Database` directly.

## What does NOT live here

- **Pure rules / value objects / aggregate methods:** `internal/petra/domain/`
  ([README](../domain/README.md)).
- **SQL queries / repository implementations:** `internal/petra/repository/`
  ([README](../repository/README.md)).
- **HTTP handlers, request/response shaping, CSRF, sessions:** `cmd/petra/`.
- **Schema and migrations:** `internal/petra/repository/` (`schema.sql`); the
  SQLite driver lives in `internal/platform/sqlitekit/`.

## Update-closure pattern

Every method that mutates a `domain.Session` does so through
`s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error
{ ... })`. The closure body should be a single call to a domain aggregate
method on the week plan (e.g. `return wp.RecordSet(...)`) or — when the
mutation is naturally scoped to a single day — to a `Session` aggregate
method via `wp.SessionOn(date)`. Domain sentinels propagate unchanged.
The service layer wraps the outer error with `fmt.Errorf` to satisfy
`wrapcheck` and to add the date for diagnostic context.

`SessionRepository` is read-only — its write surface was subsumed by
`WeekPlanRepository`, which owns the transactional boundary for any
mutation that touches workout data (see
[ADR 0003](../../../docs/adr/0003-weekplan-aggregate-delete-and-reinsert.md)).

## Where to add new code

- **New cross-aggregate orchestration:** the file matching the dominant
  aggregate (e.g. a method that mutates a `Session` lives in
  `sessions.go` or `sets.go` depending on whether it's lifecycle or
  set-level).
- **New external integrations:** their own file (precedent:
  `exercise_generation.go`, `export.go`).
- **New pure rules:** `internal/petra/domain/`, then call the rule from a
  one-line service method here.
- **New SQL:** `internal/petra/repository/`, then a one-line service method
  here that wraps the call with `fmt.Errorf` and returns.

## Testing

Service tests live in `package service_test` (external) and exercise the
real wiring — real `*sqlitekit.Database`, real repositories, real domain
methods. **Do not mock the repository layer.** The orchestration and the
SQL contract are tested together because that's where bugs hide.

- **Use `setupTestService`** (in `helpers_test.go`) to get a fresh
  in-memory database, an authenticated context, and a `*service.Service`
  with sensible default preferences (Mon/Wed/Fri at 60 min). It returns
  `(ctx, svc)` — both already configured for the test user.
- **Sentinel errors propagate unchanged.** Assert them with
  `errors.Is(err, domain.ErrNotFound)`, `errors.Is(err, domain.ErrAlreadyStarted)`,
  etc. The service wraps with `fmt.Errorf("...: %w", err)` so `errors.Is`
  still matches.
- **`ValidationError` is not a sentinel.** Detect it with `errors.As(err, &ve)`
  (see `domain.ValidationError` — its message is user-facing).
- **For OpenAI / external integrations**, test the pure logic
  (parsing, validation, schema, prompt, fallback) as internal tests in
  `package service` against fakes. Gate the live API call behind
  `os.Getenv("OPENAI_API_KEY") == ""` → `t.Skip(...)` and `testing.Short()`,
  as in `exercise_generation_internal_test.go`.

Example shape (see `sessions_test.go`, `feature_flags_test.go`):

```go
func Test_Something(t *testing.T) {
    ctx, svc := setupTestService(t)
    // arrange via service or direct SQL
    got, err := svc.DoThing(ctx, ...)
    if err != nil { t.Fatalf("DoThing: %v", err) }
    // assert outcome (returned value, error type, or DB state)
}
```
