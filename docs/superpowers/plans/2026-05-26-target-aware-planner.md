# Target-Aware Weekly Planner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the priority-MG allocation + two-phase exercise selection in `internal/domain/planner.go` with a single target-aware greedy scoring loop, so weekly plans stop over-filling Shoulders/Triceps/Upper Back while leaving Chest/Quads short.

**Architecture:** Each candidate exercise is scored by how much it reduces the sum of squared distances between the running per-MG weighted load and its target. The planner walks scheduled days Mon→Sun, picking the highest-scoring eligible candidate for each slot while updating a shared `load[mg]` tally across the week. `allocateMuscleGroups` and the Phase A/Phase B split are deleted; the score naturally handles balancing.

**Tech Stack:** Go 1.26, `internal/domain` (pure — no SQL/HTTP/logger), tests in `package domain` (`*_internal_test.go`).

**Spec:** [`docs/superpowers/specs/2026-05-26-target-aware-planner-design.md`](../specs/2026-05-26-target-aware-planner-design.md)

---

## File map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/domain/muscle_group.go` | modify | add `WeeklyPlannedLoad` helper |
| `internal/domain/muscle_group_test.go` | modify | tests for `WeeklyPlannedLoad` |
| `internal/domain/planner.go` | rewrite | add `scoreCandidate`, rewrite selection, remove allocation |
| `internal/domain/planner_internal_test.go` | modify | replace allocation/priority tests; new scoring tests; new regression test |
| `internal/domain/planner_plan_day_internal_test.go` | modify | adopt new `PlanDay` signature; add load-aware test |
| `internal/service/sessions.go` | modify | compute & pass `weekLoad` to `PlanDay` |

No new files. Each test set lives with the code it tests.

---

## Conventions for this plan

- Run `make test` after each implementation step that adds or changes code. The command is `make test` (defined in `Makefile` as `go test --race --shuffle=on ./...`); for tighter loops use `go test -v ./internal/domain -run <TestName>`.
- Comments end with a period (`.golangci.yml`).
- Use `new(v)` (Go 1.26 generic builtin) for `*int` literals in test fixtures, matching existing style in `planner_internal_test.go`.
- `//nolint:exhaustruct` comments on `Exercise{...}` literals in tests follow the existing convention; copy them verbatim.
- Each task ends with a commit. Commit message style: `<type>: <imperative>` matching recent log (`fix:`, `refactor:`, `docs:`, `schema:`). Use `feat:` for new helpers and `refactor:` for the planner rewrite.

---

## Task 1: Add `WeeklyPlannedLoad` helper

**Why:** `PlanDay` will need to seed its scoring with the running weighted-load from already-planned sessions in the same week. The existing `WeeklyMuscleGroupVolume` returns a slice of `MuscleGroupVolume` joined with targets; the planner just needs a `map[string]float64` of planned weighted load. Extract that as a small pure helper that both planning and the existing volume aggregation can build on.

**Files:**
- Modify: `internal/domain/muscle_group.go` (after `WeeklyMuscleGroupVolume`, before `aggregateMuscleGroupLoad`)
- Test: `internal/domain/muscle_group_test.go`

- [ ] **Step 1.1: Write the failing test**

Add to `internal/domain/muscle_group_test.go`:

```go
func TestWeeklyPlannedLoad(t *testing.T) {
	t.Parallel()

	bench := Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID:                    1,
		Name:                  "Bench Press",
		Category:              CategoryUpper,
		ExerciseType:          ExerciseTypeWeighted,
		PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
		SecondaryMuscleGroups: []string{"Shoulders"},
	}
	pulldown := Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID:                    2,
		Name:                  "Pulldown",
		Category:              CategoryUpper,
		ExerciseType:          ExerciseTypeWeighted,
		PrimaryMuscleGroups:   []string{"Lats"},
		SecondaryMuscleGroups: []string{"Biceps", "Shoulders"},
	}

	// One session with two exercises: bench 4 sets, pulldown 3 sets.
	session := Session{ //nolint:exhaustruct // Rest fields unused in this test.
		Slots: []ExerciseSlot{
			{Exercise: bench, Sets: make([]Set, 4)},    //nolint:exhaustruct // Sets carry no values in this test.
			{Exercise: pulldown, Sets: make([]Set, 3)}, //nolint:exhaustruct // Sets carry no values in this test.
		},
	}

	got := WeeklyPlannedLoad([]Session{session})

	want := map[string]float64{
		"Chest":     4.0,        // 4 × 1.0 primary
		"Triceps":   4.0,        // 4 × 1.0 primary
		"Shoulders": 2.0 + 1.5,  // bench secondary 4×0.5 + pulldown secondary 3×0.5
		"Lats":      3.0,        // 3 × 1.0 primary
		"Biceps":    1.5,        // 3 × 0.5 secondary
	}
	for mg, w := range want {
		if got[mg] != w {
			t.Errorf("load[%q] = %v, want %v", mg, got[mg], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d MGs, want %d (extra entries: %v)", len(got), len(want), diffKeys(got, want))
	}
}

func diffKeys(got map[string]float64, want map[string]float64) []string {
	var extra []string
	for k := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	return extra
}
```

- [ ] **Step 1.2: Verify it fails**

Run: `go test -v ./internal/domain -run TestWeeklyPlannedLoad`
Expected: `undefined: WeeklyPlannedLoad`.

- [ ] **Step 1.3: Implement the helper**

Add to `internal/domain/muscle_group.go` immediately after `WeeklyMuscleGroupVolume`:

```go
// WeeklyPlannedLoad returns the running planned weighted load per
// muscle group across the supplied sessions. Each set in the plan
// contributes PrimarySetWeight to every primary muscle group on its
// exercise and SecondarySetWeight to every secondary. Muscle groups
// with zero contributions do not appear in the map. The result is the
// running tally the target-aware planner uses to score subsequent
// picks against the configured weekly targets.
func WeeklyPlannedLoad(sessions []Session) map[string]float64 {
	load := make(map[string]float64)
	for _, sess := range sessions {
		for _, ex := range sess.Slots {
			n := float64(len(ex.Sets))
			for _, mg := range ex.Exercise.PrimaryMuscleGroups {
				load[mg] += n * PrimarySetWeight
			}
			for _, mg := range ex.Exercise.SecondaryMuscleGroups {
				load[mg] += n * SecondarySetWeight
			}
		}
	}
	return load
}
```

- [ ] **Step 1.4: Verify it passes**

Run: `go test -v ./internal/domain -run TestWeeklyPlannedLoad`
Expected: `PASS`.

- [ ] **Step 1.5: Run full domain tests**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 1.6: Commit**

```bash
git add internal/domain/muscle_group.go internal/domain/muscle_group_test.go
git commit -m "feat(domain): add WeeklyPlannedLoad helper

Pure helper that returns planned weighted load per muscle group across
a slice of sessions, sharing the primary/secondary weighting with
WeeklyMuscleGroupVolume. Will seed PlanDay's target-aware scoring with
the current week's already-planned load."
```

