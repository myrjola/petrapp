# Workout Rearchitecture — Phase 3: Extract `internal/service/`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the orchestration layer out of `internal/workout/` into a new `internal/service/` package, splitting `service.go` (~900 lines) and `generator-exercise.go` (~262 lines) across nine focused files. `internal/workout` shrinks to a backward-compat shim (`type Service = service.Service`, `NewService` re-export, plus the existing type aliases in `models.go`) so `cmd/web/` keeps compiling unchanged. Phase 3 is a relocation: no behavior change, no API change for handlers, all existing tests keep passing.

**Architecture:** New package `internal/service/` exposes one public type `Service` and one public constructor `NewService(db, logger, openaiAPIKey) *Service`. Methods are split across files by responsibility (`sessions.go`, `sets.go`, `exercises.go`, `progression.go`, `reporting.go`, `feature_flags.go`, `exercise_generation.go`, `export.go`) plus the struct/ctor/preferences shell in `service.go`. The `repo` import alias from Phase 2 is dropped — `internal/service` has no name collision with `repository`. The `internal/workout` package retains `models.go` (type aliases for `cmd/web`) and gains a one-file shim that re-exports `service.NewService` + `service.Service`. Phase 4 will delete the workout package entirely.

**Tech Stack:** Go (stdlib + `internal/domain` + `internal/repository` + `internal/sqlite` + `internal/contexthelpers` + `github.com/openai/openai-go/v3`), no new dependencies.

**Phase boundary:** This plan covers Phase 3 only. Phase 4 (separate plan) deletes `internal/workout/` entirely, drops the type aliases, and sweeps `cmd/web/` imports to reference `internal/domain` and `internal/service` directly.

**Spec:** `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md` — sections "internal/service/" and "Migration phasing".

---

## File Structure

### New files in `internal/service/`

| File | Responsibility |
|---|---|
| `internal/service/service.go` | `Service` struct, `NewService`, `GetUserPreferences`, `SaveUserPreferences` — the small constructor + preferences shell that doesn't earn its own file |
| `internal/service/sessions.go` | Session lifecycle + week generation: `GetSession`, `StartSession`, `CompleteSession`, `SaveFeedback`, `MarkWarmupComplete`, `RegenerateWeeklyPlanIfUnstarted`, `ResolveWeeklySchedule`, `generateWeeklyPlan`, `mondayOf` |
| `internal/service/sessions_internal_test.go` | The renamed `service_internal_test.go` — tests `mondayOf` (now a sessions-package helper) |
| `internal/service/sets.go` | Set mutations: `UpdateSetWeight`, `UpdateCompletedValue`, `RecordSet` |
| `internal/service/exercises.go` | Exercise CRUD + slot mutations: `List`, `GetExercise`, `UpdateExercise`, `ListMuscleGroups`, `FindCompatibleExercises`, `AddExercise`, `SwapExercise`, plus the helpers `findHistoricalSets`, `copySetsWithoutCompletion`, `buildSetsForAdd` |
| `internal/service/progression.go` | Per-exercise progression construction: `GetStartingWeight`, `GetStartingSeconds`, `BuildProgression`, `BuildTimedProgression` |
| `internal/service/reporting.go` | Read-only aggregations: `GetSessionsWithExerciseSince`, `GetExerciseSetsForExerciseSince`, `WeeklyMuscleGroupVolume` |
| `internal/service/feature_flags.go` | Feature-flag passthroughs: `GetFeatureFlag`, `IsMaintenanceModeEnabled`, `ListFeatureFlags`, `SetFeatureFlag` |
| `internal/service/exercise_generation.go` | AI exercise generation, end to end: the `exerciseGenerator` struct + `Generate` method, the unexported `exerciseJSONSchema` type, the service methods `GenerateExercise` + `generateExerciseContent` + `createMinimalExercise` |
| `internal/service/exercise_generation_internal_test.go` | The renamed `generator-exercise_internal_test.go` — long-running OpenAI smoke test |
| `internal/service/export.go` | GDPR export: `ExportUserData` (the only method that touches `s.db` directly) |
| `internal/service/service_test.go` | The relocated `service_test.go` — single file, external `package service_test`, ~2,170 lines |
| `internal/service/CLAUDE.md` | New per-package guide: orchestration responsibilities, what does/doesn't belong, dependency arrows, where new code goes |

### Modified files in `internal/workout/`

| File | Change |
|---|---|
| `internal/workout/service.go` | Replaced. New body: `type Service = service.Service` and `func NewService(db, logger, openaiAPIKey) *Service { return service.NewService(...) }`. Everything else moves out. ~20 lines total. |
| `internal/workout/models.go` | Delete the `exerciseJSONSchema` struct + `MarshalJSON` (lines 84-154). Keep all type aliases, `ErrNotFound`, `RegionFor`, `SwapSimilarityScore` for Phase 4. |
| `internal/workout/CLAUDE.md` | Replace contents with Phase 3 progress note (Phases 1-3 done; orchestration now in `internal/service/`; this package is a shim until Phase 4). |

### Deleted files

| File | Reason |
|---|---|
| `internal/workout/generator-exercise.go` | Body relocated into `internal/service/exercise_generation.go`. |
| `internal/workout/generator-exercise_internal_test.go` | Test relocated into `internal/service/exercise_generation_internal_test.go`. |
| `internal/workout/service_internal_test.go` | Test relocated into `internal/service/sessions_internal_test.go`. |
| `internal/workout/service_test.go` | Test relocated into `internal/service/service_test.go`. |

### Untouched

| Path | Reason |
|---|---|
| `internal/sqlite/` | No schema change. |
| `internal/domain/` | Phase 1 surface is stable. |
| `internal/repository/` | Phase 2 surface is stable. |
| `internal/workout/README.md` | Stays put through Phase 4. |
| `cmd/web/*.go` | All `workout.Service` / `workout.NewService` / `workout.ErrNotFound` / `workout.Exercise` etc. references continue to resolve through the shim + existing type aliases. |
| `ui/templates/`, `ui/static/` | No template or asset changes. |

---

## Migration sequencing rationale

Each task leaves the tree compiling and `make test` green. The shape:

1. **Task 1: Scaffold `internal/service/` skeleton.** Empty package with a CLAUDE.md and a placeholder `service.go` containing only `package service` so Go tools recognize it. Nothing else uses it yet.
2. **Tasks 2-9: Move methods file by file** out of `internal/workout/service.go` into `internal/service/{file}.go`. After each task, the moved methods are callable from `internal/workout/service.go` via a single helper that delegates to a `*service.Service` field — but that's fragile. Instead we use a different pattern: build up `service.Service` in the new package as we move methods, but keep `internal/workout.Service` as the user-facing type during the migration. We invert this — see the cleaner sequence below.
3. **Better sequence:** flip the cutover early. After Task 1, do **Task 2: build the full `service.Service` struct + `NewService` in the new package, identical to workout's**. Then Tasks 3-10 move method bodies one file at a time from `workout.Service` to `service.Service`, and each task replaces `workout.Service`'s method with a one-line forward to `service.Service` (via a `*service.Service` field). Once all methods are forwarded, Task 11 collapses `workout.Service` into `type Service = service.Service` and drops the field. This keeps the tree green at every step but means writing forwarders that get deleted. Wasted churn.
4. **Adopted sequence (cleanest of all): cut over by package, not by method.** Move the entire `service.go` body in one task — the result lives in `internal/service/` split across the nine files — and atomically replace `internal/workout/service.go` with the type-alias shim in the same task. `cmd/web/` keeps compiling because `workout.Service` is now an alias for `service.Service`. Tests stay in `internal/workout_test` until a follow-up task moves them. This is one big task (Task 4) flanked by smaller, easier-to-review setup and cleanup tasks. The big task is mechanical (text relocation + import rewriting), so the size is fine.

The tasks below follow this adopted sequence:

