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
func (s *Service) GetUserPreferences(ctx context.Context) (WorkoutPreferences, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	// TODO: Implement repository pattern and move SQL to repository
	var prefs WorkoutPreferences
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
		return WorkoutPreferences{
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
		return WorkoutPreferences{}, errors.Wrap(err, "query workout preferences")
	}
	return prefs, nil
}

// SaveUserPreferences saves the workout preferences for a user.
func (s *Service) SaveUserPreferences(ctx context.Context, prefs WorkoutPreferences) error {
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
func (s *Service) generateWorkout(ctx context.Context, date time.Time) (WorkoutSession, error) {
	// TODO: Implement smart workout generation logic
	// This should:
	// 1. Check if it's a workout day based on preferences
	// 2. Determine workout type (full body vs split) based on consecutive days
	// 3. Select appropriate exercises
	// 4. Calculate proper sets/reps/weights based on history
	return WorkoutSession{
		WorkoutDate: date,
		Status:      WorkoutStatusPlanned,
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
					},
				},
			},
		},
	}, nil
}

// ResolveWeeklySchedule retrieves the workout schedule for a week.
func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]WorkoutSession, error) {
	// TODO: Implement weekly schedule retrieval
	// This should:
	// 1. Get all sessions for the week
	// 2. Fill in rest days and planned workouts based on preferences
	// 3. Return complete 7-day schedule
	workouts := make([]WorkoutSession, 7)
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
