package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_application_addWorkout(t *testing.T) {
	t.Parallel()

	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// Register a user
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Set workout preferences (enable today's weekday).
	formData := map[string]string{
		time.Now().Weekday().String(): "60",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences/schedule", formData); err != nil {
		t.Fatalf("Failed to submit preferences form: %v", err)
	}

	// Start a workout for today
	today := time.Now().Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("Failed to submit start workout form: %v", err)
	}

	// Verify we're on the workout page
	if doc.Find("a.exercise").Length() == 0 {
		t.Error("Expected to find exercises on the workout page")
	}

	// Count initial exercises
	initialExerciseCount := doc.Find("a.exercise").Length()

	// Navigate to add exercise page
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise"); err != nil {
		t.Fatalf("Failed to get add exercise page: %v", err)
	}

	// Verify we're on the add exercise page
	if doc.Find("h1:contains('Add Exercise')").Length() == 0 {
		t.Error("Expected to find 'Add Exercise' heading")
	}

	if _, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/add-exercise", nil); err != nil {
		t.Fatalf("Failed to submit add exercise form: %v", err)
	}

	// The POST redirects to the new exercise's detail page (not the
	// workout overview). Re-fetch the overview to verify the exercise was
	// persisted by checking the count.
	if doc, err = client.GetDoc(ctx, "/workouts/"+today); err != nil {
		t.Fatalf("Failed to re-fetch workout overview: %v", err)
	}

	newExerciseCount := doc.Find("a.exercise").Length()
	if newExerciseCount != initialExerciseCount+1 {
		t.Errorf("Expected exercise count to increase by 1, got %d (was %d)",
			newExerciseCount, initialExerciseCount)
	}
}

func Test_application_workoutNotFound(t *testing.T) {
	t.Parallel()

	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// Register a user
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Try to access a workout that doesn't exist (a future date)
	futureDate := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	resp, err := client.Get(ctx, "/workouts/"+futureDate)
	if err != nil {
		t.Fatalf("Failed to get non-existent workout page: %v", err)
	}
	defer resp.Body.Close()

	// Verify we get a 404 status code
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	// Parse the response body into a document
	doc, err = goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse response body: %v", err)
	}

	// Verify we're on the workout not found page
	if doc.Find("h1:contains('Not in This Week\\'s Plan')").Length() == 0 {
		t.Error("Expected to find 'Not in This Week's Plan' heading on the page")
	}

	// Verify there's a back to home link
	homeLink := doc.Find("a:contains('Back to Home')")
	if homeLink.Length() == 0 {
		t.Error("Expected to find 'Back to Home' link on the page")
	}

	// Verify there's no create workout button
	createButton := doc.Find("button:contains('Create Workout')")
	if createButton.Length() > 0 {
		t.Error("Expected no 'Create Workout' button on the page")
	}
}

