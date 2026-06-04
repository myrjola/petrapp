package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/platform/contexthelpers"
)

// UpdateSetWeight updates the weight for a specific set in a workout.
func (s *Service) UpdateSetWeight(
	ctx context.Context,
	date time.Time,
	pos int,
	setIndex int,
	newWeight float64,
) error {
	if err := s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
		return wp.UpdateSetWeight(date, pos, setIndex, newWeight)
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// UpdateCompletedValue updates a previously completed set with new value (reps or seconds).
func (s *Service) UpdateCompletedValue(
	ctx context.Context,
	date time.Time,
	pos int,
	setIndex int,
	completedValue int,
) error {
	if err := s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
		return wp.UpdateCompletedValue(date, pos, setIndex, completedValue, time.Now().UTC())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	return nil
}

// RecordSet atomically persists the signal (nil for deload sets), weight
// (nil for time-based sets), completed value (reps or seconds depending on
// exercise type), and timestamp.
func (s *Service) RecordSet(
	ctx context.Context,
	date time.Time,
	pos int,
	setIndex int,
	signal *domain.Signal,
	weightKg *float64,
	completedValue int,
) error {
	var (
		wasComplete   bool
		postSlot      domain.ExerciseSlot
		postSlotOK    bool
		periodization domain.PeriodizationType
		sessionDeload bool
	)
	now := time.Now().UTC()

	err := s.repos.WeekPlans.Update(ctx, domain.MondayOf(date), func(wp *domain.WeekPlan) error {
		sess := wp.SessionOn(date)
		if sess == nil {
			return domain.ErrNotFound
		}
		if pos >= 0 && pos < len(sess.Slots) {
			slot := sess.Slots[pos]
			if setIndex >= 0 && setIndex < len(slot.Sets) {
				wasComplete = slot.Sets[setIndex].CompletedAt != nil
			}
		}
		periodization = sess.PeriodizationType
		sessionDeload = sess.IsDeload

		if recErr := sess.RecordSet(pos, setIndex, signal, weightKg, completedValue, now); recErr != nil {
			// Domain sentinels propagate unchanged so callers can errors.Is at the call site;
			// the outer `if err != nil` wraps for diagnostic context.
			return recErr //nolint:wrapcheck // outer fmt.Errorf wraps with date context.
		}
		if pos >= 0 && pos < len(sess.Slots) {
			postSlot = sess.Slots[pos]
			postSlotOK = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}

	if !wasComplete && postSlotOK {
		userID := contexthelpers.AuthenticatedUserID(ctx)
		s.applyRestPushDecision(ctx, userID, date, pos, postSlot, periodization, sessionDeload, now)
	}
	return nil
}

// applyRestPushDecision runs the rest-push policy against the post-mutation
// slot and acts on the result. The completion itself is already persisted,
// so failures here just mean the user won't get a notification — never
// propagate. Every log line carries user_id, workout_date, and position so
// triage can filter by any of them.
func (s *Service) applyRestPushDecision(
	ctx context.Context,
	userID int,
	date time.Time,
	pos int,
	slot domain.ExerciseSlot,
	periodization domain.PeriodizationType,
	isDeload bool,
	completedAt time.Time,
) {
	if s.scheduler == nil {
		return
	}

	decision := domain.PlanRestPush(slot, periodization, isDeload, completedAt)
	switch decision.Action {
	case domain.RestPushActionNoOp:
		return
	case domain.RestPushActionCancel:
		if err := s.scheduler.Cancel(ctx, userID, date, pos); err != nil {
			s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: cancel failed",
				slog.Int("user_id", userID),
				slog.String("workout_date", date.Format(time.DateOnly)),
				slog.Int("position", pos),
				slog.Any("error", err))
		}
		return
	case domain.RestPushActionSchedule:
		// fall through
	}

	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: get preferences failed",
			slog.Int("user_id", userID),
			slog.String("workout_date", date.Format(time.DateOnly)),
			slog.Int("position", pos),
			slog.Any("error", err))
		return
	}
	if !prefs.RestNotificationsEnabled {
		return
	}
	subCount, err := s.repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: count subscriptions failed",
			slog.Int("user_id", userID),
			slog.String("workout_date", date.Format(time.DateOnly)),
			slog.Int("position", pos),
			slog.Any("error", err))
		return
	}
	if subCount == 0 {
		return
	}

	payloadBytes, err := json.Marshal(struct {
		Title        string `json:"title"`
		Body         string `json:"body"`
		ExerciseName string `json:"exercise_name"`
		SetNumber    int    `json:"set_number"`
		SetsTotal    int    `json:"sets_total"`
		FireAtMS     int64  `json:"fire_at_ms"`
	}{
		Title:        decision.Payload.Title,
		Body:         decision.Payload.Body,
		ExerciseName: decision.Payload.ExerciseName,
		SetNumber:    decision.Payload.NextSetNumber,
		SetsTotal:    decision.Payload.SetsTotal,
		FireAtMS:     decision.FireAt.UnixMilli(),
	})
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: marshal payload",
			slog.Int("user_id", userID),
			slog.String("workout_date", date.Format(time.DateOnly)),
			slog.Int("position", pos),
			slog.Any("error", err))
		return
	}

	push := domain.ScheduledPush{ //nolint:exhaustruct // ID and CreatedAt assigned by the repository at insert time.
		UserID:      userID,
		WorkoutDate: date,
		Position:    pos,
		FireAt:      decision.FireAt,
		Payload:     string(payloadBytes),
	}
	if err = s.scheduler.Schedule(ctx, push); err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: schedule failed",
			slog.Int("user_id", userID),
			slog.String("workout_date", date.Format(time.DateOnly)),
			slog.Int("position", pos),
			slog.Any("error", err))
	}
}
