package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
	playwright "github.com/playwright-community/playwright-go"
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

	// Step 1: Verify unauthenticated state.
	if err = page.Locator("button:has-text('Register')").WaitFor(); err != nil {
		t.Fatalf("wait for Register button: %v", err)
	}
	if err = page.Locator("button:has-text('Sign in')").WaitFor(); err != nil {
		t.Fatalf("wait for Sign in button: %v", err)
	}

	// Step 2: Register — JS calls window.location.reload() after finishing.
	if err = page.Locator("button:has-text('Register')").Click(); err != nil {
		t.Fatalf("register click: %v", err)
	}
	// Home handler redirects users with empty preferences to /schedule.
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("expect redirect to /schedule after registration: %v", err)
	}

	// Step 3: Logout.
	if _, err = page.Goto(serverURL + "/preferences"); err != nil {
		t.Fatalf("navigate to preferences: %v", err)
	}
	if err = page.Locator("button:has-text('Log out')").Click(); err != nil {
		t.Fatalf("logout click: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect redirect to / after logout: %v", err)
	}

	// Step 4: Verify unauthenticated state.
	if err = page.Locator("button:has-text('Register')").WaitFor(); err != nil {
		t.Fatalf("wait for Register button after logout: %v", err)
	}

	// Step 5: Login — JS calls window.location.reload() after finishing.
	if err = page.Locator("button:has-text('Sign in')").Click(); err != nil {
		t.Fatalf("login click: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("expect redirect to /schedule after login: %v", err)
	}
}
