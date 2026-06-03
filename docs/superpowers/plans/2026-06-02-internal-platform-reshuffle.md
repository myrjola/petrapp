# internal/platform Reshuffle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Carve `internal/platform/` (shared infrastructure) from `internal/petra/` (fitness product), genuinely decouple only `sqlitekit` and `auth`, and prove the plumbing with a new `cmd/example` todo app — without prematurely abstracting a web Kit.

**Architecture:** Single Go module, multiple `cmd/` apps sharing `internal/platform/`. High-cost/high-divergence-risk packages (`sqlitekit`, `auth`) are shared behind data/interface seams (schema-as-data, a `Store` interface). Zero-coupling observability is relocated as-is. The web middleware/render boilerplate is *copied* into `cmd/example` rather than abstracted; the diff between two real apps will later reveal the true seam. A `depguard` rule enforces the platform/product boundary.

**Tech Stack:** Go 1.26, SQLite (mattn/go-sqlite3), html/template, scs sessions, go-webauthn, golangci-lint (depguard).

**Decisions locked during planning (deviations from the spec, all intentional):**
- `envstruct` keeps its name and relocates to `internal/platform/envstruct` (a `config` package would clash with `cmd/web/main.go`'s `type config`).
- `contexthelpers` moves wholesale to `internal/platform/contexthelpers` — no split; all helpers are auth/web-generic.
- `testhelpers` is renamed to `testkit` at `internal/platform/testkit`.
- `platform/web/` is **not** created (the web Kit is deferred).
- Module path stays `github.com/myrjola/petrapp` (rename deferred).

**Conventions for every task:**
- Verify the whole suite with `make test` (`go test --race --shuffle=on ./...`) and compilation with `go build ./...`.
- Run `make lint-fix` before each commit.
- Commit messages end with the repo's required trailer:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Work on a feature branch (the design lives on `platform-reshuffle-design`; create `platform-reshuffle` off `main` for implementation, or continue on the design branch — match your worktree setup).

---

## File Structure (end state)

```
internal/
  platform/
    sqlitekit/        # was internal/sqlite; NewDatabase(ctx, Config{...})
    auth/             # was internal/webauthnhandler; Store iface + SQLiteStore + SchemaSQL
    contexthelpers/   # was internal/contexthelpers (moved whole)
    envstruct/        # was internal/envstruct
    testkit/          # was internal/testhelpers (package renamed)
    obs/
      logging/        # was internal/logging
      errorrecorder/  # was internal/errorrecorder
      flightrecorder/ # was internal/flightrecorder
      pprofserver/    # was internal/pprofserver
  petra/
    domain/           # was internal/domain
    repository/       # was internal/repository (+ schema.sql, fixtures.sql, embed.go)
    service/          # was internal/service
    notification/     # was internal/notification (imports petra/domain)
cmd/
  petra/              # was cmd/web; + ui/templates, ui/static; fly.toml stays at repo root
    ui/
  example/            # NEW todo CRUD app
    ui/
```

`internal/loadtest` and `internal/e2etest` import `e2etest`/`domain`; they move with petra
concerns in Task 7 (path fixups only). `cmd/deployprobe`, `cmd/smoketest`,
`cmd/stresstest`, `cmd/migratetest`, `cmd/exercise-content-fixup` are petra tools; their
imports are fixed by the global sweeps as paths change.

---

## Phase 1 — Relocate zero-coupling infrastructure (pure moves)

These tasks only move directories and rewrite import paths. No behavior changes; the
existing test suite is the safety net. Each task ends green.

### Task 1: Move observability packages under `internal/platform/obs/`

**Files:**
- Move: `internal/logging/` → `internal/platform/obs/logging/`
- Move: `internal/errorrecorder/` → `internal/platform/obs/errorrecorder/`
- Move: `internal/flightrecorder/` → `internal/platform/obs/flightrecorder/`
- Move: `internal/pprofserver/` → `internal/platform/obs/pprofserver/`

Package names are unchanged (each dir's base name still equals its package name), so only
import-path strings change, never call sites.

- [ ] **Step 1: Create the target directory and move the packages**

```bash
cd /home/martin/petrapp
mkdir -p internal/platform/obs
git mv internal/logging        internal/platform/obs/logging
git mv internal/errorrecorder  internal/platform/obs/errorrecorder
git mv internal/flightrecorder internal/platform/obs/flightrecorder
git mv internal/pprofserver    internal/platform/obs/pprofserver
```

- [ ] **Step 2: Rewrite import paths across the repo**

```bash
cd /home/martin/petrapp
for p in logging errorrecorder flightrecorder pprofserver; do
  grep -rl "myrjola/petrapp/internal/$p" --include='*.go' . \
    | xargs --no-run-if-empty sed -i \
        "s|myrjola/petrapp/internal/$p|myrjola/petrapp/internal/platform/obs/$p|g"
done
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 4: Run the suite**

Run: `make test`
Expected: PASS (all packages ok).

- [ ] **Step 5: Lint and commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add -A
git commit -m "refactor(platform): relocate observability packages under platform/obs

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Move `envstruct` and `contexthelpers` under `internal/platform/`

**Files:**
- Move: `internal/envstruct/` → `internal/platform/envstruct/`
- Move: `internal/contexthelpers/` → `internal/platform/contexthelpers/`

Package names unchanged; only import paths change.

- [ ] **Step 1: Move the packages**

```bash
cd /home/martin/petrapp
git mv internal/envstruct      internal/platform/envstruct
git mv internal/contexthelpers internal/platform/contexthelpers
```

- [ ] **Step 2: Rewrite import paths**

```bash
cd /home/martin/petrapp
for p in envstruct contexthelpers; do
  grep -rl "myrjola/petrapp/internal/$p" --include='*.go' . \
    | xargs --no-run-if-empty sed -i \
        "s|myrjola/petrapp/internal/$p|myrjola/petrapp/internal/platform/$p|g"
done
```

- [ ] **Step 3: Verify build + tests**

Run: `go build ./... && make test`
Expected: PASS.

- [ ] **Step 4: Lint and commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add -A
git commit -m "refactor(platform): relocate envstruct and contexthelpers under platform

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Move + rename `testhelpers` → `internal/platform/testkit`

**Files:**
- Move: `internal/testhelpers/` → `internal/platform/testkit/`
- Edit: package clause in moved files (`package testhelpers` → `package testkit`)

Unlike Tasks 1–2 this renames the package, so call sites change too (37 files use it).

- [ ] **Step 1: Move the package**

```bash
cd /home/martin/petrapp
mkdir -p internal/platform
git mv internal/testhelpers internal/platform/testkit
```

- [ ] **Step 2: Rename the package clause in the moved files**

```bash
cd /home/martin/petrapp
sed -i 's/^package testhelpers/package testkit/' internal/platform/testkit/*.go
```

- [ ] **Step 3: Rewrite import paths and identifier references repo-wide**

```bash
cd /home/martin/petrapp
# Import path:
grep -rl "myrjola/petrapp/internal/testhelpers" --include='*.go' . \
  | xargs --no-run-if-empty sed -i \
      's|myrjola/petrapp/internal/testhelpers|myrjola/petrapp/internal/platform/testkit|g'
# Qualified identifier (testhelpers.NewWriter -> testkit.NewWriter, etc.):
grep -rl "testhelpers\." --include='*.go' . \
  | xargs --no-run-if-empty sed -i 's/\btesthelpers\./testkit./g'
```

- [ ] **Step 4: Verify build + tests**

Run: `go build ./... && make test`
Expected: PASS. (If a stray `testhelpers` identifier remains, `go build` names the file/line.)

- [ ] **Step 5: Lint and commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add -A
git commit -m "refactor(platform): move testhelpers to platform/testkit

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 2 — Decouple `sqlitekit`

### Task 4: Move `internal/sqlite` → `internal/platform/sqlitekit` and parametrize schema/fixtures

**Files:**
- Move: `internal/sqlite/` → `internal/platform/sqlitekit/`
- Modify: `internal/platform/sqlitekit/sqlite.go` (remove embeds, add `Config`, new `NewDatabase`)
- Create: `internal/repository/schema.sql`, `internal/repository/fixtures.sql` (moved from sqlite)
- Create: `internal/repository/embed.go` (exports `SchemaSQL`, `FixturesSQL`)
- Modify callers: `cmd/web/main.go:124`, `cmd/migratetest/main.go:37`, and tests that call
  `NewDatabase` (`cmd/web/handler-healthy_test.go`, `internal/repository/helpers_test.go`,
  `internal/service/helpers_test.go`, `internal/service/feature_flags_test.go`,
  `internal/platform/sqlitekit/sqlite_test.go`).

The migration engine, RO/RW split, optimizer, and healthcheck are unchanged — they just
consume a schema string instead of an embedded file.

- [ ] **Step 1: Move the package and detach the schema files**

```bash
cd /home/martin/petrapp
mkdir -p internal/platform
git mv internal/sqlite internal/platform/sqlitekit
sed -i 's/^package sqlite$/package sqlitekit/' internal/platform/sqlitekit/*.go
# Schema + fixtures now belong to petra's repository, not the generic kit:
git mv internal/platform/sqlitekit/schema.sql   internal/repository/schema.sql
git mv internal/platform/sqlitekit/fixtures.sql internal/repository/fixtures.sql
```

- [ ] **Step 2: Write the failing test for the new `Config` API**

Replace the body of the first test in `internal/platform/sqlitekit/sqlite_test.go` that
constructs a DB with the new signature, and add a focused test. Append this test:

```go
func TestNewDatabase_AppliesProvidedSchema(t *testing.T) {
	t.Parallel()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	db, err := sqlitekit.NewDatabase(t.Context(), sqlitekit.Config{
		URL:      ":memory:",
		Schema:   "CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL);",
		Fixtures: "INSERT INTO widgets (name) VALUES ('seed');",
		Logger:   logger,
	})
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var n int
	if err = db.ReadOnly.QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM widgets").Scan(&n); err != nil {
		t.Fatalf("query widgets: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 seeded widget, got %d", n)
	}
}
```

Add the import `"github.com/myrjola/petrapp/internal/platform/testkit"` to the test file if
not present.

- [ ] **Step 2b: Run it to confirm it fails**

Run: `go test ./internal/platform/sqlitekit/ -run TestNewDatabase_AppliesProvidedSchema -v`
Expected: FAIL — compile error (`Config` undefined / too many arguments).

- [ ] **Step 3: Change `NewDatabase` to take `Config`**

In `internal/platform/sqlitekit/sqlite.go`, remove the two `//go:embed` directives and the
`schemaDefinition` / `fixtures` package vars, and the `_ "embed"` import. Replace the
constructor:

```go
// Config configures a Database. Schema is the required DDL applied via the
// declarative migrator; Fixtures is optional idempotent seed SQL.
type Config struct {
	URL      string
	Schema   string
	Fixtures string
	Logger   *slog.Logger
}

// NewDatabase connects to a database, migrates it to cfg.Schema, and applies
// cfg.Fixtures. cfg.URL is a file path or ":memory:".
func NewDatabase(ctx context.Context, cfg Config) (*Database, error) {
	var (
		err error
		db  *Database
	)

	if db, err = connect(ctx, cfg.URL, cfg.Logger); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err = db.migrateTo(ctx, cfg.Schema); err != nil {
		return nil, fmt.Errorf("migrateTo: %w", err)
	}

	if cfg.Fixtures != "" {
		if _, err = db.ReadWrite.ExecContext(ctx, cfg.Fixtures); err != nil {
			return nil, fmt.Errorf("apply fixtures: %w", err)
		}
	}

	return db, nil
}
```

(Keep everything below the original fixtures-apply block — start-optimizer, return —
intact; only the embed-driven parts change. If the original applied fixtures
unconditionally, the `if cfg.Fixtures != ""` guard preserves behavior since petra always
passes fixtures.)

- [ ] **Step 4: Create petra's schema embed**

Create `internal/repository/embed.go`:

```go
package repository

import _ "embed"

// SchemaSQL is petra's product schema (workouts, exercises, preferences, etc.).
// It is concatenated after auth.SchemaSQL when constructing the database, so
// petra tables may reference auth tables (e.g. users) via foreign keys.
//
//go:embed schema.sql
var SchemaSQL string

// FixturesSQL is petra's idempotent seed data.
//
//go:embed fixtures.sql
var FixturesSQL string
```

- [ ] **Step 5: Update every `NewDatabase` caller**

Production callers (`cmd/web/main.go:124`, `cmd/migratetest/main.go:37`) pass petra's
schema/fixtures. Example for `cmd/web/main.go` — replace the call:

```go
db, err := sqlite.NewDatabase(ctx, cfg.SqliteURL, logger)
```

with (note: import path changes to sqlitekit; full schema assembled in Task 5 once auth
owns its fragment — for now use repository.SchemaSQL alone):

```go
db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
	URL:      cfg.SqliteURL,
	Schema:   repository.SchemaSQL,
	Fixtures: repository.FixturesSQL,
	Logger:   logger,
})
```

Test callers that used `:memory:` (e.g. `internal/repository/helpers_test.go:28`,
`internal/service/helpers_test.go:31`, `internal/service/feature_flags_test.go:25`,
`cmd/web/handler-healthy_test.go:22`) become:

```go
db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
	URL:      ":memory:",
	Schema:   repository.SchemaSQL,
	Fixtures: repository.FixturesSQL,
	Logger:   logger,
})
```

Then rewrite the import path repo-wide:

```bash
cd /home/martin/petrapp
grep -rl "myrjola/petrapp/internal/sqlite" --include='*.go' . \
  | xargs --no-run-if-empty sed -i \
      's|myrjola/petrapp/internal/sqlite|myrjola/petrapp/internal/platform/sqlitekit|g'
grep -rl "\bsqlite\.\(NewDatabase\|Database\|Config\)" --include='*.go' . \
  | xargs --no-run-if-empty sed -i 's/\bsqlite\./sqlitekit./g'
```

> Note: the second sed only retargets `sqlite.NewDatabase/Database/Config`. Verify no
> unrelated `sqlite.` references (e.g. driver registration) were rewritten; `go build`
> surfaces any mistake.

- [ ] **Step 6: Verify the new test passes, then the whole suite**

Run: `go test ./internal/platform/sqlitekit/ -run TestNewDatabase_AppliesProvidedSchema -v`
Expected: PASS.
Run: `go build ./... && make test`
Expected: PASS.

- [ ] **Step 7: Lint and commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add -A
git commit -m "refactor(platform): make sqlitekit schema/fixtures caller-provided

Move internal/sqlite to platform/sqlitekit and pass Schema/Fixtures via
Config instead of embedding them in the package. petra's schema/fixtures
now live in internal/repository.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 3 — Decouple `auth`

### Task 5: Move `webauthnhandler` → `internal/platform/auth`, add `Store` + `SchemaSQL`

**Files:**
- Move: `internal/webauthnhandler/` → `internal/platform/auth/`
- Create: `internal/platform/auth/schema.go` (exports `SchemaSQL`)
- Modify: `internal/platform/auth/handler.go` (constructor takes `Store`, not `*sqlitekit.Database`)
- Modify: `internal/platform/auth/persistence.go` (becomes the `SQLiteStore` implementation)
- Modify: `internal/repository/schema.sql` (remove auth tables; they move to auth.SchemaSQL)
- Modify: `cmd/web/main.go` (construct `auth.NewSQLiteStore(db)`, assemble full schema)

The auth tables (`sessions`, `users`, `credentials` + their triggers — `schema.sql` lines
1–68) travel with the auth package. Each app concatenates `auth.SchemaSQL` ahead of its own.

- [ ] **Step 1: Move the package**

```bash
cd /home/martin/petrapp
git mv internal/webauthnhandler internal/platform/auth
sed -i 's/^package webauthnhandler$/package auth/' internal/platform/auth/*.go
grep -rl "myrjola/petrapp/internal/webauthnhandler" --include='*.go' . \
  | xargs --no-run-if-empty sed -i \
      's|myrjola/petrapp/internal/webauthnhandler|myrjola/petrapp/internal/platform/auth|g'
# Update qualified references (webauthnhandler.New -> auth.New, etc.):
grep -rl "webauthnhandler\." --include='*.go' . \
  | xargs --no-run-if-empty sed -i 's/\bwebauthnhandler\./auth./g'
```

- [ ] **Step 2: Extract the auth tables into `auth.SchemaSQL`**

Cut `schema.sql` lines 1–68 (the `sessions`, `users`, `credentials` tables and their
triggers — everything above the `-- Workout planning --` banner) out of
`internal/repository/schema.sql` and paste them into a new
`internal/platform/auth/schema.go` as a raw string:

```go
package auth

// SchemaSQL defines the tables auth owns: scs sessions, users, and webauthn
// credentials. Apps concatenate this ahead of their own product schema when
// constructing the database, so product tables may FK to users.
const SchemaSQL = `
CREATE TABLE sessions
( ... );   -- paste the exact DDL + triggers from schema.sql lines 1-68 here
`
```

After this, `internal/repository/schema.sql` starts at the `CREATE TABLE
workout_preferences` block.

- [ ] **Step 3: Write the failing test for `SQLiteStore` behind a `Store` interface**

Create `internal/platform/auth/store_test.go`:

```go
package auth_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func TestSQLiteStore_SatisfiesStore(t *testing.T) {
	t.Parallel()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	db, err := sqlitekit.NewDatabase(t.Context(), sqlitekit.Config{
		URL:    ":memory:",
		Schema: auth.SchemaSQL,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var store auth.Store = auth.NewSQLiteStore(db)
	if store == nil {
		t.Fatal("NewSQLiteStore returned nil")
	}
}
```

- [ ] **Step 3b: Run it to confirm it fails**

Run: `go test ./internal/platform/auth/ -run TestSQLiteStore_SatisfiesStore -v`
Expected: FAIL — `auth.Store` / `auth.NewSQLiteStore` undefined.

- [ ] **Step 4: Define the `Store` interface and `SQLiteStore`**

The existing persistence functions are methods on `*WebAuthnHandler`
(`internal/webauthnhandler/persistence.go`): `upsertUser(ctx, webauthn.User) error`,
`getUser(ctx, []byte) (*user, error)`, `upsertCredential(ctx, ...) error`,
`getUserRole(ctx, []byte) (role, error)`, `getUserIntegerID(ctx, []byte) (int, error)`,
`deleteUser(ctx, []byte) error`. They use the unexported `user` and `role` types, all in
package `auth`. In `internal/platform/auth/persistence.go`, introduce a `Store` interface
with exactly those operations and a `SQLiteStore` that holds the DB:

```go
// Store is the persistence the auth handler needs. SQLiteStore is the default
// sqlitekit-backed implementation; the interface removes the handler's hard
// dependency on a concrete DB and gives tests a seam. user/role stay package
// types, so implementations live in package auth (both apps use SQLiteStore).
type Store interface {
	upsertUser(ctx context.Context, u webauthn.User) error
	getUser(ctx context.Context, webAuthnID []byte) (*user, error)
	upsertCredential(ctx context.Context, webAuthnID []byte, cred webauthn.Credential) error
	getUserRole(ctx context.Context, webAuthnID []byte) (role, error)
	getUserIntegerID(ctx context.Context, webAuthnID []byte) (int, error)
	deleteUser(ctx context.Context, webAuthnID []byte) error
}

// SQLiteStore implements Store against a sqlitekit.Database.
type SQLiteStore struct {
	db *sqlitekit.Database
}

func NewSQLiteStore(db *sqlitekit.Database) *SQLiteStore {
	return &SQLiteStore{db: db}
}
```

Match `upsertCredential`'s real signature to the existing function. Re-home the existing SQL
bodies as `*SQLiteStore` methods (changing the receiver from `h *WebAuthnHandler` to
`s *SQLiteStore` and `h.database` to `s.db`). In `handler.go`, replace the field
`database *sqlitekit.Database` with `store Store` and the constructor parameter
`dbs *sqlitekit.Database` with `store Store`; rewrite the handler's internal calls from
`h.upsertUser(...)` etc. to `h.store.upsertUser(...)`.

> Because the `Store` methods are unexported, alternative implementations would have to live
> in package `auth` — which is fine: both apps use `SQLiteStore`. The interface's job here is
> to cut the concrete-DB dependency, not to enable third-party stores (that would be
> premature per the design's copying-over-dependency principle).

- [ ] **Step 5: Update the handler constructor caller in `cmd/web/main.go`**

```go
authStore := auth.NewSQLiteStore(db)
webAuthnHandler, err := auth.New(cfg.Addr, fqdn, tlsEnabled, logger, sessionManager, authStore)
```

And assemble the full schema where the DB is created (supersedes Task 4's interim
`repository.SchemaSQL` alone):

```go
db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
	URL:      cfg.SqliteURL,
	Schema:   auth.SchemaSQL + "\n" + repository.SchemaSQL,
	Fixtures: repository.FixturesSQL,
	Logger:   logger,
})
```

Apply the same `auth.SchemaSQL + "\n" + repository.SchemaSQL` assembly to
`cmd/migratetest/main.go` and every test helper that builds a petra DB (the four test
files updated in Task 4). A quick way to find them:

```bash
cd /home/martin/petrapp
grep -rln "repository.SchemaSQL" --include='*.go' .
```

- [ ] **Step 6: Verify**

Run: `go test ./internal/platform/auth/ -run TestSQLiteStore_SatisfiesStore -v`
Expected: PASS.
Run: `go build ./... && make test`
Expected: PASS.

- [ ] **Step 7: Lint and commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add -A
git commit -m "refactor(platform): decouple auth from concrete DB via Store + SchemaSQL

Move webauthnhandler to platform/auth, introduce a Store interface with a
sqlitekit-backed SQLiteStore, and ship the auth tables as auth.SchemaSQL so
the package is droppable into another app.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 4 — Carve `internal/petra/`

### Task 6: Move product packages under `internal/petra/`

**Files:**
- Move: `internal/domain/` → `internal/petra/domain/`
- Move: `internal/repository/` → `internal/petra/repository/`
- Move: `internal/service/` → `internal/petra/service/`
- Move: `internal/notification/` → `internal/petra/notification/`

Package names unchanged; only import paths change (domain alone has 71 importers, so a clean
sweep matters).

- [ ] **Step 1: Move the packages**

```bash
cd /home/martin/petrapp
mkdir -p internal/petra
git mv internal/domain       internal/petra/domain
git mv internal/repository   internal/petra/repository
git mv internal/service      internal/petra/service
git mv internal/notification internal/petra/notification
```

- [ ] **Step 2: Rewrite import paths**

```bash
cd /home/martin/petrapp
for p in domain repository service notification; do
  grep -rl "myrjola/petrapp/internal/$p" --include='*.go' . \
    | xargs --no-run-if-empty sed -i \
        "s|myrjola/petrapp/internal/$p|myrjola/petrapp/internal/petra/$p|g"
done
```

- [ ] **Step 3: Verify build + tests**

Run: `go build ./... && make test`
Expected: PASS.

- [ ] **Step 4: Lint and commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add -A
git commit -m "refactor(petra): move domain/repository/service/notification under internal/petra

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 5 — Rename the petra app and colocate its UI

### Task 7: `cmd/web` → `cmd/petra`, move `ui/` in, make the template root app-relative

**Files:**
- Move: `cmd/web/` → `cmd/petra/`
- Move: `ui/templates/` → `cmd/petra/ui/templates/`, `ui/static/` → `cmd/petra/ui/static/`
- Modify: `cmd/petra/templates.go` — `resolveAndVerifyTemplatePath` to resolve relative to
  the app, not `<go.mod>/ui/templates`.
- Modify: any path references to `ui/` (Dockerfile, Makefile, `main.go` static FS wiring).

- [ ] **Step 1: Move the app and its UI**

```bash
cd /home/martin/petrapp
git mv cmd/web cmd/petra
mkdir -p cmd/petra/ui
git mv ui/templates cmd/petra/ui/templates
git mv ui/static    cmd/petra/ui/static
```

- [ ] **Step 2: Point the template/static roots at the app directory**

In `cmd/petra/templates.go`, change the default template path. The current
`resolveAndVerifyTemplatePath` falls back to `<moduleDir>/ui/templates`; update the fallback
to the app's ui dir:

```go
templatePath = filepath.Join(modulePath, "cmd", "petra", "ui", "templates")
```

Apply the analogous change wherever `ui/static` is opened in `main.go` (search for
`"ui"`/`"static"` / `os.DirFS`):

```bash
cd /home/martin/petrapp
grep -rn '"ui"\|ui/templates\|ui/static\|os.DirFS' cmd/petra/*.go
```

Update each hit to the `cmd/petra/ui/...` location.

- [ ] **Step 3: Fix Dockerfile and Makefile paths**

```bash
cd /home/martin/petrapp
grep -rn 'ui/static\|ui/templates\|cmd/web' Dockerfile Makefile fly.toml litestream.yml 2>/dev/null
```

Rewrite each `ui/static` → `cmd/petra/ui/static`, `ui/templates` →
`cmd/petra/ui/templates`, and `cmd/web` → `cmd/petra` (build target, binary path, fingerprint
step). Keep `fly.toml`/`litestream.yml` at the repo root (Fly infra stays petra-only per the
design).

- [ ] **Step 4: Verify build, tests, and a real run**

Run: `go build ./... && make test`
Expected: PASS.
Run (smoke the template resolution): `PETRAPP_ADDR=localhost:0 go run ./cmd/petra &` then
`curl -sf http://localhost:<port>/healthy` (or use the existing smoketest target). Expected:
templates load without "template path not found".

> If the project has a Playwright suite (`cmd/petra/playwright_test.go`), run
> `make test` which includes it; confirm it still finds templates/static.

- [ ] **Step 5: Lint and commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add -A
git commit -m "refactor(petra): rename cmd/web to cmd/petra and colocate ui/

Move templates/static under cmd/petra/ui and resolve the template root
relative to the app so a second app can own its own ui/.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 6 — Enforce the boundary

### Task 8: Add depguard rules for the platform/product split

**Files:**
- Modify: `.golangci.yml` (extend the existing `depguard.rules`, around line 96).

- [ ] **Step 1: Add boundary rules**

Under `linters.settings.depguard.rules`, add three rules alongside the existing ones:

```yaml
        "platform-stays-product-agnostic":
          files:
            - "**/internal/platform/**"
          deny:
            - pkg: github.com/myrjola/petrapp/internal/petra
              desc: platform/ must not depend on product code (internal/petra)
            - pkg: github.com/myrjola/petrapp/cmd
              desc: platform/ must not depend on any app (cmd/)
        "example-app-isolation":
          files:
            - "**/cmd/example/**"
          deny:
            - pkg: github.com/myrjola/petrapp/internal/petra
              desc: cmd/example must not import petra product code; share via internal/platform
        "petra-app-isolation":
          files:
            - "**/cmd/petra/**"
          deny:
            - pkg: github.com/myrjola/petrapp/internal/example
              desc: cmd/petra must not import another app's internal packages
```

- [ ] **Step 2: Verify the config is valid and currently passing**

Run: `make lint-fix`
Expected: no depguard violations (the reshuffle already removed any platform→petra edges; if
a violation appears, it is a real boundary leak to fix, not a config error).

- [ ] **Step 3: Prove the rule bites (temporary negative test)**

Temporarily add `import _ "github.com/myrjola/petrapp/internal/petra/domain"` to any file
under `internal/platform/obs/logging/`, then run `golangci-lint run ./internal/platform/...`.
Expected: depguard flags it. Remove the import.

- [ ] **Step 4: Commit**

```bash
cd /home/martin/petrapp
git add .golangci.yml
git commit -m "build(lint): enforce platform/product boundary with depguard

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 7 — Prove it: `cmd/example` todo app

This app deliberately *copies* the web middleware/render boilerplate from `cmd/petra` rather
than importing a shared Kit. It must compile and serve a list + detail page, gated behind
passkey login, using the shared `sqlitekit`, `auth`, `obs`, `envstruct`, and `testkit`.

### Task 9: Example domain, schema, and repository

**Files:**
- Create: `cmd/example/internal/todo/todo.go` (domain type — kept app-internal)
- Create: `cmd/example/schema.sql`, `cmd/example/embed.go`
- Create: `cmd/example/internal/todo/repository.go`, `cmd/example/internal/todo/repository_test.go`

> Example's product packages live under `cmd/example/internal/...` so they are private to the
> app and the depguard rule has nothing to leak into.

- [ ] **Step 1: Write the failing repository test**

Create `cmd/example/internal/todo/repository_test.go`:

```go
package todo_test

import (
	"testing"

	"github.com/myrjola/petrapp/cmd/example/internal/todo"
	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func newTestRepo(t *testing.T) *todo.Repository {
	t.Helper()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	db, err := sqlitekit.NewDatabase(t.Context(), sqlitekit.Config{
		URL:    ":memory:",
		Schema: auth.SchemaSQL + "\n" + todo.SchemaSQL,
		Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return todo.NewRepository(db)
}

func TestRepository_CreateListGetToggle(t *testing.T) {
	t.Parallel()
	repo := newTestRepo(t)
	ctx := t.Context()

	id, err := repo.Create(ctx, "buy milk", "2%")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	items, err := repo.List(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("List: items=%d err=%v", len(items), err)
	}
	got, err := repo.Get(ctx, id)
	if err != nil || got.Title != "buy milk" || got.Done {
		t.Fatalf("Get: %+v err=%v", got, err)
	}
	if err = repo.Toggle(ctx, id); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	got, _ = repo.Get(ctx, id)
	if !got.Done {
		t.Fatal("expected Done after Toggle")
	}
}
```

`todo.SchemaSQL` is referenced from the embed; for the test it is fine to reference the
package var (created in Step 3).

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./cmd/example/internal/todo/ -run TestRepository_CreateListGetToggle -v`
Expected: FAIL — package/types undefined.

- [ ] **Step 3: Create the domain type, schema, and embed**

`cmd/example/internal/todo/todo.go`:

```go
// Package todo is the example app's product domain — a minimal CRUD entity
// proving the shared platform plumbing.
package todo

import "time"

type Todo struct {
	ID      int
	Title   string
	Notes   string
	Done    bool
	Created time.Time
}
```

`cmd/example/schema.sql`:

```sql
CREATE TABLE todos
(
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    title   TEXT    NOT NULL,
    notes   TEXT    NOT NULL DEFAULT '',
    done    INTEGER NOT NULL DEFAULT 0,
    created TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
);
```

`cmd/example/embed.go`:

```go
package main

import _ "embed"

//go:embed schema.sql
var schemaSQL string
```

Also expose the schema to the `todo` package for tests — simplest is to duplicate the embed
in the todo package. Create `cmd/example/internal/todo/schema.go`:

```go
package todo

import _ "embed"

//go:embed schema.sql
var SchemaSQL string
```

and `cmd/example/internal/todo/schema.sql` (same DDL as above). (Two small embeds beat a
cross-package coupling here; the duplication is intentional and trivial.)

- [ ] **Step 4: Implement the repository**

`cmd/example/internal/todo/repository.go`:

```go
package todo

import (
	"context"
	"fmt"

	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

type Repository struct {
	db *sqlitekit.Database
}

func NewRepository(db *sqlitekit.Database) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, title, notes string) (int, error) {
	res, err := r.db.ReadWrite.ExecContext(ctx,
		"INSERT INTO todos (title, notes) VALUES (?, ?)", title, notes)
	if err != nil {
		return 0, fmt.Errorf("insert todo: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return int(id), nil
}

func (r *Repository) List(ctx context.Context) ([]Todo, error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx,
		"SELECT id, title, notes, done, created FROM todos ORDER BY id DESC")
	if err != nil {
		return nil, fmt.Errorf("query todos: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var todos []Todo
	for rows.Next() {
		var t Todo
		if err = rows.Scan(&t.ID, &t.Title, &t.Notes, &t.Done, &t.Created); err != nil {
			return nil, fmt.Errorf("scan todo: %w", err)
		}
		todos = append(todos, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate todos: %w", err)
	}
	return todos, nil
}

func (r *Repository) Get(ctx context.Context, id int) (Todo, error) {
	var t Todo
	err := r.db.ReadOnly.QueryRowContext(ctx,
		"SELECT id, title, notes, done, created FROM todos WHERE id = ?", id).
		Scan(&t.ID, &t.Title, &t.Notes, &t.Done, &t.Created)
	if err != nil {
		return Todo{}, fmt.Errorf("get todo %d: %w", id, err)
	}
	return t, nil
}

func (r *Repository) Toggle(ctx context.Context, id int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		"UPDATE todos SET done = NOT done WHERE id = ?", id); err != nil {
		return fmt.Errorf("toggle todo %d: %w", id, err)
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		"DELETE FROM todos WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete todo %d: %w", id, err)
	}
	return nil
}
```

- [ ] **Step 5: Verify the test passes**

Run: `go test ./cmd/example/internal/todo/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add cmd/example
git commit -m "feat(example): todo domain, schema, and repository on shared sqlitekit

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Example app wiring — main, config, DB, auth, copied middleware/render

**Files:**
- Create: `cmd/example/main.go`
- Create: `cmd/example/web.go` (copied/trimmed middleware + renderer from `cmd/petra`)
- Create: `cmd/example/ui/templates/base.gohtml`, `cmd/example/ui/templates/pages/...`
- Create: `cmd/example/main_test.go`

- [ ] **Step 1: Write the failing app-boot test**

Create `cmd/example/main_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func TestRoutes_HealthyOK(t *testing.T) {
	t.Parallel()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	app, cleanup, err := newTestApplication(t, logger)
	if err != nil {
		t.Fatalf("newTestApplication: %v", err)
	}
	t.Cleanup(cleanup)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthy", nil)
	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthy = %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./cmd/example/ -run TestRoutes_HealthyOK -v`
Expected: FAIL — `newTestApplication` / `routes` undefined.

- [ ] **Step 3: Implement `main.go` (config + wiring)**

`cmd/example/main.go` — small composition root using the shared platform packages. Model the
config struct and `run` pattern on `cmd/petra/main.go`, but trimmed:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexedwards/scs/sqlite3store"
	"github.com/alexedwards/scs/v2"
	"github.com/myrjola/petrapp/cmd/example/internal/todo"
	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/envstruct"
	"github.com/myrjola/petrapp/internal/platform/obs/logging"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

type config struct {
	Addr      string `env:"EXAMPLE_ADDR"       envDefault:"localhost:8082"`
	FQDN      string `env:"EXAMPLE_FQDN"       envDefault:"localhost"`
	SqliteURL string `env:"EXAMPLE_SQLITE_URL" envDefault:":memory:"`
}

type application struct {
	logger          *slog.Logger
	repo            *todo.Repository
	auth            *auth.WebAuthnHandler
	sessionManager  *scs.SessionManager
	renderer        *renderer
}

func main() {
	logger := slog.New(logging.NewContextHandler(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(logger); err != nil {
		logger.Error("startup failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	var cfg config
	if err := envstruct.Populate(&cfg, os.LookupEnv); err != nil {
		return fmt.Errorf("populate config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, cleanup, err := newApplication(ctx, logger, cfg)
	if err != nil {
		return fmt.Errorf("new application: %w", err)
	}
	defer cleanup()

	srv := &http.Server{ //nolint:exhaustruct // defaults are fine for the example.
		Addr:              cfg.Addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("listening", slog.String("addr", cfg.Addr))
	if err = srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

func newApplication(
	ctx context.Context, logger *slog.Logger, cfg config,
) (*application, func(), error) {
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:    cfg.SqliteURL,
		Schema: auth.SchemaSQL + "\n" + schemaSQL,
		Logger: logger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("new database: %w", err)
	}

	sessionManager := scs.New()
	sessionManager.Store = sqlite3store.New(db.ReadWrite)
	sessionManager.Lifetime = 24 * time.Hour

	authHandler, err := auth.New(
		cfg.Addr, cfg.FQDN, false, logger, sessionManager, auth.NewSQLiteStore(db))
	if err != nil {
		return nil, nil, fmt.Errorf("new auth handler: %w", err)
	}

	rnd, err := newRenderer()
	if err != nil {
		return nil, nil, fmt.Errorf("new renderer: %w", err)
	}

	app := &application{
		logger:         logger,
		repo:           todo.NewRepository(db),
		auth:           authHandler,
		sessionManager: sessionManager,
		renderer:       rnd,
	}
	return app, func() { _ = db.Close() }, nil
}

// newTestApplication is the test seam used by main_test.go.
func newTestApplication(
	t interface{ Context() context.Context }, logger *slog.Logger,
) (*application, func(), error) {
	return newApplication(t.Context(), logger, config{
		Addr:      "localhost:0",
		FQDN:      "localhost",
		SqliteURL: ":memory:",
	})
}
```

> Verify the exact `auth.New` parameter order and the `WebAuthnHandler` type name against
> `internal/platform/auth/handler.go` (Task 5 renamed the package but kept the type). Adjust
> field/param names to match.

- [ ] **Step 4: Implement `web.go` (routes + copied middleware + renderer)**

`cmd/example/web.go` — copy the small, generic pieces from `cmd/petra` (secure headers,
panic recovery, session load/save) and a minimal renderer. This is the deliberate copy:

```go
package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
)

type renderer struct {
	tmpl *template.Template
}

func newRenderer() (*renderer, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getwd: %w", err)
	}
	// Resolve templates relative to the module root, app-owned dir.
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("go.mod not found from working dir")
		}
		dir = parent
	}
	root := filepath.Join(dir, "cmd", "example", "ui", "templates")
	tmpl, err := template.ParseGlob(filepath.Join(root, "*.gohtml"))
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	if _, err = tmpl.ParseGlob(filepath.Join(root, "pages", "*.gohtml")); err != nil {
		return nil, fmt.Errorf("parse page templates: %w", err)
	}
	return &renderer{tmpl: tmpl}, nil
}

func (rnd *renderer) render(w http.ResponseWriter, status int, name string, data any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := rnd.tmpl.ExecuteTemplate(w, name, data); err != nil {
		return fmt.Errorf("execute template %q: %w", name, err)
	}
	return nil
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				app.logger.Error("panic recovered", "err", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthy", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /", app.handleList)
	mux.HandleFunc("GET /todos/{id}", app.handleDetail)
	mux.HandleFunc("POST /todos", app.handleCreate)
	mux.HandleFunc("POST /todos/{id}/toggle", app.handleToggle)
	mux.HandleFunc("POST /todos/{id}/delete", app.handleDelete)

	// Shared passkey auth: the handler exposes BeginLogin/FinishLogin/Logout/
	// BeginRegistration/FinishRegistration (no RegisterRoutes); wire thin
	// handlers around them, mirroring cmd/petra/handlers-webauthn.go.
	mux.HandleFunc("POST /api/registration/begin", app.beginRegistration)
	mux.HandleFunc("POST /api/registration/finish", app.finishRegistration)
	mux.HandleFunc("POST /api/login/begin", app.beginLogin)
	mux.HandleFunc("POST /api/login/finish", app.finishLogin)
	mux.HandleFunc("POST /api/logout", app.logout)
	// Demonstrate the shared gate without forcing a passkey ceremony on the
	// CRUD pages (kept open so handler tests need no auth). /account is gated.
	mux.Handle("GET /account", app.auth.AuthenticateMiddleware(
		http.HandlerFunc(app.handleAccount)))

	var handler http.Handler = mux
	handler = app.recoverPanic(handler)
	handler = secureHeaders(handler)
	handler = app.sessionManager.LoadAndSave(handler)
	return handler
}
```

The thin auth handlers (`beginRegistration`, `finishRegistration`, `beginLogin`,
`finishLogin`, `logout`) and `handleAccount` are copied/trimmed from
`cmd/petra/handlers-webauthn.go` — see Task 11 Step 3a.

> **Scope note (auth gating):** the design called the example "login-gated." Forcing the
> whole todo CRUD behind a passkey ceremony would require the webauthn client JS and would
> make the handler tests authenticate, which buys little verification for a lot of
> complexity (exactly the copying-over-dependency trade-off the design warns about). This
> plan instead *constructs and wires* the shared auth (proving `Store` + `SchemaSQL` +
> `AuthenticateMiddleware` compose) and gates a single `/account` route, while keeping the
> CRUD open and testable. If you want full CRUD gating, add `app.auth.AuthenticateMiddleware`
> around the todo routes and seed an authenticated session in the handler tests.

- [ ] **Step 5: Create minimal templates**

`cmd/example/ui/templates/base.gohtml`:

```gohtml
{{define "base"}}<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>{{block "title" .}}Todos{{end}}</title></head>
<body>{{block "main" .}}{{end}}</body>
</html>{{end}}
```

`cmd/example/ui/templates/pages/list.gohtml`:

```gohtml
{{define "list"}}{{template "base" .}}{{end}}
{{define "title"}}Todos{{end}}
{{define "main"}}
<h1>Todos</h1>
<form method="post" action="/todos">
  <input name="title" placeholder="title" required>
  <button type="submit">Add</button>
</form>
<ul>
{{range .Todos}}
  <li>
    <a href="/todos/{{.ID}}">{{.Title}}</a>{{if .Done}} ✓{{end}}
  </li>
{{end}}
</ul>
{{end}}
```

`cmd/example/ui/templates/pages/detail.gohtml`:

```gohtml
{{define "detail"}}{{template "base" .}}{{end}}
{{define "title"}}{{.Todo.Title}}{{end}}
{{define "main"}}
<h1>{{.Todo.Title}}</h1>
<p>{{.Todo.Notes}}</p>
<p>Done: {{.Todo.Done}}</p>
<form method="post" action="/todos/{{.Todo.ID}}/toggle"><button>Toggle</button></form>
<form method="post" action="/todos/{{.Todo.ID}}/delete"><button>Delete</button></form>
<a href="/">Back</a>
{{end}}
```

- [ ] **Step 6: Verify the boot test passes**

Run: `go test ./cmd/example/ -run TestRoutes_HealthyOK -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add cmd/example
git commit -m "feat(example): app wiring with copied web boilerplate over shared platform

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Example handlers (list, detail, create, toggle, delete)

**Files:**
- Create: `cmd/example/handlers.go`
- Create: `cmd/example/handlers_test.go`

- [ ] **Step 1: Write the failing handler test**

Create `cmd/example/handlers_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func TestHandlers_CreateThenListShowsItem(t *testing.T) {
	t.Parallel()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	app, cleanup, err := newTestApplication(t, logger)
	if err != nil {
		t.Fatalf("newTestApplication: %v", err)
	}
	t.Cleanup(cleanup)
	h := app.routes()

	form := url.Values{"title": {"walk the dog"}}
	postReq := httptest.NewRequest(http.MethodPost, "/todos", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusSeeOther {
		t.Fatalf("POST /todos = %d, want 303", postRec.Code)
	}

	listRec := httptest.NewRecorder()
	h.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/", nil))
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "walk the dog") {
		t.Fatalf("GET / = %d, body missing item: %s", listRec.Code, listRec.Body.String())
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `go test ./cmd/example/ -run TestHandlers_CreateThenListShowsItem -v`
Expected: FAIL — handler methods undefined.

- [ ] **Step 3: Implement handlers**

`cmd/example/handlers.go`:

```go
package main

import (
	"net/http"
	"strconv"
)

type listData struct {
	Todos []todoView
}

type detailData struct {
	Todo todoView
}

type todoView struct {
	ID    int
	Title string
	Notes string
	Done  bool
}

func (app *application) handleList(w http.ResponseWriter, r *http.Request) {
	items, err := app.repo.List(r.Context())
	if err != nil {
		app.serverError(w, err)
		return
	}
	views := make([]todoView, 0, len(items))
	for _, it := range items {
		views = append(views, todoView{ID: it.ID, Title: it.Title, Notes: it.Notes, Done: it.Done})
	}
	if err = app.renderer.render(w, http.StatusOK, "list", listData{Todos: views}); err != nil {
		app.serverError(w, err)
	}
}

func (app *application) handleDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	it, err := app.repo.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data := detailData{Todo: todoView{ID: it.ID, Title: it.Title, Notes: it.Notes, Done: it.Done}}
	if err = app.renderer.render(w, http.StatusOK, "detail", data); err != nil {
		app.serverError(w, err)
	}
}

func (app *application) handleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	title := r.PostFormValue("title")
	if title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	if _, err := app.repo.Create(r.Context(), title, r.PostFormValue("notes")); err != nil {
		app.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) handleToggle(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err = app.repo.Toggle(r.Context(), id); err != nil {
		app.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/todos/"+strconv.Itoa(id), http.StatusSeeOther)
}

func (app *application) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err = app.repo.Delete(r.Context(), id); err != nil {
		app.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (app *application) serverError(w http.ResponseWriter, err error) {
	app.logger.Error("server error", "err", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
```

- [ ] **Step 3a: Copy the thin auth handlers from petra**

Create `cmd/example/handlers-auth.go` by copying and trimming
`cmd/petra/handlers-webauthn.go` (it already wraps the shared handler's methods). The example
needs no flash/redirect helpers, so keep it minimal:

```go
package main

import "net/http"

func (app *application) beginRegistration(w http.ResponseWriter, r *http.Request) {
	out, err := app.auth.BeginRegistration(r.Context())
	if err != nil {
		app.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (app *application) finishRegistration(w http.ResponseWriter, r *http.Request) {
	if err := app.auth.FinishRegistration(r); err != nil {
		app.serverError(w, err)
	}
}

func (app *application) beginLogin(w http.ResponseWriter, r *http.Request) {
	out, err := app.auth.BeginLogin(r)
	if err != nil {
		app.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out) //#nosec G705 -- structured WebAuthn challenge, not raw user input.
}

func (app *application) finishLogin(w http.ResponseWriter, r *http.Request) {
	if err := app.auth.FinishLogin(r); err != nil {
		app.serverError(w, err)
	}
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	if err := app.auth.Logout(r.Context()); err != nil {
		app.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleAccount is the one gated page, proving AuthenticateMiddleware composes.
func (app *application) handleAccount(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("account (authenticated)"))
}
```

> `cmd/petra` handles `UnknownCredentialError` from `FinishLogin` specially; the example
> deliberately omits that branch (copying only what it needs). If you later want the full UX,
> port that block from `cmd/petra/handlers-webauthn.go`.

- [ ] **Step 4: Verify handler test + whole suite**

Run: `go test ./cmd/example/ -v`
Expected: PASS.
Run: `go build ./... && make test`
Expected: PASS (entire repo).

- [ ] **Step 5: Commit**

```bash
cd /home/martin/petrapp
make lint-fix
git add cmd/example
git commit -m "feat(example): list/detail/create/toggle/delete handlers

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: Full validation and README note

**Files:**
- Modify: `README.md` (document the platform/petra/example layout)

- [ ] **Step 1: Run the full CI pipeline**

Run: `make ci`
Expected: build + lint + test + sec all pass. depguard reports no boundary violations.

- [ ] **Step 2: Document the layout**

Add a short "Repository layout" section to `README.md` explaining `internal/platform/`
(shared infra), `internal/petra/` (fitness product), and `cmd/petra` + `cmd/example` (apps),
plus the depguard boundary rule and the deliberate "copy the web boilerplate" decision with
a pointer to this plan and the design doc.

- [ ] **Step 3: Commit**

```bash
cd /home/martin/petrapp
git add README.md
git commit -m "docs: describe platform/petra/example monorepo layout

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-review notes (for the implementer)

- **Schema ordering matters:** `auth.SchemaSQL` must precede `repository.SchemaSQL` (and the
  example's `schemaSQL`) because product/foreign-key references point at the auth `users`
  table. Every `Schema:` assembly in this plan uses `auth.SchemaSQL + "\n" + <product>`.
- **`is_admin`** stays on the auth-owned `users` table; the `IsAdmin` context helper stays in
  `internal/platform/contexthelpers`. No split was needed.
- **Mechanical sweeps** (Tasks 1–6) rely on `go build` to surface any missed reference — run
  it before `make test` each time.
- **Verify auth API names** in Tasks 5/10 against the real `handler.go` (type
  `WebAuthnHandler`, constructor `New`, route registration). The plan marks each spot.
- **`make test` includes Playwright** for `cmd/petra`; after Task 7 confirm templates/static
  resolve from the new `cmd/petra/ui` location during that suite.
