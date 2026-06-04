# Repository — SQLite Persistence

The `internal/petra/repository` package is the canonical persistence layer
for the workout bounded context. It depends on `internal/petra/domain`,
`internal/platform/sqlitekit`, and `internal/platform/contexthelpers` only —
no HTTP, no template logic, no business orchestration.

It also **owns the Petra product schema**: `schema.sql`, `fixtures.sql`, and
`embed.go` (which exports `SchemaSQL` / `FixturesSQL`). The generic migration
engine lives in `internal/platform/sqlitekit`; this package supplies the DDL it
drives toward. See "Schema & migrations" at the bottom.

## What lives here

- **SQLite implementations** of the eight repository contracts:
  `WeekPlanRepository`, `SessionRepository`, `ExerciseRepository`,
  `PreferencesRepository`, `FeatureFlagRepository`,
  `MuscleGroupTargetRepository`, `PushSubscriptionRepository`,
  `ScheduledPushRepository`. Implementations are unexported
  (`sqliteWeekPlanRepository`, `sqliteSessionRepository`, etc.).
  `WeekPlanRepository` owns workout writes at week scope; `SessionRepository`
  is read-only (see "Update closure contract" below).
- **The `Repositories` composite struct** plus the single public
  constructor `New(db *sqlitekit.Database) *Repositories` that wires every
  repository together.
- **Shared helpers** in `shared.go`:
  - Format primitives — `parseTimestamp`, `formatTimestamp`, `formatDate`,
    the `queryer` interface, the `baseRepository` mixin, and the
    timestamp/date format constants.
  - `fetchMuscleGroupsByExerciseID` — batched muscle-group hydration
    shared by the exercise and session reads.
  - Session-shaped persistence on `baseRepository` (write helpers in
    `shared.go`, read helpers in `sessions.go`): `insertSessionRowInTx`,
    `saveOneSlotInTx` (writes one slot at a caller-supplied position),
    `saveExerciseSetsInTx` (single pass over `Session.Slots`,
    passing each slot's array index as `position`), `insertSessionInTx`
    (the composite of row + sets), `deleteWeekInTx`, plus read-side
    `listSessionRows`, `listSessionRowsBetween`, and
    `loadExerciseSetsSince`. Shared by `WeekPlanRepository.Update`'s
    single-pass reinsert and the `SessionRepository` reads. If a third
    repository starts using them, consider splitting into a dedicated
    `session_persistence.go`.

## What does NOT live here

- Business rules — those live as aggregate methods on `domain.Session`
  (or as pure functions in the domain package).
- HTTP handlers, template shaping, response serialisation — `cmd/petra`.
- Service orchestration, AI exercise generation, GDPR export, anything
  that combines multiple aggregates or external systems —
  `internal/service`.
- Tests of business behaviour — those belong in `internal/petra/domain` (pure
  unit) or `internal/service` (orchestration/e2e). Repository tests
  cover repository-shape contracts: round-trip persistence, error
  translation, slot-position stability across `Update`.

## Update closure contract

`WeekPlanRepository.Update` and `ExerciseRepository.Update` accept a
closure `func(*domain.X) error` that runs inside an open transaction:

- The repo loads the aggregate (hydrating exercise data for sessions),
  runs the closure, persists the result on `nil`, rolls back on error.
- The closure expresses business invariants by calling aggregate methods
  on `domain.WeekPlan` / `domain.Session` (e.g. `wp.Start(date, now)`);
  domain sentinels (`ErrAlreadyStarted`, `ErrSlotNotFound`, etc.)
  propagate to the caller unchanged so service-layer code can
  `errors.Is` against them.
- Returning a non-domain error rolls back too — the repo doesn't try to
  classify; service code decides whether the error is fatal.

`SessionRepository` is read-only — every write that touches workout
data goes through `WeekPlanRepository.Update`, which holds the
transactional boundary at week scope.

## Diff strategy: delete-and-reinsert

`WeekPlanRepository.Update` persists by deleting every
`workout_sessions` row in `[monday, monday+6]` inside the tx (CASCADE
clears `workout_exercises` and `exercise_sets`) and re-inserting the
sessions in a single pass. Slot identity is the array index in
`Session.Slots`, written into the row's `position` column —
there is no autoincrement, so the order in which slots are inserted
does not matter and there is nothing for SQLite to collide on. For
PetrApp's data sizes (a handful of exercises × a handful of sets per
session) the cost is negligible and the simplicity is worth the trade.

