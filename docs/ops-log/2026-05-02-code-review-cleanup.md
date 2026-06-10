# Code Review Cleanup — 2026-05-02

Triaged backlog from the code review of 2026-05-02. Each item is self-contained: file, line, the actual fix, and how to verify. Item #4 (LLM URL hallucination) is intentionally excluded — handle separately.

Recommended order: bugs (1–6) first, then mechanical cleanups (7–14), then polish (15–20). Inside each band, items are independent — pick whatever you have an appetite for.

---

## 🔴 Bugs

### 1. `findHistoricalSets` returns the oldest match instead of the most recent

**File:** `internal/workout/service.go` — `findHistoricalSets` (around line 854)

`s.repo.sessions.List(ctx, threeMonthsAgo)` returns sessions `ORDER BY workout_date DESC` (see `internal/workout/repository-sessions.go:44`), so index 0 is the most recent. The current loop walks `len(history)-1 → 0`, which finds the *oldest* historical use of the exercise.

**Fix:** Iterate forward.

```go
for i := 0; i < len(history); i++ {
    session := history[i]
    if session.Date.Equal(date) {
        continue
    }
    for _, exerciseSet := range session.ExerciseSets {
        if exerciseSet.ExerciseID == exerciseID {
            return s.copySetsWithoutCompletion(exerciseSet.Sets), nil
        }
    }
}
```

(Or `for _, session := range history { ... }` — even cleaner.)

**Verify:** Add a unit/integration test that creates two sessions with the exercise (one 8 weeks ago at 60kg, one 1 week ago at 80kg), then calls `SwapExercise` or `AddExercise` on a fresh date and asserts the seeded weight is 80kg, not 60kg.

---

### 2. Flight recorder traces directory created with no write permission

**File:** `internal/flightrecorder/service.go:55`

```go
if err = os.MkdirAll(cfg.TracesDirectory, 0500); err != nil {
```

`0500` (`r-x------`) means even the owner can't write. First timeout capture will fail with permission denied when creating the trace file inside this directory.

**Fix:** Use `0700` (`rwx------`).

**Verify:** Existing test `TestService_CaptureTimeoutTrace` already creates a trace file but uses `t.TempDir()` which inherits perms, so it doesn't catch this. Add a test that points `TracesDirectory` at a non-existent path under `t.TempDir()` and confirms `CaptureTimeoutTrace` actually writes a file.

---

### 3. SQLite read-only DSN has conflicting `mode=` parameters for in-memory databases

**File:** `internal/sqlite/sqlite.go` — `connect`

In-memory branch builds:

```
file:<rand>?mode=ro&_loc=auto&...&mode=memory&cache=shared
```

SQLite's URI parser silently takes the last `mode=`, so `mode=ro` is overridden to `mode=memory`. Functionally the read handle is still read-only because of `_query_only=true`, but the URL is misleading and one PRAGMA away from a real bug.

**Fix:** Build the URL once with no duplicate keys. Suggested shape:

```go
// Decide mode/cache once based on whether the target is in-memory.
var modeAndCache string
if isInMemory {
    url = rand.Text()
    modeAndCache = "vfs=memdb&cache=shared" // or keep mode=memory but only here
}
common := "_loc=auto&_defer_foreign_keys=1&_journal_mode=wal&_busy_timeout=5000&_synchronous=normal&_foreign_keys=on"

// Read-only handle: query_only enforces RO at connection level; no `mode=ro` for in-memory.
roExtras := "_txlock=deferred&_query_only=true"
if !isInMemory {
    roExtras += "&mode=ro"
}
readConfig := fmt.Sprintf("file:%s?%s&%s&%s", url, common, roExtras, modeAndCache)
```

Goal: at most one `mode=` per URL, regardless of branch.

