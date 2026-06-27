package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/platform/testkit"
	"github.com/playwright-community/playwright-go"
)

// installPlaywrightOnce guards playwright.Install against concurrent calls
// from parallel tests. Two parallel installs race on the driver's node
// binary (one writes/holds it open while another tries to exec it),
// producing "text file busy" (ETXTBSY) in CI.
//
//nolint:gochecknoglobals // sync.OnceValue must be package-scoped to share across tests.
var installPlaywrightOnce = sync.OnceValue(func() error {
	return playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
		Verbose:  false,
	})
})

// setupPlaywrightPage installs Playwright, starts the app server, launches
// Chromium, opens a page at "/", and wires a virtual WebAuthn authenticator
// via CDP. Uncaught JS exceptions and unexpected console errors fail the
// test; a screenshot is captured on any failure for post-mortem.
//
// allowedConsoleErrors lets a test opt-in to specific known-benign console
// errors (matched by substring on the message text). Anything not on the
// list fails the test in a t.Cleanup. Tests with no benign errors pass no
// arguments.
//
// allowedConsoleErrors is an extension point that current tests don't exercise.
//
//nolint:ireturn,unparam // playwright.Page is the public interface from the binding;
func setupPlaywrightPage(t *testing.T, allowedConsoleErrors ...string) (playwright.Page, string) {
	t.Helper()

	if err := installPlaywrightOnce(); err != nil {
		t.Fatalf("install playwright browsers: %v", err)
	}

	server, err := e2etest.StartServer(t, testkit.NewWriter(t), testLookupEnv, run)
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

	// Emulate prefers-reduced-motion: reduce. The app's CSS honors this
	// media feature and collapses view transitions to 0.001ms, removes
	// button transforms, and disables the loading-bar animation — all of
	// which Playwright waits on for actionability/stability checks.
	//
	// The viewport defaults to a portrait phone size (iPhone 13). Petra
	// is a mobile-first PWA — running e2e flows at desktop dimensions
	// exercises a layout users never see.
	bCtx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		ReducedMotion: playwright.ReducedMotionReduce,
		Viewport:      &playwright.Size{Width: 390, Height: 844},
	})
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

	// Console errors fail the test unless the caller allowlisted them. Real
	// JS bugs land here as console.error; benign cases (e.g., resource
	// fetches the app deliberately ignores) must be explicitly allowed by
	// passing a substring match to setupPlaywrightPage. Uncaught JS
	// exceptions surface through pageerror and always fail.
	var (
		consoleErrMu  sync.Mutex
		consoleErrors []string
	)
	page.On("console", func(msg playwright.ConsoleMessage) {
		if msg.Type() != "error" {
			return
		}
		text := msg.Text()
		for _, allowed := range allowedConsoleErrors {
			if strings.Contains(text, allowed) {
				t.Logf("allowed browser console error: %s", text)
				return
			}
		}
		consoleErrMu.Lock()
		consoleErrors = append(consoleErrors, text)
		consoleErrMu.Unlock()
	})
	page.On("pageerror", func(err error) {
		t.Errorf("browser page error: %v", err)
	})
	t.Cleanup(func() {
		consoleErrMu.Lock()
		defer consoleErrMu.Unlock()
		if len(consoleErrors) > 0 {
			t.Errorf("%d unexpected browser console error(s):\n  %s",
				len(consoleErrors), strings.Join(consoleErrors, "\n  "))
		}
	})

	// Screenshot-on-failure: any t.Errorf / t.Fatalf in the test makes the
	// browser state visible after the run. The file lands in os.TempDir()
	// (preserved past the test exit) and its path is logged so CI artifact
	// upload or local inspection can find it.
	t.Cleanup(func() { capturePlaywrightFailureScreenshot(t, page) })

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
// `PWDEBUG=1 go test -count 1 -v -run Test_playwright_smoketest ./cmd/petra/`.
func Test_playwright_smoketest(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping slow playwright smoke test")
	}

	page, serverURL := setupPlaywrightPage(t)
	var err error

	signInBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Sign in"})
	registerBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Begin training"})
	logOutBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log out"})

	// Step 1: Verify unauthenticated state — both auth buttons must be present.
	if err = registerBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Begin training button: %v", err)
	}
	if err = signInBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Sign in button: %v", err)
	}

	// Step 2: Register, land on /schedule (home redirects empty-prefs users).
	registerAndWaitSchedule(t, page, serverURL)

	// Step 2a: Verify error handling for empty schedule submission.
	startTrackingBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Start Tracking"})
	if err = startTrackingBtn.Click(); err != nil {
		t.Fatalf("click Start Tracking with empty schedule: %v", err)
	}
	if err = page.GetByRole("alert").WaitFor(); err != nil {
		t.Fatalf("wait for validation error after empty schedule: %v", err)
	}

	// Step 2b: Submit a valid schedule. Schedule today AND tomorrow so a
	// midnight crossing between selection and the server's notion of "today"
	// still leaves today scheduled — covers both sides of the boundary.
	selectAndSubmitSchedule(t, page, serverURL, todayAndTomorrowWeekdays())

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
	warmupBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Mark done"})
	if err = warmupBtn.Click(); err != nil {
		t.Fatalf("click Mark done: %v", err)
	}
	// The warmup status indicator replaces the banner once complete.
	if err = page.GetByText("Warmup complete").WaitFor(); err != nil {
		t.Fatalf("wait for warmup completion: %v", err)
	}

	// Step 6: Complete every set for this exercise. Both weighted and bodyweight exercises are
	// handled — weighted forms auto-submit when a signal radio is selected, bodyweight forms
	// require filling the reps input and pressing the submit button.
	currentSet := page.GetByRole("group", playwright.PageGetByRoleOptions{Name: "Current set"})
	completedSets := page.Locator(".set-card.done")
	const maxSetsPerExercise = 10
	for range maxSetsPerExercise {
		var visible bool
		if visible, err = currentSet.IsVisible(); err != nil {
			t.Fatalf("check current set visibility: %v", err)
		}
		if !visible {
			break
		}

		// Snapshot the completed-set count before submitting so we can wait for the
		// post-submit reload to commit. WaitForURL is unreliable here: each submit
		// goes through popOrPushTo with the same target URL (auto-replaced by the
		// client), so the URL never changes and the wait can return before the new
		// DOM has rendered.
		var completedBefore int
		if completedBefore, err = completedSets.Count(); err != nil {
			t.Fatalf("count completed sets: %v", err)
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

		// Wait for the new DOM to render: the just-submitted set must show as
		// completed before we trust currentSet.IsVisible() on the next iteration.
		// page.WaitForURL is not enough — the URL is unchanged across replace
		// navigations, so it can resolve against the pre-submit DOM.
		if err = completedSets.Nth(completedBefore).WaitFor(); err != nil {
			t.Fatalf("expect set %d to render as completed: %v", completedBefore+1, err)
		}
		if got := page.URL(); !exerciseURLPattern.MatchString(got) {
			t.Fatalf("expect URL to match %s after set submission, got %q", exerciseURLPattern, got)
		}
	}

	// Step 7: Navigate back to the workout overview.
	backLink := page.Locator("a[data-back-button]")
	if err = backLink.Click(); err != nil {
		t.Fatalf("click Back to workout: %v", err)
	}
	if err = page.WaitForURL(workoutURL); err != nil {
		t.Fatalf("expect navigation to %s: %v", workoutURL, err)
	}

	// Step 8: Complete the workout.
	completeWorkoutBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Finish workout"})
	if err = completeWorkoutBtn.Click(); err != nil {
		t.Fatalf("click Finish workout: %v", err)
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
	settingsLink := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Settings"})
	if err = settingsLink.Click(); err != nil {
		t.Fatalf("click Settings link: %v", err)
	}
	if err = logOutBtn.Click(); err != nil {
		t.Fatalf("logout click: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect redirect to / after logout: %v", err)
	}

	// Step 11: Verify unauthenticated state.
	if err = registerBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Begin training button after logout: %v", err)
	}

	// Step 12: Login — JS calls window.location.reload() after finishing.
	// The reload lands back on "/", so the page URL never changes. Waiting on
	// the URL resolves immediately (Playwright treats an already-current URL as
	// satisfied), which would let the test return while POST /api/login/finish
	// is still in flight — the server shuts down on test return and the request
	// is refused. Wait for an authenticated-only element instead: the Settings
	// link renders only after the post-login reload commits.
	if err = signInBtn.Click(); err != nil {
		t.Fatalf("login click: %v", err)
	}
	if err = settingsLink.WaitFor(); err != nil {
		t.Fatalf("expect authenticated home after login: %v", err)
	}
}