## Hydration policy

`SessionRepository.Get` and `List` always populate `ExerciseSlot.Exercise`
inline: the base exercise columns are joined in via
`workout_exercises.exercise_id → exercises.id`, and primary/secondary
muscle groups are fetched in a single follow-up query keyed by the
deduped exercise IDs. `Get` costs two read queries on top of the session
row (the sets+exercise join plus the batched muscle-group fetch).
`List` stays flat regardless of how many sessions it returns: one query
for the session rows, one batched sets+exercise join over the whole date
range, and one muscle-group query — three queries total, not the prior
1 + 2N per session (see `loadExerciseSetsSince`). Callers receive a
"fully hydrated" `domain.Session` and never need to enrich it themselves.

## ErrNotFound translation at the boundary

Every read method translates `sql.ErrNoRows` to `domain.ErrNotFound`
explicitly. Callers `errors.Is(err, domain.ErrNotFound)` to detect
missing rows; they never see `sql.ErrNoRows` directly. This is the
symbolic move that ends "persistence leaks into the domain": the domain
sentinel has no SQL ancestry.

## Adding a new repository

1. Declare the interface in `internal/petra/repository/repository.go`.
2. Add the SQLite implementation in a new file (e.g. `widgets.go`) with
   an unexported struct `sqliteWidgetRepository` and an unexported
   constructor `newSQLiteWidgetRepository`.
3. Wire the new repo into `Repositories` and `New`.
4. Add round-trip tests in `widgets_test.go` (external `package
   repository_test`) using the `setupTestRepos` helper.

## Testing

Repository tests live in `package repository_test` (external) and run
against a real in-memory SQLite database — no mocks. They verify the
SQL-shaped contract, not the business rule.

- **Use `setupTestRepos`** (in `helpers_test.go`) to get a fresh
  database and the wired `*Repositories`. `seedWorkoutExercise` is a
  reusable fixture for tests that need a session + exercise row.
- **What to assert:**
  - Round-trip persistence — write a domain value, read it back,
    confirm the round-trip preserves every field including hydration.
  - Error translation at the boundary — missing rows surface as
    `domain.ErrNotFound` via `errors.Is`, never as `sql.ErrNoRows`.
  - Update-closure semantics — on `nil` from the closure the change
    persists; on a returned error the transaction rolls back and the
    on-disk state is unchanged.
  - Slot-position stability — each slot's array index in
    `Session.Slots` survives the delete-and-reinsert cycle inside
    `WeekPlanRepository.Update`, including across `SwapExerciseInSlot`.
- **What NOT to test here:** business rules and aggregate invariants
  (those live in `internal/petra/domain/`), and end-to-end orchestration
  across multiple aggregates (that lives in `internal/petra/service/`).

See `exercises_test.go` for a tight worked example covering all four
assertions above.

## Schema & migrations

