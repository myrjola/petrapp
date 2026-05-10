package workout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	repo "github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Service handles the business logic for workout management.
type Service struct {
	repos        *repo.Repositories
	db           *sqlite.Database
	logger       *slog.Logger
	openaiAPIKey string
	gen          *service.Service // delegate for relocated AI-generation methods (Task 2 transitional)
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	return &Service{
		repos:        repo.New(db, logger),
		db:           db,
		logger:       logger,
		openaiAPIKey: openaiAPIKey,
		gen:          service.NewService(db, logger, openaiAPIKey),
	}
}

// GetUserPreferences retrieves the workout preferences for a user.
func (s *Service) GetUserPreferences(ctx context.Context) (Preferences, error) {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return Preferences{}, fmt.Errorf("get user preferences: %w", err)
	}
	return prefs, nil
}

// SaveUserPreferences saves the workout preferences for a user.
func (s *Service) SaveUserPreferences(ctx context.Context, prefs Preferences) error {
	if err := s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save user preferences: %w", err)
	}
	return nil
}

// RegenerateWeeklyPlanIfUnstarted replaces the current week's generated plan with one
// that reflects the latest preferences, but only when no session has been started yet.
// If any workout this week has a non-zero StartedAt the existing plan is left intact.
//
// The delete and generate steps are not wrapped in a single transaction. If the process
// fails between the two, the week is left with no sessions. This is self-healing: the
// next call to ResolveWeeklySchedule (e.g. on the home page redirect) detects zero
// sessions and regenerates automatically.
func (s *Service) RegenerateWeeklyPlanIfUnstarted(ctx context.Context) error {
	monday := mondayOf(time.Now())
	sunday := monday.AddDate(0, 0, 6)

	existing, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return fmt.Errorf("list sessions for week: %w", err)
	}

	// List returns sessions from monday onwards (possibly spanning future weeks);
	// only sessions within this week are relevant for the started check.
	for _, sess := range existing {
		if !sess.Date.After(sunday) && !sess.StartedAt.IsZero() {
			return nil
		}
	}

	if err = s.repos.Sessions.DeleteWeek(ctx, monday); err != nil {
		return fmt.Errorf("delete current week: %w", err)
	}
	if err = s.generateWeeklyPlan(ctx, monday); err != nil {
		return fmt.Errorf("generate weekly plan: %w", err)
	}
	return nil
}

// ResolveWeeklySchedule retrieves the workout schedule for the current week.
// If no sessions exist for the week, it generates all scheduled days at once using
// the weekly planner and persists them in a single transaction.
func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]Session, error) {
	monday := mondayOf(time.Now())
	sunday := monday.AddDate(0, 0, 6)

	existing, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return nil, fmt.Errorf("list sessions for week: %w", err)
	}
	thisWeekCount := 0
	for _, sess := range existing {
		if !sess.Date.After(sunday) {
			thisWeekCount++
		}
	}

	if thisWeekCount == 0 {
		if err = s.generateWeeklyPlan(ctx, monday); err != nil {
			return nil, fmt.Errorf("generate weekly plan: %w", err)
		}
	}

	workouts := make([]Session, 7)
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		sess, getErr := s.repos.Sessions.Get(ctx, day)
		if getErr != nil && !errors.Is(getErr, domain.ErrNotFound) {
			return nil, fmt.Errorf("get session %s: %w", day.Format(time.DateOnly), getErr)
		}
		if errors.Is(getErr, domain.ErrNotFound) {
			workouts[i] = Session{ //nolint:exhaustruct // Rest days have no exercise data.
				Date: day,
			}
			continue
		}
		workouts[i] = sess
	}
	return workouts, nil
}

// generateWeeklyPlan uses the domain planner to create all sessions for the week starting
// on monday and persists them in a single DB transaction.
func (s *Service) generateWeeklyPlan(ctx context.Context, monday time.Time) error {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return fmt.Errorf("get exercises: %w", err)
	}
	targets, err := s.repos.MuscleTargets.List(ctx)
	if err != nil {
		return fmt.Errorf("get muscle group targets: %w", err)
	}

	planner := domain.NewPlanner(prefs, exercises, targets)
	plannedSessions, err := planner.Plan(monday)
	if err != nil {
		return fmt.Errorf("plan week: %w", err)
	}

	if err = s.repos.Sessions.CreateBatch(ctx, plannedSessions); err != nil {
		return fmt.Errorf("create batch sessions: %w", err)
	}
	return nil
}

