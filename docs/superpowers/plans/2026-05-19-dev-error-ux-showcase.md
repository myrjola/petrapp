# `/dev/error-ux` Showcase Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dev-only route at `/dev/error-ux` that exercises the four error-surfacing classes (validation, business, system, network) plus the full-page 500 path; surface discovery links to all dev routes on the home page when `app.devMode` is true.

**Architecture:** New page modelled on the existing `/dev/styleguide` (handler-side `if !app.devMode { 404 }` gating, scoped `<style>` per page, no new shared components). Live demos call into the canonical `app.userError` / `app.serverError` helpers so the banner that appears is the real flash-and-redirect path, not a screenshot. Home-page discovery wires through `homeTemplateData.DevMode` and renders in both the authenticated `week-eyebrow` and the unauthenticated landing footer.

**Tech Stack:** Go stdlib `net/http` (ServeMux Go 1.22+ method+pattern syntax), `html/template`, SQLite via `internal/sqlite`, `internal/e2etest` for handler tests, goquery for DOM assertions, `golangci-lint` (already installed by `make init`).

**Spec:** `docs/superpowers/specs/2026-05-19-dev-error-ux-showcase-design.md`

---

## File structure

- `cmd/web/handler-error-ux.go` (new) — three handlers (`devErrorUXGET`, `devErrorUXTriggerPOST`, `devErrorUXServerErrorGET`) and the `errorUXTemplateData` struct.
- `cmd/web/handler-error-ux_test.go` (new) — `e2etest`-driven coverage: render, gating, trigger dispatch, unknown-kind 404, home-link presence/absence.
- `cmd/web/routes.go` (modify) — register the three routes under `sessionStack`, alongside `/dev/styleguide`.
- `cmd/web/handler-home.go` (modify) — add `DevMode bool` to `homeTemplateData`, populate from `app.devMode`.
- `ui/templates/pages/error-ux/error-ux.gohtml` (new) — `{{ define "page" }}` with the six sections (5 live cards + banner-variants reference).
- `ui/templates/pages/home/schedule.gohtml` (modify) — eyebrow gains a `Styleguide · Error UX · ` block when `.DevMode`.
- `ui/templates/pages/home/unauthenticated.gohtml` (modify) — footer gains the same two links when `.DevMode`.

Each file has one responsibility. The handler file owns dispatch; the template owns presentation; route wiring is isolated to the single block in `routes.go`. The home-page edits touch a single conditional each — no restructuring.

---

## Task 1: Handler skeleton with dev-mode gating

**Files:**
- Create: `cmd/web/handler-error-ux.go`
- Modify: `cmd/web/routes.go`
- Create: `cmd/web/handler-error-ux_test.go`

- [ ] **Step 1: Write the failing render + gating tests**

Add to `cmd/web/handler-error-ux_test.go`:

```go
package main

import (
	"net/http"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// prodLookupEnv returns the same values as testLookupEnv but also sets
// FLY_APP_NAME so app.devMode becomes false. Uses a "pr-" prefix so the
// VAPID-keys check generates an ephemeral pair instead of failing.
func prodLookupEnv(key string) (string, bool) {
	if key == "FLY_APP_NAME" {
		return "pr-test", true
	}
	return testLookupEnv(key)
}

func Test_application_devErrorUX_render(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/dev/error-ux")
	if err != nil {
		t.Fatalf("Failed to get /dev/error-ux: %v", err)
	}

	if doc.Find("h2:contains('Server-side validation error')").Length() == 0 {
		t.Error("expected card heading on /dev/error-ux")
	}
}

func Test_application_devErrorUX_gated_outside_dev_mode(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), prodLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	for _, path := range []string{
		"/dev/error-ux",
		"/dev/error-ux/server-error",
	} {
		resp, getErr := server.Client().Get(t.Context(), path)
		if getErr != nil {
			t.Fatalf("Failed to GET %s: %v", path, getErr)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s in non-dev mode: got status %d, want 404", path, resp.StatusCode)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/web/ -run 'Test_application_devErrorUX_(render|gated)' -count=1 -v`
