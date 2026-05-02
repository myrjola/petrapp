# Code Health Refactoring Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address six maintainability findings from the May 2026 architecture review: filename typo, per-request `os.Stat` in fileServerHandler, scattered magic numbers, duplicated `PeriodizationType` across packages, inconsistent time-field convention, and the `workout.Service` god object.

**Architecture:** Six refactorings, ordered low-risk to high-risk. The recommended sequence threads small wins first to build confidence before the larger refactors. Every task ends with `make ci` passing.

**Ordering constraints:**
- Tasks 1–3 are fully independent and can be done in any order.
- **Task 6 must come after Task 4** — Task 6 replaces `internal/workout/service.go` wholesale; if Task 4's edits to `service.go` (deleting `periodizationToProgression`, adding `ToProgression`/`FromProgression`) haven't run yet, they'll be lost. If executing out of order, port the helpers into `service-sessions.go` instead.
- Task 5 and Task 6 do not conflict — Task 5 changes types on `Session`/`sessionAggregate`; Task 6 moves methods around. Either order works as long as Task 6 picks up whatever shape `Session` currently has.

**Tech Stack:** Go 1.25 (with `1m` context model in some envs), SQLite via `mattn/go-sqlite3`, server-side templates via `html/template`, golangci-lint with `mnd` (magic-number-detector) enabled.

**Repository conventions to honor:**
- "Database first, then domain, then HTTP, then UI" workflow per `CLAUDE.md`.
- Run `make lint-fix` before committing; `make ci` for comprehensive validation.
- Comments end with a period; error types suffixed with "Error", sentinels prefixed with "Err".
- 100-line function limit (enforced by `funlen`).

---

## Task 1: Fix typo in test filename

**Files:**
- Rename: `cmd/web/handler-exercies-info_test.go` → `cmd/web/handler-exercise-info_test.go`

**Why:** The source file is `cmd/web/handler-exercise-info.go` (singular, no extra 'i'). The test file should match. `git mv` preserves history.

- [ ] **Step 1: Confirm the typo exists and the corrected name is free**

Run:
```bash
ls cmd/web/handler-exercie* cmd/web/handler-exercise-info*
```
Expected: shows only `handler-exercies-info_test.go` (typo) and `handler-exercise-info.go` (source). No file already named `handler-exercise-info_test.go`.

- [ ] **Step 2: Rename via git so blame history follows**

Run:
```bash
git mv cmd/web/handler-exercies-info_test.go cmd/web/handler-exercise-info_test.go
```

- [ ] **Step 3: Verify nothing else referenced the old name**

Run:
```bash
grep -rn "exercies" . --include="*.go" --include="*.md" --include="*.gohtml" --include="Makefile" 2>/dev/null
```
Expected: no output. (The misspelling appears nowhere else in the repo.)

- [ ] **Step 4: Verify the test still runs**

Run:
```bash
go test -run Test_application_exerciseInfo ./cmd/web/ -count 1
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git commit -m "fix(web): correct typo in handler-exercise-info_test.go filename"
```

---

## Task 2: Eliminate per-request os.Stat in fileServerHandler

**Files:**
- Modify: `cmd/web/handler-fileserver.go`
- Test: `cmd/web/handler-fileserver_test.go` (create)

**Background:** Currently `cmd/web/handler-fileserver.go` does `os.Stat(staticPath)` on every static-asset request to decide between serving the file and returning the custom 404 page. `http.FileServer` already does its own stat internally, so this is one redundant syscall per request. The fix is to wrap the response writer, intercept a 404 status from `fileServer.ServeHTTP`, and only then dispatch to the custom 404 handler.

The wrapper must also discard the default `404 page not found\n` body that `http.FileServer` writes after `WriteHeader(404)`.

- [ ] **Step 1: Read the current handler to understand its shape**

Run:
```bash
cat cmd/web/handler-fileserver.go
```

- [ ] **Step 2: Write a failing test for the 404 fallback**

Create `cmd/web/handler-fileserver_test.go`:

```go
package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_fileServer_servesExistingFile(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	resp, err := server.Client().Get(t.Context(), "/main.css")
	if err != nil {
		t.Fatalf("Failed to GET /main.css: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for existing static file, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Errorf("Expected text/css Content-Type, got %q", ct)
	}
}

func Test_fileServer_missingFileReturnsCustom404(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	resp, err := server.Client().Get(t.Context(), "/this-file-does-not-exist.css")
	if err != nil {
		t.Fatalf("Failed to GET nonexistent static file: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404 for missing static file, got %d", resp.StatusCode)
	}
	// The default http.FileServer 404 body is "404 page not found\n".
	// Our custom 404 page must be served instead.
	body := make([]byte, 4096)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])
	if strings.Contains(bodyStr, "404 page not found") && !strings.Contains(bodyStr, "Page Not Found") {
		t.Errorf("Expected custom 404 page (containing 'Page Not Found'), got default Go file-server body")
	}
	if !strings.Contains(bodyStr, "Page Not Found") {
		t.Errorf("Expected custom 404 body to contain 'Page Not Found', got: %s", bodyStr)
	}
}

func Test_fileServer_directoryTraversalReturns404(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	resp, err := server.Client().Get(t.Context(), "/../../../etc/passwd")
	if err != nil {
		t.Fatalf("Failed to GET traversal path: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected 404 or 400 for directory traversal, got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 3: Run the new tests — first two should already pass on existing code, third may pass or fail**

Run:
```bash
go test -run "Test_fileServer_" ./cmd/web/ -count 1 -v
```
Expected: tests run; the existing implementation should pass them since custom-404 fallback already works (we're refactoring without behavior change).

- [ ] **Step 4: Replace the handler with a response-writer interceptor**

Replace the entire body of `cmd/web/handler-fileserver.go` with:

```go
package main

import (
	"fmt"
	"net/http"
	"os"
	"path"
)

// notFoundInterceptor wraps http.ResponseWriter so we can detect when
// http.FileServer returns 404 (file not found) and substitute our custom
// 404 page instead. This eliminates the per-request os.Stat the handler
// previously used to make the same decision up-front.
//
// The interceptor buffers the WriteHeader call so headers set by upstream
// middleware (e.g. Cache-Control: public from cacheForever) are not flushed
// before we know the response is a 404.
type notFoundInterceptor struct {
	http.ResponseWriter
	is404         bool
	headerWritten bool
}

func (i *notFoundInterceptor) WriteHeader(status int) {
	if status == http.StatusNotFound {
		i.is404 = true
		return
	}
	i.headerWritten = true
	i.ResponseWriter.WriteHeader(status)
}

func (i *notFoundInterceptor) Write(b []byte) (int, error) {
	if i.is404 {
		// Discard the body http.FileServer would write ("404 page not found\n").
		return len(b), nil
	}
	if !i.headerWritten {
		i.headerWritten = true
		i.ResponseWriter.WriteHeader(http.StatusOK)
	}
	written, err := i.ResponseWriter.Write(b)
	if err != nil {
		return written, fmt.Errorf("write static asset: %w", err)
	}
	return written, nil
}