// GetSession retrieves a workout session for a specific date.
func (s *Service) GetSession(ctx context.Context, date time.Time) (Session, error) {
	sess, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return Session{}, fmt.Errorf("get session %s: %w", date.Format(time.DateOnly), err)
	}
	return sess, nil
}

// mondayOf returns the Monday of the week containing date as midnight UTC. The
// calendar date is taken from date's location so the user's local week boundary
// is preserved, but the result is anchored to UTC so it compares cleanly against
// session dates loaded from the database (which time.Parse always returns in
// UTC). Time.Truncate is unsafe here because it rounds to UTC-midnight
// boundaries from an absolute instant, which can roll local-timezone times back
// into the previous calendar day.
func mondayOf(date time.Time) time.Time {
	y, m, d := date.Date()
	offset := int(time.Monday - date.Weekday())
	if offset > 0 {
		offset = -6
	}
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, offset)
}

// StartSession starts a new workout session.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	monday := mondayOf(date)
	existing, listErr := s.repos.Sessions.List(ctx, monday)
	if listErr != nil {
		return fmt.Errorf("list sessions for week of %s: %w", date.Format(time.DateOnly), listErr)
	}
	sunday := monday.AddDate(0, 0, 6)
	weekCount := 0
	for _, sess := range existing {
		if !sess.Date.After(sunday) {
			weekCount++
		}
	}
	if weekCount == 0 {
		if genErr := s.generateWeeklyPlan(ctx, monday); genErr != nil {
			return fmt.Errorf("generate weekly plan for %s: %w", date.Format(time.DateOnly), genErr)
		}
	}

	err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.Start(time.Now())
	})
	if errors.Is(err, domain.ErrAlreadyStarted) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// CompleteSession marks a workout session as completed.
func (s *Service) CompleteSession(ctx context.Context, date time.Time) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.Complete(time.Now())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// SaveFeedback saves the difficulty rating for a completed workout session.
func (s *Service) SaveFeedback(ctx context.Context, date time.Time, difficulty int) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.SetDifficulty(difficulty)
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// MarkWarmupComplete marks the warmup as complete for a specific workout exercise slot.
func (s *Service) MarkWarmupComplete(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.MarkWarmupComplete(workoutExerciseID, time.Now().UTC())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// UpdateSetWeight updates the weight for a specific set in a workout.
func (s *Service) UpdateSetWeight(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	newWeight float64,
) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.UpdateSetWeight(workoutExerciseID, setIndex, newWeight)
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// UpdateCompletedValue updates a previously completed set with new value (reps or seconds).
func (s *Service) UpdateCompletedValue(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	completedValue int,
) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.UpdateCompletedValue(workoutExerciseID, setIndex, completedValue, time.Now().UTC())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// RecordSet atomically persists the signal, weight (nil for time-based sets),
// completed value (reps or seconds depending on exercise type), and timestamp.
func (s *Service) RecordSet(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	signal Signal,
	weightKg *float64,
	completedValue int,
) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.RecordSet(workoutExerciseID, setIndex, signal, weightKg, completedValue, time.Now().UTC())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// List returns all available exercises.
func (s *Service) List(ctx context.Context) ([]Exercise, error) {
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list exercises: %w", err)
	}
	return exercises, nil
}

// GetExercise retrieves a specific exercise by ID.
func (s *Service) GetExercise(ctx context.Context, id int) (Exercise, error) {
	exercise, err := s.repos.Exercises.Get(ctx, id)
	if err != nil {
		return Exercise{}, fmt.Errorf("get exercise: %w", err)
	}
	return exercise, nil
}

// GetSessionsWithExerciseSince retrieves all sessions since a given date that contain the specified exercise.
func (s *Service) GetSessionsWithExerciseSince(ctx context.Context, exerciseID int, since time.Time) (
	[]Session, error) {
	sessions, err := s.repos.Sessions.List(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}

	var result []Session
	for _, session := range sessions {
		for _, es := range session.ExerciseSets {
			if es.Exercise.ID == exerciseID {
				result = append(result, session)
				break
			}
		}
	}
	return result, nil
}

