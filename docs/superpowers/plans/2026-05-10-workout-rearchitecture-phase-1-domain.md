# Workout Rearchitecture — Phase 1: Extract `internal/domain/`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a new pure `internal/domain/` package that absorbs `internal/exerciseprogression/` and `internal/weekplanner/`, owns all canonical entities/enums/value-objects, and adds aggregate methods to `domain.Session` (Start, Complete, RecordSet, etc.) with full unit-test coverage. The new methods exist but are not yet called by service code — the existing service path stays intact through this phase.

**Architecture:** All pure code (entities, value objects, domain services, error sentinels) moves to `internal/domain/`. The two existing pure packages (`exerciseprogression`, `weekplanner`) are folded in and their type duplications collapsed to one canonical home — `domain.PeriodizationType`, `domain.Exercise`, etc. To keep the application running while migration proceeds, `internal/workout/` retains type aliases (`type Session = domain.Session`) so handlers and existing tests compile unchanged. Aggregate methods on `domain.Session` are added with full TDD coverage but are not wired into `service.go` until Phase 3 — Phase 1 ends with a green `make ci` and the existing service code path unchanged.

**Tech Stack:** Go (stdlib), SQLite (only via the existing `internal/sqlite` package, untouched in this phase), no new dependencies.

**Phase boundary:** This plan covers Phase 1 only. Phases 2–4 (extracting `internal/repository/`, extracting `internal/service/`, deleting `internal/workout/`) get separate plans after Phase 1 ships.

**Spec:** `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md`

---

## File Structure

### New files in `internal/domain/`

| File | Responsibility |
|---|---|
| `internal/domain/exercise.go` | `Exercise`, `Category`, `ExerciseType`, display methods (`FormatSetValue`, `IsTimed`, `SetValueUnit`), JSON-schema marshaling, `Resource` |
| `internal/domain/exercise_test.go` | (external `domain_test`) `Exercise.FormatSetValue`, `SetValueUnit` table tests |
| `internal/domain/set.go` | `Set`, `Signal` |
| `internal/domain/session.go` | `Session`, `ExerciseSet`, `PeriodizationType` (string-typed), aggregate methods |
| `internal/domain/session_test.go` | Aggregate-method tests (Start, Complete, RecordSet, etc.) |
| `internal/domain/preferences.go` | `Preferences` + day helpers (`Monday() bool`, `IsEmpty()`, etc.) |
| `internal/domain/feature_flag.go` | `FeatureFlag` |
| `internal/domain/muscle_group.go` | `MuscleGroupTarget`, `MuscleGroupVolume`, `MuscleGroupRegion`, `RegionFor`, `PrimarySetWeight`, `SecondarySetWeight`, `WeeklyMuscleGroupVolume` |
| `internal/domain/muscle_group_test.go` | Volume aggregation tests |
| `internal/domain/progression.go` | `Progression`, `Config`, `SetTarget`, `SetResult`, `New`, `NewFromHistory` (from `exerciseprogression/progression.go`) |
| `internal/domain/progression_timed.go` | `TimedProgression`, `TimedConfig`, `TimedSetTarget`, `TimedSetResult`, `NewTimed`, `NewTimedFromHistory` (from `exerciseprogression/timed_progression.go`) |
| `internal/domain/progression_scheme.go` | `Scheme`, `DeriveScheme` (from `exerciseprogression/scheme.go`) |
| `internal/domain/progression_convert.go` | `ConvertWeight` (Epley) (from `exerciseprogression/conversion.go`) |
| `internal/domain/progression_test.go` | Moved from `exerciseprogression/progression_test.go` |
| `internal/domain/progression_timed_test.go` | Moved from `exerciseprogression/timed_progression_test.go` |
| `internal/domain/progression_scheme_test.go` | Moved from `exerciseprogression/scheme_test.go` |
| `internal/domain/progression_convert_test.go` | Moved from `exerciseprogression/conversion_test.go` |
| `internal/domain/planner.go` | `Planner`, `NewPlanner`, `Plan` (from `weekplanner/weekplanner.go`); returns `[]Session` directly |
| `internal/domain/planner_test.go` | Moved from `weekplanner/weekplanner_internal_test.go` |
| `internal/domain/planning_sets.go` | `BuildPlannedSets`, `deriveSchemeForExercise`, `defaultTargetValue`, `defaultTimedSets` (from `workout/planning.go`) |
| `internal/domain/planning_sets_test.go` | Moved from `workout/planning_internal_test.go` |
| `internal/domain/swap.go` | `SwapSimilarityScore`, `countShared` (from `workout/swap.go`) |
| `internal/domain/swap_test.go` | Moved from `workout/swap_test.go` |
| `internal/domain/history.go` | `ExerciseSetHistory`, `LatestStartingSet` (move out of `workout/repository.go`) |
| `internal/domain/errors.go` | `ErrNotFound` (own sentinel, NOT aliased to `sql.ErrNoRows`), `ErrAlreadyStarted`, `ErrNotStarted`, `ErrSlotNotFound`, `ErrSetIndexOutOfBounds`, `ErrExerciseAlreadyInSession`, `ErrInvalidDifficultyRating` |
| `internal/domain/CLAUDE.md` | Adapted from `internal/workout/CLAUDE.md`, scoped to pure-domain rules |

### Modified files

| File | Change |
|---|---|
| `internal/workout/models.go` | Replace type definitions with `type Foo = domain.Foo` aliases; remove `exerciseJSONSchema` (moves to domain) |
| `internal/workout/repository.go` | Replace `ErrNotFound = sql.ErrNoRows` with `ErrNotFound = domain.ErrNotFound`; rename internal aggregate types to keep `sessionAggregate` etc. local to workout (Phase 2 deletes them) |
| `internal/workout/repository-sessions.go` | Translate `sql.ErrNoRows` → `domain.ErrNotFound` at every read boundary; switch `LatestStartingSet` references to `domain.LatestStartingSet`, `datedExerciseSetAggregate` to consume `domain.ExerciseSetHistory` for the public-facing return |
| `internal/workout/service.go` | Update imports: `weekplanner` → `domain`, `exerciseprogression` → `domain`. Delete `periodizationToProgression` and the parallel mapping code. Delete `WeeklyMuscleGroupVolume`, `aggregateMuscleGroupLoad`, `creditMuscleGroups` (moved to domain) and call `domain.WeeklyMuscleGroupVolume` instead. Delete `buildPlannedSets`, `deriveSchemeForExercise`, `defaultTargetValue`, `defaultTimedSets` references and switch to `domain.BuildPlannedSets`. Delete `mondayOf` if unchanged or move to domain (decision in Task 16) |
| `internal/workout/swap.go` | Delete file (now in domain) |
| `internal/workout/planning.go` | Delete file (now in domain) |
| `cmd/web/handler-workout.go`, etc. | No changes — handlers continue to import `workout` package; aliases handle the type identity |
| `internal/workout/CLAUDE.md` | Add a top-of-file note: "Pure domain types now live in `internal/domain/`. The aliases here are temporary scaffolding for the multi-phase rearchitecture; new domain logic should be added to `internal/domain/`." |

### Deleted files

| File | Reason |
|---|---|
| `internal/exerciseprogression/*.go` | Code moved to `internal/domain/`; package no longer needed |
| `internal/weekplanner/*.go` | Code moved to `internal/domain/`; package no longer needed |
| `internal/workout/swap.go` | Moved to `internal/domain/swap.go` |
| `internal/workout/planning.go` | Moved to `internal/domain/planning_sets.go` |

### Untouched

| Path | Reason |
|---|---|
| `internal/sqlite/` | No schema change |
| `internal/repository/` | Doesn't exist yet — Phase 2 |
| `internal/service/` | Doesn't exist yet — Phase 3 |
| `cmd/web/*.go` | Handlers continue to import `workout` package |
| `ui/templates/` | No template changes |

---

## Migration sequencing rationale

Tasks are ordered so the working tree compiles and `make test` passes after every task — no broken intermediate states.

1. **Tasks 1–4: Foundational types in domain** (enums, Exercise, Set, Session, Preferences, FeatureFlag, MuscleGroupTarget, MuscleGroupVolume, errors, history). Workout package re-exports via aliases. Tests still pass through aliases.
2. **Tasks 5–6: Pure helpers** (SwapSimilarityScore, RegionFor + region tests). These have no upstream deps.
3. **Tasks 7–10: Subsume `exerciseprogression`** (progression, timed_progression, scheme, conversion). `internal/workout/service.go` switches to `domain.*` references.
4. **Tasks 11–12: Subsume `weekplanner`** + move `BuildPlannedSets`. `Planner.Plan` returns `[]domain.Session`. Service updates accordingly.
5. **Task 13: Move `WeeklyMuscleGroupVolume`** out of service.
6. **Tasks 14–22: Add aggregate methods on `domain.Session`** with full TDD. Each is a self-contained TDD cycle. Service code does NOT yet call these — that is Phase 3.
7. **Task 23: Delete the now-empty `exerciseprogression` and `weekplanner` packages.**
8. **Task 24: Add `internal/domain/CLAUDE.md`** and update workout CLAUDE.md.
9. **Task 25: Final `make ci` verification + commit.**

---

## Tasks

---

### Task 1: Scaffold `internal/domain/` with the core enums

**Files:**
- Create: `internal/domain/exercise.go` (initial — enums + Resource only)
- Create: `internal/domain/set.go` (Signal enum only)
- Create: `internal/domain/session.go` (PeriodizationType only)

**Goal:** Land the canonical enums in domain so subsequent tasks have somewhere to point at. No behavior change; existing workout types still own all data structures.

- [ ] **Step 1: Create `internal/domain/exercise.go` with Category and ExerciseType**

```go
// Package domain holds the pure entities, value objects, aggregate methods,
// and domain services for the workout bounded context. It depends on the Go
// standard library only — no SQL, no HTTP, no logger, no third-party clients.
//
// Domain code is the canonical home for business rules. Other layers
// (repository, service, handlers) build on top of these types.
package domain

// Category is the workout focus for a session — the muscle-group split a day
// targets.
type Category string

const (
	CategoryFullBody Category = "full_body"
	CategoryUpper    Category = "upper"
	CategoryLower    Category = "lower"
)

// ExerciseType distinguishes the load model used by an exercise.
type ExerciseType string

const (
	ExerciseTypeWeighted   ExerciseType = "weighted"
	ExerciseTypeBodyweight ExerciseType = "bodyweight"
	ExerciseTypeAssisted   ExerciseType = "assisted"
	ExerciseTypeTime       ExerciseType = "time_based"
)

// Resource is a learning link associated with an exercise (video, article).
type Resource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}
```

