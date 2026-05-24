package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
)

// RegenerateWeeklyPlanIfUnstarted replaces the current week's generated plan with one
// that reflects the latest preferences, but only when no session has been started yet.
// If any workout this week has a non-zero StartedAt the existing plan is left intact.
//
// The delete and generate steps are NOT wrapped in a single transaction. To
// prevent two concurrent callers from both passing the no-started-session
// check and racing on delete+generate, we serialize per-user via an
// in-process mutex. Multi-process deployments would need a different
// scheme (advisory lock or single-row sentinel); today's deployment is
// single-machine (see fly.toml min_machines_running = 0, no horizontal
// scaling configured).
//
// The self-heal via ResolveWeeklySchedule remains as defense-in-depth.
func (s *Service) RegenerateWeeklyPlanIfUnstarted(ctx context.Context) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	mu := s.userMutex(userID)
	mu.Lock()
	defer mu.Unlock()

	monday := domain.MondayOf(time.Now())
	sunday := monday.AddDate(0, 0, 6)

	existing, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return fmt.Errorf("list sessions for week: %w", err)
	}

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
func (s *Service) ResolveWeeklySchedule(ctx context.Context) ([]domain.Session, error) {
	monday := domain.MondayOf(time.Now())
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

	workouts := make([]domain.Session, 7)
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		sess, getErr := s.repos.Sessions.Get(ctx, day)
		if getErr != nil && !errors.Is(getErr, domain.ErrNotFound) {
			return nil, fmt.Errorf("get session %s: %w", day.Format(time.DateOnly), getErr)
		}
		if errors.Is(getErr, domain.ErrNotFound) {
			workouts[i] = domain.Session{ //nolint:exhaustruct // Rest days have no exercise data.
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

	for i := range plannedSessions {
		if !plannedSessions[i].IsDeload {
			continue
		}
		if err = s.seedDeloadWeights(ctx, &plannedSessions[i]); err != nil {
			return err
		}
	}

	if err = s.repos.Sessions.CreateBatch(ctx, plannedSessions); err != nil {
		return fmt.Errorf("create batch sessions: %w", err)
	}
	return nil
}

// seedDeloadWeights sets the per-set weight for every weighted exercise in a
// deload session to GetDeloadStartingWeight (a fraction of the user's recent
// working weight). Called for both weekly-plan generation and ad-hoc session
// creation when sess.IsDeload is true.
func (s *Service) seedDeloadWeights(ctx context.Context, sess *domain.Session) error {
	for j := range sess.ExerciseSets {
		ex := sess.ExerciseSets[j].Exercise
		if !ex.HasWeight() {
			continue
		}
		w, err := s.GetDeloadStartingWeight(ctx, ex.ID, sess.Date)
		if err != nil {
			return fmt.Errorf("seed deload weight for %s: %w", ex.Name, err)
		}
		weight := w
		for k := range sess.ExerciseSets[j].Sets {
			sess.ExerciseSets[j].Sets[k].WeightKg = &weight
		}
	}
	return nil
}

// GetSession retrieves a workout session for a specific date.
func (s *Service) GetSession(ctx context.Context, date time.Time) (domain.Session, error) {
	sess, err := s.repos.Sessions.Get(ctx, date)
	if err != nil {
		return domain.Session{}, fmt.Errorf("get session %s: %w", date.Format(time.DateOnly), err)
	}
	return sess, nil
}

// summarizeWeek walks existing and returns aggregate info needed by
// StartSession for the lazy-create branch:
//   - weekCount: number of sessions whose Date falls in monday..sunday.
//   - hasDate: whether a session exists for date specifically.
//   - usedExerciseIDs: set of exercise IDs used in any in-week session,
//     for PlanDay's no-repeat avoidance.
func summarizeWeek(existing []domain.Session, date, monday time.Time) (int, bool, map[int]bool) {
	sunday := monday.AddDate(0, 0, 6)
	used := make(map[int]bool)
	var weekCount int
	var hasDate bool
	for _, sess := range existing {
		if sess.Date.Before(monday) || sess.Date.After(sunday) {
			continue
		}
		weekCount++
		if sess.Date.Equal(date) {
			hasDate = true
		}
		for _, es := range sess.ExerciseSets {
			used[es.Exercise.ID] = true
		}
	}
	return weekCount, hasDate, used
}

// createAdHocSession plans and persists a single session for date. Used by
// StartSession when the user starts an unscheduled day (extra workout) or a
// day added to the schedule mid-week after another in-week session was
// already started. used is the set of exercise IDs already used in other
// in-week sessions, passed through to PlanDay's no-repeat selection.
func (s *Service) createAdHocSession(ctx context.Context, date time.Time, used map[int]bool) error {
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
	sess, err := planner.PlanDay(date, used)
	if err != nil {
		return fmt.Errorf("plan day %s: %w", date.Format(time.DateOnly), err)
	}

	if sess.IsDeload {
		if err = s.seedDeloadWeights(ctx, &sess); err != nil {
			return err
		}
	}
	if err = s.repos.Sessions.Create(ctx, sess); err != nil {
		return fmt.Errorf("create session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// StartSession marks the workout session for date as started. If no session
// exists for date — either because date is unscheduled (extra workout) or
// because date is a newly-scheduled day that was added mid-week after the
// weekly plan was generated — a single-day session is planned via PlanDay
// and inserted before the start mutation. If the whole week is missing the
// existing generateWeeklyPlan path runs first; only then is the per-date
// check applied.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	monday := domain.MondayOf(date)
	existing, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return fmt.Errorf("list sessions for week of %s: %w", date.Format(time.DateOnly), err)
	}

	weekCount, hasDate, used := summarizeWeek(existing, date, monday)

	if weekCount == 0 {
		// generateWeeklyPlan may race against another caller who already inserted
		// the week's sessions. Treat ErrAlreadyExists as success — the row we
		// need is now present, so we just re-list below.
		if err = s.generateWeeklyPlan(ctx, monday); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
			return fmt.Errorf("generate weekly plan for %s: %w", date.Format(time.DateOnly), err)
		}
		existing, err = s.repos.Sessions.List(ctx, monday)
		if err != nil {
			return fmt.Errorf("re-list sessions for week of %s: %w", date.Format(time.DateOnly), err)
		}
		// weekCount is irrelevant on the second call — generateWeeklyPlan (or a
		// concurrent caller) just ensured the week is populated.
		_, hasDate, used = summarizeWeek(existing, date, monday)
	}

	if !hasDate {
		if err = s.createAdHocSession(ctx, date, used); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
			return fmt.Errorf("create ad-hoc session %s: %w", date.Format(time.DateOnly), err)
		}
	}

	err = s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
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

// CompleteSession marks a workout session as completed. When the session
// has not been started yet — e.g. a user retroactively logging a workout
// they performed in real life — Start is invoked first inside the same
// transaction so completion always succeeds.
func (s *Service) CompleteSession(ctx context.Context, date time.Time) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		now := time.Now()
		if sess.StartedAt.IsZero() {
			if err := sess.Start(now); err != nil {
				return fmt.Errorf("auto-start before complete: %w", err)
			}
		}
		return sess.Complete(now)
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	if s.scheduler != nil {
		userID := contexthelpers.AuthenticatedUserID(ctx)
		if err := s.scheduler.CancelForWorkout(ctx, userID, date); err != nil {
			s.logger.LogAttrs(ctx, slog.LevelWarn, "cancel pending pushes on workout complete",
				slog.Any("error", err))
		}
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
// Schedules a rest push announcing set 1 when the warmup transitions from
// not-done to done, the user has push enabled, and at least one subscription
// exists. Re-clicking when the warmup is already done is a no-op on the
// push side (the underlying domain mutation still refreshes the timestamp,
// preserving prior behavior).
func (s *Service) MarkWarmupComplete(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
) error {
	var (
		wasComplete   bool
		postSlot      domain.ExerciseSet
		postSlotOK    bool
		periodization domain.PeriodizationType
		sessionDeload bool
	)
	now := time.Now().UTC()

	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		if slot, ok := sess.Slot(workoutExerciseID); ok {
			wasComplete = slot.WarmupCompletedAt != nil
		}
		periodization = sess.PeriodizationType
		sessionDeload = sess.IsDeload

		if mErr := sess.MarkWarmupComplete(workoutExerciseID, now); mErr != nil {
			return mErr //nolint:wrapcheck // outer fmt.Errorf wraps with date context.
		}
		postSlot, postSlotOK = sess.Slot(workoutExerciseID)
		return nil
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}

	if !wasComplete && postSlotOK {
		userID := contexthelpers.AuthenticatedUserID(ctx)
		s.applyRestPushDecision(ctx, userID, workoutExerciseID, postSlot, periodization, sessionDeload, now)
	}
	return nil
}
