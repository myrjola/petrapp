# Slow-request flight-recorder trigger — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Capture a `runtime/trace.FlightRecorder` snapshot when an HTTP request crosses 500 ms without timing out, so post-mortems can analyse user-noticeable but non-timeout slowness.

**Architecture:** Refactor `flightrecorder.Service` to share a private `captureTrace` helper between the existing `CaptureTimeoutTrace` and a new `CaptureSlowRequestTrace`. Wire the new method into `cmd/web/middleware.go`'s `logAndTraceRequest` so it fires after the response completes — gated by a 500 ms threshold, the existing nil-check on `app.flightRecorder`, and a `/admin/` path-prefix exclusion. The 30-minute cooldown atomic on the service is shared across both kinds.

**Tech Stack:** Go 1.26, `runtime/trace`, `log/slog`, stdlib `net/http` middleware.

**Working directory:** `/home/martin/petrapp/.worktrees/slow-request-flightrecorder` on branch `slow-request-flightrecorder`.

**Spec:** `docs/superpowers/specs/2026-05-24-slow-request-flight-recorder-design.md`.

---

### Task 1: Add `CaptureSlowRequestTrace` to flightrecorder service

**Files:**
- Modify: `internal/flightrecorder/service.go`
- Modify: `internal/flightrecorder/service_test.go`

#### Step 1: Write the failing tests

Append to `internal/flightrecorder/service_test.go`:

```go
//nolint:paralleltest // runtime/trace.NewFlightRecorder is a process-global singleton.
func TestService_CaptureSlowRequestTrace(t *testing.T) {
	traceDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	service, err := flightrecorder.New(flightrecorder.Config{
		Logger:          logger,
		MinAge:          0,
		MaxBytes:        0,
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

	service.CaptureSlowRequestTrace(ctx, 750*time.Millisecond)

	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatalf("failed to read trace directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one trace file, got %d", len(entries))
	}
	filename := entries[0].Name()
	if !strings.HasPrefix(filename, "slow-") {
		t.Errorf("expected filename to start with 'slow-', got %s", filename)
	}
	if !strings.HasSuffix(filename, ".trace") {
		t.Errorf("expected filename to end with '.trace', got %s", filename)
	}
}

//nolint:paralleltest // runtime/trace.NewFlightRecorder is a process-global singleton.
func TestService_CooldownIsSharedAcrossCaptureKinds(t *testing.T) {
	traceDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	service, err := flightrecorder.New(flightrecorder.Config{
		Logger:          logger,
		MinAge:          0,
		MaxBytes:        0,
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

	// First capture fires; second should be blocked by the shared cooldown
	// even though it is a different capture kind.
	service.CaptureTimeoutTrace(ctx)
	service.CaptureSlowRequestTrace(ctx, 600*time.Millisecond)

	entries, err := os.ReadDir(traceDir)
	if err != nil {
		t.Fatalf("failed to read trace directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected cooldown to prevent the second capture, got %d files", len(entries))
	}
}
```

The existing `time` import already exists; double-check `strings` is also imported (it is — used by `TestService_CaptureTimeoutTrace`).

- [ ] **Step 2: Run the new tests to confirm they fail to compile**

Run: `go test -race ./internal/flightrecorder/...`
Expected: build failure with `undefined: (*flightrecorder.Service).CaptureSlowRequestTrace`.

- [ ] **Step 3: Refactor `service.go` to extract a shared helper, then add the new method**

Replace the body of `CaptureTimeoutTrace` and add the new method + helper. Final shape of the relevant section in `internal/flightrecorder/service.go`:

