# Weekly Workout Planner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace per-day workout generation with a cohesive weekly planner that assigns exercises based on muscle group frequency and volume targets.

**Architecture:** A new `internal/weekplanner` pure-logic package (no DB access) contains `WeeklyPlanner` with a three-phase `Plan()` method: category determination (existing adjacency rule), muscle group slot allocation (most-constrained-first greedy), and exercise selection (score-based category filtering). `workout.Service.ResolveWeeklySchedule()` calls the planner when no sessions exist for the current week and persists all results in a single DB transaction. `generator.go` is deleted.

**Tech Stack:** Go 1.26, SQLite via existing `internal/sqlite` package, `math/rand/v2` for deterministic-seed tiebreaking, `slices` standard library for sorting.

---

## File Map

**New:**
- `internal/weekplanner/weekplanner.go` — all types, `WeeklyPlanner`, `Plan()`, and all three phase methods
- `internal/weekplanner/weekplanner_test.go` — `package weekplanner` internal tests

**Modified:**
- `internal/sqlite/schema.sql` — add `muscle_group_weekly_targets` table
- `internal/sqlite/fixtures.sql` — seed 9 tracked muscle group targets
- `internal/workout/models.go` — add `MuscleGroupTarget` type
- `internal/workout/repository.go` — add `muscleGroupTargetRepository` interface, `CreateBatch` to `sessionRepository`, add `muscleTargets` field to `repository` struct
- `internal/workout/repository-muscle-targets.go` — new: sqlite implementation of `muscleGroupTargetRepository`
- `internal/workout/repository-sessions.go` — add `CreateBatch` method
- `internal/workout/service.go` — add `generateWeeklyPlan`, update `ResolveWeeklySchedule`, remove `generateWorkout`
- `internal/workout/service_test.go` — add tests for weekly generation behavior
- `cmd/web/handler-workout.go` — add `ErrNotFound` handling to `workoutStartPOST`
- `ui/templates/pages/workout-not-found/workout-not-found.gohtml` — remove "Create Workout" button, update message

**Deleted:**
- `internal/workout/generator.go`
- `internal/workout/generator_internal_test.go`

---

## Task 1: DB Schema — `muscle_group_weekly_targets` table

**Files:**
- Modify: `internal/sqlite/schema.sql`
- Modify: `internal/sqlite/fixtures.sql`

- [ ] **Step 1: Add table to schema.sql**

  Append before the final comment or at the end of the tables section in `internal/sqlite/schema.sql`:

  ```sql
  CREATE TABLE IF NOT EXISTS muscle_group_weekly_targets
  (
      muscle_group_name TEXT PRIMARY KEY REFERENCES muscle_groups (name),
      weekly_sets_target INTEGER NOT NULL CHECK (weekly_sets_target > 0)
  ) STRICT;
  ```

- [ ] **Step 2: Seed fixtures.sql**

  Append to `internal/sqlite/fixtures.sql`:

  ```sql
  INSERT INTO muscle_group_weekly_targets (muscle_group_name, weekly_sets_target)
  VALUES ('Biceps', 8),
         ('Chest', 10),
         ('Glutes', 8),
         ('Hamstrings', 8),
         ('Lats', 10),
         ('Quads', 10),
         ('Shoulders', 10),
         ('Triceps', 8),
         ('Upper Back', 10) ON CONFLICT (muscle_group_name) DO
  UPDATE SET weekly_sets_target = excluded.weekly_sets_target;
  ```

- [ ] **Step 3: Run tests to verify migration works**

  ```bash
  make test
  ```

  Expected: all tests pass. The migration system auto-applies schema changes.

- [ ] **Step 4: Commit**

  ```bash
  git add internal/sqlite/schema.sql internal/sqlite/fixtures.sql
  git commit -m "feat: add muscle_group_weekly_targets table and fixtures"
  ```

---

## Task 2: MuscleGroupTarget type and repository

**Files:**
- Modify: `internal/workout/models.go`
- Modify: `internal/workout/repository.go`
- Create: `internal/workout/repository-muscle-targets.go`

- [ ] **Step 1: Add `MuscleGroupTarget` type to models.go**

  Append to `internal/workout/models.go` before the closing line:

  ```go
  // MuscleGroupTarget stores the minimum weekly set target for a tracked muscle group.
  type MuscleGroupTarget struct {
  	MuscleGroupName string
  	WeeklySetTarget int
  }
  ```

- [ ] **Step 2: Add `muscleGroupTargetRepository` interface and `CreateBatch` to `sessionRepository` in repository.go**

  In `internal/workout/repository.go`, add after the `featureFlagRepository` interface:

  ```go
  // muscleGroupTargetRepository handles muscle group weekly volume targets.
  type muscleGroupTargetRepository interface {
  	List(ctx context.Context) ([]MuscleGroupTarget, error)
  }
  ```

  Add `CreateBatch` to `sessionRepository` interface (after `CountCompleted`):

  ```go
  // CreateBatch creates multiple sessions atomically in a single transaction.
  CreateBatch(ctx context.Context, sessions []sessionAggregate) error
  ```

  Add `muscleTargets` field to the `repository` struct:

  ```go
  type repository struct {
  	prefs        preferencesRepository
  	sessions     sessionRepository
  	exercises    exerciseRepository
  	featureFlags featureFlagRepository
  	muscleTargets muscleGroupTargetRepository
  }
  ```

  Update `newRepository()` in `repositoryFactory`:

  ```go
  func (f *repositoryFactory) newRepository() *repository {
  	exerciseRepo := newSQLiteExerciseRepository(f.db)
  	preferencesRepo := newSQLitePreferenceRepository(f.db)
  	sessionRepo := newSQLiteSessionRepository(f.db)
  	featureFlagRepo := newSQLiteFeatureFlagRepository(f.db)
  	muscleTargetRepo := newSQLiteMuscleGroupTargetRepository(f.db)

  	return &repository{
  		prefs:         preferencesRepo,
  		sessions:      sessionRepo,
  		exercises:     exerciseRepo,
  		featureFlags:  featureFlagRepo,
  		muscleTargets: muscleTargetRepo,
  	}
  }
  ```

