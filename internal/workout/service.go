package workout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/exerciseprogression"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/weekplanner"
)

// Service handles the business logic for workout management.
type Service struct {
	repo         *repository
	db           *sqlite.Database
	logger       *slog.Logger
	openaiAPIKey string
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	factory := newRepositoryFactory(db, logger)
	return &Service{
		repo:         factory.newRepository(),
		db:           db,
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

	existing, err := s.repo.sessions.List(ctx, monday)
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

	if err = s.repo.sessions.DeleteWeek(ctx, monday); err != nil {
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
	now := time.Now()
	offset := int(time.Monday - now.Weekday())
	if offset > 0 {
		offset = -6
	}
	monday := now.AddDate(0, 0, offset).Truncate(24 * time.Hour)
	sunday := monday.AddDate(0, 0, 6)

	// Check for existing sessions this week.
	existing, err := s.repo.sessions.List(ctx, monday)
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

	// Build 7-day schedule: sessions from DB for scheduled days, empty for rest days.
	workouts := make([]Session, 7)
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		sessionAggr, getErr := s.repo.sessions.Get(ctx, day)
		if getErr != nil && !errors.Is(getErr, ErrNotFound) {
			return nil, fmt.Errorf("get session %s: %w", formatDate(day), getErr)
		}
		if errors.Is(getErr, ErrNotFound) {
			workouts[i] = Session{ //nolint:exhaustruct // Rest days have no exercise data.
				Date: day,
			}
			continue
		}
		workouts[i], err = s.enrichSessionAggregate(ctx, sessionAggr)
		if err != nil {
			return nil, fmt.Errorf("enrich session %s: %w", formatDate(day), err)
		}
	}
	return workouts, nil
}

// generateWeeklyPlan uses the weekplanner to create all sessions for the week starting
// on monday and persists them in a single DB transaction.
func (s *Service) generateWeeklyPlan(ctx context.Context, monday time.Time) error {
	prefs, err := s.repo.prefs.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}

	exercises, err := s.repo.exercises.List(ctx)
	if err != nil {
		return fmt.Errorf("get exercises: %w", err)
	}

	targets, err := s.repo.muscleTargets.List(ctx)
	if err != nil {
		return fmt.Errorf("get muscle group targets: %w", err)
	}

	wpPrefs := weekplanner.Preferences{
		MondayMinutes:    prefs.MondayMinutes,
		TuesdayMinutes:   prefs.TuesdayMinutes,
		WednesdayMinutes: prefs.WednesdayMinutes,
		ThursdayMinutes:  prefs.ThursdayMinutes,
		FridayMinutes:    prefs.FridayMinutes,
		SaturdayMinutes:  prefs.SaturdayMinutes,
		SundayMinutes:    prefs.SundayMinutes,
	}

	wpExercises := make([]weekplanner.Exercise, len(exercises))
	for i, ex := range exercises {
		wpExercises[i] = weekplanner.Exercise{
			ID:                    ex.ID,
			Category:              weekplanner.Category(ex.Category),
			ExerciseType:          weekplanner.ExerciseType(ex.ExerciseType),
			PrimaryMuscleGroups:   ex.PrimaryMuscleGroups,
			SecondaryMuscleGroups: ex.SecondaryMuscleGroups,
		}
	}

	wpTargets := make([]weekplanner.MuscleGroupTarget, len(targets))
	for i, t := range targets {
		wpTargets[i] = weekplanner.MuscleGroupTarget{
			Name:            t.MuscleGroupName,
			WeeklySetTarget: t.WeeklySetTarget,
		}
	}

	planner := weekplanner.NewWeeklyPlanner(wpPrefs, wpExercises, wpTargets)
	plannedSessions, err := planner.Plan(monday)
	if err != nil {
		return fmt.Errorf("plan week: %w", err)
	}

	sessionAggrs := make([]sessionAggregate, len(plannedSessions))
	for i, ps := range plannedSessions {
		periodType := PeriodizationStrength
		if ps.PeriodizationType == weekplanner.PeriodizationHypertrophy {
			periodType = PeriodizationHypertrophy
		}

		exerciseSets := make([]exerciseSetAggregate, len(ps.ExerciseSets))
		for j, pes := range ps.ExerciseSets {
			sets := make([]Set, len(pes.Sets))
			for k, planSet := range pes.Sets {
				sets[k] = Set{ //nolint:exhaustruct // WeightKg, CompletedReps, CompletedAt, Signal start nil.
					MinReps: planSet.MinReps,
					MaxReps: planSet.MaxReps,
				}
			}
			exerciseSets[j] = exerciseSetAggregate{ //nolint:exhaustruct // ID is auto-assigned, WarmupCompletedAt starts nil.
				ExerciseID: pes.ExerciseID,
				Sets:       sets,
			}
		}

		sessionAggrs[i] = sessionAggregate{ //nolint:exhaustruct // DifficultyRating, StartedAt, CompletedAt start zero.
			Date:              ps.Date,
			PeriodizationType: periodType,
			ExerciseSets:      exerciseSets,
		}
	}

	if err = s.repo.sessions.CreateBatch(ctx, sessionAggrs); err != nil {
		return fmt.Errorf("create batch sessions: %w", err)
	}
	return nil
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
		Date:              sessionAggr.Date,
		StartedAt:         sessionAggr.StartedAt,
		CompletedAt:       sessionAggr.CompletedAt,
		DifficultyRating:  sessionAggr.DifficultyRating,
		ExerciseSets:      make([]ExerciseSet, len(sessionAggr.ExerciseSets)),
		PeriodizationType: sessionAggr.PeriodizationType,
	}

	for i, ex := range sessionAggr.ExerciseSets {
		exercise, err := s.repo.exercises.Get(ctx, ex.ExerciseID)
		if err != nil {
			return Session{}, fmt.Errorf("get exercise %d: %w", ex.ExerciseID, err)
		}
		session.ExerciseSets[i].ID = ex.ID
		session.ExerciseSets[i].Exercise = exercise
		session.ExerciseSets[i].Sets = ex.Sets
		session.ExerciseSets[i].WarmupCompletedAt = ex.WarmupCompletedAt
	}
	return session, nil
}

