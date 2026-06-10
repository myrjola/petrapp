package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

func Test_Session_Start_FromZero(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	}

	if err := sess.Start(now); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !sess.StartedAt.Equal(now) {
		t.Errorf("StartedAt = %v, want %v", sess.StartedAt, now)
	}
}

func Test_Session_Start_AlreadyStarted_ReturnsErrAlreadyStarted(t *testing.T) {
	t.Parallel()

	earlier := time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date:      time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		StartedAt: earlier,
	}

	err := sess.Start(now)
	if !errors.Is(err, domain.ErrAlreadyStarted) {
		t.Fatalf("Start: got %v, want ErrAlreadyStarted", err)
	}
	if !sess.StartedAt.Equal(earlier) {
		t.Errorf("StartedAt mutated to %v, want %v (unchanged)", sess.StartedAt, earlier)
	}
}

func Test_Session_Complete_AfterStart(t *testing.T) {
	t.Parallel()

	startAt := time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date:      time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
		StartedAt: startAt,
	}

	if err := sess.Complete(now); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !sess.CompletedAt.Equal(now) {
		t.Errorf("CompletedAt = %v, want %v", sess.CompletedAt, now)
	}
}

func Test_Session_Complete_NotStarted_ReturnsErrNotStarted(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date: time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC),
	}

	err := sess.Complete(now)
	if !errors.Is(err, domain.ErrNotStarted) {
		t.Fatalf("Complete: got %v, want ErrNotStarted", err)
	}
	if !sess.CompletedAt.IsZero() {
		t.Errorf("CompletedAt = %v, want zero", sess.CompletedAt)
	}
}

func Test_Session_SetDifficulty_ValidRange(t *testing.T) {
	t.Parallel()

	for _, rating := range []int{1, 2, 3, 4, 5} {
		sess := domain.Session{} //nolint:exhaustruct // Test sessions omit irrelevant fields.
		if err := sess.SetDifficulty(rating); err != nil {
			t.Errorf("SetDifficulty(%d): %v", rating, err)
		}
		if sess.DifficultyRating == nil || *sess.DifficultyRating != rating {
			t.Errorf("DifficultyRating = %v, want %d", sess.DifficultyRating, rating)
		}
	}
}

func Test_Session_SetDifficulty_OutOfRange(t *testing.T) {
	t.Parallel()

	for _, rating := range []int{0, -1, 6, 100} {
		sess := domain.Session{} //nolint:exhaustruct // Test sessions omit irrelevant fields.
		err := sess.SetDifficulty(rating)
		if !errors.Is(err, domain.ErrInvalidDifficultyRating) {
			t.Errorf("SetDifficulty(%d): got %v, want ErrInvalidDifficultyRating", rating, err)
		}
		if sess.DifficultyRating != nil {
			t.Errorf("DifficultyRating mutated to %v, want nil", sess.DifficultyRating)
		}
	}
}

func Test_Session_MarkWarmupComplete_KnownSlot(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // Test sets omit irrelevant fields.
				Exercise: domain.Exercise{ID: 1}, //nolint:exhaustruct // Test exercises omit display fields.
			},
			{ //nolint:exhaustruct // Test sets omit irrelevant fields.
				Exercise: domain.Exercise{ID: 2}, //nolint:exhaustruct // Test exercises omit display fields.
			},
		},
	}

	if err := sess.MarkWarmupComplete(1, now); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}
	if sess.Slots[1].WarmupCompletedAt == nil || !sess.Slots[1].WarmupCompletedAt.Equal(now) {
		t.Errorf("pos 1 WarmupCompletedAt = %v, want %v", sess.Slots[1].WarmupCompletedAt, now)
	}
	if sess.Slots[0].WarmupCompletedAt != nil {
		t.Errorf("pos 0 WarmupCompletedAt mutated to %v, want nil", sess.Slots[0].WarmupCompletedAt)
	}
}

func Test_Session_MarkWarmupComplete_OutOfRange(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // Test sets omit irrelevant fields.
				Exercise: domain.Exercise{ID: 1}, //nolint:exhaustruct // Test exercises omit display fields.
			},
		},
	}

	err := sess.MarkWarmupComplete(99, now)
	if !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}

func Test_Session_UpdateSetWeight_KnownSlotAndIndex(t *testing.T) {
	t.Parallel()

	sess := domain.Session{ //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: domain.Exercise{ID: 1}, //nolint:exhaustruct // Only Exercise.ID is read.
				Sets: []domain.Set{
					{TargetValue: 5}, //nolint:exhaustruct // Other fields nil.
					{TargetValue: 5}, //nolint:exhaustruct // Other fields nil.
				},
			},
		},
	}

	if err := sess.UpdateSetWeight(0, 1, 80.0); err != nil {
		t.Fatalf("UpdateSetWeight: %v", err)
	}
	if sess.Slots[0].Sets[1].WeightKg == nil || *sess.Slots[0].Sets[1].WeightKg != 80.0 {
		t.Errorf("WeightKg = %v, want 80.0", sess.Slots[0].Sets[1].WeightKg)
	}
	if sess.Slots[0].Sets[0].WeightKg != nil {
		t.Errorf("set 0 WeightKg mutated to %v, want nil", sess.Slots[0].Sets[0].WeightKg)
	}
}

