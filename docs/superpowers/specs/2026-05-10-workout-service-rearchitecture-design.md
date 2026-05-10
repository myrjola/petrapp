# Workout service rearchitecture: domain / repository / service split

## Background

`internal/workout/` has accumulated four years of features inside a single
package. `service.go` alone is 1,299 lines and mixes session lifecycle,
set-recording, weekly plan orchestration, exercise CRUD, AI exercise
generation, feature flags, GDPR export, progression building, and
muscle-volume aggregation. `models.go` (280 lines) holds pure domain types.
The five `repository*.go` files (~1,500 lines combined) hold SQLite
implementations and persistence-shaped aggregate types
(`sessionAggregate`, `exerciseSetAggregate`).

Two pure packages already exist — `internal/exerciseprogression/` and
`internal/weekplanner/` — but each redeclares `Category`, `ExerciseType`,
`PeriodizationType`, and a stripped-down `Exercise`, with conversion code
in the workout service at every boundary
(`periodizationToProgression`, `toProgressionPeriodization`, the
`weekplanner.Exercise{...}` mapping in `generateWeeklyPlan`). That
duplication is the visible symptom of a missing canonical home for
domain rules.

Two specific pains drive this rearchitecture:

1. **Domain logic is scattered and duplicated.** The same rule (rep
   scheme derivation, periodization mapping, exercise type semantics)
   lives in three packages with conversion glue between them.
2. **Persistence leaks into the domain.** `workout.ErrNotFound` is
   `sql.ErrNoRows` aliased; the service must hand-translate
   `sessionAggregate` (lean, ID-only) into `Session` (rich, with
   embedded `Exercise`) on every read. The Update-with-closure pattern
   gives the service a `*sessionAggregate` and lets it mutate fields
   directly — the aggregate is anemic, with no invariants of its own.

Handler coupling to domain types and `service.go` file size are
secondary concerns and explicitly out of scope for the goal of this
spec, though file size will improve as a side effect.

## Goal

Reorganise `internal/workout/`, `internal/exerciseprogression/`, and
`internal/weekplanner/` into three new packages with strict dependency
direction:

```
internal/
├── domain/        ← pure: entities, value objects, aggregate methods, domain services
├── repository/    ← persistence: SQLite impls, transactional Update closures
└── service/       ← orchestration: one Service struct, called by handlers
```

After this work:

- One canonical `domain.Exercise`, `domain.PeriodizationType`, etc.
  Conversion code at the workout/weekplanner/exerciseprogression
  boundaries is deleted.
- `domain.Session` carries its own invariants as aggregate methods
  (`Start`, `Complete`, `RecordSet`, `MarkWarmupComplete`, etc.).
  Service no longer mutates aggregate fields directly.
- Repository returns `domain.*` types — no `sessionAggregate` anywhere.
  `domain.ErrNotFound` is its own sentinel, not aliased to
  `sql.ErrNoRows`.
- Behavior is identical to today. Every existing handler endpoint
  produces the same output. Tests are the contract.

## Non-goals

- **No view models for handlers.** Embedded `Exercise` in
  `Session.ExerciseSet` stays. Handlers continue to import domain
  types directly.
- **No new persistence backend.** `*sqlite.Database` remains the only
  storage; the service struct still holds a reference for
  `ExportUserData`'s `db.CreateUserDB` call.
- **No event sourcing, no CQRS read store, no domain events.**
- **No optimistic-concurrency version columns.** Closure-based tx is
  the concurrency story.
- **No splitting non-workout concerns into their own packages**
  (feature flags, AI exercise generation, GDPR export all stay inside
  `internal/service/`).
- **No `cmd/web/` restructuring.** Handlers get import renames; their
  organisation is unchanged.
- **No schema change.** SQL queries are identical, just relocated.
- **No `Endurance` periodization.** Currently dead code in
  `exerciseprogression`; deleted during the move.

## Design

### Package layout and dependency direction

```
internal/
├── domain/        ← depends on stdlib only (+ math/rand/v2)
├── repository/    ← depends on internal/domain, internal/sqlite
└── service/       ← depends on internal/domain, internal/repository,
                       internal/sqlite (for ExportUserData), internal/contexthelpers,
                       openai SDK
```

`cmd/web/` handlers depend on `internal/domain` (for types like
`domain.Session`, `domain.Signal`, `domain.SwapSimilarityScore`,
`domain.ErrNotFound`) and `internal/service` (for method calls).

