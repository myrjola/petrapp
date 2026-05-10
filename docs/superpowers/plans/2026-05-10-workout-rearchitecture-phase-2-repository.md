# Workout Rearchitecture — Phase 2: Extract `internal/repository/`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract `internal/repository/` as the canonical persistence layer, returning `domain.*` types directly. The `sessionAggregate` / `exerciseSetAggregate` / `datedExerciseSetAggregate` triple is deleted; the Update closure operates on `*domain.Session`. `internal/workout/service.go` is rewired to use `*repository.Repositories` and adopts the aggregate methods Phase 1 added on `domain.Session` (Start, Complete, RecordSet, AddExercise, SwapExerciseInSlot, etc.). `RecordSetCompletion` and `RecordTimedSetCompletion` collapse into a unified `Service.RecordSet`. `enrichSessionAggregate` disappears (hydration happens in the repo). Phase 2 ends with `internal/workout/repository*.go` deleted (6 files).

**Architecture:** New package `internal/repository/` exposes one public constructor `repository.New(db, logger) *Repositories` and five exported interfaces (`SessionRepository`, `ExerciseRepository`, `PreferencesRepository`, `FeatureFlagRepository`, `MuscleGroupTargetRepository`). SQLite implementations are unexported. Round-trip tests (external `package repository_test`) cover insert → Update → re-read for the load-bearing repos, including the slot-ID-stability invariant for `SessionRepository`. The session repo holds an `ExerciseRepository` reference (composition, injected in `New`) so `Get`/`List` can hydrate `ExerciseSet.Exercise` per the "one Session, always hydrated" decision in the spec.

**Tech Stack:** Go (stdlib + `internal/sqlite` + `internal/contexthelpers`), no new dependencies.

**Phase boundary:** This plan covers Phase 2 only. Phase 3 (move `service.go` and `generator-exercise.go` into `internal/service/`) and Phase 4 (delete `internal/workout/`, drop type aliases, sweep handler imports) get separate plans.

**Spec:** `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md` — sections "internal/repository/" and "Migration phasing".

---

## File Structure

### New files in `internal/repository/`

| File | Responsibility |
|---|---|
| `internal/repository/shared.go` | `parseTimestamp`, `formatTimestamp`, `formatDate`, `baseRepository`, `timestampFormat`, `dateFormat` (relocated from `workout/repository.go`) |
| `internal/repository/repository.go` | `Repositories` struct, `New(db, logger) *Repositories` constructor, the five exported interfaces (`SessionRepository`, `ExerciseRepository`, `PreferencesRepository`, `FeatureFlagRepository`, `MuscleGroupTargetRepository`) |
| `internal/repository/preferences.go` | `sqlitePreferencesRepository` + `newSQLitePreferencesRepository` |
| `internal/repository/preferences_test.go` | external `package repository_test` round-trip test |
| `internal/repository/muscle_targets.go` | `sqliteMuscleGroupTargetRepository` + constructor |
| `internal/repository/muscle_targets_test.go` | external `package repository_test` test |
| `internal/repository/feature_flags.go` | `sqliteFeatureFlagRepository` + constructor |
| `internal/repository/feature_flags_test.go` | external `package repository_test` test |
| `internal/repository/exercises.go` | `sqliteExerciseRepository` + constructor; `Update` signature drops the bool |
| `internal/repository/exercises_test.go` | external `package repository_test` test |
| `internal/repository/sessions.go` | `sqliteSessionRepository` + constructor (takes `ExerciseRepository` for hydration); `Update` closure is `func(*domain.Session) error`; `Get`/`List` return hydrated sessions; `ListSetsForExerciseSince` returns `[]domain.ExerciseSetHistory` |
| `internal/repository/sessions_test.go` | external `package repository_test` round-trip + slot-ID-stability + hydration + ErrNotFound translation tests |
| `internal/repository/helpers_test.go` | external `package repository_test` test setup helper (in-memory db, test user, returns ctx + db + `*Repositories`) |
| `internal/repository/CLAUDE.md` | Repository pattern, Update closure contract, hydration policy |

### Modified files

| File | Change |
|---|---|
| `internal/workout/service.go` | Replace `repo *repository` field with `repos *repository.Repositories`; switch every closure body to `func(*domain.Session) error`; adopt aggregate methods (`Session.Start`, `Session.Complete`, `Session.SetDifficulty`, `Session.MarkWarmupComplete`, `Session.UpdateSetWeight`, `Session.UpdateCompletedValue`, `Session.RecordSet`, `Session.AddExercise`, `Session.SwapExerciseInSlot`); collapse `RecordSetCompletion` + `RecordTimedSetCompletion` into a unified `RecordSet`; delete `enrichSessionAggregate`; simplify `generateWeeklyPlan` to pass planner output straight to `CreateBatch`; swallow `domain.ErrAlreadyStarted` in `StartSession` |
| `cmd/web/handler-exerciseset.go` | Update two call sites: `RecordSetCompletion(...,w,reps)` → `RecordSet(...,sig,&w,reps)`; `RecordTimedSetCompletion(...,sig,secs)` → `RecordSet(...,sig,nil,secs)` |
| `internal/workout/service_test.go` | Update two call sites for the collapsed `RecordSet` (same one-line transform as handlers) |
| `internal/workout/CLAUDE.md` | Replace contents with a Phase 2 progress note: pure logic in `internal/domain/`, persistence in `internal/repository/`, this package now contains only orchestration (Phase 3 will move it). Remove the repository-pattern guidance — it lives in `internal/repository/CLAUDE.md` now |

### Deleted files

| File | Reason |
|---|---|
| `internal/workout/repository.go` | Wrapper struct, factory, local interfaces, format helpers — all moved or obsolete |
| `internal/workout/repository-sessions.go` | Replaced by `internal/repository/sessions.go` |
| `internal/workout/repository-exercises.go` | Replaced by `internal/repository/exercises.go` |
| `internal/workout/repository-preferences.go` | Replaced by `internal/repository/preferences.go` |
| `internal/workout/repository-featureflags.go` | Replaced by `internal/repository/feature_flags.go` |
| `internal/workout/repository-muscle-targets.go` | Replaced by `internal/repository/muscle_targets.go` |

### Untouched

| Path | Reason |
|---|---|
| `internal/sqlite/` | No schema change |
| `internal/domain/` | Phase 1 surface is stable |
| `internal/workout/models.go` | Type aliases stay so `cmd/web/` imports keep working through Phase 4 |
| `internal/workout/generator-exercise.go` + test | Moves in Phase 3 |
| `internal/workout/service_internal_test.go` | Tests `mondayOf` — unchanged |
| `cmd/web/*.go` (other than `handler-exerciseset.go`) | The 28 `workout.*` references resolve through unchanged type aliases or unchanged service method names |
| `ui/templates/` | No template changes |

---

## Migration sequencing rationale

Each task leaves the tree compiling and `make test` passing.

1. **Task 1: Scaffold `internal/repository/`.** Create the package with shared helpers, declare all five interfaces, and stub `New()` returning a `Repositories` with nil fields. Add a test helper. The package compiles in isolation; nothing else uses it yet.
2. **Tasks 2–5: Move the four mechanical repos** (Preferences, MuscleGroupTargets, FeatureFlags, Exercises). Each task adds the impl, wires it into `New()`, writes a round-trip test. Tree green after every task. The old `internal/workout/repository-*.go` impls remain in place — workout's local repository struct still wires them via its factory.
3. **Task 6: Move `SessionRepository`.** The substantive change. Update closure becomes `func(*domain.Session) error`; `Get`/`List` hydrate via the injected `ExerciseRepository`; the aggregate types (`sessionAggregate` etc.) live only in the new package's mind as transient query intermediates. Comprehensive round-trip test including slot-ID stability and hydration.
4. **Task 7: Cut over `internal/workout/service.go`.** Switch `s.repo *workout.repository` → `s.repos *repository.Repositories`; rewrite every closure body using aggregate methods; collapse `RecordSetCompletion` + `RecordTimedSetCompletion` into `RecordSet`; delete `enrichSessionAggregate`; simplify `generateWeeklyPlan`. Update the two cmd/web/ handler call sites and two service-test call sites for the collapsed `RecordSet`.
5. **Task 8: Delete `internal/workout/repository*.go`** (6 files). Workout no longer references its local repo wrapper.
6. **Task 9: Add `internal/repository/CLAUDE.md`.**
7. **Task 10: Update `internal/workout/CLAUDE.md`** to reflect Phase 2 done.
8. **Task 11: Final `make ci`** verification + commit.

---

## Tasks

### Task 1: Scaffold `internal/repository/` skeleton

**Files:**
- Create: `internal/repository/shared.go`
- Create: `internal/repository/repository.go`
- Create: `internal/repository/helpers_test.go`

- [ ] **Step 1: Create `internal/repository/shared.go` with helpers relocated from workout.**

```go
package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/sqlite"
)

const (
	timestampFormat = "2006-01-02T15:04:05.000Z"
	dateFormat      = time.DateOnly
)

// baseRepository contains common functionality for all SQLite repositories.
type baseRepository struct {
	db *sqlite.Database
}

func newBaseRepository(db *sqlite.Database) baseRepository {
	return baseRepository{db: db}
}

// parseTimestamp parses a timestamp from a nullable database string.
func parseTimestamp(timestampStr sql.NullString) (time.Time, error) {
	if !timestampStr.Valid {
		return time.Time{}, nil
	}
	parsedTime, err := time.Parse(timestampFormat, timestampStr.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp format: %w", err)
	}
	return parsedTime, nil
}

// formatDate formats a time.Time to the canonical YYYY-MM-DD string.
func formatDate(date time.Time) string {
	return date.Format(dateFormat)
}

// formatTimestamp formats a time.Time to the canonical UTC ISO-8601 string.
func formatTimestamp(t time.Time) string {
	return t.UTC().Format(timestampFormat)
}
```

- [ ] **Step 2: Create `internal/repository/repository.go` with the `Repositories` struct, `New()` stub, and all five interfaces.**

