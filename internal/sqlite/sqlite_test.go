package sqlite_test

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// TestNewDatabase_ReadOnlyHandleRejectsWrites locks the in-memory DSN shape:
// the read-only *sql.DB must reject INSERT/UPDATE statements regardless of
// how the URI's mode= parameters are arranged.
func TestNewDatabase_ReadOnlyHandleRejectsWrites(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.ReadOnly.ExecContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?)",
		[]byte("ro-test"), "RO Test")
	if err == nil {
		t.Fatal("expected read-only handle to reject INSERT, got nil error")
	}
	// The driver returns an "attempt to write a readonly database" error in some
	// configurations and a query_only-style refusal in others; either is fine.
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "readonly") && !strings.Contains(msg, "read only") &&
		!strings.Contains(msg, "query_only") {
		t.Errorf("expected readonly-style error, got %v", err)
	}
}

// blockingWriter records overlapping writes so the test can prove that
// Close() returns only after the background optimizer goroutine has stopped
// emitting log records.
type blockingWriter struct {
	active atomic.Int32
	delay  time.Duration
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.active.Add(1)
	defer w.active.Add(-1)
	time.Sleep(w.delay)
	return len(p), nil
}

// TestNewDatabase_CloseWaitsForBackgroundOptimizer pins the contract that
// Close() must not return while the optimizer goroutine is still emitting
// log records. Without that guarantee the goroutine can race with the test
// writer's t.Cleanup hook and panic with "attempted to write after test
// completion".
func TestNewDatabase_CloseWaitsForBackgroundOptimizer(t *testing.T) {
	w := &blockingWriter{active: atomic.Int32{}, delay: 100 * time.Millisecond}
	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		AddSource:   false,
		Level:       slog.LevelDebug,
		ReplaceAttr: nil,
	}))

	db, err := sqlite.NewDatabase(context.Background(), ":memory:", logger)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}

	// Wait until the optimizer goroutine is mid-write so the close-vs-write
	// race is forced. The goroutine logs success on its first PRAGMA optimize.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.active.Load() > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if w.active.Load() == 0 {
		t.Fatalf("optimizer goroutine never wrote a log record within 2s")
	}

	if err = db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if active := w.active.Load(); active > 0 {
		t.Fatalf("Close returned while optimizer still writing (active=%d)", active)
	}
}
