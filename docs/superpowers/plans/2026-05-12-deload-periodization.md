# Deload Periodization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a fixed-cadence deload week (default 4:1) that forces deload weeks to a lighter hypertrophy prescription — volume halved, starting weight at 90% of the prior hypertrophy working weight, signals hidden, and deload sessions excluded from every future progression lookup.

**Architecture:** Stateless mesocycle position derivation from a stored Monday anchor + cadence preference, mirroring the existing strength/hypertrophy alternation pattern. `Session.IsDeload` propagates through the planner, the per-set Progression, and the UI; `workout_sessions.is_deload = 0` is the unconditional filter on the starting-weight history query.

**Tech Stack:** Go 1.22+, SQLite (declarative migrator), server-rendered Go templates, `internal/domain` (pure rules), `internal/repository` (persistence), `internal/service` (orchestration), `cmd/web` (handlers).

**Reference design:** `docs/superpowers/specs/2026-05-12-deload-periodization-design.md`.

---

## Conventions

- All commands run from the repo root.
- Run `make lint-fix` after material code changes; the plan calls it out at phase boundaries.
- Commit per task. Each commit message uses the conventional prefix used in the repo: bare imperative subject lines, optional body.

---

## Phase 1: Pure domain primitives

### Task 1: `IsDeloadWeek` and `WeekInBlock` (mesocycle math)

**Files:**
- Create: `internal/domain/mesocycle.go`
- Create: `internal/domain/mesocycle_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/domain/mesocycle_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestWeekInBlock(t *testing.T) {
	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // Monday

	tests := []struct {
		name   string
		date   time.Time
		length int
		want   int
	}{
		{"anchor itself", anchor, 5, 0},
		{"one week after", anchor.AddDate(0, 0, 7), 5, 1},
		{"deload week (length-1)", anchor.AddDate(0, 0, 28), 5, 4},
		{"wraps to next block", anchor.AddDate(0, 0, 35), 5, 0},
		{"mid-block, 4-week cadence", anchor.AddDate(0, 0, 14), 4, 2},
		{"date before anchor returns 0", anchor.AddDate(0, 0, -7), 5, 0},
		{"non-monday date snaps to its week", anchor.AddDate(0, 0, 10), 5, 1}, // Thu of week 1
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.WeekInBlock(tt.date, anchor, tt.length); got != tt.want {
				t.Errorf("WeekInBlock(%s, %s, %d) = %d, want %d",
					tt.date.Format("2006-01-02"), anchor.Format("2006-01-02"), tt.length, got, tt.want)
			}
		})
	}
}

func TestIsDeloadWeek(t *testing.T) {
	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC) // Monday

	tests := []struct {
		name    string
		enabled bool
		date    time.Time
		length  int
		want    bool
	}{
		{"week 0 of 5 — not deload", true, anchor, 5, false},
		{"week 4 of 5 — is deload", true, anchor.AddDate(0, 0, 28), 5, true},
		{"week 5 (wraps to 0) — not deload", true, anchor.AddDate(0, 0, 35), 5, false},
		{"week 3 of 4 — is deload", true, anchor.AddDate(0, 0, 21), 4, true},
		{"feature disabled", false, anchor.AddDate(0, 0, 28), 5, false},
		{"zero anchor returns false", true, anchor.AddDate(0, 0, 28), 5, false}, // overridden below
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := anchor
			if tt.name == "zero anchor returns false" {
				a = time.Time{}
			}
			if got := domain.IsDeloadWeek(tt.date, a, tt.length, tt.enabled); got != tt.want {
				t.Errorf("IsDeloadWeek = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDeloadWeek_LengthBounds(t *testing.T) {
	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	for _, length := range []int{0, 1, -1} {
		if got := domain.IsDeloadWeek(anchor, anchor, length, true); got {
			t.Errorf("IsDeloadWeek(length=%d) = true, want false (defensive)", length)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/ -run 'TestWeekInBlock|TestIsDeloadWeek' -v`
Expected: FAIL with `undefined: domain.WeekInBlock`.

- [ ] **Step 3: Implement**

`internal/domain/mesocycle.go`:

```go
package domain

import "time"

// minDeloadCadence is the smallest sensible block length. A length of 1 would
// make every week a deload, and a length of 0 disables the calculation;
// callers should treat both as "feature off".
const minDeloadCadence = 2

// WeekInBlock returns the 0-based week index within the mesocycle for date,
// given a Monday anchor and a block length. Dates strictly before the anchor
// return 0 (treated as "the anchor week starts in the future, count it as
// week 0 for now"). The calculation truncates to whole weeks; intra-week
// dates resolve to the same week as their Monday.
func WeekInBlock(date, anchor time.Time, length int) int {
	if length < minDeloadCadence || anchor.IsZero() {
		return 0
	}
	dayDiff := int(date.Sub(anchor).Hours() / 24)
	if dayDiff < 0 {
		return 0
	}
	weeks := dayDiff / 7
	return weeks % length
}

// IsDeloadWeek reports whether the date falls on the last (deload) week of
// its mesocycle. Returns false when the feature is disabled, when length is
// below minDeloadCadence, or when the anchor is the zero time.
func IsDeloadWeek(date, anchor time.Time, length int, enabled bool) bool {
	if !enabled || length < minDeloadCadence || anchor.IsZero() {
		return false
	}
	return WeekInBlock(date, anchor, length) == length-1
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/ -run 'TestWeekInBlock|TestIsDeloadWeek' -v`
Expected: PASS for all subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/mesocycle.go internal/domain/mesocycle_test.go
git commit -m "Add WeekInBlock and IsDeloadWeek mesocycle helpers"
```

---

### Task 2: Extend `Preferences` with deload fields

**Files:**
- Modify: `internal/domain/preferences.go`
- Modify: `internal/domain/planner_internal_test.go` (existing test constructors)

- [ ] **Step 1: Read the file**

Open `internal/domain/preferences.go`. Note the struct ends with `RestNotificationsEnabled bool`.

- [ ] **Step 2: Add fields to Preferences**

Append to the `Preferences` struct (after `RestNotificationsEnabled`):

```go
	DeloadEnabled    bool
	MesocycleLength  int
	MesocycleAnchor  time.Time
```

- [ ] **Step 3: Run the build to surface any compile errors**

Run: `go build ./...`
Expected: PASS (additive field changes are backward-compatible).

- [ ] **Step 4: Update `prefs` test-helper docstring**

In `internal/domain/planner_internal_test.go` line 19-26, look at the existing `prefs(days ...time.Weekday) Preferences` helper. Confirm it uses a struct literal with `//nolint:exhaustruct`. No changes needed — the lint directive already covers the new fields.

Run: `go test ./internal/domain/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/preferences.go
git commit -m "Add DeloadEnabled, MesocycleLength, MesocycleAnchor to Preferences"
```

---

### Task 3: Add `IsDeload` to `Session`

**Files:**
- Modify: `internal/domain/session.go`

- [ ] **Step 1: Read the file**

Open `internal/domain/session.go`. Locate the `Session` struct (around line 38).

- [ ] **Step 2: Add the field**

Append to the `Session` struct (after `PeriodizationType`):

```go
	IsDeload          bool
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 4: Run the domain tests**

Run: `go test ./internal/domain/ -count=1`
Expected: PASS — the new field defaults to `false`, no existing tests should care.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/session.go
git commit -m "Add IsDeload field to Session"
```

---

### Task 4: `DeriveScheme` and `RestSecondsFor` accept `isDeload`

**Files:**
- Modify: `internal/domain/progression_scheme.go`
- Modify: `internal/domain/progression_scheme_test.go`
- Modify: callers (mechanical — see Step 4)

- [ ] **Step 1: Write the failing tests**

In `internal/domain/progression_scheme_test.go`, replace the existing `TestDeriveScheme` body and append a deload-specific test. First, locate the existing `tests := []struct { ... }` block in `TestDeriveScheme` (around line 9–60) and update every call site to include a `false` deload argument. Then append:

```go
func TestDeriveScheme_Deload(t *testing.T) {
	tests := []struct {
		name             string
		repMin, repMax   int
		periodization   domain.PeriodizationType
		wantTargetReps  int
		wantTargetSets  int
		wantRestSeconds int
	}{
		{"low rep window, deload still hypertrophy", 3, 5, domain.PeriodizationStrength, 5, 2, 180},
		{"mid rep window, deload halves 3 sets to 2", 6, 10, domain.PeriodizationStrength, 10, 2, 90},
		{"high rep window, deload halves 3 sets to 2", 12, 15, domain.PeriodizationHypertrophy, 15, 2, 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.DeriveScheme(tt.repMin, tt.repMax, tt.periodization, true)
			if got.TargetReps != tt.wantTargetReps {
				t.Errorf("TargetReps = %d, want %d (deload always uses repMax)", got.TargetReps, tt.wantTargetReps)
			}
			if got.TargetSets != tt.wantTargetSets {
				t.Errorf("TargetSets = %d, want %d (halved, min 1)", got.TargetSets, tt.wantTargetSets)
			}
			if got.RestSeconds != tt.wantRestSeconds {
				t.Errorf("RestSeconds = %d, want %d (unchanged from hypertrophy mapping)", got.RestSeconds, tt.wantRestSeconds)
			}
		})
	}
}
```

Also update `TestRestSecondsFor` (around line 75) call sites to pass `false`, and append:

```go
func TestRestSecondsFor_Deload(t *testing.T) {
	ex := domain.Exercise{ //nolint:exhaustruct // Only fields read by RestSecondsFor are set.
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       intPtr(8),
		RepMax:       intPtr(12),
	}
	if got := domain.RestSecondsFor(ex, domain.PeriodizationStrength, true); got != 90 {
		t.Errorf("RestSecondsFor deload = %d, want 90 (hypertrophy mapping)", got)
	}
}

func intPtr(i int) *int { return &i }
```