- [ ] **Step 2: Create `internal/domain/set.go` with the Signal enum**

```go
package domain

// Signal is the user's perceived effort after completing a set.
type Signal string

const (
	SignalTooHeavy Signal = "too_heavy"
	SignalOnTarget Signal = "on_target"
	SignalTooLight Signal = "too_light"
)
```

- [ ] **Step 3: Create `internal/domain/session.go` with PeriodizationType**

```go
package domain

// PeriodizationType is the rep-target style for a session. The two values
// alternate week-to-week (see Planner.firstSessionPeriodizationType) and
// determine the rep target via DeriveScheme.
type PeriodizationType string

const (
	PeriodizationStrength    PeriodizationType = "strength"
	PeriodizationHypertrophy PeriodizationType = "hypertrophy"
)
```

- [ ] **Step 4: Verify the package compiles**

Run: `go build ./internal/domain/...`
Expected: no output (success).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/exercise.go internal/domain/set.go internal/domain/session.go
git commit -m "Scaffold internal/domain package with canonical enums"
```

---

### Task 2: Move `Exercise`, `Set`, `ExerciseSet`, `Session`, `Preferences`, `FeatureFlag`, `MuscleGroupTarget`, `MuscleGroupVolume`, `MuscleGroupRegion`/`RegionFor` from `internal/workout/models.go` to `internal/domain/`. Add aliases in workout.

**Files:**
- Modify: `internal/domain/exercise.go` (add Exercise struct + display methods + exerciseJSONSchema)
- Modify: `internal/domain/set.go` (add Set struct)
- Modify: `internal/domain/session.go` (add Session, ExerciseSet, ExerciseProgress, ExerciseProgressEntry)
- Create: `internal/domain/preferences.go`
- Create: `internal/domain/feature_flag.go`
- Create: `internal/domain/muscle_group.go` (MuscleGroupTarget, MuscleGroupVolume, MuscleGroupRegion, RegionFor)
- Modify: `internal/workout/models.go` (replace definitions with aliases)

- [ ] **Step 1: Append `Exercise` struct + display methods to `internal/domain/exercise.go`**

Copy the `Exercise` struct (lines 46-58), `IsTimed` method (line 61), `FormatSetValue` method (lines 67-72), and `SetValueUnit` method (lines 76-81) verbatim from `internal/workout/models.go`. Replace the package declaration `package workout` with `package domain`.

**Do NOT move `exerciseJSONSchema` (lines 89-156).** It is unexported and is consumed only by `generator-exercise.go:101`, which lives in `internal/workout/` and stays there until Phase 3 (when it moves into `internal/service/exercise_generation.go`). Leave `exerciseJSONSchema` and its `MarshalJSON` method in `internal/workout/models.go` for now; Task 2 Step 7 keeps it alongside the aliases.

After the copy, the domain file's exports are: `Category`, `ExerciseType`, `CategoryFullBody`/`Upper`/`Lower`, `ExerciseTypeWeighted`/`Bodyweight`/`Assisted`/`Time`, `Exercise`, `Resource`, plus the methods `Exercise.IsTimed`, `Exercise.FormatSetValue`, `Exercise.SetValueUnit`.

- [ ] **Step 2: Append `Set` struct to `internal/domain/set.go`**

Copy the `Set` struct verbatim from `internal/workout/models.go:158-165`. The file now exports `Set` + `Signal` (and constants).

- [ ] **Step 3: Append `Session`, `ExerciseSet`, `ExerciseProgress`, `ExerciseProgressEntry` to `internal/domain/session.go`**

Copy `ExerciseSet` (lines 167-175), `ExerciseProgressEntry` (177-181), `ExerciseProgress` (183-187), `Session` (189-197) verbatim from `internal/workout/models.go`. The `Exercise` and `Set` references resolve to the domain types because all live in the same package now.

- [ ] **Step 4: Create `internal/domain/preferences.go`**

```go
package domain

// Preferences stores how long a user wants to work out each day of the week.
// A value of 0 means rest day; any positive integer means workout day with
// that duration in minutes.
type Preferences struct {
	MondayMinutes    int
	TuesdayMinutes   int
	WednesdayMinutes int
	ThursdayMinutes  int
	FridayMinutes    int
	SaturdayMinutes  int
	SundayMinutes    int
}

func (p Preferences) Monday() bool    { return p.MondayMinutes > 0 }
func (p Preferences) Tuesday() bool   { return p.TuesdayMinutes > 0 }
func (p Preferences) Wednesday() bool { return p.WednesdayMinutes > 0 }
func (p Preferences) Thursday() bool  { return p.ThursdayMinutes > 0 }
func (p Preferences) Friday() bool    { return p.FridayMinutes > 0 }
func (p Preferences) Saturday() bool  { return p.SaturdayMinutes > 0 }
func (p Preferences) Sunday() bool    { return p.SundayMinutes > 0 }

// IsEmpty reports whether no workout days are scheduled.
func (p Preferences) IsEmpty() bool {
	return p.MondayMinutes == 0 && p.TuesdayMinutes == 0 && p.WednesdayMinutes == 0 &&
		p.ThursdayMinutes == 0 && p.FridayMinutes == 0 && p.SaturdayMinutes == 0 &&
		p.SundayMinutes == 0
}
```

- [ ] **Step 5: Create `internal/domain/feature_flag.go`**

```go
package domain

// FeatureFlag toggles application features at runtime.
type FeatureFlag struct {
	Name    string
	Enabled bool
}
```

- [ ] **Step 6: Create `internal/domain/muscle_group.go`**

Copy verbatim from `internal/workout/models.go:233-280` (the muscle-group block including `MuscleGroupTarget`, `MuscleGroupVolume`, `MuscleGroupRegion` enum + constants, and `RegionFor`). Change `package workout` → `package domain`. The `PrimarySetWeight` / `SecondarySetWeight` constants currently live in `service.go:810-813` — copy those into this file too.

```go
// PrimarySetWeight and SecondarySetWeight are the per-set contributions to a
// muscle group's weekly load. The split reflects that secondary engagement
// receives meaningfully less stimulus than primary engagement.
const (
	PrimarySetWeight   = 1.0
	SecondarySetWeight = 0.5
)
```

- [ ] **Step 7: Replace contents of `internal/workout/models.go` with aliases (keep `exerciseJSONSchema`)**

The `exerciseJSONSchema` type and its `MarshalJSON` method (lines 89-156 of the original file) MUST remain in this file because `generator-exercise.go` imports it as an unexported type within the same package. Keep those lines verbatim and add the alias block below them. Everything else in the original models.go is replaced by aliases.

```go
package workout

import (
	"encoding/json"
	"fmt"

	"github.com/myrjola/petrapp/internal/domain"
)

// Backward-compat aliases. The canonical types live in internal/domain;
// these aliases let handlers and existing tests continue to import "workout"
// while the multi-phase rearchitecture is in flight. They will be removed in
// Phase 4.

type Category = domain.Category

const (
	CategoryFullBody = domain.CategoryFullBody
	CategoryUpper    = domain.CategoryUpper
	CategoryLower    = domain.CategoryLower
)

type ExerciseType = domain.ExerciseType

const (
	ExerciseTypeWeighted   = domain.ExerciseTypeWeighted
	ExerciseTypeBodyweight = domain.ExerciseTypeBodyweight
	ExerciseTypeAssisted   = domain.ExerciseTypeAssisted
	ExerciseTypeTime       = domain.ExerciseTypeTime
)

type PeriodizationType = domain.PeriodizationType

const (
	PeriodizationStrength    = domain.PeriodizationStrength
	PeriodizationHypertrophy = domain.PeriodizationHypertrophy
)

type Signal = domain.Signal

const (
	SignalTooHeavy = domain.SignalTooHeavy
	SignalOnTarget = domain.SignalOnTarget
	SignalTooLight = domain.SignalTooLight
)

type (
	Exercise              = domain.Exercise
	Resource              = domain.Resource
	Set                   = domain.Set
	ExerciseSet           = domain.ExerciseSet
	Session               = domain.Session
	ExerciseProgress      = domain.ExerciseProgress
	ExerciseProgressEntry = domain.ExerciseProgressEntry
	Preferences           = domain.Preferences
	FeatureFlag           = domain.FeatureFlag
	MuscleGroupTarget     = domain.MuscleGroupTarget
	MuscleGroupVolume     = domain.MuscleGroupVolume
	MuscleGroupRegion     = domain.MuscleGroupRegion
)

const (
	RegionUpperPush = domain.RegionUpperPush
	RegionUpperPull = domain.RegionUpperPull
	RegionLegs      = domain.RegionLegs
	RegionCore      = domain.RegionCore
	RegionOther     = domain.RegionOther

	PrimarySetWeight   = domain.PrimarySetWeight
	SecondarySetWeight = domain.SecondarySetWeight
)

func RegionFor(name string) MuscleGroupRegion { return domain.RegionFor(name) }

// exerciseJSONSchema and its MarshalJSON method stay here (paste verbatim
// below) — generator-exercise.go consumes the unexported type.
```

After the alias block, paste the original `exerciseJSONSchema` type and its
`MarshalJSON` method (the ~70 lines from the original `models.go:89-156`)
unchanged. The `encoding/json` and `fmt` imports above support that code.

- [ ] **Step 8: Move `internal/workout/models_test.go` to `internal/domain/exercise_test.go`**

Rename the file. Change `package workout_test` → `package domain_test`. Change every `workout.` reference to `domain.`.

```bash
git mv internal/workout/models_test.go internal/domain/exercise_test.go
sed -i 's/package workout_test/package domain_test/; s|github.com/myrjola/petrapp/internal/workout|github.com/myrjola/petrapp/internal/domain|; s/workout\./domain./g' internal/domain/exercise_test.go
```

- [ ] **Step 9: Verify build + tests**

```bash
go build ./...
go test ./internal/domain/... ./internal/workout/...
```
Expected: all green.

- [ ] **Step 10: Commit**

```bash
git add -A
git commit -m "Move pure entities and value objects to internal/domain"
```

---

### Task 3: Move sentinel errors. Define `ErrNotFound` as own sentinel; translate at boundary.

**Files:**
- Create: `internal/domain/errors.go`
- Modify: `internal/workout/repository.go` (alias `ErrNotFound = domain.ErrNotFound`)
- Modify: `internal/workout/repository-sessions.go` and other repo files (translate `sql.ErrNoRows` → `domain.ErrNotFound`)

- [ ] **Step 1: Create `internal/domain/errors.go` with all sentinels**

```go
package domain

