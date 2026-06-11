package domain

import "time"

// SessionGoal is the rep-target style for a session. Consecutive sessions
// alternate between the two values and the week's starting goal flips each
// week (see Planner.firstSessionGoal / nextSessionGoal) — the literature's
// daily undulating periodization. The goal determines the rep target via
// DeriveScheme.
type SessionGoal string

const (
	SessionGoalStrength    SessionGoal = "strength"
	SessionGoalHypertrophy SessionGoal = "hypertrophy"
)

// SessionStatus is the lifecycle state of a workout session, for display.
type SessionStatus string

const (
	SessionNotStarted SessionStatus = "not_started"
	SessionInProgress SessionStatus = "in_progress"
	SessionCompleted  SessionStatus = "completed"
)

// ExerciseSlot is one slot in a Session: an exercise plus its sets. Slot
// identity is the slot's position in Session.Slots — there is no
// surrogate ID. Position is stable under SwapExerciseInSlot, so URLs and
// schedule keys keyed on it survive swaps.
type ExerciseSlot struct {
	Exercise          Exercise
	Sets              []Set
	WarmupCompletedAt *time.Time // Nullable timestamp when warmup for this exercise was completed
}

// ExerciseSlotState is the completion state of an exercise slot, for display.
type ExerciseSlotState string

const (
	ExerciseSlotNotStarted ExerciseSlotState = "not-started"
	ExerciseSlotStarted    ExerciseSlotState = "started"
	ExerciseSlotCompleted  ExerciseSlotState = "completed"
)

// CompletionState reports whether none, some, or all of the slot's sets have
// been completed. A slot with no sets is reported as not started. The string
// values double as CSS state tokens used by the workout page.
func (es ExerciseSlot) CompletionState() ExerciseSlotState {
	completed := es.CompletedSetCount()
	if len(es.Sets) == 0 {
		return ExerciseSlotNotStarted
	}
	switch completed {
	case 0:
		return ExerciseSlotNotStarted
	case len(es.Sets):
		return ExerciseSlotCompleted
	default:
		return ExerciseSlotStarted
	}
}

// CompletedSetCount returns how many of the slot's sets have been completed.
func (es ExerciseSlot) CompletedSetCount() int {
	n := 0
	for i := range es.Sets {
		if es.Sets[i].CompletedAt != nil {
			n++
		}
	}
	return n
}

// RestEndAt returns when this slot's inter-set rest is scheduled to end
// and whether a rest chip should be shown at all. The chip should appear
// once the warmup is done and at least one set remains incomplete with a
// defined rest period — the rest clock starts at the latest of the warmup
// completion and the most recent set completion. The returned time may be
// in the past; the on-screen chip renders that as "Ready" so a user who
// rotates through other exercises (power sets) and returns later still
// sees the slot's rest state instead of nothing.
func (es ExerciseSlot) RestEndAt(goal SessionGoal, isDeload bool) (time.Time, bool) {
	if es.WarmupCompletedAt == nil {
		return time.Time{}, false
	}
	incomplete := false
	var lastCompleted *time.Time
	for i := range es.Sets {
		s := &es.Sets[i]
		if s.CompletedAt == nil {
			incomplete = true
			continue
		}
		if lastCompleted == nil || s.CompletedAt.After(*lastCompleted) {
			lastCompleted = s.CompletedAt
		}
	}
	if !incomplete {
		return time.Time{}, false
	}
	restSeconds := RestSecondsFor(es.Exercise, goal, isDeload)
	if restSeconds <= 0 {
		return time.Time{}, false
	}
	clockStart := *es.WarmupCompletedAt
	if lastCompleted != nil && lastCompleted.After(clockStart) {
		clockStart = *lastCompleted
	}
	return clockStart.Add(time.Duration(restSeconds) * time.Second), true
}

