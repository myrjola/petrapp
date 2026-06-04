# sqlitekit — Generic SQLite Engine

`internal/platform/sqlitekit` is the **product-agnostic** SQLite kit shared by
every app in the module (`cmd/petra`, `cmd/example`, `cmd/migratetest`). It is
platform infrastructure: it must not import `internal/petra` or `cmd/` (enforced
by the depguard rules in `.golangci.yml`).

It owns the database *engine* — the declarative migrator and the connection
handle — but **not** any concrete schema. Schema and seed data are
caller-provided via `Config` (see "Caller-provided schema" below).

## What lives here

- **The `*sqlitekit.Database` connection handle** (`sqlite.go`): the `ReadWrite`
  / `ReadOnly` `*sql.DB` split, `Config`-based `NewDatabase`, PRAGMA / DSN
  setup, `HealthCheck`, and `Close`.
- **The declarative migrator** (`migrate.go`): diffs the live database against
  the caller's schema string and rebuilds tables as needed.
- **The background optimizer** (`optimizer.go`): periodic `PRAGMA optimize`,
  started by `NewDatabase` and torn down by `Close`.
- **The user/db plumbing** (`userdb.go`) used by the connection handle.

## What does NOT live here

- **Any concrete `schema.sql` / `fixtures.sql` / `embed.go`.** Those belong to
  the product that owns the tables. The Petra schema lives in
  `internal/petra/repository/` (see `internal/petra/repository/CLAUDE.md`);
  `cmd/example` has its own. sqlitekit no longer embeds any DDL.
- **Schema-design guidance** (STRICT-mode patterns, FK conventions, the
  premigration escape hatch, schema-evolution process across
  domain/repository/service). That guidance now lives with the product schema
  in `internal/petra/repository/CLAUDE.md`.
- SQL queries and repository implementations — `internal/petra/repository/`.
- Domain models — `internal/petra/domain/`. Service orchestration —
  `internal/petra/service/`. HTTP handlers — `cmd/petra/`.

## Caller-provided schema: `Config` and `NewDatabase`

`NewDatabase(ctx, Config{URL, Schema, Fixtures, Logger})` connects, migrates the
database to `Schema`, then applies `Fixtures` (if non-empty):

- **`URL`** — a file path, or `:memory:` for tests (in-memory databases use
  `cache=shared` so the RO and RW handles see the same data).
- **`Schema`** — the required declarative DDL, the single source of truth the
  migrator drives toward. The caller assembles it. Petra concatenates
  **`auth.SchemaSQL` ahead of the product schema** so that product tables may
  reference the shared auth tables (e.g. `users`) via foreign keys:

  ```go
  db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
      URL:      cfg.SqliteURL,
      Schema:   auth.SchemaSQL + "\n" + repository.SchemaSQL,
      Fixtures: repository.FixturesSQL,
      Logger:   logger,
  })
  ```

  `cmd/example/main.go` does the same with its own `schemaSQL`, and
  `cmd/migratetest` reuses the Petra assembly. The ordering matters: auth tables
  must exist before product tables that reference them.
- **`Fixtures`** — optional idempotent seed SQL, re-applied via a single
  `ExecContext` **on every boot**. Must coexist with rows production already
  holds; design seeds to be upsert-safe.
- **`Logger`** — a `*slog.Logger`; no global logger.

## RO / RW connection split

`Database` exposes two pools:

- **`ReadWrite`** — one connection (`SetMaxOpenConns(1)`), `mode=rwc`,
  `_txlock=immediate`. All writes and migrations go through it.
- **`ReadOnly`** — up to ten connections, `mode=ro` + `_query_only=true`,
  `_txlock=deferred`. Reads scale here.

Both register optimized drivers with shared PRAGMAs (`temp_store=memory`,
`mmap_size`, `wal_autocheckpoint=0` — Litestream owns checkpoints). The
read-only driver additionally enables `read_uncommitted` for in-memory
shared-cache test databases. See the comments in `sqlite.go` for the DSN
rationale (one `mode=` per URL, the WAL/`:memory:` interaction, the prepared
statement cache).

`HealthCheck` runs `SELECT 1` against the read-only pool — it exercises the full
checkout + execute path, so unlike a bare `PingContext` it catches a pool
handing out connections to a file that can no longer be read.

## The declarative migrator

`migrateTo` (in `migrate.go`) makes the live database match the caller's schema
string without hand-written migration scripts. It is purely **structural**:

1. Drops tables removed from the schema.
2. Creates new tables.
3. Migrates changed tables via SQLite's 12-step `ALTER TABLE` dance
   (`CREATE *_new` → copy → drop → rename), with foreign-key validation
   temporarily disabled and re-enabled in a `defer`.
4. Synchronises triggers and indexes.

It is inspired by
<https://david.rothlis.net/declarative-schema-migration-for-sqlite/>. Because it
only reconciles *structure*, it cannot populate new columns, re-key foreign
keys, or otherwise transform existing rows. Data-shape transformations are the
product's responsibility (the premigration pattern documented in
`internal/petra/repository/CLAUDE.md`).

### Testing the migrator

`migrate_internal_test.go::TestDatabase_migrate` is the table-driven home for
testing the migrator engine itself. Add a case there when your change exercises
a new diff/apply pattern (column add/drop, table rename, constraint change,
index/trigger sync). Each case is `{name, schemaDefinitions[], testQueries[],
wantErr}`: the migrator is applied for each schema in sequence, then
`testQueries` run against the final state and either succeed or are expected to
error. This is engine-level coverage with throwaway tables — product schema
round-trips are tested in `internal/petra/repository/*_test.go`, not here.