// fileServerHandler creates a file server handler with custom 404 handling.
func (app *application) fileServerHandler() (http.Handler, error) {
	fileRoot := path.Join(".", "ui", "static")
	if _, err := os.Stat(fileRoot); os.IsNotExist(err) {
		dir, findErr := findModuleDir()
		if findErr != nil {
			return nil, fmt.Errorf("findModuleDir: %w", findErr)
		}
		fileRoot = path.Join(dir, "ui", "static")
	}
	stat, err := os.Stat(fileRoot)
	if err != nil || !stat.IsDir() {
		return nil, fmt.Errorf("file server root %s does not exist or is not a directory", fileRoot)
	}

	fileServer := http.FileServer(http.Dir(fileRoot))
	notFoundHandler := app.sessionDeltaStack(http.HandlerFunc(app.notFound))

	return app.noAuthStack(cacheForever(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			interceptor := &notFoundInterceptor{ResponseWriter: w, is404: false, headerWritten: false}
			fileServer.ServeHTTP(interceptor, r)
			if interceptor.is404 {
				notFoundHandler.ServeHTTP(w, r)
			}
		}))), nil
}
```

Note that the `filepath.Clean` / directory-traversal check is gone — `http.FileServer` rejects `..` segments by returning 404 itself, which our interceptor will then convert to the custom 404 page.

- [ ] **Step 5: Run all the file-server tests plus the existing 404 tests**

Run:
```bash
go test -run "Test_fileServer_|Test_application_notFound" ./cmd/web/ -count 1 -v
```
Expected: all PASS. If `Test_fileServer_directoryTraversalReturns404` returns 200, the path was actually present in `ui/static` — pick a different path that genuinely doesn't exist.

- [ ] **Step 6: Run the full `make ci` to make sure nothing else broke**

Run:
```bash
make ci
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-fileserver.go cmd/web/handler-fileserver_test.go
git commit -m "perf(web): replace per-request stat with response-writer interceptor

http.FileServer already stats files internally; the up-front os.Stat in
fileServerHandler was a redundant syscall on every static-asset request.
A small ResponseWriter wrapper now intercepts 404s from FileServer and
substitutes the custom 404 page only when needed."
```

---

## Task 3: Audit and name magic constants

**Files:**
- Modify: various (enumerated in Step 1)

**Background:** 21 `nolint:mnd` suppressions exist across the codebase. Many are intrinsic time arithmetic (`24 * time.Hour`, `monday.AddDate(0, 0, 6)`) that cannot meaningfully be named. Others are configuration knobs (timeouts, lifetimes, cookie ages) that deserve named constants. The goal: extract the configuration knobs to named consts; suppress the genuinely intrinsic ones via the linter config rather than per-line comments.

**Rules to apply:**
1. **Extract a named constant** when the literal:
   - Represents a configuration knob (timeout, lifetime, max size, interval).
   - Conveys intent that the literal alone does not (e.g., `MaxBytes: 1 << 20` → `maxHeaderBytes`).
   - Appears more than once with the same meaning.
2. **Leave as a literal and remove the `nolint:mnd` comment** when:
   - It's calendar/time arithmetic (`6` for Mon→Sun, `24` for hours/day, `7` for days/week). Add these as ignored numbers in the linter config so the suppressions disappear.
3. **Update `.golangci.yml`** to add an `ignored-numbers` list for intrinsic constants:
   - `6, 7, 24` (calendar arithmetic).

- [ ] **Step 1: Enumerate every `nolint:mnd` suppression**

Run:
```bash
grep -rn "nolint:mnd" --include="*.go" cmd/ internal/ > /tmp/mnd-inventory.txt
cat /tmp/mnd-inventory.txt
```

Read each entry and categorize as either "extract to named const" or "intrinsic — covered by linter config update".

- [ ] **Step 2: Update `.golangci.yml` to ignore calendar/week-arithmetic constants**

Read the current `mnd` linter section first:

```bash
sed -n '160,180p' .golangci.yml
```

Modify the `mnd` block in `.golangci.yml` (around line 164) to add an `ignored-numbers` list. The expected new shape (preserve indentation and surrounding keys):

```yaml
    mnd:
      ignored-numbers:
        - "6"   # days from Monday to Sunday
        - "7"   # days in a week
        - "24"  # hours in a day
      ignored-functions:
        - args.Error
        - flag.Arg
        - flag.Duration.*
        - flag.Float.*
        - flag.Int.*
        - flag.Uint.*
        - os.Chmod
        - os.Mkdir.*
        - os.OpenFile
        - os.WriteFile
        - prometheus.ExponentialBuckets.*
        - prometheus.LinearBuckets
```

- [ ] **Step 3: Verify the linter still passes after the config change, then remove now-unnecessary suppressions**

Run:
```bash
make lint
```
Expected: PASS. Then for each occurrence of `6`, `7`, or `24` in the inventory from Step 1, drop the `//nolint:mnd ...` comment. Affected lines (verify they still match before editing):

- `internal/workout/service.go` lines 63, 94, 96, 97, 118, 270, 272, 283 — all calendar arithmetic.
- `internal/workout/repository-sessions.go` line 613 — `monday.AddDate(0, 0, 6)`.
- `cmd/web/main.go` line 146 — `24*time.Hour` cleanup interval. **Keep this one named** (see Step 4).

For each, change e.g.
```go
sunday := monday.AddDate(0, 0, 6) //nolint:mnd // 6 days after Monday is Sunday.
```
to
```go
sunday := monday.AddDate(0, 0, 6)
```

- [ ] **Step 4: Extract configuration knobs to named constants**

For each of the following, add the named constant and remove the `nolint` suppression:

**`cmd/web/main.go`** — replace lines around 142–150 (the session-manager block) with:

```go
const (
	// sessionStoreCleanupInterval bounds how often expired sessions are
	// pruned from sqlite3store. Daily is plenty given Lifetime below.
	sessionStoreCleanupInterval = 24 * time.Hour

	// sessionLifetime keeps users logged in across mid-workout sessions
	// so a 7am passkey login doesn't expire before the evening's lift.
	sessionLifetime = 7 * 24 * time.Hour
)

func initializeSessionManager(dbs *sqlite.Database) *scs.SessionManager {
	sessionManager := scs.New()
	sessionManager.Store = sqlite3store.NewWithCleanupInterval(dbs.ReadWrite, sessionStoreCleanupInterval)
	sessionManager.Lifetime = sessionLifetime
	sessionManager.Cookie.Persist = true
	sessionManager.Cookie.Secure = true
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	return sessionManager
}
```

**`cmd/web/server.go`** — both `idleTimeout := 2 * time.Minute` declarations (lines 24 and 53) refer to the same value. Hoist to a package-level const at the top of the file:

