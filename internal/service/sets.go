package service

import (
	"context"
	"fmt"
	"time"

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

// RecordSet atomically persists the signal, weight (nil for time-based sets),
// completed value (reps or seconds depending on exercise type), and timestamp.
func (s *Service) RecordSet(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	signal domain.Signal,
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
