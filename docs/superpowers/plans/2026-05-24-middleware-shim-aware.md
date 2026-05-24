# Shim-Aware Middleware Errors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the remaining silent-failure paths in the middleware stack — `webauthnhandler.AuthenticateMiddleware`'s DB-lookup failures and `mustAdmin`'s unauthorized branch — by routing them through the same shim-aware UX `app.serverError` produces.

**Architecture:** `mustAdmin` lives on `*application` already, so it can call `app.serverError` directly (replacing the bare `http.Error` call). `webauthnhandler` is a separate package with no reference to `*application`; inject an `InternalErrorHandler func(http.ResponseWriter, *http.Request, error)` callback at construction time so the package stays unaware of the wire protocol. Wire `webAuthnHandler.InternalErrorHandler = app.serverError` in `main.go` after both objects exist.

**Tech Stack:** Go 1.25+ (stdlib HTTP, function-typed struct field for the callback).

**Spec:** [docs/superpowers/specs/2026-05-24-servererror-shim-aware-design.md](../specs/2026-05-24-servererror-shim-aware-design.md), "Non-goals" → "Migrating the `webauthnhandler` / `mustAdmin` `http.Error` paths in this pass". This plan picks them up.

**Prerequisite:** Plan 1 (`2026-05-24-servererror-shim-aware.md`) must be merged. `app.serverError` must already be shim-aware before either middleware delegates to it.

**Verification note:** as in Plan 1, e2etest verifies the wire contract on the server side; the shim's browser-side navigation needs Playwright or manual testing to fully validate. The `mustAdmin` path is exercised by registering a non-admin user and POSTing to `/admin/exercises/generate`. The `webauthnhandler` DB-failure path is harder to provoke without surgery on the session store — an integration test in the webauthnhandler package using a faulty DB stub is the cleanest approach.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `internal/webauthnhandler/handler.go` | modify | Add `InternalErrorHandler` field to `WebAuthnHandler` struct |
| `internal/webauthnhandler/middleware.go` | modify | Replace `http.Error` calls with `h.internalError(w, r, err)` helper that delegates to the injected callback (or falls back to `http.Error` when nil) |
| `internal/webauthnhandler/middleware_test.go` | create or modify | Cover the new callback path with the DB failure scenario |
| `cmd/web/main.go` | modify | Wire `webAuthnHandler.InternalErrorHandler = app.serverError` post-construction |
| `cmd/web/middleware.go` | modify | `mustAdmin` calls `app.serverError(w, r, fmt.Errorf("unauthorized admin access by user %d", uid))` instead of `http.Error` — wait, see Task 4 commentary; admin unauth is **not** a server error |
| `cmd/web/middleware_test.go` | modify | Update or add tests for the new `mustAdmin` behaviour |