```go
const (
	defaultTimeout = 2 * time.Second

	// proxyIdleTimeout matches the upstream reverse proxy's keep-alive
	// window so we don't drop connections it expects to reuse.
	proxyIdleTimeout = 2 * time.Minute

	// maxHeaderBytes caps request header size at 1 MB. Generous to
	// accommodate WebAuthn attestation blobs.
	maxHeaderBytes = 1 << 20
)
```

Then replace the inline `idleTimeout := 2 * time.Minute //nolint:mnd ...` lines with uses of `proxyIdleTimeout`, and replace `MaxHeaderBytes: 1 << 20, //nolint:mnd ...` with `MaxHeaderBytes: maxHeaderBytes,`.

**`cmd/web/middleware.go`** — line ~302:

```go
const bfcacheCookieMaxAgeSeconds = 60
```
near the top of the file, then replace the inline literal with the constant:

```go
http.SetCookie(w, &http.Cookie{
	Name:     "inv_bfcache",
	Value:    rand.Text(),
	Path:     "/",
	MaxAge:   bfcacheCookieMaxAgeSeconds,
	SameSite: http.SameSiteLaxMode,
	Secure:   true,
	HttpOnly: false,
})
```

**`cmd/web/handlers.go`** line 19 — `formatFloat`'s `10`. Extract:
```go
const oneDecimalPlaceMultiplier = 10
```
near the function, and use it:
```go
rounded := math.Round(f*oneDecimalPlaceMultiplier) / oneDecimalPlaceMultiplier
```

**`cmd/migratetest/main.go` line 26** and **`cmd/smoketest/main.go` line 18** — extract the timeouts to package-level consts named `migrationTimeout` and `smokeTestTimeout`.

**`cmd/smoketest/main.go` line 38** — `len(os.Args) != 2` is just an arity check; keep the literal but rewrite as a guard with a named const:
```go
const expectedArgCount = 2
if len(os.Args) != expectedArgCount {
```

**`internal/e2etest/client.go` lines 89 and 124** — extract:
```go
const (
	readyPollTimeout  = 2 * time.Second
	readyPollInterval = 100 * time.Millisecond
)
```
at the top of the file and replace the literals.

- [ ] **Step 5: Verify all suppressions are accounted for**

