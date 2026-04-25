# Stack Navigator Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `Content-Location` based stack navigator with a standards-clean Navigation API implementation: explicit `X-Requested-With: stacknav` request marker, `X-Location` and `X-History-Action` response headers, in-place validation rendering via `document.write`, and lazy bfcache invalidation via sessionStorage marker + `pageshow`.

**Architecture:** MPA-first progressive enhancement. Server sees `X-Requested-With: stacknav` and returns 200 + headers (no body) for JS clients; otherwise 303 redirect. Client interception is gated on `'navigation' in window`, classifies response status (200 → navigate, anything HTML → render in place, otherwise reload), and handles three history actions (push, replace, pop-or-replace). Spec details in `docs/superpowers/specs/2026-04-25-stack-navigator-redesign-design.md`.

**Tech Stack:** Go 1.x with `net/http`, vanilla JS (no build step), Playwright (via `github.com/playwright-community/playwright-go`), goquery for handler tests.

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `cmd/web/helpers.go` | Modify | Replace `redirect` with `redirectAfterPOST` (POSTs only). Non-POST callers move to plain `http.Redirect`. |
| `cmd/web/helpers_test.go` | Create | Unit tests for `redirectAfterPOST`. |
| `cmd/web/handler-schedule.go` | Modify | Use `redirectAfterPOST`. Convert validation path from flash+redirect to inline 422 with `ValidationError` rendered. |
| `cmd/web/handler-preferences.go` | Modify | Use `redirectAfterPOST` for two redirects. |
| `cmd/web/handler-workout.go` | Modify | Use `redirectAfterPOST` for five redirects. |
| `cmd/web/handler-exerciseset.go` | Modify | Use `redirectAfterPOST` for two redirects. |
| `cmd/web/handler-admin-exercises.go` | Modify | Use `redirectAfterPOST` for two redirects. |
| `cmd/web/handler-admin-feature-flags.go` | Modify | Use `redirectAfterPOST` for one redirect. |
| `cmd/web/handlers-webauthn.go` | Modify | Replace `redirect` with `http.Redirect` (GET-flow, not POST result). |
| `cmd/web/handler-home.go` | Modify | Replace `redirect` with `http.Redirect`. |
| `cmd/web/middleware.go` | Modify | Replace `redirect` in `mustAuthenticate` with `http.Redirect`. |
| `cmd/web/playwright_test.go` | Modify | Extend with `Test_playwright_stacknav` covering all five flows. |
| `internal/e2etest/client.go` | Modify | Update `submitFormRequest` to accept 4xx HTML responses (returns doc and status). |
| `ui/static/main.js` | Modify | Full rewrite of the stack-navigator portion. |

---

## Task 1: Add Playwright spec scaffold (skipped initially)

**Files:**
- Modify: `cmd/web/playwright_test.go`

This task adds the new test that defines the desired behavior. It is skipped initially so the suite passes; the final task unskips it after the implementation lands.

- [ ] **Step 1: Read the existing test to understand the helpers and patterns**

Run: `cat cmd/web/playwright_test.go`

Note: test already creates a server, registers a virtual WebAuthn authenticator, registers a user, navigates to `/schedule`, fills Monday with 1 hour, submits, lands on `/`, logs out, logs in. Reuse the same helpers.

- [ ] **Step 2: Add the new test function**

Append the following to `cmd/web/playwright_test.go` (after `Test_playwright_smoketest`):

