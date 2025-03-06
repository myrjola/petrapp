package workout

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/myrjola/petrapp/internal/sqlite"
	"log/slog"
	"time"
)

const timestampFormat = "2006-01-02T15:04:05.000Z"
const dateFormat = time.DateOnly

// sqliteRepository handles database operations for workouts.
type sqliteRepository struct {
	db     *sqlite.Database
	logger *slog.Logger
}

// newSQLiteRepository creates a new SQLite-backed workout repository.
func newSQLiteRepository(db *sqlite.Database, logger *slog.Logger) *sqliteRepository {
	return &sqliteRepository{
		db:     db,
		logger: logger,
	}
}

// getUserPreferences retrieves the workout preferences for a user.
func (r *sqliteRepository) getUserPreferences(ctx context.Context, userID []byte) (Preferences, error) {
	var prefs Preferences
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT monday, tuesday, wednesday, thursday, friday, saturday, sunday 
		FROM workout_preferences 
		WHERE user_id = ?`, userID).Scan(
		&prefs.Monday,
		&prefs.Tuesday,
		&prefs.Wednesday,
		&prefs.Thursday,
		&prefs.Friday,
		&prefs.Saturday,
		&prefs.Sunday,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// If no preferences are found, return default preferences
		return Preferences{
			Monday:    false,
			Tuesday:   false,
			Wednesday: false,
			Thursday:  false,
			Friday:    false,
			Saturday:  false,
			Sunday:    false,
		}, nil
	}
	if err != nil {
		return Preferences{}, fmt.Errorf("query workout preferences: %w", err)
	}
	return prefs, nil
}

// saveUserPreferences saves the workout preferences for a user.
func (r *sqliteRepository) saveUserPreferences(ctx context.Context, userID []byte, prefs Preferences) error {
	_, err := r.db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_preferences (
			user_id, monday, tuesday, wednesday, thursday, friday, saturday, sunday
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET
			monday = excluded.monday,
			tuesday = excluded.tuesday,
			wednesday = excluded.wednesday,
			thursday = excluded.thursday,
			friday = excluded.friday,
			saturday = excluded.saturday,
			sunday = excluded.sunday`,
		userID,
		prefs.Monday,
		prefs.Tuesday,
		prefs.Wednesday,
		prefs.Thursday,
		prefs.Friday,
		prefs.Saturday,
		prefs.Sunday,
	)
	if err != nil {
		return fmt.Errorf("save workout preferences: %w", err)
	}
	return nil
}

