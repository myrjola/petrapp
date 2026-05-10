# Workout Rearchitecture — Phase 4: Delete `internal/workout/`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sweep `cmd/web/` to reference `internal/domain` and `internal/service` directly, rename the `workoutService` field on the `application` struct to `service`, and delete the `internal/workout/` backward-compat shim. End-state: no Go package named `workout`, no field named `workoutService`, no behavior change.

**Architecture:** Phase 3 left `internal/workout/` as a thin shim — `service.go` aliases `workout.Service = service.Service` and forwards `NewService`; `models.go` aliases the public domain types as `workout.X = domain.X`, plus re-exports `ErrNotFound`, `RegionFor`, `SwapSimilarityScore`. Phase 4 inlines those aliases at the call sites. Three commits, each leaving `make ci` green: (1) identifier sweep across cmd/web, (2) field rename, (3) package deletion.

**Tech Stack:** Go (stdlib + `internal/domain` + `internal/service`).

**Spec:** `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md` — section "Migration phasing" / Phase 4.

---

## File Structure

### Modified files in `cmd/web/`

| File | Phase 4 changes |
|---|---|
| `cmd/web/main.go` | Replace `workout` import with `service`. Field type `*workout.Service` → `*service.Service`. `workout.NewService(...)` → `service.NewService(...)`. Field rename in Task 2. |
| `cmd/web/middleware.go` | No identifier sweep needed (only field references). Field rename in Task 2. |
| `cmd/web/handler-home.go` | Replace `workout` import with `domain`. `workout.Preferences` → `domain.Preferences`, `workout.Session` → `domain.Session`, `workout.MuscleGroupVolume` → `domain.MuscleGroupVolume`, `workout.MuscleGroupRegion` → `domain.MuscleGroupRegion`, `workout.RegionFor` → `domain.RegionFor`, region constants. |
| `cmd/web/handler-admin-exercises.go` | Replace `workout` import with `domain`. `workout.Exercise` → `domain.Exercise`, `workout.Category` → `domain.Category` and constants, `workout.ExerciseType` → `domain.ExerciseType` and constants, `workout.ErrNotFound` → `domain.ErrNotFound`. |
| `cmd/web/handler-admin-feature-flags.go` | Replace `workout` import with `domain`. `workout.FeatureFlag` → `domain.FeatureFlag`. |
| `cmd/web/handler-exerciseset.go` | Drop `workout` import (already imports `domain`). `workout.ExerciseSet` → `domain.ExerciseSet`, `workout.Set` → `domain.Set`, `workout.Signal` (and constants) → `domain.Signal`, `workout.ErrNotFound` → `domain.ErrNotFound`. |
| `cmd/web/handler-exercise-info.go` | Replace `workout` import with `domain`. `workout.ErrNotFound` → `domain.ErrNotFound`, `workout.ExerciseProgressEntry` → `domain.ExerciseProgressEntry`. |
| `cmd/web/handler-preferences.go` | Replace `workout` import with `domain`. `workout.Preferences` → `domain.Preferences`. |
| `cmd/web/handler-workout.go` | Replace `workout` import with `domain`. `workout.ErrNotFound` → `domain.ErrNotFound`, `workout.Session` → `domain.Session`, `workout.Exercise` → `domain.Exercise`, `workout.SwapSimilarityScore` → `domain.SwapSimilarityScore`. |
| `cmd/web/handler-exerciseset_test.go` | Replace `workout` import with `domain`. `workout.Exercise` → `domain.Exercise`, `workout.Category(...)` → `domain.Category(...)`, `workout.SwapSimilarityScore` → `domain.SwapSimilarityScore`. |
| `cmd/web/handler-home_status_test.go` | Replace `workout` import with `domain`. `workout.Session` → `domain.Session`. |
| `cmd/web/CLAUDE.md` | Documentation updates: code example uses `domain.Session`; `errors.Is(err, workout.ErrNotFound)` → `errors.Is(err, domain.ErrNotFound)`; "move it to `internal/workout/`" → "move it to `internal/domain/`"; `app.workoutService` → `app.service`; "internal/workout package" mention removed. |