func Test_Session_UpdateSetWeight_OutOfRange(t *testing.T) {
	t.Parallel()

	sess := domain.Session{} //nolint:exhaustruct // Empty session.
	err := sess.UpdateSetWeight(99, 0, 80.0)
	if !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}

func Test_Session_UpdateSetWeight_OutOfBoundsIndex(t *testing.T) {
	t.Parallel()

	sess := domain.Session{ //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: domain.Exercise{ID: 1},         //nolint:exhaustruct // Only Exercise.ID is read.
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	err := sess.UpdateSetWeight(0, 5, 80.0)
	if !errors.Is(err, domain.ErrSetIndexOutOfBounds) {
		t.Fatalf("got %v, want ErrSetIndexOutOfBounds", err)
	}
}

func Test_Session_UpdateCompletedValue_Sets(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: domain.Exercise{ID: 1},         //nolint:exhaustruct // Only Exercise.ID is read.
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}

	if err := sess.UpdateCompletedValue(0, 0, 6, now); err != nil {
		t.Fatalf("UpdateCompletedValue: %v", err)
	}
	got := sess.Slots[0].Sets[0]
	if got.CompletedValue == nil || *got.CompletedValue != 6 {
		t.Errorf("CompletedValue = %v, want 6", got.CompletedValue)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(now) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, now)
	}
}

func Test_Session_UpdateCompletedValue_OutOfRange(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{} //nolint:exhaustruct // Empty session.
	if err := sess.UpdateCompletedValue(99, 0, 6, now); !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}

func Test_Session_UpdateCompletedValue_OutOfBoundsIndex(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: domain.Exercise{ID: 1},         //nolint:exhaustruct // Only Exercise.ID is read.
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	if err := sess.UpdateCompletedValue(0, 5, 6, now); !errors.Is(err, domain.ErrSetIndexOutOfBounds) {
		t.Fatalf("got %v, want ErrSetIndexOutOfBounds", err)
	}
}

func Test_Session_RecordSet_Weighted(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	weight := 80.0
	sess := domain.Session{ //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: domain.Exercise{ID: 1},         //nolint:exhaustruct // Only Exercise.ID is read.
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}

	sig := domain.SignalOnTarget
	err := sess.RecordSet(0, 0, &sig, &weight, 5, now)
	if err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	got := sess.Slots[0].Sets[0]
	if got.WeightKg == nil || *got.WeightKg != weight {
		t.Errorf("WeightKg = %v, want %v", got.WeightKg, weight)
	}
	if got.CompletedValue == nil || *got.CompletedValue != 5 {
		t.Errorf("CompletedValue = %v, want 5", got.CompletedValue)
	}
	if got.Signal == nil || *got.Signal != domain.SignalOnTarget {
		t.Errorf("Signal = %v, want SignalOnTarget", got.Signal)
	}
	if got.CompletedAt == nil || !got.CompletedAt.Equal(now) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, now)
	}
}

func Test_Session_RecordSet_Timed_NoWeight(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: domain.Exercise{ //nolint:exhaustruct // Only ID and ExerciseType are read.
					ID: 1, ExerciseType: domain.ExerciseTypeTime,
				},
				Sets: []domain.Set{{TargetValue: 30}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}

	sig := domain.SignalOnTarget
	err := sess.RecordSet(0, 0, &sig, nil, 32, now)
	if err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	got := sess.Slots[0].Sets[0]
	if got.WeightKg != nil {
		t.Errorf("WeightKg = %v, want nil for timed set", got.WeightKg)
	}
	if got.CompletedValue == nil || *got.CompletedValue != 32 {
		t.Errorf("CompletedValue = %v, want 32", got.CompletedValue)
	}
	if got.Signal == nil || *got.Signal != domain.SignalOnTarget {
		t.Errorf("Signal = %v, want SignalOnTarget", got.Signal)
	}
}

func Test_Session_RecordSet_OutOfRange(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{} //nolint:exhaustruct // Empty session.
	sig := domain.SignalOnTarget
	err := sess.RecordSet(99, 0, &sig, nil, 5, now)
	if !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}

func Test_Session_RecordSet_OutOfBoundsIndex(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: domain.Exercise{ID: 1},         //nolint:exhaustruct // Only Exercise.ID is read.
				Sets:     []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	sig := domain.SignalOnTarget
	err := sess.RecordSet(0, 5, &sig, nil, 5, now)
	if !errors.Is(err, domain.ErrSetIndexOutOfBounds) {
		t.Fatalf("got %v, want ErrSetIndexOutOfBounds", err)
	}
}