import "errors"

// ErrNotFound is returned by repositories when a requested record does not
// exist. It is intentionally NOT aliased to sql.ErrNoRows — repositories
// translate the SQL error at their boundary so the domain stays free of
// persistence concerns.
var ErrNotFound = errors.New("not found")

// Aggregate-method sentinels. Each is returned by a Session method when an
// invariant is violated; callers use errors.Is to branch.
var (
	ErrAlreadyStarted           = errors.New("session already started")
	ErrNotStarted               = errors.New("session not started")
	ErrSlotNotFound             = errors.New("workout exercise slot not found")
	ErrSetIndexOutOfBounds      = errors.New("set index out of bounds")
	ErrExerciseAlreadyInSession = errors.New("exercise already in session")
	ErrInvalidDifficultyRating  = errors.New("difficulty rating must be 1-5")
)
```

- [ ] **Step 2: Find every `sql.ErrNoRows` translation point in `internal/workout/`**

Run: `grep -n "sql.ErrNoRows\|ErrNotFound" internal/workout/`
Note each occurrence — there will be returns from `Get` methods on the SQLite repos (sessions, exercises, preferences, feature_flags, muscle_targets).

- [ ] **Step 3: Update `internal/workout/repository.go`**

Replace:
```go
var ErrNotFound = sql.ErrNoRows
```
with:
```go
// ErrNotFound aliases the canonical domain sentinel for backward
// compatibility — handlers and tests still import workout.ErrNotFound.
var ErrNotFound = domain.ErrNotFound
```

Add `"github.com/myrjola/petrapp/internal/domain"` to the import block; remove `"database/sql"` if it becomes unused (it likely won't, since the repo still uses other sql types).

- [ ] **Step 4: Translate `sql.ErrNoRows` → `domain.ErrNotFound` at every read boundary**

For each repo file (`repository-sessions.go`, `repository-exercises.go`, `repository-preferences.go`, `repository-featureflags.go`, `repository-muscle-targets.go`), find `Get`-style methods that return `sql.ErrNoRows`. Wrap them:

Example pattern:
```go
// Before
err := tx.QueryRowContext(...).Scan(...)
if err != nil {
    return Session{}, fmt.Errorf("query session: %w", err)
}

// After
err := tx.QueryRowContext(...).Scan(...)
if errors.Is(err, sql.ErrNoRows) {
    return Session{}, domain.ErrNotFound
}
if err != nil {
    return Session{}, fmt.Errorf("query session: %w", err)
}
```

Add `"github.com/myrjola/petrapp/internal/domain"` import as needed.

- [ ] **Step 5: Verify all callers still see `ErrNotFound` semantics**

Run: `go test ./...`
Expected: all tests still pass. Tests using `errors.Is(err, workout.ErrNotFound)` continue to work because the alias preserves identity.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "Move ErrNotFound to internal/domain; translate sql.ErrNoRows at repo boundary"
```

---

### Task 4: Move `ExerciseSetHistory` and `LatestStartingSet` types to domain

**Context:** Today `LatestStartingSet` lives in `internal/workout/repository.go:55-60`. `datedExerciseSetAggregate` is the lean wire format used by `ListSetsForExerciseSince`; the public-facing `ExerciseProgress`-related domain consumes its data. This task introduces a cleaner domain type for the read result.

**Files:**
- Create: `internal/domain/history.go`
- Modify: `internal/workout/repository.go` (alias `LatestStartingSet = domain.LatestStartingSet`)

- [ ] **Step 1: Create `internal/domain/history.go`**

```go
package domain

import "time"

// LatestStartingSet captures the weight of the most recent completed first
// set for an exercise along with the periodization type of the session it
// came from. PeriodizationType is empty when no history exists.
type LatestStartingSet struct {
	WeightKg          float64
	PeriodizationType PeriodizationType
}

// ExerciseSetHistory bundles a date with the sets recorded for one exercise
// on that date. Returned by repositories from history-style queries
// (e.g. ListSetsForExerciseSince).
type ExerciseSetHistory struct {
	Date time.Time
	Sets []Set
}
```

- [ ] **Step 2: Replace the workout-package version with an alias**

In `internal/workout/repository.go`, replace the `LatestStartingSet` struct definition (lines ~54-60) with:
```go
type LatestStartingSet = domain.LatestStartingSet
```

`datedExerciseSetAggregate` stays in workout/repository.go as a private wire-format type — it gets deleted in Phase 2 when the repository moves.

- [ ] **Step 3: Verify build**

```bash
go build ./...
go test ./...
```
Expected: green.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/history.go internal/workout/repository.go
git commit -m "Move LatestStartingSet and ExerciseSetHistory to internal/domain"
```

---

### Task 5: Move `SwapSimilarityScore` to domain

**Files:**
- Create: `internal/domain/swap.go` (copy from `internal/workout/swap.go`)
- Move: `internal/workout/swap_test.go` → `internal/domain/swap_test.go`
- Delete: `internal/workout/swap.go`
- Modify: `internal/workout/models.go` (re-export `SwapSimilarityScore` via alias)

- [ ] **Step 1: Create `internal/domain/swap.go`**

Copy the entire contents of `internal/workout/swap.go` to `internal/domain/swap.go`. Change `package workout` → `package domain`. No other changes needed — `Exercise` resolves to the domain type.

- [ ] **Step 2: Move and rewrite the test file**

```bash
git mv internal/workout/swap_test.go internal/domain/swap_test.go
sed -i 's/package workout_test/package domain_test/; s|github.com/myrjola/petrapp/internal/workout|github.com/myrjola/petrapp/internal/domain|; s/workout\./domain./g' internal/domain/swap_test.go
```

Inspect the result and verify the test compiles. If it imports `workout` for any other reason, fix manually.

- [ ] **Step 3: Delete `internal/workout/swap.go`**

```bash
git rm internal/workout/swap.go
```

- [ ] **Step 4: Add the function alias to `internal/workout/models.go`**

Append to the bottom of `internal/workout/models.go`:
```go
// SwapSimilarityScore is re-exported from internal/domain. Handlers call
// workout.SwapSimilarityScore today; that import path keeps working through
// this phase.
func SwapSimilarityScore(current, candidate Exercise) int {
	return domain.SwapSimilarityScore(current, candidate)
}
```

- [ ] **Step 5: Verify**

```bash
go build ./...
go test ./internal/domain/... ./internal/workout/... ./cmd/web/...
```
Expected: green.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "Move SwapSimilarityScore to internal/domain"
```

---

### Task 6: Move `exerciseprogression` package into domain (4 files)

**Context:** `internal/exerciseprogression/` is already pure. The move collapses its `PeriodizationType int` enum into `domain.PeriodizationType string` (the canonical form). `Endurance` is dropped (dead code per spec non-goals). `Signal` collapses into `domain.Signal`.

**Files:**
- Create: `internal/domain/progression.go` (from `exerciseprogression/progression.go`, adapted)
- Create: `internal/domain/progression_timed.go` (from `exerciseprogression/timed_progression.go`, adapted)
- Create: `internal/domain/progression_scheme.go` (from `exerciseprogression/scheme.go`, adapted)
- Create: `internal/domain/progression_convert.go` (from `exerciseprogression/conversion.go`, adapted)
- Move tests: `progression_test.go`, `timed_progression_test.go`, `scheme_test.go`, `conversion_test.go`
- Modify: `internal/workout/service.go` to import `domain` instead of `exerciseprogression`

- [ ] **Step 1: Create `internal/domain/progression_scheme.go`**

Copy `internal/exerciseprogression/scheme.go` to the new path. Change `package exerciseprogression` → `package domain`. Then change the `DeriveScheme` signature and switch to drop the `Endurance` case:

```go
// DeriveScheme returns the prescription for one exercise given its rep window
// and the session periodization. Pure: same inputs → same output, no DB, no
// clock.
//
// Reps:
//
//	Strength    → repMin (low end of the window)
//	Hypertrophy → repMax (high end of the window)
//
// Sets and rest are derived from the resulting rep target:
//
//	reps ≤ 5  → 4 sets, 180s rest
//	reps 6-10 → 3 sets, 150s rest
//	reps ≥ 11 → 3 sets, 90s rest
func DeriveScheme(repMin, repMax int, p PeriodizationType) Scheme {
	var reps int
	switch p {
	case PeriodizationStrength:
		reps = repMin
	case PeriodizationHypertrophy:
		reps = repMax
	default:
		panic(fmt.Sprintf("domain: unknown PeriodizationType %q", p))
	}
	// rest of body unchanged
}
```

The constants (`repBoundaryLowToMid` etc.) and the `Scheme` struct stay verbatim.

- [ ] **Step 2: Create `internal/domain/progression.go`**

Copy `internal/exerciseprogression/progression.go`. Change `package exerciseprogression` → `package domain`. Delete the local `Signal` type definition (lines 11-19 of source) — `domain.Signal` already exists. Delete the local `PeriodizationType` definition — already exists. Delete the `Endurance` constant.

