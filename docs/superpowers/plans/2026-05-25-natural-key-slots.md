# Natural-key slots Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `workout_exercise.id` (surrogate PK preserved across delete-reinsert) with composite natural key `(workout_user_id, workout_date, position)` so the WeekPlan three-pass slot insert collapses to a single pass.

**Architecture:** One premigration rewrites legacy `workout_exercise` (singular, surrogate-id) into `workout_exercises` (plural, composite-PK) and re-keys `exercise_sets` and `scheduled_pushes` onto the composite. Domain `ExerciseSet.ID` is removed; slot identity is array index. HTTP route param `{workoutExerciseID}` renames to `{position}`. Spec: `docs/superpowers/specs/2026-05-25-natural-key-slots-design.md`.

**Tech Stack:** Go 1.x, SQLite (modernc/mattn-go-sqlite3 with read/write split), declarative schema migrator (`internal/sqlite/migrate.go`), premigration escape hatch (`internal/sqlite/premigrate.go`), `go test --race --shuffle=on`, golangci-lint.

**Commit cadence note:** Task 1 lands as a self-contained green commit (premigration + its test, no schema or production code changes). Tasks 2–11 land as the second commit — they are tightly coupled (schema, repository SQL, domain types, service signatures, handlers all reference each other). Sub-tasks describe the recommended authoring order within that big commit; do not commit at the end of each sub-task in isolation, because intermediate states will not compile. Task 12 is the final green-build verification + commit step. Task 13 is the CLAUDE.md doc commit. Task 14 covers post-deploy cleanup (separate PR, after production has booted past the premigration).

---

## File Structure

**New files:**
- `internal/sqlite/premigrate.go` — `preMigrateWorkoutPositions` method on `*Database`.

**Existing files modified:**
- `internal/sqlite/schema.sql` — rename `workout_exercise` → `workout_exercises`, composite PK, drop `scheduled_pushes.user_id`.
- `internal/sqlite/sqlite.go` — wire premigration call between `connect` and `migrateTo`.
- `internal/sqlite/migrate_internal_test.go` — new test case with `legacyWorkoutExerciseSchema` const.
- `internal/domain/session.go` — drop `ExerciseSet.ID`, rename `slotID` → `pos` on all slot methods, delete `Session.Slot` and `Session.findSlot`.
- `internal/domain/week_plan.go` — rename `slotID` → `pos` on dispatchers.
- `internal/domain/scheduled_push.go` — drop `WorkoutExerciseID`, add `WorkoutDate` + `Position`.
- `internal/domain/planner.go` — drop `ID:` from `ExerciseSet` struct literal.
- `internal/domain/rest_push.go` — slot input now keyed by position (caller passes it).
- `internal/domain/session_test.go`, `week_plan_test.go`, `rest_push_test.go` — rename tests, update struct literals.
- `internal/repository/shared.go` — `saveOneSlotInTx` takes `pos int`, drop `idArg`/`RETURNING id`; `saveExerciseSetsInTx` collapses to single pass.
- `internal/repository/week_plans.go` — `reinsertWeekInTx` single pass; delete `reinsertSlotsForWeek`; update read SQL.
- `internal/repository/sessions.go` — read SQL projects `position`.
- `internal/repository/scheduled_push.go` — rename methods, composite-key SQL.
- `internal/repository/week_plans_test.go`, `sessions_test.go`, `scheduled_push_test.go` — update fixtures and assertions.
- `internal/service/exercises.go`, `sets.go`, `sessions.go`, `service.go`, `push.go` — rename `workoutExerciseID` → `pos`; update `Scheduler` interface.
- `internal/service/exercises_test.go`, `sets_test.go`, `sessions_test.go`, `push_test.go` — update call sites.
- `cmd/web/routes.go` — rename `{workoutExerciseID}` → `{position}`.
- `cmd/web/handler-exerciseset.go`, `handler-workout.go`, `handler-exercise-info.go` — rename param parser, inline `findExerciseSetInSession`, drop URL-stable comment.
- `cmd/web/handler-exerciseset_test.go`, `handler-workout_test.go`, `handler-exercise-info_test.go` — flip raw-SQL fixtures to insert with explicit position.
- `internal/sqlite/CLAUDE.md`, `internal/repository/CLAUDE.md`, `internal/domain/CLAUDE.md` — retire three-pass / surrogate-id language.

---

## Task 1: Premigration scaffolding + idempotence test

**Files:**
- Create: `internal/sqlite/premigrate.go`
- Modify: `internal/sqlite/migrate_internal_test.go`
- Modify: `internal/sqlite/sqlite.go:48-50` (wire premigration between `connect` and `migrateTo`)

**Goal of this task:** Land a green commit that contains the premigration logic and its test, idempotent against fresh and migrated DBs, without yet changing `schema.sql`. The test defines both legacy and target schemas inline so it does not depend on `schema.sql`.

- [ ] **Step 1: Create `internal/sqlite/premigrate.go` with detection and rewrite skeleton**

