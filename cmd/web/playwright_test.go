package main

import (
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

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

	// Step 2b: Submit a valid schedule. Schedule both today and the next weekday so that a
	// midnight crossing between setting preferences and the server's notion of "today" still
	// leaves today scheduled — covering both sides of the boundary.
	testStart := time.Now()
	todayWeekday := testStart.Weekday().String()
	nextWeekday := testStart.AddDate(0, 0, 1).Weekday().String()
	for _, day := range []string{todayWeekday, nextWeekday} {
		if _, err = page.GetByLabel(day).SelectOption(playwright.SelectOptionValues{
			Labels: &[]string{"1 hour"},
		}); err != nil {
			t.Fatalf("select %s duration: %v", day, err)
		}
	}
	if err = startTrackingBtn.Click(); err != nil {
		t.Fatalf("click Start Tracking with valid schedule: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect redirect to / after valid schedule submission: %v", err)
	}

	// Step 3: Start today's workout. Extract today's date from the resulting URL rather than
	// from the test's clock: if midnight crossed between scheduling and now, the server's
	// authoritative "today" is the one that matters for the rest of the flow.
	startWorkoutBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Start Workout"})
	if err = startWorkoutBtn.Click(); err != nil {
		t.Fatalf("click Start Workout: %v", err)
	}
	workoutURLPattern := regexp.MustCompile(fmt.Sprintf(
		`^%s/workouts/\d{4}-\d{2}-\d{2}$`, regexp.QuoteMeta(serverURL)))
	if err = page.WaitForURL(workoutURLPattern); err != nil {
		t.Fatalf("expect navigation to workout page: %v", err)
	}
	workoutURL := page.URL()
	today := workoutURL[len(workoutURL)-len("2006-01-02"):]

	// Step 4: Open the first exercise. The workout page renders each exercise as a link with
	// the exercise name as its accessible text; we pick the first one via its data attribute.
	firstExercise := page.Locator("a[data-exercise-id]").First()
	exerciseName, err := firstExercise.InnerText()
	if err != nil {
		t.Fatalf("read first exercise name: %v", err)
	}
	if err = firstExercise.Click(); err != nil {
		t.Fatalf("click first exercise %q: %v", exerciseName, err)
	}
	exerciseURLPattern := regexp.MustCompile(fmt.Sprintf(`/workouts/%s/exercises/\d+$`, today))
	if err = page.WaitForURL(exerciseURLPattern); err != nil {
		t.Fatalf("expect navigation to exercise page: %v", err)
	}

	// Step 5: Complete the warmup.
	warmupBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Mark Warmup Complete"})
	if err = warmupBtn.Click(); err != nil {
		t.Fatalf("click Mark Warmup Complete: %v", err)
	}
	// The warmup status indicator replaces the banner once complete.
	if err = page.GetByText("Warmup complete").WaitFor(); err != nil {
		t.Fatalf("wait for warmup completion: %v", err)
	}

	// Step 6: Complete every set for this exercise. Both weighted and bodyweight exercises are
	// handled — weighted forms auto-submit when a signal radio is selected, bodyweight forms
	// require filling the reps input and pressing the submit button.
	currentSet := page.GetByRole("group", playwright.PageGetByRoleOptions{Name: "Current set"})
	const maxSetsPerExercise = 10
	for range maxSetsPerExercise {
		var visible bool
		if visible, err = currentSet.IsVisible(); err != nil {
			t.Fatalf("check current set visibility: %v", err)
		}
		if !visible {
			break
		}

		// The weighted radios are visually hidden; the label wrapping each one is what the user
		// actually sees and clicks. Count on the radio locator is enough to detect whether this
		// is a weighted exercise, then we click the associated label to trigger the JS-initiated
		// form submission.
		couldDoMoreRadio := currentSet.GetByRole("radio",
			playwright.LocatorGetByRoleOptions{Name: "Could have done more reps"})
		var radioCount int
		if radioCount, err = couldDoMoreRadio.Count(); err != nil {
			t.Fatalf("count weighted signal radios: %v", err)
		}

		if radioCount > 0 {
			couldDoMoreLabel := currentSet.GetByText("Could do more", playwright.LocatorGetByTextOptions{
				Exact: new(true),
			})
			if err = couldDoMoreLabel.Click(); err != nil {
				t.Fatalf("click Could do more: %v", err)
			}
		} else {
			repsInput := currentSet.GetByRole("textbox", playwright.LocatorGetByRoleOptions{Name: "Reps"})
			if err = repsInput.Fill("10"); err != nil {
				t.Fatalf("fill reps: %v", err)
			}
			completeSetBtn := currentSet.GetByRole("button",
				playwright.LocatorGetByRoleOptions{Name: "Complete set"})
			if err = completeSetBtn.Click(); err != nil {
				t.Fatalf("click Complete set: %v", err)
			}
		}

		// Each submission reloads the exercise page with the next set (or none) active.
		if err = page.WaitForURL(exerciseURLPattern); err != nil {
			t.Fatalf("expect reload of exercise page after set submission: %v", err)
		}
	}

	// Step 7: Navigate back to the workout overview.
	backLink := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Back to workout"})
	if err = backLink.Click(); err != nil {
		t.Fatalf("click Back to workout: %v", err)
	}
	if err = page.WaitForURL(workoutURL); err != nil {
		t.Fatalf("expect navigation to %s: %v", workoutURL, err)
	}

	// Step 8: Complete the workout.
	completeWorkoutBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Complete workout"})
	if err = completeWorkoutBtn.Click(); err != nil {
		t.Fatalf("click Complete workout: %v", err)
	}
	completionURL := fmt.Sprintf("%s/workouts/%s/complete", serverURL, today)
	if err = page.WaitForURL(completionURL); err != nil {
		t.Fatalf("expect navigation to %s: %v", completionURL, err)
	}

	// Step 9: Submit a difficulty rating and land back on the home page.
	justRightBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Just right"})
	if err = justRightBtn.Click(); err != nil {
		t.Fatalf("click Just right: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect redirect to / after submitting feedback: %v", err)
	}

	// Step 10: Logout.
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

	// Step 11: Verify unauthenticated state.
	if err = registerBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Register button after logout: %v", err)
	}

	// Step 12: Login — JS calls window.location.reload() after finishing.
	if err = signInBtn.Click(); err != nil {
		t.Fatalf("login click: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect redirect to / after login: %v", err)
	}
}