The naming uses generic package names (`domain`, `repository`, `service`)
rather than `internal/workout/{...}` because there is one bounded
context — no other domain to disambiguate from.

### `internal/domain/`

#### Files

```
internal/domain/
├── exercise.go              ← Exercise, Category, ExerciseType, FormatSetValue, IsTimed, SetValueUnit, MuscleGroupRegion, RegionFor
├── set.go                   ← Set, Signal
├── session.go               ← Session, ExerciseSet, PeriodizationType, aggregate methods
├── preferences.go           ← Preferences + day helpers
├── feature_flag.go          ← FeatureFlag
├── muscle_group.go          ← MuscleGroupTarget, MuscleGroupVolume, PrimarySetWeight, SecondarySetWeight, WeeklyMuscleGroupVolume
├── progression.go           ← absorbs internal/exerciseprogression/progression.go
├── progression_timed.go     ← absorbs timed_progression.go
├── progression_scheme.go    ← absorbs scheme.go
├── progression_convert.go   ← absorbs conversion.go (Epley)
├── planner.go               ← absorbs internal/weekplanner/weekplanner.go (renamed Planner / NewPlanner)
├── planning_sets.go         ← BuildPlannedSets, deriveSchemeForExercise, defaultTimedSets, defaultTargetValue
├── swap.go                  ← SwapSimilarityScore
├── errors.go                ← sentinel errors
└── *_test.go                ← progression, planner, session, swap, etc.
```

No subpackages. Subpackages reintroduce the import-graph fragmentation
this spec is collapsing.

#### Type-level consolidation

The duplicated enums and structs in `workout`, `weekplanner`, and
`exerciseprogression` collapse to one canonical home each:

- `domain.Category` (string-typed: `"full_body"`, `"upper"`, `"lower"`).
- `domain.ExerciseType` (string-typed: `"weighted"`, `"bodyweight"`,
  `"assisted"`, `"time_based"`).
- `domain.PeriodizationType` (string-typed: `"strength"`,
  `"hypertrophy"`). The int-typed enums in `weekplanner` and
  `exerciseprogression` and the `periodizationToProgression`/
  `toProgressionPeriodization` converters all delete.
- `domain.Exercise` is the canonical shape — both planner and
  progression consume it directly. The stripped-down
  `weekplanner.Exercise` (no Name, no Description) folds into the rich
  `Exercise`; planner just doesn't read fields it doesn't need.

#### Aggregate methods on `domain.Session`

The Session aggregate gains these methods. Each enforces an invariant
and returns a sentinel error when violated. The service no longer
mutates Session fields directly.

```go
func (s *Session) Start(now time.Time) error                        // ErrAlreadyStarted if StartedAt non-zero
func (s *Session) Complete(now time.Time) error                     // ErrNotStarted if StartedAt zero
func (s *Session) SetDifficulty(rating int) error                   // validates 1-5 range
func (s *Session) MarkWarmupComplete(slotID int, now time.Time) error
func (s *Session) RecordSet(slotID, setIndex int, signal Signal, weightKg *float64, completedValue int, now time.Time) error
func (s *Session) UpdateSetWeight(slotID, setIndex int, weightKg float64) error
func (s *Session) UpdateCompletedValue(slotID, setIndex int, value int, now time.Time) error
func (s *Session) AddExercise(ex Exercise, sets []Set) (slotID int, err error)
func (s *Session) SwapExerciseInSlot(slotID int, newExercise Exercise, sets []Set) error
```

A single `RecordSet` method handles both weighted and time-based sets
via a `*float64` weightKg argument that is nil for time-based sets.
This collapses today's `RecordSetCompletion` and
`RecordTimedSetCompletion` (which differ only in which fields are nil).

#### Domain services (pure, package-level)

- `domain.NewPlanner(prefs, exercises, targets) *Planner` +
  `Planner.Plan(monday) ([]Session, error)`. Returns ready-to-persist
  `Session` aggregates. The `PlannedSession`/`PlannedExerciseSet`/
  `PlannedSet` types from `weekplanner` collapse into `Session`/
  `ExerciseSet`/`Set` with target values populated and weights nil.
- `domain.NewProgression(...)`, `domain.NewProgressionFromHistory(...)`,
  `domain.NewTimedProgression(...)` — same surface as today, relocated.
- `domain.SwapSimilarityScore(a, b Exercise) int` — pure, moved verbatim.
- `domain.WeeklyMuscleGroupVolume(sessions []Session, targets []MuscleGroupTarget, knownGroups []string) []MuscleGroupVolume` —
  moves out of `service.go` (~80 lines today) into the domain.