// Test_playwright_stacknav verifies that the stack navigator behaves like a native
// mobile app for the six core flows of the pop-or-push protocol defined in
// cmd/petra/README.md "The stack-navigator wire protocol" (rationale in
// docs/adr/0002-stack-navigator-mpa-enhancement.md).
//
// The flows:
//
//  1. Same-URL replace (set update): submit on DETAIL → client auto-detects
//     same-URL and replaces in place, so back goes to the workout overview.
//  2. Cross-URL submit, target present (swap): swap redirects to the same
//     workoutExerciseID slot, so popOrPushTo traverses to the original DETAIL.
//  3. Pop-or-push, traverse branch (schedule): submit /schedule when / is in
//     history → traverse to /; back exits the app, forward returns to /schedule.
//  4. Hierarchical back-link (data-back-button on swap page): click an in-page
//     "back to detail" link → traverse to existing detail entry rather than push.
//  5. Validation error: submit empty schedule → flash + redirect-to-form is
//     same-URL, client auto-replaces, alert visible, no history entry pushed.
//  6. Add-exercise replace: submit add-exercise → server sends X-Replace-URL,
//     client replaces /add-exercise with the new exercise's DETAIL, back goes
//     to the workout overview.
func Test_playwright_stacknav(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping slow playwright stacknav test")
	}

	page, serverURL := setupPlaywrightPage(t)
	var err error

	// Setup: register and land on /schedule.
	registerAndWaitSchedule(t, page, serverURL)

	// === Flow 5: Validation error (empty schedule submit). Run before filling
	// the form so we see the error path first. The flash + redirect-to-form
	// pattern lands at the same URL the form was submitted from, which the
	// client auto-detects as same-URL and replaces in place. ===
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
	// /schedule — meaning / never lands in the navigation stack from a normal
	// click. To exercise the "pop" branch in Flow 3 below, we first submit the
	// schedule once (landing at /) and then revisit /schedule directly. The
	// second submit is the actual Flow 3 exercise.
	//
	// First submit: prefs saved, server responds with pop-or-push → /.
	// No / in history yet, so popOrPushTo pushes / on top of /schedule.
	// History after: [..., /schedule, /].
	selectAndSubmitSchedule(t, page, serverURL, allWeekdays())
	// Navigate to /schedule directly — now history is [..., /, /schedule].
	if _, err = page.Goto(serverURL + "/schedule"); err != nil {
		t.Fatalf("goto /schedule for flow 3 setup: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("wait for /schedule: %v", err)
	}

	// === Flow 3: pop-or-push, traverse branch (schedule submit) ===
	// Re-fill the form, then submit. Now / IS in history (we navigated back to
	// /schedule via Goto, which pushed another /schedule on top), so popOrPushTo
	// traverses to / instead of pushing a duplicate.
	selectAndSubmitSchedule(t, page, serverURL, allWeekdays())
	// Verify pop-or-push traversed (rather than pushed): /schedule must be in
	// the FORWARD stack (not backward), which we prove by GoForward returning
	// to /schedule. A naive push would have placed a fresh / on top of the
	// existing /schedule, leaving the forward stack empty.
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
	// POST and calls popOrPushTo(/workouts/{date}); since the target is brand-new
	// in history this pushes (the bug fix this redesign delivered). Back will
	// return to / from the workout page.
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
	// popOrPushTo fires and the URL settles at the workout overview page.
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

	// Flow 1 below fills "Actual reps" and clicks the named "Too heavy" signal
	// button — controls that only the weighted and assisted set forms render.
	// Two planner outcomes can leave the workout without a usable exercise:
	// (1) on all-7-day schedules the weekUsedExercises rule exhausts the pool
	// by Saturday/Sunday, leaving 0 exercises; (2) on any day the planner can
	// pick only bodyweight or time_based exercises. Swap one in via the
	// add-exercise picker (also filtered by exercise type) when missing.
	weightedSelector := `a[data-workout-exercise-id][data-exercise-type="weighted"], ` +
		`a[data-workout-exercise-id][data-exercise-type="assisted"]`
	weightedCount, err := page.Locator(weightedSelector).Count()
	if err != nil {
		t.Fatalf("count weighted/assisted workout exercises: %v", err)
	}
	if weightedCount == 0 {
		addWeightedExerciseToWorkout(t, page, workoutURL)
	}

	// Click the first weighted/assisted exercise link to land on DETAIL.
	exerciseLink := page.Locator(weightedSelector).First()
	if err = exerciseLink.WaitFor(); err != nil {
		if content, contentErr := page.Content(); contentErr == nil {
			t.Logf("workout page content on timeout:\n%s", content)
		}
		t.Fatalf("wait for weighted/assisted exercise link: %v", err)
	}
	exerciseHref, err := exerciseLink.GetAttribute("href")
	if err != nil {
		t.Fatalf("get exercise href: %v", err)
	}
	if err = exerciseLink.Click(); err != nil {
		t.Fatalf("click exercise link: %v", err)
	}
	wantDetailURL := fmt.Sprintf("%s%s", serverURL, exerciseHref)
	if err = page.WaitForURL(wantDetailURL); err != nil {
		dumpNavDiagnostics(t, page, "workout-to-detail link click", wantDetailURL)
		t.Fatalf("expect %s: %v", exerciseHref, err)
	}
	detailURL := page.URL()

	// === Flow 1: same-URL replace (set update) ===
	// The exercise selected above is weighted or assisted. The page shows a
	// warmup banner first — complete it before the set form appears. The
	// set/warmup POSTs redirect back to the same DETAIL URL, and the client
	// auto-detects same-URL and replaces in place — so history does not grow
	// on either submit.
	warmupBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Mark done"})
	if err = warmupBtn.WaitFor(); err != nil {
		t.Fatalf("wait for warmup button: %v", err)
	}
	if err = warmupBtn.Click(); err != nil {
		t.Fatalf("click warmup button: %v", err)
	}
	// Wait for the reload triggered by the same-URL auto-replace to fully settle.
	if err = page.WaitForLoadState(
		playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateLoad},
	); err != nil {
		t.Fatalf("expect load after warmup complete: %v", err)
	}

	// Submit the first set via the "too heavy" signal submit button. The reps
	// input is pre-filled with the target; we override it before clicking the
	// named submit button, which sends signal=too_heavy along with the form.
	if err = page.GetByLabel("Actual reps").First().Fill("8"); err != nil {
		t.Fatalf("fill actual reps: %v", err)
	}
	tooHeavyBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Too heavy"}).First()
	if err = tooHeavyBtn.Click(); err != nil {
		t.Fatalf("click too-heavy signal: %v", err)
	}
	// After the set update, navigation.navigate(target, {history: 'replace'})
	// fires (target equals current URL → client auto-replaces). Wait for the
	// resulting reload-triggered navigation to fully commit by checking that
	// the first set now shows as completed. This is more reliable than
	// WaitForURL (URL unchanged) or WaitForLoadState (already-load resolves
	// immediately).
	if err = page.Locator(".set-card.done").First().WaitFor(); err != nil {
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

	// === Flow 2: cross-URL submit, target present (swap exercise) ===
	// Swap redirects to the same workoutExerciseID slot, so the URL matches
	// the original DETAIL the user came from. popOrPushTo finds it in the
	// backward stack and traverses to it.
	swapLink := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Swap exercise"}).First()
	if err = swapLink.WaitFor(); err != nil {
		t.Fatalf("wait for swap link: %v", err)
	}
	if err = swapLink.Click(); err != nil {
		t.Fatalf("click swap link: %v", err)
	}
	// Pick a different exercise on the swap page. Each alternative exercise has
	// a "Swap to this exercise" submit button.
	swapBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Swap to this exercise"}).First()
	if err = swapBtn.WaitFor(); err != nil {
		t.Fatalf("wait for swap button: %v", err)
	}
	// Capture swapURL only after the swap-only button is visible. Capturing
	// before this wait can return the pre-click DETAIL URL — and since swap
	// redirects back to the same slot URL, the post-submit "url != swapURL"
	// predicate below would never match (the new detail URL equals the stale
	// capture) and the test would time out.
	swapURL := page.URL()
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
	if err = page.GetByRole("link",
		playwright.PageGetByRoleOptions{Name: "Swap exercise"}).First().Click(); err != nil {
		t.Fatalf("click swap from new detail: %v", err)
	}
	// Both the previous DETAIL page and the SWAP page render an
	// `a[data-back-button]`, so we must wait for a swap-only element before
	// locating the back-link — otherwise a fast click could fire on the old
	// page's back-link (whose href points at the workout overview, not the
	// detail) and the WaitForURL below would never see newDetailURL.
	if err = page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Swap to this exercise"}).First().WaitFor(); err != nil {
		t.Fatalf("Flow 4: wait for swap page: %v", err)
	}
	flow4SwapURL := page.URL()
	backLink := page.Locator("a[data-back-button]")
	if err = backLink.Click(); err != nil {
		t.Fatalf("click data-back-button: %v", err)
	}
	// Should traverse to existing newDETAIL entry rather than push.
	if err = page.WaitForURL(newDetailURL); err != nil {
		dumpNavDiagnostics(t, page, "Flow 4 data-back-button traversal", newDetailURL)
		t.Fatalf("Flow 4: expected newDetailURL %s after data-back-button: %v", newDetailURL, err)
	}
	// Forward should go to SWAP (proves it was a traverse, not a push).
	if _, err = page.GoForward(); err != nil {
		t.Fatalf("Flow 4 forward: %v", err)
	}
	if got, want := page.URL(), flow4SwapURL; got != want {
		t.Errorf("Flow 4: forward URL = %q, want %q (traverse should preserve forward stack)", got, want)
	}

	// === Flow 6: add-exercise replace ===
	// Click the add-exercise link from the workout overview, pick an exercise,
	// and assert that the POST replaces /add-exercise with the new exercise's
	// DETAIL page (back goes to the workout overview, not the picker).
	if _, err = page.Goto(workoutURL); err != nil {
		t.Fatalf("Flow 6: goto workoutURL: %v", err)
	}
	addExerciseLink := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Add exercise"})
	if err = addExerciseLink.WaitFor(); err != nil {
		t.Fatalf("Flow 6: wait for add-exercise link: %v", err)
	}
	if err = addExerciseLink.Click(); err != nil {
		t.Fatalf("Flow 6: click add-exercise link: %v", err)
	}
	if err = page.WaitForURL(func(u string) bool { return strings.Contains(u, "/add-exercise") }); err != nil {
		t.Fatalf("Flow 6: wait for /add-exercise: %v", err)
	}

	// Capture the name of the first available exercise to verify we land on its DETAIL.
	firstAvailableName, err := page.Locator(".exercise-result .exercise-result__name").First().InnerText()
	if err != nil {
		t.Fatalf("Flow 6: read first available exercise name: %v", err)
	}

	addThisBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Add this exercise"}).First()
	if err = addThisBtn.Click(); err != nil {
		t.Fatalf("Flow 6: click Add this exercise: %v", err)
	}

	// Should land on the new exercise's DETAIL page.
	flow6DetailPattern := regexp.MustCompile(fmt.Sprintf(
		`/workouts/%s/exercises/\d+$`, regexp.QuoteMeta(strings.TrimPrefix(workoutHref, "/workouts/"))))
	if err = page.WaitForURL(flow6DetailPattern); err != nil {
		t.Fatalf("Flow 6: wait for new DETAIL URL: %v", err)
	}

	// Verify it's the exercise we picked.
	gotHeading, err := page.Locator("h1").First().InnerText()
	if err != nil {
		t.Fatalf("Flow 6: read DETAIL heading: %v", err)
	}
	if !strings.Contains(strings.TrimSpace(gotHeading), strings.TrimSpace(firstAvailableName)) {
		t.Errorf("Flow 6: DETAIL heading = %q, want to contain %q", gotHeading, firstAvailableName)
	}

	// Back should land on the workout overview, NOT /add-exercise (proves replace).
	if _, err = page.GoBack(); err != nil {
		t.Fatalf("Flow 6: GoBack: %v", err)
	}
	if got, want := page.URL(), workoutURL; got != want {
		t.Errorf("Flow 6: back from new DETAIL = %q, want %q (add-exercise should be replaced)", got, want)
	}

	// Forward should NOT return to /add-exercise — that entry was destroyed.
	if _, err = page.GoForward(); err != nil {
		t.Fatalf("Flow 6: GoForward: %v", err)
	}
	if got := page.URL(); strings.Contains(got, "/add-exercise") {
		t.Errorf("Flow 6: forward URL = %q contains /add-exercise — it should have been replaced", got)
	}
}

