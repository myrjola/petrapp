# Natural-key slots: replacing the surrogate workout_exercise.id

Date: 2026-05-25
Status: Approved — ready for implementation planning

## Background

The WeekPlan aggregate refactor (merged in `b49790c`, May 2026) moved the workout-write boundary from per-day `SessionRepository.Update` to week-scoped `WeekPlanRepository.Update`. Persistence inside that boundary is delete-and-reinsert: the whole `[monday, monday+6]` range is wiped and re-written from the in-memory `domain.WeekPlan` on every mutation.

That pattern works, but the schema fights it. `workout_exercise.id` is an `INTEGER PRIMARY KEY` (rowid alias) that must be preserved across the delete-reinsert so URL paths like `/workouts/{date}/exercises/{workoutExerciseID}/...` keep pointing at the same slot. Preserving rowids while also inserting new (auto-assign) slots in the same transaction forces a three-pass reinsert (`internal/repository/week_plans.go:155-207`) to dodge SQLite's `UNIQUE constraint failed: workout_exercise.id` when its rowid pointer lands on an ID we're about to claim explicitly.

The three-pass split is load-bearing cleverness someone needs to remember. The root cause is a model mismatch: `workout_exercise.id` implies "this slot is an independently-identified entity," but the delete-and-reinsert pattern treats slots as parts of the WeekPlan aggregate. Pick one model and the friction disappears.

This spec picks the second model: aggregate-internal entities get composite natural keys derived from their position in the aggregate. The surrogate `workout_exercise.id` is dropped. The slot's identity becomes `(workout_user_id, workout_date, position)`, where `position` is its 0-based index in `Session.ExerciseSets`.

## Goals

1. Drop `workout_exercise.id`. `workout_exercises` (renamed for plural consistency) gets composite PK `(workout_user_id, workout_date, position)`.
2. Children re-key on the composite. `exercise_sets` PK becomes `(workout_user_id, workout_date, position, set_number)`. `scheduled_pushes` cross-aggregate FK becomes the same composite (no more `workout_exercise_id` column).
3. Collapse `reinsertWeekInTx` to a single-pass insert in any order. Delete the `reinsertSlotsForWeek` helper and the three-pass commentary.
4. Domain `ExerciseSet` loses its `ID` field. Slot lookup is direct slice indexing with a bounds check; no `findSlot`-by-ID helper.
5. URL routes rename `{workoutExerciseID}` → `{position}` across the six affected paths. URL stability is explicitly out of scope (single-user app, yank tolerated).

## Non-goals

- Preserving existing slot URLs. Bookmarked links to `/workouts/.../exercises/{old-id}/...` will 404 after deploy. Acceptable.
- Splitting the rollout across multiple PRs or deploys. Single PR, single deploy, one premigration.
- Touching the `UNIQUE (workout_user_id, workout_date, exercise_id)` invariant. The "same exercise can't appear twice in one session" rule stays.
- Renaming `workout_user_id` columns to `user_id` on `workout_exercises`/`exercise_sets`/`scheduled_pushes`. Composite-FK column naming convention preserved.
- Producing a counter-migration. Rollback story is fix-forward, backed by a pre-deploy Fly snapshot.

## Schema (target state)

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
    FOREIGN KEY (workout_user_id, workout_date)
        REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
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

Notes:

- `workout_exercises` flips to `WITHOUT ROWID` (consistent with `workout_sessions` and `exercise_sets`); no autoincrement.
- `scheduled_pushes` keeps its own surrogate `id` (it is a separate aggregate — the push queue) but drops the redundant `user_id` column. The push dispatcher reads system-wide via `ORDER BY fire_at`; no per-user query path exists today.
- Cascade on user delete is preserved transitively: `users` → `workout_sessions` → `workout_exercises` → `scheduled_pushes`.
- The exercise-history lookup index (formerly `workout_exercise_user_exercise_date_idx`) survives, renamed.

## Premigration: `preMigrateWorkoutPositions`

Follows the documented pattern (`internal/sqlite/CLAUDE.md`, "Premigration Escape Hatch"). One-shot, idempotent, runs at boot between `connect` and `migrateTo`.

**Detection (early-return):** if `pragma_table_info('workout_exercise')` returns zero rows, return without doing anything. This covers both the fresh DB case (test/in-memory startup — neither table exists yet; `migrateTo` will create `workout_exercises` directly from `schema.sql`) and the already-migrated case (the legacy singular table was dropped at the end of an earlier premigration run).

**Rewrite (single transaction, FK off):**

1. Create `workout_exercises` (target name, target shape). Populate from legacy with positions derived from insert order:
   ```sql
   INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id, warmup_completed_at)
   SELECT workout_user_id, workout_date,
          ROW_NUMBER() OVER (PARTITION BY workout_user_id, workout_date ORDER BY id) - 1,
          exercise_id, warmup_completed_at
   FROM workout_exercise;
   ```
