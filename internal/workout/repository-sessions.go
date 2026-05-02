package workout

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// queryer is satisfied by both *sql.DB and *sql.Tx, so read helpers can run
// either standalone or inside an open transaction.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// sqliteSessionRepository implements sessionRepository.
type sqliteSessionRepository struct {
	baseRepository
}

// newSQLiteSessionRepository creates a new SQLite session repository.
func newSQLiteSessionRepository(
	db *sqlite.Database,
) *sqliteSessionRepository {
	return &sqliteSessionRepository{
		baseRepository: newBaseRepository(db),
	}
}

// List retrieves all workout sessions since a given date.
func (r *sqliteSessionRepository) List(ctx context.Context, sinceDate time.Time) (_ []sessionAggregate, err error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	sinceDateStr := formatDate(sinceDate)

	query := `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type
		FROM workout_sessions
		WHERE user_id = ? AND workout_date >= ?
		ORDER BY workout_date DESC`

	rows, err := r.db.ReadOnly.QueryContext(ctx, query, userID, sinceDateStr)
	if err != nil {
		return nil, fmt.Errorf("query workout history: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var sessions []sessionAggregate
	for rows.Next() {
		var (
			workoutDateStr    string
			difficultyRating  sql.NullInt32
			startedAtStr      sql.NullString
			completedAtStr    sql.NullString
			periodizationType PeriodizationType
		)

		if err = rows.Scan(
			&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType,
		); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}

		var session sessionAggregate
		session, err = r.parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType)
		if err != nil {
			return nil, err
		}

		var exerciseSets []exerciseSetAggregate
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

// Get retrieves a workout session for a specific date.
func (r *sqliteSessionRepository) Get(ctx context.Context, date time.Time) (sessionAggregate, error) {
	return r.get(ctx, r.db.ReadOnly, date)
}

// get retrieves a workout session using the supplied queryer, so it can run
// either standalone or inside an open transaction.
func (r *sqliteSessionRepository) get(
	ctx context.Context,
	q queryer,
	date time.Time,
) (sessionAggregate, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := formatDate(date)

	var (
		workoutDateStr    string
		difficultyRating  sql.NullInt32
		startedAtStr      sql.NullString
		completedAtStr    sql.NullString
		periodizationType PeriodizationType
	)

	err := q.QueryRowContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type
		FROM workout_sessions
		WHERE user_id = ? AND workout_date = ?`,
		userID, dateStr).Scan(&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sessionAggregate{}, ErrNotFound
		}
		return sessionAggregate{}, fmt.Errorf("query session: %w", err)
	}

	session, err := r.parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType)
	if err != nil {
		return sessionAggregate{}, err
	}

	exerciseSets, err := r.loadExerciseSets(ctx, q, userID, session.Date)
	if err != nil {
		return sessionAggregate{}, err
	}
	session.ExerciseSets = exerciseSets

	return session, nil
}

// Create adds new workout session.
func (r *sqliteSessionRepository) Create(ctx context.Context, sess sessionAggregate) (err error) {
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	if err = r.insertSession(ctx, tx, sess); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// Update modifies an existing workout session within a single transaction.
// The read happens inside the same BEGIN IMMEDIATE transaction as the write,
// so concurrent updates cannot interleave a read-modify-write race.
func (r *sqliteSessionRepository) Update(
	ctx context.Context,
	date time.Time,
	updateFn func(sess *sessionAggregate) (bool, error),
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

	updated, err := updateFn(&session)
	if err != nil {
		return fmt.Errorf("update function: %w", err)
	}

	if !updated {
		return nil
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

// insertSession writes a session and its sets within an existing transaction.
// The caller is responsible for ensuring no row already exists for
// (user_id, workout_date); use deleteSession first when replacing.
func (r *sqliteSessionRepository) insertSession(
	ctx context.Context,
	tx *sql.Tx,
	sess sessionAggregate,
) error {
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

// deleteSession removes a session within an existing transaction. The
// ON DELETE CASCADE foreign keys clear the related workout_exercise and
// exercise_sets rows.
func (r *sqliteSessionRepository) deleteSession(
	ctx context.Context,
	tx *sql.Tx,
	date time.Time,
) error {
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

// parseSessionRow converts database values to a Session.
func (r *sqliteSessionRepository) parseSessionRow(
	workoutDateStr string,
	difficultyRating sql.NullInt32,
	startedAtStr sql.NullString,
	completedAtStr sql.NullString,
	periodizationType PeriodizationType,
) (sessionAggregate, error) {
	var session sessionAggregate

	// Parse date
	date, err := time.Parse(dateFormat, workoutDateStr)
	if err != nil {
		return sessionAggregate{}, fmt.Errorf("parse workout date: %w", err)
	}
	session.Date = date

	// Parse difficulty rating
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

	session.PeriodizationType = periodizationType

	return session, nil
}

// loadExerciseSetsRow holds one row of the loadExerciseSets join: the
// workout_exercise columns plus the optional exercise_sets columns.
type loadExerciseSetsRow struct {
	weID                 int
	exerciseID           int
	warmupCompletedAtStr sql.NullString
	setNumber            sql.NullInt32
	weightKg             sql.NullFloat64
	minReps              sql.NullInt32
	maxReps              sql.NullInt32
	completedReps        sql.NullInt32
	completedAtStr       sql.NullString
	signalStr            sql.NullString
}

// loadExerciseSets fetches all exercise slots for a session, including ones with
// no sets yet (e.g. just-swapped exercises). The driving table is workout_exercise
// so empty slots still appear; sets are LEFT-JOINed in.
func (r *sqliteSessionRepository) loadExerciseSets(
	ctx context.Context,
	q queryer,
	userID int,
	date time.Time,
) (_ []exerciseSetAggregate, err error) {
	dateStr := formatDate(date)

	rows, err := q.QueryContext(ctx, `
		SELECT we.id, we.exercise_id, we.warmup_completed_at,
		       es.set_number, es.weight_kg, es.min_reps, es.max_reps,
		       es.completed_reps, es.completed_at, es.signal
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

	var exerciseSets []exerciseSetAggregate
	// Nil pointer means "no group started yet" — preferred over an ID=-1 sentinel
	// because it makes the empty case impossible to confuse with a real row.
	var current *exerciseSetAggregate
	flush := func() {
		if current != nil {
			exerciseSets = append(exerciseSets, *current)
		}
	}

	for rows.Next() {
		var row loadExerciseSetsRow
		if err = rows.Scan(&row.weID, &row.exerciseID, &row.warmupCompletedAtStr,
			&row.setNumber, &row.weightKg, &row.minReps, &row.maxReps,
			&row.completedReps, &row.completedAtStr, &row.signalStr); err != nil {
			return nil, fmt.Errorf("scan exercise set: %w", err)
		}

		if current == nil || row.weID != current.ID {
			flush()
			started, startErr := r.startAggregate(row)
			if startErr != nil {
				return nil, startErr
			}
			current = &started
		}

		// LEFT JOIN can yield a workout_exercise row with no sets (set_number IS NULL).
		if !row.setNumber.Valid {
			continue
		}

		set, parseErr := r.buildSet(row)
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

// startAggregate constructs a fresh exerciseSetAggregate when the workout_exercise
// id changes between rows.
func (r *sqliteSessionRepository) startAggregate(row loadExerciseSetsRow) (exerciseSetAggregate, error) {
	warmupCompletedAt, err := r.parseWarmupCompletedAtTimestamp(row.warmupCompletedAtStr)
	if err != nil {
		return exerciseSetAggregate{}, err
	}
	return exerciseSetAggregate{
		ID:                row.weID,
		ExerciseID:        row.exerciseID,
		Sets:              []Set{},
		WarmupCompletedAt: warmupCompletedAt,
	}, nil
}

// buildSet materialises a Set from a row that has set_number populated.
func (r *sqliteSessionRepository) buildSet(row loadExerciseSetsRow) (Set, error) {
	set := Set{ //nolint:exhaustruct // CompletedReps, CompletedAt, Signal are populated below.
		MinReps: int(row.minReps.Int32),
		MaxReps: int(row.maxReps.Int32),
	}
	if row.weightKg.Valid {
		w := row.weightKg.Float64
		set.WeightKg = &w
	}
	if row.completedReps.Valid {
		c := int(row.completedReps.Int32)
		set.CompletedReps = &c
	}
	if err := r.parseCompletedAtTimestamp(row.completedAtStr, &set); err != nil {
		return Set{}, err
	}
	if row.signalStr.Valid {
		s := Signal(row.signalStr.String)
		set.Signal = &s
	}
	return set, nil
}

// parseCompletedAtTimestamp parses the completed_at timestamp and sets it on the set if valid.
func (r *sqliteSessionRepository) parseCompletedAtTimestamp(completedAtStr sql.NullString, set *Set) error {
	if completedAtStr.Valid {
		completedAt, parseErr := parseTimestamp(completedAtStr)
		if parseErr != nil {
			return fmt.Errorf("parse completed_at timestamp: %w", parseErr)
		}
		if !completedAt.IsZero() {
			set.CompletedAt = &completedAt
		}
	}
	return nil
}

// parseWarmupCompletedAtTimestamp parses the warmup_completed_at timestamp.
func (r *sqliteSessionRepository) parseWarmupCompletedAtTimestamp(
	warmupCompletedAtStr sql.NullString,
) (*time.Time, error) {
	if !warmupCompletedAtStr.Valid {
		return nil, nil //nolint:nilnil // Valid case for optional timestamp
	}

	warmupTime, parseErr := parseTimestamp(warmupCompletedAtStr)
	if parseErr != nil {
		return nil, fmt.Errorf("parse warmup_completed_at timestamp: %w", parseErr)
	}

	if warmupTime.IsZero() {
		return nil, nil //nolint:nilnil // Valid case for zero timestamp
	}

	return &warmupTime, nil
}

// ListSetsForExerciseSince retrieves all sets for a specific exercise since a given date.
func (r *sqliteSessionRepository) ListSetsForExerciseSince(
	ctx context.Context,
	exerciseID int,
	sinceDate time.Time,
) (_ []datedExerciseSetAggregate, err error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	sinceDateStr := formatDate(sinceDate)

	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT we.workout_date, es.weight_kg, es.min_reps, es.max_reps,
		       es.completed_reps, es.completed_at, we.warmup_completed_at, es.signal
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

	var result []datedExerciseSetAggregate
	var current datedExerciseSetAggregate
	current.ExerciseID = -1

	for rows.Next() {
		workoutDateStr, set, warmupCompletedAtStr, scanErr := r.scanExerciseSetWithDate(rows)
		if scanErr != nil {
			return nil, scanErr
		}

		date, parseErr := time.Parse(dateFormat, workoutDateStr)
		if parseErr != nil {
			return nil, fmt.Errorf("parse workout date: %w", parseErr)
		}

		// Start a new aggregate when the date changes.
		if !date.Equal(current.Date) {
			if current.ExerciseID != -1 {
				result = append(result, current)
			}

			warmupCompletedAt, parseWarmupErr := r.parseWarmupCompletedAtTimestamp(warmupCompletedAtStr)
			if parseWarmupErr != nil {
				return nil, parseWarmupErr
			}

			// ID stays zero — this aggregate spans history, not a single live slot.
			current = datedExerciseSetAggregate{
				Date: date,
				exerciseSetAggregate: exerciseSetAggregate{ //nolint:exhaustruct // ID intentionally unset.
					ExerciseID:        exerciseID,
					Sets:              []Set{},
					WarmupCompletedAt: warmupCompletedAt,
				},
			}
		}

		current.Sets = append(current.Sets, set)
	}

	if current.ExerciseID != -1 {
		result = append(result, current)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return result, nil
}

// GetLatestStartingWeightBefore returns the weight of the first completed set
// from the most recent session strictly before beforeDate, along with that
// session's periodization type. Returns a zero-value struct when no completed
// history exists.
func (r *sqliteSessionRepository) GetLatestStartingWeightBefore(
	ctx context.Context,
	exerciseID int,
	beforeDate time.Time,
) (LatestStartingSet, error) {
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
		  AND es.completed_reps IS NOT NULL
		  AND es.weight_kg IS NOT NULL
		ORDER BY we.workout_date DESC, es.set_number ASC
		LIMIT 1`,
		userID, exerciseID, beforeDateStr).Scan(&weightKg, &periodType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LatestStartingSet{WeightKg: 0, PeriodizationType: ""}, nil
		}
		return LatestStartingSet{WeightKg: 0, PeriodizationType: ""}, fmt.Errorf("query latest starting weight: %w", err)
	}

	return LatestStartingSet{
		WeightKg:          weightKg,
		PeriodizationType: PeriodizationType(periodType),
	}, nil
}

// scanExerciseSetWithDate scans one row from the ListSetsForExerciseSince query.
func (r *sqliteSessionRepository) scanExerciseSetWithDate(
	rows *sql.Rows,
) (string, Set, sql.NullString, error) {
	var (
		workoutDateStr       string
		set                  Set
		completedAtStr       sql.NullString
		warmupCompletedAtStr sql.NullString
		signalStr            sql.NullString
	)
	if err := rows.Scan(&workoutDateStr, &set.WeightKg, &set.MinReps, &set.MaxReps,
		&set.CompletedReps, &completedAtStr, &warmupCompletedAtStr, &signalStr); err != nil {
		return "", Set{}, sql.NullString{}, fmt.Errorf("scan exercise set row: %w", err)
	}
	if err := r.parseCompletedAtTimestamp(completedAtStr, &set); err != nil {
		return "", Set{}, sql.NullString{}, err
	}
	if signalStr.Valid {
		s := Signal(signalStr.String)
		set.Signal = &s
	}
	return workoutDateStr, set, warmupCompletedAtStr, nil
}

// DeleteWeek removes all sessions for the 7-day window [monday, monday+6].
// The ON DELETE CASCADE foreign keys clear related workout_exercise and exercise_sets rows.
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

// saveExerciseSets writes the workout_exercise rows and their child exercise_sets
// for a session. Each aggregate gets one workout_exercise row; pre-existing IDs
// are preserved so the URL stays stable across delete-and-reinsert cycles in
// Update, while new aggregates (ID == 0) get an auto-assigned id.
func (r *sqliteSessionRepository) saveExerciseSets(
	ctx context.Context,
	tx *sql.Tx,
	date time.Time,
	exerciseSets []exerciseSetAggregate,
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
		err := tx.QueryRowContext(ctx, `
			INSERT INTO workout_exercise (
				id, workout_user_id, workout_date, exercise_id, warmup_completed_at
			) VALUES (?, ?, ?, ?, ?)
			RETURNING id`,
			idArg, userID, dateStr, exerciseSet.ExerciseID, warmupArg).Scan(&weID)
		if err != nil {
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

			if _, err = tx.ExecContext(ctx, `
				INSERT INTO exercise_sets (
					workout_exercise_id, set_number,
					weight_kg, min_reps, max_reps, completed_reps, completed_at, signal
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				weID, i+1,
				set.WeightKg, set.MinReps, set.MaxReps, set.CompletedReps, completedAtStr, signalValue); err != nil {
				return fmt.Errorf("insert exercise set: %w", err)
			}
		}
	}

	return nil
}