```go
package sqlite

import (
	"context"
	"fmt"
)

// preMigrateWorkoutPositions rewrites the legacy workout_exercise table
// (surrogate id PK) into workout_exercises (composite PK keyed by
// workout_user_id, workout_date, position), re-keying exercise_sets and
// scheduled_pushes onto the composite. Idempotent: returns nil immediately on
// fresh or already-migrated databases. Runs before migrateTo so the
// declarative migrator sees a database that matches schema.sql.
func (d *Database) preMigrateWorkoutPositions(ctx context.Context) error {
	// Detection: if the legacy singular table is gone, there is nothing to do.
	// This covers fresh DBs (no workout_exercise yet — migrateTo will create
	// workout_exercises directly from schema.sql) and already-migrated DBs
	// (the legacy table was dropped in a previous run).
	var present int
	if err := d.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('workout_exercise')`,
	).Scan(&present); err != nil {
		return fmt.Errorf("detect legacy workout_exercise: %w", err)
	}
	if present == 0 {
		return nil
	}

	if _, err := d.ReadWrite.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	tx, err := d.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin premigration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmts := []string{
		`CREATE TABLE workout_exercises (
			workout_user_id     INTEGER NOT NULL,
			workout_date        TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
			position            INTEGER NOT NULL CHECK (position >= 0),
			exercise_id         INTEGER NOT NULL,
			warmup_completed_at TEXT CHECK (warmup_completed_at IS NULL OR
				STRFTIME('%Y-%m-%dT%H:%M:%fZ', warmup_completed_at) = warmup_completed_at),
			PRIMARY KEY (workout_user_id, workout_date, position),
			UNIQUE (workout_user_id, workout_date, exercise_id),
			FOREIGN KEY (workout_user_id, workout_date)
				REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
			FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
		) WITHOUT ROWID, STRICT`,

		`INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id, warmup_completed_at)
		 SELECT workout_user_id, workout_date,
		        ROW_NUMBER() OVER (PARTITION BY workout_user_id, workout_date ORDER BY id) - 1,
		        exercise_id, warmup_completed_at
		 FROM workout_exercise`,

		`CREATE TABLE exercise_sets_new (
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
		) WITHOUT ROWID, STRICT`,

		`INSERT INTO exercise_sets_new (
			workout_user_id, workout_date, position, set_number,
			weight_kg, target_value, completed_value, completed_at, signal
		 )
		 SELECT new_we.workout_user_id, new_we.workout_date, new_we.position,
		        es.set_number, es.weight_kg, es.target_value, es.completed_value, es.completed_at, es.signal
		 FROM exercise_sets es
		 JOIN workout_exercise   old_we ON old_we.id = es.workout_exercise_id
		 JOIN workout_exercises  new_we
		      ON new_we.workout_user_id = old_we.workout_user_id
		     AND new_we.workout_date    = old_we.workout_date
		     AND new_we.exercise_id     = old_we.exercise_id`,

		`CREATE TABLE scheduled_pushes_new (
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
		) STRICT`,

		`INSERT INTO scheduled_pushes_new (
			id, workout_user_id, workout_date, position, fire_at, payload, created_at
		 )
		 SELECT sp.id,
		        new_we.workout_user_id, new_we.workout_date, new_we.position,
		        sp.fire_at, sp.payload, sp.created_at
		 FROM scheduled_pushes sp
		 JOIN workout_exercise  old_we ON old_we.id = sp.workout_exercise_id
		 JOIN workout_exercises new_we
		      ON new_we.workout_user_id = old_we.workout_user_id
		     AND new_we.workout_date    = old_we.workout_date
		     AND new_we.exercise_id     = old_we.exercise_id`,

		`DROP TABLE scheduled_pushes`,
		`DROP TABLE exercise_sets`,
		`DROP TABLE workout_exercise`,
		`ALTER TABLE exercise_sets_new    RENAME TO exercise_sets`,
		`ALTER TABLE scheduled_pushes_new RENAME TO scheduled_pushes`,

		`CREATE INDEX workout_exercises_user_exercise_date_idx
		    ON workout_exercises (workout_user_id, exercise_id, workout_date)`,
		`CREATE UNIQUE INDEX scheduled_pushes_slot_uidx
		    ON scheduled_pushes (workout_user_id, workout_date, position)`,
		`CREATE INDEX scheduled_pushes_fire_at ON scheduled_pushes (fire_at)`,
	}
	for i, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("premigration stmt %d: %w", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit premigration: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Wire the premigration in `sqlite.go`**

Replace the lines after `connect` in `NewDatabase` (currently `if err = db.migrateTo(...)`):

```go
if db, err = connect(ctx, url, logger); err != nil {
    return nil, fmt.Errorf("connect: %w", err)
}

if err = db.preMigrateWorkoutPositions(ctx); err != nil {
    return nil, fmt.Errorf("preMigrateWorkoutPositions: %w", err)
}

if err = db.migrateTo(ctx, schemaDefinition); err != nil {
    return nil, fmt.Errorf("migrateTo: %w", err)
}
```

- [ ] **Step 3: Write the premigration test case in `migrate_internal_test.go`**

Add this test function to the file (alongside existing `TestDatabase_migrate`):

```go
func TestDatabase_preMigrateWorkoutPositions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	db, err := NewDatabase(ctx, ":memory:", noopLogger())
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Drop the live schema's workout-related tables; we re-seed the legacy shape
	// to exercise the premigration end-to-end.
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS scheduled_pushes`,
		`DROP TABLE IF EXISTS exercise_sets`,
		`DROP TABLE IF EXISTS workout_exercises`,
		`DROP TABLE IF EXISTS workout_exercise`,
		`DROP TABLE IF EXISTS workout_sessions`,
		`DROP TABLE IF EXISTS users`,
	} {
		if _, execErr := db.ReadWrite.ExecContext(ctx, stmt); execErr != nil {
			t.Fatalf("teardown %q: %v", stmt, execErr)
		}
	}

	legacy := `
	CREATE TABLE users (id INTEGER PRIMARY KEY) STRICT;
	CREATE TABLE exercises (id INTEGER PRIMARY KEY, name TEXT NOT NULL) STRICT;
	CREATE TABLE workout_sessions (
		user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
		workout_date TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
		PRIMARY KEY (user_id, workout_date)
	) WITHOUT ROWID, STRICT;
	CREATE TABLE workout_exercise (
		id                  INTEGER PRIMARY KEY,
		workout_user_id     INTEGER NOT NULL,
		workout_date        TEXT    NOT NULL,
		exercise_id         INTEGER NOT NULL,
		warmup_completed_at TEXT,
		UNIQUE (workout_user_id, workout_date, exercise_id),
		FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
		FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
	) STRICT;
	CREATE TABLE exercise_sets (
		workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
		set_number          INTEGER NOT NULL CHECK (set_number > 0),
		weight_kg           REAL,
		target_value        INTEGER NOT NULL CHECK (target_value > 0),
		completed_value     INTEGER,
		completed_at        TEXT,
		signal              TEXT,
		PRIMARY KEY (workout_exercise_id, set_number)
	) WITHOUT ROWID, STRICT;
	CREATE TABLE scheduled_pushes (
		id                  INTEGER PRIMARY KEY,
		user_id             INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
		workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
		fire_at             TEXT    NOT NULL,
		payload             TEXT    NOT NULL,
		created_at          TEXT    NOT NULL
	) STRICT;
	`
	if _, err = db.ReadWrite.ExecContext(ctx, legacy); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	// Realistic seed: one user, two sessions on different dates. The Monday
	// session has three slots whose surrogate IDs are non-sequential (5, 12,
	// 47) — simulates a history of swaps that left gaps. Tuesday has a single
	// slot. Wednesday has a session row with zero slots (rest-day analog).
	// One slot carries a scheduled push.
	seed := []string{
		`INSERT INTO users (id) VALUES (1)`,
		`INSERT INTO exercises (id, name) VALUES (10, 'bench'), (20, 'row'), (30, 'squat'), (40, 'plank')`,
		`INSERT INTO workout_sessions (user_id, workout_date) VALUES
			(1, '2026-05-04'),
			(1, '2026-05-05'),
			(1, '2026-05-06')`,
		`INSERT INTO workout_exercise (id, workout_user_id, workout_date, exercise_id) VALUES
			(5,  1, '2026-05-04', 10),
			(12, 1, '2026-05-04', 20),
			(47, 1, '2026-05-04', 30),
			(50, 1, '2026-05-05', 40)`,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, target_value, completed_value, completed_at) VALUES
			(5,  1, 5, NULL, NULL),
			(5,  2, 5, 5,    '2026-05-04T09:00:00.000Z'),
			(12, 1, 8, NULL, NULL),
			(47, 1, 6, NULL, NULL),
			(50, 1, 30, NULL, NULL)`,
		`INSERT INTO scheduled_pushes (id, user_id, workout_exercise_id, fire_at, payload, created_at) VALUES
			(1, 1, 12, '2026-05-04T09:30:00.000Z', '{}', '2026-05-04T09:00:00.000Z')`,
	}
	for _, stmt := range seed {
		if _, err = db.ReadWrite.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed: %q: %v", stmt, err)
		}
	}

	if err = db.preMigrateWorkoutPositions(ctx); err != nil {
		t.Fatalf("preMigrateWorkoutPositions: %v", err)
	}

	// Assert positions: dense 0..N-1 per (user, date), ordered by original id.
	type row struct {
		date     string
		position int
		exercise int
	}
	rows, err := db.ReadOnly.QueryContext(ctx,
		`SELECT workout_date, position, exercise_id FROM workout_exercises
		 WHERE workout_user_id = 1 ORDER BY workout_date, position`)
	if err != nil {
		t.Fatalf("query workout_exercises: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var got []row
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.date, &r.position, &r.exercise); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	want := []row{
		{"2026-05-04", 0, 10},
		{"2026-05-04", 1, 20},
		{"2026-05-04", 2, 30},
		{"2026-05-05", 0, 40},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workout_exercises rows = %#v, want %#v", got, want)
	}

	// Assert exercise_sets re-key: every row resolves to its new (user, date, position).
	var setsCount int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM exercise_sets`).Scan(&setsCount); err != nil {
		t.Fatalf("count exercise_sets: %v", err)
	}
	if setsCount != 5 {
		t.Fatalf("exercise_sets count = %d, want 5", setsCount)
	}

	// Assert scheduled_pushes re-key: the row pointed at old id=12 (Monday slot 1
	// after re-keying) lives at position=1 with no user_id column.
	var pushPos int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT position FROM scheduled_pushes WHERE id = 1`).Scan(&pushPos); err != nil {
		t.Fatalf("scheduled_pushes lookup: %v", err)
	}
	if pushPos != 1 {
		t.Fatalf("scheduled_pushes.position = %d, want 1", pushPos)
	}

	// Idempotence: re-running is a no-op.
	if err = db.preMigrateWorkoutPositions(ctx); err != nil {
		t.Fatalf("idempotent re-run: %v", err)
	}

	// migrateTo against the live schema is a no-op (asserts shape matches).
	if err = db.migrateTo(ctx, schemaDefinition); err != nil {
		t.Fatalf("migrateTo: %v", err)
	}
}
```

