package workout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/sqlite"
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

		// Try to get existing session first
		sessionAggr, err := s.repo.sessions.Get(ctx, day)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("get session %s: %w", formatDate(day), err)
		}

		// If no existing session, generate a new one
		if errors.Is(err, ErrNotFound) {
			sessionAggr, err = s.generateWorkout(ctx, day)
			if err != nil {
				return nil, fmt.Errorf("generate workout %s: %w", formatDate(day), err)
			}
		}

		workouts[i], err = s.enrichSessionAggregate(ctx, sessionAggr)
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
		session.ExerciseSets[i].WarmupCompletedAt = ex.WarmupCompletedAt
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
				ex.Sets[setIndex].WeightKg = &newWeight
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
				now := time.Now().UTC()
				ex.Sets[setIndex].CompletedReps = &completedReps
				ex.Sets[setIndex].CompletedAt = &now
				return true, nil
			}
		}
		return false, errors.New("exercise not found")
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
	}

	return nil
}

// MarkWarmupComplete marks the warmup as complete for a specific exercise.
func (s *Service) MarkWarmupComplete(
	ctx context.Context,
	date time.Time,
	exerciseID int,
) error {
	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		for i := range sess.ExerciseSets {
			if sess.ExerciseSets[i].ExerciseID == exerciseID {
				now := time.Now().UTC()
				sess.ExerciseSets[i].WarmupCompletedAt = &now
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

// GetSessionsWithExerciseSince retrieves all sessions since a given date that contain the specified exercise.
func (s *Service) GetSessionsWithExerciseSince(ctx context.Context, exerciseID int, since time.Time) (
	[]Session, error) {
	sessions, err := s.repo.sessions.List(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}

	// Filter sessions that contain the specified exercise
	var result []Session
	for _, session := range sessions {
		// Check if this session contains the exercise
		hasExercise := false
		for _, es := range session.ExerciseSets {
			if es.ExerciseID == exerciseID {
				hasExercise = true
				break
			}
		}

		if hasExercise {
			// Convert sessionAggregate to Session by enriching with exercise data
			enrichedSession := Session{
				Date:             session.Date,
				DifficultyRating: session.DifficultyRating,
				StartedAt:        session.StartedAt,
				CompletedAt:      session.CompletedAt,
				ExerciseSets:     make([]ExerciseSet, len(session.ExerciseSets)),
			}

			// Enrich exercise sets with exercise data
			for i, es := range session.ExerciseSets {
				ex, getErr := s.repo.exercises.Get(ctx, es.ExerciseID)
				if getErr != nil {
					return nil, fmt.Errorf("get exercise %d: %w", es.ExerciseID, getErr)
				}

				enrichedSession.ExerciseSets[i] = ExerciseSet{
					Exercise:          ex,
					Sets:              es.Sets,
					WarmupCompletedAt: es.WarmupCompletedAt,
				}
			}

			result = append(result, enrichedSession)
		}
	}

	return result, nil
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
// In case of errors, it persists a minimal exercise that the user can fill in later.
// The returned exercise is guaranteed to have at least Name and ID fields set.
func (s *Service) GenerateExercise(ctx context.Context, name string) (Exercise, error) {
	// Generate exercise content
	exercise := s.generateExerciseContent(ctx, name)

	// Persist the exercise
	persisted, err := s.repo.exercises.Create(ctx, exercise)
	if err != nil {
		return Exercise{}, fmt.Errorf("create exercise: %w", err)
	}

	return persisted, nil
}

// generateExerciseContent creates exercise content, using AI generation if available
// or falling back to minimal content if not possible.
func (s *Service) generateExerciseContent(ctx context.Context, name string) Exercise {
	// Use minimal exercise if no OpenAI API key is configured
	if s.openaiAPIKey == "" {
		return createMinimalExercise(name)
	}

	// Try to get muscle groups for better generation
	muscleGroups, err := s.repo.exercises.ListMuscleGroups(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to get muscle groups", slog.Any("error", err))
		return createMinimalExercise(name)
	}

	// Try to generate a better exercise with AI
	generator := newExerciseGenerator(s.openaiAPIKey, muscleGroups)
	generated, err := generator.Generate(ctx, name)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to generate exercise details",
			slog.Any("error", err), slog.String("name", name))
		return createMinimalExercise(name)
	}

	return generated
}

// createMinimalExercise returns a basic exercise with just the essential fields populated.
func createMinimalExercise(name string) Exercise {
	return Exercise{
		ID:                    -1,
		Name:                  name,
		Category:              CategoryFullBody,
		ExerciseType:          ExerciseTypeWeighted,
		DescriptionMarkdown:   fmt.Sprintf("# %s\n\nNo description available yet.", name),
		PrimaryMuscleGroups:   []string{},
		SecondaryMuscleGroups: []string{},
	}
}

// SwapExercise replaces an exercise in a workout with another exercise.
// It retrieves weights from the previous time the new exercise was used.
func (s *Service) SwapExercise(
	ctx context.Context,
	date time.Time,
	currentExerciseID int,
	newExerciseID int,
) error {
	// 1. Validate both exercises exist
	if err := s.validateExercises(ctx, currentExerciseID, newExerciseID); err != nil {
		return err
	}

	// 2. Find historical data for the new exercise
	historicalSets, err := s.findHistoricalSets(ctx, date, newExerciseID)
	if err != nil {
		return fmt.Errorf("find historical sets: %w", err)
	}

	// 3. Update the session with the new exercise
	if err = s.replaceExerciseInSession(ctx, date, currentExerciseID, newExerciseID, historicalSets); err != nil {
		return err
	}

	return nil
}

// validateExercises checks if both exercises exist in the repository.
func (s *Service) validateExercises(ctx context.Context, currentID, newID int) error {
	_, err := s.repo.exercises.Get(ctx, currentID)
	if err != nil {
		return fmt.Errorf("get current exercise: %w", err)
	}

	_, err = s.repo.exercises.Get(ctx, newID)
	if err != nil {
		return fmt.Errorf("get new exercise: %w", err)
	}

	return nil
}

// findHistoricalSets retrieves set data from the most recent usage of an exercise.
func (s *Service) findHistoricalSets(ctx context.Context, date time.Time, exerciseID int) ([]Set, error) {
	// Get workout history (past 3 months)
	threeMonthsAgo := date.AddDate(0, -3, 0)
	history, err := s.repo.sessions.List(ctx, threeMonthsAgo)
	if err != nil {
		return nil, fmt.Errorf("get workout history: %w", err)
	}

	// Look for the most recent usage of the exercise
	for i := len(history) - 1; i >= 0; i-- {
		session := history[i]
		// Skip the current date's session
		if session.Date.Equal(date) {
			continue
		}

		for _, exerciseSet := range session.ExerciseSets {
			if exerciseSet.ExerciseID == exerciseID {
				// Copy sets and reset completion status
				return s.copySetsWithoutCompletion(exerciseSet.Sets), nil
			}
		}
	}

	// No historical data found
	return nil, nil
}

// copySetsWithoutCompletion creates a copy of sets with completed reps reset to nil.
func (s *Service) copySetsWithoutCompletion(sets []Set) []Set {
	result := make([]Set, len(sets))
	for i, set := range sets {
		result[i] = Set{
			WeightKg:      set.WeightKg,
			MinReps:       set.MinReps,
			MaxReps:       set.MaxReps,
			CompletedReps: nil, // Reset completion status
			CompletedAt:   nil,
		}
	}
	return result
}

// createEmptySets creates new sets with zero weight based on the structure of template sets.
func (s *Service) createEmptySets(templateSets []Set) []Set {
	result := make([]Set, len(templateSets))
	for i, set := range templateSets {
		var weight *float64
		if set.WeightKg != nil {
			weight = &[]float64{0}[0] // Empty weight for weighted exercises
		}
		result[i] = Set{
			WeightKg:      weight,
			MinReps:       set.MinReps,
			MaxReps:       set.MaxReps,
			CompletedReps: nil,
			CompletedAt:   nil,
		}
	}
	return result
}

// replaceExerciseInSession updates a session by replacing one exercise with another.
func (s *Service) replaceExerciseInSession(
	ctx context.Context,
	date time.Time,
	currentExerciseID int,
	newExerciseID int,
	historicalSets []Set,
) error {
	err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		// Find the exercise set to replace
		for i, exerciseSet := range sess.ExerciseSets {
			if exerciseSet.ExerciseID == currentExerciseID {
				// Replace the exercise ID
				sess.ExerciseSets[i].ExerciseID = newExerciseID

				// Replace sets with historical ones or empty sets
				if historicalSets != nil {
					sess.ExerciseSets[i].Sets = historicalSets
				} else {
					sess.ExerciseSets[i].Sets = s.createEmptySets(exerciseSet.Sets)
				}

				return true, nil
			}
		}

		return false, fmt.Errorf("exercise %d not found in workout for date %s",
			currentExerciseID, formatDate(date))
	})
	if err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
	}
	return nil
}

