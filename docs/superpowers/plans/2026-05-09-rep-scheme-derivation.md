# Rep-Scheme Derivation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace today's flat per-session rep target (5 reps for Strength weeks, 8 for Hypertrophy weeks, applied uniformly across all exercises) with a per-exercise rep window (`rep_min`, `rep_max`) where periodization biases to the low end (Strength) or high end (Hypertrophy). Sets and rest are also derived from the resulting rep target.

**Architecture:** Two new INTEGER columns on `exercises` (nullable for time-based; required for everything else, enforced via table-level CHECK). A pure `DeriveScheme(repMin, repMax, periodization) → {TargetReps, TargetSets, RestSeconds}` function in `internal/exerciseprogression`. Planner calls this when building each `PlannedExerciseSet`, replacing today's `setsForPeriodization` and `setsPerExercise` constant.

**Tech Stack:** Go, SQLite (STRICT mode, declarative migrator), table-driven tests.

**Reference spec:** `docs/superpowers/specs/2026-05-09-rep-scheme-derivation-design.md`

---

## File Structure

**Create:**
- `internal/exerciseprogression/scheme.go` — `Scheme` struct + `DeriveScheme` function
- `internal/exerciseprogression/scheme_test.go` — table-driven tests (external test package, mirrors `progression_test.go`)

**Modify:**
- `internal/sqlite/schema.sql` — add `rep_min`, `rep_max` columns + table CHECKs
- `internal/sqlite/fixtures.sql` — populate `rep_min`, `rep_max` for every exercise; NULL them on Plank's UPDATE
- `internal/workout/models.go` — add `RepMin`, `RepMax` to `Exercise`
- `internal/workout/repository-exercises.go` — extend SELECT and INSERT statements (lines 30, 64, 230, 236)
- `internal/workout/service_test.go` — extend three production-schema INSERTs (lines 374, 781, 932) to include `rep_min`, `rep_max`
- `internal/workout/service.go` — extend domain→weekplanner conversion (lines 162-172)
- `internal/weekplanner/weekplanner.go` — add `RepMin`, `RepMax` to `Exercise` and `RestSeconds` to `PlannedSet`; delete `setsForPeriodization` and `setsPerExercise`; add `timeBasedSets` constant; rewrite `buildPlannedExerciseSet` to call `DeriveScheme`
- `internal/weekplanner/weekplanner_internal_test.go` — update tests at lines 287, 295, 559, 563 (and any others that hard-code the old constants)

---

## Task 1: Implement DeriveScheme via TDD

This is the pure derivation function. Implement first because it has no dependencies and pins the contract everything else relies on.

**Files:**
- Create: `internal/exerciseprogression/scheme.go`
- Create: `internal/exerciseprogression/scheme_test.go`

- [ ] **Step 1.1: Write the failing tests**

Create `internal/exerciseprogression/scheme_test.go`:

```go
package exerciseprogression_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
)

func TestDeriveScheme(t *testing.T) {
	tests := []struct {
		name          string
		repMin        int
		repMax        int
		periodization exerciseprogression.PeriodizationType
		wantReps      int
		wantSets      int
		wantRest      int
	}{
		// Heavy spinal-load compound (3-6 window).
		{"deadlift strength", 3, 6, exerciseprogression.Strength, 3, 4, 180},
		{"deadlift hypertrophy", 3, 6, exerciseprogression.Hypertrophy, 6, 3, 150},

		// Non-spinal compound (5-10 window).
		{"bench strength", 5, 10, exerciseprogression.Strength, 5, 4, 180},
		{"bench hypertrophy", 5, 10, exerciseprogression.Hypertrophy, 10, 3, 150},

		// Lumbar-stress accessory (8-20 window).
		{"back ext strength", 8, 20, exerciseprogression.Strength, 8, 3, 150},
		{"back ext hypertrophy", 8, 20, exerciseprogression.Hypertrophy, 20, 3, 90},

		// Isolation, large muscle (8-12 window).
		{"bicep curl strength", 8, 12, exerciseprogression.Strength, 8, 3, 150},
		{"bicep curl hypertrophy", 8, 12, exerciseprogression.Hypertrophy, 12, 3, 90},

		// Isolation, small/slow muscle (10-20 window).
		{"calf strength", 10, 20, exerciseprogression.Strength, 10, 3, 150},
		{"calf hypertrophy", 10, 20, exerciseprogression.Hypertrophy, 20, 3, 90},

		// Bucket boundaries.
		{"reps=5 (top of low bucket)", 5, 5, exerciseprogression.Strength, 5, 4, 180},
		{"reps=6 (start of mid bucket)", 6, 6, exerciseprogression.Strength, 6, 3, 150},
		{"reps=10 (top of mid bucket)", 10, 10, exerciseprogression.Strength, 10, 3, 150},
		{"reps=11 (start of high bucket)", 11, 11, exerciseprogression.Strength, 11, 3, 90},

		// Single-value window: same output regardless of periodization.
		{"single 5 strength", 5, 5, exerciseprogression.Strength, 5, 4, 180},
		{"single 5 hypertrophy", 5, 5, exerciseprogression.Hypertrophy, 5, 4, 180},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exerciseprogression.DeriveScheme(tt.repMin, tt.repMax, tt.periodization)
			if got.TargetReps != tt.wantReps {
				t.Errorf("TargetReps: want %d, got %d", tt.wantReps, got.TargetReps)
			}
			if got.TargetSets != tt.wantSets {
				t.Errorf("TargetSets: want %d, got %d", tt.wantSets, got.TargetSets)
			}
			if got.RestSeconds != tt.wantRest {
				t.Errorf("RestSeconds: want %d, got %d", tt.wantRest, got.RestSeconds)
			}
		})
	}
}

func TestDeriveSchemePanicOnUnknownPeriodization(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown PeriodizationType")
		}
	}()
	_ = exerciseprogression.DeriveScheme(5, 10, exerciseprogression.PeriodizationType(99))
}
```

- [ ] **Step 1.2: Run tests to verify they fail with a clear "function not defined" error**

Run: `go test ./internal/exerciseprogression/... -run DeriveScheme -v`

Expected: build error mentioning `undefined: exerciseprogression.DeriveScheme` and `undefined: exerciseprogression.Scheme` (or similar).

- [ ] **Step 1.3: Write the implementation**

Create `internal/exerciseprogression/scheme.go`:

```go
package exerciseprogression

import "fmt"

// Scheme is the per-exercise prescription for one planned session: the rep
// target, set count, and inter-set rest. Computed from a per-exercise rep
// window (repMin, repMax) and the session's PeriodizationType.
type Scheme struct {
	TargetReps  int
	TargetSets  int
	RestSeconds int
}

// DeriveScheme returns the prescription for one exercise given its rep window
// and the session periodization. Pure: same inputs → same output, no DB, no
// clock.
//
// Reps:
//
//	Strength    → repMin (low end of the window)
//	Hypertrophy → repMax (high end of the window)
//
// Sets and rest are derived from the resulting rep target:
//
//	reps ≤ 5  → 4 sets, 180s rest  (heavy work, more sets, full ATP-PCr recovery)
//	reps 6-10 → 3 sets, 150s rest  (moderate; longer rest improves hypertrophy in trained lifters per Schoenfeld 2016)
//	reps ≥ 11 → 3 sets, 90s rest   (lighter; volume kept up, rest shortens)
//
// Endurance is defined in the package but not currently used by the planner;
// it maps to repMax to give the function exhaustive coverage and keep future
// extension safe.
func DeriveScheme(repMin, repMax int, p PeriodizationType) Scheme {
	var reps int
	switch p {
	case Strength:
		reps = repMin
	case Hypertrophy:
		reps = repMax
	case Endurance:
		reps = repMax
	default:
		panic(fmt.Sprintf("exerciseprogression: unknown PeriodizationType %d", p))
	}

	var sets, rest int
	switch {
	case reps <= 5:
		sets, rest = 4, 180
	case reps <= 10:
		sets, rest = 3, 150
	default:
		sets, rest = 3, 90
	}

	return Scheme{TargetReps: reps, TargetSets: sets, RestSeconds: rest}
}
```

- [ ] **Step 1.4: Run tests to verify they pass**

Run: `go test ./internal/exerciseprogression/... -run DeriveScheme -v`

Expected: all sub-tests PASS, including `TestDeriveSchemePanicOnUnknownPeriodization`.

- [ ] **Step 1.5: Commit**