Note: this test depends on `schemaDefinition` (the live schema.sql) matching the post-premigration shape. Since Task 1 lands before Task 2 (schema update), the *very last* assertion (`migrateTo` is a no-op) will fail in Task 1's commit. Comment it out for now, document why, and uncomment in Task 12. Use this placeholder:

```go
// The migrateTo no-op assertion is enabled in Task 12, once schema.sql is updated
// to the post-premigration shape. Until then, the declarative migrator sees a
// shape mismatch (workout_exercises exists but schema.sql still has workout_exercise).
// _ = schemaDefinition
```

- [ ] **Step 4: Run the test, confirm it passes (premigration logic correct, schema check disabled)**

Run: `go test -v -run TestDatabase_preMigrateWorkoutPositions ./internal/sqlite`
Expected: PASS

- [ ] **Step 5: Run the full test suite to confirm no regressions**

Run: `make test`
Expected: PASS (existing tests use `:memory:` DBs that go through the new `NewDatabase` path — the premigration early-returns because `workout_exercise` does not exist on a fresh DB).

- [ ] **Step 6: Commit**

```bash
git add internal/sqlite/premigrate.go internal/sqlite/sqlite.go internal/sqlite/migrate_internal_test.go
git commit -m "$(cat <<'EOF'
sqlite: add preMigrateWorkoutPositions for natural-key slot rewrite

Lands the one-shot premigration that rewrites legacy workout_exercise
(singular, surrogate-id PK) into workout_exercises (plural, composite
PK keyed by user/date/position). Idempotent: early-returns on fresh and
already-migrated databases. The follow-up commit updates schema.sql to
the post-premigration shape and re-enables the migrateTo no-op assertion
in the test.

Spec: docs/superpowers/specs/2026-05-25-natural-key-slots-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Schema rewrite

**Files:**
- Modify: `internal/sqlite/schema.sql:116-147` (workout_exercise → workout_exercises, exercise_sets) and `195-208` (scheduled_pushes).

**Do not commit yet.** Tasks 2–11 are the big swap; commit at Task 12.

- [ ] **Step 1: Replace the `workout_exercise` table block (lines ~116–132)**

Old (delete):
```sql
CREATE TABLE workout_exercise
(
    id                  INTEGER PRIMARY KEY,
    workout_user_id     INTEGER NOT NULL,
    workout_date        TEXT    NOT NULL CHECK (...),
    exercise_id         INTEGER NOT NULL,
    warmup_completed_at TEXT CHECK (...),
    UNIQUE (workout_user_id, workout_date, exercise_id),
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (...) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) STRICT;

CREATE INDEX workout_exercise_user_exercise_date_idx
    ON workout_exercise (workout_user_id, exercise_id, workout_date);
```

New:
```sql
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
```

- [ ] **Step 2: Replace the `exercise_sets` table block (lines ~134–146)**

Old (delete):
```sql
CREATE TABLE exercise_sets
(
    workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
    set_number          INTEGER NOT NULL CHECK (set_number > 0),
    weight_kg           REAL,
    target_value        INTEGER NOT NULL CHECK (target_value > 0),
    completed_value     INTEGER CHECK (...),
    completed_at        TEXT CHECK (...),
    signal              TEXT CHECK (...),
    PRIMARY KEY (workout_exercise_id, set_number)
) WITHOUT ROWID, STRICT;
```

New:
```sql
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
```

- [ ] **Step 3: Replace the `scheduled_pushes` table block (lines ~195–208)**

Old (delete):
```sql
CREATE TABLE scheduled_pushes
(
    id                  INTEGER PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
    fire_at             TEXT    NOT NULL CHECK (...),
    payload             TEXT    NOT NULL CHECK (LENGTH(payload) < 2048),
    created_at          TEXT    NOT NULL DEFAULT (...) CHECK (...)
) STRICT;

CREATE UNIQUE INDEX scheduled_pushes_workout_exercise_id
    ON scheduled_pushes (workout_exercise_id);
CREATE INDEX scheduled_pushes_fire_at ON scheduled_pushes (fire_at);
```

New:
```sql
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
```

- [ ] **Step 4: Skip running tests now**

The build will be broken until Task 11 finishes. Move on to Task 3.

---

## Task 3: Domain — drop `ExerciseSet.ID`, rename slot params

**Files:**
- Modify: `internal/domain/session.go`

- [ ] **Step 1: Drop `ID int` from `ExerciseSet` (currently at line 28)**

```go
// ExerciseSet is one slot in a Session: an exercise plus its sets.
// Identity is the slot's position in Session.ExerciseSets — no surrogate ID.
type ExerciseSet struct {
	Exercise          Exercise
	Sets              []Set
	WarmupCompletedAt *time.Time
}
```

- [ ] **Step 2: Delete `Session.Slot` (lines ~161–168)**

Remove the entire method:
```go
// DELETE:
// func (s *Session) Slot(slotID int) (ExerciseSet, bool) {
//     ... loop scanning by ID ...
// }
```

- [ ] **Step 3: Delete `Session.findSlot` (lines ~366–375)**

Remove the entire method:
```go
// DELETE:
// func (s *Session) findSlot(slotID int) (*ExerciseSet, error) {
//     for i := range s.ExerciseSets {
//         if s.ExerciseSets[i].ID == slotID { return &s.ExerciseSets[i], nil }
//     }
//     return nil, ErrSlotNotFound
// }
```

- [ ] **Step 4: Add a private `slotAt` helper that replaces `findSlot`**

Add near where `findSlot` lived:

```go
// slotAt returns a pointer to the slot at pos within s.ExerciseSets.
// Returns ErrSlotNotFound if pos is out of range.
func (s *Session) slotAt(pos int) (*ExerciseSet, error) {
	if pos < 0 || pos >= len(s.ExerciseSets) {
		return nil, ErrSlotNotFound
	}
	return &s.ExerciseSets[pos], nil
}
```

- [ ] **Step 5: Rename `slotID int` → `pos int` in all public slot methods**

Methods affected (search for `slotID int` in `session.go`): `MarkWarmupComplete`, `SwapExerciseInSlot`, `RecordSet`, `UpdateSetWeight`, `UpdateCompletedValue` (and any other slot-keyed method). For each, rename the parameter and change `s.findSlot(slotID)` to `s.slotAt(pos)`. Example:

```go
// Before
func (s *Session) MarkWarmupComplete(slotID int, now time.Time) error {
	slot, err := s.findSlot(slotID)
	if err != nil { return err }
	// ...
}