If `intPtr` already exists in the file, omit the duplicate definition.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/ -run 'TestDeriveScheme|TestRestSecondsFor' -v`
Expected: FAIL — signature mismatch (`DeriveScheme` takes 3 args, called with 4).

- [ ] **Step 3: Implement the new signatures**

In `internal/domain/progression_scheme.go`, replace the existing `DeriveScheme` with:

```go
func DeriveScheme(repMin, repMax int, p PeriodizationType, isDeload bool) Scheme {
	if isDeload {
		// Deload forces hypertrophy targets regardless of incoming p, then
		// halves the set count (min 1). Rest stays at the hypertrophy
		// mapping for the resulting rep band.
		p = PeriodizationHypertrophy
	}

	var reps int
	switch p {
	case PeriodizationStrength:
		reps = repMin
	case PeriodizationHypertrophy:
		reps = repMax
	default:
		panic(fmt.Sprintf("domain: unknown PeriodizationType %q", p))
	}

	var sets, rest int
	switch {
	case reps <= repBoundaryLowToMid:
		sets, rest = setsLow, restLow
	case reps <= repBoundaryMidToHigh:
		sets, rest = setsMid, restMid
	default:
		sets, rest = setsHigh, restHigh
	}

	if isDeload {
		sets = deloadSets(sets)
	}

	return Scheme{TargetReps: reps, TargetSets: sets, RestSeconds: rest}
}

// deloadSets halves the set count for deload weeks, with a floor of 1.
func deloadSets(normalSets int) int {
	half := (normalSets + 1) / 2 // ceil division
	if half < 1 {
		return 1
	}
	return half
}
```

Replace the existing `RestSecondsFor` with:

```go
func RestSecondsFor(ex Exercise, pt PeriodizationType, isDeload bool) int {
	if ex.IsTimed() {
		return 0
	}
	if ex.RepMin == nil || ex.RepMax == nil {
		return 0
	}
	return DeriveScheme(*ex.RepMin, *ex.RepMax, pt, isDeload).RestSeconds
}
```

- [ ] **Step 4: Update all callers (mechanical)**

Run: `grep -rn "DeriveScheme(" --include="*.go" .`

Update each non-test call site to add a fourth argument. Specifically:

- `internal/domain/progression.go:69` — `DeriveScheme(p.config.RepMin, p.config.RepMax, p.config.Type, p.config.IsDeload).TargetReps` (the `Config.IsDeload` field is added in Task 5; for now, write the call as `..., false)` and fix in Task 5).
- `internal/domain/planning_sets.go:29` — `DeriveScheme(*ex.RepMin, *ex.RepMax, pt, isDeload)` (the new `isDeload` parameter is added in Task 6; for now write `..., false)`).

For `RestSecondsFor`:

- `internal/service/sets.go:116` — `restSeconds := domain.RestSecondsFor(exercise, periodization, false)` — placeholder, fixed in Task 14.

Also update test-internal usages:

- `internal/domain/progression_test.go:381` — `domain.DeriveScheme(repMin, repMax, p, false)`.
- `internal/domain/planner_internal_test.go:304,312` — add `, false`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -count=1`
Expected: PASS for all packages.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/progression_scheme.go internal/domain/progression_scheme_test.go \
        internal/domain/progression.go internal/domain/progression_test.go \
        internal/domain/planning_sets.go internal/domain/planner_internal_test.go \
        internal/service/sets.go