```bash
git add internal/exerciseprogression/scheme.go internal/exerciseprogression/scheme_test.go
git commit -m "$(cat <<'EOF'
Add DeriveScheme: derive reps/sets/rest from a per-exercise window

Pure function: takes (repMin, repMax, PeriodizationType) and returns the
target reps (low or high end of the window), set count, and rest seconds.
Set/rest mapping calibrated to Schoenfeld 2016/2017 hypertrophy literature.

Spec: docs/superpowers/specs/2026-05-09-rep-scheme-derivation-design.md
EOF
)"
```

---

## Task 2: Add schema columns and propagate through persistence

Adds `rep_min`/`rep_max` to the schema with table-level CHECKs that require non-NULL values for non-time-based exercises, populates the seed for every exercise, plumbs the new fields through the repository and domain model, and patches the few production-schema test fixtures that would otherwise fail the new CHECK.

**Files:**
- Modify: `internal/sqlite/schema.sql:81-91`
- Modify: `internal/sqlite/fixtures.sql:25` (the main exercises INSERT) and `:368-371` (the Plank UPDATE)
- Modify: `internal/workout/models.go:46-55`
- Modify: `internal/workout/repository-exercises.go:30, 64, 230, 236`
- Modify: `internal/workout/service_test.go:374, 781, 932` (production-schema INSERTs)

- [ ] **Step 2.1: Add columns and CHECKs to schema.sql**

In `internal/sqlite/schema.sql`, replace the `exercises` table definition (lines 81-91) with:

```sql
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
```

- [ ] **Step 2.2: Update fixtures.sql exercise INSERT**

In `internal/sqlite/fixtures.sql`, change the column list on line 25 from:

```sql
INSERT INTO exercises (id, name, category, exercise_type, description_markdown)
```

to:

```sql
INSERT INTO exercises (id, name, category, exercise_type, description_markdown, rep_min, rep_max)
```

