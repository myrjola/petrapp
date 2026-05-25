# WeekPlan Aggregate — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce `WeekPlan` as the aggregate root for everything that scopes to a calendar week. Collapse cross-week service methods into single transactions and delete the per-user mutex (`userLocks`, `userMutex`, `sync.Map`).

**Architecture:** New `domain.WeekPlan` struct with `Sessions [7]Session` (rest days empty). New `repository.WeekPlanRepository` with the same `Update`-closure pattern `SessionRepository` uses today. Service methods become one-line closures over `WeekPlanRepository.Update`. `Session` keeps its per-day field set (storage stays denormalised); `WeekPlan` is a logical aggregate over the existing three tables — no schema change, no premigration.

**Tech Stack:** Go (standard library + `internal/domain`, `internal/repository`, `internal/service`, `internal/sqlite`).

**Reference:** `docs/superpowers/specs/2026-05-24-weekplan-aggregate-design.md`.

---

## File Structure

**New files:**

- `internal/domain/week_plan.go` — `WeekPlan` struct, read helpers, cross-week mutators, per-day dispatchers.
- `internal/domain/week_plan_test.go` — domain tests for `WeekPlan` (external `package domain_test`).
- `internal/repository/week_plans.go` — `sqliteWeekPlanRepository` (Get, Update, Create).
- `internal/repository/week_plans_test.go` — repo tests (external `package repository_test`).

**Modified files:**

- `internal/domain/planner.go` — `Planner.Plan(monday)` returns `WeekPlan` instead of `[]Session`.
- `internal/domain/planner_internal_test.go` — update assertions for the new return type.
- `internal/repository/repository.go` — add `WeekPlanRepository` interface; remove `Update`, `Create`, `CreateBatch`, `DeleteWeek` from `SessionRepository`; wire `WeekPlans` into `Repositories`.
- `internal/repository/sessions.go` — remove the deleted method implementations; keep reads.
- `internal/service/service.go` — delete `userLocks`, `userMutex`, `sync.Map` field, `sync` import; rewrite `RestartMesocycleAnchor`.
- `internal/service/sessions.go` — migrate `ResolveWeeklySchedule`, `StartSession`, `CompleteSession`, `SaveFeedback`, `MarkWarmupComplete`, `RegenerateWeeklyPlanIfUnstarted`, `StartDeloadNow`. Remove the inline `generateWeeklyPlan` helper (subsumed by `Planner.Plan` + `WeekPlanRepository.Create`).
- `internal/service/sets.go` — migrate `RecordSet`, `UpdateSetWeight`, `UpdateCompletedValue`.
- `internal/service/exercises.go` — migrate `SwapExercise`, `AddExercise`.
- `internal/service/sessions_test.go` — add concurrency test for `Regenerate` + `StartSession`.

**Naming convention:** the per-day struct stays `domain.Session` (no rename). `WeekPlan.Sessions` is a `[7]Session` array indexed by `Date - Monday`. Rest days have `Sessions[i] = Session{Date: ...}` (zero `ExerciseSets`).

---

## Task 1: Add `WeekPlan` struct + read helpers + cross-week mutators

**Files:**
- Create: `internal/domain/week_plan.go`
- Create: `internal/domain/week_plan_test.go`

- [ ] **Step 1: Create `internal/domain/week_plan.go` with the struct + read helpers + cross-week mutators**

```go
package domain

import (
	"time"
)

// WeekPlan is the aggregate root for one calendar week of a user's training.
// It owns seven Session values indexed by day-of-week (0 = Monday). Rest days
// carry an empty Session{Date: ...} with no ExerciseSets.
//
// All cross-week operations (regenerate, deload flip, mesocycle restart) are
// methods on *WeekPlan and are atomic when invoked inside a
// WeekPlanRepository.Update closure.
type WeekPlan struct {
	Monday   time.Time
	Sessions [7]Session
}

// PeriodizationType returns the week-wide periodization style. Every scheduled
// session shares the same value (enforced by the planner and by the repo).
// Returns the zero value when the week has no scheduled sessions.
func (wp *WeekPlan) PeriodizationType() PeriodizationType {
	for i := range wp.Sessions {
		if len(wp.Sessions[i].ExerciseSets) > 0 {
			return wp.Sessions[i].PeriodizationType
		}
	}
	return ""
}

// SessionOn returns a pointer to the session for date, or nil if date falls
// outside this WeekPlan's week. The returned pointer aliases into wp.Sessions
// so dispatchers can mutate in place.
func (wp *WeekPlan) SessionOn(date time.Time) *Session {
	d := StartOfDay(date)
	for i := range wp.Sessions {
		if wp.Sessions[i].Date.Equal(d) {
			return &wp.Sessions[i]
		}
	}
	return nil
}

// AnyStarted reports whether any session in the week has StartedAt set.
func (wp *WeekPlan) AnyStarted() bool {
	for i := range wp.Sessions {
		if !wp.Sessions[i].StartedAt.IsZero() {
			return true
		}
	}
	return false
}

// IsDeloadWeek reports whether every scheduled session is a deload session.
// Rest days are ignored. Returns false when the week has no scheduled sessions.
func (wp *WeekPlan) IsDeloadWeek() bool {
	scheduled := 0
	deload := 0
	for i := range wp.Sessions {
		if len(wp.Sessions[i].ExerciseSets) == 0 {
			continue
		}
		scheduled++
		if wp.Sessions[i].IsDeload {
			deload++
		}
	}
	return scheduled > 0 && scheduled == deload
}

// Replace replaces the plan with newPlan, preserving the Monday. Used by
// RegenerateIfUnstarted; callers normally don't invoke this directly.
func (wp *WeekPlan) Replace(newPlan WeekPlan) {
	wp.Sessions = newPlan.Sessions
}

// FlipDeloadFromToday sets IsDeload=true on every non-completed session whose
// Date is on or after today. Past sessions and completed sessions are left
// untouched. Idempotent.
func (wp *WeekPlan) FlipDeloadFromToday(today time.Time) error {
	t := StartOfDay(today)
	for i := range wp.Sessions {
		s := &wp.Sessions[i]
		if s.Date.Before(t) {
			continue
		}
		if s.Status() == SessionCompleted {
			continue
		}
		if err := s.SwitchToDeload(); err != nil {
			return err
		}
	}
	return nil
}

// ClearDeloadFromToday sets IsDeload=false on every non-completed session whose
// Date is on or after today. Counterpart to FlipDeloadFromToday. Idempotent.
func (wp *WeekPlan) ClearDeloadFromToday(today time.Time) error {
	t := StartOfDay(today)
	for i := range wp.Sessions {
		s := &wp.Sessions[i]
		if s.Date.Before(t) {
			continue
		}
		if s.Status() == SessionCompleted {
			continue
		}
		if err := s.ClearDeload(); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 2: Create `internal/domain/week_plan_test.go` covering the read helpers + mutators**

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func monday() time.Time {
	return time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
}

func sessionOn(offset int, started bool, completed bool, isDeload bool, hasSlots bool) domain.Session {
	d := monday().AddDate(0, 0, offset)
	s := domain.Session{ //nolint:exhaustruct // test scaffolding
		Date:     d,
		IsDeload: isDeload,
	}
	if started {
		s.StartedAt = d.Add(8 * time.Hour)
	}
	if completed {
		s.CompletedAt = d.Add(9 * time.Hour)
	}
	if hasSlots {
		s.ExerciseSets = []domain.ExerciseSet{{ID: 1, Sets: []domain.Set{{}}}} //nolint:exhaustruct
		s.PeriodizationType = domain.PeriodizationStrength
	}
	return s
}

func TestWeekPlan_SessionOn(t *testing.T) {
	t.Parallel()
	wp := domain.WeekPlan{Monday: monday()}
	wp.Sessions[2] = sessionOn(2, false, false, false, true)
	got := wp.SessionOn(monday().AddDate(0, 0, 2))
	if got == nil {
		t.Fatalf("expected session, got nil")
	}
	if !got.Date.Equal(monday().AddDate(0, 0, 2)) {
		t.Errorf("unexpected date: %v", got.Date)
	}
	if wp.SessionOn(monday().AddDate(0, 0, 8)) != nil {
		t.Error("expected nil for out-of-week date")
	}
}

func TestWeekPlan_AnyStarted(t *testing.T) {
	t.Parallel()
	wp := domain.WeekPlan{Monday: monday()}
	if wp.AnyStarted() {
		t.Error("empty week should not report started")
	}
	wp.Sessions[3] = sessionOn(3, true, false, false, true)
	if !wp.AnyStarted() {
		t.Error("week with one started session should report started")
	}
}

func TestWeekPlan_IsDeloadWeek(t *testing.T) {
	t.Parallel()
	wp := domain.WeekPlan{Monday: monday()}
	wp.Sessions[0] = sessionOn(0, false, false, true, true)
	wp.Sessions[2] = sessionOn(2, false, false, true, true)
	if !wp.IsDeloadWeek() {
		t.Error("all scheduled deload should report IsDeloadWeek=true")
	}
	wp.Sessions[2].IsDeload = false
	if wp.IsDeloadWeek() {
		t.Error("mixed deload state should report false")
	}
	empty := domain.WeekPlan{Monday: monday()}
	if empty.IsDeloadWeek() {
		t.Error("empty week should report false")
	}
}

func TestWeekPlan_FlipDeloadFromToday(t *testing.T) {
	t.Parallel()
	wp := domain.WeekPlan{Monday: monday()}
	wp.Sessions[0] = sessionOn(0, true, true, false, true)  // Mon: completed
	wp.Sessions[2] = sessionOn(2, true, false, false, true) // Wed: started, not completed
	wp.Sessions[4] = sessionOn(4, false, false, false, true) // Fri: not started

	today := monday().AddDate(0, 0, 2) // Wednesday
	if err := wp.FlipDeloadFromToday(today); err != nil {
		t.Fatalf("FlipDeloadFromToday: %v", err)
	}

	if wp.Sessions[0].IsDeload {
		t.Error("past completed session should be untouched")
	}
	if !wp.Sessions[2].IsDeload {
		t.Error("today's non-completed session should flip")
	}
	if !wp.Sessions[4].IsDeload {
		t.Error("future session should flip")
	}
}

func TestWeekPlan_ClearDeloadFromToday(t *testing.T) {
	t.Parallel()
	wp := domain.WeekPlan{Monday: monday()}
	wp.Sessions[0] = sessionOn(0, true, true, true, true)
	wp.Sessions[2] = sessionOn(2, false, false, true, true)
	wp.Sessions[4] = sessionOn(4, false, false, true, true)

	if err := wp.ClearDeloadFromToday(monday().AddDate(0, 0, 2)); err != nil {
		t.Fatalf("ClearDeloadFromToday: %v", err)
	}
	if !wp.Sessions[0].IsDeload {
		t.Error("past completed should keep IsDeload")
	}
	if wp.Sessions[2].IsDeload || wp.Sessions[4].IsDeload {
		t.Error("today and future should be cleared")
	}
}
```

