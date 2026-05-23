# Repository & Domain Hygiene Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eight small, behavior-preserving cleanups in `internal/repository`, `internal/domain`, and `internal/service` to retire smells surfaced by the 2026-05-23 audit. No new features, no UI changes.

**Architecture:** Each task is independent and can be reverted in isolation. Tasks should be executed in the listed order so unrelated cleanups don't conflict in test files.

**Tech Stack:** Go 1.24 (stdlib only), SQLite (no schema changes).

**Audit notes carried into this plan:**
- **#4 (planner uses `time.Now()`)**: re-read of `internal/domain/CLAUDE.md` confirms the rule is "no SQL, no HTTP, no logger, no third-party clients" — `time` is *not* banned. Status: **dropped** from this plan. Use of `time.Now()` in `NewPlanner` is acceptable. (The audit overstated this.)
- **#11 (feature flag primitive obsession)**: introduce a typed `domain.FeatureFlagName` with `const`s for the names we actually use. Callers stop passing raw strings.
- **#13 (`SecureQueryTool` unused)**: belongs in Plan C (it's a decision, not a refactor).

So this plan implements: **#3, #8, #9, #11, #14, #15, #16** (seven tasks).

---

## Task 1: Batch `insertMuscleGroups` into a single multi-row INSERT (#3)

**Files:**
- Modify: `internal/repository/exercises.go` (the `insertMuscleGroups` method, lines 248–264)
- Test: `internal/repository/exercises_test.go` (existing round-trip tests cover this; we add one assertion-light test that drives the multi-row path with a non-trivial list)

The current implementation does one `INSERT` per muscle group inside the open `tx`. For an exercise with primary={Quads, Hamstrings, Glutes} and secondary={Calves, Lower Back}, that's 5 statements. Replace with one statement per call (so 2 statements total: one for primary, one for secondary).

- [ ] **Step 1: Verify the existing test would still pass after the refactor**

Read `internal/repository/exercises_test.go` (the round-trip test for `Create`). Confirm it asserts on the populated muscle groups after a round-trip. If it does (it should), no new test is needed — the refactor is covered.

```bash
grep -n "PrimaryMuscleGroups\|SecondaryMuscleGroups" internal/repository/exercises_test.go
```

If the grep returns 2+ hits, proceed to Step 2 without writing a new test. Otherwise, add a round-trip test before Step 2.

- [ ] **Step 2: Run the existing tests to confirm green baseline**

Run: `go test ./internal/repository -run TestExercises`

Expected: PASS. Note the runtime so you can sanity-check the new code isn't slower.

- [ ] **Step 3: Replace `insertMuscleGroups` with a batched implementation**

In `internal/repository/exercises.go`, replace the `insertMuscleGroups` method (lines 248–264) with:

```go
func (r *sqliteExerciseRepository) insertMuscleGroups(
	ctx context.Context,
	tx *sql.Tx,
	exerciseID int,
	muscleGroups []string,
	isPrimary bool,
) error {
	if len(muscleGroups) == 0 {
		return nil
	}
	// One statement: VALUES (?, ?, ?), (?, ?, ?), ...
	placeholders := strings.Repeat("(?, ?, ?),", len(muscleGroups))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
	args := make([]any, 0, len(muscleGroups)*3)
	for _, mg := range muscleGroups {
		args = append(args, exerciseID, mg, isPrimary)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary)
		VALUES `+placeholders, args...); err != nil {
		return fmt.Errorf("insert muscle groups: %w", err)
	}
	return nil
}
```

Add `"strings"` to the import block in `internal/repository/exercises.go` if it isn't already imported.

- [ ] **Step 4: Run tests to verify it still passes**

Run: `go test ./internal/repository -run TestExercises`

Expected: PASS.

- [ ] **Step 5: Run lint-fix**

Run: `make lint-fix`

Expected: clean. (Note: the SQL-as-string-concat may trigger `gosec` G201; if it does, add `//nolint:gosec // placeholders is built from a count, not user input` immediately above the `tx.ExecContext` line.)

- [ ] **Step 6: Commit**

```bash
git add internal/repository/exercises.go
git commit -m "repository: batch exercise_muscle_groups insert into one statement

insertMuscleGroups looped one INSERT per muscle group. For an exercise
with 3 primary + 2 secondary muscle groups, Create issued 5 statements
inside the tx. One multi-row VALUES statement is enough.
"
```