### Deleted files

| Path | Reason |
|---|---|
| `internal/workout/service.go` | Shim no longer needed. |
| `internal/workout/models.go` | Type aliases no longer needed. |
| `internal/workout/CLAUDE.md` | Package guide no longer applies. |
| `internal/workout/README.md` | Workout-generation algorithm doc — verify nothing imports/links it before deleting; if still useful, relocate to `docs/`. |
| `internal/workout/` (directory) | Empty after the above; remove. |

### Untouched

| Path | Reason |
|---|---|
| `internal/domain/` | Stable. |
| `internal/repository/` | Stable. |
| `internal/service/` | Stable. |
| `internal/sqlite/` | No schema change. |
| `ui/templates/`, `ui/static/` | No template changes — templates already render through field methods, not package types. |

---

## Identifier substitution table

The complete list of `workout.X` identifiers in `cmd/web/` (from `grep -rhoE "workout\.[A-Z][A-Za-z_]+" cmd/web/ | sort -u`):

| Identifier | Replace with | Reason |
|---|---|---|
| `workout.Category` | `domain.Category` | Type alias in `models.go:12` |
| `workout.CategoryFullBody` | `domain.CategoryFullBody` | Constant alias in `models.go:14` |
| `workout.CategoryLower` | `domain.CategoryLower` | Constant alias in `models.go:14` |
| `workout.CategoryUpper` | `domain.CategoryUpper` | Constant alias in `models.go:14` |
| `workout.ErrNotFound` | `domain.ErrNotFound` | Re-export in `models.go:72` |
| `workout.Exercise` | `domain.Exercise` | Type alias in `models.go:44` |
| `workout.ExerciseProgressEntry` | `domain.ExerciseProgressEntry` | Type alias in `models.go:51` |
| `workout.ExerciseSet` | `domain.ExerciseSet` | Type alias in `models.go:48` |
| `workout.ExerciseType` | `domain.ExerciseType` | Type alias in `models.go:20` |
| `workout.ExerciseTypeAssisted` | `domain.ExerciseTypeAssisted` | Constant alias in `models.go:25` |
| `workout.ExerciseTypeBodyweight` | `domain.ExerciseTypeBodyweight` | Constant alias in `models.go:24` |
| `workout.ExerciseTypeTime` | `domain.ExerciseTypeTime` | Constant alias in `models.go:26` |
| `workout.ExerciseTypeWeighted` | `domain.ExerciseTypeWeighted` | Constant alias in `models.go:23` |
| `workout.FeatureFlag` | `domain.FeatureFlag` | Type alias in `models.go:53` |
| `workout.MuscleGroupRegion` | `domain.MuscleGroupRegion` | Type alias in `models.go:56` |
| `workout.MuscleGroupVolume` | `domain.MuscleGroupVolume` | Type alias in `models.go:55` |
| `workout.NewService` | `service.NewService` | Forwarder in `service.go:25` |
| `workout.Preferences` | `domain.Preferences` | Type alias in `models.go:52` |
| `workout.RegionCore` | `domain.RegionCore` | Constant alias in `models.go:63` |
| `workout.RegionFor` | `domain.RegionFor` | Helper in `models.go:67` |
| `workout.RegionLegs` | `domain.RegionLegs` | Constant alias in `models.go:62` |
| `workout.RegionOther` | `domain.RegionOther` | Constant alias in `models.go:64` |
| `workout.RegionUpperPull` | `domain.RegionUpperPull` | Constant alias in `models.go:61` |
| `workout.RegionUpperPush` | `domain.RegionUpperPush` | Constant alias in `models.go:60` |
| `workout.Service` | `service.Service` | Type alias in `service.go:21` |
| `workout.Session` | `domain.Session` | Type alias in `models.go:49` |
| `workout.Set` | `domain.Set` | Type alias in `models.go:47` |
| `workout.Signal` | `domain.Signal` | Type alias in `models.go:36` |
| `workout.SwapSimilarityScore` | `domain.SwapSimilarityScore` | Helper in `models.go:77` |