func Test_Session_AddExercise_Append(t *testing.T) {
	t.Parallel()

	bench := domain.Exercise{ID: 1, Name: "Bench"} //nolint:exhaustruct // Only ID and Name read.
	squat := domain.Exercise{ID: 2, Name: "Squat"} //nolint:exhaustruct // Only ID and Name read.
	sess := domain.Session{                        //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{Exercise: bench, Sets: nil, WarmupCompletedAt: nil},
		},
	}

	err := sess.AddExercise(squat, []domain.Set{{TargetValue: 5}}) //nolint:exhaustruct // Other Set fields nil.
	if err != nil {
		t.Fatalf("AddExercise: %v", err)
	}
	if len(sess.Slots) != 2 {
		t.Fatalf("Slots length = %d, want 2", len(sess.Slots))
	}
	added := sess.Slots[1]
	if added.Exercise.ID != squat.ID {
		t.Errorf("Exercise.ID = %d, want %d", added.Exercise.ID, squat.ID)
	}
	if len(added.Sets) != 1 || added.Sets[0].TargetValue != 5 {
		t.Errorf("Sets = %+v, want one set with TargetValue 5", added.Sets)
	}
}

func Test_Session_AddExercise_DuplicateExerciseID_ReturnsErr(t *testing.T) {
	t.Parallel()

	bench := domain.Exercise{ID: 1, Name: "Bench"} //nolint:exhaustruct // Only ID read.
	sess := domain.Session{                        //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{Exercise: bench, Sets: nil, WarmupCompletedAt: nil},
		},
	}

	err := sess.AddExercise(bench, nil)
	if !errors.Is(err, domain.ErrExerciseAlreadyInSession) {
		t.Fatalf("got %v, want ErrExerciseAlreadyInSession", err)
	}
	if len(sess.Slots) != 1 {
		t.Errorf("Slots length = %d, want 1 (no append on error)", len(sess.Slots))
	}
}

func Test_Session_SwapExerciseInSlot_PreservesPosition(t *testing.T) {
	t.Parallel()

	bench := domain.Exercise{ID: 1, Name: "Bench"} //nolint:exhaustruct // Only ID read.
	squat := domain.Exercise{ID: 2, Name: "Squat"} //nolint:exhaustruct // Only ID read.
	dip := domain.Exercise{ID: 3, Name: "Dip"}     //nolint:exhaustruct // Only ID read.
	row := domain.Exercise{ID: 4, Name: "Row"}     //nolint:exhaustruct // Only ID read.
	warmupAt := time.Date(2026, 5, 10, 8, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test only sets Slots.
		Slots: []domain.ExerciseSlot{
			{
				Exercise:          bench,
				Sets:              []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other Set fields nil.
				WarmupCompletedAt: nil,
			},
			{
				Exercise:          squat,
				Sets:              []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other Set fields nil.
				WarmupCompletedAt: &warmupAt,
			},
			{
				Exercise:          row,
				Sets:              []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other Set fields nil.
				WarmupCompletedAt: nil,
			},
		},
	}

	newSets := []domain.Set{{TargetValue: 8}, {TargetValue: 8}} //nolint:exhaustruct // Other Set fields nil.
	if err := sess.SwapExerciseInSlot(1, dip, newSets); err != nil {
		t.Fatalf("SwapExerciseInSlot: %v", err)
	}
	got := sess.Slots[1]
	if got.Exercise.ID != dip.ID {
		t.Errorf("Exercise.ID = %d, want %d (swapped at pos 1)", got.Exercise.ID, dip.ID)
	}
	if len(got.Sets) != 2 {
		t.Errorf("Sets length = %d, want 2", len(got.Sets))
	}
	if got.WarmupCompletedAt != nil {
		t.Errorf("WarmupCompletedAt = %v, want nil (reset on swap)", got.WarmupCompletedAt)
	}
	if sess.Slots[0].Exercise.ID != bench.ID {
		t.Errorf("pos 0 Exercise.ID = %d, want %d (unchanged)", sess.Slots[0].Exercise.ID, bench.ID)
	}
	if sess.Slots[2].Exercise.ID != row.ID {
		t.Errorf("pos 2 Exercise.ID = %d, want %d (unchanged)", sess.Slots[2].Exercise.ID, row.ID)
	}
}

func Test_Session_SwapExerciseInSlot_OutOfRange(t *testing.T) {
	t.Parallel()

	sess := domain.Session{}                                        //nolint:exhaustruct // Empty session.
	err := sess.SwapExerciseInSlot(99, domain.Exercise{ID: 2}, nil) //nolint:exhaustruct // Only ID read.
	if !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}

