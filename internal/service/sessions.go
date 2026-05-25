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

// RegenerateWeeklyPlanIfUnstarted replaces the current week's plan when no
// session has been started yet. Atomic via WeekPlanRepository.Update — the
// AnyStarted check and the replacement happen in one transaction.
//
// Treats ErrNotFound as a no-op: a missing week has by definition no started
// session, so there is nothing to regenerate. This keeps callers
// (e.g. handler-preferences.go) from erroring on a brand-new user's first
// regenerate before any week has been persisted.
func (s *Service) RegenerateWeeklyPlanIfUnstarted(ctx context.Context) error {
	monday := domain.MondayOf(time.Now())
	newPlan, err := s.planWeek(ctx, monday)
	if err != nil {
		return err
	}
	err = s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		if wp.AnyStarted() {
			return nil
		}
		wp.Replace(newPlan)
		return nil
	})
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("regenerate week %s: %w", monday.Format(time.DateOnly), err)
	}
	return nil
}

// ResolveWeeklySchedule returns the WeekPlan for the current week. If no plan
// exists yet, generates one via the Planner and persists it; tolerates a
// concurrent create race by re-reading on ErrAlreadyExists.
func (s *Service) ResolveWeeklySchedule(ctx context.Context) (domain.WeekPlan, error) {
	monday := domain.MondayOf(time.Now())

	plan, err := s.repos.WeekPlans.Get(ctx, monday)
	if err == nil {
		return plan, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.WeekPlan{}, fmt.Errorf("get week %s: %w", monday.Format(time.DateOnly), err)
	}

	newPlan, err := s.planWeek(ctx, monday)
	if err != nil {
		return domain.WeekPlan{}, err
	}
	if err = s.repos.WeekPlans.Create(ctx, newPlan); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
		return domain.WeekPlan{}, fmt.Errorf("create week %s: %w", monday.Format(time.DateOnly), err)
	}
	plan, err = s.repos.WeekPlans.Get(ctx, monday)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("re-get week after create: %w", err)
	}
	return plan, nil
}

// planWeek builds an in-memory WeekPlan using the Planner and seeds deload
// weights. Replaces the old generateWeeklyPlan helper.
func (s *Service) planWeek(ctx context.Context, monday time.Time) (domain.WeekPlan, error) {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("get preferences: %w", err)
	}
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("get exercises: %w", err)
	}
	targets, err := s.repos.MuscleTargets.List(ctx)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("get muscle group targets: %w", err)
	}
	planner := domain.NewPlanner(prefs, exercises, targets)
	plan, err := planner.Plan(monday)
	if err != nil {
		return domain.WeekPlan{}, fmt.Errorf("plan week: %w", err)
	}
	for i := range plan.Sessions {
		if !plan.Sessions[i].IsDeload || len(plan.Sessions[i].ExerciseSets) == 0 {
			continue
		}
		if err = s.seedDeloadWeights(ctx, &plan.Sessions[i]); err != nil {
			return domain.WeekPlan{}, err
		}
	}
	return plan, nil
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

// usedExerciseIDs returns the set of exercise IDs used by any scheduled
// session in plan, for PlanDay's no-repeat avoidance.
func usedExerciseIDs(plan domain.WeekPlan) map[int]bool {
	used := make(map[int]bool)
	for i := range plan.Sessions {
		for _, es := range plan.Sessions[i].ExerciseSets {
			used[es.Exercise.ID] = true
		}
	}
	return used
}

// planSingleDay builds a Session for date via the Planner, seeding deload
// weights if needed. Pure in-memory; no DB writes. Returns the session ready
// to be placed into a WeekPlan at the right offset.
func (s *Service) planSingleDay(
	ctx context.Context, date time.Time, used map[int]bool,
) (domain.Session, error) {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return domain.Session{}, fmt.Errorf("get preferences: %w", err)
	}
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return domain.Session{}, fmt.Errorf("get exercises: %w", err)
	}
	targets, err := s.repos.MuscleTargets.List(ctx)
	if err != nil {
		return domain.Session{}, fmt.Errorf("get muscle group targets: %w", err)
	}
	planner := domain.NewPlanner(prefs, exercises, targets)
	sess, err := planner.PlanDay(date, used)
	if err != nil {
		return domain.Session{}, fmt.Errorf("plan day %s: %w", date.Format(time.DateOnly), err)
	}
	if sess.IsDeload {
		if err = s.seedDeloadWeights(ctx, &sess); err != nil {
			return domain.Session{}, err
		}
	}
	return sess, nil
}