// mondayOf returns the Monday of the week containing date, truncated to midnight.
func mondayOf(date time.Time) time.Time {
	offset := int(time.Monday - date.Weekday())
	if offset > 0 {
		offset = -6
	}
	return date.AddDate(0, 0, offset).Truncate(24 * time.Hour)
}

// StartSession starts a new workout session.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	// Generate the week's plan if no sessions exist for this week yet.
	monday := mondayOf(date)
	existing, listErr := s.repo.sessions.List(ctx, monday)
	if listErr != nil {
		return fmt.Errorf("list sessions for week of %s: %w", formatDate(date), listErr)
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
			return fmt.Errorf("generate weekly plan for %s: %w", formatDate(date), genErr)
		}
	}

	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
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
	workoutExerciseID int,
	setIndex int,
	completedReps int,
) error {
	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		for _, ex := range sess.ExerciseSets {
			if ex.ID == workoutExerciseID {
				if setIndex >= len(ex.Sets) {
					return false, fmt.Errorf("exercise set index %d out of bounds", setIndex)
				}
				now := time.Now().UTC()
				ex.Sets[setIndex].CompletedReps = &completedReps
				ex.Sets[setIndex].CompletedAt = &now
				return true, nil
			}
		}
		return false, errors.New("workout exercise not found")
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
	}

	return nil
}

// RecordSetCompletion atomically persists the signal, weight, reps, and timestamp for a set.
func (s *Service) RecordSetCompletion(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	signal Signal,
	weightKg float64,
	reps int,
) error {
	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		for i := range sess.ExerciseSets {
			if sess.ExerciseSets[i].ID == workoutExerciseID {
				if setIndex >= len(sess.ExerciseSets[i].Sets) {
					return false, fmt.Errorf("set index %d out of bounds", setIndex)
				}
				now := time.Now().UTC()
				sess.ExerciseSets[i].Sets[setIndex].Signal = &signal
				sess.ExerciseSets[i].Sets[setIndex].WeightKg = &weightKg
				sess.ExerciseSets[i].Sets[setIndex].CompletedReps = &reps
				sess.ExerciseSets[i].Sets[setIndex].CompletedAt = &now
				return true, nil
			}
		}
		return false, errors.New("workout exercise not found")
	}); err != nil {
		return fmt.Errorf("update session %s: %w", formatDate(date), err)
	}
	return nil
}