2. Create `exercise_sets_new` with composite parent FK. Populate by joining old `workout_exercise.id` to the new composite via `(workout_user_id, workout_date, exercise_id)` (the preserved UNIQUE makes the join one-to-one):
   ```sql
   INSERT INTO exercise_sets_new (workout_user_id, workout_date, position, set_number, weight_kg, target_value, completed_value, completed_at, signal)
   SELECT new_we.workout_user_id, new_we.workout_date, new_we.position,
          es.set_number, es.weight_kg, es.target_value, es.completed_value, es.completed_at, es.signal
   FROM exercise_sets es
   JOIN workout_exercise old_we   ON old_we.id = es.workout_exercise_id
   JOIN workout_exercises new_we
        ON new_we.workout_user_id = old_we.workout_user_id
       AND new_we.workout_date    = old_we.workout_date
       AND new_we.exercise_id     = old_we.exercise_id;
   ```
3. Create `scheduled_pushes_new` (composite FK; no `user_id` column). Populate with the same join, projecting `(workout_user_id, workout_date, position)` from the new table:
   ```sql
   INSERT INTO scheduled_pushes_new (id, workout_user_id, workout_date, position, fire_at, payload, created_at)
   SELECT sp.id,
          new_we.workout_user_id, new_we.workout_date, new_we.position,
          sp.fire_at, sp.payload, sp.created_at
   FROM scheduled_pushes sp
   JOIN workout_exercise old_we ON old_we.id = sp.workout_exercise_id
   JOIN workout_exercises new_we
        ON new_we.workout_user_id = old_we.workout_user_id
       AND new_we.workout_date    = old_we.workout_date
       AND new_we.exercise_id     = old_we.exercise_id;
   ```
4. `DROP TABLE scheduled_pushes; DROP TABLE exercise_sets; DROP TABLE workout_exercise;` then `ALTER TABLE … RENAME` the `*_new` tables to their final names. `workout_exercises` is already at its final name.
5. Recreate indexes: `workout_exercises_user_exercise_date_idx`, `scheduled_pushes_slot_uidx`, `scheduled_pushes_fire_at`.

The transaction commits; FK re-enabled by the declarative migrator's outer `defer`. On error inside the transaction, rollback; the app fails to start with a clear signal (no half-migrated state).

**Idempotence:** the second invocation hits the early-return because `pragma_table_info('workout_exercise')` is empty.

## Domain layer (`internal/domain/`)

**`ExerciseSet`** drops its `ID` field:

```go
type ExerciseSet struct {
    Exercise          Exercise
    Sets              []Set
    WarmupCompletedAt *time.Time
}
```

**`Session` methods** rename the slot-keyed parameter from `slotID int` to `pos int` and look up by direct slice indexing:

- `RecordSet`, `UpdateSetWeight`, `UpdateCompletedValue`, `MarkWarmupComplete`, `SwapExerciseInSlot`, `AddExercise` (the last appends, unchanged in semantics).
- `Session.Slot(slotID int) (ExerciseSet, bool)` is **removed**. Callers slice-index with a bounds check inline; the existence test is a `if pos < 0 || pos >= len(s.ExerciseSets)`.
- `Session.findSlot(slotID int)` is **removed**. The few internal callers slice-index directly.
- `domain.ErrSlotNotFound` is **kept** (same semantic — "the slot you asked for doesn't exist"). Returned now on out-of-range position rather than ID miss.

**`WeekPlan` per-day dispatchers** (`week_plan.go:191-247`) thread the rename through:

```go
func (wp *WeekPlan) SwapExerciseInSlot(date time.Time, pos int, newEx Exercise, sets []Set) error {
    s := wp.SessionOn(date)
    if s == nil { return ErrNotFound }
    return s.SwapExerciseInSlot(pos, newEx, sets)
}
```

## Repository layer (`internal/repository/`)

**`saveOneSlotInTx`** (`shared.go:191`) takes `pos int` as a parameter; no `idArg` branch, no `RETURNING id`:

```go
func (r baseRepository) saveOneSlotInTx(
    ctx context.Context, tx *sql.Tx, date time.Time, pos int, slot domain.ExerciseSet,
) error {
    // INSERT INTO workout_exercises (workout_user_id, workout_date, position, exercise_id, warmup_completed_at)
    // VALUES (?, ?, ?, ?, ?)
    // exercise_sets children inherit (workout_user_id, workout_date, position) from the parent insert
}
```

**`reinsertWeekInTx`** (`week_plans.go:162`) collapses to a single pass:

