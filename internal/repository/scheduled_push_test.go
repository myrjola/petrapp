package repository_test

import (
	"errors"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/platform/contexthelpers"
)

func TestScheduledPushes_ReplaceUpsertsBySlot(t *testing.T) {
	t.Parallel()
	ctx, db, repos := setupTestReposWithDB(t)

	userID := contexthelpers.AuthenticatedUserID(ctx)
	date, pos := seedWorkoutExerciseSlot(ctx, t, db)

	fireAt1 := time.Now().Add(90 * time.Second).UTC().Truncate(time.Millisecond)
	first := domain.ScheduledPush{ //nolint:exhaustruct // ID/CreatedAt populated by Replace.
		UserID:      userID,
		WorkoutDate: date,
		Position:    pos,
		FireAt:      fireAt1,
		Payload:     `{"title":"Rest over","body":"Set 1"}`,
	}
	got, err := repos.ScheduledPushes.Replace(ctx, first)
	if err != nil {
		t.Fatalf("first Replace: %v", err)
	}
	if got.ID == 0 {
		t.Error("Replace returned ID 0")
	}

	// Replace with new time + payload — should overwrite, not duplicate.
	fireAt2 := time.Now().Add(150 * time.Second).UTC().Truncate(time.Millisecond)
	second := first
	second.FireAt = fireAt2
	second.Payload = `{"title":"Rest over","body":"Set 2"}`
	if _, err = repos.ScheduledPushes.Replace(ctx, second); err != nil {
		t.Fatalf("second Replace: %v", err)
	}

	all, err := repos.ScheduledPushes.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListAll returned %d rows, want 1", len(all))
	}
	if !all[0].FireAt.Equal(fireAt2) {
		t.Errorf("FireAt = %v, want %v", all[0].FireAt, fireAt2)
	}
	if all[0].Payload != second.Payload {
		t.Errorf("Payload = %q, want %q", all[0].Payload, second.Payload)
	}
}

func TestScheduledPushes_GetBySlotReturnsErrNotFoundWhenMissing(t *testing.T) {
	t.Parallel()
	ctx, _, repos := setupTestReposWithDB(t)
	userID := contexthelpers.AuthenticatedUserID(ctx)
	missingDate := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

	_, err := repos.ScheduledPushes.GetBySlot(ctx, userID, missingDate, 0)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetBySlot(missing) error = %v, want ErrNotFound", err)
	}
}
