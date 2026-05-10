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
		if err = rows.Scan(
			&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType,
		); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		var session domain.Session
		session, err = parseSessionRow(
			workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType,
		)
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
