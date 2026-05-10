# Service — Orchestration Layer

The `internal/service` package coordinates work across `internal/domain`
and `internal/repository`, exposes the API consumed by `cmd/web` HTTP
handlers, and owns the small set of integrations that don't fit either
of the lower layers (OpenAI exercise generation, GDPR export).

It depends on `internal/domain`, `internal/repository`, `internal/sqlite`
(only for the `*sqlite.Database` handle that GDPR export passes through),
and `internal/contexthelpers`. It does NOT depend on `cmd/web`,
`ui/templates`, or `internal/workout`.

## What lives here

- **`Service` struct + `NewService` constructor.** One monolithic struct
  that handlers reference as `*service.Service`. Phase 4 will rename the
  field on the web app from `workoutService` to `service` and drop the
  `internal/workout` shim.
- **Session orchestration** (`sessions.go`): start/complete/feedback,
  weekly plan generation, schedule resolution, the `mondayOf` helper.
- **Set mutations** (`sets.go`): all `Session.Update`-via-aggregate
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
  that touches `*sqlite.Database` directly.

## What does NOT live here

- **Pure rules / value objects / aggregate methods:** `internal/domain/`.
  See `internal/domain/CLAUDE.md`.
- **SQL queries / repository implementations:** `internal/repository/`.
  See `internal/repository/CLAUDE.md`.
- **HTTP handlers, request/response shaping, CSRF, sessions:** `cmd/web/`.
- **Schema and migrations:** `internal/sqlite/`.

## Update-closure pattern

Every method that mutates a `domain.Session` does so through
`s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error
{ ... })`. The closure body should be a single call to a domain aggregate
method (e.g. `return sess.RecordSet(...)`); domain sentinels propagate
unchanged. The service layer wraps the outer error with `fmt.Errorf` to
satisfy `wrapcheck` and to add the date for diagnostic context.

## Where to add new code

- **New cross-aggregate orchestration:** the file matching the dominant
  aggregate (e.g. a method that mutates a `Session` lives in
  `sessions.go` or `sets.go` depending on whether it's lifecycle or
  set-level).
- **New external integrations:** their own file (precedent:
  `exercise_generation.go`, `export.go`).
- **New pure rules:** `internal/domain/`, then call the rule from a
  one-line service method here.
- **New SQL:** `internal/repository/`, then a one-line service method
  here that wraps the call with `fmt.Errorf` and returns.
