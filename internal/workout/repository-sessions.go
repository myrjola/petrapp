package workout

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"log/slog"
	"time"
)

// sqliteSessionRepository implements SessionRepository.
type sqliteSessionRepository struct {
	baseRepository
	exerciseRepo *sqliteExerciseRepository
}

// newSQLiteSessionRepository creates a new SQLite session repository.
func newSQLiteSessionRepository(
	db *sqlite.Database,
	logger *slog.Logger,
	exerciseRepo *sqliteExerciseRepository,
) *sqliteSessionRepository {
	return &sqliteSessionRepository{
		baseRepository: newBaseRepository(db, logger),
		exerciseRepo:   exerciseRepo,
	}
}

// List retrieves all workout sessions since a given date.
func (r *sqliteSessionRepository) List(ctx context.Context, sinceDate time.Time) (_ []Session, err error) {
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

	var sessions []Session
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

		var session Session
		session, err = r.parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr)
		if err != nil {
			return nil, err
		}

		// Load exercise sets for the session
		var exerciseSets []ExerciseSet
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
func (r *sqliteSessionRepository) Get(ctx context.Context, date time.Time) (Session, error) {
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
			return Session{}, ErrNotFound
		}
		return Session{}, fmt.Errorf("query session: %w", err)
	}

	session, err := r.parseSessionRow(workoutDateStr, difficultyRating, startedAtStr, completedAtStr)
	if err != nil {
		return Session{}, err
	}

	// Load exercise sets for the session
	exerciseSets, err := r.loadExerciseSets(ctx, userID, session.Date)
	if err != nil {
		return Session{}, err
	}
	session.ExerciseSets = exerciseSets

	return session, nil
}

// Create adds new workout session.
func (r *sqliteSessionRepository) Create(ctx context.Context, sess Session) error {
	if err := r.set(ctx, sess, false); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}

// set creates a new workout session with optional upsert.
func (r *sqliteSessionRepository) set(ctx context.Context, sess Session, upsert bool) (err error) {
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

	// We delete the session if so that it can be reinserted
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
	updateFn func(sess *Session) (bool, error),
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
) (Session, error) {
	var session Session

	// Parse date
	date, err := time.Parse(dateFormat, workoutDateStr)
	if err != nil {
		return Session{}, fmt.Errorf("parse workout date: %w", err)
	}
	session.Date = date

	// Parse difficulty rating
	if difficultyRating.Valid {
		rating := int(difficultyRating.Int32)
		session.DifficultyRating = &rating
	}

	var startedAt time.Time
	if startedAt, err = parseTimestamp(startedAtStr); err != nil {
		return Session{}, fmt.Errorf("parse started_at: %w", err)
	}
	session.StartedAt = startedAt

	var completedAt time.Time
	if completedAt, err = parseTimestamp(completedAtStr); err != nil {
		return Session{}, fmt.Errorf("parse completed_at: %w", err)
	}
	session.CompletedAt = completedAt

	return session, nil
}

// loadExerciseSets fetches all exercise sets for a session.
func (r *sqliteSessionRepository) loadExerciseSets(
	ctx context.Context,
	userID []byte,
	date time.Time,
) ([]ExerciseSet, error) {
	dateStr := formatDate(date)

	// Query for exercise sets
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT es.exercise_id, es.weight_kg, es.min_reps, es.max_reps, es.completed_reps
		FROM exercise_sets es
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

	var exerciseSets []ExerciseSet
	var currentExerciseSet ExerciseSet

	for rows.Next() {
		var (
			exerciseID int
			set        Set
		)
		err = rows.Scan(&exerciseID, &set.WeightKg, &set.MinReps, &set.MaxReps, &set.CompletedReps)
		if err != nil {
			return nil, fmt.Errorf("scan exercise set: %w", err)
		}
		if exerciseID != currentExerciseSet.Exercise.ID {
			// Add the previous exercise set if it exists
			if currentExerciseSet.Exercise.ID != 0 {
				exerciseSets = append(exerciseSets, currentExerciseSet)
			}

			// Fetch exercise details
			var exercise Exercise
			exercise, err = r.exerciseRepo.Get(ctx, exerciseID)
			if err != nil {
				return nil, fmt.Errorf("fetch exercise %d: %w", exerciseID, err)
			}

			currentExerciseSet = ExerciseSet{
				Exercise: exercise,
				Sets:     []Set{},
			}
		}

		currentExerciseSet.Sets = append(currentExerciseSet.Sets, set)
	}

	// Add the last exercise if it exists
	if currentExerciseSet.Exercise.ID != 0 {
		exerciseSets = append(exerciseSets, currentExerciseSet)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return exerciseSets, nil
}

// saveExerciseSets inserts or updates exercise sets for a session.
func (r *sqliteSessionRepository) saveExerciseSets(
	ctx context.Context,
	tx *sql.Tx,
	date time.Time,
	exerciseSets []ExerciseSet,
) error {
	dateStr := formatDate(date)
	userID := contexthelpers.AuthenticatedUserID(ctx)

	for _, exerciseSet := range exerciseSets {
		for i, set := range exerciseSet.Sets {
			_, err := tx.ExecContext(ctx, `
				INSERT INTO exercise_sets (
					workout_user_id, workout_date, exercise_id, set_number,
					weight_kg, min_reps, max_reps, completed_reps
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				userID, dateStr, exerciseSet.Exercise.ID, i+1,
				set.WeightKg, set.MinReps, set.MaxReps, set.CompletedReps)

			if err != nil {
				return fmt.Errorf("upsert exercise set: %w", err)
			}
		}
	}

	return nil
}
