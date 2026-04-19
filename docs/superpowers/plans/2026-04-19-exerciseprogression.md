# exerciseprogression Package Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `internal/exerciseprogression` — a pure-logic Go package that manages set-to-set weight progression for a single weighted exercise execution using RIR-based auto-regulation.

**Architecture:** A stateful `Progression` struct is created once per exercise execution, configured with periodization type and starting weight. The caller calls `CurrentSet()` to get the next set target, then `RecordCompletion()` with what the user actually did and their RIR signal. Weight adjusts from the actual lifted weight (not the recommendation), so user overrides propagate naturally.

**Tech Stack:** Go 1.26, standard library only (`math` for rounding). No external dependencies.

---

## File Map

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/exerciseprogression/progression.go` | All types, constants, `Progression` struct, all methods |
| Create | `internal/exerciseprogression/progression_test.go` | Table-driven tests for all behaviour |

---

### Task 1: Scaffold package with types and constants

**Files:**
- Create: `internal/exerciseprogression/progression.go`

- [ ] **Step 1: Create the file with all types and unexported constants**

```go
// internal/exerciseprogression/progression.go
package exerciseprogression

import "math"

// Signal is the user's perceived effort after completing a set.
type Signal int

const (
	SignalTooHeavy Signal = iota // failed to complete target reps
	SignalOnTarget               // completed reps with ~1-2 in reserve
	SignalTooLight               // completed reps with 2+ in reserve
)

// PeriodizationType determines the fixed rep target for the exercise.
type PeriodizationType int

const (
	Strength    PeriodizationType = iota // 5 reps
	Hypertrophy                          // 8 reps
	Endurance                            // 15 reps
)

// Config is provided once when starting an exercise execution.
type Config struct {
	Type           PeriodizationType
	StartingWeight float64 // kg; caller-derived from history, may be user-overridden
}

// SetTarget is what the package recommends for the upcoming set.
type SetTarget struct {
	WeightKg   float64
	TargetReps int
}

// SetResult is recorded by the caller after the user completes a set.
type SetResult struct {
	ActualReps int
	Signal     Signal
	WeightKg   float64 // weight actually used; may differ from recommendation if user overrode
}

const (
	repsStrength    = 5
	repsHypertrophy = 8
	repsEndurance   = 15

	weightIncrementKg     = 2.5
	weightDecrementFactor = 0.10
)

// Progression manages set-to-set weight progression for one exercise execution.
type Progression struct {
	config    Config
	completed []SetResult
}

// New creates a Progression for a new exercise execution.
func New(config Config) *Progression {
	return &Progression{config: config}
}

// NewFromHistory reconstructs a Progression from sets already completed in this session.
func NewFromHistory(config Config, completed []SetResult) *Progression {
	p := New(config)
	p.completed = make([]SetResult, len(completed))
	copy(p.completed, completed)
	return p
}

// CurrentSet returns the recommended target for the next set.
func (p *Progression) CurrentSet() SetTarget {
	reps := targetReps(p.config.Type)
	if len(p.completed) == 0 {
		return SetTarget{WeightKg: p.config.StartingWeight, TargetReps: reps}
	}
	last := p.completed[len(p.completed)-1]
	weight := adjustedWeight(last)
	return SetTarget{WeightKg: weight, TargetReps: reps}
}

// RecordCompletion records what actually happened and advances internal state.
func (p *Progression) RecordCompletion(result SetResult) {
	p.completed = append(p.completed, result)
}

// SetsCompleted returns the number of sets recorded so far.
func (p *Progression) SetsCompleted() int {
	return len(p.completed)
}

func targetReps(t PeriodizationType) int {
	switch t {
	case Strength:
		return repsStrength
	case Hypertrophy:
		return repsHypertrophy
	case Endurance:
		return repsEndurance
	default:
		return repsHypertrophy
	}
}

