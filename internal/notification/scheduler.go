package notification

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

// DispatchFunc is called when a scheduled push fires. Implementations should
// load subscriptions, send the payload, and prune 410'd subscriptions. The
// scheduler invokes Dispatch from a fresh goroutine; implementations are
// responsible for their own context propagation.
type DispatchFunc func(ctx context.Context, push domain.ScheduledPush) error

// ScheduledPushRepo is the subset of repository.ScheduledPushRepository the
// scheduler needs. Declared locally so the package doesn't import the whole
// repository package — keeps notification at a lower layer in the dep graph.
type ScheduledPushRepo interface {
	Replace(ctx context.Context, push domain.ScheduledPush) (domain.ScheduledPush, error)
	Delete(ctx context.Context, id int) error
	DeleteByWorkoutExercise(ctx context.Context, workoutExerciseID int) error
	DeleteByWorkoutSession(ctx context.Context, userID int, date time.Time) error
	ListAll(ctx context.Context) ([]domain.ScheduledPush, error)
}

// SchedulerConfig configures a Scheduler.
type SchedulerConfig struct {
	Repo     ScheduledPushRepo
	Dispatch DispatchFunc
	Logger   *slog.Logger
	Now      func() time.Time // injectable for tests; defaults to time.Now when nil.
}

// Scheduler holds an in-process map of workout_exercise_id → *time.Timer,
// persisted to SQLite so pending pushes survive restarts. Goroutine-safe.
type Scheduler struct {
	cfg    SchedulerConfig
	mu     sync.Mutex
	timers map[int]*time.Timer // keyed by workout_exercise_id
}

// NewScheduler constructs a Scheduler. Call Reload once at process start.
func NewScheduler(cfg SchedulerConfig) *Scheduler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Scheduler{
		cfg:    cfg,
		mu:     sync.Mutex{},
		timers: map[int]*time.Timer{},
	}
}

// Schedule persists the push and starts an in-process timer. If a timer for
// the same workout_exercise_id already exists, it is stopped and replaced
// (the repo's UNIQUE index handles the row-level replace).
func (s *Scheduler) Schedule(ctx context.Context, push domain.ScheduledPush) error {
	stored, err := s.cfg.Repo.Replace(ctx, push)
	if err != nil {
		return fmt.Errorf("persist scheduled push: %w", err)
	}
	s.startTimer(stored)
	return nil
}

// Cancel stops the in-process timer for the given workout_exercise_id and
// deletes its row from the repo. No-op if neither exists.
func (s *Scheduler) Cancel(ctx context.Context, workoutExerciseID int) error {
	s.mu.Lock()
	if t, ok := s.timers[workoutExerciseID]; ok {
		t.Stop()
		delete(s.timers, workoutExerciseID)
	}
	s.mu.Unlock()
	if err := s.cfg.Repo.DeleteByWorkoutExercise(ctx, workoutExerciseID); err != nil {
		return fmt.Errorf("delete scheduled push row: %w", err)
	}
	return nil
}

// CancelForWorkout stops every in-process timer whose row belongs to the
// given workout session and deletes all matching rows. Used by
// CompleteSession.
func (s *Scheduler) CancelForWorkout(ctx context.Context, userID int, date time.Time) error {
	all, err := s.cfg.Repo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list scheduled pushes for cancel: %w", err)
	}
	s.mu.Lock()
	for _, p := range all {
		if p.UserID != userID {
			continue
		}
		if t, ok := s.timers[p.WorkoutExerciseID]; ok {
			t.Stop()
			delete(s.timers, p.WorkoutExerciseID)
		}
	}
	s.mu.Unlock()
	if err = s.cfg.Repo.DeleteByWorkoutSession(ctx, userID, date); err != nil {
		return fmt.Errorf("delete scheduled pushes for workout: %w", err)
	}
	return nil
}

// Reload rebuilds the in-process timer map from persistent state. Past-due
// rows fire immediately on a fresh goroutine; future rows get a timer for
// the remaining delta. Call once at process start.
func (s *Scheduler) Reload(ctx context.Context) error {
	pending, err := s.cfg.Repo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list pending pushes: %w", err)
	}
	for _, p := range pending {
		s.startTimer(p)
	}
	return nil
}

// PendingCount returns the number of in-process timers. Used by IdleMonitor
// to gate process exit on a clean (no pushes pending) state.
func (s *Scheduler) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.timers)
}

func (s *Scheduler) startTimer(push domain.ScheduledPush) {
	delay := max(push.FireAt.Sub(s.cfg.Now()), 0)
	weID := push.WorkoutExerciseID
	timer := time.AfterFunc(delay, func() {
		s.fire(push)
	})
	s.mu.Lock()
	if existing, ok := s.timers[weID]; ok {
		existing.Stop()
	}
	s.timers[weID] = timer
	s.mu.Unlock()
}

func (s *Scheduler) fire(push domain.ScheduledPush) {
	s.mu.Lock()
	delete(s.timers, push.WorkoutExerciseID)
	s.mu.Unlock()

	// 30s is generous enough for a single Web Push round-trip plus row delete; tighter than
	// the 60s push TTL so we don't outlive the message we'd dispatch.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) //nolint:mnd // see above.
	defer cancel()

	if err := s.cfg.Dispatch(ctx, push); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "push dispatch failed",
			slog.Int("workout_exercise_id", push.WorkoutExerciseID),
			slog.Any("error", err))
	}
	if err := s.cfg.Repo.Delete(ctx, push.ID); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "delete scheduled push row after fire",
			slog.Int("id", push.ID),
			slog.Any("error", err))
	}
}