// Test_playwright_preferences_fragment_redirect drives the bug where the new
// /preferences/* POST handlers redirect with #panel fragments (e.g.
// /preferences#deload-title). The stack-navigator's sameUrl() compares
// origin+pathname+search only, so a fragment-only redirect lands in the
// replace branch of popOrPushTo and resolves as a same-document
// hash-change navigation — the document is never re-fetched, the GET
// handler that pops the flash never runs, and the success banner stays
// stuck in the session. The next unrelated GET (e.g. clicking back to
// the home page or a workout) pops the orphaned flash and renders it on
// the wrong page.
//
// Before the fix: the success banner never appears on /preferences and
// the message later surfaces on another page (workoutGET hard-codes the
// variant to error, so a success message renders as an error banner).
//
// After the fix: the banner renders inside the recovery panel, the
// session flash is consumed, and navigating away leaves the next page
// banner-free.
func Test_playwright_preferences_fragment_redirect(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping slow playwright preferences-flash test")
	}

	page, serverURL := setupPlaywrightPage(t)
	var err error

	// Shrink the viewport so the #deload-title heading and the "Save
	// recovery settings" submit button cannot fit on screen together.
	// Even at the suite's mobile default (390x844) they co-fit, so
	// Playwright's pre-click auto-scroll lands both inside the viewport
	// — and location.reload()'s scroll-restoration leaves them both
	// visible even though no fragment-scroll happened. With the heading
	// and button separated by more than the viewport height, the heading
	// is only inside the viewport when a real fragment-scroll places it
	// there.
	if err = page.SetViewportSize(360, 480); err != nil {
		t.Fatalf("shrink viewport: %v", err)
	}

	// Register and fill at least one day so the schedule submit doesn't
	// bounce back with a validation error.
	registerAndWaitSchedule(t, page, serverURL)
	selectAndSubmitSchedule(t, page, serverURL, []string{"Monday"})

	// Go to /preferences and submit the recovery form. The handler sets a
	// success flash and redirects to /preferences#deload-title.
	if _, err = page.Goto(serverURL + "/preferences"); err != nil {
		t.Fatalf("goto /preferences: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/preferences", serverURL)); err != nil {
		t.Fatalf("wait for /preferences: %v", err)
	}
	saveRecoveryBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Save recovery settings"})
	if err = saveRecoveryBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Save recovery settings button: %v", err)
	}
	if err = saveRecoveryBtn.Click(); err != nil {
		t.Fatalf("click Save recovery settings: %v", err)
	}

	// The success banner must appear on /preferences. WaitFor() with a
	// generous deadline so a slow CI run isn't the failure mode; the
	// regression fails fast because no document load ever happens.
	banner := page.GetByText("Recovery settings saved.")
	if err = banner.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("expected success banner on /preferences after recovery save: %v", err)
	}

	// URL should end on /preferences (with or without the fragment); the
	// important property is that we're still on the preferences page.
	if got := page.URL(); !strings.HasPrefix(got, serverURL+"/preferences") {
		t.Errorf("URL after recovery save = %q, want prefix %q", got, serverURL+"/preferences")
	}
	// URL cleanup: the inline script in base.gohtml must have stripped
	// ?bf_inv=... from the URL bar by the time the banner is visible.
	// The canonical form is /preferences#deload-title.
	if got := page.URL(); strings.Contains(got, "bf_inv") {
		t.Errorf("URL still carries bf_inv after recovery save: %q", got)
	}
	if got := page.URL(); !strings.HasSuffix(got, "#deload-title") {
		t.Errorf("URL after recovery save = %q, want suffix %q", got, "#deload-title")
	}

	// The Save button must not still be in a busy state — a stuck spinner
	// is the visible symptom of the same hang. Web-first ToHaveAttribute
	// auto-retries until the timeout elapses, surviving any brief moment
	// the busy flag is still in the act of clearing.
	assertions := playwright.NewPlaywrightAssertions()
	if err = assertions.Locator(saveRecoveryBtn).Not().ToHaveAttribute(
		"aria-busy", "true",
	); err != nil {
		t.Errorf("Save recovery settings button still has aria-busy=true — spinner stuck: %v", err)
	}

	// Scroll-to-panel regression check. The whole point of including
	// #deload-title in the redirect target is to land the user with the
	// deload heading visible. location.reload() — the prior workaround —
	// triggers scroll-restoration on most browsers, ignoring the URL
	// fragment. With the shrunk viewport above, the heading is only
	// inside the viewport when a real cross-document fragment-scroll
	// placed it there.
	if err = assertions.Locator(
		page.Locator("#deload-title"),
	).ToBeInViewport(); err != nil {
		t.Errorf("deload heading not in viewport after recovery save: %v", err)
	}

	// Second submit from the same URL. After the first reload the URL is
	// /preferences#deload-title, so currentUrl.hash matches targetUrl.hash
	// exactly. A naive "hash differs" check skips the reload branch and
	// hands off to navigation.navigate, which on identical URL+fragment is
	// also a same-document no-op — the page never re-renders and the
	// freshly-set flash leaks just like before. The fix must force a
	// reload for any same-URL submit involving a fragment, not only when
	// the hashes differ.
	//
	// ExpectResponse for GET /preferences gates the click on a real
	// document fetch; if the reload never happens the matcher times out
	// and we get an actionable failure.
	// After this fix the second-submit GET is /preferences?bf_inv=... ;
	// before it was /preferences . Accept either.
	prefsURLRe := regexp.MustCompile("^" + regexp.QuoteMeta(serverURL+"/preferences") + `(\?|$)`)
	if _, err = page.ExpectResponse(prefsURLRe, func() error {
		return saveRecoveryBtn.Click()
	}, playwright.PageExpectResponseOptions{
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("second recovery save did not trigger a GET /preferences reload: %v", err)
	}
	// Banner must reappear after the reload — proves the GET actually
	// rendered the re-popped flash, not just a no-op response.
	if err = banner.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Fatalf("expected success banner after second recovery save: %v", err)
	}
	if got := page.URL(); strings.Contains(got, "bf_inv") {
		t.Errorf("URL still carries bf_inv after second recovery save: %q", got)
	}
	if got := page.URL(); !strings.HasSuffix(got, "#deload-title") {
		t.Errorf("URL after second recovery save = %q, want suffix %q", got, "#deload-title")
	}

	// Navigate away to /. The flash was consumed on the previous GET, so
	// the home page must not render the message.
	if _, err = page.Goto(serverURL + "/"); err != nil {
		t.Fatalf("goto / after recovery save: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("wait for / after recovery save: %v", err)
	}
	leakedCount, err := page.GetByText("Recovery settings saved.").Count()
	if err != nil {
		t.Fatalf("count leaked banner on home: %v", err)
	}
	if leakedCount != 0 {
		t.Errorf("success flash leaked to home page (count=%d) — flash must be consumed on /preferences", leakedCount)
	}
}

