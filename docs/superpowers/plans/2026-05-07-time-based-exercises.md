# Time-Based Exercise Type Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a `time_based` exercise type so plank prescribes a duration (e.g., "hold 30s") instead of nonsense reps, and collapse the redundant `min_reps`/`max_reps` columns into a single `target_value` whose unit is derived from the parent exercise.

**Architecture:** Adds a fourth value `time_based` to the `exercises.exercise_type` enum. Collapses `exercise_sets.min_reps`/`max_reps`/`completed_reps` into `target_value`/`completed_value`; the unit is reps for rep-based exercises and seconds for `time_based`. A parallel `TimedProgression` engine in `internal/exerciseprogression/` provides Signal-driven duration auto-regulation. A one-shot premigration translates legacy rows.

**Tech Stack:** Go, SQLite (STRICT mode), html/template, declarative-schema migrator with premigration escape hatch.

**Spec:** `specs/05-time-based-exercises.md`.

---

## File Structure

**Create:**
- `internal/sqlite/premigrate.go` — one-shot exercise_sets premigration
- `internal/exerciseprogression/timed_progression.go` — duration auto-regulation
- `internal/exerciseprogression/timed_progression_test.go` — table-driven tests for the ladder

**Modify:**
- `internal/sqlite/schema.sql` — `time_based` enum, `default_starting_seconds`, collapsed `exercise_sets`
- `internal/sqlite/fixtures.sql` — plank flips to `time_based`
- `internal/sqlite/sqlite.go` — wire premigration between `connect` and `migrateTo`
- `internal/sqlite/migrate_internal_test.go` — premigration test + legacy schema constant
- `internal/workout/models.go` — `ExerciseTypeTime`, `Exercise.DefaultStartingSeconds`, `Exercise.IsTimed()`, `Set` field rename
- `internal/workout/repository.go` / `repository-sessions.go` / `repository-exercises.go` — SQL select/insert with new columns
- `internal/workout/service.go` — `IsTimed` branches, `UpdateCompletedValue`, `GetStartingSeconds`
- `internal/workout/service_test.go` / `service_internal_test.go` — field references
- `internal/weekplanner/weekplanner.go` — `PlannedSet.TargetValue`, `setsForPeriodization` simplification, `time_based` emission
- `internal/weekplanner/weekplanner_internal_test.go` — field references
- `cmd/web/handler-exerciseset.go` — `formatTarget` helper, `setDisplay`, `completed_value` form
- `cmd/web/handler-exerciseset_test.go` — fixture SQL + assertions
- `cmd/web/handler-exercise-info.go` — `processEntryData` `time_based` case
- `cmd/web/handler-admin-exercises.go` — admin form for `time_based` + `default_starting_seconds`
- `ui/templates/pages/exerciseset/sets-container.gohtml` — unit-agnostic rendering
- `ui/templates/pages/workout/workout.gohtml` — same
- `ui/templates/pages/admin/exercises*.gohtml` — admin form additions

---

## Task 1: TimedProgression engine

Pure additive; no breaking changes. Implements the duration auto-regulation engine in isolation so it compiles and tests on its own before the schema refactor lands.

**Files:**
- Create: `internal/exerciseprogression/timed_progression.go`
- Test: `internal/exerciseprogression/timed_progression_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/exerciseprogression/timed_progression_test.go`:

```go
package exerciseprogression_test

import (
	"testing"

	"petrapp/internal/exerciseprogression"
)

func TestTimedProgressionCurrentSet(t *testing.T) {
	t.Parallel()

	type setup struct {
		startingSeconds int
		completed       []exerciseprogression.TimedSetResult
	}
	tests := []struct {
		name string
		in   setup
		want int
	}{
		{
			name: "first set returns starting seconds",
			in:   setup{startingSeconds: 30, completed: nil},
			want: 30,
		},
		{
			name: "on_target keeps target",
			in: setup{startingSeconds: 30, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 30, Signal: exerciseprogression.SignalOnTarget},
			}},
			want: 30,
		},
		{
			name: "too_light under 60s bumps by 5",
			in: setup{startingSeconds: 30, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 30, Signal: exerciseprogression.SignalTooLight},
			}},
			want: 35,
		},
		{
			name: "too_light at 60s bumps by 10",
			in: setup{startingSeconds: 60, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 60, Signal: exerciseprogression.SignalTooLight},
			}},
			want: 70,
		},
		{
			name: "too_light at 120s bumps by 15",
			in: setup{startingSeconds: 120, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 120, Signal: exerciseprogression.SignalTooLight},
			}},
			want: 135,
		},
		{
			name: "too_heavy under 60s drops by 5",
			in: setup{startingSeconds: 30, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 20, Signal: exerciseprogression.SignalTooHeavy},
			}},
			want: 25,
		},
		{
			name: "too_heavy 10% drop snapped to 5s when larger than ladder step",
			in: setup{startingSeconds: 90, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 70, Signal: exerciseprogression.SignalTooHeavy},
			}},
			// 10% of 90 = 9, snap5 = 10, ladder at 60-119s = 10 → max(10,10) = 10 → 80
			want: 80,
		},
		{
			name: "too_heavy floors at 5s",
			in: setup{startingSeconds: 5, completed: []exerciseprogression.TimedSetResult{
				{ActualSeconds: 5, Signal: exerciseprogression.SignalTooHeavy},
			}},
			want: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := exerciseprogression.NewTimedFromHistory(
				exerciseprogression.TimedConfig{StartingSeconds: tt.in.startingSeconds},
				tt.in.completed,
			)
			got := p.CurrentSet().TargetSeconds
			if got != tt.want {
				t.Errorf("TargetSeconds = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTimedProgressionRecordCompletion(t *testing.T) {
	t.Parallel()

	p := exerciseprogression.NewTimed(exerciseprogression.TimedConfig{StartingSeconds: 30})
	if got := p.SetsCompleted(); got != 0 {
		t.Fatalf("SetsCompleted before any record = %d, want 0", got)
	}
	p.RecordCompletion(exerciseprogression.TimedSetResult{
		ActualSeconds: 30,
		Signal:        exerciseprogression.SignalOnTarget,
	})
	if got := p.SetsCompleted(); got != 1 {
		t.Errorf("SetsCompleted after one record = %d, want 1", got)
	}
	if got := p.CurrentSet().TargetSeconds; got != 30 {
		t.Errorf("TargetSeconds after on_target = %d, want 30", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/exerciseprogression/ -run TestTimedProgression -v`
Expected: build failure — `TimedProgression`, `NewTimed`, `NewTimedFromHistory`, `TimedConfig`, `TimedSetResult` undefined.

- [ ] **Step 3: Implement TimedProgression**

Create `internal/exerciseprogression/timed_progression.go`:

```go
package exerciseprogression

import (
	"fmt"
	"math"
)

// TimedConfig is provided once when starting a timed exercise execution.
type TimedConfig struct {
	StartingSeconds int // seconds; caller-derived from history, may be user-overridden
}

// TimedSetTarget is what the package recommends for the upcoming hold.
type TimedSetTarget struct {
	TargetSeconds int
}

// TimedSetResult is recorded by the caller after the user completes a hold.
type TimedSetResult struct {
	ActualSeconds int
	Signal        Signal
}

const (
	timedSnapSeconds   = 5
	timedFloorSeconds  = 5
	timedDecrFraction  = 0.10
	timedStepShort     = 5  // < 60s
	timedStepMid       = 10 // 60–119s
	timedStepLong      = 15 // ≥ 120s
	timedMidThreshold  = 60
	timedLongThreshold = 120
)

// TimedProgression manages duration progression for one timed exercise execution.
type TimedProgression struct {
	config    TimedConfig
	completed []TimedSetResult
}

// NewTimed creates a TimedProgression for a new exercise execution.
func NewTimed(c TimedConfig) *TimedProgression {
	return &TimedProgression{config: c, completed: nil}
}

// NewTimedFromHistory reconstructs a TimedProgression from sets already completed in this session.
func NewTimedFromHistory(c TimedConfig, completed []TimedSetResult) *TimedProgression {
	p := NewTimed(c)
	p.completed = make([]TimedSetResult, len(completed))
	copy(p.completed, completed)
	return p
}

// CurrentSet returns the recommended target for the next hold.
func (p *TimedProgression) CurrentSet() TimedSetTarget {
	if len(p.completed) == 0 {
		return TimedSetTarget{TargetSeconds: p.config.StartingSeconds}
	}
	last := p.completed[len(p.completed)-1]
	return TimedSetTarget{TargetSeconds: adjustedSeconds(last)}
}

// RecordCompletion records what actually happened and advances internal state.
func (p *TimedProgression) RecordCompletion(r TimedSetResult) {
	p.completed = append(p.completed, r)
}

// SetsCompleted returns the number of holds recorded so far.
func (p *TimedProgression) SetsCompleted() int {
	return len(p.completed)
}

func adjustedSeconds(last TimedSetResult) int {
	step := timedIncrement(last.ActualSeconds)
	switch last.Signal {
	case SignalTooLight:
		return snap5(last.ActualSeconds + step)
	case SignalTooHeavy:
		decrement := step
		if pct := snap5(int(math.Round(float64(last.ActualSeconds) * timedDecrFraction))); pct > decrement {
			decrement = pct
		}
		next := last.ActualSeconds - decrement
		if next < timedFloorSeconds {
			next = timedFloorSeconds
		}
		return snap5(next)
	case SignalOnTarget:
		return last.ActualSeconds
	case SignalUnknown:
		panic("exerciseprogression: TimedSetResult must not use SignalUnknown")
	}
	panic(fmt.Sprintf("exerciseprogression: unhandled Signal %d", last.Signal))
}

func timedIncrement(current int) int {
	switch {
	case current < timedMidThreshold:
		return timedStepShort
	case current < timedLongThreshold:
		return timedStepMid
	default:
		return timedStepLong
	}
}

func snap5(seconds int) int {
	return int(math.Round(float64(seconds)/float64(timedSnapSeconds))) * timedSnapSeconds
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/exerciseprogression/ -run TestTimedProgression -v`
Expected: PASS for all sub-tests.

- [ ] **Step 5: Run full package tests**

Run: `go test ./internal/exerciseprogression/...`
Expected: all existing rep-based tests still PASS; new timed tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/exerciseprogression/timed_progression.go internal/exerciseprogression/timed_progression_test.go
git commit -m "$(cat <<'EOF'
Add TimedProgression engine for duration auto-regulation

Mirrors the weighted Progression but bumps seconds with a step ladder
(5/10/15s under/at/over 60s/120s) and a 10% decrement floor on
too_heavy. Targets snapped to multiples of 5s and floored at 5s.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Schema collapse + premigration + Go layer rename

This is the dominant refactor: schema, premigration, domain model, repository, service, planner, handlers, templates, and tests all move from `min_reps`/`max_reps`/`completed_reps` to `target_value`/`completed_value` in lockstep so the build never breaks. Adds the `time_based` enum value and `default_starting_seconds` column on `exercises` so the next tasks have somewhere to land.

