# PetrApp - Go Web Application

A fitness tracking web application built with Go, SQLite, and server-side rendered templates.

## Domain Language

The workout domain's ubiquitous language — canonical terms, their definitions,
and the aliases to avoid — lives in [CONTEXT.md](CONTEXT.md). Use those names in
code, comments, and discussion; when code and CONTEXT.md disagree, reconcile the
two rather than letting them drift.

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

## Worktree-first for design & implementation

Any multi-step design or implementation task — a brainstorming session,
writing-plans, or an executing-plans run — happens in an isolated git
worktree, not in the primary checkout.

- **Remote sessions** started via `make claude-worktree-remote`
  (`claude remote-control --spawn worktree`) already run in a fresh worktree
  under `.claude/worktrees/` — no setup needed.
- **Local sessions** isolate with the native mechanism before the first
  written artifact: the EnterWorktree tool mid-session, or `claude --worktree
  <name>` at startup. Read-only exploration may happen in the primary
  checkout, but the moment you start writing — a design spec counts — switch
  into the worktree. The spec, the plan, and the implementation all ship
  together from that branch.
- `.worktreeinclude` copies gitignored local files (currently
  `.claude/settings.local.json`, the local permission grants) into each new
  worktree. Files inside gitignored directories can't be copied, so
  `make ci`/`make init` installs `bin/golangci-lint` per worktree instead.
- Truly throwaway exploration that won't produce a committed artifact stays in
  the primary checkout.

## Shipping

Petrapp is trunk-based: work ships by pushing the worktree branch straight to
main once `make ci` passes. Validation runs in three tiers, each cheap because
of the one below it:

1. **Inner loop** — focused tests on the package you're touching
   (`go test ./internal/petra/domain -run TestX`); templates hot-reload, no
   rebuild needed.
2. **Before push** — `make ci`. Lint and test results are cache-backed, so it
   only re-validates what changed since the last run (seconds when the inner
   loop already passed).
3. **Server (the real wall)** — pushing to main runs `make ci-full` (adds
   shuffled tests + govulncheck) plus `make migratetest` in GitHub Actions;
   prod only deploys when that passes. Staging deploys without waiting for
   tests, and an occasionally broken staging is accepted.

```bash
make ci                      # must pass — fix failures, never push around them
git push origin HEAD:main
```

- The tracked `.githooks/pre-push` hook (armed by `git config core.hooksPath
  .githooks`, which `make claude-worktree-remote` runs) enforces a clean tree
  and `make -j2 lint test` before any push — a gate of last resort, not a
  substitute for running `make ci` yourself. It hits the same caches, so it's
  near-free when you already ran the checks.
- If the push is rejected because main moved: `git fetch origin && git rebase
  origin/main`, re-run `make ci`, push again. Never force-push.
- Afterwards, end the session (or ExitWorktree with remove) and let the native
  cleanup delete the worktree and branch — the commits are already on
  origin/main. The primary checkout picks them up with `git pull --ff-only`
  next time you work there.

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
make test         # `go test --race ./...` — cached, only re-runs changed packages
make lint-fix     # golangci-lint with --fix; run before committing
make ci           # init + build + lint-fix + test; the local pre-push gate
make ci-full      # ci + test-shuffle + sec; what server CI runs before prod deploys
make claude-worktree-remote  # serve remote-control sessions, one fresh worktree each
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