**Verify:** Existing tests already exercise both branches. Add a quick log assertion (or read `db.ReadOnly.Driver()` ping with a write to confirm it's rejected) to lock the behavior.

---

### 5. CSP report endpoint logs the full payload (potential PII)

**File:** `cmd/web/handler-reporting.go` — `reportingAPI`

```go
app.logger.LogAttrs(r.Context(), slog.LevelWarn, "Report received via Reporting API",
    slog.Any("payload", payload), ...)
```

CSP `script-sample` and `source-file` query strings can leak fragments of inline script, URLs with tokens, etc.

**Fix:** Before logging, walk the parsed payload and redact `script-sample`, strip query strings from `source-file`/`document-uri`/`blocked-uri`. Easiest path: define a `redactReport(any) any` that returns a copy with those fields scrubbed, then log the redacted version. Keep the unredacted body inside the request-scoped processing only.

For the array form (Chrome `application/reports+json`), redact `body.sample`, `body.documentURL`, `body.blockedURL`. For the legacy object form, redact `csp-report.script-sample` and the `*-uri` fields.

**Verify:** Update `Test_application_reportingAPI` — add assertion that `logOutput` does **not** contain the script sample / sensitive URL fragment from the test fixtures, and **does** contain the directive name (proving redaction is field-targeted).

---

### 6. Non-admin requests don't get a write deadline

**File:** `cmd/web/middleware.go` — `timeout`

Admin path:

```go
err := rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
```

Non-admin path: nothing. The server-level `WriteTimeout: 2s` is the only safety net, but the per-request contract is "1.8s timeout".

**Fix:** Set the write deadline on both branches. Pull the timeout/deadline pair into a small struct and apply it uniformly:

```go
deadline := time.Now().Add(defaultTimeout)
timeout := defaultTimeout - 200*time.Millisecond
if contexthelpers.IsAdmin(r.Context()) {
    deadline = time.Now().Add(30 * time.Second)
    timeout = 29 * time.Second
}
if err := rc.SetWriteDeadline(deadline); err != nil {
    app.serverError(w, r, err)
    return
}
http.TimeoutHandler(next, timeout, "timed out").ServeHTTP(w, r)
```

**Verify:** Extend `Test_application_timeout` with a non-admin slow-write case and assert the 503 fires (it already does for the regular-user case via `TimeoutHandler`; the new assertion is that `SetWriteDeadline` was called — easiest by checking for the underlying behavior with a rigged `ResponseController`-aware writer).

---

## 🟡 Mechanical cleanups

### 7. Replace `&[]float64{0}[0]` idiom + de-duplicate default sets

**File:** `internal/workout/service.go` — `createEmptySets`, `AddExercise`

Add a small helper near the top of `service.go`:

```go
func float64Ptr(v float64) *float64 { return &v }
```

Replace every `&[]float64{0}[0]` with `float64Ptr(0)`.

While there: `AddExercise` constructs three identical default `Set` literals. Collapse into a loop:

```go
const defaultSetCount = 3
newSets := make([]Set, defaultSetCount)
for i := range newSets {
    newSets[i] = Set{
        WeightKg: float64Ptr(0),
        MinReps:  defaultMinReps,
        MaxReps:  defaultMaxReps,
    }
}
```

**Verify:** `make test ./internal/workout/...` — existing tests cover the affected paths.

---

### 8. De-duplicate the middleware stack between `routes.go` and `handler-fileserver.go`

**Files:**
- `cmd/web/routes.go`
- `cmd/web/handler-fileserver.go`

`fileServerHandler` re-implements `session` and `noAuth` inline. If CSRF, maintenance-mode, or panic recovery changes in `routes()`, you must remember to mirror it here.

**Fix:** Promote the closures to methods on `*application` (or package-level builders that take `*application`):

```go
// In a new cmd/web/middleware-stacks.go (or inside middleware.go).
func (app *application) sharedStack(next http.Handler) http.Handler { ... }
func (app *application) sessionStack(next http.Handler) http.Handler { ... }
func (app *application) noAuthStack(next http.Handler) http.Handler { ... }
// etc.
```

Have both `routes()` and `fileServerHandler` call these. Drop the duplicate closures in `fileServerHandler`.

**Verify:** Existing handler tests (especially `handler-admin-feature-flags_test.go` maintenance-mode cases that exercise both routes and static files) must keep passing.

---

### 9. Stop using `panic("not implemented")` for placeholder template funcs

**File:** `cmd/web/handlers.go` — `baseTemplateFuncs`

Today `pageTemplate` parses with placeholder funcs that panic if called; `render` overrides them with real ones via `t.Funcs(...)`. Any code path that parses a template and *doesn't* override (or any partial that's evaluated outside `render`) crashes the server.

**Fix:** Make placeholders return safe zero values:

```go
"nonce":    func() template.HTMLAttr { return "" },
"mdToHTML": func(_ string) template.HTML { return "" },
```

`formatFloat` is already pure and stays as-is. Document in a comment that real implementations are bound per-request in `contextTemplateFuncs`.

**Verify:** `make test ./cmd/web/...`. Optionally add a regression test that parses a template via `pageTemplate` and executes it directly (no `contextTemplateFuncs`) — should produce empty nonce/mdToHTML rather than panic.

---

### 10. Collapse `parseExerciseSetURLParams` return values

**File:** `cmd/web/handler-exerciseset.go`

```go
date, workoutExerciseID, setIndex, dateStr, err := app.parseExerciseSetURLParams(r)
```

`dateStr` is redundant (`date.Format("2006-01-02")` rebuilds it).

**Fix:** Return a struct, drop `dateStr`:

```go
type exerciseSetParams struct {
    Date              time.Time
    WorkoutExerciseID int
    SetIndex          int
}

func (app *application) parseExerciseSetURLParams(r *http.Request) (exerciseSetParams, error) { ... }
```

Update the two call sites (`exerciseSetUpdatePOST`, `recordSetCompletionWithWeight`).

**Verify:** Compile + existing tests.

---

### 11. Remove the dead `pprofserver.listenAndServe` Shutdown defer

**File:** `internal/pprofserver/pprofserver.go`

The deferred `srv.Shutdown(...)` runs only after `ListenAndServe` already returns — i.e. after the server is closed. It can never do real work.

**Fix:** Drop the defer entirely. If you actually want pprof to participate in graceful shutdown, accept a `context.Context` and run shutdown when it's done:

```go
func listenAndServe(ctx context.Context, addr string) error {
    srv := newServer(addr)
    go func() {
        <-ctx.Done()
        _ = srv.Shutdown(context.Background())
    }()
    err := srv.ListenAndServe()
    if errors.Is(err, http.ErrServerClosed) {
        return nil
    }
    return err
}
```

…and propagate `ctx` from `Launch` (which already takes one).

**Verify:** No tests hit pprofserver today; manual smoke: `PETRAPP_PPROF_ADDR=localhost:6060 ./petrapp` and confirm port frees on Ctrl+C.

---

### 12. `loadExerciseSets` complexity

**File:** `internal/workout/repository-sessions.go`

The combined LEFT-JOIN + grouped iteration is correct but dense. Lower-risk refactor (no behavior change): extract the row→aggregate transition into named helpers, which the file already does partially (`startAggregate`, `buildSet`). The remaining win is eliminating the `current.ID = -1` sentinel.

**Fix (incremental):**
- Replace the `-1` sentinel with `var current *exerciseSetAggregate` (nil = no group started). Initialize on first row.
- Or, split into two queries: one for `workout_exercise` rows, one for `exercise_sets` keyed by `workout_exercise_id`. Combine in Go. Roughly the same total SQL cost (both indexed) and much simpler code.

If you go with two queries, kill `loadExerciseSetsRow`, `startAggregate`, and `buildSet` — they exist only to thread the LEFT JOIN through one cursor.

**Verify:** Existing `Test_application_addWorkout` and `Test_application_exerciseSet_*` cover this code path.

---

### 13. `webauthn.js` — `bufferEncode` blows up on large buffers

**File:** `ui/static/webauthn.js`

```js
return btoa(String.fromCharCode.apply(null, new Uint8Array(value)))
```

`Function.apply` argument-count limit is ~65k on Safari. WebAuthn payloads are well under that today, but this is a future trap.

**Fix:** Build the binary string by iterating, then `btoa`:

```js
function bufferEncode(value) {
    const bytes = new Uint8Array(value)
    let bin = ''
    for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
    return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
}
```

**Verify:** Existing playwright smoketest (`Test_playwright_smoketest`) exercises register/login and is the canonical regression check.

---

### 14. Magic numbers without `mnd` annotations in `flightrecorder/service.go`

**File:** `internal/flightrecorder/service.go`

Bare constants:
- `defaultMinAge = 5 * time.Minute`
- `defaultMaxBytes = 64 * 1024 * 1024`
- `cooldownDuration = 30 * time.Minute`

These already have explanatory names, but the codebase convention is to either name them as untyped const (good — already done) or annotate inline literals. The internal usage (`5*time.Minute`, `64*1024*1024`, etc.) is fine as constants. The actual offenders are inline literals scattered elsewhere — search and apply consistently:

```bash
grep -nE '\b(2|5|10|30|60|64|100|200|256|300|1024|1000)\b' cmd/web/*.go internal/**/*.go \
    | grep -vE '//.*nolint:mnd|const|^\s*\d|test'
```

**Fix:** Either name or annotate per the team rule. Quick wins are inside `cmd/web/middleware.go` (the timeout block), `cmd/web/server.go`, and `cmd/web/main.go`.

**Verify:** `make lint`.

---

## 🟢 Polish

### 15. Form size limit doesn't match schema cap for exercise descriptions

**Files:**
- `cmd/web/handlers.go` — `largeMaxFormSize = 1024 * 10`
- `internal/sqlite/schema.sql` — `description_markdown ... LENGTH(description_markdown) < 20000`

Web edits are capped at 10KB by `MaxBytesReader`, but the schema permits 20KB. Either bump the form limit to ~25KB (margin for other fields + form encoding overhead) or lower the schema check to match. Pick one source of truth.

**Recommended:** Raise `largeMaxFormSize` to 32KB for the admin form. Keep the schema check as a defense-in-depth safety net. Add a test that submits a 15KB description and verifies it round-trips.

---