func Test_Session_WorkoutType(t *testing.T) {
	t.Parallel()

	sessionWith := func(cats ...domain.Category) domain.Session {
		sets := make([]domain.ExerciseSlot, 0, len(cats))
		for _, c := range cats {
			sets = append(sets, domain.ExerciseSlot{ //nolint:exhaustruct // Test sets omit irrelevant fields.
				Exercise: domain.Exercise{ //nolint:exhaustruct // Only Category is read.
					Category: c,
				},
			})
		}
		return domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
			Slots: sets,
		}
	}
	tests := []struct {
		name string
		sess domain.Session
		want domain.Category
	}{
		{"empty defaults to full body", sessionWith(), domain.CategoryFullBody},
		{"only upper", sessionWith(domain.CategoryUpper, domain.CategoryUpper), domain.CategoryUpper},
		{"only lower", sessionWith(domain.CategoryLower), domain.CategoryLower},
		{
			"upper and lower is full body",
			sessionWith(domain.CategoryUpper, domain.CategoryLower),
			domain.CategoryFullBody,
		},
		{
			"any full body is full body",
			sessionWith(domain.CategoryUpper, domain.CategoryFullBody),
			domain.CategoryFullBody,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sess.WorkoutType(); got != tt.want {
				t.Errorf("WorkoutType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSession_RecordSet_NilSignalIsAllowed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.May, 4, 10, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // only fields used by RecordSet
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt not relevant to RecordSet
				Exercise: domain.Exercise{ //nolint:exhaustruct // only ID and ExerciseType matter here
					ID:           1,
					ExerciseType: domain.ExerciseTypeBodyweight,
				},
				Sets: []domain.Set{{TargetValue: 12}}, //nolint:exhaustruct // only TargetValue is needed
			},
		},
	}
	if err := sess.RecordSet(0, 0, nil, nil, 11, now); err != nil {
		t.Fatalf("RecordSet with nil signal: %v", err)
	}
	got := sess.Slots[0].Sets[0]
	if got.Signal != nil {
		t.Errorf("Signal = %v, want nil", got.Signal)
	}
	if got.CompletedValue == nil || *got.CompletedValue != 11 {
		t.Errorf("CompletedValue = %v, want 11", got.CompletedValue)
	}
}

