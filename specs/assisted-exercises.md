# Assisted Exercises with Negative Weight Support

## Overview

Some exercises (assisted pull-ups, assisted dips) reduce bodyweight via a band or machine; some
of those same exercises (weighted pull-ups with a belt) add load. We want a single exercise type
that supports both, modeled as a signed `weight_kg`:

- Negative `weight_kg` = assistance (e.g. `-20 kg` = 20 kg of help).
- Zero = pure bodyweight.
- Positive = added external load.

The signal-driven progression in `internal/exerciseprogression/` already moves a weight in the
right direction for `SignalTooLight` and `SignalOnTarget` regardless of sign; only the
`SignalTooHeavy` branch is wrong for negatives. The form needs a UI affordance so users on iOS
Safari (no `−` key on the numeric keypad) can flag a value as assistance without typing a minus.

---

## Key design decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Storage | Single signed `weight_kg` column, drop `>= 0` constraint | One column, one progression code path |
| Sign on submit | "Assisted" checkbox negates the input | iOS numeric keypad has no `−` key; checkbox also reads more clearly than a minus sign |
| Display | Raw signed value (`-20 kg`, `10 kg`) via existing `formatFloat` | No formatter changes needed |
| Default starting weight | 0 kg | Same as any new exercise without history; works for either direction |
| `adjustedWeight` math | Single sign-aware formula for `SignalTooHeavy` | Works uniformly for negative, zero, and positive weights |
| Cross-periodization conversion | Skip Epley for non-positive weights (already does) | No meaningful 1RM analog for assistance |
| `exerciseprogression` package awareness | Stays exercise-type-agnostic | Sign-aware formula removes the need to thread an `IsAssisted` flag |

---

## Implementation plan

### PR 1: Database & domain

**Files:** `internal/sqlite/schema.sql`, `internal/workout/models.go`,
`internal/weekplanner/weekplanner.go`

#### `internal/sqlite/schema.sql`

1. Add `'assisted'` to the `exercise_type` CHECK constraint on `exercises`:

   ```sql
   exercise_type TEXT NOT NULL DEFAULT 'weighted'
       CHECK (exercise_type IN ('weighted', 'bodyweight', 'assisted'))
   ```

2. Drop the `weight_kg >= 0` constraint on `exercise_sets`:

   ```sql
   weight_kg REAL
   ```

   The declarative migrator in `migrate.go` recreates the table; no premigration is required.
   Application logic still enforces `weight_kg IS NULL` only for bodyweight.

#### `internal/workout/models.go`

Add the new constant:

```go
const (
    ExerciseTypeWeighted   ExerciseType = "weighted"
    ExerciseTypeBodyweight ExerciseType = "bodyweight"
    ExerciseTypeAssisted   ExerciseType = "assisted"
)
```

Search for `switch` statements on `ExerciseType` and add an `ExerciseTypeAssisted` case to keep
exhaustive checks current.

#### `internal/weekplanner/weekplanner.go`

Mirror the constant. Treat `ExerciseTypeAssisted` exactly like `ExerciseTypeWeighted` for
selection — it occupies a "loadable" slot and contributes to the same volume planning.

---

### PR 2: Progression fix

**Files:** `internal/exerciseprogression/progression.go`,
`internal/exerciseprogression/progression_test.go`,
`internal/exerciseprogression/conversion.go`

#### `adjustedWeight()` — single sign-aware formula

Replace the `SignalTooHeavy` branch only:

```go
case SignalTooHeavy:
    decrement := math.Max(weightIncrementKg, math.Abs(last.WeightKg)*weightDecrementFactor)
    return roundToHalf(last.WeightKg - decrement)
```

`SignalTooLight` (`weight + 2.5`) and `SignalOnTarget` (`weight`) are unchanged — both already
behave correctly across sign.

Behavior summary:

| Last weight | Old `SignalTooHeavy` | New `SignalTooHeavy` | Note |
|-------------|----------------------|----------------------|------|
| 100         | 90                   | 90                   | Same |
| 50          | 45                   | 45                   | Same |
| 10          | 9                    | 7.5                  | Behavior change at low positive weights — full 2.5 kg drop instead of 1 kg. Intentional. |
| 0           | 0 (stuck)            | -2.5                 | Zero-boundary fixed |
| -20         | -18 (wrong)          | -22.5                | More assistance on failure |

