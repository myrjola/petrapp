package notification_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/notification"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// fakeDispatch records every Dispatch call and closes done after target fires.
type fakeDispatch struct {
	mu     sync.Mutex
	fires  []int
	done   chan struct{}
	target int
}

func newFakeDispatch(target int) *fakeDispatch {
	//nolint:exhaustruct // mu/fires zero values intentional.
	return &fakeDispatch{done: make(chan struct{}), target: target}
}

func (f *fakeDispatch) Dispatch(_ context.Context, push domain.ScheduledPush) error {
	f.mu.Lock()
	f.fires = append(f.fires, push.WorkoutExerciseID)
	if f.target > 0 && len(f.fires) == f.target {
		close(f.done)
	}
	f.mu.Unlock()
	return nil
}

func (f *fakeDispatch) fired() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := append([]int(nil), f.fires...)
	return out
}

func TestScheduler_ScheduleFires(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(1)
	repo := newInMemoryScheduledPushRepo()
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     repo,
		Dispatch: fd.Dispatch,
		Logger:   testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now:      time.Now,
	})

	push := domain.ScheduledPush{ //nolint:exhaustruct // ID/CreatedAt assigned by the repo.
		UserID:            1,
		WorkoutExerciseID: 42,
		FireAt:            time.Now().Add(50 * time.Millisecond),
		Payload:           `{}`,
	}
	if err := scheduler.Schedule(context.Background(), push); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if scheduler.PendingCount() != 1 {
		t.Fatalf("PendingCount = %d, want 1", scheduler.PendingCount())
	}

	select {
	case <-fd.done:
	case <-time.After(time.Second):
		t.Fatalf("timer never fired")
	}
	if got := fd.fired(); len(got) != 1 || got[0] != 42 {
		t.Errorf("fires = %v, want [42]", got)
	}
	// Allow scheduler's post-fire cleanup goroutine to run.
	time.Sleep(50 * time.Millisecond)
	if scheduler.PendingCount() != 0 {
		t.Errorf("after fire: PendingCount = %d, want 0", scheduler.PendingCount())
	}
}

func TestScheduler_CancelStopsTimer(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(0)
	repo := newInMemoryScheduledPushRepo()
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     repo,
		Dispatch: fd.Dispatch,
		Logger:   testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now:      time.Now,
	})

	push := domain.ScheduledPush{ //nolint:exhaustruct // ID/CreatedAt/Payload unused here.
		UserID: 1, WorkoutExerciseID: 99,
		FireAt: time.Now().Add(100 * time.Millisecond),
	}
	if err := scheduler.Schedule(context.Background(), push); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if err := scheduler.Cancel(context.Background(), 99); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if got := fd.fired(); len(got) != 0 {
		t.Errorf("got fires after Cancel: %v", got)
	}
	if scheduler.PendingCount() != 0 {
		t.Errorf("PendingCount = %d, want 0", scheduler.PendingCount())
	}
}

func TestScheduler_ReplaceOnlyFiresLatest(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(1)
	repo := newInMemoryScheduledPushRepo()
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     repo,
		Dispatch: fd.Dispatch,
		Logger:   testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now:      time.Now,
	})

	// Schedule far-future, then immediately reschedule near-future. Far-future is tight enough
	// that we'd catch a missed Stop() within the test window — see post-fire sleep below.
	farFuture := domain.ScheduledPush{ //nolint:exhaustruct // ID/CreatedAt assigned by the repo.
		UserID: 1, WorkoutExerciseID: 7,
		FireAt:  time.Now().Add(300 * time.Millisecond),
		Payload: `"original"`,
	}
	nearFuture := domain.ScheduledPush{ //nolint:exhaustruct // ID/CreatedAt assigned by the repo.
		UserID: 1, WorkoutExerciseID: 7,
		FireAt:  time.Now().Add(50 * time.Millisecond),
		Payload: `"replacement"`,
	}
	if err := scheduler.Schedule(context.Background(), farFuture); err != nil {
		t.Fatalf("first Schedule: %v", err)
	}
	if err := scheduler.Schedule(context.Background(), nearFuture); err != nil {
		t.Fatalf("second Schedule: %v", err)
	}

	select {
	case <-fd.done:
	case <-time.After(time.Second):
		t.Fatalf("replacement never fired within window")
	}
	// Wait long enough for far-future to have fired if it wasn't stopped.
	time.Sleep(400 * time.Millisecond)
	if got := fd.fired(); len(got) != 1 {
		t.Errorf("got %d fires, want exactly 1 (replacement only): %v", len(got), got)
	}
}

