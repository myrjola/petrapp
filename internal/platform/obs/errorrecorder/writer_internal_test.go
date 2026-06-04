package errorrecorder

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteDump_ProducesDatedDirAndJSONLFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := time.Date(2026, 5, 27, 14, 32, 17, 0, time.UTC)
	records := []bufferedRecord{
		{record: makeRecord(slog.LevelInfo, "first", slog.String("k", "v1")), addedAt: ts},
		{record: makeRecord(slog.LevelError, "boom", slog.String("k", "v2")), addedAt: ts},
	}

	//nolint:exhaustruct // remaining HandlerOptions fields are zero by design.
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	path, err := writeDump(dir, "3f9a12bcde", ts, records, opts)
	if err != nil {
		t.Fatalf("writeDump: %v", err)
	}

	wantPath := filepath.Join(dir, "2026", "05", "27", "20260527T143217Z-3f9a12bc.jsonl")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0o600", info.Mode().Perm())
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var lines []map[string]any
	for scanner.Scan() {
		var line map[string]any
		if err = json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("parse line %q: %v", scanner.Text(), err)
		}
		lines = append(lines, line)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0]["msg"] != "first" || lines[1]["msg"] != "boom" {
		t.Errorf("messages = %v, %v", lines[0]["msg"], lines[1]["msg"])
	}
	if lines[1]["level"] != "ERROR" {
		t.Errorf("second line level = %v, want ERROR", lines[1]["level"])
	}
}

func TestWriteDump_CollisionRetriesWithSuffix(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := time.Date(2026, 5, 27, 14, 32, 17, 0, time.UTC)
	records := []bufferedRecord{{record: makeRecord(slog.LevelError, "x"), addedAt: ts}}

	//nolint:exhaustruct // remaining HandlerOptions fields are zero by design.
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	first, err := writeDump(dir, "abcdef1234", ts, records, opts)
	if err != nil {
		t.Fatalf("first writeDump: %v", err)
	}
	second, err := writeDump(dir, "abcdef1234", ts, records, opts)
	if err != nil {
		t.Fatalf("second writeDump: %v", err)
	}
	if first == second {
		t.Fatalf("second dump reused path %q", first)
	}
	if !strings.HasSuffix(second, "-1.jsonl") {
		t.Errorf("second path = %q, want suffix -1.jsonl", second)
	}
}