- [ ] **Step 3: Run the tests, confirm pass**

Run: `go test ./internal/domain -run TestWeekPlan -v`
Expected: PASS (5 tests).

- [ ] **Step 4: Run the full domain suite, confirm no regressions**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/week_plan.go internal/domain/week_plan_test.go
git commit -m "domain: add WeekPlan aggregate with read helpers and cross-week mutators"
```

---

## Task 2: Add WeekPlan per-day dispatcher methods

**Files:**
- Modify: `internal/domain/week_plan.go` (append)
- Modify: `internal/domain/week_plan_test.go` (append)

Each dispatcher navigates to the right `Session` via `SessionOn` and delegates to the existing method on `*Session`. Returns `ErrNotFound` when the date is outside the week or has no scheduled session.

- [ ] **Step 1: Append dispatcher methods to `internal/domain/week_plan.go`**

```go
// Start marks the session for date as begun. Returns ErrNotFound when no
// session exists for date.
func (wp *WeekPlan) Start(date time.Time, now time.Time) error {
	s := wp.SessionOn(date)
	if s == nil {
		return ErrNotFound
	}
	return s.Start(now)
}

// Complete marks the session for date as finished. Returns ErrNotFound when no
// session exists for date.
func (wp *WeekPlan) Complete(date time.Time, now time.Time) error {
	s := wp.SessionOn(date)
	if s == nil {
		return ErrNotFound
	}
	return s.Complete(now)
}

// SetDifficulty records the post-session difficulty rating.
func (wp *WeekPlan) SetDifficulty(date time.Time, rating int) error {
	s := wp.SessionOn(date)
	if s == nil {
		return ErrNotFound
	}
	return s.SetDifficulty(rating)
}

// MarkWarmupComplete records the warmup completion timestamp for the slot.
func (wp *WeekPlan) MarkWarmupComplete(date time.Time, slotID int, now time.Time) error {
	s := wp.SessionOn(date)
	if s == nil {
		return ErrNotFound
	}
	return s.MarkWarmupComplete(slotID, now)
}

// RecordSet records the completion of a single set.
func (wp *WeekPlan) RecordSet(
	date time.Time, slotID, setIndex int,
	signal *Signal, weightKg *float64, completedValue int, now time.Time,
) error {
	s := wp.SessionOn(date)
	if s == nil {
		return ErrNotFound
	}
	return s.RecordSet(slotID, setIndex, signal, weightKg, completedValue, now)
}

// UpdateSetWeight overwrites the weight on a single set within a slot.
func (wp *WeekPlan) UpdateSetWeight(date time.Time, slotID, setIndex int, weightKg float64) error {
	s := wp.SessionOn(date)
	if s == nil {
		return ErrNotFound
	}
	return s.UpdateSetWeight(slotID, setIndex, weightKg)
}

// UpdateCompletedValue records the actual reps (or seconds) on a set.
func (wp *WeekPlan) UpdateCompletedValue(date time.Time, slotID, setIndex, value int, now time.Time) error {
	s := wp.SessionOn(date)
	if s == nil {
		return ErrNotFound
	}
	return s.UpdateCompletedValue(slotID, setIndex, value, now)
}

// SwapExerciseInSlot replaces the exercise occupying the slot.
func (wp *WeekPlan) SwapExerciseInSlot(date time.Time, slotID int, newEx Exercise, sets []Set) error {
	s := wp.SessionOn(date)
	if s == nil {
		return ErrNotFound
	}
	return s.SwapExerciseInSlot(slotID, newEx, sets)
}