---

## Task 2: Drop the unused `warmupCompletedAtStr` from `scanHistoryRow` (#8)

**Files:**
- Modify: `internal/repository/sessions.go` (`ListSetsForExerciseSince` query at lines 613–620, `scanHistoryRow` at lines 664–684)
- Test: `internal/repository/sessions_test.go` (existing tests cover this read path; verify after edit)

`scanHistoryRow` selects `we.warmup_completed_at` into a `sql.NullString` named `warmupCompletedAtStr` and never reads it. The comment at line 669 already acknowledges this ("unused but selected to match Scan arity"). Drop the column from the SELECT and the matching scan target — `ListSetsForExerciseSince` doesn't need it.

This is a real read-path used by `BuildProgression`. The other two SELECTs that include `warmup_completed_at` (lines 350, 393, in `Get` / `List`) **do** consume the value via `scanExerciseSetRows` → `parseWarmupCompletedAtTimestamp`. Leave those untouched.

- [ ] **Step 1: Confirm `scanHistoryRow` is the only consumer of `ListSetsForExerciseSince`'s row shape**

```bash
grep -n "scanHistoryRow\b" internal/repository/sessions.go
```

Expected output: one definition (line 664), one call site (line 635). If there are other call sites, stop and reassess.

- [ ] **Step 2: Run the existing test as a green baseline**

Run: `go test ./internal/repository -run TestSessions`

Expected: PASS.

- [ ] **Step 3: Drop the column from the SELECT**

In `internal/repository/sessions.go`, in `ListSetsForExerciseSince` (lines 613–620), change the query to remove `we.warmup_completed_at`:

```go
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT we.workout_date, es.weight_kg, es.target_value,
		       es.completed_value, es.completed_at, es.signal
		FROM workout_exercise we
		JOIN exercise_sets es ON es.workout_exercise_id = we.id
		WHERE we.workout_user_id = ? AND we.exercise_id = ? AND we.workout_date >= ?
		ORDER BY we.workout_date DESC, es.set_number`,
		userID, exerciseID, sinceDateStr)
