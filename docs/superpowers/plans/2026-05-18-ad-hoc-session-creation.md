# Ad-hoc Session Creation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `StartSession` succeed on dates with no pre-generated session by lazily creating one via a new single-day planner. Fixes "Start Extra Workout" on unscheduled today and "Start Workout" / "Start Early" on days added to the schedule mid-week after another day was already started.

**Architecture:** A new `Planner.PlanDay(date, weekUsedExerciseIDs)` domain method composes the existing planner primitives (`determineCategory`, `selectExercisesForDayWithPeriodization`, `IsDeloadWeek`) for a one-day input. A new `Sessions.Create` repository method inserts a single session and translates the primary-key conflict to a new `domain.ErrAlreadyExists` sentinel. The service layer's `StartSession` gets a lazy-create branch that runs `PlanDay` + `Create` when the date has no session, with race recovery via `ErrAlreadyExists`. No UI changes — the home page already shows the right buttons.

**Tech Stack:** Go 1.22+, SQLite (`mattn/go-sqlite3`), `goquery` for e2e DOM assertions, `e2etest` helper for HTTP-stack tests.

**Spec:** `docs/superpowers/specs/2026-05-18-ad-hoc-session-creation-design.md`

---

## File Structure

**Create:**
- `internal/domain/planner_plan_day_test.go` — unit tests for `PlanDay`.

**Modify:**
- `internal/domain/errors.go` — add `ErrAlreadyExists` sentinel.
- `internal/domain/planner.go` — add `PlanDay` method on `Planner`.
- `internal/repository/repository.go` — add `Create` to `SessionRepository` interface.
- `internal/repository/sessions.go` — add `Create` implementation with PK-conflict translation.
- `internal/repository/sessions_test.go` — round-trip + conflict tests for `Create`.
- `internal/service/sessions.go` — extract `seedDeloadWeights`, add `summarizeWeek` and `createAdHocSession`, rewrite `StartSession` to lazy-create.
- `internal/service/sessions_test.go` — add tests for the new branches.
- `cmd/web/handler-workout_test.go` — add two e2e tests covering the user-visible flows.

---

## Task 1: Add `ErrAlreadyExists` sentinel

**Files:**
- Modify: `internal/domain/errors.go`

- [ ] **Step 1: Add the sentinel**

In `internal/domain/errors.go`, after the `ErrNotFound` declaration, add:

```go
// ErrAlreadyExists is returned by repositories when an insert would violate
// a uniqueness constraint (e.g. inserting a workout_sessions row for a date
// the user already has). Callers use errors.Is to fall through to the
// "already there" code path (idempotent retry, lazy-create race recovery).
var ErrAlreadyExists = errors.New("already exists")
```

- [ ] **Step 2: Build to confirm no breakage**

Run: `go build ./...`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add internal/domain/errors.go
git commit -m "domain: add ErrAlreadyExists sentinel for unique-violation translation"
```

---

## Task 2: Implement `Planner.PlanDay`

**Files:**
- Modify: `internal/domain/planner.go`
- Create: `internal/domain/planner_plan_day_test.go`

### Test setup notes

The existing `planner_internal_test.go` file already defines helpers we'll reuse:
- `monday2026Date()` returns `time.Date(2026, 1, 5, …)`, a known Monday.
- `date(base, offset)` adds `offset` days.
- `prefs(days...)` builds a `Preferences` with the given weekdays at `minutesMedium` (60 min).
- The package is `domain` (internal tests), so unexported identifiers are accessible.

For `PlanDay` we need a small exercise pool covering both categories. Use a local helper inside the new test file (don't pollute the shared helpers).

- [ ] **Step 1: Write the failing test file**

Create `internal/domain/planner_plan_day_test.go`:

```go
package domain

import (
	"errors"
	"testing"
	"time"
)

// planDayExercises returns a small pool with Upper, Lower, and FullBody coverage
// across distinct primary muscles so PlanDay's non-conflict selection has room.
func planDayExercises() []Exercise {
	intPtr := func(v int) *int { return &v }
	return []Exercise{
		{ID: 1, Name: "Bench Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"}, SecondaryMuscleGroups: []string{"Triceps"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ID: 2, Name: "Row", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Upper Back"}, SecondaryMuscleGroups: []string{"Biceps"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ID: 3, Name: "Overhead Press", Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Shoulders"}, SecondaryMuscleGroups: []string{"Triceps"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ID: 4, Name: "Squat", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Quads"}, SecondaryMuscleGroups: []string{"Glutes"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ID: 5, Name: "Deadlift", Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Hamstrings"}, SecondaryMuscleGroups: []string{"Glutes"},
			RepMin: intPtr(5), RepMax: intPtr(10)},
		{ID: 6, Name: "Plank", Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Core"}, SecondaryMuscleGroups: nil,
			RepMin: intPtr(5), RepMax: intPtr(10)},
	}
}