func Test_Session_Status(t *testing.T) {
	t.Parallel()

	past := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		sess domain.Session
		want domain.SessionStatus
	}{
		{
			"not started",
			domain.Session{}, //nolint:exhaustruct // Test sessions omit irrelevant fields.
			domain.SessionNotStarted,
		},
		{
			"in progress",
			domain.Session{StartedAt: past}, //nolint:exhaustruct // Test sessions omit irrelevant fields.
			domain.SessionInProgress,
		},
		{
			"completed",
			domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
				StartedAt:   past,
				CompletedAt: later,
			},
			domain.SessionCompleted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.sess.Status(); got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_ExerciseSet_CompletionState(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	completed := domain.Set{CompletedAt: &now} //nolint:exhaustruct // Other Set fields nil.
	pending := domain.Set{}                    //nolint:exhaustruct // All fields nil — represents an unfinished set.
	tests := []struct {
		name string
		sets []domain.Set
		want domain.ExerciseSlotState
	}{
		{"no sets is not started", nil, domain.ExerciseSlotNotStarted},
		{"all pending is not started", []domain.Set{pending, pending}, domain.ExerciseSlotNotStarted},
		{"some completed is started", []domain.Set{completed, pending}, domain.ExerciseSlotStarted},
		{"all completed is completed", []domain.Set{completed, completed}, domain.ExerciseSlotCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			es := domain.ExerciseSlot{Sets: tt.sets} //nolint:exhaustruct // Only Sets is relevant here.
			if got := es.CompletionState(); got != tt.want {
				t.Errorf("CompletionState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSessionHasIncompleteSets(t *testing.T) {
	t.Parallel()

	completedAt := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	completedVal := 5
	completedSet := domain.Set{
		WeightKg:       nil,
		TargetValue:    5,
		CompletedValue: &completedVal,
		CompletedAt:    &completedAt,
		Signal:         nil,
	}
	incompleteSet := domain.Set{TargetValue: 5} //nolint:exhaustruct // Other fields nil — represents an unfinished set.

	tests := []struct {
		name string
		sess domain.Session
		want bool
	}{
		{
			name: "empty session — no sets, no incomplete",
			sess: domain.Session{}, //nolint:exhaustruct // Empty session.
			want: false,
		},
		{
			name: "all sets complete",
			sess: domain.Session{ //nolint:exhaustruct // Test only sets Slots.
				Slots: []domain.ExerciseSlot{
					{ //nolint:exhaustruct // Only Sets is read.
						Sets: []domain.Set{completedSet, completedSet},
					},
				},
			},
			want: false,
		},
		{
			name: "one set incomplete in a later slot",
			sess: domain.Session{ //nolint:exhaustruct // Test only sets Slots.
				Slots: []domain.ExerciseSlot{
					{ //nolint:exhaustruct // Only Sets is read.
						Sets: []domain.Set{completedSet, completedSet},
					},
					{ //nolint:exhaustruct // Only Sets is read.
						Sets: []domain.Set{completedSet, incompleteSet},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.sess.HasIncompleteSets()
			if got != tt.want {
				t.Errorf("HasIncompleteSets() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_Session_SwitchToDeload_SetsFlag(t *testing.T) {
	t.Parallel()

	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date: time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
	}

	if err := sess.SwitchToDeload(4); err != nil {
		t.Fatalf("SwitchToDeload: %v", err)
	}
	if !sess.IsDeload {
		t.Error("SwitchToDeload did not set IsDeload to true")
	}
}

func Test_Session_SwitchToDeload_Idempotent(t *testing.T) {
	t.Parallel()

	repMin, repMax := 3, 5
	ex := domain.Exercise{ //nolint:exhaustruct // Only planning fields read.
		ID:           1,
		Name:         "Squat",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin,
		RepMax:       &repMax,
	}
	sess := domain.Session{ //nolint:exhaustruct // Only deload-relevant fields set.
		Goal: domain.SessionGoalStrength,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: ex,
				Sets:     domain.BuildPlannedSets(ex, domain.SessionGoalStrength, false, 4),
			},
		},
	}

	if err := sess.SwitchToDeload(4); err != nil {
		t.Fatalf("SwitchToDeload first call: %v", err)
	}
	firstLen := len(sess.Slots[0].Sets)
	firstTarget := sess.Slots[0].Sets[0].TargetValue

	if err := sess.SwitchToDeload(4); err != nil {
		t.Fatalf("SwitchToDeload second call: %v", err)
	}
	if !sess.IsDeload {
		t.Error("SwitchToDeload cleared IsDeload on already-deload session")
	}
	if got := len(sess.Slots[0].Sets); got != firstLen {
		t.Errorf("second call changed len(Sets): %d -> %d", firstLen, got)
	}
	if got := sess.Slots[0].Sets[0].TargetValue; got != firstTarget {
		t.Errorf("second call changed TargetValue: %d -> %d", firstTarget, got)
	}
}

func Test_Session_ClearDeload_ClearsFlag(t *testing.T) {
	t.Parallel()

	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date:     time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
		IsDeload: true,
	}

	if err := sess.ClearDeload(4); err != nil {
		t.Fatalf("ClearDeload: %v", err)
	}
	if sess.IsDeload {
		t.Error("ClearDeload did not set IsDeload to false")
	}
}

func Test_Session_ClearDeload_Idempotent(t *testing.T) {
	t.Parallel()

	repMin, repMax := 3, 5
	ex := domain.Exercise{ //nolint:exhaustruct // Only planning fields read.
		ID:           1,
		Name:         "Squat",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin,
		RepMax:       &repMax,
	}
	sess := domain.Session{ //nolint:exhaustruct // Only deload-relevant fields set.
		Goal: domain.SessionGoalStrength,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: ex,
				Sets:     domain.BuildPlannedSets(ex, domain.SessionGoalStrength, false, 4),
			},
		},
	}

	if err := sess.ClearDeload(4); err != nil {
		t.Fatalf("ClearDeload first call: %v", err)
	}
	firstLen := len(sess.Slots[0].Sets)

	if err := sess.ClearDeload(4); err != nil {
		t.Fatalf("ClearDeload second call: %v", err)
	}
	if sess.IsDeload {
		t.Error("ClearDeload toggled IsDeload to true on already-clear session")
	}
	if got := len(sess.Slots[0].Sets); got != firstLen {
		t.Errorf("second call changed len(Sets): %d -> %d", firstLen, got)
	}
}

func Test_Session_SwitchToDeload_RebuildsUncompletedSets(t *testing.T) {
	t.Parallel()

	repMin, repMax := 3, 5
	ex := domain.Exercise{ //nolint:exhaustruct // Only planning fields read.
		ID:           1,
		Name:         "Squat",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin,
		RepMax:       &repMax,
	}
	// Build a Strength session as the planner would: 4 untouched sets, TargetValue=3 (repMin).
	sess := domain.Session{ //nolint:exhaustruct // Only deload-relevant fields set.
		Goal: domain.SessionGoalStrength,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: ex,
				Sets:     domain.BuildPlannedSets(ex, domain.SessionGoalStrength, false, 4),
			},
		},
	}
	if got := len(sess.Slots[0].Sets); got != 4 {
		t.Fatalf("precondition: want 4 sets, got %d", got)
	}

	if err := sess.SwitchToDeload(4); err != nil {
		t.Fatalf("SwitchToDeload: %v", err)
	}

	if !sess.IsDeload {
		t.Fatal("SwitchToDeload did not set IsDeload to true")
	}
	got := sess.Slots[0].Sets
	if len(got) != 3 {
		t.Fatalf("len(Sets) = %d, want 3 (deload drops one set)", len(got))
	}
	for i, s := range got {
		if s.TargetValue != 5 { // repMax (deload forces hypertrophy)
			t.Errorf("Sets[%d].TargetValue = %d, want 5 (repMax)", i, s.TargetValue)
		}
		if s.CompletedAt != nil {
			t.Errorf("Sets[%d].CompletedAt = %v, want nil", i, s.CompletedAt)
		}
		if s.WeightKg != nil {
			t.Errorf("Sets[%d].WeightKg = %v, want nil (planner shape)", i, s.WeightKg)
		}
	}
}

func Test_Session_SwitchToDeload_PreservesCompletedSets(t *testing.T) {
	t.Parallel()

	repMin, repMax := 3, 5
	ex := domain.Exercise{ //nolint:exhaustruct // Only planning fields read.
		ID:           1,
		Name:         "Squat",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin,
		RepMax:       &repMax,
	}
	completedAt := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)
	completedValue := 5
	signal := domain.SignalOnTarget
	weight := 100.0

	sess := domain.Session{ //nolint:exhaustruct // Only deload-relevant fields set.
		Goal: domain.SessionGoalStrength,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: ex,
				Sets: []domain.Set{
					// Two completed sets.
					{
						TargetValue:    3,
						WeightKg:       &weight,
						CompletedValue: &completedValue,
						CompletedAt:    &completedAt,
						Signal:         &signal,
					},
					{
						TargetValue:    3,
						WeightKg:       &weight,
						CompletedValue: &completedValue,
						CompletedAt:    &completedAt,
						Signal:         &signal,
					},
					// Two untouched sets.
					{TargetValue: 3}, //nolint:exhaustruct // Untouched set: only TargetValue set.
					{TargetValue: 3}, //nolint:exhaustruct // Untouched set: only TargetValue set.
				},
			},
		},
	}

	if err := sess.SwitchToDeload(4); err != nil {
		t.Fatalf("SwitchToDeload: %v", err)
	}

	got := sess.Slots[0].Sets
	// Strength 4 → deload 3 sets; 2 completed preserved + 1 fresh uncompleted = 3.
	if len(got) != 3 {
		t.Fatalf("len(Sets) = %d, want 3", len(got))
	}
	// First two slots: original completed sets, untouched.
	for i := range 2 {
		if got[i].CompletedAt == nil || !got[i].CompletedAt.Equal(completedAt) {
			t.Errorf(
				"Sets[%d].CompletedAt = %v, want %v (completed sets preserved)",
				i,
				got[i].CompletedAt,
				completedAt,
			)
		}
		if got[i].TargetValue != 3 {
			t.Errorf("Sets[%d].TargetValue = %d, want 3 (completed set keeps original target)", i, got[i].TargetValue)
		}
		if got[i].WeightKg == nil || *got[i].WeightKg != 100.0 {
			t.Errorf("Sets[%d].WeightKg = %v, want 100.0", i, got[i].WeightKg)
		}
	}
	// Third slot: fresh planner-shape, TargetValue=repMax, all other fields nil.
	if got[2].CompletedAt != nil {
		t.Errorf("Sets[2].CompletedAt = %v, want nil (fresh set)", got[2].CompletedAt)
	}
	if got[2].TargetValue != 5 {
		t.Errorf("Sets[2].TargetValue = %d, want 5 (repMax for deload)", got[2].TargetValue)
	}
	if got[2].WeightKg != nil {
		t.Errorf("Sets[2].WeightKg = %v, want nil (planner shape)", got[2].WeightKg)
	}
}

func Test_Session_SwitchToDeload_OverQuotaCompletedSetsKept(t *testing.T) {
	t.Parallel()

	repMin, repMax := 3, 5
	ex := domain.Exercise{ //nolint:exhaustruct // Only planning fields read.
		ID:           1,
		Name:         "Squat",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin,
		RepMax:       &repMax,
	}
	completedAt := time.Date(2026, 5, 31, 9, 0, 0, 0, time.UTC)
	completedValue := 5
	signal := domain.SignalOnTarget

	sess := domain.Session{ //nolint:exhaustruct // Only deload-relevant fields set.
		Goal: domain.SessionGoalStrength,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: ex,
				// 4 completed sets — all of a non-deload Strength prescription.
				Sets: []domain.Set{
					{ //nolint:exhaustruct // WeightKg nil.
						TargetValue:    3,
						CompletedValue: &completedValue,
						CompletedAt:    &completedAt,
						Signal:         &signal,
					},
					{ //nolint:exhaustruct // WeightKg nil.
						TargetValue:    3,
						CompletedValue: &completedValue,
						CompletedAt:    &completedAt,
						Signal:         &signal,
					},
					{ //nolint:exhaustruct // WeightKg nil.
						TargetValue:    3,
						CompletedValue: &completedValue,
						CompletedAt:    &completedAt,
						Signal:         &signal,
					},
					{ //nolint:exhaustruct // WeightKg nil.
						TargetValue:    3,
						CompletedValue: &completedValue,
						CompletedAt:    &completedAt,
						Signal:         &signal,
					},
				},
			},
		},
	}

	if err := sess.SwitchToDeload(4); err != nil {
		t.Fatalf("SwitchToDeload: %v", err)
	}

	got := sess.Slots[0].Sets
	// Deload prescription would be 3, but 4 are already completed — keep all 4.
	if len(got) != 4 {
		t.Errorf("len(Sets) = %d, want 4 (no shrink below len(completed))", len(got))
	}
	for i, s := range got {
		if s.CompletedAt == nil {
			t.Errorf("Sets[%d].CompletedAt = nil, want preserved", i)
		}
	}
}