// Test_playwright_bfcache_staleness drives a flow where the home page (/)
// is rendered before any POST (bakedToken == ""), then a POST rotates the
// inv_bfcache cookie, then the user navigates back. The browser restores /
// from bfcache (its rendered meta token is still ""), our pagereveal
// handler detects the mismatch against the now-rotated cookie, and triggers
// navigation.reload(). After the reload, meta should equal the cookie.
//
// This is regression protection for the listener wiring: if the
// pagereveal staleness check regresses (e.g. someone removes the listener
// or reverts it back to pageshow.persisted, which would miss
// prefetch-promoted loads), the assertion below fails. It does not
// exercise the Speculation Rules prefetch path — that's the TLA+
// spec's job (tlaplus/StackNav_Prefetch.cfg /
// tlaplus/StackNav_PrefetchMitigated.cfg).
func Test_playwright_bfcache_staleness(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping slow playwright bfcache staleness test")
	}

	page, serverURL := setupPlaywrightPage(t)
	var err error

	// Register and schedule today + tomorrow so today has a workout
	// (matches the smoketest setup pattern around the midnight boundary).
	registerAndWaitSchedule(t, page, serverURL)
	selectAndSubmitSchedule(t, page, serverURL, todayAndTomorrowWeekdays())

	// At this point / is loaded with meta == current inv_bfcache cookie
	// (the schedule POST already rotated it). Drive a second POST so the
	// /-as-it-currently-sits will be bfcached BEFORE the rotation we care
	// about. Click "Start Workout": POSTs /workouts/{today}/start and
	// pushes /workouts/{today} on top of /.
	if err = page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Start Workout"}).Click(); err != nil {
		t.Fatalf("click Start Workout: %v", err)
	}
	workoutURLPattern := regexp.MustCompile(fmt.Sprintf(
		`^%s/workouts/\d{4}-\d{2}-\d{2}$`, regexp.QuoteMeta(serverURL)))
	if err = page.WaitForURL(workoutURLPattern); err != nil {
		t.Fatalf("expect /workouts/{today} after Start Workout: %v", err)
	}

	// Snapshot the cookie that the Start Workout POST set. Used as the
	// expected value to assert against after the back-nav-triggered reload.
	cookieAfterPost, err := page.Evaluate(
		`document.cookie.match(/(?:^|;\s*)inv_bfcache=([^;]+)/)?.[1] ?? ''`)
	if err != nil {
		t.Fatalf("read cookie after Start Workout: %v", err)
	}
	wantToken, _ := cookieAfterPost.(string)
	if wantToken == "" {
		t.Fatalf("inv_bfcache cookie not set after Start Workout POST")
	}

	// Navigate back to /. The browser restores it from bfcache with the
	// pre-POST meta token (which was rotated when the schedule POST set the
	// cookie, but the / snapshot in bfcache was captured BEFORE the Start
	// Workout POST rotated it again). Mismatch -> pagereveal handler
	// triggers navigation.reload(). After reload, meta == cookie.
	if _, err = page.GoBack(); err != nil {
		t.Fatalf("GoBack to /: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect / after GoBack: %v", err)
	}

	// Poll until the page's meta token equals the post-Start-Workout cookie.
	// The bfcache snapshot has the OLD token; after the staleness-triggered
	// reload, the freshly-fetched / will have the new token. If no listener
	// fires the reload, this times out.
	_, err = page.WaitForFunction(`
		(want) => {
			const meta = document.querySelector('meta[name=invalidation-token]')?.content ?? '';
			return meta === want;
		}
	`, wantToken, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(5000),
	})
	if err != nil {
		// Failure path: dump current state to narrow the post-mortem.
		state, _ := page.Evaluate(`() => ({
			meta: document.querySelector('meta[name=invalidation-token]')?.content ?? null,
			cookie: document.cookie.match(/(?:^|;\s*)inv_bfcache=([^;]+)/)?.[1] ?? null,
			url: location.href,
		})`)
		t.Fatalf("post-bfcache reload did not converge meta to cookie within 5s: %v; state=%+v; want=%q",
			err, state, wantToken)
	}
}