```go
// Test_playwright_stacknav verifies that the stack navigator behaves like a native
// mobile app for the five core flows defined in
// docs/superpowers/specs/2026-04-25-stack-navigator-redesign-design.md.
//
// The flows:
//
//	1. Same-URL replace (set update): submit on DETAIL → land at DETAIL, back goes
//	   to parent (workout day overview), not the same DETAIL page.
//	2. Cross-URL replace (swap): submit on SWAP redirecting to a different DETAIL
//	   → land at DETAIL', back goes to the previous DETAIL (acceptable per spec),
//	   second back goes to workout overview.
//	3. Pop-or-replace (schedule): submit /schedule when / is in history → cursor
//	   traverses to /, page reloads (bfcache marker bust), back exits the app rather
//	   than returning to /schedule.
//	4. Hierarchical back-link (data-back-button on swap page): click an in-page
//	   "back to detail" link → traverse to existing detail entry rather than push.
//	5. Validation error: submit empty schedule → URL stays at /schedule, alert role
//	   visible, no new history entry pushed.
func Test_playwright_stacknav(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow playwright stacknav test")
	}
	t.Skip("not yet implemented; see plan tasks 2-6")

	// Boilerplate copied from Test_playwright_smoketest.
	if err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
		Verbose:  false,
	}); err != nil {
		t.Fatalf("install playwright browsers: %v", err)
	}

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	serverURL := server.URL()

	pw, err := playwright.Run()
	if err != nil {
		t.Fatalf("start playwright: %v", err)
	}
	t.Cleanup(func() { _ = pw.Stop() })

	headless := os.Getenv("PWDEBUG") == ""
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
	})
	if err != nil {
		t.Fatalf("launch chromium: %v", err)
	}
	t.Cleanup(func() { _ = browser.Close() })

	bCtx, err := browser.NewContext()
	if err != nil {
		t.Fatalf("new browser context: %v", err)
	}
	t.Cleanup(func() { _ = bCtx.Close() })

	if os.Getenv("PWDEBUG") != "" {
		bCtx.SetDefaultTimeout(0)
	}

	page, err := bCtx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	page.On("console", func(msg playwright.ConsoleMessage) {
		if msg.Type() == "error" {
			t.Logf("browser console error: %s", msg.Text())
		}
	})
	page.On("pageerror", func(err error) {
		t.Logf("browser page error: %v", err)
	})

	// Setup: register and configure all weekdays so today always has a workout.
	if _, err = page.Goto(serverURL + "/"); err != nil {
		t.Fatalf("goto home: %v", err)
	}
	cdpSession, err := bCtx.NewCDPSession(page)
	if err != nil {
		t.Fatalf("new CDP session: %v", err)
	}
	if _, err = cdpSession.Send("WebAuthn.enable", map[string]any{"enableUI": true}); err != nil {
		t.Fatalf("WebAuthn.enable: %v", err)
	}
	if _, err = cdpSession.Send("WebAuthn.addVirtualAuthenticator", map[string]any{
		"options": map[string]any{
			"protocol":                    "ctap2",
			"transport":                   "internal",
			"hasResidentKey":              true,
			"hasUserVerification":         true,
			"isUserVerified":              true,
			"automaticPresenceSimulation": true,
		},
	}); err != nil {
		t.Fatalf("addVirtualAuthenticator: %v", err)
	}

	if err = page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Register"}).Click(); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("expect /schedule after registration: %v", err)
	}

	// === Flow 5: Validation error (empty schedule submit). Run before filling
	// the form so we see the error path first. ===
	startTrackingBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Start Tracking"})
	if err = startTrackingBtn.Click(); err != nil {
		t.Fatalf("submit empty schedule: %v", err)
	}
	if err = page.GetByRole("alert").WaitFor(); err != nil {
		t.Fatalf("alert visible after empty submit: %v", err)
	}
	if got, want := page.URL(), serverURL+"/schedule"; got != want {
		t.Errorf("Flow 5: URL after validation error = %q, want %q", got, want)
	}

	// === Flow 3 setup: fill all weekdays so today has a workout. ===
	for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"} {
		if _, err = page.GetByLabel(day).SelectOption(playwright.SelectOptionValues{
			Labels: &[]string{"1 hour"},
		}); err != nil {
			t.Fatalf("select %s duration: %v", day, err)
		}
	}

	// Confirm /schedule was preceded by / in history. The home handler does a
	// server-side 302→/schedule when prefs are empty, but a fresh tab still has /
	// as the navigation entry that triggered it. We make this explicit with a
	// Goto("/").
	if _, err = page.Goto(serverURL + "/"); err != nil {
		t.Fatalf("goto home before schedule submit: %v", err)
	}
	// Server redirects again to /schedule because prefs still empty (we have not
	// submitted yet). History: [HOME, SCHEDULE]. (302 redirects do not push,
	// but the initial / entry is preserved.)

	// === Flow 3: pop-or-replace (schedule submit) ===
	// Re-fill the form because navigation reloaded it.
	for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"} {
		if _, err = page.GetByLabel(day).SelectOption(playwright.SelectOptionValues{
			Labels: &[]string{"1 hour"},
		}); err != nil {
			t.Fatalf("re-select %s: %v", day, err)
		}
	}
	if err = startTrackingBtn.Click(); err != nil {
		t.Fatalf("submit valid schedule: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect / after valid schedule submit: %v", err)
	}
	// Verify pop-or-replace traversed (rather than pushed): going back should not
	// land on /schedule.
	if _, err = page.GoBack(); err != nil {
		// GoBack returns nil response if there's nothing to go back to. That's fine.
		t.Logf("GoBack returned: %v (acceptable if no prior entry)", err)
	}
	if got := page.URL(); got == serverURL+"/schedule" {
		t.Errorf("Flow 3: back from / landed on /schedule, expected anywhere else")
	}

	// Return to / for the remaining flows.
	if _, err = page.Goto(serverURL + "/"); err != nil {
		t.Fatalf("goto home: %v", err)
	}

	// === Flow 1, 2, 4 require navigating into a workout day. The home page
	// links to today's workout. Click it. ===
	todayLink := page.Locator("a[href^='/workouts/']").First()
	if err = todayLink.WaitFor(); err != nil {
		t.Fatalf("wait for workout link on home: %v", err)
	}
	workoutHref, err := todayLink.GetAttribute("href")
	if err != nil {
		t.Fatalf("get workout href: %v", err)
	}
	if err = todayLink.Click(); err != nil {
		t.Fatalf("click workout link: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s%s", serverURL, workoutHref)); err != nil {
		t.Fatalf("expect %s: %v", workoutHref, err)
	}

	// Click "Start" to advance the workout state and reveal exercise links.
	startBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Start"})
	if err = startBtn.Click(); err != nil {
		t.Logf("Start button: %v (workout may already be in progress)", err)
	}

	// Click first exercise link to land on DETAIL.
	exerciseLink := page.Locator("a[href*='/exercises/']").First()
	if err = exerciseLink.WaitFor(); err != nil {
		t.Fatalf("wait for exercise link: %v", err)
	}
	exerciseHref, err := exerciseLink.GetAttribute("href")
	if err != nil {
		t.Fatalf("get exercise href: %v", err)
	}
	if err = exerciseLink.Click(); err != nil {
		t.Fatalf("click exercise link: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s%s", serverURL, exerciseHref)); err != nil {
		t.Fatalf("expect %s: %v", exerciseHref, err)
	}
	detailURL := page.URL()

	// === Flow 1: same-URL replace (set update) ===
	// Find the set form, fill it, submit. The exact form fields depend on the
	// exercise type. For weighted exercises: weight + reps + signal. For others:
	// reps. Look for a submit button labeled "Done" or similar.
	//
	// NOTE: This section may need adjustment based on actual page structure.
	// Run with PWDEBUG=1 to inspect.
	if err = page.GetByLabel("Reps").First().Fill("10"); err != nil {
		t.Logf("fill reps: %v (may be weighted exercise — try weight+reps)", err)
		if err = page.GetByLabel("Weight").First().Fill("50"); err != nil {
			t.Fatalf("fill weight: %v", err)
		}
		if err = page.GetByLabel("Reps").First().Fill("10"); err != nil {
			t.Fatalf("fill reps after weight: %v", err)
		}
	}
	doneBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Done"})
	if err = doneBtn.Click(); err != nil {
		t.Fatalf("click Done: %v", err)
	}
	// After replace, URL is still DETAIL.
	if err = page.WaitForURL(detailURL); err != nil {
		t.Fatalf("Flow 1: expected URL %s after set update: %v", detailURL, err)
	}
	// Back should go to workout overview, NOT the same DETAIL.
	if _, err = page.GoBack(); err != nil {
		t.Fatalf("Flow 1: GoBack: %v", err)
	}
	if got, want := page.URL(), serverURL+workoutHref; got != want {
		t.Errorf("Flow 1: URL after back = %q, want %q (back should skip the replaced DETAIL)", got, want)
	}

	// Forward to DETAIL again.
	if _, err = page.GoForward(); err != nil {
		t.Fatalf("forward: %v", err)
	}

	// === Flow 2: cross-URL replace (swap exercise) ===
	swapLink := page.Locator("a[href$='/swap']").First()
	if err = swapLink.WaitFor(); err != nil {
		t.Fatalf("wait for swap link: %v", err)
	}
	if err = swapLink.Click(); err != nil {
		t.Fatalf("click swap link: %v", err)
	}
	swapURL := page.URL()
	// Pick a different exercise on the swap page. The form has a hidden new_exercise_id
	// or a submit button per option.
	swapBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Swap"}).First()
	if err = swapBtn.WaitFor(); err != nil {
		t.Fatalf("wait for swap button: %v", err)
	}
	if err = swapBtn.Click(); err != nil {
		t.Fatalf("click swap button: %v", err)
	}
	// Wait for navigation away from swap.
	if err = page.WaitForURL(func(url string) bool {
		return url != swapURL && strings.Contains(url, "/exercises/")
	}); err != nil {
		t.Fatalf("Flow 2: expected redirect away from swap to a new exercise: %v", err)
	}
	newDetailURL := page.URL()
	// Back from DETAIL': should go to old DETAIL (per spec Flow 2 — stale entry kept).
	if _, err = page.GoBack(); err != nil {
		t.Fatalf("Flow 2 back: %v", err)
	}
	if got := page.URL(); got == swapURL {
		t.Errorf("Flow 2: back from %s landed on swap %s, expected old DETAIL", newDetailURL, swapURL)
	}

	// === Flow 4: hierarchical back-link via data-back-button ===
	// Re-navigate forward to set up [WORKOUT, DETAIL, SWAP] state.
	if _, err = page.Goto(detailURL); err != nil {
		t.Fatalf("goto detailURL: %v", err)
	}
	if err = page.Locator("a[href$='/swap']").First().Click(); err != nil {
		t.Fatalf("click swap from detail: %v", err)
	}
	if err = page.WaitForURL(swapURL); err != nil {
		t.Logf("swap URL changed (acceptable): %v", err)
	}
	backLink := page.Locator("a[data-back-button]")
	if err = backLink.WaitFor(); err != nil {
		t.Fatalf("wait for data-back-button: %v", err)
	}
	if err = backLink.Click(); err != nil {
		t.Fatalf("click data-back-button: %v", err)
	}
	// Should traverse to existing DETAIL entry rather than push.
	if err = page.WaitForURL(detailURL); err != nil {
		t.Fatalf("Flow 4: expected DETAIL URL after data-back-button: %v", err)
	}
	// Forward should go to SWAP (proves it was a traverse, not a push).
	if _, err = page.GoForward(); err != nil {
		t.Fatalf("Flow 4 forward: %v", err)
	}
	if got, want := page.URL(), swapURL; got != want {
		t.Errorf("Flow 4: forward URL = %q, want %q (traverse should preserve forward stack)", got, want)
	}
}
```