// setAt returns a pointer to the set at setIndex, or ErrSetIndexOutOfBounds
// when setIndex is out of range. The value receiver still yields a usable
// pointer: es.Sets shares its backing array with the caller's slot, so
// mutations through the returned pointer land on the original set.
func (es ExerciseSlot) setAt(setIndex int) (*Set, error) {
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
	Date             time.Time
	DifficultyRating *int
	StartedAt        time.Time
	CompletedAt      time.Time
	Slots            []ExerciseSlot
	Goal             SessionGoal
	IsDeload         bool
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

// SwitchToDeload marks the session as a deload session and rebuilds each
// slot's uncompleted sets to match the deload prescription. Completed sets
// (CompletedAt != nil) are preserved verbatim — work already done is never
// erased. Idempotent: applying twice produces the same Sets slice.
func (s *Session) SwitchToDeload(weekSets int) error {
	s.IsDeload = true
	s.rebuildUncompletedSetsForCurrentPrescription(weekSets)
	return nil
}

// ClearDeload marks the session as a non-deload session and rebuilds each
// slot's uncompleted sets to match the non-deload prescription. Counterpart
// to SwitchToDeload; used by RestartMesocycleAnchor to undo an ad-hoc early
// deload. Completed sets are preserved verbatim. Idempotent.
func (s *Session) ClearDeload(weekSets int) error {
	s.IsDeload = false
	s.rebuildUncompletedSetsForCurrentPrescription(weekSets)
	return nil
}

// SeedDeloadWeights applies per-exercise deload seed weights to every set of
// the matching slots. weights is keyed by exercise ID; slots whose exercise has
// no entry (e.g. exercises without weight) are left untouched. No-op on a
// non-deload session — seed weights are a deload-week concept. Each set gets
// its own copy of the weight so a later per-set update cannot alias across
// sets.
func (s *Session) SeedDeloadWeights(weights map[int]float64) {
	if !s.IsDeload {
		return
	}
	for i := range s.Slots {
		w, ok := weights[s.Slots[i].Exercise.ID]
		if !ok {
			continue
		}
		for k := range s.Slots[i].Sets {
			wc := w
			s.Slots[i].Sets[k].WeightKg = &wc
		}
	}
}

// MarkWarmupComplete records the warmup completion timestamp for the
// exercise slot at pos. Returns ErrSlotNotFound when pos is out of range.
func (s *Session) MarkWarmupComplete(pos int, now time.Time) error {
	slot, err := s.slotAt(pos)
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
	pos, setIndex int,
	signal *Signal,
	weightKg *float64,
	completedValue int,
	now time.Time,
) error {
	slot, err := s.slotAt(pos)
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
func (s *Session) UpdateCompletedValue(pos, setIndex, value int, now time.Time) error {
	slot, err := s.slotAt(pos)
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

// AddExercise appends a new exercise slot to the session. The slot's position
// is len(s.Slots) at the time of the append, persisted by the
// repository as the row's position column. Returns
// ErrExerciseAlreadyInSession when an existing slot already references the
// same Exercise.ID.
func (s *Session) AddExercise(ex Exercise, sets []Set) error {
	for _, existing := range s.Slots {
		if existing.Exercise.ID == ex.ID {
			return ErrExerciseAlreadyInSession
		}
	}
	s.Slots = append(s.Slots, ExerciseSlot{ //nolint:exhaustruct // WarmupCompletedAt nil.
		Exercise: ex,
		Sets:     sets,
	})
	return nil
}

// SwapExerciseInSlot replaces the exercise occupying the slot at pos with
// newExercise. The slot's position is preserved (so URLs and schedule keys
// continue to resolve). The new sets slice replaces the slot's existing
// sets entirely; any prior recorded data is dropped. The warmup-completion
// flag is reset to nil because the warmup performed for the old exercise
// does not apply to the new one. Returns ErrSlotNotFound when pos is out of
// range.
func (s *Session) SwapExerciseInSlot(pos int, newExercise Exercise, sets []Set) error {
	slot, err := s.slotAt(pos)
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
func (s *Session) UpdateSetWeight(pos, setIndex int, weightKg float64) error {
	slot, err := s.slotAt(pos)
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
	for i := range s.Slots {
		switch s.Slots[i].Exercise.Category {
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
	for i := range s.Slots {
		if s.Slots[i].CompletionState() == ExerciseSlotCompleted {
			n++
		}
	}
	return n
}

// IncompleteExerciseCount returns how many of the session's exercise slots
// still have at least one set to complete (including slots not started).
func (s Session) IncompleteExerciseCount() int {
	return len(s.Slots) - s.CompletedExerciseCount()
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
	for i := range s.Slots {
		for j := range s.Slots[i].Sets {
			if s.Slots[i].Sets[j].CompletedAt == nil {
				return true
			}
		}
	}
	return false
}

// rebuildUncompletedSetsForCurrentPrescription rewrites each slot's Sets so
// completed sets stay verbatim and uncompleted sets match what BuildPlannedSets
// would emit for the session's current SessionGoal and IsDeload. Called
// from SwitchToDeload and ClearDeload after toggling the flag so the persisted
// shape never diverges from what a fresh BuildSetsForAdd would produce.
//
// The new length is max(len(completed), planner-prescribed n). Work already
// done is never erased, but a shrinking prescription only truncates the
// uncompleted tail.
func (s *Session) rebuildUncompletedSetsForCurrentPrescription(weekSets int) {
	for i := range s.Slots {
		slot := &s.Slots[i]
		fresh := BuildPlannedSets(slot.Exercise, s.Goal, s.IsDeload, weekSets)
		n := len(fresh)

		var completed []Set
		for _, st := range slot.Sets {
			if st.CompletedAt != nil {
				completed = append(completed, st)
			}
		}
		final := max(n, len(completed))

		rebuilt := make([]Set, final)
		copy(rebuilt, completed)
		for j := len(completed); j < final; j++ {
			// fresh[0] suffices: BuildPlannedSets emits identical sets.
			rebuilt[j] = fresh[0]
		}
		slot.Sets = rebuilt
	}
}

// slotAt returns a pointer to the exercise slot at pos within Slots,
// or ErrSlotNotFound when pos is out of range. The pointer aliases into
// Slots so callers can mutate the slot in place.
func (s *Session) slotAt(pos int) (*ExerciseSlot, error) {
	if pos < 0 || pos >= len(s.Slots) {
		return nil, ErrSlotNotFound
	}
	return &s.Slots[pos], nil
}