// dumpNavDiagnostics logs the page URL, the Navigation API entry stack, the
// current load state, and the page heading. Called from t.Fatalf paths whose
// failure mode is "URL is not what we expected" — the dump narrows the
// post-mortem from "what happened in the browser?" to a specific divergence.
func dumpNavDiagnostics(t *testing.T, page playwright.Page, where, wantURL string) {
	t.Helper()
	t.Logf("[diag %s] want URL = %s", where, wantURL)
	t.Logf("[diag %s] page.URL() = %s", where, page.URL())
	state, err := page.Evaluate(`() => ({
		readyState: document.readyState,
		hasNav: 'navigation' in window,
		currentIndex: window.navigation?.currentEntry?.index ?? null,
		entries: (window.navigation?.entries() ?? []).map(e => ({index: e.index, url: e.url})),
		heading: document.querySelector('h1')?.textContent ?? null,
		invalidationMeta: document.querySelector('meta[name=invalidation-token]')?.content ?? null,
	})`)
	if err != nil {
		t.Logf("[diag %s] evaluate failed: %v", where, err)
		return
	}
	t.Logf("[diag %s] state = %+v", where, state)
}

// addWeightedExerciseToWorkout navigates from the workout page to the
// add-exercise picker, adds the first weighted or assisted exercise (filtered
// via the picker card's data-exercise-type attribute), and returns the page
// to workoutURL. The type filter is what makes the suite robust to the
// planner's exercise-pool composition: without it, callers downstream may
// land on a bodyweight or time_based exercise whose form lacks the
// "Actual reps" field and named signal buttons.
//
// Note: post add-exercise UX, the POST replaces /add-exercise with the new
// exercise's DETAIL page rather than the workout overview. This helper
// follows up with a Goto(workoutURL) so callers see the overview state they
// expect.
func addWeightedExerciseToWorkout(t *testing.T, page playwright.Page, workoutURL string) {
	t.Helper()
	var err error
	addLink := page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Add exercise"})
	if err = addLink.Click(); err != nil {
		t.Fatalf("click Add Exercise link: %v", err)
	}
	if err = page.WaitForURL(func(u string) bool { return strings.Contains(u, "/add-exercise") }); err != nil {
		t.Fatalf("wait for add-exercise page: %v", err)
	}
	weightedCard := page.Locator(
		`.exercise-result[data-exercise-type="weighted"], ` +
			`.exercise-result[data-exercise-type="assisted"]`).First()
	addBtn := weightedCard.GetByRole("button",
		playwright.LocatorGetByRoleOptions{Name: "Add this exercise"})
	if err = addBtn.WaitFor(); err != nil {
		t.Fatalf("wait for weighted/assisted Add-this-exercise button: %v", err)
	}
	if err = addBtn.Click(); err != nil {
		t.Fatalf("click Add this exercise: %v", err)
	}
	// The POST replaces /add-exercise with the new exercise's DETAIL page.
	if err = page.WaitForURL(func(u string) bool {
		return strings.Contains(u, "/exercises/")
	}); err != nil {
		t.Fatalf("wait for new exercise DETAIL after add: %v", err)
	}
	// Return the page to the workout overview for the caller's downstream
	// assertions (the helper's contract is "exercise added, page back at overview").
	if _, err = page.Goto(workoutURL); err != nil {
		t.Fatalf("goto workout overview after add: %v", err)
	}
}