1. Scaffold `internal/service/` (CLAUDE.md + empty `service.go`).
2. Move `exercise_generation.go` end-to-end (small, self-contained, exercises the new package's compile against the OpenAI dep). The OLD generator file in workout stays in place — it's still imported by workout's service. After this task workout depends on `internal/service` for its generator.

   Wait — that introduces a temporary import of `service` from `workout`, then `service` already imports `domain` + `repository`. Need to verify no cycle. `workout` imports `service` (new) + `domain` (existing) + `repository` (none today, only via service). `service` imports `domain` + `repository`. No cycle. 
3. Move all remaining service methods into the eight other files; replace `internal/workout/service.go` with the alias shim; delete `exerciseJSONSchema` from `workout/models.go`.
4. Move `service_test.go` and the two internal-tests into `internal/service/`.
5. Refresh CLAUDE.md files.

The adopted task list below uses 6 numbered tasks total (counting tests + docs).

---

## Tasks

### Task 1: Scaffold `internal/service/`

**Files:**
- Create: `internal/service/service.go`
- Create: `internal/service/CLAUDE.md`

- [ ] **Step 1: Create the empty package**

Create `internal/service/service.go` containing just the package declaration:

```go
// Package service holds workout orchestration: cross-aggregate coordination,
// external integrations (OpenAI, GDPR export), and the methods called by
// HTTP handlers. Pure rules live in internal/domain; persistence lives in
// internal/repository. This package depends on both.
package service
```

- [ ] **Step 2: Create the CLAUDE.md**

Create `internal/service/CLAUDE.md`:

```markdown
# Service — Orchestration Layer

The `internal/service` package coordinates work across `internal/domain`
and `internal/repository`, exposes the API consumed by `cmd/web` HTTP
handlers, and owns the small set of integrations that don't fit either
of the lower layers (OpenAI exercise generation, GDPR export).

It depends on `internal/domain`, `internal/repository`, `internal/sqlite`
(only for the `*sqlite.Database` handle that GDPR export passes through),
and `internal/contexthelpers`. It does NOT depend on `cmd/web`,
`ui/templates`, or `internal/workout`.

## What lives here

- **`Service` struct + `NewService` constructor.** One monolithic struct
  that handlers reference as `*service.Service`. Phase 4 will rename the
  field on the web app from `workoutService` to `service` and drop the
  `internal/workout` shim.
- **Session orchestration** (`sessions.go`): start/complete/feedback,
  weekly plan generation, schedule resolution, the `mondayOf` helper.
- **Set mutations** (`sets.go`): all `Session.Update`-via-aggregate
  calls that change recorded set data.
- **Exercise CRUD + slot ops** (`exercises.go`): list/get/update,
  `AddExercise`, `SwapExercise`, plus the historical-set lookup
  helpers used by both.
- **Progression construction** (`progression.go`): build
  `domain.Progression` and `domain.TimedProgression` values from a
  session's recorded sets and the rep/seconds cross-period conversion
  helpers.
- **Reporting** (`reporting.go`): read-only aggregations across
  sessions and muscle groups.
- **Feature flags** (`feature_flags.go`): thin passthroughs to the
  feature-flag repository plus the `IsMaintenanceModeEnabled`
  fail-safe.
- **AI exercise generation** (`exercise_generation.go`): the OpenAI
  client wrapper, the JSON-schema helper, the AI-or-fallback decision
  tree, and the wrapping `GenerateExercise` service method that
  persists the result.
- **GDPR export** (`export.go`): `ExportUserData` — the only method
  that touches `*sqlite.Database` directly.

## What does NOT live here

- **Pure rules / value objects / aggregate methods:** `internal/domain/`.
  See `internal/domain/CLAUDE.md`.
- **SQL queries / repository implementations:** `internal/repository/`.
  See `internal/repository/CLAUDE.md`.
- **HTTP handlers, request/response shaping, CSRF, sessions:** `cmd/web/`.
- **Schema and migrations:** `internal/sqlite/`.

## Update-closure pattern

Every method that mutates a `domain.Session` does so through
`s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error
{ ... })`. The closure body should be a single call to a domain aggregate
method (e.g. `return sess.RecordSet(...)`); domain sentinels propagate
unchanged. The service layer wraps the outer error with `fmt.Errorf` to
satisfy `wrapcheck` and to add the date for diagnostic context.

## Where to add new code

- **New cross-aggregate orchestration:** the file matching the dominant
  aggregate (e.g. a method that mutates a `Session` lives in
  `sessions.go` or `sets.go` depending on whether it's lifecycle or
  set-level).
- **New external integrations:** their own file (precedent:
  `exercise_generation.go`, `export.go`).
- **New pure rules:** `internal/domain/`, then call the rule from a
  one-line service method here.
- **New SQL:** `internal/repository/`, then a one-line service method
  here that wraps the call with `fmt.Errorf` and returns.
```

- [ ] **Step 3: Verify the package compiles**

Run: `go build ./internal/service/`
Expected: no output (success).

- [ ] **Step 4: Run the full test suite to confirm baseline**

Run: `make test`
Expected: PASS (no behavior changed; just a new empty package).

- [ ] **Step 5: Commit**

```bash
git add internal/service/service.go internal/service/CLAUDE.md
git commit -m "Scaffold internal/service/ package (Phase 3)"
```

---

### Task 2: Move AI exercise generation into `internal/service/`

This task moves the entire AI-generation surface into the new package as
a self-contained unit. After this, `internal/workout/service.go`'s
`GenerateExercise` calls into `service` (temporary cross-package
dependency that vanishes in Task 3).

**Files:**
- Create: `internal/service/exercise_generation.go`
- Create: `internal/service/exercise_generation_internal_test.go`
- Modify: `internal/workout/service.go` (rewire `GenerateExercise` body to call into the new package)
- Modify: `internal/workout/models.go` (delete `exerciseJSONSchema`)
- Delete: `internal/workout/generator-exercise.go`
- Delete: `internal/workout/generator-exercise_internal_test.go`

The new file consolidates: the generator struct + constructor + `Generate` method (~262 lines from `generator-exercise.go`), the `exerciseJSONSchema` JSON-schema struct + `MarshalJSON` (~70 lines from `models.go`), and the `generateExerciseContent` + `createMinimalExercise` helpers + the public `GenerateExercise` service method (~50 lines from `service.go`). Total: ~370 lines.

- [ ] **Step 1: Create `internal/service/exercise_generation.go`**

Copy the source code into a single file with this layout (top of file
to bottom): package + imports → `exerciseJSONSchema` type and
`MarshalJSON` → `exerciseGenerator` struct + `newExerciseGenerator` +
`Generate` + private generator methods → `(*Service).GenerateExercise`
+ `(*Service).generateExerciseContent` + `createMinimalExercise`.

The exact source to copy:

- The full body of `internal/workout/generator-exercise.go` lines 1-262, but:
  - Change the package declaration from `package workout` to `package service`.
  - Drop the `package workout` line and merge the imports with the schema-section imports below.
  - Inside the file, references to `Exercise`, `Category`, `ExerciseType`, etc. become `domain.Exercise`, `domain.Category`, etc. (because the type aliases live only in `internal/workout/models.go`).
