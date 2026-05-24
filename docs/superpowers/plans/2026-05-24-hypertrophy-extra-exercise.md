# Hypertrophy Extra Exercise Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add one extra exercise to hypertrophy sessions of ≥60 min (non-deload), making `exercisesPerSession` periodization-aware.

**Architecture:** Single-file change in `internal/domain/planner.go`. New constants `exercisesLongHypertrophy = 5` and `exercisesMediumHypertrophy = 4` mirror the existing constant pattern. The `exercisesPerSession` function takes two new parameters (`pt PeriodizationType, isDeload bool`) and returns the bumped count when `pt == PeriodizationHypertrophy && !isDeload && minutes >= 60`. The two existing callers (`Plan`, `PlanDay`) already have those values in scope; `PlanDay` needs a small reorder so they are computed before the count is. The unused `exercisesPerWeek` helper is deleted rather than threaded through.

**Tech Stack:** Go 1.x, standard library only. `go test --race --shuffle=on` via `make test`. `golangci-lint --fix` via `make lint-fix`.

**Reference spec:** `docs/superpowers/specs/2026-05-24-hypertrophy-extra-exercise-design.md`

---

## File touches

- **Modify** `internal/domain/planner.go`:
  - Constants block (lines 11-21): add `exercisesLongHypertrophy`, `exercisesMediumHypertrophy`.
  - `Plan` (line 92): pass `pt`, `isDeload` to `exercisesPerSession`.
  - `PlanDay` (lines 124-181): reorder to compute `pt`/`isDeload` before `n`; update fallback.
  - `exercisesPerSession` (lines 183-195): new signature, new branching.
  - `exercisesPerWeek` (lines 219-228): delete (unused, `//nolint:unused`).
- **Modify** `internal/domain/planner_internal_test.go`:
  - One existing test assertion at line 828 (the "each session has correct exercise count for duration" subtest) flips for the hypertrophy day.
  - Add a new table-driven test for `exercisesPerSession`.
  - Add a new end-to-end `Plan` test for a mixed-periodization week.

No DB migration. No HTTP, service, or template changes.

---

## Task 1: Add failing unit test for new `exercisesPerSession` signature

**Files:**
- Test: `internal/domain/planner_internal_test.go` (append)

This task writes a table-driven test that calls `exercisesPerSession` with the future four-argument signature. It will fail to compile until Task 2 lands the signature change.

- [ ] **Step 1: Append the failing test**

Append at the end of `internal/domain/planner_internal_test.go`:

```go
func Test_exercisesPerSession_PeriodizationAware(t *testing.T) {
	t.Parallel()

	// Build a Preferences value where each weekday carries a different minutes
	// value, so the test can pick a weekday to control the minutes input.
	p := Preferences{ //nolint:exhaustruct // RestNotificationsEnabled and mesocycle fields irrelevant.
		MondayMinutes:    minutesLong,   // 90
		TuesdayMinutes:   minutesMedium, // 60
		WednesdayMinutes: 45,
		ThursdayMinutes:  0,
		FridayMinutes:    0,
		SaturdayMinutes:  0,
		SundayMinutes:    0,
	}

	tests := []struct {
		name     string
		weekday  time.Weekday
		pt       PeriodizationType
		isDeload bool
		want     int
	}{
		{"90 strength non-deload", time.Monday, PeriodizationStrength, false, exercisesLong},
		{"90 hypertrophy non-deload", time.Monday, PeriodizationHypertrophy, false, exercisesLongHypertrophy},
		{"90 hypertrophy deload", time.Monday, PeriodizationHypertrophy, true, exercisesLong},
		{"60 strength non-deload", time.Tuesday, PeriodizationStrength, false, exercisesMedium},
		{"60 hypertrophy non-deload", time.Tuesday, PeriodizationHypertrophy, false, exercisesMediumHypertrophy},
		{"60 hypertrophy deload", time.Tuesday, PeriodizationHypertrophy, true, exercisesMedium},
		{"45 strength non-deload", time.Wednesday, PeriodizationStrength, false, exercisesShort},
		{"45 hypertrophy non-deload", time.Wednesday, PeriodizationHypertrophy, false, exercisesShort},
		{"45 hypertrophy deload", time.Wednesday, PeriodizationHypertrophy, true, exercisesShort},
		{"0 strength", time.Thursday, PeriodizationStrength, false, 0},
		{"0 hypertrophy", time.Thursday, PeriodizationHypertrophy, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := exercisesPerSession(p, tt.weekday, tt.pt, tt.isDeload)
			if got != tt.want {
				t.Errorf("exercisesPerSession(weekday=%s, pt=%s, deload=%v) = %d, want %d",
					tt.weekday, tt.pt, tt.isDeload, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails to compile**

Run: `go test ./internal/domain/ -run Test_exercisesPerSession_PeriodizationAware -v`

Expected: build error mentioning either `undefined: exercisesLongHypertrophy` / `undefined: exercisesMediumHypertrophy` or "too many arguments in call to exercisesPerSession". Either is correct — both go away in Task 2.

- [ ] **Step 3: Commit**

```bash
git add internal/domain/planner_internal_test.go
git commit -m "$(cat <<'EOF'
test(domain): add failing table test for periodization-aware exercisesPerSession

