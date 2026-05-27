package errorrecorder

import (
	"fmt"
	"log/slog"
	"reflect"
	"testing"
	"time"
)

//nolint:unparam // level will vary once later tasks add LevelError tests.
func makeRecord(level slog.Level, msg string, attrs ...slog.Attr) slog.Record {
	rec := slog.NewRecord(time.Unix(0, 0), level, msg, 0)
	rec.AddAttrs(attrs...)
	return rec
}

func TestResolveKey_SessionHashWinsOverTraceID(t *testing.T) {
	t.Parallel()
	rec := makeRecord(slog.LevelInfo, "x",
		slog.String("session_hash", "sess1"),
		slog.String("trace_id", "trc1"),
	)
	if got := resolveKey(rec); got != "sess1" {
		t.Fatalf("resolveKey = %q, want sess1", got)
	}
}

func TestResolveKey_TraceIDFallback(t *testing.T) {
	t.Parallel()
	rec := makeRecord(slog.LevelInfo, "x",
		slog.String("trace_id", "trc1"),
	)
	if got := resolveKey(rec); got != "trc1" {
		t.Fatalf("resolveKey = %q, want trc1", got)
	}
}

func TestResolveKey_NoKeyReturnsEmpty(t *testing.T) {
	t.Parallel()
	rec := makeRecord(slog.LevelInfo, "x")
	if got := resolveKey(rec); got != "" {
		t.Fatalf("resolveKey = %q, want empty", got)
	}
}

func TestService_RecordAndSnapshot_Roundtrip(t *testing.T) {
	t.Parallel()
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	s := newServiceForTest(t, clk, serviceTestParams{}) //nolint:exhaustruct // defaults filled in helper.

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

func TestService_RingOverflow_DropsOldest(t *testing.T) {
	t.Parallel()
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	s := newServiceForTest(t, clk, serviceTestParams{maxPerSession: 3}) //nolint:exhaustruct // helper fills the rest.

	for i := range 5 {
		s.record("sess1", makeRecord(slog.LevelInfo, fmt.Sprintf("m%d", i)))
		clk.Advance(1 * time.Second)
	}

	snap := s.snapshot("sess1")
	if len(snap) != 3 {
		t.Fatalf("snapshot length = %d, want 3", len(snap))
	}
	got := []string{snap[0].record.Message, snap[1].record.Message, snap[2].record.Message}
	want := []string{"m2", "m3", "m4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
	}
}

func TestService_PruneOnce_DropsExpiredSessions(t *testing.T) {
	t.Parallel()
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	s := newServiceForTest(
		t,
		clk,
		serviceTestParams{window: 5 * time.Minute},
	) //nolint:exhaustruct // helper fills the rest.

	s.record("old", makeRecord(slog.LevelInfo, "old"))
	clk.Advance(10 * time.Minute)
	s.record("new", makeRecord(slog.LevelInfo, "new"))

	s.pruneOnce()

	if len(s.snapshot("old")) != 0 {
		t.Errorf("expected 'old' session pruned, snapshot=%v", s.snapshot("old"))
	}
	if got := len(s.snapshot("new")); got != 1 {
		t.Errorf("expected 'new' session to survive with 1 record, got %d", got)
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
	return &Service{ //nolint:exhaustruct // mu and dump-related fields default-init for tests.
		clock:         clk,
		sessions:      map[string]*sessionBuffer{},
		maxPerSession: p.maxPerSession,
		maxSessions:   p.maxSessions,
		window:        p.window,
		rateLimit:     p.rateLimit,
	}
}