func adjustedWeight(last SetResult) float64 {
	switch last.Signal {
	case SignalTooLight:
		return last.WeightKg + weightIncrementKg
	case SignalTooHeavy:
		decreased := last.WeightKg * (1 - weightDecrementFactor)
		return roundToHalf(decreased)
	default: // SignalOnTarget
		return last.WeightKg
	}
}

func roundToHalf(kg float64) float64 {
	return math.Round(kg/0.5) * 0.5
}
```

- [ ] **Step 2: Verify the package compiles**

```bash
go build github.com/myrjola/petrapp/internal/exerciseprogression
```

Expected: no output, exit 0.

- [ ] **Step 3: Commit the scaffold**

```bash
git add internal/exerciseprogression/progression.go
git commit -m "feat: scaffold exerciseprogression package with types and constants"
```

---

### Task 2: TDD — `New` and `CurrentSet` for first set

**Files:**
- Create: `internal/exerciseprogression/progression_test.go`

- [ ] **Step 1: Write failing tests for first-set behaviour**

```go
// internal/exerciseprogression/progression_test.go
package exerciseprogression_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
)

func TestCurrentSet_FirstSet(t *testing.T) {
	tests := []struct {
		name           string
		periodization  exerciseprogression.PeriodizationType
		startingWeight float64
		wantReps       int
		wantWeight     float64
	}{
		{
			name:           "strength returns 5 reps",
			periodization:  exerciseprogression.Strength,
			startingWeight: 80.0,
			wantReps:       5,
			wantWeight:     80.0,
		},
		{
			name:           "hypertrophy returns 8 reps",
			periodization:  exerciseprogression.Hypertrophy,
			startingWeight: 60.0,
			wantReps:       8,
			wantWeight:     60.0,
		},
		{
			name:           "endurance returns 15 reps",
			periodization:  exerciseprogression.Endurance,
			startingWeight: 40.0,
			wantReps:       15,
			wantWeight:     40.0,
		},
		{
			name:           "zero starting weight is returned as-is",
			periodization:  exerciseprogression.Hypertrophy,
			startingWeight: 0.0,
			wantReps:       8,
			wantWeight:     0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := exerciseprogression.New(exerciseprogression.Config{
				Type:           tt.periodization,
				StartingWeight: tt.startingWeight,
			})
			got := p.CurrentSet()
			if got.TargetReps != tt.wantReps {
				t.Errorf("TargetReps = %d, want %d", got.TargetReps, tt.wantReps)
			}
			if got.WeightKg != tt.wantWeight {
				t.Errorf("WeightKg = %v, want %v", got.WeightKg, tt.wantWeight)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify tests pass (scaffold already implements this)**

```bash
go test github.com/myrjola/petrapp/internal/exerciseprogression -run TestCurrentSet_FirstSet -v
```

Expected: all subtests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/exerciseprogression/progression_test.go
git commit -m "test: add first-set behaviour tests for exerciseprogression"
```

---

### Task 3: TDD — signal-based weight adjustment

**Files:**
- Modify: `internal/exerciseprogression/progression_test.go`

- [ ] **Step 1: Add signal adjustment tests**

Append to `progression_test.go` (after the existing test function):

```go
func TestCurrentSet_SignalAdjustment(t *testing.T) {
	const startWeight = 100.0

	tests := []struct {
		name       string
		signal     exerciseprogression.Signal
		wantWeight float64
	}{
		{
			name:       "TooLight increases by 2.5kg",
			signal:     exerciseprogression.SignalTooLight,
			wantWeight: 102.5,
		},
		{
			name:       "TooHeavy decreases by 10 percent rounded to 0.5kg",
			signal:     exerciseprogression.SignalTooHeavy,
			wantWeight: 90.0, // 100 * 0.9 = 90.0
		},
		{
			name:       "OnTarget keeps same weight",
			signal:     exerciseprogression.SignalOnTarget,
			wantWeight: startWeight,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := exerciseprogression.New(exerciseprogression.Config{
				Type:           exerciseprogression.Hypertrophy,
				StartingWeight: startWeight,
			})
			p.RecordCompletion(exerciseprogression.SetResult{
				ActualReps: 8,
				Signal:     tt.signal,
				WeightKg:   startWeight,
			})
			got := p.CurrentSet()
			if got.WeightKg != tt.wantWeight {
				t.Errorf("WeightKg = %v, want %v", got.WeightKg, tt.wantWeight)
			}
		})
	}
}

func TestCurrentSet_TooHeavyRounding(t *testing.T) {
	// 23kg * 0.9 = 20.7kg → rounds to 20.5
	p := exerciseprogression.New(exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 23.0,
	})
	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 5,
		Signal:     exerciseprogression.SignalTooHeavy,
		WeightKg:   23.0,
	})
	got := p.CurrentSet()
	if got.WeightKg != 20.5 {
		t.Errorf("WeightKg = %v, want 20.5", got.WeightKg)
	}
}
```

- [ ] **Step 2: Run to verify new tests pass**

```bash
go test github.com/myrjola/petrapp/internal/exerciseprogression -run "TestCurrentSet_SignalAdjustment|TestCurrentSet_TooHeavyRounding" -v
```

Expected: all subtests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/exerciseprogression/progression_test.go
git commit -m "test: add signal-based weight adjustment tests for exerciseprogression"
```

