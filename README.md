# Petra

Petra is a minimalist, privacy-focused personal-trainer web app that generates
and tracks gym workouts. It plans a weekly schedule around the days you pick,
progresses weights and reps from your logged performance, and runs through a
complete mesocycle including deload weeks — for people who want guided
workouts rather than building their own programs.

Design stance, throughout:

- **Private by construction.** Anonymous passkey-only accounts — no email, no
  password, nothing to identify you (see
  [ADR 0001](docs/adr/0001-passkey-only-anonymous-auth.md)). Full data export
  and account deletion built in.
- **Mobile-first, JavaScript-optional.** A server-rendered Go MPA with
  progressive enhancement; every flow works with JS disabled
  (see [ADR 0002](docs/adr/0002-stack-navigator-mpa-enhancement.md)).
- **Deliberately small.** Go standard library focus, SQLite, vanilla JS, no
  frontend framework, no build system for assets.

## Repository layout

The module is organised as a small platform/product monorepo:

```
internal/platform/   shared infrastructure, reusable by any app in the module
  sqlitekit/         declarative SQLite migrator + Config-based NewDatabase (RO/RW split, optimizer, healthcheck)
  auth/              passkey/WebAuthn handler, Store interface, auth.SchemaSQL
  obs/               observability: logging, errorrecorder, flightrecorder, pprofserver
  envstruct/         struct-tag environment-variable config
  contexthelpers/    request-scoped context helpers
  testkit/           shared test helpers
internal/petra/      the fitness product (domain / repository / service / notification)
  repository/        owns schema.sql, fixtures.sql, and embed.go (SchemaSQL / FixturesSQL)
cmd/petra/           the Petra web app (owns its ui/templates + ui/static)
cmd/example/         a minimal todo CRUD app that proves the shared platform plumbing
```

`internal/platform/` is generic and must stay product-agnostic; `internal/petra/`
is everything specific to the workout product. The split is enforced at lint time:
[depguard](.golangci.yml) rules forbid `internal/platform/` from importing
`internal/petra` or `cmd/`, and forbid `cmd/example/` from importing
`internal/petra` (it may only build on the shared platform).

`cmd/example/` deliberately **copies** the web middleware/render boilerplate
rather than importing a shared web kit — see
[ADR 0005](docs/adr/0005-copy-dont-share-web-boilerplate.md).

## Quickstart

```sh
make                  # install dependencies, configure linting
make setup-git-hooks  # optional: pre-push gate for developing straight on main
make dev              # start the server on a free local port and open the browser
```

You can [attach a debugger](https://www.jetbrains.com/help/go/attach-to-running-go-processes-with-debugger.html)
to the running server.

## Development

When a feature spans multiple layers, work outwards from the data model;
when a change is scoped to one layer, start there. Each layer has a README
next to the code:

1. [`internal/petra/repository/`](internal/petra/repository/README.md) —
   schema (`schema.sql`), migrations, SQL persistence
2. [`internal/petra/domain/`](internal/petra/domain/README.md) — pure domain
   logic and aggregate methods
3. [`internal/petra/service/`](internal/petra/service/README.md) —
   cross-aggregate orchestration and external integrations
4. [`cmd/petra/`](cmd/petra/README.md) — HTTP handlers, routing, error
   handling, testing
5. [`cmd/petra/ui/templates/`](cmd/petra/ui/templates/README.md) — templates
   and CSS, with the visual language in
   [`cmd/petra/ui/design-system.md`](cmd/petra/ui/design-system.md)

The domain vocabulary — canonical terms and the aliases to avoid — lives in
[CONTEXT.md](CONTEXT.md). Templates and static assets load from the filesystem
at runtime, so the UI dev loop is edit → refresh, no rebuild.

```sh
make test                                     # go test --race ./... (cached)
go test -v ./path/to/package -run TestName    # single test
make ci                                       # build + lint-fix + test; the pre-push gate
```

## How work ships

Petra is trunk-based: once `make ci` passes, push to `main`. Every push runs
`make ci-full` (adds shuffled tests + govulncheck) plus `make migratetest`
(restores the latest production backup and migrates it) in GitHub Actions,
deploys to staging, and promotes to production only when staging is healthy.

`make ci` is the fast local gate and deliberately skips the shuffle,
`govulncheck`, and `migratetest` that server CI adds — so a green `make ci`
can still fail the deploy gate. Before a risky push (test-ordering changes,
`schema.sql`/migrations, or `go.mod` bumps) run `make ci-full` locally —
and `make migratetest` for schema changes — to catch it on your machine.
Opening a PR instead provisions a production-like per-PR review app — useful
for infrastructure-level changes. Details in
[`docs/operations.md`](docs/operations.md).

## Documentation index

| Doc | Contents |
|---|---|
| [`CONTEXT.md`](CONTEXT.md) | Ubiquitous language of the workout domain |
| [`docs/operations.md`](docs/operations.md) | Fly.io ops: database access, profiling, CI/CD, deployments |
| [`docs/disaster-recovery.md`](docs/disaster-recovery.md) | DR runbook: scenario catalog, rebuild from nothing |
| [`docs/adr/`](docs/adr/) | Architecture decision records |
| [`docs/ops-log/`](docs/ops-log/) | Dated records of manual prod surgery |
| Layer READMEs (above) | Per-layer architecture and conventions |
| [`tlaplus/README.md`](tlaplus/README.md) | TLA+ models of the navigation shim |
| [`secrets/README.md`](secrets/README.md) | Age-encrypted backup of app secrets |

## Attribution

Petra logo made by Martin Yrjölä using [Inkscape](https://inkscape.org/).