- The `exerciseJSONSchema` struct + `MarshalJSON` from `internal/workout/models.go` lines 87-154. No type changes needed (it's a leaf type with no domain references).
- The `(*Service).GenerateExercise`, `(*Service).generateExerciseContent`, and `createMinimalExercise` from `internal/workout/service.go` lines 581-647. Same domain-prefix substitution.

Full file contents:

```go
// Package service: AI-backed exercise generation.
//
// This file owns the OpenAI-driven generator that fills in a freshly
// named exercise's metadata (category, type, muscle groups, description,
// resources). The decision tree in generateExerciseContent prefers the
// AI path; on any failure (missing API key, network error, malformed
// response, schema validation failure) it falls back to a minimal
// exercise so the user can edit the rest by hand. GenerateExercise
// persists whichever exercise was produced.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// exerciseJSONSchema is the JSON-schema description that the OpenAI
// chat completion endpoint validates the AI's response against. The
// muscle-group enum is dynamic — the generator constructs the schema
// per call with the muscle groups the database currently exposes so
// the AI can never invent ones we don't track.
type exerciseJSONSchema struct {
	muscleGroups []string
}

func (ejs exerciseJSONSchema) MarshalJSON() ([]byte, error) {
	schema := map[string]any{
		"type": "object",
		"required": []string{
			"id",
			"name",
			"category",
			"exercise_type",
			"description_markdown",
			"primary_muscle_groups",
			"secondary_muscle_groups",
		},
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "integer",
				"description": "Unique identifier for the exercise, leave as -1 for new exercises",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Name of the exercise",
			},
			"category": map[string]any{
				"type":        "string",
				"description": "Category of the exercise",
				"enum":        []string{"full_body", "upper", "lower"},
			},
			"exercise_type": map[string]any{
				"type":        "string",
				"description": "Type of exercise: weighted, bodyweight, assisted, or time_based",
				"enum":        []string{"weighted", "bodyweight", "assisted", "time_based"},
			},
			"default_starting_seconds": map[string]any{
				"type":        "integer",
				"description": "Default starting seconds for time_based exercises; omit for other types",
			},
			"description_markdown": map[string]any{
				"type":        "string",
				"description": "Markdown description of the exercise",
			},
			"primary_muscle_groups": map[string]any{
				"type":        "array",
				"description": "Primary muscle groups targeted by the exercise",
				"items": map[string]any{
					"type": "string",
					"enum": ejs.muscleGroups,
				},
			},
			"secondary_muscle_groups": map[string]any{
				"type":        "array",
				"description": "Secondary muscle groups targeted by the exercise",
				"items": map[string]any{
					"type": "string",
					"enum": ejs.muscleGroups,
				},
			},
		},
		"additionalProperties": false,
	}
	result, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal exercise schema: %w", err)
	}
	return result, nil
}

// (then: exerciseGenerator struct, newExerciseGenerator constructor,
//  Generate method, generateBaseExercise, enhanceWithWebSearch,
//  updateResourcesInDescription, validateMuscleGroups —
//  copied verbatim from internal/workout/generator-exercise.go lines
//  17-261, with these in-place edits inside the bodies:
//    - Exercise              → domain.Exercise
//    - Category(...)         → domain.Category(...)
//    - ExerciseType(...)     → domain.ExerciseType(...)
//    - Resource              → domain.Resource
//    - any other workout-aliased domain type      → domain.<Type>
// )

// GenerateExercise generates a new exercise based on a name.
//
// In case of errors, it persists a minimal exercise that the user can fill in later.
// The returned exercise is guaranteed to have at least Name and ID fields set.
func (s *Service) GenerateExercise(ctx context.Context, name string) (domain.Exercise, error) {
	exercise := s.generateExerciseContent(ctx, name)

	persisted, err := s.repos.Exercises.Create(ctx, exercise)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("create exercise: %w", err)
	}

	return persisted, nil
}

// generateExerciseContent creates exercise content, using AI generation if available
// or falling back to minimal content if not possible.
func (s *Service) generateExerciseContent(ctx context.Context, name string) domain.Exercise {
	if s.openaiAPIKey == "" {
		return createMinimalExercise(name)
	}

	muscleGroups, err := s.repos.Exercises.ListMuscleGroups(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to get muscle groups", slog.Any("error", err))
		return createMinimalExercise(name)
	}

	generator := newExerciseGenerator(s.openaiAPIKey, muscleGroups, s.logger)
	generated, err := generator.Generate(ctx, name)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to generate exercise details",
			slog.Any("error", err), slog.String("name", name))
		return createMinimalExercise(name)
	}

	// Defensive default: the AI prompt does not carry rep_min/rep_max, and
	// the DB CHECK requires them for non-time-based exercises. Mirror the
	// values used by createMinimalExercise so the Create downstream succeeds.
	if generated.ExerciseType != domain.ExerciseTypeTime &&
		(generated.RepMin == nil || generated.RepMax == nil) {
		repMin, repMax := 5, 10
		generated.RepMin = &repMin
		generated.RepMax = &repMax
	}
	return generated
}

// createMinimalExercise returns a basic exercise with just the essential fields populated.
func createMinimalExercise(name string) domain.Exercise {
	repMin, repMax := 5, 10
	return domain.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds is nil for non-time_based exercises.
		ID:                    -1,
		Name:                  name,
		Category:              domain.CategoryFullBody,
		ExerciseType:          domain.ExerciseTypeWeighted,
		DescriptionMarkdown:   fmt.Sprintf("# %s\n\nNo description available yet.", name),
		PrimaryMuscleGroups:   []string{},
		SecondaryMuscleGroups: []string{},
		RepMin:                &repMin,
		RepMax:                &repMax,
	}
}
```

Note: this plan listing shows the file shape. The middle block marked
`// (then: exerciseGenerator struct ...)` is filled by copying the full
text of `internal/workout/generator-exercise.go` lines 17-261 verbatim
(struct, constructor, four methods, all helper logic) and applying the
domain-prefix substitution listed there. Do not retype that block — copy
it whole and run a find-replace on the listed identifiers. The
substitutions are exhaustive:

| Old | New |
|---|---|
| `Exercise` (the type, not field/var names) | `domain.Exercise` |
| `Resource` | `domain.Resource` |
| `Category(` | `domain.Category(` |
| `ExerciseType(` | `domain.ExerciseType(` |
| `ExerciseTypeTime` | `domain.ExerciseTypeTime` |

Be careful: `Exercise` appears as a struct field/identifier name (e.g.
`exercise.Name`, `&Exercise{...}`) — only the type-name occurrences
need the prefix. The struct-literal `Exercise{` becomes `domain.Exercise{`.

- [ ] **Step 2: Create the test file**

Create `internal/service/exercise_generation_internal_test.go` by copying
`internal/workout/generator-exercise_internal_test.go` and:
- Changing `package workout` to `package service`.
- Changing `Category("lower")` to `domain.Category("lower")`.
- Adding `"github.com/myrjola/petrapp/internal/domain"` to the imports.

Full file contents:

```go
package service

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func TestExerciseGenerator_Generate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")
	if openaiAPIKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	muscleGroups := []string{"quadriceps", "glutes", "hamstrings", "calves", "core"}
	eg := newExerciseGenerator(openaiAPIKey, muscleGroups, testhelpers.NewLogger(testhelpers.NewWriter(t)))

	t.Run("Successful generation", func(t *testing.T) {
		exercise, err := eg.Generate(t.Context(), "Squat")

		if err != nil {
			t.Fatalf("Failed to generate exercise: %v", err)
		}

		if got, want := exercise.Name, "Squat"; got != want {
			t.Errorf("Got exercise name %q, want %q", got, want)
		}

		if got, want := exercise.Category, domain.Category("lower"); got != want {
			t.Errorf("Got exercise category %q, want %q", got, want)
		}

		if !strings.Contains(exercise.DescriptionMarkdown, "Squat") {
			t.Errorf("No 'Squat' in description %s", exercise.DescriptionMarkdown)
		}

		if !slices.Contains(exercise.PrimaryMuscleGroups, "quadriceps") {
			t.Errorf("Primary muscle groups %v does not contain 'quadriceps'", exercise.PrimaryMuscleGroups)
		}

		if !slices.Contains(exercise.SecondaryMuscleGroups, "core") {
			t.Errorf("Secondary muscle groups %v does not contain 'core'", exercise.SecondaryMuscleGroups)
		}
	})
}
```

Note: this test file references `(*Service)` only indirectly (via
`newExerciseGenerator`, an unexported function). For it to compile, the
new package needs the `Service` struct + `repos` field defined. We
haven't created those yet. Add a temporary minimal `Service` struct to
`internal/service/service.go` so this task compiles in isolation.

- [ ] **Step 3: Add a temporary minimal `Service` struct**

Replace `internal/service/service.go` (currently just the package
declaration) with the minimal struct and constructor that supports
exercise generation only. The full struct/ctor/preferences will land in
Task 3.

```go
// Package service holds workout orchestration: cross-aggregate coordination,
// external integrations (OpenAI, GDPR export), and the methods called by
// HTTP handlers. Pure rules live in internal/domain; persistence lives in
// internal/repository. This package depends on both.
package service

import (
	"log/slog"

	"github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Service coordinates workout-domain operations across the repository
// layer and external integrations. One instance per process; safe for
// concurrent use because each method opens its own DB transaction.
type Service struct {
	repos        *repository.Repositories
	db           *sqlite.Database
	logger       *slog.Logger
	openaiAPIKey string
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	return &Service{
		repos:        repository.New(db, logger),
		db:           db,
		logger:       logger,
		openaiAPIKey: openaiAPIKey,
	}
}
```

- [ ] **Step 4: Rewire `internal/workout/service.go` `GenerateExercise` to delegate**

The old `GenerateExercise`/`generateExerciseContent`/`createMinimalExercise`
trio in `internal/workout/service.go` (lines 585-647) must go away —
they live in `internal/service` now. The shortest correct change for
this task: turn `workout.Service.GenerateExercise` into a one-liner that
constructs an inline `*service.Service` from the same fields and
delegates. But that's wasteful — the cleaner move is to keep
`workout.Service` holding a `*service.Service` field for the duration of
this task, then drop the indirection in Task 3.

Make the workout package's service hold a `*service.Service`:

Edit `internal/workout/service.go` lines 1-33 (the imports, struct, and
constructor). The replacement:

```go
package workout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	repo "github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Service handles the business logic for workout management.
type Service struct {
	repos        *repo.Repositories
	db           *sqlite.Database
	logger       *slog.Logger
	openaiAPIKey string
	gen          *service.Service // delegate for relocated AI-generation methods (Task 2 transitional)
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	return &Service{
		repos:        repo.New(db, logger),
		db:           db,
		logger:       logger,
		openaiAPIKey: openaiAPIKey,
		gen:          service.NewService(db, logger, openaiAPIKey),
	}
}
```

Then replace `GenerateExercise`, `generateExerciseContent`, and
`createMinimalExercise` (lines 585-647) with a single delegating method:

```go
// GenerateExercise delegates to internal/service. Phase 3 transitional:
// removed in Task 3 when the rest of the service layer relocates and
// internal/workout.Service becomes a type alias.
func (s *Service) GenerateExercise(ctx context.Context, name string) (Exercise, error) {
	return s.gen.GenerateExercise(ctx, name)
}
```

- [ ] **Step 5: Delete `internal/workout/generator-exercise.go`**

```bash
git rm internal/workout/generator-exercise.go
```

The file is gone but the `exerciseJSONSchema` it depended on still lives
in `models.go` for one more step.

- [ ] **Step 6: Delete `internal/workout/generator-exercise_internal_test.go`**

```bash
git rm internal/workout/generator-exercise_internal_test.go
```

- [ ] **Step 7: Delete `exerciseJSONSchema` from `internal/workout/models.go`**

Edit `internal/workout/models.go`. Remove lines 84-154 (the
`exerciseJSONSchema` struct and its `MarshalJSON` method, plus the
two-line comment block above the struct that introduces it). The
preceding `SwapSimilarityScore` function at line 80 stays.

After the edit, the bottom of `models.go` reads:

```go
// SwapSimilarityScore is re-exported from internal/domain. Handlers call
// workout.SwapSimilarityScore today; that import path keeps working through
// this phase.
func SwapSimilarityScore(current, candidate Exercise) int {
	return domain.SwapSimilarityScore(current, candidate)
}
```

The file ends after the `SwapSimilarityScore` function's closing brace.
Total file length drops from 154 to ~83 lines.

The two unused imports (`encoding/json`, `fmt`) must be removed from the
top of the file. Inspect the trimmed file's `import` block — only the
`domain` import remains.

After the trim, `internal/workout/models.go`'s import block reads:

```go
import (
	"github.com/myrjola/petrapp/internal/domain"
)
```

- [ ] **Step 8: Build and lint**

Run: `make lint-fix`
Expected: no output (success). If `goimports` or `wrapcheck` complains,
fix per the message — common issue is the `domain` import block being
formatted differently in the trimmed `models.go`.

Run: `go build ./...`
Expected: no output.

- [ ] **Step 9: Run tests**

Run: `make test`
Expected: PASS for every package. The `TestExerciseGenerator_Generate`
test runs in `internal/service` now (skipped without `OPENAI_API_KEY`,
matching the previous behavior). All `internal/workout` orchestration
tests still pass — `GenerateExercise` is exercised through the workout
shim which now forwards to `service`.

- [ ] **Step 10: Commit**

```bash
git add internal/service/exercise_generation.go \
        internal/service/exercise_generation_internal_test.go \
        internal/service/service.go \
        internal/workout/service.go \
        internal/workout/models.go
git rm internal/workout/generator-exercise.go \
       internal/workout/generator-exercise_internal_test.go
git commit -m "Move AI exercise generation into internal/service/ (Phase 3 step 1/3)"
```

---

### Task 3: Move the rest of `service.go` into `internal/service/`

This task moves all remaining service methods out of
`internal/workout/service.go` into the eight responsibility-named files
in `internal/service/`, and atomically replaces
`internal/workout/service.go` with the type-alias shim. After the task,
`workout.Service` is `service.Service`, and the temporary `gen` field
from Task 2 is gone.

The methods being moved (from `internal/workout/service.go` line ranges):

| Source lines | Method | Destination |
|---|---|---|
| 26-33 (NewService) | superseded by `internal/service/service.go` | `service.go` (already there) |
| 36-50 | `GetUserPreferences`, `SaveUserPreferences` | `service.go` |
| 60-126, 130-154, 172-179 | `RegenerateWeeklyPlanIfUnstarted`, `ResolveWeeklySchedule`, `generateWeeklyPlan`, `mondayOf` | `sessions.go` |
| 157-163 | `GetSession` | `sessions.go` |
| 182-211 | `StartSession` | `sessions.go` |
| 214-221 | `CompleteSession` | `sessions.go` |
| 224-231 | `SaveFeedback` | `sessions.go` |
| 234-245 | `MarkWarmupComplete` | `sessions.go` |
| 248-261 | `UpdateSetWeight` | `sets.go` |
| 264-277 | `UpdateCompletedValue` | `sets.go` |
| 281-296 | `RecordSet` | `sets.go` |
| 299-314 | `List`, `GetExercise` | `exercises.go` |
| 317-334 | `GetSessionsWithExerciseSince` | `reporting.go` |
| 337-366 | `GetExerciseSetsForExerciseSince` | `reporting.go` |
| 376-440 | `GetStartingWeight`, `GetStartingSeconds` | `progression.go` |
| 444-539 | `BuildProgression`, `BuildTimedProgression` | `progression.go` |
| 542-550 | `UpdateExercise` | `exercises.go` |
| 553-559 | `ListMuscleGroups` | `exercises.go` |
| 566-579 | `WeeklyMuscleGroupVolume` | `reporting.go` |
| 656-680 | `SwapExercise` | `exercises.go` |
| 686-713 | `findHistoricalSets` | `exercises.go` |
| 719-731 | `copySetsWithoutCompletion` | `exercises.go` |
| 742-776 | `buildSetsForAdd` | `exercises.go` |
| 779-795 | `FindCompatibleExercises` | `exercises.go` |
| 801-843 | `AddExercise` | `exercises.go` |
| 846-880 | `GetFeatureFlag`, `IsMaintenanceModeEnabled`, `ListFeatureFlags`, `SetFeatureFlag` | `feature_flags.go` |
| 884-899 | `ExportUserData` | `export.go` |

**Files:**
- Create: `internal/service/sessions.go`
- Create: `internal/service/sets.go`
- Create: `internal/service/exercises.go`
- Create: `internal/service/progression.go`
- Create: `internal/service/reporting.go`
- Create: `internal/service/feature_flags.go`
- Create: `internal/service/export.go`
- Modify: `internal/service/service.go` (add the preferences methods)
- Modify: `internal/workout/service.go` (replace body with shim)

For every relocated method, the body is copied verbatim with these
mechanical adjustments:
1. Receiver changes from `(s *Service)` to `(s *Service)` (no change —
   the type lives in `package service` now).
2. Domain-typed parameters and return types: `Session` → `domain.Session`,
   `Exercise` → `domain.Exercise`, `Set` → `domain.Set`,
   `ExerciseSet` → `domain.ExerciseSet`,
   `ExerciseProgress` → `domain.ExerciseProgress`,
   `ExerciseProgressEntry` → `domain.ExerciseProgressEntry`,
   `Preferences` → `domain.Preferences`,
   `FeatureFlag` → `domain.FeatureFlag`,
   `Signal` → `domain.Signal`,
   `PeriodizationType` → `domain.PeriodizationType`,
   `MuscleGroupVolume` → `domain.MuscleGroupVolume`.
3. Domain-typed constants: `ExerciseTypeBodyweight` →
   `domain.ExerciseTypeBodyweight` (and the other `ExerciseType*`
   variants), `CategoryFullBody` → `domain.CategoryFullBody`,
   `ExerciseTypeWeighted` → `domain.ExerciseTypeWeighted`.
4. Sentinel errors and helpers used inside bodies: `ErrNotFound` is only
   referenced via `domain.ErrNotFound` already (the `workout.ErrNotFound`
   alias exists for `cmd/web` callers but the service body always uses
   the domain package directly). `domain.NewPlanner`, `domain.Config`,
   `domain.Progression`, `domain.NewFromHistory`, `domain.SetResult`,
   `domain.TimedConfig`, `domain.TimedProgression`, `domain.NewTimedFromHistory`,
   `domain.TimedSetResult`, `domain.DeriveScheme`, `domain.ConvertWeight`,
   `domain.WeeklyMuscleGroupVolume`, `domain.BuildPlannedSets`,
   `domain.ErrAlreadyStarted` — already prefixed in the existing source,
   no change needed.
5. Repository import is renamed: every `repo.Foo` → `repository.Foo`.
   In practice, the only place this surfaces in the receiver bodies is
   indirectly through `s.repos` (the field type was `*repo.Repositories`;
   in the new package the field type is `*repository.Repositories`,
   already declared in Task 2's minimal `service.go`).
6. The `contexthelpers` import stays (used by `ExportUserData`).

The full file contents follow. Each file's import block lists exactly
the packages used by the bodies that file holds; no broader import set.
The bodies are verbatim copies from the line ranges in the table above
with only the substitutions listed.

- [ ] **Step 1: Update `internal/service/service.go` to hold the preferences methods**

Replace `internal/service/service.go` (the minimal struct+ctor scaffolded
in Task 2) with the final shell:

```go
// Package service holds workout orchestration: cross-aggregate coordination,
// external integrations (OpenAI, GDPR export), and the methods called by
// HTTP handlers. Pure rules live in internal/domain; persistence lives in
// internal/repository. This package depends on both.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Service coordinates workout-domain operations across the repository
// layer and external integrations. One instance per process; safe for
// concurrent use because each method opens its own DB transaction.
type Service struct {
	repos        *repository.Repositories
	db           *sqlite.Database
	logger       *slog.Logger
	openaiAPIKey string
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	return &Service{
		repos:        repository.New(db, logger),
		db:           db,
		logger:       logger,
		openaiAPIKey: openaiAPIKey,
	}
}

// GetUserPreferences retrieves the workout preferences for a user.
func (s *Service) GetUserPreferences(ctx context.Context) (domain.Preferences, error) {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return domain.Preferences{}, fmt.Errorf("get user preferences: %w", err)
	}
	return prefs, nil
}

// SaveUserPreferences saves the workout preferences for a user.
func (s *Service) SaveUserPreferences(ctx context.Context, prefs domain.Preferences) error {
	if err := s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save user preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Create `internal/service/sessions.go`**

Copy the bodies of `RegenerateWeeklyPlanIfUnstarted`, `ResolveWeeklySchedule`,
`generateWeeklyPlan`, `GetSession`, `mondayOf`, `StartSession`,
`CompleteSession`, `SaveFeedback`, `MarkWarmupComplete` from
`internal/workout/service.go` (lines listed in the table above) verbatim
with the substitutions also listed above.

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

// RegenerateWeeklyPlanIfUnstarted replaces the current week's generated plan with one
// that reflects the latest preferences, but only when no session has been started yet.
// If any workout this week has a non-zero StartedAt the existing plan is left intact.
//
// The delete and generate steps are not wrapped in a single transaction. If the process
// fails between the two, the week is left with no sessions. This is self-healing: the
// next call to ResolveWeeklySchedule (e.g. on the home page redirect) detects zero
// sessions and regenerates automatically.
func (s *Service) RegenerateWeeklyPlanIfUnstarted(ctx context.Context) error {
	monday := mondayOf(time.Now())
	sunday := monday.AddDate(0, 0, 6)

	existing, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return fmt.Errorf("list sessions for week: %w", err)
	}

	for _, sess := range existing {
		if !sess.Date.After(sunday) && !sess.StartedAt.IsZero() {
			return nil
		}
	}

	if err = s.repos.Sessions.DeleteWeek(ctx, monday); err != nil {
		return fmt.Errorf("delete current week: %w", err)
	}
	if err = s.generateWeeklyPlan(ctx, monday); err != nil {
		return fmt.Errorf("generate weekly plan: %w", err)
	}
	return nil
}

// ResolveWeeklySchedule retrieves the workout schedule for the current week.
// If no sessions exist for the week, it generates all scheduled days at once using
// the weekly planner and persists them in a single transaction.
func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]domain.Session, error) {
	monday := mondayOf(time.Now())
	sunday := monday.AddDate(0, 0, 6)

	existing, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return nil, fmt.Errorf("list sessions for week: %w", err)
	}
	thisWeekCount := 0
	for _, sess := range existing {
		if !sess.Date.After(sunday) {
			thisWeekCount++
		}
	}

	if thisWeekCount == 0 {
		if err = s.generateWeeklyPlan(ctx, monday); err != nil {
			return nil, fmt.Errorf("generate weekly plan: %w", err)
		}
	}

	workouts := make([]domain.Session, 7)
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		sess, getErr := s.repos.Sessions.Get(ctx, day)
		if getErr != nil && !errors.Is(getErr, domain.ErrNotFound) {
			return nil, fmt.Errorf("get session %s: %w", day.Format(time.DateOnly), getErr)
		}
		if errors.Is(getErr, domain.ErrNotFound) {
			workouts[i] = domain.Session{ //nolint:exhaustruct // Rest days have no exercise data.
				Date: day,
			}
			continue
		}
		workouts[i] = sess
	}
	return workouts, nil
}

// generateWeeklyPlan uses the domain planner to create all sessions for the week starting
// on monday and persists them in a single DB transaction.
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

	if err = s.repos.Sessions.CreateBatch(ctx, plannedSessions); err != nil {
		return fmt.Errorf("create batch sessions: %w", err)
	}
	return nil
}

// GetSession retrieves a workout session for a specific date.
func (s *Service) GetSession(ctx context.Context, date time.Time) (domain.Session, error) {
	sess, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return domain.Session{}, fmt.Errorf("get session %s: %w", date.Format(time.DateOnly), err)
	}
	return sess, nil
}

// mondayOf returns the Monday of the week containing date as midnight UTC. The
// calendar date is taken from date's location so the user's local week boundary
// is preserved, but the result is anchored to UTC so it compares cleanly against
// session dates loaded from the database (which time.Parse always returns in
// UTC). Time.Truncate is unsafe here because it rounds to UTC-midnight
// boundaries from an absolute instant, which can roll local-timezone times back
// into the previous calendar day.
func mondayOf(date time.Time) time.Time {
	y, m, d := date.Date()
	offset := int(time.Monday - date.Weekday())
	if offset > 0 {
		offset = -6
	}
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, offset)
}

// StartSession starts a new workout session.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	monday := mondayOf(date)
	existing, listErr := s.repos.Sessions.List(ctx, monday)
	if listErr != nil {
		return fmt.Errorf("list sessions for week of %s: %w", date.Format(time.DateOnly), listErr)
	}
	sunday := monday.AddDate(0, 0, 6)
	weekCount := 0
	for _, sess := range existing {
		if !sess.Date.After(sunday) {
			weekCount++
		}
	}
	if weekCount == 0 {
		if genErr := s.generateWeeklyPlan(ctx, monday); genErr != nil {
			return fmt.Errorf("generate weekly plan for %s: %w", date.Format(time.DateOnly), genErr)
		}
	}

	err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
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

// CompleteSession marks a workout session as completed.
func (s *Service) CompleteSession(ctx context.Context, date time.Time) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.Complete(time.Now())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// SaveFeedback saves the difficulty rating for a completed workout session.
func (s *Service) SaveFeedback(ctx context.Context, date time.Time, difficulty int) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.SetDifficulty(difficulty)
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// MarkWarmupComplete marks the warmup as complete for a specific workout exercise slot.
func (s *Service) MarkWarmupComplete(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.MarkWarmupComplete(workoutExerciseID, time.Now().UTC())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}
```

- [ ] **Step 3: Create `internal/service/sets.go`**

```go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

// UpdateSetWeight updates the weight for a specific set in a workout.
func (s *Service) UpdateSetWeight(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	newWeight float64,
) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.UpdateSetWeight(workoutExerciseID, setIndex, newWeight)
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// UpdateCompletedValue updates a previously completed set with new value (reps or seconds).
func (s *Service) UpdateCompletedValue(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	completedValue int,
) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.UpdateCompletedValue(workoutExerciseID, setIndex, completedValue, time.Now().UTC())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// RecordSet atomically persists the signal, weight (nil for time-based sets),
// completed value (reps or seconds depending on exercise type), and timestamp.
func (s *Service) RecordSet(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	signal domain.Signal,
	weightKg *float64,
	completedValue int,
) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.RecordSet(workoutExerciseID, setIndex, signal, weightKg, completedValue, time.Now().UTC())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}
```

- [ ] **Step 4: Create `internal/service/exercises.go`**

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

// List returns all available exercises.
func (s *Service) List(ctx context.Context) ([]domain.Exercise, error) {
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list exercises: %w", err)
	}
	return exercises, nil
}

// GetExercise retrieves a specific exercise by ID.
func (s *Service) GetExercise(ctx context.Context, id int) (domain.Exercise, error) {
	exercise, err := s.repos.Exercises.Get(ctx, id)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("get exercise: %w", err)
	}
	return exercise, nil
}

