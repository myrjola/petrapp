# Exercise Selection Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Modify the weekly planner's exercise selection to ensure session-level muscle group diversity and week-level exercise deduplication.

**Architecture:** Add a `weekUsedExercises` map passed through the selection flow to track which exercises have been selected. Modify the greedy selection algorithm to skip exercises that (1) have been used earlier in the week or (2) share primary muscle groups with already-selected exercises in the current session. Graceful degradation: if N exercises can't be selected without conflicts, return fewer.

**Tech Stack:** Go, standard library (no new dependencies)

---

## Task 1: Add `primaryMuscleGroupsOverlap()` helper function

**Files:**
- Modify: `internal/weekplanner/weekplanner.go` (add function after `scoreExercise()`)

- [ ] **Step 1: Add the helper function to weekplanner.go**

Open `internal/weekplanner/weekplanner.go` and add this function after `scoreExercise()` (around line 315):

```go
// primaryMuscleGroupsOverlap returns true if any of the exercise's primary muscle groups
// are already in the selectedPrimaryMuscles set.
func primaryMuscleGroupsOverlap(ex Exercise, selectedPrimaryMuscles map[string]bool) bool {
	for _, mg := range ex.PrimaryMuscleGroups {
		if selectedPrimaryMuscles[mg] {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify no regressions**

```bash
cd /Users/personal/air/petrapp
go test ./internal/weekplanner -v
```

Expected: All existing tests pass (no changes to behavior yet).

- [ ] **Step 3: Commit**

```bash
git add internal/weekplanner/weekplanner.go
git commit -m "feat: add primaryMuscleGroupsOverlap helper function"
```

---

## Task 2: Update `selectExercisesForDay()` to pass empty weekUsedExercises

**Files:**
- Modify: `internal/weekplanner/weekplanner.go` (around line 319-325)

- [ ] **Step 1: Update the wrapper function**

Find `selectExercisesForDay()` (around line 319) and update it:

```go
func (wp *WeeklyPlanner) selectExercisesForDay(
	category Category,
	priorityMuscleGroups []string,
	n int,
) []PlannedExerciseSet {
	return wp.selectExercisesForDayWithPeriodization(
		category,
		priorityMuscleGroups,
		n,
		PeriodizationStrength,
		make(map[int]bool),  // NEW: pass empty weekUsedExercises
	)
}
```

- [ ] **Step 2: Run tests to verify no breakage**

```bash
cd /Users/personal/air/petrapp
go test ./internal/weekplanner -v
```

Expected: Tests still pass. This is a backward-compatible change (existing callers work).

- [ ] **Step 3: Commit**

```bash
git add internal/weekplanner/weekplanner.go
git commit -m "feat: pass empty weekUsedExercises map to selectExercisesForDayWithPeriodization"
```

---

## Task 3: Update `selectExercisesForDayWithPeriodization()` signature

**Files:**
- Modify: `internal/weekplanner/weekplanner.go` (around line 327)

- [ ] **Step 1: Add weekUsedExercises parameter to the function signature**

Find `selectExercisesForDayWithPeriodization()` (around line 327) and update the signature:

```go
func (wp *WeeklyPlanner) selectExercisesForDayWithPeriodization(
	category Category,
	priorityMuscleGroups []string,
	n int,
	pt PeriodizationType,
	weekUsedExercises map[int]bool,  // NEW parameter
) []PlannedExerciseSet {
```

Note: Do NOT change the function body yet — that happens in Task 4.

- [ ] **Step 2: Run tests**

```bash
cd /Users/personal/air/petrapp
go test ./internal/weekplanner -v
```

Expected: Tests pass (body hasn't changed yet).

- [ ] **Step 3: Commit**

```bash
git add internal/weekplanner/weekplanner.go
git commit -m "feat: add weekUsedExercises parameter to selectExercisesForDayWithPeriodization"
```

---

## Task 4: Implement new selection logic with session diversity and week deduplication

**Files:**
- Modify: `internal/weekplanner/weekplanner.go` (lines 332-390, the entire function body)

- [ ] **Step 1: Replace the selectExercisesForDayWithPeriodization() body**

Replace lines 332-390 with this new implementation:

```go
	// Filter exercise pool by category compatibility.
	pool := make([]Exercise, 0, len(wp.Exercises))
	for _, ex := range wp.Exercises {
		if isCategoryCompatible(ex.Category, category) {
			pool = append(pool, ex)
		}
	}

	selectedPrimaryMuscles := make(map[string]bool)
	var selected []Exercise

	// Phase A: Try to satisfy each priority muscle group with a non-conflicting exercise.
	for _, priorityMG := range priorityMuscleGroups {
		if selectedPrimaryMuscles[priorityMG] {
			continue // Already satisfied by a previous exercise.
		}

		// Find best-scoring exercise that targets this priority muscle group
		// and doesn't conflict.
		bestScore := -1
		var bestCandidate *Exercise

		for i := range pool {
			ex := &pool[i]

			// Skip if used earlier in the week.
			if weekUsedExercises[ex.ID] {
				continue
			}

			// Skip if primary muscles overlap with this session's selected muscles.
			if primaryMuscleGroupsOverlap(*ex, selectedPrimaryMuscles) {
				continue
			}

			// Must target this priority muscle group.
			if !slices.Contains(ex.PrimaryMuscleGroups, priorityMG) {
				continue
			}

			// Score it.
			score := 0
			for _, mg := range ex.PrimaryMuscleGroups {
				if slices.Contains(priorityMuscleGroups, mg) && !selectedPrimaryMuscles[mg] {
					score++
				}
			}

			if score > bestScore {
				bestScore = score
				bestCandidate = ex
			}
		}

		// If a candidate was found, select it.
		if bestCandidate != nil {
			selected = append(selected, *bestCandidate)
			for _, mg := range bestCandidate.PrimaryMuscleGroups {
				selectedPrimaryMuscles[mg] = true
			}

			// Remove from pool to avoid selecting the same exercise twice in this session.
			newPool := make([]Exercise, 0, len(pool))
			for i := range pool {
				if pool[i].ID != bestCandidate.ID {
					newPool = append(newPool, pool[i])
				}
			}
			pool = newPool
		}
		// If no candidate found, this priority muscle group is skipped (graceful degradation).
	}

	// Phase B: Fill remaining slots with any non-conflicting exercise from the pool.
	for len(selected) < n && len(pool) > 0 {
		// Find best-scoring exercise among remaining pool.
		bestScore := -1
		bestIdx := -1

		for i := range pool {
			ex := &pool[i]

			// Skip if used earlier in the week.
			if weekUsedExercises[ex.ID] {
				continue
			}

			// Skip if primary muscles overlap.
			if primaryMuscleGroupsOverlap(*ex, selectedPrimaryMuscles) {
				continue
			}

			// Score by how many priority muscle groups it covers.
			score := 0
			for _, mg := range ex.PrimaryMuscleGroups {
				if slices.Contains(priorityMuscleGroups, mg) && !selectedPrimaryMuscles[mg] {
					score++
				}
			}

			if score > bestScore || (score == bestScore && bestIdx == -1) {
				bestScore = score
				bestIdx = i
			}
		}

		// If no qualifying exercise found, we're done (no more non-conflicting exercises).
		if bestIdx == -1 {
			break
		}

		// Select it.
		bestCandidate := pool[bestIdx]
		selected = append(selected, bestCandidate)
		for _, mg := range bestCandidate.PrimaryMuscleGroups {
			selectedPrimaryMuscles[mg] = true
		}

		// Remove from pool.
		pool = append(pool[:bestIdx], pool[bestIdx+1:]...)
	}

	// Build PlannedExerciseSets.
	minR, maxR := setsForPeriodization(pt)
	sets := make([]PlannedSet, setsPerExercise)
	for i := range sets {
		sets[i] = PlannedSet{MinReps: minR, MaxReps: maxR}
	}

	result := make([]PlannedExerciseSet, len(selected))
	for i, ex := range selected {
		result[i] = PlannedExerciseSet{
			ExerciseID: ex.ID,
			Sets:       slices.Clone(sets),
		}
	}
	return result
```

- [ ] **Step 2: Run tests to verify existing tests still pass**

```bash
cd /Users/personal/air/petrapp
go test ./internal/weekplanner -v
```

Expected: All existing tests pass. The new constraints don't affect tests that use empty `weekUsedExercises`.

- [ ] **Step 3: Commit**

```bash
git add internal/weekplanner/weekplanner.go
git commit -m "feat: implement session diversity and week-level deduplication in exercise selection"
```

---

## Task 5: Update Plan() to initialize and track weekUsedExercises

**Files:**
- Modify: `internal/weekplanner/weekplanner.go` (around line 439-456, the Phase 3 loop in Plan())

- [ ] **Step 1: Update Plan() to initialize and pass weekUsedExercises**

Find the Phase 3 comment in `Plan()` (around line 439) and replace the exercise selection loop (lines 440-450) with:

```go
	// Phase 3: select exercises and build sessions.
	weekUsedExercises := make(map[int]bool)
	sessions := make([]PlannedSession, len(workoutDays))
	for i, day := range workoutDays {
		pt := PeriodizationType((int(firstPT) + i) % numPeriodizationTypes)
		n := wp.Prefs.ExercisesPerSession(day.Weekday())
		exerciseSets := wp.selectExercisesForDayWithPeriodization(
			categories[day],
			dayMuscleGroups[day],
			n,
			pt,
			weekUsedExercises,
		)

		// Record which exercises were used this week.
		for _, es := range exerciseSets {
			weekUsedExercises[es.ExerciseID] = true
		}

		sessions[i] = PlannedSession{
			Date:              day,
			Category:          categories[day],
			PeriodizationType: pt,
			ExerciseSets:      exerciseSets,
		}
	}
```

- [ ] **Step 2: Run tests**

```bash
cd /Users/personal/air/petrapp
go test ./internal/weekplanner -v
```

Expected: All tests pass. The logic is complete.

- [ ] **Step 3: Commit**

```bash
git add internal/weekplanner/weekplanner.go
git commit -m "feat: initialize and track weekUsedExercises in Plan()"
```

---

## Task 6: Write test for session-level muscle group diversity

**Files:**
- Modify: `internal/weekplanner/weekplanner_internal_test.go` (add new test after TestSelectExercisesForDay)

- [ ] **Step 1: Write the test**

Add this test after the `TestSelectExercisesForDay` function (around line 270+):

```go
func TestSelectExercisesForDaySessionDiversity(t *testing.T) {
	t.Run("no primary muscle group overlap within session", func(t *testing.T) {
		// Exercise pool: multiple exercises that could target overlapping muscles.
		exercises := []Exercise{
			{ID: 1, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: []string{"Triceps"}},
			{ID: 2, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: []string{"Shoulders"}},
			{ID: 3, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Shoulders", "Triceps"}, SecondaryMuscleGroups: nil},
			{ID: 4, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Triceps"}, SecondaryMuscleGroups: nil},
		}

		p := Preferences{MondayMinutes: 60} // 3 exercises
		wp := NewWeeklyPlanner(p, exercises, nil)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		// Request 3 exercises with priority Chest, Shoulders, Triceps.
		// Expected: one exercise per primary muscle group, no overlaps.
		sets := wp.selectExercisesForDayWithPeriodization(
			CategoryChest,
			[]string{"Chest", "Shoulders", "Triceps"},
			3,
			PeriodizationStrength,
			make(map[int]bool), // Empty week-used set
		)

		if len(sets) < 2 {
			t.Fatalf("want at least 2 exercises, got %d", len(sets))
		}

		// Collect all primary muscle groups across selected exercises.
		seenPrimary := make(map[string]bool)
		for _, es := range sets {
			ex := findExercise(exercises, es.ExerciseID)
			for _, mg := range ex.PrimaryMuscleGroups {
				if seenPrimary[mg] {
					t.Errorf("primary muscle group %q appears in multiple exercises in the same session", mg)
				}
				seenPrimary[mg] = true
			}
		}
	})

	t.Run("skip priority muscle group when no non-conflicting exercise available", func(t *testing.T) {
		// Exercise pool: all Chest exercises have overlapping primary muscles.
		exercises := []Exercise{
			{ID: 1, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil},
			{ID: 2, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest", "Triceps"}, SecondaryMuscleGroups: nil},
			{ID: 3, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest", "Shoulders"}, SecondaryMuscleGroups: nil},
		}

		p := Preferences{MondayMinutes: 60} // 3 exercises
		wp := NewWeeklyPlanner(p, exercises, nil)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		// Request 3 exercises, but only 1 non-overlapping is available.
		// Expected: graceful degradation — select 1 exercise covering Chest.
		sets := wp.selectExercisesForDayWithPeriodization(
			CategoryChest,
			[]string{"Chest", "Shoulders", "Triceps"}, // Three priorities, but can't all be satisfied.
			3,
			PeriodizationStrength,
			make(map[int]bool),
		)

		if len(sets) == 0 {
			t.Fatalf("want at least 1 exercise, got 0")
		}

		// Should select 1 exercise (Chest), then can't add more without overlap.
		if len(sets) > 1 {
			// Check that no primary muscle groups repeat.
			seenPrimary := make(map[string]bool)
			for _, es := range sets {
				ex := findExercise(exercises, es.ExerciseID)
				for _, mg := range ex.PrimaryMuscleGroups {
					if seenPrimary[mg] {
						t.Errorf("primary muscle group %q appears twice; expected graceful degradation to 1 exercise", mg)
					}
					seenPrimary[mg] = true
				}
			}
		}
	})
}
```

- [ ] **Step 2: Run the new tests**

```bash
cd /Users/personal/air/petrapp
go test ./internal/weekplanner -v -run TestSelectExercisesForDaySessionDiversity
```

Expected: Both test cases PASS. The implementation handles session diversity correctly.

- [ ] **Step 3: Commit**

```bash
git add internal/weekplanner/weekplanner_internal_test.go
git commit -m "test: add session diversity test cases"
```

---

## Task 7: Write test for week-level exercise deduplication

**Files:**
- Modify: `internal/weekplanner/weekplanner_internal_test.go` (add new test)

- [ ] **Step 1: Write the deduplication test**

Add this test after `TestSelectExercisesForDaySessionDiversity`:

```go
func TestSelectExercisesForDayWeekDeduplication(t *testing.T) {
	t.Run("exercise used earlier in week is skipped", func(t *testing.T) {
		exercises := []Exercise{
			{ID: 1, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil},
			{ID: 2, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil},
			{ID: 3, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Triceps"}, SecondaryMuscleGroups: nil},
		}

		p := Preferences{MondayMinutes: 60}
		wp := NewWeeklyPlanner(p, exercises, nil)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		// Simulate that exercise 1 was already used earlier in the week.
		weekUsedExercises := map[int]bool{1: true}

		// Request exercises with Chest priority, but exercise 1 (Chest) is already used.
		// Expected: select exercise 2 (Shoulders) or 3 (Triceps) instead.
		sets := wp.selectExercisesForDayWithPeriodization(
			CategoryChest,
			[]string{"Chest"},
			1,
			PeriodizationStrength,
			weekUsedExercises,
		)

		if len(sets) == 0 {
			t.Fatalf("want 1 exercise, got 0")
		}

		selectedID := sets[0].ExerciseID
		if selectedID == 1 {
			t.Errorf("exercise 1 was already used this week; expected a different exercise, got %d", selectedID)
		}
	})

	t.Run("plan() does not repeat exercises across days", func(t *testing.T) {
		exercises := minimalExercises() // Use existing test fixture
		targets := minimalTargets()

		monday := monday2026Date()
		p := prefs(time.Monday, time.Tuesday, time.Thursday)
		wp := NewWeeklyPlanner(p, exercises, targets)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		sessions, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan failed: %v", err)
		}

		// Collect all exercise IDs across all sessions.
		usedExercises := make(map[int]bool)
		for _, session := range sessions {
			for _, es := range session.ExerciseSets {
				if usedExercises[es.ExerciseID] {
					t.Errorf("exercise %d appears in multiple sessions across the week", es.ExerciseID)
				}
				usedExercises[es.ExerciseID] = true
			}
		}

		// Verify that we have more than one session (to make the test meaningful).
		if len(sessions) < 2 {
			t.Logf("note: only %d session(s) planned, test less meaningful", len(sessions))
		}
	})
}
```

- [ ] **Step 2: Run the new tests**

```bash
cd /Users/personal/air/petrapp
go test ./internal/weekplanner -v -run TestSelectExercisesForDayWeekDeduplication
```

Expected: Both test cases PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/weekplanner/weekplanner_internal_test.go
git commit -m "test: add week-level deduplication test cases"
```

---

## Task 8: Write test for graceful degradation

**Files:**
- Modify: `internal/weekplanner/weekplanner_internal_test.go` (add new test)

- [ ] **Step 1: Write the graceful degradation test**

Add this test after `TestSelectExercisesForDayWeekDeduplication`:

```go
func TestSelectExercisesForDayGracefulDegradation(t *testing.T) {
	t.Run("returns fewer exercises if constraints can't be fully satisfied", func(t *testing.T) {
		// Exercise pool: only 2 non-overlapping exercises available.
		exercises := []Exercise{
			{ID: 1, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil},
			{ID: 2, Category: CategoryChest, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil},
		}

		p := Preferences{MondayMinutes: 60} // Requests 3 exercises
		wp := NewWeeklyPlanner(p, exercises, nil)
		wp.rng = rand.New(rand.NewPCG(42, 0))

		// Request 3 exercises, but only 2 non-overlapping available.
		// Expected: plan succeeds with 2 exercises, no error.
		sets := wp.selectExercisesForDayWithPeriodization(
			CategoryChest,
			[]string{"Chest", "Shoulders", "Triceps"},
			3,
			PeriodizationStrength,
			make(map[int]bool),
		)

		if len(sets) != 2 {
			t.Errorf("want 2 exercises (graceful degradation), got %d", len(sets))
		}

		// Verify the 2 selected have no overlapping primary muscles.
		seenPrimary := make(map[string]bool)
		for _, es := range sets {
			ex := findExercise(exercises, es.ExerciseID)
			for _, mg := range ex.PrimaryMuscleGroups {
				if seenPrimary[mg] {
					t.Errorf("primary muscle group %q appears twice", mg)
				}
				seenPrimary[mg] = true
			}
		}
	})
}
```

- [ ] **Step 2: Run the new test**

```bash
cd /Users/personal/air/petrapp
go test ./internal/weekplanner -v -run TestSelectExercisesForDayGracefulDegradation
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/weekplanner/weekplanner_internal_test.go
git commit -m "test: add graceful degradation test case"
```

---

## Task 9: Run full test suite and verify no regressions

**Files:**
- No changes; verification only

- [ ] **Step 1: Run all weekplanner tests**

```bash
cd /Users/personal/air/petrapp
go test ./internal/weekplanner -v
```

Expected: All tests PASS (both old and new).

- [ ] **Step 2: Run the full project test suite**

```bash
cd /Users/personal/air/petrapp
make test
```

Expected: All tests pass, no regressions in other packages.

- [ ] **Step 3: Commit (if needed)**

If the full test suite passes without changes, no commit is needed. If you had to fix something, commit it:

```bash
git commit -m "fix: resolve test regression in [package]"
```

---

## Task 10: Update design spec with implementation notes

**Files:**
- Modify: `docs/superpowers/specs/2026-04-22-exercise-selection-improvements-design.md`

- [ ] **Step 1: Add an implementation section to the spec**

Open the design spec and add this section after the "Testing" section:

```markdown
## Implementation Status

**Completed:** 2026-04-22

### Changes Made

1. Added `primaryMuscleGroupsOverlap()` helper function to detect primary muscle group conflicts
2. Modified `selectExercisesForDayWithPeriodization()` signature to accept `weekUsedExercises` parameter
3. Implemented two-phase selection algorithm:
   - Phase A: Satisfy priority muscle groups with non-conflicting exercises
   - Phase B: Fill remaining slots with any non-conflicting exercise
4. Updated `Plan()` to initialize and track `weekUsedExercises` across the week
5. Updated wrapper function `selectExercisesForDay()` to pass empty `weekUsedExercises`

### Test Coverage

- Session-level diversity: 2 test cases (overlap detection, graceful degradation)
- Week-level deduplication: 2 test cases (skip used exercises, no repeats across week)
- Graceful degradation: 1 test case (returns fewer exercises if constraints unsatisfiable)
- Backward compatibility: All existing tests pass unchanged

### Files Modified

- `internal/weekplanner/weekplanner.go`: 4 changes (helper, signatures, implementation, Plan update)
- `internal/weekplanner/weekplanner_internal_test.go`: 3 new test functions
```

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-04-22-exercise-selection-improvements-design.md
git commit -m "docs: update design spec with implementation notes"
```

---

## Summary

This plan implements the exercise selection improvements in 10 focused tasks:

1. **Helper function** — Add overlap detection
2-5. **Core algorithm** — Update signatures and implement new selection logic with week-level dedup
6-8. **Testing** — Write comprehensive tests for all three constraints (session diversity, week dedup, graceful degradation)
9. **Verification** — Full test suite pass
10. **Documentation** — Update spec

**Key design decisions:**
- Two-phase selection: priority groups first, then fill remaining slots
- Graceful degradation: fewer exercises selected if constraints unsatisfiable, no error thrown
- Deterministic: same inputs produce same output (uses existing seeded RNG)
- Backward compatible: existing tests pass, wrapper function maintains old API

---

Plan complete and saved to `docs/superpowers/plans/2026-04-22-exercise-selection-improvements.md`.

Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?