---

### Task 4: TDD — user weight override propagation

**Files:**
- Modify: `internal/exerciseprogression/progression_test.go`

- [ ] **Step 1: Add override propagation test**

Append to `progression_test.go`:

```go
func TestCurrentSet_OverridePropagates(t *testing.T) {
	// Recommended set 1 = 100kg. User overrides to 95kg and signals OnTarget.
	// Set 2 recommendation must be 95kg (from actual), not 100kg.
	p := exerciseprogression.New(exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 100.0,
	})
	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 8,
		Signal:     exerciseprogression.SignalOnTarget,
		WeightKg:   95.0, // user lifted less than recommended
	})
	got := p.CurrentSet()
	if got.WeightKg != 95.0 {
		t.Errorf("WeightKg = %v, want 95.0 (override weight)", got.WeightKg)
	}
}

func TestCurrentSet_OverrideThenTooLight(t *testing.T) {
	// User overrides set 2 to 90kg and signals TooLight.
	// Set 3 must be 90 + 2.5 = 92.5kg.
	p := exerciseprogression.New(exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 100.0,
	})
	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 8,
		Signal:     exerciseprogression.SignalOnTarget,
		WeightKg:   100.0,
	})
	p.RecordCompletion(exerciseprogression.SetResult{
		ActualReps: 8,
		Signal:     exerciseprogression.SignalTooLight,
		WeightKg:   90.0, // user overrode set 2 down to 90kg
	})
	got := p.CurrentSet()
	if got.WeightKg != 92.5 {
		t.Errorf("WeightKg = %v, want 92.5", got.WeightKg)
	}
}
```

- [ ] **Step 2: Run to verify tests pass**

```bash
go test github.com/myrjola/petrapp/internal/exerciseprogression -run "TestCurrentSet_Override" -v
```