```go
for i := range wp.Sessions {
    sess := wp.Sessions[i]
    if isRestDayPlaceholder(sess) { continue }
    if err := r.insertSessionRowInTx(ctx, tx, sess); err != nil { return err }
    for pos, slot := range sess.ExerciseSets {
        if err := r.saveOneSlotInTx(ctx, tx, sess.Date, pos, slot); err != nil { return err }
    }
}
```

**Deleted:** `reinsertSlotsForWeek`, the `explicitOnly` parameter, the three-pass commentary block. `saveExerciseSetsInTx`'s internal explicit/auto-ID two-pass also collapses to a single pass.

**Reads (`getInTx`, `loadExerciseSetsSince`, `listSessionRowsBetween`)** project `position` and order children by it. The hydration loop assigns `wp.Sessions[i].ExerciseSets[pos]` directly — the DB position matches the in-memory array index.

**`scheduled_push.go`:**

- `Replace` upserts on the composite, no `user_id` column:
  ```sql
  INSERT INTO scheduled_pushes (workout_user_id, workout_date, position, fire_at, payload)
  VALUES (?, ?, ?, ?, ?)
  ON CONFLICT (workout_user_id, workout_date, position) DO UPDATE SET
      fire_at = excluded.fire_at,
      payload = excluded.payload
  RETURNING id, created_at;
  ```
- `DeleteByWorkoutExercise(ctx, workoutExerciseID)` → `DeleteBySlot(ctx, userID int, date time.Time, pos int)`; SQL keys on the composite.
- `Get(ctx, workoutExerciseID)` → `GetBySlot(ctx, userID, date, pos)`.
- `DeleteByWorkoutSession(ctx, userID, date)` simplifies — the subquery through the slots table goes away; direct `WHERE workout_user_id = ? AND workout_date = ?`.
- `ListAll` projection: `workout_user_id, workout_date, position` instead of `user_id, workout_exercise_id`. Ordering unchanged.

**`domain.ScheduledPush`** loses `WorkoutExerciseID`, gains `WorkoutDate time.Time` and `Position int`. `UserID` stays — scanned from the `workout_user_id` column.

## HTTP layer (`cmd/web/`)

**Routes (`routes.go:16-26`):** rename `{workoutExerciseID}` → `{position}` across all six paths. Path shape unchanged.

**Handler-side:**

- `parseWorkoutExerciseIDParam` → `parsePositionParam`. Same Atoi + log shape.
- `exerciseSetParams.WorkoutExerciseID int` → `Position int`.
- `findExerciseSetInSession` (`handler-exerciseset.go:267`) is **inlined**. Callers do the bounds check directly:
  ```go
  if params.Position < 0 || params.Position >= len(session.ExerciseSets) {
      app.notFound(w, r); return
  }
  exerciseSet := session.ExerciseSets[params.Position]
  ```

**Service signatures** (`internal/service/exercises.go`, `sets.go`, etc.): the slot parameter on every method renames `workoutExerciseID int` → `pos int`. The closures inside thread it through to domain unchanged.

**Comment on `handler-workout.go:412`** ("URL keeps the same workoutExerciseID so any back-navigation still hits this slot") deletes — position-stability under swap is automatic. The swap mutates the slot at position N; position N does not move.

## Testing

**Premigration test (`internal/sqlite/migrate_internal_test.go`)** — new case under `TestDatabase_premigrate`-style harness, modeled on the prior PR #75 test:

- `legacyWorkoutExerciseSchema` const reproducing the pre-migration `workout_exercise` (singular, surrogate `id`), `exercise_sets` (FK to surrogate), `scheduled_pushes` (FK to surrogate, with `user_id` column).
- Seed: one user, two sessions on different dates; one session with three slots whose old IDs are non-sequential (e.g. 5, 12, 47 — simulates a history of swaps that left gaps); one session with a single slot; a rest-day session row with zero slots; `exercise_sets` rows including one with `NULL completed_at`; a `scheduled_pushes` row pointing at one of the slots.
- Call `preMigrateWorkoutPositions`, assert:
  - Positions are dense `0..N-1` per `(user, date)` partition, ordered by original `id`.
  - `exercise_sets` rows re-key cleanly; every row's `(user, date, position)` resolves to an existing parent.
  - The `scheduled_pushes` row re-keys to the composite; `user_id` column is gone.
  - Renamed table is `workout_exercises` (plural); legacy `workout_exercise` no longer present.
- Call again, assert idempotence (early-return).
- Call `migrateTo(schemaDefinition)`, assert it is a no-op.

**Repository tests (`internal/repository/`)** — updated in place:

