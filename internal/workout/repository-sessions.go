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
		SELECT workout_date, difficulty_rating, started_at, completed_at
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
			workoutDateStr   string
			difficultyRating sql.NullInt32
			startedAtStr     sql.NullString
			completedAtStr   sql.NullString
		)

		if err = rows.Scan(&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}

		var session sessionAggregate
		session, err = r.parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr)
		if err != nil {
			return nil, err
		}

		// Load exercise sets for the session
		var exerciseSets []exerciseSetAggregate
		exerciseSets, err = r.loadExerciseSets(ctx, userID, session.Date)
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
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := formatDate(date)

	var (
		workoutDateStr   string
		difficultyRating sql.NullInt32
		startedAtStr     sql.NullString
		completedAtStr   sql.NullString
	)

	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at
		FROM workout_sessions
		WHERE user_id = ? AND workout_date = ?`,
		userID, dateStr).Scan(&workoutDateStr, &difficultyRating, &startedAtStr, &completedAtStr)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sessionAggregate{}, ErrNotFound
		}
		return sessionAggregate{}, fmt.Errorf("query session: %w", err)
	}

	session, err := r.parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr)
	if err != nil {
		return sessionAggregate{}, err
	}

	// Load exercise sets for the session
	exerciseSets, err := r.loadExerciseSets(ctx, userID, session.Date)
	if err != nil {
		return sessionAggregate{}, err
	}
	session.ExerciseSets = exerciseSets

	return session, nil
}

// Create adds new workout session.
func (r *sqliteSessionRepository) Create(ctx context.Context, sess sessionAggregate) error {
	if err := r.set(ctx, sess, false); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}

// set creates a new workout session with optional upsert.
func (r *sqliteSessionRepository) set(ctx context.Context, sess sessionAggregate, upsert bool) (err error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := formatDate(sess.Date)

	// Begin transaction
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
	}()

	// We delete the session so that it can be reinserted.
	if upsert {
		_, err = tx.ExecContext(ctx, `
			DELETE FROM workout_sessions
			WHERE user_id = ? AND workout_date = ?`,
			userID, dateStr)
		if err != nil {
			return fmt.Errorf("delete session: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO workout_sessions (
			user_id, workout_date, difficulty_rating, started_at, completed_at
		) VALUES (?, ?, ?, ?, ?)`,
		userID, dateStr, sess.DifficultyRating, formatTimestamp(sess.StartedAt), formatTimestamp(sess.CompletedAt))

	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	// Insert exercise sets
	if err = r.saveExerciseSets(ctx, tx, sess.Date, sess.ExerciseSets); err != nil {
		return fmt.Errorf("save exercise sets: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// Update modifies an existing workout session.
func (r *sqliteSessionRepository) Update(
	ctx context.Context,
	date time.Time,
	updateFn func(sess *sessionAggregate) (bool, error),
) error {
	// Get current session
	session, err := r.Get(ctx, date)
	if err != nil {
		return fmt.Errorf("get session for update: %w", err)
	}

	// Apply updates
	updated, err := updateFn(&session)
	if err != nil {
		return fmt.Errorf("update function: %w", err)
	}

	// Save if changed
	if updated {
		if err = r.set(ctx, session, true); err != nil {
			return fmt.Errorf("save updated session: %w", err)
		}
	}

	return nil
}

// parseSessionRow converts database values to a Session.
func (r *sqliteSessionRepository) parseSessionRow(
	workoutDateStr string,
	difficultyRating sql.NullInt32,
	startedAtStr sql.NullString,
	completedAtStr sql.NullString,
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

	return session, nil
}

// loadExerciseSets fetches all exercise sets for a session.
func (r *sqliteSessionRepository) loadExerciseSets(
	ctx context.Context,
	userID []byte,
	date time.Time,
) (_ []exerciseSetAggregate, err error) {
	dateStr := formatDate(date)

	// Query for exercise sets with warmup completion timestamp
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT es.exercise_id, es.weight_kg, es.min_reps, es.max_reps, es.completed_reps, 
		       es.completed_at, we.warmup_completed_at
		FROM exercise_sets es
		LEFT JOIN workout_exercise we ON we.workout_user_id = es.workout_user_id 
		                              AND we.workout_date = es.workout_date 
		                              AND we.exercise_id = es.exercise_id
		WHERE es.workout_user_id = ? AND es.workout_date = ?
		ORDER BY es.exercise_id, es.set_number`,
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
	var currentExerciseSet exerciseSetAggregate
	currentExerciseSet.ExerciseID = -1

	for rows.Next() {
		var (
			exerciseID           int
			set                  Set
			completedAtStr       sql.NullString
			warmupCompletedAtStr sql.NullString
		)
		err = rows.Scan(&exerciseID, &set.WeightKg, &set.MinReps, &set.MaxReps,
			&set.CompletedReps, &completedAtStr, &warmupCompletedAtStr)
		if err != nil {
			return nil, fmt.Errorf("scan exercise set: %w", err)
		}

		// Parse the completed_at timestamp
		if err = r.parseCompletedAtTimestamp(completedAtStr, &set); err != nil {
			return nil, err
		}

		if exerciseID != currentExerciseSet.ExerciseID {
			// Add the previous exercise set if it exists
			if currentExerciseSet.ExerciseID != -1 {
				exerciseSets = append(exerciseSets, currentExerciseSet)
			}

			// Create new exercise set with parsed warmup timestamp
			warmupCompletedAt, parseErr := r.parseWarmupCompletedAtTimestamp(warmupCompletedAtStr)
			if parseErr != nil {
				return nil, parseErr
			}

			currentExerciseSet = exerciseSetAggregate{
				ExerciseID:        exerciseID,
				Sets:              []Set{},
				WarmupCompletedAt: warmupCompletedAt,
			}
		}

		currentExerciseSet.Sets = append(currentExerciseSet.Sets, set)
	}

	// Add the last exercise if it exists
	if currentExerciseSet.ExerciseID != -1 {
		exerciseSets = append(exerciseSets, currentExerciseSet)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return exerciseSets, nil
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

// saveExerciseSets inserts or updates exercise sets for a session.
func (r *sqliteSessionRepository) saveExerciseSets(
	ctx context.Context,
	tx *sql.Tx,
	date time.Time,
	exerciseSets []exerciseSetAggregate,
) error {
	dateStr := formatDate(date)
	userID := contexthelpers.AuthenticatedUserID(ctx)

	for _, exerciseSet := range exerciseSets {
		for i, set := range exerciseSet.Sets {
			// Format CompletedAt timestamp if it's not nil
			var completedAtStr interface{}
			if set.CompletedAt != nil {
				completedAtStr = formatTimestamp(*set.CompletedAt)
			}

			_, err := tx.ExecContext(ctx, `
				INSERT INTO exercise_sets (
					workout_user_id, workout_date, exercise_id, set_number,
					weight_kg, min_reps, max_reps, completed_reps, completed_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				userID, dateStr, exerciseSet.ExerciseID, i+1,
				set.WeightKg, set.MinReps, set.MaxReps, set.CompletedReps, completedAtStr)

			if err != nil {
				return fmt.Errorf("insert exercise set: %w", err)
			}
		}

		// Insert or update workout_exercise record for warmup completion tracking
		if exerciseSet.WarmupCompletedAt != nil {
			warmupCompletedAtStr := formatTimestamp(*exerciseSet.WarmupCompletedAt)

			_, err := tx.ExecContext(ctx, `
				INSERT OR REPLACE INTO workout_exercise (
					workout_user_id, workout_date, exercise_id, warmup_completed_at
				) VALUES (?, ?, ?, ?)`,
				userID, dateStr, exerciseSet.ExerciseID, warmupCompletedAtStr)

			if err != nil {
				return fmt.Errorf("insert workout exercise: %w", err)
			}
		}
	}

	return nil
}
