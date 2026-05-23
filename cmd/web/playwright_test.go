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

	// Emulate prefers-reduced-motion: reduce. The app's CSS honors this
	// media feature and collapses view transitions to 0.001ms, removes
	// button transforms, and disables the loading-bar animation — all of
	// which Playwright waits on for actionability/stability checks.
	bCtx, err := browser.NewContext(playwright.BrowserNewContextOptions{
		ReducedMotion: playwright.ReducedMotionReduce,
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

	registerBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Begin training"})
	signInBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Sign in"})
	logOutBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Log out"})

	// Step 1: Verify unauthenticated state.
	if err = registerBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Begin training button: %v", err)
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
	completedSets := page.Locator(".exercise-set.completed")
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
// mobile app for the six core flows defined in
// docs/superpowers/specs/2026-05-03-stack-navigator-push-default-design.md
// (which supersedes 2026-04-25-stack-navigator-redesign-design.md).
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
	if testing.Short() {
		t.Skip("skipping slow playwright stacknav test")
	}

	page, serverURL := setupPlaywrightPage(t)
	var err error

	// Setup: register and configure all weekdays so today always has a workout.
	if err = page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Begin training"}).Click(); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("expect /schedule after registration: %v", err)
	}

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
	for _, day := range []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"} {
		if _, err = page.GetByLabel(day).SelectOption(playwright.SelectOptionValues{
			Labels: &[]string{"1 hour"},
		}); err != nil {
			t.Fatalf("select %s duration: %v", day, err)
		}
	}
	// First submit: prefs saved, server responds with pop-or-push → /.
	// No / in history yet, so popOrPushTo pushes / on top of /schedule.
	// History after: [..., /schedule, /].
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

	// === Flow 3: pop-or-push, traverse branch (schedule submit) ===
	// Re-fill the form, then submit. Now / IS in history (we navigated back to
	// /schedule via Goto, which pushed another /schedule on top), so popOrPushTo
	// traverses to / instead of pushing a duplicate.
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
	wantDetailURL := fmt.Sprintf("%s%s", serverURL, exerciseHref)
	if err = page.WaitForURL(wantDetailURL); err != nil {
		dumpNavDiagnostics(t, page, "workout-to-detail link click", wantDetailURL)
		t.Fatalf("expect %s: %v", exerciseHref, err)
	}
	detailURL := page.URL()

	// === Flow 1: same-URL replace (set update) ===
	// All test exercises are weighted. The page shows a warmup banner first —
	// complete it before the set form appears. The set/warmup POSTs redirect
	// back to the same DETAIL URL, and the client auto-detects same-URL and
	// replaces in place — so history does not grow on either submit.
	warmupBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Mark done"})
	if err = warmupBtn.WaitFor(); err != nil {
		t.Fatalf("wait for warmup button: %v", err)
	}
	if err = warmupBtn.Click(); err != nil {
		t.Fatalf("click warmup button: %v", err)
	}
	// Wait for the reload triggered by the same-URL auto-replace to fully settle.
	if err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateLoad}); err != nil {
		t.Fatalf("expect load after warmup complete: %v", err)
	}

	// Submit the first set via the "too heavy" signal submit button. The reps
	// input is pre-filled with the target; we override it before clicking the
	// named submit button, which sends signal=too_heavy along with the form.
	if err = page.GetByLabel("Actual reps").First().Fill("8"); err != nil {
		t.Fatalf("fill actual reps: %v", err)
	}
	if err = page.Locator("button.too-heavy-btn").First().Click(); err != nil {
		t.Fatalf("click too-heavy signal: %v", err)
	}
	// After the set update, navigation.navigate(target, {history: 'replace'})
	// fires (target equals current URL → client auto-replaces). Wait for the
	// resulting reload-triggered navigation to fully commit by checking that
	// the first set now shows as completed. This is more reliable than
	// WaitForURL (URL unchanged) or WaitForLoadState (already-load resolves
	// immediately).
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

	// === Flow 2: cross-URL submit, target present (swap exercise) ===
	// Swap redirects to the same workoutExerciseID slot, so the URL matches
	// the original DETAIL the user came from. popOrPushTo finds it in the
	// backward stack and traverses to it.
	swapLink := page.Locator("a[href$='/swap']").First()
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
	if err = page.Locator("a[href$='/swap']").First().Click(); err != nil {
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
	addExerciseLink := page.Locator("a.add-exercise-link")
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

// Test_playwright_bfcache_staleness drives a flow where the home page (/)
// is rendered before any POST (bakedToken == ""), then a POST rotates the
// inv_bfcache cookie, then the user navigates back. The browser restores /
// from bfcache (its rendered meta token is still ""), our pagereveal
// handler detects the mismatch against the now-rotated cookie, and triggers
// navigation.reload(). After the reload, meta should equal the cookie.
//
// This is regression protection for the listener wiring: if both
// pageshow.persisted and pagereveal staleness paths regress simultaneously
// (e.g. someone removes the listener), the assertion below fails. It does
// not exercise the Speculation Rules prefetch path — that's the TLA+
// spec's job (tlaplus/StackNav_Prefetch.cfg /
// tlaplus/StackNav_PrefetchMitigated.cfg).
func Test_playwright_bfcache_staleness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow playwright bfcache staleness test")
	}

	page, serverURL := setupPlaywrightPage(t)
	var err error

	// Register and schedule today + tomorrow so today has a workout
	// (matches the smoketest setup pattern around the midnight boundary).
	if err = page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Begin training"}).Click(); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("expect /schedule after registration: %v", err)
	}
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
	if err = page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Start Tracking"}).Click(); err != nil {
		t.Fatalf("submit schedule: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect / after schedule submit: %v", err)
	}

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

// addExerciseToWorkout navigates from the workout page to the add-exercise page,
// adds the first available exercise, and returns the page to workoutURL.
// Used when the weekly planner exhausts its exercise pool and creates an empty workout.
//
// Note: post add-exercise UX, the POST replaces /add-exercise with the new
// exercise's DETAIL page rather than the workout overview. This helper
// follows up with a Goto(workoutURL) so callers see the overview state they
// expect.
func addExerciseToWorkout(t *testing.T, page playwright.Page, workoutURL string) {
	t.Helper()
	var err error
	if err = page.Locator("a.add-exercise-link").Click(); err != nil {
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