// UpdateExercise updates an existing exercise.
func (s *Service) UpdateExercise(ctx context.Context, ex domain.Exercise) error {
	if err := s.repos.Exercises.Update(ctx, ex.ID, func(oldEx *domain.Exercise) error {
		*oldEx = ex
		return nil
	}); err != nil {
		return fmt.Errorf("update exercise: %w", err)
	}
	return nil
}

// ListMuscleGroups retrieves all available muscle groups.
func (s *Service) ListMuscleGroups(ctx context.Context) ([]string, error) {
	groups, err := s.repos.Exercises.ListMuscleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle groups: %w", err)
	}
	return groups, nil
}

// SwapExercise replaces the exercise occupying a workout slot (identified by
// workoutExerciseID) with newExerciseID. The workout slot's stable ID is
// preserved so URLs targeting the slot keep working.
//
// Sets recorded against the old exercise are dropped — replaced with historical
// data for the new exercise when available, otherwise empty placeholders matching
// the old set count.
func (s *Service) SwapExercise(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	newExerciseID int,
) error {
	newExercise, err := s.repos.Exercises.Get(ctx, newExerciseID)
	if err != nil {
		return fmt.Errorf("get new exercise: %w", err)
	}

	historicalSets, err := s.findHistoricalSets(ctx, date, newExerciseID)
	if err != nil {
		return fmt.Errorf("find historical sets: %w", err)
	}

	err = s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		newSets := s.buildSetsForAdd(newExercise, sess.PeriodizationType, historicalSets)
		return sess.SwapExerciseInSlot(workoutExerciseID, newExercise, newSets)
	})
	if err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// findHistoricalSets retrieves set data from the most recent usage of an exercise.
