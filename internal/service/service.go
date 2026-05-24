// Package service holds workout orchestration: cross-aggregate coordination,
// external integrations (OpenAI, GDPR export), and the methods called by
// HTTP handlers. Pure rules live in internal/domain; persistence lives in
// internal/repository. This package depends on both.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// PushScheduler is the subset of notification.Scheduler the service depends on.
// Declared as an interface so tests can substitute a fake or pass nil. The real
// implementation lives in internal/notification — this package doesn't import
// it, keeping the dependency graph clean.
type PushScheduler interface {
	Schedule(ctx context.Context, push domain.ScheduledPush) error
	Cancel(ctx context.Context, workoutExerciseID int) error
	CancelForWorkout(ctx context.Context, userID int, date time.Time) error
}

// Service coordinates workout-domain operations across the repository
// layer and external integrations. One instance per process; safe for
// concurrent use because each method opens its own DB transaction.
type Service struct {
	repos            *repository.Repositories
	db               *sqlite.Database
	logger           *slog.Logger
	openaiAPIKey     string
	scheduler        PushScheduler // nil-safe; methods no-op when nil.
	maintenanceCache *maintenanceCache
	// userLocks serializes operations that must not race per-user (today:
	// RegenerateWeeklyPlanIfUnstarted). Keyed by user ID; entries are never
	// evicted — the working set is the active-user count, which is small.
	// Stored as *sync.Map (not value) so With* shallow copies share state.
	userLocks *sync.Map // map[int]*sync.Mutex
}

// NewService creates a new workout service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	return &Service{
		repos:            repository.New(db),
		db:               db,
		logger:           logger,
		openaiAPIKey:     openaiAPIKey,
		scheduler:        nil,
		maintenanceCache: newMaintenanceCache(),
		userLocks:        &sync.Map{},
	}
}

// Repos exposes the wired repositories so the notification.Scheduler can
// reuse them at process startup without re-instantiating. Only intended
// for main.go; HTTP handlers should call typed Service methods instead.
func (s *Service) Repos() *repository.Repositories {
	return s.repos
}

// WithScheduler returns a copy of the service wired to a push scheduler.
// Called from main.go after the notification package is initialised. Tests
// that need scheduling behaviour call this with a fake; tests that don't
// leave it nil.
func (s *Service) WithScheduler(scheduler PushScheduler) *Service {
	cp := *s
	cp.scheduler = scheduler
	return &cp
}

// GetUserPreferences retrieves the workout preferences for a user.
func (s *Service) GetUserPreferences(ctx context.Context) (domain.Preferences, error) {
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return domain.Preferences{}, fmt.Errorf("get user preferences: %w", err)
	}
	return prefs, nil
}

// SaveUserPreferences saves the workout preferences for a user.
// If deload is being enabled and no anchor is provided, the anchor is snapped
// to the next Monday so the first mesocycle starts with an accumulation week.
func (s *Service) SaveUserPreferences(ctx context.Context, prefs domain.Preferences) error {
	current, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("load current preferences: %w", err)
	}
	// Snap anchor to next Monday when deload is enabled but neither the incoming
	// prefs nor the stored prefs carry an anchor.
	if prefs.DeloadEnabled && prefs.MesocycleAnchor.IsZero() && current.MesocycleAnchor.IsZero() {
		prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
	}
	// Preserve an existing anchor when the caller omits it but deload is still on.
	if prefs.DeloadEnabled && prefs.MesocycleAnchor.IsZero() && !current.MesocycleAnchor.IsZero() {
		prefs.MesocycleAnchor = current.MesocycleAnchor
	}
	if err = s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save user preferences: %w", err)
	}
	return nil
}

// RestartMesocycleAnchor snaps the mesocycle anchor to the next Monday,
// effectively restarting the deload cycle from that date. Additionally
// clears IsDeload on every current-week session dated today or later
// that is not already fully completed, so an accidental StartDeloadNow
// press can be fully undone with one click.
//
//nolint:dupl // mirror of StartDeloadNow; kept separate intentionally (ClearDeload vs SwitchToDeload, distinct intent).
func (s *Service) RestartMesocycleAnchor(ctx context.Context) error {
	monday := domain.MondayOf(time.Now())
	today := domain.StartOfDay(time.Now())

	sessions, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return fmt.Errorf("list sessions for current week: %w", err)
	}
	for _, sess := range sessions {
		if sess.Date.Before(today) {
			continue
		}
		err = s.repos.Sessions.Update(ctx, sess.Date, func(latest *domain.Session) error {
			if latest.Status() == domain.SessionCompleted {
				return nil
			}
			return latest.ClearDeload()
		})
		if err != nil {
			return fmt.Errorf("clear deload for %s: %w", sess.Date.Format(time.DateOnly), err)
		}
	}

	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
	if err = s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	return nil
}

// userMutex returns the per-user mutex, creating it on first access.
func (s *Service) userMutex(userID int) *sync.Mutex {
	if m, ok := s.userLocks.Load(userID); ok {
		return m.(*sync.Mutex) //nolint:forcetypeassert,errcheck // value is always *sync.Mutex.
	}
	m, _ := s.userLocks.LoadOrStore(userID, &sync.Mutex{})
	return m.(*sync.Mutex) //nolint:forcetypeassert,errcheck // value is always *sync.Mutex.
}

// nextMonday returns the upcoming Monday at 00:00 UTC. If now is already a
// Monday, it returns that Monday. Callers pass a UTC instant.
func nextMonday(now time.Time) time.Time {
	monday := domain.MondayOf(now)
	if now.Weekday() == time.Monday {
		return monday
	}
	return monday.AddDate(0, 0, 7)
}
