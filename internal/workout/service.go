package workout

import (
	"context"
	"errors"
	"fmt"
	"github.com/myrjola/petrapp/internal/sqlite"
	"log/slog"
	"time"
)

// Service handles the business logic for workout management.
type Service struct {
	repo   *Repository
	logger *slog.Logger
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger) *Service {
	factory := NewRepositoryFactory(db, logger)
	return &Service{
		repo:   factory.NewRepository(),
		logger: logger,
	}
}

// GetUserPreferences retrieves the workout preferences for a user.
func (s *Service) GetUserPreferences(ctx context.Context) (Preferences, error) {
	prefs, err := s.repo.prefs.Get(ctx)
	if err != nil {
		return Preferences{}, fmt.Errorf("get user preferences: %w", err)
	}
	return prefs, nil
}

// SaveUserPreferences saves the workout preferences for a user.
func (s *Service) SaveUserPreferences(ctx context.Context, prefs Preferences) error {
	if err := s.repo.prefs.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save user preferences: %w", err)
	}
	return nil
}

// GenerateWorkout creates a new workout plan based on user preferences and history.
func (s *Service) generateWorkout(ctx context.Context, date time.Time) (Session, error) {
	// Get user preferences.
	prefs, err := s.repo.prefs.Get(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("get user preferences: %w", err)
	}

	// Get workout history (past 3 months).
	threeMonthsAgo := date.AddDate(0, -3, 0)
	history, err := s.repo.sessions.List(ctx, threeMonthsAgo)
	if err != nil {
		return Session{}, fmt.Errorf("get workout history: %w", err)
	}

	// Get exercise pool.
	exercisePool, err := s.repo.exercises.List(ctx)
	if err != nil {
		return Session{}, fmt.Errorf("get exercise pool: %w", err)
	}

	// Initialize workout generator.
	gen, err := NewGenerator(prefs, history, exercisePool)
	if err != nil {
		return Session{}, fmt.Errorf("initialize workout generator: %w", err)
	}

	// Generate the workout.
	session, err := gen.Generate(date)
	if err != nil {
		return Session{}, fmt.Errorf("generate workout: %w", err)
	}

	return session, nil
}

// ResolveWeeklySchedule retrieves the workout schedule for a week.
func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]Session, error) {
	workouts := make([]Session, 7) //nolint:mnd // 7 days in a week

	// Get the current date.
	now := time.Now()

	// Calculate the current week's Monday.
	offset := int(time.Monday - now.Weekday())
	if offset > 0 {
		offset = -6 //nolint:mnd // If today is Sunday, adjust the offset to get last Monday.
	}
	monday := now.AddDate(0, 0, offset)

	// Generate dates from Monday to Sunday
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		workout, err := s.generateWorkout(ctx, day)
		if err != nil {
			return nil, fmt.Errorf("generate workout %s: %w", formatDate(day), err)
		}
		workouts[i] = workout
	}

	return workouts, nil
}

// GetSession retrieves a workout session for a specific date.
func (s *Service) GetSession(ctx context.Context, date time.Time) (Session, error) {
	session, err := s.repo.sessions.Get(ctx, date)
	if err != nil {
		return Session{}, fmt.Errorf("get session %s: %w", formatDate(date), err)
	}

	return session, nil
}

// StartSession starts a new workout session.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	// Generate workout if it doesn't exist
	_, err := s.repo.sessions.Get(ctx, date)
	if errors.Is(err, ErrNotFound) {
		var sess Session
		if sess, err = s.generateWorkout(ctx, date); err != nil {
			return fmt.Errorf("generate workout %s: %w", formatDate(date), err)
		}
		if err = s.repo.sessions.Create(ctx, sess); err != nil {
			return fmt.Errorf("create session %s: %w", formatDate(date), err)
		}
	}
	if err != nil {
		return fmt.Errorf("get session %s: %w", formatDate(date), err)
	}

	if err = s.repo.sessions.Update(ctx, date, func(sess *Session) (bool, error) {
		if sess.StartedAt.IsZero() {
			sess.StartedAt = time.Now()
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
	}

	return nil
}

// CompleteSession marks a workout session as completed.
func (s *Service) CompleteSession(ctx context.Context, date time.Time) error {
	if err := s.repo.sessions.Update(ctx, date, func(sess *Session) (bool, error) {
		sess.CompletedAt = time.Now()
		return true, nil
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
	}

	return nil
}

// SaveFeedback saves the difficulty rating for a completed workout session.
func (s *Service) SaveFeedback(ctx context.Context, date time.Time, difficulty int) error {
	if err := s.repo.sessions.Update(ctx, date, func(sess *Session) (bool, error) {
		sess.DifficultyRating = &difficulty
		return true, nil
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
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
	if err := s.repo.sessions.Update(ctx, date, func(sess *Session) (bool, error) {
		for _, ex := range sess.ExerciseSets {
			if ex.Exercise.ID == exerciseID {
				if setIndex >= len(ex.Sets) {
					return false, fmt.Errorf("exercise set index %d out of bounds", setIndex)
				}
				ex.Sets[setIndex].WeightKg = newWeight
				return true, nil
			}
		}
		return false, errors.New("exercise not found")
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
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
	if err := s.repo.sessions.Update(ctx, date, func(sess *Session) (bool, error) {
		for _, ex := range sess.ExerciseSets {
			if ex.Exercise.ID == exerciseID {
				if setIndex >= len(ex.Sets) {
					return false, fmt.Errorf("exercise set index %d out of bounds", setIndex)
				}
				ex.Sets[setIndex].CompletedReps = &completedReps
				return true, nil
			}
		}
		return false, errors.New("exercise not found")
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
	}

	return nil
}

// List returns all available exercises.
func (s *Service) List(ctx context.Context) ([]Exercise, error) {
	exercises, err := s.repo.exercises.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list exercises: %w", err)
	}
	return exercises, nil
}