// GetExerciseSetsForExerciseSince retrieves all sets for a specific exercise since a given date.
func (s *Service) GetExerciseSetsForExerciseSince(ctx context.Context, exerciseID int, since time.Time) (
	ExerciseProgress, error) {
	histories, err := s.repos.Sessions.ListSetsForExerciseSince(ctx, exerciseID, since)
	if err != nil {
		return ExerciseProgress{}, fmt.Errorf("list sets for exercise: %w", err)
	}

	ex, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return ExerciseProgress{}, fmt.Errorf("get exercise %d: %w", exerciseID, err)
	}

	entries := make([]ExerciseProgressEntry, 0, len(histories))
	for _, h := range histories {
		var completedSets []Set
		for _, set := range h.Sets {
			if set.CompletedValue != nil {
				completedSets = append(completedSets, set)
			}
		}
		if len(completedSets) > 0 {
			entries = append(entries, ExerciseProgressEntry{
				Date: h.Date,
				Sets: completedSets,
			})
		}
	}

	return ExerciseProgress{Exercise: ex, Entries: entries}, nil
}

// GetStartingWeight returns the weight to seed a new session for the given exercise.
// It pulls the latest successful set (completed and not signaled too heavy) from
// the most recent qualifying session strictly before beforeDate, then converts the
// load via Epley 1RM-equivalence when that session's periodization differs from
// targetType so the relative intensity carries across rep schemes (e.g. 100 kg x5
// strength → ~92 kg x8 hypertrophy). Using a cutoff keeps the starting weight
// stable when earlier sets of beforeDate's session are edited. Returns 0 if no
// successful history exists.
func (s *Service) GetStartingWeight(
	ctx context.Context,
	exerciseID int,
	beforeDate time.Time,
	targetType PeriodizationType,
) (float64, error) {
	prev, err := s.repos.Sessions.GetLatestStartingWeightBefore(ctx, exerciseID, beforeDate)
	if err != nil {
		return 0, fmt.Errorf("get latest starting weight: %w", err)
	}
	if prev.PeriodizationType == "" || prev.PeriodizationType == targetType {
		return prev.WeightKg, nil
	}
	exercise, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("get exercise for rep window: %w", err)
	}
	if exercise.RepMin == nil || exercise.RepMax == nil {
		// time-based exercises don't carry a rep window and shouldn't reach
		// this path (their starting value is seconds via GetStartingSeconds);
		// defensive return preserves the historical weight unchanged.
		return prev.WeightKg, nil
	}
	fromReps := domain.DeriveScheme(
		*exercise.RepMin, *exercise.RepMax,
		prev.PeriodizationType,
	).TargetReps
	toReps := domain.DeriveScheme(
		*exercise.RepMin, *exercise.RepMax,
		targetType,
	).TargetReps
	return domain.ConvertWeight(prev.WeightKg, fromReps, toReps), nil
}

// GetStartingSeconds returns the seconds target to seed a new session for
// the given time-based exercise. Pulls the latest successful set's
// completed_value from sessions strictly before beforeDate; falls back to
// the exercise's DefaultStartingSeconds when no successful history exists.
// Returns an error if the exercise is not time_based, if the lookup fails,
// or if a time_based exercise has no DefaultStartingSeconds (which is a
// fixture/data invariant violation since the schema CHECK requires it).
func (s *Service) GetStartingSeconds(
	ctx context.Context,
	exerciseID int,
	beforeDate time.Time,
) (int, error) {
	exercise, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("get exercise: %w", err)
	}
	if !exercise.IsTimed() {
		return 0, fmt.Errorf("exercise %d is not time_based", exerciseID)
	}
	seconds, err := s.repos.Sessions.GetLatestSuccessfulSecondsBefore(ctx, exerciseID, beforeDate)
	if err != nil {
		return 0, fmt.Errorf("get latest successful seconds: %w", err)
	}
	if seconds > 0 {
		return seconds, nil
	}
	if exercise.DefaultStartingSeconds == nil {
		return 0, fmt.Errorf("time_based exercise %d has no default_starting_seconds", exerciseID)
	}
	return *exercise.DefaultStartingSeconds, nil
}