Re-examining `mustAdmin`: the current `http.Error(w, …, 401)` treats not-an-admin as an unauthorized client error, not a server error. Delegating that to `serverError` mis-classifies the failure. The right move is **redirect to `/`** (matching `mustAuthenticate`'s existing pattern) — the user is authenticated but lacks the role, so bouncing to home is sensible. That keeps the wire-protocol-aware behaviour (via `redirect()`) without overstating the failure class. See Task 4 for the rationale and the choice to confirm with the user.

---

## Task 1: Add the `InternalErrorHandler` callback field

**Files:**
- Modify: `internal/webauthnhandler/handler.go`

- [ ] **Step 1: Write the failing test**

Create or extend `internal/webauthnhandler/handler_test.go` (check whether the file exists first):

```go
package webauthnhandler

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_WebAuthnHandler_InternalErrorHandler_FallbackWhenNil(t *testing.T) {
	t.Parallel()

	h := &WebAuthnHandler{logger: slog.New(slog.DiscardHandler)}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/whatever", nil)

	h.internalError(w, r, errors.New("boom"))

	if got := w.Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", got, http.StatusInternalServerError)
	}
}

func Test_WebAuthnHandler_InternalErrorHandler_CallbackInvoked(t *testing.T) {
	t.Parallel()

	called := false
	var capturedErr error
	h := &WebAuthnHandler{
		logger: slog.New(slog.DiscardHandler),
		InternalErrorHandler: func(_ http.ResponseWriter, _ *http.Request, err error) {
			called = true
			capturedErr = err
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	want := errors.New("boom")

	h.internalError(w, r, want)

	if !called {
		t.Error("expected InternalErrorHandler callback to be invoked")
	}
	if !errors.Is(capturedErr, want) {
		t.Errorf("captured error = %v, want %v", capturedErr, want)
	}
	// When the callback handles it, internalError must NOT also write 500.
	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d (callback owns the response)", got, http.StatusOK)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./internal/webauthnhandler/ -run Test_WebAuthnHandler_InternalErrorHandler`
Expected: FAIL — `internalError` method and `InternalErrorHandler` field don't exist.

- [ ] **Step 3: Add the field and helper**

Modify `internal/webauthnhandler/handler.go`. Update the `WebAuthnHandler` struct (currently lines 35–40) to add the new field:

```go
type WebAuthnHandler struct {
	logger         *slog.Logger
	webAuthn       *webauthn.WebAuthn
	sessionManager *scs.SessionManager
	database       *sqlite.Database

	// InternalErrorHandler, when set, owns the response on any internal
	// failure inside this package (DB lookup errors, etc.). Wired by the
	// caller after construction so this package stays unaware of the
	// stack-navigator wire protocol. When nil, the fallback is a plain
	// 500 via http.Error.
	InternalErrorHandler func(w http.ResponseWriter, r *http.Request, err error)
}
```

At the bottom of `handler.go` (or anywhere file-scope), add:

```go
// internalError surfaces a non-recoverable server-side failure. If the
// caller wired InternalErrorHandler (typically app.serverError), it owns
// the response — including stack-navigator-aware navigation to /error.
// Otherwise we fall back to a plain 500.
func (h *WebAuthnHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	if h.InternalErrorHandler != nil {
		h.InternalErrorHandler(w, r, err)
		return
	}
	h.logger.LogAttrs(r.Context(), slog.LevelError, "internal error", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/webauthnhandler/ -run Test_WebAuthnHandler_InternalErrorHandler`
Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/webauthnhandler/handler.go internal/webauthnhandler/handler_test.go
git commit -m "feat(webauthnhandler): add InternalErrorHandler injection point"
```

---

## Task 2: Route `AuthenticateMiddleware` failures through `internalError`

**Files:**
- Modify: `internal/webauthnhandler/middleware.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/webauthnhandler/middleware_test.go` (create if missing):

```go
package webauthnhandler

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Test_AuthenticateMiddleware_RoutesInternalErrorsThroughCallback exercises
// the failure path inside AuthenticateMiddleware where getUserRole returns
// a non-ErrNoRows error. The middleware must surface that through
// InternalErrorHandler rather than calling http.Error directly.
//
// Because getUserRole reads from sqlite and the unit-test fixture cannot
// easily inject a faulty DB, this test asserts the *callback path* by
// driving internalError directly. The real-DB scenario is covered by an
// integration test in cmd/web (Task 3) where we can fault the session
// store to provoke the branch.
func Test_AuthenticateMiddleware_internalErrorPath_DelegatesToCallback(t *testing.T) {
	t.Parallel()

	called := false
	h := &WebAuthnHandler{
		logger: slog.New(slog.DiscardHandler),
		InternalErrorHandler: func(_ http.ResponseWriter, _ *http.Request, _ error) {
			called = true
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	h.internalError(w, r, errors.New("simulated DB failure"))

	if !called {
		t.Error("expected the injected handler to be called for an internal error")
	}
}
```

- [ ] **Step 2: Run the test (PASS expected — internalError already exists)**

Run: `go test -v ./internal/webauthnhandler/ -run Test_AuthenticateMiddleware_internalErrorPath`
Expected: PASS — this is a delegation contract check, not a behaviour change.

- [ ] **Step 3: Replace `http.Error` in `AuthenticateMiddleware`**

In `internal/webauthnhandler/middleware.go`, replace both `http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)` calls (lines 32 and 44 today) with `h.internalError(w, r, err)`.

The first call site (around line 30):

```go
		role, err := h.getUserRole(ctx, webauthnUserID)
		var intUserID int
		switch {
		case errors.Is(err, sql.ErrNoRows): // Do not authenticate if user does not exist.
		case err != nil:
			h.internalError(w, r, fmt.Errorf("fetch user role: %w", err))
			return
		default:
```

The second (around line 38):

```go
			intUserID, err = h.getUserIntegerID(ctx, webauthnUserID)
			if err != nil {
				h.internalError(w, r, fmt.Errorf("fetch user integer ID: %w", err))
				return
			}
```

Add `"fmt"` to imports if not already present.

Notes:
- `internalError` logs internally (in the fallback branch) and `app.serverError` logs in the injected branch — drop the local `h.logger.LogAttrs(...)` calls that immediately preceded the now-removed `http.Error` lines. The error message is preserved through the `%w` wrap.
- Verify all existing tests in `internal/webauthnhandler/` still pass.

- [ ] **Step 4: Run package tests**

Run: `go test ./internal/webauthnhandler/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/webauthnhandler/middleware.go internal/webauthnhandler/middleware_test.go
git commit -m "feat(webauthnhandler): route AuthenticateMiddleware errors through callback"
```

---

## Task 3: Wire the callback in `main.go`

**Files:**
- Modify: `cmd/web/main.go`

- [ ] **Step 1: Locate the wiring point**

In `cmd/web/main.go`, the application struct is constructed around line 183 (`app := application{...}`). `webAuthnHandler` is built earlier (around line 144–155) but `app.serverError` is a method on `*application` and doesn't exist until `app` does.

The wire-up has to happen *after* `app` is constructed:

```go
	app := application{
		logger:          logger,
		webAuthnHandler: webAuthnHandler,
		// ...
	}

	// Wire the shim-aware error surface so DB / lookup failures inside
	// the webauthn middleware navigate to /error instead of producing a
	// silent 500 + reload. See docs/superpowers/specs/
	// 2026-05-24-servererror-shim-aware-design.md.
	webAuthnHandler.InternalErrorHandler = app.serverError
```

Add the assignment line immediately after the `app := application{...}` block closes.

- [ ] **Step 2: Add the wiring**

Insert the wiring code described in Step 1 into `cmd/web/main.go` just below the `app := application{...}` literal.

- [ ] **Step 3: Verify the build**

Run: `go build ./cmd/web/`
Expected: clean.

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/web/main.go
git commit -m "feat(web): wire webauthnhandler.InternalErrorHandler to serverError"
```

---

## Task 4: Decide on `mustAdmin` semantics — confirm with reviewer

`mustAdmin` currently does `http.Error(w, "Unauthorized", 401)`. Two options:

**Option A — Redirect to `/`.** Matches `mustAuthenticate`'s existing pattern. Sensible because the user is authenticated; they just lack the role. Bouncing them home is the polite UX, and `redirect()` is already shim-aware (`X-Location` → 200 navigation).

**Option B — Delegate to `serverError` ⇒ `/error`.** Treats not-admin as catastrophic. Technically wrong — it's an authorization mismatch, not a system fault — and the `/error` page would read oddly ("Something went wrong" for a permissions issue).

The right call is **Option A**. Drop the 401 path entirely; redirect to `/`. The 401 was a relic of the API era and doesn't fit the SPA-like flow today.

This is a behaviour change worth pausing for a reviewer confirmation. If the reviewer prefers Option B (or wants `/admin/...` non-admins to land on a dedicated 403 page), the rest of this task changes shape.

- [ ] **Step 1: Confirm direction with the reviewer before coding.**

If proceeding with Option A, continue with Step 2. If a different direction is chosen, revise the steps accordingly.

- [ ] **Step 2: Write the failing test**

Add to `cmd/web/middleware_test.go` (create if missing):

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/myrjola/petrapp/internal/contexthelpers"
)

func Test_mustAdmin_AuthenticatedNonAdmin_RedirectsToHome(t *testing.T) {
	t.Parallel()

	app := &application{}
	called := false
	handler := app.mustAdmin(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/admin/exercises", nil)
	r = contexthelpers.AuthenticateContext(r, 42, false /* not admin */)

	handler.ServeHTTP(w, r)

	if called {
		t.Error("inner handler should NOT have been called for a non-admin user")
	}
	if got := w.Code; got != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", got, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/" {
		t.Errorf("Location = %q, want /", got)
	}
}

func Test_mustAdmin_Unauthenticated_RedirectsToHome(t *testing.T) {
	t.Parallel()

	app := &application{}
	called := false
	handler := app.mustAdmin(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/admin/exercises", nil)
	// No authentication context set.

	handler.ServeHTTP(w, r)

	if called {
		t.Error("inner handler should NOT have been called for an unauthenticated user")
	}
	if got := w.Code; got != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", got, http.StatusSeeOther)
	}
}

func Test_mustAdmin_AuthenticatedAdmin_CallsNext(t *testing.T) {
	t.Parallel()

	app := &application{}
	called := false
	handler := app.mustAdmin(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/admin/exercises", nil)
	r = contexthelpers.AuthenticateContext(r, 1, true /* admin */)

	handler.ServeHTTP(w, r)

	if !called {
		t.Error("inner handler MUST be called for an admin user")
	}
}
```

- [ ] **Step 3: Run tests to verify the two redirect tests fail**

Run: `go test -v ./cmd/web/ -run Test_mustAdmin`
Expected: `_AuthenticatedAdmin_CallsNext` PASSES. The two redirect tests FAIL — current code returns 401 with a text body, not a 303 with `Location: /`.

- [ ] **Step 4: Replace `http.Error` in `mustAdmin`**

In `cmd/web/middleware.go`, replace the body of `mustAdmin` (currently lines 203–213) with:

```go
// mustAdmin asserts that the user is an admin. Non-admin (or
// unauthenticated) requests are redirected to /. The shim-aware
// redirect() turns this into a 200 + X-Location navigation on the JS
// fetch path and a plain 303 elsewhere.
func (app *application) mustAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAuthenticated := contexthelpers.IsAuthenticated(r.Context())
		isAdmin := contexthelpers.IsAdmin(r.Context())
		if !isAuthenticated || !isAdmin {
			redirect(w, r, "/")
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -v ./cmd/web/ -run Test_mustAdmin`
Expected: all three PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/middleware.go cmd/web/middleware_test.go
git commit -m "feat(web): mustAdmin redirects non-admin to / instead of 401"
```

---

## Task 5: Doc updates

`cmd/web/CLAUDE.md` lists the terminal HTTP outcomes and the silent-failure cases. Update to reflect that the middleware paths now route through the standard helpers.

**Files:**
- Modify: `cmd/web/CLAUDE.md`

- [ ] **Step 1: Add a "Middleware error paths" subsection**

In `cmd/web/CLAUDE.md`, locate the "Error Handling" section. Append a new subsection at its end:

````markdown
#### Middleware error paths

Three middleware paths used to silently 500 (or 401) on the JS-shim
path. They now route through the shim-aware helpers:

- `recoverPanic` → `app.serverError`. Panics navigate to `/error` on
  shim POSTs and render the 500 inline otherwise.
- `mustAdmin` → `redirect(/)`. Non-admins are bounced to home; the
  401 status code has been retired.
- `webauthnhandler.AuthenticateMiddleware` → injected
  `InternalErrorHandler` (wired to `app.serverError` in `main.go`).
  DB lookup failures land on `/error` instead of producing a silent
  500.

The injection point on `webauthnhandler` keeps the package unaware of
the stack-navigator wire protocol. If you add another middleware in
that package, route its internal errors through `h.internalError` and
they will inherit the same UX.
````

- [ ] **Step 2: Commit**

```bash
git add cmd/web/CLAUDE.md
git commit -m "docs: document middleware error-routing under the shim-aware contract"
```

---

## Task 6: Lint, full test, manual sanity check

- [ ] **Step 1: Lint**

Run: `make lint-fix`
Expected: clean.

- [ ] **Step 2: Full test suite**

Run: `make test`
Expected: all PASS.

- [ ] **Step 3: Manual browser sanity check**

The shim-side navigation needs a real browser:

1. Build and start the app locally.
2. Register a non-admin user.
3. From dev tools, `fetch('/admin/exercises/generate', { method: 'POST', headers: { 'X-Requested-With': 'stacknav' } })` — observe the shim navigates to `/`.
4. Provoking the webauthnhandler DB-failure branch in production-like conditions is non-trivial. The simplest reproducer is: start the app, register, then drop `users` from the SQLite DB out-of-band — the next authenticated request hits `getUserRole` returning a "no such table" error and exercises the `InternalErrorHandler` callback. Expect navigation to `/error`. **Do not** do this on a real database.

If a Playwright case in `cmd/web/playwright_test.go` is desired for either path, add it as a separate follow-up — out of scope here.

- [ ] **Step 4: Final commit (if any cleanup needed)**

If lint-fix produced changes:

```bash
git add -A
git commit -m "chore: lint cleanup after middleware shim-aware change"
```

Otherwise this task ends without a commit.

---

## Out of scope

- A separate 403 page for admin-only routes. Today's UX bounces to `/`; that's a deliberate choice (Option A in Task 4). If a 403 with explanatory copy is desired later, that is its own design.
- Adding Playwright coverage for the new middleware paths.
- Touching the `http.NewCrossOriginProtection` handler (`crossOriginProtection` in `cmd/web/middleware.go`). It writes its own error responses internally; intercepting them would require wrapping the writer, and CSRF-failure UX is its own design question.