```go
// CaptureTimeoutTrace captures a trace when a request times out.
// It respects the cooldown period to avoid overwhelming the filesystem.
func (s *Service) CaptureTimeoutTrace(ctx context.Context) {
	s.captureTrace(ctx, "timeout")
}

// CaptureSlowRequestTrace captures a trace when a request completes but
// crossed the user-noticeable-delay threshold. Shares the cooldown with
// CaptureTimeoutTrace so a slow request that escalates into a 503 does
// not produce two near-identical dumps.
func (s *Service) CaptureSlowRequestTrace(ctx context.Context, duration time.Duration) {
	s.captureTrace(ctx, "slow", slog.Duration("duration", duration))
}

// captureTrace is the shared implementation behind CaptureTimeoutTrace and
// CaptureSlowRequestTrace. prefix becomes the trace filename prefix and a
// log attribute identifying which trigger fired. extraAttrs are appended
// to the success log line so callers can surface trigger-specific context.
func (s *Service) captureTrace(ctx context.Context, prefix string, extraAttrs ...slog.Attr) {
	// Check cooldown period.
	now := time.Now().Unix()
	lastCapture := s.lastCapture.Load()

	if lastCapture > 0 && time.Unix(now, 0).Sub(time.Unix(lastCapture, 0)) < cooldownDuration {
		s.logger.LogAttrs(ctx, slog.LevelDebug, "skipping trace capture due to cooldown",
			slog.String("trigger", prefix),
			slog.Time("last_capture", time.Unix(lastCapture, 0)),
			slog.Duration("remaining_cooldown", cooldownDuration-time.Unix(now, 0).Sub(time.Unix(lastCapture, 0))))
		return
	}

	// Update last capture time atomically.
	if !s.lastCapture.CompareAndSwap(lastCapture, now) {
		// Another goroutine updated the timestamp, respect that.
		return
	}

	// Generate filename with timestamp and trigger prefix.
	timestamp := time.Unix(now, 0).UTC().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.trace", prefix, timestamp)
	fPath := filepath.Join(s.tracesDirectory, filename)

	// Create and write the trace file.
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

	// Write the flight recorder trace to file.
	bytesWritten, err := s.flightRecorder.WriteTo(file)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelError, "failed to write trace",
			slog.String("file", fPath),
			slog.Any("error", err))
		return
	}

	attrs := []slog.Attr{
		slog.String("trigger", prefix),
		slog.String("file", fPath),
		slog.Int64("bytes", bytesWritten),
	}
	attrs = append(attrs, extraAttrs...)
	s.logger.LogAttrs(ctx, slog.LevelWarn, "captured trace", attrs...)
}
```

Notes:
- The `Service` struct, `Config`, `New`, `Start`, `Stop`, and the constants stay exactly as they are. Only the body of `CaptureTimeoutTrace` shrinks and the two new declarations get added.
- The success log message changed from `"captured timeout trace"` to `"captured trace"` with a `trigger` attribute. This is a small operator-visible rename but keeps log lines unambiguous for both kinds.
- `slog.Attr` is the right variadic type for `LogAttrs`; that is why the helper takes `...slog.Attr` rather than `...any`.

- [ ] **Step 4: Run all flightrecorder tests**

Run: `go test -race ./internal/flightrecorder/...`
Expected: all four existing tests plus the two new ones pass.

- [ ] **Step 5: Run lint on the package**

Run: `make lint-fix` (from worktree root).
Expected: no findings introduced by this change; if `golangci-lint` reports anything, fix inline.

- [ ] **Step 6: Commit**

```bash
git add internal/flightrecorder/service.go internal/flightrecorder/service_test.go
git commit -m "$(cat <<'EOF'
feat(flightrecorder): add CaptureSlowRequestTrace

Share the file-write/cooldown plumbing between CaptureTimeoutTrace and
the new CaptureSlowRequestTrace via a private captureTrace helper.
Files land as slow-<ts>.trace; the existing 30-minute cooldown is
shared across both trigger kinds so a slow request that escalates into
a 503 does not produce two near-identical dumps.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Trigger CaptureSlowRequestTrace from the request middleware

**Files:**
- Modify: `cmd/web/middleware.go` (the `logAndTraceRequest` method and the imports block)

#### Step 1: Add the `strings` import and the trigger logic

In `cmd/web/middleware.go`, ensure `strings` is in the import block (it is not today). The imports become:

```go
import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"runtime/trace"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/logging"
)
```

Just above `logAndTraceRequest`, add the threshold constant:

```go
// slowRequestThreshold is the duration above which a non-admin request is
// considered user-noticeable and triggers a flight recorder dump. 500ms
// matches the Web Vitals INP "poor" threshold.
const slowRequestThreshold = 500 * time.Millisecond
```

Then rewrite the tail of `logAndTraceRequest` (the block that currently logs completion and conditionally calls `CaptureTimeoutTrace`) so it hoists the duration once and dispatches both capture kinds through a single nil-guarded `switch`:

```go
		// Log request completion.
		duration := time.Since(start)
		level := slog.LevelInfo
		if sw.statusCode >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		app.logger.LogAttrs(r.Context(), level, "request completed",
			slog.Int("status_code", sw.statusCode), slog.Duration("duration", duration))

		// Capture a flight recorder dump for timed-out or user-noticeably-slow
		// requests. Admin routes are exempt because their 30s timeout budget
		// covers intentionally slow external calls.
		if app.flightRecorder != nil {
			flightRecorderCtx := context.WithoutCancel(ctx)
			switch {
			case sw.statusCode == http.StatusServiceUnavailable:
				go app.flightRecorder.CaptureTimeoutTrace(flightRecorderCtx)
			case duration >= slowRequestThreshold && !strings.HasPrefix(path, "/admin/"):
				go app.flightRecorder.CaptureSlowRequestTrace(flightRecorderCtx, duration)
			}
		}