// After
func (s *Session) MarkWarmupComplete(pos int, now time.Time) error {
	slot, err := s.slotAt(pos)
	if err != nil { return err }
	// ...
}
```

Do this for every slot-keyed method in `session.go`.

- [ ] **Step 6: Update the `ExerciseSet` struct literal in `AddExercise` (line ~249)**

Old:
```go
s.ExerciseSets = append(s.ExerciseSets, ExerciseSet{ //nolint:exhaustruct // ID set by repo; WarmupCompletedAt nil.
    Exercise: ex, Sets: sets,
})
```

New:
```go
s.ExerciseSets = append(s.ExerciseSets, ExerciseSet{ //nolint:exhaustruct // WarmupCompletedAt nil.
    Exercise: ex, Sets: sets,
})
```

---

## Task 4: Domain — `WeekPlan` dispatchers + `Planner.Plan`

**Files:**
- Modify: `internal/domain/week_plan.go`
- Modify: `internal/domain/planner.go:482` (struct literal)

- [ ] **Step 1: Rename `slotID int` → `pos int` in WeekPlan dispatchers**

The five dispatchers in `week_plan.go` that take a slot identifier are: `MarkWarmupComplete` (line 153), `RecordSet` (line 162), `UpdateSetWeight` (line 174), `UpdateCompletedValue` (line 183), `SwapExerciseInSlot` (line 192). Rename the parameter and thread `pos` through to the inner `Session` call. Example:

```go
// Before
func (wp *WeekPlan) SwapExerciseInSlot(date time.Time, slotID int, newEx Exercise, sets []Set) error {
	s := wp.SessionOn(date)
	if s == nil { return ErrNotFound }
	return s.SwapExerciseInSlot(slotID, newEx, sets)
}

// After
func (wp *WeekPlan) SwapExerciseInSlot(date time.Time, pos int, newEx Exercise, sets []Set) error {
	s := wp.SessionOn(date)
	if s == nil { return ErrNotFound }
	return s.SwapExerciseInSlot(pos, newEx, sets)
}
```

Apply to every dispatcher with the same shape.

- [ ] **Step 2: Drop `ID:` from the `ExerciseSet` struct literal in `planner.go:482`**

Old:
```go
return ExerciseSet{ //nolint:exhaustruct // ID auto-assigned at insert; WarmupCompletedAt nil.
    ID: 0, Exercise: ex, Sets: sets,
}
```

New:
```go
return ExerciseSet{ //nolint:exhaustruct // WarmupCompletedAt nil.
    Exercise: ex, Sets: sets,
}
```

(If the original code did not have an explicit `ID: 0`, just remove the field from the nolint comment.)

---

## Task 5: Domain — `ScheduledPush` rewire

**Files:**
- Modify: `internal/domain/scheduled_push.go`
- Modify: `internal/domain/rest_push.go` (caller signature change)

- [ ] **Step 1: Replace `WorkoutExerciseID` with `WorkoutDate` + `Position`**

In `internal/domain/scheduled_push.go`:

```go
package domain

import "time"

type ScheduledPush struct {
	ID          int
	UserID      int
	WorkoutDate time.Time
	Position    int
	FireAt      time.Time
	Payload     string
	CreatedAt   time.Time
}
```

- [ ] **Step 2: No change needed in `rest_push.go`**

`PlanRestPush` (line 56) returns a `RestPushDecision`, not a `ScheduledPush` — it only inspects a slot to decide what to do next. It does not embed slot identity. The conversion from decision to scheduled push happens in service code (`applyRestPushDecision` in `internal/service/sets.go`), where the `(userID, date, pos)` is already in scope; that wiring is updated in Task 11.

---

## Task 6: Domain tests — rename and update struct literals

**Files:**
- Modify: `internal/domain/session_test.go`
- Modify: `internal/domain/week_plan_test.go`
- Modify: `internal/domain/rest_push_test.go`
- Modify: `internal/domain/planner_test.go` (if it has ID literals)

- [ ] **Step 1: Rename two key tests in `session_test.go`**

```go
// Was: Test_Session_SwapExerciseInSlot_PreservesSlotID (line 411)
func Test_Session_SwapExerciseInSlot_PreservesPosition(t *testing.T) {
    // Adapt body: insert exercises at positions 0, 1, 2; swap at position 1;
    // assert position 1 still references the swapped exercise; positions 0 and 2 unchanged.
}