// registerAndWaitSchedule clicks "Begin training" on the unauthenticated home
// page and waits for the /schedule landing. Use as the first step of any
// authenticated-flow test.
func registerAndWaitSchedule(t *testing.T, page playwright.Page, serverURL string) {
	t.Helper()
	beginBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Begin training"})
	if err := beginBtn.Click(); err != nil {
		t.Fatalf("click Begin training: %v", err)
	}
	if err := page.WaitForURL(serverURL + "/schedule"); err != nil {
		t.Fatalf("wait for /schedule after registration: %v", err)
	}
}

// selectAndSubmitSchedule selects each weekday for 1 hour and clicks "Start
// Tracking", waiting for the post-submit landing at /. The caller is
// responsible for being on /schedule beforehand and for choosing a valid
// non-empty day set (an empty list lets the server's validation reject the
// submit — useful for testing that path).
func selectAndSubmitSchedule(t *testing.T, page playwright.Page, serverURL string, days []string) {
	t.Helper()
	for _, day := range days {
		if _, err := page.GetByLabel(day).SelectOption(playwright.SelectOptionValues{
			Labels: &[]string{"1 hour"},
		}); err != nil {
			t.Fatalf("select %s duration: %v", day, err)
		}
	}
	startBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Start Tracking"})
	if err := startBtn.Click(); err != nil {
		t.Fatalf("click Start Tracking: %v", err)
	}
	if err := page.WaitForURL(serverURL + "/"); err != nil {
		t.Fatalf("wait for / after schedule submit: %v", err)
	}
}