Add `, <rep_min>, <rep_max>` immediately before the closing `)` on every exercise VALUES row. Use these values (justified in the spec's Migration table):

| ID | Name | rep_min | rep_max |
|---|---|---|---|
| 1 | Deadlift | 3 | 6 |
| 2 | Bench Press | 5 | 10 |
| 3 | Tricep Pushdown | 8 | 12 |
| 4 | Dumbbell Biceps Curl | 8 | 12 |
| 5 | Lateral Raise | 10 | 20 |
| 6 | Dumbbell Shoulder Press | 5 | 10 |
| 7 | Dumbbell Bench Press | 5 | 10 |
| 8 | Cable Fly | 8 | 12 |
| 9 | Pulldown | 5 | 10 |
| 10 | Pulldown, Reverse Grip | 5 | 10 |
| 11 | Seated Cable Row | 5 | 10 |
| 12 | One-Arm Dumbell Row | 5 | 10 |
| 13 | Abdominal Machine Crunch | 8 | 15 |
| 14 | Leg Press | 5 | 10 |
| 15 | Leg Extension | 8 | 12 |
| 16 | Leg Curl | 8 | 12 |
| 17 | Calf Raise | 10 | 20 |
| 18 | Back Extension | 8 | 20 |
| 19 | Push-up | 5 | 10 |
| 20 | Ab Wheel Rollout | 8 | 15 |
| 21 | Plank | 8 | 15 |
| 24 | Assisted Pull-Up | 5 | 10 |

(Plank is inserted as `bodyweight` initially — the table CHECK requires non-NULL `rep_min`/`rep_max` for that. The next step then flips it to `time_based` and NULLs them out.)

For each row, the change looks like (example for ID 2):

```sql
       (2, 'Bench Press', 'upper', 'weighted', '## Instructions
…description…
'),
```

becomes:

```sql
       (2, 'Bench Press', 'upper', 'weighted', '## Instructions
…description…
', 5, 10),
```

Update the `ON CONFLICT(name) DO UPDATE SET` clause near line 361 to include the new columns. Replace:

```sql
UPDATE SET category = excluded.category,
    exercise_type = excluded.exercise_type,
    description_markdown = excluded.description_markdown;
```

with:

```sql
UPDATE SET category = excluded.category,
    exercise_type = excluded.exercise_type,
    description_markdown = excluded.description_markdown,
    rep_min = excluded.rep_min,
    rep_max = excluded.rep_max;
```

- [ ] **Step 2.3: Update Plank's post-INSERT UPDATE to NULL its rep window**

In `internal/sqlite/fixtures.sql`, lines 368-371 currently read:

```sql
UPDATE exercises
SET exercise_type            = 'time_based',
    default_starting_seconds = 30
WHERE name = 'Plank';
```

Change to:

```sql
UPDATE exercises
SET exercise_type            = 'time_based',
    default_starting_seconds = 30,
    rep_min                  = NULL,
    rep_max                  = NULL
WHERE name = 'Plank';
```

(Plank is time-based; per the spec, time-based exercises do not use rep-window derivation, so we NULL these to keep the DB tidy.)

- [ ] **Step 2.4: Add RepMin/RepMax to domain Exercise**

In `internal/workout/models.go`, replace the `Exercise` struct (lines 46-55):

```go
// Exercise represents a single exercise type, e.g. Squat, Bench Press, etc.
type Exercise struct {
	ID                     int          `json:"id"`
	Name                   string       `json:"name"`
	Category               Category     `json:"category"`
	ExerciseType           ExerciseType `json:"exercise_type"`
	DescriptionMarkdown    string       `json:"description_markdown"`
	PrimaryMuscleGroups    []string     `json:"primary_muscle_groups"`
	SecondaryMuscleGroups  []string     `json:"secondary_muscle_groups"`
	DefaultStartingSeconds *int         `json:"default_starting_seconds,omitempty"`
	RepMin                 *int         `json:"rep_min,omitempty"`
	RepMax                 *int         `json:"rep_max,omitempty"`
}
```

(`*int` matches the existing `DefaultStartingSeconds` nullable convention. NULL only for time-based exercises.)

- [ ] **Step 2.5: Update repository SELECT statements**

In `internal/workout/repository-exercises.go`, the `Get` method (line 25). Replace the SQL and the Scan call (lines 29-46):

```go
	var exercise Exercise
	var defaultStartingSeconds, repMin, repMax sql.NullInt64

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT id, name, category, exercise_type, description_markdown,
		       default_starting_seconds, rep_min, rep_max
		FROM exercises
		WHERE id = ?`, id).Scan(
		&exercise.ID,
		&exercise.Name,
		&exercise.Category,
		&exercise.ExerciseType,
		&exercise.DescriptionMarkdown,
		&defaultStartingSeconds,
		&repMin,
		&repMax,
	)
	if err != nil {
		return Exercise{}, fmt.Errorf("query exercise: %w", err)
	}
	if defaultStartingSeconds.Valid {
		v := int(defaultStartingSeconds.Int64)
		exercise.DefaultStartingSeconds = &v
	}
	if repMin.Valid {
		v := int(repMin.Int64)
		exercise.RepMin = &v
	}
	if repMax.Valid {
		v := int(repMax.Int64)
		exercise.RepMax = &v
	}
```

In the same file, the `List` method (line 61). Replace the SQL (lines 63-66) and scan/conversion block (lines 77-91):

```go
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, name, category, exercise_type, description_markdown,
		       default_starting_seconds, rep_min, rep_max
		FROM exercises
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query exercises: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var exercises []Exercise
	for rows.Next() {
		var exercise Exercise
		var defaultStartingSeconds, repMin, repMax sql.NullInt64
		if err = rows.Scan(
			&exercise.ID, &exercise.Name, &exercise.Category, &exercise.ExerciseType,
			&exercise.DescriptionMarkdown, &defaultStartingSeconds, &repMin, &repMax,
		); err != nil {
			return nil, fmt.Errorf("scan exercise: %w", err)
		}
		if defaultStartingSeconds.Valid {
			v := int(defaultStartingSeconds.Int64)
			exercise.DefaultStartingSeconds = &v
		}
		if repMin.Valid {
			v := int(repMin.Int64)
			exercise.RepMin = &v
		}
		if repMax.Valid {
			v := int(repMax.Int64)
			exercise.RepMax = &v
		}
		exercises = append(exercises, exercise)
	}
