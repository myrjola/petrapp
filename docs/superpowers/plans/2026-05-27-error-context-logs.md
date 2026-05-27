# Error Context Logs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the app logs an error, write a `.jsonl` file under `/data/logs/YYYY/MM/DD/` containing the recent (last ~10 min) log lines for the same session, so an operator can see what led to the failure.

**Architecture:** A new `internal/errorrecorder` package exposes an `slog.Handler` decorator. It sits *inside* the existing `logging.ContextHandler` so it observes records already enriched with context attrs. Buffered records key off `session_hash` (added by `webauthnhandler.AuthenticateMiddleware`) or fall back to `trace_id` (added by `logAndTraceRequest`). On any `slog.LevelError` record, the matching session's recent records are snapshotted under a mutex and a background worker goroutine writes them to disk. Bounded buffers, an LRU on the session map, and a global 60-dumps/hour rate limit cap resource use. Spec: `docs/superpowers/specs/2026-05-27-error-context-logs-design.md`.

**Tech Stack:** Go 1.24+, `log/slog`, `crypto/rand` (already in use for trace IDs), standard library only.

---

## File Structure

**New files:**
- `internal/errorrecorder/doc.go` — package doc + shared constants.
- `internal/errorrecorder/clock.go` — minimal `Clock` interface, `realClock`, `fakeClock`.
- `internal/errorrecorder/service.go` — `Service` struct: buffer map, key resolution, rate limit, Start/Close lifecycle.
- `internal/errorrecorder/handler.go` — `Handler` (the `slog.Handler` decorator) with `WithAttrs`/`WithGroup`/`Handle`.
- `internal/errorrecorder/writer.go` — pure file-writing helper: path computation, JSONL replay, collision retry.
- `internal/errorrecorder/service_test.go` — buffer, key, rate-limit, lifecycle tests.
- `internal/errorrecorder/handler_test.go` — decorator behavior, WithAttrs preservation, disabled mode.
- `internal/errorrecorder/writer_test.go` — path / format / collision tests.

**Modified files:**
- `internal/logging/contexthandler.go` — add `NewTraceID()` helper.
- `cmd/web/middleware.go` — use `logging.NewTraceID()` instead of inline `rand.Text()` for trace_id.
- `internal/notification/scheduler.go` — attach a fresh `trace_id` to ctx inside `fire`.
- `internal/notification/idle_monitor.go` — attach a fresh `trace_id` to ctx inside each tick.
- `cmd/web/main.go` — add `LogsDirectory` config field, wire the recorder, defer `Close`.
- `internal/e2etest/server.go` — read `PETRAPP_LOGS_DIRECTORY` from `lookupEnv` and wire the recorder if set.
- `cmd/web/handler-error_test.go` — new integration test asserting a dump file appears under a temp dir.
- `fly.toml` — add `PETRAPP_LOGS_DIRECTORY = "/data/logs"`.

---

## Coding conventions for this plan

- Match the codebase's existing style: `LogAttrs(ctx, slog.LevelX, msg, slog.<Type>(...))` for emitting, errors suffixed `Error`, sentinels `Err*`, comments end with a period.
- All new SQL: none required.
- Tests use the standard `testing` package; no third-party assertion libs.
- Don't add `clockwork` — use the `Clock` interface introduced in Task 1.
- Commit messages follow `area: short imperative summary` (mirror recent history: `feat(fixtures): …`, `fix: …`, `docs: …`).

---

### Task 1: Package skeleton — Clock interface

**Files:**
- Create: `internal/errorrecorder/doc.go`
- Create: `internal/errorrecorder/clock.go`
- Test: `internal/errorrecorder/clock_test.go`

- [ ] **Step 1.1: Write the failing test**

Create `internal/errorrecorder/clock_test.go`:

```go
package errorrecorder

import (
	"testing"
	"time"
)

func TestFakeClock_AdvanceMovesNowForward(t *testing.T) {
	start := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	clk := newFakeClock(start)
	if got := clk.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}
	clk.Advance(5 * time.Minute)
	want := start.Add(5 * time.Minute)
	if got := clk.Now(); !got.Equal(want) {
		t.Fatalf("after Advance, Now() = %v, want %v", got, want)
	}
}

func TestRealClock_NowReturnsMonotonicTime(t *testing.T) {
	clk := realClock{}
	t1 := clk.Now()
	t2 := clk.Now()
	if t2.Before(t1) {
		t.Fatalf("realClock.Now() went backwards: %v then %v", t1, t2)
	}
}
```

- [ ] **Step 1.2: Run the test, verify it fails**

```
go test ./internal/errorrecorder/...
```

Expected: build failure (`undefined: newFakeClock`, `undefined: realClock`).

- [ ] **Step 1.3: Add the package doc file**

Create `internal/errorrecorder/doc.go`:

```go
// Package errorrecorder buffers recent slog records keyed by session_hash
// (or trace_id) and, when an Error-level record is observed, dumps the
// matching session's records to a per-occurrence file under a configured
// logs directory. The on-disk format mirrors the wrapped slog.Handler so
// dump files are directly comparable with the live log stream.
package errorrecorder
```

- [ ] **Step 1.4: Implement the clock**

Create `internal/errorrecorder/clock.go`:

```go
package errorrecorder

import (
	"sync"
	"time"
)

// Clock returns the current time. Real production code uses realClock; tests
// inject fakeClock to drive deterministic time.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// fakeClock is a manually-advanced clock for tests. Goroutine-safe.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
```

- [ ] **Step 1.5: Run the test, verify it passes**

```
go test ./internal/errorrecorder/...
```

Expected: PASS.

- [ ] **Step 1.6: Commit**

```bash
git add internal/errorrecorder/doc.go internal/errorrecorder/clock.go internal/errorrecorder/clock_test.go
git commit -m "feat(errorrecorder): add Clock interface and fakeClock"
```

---

### Task 2: Service skeleton with key resolution

**Files:**
- Create: `internal/errorrecorder/service.go`
- Test: `internal/errorrecorder/service_test.go`

This task introduces the `Service` struct with key resolution, `record(key, rec)` to append to a per-key ring, and `snapshot(key)` for tests. No eviction or rate limit yet.

- [ ] **Step 2.1: Write the failing test**

Create `internal/errorrecorder/service_test.go`:

```go
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
```

- [ ] **Step 2.2: Run the test, verify it fails**

```
go test ./internal/errorrecorder/...
```

Expected: build failure (`undefined: resolveKey`, `undefined: Service`, etc.).

- [ ] **Step 2.3: Implement Service skeleton and resolveKey**

Create `internal/errorrecorder/service.go`:

```go
package errorrecorder

import (
	"log/slog"
	"sync"
	"time"
)

// sessionKeyAttrSessionHash is the record-attr key the recorder scans for
// first when grouping a record into a buffer.
const sessionKeyAttrSessionHash = "session_hash"

// sessionKeyAttrTraceID is the fallback record-attr key used when no
// session_hash is present.
const sessionKeyAttrTraceID = "trace_id"

// bufferedRecord holds a cloned slog.Record alongside the time it was added,
// so the pruner can drop entries older than the configured window.
type bufferedRecord struct {
	record  slog.Record
	addedAt time.Time
}

// sessionBuffer is a fixed-size ring of bufferedRecord for one key.
// The slice has length up to maxPerSession; full is true once it has
// wrapped at least once.
type sessionBuffer struct {
	records  []bufferedRecord
	head     int
	full     bool
	lastSeen time.Time
}

// Service owns the in-memory buffers, rate-limit budget, and on-disk writer
// for the error recorder. A single Service is shared by all clones of the
// Handler produced via WithAttrs / WithGroup.
type Service struct {
	mu            sync.Mutex
	sessions      map[string]*sessionBuffer
	maxPerSession int
	maxSessions   int
	window        time.Duration
	rateLimit     int
	clock         Clock
}

// resolveKey returns the grouping key for rec, or "" if neither
// session_hash nor trace_id is present in its attrs.
func resolveKey(rec slog.Record) string {
	var sessionHash, traceID string
	rec.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case sessionKeyAttrSessionHash:
			sessionHash = a.Value.String()
		case sessionKeyAttrTraceID:
			traceID = a.Value.String()
		}
		return sessionHash == "" // keep scanning unless we already have the winning key.
	})
	if sessionHash != "" {
		return sessionHash
	}
	return traceID
}

// record appends rec to the session buffer for key. Caller must NOT hold s.mu.
// The record is cloned so subsequent slog handling cannot mutate the buffered
// copy.
func (s *Service) record(key string, rec slog.Record) {
	cloned := rec.Clone()
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	buf, ok := s.sessions[key]
	if !ok {
		buf = &sessionBuffer{records: make([]bufferedRecord, 0, s.maxPerSession)}
		s.sessions[key] = buf
	}
	buf.lastSeen = now
	if len(buf.records) < s.maxPerSession {
		buf.records = append(buf.records, bufferedRecord{record: cloned, addedAt: now})
		return
	}
	// Ring is full — overwrite at head.
	buf.records[buf.head] = bufferedRecord{record: cloned, addedAt: now}
	buf.head = (buf.head + 1) % s.maxPerSession
	buf.full = true
}

// snapshot returns the buffered records for key in chronological order.
// Returns an empty slice if key is unknown. Caller must NOT hold s.mu.
func (s *Service) snapshot(key string) []bufferedRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf, ok := s.sessions[key]
	if !ok {
		return nil
	}
	if !buf.full {
		out := make([]bufferedRecord, len(buf.records))
		copy(out, buf.records)
		return out
	}
	out := make([]bufferedRecord, 0, len(buf.records))
	out = append(out, buf.records[buf.head:]...)
	out = append(out, buf.records[:buf.head]...)
	return out
}
```

- [ ] **Step 2.4: Run the test, verify it passes**

```
go test ./internal/errorrecorder/...
```

Expected: PASS.

- [ ] **Step 2.5: Commit**

```bash
git add internal/errorrecorder/service.go internal/errorrecorder/service_test.go
git commit -m "feat(errorrecorder): add Service skeleton with key resolution and ring buffer"
```

---

### Task 3: Ring overflow + age eviction

**Files:**
- Modify: `internal/errorrecorder/service.go`
- Modify: `internal/errorrecorder/service_test.go`

- [ ] **Step 3.1: Add the failing tests**

Append to `internal/errorrecorder/service_test.go`:

```go
func TestService_RingOverflow_DropsOldest(t *testing.T) {
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	s := newServiceForTest(t, clk, serviceTestParams{maxPerSession: 3})

	for i := 0; i < 5; i++ {
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
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	s := newServiceForTest(t, clk, serviceTestParams{window: 5 * time.Minute})

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
```

Add the imports `fmt` and `reflect` to the test file if not already present.

- [ ] **Step 3.2: Run the tests, verify the new ones fail**

```
go test ./internal/errorrecorder/... -run 'TestService_RingOverflow|TestService_PruneOnce'
```