In `adjustedWeight`, change `case SignalUnknown` to remove that branch (domain.Signal has no Unknown sentinel — it's a string enum without a zero value beyond `""`); replace with:
```go
default:
    panic(fmt.Sprintf("domain: unknown Signal %q", last.Signal))
```

The rest of `Progression`, `Config`, `SetTarget`, `SetResult`, `New`, `NewFromHistory`, `CurrentSet`, `RecordCompletion`, `SetsCompleted`, `adjustedWeight`, `incrementFor`, `snapWeight` is unchanged. The `Config.Type` field type changes from `PeriodizationType` (which was `int`) to `domain.PeriodizationType` (which is `string`) — at the new package, both names resolve to the same thing.

- [ ] **Step 3: Create `internal/domain/progression_timed.go`**

Copy `internal/exerciseprogression/timed_progression.go`. Change `package exerciseprogression` → `package domain`. Replace `case SignalUnknown:` panic with a `default:` panic identical to Step 2. Otherwise unchanged.

- [ ] **Step 4: Create `internal/domain/progression_convert.go`**

Copy `internal/exerciseprogression/conversion.go` verbatim except for the package declaration. The function calls `snapWeight` which is package-private in `progression.go` — both files now live in `domain`, so the call resolves naturally.

- [ ] **Step 5: Move the test files into `internal/domain/`**

```bash
git mv internal/exerciseprogression/progression_test.go internal/domain/progression_test.go
git mv internal/exerciseprogression/timed_progression_test.go internal/domain/progression_timed_test.go
git mv internal/exerciseprogression/scheme_test.go internal/domain/progression_scheme_test.go
git mv internal/exerciseprogression/conversion_test.go internal/domain/progression_convert_test.go
```

For each moved test file: change `package exerciseprogression` → `package domain` (or `package exerciseprogression_test` → `package domain_test`). Replace every reference to local types and constants:

- `exerciseprogression.Signal*` → `domain.Signal*` (and remove any branch that uses `SignalUnknown` since the new enum doesn't have that value).
- `exerciseprogression.Strength` → `domain.PeriodizationStrength`.
- `exerciseprogression.Hypertrophy` → `domain.PeriodizationHypertrophy`.
- `exerciseprogression.Progression` → `domain.Progression`.
- `exerciseprogression.New`, `NewFromHistory`, `NewTimed`, `NewTimedFromHistory` → `domain.*`.
- `exerciseprogression.Config`, `SetTarget`, `SetResult`, `TimedConfig`, `TimedSetTarget`, `TimedSetResult`, `Scheme` → `domain.*`.
- `exerciseprogression.DeriveScheme`, `ConvertWeight` → `domain.*`.

Then **delete every reference to the dropped `Endurance` value**. In `progression_test.go` specifically:

- Delete the entire `"endurance returns 15 reps"` subtest case (lines 37-45 of the source file).
- In `TestExhaustivePeriodizationCoverage` (around line 382), remove the `exerciseprogression.Endurance` entry from the `all` slice. The test still validates exhaustive coverage of the remaining two values.

Run the tests after each test file is updated to catch any references missed.

- [ ] **Step 6: Update `internal/workout/service.go` imports and references**

Replace `"github.com/myrjola/petrapp/internal/exerciseprogression"` with `"github.com/myrjola/petrapp/internal/domain"`. Replace every `exerciseprogression.` prefix with `domain.`.

Delete the `periodizationToProgression` function (`service.go:657-666`) — no longer needed since both periodization types are now the same.

Update `Service.GetStartingWeight` (`service.go:589-621`):
- Change `targetType PeriodizationType` to `targetType domain.PeriodizationType` (or keep — they're aliases now).
- Replace `periodizationToProgression(prev.PeriodizationType)` and `periodizationToProgression(targetType)` with `prev.PeriodizationType` and `targetType` directly.
- Change `exerciseprogression.DeriveScheme(...)` to `domain.DeriveScheme(...)`.
- Change `exerciseprogression.ConvertWeight(...)` to `domain.ConvertWeight(...)`.

Update `Service.BuildProgression` and `Service.BuildTimedProgression` (`service.go:670-785`):
- Replace `exerciseprogression.NewFromHistory` → `domain.NewFromHistory`.
- Replace `exerciseprogression.NewTimedFromHistory` → `domain.NewTimedFromHistory`.
- Replace `exerciseprogression.Config{...}` → `domain.Config{...}`. Change the `Type` field assignment from `epType` (which was `exerciseprogression.PeriodizationType`) to `sess.PeriodizationType` directly.
- Delete the `epType := periodizationToProgression(...)` line — direct field use replaces it.
- The signal mapping switch (`case SignalTooHeavy: sig = exerciseprogression.SignalTooHeavy` etc.) collapses: `domain.Signal` is the only Signal type now. Replace the entire switch block with:
  ```go
  completed = append(completed, domain.SetResult{
      ActualReps: *set.CompletedValue,
      Signal:     *set.Signal,
      WeightKg:   kg,
  })
  ```
  (And similarly for the timed version with `domain.TimedSetResult`.)

- [ ] **Step 7: Verify build and run all tests**

```bash
go build ./...
go test ./internal/domain/... ./internal/workout/... ./cmd/web/...
```
Expected: green. The exerciseprogression package still exists but is now empty — that's fine until Task 23.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "Move exerciseprogression code into internal/domain; collapse PeriodizationType"
```

---

### Task 7: Move `BuildPlannedSets` and `deriveSchemeForExercise` to domain

**Files:**
- Create: `internal/domain/planning_sets.go`
- Move: `internal/workout/planning_internal_test.go` → `internal/domain/planning_sets_test.go`
- Delete: `internal/workout/planning.go`
- Modify: `internal/workout/service.go` to call `domain.BuildPlannedSets`

- [ ] **Step 1: Create `internal/domain/planning_sets.go`**

Copy the contents of `internal/workout/planning.go` (the `buildPlannedSets` function) and the `deriveSchemeForExercise`, `defaultTargetValue`, `defaultTimedSets` definitions from `internal/workout/service.go:1051-1076` into the new file.

```go
package domain

// defaultTargetValue is the fallback target value (reps) when no history is
// available.
const defaultTargetValue = 8

// defaultTimedSets is the fixed set count for time-based exercises, matching
// the planner's timeBasedSets constant.
const defaultTimedSets = 3

// deriveSchemeForExercise returns the per-set target reps and total set
// count for an exercise within a session of the given periodization. For
// time-based exercises, uses DefaultStartingSeconds and a fixed set count of
// defaultTimedSets. For rep-based exercises, returns DeriveScheme values.
func deriveSchemeForExercise(ex Exercise, pt PeriodizationType) (int, int) {
	if ex.IsTimed() {
		if ex.DefaultStartingSeconds != nil {
			return *ex.DefaultStartingSeconds, defaultTimedSets
		}
		return defaultTargetValue, defaultTimedSets
	}
	if ex.RepMin == nil || ex.RepMax == nil {
		return defaultTargetValue, defaultTimedSets
	}
	scheme := DeriveScheme(*ex.RepMin, *ex.RepMax, pt)
	return scheme.TargetReps, scheme.TargetSets
}

// BuildPlannedSets returns the persisted set slice for an exercise prescribed
// in a session of the given periodization. Single source of truth for "what
// target value and set count does this exercise get when first added to a
// session".
//
// WeightKg is left nil. Callers that need to seed a starting weight (e.g.
// AddExercise / SwapExercise paths in service) post-process the slice.
func BuildPlannedSets(exercise Exercise, periodization PeriodizationType) []Set {
	targetValue, n := deriveSchemeForExercise(exercise, periodization)
	sets := make([]Set, n)
	for i := range sets {
		sets[i] = Set{ //nolint:exhaustruct // WeightKg, CompletedValue, CompletedAt, Signal start nil.
			TargetValue: targetValue,
		}
	}
	return sets
}
```

- [ ] **Step 2: Move and rewrite the test file**

```bash
git mv internal/workout/planning_internal_test.go internal/domain/planning_sets_test.go
```

Open the file. The package was `package workout` (internal test). Change to `package domain` (internal test). Replace any `buildPlannedSets` calls with `BuildPlannedSets`. Replace `Periodization*`/`Exercise`/`Set`/`ExerciseType*` references — they all already match domain spellings.

- [ ] **Step 3: Delete `internal/workout/planning.go`**

```bash
git rm internal/workout/planning.go
```

- [ ] **Step 4: Update `internal/workout/service.go`**

Find every call to `buildPlannedSets` in `service.go` and replace with `domain.BuildPlannedSets`. Find references to `deriveSchemeForExercise`, `defaultTargetValue`, `defaultTimedSets` — these are unexported in domain now, so service can no longer call them directly. The good news: service should never need to call them; `BuildPlannedSets` is the single funnel. Audit the file; if any call exists, route it through `BuildPlannedSets` instead.

- [ ] **Step 5: Verify**

```bash
go build ./...
go test ./...
```
Expected: green.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "Move BuildPlannedSets to internal/domain"
```

---

### Task 8: Move `weekplanner` package into domain as `Planner`; collapse `weekplanner.Exercise` into `domain.Exercise`

**Context:** `weekplanner.Plan` returns `[]PlannedSession` today, with `PlannedSession`/`PlannedExerciseSet`/`PlannedSet` parallel types. The spec collapses these into `domain.Session`/`domain.ExerciseSet`/`domain.Set` with weights left nil. The planner now returns aggregates ready to persist.

`weekplanner.Exercise` is a stripped-down struct (no Name, no Description). The planner now consumes the rich `domain.Exercise` directly — the planner just doesn't read fields it doesn't need.

**Files:**
- Create: `internal/domain/planner.go`
- Move: `internal/weekplanner/weekplanner_internal_test.go` → `internal/domain/planner_test.go`
- Modify: `internal/workout/service.go` to consume the new return type

- [ ] **Step 1: Create `internal/domain/planner.go`**

Copy `internal/weekplanner/weekplanner.go` to the new path. Change `package weekplanner` → `package domain`. Then make these substitutions:

1. **Delete** the duplicate enum definitions: `Category`, `ExerciseType`, `PeriodizationType` and their constants (lines 13-38 of source). They already exist in domain.
2. **Delete** the local `Exercise` struct (lines 113-124). The planner now consumes `domain.Exercise` directly. All references in this file to `Exercise` resolve to `domain.Exercise`.
3. **Delete** the `MuscleGroupTarget` struct (lines 126-130) — already in domain. Update planner code that used `weekplanner.MuscleGroupTarget` field access (`Name`, `WeeklySetTarget`) to use `domain.MuscleGroupTarget` (which has `MuscleGroupName`, `WeeklySetTarget`). Adjust the field name accordingly: `target.Name` → `target.MuscleGroupName`.
4. **Delete** the `PlannedSession`, `PlannedExerciseSet`, `PlannedSet` types (lines 132-151). Replace usages:
   - `PlannedSession{Date, Category, PeriodizationType, ExerciseSets}` → use a local helper type for now and convert to `domain.Session` at the API boundary, OR rewrite `Plan` to build `[]domain.Session` directly. Prefer the latter.
   - `PlannedExerciseSet{ExerciseID, Sets}` → use `domain.ExerciseSet{ID: 0, Exercise: ..., Sets: ...}` — but `Exercise` requires a full struct. Build a lookup from `[]domain.Exercise` (the planner already has the slice) and embed.
   - `PlannedSet{TargetValue, RestSeconds}` → use `domain.Set{TargetValue: ..., WeightKg: nil, CompletedValue: nil, CompletedAt: nil, Signal: nil}`. **Drop** `RestSeconds` — `domain.Set` does not carry rest. (Rest seconds are derived display-side from the periodization scheme; persisting them per-set would be redundant.)
5. **Rename** `WeeklyPlanner` → `Planner` and `NewWeeklyPlanner` → `NewPlanner`. The receiver method `wp` stays.
6. **Replace** `toProgressionPeriodization` (the int↔string converter) with direct passthrough — both types are now `domain.PeriodizationType`. Delete the function.
7. **Update** `Plan(startingDate)` return type to `([]Session, error)`. The function builds `domain.Session` aggregates with `Date`, `PeriodizationType`, `ExerciseSets` populated; `StartedAt`, `CompletedAt`, `DifficultyRating` zero. Each `ExerciseSet` has `ID: 0`, `Exercise: <full struct from planner.Exercises>`, `Sets: BuildPlannedSets(ex, pt)`.
8. **Replace** `buildPlannedExerciseSet(ex, pt)` body with `BuildPlannedSets` — same logic, single source of truth. The function should return a `domain.ExerciseSet` now, not the deleted `PlannedExerciseSet`:
   ```go
   func buildPlannedExerciseSet(ex Exercise, pt PeriodizationType) ExerciseSet {
       return ExerciseSet{ //nolint:exhaustruct // ID auto-assigned at insert; WarmupCompletedAt nil.
           Exercise: ex,
           Sets:     BuildPlannedSets(ex, pt),
       }
   }
   ```

After the rewrite, the file's exports are: `Planner`, `NewPlanner`, `Planner.Plan`. Internal helpers (`determineCategory`, `allocateMuscleGroups`, `selectExercisesForDay*`, etc.) stay unexported.

- [ ] **Step 2: Move the test file**

```bash
git mv internal/weekplanner/weekplanner_internal_test.go internal/domain/planner_test.go
```

Change `package weekplanner` → `package domain`. Replace:
- `WeeklyPlanner` → `Planner`
- `NewWeeklyPlanner` → `NewPlanner`
- `weekplanner.Exercise{...}` → `Exercise{...}` (with the additional rich fields the planner doesn't read — pass zero values)
- `weekplanner.MuscleGroupTarget{Name: "X", WeeklySetTarget: N}` → `MuscleGroupTarget{MuscleGroupName: "X", WeeklySetTarget: N}`
- Any test that walks `PlannedSession`/`PlannedExerciseSet`/`PlannedSet` fields — rewrite to walk `Session`/`ExerciseSet`/`Set` fields. `RestSeconds` checks must be removed (the field no longer exists). If any test hard-coded `RestSeconds` expectations, replace with a check that `len(Sets) == expected` or skip — the rest-seconds rule is now implicit in `DeriveScheme` and doesn't need per-test verification (it's covered by `progression_scheme_test.go`).

- [ ] **Step 3: Update `internal/workout/service.go::generateWeeklyPlan`**

The `generateWeeklyPlan` function (`service.go:136-221`) currently:
1. Builds `weekplanner.Preferences`, `weekplanner.Exercise[]`, `weekplanner.MuscleGroupTarget[]`.
2. Calls `planner.Plan(monday)` → `[]weekplanner.PlannedSession`.
3. Walks the result, builds `sessionAggregate{...}` for each, calls `s.repo.sessions.CreateBatch`.

Rewrite to:
1. Call `planner := domain.NewPlanner(prefs, exercises, targets)` — `prefs`, `exercises`, `targets` are now passed directly without mapping.
2. Call `plannedSessions, err := planner.Plan(monday)` → `[]domain.Session`.
3. Walk `plannedSessions`, building `sessionAggregate{...}` from each `domain.Session`. The aggregate's `ExerciseSets` get filled from `session.ExerciseSets`, taking the `ExerciseID` from `session.ExerciseSets[i].Exercise.ID` (the planner now embeds the full exercise, so this lookup is trivial).
4. The `exerciseByID` lookup in `service.go:190-193` is no longer needed for planner output — remove. The lookup may still be needed for buildPlannedSets — re-audit and remove if unused.
5. Drop the `weekplanner.Preferences{...}` mapping (lines 152-160) — pass `prefs` (which is already `domain.Preferences`) directly.
6. Drop the `weekplanner.Exercise{...}` mapping (lines 162-174) — pass `exercises` directly.
7. Drop the `weekplanner.MuscleGroupTarget{...}` mapping (lines 176-182) — pass `targets` directly.

Update the import: replace `"github.com/myrjola/petrapp/internal/weekplanner"` with `"github.com/myrjola/petrapp/internal/domain"` if not already added.

- [ ] **Step 4: Verify**

```bash
go build ./...
go test ./internal/domain/... ./internal/workout/... ./cmd/web/...
```
Expected: green. Some weekplanner tests may need touch-ups for the new return shape — fix as you go.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "Move weekplanner into internal/domain as Planner; collapse parallel types"
```

---

### Task 9: Move `WeeklyMuscleGroupVolume` aggregation from service to domain

**Files:**
- Modify: `internal/domain/muscle_group.go` (append the function + helpers)
- Modify: `internal/workout/service.go` (replace `Service.WeeklyMuscleGroupVolume` body with a thin wrapper)

- [ ] **Step 1: Append `WeeklyMuscleGroupVolume` and helpers to `internal/domain/muscle_group.go`**

Copy `aggregateMuscleGroupLoad` (`service.go:866-880`) and `creditMuscleGroups` (`service.go:884-900`) verbatim. Then add the public function:

```go
// WeeklyMuscleGroupVolume aggregates planned-vs-completed weekly load per
// muscle group across the supplied sessions. One entry is returned for
// every muscle group in groupNames, sorted to match groupNames' order.
// Groups with no contributions appear as zero-load rows so callers can
// render them without a separate query. Targets are joined from the targets
// slice; muscle groups missing from targets carry TargetSets = 0.
func WeeklyMuscleGroupVolume(
	sessions []Session,
	targets []MuscleGroupTarget,
	groupNames []string,
) []MuscleGroupVolume {
	targetByName := make(map[string]int, len(targets))
	for _, t := range targets {
		targetByName[t.MuscleGroupName] = t.WeeklySetTarget
	}

	known := make(map[string]struct{}, len(groupNames))
	for _, name := range groupNames {
		known[name] = struct{}{}
	}

	planned := make(map[string]float64, len(groupNames))
	completed := make(map[string]float64, len(groupNames))
	aggregateMuscleGroupLoad(sessions, known, planned, completed)

	result := make([]MuscleGroupVolume, 0, len(groupNames))
	for _, name := range groupNames {
		result = append(result, MuscleGroupVolume{
			Name:          name,
			CompletedLoad: completed[name],
			PlannedLoad:   planned[name],
			TargetSets:    targetByName[name],
		})
	}
	return result
}
```

- [ ] **Step 2: Add a focused unit test for the new function**

Create `internal/domain/muscle_group_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func Test_WeeklyMuscleGroupVolume_PlannedAndCompleted(t *testing.T) {
	chest := domain.Exercise{ //nolint:exhaustruct
		ID:                  1,
		Name:                "Bench Press",
		PrimaryMuscleGroups: []string{"Chest"},
	}
	completedAt := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	completedValue := 8

	sess := domain.Session{ //nolint:exhaustruct
		Date: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		ExerciseSets: []domain.ExerciseSet{ //nolint:exhaustruct
			{
				Exercise: chest,
				Sets: []domain.Set{
					{TargetValue: 5, CompletedAt: &completedAt, CompletedValue: &completedValue}, //nolint:exhaustruct
					{TargetValue: 5}, //nolint:exhaustruct // Not completed.
				},
			},
		},
	}

	got := domain.WeeklyMuscleGroupVolume(
		[]domain.Session{sess},
		nil,
		[]string{"Chest"},
	)

	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].Name != "Chest" {
		t.Errorf("Name = %q, want Chest", got[0].Name)
	}
	if got[0].PlannedLoad != 2*domain.PrimarySetWeight {
		t.Errorf("PlannedLoad = %v, want %v", got[0].PlannedLoad, 2*domain.PrimarySetWeight)
	}
	if got[0].CompletedLoad != domain.PrimarySetWeight {
		t.Errorf("CompletedLoad = %v, want %v", got[0].CompletedLoad, domain.PrimarySetWeight)
	}
}
```

- [ ] **Step 3: Replace `Service.WeeklyMuscleGroupVolume` body**

In `internal/workout/service.go`, replace the entire body of `Service.WeeklyMuscleGroupVolume` (lines 820-858) with:

```go
func (s *Service) WeeklyMuscleGroupVolume(
	ctx context.Context,
	sessions []Session,
) ([]MuscleGroupVolume, error) {
	groupNames, err := s.repo.exercises.ListMuscleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle groups: %w", err)
	}
	targets, err := s.repo.muscleTargets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle group targets: %w", err)
	}
	return domain.WeeklyMuscleGroupVolume(sessions, targets, groupNames), nil
}
```

Delete `aggregateMuscleGroupLoad` and `creditMuscleGroups` from `service.go`.

- [ ] **Step 4: Verify**

```bash
go test ./internal/domain/... ./internal/workout/... ./cmd/web/...
```
Expected: green.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "Move WeeklyMuscleGroupVolume aggregation to internal/domain"
```