---

## Task 2: Add `scoreCandidate` helper

**Why:** Pure scoring function used by the new selection loop. Extracting it as a standalone unit lets us assert the score behavior (positive when pulling toward target, negative when pushing over, zero when no targeted MG is touched) without exercising the full planner.

**Files:**
- Modify: `internal/domain/planner.go` (add at the bottom, near the other small helpers like `nextPeriodizationType`)
- Test: `internal/domain/planner_internal_test.go`

- [ ] **Step 2.1: Write the failing tests**

Add to `internal/domain/planner_internal_test.go` (near the existing `Test_exercisesPerSession_PeriodizationAware` block):

```go
func Test_scoreCandidate(t *testing.T) {
	t.Parallel()

	bench := Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
		ID:                    1,
		Category:              CategoryUpper,
		ExerciseType:          ExerciseTypeWeighted,
		PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
		SecondaryMuscleGroups: []string{"Shoulders"},
		RepMin:                new(5), RepMax: new(10),
	}

	targets := map[string]int{"Chest": 10, "Triceps": 8, "Shoulders": 10}

	t.Run("positive when pulling under-target MGs up", func(t *testing.T) {
		t.Parallel()
		// Empty load: every targeted MG at full deficit.
		load := map[string]float64{}
		// Strength + 5-10 window: reps=5, sets=4 (DeriveScheme low band).
		score := scoreCandidate(bench, PeriodizationStrength, false, load, targets)
		// Chest: before=10, after=10-4=6; contribution 100-36=64.
		// Triceps: before=8, after=8-4=4; contribution 64-16=48.
		// Shoulders: before=10, after=10-2=8; contribution 100-64=36.
		// Total = 64 + 48 + 36 = 148.
		if score != 148 {
			t.Errorf("score = %v, want 148", score)
		}
	})

	t.Run("negative when pushing on-target MG further over", func(t *testing.T) {
		t.Parallel()
		// Shoulders already at 12 (2 over target 10); other MGs at target.
		load := map[string]float64{"Chest": 10, "Triceps": 8, "Shoulders": 12}
		score := scoreCandidate(bench, PeriodizationStrength, false, load, targets)
		// Chest: before=0, after=-4; 0-16=-16.
		// Triceps: before=0, after=-4; 0-16=-16.
		// Shoulders: before=-2, after=-4; 4-16=-12.
		// Total = -16 + -16 + -12 = -44.
		if score != -44 {
			t.Errorf("score = %v, want -44", score)
		}
	})

	t.Run("zero when no targeted MG is touched", func(t *testing.T) {
		t.Parallel()
		calfRaise := Exercise{ //nolint:exhaustruct // Test exercise omits display fields.
			ID:                    99,
			Category:              CategoryLower,
			ExerciseType:          ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Calves"},
			SecondaryMuscleGroups: nil,
			RepMin:                new(10), RepMax: new(20),
		}
		load := map[string]float64{}
		score := scoreCandidate(calfRaise, PeriodizationStrength, false, load, targets)
		if score != 0 {
			t.Errorf("score = %v, want 0", score)
		}
	})

	t.Run("deload halves set count", func(t *testing.T) {
		t.Parallel()
		load := map[string]float64{}
		// Strength + deload + 5-10 window: reps=10 (deload forces hypertrophy),
		// base sets = 3 (mid band, 6 <= reps <= 10), halved to 2.
		score := scoreCandidate(bench, PeriodizationStrength, true, load, targets)
		// Chest: 100 - (10-2)^2 = 100 - 64 = 36.
		// Triceps: 64 - (8-2)^2 = 64 - 36 = 28.
		// Shoulders: 100 - (10-1)^2 = 100 - 81 = 19.
		// Total = 36 + 28 + 19 = 83.
		if score != 83 {
			t.Errorf("score = %v, want 83", score)
		}
	})
}
```

- [ ] **Step 2.2: Verify failure**

Run: `go test -v ./internal/domain -run Test_scoreCandidate`
Expected: `undefined: scoreCandidate`.

- [ ] **Step 2.3: Implement `scoreCandidate`**

Add to `internal/domain/planner.go` (after `nextPeriodizationType`):

```go
// scoreCandidate returns the gain in target-balance from adding ex to a
// session: positive when the exercise pulls the running per-MG load
// closer to its target, negative when it pushes an MG further from
// target. The metric is the change in the sum of squared distances
// over targeted muscle groups; untargeted MGs are ignored. Set count
// comes from the same deriveSchemeForExercise the planner uses to
// persist sets, so deload halving and periodization-driven set-count
// shifts are reflected automatically.
func scoreCandidate(
	ex Exercise,
	pt PeriodizationType,
	isDeload bool,
	load map[string]float64,
	targets map[string]int,
) float64 {
	_, nSets := deriveSchemeForExercise(ex, pt, isDeload)
	n := float64(nSets)
	contrib := make(map[string]float64, len(ex.PrimaryMuscleGroups)+len(ex.SecondaryMuscleGroups))
	for _, mg := range ex.PrimaryMuscleGroups {
		contrib[mg] += n * PrimarySetWeight
	}
	for _, mg := range ex.SecondaryMuscleGroups {
		contrib[mg] += n * SecondarySetWeight
	}
	var delta float64
	for mg, target := range targets {
		before := float64(target) - load[mg]
		after := before - contrib[mg]
		delta += before*before - after*after
	}
	return delta
}
```

- [ ] **Step 2.4: Verify it passes**

Run: `go test -v ./internal/domain -run Test_scoreCandidate`
Expected: all four subtests PASS.

- [ ] **Step 2.5: Commit**

```bash
git add internal/domain/planner.go internal/domain/planner_internal_test.go
git commit -m "feat(domain): add scoreCandidate scoring helper

Pure helper computing the change in sum-of-squared-distances when an
exercise is added to a session's running per-MG load. Positive scores
mean the pick pulls targeted MGs closer to their target; negative
scores mean it pushes one further over. Used by the upcoming
target-aware selection loop."
```

---

## Task 3: Rewrite `selectExercisesForDayWithPeriodization`

**Why:** Collapse Phase A / Phase B / priority-MG plumbing into a single greedy scoring loop. Each pick mutates the shared `load` map so subsequent picks (within the same session and across later days) react to what's already been allocated.

**Files:**
- Modify: `internal/domain/planner.go` (rewrite the function ~lines 421-472; also delete the `selectExercisesForDay` convenience wrapper at lines 405-419)
- Test: `internal/domain/planner_internal_test.go` (replace existing `TestSelectExercisesForDay*` tests with new ones aligned to the new signature)

- [ ] **Step 3.1: Delete the obsolete `selectExercisesForDay` convenience wrapper**