// BuildProgression constructs a domain.Progression for the given exercise
// in the given session, ready to call CurrentSet() for the next set recommendation.
func (s *Service) BuildProgression(
	ctx context.Context,
	date time.Time,
	exerciseID int,
) (*domain.Progression, error) {
	sess, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	exercise, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return nil, fmt.Errorf("get exercise: %w", err)
	}
	if exercise.RepMin == nil || exercise.RepMax == nil {
		return nil, fmt.Errorf("exercise %d has no rep window (use BuildTimedProgression for time_based)", exerciseID)
	}

	startingWeight, err := s.GetStartingWeight(ctx, exerciseID, date, sess.PeriodizationType)
	if err != nil {
		return nil, fmt.Errorf("get starting weight: %w", err)
	}

	config := domain.Config{
		Type:           sess.PeriodizationType,
		RepMin:         *exercise.RepMin,
		RepMax:         *exercise.RepMax,
		StartingWeight: startingWeight,
	}

	var completed []domain.SetResult
	for _, es := range sess.ExerciseSets {
		if es.Exercise.ID != exerciseID {
			continue
		}
		for _, set := range es.Sets {
			if set.CompletedValue == nil || set.Signal == nil {
				continue
			}
			var kg float64
			if set.WeightKg != nil {
				kg = *set.WeightKg
			}
			completed = append(completed, domain.SetResult{
				ActualReps: *set.CompletedValue,
				Signal:     *set.Signal,
				WeightKg:   kg,
			})
		}
		break
	}

	return domain.NewFromHistory(config, completed), nil
}

// BuildTimedProgression constructs a domain.TimedProgression
// for the given time-based exercise in the given session, ready to call
// CurrentSet() for the next hold's recommendation. Returns an error if the
// exercise is not time_based or if the lookup fails.
func (s *Service) BuildTimedProgression(
	ctx context.Context,
	date time.Time,
	exerciseID int,
) (*domain.TimedProgression, error) {
	starting, err := s.GetStartingSeconds(ctx, exerciseID, date)
	if err != nil {
		return nil, fmt.Errorf("get starting seconds: %w", err)
	}

	sess, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	var completed []domain.TimedSetResult
	for _, es := range sess.ExerciseSets {
		if es.Exercise.ID != exerciseID {
			continue
		}
		for _, set := range es.Sets {
			if set.CompletedValue == nil || set.Signal == nil {
				continue
			}
			completed = append(completed, domain.TimedSetResult{
				ActualSeconds: *set.CompletedValue,
				Signal:        *set.Signal,
			})
		}
		break
	}

	return domain.NewTimedFromHistory(
		domain.TimedConfig{StartingSeconds: starting},
		completed,
	), nil
}

// UpdateExercise updates an existing exercise.
func (s *Service) UpdateExercise(ctx context.Context, ex Exercise) error {
	if err := s.repos.Exercises.Update(ctx, ex.ID, func(oldEx *Exercise) error {
		*oldEx = ex
		return nil
	}); err != nil {
		return fmt.Errorf("update exercise: %w", err)
	}
	return nil
}

// ListMuscleGroups retrieves all available muscle groups.
func (s *Service) ListMuscleGroups(ctx context.Context) ([]string, error) {
	groups, err := s.repos.Exercises.ListMuscleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle groups: %w", err)
	}
	return groups, nil
}

// WeeklyMuscleGroupVolume aggregates planned-vs-completed weekly load per muscle
// group across the supplied sessions. One entry is returned for every known
// muscle group, sorted alphabetically; groups with no contributions appear as
// zero-load rows so the UI can render them without a separate query. Targets are
// joined from muscle_group_weekly_targets; untracked groups carry TargetSets = 0.
func (s *Service) WeeklyMuscleGroupVolume(
	ctx context.Context,
	sessions []Session,
) ([]MuscleGroupVolume, error) {
	groupNames, err := s.repos.Exercises.ListMuscleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle groups: %w", err)
	}
	targets, err := s.repos.MuscleTargets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle group targets: %w", err)
	}
	return domain.WeeklyMuscleGroupVolume(sessions, targets, groupNames), nil
}

// GenerateExercise delegates to internal/service. Phase 3 transitional:
// removed in Task 3 when the rest of the service layer relocates and
// internal/workout.Service becomes a type alias.
func (s *Service) GenerateExercise(ctx context.Context, name string) (Exercise, error) {
	exercise, err := s.gen.GenerateExercise(ctx, name)
	if err != nil {
		return Exercise{}, fmt.Errorf("generate exercise: %w", err)
	}
	return exercise, nil
}