// Aggregates with no sets are skipped — they exist for exercises whose historical
// exercise_sets rows were dropped by the time-based premigration but whose
// workout_exercise slot survived. Returns nil when no usable history is found.
func (s *Service) findHistoricalSets(ctx context.Context, date time.Time, exerciseID int) ([]domain.Set, error) {
	threeMonthsAgo := date.AddDate(0, -3, 0)
	history, err := s.repos.Sessions.List(ctx, threeMonthsAgo)
	if err != nil {
		return nil, fmt.Errorf("get workout history: %w", err)
	}

	for _, session := range history {
		if session.Date.Equal(date) {
			continue
		}

		for _, exerciseSet := range session.ExerciseSets {
			if exerciseSet.Exercise.ID != exerciseID || len(exerciseSet.Sets) == 0 {
				continue
			}
			return s.copySetsWithoutCompletion(exerciseSet.Sets), nil
		}
	}

	return nil, nil
}

// copySetsWithoutCompletion creates a copy of sets with completion reset to nil.
// Note: callers in the AddExercise/swap paths route the result through
// buildSetsForAdd, which overrides TargetValue from the session's periodization.
// This function preserves all fields verbatim including TargetValue.
func (s *Service) copySetsWithoutCompletion(sets []domain.Set) []domain.Set {
	result := make([]domain.Set, len(sets))
	for i, set := range sets {
		result[i] = domain.Set{
			WeightKg:       set.WeightKg,
			TargetValue:    set.TargetValue,
			CompletedValue: nil,
			CompletedAt:    nil,
			Signal:         nil,
		}
	}
	return result
}