---

### Task 10: Add `Session.Start(now time.Time) error` aggregate method (TDD)

**Files:**
- Create or modify: `internal/domain/session_test.go`
- Modify: `internal/domain/session.go`

- [ ] **Step 1: Write the failing test**

Create `internal/domain/session_test.go`:

```go
package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func Test_Session_Start_FromZero(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		Date: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	}

	if err := sess.Start(now); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !sess.StartedAt.Equal(now) {
		t.Errorf("StartedAt = %v, want %v", sess.StartedAt, now)
	}
}

func Test_Session_Start_AlreadyStarted_ReturnsErrAlreadyStarted(t *testing.T) {
	earlier := time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		Date:      time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		StartedAt: earlier,
	}

	err := sess.Start(now)
	if !errors.Is(err, domain.ErrAlreadyStarted) {
		t.Fatalf("Start: got %v, want ErrAlreadyStarted", err)
	}
	if !sess.StartedAt.Equal(earlier) {
		t.Errorf("StartedAt mutated to %v, want %v (unchanged)", sess.StartedAt, earlier)
	}
}
```

- [ ] **Step 2: Run the failing test**

Run: `go test ./internal/domain/ -run Test_Session_Start -v`
Expected: FAIL — `sess.Start undefined (type domain.Session has no field or method Start)`.