```go
// Package repository contains SQLite-backed implementations of the workout
// domain's data-access contracts. Repositories return domain.* types directly;
// no persistence-shaped intermediate aggregate is exposed to callers. Update
// closures operate on *domain.Session — invariants are enforced via the
// aggregate methods on domain.Session, not by the repository.
package repository

import (
	"context"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Repositories bundles the per-aggregate repository handles wired together by
// New. Its fields are interface-typed so callers depend on the contract, not
// the SQLite implementation.
type Repositories struct {
	Sessions      SessionRepository
	Exercises     ExerciseRepository
	Preferences   PreferencesRepository
	FeatureFlags  FeatureFlagRepository
	MuscleTargets MuscleGroupTargetRepository
}

// New constructs all five SQLite-backed repositories, wiring the
// ExerciseRepository into the SessionRepository so Get/List can hydrate
// ExerciseSet.Exercise inside a single read.
func New(db *sqlite.Database, logger *slog.Logger) *Repositories {
	_ = logger // reserved for future per-repo logging; unused today.
	return &Repositories{ //nolint:exhaustruct // Fields wired by per-repo tasks.
	}
}

// SessionRepository persists workout sessions and their exercise slots.
type SessionRepository interface {
	Get(ctx context.Context, date time.Time) (domain.Session, error)
	List(ctx context.Context, sinceDate time.Time) ([]domain.Session, error)
	CreateBatch(ctx context.Context, sessions []domain.Session) error

	// Update loads the session inside a single transaction, runs fn against
	// the hydrated *domain.Session, and persists the result. Returning nil
	// from fn commits; returning an error rolls back. Sentinel errors from
	// domain (e.g. ErrAlreadyStarted) propagate so callers can detect no-op
	// cases via errors.Is.
	Update(ctx context.Context, date time.Time, fn func(*domain.Session) error) error

	DeleteWeek(ctx context.Context, monday time.Time) error

	// Read-only specialised queries.
	ListSetsForExerciseSince(ctx context.Context, exerciseID int, sinceDate time.Time) ([]domain.ExerciseSetHistory, error)
	GetLatestStartingWeightBefore(ctx context.Context, exerciseID int, beforeDate time.Time) (domain.LatestStartingSet, error)
	GetLatestSuccessfulSecondsBefore(ctx context.Context, exerciseID int, beforeDate time.Time) (int, error)
	CountCompleted(ctx context.Context) (int, error)
}

// ExerciseRepository persists exercise definitions and their muscle-group
// associations.
type ExerciseRepository interface {
	Get(ctx context.Context, id int) (domain.Exercise, error)
	List(ctx context.Context) ([]domain.Exercise, error)
	Create(ctx context.Context, ex domain.Exercise) (domain.Exercise, error)

	// Update reads the exercise, runs fn, and persists the result if fn
	// returned nil. fn returning an error rolls back without writing.
	Update(ctx context.Context, exerciseID int, fn func(*domain.Exercise) error) error

	ListMuscleGroups(ctx context.Context) ([]string, error)
}

// PreferencesRepository persists per-user weekly schedule preferences.
type PreferencesRepository interface {
	Get(ctx context.Context) (domain.Preferences, error)
	Set(ctx context.Context, prefs domain.Preferences) error
}

// FeatureFlagRepository persists boolean feature toggles by name.
type FeatureFlagRepository interface {
	Get(ctx context.Context, name string) (domain.FeatureFlag, error)
	Set(ctx context.Context, flag domain.FeatureFlag) error
	List(ctx context.Context) ([]domain.FeatureFlag, error)
}

// MuscleGroupTargetRepository serves the static muscle-group weekly volume
// targets used by the planner.
type MuscleGroupTargetRepository interface {
	List(ctx context.Context) ([]domain.MuscleGroupTarget, error)
}
```

- [ ] **Step 3: Create `internal/repository/helpers_test.go` with a shared test setup.**

```go
package repository_test

import (
	"context"
	"testing"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// setupTestRepos creates an in-memory database, inserts a test user, and
// returns the authenticated context plus a populated *Repositories.
func setupTestRepos(t *testing.T) (context.Context, *repository.Repositories) {
	t.Helper()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user"), "Test User").Scan(&userID); err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	return ctx, repository.New(db, logger)
}
```

- [ ] **Step 4: Verify the new package builds.**

Run: `go build ./internal/repository/`
Expected: no output (build succeeds).

- [ ] **Step 5: Verify the rest of the tree still compiles.**

Run: `go build ./...`
Expected: no output.

- [ ] **Step 6: Commit.**

```bash
git add internal/repository/
git commit -m "Scaffold internal/repository/ with shared helpers and interfaces"
```

---

### Task 2: Move `PreferencesRepository`

**Files:**
- Create: `internal/repository/preferences.go`
- Create: `internal/repository/preferences_test.go`
- Modify: `internal/repository/repository.go` (wire `Preferences` into `New`)

- [ ] **Step 1: Create `internal/repository/preferences.go`.**

```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqlitePreferencesRepository struct {
	baseRepository
}

func newSQLitePreferencesRepository(db *sqlite.Database) *sqlitePreferencesRepository {
	return &sqlitePreferencesRepository{baseRepository: newBaseRepository(db)}
}

// Get returns the authenticated user's weekly schedule preferences. When no
// row exists yet the all-zero (all rest days) Preferences value is returned —
// this mirrors the previous workout package behaviour and keeps first-time
// users on a clean slate without a special "missing" sentinel.
func (r *sqlitePreferencesRepository) Get(ctx context.Context) (domain.Preferences, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	var prefs domain.Preferences
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
		       friday_minutes, saturday_minutes, sunday_minutes
		FROM workout_preferences
		WHERE user_id = ?`, userID).Scan(
		&prefs.MondayMinutes, &prefs.TuesdayMinutes, &prefs.WednesdayMinutes, &prefs.ThursdayMinutes,
		&prefs.FridayMinutes, &prefs.SaturdayMinutes, &prefs.SundayMinutes,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.Preferences{ //nolint:exhaustruct // All fields zero by design.
		}, nil
	}
	if err != nil {
		return domain.Preferences{}, fmt.Errorf("query workout preferences: %w", err)
	}
	return prefs, nil
}

// Set upserts the authenticated user's weekly schedule preferences.
func (r *sqlitePreferencesRepository) Set(ctx context.Context, prefs domain.Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_preferences (
			user_id, monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
			friday_minutes, saturday_minutes, sunday_minutes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			monday_minutes = excluded.monday_minutes,
			tuesday_minutes = excluded.tuesday_minutes,
			wednesday_minutes = excluded.wednesday_minutes,
			thursday_minutes = excluded.thursday_minutes,
			friday_minutes = excluded.friday_minutes,
			saturday_minutes = excluded.saturday_minutes,
			sunday_minutes = excluded.sunday_minutes`,
		userID,
		prefs.MondayMinutes, prefs.TuesdayMinutes, prefs.WednesdayMinutes, prefs.ThursdayMinutes,
		prefs.FridayMinutes, prefs.SaturdayMinutes, prefs.SundayMinutes,
	); err != nil {
		return fmt.Errorf("save workout preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Wire `Preferences` into `New` in `internal/repository/repository.go`.**

Replace the body of `New` with:

```go
func New(db *sqlite.Database, logger *slog.Logger) *Repositories {
	_ = logger // reserved for future per-repo logging; unused today.
	prefs := newSQLitePreferencesRepository(db)
	return &Repositories{ //nolint:exhaustruct // Other fields wired in later tasks.
		Preferences: prefs,
	}
}
```

- [ ] **Step 3: Write the round-trip test in `internal/repository/preferences_test.go`.**

```go
package repository_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestPreferencesRepository_GetEmptyReturnsZeroValue(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get on empty: %v", err)
	}
	want := domain.Preferences{} //nolint:exhaustruct // All zero by design.
	if got != want {
		t.Errorf("empty Get: want %+v, got %+v", want, got)
	}
}

func TestPreferencesRepository_SetThenGetRoundTrip(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	want := domain.Preferences{ //nolint:exhaustruct // Untouched days stay zero.
		MondayMinutes:    60,
		WednesdayMinutes: 45,
		FridayMinutes:    30,
	}
	if err := repos.Preferences.Set(ctx, want); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("round-trip: want %+v, got %+v", want, got)
	}
}

func TestPreferencesRepository_SetUpdatesExisting(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	if err := repos.Preferences.Set(ctx, domain.Preferences{ //nolint:exhaustruct // First write.
		MondayMinutes: 30,
	}); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	updated := domain.Preferences{ //nolint:exhaustruct // Second write — Monday changes, others stay zero.
		MondayMinutes:  90,
		TuesdayMinutes: 45,
	}
	if err := repos.Preferences.Set(ctx, updated); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != updated {
		t.Errorf("after upsert: want %+v, got %+v", updated, got)
	}
}
```

- [ ] **Step 4: Run the new tests.**

Run: `go test -v ./internal/repository/ -run TestPreferencesRepository`
Expected: PASS for all three tests.

- [ ] **Step 5: Run the full repo test suite + the rest of the tree.**

Run: `go test ./...`
Expected: PASS (workout package still works because its old impls are still in place).

- [ ] **Step 6: Commit.**

```bash
git add internal/repository/preferences.go internal/repository/preferences_test.go internal/repository/repository.go
git commit -m "Move PreferencesRepository to internal/repository/"
```

---

### Task 3: Move `MuscleGroupTargetRepository`

**Files:**
- Create: `internal/repository/muscle_targets.go`
- Create: `internal/repository/muscle_targets_test.go`
- Modify: `internal/repository/repository.go` (wire `MuscleTargets` into `New`)

- [ ] **Step 1: Create `internal/repository/muscle_targets.go`.**

```go
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteMuscleGroupTargetRepository struct {
	baseRepository
}

func newSQLiteMuscleGroupTargetRepository(db *sqlite.Database) *sqliteMuscleGroupTargetRepository {
	return &sqliteMuscleGroupTargetRepository{baseRepository: newBaseRepository(db)}
}

// List returns all configured weekly volume targets, ordered by muscle-group
// name. The targets table is seeded by migrations and is not user-editable.
func (r *sqliteMuscleGroupTargetRepository) List(ctx context.Context) (_ []domain.MuscleGroupTarget, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT muscle_group_name, weekly_sets_target
		FROM muscle_group_weekly_targets
		ORDER BY muscle_group_name`)
	if err != nil {
		return nil, fmt.Errorf("query muscle group targets: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var targets []domain.MuscleGroupTarget
	for rows.Next() {
		var t domain.MuscleGroupTarget
		if err = rows.Scan(&t.MuscleGroupName, &t.WeeklySetTarget); err != nil {
			return nil, fmt.Errorf("scan muscle group target: %w", err)
		}
		targets = append(targets, t)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return targets, nil
}
```

(`sqlite` is referenced via the `*sqlite.Database` parameter on the constructor; the import stays.)

- [ ] **Step 2: Wire `MuscleTargets` into `New`.**

Replace the body of `New` in `internal/repository/repository.go` with:

```go
func New(db *sqlite.Database, logger *slog.Logger) *Repositories {
	_ = logger
	prefs := newSQLitePreferencesRepository(db)
	muscleTargets := newSQLiteMuscleGroupTargetRepository(db)
	return &Repositories{ //nolint:exhaustruct // Other fields wired in later tasks.
		Preferences:   prefs,
		MuscleTargets: muscleTargets,
	}
}
```

- [ ] **Step 3: Write the test in `internal/repository/muscle_targets_test.go`.**

```go
package repository_test

import (
	"sort"
	"testing"
)

func TestMuscleGroupTargetRepository_ListReturnsSeededTargets(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	got, err := repos.MuscleTargets.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected seeded muscle-group targets, got 0 rows")
	}
	// Verify alphabetical ordering — the planner relies on it.
	names := make([]string, len(got))
	for i, t := range got {
		names[i] = t.MuscleGroupName
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("muscle-group targets must be sorted by name: %v", names)
	}
	// Verify every row has a positive weekly set target — defensive against
	// schema regressions that would let the planner divide by zero.
	for _, target := range got {
		if target.WeeklySetTarget <= 0 {
			t.Errorf("muscle-group %q has non-positive WeeklySetTarget %d",
				target.MuscleGroupName, target.WeeklySetTarget)
		}
	}
}
```

- [ ] **Step 4: Run the new test.**

Run: `go test -v ./internal/repository/ -run TestMuscleGroupTargetRepository`
Expected: PASS.

- [ ] **Step 5: Run the full tree.**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add internal/repository/muscle_targets.go internal/repository/muscle_targets_test.go internal/repository/repository.go
git commit -m "Move MuscleGroupTargetRepository to internal/repository/"
```

---

### Task 4: Move `FeatureFlagRepository`

**Files:**
- Create: `internal/repository/feature_flags.go`
- Create: `internal/repository/feature_flags_test.go`
- Modify: `internal/repository/repository.go` (wire `FeatureFlags` into `New`)

- [ ] **Step 1: Create `internal/repository/feature_flags.go`.**

```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteFeatureFlagRepository struct {
	baseRepository
}

func newSQLiteFeatureFlagRepository(db *sqlite.Database) *sqliteFeatureFlagRepository {
	return &sqliteFeatureFlagRepository{baseRepository: newBaseRepository(db)}
}

func (r *sqliteFeatureFlagRepository) Get(ctx context.Context, name string) (domain.FeatureFlag, error) {
	var flag domain.FeatureFlag
	var enabled int

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT name, enabled
		FROM feature_flags
		WHERE name = ?`, name).Scan(&flag.Name, &enabled)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.FeatureFlag{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("query feature flag %s: %w", name, err)
	}

	flag.Enabled = enabled == 1
	return flag, nil
}

func (r *sqliteFeatureFlagRepository) Set(ctx context.Context, flag domain.FeatureFlag) error {
	enabled := 0
	if flag.Enabled {
		enabled = 1
	}

	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO feature_flags (name, enabled)
		VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET enabled = excluded.enabled`,
		flag.Name, enabled); err != nil {
		return fmt.Errorf("save feature flag %s: %w", flag.Name, err)
	}
	return nil
}

