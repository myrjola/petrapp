# Stack Navigator Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **Status:** Executed. The simplifications below are already in the codebase. The plan was originally drafted around a richer protocol (`X-History-Action`, bfcache invalidation marker, dedicated `redirectAfterPOST` helper); during implementation we collapsed to a single history strategy (pop-or-replace) and reused the existing `redirect` helper. The spec at `docs/superpowers/specs/2026-04-25-stack-navigator-redesign-design.md` reflects what was built.

**Goal:** Replace the `Content-Location` based stack navigator with a standards-clean Navigation API implementation: explicit `X-Requested-With: stacknav` request marker, `X-Location` response header, single pop-or-replace history strategy on the client. Validation errors continue to use the existing `putFlashError` + redirect-to-form pattern (the app's CSP `require-trusted-types-for 'script'` blocks any in-place HTML rendering, so we lean on the established flash mechanism).

**Architecture:** MPA-first progressive enhancement. Server sees `X-Requested-With: stacknav` and returns 200 + `X-Location` (no body) for JS clients; otherwise 303 redirect. Client interception is gated on `'navigation' in window`, uses `e.preventDefault()` (not `e.intercept()`, due to iOS WebKit bug 293952), treats 200 as the only navigate-success path, and falls back to `location.reload()` on anything else. One history strategy: pop-or-replace — walk back through history for a URL match, traverse if found, otherwise replace. Spec details in `docs/superpowers/specs/2026-04-25-stack-navigator-redesign-design.md`.

**Tech Stack:** Go 1.x with `net/http`, vanilla JS (no build step), Playwright (via `github.com/playwright-community/playwright-go`), goquery for handler tests.

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `cmd/web/helpers.go` | Modify | Rewrite `redirect` to negotiate the wire protocol (200 + `X-Location` for stacknav clients, 303 otherwise). Same signature, same call sites. |
| `cmd/web/handler-schedule.go` | Unchanged call site | Existing `redirect` calls now route through the new wire protocol. |
| `cmd/web/handler-preferences.go` | Unchanged call site | Same. |
| `cmd/web/handler-workout.go` | Unchanged call site | Same. |
| `cmd/web/handler-exerciseset.go` | Unchanged call site | Same. |
| `cmd/web/handler-admin-exercises.go` | Unchanged call site | Same. |
| `cmd/web/handler-admin-feature-flags.go` | Unchanged call site | Same. |
| `cmd/web/handlers-webauthn.go` | Unchanged call site | Same (non-POST callers transparently fall through to 303 since they don't carry `X-Requested-With`). |
| `cmd/web/handler-home.go` | Unchanged call site | Same. |
| `cmd/web/middleware.go` | Unchanged call site | Same. |
| `cmd/web/playwright_test.go` | Modify | Extend with `Test_playwright_stacknav` covering all five flows. |
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
	t.Skip("not yet implemented; see plan tasks 2-5")

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

## Task 2: Rewrite the `redirect` helper

**Files:**
- Modify: `cmd/web/helpers.go`

The dispatch is updated in place; the signature and every call site stay the same. POST handlers and non-POST mid-request bounces all use this single helper. Non-POST callers transparently fall through to `http.Redirect` because they don't carry `X-Requested-With: stacknav`.

- [ ] **Step 1: Replace the function body**

Edit `cmd/web/helpers.go`:

```go
// redirect detects if the request is originating from a fetch API call or a
// top-level navigation and points the user to the correct URL.
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("X-Requested-With") == "stacknav" {
		w.Header().Set("X-Location", path)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, path, http.StatusSeeOther)
}
```

The old body sniffed `Sec-Fetch-Dest: empty` and set `Content-Location`; both are gone.

- [ ] **Step 2: Build and run all tests**

Run: `go build ./...`
Expected: no errors.

Run: `make test`
Expected: all tests pass. The existing handler tests cover the redirect call sites; the playwright smoke test covers the JS path end-to-end.

- [ ] **Step 3: Run lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add cmd/web/helpers.go
git commit -m "$(cat <<'EOF'
refactor(web): rewrite redirect to use X-Requested-With/X-Location protocol

Replaces Sec-Fetch-Dest sniffing and Content-Location misuse with an
explicit X-Requested-With: stacknav marker on the request and an
X-Location header on the 200 response. Non-stacknav requests still get
a 303 See Other. All call sites are unchanged.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Rewrite `ui/static/main.js` stack navigator

**Files:**
- Modify: `ui/static/main.js`

This task is the core client-side rewrite. The current file is replaced wholesale; see `ui/static/main.js` for the as-built version. Key shape:

- Top-of-file JSDoc block documenting the architecture (mission, wire protocol, navigation strategy, hierarchical back link, why `preventDefault` over `intercept`, progressive enhancement).
- `navigate` listener gated on `'navigation' in window` that filters non-form events and calls `e.preventDefault()` + `submitForm(e)`. We use `preventDefault()` rather than `e.intercept()` because iOS Safari does not yet fire precommit handlers (WebKit bug 293952); revisit when that ships.
- `submitForm` issues the POST with `X-Requested-With: stacknav`, treats 200 + `X-Location` as success and falls back to `location.reload()` on anything else (CSP blocks in-place HTML rendering).
- `popOrReplaceTo` walks `navigation.entries()` backward from the entry behind the cursor, traverses to the first URL match, otherwise calls `navigation.navigate(target, { history: 'replace' })`. One strategy covers every flow.
- `pagereveal` listener uses `navigation.activation` index comparison to set `forward`/`backward` view-transition types; replace/reload skip the transition; null-`from` no longer crashes.
- `click` delegator on `a[data-back-button]` uses the same backward walk; on no match the link's natural href takes over.
- `submit` and `pageshow` listeners handle the `.submitting` class and submit-button disable across bfcache restores.

Key differences from the prior implementation:

- Feature-gated on `'navigation' in window`.
- Uses `X-Requested-With: stacknav` request marker and `X-Location` response header instead of `Content-Location`.
- One history strategy (pop-or-replace); no `X-History-Action` discriminator.
- `popOrReplaceTo` walks entries backward from `currentEntry.index - 1` (was forward, picking oldest match).
- `sameUrl` compares `origin + pathname + search` (was pathname-only).
- `pagereveal` uses `navigation.activation` index comparison instead of URL-depth heuristic; null-`from` no longer crashes; replace/reload skip the transition.
- `data-back-button` click handler uses event delegation, walks backward, compares full URL.
- Validation errors arrive as normal 200 + `X-Location` (server uses flash + redirect-to-form), so the client treats them like any other navigation. The CSP blocks `document.write`/`innerHTML` of HTML strings, so any non-200 response triggers `location.reload()` as the safe fallback.

- [ ] **Step 2: Smoke-check the file by serving it**

Run: `make run` (or whatever the project's local-run is — check `Makefile`).

Expected: server starts. Open `http://localhost:<port>/main.js` in a browser; file should serve without errors.

If you don't have a quick way to start the server, skip this manual check; Task 4 will verify via Playwright.

- [ ] **Step 3: Run static asset tests if any**

Run: `make test`
Expected: passes. (No JS unit tests in this codebase — coverage comes from Playwright in Task 4.)

- [ ] **Step 4: Run lint**

Run: `make lint-fix`
Expected: no Go-side errors. (No JS linter is configured in this project.)

- [ ] **Step 5: Commit**

```bash
git add ui/static/main.js
git commit -m "$(cat <<'EOF'
feat(stacknav): rewrite client to use X-Location protocol

Replaces the Content-Location-based interceptor with a Navigation API
client gated on 'navigation' in window. New behavior:

- X-Requested-With: stacknav request marker
- X-Location response header
- One history strategy (pop-or-replace); walks entries backward,
  compares full URL, falls through to replace on no match
- e.preventDefault() instead of e.intercept() — iOS Safari precommit
  handlers are not yet implemented (WebKit bug 293952)
- pagereveal uses navigation.activation index comparison (no URL-depth)
- Smart back-link uses event delegation and full-URL match
- Feature-gated; no-op without Navigation API
- Non-200 responses fall back to location.reload() (CSP blocks
  in-place HTML rendering)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Unskip Playwright spec, run, fix flakiness, full CI

**Files:**
- Modify: `cmd/web/playwright_test.go`

- [ ] **Step 1: Remove the skip line**

Edit `cmd/web/playwright_test.go` `Test_playwright_stacknav`. Delete the line:

```go
	t.Skip("not yet implemented; see plan tasks 2-5")
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
Expected: passes. The schedule submit flow now exercises the new `redirect` + JS path.

- [ ] **Step 4: Run full CI**

Run: `make ci`
Expected: passes (init + build + lint + test + sec).

- [ ] **Step 5: Commit**

```bash
git add cmd/web/playwright_test.go
git commit -m "$(cat <<'EOF'
test: enable stacknav playwright spec

Verifies the five flows from the design spec end-to-end:
1. Same-URL submit (set update)
2. Cross-URL submit, target absent (swap exercise)
3. Cross-URL submit, target present (schedule submit)
4. Hierarchical back-link (data-back-button)
5. Validation error (empty schedule)

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review Notes

The plan covers all sections of the spec:

- Wire protocol → Tasks 2, 3.
- Per-flow behavior (5 flows) → Task 1 (spec-as-test) + Task 4 (verification).
- Server-side changes → Task 2 (single helper rewrite, all call sites unchanged).
- Client implementation → Task 3 (full file rewrite).
- Testing → Task 1 (Playwright spec) + Task 4 (verification).
- Removed code (`Content-Location`, `Sec-Fetch-Dest` branch) → Task 2.
- Browser support gate (`'navigation' in window`) → Task 3.

Identifiers used consistently: `redirect`, `popOrReplaceTo`, `sameUrl`, `submitForm`, `X-Requested-With: stacknav`, `X-Location`, `Test_playwright_stacknav`.

The Playwright test in Task 1 has Flow 1, 2, 4 navigation that depends on the actual page structure (button names, link selectors). The test code includes notes to run `PWDEBUG=1` and adjust if locators don't match. This is acknowledged-imprecise rather than wrong: the *assertions* (back-button URL after various submits) are exact; the *path to set them up* may need locator tweaks during Task 4.