func TestScheduler_ReloadReconstitutesFutureTimers(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(1)
	repo := newInMemoryScheduledPushRepo()
	// Pre-seed the repo with a future fire.
	//nolint:exhaustruct // ID/CreatedAt assigned by the repo.
	seed := domain.ScheduledPush{
		UserID: 1, WorkoutExerciseID: 11,
		FireAt:  time.Now().Add(50 * time.Millisecond),
		Payload: "{}",
	}
	if _, err := repo.Replace(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     repo,
		Dispatch: fd.Dispatch,
		Logger:   testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now:      time.Now,
	})

	if err := scheduler.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if scheduler.PendingCount() != 1 {
		t.Fatalf("PendingCount after Reload = %d, want 1", scheduler.PendingCount())
	}
	select {
	case <-fd.done:
	case <-time.After(time.Second):
		t.Fatalf("reloaded timer never fired")
	}
}

func TestScheduler_ReloadFiresPastDueImmediately(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(1)
	repo := newInMemoryScheduledPushRepo()
	//nolint:exhaustruct // ID/CreatedAt assigned by the repo.
	seed := domain.ScheduledPush{
		UserID: 1, WorkoutExerciseID: 22,
		FireAt:  time.Now().Add(-time.Minute),
		Payload: "{}",
	}
	if _, err := repo.Replace(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var fakeNow atomic.Value
	fakeNow.Store(time.Now())
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     repo,
		Dispatch: fd.Dispatch,
		Logger:   testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now: func() time.Time {
			v, _ := fakeNow.Load().(time.Time)
			return v
		},
	})

	if err := scheduler.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	select {
	case <-fd.done:
	case <-time.After(time.Second):
		t.Fatalf("past-due fire never dispatched on Reload")
	}
}

// newInMemoryScheduledPushRepo returns an in-memory implementation of the
// scheduler's ScheduledPushRepo interface for tests.
func newInMemoryScheduledPushRepo() *inMemScheduledPushRepo {
	//nolint:exhaustruct // mu/nextID zero values intentional.
	return &inMemScheduledPushRepo{rows: map[int]domain.ScheduledPush{}}
}

type inMemScheduledPushRepo struct {
	mu     sync.Mutex
	rows   map[int]domain.ScheduledPush
	nextID int
}

func (m *inMemScheduledPushRepo) Replace(_ context.Context, push domain.ScheduledPush) (domain.ScheduledPush, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	push.ID = m.nextID
	push.CreatedAt = time.Now()
	m.rows[push.WorkoutExerciseID] = push
	return push, nil
}

func (m *inMemScheduledPushRepo) Delete(_ context.Context, id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.rows {
		if v.ID == id {
			delete(m.rows, k)
			return nil
		}
	}
	return nil
}

func (m *inMemScheduledPushRepo) DeleteByWorkoutExercise(_ context.Context, weID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, weID)
	return nil
}

func (m *inMemScheduledPushRepo) DeleteByWorkoutSession(_ context.Context, _ int, _ time.Time) error {
	return errors.New("not implemented in test")
}

func (m *inMemScheduledPushRepo) ListAll(_ context.Context) ([]domain.ScheduledPush, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.ScheduledPush, 0, len(m.rows))
	for _, v := range m.rows {
		out = append(out, v)
	}
	return out, nil
}

func TestScheduler_DispatchFailure_LogsUserID(t *testing.T) {
	t.Parallel()

	var logBuf bytes.Buffer
	//nolint:exhaustruct // AddSource/ReplaceAttr zero.
	handlerOpts := &slog.HandlerOptions{Level: slog.LevelDebug}
	logger := slog.New(slog.NewTextHandler(&logBuf, handlerOpts))
	repo := newInMemoryScheduledPushRepo()
	dispatchErr := errors.New("push gateway down")
	dispatched := make(chan struct{})
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo: repo,
		Dispatch: func(_ context.Context, _ domain.ScheduledPush) error {
			close(dispatched)
			return dispatchErr
		},
		Logger: logger,
		Now:    time.Now,
	})

	push := domain.ScheduledPush{ //nolint:exhaustruct // ID/CreatedAt assigned by the repo.
		UserID:            7,
		WorkoutExerciseID: 314,
		FireAt:            time.Now().Add(30 * time.Millisecond),
		Payload:           `{}`,
	}
	if err := scheduler.Schedule(context.Background(), push); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	select {
	case <-dispatched:
	case <-time.After(time.Second):
		t.Fatalf("dispatch never fired")
	}
	// Give the scheduler a moment to log after Dispatch returns.
	time.Sleep(50 * time.Millisecond)

	out := logBuf.String()
	if !strings.Contains(out, "user_id=7") {
		t.Errorf("log missing user_id=7; got:\n%s", out)
	}
	if !strings.Contains(out, "workout_exercise_id=314") {
		t.Errorf("log missing workout_exercise_id=314; got:\n%s", out)
	}
	if !strings.Contains(out, "push dispatch failed") {
		t.Errorf("log missing dispatch failure message; got:\n%s", out)
	}
}