// SwapExercise replaces the exercise occupying a workout slot (identified by
// workoutExerciseID) with newExerciseID. The workout slot's stable ID is
// preserved so URLs targeting the slot keep working.
//
// Sets recorded against the old exercise are dropped — replaced with historical
// data for the new exercise when available, otherwise empty placeholders matching
// the old set count.
func (s *Service) SwapExercise(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	newExerciseID int,
) error {
	newExercise, err := s.repos.Exercises.Get(ctx, newExerciseID)
	if err != nil {
		return fmt.Errorf("get new exercise: %w", err)
	}

	historicalSets, err := s.findHistoricalSets(ctx, date, newExerciseID)
	if err != nil {
		return fmt.Errorf("find historical sets: %w", err)
	}

	err = s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		newSets := s.buildSetsForAdd(newExercise, sess.PeriodizationType, historicalSets)
		return sess.SwapExerciseInSlot(workoutExerciseID, newExercise, newSets)
	})
	if err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// findHistoricalSets retrieves set data from the most recent usage of an exercise.
// Aggregates with no sets are skipped — they exist for exercises whose historical
// exercise_sets rows were dropped by the time-based premigration but whose
// workout_exercise slot survived. Returns nil when no usable history is found.
func (s *Service) findHistoricalSets(ctx context.Context, date time.Time, exerciseID int) ([]Set, error) {
	// Get workout history (past 3 months)
	threeMonthsAgo := date.AddDate(0, -3, 0)
	history, err := s.repos.Sessions.List(ctx, threeMonthsAgo)
	if err != nil {
		return nil, fmt.Errorf("get workout history: %w", err)
	}

	// Look for the most recent usage of the exercise. List orders sessions
	// by workout_date DESC, so iterating forward visits newest first.
	for _, session := range history {
		// Skip the current date's session
		if session.Date.Equal(date) {
			continue
		}

		for _, exerciseSet := range session.ExerciseSets {
			if exerciseSet.Exercise.ID != exerciseID || len(exerciseSet.Sets) == 0 {
				continue
			}
			// Copy sets and reset completion status
			return s.copySetsWithoutCompletion(exerciseSet.Sets), nil
		}
	}

	// No historical data found
	return nil, nil
}

// copySetsWithoutCompletion creates a copy of sets with completion reset to nil.
// Note: callers in the AddExercise/swap paths route the result through
// buildSetsForAdd, which overrides TargetValue from the session's periodization.
// This function preserves all fields verbatim including TargetValue.
func (s *Service) copySetsWithoutCompletion(sets []Set) []Set {
	result := make([]Set, len(sets))
	for i, set := range sets {
		result[i] = Set{
			WeightKg:       set.WeightKg,
			TargetValue:    set.TargetValue,
			CompletedValue: nil, // Reset completion status.
			CompletedAt:    nil,
			Signal:         nil,
		}
	}
	return result
}

// buildSetsForAdd produces the Set slice for an exercise being added to or
// swapping into an existing session. The session's periodization always
// dictates TargetValue and TargetSets (so a Deadlift added in a Strength
// week gets 3 reps × 4 sets, not whatever the historical session had).
//
// When historicalSets is non-nil and contains weight data, the most recent
// completed weight is preserved as the starting weight for every new set —
// the user's progression isn't lost just because the prescription changed.
// Completion fields are always reset.
func (s *Service) buildSetsForAdd(ex Exercise, pt PeriodizationType, historicalSets []Set) []Set {
	sets := domain.BuildPlannedSets(ex, pt)
	// Allocate empty weight pointers for weighted/assisted exercises. The
	// form input on the per-set page binds to *float64; nil would render
	// as "no weight" instead of an empty editable input. Bodyweight and
	// time-based stay nil.
	if !ex.IsTimed() && ex.ExerciseType != ExerciseTypeBodyweight {
		for i := range sets {
			sets[i].WeightKg = new(float64)
		}
	}
	if len(historicalSets) == 0 {
		return sets
	}
	// Pull the latest completed weight from history; fall back to the first
	// historical set's WeightKg if no completion is present.
	var seedWeight *float64
	for i := len(historicalSets) - 1; i >= 0; i-- {
		if historicalSets[i].WeightKg != nil {
			seedWeight = historicalSets[i].WeightKg
			break
		}
	}
	if seedWeight == nil {
		return sets
	}
	for i := range sets {
		// Only seed weight for exercise types that carry weight (same guard as the allocation above).
		if !ex.IsTimed() && ex.ExerciseType != ExerciseTypeBodyweight {
			w := *seedWeight
			sets[i].WeightKg = &w
		}
	}
	return sets
}

