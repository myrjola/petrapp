package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func Test_Session_Start_FromZero(t *testing.T) {
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
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // Test sets omit irrelevant fields.
				ID:       11,
				Exercise: domain.Exercise{ID: 1}, //nolint:exhaustruct // Test exercises omit display fields.
			},
			{ //nolint:exhaustruct // Test sets omit irrelevant fields.
				ID:       12,
				Exercise: domain.Exercise{ID: 2}, //nolint:exhaustruct // Test exercises omit display fields.
			},
		},
	}

	if err := sess.MarkWarmupComplete(12, now); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}
	if sess.ExerciseSets[1].WarmupCompletedAt == nil || !sess.ExerciseSets[1].WarmupCompletedAt.Equal(now) {
		t.Errorf("slot 12 WarmupCompletedAt = %v, want %v", sess.ExerciseSets[1].WarmupCompletedAt, now)
	}
	if sess.ExerciseSets[0].WarmupCompletedAt != nil {
		t.Errorf("slot 11 WarmupCompletedAt mutated to %v, want nil", sess.ExerciseSets[0].WarmupCompletedAt)
	}
}

func Test_Session_MarkWarmupComplete_UnknownSlot(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // Test sets omit irrelevant fields.
				ID:       11,
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
	sess := domain.Session{ //nolint:exhaustruct // Test only sets ExerciseSets.
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				ID: 11, Exercise: domain.Exercise{ID: 1}, //nolint:exhaustruct // Only ID is read.
				Sets: []domain.Set{
					{TargetValue: 5}, //nolint:exhaustruct // Other fields nil.
					{TargetValue: 5}, //nolint:exhaustruct // Other fields nil.
				},
			},
		},
	}

	if err := sess.UpdateSetWeight(11, 1, 80.0); err != nil {
		t.Fatalf("UpdateSetWeight: %v", err)
	}
	if sess.ExerciseSets[0].Sets[1].WeightKg == nil || *sess.ExerciseSets[0].Sets[1].WeightKg != 80.0 {
		t.Errorf("WeightKg = %v, want 80.0", sess.ExerciseSets[0].Sets[1].WeightKg)
	}
	if sess.ExerciseSets[0].Sets[0].WeightKg != nil {
		t.Errorf("set 0 WeightKg mutated to %v, want nil", sess.ExerciseSets[0].Sets[0].WeightKg)
	}
}

func Test_Session_UpdateSetWeight_UnknownSlot(t *testing.T) {
	sess := domain.Session{} //nolint:exhaustruct // Empty session.
	err := sess.UpdateSetWeight(99, 0, 80.0)
	if !errors.Is(err, domain.ErrSlotNotFound) {
		t.Fatalf("got %v, want ErrSlotNotFound", err)
	}
}

func Test_Session_UpdateSetWeight_OutOfBoundsIndex(t *testing.T) {
	sess := domain.Session{ //nolint:exhaustruct // Test only sets ExerciseSets.
		ExerciseSets: []domain.ExerciseSet{
			{ //nolint:exhaustruct // WarmupCompletedAt nil.
				ID: 11, Exercise: domain.Exercise{ID: 1}, //nolint:exhaustruct // Only ID is read.
				Sets: []domain.Set{{TargetValue: 5}}, //nolint:exhaustruct // Other fields nil.
			},
		},
	}
	err := sess.UpdateSetWeight(11, 5, 80.0)
	if !errors.Is(err, domain.ErrSetIndexOutOfBounds) {
		t.Fatalf("got %v, want ErrSetIndexOutOfBounds", err)
	}
}