Asserts the future 4-arg signature and the 90/60 hypertrophy bump rule.
Fails to compile until the signature change in the next commit.
EOF
)"
```

---

## Task 2: Implement periodization-aware `exercisesPerSession`

**Files:**
- Modify: `internal/domain/planner.go:11-21` (constants)
- Modify: `internal/domain/planner.go:84-115` (Plan call site)
- Modify: `internal/domain/planner.go:124-181` (PlanDay call site, reorder)
- Modify: `internal/domain/planner.go:183-195` (function signature/body)
- Modify: `internal/domain/planner.go:219-228` (delete `exercisesPerWeek`)

- [ ] **Step 1: Add the new constants**

Replace the constants block at `internal/domain/planner.go:11-21`:

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

- [ ] **Step 2: Change `exercisesPerSession`**

Replace the function at `internal/domain/planner.go:183-195`:

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

- [ ] **Step 3: Update the `Plan` call site**

At `internal/domain/planner.go:92`, change:

```go
n := exercisesPerSession(wp.Prefs, day.Weekday())
```

to:

```go
n := exercisesPerSession(wp.Prefs, day.Weekday(), pt, isDeload)
```

(`pt` and `isDeload` are already in scope from lines 82, 88-91.)

- [ ] **Step 4: Update the `PlanDay` call site (reorder)**

The current `PlanDay` body (`internal/domain/planner.go:124-181`) computes `n` at the top (line 132), then `pt`/`isDeload` further down (lines 142-165). Reorder so `pt`/`isDeload` are computed first.

Replace the body after the category check (from line 130 onward) with:

```go
	// Periodization: replicate the weekly planner's per-day alternation.
	// Count scheduled prefs days strictly before date.Weekday() in Mon-first
	// week order. Iterating Mon..Sat explicitly (rather than as an int range)
	// handles Sunday correctly: time.Sunday = 0 < time.Monday = 1, so an int
	// range would never count anything for a Sunday date.
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
	// Sunday never matches any d above, so it falls through with the full count
	// of scheduled Mon..Sat days — exactly the index workoutDays[i==len-1] would
	// have produced for it.
	monday := MondayOf(date)
	firstPT := wp.firstSessionPeriodizationType(monday)
	pt := nextPeriodizationType(firstPT, idx)

	isDeload := IsDeloadWeek(monday, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled)
	if isDeload {
		pt = PeriodizationHypertrophy
	}

	// Exercise count: from prefs if the day is scheduled, otherwise medium
	// (bumped to medium-hypertrophy when the ad-hoc day is hypertrophy and
	// not deload, matching the scheduled-day rule).
	n := exercisesPerSession(wp.Prefs, date.Weekday(), pt, isDeload)
	if n == 0 {
		n = exercisesMedium
		if pt == PeriodizationHypertrophy && !isDeload {
			n = exercisesMediumHypertrophy
		}
	}

	used := weekUsedExerciseIDs
	if used == nil {
		used = make(map[int]bool)
	}
	exerciseSets := wp.selectExercisesForDayWithPeriodization(
		category, nil, n, pt, isDeload, used,
	)

	return Session{ //nolint:exhaustruct // DifficultyRating, StartedAt, CompletedAt start zero.
		Date:              date,
		PeriodizationType: pt,
		IsDeload:          isDeload,
		ExerciseSets:      exerciseSets,
	}, nil
}
```

- [ ] **Step 5: Delete `exercisesPerWeek`**

Delete the entire unused helper at `internal/domain/planner.go:216-228` (the comment block "exercisesPerWeek sums the exercise count across all scheduled days." through the closing `}`).

- [ ] **Step 6: Run the new unit test to verify it passes**

Run: `go test ./internal/domain/ -run Test_exercisesPerSession_PeriodizationAware -v`

Expected: PASS for all eleven subtests.

- [ ] **Step 7: Run the full domain test suite**

Run: `go test --race ./internal/domain/...`

Expected: PASS overall, but one subtest will likely fail: `Test*/each session has correct exercise count for duration` (Task 3 fixes it).

If any other test fails, stop and surface the failure — the spec only anticipated the one assertion at line 828.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/planner.go
git commit -m "$(cat <<'EOF'
feat(domain): bump hypertrophy session count by 1 at >= 60 min

exercisesPerSession is now periodization-aware:
  90 min hypertrophy → 5 (was 4)
  60 min hypertrophy → 4 (was 3)
Strength and deload sessions keep their base counts. 45 min and rest
days are unaffected.

Plan and PlanDay already had pt and isDeload in scope; PlanDay
reordered so they are computed before the count. The unused
exercisesPerWeek helper is deleted rather than threaded through.

Spec: docs/superpowers/specs/2026-05-24-hypertrophy-extra-exercise-design.md
EOF
)"
```