In `internal/domain/planner.go`, delete the function defined at the current location of `selectExercisesForDay` (currently lines 405–419, the Strength-default wrapper). The new signature requires `load` and a `targets` lookup which the wrapper can't provide cleanly; its only caller is the existing tests we're rewriting in this task.

- [ ] **Step 3.2: Replace the body of `selectExercisesForDayWithPeriodization`**

Replace the entire function (currently lines 421–472) with the new implementation:

```go
// selectExercisesForDayWithPeriodization picks up to n category-compatible
// exercises for a session, mutating load with each pick's primary
// (PrimarySetWeight) and secondary (SecondarySetWeight) contributions
// and marking each picked exercise's ID in weekUsedExercises so later
// days in the same week skip it. The chosen exercise on every slot is
// the one that maximises scoreCandidate against the current load and
// the planner's Targets, with the lowest exercise ID winning ties.
// Within a session, exercises whose primary MGs overlap with already
// selected primaries are skipped (no two chest-primary picks in one
// session). When no eligible candidate remains, selection stops early
// (graceful degradation: the session may have fewer than n slots).
func (wp *Planner) selectExercisesForDayWithPeriodization(
	category Category,
	n int,
	pt PeriodizationType,
	isDeload bool,
	weekUsedExercises map[int]bool,
	load map[string]float64,
) []ExerciseSlot {
	targets := make(map[string]int, len(wp.Targets))
	for _, t := range wp.Targets {
		targets[t.MuscleGroupName] = t.WeeklySetTarget
	}

	selectedPrimaryMGs := make(map[string]bool)
	selected := make([]ExerciseSlot, 0, n)

	for len(selected) < n {
		bestIdx := -1
		bestScore := 0.0
		for i := range wp.Exercises {
			ex := wp.Exercises[i]
			if !isCategoryCompatible(ex.Category, category) {
				continue
			}
			if weekUsedExercises[ex.ID] {
				continue
			}
			if primaryMuscleGroupsOverlap(ex, selectedPrimaryMGs) {
				continue
			}
			score := scoreCandidate(ex, pt, isDeload, load, targets)
			if bestIdx < 0 || score > bestScore {
				bestIdx = i
				bestScore = score
			}
		}
		if bestIdx < 0 {
			break
		}
		ex := wp.Exercises[bestIdx]
		slot := buildPlannedExerciseSlot(ex, pt, isDeload)
		selected = append(selected, slot)
		for _, mg := range ex.PrimaryMuscleGroups {
			selectedPrimaryMGs[mg] = true
		}
		weekUsedExercises[ex.ID] = true
		nSets := float64(len(slot.Sets))
		for _, mg := range ex.PrimaryMuscleGroups {
			load[mg] += nSets * PrimarySetWeight
		}
		for _, mg := range ex.SecondaryMuscleGroups {
			load[mg] += nSets * SecondarySetWeight
		}
	}

	return selected
}
```

- [ ] **Step 3.3: Delete obsolete helpers no longer referenced**

In `internal/domain/planner.go`:

- Delete `scoreExerciseForPriority` (currently lines 357–366).
- Delete `findBestExerciseInPool` (currently lines 370–392).
- Delete `selectAndRemoveFromPool` (currently lines 395–402).

Keep `primaryMuscleGroupsOverlap` (still used to prevent intra-session primary overlap).

- [ ] **Step 3.4: Replace `TestSelectExercisesForDay` block**

Delete the existing `TestSelectExercisesForDay`, `TestSelectExercisesForDaySessionDiversity`, `TestSelectExercisesForDayWeekDeduplication`, `TestSelectExercisesForDayGracefulDegradation`, and `TestSelectExercisesForDay_TimeBasedTarget` functions in `internal/domain/planner_internal_test.go`. They test the old signature. Replace them with:

```go
func TestSelectExercises_CategoryFilter(t *testing.T) {
	t.Parallel()

	p := prefs(time.Tuesday)
	wp := NewPlanner(p, minimalExercises(), minimalTargets())

	t.Run("lower day only selects lower exercises", func(t *testing.T) {
		t.Parallel()
		load := map[string]float64{}
		used := map[int]bool{}
		slots := wp.selectExercisesForDayWithPeriodization(
			CategoryLower, 2, PeriodizationStrength, false, used, load,
		)
		if len(slots) != 2 {
			t.Fatalf("want 2 slots, got %d", len(slots))
		}
		for _, s := range slots {
			ex := findExercise(wp.Exercises, s.Exercise.ID)
			if ex.Category != CategoryLower {
				t.Errorf("lower day got exercise with category %s", ex.Category)
			}
		}
	})

	t.Run("upper day only selects upper exercises", func(t *testing.T) {
		t.Parallel()
		load := map[string]float64{}
		used := map[int]bool{}
		slots := wp.selectExercisesForDayWithPeriodization(
			CategoryUpper, 2, PeriodizationStrength, false, used, load,
		)
		for _, s := range slots {
			ex := findExercise(wp.Exercises, s.Exercise.ID)
			if ex.Category != CategoryUpper {
				t.Errorf("upper day got exercise with category %s", ex.Category)
			}
		}
	})

	t.Run("full body day can select any category", func(t *testing.T) {
		t.Parallel()
		load := map[string]float64{}
		used := map[int]bool{}
		slots := wp.selectExercisesForDayWithPeriodization(
			CategoryFullBody, 3, PeriodizationStrength, false, used, load,
		)
		seen := map[Category]bool{}
		for _, s := range slots {
			ex := findExercise(wp.Exercises, s.Exercise.ID)
			seen[ex.Category] = true
		}
		if !seen[CategoryLower] || !seen[CategoryUpper] {
			t.Error("full body day should draw from multiple categories with targets across both")
		}
	})
}

func TestSelectExercises_SessionDiversity(t *testing.T) {
	t.Parallel()

	t.Run("no primary muscle group repeats within a session", func(t *testing.T) {
		t.Parallel()
		// Three Chest-primary exercises in the pool; we ask for 3 slots.
		// Only one can be picked (no primary overlap); the other 2 must come
		// from non-Chest primaries (Triceps-only exercise).
		exercises := []Exercise{
			{ //nolint:exhaustruct // Test exercises omit display fields.
				ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
				RepMin: new(5), RepMax: new(10),
			},
			{ //nolint:exhaustruct // Test exercises omit display fields.
				ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Chest", "Triceps"}, SecondaryMuscleGroups: nil,
				RepMin: new(5), RepMax: new(10),
			},
			{ //nolint:exhaustruct // Test exercises omit display fields.
				ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
				PrimaryMuscleGroups: []string{"Triceps"}, SecondaryMuscleGroups: nil,
				RepMin: new(5), RepMax: new(10),
			},
		}
		wp := NewPlanner(prefs(time.Tuesday), exercises, []MuscleGroupTarget{
			{MuscleGroupName: "Chest", WeeklySetTarget: 10},
			{MuscleGroupName: "Triceps", WeeklySetTarget: 8},
		})
		load := map[string]float64{}
		used := map[int]bool{}
		slots := wp.selectExercisesForDayWithPeriodization(
			CategoryUpper, 3, PeriodizationStrength, false, used, load,
		)

		seenPrimary := map[string]bool{}
		for _, s := range slots {
			ex := findExercise(exercises, s.Exercise.ID)
			for _, mg := range ex.PrimaryMuscleGroups {
				if seenPrimary[mg] {
					t.Errorf("primary muscle group %q appears in two picks in the same session", mg)
				}
				seenPrimary[mg] = true
			}
		}
	})
}

func TestSelectExercises_WeekUsedExclusion(t *testing.T) {
	t.Parallel()

	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
	}
	wp := NewPlanner(prefs(time.Tuesday), exercises, []MuscleGroupTarget{
		{MuscleGroupName: "Chest", WeeklySetTarget: 10},
		{MuscleGroupName: "Shoulders", WeeklySetTarget: 10},
	})
	load := map[string]float64{}
	used := map[int]bool{1: true} // Exercise 1 was used earlier in the week.

	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 1, PeriodizationStrength, false, used, load,
	)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	if slots[0].Exercise.ID == 1 {
		t.Errorf("week-used exercise was picked anyway")
	}
}

func TestSelectExercises_TargetAwarePrefersUnderloadedMG(t *testing.T) {
	t.Parallel()
	// Pool has two equally-eligible exercises. Chest is at zero load,
	// Shoulders already at target. The Chest exercise must win.
	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
	}
	wp := NewPlanner(prefs(time.Tuesday), exercises, []MuscleGroupTarget{
		{MuscleGroupName: "Chest", WeeklySetTarget: 10},
		{MuscleGroupName: "Shoulders", WeeklySetTarget: 10},
	})
	load := map[string]float64{"Shoulders": 10}
	used := map[int]bool{}
	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 1, PeriodizationStrength, false, used, load,
	)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	if slots[0].Exercise.ID != 2 {
		t.Errorf("picked exercise %d (Shoulders); expected exercise 2 (Chest, under target)", slots[0].Exercise.ID)
	}
}

func TestSelectExercises_FallsBackToLowestIDWhenScoresEqual(t *testing.T) {
	t.Parallel()
	// Empty targets: every candidate scores 0. Picker must return the
	// lowest-id eligible candidate deterministically.
	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 7, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercises omit display fields.
			ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
	}
	wp := NewPlanner(prefs(time.Tuesday), exercises, nil)
	load := map[string]float64{}
	used := map[int]bool{}
	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 1, PeriodizationStrength, false, used, load,
	)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	if slots[0].Exercise.ID != 3 {
		t.Errorf("got exercise %d; expected exercise 3 (lowest id among ties)", slots[0].Exercise.ID)
	}
}

func TestSelectExercises_TimeBasedExerciseGetsThreeSets(t *testing.T) {
	t.Parallel()
	plank := Exercise{ //nolint:exhaustruct // Test exercises omit display fields.
		ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeTime,
		PrimaryMuscleGroups: []string{"Abs"}, SecondaryMuscleGroups: nil,
		DefaultStartingSeconds: new(30),
	}
	wp := NewPlanner(prefs(time.Tuesday), []Exercise{plank}, []MuscleGroupTarget{
		{MuscleGroupName: "Abs", WeeklySetTarget: 4},
	})
	load := map[string]float64{}
	used := map[int]bool{}
	slots := wp.selectExercisesForDayWithPeriodization(
		CategoryUpper, 1, PeriodizationStrength, false, used, load,
	)
	if len(slots) != 1 {
		t.Fatalf("want 1 slot, got %d", len(slots))
	}
	if len(slots[0].Sets) != defaultTimedSets {
		t.Errorf("time-based slot has %d sets, want %d", len(slots[0].Sets), defaultTimedSets)
	}
	for _, s := range slots[0].Sets {
		if s.TargetValue != 30 {
			t.Errorf("target seconds = %d, want 30", s.TargetValue)
		}
	}
}
```

- [ ] **Step 3.5: Update the surviving `Plan`-level test for week dedup**

In `internal/domain/planner_internal_test.go`, the existing `TestSelectExercisesForDayWeekDeduplication/plan() does not repeat exercises across days` subtest exercises the top-level `Plan` API rather than the renamed selection function — keep it but lift it to its own top-level test for clarity. Replace its parent `TestSelectExercisesForDayWeekDeduplication` (which is now gone) with:

```go
func TestPlan_DoesNotRepeatExercisesAcrossDays(t *testing.T) {
	t.Parallel()
	exercises := minimalExercises()
	targets := minimalTargets()
	monday := monday2026Date()
	p := prefs(time.Monday, time.Tuesday, time.Thursday)
	wp := NewPlanner(p, exercises, targets)

	plan, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	used := map[int]bool{}
	for i := range plan.Sessions {
		for _, slot := range plan.Sessions[i].Slots {
			if used[slot.Exercise.ID] {
				t.Errorf("exercise %d appears in two sessions across the week", slot.Exercise.ID)
			}
			used[slot.Exercise.ID] = true
		}
	}
}
```

- [ ] **Step 3.6: Compile-check the file**

Run: `go build ./internal/domain/...`
Expected: compile succeeds. If anything fails (e.g. stale call sites of the deleted helpers), fix the call site by adopting the new signature. The only intra-package caller of `selectExercisesForDayWithPeriodization` is `Plan` itself, which is rewritten in Task 4 — at this point `Plan` still references the old internal scaffolding and will need its prior call site updated; do so in Task 4 rather than ad-hoc here.

If `go build` reports errors on `Plan` referencing `dayMuscleGroups`/`priorityMuscleGroups`, leave the errors in place — Task 4 fixes them. To validate Task 3 in isolation:

```bash
go test -v ./internal/domain -run 'Test_scoreCandidate|TestSelectExercises_|TestWeeklyPlannedLoad'
```

(That subset doesn't transitively need `Plan`'s body to compile inside `*_test.go`, but `planner.go` itself must compile. If it doesn't, move directly to Task 4 — they're tightly coupled.)

- [ ] **Step 3.7: Commit** (defer running full tests until Task 4 finishes; Plan still references old shape)

```bash
git add internal/domain/planner.go internal/domain/planner_internal_test.go
git commit -m "refactor(domain): target-aware exercise selection loop

Replace Phase-A/Phase-B selection with a single greedy loop scored by
scoreCandidate against the running per-MG weighted load. Delete
scoreExerciseForPriority, findBestExerciseInPool,
selectAndRemoveFromPool, and the selectExercisesForDay convenience
wrapper. Plan rewrite to follow in the next commit."
```

---

## Task 4: Simplify `Plan` and delete `allocateMuscleGroups`

**Why:** The new selection loop carries all the balancing work; the allocation pre-pass is dead weight. Removing it also drops one place where determinism could leak through map iteration.

