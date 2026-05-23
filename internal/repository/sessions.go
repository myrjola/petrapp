package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteSessionRepository struct {
	baseRepository
}

func newSQLiteSessionRepository(db *sqlite.Database) *sqliteSessionRepository {
	return &sqliteSessionRepository{
		baseRepository: newBaseRepository(db),
	}
}

// List returns the user's sessions on or after sinceDate, newest first, each
// fully hydrated. Exercise slots for the entire range are loaded with a single
// batched query (plus one muscle-group query), so List issues three queries
// total regardless of how many sessions it returns — see loadExerciseSetsSince.
func (r *sqliteSessionRepository) List(ctx context.Context, sinceDate time.Time) ([]domain.Session, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	sessions, err := r.listSessionRows(ctx, userID, sinceDate)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return sessions, nil
	}

	setsByDate, err := r.loadExerciseSetsSince(ctx, r.db.ReadOnly, userID, sinceDate)
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		sessions[i].ExerciseSets = setsByDate[formatDate(sessions[i].Date)]
	}
	return sessions, nil
}

// listSessionRows scans the workout_sessions scalar rows for a user on or
// after sinceDate, newest first. ExerciseSets is left nil — List hydrates it
// in a single batched follow-up query.
func (r *sqliteSessionRepository) listSessionRows(
	ctx context.Context,
	userID int,
	sinceDate time.Time,
) (_ []domain.Session, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type, is_deload
		FROM workout_sessions
		WHERE user_id = ? AND workout_date >= ?
		ORDER BY workout_date DESC`,
		userID, formatDate(sinceDate))
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
			isDeload          bool
		)
		if err = rows.Scan(
			&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType, &isDeload,
		); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		var session domain.Session
		session, err = parseSessionRow(
			workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType, isDeload,
		)
		if err != nil {
			return nil, err
		}
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
		isDeload          bool
	)
	err := q.QueryRowContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at, periodization_type, is_deload
		FROM workout_sessions
		WHERE user_id = ? AND workout_date = ?`,
		userID, dateStr).Scan(
		&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr, &periodizationType, &isDeload)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Session{}, fmt.Errorf("query session: %w", err)
	}

	session, err := parseSessionRow(
		workoutDateStr, difficultyRating, startedAtStr, completedAtStr, periodizationType, isDeload,
	)
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

// CreateBatch inserts a slice of sessions and their exercise slots in one transaction.
// Returns domain.ErrAlreadyExists (wrapped) if any session in the batch conflicts with an
// existing row, so callers can recover from concurrent weekly-plan-generation races.
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
			// Only the workout_sessions PK conflict (duplicate date for this user) maps to
			// ErrAlreadyExists. UNIQUE violations from saveExerciseSets propagate as-is —
			// those are programming errors, not concurrent-insert races.
			var sqliteErr sqlite3.Error
			if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
				return fmt.Errorf("insert session %s: %w", formatDate(sess.Date), domain.ErrAlreadyExists)
			}
			return fmt.Errorf("insert session %s: %w", formatDate(sess.Date), err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit batch sessions: %w", err)
	}
	return nil
}

// Create inserts a single session and its exercise slots in one transaction.
// Translates the workout_sessions PRIMARY KEY conflict to domain.ErrAlreadyExists
// so callers can detect concurrent-insert races.
func (r *sqliteSessionRepository) Create(ctx context.Context, sess domain.Session) (err error) {
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
		// Only the workout_sessions PK conflict (duplicate date for this user) maps to
		// ErrAlreadyExists. UNIQUE violations from saveExerciseSets propagate as-is —
		// those are programming errors, not concurrent-insert races.
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintPrimaryKey {
			return fmt.Errorf("insert session %s: %w", formatDate(sess.Date), domain.ErrAlreadyExists)
		}
		return fmt.Errorf("insert session %s: %w", formatDate(sess.Date), err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit create session: %w", err)
	}
	return nil
}