---

## Task 3: Fix the broken existing test assertion

**Files:**
- Modify: `internal/domain/planner_internal_test.go:816-832` (the "each session has correct exercise count for duration" subtest)

The subtest schedules Mon+Wed at 60 min. With strength-first alternation, Mon is strength (3 exercises) and Wed is hypertrophy (now 4 exercises). The current assertion expects both to equal `exercisesMedium = 3`.

- [ ] **Step 1: Update the subtest assertion**

Replace the body of the `t.Run("each session has correct exercise count for duration", ...)` subtest (around lines 816-832) with:

```go
	t.Run("each session has correct exercise count for duration", func(t *testing.T) {
		t.Parallel()
		// 60 min: strength → 3 exercises, hypertrophy → 4 exercises.
		p := prefs(time.Monday, time.Wednesday)
		wp := NewPlanner(p, exercises, targets)
		wp.rng = rand.New(rand.NewPCG(2, 0))

		sessions, err := wp.Plan(monday)
		if err != nil {
			t.Fatalf("Plan returned error: %v", err)
		}
		for _, sess := range sessions {
			want := exercisesMedium
			if sess.PeriodizationType == PeriodizationHypertrophy && !sess.IsDeload {
				want = exercisesMediumHypertrophy
			}
			if len(sess.ExerciseSets) != want {
				t.Errorf("60-min %s session: want %d exercises, got %d",
					sess.PeriodizationType, want, len(sess.ExerciseSets))
			}
		}
	})
```

- [ ] **Step 2: Run the updated subtest**

Run: `go test ./internal/domain/ -run "TestPlan" -v`

Expected: the "each session has correct exercise count for duration" subtest passes. (Use whatever the parent test name is — `grep -n "each session has correct exercise count" internal/domain/planner_internal_test.go` and look for the enclosing `func Test...` to confirm.)

- [ ] **Step 3: Run the full domain suite to confirm no other regressions**

Run: `go test --race ./internal/domain/...`

Expected: PASS.

If anything else fails, stop and surface it. The spec audit anticipated only this one assertion.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/planner_internal_test.go
git commit -m "$(cat <<'EOF'
test(domain): fix 60-min count assertion for hypertrophy bump

