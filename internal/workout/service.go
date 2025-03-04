package workout

import (
	"context"
	"database/sql"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/sqlite"
	"log/slog"
	"time"
)

// Service handles the business logic for workout management.
type Service struct {
	db     *sqlite.Database
	logger *slog.Logger
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger) *Service {
	return &Service{
		db:     db,
		logger: logger,
	}
}

// GetUserPreferences retrieves the workout preferences for a user.
func (s *Service) GetUserPreferences(ctx context.Context) (Preferences, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	//nolint:godox // temporary todo
	// TODO: Implement repository pattern and move SQL to repository
	var prefs Preferences
	err := s.db.ReadOnly.QueryRowContext(ctx, `
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
		return Preferences{}, errors.Wrap(err, "query workout preferences")
	}
	return prefs, nil
}

// SaveUserPreferences saves the workout preferences for a user.
func (s *Service) SaveUserPreferences(ctx context.Context, prefs Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	_, err := s.db.ReadWrite.ExecContext(ctx, `
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
		return errors.Wrap(err, "save workout preferences")
	}
	return nil
}

// GenerateWorkout creates a new workout plan based on user preferences and history.
func (s *Service) generateWorkout(_ context.Context, date time.Time) (Session, error) {
	//nolint:godox // temporary todo
	// TODO: Implement smart workout generation logic
	// This should:
	// 1. Check if it's a workout day based on preferences
	// 2. Determine workout type (full body vs split) based on consecutive days
	// 3. Select appropriate exercises
	// 4. Calculate proper sets/reps/weights based on history
	if date.Weekday() == -1 {
		return Session{}, errors.New("test") // to keep the linter happy for now.
	}
	return Session{
		WorkoutDate:      date,
		Status:           StatusPlanned,
		DifficultyRating: nil,
		StartedAt:        nil,
		CompletedAt:      nil,
		ExerciseSets: []ExerciseSet{
			{
				Exercise: Exercise{
					ID:       1,
					Name:     "Squat",
					Category: CategoryFullBody,
				},
				Sets: []Set{
					{
						WeightKg:         20, //nolint:mnd // 20kg barbell
						AdjustedWeightKg: 20, //nolint:mnd // 20kg barbell
						MinReps:          8,  //nolint:mnd // 8 reps
						MaxReps:          12, //nolint:mnd // 12 reps
						CompletedReps:    nil,
					},
				},
			},
		},
	}, nil
}

// ResolveWeeklySchedule retrieves the workout schedule for a week.
func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]Session, error) {
	//nolint:godox // temporary todo
	// TODO: Implement weekly schedule retrieval
	// This should:
	// 1. Get all sessions for the week
	// 2. Fill in rest days and planned workouts based on preferences
	// 3. Return complete 7-day schedule
	workouts := make([]Session, 7) //nolint:mnd // 7 days in a week
	// Get the current date
	now := time.Now()

	// Calculate the current week's Monday
	// Weekday() returns the day of the week with Sunday as 0
	// We need to adjust this to 1-based with Monday as 1
	offset := int(time.Monday - now.Weekday())
	if offset > 0 {
		offset = -6 //nolint:mnd // If today is Sunday, adjust the offset to get last Monday
	}
	monday := now.AddDate(0, 0, offset)

	// Generate dates from Monday to Sunday
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		workout, err := s.generateWorkout(ctx, day)
		if err != nil {
			return nil, errors.Wrap(err, "generate workout")
		}
		workouts[i] = workout
	}

	return workouts, nil
}

