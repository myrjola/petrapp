package errorrecorder

import (
	"log/slog"
	"testing"
	"time"
)

func makeRecord(level slog.Level, msg string, attrs ...slog.Attr) slog.Record {
	rec := slog.NewRecord(time.Unix(0, 0), level, msg, 0)
	rec.AddAttrs(attrs...)
	return rec
}

func TestResolveKey_SessionHashWinsOverTraceID(t *testing.T) {
	rec := makeRecord(slog.LevelInfo, "x",
		slog.String("session_hash", "sess1"),
		slog.String("trace_id", "trc1"),
	)
	if got := resolveKey(rec); got != "sess1" {
		t.Fatalf("resolveKey = %q, want sess1", got)
	}
}

func TestResolveKey_TraceIDFallback(t *testing.T) {
	rec := makeRecord(slog.LevelInfo, "x",
		slog.String("trace_id", "trc1"),
	)
	if got := resolveKey(rec); got != "trc1" {
		t.Fatalf("resolveKey = %q, want trc1", got)
	}
}

func TestResolveKey_NoKeyReturnsEmpty(t *testing.T) {
	rec := makeRecord(slog.LevelInfo, "x")
	if got := resolveKey(rec); got != "" {
		t.Fatalf("resolveKey = %q, want empty", got)
	}
}

func TestService_RecordAndSnapshot_Roundtrip(t *testing.T) {
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	s := newServiceForTest(t, clk, serviceTestParams{})

	s.record("sess1", makeRecord(slog.LevelInfo, "first"))
	clk.Advance(1 * time.Second)
	s.record("sess1", makeRecord(slog.LevelInfo, "second"))

	snap := s.snapshot("sess1")
	if len(snap) != 2 {
		t.Fatalf("snapshot length = %d, want 2", len(snap))
	}
	if snap[0].record.Message != "first" || snap[1].record.Message != "second" {
		t.Fatalf("snapshot order wrong: %q, %q", snap[0].record.Message, snap[1].record.Message)
	}
}

// serviceTestParams holds the optional knobs a test may override. Zero values
// mean "use sensible defaults". newServiceForTest fills in the rest.
type serviceTestParams struct {
	maxPerSession int
	maxSessions   int
	window        time.Duration
	rateLimit     int
}

func newServiceForTest(t *testing.T, clk Clock, p serviceTestParams) *Service {
	t.Helper()
	if p.maxPerSession == 0 {
		p.maxPerSession = 500
	}
	if p.maxSessions == 0 {
		p.maxSessions = 1000
	}
	if p.window == 0 {
		p.window = 10 * time.Minute
	}
	if p.rateLimit == 0 {
		p.rateLimit = 60
	}
	return &Service{
		clock:         clk,
		sessions:      map[string]*sessionBuffer{},
		maxPerSession: p.maxPerSession,
		maxSessions:   p.maxSessions,
		window:        p.window,
		rateLimit:     p.rateLimit,
	}
}
