package main

import (
	"fmt"
	"os"
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

// Test_playwright_schedule_wednesday_submission demonstrates a form submission bug
// in the MPA stack navigator: when a newly registered user selects Wednesday with
// 1 hour on the /schedule page and submits, the page stays on /schedule instead of
// navigating to /. The expected behaviour matches the Monday case in the smoketest.
func Test_playwright_schedule_wednesday_submission(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow playwright test")
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
	if err = registerBtn.Click(); err != nil {
		t.Fatalf("register click: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("expect redirect to /schedule after registration: %v", err)
	}

	// Select Wednesday with "1 hour" and submit — navigator should replace /schedule with / in history.
	if _, err = page.GetByLabel("Wednesday").SelectOption(playwright.SelectOptionValues{
		Labels: &[]string{"1 hour"},
	}); err != nil {
		t.Fatalf("select Wednesday duration: %v", err)
	}
	startTrackingBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Start Tracking"})
	if err = startTrackingBtn.Click(); err != nil {
		t.Fatalf("click Start Tracking with Wednesday 1 hour: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect redirect to / after submitting Wednesday 1 hour schedule: %v", err)
	}
}
