package flightrecorder_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/platform/obs/flightrecorder"
)

// TestService_CapturePrunesOldTraces verifies that a capture caps the trace
// directory at MaxFiles, deleting the oldest files. Each trace is ~tens of MB,
// so an unbounded directory would eventually exhaust the Fly volume.
//
//nolint:paralleltest // runtime/trace.NewFlightRecorder is a process-global singleton.
func TestService_CapturePrunesOldTraces(t *testing.T) {
	traceDir := t.TempDir()

	// Pre-populate the directory with stale trace files, back-dated so the
	// real capture below is unambiguously the newest by modification time.
	old := time.Now().Add(-time.Hour)
	for _, name := range []string{"slow-1.trace", "slow-2.trace", "timeout-3.trace", "slow-4.trace"} {
		fPath := filepath.Join(traceDir, name)
		if err := os.WriteFile(fPath, []byte("stale"), 0o600); err != nil {
			t.Fatalf("write stale trace %s: %v", name, err)
		}
		if err := os.Chtimes(fPath, old, old); err != nil {
			t.Fatalf("backdate stale trace %s: %v", name, err)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	service, err := flightrecorder.New(flightrecorder.Config{
		Logger:          logger,
		MinAge:          0,
		MaxBytes:        0,
		MaxFiles:        2,
		TracesDirectory: traceDir,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	if err = service.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer service.Stop(ctx)

	service.CaptureTimeoutTrace(ctx)

	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatalf("read trace directory: %v", err)
	}
	if len(entries) != 2 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("expected MaxFiles=2 traces after prune, got %d: %v", len(entries), names)
	}

	// The freshly captured trace must survive the prune. The stale files all
	// carry the "stale" marker; the real capture carries actual trace bytes.
	var keptCapture bool
	for _, e := range entries {
		content, readErr := os.ReadFile(filepath.Join(traceDir, e.Name()))
		if readErr != nil {
			t.Fatalf("read trace %s: %v", e.Name(), readErr)
		}
		if string(content) != "stale" {
			keptCapture = true
		}
	}
	if !keptCapture {
		t.Error("expected the freshly captured trace to survive pruning")
	}
}
