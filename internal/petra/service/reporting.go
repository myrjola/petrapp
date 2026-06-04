package service

import (
	"context"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

// GetSessionsWithExerciseSince retrieves all sessions since a given date that contain the specified exercise.
func (s *Service) GetSessionsWithExerciseSince(ctx context.Context, exerciseID int, since time.Time) (
	[]domain.Session, error,
) {
	sessions, err := s.repos.Sessions.List(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}

	var result []domain.Session
	for _, session := range sessions {
		for _, es := range session.Slots {
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
	domain.ExerciseProgress, error,
) {
	histories, err := s.repos.Sessions.ListSetsForExerciseSince(ctx, exerciseID, since)
	if err != nil {
		return domain.ExerciseProgress{}, fmt.Errorf("list sets for exercise: %w", err)
	}

	ex, err := s.repos.Exercises.Get(ctx, exerciseID)
	if err != nil {
		return domain.ExerciseProgress{}, fmt.Errorf("get exercise %d: %w", exerciseID, err)
	}

	entries := make([]domain.ExerciseProgressEntry, 0, len(histories))
	for _, h := range histories {
		var completedSets []domain.Set
		for _, set := range h.Sets {
			if set.CompletedValue != nil {
				completedSets = append(completedSets, set)
			}
		}
		if len(completedSets) > 0 {
			entries = append(entries, domain.ExerciseProgressEntry{
				Date: h.Date,
				Sets: completedSets,
			})
		}
	}

	return domain.ExerciseProgress{Exercise: ex, Entries: entries}, nil
}

// WeeklyMuscleGroupVolume aggregates planned-vs-completed weekly load per muscle
// group across the supplied sessions. One entry is returned for every known
// muscle group, sorted alphabetically; groups with no contributions appear as
// zero-load rows so the UI can render them without a separate query. Targets are
// joined from muscle_group_weekly_targets; untracked groups carry TargetSets = 0.
func (s *Service) WeeklyMuscleGroupVolume(
	ctx context.Context,
	sessions []domain.Session,
) ([]domain.MuscleGroupVolume, error) {
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
