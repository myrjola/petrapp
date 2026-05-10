# Planning data flow: one funnel for set construction, one home for set-value display

## Background

The 2026-05-09 hypertrophy-window incident (PR #89, fix in `1cbde68`) was the
visible symptom of a broader pattern in the planning slice: the rule that
turns `(Exercise, PeriodizationType)` into a per-set prescription is consumed
in two places that can drift.

- **Insert path A — PlanWeek.** `service.go:197-211` walks the week
  planner's `[]weekplanner.PlannedSet` output and unwraps `TargetValue`
  directly into `workout.Set` rows. The planner itself called
  `exerciseprogression.DeriveScheme` to produce those values.
- **Insert path B — AddExercise / SwapExercise.** Goes through
  `Service.buildSetsForAdd → Service.createDefaultSets → deriveSchemeForExercise
  → exerciseprogression.DeriveScheme`. PR #89 added regression tests
  (`Test_AddExercise_DerivesTargetValueFromPeriodization`,
  `Test_ReplaceExerciseInSession_DerivesTargetValueFromPeriodization`) after
  the same kind of bug bit there.

Two paths means two places to keep in sync. Yesterday's narrow fix collapsed
the display-side branch but did nothing about insert paths, by deliberate
scope.

On the display side, `formatTarget` in `cmd/web/handler-exerciseset.go` is
now a clean two-branch primitive after yesterday's collapse. But the function
still lives in the handler package and takes `Exercise` as input — leaving
room for a future contributor to grow it back by adding "just one more"
input. The same handler also branches on `Exercise.IsTimed()` two more times
inside `prepareSetsDisplay` (for `CompletedStr` and `Unit`).

The bug class — display logic reconstructing what the rule produced, or two
insert paths derived independently — is what this spec is aimed at.

## Goal

Two changes, both in the planning slice:

1. **Single insert funnel.** Exactly one function in the workout package
   takes `(Exercise, PeriodizationType)` and returns the `[]Set` that gets
   persisted. PlanWeek, AddExercise, and SwapExercise all call it.
2. **Single display home for set values.** Rep-vs-second formatting and
   unit-label selection move onto `Exercise` as methods. Handler-side
   `formatTarget` deletes; `prepareSetsDisplay` calls the domain methods.

`Set.TargetValue` continues to be persisted at write time — historical
sessions preserve the prescription that was active when they were generated.
Re-derivation at read time is not introduced.

`Scheme.RestSeconds` continues to be unstored. Rest UI is out of scope; the
storage-vs-derive principle for it is deferred until rest UI lands.

## The rule, generalized

The 2026-05-09 spec codified:

> Any value that depends on multiple domain attributes, or that encodes a
> business rule, lives as a method on the domain type that owns the rule.

This spec extends the same principle to single-attribute formatting that is
called from two or more sites: when the same `if exercise.IsTimed() { ... }`
branch appears twice in handler code, that's a domain method waiting to
exist.

The rule's enforcement remains by convention, not types. The structural
protection comes from a) the obvious method-to-call living next to the
domain type, and b) the funnel being the only thing in the workout package
that constructs prescription `[]Set` slices.

## Design

### 1. The insert funnel — `buildPlannedSets`

**New file `internal/workout/planning.go`** holds the funnel. A new file
keeps `service.go`'s 1,319-line growth from continuing.

```go
package workout

// buildPlannedSets returns the persisted set slice for an exercise
// prescribed in a session of the given periodization. Single source of
// truth for "what sets does this exercise get when first added to a
// session" — used by PlanWeek, AddExercise, and SwapExercise.
//
// For weighted/assisted exercises with prior history, callers may
// post-process the returned slice to seed WeightKg from the latest
// completed set (see buildSetsForAdd). This function deliberately does
// not load history; it produces a clean prescription.
func buildPlannedSets(exercise Exercise, periodization PeriodizationType) []Set {
    targetValue, n := deriveSchemeForExercise(exercise, periodization)
    sets := make([]Set, n)
    for i := range sets {
        var weight *float64
        if !exercise.IsTimed() && exercise.ExerciseType != ExerciseTypeBodyweight {
            weight = new(float64)
        }
        sets[i] = Set{ //nolint:exhaustruct // CompletedValue, CompletedAt, Signal start nil.
            WeightKg:    weight,
            TargetValue: targetValue,
        }
    }
    return sets
}
```