- `domain.BuildPlannedSets(ex Exercise, pt PeriodizationType) []Set` —
  single source of truth for set count + target derivation (today's
  `buildPlannedSets` in `workout/planning.go`).

#### Errors

`domain.ErrNotFound = errors.New("not found")` — *not* aliased to
`sql.ErrNoRows`. Repository translates `sql.ErrNoRows` →
`domain.ErrNotFound` at the boundary. This small change is the
symbolic move that ends "persistence leaks into domain": the domain's
NotFound has no SQL ancestry.

Other sentinels follow the codebase's `Err` prefix convention:
`ErrAlreadyStarted`, `ErrNotStarted`, `ErrSlotNotFound`,
`ErrSetIndexOutOfBounds`, `ErrExerciseAlreadyInSession`,
`ErrInvalidDifficultyRating`. Every sentinel has unhappy-path test
coverage.

### `internal/repository/`

#### Files

```
internal/repository/
├── repository.go            ← Repositories struct (composite holder)
├── sessions.go              ← sqliteSessionRepository (unexported)
├── exercises.go             ← sqliteExerciseRepository
├── preferences.go           ← sqlitePreferencesRepository
├── feature-flags.go         ← sqliteFeatureFlagRepository
├── muscle-targets.go        ← sqliteMuscleGroupTargetRepository
├── shared.go                ← parseTimestamp, formatDate, formatTimestamp, baseRepository, error translation
└── *_test.go                ← per-repository internal tests
```

#### Public surface

