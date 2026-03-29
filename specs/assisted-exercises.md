# Assisted Exercises with Negative Weight Support

## Overview

Assisted exercises (e.g. assisted pull-up, assisted dip) allow bodyweight reduction via a machine
or rubber band. The assistance amount is represented as a **negative `weight_kg`** value — for
example, `-20 kg` means 20 kg of assistance is applied.

The goal of progression is to reduce assistance over time:
`-20 kg → -17.5 kg → … → 0 kg → +2.5 kg`

Default starting weight for assisted exercises is **0 kg** (no assistance). Raw negative values are
shown in the UI.

---

## Implementation Plan

### PR 1: Database schema and domain model

**Files:** `internal/sqlite/schema.sql`, `internal/workout/models.go`

#### `internal/sqlite/schema.sql`

1. Add `'assisted'` to the `exercise_type` CHECK constraint in the `exercises` table:

   ```sql
   -- Before:
   exercise_type TEXT NOT NULL DEFAULT 'weighted' CHECK (exercise_type IN ('weighted', 'bodyweight'))
   -- After:
   exercise_type TEXT NOT NULL DEFAULT 'weighted' CHECK (exercise_type IN ('weighted', 'bodyweight', 'assisted'))
   ```

2. Remove the `weight_kg >= 0` constraint from `exercise_sets`:

   ```sql
   -- Before:
   weight_kg REAL CHECK (weight_kg IS NULL OR weight_kg >= 0)
   -- After:
   weight_kg REAL
   ```

   Application-level logic enforces that `weight_kg` is `NULL` only for bodyweight exercises and can
   be negative for assisted ones. The declarative migration system in `migrate.go` handles schema
   evolution automatically.

   The enforcement point today is `parseWeightAndReps()` in
   `cmd/web/handler-exerciseset.go` (currently guards weight parsing behind
   `exercise.ExerciseType == ExerciseTypeWeighted`) and the corresponding branch in
   `exerciseSetUpdatePOST` that only calls `UpdateSetWeight` for the weighted type. PR 3 extends
   both guards to also cover the `assisted` type — that is where the application rule is enforced.
   No additional validation function is required.

#### `internal/workout/models.go`

Add the new exercise type constant:

```go
const (
    ExerciseTypeWeighted   ExerciseType = "weighted"
    ExerciseTypeBodyweight ExerciseType = "bodyweight"
    ExerciseTypeAssisted   ExerciseType = "assisted" // new
)
```

Update the AI JSON schema description string to include the new type.

Search for all `switch` statements on `ExerciseType` and add an `ExerciseTypeAssisted` case to keep
the codebase exhaustive (at minimum `cmd/web/handler-exercise-info.go`).

#### Tests

- Update any test assertions that guard against `*set.WeightKg < 0` to exclude the `assisted` type
  (see `internal/workout/generator_internal_test.go`).

---

### PR 2: Progression system

**Files:** `internal/workout/generator.go`, `internal/workout/generator_internal_test.go`

#### Fix `reduceWeight()` for negative weights

The current implementation is wrong for negative weights because `math.Max(0, weight - reduction)`
moves an assisted weight *toward zero* (easier), not further negative (harder). There is also a
zero-boundary bug: when an assisted exercise starts at the default **0 kg** and the user fails,
a percentage-of-zero reduction is `0` — the weight would stay at 0 indefinitely regardless of the
sign check.

The fix adds an `exerciseType ExerciseType` parameter and uses `StandardWeightIncrementKg` as the
minimum reduction for the assisted path:

```go
// Before (broken for negatives):
reduction := *set.WeightKg * percentage
weightValue := math.Max(0, *set.WeightKg-reduction)

// After (correct for all signs, including assisted at 0 kg):
func reduceWeight(sets []Set, percentage float64, exerciseType ExerciseType) []Set {
    return transformSets(sets, func(set Set) Set {
        var newWeight *float64
        if set.WeightKg != nil {
            var weightValue float64
            if exerciseType == ExerciseTypeAssisted {
                // Go further negative (more assistance) on failure.
                // Use a minimum of StandardWeightIncrementKg so a 0 kg start can go negative.
                reduction := math.Max(StandardWeightIncrementKg, math.Abs(*set.WeightKg)*percentage)
                weightValue = *set.WeightKg - reduction
            } else {
                reduction := *set.WeightKg * percentage
                weightValue = math.Max(0, *set.WeightKg-reduction) // clamp to 0 for weighted
            }
            newWeight = &weightValue
        }
        return Set{
            WeightKg:      newWeight,
            MinReps:       set.MinReps,
            MaxReps:       set.MaxReps,
            CompletedReps: nil,
            CompletedAt:   nil,
        }
    })
}
```