// GetSession retrieves a workout session for a specific date.
func (s *Service) GetSession(ctx context.Context, date time.Time) (Session, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	// First check if there's an existing session
	var (
		session                      Session
		startedAtStr, completedAtStr sql.NullString
		workoutDateStr               string
	)
	err := s.db.ReadOnly.QueryRowContext(ctx, `
        SELECT workout_date, difficulty_rating, started_at, completed_at
        FROM workout_sessions 
        WHERE user_id = ? AND workout_date = ?`,
		userID, date.Format("2006-01-02")).
		Scan(&workoutDateStr, &session.DifficultyRating, &startedAtStr, &completedAtStr)

	if errors.Is(err, sql.ErrNoRows) {
		// If no session exists, generate a new one
		return s.generateWorkout(ctx, date)
	}
	if err != nil {
		return Session{}, errors.Wrap(err, "query workout session")
	}
	// Parse timestamps
	session.WorkoutDate = date // Use the input date since we know it matches

	var startedAt, completedAt *time.Time
	if startedAt, err = parseTimestamp(startedAtStr); err != nil {
		return Session{}, errors.Wrap(err, "parse started_at")
	}
	session.StartedAt = startedAt

	if completedAt, err = parseTimestamp(completedAtStr); err != nil {
		return Session{}, errors.Wrap(err, "parse completed_at")
	}
	session.CompletedAt = completedAt

	// Load exercise sets
	rows, err := s.db.ReadOnly.QueryContext(ctx, `
        SELECT e.id, e.name, e.category, 
               es.set_number, es.weight_kg, es.adjusted_weight_kg,
               es.min_reps, es.max_reps, es.completed_reps
        FROM exercise_sets es
        JOIN exercises e ON e.id = es.exercise_id
        WHERE es.workout_user_id = ? AND es.workout_date = ?
        ORDER BY es.exercise_id, es.set_number`,
		userID, date.Format("2006-01-02"))
	if err != nil {
		return Session{}, errors.Wrap(err, "query exercise sets")
	}
	defer rows.Close()

	var currentExercise *ExerciseSet
	for rows.Next() {
		var (
			exercise Exercise
			set      Set
			setNum   int
		)

		err = rows.Scan(
			&exercise.ID, &exercise.Name, &exercise.Category,
			&setNum, &set.WeightKg, &set.AdjustedWeightKg,
			&set.MinReps, &set.MaxReps, &set.CompletedReps)
		if err != nil {
			return Session{}, errors.Wrap(err, "scan exercise set")
		}

		// If this is a new exercise or the first one
		if currentExercise == nil || currentExercise.Exercise.ID != exercise.ID {
			if currentExercise != nil {
				session.ExerciseSets = append(session.ExerciseSets, *currentExercise)
			}
			currentExercise = &ExerciseSet{
				Exercise: exercise,
				Sets:     []Set{},
			}
		}

		currentExercise.Sets = append(currentExercise.Sets, set)
	}

	// Add the last exercise if it exists
	if currentExercise != nil {
		session.ExerciseSets = append(session.ExerciseSets, *currentExercise)
	}

	if err = rows.Err(); err != nil {
		return Session{}, errors.Wrap(err, "rows error")
	}

	// Determine status
	if session.CompletedAt != nil {
		session.Status = StatusDone
	} else {
		session.Status = StatusPlanned
	}

	return session, nil
}

func parseTimestamp(timestampStr sql.NullString) (*time.Time, error) {
	if timestampStr.Valid {
		parsedTime, err := time.Parse(time.RFC3339, timestampStr.String)
		if err != nil {
			return nil, errors.Wrap(err, "parse RFC3339")
		}
		return &parsedTime, nil
	}
	return nil, nil //nolint:nilnil// Return nil for null timestamps
}

// StartSession starts a new workout session or returns an error if one already exists.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := date.Format("2006-01-02")
	startedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// Start a transaction since we need to insert multiple rows
	tx, err := s.db.ReadWrite.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}
	defer func(tx *sql.Tx) {
		err = tx.Rollback()
		if err != nil {
			s.logger.LogAttrs(ctx, slog.LevelError, "rollback transaction", errors.SlogError(err))
		}
	}(tx)

	// First create the session
	_, err = tx.ExecContext(ctx, `
        INSERT INTO workout_sessions (user_id, workout_date, started_at)
        VALUES (?, ?, ?)
        ON CONFLICT (user_id, workout_date) DO UPDATE SET
            started_at = COALESCE(workout_sessions.started_at, ?)`,
		userID, dateStr, startedAt, startedAt)
	if err != nil {
		return errors.Wrap(err, "insert workout session")
	}

	// Generate workout if it doesn't exist
	session, err := s.generateWorkout(ctx, date)
	if err != nil {
		return errors.Wrap(err, "generate workout")
	}

	// Insert exercise sets
	for _, exerciseSet := range session.ExerciseSets {
		for i, set := range exerciseSet.Sets {
			_, err = tx.ExecContext(ctx, `
                INSERT INTO exercise_sets (
                    workout_user_id, workout_date, exercise_id, set_number,
                    weight_kg, adjusted_weight_kg, min_reps, max_reps
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT (workout_user_id, workout_date, exercise_id, set_number) DO NOTHING`,
				userID, dateStr, exerciseSet.Exercise.ID, i+1,
				set.WeightKg, set.AdjustedWeightKg, set.MinReps, set.MaxReps)
			if err != nil {
				return errors.Wrap(err, "insert exercise set")
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return errors.Wrap(err, "commit transaction")
	}

	return nil
}

// CompleteSession marks a workout session as completed.
func (s *Service) CompleteSession(ctx context.Context, date time.Time) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	completedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	result, err := s.db.ReadWrite.ExecContext(ctx, `
        UPDATE workout_sessions 
        SET completed_at = ?
        WHERE user_id = ? AND workout_date = ? AND completed_at IS NULL`,
		completedAt, userID, date.Format("2006-01-02"))
	if err != nil {
		return errors.Wrap(err, "complete workout session")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected")
	}
	if rows == 0 {
		return errors.New("workout session not found or already completed")
	}

	return nil
}

