// Package flightrecorder provides trace capture functionality for request timeouts.
package flightrecorder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/trace"
	"sync/atomic"
	"time"
)

const (
	// defaultMinAge is the minimum age of trace events to keep.
	defaultMinAge = 5 * time.Minute

	// defaultMaxBytes is the maximum size of the trace buffer.
	defaultMaxBytes = 64 * 1024 * 1024 // 64MB

	// cooldownDuration is the minimum time between trace captures.
	cooldownDuration = 30 * time.Minute
)

// Service manages flight recording for timeout detection.
type Service struct {
	logger          *slog.Logger
	flightRecorder  *trace.FlightRecorder
	tracesDirectory string
	lastCapture     atomic.Int64 // Unix timestamp of last capture
}

// Config configures the flight recorder service.
type Config struct {
	Logger          *slog.Logger
	MinAge          time.Duration // Minimum age of trace events
	MaxBytes        uint64        // Maximum size of trace buffer
	TracesDirectory string        // Directory where trace files are written
}

// New creates a new flight recorder service.
func New(cfg Config) (*Service, error) {
	if cfg.Logger == nil {
		return nil, errors.New("logger is required")
	}

	if cfg.TracesDirectory == "" {
		return nil, errors.New("traces directory is required")
	}

	if stat, err := os.Stat(cfg.TracesDirectory); err != nil {
		// Create the directory if it doesn't exist
		if err = os.MkdirAll(cfg.TracesDirectory, 0500); err != nil {
			return nil, fmt.Errorf("create traces directory: %w", err)
		}
	} else if !stat.IsDir() {
		return nil, fmt.Errorf("traces path is not a directory: %s", cfg.TracesDirectory)
	}

	minAge := cfg.MinAge
	if minAge == 0 {
		minAge = defaultMinAge
	}

	maxBytes := cfg.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxBytes
	}

	flightRecorderCfg := trace.FlightRecorderConfig{
		MinAge:   minAge,
		MaxBytes: maxBytes,
	}

	flightRecorder := trace.NewFlightRecorder(flightRecorderCfg)
	if flightRecorder == nil {
		return nil, errors.New("failed to create flight recorder")
	}

	return &Service{
		logger:          cfg.Logger,
		flightRecorder:  flightRecorder,
		tracesDirectory: cfg.TracesDirectory,
		lastCapture:     atomic.Int64{},
	}, nil
}

// Start begins flight recording.
func (s *Service) Start(ctx context.Context) error {
	if err := s.flightRecorder.Start(); err != nil {
		return fmt.Errorf("start flight recorder: %w", err)
	}

	s.logger.LogAttrs(ctx, slog.LevelInfo, "flight recorder started",
		slog.String("min_age", defaultMinAge.String()),
		slog.Uint64("max_bytes", defaultMaxBytes),
		slog.String("cooldown", cooldownDuration.String()))

	return nil
}

// Stop ends flight recording.
func (s *Service) Stop(ctx context.Context) {
	s.flightRecorder.Stop()

	s.logger.LogAttrs(ctx, slog.LevelInfo, "flight recorder stopped")
}

// CaptureTimeoutTrace captures a trace when a request times out.
// It respects the cooldown period to avoid overwhelming the filesystem.
func (s *Service) CaptureTimeoutTrace(ctx context.Context) {
	// Check cooldown period
	now := time.Now().Unix()
	lastCapture := s.lastCapture.Load()

	if lastCapture > 0 && time.Unix(now, 0).Sub(time.Unix(lastCapture, 0)) < cooldownDuration {
		s.logger.LogAttrs(ctx, slog.LevelDebug, "skipping trace capture due to cooldown",
			slog.Time("last_capture", time.Unix(lastCapture, 0)),
			slog.Duration("remaining_cooldown", cooldownDuration-time.Unix(now, 0).Sub(time.Unix(lastCapture, 0))))
		return
	}

	// Update last capture time atomically
	if !s.lastCapture.CompareAndSwap(lastCapture, now) {
		// Another goroutine updated the timestamp, respect that
		return
	}

	// Generate filename with timestamp and request info
	timestamp := time.Unix(now, 0).UTC().Format("20060102-150405")
	filename := fmt.Sprintf("timeout-%s.trace", timestamp)
	fPath := filepath.Join(s.tracesDirectory, filename)

	// Create and write the trace file
	file, err := os.Create(fPath)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelError, "failed to create trace file",
			slog.String("file", fPath),
			slog.Any("error", err))
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			s.logger.LogAttrs(ctx, slog.LevelError, "failed to close trace file",
				slog.String("file", fPath),
				slog.Any("error", closeErr))
		}
	}()

	// Write the flight recorder trace to file
	bytesWritten, err := s.flightRecorder.WriteTo(file)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelError, "failed to write trace",
			slog.String("file", fPath),
			slog.Any("error", err))
		return
	}

	s.logger.LogAttrs(ctx, slog.LevelWarn, "captured timeout trace",
		slog.String("file", fPath),
		slog.Int64("bytes", bytesWritten))
}
