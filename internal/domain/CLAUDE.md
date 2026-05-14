# Domain — Pure Entities & Business Rules

The `internal/domain` package is the canonical home for the workout
bounded context's pure logic. It depends on the Go standard library
only — no SQL, no HTTP, no logger, no third-party clients.

## What lives here

- **Entities:** `Exercise`, `Session`, `ExerciseSet`, `Set`, `Preferences`,
  `FeatureFlag`, `MuscleGroupTarget`, `MuscleGroupVolume`.
- **Value objects / enums:** `Category`, `ExerciseType`, `Signal`,
  `PeriodizationType`, `MuscleGroupRegion`, `SessionStatus`,
  `ExerciseSetState`. The last two are display-state enums whose string
  values double as CSS state tokens on the workout page.
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
- **`ValidationError`:** a struct error carrying a user-facing message,
  distinct from the sentinels above. `Exercise.Validate()` is the single
  source of truth for exercise-form validation and returns one. Unlike a
  sentinel (matched with `errors.Is`), callers detect it with `errors.As`
  and surface the message through the flash + banner flow — see
  `cmd/web/CLAUDE.md`.

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