Expected: FAIL — handler `devErrorUXGET` not defined; template `error-ux` not found; route not registered.

- [ ] **Step 3: Create the handler file with the GET + server-error handlers**

Write `cmd/web/handler-error-ux.go`:

```go
package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/myrjola/petrapp/internal/domain"
)

const devErrorUXPath = "/dev/error-ux"

type errorUXTemplateData struct {
	BaseTemplateData
	Flash          BannerData
	BannerVariants []BannerData
}

// devErrorUXGET renders the live catalog of the four error-surfacing classes
// documented in docs/superpowers/specs/2026-05-18-error-ux-conventions-design.md.
// Wired in routes.go only when app.devMode is true; returns 404 otherwise.
func (app *application) devErrorUXGET(w http.ResponseWriter, r *http.Request) {
	if !app.devMode {
		http.NotFound(w, r)
		return
	}

	flash := BannerData{Variant: "error", Message: app.popFlashError(r.Context())}
	data := errorUXTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Flash:            flash,
		BannerVariants: []BannerData{
			{Variant: "error", Message: "Something went wrong. Please try again."},
			{Variant: "success", Message: "Your changes have been saved."},
			{Variant: "info", Message: "Heads up — this is informational."},
		},
	}
	app.render(w, r, http.StatusOK, "error-ux", data)
}

// devErrorUXTriggerPOST dispatches on the {kind} path parameter and routes
// the resulting error through app.userError so the live banner appears on
// the next GET of /dev/error-ux. Unknown kinds return 404 — there is no
// graceful UX path for an unrecognised demo trigger.
func (app *application) devErrorUXTriggerPOST(w http.ResponseWriter, r *http.Request) {
	if !app.devMode {
		http.NotFound(w, r)
		return
	}

	var err error
	switch r.PathValue("kind") {
	case "validation":
		err = domain.ValidationError{Message: "Name must be 1–50 characters."}
	case "business":
		err = domain.ValidationError{
			Message: "This day has no planned workout. Schedule one from the home page first.",
		}
	case "system":
		err = fmt.Errorf("simulated system fault: %w", io.ErrUnexpectedEOF)
	default:
		http.NotFound(w, r)
		return
	}
	app.userError(w, r, err, devErrorUXPath)
}

// devErrorUXServerErrorGET exists to demonstrate the rare class E path:
// app.serverError renders the full-page 500 directly because no safe URL
// exists. Hit via a regular anchor on /dev/error-ux.
func (app *application) devErrorUXServerErrorGET(w http.ResponseWriter, r *http.Request) {
	if !app.devMode {
		http.NotFound(w, r)
		return
	}
	app.serverError(w, r, fmt.Errorf("simulated full-page server error: %w", io.ErrUnexpectedEOF))
}
```

- [ ] **Step 4: Wire the routes**

Edit `cmd/web/routes.go`. Find the block:

```go
	// Developer-only design-token reference. Gated inside the handler on app.devMode
	// so prod returns 404; route is registered unconditionally to keep startup simple.
	mux.Handle("GET /dev/styleguide", app.sessionStack(http.HandlerFunc(app.styleguideGET)))
```

Add immediately after:

```go

	// Developer-only error-UX showcase. Same dev-mode gating as /dev/styleguide.
	// See docs/superpowers/specs/2026-05-19-dev-error-ux-showcase-design.md.
	mux.Handle("GET /dev/error-ux", app.sessionStack(http.HandlerFunc(app.devErrorUXGET)))
	mux.Handle("POST /dev/error-ux/trigger/{kind}",
		app.sessionStack(http.HandlerFunc(app.devErrorUXTriggerPOST)))
	mux.Handle("GET /dev/error-ux/server-error",
		app.sessionStack(http.HandlerFunc(app.devErrorUXServerErrorGET)))
```

- [ ] **Step 5: Build and verify compile**

Run: `go build ./...`
Expected: no output (success).