- [ ] **Step 3: Create `repository-muscle-targets.go`**

  Create `internal/workout/repository-muscle-targets.go`:

  ```go
  package workout

  import (
  	"context"
  	"errors"
  	"fmt"

  	"github.com/myrjola/petrapp/internal/sqlite"
  )

  type sqliteMuscleGroupTargetRepository struct {
  	baseRepository
  }

  func newSQLiteMuscleGroupTargetRepository(db *sqlite.Database) *sqliteMuscleGroupTargetRepository {
  	return &sqliteMuscleGroupTargetRepository{baseRepository: newBaseRepository(db)}
  }

  func (r *sqliteMuscleGroupTargetRepository) List(ctx context.Context) (_ []MuscleGroupTarget, err error) {
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

  	var targets []MuscleGroupTarget
  	for rows.Next() {
  		var t MuscleGroupTarget
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

- [ ] **Step 4: Build to verify compilation**

  ```bash
  make build
  ```

  Expected: builds successfully. `CreateBatch` not yet implemented on the session repository — the compiler will flag this. That's fine; we'll add it in the next step.

- [ ] **Step 5: Add `CreateBatch` to `repository-sessions.go`**

  Append to `internal/workout/repository-sessions.go`:

  ```go
  // CreateBatch creates multiple sessions atomically in a single transaction.
  func (r *sqliteSessionRepository) CreateBatch(ctx context.Context, sessions []sessionAggregate) (err error) {
  	userID := contexthelpers.AuthenticatedUserID(ctx)

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
  		dateStr := formatDate(sess.Date)
  		if _, execErr := tx.ExecContext(ctx, `
  			INSERT INTO workout_sessions (
  				user_id, workout_date, difficulty_rating, started_at, completed_at, periodization_type
  			) VALUES (?, ?, ?, ?, ?, ?)`,
  			userID, dateStr, sess.DifficultyRating,
  			formatTimestamp(sess.StartedAt), formatTimestamp(sess.CompletedAt),
  			sess.PeriodizationType); execErr != nil {
  			return fmt.Errorf("insert session %s: %w", dateStr, execErr)
  		}
  		if saveErr := r.saveExerciseSets(ctx, tx, sess.Date, sess.ExerciseSets); saveErr != nil {
  			return fmt.Errorf("save exercise sets %s: %w", dateStr, saveErr)
  		}
  	}

  	if err = tx.Commit(); err != nil {
  		return fmt.Errorf("commit batch sessions: %w", err)
  	}
  	return nil
  }
  ```

- [ ] **Step 6: Build and test**

  ```bash
  make build && make test
  ```

  Expected: builds and all tests pass.

- [ ] **Step 7: Commit**

  ```bash
  git add internal/workout/models.go internal/workout/repository.go \
          internal/workout/repository-muscle-targets.go internal/workout/repository-sessions.go
  git commit -m "feat: add MuscleGroupTarget repository and session CreateBatch"
  ```

---

## Task 3: weekplanner package — types and category determination

**Files:**
- Create: `internal/weekplanner/weekplanner.go`
- Create: `internal/weekplanner/weekplanner_test.go`

- [ ] **Step 1: Write failing tests for category determination**

  Create `internal/weekplanner/weekplanner_test.go`:

  ```go
  package weekplanner

  import (
  	"testing"
  	"time"
  )

  // monday2026 is 2026-01-05, a known Monday.
  var monday2026 = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

  func date(base time.Time, offsetDays int) time.Time {
  	return base.AddDate(0, 0, offsetDays)
  }

  func prefs(days ...time.Weekday) Preferences {
  	p := Preferences{}
  	for _, d := range days {
  		switch d {
  		case time.Monday:
  			p.MondayMinutes = 60
  		case time.Tuesday:
  			p.TuesdayMinutes = 60
  		case time.Wednesday:
  			p.WednesdayMinutes = 60
  		case time.Thursday:
  			p.ThursdayMinutes = 60
  		case time.Friday:
  			p.FridayMinutes = 60
  		case time.Saturday:
  			p.SaturdayMinutes = 60
  		case time.Sunday:
  			p.SundayMinutes = 60
  		}
  	}
  	return p
  }

  func TestDetermineCategory(t *testing.T) {
  	tests := []struct {
  		name     string
  		prefs    Preferences
  		date     time.Time
  		expected Category
  	}{
  		{
  			name:     "isolated day is full body",
  			prefs:    prefs(time.Monday, time.Wednesday, time.Friday),
  			date:     monday2026, // Mon: tomorrow=Tue not workout, yesterday=Sun not workout
  			expected: CategoryFullBody,
  		},
  		{
  			name:     "first of consecutive days is lower",
  			prefs:    prefs(time.Monday, time.Tuesday),
  			date:     monday2026, // Mon: tomorrow=Tue is workout
  			expected: CategoryLower,
  		},
  		{
  			name:     "second of consecutive days is upper",
  			prefs:    prefs(time.Monday, time.Tuesday),
  			date:     date(monday2026, 1), // Tue: yesterday=Mon was workout
  			expected: CategoryUpper,
  		},
  		{
  			name:     "week wrap: Sunday before Monday is lower",
  			prefs:    prefs(time.Sunday, time.Monday, time.Tuesday),
  			date:     date(monday2026, 6), // Sun (next week context doesn't matter — prefs wrap)
  			expected: CategoryLower,       // Sun: today=workout, tomorrow=Mon=workout
  		},
  		{
  			name:     "week wrap: Monday after Sunday is upper",
  			prefs:    prefs(time.Sunday, time.Monday),
  			date:     monday2026, // Mon: yesterday=Sun=workout
  			expected: CategoryUpper,
  		},
  	}

  	for _, tt := range tests {
  		t.Run(tt.name, func(t *testing.T) {
  			wp := NewWeeklyPlanner(tt.prefs, nil, nil)
  			got := wp.determineCategory(tt.date)
  			if got != tt.expected {
  				t.Errorf("determineCategory(%s) = %s, want %s", tt.date.Weekday(), got, tt.expected)
  			}
  		})
  	}
  }
  ```

- [ ] **Step 2: Run tests to verify they fail**

  ```bash
  go test ./internal/weekplanner/... -v
  ```

  Expected: compilation error — package does not exist yet.

- [ ] **Step 3: Create `internal/weekplanner/weekplanner.go` with types and `determineCategory`**

  ```go
  package weekplanner

  import (
  	"fmt"
  	"math/rand/v2"
  	"slices"
  	"time"
  )

  // Category is the workout focus for a session.
  type Category string

  const (
  	CategoryFullBody Category = "full_body"
  	CategoryUpper    Category = "upper"
  	CategoryLower    Category = "lower"
  )

  // ExerciseType distinguishes weighted from bodyweight exercises.
  type ExerciseType string

  const (
  	ExerciseTypeWeighted   ExerciseType = "weighted"
  	ExerciseTypeBodyweight ExerciseType = "bodyweight"
  )

  // PeriodizationType controls rep targets for the session.
  type PeriodizationType int

  const (
  	PeriodizationStrength    PeriodizationType = 0 // 5 reps
  	PeriodizationHypertrophy PeriodizationType = 1 // 6-10 reps
  )

  const (
  	setsPerExercise   = 3
  	minRepsStrength   = 5
  	maxRepsStrength   = 5
  	minRepsHypertrophy = 6
  	maxRepsHypertrophy = 10
  )

  // Preferences describes which days are workout days and their duration in minutes.
  // A value of 0 means rest day; 45, 60, or 90 means workout day.
  type Preferences struct {
  	MondayMinutes    int
  	TuesdayMinutes   int
  	WednesdayMinutes int
  	ThursdayMinutes  int
  	FridayMinutes    int
  	SaturdayMinutes  int
  	SundayMinutes    int
  }

  func (p Preferences) minutesForDay(weekday time.Weekday) int {
  	switch weekday {
  	case time.Monday:
  		return p.MondayMinutes
  	case time.Tuesday:
  		return p.TuesdayMinutes
  	case time.Wednesday:
  		return p.WednesdayMinutes
  	case time.Thursday:
  		return p.ThursdayMinutes
  	case time.Friday:
  		return p.FridayMinutes
  	case time.Saturday:
  		return p.SaturdayMinutes
  	case time.Sunday:
  		return p.SundayMinutes
  	default:
  		return 0
  	}
  }

  // IsWorkoutDay returns true if the given weekday has a non-zero duration in preferences.
  func (p Preferences) IsWorkoutDay(weekday time.Weekday) bool {
  	return p.minutesForDay(weekday) > 0
  }

  // ExercisesPerSession returns how many exercises to include based on session duration.
  func (p Preferences) ExercisesPerSession(weekday time.Weekday) int {
  	switch minutes := p.minutesForDay(weekday); {
  	case minutes >= 90:
  		return 4
  	case minutes >= 60:
  		return 3
  	case minutes > 0:
  		return 2
  	default:
  		return 0
  	}
  }

  // Exercise is a dependency-free representation of an exercise for planning.
  // StartingWeightKg is intentionally absent — resolved lazily by exerciseprogression.
  type Exercise struct {
  	ID                    int
  	Category              Category
  	ExerciseType          ExerciseType
  	PrimaryMuscleGroups   []string
  	SecondaryMuscleGroups []string
  }

  // MuscleGroupTarget holds the minimum weekly set target for a tracked muscle group.
  type MuscleGroupTarget struct {
  	Name           string
  	WeeklySetTarget int
  }

  // PlannedSession is the output of Plan() for a single workout day.
  type PlannedSession struct {
  	Date              time.Time
  	Category          Category
  	PeriodizationType PeriodizationType
  	ExerciseSets      []PlannedExerciseSet
  }

  // PlannedExerciseSet groups the planned sets for one exercise.
  type PlannedExerciseSet struct {
  	ExerciseID int
  	Sets       []PlannedSet
  }

  // PlannedSet holds rep targets only; WeightKg is always nil at plan time.
  type PlannedSet struct {
  	MinReps int
  	MaxReps int
  }

  // WeeklyPlanner holds the static inputs needed to plan a full week of workouts.
  type WeeklyPlanner struct {
  	Prefs     Preferences
  	Exercises []Exercise
  	Targets   []MuscleGroupTarget
  	rng       *rand.Rand
  }

  // NewWeeklyPlanner creates a WeeklyPlanner with a randomly seeded RNG.
  func NewWeeklyPlanner(prefs Preferences, exercises []Exercise, targets []MuscleGroupTarget) *WeeklyPlanner {
  	return &WeeklyPlanner{
  		Prefs:     prefs,
  		Exercises: exercises,
  		Targets:   targets,
  		rng:       rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)),
  	}
  }

  // determineCategory returns the workout category for a given date using the adjacency rule.
  // Uses preference-based weekday checks so week boundaries wrap naturally through date arithmetic:
  // Sunday's "tomorrow" is Monday, Monday's "yesterday" is Sunday.
  func (wp *WeeklyPlanner) determineCategory(date time.Time) Category {
  	today := date.Weekday()
  	tomorrow := date.AddDate(0, 0, 1).Weekday()
  	yesterday := date.AddDate(0, 0, -1).Weekday()

  	if wp.Prefs.IsWorkoutDay(today) && wp.Prefs.IsWorkoutDay(tomorrow) {
  		return CategoryLower
  	}
  	if wp.Prefs.IsWorkoutDay(yesterday) {
  		return CategoryUpper
  	}
  	return CategoryFullBody
  }
  ```

- [ ] **Step 4: Run tests to verify they pass**

  ```bash
  go test ./internal/weekplanner/... -v -run TestDetermineCategory
  ```

  Expected: all 5 category tests pass.

- [ ] **Step 5: Commit**

  ```bash
  git add internal/weekplanner/
  git commit -m "feat: add weekplanner package with types and category determination"
  ```

---

## Task 4: weekplanner — periodization calculation

**Files:**
- Modify: `internal/weekplanner/weekplanner.go`
- Modify: `internal/weekplanner/weekplanner_test.go`

- [ ] **Step 1: Write failing test**

  Append to `internal/weekplanner/weekplanner_test.go`:

  ```go
  func TestFirstSessionPeriodizationType(t *testing.T) {
  	// Mon/Wed/Fri at 60 min = 3 exercises each = 9 exercises/week.
  	p := prefs(time.Monday, time.Wednesday, time.Friday)
  	wp := NewWeeklyPlanner(p, nil, nil)

  	// Verify formula: (weeksSinceEpoch * exercisesPerWeek) % 2.
  	// For any two Mondays 2 weeks apart the periodization must differ.
  	monday1 := monday2026                    // week N
  	monday2 := monday2026.AddDate(0, 0, 7)  // week N+1

  	pt1 := wp.firstSessionPeriodizationType(monday1)
  	pt2 := wp.firstSessionPeriodizationType(monday2)

  	if pt1 == pt2 {
  		t.Errorf("consecutive weeks with odd exercisesPerWeek must alternate: both got %v", pt1)
  	}

  	// Verify determinism: same date always returns the same value.
  	if wp.firstSessionPeriodizationType(monday1) != pt1 {
  		t.Error("firstSessionPeriodizationType is not deterministic")
  	}
  }
  ```

- [ ] **Step 2: Run to verify failure**

  ```bash
  go test ./internal/weekplanner/... -v -run TestFirstSessionPeriodizationType
  ```

  Expected: compilation error — `firstSessionPeriodizationType` undefined.

- [ ] **Step 3: Implement `firstSessionPeriodizationType` in weekplanner.go**

  Append to `internal/weekplanner/weekplanner.go`:

  ```go
  // exercisesPerWeek sums the exercise count across all scheduled days.
  func (wp *WeeklyPlanner) exercisesPerWeek() int {
  	total := 0
  	for _, wd := range []time.Weekday{
  		time.Monday, time.Tuesday, time.Wednesday,
  		time.Thursday, time.Friday, time.Saturday, time.Sunday,
  	} {
  		total += wp.Prefs.ExercisesPerSession(wd)
  	}
  	return total
  }

  // firstSessionPeriodizationType derives the periodization type for the first session of the
  // week deterministically from the start date and preferences — no DB query needed.
  func (wp *WeeklyPlanner) firstSessionPeriodizationType(startingDate time.Time) PeriodizationType {
  	const secondsPerWeek = 7 * 24 * 3600
  	weeksSinceEpoch := startingDate.Unix() / secondsPerWeek
  	epw := int64(wp.exercisesPerWeek())
  	if epw == 0 {
  		return PeriodizationStrength
  	}
  	if (weeksSinceEpoch*epw)%2 == 0 {
  		return PeriodizationStrength
  	}
  	return PeriodizationHypertrophy
  }
  ```

- [ ] **Step 4: Run tests**

  ```bash
  go test ./internal/weekplanner/... -v -run TestFirstSessionPeriodizationType
  ```

  Expected: PASS.

- [ ] **Step 5: Commit**

  ```bash
  git add internal/weekplanner/weekplanner.go internal/weekplanner/weekplanner_test.go
  git commit -m "feat: add deterministic periodization calculation to weekplanner"
  ```

---

## Task 5: weekplanner — Phase 2 muscle group slot allocation

**Files:**
- Modify: `internal/weekplanner/weekplanner.go`
- Modify: `internal/weekplanner/weekplanner_test.go`

- [ ] **Step 1: Write failing tests**

  Append to `internal/weekplanner/weekplanner_test.go`:

  ```go
  func minimalExercises() []Exercise {
  	return []Exercise{
  		{ID: 1, Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
  			PrimaryMuscleGroups: []string{"Quads", "Glutes"}},
  		{ID: 2, Category: CategoryLower, ExerciseType: ExerciseTypeWeighted,
  			PrimaryMuscleGroups: []string{"Hamstrings"}},
  		{ID: 3, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
  			PrimaryMuscleGroups: []string{"Chest", "Triceps", "Shoulders"}},
  		{ID: 4, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
  			PrimaryMuscleGroups: []string{"Lats", "Upper Back"}},
  		{ID: 5, Category: CategoryUpper, ExerciseType: ExerciseTypeWeighted,
  			PrimaryMuscleGroups: []string{"Biceps"}},
  		{ID: 6, Category: CategoryFullBody, ExerciseType: ExerciseTypeWeighted,
  			PrimaryMuscleGroups: []string{"Hamstrings", "Glutes"}},
  	}
  }

  func minimalTargets() []MuscleGroupTarget {
  	return []MuscleGroupTarget{
  		{Name: "Chest", WeeklySetTarget: 10},
  		{Name: "Shoulders", WeeklySetTarget: 10},
  		{Name: "Triceps", WeeklySetTarget: 8},
  		{Name: "Biceps", WeeklySetTarget: 8},
  		{Name: "Upper Back", WeeklySetTarget: 10},
  		{Name: "Lats", WeeklySetTarget: 10},
  		{Name: "Quads", WeeklySetTarget: 10},
  		{Name: "Hamstrings", WeeklySetTarget: 8},
  		{Name: "Glutes", WeeklySetTarget: 8},
  	}
  }

  func TestAllocateMuscleGroups(t *testing.T) {
  	// Mon(Lower), Tue(Upper), Thu(Full Body) schedule.
  	p := prefs(time.Monday, time.Tuesday, time.Thursday)
  	wp := NewWeeklyPlanner(p, minimalExercises(), minimalTargets())

  	mon := monday2026                   // Lower
  	tue := date(monday2026, 1)         // Upper
  	thu := date(monday2026, 3)         // Full Body

  	workoutDays := []time.Time{mon, tue, thu}
  	categories := map[time.Time]Category{
  		mon: CategoryLower,
  		tue: CategoryUpper,
  		thu: CategoryFullBody,
  	}

  	alloc := wp.allocateMuscleGroups(workoutDays, categories)

  	// Lower muscle groups (Quads, Hamstrings, Glutes) must appear on Mon (Lower
  	// compatible) and/or Thu (Full Body compatible), never on Tue (Upper only).
  	for _, mg := range []string{"Quads", "Hamstrings", "Glutes"} {
  		for _, assignedMG := range alloc[tue] {
  			if assignedMG == mg {
  				t.Errorf("lower muscle group %q must not be assigned to Upper day", mg)
  			}
  		}
  	}

  	// Upper muscle groups must not appear on Mon (Lower only).
  	for _, mg := range []string{"Chest", "Shoulders", "Triceps", "Biceps", "Upper Back", "Lats"} {
  		for _, assignedMG := range alloc[mon] {
  			if assignedMG == mg {
  				t.Errorf("upper muscle group %q must not be assigned to Lower day", mg)
  			}
  		}
  	}

  	// Every tracked muscle group must appear in at least 1 day's allocation.
  	allGroups := make(map[string]bool)
  	for _, groups := range alloc {
  		for _, g := range groups {
  			allGroups[g] = true
  		}
  	}
  	for _, target := range minimalTargets() {
  		if !allGroups[target.Name] {
  			t.Errorf("muscle group %q not assigned to any day", target.Name)
  		}
  	}
  }
  ```

- [ ] **Step 2: Run to verify failure**

  ```bash
  go test ./internal/weekplanner/... -v -run TestAllocateMuscleGroups
  ```

  Expected: compilation error — `allocateMuscleGroups` undefined.

- [ ] **Step 3: Implement `allocateMuscleGroups` and helpers in weekplanner.go**

  Append to `internal/weekplanner/weekplanner.go`:

  ```go
  // isCategoryCompatible reports whether an exercise of exerciseCategory can be
  // used on a day with dayCategory.
  //   - Full Body days accept all exercise categories.
  //   - Upper/Lower days only accept their matching exercise category.
  func isCategoryCompatible(exerciseCategory, dayCategory Category) bool {
  	if dayCategory == CategoryFullBody {
  		return true
  	}
  	return exerciseCategory == dayCategory
  }

  // hasCategoryExerciseForMuscleGroup reports whether the pool contains at least
  // one exercise compatible with dayCategory whose primary muscles include muscleGroup.
  func (wp *WeeklyPlanner) hasCategoryExerciseForMuscleGroup(dayCategory Category, muscleGroup string) bool {
  	for _, ex := range wp.Exercises {
  		if !isCategoryCompatible(ex.Category, dayCategory) {
  			continue
  		}
  		for _, mg := range ex.PrimaryMuscleGroups {
  			if mg == muscleGroup {
  				return true
  			}
  		}
  	}
  	return false
  }

  // allocateMuscleGroups assigns each tracked muscle group to up to 2 workout days
  // using a most-constrained-first greedy algorithm. A muscle group is valid for a
  // day if at least one compatible exercise targets it as a primary muscle.
  func (wp *WeeklyPlanner) allocateMuscleGroups(
  	workoutDays []time.Time,
  	categories map[time.Time]Category,
  ) map[time.Time][]string {
  	// Build valid-day lists for each muscle group.
  	type mgEntry struct {
  		name      string
  		validDays []time.Time
  	}
  	entries := make([]mgEntry, len(wp.Targets))
  	for i, target := range wp.Targets {
  		var valid []time.Time
  		for _, day := range workoutDays {
  			if wp.hasCategoryExerciseForMuscleGroup(categories[day], target.Name) {
  				valid = append(valid, day)
  			}
  		}
  		entries[i] = mgEntry{name: target.Name, validDays: valid}
  	}

  	// Sort ascending by number of valid days (most constrained first).
  	// Alphabetical name as tiebreaker for determinism.
  	slices.SortFunc(entries, func(a, b mgEntry) int {
  		if len(a.validDays) != len(b.validDays) {
  			return len(a.validDays) - len(b.validDays)
  		}
  		if a.name < b.name {
  			return -1
  		}
  		if a.name > b.name {
  			return 1
  		}
  		return 0
  	})

  	assignmentCount := make(map[time.Time]int)
  	result := make(map[time.Time][]string)

  	for _, entry := range entries {
  		if len(entry.validDays) == 0 {
  			continue
  		}

  		// Sort valid days by current assignment count (least loaded first).
  		sortedDays := slices.Clone(entry.validDays)
  		slices.SortFunc(sortedDays, func(a, b time.Time) int {
  			return assignmentCount[a] - assignmentCount[b]
  		})

  		// Assign to up to 2 days.
  		limit := min(2, len(sortedDays))
  		for i := range limit {
  			day := sortedDays[i]
  			result[day] = append(result[day], entry.name)
  			assignmentCount[day]++
  		}
  	}

  	return result
  }
  ```

- [ ] **Step 4: Run tests**

  ```bash
  go test ./internal/weekplanner/... -v -run TestAllocateMuscleGroups
  ```

  Expected: PASS.

- [ ] **Step 5: Commit**

  ```bash
  git add internal/weekplanner/weekplanner.go internal/weekplanner/weekplanner_test.go
  git commit -m "feat: add muscle group slot allocation to weekplanner"
  ```

---

## Task 6: weekplanner — Phase 3 exercise selection

**Files:**
- Modify: `internal/weekplanner/weekplanner.go`
- Modify: `internal/weekplanner/weekplanner_test.go`

- [ ] **Step 1: Write failing tests**

  Append to `internal/weekplanner/weekplanner_test.go`:

  ```go
  func TestSelectExercisesForDay(t *testing.T) {
  	p := prefs(time.Monday, time.Tuesday, time.Thursday)
  	wp := NewWeeklyPlanner(p, minimalExercises(), minimalTargets())
  	wp.rng = rand.New(rand.NewPCG(42, 0)) // fixed seed for determinism

  	t.Run("lower day only selects lower exercises", func(t *testing.T) {
  		sets := wp.selectExercisesForDay(CategoryLower, []string{"Quads", "Hamstrings"}, 2)
  		if len(sets) != 2 {
  			t.Fatalf("want 2 exercise sets, got %d", len(sets))
  		}
  		for _, es := range sets {
  			ex := findExercise(wp.Exercises, es.ExerciseID)
  			if ex.Category != CategoryLower {
  				t.Errorf("lower day got exercise with category %s", ex.Category)
  			}
  		}
  	})

  	t.Run("upper day only selects upper exercises", func(t *testing.T) {
  		sets := wp.selectExercisesForDay(CategoryUpper, []string{"Chest", "Lats"}, 2)
  		for _, es := range sets {
  			ex := findExercise(wp.Exercises, es.ExerciseID)
  			if ex.Category != CategoryUpper {
  				t.Errorf("upper day got exercise with category %s", ex.Category)
  			}
  		}
  	})

  	t.Run("full body day can select any category", func(t *testing.T) {
  		sets := wp.selectExercisesForDay(CategoryFullBody, []string{"Hamstrings", "Chest"}, 3)
  		categories := make(map[Category]bool)
  		for _, es := range sets {
  			ex := findExercise(wp.Exercises, es.ExerciseID)
  			categories[ex.Category] = true
  		}
  		// With Hamstrings and Chest as priorities, expect both lower and upper exercises selected.
  		if !categories[CategoryLower] || !categories[CategoryUpper] {
  			t.Error("full body day should draw from multiple categories when priorities span both")
  		}
  	})

  	t.Run("each exercise set has setsPerExercise sets", func(t *testing.T) {
  		sets := wp.selectExercisesForDay(CategoryUpper, []string{"Chest"}, 1)
  		if len(sets) != 1 {
  			t.Fatalf("want 1 exercise set, got %d", len(sets))
  		}
  		if len(sets[0].Sets) != setsPerExercise {
  			t.Errorf("want %d sets, got %d", setsPerExercise, len(sets[0].Sets))
  		}
  	})

  	t.Run("strength periodization sets correct rep range", func(t *testing.T) {
  		sets := wp.selectExercisesForDay(CategoryUpper, nil, 1)
  		for _, s := range sets[0].Sets {
  			if s.MinReps != minRepsStrength || s.MaxReps != maxRepsStrength {
  				t.Errorf("strength set: want min=%d max=%d, got min=%d max=%d",
  					minRepsStrength, maxRepsStrength, s.MinReps, s.MaxReps)
  			}
  		}
  	})
  }

  func findExercise(exercises []Exercise, id int) Exercise {
  	for _, ex := range exercises {
  		if ex.ID == id {
  			return ex
  		}
  	}
  	panic(fmt.Sprintf("exercise %d not found", id))
  }
  ```

  Add `"fmt"` to the test file imports.

- [ ] **Step 2: Run to verify failure**

  ```bash
  go test ./internal/weekplanner/... -v -run TestSelectExercisesForDay
  ```

  Expected: compilation error — `selectExercisesForDay` undefined.

- [ ] **Step 3: Implement `selectExercisesForDay` in weekplanner.go**

  Append to `internal/weekplanner/weekplanner.go`:

  ```go
  // setsForPeriodization returns MinReps/MaxReps for a PlannedSet based on periodization type.
  func setsForPeriodization(pt PeriodizationType) (minReps, maxReps int) {
  	if pt == PeriodizationStrength {
  		return minRepsStrength, maxRepsStrength
  	}
  	return minRepsHypertrophy, maxRepsHypertrophy
  }

  // scoreExercise returns how many of the priority muscle groups the exercise covers
  // via primary muscle groups and are not yet satisfied.
  func scoreExercise(ex Exercise, priority []string, satisfied map[string]bool) int {
  	score := 0
  	for _, mg := range ex.PrimaryMuscleGroups {
  		for _, p := range priority {
  			if mg == p && !satisfied[mg] {
  				score++
  			}
  		}
  	}
  	return score
  }

  // selectExercisesForDay picks n exercises for a day via category-filtered, score-based
  // greedy selection. priorityMuscleGroups are the muscle groups Phase 2 assigned to this day.
  // The periodization type is used to set MinReps/MaxReps on each planned set.
  // The default periodization is Strength; callers that need Hypertrophy pass it explicitly
  // via selectExercisesForDayWithPeriodization.
  func (wp *WeeklyPlanner) selectExercisesForDay(
  	category Category,
  	priorityMuscleGroups []string,
  	n int,
  ) []PlannedExerciseSet {
  	return wp.selectExercisesForDayWithPeriodization(category, priorityMuscleGroups, n, PeriodizationStrength)
  }

  func (wp *WeeklyPlanner) selectExercisesForDayWithPeriodization(
  	category Category,
  	priorityMuscleGroups []string,
  	n int,
  	pt PeriodizationType,
  ) []PlannedExerciseSet {
  	// Filter exercise pool by category compatibility.
  	pool := make([]Exercise, 0, len(wp.Exercises))
  	for _, ex := range wp.Exercises {
  		if isCategoryCompatible(ex.Category, category) {
  			pool = append(pool, ex)
  		}
  	}

  	satisfied := make(map[string]bool)
  	var selected []Exercise

  	for len(selected) < n && len(pool) > 0 {
  		// Find best score among remaining pool.
  		bestScore := -1
  		for _, ex := range pool {
  			if s := scoreExercise(ex, priorityMuscleGroups, satisfied); s > bestScore {
  				bestScore = s
  			}
  		}

  		// Collect all exercises with best score.
  		var candidates []int
  		for i, ex := range pool {
  			if scoreExercise(ex, priorityMuscleGroups, satisfied) == bestScore {
  				candidates = append(candidates, i)
  			}
  		}

  		// Pick one at random from best candidates.
  		chosen := candidates[wp.rng.IntN(len(candidates))]
  		ex := pool[chosen]
  		selected = append(selected, ex)

  		// Mark primary muscle groups satisfied.
  		for _, mg := range ex.PrimaryMuscleGroups {
  			satisfied[mg] = true
  		}

  		// Remove chosen from pool.
  		pool = append(pool[:chosen], pool[chosen+1:]...)
  	}

  	// Build PlannedExerciseSets.
  	minR, maxR := setsForPeriodization(pt)
  	sets := make([]PlannedSet, setsPerExercise)
  	for i := range sets {
  		sets[i] = PlannedSet{MinReps: minR, MaxReps: maxR}
  	}

  	result := make([]PlannedExerciseSet, len(selected))
  	for i, ex := range selected {
  		result[i] = PlannedExerciseSet{
  			ExerciseID: ex.ID,
  			Sets:       slices.Clone(sets),
  		}
  	}
  	return result
  }
  ```

- [ ] **Step 4: Run tests**

  ```bash
  go test ./internal/weekplanner/... -v -run TestSelectExercisesForDay
  ```

  Expected: PASS.

- [ ] **Step 5: Commit**

  ```bash
  git add internal/weekplanner/weekplanner.go internal/weekplanner/weekplanner_test.go
  git commit -m "feat: add exercise selection phase to weekplanner"
  ```

---

## Task 7: weekplanner — `Plan()` orchestration

**Files:**
- Modify: `internal/weekplanner/weekplanner.go`
- Modify: `internal/weekplanner/weekplanner_test.go`

- [ ] **Step 1: Write failing tests**

  Append to `internal/weekplanner/weekplanner_test.go`:

  ```go
  func TestPlan(t *testing.T) {
  	exercises := minimalExercises()
  	targets := minimalTargets()

  	t.Run("returns error for non-Monday start date", func(t *testing.T) {
  		p := prefs(time.Monday, time.Wednesday)
  		wp := NewWeeklyPlanner(p, exercises, targets)
  		_, err := wp.Plan(date(monday2026, 1)) // Tuesday
  		if err == nil {
  			t.Error("want error for non-Monday start date, got nil")
  		}
  	})

  	t.Run("returns error when no workout days scheduled", func(t *testing.T) {
  		wp := NewWeeklyPlanner(Preferences{}, exercises, targets)
  		_, err := wp.Plan(monday2026)
  		if err == nil {
  			t.Error("want error when no workout days scheduled, got nil")
  		}
  	})

  	t.Run("returns one session per scheduled day", func(t *testing.T) {
  		p := prefs(time.Monday, time.Wednesday, time.Friday)
  		wp := NewWeeklyPlanner(p, exercises, targets)
  		wp.rng = rand.New(rand.NewPCG(1, 0))

  		sessions, err := wp.Plan(monday2026)
  		if err != nil {
  			t.Fatalf("Plan returned error: %v", err)
  		}
  		if len(sessions) != 3 {
  			t.Fatalf("want 3 sessions, got %d", len(sessions))
  		}
  	})

  	t.Run("session dates match scheduled weekdays", func(t *testing.T) {
  		p := prefs(time.Monday, time.Wednesday, time.Friday)
  		wp := NewWeeklyPlanner(p, exercises, targets)
  		wp.rng = rand.New(rand.NewPCG(1, 0))

  		sessions, err := wp.Plan(monday2026)
  		if err != nil {
  			t.Fatalf("Plan returned error: %v", err)
  		}
  		expected := []time.Weekday{time.Monday, time.Wednesday, time.Friday}
  		for i, sess := range sessions {
  			if sess.Date.Weekday() != expected[i] {
  				t.Errorf("session %d: want %s, got %s", i, expected[i], sess.Date.Weekday())
  			}
  		}
  	})

  	t.Run("each session has correct exercise count for duration", func(t *testing.T) {
  		// 60 min → 3 exercises
  		p := prefs(time.Monday, time.Wednesday)
  		wp := NewWeeklyPlanner(p, exercises, targets)
  		wp.rng = rand.New(rand.NewPCG(2, 0))

  		sessions, err := wp.Plan(monday2026)
  		if err != nil {
  			t.Fatalf("Plan returned error: %v", err)
  		}
  		for _, sess := range sessions {
  			if len(sess.ExerciseSets) != 3 {
  				t.Errorf("60-min session: want 3 exercises, got %d", len(sess.ExerciseSets))
  			}
  		}
  	})

  	t.Run("consecutive sessions alternate periodization", func(t *testing.T) {
  		p := prefs(time.Monday, time.Tuesday)
  		wp := NewWeeklyPlanner(p, exercises, targets)
  		wp.rng = rand.New(rand.NewPCG(3, 0))

  		sessions, err := wp.Plan(monday2026)
  		if err != nil {
  			t.Fatalf("Plan returned error: %v", err)
  		}
  		if len(sessions) < 2 {
  			t.Fatal("need at least 2 sessions to test alternation")
  		}
  		if sessions[0].PeriodizationType == sessions[1].PeriodizationType {
  			t.Error("consecutive sessions must have different periodization types")
  		}
  	})
  }
  ```

- [ ] **Step 2: Run to verify failure**

  ```bash
  go test ./internal/weekplanner/... -v -run TestPlan
  ```

  Expected: compilation error — `Plan` undefined.

- [ ] **Step 3: Implement `Plan()` and `hasExercisesForCategory` in weekplanner.go**

  Append to `internal/weekplanner/weekplanner.go`:

  ```go
  // hasExercisesForCategory reports whether the exercise pool contains at least one
  // exercise compatible with the given day category.
  func (wp *WeeklyPlanner) hasExercisesForCategory(category Category) bool {
  	for _, ex := range wp.Exercises {
  		if isCategoryCompatible(ex.Category, category) {
  			return true
  		}
  	}
  	return false
  }

  // Plan generates one PlannedSession per scheduled workout day for the week beginning on
  // startingDate. Returns an error if startingDate is not a Monday, if no workout days are
  // scheduled, or if a scheduled day has no compatible exercises.
  func (wp *WeeklyPlanner) Plan(startingDate time.Time) ([]PlannedSession, error) {
  	if startingDate.Weekday() != time.Monday {
  		return nil, fmt.Errorf("startingDate must be a Monday, got %s", startingDate.Weekday())
  	}

  	// Collect scheduled workout days Mon–Sun.
  	var workoutDays []time.Time
  	for i := range 7 {
  		day := startingDate.AddDate(0, 0, i)
  		if wp.Prefs.IsWorkoutDay(day.Weekday()) {
  			workoutDays = append(workoutDays, day)
  		}
  	}
  	if len(workoutDays) == 0 {
  		return nil, fmt.Errorf("no workout days scheduled in preferences")
  	}

  	// Phase 1: determine category for each scheduled day.
  	categories := make(map[time.Time]Category, len(workoutDays))
  	for _, day := range workoutDays {
  		cat := wp.determineCategory(day)
  		if !wp.hasExercisesForCategory(cat) {
  			return nil, fmt.Errorf("no exercises available for %s day (%s)", cat, day.Weekday())
  		}
  		categories[day] = cat
  	}

  	// Phase 2: allocate muscle group slots across days.
  	dayMuscleGroups := wp.allocateMuscleGroups(workoutDays, categories)

  	// Determine periodization type for first session.
  	firstPT := wp.firstSessionPeriodizationType(startingDate)

  	// Phase 3: select exercises and build sessions.
  	sessions := make([]PlannedSession, len(workoutDays))
  	for i, day := range workoutDays {
  		pt := PeriodizationType((int(firstPT) + i) % 2)
  		n := wp.Prefs.ExercisesPerSession(day.Weekday())
  		exerciseSets := wp.selectExercisesForDayWithPeriodization(
  			categories[day],
  			dayMuscleGroups[day],
  			n,
  			pt,
  		)
  		sessions[i] = PlannedSession{
  			Date:              day,
  			Category:          categories[day],
  			PeriodizationType: pt,
  			ExerciseSets:      exerciseSets,
  		}
  	}

  	return sessions, nil
  }
  ```

- [ ] **Step 4: Run all weekplanner tests**

  ```bash
  go test ./internal/weekplanner/... -v
  ```

  Expected: all tests pass.

- [ ] **Step 5: Commit**

  ```bash
  git add internal/weekplanner/weekplanner.go internal/weekplanner/weekplanner_test.go
  git commit -m "feat: implement Plan() orchestration in weekplanner"
  ```

---

## Task 8: Service — `generateWeeklyPlan` and updated `ResolveWeeklySchedule`

**Files:**
- Modify: `internal/workout/service.go`
- Modify: `internal/workout/service_test.go`

- [ ] **Step 1: Write failing service integration tests**

  Append to `internal/workout/service_test.go`:

  ```go
  func setupTestService(t *testing.T) (context.Context, *workout.Service) {
  	t.Helper()
  	ctx := t.Context()
  	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
  	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
  	if err != nil {
  		t.Fatalf("create test database: %v", err)
  	}
  	t.Cleanup(func() { _ = db.Close() })

  	var userID int
  	err = db.ReadWrite.QueryRowContext(ctx,
  		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
  		[]byte("test-user"), "Test User").Scan(&userID)
  	if err != nil {
  		t.Fatalf("insert test user: %v", err)
  	}
  	ctx = contexthelpers.WithAuthenticatedUserID(ctx, userID)

  	// Set preferences: Mon, Wed, Fri at 60 min.
  	svc := workout.NewService(db, logger, "")
  	if err = svc.SaveUserPreferences(ctx, workout.Preferences{
  		MondayMinutes:    60,
  		WednesdayMinutes: 60,
  		FridayMinutes:    60,
  	}); err != nil {
  		t.Fatalf("save preferences: %v", err)
  	}
  	return ctx, svc
  }

  func Test_ResolveWeeklySchedule_GeneratesFullWeekOnFirstLoad(t *testing.T) {
  	ctx, svc := setupTestService(t)

  	sessions, err := svc.ResolveWeeklySchedule(ctx)
  	if err != nil {
  		t.Fatalf("ResolveWeeklySchedule: %v", err)
  	}
  	if len(sessions) != 7 {
  		t.Fatalf("want 7 sessions (one per day), got %d", len(sessions))
  	}

  	// Scheduled days (Mon=0, Wed=2, Fri=4) must have exercises.
  	for _, i := range []int{0, 2, 4} {
  		if len(sessions[i].ExerciseSets) == 0 {
  			t.Errorf("sessions[%d] (%s) must have exercise sets", i, sessions[i].Date.Weekday())
  		}
  	}

  	// Rest days must be empty sessions.
  	for _, i := range []int{1, 3, 5, 6} {
  		if len(sessions[i].ExerciseSets) != 0 {
  			t.Errorf("sessions[%d] (%s) must be empty (rest day)", i, sessions[i].Date.Weekday())
  		}
  	}
  }

  func Test_ResolveWeeklySchedule_DoesNotRegenerateExistingSessions(t *testing.T) {
  	ctx, svc := setupTestService(t)

  	sessions1, err := svc.ResolveWeeklySchedule(ctx)
  	if err != nil {
  		t.Fatalf("first ResolveWeeklySchedule: %v", err)
  	}

  	sessions2, err := svc.ResolveWeeklySchedule(ctx)
  	if err != nil {
  		t.Fatalf("second ResolveWeeklySchedule: %v", err)
  	}

  	// Same scheduled days must have the same exercise IDs on both calls.
  	for _, i := range []int{0, 2, 4} {
  		ids1 := extractExerciseIDs(sessions1[i])
  		ids2 := extractExerciseIDs(sessions2[i])
  		if !slices.Equal(ids1, ids2) {
  			t.Errorf("sessions[%d] exercise IDs changed on second call: %v → %v", i, ids1, ids2)
  		}
  	}
  }

  func Test_GetSession_ReturnsErrNotFoundForUnplannedDate(t *testing.T) {
  	ctx, svc := setupTestService(t)

  	// Generate this week's plan.
  	if _, err := svc.ResolveWeeklySchedule(ctx); err != nil {
  		t.Fatalf("ResolveWeeklySchedule: %v", err)
  	}

  	// Request a date in a different week.
  	nextWeekTuesday := time.Now().AddDate(0, 0, 14)
  	_, err := svc.GetSession(ctx, nextWeekTuesday)
  	if !errors.Is(err, workout.ErrNotFound) {
  		t.Errorf("want ErrNotFound for unplanned date, got %v", err)
  	}
  }

  func extractExerciseIDs(session workout.Session) []int {
  	ids := make([]int, len(session.ExerciseSets))
  	for i, es := range session.ExerciseSets {
  		ids[i] = es.Exercise.ID
  	}
  	return ids
  }
  ```

  Add missing imports to `service_test.go`: `"slices"`, `"time"`, `"github.com/myrjola/petrapp/internal/contexthelpers"`.

- [ ] **Step 2: Run to verify failure**

  ```bash
  go test ./internal/workout/... -v -run "Test_ResolveWeeklySchedule|Test_GetSession_ReturnsErrNotFound"
  ```

  Expected: tests fail — `ResolveWeeklySchedule` still generates per-day with the old generator.

- [ ] **Step 3: Update `service.go` — add `generateWeeklyPlan`, update `ResolveWeeklySchedule`**

  Replace the existing `generateWorkout` and `ResolveWeeklySchedule` methods in `internal/workout/service.go`. Remove `generateWorkout` entirely and replace `ResolveWeeklySchedule`:

  ```go
  // ResolveWeeklySchedule retrieves the workout schedule for the current week.
  // If no sessions exist for the week, it generates all scheduled days at once using
  // the weekly planner and persists them in a single transaction.
  func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]Session, error) {
  	now := time.Now()
  	offset := int(time.Monday - now.Weekday())
  	if offset > 0 {
  		offset = -6 //nolint:mnd // If today is Sunday, adjust to get last Monday.
  	}
  	monday := now.AddDate(0, 0, offset).Truncate(24 * time.Hour)
  	sunday := monday.AddDate(0, 0, 6)

  	// Check for existing sessions this week.
  	existing, err := s.repo.sessions.List(ctx, monday)
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

  	// Build 7-day schedule: sessions from DB for scheduled days, empty for rest days.
  	workouts := make([]Session, 7) //nolint:mnd
  	for i := range 7 {
  		day := monday.AddDate(0, 0, i)
  		sessionAggr, getErr := s.repo.sessions.Get(ctx, day)
  		if getErr != nil && !errors.Is(getErr, ErrNotFound) {
  			return nil, fmt.Errorf("get session %s: %w", formatDate(day), getErr)
  		}
  		if errors.Is(getErr, ErrNotFound) {
  			workouts[i] = Session{Date: day}
  			continue
  		}
  		workouts[i], err = s.enrichSessionAggregate(ctx, sessionAggr)
  		if err != nil {
  			return nil, fmt.Errorf("enrich session %s: %w", formatDate(day), err)
  		}
  	}
  	return workouts, nil
  }

  // generateWeeklyPlan uses the weekplanner to create all sessions for the week starting
  // on monday and persists them in a single DB transaction.
  func (s *Service) generateWeeklyPlan(ctx context.Context, monday time.Time) error {
  	prefs, err := s.repo.prefs.Get(ctx)
  	if err != nil {
  		return fmt.Errorf("get preferences: %w", err)
  	}

  	exercises, err := s.repo.exercises.List(ctx)
  	if err != nil {
  		return fmt.Errorf("get exercises: %w", err)
  	}

  	targets, err := s.repo.muscleTargets.List(ctx)
  	if err != nil {
  		return fmt.Errorf("get muscle group targets: %w", err)
  	}

  	wpPrefs := weekplanner.Preferences{
  		MondayMinutes:    prefs.MondayMinutes,
  		TuesdayMinutes:   prefs.TuesdayMinutes,
  		WednesdayMinutes: prefs.WednesdayMinutes,
  		ThursdayMinutes:  prefs.ThursdayMinutes,
  		FridayMinutes:    prefs.FridayMinutes,
  		SaturdayMinutes:  prefs.SaturdayMinutes,
  		SundayMinutes:    prefs.SundayMinutes,
  	}

  	wpExercises := make([]weekplanner.Exercise, len(exercises))
  	for i, ex := range exercises {
  		wpExercises[i] = weekplanner.Exercise{
  			ID:                    ex.ID,
  			Category:              weekplanner.Category(ex.Category),
  			ExerciseType:          weekplanner.ExerciseType(ex.ExerciseType),
  			PrimaryMuscleGroups:   ex.PrimaryMuscleGroups,
  			SecondaryMuscleGroups: ex.SecondaryMuscleGroups,
  		}
  	}

  	wpTargets := make([]weekplanner.MuscleGroupTarget, len(targets))
  	for i, t := range targets {
  		wpTargets[i] = weekplanner.MuscleGroupTarget{
  			Name:           t.MuscleGroupName,
  			WeeklySetTarget: t.WeeklySetTarget,
  		}
  	}

  	planner := weekplanner.NewWeeklyPlanner(wpPrefs, wpExercises, wpTargets)
  	plannedSessions, err := planner.Plan(monday)
  	if err != nil {
  		return fmt.Errorf("plan week: %w", err)
  	}

  	sessionAggrs := make([]sessionAggregate, len(plannedSessions))
  	for i, ps := range plannedSessions {
  		periodType := PeriodizationStrength
  		if ps.PeriodizationType == weekplanner.PeriodizationHypertrophy {
  			periodType = PeriodizationHypertrophy
  		}

  		exerciseSets := make([]exerciseSetAggregate, len(ps.ExerciseSets))
  		for j, pes := range ps.ExerciseSets {
  			sets := make([]Set, len(pes.Sets))
  			for k, s := range pes.Sets {
  				sets[k] = Set{MinReps: s.MinReps, MaxReps: s.MaxReps}
  			}
  			exerciseSets[j] = exerciseSetAggregate{
  				ExerciseID: pes.ExerciseID,
  				Sets:       sets,
  			}
  		}

  		sessionAggrs[i] = sessionAggregate{
  			Date:              ps.Date,
  			PeriodizationType: periodType,
  			ExerciseSets:      exerciseSets,
  		}
  	}

  	return s.repo.sessions.CreateBatch(ctx, sessionAggrs)
  }
  ```

  Add `"github.com/myrjola/petrapp/internal/weekplanner"` to the imports in `service.go`.

- [ ] **Step 4: Run tests**

  ```bash
  go test ./internal/workout/... -v -run "Test_ResolveWeeklySchedule|Test_GetSession_ReturnsErrNotFound"
  ```

  Expected: all three new service tests pass.

- [ ] **Step 5: Run full test suite**

  ```bash
  make test
  ```

  Expected: all tests pass. The existing service test `Test_UpdateExercise_PreservesExerciseSets` must still pass.

- [ ] **Step 6: Commit**

  ```bash
  git add internal/workout/service.go internal/workout/service_test.go
  git commit -m "feat: replace per-day generation with weekly planner in service"
  ```

---

## Task 9: Handler and template updates for unplanned dates

**Files:**
- Modify: `cmd/web/handler-workout.go`
- Modify: `ui/templates/pages/workout-not-found/workout-not-found.gohtml`

- [ ] **Step 1: Update `workoutStartPOST` in handler-workout.go**

  Replace the `workoutStartPOST` function body so it handles `ErrNotFound` instead of 500-ing:

  ```go
  func (app *application) workoutStartPOST(w http.ResponseWriter, r *http.Request) {
  	date, ok := app.parseDateParam(w, r)
  	if !ok {
  		return
  	}

  	if err := app.workoutService.StartSession(r.Context(), date); err != nil {
  		if errors.Is(err, workout.ErrNotFound) {
  			data := workoutNotFoundTemplateData{
  				BaseTemplateData: newBaseTemplateData(r),
  				Date:             date,
  			}
  			app.render(w, r, http.StatusNotFound, "workout-not-found", data)
  			return
  		}
  		app.serverError(w, r, err)
  		return
  	}

  	redirect(w, r, fmt.Sprintf("/workouts/%s", date.Format("2006-01-02")))
  }
  ```

- [ ] **Step 2: Update `workout-not-found.gohtml` to remove "Create Workout" button**

  Replace the full content of `ui/templates/pages/workout-not-found/workout-not-found.gohtml`:

  ```gohtml
  {{- /*gotype: github.com/myrjola/petrapp/cmd/web.workoutNotFoundTemplateData*/ -}}

  {{ define "page" }}
      <main>
          <style {{ nonce }}>
              @scope {
                  :scope {
                      margin: var(--size-4);
                      display: flex;
                      flex-direction: column;
                      gap: var(--size-4);
                      align-items: center;
                      text-align: center;
                      padding: var(--size-6);
                  }

                  .message {
                      max-width: 400px;
                  }

                  .message h1 {
                      color: var(--gray-9);
                      font-size: var(--font-size-4);
                      margin-bottom: var(--size-3);
                  }

                  .message p {
                      color: var(--gray-7);
                      font-size: var(--font-size-1);
                      margin-bottom: var(--size-4);
                      line-height: 1.6;
                  }

                  .actions {
                      display: flex;
                      flex-direction: column;
                      gap: var(--size-3);
                      width: 100%;
                      max-width: 200px;
                  }

                  .primary-button {
                      background-color: var(--lime-6);
                      color: var(--white);
                      border: none;
                      border-radius: var(--radius-2);
                      padding: var(--size-3) var(--size-4);
                      font-weight: var(--font-weight-6);
                      cursor: pointer;
                      text-decoration: none;
                      display: inline-block;
                      font-size: var(--font-size-1);

                      &:hover {
                          background-color: var(--lime-7);
                      }
                  }
              }
          </style>

          <div class="message">
              <h1>Not in This Week's Plan</h1>
              <p>{{ .Date.Format "January 2, 2006" }} is not part of your current weekly plan.
                  Your plan is generated each Monday based on your scheduled workout days.</p>
          </div>

          <div class="actions">
              <a href="/" class="primary-button">Back to Home</a>
          </div>
      </main>
  {{ end }}
  ```

- [ ] **Step 3: Build and verify**

  ```bash
  make build
  ```

  Expected: builds successfully.

- [ ] **Step 4: Run tests**

  ```bash
  make test
  ```

  Expected: all tests pass.

- [ ] **Step 5: Commit**

  ```bash
  git add cmd/web/handler-workout.go \
          ui/templates/pages/workout-not-found/workout-not-found.gohtml
  git commit -m "fix: render not-in-plan page for unscheduled workout dates"
  ```

---

## Task 10: Delete generator.go and run full CI

**Files:**
- Delete: `internal/workout/generator.go`
- Delete: `internal/workout/generator_internal_test.go`

- [ ] **Step 1: Delete generator files**

  ```bash
  git rm internal/workout/generator.go internal/workout/generator_internal_test.go
  ```

- [ ] **Step 2: Check for remaining references**

  ```bash
  grep -r "newGenerator\|generator\." internal/workout/ --include="*.go"
  ```

  Expected: no matches. If any remain, the service still references the old generator — fix them.

- [ ] **Step 3: Run full CI**

  ```bash
  make ci
  ```

  Expected: all checks pass: build, lint, tests, security scan.

- [ ] **Step 4: Commit**

  ```bash
  git commit -m "refactor: delete generator.go, replaced by weekplanner package"
  ```

---

## Self-Review Checklist

- [x] **Schema task covers** `muscle_group_weekly_targets` table + fixtures seeded
- [x] **MuscleGroupTarget repository** covers type in models.go, interface in repository.go, implementation in repository-muscle-targets.go, wired in newRepository()
- [x] **CreateBatch** covers interface addition and sqlite implementation
- [x] **weekplanner Phase 1** (category determination) — tested including week-wrap edge case
- [x] **weekplanner Phase 2** (muscle group allocation) — tested category compatibility and 2× frequency
- [x] **weekplanner Phase 3** (exercise selection) — tested category filtering, scoring, rep ranges
- [x] **weekplanner Plan()** — tested error cases, session count, dates, exercise count, periodization alternation
- [x] **Service ResolveWeeklySchedule** — generates on first load, stable on repeat, returns 7 sessions with empty rest days
- [x] **Service GetSession** — unchanged; already returns ErrNotFound when not found
- [x] **Handler workoutStartPOST** — handles ErrNotFound with workout-not-found page
- [x] **workout-not-found template** — "Create Workout" form removed, message updated
- [x] **generator.go deleted** — no remaining references
- [x] **generator-exercise.go kept** — AI exercise creation unrelated to this feature
