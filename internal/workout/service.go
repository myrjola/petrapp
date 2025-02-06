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
	// TODO: Implement smart workout generation logic
	// This should:
	// 1. Check if it's a workout day based on preferences
	// 2. Determine workout type (full body vs split) based on consecutive days
	// 3. Select appropriate exercises
	// 4. Calculate proper sets/reps/weights based on history
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
						WeightKg:         20,
						AdjustedWeightKg: 20,
						MinReps:          8,
						MaxReps:          12,
						CompletedReps:    nil,
					},
				},
			},
		},
	}, nil
}

// ResolveWeeklySchedule retrieves the workout schedule for a week.
func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]Session, error) {
	// TODO: Implement weekly schedule retrieval
	// This should:
	// 1. Get all sessions for the week
	// 2. Fill in rest days and planned workouts based on preferences
	// 3. Return complete 7-day schedule
	workouts := make([]Session, 7)
	// Get the current date
	now := time.Now()

	// Calculate the current week's Monday
	// Weekday() returns the day of the week with Sunday as 0
	// We need to adjust this to 1-based with Monday as 1
	offset := int(time.Monday - now.Weekday())
	if offset > 0 {
		offset = -6 // If today is Sunday, adjust the offset to get last Monday
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
	var session Session
	err := s.db.ReadOnly.QueryRowContext(ctx, `
        SELECT workout_date, difficulty_rating, started_at, completed_at
        FROM workout_sessions 
        WHERE user_id = ? AND workout_date = ?`,
		userID, date.Format("2006-01-02")).
		Scan(&session.WorkoutDate, &session.DifficultyRating, &session.StartedAt, &session.CompletedAt)

	if errors.Is(err, sql.ErrNoRows) {
		// If no session exists, generate a new one
		return s.generateWorkout(ctx, date)
	}
	if err != nil {
		return Session{}, errors.Wrap(err, "query workout session")
	}

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

		err := rows.Scan(
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