```

- [ ] **Step 2.6: Update repository INSERT statements**

In `internal/workout/repository-exercises.go`, replace the upsert + create blocks (lines 226-239):

```go
	// Insert or reinsert the exercise
	var result sql.Result
	if upsert {
		// When upserting, use the existing ID
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (id, name, category, exercise_type, description_markdown,
			                       default_starting_seconds, rep_min, rep_max)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			ex.ID, ex.Name, ex.Category, ex.ExerciseType, ex.DescriptionMarkdown,
			ex.DefaultStartingSeconds, ex.RepMin, ex.RepMax)
	} else {
		// When creating new, let SQLite assign the ID
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (name, category, exercise_type, description_markdown,
			                       default_starting_seconds, rep_min, rep_max)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ex.Name, ex.Category, ex.ExerciseType, ex.DescriptionMarkdown,
			ex.DefaultStartingSeconds, ex.RepMin, ex.RepMax)
	}
```

(`*int` arguments work directly with `database/sql` — they bind NULL when nil.)

- [ ] **Step 2.7: Update production-schema test fixtures**

`internal/workout/service_test.go` has three production-schema INSERTs that don't specify `exercise_type` (so it defaults to `'weighted'`) and would fail the new CHECK without `rep_min`/`rep_max`.

Replace line 373-375:

```go
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Test Exercise", "lower", "Test description", 5, 10)
```

Replace line 780-782 (similarly — locate the same pattern around line 781 and apply the same change):

```go
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Test Exercise", "lower", "Test description", 5, 10)
```

Replace line 931-933 (same pattern around line 932):

```go
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown, rep_min, rep_max) VALUES (?, ?, ?, ?, ?)",
		"Test Exercise", "lower", "Test description", 5, 10)
```

(Use `grep -n 'INSERT INTO exercises (name, category, description_markdown)' internal/workout/service_test.go` to locate exact lines if they have shifted.)

The two test fixtures that insert `time_based` rows (around lines 1183, 1284) are already exempted by the `exercise_type = 'time_based'` branch of the CHECK and need no change.

- [ ] **Step 2.8: Run all tests to verify the schema, fixtures, model, and repository agree**

Run: `make test`

Expected: all tests pass. If `migrate_internal_test` fails, double-check that schema.sql is exactly as written above. If `service_test` fails on an INSERT, you missed one of the three lines in step 2.7.

- [ ] **Step 2.9: Commit**

```bash
git add internal/sqlite/schema.sql internal/sqlite/fixtures.sql \
        internal/workout/models.go internal/workout/repository-exercises.go \
        internal/workout/service_test.go
git commit -m "$(cat <<'EOF'
Add rep_min/rep_max columns to exercises and propagate through persistence

Two new INTEGER columns on the exercises table, nullable for time_based
exercises, required for everything else (table-level CHECK). Fixtures seed
all 22 catalog exercises by family (heavy spinal compounds 3-6, non-spinal
compounds 5-10, lumbar accessories 8-20, isolation 8-12, small/slow 10-20,
core stability 8-15). Repository SELECT/INSERT and the domain Exercise
model both carry the new fields.

Spec: docs/superpowers/specs/2026-05-09-rep-scheme-derivation-design.md
EOF
)"
```

---

## Task 3: Wire derivation into the planner

Plumb `RepMin`/`RepMax` from the domain model through the planner's `Exercise` view, add `RestSeconds` to `PlannedSet`, replace the old constants and helper, and call `DeriveScheme` when building each `PlannedExerciseSet`. Update the existing tests that hard-code the old behavior.

**Files:**
- Modify: `internal/weekplanner/weekplanner.go:38-42` (constants), `:108-117` (Exercise), `:139-143` (PlannedSet), `:296-302` (delete setsForPeriodization), `:420-448` (planner integration)
- Modify: `internal/workout/service.go:162-172` (conversion)
- Modify: `internal/weekplanner/weekplanner_internal_test.go:282-296, 559-564` (and any other tests that use the deleted constants)

- [ ] **Step 3.1: Add RepMin/RepMax to weekplanner.Exercise and RestSeconds to PlannedSet**

In `internal/weekplanner/weekplanner.go`, replace the `Exercise` struct (lines 108-117):

```go
// Exercise is a dependency-free representation of an exercise for planning.
// StartingWeightKg is intentionally absent — resolved lazily by exerciseprogression.
// RepMin/RepMax are nil for time_based exercises (which use DefaultStartingSeconds);
// non-nil for everything else, enforced at the DB layer by a CHECK constraint.
type Exercise struct {
	ID                     int
	Category               Category
	ExerciseType           ExerciseType
	PrimaryMuscleGroups    []string
	SecondaryMuscleGroups  []string
	DefaultStartingSeconds *int
	RepMin                 *int
	RepMax                 *int
}
```

In the same file, replace `PlannedSet` (lines 139-143):

```go
// PlannedSet holds the target value and rest. WeightKg is always nil at plan time.
// TargetValue's unit (reps or seconds) is derived from the parent exercise type.
type PlannedSet struct {
	TargetValue int
	RestSeconds int
}
```

- [ ] **Step 3.2: Update domain→weekplanner conversion**

In `internal/workout/service.go`, replace the conversion block (lines 162-172):

```go
	wpExercises := make([]weekplanner.Exercise, len(exercises))
	for i, ex := range exercises {
		wpExercises[i] = weekplanner.Exercise{
			ID:                     ex.ID,
			Category:               weekplanner.Category(ex.Category),
			ExerciseType:           weekplanner.ExerciseType(ex.ExerciseType),
			PrimaryMuscleGroups:    ex.PrimaryMuscleGroups,
			SecondaryMuscleGroups:  ex.SecondaryMuscleGroups,
			DefaultStartingSeconds: ex.DefaultStartingSeconds,
			RepMin:                 ex.RepMin,
			RepMax:                 ex.RepMax,
		}
	}