// AddExercise appends a new exercise slot to the session for date.
func (wp *WeekPlan) AddExercise(date time.Time, ex Exercise, sets []Set) error {
	s := wp.SessionOn(date)
	if s == nil {
		return ErrNotFound
	}
	return s.AddExercise(ex, sets)
}
```

- [ ] **Step 2: Append tests covering the dispatcher path**

```go
func TestWeekPlan_Dispatchers_NotFound(t *testing.T) {
	t.Parallel()
	wp := domain.WeekPlan{Monday: monday()}
	outOfWeek := monday().AddDate(0, 0, 8)
	if err := wp.Start(outOfWeek, time.Now()); err != domain.ErrNotFound {
		t.Errorf("Start out-of-week: got %v, want ErrNotFound", err)
	}
	if err := wp.Complete(outOfWeek, time.Now()); err != domain.ErrNotFound {
		t.Errorf("Complete out-of-week: got %v, want ErrNotFound", err)
	}
}

func TestWeekPlan_Dispatchers_DelegateToSession(t *testing.T) {
	t.Parallel()
	wp := domain.WeekPlan{Monday: monday()}
	wp.Sessions[2] = sessionOn(2, false, false, false, true)
	now := monday().AddDate(0, 0, 2).Add(10 * time.Hour)
	if err := wp.Start(wp.Sessions[2].Date, now); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if wp.Sessions[2].StartedAt.IsZero() {
		t.Error("Start should set StartedAt on the underlying session")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/domain -run TestWeekPlan -v`
Expected: PASS (7 tests now).

- [ ] **Step 4: Commit**

```bash
git add internal/domain/week_plan.go internal/domain/week_plan_test.go
git commit -m "domain: add WeekPlan per-day dispatcher methods"
```

---

## Task 3: Change `Planner.Plan` to return `WeekPlan`

**Files:**
- Modify: `internal/domain/planner.go` (the `Plan` method)
- Modify: `internal/domain/planner_internal_test.go` (assertion updates)

The planner today returns `[]Session` containing only scheduled days. After this change it returns a `WeekPlan` whose `Sessions` array is length 7 with rest days populated as `Session{Date: ...}` (no slots).

- [ ] **Step 1: Read current `Plan` signature and body so the rewrite preserves logic**

Run: `sed -n '49,160p' internal/domain/planner.go`

Note where the function currently returns `[]Session`. Identify the existing `firstSessionPeriodizationType` call and the per-day loop that builds the result slice.

- [ ] **Step 2: Update `Plan` to assemble a `WeekPlan`**

Replace the return type and the result assembly. Roughly:

```go
// Plan generates one Session per scheduled workout day for the week beginning on
// startingDate and returns them packaged into a WeekPlan. Sessions[i] for
// non-scheduled days carries only Date.
func (wp *Planner) Plan(startingDate time.Time) (WeekPlan, error) {
	if startingDate.Weekday() != time.Monday {
		return WeekPlan{}, fmt.Errorf("startingDate must be a Monday, got %s", startingDate.Weekday())
	}
	// ... existing scheduling logic unchanged, but instead of appending to
	// []Session, write into result.Sessions[i] where i = day offset 0..6.

	result := WeekPlan{Monday: startingDate}
	for i := range 7 {
		result.Sessions[i] = Session{Date: startingDate.AddDate(0, 0, i)} //nolint:exhaustruct
	}
	// ... after planning each scheduled day, write:
	//     result.Sessions[dayOffset] = plannedSession
	return result, nil
}
```

Keep the existing internal logic (category determination, muscle group allocation, exercise selection, set generation) exactly as it is. Only the result-shape changes.

- [ ] **Step 3: Update `planner_internal_test.go` assertions**

The existing tests look like:
```go
sessions, err := planner.Plan(monday)
// ... checks on len(sessions), sessions[0].Date, etc.
```

Rewrite each as:
```go
plan, err := planner.Plan(monday)
// ... build a scheduled-sessions slice from plan.Sessions for assertions:
var scheduled []domain.Session
for i := range plan.Sessions {
	if len(plan.Sessions[i].ExerciseSets) > 0 {
		scheduled = append(scheduled, plan.Sessions[i])
	}
}
// then assert on `scheduled` as before
```

Run: `grep -n "planner.Plan\|wp.Plan\|p.Plan(" internal/domain/planner_internal_test.go` to enumerate every call site.

- [ ] **Step 4: Run planner tests, confirm pass**

Run: `go test ./internal/domain -run TestPlanner -v`
Expected: PASS.

- [ ] **Step 5: Run full domain suite**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 6: Note the broken callers and proceed regardless (they're rewritten in Task 7+)**

Run: `go build ./...`
Expected: build errors only in `internal/service/sessions.go` and `internal/service/exercises.go` referencing `planner.Plan(...).` returning `[]Session`. These will be fixed in Tasks 7 and 9.

To keep the tree compiling for intermediate commits, **temporarily** update `internal/service/sessions.go` `generateWeeklyPlan` to call:

```go
plan, err := planner.Plan(monday)
if err != nil {
    return fmt.Errorf("plan week: %w", err)
}
plannedSessions := make([]domain.Session, 0, 7)
for i := range plan.Sessions {
    if len(plan.Sessions[i].ExerciseSets) > 0 {
        plannedSessions = append(plannedSessions, plan.Sessions[i])
    }
}
```

…and `internal/service/sessions.go` `createAdHocSession` is unaffected (it uses `planner.PlanDay`, not `Plan`).

- [ ] **Step 7: Build, run full test suite**

Run: `go build ./...`
Expected: success.

Run: `go test ./...`
Expected: PASS (the temporary adapter preserves prior behaviour).

- [ ] **Step 8: Commit**

```bash
git add internal/domain/planner.go internal/domain/planner_internal_test.go internal/service/sessions.go
git commit -m "domain: Planner.Plan returns WeekPlan; temporary adapter in service"
```

---

## Task 4: Add `WeekPlanRepository.Get`

**Files:**
- Create: `internal/repository/week_plans.go`
- Create: `internal/repository/week_plans_test.go`
- Modify: `internal/repository/repository.go` (add interface + wire into Repositories)

- [ ] **Step 1: Add the interface declaration to `internal/repository/repository.go`**

After the `SessionRepository` interface block (around line 82), insert:

```go
// WeekPlanRepository persists the full week aggregate. The Update closure
// pattern mirrors SessionRepository.Update but at week scope: load the seven
// days into a domain.WeekPlan, run fn under a single transaction, persist the
// diff on nil.
type WeekPlanRepository interface {
	// Get returns the lazily-materialised week. Sessions is always length 7;
	// non-scheduled dates carry an empty Session{Date: ...}. Returns
	// domain.ErrNotFound when no workout_sessions row exists for the week.
	Get(ctx context.Context, monday time.Time) (domain.WeekPlan, error)
}
```

(Update + Create added in later tasks; build keeps passing because the implementation will satisfy the growing interface.)

Add a `WeekPlans WeekPlanRepository` field to the `Repositories` struct and wire it in `New`:

```go
type Repositories struct {
	Sessions          SessionRepository
	WeekPlans         WeekPlanRepository
	// ...
}

func New(db *sqlite.Database) *Repositories {
	// ...
	weekPlans := newSQLiteWeekPlanRepository(db)
	return &Repositories{
		// ...
		Sessions:          sessions,
		WeekPlans:         weekPlans,
		// ...
	}
}
```

- [ ] **Step 2: Create `internal/repository/week_plans.go` skeleton + `Get`**

```go
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteWeekPlanRepository struct {
	baseRepository
}

func newSQLiteWeekPlanRepository(db *sqlite.Database) *sqliteWeekPlanRepository {
	return &sqliteWeekPlanRepository{baseRepository: newBaseRepository(db)}
}

// Get loads the WeekPlan for the week beginning on monday. Sessions is always
// length 7; non-scheduled dates carry an empty Session{Date: ...}. Returns
// domain.ErrNotFound when no workout_sessions row exists for the week.
func (r *sqliteWeekPlanRepository) Get(ctx context.Context, monday time.Time) (domain.WeekPlan, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	sunday := monday.AddDate(0, 0, 6)

	// Reuse the existing range-scan helper from sessions.go.
	sessionRows, err := r.listSessionRowsBetween(ctx, userID, monday, sunday)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("list session rows for week: %w", err)
	}
	if len(sessionRows) == 0 {
		return domain.WeekPlan{}, fmt.Errorf("week starting %s: %w", monday.Format(time.DateOnly), domain.ErrNotFound)
	}

	setsByDate, err := r.loadExerciseSetsSince(ctx, r.db.ReadOnly, userID, monday)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("load exercise sets for week: %w", err)
	}

	wp := domain.WeekPlan{Monday: monday}
	for i := range 7 {
		wp.Sessions[i] = domain.Session{Date: monday.AddDate(0, 0, i)} //nolint:exhaustruct
	}
	for _, sess := range sessionRows {
		offset := int(sess.Date.Sub(monday).Hours() / 24)
		if offset < 0 || offset > 6 {
			continue
		}
		sess.ExerciseSets = setsByDate[formatDate(sess.Date)]
		wp.Sessions[offset] = sess
	}
	return wp, nil
}
```

Add the helper `listSessionRowsBetween` to `internal/repository/shared.go` as a method on `baseRepository` so both `sqliteSessionRepository` and `sqliteWeekPlanRepository` can call it. **Before pasting, read `listSessionRows` in `internal/repository/sessions.go` and verify the column list / scan order match `parseSessionRow`'s parameter order exactly.**

```go
// listSessionRowsBetween returns sessions whose workout_date is in [from, to]
// inclusive, oldest first. ExerciseSets is left nil — caller hydrates.
func (r baseRepository) listSessionRowsBetween(
	ctx context.Context, userID []byte, from, to time.Time,
) ([]domain.Session, error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at,
		       periodization_type, is_deload
		FROM workout_sessions
		WHERE user_id = ? AND workout_date BETWEEN ? AND ?
		ORDER BY workout_date ASC`,
		userID, formatDate(from), formatDate(to))
	if err != nil {
		return nil, fmt.Errorf("query workout_sessions between: %w", err)
	}
	defer rows.Close()

	var sessions []domain.Session
	for rows.Next() {
		var (
			workoutDateStr    string
			difficultyRating  sql.NullInt32
			startedAtStr      sql.NullString
			completedAtStr    sql.NullString
			periodizationType domain.PeriodizationType
			isDeload          bool
		)
		if err = rows.Scan(&workoutDateStr, &difficultyRating, &startedAtStr,
			&completedAtStr, &periodizationType, &isDeload); err != nil {
			return nil, fmt.Errorf("scan workout_sessions row: %w", err)
		}
		sess, parseErr := parseSessionRow(workoutDateStr, difficultyRating,
			startedAtStr, completedAtStr, periodizationType, isDeload)
		if parseErr != nil {
			return nil, parseErr
		}
		sessions = append(sessions, sess)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workout_sessions rows: %w", err)
	}
	return sessions, nil
}
```

- [ ] **Step 3: Create `internal/repository/week_plans_test.go` with a Get round-trip test and a reusable seed helper**

```go
package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// seedScheduledSession inserts a workout_sessions row + one workout_exercise
// (the Deadlift seed exercise) for the authenticated user on date. Returns the
// workout_exercise.id. Used by WeekPlan tests to populate a scheduled day.
// Once Task 6 lands, future tests can prefer WeekPlanRepository.Create.
func seedScheduledSession(ctx context.Context, t *testing.T, db *sqlite.Database, date time.Time) int {
	t.Helper()
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := date.Format("2006-01-02")
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, periodization_type, is_deload)
		 VALUES (?, ?, 'strength', 0) ON CONFLICT DO NOTHING`,
		userID, dateStr); err != nil {
		t.Fatalf("insert session %s: %v", dateStr, err)
	}
	var exerciseID int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Deadlift'`).Scan(&exerciseID); err != nil {
		t.Fatalf("fetch Deadlift: %v", err)
	}
	var weID int
	if err := db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		 VALUES (?, ?, ?) RETURNING id`,
		userID, dateStr, exerciseID).Scan(&weID); err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, target_value, weight_kg)
		 VALUES (?, 1, 5, 60.0)`, weID); err != nil {
		t.Fatalf("insert exercise_set: %v", err)
	}
	return weID
}

