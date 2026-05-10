package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func Test_Session_Start_FromZero(t *testing.T) {
	now := time.Date(2026, 5, 10, 9, 0, 0, 0, time.UTC)
	sess := domain.Session{ //nolint:exhaustruct
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
	sess := domain.Session{ //nolint:exhaustruct
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
	sess := domain.Session{ //nolint:exhaustruct
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
	sess := domain.Session{ //nolint:exhaustruct
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
		sess := domain.Session{} //nolint:exhaustruct
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
		sess := domain.Session{} //nolint:exhaustruct
		err := sess.SetDifficulty(rating)
		if !errors.Is(err, domain.ErrInvalidDifficultyRating) {
			t.Errorf("SetDifficulty(%d): got %v, want ErrInvalidDifficultyRating", rating, err)
		}
		if sess.DifficultyRating != nil {
			t.Errorf("DifficultyRating mutated to %v, want nil", sess.DifficultyRating)
		}
	}
}
