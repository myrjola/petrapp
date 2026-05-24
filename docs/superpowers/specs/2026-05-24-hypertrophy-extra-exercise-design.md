# Hypertrophy Extra Exercise — Design

**Date:** 2026-05-24
**Status:** Draft for review
**Type:** Feature design

## Motivation

The weekly planner maps session duration to a fixed exercise count:
`>=90 min → 4`, `>=60 min → 3`, `>0 → 2`
(`internal/domain/planner.go:184-195`). The rule is periodization-blind.

Working-set math under the current set/rest scheme
(`internal/domain/progression_scheme.go:5-16`) makes strength days
(4 sets × 180 s rest) crowd the 90-minute budget, while hypertrophy days
(3 sets × 150 s rest) finish with ~30 minutes of slack. The slack is
unused: there is no "auto-extend session" or "longer rest" path that
absorbs it.

This design uses that slack on hypertrophy days by adding one extra
exercise per session, which raises weekly volume on the periodization
type where added volume is most aligned with the training goal.

## Rule

`exercisesPerSession` becomes periodization-aware:

| Minutes | Strength | Hypertrophy (non-deload) | Deload (`pt` forced to Hypertrophy) |
|---|---|---|---|
| ≥ 90 | 4 | **5** | 4 |
| ≥ 60 | 3 | **4** | 3 |
| > 0  | 2 | 2 | 2 |
| 0    | 0 | 0 | 0 |

Bolded cells are the only behavioural changes. Two gates carve out the
bump:

- `minutes >= 60` — 45-minute sessions stay at 2. The 45-min budget is
  too tight under any periodization for an extra exercise.
- `!isDeload` — deload weeks keep their base counts even though `pt`
  is forced to Hypertrophy (`planner.go:89-91`). Deload's intent is
  reduced volume; bumping its count would invert that intent.

## Why not re-tune `MuscleGroupTarget.WeeklySetTarget`

`WeeklySetTarget` values are minimums: `MuscleGroupVolume.Planned >=
Completed` semantics already accept overshoot, and the volume bars in
the UI simply render further past target on hypertrophy weeks. Targets
are also user-editable per-muscle-group, so there is no single source
of truth to migrate. Users who want tighter weekly volume can adjust
targets themselves.

## Why not surface "expected session duration"

The codebase has no minute-budget arithmetic — the duration preference
is only ever consumed by `exercisesPerSession` to pick a count. Adding
a duration estimator (warm-up minutes, per-set time, rest aggregation)
is a larger surface than this change merits and is orthogonal to the
volume-allocation question. Tracked as out-of-scope below.

## Code changes

All changes in `internal/domain/planner.go`. No new files, no
migration, no feature flag.

### Constants (`planner.go:11-21`)

Add two constants mirroring the existing pattern; do not modify the
existing three.

```go
const (
    minutesLong   = 90
    minutesMedium = 60

    exercisesLong              = 4
    exercisesLongHypertrophy   = 5
    exercisesMedium            = 3
    exercisesMediumHypertrophy = 4
    exercisesShort             = 2

    maxMuscleGroupDaysPerWeek = 2
    numPeriodizationTypes     = 2
)
```

### `exercisesPerSession` (`planner.go:183-195`)

New signature, new branch. The function stays small and centralised.

```go
// exercisesPerSession returns how many exercises to include based on session
// duration and periodization. Hypertrophy non-deload sessions of >= 60 min
// get one extra exercise to use the working-set time budget more fully;
// strength and deload sessions keep their base counts.
func exercisesPerSession(prefs Preferences, weekday time.Weekday, pt PeriodizationType, isDeload bool) int {
    hyperBonus := pt == PeriodizationHypertrophy && !isDeload
    switch minutes := prefs.MinutesForDay(weekday); {
    case minutes >= minutesLong:
        if hyperBonus {
            return exercisesLongHypertrophy
        }
        return exercisesLong
    case minutes >= minutesMedium:
        if hyperBonus {
            return exercisesMediumHypertrophy
        }
        return exercisesMedium
    case minutes > 0:
        return exercisesShort
    default:
        return 0
    }
}
```

### `Plan` (`planner.go:84-115`)

`pt` and `isDeload` are already in scope at the call site (lines 88-92).
Pass them through:

```go
pt := nextPeriodizationType(firstPT, i)
if isDeload {
    pt = PeriodizationHypertrophy
}
n := exercisesPerSession(wp.Prefs, day.Weekday(), pt, isDeload)
```

### `PlanDay` (`planner.go:124-181`)

Reorder so `pt` and `isDeload` are computed before `n`, then apply the
same rule to the unscheduled-day fallback:

```go
// ... compute idx, firstPT, pt, isDeload (existing logic, moved up) ...

n := exercisesPerSession(wp.Prefs, date.Weekday(), pt, isDeload)
if n == 0 {
    n = exercisesMedium
    if pt == PeriodizationHypertrophy && !isDeload {
        n = exercisesMediumHypertrophy
    }
}
```

The `n == 0` branch handles the case where `PlanDay` is invoked for a
weekday the user has not scheduled (ad-hoc workout); the fallback gets
the same hypertrophy bump as a scheduled medium-length day.

### Remove `exercisesPerWeek` (`planner.go:219-228`)

This helper is `//nolint:unused` with a "kept for future extensibility"
comment. Maintaining a periodization-blind weekly aggregate while the
per-day rule becomes periodization-aware would be misleading; delete
the function rather than thread `pt`/`isDeload` through dead code.

## Edge cases

- **Pool exhaustion.** Phase B of
  `selectExercisesForDayWithPeriodization` (`planner.go:437-446`)
  returns fewer than `n` if the category-filtered pool runs dry —
  existing behaviour. With `n = 5` the case becomes slightly more
  likely on small custom exercise libraries, but the planner already
  handles it without error.
- **Mixed-periodization week.** Each day's count is computed
  independently from that day's `pt`, so a strength-first 4-day week
  naturally produces `[4, 5, 4, 5]`.
- **Deload + 90 min.** `pt` is forced to Hypertrophy but
  `isDeload == true` excludes the bump. Tested explicitly below.
- **Already-generated weekly plans.** `Plan()` writes sessions to the
  DB at generation time; existing plans keep their counts. The next
  Monday's `Plan()` invocation picks up the new rule. No migration.

## Testing

### New unit test for `exercisesPerSession`

In `internal/domain/planner_internal_test.go`, table-driven across the
duration × periodization × deload matrix:

| Minutes | Periodization | Deload | Want |
|---|---|---|---|
| 90 | Strength    | false | 4 |
| 90 | Hypertrophy | false | 5 |
| 90 | Hypertrophy | true  | 4 |
| 60 | Strength    | false | 3 |
| 60 | Hypertrophy | false | 4 |
| 60 | Hypertrophy | true  | 3 |
| 45 | Strength    | false | 2 |
| 45 | Hypertrophy | false | 2 |
| 45 | Hypertrophy | true  | 2 |
| 0  | Strength    | false | 0 |
| 0  | Hypertrophy | false | 0 |

### New end-to-end planner test

In `internal/domain/planner_internal_test.go`, one `Plan()` test with
a 4-day schedule (e.g. Mon/Tue/Thu/Fri at 90 min each) on a
strength-first, non-deload week:

- Assert Mon (strength) has 4 exercise sets.
- Assert Tue (hypertrophy) has 5 exercise sets.
- Assert Thu (strength) has 4 exercise sets.
- Assert Fri (hypertrophy) has 5 exercise sets.

Fixture must supply enough non-conflicting exercises to fill 5 slots
on a hypertrophy day; verify against the existing test fixtures
before writing.

### Updates to existing tests

Audit `planner_internal_test.go` for assertions of the form "90-min
hypertrophy day → 4 exercises" or equivalent computed-from-`len`
checks. Update each to the new expected count. No structural
changes — only constant flips.

### Not tested

- Weekly volume target overshoot. No behavioural change in code; the
  rendered bars simply pass their target marks more often. Covered by
  documentation in this design, not by an assertion.
- `PlanDay` ad-hoc path with `n == 0` fallback. The fallback's
  bump-when-hypertrophy branch is exercised by the
  `exercisesPerSession` unit test; the surrounding `PlanDay` logic
  has not changed.

## File touches

- `internal/domain/planner.go` — constants, signature change,
  `Plan`/`PlanDay` call sites, `exercisesPerWeek` removal.
- `internal/domain/planner_internal_test.go` — new
  `exercisesPerSession` table test, new `Plan` end-to-end test,
  fixture audits.

## Out of scope

- A duration estimator that aggregates warm-up + working-set + rest +
  transition time into a displayed "expected session length". Useful
  separately, not needed for the volume-allocation decision this
  design makes.
- Re-tuning baseline `MuscleGroupTarget.WeeklySetTarget` values.
  Targets are user-editable; overshoot is acceptable under existing
  `Planned >= Completed` semantics.
- A user preference to opt out of the bump. The rule is automatic.
- Changes to the set/rest scheme in `progression_scheme.go`. Per-set
  prescription is unaffected.
- Backfilling already-generated weekly plans. Existing plans keep
  their counts; the rule applies on the next plan generation.