// MarkWarmupComplete marks the warmup as complete for a specific workout exercise slot.
func (s *Service) MarkWarmupComplete(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
) error {
	if err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		for i := range sess.ExerciseSets {
			if sess.ExerciseSets[i].ID == workoutExerciseID {
				now := time.Now().UTC()
				sess.ExerciseSets[i].WarmupCompletedAt = &now
				return true, nil
			}
		}
		return false, errors.New("workout exercise not found")
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
				Date:              session.Date,
				DifficultyRating:  session.DifficultyRating,
				StartedAt:         session.StartedAt,
				CompletedAt:       session.CompletedAt,
				ExerciseSets:      make([]ExerciseSet, len(session.ExerciseSets)),
				PeriodizationType: session.PeriodizationType,
			}

			// Enrich exercise sets with exercise data
			for i, es := range session.ExerciseSets {
				ex, getErr := s.repo.exercises.Get(ctx, es.ExerciseID)
				if getErr != nil {
					return nil, fmt.Errorf("get exercise %d: %w", es.ExerciseID, getErr)
				}

				enrichedSession.ExerciseSets[i] = ExerciseSet{
					ID:                es.ID,
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

// GetExerciseSetsForExerciseSince retrieves all sets for a specific exercise since a given date.
func (s *Service) GetExerciseSetsForExerciseSince(ctx context.Context, exerciseID int, since time.Time) (
	ExerciseProgress, error) {
	aggs, err := s.repo.sessions.ListSetsForExerciseSince(ctx, exerciseID, since)
	if err != nil {
		return ExerciseProgress{}, fmt.Errorf("list sets for exercise: %w", err)
	}

	ex, err := s.repo.exercises.Get(ctx, exerciseID)
	if err != nil {
		return ExerciseProgress{}, fmt.Errorf("get exercise %d: %w", exerciseID, err)
	}

	entries := make([]ExerciseProgressEntry, 0, len(aggs))
	for _, agg := range aggs {
		// Collect only completed sets; skip entries with none.
		var completedSets []Set
		for _, set := range agg.Sets {
			if set.CompletedReps != nil {
				completedSets = append(completedSets, set)
			}
		}
		if len(completedSets) > 0 {
			entries = append(entries, ExerciseProgressEntry{
				Date: agg.Date,
				Sets: completedSets,
			})
		}
	}

	return ExerciseProgress{
		Exercise: ex,
		Entries:  entries,
	}, nil
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
	prev, err := s.repo.sessions.GetLatestStartingWeightBefore(ctx, exerciseID, beforeDate)
	if err != nil {
		return 0, fmt.Errorf("get latest starting weight: %w", err)
	}
	if prev.PeriodizationType == "" || prev.PeriodizationType == targetType {
		return prev.WeightKg, nil
	}
	fromReps := exerciseprogression.TargetReps(periodizationToProgression(prev.PeriodizationType))
	toReps := exerciseprogression.TargetReps(periodizationToProgression(targetType))
	return exerciseprogression.ConvertWeight(prev.WeightKg, fromReps, toReps), nil
}

// periodizationToProgression maps a workout periodization to its exerciseprogression
// counterpart. Unknown values default to Strength.
func periodizationToProgression(p PeriodizationType) exerciseprogression.PeriodizationType {
	switch p {
	case PeriodizationHypertrophy:
		return exerciseprogression.Hypertrophy
	case PeriodizationStrength:
		return exerciseprogression.Strength
	default:
		return exerciseprogression.Strength
	}
}

// BuildProgression constructs an exerciseprogression.Progression for the given exercise
// in the given session, ready to call CurrentSet() for the next set recommendation.
func (s *Service) BuildProgression(
	ctx context.Context,
	date time.Time,
	exerciseID int,
) (*exerciseprogression.Progression, error) {
	sess, err := s.repo.sessions.Get(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	startingWeight, err := s.GetStartingWeight(ctx, exerciseID, date, sess.PeriodizationType)
	if err != nil {
		return nil, fmt.Errorf("get starting weight: %w", err)
	}

	epType := periodizationToProgression(sess.PeriodizationType)

	config := exerciseprogression.Config{
		Type:           epType,
		StartingWeight: startingWeight,
	}

	var completed []exerciseprogression.SetResult
	for _, es := range sess.ExerciseSets {
		if es.ExerciseID != exerciseID {
			continue
		}
		for _, set := range es.Sets {
			if set.CompletedReps == nil || set.Signal == nil {
				continue
			}
			var sig exerciseprogression.Signal
			switch *set.Signal {
			case SignalTooHeavy:
				sig = exerciseprogression.SignalTooHeavy
			case SignalOnTarget:
				sig = exerciseprogression.SignalOnTarget
			case SignalTooLight:
				sig = exerciseprogression.SignalTooLight
			}
			var kg float64
			if set.WeightKg != nil {
				kg = *set.WeightKg
			}
			completed = append(completed, exerciseprogression.SetResult{
				ActualReps: *set.CompletedReps,
				Signal:     sig,
				WeightKg:   kg,
			})
		}
		break
	}

	return exerciseprogression.NewFromHistory(config, completed), nil
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

// PrimarySetWeight and SecondarySetWeight are the per-set contributions to a
// muscle group's weekly load. The split reflects that secondary engagement
// receives meaningfully less stimulus than primary engagement.
const (
	PrimarySetWeight   = 1.0
	SecondarySetWeight = 0.5
)

// WeeklyMuscleGroupVolume aggregates planned-vs-completed weekly load per muscle
// group across the supplied sessions. One entry is returned for every known
// muscle group, sorted alphabetically; groups with no contributions appear as
// zero-load rows so the UI can render them without a separate query. Targets are
// joined from muscle_group_weekly_targets; untracked groups carry TargetSets = 0.
func (s *Service) WeeklyMuscleGroupVolume(
	ctx context.Context,
	sessions []Session,
) ([]MuscleGroupVolume, error) {
	groupNames, err := s.repo.exercises.ListMuscleGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle groups: %w", err)
	}

	targets, err := s.repo.muscleTargets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list muscle group targets: %w", err)
	}
	targetByName := make(map[string]int, len(targets))
	for _, t := range targets {
		targetByName[t.MuscleGroupName] = t.WeeklySetTarget
	}

	known := make(map[string]struct{}, len(groupNames))
	for _, name := range groupNames {
		known[name] = struct{}{}
	}

	planned := make(map[string]float64, len(groupNames))
	completed := make(map[string]float64, len(groupNames))
	aggregateMuscleGroupLoad(sessions, known, planned, completed)

	result := make([]MuscleGroupVolume, 0, len(groupNames))
	for _, name := range groupNames {
		result = append(result, MuscleGroupVolume{
			Name:          name,
			CompletedLoad: completed[name],
			PlannedLoad:   planned[name],
			TargetSets:    targetByName[name],
		})
	}
	// ListMuscleGroups orders by name, so result is already alphabetical.
	return result, nil
}

// aggregateMuscleGroupLoad walks every set in the supplied sessions and totals the
// weighted load for each muscle group, accumulating into the planned and completed
// maps. Primary contributions count as PrimarySetWeight, secondary as
// SecondarySetWeight. Muscle group names not present in known are silently skipped
// — they cannot occur in production due to FK constraints, but the guard keeps
// tests safe when synthetic exercises reference unknown groups.
func aggregateMuscleGroupLoad(
	sessions []Session,
	known map[string]struct{},
	planned, completed map[string]float64,
) {
	for _, sess := range sessions {
		for _, ex := range sess.ExerciseSets {
			for _, set := range ex.Sets {
				done := set.CompletedAt != nil
				creditMuscleGroups(ex.Exercise.PrimaryMuscleGroups, PrimarySetWeight, done, known, planned, completed)
				creditMuscleGroups(ex.Exercise.SecondaryMuscleGroups, SecondarySetWeight, done, known, planned, completed)
			}
		}
	}
}

// creditMuscleGroups credits weight to each muscle group in names, both to planned
// and (when done) to completed. Groups missing from known are ignored.
func creditMuscleGroups(
	names []string,
	weight float64,
	done bool,
	known map[string]struct{},
	planned, completed map[string]float64,
) {
	for _, mg := range names {
		if _, ok := known[mg]; !ok {
			continue
		}
		planned[mg] += weight
		if done {
			completed[mg] += weight
		}
	}
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
	generator := newExerciseGenerator(s.openaiAPIKey, muscleGroups, s.logger)
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
	if _, err := s.repo.exercises.Get(ctx, newExerciseID); err != nil {
		return fmt.Errorf("get new exercise: %w", err)
	}

	historicalSets, err := s.findHistoricalSets(ctx, date, newExerciseID)
	if err != nil {
		return fmt.Errorf("find historical sets: %w", err)
	}

	if err = s.replaceExerciseInSession(ctx, date, workoutExerciseID, newExerciseID, historicalSets); err != nil {
		return err
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

	// Look for the most recent usage of the exercise. List orders sessions
	// by workout_date DESC, so iterating forward visits newest first.
	for _, session := range history {
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
			Signal:        nil,
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
			weight = new(float64) // Empty weight for weighted exercises
		}
		result[i] = Set{
			WeightKg:      weight,
			MinReps:       set.MinReps,
			MaxReps:       set.MaxReps,
			CompletedReps: nil,
			CompletedAt:   nil,
			Signal:        nil,
		}
	}
	return result
}

// replaceExerciseInSession swaps the exercise occupying a workout slot, keeping
// the slot's stable ID intact. Any previously recorded sets on the slot are
// dropped — historicalSets seeds the replacement when present, otherwise we
// generate empty placeholders matching the old set count.
func (s *Service) replaceExerciseInSession(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	newExerciseID int,
	historicalSets []Set,
) error {
	err := s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		for i, exerciseSet := range sess.ExerciseSets {
			if exerciseSet.ID != workoutExerciseID {
				continue
			}
			sess.ExerciseSets[i].ExerciseID = newExerciseID
			sess.ExerciseSets[i].WarmupCompletedAt = nil
			if historicalSets != nil {
				sess.ExerciseSets[i].Sets = historicalSets
			} else {
				sess.ExerciseSets[i].Sets = s.createEmptySets(exerciseSet.Sets)
			}
			return true, nil
		}
		return false, fmt.Errorf("workout exercise %d not found in workout for date %s",
			workoutExerciseID, formatDate(date))
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
			const defaultSetCount = 3
			newSets = make([]Set, defaultSetCount)
			for i := range newSets {
				newSets[i] = Set{
					WeightKg:      new(float64),
					MinReps:       defaultMinReps,
					MaxReps:       defaultMaxReps,
					CompletedReps: nil,
					CompletedAt:   nil,
					Signal:        nil,
				}
			}
		}

		// Add the new exercise to the session. ID stays 0 so the repository
		// assigns a fresh workout_exercise.id on insert.
		newExerciseSet := exerciseSetAggregate{ //nolint:exhaustruct // ID is auto-assigned by repository.
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

// ListFeatureFlags retrieves all feature flags.
func (s *Service) ListFeatureFlags(ctx context.Context) ([]FeatureFlag, error) {
	flags, err := s.repo.featureFlags.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feature flags: %w", err)
	}
	return flags, nil
}

// SetFeatureFlag updates or creates a feature flag.
func (s *Service) SetFeatureFlag(ctx context.Context, flag FeatureFlag) error {
	if err := s.repo.featureFlags.Set(ctx, flag); err != nil {
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