// Test_application_workoutAddExercise_search_filters_by_name verifies that the
// add-exercise page filters available exercises by name substring
// (case-insensitive) when ?q= is set, echoes the query into the search input,
// and renders an empty state when nothing matches.
func Test_application_workoutAddExercise_search_filters_by_name(t *testing.T) {
	t.Parallel()

	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	formData := map[string]string{time.Now().Weekday().String(): "60"}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences/schedule", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if _, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("Start workout: %v", err)
	}

	// Collect the unfiltered set of available exercise names so the test is
	// data-driven against fixtures rather than coupled to specific names.
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise"); err != nil {
		t.Fatalf("Get add-exercise page: %v", err)
	}
	var allNames []string
	doc.Find(".exercise-result .exercise-result__name").Each(func(_ int, s *goquery.Selection) {
		allNames = append(allNames, strings.TrimSpace(s.Text()))
	})
	if len(allNames) < 2 {
		t.Fatalf("Need at least 2 available exercises to exercise the filter, got %d", len(allNames))
	}

	// Pick the first word of the first exercise name as the search substring.
	// Mixed case to verify case-insensitivity.
	firstWord := strings.Fields(allNames[0])[0]
	if len(firstWord) < 3 {
		t.Fatalf("First word too short to be a useful filter: %q", firstWord)
	}
	needle := strings.ToUpper(firstWord)

	// Filtered query should return a (non-empty) subset, all containing the
	// substring case-insensitively.
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise?q="+url.QueryEscape(needle)); err != nil {
		t.Fatalf("Get add-exercise page with query: %v", err)
	}
	var filteredNames []string
	doc.Find(".exercise-result .exercise-result__name").Each(func(_ int, s *goquery.Selection) {
		filteredNames = append(filteredNames, strings.TrimSpace(s.Text()))
	})
	if len(filteredNames) == 0 {
		t.Fatalf("Expected at least one match for %q, got none", needle)
	}
	if len(filteredNames) > len(allNames) {
		t.Errorf("Filtered list (%d) larger than unfiltered (%d)", len(filteredNames), len(allNames))
	}
	needleLower := strings.ToLower(needle)
	for _, name := range filteredNames {
		if !strings.Contains(strings.ToLower(name), needleLower) {
			t.Errorf("Result %q does not contain %q", name, needle)
		}
	}

	// Search input must echo the query so reloading preserves state.
	gotQuery, _ := doc.Find("input[name='q']").Attr("value")
	if gotQuery != needle {
		t.Errorf("Search input value = %q, want %q", gotQuery, needle)
	}

	// A query that matches nothing must render the empty-state copy and no
	// add forms.
	noMatch := "zzznotreal"
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise?q="+url.QueryEscape(noMatch)); err != nil {
		t.Fatalf("Get add-exercise page with no-match query: %v", err)
	}
	if doc.Find(".exercise-result").Length() != 0 {
		t.Error("Expected zero exercise options when query matches nothing")
	}
	emptyState := doc.Find(".no-results")
	if emptyState.Length() == 0 {
		t.Fatal("Expected .no-results empty state when query matches nothing")
	}
	if !strings.Contains(emptyState.Text(), noMatch) {
		t.Errorf("Empty state %q should echo the query %q", strings.TrimSpace(emptyState.Text()), noMatch)
	}

	// Re-fetch unfiltered page so the card structure assertions have exercises to inspect.
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise"); err != nil {
		t.Fatalf("Get add-exercise page (card structure check): %v", err)
	}

	// Lock in the new card structure.
	card := doc.Find(".exercise-result").First()
	if card.Length() == 0 {
		t.Fatal("expected at least one .exercise-result on the add page")
	}
	if card.Find(".badge").Length() == 0 {
		t.Error("expected category badge inside the exercise card")
	}
	if card.Find(".muscle-chip.muscle-chip--primary").Length() == 0 {
		t.Error("expected at least one .muscle-chip--primary in the card")
	}
	if card.Find(".actions .btn.btn--quiet[type='submit']").Length() == 0 {
		t.Error("expected the primary Add action as .btn.btn--quiet submit inside .actions")
	}
	if card.Find(".actions .btn.btn--ghost.btn--sm").Length() == 0 {
		t.Error("expected the secondary Info action as .btn.btn--ghost.btn--sm inside .actions")
	}
	if card.Find("dialog.sheet-dialog").Length() == 0 {
		t.Error("expected the per-card info dialog as dialog.sheet-dialog")
	}
}