func TestWeekPlanRepository_Get_ReturnsErrNotFoundForEmptyWeek(t *testing.T) {
	ctx, repos := setupTestRepos(t)
	_, err := repos.WeekPlans.Get(ctx, time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestWeekPlanRepository_Get_HydratesScheduledDays(t *testing.T) {
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	seedScheduledSession(ctx, t, db, monday)
	seedScheduledSession(ctx, t, db, monday.AddDate(0, 0, 2)) // Wednesday

	wp, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !wp.Monday.Equal(monday) {
		t.Errorf("Monday: got %v, want %v", wp.Monday, monday)
	}
	if len(wp.Sessions[0].ExerciseSets) == 0 {
		t.Error("Monday should be scheduled")
	}
	if len(wp.Sessions[1].ExerciseSets) != 0 {
		t.Error("Tuesday should be a rest day (empty)")
	}
	if !wp.Sessions[1].Date.Equal(monday.AddDate(0, 0, 1)) {
		t.Errorf("Tuesday rest day date: got %v", wp.Sessions[1].Date)
	}
	if len(wp.Sessions[2].ExerciseSets) == 0 {
		t.Error("Wednesday should be scheduled")
	}
}
```

Note: `setupTestReposWithDB` is the existing helper in `helpers_test.go` that also returns the raw `*sqlite.Database`.

- [ ] **Step 4: Build, run tests**

Run: `go build ./...`
Expected: success.

Run: `go test ./internal/repository -run TestWeekPlanRepository -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/week_plans.go internal/repository/week_plans_test.go internal/repository/repository.go internal/repository/sessions.go
git commit -m "repository: add WeekPlanRepository.Get with lazy materialisation"
```

---

## Task 5: Add `WeekPlanRepository.Update`

**Files:**
- Modify: `internal/repository/repository.go` (extend the interface)
- Modify: `internal/repository/week_plans.go` (add `Update`)
- Modify: `internal/repository/week_plans_test.go` (add closure tests)

The `Update` closure pattern mirrors `SessionRepository.Update`. Persist via delete-then-reinsert across the week's date range (CASCADE cleans children).

- [ ] **Step 1: Extend the interface**

```go
type WeekPlanRepository interface {
	Get(ctx context.Context, monday time.Time) (domain.WeekPlan, error)
	// Update loads the week, runs fn under a single transaction, and commits
	// on nil. Returning an error rolls back. Domain sentinels propagate
	// unchanged so callers can errors.Is on them.
	Update(ctx context.Context, monday time.Time, fn func(*domain.WeekPlan) error) error
}
```

- [ ] **Step 2: Implement `Update`**

Append to `internal/repository/week_plans.go`:

```go
// Update loads the WeekPlan for monday inside a single BEGIN IMMEDIATE
// transaction, runs fn, then persists the result via delete-then-reinsert
// across the week's date range. Slot IDs are preserved via INSERT ... RETURNING id
// (same trick as SessionRepository.Update).
func (r *sqliteWeekPlanRepository) Update(
	ctx context.Context, monday time.Time, fn func(*domain.WeekPlan) error,
) (err error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	sunday := monday.AddDate(0, 0, 6)

	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	// Load inside the tx so the read sees a consistent snapshot.
	wp, err := r.getInTx(ctx, tx, userID, monday)
	if err != nil {
		return fmt.Errorf("get week for update: %w", err)
	}

	if err = fn(&wp); err != nil {
		return err //nolint:wrapcheck // domain sentinels propagate unchanged.
	}

	if err = r.deleteWeekInTx(ctx, tx, userID, monday, sunday); err != nil {
		return fmt.Errorf("delete week for rewrite: %w", err)
	}
	for i := range wp.Sessions {
		if len(wp.Sessions[i].ExerciseSets) == 0 && wp.Sessions[i].StartedAt.IsZero() && wp.Sessions[i].CompletedAt.IsZero() {
			continue // rest day, nothing to persist
		}
		if err = r.insertSessionInTx(ctx, tx, wp.Sessions[i]); err != nil {
			return fmt.Errorf("insert session %s: %w", formatDate(wp.Sessions[i].Date), err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit week: %w", err)
	}
	return nil
}
```

`getInTx`, `deleteWeekInTx`, and `insertSessionInTx` are helper methods. Two are extractions of existing private logic in `sessions.go`:

- `getInTx`: same as `Get`, but uses the passed `tx` as the queryer (parameterise the existing helpers if needed).
- `deleteWeekInTx`: `DELETE FROM workout_sessions WHERE user_id=? AND workout_date BETWEEN ? AND ?` (CASCADE clears children). The existing `SessionRepository.DeleteWeek` already does this — extract or inline.
- `insertSessionInTx`: lift the existing `r.insertSession(ctx, tx, sess)` body in `sessions.go`. Make it a method on `baseRepository` so both repositories can call it.

Place all three in `internal/repository/shared.go` or a new `internal/repository/session_persistence.go` so both repos share one canonical impl. Avoid duplication.

- [ ] **Step 3: Add tests for the closure pattern**

```go
func TestWeekPlanRepository_Update_CommitsOnNil(t *testing.T) {
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	seedScheduledSession(ctx, t, db, monday)
	err := repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		return wp.Start(monday, time.Now().UTC())
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	reloaded, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if reloaded.Sessions[0].StartedAt.IsZero() {
		t.Error("Start should have persisted")
	}
}

func TestWeekPlanRepository_Update_RollsBackOnError(t *testing.T) {
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	seedScheduledSession(ctx, t, db, monday)
	sentinel := errors.New("rollback me")
	err := repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		_ = wp.Start(monday, time.Now().UTC())
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Update: got %v, want sentinel", err)
	}
	reloaded, _ := repos.WeekPlans.Get(ctx, monday)
	if !reloaded.Sessions[0].StartedAt.IsZero() {
		t.Error("rollback should have left StartedAt unset")
	}
}

func TestWeekPlanRepository_Update_PreservesSlotIDs(t *testing.T) {
	ctx, db, repos := setupTestReposWithDB(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	seedScheduledSession(ctx, t, db, monday)

	wp, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(wp.Sessions[0].ExerciseSets) == 0 {
		t.Fatalf("seed should have produced a slot")
	}
	originalSlotID := wp.Sessions[0].ExerciseSets[0].ID

	err = repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		return wp.Start(monday, time.Now().UTC())
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	reloaded, _ := repos.WeekPlans.Get(ctx, monday)
	if reloaded.Sessions[0].ExerciseSets[0].ID != originalSlotID {
		t.Errorf("slot ID changed: got %d, want %d", reloaded.Sessions[0].ExerciseSets[0].ID, originalSlotID)
	}
}
```

- [ ] **Step 4: Build, run repo tests**

Run: `go build ./...`
Expected: success.

Run: `go test ./internal/repository -v -run TestWeekPlanRepository`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/
git commit -m "repository: add WeekPlanRepository.Update with closure pattern"
```

---

## Task 6: Add `WeekPlanRepository.Create`

**Files:**
- Modify: `internal/repository/repository.go` (extend interface)
- Modify: `internal/repository/week_plans.go` (add Create)
- Modify: `internal/repository/week_plans_test.go` (add test)

- [ ] **Step 1: Extend the interface**

```go
type WeekPlanRepository interface {
	Get(ctx context.Context, monday time.Time) (domain.WeekPlan, error)
	Update(ctx context.Context, monday time.Time, fn func(*domain.WeekPlan) error) error
	// Create persists a freshly-planned WeekPlan. Returns domain.ErrAlreadyExists
	// (wrapped) when any session row already exists for the week, so callers can
	// recover from concurrent first-time generation races.
	Create(ctx context.Context, plan domain.WeekPlan) error
}
```

- [ ] **Step 2: Implement `Create`**

```go
func (r *sqliteWeekPlanRepository) Create(ctx context.Context, plan domain.WeekPlan) (err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()
	for i := range plan.Sessions {
		if len(plan.Sessions[i].ExerciseSets) == 0 {
			continue
		}
		if err = r.insertSessionInTx(ctx, tx, plan.Sessions[i]); err != nil {
			var sqliteErr sqlite3.Error
			if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
				return fmt.Errorf("create week starting %s: %w", formatDate(plan.Monday), domain.ErrAlreadyExists)
			}
			return fmt.Errorf("insert session %s: %w", formatDate(plan.Sessions[i].Date), err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit create week: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Add `Create` tests**

```go
// buildPlanWithOneSlot constructs a minimal in-memory WeekPlan with one
// scheduled session on monday containing the Deadlift exercise + one set.
// Used by Create tests.
func buildPlanWithOneSlot(ctx context.Context, t *testing.T, repos *repository.Repositories, monday time.Time) domain.WeekPlan {
	t.Helper()
	exs, err := repos.Exercises.List(ctx)
	if err != nil {
		t.Fatalf("list exercises: %v", err)
	}
	var deadlift domain.Exercise
	for _, e := range exs {
		if e.Name == "Deadlift" {
			deadlift = e
			break
		}
	}
	if deadlift.ID == 0 {
		t.Fatalf("Deadlift seed exercise not found")
	}
	w := 60.0
	plan := domain.WeekPlan{Monday: monday}
	for i := range 7 {
		plan.Sessions[i] = domain.Session{Date: monday.AddDate(0, 0, i)} //nolint:exhaustruct
	}
	plan.Sessions[0] = domain.Session{ //nolint:exhaustruct
		Date:              monday,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{{ //nolint:exhaustruct
			Exercise: deadlift,
			Sets:     []domain.Set{{TargetValue: 5, WeightKg: &w}}, //nolint:exhaustruct
		}},
	}
	return plan
}

func TestWeekPlanRepository_Create_PersistsWeek(t *testing.T) {
	ctx, repos := setupTestRepos(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	plan := buildPlanWithOneSlot(ctx, t, repos, monday)
	if err := repos.WeekPlans.Create(ctx, plan); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Sessions[0].ExerciseSets) == 0 {
		t.Error("session not persisted")
	}
}

func TestWeekPlanRepository_Create_ReturnsErrAlreadyExistsOnConflict(t *testing.T) {
	ctx, repos := setupTestRepos(t)
	monday := time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC)
	plan := buildPlanWithOneSlot(ctx, t, repos, monday)
	if err := repos.WeekPlans.Create(ctx, plan); err != nil {
		t.Fatalf("Create first: %v", err)
	}
	err := repos.WeekPlans.Create(ctx, plan)
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("Create second: got %v, want ErrAlreadyExists", err)
	}
}
```

Add the `"github.com/myrjola/petrapp/internal/repository"` import to `week_plans_test.go` for the helper signature.

- [ ] **Step 4: Build, run tests**

Run: `go test ./internal/repository -run TestWeekPlanRepository -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/
git commit -m "repository: add WeekPlanRepository.Create with ErrAlreadyExists translation"
```

---

## Task 7: Migrate `ResolveWeeklySchedule` and `generateWeeklyPlan`

**Files:**
- Modify: `internal/service/sessions.go` — replace `ResolveWeeklySchedule` body and delete `generateWeeklyPlan` helper.

After this task, `ResolveWeeklySchedule` returns `domain.WeekPlan` instead of `[]domain.Session`. Update every caller (handlers, tests) accordingly. Grep first.

- [ ] **Step 1: Enumerate callers**

Run: `grep -rn "ResolveWeeklySchedule" --include="*.go"`

Record every call site. Each caller will need to migrate from `[]domain.Session` to `domain.WeekPlan` (or use `plan.Sessions[:]` if it needs a slice).

- [ ] **Step 2: Rewrite `ResolveWeeklySchedule` in `internal/service/sessions.go`**

```go
// ResolveWeeklySchedule returns the WeekPlan for the current week. If no plan
// exists yet, generates one via the Planner and persists it; tolerates a
// concurrent create race by re-reading on ErrAlreadyExists.
func (s *Service) ResolveWeeklySchedule(ctx context.Context) (domain.WeekPlan, error) {
	monday := domain.MondayOf(time.Now())

	plan, err := s.repos.WeekPlans.Get(ctx, monday)
	if err == nil {
		return plan, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.WeekPlan{}, fmt.Errorf("get week %s: %w", monday.Format(time.DateOnly), err)
	}

	newPlan, err := s.planWeek(ctx, monday)
	if err != nil {
		return domain.WeekPlan{}, err
	}
	if err = s.repos.WeekPlans.Create(ctx, newPlan); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.WeekPlan{}, fmt.Errorf("create week %s: %w", monday.Format(time.DateOnly), err)
	}
	// Re-read so deload-seeded weights are sourced from persisted state.
	plan, err = s.repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("re-get week after create: %w", err)
	}
	return plan, nil
}