```

- [ ] **Step 3.3: Replace constants in weekplanner.go**

In `internal/weekplanner/weekplanner.go`, replace the constant block at lines 38-42:

```go
const (
	// timeBasedSets is the fixed set count for time-based exercises (e.g.
	// planks). Rep-based exercises derive their set count via
	// exerciseprogression.DeriveScheme.
	timeBasedSets = 3
)
```

(`setsPerExercise`, `repsStrength`, and `repsHypertrophy` are deleted — `DeriveScheme` owns those numbers now.)

- [ ] **Step 3.4: Delete the setsForPeriodization helper**

In `internal/weekplanner/weekplanner.go`, delete the function at lines 296-302 (the `setsForPeriodization` definition and the docstring above it). It has one caller, which is rewritten in the next step.

- [ ] **Step 3.5: Rewrite planner integration to call DeriveScheme**

In `internal/weekplanner/weekplanner.go`, add `"github.com/myrjola/petrapp/internal/exerciseprogression"` to the existing import block (the new import is the only change to the import statement). The final block should read:

```go
import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
)
```

Replace lines 420-448 (the comment, the `repTarget := setsForPeriodization(pt)` line, the loop building `result`, and `buildPlannedExerciseSet`):

```go
	// Build PlannedExerciseSets. Time-based exercises use their own
	// DefaultStartingSeconds with a fixed set count; rep-based exercises
	// derive their full prescription from the per-exercise window via
	// exerciseprogression.DeriveScheme.
	result := make([]PlannedExerciseSet, len(selected))
	for i, ex := range selected {
		result[i] = buildPlannedExerciseSet(ex, pt)
	}
	return result
}