// Test_application_workoutAddExercisePOST_unplanned_day verifies that
// POSTing /workouts/{date}/add-exercise on a date with no planned session
// surfaces the flash + banner explaining the situation instead of a 500.
// The user lands back on the workout-not-found page with role="alert"
// banner content.
func Test_application_workoutAddExercisePOST_unplanned_day(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Set preferences so today is a planned day; the date 8 days out lands
	// on a different weekday (an unplanned day) for every possible "today".
	formData := map[string]string{time.Now().Weekday().String(): "60"}
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences/schedule", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	plannedDate := time.Now().Format("2006-01-02")
	unplannedDate := time.Now().AddDate(0, 0, 8).Format("2006-01-02")

	// Start the planned workout so the picker page renders, then grab any
	// exercise_id off the picker — data-driven against fixtures.
	if _, err = client.SubmitForm(ctx, doc, "/workouts/"+plannedDate+"/start", nil); err != nil {
		t.Fatalf("Start planned workout: %v", err)
	}
	pickerDoc, err := client.GetDoc(ctx, "/workouts/"+plannedDate+"/add-exercise")
	if err != nil {
		t.Fatalf("Get add-exercise picker: %v", err)
	}
	exerciseID, ok := pickerDoc.Find(".exercise-result [name='exercise_id']").
		First().Attr("value")
	if !ok || exerciseID == "" {
		t.Fatalf("Could not find an exercise_id in the picker page")
	}

	// POST directly to the unplanned date's add-exercise endpoint. Disable
	// auto-redirect following on this one-off request so we can assert the
	// raw 303 + Location from userError before manually following it below.
	body := url.Values{"exercise_id": {exerciseID}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		server.URL()+"/workouts/"+unplannedDate+"/add-exercise",
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("Build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpClient := *client.HTTPClient() // shallow copy preserves jar + transport.
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST to unplanned date: %v", err)
	}
	if cerr := resp.Body.Close(); cerr != nil {
		t.Fatalf("Close response body: %v", cerr)
	}
	// Non-stacknav POSTs get a 303 See Other (see helpers.go:redirect).
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("Expected 303 See Other from userError redirect, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/workouts/"+unplannedDate {
		t.Errorf("Expected Location: /workouts/%s, got %q", unplannedDate, loc)
	}

	// Follow the redirect and assert the banner is rendered on the
	// workout-not-found page.
	workoutResp, err := client.Get(ctx, "/workouts/"+unplannedDate)
	if err != nil {
		t.Fatalf("Get workout page after redirect: %v", err)
	}
	defer func() { _ = workoutResp.Body.Close() }()
	// workout-not-found renders with HTTP 404; the banner is still in the body.
	workoutDoc, err := goquery.NewDocumentFromReader(workoutResp.Body)
	if err != nil {
		t.Fatalf("Parse workout page: %v", err)
	}
	banner := workoutDoc.Find(".banner.banner--error[role='alert']").First()
	if banner.Length() == 0 {
		t.Fatalf("Expected .banner.banner--error[role=\"alert\"] on workout-not-found page; got none")
	}
	wantSubstr := "no planned workout"
	if !strings.Contains(strings.ToLower(banner.Text()), wantSubstr) {
		t.Errorf("Banner text = %q, want substring %q",
			strings.TrimSpace(banner.Text()), wantSubstr)
	}
}

func Test_application_startExtraWorkoutOnUnscheduledToday(t *testing.T) {
	t.Parallel()

	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Schedule a non-today weekday at 60 min so today is unscheduled.
	today := time.Now().Weekday()
	nonToday := time.Monday
	if today == time.Monday {
		nonToday = time.Tuesday
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences/schedule", map[string]string{
		nonToday.String(): "60",
	}); err != nil {
		t.Fatalf("Failed to submit preferences: %v", err)
	}

	// Submit "Start Extra Workout" for today.
	todayStr := time.Now().Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+todayStr+"/start", nil); err != nil {
		t.Fatalf("Failed to start extra workout: %v", err)
	}

	// The workout page must list exercises after lazy-create.
	if doc.Find("a.exercise").Length() == 0 {
		t.Error("Expected exercises on workout page after starting ad-hoc session")
	}
}

func TestWorkoutFeedbackPOST_BadDifficultyParamReturns404(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// POST to /workouts/2026-05-22/feedback/not-a-number and expect 404.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		server.URL()+"/workouts/2026-05-22/feedback/not-a-number",
		strings.NewReader(""))
	if err != nil {
		t.Fatalf("Build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("POST to bad difficulty param: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}
}