// planWeek builds an in-memory WeekPlan using the Planner and seeds deload
// weights. Replaces the old generateWeeklyPlan helper.
func (s *Service) planWeek(ctx context.Context, monday time.Time) (domain.WeekPlan, error) {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("get preferences: %w", err)
	}
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("get exercises: %w", err)
	}
	targets, err := s.repos.MuscleTargets.List(ctx)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("get muscle group targets: %w", err)
	}
	planner := domain.NewPlanner(prefs, exercises, targets)
	plan, err := planner.Plan(monday)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("plan week: %w", err)
	}
	for i := range plan.Sessions {
		if !plan.Sessions[i].IsDeload || len(plan.Sessions[i].ExerciseSets) == 0 {
			continue
		}
		if err = s.seedDeloadWeights(ctx, &plan.Sessions[i]); err != nil {
			return domain.WeekPlan{}, err
		}
	}
	return plan, nil
}
```

Delete the old `generateWeeklyPlan` helper and the temporary adapter that Task 3 inserted.

- [ ] **Step 3: Update every `ResolveWeeklySchedule` caller**

Each caller previously got `[]domain.Session`. For callers that want all 7 elements as a slice, replace:
```go
schedule, err := app.service.ResolveWeeklySchedule(ctx)
```
with:
```go
plan, err := app.service.ResolveWeeklySchedule(ctx)
// then use plan.Sessions[:] or plan directly
```

Likely callers: `cmd/web/handler-schedule.go` and possibly a test in `cmd/web/`. Inspect each.

- [ ] **Step 4: Build, run tests**

Run: `go build ./...`
Expected: success.

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/sessions.go cmd/web/
git commit -m "service: ResolveWeeklySchedule returns WeekPlan; remove generateWeeklyPlan"
```

