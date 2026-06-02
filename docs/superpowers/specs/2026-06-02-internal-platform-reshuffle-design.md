# Design: internal/platform reshuffle

**Date:** 2026-06-02
**Status:** Approved, pending implementation plan

## Goal

Restructure the single Go module so a second app (`cmd/example`) can share
petrapp's infrastructure cleanly, while keeping the fitness product code
clearly separated. The driver is real: a second MPA is coming, so the shared
code has to actually work for another consumer — not just look tidy.

## Guiding principle: a little copying beats a little dependency

The reshuffle has two separable goals, and only one carries dependency cost:

- **Organization** — declaring a `platform/` vs `petra/` boundary and
  relocating packages. This is the same code in honester folders; it is not a
  dependency and carries no abstraction tax.
- **Abstraction-for-reuse** — the decoupling refactors that make a package
  generic enough for a second app. This is where dependency cost lives, and
  it is taken on *selectively*.

We share only what is expensive to reimplement and dangerous to let diverge.
Everything small, trivial, or likely to diverge is copied or relocated without
abstraction.

### Triage

| Package(s) | Decision | Rationale |
|---|---|---|
| `sqlitekit` (declarative 12-step migration) | **Share + decouple** | High, subtle reimplement cost; two drifting copies = bug factory. |
| `auth` (webauthn / passkeys) | **Share + decouple** | Security-sensitive; must not have two copies. |
| `errorrecorder`, `flightrecorder`, `pprofserver` | **Share (relocate)** | Zero internal coupling; relocation is ~free. |
| `logging`, `envstruct`, `contexthelpers`, `testkit` | **Relocate, no abstraction** | ~60–90 lines each, low stakes; copying would also be fine. |
| **web middleware / render / flash / fileserver** | **Copy into example; defer `web.Kit`** | Designing a Kit against one real app + a toy todo encodes the wrong joints. Let the example duplicate; the diff between two real apps reveals the true seam later (rule of three). |
| MPA stack navigator (client-side progressive enhancement) | **Stays in petra** | Extract later once usage patterns are clear. |

## Target structure

Single module (`github.com/myrjola/petrapp`, **not renamed** — see below),
multiple `cmd/` apps sharing `internal/platform/`.

```
internal/
  platform/
    sqlitekit/        # NewDatabase(ctx, Config{URL, Schema, Fixtures, Logger})
    web/              # (future) extracted Kit — NOT built now
    auth/             # passkey handler + scs wiring; Store interface; auth.SchemaSQL
    obs/
      logging/
      errorrecorder/
      flightrecorder/
      pprofserver/
    config/           # ex-envstruct
    testkit/          # ex-testhelpers (NewLogger, Writer)
  petra/
    domain/
    repository/
    service/
cmd/
  petra/              # ex-cmd/web: handlers, routes, petra-specific middleware,
                      #   ui/templates, ui/static, fly.toml, litestream.yml
    ui/
      templates/
      static/
  example/            # todo CRUD app proving the plumbing
    ui/
      templates/
      static/
```

Notes:
- **App owns its UI.** Templates/static live under each app
  (`cmd/petra/ui`, `cmd/example/ui`), not a shared top-level `ui/`. The
  template resolver takes an app-relative `fs.FS` instead of the hardcoded
  `<go.mod>/ui/templates` (`findModuleDir()` path).
- **`contexthelpers` splits.** Generic context plumbing (CSP nonce, current
  path, trace) is platform; petra-specific keys (`AuthenticatedUserID`,
  `IsAdmin`) stay with petra/auth.
- **Fly.io infra stays petra-only** for now. Multi-app deploy is a deliberate
  later step, designed once the structure has settled.
- **`platform/web/` is reserved but empty** until the web Kit extraction is
  justified by two real apps.

## Decoupling details

### sqlitekit

Move the `//go:embed schema.sql` / `fixtures.sql` out of the package and into
each app. The kit operates on passed-in data:

```go
type Config struct {
    URL      string        // ":memory:" or path
    Schema   string        // required DDL
    Fixtures string        // optional seed
    Logger   *slog.Logger
}
func NewDatabase(ctx context.Context, cfg Config) (*Database, error)
```

The migration engine, RO/RW connection split, background optimizer, and
healthcheck are unchanged — they just consume a schema string. petra's
`schema.sql` moves next to its repository; example ships its own.

### auth

- **Storage:** `auth` defines the `Store` interface it needs (credential/user
  CRUD) and ships one `sqlitekit`-backed implementation. The package no longer
  references a concrete petra `*sqlite.Database`; the app wires the store.
- **Schema fragment:** `auth` exports its table DDL as `auth.SchemaSQL`. Each
  app concatenates it into the schema string handed to
  `sqlitekit.NewDatabase`, so the auth tables travel with the package. This
  reinforces the sqlitekit decoupling — both rely on schema-as-data.

### cmd/example (todo CRUD)

Minimal but exercises every shared package:

- `domain.Todo` (ID, Title, Done, Notes, CreatedAt)
- sqlitekit-backed repository: list, get, create, toggle, delete
- list page `GET /`, detail page `GET /todos/{id}`, plus create/toggle/delete posts
- gated behind passkey login, so it exercises `auth` sharing too
- its own `ui/` and `schema.sql` (todos + `auth.SchemaSQL`)
- web middleware/render/flash/fileserver **copied** from `cmd/petra`

Purpose: verify the shared plumbing works end-to-end and generate the second
real data point for a future web Kit extraction.

## Boundary linter

Enforced via `depguard` (already part of golangci-lint — no new tool), rules
in `.golangci.yml`:

- `internal/platform/**` may **not** import `internal/petra/**` or `cmd/**`
  (platform stays product-agnostic).
- `cmd/example/**` may **not** import `internal/petra/**`.
- `cmd/petra/**` may **not** import any future `internal/example/**`.

This is the durable guarantee that the platform/product boundary does not rot.

## Module rename: deferred

`github.com/myrjola/petrapp` is app-branded but functionally fine; `cmd/example`
reads fine under it. Renaming is cosmetic, touches every import, and adds churn
with no functional benefit. Defer to if/when modules are ever split.

## Sequencing

Each step keeps `make test` green:

1. Relocate zero-coupling obs/config/testkit → `platform/` (pure moves).
2. Decouple `sqlitekit` (schema as `Config` param; move embeds to app).
3. Decouple `auth` (`Store` interface + `auth.SchemaSQL` fragment).
4. Carve `internal/petra/` (move domain/repository/service).
5. Rename `cmd/web` → `cmd/petra`; move `ui/` under it; fix the template resolver.
6. Add depguard boundary rules.
7. Build `cmd/example`, copying the web boilerplate.

## Out of scope (deliberately deferred)

- The `web.Kit` extraction (wait for two real apps).
- The MPA stack navigator as a shared component.
- Fly.io / deploy infrastructure splitting.
- Module rename.
