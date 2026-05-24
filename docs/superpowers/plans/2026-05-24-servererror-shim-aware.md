# Shim-Aware `serverError` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `app.serverError` produce a visible, distinct UX on POSTs intercepted by the stack-navigator JS shim by navigating to a real `GET /error` page; narrow `app.userError` to delegate non-validation errors to `serverError` so predictable and catastrophic failures get different UX.

**Architecture:** `serverError` detects `X-Requested-With: stacknav` and replies `200 + X-Location: /error[?from=<sanitised>]` instead of `500 + body`, driving the existing shim's "navigate on 200" path. A new `app.errorGET` handler renders `error.gohtml` against a `From` field sanitised to same-origin paths. The error template's reload-button is replaced with a context-aware Back link. `userError` keeps `domain.ValidationError` on the banner path and delegates everything else to `serverError`. Wire protocol (`X-Location`, `X-Replace-Url`) is unchanged.

**Tech Stack:** Go 1.25+ (stdlib HTTP, `errors.As`, `url.Parse`/`url.QueryEscape`), existing `app.render` / template machinery, existing `e2etest` harness, `httptest` for unit tests.

**Spec:** [docs/superpowers/specs/2026-05-24-servererror-shim-aware-design.md](../specs/2026-05-24-servererror-shim-aware-design.md)

**Verification note:** the JS shim's reload-vs-navigate decision only runs in a real browser. The e2etest harness drives raw HTTP and can verify the *server side* of the new contract (status, headers, response body, follow-up GET rendering) but cannot prove the browser actually navigates. Treat e2etest as the source of truth for the wire contract; reserve Playwright (`cmd/web/playwright_test.go`) or manual browser testing for the end-to-end UX check after the plan lands.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `cmd/web/helpers.go` | modify | `serverError` body (shim-aware), `userError` body (delegate non-validation) |
| `cmd/web/handler-error.go` | create | `errorGET` handler + `errorTemplateData` struct + `sanitiseFromPath` helper |
| `cmd/web/handler-error_test.go` | create | unit tests for `sanitiseFromPath` + e2etest scenarios for `GET /error` |
| `cmd/web/helpers_test.go` | modify | add unit tests for `serverError` shim-aware path and `userError` delegation |
| `cmd/web/routes.go` | modify | register `GET /error` on `app.sessionStack` |
| `ui/templates/pages/error/error.gohtml` | modify | replace Retry button with context-aware "← Back" link |
| `cmd/web/handler-error-ux_test.go` | modify | flip "system trigger surfaces banner" assertion to "system trigger navigates to /error" |
| `cmd/web/handler-workout_test.go` | modify (add) | e2etest: stacknav-headed POST to `/workouts/{date}/complete` without `/start` produces `X-Location: /error?from=…` |
| `cmd/web/CLAUDE.md` | modify | update the Error Handling section to reflect the new `serverError`/`userError` split |
| `docs/superpowers/specs/2026-05-18-error-ux-conventions-design.md` | modify | add "Superseded by" pointer to the 2026-05-24 design for the class-C path |

The new file `handler-error.go` is intentionally small (one handler + one struct + one helper) so it can be reasoned about in isolation. Tests live next to it (`handler-error_test.go`) following the existing convention (`handler-error-ux.go` / `handler-error-ux_test.go`).

---

## Task 1: Sanitiser helper + unit tests

The `?from=` sanitiser is the trickiest piece — it gates the back-link rendering against same-origin paths. Build it first, test-first, in isolation. Subsequent tasks depend on it.

**Files:**
- Create: `cmd/web/handler-error.go`
- Create: `cmd/web/handler-error_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/web/handler-error_test.go`:

```go
package main

import "testing"

func Test_sanitiseFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "absolute path", in: "/workouts/2026-05-24", want: "/workouts/2026-05-24"},
		{name: "absolute path with query", in: "/workouts/2026-05-24?x=1", want: "/workouts/2026-05-24?x=1"},
		{name: "protocol-relative URL rejected", in: "//evil.example.com/foo", want: ""},
		{name: "absolute http URL rejected", in: "http://evil.example.com/foo", want: ""},
		{name: "relative path without slash rejected", in: "workouts/2026-05-24", want: ""},
		{name: "double-slash inside path rejected", in: "/foo//bar", want: ""},
		{name: "javascript scheme rejected", in: "javascript:alert(1)", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitiseFromPath(tc.in); got != tc.want {
				t.Errorf("sanitiseFromPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/web/ -run Test_sanitiseFromPath`
Expected: FAIL with `undefined: sanitiseFromPath`.

- [ ] **Step 3: Write minimal implementation**

Create `cmd/web/handler-error.go`:

```go
package main

import (
	"net/http"
	"strings"
)

// errorTemplateData feeds error.gohtml. From is a sanitised same-origin path
// the user came from, or "" when none was provided / it failed sanitisation.
type errorTemplateData struct {
	BaseTemplateData

	From string
}

// sanitiseFromPath returns the input if it is a same-origin path safe to use
// as an anchor href, or "" otherwise. Defence-in-depth against open-redirect
// vectors via the ?from= query parameter:
//
//   - must start with "/" (rejects relative paths and absolute URLs)
//   - must not start with "//" (rejects protocol-relative URLs that the
//     browser would resolve to a different origin)
//   - must not contain "//" anywhere (rejects "/path//evil.example.com")
//
// The query string is preserved verbatim — the user's original location may
// have legitimately carried one.
func sanitiseFromPath(s string) string {
	if s == "" || !strings.HasPrefix(s, "/") || strings.Contains(s, "//") {
		return ""
	}
	return s
}

func (app *application) errorGET(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusOK, "error", errorTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		From:             sanitiseFromPath(r.URL.Query().Get("from")),
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./cmd/web/ -run Test_sanitiseFromPath`
Expected: all subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/web/handler-error.go cmd/web/handler-error_test.go
git commit -m "feat(web): add sanitiseFromPath helper for /error back-link"
```

---

## Task 2: Wire `GET /error` route and render the error page with no `?from=`

`errorGET` is already implemented; this task wires it into the mux and proves the route serves the template.

**Files:**
- Modify: `cmd/web/routes.go`
- Modify: `cmd/web/handler-error_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/web/handler-error_test.go`:

```go
func Test_application_errorGET_noFromParam_rendersGoHomeOnly(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/error")
	if err != nil {
		t.Fatalf("Get /error: %v", err)
	}

	if doc.Find("h1:contains('Something went wrong')").Length() == 0 {
		t.Error("expected the error title on /error")
	}
	if doc.Find("a[href='/']:contains('Go Home')").Length() == 0 {
		t.Error("expected a Go Home link on /error")
	}
	// No ?from= ⇒ no back link.
	if doc.Find("a.error-back-link").Length() != 0 {
		t.Error("expected no back link when ?from= is absent")
	}
}
```

You'll need to add these imports if not already present in the file:

```go
import (
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/web/ -run Test_application_errorGET_noFromParam_rendersGoHomeOnly`
Expected: FAIL — the `/error` route isn't registered yet, so the server returns 404 (the file-server 404 page won't contain the "Something went wrong" h1).

- [ ] **Step 3: Register the route**

Modify `cmd/web/routes.go`. Locate the block where dev-only routes are registered (lines 80–84, the `/dev/error-ux*` routes). Immediately *before* the comment `// Home route (most specific)` (currently line 86), insert:

```go
	// Catastrophic-failure surface. Reached either by GET (a browser hitting
	// a stale link) or by the JS shim navigating after serverError on a POST.
	// Sits on sessionStack — must be reachable from authenticated and
	// unauthenticated states alike.
	mux.Handle("GET /error", app.sessionStack(http.HandlerFunc(app.errorGET)))

```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./cmd/web/ -run Test_application_errorGET_noFromParam_rendersGoHomeOnly`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/web/routes.go cmd/web/handler-error_test.go
git commit -m "feat(web): register GET /error route"
```

---

## Task 3: Render context-aware Back link in `error.gohtml`

Replace the Retry button (which only reloads `/error`) with a context-aware "← Back" link driven by `errorTemplateData.From`.

**Files:**
- Modify: `ui/templates/pages/error/error.gohtml`
- Modify: `cmd/web/handler-error_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/web/handler-error_test.go`:

```go
func Test_application_errorGET_withSafeFromParam_rendersBackLink(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/error?from=%2Fworkouts%2F2026-05-24")
	if err != nil {
		t.Fatalf("Get /error?from=...: %v", err)
	}

	link := doc.Find("a.error-back-link").First()
	if link.Length() == 0 {
		t.Fatal("expected a back link on /error when ?from= is a safe path")
	}
	if href, _ := link.Attr("href"); href != "/workouts/2026-05-24" {
		t.Errorf("back link href = %q, want %q", href, "/workouts/2026-05-24")
	}
	// Retry button is gone — its inline reload script bypassed CSP / Trusted
	// Types only because it lived in this template. Removing the button
	// removes the rationale; the back link is a plain anchor.
	if doc.Find(".error-actions button:contains('Retry')").Length() != 0 {
		t.Error("expected the legacy Retry button to be removed")
	}
}

func Test_application_errorGET_withUnsafeFromParam_omitsBackLink(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	// Protocol-relative URL — must be rejected by sanitiseFromPath.
	doc, err := client.GetDoc(ctx, "/error?from=%2F%2Fevil.example.com%2Ffoo")
	if err != nil {
		t.Fatalf("Get /error?from=//evil...: %v", err)
	}

	if doc.Find("a.error-back-link").Length() != 0 {
		t.Error("expected no back link when ?from= fails sanitisation")
	}
	// Page still renders.
	if doc.Find("h1:contains('Something went wrong')").Length() == 0 {
		t.Error("expected the error title to still render with unsafe ?from=")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./cmd/web/ -run 'Test_application_errorGET_with(Safe|Unsafe)FromParam'`
Expected: FAIL on `_withSafeFromParam_rendersBackLink` (the back link isn't rendered yet — neither the template nor `nil` data exposes `.From`). The retry-button-removed assertion will also fail since the template still has the Retry button.

- [ ] **Step 3: Update the template**

Replace `ui/templates/pages/error/error.gohtml` in full with:

```html
{{ define "page" }}
    <style {{ nonce }}>
        @scope {
            .error-container {
                display: flex;
                flex-direction: column;
                align-items: center;
                justify-content: center;
                min-height: 100dvh;
                padding: var(--size-4);
                text-align: center;
                background: var(--color-surface);
            }

            .error-content {
                max-width: var(--size-15);
                padding: var(--size-8) var(--size-6);
            }

            .error-title {
                font-size: var(--font-size-6);
                font-weight: var(--font-weight-7);
                color: var(--color-text-primary);
                margin-bottom: var(--size-4);
                line-height: var(--font-lineheight-1);
            }

            .error-message {
                font-size: var(--font-size-2);
                color: var(--color-text-secondary);
                margin-bottom: var(--size-6);
                line-height: var(--font-lineheight-3);
            }

            .error-actions {
                gap: var(--size-3);
                justify-content: center;
            }
        }
    </style>

    <div class="error-container">
        <div class="error-content card">
            <h1 class="error-title">Something went wrong</h1>
            <p class="error-message">
                We encountered an unexpected error while processing your request.
                Please try again or return to the home page.
            </p>
            <div class="error-actions cluster">
                {{ if .From }}
                    <a href="{{ .From }}" class="btn btn--ghost error-back-link">← Back</a>
                {{ end }}
                <a href="/" class="btn">Go Home</a>
            </div>
        </div>
    </div>
{{ end }}
```

Key changes:
- The Retry `<button>` plus its inline reload `<script>` is gone — `location.reload()` on `/error` is a no-op for any real recovery, and the inline script was the only CSP carve-out the template needed.
- A new `<a class="error-back-link">` is rendered only when `.From` is set. Class name is a unique selector for tests.

- [ ] **Step 4: Handle the `nil` data path for the existing 500 fallback**

`serverError` currently calls `app.render(w, r, http.StatusInternalServerError, "error", nil)`. With the template now reading `.From`, passing `nil` would panic on `{{ if .From }}`.

Modify `cmd/web/helpers.go`. Replace the body of `serverError` with the version that handles both branches; we'll do this fully in Task 4 but for the template to keep rendering on the GET path *right now*, change the `nil` arg to an empty struct.

In `cmd/web/helpers.go`, in the existing `serverError`, change:

```go
app.render(w, r, http.StatusInternalServerError, "error", nil)
```

to:

```go
app.render(w, r, http.StatusInternalServerError, "error", errorTemplateData{
	BaseTemplateData: newBaseTemplateData(r),
})
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test -v ./cmd/web/ -run 'Test_application_errorGET'`
Expected: all three `Test_application_errorGET_*` subtests PASS.

Also run the full package to check nothing regressed:

Run: `go test ./cmd/web/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/error/error.gohtml cmd/web/helpers.go cmd/web/handler-error_test.go
git commit -m "feat(web): replace Retry button with context-aware back link on /error"
```

---

## Task 4: Make `serverError` shim-aware (the core change)

Now wire the actual shim-aware navigation into `serverError`. Test-first.

**Files:**
- Modify: `cmd/web/helpers.go`
- Modify: `cmd/web/helpers_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/web/helpers_test.go`:

```go
func Test_serverError_StackNavRequest_NavigatesToErrorPage(t *testing.T) {
	t.Parallel()

	app := &application{logger: slog.New(slog.DiscardHandler)}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/workouts/2026-05-24/complete", nil)
	r.Header.Set("X-Requested-With", "stacknav")
	r.Header.Set("Referer", "https://example.test/workouts/2026-05-24")
	r.Host = "example.test"

	app.serverError(w, r, errors.New("boom"))

	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
	want := "/error?from=%2Fworkouts%2F2026-05-24"
	if got := w.Header().Get("X-Location"); got != want {
		t.Errorf("X-Location = %q, want %q", got, want)
	}
	if got := w.Body.Len(); got != 0 {
		t.Errorf("body length = %d, want 0", got)
	}
}

func Test_serverError_StackNavRequest_CrossOriginReferer_OmitsFrom(t *testing.T) {
	t.Parallel()

	app := &application{logger: slog.New(slog.DiscardHandler)}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/workouts/2026-05-24/complete", nil)
	r.Header.Set("X-Requested-With", "stacknav")
	r.Header.Set("Referer", "https://other.example/workouts/2026-05-24")
	r.Host = "example.test"

	app.serverError(w, r, errors.New("boom"))

	if got := w.Header().Get("X-Location"); got != "/error" {
		t.Errorf("X-Location = %q, want /error (cross-origin Referer should be dropped)", got)
	}
}

func Test_serverError_StackNavRequest_NoReferer_BareErrorPath(t *testing.T) {
	t.Parallel()

	app := &application{logger: slog.New(slog.DiscardHandler)}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/workouts/2026-05-24/complete", nil)
	r.Header.Set("X-Requested-With", "stacknav")

	app.serverError(w, r, errors.New("boom"))

	if got := w.Header().Get("X-Location"); got != "/error" {
		t.Errorf("X-Location = %q, want /error", got)
	}
}

func Test_serverError_NonStackNavRequest_Renders500Body(t *testing.T) {
	t.Parallel()

	// This case uses the real template renderer, so set up the template FS.
	app := newTestApplicationForTemplateRender(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/something", nil)

	app.serverError(w, r, errors.New("boom"))

	if got := w.Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", got, http.StatusInternalServerError)
	}
	if !strings.Contains(w.Body.String(), "Something went wrong") {
		t.Errorf("expected the error page body, got %q", w.Body.String())
	}
}
```

Required imports to add to `helpers_test.go`:

```go
import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)
```

And a test helper that wires up `app.templateFS`, `app.parsedTemplates`, and `app.devMode`. Look for an existing one in the package — if there is none, add this to `helpers_test.go`:

```go
// newTestApplicationForTemplateRender returns a minimal *application ready to
// render templates against the on-disk ui/templates tree. Used by tests that
// exercise the non-stacknav serverError path which renders error.gohtml.
func newTestApplicationForTemplateRender(t *testing.T) *application {
	t.Helper()
	templatePath, err := resolveAndVerifyTemplatePath("")
	if err != nil {
		t.Fatalf("resolve template path: %v", err)
	}
	return &application{
		logger:          slog.New(slog.DiscardHandler),
		templateFS:      os.DirFS(templatePath),
		parsedTemplates: newTemplateCache(),
		devMode:         true,
	}
}
```

Add `"os"` to the imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./cmd/web/ -run 'Test_serverError'`
Expected: the three `_StackNavRequest_*` tests FAIL because `serverError` doesn't yet read `X-Requested-With` — it renders the body unconditionally. The `_NonStackNavRequest_Renders500Body` test should PASS already (existing behaviour).

- [ ] **Step 3: Update `serverError`**

In `cmd/web/helpers.go`, replace the existing `serverError` with:

```go
func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.LogAttrs(r.Context(), slog.LevelError, "server error", slog.Any("error", err))

	if r.Header.Get("X-Requested-With") == "stacknav" {
		// Drive the shim's "200 + X-Location ⇒ navigate" path so the user
		// sees the error page instead of a silent reload on the form page.
		// Referer is a UX hint only — sanitised same-origin path becomes a
		// "← Back" link on the error page, cross-origin / missing is fine.
		target := "/error"
		if from := r.Referer(); from != "" {
			if u, parseErr := url.Parse(from); parseErr == nil && u.Host == r.Host {
				target = "/error?from=" + url.QueryEscape(u.Path)
			}
		}
		w.Header().Set("X-Location", target)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Non-shim path: render the error page inline with a 500. This is the
	// path for GET handlers, curl, and no-JS browsers.
	app.render(w, r, http.StatusInternalServerError, "error", errorTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
	})
}
```

Add `"net/url"` to the imports in `cmd/web/helpers.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./cmd/web/ -run 'Test_serverError'`
Expected: all four PASS.

Then run the full package to verify nothing else regressed:

Run: `go test ./cmd/web/`
Expected: PASS — except possibly the existing `handler-error-ux_test.go::Test_application_devErrorUX_triggerSystem_surfacesGenericMessage` test, which assumed the system error path produced a banner. That test gets fixed in Task 6.

If `Test_application_devErrorUX_triggerSystem_surfacesGenericMessage` fails, that's expected — leave it red and proceed; Task 6 fixes it.

- [ ] **Step 5: Add a `recoverPanic` regression test**

`recoverPanic` (`cmd/web/middleware.go:177`) calls `serverError` on any caught panic. The shim-aware change applies transparently, but the spec's test plan calls out a specific regression test for this. Append to `cmd/web/helpers_test.go`:

```go
func Test_recoverPanic_StackNavRequest_NavigatesToErrorPage(t *testing.T) {
	t.Parallel()

	app := &application{logger: slog.New(slog.DiscardHandler)}
	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("simulated handler panic")
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
	r.Header.Set("X-Requested-With", "stacknav")
	r.Header.Set("Referer", "https://example.test/some/page")
	r.Host = "example.test"

	app.recoverPanic(panicking).ServeHTTP(w, r)

	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
	want := "/error?from=%2Fsome%2Fpage"
	if got := w.Header().Get("X-Location"); got != want {
		t.Errorf("X-Location = %q, want %q", got, want)
	}
}
```

Run: `go test -v ./cmd/web/ -run Test_recoverPanic_StackNavRequest`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/helpers.go cmd/web/helpers_test.go
git commit -m "feat(web): make serverError shim-aware on POST"
```

---

## Task 5: End-to-end test through `workoutCompletePOST`

Prove the wire contract with a full e2etest scenario: register, POST to `/workouts/{today}/complete` without `/start`, assert `X-Location: /error?from=...`. This is the "real" silent-failure case the design is built around.

**Files:**
- Modify: `cmd/web/handler-workout_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/web/handler-workout_test.go`:

```go
// Test_application_workoutCompletePOST_unstartedSession_navigatesToErrorPage
// covers the canonical silent-failure case from the 2026-05-24 shim-aware
// design. POST /workouts/{today}/complete before /start returns
// domain.ErrNotStarted from Session.Complete, which workoutCompletePOST
// passes to serverError. With the shim header set, serverError must reply
// 200 + X-Location: /error?from=/workouts/{today} so the JS shim navigates
// the user to the error page instead of silently reloading the workout.
func Test_application_workoutCompletePOST_unstartedSession_navigatesToErrorPage(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Schedule today so a session exists. We deliberately do NOT call /start.
	formData := map[string]string{time.Now().Weekday().String(): "60"}
	prefsDoc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if _, err = client.SubmitForm(ctx, prefsDoc, "/preferences", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	target := server.URL() + "/workouts/" + today + "/complete"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "stacknav")
	req.Header.Set("Referer", server.URL()+"/workouts/"+today)

	httpClient := *client.HTTPClient() // shallow copy preserves jar.
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST /complete: %v", err)
	}
	if cerr := resp.Body.Close(); cerr != nil {
		t.Fatalf("Close response body: %v", cerr)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (shim-aware serverError)", resp.StatusCode)
	}
	wantLoc := "/error?from=%2Fworkouts%2F" + today
	if got := resp.Header.Get("X-Location"); got != wantLoc {
		t.Errorf("X-Location = %q, want %q", got, wantLoc)
	}
}

// Test_application_workoutCompletePOST_unstartedSession_noShimHeader_500s
// covers the same scenario without the shim header — a curl-style client.
// serverError should fall through to the 500 + error.gohtml body path.
func Test_application_workoutCompletePOST_unstartedSession_noShimHeader_500s(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	formData := map[string]string{time.Now().Weekday().String(): "60"}
	prefsDoc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if _, err = client.SubmitForm(ctx, prefsDoc, "/preferences", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	target := server.URL() + "/workouts/" + today + "/complete"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No X-Requested-With.

	httpClient := *client.HTTPClient()
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST /complete: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (non-shim path)", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "Something went wrong") {
		t.Errorf("expected the error page body, got %q", string(body))
	}
}
```

Required imports if not already present: `"io"`, `"net/http"`, `"strings"`, `"time"`, plus the existing `e2etest` and `testhelpers` imports.

- [ ] **Step 2: Run the tests**

Run: `go test -v ./cmd/web/ -run 'Test_application_workoutCompletePOST_unstartedSession'`
Expected: both PASS — `serverError` is already shim-aware from Task 4 and the no-shim path already 500s.

If either fails, do not proceed; the wire contract from Task 4 is not actually being exercised against `workoutCompletePOST`. Investigate before continuing.

- [ ] **Step 3: Commit**

```bash
git add cmd/web/handler-workout_test.go
git commit -m "test(web): e2e coverage for serverError navigation through workoutCompletePOST"
```

---

## Task 6: Narrow `userError` — delegate non-validation to `serverError`

`userError` currently flashes a generic message and redirects on any non-`ValidationError`. After Task 4, that produces a banner on the safe URL even though `serverError` would now produce a more accurate "catastrophic" UX. Delegate.

This task changes behaviour: the existing `Test_application_devErrorUX_triggerSystem_surfacesGenericMessage` test (which asserts a banner appears after a non-validation `userError` call) must be rewritten to expect navigation to `/error`.

**Files:**
- Modify: `cmd/web/helpers.go`
- Modify: `cmd/web/helpers_test.go`
- Modify: `cmd/web/handler-error-ux_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `cmd/web/helpers_test.go`:

```go
func Test_userError_ValidationError_FlashesAndRedirects(t *testing.T) {
	t.Parallel()

	app := &application{
		logger:         slog.New(slog.DiscardHandler),
		sessionManager: newTestSessionManager(t),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
	r = r.WithContext(app.sessionManager.Load(r.Context(), ""))

	ve := domain.ValidationError{Message: "Name must be 1–50 characters."}
	app.userError(w, r, ve, "/safe")

	if got := w.Code; got != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", got, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/safe" {
		t.Errorf("Location = %q, want /safe", got)
	}
	// The flash must be populated for the safe URL's GET to surface the banner.
	if got := app.popFlashError(r.Context()); got != "Name must be 1–50 characters." {
		t.Errorf("flash = %q, want validation message", got)
	}
}

func Test_userError_NonValidation_StackNav_DelegatesToServerError(t *testing.T) {
	t.Parallel()

	app := &application{
		logger:         slog.New(slog.DiscardHandler),
		sessionManager: newTestSessionManager(t),
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/workouts/2026-05-24/add-exercise", nil)
	r.Header.Set("X-Requested-With", "stacknav")
	r.Header.Set("Referer", "https://example.test/workouts/2026-05-24")
	r.Host = "example.test"
	r = r.WithContext(app.sessionManager.Load(r.Context(), ""))

	app.userError(w, r, errors.New("db hiccup"), "/workouts/2026-05-24")

	// Shim path: serverError handles it.
	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d (delegation to serverError shim path)", got, http.StatusOK)
	}
	want := "/error?from=%2Fworkouts%2F2026-05-24"
	if got := w.Header().Get("X-Location"); got != want {
		t.Errorf("X-Location = %q, want %q", got, want)
	}
	// Flash must NOT be populated — the banner-on-safe-URL UX is intentionally
	// abandoned for non-validation errors.
	if got := app.popFlashError(r.Context()); got != "" {
		t.Errorf("flash = %q, want empty (non-validation must not flash)", got)
	}
}
```

`newTestSessionManager` may need to be created — look for `sessionManager` test fixtures in the package first. If none exist, add:

```go
// newTestSessionManager builds an in-memory scs session manager for tests
// that need to round-trip flash messages. The session is not persisted;
// each test gets a fresh empty store.
func newTestSessionManager(t *testing.T) *scs.SessionManager {
	t.Helper()
	sm := scs.New()
	sm.Store = memstore.New()
	return sm
}
```

with imports `"github.com/alexedwards/scs/v2"` and `"github.com/alexedwards/scs/v2/memstore"`. Verify both packages are in `go.mod`; if not, find how the production code constructs `sessionManager` (look at `cmd/web/main.go`) and mirror that.

Add `"github.com/myrjola/petrapp/internal/domain"` to the imports too.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./cmd/web/ -run 'Test_userError'`
Expected: `_ValidationError_FlashesAndRedirects` PASSES (existing behaviour). `_NonValidation_StackNav_DelegatesToServerError` FAILS — current `userError` flashes a generic message and 303s to `/workouts/...` instead of producing the shim-aware navigation to `/error`.

- [ ] **Step 3: Update `userError`**

In `cmd/web/helpers.go`, replace the existing `userError` with:

```go
// userError surfaces a failure of an in-flight user action.
//
// Routing:
//   - domain.ValidationError → flash with ve.Message and redirect to safeURL.
//     The safe URL's GET handler pops the flash and renders the banner.
//   - any other error → delegate to serverError. On the shim path that
//     navigates the user to /error (catastrophic-failure UX); on the
//     non-shim path it renders error.gohtml with a 500.
//
// safeURL is only used on the validation branch. It must point at a GET
// handler known to render successfully AND that pops + renders the flash.
// See cmd/web/CLAUDE.md "userError semantics" for the rationale and the
// list of currently-supported safe URLs.
func (app *application) userError(
	w http.ResponseWriter, r *http.Request, err error, safeURL string,
) {
	var ve domain.ValidationError
	if errors.As(err, &ve) {
		app.putFlashError(r.Context(), ve.Message)
		redirect(w, r, safeURL)
		return
	}
	app.serverError(w, r, err)
}
```

The doc comment changes too — the table in CLAUDE.md will follow in Task 8.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./cmd/web/ -run 'Test_userError'`
Expected: both PASS.

- [ ] **Step 5: Update the now-stale dev-error-ux test**

The test `Test_application_devErrorUX_triggerSystem_surfacesGenericMessage` in `handler-error-ux_test.go:124` expects a banner on the dev page after the `system` trigger. Under the new contract, the system trigger should navigate to `/error?from=/dev/error-ux` instead.

Replace the test in `cmd/web/handler-error-ux_test.go` (lines 124–152) with:

```go
func Test_application_devErrorUX_triggerSystem_navigatesToErrorPage(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	// GET first so we have a session + a Referer that resolves to /dev/error-ux.
	if _, err = client.GetDoc(ctx, "/dev/error-ux"); err != nil {
		t.Fatalf("Failed to get /dev/error-ux: %v", err)
	}

	// POST the system trigger directly with the stacknav header so we exercise
	// the shim wire contract. SubmitForm follows redirects; here we want to
	// observe the X-Location header on the response, not the followed page.
	target := server.URL() + "/dev/error-ux/trigger/system"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "stacknav")
	req.Header.Set("Referer", server.URL()+"/dev/error-ux")

	httpClient := *client.HTTPClient()
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST system trigger: %v", err)
	}
	if cerr := resp.Body.Close(); cerr != nil {
		t.Fatalf("Close response body: %v", cerr)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	want := "/error?from=%2Fdev%2Ferror-ux"
	if got := resp.Header.Get("X-Location"); got != want {
		t.Errorf("X-Location = %q, want %q", got, want)
	}
}
```

- [ ] **Step 6: Run the full package**

Run: `go test ./cmd/web/`
Expected: PASS across the board.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/helpers.go cmd/web/helpers_test.go cmd/web/handler-error-ux_test.go
git commit -m "feat(web): userError delegates non-validation errors to serverError"
```

---

## Task 7: Update the dev/error-ux page copy

The dev showcase page (`/dev/error-ux`) describes the "Unexpected system error" card as producing a banner with a generic message. That description is now wrong — under the new contract it produces a navigation to `/error`. The card text and the trigger button label should change too. Keep the change purely textual; the trigger handler itself already routes through `userError`, which now does the right thing.

**Files:**
- Modify: `ui/templates/pages/error-ux/error-ux.gohtml`
- Modify: `cmd/web/handler-error-ux_test.go`

- [ ] **Step 1: Inspect the existing template**

Read `ui/templates/pages/error-ux/error-ux.gohtml` to locate the "Unexpected system error" card. Note the exact copy currently in place.

- [ ] **Step 2: Update the card copy**

Within `ui/templates/pages/error-ux/error-ux.gohtml`, find the card with heading "Unexpected system error". Update its descriptive paragraph to reflect the new behaviour. The replacement copy:

> A non-validation error from a service call. Falls through `userError` to `serverError`, which on a JS-shim POST navigates to the `/error` catastrophic-failure page. Without the shim (curl, no-JS), the same path renders `error.gohtml` with a 500.

Leave the trigger button's `action` (`/dev/error-ux/trigger/system`) unchanged.

- [ ] **Step 3: Update the existing render-list test**

The test `Test_application_devErrorUX_render` at `handler-error-ux_test.go:23` lists the heading `"Unexpected system error"` — keep that exact heading. No code change needed unless the heading itself changes; do not rename the heading.

- [ ] **Step 4: Run tests**

Run: `go test ./cmd/web/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/error-ux/error-ux.gohtml
git commit -m "docs(web): refresh /dev/error-ux copy for new system-error UX"
```

---

## Task 8: Update CLAUDE.md and supersede the prior design

The `cmd/web/CLAUDE.md` Error Handling section was sharpened in the previous branch; it now needs the final pass to reflect the actual `userError` semantics (delegates to `serverError`) and the new `serverError` contract.

**Files:**
- Modify: `cmd/web/CLAUDE.md`
- Modify: `docs/superpowers/specs/2026-05-18-error-ux-conventions-design.md`

- [ ] **Step 1: Update the `serverError` table row**

In `cmd/web/CLAUDE.md`, the table at line 142–144 currently reads:

```markdown
| `app.serverError(w, r, err)` | GET failures, or the POST escape hatch when the safe URL itself is broken | Logs and renders `error.gohtml` 500 |
```

Replace with:

```markdown
| `app.serverError(w, r, err)` | Catastrophic failures: panics, template render errors, escape hatch when no safe URL fits | Logs. On a shim POST navigates to `/error?from=<sanitised>`; on GET / non-shim renders `error.gohtml` with 500 |
```

- [ ] **Step 2: Replace the "Why serverError on POST is silent failure" subsection**

The subsection currently spans lines 146–162. Replace the entire subsection with:

````markdown
#### `serverError` on POST navigates to `/error`

The stack-navigator JS shim (`ui/static/main.js`) intercepts every form
POST, replays it via `fetch`, and on any non-200 response calls
`location.reload()` — CSP / Trusted Types block injecting the response
body in place. Historically that meant `serverError` on a POST produced
a silent failure: reload landed on the form page with no flash, no
banner.

Today `serverError` detects the `X-Requested-With: stacknav` header and,
on the shim path, replies `200 + X-Location: /error[?from=…]` instead
of a 500 body. The shim navigates to `/error`, which renders a
catastrophic-failure page with a "← Back" link (when the originating
path was same-origin) and Go Home. On a curl / no-JS request the same
helper falls through to the inline 500 + `error.gohtml` body — the
browser renders it directly.

Use `serverError` when there is no safe URL to flash + redirect to:
panics caught by `recoverPanic`, template render failures, and the
escape hatches inside helpers like `parseForm`. Use `userError` for
in-flight POST failures where validation needs a banner on a known-good
GET handler — it routes the validation case to the banner and
delegates everything else back to `serverError`.
````

- [ ] **Step 3: Replace the "userError semantics" subsection**

The subsection currently spans lines 166–178. Replace with:

````markdown
#### `userError` semantics

`userError` routes by error type:

- `errors.As(err, &ve)` matching `domain.ValidationError` → flash with
  `ve.Message` verbatim and `redirect(safeURL)`. The form's GET handler
  pops the flash with `app.popFlashError(...)` and renders the
  `banner` component.
- Anything else → delegate to `serverError`. The non-validation case
  produces the catastrophic-failure UX (shim navigation to `/error`,
  or inline 500 for non-shim clients). `userError` does not
  flash a generic "Couldn't complete that action" message any more —
  banner UX is reserved for failures the user can act on by adjusting
  their input.

`safeURL` is only consulted on the validation branch. It must point at
a GET handler known to render successfully AND that pops + renders the
flash banner. See the `safeURL` requirements below.
````

- [ ] **Step 4: Update the "Existing handlers may still use inline" block**

The block at lines 195–204 still references the silent-failure case. Replace its body with:

````markdown
> Go-forward convention for new and migrating handlers. Existing
> handlers predate `userError` and may still use the inline
> `errors.As(&ve) { putFlashError(ve.Message); redirect(formURL) }`
> + trailing `app.serverError(w, r, err)` pattern. Under the
> shim-aware `serverError`, that inline pattern is now
> *functionally equivalent* to calling `userError` — both route
> `ValidationError` to the banner and non-validation to the
> catastrophic-failure page. Migrate opportunistically when next
> touching the handler; there is no UX gap remaining.
````

- [ ] **Step 5: Add "Superseded by" note on the prior design**

In `docs/superpowers/specs/2026-05-18-error-ux-conventions-design.md`, locate the front-matter section near the top (the lines stating Status / Scope). Immediately after the Status line, add:

```markdown
**Class-C update:** the "silent failure when handlers stay on `serverError`"
note in this design has been superseded by
[2026-05-24 Shim-Aware `serverError`](2026-05-24-servererror-shim-aware-design.md).
Class C now navigates to `/error` instead of producing a silent reload.
```

If there is no existing Status line, add this note at the very top of the document directly under the title.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/CLAUDE.md docs/superpowers/specs/2026-05-18-error-ux-conventions-design.md
git commit -m "docs: document shim-aware serverError + userError delegation"
```

---

## Task 9: Lint, full test, manual sanity check

- [ ] **Step 1: Lint**

Run: `make lint-fix`
Expected: clean.

- [ ] **Step 2: Full test suite**

Run: `make test`
Expected: all PASS.

If anything outside the planned touch-list fails, do not paper over it — check whether the failure is downstream of the `userError` semantic change (a test elsewhere may have asserted a banner on a non-validation path) and either:
- Update the test if the new behaviour is correct (banner → /error navigation).
- Roll back if there's a real regression you didn't anticipate.

- [ ] **Step 3: Manual browser sanity check**

The e2etest layer does not drive the JS shim — it submits raw HTTP. To verify the shim actually navigates correctly after the new wire contract:

1. `make ci` to ensure a clean build.
2. Start the app locally (the usual dev recipe).
3. Register a user, schedule today.
4. From the workout page, open dev tools, then `fetch('/workouts/<today>/complete', { method: 'POST', headers: { 'X-Requested-With': 'stacknav' } })` — observe the shim navigates to `/error?from=/workouts/<today>`.
5. Verify the "← Back" link points at the workout page.
6. Click "Go Home" — lands on `/`.
7. Hit `/dev/error-ux` and click the "Unexpected system error" trigger — observe the navigation to `/error?from=/dev/error-ux`.

This step is **manual** and not automatable in the present test harness; an alternative is to add a Playwright case in `cmd/web/playwright_test.go` — out of scope for this plan, tracked separately if desired.

- [ ] **Step 4: Final commit (if any cleanup needed)**

If lint-fix produced any changes that aren't yet committed:

```bash
git add -A
git commit -m "chore: lint cleanup after serverError shim-aware change"
```

If not, this task ends without a commit.

---

## Out of scope (handled in Plan 2)

The middleware paths `webauthnhandler.AuthenticateMiddleware` and `mustAdmin` still use `http.Error` and produce the same silent-failure UX they did before this plan. They get the same treatment in `docs/superpowers/plans/2026-05-24-middleware-shim-aware.md` — separate review because the plumbing decision is non-trivial.

`workoutCompletePOST` and any other handler still using inline `errors.As(&ve) { … } app.serverError(err)` are now functionally equivalent to `userError`. Migration to `userError` is code-tidiness, not a UX fix, and is opportunistic when next touching the handler — not blocked behind this plan.