The Plan-level subtest expected both Mon (strength) and Wed
(hypertrophy) to have 3 exercises at 60 min. With the new rule,
hypertrophy days at 60 min get 4 exercises. The assertion now
branches on PeriodizationType.
EOF
)"
```

---

## Task 4: Add end-to-end `Plan` test for mixed-periodization week

**Files:**
- Modify: `internal/domain/planner_internal_test.go` (append)

A `Plan()` test that schedules four 90-min days and asserts the strength/hypertrophy alternation produces a `[4, 5, 4, 5]` exercise-count pattern.

- [ ] **Step 1: Pick a strength-first Monday**

The helper `firstSessionPeriodizationType` (`planner.go:232-239`) is deterministic from `startingDate.Unix() / secondsPerWeek` parity. Existing tests use `monday2026Date()` — read its definition to confirm which periodization that Monday produces. Use it directly if strength-first; otherwise add one week (`monday2026Date().AddDate(0, 0, 7)`) to flip parity.

Run: `grep -n "func monday2026Date" internal/domain/` to find the helper definition.

You can also compute it inline at test time:

```go
firstPT := (&Planner{}).firstSessionPeriodizationType(monday)
if firstPT != PeriodizationStrength {
	monday = monday.AddDate(0, 0, 7)
}
```

Use whichever fits the file's existing style.

- [ ] **Step 2: Append the failing end-to-end test**

Append at the end of `internal/domain/planner_internal_test.go`:

```go
func Test_Plan_HypertrophyDaysGetFiveExercisesAt90Min(t *testing.T) {
	t.Parallel()

	// Anchor on a strength-first Monday so the alternation is deterministic.
	monday := monday2026Date()
	pl := &Planner{} //nolint:exhaustruct // only firstSessionPeriodizationType is used.
	if pl.firstSessionPeriodizationType(monday) != PeriodizationStrength {
		monday = monday.AddDate(0, 0, 7)
	}

	// Four 90-min days: Mon, Tue, Thu, Fri. Strength-first alternation
	// yields [strength, hypertrophy, strength, hypertrophy].
	p := Preferences{ //nolint:exhaustruct // RestNotificationsEnabled and mesocycle fields irrelevant.
		MondayMinutes:    minutesLong,
		TuesdayMinutes:   minutesLong,
		WednesdayMinutes: 0,
		ThursdayMinutes:  minutesLong,
		FridayMinutes:    minutesLong,
		SaturdayMinutes:  0,
		SundayMinutes:    0,
	}
	wp := NewPlanner(p, minimalExercises(), minimalTargets())
	wp.rng = rand.New(rand.NewPCG(7, 0))

	sessions, err := wp.Plan(monday)
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if len(sessions) != 4 {
		t.Fatalf("want 4 sessions, got %d", len(sessions))
	}

	wantCount := []int{exercisesLong, exercisesLongHypertrophy, exercisesLong, exercisesLongHypertrophy}
	wantPT := []PeriodizationType{
		PeriodizationStrength, PeriodizationHypertrophy,
		PeriodizationStrength, PeriodizationHypertrophy,
	}
	for i, sess := range sessions {
		if sess.PeriodizationType != wantPT[i] {
			t.Errorf("session %d periodization: want %s, got %s", i, wantPT[i], sess.PeriodizationType)
		}
		if got := len(sess.ExerciseSets); got != wantCount[i] {
			t.Errorf("session %d (%s) exercise count: want %d, got %d",
				i, sess.PeriodizationType, wantCount[i], got)
		}
	}
}
```

- [ ] **Step 3: Run the new test**

Run: `go test ./internal/domain/ -run Test_Plan_HypertrophyDaysGetFiveExercisesAt90Min -v`

Expected: PASS. If the fixture's `exercises` slice cannot supply five non-conflicting exercises on a hypertrophy day, the assertion `got != wantCount[i]` will fire for a hypertrophy index. In that case, surface the failure and stop — extending the test fixture is a separate change worth user input (the spec calls this out under "Pool exhaustion").

- [ ] **Step 4: Run the full domain suite**

Run: `go test --race --shuffle=on ./internal/domain/...`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/planner_internal_test.go
git commit -m "$(cat <<'EOF'
test(domain): end-to-end Plan test for hypertrophy 5-exercise weeks

Schedules four 90-min days on a strength-first week and asserts the
alternation produces [4, 5, 4, 5] exercise counts.
EOF
)"
```

---

## Task 5: Full CI run

- [ ] **Step 1: Run lint-fix**

Run: `make lint-fix`

Expected: no errors. golangci-lint may auto-fix formatting (e.g. import ordering).

- [ ] **Step 2: If lint-fix changed any files, commit the fix**

Run: `git status`

If files changed:

```bash
git add -u
git commit -m "$(cat <<'EOF'
chore: lint-fix after hypertrophy extra exercise change
EOF
)"
```

If no files changed, skip the commit.

- [ ] **Step 3: Run the full test suite**

Run: `make test`

Expected: all packages PASS.

- [ ] **Step 4: Confirm clean working tree**

Run: `git status`

Expected: "nothing to commit, working tree clean".

---

## Done

The change is complete when:

- `Test_exercisesPerSession_PeriodizationAware` passes (11 subtests across the matrix).
- `Test_Plan_HypertrophyDaysGetFiveExercisesAt90Min` passes.
- The existing "each session has correct exercise count for duration" subtest passes against the new periodization-aware assertion.
- `make test` and `make lint-fix` are clean.
- `exercisesPerWeek` no longer exists in `internal/domain/planner.go`.
