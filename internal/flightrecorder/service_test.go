package flightrecorder_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/flightrecorder"
)

func TestService_StartStop(t *testing.T) {
	// Create temporary trace directory
	traceDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	service, err := flightrecorder.New(flightrecorder.Config{
		Logger:          logger,
		MinAge:          0, // Use default
		MaxBytes:        0, // Use default
		TracesDirectory: traceDir,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()

	// Test starting
	if err = service.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Test stopping
	service.Stop(ctx)
}

func TestService_CaptureTimeoutTrace(t *testing.T) {
	// Create temporary trace directory
	traceDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	service, err := flightrecorder.New(flightrecorder.Config{
		Logger:          logger,
		MinAge:          0, // Use default
		MaxBytes:        0, // Use default
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

	// Capture a trace
	service.CaptureTimeoutTrace(ctx)

	// Check that a trace file was created
	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatalf("failed to read trace directory: %v", err)
	}

	if len(entries) == 0 {
		t.Error("expected at least one trace file to be created")
		return
	}

	// Verify the filename format
	filename := entries[0].Name()
	if !strings.HasPrefix(filename, "timeout-") {
		t.Errorf("expected filename to start with 'timeout-', got %s", filename)
	}
	if !strings.HasSuffix(filename, ".trace") {
		t.Errorf("expected filename to end with '.trace', got %s", filename)
	}
}

func TestService_CooldownPreventsCapture(t *testing.T) {
	// This test is simplified since we can't access private fields from external package
	// We'll test that the service can start and stop without errors

	// Create temporary trace directory
	traceDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	service, err := flightrecorder.New(flightrecorder.Config{
		Logger:          logger,
		MinAge:          0, // Use default
		MaxBytes:        0, // Use default
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

	// Capture first trace
	service.CaptureTimeoutTrace(ctx)

	// Immediately try to capture another trace (should be blocked by cooldown)
	service.CaptureTimeoutTrace(ctx)

	// Check that only one trace file was created (due to cooldown)
	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatalf("failed to read trace directory: %v", err)
	}

	if len(entries) > 1 {
		t.Error("expected cooldown to prevent rapid successive captures")
	}
}
