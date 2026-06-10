# exercise_slots Table Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the `workout_exercises` table to `exercise_slots` so the schema speaks the domain's "Exercise slot" vocabulary (CONTEXT.md), preserving production data via a native `ALTER TABLE … RENAME` premigration.

**Architecture:** The declarative migrator (`internal/platform/sqlitekit`) is structural only — it would read a table rename as a drop + create and lose data. So a premigration runs `ALTER TABLE workout_exercises RENAME TO exercise_slots` *before* the migrator; modern SQLite auto-rewrites the child foreign keys (`exercise_sets`, `scheduled_pushes`), and the migrator then reconciles the renamed index for free. The Go domain layer already uses `ExerciseSlot`/`Session.Slots`, so no domain-model changes are needed — only SQL strings, comments, tests, the premigration, its wiring, and a docs update.

**Tech Stack:** Go, SQLite (STRICT, `WITHOUT ROWID`), `database/sql`, the project's declarative migrator in `internal/platform/sqlitekit`.

**Reference spec:** `docs/superpowers/specs/2026-06-10-exercise-slots-rename-design.md`

---

## File Structure

- **Modify** `internal/petra/repository/schema.sql` — rename the table, its index, and the two child FK targets.
- **Modify** (mechanical token swap `workout_exercises` → `exercise_slots`) the repository SQL + comments and all test seed SQL:
  - `internal/petra/repository/shared.go`, `sessions.go`, `repository.go`, `helpers_test.go`, `week_plans_test.go`
  - `internal/petra/service/sessions.go`, `exercises.go`, `helpers_test.go`, `progression_test.go`, `exercises_test.go`, `sessions_test.go`, `sets_test.go`
  - `cmd/petra/handler-exerciseset_test.go`
- **Create** `internal/petra/repository/premigrate_exercise_slots.go` — the `PreMigrateExerciseSlots` premigration.
- **Create** `internal/petra/repository/premigrate_exercise_slots_test.go` — premigration tests (legacy-schema const, rename/preserve/idempotence, full migrate path, fresh-DB no-op).
- **Modify** `cmd/petra/main.go` and `cmd/migratetest/main.go` — wire `Premigration: repository.PreMigrateExerciseSlots`.
- **Modify** `internal/petra/repository/CLAUDE.md` — document native `RENAME` as the premigration path for pure renames.

**Task order rationale:** Task 1 does the blanket `workout_exercises` → `exercise_slots` swap while no file legitimately contains the old name. Tasks 2–3 then *reintroduce* `workout_exercises` only inside the premigration code and its test's legacy-schema const (the old name is correct there). This ordering keeps the verification grep in Task 1 meaningful.

---

### Task 1: Rename the table everywhere except the premigration

**Files:**
- Modify: `internal/petra/repository/schema.sql`
- Modify: every Go file containing the `workout_exercises` token (SQL strings + comments + test seeds), listed above.

- [ ] **Step 1: Confirm the starting reference set**

Run: `grep -rIn 'workout_exercises' --include='*.go' --include='*.sql' .`
Expected: matches in `schema.sql` and the repository/service/cmd files listed above (≈70 lines). Note the count; Step 4 expects it to reach zero.

- [ ] **Step 2: Apply the token swap across Go and SQL files**

This rewrites the table name in DDL, SQL query strings, FK references, the index name, and comments in one pass. No file contains `workout_exercises` as part of a Go identifier (those use `WorkoutExercise` camelCase), so the snake_case token swap is safe.

Run:
```bash
grep -rIl 'workout_exercises' --include='*.go' --include='*.sql' . \
  | xargs sed -i 's/workout_exercises/exercise_slots/g'
```

This turns, among others:
- `CREATE TABLE workout_exercises` → `CREATE TABLE exercise_slots`
- `CREATE INDEX workout_exercises_user_exercise_date_idx ON workout_exercises` → `CREATE INDEX exercise_slots_user_exercise_date_idx ON exercise_slots`
- `REFERENCES workout_exercises (...)` (in `exercise_sets` and `scheduled_pushes`) → `REFERENCES exercise_slots (...)`
- `INSERT INTO workout_exercises (...)` / `FROM workout_exercises we` / `JOIN workout_exercises we` → `exercise_slots`