Repositories return `domain.*` types directly. The lean
`sessionAggregate`/`exerciseSetAggregate` pair disappears from the
codebase entirely. Get/List always hydrate exercises (per the "one
Session, always hydrated" decision).

```go
type Repositories struct {
    Sessions      SessionRepository
    Exercises     ExerciseRepository
    Preferences   PreferencesRepository
    FeatureFlags  FeatureFlagRepository
    MuscleTargets MuscleGroupTargetRepository
}

func New(db *sqlite.Database, logger *slog.Logger) *Repositories { ... }

type SessionRepository interface {
    Get(ctx context.Context, date time.Time) (domain.Session, error)
    List(ctx context.Context, sinceDate time.Time) ([]domain.Session, error)
    CreateBatch(ctx context.Context, sessions []domain.Session) error

    // Update loads the session, runs fn inside a single transaction with the
    // hydrated *domain.Session, then persists it. fn enforces invariants via
    // aggregate methods. Any error returned by fn rolls back; nil commits.
    Update(ctx context.Context, date time.Time, fn func(*domain.Session) error) error

    DeleteWeek(ctx context.Context, monday time.Time) error

    // Read-only specialised queries.
    ListSetsForExerciseSince(ctx context.Context, exerciseID int, sinceDate time.Time) ([]domain.ExerciseSetHistory, error)
    GetLatestStartingWeightBefore(ctx context.Context, exerciseID int, beforeDate time.Time) (domain.LatestStartingSet, error)
    GetLatestSuccessfulSecondsBefore(ctx context.Context, exerciseID int, beforeDate time.Time) (int, error)
    CountCompleted(ctx context.Context) (int, error)
}
```

`ExerciseSetHistory` and `LatestStartingSet` are pure value types in
`domain/` — they're domain query results, not persistence shapes.

Repository **interfaces** are exported (so service can declare
dependencies on them). Repository **structs**
(`sqliteSessionRepository`, etc.) are unexported. Only
`repository.New(db, logger) *Repositories` is the public constructor —
wiring stays one line in `cmd/web/main.go`.

#### Update closure semantics (the atomicity story)

The closure-based Update pattern survives the rearchitecture because
it is the load-bearing reason we get atomic read-modify-write today.
Concurrent submits are rare in PetrApp but real (a user double-tapping
submit, or two browser tabs).

Contract:

- Repo opens a tx, loads + hydrates the Session, calls
  `fn(*domain.Session)`.
- If fn returns nil → repo computes the diff, persists, commits.
- If fn returns an error → rollback. The error is returned to the
  caller as-is so `errors.Is(err, domain.ErrAlreadyStarted)` works at
  the service layer.
- The "no-op" case (e.g. `StartSession` called on an already-started
  session) returns `domain.ErrAlreadyStarted`; service decides whether
  to swallow it. Today's `StartSession` swallows the no-op — same
  behavior, expressed as `if errors.Is(err, domain.ErrAlreadyStarted)
  { return nil }`.

Diff strategy: delete-and-reinsert the dependent rows
(`workout_exercise`, `exercise_sets`) inside the tx. This is what
`saveExerciseSets` already does today (see
`internal/workout/repository-sessions.go:706`). The crucial detail
preserved by the rearchitecture: pre-existing `workout_exercise.id`
values are passed back into the `INSERT ... RETURNING id` so the slot
ID stays stable across an Update cycle — handler URLs that target a
slot (e.g. `/workouts/{date}/exercises/{slotID}`) continue to work
after any mutation. New slots (zero ID) get an auto-assigned ID, which
the new `Session.AddExercise` aggregate method then surfaces back to
the service so handlers can build URLs that point at the new slot.

For sessions of the size PetrApp deals with (a handful of exercises ×
a handful of sets), the delete-and-reinsert cost is negligible, and it
avoids the version-tracking complexity of dirty-checking individual
fields. Repository tests cover the round-trip explicitly: insert,
mutate via the closure, re-read, assert that `workout_exercise.id`
values are unchanged for pre-existing slots and that `sets` rows
reflect the in-memory mutations.

We considered and rejected the alternatives:

- **Optimistic concurrency** (version column + retries): clean, but
  schema change + retry plumbing for every write + conflict semantics
  in the UX layer is overkill for SQLite + single-process PetrApp.
- **Explicit Unit-of-Work / `tx` handle in service**: cleanest in
  theory, but doubles every repo method (`Get`/`GetTx`/`Save`/
  `SaveTx`) or threads tx through context. Heavyweight for this
  codebase.

### `internal/service/`

#### Files

```
internal/service/
├── service.go               ← Service struct, NewService, top-level orchestration, week generation
├── sessions.go              ← Start, Complete, SaveFeedback
├── sets.go                  ← RecordSet, MarkWarmupComplete, UpdateSetWeight, UpdateCompletedValue
├── exercises.go             ← Exercise CRUD + AddExercise / SwapExercise
├── progression.go           ← BuildProgression, BuildTimedProgression, GetStartingWeight, GetStartingSeconds
├── reporting.go             ← GetSessionsWithExerciseSince, GetExerciseSetsForExerciseSince, WeeklyMuscleGroupVolume passthrough
├── feature_flags.go         ← feature-flag passthroughs
├── exercise_generation.go   ← AI generation + GenerateExercise
├── export.go                ← ExportUserData
└── *_test.go
```

The split is by responsibility, not by file size. Once the
closure-pattern simplification lands, the methods in each file are
short enough that grouping by concern is the natural structure. A
monolithic `service.go` after the simplification would still be ~700
lines of mostly trivial wrappers.

#### Service struct

```go
type Service struct {
    repos        *repository.Repositories
    db           *sqlite.Database  // only for ExportUserData's CreateUserDB
    logger       *slog.Logger
    openaiAPIKey string
}

func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
    return &Service{
        repos:        repository.New(db, logger),
        db:           db,
        logger:       logger,
        openaiAPIKey: openaiAPIKey,
    }
}
```

Same constructor signature as today's `workout.NewService` —
`cmd/web/main.go` changes by one import + one type rename.

#### Method shape after the simplification

The set-recording methods are the biggest visible win. Today's
`RecordSetCompletion` is ~30 lines of in-place mutation embedded in a
closure. After:

```go
// service/sets.go
func (s *Service) RecordSet(
    ctx context.Context,
    date time.Time,
    workoutExerciseID, setIndex int,
    signal domain.Signal,
    weightKg *float64,
    completedValue int,
) error {
    err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
        return sess.RecordSet(workoutExerciseID, setIndex, signal, weightKg, completedValue, time.Now().UTC())
    })
    if err != nil {
        return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
    }
    return nil
}
```

Validation (slot lookup, bounds check) lives on `domain.Session.RecordSet`;
service is just orchestration + error wrapping. Same pattern for Start,
Complete, MarkWarmupComplete, UpdateSetWeight — each becomes a 3-line
closure.

Cross-aggregate methods (`AddExercise`, `SwapExercise`, week
generation) keep more logic in service because they orchestrate
multiple repositories: load history from sessions repo, load exercise
from exercises repo, build sets via `domain.BuildPlannedSets`, then
call `Sessions.Update` to mutate the session. Domain methods
(`Session.AddExercise`, `Session.SwapExerciseInSlot`) take the
already-built `[]Set` and just check the invariant + insert.

### Handler impact

Mechanical rename of imports — no semantic change to handler code:

```go
// Before
import "github.com/myrjola/petrapp/internal/workout"
var s workout.Session
errors.Is(err, workout.ErrNotFound)
score := workout.SwapSimilarityScore(a, b)

// After
import "github.com/myrjola/petrapp/internal/domain"
var s domain.Session
errors.Is(err, domain.ErrNotFound)
score := domain.SwapSimilarityScore(a, b)
```

11 handler files (per `grep workout. cmd/web/`) get this rename. Display
methods still live on the same types: `domain.Exercise.FormatSetValue(v)`,
`domain.Exercise.IsTimed()`, `domain.RegionFor(name)` — called from
templates/handlers identically.

The `application` struct in `cmd/web/main.go` changes one field type
(`workoutService *workout.Service` → `workoutService *service.Service`)
and one import. Service method names are unchanged so handler call
sites don't move.

### Template impact

Zero change. Templates only read fields off `domain.Session`/
`domain.Exercise` which are unchanged in shape. The current contract
that "display derivations belong on domain types" survives the move —
the methods just live on `domain.Exercise` instead of `workout.Exercise`.

### Test impact

This is the largest blast radius. Today's tests live in three places:

1. `internal/workout/service_test.go` (2,167 lines),
   `service_internal_test.go`, `models_test.go`, `swap_test.go`,
   `planning_internal_test.go`, `generator-exercise_internal_test.go`
   → most move to `internal/service/*_test.go` (orchestration tests
   using e2e-style setup) or `internal/domain/*_test.go` (pure-logic
   tests).
2. `internal/exerciseprogression/*_test.go` and
   `internal/weekplanner/*_test.go` → move to `internal/domain/`,
   renamed per the new types.
3. `cmd/web/handler-*_test.go` → only the import lines and type
   references change; assertions and HTTP behavior stay identical.

The new aggregate methods on `domain.Session` need their own focused
unit tests in `internal/domain/session_test.go` — these are the new
invariants, and the most important things to lock down before relying
on them.

### Documentation impact

- `internal/workout/CLAUDE.md` is replaced by `internal/domain/CLAUDE.md`,
  `internal/repository/CLAUDE.md`, `internal/service/CLAUDE.md`. Each
  inherits the relevant sections of the old file. The current rule
  about "display derivations belong on domain types" lives in the new
  `internal/domain/CLAUDE.md`, with the Test rule rephrased to point
  at `internal/domain/`.
- Root `CLAUDE.md` workflow step 2 changes "internal/workout/" →
  "internal/domain/ + internal/repository/ + internal/service/", with
  guidance on where each kind of change starts.

## Migration phasing

Too big for one PR. Four phases, each independently mergeable and
runnable, with no intermediate broken states:

1. **Phase 1 — extract `internal/domain/`.** Create the new package,
   move pure logic in (entities, planning helpers, swap, progression,
   weekplanner, BuildPlannedSets, MuscleGroupVolume aggregation, error
   sentinels). Add aggregate methods on `domain.Session` (`Start`,
   `Complete`, `RecordSet`, etc.) with full unit-test coverage. The
   methods exist and are tested in this phase but are not yet called
   by service code — the existing Update-with-closure-on-aggregate
   pattern continues running through Phase 2. Keep
   `internal/workout/` compiling by aliasing types
   (`type Session = domain.Session`) or re-exporting where structurally
   feasible. Handlers and tests start switching imports incrementally.
2. **Phase 2 — extract `internal/repository/`.** New package; repo
   returns `domain.Session` directly; aggregate types
   (`sessionAggregate`, `exerciseSetAggregate`) deleted. The Update
   closure now operates on `*domain.Session`. `internal/workout`
   repository code re-exports from `internal/repository/` during the
   phase.
3. **Phase 3 — extract `internal/service/`.** New package; service
   methods relocate; `internal/workout.NewService` becomes a thin
   alias. Set-recording methods rewritten to use aggregate methods on
   `domain.Session` (the visible payoff of Phases 1–2 lands here).
4. **Phase 4 — delete `internal/workout/`,
   `internal/exerciseprogression/`, `internal/weekplanner/`.** Aliases
   removed. Handlers and tests reference `domain`/`service` directly.
   Final import sweep across `cmd/web/`. Root `CLAUDE.md` and
   per-package CLAUDE.md updates land here.

Each phase ships passing `make ci`. The writing-plans pass will
detail concrete tasks and acceptance criteria per phase.