**Files:**
- Modify: `internal/domain/planner.go` (rewrite `Plan` body; delete `allocateMuscleGroups`, `hasCategoryExerciseForMuscleGroup`)
- Modify: `internal/domain/planner_internal_test.go` (delete `TestAllocateMuscleGroups`; add prod-scenario regression; add determinism test)

- [ ] **Step 4.1: Rewrite `Plan`**

Replace the body of `Plan` in `internal/domain/planner.go` (currently lines 52–131). Keep the function signature unchanged. The new body:

```go
func (wp *Planner) Plan(startingDate time.Time) (WeekPlan, error) {
	if startingDate.Weekday() != time.Monday {
		return WeekPlan{}, fmt.Errorf("startingDate must be a Monday, got %s", startingDate.Weekday())
	}

	result := WeekPlan{
		Monday:   startingDate,
		Sessions: [7]Session{},
	}
	for i := range 7 {
		result.Sessions[i] = Session{ //nolint:exhaustruct // Rest-day placeholder; no slots or periodization.
			Date: startingDate.AddDate(0, 0, i),
		}
	}

	var workoutDays []time.Time
	for i := range 7 {
		day := startingDate.AddDate(0, 0, i)
		if wp.Prefs.IsWorkoutDay(day.Weekday()) {
			workoutDays = append(workoutDays, day)
		}
	}
	if len(workoutDays) == 0 {
		return WeekPlan{}, errors.New("no workout days scheduled in preferences")
	}

	for _, day := range workoutDays {
		cat := wp.determineCategory(day)
		if !wp.hasExercisesForCategory(cat) {
			return WeekPlan{}, fmt.Errorf("%w: %s day (%s)", errNoExercisesForCategory, cat, day.Weekday())
		}
	}

	firstPT := wp.firstSessionPeriodizationType(startingDate)
	isDeload := IsDeloadWeek(
		startingDate, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled,
	)

	weekUsedExercises := map[int]bool{}
	load := map[string]float64{}
	for i, day := range workoutDays {
		pt := nextPeriodizationType(firstPT, i)
		if isDeload {
			pt = PeriodizationHypertrophy
		}
		n := exercisesPerSession(wp.Prefs, day.Weekday(), pt, isDeload)
		slots := wp.selectExercisesForDayWithPeriodization(
			wp.determineCategory(day), n, pt, isDeload, weekUsedExercises, load,
		)
		dayOffset := int(day.Sub(startingDate).Hours() / hoursPerDay)
		result.Sessions[dayOffset] = Session{ //nolint:exhaustruct // DifficultyRating/StartedAt/CompletedAt start zero.
			Date:              day,
			PeriodizationType: pt,
			IsDeload:          isDeload,
			Slots:             slots,
		}
	}

	return result, nil
}
```

- [ ] **Step 4.2: Delete `allocateMuscleGroups` and `hasCategoryExerciseForMuscleGroup`**

In `internal/domain/planner.go`, delete:

- `hasCategoryExerciseForMuscleGroup` (currently around line 270).
- `allocateMuscleGroups` (currently around line 285).

The `mgEntry` struct lived inside `allocateMuscleGroups`; it goes with it.

Also remove the unused constants `maxMuscleGroupDaysPerWeek` (currently in the `const` block at line 20) — Go's compiler will flag it once the only caller is gone.

- [ ] **Step 4.3: Delete `TestAllocateMuscleGroups`**

In `internal/domain/planner_internal_test.go`, delete the entire `TestAllocateMuscleGroups` function (currently around line 189). It tests a function that no longer exists.

- [ ] **Step 4.4: Add the prod-scenario regression test**

Append to `internal/domain/planner_internal_test.go`:

```go
// prodLikePool returns a 15-exercise pool that mirrors the prod
// secondary-footprint asymmetry: dense secondary coverage on Shoulders/
// Upper Back/Triceps (so they could balloon under the old algorithm)
// and sparse secondary coverage on Chest/Quads (so they could only
// accumulate via primary picks).
func prodLikePool() []Exercise {
	return []Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Name: "Deadlift", Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Hamstrings", "Lower Back"},
			SecondaryMuscleGroups: []string{"Forearms", "Lats", "Quads", "Traps", "Upper Back"},
			RepMin:                new(3), RepMax: new(6),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Name: "Bench Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
			SecondaryMuscleGroups: []string{"Abs", "Forearms", "Shoulders"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 3, Name: "Tricep Pushdown", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Triceps"},
			SecondaryMuscleGroups: []string{"Shoulders"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 4, Name: "Dumbbell Biceps Curl", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Biceps"},
			SecondaryMuscleGroups: []string{"Forearms"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 5, Name: "Lateral Raise", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{"Traps", "Upper Back"},
			RepMin:                new(10), RepMax: new(20),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 6, Name: "Shoulder Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders"},
			SecondaryMuscleGroups: []string{"Triceps", "Upper Back"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 7, Name: "Push-Up", Category: CategoryUpper, ExerciseType: ExerciseTypeBodyweight,
			PrimaryMuscleGroups:   []string{"Chest", "Triceps"},
			SecondaryMuscleGroups: []string{"Abs", "Forearms", "Shoulders", "Upper Back"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 8, Name: "Cable Fly", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Chest"},
			SecondaryMuscleGroups: []string{"Shoulders", "Triceps"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 9, Name: "Pulldown", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Lats", "Upper Back"},
			SecondaryMuscleGroups: []string{"Biceps", "Shoulders"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 10, Name: "Face Pull", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Shoulders", "Upper Back"},
			SecondaryMuscleGroups: []string{"Traps", "Triceps"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 11, Name: "Leg Press", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Calves", "Hamstrings"},
			RepMin:                new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 12, Name: "Leg Extension", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Quads"},
			SecondaryMuscleGroups: []string{"Hip Flexors"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 13, Name: "Squat", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Quads"},
			SecondaryMuscleGroups: []string{"Hamstrings", "Lower Back"},
			RepMin:                new(3), RepMax: new(6),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 14, Name: "Leg Curl", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Hamstrings"},
			SecondaryMuscleGroups: []string{"Calves"},
			RepMin:                new(8), RepMax: new(12),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 15, Name: "Romanian Deadlift", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups:   []string{"Glutes", "Hamstrings"},
			SecondaryMuscleGroups: []string{"Lower Back"},
			RepMin:                new(8), RepMax: new(20),
		},
	}
}

func prodLikeTargets() []MuscleGroupTarget {
	return []MuscleGroupTarget{
		{MuscleGroupName: "Chest", WeeklySetTarget: 10},
		{MuscleGroupName: "Shoulders", WeeklySetTarget: 10},
		{MuscleGroupName: "Triceps", WeeklySetTarget: 8},
		{MuscleGroupName: "Biceps", WeeklySetTarget: 8},
		{MuscleGroupName: "Upper Back", WeeklySetTarget: 10},
		{MuscleGroupName: "Lats", WeeklySetTarget: 10},
		{MuscleGroupName: "Quads", WeeklySetTarget: 10},
		{MuscleGroupName: "Hamstrings", WeeklySetTarget: 8},
		{MuscleGroupName: "Glutes", WeeklySetTarget: 8},
	}
}

// prefs90 returns prefs with the given weekdays scheduled at 90 minutes
// (the size that triggers exercisesLong / exercisesLongHypertrophy).
func prefs90(days ...time.Weekday) Preferences {
	p := Preferences{} //nolint:exhaustruct // Other prefs irrelevant to this test.
	for _, d := range days {
		p.Minutes[d] = 90
	}
	return p
}

func TestPlan_TargetAwareBalanceUnderProdLikePool(t *testing.T) {
	t.Parallel()
	// Tue/Thu/Sat 90 min, all FullBody. This is user 24's current schedule;
	// under the old algorithm Shoulders/Triceps/Upper Back ballooned and
	// Chest/Quads sat under target.
	p := prefs90(time.Tuesday, time.Thursday, time.Saturday)
	wp := NewPlanner(p, prodLikePool(), prodLikeTargets())

	plan, err := wp.Plan(monday2026Date())
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	load := WeeklyPlannedLoad(planSessions(plan))
	for _, target := range prodLikeTargets() {
		l := load[target.MuscleGroupName]
		t.Logf("%s planned %.1f / target %d", target.MuscleGroupName, l, target.WeeklySetTarget)
	}

	for _, target := range prodLikeTargets() {
		l := load[target.MuscleGroupName]
		upper := 1.5 * float64(target.WeeklySetTarget)
		if l > upper {
			t.Errorf("%s planned %.1f exceeds 1.5x target (%v)", target.MuscleGroupName, l, upper)
		}
	}
	if got := load["Chest"]; got < 8 {
		t.Errorf("Chest planned %.1f is below the prior under-load floor (8.0)", got)
	}
}

func planSessions(plan WeekPlan) []Session {
	var ss []Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].Slots) > 0 {
			ss = append(ss, plan.Sessions[i])
		}
	}
	return ss
}
```