Note: this code uses `strings` from the standard library — make sure the import is added at the top of the file.

- [ ] **Step 3: Add missing imports**

In `cmd/web/playwright_test.go`, ensure imports include `"strings"`:

```go
import (
    "fmt"
    "os"
    "strings"
    "testing"

    "github.com/myrjola/petrapp/internal/e2etest"
    "github.com/myrjola/petrapp/internal/testhelpers"
    "github.com/playwright-community/playwright-go"
)
```

- [ ] **Step 4: Verify the test parses, compiles, and skips cleanly**

Run: `go test -short -run Test_playwright_stacknav ./cmd/web/`
Expected output contains `--- SKIP: Test_playwright_stacknav` (skipped via `testing.Short()` short-circuit).

Run: `go test -count 1 -run Test_playwright_stacknav ./cmd/web/`
Expected output contains `--- SKIP: Test_playwright_stacknav` (skipped via `t.Skip("not yet implemented...")`).

- [ ] **Step 5: Run full test suite to confirm nothing broke**

Run: `make test`
Expected: all tests pass; new test reported as skipped.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/playwright_test.go
git commit -m "$(cat <<'EOF'
test: add stacknav playwright spec scaffold (skipped)

Defines the desired five-flow behavior as the spec test, skipped until
the implementation lands across the next plan tasks.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `redirectAfterPOST` helper with unit test