- `TestWeekPlanRepository_Update_PreservesSlotIDs` → `_PreservesSlotPositions`. Round-trip a week through `Update` with no-op closure; assert slot at position N is still at position N with the same exercise.
- `scheduled_push_test.go`: all fixtures and assertions switch from `(user_id, workout_exercise_id)` to `(workout_user_id, workout_date, position)`. The 1:1 invariant test ("one pending push per slot") retargets to `UNIQUE (workout_user_id, workout_date, position)`.

**Domain tests (`internal/domain/`)** — mechanical:

- `Test_Session_SwapExerciseInSlot_PreservesSlotID` → `_PreservesPosition`.
- `Test_Session_SwapExerciseInSlot_UnknownSlot` → `_OutOfRange`. Pass `pos=99`, expect `ErrSlotNotFound`.
- Every `ExerciseSet{ID: N, ...}` struct literal across test files drops the `ID` field.

**Service tests (`internal/service/`):** concurrency story unchanged; the race test at `sessions_test.go:708` (50× Regenerate vs. 50× StartSession) stays as-is. Other tests update the slot parameter at call sites.

**Handler tests (`cmd/web/handler-exerciseset_test.go`):** the four call sites that seed slots via raw SQL (`RETURNING id`, then build `slotPath` from `slotID`) switch to inserting with explicit `position` and computing the URL from that position. No more `RETURNING` round-trip.

## Rollout

**Single PR, single deploy.** Premigration runs once at boot, before `migrateTo`.

**Recommended PR-internal sequencing** (compiler walks you through after step 1):

1. Schema (`internal/sqlite/schema.sql`) + premigration (`internal/sqlite/premigrate.go`) + migration test. Compiles standalone.
2. Domain struct + method signatures.
3. Repository SQL + helpers; repository tests updated.
4. Service signature renames.
5. HTTP routes + handlers; handler tests updated.
6. `CLAUDE.md` updates: `internal/repository/CLAUDE.md` "Diff strategy: delete-and-reinsert" section shortens (three-pass → single-pass; drop the explicit-ID / auto-ID preservation language); `internal/sqlite/CLAUDE.md` "Premigration Escape Hatch" worked-example reference updated to point at this premigration after PR #75's is retired; `internal/domain/CLAUDE.md` updates any references to `ExerciseSet.ID` or slot-ID lookup.

**Pre-deploy:** `fly-ops` snapshot of prod DB so a manual restore point exists.

**Post-deploy cleanup (separate PR, after prod has booted past the premigration):** delete `preMigrateWorkoutPositions`, its wiring in `sqlite.go`, the `legacyWorkoutExerciseSchema` const, and its test case. Commit body notes the production commit hash that booted past it. Same lifecycle as PR #75.

**Risk + rollback:** premigration runs inside a single transaction with FK off; failure aborts the boot with a clear error, no half-migrated state. There is no automatic counter-migration (would need to fabricate surrogate IDs from nothing). If post-migration code is broken in production, fix-forward; the Fly snapshot is the manual escape hatch.

## Decisions log

- **URL stability is not a constraint.** Single-user app, user tolerates yank. Old bookmarks 404 after deploy. (Driving constraint that enables the rest of the design.)
- **Composite key in the children, not a surrogate token.** Considered keeping `workout_exercise.id` as an opaque `slot_uid` while still using composite for joins; rejected as half-finished — surrogate would still need three-pass preservation.
- **Position from `ROW_NUMBER() OVER (PARTITION BY user, date ORDER BY id)`.** No separate display-order column exists; insert order is canonical display order (`session.go:249` appends).
- **`UNIQUE (workout_user_id, workout_date, exercise_id)` preserved.** Planner generates one of each exercise per session; keeping the invariant costs nothing.
- **`scheduled_pushes.user_id` dropped.** Never used in WHERE/JOIN clauses (`scheduled_push.go`). Cascade preserved transitively through the composite FK chain.
- **`Session.Slot`/`findSlot` deleted, not renamed.** Direct slice indexing with inline bounds check is more Go-idiomatic and there are few callers.
- **`ErrSlotNotFound` sentinel name kept.** Same semantic ("the slot you asked for does not exist"), avoids service-layer churn.
- **`saveOneSlotInTx` takes `pos int` as a parameter, not embedded in the struct.** Position is a function of array index; embedding it would be denormalized state and the exact pattern this migration is trying to delete.
- **`DeleteBySlot` / `GetBySlot` naming on `scheduled_push.go`.** Shorter than `DeleteByWorkoutExerciseSlot`; "slot" is already the informal term in the codebase.
- **`ScheduledPush.WorkoutDate` field name (not `Date`).** Consistent with the slot's domain language; sessions are keyed by `WorkoutDate` elsewhere.
- **Table rename `workout_exercise` → `workout_exercises` folded in.** 13 of 14 tables are plural; this was the lone outlier. Free move since the table is being rewritten anyway.