// buildSetsForAdd produces the Set slice for an exercise being added to or
// swapping into an existing session. The session's periodization always
// dictates TargetValue and TargetSets (so a Deadlift added in a Strength
// week gets 3 reps × 4 sets, not whatever the historical session had).
//
// When historicalSets is non-nil and contains weight data, the most recent
// completed weight is preserved as the starting weight for every new set —
// the user's progression isn't lost just because the prescription changed.
// Completion fields are always reset.
func (s *Service) buildSetsForAdd(ex domain.Exercise, pt domain.PeriodizationType, historicalSets []domain.Set) []domain.Set {
	sets := domain.BuildPlannedSets(ex, pt)
	// Allocate empty weight pointers for weighted/assisted exercises. The
	// form input on the per-set page binds to *float64; nil would render
	// as "no weight" instead of an empty editable input. Bodyweight and
	// time-based stay nil.
	if !ex.IsTimed() && ex.ExerciseType != domain.ExerciseTypeBodyweight {
		for i := range sets {
			sets[i].WeightKg = new(float64)
		}
	}
	if len(historicalSets) == 0 {
		return sets
	}
	var seedWeight *float64
	for i := len(historicalSets) - 1; i >= 0; i-- {
		if historicalSets[i].WeightKg != nil {
			seedWeight = historicalSets[i].WeightKg
			break
		}
	}
	if seedWeight == nil {
		return sets
	}
	for i := range sets {
		if !ex.IsTimed() && ex.ExerciseType != domain.ExerciseTypeBodyweight {
			w := *seedWeight
			sets[i].WeightKg = &w
		}
	}
	return sets
}

// FindCompatibleExercises returns all exercises except the specified one.
func (s *Service) FindCompatibleExercises(ctx context.Context, exerciseID int) ([]domain.Exercise, error) {
	allExercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all exercises: %w", err)
	}

	var otherExercises []domain.Exercise
	for _, exercise := range allExercises {
		if exercise.ID != exerciseID {
			otherExercises = append(otherExercises, exercise)
		}
	}

	return otherExercises, nil
}

// AddExercise adds a new exercise to an existing workout session.
// It will retrieve historical weight data if available. Returns the
// workout_exercise.id assigned to the new slot, so callers can build URLs
// that point at the new exercise's detail page.
func (s *Service) AddExercise(ctx context.Context, date time.Time, exerciseID int) (int, error) {
	exercise, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("get exercise: %w", err)
	}

	historicalSets, err := s.findHistoricalSets(ctx, date, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("find historical sets: %w", err)
	}

	if _, err = s.repos.Sessions.Get(ctx, date); errors.Is(err, domain.ErrNotFound) {
		return 0, fmt.Errorf("workout session for date %s does not exist", date.Format(time.DateOnly))
	} else if err != nil {
		return 0, fmt.Errorf("check session existence: %w", err)
	}

	err = s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		newSets := s.buildSetsForAdd(exercise, sess.PeriodizationType, historicalSets)
		_, addErr := sess.AddExercise(exercise, newSets)
		if addErr != nil {
			return fmt.Errorf("add exercise to session: %w", addErr)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("update session with new exercise: %w", err)
	}

	updated, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return 0, fmt.Errorf("re-fetch session after add: %w", err)
	}
	for _, es := range updated.ExerciseSets {
		if es.Exercise.ID == exerciseID {
			return es.ID, nil
		}
	}
	return 0, fmt.Errorf("added exercise %d not found in session %s", exerciseID, date.Format(time.DateOnly))
}
```

- [ ] **Step 5: Create `internal/service/progression.go`**

```go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

// GetStartingWeight returns the weight to seed a new session for the given exercise.
// It pulls the latest successful set (completed and not signaled too heavy) from
// the most recent qualifying session strictly before beforeDate, then converts the
// load via Epley 1RM-equivalence when that session's periodization differs from
// targetType so the relative intensity carries across rep schemes (e.g. 100 kg x5
// strength → ~92 kg x8 hypertrophy). Using a cutoff keeps the starting weight
// stable when earlier sets of beforeDate's session are edited. Returns 0 if no
// successful history exists.
func (s *Service) GetStartingWeight(
	ctx context.Context,
	exerciseID int,
	beforeDate time.Time,
	targetType domain.PeriodizationType,
) (float64, error) {
	prev, err := s.repos.Sessions.GetLatestStartingWeightBefore(ctx, exerciseID, beforeDate)
	if err != nil {
		return 0, fmt.Errorf("get latest starting weight: %w", err)
	}
	if prev.PeriodizationType == "" || prev.PeriodizationType == targetType {
		return prev.WeightKg, nil
	}
	exercise, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("get exercise for rep window: %w", err)
	}
	if exercise.RepMin == nil || exercise.RepMax == nil {
		// time-based exercises don't carry a rep window and shouldn't reach
		// this path (their starting value is seconds via GetStartingSeconds);
		// defensive return preserves the historical weight unchanged.
		return prev.WeightKg, nil
	}
	fromReps := domain.DeriveScheme(
		*exercise.RepMin, *exercise.RepMax,
		prev.PeriodizationType,
	).TargetReps
	toReps := domain.DeriveScheme(
		*exercise.RepMin, *exercise.RepMax,
		targetType,
	).TargetReps
	return domain.ConvertWeight(prev.WeightKg, fromReps, toReps), nil
}

// GetStartingSeconds returns the seconds target to seed a new session for
// the given time-based exercise. Pulls the latest successful set's
// completed_value from sessions strictly before beforeDate; falls back to
// the exercise's DefaultStartingSeconds when no successful history exists.
// Returns an error if the exercise is not time_based, if the lookup fails,
// or if a time_based exercise has no DefaultStartingSeconds (which is a
// fixture/data invariant violation since the schema CHECK requires it).
func (s *Service) GetStartingSeconds(
	ctx context.Context,
	exerciseID int,
	beforeDate time.Time,
) (int, error) {
	exercise, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("get exercise: %w", err)
	}
	if !exercise.IsTimed() {
		return 0, fmt.Errorf("exercise %d is not time_based", exerciseID)
	}
	seconds, err := s.repos.Sessions.GetLatestSuccessfulSecondsBefore(ctx, exerciseID, beforeDate)
	if err != nil {
		return 0, fmt.Errorf("get latest successful seconds: %w", err)
	}
	if seconds > 0 {
		return seconds, nil
	}
	if exercise.DefaultStartingSeconds == nil {
		return 0, fmt.Errorf("time_based exercise %d has no default_starting_seconds", exerciseID)
	}
	return *exercise.DefaultStartingSeconds, nil
}

// BuildProgression constructs a domain.Progression for the given exercise
// in the given session, ready to call CurrentSet() for the next set recommendation.
func (s *Service) BuildProgression(
	ctx context.Context,
	date time.Time,
	exerciseID int,
) (*domain.Progression, error) {
	sess, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	exercise, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return nil, fmt.Errorf("get exercise: %w", err)
	}
	if exercise.RepMin == nil || exercise.RepMax == nil {
		return nil, fmt.Errorf("exercise %d has no rep window (use BuildTimedProgression for time_based)", exerciseID)
	}

	startingWeight, err := s.GetStartingWeight(ctx, exerciseID, date, sess.PeriodizationType)
	if err != nil {
		return nil, fmt.Errorf("get starting weight: %w", err)
	}

	config := domain.Config{
		Type:           sess.PeriodizationType,
		RepMin:         *exercise.RepMin,
		RepMax:         *exercise.RepMax,
		StartingWeight: startingWeight,
	}

	var completed []domain.SetResult
	for _, es := range sess.ExerciseSets {
		if es.Exercise.ID != exerciseID {
			continue
		}
		for _, set := range es.Sets {
			if set.CompletedValue == nil || set.Signal == nil {
				continue
			}
			var kg float64
			if set.WeightKg != nil {
				kg = *set.WeightKg
			}
			completed = append(completed, domain.SetResult{
				ActualReps: *set.CompletedValue,
				Signal:     *set.Signal,
				WeightKg:   kg,
			})
		}
		break
	}

	return domain.NewFromHistory(config, completed), nil
}

