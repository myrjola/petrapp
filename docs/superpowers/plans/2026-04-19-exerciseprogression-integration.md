# exerciseprogression Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `internal/exerciseprogression` into the workout service and exercise set UI, replacing the static weight+rep-range model for weighted exercises with RIR signal-driven auto-regulation and two-type DUP periodization.

**Architecture:** Approach A — extend `workout.Service` directly. A `periodization_type` column on `workout_sessions` drives the rep target; a `signal` column on `exercise_sets` stores the user's effort rating per set. The service reconstructs a `Progression` via `NewFromHistory` on each request (stateless, DB-backed). Handlers call `BuildProgression()` to get the next set recommendation and `RecordSetCompletion()` to persist signal + weight + reps atomically.

**Tech Stack:** Go 1.23, SQLite (STRICT mode), `internal/exerciseprogression` package, Go templates with inline scoped CSS/JS.

**Design spec:** `docs/superpowers/specs/2026-04-19-exerciseprogression-integration-design.md`

---

### Task 1: Schema — add periodization_type and signal columns

**Files:**
- Modify: `internal/sqlite/schema.sql`

- [ ] **Step 1: Add `periodization_type` to `workout_sessions`**

In `schema.sql`, replace the `workout_sessions` CREATE TABLE block (lines 88–98) with:

```sql
CREATE TABLE workout_sessions
(
    user_id            INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workout_date       TEXT    NOT NULL CHECK (LENGTH(workout_date) <= 10 AND
                                               DATE(workout_date, '+0 days') IS workout_date),
    difficulty_rating  INTEGER CHECK (difficulty_rating BETWEEN 1 AND 5),
    started_at         TEXT CHECK (started_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', started_at) = started_at),
    completed_at       TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
    periodization_type TEXT    NOT NULL DEFAULT 'strength'
        CHECK (periodization_type IN ('strength', 'hypertrophy')),

    PRIMARY KEY (user_id, workout_date)
) WITHOUT ROWID, STRICT;
```

- [ ] **Step 2: Add `signal` to `exercise_sets`**

In `schema.sql`, replace the `exercise_sets` CREATE TABLE block (lines 113–128) with:

```sql
CREATE TABLE exercise_sets
(
    workout_user_id INTEGER NOT NULL,
    workout_date    TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%d', workout_date) = workout_date),
    exercise_id     INTEGER NOT NULL,
    set_number      INTEGER NOT NULL CHECK (set_number > 0),
    weight_kg       REAL CHECK (weight_kg IS NULL OR weight_kg >= 0),
    min_reps        INTEGER NOT NULL CHECK (min_reps > 0),
    max_reps        INTEGER NOT NULL CHECK (max_reps >= min_reps),
    completed_reps  INTEGER,
    completed_at    TEXT CHECK (completed_at IS NULL OR STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
    signal          TEXT CHECK (signal IS NULL OR signal IN ('too_heavy', 'on_target', 'too_light')),

    PRIMARY KEY (workout_user_id, workout_date, exercise_id, set_number),
    FOREIGN KEY (workout_user_id, workout_date) REFERENCES workout_sessions (user_id, workout_date) ON DELETE CASCADE,
    FOREIGN KEY (exercise_id) REFERENCES exercises (id) DEFERRABLE INITIALLY DEFERRED
) WITHOUT ROWID, STRICT;
```

- [ ] **Step 3: Verify migration applies cleanly**

```bash
make test
```

Expected: all tests pass; migration system detects both new columns and applies them.

- [ ] **Step 4: Commit**

```bash
git add internal/sqlite/schema.sql
git commit -m "feat: add periodization_type to workout_sessions and signal to exercise_sets"
```

---

### Task 2: Domain types — PeriodizationType, Signal, update Set and Session

**Files:**
- Modify: `internal/workout/models.go`

- [ ] **Step 1: Add PeriodizationType type and constants**

After the `ExerciseType` constants block (after line 24), add:

```go
// PeriodizationType determines the fixed rep target for all exercises in a session.
type PeriodizationType string

const (
	PeriodizationStrength    PeriodizationType = "strength"
	PeriodizationHypertrophy PeriodizationType = "hypertrophy"
)

// Signal is the user's perceived effort after completing a set.
type Signal string

const (
	SignalTooHeavy Signal = "too_heavy"
	SignalOnTarget Signal = "on_target"
	SignalTooLight Signal = "too_light"
)
```

- [ ] **Step 2: Add Signal field to Set**

Replace the `Set` struct (lines 103–109) with:

```go
// Set represents a single set of an exercise with target and actual performance.
type Set struct {
	WeightKg      *float64   // Nullable for bodyweight exercises
	MinReps       int
	MaxReps       int
	CompletedReps *int
	CompletedAt   *time.Time // Nullable timestamp when set was completed
	Signal        *Signal    // Nullable; nil until the set is completed
}
```

- [ ] **Step 3: Add PeriodizationType field to Session**

Replace the `Session` struct (lines 131–137) with:

```go
// Session represents a complete workout session including all exercises and their sets.
type Session struct {
	Date              time.Time
	DifficultyRating  *int
	StartedAt         time.Time
	CompletedAt       time.Time
	ExerciseSets      []ExerciseSet
	PeriodizationType PeriodizationType
}
```

- [ ] **Step 4: Verify compilation**

```bash
go build ./...
```

Expected: compiles without errors.

- [ ] **Step 5: Commit**

```bash
git add internal/workout/models.go
git commit -m "feat: add PeriodizationType, Signal domain types and update Set, Session structs"
```

---

### Task 3: Repository — wire periodization_type through sessionAggregate and SQL

**Files:**
- Modify: `internal/workout/repository.go`
- Modify: `internal/workout/repository-sessions.go`
- Modify: `internal/workout/service.go`

- [ ] **Step 1: Add PeriodizationType to sessionAggregate**

In `repository.go`, replace the `sessionAggregate` struct (lines 35–41) with:

```go
// sessionAggregate represents a complete workout session including all exercises and their sets.
type sessionAggregate struct {
	Date              time.Time
	DifficultyRating  *int
	StartedAt         time.Time
	CompletedAt       time.Time
	ExerciseSets      []exerciseSetAggregate
	PeriodizationType PeriodizationType
}
```