The render test still fails (template missing) — that's expected, Task 3 builds it. The gating test should now compile.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/handler-error-ux.go cmd/web/handler-error-ux_test.go cmd/web/routes.go
git commit -m "web: dev/error-ux handler skeleton with devMode gating"
```

---

## Task 2: Template — error-ux page

**Files:**
- Create: `ui/templates/pages/error-ux/error-ux.gohtml`

- [ ] **Step 1: Write the template**

The page renders six sections: a header, the flash banner, five live-trigger cards (validation, business, system, network, full-page 500), and the banner-variants reference.

Conventions to follow (from `ui/templates/CLAUDE.md`):
- `{{- /*gotype: github.com/myrjola/petrapp/cmd/web.errorUXTemplateData*/ -}}` at the top so JetBrains can type-check the dot.
- One `{{ define "page" }}` block; `app.render` resolves it.
- Scoped styles go inside `<style {{ nonce }}>` next to the markup they style; no inline `style="..."` attributes.
- Use the `banner` partial via `{{ template "banner" . }}` — never re-emit banner HTML.
- The simulated-network-failure button populates `#js-flash` via `textContent` and toggles `hidden`. No `innerHTML`, no script-URL sinks.
- Forms POST to `/dev/error-ux/trigger/{kind}` — each kind gets its own form `action` (path parameter), so there are no duplicate-action forms on the page that would confuse `e2etest.Client.SubmitForm`.

Write `ui/templates/pages/error-ux/error-ux.gohtml`:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.errorUXTemplateData*/ -}}