func Test_application_startNewlyScheduledMidWeekDay(t *testing.T) {
	t.Parallel()

	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Set schedule to today only.
	today := time.Now().Weekday()
	// Petra's week ends Sunday; on Sunday there is no later day this week to schedule.
	if today == time.Sunday {
		t.Skip("Cannot run on Sunday: no remaining day in the current week")
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences/schedule", map[string]string{
		today.String(): "60",
	}); err != nil {
		t.Fatalf("Failed to submit preferences: %v", err)
	}

	// Start today's workout so RegenerateWeeklyPlanIfUnstarted will later skip.
	todayStr := time.Now().Format("2006-01-02")
	if _, err = client.SubmitForm(ctx, doc, "/workouts/"+todayStr+"/start", nil); err != nil {
		t.Fatalf("Failed to start today's workout: %v", err)
	}

	// Add another weekday mid-week — pick the next day this week (Sun follows Sat).
	addedDay := today + 1
	if addedDay > time.Saturday {
		addedDay = time.Sunday
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences (2nd): %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences/schedule", map[string]string{
		today.String():    "60",
		addedDay.String(): "60",
	}); err != nil {
		t.Fatalf("Failed to submit updated preferences: %v", err)
	}

	// Compute the calendar date for addedDay in this week (UTC, matching service.mondayOf).
	now := time.Now().UTC()
	y, m, d := now.Date()
	mondayOffset := int(time.Monday - now.Weekday())
	if mondayOffset > 0 {
		mondayOffset = -6
	}
	monday := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, mondayOffset)
	// Petra's week starts Monday; Sunday is the last day (offset 6).
	offsetFromMonday := (int(addedDay) + 6) % 7
	addedDateStr := monday.AddDate(0, 0, offsetFromMonday).Format("2006-01-02")

	// Start the newly-scheduled day — this is the case that fails without lazy-create.
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+addedDateStr+"/start", nil); err != nil {
		t.Fatalf("Failed to start newly-scheduled day: %v", err)
	}
	if doc.Find("a.exercise").Length() == 0 {
		t.Error("Expected exercises on newly-scheduled day's workout page after lazy-create")
	}
}

// Test_application_workoutCompletePOST_unstartedSession_shimHeader_redirectsToCompletionForm
// covers the retroactive-finish flow on the JS shim path: a user POSTs
// /workouts/{today}/complete on a scheduled-but-unstarted session.
// CompleteSession auto-starts inside the same transaction (see
// service.CompleteSession), so the handler succeeds and the shim receives
// 200 + X-Location: /workouts/{today}/complete (the feedback form).
func Test_application_workoutCompletePOST_unstartedSession_shimHeader_redirectsToCompletionForm(
	t *testing.T,
) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Schedule today so a session exists. We deliberately do NOT call /start.
	formData := map[string]string{time.Now().Weekday().String(): "60"}
	prefsDoc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if _, err = client.SubmitForm(ctx, prefsDoc, "/preferences/schedule", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	target := server.URL() + "/workouts/" + today + "/complete"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "stacknav")
	req.Header.Set("Referer", server.URL()+"/workouts/"+today)

	httpClient := *client.HTTPClient() // shallow copy preserves jar.
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST /complete: %v", err)
	}
	if cerr := resp.Body.Close(); cerr != nil {
		t.Fatalf("Close response body: %v", cerr)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (shim-aware redirect)", resp.StatusCode)
	}
	wantLoc := "/workouts/" + today + "/complete"
	if got := resp.Header.Get("X-Location"); got != wantLoc {
		t.Errorf("X-Location = %q, want %q", got, wantLoc)
	}
}

// Test_application_workoutCompletePOST_unstartedSession_noShimHeader_redirectsToCompletionForm
// covers the same retroactive-finish flow for a plain (no JS shim) client:
// the server falls through to a 303 See Other pointing at the feedback form.
func Test_application_workoutCompletePOST_unstartedSession_noShimHeader_redirectsToCompletionForm(
	t *testing.T,
) {
	t.Parallel()

	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	formData := map[string]string{time.Now().Weekday().String(): "60"}
	prefsDoc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if _, err = client.SubmitForm(ctx, prefsDoc, "/preferences/schedule", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	target := server.URL() + "/workouts/" + today + "/complete"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(""))
	if err != nil {
		t.Fatalf("Build POST request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No X-Requested-With.

	httpClient := *client.HTTPClient()
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST /complete: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("status = %d, want 303 (plain redirect)", resp.StatusCode)
	}
	wantLoc := "/workouts/" + today + "/complete"
	if got := resp.Header.Get("Location"); got != wantLoc {
		t.Errorf("Location = %q, want %q", got, wantLoc)
	}
}