func (r *sqliteFeatureFlagRepository) List(ctx context.Context) (_ []domain.FeatureFlag, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT name, enabled
		FROM feature_flags
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query feature flags: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var flags []domain.FeatureFlag
	for rows.Next() {
		var flag domain.FeatureFlag
		var enabled int
		if scanErr := rows.Scan(&flag.Name, &enabled); scanErr != nil {
			return nil, fmt.Errorf("scan feature flag: %w", scanErr)
		}
		flag.Enabled = enabled == 1
		flags = append(flags, flag)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feature flags: %w", err)
	}
	return flags, nil
}
```

(`sqlite` is referenced via `*sqlite.Database` in the constructor signature; the import stays.)

- [ ] **Step 2: Wire `FeatureFlags` into `New` in `internal/repository/repository.go`.**

```go
func New(db *sqlite.Database, logger *slog.Logger) *Repositories {
	_ = logger
	prefs := newSQLitePreferencesRepository(db)
	muscleTargets := newSQLiteMuscleGroupTargetRepository(db)
	featureFlags := newSQLiteFeatureFlagRepository(db)
	return &Repositories{ //nolint:exhaustruct // Other fields wired in later tasks.
		Preferences:   prefs,
		MuscleTargets: muscleTargets,
		FeatureFlags:  featureFlags,
	}
}
```

- [ ] **Step 3: Write tests in `internal/repository/feature_flags_test.go`.**

```go
package repository_test

import (
	"errors"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestFeatureFlagRepository_GetMissingReturnsErrNotFound(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	_, err := repos.FeatureFlags.Get(ctx, "nonexistent_flag")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound, got %v", err)
	}
}

func TestFeatureFlagRepository_SetThenGetRoundTrip(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	want := domain.FeatureFlag{Name: "experimental_x", Enabled: true}
	if err := repos.FeatureFlags.Set(ctx, want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := repos.FeatureFlags.Get(ctx, "experimental_x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Errorf("round-trip: want %+v, got %+v", want, got)
	}
}

func TestFeatureFlagRepository_SetUpsertsExisting(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	if err := repos.FeatureFlags.Set(ctx, domain.FeatureFlag{Name: "x", Enabled: true}); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	if err := repos.FeatureFlags.Set(ctx, domain.FeatureFlag{Name: "x", Enabled: false}); err != nil {
		t.Fatalf("second Set: %v", err)
	}
	got, err := repos.FeatureFlags.Get(ctx, "x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Enabled {
		t.Errorf("expected upsert to disable flag, got Enabled=true")
	}
}

func TestFeatureFlagRepository_ListSortedByName(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	for _, name := range []string{"zebra", "apple", "mango"} {
		if err := repos.FeatureFlags.Set(ctx, domain.FeatureFlag{Name: name, Enabled: true}); err != nil {
			t.Fatalf("Set %s: %v", name, err)
		}
	}
	got, err := repos.FeatureFlags.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"apple", "mango", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("List returned %d rows, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("List[%d]: want %q, got %q", i, w, got[i].Name)
		}
	}
}
```

- [ ] **Step 4: Run the new tests.**

Run: `go test -v ./internal/repository/ -run TestFeatureFlagRepository`
Expected: PASS for all four tests.

- [ ] **Step 5: Run the full tree.**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add internal/repository/feature_flags.go internal/repository/feature_flags_test.go internal/repository/repository.go
git commit -m "Move FeatureFlagRepository to internal/repository/"
```

---

### Task 5: Move `ExerciseRepository` (drop bool from `Update`)

**Files:**
- Create: `internal/repository/exercises.go`
- Create: `internal/repository/exercises_test.go`
- Modify: `internal/repository/repository.go` (wire `Exercises` into `New`)

The migrated `Update` signature drops the bool: `func(*domain.Exercise) error`. nil → persist; error → rollback. Today's `workout.exerciseRepository.Update` is the only caller pattern in service.go (`UpdateExercise` always returns `(true, nil)`), so the bool was redundant.

`Get` translates `sql.ErrNoRows` → `domain.ErrNotFound` (today's workout impl does not — Phase 2 fixes that boundary inconsistency so all repos behave the same).

- [ ] **Step 1: Create `internal/repository/exercises.go`.**

```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteExerciseRepository struct {
	baseRepository
}

func newSQLiteExerciseRepository(db *sqlite.Database) *sqliteExerciseRepository {
	return &sqliteExerciseRepository{baseRepository: newBaseRepository(db)}
}

func (r *sqliteExerciseRepository) Get(ctx context.Context, id int) (domain.Exercise, error) {
	var exercise domain.Exercise
	var defaultStartingSeconds, repMin, repMax sql.NullInt64

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT id, name, category, exercise_type, description_markdown,
		       default_starting_seconds, rep_min, rep_max
		FROM exercises
		WHERE id = ?`, id).Scan(
		&exercise.ID,
		&exercise.Name,
		&exercise.Category,
		&exercise.ExerciseType,
		&exercise.DescriptionMarkdown,
		&defaultStartingSeconds,
		&repMin,
		&repMax,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Exercise{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("query exercise: %w", err)
	}
	if defaultStartingSeconds.Valid {
		v := int(defaultStartingSeconds.Int64)
		exercise.DefaultStartingSeconds = &v
	}
	if repMin.Valid {
		v := int(repMin.Int64)
		exercise.RepMin = &v
	}
	if repMax.Valid {
		v := int(repMax.Int64)
		exercise.RepMax = &v
	}

	primaryMuscleGroups, secondaryMuscleGroups, err := r.fetchMuscleGroups(ctx, exercise.ID)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
	}
	exercise.PrimaryMuscleGroups = primaryMuscleGroups
	exercise.SecondaryMuscleGroups = secondaryMuscleGroups

	return exercise, nil
}

func (r *sqliteExerciseRepository) List(ctx context.Context) (_ []domain.Exercise, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, name, category, exercise_type, description_markdown,
		       default_starting_seconds, rep_min, rep_max
		FROM exercises
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query exercises: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var exercises []domain.Exercise
	for rows.Next() {
		var exercise domain.Exercise
		var defaultStartingSeconds, repMin, repMax sql.NullInt64
		if err = rows.Scan(
			&exercise.ID, &exercise.Name, &exercise.Category, &exercise.ExerciseType,
			&exercise.DescriptionMarkdown, &defaultStartingSeconds, &repMin, &repMax,
		); err != nil {
			return nil, fmt.Errorf("scan exercise: %w", err)
		}
		if defaultStartingSeconds.Valid {
			v := int(defaultStartingSeconds.Int64)
			exercise.DefaultStartingSeconds = &v
		}
		if repMin.Valid {
			v := int(repMin.Int64)
			exercise.RepMin = &v
		}
		if repMax.Valid {
			v := int(repMax.Int64)
			exercise.RepMax = &v
		}
		exercises = append(exercises, exercise)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	for i, exercise := range exercises {
		var primary, secondary []string
		primary, secondary, err = r.fetchMuscleGroups(ctx, exercise.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
		}
		exercises[i].PrimaryMuscleGroups = primary
		exercises[i].SecondaryMuscleGroups = secondary
	}
	return exercises, nil
}

func (r *sqliteExerciseRepository) fetchMuscleGroups(
	ctx context.Context,
	exerciseID int,
) (_ []string, _ []string, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT mg.name, emg.is_primary
		FROM exercise_muscle_groups emg
		JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
		WHERE emg.exercise_id = ?`, exerciseID)
	if err != nil {
		return nil, nil, fmt.Errorf("query muscle groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var primary, secondary []string
	for rows.Next() {
		var (
			name      string
			isPrimary bool
		)
		if err = rows.Scan(&name, &isPrimary); err != nil {
			return nil, nil, fmt.Errorf("scan muscle group row: %w", err)
		}
		if isPrimary {
			primary = append(primary, name)
		} else {
			secondary = append(secondary, name)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate muscle group rows: %w", err)
	}
	return primary, secondary, nil
}

func (r *sqliteExerciseRepository) Create(ctx context.Context, ex domain.Exercise) (domain.Exercise, error) {
	created, err := r.set(ctx, ex, false)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("create exercise: %w", err)
	}
	return created, nil
}

// Update reads the exercise, runs fn, and persists the result if fn returned
// nil. fn returning an error rolls back without writing.
func (r *sqliteExerciseRepository) Update(
	ctx context.Context,
	exerciseID int,
	fn func(*domain.Exercise) error,
) error {
	exercise, err := r.Get(ctx, exerciseID)
	if err != nil {
		return fmt.Errorf("get exercise for update: %w", err)
	}
	if err = fn(&exercise); err != nil {
		return err
	}
	if _, err = r.set(ctx, exercise, true); err != nil {
		return fmt.Errorf("save updated exercise: %w", err)
	}
	return nil
}

func (r *sqliteExerciseRepository) set(
	ctx context.Context,
	ex domain.Exercise,
	upsert bool,
) (_ domain.Exercise, err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return ex, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	if upsert {
		if _, err = tx.ExecContext(ctx, `DELETE FROM exercises WHERE id = ?`, ex.ID); err != nil {
			return ex, fmt.Errorf("delete exercise: %w", err)
		}
	}

	var result sql.Result
	if upsert {
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (id, name, category, exercise_type, description_markdown,
			                       default_starting_seconds, rep_min, rep_max)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			ex.ID, ex.Name, ex.Category, ex.ExerciseType, ex.DescriptionMarkdown,
			ex.DefaultStartingSeconds, ex.RepMin, ex.RepMax)
	} else {
		result, err = tx.ExecContext(ctx, `
			INSERT INTO exercises (name, category, exercise_type, description_markdown,
			                       default_starting_seconds, rep_min, rep_max)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ex.Name, ex.Category, ex.ExerciseType, ex.DescriptionMarkdown,
			ex.DefaultStartingSeconds, ex.RepMin, ex.RepMax)
	}
	if err != nil {
		return ex, fmt.Errorf("insert exercise: %w", err)
	}

	if !upsert {
		var id int64
		if id, err = result.LastInsertId(); err != nil {
			return ex, fmt.Errorf("get last insert ID: %w", err)
		}
		ex.ID = int(id)
	}

	if err = r.insertMuscleGroups(ctx, tx, ex.ID, ex.PrimaryMuscleGroups, true); err != nil {
		return ex, fmt.Errorf("insert primary muscle groups: %w", err)
	}
	if err = r.insertMuscleGroups(ctx, tx, ex.ID, ex.SecondaryMuscleGroups, false); err != nil {
		return ex, fmt.Errorf("insert secondary muscle groups: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return ex, fmt.Errorf("commit transaction: %w", err)
	}
	return ex, nil
}

