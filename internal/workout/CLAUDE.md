# Workout Package — Migration Status

> **Migration in progress (Phases 1–3 of 4 complete as of 2026-05-10).**
> - Pure logic lives in `internal/domain/` (Phase 1).
> - Persistence lives in `internal/repository/` (Phase 2).
> - Orchestration lives in `internal/service/` (Phase 3).
> - This package is now a backward-compat shim that re-exports the
>   `Service` type and `NewService` constructor for `cmd/web` callers,
>   plus the type aliases that let handlers reference domain types as
>   `workout.Foo`. Phase 4 deletes the package entirely.
>
> See `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md`.

## What still lives here

- **`models.go`** — type aliases (`type Session = domain.Session`,
  etc.), the `RegionFor` helper, the `SwapSimilarityScore` helper, and
  the `ErrNotFound` re-export. Phase 4 sweeps the import path in
  `cmd/web/` and deletes this file.
- **`service.go`** — five-line shim: `type Service = service.Service`
  plus a `NewService` forwarder. Phase 4 deletes this file too.

## Where to add new code

- **Pure rules / value objects / aggregate methods:** `internal/domain/`.
- **New SQL queries / repository methods:** `internal/repository/`.
- **Cross-aggregate orchestration / external integrations / GDPR:**
  `internal/service/`.
- **Nothing new lands here.** The package is closing down.

## Sentinel errors

`workout.ErrNotFound` re-exports `domain.ErrNotFound` for handler-side
`errors.Is` checks. New sentinels go in `internal/domain/errors.go`.
