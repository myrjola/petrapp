package notification

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
	"github.com/myrjola/petrapp/internal/platform/obs/logging"
)

// DispatchFunc is called when a scheduled push fires. Implementations should
// load subscriptions, send the payload, and prune 410'd subscriptions. The
// scheduler invokes Dispatch from a fresh goroutine; implementations are
// responsible for their own context propagation.
type DispatchFunc func(ctx context.Context, push domain.ScheduledPush) error

// ScheduledPushRepo is the subset of the scheduled-push repository's methods the
// scheduler needs. Declared locally so the package doesn't import the whole
// repository package — keeps notification at a lower layer in the dep graph.
// repository's concrete *sqliteScheduledPushRepository satisfies it structurally.
type ScheduledPushRepo interface {
	Replace(ctx context.Context, push domain.ScheduledPush) (domain.ScheduledPush, error)
	Delete(ctx context.Context, id int) error
	DeleteBySlot(ctx context.Context, userID int, date time.Time, pos int) error
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

// slotKey identifies a slot for timer bookkeeping. Mirrors the composite
// natural key on scheduled_pushes (workout_user_id, workout_date, position).
type slotKey struct {
	userID int
	date   string // YYYY-MM-DD, formatted via time.DateOnly so map keys compare cleanly.
	pos    int
}

// Scheduler holds an in-process map of slot → *time.Timer, persisted to
// SQLite so pending pushes survive restarts. Goroutine-safe.
type Scheduler struct {
	cfg    SchedulerConfig
	mu     sync.Mutex
	timers map[slotKey]*time.Timer
}

// NewScheduler constructs a Scheduler. Call Reload once at process start.
func NewScheduler(cfg SchedulerConfig) *Scheduler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Scheduler{
		cfg:    cfg,
		mu:     sync.Mutex{},
		timers: map[slotKey]*time.Timer{},
	}
}

// Schedule persists the push and starts an in-process timer. If a timer for
// the same slot already exists, it is stopped and replaced (the repo's UNIQUE
// index handles the row-level replace).
func (s *Scheduler) Schedule(ctx context.Context, push domain.ScheduledPush) error {
	stored, err := s.cfg.Repo.Replace(ctx, push)
	if err != nil {
		return fmt.Errorf("persist scheduled push: %w", err)
	}
	s.startTimer(stored)
	return nil
}

// Cancel stops the in-process timer for the given slot and deletes its row
// from the repo. No-op if neither exists.
func (s *Scheduler) Cancel(ctx context.Context, userID int, date time.Time, pos int) error {
	key := slotKey{userID: userID, date: date.Format(time.DateOnly), pos: pos}
	s.mu.Lock()
	if t, ok := s.timers[key]; ok {
		t.Stop()
		delete(s.timers, key)
	}
	s.mu.Unlock()
	if err := s.cfg.Repo.DeleteBySlot(ctx, userID, date, pos); err != nil {
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
	dateStr := date.Format(time.DateOnly)
	s.mu.Lock()
	for _, p := range all {
		if p.UserID != userID || p.WorkoutDate.Format(time.DateOnly) != dateStr {
			continue
		}
		key := slotKey{userID: p.UserID, date: dateStr, pos: p.Position}
		if t, ok := s.timers[key]; ok {
			t.Stop()
			delete(s.timers, key)
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
	key := slotKey{
		userID: push.UserID,
		date:   push.WorkoutDate.Format(time.DateOnly),
		pos:    push.Position,
	}

	// Hold the mutex across AfterFunc creation and map install. AfterFunc's callback runs on its
	// own goroutine and will block on s.mu inside fire() until we release here, so the map entry
	// is guaranteed installed before fire() can observe (or fail to find) it. This closes a race
	// where a zero-delay timer could fire before the map install completed, leaking the entry.
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.timers[key]; ok {
		existing.Stop()
	}
	// Heap-allocate the timer-pointer holder so the AfterFunc closure can read it after we've
	// written it without racing on the stack-local. We hold s.mu here, and fire() acquires s.mu
	// before dereferencing the holder, so the write below happens-before any read in fire().
	selfBox := new(*time.Timer)
	*selfBox = time.AfterFunc(delay, func() {
		s.fire(selfBox, key, push)
	})
	s.timers[key] = *selfBox
}

func (s *Scheduler) fire(selfBox **time.Timer, key slotKey, push domain.ScheduledPush) {
	s.mu.Lock()
	// Identity check: only clear the map entry if it still points at *this* timer. A concurrent
	// Schedule may have installed a replacement between our timer firing and us acquiring the
	// lock; in that case the replacement is the rightful map owner and must not be evicted.
	self := *selfBox
	if current, ok := s.timers[key]; ok && current == self {
		delete(s.timers, key)
	}
	s.mu.Unlock()

	// 30s is generous enough for a single Web Push round-trip plus row delete; tighter than
	// the 60s push TTL so we don't outlive the message we'd dispatch.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) //nolint:mnd // see above.
	defer cancel()
	ctx = logging.WithAttrs(ctx, slog.String("trace_id", logging.NewTraceID()))

	if err := s.cfg.Dispatch(ctx, push); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "push dispatch failed",
			slog.Int("user_id", push.UserID),
			slog.String("workout_date", key.date),
			slog.Int("position", key.pos),
			slog.Any("error", err))
	}
	if err := s.cfg.Repo.Delete(ctx, push.ID); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "delete scheduled push row after fire",
			slog.Int("user_id", push.UserID),
			slog.String("workout_date", key.date),
			slog.Int("position", key.pos),
			slog.Int("id", push.ID),
			slog.Any("error", err))
	}
}