- [ ] **Step 3: Rename the lone slot test helper for consistency**

The token swap leaves the camelCase helper `seedWorkoutExerciseSlot` untouched. Rename it so the test vocabulary matches the schema.

In `internal/petra/repository/helpers_test.go`, rename the function `seedWorkoutExerciseSlot` → `seedExerciseSlot` (definition near line 57 and its doc comment).
In `internal/petra/repository/scheduled_push_test.go:17`, update the call site `seedWorkoutExerciseSlot(ctx, t, db)` → `seedExerciseSlot(ctx, t, db)`.

Run: `grep -rn 'seedWorkoutExerciseSlot' --include='*.go' .`
Expected: no matches.

- [ ] **Step 4: Verify no stale references remain**

Run: `grep -rIn 'workout_exercises' --include='*.go' --include='*.sql' .`
Expected: **no output** (zero matches). The premigration that deliberately uses the old name is added in Task 2.

- [ ] **Step 5: Run the full test suite**

Run: `make test`
Expected: PASS. Tests build fresh in-memory databases from the renamed `schema.sql` (no legacy table exists, so the migrator creates `exercise_slots` directly) and every seed/query now uses `exercise_slots`.

- [ ] **Step 6: Lint and commit**

```bash
make lint-fix
git add -A
git commit -m "refactor(repository): rename workout_exercises table to exercise_slots

Align the schema with the domain's Exercise slot vocabulary (CONTEXT.md).
Pure token swap across schema.sql, repository SQL strings, comments, and
test seeds; the index and the exercise_sets/scheduled_pushes foreign keys
follow the rename. Domain models already used ExerciseSlot/Slots, so no
Go model changes. Production data migration is wired in a later commit.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Add the `PreMigrateExerciseSlots` premigration (TDD)

**Files:**
- Create: `internal/petra/repository/premigrate_exercise_slots.go`
- Create: `internal/petra/repository/premigrate_exercise_slots_test.go`

- [ ] **Step 1: Write the failing test (rename + data preservation + FK rewrite + idempotence)**

Create `internal/petra/repository/premigrate_exercise_slots_test.go`. The `legacyProductSchemaSQL` const reproduces the product schema **before** Task 1 (i.e. with `workout_exercises`); it is concatenated after `auth.SchemaSQL` exactly as production does, so a `NewDatabase` call builds a database in the pre-rename shape. Keep this const in sync only until the premigration is deleted (see Task 5 lifecycle).

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/myrjola/petrapp/internal/petra/repository"
	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

// legacyProductSchemaSQL is the petra product schema as it stood before the
// workout_exercises -> exercise_slots rename. It lets the premigration tests
// build a database in the legacy shape. Delete it together with
// PreMigrateExerciseSlots once production has booted past the rename.
const legacyProductSchemaSQL = `
CREATE TABLE workout_preferences
(
    user_id                    INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    monday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (monday_minutes IN (0, 45, 60, 90)),
    tuesday_minutes            INTEGER NOT NULL DEFAULT 0 CHECK (tuesday_minutes IN (0, 45, 60, 90)),
    wednesday_minutes          INTEGER NOT NULL DEFAULT 0 CHECK (wednesday_minutes IN (0, 45, 60, 90)),
    thursday_minutes           INTEGER NOT NULL DEFAULT 0 CHECK (thursday_minutes IN (0, 45, 60, 90)),
    friday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (friday_minutes IN (0, 45, 60, 90)),
    saturday_minutes           INTEGER NOT NULL DEFAULT 0 CHECK (saturday_minutes IN (0, 45, 60, 90)),
    sunday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (sunday_minutes IN (0, 45, 60, 90)),
    rest_notifications_enabled INTEGER NOT NULL DEFAULT 1 CHECK (rest_notifications_enabled IN (0, 1)),
    deload_enabled             INTEGER NOT NULL DEFAULT 0 CHECK (deload_enabled IN (0, 1)),
    mesocycle_length           INTEGER NOT NULL DEFAULT 5 CHECK (mesocycle_length BETWEEN 4 AND 7),
    mesocycle_anchor           TEXT CHECK (mesocycle_anchor IS NULL
                                           OR STRFTIME('%Y-%m-%d', mesocycle_anchor) = mesocycle_anchor)
) STRICT;