// SaveFeedback saves the difficulty rating for a completed workout session.
func (s *Service) SaveFeedback(ctx context.Context, date time.Time, difficulty int) error {
	if difficulty < 1 || difficulty > 5 {
		return errors.New("invalid difficulty rating",
			slog.Int("difficulty", difficulty),
			slog.String("date", date.Format("2006-01-02")))
	}

	userID := contexthelpers.AuthenticatedUserID(ctx)
	result, err := s.db.ReadWrite.ExecContext(ctx, `
		SELECT * FROM workout_sessions
        WHERE user_id = ? AND workout_date = ?`,
		difficulty, userID, date.Format("2006-01-02"))
	if err != nil {
		return errors.Wrap(err, "save difficulty rating")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected")
	}
	if rows == 0 {
		return errors.New("workout session not found or not completed")
	}

	return nil
}

// UpdateSetWeight updates the weight for a specific set in a workout.
func (s *Service) UpdateSetWeight(
	ctx context.Context,
	date time.Time,
	exerciseID int,
	setIndex int,
	newWeight float64,
) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := date.Format("2006-01-02")

	result, err := s.db.ReadWrite.ExecContext(ctx, `
        UPDATE exercise_sets 
        SET weight_kg = ?,
            adjusted_weight_kg = ?
        WHERE workout_user_id = ? 
        AND workout_date = ? 
        AND exercise_id = ?
        AND set_number = ?`,
		newWeight, newWeight, userID, dateStr, exerciseID, setIndex+1)
	if err != nil {
		return errors.Wrap(err, "UPDATE set weight")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected")
	}
	if rows == 0 {
		return errors.New("set not found")
	}

	return nil
}

// CompleteSet marks a specific set as completed with the given number of reps.
func (s *Service) CompleteSet(
	ctx context.Context,
	date time.Time,
	exerciseID int,
	setIndex int,
	completedReps int,
) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := date.Format("2006-01-02")

	// First verify the reps are within the target range
	var minReps, maxReps int
	err := s.db.ReadOnly.QueryRowContext(ctx, `
        SELECT min_reps, max_reps
        FROM exercise_sets
        WHERE workout_user_id = ?
        AND workout_date = ?
        AND exercise_id = ?
        AND set_number = ?`,
		userID, dateStr, exerciseID, setIndex+1).Scan(&minReps, &maxReps)
	if err != nil {
		return errors.Wrap(err, "get set rep range")
	}

	// Allow completing with reps outside the target range, but log it
	if completedReps < minReps || completedReps > maxReps {
		s.logger.LogAttrs(ctx, slog.LevelInfo, "completed reps outside target range",
			slog.Int("completed_reps", completedReps),
			slog.Int("min_reps", minReps),
			slog.Int("max_reps", maxReps))
	}

	result, err := s.db.ReadWrite.ExecContext(ctx, `
        UPDATE exercise_sets
        SET completed_reps = ?
        WHERE workout_user_id = ?
        AND workout_date = ?
        AND exercise_id = ?
        AND set_number = ?`,
		completedReps, userID, dateStr, exerciseID, setIndex+1)
	if err != nil {
		return errors.Wrap(err, "complete set")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected")
	}
	if rows == 0 {
		return errors.New("set not found")
	}

	return nil
}

// UpdateCompletedReps updates a previously completed set with new rep count.
func (s *Service) UpdateCompletedReps(
	ctx context.Context,
	date time.Time,
	exerciseID int,
	setIndex int,
	completedReps int,
) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	dateStr := date.Format("2006-01-02")

	// First verify the set exists and is already completed
	var minReps, maxReps int
	var currentReps sql.NullInt64
	err := s.db.ReadOnly.QueryRowContext(ctx, `
        SELECT min_reps, max_reps, completed_reps
        FROM exercise_sets
        WHERE workout_user_id = ?
        AND workout_date = ?
        AND exercise_id = ?
        AND set_number = ?`,
		userID, dateStr, exerciseID, setIndex+1).Scan(&minReps, &maxReps, &currentReps)
	if err != nil {
		return errors.Wrap(err, "get set rep range")
	}

	// Allow updating with reps outside the target range, but log it
	if completedReps < minReps || completedReps > maxReps {
		s.logger.LogAttrs(ctx, slog.LevelInfo, "updated reps outside target range",
			slog.Int("completed_reps", completedReps),
			slog.Int("min_reps", minReps),
			slog.Int("max_reps", maxReps))
	}

	result, err := s.db.ReadWrite.ExecContext(ctx, `
        UPDATE exercise_sets
        SET completed_reps = ?
        WHERE workout_user_id = ?
        AND workout_date = ?
        AND exercise_id = ?
        AND set_number = ?`,
		completedReps, userID, dateStr, exerciseID, setIndex+1)
	if err != nil {
		return errors.Wrap(err, "update completed reps")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "get rows affected")
	}
	if rows == 0 {
		return errors.New("set not found")
	}

	return nil
}