{{ define "page" }}
    <main>
        <style {{ nonce }}>
            @scope {
                :scope {
                    max-width: 56rem;
                    margin: 0 auto;
                    padding: var(--size-6) var(--size-4);
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-6);
                }

                h1 {
                    font-family: var(--font-serif);
                    font-weight: var(--font-weight-6);
                    font-size: clamp(2rem, 7vw, 2.5rem);
                    line-height: 0.95;
                    letter-spacing: -0.025em;
                    color: var(--color-text-primary);
                }

                .overline {
                    font-family: var(--font-mono);
                    font-size: var(--font-size-0);
                    letter-spacing: var(--font-letterspacing-3);
                    text-transform: uppercase;
                    font-weight: var(--font-weight-6);
                    color: var(--clay-4);
                    margin-bottom: var(--size-2);
                }

                .lead {
                    color: var(--color-text-secondary);
                    font-size: var(--font-size-2);
                    line-height: var(--font-lineheight-3);
                    max-width: 42rem;
                }

                .lead code {
                    font-family: var(--font-mono);
                    font-size: 0.95em;
                    background: var(--stone-1);
                    padding: 0 var(--size-1);
                    border-radius: var(--radius-1);
                }

                .cards {
                    display: grid;
                    grid-template-columns: repeat(auto-fit, minmax(20rem, 1fr));
                    gap: var(--size-4);
                }

                .card {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-3);
                    padding: var(--size-4);
                }

                .card h2 {
                    font-size: var(--font-size-3);
                    font-weight: var(--font-weight-6);
                    color: var(--color-text-primary);
                    margin: 0;
                }

                .card .tag {
                    font-family: var(--font-mono);
                    font-size: var(--font-size-0);
                    letter-spacing: var(--font-letterspacing-3);
                    text-transform: uppercase;
                    font-weight: var(--font-weight-6);
                    color: var(--color-text-muted);
                }

                .card p {
                    color: var(--color-text-secondary);
                    font-size: var(--font-size-1);
                    line-height: var(--font-lineheight-3);
                    margin: 0;
                }

                .card .action {
                    margin-top: auto;
                }

                .card code {
                    font-family: var(--font-mono);
                    font-size: 0.95em;
                    background: var(--stone-1);
                    padding: 0 var(--size-1);
                    border-radius: var(--radius-1);
                }

                .static-banners {
                    display: flex;
                    flex-direction: column;
                    gap: var(--size-2);
                }

                section.variants > h2 {
                    font-size: var(--font-size-4);
                    font-weight: var(--font-weight-6);
                    color: var(--color-text-primary);
                    margin-bottom: var(--size-2);
                    padding-bottom: var(--size-2);
                    border-bottom: var(--border-size-1) solid var(--color-border);
                }
            }
        </style>

        <header>
            <p class="overline">Dev · Error UX</p>
            <h1>Error UX showcase</h1>
            <p class="lead">
                Live exercises of the four error-surfacing classes documented in the
                <code>2026-05-18-error-ux-conventions</code> spec. Each card triggers
                a real handler path; the resulting banner appears at the top of the page
                via flash + redirect. The client-only path populates
                <code>#js-flash</code>.
            </p>
        </header>

        {{ template "banner" .Flash }}

        <section class="variants">
            <h2>Live triggers</h2>
            <div class="cards">

                <article class="card">
                    <span class="tag">A · Validation</span>
                    <h2>Server-side validation error</h2>
                    <p>
                        Service returns <code>domain.ValidationError</code>;
                        <code>userError</code> flashes the message verbatim and
                        redirects to <code>safeURL</code>. The banner re-renders
                        with <code>role="alert"</code>.
                    </p>
                    <form class="action" method="post" action="/dev/error-ux/trigger/validation">
                        <button type="submit" class="btn btn--block">
                            Trigger validation error
                        </button>
                    </form>
                </article>

                <article class="card">
                    <span class="tag">B · Business</span>
                    <h2>Expected business error</h2>
                    <p>
                        Logically-not-allowed action (e.g. "Add exercise on
                        unplanned day"). Same channel as A — the service returns
                        <code>ValidationError</code> with a user-actionable
                        message.
                    </p>
                    <form class="action" method="post" action="/dev/error-ux/trigger/business">
                        <button type="submit" class="btn btn--block">
                            Trigger business error
                        </button>
                    </form>
                </article>

                <article class="card">
                    <span class="tag">C · System</span>
                    <h2>Unexpected system error</h2>
                    <p>
                        Anything else — DB hiccup, plumbing fault.
                        <code>userError</code> logs the underlying error and
                        flashes a generic message; the user keeps their place
                        instead of getting a silent reload.
                    </p>
                    <form class="action" method="post" action="/dev/error-ux/trigger/system">
                        <button type="submit" class="btn btn--block btn--danger">
                            Trigger system error
                        </button>
                    </form>
                </article>

                <article class="card">
                    <span class="tag">D · Network</span>
                    <h2>Client-side network failure</h2>
                    <p>
                        <code>fetch</code> throws (offline / DNS / CORS). The
                        shim populates the page-top
                        <code>#js-flash</code> via <code>textContent</code>.
                        Click below to simulate the visual.
                    </p>
                    <button id="simulate-network-failure" type="button" class="btn btn--block btn--quiet action">
                        Simulate network failure
                    </button>
                    <script {{ nonce }}>
                      (() => {
                        const btn = document.getElementById('simulate-network-failure')
                        const flash = document.getElementById('js-flash')
                        if (!btn || !flash) return
                        btn.addEventListener('click', () => {
                          flash.textContent =
                            'Connection lost. Check your network and try again.'
                          flash.hidden = false
                        })
                      })()
                    </script>
                </article>

                <article class="card">
                    <span class="tag">E · Full page</span>
                    <h2>Full-page server error</h2>
                    <p>
                        For the rare case with no safe URL — template render
                        failure, broken session. <code>serverError</code> logs
                        and renders <code>error.gohtml</code> 500 directly.
                        No flash, no banner. Hitting the link below navigates
                        away from this page.
                    </p>
                    <a class="btn btn--block btn--quiet action" href="/dev/error-ux/server-error">
                        Trigger full-page 500
                    </a>
                </article>

            </div>
        </section>

        <section class="variants">
            <h2>Banner variants</h2>
            <p class="lead">
                Static reference. Mirrors the same component (<code>banner</code>)
                used by the live paths above; matches the styleguide for
                cross-reference.
            </p>
            <div class="static-banners">
                {{ range .BannerVariants }}
                    {{ template "banner" . }}
                {{ end }}
            </div>
        </section>
    </main>
{{ end }}
```

- [ ] **Step 2: Expand the render test to cover all sections**

Replace the body of `Test_application_devErrorUX_render` in `cmd/web/handler-error-ux_test.go`:

```go
func Test_application_devErrorUX_render(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/dev/error-ux")
	if err != nil {
		t.Fatalf("Failed to get /dev/error-ux: %v", err)
	}

	for _, heading := range []string{
		"Server-side validation error",
		"Expected business error",
		"Unexpected system error",
		"Client-side network failure",
		"Full-page server error",
	} {
		if doc.Find("h2:contains('" + heading + "')").Length() == 0 {
			t.Errorf("expected card heading %q on /dev/error-ux", heading)
		}
	}

	if doc.Find(".banner.banner--success").Length() == 0 {
		t.Error("expected the banner-variant reference to include a success example")
	}
	if doc.Find(".banner.banner--info").Length() == 0 {
		t.Error("expected the banner-variant reference to include an info example")
	}

	for _, action := range []string{
		"/dev/error-ux/trigger/validation",
		"/dev/error-ux/trigger/business",
		"/dev/error-ux/trigger/system",
	} {
		if doc.Find("form[action='" + action + "']").Length() == 0 {
			t.Errorf("expected a form posting to %s", action)
		}
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./cmd/web/ -run 'Test_application_devErrorUX_(render|gated)' -count=1`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/templates/pages/error-ux/error-ux.gohtml cmd/web/handler-error-ux_test.go
git commit -m "web: render /dev/error-ux page with five live-trigger cards"
```

---

## Task 3: Trigger dispatch tests

**Files:**
- Modify: `cmd/web/handler-error-ux_test.go`

- [ ] **Step 1: Write failing trigger tests**

Append to `cmd/web/handler-error-ux_test.go`. Add imports for `"net/url"` and `"strings"` if not already present.

```go
func Test_application_devErrorUX_triggerValidation_surfacesMessage(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/dev/error-ux")
	if err != nil {
		t.Fatalf("Failed to get /dev/error-ux: %v", err)
	}

	doc, err = client.SubmitForm(ctx, doc, "/dev/error-ux/trigger/validation", nil)
	if err != nil {
		t.Fatalf("Failed to submit validation trigger: %v", err)
	}

	banner := doc.Find(".banner.banner--error[role='alert']")
	if banner.Length() == 0 {
		t.Fatal("expected an error banner with role=alert after validation trigger")
	}
	if !strings.Contains(banner.Text(), "Name must be") {
		t.Errorf("validation banner missing expected message; got %q", banner.Text())
	}
}

func Test_application_devErrorUX_triggerSystem_surfacesGenericMessage(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/dev/error-ux")
	if err != nil {
		t.Fatalf("Failed to get /dev/error-ux: %v", err)
	}

	doc, err = client.SubmitForm(ctx, doc, "/dev/error-ux/trigger/system", nil)
	if err != nil {
		t.Fatalf("Failed to submit system trigger: %v", err)
	}

	banner := doc.Find(".banner.banner--error[role='alert']")
	if banner.Length() == 0 {
		t.Fatal("expected an error banner with role=alert after system trigger")
	}
	if !strings.Contains(banner.Text(), "Couldn't complete that action") {
		t.Errorf("system banner missing generic message; got %q", banner.Text())
	}
}

func Test_application_devErrorUX_triggerUnknownKind_returns404(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	form := url.Values{}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		server.URL()+"/dev/error-ux/trigger/bogus", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := server.Client().HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("Failed to POST trigger/bogus: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /dev/error-ux/trigger/bogus: got %d, want 404", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./cmd/web/ -run 'Test_application_devErrorUX_trigger' -count=1`
Expected: PASS — the dispatch and `userError` plumbing landed in Task 1, the form actions in Task 2, so this is a verification step.

- [ ] **Step 3: Commit**

```bash
git add cmd/web/handler-error-ux_test.go
git commit -m "web: cover /dev/error-ux trigger dispatch (validation, system, unknown)"
```

---

## Task 4: Home-page DevMode plumbing

**Files:**
- Modify: `cmd/web/handler-home.go`
- Modify: `cmd/web/handler-error-ux_test.go`

- [ ] **Step 1: Write failing home-page link tests**

Append to `cmd/web/handler-error-ux_test.go`:

```go
func Test_application_home_devLinks_devMode(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	doc, err := server.Client().GetDoc(t.Context(), "/")
	if err != nil {
		t.Fatalf("Failed to GET /: %v", err)
	}

	for _, href := range []string{"/dev/styleguide", "/dev/error-ux"} {
		if doc.Find("a[href='" + href + "']").Length() == 0 {
			t.Errorf("expected dev-mode home to surface a link to %s", href)
		}
	}
}

func Test_application_home_devLinks_hiddenOutsideDevMode(t *testing.T) {
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), prodLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	doc, err := server.Client().GetDoc(t.Context(), "/")
	if err != nil {
		t.Fatalf("Failed to GET /: %v", err)
	}

	for _, href := range []string{"/dev/styleguide", "/dev/error-ux"} {
		if doc.Find("a[href='" + href + "']").Length() != 0 {
			t.Errorf("non-dev home should not surface a link to %s", href)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/web/ -run Test_application_home_devLinks -count=1`
Expected: FAIL — `dev-mode home` test fails because the home templates don't render those anchors yet.

- [ ] **Step 3: Add `DevMode` to `homeTemplateData`**

Edit `cmd/web/handler-home.go`. Find the end of the `homeTemplateData` struct (the `DeloadEnabled bool` field):

```go
	// DeloadEnabled reports whether the deload feature is enabled for this user.
	DeloadEnabled bool
}
```

Replace with:

```go
	// DeloadEnabled reports whether the deload feature is enabled for this user.
	DeloadEnabled bool
	// DevMode mirrors app.devMode so the template can surface dev-only
	// affordances (links to /dev/styleguide, /dev/error-ux). Prod renders
	// nothing because the field is false there.
	DevMode bool
}
```

- [ ] **Step 4: Populate `DevMode` in the `home` handler**

In the same file, find the `data := homeTemplateData{...}` literal inside `(app *application).home`. The current zero-value fields end with `DeloadEnabled: false,`. Add one line:

```go
	data := homeTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Header:           PageHeaderData{Title: "This Week", Subtitle: ""},
		Days:             nil,
		MuscleBalance:    muscleBalanceView{Regions: nil},
		WeekInBlock:      0,
		MesocycleLength:  0,
		IsDeloadWeek:     false,
		DeloadEnabled:    false,
		DevMode:          app.devMode,
	}
```

- [ ] **Step 5: Build and verify compile**

Run: `go build ./...`
Expected: no output (success). Tests still fail — templates not yet updated.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/handler-home.go cmd/web/handler-error-ux_test.go
git commit -m "web: thread DevMode into homeTemplateData"
```

---

## Task 5: Home-page link sections

**Files:**
- Modify: `ui/templates/pages/home/schedule.gohtml`
- Modify: `ui/templates/pages/home/unauthenticated.gohtml`

- [ ] **Step 1: Authenticated eyebrow — add styling + dev-link group**

Edit `ui/templates/pages/home/schedule.gohtml`. Find the `.settings-link:focus-visible` rule (closes around line 56):

```css
                .week-eyebrow .settings-link:focus-visible {
                    outline: 2px solid var(--color-border-focus);
                    outline-offset: 2px;
                }
```

Add immediately after:

```css

                .week-eyebrow .dev-link {
                    color: var(--clay-4);
                    text-decoration: none;
                    padding: var(--size-1) var(--size-2);
                    border-radius: var(--radius-2);
                }

                .week-eyebrow .dev-link:focus-visible {
                    outline: 2px solid var(--color-border-focus);
                    outline-offset: 2px;
                }
```

- [ ] **Step 2: Authenticated eyebrow — inject the links into the eyebrow row**

In the same file, find:

```gohtml
                <span class="eyebrow-rule" aria-hidden="true"></span>
                <a href="/preferences" class="settings-link">Settings</a>
            </div>
```

Replace with:

```gohtml
                <span class="eyebrow-rule" aria-hidden="true"></span>
                {{ if .DevMode }}
                    <a href="/dev/styleguide" class="dev-link">Styleguide</a>
                    <span class="dot">·</span>
                    <a href="/dev/error-ux" class="dev-link">Error UX</a>
                    <span class="dot">·</span>
                {{ end }}
                <a href="/preferences" class="settings-link">Settings</a>
            </div>
```

(The `.dot` class is reused from the existing eyebrow rule — no new CSS needed for the separators.)

- [ ] **Step 3: Unauthenticated footer — add dev links**

Edit `ui/templates/pages/home/unauthenticated.gohtml`. Find the final footer link:

```gohtml
        <a href="/privacy">Privacy <span class="sep"></span> Security</a>
    </footer>
```

Replace with:

```gohtml
        <a href="/privacy">Privacy <span class="sep"></span> Security</a>
        {{ if .DevMode }}
            <span class="sep"></span>
            <a href="/dev/styleguide">Styleguide</a>
            <span class="sep"></span>
            <a href="/dev/error-ux">Error UX</a>
        {{ end }}
    </footer>
```

- [ ] **Step 4: Run the home-page link tests**

Run: `go test ./cmd/web/ -run Test_application_home_devLinks -count=1`
Expected: PASS — `Test_application_home_devLinks_devMode` finds the anchors in the unauthenticated footer; `_hiddenOutsideDevMode` confirms they vanish under `prodLookupEnv`.

- [ ] **Step 5: Run the full cmd/web suite to catch regressions**

Run: `go test --race --shuffle=on ./cmd/web/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add ui/templates/pages/home/schedule.gohtml ui/templates/pages/home/unauthenticated.gohtml
git commit -m "ui: surface dev-mode discovery links on home"
```

---

## Task 6: Lint + final verification

**Files:** none (verification only).

- [ ] **Step 1: Run the linter**

Run: `make lint-fix`
Expected: zero issues attributable to changed files. The pre-existing `gocognit` warning on `cmd/stresstest/main.go:422` is unrelated to this change and may surface — confirm it exists on `main` before this branch (`git log -1 -- cmd/stresstest/main.go` should show commit `c878522`) and leave it for a separate fix.

- [ ] **Step 2: Full repo test sweep**

Run: `go test --race --shuffle=on ./... -count=1`
Expected: PASS across all packages.

- [ ] **Step 3: Manual smoke (browser)**

Start the dev server (`go run ./cmd/web`) and open `http://localhost:8080/` in a browser:
- Confirm the dev-tools links appear in the unauthenticated footer.
- Click "Error UX" → page renders with five cards.
- Click "Trigger validation error" → banner reads "Name must be 1–50 characters."
- Click "Trigger system error" → banner reads "Couldn't complete that action. Please try again."
- Click "Simulate network failure" → the page-top `#js-flash` element reveals with "Connection lost. Check your network and try again."
- Click "Trigger full-page 500" → 500 page renders, no flash.

- [ ] **Step 4: Final commit (only if anything still uncommitted)**

```bash
git status
```

If clean, no commit. Otherwise stage and commit any remaining files.

---

## Self-review notes

- Spec coverage: all six file-touch items from the spec map to Tasks 1–5. The CLAUDE.md updates listed under "Migration" in the conventions spec (`2026-05-18`) are not part of *this* plan because they were landed with the conventions work; this plan only adds the *showcase*.
- The `Test_application_devErrorUX_render` body grows across Tasks 1 and 2 deliberately: Task 1's minimal assertion keeps the test green while Task 2 is in progress so the suite isn't left broken between commits.
- Type consistency: `errorUXTemplateData`, `BannerData`, `homeTemplateData.DevMode`, `devErrorUXPath`, and all handler names match across tasks.
- The `prodLookupEnv` helper lives in `handler-error-ux_test.go` (Task 1). Tasks 2/3/4 reuse it; placement is fine because Go tests in the same package share file scope.