CREATE TABLE exercises
(
    id                       INTEGER PRIMARY KEY,
    name                     TEXT    NOT NULL UNIQUE CHECK (LENGTH(name) < 124),
    category                 TEXT    NOT NULL CHECK (category IN ('full_body', 'upper', 'lower')),
    exercise_type            TEXT    NOT NULL DEFAULT 'weighted'
                             CHECK (exercise_type IN ('weighted', 'bodyweight', 'assisted', 'time_based')),
    description_markdown     TEXT    NOT NULL DEFAULT '' CHECK (LENGTH(description_markdown) < 20000),
    default_starting_seconds INTEGER CHECK (default_starting_seconds IS NULL OR default_starting_seconds > 0),
    rep_min                  INTEGER CHECK (rep_min IS NULL OR (rep_min >= 1 AND rep_min <= 50)),
    rep_max                  INTEGER CHECK (rep_max IS NULL OR (rep_max >= 1 AND rep_max <= 50)),
    CHECK (exercise_type <> 'time_based' OR default_starting_seconds IS NOT NULL),
    CHECK (exercise_type =  'time_based' OR (rep_min IS NOT NULL AND rep_max IS NOT NULL)),
    CHECK (rep_min IS NULL OR rep_max IS NULL OR rep_min <= rep_max)
) STRICT;

CREATE TABLE workout_sessions
(
    user_id            INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workout_date       TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    difficulty_rating  INTEGER CHECK (difficulty_rating BETWEEN 1 AND 5),
    started_at         TEXT CHECK (started_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', started_at) = started_at),
    completed_at       TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
    periodization_type TEXT    NOT NULL DEFAULT 'strength'
        CHECK (periodization_type IN ('strength', 'hypertrophy')),
    is_deload          INTEGER NOT NULL DEFAULT 0 CHECK (is_deload IN (0, 1)),

    PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;

CREATE TABLE workout_exercises
(
    workout_user_id     INTEGER NOT NULL,
    workout_date        TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    position            INTEGER NOT NULL CHECK (position >= 0),
    exercise_id         INTEGER NOT NULL,
    warmup_completed_at TEXT CHECK (warmup_completed_at IS NULL OR
                                    STRFTIME('%Y-%m-%dT%H:%M:%fZ', warmup_completed_at) = warmup_completed_at),

    PRIMARY KEY (workout_user_id, workout_date, position),
    UNIQUE (workout_user_id, workout_date, exercise_id),
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) WITHOUT ROWID, STRICT;

CREATE INDEX workout_exercises_user_exercise_date_idx
    ON workout_exercises (workout_user_id, exercise_id, workout_date);

CREATE TABLE exercise_sets
(
    workout_user_id INTEGER NOT NULL,
    workout_date    TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    position        INTEGER NOT NULL,
    set_number      INTEGER NOT NULL CHECK (set_number > 0),
    weight_kg       REAL,
    target_value    INTEGER NOT NULL CHECK (target_value > 0),
    completed_value INTEGER CHECK (completed_value IS NULL OR completed_value >= 0),
    completed_at    TEXT CHECK (completed_at IS NULL OR
                                STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
    signal          TEXT CHECK (signal IS NULL OR signal IN ('too_heavy', 'on_target', 'too_light')),

    PRIMARY KEY (workout_user_id, workout_date, position, set_number),
    FOREIGN KEY (workout_user_id, workout_date, position)
        REFERENCES workout_exercises (workout_user_id, workout_date, position) ON DELETE CASCADE
) WITHOUT ROWID, STRICT;

CREATE TABLE muscle_groups
(
    name TEXT NOT NULL PRIMARY KEY CHECK (LENGTH(name) < 64)
) WITHOUT ROWID, STRICT;

CREATE TABLE exercise_muscle_groups
(
    exercise_id       INTEGER NOT NULL REFERENCES exercises (id) ON DELETE CASCADE,
    muscle_group_name TEXT    NOT NULL REFERENCES muscle_groups (name) ON DELETE CASCADE,
    is_primary        INTEGER NOT NULL DEFAULT 0 CHECK (is_primary IN (0, 1)),

    PRIMARY KEY (exercise_id, muscle_group_name)
) WITHOUT ROWID, STRICT;

CREATE TABLE muscle_group_weekly_targets
(
    muscle_group_name   TEXT    PRIMARY KEY REFERENCES muscle_groups (name) ON DELETE CASCADE,
    min_sets            INTEGER NOT NULL CHECK (min_sets > 0),
    max_sets            INTEGER NOT NULL CHECK (max_sets >= min_sets)
) WITHOUT ROWID, STRICT;

CREATE TABLE feature_flags
(
    name    TEXT PRIMARY KEY CHECK (LENGTH(name) < 256),
    enabled INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1))
) WITHOUT ROWID, STRICT;

CREATE TABLE push_subscriptions
(
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint   TEXT    NOT NULL UNIQUE CHECK (LENGTH(endpoint) < 1024),
    p256dh     TEXT    NOT NULL CHECK (LENGTH(p256dh) < 256),
    auth       TEXT    NOT NULL CHECK (LENGTH(auth) < 256),
    created_at TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at)
) STRICT;

