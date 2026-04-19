# exerciseprogression Package Design

**Date:** 2026-04-19
**Scope:** New `internal/exerciseprogression` package — set-to-set weight progression for a single weighted exercise execution.

## Context

Currently, all set weights and reps for a workout are resolved upfront at generation time inside `internal/workout/generator.go`. The goal is to split this into two phases:

1. **Generation time** — resolve which exercises to perform and how many sets.
2. **Execution time** — resolve weight and reps per set dynamically, evolving based on user input after each set.

This package handles phase 2 exclusively: the set-to-set progression of a single weighted exercise within one session.

## Scope Constraints

- **Within-session only.** The package has no access to cross-session history. The caller (workout service) derives the starting weight from history and passes it in.
- **Weighted exercises only** for this increment. Bodyweight rep-based progression is out of scope.
- **No I/O.** Pure logic — no database access, no HTTP, no external dependencies.

## Progression Model

The package implements **RIR-based auto-regulation** (Reps in Reserve). Rather than rep ranges (e.g. 8–12), each periodization type maps to a single fixed rep target. Weight is the only variable the package adjusts. The user signals proximity to failure after each set via a 3-tier input; the package adjusts weight for the next set accordingly.

This replaces the traditional rep-range model where hitting the top of the range is a proxy for readiness to increase. With an explicit RIR signal, the rep count stays constant and the weight floats to find the right stimulus.

## Types

```go
package exerciseprogression

type Signal int

const (
    SignalTooHeavy  Signal = iota // failed to complete target reps
    SignalOnTarget                // completed reps with ~1-2 in reserve
    SignalTooLight                // completed reps with 2+ in reserve
)

type PeriodizationType int

const (
    Strength    PeriodizationType = iota // 5 reps
    Hypertrophy                          // 8 reps
    Endurance                            // 15 reps
)

type Config struct {
    Type           PeriodizationType
    StartingWeight float64 // kg; caller-derived from history, may be user-overridden
}

type SetTarget struct {
    WeightKg   float64
    TargetReps int
}

type SetResult struct {
    ActualReps int
    Signal     Signal
    WeightKg   float64 // weight actually used; may differ from recommendation if user overrode
}
```

Rep targets per periodization type are unexported package-level constants:

```go
const (
    repsStrength    = 5
    repsHypertrophy = 8
    repsEndurance   = 15
)
```

Weight adjustment constants:

```go
const (
    weightIncrementKg     = 2.5
    weightDecrementFactor = 0.10 // 10% reduction, rounded to nearest 0.5kg
)
```

## API

```go
type Progression struct {
    config    Config
    completed []SetResult
}

// New creates a Progression for a new exercise execution.
func New(config Config) *Progression

// NewFromHistory reconstructs a Progression from sets already completed in this session.
// Enables resuming after a page reload or reconnect.
func NewFromHistory(config Config, completed []SetResult) *Progression

// CurrentSet returns the recommended target for the next set.
// First call: config.StartingWeight + periodization rep target.
// Subsequent calls: weight adjusted from last SetResult based on its Signal.
func (p *Progression) CurrentSet() SetTarget

// RecordCompletion records what actually happened and advances internal state.
func (p *Progression) RecordCompletion(result SetResult)

// SetsCompleted returns the number of sets recorded so far.
func (p *Progression) SetsCompleted() int
```

### Weight adjustment logic in `CurrentSet()`

- **First set:** return `Config.StartingWeight` and the periodization rep target.
- **Subsequent sets:** base weight is `last SetResult.WeightKg` (actual weight lifted, not the prior recommendation). Apply:
  - `SignalTooLight` → `+2.5kg`
  - `SignalTooHeavy` → `-10%`, rounded to nearest `0.5kg`
  - `SignalOnTarget` → no change

Using `SetResult.WeightKg` rather than the prior `SetTarget.WeightKg` ensures user overrides propagate naturally: if the user lifts a different weight than recommended, the next set adjusts from what was actually lifted.

## Package Structure

```
internal/exerciseprogression/
    progression.go       — all types, constants, Progression struct and methods
    progression_test.go  — table-driven tests
```

No sub-packages, no exported interfaces, no exported errors.

## Testing

Pure logic with no I/O means tests need zero infrastructure — construct a `Progression` and assert on `CurrentSet()` after sequences of `RecordCompletion` calls.

Required test cases:

| Scenario | Assertion |
|---|---|
| First set, each PeriodizationType | Correct rep target and starting weight returned |
| `SignalTooLight` after set 1 | Set 2 weight = starting + 2.5kg |
| `SignalTooHeavy` after set 1 | Set 2 weight = starting × 0.9, rounded to 0.5kg |
| `SignalOnTarget` after set 1 | Set 2 weight = set 1 actual weight |
| User overrides weight on set 2, then `SignalOnTarget` | Set 3 weight = overridden weight, not recommendation |
| `NewFromHistory` with n completed sets | `CurrentSet()` identical to replaying n `RecordCompletion` calls |
| `SetsCompleted()` increments correctly | Returns count equal to `RecordCompletion` calls made |

## Integration

The `workout` package imports `exerciseprogression`. Translation from workout domain types (periodization cycle, last session weight) to `exerciseprogression.Config` lives in the workout service layer — outside this package's scope for this increment.

Dependency direction: `internal/workout` → `internal/exerciseprogression`. No reverse dependency.
