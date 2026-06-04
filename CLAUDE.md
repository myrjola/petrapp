# PetrApp - Go Web Application

A fitness tracking web application built with Go, SQLite, and server-side rendered templates.

## Development Workflow

When a feature spans multiple layers, work outwards from the data model
(shared infrastructure lives under `internal/platform/`, product code under
`internal/petra/` and `cmd/petra/`):

1. **Database First** - Start with the SQLite driver/helpers in
   `internal/platform/sqlitekit/` (see
   [Database Guidelines](internal/platform/sqlitekit/CLAUDE.md)); the workout
   schema (`schema.sql`) now lives alongside the repository in
   `internal/petra/repository/`.
2. **Domain Models** - Update pure domain logic in `internal/petra/domain/` (
   see [Domain Guidelines](internal/petra/domain/CLAUDE.md))
3. **Repository Layer** - Add SQL persistence and query implementations in
   `internal/petra/repository/` (see [Repository Guidelines](internal/petra/repository/CLAUDE.md))
4. **Service Layer** - Add orchestration / cross-aggregate logic in `internal/petra/service/` (
   see [Service Guidelines](internal/petra/service/CLAUDE.md))
5. **HTTP Layer** - Add handlers and routing in `cmd/petra/` (see [Web Guidelines](cmd/petra/CLAUDE.md))
6. **Templates & UI** - Build frontend in `cmd/petra/ui/templates/` (see [Template Guidelines](cmd/petra/ui/templates/CLAUDE.md))

If a change is scoped to one or two layers (e.g. a UI-only tweak or a handler-only bug fix), start at the lowest relevant layer — you don't have to touch the ones above.

## Runtime Layout

- Templates (`cmd/petra/ui/templates/`) and static assets (`cmd/petra/ui/static/`) are loaded from the filesystem at runtime (`os.DirFS`, not `//go:embed`). Editing a template and refreshing the browser is the whole dev loop — no rebuild needed.
- In the Docker image, `main.css` and `main.js` are fingerprinted with an md5 hash at build time and `base.gohtml` is rewritten to reference the hashed names (see `Dockerfile`). Other static files (`webauthn.js`, icons) are served under their original names.

## One-shot scripts and post-mortems

Recovery SQL, one-shot prod migrations, and post-incident write-ups go in `docs/` named
`YYYY-MM-DD-<slug>.{md,sql}`. Keep them checked in — they're the only durable record of
manual prod surgery, and the `make migratetest` flow runs them only via the deploy migration
path, not from `docs/`. Examples: `docs/2026-05-02-code-review-cleanup.md`,
`docs/2026-05-02-recover-incline-dumbbell-bench-press.sql`.

## Build & Run Commands

```bash
make init         # One-time setup after cloning
make test         # `go test --race --shuffle=on ./...`
make lint-fix     # golangci-lint with --fix; run before committing
make ci           # init + build + lint-fix + test + sec; full validation
```

Run a single test: `go test -v ./path/to/package -run TestName`.

## Code Style

- Standard Go formatting with 100-line function limit (`.golangci.yml` funlen)
- Error types must be suffixed with "Error", sentinel errors with "Err" prefix
- Strongly typed with exhaustive enum checking
- No global loggers or init functions
- Comments must end with a period
- govet shadow is usually fixed by reusing the earlier `err` variable instead of introducing a new name

## Security Guidelines

- HTTP handlers must use context
- CSP and CSRF protection enforced
- All SQL queries parameterized
- Never introduce code that exposes or logs secrets and keys
- Never commit secrets or keys to the repository

## Following Conventions

Match the existing code: check `go.mod` and neighboring files before assuming a
library is available, and mirror the patterns and imports of nearby code.

Test-selector resilience (goquery patterns, `SubmitForm`, etc.) lives in
[Web Guidelines](cmd/petra/CLAUDE.md) under "Testing with e2etest" — consult it
when UI changes break handler tests.