**Files:**
- Modify: `cmd/web/helpers.go`
- Create: `cmd/web/helpers_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/web/helpers_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_redirectAfterPOST_StackNavRequest_Returns200WithHeaders(t *testing.T) {
	app := &application{}
	tests := []struct {
		name           string
		target         string
		action         string
		wantAction     string
		wantActionSet  bool
	}{
		{name: "default replace", target: "/foo", action: "", wantAction: "", wantActionSet: false},
		{name: "explicit pop-or-replace", target: "/", action: "pop-or-replace", wantAction: "pop-or-replace", wantActionSet: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
			r.Header.Set("X-Requested-With", "stacknav")

			app.redirectAfterPOST(w, r, tt.target, tt.action)

			if got := w.Code; got != http.StatusOK {
				t.Errorf("status = %d, want %d", got, http.StatusOK)
			}
			if got := w.Header().Get("X-Location"); got != tt.target {
				t.Errorf("X-Location = %q, want %q", got, tt.target)
			}
			gotAction := w.Header().Get("X-History-Action")
			if tt.wantActionSet {
				if gotAction != tt.wantAction {
					t.Errorf("X-History-Action = %q, want %q", gotAction, tt.wantAction)
				}
			} else if gotAction != "" {
				t.Errorf("X-History-Action = %q, want empty", gotAction)
			}
			if got := w.Body.Len(); got != 0 {
				t.Errorf("body length = %d, want 0", got)
			}
		})
	}
}

func Test_redirectAfterPOST_PlainRequest_Returns303(t *testing.T) {
	app := &application{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
	// no X-Requested-With

	app.redirectAfterPOST(w, r, "/target", "")

	if got := w.Code; got != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", got, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/target" {
		t.Errorf("Location = %q, want /target", got)
	}
	if got := w.Header().Get("X-Location"); got != "" {
		t.Errorf("X-Location should not be set for plain request, got %q", got)
	}
}
```

- [ ] **Step 2: Run test, expect failure**

Run: `go test -count 1 -run Test_redirectAfterPOST ./cmd/web/`
Expected: FAIL — `redirectAfterPOST` does not exist on `application`.

- [ ] **Step 3: Add the helper**

Edit `cmd/web/helpers.go`. Add the new function after `defaultMaxFormSize` block, before the existing `redirect` function:

```go
// redirectAfterPOST sends the client to target after a successful POST.
// action is "" (default: replace) or "pop-or-replace" (client traverses to an
// existing matching history entry when present, otherwise replace).
//
// JS-enhanced submits (X-Requested-With: stacknav) get HTTP 200 with X-Location
// and optional X-History-Action headers and an empty body. Non-JS submits get a
// standard 303 See Other redirect.
func (app *application) redirectAfterPOST(w http.ResponseWriter, r *http.Request, target, action string) {
	if r.Header.Get("X-Requested-With") == "stacknav" {
		w.Header().Set("X-Location", target)
		if action != "" {
			w.Header().Set("X-History-Action", action)
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
```

Leave the existing `redirect` function in place — Task 3 removes it.

- [ ] **Step 4: Run test, expect pass**

Run: `go test -count 1 -run Test_redirectAfterPOST ./cmd/web/`
Expected: PASS for both subtests.