- [ ] **Step 2: Update List query and scan**

In `repository-sessions.go`, replace the `List` method's query and scan logic.

Replace the query string (currently at line 33):
```go
	query := `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type
		FROM workout_sessions
		WHERE user_id = ? AND workout_date >= ?
		ORDER BY workout_date DESC`
```

Replace the Scan variables and call (currently at lines 51–58):
```go
		var (
			workoutDateStr    string
			difficultyRating  sql.NullInt32
			startedAtStr      sql.NullString
			completedAtStr    sql.NullString
			periodizationType string
		)

		if err = rows.Scan(&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}

		var session sessionAggregate
		session, err = r.parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType)
```

- [ ] **Step 3: Update Get query and scan**

In `repository-sessions.go`, replace the `Get` method's query (currently at line 98):
```go
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type
		FROM workout_sessions
		WHERE user_id = ? AND workout_date = ?`,
		userID, dateStr).Scan(&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType)
```

And update the variable declarations before the QueryRowContext call:
```go
	var (
		workoutDateStr    string
		difficultyRating  sql.NullInt32
		startedAtStr      sql.NullString
		completedAtStr    sql.NullString
		periodizationType string
	)
```

And the `parseSessionRow` call:
```go
	session, err := r.parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType)
```

- [ ] **Step 4: Update set() INSERT to include periodization_type**

In `repository-sessions.go`, replace the INSERT in `set()` (currently at lines 162–166):
```go
	_, err = tx.ExecContext(ctx, `
		INSERT INTO workout_sessions (
			user_id, workout_date, difficulty_rating, started_at, completed_at, periodization_type
		) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, dateStr, sess.DifficultyRating, formatTimestamp(sess.StartedAt), formatTimestamp(sess.CompletedAt),
		string(sess.PeriodizationType))
```

- [ ] **Step 5: Update parseSessionRow signature and body**

Replace the `parseSessionRow` method (currently at lines 212–247):
```go
func (r *sqliteSessionRepository) parseSessionRow(
	workoutDateStr string,
	difficultyRating sql.NullInt32,
	startedAtStr sql.NullString,
	completedAtStr sql.NullString,
	periodizationType string,
) (sessionAggregate, error) {
	var session sessionAggregate

	date, err := time.Parse(dateFormat, workoutDateStr)
	if err != nil {
		return sessionAggregate{}, fmt.Errorf("parse workout date: %w", err)
	}
	session.Date = date

	if difficultyRating.Valid {
		rating := int(difficultyRating.Int32)
		session.DifficultyRating = &rating
	}

	var startedAt time.Time
	if startedAt, err = parseTimestamp(startedAtStr); err != nil {
		return sessionAggregate{}, fmt.Errorf("parse started_at: %w", err)
	}
	session.StartedAt = startedAt

	var completedAt time.Time
	if completedAt, err = parseTimestamp(completedAtStr); err != nil {
		return sessionAggregate{}, fmt.Errorf("parse completed_at: %w", err)
	}
	session.CompletedAt = completedAt

	session.PeriodizationType = PeriodizationType(periodizationType)

	return session, nil
}
```

- [ ] **Step 6: Update enrichSessionAggregate in service.go to propagate PeriodizationType**

In `service.go`, replace the `session` initializer inside `enrichSessionAggregate` (currently at lines 145–151):
```go
	session := Session{
		Date:              sessionAggr.Date,
		StartedAt:         sessionAggr.StartedAt,
		CompletedAt:       sessionAggr.CompletedAt,
		DifficultyRating:  sessionAggr.DifficultyRating,
		PeriodizationType: sessionAggr.PeriodizationType,
		ExerciseSets:      make([]ExerciseSet, len(sessionAggr.ExerciseSets)),
	}
```

- [ ] **Step 7: Run tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/workout/repository.go internal/workout/repository-sessions.go internal/workout/service.go
git commit -m "feat: wire periodization_type through sessionAggregate and SQL layer"
```

---

### Task 4: Repository — wire signal through exercise_sets SQL

**Files:**
- Modify: `internal/workout/repository-sessions.go`

- [ ] **Step 1: Update loadExerciseSets query to SELECT signal**

In `repository-sessions.go`, replace the `loadExerciseSets` query (currently at lines 258–266):
```go
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT es.exercise_id, es.weight_kg, es.min_reps, es.max_reps, es.completed_reps,
		       es.completed_at, we.warmup_completed_at, es.signal
		FROM exercise_sets es
		LEFT JOIN workout_exercise we ON we.workout_user_id = es.workout_user_id
		                              AND we.workout_date = es.workout_date
		                              AND we.exercise_id = es.exercise_id
		WHERE es.workout_user_id = ? AND es.workout_date = ?
		ORDER BY es.exercise_id, es.set_number`,
		userID, dateStr)
```

- [ ] **Step 2: Update loadExerciseSets scan to include signalStr**

In `repository-sessions.go`, replace the variables and Scan call in the `loadExerciseSets` loop (currently at lines 281–292):
```go
		var (
			exerciseID           int
			set                  Set
			completedAtStr       sql.NullString
			warmupCompletedAtStr sql.NullString
			signalStr            sql.NullString
		)
		err = rows.Scan(&exerciseID, &set.WeightKg, &set.MinReps, &set.MaxReps,
			&set.CompletedReps, &completedAtStr, &warmupCompletedAtStr, &signalStr)
		if err != nil {
			return nil, fmt.Errorf("scan exercise set: %w", err)
		}

		if err = r.parseCompletedAtTimestamp(completedAtStr, &set); err != nil {
			return nil, err
		}

		if signalStr.Valid {
			s := Signal(signalStr.String)
			set.Signal = &s
		}
```

- [ ] **Step 3: Update ListSetsForExerciseSince query to SELECT signal**

In `repository-sessions.go`, replace the query in `ListSetsForExerciseSince` (currently at lines 376–385):
```go
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT es.workout_date, es.weight_kg, es.min_reps, es.max_reps,
		       es.completed_reps, es.completed_at, we.warmup_completed_at, es.signal
		FROM exercise_sets es
		LEFT JOIN workout_exercise we ON we.workout_user_id = es.workout_user_id
		                              AND we.workout_date = es.workout_date
		                              AND we.exercise_id = es.exercise_id
		WHERE es.workout_user_id = ? AND es.exercise_id = ? AND es.workout_date >= ?
		ORDER BY es.workout_date DESC, es.set_number`,
		userID, exerciseID, sinceDateStr)