git commit -m "Thread isDeload through DeriveScheme and RestSecondsFor"
```

---

### Task 5: `Progression.Config.IsDeload` and constant-weight `CurrentSet`

**Files:**
- Modify: `internal/domain/progression.go`
- Modify: `internal/domain/progression_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/domain/progression_test.go`:

```go
func TestProgression_DeloadHoldsStartingWeight(t *testing.T) {
	cfg := domain.Config{
		Type:           domain.PeriodizationHypertrophy,
		RepMin:         8,
		RepMax:         12,
		StartingWeight: 67.5,
		IsDeload:       true,
	}
	p := domain.New(cfg)

	target := p.CurrentSet()
	if target.WeightKg != 67.5 {
		t.Errorf("initial CurrentSet WeightKg = %v, want 67.5", target.WeightKg)
	}
	if target.TargetReps != 12 {
		t.Errorf("initial CurrentSet TargetReps = %d, want 12 (hypertrophy → repMax)", target.TargetReps)
	}

	// Even with completed sets recorded (signals present), deload returns
	// the starting weight every time — no autoregulation.
	p.RecordCompletion(domain.SetResult{
		ActualReps: 12,
		Signal:     domain.SignalTooLight,
		WeightKg:   67.5,
	})
	if got := p.CurrentSet().WeightKg; got != 67.5 {
		t.Errorf("after SignalTooLight, deload CurrentSet WeightKg = %v, want 67.5 (no progression)", got)
	}

	p.RecordCompletion(domain.SetResult{
		ActualReps: 10,
		Signal:     domain.SignalTooHeavy,
		WeightKg:   67.5,
	})
	if got := p.CurrentSet().WeightKg; got != 67.5 {
		t.Errorf("after SignalTooHeavy, deload CurrentSet WeightKg = %v, want 67.5 (no autoreg)", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run TestProgression_DeloadHoldsStartingWeight -v`
Expected: FAIL — `unknown field IsDeload in struct literal of type domain.Config`.

- [ ] **Step 3: Implement**

In `internal/domain/progression.go`, modify `Config`:

```go
type Config struct {
	Type           PeriodizationType
	RepMin         int
	RepMax         int
	StartingWeight float64
	IsDeload       bool
}
```

Modify `CurrentSet`:

```go
func (p *Progression) CurrentSet() SetTarget {
	reps := DeriveScheme(p.config.RepMin, p.config.RepMax, p.config.Type, p.config.IsDeload).TargetReps
	if p.config.IsDeload || len(p.completed) == 0 {
		return SetTarget{WeightKg: p.config.StartingWeight, TargetReps: reps}
	}
	last := p.completed[len(p.completed)-1]
	weight := adjustedWeight(last)
	return SetTarget{WeightKg: weight, TargetReps: reps}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/domain/ -run TestProgression -v`
Expected: PASS, including the new deload test and all existing tests.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/progression.go internal/domain/progression_test.go
git commit -m "Hold starting weight across deload sets in Progression.CurrentSet"
```

---

### Task 6: `BuildPlannedSets` and `BuildSetsForAdd` accept `isDeload`

**Files:**
- Modify: `internal/domain/planning_sets.go`
- Modify: `internal/domain/planning_sets_test.go`
- Modify: `internal/domain/planner.go`
- Modify: `internal/service/exercises.go`

- [ ] **Step 1: Inspect existing test**

Run: `cat internal/domain/planning_sets_test.go | head -40`

You'll be appending a new test that asserts the deload prescription. The existing tests must keep passing with a `false` argument added to current calls.

- [ ] **Step 2: Update existing call sites to pass `false`**

In `internal/domain/planning_sets_test.go`, find every `BuildPlannedSets(` and `BuildSetsForAdd(` call and add a `, false` argument. (Use `grep -n "BuildPlannedSets\|BuildSetsForAdd" internal/domain/planning_sets_test.go` to locate them.)

Append the deload-specific test:

```go
func TestBuildPlannedSets_Deload(t *testing.T) {
	ex := domain.Exercise{ //nolint:exhaustruct // Only the planning fields are read.
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       intPtr(8),
		RepMax:       intPtr(12),
	}
	got := domain.BuildPlannedSets(ex, domain.PeriodizationStrength, true)
	// Normal mid-rep band: 3 sets. Deload: ceil(3/2) = 2.
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (deload halves sets)", len(got))
	}
	for i, s := range got {
		if s.TargetValue != 12 {
			t.Errorf("set %d TargetValue = %d, want 12 (deload forces repMax)", i, s.TargetValue)
		}
	}
}

func intPtr(i int) *int { return &i } // If already defined in this file, drop the duplicate.
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/domain/ -run 'TestBuildPlannedSets|TestBuildSetsForAdd' -v`
Expected: FAIL — signature mismatch.

- [ ] **Step 4: Implement**

In `internal/domain/planning_sets.go`, modify `deriveSchemeForExercise`, `BuildPlannedSets`, and `BuildSetsForAdd`:

```go
func deriveSchemeForExercise(ex Exercise, pt PeriodizationType, isDeload bool) (int, int) {
	if ex.IsTimed() {
		if ex.DefaultStartingSeconds != nil {
			return *ex.DefaultStartingSeconds, defaultTimedSets
		}
		return defaultTargetValue, defaultTimedSets
	}
	if ex.RepMin == nil || ex.RepMax == nil {
		return defaultTargetValue, defaultTimedSets
	}
	scheme := DeriveScheme(*ex.RepMin, *ex.RepMax, pt, isDeload)
	return scheme.TargetReps, scheme.TargetSets
}

func BuildPlannedSets(exercise Exercise, periodization PeriodizationType, isDeload bool) []Set {
	targetValue, n := deriveSchemeForExercise(exercise, periodization, isDeload)
	sets := make([]Set, n)
	for i := range sets {
		sets[i] = Set{ //nolint:exhaustruct // WeightKg, CompletedValue, CompletedAt, Signal start nil.
			TargetValue: targetValue,
		}
	}
	return sets
}

func BuildSetsForAdd(exercise Exercise, periodization PeriodizationType, isDeload bool, historicalSets []Set) []Set {
	sets := BuildPlannedSets(exercise, periodization, isDeload)
	if !exercise.HasWeight() {
		return sets
	}
	var seedWeight float64
	for i := len(historicalSets) - 1; i >= 0; i-- {
		if historicalSets[i].WeightKg != nil {
			seedWeight = *historicalSets[i].WeightKg
			break
		}
	}
	for i := range sets {
		w := seedWeight
		sets[i].WeightKg = &w
	}
	return sets
}
```

In `internal/domain/planner.go` line 319, change to:

```go
		Sets:     BuildPlannedSets(ex, pt, false), // false: planner deload override applied in Task 11.
```

In `internal/service/exercises.go` lines 75 and 159, change `BuildSetsForAdd(newExercise, sess.PeriodizationType, historicalSets)` (and the other occurrence) to:

```go
		newSets := domain.BuildSetsForAdd(newExercise, sess.PeriodizationType, sess.IsDeload, historicalSets)
```

and respectively:

```go
		newSets := domain.BuildSetsForAdd(exercise, sess.PeriodizationType, sess.IsDeload, historicalSets)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/planning_sets.go internal/domain/planning_sets_test.go \
        internal/domain/planner.go internal/service/exercises.go
git commit -m "Thread isDeload through BuildPlannedSets and BuildSetsForAdd"
```

---

### Task 7: `Session.RecordSet` accepts `*Signal` (nullable)

**Files:**
- Modify: `internal/domain/session.go`
- Modify: `internal/domain/session_test.go` (if exists) or add coverage in this task
- Modify: `internal/service/sets.go`
- Modify: `cmd/web/handler-exerciseset.go`

- [ ] **Step 1: Write the failing test**

Append to (or create) `internal/domain/session_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestSession_RecordSet_NilSignalIsAllowed(t *testing.T) {
	now := time.Date(2026, time.May, 4, 10, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // only fields used by RecordSet
		ExerciseSets: []domain.ExerciseSet{ //nolint:exhaustruct
			{
				ID:       1,
				Exercise: domain.Exercise{ID: 1, ExerciseType: domain.ExerciseTypeBodyweight}, //nolint:exhaustruct
				Sets:     []domain.Set{ {TargetValue: 12} },
			},
		},
	}
	if err := sess.RecordSet(1, 0, nil, nil, 11, now); err != nil {
		t.Fatalf("RecordSet with nil signal: %v", err)
	}
	got := sess.ExerciseSets[0].Sets[0]
	if got.Signal != nil {
		t.Errorf("Signal = %v, want nil", got.Signal)
	}
	if got.CompletedValue == nil || *got.CompletedValue != 11 {
		t.Errorf("CompletedValue = %v, want 11", got.CompletedValue)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/ -run TestSession_RecordSet_NilSignalIsAllowed -v`
Expected: FAIL — `cannot use nil as domain.Signal value in argument`.

- [ ] **Step 3: Update the domain method signature**

In `internal/domain/session.go`, change `RecordSet`:

```go
func (s *Session) RecordSet(
	slotID, setIndex int,
	signal *Signal,
	weightKg *float64,
	completedValue int,
	now time.Time,
) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID != slotID {
			continue
		}
		if setIndex < 0 || setIndex >= len(s.ExerciseSets[i].Sets) {
			return ErrSetIndexOutOfBounds
		}
		set := &s.ExerciseSets[i].Sets[setIndex]
		if signal != nil {
			sigCopy := *signal
			set.Signal = &sigCopy
		} else {
			set.Signal = nil
		}
		if weightKg != nil {
			w := *weightKg
			set.WeightKg = &w
		}
		v := completedValue
		set.CompletedValue = &v
		t := now
		set.CompletedAt = &t
		return nil
	}
	return ErrSlotNotFound
}
```

- [ ] **Step 4: Update service caller**

In `internal/service/sets.go`, change `RecordSet`:

```go
func (s *Service) RecordSet(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	signal *domain.Signal,
	weightKg *float64,
	completedValue int,
) error {
```

And the body:

```go
		if recErr := sess.RecordSet(workoutExerciseID, setIndex, signal, weightKg, completedValue, now); recErr != nil {
```

Also update `maybeSchedulePush`'s callers if they read `periodization` (no change needed here; it was a local variable).

- [ ] **Step 5: Update handler callers**

In `cmd/web/handler-exerciseset.go` line 225, change:

```go
	signal := domain.Signal(r.PostForm.Get("signal"))
```

to:

```go
	var signal *domain.Signal
	if raw := r.PostForm.Get("signal"); raw != "" {
		s := domain.Signal(raw)
		signal = &s
	}
```

And line 233 — pass `signal` (now `*domain.Signal`):

```go
	err = app.service.RecordSet(
		r.Context(), params.Date, params.WorkoutExerciseID, params.SetIndex, signal, &weight, reps)
```

Repeat the same translation for the timed-set branch at line 291–293:

```go
	var signal *domain.Signal
	if raw := r.PostForm.Get("signal"); raw != "" {
		s := domain.Signal(raw)
		signal = &s
	}

	if err = app.service.RecordSet(
		r.Context(), params.Date, params.WorkoutExerciseID, params.SetIndex, signal, nil, completedSeconds); err != nil {
```

Adjust the `slog.LogAttrs` calls below to handle nil signal — render an empty string when nil:

```go
	signalStr := ""
	if signal != nil {
		signalStr = string(*signal)
	}
	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "recorded set completion",
		slog.String("date", params.Date.Format("2006-01-02")),
		slog.Int("workout_exercise_id", params.WorkoutExerciseID),
		slog.Int("set_index", params.SetIndex),
		slog.String("signal", signalStr),
		slog.Float64("weight", weight),
		slog.Int("reps", reps))
```

(Apply the same `signalStr` derivation in the timed-set log block at line 299–304.)

- [ ] **Step 6: Run tests to verify everything still passes**

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/session.go internal/domain/session_test.go \
        internal/service/sets.go cmd/web/handler-exerciseset.go
git commit -m "Accept nil signal in Session.RecordSet for deload sets"
```

---

## Phase 2: Schema and Repository

### Task 8: Schema additions

**Files:**
- Modify: `internal/sqlite/schema.sql`

- [ ] **Step 1: Add columns**

In `internal/sqlite/schema.sql`, find `CREATE TABLE workout_preferences` (line 69) and replace it with:

```sql
CREATE TABLE workout_preferences
(
    user_id                    INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    monday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (monday_minutes IN (0, 45, 60, 90)),
    tuesday_minutes            INTEGER NOT NULL DEFAULT 0 CHECK (tuesday_minutes IN (0, 45, 60, 90)),
    wednesday_minutes          INTEGER NOT NULL DEFAULT 0 CHECK (wednesday_minutes IN (0, 45, 60, 90)),
    thursday_minutes           INTEGER NOT NULL DEFAULT 0 CHECK (thursday_minutes IN (0, 45, 60, 90)),
    friday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (friday_minutes IN (0, 45, 60, 90)),
    saturday_minutes           INTEGER NOT NULL DEFAULT 0 CHECK (saturday_minutes IN (0, 45, 60, 90)),
    sunday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (sunday_minutes IN (0, 45, 60, 90)),
    rest_notifications_enabled INTEGER NOT NULL DEFAULT 1 CHECK (rest_notifications_enabled IN (0, 1)),
    deload_enabled             INTEGER NOT NULL DEFAULT 0 CHECK (deload_enabled IN (0, 1)),
    mesocycle_length           INTEGER NOT NULL DEFAULT 5 CHECK (mesocycle_length BETWEEN 4 AND 7),
    mesocycle_anchor           TEXT CHECK (mesocycle_anchor IS NULL
                                           OR STRFTIME('%Y-%m-%d', mesocycle_anchor) = mesocycle_anchor)
) WITHOUT ROWID, STRICT;
```

Find `CREATE TABLE workout_sessions` (line 98) and add the `is_deload` column before the `PRIMARY KEY` line:

```sql
CREATE TABLE workout_sessions
(
    user_id            INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workout_date       TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    difficulty_rating  INTEGER CHECK (difficulty_rating BETWEEN 1 AND 5),
    started_at         TEXT CHECK (started_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', started_at) = started_at),
    completed_at       TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
    periodization_type TEXT    NOT NULL DEFAULT 'strength'
        CHECK (periodization_type IN ('strength', 'hypertrophy')),
    is_deload          INTEGER NOT NULL DEFAULT 0 CHECK (is_deload IN (0, 1)),

    PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;
```

- [ ] **Step 2: Run the migrator tests**

Run: `go test ./internal/sqlite/ -count=1`
Expected: PASS — the declarative migrator handles additive columns with defaults.

- [ ] **Step 3: Commit**

```bash
git add internal/sqlite/schema.sql
git commit -m "Schema: add deload_enabled, mesocycle_length, mesocycle_anchor, is_deload"
```

---

### Task 9: `PreferencesRepository` reads/writes new fields

**Files:**
- Modify: `internal/repository/preferences.go`
- Modify: `internal/repository/preferences_test.go` (or `repository_test.go` if that's where prefs tests live)

- [ ] **Step 1: Locate the existing tests**

Run: `grep -rn "PreferencesRepository\|prefs.RestNotificationsEnabled" internal/repository/ --include="*.go"`

Identify the round-trip test file for preferences. Append the failing test there:

```go
func TestPreferencesRepository_DeloadFields(t *testing.T) {
	ctx, repos, cleanup := setupTestRepos(t)
	defer cleanup()

	anchor := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	prefs := domain.Preferences{ //nolint:exhaustruct
		DeloadEnabled:   true,
		MesocycleLength: 4,
		MesocycleAnchor: anchor,
	}
	if err := repos.Preferences.Set(ctx, prefs); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.DeloadEnabled {
		t.Error("DeloadEnabled = false, want true")
	}
	if got.MesocycleLength != 4 {
		t.Errorf("MesocycleLength = %d, want 4", got.MesocycleLength)
	}
	if !got.MesocycleAnchor.Equal(anchor) {
		t.Errorf("MesocycleAnchor = %s, want %s", got.MesocycleAnchor, anchor)
	}
}
```

If `setupTestRepos` differs in name (e.g. `setupRepositories`), match the existing pattern by reading neighbouring tests in the same file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/repository/ -run TestPreferencesRepository_DeloadFields -v`
Expected: FAIL — columns not yet read/written by the repo.

- [ ] **Step 3: Update the SQL**

In `internal/repository/preferences.go`, update `Get`'s SELECT and Scan to include the new columns:

```go
func (r *sqlitePreferencesRepository) Get(ctx context.Context) (domain.Preferences, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var (
		prefs        domain.Preferences
		anchorStr    sql.NullString
	)
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
		       friday_minutes, saturday_minutes, sunday_minutes,
		       rest_notifications_enabled,
		       deload_enabled, mesocycle_length, mesocycle_anchor
		FROM workout_preferences
		WHERE user_id = ?`, userID).Scan(
		&prefs.MondayMinutes, &prefs.TuesdayMinutes, &prefs.WednesdayMinutes, &prefs.ThursdayMinutes,
		&prefs.FridayMinutes, &prefs.SaturdayMinutes, &prefs.SundayMinutes,
		&prefs.RestNotificationsEnabled,
		&prefs.DeloadEnabled, &prefs.MesocycleLength, &anchorStr,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.Preferences{ //nolint:exhaustruct // Weekday minutes zero by design.
			RestNotificationsEnabled: true,
			MesocycleLength:          5,
		}, nil
	}
	if err != nil {
		return domain.Preferences{}, fmt.Errorf("query workout preferences: %w", err)
	}
	if anchorStr.Valid {
		anchor, parseErr := time.Parse(dateFormat, anchorStr.String)
		if parseErr != nil {
			return domain.Preferences{}, fmt.Errorf("parse mesocycle_anchor: %w", parseErr)
		}
		prefs.MesocycleAnchor = anchor
	}
	return prefs, nil
}
```

Update `Set`'s INSERT to include the new columns and the ON CONFLICT branch:

```go
func (r *sqlitePreferencesRepository) Set(ctx context.Context, prefs domain.Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var anchorStr sql.NullString
	if !prefs.MesocycleAnchor.IsZero() {
		anchorStr = sql.NullString{Valid: true, String: formatDate(prefs.MesocycleAnchor)}
	}
	length := prefs.MesocycleLength
	if length == 0 {
		length = 5
	}

	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_preferences (
			user_id, monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
			friday_minutes, saturday_minutes, sunday_minutes, rest_notifications_enabled,
			deload_enabled, mesocycle_length, mesocycle_anchor
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			monday_minutes = excluded.monday_minutes,
			tuesday_minutes = excluded.tuesday_minutes,
			wednesday_minutes = excluded.wednesday_minutes,
			thursday_minutes = excluded.thursday_minutes,
			friday_minutes = excluded.friday_minutes,
			saturday_minutes = excluded.saturday_minutes,
			sunday_minutes = excluded.sunday_minutes,
			rest_notifications_enabled = excluded.rest_notifications_enabled,
			deload_enabled = excluded.deload_enabled,
			mesocycle_length = excluded.mesocycle_length,
			mesocycle_anchor = excluded.mesocycle_anchor`,
		userID,
		prefs.MondayMinutes, prefs.TuesdayMinutes, prefs.WednesdayMinutes, prefs.ThursdayMinutes,
		prefs.FridayMinutes, prefs.SaturdayMinutes, prefs.SundayMinutes,
		prefs.RestNotificationsEnabled,
		prefs.DeloadEnabled, length, anchorStr,
	); err != nil {
		return fmt.Errorf("save workout preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/repository/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/preferences.go internal/repository/preferences_test.go
git commit -m "Persist deload preferences via repository"
```

---

### Task 10: `SessionRepository` persists `is_deload`

**Files:**
- Modify: `internal/repository/sessions.go`
- Modify: `internal/repository/sessions_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/repository/sessions_test.go`, append:

```go
func TestSessionRepository_RoundTripIsDeload(t *testing.T) {
	ctx, repos, cleanup := setupTestRepos(t)
	defer cleanup()

	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		Date:              date,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          true,
	}
	if err := repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	got, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.IsDeload {
		t.Error("IsDeload = false, want true")
	}
}
```

(Match the file's existing setup helper name.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/repository/ -run TestSessionRepository_RoundTripIsDeload -v`
Expected: FAIL — `is_deload` not in queries.

- [ ] **Step 3: Update SELECTs**

In `internal/repository/sessions.go`:

`List` (line ~38) — change the SELECT to include `is_deload`, the scan destinations, and `parseSessionRow` arguments:

```go
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type, is_deload
		FROM workout_sessions
		WHERE user_id = ? AND workout_date >= ?
		ORDER BY workout_date DESC`,
		userID, sinceDateStr)
```

```go
	for rows.Next() {
		var (
			workoutDateStr    string
			difficultyRating  sql.NullInt32
			startedAtStr      sql.NullString
			completedAtStr    sql.NullString
			periodizationType domain.PeriodizationType
			isDeload          bool
		)
		if err = rows.Scan(
			&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType, &isDeload,
		); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		var session domain.Session
		session, err = parseSessionRow(
			workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType, isDeload,
		)
```

`get` (line ~92) — same shape:

```go
	var (
		workoutDateStr    string
		difficultyRating  sql.NullInt32
		startedAtStr      sql.NullString
		completedAtStr    sql.NullString
		periodizationType domain.PeriodizationType
		isDeload          bool
	)
	err := q.QueryRowContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type, is_deload
		FROM workout_sessions
		WHERE user_id = ? AND workout_date = ?`,
		userID, dateStr).Scan(&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType, &isDeload)
```

```go
	session, err := parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType, isDeload)
```

Update `parseSessionRow`:

```go
func parseSessionRow(
	workoutDateStr string,
	difficultyRating sql.NullInt32,
	startedAtStr sql.NullString,
	completedAtStr sql.NullString,
	periodizationType domain.PeriodizationType,
	isDeload bool,
) (domain.Session, error) {
	date, err := time.Parse(dateFormat, workoutDateStr)
	if err != nil {
		return domain.Session{}, fmt.Errorf("parse workout date: %w", err)
	}
	session := domain.Session{ //nolint:exhaustruct // ExerciseSets filled by caller.
		Date:              date,
		PeriodizationType: periodizationType,
		IsDeload:          isDeload,
	}
	// ... existing parsing of timestamps and difficulty stays unchanged ...
```

(Read the rest of `parseSessionRow` and leave the timestamp parsing untouched.)

Update `insertSession`:

```go
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workout_sessions (
			user_id, workout_date, difficulty_rating, started_at, completed_at, periodization_type, is_deload
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, dateStr, sess.DifficultyRating,
		formatTimestamp(sess.StartedAt), formatTimestamp(sess.CompletedAt),
		sess.PeriodizationType, sess.IsDeload); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/repository/ -count=1`
Expected: PASS, including the new round-trip test.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/sessions.go internal/repository/sessions_test.go
git commit -m "Persist Session.IsDeload through the session repository"
```

---

### Task 11: `GetLatestStartingWeightBefore` excludes deload sessions

**Files:**
- Modify: `internal/repository/sessions.go`
- Modify: `internal/repository/sessions_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/repository/sessions_test.go`:

```go
func TestSessionRepository_StartingWeight_SkipsDeloadSessions(t *testing.T) {
	ctx, repos, cleanup := setupTestRepos(t)
	defer cleanup()

	// Use a real exercise from the fixture so FK constraints are satisfied.
	exs, err := repos.Exercises.List(ctx)
	if err != nil {
		t.Fatalf("list exercises: %v", err)
	}
	var ex domain.Exercise
	for _, e := range exs {
		if e.ExerciseType == domain.ExerciseTypeWeighted && e.RepMin != nil {
			ex = e
			break
		}
	}
	if ex.ID == 0 {
		t.Fatal("no weighted exercise fixture available")
	}

	mondayNormal := time.Date(2026, time.April, 27, 0, 0, 0, 0, time.UTC)
	mondayDeload := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)

	weight100 := 100.0
	weight90 := 90.0
	onTarget := domain.SignalOnTarget
	completedAt := time.Date(2026, time.April, 27, 10, 0, 0, 0, time.UTC)

	normal := domain.Session{ //nolint:exhaustruct
		Date:              mondayNormal,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          false,
		ExerciseSets: []domain.ExerciseSet{ //nolint:exhaustruct
			{
				Exercise: ex,
				Sets: []domain.Set{
					{
						TargetValue:    10,
						WeightKg:       &weight100,
						CompletedValue: intPtrLocal(10),
						CompletedAt:    &completedAt,
						Signal:         &onTarget,
					},
				},
			},
		},
	}
	deload := domain.Session{ //nolint:exhaustruct
		Date:              mondayDeload,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          true,
		ExerciseSets: []domain.ExerciseSet{ //nolint:exhaustruct
			{
				Exercise: ex,
				Sets: []domain.Set{
					{
						TargetValue:    10,
						WeightKg:       &weight90,
						CompletedValue: intPtrLocal(10),
						CompletedAt:    &completedAt,
						Signal:         &onTarget,
					},
				},
			},
		},
	}

	if err := repos.Sessions.CreateBatch(ctx, []domain.Session{normal, deload}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	got, err := repos.Sessions.GetLatestStartingWeightBefore(ctx, ex.ID, mondayDeload.AddDate(0, 0, 7))
	if err != nil {
		t.Fatalf("GetLatestStartingWeightBefore: %v", err)
	}
	if got.WeightKg != 100.0 {
		t.Errorf("WeightKg = %v, want 100.0 (deload session must be excluded)", got.WeightKg)
	}
}

func intPtrLocal(i int) *int { return &i } // drop if a helper of this name already exists
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/repository/ -run TestSessionRepository_StartingWeight_SkipsDeloadSessions -v`
Expected: FAIL — returns 90 (the deload weight) rather than 100.

- [ ] **Step 3: Add the filter**

In `internal/repository/sessions.go`, update `GetLatestStartingWeightBefore`'s query — add `AND ws.is_deload = 0` to the WHERE clause:

```go
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT es.weight_kg, ws.periodization_type
		FROM exercise_sets es
		JOIN workout_exercise we ON we.id = es.workout_exercise_id
		JOIN workout_sessions ws
		  ON ws.user_id = we.workout_user_id
		 AND ws.workout_date = we.workout_date
		WHERE we.workout_user_id = ?
		  AND we.exercise_id = ?
		  AND we.workout_date < ?
		  AND ws.is_deload = 0
		  AND es.completed_value IS NOT NULL
		  AND es.weight_kg IS NOT NULL
		  AND es.signal IN ('on_target', 'too_light')
		ORDER BY we.workout_date DESC, es.set_number DESC
		LIMIT 1`,
		userID, exerciseID, beforeDateStr).Scan(&weightKg, &periodType)
```

Apply the same filter to `GetLatestSuccessfulSecondsBefore` (line 533+) — join `workout_sessions ws` and add `AND ws.is_deload = 0`:

```go
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT es.completed_value
		FROM exercise_sets es
		JOIN workout_exercise we ON we.id = es.workout_exercise_id
		JOIN workout_sessions ws
		  ON ws.user_id = we.workout_user_id
		 AND ws.workout_date = we.workout_date
		WHERE we.workout_user_id = ?
		  AND we.exercise_id = ?
		  AND we.workout_date < ?
		  AND ws.is_deload = 0
		  AND es.completed_value IS NOT NULL
		  AND es.signal IN ('on_target', 'too_light')
		ORDER BY we.workout_date DESC, es.set_number DESC
		LIMIT 1`,
		userID, exerciseID, formatDate(beforeDate)).Scan(&seconds)
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/repository/ -count=1`
Expected: PASS, including the new exclusion test.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/sessions.go internal/repository/sessions_test.go
git commit -m "Exclude deload sessions from starting-weight history lookups"
```

---

## Phase 3: Planner

### Task 12: Planner forces hypertrophy + IsDeload on deload weeks

**Files:**
- Modify: `internal/domain/planner.go`
- Modify: `internal/domain/planner_internal_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/domain/planner_internal_test.go`:

```go
func TestPlanner_DeloadWeekForcesHypertrophyAndHalvesSets(t *testing.T) {
	// Anchor on the same Monday we'll plan: week 0 of length 4 would NOT be a
	// deload (we want length-1 → 3, so plan on a date that is anchor + 21 days).
	anchor := time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC) // Monday
	planMonday := anchor.AddDate(0, 0, 21)                         // week 3 of 4 → deload

	prefs := Preferences{ //nolint:exhaustruct
		MondayMinutes:    60,
		TuesdayMinutes:   60,
		DeloadEnabled:    true,
		MesocycleLength:  4,
		MesocycleAnchor:  anchor,
	}
	// Use a fixture exercise list — borrow from another test helper if one exists;
	// otherwise build a minimal one:
	repMin, repMax := 8, 12
	exercises := []Exercise{
		{ //nolint:exhaustruct
			ID:                  1,
			Name:                "Bench Press",
			Category:            CategoryUpper,
			ExerciseType:        ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"chest"},
			RepMin:              &repMin,
			RepMax:              &repMax,
		},
		{ //nolint:exhaustruct
			ID:                  2,
			Name:                "Squat",
			Category:            CategoryLower,
			ExerciseType:        ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"quads"},
			RepMin:              &repMin,
			RepMax:              &repMax,
		},
	}
	targets := []MuscleGroupTarget{
		{MuscleGroupName: "chest", WeeklySetsTarget: 6},
		{MuscleGroupName: "quads", WeeklySetsTarget: 6},
	}
	wp := NewPlanner(prefs, exercises, targets)
	sessions, err := wp.Plan(planMonday)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(sessions) == 0 {
		t.Fatal("expected at least one session")
	}
	for _, s := range sessions {
		if !s.IsDeload {
			t.Errorf("session %s IsDeload = false, want true", s.Date.Format("2006-01-02"))
		}
		if s.PeriodizationType != PeriodizationHypertrophy {
			t.Errorf("session %s PeriodizationType = %s, want hypertrophy", s.Date.Format("2006-01-02"), s.PeriodizationType)
		}
		for _, es := range s.ExerciseSets {
			// Normal mid-rep band has 3 sets. Deload halves to 2.
			if len(es.Sets) != 2 {
				t.Errorf("session %s, exercise %s: %d sets, want 2 (deload halves)",
					s.Date.Format("2006-01-02"), es.Exercise.Name, len(es.Sets))
			}
			for _, set := range es.Sets {
				if set.TargetValue != 12 {
					t.Errorf("set TargetValue = %d, want 12 (repMax for hypertrophy)", set.TargetValue)
				}
			}
		}
	}
}

func TestPlanner_NonDeloadWeekUnchanged(t *testing.T) {
	anchor := time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC)
	planMonday := anchor.AddDate(0, 0, 7) // week 1 → not a deload

	prefs := Preferences{ //nolint:exhaustruct
		MondayMinutes:    60,
		DeloadEnabled:    true,
		MesocycleLength:  4,
		MesocycleAnchor:  anchor,
	}
	repMin, repMax := 8, 12
	exercises := []Exercise{
		{ //nolint:exhaustruct
			ID: 1, Name: "Bench", Category: CategoryFullBody,
			ExerciseType: ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"chest"},
			RepMin: &repMin, RepMax: &repMax,
		},
	}
	targets := []MuscleGroupTarget{{MuscleGroupName: "chest", WeeklySetsTarget: 3}}
	wp := NewPlanner(prefs, exercises, targets)
	sessions, err := wp.Plan(planMonday)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, s := range sessions {
		if s.IsDeload {
			t.Errorf("session %s IsDeload = true, want false (week 1 is not deload)", s.Date.Format("2006-01-02"))
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/domain/ -run 'TestPlanner_Deload|TestPlanner_NonDeload' -v`
Expected: FAIL — current planner doesn't read deload prefs.

- [ ] **Step 3: Implement**

In `internal/domain/planner.go`, locate `Plan(startingDate time.Time)`. Just before Phase 3 (line ~382, where periodization is picked per day), compute the deload flag once for the whole week:

```go
	// Determine periodization type for first session.
	firstPT := wp.firstSessionPeriodizationType(startingDate)
	isDeload := IsDeloadWeek(startingDate, wp.Prefs.MesocycleAnchor, wp.Prefs.MesocycleLength, wp.Prefs.DeloadEnabled)
```

Update the per-day loop (line ~385–406) to override periodization on deload weeks:

```go
	// Phase 3: select exercises and build sessions.
	weekUsedExercises := make(map[int]bool)
	sessions := make([]Session, len(workoutDays))
	for i, day := range workoutDays {
		pt := nextPeriodizationType(firstPT, i)
		if isDeload {
			pt = PeriodizationHypertrophy
		}
		n := exercisesPerSession(wp.Prefs, day.Weekday())
		exerciseSets := wp.selectExercisesForDayWithPeriodization(
			categories[day],
			dayMuscleGroups[day],
			n,
			pt,
			isDeload,
			weekUsedExercises,
		)

		for _, es := range exerciseSets {
			weekUsedExercises[es.Exercise.ID] = true
		}

		sessions[i] = Session{ //nolint:exhaustruct
			Date:              day,
			PeriodizationType: pt,
			IsDeload:          isDeload,
			ExerciseSets:      exerciseSets,
		}
	}

	return sessions, nil
}
```

Update `selectExercisesForDayWithPeriodization` to take an `isDeload bool` parameter and thread it into `buildPlannedExerciseSet`:

```go
func (wp *Planner) selectExercisesForDayWithPeriodization(
	category Category,
	priorityMuscleGroups []string,
	n int,
	pt PeriodizationType,
	isDeload bool,
	weekUsedExercises map[int]bool,
) []ExerciseSet {
	// ... existing body unchanged until the result build loop ...

	result := make([]ExerciseSet, len(selected))
	for i, ex := range selected {
		result[i] = buildPlannedExerciseSet(ex, pt, isDeload)
	}
	return result
}
```

Update `buildPlannedExerciseSet`:

```go
func buildPlannedExerciseSet(ex Exercise, pt PeriodizationType, isDeload bool) ExerciseSet {
	return ExerciseSet{ //nolint:exhaustruct
		Exercise: ex,
		Sets:     BuildPlannedSets(ex, pt, isDeload),
	}
}
```

Also update the wrapper `selectExercisesForDay` (which today calls the non-periodization variant) to pass `false`:

```go
func (wp *Planner) selectExercisesForDay(
	category Category,
	priorityMuscleGroups []string,
	n int,
) []ExerciseSet {
	return wp.selectExercisesForDayWithPeriodization(
		category, priorityMuscleGroups, n,
		PeriodizationStrength, false,
		make(map[int]bool),
	)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/domain/ -count=1`
Expected: PASS — the new tests and all prior planner tests.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/planner.go internal/domain/planner_internal_test.go
git commit -m "Force hypertrophy + halved sets on deload weeks in Planner.Plan"
```

---

## Phase 4: Service layer

### Task 13: `BuildProgression` propagates `IsDeload`

**Files:**
- Modify: `internal/service/progression.go`
- Modify: `internal/service/progression_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/service/progression_test.go`:

```go
func Test_BuildProgression_DeloadHoldsStartingWeight(t *testing.T) {
	ctx, svc, cleanup := setupTestService(t) // match existing helper name
	defer cleanup()

	// Seed a deload session for an exercise.
	exs, err := svc.ListExercises(ctx)
	if err != nil {
		t.Fatalf("list exercises: %v", err)
	}
	var ex domain.Exercise
	for _, e := range exs {
		if e.ExerciseType == domain.ExerciseTypeWeighted && e.RepMin != nil {
			ex = e
			break
		}
	}
	if ex.ID == 0 {
		t.Fatal("no weighted exercise fixture")
	}

	date := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	// Construct a deload session with a single planned set at 60kg.
	w := 60.0
	sess := domain.Session{ //nolint:exhaustruct
		Date:              date,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          true,
		ExerciseSets: []domain.ExerciseSet{ //nolint:exhaustruct
			{
				Exercise: ex,
				Sets: []domain.Set{
					{TargetValue: 12, WeightKg: &w},
				},
			},
		},
	}
	if err := svc.SaveSession(ctx, sess); err != nil { // or whatever the test helper exposes
		t.Fatalf("save session: %v", err)
	}

	prog, err := svc.BuildProgression(ctx, date, ex.ID)
	if err != nil {
		t.Fatalf("BuildProgression: %v", err)
	}
	target := prog.CurrentSet()
	if target.WeightKg != 60.0 {
		t.Errorf("CurrentSet WeightKg = %v, want 60.0 (deload holds starting weight)", target.WeightKg)
	}
}
```

If the test setup helper does not expose a way to save an arbitrary session, prefer `svc.RecordSet` chains or extend the helper minimally. Match existing patterns in `progression_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run Test_BuildProgression_DeloadHoldsStartingWeight -v`
Expected: FAIL — `IsDeload` not yet propagated.

- [ ] **Step 3: Implement**

In `internal/service/progression.go`, update `BuildProgression` to thread `sess.IsDeload`:

```go
	config := domain.Config{
		Type:           sess.PeriodizationType,
		RepMin:         *exercise.RepMin,
		RepMax:         *exercise.RepMax,
		StartingWeight: startingWeight,
		IsDeload:       sess.IsDeload,
	}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/service/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/progression.go internal/service/progression_test.go
git commit -m "Propagate Session.IsDeload into Progression.Config"
```

---

### Task 14: Rest scheduling skips deload sessions; service callers thread `isDeload`

**Files:**
- Modify: `internal/service/sets.go`

- [ ] **Step 1: Read the call site**

Open `internal/service/sets.go`. Find `maybeSchedulePush` (line ~105) and the place it is called from `RecordSet` (line ~96).

- [ ] **Step 2: Update the signature and behaviour**

Inside `RecordSet`, the inner `Update` closure already reads `sess.PeriodizationType`. Also capture `sess.IsDeload` and pass it through. Replace the relevant block:

```go
	var (
		wasComplete        bool
		exercise           domain.Exercise
		periodization      domain.PeriodizationType
		sessionIsDeload    bool
		completedSetNumber int
		setsTotal          int
		hasMoreAfter       bool
	)
	now := time.Now().UTC()

	err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		for i := range sess.ExerciseSets {
			if sess.ExerciseSets[i].ID != workoutExerciseID {
				continue
			}
			if setIndex < 0 || setIndex >= len(sess.ExerciseSets[i].Sets) {
				break
			}
			wasComplete = sess.ExerciseSets[i].Sets[setIndex].CompletedAt != nil
			exercise = sess.ExerciseSets[i].Exercise
			completedSetNumber = setIndex + 1
			setsTotal = len(sess.ExerciseSets[i].Sets)
			break
		}
		periodization = sess.PeriodizationType
		sessionIsDeload = sess.IsDeload

		if recErr := sess.RecordSet(workoutExerciseID, setIndex, signal, weightKg, completedValue, now); recErr != nil {
			return recErr //nolint:wrapcheck
		}
		hasMoreAfter = sess.HasIncompleteSets()
		return nil
	})
```

Then change the `maybeSchedulePush` invocation:

```go
	if !wasComplete && hasMoreAfter {
		s.maybeSchedulePush(ctx, workoutExerciseID, exercise, periodization, sessionIsDeload, completedSetNumber, setsTotal, now)
	}
```

And update `maybeSchedulePush` to accept and forward the flag:

```go
func (s *Service) maybeSchedulePush(
	ctx context.Context,
	workoutExerciseID int,
	exercise domain.Exercise,
	periodization domain.PeriodizationType,
	isDeload bool,
	completedSetNumber, setsTotal int,
	completedAt time.Time,
) {
	if s.scheduler == nil {
		return
	}
	restSeconds := domain.RestSecondsFor(exercise, periodization, isDeload)
	if restSeconds <= 0 {
		return
	}
	// ... rest of the function unchanged ...
```

- [ ] **Step 3: Run all tests**

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/service/sets.go
git commit -m "Thread session IsDeload into RestSecondsFor call"
```

---

### Task 15: `SaveUserPreferences` snaps `MesocycleAnchor` on enable

**Files:**
- Modify: `internal/service/service.go` (contains `SaveUserPreferences` at line 77 and `GetUserPreferences` at line 68)
- Modify: `internal/service/service_test.go` (create if it doesn't exist; otherwise use the file your environment's `grep -rn "TestService_" internal/service/` points at)

- [ ] **Step 1: Confirm location**

Run: `grep -n "func.*SaveUserPreferences\|func.*GetUserPreferences" internal/service/service.go`
Expected: two hits in `service.go`.

- [ ] **Step 2: Write the failing test**

In the matching `_test.go` file, append:

```go
func Test_SaveUserPreferences_SnapsAnchorOnEnable(t *testing.T) {
	ctx, svc, cleanup := setupTestService(t)
	defer cleanup()

	// Anchor starts zero. Enable deload and confirm anchor lands on a Monday >= today.
	prefs := domain.Preferences{ //nolint:exhaustruct
		MondayMinutes:   60,
		DeloadEnabled:   true,
		MesocycleLength: 5,
	}
	if err := svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	got, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	if got.MesocycleAnchor.IsZero() {
		t.Fatal("MesocycleAnchor was not set on enable")
	}
	if got.MesocycleAnchor.Weekday() != time.Monday {
		t.Errorf("MesocycleAnchor weekday = %s, want Monday", got.MesocycleAnchor.Weekday())
	}
	if got.MesocycleAnchor.Before(time.Now().UTC().Truncate(24 * time.Hour)) {
		t.Errorf("MesocycleAnchor = %s is in the past", got.MesocycleAnchor)
	}
}

func Test_SaveUserPreferences_NoSnapWhenAnchorAlreadySet(t *testing.T) {
	ctx, svc, cleanup := setupTestService(t)
	defer cleanup()

	existing := time.Date(2026, time.April, 6, 0, 0, 0, 0, time.UTC)
	// First enable with an explicit anchor.
	first := domain.Preferences{ //nolint:exhaustruct
		MondayMinutes:   60,
		DeloadEnabled:   true,
		MesocycleLength: 5,
		MesocycleAnchor: existing,
	}
	if err := svc.SaveUserPreferences(ctx, first); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// Toggle a non-deload field; anchor should not move.
	first.MondayMinutes = 90
	if err := svc.SaveUserPreferences(ctx, first); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.MesocycleAnchor.Equal(existing) {
		t.Errorf("MesocycleAnchor = %s, want %s", got.MesocycleAnchor, existing)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/service/ -run Test_SaveUserPreferences_Snaps -v`
Expected: FAIL — anchor remains zero.

- [ ] **Step 4: Implement**

In the same service file, locate `SaveUserPreferences` (or equivalent). Add anchor-snap logic. The shape is:

```go
func (s *Service) SaveUserPreferences(ctx context.Context, prefs domain.Preferences) error {
	current, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("load current preferences: %w", err)
	}
	// If the user just enabled deload (or has it enabled but the anchor is
	// missing), snap the anchor to the next Monday so the first cycle starts
	// with an accumulation week.
	if prefs.DeloadEnabled && prefs.MesocycleAnchor.IsZero() && current.MesocycleAnchor.IsZero() {
		prefs.MesocycleAnchor = nextMonday(s.clock.Now())
	}
	// If the user is keeping their existing anchor, preserve it when prefs
	// doesn't carry one (handler may post a partial update).
	if prefs.DeloadEnabled && prefs.MesocycleAnchor.IsZero() && !current.MesocycleAnchor.IsZero() {
		prefs.MesocycleAnchor = current.MesocycleAnchor
	}
	if err := s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	return nil
}
```

If the service struct does not yet expose a clock, use `time.Now().UTC()` directly to match existing patterns. Search the file for `time.Now()` to match the convention.

Add the helper `nextMonday` to the same file (or to an existing helpers file in `internal/service/`):

```go
// nextMonday returns the upcoming Monday at 00:00 UTC. If now is a Monday,
// it returns now (truncated to date).
func nextMonday(now time.Time) time.Time {
	d := now.UTC().Truncate(24 * time.Hour)
	for d.Weekday() != time.Monday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/service/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/service.go internal/service/service_test.go
git commit -m "Snap MesocycleAnchor to next Monday on deload enable"
```

---

### Task 16: `GetStartingWeight` for deload sessions uses 90% of hypertrophy history

**Files:**
- Modify: `internal/service/progression.go`
- Modify: `internal/service/progression_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/service/progression_test.go`:

```go
func Test_GetStartingWeight_DeloadAppliesNinetyPercent(t *testing.T) {
	ctx, svc, cleanup := setupTestService(t)
	defer cleanup()

	exs, _ := svc.ListExercises(ctx)
	var ex domain.Exercise
	for _, e := range exs {
		if e.ExerciseType == domain.ExerciseTypeWeighted && e.RepMin != nil {
			ex = e
			break
		}
	}
	if ex.ID == 0 {
		t.Fatal("no weighted exercise fixture")
	}

	monday := time.Date(2026, time.April, 27, 0, 0, 0, 0, time.UTC)
	deloadMonday := time.Date(2026, time.May, 4, 0, 0, 0, 0, time.UTC)
	w := 80.0
	completedAt := time.Date(2026, time.April, 27, 10, 0, 0, 0, time.UTC)
	onTarget := domain.SignalOnTarget

	hypertrophySess := domain.Session{ //nolint:exhaustruct
		Date:              monday,
		PeriodizationType: domain.PeriodizationHypertrophy,
		IsDeload:          false,
		ExerciseSets: []domain.ExerciseSet{ //nolint:exhaustruct
			{
				Exercise: ex,
				Sets: []domain.Set{
					{TargetValue: 12, WeightKg: &w, CompletedValue: intPtrSvc(12), CompletedAt: &completedAt, Signal: &onTarget},
				},
			},
		},
	}
	if err := svc.SaveSession(ctx, hypertrophySess); err != nil {
		t.Fatalf("save: %v", err)
	}

	// New call: GetDeloadStartingWeight returns 80 × 0.9 = 72 (snapped to 0.5).
	got, err := svc.GetDeloadStartingWeight(ctx, ex.ID, deloadMonday)
	if err != nil {
		t.Fatalf("GetDeloadStartingWeight: %v", err)
	}
	if got != 72.0 {
		t.Errorf("got %v, want 72.0 (= 80 * 0.9, snapped)", got)
	}
}

func intPtrSvc(i int) *int { return &i }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/service/ -run Test_GetStartingWeight_DeloadAppliesNinetyPercent -v`
Expected: FAIL — `GetDeloadStartingWeight` undefined.

- [ ] **Step 3: Implement**

Append to `internal/service/progression.go`:

```go
const (
	deloadFactor     = 0.90
	deloadFallback   = 0.80
)

// GetDeloadStartingWeight returns the seed weight for a deload week's first
// set of the given exercise: 90% of the most recent hypertrophy working
// weight, falling back to 80% of any recent working weight, then to the
// exercise's default. Snapped via the existing weight-grid rule.
//
// The repository's GetLatestStartingWeightBefore already excludes deload
// sessions (Task 11), so the lookups below see only normal-week history.
func (s *Service) GetDeloadStartingWeight(
	ctx context.Context,
	exerciseID int,
	beforeDate time.Time,
) (float64, error) {
	prev, err := s.repos.Sessions.GetLatestStartingWeightBefore(ctx, exerciseID, beforeDate)
	if err != nil {
		return 0, fmt.Errorf("get latest starting weight: %w", err)
	}
	if prev.PeriodizationType == domain.PeriodizationHypertrophy && prev.WeightKg > 0 {
		return domain.SnapWeightKg(prev.WeightKg * deloadFactor), nil
	}
	// No hypertrophy history (or zero weight): use the broader fallback.
	if prev.WeightKg > 0 {
		return domain.SnapWeightKg(prev.WeightKg * deloadFallback), nil
	}
	return 0, nil
}
```

Then expose `SnapWeightKg` from the domain package. In `internal/domain/progression.go`, change the unexported `snapWeight` to a thin wrapper around a new exported helper:

```go
// SnapWeightKg rounds a kilo value to the nearest realisable load: 1kg
// inside the dumbbell range (|w| < 10kg), 0.5kg above. Exposed for service
// callers that derive weights outside the per-set progression flow.
func SnapWeightKg(kg float64) float64 {
	if math.Abs(kg) < dumbbellThresholdKg {
		return math.Round(kg)
	}
	const halfKg = 0.5
	return math.Round(kg/halfKg) * halfKg
}

func snapWeight(kg float64) float64 { return SnapWeightKg(kg) }
```

Modify `BuildProgression` to use the deload-aware seed when the session is a deload:

```go
	var startingWeight float64
	if sess.IsDeload {
		startingWeight, err = s.GetDeloadStartingWeight(ctx, exerciseID, date)
	} else {
		startingWeight, err = s.GetStartingWeight(ctx, exerciseID, date, sess.PeriodizationType)
	}
	if err != nil {
		return nil, fmt.Errorf("get starting weight: %w", err)
	}
```

- [ ] **Step 4: Run tests**

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/progression.go internal/service/progression_test.go internal/domain/progression.go
git commit -m "Add GetDeloadStartingWeight (90% of prior hypertrophy load)"
```

---

### Task 17: Planner seeds deload-week set weights via the service

**Files:**
- Modify: `internal/service/sessions.go` (the call to `planner.Plan(monday)` lives in `RegenerateWeeklyPlanIfUnstarted` around line 105; `CreateBatch` follows at line 110)

- [ ] **Step 1: Inject deload weights between planning and persistence**

In `internal/service/sessions.go`, in `RegenerateWeeklyPlanIfUnstarted`, insert a loop between the `planner.Plan(monday)` call (line 105) and the `s.repos.Sessions.CreateBatch(ctx, plannedSessions)` call (line 110):

```go
	for i := range plannedSessions {
		if !plannedSessions[i].IsDeload {
			continue
		}
		for j := range plannedSessions[i].ExerciseSets {
			ex := plannedSessions[i].ExerciseSets[j].Exercise
			if !ex.HasWeight() {
				continue
			}
			w, err := s.GetDeloadStartingWeight(ctx, ex.ID, plannedSessions[i].Date)
			if err != nil {
				return fmt.Errorf("seed deload weight for %s: %w", ex.Name, err)
			}
			weight := w
			for k := range plannedSessions[i].ExerciseSets[j].Sets {
				plannedSessions[i].ExerciseSets[j].Sets[k].WeightKg = &weight
			}
		}
	}
```

(The deload prescription pre-stamps the same weight on every set; the user does not progress within the session.)

- [ ] **Step 2: Run the full test suite**

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/service/sessions.go
git commit -m "Pre-stamp deload-week set weights from hypertrophy history"
```

---

## Phase 5: Web & UI

### Task 18: Preferences handler accepts deload form fields

**Files:**
- Modify: `cmd/web/handler-preferences.go`

- [ ] **Step 1: Update the template data struct**

In `cmd/web/handler-preferences.go`, extend `preferencesTemplateData`:

```go
type preferencesTemplateData struct {
	BaseTemplateData
	Weekdays                 []weekdayPreference
	DurationOptions          []workoutDurationOption
	VAPIDPublicKey           string
	PushSubscriptionCount    int
	RestNotificationsEnabled bool
	DeloadEnabled            bool
	MesocycleLength          int
	MesocycleLengthOptions   []int
	MesocycleAnchor          time.Time
}
```

- [ ] **Step 2: Populate it in `preferencesGET`**

Add to the `data := preferencesTemplateData{ ... }` literal:

```go
		DeloadEnabled:          prefs.DeloadEnabled,
		MesocycleLength:        prefs.MesocycleLength,
		MesocycleLengthOptions: []int{4, 5, 6, 7},
		MesocycleAnchor:        prefs.MesocycleAnchor,
```

- [ ] **Step 3: Read and validate the new POST fields**

In `preferencesPOST`, after the weekday-minute parses, add:

```go
	prefs.DeloadEnabled = r.Form.Get("deload_enabled") == "on"
	prefs.MesocycleLength = parseMesocycleLength(r.Form.Get("mesocycle_length"))
```

Add `parseMesocycleLength` near `parseMinutes`:

```go
func parseMesocycleLength(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return 5 // default
	}
	if n < 4 || n > 7 {
		return 5
	}
	return n
}
```

- [ ] **Step 4: Add a "Restart cycle" endpoint**

In the same file, add:

```go
func (app *application) preferencesRestartMesocyclePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}
	if err := app.service.RestartMesocycleAnchor(r.Context()); err != nil {
		app.serverError(w, r, fmt.Errorf("restart mesocycle: %w", err))
		return
	}
	redirect(w, r, "/preferences")
}
```

You will register the new route in Task 21.

- [ ] **Step 5: Add `RestartMesocycleAnchor` to the service**

In `internal/service/service.go` (the file you modified in Task 15), add:

```go
func (s *Service) RestartMesocycleAnchor(ctx context.Context) error {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
	if err := s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-preferences.go internal/service/service.go
git commit -m "Handle deload toggle and restart-cycle in preferences"
```

---

### Task 19: Preferences template — Deload section

**Files:**
- Modify: `ui/templates/pages/preferences/preferences.gohtml`

- [ ] **Step 1: Inspect the existing template**

Run: `grep -n "fieldset\|notifications" ui/templates/pages/preferences/preferences.gohtml | head -20` and skim the structure. The rest-notifications section gives you the layout convention to follow.

- [ ] **Step 2: Add the Deload section**

After the rest-notifications section (or wherever feels semantically grouped), append:

```gohtml
<section class="deload-section" aria-labelledby="deload-heading">
    <style {{ nonce }}>
        @scope (.deload-section) {
            :scope {
                margin-top: var(--size-5);
                padding: var(--size-4);
                background: var(--gray-0);
                border-radius: var(--radius-2);

                h2 {
                    margin-top: 0;
                }

                .deload-controls {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-3);
                }

                .anchor-line {
                    font-size: var(--font-size-0);
                    color: var(--gray-7);
                }
            }
        }
    </style>

    <h2 id="deload-heading">Deload</h2>
    <p>Planned deload weeks give your body time to recover. On a deload week, every session
       runs lighter and shorter — the goal is recovery, not progress.</p>

    <form method="post" action="/preferences" class="deload-controls">
        <label>
            <input type="checkbox" name="deload_enabled" {{ if .DeloadEnabled }}checked{{ end }}>
            Enable planned deloads
        </label>

        <label>
            Block length
            <select name="mesocycle_length">
                {{ range .MesocycleLengthOptions }}
                    <option value="{{ . }}" {{ if eq . $.MesocycleLength }}selected{{ end }}>
                        {{ . }} weeks ({{ printf "%d+1" (sub . 1) }})
                    </option>
                {{ end }}
            </select>
        </label>

        <!-- We re-post the rest of the prefs so the existing form handler stays
             a single endpoint. Hidden mirrors of the weekday selects: -->
        {{ range .Weekdays }}
            <input type="hidden" name="{{ .ID }}_minutes" value="{{ .Minutes }}">
        {{ end }}

        <button type="submit">Save deload settings</button>
    </form>

    {{ if and .DeloadEnabled (not .MesocycleAnchor.IsZero) }}
        <p class="anchor-line">Current cycle starts on {{ .MesocycleAnchor.Format "Mon 2 Jan 2006" }}.</p>
    {{ end }}

    <form method="post" action="/preferences/mesocycle/restart">
        <button type="submit" {{ if not .DeloadEnabled }}disabled{{ end }}>
            Restart cycle from next Monday
        </button>
    </form>
</section>
```

If `sub` is not already a registered template function, add it. The function maps live in `cmd/web/handlers.go` in `baseTemplateFuncs()` (around line 29). Append a new entry inside the returned `template.FuncMap{}`:

```go
		"sub": func(a, b int) int { return a - b },
```

The project does not use sprig, so this addition is required.

- [ ] **Step 3: Build and run an ad-hoc render check**

Run: `make build && go test ./cmd/web/ -run TestPreferences -count=1`
Expected: PASS, the preferences-page tests still render the template without errors.

- [ ] **Step 4: Commit**

```bash
git add ui/templates/pages/preferences/preferences.gohtml cmd/web/handlers.go
git commit -m "Preferences page: deload section with cadence and restart"
```

---

### Task 20: Plan / week view chip "Week N of M · Deload"

**Files:**
- Modify: `cmd/web/handler-home.go` (the weekly schedule is rendered via the `homeTemplateData` struct at line 49; populated at line 386; sessions resolved at line 405)
- Modify: `ui/templates/pages/home/home.gohtml`

- [ ] **Step 1: Confirm the data struct**

Run: `grep -n "homeTemplateData\|ResolveWeeklySchedule" cmd/web/handler-home.go`
Expected: the struct at line 49 and the resolver call near line 405.

Add to the `homeTemplateData` struct:

```go
	WeekInBlock      int    // 1-based for display
	MesocycleLength  int
	IsDeloadWeek     bool
```

- [ ] **Step 2: Compute the values in the handler**

After loading prefs and the Monday-of date in the home GET handler, add:

```go
	weekInBlock := domain.WeekInBlock(monday, prefs.MesocycleAnchor, prefs.MesocycleLength)
	isDeload := domain.IsDeloadWeek(monday, prefs.MesocycleAnchor, prefs.MesocycleLength, prefs.DeloadEnabled)
	data.WeekInBlock = weekInBlock + 1
	data.MesocycleLength = prefs.MesocycleLength
	data.IsDeloadWeek = isDeload
```

- [ ] **Step 3: Render the chip**

In the template that draws the week header (`ui/templates/pages/home/home.gohtml` or the relevant partial), near the existing date display, add:

```gohtml
{{ if and .MesocycleLength (gt .MesocycleLength 0) }}
    <span class="week-chip{{ if .IsDeloadWeek }} week-chip--deload{{ end }}">
        Week {{ .WeekInBlock }} of {{ .MesocycleLength }}{{ if .IsDeloadWeek }} · Deload{{ end }}
    </span>
{{ end }}
```

With a colocated `<style {{ nonce }}>` block:

```gohtml
<style {{ nonce }}>
    @scope (.week-chip) {
        :scope {
            display: inline-block;
            padding: var(--size-1) var(--size-2);
            border-radius: var(--radius-2);
            background: var(--gray-1);
            color: var(--gray-9);
            font-size: var(--font-size-0);

            &.week-chip--deload {
                background: var(--sky-2);
                color: var(--sky-10);
                font-weight: var(--font-weight-7);
            }
        }
    }
</style>
```

(Place the style block immediately before the chip's first occurrence.)

- [ ] **Step 4: Run the home tests**

Run: `go test ./cmd/web/ -run TestHome -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/web/handler-home.go ui/templates/pages/home/home.gohtml
git commit -m "Home page: week-in-block chip with deload variant"
```

---

### Task 21: Deload banner + hide signal buttons on session page

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`
- Modify: `cmd/web/handler-exerciseset.go` (template data)

- [ ] **Step 1: Pass `IsDeload` to the template**

In `cmd/web/handler-exerciseset.go`, find the struct passed to the `exerciseset` template (search the file for `app.render(.*"exerciseset"`). Add a field:

```go
	IsDeload bool
```

Populate it from `session.IsDeload` in the GET handler.

- [ ] **Step 2: Add the banner**

At the top of `ui/templates/pages/exerciseset/sets-container.gohtml` (after the existing header partial), add:

```gohtml
{{ if .IsDeload }}
    <style {{ nonce }}>
        @scope (.deload-banner) {
            :scope {
                margin: var(--size-3) 0;
                padding: var(--size-3) var(--size-4);
                background: var(--sky-1);
                color: var(--sky-10);
                border-left: var(--border-size-3) solid var(--sky-7);
                border-radius: var(--radius-2);
                font-size: var(--font-size-1);
            }
        }
    </style>
    <div class="deload-banner" role="status">
        Deload week — lighter loads, same weight every set. Just hit your reps and rest.
        These sets don't influence future progression.
    </div>
{{ end }}
```

- [ ] **Step 3: Hide the signal fieldset and add a Done button on deload**

In the weighted-form block (around line 442–455), wrap the `<fieldset class="signal-group">` in a conditional, and add an alternate Done button:

```gohtml
{{ if $.IsDeload }}
    <button type="submit" class="submit-button" aria-label="Complete set">Done!</button>
{{ else }}
    <fieldset class="signal-group">
        <legend>Did you reach {{ $.CurrentSetTarget.TargetReps }} reps?</legend>
        <div class="signal-buttons">
            <button type="submit" name="signal" value="too_heavy"
                    class="signal-btn too-heavy-btn"
                    aria-label="No, I failed to reach target reps">No</button>
            <button type="submit" name="signal" value="on_target"
                    class="signal-btn on-target-btn"
                    aria-label="Barely reached target reps">Barely</button>
            <button type="submit" name="signal" value="too_light"
                    class="signal-btn too-light-btn"
                    aria-label="Could have done more reps">Could do more</button>
        </div>
    </fieldset>
{{ end }}
```

Apply the same conditional to the timed-form block (around line 475–488). The bodyweight-form already has a "Done!" button and no signals, so leave it alone.

- [ ] **Step 4: Verify the handler accepts no-signal submissions**

Task 7 already updated the handler to read `r.PostForm.Get("signal")` and treat empty as nil. No further handler change is needed.

- [ ] **Step 5: Run the exerciseset tests**

Run: `go test ./cmd/web/ -run 'TestExerciseSet' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml cmd/web/handler-exerciseset.go
git commit -m "Session page: deload banner and hidden signal buttons"
```

---

### Task 22: Register the restart-mesocycle route

**Files:**
- Modify: `cmd/web/routes.go`

- [ ] **Step 1: Read the existing routes**

Run: `grep -n "preferences" cmd/web/routes.go`

- [ ] **Step 2: Register the new POST**

Near the existing preferences routes, add:

```go
	mux.Handle("POST /preferences/mesocycle/restart", authStack(http.HandlerFunc(app.preferencesRestartMesocyclePOST)))
```

(Match the existing middleware-stack wrapper used by the neighbouring `preferencesPOST` registration — look at the surrounding lines for the exact `authStack` / `csrfStack` etc.)

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/web/routes.go
git commit -m "Route POST /preferences/mesocycle/restart"
```

---

## Phase 6: Verification

### Task 23: End-to-end test for the deload-day session page

**Files:**
- Modify: `cmd/web/handler-exerciseset_test.go`

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestExerciseSetGET_DeloadHidesSignalButtons(t *testing.T) {
	ctx := context.Background()
	server, err := e2etest.StartServer(ctx, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()
	client := server.Client()

	// Set up a user with a deload week starting today.
	// (Follow the pattern used by neighbouring tests in this file to authenticate
	// and seed a session.)
	// ...

	doc, err := client.GetDocument(ctx, fmt.Sprintf("/workouts/%s/exercises/%d/sets/0",
		deloadDate.Format("2006-01-02"), workoutExerciseID))
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if doc.Find("button.signal-btn").Length() != 0 {
		t.Error("signal buttons rendered on deload session, want zero")
	}
	if doc.Find(".deload-banner").Length() == 0 {
		t.Error("deload banner not rendered")
	}
	if doc.Find("button:contains('Done!')").Length() == 0 {
		t.Error("Done button not rendered on deload session")
	}
}
```

Replace the "Set up a user with a deload week starting today" block with the concrete seeding used by neighbouring tests; the existing test file should have a fixture helper for authenticated requests.

- [ ] **Step 2: Run it**

Run: `go test ./cmd/web/ -run TestExerciseSetGET_DeloadHidesSignalButtons -v`
Expected: PASS if Tasks 18–22 are in place.

- [ ] **Step 3: Commit**

```bash
git add cmd/web/handler-exerciseset_test.go
git commit -m "E2E: deload session hides signal buttons"
```

---

### Task 24: Full CI

- [ ] **Step 1: Run lint with autofix**

Run: `make lint-fix`
Expected: zero errors. Stage any formatting fixes.

- [ ] **Step 2: Run full CI**

Run: `make ci`
Expected: PASS.

- [ ] **Step 3: Commit any final fixups**

If `make lint-fix` produced changes:

```bash
git add -u
git commit -m "Lint fixups"
```

- [ ] **Step 4: Done**

Verify the working tree is clean (besides pre-existing unrelated modifications in `ui/static/main.js` / `ui/templates/...` if any).

Run: `git status`
Expected: clean (modulo the pre-existing modifications).

---

## Self-review checklist (run after implementation)

- [ ] `IsDeloadWeek(date, anchor, length, enabled)` returns false for length<2, zero anchor, or disabled — covered in Task 1.
- [ ] `DeriveScheme(..., isDeload=true)` returns hypertrophy reps + halved sets regardless of incoming `p` — Task 4.
- [ ] `Session.IsDeload` round-trips through repo (insert + read) — Task 10.
- [ ] `GetLatestStartingWeightBefore` excludes deload rows in BOTH the weight and seconds queries — Task 11.
- [ ] Planner sets `IsDeload=true` and `PeriodizationType=Hypertrophy` on every session of a deload week — Task 12.
- [ ] `Progression.CurrentSet()` returns starting weight for every call when `Config.IsDeload` — Task 5.
- [ ] Service `BuildProgression` propagates `IsDeload` — Task 13.
- [ ] Service stamps the 90% deload weight on every set when planning the week — Task 17.
- [ ] Service `SaveUserPreferences` snaps anchor on deload enable; `RestartMesocycleAnchor` re-snaps — Tasks 15 & 18.
- [ ] Preferences template offers toggle, cadence dropdown, restart button — Task 19.
- [ ] Home view shows "Week N of M · Deload" chip — Task 20.
- [ ] Session page shows banner and "Done!" button (no signals) on deload — Tasks 21 & 23.
- [ ] `make ci` is green — Task 24.