- [ ] **Step 4.5: Add the determinism test**

Append to `internal/domain/planner_internal_test.go`:

```go
func TestPlan_DeterministicAcrossRuns(t *testing.T) {
	t.Parallel()
	// Same inputs twice → byte-equal Sessions output. Guards against map
	// iteration order leaking into selection.
	p := prefs90(time.Tuesday, time.Thursday, time.Saturday)
	wp := NewPlanner(p, prodLikePool(), prodLikeTargets())

	monday := monday2026Date()
	planA, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan A failed: %v", err)
	}
	planB, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan B failed: %v", err)
	}

	for i := range 7 {
		if got, want := slotIDs(planA.Sessions[i]), slotIDs(planB.Sessions[i]); !sliceEqInt(got, want) {
			t.Errorf("day %d differs: A=%v B=%v", i, got, want)
		}
	}
}

func slotIDs(s Session) []int {
	ids := make([]int, len(s.Slots))
	for i, slot := range s.Slots {
		ids[i] = slot.Exercise.ID
	}
	return ids
}

func sliceEqInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4.6: Run the full domain test suite**

Run: `go test ./internal/domain/...`
Expected: PASS. If any pre-existing `TestPlan` / `TestPlanner_*` test fails because it asserted specific exercise IDs from the old algorithm, replace that assertion with a property assertion (e.g. category, set count, periodization) or delete the test if it was algorithm-internal. Document each such replacement in the commit message.

- [ ] **Step 4.7: Run race + shuffle**

Run: `make test`
Expected: PASS.

- [ ] **Step 4.8: Commit**

```bash
git add internal/domain/planner.go internal/domain/planner_internal_test.go
git commit -m "refactor(domain): drop priority-MG allocation, simplify Plan

Plan now walks workout days greedily, sharing a per-MG weighted-load
tally across days. allocateMuscleGroups, hasCategoryExerciseForMuscleGroup,
and maxMuscleGroupDaysPerWeek are removed; the target-aware selection
loop carries all balancing work.