// todayAndTomorrowWeekdays returns the current and next weekday names as
// shown by the schedule form's day labels. Scheduling both sides of the
// midnight boundary keeps the test honest even if the local clock crosses
// midnight between selection and the server's notion of "today".
func todayAndTomorrowWeekdays() []string {
	now := time.Now()
	return []string{now.Weekday().String(), now.AddDate(0, 0, 1).Weekday().String()}
}

// allWeekdays returns every weekday name in the order the schedule form
// renders them. Useful for tests that need today to have a workout no
// matter when they run.
func allWeekdays() []string {
	return []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
}

// capturePlaywrightFailureScreenshot writes a PNG of the page when the test
// failed, to a stable os.TempDir path, and logs the location. Called from a
// t.Cleanup wired in setupPlaywrightPage. Silent (best-effort) when the test
// passed or the page is already torn down — those aren't actionable.
func capturePlaywrightFailureScreenshot(t *testing.T, page playwright.Page) {
	t.Helper()
	if !t.Failed() {
		return
	}
	safeName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	// os.TempDir (not t.TempDir): the screenshot must outlive the test run
	// so a human can inspect it. t.TempDir is wiped at test end.
	path := filepath.Join(os.TempDir(), //nolint:usetesting // see comment above
		fmt.Sprintf("playwright-failure-%s-%s.png", safeName, time.Now().Format("20060102-150405")))
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path:     &path,
		FullPage: new(true),
	}); err != nil {
		t.Logf("failure screenshot capture failed: %v", err)
		return
	}
	t.Logf("failure screenshot: %s", path)
}

// axeAsset is the vendored axe-core build used for in-browser accessibility
// scanning. It is a dev-only test asset — never embedded in the binary, never
// shipped to clients. Fetch it once with `make fetch-axe`; Test_playwright_axe_aa
// skips cleanly while it is absent.
const axeAsset = "testdata/axe.min.js"

// runAxeAA injects axe-core into the current page and fails the test on any
// WCAG 2.x A/AA violation. axe evaluates the *rendered* DOM, so it measures
// real computed colour contrast — this is what retires the hand-maintained
// token-pairing matrix for every page it runs on (ADR 0008).
//
// Injection goes through page.Evaluate (CDP main-world eval), which is exempt
// from the page CSP and the require-trusted-types-for enforcement that would
// otherwise block a normal <script> injection.
func runAxeAA(t *testing.T, page playwright.Page, label string) {
	t.Helper()

	src, err := os.ReadFile(axeAsset)
	if err != nil {
		t.Fatalf("[%s] read axe asset %s: %v", label, axeAsset, err)
	}
	if _, err = page.Evaluate(string(src)); err != nil {
		t.Fatalf("[%s] inject axe-core: %v", label, err)
	}

	raw, err := page.Evaluate(`async () => {
		const result = await axe.run(document, {
			runOnly: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'],
		});
		return result.violations.map(v => ({
			id: v.id,
			impact: v.impact,
			help: v.help,
			targets: v.nodes.map(n => n.target.join(' ')),
		}));
	}`)
	if err != nil {
		t.Fatalf("[%s] run axe: %v", label, err)
	}

	violations, _ := raw.([]any)
	if len(violations) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %d axe WCAG A/AA violation(s):", label, len(violations))
	for _, v := range violations {
		m, _ := v.(map[string]any)
		fmt.Fprintf(&b, "\n  - %v (%v): %v\n    at: %v", m["id"], m["impact"], m["help"], m["targets"])
	}
	t.Error(b.String())
}

// Test_playwright_axe_aa runs axe-core against the rendered DOM of the pages the
// suite already drives through — unauthenticated home, schedule, authenticated
// home, and a workout — asserting zero WCAG A/AA violations. It reuses the
// existing fixture helpers so it adds coverage without new navigation scaffolding.
//
// It skips when the dev-only axe asset is not vendored (see axeAsset); fetch it
// with `make fetch-axe`.
func Test_playwright_axe_aa(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping slow playwright accessibility scan")
	}
	if _, err := os.Stat(axeAsset); err != nil {
		t.Skipf("axe asset %s not vendored — run `make fetch-axe` to enable the accessibility scan", axeAsset)
	}

	page, serverURL := setupPlaywrightPage(t)

	runAxeAA(t, page, "home (unauthenticated)")

	registerAndWaitSchedule(t, page, serverURL)
	runAxeAA(t, page, "schedule")

	selectAndSubmitSchedule(t, page, serverURL, allWeekdays())
	runAxeAA(t, page, "home (authenticated)")

	workoutURL := serverURL + "/workouts/" + time.Now().Format("2006-01-02")
	if _, err := page.Goto(workoutURL); err != nil {
		t.Fatalf("goto today's workout: %v", err)
	}
	runAxeAA(t, page, "workout")
}