func (r *sqliteExerciseRepository) insertMuscleGroups(
	ctx context.Context,
	tx *sql.Tx,
	exerciseID int,
	muscleGroups []string,
	isPrimary bool,
) error {
	for _, muscleGroup := range muscleGroups {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary)
			VALUES (?, ?, ?)`,
			exerciseID, muscleGroup, isPrimary); err != nil {
			return fmt.Errorf("insert muscle group %s: %w", muscleGroup, err)
		}
	}
	return nil
}

func (r *sqliteExerciseRepository) ListMuscleGroups(ctx context.Context) (_ []string, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT name
		FROM muscle_groups
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query muscle groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var muscleGroups []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan muscle group: %w", err)
		}
		muscleGroups = append(muscleGroups, name)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return muscleGroups, nil
}
```

- [ ] **Step 2: Wire `Exercises` into `New` in `internal/repository/repository.go`.**

```go
func New(db *sqlite.Database, logger *slog.Logger) *Repositories {
	_ = logger
	prefs := newSQLitePreferencesRepository(db)
	muscleTargets := newSQLiteMuscleGroupTargetRepository(db)
	featureFlags := newSQLiteFeatureFlagRepository(db)
	exercises := newSQLiteExerciseRepository(db)
	return &Repositories{ //nolint:exhaustruct // Sessions wired in Task 6.
		Preferences:   prefs,
		MuscleTargets: muscleTargets,
		FeatureFlags:  featureFlags,
		Exercises:     exercises,
	}
}
```

- [ ] **Step 3: Write tests in `internal/repository/exercises_test.go`.**

```go
package repository_test

import (
	"errors"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func ptrInt(v int) *int { return &v }

func newTestExercise(name string) domain.Exercise {
	return domain.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds nil for non-time_based.
		Name:                  name,
		Category:              domain.CategoryUpper,
		ExerciseType:          domain.ExerciseTypeWeighted,
		DescriptionMarkdown:   "# " + name,
		PrimaryMuscleGroups:   []string{"chest"},
		SecondaryMuscleGroups: []string{"triceps"},
		RepMin:                ptrInt(5),
		RepMax:                ptrInt(10),
	}
}

func TestExerciseRepository_GetMissingReturnsErrNotFound(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	_, err := repos.Exercises.Get(ctx, 999_999)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound for missing exercise, got %v", err)
	}
}

func TestExerciseRepository_CreateAssignsID(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise("Bench"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID <= 0 {
		t.Errorf("expected assigned positive ID, got %d", created.ID)
	}
}

func TestExerciseRepository_CreateThenGetRoundTrip(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise("Bench"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := repos.Exercises.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Bench" {
		t.Errorf("Name: want Bench, got %q", got.Name)
	}
	if got.Category != domain.CategoryUpper {
		t.Errorf("Category: want %q, got %q", domain.CategoryUpper, got.Category)
	}
	if len(got.PrimaryMuscleGroups) != 1 || got.PrimaryMuscleGroups[0] != "chest" {
		t.Errorf("PrimaryMuscleGroups: want [chest], got %v", got.PrimaryMuscleGroups)
	}
	if len(got.SecondaryMuscleGroups) != 1 || got.SecondaryMuscleGroups[0] != "triceps" {
		t.Errorf("SecondaryMuscleGroups: want [triceps], got %v", got.SecondaryMuscleGroups)
	}
}

func TestExerciseRepository_UpdatePersistsChanges(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise("Bench"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err = repos.Exercises.Update(ctx, created.ID, func(ex *domain.Exercise) error {
		ex.Name = "Bench Press"
		ex.PrimaryMuscleGroups = []string{"chest", "shoulders"}
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := repos.Exercises.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Bench Press" {
		t.Errorf("Name after update: want Bench Press, got %q", got.Name)
	}
	if len(got.PrimaryMuscleGroups) != 2 {
		t.Errorf("PrimaryMuscleGroups after update: want 2, got %v", got.PrimaryMuscleGroups)
	}
}

func TestExerciseRepository_UpdateRollsBackOnError(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	created, err := repos.Exercises.Create(ctx, newTestExercise("Bench"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	wantErr := errors.New("user-injected failure")
	if err = repos.Exercises.Update(ctx, created.ID, func(ex *domain.Exercise) error {
		ex.Name = "MUTATED"
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Update: want injected error, got %v", err)
	}
	got, err := repos.Exercises.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Bench" {
		t.Errorf("expected rollback to preserve original name, got %q", got.Name)
	}
}
```

- [ ] **Step 4: Run the new tests.**

Run: `go test -v ./internal/repository/ -run TestExerciseRepository`
Expected: PASS for all five tests.

- [ ] **Step 5: Run the full tree.**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add internal/repository/exercises.go internal/repository/exercises_test.go internal/repository/repository.go
git commit -m "Move ExerciseRepository to internal/repository/; drop bool from Update"
```

---

### Task 6: Move `SessionRepository` (substantive change)

This is the load-bearing task. Three changes from the workout impl:

1. **Closure type:** `func(*sessionAggregate) (bool, error)` → `func(*domain.Session) error`. nil → diff and persist; error → rollback. The bool disappears.
2. **Hydration in Get/List:** the repo holds a reference to `ExerciseRepository` and populates `ExerciseSet.Exercise` per slot (replacing `enrichSessionAggregate` in service.go).
3. **Persistence types deleted:** `sessionAggregate` / `exerciseSetAggregate` / `datedExerciseSetAggregate` are gone. Internal scan helpers operate directly on `domain.Session` / `domain.ExerciseSet` / `domain.ExerciseSetHistory`.

The diff strategy is unchanged: delete the `workout_sessions` row inside the tx (CASCADE clears `workout_exercise` and `exercise_sets`), then re-insert. Pre-existing `ExerciseSet.ID` values are passed back into `INSERT ... RETURNING id` so URL-stable slot IDs survive a delete-and-reinsert cycle.

**Files:**
- Create: `internal/repository/sessions.go`
- Create: `internal/repository/sessions_test.go`
- Modify: `internal/repository/repository.go` (wire `Sessions` into `New`, injecting `Exercises`)

- [ ] **Step 1: Create `internal/repository/sessions.go`.**

```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// queryer is satisfied by both *sql.DB and *sql.Tx, so read helpers can run
// either standalone or inside an open transaction.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type sqliteSessionRepository struct {
	baseRepository
	exercises ExerciseRepository
}

func newSQLiteSessionRepository(db *sqlite.Database, exercises ExerciseRepository) *sqliteSessionRepository {
	return &sqliteSessionRepository{
		baseRepository: newBaseRepository(db),
		exercises:      exercises,
	}
}

func (r *sqliteSessionRepository) List(ctx context.Context, sinceDate time.Time) (_ []domain.Session, err error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	sinceDateStr := formatDate(sinceDate)

	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type
		FROM workout_sessions
		WHERE user_id = ? AND workout_date >= ?
		ORDER BY workout_date DESC`,
		userID, sinceDateStr)
	if err != nil {
		return nil, fmt.Errorf("query workout history: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var sessions []domain.Session
	for rows.Next() {
		var (
			workoutDateStr    string
			difficultyRating  sql.NullInt32
			startedAtStr      sql.NullString
			completedAtStr    sql.NullString
			periodizationType domain.PeriodizationType
		)
		if err = rows.Scan(&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		var session domain.Session
		session, err = parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType)
		if err != nil {
			return nil, err
		}
		var exerciseSets []domain.ExerciseSet
		exerciseSets, err = r.loadExerciseSets(ctx, r.db.ReadOnly, userID, session.Date)
		if err != nil {
			return nil, err
		}
		session.ExerciseSets = exerciseSets
		sessions = append(sessions, session)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return sessions, nil
}

func (r *sqliteSessionRepository) Get(ctx context.Context, date time.Time) (domain.Session, error) {
	return r.get(ctx, r.db.ReadOnly, date)
}

func (r *sqliteSessionRepository) get(ctx context.Context, q queryer, date time.Time) (domain.Session, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := formatDate(date)

	var (
		workoutDateStr    string
		difficultyRating  sql.NullInt32
		startedAtStr      sql.NullString
		completedAtStr    sql.NullString
		periodizationType domain.PeriodizationType
	)
	err := q.QueryRowContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type
		FROM workout_sessions
		WHERE user_id = ? AND workout_date = ?`,
		userID, dateStr).Scan(&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("query session: %w", err)
	}

	session, err := parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType)
	if err != nil {
		return domain.Session{}, err
	}

	exerciseSets, err := r.loadExerciseSets(ctx, q, userID, session.Date)
	if err != nil {
		return domain.Session{}, err
	}
	session.ExerciseSets = exerciseSets

	return session, nil
}