Adds two property tests: a prod-like 15-exercise / 9-target / Tue-Thu-Sat
fixture asserts no MG exceeds 1.5x target and Chest stays >= 8, and a
determinism test asserts identical inputs produce byte-equal plans."
```

---

## Task 5: Update `PlanDay` to take `weekLoad`

**Why:** Ad-hoc days need the same target-awareness as the weekly plan, so calling `PlanDay` with the current week's already-planned load lets the picker avoid stacking on top of saturated MGs.

**Files:**
- Modify: `internal/domain/planner.go` (function `PlanDay`)
- Modify: `internal/domain/planner_plan_day_internal_test.go` (every test that calls `PlanDay`)

- [ ] **Step 5.1: Update `PlanDay` signature and body**

Replace the function (currently lines 139–201) with:

```go
// PlanDay generates one Session for date, suitable for ad-hoc workouts on
// days outside the weekly plan (extra workouts, or days added mid-week
// after Plan(monday) already ran). weekUsedExerciseIDs is the set of
// exercise IDs already used in other sessions this week; weekLoad is
// the running per-MG weighted load from those sessions (built by the
// caller via WeeklyPlannedLoad). The planner mutates a copy of
// weekLoad internally for scoring this day's picks and returns the
// resulting Session. Returns errNoExercisesForCategory (wrapped) if
// the derived category has no compatible exercises.
func (wp *Planner) PlanDay(
	date time.Time,
	weekUsedExerciseIDs map[int]bool,
	weekLoad map[string]float64,
) (Session, error) {
	category := wp.determineCategory(date)
	if !wp.hasExercisesForCategory(category) {
		return Session{}, fmt.Errorf(
			"%w: %s day (%s)", errNoExercisesForCategory, category, date.Weekday(),
		)
	}

	idx := 0
	target := date.Weekday()
	for _, d := range []time.Weekday{
		time.Monday, time.Tuesday, time.Wednesday,
		time.Thursday, time.Friday, time.Saturday,
	} {
		if d == target {
			break
		}
		if wp.Prefs.IsWorkoutDay(d) {
			idx++
		}
	}
	monday := MondayOf(date)
	firstPT := wp.firstSessionPeriodizationType(monday)
	pt := nextPeriodizationType(firstPT, idx)

	isDeload := IsDeloadWeek(
		monday, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled,
	)
	if isDeload {
		pt = PeriodizationHypertrophy
	}

	n := exercisesPerSession(wp.Prefs, date.Weekday(), pt, isDeload)
	if n == 0 {
		n = exercisesMedium
		if pt == PeriodizationHypertrophy && !isDeload {
			n = exercisesMediumHypertrophy
		}
	}

	used := weekUsedExerciseIDs
	if used == nil {
		used = map[int]bool{}
	}
	load := make(map[string]float64, len(weekLoad))
	for k, v := range weekLoad {
		load[k] = v
	}
	slots := wp.selectExercisesForDayWithPeriodization(category, n, pt, isDeload, used, load)

	return Session{ //nolint:exhaustruct // DifficultyRating/StartedAt/CompletedAt start zero.
		Date:              date,
		PeriodizationType: pt,
		IsDeload:          isDeload,
		Slots:             slots,
	}, nil
}
```

(The copy of `weekLoad` insulates the caller from in-place mutation by the selection loop, so calling `PlanDay` is side-effect-free on the caller's load map.)

- [ ] **Step 5.2: Update every `PlanDay` call site in `planner_plan_day_internal_test.go`**

For each test in `internal/domain/planner_plan_day_internal_test.go` (`TestPlanner_PlanDay_IsolatedDateDefaultsToFullBody`, `TestPlanner_PlanDay_AdjacencyToScheduledTomorrowPicksLower`, `TestPlanner_PlanDay_PeriodizationMatchesWeeklyPlannerForScheduledDate`, `TestPlanner_PlanDay_AvoidsUsedExercises`, `TestPlanner_PlanDay_UsesPrefsExerciseCountWhenScheduled`, `TestPlanner_PlanDay_EmptyCategoryPoolReturnsError`, `TestPlanner_PlanDay_PeriodizationMatchesWeeklyPlannerForSundaySchedule`), change the `PlanDay` call from:

```go
sess, err := wp.PlanDay(date, used)
```

to:

```go
sess, err := wp.PlanDay(date, used, nil)
```

(`nil` is acceptable — the function tolerates a nil map.)

- [ ] **Step 5.3: Add a load-aware `PlanDay` test**

Append to `internal/domain/planner_plan_day_internal_test.go`:

```go
func TestPlanner_PlanDay_AvoidsAlreadyLoadedMuscleGroup(t *testing.T) {
	t.Parallel()
	// Pool: one Shoulders-primary exercise, one Chest-primary exercise.
	exercises := []Exercise{
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 1, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
		{ //nolint:exhaustruct // Test exercise omits display fields.
			ID: 2, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: nil,
			RepMin: new(5), RepMax: new(10),
		},
	}
	targets := []MuscleGroupTarget{
		{MuscleGroupName: "Shoulders", WeeklySetTarget: 10},
		{MuscleGroupName: "Chest", WeeklySetTarget: 10},
	}
	// Tuesday scheduled so category=FullBody (isolated day).
	p := Preferences{} //nolint:exhaustruct // Other prefs irrelevant.
	p.Minutes[time.Tuesday] = 60
	wp := NewPlanner(p, exercises, targets)

	weekLoad := map[string]float64{"Shoulders": 10} // Already at target.
	sess, err := wp.PlanDay(time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), nil, weekLoad)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.Slots) == 0 {
		t.Fatalf("no slots picked")
	}
	if sess.Slots[0].Exercise.ID != 2 {
		t.Errorf("first pick is exercise %d; expected exercise 2 (Chest, under target)", sess.Slots[0].Exercise.ID)
	}
}
```

- [ ] **Step 5.4: Run domain tests**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5.5: Commit**

```bash
git add internal/domain/planner.go internal/domain/planner_plan_day_internal_test.go
git commit -m "refactor(domain): PlanDay accepts weekLoad for target-aware ad-hoc picks

PlanDay now takes the running per-MG weighted load from the week's
existing sessions as a third argument. It copies the map before
mutating so callers are not aliased into the selection loop. All
existing PlanDay tests pass nil (no prior load); a new test asserts
ad-hoc picks avoid a muscle group that the rest of the week has
already saturated."
```

---

## Task 6: Service caller computes and passes `weekLoad`

**Why:** `internal/service/sessions.go::planSingleDay` is the only non-test caller of `PlanDay`. It must read the week's existing sessions and pass their `WeeklyPlannedLoad` so the ad-hoc pick is genuinely target-aware in prod.

**Files:**
- Modify: `internal/service/sessions.go`
- Test: rely on existing `internal/service/sessions_test.go` (no failure expected — the load is informational unless prior sessions exist for that week, which the existing test setup creates).

- [ ] **Step 6.1: Locate the week-fetch site near `planSingleDay`**

Open `internal/service/sessions.go`. `planSingleDay` (around line 149) currently fetches `prefs`, `exercises`, `targets` and calls `planner.PlanDay(date, used)`. We need to also fetch the week's existing sessions and compute their load.

Look at how `usedExerciseIDs` is computed in the caller graph: `createAdHocSession` passes in `used` built from the week's `WeekPlan`. The plan is already loaded by the caller upstream of `planSingleDay`. The simplest change: thread the same plan into `planSingleDay` and derive both `used` and `weekLoad` from it inside `planSingleDay`, OR keep the current parameter shape and have `planSingleDay` accept the plan directly.

- [ ] **Step 6.2: Inspect callers of `planSingleDay`**

Run: `grep -n "planSingleDay" /home/martin/petrapp/internal/service/sessions.go`

Expected output identifies the call sites (at the time of writing, only `createAdHocSession` calls `planSingleDay`). Read those call sites to see how they construct `used`.

- [ ] **Step 6.3: Change `planSingleDay` to take the existing plan**

In `internal/service/sessions.go`, change `planSingleDay` from:

```go
func (s *Service) planSingleDay(
	ctx context.Context, date time.Time, used map[int]bool,
) (domain.Session, error) {
```

to:

```go
func (s *Service) planSingleDay(
	ctx context.Context, date time.Time, plan domain.WeekPlan,
) (domain.Session, error) {
```

And rewrite the body to derive `used` and `weekLoad` from `plan` before calling `PlanDay`:

```go
func (s *Service) planSingleDay(
	ctx context.Context, date time.Time, plan domain.WeekPlan,
) (domain.Session, error) {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return domain.Session{}, fmt.Errorf("get preferences: %w", err)
	}
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return domain.Session{}, fmt.Errorf("get exercises: %w", err)
	}
	targets, err := s.repos.MuscleTargets.List(ctx)
	if err != nil {
		return domain.Session{}, fmt.Errorf("get muscle group targets: %w", err)
	}
	used := usedExerciseIDs(plan)
	var sessions []domain.Session
	for i := range plan.Sessions {
		if len(plan.Sessions[i].Slots) > 0 {
			sessions = append(sessions, plan.Sessions[i])
		}
	}
	weekLoad := domain.WeeklyPlannedLoad(sessions)
	planner := domain.NewPlanner(prefs, exercises, targets)
	sess, err := planner.PlanDay(date, used, weekLoad)
	if err != nil {
		return domain.Session{}, fmt.Errorf("plan day %s: %w", date.Format(time.DateOnly), err)
	}
	if sess.IsDeload {
		if err = s.seedDeloadWeights(ctx, &sess); err != nil {
			return domain.Session{}, err
		}
	}
	return sess, nil
}
```

- [ ] **Step 6.4: Update `createAdHocSession` to thread the plan through**

In `internal/service/sessions.go`, change `createAdHocSession`'s signature from `(ctx, date, used map[int]bool)` to `(ctx, date, plan domain.WeekPlan)` and forward `plan` to `planSingleDay`:

```go
func (s *Service) createAdHocSession(ctx context.Context, date time.Time, plan domain.WeekPlan) error {
	sess, err := s.planSingleDay(ctx, date, plan)
	if err != nil {
		return err
	}
	monday := domain.MondayOf(date)
	err = s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		offset := int(date.Sub(wp.Monday).Hours() / 24)
		if offset < 0 || offset > 6 {
			return fmt.Errorf(
				"date %s outside week %s",
				date.Format(time.DateOnly), monday.Format(time.DateOnly),
			)
		}
		wp.Sessions[offset] = sess
		return nil
	})
	if err != nil {
		return fmt.Errorf("create ad-hoc session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}
```

- [ ] **Step 6.5: Update `StartSession` to pass the loaded `plan` instead of `used`**

In the same file, find the block in `StartSession` (around line 243-247) that currently reads:

```go
sessOnDate := plan.SessionOn(date)
hasDate := sessOnDate != nil && len(sessOnDate.Slots) > 0
if !hasDate {
    used := usedExerciseIDs(plan)
    if err = s.createAdHocSession(ctx, date, used); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
        return fmt.Errorf("create ad-hoc %s: %w", date.Format(time.DateOnly), err)
    }
}
```

Replace with:

```go
sessOnDate := plan.SessionOn(date)
hasDate := sessOnDate != nil && len(sessOnDate.Slots) > 0
if !hasDate {
    if err = s.createAdHocSession(ctx, date, plan); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
        return fmt.Errorf("create ad-hoc %s: %w", date.Format(time.DateOnly), err)
    }
}
```

The local `used` variable is no longer needed (the plan carries the same information). The `usedExerciseIDs` helper is still used inside `planSingleDay` after Task 6.3.

- [ ] **Step 6.6: Run service tests**

Run: `go test ./internal/service/...`
Expected: PASS. The existing tests already create weeks with prior sessions; they should still pass because `weekLoad` is purely informational unless prior sessions exist, in which case the planner picks will already match-or-improve the old behavior on the test data.

If any test asserts a specific exercise ID for an ad-hoc day and fails because the load shifted the pick, restate the assertion as a property (e.g. category match, set count). Document any such replacement in the commit.

- [ ] **Step 6.7: Run the full test suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 6.8: Commit**

```bash
git add internal/service/sessions.go
git commit -m "feat(service): pass weekLoad into PlanDay for target-aware ad-hoc days

