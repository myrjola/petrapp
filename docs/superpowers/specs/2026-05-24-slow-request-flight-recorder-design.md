# Slow-request flight-recorder trigger

## Problem

`internal/flightrecorder` already captures a `runtime/trace` snapshot when a
request returns 503 (the `http.TimeoutHandler` deadline fired). That covers
the worst case — total stalls — but misses the much more common
user-impacting case: a response that *did* complete, but took long enough
the user felt it. Today we have no way to reconstruct what blocked those
requests after the fact.

The flight recorder buffer (5 min, 64 MB) is already running in production;
we just need a second trigger.

## Goal

When an HTTP request crosses 500 ms (Web Vitals INP "poor" threshold)
without timing out, dump the flight-recorder buffer to disk so a
post-mortem can identify the blocking goroutine, syscall, or lock.

## Non-goals

- Per-route SLOs. One threshold for everything not on `/admin/*`.
- Live alerting / paging. Disk dumps are inspected manually.
- Per-request structured trace metadata. The existing `runtime/trace.Task`
  in `logAndTraceRequest` already tags requests; the dump inherits that.

## Design

### Service change — `internal/flightrecorder/service.go`

Add a sibling capture method:

```go
// CaptureSlowRequestTrace writes a trace snapshot when a request crossed
// the user-noticeable-delay threshold. Shares cooldown with
// CaptureTimeoutTrace so a slow request that escalates into a 503 does
// not produce two near-identical dumps.
func (s *Service) CaptureSlowRequestTrace(ctx context.Context, duration time.Duration)
```

Implementation differs from `CaptureTimeoutTrace` only in:

- Filename prefix: `slow-<UTC-timestamp>.trace`.
- Extra log attribute: `slog.Duration("duration", duration)` at
  `slog.LevelWarn`.

To avoid duplicating the cooldown / file open / `WriteTo` / close /
log dance, extract a private helper:

```go
func (s *Service) captureTrace(ctx context.Context, prefix string, extraAttrs ...slog.Attr)
```

Both `CaptureTimeoutTrace` and `CaptureSlowRequestTrace` become two-line
wrappers around it. The `lastCapture` atomic / 30-minute cooldown stays
on the `Service` and is shared across both kinds.

### Middleware change — `cmd/web/middleware.go`

In `logAndTraceRequest`, immediately after the existing 503 branch (which
keeps its current behaviour), add:

```go
const slowRequestThreshold = 500 * time.Millisecond

if app.flightRecorder != nil &&
    sw.statusCode != http.StatusServiceUnavailable &&
    duration >= slowRequestThreshold &&
    !strings.HasPrefix(path, "/admin/") {
    go app.flightRecorder.CaptureSlowRequestTrace(flightRecorderCtx, duration)
}
```

`duration` is `time.Since(start)`, already computed for the completion
log line just above. The dispatch uses the existing
`context.WithoutCancel(ctx)` so the background goroutine survives the
client connection closing.

### Why the admin exemption uses a path prefix, not `IsAdmin`

`logAndTraceRequest` sits at the outer edge of the middleware stack —
outside `mustAdminStack`. The `IsAdmin` context value is set on a child
`*http.Request` that downstream middleware builds with
`r.WithContext(...)`; that new request never propagates back to the
outer middleware's `r` variable, so calling `contexthelpers.IsAdmin(r.Context())`
here returns `false` even for admin routes.

`r.URL.Path` is the stable signal at this layer. Every admin route is
mounted under `/admin/...` via `mustAdminStack` in `routes.go`, so a
`strings.HasPrefix(path, "/admin/")` check is the source of truth at this
boundary.

### Why cooldown is shared, not per-kind

A request that crosses 500 ms and then escalates into a 503 would
otherwise produce two dumps in quick succession that share almost the
same 5-minute flight-recorder window. The marginal information from the
second dump is near zero; the operator cost (extra file to download,
extra log line) is non-zero. Collapsing them is correct.

### Why no per-route threshold

Static-file routes and the health-check endpoint were considered for
exemption. Decision: keep them in. A 500 ms `/static/main.css` serve from
the local fs on a Fly machine is itself a bug worth investigating;
likewise a slow `/api/healthy`. The cooldown caps the noise either way.

## Testing

Extend `internal/flightrecorder/service_test.go`:

- `TestService_CaptureSlowRequestTrace` — call
  `CaptureSlowRequestTrace(ctx, 750*time.Millisecond)` on a started
  service and assert the resulting directory entry has prefix `slow-`
  and suffix `.trace`.
- `TestService_CooldownIsSharedAcrossCaptureKinds` — call
  `CaptureTimeoutTrace` and then `CaptureSlowRequestTrace` (or
  vice-versa); assert only one file lands in the directory.

No new middleware test: `app.flightRecorder` is nil in the e2e harness
(no `PETRAPP_TRACES_DIRECTORY` env var), and exercising the new branch
end-to-end would require a parallel scaffold for marginal value. The
new middleware logic is a 4-line guarded `go` call whose risk is
dominated by the service implementation, which is covered.

## Rollout

No config flag. The flight recorder itself is already gated by
`PETRAPP_TRACES_DIRECTORY` being set; the new trigger inherits that
gate. Production already has the directory configured, so this lights
up on the next deploy.

## Out of scope (followups, if the trigger proves useful)

- Surfacing trace files via an admin UI for download.
- Auto-uploading dumps to object storage.
- Tightening / loosening the 500 ms constant after a few weeks of
  observation.
