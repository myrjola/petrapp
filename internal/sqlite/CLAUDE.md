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
5. **Test with `make test` to ensure migrations and queries work correctly
6. **Add test fixtures** in `fixtures.sql` if needed for new data that needs to be seeded

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

```sql
-- Standard foreign key with cascade
user_id BLOB NOT NULL REFERENCES users (id) ON DELETE CASCADE

-- Deferred foreign key (for complex relationships)
FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED

-- Composite foreign key
FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE
```