// Update modifies an existing session within a single transaction. The read
// happens inside the same BEGIN IMMEDIATE transaction as the write so concurrent
// updates cannot interleave a read-modify-write race. fn returning an error
// rolls back without writing; nil commits the diff. Sentinel errors from
// domain (e.g. ErrAlreadyStarted) propagate through unchanged.
func (r *sqliteSessionRepository) Update(
	ctx context.Context,
	date time.Time,
	fn func(*domain.Session) error,
) (err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	session, err := r.get(ctx, tx, date)
	if err != nil {
		return fmt.Errorf("get session for update: %w", err)
	}

	if err = fn(&session); err != nil {
		return err
	}

	if err = r.deleteSession(ctx, tx, date); err != nil {
		return err
	}
	if err = r.insertSession(ctx, tx, session); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (r *sqliteSessionRepository) CreateBatch(ctx context.Context, sessions []domain.Session) (err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()
	for _, sess := range sessions {
		if err = r.insertSession(ctx, tx, sess); err != nil {
			return fmt.Errorf("insert session %s: %w", formatDate(sess.Date), err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit batch sessions: %w", err)
	}
	return nil
}

func (r *sqliteSessionRepository) insertSession(ctx context.Context, tx *sql.Tx, sess domain.Session) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := formatDate(sess.Date)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workout_sessions (
			user_id, workout_date, difficulty_rating, started_at, completed_at, periodization_type
		) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, dateStr, sess.DifficultyRating,
		formatTimestamp(sess.StartedAt), formatTimestamp(sess.CompletedAt),
		sess.PeriodizationType); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	if err := r.saveExerciseSets(ctx, tx, sess.Date, sess.ExerciseSets); err != nil {
		return fmt.Errorf("save exercise sets: %w", err)
	}
	return nil
}

func (r *sqliteSessionRepository) deleteSession(ctx context.Context, tx *sql.Tx, date time.Time) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := formatDate(date)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM workout_sessions
		WHERE user_id = ? AND workout_date = ?`,
		userID, dateStr); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// parseSessionRow converts the workout_sessions row scalars into a partial
// domain.Session (ExerciseSets is filled in by loadExerciseSets).
func parseSessionRow(
	workoutDateStr string,
	difficultyRating sql.NullInt32,
	startedAtStr sql.NullString,
	completedAtStr sql.NullString,
	periodizationType domain.PeriodizationType,
) (domain.Session, error) {
	date, err := time.Parse(dateFormat, workoutDateStr)
	if err != nil {
		return domain.Session{}, fmt.Errorf("parse workout date: %w", err)
	}
	session := domain.Session{ //nolint:exhaustruct // ExerciseSets filled by caller.
		Date:              date,
		PeriodizationType: periodizationType,
	}
	if difficultyRating.Valid {
		rating := int(difficultyRating.Int32)
		session.DifficultyRating = &rating
	}
	if session.StartedAt, err = parseTimestamp(startedAtStr); err != nil {
		return domain.Session{}, fmt.Errorf("parse started_at: %w", err)
	}
	if session.CompletedAt, err = parseTimestamp(completedAtStr); err != nil {
		return domain.Session{}, fmt.Errorf("parse completed_at: %w", err)
	}
	return session, nil
}

// loadExerciseSetsRow holds one row of the loadExerciseSets join.
type loadExerciseSetsRow struct {
	weID                 int
	exerciseID           int
	warmupCompletedAtStr sql.NullString
	setNumber            sql.NullInt32
	weightKg             sql.NullFloat64
	targetValue          sql.NullInt32
	completedValue       sql.NullInt32
	completedAtStr       sql.NullString
	signalStr            sql.NullString
}

// loadExerciseSets fetches all exercise slots for a session, including ones
// with no sets yet (e.g. just-swapped exercises). The driving table is
// workout_exercise so empty slots still appear; sets are LEFT-JOINed in. Each
// slot is hydrated by calling exercises.Get for its exercise_id — preserving
// today's N+1 pattern (relocated from service.enrichSessionAggregate).
func (r *sqliteSessionRepository) loadExerciseSets(
	ctx context.Context,
	q queryer,
	userID int,
	date time.Time,
) (_ []domain.ExerciseSet, err error) {
	dateStr := formatDate(date)

	rows, err := q.QueryContext(ctx, `
		SELECT we.id, we.exercise_id, we.warmup_completed_at,
		       es.set_number, es.weight_kg, es.target_value,
		       es.completed_value, es.completed_at, es.signal
		FROM workout_exercise we
		LEFT JOIN exercise_sets es ON es.workout_exercise_id = we.id
		WHERE we.workout_user_id = ? AND we.workout_date = ?
		ORDER BY we.id, es.set_number`,
		userID, dateStr)
	if err != nil {
		return nil, fmt.Errorf("query exercise sets: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var exerciseSets []domain.ExerciseSet
	var current *domain.ExerciseSet
	flush := func() {
		if current != nil {
			exerciseSets = append(exerciseSets, *current)
		}
	}

	for rows.Next() {
		var row loadExerciseSetsRow
		if err = rows.Scan(&row.weID, &row.exerciseID, &row.warmupCompletedAtStr,
			&row.setNumber, &row.weightKg, &row.targetValue,
			&row.completedValue, &row.completedAtStr, &row.signalStr); err != nil {
			return nil, fmt.Errorf("scan exercise set: %w", err)
		}

		if current == nil || row.weID != current.ID {
			flush()
			started, startErr := r.startExerciseSet(ctx, row)
			if startErr != nil {
				return nil, startErr
			}
			current = &started
		}

		// LEFT JOIN can yield a workout_exercise row with no sets (set_number IS NULL).
		if !row.setNumber.Valid {
			continue
		}
		set, parseErr := buildSet(row)
		if parseErr != nil {
			return nil, parseErr
		}
		current.Sets = append(current.Sets, set)
	}
	flush()

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return exerciseSets, nil
}

// startExerciseSet constructs a fresh domain.ExerciseSet with the hydrated
// Exercise filled in from the exercises repository.
func (r *sqliteSessionRepository) startExerciseSet(
	ctx context.Context,
	row loadExerciseSetsRow,
) (domain.ExerciseSet, error) {
	warmupCompletedAt, err := parseWarmupCompletedAtTimestamp(row.warmupCompletedAtStr)
	if err != nil {
		return domain.ExerciseSet{}, err
	}
	exercise, err := r.exercises.Get(ctx, row.exerciseID)
	if err != nil {
		return domain.ExerciseSet{}, fmt.Errorf("hydrate exercise %d: %w", row.exerciseID, err)
	}
	return domain.ExerciseSet{
		ID:                row.weID,
		Exercise:          exercise,
		Sets:              []domain.Set{},
		WarmupCompletedAt: warmupCompletedAt,
	}, nil
}

func buildSet(row loadExerciseSetsRow) (domain.Set, error) {
	set := domain.Set{ //nolint:exhaustruct // CompletedValue, CompletedAt, Signal populated below.
		TargetValue: int(row.targetValue.Int32),
	}
	if row.weightKg.Valid {
		w := row.weightKg.Float64
		set.WeightKg = &w
	}
	if row.completedValue.Valid {
		c := int(row.completedValue.Int32)
		set.CompletedValue = &c
	}
	if err := parseCompletedAtTimestamp(row.completedAtStr, &set); err != nil {
		return domain.Set{}, err
	}
	if row.signalStr.Valid {
		s := domain.Signal(row.signalStr.String)
		set.Signal = &s
	}
	return set, nil
}

func parseCompletedAtTimestamp(completedAtStr sql.NullString, set *domain.Set) error {
	if !completedAtStr.Valid {
		return nil
	}
	completedAt, parseErr := parseTimestamp(completedAtStr)
	if parseErr != nil {
		return fmt.Errorf("parse completed_at timestamp: %w", parseErr)
	}
	if !completedAt.IsZero() {
		set.CompletedAt = &completedAt
	}
	return nil
}

func parseWarmupCompletedAtTimestamp(warmupCompletedAtStr sql.NullString) (*time.Time, error) {
	if !warmupCompletedAtStr.Valid {
		return nil, nil //nolint:nilnil // Valid case for optional timestamp.
	}
	warmupTime, parseErr := parseTimestamp(warmupCompletedAtStr)
	if parseErr != nil {
		return nil, fmt.Errorf("parse warmup_completed_at timestamp: %w", parseErr)
	}
	if warmupTime.IsZero() {
		return nil, nil //nolint:nilnil // Valid case for zero timestamp.
	}
	return &warmupTime, nil
}

func (r *sqliteSessionRepository) ListSetsForExerciseSince(
	ctx context.Context,
	exerciseID int,
	sinceDate time.Time,
) (_ []domain.ExerciseSetHistory, err error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	sinceDateStr := formatDate(sinceDate)

	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT we.workout_date, es.weight_kg, es.target_value,
		       es.completed_value, es.completed_at, we.warmup_completed_at, es.signal
		FROM workout_exercise we
		JOIN exercise_sets es ON es.workout_exercise_id = we.id
		WHERE we.workout_user_id = ? AND we.exercise_id = ? AND we.workout_date >= ?
		ORDER BY we.workout_date DESC, es.set_number`,
		userID, exerciseID, sinceDateStr)
	if err != nil {
		return nil, fmt.Errorf("query exercise sets for exercise: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var result []domain.ExerciseSetHistory
	var current domain.ExerciseSetHistory
	currentSeen := false

	for rows.Next() {
		workoutDateStr, set, scanErr := scanHistoryRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		date, parseErr := time.Parse(dateFormat, workoutDateStr)
		if parseErr != nil {
			return nil, fmt.Errorf("parse workout date: %w", parseErr)
		}
		if !currentSeen || !date.Equal(current.Date) {
			if currentSeen {
				result = append(result, current)
			}
			current = domain.ExerciseSetHistory{
				Date: date,
				Sets: []domain.Set{},
			}
			currentSeen = true
		}
		current.Sets = append(current.Sets, set)
	}
	if currentSeen {
		result = append(result, current)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return result, nil
}

func scanHistoryRow(rows *sql.Rows) (string, domain.Set, error) {
	var (
		workoutDateStr       string
		set                  domain.Set
		completedAtStr       sql.NullString
		warmupCompletedAtStr sql.NullString // unused but selected to match Scan arity.
		signalStr            sql.NullString
	)
	if err := rows.Scan(&workoutDateStr, &set.WeightKg, &set.TargetValue,
		&set.CompletedValue, &completedAtStr, &warmupCompletedAtStr, &signalStr); err != nil {
		return "", domain.Set{}, fmt.Errorf("scan exercise set row: %w", err)
	}
	if err := parseCompletedAtTimestamp(completedAtStr, &set); err != nil {
		return "", domain.Set{}, err
	}
	if signalStr.Valid {
		s := domain.Signal(signalStr.String)
		set.Signal = &s
	}
	return workoutDateStr, set, nil
}

func (r *sqliteSessionRepository) GetLatestStartingWeightBefore(
	ctx context.Context,
	exerciseID int,
	beforeDate time.Time,
) (domain.LatestStartingSet, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	beforeDateStr := formatDate(beforeDate)

	var (
		weightKg   float64
		periodType string
	)
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
		  AND es.completed_value IS NOT NULL
		  AND es.weight_kg IS NOT NULL
		  AND es.signal IN ('on_target', 'too_light')
		ORDER BY we.workout_date DESC, es.set_number DESC
		LIMIT 1`,
		userID, exerciseID, beforeDateStr).Scan(&weightKg, &periodType)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LatestStartingSet{}, nil //nolint:exhaustruct // Caller handles empty.
	}
	if err != nil {
		return domain.LatestStartingSet{}, fmt.Errorf("query latest starting weight: %w", err)
	}
	return domain.LatestStartingSet{
		WeightKg:          weightKg,
		PeriodizationType: domain.PeriodizationType(periodType),
	}, nil
}

func (r *sqliteSessionRepository) GetLatestSuccessfulSecondsBefore(
	ctx context.Context,
	exerciseID int,
	beforeDate time.Time,
) (int, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	var seconds int
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT es.completed_value
		FROM exercise_sets es
		JOIN workout_exercise we ON we.id = es.workout_exercise_id
		WHERE we.workout_user_id = ?
		  AND we.exercise_id = ?
		  AND we.workout_date < ?
		  AND es.completed_value IS NOT NULL
		  AND es.signal IN ('on_target', 'too_light')
		ORDER BY we.workout_date DESC, es.set_number DESC
		LIMIT 1`,
		userID, exerciseID, formatDate(beforeDate)).Scan(&seconds)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("query latest successful seconds: %w", err)
	}
	return seconds, nil
}