// buildPlannedExerciseSet creates a PlannedExerciseSet for one exercise.
// For time_based exercises, sets count is fixed at timeBasedSets and target
// is DefaultStartingSeconds. For rep-based exercises, all three (reps, sets,
// rest) come from DeriveScheme.
func buildPlannedExerciseSet(ex Exercise, pt PeriodizationType) PlannedExerciseSet {
	if ex.ExerciseType == ExerciseTypeTime {
		if ex.DefaultStartingSeconds == nil {
			panic(fmt.Sprintf("time_based exercise %d missing DefaultStartingSeconds (fixture invariant violation)", ex.ID))
		}
		sets := make([]PlannedSet, timeBasedSets)
		for j := range sets {
			sets[j] = PlannedSet{TargetValue: *ex.DefaultStartingSeconds, RestSeconds: 0}
		}
		return PlannedExerciseSet{ExerciseID: ex.ID, Sets: sets}
	}

	if ex.RepMin == nil || ex.RepMax == nil {
		panic(fmt.Sprintf("non-time_based exercise %d missing RepMin/RepMax (fixture invariant violation)", ex.ID))
	}
	scheme := exerciseprogression.DeriveScheme(*ex.RepMin, *ex.RepMax, exerciseprogression.PeriodizationType(pt))
	sets := make([]PlannedSet, scheme.TargetSets)
	for j := range sets {
		sets[j] = PlannedSet{TargetValue: scheme.TargetReps, RestSeconds: scheme.RestSeconds}
	}
	return PlannedExerciseSet{ExerciseID: ex.ID, Sets: sets}
}
```

(The cast `exerciseprogression.PeriodizationType(pt)` is safe: both enums are `int` with `Strength=0`, `Hypertrophy=1`. The planner only emits `PeriodizationStrength` and `PeriodizationHypertrophy`, so the cast lands on the matching values in the target package.)

- [ ] **Step 3.6: Update existing planner tests**

The test file builds its exercise pool via `minimalExercises()` (around line 146) plus inline `Exercise{…}` literals at lines 305, 353, 402, 476, 521. Every non-time-based test exercise needs `RepMin`/`RepMax` populated, otherwise the planner panics on the new "missing RepMin/RepMax" invariant check.

First, add a tiny pointer helper at the top of the file (right after the imports), if one isn't already present:

```go
func intPtr(v int) *int { return &v }
```

Then in `minimalExercises()` (lines 147-onwards), add `RepMin: intPtr(5), RepMax: intPtr(10)` to each weighted/bodyweight/assisted exercise literal. Example for ID 1:

```go
		{ID: 1, Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads", "Glutes"}, SecondaryMuscleGroups: nil,
			DefaultStartingSeconds: nil,
			RepMin: intPtr(5), RepMax: intPtr(10)},
```

Apply the same change to every inline `Exercise{…}` literal whose `ExerciseType` is not `ExerciseTypeTime`. The plank fixture at line 521 stays as-is — it's time-based.

Run this to find every literal so none are missed:

```bash
grep -n "ExerciseType: ExerciseType" internal/weekplanner/weekplanner_internal_test.go
```

For each weighted/bodyweight/assisted line, ensure the surrounding struct literal has `RepMin: intPtr(5), RepMax: intPtr(10)`.

Now update the two assertions that referenced the deleted constants.

Replace the "each exercise set has setsPerExercise sets" assertion (around line 287):

```go
		// With Strength + window 5-10, DeriveScheme returns 4 sets (reps=5 ≤ 5).
		expectedSets := exerciseprogression.DeriveScheme(5, 10, exerciseprogression.Strength).TargetSets
		if len(sets[0].Sets) != expectedSets {
			t.Errorf("want %d sets, got %d", expectedSets, len(sets[0].Sets))
		}
```

Replace the "strength set" assertion (around line 295):

```go
			expectedReps := exerciseprogression.DeriveScheme(5, 10, exerciseprogression.Strength).TargetReps
			if s.TargetValue != expectedReps {
				t.Errorf("strength set: want TargetValue=%d, got %d", expectedReps, s.TargetValue)
			}
```

Add `"github.com/myrjola/petrapp/internal/exerciseprogression"` to the existing import block in this test file.

For the time-based assertion at line 559 (`len(sets[0].Sets) != setsPerExercise`), replace `setsPerExercise` with `timeBasedSets` directly (the constant is in the same package so it's accessible from `_internal_test.go`):

```go
	if len(sets[0].Sets) != timeBasedSets {
		t.Fatalf("got %d sets, want %d", len(sets[0].Sets), timeBasedSets)
	}
```

Find any other test usages of the removed constants. Run:

```bash
grep -n "setsPerExercise\|repsStrength\|repsHypertrophy\|setsForPeriodization" internal/weekplanner/
```

Apply the same pattern to each — derived expectations for rep-based, `timeBasedSets` for time-based.

- [ ] **Step 3.7: Run all tests**

Run: `make test`

Expected: all tests pass. If a planner test fails with an unexpected set count, check that the test exercise has `RepMin`/`RepMax` populated. If it fails with a panic from `buildPlannedExerciseSet`, the test fixture's exercise is non-time-based but has nil `RepMin`/`RepMax` — fix the fixture.

- [ ] **Step 3.8: Commit**

```bash
git add internal/weekplanner/weekplanner.go \
        internal/weekplanner/weekplanner_internal_test.go \
        internal/workout/service.go
git commit -m "$(cat <<'EOF'
Wire DeriveScheme into the weekly planner