- [ ] **Step 3: Implement `Session.Start`**

Append to `internal/domain/session.go`:

```go
import "time"

// Start marks the session as begun at now. Returns ErrAlreadyStarted if the
// session was previously started; the existing StartedAt is left untouched
// in that case.
func (s *Session) Start(now time.Time) error {
	if !s.StartedAt.IsZero() {
		return ErrAlreadyStarted
	}
	s.StartedAt = now
	return nil
}
```

(Add `import "time"` to the top of the file if not already present. After Task 2 this file imports nothing, so add the import block.)

- [ ] **Step 4: Run tests; expect green**

Run: `go test ./internal/domain/ -run Test_Session_Start -v`
Expected: PASS for both subtests.

- [ ] **Step 5: Run the full domain suite to catch regressions**

Run: `go test ./internal/domain/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/session.go internal/domain/session_test.go
git commit -m "Add Session.Start aggregate method"
```

---

### Task 11: Add `Session.Complete(now time.Time) error` aggregate method (TDD)

**Files:**
- Modify: `internal/domain/session_test.go`
- Modify: `internal/domain/session.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/domain/session_test.go`:

```go
func Test_Session_Complete_AfterStart(t *testing.T) {
	startAt := time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		Date:      time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		StartedAt: startAt,
	}

	if err := sess.Complete(now); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !sess.CompletedAt.Equal(now) {
		t.Errorf("CompletedAt = %v, want %v", sess.CompletedAt, now)
	}
}

func Test_Session_Complete_NotStarted_ReturnsErrNotStarted(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		Date: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	}

	err := sess.Complete(now)
	if !errors.Is(err, domain.ErrNotStarted) {
		t.Fatalf("Complete: got %v, want ErrNotStarted", err)
	}
	if !sess.CompletedAt.IsZero() {
		t.Errorf("CompletedAt = %v, want zero", sess.CompletedAt)
	}
}
```

- [ ] **Step 2: Run failing**

Run: `go test ./internal/domain/ -run Test_Session_Complete -v`
Expected: FAIL — method undefined.

- [ ] **Step 3: Implement**

```go
// Complete marks the session as finished at now. Returns ErrNotStarted if
// the session has not been started yet — completion implies a prior start.
func (s *Session) Complete(now time.Time) error {
	if s.StartedAt.IsZero() {
		return ErrNotStarted
	}
	s.CompletedAt = now
	return nil
}
```

- [ ] **Step 4: Run; expect pass**

```bash
go test ./internal/domain/ -run Test_Session_Complete -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/domain/session.go internal/domain/session_test.go
git commit -m "Add Session.Complete aggregate method"
```

---

### Task 12: Add `Session.SetDifficulty(rating int) error` aggregate method (TDD)

**Files:** `internal/domain/session.go`, `internal/domain/session_test.go`

- [ ] **Step 1: Write failing tests**

```go
func Test_Session_SetDifficulty_ValidRange(t *testing.T) {
	for _, rating := range []int{1, 2, 3, 4, 5} {
		sess := domain.Session{} //nolint:exhaustruct
		if err := sess.SetDifficulty(rating); err != nil {
			t.Errorf("SetDifficulty(%d): %v", rating, err)
		}
		if sess.DifficultyRating == nil || *sess.DifficultyRating != rating {
			t.Errorf("DifficultyRating = %v, want %d", sess.DifficultyRating, rating)
		}
	}
}

func Test_Session_SetDifficulty_OutOfRange(t *testing.T) {
	for _, rating := range []int{0, -1, 6, 100} {
		sess := domain.Session{} //nolint:exhaustruct
		err := sess.SetDifficulty(rating)
		if !errors.Is(err, domain.ErrInvalidDifficultyRating) {
			t.Errorf("SetDifficulty(%d): got %v, want ErrInvalidDifficultyRating", rating, err)
		}
		if sess.DifficultyRating != nil {
			t.Errorf("DifficultyRating mutated to %v, want nil", sess.DifficultyRating)
		}
	}
}
```

- [ ] **Step 2: Run; expect FAIL**
- [ ] **Step 3: Implement**

```go
// SetDifficulty records the post-session difficulty rating (1-5). Returns
// ErrInvalidDifficultyRating when rating is outside that range.
func (s *Session) SetDifficulty(rating int) error {
	if rating < 1 || rating > 5 {
		return ErrInvalidDifficultyRating
	}
	s.DifficultyRating = &rating
	return nil
}
```

- [ ] **Step 4: Run; expect PASS**
- [ ] **Step 5: Commit** — `Add Session.SetDifficulty aggregate method`

---

### Task 13: Add `Session.MarkWarmupComplete(slotID int, now time.Time) error` (TDD)

**Files:** `internal/domain/session.go`, `internal/domain/session_test.go`

- [ ] **Step 1: Write failing tests**

```go
func Test_Session_MarkWarmupComplete_KnownSlot(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{ID: 11, Exercise: domain.Exercise{ID: 1}, Sets: nil, WarmupCompletedAt: nil}, //nolint:exhaustruct
			{ID: 12, Exercise: domain.Exercise{ID: 2}, Sets: nil, WarmupCompletedAt: nil}, //nolint:exhaustruct
		},
	}

	if err := sess.MarkWarmupComplete(12, now); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}
	if sess.ExerciseSets[1].WarmupCompletedAt == nil || !sess.ExerciseSets[1].WarmupCompletedAt.Equal(now) {
		t.Errorf("slot 12 WarmupCompletedAt = %v, want %v", sess.ExerciseSets[1].WarmupCompletedAt, now)
	}
	if sess.ExerciseSets[0].WarmupCompletedAt != nil {
		t.Errorf("slot 11 WarmupCompletedAt mutated to %v, want nil", sess.ExerciseSets[0].WarmupCompletedAt)
	}
}

func Test_Session_MarkWarmupComplete_UnknownSlot(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{ID: 11, Exercise: domain.Exercise{ID: 1}, Sets: nil, WarmupCompletedAt: nil}, //nolint:exhaustruct
		},
	}

	err := sess.MarkWarmupComplete(99, now)
	if !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}
```

- [ ] **Step 2: Run; expect FAIL**
- [ ] **Step 3: Implement**

```go
// MarkWarmupComplete records the warmup completion timestamp for the
// exercise slot identified by slotID. Returns ErrSlotNotFound if no slot
// matches.
func (s *Session) MarkWarmupComplete(slotID int, now time.Time) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID == slotID {
			s.ExerciseSets[i].WarmupCompletedAt = &now
			return nil
		}
	}
	return ErrSlotNotFound
}
```

- [ ] **Step 4: Run; expect PASS**
- [ ] **Step 5: Commit** — `Add Session.MarkWarmupComplete aggregate method`

---

### Task 14: Add `Session.UpdateSetWeight(slotID, setIndex int, weightKg float64) error` (TDD)

**Files:** `internal/domain/session.go`, `internal/domain/session_test.go`

- [ ] **Step 1: Write failing tests**

```go
func Test_Session_UpdateSetWeight_KnownSlotAndIndex(t *testing.T) {
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{
				ID: 11, Exercise: domain.Exercise{ID: 1}, //nolint:exhaustruct
				Sets: []domain.Set{
					{TargetValue: 5}, //nolint:exhaustruct
					{TargetValue: 5}, //nolint:exhaustruct
				},
			},
		},
	}

	if err := sess.UpdateSetWeight(11, 1, 80.0); err != nil {
		t.Fatalf("UpdateSetWeight: %v", err)
	}
	if sess.ExerciseSets[0].Sets[1].WeightKg == nil || *sess.ExerciseSets[0].Sets[1].WeightKg != 80.0 {
		t.Errorf("WeightKg = %v, want 80.0", sess.ExerciseSets[0].Sets[1].WeightKg)
	}
	if sess.ExerciseSets[0].Sets[0].WeightKg != nil {
		t.Errorf("set 0 WeightKg mutated to %v, want nil", sess.ExerciseSets[0].Sets[0].WeightKg)
	}
}

func Test_Session_UpdateSetWeight_UnknownSlot(t *testing.T) {
	sess := domain.Session{} //nolint:exhaustruct
	err := sess.UpdateSetWeight(99, 0, 80.0)
	if !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}

func Test_Session_UpdateSetWeight_OutOfBoundsIndex(t *testing.T) {
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{ID: 11, Exercise: domain.Exercise{ID: 1}, Sets: []domain.Set{{TargetValue: 5}}}, //nolint:exhaustruct
		},
	}
	err := sess.UpdateSetWeight(11, 5, 80.0)
	if !errors.Is(err, domain.ErrSetIndexOutOfBounds) {
		t.Fatalf("got %v, want ErrSetIndexOutOfBounds", err)
	}
}
```

- [ ] **Step 2: Run; expect FAIL**
- [ ] **Step 3: Implement**