func (r *sqliteSessionRepository) DeleteWeek(ctx context.Context, monday time.Time) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	sunday := monday.AddDate(0, 0, 6)
	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		DELETE FROM workout_sessions
		WHERE user_id = ? AND workout_date >= ? AND workout_date <= ?`,
		userID, formatDate(monday), formatDate(sunday)); err != nil {
		return fmt.Errorf("delete week sessions: %w", err)
	}
	return nil
}

func (r *sqliteSessionRepository) CountCompleted(ctx context.Context) (int, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	var count int
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM workout_sessions
		WHERE user_id = ? AND completed_at IS NOT NULL`,
		userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count completed sessions: %w", err)
	}
	return count, nil
}

// saveExerciseSets writes the workout_exercise rows and their child
// exercise_sets for a session. Pre-existing IDs are preserved so URL-stable
// slot IDs survive delete-and-reinsert cycles in Update; new aggregates
// (ID == 0) get an auto-assigned id.
func (r *sqliteSessionRepository) saveExerciseSets(
	ctx context.Context,
	tx *sql.Tx,
	date time.Time,
	exerciseSets []domain.ExerciseSet,
) error {
	dateStr := formatDate(date)
	userID := contexthelpers.AuthenticatedUserID(ctx)

	for _, exerciseSet := range exerciseSets {
		var idArg any
		if exerciseSet.ID > 0 {
			idArg = exerciseSet.ID
		}
		var warmupArg any
		if exerciseSet.WarmupCompletedAt != nil {
			warmupArg = formatTimestamp(*exerciseSet.WarmupCompletedAt)
		}
		var weID int
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO workout_exercise (
				id, workout_user_id, workout_date, exercise_id, warmup_completed_at
			) VALUES (?, ?, ?, ?, ?)
			RETURNING id`,
			idArg, userID, dateStr, exerciseSet.Exercise.ID, warmupArg).Scan(&weID); err != nil {
			return fmt.Errorf("insert workout exercise: %w", err)
		}
		for i, set := range exerciseSet.Sets {
			var completedAtStr any
			if set.CompletedAt != nil {
				completedAtStr = formatTimestamp(*set.CompletedAt)
			}
			var signalValue any
			if set.Signal != nil {
				signalValue = string(*set.Signal)
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO exercise_sets (
					workout_exercise_id, set_number,
					weight_kg, target_value, completed_value, completed_at, signal
				) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				weID, i+1,
				set.WeightKg, set.TargetValue, set.CompletedValue, completedAtStr, signalValue); err != nil {
				return fmt.Errorf("insert exercise set: %w", err)
			}
		}
	}
	return nil
}
```

- [ ] **Step 2: Wire `Sessions` into `New` (final form).**

Replace `New` in `internal/repository/repository.go` with:

```go
func New(db *sqlite.Database, logger *slog.Logger) *Repositories {
	_ = logger // reserved for future per-repo logging; unused today.
	prefs := newSQLitePreferencesRepository(db)
	muscleTargets := newSQLiteMuscleGroupTargetRepository(db)
	featureFlags := newSQLiteFeatureFlagRepository(db)
	exercises := newSQLiteExerciseRepository(db)
	sessions := newSQLiteSessionRepository(db, exercises)
	return &Repositories{
		Preferences:   prefs,
		MuscleTargets: muscleTargets,
		FeatureFlags:  featureFlags,
		Exercises:     exercises,
		Sessions:      sessions,
	}
}
```

- [ ] **Step 3: Write the round-trip + slot-ID-stability + hydration + ErrNotFound tests in `internal/repository/sessions_test.go`.**

```go
package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func newTestExerciseFor(t *testing.T) domain.Exercise {
	t.Helper()
	return domain.Exercise{ //nolint:exhaustruct // DefaultStartingSeconds nil for non-time_based.
		Name:                  "Bench Press",
		Category:              domain.CategoryUpper,
		ExerciseType:          domain.ExerciseTypeWeighted,
		DescriptionMarkdown:   "# Bench Press",
		PrimaryMuscleGroups:   []string{"chest"},
		SecondaryMuscleGroups: []string{"triceps"},
		RepMin:                ptrInt(5),
		RepMax:                ptrInt(10),
	}
}

func TestSessionRepository_GetMissingReturnsErrNotFound(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	missing := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := repos.Sessions.Get(ctx, missing)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("want domain.ErrNotFound, got %v", err)
	}
}

func TestSessionRepository_CreateBatchThenGetHydratesExercise(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}

	date := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero by design.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned by DB; WarmupCompletedAt nil.
				Exercise: exercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	got, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.ExerciseSets) != 1 {
		t.Fatalf("want 1 ExerciseSet, got %d", len(got.ExerciseSets))
	}
	hydrated := got.ExerciseSets[0].Exercise
	if hydrated.ID != exercise.ID {
		t.Errorf("Exercise.ID: want %d, got %d", exercise.ID, hydrated.ID)
	}
	if hydrated.Name != "Bench Press" {
		t.Errorf("Exercise.Name: want Bench Press, got %q", hydrated.Name)
	}
	if len(hydrated.PrimaryMuscleGroups) != 1 || hydrated.PrimaryMuscleGroups[0] != "chest" {
		t.Errorf("Exercise.PrimaryMuscleGroups: want [chest], got %v", hydrated.PrimaryMuscleGroups)
	}
}

func TestSessionRepository_UpdatePreservesSlotID(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	date := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned by DB.
				Exercise: exercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	fetched, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get after create: %v", err)
	}
	originalSlotID := fetched.ExerciseSets[0].ID
	if originalSlotID == 0 {
		t.Fatalf("expected non-zero slot ID after insert")
	}

	if err = repos.Sessions.Update(ctx, date, func(s *domain.Session) error {
		s.StartedAt = time.Now().UTC()
		return nil
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	after, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}
	if len(after.ExerciseSets) != 1 {
		t.Fatalf("want 1 slot after Update, got %d", len(after.ExerciseSets))
	}
	if after.ExerciseSets[0].ID != originalSlotID {
		t.Errorf("slot ID changed across Update: %d → %d", originalSlotID, after.ExerciseSets[0].ID)
	}
	if after.StartedAt.IsZero() {
		t.Errorf("expected StartedAt to be set after Update closure")
	}
}

func TestSessionRepository_UpdateRollsBackOnError(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	date := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned by DB.
				Exercise: exercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	wantErr := errors.New("user-injected failure")
	if err = repos.Sessions.Update(ctx, date, func(s *domain.Session) error {
		s.StartedAt = time.Now().UTC()
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Update: want injected error, got %v", err)
	}

	after, err := repos.Sessions.Get(ctx, date)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !after.StartedAt.IsZero() {
		t.Errorf("expected rollback to leave StartedAt zero, got %v", after.StartedAt)
	}
}

func TestSessionRepository_UpdatePropagatesDomainSentinel(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	date := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	now := time.Now().UTC()
	sess := domain.Session{ //nolint:exhaustruct // CompletedAt zero.
		Date:              date,
		PeriodizationType: domain.PeriodizationStrength,
		StartedAt:         now,
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // ID assigned by DB.
				Exercise: exercise,
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{sess}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	err = repos.Sessions.Update(ctx, date, func(s *domain.Session) error {
		return s.Start(time.Now().UTC()) // already started: returns ErrAlreadyStarted
	})
	if !errors.Is(err, domain.ErrAlreadyStarted) {
		t.Errorf("want domain.ErrAlreadyStarted to propagate, got %v", err)
	}
}

func TestSessionRepository_DeleteWeek(t *testing.T) {
	ctx, repos := setupTestRepos(t)

	monday := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	exercise, err := repos.Exercises.Create(ctx, newTestExerciseFor(t))
	if err != nil {
		t.Fatalf("Create exercise: %v", err)
	}
	mkSession := func(day time.Time) domain.Session {
		return domain.Session{ //nolint:exhaustruct // StartedAt/CompletedAt zero.
			Date:              day,
			PeriodizationType: domain.PeriodizationStrength,
			ExerciseSets: []domain.ExerciseSet{
				{ //nolint:exhaustruct // ID assigned by DB.
					Exercise: exercise,
					Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
				},
			},
		}
	}
	if err = repos.Sessions.CreateBatch(ctx, []domain.Session{
		mkSession(monday),
		mkSession(monday.AddDate(0, 0, 2)),
		mkSession(monday.AddDate(0, 0, 4)),
	}); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}

	if err = repos.Sessions.DeleteWeek(ctx, monday); err != nil {
		t.Fatalf("DeleteWeek: %v", err)
	}

	for _, day := range []time.Time{monday, monday.AddDate(0, 0, 2), monday.AddDate(0, 0, 4)} {
		_, err = repos.Sessions.Get(ctx, day)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Get %s after DeleteWeek: want ErrNotFound, got %v", day.Format(time.DateOnly), err)
		}
	}
}
```

- [ ] **Step 4: Run the new tests.**

Run: `go test -v ./internal/repository/ -run TestSessionRepository`
Expected: PASS for all six tests.

- [ ] **Step 5: Run the full tree.**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit.**

```bash
git add internal/repository/sessions.go internal/repository/sessions_test.go internal/repository/repository.go
git commit -m "Move SessionRepository to internal/repository/; closure operates on *domain.Session"
```

---

### Task 7: Cut over `internal/workout/service.go`

The substantive consumer change. Service stops referencing `*workout.repository`, `sessionAggregate`, `exerciseSetAggregate`, `enrichSessionAggregate`, `RecordSetCompletion`, and `RecordTimedSetCompletion`. Service starts using `*repository.Repositories`, aggregate methods on `domain.Session`, and a unified `RecordSet`. The two cmd/web/ handler call sites for the recording methods get one-liner edits, and the corresponding `service_test.go` call sites get the same edit.

**Files:**
- Modify: `internal/workout/service.go` (the whole file is in scope; key methods listed below)
- Modify: `cmd/web/handler-exerciseset.go` (two call sites)
- Modify: `internal/workout/service_test.go` (two call sites: lines ~1625 and ~1732)

- [ ] **Step 1: Replace the `Service` struct field and `NewService`.**

In `internal/workout/service.go`, replace:

```go
type Service struct {
	repo         *repository
	db           *sqlite.Database
	logger       *slog.Logger
	openaiAPIKey string
}

func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	factory := newRepositoryFactory(db, logger)
	return &Service{
		repo:         factory.newRepository(),
		db:           db,
		logger:       logger,
		openaiAPIKey: openaiAPIKey,
	}
}
```

with:

```go
type Service struct {
	repos        *repository.Repositories
	db           *sqlite.Database
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

Also add the import `"github.com/myrjola/petrapp/internal/repository"` to the imports block.

- [ ] **Step 2: Update `GetUserPreferences` and `SaveUserPreferences`.**

Replace `s.repo.prefs.Get` with `s.repos.Preferences.Get`, and `s.repo.prefs.Set` with `s.repos.Preferences.Set`. The function bodies are otherwise unchanged.

- [ ] **Step 3: Update `RegenerateWeeklyPlanIfUnstarted`.**

Replace `s.repo.sessions.List` and `s.repo.sessions.DeleteWeek` with `s.repos.Sessions.List` and `s.repos.Sessions.DeleteWeek`. Body otherwise unchanged.

- [ ] **Step 4: Rewrite `ResolveWeeklySchedule` (sessions are now hydrated by the repo).**

Replace the body with:

```go
func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]Session, error) {
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

	workouts := make([]Session, 7)
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		sess, getErr := s.repos.Sessions.Get(ctx, day)
		if getErr != nil && !errors.Is(getErr, domain.ErrNotFound) {
			return nil, fmt.Errorf("get session %s: %w", day.Format(time.DateOnly), getErr)
		}
		if errors.Is(getErr, domain.ErrNotFound) {
			workouts[i] = Session{ //nolint:exhaustruct // Rest days have no exercise data.
				Date: day,
			}
			continue
		}
		workouts[i] = sess
	}
	return workouts, nil
}
```

- [ ] **Step 5: Rewrite `generateWeeklyPlan` (no aggregate conversion needed).**

Replace the body with:

```go
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
```

- [ ] **Step 6: Rewrite `GetSession` (no enrichment needed).**

Replace the body with:

```go
func (s *Service) GetSession(ctx context.Context, date time.Time) (Session, error) {
	sess, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return Session{}, fmt.Errorf("get session %s: %w", date.Format(time.DateOnly), err)
	}
	return sess, nil
}
```

Delete the entire `enrichSessionAggregate` method.

- [ ] **Step 7: Rewrite `StartSession` (adopt `Session.Start`; swallow `ErrAlreadyStarted`).**

Replace the body with:

```go
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
```

- [ ] **Step 8: Rewrite `CompleteSession`, `SaveFeedback`, `MarkWarmupComplete`, `UpdateSetWeight`, `UpdateCompletedValue`.**

Each adopts an aggregate method:

```go
func (s *Service) CompleteSession(ctx context.Context, date time.Time) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.Complete(time.Now())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