// Was: Test_Session_SwapExerciseInSlot_UnknownSlot (line 446)
func Test_Session_SwapExerciseInSlot_OutOfRange(t *testing.T) {
    // Pass pos=99 to a session with 2 slots; expect ErrSlotNotFound.
    err := sess.SwapExerciseInSlot(99, domain.Exercise{ID: 2}, nil) //nolint:exhaustruct
    if !errors.Is(err, domain.ErrSlotNotFound) {
        t.Fatalf("err = %v, want ErrSlotNotFound", err)
    }
}
```

- [ ] **Step 2: Strip `ID:` from every `ExerciseSet` struct literal across the four test files**

Search pattern: `ExerciseSet{` followed by an `ID:` field. Remove the `ID:` line. Update accompanying `//nolint:exhaustruct` comments if they referenced `ID`.

Example sites to fix include the six occurrences in `rest_push_test.go` (lines 55, 64, 74, 93, 112, 122). Apply the same change anywhere else such a literal appears.

- [ ] **Step 3: Update any test that calls a slot method by ID**

If a test did `sess.RecordSet(11, ...)` to address "the slot whose ID is 11", change it to `sess.RecordSet(pos, ...)` where `pos` is the array index of that slot in the test's setup. Most tests build sessions with a known slot order so this is mechanical.

- [ ] **Step 4: Update `ScheduledPush` test fixtures**

Anywhere `domain.ScheduledPush{WorkoutExerciseID: 12, ...}` appears, change to `domain.ScheduledPush{WorkoutDate: time.Date(...), Position: 1, ...}`. Same for `PlanRestPush` callers — pass the position explicitly.

---

## Task 7: Repository — `saveOneSlotInTx` + `saveExerciseSetsInTx`

**Files:**
- Modify: `internal/repository/shared.go:187–237` (the slot-write helpers)

- [ ] **Step 1: Rewrite `saveOneSlotInTx` to take an explicit position**

```go
// saveOneSlotInTx inserts a single workout_exercises row at the given position
// and its child exercise_sets rows. The set_number sequence is per-slot and
// starts at 1; reordering slots at the caller does not affect that numbering.
func (r baseRepository) saveOneSlotInTx(
    ctx context.Context,
    tx *sql.Tx,
    date time.Time,
    pos int,
    slot domain.ExerciseSet,
) error {
    dateStr := formatDate(date)
    userID := contexthelpers.AuthenticatedUserID(ctx)

    var warmupArg any
    if slot.WarmupCompletedAt != nil {
        warmupArg = formatTimestamp(*slot.WarmupCompletedAt)
    }
    if _, err := tx.ExecContext(ctx, `
        INSERT INTO workout_exercises (
            workout_user_id, workout_date, position, exercise_id, warmup_completed_at
        ) VALUES (?, ?, ?, ?, ?)`,
        userID, dateStr, pos, slot.Exercise.ID, warmupArg); err != nil {
        return fmt.Errorf("insert workout_exercises: %w", err)
    }
    for i, set := range slot.Sets {
        var completedAtStr any
        if set.CompletedAt != nil {
            completedAtStr = formatTimestamp(*set.CompletedAt)
        }
        var signalValue any
        if set.Signal != nil {
            signalValue = string(*set.Signal)
        }
        if _, err := tx.ExecContext(ctx, `
            INSERT INTO exercise_sets (
                workout_user_id, workout_date, position, set_number,
                weight_kg, target_value, completed_value, completed_at, signal
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
            userID, dateStr, pos, i+1,
            set.WeightKg, set.TargetValue, set.CompletedValue, completedAtStr, signalValue); err != nil {
            return fmt.Errorf("insert exercise_sets: %w", err)
        }
    }
    return nil
}
```

- [ ] **Step 2: Collapse `saveExerciseSetsInTx` to a single pass**

The current implementation does an explicit-ID-then-auto-ID two-pass over `sess.ExerciseSets`. Replace it with a single loop that calls `saveOneSlotInTx(..., pos, slot)` with `pos` being the loop index:

```go
func (r baseRepository) saveExerciseSetsInTx(
    ctx context.Context, tx *sql.Tx, sess domain.Session,
) error {
    for pos, slot := range sess.ExerciseSets {
        if err := r.saveOneSlotInTx(ctx, tx, sess.Date, pos, slot); err != nil {
            return err
        }
    }
    return nil
}
```

- [ ] **Step 3: Update `insertSessionInTx` if it called `saveOneSlotInTx` directly**

If the composite write helper passed `slot` without `pos`, change call sites to use the loop above or pass the index explicitly.

---

## Task 8: Repository — `week_plans.go` single-pass reinsert + read SQL

**Files:**
- Modify: `internal/repository/week_plans.go`

- [ ] **Step 1: Replace `reinsertWeekInTx` and delete `reinsertSlotsForWeek`**

```go
// reinsertWeekInTx persists wp's sessions in a single pass. Positions come
// from the in-memory array index; no autoincrement, no rowid collisions.
func (r *sqliteWeekPlanRepository) reinsertWeekInTx(
    ctx context.Context, tx *sql.Tx, wp domain.WeekPlan,
) error {
    for i := range wp.Sessions {
        sess := wp.Sessions[i]
        if isRestDayPlaceholder(sess) {
            continue
        }
        if err := r.insertSessionRowInTx(ctx, tx, sess); err != nil {
            return fmt.Errorf("insert session row %s: %w", formatDate(sess.Date), err)
        }
        for pos, slot := range sess.ExerciseSets {
            if err := r.saveOneSlotInTx(ctx, tx, sess.Date, pos, slot); err != nil {
                return fmt.Errorf("save slot %d for %s: %w", pos, formatDate(sess.Date), err)
            }
        }
    }
    return nil
}
```

Delete `reinsertSlotsForWeek` entirely (no callers remain).

- [ ] **Step 2: Update the doc comment on `Update`**

```go
// Update loads the WeekPlan for monday inside a single transaction, runs fn,
// then persists the result via delete-then-reinsert across the week's date
// range. The reinsert is a single pass — slot identity is the array index in
// Session.ExerciseSets, written into the row's position column, so SQLite
// never auto-assigns a rowid. Domain sentinels returned by fn propagate
// unchanged so callers can errors.Is against them.
```

- [ ] **Step 3: Update `Create`'s comment and any references to surrogate id**

Replace "INSERT ... RETURNING id" comments with "INSERT with explicit (user, date, position)". The PK conflict check now triggers on the workout_sessions PK or the new composite slot PK; the existing `ErrConstraintPrimaryKey` mapping still works.

---

## Task 9: Repository — read SQL projection

**Files:**
- Modify: `internal/repository/sessions.go` (and any other file that does `SELECT ... FROM workout_exercise`)
- Modify: `internal/repository/week_plans.go:115–153` (`getInTx`)

- [ ] **Step 1: Update `listSessionRowsBetween` (or equivalent)**

If it has a JOIN to `workout_exercise`, rename to `workout_exercises` and project no surrogate id.

- [ ] **Step 2: Update `loadExerciseSetsSince`**

Project the new composite. Example shape:

```go
rows, err := q.QueryContext(ctx, `
    SELECT we.workout_date, we.position, we.exercise_id, we.warmup_completed_at,
           es.set_number, es.weight_kg, es.target_value,
           es.completed_value, es.completed_at, es.signal,
           e.name -- or whatever exercise columns are joined
    FROM workout_exercises we
    LEFT JOIN exercise_sets es
        ON  es.workout_user_id = we.workout_user_id
        AND es.workout_date    = we.workout_date
        AND es.position        = we.position
    LEFT JOIN exercises e ON e.id = we.exercise_id
    WHERE we.workout_user_id = ? AND we.workout_date >= ?
    ORDER BY we.workout_date, we.position, es.set_number
`, userID, formatDate(since))
```

Hydration loop builds `map[string][]domain.ExerciseSet` keyed by date, with the inner slice indexed by position. Scan each row's `position` to assign to the correct slot index.

- [ ] **Step 3: Update `getInTx` in `week_plans.go`**

The session-population loop already uses an offset into a fixed-size `Sessions[7]`. The exercise-set hydration now indexes by position directly: `sess.ExerciseSets[pos] = slot` (grow the slice first if needed, or build it sized to `MAX(position)+1` from the read).

---

## Task 10: Repository — `scheduled_push.go` rewire

**Files:**
- Modify: `internal/repository/scheduled_push.go`

- [ ] **Step 1: Rewrite `Replace` for the composite upsert**

```go
func (r *sqliteScheduledPushRepository) Replace(
    ctx context.Context, push domain.ScheduledPush,
) (domain.ScheduledPush, error) {
    var createdAt sql.NullString
    err := r.db.ReadWrite.QueryRowContext(ctx, `
        INSERT INTO scheduled_pushes (workout_user_id, workout_date, position, fire_at, payload)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT (workout_user_id, workout_date, position) DO UPDATE SET
            fire_at = excluded.fire_at,
            payload = excluded.payload
        RETURNING id, created_at`,
        push.UserID, formatDate(push.WorkoutDate), push.Position,
        formatTimestamp(push.FireAt), push.Payload,
    ).Scan(&push.ID, &createdAt)
    if err != nil {
        return domain.ScheduledPush{}, fmt.Errorf("upsert scheduled push: %w", err)
    }
    if push.CreatedAt, err = parseTimestamp(createdAt); err != nil {
        return domain.ScheduledPush{}, fmt.Errorf("parse created_at: %w", err)
    }
    return push, nil
}
```

- [ ] **Step 2: Replace `DeleteByWorkoutExercise` with `DeleteBySlot`**

```go
func (r *sqliteScheduledPushRepository) DeleteBySlot(
    ctx context.Context, userID int, date time.Time, pos int,
) error {
    if _, err := r.db.ReadWrite.ExecContext(ctx, `
        DELETE FROM scheduled_pushes
        WHERE workout_user_id = ? AND workout_date = ? AND position = ?`,
        userID, formatDate(date), pos,
    ); err != nil {
        return fmt.Errorf("delete scheduled push by slot: %w", err)
    }
    return nil
}
```

- [ ] **Step 3: Replace `Get` with `GetBySlot`**

```go
func (r *sqliteScheduledPushRepository) GetBySlot(
    ctx context.Context, userID int, date time.Time, pos int,
) (domain.ScheduledPush, error) {
    var (
        push      domain.ScheduledPush
        workDate  sql.NullString
        fireAt    sql.NullString
        createdAt sql.NullString
    )
    err := r.db.ReadOnly.QueryRowContext(ctx, `
        SELECT id, workout_user_id, workout_date, position, fire_at, payload, created_at
        FROM scheduled_pushes
        WHERE workout_user_id = ? AND workout_date = ? AND position = ?`,
        userID, formatDate(date), pos,
    ).Scan(&push.ID, &push.UserID, &workDate, &push.Position, &fireAt, &push.Payload, &createdAt)
    if errors.Is(err, sql.ErrNoRows) {
        return domain.ScheduledPush{}, domain.ErrNotFound
    }
    if err != nil {
        return domain.ScheduledPush{}, fmt.Errorf("query scheduled push: %w", err)
    }
    if push.WorkoutDate, err = parseDate(workDate.String); err != nil {
        return domain.ScheduledPush{}, fmt.Errorf("parse workout_date: %w", err)
    }
    if push.FireAt, err = parseTimestamp(fireAt); err != nil {
        return domain.ScheduledPush{}, fmt.Errorf("parse fire_at: %w", err)
    }
    if push.CreatedAt, err = parseTimestamp(createdAt); err != nil {
        return domain.ScheduledPush{}, fmt.Errorf("parse created_at: %w", err)
    }
    return push, nil
}
```

If a `parseDate` helper does not exist, add one (see `parseTimestamp` for the pattern, but parse `time.DateOnly` format).

- [ ] **Step 4: Simplify `DeleteByWorkoutSession`**

```go
func (r *sqliteScheduledPushRepository) DeleteByWorkoutSession(
    ctx context.Context, userID int, date time.Time,
) error {
    if _, err := r.db.ReadWrite.ExecContext(ctx, `
        DELETE FROM scheduled_pushes
        WHERE workout_user_id = ? AND workout_date = ?`,
        userID, formatDate(date),
    ); err != nil {
        return fmt.Errorf("delete scheduled pushes by session: %w", err)
    }
    return nil
}
```

- [ ] **Step 5: Update `ListAll` projection**

```go
rows, err := r.db.ReadOnly.QueryContext(ctx, `
    SELECT id, workout_user_id, workout_date, position, fire_at, payload, created_at
    FROM scheduled_pushes
    ORDER BY fire_at ASC`)