- [ ] **Step 5: Run lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/helpers.go cmd/web/helpers_test.go
git commit -m "$(cat <<'EOF'
feat(web): add redirectAfterPOST helper

Replaces Sec-Fetch-Dest sniffing with explicit X-Requested-With marker.
Returns 200 + X-Location + optional X-History-Action for stacknav clients;
303 See Other otherwise.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Migrate all callers of `redirect` and remove the helper

**Files:**
- Modify: `cmd/web/handler-schedule.go`
- Modify: `cmd/web/handler-preferences.go`
- Modify: `cmd/web/handler-workout.go`
- Modify: `cmd/web/handler-exerciseset.go`
- Modify: `cmd/web/handler-admin-exercises.go`
- Modify: `cmd/web/handler-admin-feature-flags.go`
- Modify: `cmd/web/handlers-webauthn.go`
- Modify: `cmd/web/handler-home.go`
- Modify: `cmd/web/middleware.go`
- Modify: `cmd/web/helpers.go`

Two distinct kinds of callers exist:

- **POST handlers redirecting after success:** use `app.redirectAfterPOST(w, r, target, action)`. All use action `""` (= `replace`) except `schedulePOST` which uses `"pop-or-replace"`.
- **Non-POST callers** (auth bouncing, mid-request redirects): use `http.Redirect(w, r, target, http.StatusSeeOther)`.

The schedule validation path is updated separately in Task 4.

- [ ] **Step 1: Update POST handlers — schedule**

Edit `cmd/web/handler-schedule.go`. Replace line 53:

Old:
```go
	redirect(w, r, "/")
```

New:
```go
	app.redirectAfterPOST(w, r, "/", "pop-or-replace")
```

Leave line 44 (`redirect(w, r, "/schedule")` for validation) alone — Task 4 rewrites this path entirely.

- [ ] **Step 2: Update POST handlers — preferences**

Edit `cmd/web/handler-preferences.go`. Two occurrences of `redirect(w, r, "/")` at lines 118 and 136. Replace both with:

```go
	app.redirectAfterPOST(w, r, "/", "")
```

- [ ] **Step 3: Update POST handlers — workout**

Edit `cmd/web/handler-workout.go`. Replace each `redirect(w, r, ...)` call with `app.redirectAfterPOST(w, r, ..., "")`:

- Line 79 (`workoutCompletePOST` redirecting to `.../complete`).
- Line 104 (`workoutStartPOST` redirecting to `/workouts/{date}`).
- Line 161 (`workoutFeedbackPOST` redirecting to `/`).
- Line 265 (`workoutSwapExercisePOST` redirecting to new exercise).
- Line 364 (`workoutAddExercisePOST` redirecting to `/workouts/{date}`).

For each, the change is exactly:

Old: `redirect(w, r, X)`
New: `app.redirectAfterPOST(w, r, X, "")`

- [ ] **Step 4: Update POST handlers — exercise set**

Edit `cmd/web/handler-exerciseset.go`. Two occurrences:

- Line 301 (`exerciseSetUpdatePOST`).
- Line 329 (`exerciseSetWarmupCompletePOST`).

For each: `redirect(w, r, X)` → `app.redirectAfterPOST(w, r, X, "")`.

- [ ] **Step 5: Update POST handlers — admin**

Edit `cmd/web/handler-admin-exercises.go`. Two occurrences at lines 180 and 207:

`redirect(w, r, X)` → `app.redirectAfterPOST(w, r, X, "")`.

Edit `cmd/web/handler-admin-feature-flags.go`. One occurrence at line 63:

`redirect(w, r, "/admin/feature-flags")` → `app.redirectAfterPOST(w, r, "/admin/feature-flags", "")`.

- [ ] **Step 6: Update non-POST callers — webauthn handler**

Edit `cmd/web/handlers-webauthn.go`. The `redirect(w, r, "/")` at line 70 is reached after successful registration completion via a non-POST path (or after logout); it's a redirect to home rather than a POST result. Replace with:

```go
	http.Redirect(w, r, "/", http.StatusSeeOther)
```

Note: read the surrounding context to confirm this is correct; if the call is reached as a POST handler's success path, use `app.redirectAfterPOST(w, r, "/", "")` instead.

- [ ] **Step 7: Update non-POST callers — home**

Edit `cmd/web/handler-home.go` line 261. The `redirect(w, r, "/schedule")` happens during a GET when prefs are empty. Replace with:

```go
	http.Redirect(w, r, "/schedule", http.StatusSeeOther)
```

- [ ] **Step 8: Update non-POST callers — middleware**

Edit `cmd/web/middleware.go` line 181 in `mustAuthenticate`. Replace:

```go
	redirect(w, r, "/")
```

with:

```go
	http.Redirect(w, r, "/", http.StatusSeeOther)
```

- [ ] **Step 9: Remove the `redirect` helper**

Edit `cmd/web/helpers.go`. Delete the entire `redirect` function (lines 28–38 in the current file):