Run:
```bash
grep -rn "nolint:mnd" --include="*.go" cmd/ internal/ | wc -l
```
Expected: a number near zero (a handful of irreducibly local literals may remain — that's OK; the goal is "don't have 21").

- [ ] **Step 6: Run full CI**

Run:
```bash
make ci
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add .golangci.yml cmd/ internal/
git commit -m "refactor: name configuration constants, ignore calendar arithmetic in mnd

Extract timeouts, lifetimes, and limits to named constants. Configure
golangci-lint's mnd linter to allow 6/7/24 (intrinsic calendar
arithmetic) so per-line nolint suppressions disappear. Reduces 21
nolint:mnd comments to a small remainder of truly local literals."
```

---

## Task 4: Unify PeriodizationType across packages

**Files:**
- Modify: `internal/exerciseprogression/progression.go` — add `String()` method, optionally `Parse()` helper.
- Modify: `internal/weekplanner/weekplanner.go` — replace local `PeriodizationType` and convert helpers to use the shared one.
- Modify: `internal/weekplanner/weekplanner_internal_test.go` — adjust references.
- Modify: `internal/workout/models.go` — replace local `PeriodizationType` with a type alias to the shared one.
- Modify: `internal/workout/service.go` — drop `periodizationToProgression` conversion.
- Modify: `internal/workout/repository-sessions.go` — adjust string round-trip.
- Modify: `internal/sqlite/schema.sql` — no change (DB column stays `'strength' | 'hypertrophy'`).

**Background:** Three packages each define their own `PeriodizationType`:
- `internal/exerciseprogression/progression.go`: `int` enum with `Strength`, `Hypertrophy`, `Endurance`.
- `internal/weekplanner/weekplanner.go`: `int` enum with `PeriodizationStrength = 0`, `PeriodizationHypertrophy = 1` (no Endurance).
- `internal/workout/models.go`: `string` enum with `"strength"`, `"hypertrophy"` (matches DB CHECK constraint, no Endurance).

The DB column stays a string (CHECK constraint requires "strength" or "hypertrophy"). The Go-side type should be the `int` enum from `exerciseprogression` since that's the lowest-level pure package and already covers all three values. We add `String()` and `ParsePeriodizationType` for serialization at the SQLite repo boundary.

- [ ] **Step 1: Add String/Parse helpers to `exerciseprogression.PeriodizationType`**

Append to `internal/exerciseprogression/progression.go`:

```go
// String returns the canonical lowercase name used for DB serialization.
// Panics on unknown values; callers must not pass uninitialized zero values
// other than Strength.
func (t PeriodizationType) String() string {
	switch t {
	case Strength:
		return "strength"
	case Hypertrophy:
		return "hypertrophy"
	case Endurance:
		return "endurance"
	}
	panic(fmt.Sprintf("exerciseprogression: unknown PeriodizationType %d", t))
}

// ParsePeriodizationType is the inverse of String.
func ParsePeriodizationType(s string) (PeriodizationType, error) {
	switch s {
	case "strength":
		return Strength, nil
	case "hypertrophy":
		return Hypertrophy, nil
	case "endurance":
		return Endurance, nil
	}
	return 0, fmt.Errorf("exerciseprogression: unknown periodization %q", s)
}
```

- [ ] **Step 2: Add a unit test for String/Parse round-trip**

Append to `internal/exerciseprogression/progression_test.go`:

```go
func TestPeriodizationType_StringParseRoundTrip(t *testing.T) {
	all := []exerciseprogression.PeriodizationType{
		exerciseprogression.Strength,
		exerciseprogression.Hypertrophy,
		exerciseprogression.Endurance,
	}
	for _, p := range all {
		got, err := exerciseprogression.ParsePeriodizationType(p.String())
		if err != nil {
			t.Errorf("ParsePeriodizationType(%q) error: %v", p.String(), err)
		}
		if got != p {
			t.Errorf("round trip: %v -> %q -> %v", p, p.String(), got)
		}
	}
	if _, err := exerciseprogression.ParsePeriodizationType("bogus"); err == nil {
		t.Error("expected error for unknown periodization, got nil")
	}
}
```

Run:
```bash
go test ./internal/exerciseprogression/ -count 1 -run TestPeriodizationType
```
Expected: PASS.

- [ ] **Step 3: Replace `weekplanner.PeriodizationType` with the shared type**

In `internal/weekplanner/weekplanner.go`:

1. Add an import for `github.com/myrjola/petrapp/internal/exerciseprogression`.
2. Replace the local type and constants:

```go
// Use exerciseprogression.PeriodizationType as the single source of truth.
type PeriodizationType = exerciseprogression.PeriodizationType

const (
	PeriodizationStrength    = exerciseprogression.Strength
	PeriodizationHypertrophy = exerciseprogression.Hypertrophy
)

// Number of periodization types we cycle through when alternating across sessions.
// Endurance exists in exerciseprogression but is not used by the weekly planner.
const numPeriodizationTypes = 2
```

3. The arithmetic in `Plan()` (`PeriodizationType((int(firstPT) + i) % numPeriodizationTypes)`) needs `int(firstPT)`. Since the underlying type is `int`, this still compiles. Verify by running:

```bash
go build ./internal/weekplanner/
```
Expected: PASS.

- [ ] **Step 4: Replace `workout.PeriodizationType` with a string-coded view of the shared type**

`workout.PeriodizationType` is currently a `string` because the repository scans it directly from the DB column. Two ways to handle this — pick option A:

**Option A (recommended):** Keep the surface API string-typed at the workout package boundary (simplest, fewest call-site edits) but make `workout.PeriodizationType` an alias to a method-bearing string type, then add converters that go through `exerciseprogression`. This narrows the duplication without forcing a sweeping change.

In `internal/workout/models.go`, change:

```go
// PeriodizationType determines the fixed rep target for all exercises in a session.
// It's a string at this layer because the SQLite CHECK constraint enforces
// 'strength' | 'hypertrophy'; conversion to the canonical exerciseprogression
// type happens via ToProgression / FromProgression below.
type PeriodizationType string

const (
	PeriodizationStrength    PeriodizationType = "strength"
	PeriodizationHypertrophy PeriodizationType = "hypertrophy"
)

// ToProgression converts to the canonical exerciseprogression type. Unknown
// values default to Strength to avoid panicking on legacy DB rows.
func (p PeriodizationType) ToProgression() exerciseprogression.PeriodizationType {
	switch p {
	case PeriodizationHypertrophy:
		return exerciseprogression.Hypertrophy
	case PeriodizationStrength:
		return exerciseprogression.Strength
	}
	return exerciseprogression.Strength
}

// FromProgression converts back, panicking on Endurance (which the workout
// package does not currently support — surface as a hard failure rather than
// silently mapping to Strength).
func FromProgression(p exerciseprogression.PeriodizationType) PeriodizationType {
	switch p {
	case exerciseprogression.Strength:
		return PeriodizationStrength
	case exerciseprogression.Hypertrophy:
		return PeriodizationHypertrophy
	case exerciseprogression.Endurance:
		panic("workout: endurance periodization not supported")
	}
	panic(fmt.Sprintf("workout: unknown PeriodizationType %v", p))
}
```

Add the necessary imports:

```go
import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
)
```

- [ ] **Step 5: Delete the duplicated conversion in `service.go`**

In `internal/workout/service.go`, find and delete the `periodizationToProgression` function (around the bottom of the file). Replace its callers with the new method:

```bash
grep -n "periodizationToProgression" internal/workout/service.go
```

For each call like:
```go
epType := periodizationToProgression(sess.PeriodizationType)
```
change it to:
```go
epType := sess.PeriodizationType.ToProgression()
```

For the `fromReps`/`toReps` calculations in `GetStartingWeight`:
```go
fromReps := exerciseprogression.TargetReps(periodizationToProgression(prev.PeriodizationType))
toReps := exerciseprogression.TargetReps(periodizationToProgression(targetType))
```
change to:
```go
fromReps := exerciseprogression.TargetReps(prev.PeriodizationType.ToProgression())
toReps := exerciseprogression.TargetReps(targetType.ToProgression())
```

- [ ] **Step 6: Update `service.go`'s `generateWeeklyPlan` to use the new converter**

In `generateWeeklyPlan`, find the periodization conversion (currently uses an `if` on `weekplanner.PeriodizationHypertrophy`):

```go
periodType := PeriodizationStrength
if ps.PeriodizationType == weekplanner.PeriodizationHypertrophy {
    periodType = PeriodizationHypertrophy
}
```

Replace with:
```go
periodType := FromProgression(ps.PeriodizationType)
```

(Now valid because `weekplanner.PeriodizationType` is an alias for `exerciseprogression.PeriodizationType`.)

- [ ] **Step 7: Run all affected tests**

Run:
```bash
go test ./internal/weekplanner/ ./internal/workout/ ./internal/exerciseprogression/ -count 1
```
Expected: PASS.

- [ ] **Step 8: Run full CI**

Run:
```bash
make ci
```
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add .
git commit -m "refactor: unify PeriodizationType using exerciseprogression as canonical source

Three packages defined their own PeriodizationType (two int enums and a
string enum). Make exerciseprogression.PeriodizationType the single
source of truth; weekplanner uses it via type alias; workout keeps a
string surface for DB serialization but provides ToProgression /
FromProgression converters and drops the private conversion helper."
```

---

## Task 5: Standardize nullable timestamps to *time.Time

**Files:**
- Modify: `internal/workout/models.go` — change `Session.StartedAt`, `Session.CompletedAt` to `*time.Time`.
- Modify: `internal/workout/repository.go` — change `sessionAggregate.StartedAt`, `sessionAggregate.CompletedAt` to `*time.Time`; update `parseTimestamp`.
- Modify: `internal/workout/repository-sessions.go` — propagate pointer through scans/inserts.
- Modify: `internal/workout/service.go` — replace all `IsZero()` checks with nil checks.
- Modify: `cmd/web/handler-home.go` — replace `IsZero()` checks.
- Modify: `cmd/web/handler-home_status_test.go` — adjust test helper.
- Modify: `cmd/web/handler-workout.go` — search for `.IsZero()` on Session timestamps.
- Modify: `ui/templates/pages/workout/workout.gohtml` — `.IsZero` → truthiness.
- Modify: `ui/templates/pages/home/day-cards.gohtml` and other templates as needed.

**Background:** `Session.StartedAt`/`CompletedAt` are `time.Time` (zero-value sentinel), but `Set.CompletedAt` and `ExerciseSet.WarmupCompletedAt` are `*time.Time` (nil sentinel). Pick one convention: **`*time.Time` for nullable timestamps**, because:
- It survives JSON serialization correctly (zero `time.Time` is non-null).
- `IsZero()` checks have subtle bugs when timestamps land in non-UTC zones.
- Maps directly to the DB's NULL semantics without sentinel decoding.

The change touches ~6 Go files plus 1–2 templates.

- [ ] **Step 1: Inventory all usages so the change is exhaustive**

Run:
```bash
grep -rn "StartedAt\|CompletedAt" --include="*.go" --include="*.gohtml" cmd/ internal/ ui/ > /tmp/timefields.txt
wc -l /tmp/timefields.txt
cat /tmp/timefields.txt
```

- [ ] **Step 2: Change the public model fields to *time.Time**

In `internal/workout/models.go`, update the `Session` struct:

```go
type Session struct {
	Date              time.Time
	DifficultyRating  *int
	StartedAt         *time.Time
	CompletedAt       *time.Time
	ExerciseSets      []ExerciseSet
	PeriodizationType PeriodizationType
}
```

(`Set.CompletedAt` and `ExerciseSet.WarmupCompletedAt` are already `*time.Time` — no change needed.)

- [ ] **Step 3: Change the repository aggregate**

In `internal/workout/repository.go`, update `sessionAggregate`:

```go
type sessionAggregate struct {
	Date              time.Time
	DifficultyRating  *int
	StartedAt         *time.Time
	CompletedAt       *time.Time
	ExerciseSets      []exerciseSetAggregate
	PeriodizationType PeriodizationType
}
```

Also update the `parseTimestamp` helper to return `*time.Time`. **Legacy-data care:** the pre-refactor code called `formatTimestamp(time.Time{})` even for unstarted sessions, storing the literal string `"0001-01-01T00:00:00.000Z"` in the DB. The old code happened to work because `IsZero()` returns true for both NULL-decoded and zero-parsed values. The new pointer version must treat parsed-zero as nil too, otherwise existing rows would suddenly look "started":

```go
// parseTimestamp parses a nullable database timestamp string. Returns nil
// for both SQL NULL and the legacy "0001-01-01T00:00:00.000Z" placeholder
// that pre-refactor code stored when sess.StartedAt was the zero time.
func parseTimestamp(timestampStr sql.NullString) (*time.Time, error) {
	if !timestampStr.Valid {
		return nil, nil //nolint:nilnil // valid case for nullable timestamps
	}
	parsedTime, err := time.Parse(timestampFormat, timestampStr.String)
	if err != nil {
		return nil, fmt.Errorf("parse timestamp format: %w", err)
	}
	if parsedTime.IsZero() {
		// Legacy data: pre-migration code stored time.Time{} as
		// "0001-01-01T00:00:00.000Z". Treat as NULL so callers see nil.
		return nil, nil //nolint:nilnil // valid case for nullable timestamps
	}
	return &parsedTime, nil
}
```

- [ ] **Step 4: Update the repository-sessions parsing/serialization**

In `internal/workout/repository-sessions.go`:

1. In `parseSessionRow`, replace the assignments:

```go
session.StartedAt, err = parseTimestamp(startedAtStr)
if err != nil {
	return sessionAggregate{}, fmt.Errorf("parse started_at: %w", err)
}
session.CompletedAt, err = parseTimestamp(completedAtStr)
if err != nil {
	return sessionAggregate{}, fmt.Errorf("parse completed_at: %w", err)
}
```

(Drop the `if !startedAt.IsZero()` style assignments — `parseTimestamp` now returns the pointer directly.)

2. In `insertSession` and `CreateBatch`, the existing `formatTimestamp(sess.StartedAt)` calls won't compile because `formatTimestamp` takes `time.Time`. Add a nil-aware wrapper near the existing helper:

```go
// formatNullableTimestamp returns the formatted timestamp or nil for NULL.
// Use this for *time.Time columns so the SQL driver inserts NULL rather
// than the formatted zero value.
func formatNullableTimestamp(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTimestamp(*t)
}
```

Then update the inserts:
```go
INSERT INTO workout_sessions (
    user_id, workout_date, difficulty_rating, started_at, completed_at, periodization_type
) VALUES (?, ?, ?, ?, ?, ?)`,
    userID, dateStr, sess.DifficultyRating,
    formatNullableTimestamp(sess.StartedAt), formatNullableTimestamp(sess.CompletedAt),
    sess.PeriodizationType
```

- [ ] **Step 5: Update `enrichSessionAggregate` in service.go**

In `internal/workout/service.go`, the enrich function copies fields. Change:
```go
session := Session{
	Date:              sessionAggr.Date,
	StartedAt:         sessionAggr.StartedAt,
	CompletedAt:       sessionAggr.CompletedAt,
	...
}
```

These already work after Steps 2–3 because both sides are `*time.Time`. Verify by reading the function and ensuring no `.IsZero()` is called on `sessionAggr.StartedAt`/`CompletedAt`.

- [ ] **Step 6: Update IsZero call sites in service.go**

Find them:
```bash
grep -n "StartedAt\.IsZero\|CompletedAt\.IsZero" internal/workout/service.go
```

Replace patterns:
```go
if !sess.Date.After(sunday) && !sess.StartedAt.IsZero() {
```
becomes:
```go
if !sess.Date.After(sunday) && sess.StartedAt != nil {
```

```go
if sess.StartedAt.IsZero() {
    sess.StartedAt = time.Now()
    return true, nil
}
```
becomes:
```go
if sess.StartedAt == nil {
    now := time.Now()
    sess.StartedAt = &now
    return true, nil
}
```

In `CompleteSession`:
```go
sess.CompletedAt = time.Now()
```
becomes:
```go
now := time.Now()
sess.CompletedAt = &now
```

- [ ] **Step 7: Update IsZero call sites in cmd/web**

In `cmd/web/handler-home.go`, replace:
```go
hasStarted := !session.StartedAt.IsZero() || completedSets > 0
```
with:
```go
hasStarted := session.StartedAt != nil || completedSets > 0
```

And:
```go
case !session.CompletedAt.IsZero() || allSetsCompleted:
```
with:
```go
case session.CompletedAt != nil || allSetsCompleted:
```

In `cmd/web/handler-home_status_test.go`, the `newSession` helper currently takes `time.Time` parameters and assigns them directly. Change it to accept pointers, or convert internally:

```go
newSession := func(startedAt, completedAt time.Time) workout.Session {
    var sa, ca *time.Time
    if !startedAt.IsZero() {
        sa = &startedAt
    }
    if !completedAt.IsZero() {
        ca = &completedAt
    }
    return workout.Session{
        Date:              time.Time{},
        DifficultyRating:  nil,
        StartedAt:         sa,
        CompletedAt:       ca,
        ExerciseSets:      nil,
        PeriodizationType: "",
    }
}
```

Other test files in `cmd/web/` may construct Session values directly (e.g. for table-driven tests). Grep:
```bash
grep -rn "StartedAt:\|CompletedAt:" --include="*.go" cmd/web/
```
For each direct field assignment using `time.Time{}` or a non-pointer, convert to nil or `&value`.

- [ ] **Step 8: Update templates**

In `ui/templates/pages/workout/workout.gohtml` (around line 100), replace:

```gotemplate
{{ if not .Session.CompletedAt.IsZero }}
    <span class="status-badge completed">Completed</span>
{{ else if not .Session.StartedAt.IsZero }}
    <span class="status-badge in-progress">In Progress</span>
{{ else }}
    <span class="status-badge not-started">Not Started</span>
{{ end }}
```

with:
```gotemplate
{{ if .Session.CompletedAt }}
    <span class="status-badge completed">Completed</span>
{{ else if .Session.StartedAt }}
    <span class="status-badge in-progress">In Progress</span>
{{ else }}
    <span class="status-badge not-started">Not Started</span>
{{ end }}
```

Search for any other templates that touch these fields:
```bash
grep -rn "StartedAt\|CompletedAt" ui/templates/
```
Apply the same `.IsZero` → truthiness rewrite to each.

- [ ] **Step 9: Build and run all tests**

Run:
```bash
go build ./...
```
Expected: builds clean. If you see "cannot use ... as ...", you missed a call site.

Then:
```bash
make test
```
Expected: PASS. Pay particular attention to:
- `TestDetermineWorkoutStatus` — tests build sessions directly; verify they still pass.
- `Test_application_workoutSwapExercise_*` — exercises session timestamp logic.
- Any e2e test that asserts the workout completion flow.

- [ ] **Step 10: Run full CI**

Run:
```bash
make ci
```
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add .
git commit -m "refactor(workout): standardize nullable timestamps on *time.Time

Session.StartedAt/CompletedAt were time.Time using zero-value sentinels;
Set.CompletedAt and ExerciseSet.WarmupCompletedAt were already
*time.Time. Unify on the pointer convention: it survives JSON
marshalling correctly, maps directly to SQL NULL, and avoids subtle
zone-related IsZero() bugs.

Templates use truthiness checks (Go templates treat nil as false), so
'.IsZero' becomes a plain '{{ if .Field }}'."
```

---

## Task 6: Split workout.Service into focused services

**Files:**
- Create: `internal/workout/service-preferences.go`
- Create: `internal/workout/service-sessions.go`
- Create: `internal/workout/service-exercises.go`
- Create: `internal/workout/service-feature-flags.go`
- Create: `internal/workout/service-data-export.go`
- Modify: `internal/workout/service.go` — becomes a thin façade holding the sub-services.
- Modify: every file in `cmd/web/` that calls `app.workoutService.<MethodName>` (10 files).
- Modify: `internal/workout/service_test.go` — adjust to use the sub-services.

**Background:** `internal/workout/service.go` is 1095 lines with 35+ methods spanning preferences, sessions/plan generation, exercises (CRUD + generation + history + swap + add), feature flags, volume aggregation, and data export. The repository layer already separates concerns (`prefs`, `sessions`, `exercises`, `featureFlags`, `muscleTargets`); the service should mirror that boundary.

**Strategy:** Define five focused service types in the same package and have `Service` become a struct holding them as exported fields. Callers move from `app.workoutService.GetSession(...)` to `app.workoutService.Sessions.Get(...)`. The package boundary is unchanged — only intra-package structure.

**Method map:**

| New service | Methods | Rough source line range |
|---|---|---|
| `Preferences` | GetUserPreferences, SaveUserPreferences | 37–58 |
| `Sessions` | RegenerateWeeklyPlanIfUnstarted, ResolveWeeklySchedule, generateWeeklyPlan, GetSession, enrichSessionAggregate, StartSession, CompleteSession, SaveFeedback, UpdateSetWeight, UpdateCompletedReps, RecordSetCompletion, MarkWarmupComplete, GetSessionsWithExerciseSince, GetStartingWeight, BuildProgression, WeeklyMuscleGroupVolume | 61–442, 459–510, 554–675, 676–760 |
| `Exercises` | List, GetExercise, GetExerciseSetsForExerciseSince, UpdateExercise, ListMuscleGroups, GenerateExercise, generateExerciseContent, FindCompatibleExercises, AddExercise, SwapExercise, findHistoricalSets, copySetsWithoutCompletion, createEmptySets, replaceExerciseInSession | 441–457, 512–552, 644–674, 762–944, 970–1040 |
| `FeatureFlags` | GetFeatureFlag, IsMaintenanceModeEnabled, ListFeatureFlags, SetFeatureFlag | 1042–1077 |
| `DataExport` | ExportUserData | 1080–1093 |

Note: `WeeklyMuscleGroupVolume` operates on sessions and uses the muscle-targets repo — put it in `Sessions` since callers already have a `[]Session`.

Note: `BuildProgression` reads sessions and exercises but its primary subject is the session — keep in `Sessions`.

Note: `Sessions` needs access to `prefs`, `exercises`, `muscleTargets` (for plan generation). It will hold references to those repos directly.

- [ ] **Step 1: Create the package-level types and shared dependencies**

Replace the entire body of `internal/workout/service.go` with:

```go
package workout

import (
	"log/slog"

	"github.com/myrjola/petrapp/internal/sqlite"
)

// Service composes the five focused sub-services. Each sub-service owns one
// area of the domain (preferences, sessions, exercises, feature flags, data
// export) and holds only the repositories it needs. Callers go through the
// matching field, e.g. svc.Sessions.Get(ctx, date).
type Service struct {
	Preferences  *PreferenceService
	Sessions     *SessionService
	Exercises    *ExerciseService
	FeatureFlags *FeatureFlagService
	DataExport   *DataExportService
}

// NewService creates a new workout service with all sub-services wired up.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	repo := newRepositoryFactory(db, logger).newRepository()
	exercises := newExerciseService(db, logger, repo, openaiAPIKey)
	sessions := newSessionService(db, logger, repo, exercises)
	return &Service{
		Preferences:  newPreferenceService(repo),
		Sessions:     sessions,
		Exercises:    exercises,
		FeatureFlags: newFeatureFlagService(logger, repo),
		DataExport:   newDataExportService(db),
	}
}
```

`SessionService` takes a reference to `ExerciseService` because its plan-generation paths call `Exercises.findHistoricalSets` (used by Add/Swap, but also by plan seeding). If on inspection `SessionService` doesn't need `Exercises`, drop the parameter.

- [ ] **Step 2: Create `service-preferences.go`**

```go
package workout