CREATE INDEX push_subscriptions_user_id ON push_subscriptions (user_id);

CREATE TABLE scheduled_pushes
(
    id              INTEGER PRIMARY KEY,
    workout_user_id INTEGER NOT NULL,
    workout_date    TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    position        INTEGER NOT NULL,
    fire_at         TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', fire_at) = fire_at),
    payload         TEXT    NOT NULL CHECK (LENGTH(payload) < 2048),
    created_at      TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at),

    FOREIGN KEY (workout_user_id, workout_date, position)
        REFERENCES workout_exercises (workout_user_id, workout_date, position) ON DELETE CASCADE
) STRICT;

CREATE UNIQUE INDEX scheduled_pushes_slot_uidx
    ON scheduled_pushes (workout_user_id, workout_date, position);
CREATE INDEX scheduled_pushes_fire_at ON scheduled_pushes (fire_at);
`

// newLegacyDB builds an in-memory database in the pre-rename shape (with the
// workout_exercises table) and returns it. cache=shared (used for :memory:)
// means the premigration's writes are visible to all pooled connections.
func newLegacyDB(t *testing.T) (context.Context, *sqlitekit.Database) {
	t.Helper()
	ctx := t.Context()
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + legacyProductSchemaSQL,
		Fixtures:     "",
		Logger:       testkit.NewLogger(testkit.NewWriter(t)),
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create legacy database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return ctx, db
}

// seedLegacySlot inserts a user, session, exercise, a workout_exercises slot,
// one exercise_sets row, and one scheduled_pushes row, all wired by foreign
// key, so the premigration can be observed to preserve child relationships.
func seedLegacySlot(ctx context.Context, t *testing.T, db *sqlitekit.Database) {
	t.Helper()
	stmts := []string{
		`INSERT INTO users (webauthn_user_id, display_name) VALUES (X'01', 'Test User')`,
		`INSERT INTO exercises (id, name, category, rep_min, rep_max) VALUES (1, 'Squat', 'lower', 5, 10)`,
		`INSERT INTO workout_sessions (user_id, workout_date) VALUES (1, '2026-06-10')`,
		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
		 VALUES (1, '2026-06-10', 0, 1)`,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, target_value)
		 VALUES (1, '2026-06-10', 0, 1, 8)`,
		`INSERT INTO scheduled_pushes (workout_user_id, workout_date, position, fire_at, payload)
		 VALUES (1, '2026-06-10', 0, '2026-06-10T10:00:00.000Z', '{}')`,
	}
	for _, stmt := range stmts {
		if _, err := db.ReadWrite.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed legacy slot (%q): %v", stmt, err)
		}
	}
}

func TestPreMigrateExerciseSlots_renamesAndPreservesData(t *testing.T) {
	t.Parallel()
	ctx, db := newLegacyDB(t)
	seedLegacySlot(ctx, t, db)

	if err := repository.PreMigrateExerciseSlots(ctx, db); err != nil {
		t.Fatalf("premigration: %v", err)
	}

	// The old table is gone and the new one carries the row.
	var legacyCount int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'workout_exercises'`,
	).Scan(&legacyCount); err != nil {
		t.Fatalf("count legacy table: %v", err)
	}
	if legacyCount != 0 {
		t.Errorf("workout_exercises still present after rename")
	}
	var slotCount int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM exercise_slots`,
	).Scan(&slotCount); err != nil {
		t.Fatalf("count exercise_slots: %v", err)
	}
	if slotCount != 1 {
		t.Errorf("exercise_slots row count = %d, want 1", slotCount)
	}

	// The child foreign key was auto-rewritten to point at exercise_slots.
	var ref string
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT "table" FROM pragma_foreign_key_list('exercise_sets')`,
	).Scan(&ref); err != nil {
		t.Fatalf("read exercise_sets foreign key: %v", err)
	}
	if ref != "exercise_slots" {
		t.Errorf("exercise_sets foreign key references %q, want exercise_slots", ref)
	}

	// Idempotent: a second run is a no-op (the guard short-circuits).
	if err := repository.PreMigrateExerciseSlots(ctx, db); err != nil {
		t.Fatalf("second premigration run: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/petra/repository/ -run TestPreMigrateExerciseSlots_renamesAndPreservesData -v`
Expected: FAIL — `undefined: repository.PreMigrateExerciseSlots` (the function does not exist yet).

- [ ] **Step 3: Implement the premigration**

Create `internal/petra/repository/premigrate_exercise_slots.go`:

```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
)

// PreMigrateExerciseSlots renames the legacy workout_exercises table to
// exercise_slots before the declarative migrator runs. The migrator is
// structural only and would read the rename as a drop + create, losing data,
// so this native ALTER TABLE RENAME runs first. Modern SQLite (legacy_alter_table
// off, the default) auto-rewrites the foreign keys in the child tables
// (exercise_sets, scheduled_pushes) to the new name, and the migrator then
// reconciles the renamed index. RENAME does not revalidate foreign keys, so no
// PRAGMA foreign_keys = OFF is needed.
//
// It is idempotent and safe on a fresh database: if workout_exercises is
// absent (already renamed, or a brand-new database) it returns nil.
//
// Delete this function, its wiring in cmd/petra and cmd/migratetest, and its
// test once production has booted past the rename.
func PreMigrateExerciseSlots(ctx context.Context, db *sqlitekit.Database) error {
	// A transaction pins a single pooled connection for the check and the
	// rename, and rolls back automatically on any early return or error.
	tx, err := db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var name string
	err = tx.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'workout_exercises'`,
	).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // Already renamed, or a fresh database — nothing to do.
	}
	if err != nil {
		return fmt.Errorf("check for legacy workout_exercises table: %w", err)
	}

	if _, err = tx.ExecContext(ctx,
		`ALTER TABLE workout_exercises RENAME TO exercise_slots`,
	); err != nil {
		return fmt.Errorf("rename workout_exercises to exercise_slots: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit exercise_slots rename: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/petra/repository/ -run TestPreMigrateExerciseSlots_renamesAndPreservesData -v`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
make lint-fix
git add internal/petra/repository/premigrate_exercise_slots.go internal/petra/repository/premigrate_exercise_slots_test.go
git commit -m "feat(repository): add PreMigrateExerciseSlots rename premigration

Native ALTER TABLE RENAME of workout_exercises -> exercise_slots, run
before the structural migrator so production data survives. Idempotent and
fresh-database safe via a sqlite_master guard. Temporary; delete once prod
has booted past the rename.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Test the full migrate path and the fresh-database no-op

**Files:**
- Modify: `internal/petra/repository/premigrate_exercise_slots_test.go`

- [ ] **Step 1: Write the failing full-path test**

Append to `internal/petra/repository/premigrate_exercise_slots_test.go`. This uses a real file database so it can be closed and reopened: first built in the legacy shape and seeded, then reopened with the **real** schema (`repository.SchemaSQL`, which now has `exercise_slots`) and the premigration wired — exactly the production boot path. Add `"path/filepath"` to the import block.

```go
func TestPreMigrateExerciseSlots_fullMigratePathPreservesData(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dbPath := filepath.Join(t.TempDir(), "petra.db")
	logger := testkit.NewLogger(testkit.NewWriter(t))

	// Build the database in the legacy shape and seed a slot with children.
	legacy, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          dbPath,
		Schema:       auth.SchemaSQL + "\n" + legacyProductSchemaSQL,
		Fixtures:     "",
		Logger:       logger,
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create legacy file database: %v", err)
	}
	seedLegacySlot(ctx, t, legacy)
	if err = legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	// Reopen with the real schema + premigration: the production boot path.
	migrated, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          dbPath,
		Schema:       auth.SchemaSQL + "\n" + repository.SchemaSQL,
		Fixtures:     repository.FixturesSQL,
		Logger:       logger,
		Premigration: repository.PreMigrateExerciseSlots,
	})
	if err != nil {
		t.Fatalf("reopen with real schema + premigration: %v", err)
	}
	t.Cleanup(func() { _ = migrated.Close() })

	// The seeded slot and its child set survived the rename + migrate.
	var joined int
	if err = migrated.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM exercise_slots s
		   JOIN exercise_sets es
		     ON es.workout_user_id = s.workout_user_id
		    AND es.workout_date = s.workout_date
		    AND es.position = s.position`,
	).Scan(&joined); err != nil {
		t.Fatalf("join exercise_slots/exercise_sets: %v", err)
	}
	if joined != 1 {
		t.Errorf("joined slot+set rows = %d, want 1", joined)
	}

	// The index was reconciled to the new name by the declarative migrator.
	var idxCount int
	if err = migrated.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master
		  WHERE type = 'index' AND name = 'exercise_slots_user_exercise_date_idx'`,
	).Scan(&idxCount); err != nil {
		t.Fatalf("count renamed index: %v", err)
	}
	if idxCount != 1 {
		t.Errorf("renamed index present = %d, want 1", idxCount)
	}
}

func TestPreMigrateExerciseSlots_freshDatabaseIsNoOp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	// A database already on the real schema has exercise_slots and no
	// workout_exercises; the premigration must be a clean no-op.
	db, err := sqlitekit.NewDatabase(ctx, sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL + "\n" + repository.SchemaSQL,
		Fixtures:     repository.FixturesSQL,
		Logger:       testkit.NewLogger(testkit.NewWriter(t)),
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("create fresh database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err = repository.PreMigrateExerciseSlots(ctx, db); err != nil {
		t.Fatalf("premigration on fresh database: %v", err)
	}

	var slotTableCount int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'exercise_slots'`,
	).Scan(&slotTableCount); err != nil {
		t.Fatalf("count exercise_slots table: %v", err)
	}
	if slotTableCount != 1 {
		t.Errorf("exercise_slots table present = %d, want 1", slotTableCount)
	}
}
```

- [ ] **Step 2: Run the new tests to verify they pass**

Run: `go test ./internal/petra/repository/ -run TestPreMigrateExerciseSlots -v`
Expected: PASS for all three `TestPreMigrateExerciseSlots_*` tests. (They were written against the function from Task 2; the full-path test confirms the premigration + declarative migrator compose correctly and preserve data.)

- [ ] **Step 3: Lint and commit**

```bash
make lint-fix
git add internal/petra/repository/premigrate_exercise_slots_test.go
git commit -m "test(repository): cover full premigrate+migrate path and fresh-db no-op

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Wire the premigration into the boot paths