---

## Task 8: Migrate per-day mutation methods

**Files:**
- Modify: `internal/service/sessions.go` — `StartSession`, `CompleteSession`, `SaveFeedback`, `MarkWarmupComplete`.
- Modify: `internal/service/sets.go` — `RecordSet`, `UpdateSetWeight`, `UpdateCompletedValue`.

Pattern: every method that today calls `s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error { ... })` becomes `s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error { ... })`. The closure body invokes the matching `WeekPlan` dispatcher.

- [ ] **Step 1: Migrate `CompleteSession`**

Today:
```go
if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
    now := time.Now()
    if sess.StartedAt.IsZero() {
        if err := sess.Start(now); err != nil { ... }
    }
    return sess.Complete(now)
}); err != nil { ... }
```

Rewrite as:
```go
monday := domain.MondayOf(date)
err := s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
    now := time.Now()
    sess := wp.SessionOn(date)
    if sess == nil {
        return fmt.Errorf("no session for %s", date.Format(time.DateOnly))
    }
    if sess.StartedAt.IsZero() {
        if err := sess.Start(now); err != nil {
            return fmt.Errorf("auto-start before complete: %w", err)
        }
    }
    return sess.Complete(now)
})
```

- [ ] **Step 2: Migrate `SaveFeedback`**

```go
return s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
    return wp.SetDifficulty(date, difficulty)
})
```

(Wrap the outer error with `fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)` to match existing conventions.)

- [ ] **Step 3: Migrate `MarkWarmupComplete`**

Same pattern; the pre/post-state capture for rest-push scheduling stays inside the closure:

```go
err := s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
    sess := wp.SessionOn(date)
    if sess == nil {
        return domain.ErrNotFound
    }
    if slot, ok := sess.Slot(workoutExerciseID); ok {
        wasComplete = slot.WarmupCompletedAt != nil
    }
    periodization = sess.PeriodizationType
    sessionDeload = sess.IsDeload

    if mErr := sess.MarkWarmupComplete(workoutExerciseID, now); mErr != nil {
        return mErr //nolint:wrapcheck
    }
    postSlot, postSlotOK = sess.Slot(workoutExerciseID)
    return nil
})
```