func (s *Service) SaveFeedback(ctx context.Context, date time.Time, difficulty int) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.SetDifficulty(difficulty)
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

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
```

Note: the older `UpdateSetWeight` looked up by `ExerciseID` (not the slot's `ID`). The new aggregate method looks up by slot ID. Today's call sites in `cmd/web/` already pass the workout_exercise.id (the URL slug), so the call signature stays compatible — only the parameter name changes for clarity. Verify by grepping `app.workoutService.UpdateSetWeight(` in `cmd/web/`: the third arg is the slot ID (URL `{exerciseID}`), which IS the workout_exercise.id by the time it reaches service. (Today's service incorrectly named it `exerciseID`; the new code is correct.)

- [ ] **Step 9: Replace `RecordSetCompletion` + `RecordTimedSetCompletion` with a unified `RecordSet`.**

Delete both old methods. Add:

```go
// RecordSet atomically persists the signal, weight (nil for time-based sets),
// completed value (reps or seconds depending on exercise type), and timestamp.
func (s *Service) RecordSet(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	signal Signal,
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

- [ ] **Step 10: Rewrite `GetSessionsWithExerciseSince` (no manual enrichment).**

Replace the body with:

```go
func (s *Service) GetSessionsWithExerciseSince(ctx context.Context, exerciseID int, since time.Time) (
	[]Session, error) {
	sessions, err := s.repos.Sessions.List(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}

	var result []Session
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
```

- [ ] **Step 11: Rewrite `GetExerciseSetsForExerciseSince` to consume `[]domain.ExerciseSetHistory`.**

Replace the body with:

```go
func (s *Service) GetExerciseSetsForExerciseSince(ctx context.Context, exerciseID int, since time.Time) (
	ExerciseProgress, error) {
	histories, err := s.repos.Sessions.ListSetsForExerciseSince(ctx, exerciseID, since)
	if err != nil {
		return ExerciseProgress{}, fmt.Errorf("list sets for exercise: %w", err)
	}

	ex, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return ExerciseProgress{}, fmt.Errorf("get exercise %d: %w", exerciseID, err)
	}

	entries := make([]ExerciseProgressEntry, 0, len(histories))
	for _, h := range histories {
		var completedSets []Set
		for _, set := range h.Sets {
			if set.CompletedValue != nil {
				completedSets = append(completedSets, set)
			}
		}
		if len(completedSets) > 0 {
			entries = append(entries, ExerciseProgressEntry{
				Date: h.Date,
				Sets: completedSets,
			})
		}
	}

	return ExerciseProgress{Exercise: ex, Entries: entries}, nil
}
```

- [ ] **Step 12: Rewrite `BuildProgression` and `BuildTimedProgression`.**

Both call `s.repo.sessions.Get` and `s.repo.exercises.Get` today, then walk `sess.ExerciseSets` looking for matching `ExerciseID`. After the cutover, the lookup key becomes `es.Exercise.ID`:

```go
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

- [ ] **Step 13: Update `GetStartingWeight` and `GetStartingSeconds`.**

Replace `s.repo.sessions.GetLatestStartingWeightBefore` with `s.repos.Sessions.GetLatestStartingWeightBefore`, `s.repo.exercises.Get` with `s.repos.Exercises.Get`, and `s.repo.sessions.GetLatestSuccessfulSecondsBefore` with `s.repos.Sessions.GetLatestSuccessfulSecondsBefore`. Bodies otherwise unchanged.

- [ ] **Step 14: Update `UpdateExercise` (drop the bool return from the closure).**

Replace the body with:

```go
func (s *Service) UpdateExercise(ctx context.Context, ex Exercise) error {
	if err := s.repos.Exercises.Update(ctx, ex.ID, func(oldEx *Exercise) error {
		*oldEx = ex
		return nil
	}); err != nil {
		return fmt.Errorf("update exercise: %w", err)
	}
	return nil
}
```

- [ ] **Step 15: Update `List`, `GetExercise`, `ListMuscleGroups`, `WeeklyMuscleGroupVolume`, `FindCompatibleExercises`.**

Mechanical: every `s.repo.exercises.X` becomes `s.repos.Exercises.X`; every `s.repo.muscleTargets.X` becomes `s.repos.MuscleTargets.X`; every `s.repo.featureFlags.X` becomes `s.repos.FeatureFlags.X`. Bodies otherwise unchanged.

- [ ] **Step 16: Rewrite `SwapExercise` and `replaceExerciseInSession`.**

Adopt `Session.SwapExerciseInSlot`. Replace the two methods with:

```go
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
```

Delete `replaceExerciseInSession` entirely.

`findHistoricalSets` walks `session.ExerciseSets` looking for `exerciseSet.ExerciseID`. Update it to look at `exerciseSet.Exercise.ID`:

```go
for _, exerciseSet := range session.ExerciseSets {
    if exerciseSet.Exercise.ID != exerciseID || len(exerciseSet.Sets) == 0 {
        continue
    }
    return s.copySetsWithoutCompletion(exerciseSet.Sets), nil
}
```

- [ ] **Step 17: Rewrite `AddExercise` (adopt `Session.AddExercise`; re-fetch for slot ID).**

Replace the body with:

```go
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
		return addErr
	})
	if err != nil {
		return 0, fmt.Errorf("update session with new exercise: %w", err)
	}

	// Re-fetch to learn the slot ID assigned by the repository on insert. The
	// slot is unique by Exercise.ID within a session (Session.AddExercise
	// rejected duplicates), so locating it is unambiguous.
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

- [ ] **Step 18: Update feature-flag helpers.**

Mechanical: `s.repo.featureFlags.X` → `s.repos.FeatureFlags.X` in `GetFeatureFlag`, `IsMaintenanceModeEnabled`, `ListFeatureFlags`, `SetFeatureFlag`. Bodies otherwise unchanged.

- [ ] **Step 19: Remove the `formatDate` helper from service.go.**

The local `formatDate` (relocated to `internal/repository/shared.go`) is no longer needed in service. Replace every `formatDate(...)` call inside service.go with `(...).Format(time.DateOnly)` (the canonical inline form already used in the rewrites above).

- [ ] **Step 20: Update cmd/web/handler-exerciseset.go for the unified `RecordSet`.**

Find line ~225:

```go
err = app.workoutService.RecordSetCompletion(
    r.Context(), date, workoutExerciseID, setIndex, signal, weightKg, completedReps,
)
```

Replace with:

```go
err = app.workoutService.RecordSet(
    r.Context(), date, workoutExerciseID, setIndex, signal, &weightKg, completedReps,
)
```

Find line ~285:

```go
if err = app.workoutService.RecordTimedSetCompletion(
    r.Context(), date, workoutExerciseID, setIndex, signal, completedSeconds,
); err != nil {
```

Replace with:

```go
if err = app.workoutService.RecordSet(
    r.Context(), date, workoutExerciseID, setIndex, signal, nil, completedSeconds,
); err != nil {
```

Confirm there is no other reference to `RecordSetCompletion`/`RecordTimedSetCompletion` in `cmd/web/`:

Run: `grep -rn "RecordSetCompletion\|RecordTimedSetCompletion" cmd/web/`
Expected: no matches.

- [ ] **Step 21: Update internal/workout/service_test.go for the unified `RecordSet`.**

Two call sites — line ~1625 and ~1732. Both look like:

```go
if err = svc.RecordSetCompletion(ctx, date, weID, 0, workout.SignalOnTarget, 102.5, 5); err != nil {
    t.Fatalf("RecordSetCompletion: %v", err)
}
```

Replace each with:

```go
weight := 102.5
if err = svc.RecordSet(ctx, date, weID, 0, workout.SignalOnTarget, &weight, 5); err != nil {
    t.Fatalf("RecordSet: %v", err)
}
```

(Use the actual literal weight from each call site; the second one passes `0` — keep it as `weight := 0.0`.)

Confirm no other references:

Run: `grep -rn "RecordSetCompletion\|RecordTimedSetCompletion" internal/workout/`
Expected: no matches (both old method definitions are deleted in step 9).

- [ ] **Step 22: Build and run the full test suite.**

Run: `go build ./...`
Expected: no errors.

Run: `go test ./...`
Expected: PASS across the tree.

- [ ] **Step 23: Lint.**

Run: `make lint-fix`
Expected: no remaining issues.

- [ ] **Step 24: Commit.**

```bash
git add internal/workout/service.go cmd/web/handler-exerciseset.go internal/workout/service_test.go
git commit -m "Cut over workout.Service to internal/repository/; adopt aggregate methods; collapse RecordSet"
```

---

### Task 8: Delete `internal/workout/repository*.go`

The new repository package is wired and the workout service no longer references the local repository wrapper. Time to remove the dead code.

**Files (all deleted):**
- `internal/workout/repository.go`
- `internal/workout/repository-sessions.go`
- `internal/workout/repository-exercises.go`
- `internal/workout/repository-preferences.go`
- `internal/workout/repository-featureflags.go`
- `internal/workout/repository-muscle-targets.go`

- [ ] **Step 1: Verify nothing in the tree still references the workout repo internals.**

