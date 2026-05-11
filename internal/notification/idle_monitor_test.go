package notification_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/notification"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func TestIdleMonitor_TriggersWhenIdleAndNoPending(t *testing.T) {
	t.Parallel()

	var fakeNow atomic.Int64
	fakeNow.Store(time.Now().UnixNano())
	var lastRequest atomic.Int64
	lastRequest.Store(time.Now().UnixNano())
	var pending atomic.Int32
	var triggered atomic.Bool

	mon := notification.NewIdleMonitor(notification.IdleMonitorConfig{
		IdleThreshold: 50 * time.Millisecond,
		TickInterval:  10 * time.Millisecond,
		Now:           func() time.Time { return time.Unix(0, fakeNow.Load()) },
		LastRequestAt: func() time.Time { return time.Unix(0, lastRequest.Load()) },
		PendingCount:  func() int { return int(pending.Load()) },
		Trigger:       func() { triggered.Store(true) },
		Logger:        testhelpers.NewLogger(testhelpers.NewWriter(t)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go mon.Run(ctx)

	// Advance simulated time past the idle threshold while keeping pending=0.
	fakeNow.Store(time.Now().Add(time.Second).UnixNano())

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !triggered.Load() {
		time.Sleep(10 * time.Millisecond)
	}
	if !triggered.Load() {
		t.Fatal("idle monitor never triggered")
	}
}

func TestIdleMonitor_BlockedByPending(t *testing.T) {
	t.Parallel()

	var fakeNow atomic.Int64
	fakeNow.Store(time.Now().UnixNano())
	var lastRequest atomic.Int64
	lastRequest.Store(time.Now().UnixNano())
	var pending atomic.Int32
	pending.Store(1)
	var triggered atomic.Bool

	mon := notification.NewIdleMonitor(notification.IdleMonitorConfig{
		IdleThreshold: 20 * time.Millisecond,
		TickInterval:  5 * time.Millisecond,
		Now:           func() time.Time { return time.Unix(0, fakeNow.Load()) },
		LastRequestAt: func() time.Time { return time.Unix(0, lastRequest.Load()) },
		PendingCount:  func() int { return int(pending.Load()) },
		Trigger:       func() { triggered.Store(true) },
		Logger:        testhelpers.NewLogger(testhelpers.NewWriter(t)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go mon.Run(ctx)

	fakeNow.Store(time.Now().Add(time.Second).UnixNano())
	time.Sleep(100 * time.Millisecond)
	if triggered.Load() {
		t.Error("idle monitor triggered despite pending > 0")
	}
}