There are no remaining `workout.X` identifiers outside this list — the union of `models.go` aliases and `service.go` shim covers everything.

The shim does **not** contain any helper that isn't a domain re-export. If Task 1's grep turns up an unfamiliar identifier, **STOP**: that's a design question, not a sweep mechanic — surface it before continuing.

---

## Tasks

### Task 1: Sweep `workout.X` identifiers in `cmd/web/`

**Files:**
- Modify: `cmd/web/main.go` (lines 22, 30, 131)
- Modify: `cmd/web/handler-home.go`
- Modify: `cmd/web/handler-admin-exercises.go`
- Modify: `cmd/web/handler-admin-feature-flags.go`
- Modify: `cmd/web/handler-exerciseset.go`
- Modify: `cmd/web/handler-exercise-info.go`
- Modify: `cmd/web/handler-preferences.go`
- Modify: `cmd/web/handler-workout.go`
- Modify: `cmd/web/handler-exerciseset_test.go`
- Modify: `cmd/web/handler-home_status_test.go`
- Modify: `cmd/web/CLAUDE.md` (lines 39, 109; the `internal/workout/` reference on line 51 stays for Task 3)

This task does **not** rename the `workoutService` field — that's Task 2. After Task 1 the field is still spelled `workoutService` but its type is `*service.Service`.

The `cmd/web/middleware.go` file references `app.workoutService` but no `workout.X` identifier; it gets touched in Task 2, not here.

- [ ] **Step 1: Confirm starting state**

Run:
```bash
grep -rhoE "workout\.[A-Z][A-Za-z_]+" cmd/web/ | sort -u
```

Expected output (28 lines):
```
workout.Category
workout.CategoryFullBody
workout.CategoryLower
workout.CategoryUpper
workout.ErrNotFound
workout.Exercise
workout.ExerciseProgressEntry
workout.ExerciseSet
workout.ExerciseType
workout.ExerciseTypeAssisted
workout.ExerciseTypeBodyweight
workout.ExerciseTypeTime
workout.ExerciseTypeWeighted
workout.FeatureFlag
workout.MuscleGroupRegion
workout.MuscleGroupVolume
workout.NewService
workout.Preferences
workout.RegionCore
workout.RegionFor
workout.RegionLegs
workout.RegionOther
workout.RegionUpperPull
workout.RegionUpperPush
workout.Service
workout.Session
workout.Set
workout.Signal
workout.SwapSimilarityScore
```

If the grep returns anything not on this list, STOP and surface it.

- [ ] **Step 2: Update `cmd/web/main.go`**

Replace the `workout` import with `service`:
```go
"github.com/myrjola/petrapp/internal/service"
```

Replace the field type and constructor:
```go
service *service.Service       // line 30 — keep field NAME workoutService for Task 1, change in Task 2
```

Wait — that introduces the rename inside Task 1, which we explicitly deferred to Task 2. The correct edit for Task 1 is:

Field declaration on line 30 of `main.go` becomes:
```go
workoutService *service.Service
```

Constructor call on line 131 becomes:
```go
workoutService: service.NewService(db, logger, cfg.OpenAIAPIKey),
```

(Field name unchanged. Type and constructor switch from `workout.*` to `service.*`.)

After this edit, `cmd/web/main.go` no longer imports `internal/workout`.

- [ ] **Step 3: Update `cmd/web/handler-home.go`**

Replace the import:
```go
"github.com/myrjola/petrapp/internal/domain"
```

Then replace every `workout.X` in this file with `domain.X` per the substitution table. Identifiers in this file (verify with `grep -nE "workout\." cmd/web/handler-home.go`): `Preferences`, `Session`, `MuscleGroupVolume`, `MuscleGroupRegion`, `RegionFor`, `RegionUpperPush`, `RegionUpperPull`, `RegionLegs`, `RegionCore`, `RegionOther`.