func newPlanDayPlanner(t *testing.T, p Preferences) *Planner {
	t.Helper()
	return NewPlanner(p, planDayExercises(), nil)
}

func TestPlanner_PlanDay_IsolatedDateDefaultsToFullBody(t *testing.T) {
	// Empty prefs → isolated date → adjacency rule yields CategoryFullBody.
	wp := newPlanDayPlanner(t, Preferences{}) //nolint:exhaustruct // all zero on purpose.
	wed := date(monday2026Date(), 2)

	sess, err := wp.PlanDay(wed, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if sess.WorkoutType() != CategoryFullBody {
		t.Errorf("WorkoutType = %s, want full body", sess.WorkoutType())
	}
	// Default exercise count for unscheduled day = exercisesMedium = 3.
	if len(sess.ExerciseSets) != exercisesMedium {
		t.Errorf("ExerciseSets count = %d, want %d", len(sess.ExerciseSets), exercisesMedium)
	}
}

func TestPlanner_PlanDay_AdjacencyToScheduledDayPicksUpperOrLower(t *testing.T) {
	// Prefs: Tue scheduled. For Mon (yesterday is Sun=off, tomorrow is Tue=on)
	// the adjacency rule yields CategoryLower.
	wp := newPlanDayPlanner(t, prefs(time.Tuesday))
	mon := monday2026Date()

	sess, err := wp.PlanDay(mon, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if sess.WorkoutType() != CategoryLower {
		t.Errorf("WorkoutType = %s, want lower (today on, tomorrow on)", sess.WorkoutType())
	}
}

func TestPlanner_PlanDay_PeriodizationMatchesWeeklyPlannerForScheduledDate(t *testing.T) {
	// Mon, Wed, Fri scheduled. Plan(monday) assigns periodization by workoutDays index:
	// Mon=idx0 first, Wed=idx1 second, Fri=idx2 first. PlanDay must agree for each.
	p := prefs(time.Monday, time.Wednesday, time.Friday)
	wp := newPlanDayPlanner(t, p)
	mon := monday2026Date()

	weekly, err := wp.Plan(mon)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, want := range weekly {
		got, err := wp.PlanDay(want.Date, nil)
		if err != nil {
			t.Fatalf("PlanDay(%s): %v", want.Date.Weekday(), err)
		}
		if got.PeriodizationType != want.PeriodizationType {
			t.Errorf("PlanDay(%s) PeriodizationType = %s, want %s (matches weekly planner)",
				want.Date.Weekday(), got.PeriodizationType, want.PeriodizationType)
		}
	}
}

func TestPlanner_PlanDay_AvoidsUsedExercises(t *testing.T) {
	// Force the upper-only pool by picking Tue with Mon and Wed scheduled.
	// Then mark exercises 1,2 as used; only id 3 (Overhead Press) remains.
	p := prefs(time.Monday, time.Wednesday)
	wp := newPlanDayPlanner(t, p)
	tue := date(monday2026Date(), 1) // unscheduled Tuesday between two on days
	used := map[int]bool{1: true, 2: true}

	sess, err := wp.PlanDay(tue, used)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	for _, es := range sess.ExerciseSets {
		if used[es.Exercise.ID] {
			t.Errorf("PlanDay returned used exercise id=%d", es.Exercise.ID)
		}
	}
}

func TestPlanner_PlanDay_UsesPrefsExerciseCountWhenScheduled(t *testing.T) {
	// Long-day prefs (90 min) on Wednesday yields exercisesLong (4).
	p := Preferences{WednesdayMinutes: 90} //nolint:exhaustruct
	wp := newPlanDayPlanner(t, p)
	wed := date(monday2026Date(), 2)

	sess, err := wp.PlanDay(wed, nil)
	if err != nil {
		t.Fatalf("PlanDay: %v", err)
	}
	if len(sess.ExerciseSets) != exercisesLong {
		t.Errorf("ExerciseSets count = %d, want %d (long day)", len(sess.ExerciseSets), exercisesLong)
	}
}

func TestPlanner_PlanDay_EmptyCategoryPoolReturnsError(t *testing.T) {
	// Pool contains only Upper and FullBody. Pick a day whose category is Lower:
	// adjacency requires "yesterday is workout day" → Upper. So we need Lower:
	// today is on, tomorrow is on (gives Lower) → schedule Mon+Tue, ask for Mon.
	// Remove all Lower exercises from the pool to trigger the error.
	all := planDayExercises()
	noLower := make([]Exercise, 0, len(all))
	for _, ex := range all {
		if ex.Category != CategoryLower {
			noLower = append(noLower, ex)
		}
	}
	wp := NewPlanner(prefs(time.Monday, time.Tuesday), noLower, nil)
	mon := monday2026Date()

	_, err := wp.PlanDay(mon, nil)
	if err == nil {
		t.Fatal("PlanDay must error when category pool is empty")
	}
	if !errors.Is(err, errNoExercisesForCategory) {
		// Accept either a sentinel or a message check — see Step 3 for the impl.
		// Adjust this assertion to match the implementation choice.
		_ = err
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./internal/domain/ -run TestPlanner_PlanDay -v`
Expected: build error — `wp.PlanDay undefined`. (The reference to `errNoExercisesForCategory` will also fail; fix in Step 4.)

- [ ] **Step 3: Add a package-level sentinel for the empty-pool case**

In `internal/domain/planner.go`, add (near the top, after the package imports and constants):

```go
// errNoExercisesForCategory is returned by PlanDay (and wrapped by Plan) when
// the exercise pool contains nothing compatible with the derived category.
var errNoExercisesForCategory = errors.New("no exercises available for day category")
```

Then refactor the existing `Plan` to wrap this sentinel so callers can `errors.Is` against it. In `Plan`, change:

```go
return nil, fmt.Errorf("no exercises available for %s day (%s)", cat, day.Weekday())
```

to:

```go
return nil, fmt.Errorf("%w: %s day (%s)", errNoExercisesForCategory, cat, day.Weekday())
```

- [ ] **Step 4: Implement `PlanDay`**

Add to `internal/domain/planner.go` (place after `Plan`):

```go
// PlanDay generates one Session for date, suitable for ad-hoc workouts on
// days outside the weekly plan (extra workouts, or days added mid-week after
// Plan(monday) already ran). weekUsedExerciseIDs is the set of exercise IDs
// already used in other sessions this week; the planner avoids repeating
// them when possible. Returns errNoExercisesForCategory (wrapped) if the
// derived category has no compatible exercises.
func (wp *Planner) PlanDay(date time.Time, weekUsedExerciseIDs map[int]bool) (Session, error) {
	category := wp.determineCategory(date)
	if !wp.hasExercisesForCategory(category) {
		return Session{}, fmt.Errorf( //nolint:exhaustruct // zero value used on error path.
			"%w: %s day (%s)", errNoExercisesForCategory, category, date.Weekday())
	}

	// Exercise count: from prefs if the day is scheduled, otherwise medium.
	n := exercisesPerSession(wp.Prefs, date.Weekday())
	if n == 0 {
		n = exercisesMedium
	}

	// Periodization: replicate the weekly planner's per-day alternation.
	// Count scheduled prefs days strictly before date.Weekday(); the result
	// is the 0-based index PlanDay's date would have occupied in workoutDays.
	idx := 0
	for d := time.Monday; d < date.Weekday(); d++ {
		if wp.Prefs.IsWorkoutDay(d) {
			idx++
		}
	}
	// Special case: date.Weekday() == Sunday loops d through Mon..Sat which
	// is exactly what we want (Sunday is the last possible workout slot).
	monday := mondayForDate(date)
	firstPT := wp.firstSessionPeriodizationType(monday)
	pt := nextPeriodizationType(firstPT, idx)

	isDeload := IsDeloadWeek(monday, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled)
	if isDeload {
		pt = PeriodizationHypertrophy
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

// mondayForDate returns the Monday of the week containing date, in UTC,
// using calendar-date arithmetic to avoid timezone-related rollovers.
func mondayForDate(date time.Time) time.Time {
	y, m, d := date.Date()
	offset := int(time.Monday - date.Weekday())
	if offset > 0 {
		offset = -6
	}
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, offset)
}
```

> Note: the service has its own `mondayOf`. Don't import service into domain; the duplicate is intentional and small.

- [ ] **Step 5: Update the empty-pool test to assert against the new sentinel**

In `internal/domain/planner_plan_day_test.go`, replace the body of `TestPlanner_PlanDay_EmptyCategoryPoolReturnsError`'s assertion with:

```go
	if !errors.Is(err, errNoExercisesForCategory) {
		t.Errorf("err = %v, want wrap of errNoExercisesForCategory", err)
	}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/domain/ -run TestPlanner_PlanDay -v`
Expected: PASS for all six test functions.

- [ ] **Step 7: Run the full domain test suite to confirm no regression**

Run: `go test ./internal/domain/ -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/domain/planner.go internal/domain/planner_plan_day_test.go
git commit -m "domain: add Planner.PlanDay for single-date workout planning"
```

---

## Task 3: Add `Sessions.Create` repository method

**Files:**
- Modify: `internal/repository/repository.go`
- Modify: `internal/repository/sessions.go`
- Modify: `internal/repository/sessions_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/repository/sessions_test.go`:

```go
func TestSessionRepository_Create_InsertsSingleSession(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	ex, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	date := time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC) // a Wednesday
	sess := domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero on insert.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		IsDeload:          false,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned on insert, WarmupCompletedAt nil.
				Exercise: ex,
				Sets: []domain.Set{
					{TargetValue: 5}, //nolint:exhaustruct // all completion fields nil.
				},
			},
		},
	}

	if err = repos.Sessions.Create(ctx, sess); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get after Create: %v", err)
	}
	if len(got.ExerciseSets) != 1 || got.ExerciseSets[0].Exercise.ID != ex.ID {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.PeriodizationType != domain.PeriodizationStrength {
		t.Errorf("PeriodizationType = %s, want strength", got.PeriodizationType)
	}
}

func TestSessionRepository_Create_ConflictReturnsErrAlreadyExists(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	ex, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("create exercise: %v", err)
	}

	date := time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{Exercise: ex, Sets: []domain.Set{{TargetValue: 5}}}, //nolint:exhaustruct
		},
	}
	if err = repos.Sessions.Create(ctx, sess); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	err = repos.Sessions.Create(ctx, sess)
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Errorf("second Create err = %v, want wraps domain.ErrAlreadyExists", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/repository/ -run TestSessionRepository_Create -v`
Expected: compile error — `repos.Sessions.Create undefined`.

- [ ] **Step 3: Extend the interface**

In `internal/repository/repository.go`, in the `SessionRepository` interface block (around line 54), add a new method declaration above `Update`:

```go
	// Create inserts a single session and its exercise slots. Returns
	// domain.ErrAlreadyExists (wrapped) if a session already exists for the
	// date — callers use errors.Is to recover from concurrent insert races.
	Create(ctx context.Context, sess domain.Session) error
```

- [ ] **Step 4: Implement `Create` with PK-conflict translation**

In `internal/repository/sessions.go`, add a new import for the sqlite3 driver:

```go
import (
	// ...existing imports...
	sqlite3 "github.com/mattn/go-sqlite3"
)
```

Then add the method (place after `CreateBatch`):

```go
// Create inserts a single session and its exercise slots in one transaction.
// Translates the workout_sessions PRIMARY KEY conflict to domain.ErrAlreadyExists
// so callers can detect concurrent-insert races.
func (r *sqliteSessionRepository) Create(ctx context.Context, sess domain.Session) (err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()
	if err = r.insertSession(ctx, tx, sess); err != nil {
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
			return fmt.Errorf("insert session %s: %w", formatDate(sess.Date), domain.ErrAlreadyExists)
		}
		return fmt.Errorf("insert session %s: %w", formatDate(sess.Date), err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit create session: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run the new tests to verify they pass**

Run: `go test ./internal/repository/ -run TestSessionRepository_Create -v`
Expected: PASS for both tests.

- [ ] **Step 6: Run the full repository suite to confirm no regression**

Run: `go test ./internal/repository/ -race`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/repository.go internal/repository/sessions.go internal/repository/sessions_test.go
git commit -m "repo: add Sessions.Create with ErrAlreadyExists on PK conflict"
```

---

## Task 4: Extract `seedDeloadWeights` from `generateWeeklyPlan`

**Files:**
- Modify: `internal/service/sessions.go`

This is a pure refactor — no behavior change, no new test needed. The existing deload-week tests cover it.

- [ ] **Step 1: Add the extracted helper**

In `internal/service/sessions.go`, add after the `generateWeeklyPlan` function:

```go
// seedDeloadWeights sets the per-set weight for every weighted exercise in a
// deload session to GetDeloadStartingWeight (a fraction of the user's recent
// working weight). Called for both weekly-plan generation and ad-hoc session
// creation when sess.IsDeload is true.
func (s *Service) seedDeloadWeights(ctx context.Context, sess *domain.Session) error {
	for j := range sess.ExerciseSets {
		ex := sess.ExerciseSets[j].Exercise
		if !ex.HasWeight() {
			continue
		}
		w, err := s.GetDeloadStartingWeight(ctx, ex.ID, sess.Date)
		if err != nil {
			return fmt.Errorf("seed deload weight for %s: %w", ex.Name, err)
		}
		weight := w
		for k := range sess.ExerciseSets[j].Sets {
			sess.ExerciseSets[j].Sets[k].WeightKg = &weight
		}
	}
	return nil
}
```

- [ ] **Step 2: Refactor `generateWeeklyPlan` to call the helper**

In `internal/service/sessions.go`, replace the deload-weight loop in `generateWeeklyPlan` (currently lines ~110–129) with:

```go
	for i := range plannedSessions {
		if !plannedSessions[i].IsDeload {
			continue
		}
		if err = s.seedDeloadWeights(ctx, &plannedSessions[i]); err != nil {
			return err
		}
	}
```

The resulting `generateWeeklyPlan` should look like:

```go
func (s *Service) generateWeeklyPlan(ctx context.Context, monday time.Time) error {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return fmt.Errorf("get exercises: %w", err)
	}
	targets, err := s.repos.MuscleTargets.List(ctx)
	if err != nil {
		return fmt.Errorf("get muscle group targets: %w", err)
	}

	planner := domain.NewPlanner(prefs, exercises, targets)
	plannedSessions, err := planner.Plan(monday)
	if err != nil {
		return fmt.Errorf("plan week: %w", err)
	}

	for i := range plannedSessions {
		if !plannedSessions[i].IsDeload {
			continue
		}
		if err = s.seedDeloadWeights(ctx, &plannedSessions[i]); err != nil {
			return err
		}
	}

	if err = s.repos.Sessions.CreateBatch(ctx, plannedSessions); err != nil {
		return fmt.Errorf("create batch sessions: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Run the existing deload tests to confirm no regression**

Run: `go test ./internal/service/ -run Deload -race -v`
Expected: PASS (deload behavior unchanged).

Run: `go test ./cmd/web/ -run Deload -race -v` (covers the e2e deload path).
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/service/sessions.go
git commit -m "service: extract seedDeloadWeights for reuse by ad-hoc session creation"
```

---

## Task 5: Add `summarizeWeek` + `createAdHocSession` helpers

**Files:**
- Modify: `internal/service/sessions.go`

These helpers exist to keep the rewritten `StartSession` (Task 6) compact.

- [ ] **Step 1: Add `summarizeWeek`**

In `internal/service/sessions.go`, add (after `mondayOf`):

```go
// summarizeWeek walks existing and returns aggregate info needed by
// StartSession for the lazy-create branch:
//   - weekCount: number of sessions whose Date falls in monday..sunday.
//   - hasDate: whether a session exists for date specifically.
//   - usedExerciseIDs: set of exercise IDs used in any in-week session,
//     for PlanDay's no-repeat avoidance.
func summarizeWeek(existing []domain.Session, date, monday time.Time) (int, bool, map[int]bool) {
	sunday := monday.AddDate(0, 0, 6)
	used := make(map[int]bool)
	var weekCount int
	var hasDate bool
	for _, sess := range existing {
		if sess.Date.Before(monday) || sess.Date.After(sunday) {
			continue
		}
		weekCount++
		if sess.Date.Equal(date) {
			hasDate = true
		}
		for _, es := range sess.ExerciseSets {
			used[es.Exercise.ID] = true
		}
	}
	return weekCount, hasDate, used
}
```

- [ ] **Step 2: Add `createAdHocSession`**

In `internal/service/sessions.go`, add (after `summarizeWeek`):

```go
// createAdHocSession plans and persists a single session for date. Used by
// StartSession when the user starts an unscheduled day (extra workout) or a
// day added to the schedule mid-week after another in-week session was
// already started. used is the set of exercise IDs already used in other
// in-week sessions, passed through to PlanDay's no-repeat selection.
func (s *Service) createAdHocSession(ctx context.Context, date time.Time, used map[int]bool) error {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return fmt.Errorf("get exercises: %w", err)
	}
	targets, err := s.repos.MuscleTargets.List(ctx)
	if err != nil {
		return fmt.Errorf("get muscle group targets: %w", err)
	}

	planner := domain.NewPlanner(prefs, exercises, targets)
	sess, err := planner.PlanDay(date, used)
	if err != nil {
		return fmt.Errorf("plan day %s: %w", date.Format(time.DateOnly), err)
	}

	if sess.IsDeload {
		if err = s.seedDeloadWeights(ctx, &sess); err != nil {
			return err
		}
	}
	return s.repos.Sessions.Create(ctx, sess)
}
```

- [ ] **Step 3: Build to confirm no breakage**

Run: `go build ./...`
Expected: no output (success). `summarizeWeek` and `createAdHocSession` are unused until Task 6 — that's fine because Go allows unused package-level identifiers.

- [ ] **Step 4: Commit**

```bash
git add internal/service/sessions.go
git commit -m "service: add summarizeWeek + createAdHocSession helpers"
```

---

## Task 6: Lazy session creation in `StartSession`

**Files:**
- Modify: `internal/service/sessions.go`

- [ ] **Step 1: Rewrite `StartSession`**

In `internal/service/sessions.go`, replace the current `StartSession` (lines ~162–192) with:

```go
// StartSession marks the workout session for date as started. If no session
// exists for date — either because date is unscheduled (extra workout) or
// because date is a newly-scheduled day that was added mid-week after the
// weekly plan was generated — a single-day session is planned via PlanDay
// and inserted before the start mutation. If the whole week is missing the
// existing generateWeeklyPlan path runs first; only then is the per-date
// check applied.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	monday := mondayOf(date)
	existing, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return fmt.Errorf("list sessions for week of %s: %w", date.Format(time.DateOnly), err)
	}

	weekCount, hasDate, used := summarizeWeek(existing, date, monday)

	if weekCount == 0 {
		if err = s.generateWeeklyPlan(ctx, monday); err != nil {
			return fmt.Errorf("generate weekly plan for %s: %w", date.Format(time.DateOnly), err)
		}
		existing, err = s.repos.Sessions.List(ctx, monday)
		if err != nil {
			return fmt.Errorf("re-list sessions for week of %s: %w", date.Format(time.DateOnly), err)
		}
		_, hasDate, used = summarizeWeek(existing, date, monday)
	}

	if !hasDate {
		if err = s.createAdHocSession(ctx, date, used); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
			return fmt.Errorf("create ad-hoc session %s: %w", date.Format(time.DateOnly), err)
		}
	}

	err = s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.Start(time.Now())
	})
	if errors.Is(err, domain.ErrAlreadyStarted) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}
