# Dev-mode Static Asset No-Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the browser from caching static assets in dev so editing `ui/static/main.css` or `main.js` shows up after a refresh, without changing production behavior.

**Architecture:** Select the static-file cache middleware at handler-construction time based on `app.devMode` (already `true` whenever `FLY_APP_NAME` is unset). In dev use the existing `noStore` middleware; in prod keep `cacheForever` exactly as today. No Dockerfile changes, no new middleware, no new helpers.

**Tech Stack:** Go stdlib `net/http`, existing `e2etest` harness for the assertion test.

**Spec:** `docs/superpowers/specs/2026-05-17-dev-static-no-cache-design.md`

---

## File Structure

- `cmd/web/handler-fileserver.go` — modify `fileServerHandler` to pick `cache := cacheForever` or `cache := noStore` based on `app.devMode`, then wrap as today. Update the `notFoundInterceptor` doc comment that names `cacheForever` so it covers both middlewares.
- `cmd/web/handler-fileserver_test.go` — add one test asserting the dev `Cache-Control` header.

No other files. No new files.

---

### Task 1: Add failing test asserting dev `Cache-Control: no-store`

**Files:**
- Modify: `cmd/web/handler-fileserver_test.go` — append a new test after `Test_fileServer_missingFileReturnsCustom404`.

- [ ] **Step 1: Add the failing test**

Append to `cmd/web/handler-fileserver_test.go`:

```go
func Test_fileServer_devModeUsesNoStoreCacheControl(t *testing.T) {
	// testLookupEnv does not set FLY_APP_NAME, so app.devMode is true.
	// In dev the static file server must disable browser caching so that
	// edits to ui/static/main.css and main.js are visible on refresh.
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
		t.Fatalf("Expected 200 for existing static file, got %d", resp.StatusCode)
	}
	got := resp.Header.Get("Cache-Control")
	want := "no-store, max-age=0, must-revalidate"
	if got != want {
		t.Errorf("Expected Cache-Control %q in dev mode, got %q", want, got)
	}
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test -v ./cmd/web -run Test_fileServer_devModeUsesNoStoreCacheControl`

Expected: FAIL. The current handler wraps with `cacheForever`, so the response carries `Cache-Control: public, max-age=31536000, immutable`. The assertion compares against `no-store, max-age=0, must-revalidate` and fails with a message like:

```
Expected Cache-Control "no-store, max-age=0, must-revalidate" in dev mode, got "public, max-age=31536000, immutable"
```

If the test fails for any other reason (compile error, server start error, 404 on `/main.css`), stop and investigate before continuing — the assertion is the only failure we want at this step.

- [ ] **Step 3: Commit the failing test**

```bash
git add cmd/web/handler-fileserver_test.go
git commit -m "test: assert dev-mode static file server sends no-store Cache-Control"
```

---

### Task 2: Switch dev mode to `noStore` middleware

**Files:**
- Modify: `cmd/web/handler-fileserver.go` — change the doc comment on `notFoundInterceptor` and the middleware selection in `fileServerHandler`.

- [ ] **Step 1: Update the `notFoundInterceptor` doc comment**

The current comment at `cmd/web/handler-fileserver.go:10-17` names `cacheForever` specifically. Generalize it so the reference is still accurate after Task 2's wiring change.

Replace this block:

```go
// notFoundInterceptor wraps http.ResponseWriter so we can detect when
// http.FileServer returns 404 (file not found) and substitute our custom
// 404 page instead. This eliminates the per-request os.Stat the handler
// previously used to make the same decision up-front.
//
// The interceptor buffers the WriteHeader call so headers set by upstream
// middleware (e.g. Cache-Control: public from cacheForever) are not flushed
// before we know the response is a 404.
```

With:

```go
// notFoundInterceptor wraps http.ResponseWriter so we can detect when
// http.FileServer returns 404 (file not found) and substitute our custom
// 404 page instead. This eliminates the per-request os.Stat the handler
// previously used to make the same decision up-front.
//
// The interceptor buffers the WriteHeader call so headers set by upstream
// middleware (Cache-Control from cacheForever in prod or noStore in dev)
// are not flushed before we know the response is a 404.
```

- [ ] **Step 2: Select middleware by `app.devMode`**

In `fileServerHandler` (`cmd/web/handler-fileserver.go:57-82`), replace the current `return` block:

```go
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

With:

```go
	fileServer := http.FileServer(http.Dir(fileRoot))
	notFoundHandler := app.sessionDeltaStack(http.HandlerFunc(app.notFound))

	// In dev, disable browser caching so edits to ui/static/* are visible on
	// refresh. In prod, main.css and main.js are md5-fingerprinted by the
	// Dockerfile ui-builder stage so cacheForever (immutable) is safe.
	cache := cacheForever
	if app.devMode {
		cache = noStore
	}

	return app.noAuthStack(cache(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			interceptor := &notFoundInterceptor{ResponseWriter: w, is404: false, headerWritten: false}
			fileServer.ServeHTTP(interceptor, r)
			if interceptor.is404 {
				notFoundHandler.ServeHTTP(w, r)
			}
		}))), nil
}
```

`cacheForever` and `noStore` both have signature `func(http.Handler) http.Handler` (see `cmd/web/middleware.go:88-95` and `:107-113`), so the variable assignment compiles directly.

- [ ] **Step 3: Run the new test and confirm it passes**

Run: `go test -v ./cmd/web -run Test_fileServer_devModeUsesNoStoreCacheControl`

Expected: PASS.

- [ ] **Step 4: Run the full file-server tests and confirm nothing regressed**

Run: `go test -v ./cmd/web -run Test_fileServer`

Expected: all three `Test_fileServer_*` tests PASS.

- [ ] **Step 5: Run lint-fix and full test suite**

Per `CLAUDE.md` ("run before committing"):

Run: `make lint-fix`
Expected: clean exit, no diagnostics on the modified files.

Run: `make test`
Expected: full suite passes.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/handler-fileserver.go
git commit -m "fix: disable static asset browser cache in dev mode

In dev (FLY_APP_NAME unset) the static file server now sends
Cache-Control: no-store instead of the immutable header used in
production, so edits to ui/static/main.css and main.js are visible on
refresh without a hard reload. Production behavior is unchanged."
```

---

### Task 3: Manual smoke test in browser

**Files:** none (manual verification).

- [ ] **Step 1: Start the dev server**

Run: `make run` (or your usual dev entry; if your local convention differs, use that — the goal is a server with `FLY_APP_NAME` unset).

- [ ] **Step 2: Load the app and inspect a static asset response**

Open the app in a browser, open DevTools → Network, reload, click the `main.css` request. Confirm the response header `Cache-Control: no-store, max-age=0, must-revalidate`.

- [ ] **Step 3: Edit `ui/static/main.css` and refresh**

Make a visible CSS change (e.g. set `body { background: hotpink; }` temporarily), save, do a normal browser refresh (not hard reload). Confirm the change appears. Revert the CSS edit.

Expected: change is visible on the first plain refresh. If it is not, the cache header is not being applied as expected — re-check Task 2.

- [ ] **Step 4: Stop the dev server**

No commit for this task.

---

## Self-review

**Spec coverage:**
- Spec "Mode selection" → Task 2 Step 2 (uses `app.devMode`).
- Spec "Middleware" (reuse `noStore`, no new helper) → Task 2 Step 2 (assigns `cache = noStore`; no new function defined).
- Spec "Wiring" → Task 2 Step 2 (exact code matches the spec snippet, expanded with the comment the spec asked for).
- Spec "Why `no-store`" → no implementation needed; rationale only.
- Spec "Testing" (assert dev `Cache-Control` header in `handler-fileserver_test.go`, leave existing tests untouched, no prod-path assertion) → Task 1 (new test) + Task 2 Step 4 (existing tests still pass).
- Spec "Non-goals" (no Dockerfile change, no new fingerprinting, no file watcher, no asset-pipeline reconciliation) → no tasks touch any of those areas.

**Placeholder scan:** No TBDs, no "implement appropriate X", no "similar to Task N". Every code step shows the full code or full replacement.

**Type/name consistency:** Function names used — `cacheForever`, `noStore`, `fileServerHandler`, `notFoundInterceptor`, `Test_fileServer_devModeUsesNoStoreCacheControl` — match across tasks and match the existing code (`cmd/web/middleware.go:89,107`, `cmd/web/handler-fileserver.go:18,57`). The expected `Cache-Control` value `"no-store, max-age=0, must-revalidate"` matches the literal string set by `noStore` at `cmd/web/middleware.go:109`.