- [ ] **Step 4: Migrate `RecordSet`, `UpdateSetWeight`, `UpdateCompletedValue`**

Same translation in `internal/service/sets.go`. Each becomes a one-line closure invoking the matching `WeekPlan` dispatcher.

- [ ] **Step 5: Migrate `StartSession`**

This one is special — it has the lazy-create branch for ad-hoc sessions. Today:

```go
monday := domain.MondayOf(date)
existing, err := s.repos.Sessions.List(ctx, monday)
// ... summarizeWeek ...
if weekCount == 0 {
    if err = s.generateWeeklyPlan(ctx, monday); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
        return ...
    }
    existing, err = s.repos.Sessions.List(ctx, monday)
    // ... re-summarize ...
}
if !hasDate {
    if err = s.createAdHocSession(ctx, date, used); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
        return ...
    }
}
err = s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
    return sess.Start(time.Now())
})
```

Rewrite using WeekPlan:

```go
monday := domain.MondayOf(date)
plan, err := s.repos.WeekPlans.Get(ctx, monday)
if err != nil && !errors.Is(err, domain.ErrNotFound) {
    return fmt.Errorf("get week of %s: %w", date.Format(time.DateOnly), err)
}
weekMissing := errors.Is(err, domain.ErrNotFound)
if weekMissing {
    newPlan, planErr := s.planWeek(ctx, monday)
    if planErr != nil {
        return planErr
    }
    if createErr := s.repos.WeekPlans.Create(ctx, newPlan); createErr != nil && !errors.Is(createErr, domain.ErrAlreadyExists) {
        return fmt.Errorf("create week for %s: %w", date.Format(time.DateOnly), createErr)
    }
    plan, err = s.repos.WeekPlans.Get(ctx, monday)
    if err != nil {
        return fmt.Errorf("re-get week for %s: %w", date.Format(time.DateOnly), err)
    }
}
// hasDate check + ad-hoc creation:
if hasDate := plan.SessionOn(date) != nil && len(plan.SessionOn(date).ExerciseSets) > 0; !hasDate {
    used := usedExerciseIDs(plan)
    if err = s.createAdHocSession(ctx, date, used); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
        return fmt.Errorf("create ad-hoc %s: %w", date.Format(time.DateOnly), err)
    }
}

err = s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
    return wp.Start(date, time.Now())
})
if errors.Is(err, domain.ErrAlreadyStarted) {
    return nil
}
if err != nil {
    return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
}
return nil
```

Add a helper:
```go
func usedExerciseIDs(plan domain.WeekPlan) map[int]bool {
    used := make(map[int]bool)
    for i := range plan.Sessions {
        for _, es := range plan.Sessions[i].ExerciseSets {
            used[es.Exercise.ID] = true
        }
    }
    return used
}
```

`createAdHocSession` continues to call `s.repos.Sessions.Create` (one-day insert) — leave that path alone until Task 11. Or: rewrite it to use `WeekPlans.Update` (load + add the session + save). The latter is cleaner; do it. Pattern:

```go
err = s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
    sess, err := s.planSingleDay(ctx, date, used)
    if err != nil {
        return err
    }
    // Place into the WeekPlan at the right offset.
    offset := int(date.Sub(wp.Monday).Hours() / 24)
    wp.Sessions[offset] = sess
    return nil
})
```

…where `planSingleDay` is the contents of today's `createAdHocSession` minus the final `Sessions.Create` (i.e. just the planner call + deload seeding, returning the in-memory `Session`).

This eliminates the last write-side caller of `SessionRepository`.

- [ ] **Step 6: Build, run all tests**

Run: `go build ./...`
Expected: success.

Run: `make test`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/service/
git commit -m "service: migrate per-day mutations and StartSession to WeekPlanRepository"
```

---

## Task 9: Migrate `SwapExercise` and `AddExercise`

**Files:**
- Modify: `internal/service/exercises.go`.

These currently use `s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error { return sess.SwapExerciseInSlot(...) })`. Same translation as Task 8.

- [ ] **Step 1: Rewrite each `Sessions.Update` call as a `WeekPlans.Update` call invoking the matching dispatcher**

For `SwapExercise`:

```go
err = s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
    return wp.SwapExerciseInSlot(date, slotID, newEx, sets)
})
```

For `AddExercise`:

```go
err = s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
    return wp.AddExercise(date, ex, sets)
})
```

Read the existing surrounding logic (the read-after-write to fetch the assigned slot ID, validation, etc.) and preserve it. For reads that used `s.repos.Sessions.Get(ctx, date)`, replace with:

```go
plan, err := s.repos.WeekPlans.Get(ctx, domain.MondayOf(date))
if err != nil { ... }
sess := plan.SessionOn(date)
if sess == nil { ... }
```

- [ ] **Step 2: Build, run tests**

Run: `make test`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/service/exercises.go
git commit -m "service: migrate SwapExercise and AddExercise to WeekPlanRepository"
```

---

## Task 10: Migrate `RegenerateWeeklyPlanIfUnstarted`

**Files:**
- Modify: `internal/service/sessions.go` — replace body with a single `WeekPlans.Update` closure. Do NOT remove the `userMutex` call yet (kept for one more commit so the mutex deletion is a focused diff).

- [ ] **Step 1: Rewrite the method**

```go
// RegenerateWeeklyPlanIfUnstarted replaces the current week's plan when no
// session has been started yet. Atomic via WeekPlanRepository.Update — the
// AnyStarted check and the replacement happen in one transaction, closing the
// race window the old userMutex existed to mitigate.
func (s *Service) RegenerateWeeklyPlanIfUnstarted(ctx context.Context) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	mu := s.userMutex(userID) // TEMP: removed in next task
	mu.Lock()
	defer mu.Unlock()

	monday := domain.MondayOf(time.Now())
	newPlan, err := s.planWeek(ctx, monday)
	if err != nil {
		return err
	}
	return s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		if wp.AnyStarted() {
			return nil
		}
		wp.Replace(newPlan)
		return nil
	})
}
```

Note: `planWeek` (added in Task 7) is the planner call. It runs outside the tx — same data freshness as today.

The `WeekPlans.Update` will fail with `ErrNotFound` if no session rows exist for the week yet. Handle that case: if `ErrNotFound`, this is a no-op (nothing to regenerate, nothing started by definition):

```go
err = s.repos.WeekPlans.Update(...)
if errors.Is(err, domain.ErrNotFound) {
    return nil
}
return err
```

- [ ] **Step 2: Build, run tests**

Run: `make test`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/service/sessions.go
git commit -m "service: RegenerateWeeklyPlanIfUnstarted uses WeekPlans.Update (mutex still in place)"
```

---

## Task 11: Migrate `StartDeloadNow` and `RestartMesocycleAnchor`; delete the mutex

**Files:**
- Modify: `internal/service/sessions.go` — `StartDeloadNow`.
- Modify: `internal/service/service.go` — `RestartMesocycleAnchor`; delete `userLocks`, `userMutex`, `sync.Map` field, `sync` import; remove the `mu.Lock/Unlock` lines from `RegenerateWeeklyPlanIfUnstarted`.

- [ ] **Step 1: Rewrite `StartDeloadNow`**

```go
func (s *Service) StartDeloadNow(ctx context.Context) error {
	monday := domain.MondayOf(time.Now())
	today := domain.StartOfDay(time.Now())

	err := s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		return wp.FlipDeloadFromToday(today)
	})
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("flip deload for week %s: %w", monday.Format(time.DateOnly), err)
	}

	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
	if err = s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Rewrite `RestartMesocycleAnchor`**

