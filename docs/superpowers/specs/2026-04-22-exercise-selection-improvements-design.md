# Exercise Selection Improvements: Session Diversity & Week-Level Deduplication

**Date:** 2026-04-22  
**Status:** Design Review

## Problem

The current exercise selection algorithm in the weekly planner can select exercises with overlapping primary muscle groups in the same session (e.g., bench press and dumbbell press, both targeting chest primarily). Additionally, the same exercise can be selected on multiple days within the same week, reducing variety. These issues lead to:

1. **Session-level repetition:** Multiple exercises in one session target the same muscle groups, reducing variety of stimulus
2. **Week-level repetition:** The same exercise appears across different days, reducing overall weekly variety

## Goal

Improve the exercise selection algorithm to:
1. Ensure no two exercises in the same session share a primary muscle group
2. Ensure no exercise is used more than once across the entire week
3. Gracefully degrade when constraints can't be fully satisfied

## Scope

- Modify only the exercise selection phase in `internal/weekplanner/`
- No changes to the data model, schema, or HTTP layer
- No changes to muscle group allocation (Phase 2) or category determination (Phase 1)
- Graceful degradation is silent; no new error messages or logging

---

## Design

### Priority Order (Constraint Hierarchy)

When constraints conflict, this is the priority order (highest to lowest):

1. **Session diversity (highest):** No two exercises in a session can share primary muscle groups
2. **Week-level deduplication (middle):** No exercise can be used twice in the same week
3. **Weekly targets (lowest):** Hit each muscle group ~2x per week (acceptable to miss)

If session diversity and week dedup can't both be satisfied, we relax weekly targets. If only one exercise remains without conflicts, we use it even if other sessions also selected it (violating week dedup).

### Algorithm: Multi-Pass Greedy Selection

**Overview:**
- Before planning, initialize a `weekUsedExercises` set (tracks IDs of exercises selected for any day)
- For each day: select exercises while avoiding (1) primary muscle group overlap within the session and (2) previously-used exercises
- If a priority muscle group can't be satisfied without conflict, skip it (graceful degradation)

**Execution flow:**

1. **Initialize:** `weekUsedExercises := make(map[int]bool)`

2. **For each workout day in the week:**
   - Call `selectExercisesForDayWithPeriodization(..., weekUsedExercises)`
   - This function returns exercises selected for the day
   - Add all returned exercise IDs to `weekUsedExercises`

3. **Inside `selectExercisesForDayWithPeriodization()`:**
   - Maintain `selectedPrimaryMuscles` (set of primary muscle groups already chosen in this session)
   - Filter exercise pool by category (upper/lower/full-body compatibility)
   - **Phase A — Priority muscle groups:** Iterate through each priority muscle group. For each, find the best-scoring exercise that:
     - Has not been used earlier in the week
     - Does not share primary muscle groups with already-selected exercises in this session
     - Covers this priority muscle group
     - If found: select it, mark its primary muscles as satisfied, remove from pool
     - If not found: skip this priority muscle group
   - **Phase B — Fill remaining slots:** Continue selecting non-conflicting exercises until we have N total exercises or run out of compatible exercises

4. **Graceful degradation:**
   - If we can't select N exercises without violating constraints, return fewer than N
   - This is silent; the session is created with however many exercises fit the constraints

### Helper Functions

**`primaryMuscleGroupsOverlap(ex Exercise, selectedPrimaryMuscles map[string]bool) bool`**

Returns true if any of the exercise's primary muscle groups are already in `selectedPrimaryMuscles`.

```go
func primaryMuscleGroupsOverlap(ex Exercise, selectedPrimaryMuscles map[string]bool) bool {
  for _, mg := range ex.PrimaryMuscleGroups {
    if selectedPrimaryMuscles[mg] {
      return true
    }
  }
  return false
}
```

### Modified Function Signatures

**`selectExercisesForDayWithPeriodization()`:**
```go
func (wp *WeeklyPlanner) selectExercisesForDayWithPeriodization(
  category Category,
  priorityMuscleGroups []string,
  n int,
  pt PeriodizationType,
  weekUsedExercises map[int]bool,
) []PlannedExerciseSet
```

New parameter `weekUsedExercises` tracks which exercise IDs have been selected for any day in the current week.

**`Plan()` method:**

Add initialization and passing of `weekUsedExercises`:
```go
weekUsedExercises := make(map[int]bool)

for i, day := range workoutDays {
  // ... existing category/periodization logic ...
  
  exerciseSets := wp.selectExercisesForDayWithPeriodization(
    categories[day],
    dayMuscleGroups[day],
    n,
    pt,
    weekUsedExercises,
  )
  
  // Record which exercises were used
  for _, es := range exerciseSets {
    weekUsedExercises[es.ExerciseID] = true
  }
  
  sessions[i] = PlannedSession{...}
}
```

### Data Model Changes

No schema or type changes. The `weekUsedExercises` map is a temporary runtime structure, not persisted.

---

## Testing

### Unit Tests (`internal/weekplanner/`)

**New test cases for session diversity:**
- "no primary muscle group overlap within session" — select exercises for a day, verify no two selected exercises share primary muscle groups
- "skip muscle group when all compatible exercises conflict" — verify graceful degradation when a priority muscle group can't be added without overlap
- "exercises with same primary but different secondaries are still considered overlapping" — clarify that secondary muscles don't prevent selection, only primary

**New test cases for week-level deduplication:**
- "exercise used on Monday is not reused on Tuesday" — verify `weekUsedExercises` prevents repeats across days
- "exercises not filtered from subsequent days until actually selected" — verify we only track used exercises, not entire filtered pool
- "week wrap: exercises selected for Sunday are unavailable for Monday" (if multi-week planning ever happens)

**Modified test cases:**
- `TestSelectExercisesForDay` — update to pass empty `weekUsedExercises` for backward compatibility
- `TestPlan` — verify full week planning respects both constraints

### Edge Cases

- **No compatible exercises:** If all pool exercises that target a priority muscle group were already used, that group is silently skipped
- **Small exercise pool:** With very few exercises, early days might consume all non-conflicting options, leaving later days with fewer choices
- **Single-session weeks:** Constraints are trivially satisfied (no repeats across days, no overlap conflicts from prior days)

---

## Backward Compatibility

- `selectExercisesForDay()` (parameterless version) remains unchanged; it's a wrapper that calls `selectExercisesForDayWithPeriodization()` with an empty `weekUsedExercises` map
- The output shape (`[]PlannedSession`) is identical
- Existing HTTP handlers, database layer, and frontend require no changes

---

## Success Criteria

1. **Session diversity:** No two exercises in any session share primary muscle groups
2. **Week deduplication:** No exercise ID appears twice across different days in the same week
3. **Graceful degradation:** Plan never fails; if constraints can't be met, fewer exercises are selected
4. **Determinism:** For the same inputs (preferences, exercise pool, start date), the plan is always identical
5. **Performance:** Algorithm completes in < 10ms for typical exercise pools (50–200 exercises, 3–6 workout days)