**Files:**
- Modify: `cmd/petra/main.go` (the `openDatabase` `sqlitekit.Config`, near line 220)
- Modify: `cmd/migratetest/main.go` (the `sqlitekit.Config`, near line 44)

- [ ] **Step 1: Wire `cmd/petra/main.go`**

In `openDatabase`, change the config field:
```go
		Premigration: nil,
```
to:
```go
		Premigration: repository.PreMigrateExerciseSlots,
```
(The `repository` package is already imported in this file.)

- [ ] **Step 2: Wire `cmd/migratetest/main.go`**

Make the identical change in that file's `sqlitekit.Config` literal so `make migratetest` exercises the premigration against a production snapshot:
```go
		Premigration: repository.PreMigrateExerciseSlots,
```

- [ ] **Step 3: Verify the build and that wiring is in place**

Run: `go build ./cmd/...`
Expected: builds with no errors.

Run: `grep -rn 'Premigration:' cmd/`
Expected: `cmd/petra/main.go` and `cmd/migratetest/main.go` show `repository.PreMigrateExerciseSlots`; `cmd/example/main.go` still shows `nil`.

- [ ] **Step 4: Commit**

```bash
git add cmd/petra/main.go cmd/migratetest/main.go
git commit -m "feat(petra): wire PreMigrateExerciseSlots into boot and migratetest

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Document native RENAME as the premigration path for pure renames

**Files:**
- Modify: `internal/petra/repository/CLAUDE.md` (the "Premigration escape hatch" section)

- [ ] **Step 1: Add the pure-rename subsection**

In `internal/petra/repository/CLAUDE.md`, immediately after the "Premigration escape hatch" `A premigration function must:` list (the block describing the `CREATE *_new → INSERT … SELECT → DROP → RENAME` pattern), insert:

```markdown
#### Pure renames: prefer native `ALTER TABLE … RENAME`