```

- [ ] **Step 4: Update ListSetsForExerciseSince scan to include signalStr**

In the scan variables and call (currently at lines 400–407):
```go
		var (
			workoutDateStr       string
			set                  Set
			completedAtStr       sql.NullString
			warmupCompletedAtStr sql.NullString
			signalStr            sql.NullString
		)
		if err = rows.Scan(&workoutDateStr, &set.WeightKg, &set.MinReps, &set.MaxReps,
			&set.CompletedReps, &completedAtStr, &warmupCompletedAtStr, &signalStr); err != nil {
			return nil, fmt.Errorf("scan exercise set row: %w", err)
		}

		if err = r.parseCompletedAtTimestamp(completedAtStr, &set); err != nil {
			return nil, err
		}

		if signalStr.Valid {
			s := Signal(signalStr.String)
			set.Signal = &s
		}
```

- [ ] **Step 5: Update saveExerciseSets INSERT to include signal**

In `repository-sessions.go`, replace the INSERT inside `saveExerciseSets` (currently at lines 473–479):
```go
			var signalValue any
			if set.Signal != nil {
				signalValue = string(*set.Signal)
			}

			_, err := tx.ExecContext(ctx, `
				INSERT INTO exercise_sets (
					workout_user_id, workout_date, exercise_id, set_number,
					weight_kg, min_reps, max_reps, completed_reps, completed_at, signal
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				userID, dateStr, exerciseSet.ExerciseID, i+1,
				set.WeightKg, set.MinReps, set.MaxReps, set.CompletedReps, completedAtStr, signalValue)

			if err != nil {
				return fmt.Errorf("insert exercise set: %w", err)
			}
```

- [ ] **Step 6: Run tests**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/workout/repository-sessions.go
git commit -m "feat: wire signal through exercise_sets SQL queries"
```

---

### Task 5: Repository + Service — CountCompleted and generateWorkout periodization

**Files:**
- Modify: `internal/workout/repository.go`
- Modify: `internal/workout/repository-sessions.go`
- Modify: `internal/workout/service.go`
- Modify: `internal/workout/service_test.go`

- [ ] **Step 1: Write failing test**

In `service_test.go`, add a new test after the existing test:

```go
func Test_GenerateWorkout_PeriodizationTypeCycles(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	if err = tryInsertMuscleGroup(ctx, t, db, "Chest"); err != nil {
		t.Fatalf("insert muscle group: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_preferences (user_id, monday_minutes) VALUES (?, ?)", userID, 60)
	if err != nil {
		t.Fatalf("insert preferences: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown) VALUES (?, ?, ?)",
		"Bench Press", "upper", "desc")
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}

	svc := workout.NewService(db, logger, "")

	// Session 0 completed: expect Strength.
	monday := nextMonday(t)
	if err = svc.StartSession(ctx, monday); err != nil {
		t.Fatalf("StartSession 1: %v", err)
	}
	sess, err := svc.GetSession(ctx, monday)
	if err != nil {
		t.Fatalf("GetSession 1: %v", err)
	}
	if sess.PeriodizationType != workout.PeriodizationStrength {
		t.Errorf("session 1: want Strength, got %q", sess.PeriodizationType)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"UPDATE workout_sessions SET completed_at = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE user_id = ? AND workout_date = ?",
		userID, monday.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("complete session 1: %v", err)
	}

	// Session 1 completed: expect Hypertrophy.
	tuesday := monday.AddDate(0, 0, 1)
	if err = svc.StartSession(ctx, tuesday); err != nil {
		t.Fatalf("StartSession 2: %v", err)
	}
	sess, err = svc.GetSession(ctx, tuesday)
	if err != nil {
		t.Fatalf("GetSession 2: %v", err)
	}
	if sess.PeriodizationType != workout.PeriodizationHypertrophy {
		t.Errorf("session 2: want Hypertrophy, got %q", sess.PeriodizationType)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"UPDATE workout_sessions SET completed_at = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE user_id = ? AND workout_date = ?",
		userID, tuesday.Format("2006-01-02"))
	if err != nil {
		t.Fatalf("complete session 2: %v", err)
	}

	// Session 2 completed: wraps back to Strength.
	wednesday := monday.AddDate(0, 0, 2)
	if err = svc.StartSession(ctx, wednesday); err != nil {
		t.Fatalf("StartSession 3: %v", err)
	}
	sess, err = svc.GetSession(ctx, wednesday)
	if err != nil {
		t.Fatalf("GetSession 3: %v", err)
	}
	if sess.PeriodizationType != workout.PeriodizationStrength {
		t.Errorf("session 3: want Strength, got %q", sess.PeriodizationType)
	}
}

func nextMonday(t *testing.T) time.Time {
	t.Helper()
	now := time.Now()
	daysUntilMonday := (int(time.Monday) - int(now.Weekday()) + 7) % 7
	if daysUntilMonday == 0 {
		daysUntilMonday = 7
	}
	return now.AddDate(0, 0, daysUntilMonday).Truncate(24 * time.Hour)
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test -v ./internal/workout/... -run Test_GenerateWorkout_PeriodizationTypeCycles
```

Expected: FAIL — `PeriodizationType` is always "" because it's not set yet.

- [ ] **Step 3: Add CountCompleted to sessionRepository interface**

In `repository.go`, add to the `sessionRepository` interface after `ListSetsForExerciseSince`:
```go
	// CountCompleted returns the count of sessions with completed_at IS NOT NULL.
	CountCompleted(ctx context.Context) (int, error)
```

- [ ] **Step 4: Implement CountCompleted**

Add to `repository-sessions.go`:
```go
// CountCompleted returns the number of completed sessions for the authenticated user.
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
```

- [ ] **Step 5: Update generateWorkout in service.go to assign PeriodizationType**

In `service.go`, add the periodization assignment at the end of `generateWorkout`, just before the final `return session, nil` (currently line 84):

```go
	count, err := s.repo.sessions.CountCompleted(ctx)
	if err != nil {
		return sessionAggregate{}, fmt.Errorf("count completed sessions: %w", err)
	}
	if count%2 == 0 {
		session.PeriodizationType = PeriodizationStrength
	} else {
		session.PeriodizationType = PeriodizationHypertrophy
	}

	return session, nil
```

Remove the existing bare `return session, nil` that was there.

- [ ] **Step 6: Run test to verify it passes**

```bash
go test -v ./internal/workout/... -run Test_GenerateWorkout_PeriodizationTypeCycles
```

Expected: PASS.

- [ ] **Step 7: Run full test suite**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/workout/repository.go internal/workout/repository-sessions.go \
        internal/workout/service.go internal/workout/service_test.go
git commit -m "feat: assign periodization_type via CountCompleted-based DUP rotation"
```

---

### Task 6: Service — GetStartingWeight

**Files:**
- Modify: `internal/workout/service.go`
- Modify: `internal/workout/service_test.go`

- [ ] **Step 1: Write failing test**

Add to `service_test.go`:

```go
func Test_GetStartingWeight(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("sw-user"), "SW User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown) VALUES (?, ?, ?)",
		"Squat", "lower", "desc")
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Squat'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	svc := workout.NewService(db, logger, "")

	// No history: expect 0.
	got, err := svc.GetStartingWeight(ctx, exerciseID)
	if err != nil {
		t.Fatalf("GetStartingWeight no history: %v", err)
	}
	if got != 0 {
		t.Errorf("no history: want 0, got %v", got)
	}

	// Insert a completed session with set 1 weight = 60kg.
	dateStr := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, completed_at) VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))",
		userID, dateStr)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number,
		 weight_kg, min_reps, max_reps, completed_reps)
		 VALUES (?, ?, ?, 1, 60.0, 5, 5, 5)`,
		userID, dateStr, exerciseID)
	if err != nil {
		t.Fatalf("insert set: %v", err)
	}

	got, err = svc.GetStartingWeight(ctx, exerciseID)
	if err != nil {
		t.Fatalf("GetStartingWeight with history: %v", err)
	}
	if got != 60.0 {
		t.Errorf("with history: want 60.0, got %v", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test -v ./internal/workout/... -run Test_GetStartingWeight
```

Expected: FAIL — `GetStartingWeight` undefined.

- [ ] **Step 3: Implement GetStartingWeight in service.go**

Add after `GetExerciseSetsForExerciseSince` in `service.go`:

```go
// GetStartingWeight returns the weight from the first set of the most recent completed
// session for the given exercise. Returns 0 if no completed history exists.
func (s *Service) GetStartingWeight(ctx context.Context, exerciseID int) (float64, error) {
	since := time.Now().AddDate(0, -3, 0)
	aggs, err := s.repo.sessions.ListSetsForExerciseSince(ctx, exerciseID, since)
	if err != nil {
		return 0, fmt.Errorf("list sets for exercise: %w", err)
	}
	// aggs is ordered DESC by date; first element is most recent session.
	for _, agg := range aggs {
		if len(agg.Sets) > 0 && agg.Sets[0].WeightKg != nil && agg.Sets[0].CompletedReps != nil {
			return *agg.Sets[0].WeightKg, nil
		}
	}
	return 0, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test -v ./internal/workout/... -run Test_GetStartingWeight
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workout/service.go internal/workout/service_test.go
git commit -m "feat: add GetStartingWeight service method"
```

---

### Task 7: Service — RecordSetCompletion

**Files:**
- Modify: `internal/workout/service.go`
- Modify: `internal/workout/service_test.go`

- [ ] **Step 1: Write failing test**

Add to `service_test.go`:

```go
func Test_RecordSetCompletion(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("rsc-user"), "RSC User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown) VALUES (?, ?, ?)",
		"Deadlift", "lower", "desc")
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'Deadlift'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_sessions (user_id, workout_date, started_at) VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))",
		userID, today)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number,
		 weight_kg, min_reps, max_reps)
		 VALUES (?, ?, ?, 1, 100.0, 5, 5)`,
		userID, today, exerciseID)
	if err != nil {
		t.Fatalf("insert set: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?)",
		userID, today, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}

	svc := workout.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", today)

	if err = svc.RecordSetCompletion(ctx, date, exerciseID, 0, workout.SignalOnTarget, 102.5, 5); err != nil {
		t.Fatalf("RecordSetCompletion: %v", err)
	}

	sess, err := svc.GetSession(ctx, date)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	var es *workout.ExerciseSet
	for i := range sess.ExerciseSets {
		if sess.ExerciseSets[i].Exercise.ID == exerciseID {
			es = &sess.ExerciseSets[i]
			break
		}
	}
	if es == nil {
		t.Fatal("exercise not found in session")
	}

	set := es.Sets[0]
	if set.Signal == nil || *set.Signal != workout.SignalOnTarget {
		t.Errorf("signal: want on_target, got %v", set.Signal)
	}
	if set.WeightKg == nil || *set.WeightKg != 102.5 {
		t.Errorf("weight: want 102.5, got %v", set.WeightKg)
	}
	if set.CompletedReps == nil || *set.CompletedReps != 5 {
		t.Errorf("reps: want 5, got %v", set.CompletedReps)
	}
	if set.CompletedAt == nil {
		t.Error("completed_at: want non-nil")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test -v ./internal/workout/... -run Test_RecordSetCompletion
```

Expected: FAIL — `RecordSetCompletion` undefined.

- [ ] **Step 3: Implement RecordSetCompletion in service.go**

Add to `service.go`:

```go
// RecordSetCompletion atomically persists the signal, weight, reps, and timestamp for a set.
func (s *Service) RecordSetCompletion(
	ctx context.Context,
	date time.Time,
	exerciseID int,
	setIndex int,
	signal Signal,
	weightKg float64,
	reps int,
) error {
	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		for i := range sess.ExerciseSets {
			if sess.ExerciseSets[i].ExerciseID == exerciseID {
				if setIndex >= len(sess.ExerciseSets[i].Sets) {
					return false, fmt.Errorf("set index %d out of bounds", setIndex)
				}
				now := time.Now().UTC()
				sess.ExerciseSets[i].Sets[setIndex].Signal = &signal
				sess.ExerciseSets[i].Sets[setIndex].WeightKg = &weightKg
				sess.ExerciseSets[i].Sets[setIndex].CompletedReps = &reps
				sess.ExerciseSets[i].Sets[setIndex].CompletedAt = &now
				return true, nil
			}
		}
		return false, errors.New("exercise not found")
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test -v ./internal/workout/... -run Test_RecordSetCompletion
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/workout/service.go internal/workout/service_test.go
git commit -m "feat: add RecordSetCompletion service method"
```

---

### Task 8: Service — BuildProgression

**Files:**
- Modify: `internal/workout/service.go`
- Modify: `internal/workout/service_test.go`

- [ ] **Step 1: Write failing test**

Add to `service_test.go`:

```go
func Test_BuildProgression(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	defer func() { _ = db.Close() }()

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("bp-user"), "BP User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO exercises (name, category, description_markdown) VALUES (?, ?, ?)",
		"OHP", "upper", "desc")
	if err != nil {
		t.Fatalf("insert exercise: %v", err)
	}
	var exerciseID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM exercises WHERE name = 'OHP'").Scan(&exerciseID)
	if err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	// Hypertrophy session (1 completed before this one).
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at, periodization_type)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'), 'hypertrophy')`,
		userID, today)
	if err != nil {
		t.Fatalf("insert session: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps)
		 VALUES (?, ?, ?, 1, 40.0, 8, 8), (?, ?, ?, 2, 40.0, 8, 8), (?, ?, ?, 3, 40.0, 8, 8)`,
		userID, today, exerciseID, userID, today, exerciseID, userID, today, exerciseID)
	if err != nil {
		t.Fatalf("insert sets: %v", err)
	}
	_, err = db.ReadWrite.ExecContext(ctx,
		"INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES (?, ?, ?)",
		userID, today, exerciseID)
	if err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}

	svc := workout.NewService(db, logger, "")
	date, _ := time.Parse("2006-01-02", today)

	// No history: starting weight 0, target 8 reps (hypertrophy).
	prog, err := svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression: %v", err)
	}
	target := prog.CurrentSet()
	if target.WeightKg != 0 {
		t.Errorf("first set weight: want 0, got %v", target.WeightKg)
	}
	if target.TargetReps != 8 {
		t.Errorf("first set reps: want 8, got %v", target.TargetReps)
	}

	// Record set 0 as TooLight at 0kg.
	if err = svc.RecordSetCompletion(ctx, date, exerciseID, 0, workout.SignalTooLight, 0, 8); err != nil {
		t.Fatalf("RecordSetCompletion: %v", err)
	}

	// Rebuild: next set should be 0 + 2.5 = 2.5 kg.
	prog, err = svc.BuildProgression(ctx, date, exerciseID)
	if err != nil {
		t.Fatalf("BuildProgression after set 1: %v", err)
	}
	target = prog.CurrentSet()
	if target.WeightKg != 2.5 {
		t.Errorf("second set weight: want 2.5, got %v", target.WeightKg)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test -v ./internal/workout/... -run Test_BuildProgression
```

Expected: FAIL — `BuildProgression` undefined.

- [ ] **Step 3: Implement BuildProgression in service.go**

Add import at top of `service.go` (in the import block):
```go
"github.com/myrjola/petrapp/internal/exerciseprogression"
```

Add method to `service.go`:

```go
// BuildProgression constructs an exerciseprogression.Progression for the given exercise
// in the given session, ready to call CurrentSet() for the next set recommendation.
func (s *Service) BuildProgression(
	ctx context.Context,
	date time.Time,
	exerciseID int,
) (*exerciseprogression.Progression, error) {
	sess, err := s.repo.sessions.Get(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	startingWeight, err := s.GetStartingWeight(ctx, exerciseID)
	if err != nil {
		return nil, fmt.Errorf("get starting weight: %w", err)
	}

	var epType exerciseprogression.PeriodizationType
	switch sess.PeriodizationType {
	case PeriodizationHypertrophy:
		epType = exerciseprogression.Hypertrophy
	default:
		epType = exerciseprogression.Strength
	}

	config := exerciseprogression.Config{
		Type:           epType,
		StartingWeight: startingWeight,
	}

	var completed []exerciseprogression.SetResult
	for _, es := range sess.ExerciseSets {
		if es.ExerciseID != exerciseID {
			continue
		}
		for _, set := range es.Sets {
			if set.CompletedReps == nil || set.Signal == nil {
				continue
			}
			var sig exerciseprogression.Signal
			switch *set.Signal {
			case SignalTooHeavy:
				sig = exerciseprogression.SignalTooHeavy
			case SignalOnTarget:
				sig = exerciseprogression.SignalOnTarget
			case SignalTooLight:
				sig = exerciseprogression.SignalTooLight
			}
			var kg float64
			if set.WeightKg != nil {
				kg = *set.WeightKg
			}
			completed = append(completed, exerciseprogression.SetResult{
				ActualReps: *set.CompletedReps,
				Signal:     sig,
				WeightKg:   kg,
			})
		}
		break
	}

	return exerciseprogression.NewFromHistory(config, completed), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test -v ./internal/workout/... -run Test_BuildProgression
```

Expected: PASS.

- [ ] **Step 5: Run full suite**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/workout/service.go internal/workout/service_test.go
git commit -m "feat: add BuildProgression service method"
```

---

### Task 9: Handler — update exerciseSetGET with CurrentSetTarget

**Files:**
- Modify: `cmd/web/handler-exerciseset.go`

- [ ] **Step 1: Add exerciseprogression import and CurrentSetTarget to template data struct**

In `handler-exerciseset.go`, update the import block to add:
```go
"github.com/myrjola/petrapp/internal/exerciseprogression"
```

Replace the `exerciseSetTemplateData` struct (lines 20–29):
```go
type exerciseSetTemplateData struct {
	BaseTemplateData
	Date                 time.Time
	ExerciseSet          workout.ExerciseSet
	SetsDisplay          []setDisplay
	FirstIncompleteIndex int
	EditingIndex         int
	IsEditing            bool
	LastCompletedAt      *time.Time
	CurrentSetTarget     exerciseprogression.SetTarget
}
```

- [ ] **Step 2: Call BuildProgression in exerciseSetGET**

In `exerciseSetGET`, after finding `exerciseSet` and before building `data`, add:

```go
	var currentSetTarget exerciseprogression.SetTarget
	if exerciseSet.Exercise.ExerciseType == workout.ExerciseTypeWeighted {
		progression, progressionErr := app.workoutService.BuildProgression(r.Context(), date, exerciseID)
		if progressionErr != nil {
			app.serverError(w, r, progressionErr)
			return
		}
		currentSetTarget = progression.CurrentSet()
	}
```

Then update the `data` literal to include it:
```go
	data := exerciseSetTemplateData{
		BaseTemplateData:     newBaseTemplateData(r),
		Date:                 date,
		ExerciseSet:          exerciseSet,
		SetsDisplay:          prepareSetsDisplay(exerciseSet.Sets),
		FirstIncompleteIndex: getFirstIncompleteIndex(exerciseSet.Sets),
		EditingIndex:         editingIndex,
		IsEditing:            isEditing,
		LastCompletedAt:      getLastCompletedAt(exerciseSet.Sets),
		CurrentSetTarget:     currentSetTarget,
	}
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./...
```

Expected: compiles without errors.

- [ ] **Step 4: Commit**

```bash
git add cmd/web/handler-exerciseset.go
git commit -m "feat: add CurrentSetTarget to exerciseSetTemplateData via BuildProgression"
```

---

### Task 10: Handler — update exerciseSetUpdatePOST for signal-based submission

**Files:**
- Modify: `cmd/web/handler-exerciseset.go`

- [ ] **Step 1: Replace exerciseSetUpdatePOST body**

Replace the entire `exerciseSetUpdatePOST` function (lines 202–261) with:

```go
func (app *application) exerciseSetUpdatePOST(w http.ResponseWriter, r *http.Request) {
	date, exerciseID, setIndex, dateStr, err := app.parseExerciseSetURLParams(r)
	if err != nil {
		app.notFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	session, err := app.workoutService.GetSession(r.Context(), date)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	exercise, found := app.findExerciseInSession(&session, exerciseID)
	if !found {
		app.notFound(w, r)
		return
	}

	if exercise.ExerciseType == workout.ExerciseTypeWeighted {
		weightStr := strings.Replace(r.PostForm.Get("weight"), ",", ".", 1)
		weight, parseErr := strconv.ParseFloat(weightStr, 64)
		if parseErr != nil {
			app.serverError(w, r, fmt.Errorf("parse weight: %w", parseErr))
			return
		}

		signal := workout.Signal(r.PostForm.Get("signal"))

		var reps int
		if signal == workout.SignalTooHeavy {
			reps, err = strconv.Atoi(r.PostForm.Get("reps"))
			if err != nil {
				app.serverError(w, r, fmt.Errorf("parse reps: %w", err))
				return
			}
		} else {
			reps, err = strconv.Atoi(r.PostForm.Get("target_reps"))
			if err != nil {
				app.serverError(w, r, fmt.Errorf("parse target_reps: %w", err))
				return
			}
		}

		if err = app.workoutService.RecordSetCompletion(r.Context(), date, exerciseID, setIndex, signal, weight, reps); err != nil {
			app.serverError(w, r, fmt.Errorf("record set completion: %w", err))
			return
		}

		app.logger.LogAttrs(r.Context(), slog.LevelInfo, "recorded set completion",
			slog.String("date", dateStr),
			slog.Int("exercise_id", exerciseID),
			slog.Int("set_index", setIndex),
			slog.String("signal", string(signal)),
			slog.Float64("weight", weight),
			slog.Int("reps", reps))
	} else {
		_, reps, parseErr := app.parseWeightAndReps(r, exercise)
		if parseErr != nil {
			app.serverError(w, r, parseErr)
			return
		}
		if err = app.workoutService.UpdateCompletedReps(r.Context(), date, exerciseID, setIndex, reps); err != nil {
			app.serverError(w, r, fmt.Errorf("update completed reps: %w", err))
			return
		}
	}

	redirect(w, r, fmt.Sprintf("/workouts/%s/exercises/%d", date.Format("2006-01-02"), exerciseID))
}
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./...
```

Expected: compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/web/handler-exerciseset.go
git commit -m "feat: update exerciseSetUpdatePOST to handle signal-based form submission"
```

---

### Task 11: Template — signal-first UI and update handler test

**Files:**
- Modify: `ui/templates/pages/exerciseset/exerciseset.gohtml`
- Modify: `cmd/web/handler-exerciseset_test.go`

- [ ] **Step 1: Write failing test (verify new form fields)**

In `handler-exerciseset_test.go`, replace the set-completion section of `Test_application_exerciseSet` (lines 133–166). The new test finds the signal form and submits with `signal=on_target`:

Replace lines 133–166 with:
```go
	// Find and complete the warmup first (unchanged).
	// The active set for weighted exercises now shows signal buttons, not "Done!".
	// Submit with signal=on_target to record the set.
	setForm := doc.Find("form.signal-form").First()
	if setForm.Length() == 0 {
		t.Fatalf("Expected to find signal-form for active set")
	}

	setAction, exists := setForm.Attr("action")
	if !exists {
		t.Fatalf("Signal form has no action attribute")
	}

	formData = map[string]string{
		"weight":      "20.5",
		"signal":      "on_target",
		"target_reps": "5", // Strength = 5 reps (0 completed sessions → type=strength)
	}

	if doc, err = client.SubmitForm(ctx, doc, setAction, formData); err != nil {
		t.Fatalf("Failed to submit signal form: %v", err)
	}

	if doc.Find("h1").Length() == 0 {
		t.Error("Expected to find heading on exercise set page after set completion")
	}

	if doc.Find(".exercise-set.completed").Length() == 0 {
		t.Error("Expected to find a completed set")
	}
```

Also replace the edit section (lines 168–266) — the edit form now also uses signal buttons:
```go
	// Test editing a completed set.
	if doc, err = client.GetDoc(ctx, "/workouts/"+today); err != nil {
		t.Fatalf("Failed to get workout page for edit test: %v", err)
	}

	doc.Find("a.exercise").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			var href string
			href, exists = s.Attr("href")
			if exists {
				exerciseID = href[len("/workouts/"+today+"/exercises/"):]
			}
		}
	})

	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID); err != nil {
		t.Fatalf("Failed to get exercise set page for edit: %v", err)
	}

	editLink := doc.Find(".exercise-set.completed .edit-button").First()
	if editLink.Length() == 0 {
		t.Fatalf("No edit button found for completed set")
	}

	href, exists := editLink.Attr("href")
	if !exists {
		t.Fatalf("Edit link has no href")
	}

	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID+href); err != nil {
		t.Fatalf("Failed to load edit page: %v", err)
	}

	editForm := doc.Find("form.signal-form").First()
	if editForm.Length() == 0 {
		t.Fatalf("Edit signal-form not found")
	}

	editAction, exists := editForm.Attr("action")
	if !exists {
		t.Fatalf("Edit form has no action attribute")
	}

	editFormData := map[string]string{
		"weight":      "22.5",
		"signal":      "on_target",
		"target_reps": "5",
	}

	if doc, err = client.SubmitForm(ctx, doc, editAction, editFormData); err != nil {
		t.Fatalf("Failed to submit edit form: %v", err)
	}

	if doc.Find("h1").Length() == 0 {
		t.Error("Expected to find heading after edit")
	}

	if doc.Find(".exercise-set.completed").Length() == 0 {
		t.Error("Expected set to still be completed after edit")
	}

	setWeight := doc.Find(".exercise-set.completed .weight").First().Text()
	if setWeight == "" {
		t.Error("Expected to find weight in completed set after edit")
	}
```

- [ ] **Step 2: Run to verify it fails**

```bash
go test -v ./cmd/web/... -run Test_application_exerciseSet$
```

Expected: FAIL — `signal-form` class not found because template not yet updated.

- [ ] **Step 3: Update the template — active set form**

In `exerciseset.gohtml`, replace the entire form block inside the `.exercise-set` loop (lines 463–511) with:

```gohtml
                    {{ if and $.ExerciseSet.WarmupCompletedAt (or (and (not $set.CompletedReps) (eq $.FirstIncompleteIndex $index)) (and $.IsEditing (eq $index $.EditingIndex))) }}
                        {{ if eq $.ExerciseSet.Exercise.ExerciseType "weighted" }}
                            <form method="post"
                                  action="/workouts/{{ $.Date.Format "2006-01-02" }}/exercises/{{ $.ExerciseSet.Exercise.ID }}/sets/{{ $index }}/update"
                                  id="form-{{ $index }}"
                                  class="set-form signal-form"
                                  aria-label="Complete set {{ $index }}">
                                <input type="hidden" id="signal-{{ $index }}" name="signal" value="">
                                <input type="hidden" name="target_reps" value="{{ $.CurrentSetTarget.TargetReps }}">
                                <div class="form-inputs">
                                    <div class="input-field">
                                        <label for="weight-{{ $index }}">Weight (kg)</label>
                                        <input
                                                id="weight-{{ $index }}"
                                                inputmode="decimal"
                                                pattern="[0-9,\.]*"
                                                name="weight"
                                                value="{{ formatFloat $.CurrentSetTarget.WeightKg }}"
                                                step="0.5"
                                                required
                                                aria-describedby="weight-help-{{ $index }}"
                                        >
                                        <div id="weight-help-{{ $index }}" class="sr-only">Enter weight in kilograms</div>
                                    </div>
                                </div>
                                <p class="signal-question">Did you reach {{ $.CurrentSetTarget.TargetReps }} reps?</p>
                                <div class="signal-buttons">
                                    <button type="button"
                                            class="signal-btn too-heavy-btn"
                                            aria-label="No, I failed to reach target reps"
                                            onclick="handleSignal({{ $index }}, 'too_heavy')">No
                                    </button>
                                    <button type="button"
                                            class="signal-btn on-target-btn"
                                            aria-label="Barely reached target reps"
                                            onclick="handleSignal({{ $index }}, 'on_target')">Barely
                                    </button>
                                    <button type="button"
                                            class="signal-btn too-light-btn"
                                            aria-label="Could have done more reps"
                                            onclick="handleSignal({{ $index }}, 'too_light')">Could do more
                                    </button>
                                </div>
                                <div id="reps-section-{{ $index }}" hidden class="reps-section">
                                    <div class="input-field">
                                        <label for="reps-{{ $index }}">Actual reps</label>
                                        <input
                                                id="reps-{{ $index }}"
                                                inputmode="numeric"
                                                pattern="[0-9]*"
                                                name="reps"
                                                class="reps-input"
                                                aria-describedby="reps-help-{{ $index }}"
                                        >
                                        <div id="reps-help-{{ $index }}" class="sr-only">Enter actual repetitions completed</div>
                                    </div>
                                    <button type="submit" class="submit-button" aria-label="Submit set {{ $index }}">Submit</button>
                                </div>
                            </form>
                        {{ else }}
                            <form method="post"
                                  action="/workouts/{{ $.Date.Format "2006-01-02" }}/exercises/{{ $.ExerciseSet.Exercise.ID }}/sets/{{ $index }}/update"
                                  id="form-{{ $index }}"
                                  class="set-form"
                                  aria-label="Complete set {{ $index }}">
                                <div class="form-inputs">
                                    <div class="input-field">
                                        <label for="reps-{{ $index }}">Reps</label>
                                        <input
                                                id="reps-{{ $index }}"
                                                inputmode="numeric"
                                                pattern="[0-9]*"
                                                name="reps"
                                                placeholder="{{ $setDisplay.RepStr }}"
                                                {{ if $set.CompletedReps }}value="{{ $set.CompletedReps }}"{{ end }}
                                                required
                                                {{ if $set.CompletedReps }}disabled{{ end }}
                                                class="reps-input"
                                        >
                                    </div>
                                </div>
                                <button type="submit" class="submit-button" aria-label="Complete set {{ $index }}">Done!</button>
                            </form>
                        {{ end }}
                    {{ end }}
```

- [ ] **Step 4: Update the active set `.set-info` to show progression recommendation**

In the `.set-info` div (lines 449–461), for the active weighted set show the progression recommendation weight and target reps instead of the stored values. Replace that block with:

```gohtml
                    <div class="set-info">
                        {{ if eq $.ExerciseSet.Exercise.ExerciseType "weighted" }}
                            {{ if $set.CompletedReps }}
                                <span class="weight" aria-label="Weight">{{ formatFloat $set.WeightKg }} kg</span>
                                <span class="reps" aria-label="Completed reps">{{ $set.CompletedReps }} reps</span>
                                {{ if $set.Signal }}
                                    <span class="signal-badge" aria-label="Signal">{{ $set.Signal }}</span>
                                {{ end }}
                                <a href="?edit={{ $index }}" class="edit-button" aria-label="Edit set {{ $index }}">Edit</a>
                            {{ else if and $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
                                <span class="weight" aria-label="Recommended weight">{{ formatFloat $.CurrentSetTarget.WeightKg }} kg</span>
                                <span class="reps" aria-label="Target reps">{{ $.CurrentSetTarget.TargetReps }} reps</span>
                            {{ else }}
                                <span class="reps" aria-label="Target reps">{{ $.CurrentSetTarget.TargetReps }} reps</span>
                            {{ end }}
                        {{ else }}
                            {{ if $set.CompletedReps }}
                                <span class="reps" aria-label="Completed reps">{{ $set.CompletedReps }} reps</span>
                                <a href="?edit={{ $index }}" class="edit-button" aria-label="Edit set {{ $index }}">Edit</a>
                            {{ else }}
                                <span class="reps" aria-label="Target reps">{{ $setDisplay.RepStr }} reps</span>
                            {{ end }}
                        {{ end }}
                    </div>
```

- [ ] **Step 5: Add signal-form JS and CSS to the template's `<script>` block**

In the `<script {{ nonce }}>` block, add the `handleSignal` function before `initializeTimer()`:

```javascript
          function handleSignal(index, value) {
            document.getElementById('signal-' + index).value = value
            if (value === 'too_heavy') {
              var section = document.getElementById('reps-section-' + index)
              section.removeAttribute('hidden')
              var repsInput = document.getElementById('reps-' + index)
              repsInput.required = true
              setTimeout(function () { repsInput.focus() }, 50)
              return
            }
            document.getElementById('form-' + index).submit()
          }
```

Add signal-form CSS inside the `@scope` block (add before the closing `}`):

```css
                .signal-question {
                    font-size: var(--font-size-1);
                    font-weight: var(--font-weight-6);
                    color: var(--color-text-primary);
                    margin: var(--size-2) 0;
                }

                .signal-buttons {
                    display: flex;
                    gap: var(--size-2);
                    flex-wrap: wrap;
                }

                .signal-btn {
                    padding: var(--size-3) var(--size-4);
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-6);
                    font-size: var(--font-size-1);
                    cursor: pointer;
                    border: 2px solid transparent;
                    transition: all 0.2s ease;

                    &.too-heavy-btn {
                        background: var(--color-error-bg, var(--red-1));
                        color: var(--red-8);
                        border-color: var(--red-3);
                        &:hover { background: var(--red-2); }
                    }

                    &.on-target-btn {
                        background: var(--color-success-bg);
                        color: var(--color-success);
                        border-color: var(--lime-4);
                        &:hover { background: var(--lime-2); }
                    }

                    &.too-light-btn {
                        background: var(--color-info-bg);
                        color: var(--color-info);
                        border-color: var(--sky-3);
                        &:hover { background: var(--sky-2); }
                    }
                }

                .reps-section {
                    display: flex;
                    gap: var(--size-3);
                    align-items: end;
                    margin-top: var(--size-3);
                }

                .signal-badge {
                    font-size: var(--font-size-0);
                    color: var(--color-text-secondary);
                    background: var(--color-surface-elevated);
                    padding: var(--size-1) var(--size-2);
                    border-radius: var(--radius-2);
                }
```

- [ ] **Step 6: Run test to verify it passes**

```bash
go test -v ./cmd/web/... -run Test_application_exerciseSet$
```

Expected: PASS.

- [ ] **Step 7: Run full test suite**

```bash
make test
```

Expected: all tests pass.

- [ ] **Step 8: Run linter**

```bash
make lint-fix
```

Fix any issues reported.

- [ ] **Step 9: Commit**

```bash
git add ui/templates/pages/exerciseset/exerciseset.gohtml cmd/web/handler-exerciseset_test.go
git commit -m "feat: signal-first UI for weighted exercise sets with RIR auto-regulation"
```

---

## Self-Review

**Spec coverage check:**

| Spec requirement | Task |
|---|---|
| `periodization_type` column on `workout_sessions` | Task 1 |
| `signal` column on `exercise_sets` | Task 1 |
| `PeriodizationType` / `Signal` domain types | Task 2 |
| `Signal *Signal` on `Set`, `PeriodizationType` on `Session` | Task 2 |
| Repository wires `periodization_type` through SQL | Task 3 |
| Repository wires `signal` through SQL | Task 4 |
| `CountCompleted` for DUP rotation | Task 5 |
| `generateWorkout` assigns `periodization_type` | Task 5 |
| `GetStartingWeight` from most recent first set | Task 6 |
| `RecordSetCompletion` atomically persists signal+weight+reps | Task 7 |
| `BuildProgression` reconstructs `Progression` via `NewFromHistory` | Task 8 |
| Handler `exerciseSetGET` calls `BuildProgression`, adds `CurrentSetTarget` | Task 9 |
| Handler `exerciseSetUpdatePOST` parses `signal`, routes to `RecordSetCompletion` | Task 10 |
| Template: signal-first UI with 3 buttons, TooHeavy 2-step reveal via JS | Task 11 |
| Template: bodyweight exercises unchanged | Task 11 |
| Existing handler test updated for new form | Task 11 |

All spec requirements are covered. No gaps found.