Expected: both subtests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/exerciseprogression/progression_test.go
git commit -m "test: add user weight override propagation tests for exerciseprogression"
```

---

### Task 5: TDD — `NewFromHistory`

**Files:**
- Modify: `internal/exerciseprogression/progression_test.go`

- [ ] **Step 1: Add NewFromHistory test**

Append to `progression_test.go`:

```go
func TestNewFromHistory_MatchesReplay(t *testing.T) {
	config := exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 80.0,
	}
	results := []exerciseprogression.SetResult{
		{ActualReps: 8, Signal: exerciseprogression.SignalTooLight, WeightKg: 80.0},
		{ActualReps: 8, Signal: exerciseprogression.SignalOnTarget, WeightKg: 82.5},
	}

	// Build via replay.
	replay := exerciseprogression.New(config)
	for _, r := range results {
		replay.RecordCompletion(r)
	}

	// Build via NewFromHistory.
	history := exerciseprogression.NewFromHistory(config, results)

	replayTarget := replay.CurrentSet()
	historyTarget := history.CurrentSet()

	if replayTarget != historyTarget {
		t.Errorf("NewFromHistory CurrentSet = %+v, want %+v", historyTarget, replayTarget)
	}
	if history.SetsCompleted() != replay.SetsCompleted() {
		t.Errorf("SetsCompleted = %d, want %d", history.SetsCompleted(), replay.SetsCompleted())
	}
}

func TestNewFromHistory_EmptySliceEqualsNew(t *testing.T) {
	config := exerciseprogression.Config{
		Type:           exerciseprogression.Strength,
		StartingWeight: 60.0,
	}
	fresh := exerciseprogression.New(config)
	fromEmpty := exerciseprogression.NewFromHistory(config, nil)

	if fresh.CurrentSet() != fromEmpty.CurrentSet() {
		t.Errorf("CurrentSet mismatch: fresh=%+v history=%+v", fresh.CurrentSet(), fromEmpty.CurrentSet())
	}
}
```

- [ ] **Step 2: Run to verify tests pass**

```bash
go test github.com/myrjola/petrapp/internal/exerciseprogression -run "TestNewFromHistory" -v
```

Expected: both subtests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/exerciseprogression/progression_test.go
git commit -m "test: add NewFromHistory tests for exerciseprogression"
```

---

### Task 6: TDD — `SetsCompleted`

**Files:**
- Modify: `internal/exerciseprogression/progression_test.go`

- [ ] **Step 1: Add SetsCompleted test**

Append to `progression_test.go`:

```go
func TestSetsCompleted(t *testing.T) {
	p := exerciseprogression.New(exerciseprogression.Config{
		Type:           exerciseprogression.Hypertrophy,
		StartingWeight: 60.0,
	})

	if p.SetsCompleted() != 0 {
		t.Errorf("SetsCompleted before any sets = %d, want 0", p.SetsCompleted())
	}

	p.RecordCompletion(exerciseprogression.SetResult{ActualReps: 8, Signal: exerciseprogression.SignalOnTarget, WeightKg: 60.0})
	if p.SetsCompleted() != 1 {
		t.Errorf("SetsCompleted after 1 set = %d, want 1", p.SetsCompleted())
	}

	p.RecordCompletion(exerciseprogression.SetResult{ActualReps: 8, Signal: exerciseprogression.SignalTooLight, WeightKg: 60.0})
	if p.SetsCompleted() != 2 {
		t.Errorf("SetsCompleted after 2 sets = %d, want 2", p.SetsCompleted())
	}
}
```

- [ ] **Step 2: Run to verify test passes**

```bash
go test github.com/myrjola/petrapp/internal/exerciseprogression -run "TestSetsCompleted" -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/exerciseprogression/progression_test.go
git commit -m "test: add SetsCompleted tests for exerciseprogression"
```

---

### Task 7: Full verification and final commit

**Files:** none changed

- [ ] **Step 1: Run full test suite for the package**

```bash
go test github.com/myrjola/petrapp/internal/exerciseprogression -v
```

Expected: all tests PASS, zero failures.

- [ ] **Step 2: Run lint**

```bash
make lint
```

Expected: no lint errors. If any are reported, fix them before continuing.

- [ ] **Step 3: Run project-wide tests to confirm no regressions**

```bash
make test
```

Expected: all tests PASS.

- [ ] **Step 4: Commit plan document**

```bash
git add docs/superpowers/plans/2026-04-19-exerciseprogression.md
git commit -m "docs: add exerciseprogression implementation plan"
```