Replace the flat setsForPeriodization helper and the setsPerExercise=3
constant with a call to exerciseprogression.DeriveScheme that returns
target reps, set count, and rest seconds from each exercise's rep window
plus the session periodization. Time-based exercises keep a fixed set
count via the new timeBasedSets constant. PlannedSet gains RestSeconds.

Spec: docs/superpowers/specs/2026-05-09-rep-scheme-derivation-design.md
EOF
)"
```

---

## Task 4: Verify production has no orphan exercises

Before merging, confirm that every `exercises` row in production gets covered by the fixture's upsert (or a one-off backfill). A row that remains NULL on `rep_min`/`rep_max` after fixture and any backfill SQL apply will fail the new table CHECK on the next boot.

**Files:**
- Optionally create: `docs/2026-05-09-backfill-rep-windows.sql` (only if orphans exist)

- [ ] **Step 4.1: Snapshot production exercises via fly-ops**

Use the `fly-ops` skill (it knows how to wake the scaled-to-zero instance and run a read-only query against the production DB). Ask it to run:

```sql
SELECT id, name, exercise_type FROM exercises ORDER BY id;
```

- [ ] **Step 4.2: Diff against fixtures.sql**

Locally extract the exercise IDs that fixtures.sql defines:

```bash
grep -oE "^\s+\([0-9]+, '[^']*'" internal/sqlite/fixtures.sql | grep -oE "\([0-9]+" | tr -d '(' | sort -n
```

Compare against the production list. Any production ID that is not in this list is an orphan.

- [ ] **Step 4.3: If orphans exist, write a backfill SQL file**

Create `docs/2026-05-09-backfill-rep-windows.sql` with one UPDATE per orphan. Use the spec's family table to choose the right window. Example template:

```sql
-- One-shot backfill for production exercises absent from fixtures.sql.
-- Sets rep_min/rep_max so the new CHECK constraint on exercises is satisfied.
-- Apply once via fly-ops, then this file is historical.

UPDATE exercises SET rep_min = <min>, rep_max = <max> WHERE name = '<name>';
-- … one line per orphan …
```

Apply the file via fly-ops (the skill knows the safe-write workflow). After applying, re-run the snapshot query and confirm zero rows have NULL `rep_min` for non-time-based exercises:

```sql
SELECT id, name FROM exercises
WHERE exercise_type <> 'time_based'
  AND (rep_min IS NULL OR rep_max IS NULL);
```

Expected: zero rows.

- [ ] **Step 4.4: Commit the backfill SQL (if created)**

```bash
git add docs/2026-05-09-backfill-rep-windows.sql
git commit -m "Backfill rep_min/rep_max for production exercises absent from fixtures"
```

If no backfill was needed, skip this step.

---

## Task 5: Final verification

- [ ] **Step 5.1: Run the full CI suite**

Run: `make ci`

Expected: build, lint, test, sec all pass.

- [ ] **Step 5.2: Manual eyeball of generated weeks**

Bring up the dev environment (`make init` + run the binary if not already running), generate a week, and confirm at the planner level:

- Deadlift session: 3 reps × 4 sets at Strength weeks; 6 reps × 3 sets at Hypertrophy weeks. Rest 180s on Strength, 150s on Hypertrophy.
- Calf raise session: 10 reps × 3 sets at Strength; 20 reps × 3 sets at Hypertrophy. Rest 150s on Strength, 90s on Hypertrophy.
- Back extension session: 8 reps × 3 sets at Strength; 20 reps × 3 sets at Hypertrophy. Rest 150s, 90s.
- Bench press session: 5 reps × 4 sets at Strength; 10 reps × 3 sets at Hypertrophy. Rest 180s, 150s.

If the UI doesn't surface set counts or rest yet (it doesn't — see spec's "Out of scope"), this verification can be done by adding a small ad-hoc print in the planner test or by inspecting the in-memory `PlannedExerciseSet` via a test fixture. Document what you actually verified in the PR description.

- [ ] **Step 5.3: Open a PR**

Push the branch and open a PR. In the body, link the spec
(`docs/superpowers/specs/2026-05-09-rep-scheme-derivation-design.md`) and
this plan, and note explicitly:

- New columns are nullable; declarative migrator handles them — no premigration.
- Production was checked for orphans before merge (link the fly-ops session or paste the orphan-query result).
- RIR/RPE-driven prescription, rest UI, and per-exercise set overrides remain out of scope.