// getSession retrieves a workout session for a specific date.
func (r *sqliteRepository) getSession(ctx context.Context, userID []byte, date time.Time) (Session, error) {
	session, err := r.queryWorkoutSession(ctx, userID, date)
	if err != nil {
		return Session{}, fmt.Errorf("query workout session: %w", err)
	}

	// Load exercise sets.
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
        SELECT e.id, e.name, e.category, 
               es.set_number, es.weight_kg,
               es.min_reps, es.max_reps, es.completed_reps
        FROM exercise_sets es
        JOIN exercises e ON e.id = es.exercise_id
        WHERE es.workout_user_id = ? AND es.workout_date = ?
        ORDER BY es.exercise_id, es.set_number`,
		userID, date.Format(dateFormat))
	if err != nil {
		return Session{}, fmt.Errorf("query exercise sets: %w", err)
	}
	defer rows.Close()

	var currentExerciseSet *ExerciseSet
	for rows.Next() {
		var (
			exercise Exercise
			set      Set
			setNum   int
		)

		err = rows.Scan(
			&exercise.ID, &exercise.Name, &exercise.Category,
			&setNum, &set.WeightKg, &set.MinReps, &set.MaxReps,
			&set.CompletedReps)
		if err != nil {
			return Session{}, fmt.Errorf("scan exercise set: %w", err)
		}

		// If this is a new exercise or the first one.
		if currentExerciseSet == nil || currentExerciseSet.Exercise.ID != exercise.ID {
			if currentExerciseSet != nil {
				session.ExerciseSets = append(session.ExerciseSets, *currentExerciseSet)
			}

			// Load muscle groups for the exercise.
			var primaryMuscleGroups, secondaryMuscleGroups []string
			primaryMuscleGroups, secondaryMuscleGroups, err = r.fetchExerciseMuscleGroups(ctx, exercise.ID)
			if err != nil {
				return Session{}, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
			}
			exercise.PrimaryMuscleGroups = primaryMuscleGroups
			exercise.SecondaryMuscleGroups = secondaryMuscleGroups

			currentExerciseSet = &ExerciseSet{
				Exercise: exercise,
				Sets:     []Set{},
			}
		}

		currentExerciseSet.Sets = append(currentExerciseSet.Sets, set)
	}

	// Add the last exercise if it exists.
	if currentExerciseSet != nil {
		session.ExerciseSets = append(session.ExerciseSets, *currentExerciseSet)
	}

	if err = rows.Err(); err != nil {
		return Session{}, fmt.Errorf("rows error: %w", err)
	}

	// Determine status.
	if session.CompletedAt != nil {
		session.Status = StatusDone
	} else {
		session.Status = StatusPlanned
	}

	return session, nil
}

func (r *sqliteRepository) queryWorkoutSession(ctx context.Context, userID []byte, date time.Time) (Session, error) {
	var (
		workoutDateStr string
		session        Session
		startedAtStr   sql.NullString
		completedAtStr sql.NullString
	)
	err := r.db.ReadOnly.QueryRowContext(ctx, `
        SELECT workout_date, difficulty_rating, started_at, completed_at
        FROM workout_sessions 
        WHERE user_id = ? AND workout_date = ?`,
		userID, date.Format(dateFormat)).
		Scan(&workoutDateStr, &session.DifficultyRating, &startedAtStr, &completedAtStr)

	if err != nil {
		return Session{}, fmt.Errorf("query workout session: %w", err)
	}

	// Parse timestamps.
	session.WorkoutDate = date // Use the input date since we know it matches

	var startedAt, completedAt *time.Time
	if startedAt, err = parseTimestamp(startedAtStr); err != nil {
		return Session{}, fmt.Errorf("parse started_at: %w", err)
	}
	session.StartedAt = startedAt

	if completedAt, err = parseTimestamp(completedAtStr); err != nil {
		return Session{}, fmt.Errorf("parse completed_at: %w", err)
	}
	session.CompletedAt = completedAt
	return session, nil
}

// startSession starts a new workout session or returns an error if one already exists.
func (r *sqliteRepository) startSession(ctx context.Context, userID []byte, date time.Time) error {
	dateStr := date.Format(dateFormat)
	startedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// Start a transaction since we need to insert multiple rows
	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func(tx *sql.Tx) {
		err = tx.Rollback()
		if err != nil && !errors.Is(err, sql.ErrTxDone) {
			r.logger.LogAttrs(ctx, slog.LevelError, "rollback transaction", slog.Any("error", err))
		}
	}(tx)

	_, err = tx.ExecContext(ctx, `
        INSERT INTO workout_sessions (user_id, workout_date, started_at)
        VALUES (?, ?, ?)
        ON CONFLICT (user_id, workout_date) DO UPDATE SET
            started_at = COALESCE(workout_sessions.started_at, ?)`,
		userID, dateStr, startedAt, startedAt)
	if err != nil {
		return fmt.Errorf("insert workout session: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// saveExerciseSets adds exercise sets to a workout session.
func (r *sqliteRepository) saveExerciseSets(
	ctx context.Context,
	userID []byte,
	date time.Time,
	exerciseSets []ExerciseSet,
) error {
	dateStr := date.Format(dateFormat)

	tx, err := r.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func(tx *sql.Tx) {
		err = tx.Rollback()
		if err != nil && !errors.Is(err, sql.ErrTxDone) {
			r.logger.LogAttrs(ctx, slog.LevelError, "rollback transaction", slog.Any("error", err))
		}
	}(tx)

	// Insert exercise sets
	for _, exerciseSet := range exerciseSets {
		for i, set := range exerciseSet.Sets {
			_, err = tx.ExecContext(ctx, `
                INSERT INTO exercise_sets (
                    workout_user_id, workout_date, exercise_id, set_number,
                    weight_kg, min_reps, max_reps
                ) VALUES (?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT (workout_user_id, workout_date, exercise_id, set_number) DO NOTHING`,
				userID, dateStr, exerciseSet.Exercise.ID, i+1,
				set.WeightKg, set.MinReps, set.MaxReps)
			if err != nil {
				return fmt.Errorf("insert exercise set: %w", err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// completeSession marks a workout session as completed.
func (r *sqliteRepository) completeSession(ctx context.Context, userID []byte, date time.Time) error {
	completedAt := time.Now().UTC().Format(timestampFormat)

	result, err := r.db.ReadWrite.ExecContext(ctx, `
        UPDATE workout_sessions 
        SET completed_at = ?
        WHERE user_id = ? AND workout_date = ? AND completed_at IS NULL`,
		completedAt, userID, date.Format(dateFormat))
	if err != nil {
		return fmt.Errorf("complete workout session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return errors.New("workout session not found or already completed")
	}

	return nil
}

// saveFeedback saves the difficulty rating for a completed workout session.
func (r *sqliteRepository) saveFeedback(ctx context.Context, userID []byte, date time.Time, difficulty int) error {
	if difficulty < 1 || difficulty > 5 {
		return fmt.Errorf("invalid difficulty rating (difficulty: %d, date: %s)",
			difficulty, date.Format(dateFormat))
	}

	result, err := r.db.ReadWrite.ExecContext(ctx, `
		UPDATE workout_sessions
        SET difficulty_rating = ?
        WHERE user_id = ? AND workout_date = ?`,
		difficulty, userID, date.Format(dateFormat))
	if err != nil {
		return fmt.Errorf("save difficulty rating: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return errors.New("workout session not found")
	}

	return nil
}

// updateSetWeight updates the weight for a specific set in a workout.
func (r *sqliteRepository) updateSetWeight(
	ctx context.Context,
	userID []byte,
	date time.Time,
	exerciseID int,
	setIndex int,
	newWeight float64,
) error {
	dateStr := date.Format(dateFormat)

	result, err := r.db.ReadWrite.ExecContext(ctx, `
        UPDATE exercise_sets 
        SET weight_kg = ?
        WHERE workout_user_id = ? 
        AND workout_date = ? 
        AND exercise_id = ?
        AND set_number = ?`,
		newWeight, userID, dateStr, exerciseID, setIndex+1)
	if err != nil {
		return fmt.Errorf("update exercise set: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return errors.New("set not found")
	}

	return nil
}

// updateCompletedReps updates a previously completed set with new rep count.
func (r *sqliteRepository) updateCompletedReps(
	ctx context.Context,
	userID []byte,
	date time.Time,
	exerciseID int,
	setIndex int,
	completedReps int,
) error {
	dateStr := date.Format(dateFormat)

	// First verify the set exists and is already completed
	var minReps, maxReps int
	var currentReps sql.NullInt64
	err := r.db.ReadOnly.QueryRowContext(ctx, `
        SELECT min_reps, max_reps, completed_reps
        FROM exercise_sets
        WHERE workout_user_id = ?
        AND workout_date = ?
        AND exercise_id = ?
        AND set_number = ?`,
		userID, dateStr, exerciseID, setIndex+1).Scan(&minReps, &maxReps, &currentReps)
	if err != nil {
		return fmt.Errorf("get set rep range: %w", err)
	}

	// Allow updating with reps outside the target range, but log it
	if completedReps < minReps || completedReps > maxReps {
		r.logger.LogAttrs(ctx, slog.LevelInfo, "updated reps outside target range",
			slog.Int("completed_reps", completedReps),
			slog.Int("min_reps", minReps),
			slog.Int("max_reps", maxReps))
	}

	result, err := r.db.ReadWrite.ExecContext(ctx, `
        UPDATE exercise_sets
        SET completed_reps = ?
        WHERE workout_user_id = ?
        AND workout_date = ?
        AND exercise_id = ?
        AND set_number = ?`,
		completedReps, userID, dateStr, exerciseID, setIndex+1)
	if err != nil {
		return fmt.Errorf("update completed reps: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return errors.New("set not found")
	}

	return nil
}

// parseTimestamp parses a timestamp from a nullable database string.
func parseTimestamp(timestampStr sql.NullString) (*time.Time, error) {
	if timestampStr.Valid {
		parsedTime, err := time.Parse(timestampFormat, timestampStr.String)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp format: %w", err)
		}
		return &parsedTime, nil
	}
	return nil, nil //nolint:nilnil // nil time.Time is expected when the string is NULL.
}

// fetchExerciseMuscleGroups retrieves the muscle groups associated with an exercise.
func (r *sqliteRepository) fetchExerciseMuscleGroups(ctx context.Context, exerciseID int) ([]string, []string, error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
        SELECT mg.name, emg.is_primary
        FROM exercise_muscle_groups emg
        JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
        WHERE emg.exercise_id = ?
    `, exerciseID)
	if err != nil {
		return nil, nil, fmt.Errorf("query muscle groups: %w", err)
	}
	defer rows.Close()

	var primaryMuscleGroups []string
	var secondaryMuscleGroups []string

	for rows.Next() {
		var (
			name      string
			isPrimary bool
		)

		if err = rows.Scan(&name, &isPrimary); err != nil {
			return nil, nil, fmt.Errorf("scan muscle group row: %w", err)
		}

		if isPrimary {
			primaryMuscleGroups = append(primaryMuscleGroups, name)
		} else {
			secondaryMuscleGroups = append(secondaryMuscleGroups, name)
		}
	}

	// Check for errors from iterating over rows
	if err = rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate muscle group rows: %w", err)
	}

	return primaryMuscleGroups, secondaryMuscleGroups, nil
}