This package owns the Petra product schema and seed data; the generic
declarative migrator and `Config`-based `NewDatabase` live in
`internal/platform/sqlitekit` (see that package's `CLAUDE.md`).

- **`schema.sql`** — the declarative single source of truth for the Petra
  schema. The migrator compares the live database to this file and applies the
  diff; you never hand-write `ALTER TABLE` scripts.
- **`fixtures.sql`** — seed data re-applied on every boot.
- **`embed.go`** — `//go:embed`s both files and exports `SchemaSQL` /
  `FixturesSQL`. Callers (`cmd/petra`, `cmd/migratetest`) concatenate
  `auth.SchemaSQL` **ahead of** `repository.SchemaSQL` so product tables may
  reference the shared auth tables (e.g. `users`) via foreign keys, then pass
  the result as `sqlitekit.Config{Schema, Fixtures, ...}`.

### Schema evolution process

When a change spans layers, work outwards from `schema.sql`:

1. **Update `schema.sql`** with your change (new columns, tables, constraints).
2. **Update Go models** in both `internal/petra/domain/` and this repository
   layer.
3. **Update repository SQL queries** (SELECT/INSERT/UPDATE) to include new
   fields.
4. **Update service-layer** conversion functions between domain and repository
   types in `internal/petra/service/`.
5. **Test with `make test`** to confirm migrations and queries work.
6. **Add fixtures** in `fixtures.sql` if new data needs seeding.

### Premigration escape hatch

The declarative migrator in `internal/platform/sqlitekit/migrate.go` is purely
**structural**: it diffs the live schema against the schema string and rebuilds
tables when needed, but it cannot infer how to populate new columns, re-key
foreign keys, or otherwise transform existing rows. For changes the migrator
cannot express, run a one-shot **premigration** that rewrites legacy data into
the new shape *before* the declarative migrate, after which the migrator sees a
database that already matches `schema.sql` and is a no-op.

A premigration method must:

- **Detect already-migrated state first** via `pragma_table_info` or
  `sqlite_master` and return early. It must also short-circuit on a fresh
  database (no legacy table) so test/in-memory startups skip it. Idempotent —
  safe to run on every boot.
- **Disable foreign keys** (`PRAGMA foreign_keys = OFF`) before the table swap;
  the declarative migrator re-enables them in its own `defer`.
- Run the rewrite inside a single transaction with rollback on error.
- Use the `CREATE TABLE *_new` → `INSERT … SELECT` → `DROP TABLE` →
  `ALTER TABLE … RENAME` pattern. When merging data sources (e.g. legacy rows +
  rows synthesized from a child table), `UNION` them in the `INSERT … SELECT`.

Test it by reproducing the pre-migration table shapes in a const (the live
`schema.sql` no longer contains them), seeding realistic edge-case data, calling
the premigration, asserting post-state, calling it again to prove idempotence,
and finally confirming the declarative migrator accepts the rewritten schema
without further changes. After a premigration has run in production, **delete it,
its call site, its test, and the legacy-schema fixture in the same commit** —
there is no version table, so the only signal it is no longer needed is that
production has booted past it.

### Table design patterns

#### STRICT mode and constraints

Always use these patterns for new tables:

```sql
CREATE TABLE table_name
(
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL CHECK (LENGTH(name) < 256),
    created_at  TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at),
    is_active   INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
) STRICT;
```

- **Always use `STRICT` mode** for type safety.
- **Use `WITHOUT ROWID`** for tables that do not have an integer primary key.
- **Always include length constraints** for TEXT fields:
  `CHECK (LENGTH(field) < N)`.
- **Use CHECK constraints for enums**:
  `CHECK (status IN ('pending', 'active', 'completed'))`.
- **Use proper foreign-key constraints** with CASCADE behavior where
  appropriate.

#### Timestamp patterns

Use ISO 8601 timestamps with automatic triggers for updates:

```sql
created_at TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
    CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at),
updated_at TEXT NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
    CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', updated_at) = updated_at)

-- Include update trigger
CREATE TRIGGER table_name_updated_timestamp
    AFTER UPDATE ON table_name
BEGIN
    UPDATE table_name SET updated_at = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE id = old.id;
END;
```

#### Foreign-key patterns

Match the FK column type to the referenced primary key. In this schema
`users.id` is `INTEGER` and `credentials.id` is `BLOB` (the WebAuthn credential
ID), so pick accordingly:

```sql
-- Standard foreign key with cascade (users.id is INTEGER)
user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE

-- BLOB foreign key when referencing credentials.id
credential_id BLOB NOT NULL REFERENCES credentials (id) ON DELETE CASCADE

-- Deferred foreign key (for complex relationships)
FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED

-- Composite foreign key
FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE
```

### Fixtures and conflict handling

`fixtures.sql` is re-applied on every boot (a single `ExecContext` in
`sqlitekit.NewDatabase`). Production may hold rows the fixture doesn't know
about — manually backfilled data — and the seed must coexist with them. When
changing fixtures, consider using the fly-ops skill to fetch a production
snapshot and verify you won't destroy existing data.

### Testing schema changes

Engine-level migrator behavior (column add/drop, table rename, constraint
change, index/trigger sync) is tested in
`internal/platform/sqlitekit/migrate_internal_test.go::TestDatabase_migrate`.
Petra schema round-trips are covered end-to-end by the repository tests in this
package; you usually do not need both.
