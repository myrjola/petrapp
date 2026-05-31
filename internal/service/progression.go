package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

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
	targetType domain.PeriodizationType,
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
		false,
	).TargetReps
	toReps := domain.DeriveScheme(
		*exercise.RepMin, *exercise.RepMax,
		targetType,
		false,
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
	switch {
	case err == nil:
		return seconds, nil
	case errors.Is(err, domain.ErrNotFound):
		if exercise.DefaultStartingSeconds == nil {
			return 0, fmt.Errorf("time_based exercise %d has no default_starting_seconds", exerciseID)
		}
		return *exercise.DefaultStartingSeconds, nil
	default:
		return 0, fmt.Errorf("get latest successful seconds: %w", err)
	}
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

	var startingWeight float64
	if sess.IsDeload {
		startingWeight, err = s.GetDeloadStartingWeight(ctx, exerciseID, date)
	} else {
		startingWeight, err = s.GetStartingWeight(ctx, exerciseID, date, sess.PeriodizationType)
	}
	if err != nil {
		return nil, fmt.Errorf("get starting weight: %w", err)
	}

	config := domain.Config{
		Type:           sess.PeriodizationType,
		RepMin:         *exercise.RepMin,
		RepMax:         *exercise.RepMax,
		StartingWeight: startingWeight,
		IsDeload:       sess.IsDeload,
	}

	return domain.NewProgressionFromHistory(config, collectWeightedHistory(sess, exerciseID)), nil
}

// collectWeightedHistory returns the completed weighted sets for the given
// exercise in sess, in completion order. Deload sets are recorded without a
// signal (the form has only "Done!"), so a nil signal is expected for them
// and we keep the set so the deload progression can carry the user's weight
// forward; non-deload sessions still require a signal.
func collectWeightedHistory(sess domain.Session, exerciseID int) []domain.SetResult {
	var completed []domain.SetResult
	for _, es := range sess.Slots {
		if es.Exercise.ID != exerciseID {
			continue
		}
		for _, set := range es.Sets {
			if set.CompletedValue == nil {
				continue
			}
			if !sess.IsDeload && set.Signal == nil {
				continue
			}
			var kg float64
			if set.WeightKg != nil {
				kg = *set.WeightKg
			}
			var sig domain.Signal
			if set.Signal != nil {
				sig = *set.Signal
			}
			completed = append(completed, domain.SetResult{
				ActualReps: *set.CompletedValue,
				Signal:     sig,
				WeightKg:   kg,
			})
		}
		break
	}
	return completed
}

const (
	deloadFactor   = 0.90
	deloadFallback = 0.80
)

// GetDeloadStartingWeight returns the seed weight for a deload week's first
// set of the given exercise: 90% of the most recent hypertrophy working
// weight, falling back to 80% of any recent working weight, then to zero
// when no history exists. domain.DeloadSeedWeight rounds toward a
// definitely-loadable whole-kg load and handles the assisted-exercise sign
// convention (negative weight = machine assistance, larger magnitude =
// easier lift), so the deload always lands on the easier side of the
// calculation regardless of sign.
//
// The repository's GetLatestStartingWeightBefore already excludes deload
// sessions (Task 11), so the lookups below see only normal-week history.
func (s *Service) GetDeloadStartingWeight(
	ctx context.Context,
	exerciseID int,
	beforeDate time.Time,
) (float64, error) {
	prev, err := s.repos.Sessions.GetLatestStartingWeightBefore(ctx, exerciseID, beforeDate)
	if err != nil {
		return 0, fmt.Errorf("get latest starting weight: %w", err)
	}
	if prev.WeightKg == 0 {
		return 0, nil
	}
	factor := deloadFallback
	if prev.PeriodizationType == domain.PeriodizationHypertrophy {
		factor = deloadFactor
	}
	return domain.DeloadSeedWeight(prev.WeightKg, factor), nil
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
	for _, es := range sess.Slots {
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

	return domain.NewTimedProgressionFromHistory(
		domain.TimedConfig{StartingSeconds: starting},
		completed,
	), nil
}
