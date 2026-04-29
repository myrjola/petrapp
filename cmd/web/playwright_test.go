package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"github.com/playwright-community/playwright-go"
)

// setupPlaywrightPage installs Playwright, starts the app server, launches
// Chromium, opens a page at "/", and wires a virtual WebAuthn authenticator
// via CDP. Uncaught JS exceptions fail the test; console errors are logged.
//
//nolint:ireturn // playwright.Page is the public interface from the binding
func setupPlaywrightPage(t *testing.T) (playwright.Page, string) {
	t.Helper()

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

	// Console errors are logged for debugging — they include benign network
	// resource failures (e.g., stale-history 404s per Flow 2 in the stacknav
	// spec) that don't indicate JS bugs. Uncaught JS exceptions surface
	// through pageerror and fail the test.
	page.On("console", func(msg playwright.ConsoleMessage) {
		if msg.Type() == "error" {
			t.Logf("browser console error: %s", msg.Text())
		}
	})
	page.On("pageerror", func(err error) {
		t.Errorf("browser page error: %v", err)
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

	return page, serverURL
}

// smoke test using playwright. To debug, set PWDEBUG=1 environment variable.
// Place a `page.Pause()` in the place you want to debug.
//
// `PWDEBUG=1 go test -count 1 -v -run Test_playwright_smoketest ./cmd/web/`.
func Test_playwright_smoketest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow playwright smoke test")
	}

	page, serverURL := setupPlaywrightPage(t)
	var err error

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
	firstExercise := page.Locator("a[data-workout-exercise-id]").First()
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

		// Weighted exercises show three named signal submit buttons; bodyweight exercises show
		// a single "Complete set" submit button and require filling the reps input first.
		couldDoMoreBtn := currentSet.GetByRole("button",
			playwright.LocatorGetByRoleOptions{Name: "Could have done more reps"})
		var btnCount int
		if btnCount, err = couldDoMoreBtn.Count(); err != nil {
			t.Fatalf("count weighted signal buttons: %v", err)
		}

		if btnCount > 0 {
			if err = couldDoMoreBtn.Click(); err != nil {
				t.Fatalf("click Could have done more reps: %v", err)
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

// Test_playwright_stacknav verifies that the stack navigator behaves like a native
// mobile app for the five core flows defined in
// docs/superpowers/specs/2026-04-25-stack-navigator-redesign-design.md.
//
// The flows:
//
//  1. Same-URL replace (set update): submit on DETAIL → land at DETAIL, back goes
//     to parent (workout day overview), not the same DETAIL page.
//  2. Cross-URL replace (swap): submit on SWAP redirecting to a different DETAIL
//     → land at DETAIL', back goes to the previous DETAIL (acceptable per spec),
//     second back goes to workout overview.
//  3. Pop-or-replace (schedule): submit /schedule when / is in history → cursor
//     traverses to /, page reloads (bfcache marker bust), back exits the app rather
//     than returning to /schedule.
//  4. Hierarchical back-link (data-back-button on swap page): click an in-page
//     "back to detail" link → traverse to existing detail entry rather than push.
//  5. Validation error: submit empty schedule → URL stays at /schedule, alert role
//     visible, no new history entry pushed.
func Test_playwright_stacknav(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow playwright stacknav test")
	}

	page, serverURL := setupPlaywrightPage(t)
	var err error

	// Setup: register and configure all weekdays so today always has a workout.
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

	// === Flow 3 setup: fill all weekdays, then submit to get to /. ===
	// Because the server has no preferences yet, navigating to / redirects to
	// /schedule — meaning / never lands in the navigation stack.  To get / into
	// history we first submit the schedule once (landing at /) and then revisit
	// /schedule directly.  The second submit is the actual Flow 3 exercise.
	for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"} {
		if _, err = page.GetByLabel(day).SelectOption(playwright.SelectOptionValues{
			Labels: &[]string{"1 hour"},
		}); err != nil {
			t.Fatalf("select %s duration: %v", day, err)
		}
	}
	// First submit: prefs saved, server responds with pop-or-replace → /.
	// No / in history yet, so popOrReplaceTo falls back to replaceTo(/).
	// History after: [..., /]  (the /schedule entry is replaced).
	if err = startTrackingBtn.Click(); err != nil {
		t.Fatalf("first schedule submit: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect / after first schedule submit: %v", err)
	}
	// Navigate to /schedule directly — now history is [..., /, /schedule].
	if _, err = page.Goto(serverURL + "/schedule"); err != nil {
		t.Fatalf("goto /schedule for flow 3 setup: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("wait for /schedule: %v", err)
	}

	// === Flow 3: pop-or-replace (schedule submit) ===
	// Re-fill the form, then submit. Now / IS in history so popOrReplaceTo
	// traverses to it instead of pushing — /schedule is removed from history.
	for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"} {
		if _, err = page.GetByLabel(day).SelectOption(playwright.SelectOptionValues{
			Labels: &[]string{"1 hour"},
		}); err != nil {
			t.Fatalf("re-select %s: %v", day, err)
		}
	}
	if err = startTrackingBtn.Click(); err != nil {
		t.Fatalf("submit valid schedule (flow 3): %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect / after flow 3 schedule submit: %v", err)
	}
	// Verify pop-or-replace traversed (rather than pushed): /schedule must be
	// in the FORWARD stack (not backward), which we prove by GoForward returning
	// to /schedule.  A push-based implementation would instead have /schedule in
	// the backward stack and GoForward would go somewhere else.
	if _, err = page.GoForward(); err != nil {
		t.Fatalf("Flow 3: GoForward after schedule submit: %v", err)
	}
	if got, want := page.URL(), serverURL+"/schedule"; got != want {
		t.Errorf("Flow 3: forward from / = %q, want %q (traversed /schedule must be in forward stack)", got, want)
	}

	// Return to / for the remaining flows.
	if _, err = page.Goto(serverURL + "/"); err != nil {
		t.Fatalf("goto home: %v", err)
	}

	// === Flow 1, 2, 4 require navigating into a workout day. Today's workout
	// has not been started yet, so the home page shows a "Start Workout" form
	// button (not a plain link). Click it — the navigation API intercepts the
	// POST and calls replaceTo(/workouts/{date}), landing us on the workout page.
	startWorkoutBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Start Workout"})
	if err = startWorkoutBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Start Workout button on home: %v", err)
	}
	if err = startWorkoutBtn.Click(); err != nil {
		t.Fatalf("click Start Workout: %v", err)
	}
	// Wait for the final workout page URL (/workouts/YYYY-MM-DD, not /start).
	// The navigate API commits the form's destination URL (/start) first, then
	// replaceTo() fires and the URL settles at the workout overview page.
	if err = page.WaitForURL(func(u string) bool {
		return strings.Contains(u, "/workouts/") &&
			!strings.Contains(u, "/start") &&
			!strings.Contains(u, "/exercises/")
	}); err != nil {
		t.Fatalf("wait for workout URL after Start Workout: %v", err)
	}
	// Record the workout URL for later assertions.
	workoutURL := page.URL()
	workoutHref := strings.TrimPrefix(workoutURL, serverURL)

	// The weekly planner exhausts its exercise pool after a few days when
	// scheduling all 7 days (weekUsedExercises prevents reuse). So later in the
	// week (Saturday) may end up with 0 exercises. Add one manually if needed.
	if noExercises, _ := page.Locator("a[href*='/exercises/']").Count(); noExercises == 0 {
		addExerciseToWorkout(t, page, workoutURL)
	}

	// Click first exercise link to land on DETAIL.
	exerciseLink := page.Locator("a[href*='/exercises/']").First()
	if err = exerciseLink.WaitFor(); err != nil {
		if content, contentErr := page.Content(); contentErr == nil {
			t.Logf("workout page content on timeout:\n%s", content)
		}
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
	// All test exercises are weighted. The page shows a warmup banner first —
	// complete it before the set form appears. Warmup uses replaceTo(same-URL),
	// so history does not grow.
	warmupBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Mark Warmup Complete"})
	if err = warmupBtn.WaitFor(); err != nil {
		t.Fatalf("wait for warmup button: %v", err)
	}
	if err = warmupBtn.Click(); err != nil {
		t.Fatalf("click warmup button: %v", err)
	}
	// Wait for the reload triggered by replaceTo(same-URL) to fully settle.
	if err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateLoad}); err != nil {
		t.Fatalf("expect load after warmup complete: %v", err)
	}

	// Submit the first set via the "No" (too-heavy) signal path: select the
	// radio, fill actual reps, then click Submit.  The "Barely" / "Could do
	// more" labels auto-submit via form.submit() which may not carry formData
	// to the navigation API, so we use the explicit submit-button path instead.
	if err = page.Locator("label.too-heavy-btn").First().Click(); err != nil {
		t.Fatalf("click No (too-heavy) signal: %v", err)
	}
	// The reps-section is now visible via CSS :has selector.
	if err = page.GetByLabel("Actual reps").First().Fill("8"); err != nil {
		t.Fatalf("fill actual reps: %v", err)
	}
	submitBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Submit"}).First()
	if err = submitBtn.Click(); err != nil {
		t.Fatalf("click Submit set button: %v", err)
	}
	// After the set update, replaceTo(same-URL) fires history.replaceState +
	// location.reload().  Wait for the reload-triggered page navigation to fully
	// commit by checking that the first set now shows as completed.  This is more
	// reliable than WaitForURL (URL unchanged) or WaitForLoadState (already-load
	// resolves immediately).
	if err = page.Locator(".exercise-set.completed").First().WaitFor(); err != nil {
		t.Fatalf("Flow 1: expected first set completed after set update: %v", err)
	}
	if got, want := page.URL(), detailURL; got != want {
		t.Errorf("Flow 1: URL after set update = %q, want %q", got, want)
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
	// Pick a different exercise on the swap page. Each alternative exercise has
	// a "Swap to this exercise" submit button.
	swapBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Swap to this exercise"}).First()
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
	// After Flow 2, the original exercise (detailURL) was swapped out — it is
	// no longer part of the workout. Use newDetailURL (the swapped-in exercise)
	// to set up [WORKOUT, newDETAIL, newSWAP] history state.
	if _, err = page.Goto(newDetailURL); err != nil {
		t.Fatalf("goto newDetailURL for flow 4: %v", err)
	}
	if err = page.Locator("a[href$='/swap']").First().Click(); err != nil {
		t.Fatalf("click swap from new detail: %v", err)
	}
	flow4SwapURL := page.URL()
	backLink := page.Locator("a[data-back-button]")
	if err = backLink.WaitFor(); err != nil {
		t.Fatalf("wait for data-back-button: %v", err)
	}
	if err = backLink.Click(); err != nil {
		t.Fatalf("click data-back-button: %v", err)
	}
	// Should traverse to existing newDETAIL entry rather than push.
	if err = page.WaitForURL(newDetailURL); err != nil {
		t.Fatalf("Flow 4: expected newDetailURL %s after data-back-button: %v", newDetailURL, err)
	}
	// Forward should go to SWAP (proves it was a traverse, not a push).
	if _, err = page.GoForward(); err != nil {
		t.Fatalf("Flow 4 forward: %v", err)
	}
	if got, want := page.URL(), flow4SwapURL; got != want {
		t.Errorf("Flow 4: forward URL = %q, want %q (traverse should preserve forward stack)", got, want)
	}
}

// addExerciseToWorkout navigates from the workout page to the add-exercise page,
// adds the first available exercise, and waits to return to workoutURL.
// Used when the weekly planner exhausts its exercise pool and creates an empty workout.
func addExerciseToWorkout(t *testing.T, page playwright.Page, workoutURL string) {
	t.Helper()
	var err error
	if err = page.Locator("a.add-exercise-button").Click(); err != nil {
		t.Fatalf("click Add Exercise link: %v", err)
	}
	if err = page.WaitForURL(func(u string) bool { return strings.Contains(u, "/add-exercise") }); err != nil {
		t.Fatalf("wait for add-exercise page: %v", err)
	}
	addBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Add this exercise"}).First()
	if err = addBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Add this exercise button: %v", err)
	}
	if err = addBtn.Click(); err != nil {
		t.Fatalf("click Add this exercise: %v", err)
	}
	// The POST replaces the add-exercise entry back to workoutURL.
	if err = page.WaitForURL(workoutURL); err != nil {
		t.Fatalf("wait to return to workout page after add: %v", err)
	}
}