Run: `grep -rn "sessionAggregate\|exerciseSetAggregate\|datedExerciseSetAggregate\|repositoryFactory\|newRepositoryFactory\|preferencesRepository\|exerciseRepository\|sessionRepository\|featureFlagRepository\|muscleGroupTargetRepository" .`
Expected: no matches outside `docs/`.

If any matches surface, fix them in service.go before deleting.

- [ ] **Step 2: Verify nothing references the workout-package `ErrNotFound` alias (it lives in `repository.go`).**

Run: `grep -rn "workout.ErrNotFound" .`
Expected: matches only in `cmd/web/` (3 sites: `handler-exercise-info.go`, `handler-workout.go` x2) and `internal/workout/service_test.go`. These continue to work because `internal/workout/models.go` re-exports `ErrNotFound` from domain via... it doesn't yet. We need to add it.

Add to `internal/workout/models.go` (anywhere in the alias block):

```go
// ErrNotFound is re-exported from internal/domain for the duration of the
// rearchitecture. Phase 4 will retire this alias along with the rest of the
// workout package.
var ErrNotFound = domain.ErrNotFound
```

This replaces the soon-to-be-deleted alias in `repository.go`. Sanity-check: the `var ErrNotFound = domain.ErrNotFound` line currently in `internal/workout/repository.go:189` is being deleted in this task, so adding the same line to `models.go` keeps callers (`workout.ErrNotFound`) compiling.

- [ ] **Step 3: Delete the six files.**

```bash
rm internal/workout/repository.go \
   internal/workout/repository-sessions.go \
   internal/workout/repository-exercises.go \
   internal/workout/repository-preferences.go \
   internal/workout/repository-featureflags.go \
   internal/workout/repository-muscle-targets.go
```

- [ ] **Step 4: Build and test.**

Run: `go build ./...`
Expected: no errors.

Run: `go test ./...`
Expected: PASS across the tree.

- [ ] **Step 5: Commit.**

```bash
git add -A internal/workout/
git commit -m "Delete internal/workout/repository*.go (replaced by internal/repository/)"
```

---

### Task 9: Add `internal/repository/CLAUDE.md`

**Files:**
- Create: `internal/repository/CLAUDE.md`

- [ ] **Step 1: Create the file.**

Content:

```markdown
# Repository — SQLite Persistence

The `internal/repository` package is the canonical persistence layer for
the workout bounded context. It depends on `internal/domain`,
`internal/sqlite`, and `internal/contexthelpers` only — no HTTP, no
template logic, no business orchestration.

## What lives here

- **SQLite implementations** of the five repository contracts:
  `SessionRepository`, `ExerciseRepository`, `PreferencesRepository`,
  `FeatureFlagRepository`, `MuscleGroupTargetRepository`. Implementations
  are unexported (`sqliteSessionRepository`, etc.).
- **The `Repositories` composite struct** plus the single public
  constructor `New(db *sqlite.Database, logger *slog.Logger) *Repositories`
  that wires everything together (notably injecting `ExerciseRepository`
  into `SessionRepository` for hydration).
- **Shared helpers** in `shared.go`: `parseTimestamp`, `formatTimestamp`,
  `formatDate`, the `baseRepository` mixin, and the timestamp/date format
  constants.

## What does NOT live here

- Business rules — those live as aggregate methods on `domain.Session`
  (or as pure functions in the domain package).
- HTTP handlers, template shaping, response serialisation — `cmd/web`.
- Service orchestration, AI exercise generation, GDPR export, anything
  that combines multiple aggregates or external systems —
  `internal/workout` (Phase 3 will move this to `internal/service/`).
- Tests of business behaviour — those belong in `internal/domain` (pure
  unit) or `internal/workout` (orchestration/e2e). Repository tests
  cover repository-shape contracts: round-trip persistence, error
  translation, slot-ID stability across `Update`.

## Update closure contract

`SessionRepository.Update` and `ExerciseRepository.Update` accept a
closure `func(*domain.X) error` that runs inside an open transaction:

- The repo loads the aggregate (hydrating exercise data for sessions),
  runs the closure, persists the result on `nil`, rolls back on error.
- The closure expresses business invariants by calling aggregate methods
  on `domain.Session` (e.g. `sess.Start(now)`); domain sentinels
  (`ErrAlreadyStarted`, `ErrSlotNotFound`, etc.) propagate to the caller
  unchanged so service-layer code can `errors.Is` against them.
- Returning a non-domain error rolls back too — the repo doesn't try to
  classify; service code decides whether the error is fatal.

## Diff strategy: delete-and-reinsert

`SessionRepository.Update` persists by deleting the
`workout_sessions` row inside the tx (CASCADE clears
`workout_exercise` and `exercise_sets`) and re-inserting the entire
session. Pre-existing `ExerciseSet.ID` values are passed back into
`INSERT ... RETURNING id` so URL-stable slot IDs survive the cycle. New
slots (ID == 0) get auto-assigned IDs. For PetrApp's data sizes (a
handful of exercises × a handful of sets per session) the cost is
negligible and the simplicity is worth the trade.

## Hydration policy

`SessionRepository.Get` and `List` always populate `ExerciseSet.Exercise`
by calling the injected `ExerciseRepository.Get` per slot. Callers
receive a "fully hydrated" `domain.Session` and never need to enrich it
themselves. The N+1 query pattern is preserved from the pre-rearchitecture
service code; for current data sizes the convenience outweighs the cost.

## ErrNotFound translation at the boundary

Every read method translates `sql.ErrNoRows` to `domain.ErrNotFound`
explicitly. Callers `errors.Is(err, domain.ErrNotFound)` to detect
missing rows; they never see `sql.ErrNoRows` directly. This is the
symbolic move that ends "persistence leaks into the domain": the domain
sentinel has no SQL ancestry.

## Adding a new repository

1. Declare the interface in `internal/repository/repository.go`.
2. Add the SQLite implementation in a new file (e.g. `widgets.go`) with
   an unexported struct `sqliteWidgetRepository` and an unexported
   constructor `newSQLiteWidgetRepository`.
3. Wire the new repo into `Repositories` and `New`.
4. Add round-trip tests in `widgets_test.go` (external `package
   repository_test`) using the `setupTestRepos` helper.
```

- [ ] **Step 2: Run lint to confirm markdown is clean (if a markdown linter is configured).**

Run: `make lint-fix`
Expected: no remaining issues.

- [ ] **Step 3: Commit.**

```bash
git add internal/repository/CLAUDE.md
git commit -m "Add internal/repository/CLAUDE.md"
```

---

### Task 10: Update `internal/workout/CLAUDE.md`

**Files:**
- Modify: `internal/workout/CLAUDE.md`

- [ ] **Step 1: Replace the file content.**

```markdown
# Workout Package — Migration Status

> **Migration in progress (Phases 1 + 2 of 4 complete as of 2026-05-10).**
> - Pure logic lives in `internal/domain/` (Phase 1).
> - Persistence lives in `internal/repository/` (Phase 2).
> - This package now contains only orchestration code (`service.go`, the
>   AI exercise generator, and backward-compat type aliases). Phase 3
>   moves these to `internal/service/`. Phase 4 deletes the package.
>
> See `docs/superpowers/specs/2026-05-10-workout-service-rearchitecture-design.md`.

## What still lives here

- **`models.go`** — type aliases (`type Session = domain.Session`, etc.)
  so `cmd/web/` handlers continue to import `workout.*` symbols without
  edit. Removed in Phase 4.
- **`service.go`** — `Service` struct, `NewService`, orchestration that
  combines repository calls with domain logic, AI exercise creation,
  weekly plan generation, GDPR export. Moves to `internal/service/` in
  Phase 3.
- **`generator-exercise.go`** — OpenAI-backed exercise content
  generator. Moves to `internal/service/` in Phase 3 alongside its
  unexported JSON-schema type.
- **`service_test.go` / `service_internal_test.go` /
  `generator-exercise_internal_test.go`** — orchestration and helper
  unit tests; relocate alongside their subjects in Phase 3.

## Where to add new code

- **Pure rules / value objects / aggregate methods:** `internal/domain/`.
- **New SQL queries / repository methods:** `internal/repository/`.
- **Cross-aggregate orchestration / external integrations / GDPR:**
  here, in `service.go` (Phase 3 will move it intact).

## Sentinel errors

`workout.ErrNotFound` re-exports `domain.ErrNotFound`. Handlers and
existing tests use it; no behaviour change. New sentinels go in
`internal/domain/errors.go`.

## Display derivations belong on domain types

Unchanged from Phase 1: any value that depends on multiple domain
attributes, or that encodes a business rule, lives as a method on the
domain type. See `internal/domain/CLAUDE.md`.
```

- [ ] **Step 2: Commit.**

```bash
git add internal/workout/CLAUDE.md
git commit -m "Update internal/workout/CLAUDE.md to reflect Phase 2 done"
```

---

### Task 11: Final verification — full `make ci`

**Files:** none (verification only).

- [ ] **Step 1: Run the full CI pipeline.**

Run: `make ci`
Expected: PASS (all of init, build, lint, test, sec).

- [ ] **Step 2: Run a small grep audit confirming the migration is complete.**

Run each, expecting **no matches**:

```bash
grep -rn "sessionAggregate\|exerciseSetAggregate\|datedExerciseSetAggregate" --include="*.go" .
grep -rn "RecordSetCompletion\|RecordTimedSetCompletion" --include="*.go" .
grep -rn "enrichSessionAggregate" --include="*.go" .
grep -rn "newRepositoryFactory\|repositoryFactory" --include="*.go" .
grep -rn "internal/workout/repository" --include="*.go" .
```

- [ ] **Step 3: Confirm all the spec's Phase 2 acceptance criteria.**

- `internal/repository/` exists with the 5 repos + `Repositories` struct + `New` constructor: `ls internal/repository/`
- The Update closure operates on `*domain.Session`: `grep -n "func.*Sessions.*Update" internal/repository/repository.go`
- `domain.ErrNotFound` is its own sentinel and translated at every read boundary: `grep -n "domain.ErrNotFound" internal/repository/*.go`
- Repository interfaces are exported, structs unexported: `grep -n "^type sqlite" internal/repository/*.go` (should match impls), `grep -n "^type [A-Z][A-Za-z]*Repository " internal/repository/repository.go` (should match interfaces)

- [ ] **Step 4: Final summary commit (no-op acknowledging Phase 2 done).**

If everything is green and no further changes are needed, no extra commit. Otherwise, fix and re-run `make ci`.

---

## Phase 2 done. Next steps

Phase 3 will:
- Move `internal/workout/service.go` → `internal/service/` (split into `sessions.go`, `sets.go`, `exercises.go`, `progression.go`, `reporting.go`, `feature_flags.go`, `exercise_generation.go`, `export.go`).
- Move `internal/workout/generator-exercise.go` and the unexported `exerciseJSONSchema` type to `internal/service/exercise_generation.go`.
- Make `internal/workout.NewService` a thin alias for `service.NewService`.
- Move `service_test.go` to `internal/service/`.

Phase 4 will:
- Delete `internal/workout/` entirely (including `models.go`'s type aliases).
- Sweep `cmd/web/` imports: `workout.X` → `domain.X` or `service.X` as appropriate.
- Update root `CLAUDE.md` workflow guidance.