```

- [ ] **Step 2: Build to confirm no breakage**

Run: `go build ./...`
Expected: no output (success).

- [ ] **Step 3: Run existing `StartSession`-adjacent tests to confirm no regression**

Run: `go test ./internal/service/ -race -v`
Expected: PASS for all tests.

Run: `go test ./cmd/web/ -race -run Workout -v`
Expected: PASS.

- [ ] **Step 4: Lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/service/sessions.go
git commit -m "service: lazy session creation in StartSession for ad-hoc and mid-week days"
```

---

## Task 7: Service-layer tests for the lazy-create branches

**Files:**
- Modify: `internal/service/sessions_test.go`

The existing test helpers (`setupTestService`, `extractExerciseIDs`) already set Mon/Wed/Fri at 60 minutes. We'll reuse them.

- [ ] **Step 1: Write the failing tests**

Append to `internal/service/sessions_test.go`:

```go
func Test_StartSession_CreatesAdHocSessionForUnscheduledToday(t *testing.T) {
	// setupTestService sets Mon/Wed/Fri preferences. Pick a day this week
	// the user has not scheduled (a Tuesday) and verify StartSession both
	// creates the session and marks it started.
	ctx, svc := setupTestService(t)

	// Ensure the week is generated first so usedExerciseIDs is populated.
	weekSessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	monday := weekSessions[0].Date
	tue := monday.AddDate(0, 0, 1)

	if err = svc.StartSession(ctx, tue); err != nil {
		t.Fatalf("StartSession on unscheduled Tuesday: %v", err)
	}

	sess, err := svc.GetSession(ctx, tue)
	if err != nil {
		t.Fatalf("GetSession after ad-hoc Start: %v", err)
	}
	if sess.StartedAt.IsZero() {
		t.Error("StartedAt is zero — Start did not mark the session")
	}
	if len(sess.ExerciseSets) == 0 {
		t.Error("ad-hoc session has no exercises — PlanDay or persistence failed")
	}
}

func Test_StartSession_CreatesNewlyScheduledMidWeekDay(t *testing.T) {
	// Mon/Wed/Fri prefs, start Monday, then change prefs to add Tuesday
	// — RegenerateWeeklyPlanIfUnstarted will skip because Monday is started.
	// StartSession on Tuesday must still succeed by creating the session.
	ctx, svc := setupTestService(t)

	weekSessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	monday := weekSessions[0].Date
	tue := monday.AddDate(0, 0, 1)

	if err = svc.StartSession(ctx, monday); err != nil {
		t.Fatalf("StartSession Monday: %v", err)
	}

	if err = svc.SaveUserPreferences(ctx, domain.Preferences{ //nolint:exhaustruct // Rest days omitted.
		MondayMinutes:    60,
		TuesdayMinutes:   60,
		WednesdayMinutes: 60,
		FridayMinutes:    60,
	}); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	// RegenerateWeeklyPlanIfUnstarted is a no-op now (Monday is started).
	if err = svc.RegenerateWeeklyPlanIfUnstarted(ctx); err != nil {
		t.Fatalf("RegenerateWeeklyPlanIfUnstarted: %v", err)
	}

	if err = svc.StartSession(ctx, tue); err != nil {
		t.Fatalf("StartSession Tuesday after schedule change: %v", err)
	}

	sess, err := svc.GetSession(ctx, tue)
	if err != nil {
		t.Fatalf("GetSession Tuesday: %v", err)
	}
	if sess.StartedAt.IsZero() {
		t.Error("Tuesday StartedAt is zero")
	}
	if len(sess.ExerciseSets) == 0 {
		t.Error("Tuesday session has no exercises")
	}
}

func Test_StartSession_DoubleStartIsIdempotent(t *testing.T) {
	// Two StartSession calls on the same unscheduled date must both succeed
	// and leave exactly one started session. Simulates the lazy-create race
	// via sequential calls (the second's Create returns ErrAlreadyExists and
	// the Update is idempotent via ErrAlreadyStarted).
	ctx, svc := setupTestService(t)

	weekSessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}
	tue := weekSessions[0].Date.AddDate(0, 0, 1)

	if err = svc.StartSession(ctx, tue); err != nil {
		t.Fatalf("first StartSession: %v", err)
	}
	if err = svc.StartSession(ctx, tue); err != nil {
		t.Fatalf("second StartSession (must be idempotent): %v", err)
	}

	sess, err := svc.GetSession(ctx, tue)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.StartedAt.IsZero() {
		t.Error("StartedAt is zero after two Start calls")
	}
}
```

