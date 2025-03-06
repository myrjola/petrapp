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

// Service handles the business logic for workout management.
type Service struct {
	repo   *sqliteRepository
	logger *slog.Logger
}

// NewService creates a new workout service with SQLite repository.
func NewService(db *sqlite.Database, logger *slog.Logger) *Service {
	return &Service{
		repo:   newSQLiteRepository(db, logger),
		logger: logger,
	}
}

// GetUserPreferences retrieves the workout preferences for a user.
func (s *Service) GetUserPreferences(ctx context.Context) (Preferences, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	prefs, err := s.repo.getUserPreferences(ctx, userID)
	if err != nil {
		return Preferences{}, fmt.Errorf("get user preferences: %w", err)
	}
	return prefs, nil
}

// SaveUserPreferences saves the workout preferences for a user.
func (s *Service) SaveUserPreferences(ctx context.Context, prefs Preferences) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if err := s.repo.saveUserPreferences(ctx, userID, prefs); err != nil {
		return fmt.Errorf("save user preferences: %w", err)
	}
	return nil
}

// generateWorkout creates a new workout plan based on user preferences and history.
func (s *Service) generateWorkout(ctx context.Context, date time.Time) (Session, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	// Get user preferences
	prefs, err := s.repo.getUserPreferences(ctx, userID)
	if err != nil {
		return Session{}, fmt.Errorf("get user preferences: %w", err)
	}

	// Get workout history (past 3 months)
	threeMonthsAgo := date.AddDate(0, -3, 0)
	history, err := s.repo.getWorkoutHistory(ctx, userID, threeMonthsAgo, date)
	if err != nil {
		return Session{}, fmt.Errorf("get workout history: %w", err)
	}

	// Get exercise pool
	exercisePool, err := s.repo.getExercisePool(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("get exercise pool: %w", err)
	}

	// If exercise pool is empty, create a session with a friendly error
	if len(exercisePool) == 0 {
		return Session{
			WorkoutDate:      date,
			DifficultyRating: nil,
			StartedAt:        nil,
			CompletedAt:      nil,
			ExerciseSets:     []ExerciseSet{},
			Status:           StatusPlanned,
		}, nil
	}

	// Initialize workout generator
	gen, err := NewGenerator(prefs, history, exercisePool)
	if err != nil {
		return Session{}, fmt.Errorf("initialize workout generator: %w", err)
	}

	// Generate the workout
	session, err := gen.Generate(date)
	if err != nil {
		return Session{}, fmt.Errorf("generate workout: %w", err)
	}

	return session, nil
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
			return nil, fmt.Errorf("generate workout: %w", err)
		}
		workouts[i] = workout
	}

	return workouts, nil
}

// GetSession retrieves a workout session for a specific date.
func (s *Service) GetSession(ctx context.Context, date time.Time) (Session, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	session, err := s.repo.getSession(ctx, userID, date)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// If no session exists, generate a new one
			return s.generateWorkout(ctx, date)
		}
		return Session{}, fmt.Errorf("get session: %w", err)
	}

	return session, nil
}

// StartSession starts a new workout session or returns an error if one already exists.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	if err := s.repo.startSession(ctx, userID, date); err != nil {
		return fmt.Errorf("start session: %w", err)
	}

	// Generate workout if it doesn't exist
	session, err := s.generateWorkout(ctx, date)
	if err != nil {
		return fmt.Errorf("generate workout: %w", err)
	}

	// Save the generated exercise sets to the database
	if err = s.repo.saveExerciseSets(ctx, userID, date, session.ExerciseSets); err != nil {
		return fmt.Errorf("save exercise sets: %w", err)
	}

	return nil
}

// CompleteSession marks a workout session as completed.
func (s *Service) CompleteSession(ctx context.Context, date time.Time) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if err := s.repo.completeSession(ctx, userID, date); err != nil {
		return fmt.Errorf("complete session: %w", err)
	}
	return nil
}

// SaveFeedback saves the difficulty rating for a completed workout session.
func (s *Service) SaveFeedback(ctx context.Context, date time.Time, difficulty int) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if err := s.repo.saveFeedback(ctx, userID, date, difficulty); err != nil {
		return fmt.Errorf("save feedback: %w", err)
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
	if err := s.repo.updateSetWeight(ctx, userID, date, exerciseID, setIndex, newWeight); err != nil {
		return fmt.Errorf("set weight: %w", err)
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
	if err := s.repo.updateCompletedReps(ctx, userID, date, exerciseID, setIndex, completedReps); err != nil {
		return fmt.Errorf("update completed reps: %w", err)
	}
	return nil
}
