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
	IsDeload          bool
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

// RecordSet records the completion of a single set: signal (perceived
// effort, nil for deload sets), weight (nil for time-based exercises),
// the actual value (reps or seconds), and the completion timestamp.
// Returns ErrSlotNotFound or ErrSetIndexOutOfBounds when the lookup fails.
func (s *Session) RecordSet(
	slotID, setIndex int,
	signal *Signal,
	weightKg *float64,
	completedValue int,
	now time.Time,
) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID != slotID {
			continue
		}
		if setIndex < 0 || setIndex >= len(s.ExerciseSets[i].Sets) {
			return ErrSetIndexOutOfBounds
		}
		set := &s.ExerciseSets[i].Sets[setIndex]
		if signal != nil {
			sigCopy := *signal
			set.Signal = &sigCopy
		} else {
			set.Signal = nil
		}
		if weightKg != nil {
			w := *weightKg
			set.WeightKg = &w
		}
		v := completedValue
		set.CompletedValue = &v
		t := now
		set.CompletedAt = &t
		return nil
	}
	return ErrSlotNotFound
}

// UpdateCompletedValue records the actual reps (or seconds for time-based)
// achieved on a set, and stamps the completion time. Returns
// ErrSlotNotFound or ErrSetIndexOutOfBounds when the lookup fails.
func (s *Session) UpdateCompletedValue(slotID, setIndex, value int, now time.Time) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID != slotID {
			continue
		}
		if setIndex < 0 || setIndex >= len(s.ExerciseSets[i].Sets) {
			return ErrSetIndexOutOfBounds
		}
		v := value
		s.ExerciseSets[i].Sets[setIndex].CompletedValue = &v
		t := now
		s.ExerciseSets[i].Sets[setIndex].CompletedAt = &t
		return nil
	}
	return ErrSlotNotFound
}

// AddExercise appends a new exercise slot to the session. The slot's stable
// ID is left as 0 — the repository assigns it at insert time, then mirrors
// the assigned ID back to the caller. Returns ErrExerciseAlreadyInSession
// when an existing slot already references the same Exercise.ID.
//
// The returned slotID is always 0 from the aggregate's POV; the actual
// workout_exercise.id is determined by SQLite. Service code reads the new
// ID by re-fetching the session after the Update closure commits.
func (s *Session) AddExercise(ex Exercise, sets []Set) (int, error) {
	for _, existing := range s.ExerciseSets {
		if existing.Exercise.ID == ex.ID {
			return 0, ErrExerciseAlreadyInSession
		}
	}
	s.ExerciseSets = append(s.ExerciseSets, ExerciseSet{ //nolint:exhaustruct // ID set by repo; WarmupCompletedAt nil.
		ID:       0,
		Exercise: ex,
		Sets:     sets,
	})
	return 0, nil
}

// SwapExerciseInSlot replaces the exercise occupying the slot identified by
// slotID with newExercise. The slot's stable ID is preserved (so URLs
// continue to resolve). The new sets slice replaces the slot's existing
// sets entirely; any prior recorded data is dropped. The warmup-completion
// flag is reset to nil because the warmup performed for the old exercise
// does not apply to the new one. Returns ErrSlotNotFound when no slot
// matches.
func (s *Session) SwapExerciseInSlot(slotID int, newExercise Exercise, sets []Set) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID != slotID {
			continue
		}
		s.ExerciseSets[i].Exercise = newExercise
		s.ExerciseSets[i].Sets = sets
		s.ExerciseSets[i].WarmupCompletedAt = nil
		return nil
	}
	return ErrSlotNotFound
}

// UpdateSetWeight overwrites the weight on a single set within a slot.
// Returns ErrSlotNotFound or ErrSetIndexOutOfBounds when the lookup fails.
func (s *Session) UpdateSetWeight(slotID, setIndex int, weightKg float64) error {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID != slotID {
			continue
		}
		if setIndex < 0 || setIndex >= len(s.ExerciseSets[i].Sets) {
			return ErrSetIndexOutOfBounds
		}
		w := weightKg
		s.ExerciseSets[i].Sets[setIndex].WeightKg = &w
		return nil
	}
	return ErrSlotNotFound
}

// HasIncompleteSets reports whether any set across any exercise slot in the
// session has not yet been completed. Used by the service layer to decide
// whether a just-completed set is the final set of the workout — if so, no
// rest push should be scheduled.
func (s *Session) HasIncompleteSets() bool {
	for i := range s.ExerciseSets {
		for j := range s.ExerciseSets[i].Sets {
			if s.ExerciseSets[i].Sets[j].CompletedAt == nil {
				return true
			}
		}
	}
	return false
}