// createAdHocSession plans and persists a single session for date via the
// WeekPlanRepository.Update closure. Used by StartSession when the user
// starts an unscheduled day (extra workout) or a day added to the schedule
// mid-week after another in-week session was already started. used is the
// set of exercise IDs already used in other in-week sessions, passed
// through to PlanDay's no-repeat selection.
//
// The closure overwrites the rest-day placeholder at the right offset; the
// single-pass reinsert in WeekPlanRepository.Update writes each slot's
// array index into the workout_exercises.position column, so other
// sessions' slot positions survive untouched.
// Callers must ensure the week row exists first (StartSession does so via
// WeekPlans.Create) — Update returns domain.ErrNotFound otherwise.
func (s *Service) createAdHocSession(ctx context.Context, date time.Time, used map[int]bool) error {
	sess, err := s.planSingleDay(ctx, date, used)
	if err != nil {
		return err
	}
	monday := domain.MondayOf(date)
	err = s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		offset := int(date.Sub(wp.Monday).Hours() / 24)
		if offset < 0 || offset > 6 {
			return fmt.Errorf(
				"date %s outside week %s",
				date.Format(time.DateOnly), monday.Format(time.DateOnly),
			)
		}
		wp.Sessions[offset] = sess
		return nil
	})
	if err != nil {
		return fmt.Errorf("create ad-hoc session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// StartSession marks the workout session for date as started. If no session
// exists for date — either because date is unscheduled (extra workout) or
// because date is a newly-scheduled day that was added mid-week after the
// weekly plan was generated — a single-day session is planned via PlanDay
// and inserted before the start mutation. If the whole week is missing the
// existing weekly-plan generation path runs first; only then is the per-date
// check applied.
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
	monday := domain.MondayOf(date)
	plan, err := s.repos.WeekPlans.Get(ctx, monday)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("get week of %s: %w", date.Format(time.DateOnly), err)
	}
	if errors.Is(err, domain.ErrNotFound) {
		newPlan, planErr := s.planWeek(ctx, monday)
		if planErr != nil {
			return planErr
		}
		if createErr := s.repos.WeekPlans.Create(ctx, newPlan); createErr != nil &&
			!errors.Is(createErr, domain.ErrAlreadyExists) {
			return fmt.Errorf("create week for %s: %w", date.Format(time.DateOnly), createErr)
		}
		plan, err = s.repos.WeekPlans.Get(ctx, monday)
		if err != nil {
			return fmt.Errorf("re-get week for %s: %w", date.Format(time.DateOnly), err)
		}
	}

	sessOnDate := plan.SessionOn(date)
	hasDate := sessOnDate != nil && len(sessOnDate.ExerciseSets) > 0
	if !hasDate {
		used := usedExerciseIDs(plan)
		if err = s.createAdHocSession(ctx, date, used); err != nil && !errors.Is(err, domain.ErrAlreadyExists) {
			return fmt.Errorf("create ad-hoc %s: %w", date.Format(time.DateOnly), err)
		}
	}

	err = s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		return wp.Start(date, time.Now())
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
	if err := s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
		sess := wp.SessionOn(date)
		if sess == nil {
			return domain.ErrNotFound
		}
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
	if err := s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
		return wp.SetDifficulty(date, difficulty)
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// MarkWarmupComplete marks the warmup as complete for the slot at pos on date.
// Schedules a rest push announcing set 1 when the warmup transitions from
// not-done to done, the user has push enabled, and at least one subscription
// exists. Re-clicking when the warmup is already done is a no-op on the
// push side (the underlying domain mutation still refreshes the timestamp,
// preserving prior behavior).
func (s *Service) MarkWarmupComplete(
	ctx context.Context,
	date time.Time,
	pos int,
) error {
	var (
		wasComplete   bool
		postSlot      domain.ExerciseSet
		postSlotOK    bool
		periodization domain.PeriodizationType
		sessionDeload bool
	)
	now := time.Now().UTC()

	if err := s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
		sess := wp.SessionOn(date)
		if sess == nil {
			return domain.ErrNotFound
		}
		if pos >= 0 && pos < len(sess.ExerciseSets) {
			wasComplete = sess.ExerciseSets[pos].WarmupCompletedAt != nil
		}
		periodization = sess.PeriodizationType
		sessionDeload = sess.IsDeload

		if mErr := sess.MarkWarmupComplete(pos, now); mErr != nil {
			return mErr //nolint:wrapcheck // outer fmt.Errorf wraps with date context.
		}
		if pos >= 0 && pos < len(sess.ExerciseSets) {
			postSlot = sess.ExerciseSets[pos]
			postSlotOK = true
		}
		return nil
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}

	if !wasComplete && postSlotOK {
		userID := contexthelpers.AuthenticatedUserID(ctx)
		s.applyRestPushDecision(ctx, userID, date, pos, postSlot, periodization, sessionDeload, now)
	}
	return nil
}

// StartDeloadNow flips IsDeload to true on every current-week session
// dated today or later that is not already fully completed, then snaps
// the mesocycle anchor to next Monday. Used when the user needs
// recovery off-schedule (e.g. returning from sickness). Undone by
// RestartMesocycleAnchor, which clears the same set of flips.
//
// Atomic within the week via WeekPlans.Update — the for-each-session
// flip happens in one transaction. The mesocycle-anchor write is a
// separate tx; only the flip needs week-level atomicity. Treats
// ErrNotFound as a no-op (no week persisted yet → nothing to flip).
//
//nolint:dupl // mirror of RestartMesocycleAnchor; kept separate intentionally (SwitchToDeload vs ClearDeload, distinct intent).
func (s *Service) StartDeloadNow(ctx context.Context) error {
	monday := domain.MondayOf(time.Now())
	today := domain.StartOfDay(time.Now())

	err := s.repos.WeekPlans.Update(ctx, monday, func(wp *domain.WeekPlan) error {
		return wp.FlipDeloadFromToday(today)
	})
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("flip deload for week %s: %w", monday.Format(time.DateOnly), err)
	}

	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
	if err = s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	return nil
}