// getWorkoutHistory retrieves the user's workout history within a date range.
func (r *sqliteRepository) getWorkoutHistory(
	ctx context.Context,
	userID []byte,
	startDate, endDate time.Time,
) ([]Session, error) {
	startDateStr := startDate.Format(dateFormat)
	endDateStr := endDate.Format(dateFormat)

	// First get all workout sessions in the date range
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT workout_date, difficulty_rating, started_at, completed_at
		FROM workout_sessions
		WHERE user_id = ? AND workout_date BETWEEN ? AND ?
		ORDER BY workout_date DESC`,
		userID, startDateStr, endDateStr)
	if err != nil {
		return nil, fmt.Errorf("query workout history: %w", err)
	}
	defer rows.Close()

	var sessions []Session

	for rows.Next() {
		var (
			session        Session
			workoutDateStr string
			startedAtStr   sql.NullString
			completedAtStr sql.NullString
		)

		err = rows.Scan(&workoutDateStr, &session.DifficultyRating, &startedAtStr, &completedAtStr)
		if err != nil {
			return nil, fmt.Errorf("scan workout history row: %w", err)
		}

		session.WorkoutDate, err = time.Parse(dateFormat, workoutDateStr)
		if err != nil {
			return nil, fmt.Errorf("parse workout date: %w", err)
		}

		var startedAt, completedAt *time.Time
		if startedAt, err = parseTimestamp(startedAtStr); err != nil {
			return nil, fmt.Errorf("parse started_at: %w", err)
		}
		session.StartedAt = startedAt

		if completedAt, err = parseTimestamp(completedAtStr); err != nil {
			return nil, fmt.Errorf("parse completed_at: %w", err)
		}
		session.CompletedAt = completedAt

		// Determine status
		if session.CompletedAt != nil {
			session.Status = StatusDone
		} else {
			session.Status = StatusPlanned
		}

		sessions = append(sessions, session)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Now load exercise sets for all sessions
	for i, session := range sessions {
		// Skip loading exercises for non-completed workouts
		if session.Status != StatusDone {
			continue
		}

		var exerciseSets []ExerciseSet
		exerciseSets, err = r.getExerciseSetsForWorkout(ctx, userID, session.WorkoutDate)
		if err != nil {
			return nil, fmt.Errorf("get exercise sets for workout: %w", err)
		}

		sessions[i].ExerciseSets = exerciseSets
	}

	return sessions, nil
}

// getExerciseSetsForWorkout fetches all exercise sets for a specific workout.
func (r *sqliteRepository) getExerciseSetsForWorkout(
	ctx context.Context,
	userID []byte,
	date time.Time,
) ([]ExerciseSet, error) {
	dateStr := date.Format(dateFormat)

	// Load exercise sets
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT e.id, e.name, e.category, 
		       es.set_number, es.weight_kg,
		       es.min_reps, es.max_reps, es.completed_reps
		FROM exercise_sets es
		JOIN exercises e ON e.id = es.exercise_id
		WHERE es.workout_user_id = ? AND es.workout_date = ?
		ORDER BY es.exercise_id, es.set_number`,
		userID, dateStr)
	if err != nil {
		return nil, fmt.Errorf("query exercise sets: %w", err)
	}
	defer rows.Close()

	var exerciseSets []ExerciseSet
	var currentExerciseSet *ExerciseSet

	for rows.Next() {
		var (
			exercise Exercise
			set      Set
			setNum   int
		)

		err = rows.Scan(
			&exercise.ID, &exercise.Name, &exercise.Category,
			&setNum, &set.WeightKg, &set.MinReps, &set.MaxReps,
			&set.CompletedReps)
		if err != nil {
			return nil, fmt.Errorf("scan exercise set: %w", err)
		}

		// If this is a new exercise or the first one
		if currentExerciseSet == nil || currentExerciseSet.Exercise.ID != exercise.ID {
			if currentExerciseSet != nil {
				exerciseSets = append(exerciseSets, *currentExerciseSet)
			}

			// Load muscle groups for the exercise
			var primaryMuscleGroups, secondaryMuscleGroups []string
			primaryMuscleGroups, secondaryMuscleGroups, err = r.fetchExerciseMuscleGroups(ctx, exercise.ID)
			if err != nil {
				return nil, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
			}
			exercise.PrimaryMuscleGroups = primaryMuscleGroups
			exercise.SecondaryMuscleGroups = secondaryMuscleGroups

			currentExerciseSet = &ExerciseSet{
				Exercise: exercise,
				Sets:     []Set{},
			}
		}

		currentExerciseSet.Sets = append(currentExerciseSet.Sets, set)
	}

	// Add the last exercise if it exists
	if currentExerciseSet != nil {
		exerciseSets = append(exerciseSets, *currentExerciseSet)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return exerciseSets, nil
}

// getExercisePool returns all available exercises with their muscle groups.
func (r *sqliteRepository) getExercisePool(ctx context.Context) ([]Exercise, error) {
	// First, get all exercises
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, name, category
		FROM exercises
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query exercises: %w", err)
	}
	defer rows.Close()

	var exercises []Exercise
	for rows.Next() {
		var exercise Exercise
		if err = rows.Scan(&exercise.ID, &exercise.Name, &exercise.Category); err != nil {
			return nil, fmt.Errorf("scan exercise: %w", err)
		}
		exercises = append(exercises, exercise)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Now fetch muscle groups for each exercise
	for i, exercise := range exercises {
		var primaryMuscleGroups, secondaryMuscleGroups []string
		primaryMuscleGroups, secondaryMuscleGroups, err = r.fetchExerciseMuscleGroups(ctx, exercise.ID)
		if err != nil {
			return nil, fmt.Errorf("fetch muscle groups for exercise %d: %w", exercise.ID, err)
		}
		exercises[i].PrimaryMuscleGroups = primaryMuscleGroups
		exercises[i].SecondaryMuscleGroups = secondaryMuscleGroups
	}

	return exercises, nil
}
