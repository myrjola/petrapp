package main

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/errorrecorder"
	"github.com/myrjola/petrapp/internal/logging"
)

// TestRecorderProducesDumpFileOnError verifies the end-to-end wiring:
// ContextHandler enriches with session_hash from ctx, the recorder
// buffers the enriched record, and an Error-level record produces a
// JSONL file under a dated directory.
func TestRecorderProducesDumpFileOnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var stdout bytes.Buffer

	handlerOpts := &slog.HandlerOptions{Level: slog.LevelDebug} //nolint:exhaustruct // AddSource/ReplaceAttr default.
	recorder, err := errorrecorder.New(errorrecorder.Config{    //nolint:exhaustruct // Clock defaults to realClock.
		Inner:         slog.NewJSONHandler(&stdout, handlerOpts),
		LogsDirectory: dir,
		Window:        10 * time.Minute,
		RateLimit:     60,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = recorder.Close() })

	logger := slog.New(logging.NewContextHandler(recorder.Handler()))

	ctx := logging.WithAttrs(context.Background(),
		slog.String("session_hash", "abcdef0123456789"),
		slog.String("trace_id", "trc-xxxx"),
	)

	logger.LogAttrs(ctx, slog.LevelInfo, "user opened workout page")
	logger.LogAttrs(ctx, slog.LevelInfo, "loaded session for date 2026-05-27")
	logger.LogAttrs(ctx, slog.LevelError, "failed to render workout", slog.String("err", "boom"))

	if err = recorder.WaitForDumps(1, 2*time.Second); err != nil {
		t.Fatalf("WaitForDumps: %v", err)
	}

	var found []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly 1 dump file, got %d: %v", len(found), found)
	}

	body, err := os.ReadFile(found[0])
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	content := string(body)
	for _, msg := range []string{"user opened workout page", "loaded session for date 2026-05-27", "failed to render workout"} {
		if !strings.Contains(content, msg) {
			t.Errorf("dump missing %q; content=%q", msg, content)
		}
	}
	if !strings.Contains(content, `"session_hash":"abcdef0123456789"`) {
		t.Errorf("dump missing session_hash; content=%q", content)
	}
}