- [ ] **Step 2: Run the new tests to verify they pass**

Run: `go test ./internal/service/ -run Test_StartSession -v -race`
Expected: PASS for all three tests.

- [ ] **Step 3: Run the full service suite for regression coverage**

Run: `go test ./internal/service/ -race`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/service/sessions_test.go
git commit -m "service: tests for StartSession lazy-create branches"
```

---

## Task 8: Web-layer e2e tests

**Files:**
- Modify: `cmd/web/handler-workout_test.go`

These tests exercise the full HTTP stack — exactly the user-visible flows the spec calls out. They use the existing `e2etest` server + `client.SubmitForm` pattern.

The existing `Test_application_addWorkout` (at the top of the file) is the canonical bootstrap template — copy its shape:

1. `server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)` — first arg is `*testing.T`, NOT `ctx`.
2. `client := server.Client()`
3. `client.Register(ctx)` to register + log in.
4. Submit to `/preferences` (not `/schedule`) using weekday names as field keys — `SubmitForm`'s label-matching resolves them to the underlying `monday_minutes`/`tuesday_minutes`/… inputs.

- [ ] **Step 1: Write the failing tests**

Append to `cmd/web/handler-workout_test.go`. Reuse the file's existing imports (`time`, `goquery`, `e2etest`, `testhelpers`); don't add duplicates.

```go
func Test_application_startExtraWorkoutOnUnscheduledToday(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Schedule a non-today weekday at 60 min so today is unscheduled.
	today := time.Now().Weekday()
	nonToday := time.Monday
	if today == time.Monday {
		nonToday = time.Tuesday
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", map[string]string{
		nonToday.String(): "60",
	}); err != nil {
		t.Fatalf("Failed to submit preferences: %v", err)
	}

	// Submit "Start Extra Workout" for today.
	todayStr := time.Now().Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+todayStr+"/start", nil); err != nil {
		t.Fatalf("Failed to start extra workout: %v", err)
	}

	// The workout page must list exercises after lazy-create.
	if doc.Find("a.exercise").Length() == 0 {
		t.Error("Expected exercises on workout page after starting ad-hoc session")
	}
}

