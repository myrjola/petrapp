package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
)

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

// RecordSet atomically persists the signal (nil for deload sets), weight
// (nil for time-based sets), completed value (reps or seconds depending on
// exercise type), and timestamp.
func (s *Service) RecordSet(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	signal *domain.Signal,
	weightKg *float64,
	completedValue int,
) error {
	var (
		wasComplete        bool
		exercise           domain.Exercise
		periodization      domain.PeriodizationType
		sessionIsDeload    bool
		completedSetNumber int
		setsTotal          int
		hasMoreAfter       bool
	)
	now := time.Now().UTC()

	err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		for i := range sess.ExerciseSets {
			if sess.ExerciseSets[i].ID != workoutExerciseID {
				continue
			}
			if setIndex < 0 || setIndex >= len(sess.ExerciseSets[i].Sets) {
				break
			}
			wasComplete = sess.ExerciseSets[i].Sets[setIndex].CompletedAt != nil
			exercise = sess.ExerciseSets[i].Exercise
			completedSetNumber = setIndex + 1
			setsTotal = len(sess.ExerciseSets[i].Sets)
			break
		}
		periodization = sess.PeriodizationType
		sessionIsDeload = sess.IsDeload

		if recErr := sess.RecordSet(workoutExerciseID, setIndex, signal, weightKg, completedValue, now); recErr != nil {
			// Domain sentinels propagate unchanged so callers can errors.Is at the call site;
			// the outer `if err != nil` wraps for diagnostic context.
			return recErr //nolint:wrapcheck // outer fmt.Errorf wraps with date context.
		}
		hasMoreAfter = sess.HasIncompleteSets()
		return nil
	})
	if err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}

	if !wasComplete && hasMoreAfter {
		s.maybeSchedulePush(ctx, workoutExerciseID, exercise, periodization, sessionIsDeload, completedSetNumber, setsTotal, now)
	}
	return nil
}

// maybeSchedulePush schedules a rest-over push if every precondition holds:
// the user has push enabled, has at least one subscription, and the exercise's
// derivation yields a positive RestSeconds. The completion itself is already
// persisted, so failures here just mean the user won't get a notification.
func (s *Service) maybeSchedulePush(
	ctx context.Context,
	workoutExerciseID int,
	exercise domain.Exercise,
	periodization domain.PeriodizationType,
	isDeload bool,
	completedSetNumber, setsTotal int,
	completedAt time.Time,
) {
	if s.scheduler == nil {
		return
	}
	restSeconds := domain.RestSecondsFor(exercise, periodization, isDeload)
	if restSeconds <= 0 {
		return
	}
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: get preferences failed",
			slog.Any("error", err))
		return
	}
	if !prefs.RestNotificationsEnabled {
		return
	}
	subCount, err := s.repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: count subscriptions failed",
			slog.Any("error", err))
		return
	}
	if subCount == 0 {
		return
	}
	userID := contexthelpers.AuthenticatedUserID(ctx)
	fireAt := completedAt.Add(time.Duration(restSeconds) * time.Second)
	// nextSetNumber is the upcoming set the notification is announcing. The
	// completion that triggered scheduling means the next set is the rest's reason.
	nextSetNumber := completedSetNumber + 1

	payloadBytes, err := json.Marshal(struct {
		Title        string `json:"title"`
		Body         string `json:"body"`
		ExerciseName string `json:"exercise_name"`
		SetNumber    int    `json:"set_number"`
		SetsTotal    int    `json:"sets_total"`
		FireAtMS     int64  `json:"fire_at_ms"`
	}{
		Title:        "Rest over",
		Body:         fmt.Sprintf("Time for set %d of %d — %s", nextSetNumber, setsTotal, exercise.Name),
		ExerciseName: exercise.Name,
		SetNumber:    nextSetNumber,
		SetsTotal:    setsTotal,
		FireAtMS:     fireAt.UnixMilli(),
	})
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: marshal payload",
			slog.Any("error", err))
		return
	}

	push := domain.ScheduledPush{ //nolint:exhaustruct // ID and CreatedAt assigned by the repository at insert time.
		UserID:            userID,
		WorkoutExerciseID: workoutExerciseID,
		FireAt:            fireAt,
		Payload:           string(payloadBytes),
	}
	if err = s.scheduler.Schedule(ctx, push); err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: schedule failed",
			slog.Any("error", err))
	}
}