// minTouchTargetPx is the design system's tap-target floor. Petra sizes every
// interactive control to at least 48 CSS px in both dimensions — `.btn` via
// `min-height: 3rem`, and the visually-small `.btn--sm` via a centred 48×48
// `::before` hit-area expander (see ui/static/main.css). This is stricter than
// WCAG 2.5.8 (AA, 24px) and clears WCAG 2.5.5 (AAA, 44px). axe-core only checks
// the 24px floor, so this dedicated scan guards the 48px rule the CSS documents
// but nothing else asserts.
const minTouchTargetPx = 48

// assertTouchTargets measures the effective tap area of every visible
// interactive control on the current page and fails for any below
// minTouchTargetPx in either dimension. "Effective" area is the larger of the
// element's own border box and its ::before/::after pseudo boxes, so the
// .btn--sm expander is measured the way a finger actually lands on it. Inline
// links (the WCAG 2.5.5/2.5.8 running-text exception) and visually-hidden or
// non-interactive (pointer-events:none) helpers are excluded.
func assertTouchTargets(t *testing.T, page playwright.Page, label string) {
	t.Helper()

	raw, err := page.Evaluate(`(min) => {
		const TOL = 0.5; // tolerate sub-pixel rounding on a 3rem (48px) box
		const sel = [
			'a[href]', 'button', 'input:not([type="hidden"])', 'select',
			'textarea', 'summary', '[role="button"]', '[role="link"]',
			'[role="checkbox"]', '[role="switch"]', '[role="tab"]', '[role="menuitem"]',
		].join(', ');
		const px = (v) => { const n = parseFloat(v); return Number.isNaN(n) ? 0 : n; };
		const pseudo = (el, which) => {
			const cs = getComputedStyle(el, which);
			if (!cs || cs.content === 'none' || cs.content === 'normal') return { w: 0, h: 0 };
			return { w: Math.max(px(cs.width), px(cs.minWidth)), h: Math.max(px(cs.height), px(cs.minHeight)) };
		};
		const out = [];
		for (const el of document.querySelectorAll(sel)) {
			const cs = getComputedStyle(el);
			if (cs.display === 'none' || cs.visibility === 'hidden' || cs.visibility === 'collapse') continue;
			if (cs.pointerEvents === 'none') continue;
			// WCAG 2.5.5 / 2.5.8 inline exception: links flowing inside running text.
			if (el.tagName === 'A' && cs.display === 'inline') continue;
			const r = el.getBoundingClientRect();
			if (r.width < 1 || r.height < 1) continue; // visually-hidden / collapsed helpers
			const before = pseudo(el, '::before');
			const after = pseudo(el, '::after');
			const w = Math.max(r.width, before.w, after.w);
			const h = Math.max(r.height, before.h, after.h);
			if (w < min - TOL || h < min - TOL) {
				const cls = (el.className && el.className.toString) ? el.className.toString() : '';
				out.push({
					tag: el.tagName.toLowerCase(),
					type: el.getAttribute('type') || '',
					id: el.id || '',
					cls: cls,
					text: (el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 40),
					// Emit as strings: playwright-go decodes whole-number JS values
					// as int and fractional ones as float64, so a numeric field
					// can't be read with a single type assertion.
					w: w.toFixed(1),
					h: h.toFixed(1),
				});
			}
		}
		return out;
	}`, minTouchTargetPx)
	if err != nil {
		t.Fatalf("[%s] measure touch targets: %v", label, err)
	}

	targets, _ := raw.([]any)
	if len(targets) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %d interactive element(s) below the %dpx tap-target minimum:",
		label, len(targets), minTouchTargetPx)
	for _, tgt := range targets {
		m, _ := tgt.(map[string]any)
		tag, _ := m["tag"].(string)
		if typ, _ := m["type"].(string); typ != "" {
			tag = fmt.Sprintf("%s type=%s", tag, typ)
		}
		w, _ := m["w"].(string)
		h, _ := m["h"].(string)
		fmt.Fprintf(&b, "\n  - <%s> %s×%s px", tag, w, h)
		if id, _ := m["id"].(string); id != "" {
			fmt.Fprintf(&b, " #%s", id)
		}
		if cls, _ := m["cls"].(string); cls != "" {
			fmt.Fprintf(&b, " class=%q", cls)
		}
		if txt, _ := m["text"].(string); txt != "" {
			fmt.Fprintf(&b, " text=%q", txt)
		}
	}
	t.Error(b.String())
}

// Test_playwright_touch_targets asserts that every visible interactive control
// on the pages the suite drives meets Petra's 48px tap-target minimum. axe-core
// (Test_playwright_axe_aa) only enforces WCAG 2.5.8's 24px AA floor, so this
// guards the stricter design-system rule. It runs at the iPhone-13 viewport the
// fixtures default to and reuses the same navigation helpers, so it adds
// coverage without new scaffolding.
func Test_playwright_touch_targets(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping slow playwright touch-target scan")
	}

	page, serverURL := setupPlaywrightPage(t)

	assertTouchTargets(t, page, "home (unauthenticated)")

	registerAndWaitSchedule(t, page, serverURL)
	assertTouchTargets(t, page, "schedule")

	selectAndSubmitSchedule(t, page, serverURL, allWeekdays())
	assertTouchTargets(t, page, "home (authenticated)")

	workoutURL := serverURL + "/workouts/" + time.Now().Format("2006-01-02")
	if _, err := page.Goto(workoutURL); err != nil {
		t.Fatalf("goto today's workout: %v", err)
	}
	assertTouchTargets(t, page, "workout")
}
