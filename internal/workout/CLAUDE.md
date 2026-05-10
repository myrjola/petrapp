# Workout Package — Migration Status

> **Migration in progress (Phases 1 + 2 of 4 complete as of 2026-05-10).**
> - Pure logic lives in `internal/domain/` (Phase 1).
> - Persistence lives in `internal/repository/` (Phase 2).
> - This package now contains only orchestration code (`service.go`, the
>   AI exercise generator, and backward-compat type aliases). Phase 3
>   moves these to `internal/service/`. Phase 4 deletes the package.
>
> See `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md`.

## What still lives here

- **`models.go`** — type aliases (`type Session = domain.Session`, etc.)
  so `cmd/web/` handlers continue to import `workout.*` symbols without
  edit. Removed in Phase 4.
- **`service.go`** — `Service` struct, `NewService`, orchestration that
  combines repository calls with domain logic, AI exercise creation,
  weekly plan generation, GDPR export. Moves to `internal/service/` in
  Phase 3.
- **`generator-exercise.go`** — OpenAI-backed exercise content
  generator. Moves to `internal/service/` in Phase 3 alongside its
  unexported JSON-schema type.
- **`service_test.go` / `service_internal_test.go` /
  `generator-exercise_internal_test.go`** — orchestration and helper
  unit tests; relocate alongside their subjects in Phase 3.

## Where to add new code

- **Pure rules / value objects / aggregate methods:** `internal/domain/`.
- **New SQL queries / repository methods:** `internal/repository/`.
- **Cross-aggregate orchestration / external integrations / GDPR:**
  here, in `service.go` (Phase 3 will move it intact).

## Sentinel errors

`workout.ErrNotFound` re-exports `domain.ErrNotFound`. Handlers and
existing tests use it; no behaviour change. New sentinels go in
`internal/domain/errors.go`.

## Display derivations belong on domain types

Unchanged from Phase 1: any value that depends on multiple domain
attributes, or that encodes a business rule, lives as a method on the
domain type. See `internal/domain/CLAUDE.md`.
