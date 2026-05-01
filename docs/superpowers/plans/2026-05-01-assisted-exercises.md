# Assisted Exercises Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an `assisted` exercise type with signed `weight_kg` storage, where negative values represent assistance (band/machine help) and positive values represent added load (e.g. weight belt). UI surfaces an "Assisted" checkbox to flip sign at submit time, working around iOS Safari numeric keypads that lack a `−` key.

**Architecture:** Single signed `weight_kg` column drives both directions; `internal/exerciseprogression/adjustedWeight()` gets a sign-aware decrement formula so progression flows continuously across the zero boundary. The handler reads an `assisted` checkbox to negate the form value before persisting.

**Tech Stack:** Go 1.23, SQLite (STRICT mode, declarative migrations), `internal/exerciseprogression` package, Go templates with inline scoped CSS.

**Design spec:** `specs/assisted-exercises.md`

---

### Task 1: Schema — allow 'assisted' type and signed weight_kg

**Files:**
- Modify: `internal/sqlite/schema.sql`

- [ ] **Step 1: Add 'assisted' to exercise_type enum**

In `internal/sqlite/schema.sql`, change line 86 from:

```sql
    exercise_type        TEXT NOT NULL DEFAULT 'weighted' CHECK (exercise_type IN ('weighted', 'bodyweight')),
```

to:

```sql
    exercise_type        TEXT NOT NULL DEFAULT 'weighted' CHECK (exercise_type IN ('weighted', 'bodyweight', 'assisted')),
```

- [ ] **Step 2: Drop weight_kg >= 0 constraint**

In the same file, change line 125 from:

```sql
    weight_kg           REAL CHECK (weight_kg IS NULL OR weight_kg >= 0),
```

to:

```sql
    weight_kg           REAL,
```

Application logic enforces `weight_kg IS NULL` only for bodyweight exercises; the table-level constraint is gone.

- [ ] **Step 3: Verify migration applies cleanly**

Run: `make test`
Expected: all tests pass. The declarative migrator detects the changed CHECK constraints and rebuilds both tables automatically — no premigration needed.

- [ ] **Step 4: Commit**

```bash
git add internal/sqlite/schema.sql
git commit -m "feat(schema): allow 'assisted' exercise_type and signed weight_kg"
```

---

### Task 2: Domain — add ExerciseTypeAssisted constants

**Files:**
- Modify: `internal/workout/models.go:21-24`
- Modify: `internal/weekplanner/weekplanner.go:23-26`

- [ ] **Step 1: Add constant in workout package**

In `internal/workout/models.go`, change the `ExerciseType` const block (lines 21-24) to:

```go
const (
    ExerciseTypeWeighted   ExerciseType = "weighted"
    ExerciseTypeBodyweight ExerciseType = "bodyweight"
    ExerciseTypeAssisted   ExerciseType = "assisted"
)
```

- [ ] **Step 2: Add constant in weekplanner package**

In `internal/weekplanner/weekplanner.go`, change the `ExerciseType` const block (lines 23-26) to:

```go
const (
    ExerciseTypeWeighted   ExerciseType = "weighted"
    ExerciseTypeBodyweight ExerciseType = "bodyweight"
    ExerciseTypeAssisted   ExerciseType = "assisted"
)
```

The weekplanner treats assisted exactly like weighted in selection logic — it occupies a "loadable" slot. No further weekplanner changes are needed.

- [ ] **Step 3: Verify build**