```go
// UpdateSetWeight overwrites the weight on a single set within a slot.
// Returns ErrSlotNotFound or ErrSetIndexOutOfBounds when the lookup fails.
func (s *Session) UpdateSetWeight(slotID, setIndex int, weightKg float64) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID != slotID {
			continue
		}
		if setIndex < 0 || setIndex >= len(s.ExerciseSets[i].Sets) {
			return ErrSetIndexOutOfBounds
		}
		w := weightKg
		s.ExerciseSets[i].Sets[setIndex].WeightKg = &w
		return nil
	}
	return ErrSlotNotFound
}
```

- [ ] **Step 4: Run; expect PASS**
- [ ] **Step 5: Commit** — `Add Session.UpdateSetWeight aggregate method`

---

### Task 15: Add `Session.UpdateCompletedValue(slotID, setIndex, value int, now time.Time) error` (TDD)

**Files:** `internal/domain/session.go`, `internal/domain/session_test.go`

- [ ] **Step 1: Write failing tests**

```go
func Test_Session_UpdateCompletedValue_Sets(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{ID: 11, Exercise: domain.Exercise{ID: 1}, Sets: []domain.Set{{TargetValue: 5}}}, //nolint:exhaustruct
		},
	}

	if err := sess.UpdateCompletedValue(11, 0, 6, now); err != nil {
		t.Fatalf("UpdateCompletedValue: %v", err)
	}
	got := sess.ExerciseSets[0].Sets[0]
	if got.CompletedValue == nil || *got.CompletedValue != 6 {
		t.Errorf("CompletedValue = %v, want 6", got.CompletedValue)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(now) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, now)
	}
}

func Test_Session_UpdateCompletedValue_UnknownSlot(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{} //nolint:exhaustruct
	if err := sess.UpdateCompletedValue(99, 0, 6, now); !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}

func Test_Session_UpdateCompletedValue_OutOfBoundsIndex(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{ID: 11, Exercise: domain.Exercise{ID: 1}, Sets: []domain.Set{{TargetValue: 5}}}, //nolint:exhaustruct
		},
	}
	if err := sess.UpdateCompletedValue(11, 5, 6, now); !errors.Is(err, domain.ErrSetIndexOutOfBounds) {
		t.Fatalf("got %v, want ErrSetIndexOutOfBounds", err)
	}
}
```

- [ ] **Step 2: Run; expect FAIL**
- [ ] **Step 3: Implement**

```go
// UpdateCompletedValue records the actual reps (or seconds for time-based)
// achieved on a set, and stamps the completion time. Returns
// ErrSlotNotFound or ErrSetIndexOutOfBounds when the lookup fails.
func (s *Session) UpdateCompletedValue(slotID, setIndex, value int, now time.Time) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID != slotID {
			continue
		}
		if setIndex < 0 || setIndex >= len(s.ExerciseSets[i].Sets) {
			return ErrSetIndexOutOfBounds
		}
		v := value
		s.ExerciseSets[i].Sets[setIndex].CompletedValue = &v
		t := now
		s.ExerciseSets[i].Sets[setIndex].CompletedAt = &t
		return nil
	}
	return ErrSlotNotFound
}
```

- [ ] **Step 4: Run; expect PASS**
- [ ] **Step 5: Commit** — `Add Session.UpdateCompletedValue aggregate method`

---

### Task 16: Add unified `Session.RecordSet(slotID, setIndex int, signal Signal, weightKg *float64, completedValue int, now time.Time) error` (TDD)

This collapses today's `RecordSetCompletion` (weighted) and `RecordTimedSetCompletion` (timed) into one method. `weightKg` is nil for time-based sets.

**Files:** `internal/domain/session.go`, `internal/domain/session_test.go`

- [ ] **Step 1: Write failing tests**

```go
func Test_Session_RecordSet_Weighted(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	weight := 80.0
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{ID: 11, Exercise: domain.Exercise{ID: 1}, Sets: []domain.Set{{TargetValue: 5}}}, //nolint:exhaustruct
		},
	}

	err := sess.RecordSet(11, 0, domain.SignalOnTarget, &weight, 5, now)
	if err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	got := sess.ExerciseSets[0].Sets[0]
	if got.WeightKg == nil || *got.WeightKg != weight {
		t.Errorf("WeightKg = %v, want %v", got.WeightKg, weight)
	}
	if got.CompletedValue == nil || *got.CompletedValue != 5 {
		t.Errorf("CompletedValue = %v, want 5", got.CompletedValue)
	}
	if got.Signal == nil || *got.Signal != domain.SignalOnTarget {
		t.Errorf("Signal = %v, want SignalOnTarget", got.Signal)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(now) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, now)
	}
}

func Test_Session_RecordSet_Timed_NoWeight(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{
				ID: 11,
				Exercise: domain.Exercise{ //nolint:exhaustruct
					ID: 1, ExerciseType: domain.ExerciseTypeTime,
				},
				Sets: []domain.Set{{TargetValue: 30}}, //nolint:exhaustruct
			},
		},
	}

	err := sess.RecordSet(11, 0, domain.SignalOnTarget, nil, 32, now)
	if err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	got := sess.ExerciseSets[0].Sets[0]
	if got.WeightKg != nil {
		t.Errorf("WeightKg = %v, want nil for timed set", got.WeightKg)
	}
	if got.CompletedValue == nil || *got.CompletedValue != 32 {
		t.Errorf("CompletedValue = %v, want 32", got.CompletedValue)
	}
	if got.Signal == nil || *got.Signal != domain.SignalOnTarget {
		t.Errorf("Signal = %v, want SignalOnTarget", got.Signal)
	}
}

func Test_Session_RecordSet_UnknownSlot(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{} //nolint:exhaustruct
	err := sess.RecordSet(99, 0, domain.SignalOnTarget, nil, 5, now)
	if !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}

func Test_Session_RecordSet_OutOfBoundsIndex(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{ID: 11, Exercise: domain.Exercise{ID: 1}, Sets: []domain.Set{{TargetValue: 5}}}, //nolint:exhaustruct
		},
	}
	err := sess.RecordSet(11, 5, domain.SignalOnTarget, nil, 5, now)
	if !errors.Is(err, domain.ErrSetIndexOutOfBounds) {
		t.Fatalf("got %v, want ErrSetIndexOutOfBounds", err)
	}
}
```

- [ ] **Step 2: Run; expect FAIL**
- [ ] **Step 3: Implement**

```go
// RecordSet records the completion of a single set: signal (perceived
// effort), weight (nil for time-based exercises), the actual value (reps
// or seconds), and the completion timestamp. Returns ErrSlotNotFound or
// ErrSetIndexOutOfBounds when the lookup fails.
func (s *Session) RecordSet(
	slotID, setIndex int,
	signal Signal,
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
		sigCopy := signal
		set.Signal = &sigCopy
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

- [ ] **Step 4: Run; expect PASS**
- [ ] **Step 5: Commit** — `Add Session.RecordSet unified aggregate method`

---

### Task 17: Add `Session.AddExercise(ex Exercise, sets []Set) (slotID int, err error)` (TDD)

**Context:** The repo is responsible for assigning the actual `workout_exercise.id` (auto-increment from SQLite) when persisted, so the aggregate method here just appends the new `ExerciseSet` with `ID: 0` and returns 0. The repo's Update closure machinery is what later surfaces the auto-assigned ID. To make this testable in isolation we change the contract: the method assigns a stable in-aggregate negative sentinel (e.g. `-1`) for new slots, and the repo overrides `0`/negative IDs at insert time. Choose `0` to match the existing `saveExerciseSets` convention (zero ID = insert-and-assign).

**Files:** `internal/domain/session.go`, `internal/domain/session_test.go`

- [ ] **Step 1: Write failing tests**

```go
func Test_Session_AddExercise_Append(t *testing.T) {
	bench := domain.Exercise{ID: 1, Name: "Bench"} //nolint:exhaustruct
	squat := domain.Exercise{ID: 2, Name: "Squat"} //nolint:exhaustruct
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{ID: 11, Exercise: bench, Sets: nil}, //nolint:exhaustruct
		},
	}

	slotID, err := sess.AddExercise(squat, []domain.Set{{TargetValue: 5}}) //nolint:exhaustruct
	if err != nil {
		t.Fatalf("AddExercise: %v", err)
	}
	if slotID != 0 {
		t.Errorf("slotID = %d, want 0 (repo will assign on insert)", slotID)
	}
	if len(sess.ExerciseSets) != 2 {
		t.Fatalf("ExerciseSets length = %d, want 2", len(sess.ExerciseSets))
	}
	added := sess.ExerciseSets[1]
	if added.Exercise.ID != squat.ID {
		t.Errorf("Exercise.ID = %d, want %d", added.Exercise.ID, squat.ID)
	}
	if added.ID != 0 {
		t.Errorf("ID = %d, want 0", added.ID)
	}
	if len(added.Sets) != 1 || added.Sets[0].TargetValue != 5 {
		t.Errorf("Sets = %+v, want one set with TargetValue 5", added.Sets)
	}
}

