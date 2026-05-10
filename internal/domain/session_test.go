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