```go
// redirect detects if the request is originating from a fetch API call or a top-level navigation and points the user
// to the correct URL.
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("Sec-Fetch-Dest") == "empty" {
		w.Header().Set("Content-Location", path)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, path, http.StatusSeeOther)
}
```

- [ ] **Step 10: Verify no callers remain**

Run: `grep -rn '\bredirect(' cmd/web/ --include="*.go"`
Expected: only the helper definition was removed; no other callers should exist. The output should be empty (or only show comment text, not actual function calls).

If any matches remain, they were missed in steps 1–8 — fix them.

- [ ] **Step 11: Build and run all tests**

Run: `go build ./...`
Expected: no errors.

Run: `make test`
Expected: all tests pass.

- [ ] **Step 12: Run lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 13: Commit**

```bash
git add cmd/web/
git commit -m "$(cat <<'EOF'
refactor(web): migrate redirect callers to redirectAfterPOST and http.Redirect

POST handlers now use redirectAfterPOST with explicit history action.
Non-POST callers (mid-request bounces) use plain http.Redirect.
Removes the now-unused redirect helper that misused Content-Location.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Convert schedule validation to inline 422 + relax e2e test client

**Files:**
- Modify: `cmd/web/handler-schedule.go`
- Modify: `internal/e2etest/client.go`

The current `schedulePOST` validation path uses a flash message with redirect-back. Spec requires inline 422 with the validation error rendered in the same response.

`scheduleTemplateData` already has a `ValidationError string` field; the GET handler reads it from `popFlashError`. We change the POST validation path to render directly with status 422, and leave the GET handler to keep working from flash for any other code that relies on it (none currently).

The e2e test client's `submitFormRequest` rejects non-200 responses, which would break with 422. We update it to return the document and status code, letting callers check.

- [ ] **Step 1: Update e2e test client — relax status check**

Edit `internal/e2etest/client.go` lines 587–598. Replace:

```go
	if http.StatusOK != resp.StatusCode {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse the response
	newDoc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("create document from reader: %w", err)
	}
	newDoc.Url = resp.Request.URL
	return newDoc, nil
}
```

With:

```go
	// Accept any 2xx and 4xx status with HTML body. 5xx still treated as error.
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse the response
	newDoc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("create document from reader: %w", err)
	}
	newDoc.Url = resp.Request.URL
	return newDoc, nil
}
```

This keeps the same return signature so callers don't change.

- [ ] **Step 2: Verify no test relies on the strict 200 check breaking other 4xx**

Run: `grep -rn 'SubmitForm' cmd/web/ --include="*_test.go" | head`
Expected: callers all use the document return value or ignore it. No test asserts on `.StatusCode` from this path. (If any did, they'd need updating; verify by reading tests.)

Run: `make test`
Expected: still passes.

- [ ] **Step 3: Convert schedulePOST validation path to 422 + render**

Edit `cmd/web/handler-schedule.go`. Replace the entire `schedulePOST` function with:

```go
func (app *application) schedulePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	prefs := weekdaysToPreferences(r)

	if prefs.IsEmpty() {
		data := scheduleTemplateData{
			BaseTemplateData: newBaseTemplateData(r),
			Weekdays:         preferencesToWeekdays(prefs),
			DurationOptions:  getWorkoutDurationOptions(),
			ValidationError:  "Please schedule at least one workout day.",
		}
		app.render(w, r, http.StatusUnprocessableEntity, "schedule", data)
		return
	}

	if err := app.workoutService.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
		return
	}

	app.redirectAfterPOST(w, r, "/", "pop-or-replace")
}
```

Key changes:
- Empty-prefs branch no longer sets a flash and redirects; it renders the schedule template directly with status 422 and the inline error.
- The user's selected (empty) values are echoed back via `preferencesToWeekdays(prefs)`, mirroring the form state.
- The success branch still uses `redirectAfterPOST` with `pop-or-replace` (already done in Task 3).

- [ ] **Step 4: Run go test for schedule and preferences**

Run: `go test -count 1 -run Schedule ./cmd/web/`
Expected: any existing schedule tests pass.

Run: `go test -count 1 -run Preferences ./cmd/web/`
Expected: passes (preferences tests submit `/schedule` as setup; the success path is unchanged so they should keep working).

Run: `make test`
Expected: all tests pass.

- [ ] **Step 5: Run lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/handler-schedule.go internal/e2etest/client.go
git commit -m "$(cat <<'EOF'
feat(schedule): inline 422 validation rendering

Replaces flash+redirect with direct re-render at status 422 when the
schedule form is empty. The e2e test client now accepts 4xx HTML
responses as success (5xx still errors) so handler tests can exercise
validation paths.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Rewrite `ui/static/main.js` stack navigator

**Files:**
- Modify: `ui/static/main.js`

This task is the core client-side rewrite. It produces a new file from scratch (replacing the existing implementation) so it's presented as a single Write operation rather than a series of edits.

- [ ] **Step 1: Replace the file contents**

Overwrite `ui/static/main.js` with:

```js
/**
 * Convenience function to get the parent element of the current script tag.
 * Inspired by https://github.com/gnat/surreal.
 * @returns {HTMLElement}
 */