```go
func (s *Service) RestartMesocycleAnchor(ctx context.Context) error {
	monday := domain.MondayOf(time.Now())
	today := domain.StartOfDay(time.Now())

	err := s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		return wp.ClearDeloadFromToday(today)
	})
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("clear deload for week %s: %w", monday.Format(time.DateOnly), err)
	}

	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
	if err = s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Delete the mutex**

In `internal/service/service.go`:

- Delete the `"sync"` import.
- Delete the `userLocks *sync.Map // map[int]*sync.Mutex` field.
- Delete the `userLocks: &sync.Map{},` initialiser in `NewService`.
- Delete the `userMutex(userID int) *sync.Mutex` method.
- Update the `Service` struct doc-comment to remove the mutex paragraph.

In `internal/service/sessions.go`:

- Delete the `mu := s.userMutex(userID); mu.Lock(); defer mu.Unlock()` lines from `RegenerateWeeklyPlanIfUnstarted`.
- Delete the `contexthelpers.AuthenticatedUserID(ctx)` line if it became dead.
- Update the method doc-comment to describe the new atomicity story (no more multi-process disclaimer needed).

- [ ] **Step 4: Build, run tests**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Confirm no `sync.Mutex` or `sync.Map` references remain in service package**

Run: `grep -n "sync\." internal/service/*.go | grep -v _test.go`
Expected: no matches (or only `sync/atomic` from `feature_flags.go`, which is unrelated).

- [ ] **Step 6: Commit**

```bash
git add internal/service/sessions.go internal/service/service.go
git commit -m "service: migrate deload methods to WeekPlans.Update; delete userLocks mutex"
```

---

## Task 12: Add race test proving the closed race window

**Files:**
- Modify: `internal/service/sessions_test.go` — add a goroutine race covering `RegenerateWeeklyPlanIfUnstarted` + `StartSession`.

The spec's main correctness claim is that this race is no longer possible. This test pins that claim.

- [ ] **Step 1: Add the test**

```go
func Test_RegenerateAndStart_AreSerializedByTheDatabase(t *testing.T) {
	t.Parallel()
	ctx, svc := setupTestService(t)

	monday := domain.MondayOf(time.Now())
	// Ensure today is a scheduled workout day. setupTestService configures
	// Mon/Wed/Fri at 60 min — use a Monday in the current ISO week.
	today := monday

	// Pre-generate the week so Regenerate and Start operate on existing rows.
	if _, err := svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(2)

	errCh := make(chan error, 2*iterations)

	go func() {
		defer wg.Done()
		for range iterations {
			if err := svc.RegenerateWeeklyPlanIfUnstarted(ctx); err != nil {
				errCh <- fmt.Errorf("regenerate: %w", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range iterations {
			err := svc.StartSession(ctx, today)
			if err != nil && !errors.Is(err, domain.ErrAlreadyStarted) {
				errCh <- fmt.Errorf("start: %w", err)
			}
		}
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("unexpected concurrent error: %v", err)
	}

	// Final invariant: today's session is started AND has slots.
	plan, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after: %v", err)
	}
	sess := plan.SessionOn(today)
	if sess == nil {
		t.Fatalf("today's session missing")
	}
	if sess.StartedAt.IsZero() {
		t.Error("today's session should be started after the race")
	}
	if len(sess.ExerciseSets) == 0 {
		t.Error("today's session should have slots after the race")
	}
}
```

Add the imports `"sync"`, `"fmt"`, `"errors"` if not already present.

- [ ] **Step 2: Run with the race detector**

Run: `go test -race ./internal/service -run Test_RegenerateAndStart_AreSerializedByTheDatabase -v`
Expected: PASS, no races.

- [ ] **Step 3: Run full service suite under race**

Run: `go test -race ./internal/service/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/service/sessions_test.go
git commit -m "test: cover Regenerate + StartSession race; proves WeekPlan atomicity"
```

---

## Task 13: Delete `SessionRepository` write methods

**Files:**
- Modify: `internal/repository/repository.go` — remove `Update`, `Create`, `CreateBatch`, `DeleteWeek` from the `SessionRepository` interface.
- Modify: `internal/repository/sessions.go` — remove the four method implementations and any private helpers they exclusively used (e.g. `deleteSession` if only `Update` called it; `insertSession` is now in shared scope from Task 5).
- Modify: `internal/repository/sessions_test.go` — delete tests covering the deleted methods.

- [ ] **Step 1: Verify no service code still calls the deleted methods**

Run: `grep -n "Sessions\.\(Update\|Create\|CreateBatch\|DeleteWeek\)" internal/service/ cmd/web/`
Expected: no matches outside tests.

If matches exist, finish migrating them before this task.

- [ ] **Step 2: Remove from interface**

In `internal/repository/repository.go`, `SessionRepository` becomes:

```go
type SessionRepository interface {
	Get(ctx context.Context, date time.Time) (domain.Session, error)
	List(ctx context.Context, sinceDate time.Time) ([]domain.Session, error)

	// Read-only specialised queries used by reporting.
	ListSetsForExerciseSince(
		ctx context.Context, exerciseID int, sinceDate time.Time,
	) ([]domain.ExerciseSetHistory, error)
	GetLatestStartingWeightBefore(
		ctx context.Context, exerciseID int, beforeDate time.Time,
	) (domain.LatestStartingSet, error)
	GetLatestSuccessfulSecondsBefore(
		ctx context.Context, exerciseID int, beforeDate time.Time,
	) (int, error)
	CountCompleted(ctx context.Context) (int, error)
}
```

Update the doc-comment to call out the read-only role.

- [ ] **Step 3: Remove implementations**

Delete the bodies of `Update`, `Create`, `CreateBatch`, `DeleteWeek` from `internal/repository/sessions.go`. Leave shared helpers (`insertSession`, `deleteWeek` if extracted to shared scope) in their new shared home.

- [ ] **Step 4: Remove obsolete tests**

In `internal/repository/sessions_test.go`, delete any tests that exclusively exercised the removed methods (e.g. `Test_SessionRepository_Update*`, `Test_SessionRepository_CreateBatch*`, etc.). Keep tests for `Get`, `List`, and the specialised reads.

- [ ] **Step 5: Build, run full CI**

Run: `make ci`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/repository/
git commit -m "repository: remove SessionRepository write surface (subsumed by WeekPlanRepository)"
```

---

## Final verification

- [ ] Run the full CI suite:

```bash
make ci
```

- [ ] Confirm the spec's "Goal" claims hold:
  - `grep -n "userLocks\|userMutex" internal/service/*.go` → no matches.
  - `grep -n "Sessions\.\(Update\|Create\|CreateBatch\|DeleteWeek\)" internal/service/ cmd/web/` → no matches.
  - `make test` passes with `-race`.

- [ ] Inspect a smoke flow manually if possible: start the dev server, run through a workout end-to-end (start session, record sets, complete, view schedule, regenerate next week).
