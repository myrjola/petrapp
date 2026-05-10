package domain

import "time"

// PeriodizationType is the rep-target style for a session. The two values
// alternate week-to-week (see Planner.firstSessionPeriodizationType) and
// determine the rep target via DeriveScheme.
type PeriodizationType string

const (
	PeriodizationStrength    PeriodizationType = "strength"
	PeriodizationHypertrophy PeriodizationType = "hypertrophy"
)

// ExerciseSet groups all sets for a specific exercise in a workout.
type ExerciseSet struct {
	// ID is the stable identifier of this exercise slot within the workout. It survives
	// swapping the exercise to a different one, which is what keeps URLs stable.
	ID                int
	Exercise          Exercise
	Sets              []Set
	WarmupCompletedAt *time.Time // Nullable timestamp when warmup for this exercise was completed
}

// ExerciseProgressEntry represents the sets performed for an exercise on a specific date.
type ExerciseProgressEntry struct {
	Date time.Time
	Sets []Set
}

// ExerciseProgress represents an exercise with its historical performance data across sessions.
type ExerciseProgress struct {
	Exercise Exercise
	Entries  []ExerciseProgressEntry
}

// Session represents a complete workout session including all exercises and their sets.
type Session struct {
	Date              time.Time
	DifficultyRating  *int
	StartedAt         time.Time
	CompletedAt       time.Time
	ExerciseSets      []ExerciseSet
	PeriodizationType PeriodizationType
}

// Start marks the session as begun at now. Returns ErrAlreadyStarted if the
// session was previously started; the existing StartedAt is left untouched
// in that case.
func (s *Session) Start(now time.Time) error {
	if !s.StartedAt.IsZero() {
		return ErrAlreadyStarted
	}
	s.StartedAt = now
	return nil
}

// Complete marks the session as finished at now. Returns ErrNotStarted if
// the session has not been started yet — completion implies a prior start.
func (s *Session) Complete(now time.Time) error {
	if s.StartedAt.IsZero() {
		return ErrNotStarted
	}
	s.CompletedAt = now
	return nil
}

// SetDifficulty records the post-session difficulty rating (1-5). Returns
// ErrInvalidDifficultyRating when rating is outside that range.
func (s *Session) SetDifficulty(rating int) error {
	if rating < 1 || rating > 5 {
		return ErrInvalidDifficultyRating
	}
	s.DifficultyRating = &rating
	return nil
}

// MarkWarmupComplete records the warmup completion timestamp for the
// exercise slot identified by slotID. Returns ErrSlotNotFound if no slot
// matches.
func (s *Session) MarkWarmupComplete(slotID int, now time.Time) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID == slotID {
			s.ExerciseSets[i].WarmupCompletedAt = &now
			return nil
		}
	}
	return ErrSlotNotFound
}