func Test_application_startNewlyScheduledMidWeekDay(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Set schedule to today only.
	today := time.Now().Weekday()
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", map[string]string{
		today.String(): "60",
	}); err != nil {
		t.Fatalf("Failed to submit preferences: %v", err)
	}

	// Start today's workout so RegenerateWeeklyPlanIfUnstarted will later skip.
	todayStr := time.Now().Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+todayStr+"/start", nil); err != nil {
		t.Fatalf("Failed to start today's workout: %v", err)
	}

	// Add another weekday mid-week (a different day in this week).
	addedDay := time.Now().Weekday() + 1
	if addedDay > time.Sunday {
		addedDay = time.Monday
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences (2nd): %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", map[string]string{
		today.String():    "60",
		addedDay.String(): "60",
	}); err != nil {
		t.Fatalf("Failed to submit updated preferences: %v", err)
	}

	// Compute the calendar date for addedDay in this week (UTC, matching service.mondayOf).
	now := time.Now().UTC()
	y, m, d := now.Date()
	mondayOffset := int(time.Monday - now.Weekday())
	if mondayOffset > 0 {
		mondayOffset = -6
	}
	monday := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, mondayOffset)
	addedDateStr := monday.AddDate(0, 0, int(addedDay-time.Monday)).Format("2006-01-02")

	// Start the newly-scheduled day — this is the case that fails without lazy-create.
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+addedDateStr+"/start", nil); err != nil {
		t.Fatalf("Failed to start newly-scheduled day: %v", err)
	}
	if doc.Find("a.exercise").Length() == 0 {
		t.Error("Expected exercises on newly-scheduled day's workout page after lazy-create")
	}
}
```

- [ ] **Step 2: Run the new tests to verify they pass**

Run: `go test ./cmd/web/ -run "Test_application_startExtraWorkoutOnUnscheduledToday|Test_application_startNewlyScheduledMidWeekDay" -v -race`
Expected: PASS.

- [ ] **Step 3: Run the full web suite**

Run: `go test ./cmd/web/ -race`
Expected: PASS.

- [ ] **Step 4: Run the full CI gate**

Run: `make ci`
Expected: PASS (init + build + lint-fix + test + sec).

- [ ] **Step 5: Commit**

```bash
git add cmd/web/handler-workout_test.go
git commit -m "web: e2e tests for ad-hoc and mid-week-added workout starts"
```

---

## Self-Review Notes

Coverage against the spec:

| Spec requirement | Task(s) |
|------------------|---------|
| `Planner.PlanDay(date, used)` with category, count, periodization, deload, used-avoidance | Task 2 |
| `ErrAlreadyExists` sentinel | Task 1 |
| `Sessions.Create` repo method translating PK conflict | Task 3 |
| `seedDeloadWeights` extraction | Task 4 |
| `summarizeWeek` + `createAdHocSession` helpers | Task 5 |
| `StartSession` lazy-create + race recovery | Task 6 |
| Domain unit tests for `PlanDay` | Task 2 |
| Service tests: unscheduled today, mid-week added day, double-start race | Task 7 |
| Web e2e tests: "Start Extra Workout" + mid-week schedule change | Task 8 |
| Empty-pool error from `PlanDay` matches `Plan` shape | Task 2 (wrapping `errNoExercisesForCategory`) |
| No UI changes | Tasks 6–8 modify only handlers' upstream service; no template files touched |
| GET-side does not auto-create | Confirmed: only `StartSession` (POST) gains the lazy-create branch |