**Files:**
- Modify: `internal/sqlite/schema.sql`
- Create: `internal/sqlite/premigrate.go`
- Modify: `internal/sqlite/sqlite.go`
- Modify: `internal/sqlite/migrate_internal_test.go`
- Modify: `internal/workout/models.go`
- Modify: `internal/workout/repository-sessions.go`
- Modify: `internal/workout/repository-exercises.go`
- Modify: `internal/workout/service.go`
- Modify: `internal/workout/service_test.go`, `internal/workout/service_internal_test.go`
- Modify: `internal/weekplanner/weekplanner.go`
- Modify: `internal/weekplanner/weekplanner_internal_test.go`
- Modify: `cmd/web/handler-exerciseset.go`
- Modify: `cmd/web/handler-exerciseset_test.go`
- Modify: `cmd/web/handler-exercise-info.go`
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`
- Modify: `ui/templates/pages/workout/workout.gohtml`

This task is structured as phases. Each phase is a coherent change; commit only at the end after the full build + test passes.

### Phase 2A: Schema + premigration

- [ ] **Step 2A.1: Modify `internal/sqlite/schema.sql` — exercises and exercise_sets**

Replace the `exercises` table definition with:

```sql
CREATE TABLE exercises
(
    id                       INTEGER PRIMARY KEY,
    name                     TEXT NOT NULL UNIQUE CHECK (LENGTH(name) < 124),
    category                 TEXT NOT NULL CHECK (category IN ('full_body', 'upper', 'lower')),
    exercise_type            TEXT NOT NULL DEFAULT 'weighted'
                             CHECK (exercise_type IN ('weighted', 'bodyweight', 'assisted', 'time_based')),
    description_markdown     TEXT NOT NULL DEFAULT '' CHECK (LENGTH(description_markdown) < 20000),
    default_starting_seconds INTEGER CHECK (default_starting_seconds IS NULL OR default_starting_seconds > 0),
    CHECK (exercise_type <> 'time_based' OR default_starting_seconds IS NOT NULL)
) STRICT;
```

Replace the `exercise_sets` table with the collapsed shape:

```sql
CREATE TABLE exercise_sets
(
    workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
    set_number          INTEGER NOT NULL CHECK (set_number > 0),
    weight_kg           REAL,
    target_value        INTEGER NOT NULL CHECK (target_value > 0),
    completed_value     INTEGER CHECK (completed_value IS NULL OR completed_value >= 0),
    completed_at        TEXT CHECK (completed_at IS NULL OR
                                    STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
    signal              TEXT CHECK (signal IS NULL OR signal IN ('too_heavy', 'on_target', 'too_light')),

    PRIMARY KEY (workout_exercise_id, set_number)
) WITHOUT ROWID, STRICT;
```

- [ ] **Step 2A.2: Create premigration**

Create `internal/sqlite/premigrate.go`:

```go
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// preMigrateExerciseSetTarget rewrites the legacy exercise_sets shape
// (min_reps/max_reps/completed_reps) into the collapsed target_value/
// completed_value shape, dropping historical Plank rows whose reps would be
// nonsensical to reinterpret as seconds. Idempotent and safe on a fresh DB.
//
// Delete this file, its call site in NewDatabase, the test in
// migrate_internal_test.go, and the legacy schema constant once it has run
// in production.
func (db *Database) preMigrateExerciseSetTarget(ctx context.Context) error {
	// Already migrated? (target_value column exists)
	var hasTarget int
	err := db.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('exercise_sets') WHERE name = 'target_value'`,
	).Scan(&hasTarget)
	if err != nil {
		return fmt.Errorf("pragma_table_info exercise_sets: %w", err)
	}
	if hasTarget > 0 {
		return nil
	}

	// Fresh DB? (no exercise_sets table at all)
	var hasTable int
	err = db.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='exercise_sets'`,
	).Scan(&hasTable)
	if err != nil {
		return fmt.Errorf("sqlite_master exercise_sets: %w", err)
	}
	if hasTable == 0 {
		return nil
	}

	if _, err = db.ReadWrite.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	tx, err := db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				err = fmt.Errorf("%w; rollback: %w", err, rbErr)
			}
		}
	}()

	stmts := []string{
		`CREATE TABLE exercise_sets_new (
			workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
			set_number          INTEGER NOT NULL CHECK (set_number > 0),
			weight_kg           REAL,
			target_value        INTEGER NOT NULL CHECK (target_value > 0),
			completed_value     INTEGER CHECK (completed_value IS NULL OR completed_value >= 0),
			completed_at        TEXT CHECK (completed_at IS NULL OR
			                                STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
			signal              TEXT CHECK (signal IS NULL OR signal IN ('too_heavy', 'on_target', 'too_light')),
			PRIMARY KEY (workout_exercise_id, set_number)
		) WITHOUT ROWID, STRICT`,
		`INSERT INTO exercise_sets_new
			(workout_exercise_id, set_number, weight_kg, target_value,
			 completed_value, completed_at, signal)
		 SELECT s.workout_exercise_id, s.set_number, s.weight_kg,
		        s.max_reps, s.completed_reps, s.completed_at, s.signal
		   FROM exercise_sets s
		   JOIN workout_exercise wx ON wx.id = s.workout_exercise_id
		   JOIN exercises e         ON e.id = wx.exercise_id
		  WHERE e.name <> 'Plank'`,
		`DROP TABLE exercise_sets`,
		`ALTER TABLE exercise_sets_new RENAME TO exercise_sets`,
	}
	for _, q := range stmts {
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("premigrate exercise_sets: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
```

- [ ] **Step 2A.3: Wire premigration into `NewDatabase`**

In `internal/sqlite/sqlite.go`, between the `connect` call (line 51) and the `migrateTo` call (line 55), add:

```go
	if err = db.preMigrateExerciseSetTarget(ctx); err != nil {
		return nil, fmt.Errorf("preMigrateExerciseSetTarget: %w", err)
	}
```

So that block becomes:

```go
	if db, err = connect(ctx, url, logger); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err = db.preMigrateExerciseSetTarget(ctx); err != nil {
		return nil, fmt.Errorf("preMigrateExerciseSetTarget: %w", err)
	}

	if err = db.migrateTo(ctx, schemaDefinition); err != nil {
		return nil, fmt.Errorf("migrateTo: %w", err)
	}
```

- [ ] **Step 2A.4: Write premigration test**

In `internal/sqlite/migrate_internal_test.go`, add the legacy schema constant and the test. Find an open spot near other premigration tests if any exist; otherwise append:

```go
const legacyExerciseSetsSchema = `
CREATE TABLE workout_sessions (
	user_id            INTEGER NOT NULL,
	workout_date       TEXT    NOT NULL,
	difficulty_rating  INTEGER,
	started_at         TEXT,
	completed_at       TEXT,
	periodization_type TEXT NOT NULL DEFAULT 'strength',
	PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;

CREATE TABLE workout_exercise (
	id                  INTEGER PRIMARY KEY,
	workout_user_id     INTEGER NOT NULL,
	workout_date        TEXT    NOT NULL,
	exercise_id         INTEGER NOT NULL,
	warmup_completed_at TEXT
) STRICT;

CREATE TABLE exercises (
	id                   INTEGER PRIMARY KEY,
	name                 TEXT NOT NULL UNIQUE,
	category             TEXT NOT NULL,
	exercise_type        TEXT NOT NULL DEFAULT 'weighted',
	description_markdown TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE TABLE exercise_sets (
	workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
	set_number          INTEGER NOT NULL CHECK (set_number > 0),
	weight_kg           REAL,
	min_reps            INTEGER NOT NULL CHECK (min_reps > 0),
	max_reps            INTEGER NOT NULL CHECK (max_reps >= min_reps),
	completed_reps      INTEGER CHECK (completed_reps IS NULL OR completed_reps >= 0),
	completed_at        TEXT,
	signal              TEXT,
	PRIMARY KEY (workout_exercise_id, set_number)
) WITHOUT ROWID, STRICT;
`

func TestPreMigrateExerciseSetTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := connect(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ReadWrite.ExecContext(ctx, legacyExerciseSetsSchema); err != nil {
		t.Fatalf("apply legacy schema: %v", err)
	}
	seed := `
INSERT INTO exercises (id, name, category, exercise_type) VALUES
	(1, 'Bench Press', 'upper', 'weighted'),
	(2, 'Plank',       'upper', 'bodyweight');
INSERT INTO workout_sessions (user_id, workout_date, periodization_type) VALUES
	(1, '2026-04-01', 'hypertrophy');
INSERT INTO workout_exercise (id, workout_user_id, workout_date, exercise_id) VALUES
	(10, 1, '2026-04-01', 1),
	(11, 1, '2026-04-01', 2);
INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps, signal) VALUES
	(10, 1, 60.0, 6, 10, 8, 'on_target'),
	(11, 1, NULL, 5, 5, 5, 'on_target');
`
	if _, err := db.ReadWrite.ExecContext(ctx, seed); err != nil {
		t.Fatalf("seed legacy data: %v", err)
	}

	if err := db.preMigrateExerciseSetTarget(ctx); err != nil {
		t.Fatalf("preMigrate: %v", err)
	}

	// Bench row: target_value = max_reps = 10, completed_value = 8.
	var target, completed int
	if err := db.ReadWrite.QueryRowContext(ctx,
		`SELECT target_value, completed_value FROM exercise_sets WHERE workout_exercise_id = 10`,
	).Scan(&target, &completed); err != nil {
		t.Fatalf("read bench row: %v", err)
	}
	if target != 10 || completed != 8 {
		t.Errorf("bench row: target=%d completed=%d, want 10 and 8", target, completed)
	}

	// Plank row dropped.
	var plankCount int
	if err := db.ReadWrite.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM exercise_sets WHERE workout_exercise_id = 11`,
	).Scan(&plankCount); err != nil {
		t.Fatalf("count plank rows: %v", err)
	}
	if plankCount != 0 {
		t.Errorf("plank rows after migration = %d, want 0", plankCount)
	}

	// Idempotent: second call is a no-op and does not error.
	if err := db.preMigrateExerciseSetTarget(ctx); err != nil {
		t.Fatalf("preMigrate idempotent call: %v", err)
	}

	// Declarative migrator must accept the new schema as no-op for exercise_sets.
	if err := db.migrateTo(ctx, schemaDefinition); err != nil {
		t.Fatalf("migrateTo after premigration: %v", err)
	}
}
```

- [ ] **Step 2A.5: Run premigration test**

Run: `go test ./internal/sqlite/ -run TestPreMigrateExerciseSetTarget -v`
Expected: PASS.

### Phase 2B: Domain model + repository

- [ ] **Step 2B.1: Update `internal/workout/models.go`**

Add the `ExerciseTypeTime` constant and `IsTimed()` helper, add `DefaultStartingSeconds` to `Exercise`, and replace the rep fields on `Set` with `TargetValue`/`CompletedValue`.

```go
const (
    ExerciseTypeWeighted   ExerciseType = "weighted"
    ExerciseTypeBodyweight ExerciseType = "bodyweight"
    ExerciseTypeAssisted   ExerciseType = "assisted"
    ExerciseTypeTime       ExerciseType = "time_based"
)

type Exercise struct {
    ID                     int          `json:"id"`
    Name                   string       `json:"name"`
    Category               Category     `json:"category"`
    ExerciseType           ExerciseType `json:"exercise_type"`
    DescriptionMarkdown    string       `json:"description_markdown"`
    PrimaryMuscleGroups    []string     `json:"primary_muscle_groups"`
    SecondaryMuscleGroups  []string     `json:"secondary_muscle_groups"`
    DefaultStartingSeconds *int         `json:"default_starting_seconds,omitempty"`
}

func (e Exercise) IsTimed() bool { return e.ExerciseType == ExerciseTypeTime }

type Set struct {
    WeightKg       *float64
    TargetValue    int
    CompletedValue *int
    CompletedAt    *time.Time
    Signal         *Signal
}
```

Update `exerciseJSONSchema.MarshalJSON` to include `'time_based'` in the `exercise_type` enum and add the optional `default_starting_seconds` property.

- [ ] **Step 2B.2: Update `internal/workout/repository-sessions.go`**

Replace each occurrence of `min_reps`, `max_reps`, `completed_reps` in SQL strings with `target_value` and `completed_value` as appropriate. Update the row scanner near line 597 to scan `target_value` and `completed_value`. Update INSERT around line 722 to insert into the new columns. Update the `loadExerciseSets` SELECT around line 327. Update `ListSetsForExerciseSince`'s SELECT around line 473. Update `GetLatestStartingWeightBefore`'s WHERE at line 567 to use `completed_value IS NOT NULL`.

Also update the row struct around line 408–417 to scan into `set.TargetValue` and `set.CompletedValue` directly.

Concretely, the scanner block near line 597 becomes:

```go
if err := rows.Scan(&workoutDateStr, &set.WeightKg, &set.TargetValue,
    &set.CompletedValue, &completedAtStr, &warmupCompletedAtStr, &signalStr); err != nil {
    return "", Set{}, sql.NullString{}, fmt.Errorf("scan exercise set row: %w", err)
}
```

The INSERT around line 722 becomes:

```go
INSERT INTO exercise_sets (
    workout_exercise_id, set_number,
    weight_kg, target_value, completed_value, completed_at, signal
) VALUES (?, ?, ?, ?, ?, ?, ?)
```

And the parameterised values change to `set.WeightKg, set.TargetValue, set.CompletedValue, completedAtStr, signalValue`.

- [ ] **Step 2B.3: Update `internal/workout/repository-exercises.go` to load `default_starting_seconds`**

Add `default_starting_seconds` to every `SELECT` from `exercises`, scan into `*int`, and assign to the `Exercise` struct.

### Phase 2C: Service + planner

- [ ] **Step 2C.1: Update `internal/workout/service.go`**

Replace every `MinReps`/`MaxReps`/`CompletedReps` reference with `TargetValue`/`CompletedValue`. Specifically:

- Line ~198–200 (set construction in workout-generation path): `MinReps`/`MaxReps` → `TargetValue: planSet.TargetValue`
- Line ~361–408 (`UpdateCompletedReps`): rename method to `UpdateCompletedValue`, parameter `completedReps int` → `completedValue int`, field assignments use `CompletedValue`.
- Line ~531 (signal-conditional read): `set.CompletedReps != nil` → `set.CompletedValue != nil`.
- Line ~618–635 (history reconstruction for Progression): `set.CompletedReps` → `set.CompletedValue`; `ActualReps: *set.CompletedReps` → `ActualReps: *set.CompletedValue` (the rep-based Progression engine still calls it `ActualReps`; this is a unit alias on the rep-based path).
- Line ~882–904 (set-shape duplication for swap path): `MinReps`/`MaxReps` → `TargetValue`; `CompletedReps: nil` → `CompletedValue: nil`.
- Line ~967–1019 (default constants and placeholder generation): `defaultMinReps`/`defaultMaxReps` collapse into `defaultTargetValue = 8`; placeholder set construction uses `TargetValue: defaultTargetValue`.

- [ ] **Step 2C.2: Update `internal/workout/service_test.go` and `internal/workout/service_internal_test.go`**

Replace every `MinReps`, `MaxReps`, `CompletedReps` field reference with `TargetValue` and `CompletedValue`. For tests that use `MinReps: 8, MaxReps: 12`, set `TargetValue: 12` (matching the migration rule of taking the top of the range).

- [ ] **Step 2C.3: Update `internal/weekplanner/weekplanner.go`**

Replace `PlannedSet`'s `MinReps`/`MaxReps` (lines 141–142) with a single field:

```go
type PlannedSet struct {
    TargetValue int
}
```

Replace `setsForPeriodization` (line 296) — change signature from `(int, int)` to a single `int`:

```go
func setsForPeriodization(p PeriodizationType) int {
    if p == PeriodizationStrength {
        return repsStrength
    }
    return repsHypertrophy
}
```

(introducing `repsStrength = 5` and `repsHypertrophy = 8` constants if needed; pull from `internal/exerciseprogression/progression.go` if importable cleanly, otherwise duplicate the literals — they're a single line.)

Update line 424 to:

```go
sets[i] = PlannedSet{TargetValue: targetValue}
```

- [ ] **Step 2C.4: Update `internal/weekplanner/weekplanner_internal_test.go`**

Replace `s.MinReps != minRepsStrength || s.MaxReps != maxRepsStrength` (line 287) with a check against `TargetValue`. For example:

```go
if s.TargetValue != 5 {
    t.Errorf("expected TargetValue 5, got %d", s.TargetValue)
}
```

### Phase 2D: Handlers + templates

- [ ] **Step 2D.1: Update `cmd/web/handler-exerciseset.go`**

Replace `formatRepRange` (line 36) with `formatTarget`:

```go
func formatTarget(exercise workout.Exercise, session workout.Session, target int) string {
    if exercise.IsTimed() {
        return fmt.Sprintf("%ds", target)
    }
    if session.PeriodizationType == workout.PeriodizationHypertrophy {
        return "6-10"
    }
    return strconv.Itoa(target)
}
```

Update `setDisplay` (line ~26–33) to expose pre-formatted strings:

```go
type setDisplay struct {
    Set            workout.Set
    TargetStr      string
    CompletedStr   string
    Unit           string // "reps" or "seconds" — for input labels
    Number         int
    // ...preserve other existing fields...
}
```

Update `prepareSetsDisplay` to construct `TargetStr`/`CompletedStr`/`Unit` from the set + parent exercise + session.

Update the form-handling endpoint near line 296:

- Form field name: `completed_reps` → `completed_value`.
- Service call: `app.workoutService.UpdateCompletedReps(...)` → `app.workoutService.UpdateCompletedValue(...)`.

- [ ] **Step 2D.2: Update `cmd/web/handler-exerciseset_test.go`**

Replace the test SQL fixture `weight_kg, min_reps, max_reps` (line 806) with `weight_kg, target_value`. Update assertions on rep fields to assert on `TargetValue` / `CompletedValue`.

- [ ] **Step 2D.3: Update `cmd/web/handler-exercise-info.go`**

In `processEntryData` (line 108):

```go
case workout.ExerciseTypeBodyweight:
    setDescriptions = append(setDescriptions, fmt.Sprintf("%d reps", *set.CompletedValue))
```

(Also update the weighted/assisted branch at line 117 to use `*set.CompletedValue` instead of `reps`. Remove the `reps := *set.CompletedReps` line at line 112.)

The `time_based` case is added in Task 7 — leave a `default` panic for now or add the empty case to the switch (Go's exhaustive-enum check will require it):

```go
case workout.ExerciseTypeTime:
    // populated in Task 7
```

- [ ] **Step 2D.4: Update templates**

In `ui/templates/pages/exerciseset/sets-container.gohtml`: every `$set.CompletedReps` becomes `$set.CompletedValue`. Where the template renders "X reps" by reading the model directly, switch to reading the pre-formatted string from the `setDisplay` struct (e.g., `{{ .CompletedStr }}`).

In `ui/templates/pages/workout/workout.gohtml`: same — `.CompletedReps` → `.CompletedValue`.

Search for any remaining template references with: `grep -rn "CompletedReps\|MinReps\|MaxReps" ui/templates/`. There should be zero matches after the edits.

Also rename the form input name in the input element from `completed_reps` to `completed_value`.

### Phase 2E: Build, test, commit

- [ ] **Step 2E.1: Build the project**

Run: `go build ./...`
Expected: clean build with no errors.

- [ ] **Step 2E.2: Run lint with auto-fix**

Run: `make lint-fix`
Expected: clean exit. Address any remaining complaints (typically govet shadow can be fixed by reusing `err`).

- [ ] **Step 2E.3: Run tests**

Run: `make test`
Expected: PASS for all packages.

- [ ] **Step 2E.4: Commit**

```bash
git add internal/sqlite internal/workout internal/weekplanner internal/exerciseprogression cmd/web ui/templates
git commit -m "$(cat <<'EOF'
Collapse exercise_sets min/max reps into single target_value

Adds a one-shot premigration that rewrites legacy exercise_sets rows
into the new shape (target_value = max_reps, completed_value =
completed_reps), dropping historical Plank rows that would otherwise be
mis-rendered as seconds. Renames Set fields, planner emission,
repository SQL, handlers, and templates to match.

Also adds 'time_based' to the exercise_type enum and a
default_starting_seconds column on exercises so the next changes have
somewhere to land. Plank fixture itself is updated in a follow-up
commit.

Delete internal/sqlite/premigrate.go, the call site in NewDatabase, and
the test/legacy-schema constant in migrate_internal_test.go after this
ships to production.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Service `GetStartingSeconds` + repository accessor

Adds the cross-session derivation for time-based exercises. Mirrors `GetStartingWeight` shape.

**Files:**
- Modify: `internal/workout/repository-sessions.go`
- Modify: `internal/workout/service.go`
- Modify: `internal/workout/service_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/workout/service_test.go`, add:

```go
func Test_GetStartingSeconds(t *testing.T) {
    t.Parallel()

    ctx, svc, db := setupServiceTest(t) // existing helper used by Test_GetStartingWeight
    defer db.Close()

    today := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)

    // Insert a time_based exercise with default_starting_seconds = 30.
    if _, err := db.ReadWrite.ExecContext(ctx, `
        INSERT INTO exercises (id, name, category, exercise_type, default_starting_seconds)
        VALUES (99, 'Plank Test', 'upper', 'time_based', 30)
    `); err != nil {
        t.Fatalf("seed exercise: %v", err)
    }

    // No history → falls back to default.
    got, err := svc.GetStartingSeconds(ctx, 99, today)
    if err != nil {
        t.Fatalf("GetStartingSeconds no history: %v", err)
    }
    if got != 30 {
        t.Errorf("no history: got %d, want 30 (default)", got)
    }

    // Seed a successful prior session: completed 40s, on_target.
    seedTimedSet(t, db, ctx, /* userID */ 1, /* exerciseID */ 99,
        time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC), 40, "on_target")

    got, err = svc.GetStartingSeconds(ctx, 99, today)
    if err != nil {
        t.Fatalf("GetStartingSeconds with history: %v", err)
    }
    if got != 40 {
        t.Errorf("with history: got %d, want 40", got)
    }

    // Seed a too_heavy set today (after `today`'s date cutoff is exclusive,
    // earlier date with too_heavy should be skipped).
    seedTimedSet(t, db, ctx, 1, 99,
        time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC), 50, "too_heavy")

    got, err = svc.GetStartingSeconds(ctx, 99, today)
    if err != nil {
        t.Fatalf("GetStartingSeconds skipping too_heavy: %v", err)
    }
    if got != 40 {
        t.Errorf("too_heavy filter: got %d, want 40 (older successful set)", got)
    }
}

// seedTimedSet inserts a single completed time-based set for the given
// (user, exercise, date). Helper keeps the test body short.
func seedTimedSet(t *testing.T, db *sqlite.Database, ctx context.Context,
    userID, exerciseID int, date time.Time, completed int, signal string,
) {
    t.Helper()
    dateStr := date.Format("2006-01-02")
    if _, err := db.ReadWrite.ExecContext(ctx, `
        INSERT INTO workout_sessions (user_id, workout_date, periodization_type)
        VALUES (?, ?, 'strength')
        ON CONFLICT DO NOTHING
    `, userID, dateStr); err != nil {
        t.Fatalf("seed session: %v", err)
    }
    var weID int
    if err := db.ReadWrite.QueryRowContext(ctx, `
        INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
        VALUES (?, ?, ?) RETURNING id
    `, userID, dateStr, exerciseID).Scan(&weID); err != nil {
        t.Fatalf("seed workout_exercise: %v", err)
    }
    if _, err := db.ReadWrite.ExecContext(ctx, `
        INSERT INTO exercise_sets
            (workout_exercise_id, set_number, target_value, completed_value, completed_at, signal)
        VALUES (?, 1, ?, ?, '2026-05-05T12:00:00.000Z', ?)
    `, weID, completed, completed, signal); err != nil {
        t.Fatalf("seed exercise_set: %v", err)
    }
}
```

(If `setupServiceTest` doesn't exist as a helper today, follow the existing `Test_GetStartingWeight` setup pattern in `service_test.go` to wire `ctx`, `svc`, and `db`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/workout/ -run Test_GetStartingSeconds -v`
Expected: FAIL — `svc.GetStartingSeconds` undefined.

- [ ] **Step 3: Add repository accessor**

In `internal/workout/repository-sessions.go`, near `GetLatestStartingWeightBefore`:

```go
// GetLatestSuccessfulSecondsBefore returns the completed_value of the latest
// successful set for the given time-based exercise from a session strictly
// before beforeDate. A set is "successful" when completed_value IS NOT NULL
// and signal is 'on_target' or 'too_light'. Returns 0 when no successful
// history exists.
func (r *sqliteSessionRepository) GetLatestSuccessfulSecondsBefore(
    ctx context.Context,
    exerciseID int,
    beforeDate time.Time,
) (int, error) {
    userID := contexthelpers.AuthenticatedUserID(ctx)
    var seconds int
    err := r.db.ReadOnly.QueryRowContext(ctx, `
        SELECT es.completed_value
        FROM exercise_sets es
        JOIN workout_exercise we ON we.id = es.workout_exercise_id
        WHERE we.workout_user_id = ?
          AND we.exercise_id = ?
          AND we.workout_date < ?
          AND es.completed_value IS NOT NULL
          AND es.signal IN ('on_target', 'too_light')
        ORDER BY we.workout_date DESC, es.set_number DESC
        LIMIT 1`,
        userID, exerciseID, formatDate(beforeDate)).Scan(&seconds)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return 0, nil
        }
        return 0, fmt.Errorf("query latest successful seconds: %w", err)
    }
    return seconds, nil
}
```

Add the method to the `sessionRepository` interface (near the other methods).

- [ ] **Step 4: Add service method**

In `internal/workout/service.go`, near `GetStartingWeight`:

```go
// GetStartingSeconds returns the seconds target to seed a new session for
// the given time-based exercise. Pulls the latest successful set's
// completed_value from sessions strictly before beforeDate; falls back to
// exercise.DefaultStartingSeconds when no successful history exists. Returns
// an error if the exercise is not time_based or if the lookup fails.
func (s *Service) GetStartingSeconds(
    ctx context.Context,
    exerciseID int,
    beforeDate time.Time,
) (int, error) {
    exercise, err := s.repo.exercises.Get(ctx, exerciseID)
    if err != nil {
        return 0, fmt.Errorf("get exercise: %w", err)
    }
    if !exercise.IsTimed() {
        return 0, fmt.Errorf("exercise %d is not time_based", exerciseID)
    }
    seconds, err := s.repo.sessions.GetLatestSuccessfulSecondsBefore(ctx, exerciseID, beforeDate)
    if err != nil {
        return 0, fmt.Errorf("get latest successful seconds: %w", err)
    }
    if seconds > 0 {
        return seconds, nil
    }
    if exercise.DefaultStartingSeconds == nil {
        return 0, fmt.Errorf("time_based exercise %d has no default_starting_seconds", exerciseID)
    }
    return *exercise.DefaultStartingSeconds, nil
}
```

(If `s.repo.exercises.Get` doesn't exist with that exact signature, mirror the existing read pattern used elsewhere in the service.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/workout/ -run Test_GetStartingSeconds -v`
Expected: PASS.

- [ ] **Step 6: Run full package tests**

Run: `go test ./internal/workout/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/workout
git commit -m "$(cat <<'EOF'
Add GetStartingSeconds for time-based exercise progression

Mirrors GetStartingWeight: pulls the latest successful set's
completed_value from sessions strictly before the cutoff date, filtered
to signal IN ('on_target', 'too_light'). Falls back to
exercise.DefaultStartingSeconds when no qualifying history exists.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: BuildProgression `IsTimed` branch

Wires `TimedProgression` into the service so the rest of the app can ask for the current target regardless of unit. Mirrors the existing `BuildProgression` shape with a parallel `BuildTimedProgression`.

**Files:**
- Modify: `internal/workout/service.go`
- Modify: `internal/workout/service_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/workout/service_test.go`:

```go
func Test_BuildTimedProgression(t *testing.T) {
    t.Parallel()

    ctx, svc, db := setupServiceTest(t)
    defer db.Close()

    today := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)

    if _, err := db.ReadWrite.ExecContext(ctx, `
        INSERT INTO exercises (id, name, category, exercise_type, default_starting_seconds)
        VALUES (99, 'Plank Test', 'upper', 'time_based', 30);
        INSERT INTO workout_sessions (user_id, workout_date, periodization_type)
        VALUES (1, '2026-05-07', 'strength');
    `); err != nil {
        t.Fatalf("seed: %v", err)
    }

    progression, err := svc.BuildTimedProgression(ctx, today, 99)
    if err != nil {
        t.Fatalf("BuildTimedProgression: %v", err)
    }
    if got := progression.CurrentSet().TargetSeconds; got != 30 {
        t.Errorf("CurrentSet().TargetSeconds = %d, want 30 (default)", got)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/workout/ -run Test_BuildTimedProgression -v`
Expected: FAIL — `BuildTimedProgression` undefined.

- [ ] **Step 3: Implement the service method**

In `internal/workout/service.go`, near `BuildProgression`:

```go
// BuildTimedProgression constructs an exerciseprogression.TimedProgression
// for the given time-based exercise in the given session. Returns an error if
// the exercise is not time_based.
func (s *Service) BuildTimedProgression(
    ctx context.Context,
    date time.Time,
    exerciseID int,
) (*exerciseprogression.TimedProgression, error) {
    starting, err := s.GetStartingSeconds(ctx, exerciseID, date)
    if err != nil {
        return nil, fmt.Errorf("get starting seconds: %w", err)
    }

    sess, err := s.repo.sessions.Get(ctx, date)
    if err != nil {
        return nil, fmt.Errorf("get session: %w", err)
    }

    completed := make([]exerciseprogression.TimedSetResult, 0)
    for _, exSet := range sess.ExerciseSets {
        if exSet.Exercise.ID != exerciseID {
            continue
        }
        for _, set := range exSet.Sets {
            if set.CompletedValue == nil || set.Signal == nil {
                continue
            }
            completed = append(completed, exerciseprogression.TimedSetResult{
                ActualSeconds: *set.CompletedValue,
                Signal:        toProgressionSignal(*set.Signal),
            })
        }
    }
    return exerciseprogression.NewTimedFromHistory(
        exerciseprogression.TimedConfig{StartingSeconds: starting},
        completed,
    ), nil
}
```

(Reuse the existing `toProgressionSignal` helper if present; otherwise inline the same Signal mapping used by `BuildProgression`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/workout/ -run Test_BuildTimedProgression -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workout
git commit -m "$(cat <<'EOF'
Add BuildTimedProgression for time-based exercises

Parallels BuildProgression: derives starting seconds via
GetStartingSeconds, replays this session's completed time-based sets
into a TimedProgression so the next CurrentSet() call returns the
auto-regulated target.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Planner emits `time_based` sets

Updates the weekplanner so that when it picks a `time_based` exercise it emits `target_value` from the exercise's default starting seconds (the planner doesn't have user history; the service layer enriches per-execution targets via `BuildTimedProgression`).

**Files:**
- Modify: `internal/weekplanner/weekplanner.go`
- Modify: `internal/weekplanner/weekplanner_internal_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/weekplanner/weekplanner_internal_test.go`:

```go
func TestPlannerEmitsTimeBasedTarget(t *testing.T) {
    t.Parallel()

    plank := Exercise{
        ID:                     21,
        Name:                   "Plank",
        Category:               CategoryUpper,
        ExerciseType:           ExerciseTypeTime,
        DefaultStartingSeconds: ptrInt(30),
    }

    sets := buildPlannedSets(plank, PeriodizationStrength)
    if len(sets) != setsPerExercise {
        t.Fatalf("len(sets) = %d, want %d", len(sets), setsPerExercise)
    }
    for i, s := range sets {
        if s.TargetValue != 30 {
            t.Errorf("set %d: TargetValue = %d, want 30", i, s.TargetValue)
        }
    }
}

func ptrInt(i int) *int { return &i }
```

(`buildPlannedSets` is the helper that sits inside `weekplanner.go`'s set-emission code path — extract it if it isn't already a free function. Whatever the engineer renames the existing internal logic to, the test should target the same call site that today produces `[]PlannedSet`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/weekplanner/ -run TestPlannerEmitsTimeBasedTarget -v`
Expected: FAIL — either `Exercise.DefaultStartingSeconds` or the time-based branch missing.

- [ ] **Step 3: Add `Exercise.DefaultStartingSeconds` and `ExerciseTypeTime` in the weekplanner package**

The weekplanner has its own dependency-free `Exercise` struct
(`internal/weekplanner/weekplanner.go:111`) and `ExerciseType` enum
(`weekplanner.go:20-27`). Add `time_based` to the enum and the new field to
the struct:

```go
const (
    ExerciseTypeWeighted   ExerciseType = "weighted"
    ExerciseTypeBodyweight ExerciseType = "bodyweight"
    ExerciseTypeAssisted   ExerciseType = "assisted"
    ExerciseTypeTime       ExerciseType = "time_based"
)

type Exercise struct {
    ID                     int
    Category               Category
    ExerciseType           ExerciseType
    PrimaryMuscleGroups    []string
    SecondaryMuscleGroups  []string
    DefaultStartingSeconds *int
}
```

- [ ] **Step 3b: Copy the new field through the workout→weekplanner mapping**

In `internal/workout/service.go` around line 164 there is:

```go
wpExercises[i] = weekplanner.Exercise{
    ID:                    ex.ID,
    Category:              weekplanner.Category(ex.Category),
    ExerciseType:          weekplanner.ExerciseType(ex.ExerciseType),
    PrimaryMuscleGroups:   ex.PrimaryMuscleGroups,
    SecondaryMuscleGroups: ex.SecondaryMuscleGroups,
}
```

Add `DefaultStartingSeconds: ex.DefaultStartingSeconds,` so the planner sees
the time-based default at plan time.

- [ ] **Step 4: Branch the set-emission path on `ExerciseTypeTime`**

In the existing set-emission logic (around `weekplanner.go:424`):

```go
target := setsForPeriodization(periodization)
if exercise.ExerciseType == ExerciseTypeTime {
    if exercise.DefaultStartingSeconds == nil {
        // schema CHECK guarantees a non-nil default for time_based, so this
        // is a fixture/data invariant violation.
        return nil, fmt.Errorf("time_based exercise %d missing DefaultStartingSeconds", exercise.ID)
    }
    target = *exercise.DefaultStartingSeconds
}
for i := range sets {
    sets[i] = PlannedSet{TargetValue: target}
}
```

(If the surrounding function doesn't return error today, propagate the error through whichever signature the planner uses; the spec's data invariant is enforced by the schema CHECK so this branch is genuinely unreachable in production but worth a clear panic/error for fixture-development time.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/weekplanner/ -run TestPlannerEmitsTimeBasedTarget -v`
Expected: PASS.

- [ ] **Step 6: Run full package tests**

Run: `go test ./internal/weekplanner/...`
Expected: PASS — including all existing rep-based planner tests.

- [ ] **Step 7: Commit**

```bash
git add internal/weekplanner
git commit -m "$(cat <<'EOF'
Emit DefaultStartingSeconds as TargetValue for time_based exercises

Adds the time_based branch to the planner's set-emission so picking
plank produces the right per-set duration target. Rep-based exercises
keep using TargetReps unchanged.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Wire `BuildTimedProgression` into the workout-execution path

Updates the handler that hands the next-set target to the template. When the active exercise is `time_based`, call `BuildTimedProgression`; otherwise call `BuildProgression`. Updates `formatTarget` to receive both kinds of targets.

**Files:**
- Modify: `cmd/web/handler-exerciseset.go`

- [ ] **Step 1: Identify the call site**

Read `cmd/web/handler-exerciseset.go` and find where `BuildProgression` is called on the active exercise (it sits near where `CurrentSetTarget` gets populated — see the struct around line 33 from the earlier exploration).

- [ ] **Step 2: Branch on `IsTimed`**

Replace the single `BuildProgression` call with a branch:

```go
var targetValue int
if exercise.IsTimed() {
    progression, err := app.workoutService.BuildTimedProgression(r.Context(), date, exercise.ID)
    if err != nil {
        app.serverError(w, r, fmt.Errorf("build timed progression: %w", err))
        return
    }
    targetValue = progression.CurrentSet().TargetSeconds
} else {
    progression, err := app.workoutService.BuildProgression(r.Context(), date, exercise.ID)
    if err != nil {
        app.serverError(w, r, fmt.Errorf("build progression: %w", err))
        return
    }
    target := progression.CurrentSet()
    targetValue = target.TargetReps
    // existing weight-handling code remains for non-time_based exercises.
}

displayTarget := formatTarget(exercise, session, targetValue)
```

The existing `WeightKg`/`AbsCurrentWeight` template-data fields stay for the weighted/bodyweight/assisted branch and are not populated for `time_based`.

- [ ] **Step 3: Update template data struct**

Ensure the template data carries `displayTarget` (e.g., as `CurrentSetTargetStr string`). Templates should read this string instead of computing the range themselves.

- [ ] **Step 4: Build and run tests**

Run: `go build ./...` then `go test ./cmd/web/...`
Expected: clean build, all handler tests still PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/web internal/workout
git commit -m "$(cat <<'EOF'
Branch handler-exerciseset on IsTimed for next-set target

When the active exercise is time_based, the workout view asks
BuildTimedProgression for the current target seconds; otherwise it
keeps using BuildProgression for the rep-and-weight path. Display
string flows through formatTarget so templates render
unit-appropriately without per-template branching.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Exercise-info chart — `time_based` case

Adds the missing case in `processEntryData` so the progress chart renders "30s" for time-based history entries.

**Files:**
- Modify: `cmd/web/handler-exercise-info.go`

- [ ] **Step 1: Find the switch**

`cmd/web/handler-exercise-info.go` `processEntryData` (line ~108). After Task 2 it should already have a placeholder `case workout.ExerciseTypeTime:`.

- [ ] **Step 2: Implement the case**

```go
case workout.ExerciseTypeTime:
    if set.CompletedValue != nil {
        setDescriptions = append(setDescriptions, fmt.Sprintf("%ds", *set.CompletedValue))
    }
```

- [ ] **Step 3: Build and test**

Run: `go test ./cmd/web/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/web/handler-exercise-info.go
git commit -m "$(cat <<'EOF'
Render time_based sets as Xs in the exercise progress chart

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Flip plank fixture to `time_based`

Updates the seed so plank gets prescribed as a 30-second hold for new users. Existing plank rows in production were already dropped by the premigration in Task 2.

**Files:**
- Modify: `internal/sqlite/fixtures.sql`

- [ ] **Step 1: Locate plank in the fixture**

`internal/sqlite/fixtures.sql:329` contains the `(21, 'Plank', 'upper', 'bodyweight', '## Instructions...` row. The trailing `ON CONFLICT (name) DO UPDATE` clause at line 360 means the row will UPSERT on next boot.

- [ ] **Step 2: Modify the row**

Change `'bodyweight'` to `'time_based'`. Add `default_starting_seconds = 30`. The exact column list needs to include `default_starting_seconds`; if the existing INSERT statement uses positional VALUES, append the column to the column list and `30` to plank's row, leaving other rows with `NULL` — explicit:

```sql
INSERT INTO exercises (id, name, category, exercise_type, description_markdown, default_starting_seconds)
VALUES
  (1, 'Deadlift', 'full_body', 'weighted', '...', NULL),
  ...
  (21, 'Plank', 'upper', 'time_based', '## Instructions
...content unchanged...', 30),
  ...
ON CONFLICT (name) DO UPDATE SET
    category                 = excluded.category,
    exercise_type            = excluded.exercise_type,
    description_markdown     = excluded.description_markdown,
    default_starting_seconds = excluded.default_starting_seconds;
```

(Look at the existing `ON CONFLICT (name) DO UPDATE` block for the exercises insert — it already lists each column being updated. Append `default_starting_seconds = excluded.default_starting_seconds` to that list.)

- [ ] **Step 3: Run the database boot path**

Run: `go test ./internal/sqlite/...`
Expected: PASS — the schema CHECK rejects time_based exercises with NULL default_starting_seconds, so a typo here will fail loudly.

Then run the full test suite: `make test`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/sqlite/fixtures.sql
git commit -m "$(cat <<'EOF'
Flip plank fixture to time_based with 30s default

Plank now prescribes a 30-second hold instead of nonsense reps. The
ON CONFLICT clause re-applies on every boot so existing rows upgrade
in place.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Admin form — `time_based` option + `default_starting_seconds`

Adds the type to the admin dropdown and a number field shown only when type is `time_based`. Also wires the Service's exercise-create/update path to accept and persist `DefaultStartingSeconds`.

**Files:**
- Modify: `cmd/web/handler-admin-exercises.go`
- Modify: relevant admin template under `ui/templates/pages/admin/`
- Modify: `internal/workout/service.go` and `repository-exercises.go` if the create/update path needs the new field

- [ ] **Step 1: Find the admin handler**

`cmd/web/handler-admin-exercises.go:154` is the validation gate that today restricts `exercise_type` to known values. Read 50 lines around that point to see the form-handling shape.

- [ ] **Step 2: Add `time_based` to the validation**

```go
if exerciseType != workout.ExerciseTypeWeighted &&
    exerciseType != workout.ExerciseTypeBodyweight &&
    exerciseType != workout.ExerciseTypeAssisted &&
    exerciseType != workout.ExerciseTypeTime {
    // existing error response
}
```

- [ ] **Step 3: Parse `default_starting_seconds`**

When `exerciseType == workout.ExerciseTypeTime`, parse the form field `default_starting_seconds` as a positive integer and reject the form if it's missing or non-positive. For other types, leave it nil.

```go
var defaultStartingSeconds *int
if exerciseType == workout.ExerciseTypeTime {
    raw := r.PostForm.Get("default_starting_seconds")
    n, err := strconv.Atoi(raw)
    if err != nil || n <= 0 {
        http.Error(w, "default_starting_seconds must be a positive integer for time_based exercises", http.StatusBadRequest)
        return
    }
    defaultStartingSeconds = &n
}
```

- [ ] **Step 4: Pass the field through the service + repo**

Add the field to the create/update method signatures or the `Exercise` value used by them; ensure `INSERT`/`UPDATE` SQL in `repository-exercises.go` reads/writes `default_starting_seconds`.

- [ ] **Step 5: Update the admin template**

Find the admin exercise form template (`ui/templates/pages/admin/exercises*.gohtml`). Add `time_based` as a fourth option to the type dropdown. Add a number input with `name="default_starting_seconds"`, label "Default starting seconds", marked `required` when type is `time_based`. Use a small inline progressive-enhancement script or HTMX-style hide/show — match the existing pattern used for hiding the weight field on `bodyweight`.

- [ ] **Step 6: Run handler tests**

Run: `go test ./cmd/web/ -run Admin -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/web internal/workout ui/templates
git commit -m "$(cat <<'EOF'
Admin form support for time_based exercises

Adds time_based to the type dropdown and a default_starting_seconds
number field shown only when the type is time_based. Service and
repository persist the new field; schema CHECK enforces presence.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Manual smoke test

End-to-end verification in the running app that wasn't covered by unit tests.

- [ ] **Step 1: Boot the app fresh against an in-memory DB or a wiped local DB**

```bash
make init  # if not already done
go run ./cmd/web
```

- [ ] **Step 2: Create a test user, configure a workout day, and start a workout that includes plank**

Verify that the plank exercise card shows "Target: 30s" and a numeric input for completed seconds.

- [ ] **Step 3: Submit a completed set with 35 seconds and `Signal=too_light`**

Verify that the next set in the same session shows "Target: 40s" (35 + 5s ladder, rounded to nearest 5).

- [ ] **Step 4: Complete all three sets and finish the session**

- [ ] **Step 5: Generate the next plank session**

Verify it opens at the seconds completed on the final set (per `GetLatestSuccessfulSecondsBefore`).

- [ ] **Step 6: Visit the plank exercise info / progress chart page**

Verify each historical entry renders `"NNs"` instead of `"NN reps"`.

- [ ] **Step 7: Verify rep-based exercises are unchanged**

Visit a hypertrophy bench session — display reads "6-10". Visit a strength deadlift session — display reads "5".

If anything looks wrong, file the bug at the corresponding task and patch.