func Test_Session_ClearDeload_ExpandsUncompletedSets(t *testing.T) {
	t.Parallel()

	repMin, repMax := 3, 5
	ex := domain.Exercise{ //nolint:exhaustruct // Only planning fields read.
		ID:           1,
		Name:         "Squat",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin,
		RepMax:       &repMax,
	}
	// Start in deload: 3 untouched sets, TargetValue=repMax=5.
	sess := domain.Session{ //nolint:exhaustruct // Only deload-relevant fields set.
		Goal:     domain.SessionGoalStrength,
		IsDeload: true,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: ex,
				Sets:     domain.BuildPlannedSets(ex, domain.SessionGoalStrength, true, 4),
			},
		},
	}
	if got := len(sess.Slots[0].Sets); got != 3 {
		t.Fatalf("precondition: want 3 deload sets, got %d", got)
	}

	if err := sess.ClearDeload(4); err != nil {
		t.Fatalf("ClearDeload: %v", err)
	}

	if sess.IsDeload {
		t.Fatal("ClearDeload did not unset IsDeload")
	}
	got := sess.Slots[0].Sets
	// Non-deload, weekSets=4: 4 sets, TargetValue=repMin=3.
	if len(got) != 4 {
		t.Fatalf("len(Sets) = %d, want 4 (ClearDeload restores week-driven count)", len(got))
	}
	for i, s := range got {
		if s.TargetValue != 3 {
			t.Errorf("Sets[%d].TargetValue = %d, want 3 (repMin for Strength)", i, s.TargetValue)
		}
		if s.CompletedAt != nil {
			t.Errorf("Sets[%d].CompletedAt = %v, want nil", i, s.CompletedAt)
		}
	}
}