// BuildTimedProgression constructs a domain.TimedProgression
// for the given time-based exercise in the given session, ready to call
// CurrentSet() for the next hold's recommendation. Returns an error if the
// exercise is not time_based or if the lookup fails.
func (s *Service) BuildTimedProgression(
	ctx context.Context,
	date time.Time,
	exerciseID int,
) (*domain.TimedProgression, error) {
	starting, err := s.GetStartingSeconds(ctx, exerciseID, date)
	if err != nil {
		return nil, fmt.Errorf("get starting seconds: %w", err)
	}

	sess, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	var completed []domain.TimedSetResult
	for _, es := range sess.ExerciseSets {
		if es.Exercise.ID != exerciseID {
			continue
		}
		for _, set := range es.Sets {
			if set.CompletedValue == nil || set.Signal == nil {
				continue
			}
			completed = append(completed, domain.TimedSetResult{
				ActualSeconds: *set.CompletedValue,
				Signal:        *set.Signal,
			})
		}
		break
	}

	return domain.NewTimedFromHistory(
		domain.TimedConfig{StartingSeconds: starting},
		completed,
	), nil
}
```

- [ ] **Step 6: Create `internal/service/reporting.go`**

```go
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

// GetSessionsWithExerciseSince retrieves all sessions since a given date that contain the specified exercise.
func (s *Service) GetSessionsWithExerciseSince(ctx context.Context, exerciseID int, since time.Time) (
	[]domain.Session, error,
) {
	sessions, err := s.repos.Sessions.List(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}

	var result []domain.Session
	for _, session := range sessions {
		for _, es := range session.ExerciseSets {
			if es.Exercise.ID == exerciseID {
				result = append(result, session)
				break
			}
		}
	}
	return result, nil
}

// GetExerciseSetsForExerciseSince retrieves all sets for a specific exercise since a given date.
func (s *Service) GetExerciseSetsForExerciseSince(ctx context.Context, exerciseID int, since time.Time) (
	domain.ExerciseProgress, error,
) {
	histories, err := s.repos.Sessions.ListSetsForExerciseSince(ctx, exerciseID, since)
	if err != nil {
		return domain.ExerciseProgress{}, fmt.Errorf("list sets for exercise: %w", err)
	}

	ex, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return domain.ExerciseProgress{}, fmt.Errorf("get exercise %d: %w", exerciseID, err)
	}

	entries := make([]domain.ExerciseProgressEntry, 0, len(histories))
	for _, h := range histories {
		var completedSets []domain.Set
		for _, set := range h.Sets {
			if set.CompletedValue != nil {
				completedSets = append(completedSets, set)
			}
		}
		if len(completedSets) > 0 {
			entries = append(entries, domain.ExerciseProgressEntry{
				Date: h.Date,
				Sets: completedSets,
			})
		}
	}

	return domain.ExerciseProgress{Exercise: ex, Entries: entries}, nil
}

// WeeklyMuscleGroupVolume aggregates planned-vs-completed weekly load per muscle
// group across the supplied sessions. One entry is returned for every known
// muscle group, sorted alphabetically; groups with no contributions appear as
// zero-load rows so the UI can render them without a separate query. Targets are
// joined from muscle_group_weekly_targets; untracked groups carry TargetSets = 0.
func (s *Service) WeeklyMuscleGroupVolume(
	ctx context.Context,
	sessions []domain.Session,
) ([]domain.MuscleGroupVolume, error) {
	groupNames, err := s.repos.Exercises.ListMuscleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle groups: %w", err)
	}
	targets, err := s.repos.MuscleTargets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle group targets: %w", err)
	}
	return domain.WeeklyMuscleGroupVolume(sessions, targets, groupNames), nil
}
```

- [ ] **Step 7: Create `internal/service/feature_flags.go`**

```go
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/myrjola/petrapp/internal/domain"
)

// GetFeatureFlag retrieves a feature flag by name.
func (s *Service) GetFeatureFlag(ctx context.Context, name string) (domain.FeatureFlag, error) {
	flag, err := s.repos.FeatureFlags.Get(ctx, name)
	if err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("get feature flag %s: %w", name, err)
	}
	return flag, nil
}

// IsMaintenanceModeEnabled checks if maintenance mode is enabled.
func (s *Service) IsMaintenanceModeEnabled(ctx context.Context) bool {
	flag, err := s.repos.FeatureFlags.Get(ctx, "maintenance_mode")
	if err != nil {
		// If we can't check the flag, assume maintenance is disabled for safety.
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to check maintenance mode flag", slog.Any("error", err))
		return false
	}
	return flag.Enabled
}

// ListFeatureFlags retrieves all feature flags.
func (s *Service) ListFeatureFlags(ctx context.Context) ([]domain.FeatureFlag, error) {
	flags, err := s.repos.FeatureFlags.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feature flags: %w", err)
	}
	return flags, nil
}

// SetFeatureFlag updates or creates a feature flag.
func (s *Service) SetFeatureFlag(ctx context.Context, flag domain.FeatureFlag) error {
	if err := s.repos.FeatureFlags.Set(ctx, flag); err != nil {
		return fmt.Errorf("set feature flag %s: %w", flag.Name, err)
	}
	return nil
}
```

- [ ] **Step 8: Create `internal/service/export.go`**

```go
package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/myrjola/petrapp/internal/contexthelpers"
)

// ExportUserData creates an SQLite database export containing all data for the authenticated user.
// This method is intended for GDPR compliance and allows users to download their complete data.
func (s *Service) ExportUserData(ctx context.Context) (string, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return "", errors.New("no authenticated user found in context")
	}

	tempDir := os.TempDir()

	exportPath, err := s.db.CreateUserDB(ctx, userID, tempDir)
	if err != nil {
		return "", fmt.Errorf("create user database: %w", err)
	}

	return exportPath, nil
}
```

- [ ] **Step 9: Replace `internal/workout/service.go` with the alias shim**

The workout package's service is now a paper-thin re-export. Replace
the entire file contents with:

```go
// Package workout is a backward-compat shim for cmd/web through Phase 4
// of the workout-service rearchitecture. The Service type and the
// NewService constructor live in internal/service; this package
// re-exports them so handlers can keep importing "workout" without
// edits. The type aliases in models.go cover the rest of the public
// surface (domain types, sentinel errors, helper functions).
package workout