// FindCompatibleExercises returns all exercises except the specified one.
func (s *Service) FindCompatibleExercises(ctx context.Context, exerciseID int) ([]Exercise, error) {
	// Get all exercises
	allExercises, err := s.repo.exercises.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all exercises: %w", err)
	}

	// Filter out the current exercise
	var otherExercises []Exercise
	for _, exercise := range allExercises {
		if exercise.ID != exerciseID {
			otherExercises = append(otherExercises, exercise)
		}
	}

	return otherExercises, nil
}

// Default rep ranges.
const (
	defaultMinReps = 8
	defaultMaxReps = 12
)

// AddExercise adds a new exercise to an existing workout session.
// It will retrieve historical weight data if available.
func (s *Service) AddExercise(ctx context.Context, date time.Time, exerciseID int) error {
	// 1. Validate the exercise exists
	exercise, err := s.repo.exercises.Get(ctx, exerciseID)
	if err != nil {
		return fmt.Errorf("get exercise: %w", err)
	}

	// 2. Find historical data for the exercise
	historicalSets, err := s.findHistoricalSets(ctx, date, exerciseID)
	if err != nil {
		return fmt.Errorf("find historical sets: %w", err)
	}

	// 3. Check if the workout session exists
	_, err = s.repo.sessions.Get(ctx, date)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("workout session for date %s does not exist", formatDate(date))
	} else if err != nil {
		return fmt.Errorf("check session existence: %w", err)
	}

	// 4. Update the session to add the new exercise
	err = s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		// Check if the exercise already exists in the session
		for _, existingExercise := range sess.ExerciseSets {
			if existingExercise.ExerciseID == exerciseID {
				return false, fmt.Errorf("exercise %s already exists in workout for date %s",
					exercise.Name, formatDate(date))
			}
		}

		// Create sets for the exercise
		var newSets []Set
		if historicalSets != nil {
			// Use historical sets if available
			newSets = historicalSets
		} else {
			// Create default sets if no historical data exists
			newSets = []Set{
				{
					WeightKg:      &[]float64{0}[0],
					MinReps:       defaultMinReps,
					MaxReps:       defaultMaxReps,
					CompletedReps: nil,
					CompletedAt:   nil,
				},
				{
					WeightKg:      &[]float64{0}[0],
					MinReps:       defaultMinReps,
					MaxReps:       defaultMaxReps,
					CompletedReps: nil,
					CompletedAt:   nil,
				},
				{
					WeightKg:      &[]float64{0}[0],
					MinReps:       defaultMinReps,
					MaxReps:       defaultMaxReps,
					CompletedReps: nil,
					CompletedAt:   nil,
				},
			}
		}

		// Add the new exercise to the session
		newExerciseSet := exerciseSetAggregate{
			ExerciseID:        exerciseID,
			Sets:              newSets,
			WarmupCompletedAt: nil,
		}

		sess.ExerciseSets = append(sess.ExerciseSets, newExerciseSet)
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("update session with new exercise: %w", err)
	}

	return nil
}

// GetFeatureFlag retrieves a feature flag by name.
func (s *Service) GetFeatureFlag(ctx context.Context, name string) (FeatureFlag, error) {
	flag, err := s.repo.featureFlags.Get(ctx, name)
	if err != nil {
		return FeatureFlag{}, fmt.Errorf("get feature flag %s: %w", name, err)
	}
	return flag, nil
}

// IsMaintenanceModeEnabled checks if maintenance mode is enabled.
func (s *Service) IsMaintenanceModeEnabled(ctx context.Context) bool {
	flag, err := s.repo.featureFlags.Get(ctx, "maintenance_mode")
	if err != nil {
		// If we can't check the flag, assume maintenance is disabled for safety
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to check maintenance mode flag", slog.Any("error", err))
		return false
	}
	return flag.Enabled
}
