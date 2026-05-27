package errorrecorder

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func newHandlerForTest(t *testing.T, inner slog.Handler, clk Clock) *Handler {
	t.Helper()
	svc := newServiceForTest(t, clk, serviceTestParams{}) //nolint:exhaustruct // helper fills the rest.
	svc.inner = inner
	return &Handler{service: svc, inner: inner} //nolint:exhaustruct // withAttrs/withinGroup zero-valued by design.
}

func TestHandler_ForwardsToInnerAndBuffersRecord(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	h := newHandlerForTest(t, inner, clk)
	logger := slog.New(h)

	logger.LogAttrs(context.Background(), slog.LevelInfo, "hello",
		slog.String("session_hash", "sess1"))

	if !strings.Contains(buf.String(), `"msg":"hello"`) {
		t.Errorf("inner handler did not see record; buf=%q", buf.String())
	}
	if got := len(h.service.snapshot("sess1")); got != 1 {
		t.Errorf("buffer length = %d, want 1", got)
	}
}

func TestHandler_WithAttrs_PreservesAttrInBufferedRecord(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	h := newHandlerForTest(t, inner, clk)
	logger := slog.New(h).With(slog.String("session_hash", "sess-from-With"))

	logger.LogAttrs(context.Background(), slog.LevelInfo, "hi")

	snap := h.service.snapshot("sess-from-With")
	if len(snap) != 1 {
		t.Fatalf("expected record under sess-from-With, snapshot len=%d", len(snap))
	}
}

func TestHandler_NoKey_SkipsBufferingButForwards(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, nil)
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	h := newHandlerForTest(t, inner, clk)
	logger := slog.New(h)

	logger.LogAttrs(context.Background(), slog.LevelInfo, "no-key")

	if !strings.Contains(buf.String(), `"msg":"no-key"`) {
		t.Errorf("inner handler missed record; buf=%q", buf.String())
	}
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	if len(h.service.sessions) != 0 {
		t.Errorf("expected no buffered sessions, got %d", len(h.service.sessions))
	}
}