```

Scan into `&push.ID, &push.UserID, &workDate, &push.Position, &fireAt, &push.Payload, &createdAt` and parse the date / timestamps as in `GetBySlot`.

- [ ] **Step 6: Update the interface declaration**

`internal/repository/repository.go` likely exports a `ScheduledPushRepository` interface. Rename the methods there:

```go
type ScheduledPushRepository interface {
    Replace(ctx context.Context, push domain.ScheduledPush) (domain.ScheduledPush, error)
    Delete(ctx context.Context, id int) error
    DeleteBySlot(ctx context.Context, userID int, date time.Time, pos int) error
    DeleteByWorkoutSession(ctx context.Context, userID int, date time.Time) error
    GetBySlot(ctx context.Context, userID int, date time.Time, pos int) (domain.ScheduledPush, error)
    ListAll(ctx context.Context) ([]domain.ScheduledPush, error)
}
```

---

## Task 11: Service layer — rename `workoutExerciseID` → `pos`; rewire push calls

**Files:**
- Modify: `internal/service/service.go` (Scheduler interface)
- Modify: `internal/service/exercises.go`
- Modify: `internal/service/sets.go`
- Modify: `internal/service/sessions.go`
- Modify: `internal/service/push.go`
- Modify: `internal/service/*_test.go` (call sites)

- [ ] **Step 1: Update the `Scheduler` interface in `service.go:25`**

```go
type Scheduler interface {
    Cancel(ctx context.Context, userID int, date time.Time, pos int) error
    // ... other methods unchanged
}
```

If the scheduler has a `Schedule` method too, update its signature to take a `domain.ScheduledPush` (which now carries the composite). The implementation in `internal/service/push.go` follows the new shape.

- [ ] **Step 2: Update every service method that takes a slot identifier**

Affected methods (search for `workoutExerciseID int` in `internal/service/`): `SwapExercise`, `ListSwapCandidates`, `MarkWarmupComplete`, `RecordSet`, `UpdateSetWeight`, `UpdateCompletedValue` (and any other). Rename parameter to `pos int`, update log fields from `workout_exercise_id` to `position`, and call domain methods with `pos`.

Example (`exercises.go:63`):

```go
// Before
func (s *Service) SwapExercise(
    ctx context.Context,
    date time.Time,
    workoutExerciseID int,
    newExerciseID int,
) error {
    // ... uses workoutExerciseID throughout ...
    return sess.SwapExerciseInSlot(workoutExerciseID, newExercise, newSets)
}

// After
func (s *Service) SwapExercise(
    ctx context.Context,
    date time.Time,
    pos int,
    newExerciseID int,
) error {
    // ... uses pos throughout ...
    return sess.SwapExerciseInSlot(pos, newExercise, newSets)
}
```

The slog field name in calls like `slog.Int("workout_exercise_id", workoutExerciseID)` becomes `slog.Int("position", pos)`.

- [ ] **Step 3: Update the `findInSession`-by-ID code in `exercises.go:150`**

The current code scans `sess.ExerciseSets` for `es.ID == workoutExerciseID`. Replace with a bounds check + index:

```go
if pos < 0 || pos >= len(sess.ExerciseSets) {
    return domain.Exercise{}, nil, fmt.Errorf("slot %d: %w", pos, domain.ErrSlotNotFound)
}
es := sess.ExerciseSets[pos]
```

- [ ] **Step 4: Update push wiring in `sets.go` and `sessions.go`**

The `applyRestPushDecision` helper and `Cancel`/`Schedule` calls pass `(userID, date, pos)` instead of `(userID, workoutExerciseID)`. The pre/post slot reads use `len`-check + index, not `sess.Slot(workoutExerciseID)`:

```go
// Before
if slot, ok := sess.Slot(workoutExerciseID); ok && setIndex >= 0 && setIndex < len(slot.Sets) { ... }

// After
if pos < 0 || pos >= len(sess.ExerciseSets) { return /* not found */ }
slot := sess.ExerciseSets[pos]
if setIndex < 0 || setIndex >= len(slot.Sets) { return /* not found */ }
```

- [ ] **Step 5: Update service tests**

Every test that calls a slot-keyed service method renames the slot argument to a position. Example (`exercises_test.go:694`):

```go
// Before: squatSlotID was captured from an INSERT ... RETURNING id earlier in the test
if err = svc.SwapExercise(ctx, today, squatSlotID, plankID); err != nil { ... }