Expected: `TestService_RingOverflow_DropsOldest` PASS (already handled by Task 2's implementation), `TestService_PruneOnce_DropsExpiredSessions` FAIL (`undefined: s.pruneOnce`).

- [ ] **Step 3.3: Implement pruneOnce**

Append to `internal/errorrecorder/service.go`:

```go
// pruneOnce drops sessions whose lastSeen is older than now-window. Safe to
// call concurrently with record/snapshot. Exposed package-private so tests
// can drive pruning deterministically without waiting on the ticker.
func (s *Service) pruneOnce() {
	cutoff := s.clock.Now().Add(-s.window)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, buf := range s.sessions {
		if buf.lastSeen.Before(cutoff) {
			delete(s.sessions, key)
		}
	}
}
```

- [ ] **Step 3.4: Run the tests, verify they pass**

```
go test ./internal/errorrecorder/...
```

Expected: PASS.

- [ ] **Step 3.5: Commit**

```bash
git add internal/errorrecorder/service.go internal/errorrecorder/service_test.go
git commit -m "feat(errorrecorder): age-based pruning of stale session buffers"
```

---

### Task 4: LRU eviction when session map is full

**Files:**
- Modify: `internal/errorrecorder/service.go`
- Modify: `internal/errorrecorder/service_test.go`

- [ ] **Step 4.1: Add the failing test**

Append to `internal/errorrecorder/service_test.go`:

```go
func TestService_LRUEviction_DropsLeastRecentlySeen(t *testing.T) {
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	s := newServiceForTest(t, clk, serviceTestParams{maxSessions: 2})

	s.record("a", makeRecord(slog.LevelInfo, "a-record"))
	clk.Advance(1 * time.Second)
	s.record("b", makeRecord(slog.LevelInfo, "b-record"))
	clk.Advance(1 * time.Second)
	// Refresh "a" so "b" becomes the LRU.
	s.record("a", makeRecord(slog.LevelInfo, "a-record-2"))
	clk.Advance(1 * time.Second)
	// Adding "c" should evict "b".
	s.record("c", makeRecord(slog.LevelInfo, "c-record"))

	if got := len(s.snapshot("b")); got != 0 {
		t.Errorf("expected 'b' evicted, got %d records", got)
	}
	if got := len(s.snapshot("a")); got != 2 {
		t.Errorf("expected 'a' to survive with 2 records, got %d", got)
	}
	if got := len(s.snapshot("c")); got != 1 {
		t.Errorf("expected 'c' to have 1 record, got %d", got)
	}
}
```

- [ ] **Step 4.2: Run the test, verify it fails**

```
go test ./internal/errorrecorder/... -run TestService_LRUEviction
```

Expected: FAIL — the map grows past `maxSessions=2` because no eviction code exists yet, so all three sessions are present and the assertion on `b` will fail.

- [ ] **Step 4.3: Add LRU eviction inside record**

Replace the existing `record` method in `internal/errorrecorder/service.go` with:

```go
func (s *Service) record(key string, rec slog.Record) {
	cloned := rec.Clone()
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	buf, ok := s.sessions[key]
	if !ok {
		// New session — evict LRU if we are at the cap.
		if len(s.sessions) >= s.maxSessions {
			s.evictLRULocked()
		}
		buf = &sessionBuffer{records: make([]bufferedRecord, 0, s.maxPerSession)}
		s.sessions[key] = buf
	}
	buf.lastSeen = now
	if len(buf.records) < s.maxPerSession {
		buf.records = append(buf.records, bufferedRecord{record: cloned, addedAt: now})
		return
	}
	buf.records[buf.head] = bufferedRecord{record: cloned, addedAt: now}
	buf.head = (buf.head + 1) % s.maxPerSession
	buf.full = true
}

// evictLRULocked drops the session with the oldest lastSeen. Caller MUST hold s.mu.
func (s *Service) evictLRULocked() {
	var oldestKey string
	var oldestSeen time.Time
	first := true
	for k, b := range s.sessions {
		if first || b.lastSeen.Before(oldestSeen) {
			oldestKey = k
			oldestSeen = b.lastSeen
			first = false
		}
	}
	if oldestKey != "" {
		delete(s.sessions, oldestKey)
	}
}
```

- [ ] **Step 4.4: Run the test, verify it passes**

```
go test ./internal/errorrecorder/...
```

Expected: PASS.

- [ ] **Step 4.5: Commit**

```bash
git add internal/errorrecorder/service.go internal/errorrecorder/service_test.go
git commit -m "feat(errorrecorder): evict least-recently-seen session when cap hit"
```

---

### Task 5: Global rate limit

**Files:**
- Modify: `internal/errorrecorder/service.go`
- Modify: `internal/errorrecorder/service_test.go`

This task introduces `tryReserveDump(now)`, which atomically checks the per-hour budget and either reserves a slot (returns true) or refuses (returns false).

- [ ] **Step 5.1: Add the failing test**

Append to `internal/errorrecorder/service_test.go`:

```go
func TestService_TryReserveDump_AllowsUpToLimit(t *testing.T) {
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	s := newServiceForTest(t, clk, serviceTestParams{rateLimit: 3})

	for i := 0; i < 3; i++ {
		if !s.tryReserveDump() {
			t.Fatalf("reservation %d should have succeeded", i)
		}
		clk.Advance(1 * time.Second)
	}
	if s.tryReserveDump() {
		t.Fatalf("reservation #4 within the window should have been refused")
	}
}

func TestService_TryReserveDump_AllowsAgainAfterWindow(t *testing.T) {
	clk := newFakeClock(time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC))
	s := newServiceForTest(t, clk, serviceTestParams{rateLimit: 1})

	if !s.tryReserveDump() {
		t.Fatalf("first reservation should have succeeded")
	}
	if s.tryReserveDump() {
		t.Fatalf("second reservation within the window should have been refused")
	}
	clk.Advance(61 * time.Minute)
	if !s.tryReserveDump() {
		t.Fatalf("reservation after window expiry should have succeeded")
	}
}
```

- [ ] **Step 5.2: Run the tests, verify they fail**

```
go test ./internal/errorrecorder/... -run TestService_TryReserveDump
```

Expected: build failure (`undefined: s.tryReserveDump`).

- [ ] **Step 5.3: Implement tryReserveDump**

Append to `internal/errorrecorder/service.go`:

```go
// rateLimitWindow is the time over which rateLimit caps the number of dumps.
const rateLimitWindow = 1 * time.Hour

// dumpTimes is the per-Service field; declare it on the struct.
```

Add the `dumpTimes []time.Time` field to the `Service` struct (insert between `clock` and the closing brace):

```go
type Service struct {
	mu            sync.Mutex
	sessions      map[string]*sessionBuffer
	dumpTimes     []time.Time
	maxPerSession int
	maxSessions   int
	window        time.Duration
	rateLimit     int
	clock         Clock
}
```

And append the implementation:

```go
// tryReserveDump returns true iff a dump may proceed under the global
// rate limit. Side effect on success: records the dump time in the
// sliding window.
func (s *Service) tryReserveDump() bool {
	now := s.clock.Now()
	cutoff := now.Add(-rateLimitWindow)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Drop entries that fell out of the sliding window.
	kept := s.dumpTimes[:0]
	for _, t := range s.dumpTimes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	s.dumpTimes = kept
	if len(s.dumpTimes) >= s.rateLimit {
		return false
	}
	s.dumpTimes = append(s.dumpTimes, now)
	return true
}
```

- [ ] **Step 5.4: Run the tests, verify they pass**

```
go test ./internal/errorrecorder/...
```

Expected: PASS.

- [ ] **Step 5.5: Commit**

```bash
git add internal/errorrecorder/service.go internal/errorrecorder/service_test.go
git commit -m "feat(errorrecorder): global per-hour dump rate limit"
```

---

### Task 6: File writer (path layout, JSONL replay, collision retry)

**Files:**
- Create: `internal/errorrecorder/writer.go`
- Create: `internal/errorrecorder/writer_test.go`

- [ ] **Step 6.1: Write the failing test**

Create `internal/errorrecorder/writer_test.go`:

```go
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
	dir := t.TempDir()
	ts := time.Date(2026, 5, 27, 14, 32, 17, 0, time.UTC)
	records := []bufferedRecord{
		{record: makeRecord(slog.LevelInfo, "first", slog.String("k", "v1")), addedAt: ts},
		{record: makeRecord(slog.LevelError, "boom", slog.String("k", "v2")), addedAt: ts},
	}

	path, err := writeDump(dir, "3f9a12bcde", ts, records, &slog.HandlerOptions{Level: slog.LevelDebug})
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
	dir := t.TempDir()
	ts := time.Date(2026, 5, 27, 14, 32, 17, 0, time.UTC)
	records := []bufferedRecord{{record: makeRecord(slog.LevelError, "x"), addedAt: ts}}

	first, err := writeDump(dir, "abcdef1234", ts, records, &slog.HandlerOptions{Level: slog.LevelDebug})
	if err != nil {
		t.Fatalf("first writeDump: %v", err)
	}
	second, err := writeDump(dir, "abcdef1234", ts, records, &slog.HandlerOptions{Level: slog.LevelDebug})
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
```

- [ ] **Step 6.2: Run the test, verify it fails**

```
go test ./internal/errorrecorder/... -run TestWriteDump
```

Expected: build failure (`undefined: writeDump`).

- [ ] **Step 6.3: Implement the writer**

Create `internal/errorrecorder/writer.go`:

```go
package errorrecorder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// maxCollisionRetries bounds how many "-N" suffixes we try when the
// same-second filename already exists. Beyond this we surface the error.
const maxCollisionRetries = 9

// writeDump serialises records to <root>/<YYYY>/<MM>/<DD>/<ts>-<key8>.jsonl
// using a fresh slog.JSONHandler so the on-disk format matches the live
// stdout stream. ts is formatted as UTC YYYYMMDDTHHMMSSZ. key8 is the first
// 8 chars of key (or fewer if key is shorter). Returns the final file path.
func writeDump(
	root string,
	key string,
	ts time.Time,
	records []bufferedRecord,
	opts *slog.HandlerOptions,
) (string, error) {
	if len(records) == 0 {
		return "", errors.New("writeDump: no records")
	}
	ts = ts.UTC()
	dayDir := filepath.Join(root,
		fmt.Sprintf("%04d", ts.Year()),
		fmt.Sprintf("%02d", ts.Month()),
		fmt.Sprintf("%02d", ts.Day()),
	)
	if err := os.MkdirAll(dayDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	key8 := key
	if len(key8) > 8 {
		key8 = key8[:8]
	}
	base := fmt.Sprintf("%s-%s", ts.Format("20060102T150405Z"), key8)

	for attempt := 0; attempt <= maxCollisionRetries; attempt++ {
		name := base + ".jsonl"
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d.jsonl", base, attempt)
		}
		path := filepath.Join(dayDir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", fmt.Errorf("open: %w", err)
		}
		writeErr := replayRecords(f, records, opts)
		closeErr := f.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", fmt.Errorf("close: %w", closeErr)
		}
		return path, nil
	}
	return "", fmt.Errorf("dump filename collision retries exhausted under %s", dayDir)
}

// replayRecords pipes each bufferedRecord through a fresh slog.JSONHandler
// writing into w. The handler is configured with opts so dump output matches
// the live JSONHandler's encoding.
func replayRecords(w *os.File, records []bufferedRecord, opts *slog.HandlerOptions) error {
	h := slog.NewJSONHandler(w, opts)
	ctx := context.Background()
	for _, br := range records {
		if err := h.Handle(ctx, br.record); err != nil {
			return fmt.Errorf("handle: %w", err)
		}
	}
	return nil
}
```

- [ ] **Step 6.4: Run the test, verify it passes**

```
go test ./internal/errorrecorder/...
```

Expected: PASS.

- [ ] **Step 6.5: Commit**

```bash
git add internal/errorrecorder/writer.go internal/errorrecorder/writer_test.go
git commit -m "feat(errorrecorder): JSONL dump writer with collision retry"
```

---

### Task 7: Handler decorator (Handle / WithAttrs / WithGroup)

**Files:**
- Create: `internal/errorrecorder/handler.go`
- Create: `internal/errorrecorder/handler_test.go`

The Handler is the public `slog.Handler` decorator. It:
- Forwards every `Handle()` call to its inner handler first (stdout must not be affected by the recorder).
- Calls `service.observe(rec)` for buffering.
- Preserves attrs across `WithAttrs`/`WithGroup` by wrapping the inner handler, keeping the same `*Service` pointer.

This task does NOT yet trigger dumps. Dump triggering and the worker goroutine come in Task 8.

- [ ] **Step 7.1: Write the failing test**

Create `internal/errorrecorder/handler_test.go`:

```go
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
	svc := newServiceForTest(t, clk, serviceTestParams{})
	svc.inner = inner
	return &Handler{service: svc, inner: inner}
}

func TestHandler_ForwardsToInnerAndBuffersRecord(t *testing.T) {
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
```

- [ ] **Step 7.2: Run the test, verify it fails**

```
go test ./internal/errorrecorder/... -run TestHandler_
```

Expected: build failure (`undefined: Handler`, `h.service.inner`).

- [ ] **Step 7.3: Implement the Handler**

Add the `inner slog.Handler` field to the `Service` struct in `internal/errorrecorder/service.go` (insert below `clock`):

```go
type Service struct {
	mu            sync.Mutex
	sessions      map[string]*sessionBuffer
	dumpTimes     []time.Time
	maxPerSession int
	maxSessions   int
	window        time.Duration
	rateLimit     int
	clock         Clock
	inner         slog.Handler // for the recorder's own log lines and the wrapped chain.
}
```

Now add an `observe` method to the Service in `internal/errorrecorder/service.go`:

```go
// observe is the per-record entry point called by the Handler decorator.
// It is responsible for buffering (and, in a later task, dump triggering).
// Caller has already forwarded the record to the inner handler.
func (s *Service) observe(rec slog.Record) {
	key := resolveKey(rec)
	if key == "" {
		return
	}
	s.record(key, rec)
}
```

Create `internal/errorrecorder/handler.go`:

```go
package errorrecorder

import (
	"context"
	"fmt"
	"log/slog"
)

// Handler is the slog.Handler decorator that feeds records into a Service's
// buffers. It is constructed via Service.Handler(); callers do not
// instantiate it directly.
type Handler struct {
	service *Service
	inner   slog.Handler
}

// Enabled defers to the inner handler.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle forwards rec to the inner handler first (stdout must not be affected
// by the recorder), then feeds the record into the per-session buffer.
func (h *Handler) Handle(ctx context.Context, rec slog.Record) error {
	if err := h.inner.Handle(ctx, rec); err != nil {
		return fmt.Errorf("inner handle: %w", err)
	}
	h.service.observe(rec)
	return nil
}

// WithAttrs returns a Handler whose inner is the inner handler's
// WithAttrs result. The Service pointer is preserved so all clones share
// the same buffer map.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{service: h.service, inner: h.inner.WithAttrs(attrs)}
}

// WithGroup returns a Handler whose inner is the inner handler's
// WithGroup result. The Service pointer is preserved.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{service: h.service, inner: h.inner.WithGroup(name)}
}
```

- [ ] **Step 7.4: Run the test, verify it passes**

```
go test ./internal/errorrecorder/...
```

Expected: PASS.

- [ ] **Step 7.5: Commit**

```bash
git add internal/errorrecorder/handler.go internal/errorrecorder/handler_test.go internal/errorrecorder/service.go
git commit -m "feat(errorrecorder): slog.Handler decorator that buffers records"
```

---

### Task 8: Worker, lifecycle, dump triggering

**Files:**
- Modify: `internal/errorrecorder/service.go`
- Modify: `internal/errorrecorder/service_test.go`

This is the largest task. It adds the public `New(Config)` constructor, the worker goroutine, dump-triggering via `observe`, graceful shutdown via `Close`, and a test helper `waitForDumps(n)` so tests can synchronise without sleeping.

- [ ] **Step 8.1: Add the failing tests**

Append to `internal/errorrecorder/service_test.go`:

```go
func TestService_ErrorRecordProducesDumpFile(t *testing.T) {
	dir := t.TempDir()
	clk := newFakeClock(time.Date(2026, 5, 27, 14, 32, 17, 0, time.UTC))
	var sink bytes.Buffer
	svc, err := New(Config{
		Inner:         slog.NewJSONHandler(&sink, nil),
		LogsDirectory: dir,
		Window:        10 * time.Minute,
		RateLimit:     60,
		Clock:         clk,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	logger := slog.New(svc.Handler())
	logger.LogAttrs(context.Background(), slog.LevelInfo, "step1",
		slog.String("session_hash", "abcdef1234"))
	logger.LogAttrs(context.Background(), slog.LevelError, "boom",
		slog.String("session_hash", "abcdef1234"))

	if err = svc.waitForDumps(1, 2*time.Second); err != nil {
		t.Fatalf("waitForDumps: %v", err)
	}

	files := listJSONLFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 dump file, got %d: %v", len(files), files)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, `"msg":"step1"`) || !strings.Contains(got, `"msg":"boom"`) {
		t.Errorf("dump file missing expected records: %q", got)
	}
}

func TestService_ErrorWithoutKey_NoDump(t *testing.T) {
	dir := t.TempDir()
	clk := newFakeClock(time.Date(2026, 5, 27, 14, 32, 17, 0, time.UTC))
	svc, err := New(Config{
		Inner:         slog.NewJSONHandler(io.Discard, nil),
		LogsDirectory: dir,
		Window:        10 * time.Minute,
		RateLimit:     60,
		Clock:         clk,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	logger := slog.New(svc.Handler())
	logger.LogAttrs(context.Background(), slog.LevelError, "stray error")

	// Give the worker a chance to do something. waitForDumps blocks
	// only on outstanding jobs, so when none are queued it returns
	// quickly.
	if err = svc.waitForDumps(0, 200*time.Millisecond); err != nil {
		t.Fatalf("waitForDumps: %v", err)
	}
	files := listJSONLFiles(t, dir)
	if len(files) != 0 {
		t.Fatalf("expected 0 dump files, got %d: %v", len(files), files)
	}
}

func TestService_RateLimit_DropsExcessDumps(t *testing.T) {
	dir := t.TempDir()
	clk := newFakeClock(time.Date(2026, 5, 27, 14, 32, 17, 0, time.UTC))
	svc, err := New(Config{
		Inner:         slog.NewJSONHandler(io.Discard, nil),
		LogsDirectory: dir,
		Window:        10 * time.Minute,
		RateLimit:     1,
		Clock:         clk,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	logger := slog.New(svc.Handler())
	for i := 0; i < 3; i++ {
		logger.LogAttrs(context.Background(), slog.LevelError, fmt.Sprintf("err-%d", i),
			slog.String("session_hash", "sess1"))
		clk.Advance(1 * time.Second)
	}

	if err = svc.waitForDumps(1, 2*time.Second); err != nil {
		t.Fatalf("waitForDumps: %v", err)
	}
	files := listJSONLFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("expected exactly 1 dump file under the rate limit, got %d", len(files))
	}
}

// listJSONLFiles returns all .jsonl files under root, sorted.
func listJSONLFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".jsonl") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(out)
	return out
}
```

Add the necessary test-file imports near the existing ones:

```go
import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)
```

- [ ] **Step 8.2: Run the tests, verify they fail**

```
go test ./internal/errorrecorder/... -run TestService_Error
```

Expected: build failure (`undefined: New`, `Config`, `Handler()`, `Close`, `waitForDumps`).

- [ ] **Step 8.3: Implement Config, New, dump pipeline, worker, lifecycle**

Replace the entire `internal/errorrecorder/service.go` with this expanded version. Preserve the existing `resolveKey`, `bufferedRecord`, `sessionBuffer`, `record`, `snapshot`, `pruneOnce`, `evictLRULocked`, `tryReserveDump` definitions; add Config, New, Handler() accessor, observe() with trigger, worker, Close. (Below is the full file for clarity.)

```go
package errorrecorder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	sessionKeyAttrSessionHash = "session_hash"
	sessionKeyAttrTraceID     = "trace_id"
	rateLimitWindow           = 1 * time.Hour
	defaultMaxPerSession      = 500
	defaultMaxSessions        = 1000
	defaultJobsBuffer         = 8
	prunerInterval            = 1 * time.Minute
	shutdownGrace             = 5 * time.Second
)

// Config configures a Service. Inner and LogsDirectory are required when
// the recorder is enabled (LogsDirectory != ""). The remaining fields
// default to sensible production values.
type Config struct {
	// Inner is the wrapped slog.Handler. Records are forwarded here first;
	// the recorder also emits its own log lines through this handler to
	// avoid feedback loops.
	Inner slog.Handler
	// LogsDirectory is the root under which YYYY/MM/DD/<file>.jsonl dump
	// files are written. Empty disables the recorder entirely.
	LogsDirectory string
	// Window is the lookback window for buffered records. Records older
	// than now-Window are pruned.
	Window time.Duration
	// RateLimit is the maximum number of dumps the recorder will produce
	// in a rolling one-hour window across all sessions.
	RateLimit int
	// HandlerOptions configures the JSONHandler used to format dump
	// files. Defaults to {Level: slog.LevelDebug}.
	HandlerOptions *slog.HandlerOptions
	// Clock is the time source. Defaults to realClock.
	Clock Clock
}

// bufferedRecord, sessionBuffer, Service definitions follow (see Task 2 / 5).

type bufferedRecord struct {
	record  slog.Record
	addedAt time.Time
}

type sessionBuffer struct {
	records  []bufferedRecord
	head     int
	full     bool
	lastSeen time.Time
}

type dumpJob struct {
	key     string
	records []bufferedRecord
	ts      time.Time
}

type Service struct {
	mu             sync.Mutex
	sessions       map[string]*sessionBuffer
	dumpTimes      []time.Time
	maxPerSession  int
	maxSessions    int
	window         time.Duration
	rateLimit      int
	clock          Clock
	inner          slog.Handler
	logsDirectory  string
	handlerOptions *slog.HandlerOptions
	jobs           chan dumpJob
	stop           chan struct{}
	wg             sync.WaitGroup
	// pendingJobs counts queued + in-flight dumps. waitForDumps tracks
	// completion via this counter so tests don't need to sleep.
	pendingJobsMu   sync.Mutex
	pendingJobs     int
	completedDumps  int
	completedSignal chan struct{}
	disabled        bool
}

// New constructs a Service. When cfg.LogsDirectory == "", New returns a
// disabled Service whose Handler() returns cfg.Inner unchanged and whose
// Close is a no-op. Otherwise New spawns the pruner and worker goroutines.
func New(cfg Config) (*Service, error) {
	if cfg.Inner == nil {
		return nil, errors.New("errorrecorder: Config.Inner is required")
	}
	if cfg.LogsDirectory == "" {
		return &Service{
			inner:    cfg.Inner,
			disabled: true,
		}, nil
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	if cfg.Window <= 0 {
		return nil, errors.New("errorrecorder: Config.Window must be positive")
	}
	if cfg.RateLimit <= 0 {
		return nil, errors.New("errorrecorder: Config.RateLimit must be positive")
	}
	if cfg.HandlerOptions == nil {
		cfg.HandlerOptions = &slog.HandlerOptions{Level: slog.LevelDebug}
	}

	s := &Service{
		sessions:        map[string]*sessionBuffer{},
		maxPerSession:   defaultMaxPerSession,
		maxSessions:     defaultMaxSessions,
		window:          cfg.Window,
		rateLimit:       cfg.RateLimit,
		clock:           cfg.Clock,
		inner:           cfg.Inner,
		logsDirectory:   cfg.LogsDirectory,
		handlerOptions:  cfg.HandlerOptions,
		jobs:            make(chan dumpJob, defaultJobsBuffer),
		stop:            make(chan struct{}),
		completedSignal: make(chan struct{}, 1),
	}

	s.wg.Add(2)
	go s.runPruner()
	go s.runWorker()
	s.emitInfo("error recorder started",
		slog.String("logs_directory", cfg.LogsDirectory),
		slog.Duration("window", cfg.Window),
		slog.Int("rate_limit", cfg.RateLimit),
	)
	return s, nil
}

// Handler returns the slog.Handler to install. When the recorder is
// disabled, this returns the inner handler unchanged.
func (s *Service) Handler() slog.Handler {
	if s.disabled {
		return s.inner
	}
	return &Handler{service: s, inner: s.inner}
}

// Close stops the pruner and worker goroutines and drains queued dumps
// up to a fixed grace period. Safe to call on a disabled Service.
func (s *Service) Close() error {
	if s.disabled {
		return nil
	}
	close(s.stop)
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownGrace):
	}
	s.emitInfo("error recorder stopped")
	return nil
}

// observe is called by the Handler after the inner.Handle returned. It
// buffers the record and, if it is an error, schedules a dump.
func (s *Service) observe(rec slog.Record) {
	key := resolveKey(rec)
	if key == "" {
		return
	}
	s.record(key, rec)
	if rec.Level < slog.LevelError {
		return
	}
	if !s.tryReserveDump() {
		s.emitWarn("error recorder rate limit exceeded",
			slog.String("session_key_prefix", keyPrefix(key)))
		return
	}
	snap := s.snapshot(key)
	if len(snap) == 0 {
		return
	}
	s.pendingJobsMu.Lock()
	s.pendingJobs++
	s.pendingJobsMu.Unlock()
	job := dumpJob{key: key, records: snap, ts: s.clock.Now()}
	select {
	case s.jobs <- job:
	default:
		s.pendingJobsMu.Lock()
		s.pendingJobs--
		s.pendingJobsMu.Unlock()
		s.emitWarn("error recorder dropped dump",
			slog.String("session_key_prefix", keyPrefix(key)))
	}
}

func (s *Service) runPruner() {
	defer s.wg.Done()
	ticker := time.NewTicker(prunerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.pruneOnce()
		}
	}
}

func (s *Service) runWorker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.stop:
			// Drain any queued jobs before exiting.
			for {
				select {
				case job := <-s.jobs:
					s.handleDumpSafely(job)
				default:
					return
				}
			}
		case job := <-s.jobs:
			s.handleDumpSafely(job)
		}
	}
}

func (s *Service) handleDumpSafely(job dumpJob) {
	defer func() {
		if r := recover(); r != nil {
			s.emitWarn("error recorder worker panic",
				slog.Any("recovered", r))
		}
		s.pendingJobsMu.Lock()
		s.pendingJobs--
		s.completedDumps++
		s.pendingJobsMu.Unlock()
		select {
		case s.completedSignal <- struct{}{}:
		default:
		}
	}()

	path, err := writeDump(s.logsDirectory, job.key, job.ts, job.records, s.handlerOptions)
	if err != nil {
		s.emitWarn("error recorder write failed",
			slog.String("session_key_prefix", keyPrefix(job.key)),
			slog.Any("error", err))
		return
	}
	s.emitInfo("captured error context",
		slog.String("file", path),
		slog.Int("records", len(job.records)),
		slog.String("session_key_prefix", keyPrefix(job.key)),
	)
}

// emitInfo / emitWarn route the recorder's own log lines through the
// inner handler, bypassing this Service's Handler so they cannot
// reenter observe().
func (s *Service) emitInfo(msg string, attrs ...slog.Attr) {
	s.emit(slog.LevelInfo, msg, attrs...)
}

func (s *Service) emitWarn(msg string, attrs ...slog.Attr) {
	s.emit(slog.LevelWarn, msg, attrs...)
}

func (s *Service) emit(level slog.Level, msg string, attrs ...slog.Attr) {
	rec := slog.NewRecord(s.clock.Now(), level, msg, 0)
	rec.AddAttrs(attrs...)
	_ = s.inner.Handle(context.Background(), rec)
}

// keyPrefix returns the first 8 chars of key (or fewer if shorter). Used
// in operator log lines so the recorder's own output never carries the
// full session key.
func keyPrefix(key string) string {
	if len(key) > 8 {
		return key[:8]
	}
	return key
}

// waitForDumps blocks until completedDumps reaches n or timeout elapses.
// Test-only helper. Returns an error if the timeout expires before n
// dumps have completed.
func (s *Service) waitForDumps(n int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		s.pendingJobsMu.Lock()
		if s.completedDumps >= n && s.pendingJobs == 0 {
			s.pendingJobsMu.Unlock()
			return nil
		}
		s.pendingJobsMu.Unlock()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("waitForDumps: timed out after %s; completed=%d pending=%d",
				timeout, s.completedDumps, s.pendingJobs)
		}
		select {
		case <-s.completedSignal:
		case <-time.After(remaining):
		}
	}
}

// resolveKey, record, snapshot, pruneOnce, evictLRULocked, tryReserveDump
// retain their definitions from earlier tasks below.

// (Paste resolveKey from Task 2, record / snapshot from Task 2 / 4,
// pruneOnce from Task 3, evictLRULocked from Task 4, tryReserveDump from
// Task 5 here without modification.)
```

When pasting the helpers preserved from earlier tasks, keep their bodies byte-identical to what landed in Tasks 2–5. They are reproduced explicitly for clarity:

```go
func resolveKey(rec slog.Record) string {
	var sessionHash, traceID string
	rec.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case sessionKeyAttrSessionHash:
			sessionHash = a.Value.String()
		case sessionKeyAttrTraceID:
			traceID = a.Value.String()
		}
		return sessionHash == ""
	})
	if sessionHash != "" {
		return sessionHash
	}
	return traceID
}

func (s *Service) record(key string, rec slog.Record) {
	cloned := rec.Clone()
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	buf, ok := s.sessions[key]
	if !ok {
		if len(s.sessions) >= s.maxSessions {
			s.evictLRULocked()
		}
		buf = &sessionBuffer{records: make([]bufferedRecord, 0, s.maxPerSession)}
		s.sessions[key] = buf
	}
	buf.lastSeen = now
	if len(buf.records) < s.maxPerSession {
		buf.records = append(buf.records, bufferedRecord{record: cloned, addedAt: now})
		return
	}
	buf.records[buf.head] = bufferedRecord{record: cloned, addedAt: now}
	buf.head = (buf.head + 1) % s.maxPerSession
	buf.full = true
}

func (s *Service) snapshot(key string) []bufferedRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	buf, ok := s.sessions[key]
	if !ok {
		return nil
	}
	if !buf.full {
		out := make([]bufferedRecord, len(buf.records))
		copy(out, buf.records)
		return out
	}
	out := make([]bufferedRecord, 0, len(buf.records))
	out = append(out, buf.records[buf.head:]...)
	out = append(out, buf.records[:buf.head]...)
	return out
}

func (s *Service) pruneOnce() {
	cutoff := s.clock.Now().Add(-s.window)
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, buf := range s.sessions {
		if buf.lastSeen.Before(cutoff) {
			delete(s.sessions, key)
		}
	}
}

func (s *Service) evictLRULocked() {
	var oldestKey string
	var oldestSeen time.Time
	first := true
	for k, b := range s.sessions {
		if first || b.lastSeen.Before(oldestSeen) {
			oldestKey = k
			oldestSeen = b.lastSeen
			first = false
		}
	}
	if oldestKey != "" {
		delete(s.sessions, oldestKey)
	}
}

func (s *Service) tryReserveDump() bool {
	now := s.clock.Now()
	cutoff := now.Add(-rateLimitWindow)
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.dumpTimes[:0]
	for _, t := range s.dumpTimes {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	s.dumpTimes = kept
	if len(s.dumpTimes) >= s.rateLimit {
		return false
	}
	s.dumpTimes = append(s.dumpTimes, now)
	return true
}
```

Also: `newServiceForTest` in `service_test.go` constructs a `*Service` with only the test-relevant fields. It should now also initialise `inner: slog.NewJSONHandler(io.Discard, nil)` so existing tests still pass after the field was added. Update it accordingly:

```go
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
		inner:         slog.NewJSONHandler(io.Discard, nil),
	}
}
```

- [ ] **Step 8.4: Run the full package test suite, verify it passes**

```
go test ./internal/errorrecorder/...
```

Expected: PASS. The race detector should be clean too:

```
go test --race ./internal/errorrecorder/...
```

- [ ] **Step 8.5: Commit**

```bash
git add internal/errorrecorder/service.go internal/errorrecorder/service_test.go
git commit -m "feat(errorrecorder): worker, dump triggering, lifecycle"
```

---

### Task 9: Disabled-mode short-circuit test

**Files:**
- Modify: `internal/errorrecorder/handler_test.go`

Defence-in-depth test: when `LogsDirectory == ""`, `Handler()` must return the raw inner handler and no goroutines should be running.

- [ ] **Step 9.1: Add the test**

Append to `internal/errorrecorder/handler_test.go`:

```go
func TestNew_DisabledWhenLogsDirectoryEmpty(t *testing.T) {
	inner := slog.NewJSONHandler(io.Discard, nil)
	svc, err := New(Config{
		Inner: inner,
		// LogsDirectory intentionally empty.
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })

	if got := svc.Handler(); got != inner {
		t.Errorf("Handler() = %T, want exactly the inner handler", got)
	}
	if !svc.disabled {
		t.Errorf("disabled flag = false, want true")
	}
}
```

Add `"io"` to the test file's import block if missing.

- [ ] **Step 9.2: Run the tests, verify they pass**

```
go test ./internal/errorrecorder/...
```

Expected: PASS (the implementation in Task 8 already handles this path).

- [ ] **Step 9.3: Commit**

```bash
git add internal/errorrecorder/handler_test.go
git commit -m "test(errorrecorder): cover disabled-mode short-circuit"
```

---

### Task 10: NewTraceID helper in internal/logging

**Files:**
- Modify: `internal/logging/contexthandler.go`
- Create: `internal/logging/contexthandler_test.go` (or extend existing test if present)

- [ ] **Step 10.1: Check whether a test file already exists**

```
ls internal/logging/
```

If `contexthandler_test.go` exists, extend it. Otherwise create it.

- [ ] **Step 10.2: Write the failing test**

Append (or create) `internal/logging/contexthandler_test.go`:

```go
package logging

import "testing"

func TestNewTraceID_GeneratesDistinctValues(t *testing.T) {
	a := NewTraceID()
	b := NewTraceID()
	if a == "" || b == "" {
		t.Fatalf("got empty trace IDs: %q, %q", a, b)
	}
	if a == b {
		t.Fatalf("expected distinct trace IDs, got %q twice", a)
	}
}
```

- [ ] **Step 10.3: Run the test, verify it fails**

```
go test ./internal/logging/...
```

Expected: build failure (`undefined: NewTraceID`).

- [ ] **Step 10.4: Add the helper**

Append to `internal/logging/contexthandler.go`:

```go
import "crypto/rand"
```

(Add to the existing import block; do not duplicate.)

```go
// NewTraceID returns a fresh URL-safe random trace ID. Matches the same
// generator used by the existing HTTP middleware so format consistency
// holds across HTTP and background-job contexts.
func NewTraceID() string {
	return rand.Text()
}
```

- [ ] **Step 10.5: Run the test, verify it passes**

```
go test ./internal/logging/...
```

Expected: PASS.

- [ ] **Step 10.6: Commit**

```bash
git add internal/logging/contexthandler.go internal/logging/contexthandler_test.go
git commit -m "feat(logging): expose NewTraceID helper for trace_id generation"
```

---

### Task 11: Switch logAndTraceRequest to logging.NewTraceID

**Files:**
- Modify: `cmd/web/middleware.go`

This is a no-functional-change refactor; the helper centralises trace_id generation so it can be reused in background jobs (next tasks).

- [ ] **Step 11.1: Apply the edit**

In `cmd/web/middleware.go`, around line 132, replace:

```go
		traceID := rand.Text()
```

with:

```go
		traceID := logging.NewTraceID()
```

The `logging` import is already present in this file. Remove the `crypto/rand` import only if it is no longer used elsewhere in this file. (`cspNonce := rand.Text()` and `Value: rand.Text()` still use it, so the import stays.)

- [ ] **Step 11.2: Run the cmd/web tests, verify nothing regresses**

```
go test ./cmd/web/...
```

Expected: PASS.

- [ ] **Step 11.3: Commit**

```bash
git add cmd/web/middleware.go
git commit -m "refactor(cmd/web): use logging.NewTraceID for request trace IDs"
```

---

### Task 12: trace_id per scheduler fire

**Files:**
- Modify: `internal/notification/scheduler.go`

The scheduler's `fire` method runs in a fresh goroutine when a push is due. It logs at Warn on dispatch failure. Today those log lines have no trace_id — so an error-triggered dump for that session would lack the lead-up. Inject a fresh trace_id into the request context so it appears on every log line emitted under that fire.

- [ ] **Step 12.1: Apply the edit**

In `internal/notification/scheduler.go`, change the imports to add `logging`:

```go
import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/logging"
)
```

Modify the `fire` method so the ctx carries a fresh trace_id from the start. Leave the inline `slog.Int("user_id", push.UserID)` attrs on the existing `LogAttrs` calls alone — they continue to work whether or not the production chain has ContextHandler installed, and scheduler unit tests build a bare logger.

```go
func (s *Scheduler) fire(selfBox **time.Timer, key slotKey, push domain.ScheduledPush) {
	s.mu.Lock()
	self := *selfBox
	if current, ok := s.timers[key]; ok && current == self {
		delete(s.timers, key)
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) //nolint:mnd // see above.
	defer cancel()
	ctx = logging.WithAttrs(ctx, slog.String("trace_id", logging.NewTraceID()))

	if err := s.cfg.Dispatch(ctx, push); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "push dispatch failed",
			slog.Int("user_id", push.UserID),
			slog.String("workout_date", key.date),
			slog.Int("position", key.pos),
			slog.Any("error", err))
	}
	if err := s.cfg.Repo.Delete(ctx, push.ID); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "delete scheduled push row after fire",
			slog.Int("user_id", push.UserID),
			slog.String("workout_date", key.date),
			slog.Int("position", key.pos),
			slog.Int("id", push.ID),
			slog.Any("error", err))
	}
}
```

- [ ] **Step 12.2: Run the scheduler tests**

```
go test ./internal/notification/...
```

Expected: PASS. If any test asserts on log output structure and breaks on the moved `user_id`, update the assertion to read from the enriched record.

- [ ] **Step 12.3: Commit**

```bash
git add internal/notification/scheduler.go
git commit -m "feat(notification): attach trace_id to scheduler fire context"
```

---

### Task 13: trace_id per idle-monitor tick

**Files:**
- Modify: `internal/notification/idle_monitor.go`

- [ ] **Step 13.1: Apply the edit**

In `internal/notification/idle_monitor.go`, change the imports:

```go
import (
	"context"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/logging"
)
```

Replace the `Run` method:

```go
func (m *IdleMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tickCtx := logging.WithAttrs(ctx,
				slog.String("trace_id", logging.NewTraceID()),
				slog.String("component", "idle_monitor"),
			)
			if m.shouldTrigger() {
				m.cfg.Logger.LogAttrs(tickCtx, slog.LevelInfo, "idle monitor triggering shutdown")
				m.cfg.Trigger()
				return
			}
		}
	}
}
```

- [ ] **Step 13.2: Run the notification tests**

```
go test ./internal/notification/...
```

Expected: PASS.

- [ ] **Step 13.3: Commit**

```bash
git add internal/notification/idle_monitor.go
git commit -m "feat(notification): attach trace_id to idle-monitor tick context"
```

---

### Task 14: Wire recorder into cmd/web/main.go

**Files:**
- Modify: `cmd/web/main.go`

This task adds the `PETRAPP_LOGS_DIRECTORY` config field, constructs the recorder in `main()` (where the slog chain is built), inserts it inside `ContextHandler`, and defers `Close()`.

- [ ] **Step 14.1: Add config field**

In `cmd/web/main.go`, add to the `config` struct alongside `TracesDirectory`:

```go
	// LogsDirectory is the path to the root directory under which the
	// error recorder writes per-occurrence dump files. Empty disables the
	// recorder.
	LogsDirectory string `env:"PETRAPP_LOGS_DIRECTORY" envDefault:""`
```

- [ ] **Step 14.2: Replace `main()` so it constructs the recorder**

Replace the existing `main()` function:

```go
func main() {
	ctx := context.Background()
	handlerOptions := &slog.HandlerOptions{
		AddSource:   false,
		Level:       slog.LevelDebug,
		ReplaceAttr: nil,
	}
	var baseHandler slog.Handler
	baseHandler = slog.NewTextHandler(os.Stdout, handlerOptions)
	if os.Getenv("FLY_MACHINE_ID") != "" {
		baseHandler = slog.NewJSONHandler(os.Stdout, handlerOptions)
	}

	logsDirectory := os.Getenv("PETRAPP_LOGS_DIRECTORY")
	recorder, err := errorrecorder.New(errorrecorder.Config{
		Inner:          baseHandler,
		LogsDirectory:  logsDirectory,
		Window:         10 * time.Minute,
		RateLimit:      60,
		HandlerOptions: handlerOptions,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "build error recorder: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := recorder.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "close error recorder: %v\n", closeErr)
		}
	}()

	loggerHandler := logging.NewContextHandler(recorder.Handler())
	appName := os.Getenv("FLY_APP_NAME")
	if appName == "" {
		appName = "petra-local"
	}
	logger := slog.New(loggerHandler).With(slog.String("service_name", appName))
	if runErr := run(ctx, logger, os.LookupEnv); runErr != nil {
		logger.LogAttrs(ctx, slog.LevelError, "failure starting application", slog.Any("error", runErr))
		os.Exit(1)
	}
}
```

Add the new import:

```go
	"github.com/myrjola/petrapp/internal/errorrecorder"
```

The `config.LogsDirectory` field is consulted only by the e2etest plumbing (Task 15) and by anyone reading `cfg`. `main()` reads the env var directly via `os.Getenv` rather than going through `envstruct.Populate`, which mirrors how `FLY_MACHINE_ID` and `FLY_APP_NAME` are handled in the existing code.

- [ ] **Step 14.3: Build and run the existing tests**

```
go build ./...
go test ./cmd/web/...
```

Expected: PASS. The existing logger plumbing is intact; the recorder is disabled by default (empty `LogsDirectory`).

- [ ] **Step 14.4: Commit**

```bash
git add cmd/web/main.go
git commit -m "feat(cmd/web): wire error recorder into slog chain"
```

---

### Task 15: Wire recorder into e2etest

**Files:**
- Modify: `internal/e2etest/server.go`

So integration tests can opt in by setting `PETRAPP_LOGS_DIRECTORY` via their `lookupEnv` and assert dump files appear under that directory.

- [ ] **Step 15.1: Apply the edit**

In `internal/e2etest/server.go`, modify `StartServer` to wrap the inner handler with the recorder when the env var is present. Replace the handler construction block:

```go
	innerHandler := slog.NewTextHandler(logSink, &slog.HandlerOptions{
		AddSource: false,
		Level:     slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == LogAddrKey {
				addrCh <- a.Value.String()
			}
			if a.Key == LogDsnKey {
				dsnCh <- a.Value.String()
			}
			return a
		},
	})

	logsDir, _ := lookupEnv("PETRAPP_LOGS_DIRECTORY")
	recorder, err := errorrecorder.New(errorrecorder.Config{
		Inner:         innerHandler,
		LogsDirectory: logsDir,
		Window:        10 * time.Minute,
		RateLimit:     60,
	})
	if err != nil {
		return nil, fmt.Errorf("build error recorder: %w", err)
	}
	t.Cleanup(func() { _ = recorder.Close() })

	logger := slog.New(logging.NewContextHandler(recorder.Handler()))
```

Add the `errorrecorder` import:

```go
import (
	// ...existing...
	"github.com/myrjola/petrapp/internal/errorrecorder"
)
```

- [ ] **Step 15.2: Run all tests**

```
go test ./...
```

Expected: PASS. Disabled mode (`logsDir == ""`) keeps the chain identical to today, so all existing e2etest-backed tests are unaffected.

- [ ] **Step 15.3: Commit**

```bash
git add internal/e2etest/server.go
git commit -m "feat(e2etest): plumb PETRAPP_LOGS_DIRECTORY into error recorder"
```

---

### Task 16: Wiring test — error triggers a dump file

**Files:**
- Create: `cmd/web/main_test.go`
- Modify: `internal/errorrecorder/service.go` (export WaitForDumps)

This test exercises the full slog chain — `ContextHandler` → `recorder.Handler()` → JSON — exactly as `main()` constructs it, then asserts that an Error-level log line produces a dump file under a temp directory. It doesn't drive an HTTP request because today's e2etest setup has no unconditionally-500-producing route; spinning up admin/auth fixtures just to trigger `serverError` would dwarf the test it serves. Wiring-level coverage at the chain composition is the right grain.

- [ ] **Step 16.1: Add the test**

Create `cmd/web/main_test.go`:

```go
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
	dir := t.TempDir()
	var stdout bytes.Buffer

	recorder, err := errorrecorder.New(errorrecorder.Config{
		Inner:         slog.NewJSONHandler(&stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
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
```

Note: this test calls `recorder.WaitForDumps`. The package-private `waitForDumps` from Task 8 is package-internal. Export it as `WaitForDumps` in this task — production code won't call it but having an exported helper is acceptable for a recorder service that explicitly supports test synchronisation. Update `internal/errorrecorder/service.go` Task 8 method:

```go
// WaitForDumps blocks until completedDumps reaches n or timeout elapses.
// Intended for test synchronisation; safe to call from production code
// but has no purpose there.
func (s *Service) WaitForDumps(n int, timeout time.Duration) error {
	return s.waitForDumps(n, timeout)
}
```

(Keep the lowercase `waitForDumps` for in-package tests; the exported wrapper just delegates.)

- [ ] **Step 16.2: Run the new test**

```
go test ./cmd/web/... -run TestRecorderProducesDumpFileOnError
```

Expected: build failure first (`WaitForDumps undefined`); after adding the exported method, PASS.

- [ ] **Step 16.3: Run the full test suite**

```
go test ./...
```

Expected: PASS.

- [ ] **Step 16.4: Commit**

```bash
git add cmd/web/main_test.go internal/errorrecorder/service.go
git commit -m "test(cmd/web): integration test for error context dump files"
```

---

### Task 17: Enable in production via fly.toml

**Files:**
- Modify: `fly.toml`

- [ ] **Step 17.1: Apply the edit**

In `fly.toml`, under `[env]`, add the new variable next to the existing `PETRAPP_TRACES_DIRECTORY`:

```toml
[env]
PETRAPP_SQLITE_URL = "/data/petrapp.sqlite3"
PETRAPP_TRACES_DIRECTORY = "/data/traces"
PETRAPP_LOGS_DIRECTORY = "/data/logs"
LITESTREAM_REPLICA_TYPE = "s3"
```

- [ ] **Step 17.2: Run lint + tests one more time**

```
make ci
```

Expected: PASS.

- [ ] **Step 17.3: Commit**

```bash
git add fly.toml
git commit -m "feat(fly): enable error recorder with /data/logs"
```

---

## Self-Review

**Spec coverage:**
- High-level architecture (§Architecture): Task 7 (Handler), Task 8 (Service), Task 14 (main wiring). ✓
- Buffer & key model — key resolution, per-key state, service state, bounds (§Buffer): Tasks 2–4. ✓
- Trigger & dump pipeline — hot path, worker, feedback loop, lifecycle (§Trigger): Task 8. ✓
- File layout & format (§File layout): Task 6. ✓
- Trace IDs in background jobs (§Trace IDs): Tasks 10, 12, 13. ✓
- Testing — unit + integration (§Testing): Tasks 1–9 unit, Task 16 integration. ✓
- Operational concerns — disk, permissions, failure modes, observability, privacy (§Operational): Task 6 perms, Task 8 failure modes + self-logging, Task 17 enable. ✓
- Out-of-scope items (§Out of scope): not implemented, by design. ✓

**Placeholder scan:** no TBDs; every code step shows full code; commands explicit; expected outputs stated.

**Type consistency:**
- `Service`, `Handler`, `Config`, `bufferedRecord`, `sessionBuffer` names used identically across Tasks 2, 4, 5, 7, 8.
- `record / snapshot / pruneOnce / evictLRULocked / tryReserveDump / observe / Handler() / WaitForDumps / Close` method names consistent.
- `resolveKey` function consistent.
- `writeDump` signature matches between Task 6 (definition) and Task 8 (call site).

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-27-error-context-logs.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