For a *pure rename* — a table (`ALTER TABLE old RENAME TO new`) or a column
(`ALTER TABLE t RENAME COLUMN a TO b`) with no data reshaping — skip the
`CREATE *_new → INSERT … SELECT → DROP → RENAME` dance. A single `RENAME` is
enough: modern SQLite (with `legacy_alter_table` off, the default) automatically
rewrites the foreign-key references in child tables to the new name, and the
declarative migrator then reconciles any renamed index for free (indexes carry
no data). `RENAME` does not revalidate foreign keys, so the
`PRAGMA foreign_keys = OFF` step is unnecessary too.

Two things still apply: wrap the check and the `RENAME` in a single
transaction (`db.ReadWrite.BeginTx`) so they share one pooled connection and
roll back together, and keep the idempotency guard — query `sqlite_master` for
the old name and return early when it is gone (this also short-circuits a fresh
database). See `PreMigrateExerciseSlots` for a worked example. Reserve the copy
pattern above for genuine data *reshaping* (re-keying, column splits, merges).
```

- [ ] **Step 2: Verify the edit reads correctly**

Run: `grep -n 'Pure renames' internal/petra/repository/CLAUDE.md`
Expected: one match.

- [ ] **Step 3: Commit**

```bash
git add internal/petra/repository/CLAUDE.md
git commit -m "docs(repository): document native RENAME premigration for pure renames

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Full validation and ship

**Files:** none (validation + integration)

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: init + build + lint-fix + test + sec all pass. This is the worktreeflow Phase 3 gate.

- [ ] **Step 2: Ship via worktreeflow Phase 3**

Follow the worktreeflow skill's Phase 3: push the branch to `origin/main`, then tear down the worktree and re-sync the outer checkout. Do not push until `make ci` is green.

---

## Notes for the implementer

- **Why no domain changes:** the Go domain already uses `domain.ExerciseSlot` and `Session.Slots`; only the SQL persistence layer carried the old `workout_exercises` name. The rename brings the schema in line with the existing code and the CONTEXT.md glossary.
- **The old name lives in exactly two places after Task 2:** `premigrate_exercise_slots.go` (the `ALTER TABLE` statement and its `sqlite_master` guard) and `premigrate_exercise_slots_test.go` (`legacyProductSchemaSQL` and seed SQL). Both are correct and intentional — they describe the pre-rename world the premigration upgrades from.
- **Lifecycle:** this premigration is temporary. Once production has booted past it, delete `PreMigrateExerciseSlots`, both `main.go` wirings, `premigrate_exercise_slots_test.go` (including `legacyProductSchemaSQL`), in one commit. The `CLAUDE.md` guidance from Task 5 is permanent.
```
