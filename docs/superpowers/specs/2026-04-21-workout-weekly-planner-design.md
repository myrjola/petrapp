# Weekly Workout Planner Design

**Date:** 2026-04-21
**Status:** Approved for implementation

## Problem

The current workout generator creates each session independently when the user loads the home page. Each day is generated without awareness of the rest of the week, so muscle group distribution across the week is accidental rather than deliberate. There is no guarantee that each muscle group is trained twice per week, and exercises are selected with an 80% continuity bias from the same weekday in the previous week rather than from science-based volume principles.

## Goal

Generate a full week of workouts at once when the user loads the home page and no plan exists for the current week. Each session is planned as part of a coherent weekly unit, ensuring each major muscle group is trained approximately twice per week and exercise selection is driven by muscle group coverage rather than historical continuity.

## Scope

- Out of scope for this change: requesting a workout on a day outside the weekly plan. For now, this returns an error page. Handling ad-hoc sessions is deferred.
- Out of scope: per-user muscle group volume targets. Targets are global defaults seeded in fixtures.

---

## Architecture

### New Package: `internal/weekplanner/`

A pure logic package with no database access, following the pattern of `internal/exerciseprogression/`. The `internal/workout` service imports it; it does not import workout.

### Deleted Files

- `internal/workout/generator.go` — all logic either moves to `weekplanner` or is dropped (continuity logic is removed entirely)
- `internal/workout/generator_internal_test.go` — replaced by weekplanner tests

### Kept Files

- `internal/workout/generator-exercise.go` — AI-powered exercise creation, unrelated to workout planning; kept unchanged
- `internal/workout/generator-exercise_internal_test.go` — kept unchanged

### Service Changes (`internal/workout/service.go`)

- `ResolveWeeklySchedule()` gains a new branch: if zero sessions exist for the current week, call `weekplanner.WeeklyPlanner.Plan()` and persist all resulting sessions in a single DB transaction.
- `GetSession()` no longer generates sessions on demand. If no session exists for the requested date, it returns `ErrSessionNotFound`. The HTTP handler renders an error page.

### Schema Change (`internal/sqlite/schema.sql`)

One new table: `muscle_group_weekly_targets`. Seeded in `fixtures.sql`.

---

## Data Model

### New DB Table

```sql
CREATE TABLE muscle_group_weekly_targets
(
    muscle_group_name TEXT PRIMARY KEY REFERENCES muscle_groups (name),
    weekly_sets_target INTEGER NOT NULL CHECK (weekly_sets_target > 0)
) STRICT;
```

Seeded with targets for the 9 tracked muscle groups:

| Muscle Group | Weekly Sets Target |
|---|---|
| Chest | 10 |
| Shoulders | 10 |
| Upper Back | 10 |
| Lats | 10 |
| Quads | 10 |
| Triceps | 8 |
| Biceps | 8 |
| Hamstrings | 8 |
| Glutes | 8 |

Untracked muscle groups (Abs, Calves, Traps, Forearms, Lower Back, Adductors, Hip Flexors, Obliques) accumulate volume passively as secondary muscles and are not targeted explicitly.

### No Other Schema Changes

Existing tables (`workout_sessions`, `exercise_sets`, `workout_exercise`) are unchanged. Planned sessions store `weight_kg = NULL` for all sets — weight is resolved lazily by `exerciseprogression` when the user opens the exercise set page.

### `internal/weekplanner/` Types

```go
type Category string

const (
    CategoryFullBody Category = "full_body"
    CategoryUpper    Category = "upper"
    CategoryLower    Category = "lower"
)

type ExerciseType string

const (
    ExerciseTypeWeighted   ExerciseType = "weighted"
    ExerciseTypeBodyweight ExerciseType = "bodyweight"
)

type PeriodizationType int

const (
    PeriodizationStrength    PeriodizationType = 0 // ~5 reps
    PeriodizationHypertrophy PeriodizationType = 1 // ~8 reps
)

// Preferences describes which days are workout days and their duration.
type Preferences struct {
    // MondayMinutes through SundayMinutes: 0 = rest day, 45/60/90 = workout day
    MondayMinutes    int
    TuesdayMinutes   int
    WednesdayMinutes int
    ThursdayMinutes  int
    FridayMinutes    int
    SaturdayMinutes  int
    SundayMinutes    int
}

// Exercise is a dependency-free representation of an exercise for planning purposes.
// StartingWeightKg is intentionally absent — weight is resolved at runtime by exerciseprogression.
type Exercise struct {
    ID                    int
    Category              Category
    ExerciseType          ExerciseType
    PrimaryMuscleGroups   []string
    SecondaryMuscleGroups []string
}

type MuscleGroupTarget struct {
    Name           string
    WeeklySetTarget int
}

// WeeklyPlanner holds the inputs for planning a week of workouts.
type WeeklyPlanner struct {
    Prefs     Preferences
    Exercises []Exercise
    Targets   []MuscleGroupTarget
}

func NewWeeklyPlanner(prefs Preferences, exercises []Exercise, targets []MuscleGroupTarget) *WeeklyPlanner

// Plan returns one PlannedSession per scheduled workout day for the week starting on startingDate.
// Returns an error if startingDate is not a Monday.
func (wp *WeeklyPlanner) Plan(startingDate time.Time) ([]PlannedSession, error)

type PlannedSession struct {
    Date              time.Time
    Category          Category
    PeriodizationType PeriodizationType
    ExerciseSets      []PlannedExerciseSet
}

type PlannedExerciseSet struct {
    ExerciseID int
    Sets       []PlannedSet
}

// PlannedSet has no WeightKg — resolved lazily by exerciseprogression when user works out.
type PlannedSet struct {
    MinReps int
    MaxReps int
}
```