```

Notes:
- `path` is already in scope (declared at the top of `logAndTraceRequest` as `path = r.URL.Path`).
- `ctx` is the outer context the middleware constructed with the trace_id / proto / method / uri attrs; `context.WithoutCancel(ctx)` survives the connection closing.
- `app.flightRecorder` is nil in dev and in tests where `PETRAPP_TRACES_DIRECTORY` is unset; the outer nil-check preserves that behaviour.

- [ ] **Step 2: Build to confirm the file compiles**

Run: `go build ./...`
Expected: clean build.

- [ ] **Step 3: Run the cmd/web tests**

Run: `go test -race ./cmd/web/...`
Expected: pass. The middleware change is dormant in tests (flightRecorder is nil) so no test should change behaviour.

- [ ] **Step 4: Lint**

Run: `make lint-fix`
Expected: no findings.

- [ ] **Step 5: Commit**

```bash
git add cmd/web/middleware.go
git commit -m "$(cat <<'EOF'
feat(web): capture flight recorder trace for slow requests

logAndTraceRequest now fires CaptureSlowRequestTrace when a non-admin
request crosses 500ms without timing out. The existing 503 timeout
branch keeps its behaviour; both share the recorder's 30-minute
cooldown. /admin/* routes are exempt because their 30s timeout budget
covers intentionally slow external calls.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Full CI gate

This is the worktreeflow Phase 3 entry — the worktreeflow skill takes over after this passes.

- [ ] **Step 1: Run `make ci`**

Run: `make ci` (from the worktree root).
Expected: init + build + lint-fix + test + sec all clean. If lint-fix produces a diff, commit it as `lint: gofmt/golangci-lint --fix` and re-run.

- [ ] **Step 2: Confirm no uncommitted changes remain**

Run: `git status`
Expected: clean working tree on `slow-request-flightrecorder`.

At this point the worktreeflow skill's Phase 3 push (`git push origin HEAD:main`) and cleanup take over.

---

## Self-review

- **Spec coverage.**
  - "Add `CaptureSlowRequestTrace(ctx, duration)` to service" → Task 1, Step 3.
  - "Filename prefix `slow-`" → asserted in Task 1, Step 1 test; produced in Task 1, Step 3 helper.
  - "`slog.Duration("duration", duration)` at warn level" → Task 1, Step 3 (`extraAttrs`).
  - "Private `captureTrace` helper" → Task 1, Step 3.
  - "Shared cooldown" → Task 1, Step 1 second test; Task 1, Step 3 (same `s.lastCapture` atomic for both wrappers).
  - "`slowRequestThreshold = 500 * time.Millisecond` in middleware" → Task 2, Step 1.
  - "`!strings.HasPrefix(path, "/admin/")`" → Task 2, Step 1.
  - "Existing 503 branch keeps behaviour" → Task 2, Step 1 (`switch` first case is the same call).
  - "No new middleware integration test" → Task 2 has only build + existing-tests verification. Matches the spec's testing section.
- **Placeholder scan.** None — every code block is complete, every command has expected output, every commit has its actual message.
- **Type consistency.** `CaptureSlowRequestTrace(ctx context.Context, duration time.Duration)` is used identically in the service file, the test file, and the middleware call. `captureTrace(ctx context.Context, prefix string, extraAttrs ...slog.Attr)` is defined and called consistently.