After this edit, `cmd/web/handler-home.go` imports `internal/domain` and not `internal/workout`.

- [ ] **Step 4: Update `cmd/web/handler-admin-exercises.go`**

Replace the import:
```go
"github.com/myrjola/petrapp/internal/domain"
```

Replace every `workout.X` in this file with `domain.X`. Identifiers (verify with `grep -nE "workout\." cmd/web/handler-admin-exercises.go`): `Exercise`, `Category`, `CategoryFullBody`, `CategoryUpper`, `CategoryLower`, `ExerciseType`, `ExerciseTypeWeighted`, `ExerciseTypeBodyweight`, `ExerciseTypeAssisted`, `ExerciseTypeTime`, `ErrNotFound`.

- [ ] **Step 5: Update `cmd/web/handler-admin-feature-flags.go`**

Replace the import:
```go
"github.com/myrjola/petrapp/internal/domain"
```

Replace `workout.FeatureFlag` with `domain.FeatureFlag`.

- [ ] **Step 6: Update `cmd/web/handler-exerciseset.go`**

This file already imports `internal/domain` (line 13). Drop the `internal/workout` import on line 14.

Replace every `workout.X` with `domain.X`. Identifiers: `ExerciseSet`, `Set`, `Signal` (with `SignalTooHeavy`/`SignalOnTarget`/`SignalTooLight` if present), `ErrNotFound`.

- [ ] **Step 7: Update `cmd/web/handler-exercise-info.go`**

Replace the import:
```go
"github.com/myrjola/petrapp/internal/domain"
```

Replace `workout.ErrNotFound` → `domain.ErrNotFound`, `workout.ExerciseProgressEntry` → `domain.ExerciseProgressEntry`.

- [ ] **Step 8: Update `cmd/web/handler-preferences.go`**

Replace the import:
```go
"github.com/myrjola/petrapp/internal/domain"
```

Replace `workout.Preferences` → `domain.Preferences`.

- [ ] **Step 9: Update `cmd/web/handler-workout.go`**

Replace the import:
```go
"github.com/myrjola/petrapp/internal/domain"
```

Replace every `workout.X` with `domain.X`. Identifiers (verify with grep): `ErrNotFound`, `Session`, `Exercise`, `SwapSimilarityScore`.

- [ ] **Step 10: Update `cmd/web/handler-exerciseset_test.go`**

Replace the import:
```go
"github.com/myrjola/petrapp/internal/domain"
```

Replace `workout.Exercise` → `domain.Exercise`, `workout.Category(...)` → `domain.Category(...)`, `workout.SwapSimilarityScore` → `domain.SwapSimilarityScore`.

- [ ] **Step 11: Update `cmd/web/handler-home_status_test.go`**

Replace the import:
```go
"github.com/myrjola/petrapp/internal/domain"
```

Replace `workout.Session` → `domain.Session`.

- [ ] **Step 12: Update `cmd/web/CLAUDE.md` (identifier-related lines only)**

Two textual edits in this task. The `internal/workout/` reference on line 51 and the `app.workoutService` references on lines 177-178 stay for now — they belong to later tasks.

Line 39: replace
```
  Session workout.Session
```
with
```
  Session domain.Session
```

Line 109: replace
```
- Check for specific business errors using `errors.Is(err, workout.ErrNotFound)`
```
with
```
- Check for specific business errors using `errors.Is(err, domain.ErrNotFound)`
```

- [ ] **Step 13: Verify the sweep is complete**

Run:
```bash
grep -rnE "workout\." cmd/web/*.go
```

Expected: only `cmd/web/main.go` and `cmd/web/middleware.go` should appear. `main.go` should show only the `workoutService` field name (lines 30, 131); `middleware.go` should show only `app.workoutService` (lines 272-273). No `workout.SomeType` style references.

Run:
```bash
grep -rnE "github.com/myrjola/petrapp/internal/workout" cmd/web/
```

