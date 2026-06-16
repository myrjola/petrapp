# Domain — Pure Entities & Business Rules

The `internal/petra/domain` package is the canonical home for the workout
bounded context's pure logic. It depends on the Go standard library
only — no SQL, no HTTP, no logger, no third-party clients. The vocabulary used
here is the repo's ubiquitous language — see [CONTEXT.md](../../../CONTEXT.md).

## What lives here

- **Entities:** `Exercise`, `Session`, `ExerciseSlot`, `Set`, `Preferences`,
  `FeatureFlag`, `MuscleGroupTarget`, `MuscleGroupVolume`.
- **Value objects / enums:** `Category`, `ExerciseType`, `Signal`,
  `SessionGoal`, `MuscleGroupRegion`, `SessionStatus`,
  `ExerciseSlotState`, `MuscleGroupVolumeStatus`. The last three are
  display-state enums whose string values double as CSS state tokens.
- **Aggregate methods on `Session`:** `Start`, `Complete`,
  `SetDifficulty`, `MarkWarmupComplete`, `RecordSet`, `UpdateSetWeight`,
  `UpdateCompletedValue`, `AddExercise`, `SwapExerciseInSlot`,
  `SwitchToDeload`, `ClearDeload`, `SeedDeloadWeights`. These
  enforce invariants and return sentinel errors when violated.
- **Domain services:** `Planner` (weekly plan generation),
  `Progression` / `TimedProgression` (set-to-set weight/seconds
  progression), `SwapSimilarityScore` (exercise-similarity score for
  swap UI), `WeeklyMuscleGroupVolume` (volume aggregation),
  `BuildPlannedSets`, `DeriveScheme`, `ConvertWeight` (Epley).
- **Sentinel errors:** `ErrNotFound`, `ErrAlreadyStarted`,
  `ErrNotStarted`, `ErrSlotNotFound`, `ErrSetIndexOutOfBounds`,
  `ErrExerciseAlreadyInSession`, `ErrInvalidDifficultyRating`.
- **`ValidationError`:** a struct error carrying a user-facing message,
  distinct from the sentinels above. `Exercise.Validate()` is the single
  source of truth for exercise-form validation and returns one. Unlike a
  sentinel (matched with `errors.Is`), callers detect it with `errors.As`
  and surface the message through the flash + banner flow — see
  [`cmd/petra/README.md`](../../../cmd/petra/README.md).

## What does NOT live here

- SQL, query strings, transactions — those live in `internal/petra/repository`
  (see its [README](../repository/README.md)).
- HTTP handlers, template data shaping — `cmd/petra`.
- Service orchestration that touches multiple aggregates or external systems
  (OpenAI) — `internal/petra/service` (see its [README](../service/README.md)).
- `sql.ErrNoRows` aliasing — `domain.ErrNotFound` is its own sentinel; the
  repository translates at the boundary.

## Display derivations belong on domain types

Any value that depends on multiple domain attributes, or that encodes
a business rule, lives as a method on the domain type that owns the
rule (`Exercise.IsTimed()`, `Exercise.FormatSetValue(v)`,
`Exercise.SetValueUnit()` are canonical examples). Handlers may
format primitives (`%d`, `%.1fkg`, `time.Format`) and shape data
into per-page template structs, but they may not branch on multiple
domain fields to compute a value.

**Test:** if changing the rule would force edits in two or more
files outside `internal/petra/domain/`, it is a domain method.

## Aggregate methods

When adding behavior to `Session` (or any future aggregate), prefer a
method on the aggregate over a free function in service code. The
method enforces invariants in one place and returns a sentinel error
when violated; the service layer calls the method inside a repository
Update closure (the closure pattern is what gives us atomicity — see
the [repository README](../repository/README.md) and
[ADR 0003](../../../docs/adr/0003-weekplan-aggregate-delete-and-reinsert.md)).

## Testing

Domain tests live in `package domain_test` (external) and assert behaviour
through the exported API — that is the default and covers all but a sliver of
this package. A white-box test in `package domain` (a `*_internal_test.go`
file) is justified **only** when an invariant the code genuinely relies on is
invisible to every public seam, and the test must name the seam it protects and
why no behavioural assertion reaches it. The bar is deliberately high: a
white-box test that *could* be written behaviourally is a duplicate that pins an
implementation detail, and it will rot.

The lone current example is `Test_goalForWeek` in
`planner_scoring_internal_test.go`: it pins the half-integer quantisation that
keeps planner scores reproducible under Go's randomised map iteration — a
numeric property no `Plan`/`PlanDay` output exposes (and that
`TestPlanner_Plan_Deterministic` cannot catch, since one binary may pick the
same map order twice by luck). The scoring *ordering*, by contrast, **is**
observable — `TestPlanner_PlanDay_PrefersUnderTargetMuscle` and
`OverSaturatedMuscleLosesToFreshMuscle` assert it through the public surface — so
it gets no white-box test.
