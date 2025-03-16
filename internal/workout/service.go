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
	repo         *repository
	logger       *slog.Logger
	openaiAPIKey string
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	factory := newRepositoryFactory(db, logger)
	return &Service{
		repo:         factory.newRepository(),
		logger:       logger,
		openaiAPIKey: openaiAPIKey,
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
func (s *Service) generateWorkout(ctx context.Context, date time.Time) (sessionAggregate, error) {
	// Get user preferences.
	prefs, err := s.repo.prefs.Get(ctx)
	if err != nil {
		return sessionAggregate{}, fmt.Errorf("get user preferences: %w", err)
	}

	// Get workout history (past 3 months).
	threeMonthsAgo := date.AddDate(0, -3, 0)
	history, err := s.repo.sessions.List(ctx, threeMonthsAgo)
	if err != nil {
		return sessionAggregate{}, fmt.Errorf("get workout history: %w", err)
	}

	// Get exercise pool.
	exercisePool, err := s.repo.exercises.List(ctx)
	if err != nil {
		return sessionAggregate{}, fmt.Errorf("get exercise pool: %w", err)
	}

	// Initialize workout generator.
	gen, err := newGenerator(prefs, history, exercisePool)
	if err != nil {
		return sessionAggregate{}, fmt.Errorf("initialize workout generator: %w", err)
	}

	// Generate the workout.
	session, err := gen.Generate(date)
	if err != nil {
		return sessionAggregate{}, fmt.Errorf("generate workout: %w", err)
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

		workouts[i], err = s.enrichSessionAggregate(ctx, workout)
		if err != nil {
			return nil, fmt.Errorf("enrich workout %s: %w", formatDate(day), err)
		}
	}

	return workouts, nil
}

// GetSession retrieves a workout session for a specific date.
func (s *Service) GetSession(ctx context.Context, date time.Time) (Session, error) {
	sessionAggr, err := s.repo.sessions.Get(ctx, date)
	if err != nil {
		return Session{}, fmt.Errorf("get session %s: %w", formatDate(date), err)
	}

	var session Session
	session, err = s.enrichSessionAggregate(ctx, sessionAggr)
	if err != nil {
		return Session{}, fmt.Errorf("enrich session %s: %w", formatDate(date), err)
	}

	return session, nil
}

func (s *Service) enrichSessionAggregate(ctx context.Context, sessionAggr sessionAggregate) (Session, error) {
	session := Session{
		Date:             sessionAggr.Date,
		StartedAt:        sessionAggr.StartedAt,
		CompletedAt:      sessionAggr.CompletedAt,
		DifficultyRating: sessionAggr.DifficultyRating,
		ExerciseSets:     make([]ExerciseSet, len(sessionAggr.ExerciseSets)),
	}

	for i, ex := range sessionAggr.ExerciseSets {
		exercise, err := s.repo.exercises.Get(ctx, ex.ExerciseID)
		if err != nil {
			return Session{}, fmt.Errorf("get exercise %d: %w", ex.ExerciseID, err)
		}
		session.ExerciseSets[i].Exercise = exercise
		session.ExerciseSets[i].Sets = ex.Sets
	}
	return session, nil
}

// StartSession starts a new workout session.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	// Generate workout if it doesn't exist
	_, err := s.repo.sessions.Get(ctx, date)
	if errors.Is(err, ErrNotFound) {
		var sess sessionAggregate
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

	if err = s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
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
	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		sess.CompletedAt = time.Now()
		return true, nil
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
	}

	return nil
}

// SaveFeedback saves the difficulty rating for a completed workout session.
func (s *Service) SaveFeedback(ctx context.Context, date time.Time, difficulty int) error {
	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
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
	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		for _, ex := range sess.ExerciseSets {
			if ex.ExerciseID == exerciseID {
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
	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		for _, ex := range sess.ExerciseSets {
			if ex.ExerciseID == exerciseID {
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

// GetExercise retrieves a specific exercise by ID.
func (s *Service) GetExercise(ctx context.Context, id int) (Exercise, error) {
	exercise, err := s.repo.exercises.Get(ctx, id)
	if err != nil {
		return Exercise{}, fmt.Errorf("get exercise: %w", err)
	}
	return exercise, nil
}

// UpdateExercise updates an existing exercise.
func (s *Service) UpdateExercise(ctx context.Context, ex Exercise) error {
	if err := s.repo.exercises.Update(ctx, ex.ID, func(oldEx *Exercise) (bool, error) {
		*oldEx = ex
		return true, nil
	}); err != nil {
		return fmt.Errorf("update exercise: %w", err)
	}
	return nil
}

// ListMuscleGroups retrieves all available muscle groups.
func (s *Service) ListMuscleGroups(ctx context.Context) ([]string, error) {
	groups, err := s.repo.exercises.ListMuscleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle groups: %w", err)
	}
	return groups, nil
}

// GenerateExercise generates a new exercise based on a name.
//
// In case of errors, it strives to persist a minimal exercise that the user can fill in later. In this case, no error
// is returned, but the exercise is guaranteed to have Name and ID fields set.
func (s *Service) GenerateExercise(ctx context.Context, name string) (Exercise, error) {
	// Start with a minimal exercise in case generation fails
	exercise := Exercise{
		Name:                  name,
		Category:              CategoryFullBody,
		DescriptionMarkdown:   fmt.Sprintf("# %s\n\nNo description available yet.", name),
		PrimaryMuscleGroups:   []string{},
		SecondaryMuscleGroups: []string{},
	}

	// Try to generate a better exercise if possible
	if s.openaiAPIKey != "" {
		muscleGroups, err := s.repo.exercises.ListMuscleGroups(ctx)
		if err == nil {
			generator := newExerciseGenerator(s.openaiAPIKey, muscleGroups)
			generatedExercise, err := generator.Generate(ctx, name)
			if err == nil {
				exercise = generatedExercise
			} else {
				s.logger.Warn("Failed to generate exercise details", "error", err, "name", name)
				// Fall back to minimal exercise (already set)
			}
		} else {
			s.logger.Warn("Failed to get muscle groups", "error", err)
			// Fall back to minimal exercise (already set)
		}
	}

	// Persist the exercise
	var err error
	exercise, err = s.repo.exercises.Create(ctx, exercise)
	if err != nil {
		return Exercise{}, fmt.Errorf("create exercise: %w", err)
	}
	return exercise, nil
}
