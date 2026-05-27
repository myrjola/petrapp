# Error Context Logs (per-session flight recorder for logs)

Date: 2026-05-27

## Goal

When the application logs an error, also write out a separate file containing
the recent log lines from the same user session — the "what was this user
doing that led to the error" view. The existing stdout JSON log stream is
unchanged; the dump file is supplementary context.

This is the same pattern as the Go runtime `trace.FlightRecorder` we already
use in `internal/flightrecorder/` — buffer recent events in memory, write
them out only when something interesting happens. It is also the same idea
as Sentry's "breadcrumbs" or generic "error-triggered log dump".

## Why

`session_hash` is on every log line for an authenticated request
(`internal/webauthnhandler/middleware.go:48`), and `trace_id` is on every
request log line (`cmd/web/middleware.go:135`). Stitching together the lead-
up to a production error today means grepping the JSON logs by either ID and
hoping the relevant window fits in the operator's mental cache. A single
file per error, scoped to the session and the last ~10 minutes, removes that
step.

We do not need log rotation yet. Volume estimates are in §Operational
concerns.

## High-level architecture

A new `internal/errorrecorder` package exposes:

- `Handler` — an `slog.Handler` decorator that wraps another handler.
- `Service` — owns the in-memory buffer, the rate limit, and the on-disk
  writer goroutine.

`cmd/web/main.go` composes the slog chain as (outer first):

```
logging.ContextHandler      (existing — adds ctx attrs to record)
  → errorrecorder.Handler   (new — buffers enriched records, triggers dump)
    → slog.JSONHandler      (stdout — or dev TextHandler)
```

`Handle` calls flow outer-to-inner: `ContextHandler` enriches the
record with ctx-bound attrs (`session_hash`, `trace_id`, `proto`, etc.)
and forwards. The recorder receives the *already-enriched* record,
which is exactly what we want to buffer and replay into dump files —
the dump file then byte-matches the stdout JSON the wrapped
`JSONHandler` produces.

Construction:

```go
recorder, err := errorrecorder.New(errorrecorder.Config{
    Inner:         jsonHandler,        // the stdout encoder; also used for the recorder's own log lines
    LogsDirectory: cfg.LogsDirectory,  // "/data/logs" in prod, "" disables
    Window:        10 * time.Minute,
    RateLimit:     60,                 // dumps/hour, global
    // Clock omitted — defaults to wall-clock; tests inject a fakeClock
})
handler := logging.NewContextHandler(recorder.Handler())
logger := slog.New(handler).With(slog.String("service_name", appName))
```

`recorder.Handler()` returns the decorating `slog.Handler` (returns
`Inner` unchanged when `LogsDirectory == ""`). The recorder also emits
its own log lines through `Inner` directly, avoiding any feedback loop.

The recorder's own log lines bypass `ContextHandler` enrichment by
design — they are a global emission, not scoped to a user request.

`recorder.Handler()` returns a `*Handler` value whose `WithAttrs` /
`WithGroup` implementations clone only the wrapped inner handler — the
pointer to the shared `Service` is preserved across
`slog.Logger.With(...)` calls.

New environment variable `PETRAPP_LOGS_DIRECTORY`, declared in
`cmd/web/main.go` next to `PETRAPP_TRACES_DIRECTORY`. `fly.toml` sets it to
`/data/logs`. Empty disables the recorder entirely — `recorder.Handler()`
returns the inner handler unchanged, so dev / test runs have zero
overhead and no disk writes.

## Buffer & key model

### Key resolution

The recorder sits inside `ContextHandler` in the chain, so by the time
`errorrecorder.Handler.Handle(ctx, record)` is called the record's
`Attrs` already contain whatever `logging.WithAttrs` placed on the
context. Resolution is a single scan over `record.Attrs`:

1. Scan record attrs for `session_hash`. If present, use it.
2. Else scan for `trace_id`. If present, use it.
3. Else — no key — skip buffering. The record still goes through to the
   inner handler (stdout).

`WithAttrs` and `WithGroup` on the decorating handler must preserve any
attrs added via `slog.Logger.With(...)` so they appear in the buffered
record. This is the standard pattern: `WithAttrs` clones the handler
with a wrapped inner, leaving the recorder's pointer-to-Service
unchanged.

### Per-key state

```go
type bufferedRecord struct {
    record  slog.Record // cloned via record.Clone()
    addedAt time.Time
}

type sessionBuffer struct {
    records  []bufferedRecord // ring slice, fixed cap
    head     int
    full     bool
    lastSeen time.Time
}
```

### Service state

