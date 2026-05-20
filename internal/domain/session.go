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

// SessionStatus is the lifecycle state of a workout session, for display.
type SessionStatus string

const (
	SessionNotStarted SessionStatus = "not_started"
	SessionInProgress SessionStatus = "in_progress"
	SessionCompleted  SessionStatus = "completed"
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

// ExerciseSetState is the completion state of an exercise slot, for display.
type ExerciseSetState string

const (
	ExerciseSetNotStarted ExerciseSetState = "not-started"
	ExerciseSetStarted    ExerciseSetState = "started"
	ExerciseSetCompleted  ExerciseSetState = "completed"
)

// CompletionState reports whether none, some, or all of the slot's sets have
// been completed. A slot with no sets is reported as not started. The string
// values double as CSS state tokens used by the workout page.
func (es ExerciseSet) CompletionState() ExerciseSetState {
	completed := es.CompletedSetCount()
	if len(es.Sets) == 0 {
		return ExerciseSetNotStarted
	}
	switch completed {
	case 0:
		return ExerciseSetNotStarted
	case len(es.Sets):
		return ExerciseSetCompleted
	default:
		return ExerciseSetStarted
	}
}

// CompletedSetCount returns how many of the slot's sets have been completed.
func (es ExerciseSet) CompletedSetCount() int {
	n := 0
	for i := range es.Sets {
		if es.Sets[i].CompletedAt != nil {
			n++
		}
	}
	return n
}

// setAt returns a pointer to the set at setIndex, or ErrSetIndexOutOfBounds
// when setIndex is out of range. The value receiver still yields a usable
// pointer: es.Sets shares its backing array with the caller's slot, so
// mutations through the returned pointer land on the original set.
func (es ExerciseSet) setAt(setIndex int) (*Set, error) {
	if setIndex < 0 || setIndex >= len(es.Sets) {
		return nil, ErrSetIndexOutOfBounds
	}
	return &es.Sets[setIndex], nil
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
//
//nolint:recvcheck // WorkoutType uses a value receiver so templates can call it on non-addressable Session values.
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

// findSlot returns a pointer to the exercise slot identified by slotID, or
// ErrSlotNotFound when no slot matches. The pointer aliases into
// ExerciseSets so callers can mutate the slot in place.
func (s *Session) findSlot(slotID int) (*ExerciseSet, error) {
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].ID == slotID {
			return &s.ExerciseSets[i], nil
		}
	}
	return nil, ErrSlotNotFound
}

// MarkWarmupComplete records the warmup completion timestamp for the
// exercise slot identified by slotID. Returns ErrSlotNotFound if no slot
// matches.
func (s *Session) MarkWarmupComplete(slotID int, now time.Time) error {
	slot, err := s.findSlot(slotID)
	if err != nil {
		return err
	}
	slot.WarmupCompletedAt = &now
	return nil
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
	slot, err := s.findSlot(slotID)
	if err != nil {
		return err
	}
	set, err := slot.setAt(setIndex)
	if err != nil {
		return err
	}
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

// UpdateCompletedValue records the actual reps (or seconds for time-based)
// achieved on a set, and stamps the completion time. Returns
// ErrSlotNotFound or ErrSetIndexOutOfBounds when the lookup fails.
func (s *Session) UpdateCompletedValue(slotID, setIndex, value int, now time.Time) error {
	slot, err := s.findSlot(slotID)
	if err != nil {
		return err
	}
	set, err := slot.setAt(setIndex)
	if err != nil {
		return err
	}
	v := value
	set.CompletedValue = &v
	t := now
	set.CompletedAt = &t
	return nil
}

// AddExercise appends a new exercise slot to the session. The slot's stable
// ID is left as 0 — the repository assigns the workout_exercise.id at insert
// time. Returns ErrExerciseAlreadyInSession when an existing slot already
// references the same Exercise.ID.
//
// Service code that needs the assigned ID re-fetches the session after the
// Update closure commits; the aggregate itself never observes it.
func (s *Session) AddExercise(ex Exercise, sets []Set) error {
	for _, existing := range s.ExerciseSets {
		if existing.Exercise.ID == ex.ID {
			return ErrExerciseAlreadyInSession
		}
	}
	s.ExerciseSets = append(s.ExerciseSets, ExerciseSet{ //nolint:exhaustruct // ID set by repo; WarmupCompletedAt nil.
		ID:       0,
		Exercise: ex,
		Sets:     sets,
	})
	return nil
}

// SwapExerciseInSlot replaces the exercise occupying the slot identified by
// slotID with newExercise. The slot's stable ID is preserved (so URLs
// continue to resolve). The new sets slice replaces the slot's existing
// sets entirely; any prior recorded data is dropped. The warmup-completion
// flag is reset to nil because the warmup performed for the old exercise
// does not apply to the new one. Returns ErrSlotNotFound when no slot
// matches.
func (s *Session) SwapExerciseInSlot(slotID int, newExercise Exercise, sets []Set) error {
	slot, err := s.findSlot(slotID)
	if err != nil {
		return err
	}
	slot.Exercise = newExercise
	slot.Sets = sets
	slot.WarmupCompletedAt = nil
	return nil
}

// UpdateSetWeight overwrites the weight on a single set within a slot.
// Returns ErrSlotNotFound or ErrSetIndexOutOfBounds when the lookup fails.
func (s *Session) UpdateSetWeight(slotID, setIndex int, weightKg float64) error {
	slot, err := s.findSlot(slotID)
	if err != nil {
		return err
	}
	set, err := slot.setAt(setIndex)
	if err != nil {
		return err
	}
	w := weightKg
	set.WeightKg = &w
	return nil
}

// WorkoutType derives the muscle-split category for the session from the
// categories of its exercise slots: full body if any full-body exercise is
// present or both upper and lower are represented, otherwise whichever of
// upper or lower is present. An empty session defaults to full body.
func (s Session) WorkoutType() Category {
	hasUpper, hasLower := false, false
	for i := range s.ExerciseSets {
		switch s.ExerciseSets[i].Exercise.Category {
		case CategoryFullBody:
			return CategoryFullBody
		case CategoryUpper:
			hasUpper = true
		case CategoryLower:
			hasLower = true
		}
	}
	if hasUpper && hasLower {
		return CategoryFullBody
	}
	if hasUpper {
		return CategoryUpper
	}
	if hasLower {
		return CategoryLower
	}
	return CategoryFullBody
}

// CompletedExerciseCount returns how many of the session's exercise slots have
// every set completed. Used by the workout overview to show progress.
func (s Session) CompletedExerciseCount() int {
	n := 0
	for i := range s.ExerciseSets {
		if s.ExerciseSets[i].CompletionState() == ExerciseSetCompleted {
			n++
		}
	}
	return n
}

// IncompleteExerciseCount returns how many of the session's exercise slots
// still have at least one set to complete (including slots not started).
func (s Session) IncompleteExerciseCount() int {
	return len(s.ExerciseSets) - s.CompletedExerciseCount()
}

// Status reports the session's lifecycle state from its timestamps.
func (s Session) Status() SessionStatus {
	if !s.CompletedAt.IsZero() {
		return SessionCompleted
	}
	if !s.StartedAt.IsZero() {
		return SessionInProgress
	}
	return SessionNotStarted
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