#### `conversion.go`

Add a comment to `ConvertWeight` noting that the existing `weight <= 0` early return
intentionally skips Epley conversion for assisted exercises (no meaningful 1RM analog). No code
change.

#### Tests

| Test | Description |
|------|-------------|
| `TestAdjustedWeight_NegativeTooHeavy` | `-20 → -22.5` |
| `TestAdjustedWeight_ZeroTooHeavy` | `0 → -2.5` |
| `TestAdjustedWeight_NegativeTooLight` | `-20 → -17.5` |
| `TestAdjustedWeight_PositiveTooHeavyRegression` | `100 → 90`, `50 → 45` unchanged |
| `TestAdjustedWeight_LowPositiveTooHeavy` | `10 → 7.5` (document intentional change) |

---

### PR 3: HTTP handler

**Files:** `cmd/web/handler-exerciseset.go`, `cmd/web/handler-admin-exercises.go`

#### `cmd/web/handler-exerciseset.go`

Two gates currently keyed on `== ExerciseTypeWeighted` extend to also match
`ExerciseTypeAssisted`:

- Line ~113 (`exerciseSetGET`): the gate around `BuildProgression` — assisted needs the
  per-set recommendation too.
- Line ~264 (`exerciseSetUpdatePOST`): the gate that routes to the weight+signal write path
  (currently `recordWeightedSetCompletion`, renamed below).

The `parseWeightAndReps` gate at line ~174 does **not** need to change — that helper is only
called from the `else` branch of `exerciseSetUpdatePOST` (the bodyweight path), and the
internal `== ExerciseTypeWeighted` check is dead in that call site. Leave it alone.

Rename `recordWeightedSetCompletion` → `recordSetCompletionWithWeight`. The function now serves
both weighted and assisted, so the old name is misleading.

Inside `recordSetCompletionWithWeight`, after parsing the positive weight value, negate when the
checkbox is set on an assisted exercise:

```go
if exercise.ExerciseType == workout.ExerciseTypeAssisted &&
    r.PostForm.Get("assisted") != "" {
    weight = -weight
}
```

For weighted exercises the checkbox is not rendered, so the field is absent and the negation
is a no-op even if the gate is generic.

The handler also needs to expose `AbsCurrentWeight float64` on `exerciseSetTemplateData` for the
form's `value` attribute (so the template doesn't need a math function). Computed as
`math.Abs(currentSetTarget.WeightKg)`.

#### `cmd/web/handler-admin-exercises.go`

Extend the validator at line ~148:

```go
if exerciseType != workout.ExerciseTypeWeighted &&
    exerciseType != workout.ExerciseTypeBodyweight &&
    exerciseType != workout.ExerciseTypeAssisted {
    app.serverError(w, r, errors.New("invalid exercise type"))
    return
}
```

#### Tests

- `parseWeightAndReps` for an assisted exercise with `weight="20"` → returns `+20.0` (handler
  layer is responsible for the sign flip).
- e2e POST: assisted set with `weight="20"`, `assisted="on"`, `reps="8"`,
  `signal="on_target"` → stored with `WeightKg=-20.0`, signal recorded, completion
  timestamp set.
- e2e POST: assisted set with `weight="5"`, no `assisted` field → stored with `WeightKg=+5.0`
  (mixed-direction progression: weighted day on an assisted exercise).

---

### PR 4: Templates & form

**Files:** `ui/templates/pages/exerciseset/sets-container.gohtml`,
`cmd/web/handler-exercise-info.go`,
`ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml`

#### `ui/templates/pages/exerciseset/sets-container.gohtml`

Both gates that currently read `eq $.ExerciseSet.Exercise.ExerciseType "weighted"` (display
section ~line 266 and form section ~line 294) become:

```gotemplate
{{ if or
    (eq $.ExerciseSet.Exercise.ExerciseType "weighted")
    (eq $.ExerciseSet.Exercise.ExerciseType "assisted") }}
```

In the form section, when the exercise type is assisted, render an additional checkbox next to
the weight input:

```html
<div class="input-field">
    <label>
        <input type="checkbox" name="assisted" {{ if lt $.CurrentSetTarget.WeightKg 0.0 }}checked{{ end }}>
        Assisted (band/machine)
    </label>
    <details class="assisted-help">
        <summary>What's this?</summary>
        <p>Check this when you used a band or machine to make the exercise easier.
           Leave unchecked if you added weight (e.g. with a belt).</p>
    </details>
</div>
```

The weight input's `value` attribute uses `AbsCurrentWeight` (handler-prepared) so the field
always shows a positive number. Pattern stays `[0-9,\.]*` — no minus needed:

```html
<input
    id="weight-{{ $index }}"
    inputmode="decimal"
    pattern="[0-9,\.]*"
    name="weight"
    value="{{ formatFloat $.AbsCurrentWeight }}"
    step="0.5"
    required
>
```

The display section uses raw `formatFloat $set.WeightKg` — `-20 kg` reads correctly.

`<details>` is the native progressive-enhancement disclosure widget — works without JS, no
script needed.

#### `cmd/web/handler-exercise-info.go`

In `processEntryData` add:

```go
case workout.ExerciseTypeAssisted:
    // Same metric as weighted — max signed weight as Y-axis.
    // -20 → -10 → 0 → +5 charts as continuous progress.
    if set.WeightKg != nil {
        weight := *set.WeightKg
        if weight > progress {
            progress = weight
        }
        setDescriptions = append(setDescriptions, fmt.Sprintf("%dx%.1fkg", reps, weight))
    }
```

(Identical to the weighted branch — could be combined into a fall-through case.)

#### `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml`

Add `<option value="assisted">Assisted</option>` to the exercise type select.

---

### PR 5: Seed data & AI generator

**Files:** `internal/sqlite/fixtures.sql`, `internal/workout/generator-exercise.go`

#### `internal/sqlite/fixtures.sql`

Add an Assisted Pull-Up row. Use the next free id (22) and the existing `ON CONFLICT (id) DO
UPDATE` upsert pattern:

```sql
(22, 'Assisted Pull-Up', 'upper', 'assisted', '## Instructions
1. ... pull-up form guidance ...

## Common Mistakes
- ...

## Resources
- ...')
```

Description should explain the checkbox: "Check the *Assisted* box when using a band or machine
for help. Leave it unchecked if you add weight with a belt as you progress."

`exercise_muscle_groups` rows:
- Primary: Lats, Biceps
- Secondary: Upper Back, Forearms

#### `internal/workout/generator-exercise.go`

Two updates to the AI generator:

1. In `exerciseJSONSchema.MarshalJSON`, extend the `exercise_type` enum:

   ```go
   "exercise_type": map[string]any{
       "type":        "string",
       "description": "Type of exercise (weighted, bodyweight, or assisted)",
       "enum":        []string{"weighted", "bodyweight", "assisted"},
   },
   ```

2. In the prompt template inside `generateBaseExercise`:

   ```
   For "exercise_type", use one of: "weighted", "bodyweight", "assisted"
   ```

---

## End-to-end progression example

A user starting Assisted Pull-Up from scratch:

| Set | Recommended | User input | Checkbox | Stored | Signal | Next |
|-----|-------------|------------|----------|--------|--------|------|
| 1   | 0 kg        | 20         | ✓        | -20    | on_target | -20 |
| 2   | -20 kg (abs `20`, ✓) | 20 | ✓ | -20 | too_light | -17.5 |
| 3   | -17.5 kg    | 17.5       | ✓        | -17.5  | too_heavy | -20 |

Continued across sessions until the user can do bodyweight, then transitions:

| Session | Recommended | User input | Checkbox | Stored | Signal | Next |
|---------|-------------|------------|----------|--------|--------|------|
| N       | -2.5 kg     | 2.5        | ✓        | -2.5   | too_light | 0 |
| N+1     | 0 kg        | 0          | (either) | 0      | too_light | 2.5 |
| N+2     | 2.5 kg      | 2.5        | ✗        | 2.5    | on_target | 2.5 |

`adjustedWeight` produces this continuum without any special-case code.