func (r *sqliteSessionRepository) insertSession(ctx context.Context, tx *sql.Tx, sess domain.Session) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := formatDate(sess.Date)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workout_sessions (
			user_id, workout_date, difficulty_rating, started_at, completed_at, periodization_type, is_deload
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, dateStr, sess.DifficultyRating,
		formatTimestamp(sess.StartedAt), formatTimestamp(sess.CompletedAt),
		sess.PeriodizationType, sess.IsDeload); err != nil {
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
	isDeload bool,
) (domain.Session, error) {
	date, err := time.Parse(dateFormat, workoutDateStr)
	if err != nil {
		return domain.Session{}, fmt.Errorf("parse workout date: %w", err)
	}
	session := domain.Session{ //nolint:exhaustruct // ExerciseSets filled by caller.
		Date:              date,
		PeriodizationType: periodizationType,
		IsDeload:          isDeload,
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

// loadExerciseSetsRow holds one scanned row of the workout_exercise /
// exercise_sets / exercises join consumed by scanExerciseSetRows. Exercise
// columns join from `exercises` so we don't issue a per-slot follow-up
// query for the base exercise.
type loadExerciseSetsRow struct {
	weID                   int
	exerciseID             int
	warmupCompletedAtStr   sql.NullString
	setNumber              sql.NullInt32
	weightKg               sql.NullFloat64
	targetValue            sql.NullInt32
	completedValue         sql.NullInt32
	completedAtStr         sql.NullString
	signalStr              sql.NullString
	exerciseName           string
	exerciseCategory       domain.Category
	exerciseType           domain.ExerciseType
	exerciseDescription    string
	defaultStartingSeconds sql.NullInt64
	repMin                 sql.NullInt64
	repMax                 sql.NullInt64
}

// loadExerciseSets fetches all exercise slots for a single session, including
// ones with no sets yet (e.g. just-swapped exercises). The driving table is
// workout_exercise so empty slots still appear; sets are LEFT-JOINed in and
// the base exercise definition is JOINed from `exercises`. Muscle groups are
// hydrated in one follow-up query. Used by Get, including inside the Update
// transaction, where a single-date point read is what we want.
func (r *sqliteSessionRepository) loadExerciseSets(
	ctx context.Context,
	q queryer,
	userID int,
	date time.Time,
) (_ []domain.ExerciseSet, err error) {
	rows, err := q.QueryContext(ctx, `
		SELECT we.workout_date, we.id, we.exercise_id, we.warmup_completed_at,
		       es.set_number, es.weight_kg, es.target_value,
		       es.completed_value, es.completed_at, es.signal,
		       e.name, e.category, e.exercise_type, e.description_markdown,
		       e.default_starting_seconds, e.rep_min, e.rep_max
		FROM workout_exercise we
		LEFT JOIN exercise_sets es ON es.workout_exercise_id = we.id
		JOIN exercises e ON e.id = we.exercise_id
		WHERE we.workout_user_id = ? AND we.workout_date = ?
		ORDER BY we.id, es.set_number`,
		userID, formatDate(date))
	if err != nil {
		return nil, fmt.Errorf("query exercise sets: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	slots, _, err := scanExerciseSetRows(rows)
	if err != nil {
		return nil, err
	}
	if err = hydrateMuscleGroups(ctx, q, slots); err != nil {
		return nil, err
	}
	return slots, nil
}

// loadExerciseSetsSince fetches every exercise slot (with its sets) for the
// user's sessions on or after sinceDate in one query and returns them grouped
// by workout-date string. Muscle groups are hydrated in a single further
// query across all slots. This is the batched equivalent of loadExerciseSets
// used by List: the whole date range costs this one query plus one muscle-
// group query, replacing the prior per-session 1 + 2N N+1.
func (r *sqliteSessionRepository) loadExerciseSetsSince(
	ctx context.Context,
	q queryer,
	userID int,
	sinceDate time.Time,
) (_ map[string][]domain.ExerciseSet, err error) {
	rows, err := q.QueryContext(ctx, `
		SELECT we.workout_date, we.id, we.exercise_id, we.warmup_completed_at,
		       es.set_number, es.weight_kg, es.target_value,
		       es.completed_value, es.completed_at, es.signal,
		       e.name, e.category, e.exercise_type, e.description_markdown,
		       e.default_starting_seconds, e.rep_min, e.rep_max
		FROM workout_exercise we
		LEFT JOIN exercise_sets es ON es.workout_exercise_id = we.id
		JOIN exercises e ON e.id = we.exercise_id
		WHERE we.workout_user_id = ? AND we.workout_date >= ?
		ORDER BY we.workout_date DESC, we.id, es.set_number`,
		userID, formatDate(sinceDate))
	if err != nil {
		return nil, fmt.Errorf("query exercise sets: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	slots, dates, err := scanExerciseSetRows(rows)
	if err != nil {
		return nil, err
	}
	if err = hydrateMuscleGroups(ctx, q, slots); err != nil {
		return nil, err
	}

	byDate := make(map[string][]domain.ExerciseSet)
	for i := range slots {
		byDate[dates[i]] = append(byDate[dates[i]], slots[i])
	}
	return byDate, nil
}

// scanExerciseSetRows consumes the workout_exercise / exercise_sets /
// exercises join (with we.workout_date as the first selected column) into
// exercise slots. It returns the slots and a parallel slice of their
// workout-date strings; consecutive rows sharing a workout_exercise.id
// collapse into one slot. Muscle groups are left empty for the caller to
// hydrate in a single batched query.
func scanExerciseSetRows(rows *sql.Rows) ([]domain.ExerciseSet, []string, error) {
	var (
		slots       []domain.ExerciseSet
		dates       []string
		current     *domain.ExerciseSet
		currentDate string
		err         error
	)
	flush := func() {
		if current != nil {
			slots = append(slots, *current)
			dates = append(dates, currentDate)
		}
	}

	for rows.Next() {
		var (
			workoutDateStr string
			row            loadExerciseSetsRow
		)
		if err = rows.Scan(&workoutDateStr, &row.weID, &row.exerciseID, &row.warmupCompletedAtStr,
			&row.setNumber, &row.weightKg, &row.targetValue,
			&row.completedValue, &row.completedAtStr, &row.signalStr,
			&row.exerciseName, &row.exerciseCategory, &row.exerciseType, &row.exerciseDescription,
			&row.defaultStartingSeconds, &row.repMin, &row.repMax); err != nil {
			return nil, nil, fmt.Errorf("scan exercise set: %w", err)
		}

		if current == nil || row.weID != current.ID {
			flush()
			started, startErr := startExerciseSet(row)
			if startErr != nil {
				return nil, nil, startErr
			}
			current = &started
			currentDate = workoutDateStr
		}

		// LEFT JOIN can yield a workout_exercise row with no sets (set_number IS NULL).
		if !row.setNumber.Valid {
			continue
		}
		set, parseErr := buildSet(row)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		current.Sets = append(current.Sets, set)
	}
	flush()

	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("rows error: %w", err)
	}
	return slots, dates, nil
}

// hydrateMuscleGroups fetches primary/secondary muscle groups for every
// distinct Exercise.ID across the given slots in a single query and writes
// them back onto the slots' Exercise fields.
func hydrateMuscleGroups(ctx context.Context, q queryer, sets []domain.ExerciseSet) error {
	if len(sets) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(sets))
	ids := make([]int, 0, len(sets))
	for _, es := range sets {
		if _, ok := seen[es.Exercise.ID]; ok {
			continue
		}
		seen[es.Exercise.ID] = struct{}{}
		ids = append(ids, es.Exercise.ID)
	}

	byExercise, err := fetchMuscleGroupsByExerciseID(ctx, q, ids)
	if err != nil {
		return err
	}

	for i := range sets {
		g := byExercise[sets[i].Exercise.ID]
		sets[i].Exercise.PrimaryMuscleGroups = g.primary
		sets[i].Exercise.SecondaryMuscleGroups = g.secondary
	}
	return nil
}

// startExerciseSet constructs a fresh domain.ExerciseSet from a joined row.
// Muscle-group fields stay empty here — they are populated in a single
// follow-up query by hydrateMuscleGroups once all slots are read.
func startExerciseSet(row loadExerciseSetsRow) (domain.ExerciseSet, error) {
	warmupCompletedAt, err := parseWarmupCompletedAtTimestamp(row.warmupCompletedAtStr)
	if err != nil {
		return domain.ExerciseSet{}, err
	}
	exercise := domain.Exercise{ //nolint:exhaustruct // muscle groups filled in by hydrateMuscleGroups.
		ID:                  row.exerciseID,
		Name:                row.exerciseName,
		Category:            row.exerciseCategory,
		ExerciseType:        row.exerciseType,
		DescriptionMarkdown: row.exerciseDescription,
	}
	if row.defaultStartingSeconds.Valid {
		v := int(row.defaultStartingSeconds.Int64)
		exercise.DefaultStartingSeconds = &v
	}
	if row.repMin.Valid {
		v := int(row.repMin.Int64)
		exercise.RepMin = &v
	}
	if row.repMax.Valid {
		v := int(row.repMax.Int64)
		exercise.RepMax = &v
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
		       es.completed_value, es.completed_at, es.signal
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
		  AND ws.is_deload = 0
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