planSingleDay now derives the running per-MG weighted load from the
current WeekPlan via domain.WeeklyPlannedLoad and forwards it to
PlanDay. Ad-hoc picks (extra workouts, mid-week added days) now avoid
stacking on top of muscle groups the rest of the week has already
saturated."
```

---

## Task 7: Pre-merge validation

**Why:** Belt-and-braces. The planner is on a critical path — every Monday a fresh week is generated for every user — and the change spans domain + service. A green CI matrix is a stronger signal than the local quick-run.

**Files:** none (validation only)

- [ ] **Step 7.1: Run lint with auto-fix**

Run: `make lint-fix`
Expected: no errors. Common fixes the rewrite may need: `gofmt` on the new function bodies, `funlen` if `Plan` grew beyond 100 lines (split if so).

- [ ] **Step 7.2: Run full CI**

Run: `make ci`
Expected: PASS. (`make ci` is `init + build + lint-fix + test + sec` per `CLAUDE.md`.)

- [ ] **Step 7.3: Smoke-check the planned week locally**

Spot-check the new behavior against the prod-like fixture by running just the regression test with verbose output:

```bash
go test -v ./internal/domain -run TestPlan_TargetAwareBalanceUnderProdLikePool
```

Expected: prints each MG's planned load. Eyeball that Chest/Quads are at or above 8 and that Shoulders/Triceps/Upper Back are inside `[0, 15]`.

- [ ] **Step 7.4: Push and open a PR**

```bash
git push -u origin HEAD
gh pr create --title "Target-aware weekly planner" --body "$(cat <<'EOF'
## Summary

- Replace priority-MG allocation + two-phase exercise selection with a single greedy scoring loop that minimises sum-of-squared-distances between the running per-MG weighted load and configured targets.
- Add `WeeklyPlannedLoad` helper and `scoreCandidate` pure scoring function.
- Extend `PlanDay` to accept the week's existing load so ad-hoc picks share the same target-awareness.

Spec: [`docs/superpowers/specs/2026-05-26-target-aware-planner-design.md`](docs/superpowers/specs/2026-05-26-target-aware-planner-design.md)

## Test plan

- [ ] `make ci` passes locally
- [ ] On staging review app, generate a fresh week for a 3-day Tue/Thu/Sat 90 min profile and confirm muscle-balance bar shows Shoulders/Triceps/Upper Back inside their target band and Chest/Quads at or above target.
- [ ] On prod after merge, watch user 24's next-week regeneration and confirm the original symptom is resolved.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

After CI passes and the review app spot-check is green, merge per the repo's standard CI/CD flow (`make` flows handle staging then prod auto-deploy).

---

## Spec coverage check

| Spec section / requirement | Implemented in |
|----------------------------|----------------|
| Scoring formula (squared-distance reduction)              | Task 2 |
| `scoreCandidate` pure helper                              | Task 2 |
| Greedy pick loop with shared `load` across days           | Task 3, Task 4 |
| Delete `allocateMuscleGroups`                             | Task 4 |
| Delete `scoreExerciseForPriority`, `findBestExerciseInPool`, `selectAndRemoveFromPool`, `selectExercisesForDay` | Task 3 |
| Keep `primaryMuscleGroupsOverlap` (intra-session dedup)   | Task 3 (kept, not deleted) |
| Category compatibility filter retained                    | Task 3 |
| Periodization + deload set count via `deriveSchemeForExercise` | Task 2 (used in `scoreCandidate`), Task 3 (used in selection) |
| Tiebreaker by lowest exercise ID                          | Task 3 (`if bestIdx < 0 || score > bestScore`) |
| Graceful degradation when pool exhausts mid-session       | Task 3 (early `break`) |
| `PlanDay` new `weekLoad` parameter                        | Task 5 |
| Caller in `internal/service/sessions.go` updated          | Task 6 |
| `WeeklyPlannedLoad` helper                                | Task 1 |
| Unit tests for scoring (positive/negative/zero/deload)    | Task 2 |
| Updated selection tests (category, diversity, week-used)  | Task 3 |
| Prod-scenario regression                                  | Task 4 |
| Determinism test                                          | Task 4 |
| `PlanDay` parity test (avoids loaded MG)                  | Task 5 |
| Out-of-scope: target recalibration                        | Not in plan (explicit follow-up per spec) |
| Out-of-scope: visualization                               | Not in plan |
| Out-of-scope: untargeted-MG handling beyond "ignore"      | Not in plan |