// After: squatPos is the array index of that slot when the session was seeded
if err = svc.SwapExercise(ctx, today, squatPos, plankID); err != nil { ... }
```

For tests that seed slots via raw SQL, change the INSERT to use the new schema with explicit `position`, then use that position directly.

---

## Task 12: HTTP layer — routes + handlers + handler tests

**Files:**
- Modify: `cmd/web/routes.go:16-26` (six routes)
- Modify: `cmd/web/handler-exerciseset.go`
- Modify: `cmd/web/handler-workout.go`
- Modify: `cmd/web/handler-exercise-info.go`
- Modify: `cmd/web/handler-exerciseset_test.go`
- Modify: `cmd/web/handler-workout_test.go`
- Modify: `cmd/web/handler-exercise-info_test.go`

- [ ] **Step 1: Rename `{workoutExerciseID}` → `{position}` in routes**

Edit `cmd/web/routes.go` lines 16–26. Each route path containing `{workoutExerciseID}` becomes `{position}`. Example:

```go
mux.Handle("GET /workouts/{date}/exercises/{position}",
    requireAuthenticated(http.HandlerFunc(app.handleExerciseSetGET)))
mux.Handle("POST /workouts/{date}/exercises/{position}/sets/{setIndex}/update",
    requireAuthenticated(http.HandlerFunc(app.handleExerciseSetPOST)))
// ... and the remaining 4 routes.
```

- [ ] **Step 2: Rename `parseWorkoutExerciseIDParam` to `parsePositionParam`**

In `handler-exerciseset.go`, find the helper:

```go
// Before
func (app *application) parseWorkoutExerciseIDParam(w http.ResponseWriter, r *http.Request) (int, bool) {
    workoutExerciseID, err := strconv.Atoi(r.PathValue("workoutExerciseID"))
    // ...
    return workoutExerciseID, true
}

// After
func (app *application) parsePositionParam(w http.ResponseWriter, r *http.Request) (int, bool) {
    pos, err := strconv.Atoi(r.PathValue("position"))
    if err != nil || pos < 0 {
        app.notFound(w, r)
        return 0, false
    }
    return pos, true
}
```

Update every call site (search for `parseWorkoutExerciseIDParam`).

- [ ] **Step 3: Rename `exerciseSetParams.WorkoutExerciseID` → `Position`**

In `handler-exerciseset.go:236-242`:

```go
// Position is the slot's 0-based index in Session.ExerciseSets, taken from the URL.
type exerciseSetParams struct {
    Date     time.Time
    Position int
    SetIndex int
}
```

Update `parseExerciseSetURLParams` (around line 250) to populate `Position` from `r.PathValue("position")` instead of `WorkoutExerciseID` from `r.PathValue("workoutExerciseID")`.

- [ ] **Step 4: Inline `findExerciseSetInSession`**

Delete the function at `handler-exerciseset.go:267-272`. Replace its 3 call sites (lines 173, 415; also one in `handler-exercise-info.go:73`) with a bounds check + index:

```go
// Before
exerciseSet, found := findExerciseSetInSession(&session, workoutExerciseID)
if !found {
    app.notFound(w, r); return
}

// After
if params.Position < 0 || params.Position >= len(session.ExerciseSets) {
    app.notFound(w, r); return
}
exerciseSet := session.ExerciseSets[params.Position]
```

- [ ] **Step 5: Rename slog fields and local variables**

Across `handler-exerciseset.go`, `handler-workout.go`, `handler-exercise-info.go`: every `slog.Int("workout_exercise_id", ...)` becomes `slog.Int("position", ...)` and the local variable `workoutExerciseID` becomes `pos`. Format strings that produce URLs (`/workouts/%s/exercises/%d`) work unchanged — the integer value is now a position rather than a slot ID, same path shape.

Delete the "URL keeps the same workoutExerciseID so any back-navigation still hits this slot" comment at `handler-workout.go:412`; position-stability under swap is automatic.

- [ ] **Step 6: Update handler-test fixtures that seed slots via raw SQL**

The four call sites in `handler-exerciseset_test.go` (lines ~829, 1380, 1485, 1579) currently:

```go
var slotID int
if err := db.QueryRowContext(ctx, `
    INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
    VALUES (1, ?, ?) RETURNING id`, today, exerciseID).Scan(&slotID); err != nil { ... }
slotPath := "/workouts/" + today + "/exercises/" + strconv.Itoa(slotID)
```

Replace with explicit-position inserts:

```go
const pos = 0
if _, err := db.ExecContext(ctx, `
    INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id)
    VALUES (1, ?, ?, ?)`, today, pos, exerciseID); err != nil { ... }
slotPath := "/workouts/" + today + "/exercises/" + strconv.Itoa(pos)
```

And the corresponding `INSERT INTO exercise_sets` follow-ups:

```go
// Before
if _, err := db.ExecContext(ctx, `
    INSERT INTO exercise_sets (workout_exercise_id, set_number, target_value)
    VALUES (?, ?, 5)`, slotID, setNum); err != nil { ... }

// After
if _, err := db.ExecContext(ctx, `
    INSERT INTO exercise_sets (workout_user_id, workout_date, position, set_number, target_value)
    VALUES (1, ?, ?, ?, 5)`, today, pos, setNum); err != nil { ... }
```

Apply the same shape to the SELECT statements lower in those tests that read back `weight_kg` keyed by `slotID` (e.g. line 875) — key by `(workout_user_id, workout_date, position, set_number)` instead.

---

## Task 13: Repository tests — rename and update assertions

**Files:**
- Modify: `internal/repository/week_plans_test.go`
- Modify: `internal/repository/sessions_test.go`
- Modify: `internal/repository/scheduled_push_test.go`

- [ ] **Step 1: Rename `TestWeekPlanRepository_Update_PreservesSlotIDs` → `_PreservesSlotPositions`**

Adapt the body: round-trip a week through `Update` with a no-op closure. Assert each slot is at the same position before and after, with the same `Exercise.ID`. Drop any `ExerciseSet.ID` field references.

- [ ] **Step 2: Update `scheduled_push_test.go` fixtures**

Every raw-SQL fixture that uses `(user_id, workout_exercise_id)` flips to `(workout_user_id, workout_date, position)`. Every assertion that scans `workout_exercise_id` flips to scanning `(workout_date, position)`. The 1:1 invariant test ("one pending push per slot") retargets to the new UNIQUE index.

- [ ] **Step 3: Update `sessions_test.go` fixtures**

Any direct SQL that inserts into `workout_exercise` updates to `workout_exercises (workout_user_id, workout_date, position, exercise_id)`. Any SELECT that joins via `workout_exercise_id` updates to the composite join.

---

## Task 14: Final verification + commit + CLAUDE.md updates

**Files:**
- Modify: `internal/sqlite/migrate_internal_test.go` (un-skip the `migrateTo` no-op assertion)
- Modify: `internal/repository/CLAUDE.md`
- Modify: `internal/sqlite/CLAUDE.md`
- Modify: `internal/domain/CLAUDE.md`

- [ ] **Step 1: Re-enable the deferred assertion in the premigration test**

Replace the `_ = schemaDefinition` placeholder added in Task 1 with the real assertion:

```go
// migrateTo against the live schema is a no-op (asserts shape matches).
if err = db.migrateTo(ctx, schemaDefinition); err != nil {
    t.Fatalf("migrateTo: %v", err)
}
```

- [ ] **Step 2: Run the full pipeline**

Run: `make ci`
Expected: PASS (init + build + lint-fix + test + sec).

If anything fails, fix it. Common pitfalls:
- A test file still has `domain.ExerciseSet{ID: ...}` — strip the field.
- A service test passes a stored `workoutExerciseID` value that no longer exists — replace with the slot's array position.
- A handler test compares URL strings that referenced the old slot ID — recompute the URL from position.

- [ ] **Step 3: Update `internal/repository/CLAUDE.md`**

The "Diff strategy: delete-and-reinsert" section currently describes three-pass logic. Rewrite to:

```markdown
## Diff strategy: delete-and-reinsert