```go
type Service struct {
    mu            sync.Mutex
    sessions      map[string]*sessionBuffer
    dumpTimes     []time.Time     // for global rate limit, sliding 1h
    maxSessions   int             // 1000
    maxPerSession int             // 500 records
    window        time.Duration   // 10 min
    rateLimit     int             // 60/hour
    logsDirectory string
    inner         slog.Handler    // for the recorder's own log lines
    clock         Clock           // Now() — see §Testing
    jobs          chan dumpJob    // buffered, cap 8
    workerWG      sync.WaitGroup
    stop          chan struct{}
}
```

### Bounds & pruning

- Per-session ring cap: 500 records. Oldest dropped on overflow.
- Max tracked sessions: 1000. LRU eviction by `lastSeen` when the map
  reaches the cap.
- A background pruner ticks every minute: drops sessions where
  `lastSeen < now - window`, drops `dumpTimes` entries older than an hour.
- Single `sync.Mutex` guards the map and `dumpTimes`. All operations are
  O(1) amortized. Sharding can come later if contention shows up.

## Trigger & dump pipeline

### Hot path (`Handle(ctx, record)`)

Reached only when the recorder is active (when `LogsDirectory == ""`,
`recorder.Handler()` returned the inner handler unchanged so this
decorator is not in the chain). Synchronous in the caller's
goroutine, no I/O:

1. Call `inner.Handle(ctx, record)` first — stdout must not be affected by
   recorder slowness or failure.
2. Resolve key as in §Buffer & key model. If none, return.
3. `record.Clone()`, acquire mutex, append to ring, update `lastSeen`.
4. If `record.Level < slog.LevelError`, release lock and return.
5. Check rate budget: prune `dumpTimes` older than 1h; if `len(dumpTimes) >= 60`,
   log `"error recorder rate limit exceeded"` at Warn through `inner` and
   return. The triggering record stays buffered for the next dump on this
   session.
6. Append `now` to `dumpTimes`. Copy the buffered records into a snapshot
   slice (cheap — records are small structs containing a `slog.Record`).
7. Release lock. Send a `dumpJob{key, snapshot}` on the `jobs` channel
   non-blocking (`select { case jobs <- j: default: log "dropped" }`).

### Worker goroutine

One worker is enough — at 60 dumps/hour the average gap is one minute.

- Reads `dumpJob` from `jobs`.
- Computes path:
  `<LogsDirectory>/<YYYY>/<MM>/<DD>/<ts>-<key8>.jsonl`
  where `ts = now.UTC().Format("20060102T150405Z")` and `key8` is the
  first 8 chars of the key.
- `os.MkdirAll(parent, 0o700)`.
- `os.OpenFile(path, O_CREATE|O_WRONLY|O_EXCL, 0o600)`. On collision
  (same-second), retry with suffix `-1`, `-2`, …, up to 9.
- Constructs a `slog.NewJSONHandler` writing to the open file with the same
  handler options used for stdout (so dump records match stdout format
  byte-for-byte).
- Replays each buffered `slog.Record` through it.
- Closes the file, logs `"captured error context" file=<path> records=<n> key=<masked>`
  at Info through `inner`.
- On write error: log Warn through `inner`, drop the dump, continue.
- `defer recover()` around the body — a worker panic logs through `inner`
  and the worker loop restarts.

### Feedback-loop guard

The recorder's own log lines (`"error recorder started"`, `"captured error
context"`, `"... rate limit exceeded"`, `"... dropped dump"`, `"... write
failed"`) go through the *inner* handler, never through the recorder's own
`Handle`. No goroutine-local flag is needed — the inner handler is held
directly on `Service` and accessed only by the recorder's own code.

### Lifecycle

- `Service.Start(ctx)` spawns the pruner goroutine and the worker
  goroutine. Logs `"error recorder started"` at Info through `inner` with
  the effective config.
- `Service.Stop(ctx)` closes `stop`, waits for the pruner and worker via
  `workerWG`, draining `jobs` with a 5s grace deadline before giving up.
- Wired in `cmd/web/main.go` `run()` next to the flight-recorder
  lifecycle.

## File layout & format

```
/data/logs/
  2026/
    05/
      27/
        20260527T143217Z-3f9a12bc.jsonl
        20260527T144501Z-3f9a12bc.jsonl
        20260527T150812Z-trace-7a2c.jsonl
```

- One file per error occurrence.
- Year / month / day directories make manual cleanup trivial
  (`rm -rf /data/logs/2026/01`).
- File contents: one JSON object per line, identical schema to the stdout
  JSON log stream.
- File permissions `0o600`, directory permissions `0o700` (mirrors
  `internal/flightrecorder/`).

## Trace IDs in background jobs

For the trace_id fallback to give context to background-job errors, those
goroutines need a `trace_id` in their root context. Today only
`logAndTraceRequest` sets one.

Touchpoints:

- `internal/notification/scheduler.go` — wrap each tick: set a fresh
  `trace_id` via `logging.WithAttrs(ctx, slog.String("trace_id", logging.NewTraceID()))`
  at the top of the iteration.
- `internal/notification/idle_monitor.go` — same pattern, per-check.
- `internal/flightrecorder/service.go` — skip. Flight-recorder log lines
  are rare and self-contained.
- `cmd/web/main.go` startup path — skip. Startup failures with no key
  are visible in stdout anyway, and nothing else is running in parallel.

Add a tiny helper in `internal/logging`:

```go
// NewTraceID returns a fresh 16-byte random trace ID, encoded as text.
func NewTraceID() string { return rand.Text() }
```

`logAndTraceRequest` switches to it; the scheduler / monitor loops call it
once per iteration.

## Testing

### Unit tests in `internal/errorrecorder/`

- Buffer eviction by age (fake clock advances past `window`).
- Buffer eviction by per-session cap (push 501 records, oldest dropped).
- LRU eviction when `maxSessions` reached.
- Key resolution priority: `session_hash` > `trace_id`; both missing →
  not buffered.
- Keys added via `logger.With(slog.String("session_hash", ...))` survive
  through `WithAttrs` cloning of the decorating handler.
- Rate limit: 61st dump within an hour is dropped, Warn line emitted to
  inner.
- File format: read back the JSONL, assert each line round-trips and
  matches the buffered record's `time`, `level`, `msg`, and attrs.
- File collision: two dumps in the same second produce `…-1.jsonl`.
- Feedback-loop guard: a Warn line from the recorder itself is not
  re-buffered (assert the inner handler sees it but the ring stays the
  same size).
- `Stop()` drains in-flight jobs within the deadline.
- `LogsDirectory == ""` short-circuits — no goroutines spawned, no
  buffering.

### Integration test in `cmd/web/`

Spin up an `e2etest` server with `PETRAPP_LOGS_DIRECTORY` set to a temp
dir, drive a request that triggers `serverError` (existing
`handler-error_test.go` has a 500-producing path), assert a file appears
under the dated folder containing the request's log lines and the error
record.

### Dependency

The codebase does not currently use `clockwork`. Rather than add a
dependency, introduce a small `internal/errorrecorder/clock.go` with a
minimal `Clock` interface used only for `Now()`:

```go
type Clock interface {
    Now() time.Time
}
```

A `realClock` wraps `time.Now`. A `fakeClock` used in tests holds a
mutex-guarded `time.Time` and exposes `Advance(d time.Duration)`.

The pruner uses `time.NewTicker` directly (its 1-minute cadence is not
exercised by unit tests). Tests that need to assert pruning behavior
call a package-private `service.pruneOnce()` method directly with a
controlled clock instead of waiting on the ticker.

## Operational concerns

### Disk

No rotation, per the requirement. Worst-case at the rate limit:
60 dumps/hour × 500 records × ~500 bytes ≈ 15 MB/hour, ~360 MB/day.
Realistic load will be far lower. Manual cleanup:

```sh
rm -rf /data/logs/2026/01   # drop January
```

A one-liner in `docs/` describing the cleanup command should land when
disk usage becomes a real concern.

### Permissions

Files `0o600`, directories `0o700`. Mirrors flight-recorder behavior.

### Failure modes

- Disk full / write error → Warn through inner, drop dump, continue.
- Worker panic → `defer recover()` logs through inner, worker loop
  restarts.
- Shutdown mid-write → `Stop()` waits up to 5s for the worker to drain
  `jobs`.

### Observability of the recorder itself

Log lines emitted through the inner handler:

- `"error recorder started"` (Info, at boot, with config).
- `"captured error context"` (Info, per successful dump, with file path,
  record count, masked key).
- `"error recorder rate limit exceeded"` (Warn, when the 60/hour budget
  is exhausted).
- `"error recorder dropped dump"` (Warn, when the `jobs` channel is
  full).
- `"error recorder write failed"` (Warn, on file / encoder error).
- `"error recorder stopped"` (Info, on shutdown).

No new metrics endpoint — observability remains log-based.

### Privacy

`session_hash` is already a sha256 of the SCS token; the raw token never
appears in logs. Dump files contain the same content the stdout handler
produces, so no new exposure surface. Existing repo-wide rule
("Never introduce code that exposes or logs secrets and keys",
`CLAUDE.md`) continues to apply.

## Out of scope

- Log rotation / size-based pruning.
- Compression of dump files.
- Cross-session correlation tooling ("all dumps for user X").
- Shipping dumps off-host (Fly log shipper path explicitly rejected for
  current scale).
- Per-environment trigger configuration (single trigger level: Error).
- Per-request flight-recorder hook beyond what already exists.