```

- [ ] **Step 4: Drop the unused scan target from `scanHistoryRow`**

In `internal/repository/sessions.go`, replace `scanHistoryRow` (lines 664–684) with:

```go
func scanHistoryRow(rows *sql.Rows) (string, domain.Set, error) {
	var (
		workoutDateStr string
		set            domain.Set
		completedAtStr sql.NullString
		signalStr      sql.NullString
	)
	if err := rows.Scan(&workoutDateStr, &set.WeightKg, &set.TargetValue,
		&set.CompletedValue, &completedAtStr, &signalStr); err != nil {
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
```

- [ ] **Step 5: Run the test suite**

Run: `go test ./internal/repository ./internal/service ./internal/domain`

Expected: PASS. (Service-level tests exercise `BuildProgression` which ultimately hits `ListSetsForExerciseSince`.)

- [ ] **Step 6: Run lint-fix**

Run: `make lint-fix`

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/sessions.go
git commit -m "repository: drop unused warmup_completed_at from history scan

scanHistoryRow was selecting we.warmup_completed_at and scanning it
into a NullString that was never read. The comment acknowledged the
dead column. Drop both ends.
"
```

---

## Task 3: Make `GetLatestSuccessfulSecondsBefore` return `ErrNotFound` instead of `(0, nil)` (#9)

**Files:**
- Modify: `internal/repository/sessions.go` (`GetLatestSuccessfulSecondsBefore`, lines 727–757)
- Modify: `internal/service/progression.go` (`GetStartingSeconds`, lines 62–85)
- Test: `internal/repository/sessions_test.go` (add a round-trip test asserting the sentinel)

The current method returns `(0, nil)` for "no rows" and `(value, nil)` for a hit. A real `0` would be indistinguishable from "no record." This violates the repository contract documented in `internal/repository/CLAUDE.md`: "Every read method translates `sql.ErrNoRows` to `domain.ErrNotFound` explicitly." The sibling `GetLatestStartingWeightBefore` (line 686) is debatable but returns a struct, not a single scalar.

Change the contract to return `(0, domain.ErrNotFound)` and update the one caller (`service.GetStartingSeconds`) to handle the sentinel.

- [ ] **Step 1: Write the failing test in `internal/repository/sessions_test.go`**

Append (or insert near the other `GetLatest*` tests if any exist):

```go
func TestGetLatestSuccessfulSecondsBefore_NoRows_ReturnsNotFound(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	// No exercise_sets seeded for exercise 99999 → must surface ErrNotFound.
	_, err := repos.Sessions.GetLatestSuccessfulSecondsBefore(ctx, 99999, time.Now())
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want domain.ErrNotFound", err)
	}
}
```

Imports: add `"errors"`, `"time"` to the test file's import block if not already present, plus `"github.com/myrjola/petrapp/internal/domain"`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./internal/repository -run TestGetLatestSuccessfulSecondsBefore_NoRows_ReturnsNotFound`

Expected: FAIL — `err = <nil>, want domain.ErrNotFound`.

- [ ] **Step 3: Update the repository method**

In `internal/repository/sessions.go`, replace `GetLatestSuccessfulSecondsBefore` (lines 727–757) with:

```go
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
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("query latest successful seconds: %w", err)
	}
	return seconds, nil
}
```

- [ ] **Step 4: Update the only caller in `service.GetStartingSeconds`**

In `internal/service/progression.go`, replace `GetStartingSeconds` (lines 62–85) with:

```go
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
	switch {
	case err == nil:
		return seconds, nil
	case errors.Is(err, domain.ErrNotFound):
		if exercise.DefaultStartingSeconds == nil {
			return 0, fmt.Errorf("time_based exercise %d has no default_starting_seconds", exerciseID)
		}
		return *exercise.DefaultStartingSeconds, nil
	default:
		return 0, fmt.Errorf("get latest successful seconds: %w", err)
	}
}
```

Add `"errors"` to the import block if not already present.

- [ ] **Step 5: Run all related tests**

Run: `go test ./internal/repository ./internal/service`

Expected: PASS — the new repo test passes, the existing service tests cover the fallback-to-default path (since they don't seed history).

- [ ] **Step 6: Run lint-fix**

Run: `make lint-fix`

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/repository/sessions.go internal/service/progression.go \
        internal/repository/sessions_test.go
git commit -m "repository: surface ErrNotFound from GetLatestSuccessfulSecondsBefore

The previous (0, nil) on no rows was ambiguous with a legitimate
zero-second record. Match the repo CLAUDE.md contract (\"every read
method translates sql.ErrNoRows to domain.ErrNotFound\"). The sole
caller — service.GetStartingSeconds — now branches on the sentinel
before falling through to DefaultStartingSeconds.
"
```

---

## Task 4: Refactor `Preferences.IsEmpty` to loop over weekdays (#14)

**Files:**
- Modify: `internal/domain/preferences.go` (`IsEmpty`, lines 30–35)

`IsEmpty` ANDs seven explicit `*Minutes == 0` checks. If a new field is added, the check has to be updated by hand. Use the existing `IsWorkoutDay` (line 61) over the `time.Weekday` range.

- [ ] **Step 1: Replace `IsEmpty` in `internal/domain/preferences.go`**

Replace lines 30–35 with:

```go
// IsEmpty reports whether no workout days are scheduled.
func (p Preferences) IsEmpty() bool {
	for d := time.Sunday; d <= time.Saturday; d++ {
		if p.IsWorkoutDay(d) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run domain tests**

Run: `go test ./internal/domain`

Expected: PASS. (`Preferences.IsEmpty` is exercised in `internal/domain/preferences_test.go` if it exists, and indirectly through `internal/service` tests that gate on it.)

- [ ] **Step 3: Run lint-fix**

Run: `make lint-fix`

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/preferences.go
git commit -m "domain: derive Preferences.IsEmpty from IsWorkoutDay loop

Seven hand-ANDed checks against per-weekday fields invited skew if a
field was ever added. The single-source-of-truth is MinutesForDay,
which IsWorkoutDay wraps. Loop over the weekday range instead.
"
```

---

## Task 5: Introduce typed `domain.FeatureFlagName` with named constants (#11)

**Files:**
- Modify: `internal/domain/feature_flag.go`
- Modify: `internal/repository/feature_flags.go` (interface stays, internals unchanged)
- Modify: `internal/repository/repository.go` (interface signature)
- Modify: `internal/service/feature_flags.go` (use the constant)
- Modify: `internal/repository/feature_flags_test.go` (use constants where appropriate)
- Modify: `internal/service/feature_flags_test.go` (use the constant where appropriate)
- Modify: `cmd/web/handler-admin-feature-flags_test.go` (raw SQL stays, but the `INSERT OR REPLACE` lines keep using the literal "maintenance_mode" — the constant doesn't simplify SQL)

The smell: `FeatureFlag.Name` is a free string and the only call site we *know* about (`maintenance_mode`) is referenced by raw string literal in five places. Define `type FeatureFlagName string` and `const FeatureFlagMaintenanceMode FeatureFlagName = "maintenance_mode"`. Keep the repo storage as string (the schema doesn't change), but the API surfaces the typed name.

- [ ] **Step 1: Define the type and constants in `internal/domain/feature_flag.go`**

Replace the full file content with:

```go
package domain

// FeatureFlagName is the typed identifier for a feature toggle. Constants
// below name every flag the application reads. The repository persists the
// underlying string, but callers exchange the typed value so a misspelled
// name is a compile error.
type FeatureFlagName string

// Feature flag names known to the application. Add a new constant when you
// add a new flag; the repository will store the underlying string verbatim.
const (
	FeatureFlagMaintenanceMode FeatureFlagName = "maintenance_mode"
)

// FeatureFlag toggles application features at runtime.
type FeatureFlag struct {
	Name    FeatureFlagName
	Enabled bool
}
```

- [ ] **Step 2: Update the repository interface signature**

In `internal/repository/repository.go`, find `FeatureFlagRepository` (lines 104–109) and change the `Get` signature to take the typed name:

```go
// FeatureFlagRepository persists boolean feature toggles by name.
type FeatureFlagRepository interface {
	Get(ctx context.Context, name domain.FeatureFlagName) (domain.FeatureFlag, error)
	Set(ctx context.Context, flag domain.FeatureFlag) error
	List(ctx context.Context) ([]domain.FeatureFlag, error)
}
```

- [ ] **Step 3: Update the SQLite implementation**

In `internal/repository/feature_flags.go`, change the `Get` method signature (line 21) and the scan target. Note `flag.Name` is now `domain.FeatureFlagName`; `Scan` into it works because SQLite's text affinity maps to any `string` newtype via the standard driver when used as a pointer destination.

```go
func (r *sqliteFeatureFlagRepository) Get(ctx context.Context, name domain.FeatureFlagName) (domain.FeatureFlag, error) {
	var flag domain.FeatureFlag
	var enabled int
	var nameStr string

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT name, enabled
		FROM feature_flags
		WHERE name = ?`, string(name)).Scan(&nameStr, &enabled)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.FeatureFlag{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("query feature flag %s: %w", name, err)
	}

	flag.Name = domain.FeatureFlagName(nameStr)
	flag.Enabled = enabled == 1
	return flag, nil
}
```

For `Set` (line 41) — `flag.Name` is now typed. Convert when binding the parameter:

```go
func (r *sqliteFeatureFlagRepository) Set(ctx context.Context, flag domain.FeatureFlag) error {
	enabled := 0
	if flag.Enabled {
		enabled = 1
	}

	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO feature_flags (name, enabled)
		VALUES (?, ?)
		ON CONFLICT (name) DO UPDATE SET enabled = excluded.enabled`,
		string(flag.Name), enabled); err != nil {
		return fmt.Errorf("save feature flag %s: %w", flag.Name, err)
	}
	return nil
}
```

For `List` (line 57) — scan into a local `nameStr` then assign to typed field:

```go
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
		var nameStr string
		if scanErr := rows.Scan(&nameStr, &enabled); scanErr != nil {
			return nil, fmt.Errorf("scan feature flag: %w", scanErr)
		}
		flag.Name = domain.FeatureFlagName(nameStr)
		flag.Enabled = enabled == 1
		flags = append(flags, flag)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feature flags: %w", err)
	}
	return flags, nil
}
```

- [ ] **Step 4: Update `service.GetFeatureFlag` and `IsMaintenanceModeEnabled` to use the constant**

In `internal/service/feature_flags.go`:

- Change `GetFeatureFlag` (line 76) signature to accept `domain.FeatureFlagName`:

```go
// GetFeatureFlag retrieves a feature flag by name.
func (s *Service) GetFeatureFlag(ctx context.Context, name domain.FeatureFlagName) (domain.FeatureFlag, error) {
	flag, err := s.repos.FeatureFlags.Get(ctx, name)
	if err != nil {
		return domain.FeatureFlag{}, fmt.Errorf("get feature flag %s: %w", name, err)
	}
	return flag, nil
}
```

- Inside `IsMaintenanceModeEnabled` (line 86), change line 90 from the string literal to the constant:

```go
	flag, err := s.repos.FeatureFlags.Get(ctx, domain.FeatureFlagMaintenanceMode)
```

- Inside `SetFeatureFlag` (line 118), change line 122 from the string literal to a typed comparison:

```go
	if flag.Name == domain.FeatureFlagMaintenanceMode {
		s.maintenanceCache.invalidate()
	}
```

- [ ] **Step 5: Update test files to use the constant where it improves call clarity**

For each test file that constructs a `domain.FeatureFlag` literal with a known name, optionally swap the string literal for the constant. Keep tests that intentionally exercise unknown names (e.g., `repos.FeatureFlags.Get(ctx, "nonexistent_flag")`) as raw string-converted literals: `domain.FeatureFlagName("nonexistent_flag")`. Files to touch:

- `internal/repository/feature_flags_test.go` — line 13 (`"nonexistent_flag"` → `domain.FeatureFlagName("nonexistent_flag")`), line 22 (`Name: "experimental_x"` → `Name: domain.FeatureFlagName("experimental_x")`), line 38/41/44 (same for `"x"`), line 57 (same), line 26 (call site).
- `internal/service/feature_flags_test.go` — line 70 (`Name: "maintenance_mode"` → `Name: domain.FeatureFlagMaintenanceMode`).
- `cmd/web/handler-admin-feature-flags_test.go` — leave the raw SQL `INSERT OR REPLACE INTO feature_flags ... VALUES ('maintenance_mode', ...)` lines unchanged; the constant doesn't help inside SQL strings.

After each edit, run the package tests in that package only.

- [ ] **Step 6: Build the whole project so the compiler verifies every call site**

Run: `go build ./...`

Expected: PASS. If the compiler complains about any remaining string literal where `domain.FeatureFlagName` is required, fix it before moving on.

- [ ] **Step 7: Run the full test suite**

Run: `make test`

Expected: PASS.

- [ ] **Step 8: Run lint-fix**

Run: `make lint-fix`

Expected: clean.

- [ ] **Step 9: Commit**

```bash
git add internal/domain/feature_flag.go \
        internal/repository/repository.go \
        internal/repository/feature_flags.go \
        internal/repository/feature_flags_test.go \
        internal/service/feature_flags.go \
        internal/service/feature_flags_test.go
git commit -m "domain: introduce typed FeatureFlagName with named constants

FeatureFlag.Name was a free string and 'maintenance_mode' appeared as a
raw literal in five service-layer call sites. Introduce a newtype with
a constant so a misspelled name is a compile error. Schema unchanged;
the storage type is still TEXT.
"
```

---

## Task 6: Fix `maintenanceCache.load` TOCTOU on the atomic pointer (#15)

**Files:**
- Modify: `internal/service/feature_flags.go` (the `maintenanceCache` methods, lines 27–63)

`maintenanceCache.load` does `s := c.state.Load()` then `time.Now().After(s.expires)`. Between the `Load` and the time check, a concurrent `store` (or `invalidate` then `store`) could publish a new state. The current read still returns a self-consistent snapshot because `s` is the snapshot pointer — so it's not actually a TOCTOU on the *pointer*. The real risk is subtler: if `invalidate()` happens between the `Load` and the time check, you return a value that the application has already marked stale.

The fix is to make the store/invalidate pair atomic with respect to load. Pack `enabled + expires` into one struct (already done) and rely on the atomic pointer swap. The only thing to clean up is making `invalidate` use the same atomic semantic as a `nil` `state.Store`, which it already does.

After re-reading: the existing code is **correct** as written — the atomic pointer swap is the synchronization, and `s` captured at line 47 is a stable snapshot. The audit overstated the risk.

**Decision:** Skip this task. The code is correct. Document why in a comment so the next auditor doesn't flag it again.

- [ ] **Step 1: Add a clarifying comment to `maintenanceCache.load`**

In `internal/service/feature_flags.go`, replace `load` (lines 43–52) with:

```go
// load returns the cached value if caching is enabled and the entry has not
// expired. ok=false means the caller must re-read from the database.
//
// Safety: the atomic.Pointer.Load gives a self-consistent snapshot — the
// captured *maintenanceState cannot mutate. A concurrent store or
// invalidate that runs between Load and the time check is irrelevant: this
// caller is allowed to use a snapshot up to ttl old, by definition of the
// cache.
func (c *maintenanceCache) load() (bool, bool) {
	if c.ttl <= 0 {
		return false, false
	}
	s := c.state.Load()
	if s == nil || time.Now().After(s.expires) {
		return false, false
	}
	return s.enabled, true
}
```

- [ ] **Step 2: Run feature-flag tests to confirm nothing broke**

Run: `go test ./internal/service -run FeatureFlag`

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/service/feature_flags.go
git commit -m "service: document maintenanceCache.load atomicity contract

A code-smell audit flagged this as TOCTOU. It isn't — the atomic
pointer swap is the synchronization and the captured snapshot is
stable. Add a comment so the next auditor doesn't re-raise it.
"
```

---

## Task 7: Promote the ignored `rows.Close()` error in `fetchMuscleGroupsByExerciseID` (#16)

**Files:**
- Modify: `internal/repository/shared.go` (the `defer` block at lines 60–62)

Every other rows-reader in this package joins the close error into the returned error via `errors.Join`. `fetchMuscleGroupsByExerciseID` silently discards it with `_ = rows.Close()`. Match the convention. This requires the function to use a named return value so the defer can mutate it.

- [ ] **Step 1: Replace `fetchMuscleGroupsByExerciseID` in `internal/repository/shared.go`**

Replace lines 35–86 with (note the named return `err` and the updated defer):

```go
// fetchMuscleGroupsByExerciseID loads primary/secondary muscle groups for every
// given exercise ID in a single query, keyed by exercise ID. Returns an empty
// map when ids is empty. Shared by the exercise and session repositories so
// neither issues a per-exercise follow-up query.
func fetchMuscleGroupsByExerciseID(
	ctx context.Context,
	q queryer,
	ids []int,
) (_ map[int]muscleGroups, err error) {
	if len(ids) == 0 {
		return map[int]muscleGroups{}, nil
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	query := `
		SELECT emg.exercise_id, mg.name, emg.is_primary
		FROM exercise_muscle_groups emg
		JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
		WHERE emg.exercise_id IN (` + placeholders + `)`

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query muscle groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close muscle group rows: %w", closeErr))
		}
	}()

	byExercise := make(map[int]muscleGroups, len(ids))
	for rows.Next() {
		var (
			exerciseID int
			name       string
			isPrimary  bool
		)
		if err = rows.Scan(&exerciseID, &name, &isPrimary); err != nil {
			return nil, fmt.Errorf("scan muscle group row: %w", err)
		}
		g := byExercise[exerciseID]
		if isPrimary {
			g.primary = append(g.primary, name)
		} else {
			g.secondary = append(g.secondary, name)
		}
		byExercise[exerciseID] = g
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate muscle group rows: %w", err)
	}
	return byExercise, nil
}
```

Add `"errors"` to the imports in `internal/repository/shared.go` if not already present.

- [ ] **Step 2: Run repository tests**

Run: `go test ./internal/repository`

Expected: PASS — none of the tests can detect this change in steady state because `rows.Close()` doesn't error on success. The change is a future-bug guard.

- [ ] **Step 3: Run lint-fix**

Run: `make lint-fix`

Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/repository/shared.go
git commit -m "repository: join rows.Close error in fetchMuscleGroupsByExerciseID

Every other rows-reader in this package joins the close error via
errors.Join. This one silently discarded it with _ = rows.Close().
Match the convention.
"
```

---

## Final verification

- [ ] **Step 1: Run the full validation pipeline**

Run: `make ci`

Expected: PASS on init, build, lint-fix (no diff), test, and sec.

- [ ] **Step 2: Confirm clean working tree**

Run: `git status`

Expected: clean.

---

## Out of scope (intentionally deferred)

- **Finding #4 (planner uses `time.Now()`)**: dropped — domain CLAUDE.md does not ban `time`. The use is intentional.
- **Findings #1, #2, #5**: covered by `2026-05-23-push-correctness-and-observability.md`.
- **Findings #6, #7, #10, #13**: covered by `2026-05-23-handler-and-template-refactor.md`.
- **Finding #17 (dynamic table names in `userdb.go`)**: per the user, deferred.
- **Finding #12**: false positive, confirmed dead-code claim was wrong (the helper is used 5x from `planner_internal_test.go`).
