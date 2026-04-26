# Database Schema Guidelines - SQLite Layer

Guidelines for working with database schema, migrations, and data access patterns in `internal/sqlite/`.

## Schema Architecture

### Declarative Schema Management

- **All schema changes go in `schema.sql`** - this is the single source of truth
- The migration system compares current database to `schema.sql` and automatically applies changes

### Schema Evolution Process

1. **Update `schema.sql`** with your changes (new columns, tables, constraints, etc.)
2. **Update Go models** in both domain (`internal/workout/models.go`) and repository layer
3. **Update repository SQL queries** (SELECT, INSERT, UPDATE) to include new fields
4. **Update service layer** conversion functions between domain and repository types
5. **Test with `make test`** to ensure migrations and queries work correctly
6. **Add test fixtures** in `fixtures.sql` if needed for new data that needs to be seeded

### Premigration Escape Hatch

The declarative migrator in `migrate.go` is purely structural: it diffs the live schema against
`schema.sql` and rebuilds tables when needed, but it cannot infer how to populate new columns,
re-key foreign keys, or otherwise transform existing rows. For changes the migrator cannot express,
add a one-shot **premigration** that runs *before* `migrateTo` and rewrites the legacy data into
the new shape. After the rewrite, the declarative migrator sees a database that already matches
`schema.sql` and is a no-op.

When you need a premigration:

1. Create `internal/sqlite/premigrate.go` with a `(db *Database) preMigrate<Name>(ctx)` method
   that:
   - **Detects already-migrated state first** via `pragma_table_info` or `sqlite_master` and
     returns early. Must also short-circuit on a fresh database (no legacy table) so
     test/in-memory startups skip it. Idempotent — safe to run on every boot.
   - **Disables foreign keys** (`PRAGMA foreign_keys = OFF`) before the table swap. The
     declarative migrator re-enables them in its own `defer`.
   - Runs the rewrite inside a single transaction with rollback on error.
   - Uses the `CREATE TABLE *_new` → `INSERT … SELECT` → `DROP TABLE` → `ALTER TABLE … RENAME`
     pattern. When merging data sources (e.g., legacy rows + rows synthesized from a child
     table), `UNION` them in the `INSERT … SELECT`.
2. Wire it into `NewDatabase` in `sqlite.go` *between* `connect` and `migrateTo`.
3. Add a test in `migrate_internal_test.go` that:
   - Defines a `legacyWorkoutSchema`-style const reproducing the pre-migration table shapes
     (the live `schema.sql` no longer contains them).
   - Seeds realistic edge-case data (e.g., child rows with no parent, NULL columns).
   - Calls the premigration, asserts post-state, calls it again to prove idempotence, then
     calls `migrateTo(schemaDefinition)` to confirm the declarative migrator accepts the
     rewritten schema without further changes.
4. After the premigration has run in production, **delete the file, the call site, the test, and
   the legacy-schema fixture in the same commit**. There is no version table — the only signal
   that a premigration is no longer needed is that production has booted past it.

See git history for `internal/sqlite/premigrate.go` (workout_exercise stable-id migration,
PR #75) for a worked example.

## Table Design Patterns

### STRICT Mode and Constraints

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

### Required Patterns

- **Always use `STRICT` mode** for type safety
- **Use WITHOUT ROWID** for tables that do not have an integer primary key
- **Always include length constraints** for TEXT fields: `CHECK (LENGTH(field) < N)`
- **Use CHECK constraints for enums**: `CHECK (status IN ('pending', 'active', 'completed'))`
- **Use proper foreign key constraints** with CASCADE behavior where appropriate

### Timestamp Patterns

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

### Foreign Key Patterns

Match the FK column type to the referenced primary key. In this schema `users.id` is `INTEGER` and `credentials.id` is `BLOB` (WebAuthn credential ID), so pick accordingly:

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
