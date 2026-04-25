package main

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"github.com/playwright-community/playwright-go"
)

// smoke test using playwright. To debug, set PWDEBUG=1 environment variable.
// Place a `page.Pause()` in the place you want to debug.
//
// `PWDEBUG=1 go test -count 1 -v -run Test_playwright_smoketest ./cmd/web/`.
func Test_playwright_smoketest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow playwright smoke test")
	}

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

	// Surface browser console errors in test output for easier debugging.
	page.On("console", func(msg playwright.ConsoleMessage) {
		if msg.Type() == "error" {
			t.Logf("browser console error: %s", msg.Text())
		}
	})
	page.On("pageerror", func(err error) {
		t.Logf("browser page error: %v", err)
	})

	if _, err = page.Goto(serverURL + "/"); err != nil {
		t.Fatalf("navigate to home: %v", err)
	}

	// Enable virtual WebAuthn via CDP (Go bindings don't wrap AddVirtualAuthenticator).
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
		t.Fatalf("WebAuthn.addVirtualAuthenticator: %v", err)
	}

	registerBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Register"})
	signInBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Sign in"})
	logOutBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log out"})

	// Step 1: Verify unauthenticated state.
	if err = registerBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Register button: %v", err)
	}
	if err = signInBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Sign in button: %v", err)
	}

	// Step 2: Register — JS calls window.location.reload() after finishing.
	if err = registerBtn.Click(); err != nil {
		t.Fatalf("register click: %v", err)
	}
	// Home handler redirects users with empty preferences to /schedule.
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("expect redirect to /schedule after registration: %v", err)
	}

	// Step 2a: Verify error handling for empty schedule submission.
	startTrackingBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Start Tracking"})
	if err = startTrackingBtn.Click(); err != nil {
		t.Fatalf("click Start Tracking with empty schedule: %v", err)
	}
	validationError := page.GetByRole("alert")
	if err = validationError.WaitFor(); err != nil {
		t.Fatalf("wait for validation error after empty schedule: %v", err)
	}

	// Step 2b: Submit a valid schedule — navigator replaces /schedule with / in history.
	if _, err = page.GetByLabel("Monday").SelectOption(playwright.SelectOptionValues{
		Labels: &[]string{"1 hour"},
	}); err != nil {
		t.Fatalf("select Monday duration: %v", err)
	}
	if err = startTrackingBtn.Click(); err != nil {
		t.Fatalf("click Start Tracking with valid schedule: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect redirect to / after valid schedule submission: %v", err)
	}

	// Step 3: Logout.
	menuLink := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Menu"})
	if err = menuLink.Click(); err != nil {
		t.Fatalf("click Menu link: %v", err)
	}
	if err = logOutBtn.Click(); err != nil {
		t.Fatalf("logout click: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect redirect to / after logout: %v", err)
	}

	// Step 4: Verify unauthenticated state.
	if err = registerBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Register button after logout: %v", err)
	}

	// Step 5: Login — JS calls window.location.reload() after finishing.
	if err = signInBtn.Click(); err != nil {
		t.Fatalf("login click: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect redirect to / after login: %v", err)
	}
}

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