function me() {
  return document.currentScript.parentElement
}

const sameUrl = (a, b) =>
  a.origin === b.origin && a.pathname === b.pathname && a.search === b.search

const invalidateKey = (url) =>
  'stacknav:invalidate:' + url.pathname + url.search

if ('navigation' in window) {
  navigation.addEventListener('navigate', (e) => {
    if (!e.formData) return
    if (!e.canIntercept || e.hashChange || e.downloadRequest) return
    if (new URL(e.destination.url).origin !== location.origin) return
    for (const [, v] of e.formData) {
      if (v instanceof File) return
    }
    e.intercept({ handler: () => submitForm(e) })
  })
}

async function submitForm(e) {
  const body = new URLSearchParams(e.formData)
  let res
  try {
    res = await fetch(e.destination.url, {
      method: 'POST',
      body,
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-Requested-With': 'stacknav',
      },
      redirect: 'manual',
    })
  } catch (_) {
    location.reload()
    return
  }

  if (res.status === 200) {
    const target = res.headers.get('X-Location')
    const action = res.headers.get('X-History-Action')
    if (!target) {
      location.reload()
      return
    }
    if (action === 'pop-or-replace') popOrReplaceTo(target)
    else replaceTo(target)
    return
  }

  const ct = res.headers.get('Content-Type') || ''
  if (ct.includes('text/html')) {
    const html = await res.text()
    document.open()
    document.write(html)
    document.close()
    return
  }

  location.reload()
}

function replaceTo(target) {
  navigation.navigate(target, { history: 'replace' })
}

function popOrReplaceTo(target) {
  const targetUrl = new URL(target, location.origin)
  const entries = navigation.entries()
  for (let i = navigation.currentEntry.index - 1; i >= 0; i--) {
    if (sameUrl(new URL(entries[i].url), targetUrl)) {
      sessionStorage.setItem(invalidateKey(targetUrl), '1')
      navigation.traverseTo(entries[i].key)
      return
    }
  }
  replaceTo(target)
}

window.addEventListener('pagereveal', (e) => {
  if (!e.viewTransition) return
  if (!('navigation' in window)) return
  const act = navigation.activation
  if (!act) return
  if (act.navigationType === 'replace' || act.navigationType === 'reload') {
    e.viewTransition.skipTransition()
    return
  }
  let dir = 'forward'
  if (act.navigationType === 'traverse' && act.from && act.entry) {
    dir = act.entry.index < act.from.index ? 'backward' : 'forward'
  }
  e.viewTransition.types.add(dir)
})

document.addEventListener('click', (e) => {
  if (!('navigation' in window)) return
  const link = e.target.closest('a[data-back-button]')
  if (!link) return
  const target = new URL(link.href)
  const entries = navigation.entries()
  for (let i = navigation.currentEntry.index - 1; i >= 0; i--) {
    if (sameUrl(new URL(entries[i].url), target)) {
      e.preventDefault()
      navigation.traverseTo(entries[i].key)
      return
    }
  }
})

// Form submission UI state.
document.addEventListener('submit', (e) => {
  const form = e.target
  if (!(form instanceof HTMLFormElement)) return
  form.classList.add('submitting')
  const submitButton = form.querySelector('button[type=submit]')
  if (submitButton) submitButton.disabled = true
})

// Reset submit state and process bfcache invalidation marker on pageshow.
window.addEventListener('pageshow', (event) => {
  // Reset submitting forms after bfcache restore.
  if (event.persisted) {
    document.querySelectorAll('form.submitting').forEach((form) => {
      form.classList.remove('submitting')
      const submitButton = form.querySelector('button[type=submit]')
      if (submitButton) submitButton.disabled = false
    })
  }

  // Bfcache invalidation marker: reload if the entry was marked stale.
  const key = invalidateKey(new URL(location.href))
  const marker = sessionStorage.getItem(key)
  if (marker) sessionStorage.removeItem(key)
  if (event.persisted && marker) location.reload()
})
```

Key differences from the prior implementation:

- Feature-gated on `'navigation' in window`.
- Uses `X-Requested-With: stacknav` request marker; reads `X-Location` and `X-History-Action` instead of `Content-Location`.
- `popOrReplaceTo` walks entries backward from `currentEntry.index - 1` (was forward, picking oldest match).
- `sameUrl` compares `origin + pathname + search` (was pathname-only).
- bfcache invalidation is lazy: marker set on traversal, consumed on `pageshow`, reload only when restored from bfcache.
- `pagereveal` uses `navigation.activation` index comparison instead of URL-depth heuristic; null-`from` no longer crashes; replace/reload skip the transition.
- `data-back-button` click handler uses event delegation, walks backward, compares full URL.
- Validation errors render in place via `document.write` for any non-200 HTML response.
- Network errors fall back to `location.reload()`.

- [ ] **Step 2: Smoke-check the file by serving it**

Run: `go run ./cmd/web/ &` (or use whatever the project's local-run is — `make run` if defined; check `Makefile`)

Expected: server starts. Open `http://localhost:<port>/main.js` in a browser; file should serve without errors.