Run: `make build`
Expected: clean build, no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/workout/models.go internal/weekplanner/weekplanner.go
git commit -m "feat(domain): add ExerciseTypeAssisted constant"
```

---

### Task 3: Progression — fix adjustedWeight for negative/zero weights

**Files:**
- Modify: `internal/exerciseprogression/progression.go:112-129`
- Modify: `internal/exerciseprogression/progression_test.go`
- Modify: `internal/exerciseprogression/conversion.go:1-9`

- [ ] **Step 1: Write failing tests**

Append to `internal/exerciseprogression/progression_test.go`:

```go
func TestAdjustedWeight_AssistedAndZeroBoundary(t *testing.T) {
    tests := []struct {
        name       string
        lastWeight float64
        signal     exerciseprogression.Signal
        wantWeight float64
    }{
        {
            name:       "negative TooHeavy goes further negative (more assistance)",
            lastWeight: -20.0,
            signal:     exerciseprogression.SignalTooHeavy,
            wantWeight: -22.5,
        },
        {
            name:       "zero TooHeavy goes negative (zero boundary fixed)",
            lastWeight: 0.0,
            signal:     exerciseprogression.SignalTooHeavy,
            wantWeight: -2.5,
        },
        {
            name:       "negative TooLight goes less negative (less assistance)",
            lastWeight: -20.0,
            signal:     exerciseprogression.SignalTooLight,
            wantWeight: -17.5,
        },
        {
            name:       "negative OnTarget holds steady",
            lastWeight: -20.0,
            signal:     exerciseprogression.SignalOnTarget,
            wantWeight: -20.0,
        },
        {
            name:       "low positive TooHeavy uses 2.5kg minimum decrement",
            lastWeight: 10.0,
            signal:     exerciseprogression.SignalTooHeavy,
            wantWeight: 7.5,
        },
        {
            name:       "high positive TooHeavy uses percentage (regression)",
            lastWeight: 100.0,
            signal:     exerciseprogression.SignalTooHeavy,
            wantWeight: 90.0,
        },
        {
            name:       "mid positive TooHeavy uses percentage (regression)",
            lastWeight: 50.0,
            signal:     exerciseprogression.SignalTooHeavy,
            wantWeight: 45.0,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            p := exerciseprogression.NewFromHistory(
                exerciseprogression.Config{
                    Type:           exerciseprogression.Strength,
                    StartingWeight: 0,
                },
                []exerciseprogression.SetResult{
                    {ActualReps: 5, Signal: tt.signal, WeightKg: tt.lastWeight},
                },
            )
            got := p.CurrentSet().WeightKg
            if got != tt.wantWeight {
                t.Errorf("WeightKg = %v, want %v", got, tt.wantWeight)
            }
        })
    }
}
```

- [ ] **Step 2: Verify tests fail**

Run: `go test -v ./internal/exerciseprogression -run TestAdjustedWeight_AssistedAndZeroBoundary`

Expected: FAIL on:
- `negative TooHeavy goes further negative` — current code yields `-18` (wrong direction)
- `zero TooHeavy goes negative` — current code yields `0` (stuck)
- `low positive TooHeavy uses 2.5kg minimum decrement` — current code yields `9`

The four other subtests should already PASS (existing TooLight, OnTarget, and high-positive TooHeavy behavior is correct).

- [ ] **Step 3: Implement the sign-aware fix**

In `internal/exerciseprogression/progression.go`, replace the `SignalTooHeavy` branch in `adjustedWeight` (around lines 116-118). Change from:

```go
    case SignalTooHeavy:
        decreased := last.WeightKg * (1 - weightDecrementFactor)
        return roundToHalf(decreased)
```

to:

```go
    case SignalTooHeavy:
        decrement := math.Max(weightIncrementKg, math.Abs(last.WeightKg)*weightDecrementFactor)
        return roundToHalf(last.WeightKg - decrement)
