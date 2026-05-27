package notification

import (
	"context"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/logging"
)

// IdleMonitorConfig configures an IdleMonitor.
type IdleMonitorConfig struct {
	IdleThreshold time.Duration
	TickInterval  time.Duration
	Now           func() time.Time
	LastRequestAt func() time.Time
	PendingCount  func() int
	// Trigger fires when idle + no pending. In production this sends
	// SIGTERM to the process; tests substitute a flag-set.
	Trigger func()
	Logger  *slog.Logger
}

// IdleMonitor watches request inactivity and a pending-push count, and fires
// Trigger when both conditions are met. Used by main.go to scale the Fly
// Machine to zero between workouts.
type IdleMonitor struct {
	cfg IdleMonitorConfig
}

// NewIdleMonitor constructs an IdleMonitor.
func NewIdleMonitor(cfg IdleMonitorConfig) *IdleMonitor {
	return &IdleMonitor{cfg: cfg}
}

// Run blocks until ctx is cancelled. Fires Trigger at most once per Run.
func (m *IdleMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickCtx := logging.WithAttrs(ctx,
				slog.String("trace_id", logging.NewTraceID()),
				slog.String("component", "idle_monitor"),
			)
			if m.shouldTrigger() {
				m.cfg.Logger.LogAttrs(tickCtx, slog.LevelInfo, "idle monitor triggering shutdown")
				m.cfg.Trigger()
				return
			}
		}
	}
}

func (m *IdleMonitor) shouldTrigger() bool {
	idleFor := m.cfg.Now().Sub(m.cfg.LastRequestAt())
	if idleFor < m.cfg.IdleThreshold {
		return false
	}
	if m.cfg.PendingCount() > 0 {
		return false
	}
	return true
}