func Test_Session_AddExercise_DuplicateExerciseID_ReturnsErr(t *testing.T) {
	bench := domain.Exercise{ID: 1, Name: "Bench"} //nolint:exhaustruct
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{ID: 11, Exercise: bench, Sets: nil}, //nolint:exhaustruct
		},
	}

	_, err := sess.AddExercise(bench, nil)
	if !errors.Is(err, domain.ErrExerciseAlreadyInSession) {
		t.Fatalf("got %v, want ErrExerciseAlreadyInSession", err)
	}
	if len(sess.ExerciseSets) != 1 {
		t.Errorf("ExerciseSets length = %d, want 1 (no append on error)", len(sess.ExerciseSets))
	}
}
```

- [ ] **Step 2: Run; expect FAIL**
- [ ] **Step 3: Implement**

```go
// AddExercise appends a new exercise slot to the session. The slot's stable
// ID is left as 0 — the repository assigns it at insert time, then mirrors
// the assigned ID back to the caller. Returns ErrExerciseAlreadyInSession
// when an existing slot already references the same Exercise.ID.
//
// The returned slotID is always 0 from the aggregate's POV; the actual
// workout_exercise.id is determined by SQLite. Service code reads the new
// ID by re-fetching the session after the Update closure commits.
func (s *Session) AddExercise(ex Exercise, sets []Set) (int, error) {
	for _, existing := range s.ExerciseSets {
		if existing.Exercise.ID == ex.ID {
			return 0, ErrExerciseAlreadyInSession
		}
	}
	s.ExerciseSets = append(s.ExerciseSets, ExerciseSet{ //nolint:exhaustruct // ID auto-assigned by repo; WarmupCompletedAt nil.
		ID:       0,
		Exercise: ex,
		Sets:     sets,
	})
	return 0, nil
}
```

- [ ] **Step 4: Run; expect PASS**
- [ ] **Step 5: Commit** — `Add Session.AddExercise aggregate method`

---

### Task 18: Add `Session.SwapExerciseInSlot(slotID int, newExercise Exercise, sets []Set) error` (TDD)

**Context:** Replaces the exercise occupying a slot, preserving the slot's stable ID (so URLs keep working). Drops any previously recorded sets — sets is the new prescription.

**Files:** `internal/domain/session.go`, `internal/domain/session_test.go`

- [ ] **Step 1: Write failing tests**

```go
func Test_Session_SwapExerciseInSlot_PreservesSlotID(t *testing.T) {
	bench := domain.Exercise{ID: 1, Name: "Bench"} //nolint:exhaustruct
	dip := domain.Exercise{ID: 2, Name: "Dip"}     //nolint:exhaustruct
	sess := domain.Session{ //nolint:exhaustruct
		ExerciseSets: []domain.ExerciseSet{
			{
				ID: 11, Exercise: bench, //nolint:exhaustruct
				Sets:              []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct
				WarmupCompletedAt: nil,
			},
		},
	}

	newSets := []domain.Set{{TargetValue: 8}, {TargetValue: 8}} //nolint:exhaustruct
	if err := sess.SwapExerciseInSlot(11, dip, newSets); err != nil {
		t.Fatalf("SwapExerciseInSlot: %v", err)
	}
	got := sess.ExerciseSets[0]
	if got.ID != 11 {
		t.Errorf("ID = %d, want 11 (preserved)", got.ID)
	}
	if got.Exercise.ID != dip.ID {
		t.Errorf("Exercise.ID = %d, want %d", got.Exercise.ID, dip.ID)
	}
	if len(got.Sets) != 2 {
		t.Errorf("Sets length = %d, want 2", len(got.Sets))
	}
	if got.WarmupCompletedAt != nil {
		t.Errorf("WarmupCompletedAt = %v, want nil (reset on swap)", got.WarmupCompletedAt)
	}
}

func Test_Session_SwapExerciseInSlot_UnknownSlot(t *testing.T) {
	sess := domain.Session{} //nolint:exhaustruct
	err := sess.SwapExerciseInSlot(99, domain.Exercise{ID: 2}, nil) //nolint:exhaustruct
	if !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}
```

- [ ] **Step 2: Run; expect FAIL**
- [ ] **Step 3: Implement**

```go
// SwapExerciseInSlot replaces the exercise occupying the slot identified by
// slotID with newExercise. The slot's stable ID is preserved (so URLs
// continue to resolve). The new sets slice replaces the slot's existing
// sets entirely; any prior recorded data is dropped. The warmup-completion
// flag is reset to nil because the warmup performed for the old exercise
// does not apply to the new one. Returns ErrSlotNotFound when no slot
// matches.
func (s *Session) SwapExerciseInSlot(slotID int, newExercise Exercise, sets []Set) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID != slotID {
			continue
		}
		s.ExerciseSets[i].Exercise = newExercise
		s.ExerciseSets[i].Sets = sets
		s.ExerciseSets[i].WarmupCompletedAt = nil
		return nil
	}
	return ErrSlotNotFound
}
```

- [ ] **Step 4: Run; expect PASS**
- [ ] **Step 5: Commit** — `Add Session.SwapExerciseInSlot aggregate method`

---

### Task 19: Delete the now-empty `internal/exerciseprogression/` and `internal/weekplanner/` packages

**Files:** `internal/exerciseprogression/`, `internal/weekplanner/`

- [ ] **Step 1: Verify nothing in the workspace still imports these packages**

```bash
grep -rn "internal/exerciseprogression\|internal/weekplanner" --include="*.go" .
```
Expected: no matches. If matches exist, fix the importing file to use `domain` first.

- [ ] **Step 2: Delete the package directories**

```bash
git rm -r internal/exerciseprogression internal/weekplanner
```

- [ ] **Step 3: Verify build + tests**

```bash
go build ./...
go test ./...
```
Expected: green.

- [ ] **Step 4: Commit**

```bash
git commit -m "Delete internal/exerciseprogression and internal/weekplanner (subsumed by domain)"
```

---

### Task 20: Add `internal/domain/CLAUDE.md` and update `internal/workout/CLAUDE.md`

**Files:**
- Create: `internal/domain/CLAUDE.md`
- Modify: `internal/workout/CLAUDE.md` (top-of-file note)

- [ ] **Step 1: Create `internal/domain/CLAUDE.md`**

```markdown
# Domain — Pure Entities & Business Rules

The `internal/domain` package is the canonical home for the workout
bounded context's pure logic. It depends on the Go standard library
only — no SQL, no HTTP, no logger, no third-party clients.

## What lives here

- **Entities:** `Exercise`, `Session`, `ExerciseSet`, `Set`, `Preferences`,
  `FeatureFlag`, `MuscleGroupTarget`, `MuscleGroupVolume`.
- **Value objects / enums:** `Category`, `ExerciseType`, `Signal`,
  `PeriodizationType`, `MuscleGroupRegion`.
- **Aggregate methods on `Session`:** `Start`, `Complete`,
  `SetDifficulty`, `MarkWarmupComplete`, `RecordSet`, `UpdateSetWeight`,
  `UpdateCompletedValue`, `AddExercise`, `SwapExerciseInSlot`. These
  enforce invariants and return sentinel errors when violated.
- **Domain services:** `Planner` (weekly plan generation),
  `Progression` / `TimedProgression` (set-to-set weight/seconds
  progression), `SwapSimilarityScore` (exercise-similarity score for
  swap UI), `WeeklyMuscleGroupVolume` (set-load aggregation),
  `BuildPlannedSets`, `DeriveScheme`, `ConvertWeight` (Epley).
- **Sentinel errors:** `ErrNotFound`, `ErrAlreadyStarted`,
  `ErrNotStarted`, `ErrSlotNotFound`, `ErrSetIndexOutOfBounds`,
  `ErrExerciseAlreadyInSession`, `ErrInvalidDifficultyRating`.

## What does NOT live here

- SQL, query strings, transactions — those live in
  `internal/repository` (Phase 2+).
- HTTP handlers, template data shaping — `cmd/web`.
- Service orchestration that touches multiple aggregates or
  external systems (OpenAI) — `internal/service` (Phase 3+).
- `sql.ErrNoRows` aliasing — `domain.ErrNotFound` is its own
  sentinel; the repository translates at the boundary.

## Display derivations belong on domain types

Any value that depends on multiple domain attributes, or that encodes
a business rule, lives as a method on the domain type that owns the
rule (`Exercise.IsTimed()`, `Exercise.FormatSetValue(v)`,
`Exercise.SetValueUnit()` are canonical examples). Handlers may
format primitives (`%d`, `%.1fkg`, `time.Format`) and shape data
into per-page template structs, but they may not branch on multiple
domain fields to compute a value.

**Test:** if changing the rule would force edits in two or more
files outside `internal/domain/`, it is a domain method.

## Aggregate methods

When adding behavior to `Session` (or any future aggregate), prefer a
method on the aggregate over a free function in service code. The
method enforces invariants in one place and returns a sentinel error
when violated; the service layer calls the method inside a repository
Update closure (the closure pattern is what gives us atomicity — see
`internal/repository/CLAUDE.md` once Phase 2 lands).
```

- [ ] **Step 2: Add a note to `internal/workout/CLAUDE.md`**

Insert at the top of the file (after the H1):

```markdown
> **Migration in progress (Phase 1 of 4 complete as of 2026-05-10).**
> Pure domain types now live in `internal/domain`. The `workout`
> package still exists but most of its public types are aliases for
> `domain` equivalents. New domain logic should be added to
> `internal/domain`. Phases 2–4 will extract `internal/repository`
> and `internal/service`, then delete this package entirely. See
> `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md`.
```

- [ ] **Step 3: Commit**

```bash
git add internal/domain/CLAUDE.md internal/workout/CLAUDE.md
git commit -m "Add internal/domain/CLAUDE.md; note migration status in workout/CLAUDE.md"
```

---

### Task 21: Final verification — full `make ci`

- [ ] **Step 1: Run the full CI pipeline**

```bash
make ci
```
Expected: green. This runs init, build, lint, test, sec.

If `golangci-lint` complains about anything (likely candidates: unused imports in workout/, exhaustruct on the new domain structs, govet shadow), fix in place. Don't add new `nolint` directives without justification — most lint complaints surface real issues.

- [ ] **Step 2: Spot-check the application boots**

```bash
make run
```

Manually navigate to `/` and confirm the home page loads. Then ctrl-C the server.

(This is the "type checking and test suites verify code correctness, not feature correctness" check from the project CLAUDE.md.)

- [ ] **Step 3: If everything is green, no further commit needed**

The plan is complete when `make ci` passes and the app boots. The workout package still functions exactly as before — Phase 1 is purely additive (new `internal/domain`) and structural (re-exports through aliases). The new aggregate methods exist and are tested but are not yet wired into service code; Phase 3 will do that wiring.

---

## Phase 1 done. Next steps:

- **Phase 2** plan: extract `internal/repository/`. Returns `domain.Session` directly, deletes aggregate types, switches `Update` closures to operate on `*domain.Session`.
- **Phase 3** plan: extract `internal/service/`. Service methods rewrite to call the new aggregate methods on `*domain.Session` inside the repository Update closures. The 1,299-line `service.go` splits across 9 files by responsibility.
- **Phase 4** plan: delete `internal/workout/`. Update all handler imports from `workout` to `domain`/`service`. Final CLAUDE.md sweep.

Each subsequent phase gets its own design discussion + plan — execution may surface refinements that make a fresh brainstorming pass cheaper than blindly committing now.
