package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

// List returns all available exercises.
func (s *Service) List(ctx context.Context) ([]domain.Exercise, error) {
	exercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list exercises: %w", err)
	}
	return exercises, nil
}

// GetExercise retrieves a specific exercise by ID.
func (s *Service) GetExercise(ctx context.Context, id int) (domain.Exercise, error) {
	exercise, err := s.repos.Exercises.Get(ctx, id)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("get exercise: %w", err)
	}
	return exercise, nil
}

// UpdateExercise updates an existing exercise.
func (s *Service) UpdateExercise(ctx context.Context, ex domain.Exercise) error {
	if err := s.repos.Exercises.Update(ctx, ex.ID, func(oldEx *domain.Exercise) error {
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
func (s *Service) findHistoricalSets(ctx context.Context, date time.Time, exerciseID int) ([]domain.Set, error) {
	threeMonthsAgo := date.AddDate(0, -3, 0)
	history, err := s.repos.Sessions.List(ctx, threeMonthsAgo)
	if err != nil {
		return nil, fmt.Errorf("get workout history: %w", err)
	}

	for _, session := range history {
		if session.Date.Equal(date) {
			continue
		}

		for _, exerciseSet := range session.ExerciseSets {
			if exerciseSet.Exercise.ID != exerciseID || len(exerciseSet.Sets) == 0 {
				continue
			}
			return s.copySetsWithoutCompletion(exerciseSet.Sets), nil
		}
	}

	return nil, nil
}

// copySetsWithoutCompletion creates a copy of sets with completion reset to nil.
// Note: callers in the AddExercise/swap paths route the result through
// buildSetsForAdd, which overrides TargetValue from the session's periodization.
// This function preserves all fields verbatim including TargetValue.
func (s *Service) copySetsWithoutCompletion(sets []domain.Set) []domain.Set {
	result := make([]domain.Set, len(sets))
	for i, set := range sets {
		result[i] = domain.Set{
			WeightKg:       set.WeightKg,
			TargetValue:    set.TargetValue,
			CompletedValue: nil,
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
func (s *Service) buildSetsForAdd(
	ex domain.Exercise,
	pt domain.PeriodizationType,
	historicalSets []domain.Set,
) []domain.Set {
	sets := domain.BuildPlannedSets(ex, pt)
	// Allocate empty weight pointers for weighted/assisted exercises. The
	// form input on the per-set page binds to *float64; nil would render
	// as "no weight" instead of an empty editable input. Bodyweight and
	// time-based stay nil.
	if !ex.IsTimed() && ex.ExerciseType != domain.ExerciseTypeBodyweight {
		for i := range sets {
			sets[i].WeightKg = new(float64)
		}
	}
	if len(historicalSets) == 0 {
		return sets
	}
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
		if !ex.IsTimed() && ex.ExerciseType != domain.ExerciseTypeBodyweight {
			w := *seedWeight
			sets[i].WeightKg = &w
		}
	}
	return sets
}

// FindCompatibleExercises returns all exercises except the specified one.
func (s *Service) FindCompatibleExercises(ctx context.Context, exerciseID int) ([]domain.Exercise, error) {
	allExercises, err := s.repos.Exercises.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all exercises: %w", err)
	}

	var otherExercises []domain.Exercise
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