import (
	"context"
	"fmt"
)

// PreferenceService manages the user's weekly workout preferences.
type PreferenceService struct {
	repo *repository
}

func newPreferenceService(repo *repository) *PreferenceService {
	return &PreferenceService{repo: repo}
}

// Get retrieves the workout preferences for the authenticated user.
func (s *PreferenceService) Get(ctx context.Context) (Preferences, error) {
	prefs, err := s.repo.prefs.Get(ctx)
	if err != nil {
		return Preferences{}, fmt.Errorf("get user preferences: %w", err)
	}
	return prefs, nil
}

// Save persists workout preferences for the authenticated user.
func (s *PreferenceService) Save(ctx context.Context, prefs Preferences) error {
	if err := s.repo.prefs.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save user preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Create `service-sessions.go`**

Move all session-related methods from the original `service.go`. Imports needed: `context`, `errors`, `fmt`, `log/slog`, `time`, `github.com/myrjola/petrapp/internal/exerciseprogression`, `github.com/myrjola/petrapp/internal/sqlite`, `github.com/myrjola/petrapp/internal/weekplanner`.

Top of file:

```go
package workout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/exerciseprogression"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/weekplanner"
)

// SessionService manages workout sessions, weekly plan generation,
// per-set updates, and weekly volume aggregation.
type SessionService struct {
	db        *sqlite.Database
	logger    *slog.Logger
	repo      *repository
	exercises *ExerciseService // for findHistoricalSets when seeding new exercises in plans
}

func newSessionService(
	db *sqlite.Database,
	logger *slog.Logger,
	repo *repository,
	exercises *ExerciseService,
) *SessionService {
	return &SessionService{db: db, logger: logger, repo: repo, exercises: exercises}
}
```

Then for each method moved from the old `Service`, change the receiver:
- `func (s *Service) ResolveWeeklySchedule(...)` → `func (s *SessionService) ResolveWeeklySchedule(...)`
- `func (s *Service) GetSession(...)` → `func (s *SessionService) Get(...)` (rename: `GetSession` → `Get` since the type is now `SessionService`)
- `func (s *Service) StartSession(...)` → `func (s *SessionService) Start(...)`
- `func (s *Service) CompleteSession(...)` → `func (s *SessionService) Complete(...)`
- `func (s *Service) SaveFeedback(...)` → `func (s *SessionService) SaveFeedback(...)`
- `func (s *Service) UpdateSetWeight(...)` → `func (s *SessionService) UpdateSetWeight(...)`
- `func (s *Service) UpdateCompletedReps(...)` → `func (s *SessionService) UpdateCompletedReps(...)`
- `func (s *Service) RecordSetCompletion(...)` → `func (s *SessionService) RecordSetCompletion(...)`
- `func (s *Service) MarkWarmupComplete(...)` → `func (s *SessionService) MarkWarmupComplete(...)`
- `func (s *Service) RegenerateWeeklyPlanIfUnstarted(...)` → `func (s *SessionService) RegenerateWeeklyPlanIfUnstarted(...)`
- `func (s *Service) generateWeeklyPlan(...)` → `func (s *SessionService) generateWeeklyPlan(...)` (still package-private)
- `func (s *Service) enrichSessionAggregate(...)` → `func (s *SessionService) enrichSessionAggregate(...)`
- `func (s *Service) GetSessionsWithExerciseSince(...)` → `func (s *SessionService) GetSessionsWithExerciseSince(...)`
- `func (s *Service) GetStartingWeight(...)` → `func (s *SessionService) GetStartingWeight(...)`
- `func (s *Service) BuildProgression(...)` → `func (s *SessionService) BuildProgression(...)`
- `func (s *Service) WeeklyMuscleGroupVolume(...)` → `func (s *SessionService) WeeklyMuscleGroupVolume(...)`

Also move the package-level helpers used only by sessions: `mondayOf`, `aggregateMuscleGroupLoad`, `creditMuscleGroups`, `PrimarySetWeight`, `SecondarySetWeight`.

Internal calls within the sessions code that previously read `s.repo.X` keep working (the field is still `s.repo`).

- [ ] **Step 4: Create `service-exercises.go`**

```go
package workout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/sqlite"
)

// ExerciseService manages exercise CRUD, AI-assisted generation, history
// queries, and the add/swap operations that mutate workout slots.
type ExerciseService struct {
	db           *sqlite.Database
	logger       *slog.Logger
	repo         *repository
	openaiAPIKey string
}

func newExerciseService(
	db *sqlite.Database,
	logger *slog.Logger,
	repo *repository,
	openaiAPIKey string,
) *ExerciseService {
	return &ExerciseService{
		db:           db,
		logger:       logger,
		repo:         repo,
		openaiAPIKey: openaiAPIKey,
	}
}
```

Move these methods from old `Service`:
- `List` → `func (s *ExerciseService) List(...)`
- `GetExercise` → `func (s *ExerciseService) Get(...)` (rename)
- `GetExerciseSetsForExerciseSince` → `func (s *ExerciseService) GetSetsForExerciseSince(...)` (rename for brevity)
- `UpdateExercise` → `func (s *ExerciseService) Update(...)`
- `ListMuscleGroups` → `func (s *ExerciseService) ListMuscleGroups(...)`
- `GenerateExercise` → `func (s *ExerciseService) Generate(...)`
- `generateExerciseContent` → `func (s *ExerciseService) generateContent(...)`
- `FindCompatibleExercises` → `func (s *ExerciseService) FindCompatible(...)`
- `AddExercise` → `func (s *ExerciseService) AddToWorkout(...)` — still touches sessions repo, no need to delegate to SessionService since it's a write-through.
- `SwapExercise` → `func (s *ExerciseService) SwapInWorkout(...)`
- `findHistoricalSets` (private)
- `copySetsWithoutCompletion` (private)
- `createEmptySets` (private)
- `replaceExerciseInSession` (private)

Also move `createMinimalExercise` (free function near the bottom).

Constants `defaultMinReps`, `defaultMaxReps` move with `AddToWorkout`.

- [ ] **Step 5: Create `service-feature-flags.go`**

```go
package workout

import (
	"context"
	"fmt"
	"log/slog"
)

// FeatureFlagService manages persisted feature flags.
type FeatureFlagService struct {
	logger *slog.Logger
	repo   *repository
}

func newFeatureFlagService(logger *slog.Logger, repo *repository) *FeatureFlagService {
	return &FeatureFlagService{logger: logger, repo: repo}
}

// Get retrieves a feature flag by name.
func (s *FeatureFlagService) Get(ctx context.Context, name string) (FeatureFlag, error) {
	flag, err := s.repo.featureFlags.Get(ctx, name)
	if err != nil {
		return FeatureFlag{}, fmt.Errorf("get feature flag %s: %w", name, err)
	}
	return flag, nil
}

// IsMaintenanceModeEnabled checks if maintenance mode is enabled. Returns
// false on lookup error to fail safe.
func (s *FeatureFlagService) IsMaintenanceModeEnabled(ctx context.Context) bool {
	flag, err := s.repo.featureFlags.Get(ctx, "maintenance_mode")
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "failed to check maintenance mode flag", slog.Any("error", err))
		return false
	}
	return flag.Enabled
}

// List retrieves all feature flags.
func (s *FeatureFlagService) List(ctx context.Context) ([]FeatureFlag, error) {
	flags, err := s.repo.featureFlags.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list feature flags: %w", err)
	}
	return flags, nil
}

// Set updates or creates a feature flag.
func (s *FeatureFlagService) Set(ctx context.Context, flag FeatureFlag) error {
	if err := s.repo.featureFlags.Set(ctx, flag); err != nil {
		return fmt.Errorf("set feature flag %s: %w", flag.Name, err)
	}
	return nil
}
```

- [ ] **Step 6: Create `service-data-export.go`**

```go
package workout

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// DataExportService produces per-user SQLite database dumps for GDPR
// data-portability requests.
type DataExportService struct {
	db *sqlite.Database
}

func newDataExportService(db *sqlite.Database) *DataExportService {
	return &DataExportService{db: db}
}

// ExportForCurrentUser writes a SQLite database containing only the
// authenticated user's data to a temp file and returns the path. The
// caller is responsible for deleting the file when done streaming it.
func (s *DataExportService) ExportForCurrentUser(ctx context.Context) (string, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return "", errors.New("no authenticated user found in context")
	}
	exportPath, err := s.db.CreateUserDB(ctx, userID, os.TempDir())
	if err != nil {
		return "", fmt.Errorf("create user database: %w", err)
	}
	return exportPath, nil
}
```

- [ ] **Step 7: Update all handler call sites**

Search:
```bash
grep -rn "app\.workoutService\." cmd/web/ --include="*.go"
```

Apply this rewrite map (do it file by file, building between files to catch typos early):

| Old call | New call |
|---|---|
| `app.workoutService.GetUserPreferences(...)` | `app.workoutService.Preferences.Get(...)` |
| `app.workoutService.SaveUserPreferences(...)` | `app.workoutService.Preferences.Save(...)` |
| `app.workoutService.RegenerateWeeklyPlanIfUnstarted(...)` | `app.workoutService.Sessions.RegenerateWeeklyPlanIfUnstarted(...)` |
| `app.workoutService.ResolveWeeklySchedule(...)` | `app.workoutService.Sessions.ResolveWeeklySchedule(...)` |
| `app.workoutService.GetSession(...)` | `app.workoutService.Sessions.Get(...)` |
| `app.workoutService.StartSession(...)` | `app.workoutService.Sessions.Start(...)` |
| `app.workoutService.CompleteSession(...)` | `app.workoutService.Sessions.Complete(...)` |
| `app.workoutService.SaveFeedback(...)` | `app.workoutService.Sessions.SaveFeedback(...)` |
| `app.workoutService.UpdateSetWeight(...)` | `app.workoutService.Sessions.UpdateSetWeight(...)` |
| `app.workoutService.UpdateCompletedReps(...)` | `app.workoutService.Sessions.UpdateCompletedReps(...)` |
| `app.workoutService.RecordSetCompletion(...)` | `app.workoutService.Sessions.RecordSetCompletion(...)` |
| `app.workoutService.MarkWarmupComplete(...)` | `app.workoutService.Sessions.MarkWarmupComplete(...)` |
| `app.workoutService.GetStartingWeight(...)` | `app.workoutService.Sessions.GetStartingWeight(...)` |
| `app.workoutService.BuildProgression(...)` | `app.workoutService.Sessions.BuildProgression(...)` |
| `app.workoutService.WeeklyMuscleGroupVolume(...)` | `app.workoutService.Sessions.WeeklyMuscleGroupVolume(...)` |
| `app.workoutService.GetSessionsWithExerciseSince(...)` | `app.workoutService.Sessions.GetSessionsWithExerciseSince(...)` |
| `app.workoutService.List(...)` (exercises) | `app.workoutService.Exercises.List(...)` |
| `app.workoutService.GetExercise(...)` | `app.workoutService.Exercises.Get(...)` |
| `app.workoutService.UpdateExercise(...)` | `app.workoutService.Exercises.Update(...)` |
| `app.workoutService.ListMuscleGroups(...)` | `app.workoutService.Exercises.ListMuscleGroups(...)` |
| `app.workoutService.GenerateExercise(...)` | `app.workoutService.Exercises.Generate(...)` |
| `app.workoutService.AddExercise(...)` | `app.workoutService.Exercises.AddToWorkout(...)` |
| `app.workoutService.SwapExercise(...)` | `app.workoutService.Exercises.SwapInWorkout(...)` |
| `app.workoutService.FindCompatibleExercises(...)` | `app.workoutService.Exercises.FindCompatible(...)` |
| `app.workoutService.GetExerciseSetsForExerciseSince(...)` | `app.workoutService.Exercises.GetSetsForExerciseSince(...)` |
| `app.workoutService.GetFeatureFlag(...)` | `app.workoutService.FeatureFlags.Get(...)` |
| `app.workoutService.IsMaintenanceModeEnabled(...)` | `app.workoutService.FeatureFlags.IsMaintenanceModeEnabled(...)` |
| `app.workoutService.ListFeatureFlags(...)` | `app.workoutService.FeatureFlags.List(...)` |
| `app.workoutService.SetFeatureFlag(...)` | `app.workoutService.FeatureFlags.Set(...)` |
| `app.workoutService.ExportUserData(...)` | `app.workoutService.DataExport.ExportForCurrentUser(...)` |

After each file, run:
```bash
go build ./cmd/web/
```
Expected: PASS.

- [ ] **Step 8: Update the middleware that uses workoutService**

`cmd/web/middleware.go` calls `app.workoutService.IsMaintenanceModeEnabled(ctx)` — already covered by the rewrite above, but double-check the maintenance-mode middleware compiles.

- [ ] **Step 9: Update tests**

`internal/workout/service_test.go` constructs services and calls methods directly. Apply the same rewrite map (e.g. `svc.GetSession` → `svc.Sessions.Get`). Also update `NewService` callers if signatures change.

Other tests in `cmd/web/` that touch `server.workoutService` (rare — most go through the HTTP surface) need the rewrite too. Find them:

```bash
grep -rn "workoutService\." --include="*.go" .
```

- [ ] **Step 10: Build and run all tests**

Run:
```bash
go build ./...
```
Expected: PASS.

```bash
make test
```
Expected: PASS.

- [ ] **Step 11: Run full CI**

Run:
```bash
make ci
```
Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/workout/ cmd/web/
git commit -m "refactor(workout): split Service into Preferences/Sessions/Exercises/FeatureFlags/DataExport

The workout.Service god object had grown to 1095 lines spanning five
distinct concerns. Split along the existing repository boundaries into
focused sub-services exposed as fields of Service, so callers move from
'svc.GetSession(...)' to 'svc.Sessions.Get(...)'. The package boundary
is unchanged.

Method renames drop redundant suffixes now that the receiver type
disambiguates (e.g. GetExercise -> Exercises.Get, AddExercise ->
Exercises.AddToWorkout)."
```

---

## Final verification

After all six tasks are complete and committed:

- [ ] **Run the full CI pipeline once more on the cumulative result**

```bash
make ci
```
Expected: PASS.

- [ ] **Verify the magic-number suppression count dropped meaningfully**

```bash
grep -rn "nolint:mnd" --include="*.go" cmd/ internal/ | wc -l
```
Expected: < 5 (down from 21).

- [ ] **Verify no `IsZero()` on session timestamps remains**

```bash
grep -rn "StartedAt\.IsZero\|CompletedAt\.IsZero" --include="*.go" --include="*.gohtml" cmd/ internal/ ui/
```
Expected: no output.

- [ ] **Verify the Service file shrank**

```bash
wc -l internal/workout/service*.go
```
Expected: each sub-service file under ~400 lines; `service.go` itself under ~50 lines (just the Service struct and constructor).

- [ ] **Verify only one PeriodizationType definition exists**

```bash
grep -rn "^type PeriodizationType" --include="*.go" internal/
```
Expected: exactly one definition (in `internal/exerciseprogression/progression.go`); the workout and weekplanner ones are aliases.

- [ ] **Verify the typo is gone everywhere**

```bash
grep -rn "exercies" --include="*.go" --include="*.md" --include="*.gohtml" .
```
Expected: no output.

- [ ] **Smoke-test the legacy-timestamp parsing path**

Run the Playwright smoke test, which exercises the full register → schedule → start → complete workflow and would surface any timestamp regression in the home/workout templates:

```bash
go test -run Test_playwright_smoketest ./cmd/web/ -count 1
```
Expected: PASS. (Skip with `-short` if Playwright browsers aren't installed; the unit-test suite already covers the parse logic.)