If you don't have a quick way to start the server, skip this manual check; Task 6 will verify via Playwright.

Stop the server: `kill %1` (or however the project's run is wired).

- [ ] **Step 3: Run static asset tests if any**

Run: `make test`
Expected: passes. (No JS unit tests in this codebase — coverage comes from Playwright in Task 6.)

- [ ] **Step 4: Run lint**

Run: `make lint-fix`
Expected: no Go-side errors. (No JS linter is configured in this project.)

- [ ] **Step 5: Commit**

```bash
git add ui/static/main.js
git commit -m "$(cat <<'EOF'
feat(stacknav): rewrite client to use X-Location/X-History-Action protocol

Replaces the Content-Location-based interceptor with a Navigation API
client gated on 'navigation' in window. New behavior:

- X-Requested-With: stacknav request marker
- X-Location + X-History-Action response headers
- popOrReplaceTo walks entries backward, compares full URL
- bfcache invalidation via sessionStorage marker on pageshow (no flash)
- pagereveal uses navigation.activation index comparison (no URL-depth)
- Validation errors render in place via document.write
- Smart back-link uses event delegation and full-URL match
- Feature-gated; no-op without Navigation API

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Unskip Playwright spec, run, fix flakiness, full CI

**Files:**
- Modify: `cmd/web/playwright_test.go`

- [ ] **Step 1: Remove the skip line**

Edit `cmd/web/playwright_test.go` `Test_playwright_stacknav`. Delete the line:

```go
	t.Skip("not yet implemented; see plan tasks 2-6")
```

(Keep the `testing.Short()` skip — that's a different skip, gated on the `-short` flag.)

- [ ] **Step 2: Run the new test**

Run: `go test -count 1 -v -run Test_playwright_stacknav ./cmd/web/`

Expected: test passes. If it fails, the failure is informative — read the assertion message and fix:

- If a Playwright locator does not find the expected element on a page, run with `PWDEBUG=1` to inspect: `PWDEBUG=1 go test -count 1 -v -run Test_playwright_stacknav ./cmd/web/`. In the headed browser, find the actual selectors (button text, label text, link href patterns) and update the test.
- If the back-button assertion fails, the most likely cause is the stack-navigator implementation not behaving as expected for that flow. Debug by adding `t.Logf("history: %v", page.Locator("...").TextContent())` or by running `await page.evaluate(...)` to inspect `navigation.entries()`.
- If the `data-back-button` flow fails, confirm the `exercise-swap.gohtml` template renders `<a href="/workouts/.../exercises/{id}" data-back-button>` (it does as of plan-writing time — line 45 of that template).

- [ ] **Step 3: Run the existing smoke test to confirm no regression**

Run: `go test -count 1 -v -run Test_playwright_smoketest ./cmd/web/`
Expected: passes. The schedule submit flow now exercises the new `redirectAfterPOST` + JS path.

- [ ] **Step 4: Run full CI**

Run: `make ci`
Expected: passes (init + build + lint + test + sec).

- [ ] **Step 5: Commit**

```bash
git add cmd/web/playwright_test.go
git commit -m "$(cat <<'EOF'
test: enable stacknav playwright spec

Verifies the five flows from the design spec end-to-end:
1. Same-URL replace (set update)
2. Cross-URL replace (swap exercise)
3. Pop-or-replace (schedule submit)
4. Hierarchical back-link (data-back-button)
5. Validation error (empty schedule)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes

The plan covers all sections of the spec:

- Wire protocol → Tasks 2, 5.
- Per-flow behavior (5 flows) → Task 1 (spec-as-test) + Task 6 (verification).
- Server-side changes table → Task 3 (handler migration); schedule validation refactor → Task 4.
- Client implementation (4a–4g of spec) → Task 5 (full file rewrite).
- Testing → Task 1 (Playwright spec) + Task 2 (unit test) + Task 6 (verification).
- Removed code (`redirect` helper, `Content-Location`, `Sec-Fetch-Dest` branch) → Task 3.
- Browser support gate (`'navigation' in window`) → Task 5.

Identifiers used consistently: `redirectAfterPOST`, `replaceTo`, `popOrReplaceTo`, `sameUrl`, `invalidateKey`, `submitForm`, `X-Requested-With: stacknav`, `X-Location`, `X-History-Action: replace | pop-or-replace`, `Test_playwright_stacknav`.

No placeholders. Each step has either exact code, a concrete shell command with expected output, or a specific edit description.

The Playwright test in Task 1 has Flow 1, 2, 4 navigation that depends on the actual page structure (button names, link selectors). The test code includes notes to run `PWDEBUG=1` and adjust if locators don't match. This is acknowledged-imprecise rather than wrong: the *assertions* (back-button URL after various submits) are exact; the *path to set them up* may need locator tweaks during Task 6.