---

## Session Size

Session duration from preferences maps to exercise count:

| Duration | Exercises per session | Sets per exercise | Total sets |
|---|---|---|---|
| 45 min | 2 | 3 | 6 |
| 60 min | 3 | 3 | 9 |
| 90 min | 4 | 3 | 12 |

---

## Planning Algorithm

`Plan(startingDate)` runs in three phases.

### Phase 1 — Category Determination

For each scheduled workout day, apply the adjacency rule:

```
if isWorkoutDay(today) AND isWorkoutDay(tomorrow):
    category = Lower
else if isWorkoutDay(yesterday):
    category = Upper
else:
    category = Full Body
```

`isWorkoutDay(t)` checks `prefs.IsWorkoutDay(t.Weekday())` — preference-based, not session-based. This means week boundaries wrap naturally through ordinary date arithmetic:

- Sunday's "tomorrow" = Monday → checks Monday preference
- Monday's "yesterday" = Sunday → checks Sunday preference

**Example:** Sun/Mon/Tue all scheduled → Sun=Lower, Mon=Upper, Tue=Upper. No consecutive same-region training, no special wrap-around logic needed.

### Phase 2 — Muscle Group Slot Allocation

Goal: assign each tracked muscle group to at least 2 compatible workout days, distributed as evenly as possible.

Muscle group to day compatibility (based on which exercise categories can target that group):
- **Upper muscle groups** (Chest, Shoulders, Triceps, Biceps, Upper Back, Lats) → valid on Upper and Full Body days
- **Lower muscle groups** (Quads, Hamstrings, Glutes) → valid on Lower and Full Body days

Full Body days are valid for all muscle groups. Upper days are valid only for upper muscle groups. Lower days are valid only for lower muscle groups.

Algorithm (most-constrained-first greedy):
1. For each tracked muscle group, compute the set of valid days (days whose category allows that group).
2. Sort muscle groups ascending by number of valid days (most constrained first).
3. For each muscle group in order, assign it to the 2 valid days with the fewest muscle group assignments so far.
4. If fewer than 2 valid days exist, assign to however many are available (graceful degradation).

Result: `dayMuscleGroups[day] = []string` — priority muscle groups for each day.

### Phase 3 — Exercise Selection

For each scheduled day, given `dayMuscleGroups[day]` and the day's category:

**Exercise pool filtering by category:**
- Upper day → only `upper` exercises
- Lower day → only `lower` exercises
- Full Body day → `upper`, `lower`, and `full_body` exercises

**Selection within the filtered pool:**
1. Score each exercise by how many of the day's priority muscle groups it covers via primary muscle groups.
2. Greedily select the top N exercises by score (N = exercises per session from the duration table). Break ties randomly. When an exercise covers multiple priority groups, all covered groups are marked satisfied.
3. If priority groups remain unsatisfied after N exercises, they are deferred (best-effort — not an error).
4. Create 3 sets per exercise with MinReps/MaxReps from the session's periodization type. WeightKg = NULL.

### Periodization

The periodization type for the first session of the week is derived deterministically:

```
exercisesPerWeek = sum of exercises-per-session across all scheduled days
weeksSinceEpoch  = startingDate.Unix() / (7 * 24 * 3600)
firstPeriodization = (weeksSinceEpoch * exercisesPerWeek) % 2
```

Each subsequent session in the week alternates. This is fully deterministic from inputs — no DB query for completed session count.

Rep ranges by periodization type:
- `PeriodizationStrength` (0): MinReps=5, MaxReps=5
- `PeriodizationHypertrophy` (1): MinReps=6, MaxReps=10

---

## Trigger and Lifecycle

`ResolveWeeklySchedule()` in the service:
1. Compute Monday of the current week.
2. Count existing sessions for the week (Mon–Sun).
3. If count > 0: return existing sessions (no replanning mid-week).
4. If count = 0: call `weekplanner.Plan(monday)`, persist all resulting sessions in a single DB transaction, return them.

Sessions for days outside the plan (e.g. user had 3-day prefs and requests a 4th day) return `ErrSessionNotFound`. The HTTP handlers for `workoutGET` and `workoutStartPOST` render an error page for this error.

---

## Error Handling

`WeeklyPlanner.Plan()` returns errors for:
- `startingDate` is not a Monday
- No exercises exist for a scheduled day's category
- Preferences have no workout days scheduled

Service errors from planning propagate to the home page handler rather than being silently swallowed.

---

## Testing

### `internal/weekplanner/` (pure unit tests — no DB, no context)

- Category determination: standard cases, Sun/Mon/Tue week-wrap, isolated days
- Periodization formula: verified against known week/preference combinations
- Muscle group allocation: correct 2× assignment, most-constrained-first ordering, graceful degradation when sessions have fewer slots than priority groups
- Exercise selection: strict category filtering, compound exercises satisfying multiple priority groups, random tiebreaking seeded for determinism
- `Plan()` error cases: non-Monday start date, empty exercise pool for a category, empty preferences

### `internal/workout/` (service integration tests)

- `ResolveWeeklySchedule()` generates all scheduled days when no sessions exist for the week
- `ResolveWeeklySchedule()` does not regenerate when sessions already exist
- `GetSession()` returns `ErrSessionNotFound` for dates outside the plan

### Existing e2e tests

Session structure is unchanged — only creation path changes. Existing workout start/complete/set flows should pass without modification. Review after implementation.