### 16. `stresstest` reports overall success even when every exercise fails

**File:** `cmd/stresstest/main.go` — `generateSingleWorkout`

Per-exercise failures are logged and `continue`'d, but the function returns `nil` regardless. A run where every exercise fails reports as success.

**Fix:** Track failure count, return an error if more than X% of exercises fail (or simply if any did, depending on intent). Surface it in the run summary.

---

### 17. `exerciseprogression` panics on unknown enum values

**File:** `internal/exerciseprogression/progression.go` — `TargetReps`, `adjustedWeight`

`panic` on unknown `Signal`/`PeriodizationType`. Internal package, so callers shouldn't construct invalid values, but a typo turns into a 500.

**Fix:** Either return a sentinel error from these helpers, or — since they're internal — accept the panic but ensure tests cover every enum value to make the panic statically unreachable. Lower-effort: add an `_ = exhaustive` test that switches over every enum variant and never falls through.

---

### 18. Session lifetime 12h too short for a fitness app

**File:** `cmd/web/main.go` — `initializeSessionManager`

```go
sessionManager.Lifetime = 12 * time.Hour
```

Users authenticated by passkey at 7am get re-prompted mid-workout that evening. If compliance allows, bump to 7d.

**Fix:** Change to `7 * 24 * time.Hour` (annotate `mnd`). Optional: implement sliding-window renewal so active users don't expire.

---

### 19. `userdb.copyTableSchema` uses naive string-replace

**File:** `internal/sqlite/userdb.go`

```go
skipUntilLeftParens := strings.Index(createSQL, "(")
exportSQL := fmt.Sprintf("CREATE TABLE export.%s%s", tableName, createSQL[skipUntilLeftParens:])
```

Works today because the schema doesn't put comments between `CREATE TABLE name` and `(`, but `sqlite_master.sql` doesn't normalize whitespace. One day someone adds a comment and silently produces a malformed export.

**Fix:** Use a regex anchored to the table name:

```go
re := regexp.MustCompile(`(?i)^\s*CREATE\s+TABLE\s+("?` + regexp.QuoteMeta(tableName) + `"?)\b`)
exportSQL := re.ReplaceAllString(createSQL, "CREATE TABLE export."+tableName)
```

(Pre-compile the regex once per call — or memoize per process.)

**Verify:** `TestDatabase_CreateUserDB` already exercises every table type in fixtures; add a fixture table whose `CREATE TABLE` statement has unusual whitespace to lock the behavior.

---

### 20. Admin exercise edit returns 500 for user input errors

**File:** `cmd/web/handler-admin-exercises.go` — `adminExerciseUpdatePOST`

Empty name, invalid category, missing primary muscles all flow into `app.serverError` → 500 + generic error page. These are 400-level user errors.

**Fix:** Use the existing `putFlashError` / form-redirect pattern (see `schedulePOST` in `handler-schedule.go` for a working example):

```go
if name == "" {
    app.putFlashError(r.Context(), "Name is required.")
    redirect(w, r, fmt.Sprintf("/admin/exercises/%d", id))
    return
}
```

Then surface `popFlashError` in `adminExerciseEditGET` and add `{{ if .ValidationError }}<p role="alert">…</p>{{ end }}` to the template (mirror `schedule.gohtml`).

**Verify:** Add a sub-test to `Test_application_adminExercises` that submits the edit form with an empty name and asserts a 200 (or 303→200) response carrying the error message, not a 500.

---

## Quick checklist

- [ ] 1. Walk history forward in `findHistoricalSets`
- [ ] 2. `0500` → `0700` in flightrecorder
- [ ] 3. SQLite read-only DSN — single `mode=` param
- [ ] 5. Redact CSP report payloads before logging
- [ ] 6. Set write deadline for non-admin requests
- [ ] 7. `float64Ptr` helper + collapse default sets
- [ ] 8. Promote middleware stack closures to shared methods
- [ ] 9. Replace placeholder template funcs with safe zeros
- [ ] 10. `parseExerciseSetURLParams` → struct, drop `dateStr`
- [ ] 11. Remove dead pprof Shutdown defer (or wire to ctx)
- [ ] 12. Simplify `loadExerciseSets` (sentinel → nil pointer or split queries)
- [ ] 13. `bufferEncode` chunked / iterative path
- [ ] 14. Sweep `mnd` annotations
- [ ] 15. Reconcile form size vs schema cap for descriptions
- [ ] 16. `stresstest` reports failures honestly
- [ ] 17. Replace `panic` in exerciseprogression or prove unreachable
- [ ] 18. Bump session lifetime
- [ ] 19. Regex-based table rename in `userdb.copyTableSchema`
- [ ] 20. Validation errors → flash, not 500, in admin exercise edit