// FindCompatibleExercises returns all exercises except the specified one.
func (s *Service) FindCompatibleExercises(ctx context.Context, exerciseID int) ([]Exercise, error) {
	// Get all exercises
	allExercises, err := s.repos.Exercises.List(ctx)
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

// AddExercise adds a new exercise to an existing workout session.
// It will retrieve historical weight data if available. Returns the
// workout_exercise.id assigned to the new slot, so callers can build URLs
// that point at the new exercise's detail page.
func (s *Service) AddExercise(ctx context.Context, date time.Time, exerciseID int) (int, error) {
	exercise, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("get exercise: %w", err)
	}

	historicalSets, err := s.findHistoricalSets(ctx, date, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("find historical sets: %w", err)
	}

	if _, err = s.repos.Sessions.Get(ctx, date); errors.Is(err, domain.ErrNotFound) {
		return 0, fmt.Errorf("workout session for date %s does not exist", date.Format(time.DateOnly))
	} else if err != nil {
		return 0, fmt.Errorf("check session existence: %w", err)
	}

	err = s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		newSets := s.buildSetsForAdd(exercise, sess.PeriodizationType, historicalSets)
		_, addErr := sess.AddExercise(exercise, newSets)
		if addErr != nil {
			return fmt.Errorf("add exercise to session: %w", addErr)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("update session with new exercise: %w", err)
	}

	// Re-fetch to learn the slot ID assigned by the repository on insert. The
	// slot is unique by Exercise.ID within a session (Session.AddExercise
	// rejected duplicates), so locating it is unambiguous.
	updated, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return 0, fmt.Errorf("re-fetch session after add: %w", err)
	}
	for _, es := range updated.ExerciseSets {
		if es.Exercise.ID == exerciseID {
			return es.ID, nil
		}
	}
	return 0, fmt.Errorf("added exercise %d not found in session %s", exerciseID, date.Format(time.DateOnly))
}

// GetFeatureFlag retrieves a feature flag by name.
func (s *Service) GetFeatureFlag(ctx context.Context, name string) (FeatureFlag, error) {
	flag, err := s.repos.FeatureFlags.Get(ctx, name)
	if err != nil {
		return FeatureFlag{}, fmt.Errorf("get feature flag %s: %w", name, err)
	}
	return flag, nil
}

// IsMaintenanceModeEnabled checks if maintenance mode is enabled.
func (s *Service) IsMaintenanceModeEnabled(ctx context.Context) bool {
	flag, err := s.repos.FeatureFlags.Get(ctx, "maintenance_mode")
	if err != nil {
		// If we can't check the flag, assume maintenance is disabled for safety
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to check maintenance mode flag", slog.Any("error", err))
		return false
	}
	return flag.Enabled
}

// ListFeatureFlags retrieves all feature flags.
func (s *Service) ListFeatureFlags(ctx context.Context) ([]FeatureFlag, error) {
	flags, err := s.repos.FeatureFlags.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feature flags: %w", err)
	}
	return flags, nil
}

// SetFeatureFlag updates or creates a feature flag.
func (s *Service) SetFeatureFlag(ctx context.Context, flag FeatureFlag) error {
	if err := s.repos.FeatureFlags.Set(ctx, flag); err != nil {
		return fmt.Errorf("set feature flag %s: %w", flag.Name, err)
	}
	return nil
}

// ExportUserData creates an SQLite database export containing all data for the authenticated user.
// This method is intended for GDPR compliance and allows users to download their complete data.
func (s *Service) ExportUserData(ctx context.Context) (string, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return "", errors.New("no authenticated user found in context")
	}

	tempDir := os.TempDir()

	// Call the database's createUserDB method
	exportPath, err := s.db.CreateUserDB(ctx, userID, tempDir)
	if err != nil {
		return "", fmt.Errorf("create user database: %w", err)
	}

	return exportPath, nil
}