func Test_Session_SwitchToDeload_TimeBasedExercise(t *testing.T) {
	t.Parallel()

	startingSeconds := 30
	ex := domain.Exercise{ //nolint:exhaustruct // Only planning fields read.
		ID:                     2,
		Name:                   "Plank",
		ExerciseType:           domain.ExerciseTypeTime,
		DefaultStartingSeconds: &startingSeconds,
	}
	sess := domain.Session{ //nolint:exhaustruct // Only deload-relevant fields set.
		Goal: domain.SessionGoalStrength,
		Slots: []domain.ExerciseSlot{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				Exercise: ex,
				Sets:     domain.BuildPlannedSets(ex, domain.SessionGoalStrength, false, 4),
			},
		},
	}
	if got := len(sess.Slots[0].Sets); got != 3 {
		t.Fatalf("precondition: want 3 time-based sets, got %d", got)
	}

	if err := sess.SwitchToDeload(4); err != nil {
		t.Fatalf("SwitchToDeload: %v", err)
	}

	got := sess.Slots[0].Sets
	if len(got) != 2 {
		t.Fatalf("len(Sets) = %d, want 2 (3-1 floored at 2)", len(got))
	}
	for i, s := range got {
		if s.TargetValue != startingSeconds {
			t.Errorf("Sets[%d].TargetValue = %d, want %d (DefaultStartingSeconds)", i, s.TargetValue, startingSeconds)
		}
	}
}