All call sites that pass `reduceWeight` must be updated to supply the exercise type.

#### Add `ExerciseTypeAssisted` case to `createDefaultSets()`

Assisted exercises follow the same weighted progression logic but start at **0 kg**:

```go
case ExerciseTypeAssisted:
    weight := 0.0
    return defaultSetsWithWeight(&weight)
```

#### Verify `increaseWeight()` is safe for negative weights

`increaseWeight` in `generator.go` contains no clamping — it simply adds the increment to the
current weight:

```go
newWeight = &[]float64{*set.WeightKg + increment}[0]
```

For an assisted exercise at `-20 kg`, `increaseWeight(..., StandardWeightIncrementKg)` correctly
produces `-17.5 kg` (less assistance = harder). No change needed; this is confirmed safe.

#### Tests to add

| Test | Description |
|------|-------------|
| `TestAssistedExerciseDefaultSets` | Default sets have `WeightKg = 0.0` |
| `TestReduceWeightForNegativeWeights` | `reduceWeight` moves further negative: `-20 → -22` and `0 → -2.5` (zero-boundary case) |
| `TestAssistedExerciseProgression` | Full progression: `0 → 2.5`, and `-20 → -17.5 → … → 0 → 2.5` |

---

### PR 3: HTTP handler

**File:** `cmd/web/handler-exerciseset.go`

#### `parseWeightAndReps()`

Extend the condition that parses weight to include `assisted` type:

```go
// Before:
if exercise.ExerciseType == workout.ExerciseTypeWeighted {

// After:
if exercise.ExerciseType == workout.ExerciseTypeWeighted ||
    exercise.ExerciseType == workout.ExerciseTypeAssisted {
```

`strconv.ParseFloat` already handles negative strings (`"-20.5" → -20.5`), so no parsing change is
needed.

#### Weight update in `exerciseSetUpdatePOST`

Apply the same condition extension in the section that writes `weight_kg` back to the database.

#### Tests to add

- Add a `parseWeightAndReps` test case for an assisted exercise submitting `weight="-20"` and
  verify it parses to `-20.0`.
- Add an `exerciseSetUpdatePOST` integration test case: POST an assisted exercise set with
  `weight="-20"` and `reps="8"`, verify the set's `WeightKg` is stored as `-20.0` and
  `CompletedReps` as `8` (symmetry with the existing weighted handler test).

---

### PR 4: UI template

**File:** `ui/templates/pages/exerciseset/exerciseset.gohtml`

#### Show weight input for assisted exercises

```html
<!-- Before: -->
{{ if eq $.ExerciseSet.Exercise.ExerciseType "weighted" }}
<!-- After: -->
{{ if or (eq $.ExerciseSet.Exercise.ExerciseType "weighted") (eq $.ExerciseSet.Exercise.ExerciseType "assisted") }}
```

Apply this change in every location that gates the weight `<input>` on exercise type (both the
display section and the form section).

#### Allow minus sign in the weight input pattern

```html
<!-- Before: -->
pattern="[0-9,\.]*"
<!-- After: -->
pattern="-?[0-9]*[,\.]?[0-9]*"
```

The tighter pattern allows an optional leading `-`, an optional decimal separator (`,` or `.`),
and digits on either side — preventing malformed inputs like `-.` or `-,` that the naive
`-?[0-9,\.]*` would accept.

No label change needed — raw negative values are shown as-is.

---

### PR 5: Seed data

**File:** `internal/sqlite/fixtures.sql`

Add at least one assisted exercise so the feature is usable immediately:

```sql
INSERT INTO exercises (name, category, exercise_type, description_markdown) VALUES
  ('Assisted Pull-Up', 'upper', 'assisted',
   'Use a band or machine for assistance. Enter a **negative** weight to indicate the amount of assistance (e.g. `-20` for 20 kg of assistance). Aim to reduce assistance over time.');
```

Add corresponding muscle group rows in `exercise_muscle_groups`:
- Primary: Lats, Biceps
- Secondary: Upper Back, Forearms

---

## Key Design Decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Default starting weight | 0 kg | No assumed assistance level; user adjusts as needed |
| UI representation | Raw negative value (e.g. `-20`) | Simpler, no internal sign flip |
| Weight floor | None | Assistance can go arbitrarily negative if needed |
| `reduceWeight` direction | Further negative (more assistance) on failure | Consistent with "harder = higher absolute value" for weighted |
| `increaseWeight` direction | Less negative (less assistance) on success | Existing `increaseWeight` adds a fixed increment with no clamping — already correct for negatives, no change needed |
| Schema migration | Remove `weight_kg >= 0` constraint entirely | Declarative migration system handles this; application enforces NULL-only-for-bodyweight |