Expected: no output (zero hits). The `internal/workout` import is gone from every cmd/web file.

- [ ] **Step 14: Run `make ci`**

Run:
```bash
make ci
```

Expected: green. (`init`, `build`, `lint`, `test`, `sec` all pass.)

If anything fails, do **not** proceed. Diagnose first; almost certainly it's a missed identifier.

- [ ] **Step 15: Commit**

```bash
git add cmd/web/
git commit -m "$(cat <<'EOF'
Sweep workout.X to domain.X / service.X in cmd/web (Phase 4 step 1/3)

Replaces every workout.<DomainType> reference in cmd/web with
domain.<DomainType>, and the two service references (workout.Service,
workout.NewService) with service.Service / service.NewService. Updates
the cmd/web/CLAUDE.md code example and ErrNotFound reference to match.

The workoutService field on the application struct keeps its name for
now — only the type and constructor switch packages here. Field rename
is step 2/3.

internal/workout/ remains in place as an unused shim until step 3/3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Rename `workoutService` field to `service`

**Files:**
- Modify: `cmd/web/main.go` (field declaration + constructor call)
- Modify: `cmd/web/middleware.go` (`app.workoutService` references)
- Modify: `cmd/web/handler-home.go`
- Modify: `cmd/web/handler-admin-exercises.go`
- Modify: `cmd/web/handler-admin-feature-flags.go`
- Modify: `cmd/web/handler-exerciseset.go`
- Modify: `cmd/web/handler-exercise-info.go`
- Modify: `cmd/web/handler-preferences.go`
- Modify: `cmd/web/handler-schedule.go`
- Modify: `cmd/web/handler-workout.go`
- Modify: `cmd/web/CLAUDE.md` (lines 177-178)

The new field name is `service`. Reading `app.service.GetSession(...)` is fine — the field's type already says it's a service, so the prefix would be redundant.

- [ ] **Step 1: Confirm starting state**

Run:
```bash
grep -rln "workoutService" cmd/web/
```

Expected: 11 files (10 .go files + CLAUDE.md). Specifically:
```
cmd/web/CLAUDE.md
cmd/web/main.go
cmd/web/handler-home.go
cmd/web/handler-admin-exercises.go
cmd/web/handler-preferences.go
cmd/web/handler-exercise-info.go
cmd/web/handler-admin-feature-flags.go
cmd/web/handler-exerciseset.go
cmd/web/handler-schedule.go
cmd/web/handler-workout.go
cmd/web/middleware.go
```

Total reference count:
```bash
grep -rnE "workoutService" cmd/web/ | wc -l
```
Expected: ~45 lines.

- [ ] **Step 2: Mechanical rename across all 11 files**

In every file, replace the identifier `workoutService` with `service`. This is a simple word-boundary replacement; there are no other identifiers that share this name.

Per-file shape of the change:
- `cmd/web/main.go` line 30: `workoutService *service.Service` → `service *service.Service`
- `cmd/web/main.go` line 131: `workoutService: service.NewService(...)` → `service: service.NewService(...)`
- All other `cmd/web/*.go` files: every `app.workoutService.Method(...)` → `app.service.Method(...)`
- `cmd/web/middleware.go` lines 272-273:
  - Comment: `// Check if maintenance mode is enabled (skip if workoutService is nil for tests)` → `// Check if maintenance mode is enabled (skip if service is nil for tests)`
  - Code: `if app.workoutService != nil && app.workoutService.IsMaintenanceModeEnabled(ctx) {` → `if app.service != nil && app.service.IsMaintenanceModeEnabled(ctx) {`
- `cmd/web/CLAUDE.md` lines 177-178:
  ```
  - All business logic goes through service layer (`app.workoutService`, etc.)
  - Pass request context to service methods: `app.workoutService.Method(r.Context(), params)`
  ```
  becomes:
  ```
  - All business logic goes through service layer (`app.service`, etc.)
  - Pass request context to service methods: `app.service.Method(r.Context(), params)`
  ```

A `sed` one-liner that does this safely (the identifier `workoutService` is unique enough that word-boundary matching is unnecessary, but explicit boundaries are safer):
```bash
find cmd/web -type f \( -name '*.go' -o -name 'CLAUDE.md' \) -exec sed -i 's/\bworkoutService\b/service/g' {} +
```

- [ ] **Step 3: Watch for the field-name shadow in main.go**

Note that `main.go` line 131 ends up reading `service: service.NewService(db, logger, cfg.OpenAIAPIKey),` — a struct-literal field named `service` initialized from the package `service`. Go allows this: in `Field: Expression`, `Field` is in the struct's namespace and `Expression` is in the surrounding scope. There's no shadowing because the struct field name doesn't shadow the package import.

If the build fails complaining about `service` being ambiguous, that's a parse error in your edit, not a real ambiguity — re-read the line.

- [ ] **Step 4: Verify the rename is complete**

Run:
```bash
grep -rnE "workoutService" cmd/web/
```

Expected: no output.

```bash
grep -rnE "\bservice\b" cmd/web/main.go
```

Expected: at least the field declaration on the `application` struct and the struct-literal field init in `application{...}`. The package import `"github.com/myrjola/petrapp/internal/service"` is also a hit on the line of the import statement.

- [ ] **Step 5: Run `make ci`**

```bash
make ci
```

Expected: green.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/
git commit -m "$(cat <<'EOF'
Rename application.workoutService to service in cmd/web (Phase 4 step 2/3)

Mechanical rename across cmd/web. The field type is already
*service.Service after step 1/3; reading "service" from the field name
is now redundant with the type so the workout prefix earns nothing.

Touches main.go (declaration + ctor call), middleware.go (maintenance
gate), and every handler that calls into the service layer. CLAUDE.md
guidance updated to match.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Delete `internal/workout/` and finalize CLAUDE.md

**Files:**
- Delete: `internal/workout/service.go`
- Delete: `internal/workout/models.go`
- Delete: `internal/workout/CLAUDE.md`
- Delete: `internal/workout/README.md` (move/archive if it's the only home for the workout-generation algorithm — see Step 1)
- Delete: `internal/workout/` (directory, after files are gone)
- Modify: `cmd/web/CLAUDE.md` (line 51 reference to `internal/workout/`)

- [ ] **Step 1: Verify nothing imports `internal/workout` and decide on README.md**

```bash
grep -rln "github.com/myrjola/petrapp/internal/workout" .
```

Expected: zero hits. (Earlier tasks already removed all imports; this confirms.)

```bash
grep -rln "internal/workout" --exclude-dir=docs --exclude-dir=.git .
```

Expected: only matches inside `internal/workout/` itself (the package's own files), and possibly `cmd/web/CLAUDE.md` line 51 (handled in Step 5). Nothing in source files outside `internal/workout/`.

If `docs/` or other locations have prose references to `internal/workout/`, they're historical context and can stay. The only blocking case is a code import.

`internal/workout/README.md` is the workout-generation algorithm description. Check whether it's the only such write-up:
```bash
grep -rln "Workout Type Determination\|Undulating periodization\|Linear progression" --include='*.md' .
```

If `README.md` is the only hit, consider moving it to `docs/workout-generation.md` rather than deleting. If it duplicates content already in `docs/superpowers/specs/`, delete it.

In this task we **delete** the README on the assumption that the algorithm description is informational and hasn't been the canonical reference for handler/service work — the design spec at `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md` and the per-package CLAUDE.md files in `internal/domain/`, `internal/repository/`, `internal/service/` are the live references. If, while reading the file, you discover it documents non-obvious algorithm decisions that aren't captured elsewhere, **STOP** and surface that — moving it to `docs/2026-05-10-workout-generation.md` is the right call, not deletion.

- [ ] **Step 2: Delete the workout package files**

```bash
rm internal/workout/service.go internal/workout/models.go internal/workout/CLAUDE.md internal/workout/README.md
```

- [ ] **Step 3: Remove the empty directory**

```bash
rmdir internal/workout
```

`rmdir` will fail if anything else lives in the directory — that's a useful safety check. If it fails, list the contents (`ls internal/workout/`) and decide what to do.

- [ ] **Step 4: Verify the package is gone**

```bash
ls internal/workout/ 2>&1 | head
```

Expected: `ls: cannot access 'internal/workout/': No such file or directory`.

```bash
go list ./...
```

Expected: a list of packages that does **not** include `github.com/myrjola/petrapp/internal/workout`.

- [ ] **Step 5: Update `cmd/web/CLAUDE.md` line 51**

The line currently reads:
```
- **Don't recompute domain rules.** Handlers may format primitives and shape data, but any value that depends on multiple domain fields must come from a method on the domain type. If you find yourself writing `if exercise.X && session.Y { ... }` in a handler, move it to `internal/workout/`.
```

Replace `internal/workout/` with `internal/domain/`:
```
- **Don't recompute domain rules.** Handlers may format primitives and shape data, but any value that depends on multiple domain fields must come from a method on the domain type. If you find yourself writing `if exercise.X && session.Y { ... }` in a handler, move it to `internal/domain/`.
```

- [ ] **Step 6: Verify no stale references in code or docs**

```bash
grep -rnE "internal/workout|workout\.[A-Z]" --include='*.go' .
```

Expected: zero hits.

```bash
grep -rln "workoutService" --include='*.go' .
```

Expected: zero hits.

(Prose references in `docs/superpowers/plans/` or `docs/superpowers/specs/` to the old `internal/workout/` package as historical context are fine and don't need scrubbing — those documents are time-stamped artifacts.)

- [ ] **Step 7: Run `make ci`**

```bash
make ci
```

Expected: green.

- [ ] **Step 8: Commit**

```bash
git add -A internal/workout cmd/web/CLAUDE.md
git commit -m "$(cat <<'EOF'
Delete internal/workout/ (Phase 4 step 3/3)

The package was a backward-compat shim after Phase 3 — all of cmd/web
now imports internal/domain and internal/service directly, so the
type-alias layer is dead weight. Removes service.go, models.go,
CLAUDE.md, README.md, and the directory itself.

cmd/web/CLAUDE.md updated to point would-be domain-rule additions at
internal/domain/ instead of the deleted workout package.

End-state: no Go package named workout, no handler imports referring to
it, and the field on the application struct is application.service.
This closes out the four-phase workout rearchitecture.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Checklist (run after Task 3 commit, before merging back to main)

- [ ] `grep -rnE "internal/workout|workout\.[A-Z]" --include='*.go' .` returns zero hits.
- [ ] `grep -rln "workoutService" --include='*.go' .` returns zero hits.
- [ ] `ls internal/workout/ 2>&1` shows `No such file or directory`.
- [ ] `go list ./...` does not list `github.com/myrjola/petrapp/internal/workout`.
- [ ] `make ci` is green on the final commit.
- [ ] Three commits land on the branch with subjects matching `Sweep workout.X .* (Phase 4 step 1/3)`, `Rename application.workoutService .* (Phase 4 step 2/3)`, `Delete internal/workout/ .* (Phase 4 step 3/3)`.
- [ ] No template files in `ui/templates/` were touched (templates render through field methods, not package types).

---

## Stop conditions (surface to user, do not work around)

- Step 1 of Task 1 finds `workout.X` identifiers not on the substitution table → unknown re-export needs a design call.
- Anywhere `cmd/web/` imports something from `internal/workout/` other than the documented Service/aliases → an undocumented helper was being relied on; flag and discuss.
- `make ci` fails after a step with errors that aren't "missed identifier" / "missing import" / "unused import" → diagnose; don't proceed.
- `git status` shows changes outside `cmd/web/` and `internal/workout/` after any task → the sweep over-reached.