import (
	"log/slog"

	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Service is the workout orchestration entry point. The implementation
// lives in internal/service; this alias exists so that
// cmd/web/main.go's `workout.Service` field type keeps resolving. Phase
// 4 will rename the field to reference internal/service directly and
// delete this package.
type Service = service.Service

// NewService creates a new workout service. It delegates to
// service.NewService.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	return service.NewService(db, logger, openaiAPIKey)
}
```

The `gen` field added in Task 2 is gone — the `Service` type is now a
direct alias, so there's no struct in `package workout` to hold a
field. All method calls through `workout.Service` resolve to methods on
`service.Service`, which are now defined across the eight files in
`internal/service/`.

- [ ] **Step 10: Build**

Run: `go build ./...`
Expected: no output. The compilation graph: `cmd/web` →
`internal/workout` (alias only) → `internal/service` →
{`internal/domain`, `internal/repository`, `internal/sqlite`,
`internal/contexthelpers`, OpenAI SDK}.

If the build fails with `undefined: workout.Foo`, search
`internal/workout/models.go` for the type — it should be an alias for
`domain.Foo`. If it's not, the type is a method on `Service` and the
forwarder above is wrong; find the missing method in
`internal/service/*.go`. If a build error mentions a method receiver
defined twice, an old file in `internal/workout` wasn't deleted.

- [ ] **Step 11: Lint-fix**

Run: `make lint-fix`
Expected: no output. Common fixes:
- Unused imports in the trimmed `internal/workout/service.go`
  (only `log/slog`, `internal/service`, `internal/sqlite` should
  remain).
- `wrapcheck` complaints if the alias shim's `NewService` was
  copied with `fmt.Errorf` wrappers — there should be none; the body is
  one statement.

- [ ] **Step 12: Run tests**

Run: `make test`
Expected: PASS. The existing `internal/workout/service_test.go` (still
in place, still in `package workout_test`) imports the workout package
and calls `workout.NewService` which now returns
`*service.Service` aliased as `*workout.Service`. All method calls
resolve through the alias.

If a test fails with a compile error like "cannot use *service.Service
as *workout.Service": the type alias works at the language level but
some test code may be using a struct literal to construct a Service. Go
type aliases make struct literals interchangeable, so this would only
fail if the test referenced an unexported field. Inspect the failing
test and fix accordingly.

- [ ] **Step 13: Commit**

```bash
git add internal/service/service.go \
        internal/service/sessions.go \
        internal/service/sets.go \
        internal/service/exercises.go \
        internal/service/progression.go \
        internal/service/reporting.go \
        internal/service/feature_flags.go \
        internal/service/export.go \
        internal/workout/service.go
git commit -m "Move service.go into internal/service/ split by responsibility (Phase 3 step 2/3)"
```

---

### Task 4: Move tests into `internal/service/`

This task relocates `service_test.go` (single file, 2,170 lines) and
`service_internal_test.go` (renamed to `sessions_internal_test.go`) into
the new package.

**Files:**
- Create: `internal/service/service_test.go`
- Create: `internal/service/sessions_internal_test.go`
- Delete: `internal/workout/service_test.go`
- Delete: `internal/workout/service_internal_test.go`

- [ ] **Step 1: Move `service_test.go`**

```bash
git mv internal/workout/service_test.go internal/service/service_test.go
```

The file's package declaration is `package workout_test` and references
`workout.Foo` types throughout. It must become `package service_test`
referencing `service.Foo` for the orchestrator type and
`workout.Foo` for the domain-typed aliases that handlers and fixtures
use. Wait — that's a question worth answering precisely.

The test file currently imports `"github.com/myrjola/petrapp/internal/workout"`
and uses these symbols (verified by
`grep -hoE "workout\.[A-Z][A-Za-z_]+" internal/workout/service_test.go | sort -u`):

| Reference | Resolves to |
|---|---|
| `workout.Service` | `service.Service` |
| `workout.NewService` | `service.NewService` |
| `workout.Session` | `domain.Session` |
| `workout.Exercise` | `domain.Exercise` |
| `workout.ExerciseSet` | `domain.ExerciseSet` |
| `workout.Set` | `domain.Set` |
| `workout.Preferences` | `domain.Preferences` |
| `workout.PeriodizationType` | `domain.PeriodizationType` |
| `workout.PeriodizationHypertrophy` | `domain.PeriodizationHypertrophy` |
| `workout.PeriodizationStrength` | `domain.PeriodizationStrength` |
| `workout.CategoryLower` | `domain.CategoryLower` |
| `workout.ExerciseTypeWeighted` | `domain.ExerciseTypeWeighted` |
| `workout.SignalOnTarget` | `domain.SignalOnTarget` |
| `workout.SignalTooLight` | `domain.SignalTooLight` |
| `workout.MuscleGroupVolume` | `domain.MuscleGroupVolume` |
| `workout.ErrNotFound` | `domain.ErrNotFound` |

After Task 3, `workout.Service` and `workout.NewService` resolve to the
service package; everything else still resolves through `workout`'s
type aliases. The cleanest move:

- Change `package workout_test` to `package service_test`.
- Replace the `workout` import with both `domain` and `service` imports.
- Apply the substitutions in the table above. The `workout` → `service`
  prefix applies only to `Service` and `NewService`; everything else
  maps `workout` → `domain`.
- Verify no other `workout.*` references remain.

Apply these edits to `internal/service/service_test.go`. The grep to
double-check after editing:

```bash
grep -nE "\bworkout\." internal/service/service_test.go
```

Expected output: empty.

- [ ] **Step 2: Verify `service_test.go` compiles**

Run: `go test -count=1 -run='^$' ./internal/service/`
Expected: ok (compile-only run; no tests match the empty pattern).

- [ ] **Step 3: Move `service_internal_test.go` and rename**

```bash
git mv internal/workout/service_internal_test.go internal/service/sessions_internal_test.go
```

Edit the file: change `package workout` to `package service`. The body
references only `mondayOf` (private function in `internal/service/sessions.go`)
and stdlib `time`/`testing`. No further edits needed.

Verify:

```bash
grep -nE "^package " internal/service/sessions_internal_test.go
```

Expected: `package service`.

- [ ] **Step 4: Run all internal-package tests in `internal/service/`**

Run: `go test -count=1 ./internal/service/`
Expected: PASS for every test (orchestration tests + the `mondayOf`
table tests). The OpenAI smoke test
`TestExerciseGenerator_Generate` skips without `OPENAI_API_KEY`.

- [ ] **Step 5: Run the full test suite**

Run: `make test`
Expected: PASS. The `internal/workout` package no longer has tests of
its own (the two test files moved); the workout package still compiles
because it has the alias shim plus `models.go`.

- [ ] **Step 6: Lint-fix**

Run: `make lint-fix`
Expected: no output. If lint complains about test imports (e.g.
`workout` imported but not used), fix per the message — typical issue
is stray `workout.` references missed in step 1.

- [ ] **Step 7: Commit**

```bash
git add internal/service/service_test.go \
        internal/service/sessions_internal_test.go
git rm internal/workout/service_test.go \
       internal/workout/service_internal_test.go
git commit -m "Move service tests into internal/service/ (Phase 3 step 3/3)"
```

---

### Task 5: Refresh `internal/workout/CLAUDE.md`

The workout package is now a single-file shim plus type aliases. Its
CLAUDE.md still describes Phase 2 state.

**Files:**
- Modify: `internal/workout/CLAUDE.md`

- [ ] **Step 1: Replace the file contents**

Replace `internal/workout/CLAUDE.md` with:

```markdown
# Workout Package — Migration Status

> **Migration in progress (Phases 1–3 of 4 complete as of 2026-05-10).**
> - Pure logic lives in `internal/domain/` (Phase 1).
> - Persistence lives in `internal/repository/` (Phase 2).
> - Orchestration lives in `internal/service/` (Phase 3).
> - This package is now a backward-compat shim that re-exports the
>   `Service` type and `NewService` constructor for `cmd/web` callers,
>   plus the type aliases that let handlers reference domain types as
>   `workout.Foo`. Phase 4 deletes the package entirely.
>
> See `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md`.

## What still lives here

- **`models.go`** — type aliases (`type Session = domain.Session`,
  etc.), the `RegionFor` helper, the `SwapSimilarityScore` helper, and
  the `ErrNotFound` re-export. Phase 4 sweeps the import path in
  `cmd/web/` and deletes this file.
- **`service.go`** — five-line shim: `type Service = service.Service`
  plus a `NewService` forwarder. Phase 4 deletes this file too.

## Where to add new code

- **Pure rules / value objects / aggregate methods:** `internal/domain/`.
- **New SQL queries / repository methods:** `internal/repository/`.
- **Cross-aggregate orchestration / external integrations / GDPR:**
  `internal/service/`.
- **Nothing new lands here.** The package is closing down.

## Sentinel errors

`workout.ErrNotFound` re-exports `domain.ErrNotFound` for handler-side
`errors.Is` checks. New sentinels go in `internal/domain/errors.go`.
```

- [ ] **Step 2: Run lint and tests one final time**

Run: `make ci`
Expected: PASS (init, build, lint, test, sec all green).

- [ ] **Step 3: Commit**

```bash
git add internal/workout/CLAUDE.md
git commit -m "Update internal/workout/CLAUDE.md to reflect Phase 3 done"
```

---

## Self-review

**Spec coverage:**
- ✅ "Move internal/workout/service.go → internal/service/, splitting it into focused files" — Tasks 2-3 produce nine files matching the spec's responsibility list (with the documented regrouping: week-generation in `sessions.go` instead of `service.go`, preferences in `service.go` instead of a separate file).
- ✅ "Move generator-exercise.go and the unexported exerciseJSONSchema into internal/service/exercise_generation.go" — Task 2.
- ✅ "Make internal/workout.NewService a thin alias for service.NewService so cmd/web/ imports keep compiling without edits" — Task 3 step 9.
- ✅ "Move service_test.go (or split it alongside the file split)" — Task 4 moves it as one file (the choice approved during brainstorming).
- ✅ "service_internal_test.go (mondayOf) goes with whichever file owns mondayOf" — Task 4 moves it as `sessions_internal_test.go` because `mondayOf` lives in `sessions.go`.
- ✅ Drops the `repo` import alias (Phase 2 quirk) — Task 3 imports `repository` directly.
- ✅ Preserves the `fmt.Errorf("add exercise to session: %w", addErr)` wrap in `AddExercise` — Task 3 step 4 copies it verbatim.
- ✅ `models.go` keeps type aliases + `ErrNotFound` for Phase 4 — Task 2 step 7 only deletes the schema struct.

**Type/identifier consistency:** Every relocated function uses
`*Service` as receiver in `package service`; the workout shim uses
`type Service = service.Service`, so callers see the same identifier.
Domain-typed parameters and return types are consistently
`domain.Foo` in the new package; `workout.Foo` aliases continue to work
in `cmd/web` and in moved tests via the substitution rules.

**Behavior preservation:** Each method body is copied verbatim with only
domain-prefix substitutions and the `repo` → `repository` rename. No
control flow, error wrapping, logging, or transaction shape is
changed. The two test files relocate without behavior changes — only
their package and import lines move.

**Granularity check:** Each step is 2–5 minutes (write a file, run a
command, commit). The largest single step is Task 3 step 4 (write
`internal/service/exercises.go`, ~200 lines). That's larger than 5
minutes but the work is mechanical (copy + substitute) and splitting it
further would only fragment a coherent file's creation.

**Make ci green at end of every task:** Verified per task — Task 1
(empty package, nothing changes), Task 2 (cross-package delegate keeps
`GenerateExercise` callable), Task 3 (full alias swap, all method
resolutions through alias), Task 4 (tests run in their new home),
Task 5 (docs only). Each task ends with a commit.

**Phase boundary:** Phase 4 is explicitly out of scope (handler import
sweep, deletion of `models.go`, deletion of the workout shim). The plan
ends with the workout package as a two-file shim.
