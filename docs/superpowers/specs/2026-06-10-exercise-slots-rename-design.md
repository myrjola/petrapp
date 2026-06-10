# Design: rename `workout_exercises` → `exercise_slots`

**Date:** 2026-06-10
**Status:** Approved, ready for implementation plan

## Problem

The table `workout_exercises` models the domain's **Exercise slot** concept
(CONTEXT.md: "one position in a session: an exercise plus its sets; identity
*is* the position"). The glossary lists "exercise (bare)" as an alias to avoid
for that term, yet the table uses exactly that bare word — and it collides
conceptually with `exercises`, the movement catalogue. The Go layer already
speaks the right language (`domain.ExerciseSlot`, `Session.Slots`,
repository helpers named `saveOneSlotInTx`), so the schema is the lone holdout.
Per the CONTEXT.md reconciliation rule (when code and the glossary disagree,
fix one of them), the fix is to rename the table to `exercise_slots`.

## Scope

- **In:** rename the table and its index; repoint the two child foreign keys;
  swap the table token in repository SQL and tests; a data-preserving
  premigration; a generalisable docs update.
- **Out:** renaming `workout_sessions` (assessment finding #2) — a much larger
  blast radius, deferred to a separate effort.
- **Columns:** audited and left unchanged. `position`, `exercise_id`, and
  `warmup_completed_at` are already glossary-aligned; `workout_user_id` /
  `workout_date` are the foreign key to the *unchanged* `workout_sessions`, so
  renaming them would create inconsistency with their parent.

## Design

### 1. Schema (`internal/petra/repository/schema.sql`)

- Rename table `workout_exercises` → `exercise_slots`. All columns unchanged.
- Rename index `workout_exercises_user_exercise_date_idx` →
  `exercise_slots_user_exercise_date_idx`.
- Repoint the child foreign keys that name the table:
  - `exercise_sets`: `REFERENCES workout_exercises (...)` → `exercise_slots`.
  - `scheduled_pushes`: `REFERENCES workout_exercises (...)` → `exercise_slots`.
- The intra-table FK `FOREIGN KEY (workout_user_id, workout_date) REFERENCES
  workout_sessions (...)` stays as-is.

### 2. Premigration

The declarative migrator is structural only: it would read the rename as
"`workout_exercises` dropped (data lost), `exercise_slots` added (empty)". A
premigration performs the rename first so the migrator sees a matching schema.

New exported `repository.PreMigrateExerciseSlots(ctx context.Context, db
*sqlitekit.Database) error`:

- **Guard (idempotent + fresh-DB safe):** query `sqlite_master` for table
  `workout_exercises`. If absent, return nil. This short-circuits both the
  already-migrated production database and fresh/in-memory test databases
  (which have no tables at premigration time and receive `exercise_slots`
  directly from `schema.sql`).
- **Action:** on a single pinned connection (`conn, err :=
  db.ReadWrite.Conn(ctx)`; `defer conn.Close()` — `*sql.DB` is a pool, so the
  PRAGMA/DDL must share one connection), inside a transaction with rollback on
  error: `ALTER TABLE workout_exercises RENAME TO exercise_slots`.
- SQLite auto-rewrites the FK references in `exercise_sets` and
  `scheduled_pushes` to the new table name (relies on `legacy_alter_table`
  being OFF — the modern default; the test is the guardrail).
- `RENAME TO` does not revalidate foreign keys, so no `PRAGMA foreign_keys =
  OFF` is required (a simplification over the documented copy pattern).
- The old index keeps its old name on the renamed table; the premigration
  leaves it for the declarative migrator to drop and recreate under the new
  name. Indexes carry no data, so this is free.

### 3. Premigration test (`package repository_test`)

Because `schema.sql` no longer contains the legacy shape, reproduce it in a
const (the `workout_exercises` table + its index). Then:

1. Build a DB with the legacy shape; seed `workout_exercises` rows plus child
   `exercise_sets` and `scheduled_pushes` rows.
2. Call `PreMigrateExerciseSlots` → assert `workout_exercises` is gone,
   `exercise_slots` exists with the same data, and the child rows still
   resolve their FK to the renamed table.
3. Call it again → idempotent no-op (no error, the guard short-circuits).
4. Call it on an empty database → returns nil.
5. Run `migrateTo(schema.sql)` over the renamed database → succeeds with no
   further structural surprise; the index is reconciled to the new name.

### 4. Wiring and call-side churn

- Set `Premigration: repository.PreMigrateExerciseSlots` in
  `cmd/petra/main.go` and `cmd/migratetest/main.go` (the latter so
  `make migratetest` exercises it against a production snapshot).
  `cmd/example/main.go` stays `nil`.
- Mechanical token swap `workout_exercises` → `exercise_slots` in the
  repository SQL strings (`shared.go`, `sessions.go`, `exercises.go`,
  `repository.go`) and the service/handler tests that embed the table name.
  Verify each hit is the table name, not a column. **No domain-model changes** —
  `ExerciseSlot` / `Slots` already match.

### 5. Docs update (generalisable)

In `internal/petra/repository/CLAUDE.md` "Premigration escape hatch", add
native `ALTER TABLE … RENAME` as the recommended premigration for **pure
renames**:

- Table rename via `RENAME TO`, column rename via `RENAME COLUMN`.
- It auto-updates child FK references and lets the structural migrator
  reconcile indexes for free — lighter than the `CREATE *_new → INSERT…SELECT
  → DROP → RENAME` pattern, which stays the path for genuine data *reshaping*.
- Note the pool caveat (pin one `Conn`) and that `RENAME` skips FK-disabling.

### 6. Lifecycle

The premigration is temporary. Once production has booted past it, delete
`PreMigrateExerciseSlots`, both `main.go` wirings, the test, and the
legacy-shape const in a single commit (no version table exists; the only
signal it is obsolete is that production has booted past it). The docs guidance
is permanent and stays.

## Testing

- `make test` — repository round-trip tests confirm the renamed table and
  queries work end-to-end; the premigration test confirms the data-preserving
  rename, idempotence, fresh-DB short-circuit, and migrator acceptance.
- `make migratetest` — exercises the premigration against a production snapshot
  via the `cmd/migratetest` wiring.

## Risks

- **FK auto-rewrite depends on `legacy_alter_table` being off.** This is the
  SQLite default; the premigration test verifies the child FKs point at
  `exercise_slots` after the rename, so a regression surfaces in CI.
- **Missed token in a SQL string.** STRICT-mode queries against a non-existent
  table fail loudly; the existing repository/service tests cover the query
  paths.