#### Call site changes

- **`service.go:197-211`** (PlanWeek's inner per-exercise loop). Build a
  lookup map once before the outer loop, then call `buildPlannedSets`
  per exercise:

  ```go
  // Before (lines 196-211)
  for i, ps := range plannedSessions {
      periodType := PeriodizationStrength
      if ps.PeriodizationType == weekplanner.PeriodizationHypertrophy {
          periodType = PeriodizationHypertrophy
      }
      exerciseSets := make([]exerciseSetAggregate, len(ps.ExerciseSets))
      for j, pes := range ps.ExerciseSets {
          sets := make([]Set, len(pes.Sets))
          for k, planSet := range pes.Sets {
              sets[k] = Set{
                  TargetValue: planSet.TargetValue,
              }
          }
          exerciseSets[j] = exerciseSetAggregate{
              ExerciseID: pes.ExerciseID,
              Sets:       sets,
          }
      }
      // ... assemble sessionAggrs[i]
  }

  // After
  exerciseByID := make(map[int]Exercise, len(exercises))
  for _, ex := range exercises {
      exerciseByID[ex.ID] = ex
  }
  for i, ps := range plannedSessions {
      periodType := PeriodizationStrength
      if ps.PeriodizationType == weekplanner.PeriodizationHypertrophy {
          periodType = PeriodizationHypertrophy
      }
      exerciseSets := make([]exerciseSetAggregate, len(ps.ExerciseSets))
      for j, pes := range ps.ExerciseSets {
          exerciseSets[j] = exerciseSetAggregate{
              ExerciseID: pes.ExerciseID,
              Sets:       buildPlannedSets(exerciseByID[pes.ExerciseID], periodType),
          }
      }
      // ... assemble sessionAggrs[i]
  }
  ```

  The `exercises` slice (`[]workout.Exercise`) is already in scope at line
  162 where `wpExercises` is built from it; the lookup map reuses that
  data. The inner two loops collapse into one. The
  `weekplanner.PlannedSet` slice is no longer consulted by service.go
  for the workout package's persistence shape; it remains internal to
  weekplanner for muscle-volume tracking.

- **`service.go:1088-1105`** (`createDefaultSets`). Collapse to one call:

  ```go
  func (s *Service) createDefaultSets(ex Exercise, pt PeriodizationType) []Set {
      return buildPlannedSets(ex, pt)
  }
  ```

  Or delete the wrapper and have `buildSetsForAdd` call `buildPlannedSets`
  directly. Either is fine — the wrapper has one caller (`buildSetsForAdd`)
  and adds no behavior. Implementation may pick.

- **`deriveSchemeForExercise`** stays where it is. It's the per-exercise
  scheme step inside the funnel.

#### What does not change

- `weekplanner` continues to compute and return `PlannedSet`s. The
  planner's internal use of them (for muscle-group volume calculations)
  is unchanged. Service.go simply stops consulting `PlannedSet.TargetValue`
  and re-derives via `buildPlannedSets`.
- `exerciseprogression.DeriveScheme` is unchanged — still the pure rule.
- `periodizationToProgression(pt)` continues to bridge the
  `workout.PeriodizationType` (string) → `exerciseprogression.PeriodizationType`
  (int) gap. Resolving that duplication is friction A, deferred.

#### Re-derivation cost

`DeriveScheme` is a pure 5-line function. Re-deriving inside service.go
when the planner already derived once is negligible cost and removes the
asymmetry where AddExercise re-derives but PlanWeek consumes a pre-derived
value. Both paths now derive at the same point.

### 2. Display methods on `Exercise`

**Add to `internal/workout/models.go`:**

```go
// FormatSetValue returns the user-visible string for a set's target or
// completed value. Reps render as "%d"; seconds render as "%ds". The
// unit choice is driven by ExerciseType — display layers must call this
// rather than reconstruct the formatting from periodization or any
// other field.
func (e Exercise) FormatSetValue(value int) string {
    if e.IsTimed() {
        return fmt.Sprintf("%ds", value)
    }
    return strconv.Itoa(value)
}

// SetValueUnit returns the input-label unit for a set value: "reps" or
// "seconds". Used by handlers when rendering input form labels.
func (e Exercise) SetValueUnit() string {
    if e.IsTimed() {
        return "seconds"
    }
    return "reps"
}
```

This adds `fmt` and `strconv` imports to `models.go`. Acceptable — the
"models hold pure data only" framing in the package CLAUDE.md is already
relaxed by `IsTimed()` and the JSON-schema marshaller.

**Handler changes (`cmd/web/handler-exerciseset.go`):**

```go
// Before — formatTarget at lines 17-25 (deleted entirely)

// Before — prepareSetsDisplay at lines 49-74
func prepareSetsDisplay(exercise workout.Exercise, sets []workout.Set) []setDisplay {
    unit := "reps"
    if exercise.IsTimed() {
        unit = "seconds"
    }
    displays := make([]setDisplay, len(sets))
    for i, set := range sets {
        targetStr := formatTarget(exercise, set.TargetValue)
        completedStr := ""
        if set.CompletedValue != nil {
            if exercise.IsTimed() {
                completedStr = fmt.Sprintf("%ds", *set.CompletedValue)
            } else {
                completedStr = strconv.Itoa(*set.CompletedValue)
            }
        }
        displays[i] = setDisplay{ Set: set, TargetStr: targetStr, CompletedStr: completedStr, Unit: unit, Number: i + 1 }
    }
    return displays
}

// After
func prepareSetsDisplay(exercise workout.Exercise, sets []workout.Set) []setDisplay {
    unit := exercise.SetValueUnit()
    displays := make([]setDisplay, len(sets))
    for i, set := range sets {
        completedStr := ""
        if set.CompletedValue != nil {
            completedStr = exercise.FormatSetValue(*set.CompletedValue)
        }
        displays[i] = setDisplay{
            Set:          set,
            TargetStr:    exercise.FormatSetValue(set.TargetValue),
            CompletedStr: completedStr,
            Unit:         unit,
            Number:       i + 1,
        }
    }
    return displays
}
```

After this, `prepareSetsDisplay` has zero `IsTimed()` calls; the
`fmt`/`strconv` imports remain used by other functions in the file
(weight formatting in the form-handling helpers), so they don't go away.

#### What does not move into the domain

- `processEntryData` in `handler-exercise-info.go:108` switches on
  `ExerciseType` to format chart entries with distinct per-type shapes
  (with-weight vs reps vs seconds). Yesterday's spec called this out as
  legitimate per-type display shape, not rule reconstruction. Honor that
  precedent.
- The `exerciseSetGET` switch at `handler-exerciseset.go:137-154` picks a
  progression engine. Workflow choice, not display rule.
- `setDisplay`, `exerciseSetTemplateData`, and other per-page template
  structs stay handler-owned. Per-page concerns (`Number`, `EditingIndex`,
  `IsEditing`, `LastCompletedAt`, weight inputs) belong with the page
  that needs them. This is the deliberate stop short of moving display
  structs into the domain.

### 3. CLAUDE.md updates

No documentation changes in this spec. The existing 2026-05-09 rule
(multi-attribute derivations belong on the domain) already covers the
spirit. The new methods (`FormatSetValue`, `SetValueUnit`) are
single-attribute branches that became domain methods because they have
2+ call sites — a generalization that's evident from the methods
themselves.

If the rule in `internal/workout/CLAUDE.md` proves insufficient over
time, a follow-up spec can extend it. Adding doc text speculatively now
risks rule sprawl.

### 4. Testing

**New tests:**

- `internal/workout/models_test.go` (or a new file): table-driven
  `Test_Exercise_FormatSetValue` covering all four `ExerciseType` values
  for both rep-based and timed display. `Test_Exercise_SetValueUnit`
  covering the same.
- `internal/workout/planning_test.go` (new): `Test_buildPlannedSets`
  covering each `(ExerciseType × PeriodizationType)` combination.
  Verifies target value matches `DeriveScheme` output, set count matches
  `Scheme.TargetSets` (or `defaultTimedSets` for time-based), and the
  `WeightKg` pointer presence rule (nil for time-based and bodyweight,
  non-nil empty pointer for weighted/assisted).

**Existing tests to update:**

- `cmd/web/handler-exerciseset_test.go`: remove `Test_formatTarget` (the
  function it tested is gone). Its coverage moves to
  `Test_Exercise_FormatSetValue` in the workout package.
- The PR #89 regression tests
  (`Test_AddExercise_DerivesTargetValueFromPeriodization`,
  `Test_ReplaceExerciseInSession_DerivesTargetValueFromPeriodization`)
  should pass unchanged. They verify the post-funnel TargetValue is
  correct, which it still is.
- Any handler test that asserts on the rendered set-display HTML
  continues to assert on the same strings — `FormatSetValue` produces
  the same output as the deleted `formatTarget` did.

**The contract that pins the bug class** is structural, not test-shaped:
after this change, there is exactly one function in the workout package
that constructs prescription `[]Set` slices (`buildPlannedSets`), and
exactly one place in `cmd/web/` that formats a set value
(`Exercise.FormatSetValue`). Code review enforces single-callsite
discipline; the unit tests above pin each function's output.

A property-style "PlanWeek output equals buildPlannedSets output" test
was considered and rejected as redundant: if both paths call the same
function, they cannot drift; the unit tests on `buildPlannedSets` are
sufficient.

## Risks and trade-offs

- **Convention, not types.** The display methods *encourage* but don't
  *enforce* "no reconstruction." A future handler can still introduce
  its own `formatTarget` clone. The CLAUDE.md rule plus the obvious
  method-to-call are the protection. Approach 3 (move `setDisplay` into
  the domain with unexported fields) would have made this structural; we
  deliberately stopped short to keep per-page UI flexibility.
- **PlanWeek path re-derives what the planner derived.** Negligible cost
  (`DeriveScheme` is a pure 5-line function). The behavioral output is
  unchanged because both call sites consult the same pure function with
  the same inputs.
- **`models.go` gains `fmt`/`strconv` imports** for the formatting
  methods. Mild concern about mixing presentation into domain, but the
  alternative (a separate display package) adds a layer for two methods.
  Not worth the indirection.

## Out of scope

- **`RestSeconds` storage.** When rest UI lands, decide schema vs
  re-derive consistently with the precedent set here (TargetValue is
  stored at write time, never re-derived at read time).
- **Parallel types between packages** (`weekplanner.PeriodizationType`
  int, `workout.PeriodizationType` string,
  `exerciseprogression.PeriodizationType` int; duplicated `Category`,
  `ExerciseType`, `Preferences`, `Exercise`). Friction A. Future spec.
- **`service.go` size.** This funnel collapses ~15 lines in PlanWeek but
  doesn't restructure orchestration. Friction C. Future spec.
- **Weight-seeding asymmetry** between PlanWeek (no historical weight
  seeded; resolved lazily by exerciseprogression at display time) and
  Add/Swap (seeds `WeightKg` from `findHistoricalSets` at write time).
  Pre-existing and orthogonal to set-target derivation.
- **`processEntryData`'s per-type chart formatting.** Different per-type
  display shapes, not rule reconstruction. Leave alone per the
  2026-05-09 precedent.

## Change set

Three logical changes:

1. `internal/workout/planning.go` (new): `buildPlannedSets`. Plus the
   call site collapses in `service.go` (PlanWeek path) and the wrapper
   collapse in `createDefaultSets`.
2. `internal/workout/models.go`: add `FormatSetValue` and
   `SetValueUnit` methods on `Exercise`. Add `fmt` and `strconv` imports.
3. `cmd/web/handler-exerciseset.go`: delete `formatTarget`; update
   `prepareSetsDisplay` to call the domain methods. Plus the deletion of
   `Test_formatTarget` in the matching test file.

Tests covering the new code live alongside it as described above.

## Acceptance

- `make ci` passes (init, build, lint, test, sec).
- Manual spot-check of one weighted, one bodyweight, one assisted, and
  one time-based exercise in a hypertrophy session: per-set target string
  is the planner's integer (or `"Ns"` for time-based), unit label is
  "reps" or "seconds" as appropriate, completed string formats match.
- `grep -n "formatTarget" cmd/web/` returns no results.
- `grep -rn "DeriveScheme" internal/workout/` shows callers only inside
  `deriveSchemeForExercise` (not in PlanWeek, AddExercise, or
  SwapExercise directly).