`WeekPlanRepository.Update` persists by deleting every `workout_sessions`
row in `[monday, monday+6]` inside the tx (CASCADE clears `workout_exercises`
and `exercise_sets`) and re-inserting the sessions in a single pass.
Slot identity is the array index in `Session.ExerciseSets`, written into
the row's `position` column — there is no autoincrement, so the order in
which slots are inserted does not matter. For PetrApp's data sizes (a
handful of exercises × a handful of sets per session) the cost is
negligible and the simplicity is worth the trade.
```

Update the "Shared helpers" section to drop the "two-pass: explicit-ID slots before auto-ID slots" parenthetical on `saveExerciseSetsInTx`. Note that `WeekPlanRepository.Update`'s reinsert is single-pass.

- [ ] **Step 4: Update `internal/sqlite/CLAUDE.md`**

The "See git history for `internal/sqlite/premigrate.go` (workout_exercise stable-id migration, PR #75) for a worked example" line is now superseded. Update to:

```markdown
See git history for `internal/sqlite/premigrate.go` (workout_exercise natural-key
migration; see spec at `docs/superpowers/specs/2026-05-25-natural-key-slots-design.md`)
for the most recent worked example.
```

- [ ] **Step 5: Update `internal/domain/CLAUDE.md`** (verified: no slot-ID references today; skip unless the engineer's edits surfaced new references worth documenting)

- [ ] **Step 6: Commit the big swap**

```bash
git add internal/sqlite/schema.sql \
        internal/sqlite/migrate_internal_test.go \
        internal/domain/session.go internal/domain/week_plan.go \
        internal/domain/scheduled_push.go internal/domain/planner.go \
        internal/domain/rest_push.go \
        internal/domain/session_test.go internal/domain/week_plan_test.go \
        internal/domain/rest_push_test.go internal/domain/planner_test.go \
        internal/repository/shared.go internal/repository/week_plans.go \
        internal/repository/sessions.go internal/repository/scheduled_push.go \
        internal/repository/repository.go \
        internal/repository/week_plans_test.go internal/repository/sessions_test.go \
        internal/repository/scheduled_push_test.go \
        internal/service/service.go internal/service/exercises.go \
        internal/service/sets.go internal/service/sessions.go \
        internal/service/push.go \
        internal/service/exercises_test.go internal/service/sets_test.go \
        internal/service/sessions_test.go internal/service/push_test.go \
        cmd/web/routes.go cmd/web/handler-exerciseset.go \
        cmd/web/handler-workout.go cmd/web/handler-exercise-info.go \
        cmd/web/handler-exerciseset_test.go cmd/web/handler-workout_test.go \
        cmd/web/handler-exercise-info_test.go \
        internal/repository/CLAUDE.md internal/sqlite/CLAUDE.md \
        internal/domain/CLAUDE.md

git commit -m "$(cat <<'EOF'
feat: replace workout_exercise.id with natural composite key

Slot identity is now (workout_user_id, workout_date, position). Drops
ExerciseSet.ID and the URL-stable surrogate id; HTTP routes rename
{workoutExerciseID} -> {position}. Collapses WeekPlanRepository.Update's
three-pass reinsert to a single pass. Re-keys scheduled_pushes onto the
composite, drops its redundant user_id column. Renames workout_exercise
to workout_exercises for plural consistency.

Spec: docs/superpowers/specs/2026-05-25-natural-key-slots-design.md

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 7: Manual smoke test against the dev app**

Run: `make run` (or whichever target boots the local app).
- Open `/workouts/<today>` and confirm the session renders.
- Click into a slot — confirm the URL is `/workouts/<today>/exercises/0` (or similar low integer).
- Tap "Swap exercise" — confirm the swap completes and the URL position remains stable.
- Mark a set complete — confirm the page redirects correctly and the data persists across reload.

If anything is wrong, fix forward; do not amend the commit (system rules) — create a new follow-up commit.

---

## Task 15: Pre-deploy snapshot + deploy

**Outside the codebase** — operational checklist:

- [ ] **Step 1: Take a Fly snapshot of the production DB**

Use the `fly-ops` skill to snapshot the live SQLite volume before deploying. This is the manual rollback point.

- [ ] **Step 2: Deploy**

Push to main; Fly redeploys. The premigration runs once at boot, exits, and `migrateTo` is a no-op against the new shape.

- [ ] **Step 3: Smoke test in production**

Open the deployed app, navigate to today's workout, do one set or one swap. Confirm push notifications still schedule (if applicable).

---

## Task 16: Post-deploy cleanup (separate PR)

**Files:**
- Modify: `internal/sqlite/premigrate.go` — delete the file.
- Modify: `internal/sqlite/sqlite.go` — remove the `preMigrateWorkoutPositions` call.
- Modify: `internal/sqlite/migrate_internal_test.go` — delete `TestDatabase_preMigrateWorkoutPositions` and its `legacy` schema fixture.

**Run only after production has booted past the premigration at least once.** The CLAUDE.md says: "There is no version table — the only signal that a premigration is no longer needed is that production has booted past it."

- [ ] **Step 1: Delete `internal/sqlite/premigrate.go`**

```bash
rm internal/sqlite/premigrate.go
```

- [ ] **Step 2: Remove the call from `NewDatabase` in `sqlite.go`**

Delete the `if err = db.preMigrateWorkoutPositions(ctx); err != nil { ... }` block.

- [ ] **Step 3: Delete `TestDatabase_preMigrateWorkoutPositions` from `migrate_internal_test.go`**

Remove the entire test function added in Task 1.

- [ ] **Step 4: Run the full pipeline**

Run: `make ci`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sqlite/sqlite.go internal/sqlite/migrate_internal_test.go
git rm internal/sqlite/premigrate.go
git commit -m "$(cat <<'EOF'
sqlite: retire preMigrateWorkoutPositions

Production booted past the natural-key slot premigration in commit
<HASH>. Removing the premigration file, its wiring in NewDatabase,
and the test fixture.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

(Replace `<HASH>` with the deploy commit hash before committing.)