```

- [ ] **Step 4: Update conversion.go comment**

In `internal/exerciseprogression/conversion.go`, replace the comment block on `ConvertWeight` (lines 3-8). Change from:

```go
// ConvertWeight translates a load chosen for fromReps into an equivalent load
// for toReps at the same estimated one-rep max, rounded to the nearest 0.5 kg.
// It uses the Epley formula: 1RM = w * (1 + r/30).
//
// Returns weight unchanged when fromReps == toReps, or when any input is
// non-positive.
```

to:

```go
// ConvertWeight translates a load chosen for fromReps into an equivalent load
// for toReps at the same estimated one-rep max, rounded to the nearest 0.5 kg.
// It uses the Epley formula: 1RM = w * (1 + r/30).
//
// Returns weight unchanged when fromReps == toReps, or when any input is
// non-positive. Non-positive weights cover assisted exercises (negative
// weight_kg representing assistance), where there is no meaningful 1RM
// analog — the assistance value is preserved verbatim across periodizations.
```

- [ ] **Step 5: Verify all tests pass**

Run: `go test -v ./internal/exerciseprogression`
Expected: all tests PASS — the new subtests and all existing regression suites.

- [ ] **Step 6: Commit**

```bash
git add internal/exerciseprogression/progression.go internal/exerciseprogression/progression_test.go internal/exerciseprogression/conversion.go
git commit -m "feat(exerciseprogression): sign-aware decrement for assisted/zero weights"
```

---

### Task 4: Handler — extend gates and rename helper

**Files:**
- Modify: `cmd/web/handler-exerciseset.go`

- [ ] **Step 1: Extend BuildProgression gate in exerciseSetGET**

In `cmd/web/handler-exerciseset.go`, change line 113 from:

```go
    if exerciseSet.Exercise.ExerciseType == workout.ExerciseTypeWeighted {
```

to:

```go
    if exerciseSet.Exercise.ExerciseType == workout.ExerciseTypeWeighted ||
        exerciseSet.Exercise.ExerciseType == workout.ExerciseTypeAssisted {
```

- [ ] **Step 2: Rename recordWeightedSetCompletion**

In the same file, rename the function `recordWeightedSetCompletion` (declared at line 203) to `recordSetCompletionWithWeight`. Use Edit with `replace_all=true` on the identifier so both the declaration and the call site (line 265) are updated together.

Verify only the new name remains:

Run: `grep -n "recordWeightedSetCompletion\|recordSetCompletionWithWeight" cmd/web/handler-exerciseset.go`
Expected: only `recordSetCompletionWithWeight` references — one declaration, one call.

- [ ] **Step 3: Extend dispatch gate in exerciseSetUpdatePOST**

In the same file, change the dispatch block at line 264-267 from:

```go
    if exercise.ExerciseType == workout.ExerciseTypeWeighted {
        if !app.recordSetCompletionWithWeight(w, r, date, workoutExerciseID, setIndex, dateStr) {
            return
        }
    } else {
```

to:

```go
    if exercise.ExerciseType == workout.ExerciseTypeWeighted ||
        exercise.ExerciseType == workout.ExerciseTypeAssisted {
        if !app.recordSetCompletionWithWeight(w, r, date, workoutExerciseID, setIndex, dateStr) {
            return
        }
    } else {
```

The `parseWeightAndReps` helper at line ~174 stays as-is — its internal `== ExerciseTypeWeighted` check is dead code in the bodyweight call path and doesn't need to change.

- [ ] **Step 4: Verify existing tests still pass**

Run: `make test`
Expected: all tests PASS — no behavior change yet for existing types since no assisted exercise exists in fixtures.

- [ ] **Step 5: Commit**

```bash
git add cmd/web/handler-exerciseset.go
git commit -m "refactor(handler): extend weight-path gates to include assisted, rename helper"
```

---

### Task 5: Handler — extend admin exercise type validator

**Files:**
- Modify: `cmd/web/handler-admin-exercises.go:148`

- [ ] **Step 1: Accept assisted in admin POST validator**

In `cmd/web/handler-admin-exercises.go`, change line 148 from:

```go
    if exerciseType != workout.ExerciseTypeWeighted && exerciseType != workout.ExerciseTypeBodyweight {
        app.serverError(w, r, errors.New("invalid exercise type"))
        return
    }
```

to:

```go
    if exerciseType != workout.ExerciseTypeWeighted &&
        exerciseType != workout.ExerciseTypeBodyweight &&
        exerciseType != workout.ExerciseTypeAssisted {
        app.serverError(w, r, errors.New("invalid exercise type"))
        return
    }
```

- [ ] **Step 2: Verify build and tests**

Run: `make test`
Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/web/handler-admin-exercises.go
git commit -m "feat(admin): accept 'assisted' as valid exercise type"
```

---

### Task 6: Seed fixture — Assisted Pull-Up

**Files:**
- Modify: `internal/sqlite/fixtures.sql`

- [ ] **Step 1: Add Assisted Pull-Up to exercises**

In `internal/sqlite/fixtures.sql`, find the `INSERT INTO exercises` block. The last existing row is id 21 (Plank, ending around line 342). Insert a new row immediately before the closing `ON CONFLICT(id) DO` clause. First, change the comma at the end of the Plank row from a closing `)` to `),` if it isn't already, then add:

```sql
       (22, 'Assisted Pull-Up', 'upper', 'assisted', '## Instructions
1. Set up the assistance: loop a resistance band over the pull-up bar and place one foot or knee in the loop, or use an assisted pull-up machine and select an assistance weight.
2. Grip the bar slightly wider than shoulder width with palms facing away.
3. Engage your lats and pull your chest toward the bar, keeping elbows tucked and shoulders down.
4. Lower yourself with control until your arms are fully extended.

## Common Mistakes
- **Swinging or kipping**: Use a controlled tempo throughout.
- **Half reps**: Lower all the way to a full hang to train the full range.
- **Shrugged shoulders**: Pull your shoulder blades down and back before each rep.

## Resources
- [Video tutorial](https://www.youtube.com/watch?v=eGo4IYlbE5g)
- [Form guide](https://www.verywellfit.com/how-to-do-the-assisted-pull-up-3498379)

## Tracking your progress
Check the **Assisted** box and enter the assistance amount as a positive number — the app stores it as negative weight. As you get stronger, reduce the assistance. Once you can do unassisted reps, leave the box unchecked. To progress further, add weight with a belt and continue with the box unchecked.')
```

The row above ends with a single `)` (no trailing comma) because it is the last row before the `ON CONFLICT` clause.

- [ ] **Step 2: Add muscle group rows**

In the same file, find the `INSERT INTO exercise_muscle_groups` block. The current last entries are for exercise 21 (Plank, lines 420-422). Append before the closing `ON CONFLICT(exercise_id, muscle_group_name) DO`:

```sql
-- Assisted exercises
       (22, 'Lats', 1),
       (22, 'Biceps', 1),
       (22, 'Upper Back', 0),
       (22, 'Forearms', 0)
```

Make sure the existing last row (`(21, 'Glutes', 0)`) ends with `,` not `)`.

- [ ] **Step 3: Verify fixtures load and migrations apply**

Run: `make test`
Expected: all PASS — the test database loads fixtures, and the new exercise round-trips with `exercise_type = 'assisted'`.

- [ ] **Step 4: Commit**

```bash
git add internal/sqlite/fixtures.sql
git commit -m "feat(fixtures): add Assisted Pull-Up exercise"
```

---

### Task 7: Template — admin exercise edit option

**Files:**
- Modify: `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml:23-29`

- [ ] **Step 1: Add 'assisted' option to the select**

In `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml`, change the exercise type select block (lines 23-29) from:

```gohtml
                <select id="exercise_type" name="exercise_type" required>
                    <option value="weighted" {{ if eq .Exercise.ExerciseType "weighted" }}selected{{ end }}>Weighted
                    </option>
                    <option value="bodyweight" {{ if eq .Exercise.ExerciseType "bodyweight" }}selected{{ end }}>
                        Bodyweight
                    </option>
                </select>
```

to:

```gohtml
                <select id="exercise_type" name="exercise_type" required>
                    <option value="weighted" {{ if eq .Exercise.ExerciseType "weighted" }}selected{{ end }}>Weighted
                    </option>
                    <option value="bodyweight" {{ if eq .Exercise.ExerciseType "bodyweight" }}selected{{ end }}>
                        Bodyweight
                    </option>
                    <option value="assisted" {{ if eq .Exercise.ExerciseType "assisted" }}selected{{ end }}>
                        Assisted
                    </option>
                </select>
```

- [ ] **Step 2: Verify rendering by visiting the admin edit page**

Templates load from disk — no rebuild needed. Run the dev server briefly to confirm the dropdown shows three options. (Optional manual check; existing template tests still pass with `make test`.)

Run: `make test`
Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml
git commit -m "feat(admin-ui): add 'assisted' option to exercise type select"
```

---

### Task 8: Chart — handle assisted exercises in progress chart

**Files:**
- Modify: `cmd/web/handler-exercise-info.go:111-140`

- [ ] **Step 1: Add ExerciseTypeAssisted case to processEntryData**

In `cmd/web/handler-exercise-info.go`, in the `processEntryData` function, find the `switch typ` block (around lines 118-133). Add the assisted case:

```go
        switch typ {
        case workout.ExerciseTypeWeighted, workout.ExerciseTypeAssisted:
            if set.WeightKg != nil {
                weight := *set.WeightKg
                if weight > progress {
                    progress = weight
                }
                setDescriptions = append(setDescriptions, fmt.Sprintf("%dx%.1fkg", reps, weight))
            }
        case workout.ExerciseTypeBodyweight:
            if float64(reps) > progress {
                progress = float64(reps)
            }
            setDescriptions = append(setDescriptions, fmt.Sprintf("%d reps", reps))
        }
```

(This combines the existing weighted branch with a new assisted alternative — the metric is still the max signed weight, so `-20 → -10 → 0 → +5` charts as continuous progress.)

- [ ] **Step 2: Verify existing tests pass**

Run: `make test`
Expected: all PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/web/handler-exercise-info.go
git commit -m "feat(chart): handle assisted exercises like weighted in progress chart"
```

---

### Task 9: Handler + template — surface AbsCurrentWeight and Assisted checkbox

**Files:**
- Modify: `cmd/web/handler-exerciseset.go` (template data + GET handler)
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`

- [ ] **Step 1: Add AbsCurrentWeight field to template data**

In `cmd/web/handler-exerciseset.go`, find the `exerciseSetTemplateData` struct (around lines 22-32) and add a new field:

```go
type exerciseSetTemplateData struct {
    BaseTemplateData
    Date                 time.Time
    ExerciseSet          workout.ExerciseSet
    SetsDisplay          []setDisplay
    FirstIncompleteIndex int
    EditingIndex         int
    IsEditing            bool
    LastCompletedAt      *time.Time
    CurrentSetTarget     exerciseprogression.SetTarget
    AbsCurrentWeight     float64 // |CurrentSetTarget.WeightKg|, for assisted form input
}
```

- [ ] **Step 2: Populate AbsCurrentWeight in exerciseSetGET**

In the same file, in `exerciseSetGET`, after `currentSetTarget` is computed (around line 120), add:

```go
    absCurrentWeight := math.Abs(currentSetTarget.WeightKg)
```

Then in the `data := exerciseSetTemplateData{...}` literal (around line 122), add the new field:

```go
    data := exerciseSetTemplateData{
        BaseTemplateData:     newBaseTemplateData(r),
        Date:                 date,
        ExerciseSet:          exerciseSet,
        SetsDisplay:          prepareSetsDisplay(exerciseSet.Sets),
        FirstIncompleteIndex: getFirstIncompleteIndex(exerciseSet.Sets),
        EditingIndex:         editingIndex,
        IsEditing:            isEditing,
        LastCompletedAt:      getLastCompletedAt(exerciseSet.Sets),
        CurrentSetTarget:     currentSetTarget,
        AbsCurrentWeight:     absCurrentWeight,
    }
```

You'll also need to add `"math"` to the import block at the top of the file if not already present.

- [ ] **Step 3: Verify build**

Run: `make build`
Expected: clean build.

- [ ] **Step 4: Extend display gate in sets-container.gohtml**

In `ui/templates/pages/exerciseset/sets-container.gohtml`, change line 266 from:

```gohtml
                    {{ if eq $.ExerciseSet.Exercise.ExerciseType "weighted" }}
```

to:

```gohtml
                    {{ if or (eq $.ExerciseSet.Exercise.ExerciseType "weighted") (eq $.ExerciseSet.Exercise.ExerciseType "assisted") }}
```

The display body inside this branch (showing weight + reps + signal badge) does not need any changes — `formatFloat` handles negative numbers correctly, so `-20 kg` renders as-is.

- [ ] **Step 5: Extend form gate and add Assisted checkbox**

In the same file, change line 294 from:

```gohtml
                    {{ if eq $.ExerciseSet.Exercise.ExerciseType "weighted" }}
```

to:

```gohtml
                    {{ if or (eq $.ExerciseSet.Exercise.ExerciseType "weighted") (eq $.ExerciseSet.Exercise.ExerciseType "assisted") }}
```

Then change the weight input's `value` attribute (around line 307) from:

```gohtml
                                        value="{{ formatFloat $.CurrentSetTarget.WeightKg }}"
```

to:

```gohtml
                                        value="{{ formatFloat $.AbsCurrentWeight }}"
```

(The input is always a positive number; the sign is controlled by the checkbox below.)

Then, immediately after the closing `</div>` of the weight `input-field` (the `<div id="weight-help-{{ $index }}" class="sr-only">...</div>` followed by `</div>` around line 313), insert a new `input-field` block — but only render it when the exercise is assisted:

```gohtml
                            {{ if eq $.ExerciseSet.Exercise.ExerciseType "assisted" }}
                            <div class="input-field assisted-field">
                                <label>
                                    <input type="checkbox" name="assisted"
                                           {{ if lt $.CurrentSetTarget.WeightKg 0.0 }}checked{{ end }}>
                                    Assisted (band/machine)
                                </label>
                                <details>
                                    <summary>What's this?</summary>
                                    <p>Check this when you used a band or machine to make the exercise easier.
                                       Leave it unchecked if you added weight (e.g. with a belt).</p>
                                </details>
                            </div>
                            {{ end }}
```

- [ ] **Step 6: Add scoped styles for the assisted-field**

The existing `<style {{ nonce }}>` block in `sets-container.gohtml` already scopes rules under `:scope` for the sets container. Add these rules inside the `.exercise-set` selector block (anywhere alongside the other `.input-field` rules — search for `.input-field {` around line 114):

```css
                    .assisted-field {
                        flex-direction: row;
                        align-items: center;
                        gap: var(--size-2);

                        label {
                            display: flex;
                            align-items: center;
                            gap: var(--size-2);
                            font-size: var(--font-size-1);
                            color: var(--color-text-primary);
                            cursor: pointer;
                        }

                        input[type="checkbox"] {
                            width: auto;
                            height: 1.25rem;
                            margin: 0;
                        }

                        details {
                            font-size: var(--font-size-0);
                            color: var(--color-text-secondary);

                            summary {
                                cursor: pointer;
                                color: var(--color-info);
                            }
                        }
                    }
```

- [ ] **Step 7: Verify existing tests still pass**

Run: `make test`
Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/web/handler-exerciseset.go ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "feat(ui): assisted checkbox and absolute-value weight input"
```

---

### Task 10: Handler — sign-flip on assisted POST (TDD)

**Files:**
- Modify: `cmd/web/handler-exerciseset.go`
- Modify: `cmd/web/handler-exerciseset_test.go`

- [ ] **Step 1: Write failing e2e test**

This test inserts an assisted `workout_exercise` row directly via SQL to bypass the swap UI (which only offers exercises matching its own filter logic). The point of the test is the *handler* sign-flip, not the swap flow.

Append to `cmd/web/handler-exerciseset_test.go`:

```go
func Test_application_exerciseSet_assisted_storage(t *testing.T) {
    var (
        ctx = t.Context()
        doc *goquery.Document
        err error
    )

    server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
    if err != nil {
        t.Fatalf("start server: %v", err)
    }
    client := server.Client()

    if _, err = client.Register(ctx); err != nil {
        t.Fatalf("register: %v", err)
    }

    // Set preferences and start a workout for today.
    formData := map[string]string{time.Now().Weekday().String(): "60"}
    if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
        t.Fatalf("get preferences: %v", err)
    }
    if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
        t.Fatalf("submit preferences: %v", err)
    }
    today := time.Now().Format("2006-01-02")
    if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
        t.Fatalf("start workout: %v", err)
    }

    // Look up the seeded "Assisted Pull-Up" id (added in Task 6) and the
    // current authenticated user id (Register stores it in the session).
    db := server.DB()
    var assistedID int
    if err = db.QueryRowContext(ctx,
        `SELECT id FROM exercises WHERE name = 'Assisted Pull-Up'`).Scan(&assistedID); err != nil {
        t.Fatalf("get Assisted Pull-Up id: %v", err)
    }

    // Insert a workout_exercise row pointing at Assisted Pull-Up, attached to
    // the just-started session for the test user. This avoids depending on
    // the swap UI's offered-alternatives logic.
    var slotID int
    if err = db.QueryRowContext(ctx,
        `INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id,
            warmup_completed_at)
         SELECT user_id, workout_date, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ')
         FROM workout_sessions WHERE workout_date = ?
         RETURNING id`, assistedID, today).Scan(&slotID); err != nil {
        t.Fatalf("insert assisted slot: %v", err)
    }
    // Seed three placeholder sets so the form has rows to submit against.
    for setNum := 1; setNum <= 3; setNum++ {
        if _, err = db.ExecContext(ctx,
            `INSERT INTO exercise_sets (workout_exercise_id, set_number,
                weight_kg, min_reps, max_reps)
             VALUES (?, ?, 0.0, 5, 5)`, slotID, setNum); err != nil {
            t.Fatalf("insert placeholder set %d: %v", setNum, err)
        }
    }

    slotPath := "/workouts/" + today + "/exercises/" + strconv.Itoa(slotID)
    if doc, err = client.GetDoc(ctx, slotPath); err != nil {
        t.Fatalf("get exercise set: %v", err)
    }

    setForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
        return s.Find("button[name='signal']").Length() > 0
    }).First()
    if setForm.Length() == 0 {
        t.Fatalf("expected signal form on assisted exercise")
    }
    setAction, _ := setForm.Attr("action")

    // Submit set 1 with Assisted checkbox CHECKED → expect stored as negative.
    if doc, err = client.SubmitForm(ctx, doc, setAction, map[string]string{
        "weight":   "20",
        "assisted": "on",
        "signal":   "on_target",
        "reps":     "8",
    }); err != nil {
        t.Fatalf("submit assisted set: %v", err)
    }

    var weight1 float64
    if err = db.QueryRowContext(ctx,
        `SELECT weight_kg FROM exercise_sets
         WHERE workout_exercise_id = ? AND set_number = 1`,
        slotID).Scan(&weight1); err != nil {
        t.Fatalf("query set 1 weight: %v", err)
    }
    if weight1 != -20.0 {
        t.Errorf("set 1 weight = %v, want -20.0 (assisted checkbox should negate)", weight1)
    }

    // Submit set 2 with Assisted checkbox UNCHECKED → expect stored as positive.
    setForm2 := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
        return s.Find("button[name='signal']").Length() > 0
    }).First()
    if setForm2.Length() == 0 {
        t.Fatalf("expected signal form for set 2")
    }
    setAction2, _ := setForm2.Attr("action")
    if _, err = client.SubmitForm(ctx, doc, setAction2, map[string]string{
        "weight": "5",
        // no "assisted" field → unchecked
        "signal": "on_target",
        "reps":   "8",
    }); err != nil {
        t.Fatalf("submit weighted set on assisted exercise: %v", err)
    }

    var weight2 float64
    if err = db.QueryRowContext(ctx,
        `SELECT weight_kg FROM exercise_sets
         WHERE workout_exercise_id = ? AND set_number = 2`,
        slotID).Scan(&weight2); err != nil {
        t.Fatalf("query set 2 weight: %v", err)
    }
    if weight2 != 5.0 {
        t.Errorf("set 2 weight = %v, want +5.0 (no checkbox should leave sign positive)", weight2)
    }
}
```

- [ ] **Step 2: Verify the test fails**

Run: `go test -v ./cmd/web -run Test_application_exerciseSet_assisted_storage`
Expected: FAIL on `set 1 weight = 20, want -20.0` — the handler currently stores the positive value as-is because the sign-flip logic doesn't exist yet.

- [ ] **Step 3: Implement the sign flip**

In `cmd/web/handler-exerciseset.go`, modify `recordSetCompletionWithWeight`. Find the section where `weight` is parsed (around lines 207-211):

```go
    weightStr := strings.Replace(r.PostForm.Get("weight"), ",", ".", 1)
    weight, err := strconv.ParseFloat(weightStr, 64)
    if err != nil {
        app.serverError(w, r, fmt.Errorf("parse weight: %w", err))
        return false
    }
```

Then locate where `signal` is read (around line 214). Insert the sign-flip immediately after `weight` is parsed and before `signal` is read. We need access to the `exercise` to check its type — pass it in.

Update the function signature and body. Change the function declaration (around lines 203-207) from:

```go
func (app *application) recordSetCompletionWithWeight(
    w http.ResponseWriter, r *http.Request,
    date time.Time, workoutExerciseID, setIndex int, dateStr string,
) bool {
```

to:

```go
func (app *application) recordSetCompletionWithWeight(
    w http.ResponseWriter, r *http.Request,
    date time.Time, workoutExerciseID, setIndex int, dateStr string,
    exercise workout.Exercise,
) bool {
```

Then insert the sign-flip right after weight parsing (between the existing `if err != nil { ... }` block and the `signal := workout.Signal(...)` line):

```go
    if exercise.ExerciseType == workout.ExerciseTypeAssisted &&
        r.PostForm.Get("assisted") != "" {
        weight = -weight
    }
```

Update the call site in `exerciseSetUpdatePOST` (around line 265). Change:

```go
        if !app.recordSetCompletionWithWeight(w, r, date, workoutExerciseID, setIndex, dateStr) {
            return
        }
```

to:

```go
        if !app.recordSetCompletionWithWeight(w, r, date, workoutExerciseID, setIndex, dateStr, exercise) {
            return
        }
```

- [ ] **Step 4: Verify the test passes**

Run: `go test -v ./cmd/web -run Test_application_exerciseSet_assisted_storage`
Expected: PASS — both subtests (assisted-checked stores -20, assisted-unchecked stores +5).

- [ ] **Step 5: Verify all tests pass**

Run: `make test`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/handler-exerciseset.go cmd/web/handler-exerciseset_test.go
git commit -m "feat(handler): negate weight on POST when Assisted checkbox is set"
```

---

### Task 11: AI generator — extend exercise_type enum and prompt

**Files:**
- Modify: `internal/workout/generator-exercise.go:90-94`
- Modify: `internal/workout/generator-exercise.go:69` (prompt template)

- [ ] **Step 1: Extend the JSON schema enum**

In `internal/workout/generator-exercise.go`, find the `exercise_type` property in `exerciseJSONSchema.MarshalJSON` (around lines 90-94). Change from:

```go
            "exercise_type": map[string]any{
                "type":        "string",
                "description": "StartWorkout of exercise (weighted or bodyweight)",
                "enum":        []string{"weighted", "bodyweight"},
            },
```

to:

```go
            "exercise_type": map[string]any{
                "type":        "string",
                "description": "Type of exercise: weighted (external load), bodyweight (no load), or assisted (band/machine help, can also be loaded later)",
                "enum":        []string{"weighted", "bodyweight", "assisted"},
            },
```

- [ ] **Step 2: Update the prompt instructions**

In the same file, find the prompt template inside `generateBaseExercise` (around line 69):

```go
For "exercise_type", use one of: "weighted", "bodyweight"
```

Change to:

```go
For "exercise_type", use one of: "weighted", "bodyweight", "assisted"
```

- [ ] **Step 3: Verify build**

Run: `make build`
Expected: clean build.

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: all PASS — the AI generator is exercised via integration paths only when `OPENAI_API_KEY` is set; the schema change is a pure data update.

- [ ] **Step 5: Commit**

```bash
git add internal/workout/generator-exercise.go
git commit -m "feat(generator): include 'assisted' in AI exercise type enum and prompt"
```

---

### Task 12: Final verification and CI

**Files:**
- (none modified — verification only)

- [ ] **Step 1: Run full CI**

Run: `make ci`
Expected: build, lint, test, sec — all PASS.

- [ ] **Step 2: Manual smoke test**

Start the dev server: `make run` (or whatever the project's run target is).

In a browser:
1. Log in as a registered user.
2. Visit the admin exercise edit page for "Assisted Pull-Up" — confirm the type shows `assisted`.
3. Set workout preferences for today, start a workout.
4. Swap one exercise slot to "Assisted Pull-Up".
5. On the exercise set page, complete the warmup. Confirm the weight input is positive (no minus sign required) and the "Assisted" checkbox appears with a `<details>` "What's this?" disclosure.
6. Submit a set with weight=20, checkbox checked → confirm display now shows `-20 kg` and the next set's recommendation reflects the assisted progression.
7. Submit a second set with weight=20, checkbox checked, signal="too_light" → next set should recommend `-17.5 kg`.
8. Submit a third set with weight=0, checkbox unchecked, signal="too_heavy" → next set should recommend `-2.5 kg` (zero-boundary fix proven via UI).
9. Visit the exercise info page for Assisted Pull-Up — confirm the chart renders with negative Y-axis values.

If any step fails, file an issue rather than patching ad-hoc — the tests should have caught it.

- [ ] **Step 3: Done**

No commit needed for this task.

---

## Self-review notes

This plan covers every section of `specs/assisted-exercises.md`:

- PR 1 (schema, domain, weekplanner): Tasks 1, 2
- PR 2 (progression math): Task 3
- PR 3 (HTTP handler): Tasks 4, 5, 10
- PR 4 (templates & form): Tasks 7, 8, 9
- PR 5 (seed + AI generator): Tasks 6, 11

Order honors dependencies: schema before fixtures (Task 6 needs Task 1), domain constants before handler gates (Task 4 needs Task 2), fixtures before e2e test (Task 10 needs Task 6). Each task ends with a commit so partial completion is recoverable.