func Test_ExerciseSlot_RestEndAt(t *testing.T) {
	t.Parallel()

	repMin5, repMax5 := 5, 5     // strength → 180s rest.
	repMin12, repMax15 := 12, 15 // hypertrophy → 90s rest.
	startSecs := 30              // for the timed-exercise case.

	squat := domain.Exercise{ //nolint:exhaustruct // Only rest-relevant fields set.
		Name: "Squat", ExerciseType: domain.ExerciseTypeWeighted,
		RepMin: &repMin5, RepMax: &repMax5,
	}
	curl := domain.Exercise{ //nolint:exhaustruct // Only rest-relevant fields set.
		Name: "Bicep Curl", ExerciseType: domain.ExerciseTypeWeighted,
		RepMin: &repMin12, RepMax: &repMax15,
	}
	plank := domain.Exercise{ //nolint:exhaustruct // Only rest-relevant fields set.
		Name: "Plank", ExerciseType: domain.ExerciseTypeTime,
		DefaultStartingSeconds: &startSecs,
	}

	warmupAt := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	setDoneAt := warmupAt.Add(2 * time.Minute)
	v := 5
	w := 100.0
	completedSet := domain.Set{ //nolint:exhaustruct // Only completion fields set.
		WeightKg: &w, TargetValue: 5, CompletedValue: &v, CompletedAt: &setDoneAt,
	}
	incompleteSet := domain.Set{ //nolint:exhaustruct // Only target fields set.
		WeightKg: &w, TargetValue: 5,
	}

	tests := []struct {
		name      string
		slot      domain.ExerciseSlot
		pt        domain.SessionGoal
		isDeload  bool
		wantOK    bool
		wantEndAt time.Time
	}{
		{ //nolint:exhaustruct // wantOK/wantEndAt default to zero; isDeload defaults to false.
			name: "warmup not done returns false",
			slot: domain.ExerciseSlot{ //nolint:exhaustruct // WarmupCompletedAt intentionally nil.
				Exercise: squat, Sets: []domain.Set{incompleteSet, incompleteSet},
			},
			pt: domain.SessionGoalStrength,
		},
		{ //nolint:exhaustruct // wantOK/wantEndAt default to zero; isDeload defaults to false.
			name: "all sets complete returns false",
			slot: domain.ExerciseSlot{
				Exercise: squat, WarmupCompletedAt: &warmupAt,
				Sets: []domain.Set{completedSet, completedSet},
			},
			pt: domain.SessionGoalStrength,
		},
		{ //nolint:exhaustruct // wantOK/wantEndAt default to zero; isDeload defaults to false.
			name: "no sets planned returns false",
			slot: domain.ExerciseSlot{
				Exercise: squat, WarmupCompletedAt: &warmupAt, Sets: []domain.Set{},
			},
			pt: domain.SessionGoalStrength,
		},
		{ //nolint:exhaustruct // wantOK/wantEndAt default to zero; isDeload defaults to false.
			name: "timed exercise returns false (no rest defined)",
			slot: domain.ExerciseSlot{
				Exercise: plank, WarmupCompletedAt: &warmupAt,
				Sets: []domain.Set{incompleteSet, incompleteSet},
			},
			pt: domain.SessionGoalHypertrophy,
		},
		{ //nolint:exhaustruct // isDeload defaults to false; only deload-true case overrides.
			name: "warmup just done, no sets started: clock starts at warmup",
			slot: domain.ExerciseSlot{
				Exercise: squat, WarmupCompletedAt: &warmupAt,
				Sets: []domain.Set{incompleteSet, incompleteSet, incompleteSet},
			},
			pt:        domain.SessionGoalStrength,
			wantOK:    true,
			wantEndAt: warmupAt.Add(180 * time.Second),
		},
		{ //nolint:exhaustruct // isDeload defaults to false; only deload-true case overrides.
			name: "first set done: clock starts at set completion (later of warmup/set)",
			slot: domain.ExerciseSlot{
				Exercise: squat, WarmupCompletedAt: &warmupAt,
				Sets: []domain.Set{completedSet, incompleteSet, incompleteSet},
			},
			pt:        domain.SessionGoalStrength,
			wantOK:    true,
			wantEndAt: setDoneAt.Add(180 * time.Second),
		},
		{ //nolint:exhaustruct // isDeload defaults to false; only deload-true case overrides.
			name: "hypertrophy curls (15 reps): 90s rest",
			slot: domain.ExerciseSlot{
				Exercise: curl, WarmupCompletedAt: &warmupAt,
				Sets: []domain.Set{completedSet, incompleteSet},
			},
			pt:        domain.SessionGoalHypertrophy,
			wantOK:    true,
			wantEndAt: setDoneAt.Add(90 * time.Second),
		},
		{
			name: "deload forces hypertrophy mapping for the rest band",
			slot: domain.ExerciseSlot{
				Exercise: squat, WarmupCompletedAt: &warmupAt,
				Sets: []domain.Set{incompleteSet, incompleteSet},
			},
			pt:        domain.SessionGoalStrength,
			isDeload:  true,
			wantOK:    true,
			wantEndAt: warmupAt.Add(180 * time.Second),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotEndAt, gotOK := tt.slot.RestEndAt(tt.pt, tt.isDeload)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if !gotEndAt.Equal(tt.wantEndAt) {
				t.Errorf("endAt = %v, want %v", gotEndAt, tt.wantEndAt)
			}
		})
	}
}
