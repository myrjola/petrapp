package main

import (
	"database/sql"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"github.com/myrjola/petrapp/internal/workout"
)

func Test_application_exerciseSet(t *testing.T) {
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

	// First register and set up a workout
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Schedule today and the next weekday so a midnight crossing during the test
	// can't desync the saved weekday from the URL date computed below.
	testStart := time.Now()
	formData := map[string]string{
		testStart.Weekday().String():                  "60",
		testStart.AddDate(0, 0, 1).Weekday().String(): "60",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Failed to submit form: %v", err)
	}

	// Start a workout for today
	today := testStart.Format("2006-01-02")
	// Find and submit the start workout form
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("Failed to submit start workout form: %v", err)
	}

	// Test viewing an exercise set
	// Extract the first exercise ID from the workout page
	var exerciseID string
	doc.Find("a.exercise").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			href, exists := s.Attr("href")
			if exists {
				exerciseID = href[len("/workouts/"+today+"/exercises/"):]
			}
		}
	})

	if exerciseID == "" {
		t.Fatalf("No exercise found on workout page")
	}

	// View the exercise set page
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID); err != nil {
		t.Fatalf("Failed to get exercise set page: %v", err)
	}

	// Verify page content
	if doc.Find("h1").Length() == 0 {
		t.Error("Expected exercise name heading")
	}

	// Check for set information
	if doc.Find(".exercise-set").Length() == 0 {
		t.Error("Expected to find sets on the page")
	}

	// Check for weight and reps information
	if doc.Find(".weight").Length() == 0 || doc.Find(".reps").Length() == 0 {
		t.Error("Expected to find weight and reps information")
	}

	// Check for the form to complete a set
	if doc.Find("form").Length() == 0 {
		t.Error("Expected to find a form to complete a set")
	}

	// Test completing a set

	// First view the workout to find an exercise
	if doc, err = client.GetDoc(ctx, "/workouts/"+today); err != nil {
		t.Fatalf("Failed to get workout page: %v", err)
	}

	// Extract the first exercise ID
	doc.Find("a.exercise").Each(func(i int, s *goquery.Selection) {
		if i == 0 { // Just take the first exercise
			href, exists := s.Attr("href")
			if exists {
				exerciseID = href[len("/workouts/"+today+"/exercises/"):]
			}
		}
	})

	if exerciseID == "" {
		t.Fatalf("No exercise found on workout page")
	}

	// View the exercise set page
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID); err != nil {
		t.Fatalf("Failed to get exercise set page: %v", err)
	}

	// Complete the warmup first
	warmupForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button:contains('Mark Warmup Complete')").Length() > 0
	}).First()

	if warmupForm.Length() > 0 {
		warmupAction, exists := warmupForm.Attr("action")
		if !exists {
			t.Fatalf("Warmup form has no action attribute")
		}
		if doc, err = client.SubmitForm(ctx, doc, warmupAction, nil); err != nil {
			t.Fatalf("Failed to submit warmup completion form: %v", err)
		}
	}

	// Find the form for completing a weighted set (has signal submit buttons).
	setForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[name='signal']").Length() > 0
	}).First()
	if setForm.Length() == 0 {
		t.Fatalf("Expected to find set form with signal buttons for active set")
	}

	setAction, exists := setForm.Attr("action")
	if !exists {
		t.Fatalf("Signal form has no action attribute")
	}

	if doc, err = client.SubmitForm(ctx, doc, setAction, map[string]string{
		"weight": "20.5",
		"signal": "on_target",
		"reps":   "5",
	}); err != nil {
		t.Fatalf("Failed to submit signal form: %v", err)
	}

	// Verify we're back on the exercise set page
	if doc.Find("h1").Length() == 0 {
		t.Error("Expected to find heading on exercise set page")
	}

	// Check that the set is now marked as completed
	if doc.Find(".exercise-set.completed").Length() == 0 {
		t.Error("Expected to find a completed set")
	}

	// Test editing a completed set
	// First view the workout to find an exercise
	if doc, err = client.GetDoc(ctx, "/workouts/"+today); err != nil {
		t.Fatalf("Failed to get workout page: %v", err)
	}

	// Extract the first exercise ID
	doc.Find("a.exercise").Each(func(i int, s *goquery.Selection) {
		if i == 0 { // Just take the first exercise
			var href string
			href, exists = s.Attr("href")
			if exists {
				exerciseID = href[len("/workouts/"+today+"/exercises/"):]
			}
		}
	})

	if exerciseID == "" {
		t.Fatalf("No exercise found on workout page")
	}

	// View the exercise set page
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID); err != nil {
		t.Fatalf("Failed to get exercise set page: %v", err)
	}

	// Find the "Edit" link in the first completed set
	editLink := doc.Find(".exercise-set.completed .edit-button").First()
	if editLink.Length() == 0 {
		t.Fatalf("No edit button found for completed set")
	}

	href, exists := editLink.Attr("href")
	if !exists {
		t.Fatalf("Edit link has no href")
	}

	// Visit the edit page
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID+href); err != nil {
		t.Fatalf("Failed to load edit page: %v", err)
	}

	// Find the form for the edit page (has signal submit buttons).
	editSignalForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[name='signal']").Length() > 0
	}).First()
	if editSignalForm.Length() == 0 {
		t.Fatalf("Edit signal form not found")
	}

	editAction, editActionExists := editSignalForm.Attr("action")
	if !editActionExists {
		t.Fatalf("Edit signal form has no action attribute")
	}

	// Get the current weight value from the edit form
	var weight string
	weight, exists = editSignalForm.Find("input[name='weight']").Attr("value")
	if !exists {
		t.Fatalf("Edit form has no weight input")
	}

	// Convert weight to float and increase it
	weightFloat, _ := strconv.ParseFloat(weight, 64)
	newWeight := weightFloat + 2.5 // Increase by 2.5 kg

	// Update the completed set with new weight and signal
	if doc, err = client.SubmitForm(ctx, doc, editAction, map[string]string{
		"weight": strconv.FormatFloat(newWeight, 'f', 1, 64),
		"signal": "on_target",
		"reps":   "12",
	}); err != nil {
		t.Fatalf("Failed to submit set update form: %v", err)
	}

	// Verify we're back on the exercise set page
	if doc.Find("h1").Length() == 0 {
		t.Error("Expected to find heading on exercise set page")
	}

	// Verify the updated values are shown
	// Extract the first completed set's weight
	setWeight := doc.Find(".exercise-set.completed .weight").First().Text()
	if setWeight == "" {
		t.Error("Expected to find weight in completed set")
	}

	// Extract the reps value
	setReps := doc.Find(".exercise-set.completed .reps").First().Text()
	if setReps == "" {
		t.Error("Expected to find reps in completed set")
	}

	// Check if the reps have been updated (should contain "12")
	if setReps != "12 reps" {
		t.Errorf("Expected reps to be updated to 12, got %s", setReps)
	}
}

// Test_application_exerciseSet_swap_preserves_url_and_drops_completed_sets verifies
// that swapping an exercise keeps the workout slot's stable URL working (regression
// for navigating-back-after-swap hitting 404), and that completed sets recorded
// against the previous exercise do not carry over.
func Test_application_exerciseSet_swap_preserves_url_and_drops_completed_sets(t *testing.T) {
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

	testStart := time.Now()
	formData := map[string]string{
		testStart.Weekday().String():                  "60",
		testStart.AddDate(0, 0, 1).Weekday().String(): "60",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := testStart.Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("Start workout: %v", err)
	}

	// Pick the first exercise's slot URL.
	var slotURL string
	doc.Find("a.exercise").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			if href, exists := s.Attr("href"); exists {
				slotURL = href
			}
		}
	})
	if slotURL == "" {
		t.Fatal("No exercise found on workout page")
	}

	// Visit slot, complete warmup, complete a set so we can assert it gets dropped.
	if doc, err = client.GetDoc(ctx, slotURL); err != nil {
		t.Fatalf("Get slot page: %v", err)
	}
	warmupForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button:contains('Mark Warmup Complete')").Length() > 0
	}).First()
	if warmupForm.Length() > 0 {
		action, _ := warmupForm.Attr("action")
		if doc, err = client.SubmitForm(ctx, doc, action, nil); err != nil {
			t.Fatalf("Submit warmup: %v", err)
		}
	}
	setForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[name='signal']").Length() > 0
	}).First()
	if setForm.Length() == 0 {
		t.Fatal("No signal form on slot page")
	}
	setAction, _ := setForm.Attr("action")
	if doc, err = client.SubmitForm(ctx, doc, setAction, map[string]string{
		"weight": "42.5",
		"signal": "on_target",
		"reps":   "5",
	}); err != nil {
		t.Fatalf("Submit set: %v", err)
	}
	if doc.Find(".exercise-set.completed").Length() == 0 {
		t.Fatal("Expected a completed set before swap")
	}

	// Swap the exercise in this slot to one of the offered alternatives.
	if doc, err = client.GetDoc(ctx, slotURL+"/swap"); err != nil {
		t.Fatalf("Get swap page: %v", err)
	}
	swapForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		action, exists := s.Attr("action")
		return exists && strings.HasSuffix(action, "/swap")
	}).First()
	if swapForm.Length() == 0 {
		t.Fatal("No swap form found on swap page")
	}
	swapAction, _ := swapForm.Attr("action")
	newExerciseID, exists := swapForm.Find("input[name='new_exercise_id']").Attr("value")
	if !exists || newExerciseID == "" {
		t.Fatal("No new_exercise_id offered on swap page")
	}
	if _, err = client.SubmitForm(ctx, doc, swapAction, map[string]string{
		"new_exercise_id": newExerciseID,
	}); err != nil {
		t.Fatalf("Submit swap: %v", err)
	}

	// The original slot URL must still resolve to a 200 — that's the whole point
	// of stable IDs.
	resp, err := client.Get(ctx, slotURL)
	if err != nil {
		t.Fatalf("Get slot URL after swap: %v", err)
	}
	if cerr := resp.Body.Close(); cerr != nil {
		t.Errorf("Close body: %v", cerr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected slot URL to remain valid after swap, got status %d", resp.StatusCode)
	}

	// Re-fetch the page to confirm the previously completed set is gone.
	if doc, err = client.GetDoc(ctx, slotURL); err != nil {
		t.Fatalf("Get slot page after swap: %v", err)
	}
	if doc.Find(".exercise-set.completed").Length() != 0 {
		t.Error("Completed sets from the pre-swap exercise must not carry over")
	}
}

// Test_application_workoutSwapExercise_search_filters_by_name verifies that the
// swap page filters compatible exercises by name substring (case-insensitive)
// when ?q= is set, echoes the query into the search input, and renders an empty
// state when nothing matches.
func Test_application_workoutSwapExercise_search_filters_by_name(t *testing.T) {
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

	testStart := time.Now()
	formData := map[string]string{
		testStart.Weekday().String():                  "60",
		testStart.AddDate(0, 0, 1).Weekday().String(): "60",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := testStart.Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("Start workout: %v", err)
	}

	var slotURL string
	doc.Find("a.exercise").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			if href, exists := s.Attr("href"); exists {
				slotURL = href
			}
		}
	})
	if slotURL == "" {
		t.Fatal("No exercise found on workout page")
	}

	// Collect the unfiltered set of compatible exercise names so the test is
	// data-driven against fixtures rather than coupled to specific names.
	if doc, err = client.GetDoc(ctx, slotURL+"/swap"); err != nil {
		t.Fatalf("Get swap page: %v", err)
	}
	var allNames []string
	doc.Find(".exercise-option .exercise-name").Each(func(_ int, s *goquery.Selection) {
		allNames = append(allNames, strings.TrimSpace(s.Text()))
	})
	if len(allNames) < 2 {
		t.Fatalf("Need at least 2 compatible exercises to exercise the filter, got %d", len(allNames))
	}

	// Pick the first word of the first exercise name as the search substring.
	// "Bench Press" → "bench". Mixed case to verify case-insensitivity.
	firstWord := strings.Fields(allNames[0])[0]
	if len(firstWord) < 3 {
		t.Fatalf("First word too short to be a useful filter: %q", firstWord)
	}
	needle := strings.ToUpper(firstWord)

	// Filtered query should return a (non-empty) subset, all containing the
	// substring case-insensitively.
	if doc, err = client.GetDoc(ctx, slotURL+"/swap?q="+url.QueryEscape(needle)); err != nil {
		t.Fatalf("Get swap page with query: %v", err)
	}
	var filteredNames []string
	doc.Find(".exercise-option .exercise-name").Each(func(_ int, s *goquery.Selection) {
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
	// swap forms.
	noMatch := "zzznotreal"
	if doc, err = client.GetDoc(ctx, slotURL+"/swap?q="+url.QueryEscape(noMatch)); err != nil {
		t.Fatalf("Get swap page with no-match query: %v", err)
	}
	if doc.Find(".exercise-option").Length() != 0 {
		t.Error("Expected zero exercise options when query matches nothing")
	}
	emptyState := doc.Find(".no-results")
	if emptyState.Length() == 0 {
		t.Fatal("Expected .no-results empty state when query matches nothing")
	}
	if !strings.Contains(emptyState.Text(), noMatch) {
		t.Errorf("Empty state %q should echo the query %q", strings.TrimSpace(emptyState.Text()), noMatch)
	}
}

func Test_application_exerciseSet_nonexistent_exercise_returns_custom_404(t *testing.T) {
	var (
		ctx = t.Context()
		err error
		doc *goquery.Document
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// Register and set up a workout
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Schedule today and the next weekday so a midnight crossing can't desync
	// the saved weekday from the URL date.
	testStart := time.Now()
	formData := map[string]string{
		testStart.Weekday().String():                  "60",
		testStart.AddDate(0, 0, 1).Weekday().String(): "60",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Failed to submit form: %v", err)
	}

	// Start a workout for today
	today := testStart.Format("2006-01-02")
	if _, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("Failed to submit start workout form: %v", err)
	}

	// Try to access a nonexistent exercise ID
	resp, err := client.Get(ctx, "/workouts/"+today+"/exercises/99999")
	if err != nil {
		t.Fatalf("Failed to get nonexistent exercise: %v", err)
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			t.Errorf("Failed to close response body: %v", err)
		}
	}()

	// Verify we get a 404 status code
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status code %d for nonexistent exercise, got %d", http.StatusNotFound, resp.StatusCode)
	}

	// Parse the response to check for custom 404 page
	doc, err = goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		t.Fatalf("Failed to parse 404 document: %v", err)
	}

	// Check for custom 404 page title
	title := doc.Find("h1").Text()
	if title != "404" {
		t.Errorf("Expected custom 404 page title '404', got: %q", title)
	}

	// Check for "Page Not Found" subtitle
	subtitle := doc.Find("h2").Text()
	if subtitle != "Page Not Found" {
		t.Errorf("Expected 'Page Not Found' subtitle, got: %q", subtitle)
	}

	// Check for Go Home link
	homeLinks := doc.Find("a[href='/']")
	if homeLinks.Length() == 0 {
		t.Error("Expected custom 404 page to contain home link")
	}

	// Check for Go Back button
	backButtons := doc.Find("button:contains('Go Back')")
	if backButtons.Length() == 0 {
		t.Error("Expected custom 404 page to contain 'Go Back' button")
	}
}

// Test_application_workoutSwapExercise_sorts_by_similarity verifies that the
// swap page renders compatible exercises in descending order of
// SwapSimilarityScore against the current slot's exercise, with alphabetical
// tie-breaks. Reads muscle-group data from the test DB so the assertion
// tracks fixture changes automatically.
func Test_application_workoutSwapExercise_sorts_by_similarity(t *testing.T) {
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

	testStart := time.Now()
	formData := map[string]string{
		testStart.Weekday().String():                  "60",
		testStart.AddDate(0, 0, 1).Weekday().String(): "60",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := testStart.Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("Start workout: %v", err)
	}

	var slotURL string
	doc.Find("a.exercise").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			if href, exists := s.Attr("href"); exists {
				slotURL = href
			}
		}
	})
	if slotURL == "" {
		t.Fatal("No exercise found on workout page")
	}

	if doc, err = client.GetDoc(ctx, slotURL+"/swap"); err != nil {
		t.Fatalf("Get swap page: %v", err)
	}

	// Recover the current exercise's name from the rendered Current Exercise
	// section so we can identify it in the DB load below.
	currentName := strings.TrimSpace(doc.Find(".current-exercise .name").Text())
	if currentName == "" {
		t.Fatal("Could not locate current exercise name on swap page")
	}

	// Load every exercise plus its muscle groups directly from the test
	// database. We build minimal workout.Exercise values populated with just
	// the fields SwapSimilarityScore reads (Category, PrimaryMuscleGroups,
	// SecondaryMuscleGroups, plus ID/Name for lookup and tie-breaks).
	db := server.DB()
	rows, err := db.QueryContext(ctx, `
		SELECT e.id, e.name, e.category, emg.muscle_group_name, emg.is_primary
		FROM exercises e
		LEFT JOIN exercise_muscle_groups emg ON emg.exercise_id = e.id
		ORDER BY e.id`)
	if err != nil {
		t.Fatalf("Query exercises: %v", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			t.Errorf("Close rows: %v", cerr)
		}
	}()
	byID := make(map[int]workout.Exercise)
	byName := make(map[string]workout.Exercise)
	for rows.Next() {
		var (
			id        int
			name      string
			category  string
			muscle    sql.NullString
			isPrimary sql.NullBool
		)
		if err = rows.Scan(&id, &name, &category, &muscle, &isPrimary); err != nil {
			t.Fatalf("Scan exercise row: %v", err)
		}
		ex, ok := byID[id]
		if !ok {
			ex = workout.Exercise{
				ID:                    id,
				Name:                  name,
				Category:              workout.Category(category),
				ExerciseType:          "",
				DescriptionMarkdown:   "",
				PrimaryMuscleGroups:   nil,
				SecondaryMuscleGroups: nil,
			}
		}
		if muscle.Valid {
			if isPrimary.Valid && isPrimary.Bool {
				ex.PrimaryMuscleGroups = append(ex.PrimaryMuscleGroups, muscle.String)
			} else {
				ex.SecondaryMuscleGroups = append(ex.SecondaryMuscleGroups, muscle.String)
			}
		}
		byID[id] = ex
		byName[name] = ex
	}
	if err = rows.Err(); err != nil {
		t.Fatalf("Iterate exercise rows: %v", err)
	}
	current, ok := byName[currentName]
	if !ok {
		t.Fatalf("Current exercise %q not found in DB", currentName)
	}

	// Walk rendered options in DOM order, capturing (id, name).
	type rendered struct {
		id   int
		name string
	}
	var renderedOpts []rendered
	doc.Find(".exercise-option").Each(func(_ int, s *goquery.Selection) {
		idStr, _ := s.Find("input[name='new_exercise_id']").Attr("value")
		id, convErr := strconv.Atoi(idStr)
		if convErr != nil {
			return
		}
		name := strings.TrimSpace(s.Find(".exercise-name").Text())
		renderedOpts = append(renderedOpts, rendered{id: id, name: name})
	})
	if len(renderedOpts) < 2 {
		t.Fatalf("Need at least 2 rendered options to assert ordering, got %d", len(renderedOpts))
	}

	// Build expected order: same set of ids, sorted by score desc then name asc.
	expected := make([]rendered, len(renderedOpts))
	copy(expected, renderedOpts)
	sort.SliceStable(expected, func(i, j int) bool {
		si := workout.SwapSimilarityScore(current, byID[expected[i].id])
		sj := workout.SwapSimilarityScore(current, byID[expected[j].id])
		if si != sj {
			return si > sj
		}
		return expected[i].name < expected[j].name
	})

	for i := range renderedOpts {
		if renderedOpts[i].id != expected[i].id {
			gotNames := make([]string, len(renderedOpts))
			wantNames := make([]string, len(expected))
			for k := range renderedOpts {
				gotNames[k] = renderedOpts[k].name
				wantNames[k] = expected[k].name
			}
			t.Errorf("Rendered order does not match score-sorted order.\n got: %v\nwant: %v",
				gotNames, wantNames)
			return
		}
	}
}

func Test_application_exerciseSet_assisted_storage(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Set preferences and start a workout for today. Schedule both today and the
	// next weekday so a midnight crossing during the test still leaves today
	// scheduled.
	testStart := time.Now()
	formData := map[string]string{
		testStart.Weekday().String():                  "60",
		testStart.AddDate(0, 0, 1).Weekday().String(): "60",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("submit preferences: %v", err)
	}
	today := testStart.Format("2006-01-02")
	if _, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("start workout: %v", err)
	}

	// Look up the seeded "Assisted Pull-Up" id (added in Task 6) and the
	// current authenticated user id (Register stores it in the session).
	db := server.DB()
	var assistedID int
	if err = db.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Assisted Pull-Up'`).Scan(&assistedID); err != nil {
		t.Fatalf("get Assisted Pull-Up id: %v", err)
	}

	// Insert a workout_exercise row pointing at Assisted Pull-Up, attached to
	// the just-started session for the test user. This avoids depending on
	// the swap UI's offered-alternatives logic.
	var slotID int
	if err = db.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id,
            warmup_completed_at)
         SELECT user_id, workout_date, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ')
         FROM workout_sessions WHERE workout_date = ?
         RETURNING id`, assistedID, today).Scan(&slotID); err != nil {
		t.Fatalf("insert assisted slot: %v", err)
	}
	// Seed four placeholder sets so the form has rows to submit against.
	for setNum := 1; setNum <= 4; setNum++ {
		if _, err = db.ExecContext(ctx,
			`INSERT INTO exercise_sets (workout_exercise_id, set_number,
                weight_kg, min_reps, max_reps)
             VALUES (?, ?, 0.0, 5, 5)`, slotID, setNum); err != nil {
			t.Fatalf("insert placeholder set %d: %v", setNum, err)
		}
	}

	slotPath := "/workouts/" + today + "/exercises/" + strconv.Itoa(slotID)
	if doc, err = client.GetDoc(ctx, slotPath); err != nil {
		t.Fatalf("get exercise set: %v", err)
	}

	setForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[name='signal']").Length() > 0
	}).First()
	if setForm.Length() == 0 {
		t.Fatalf("expected signal form on assisted exercise")
	}
	setAction, _ := setForm.Attr("action")

	// Submit set 1 with Assisted checkbox CHECKED → expect stored as negative.
	if doc, err = client.SubmitForm(ctx, doc, setAction, map[string]string{
		"weight":   "20",
		"assisted": "on",
		"signal":   "on_target",
		"reps":     "8",
	}); err != nil {
		t.Fatalf("submit assisted set: %v", err)
	}

	var weight1 float64
	if err = db.QueryRowContext(ctx,
		`SELECT weight_kg FROM exercise_sets
         WHERE workout_exercise_id = ? AND set_number = 1`,
		slotID).Scan(&weight1); err != nil {
		t.Fatalf("query set 1 weight: %v", err)
	}
	if weight1 != -20.0 {
		t.Errorf("set 1 weight = %v, want -20.0 (assisted checkbox should negate)", weight1)
	}

	// Submit set 2 with Assisted checkbox UNCHECKED → expect stored as positive.
	setForm2 := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[name='signal']").Length() > 0
	}).First()
	if setForm2.Length() == 0 {
		t.Fatalf("expected signal form for set 2")
	}
	setAction2, _ := setForm2.Attr("action")
	if doc, err = client.SubmitForm(ctx, doc, setAction2, map[string]string{
		"weight": "5",
		// no "assisted" field → unchecked
		"signal": "on_target",
		"reps":   "8",
	}); err != nil {
		t.Fatalf("submit weighted set on assisted exercise: %v", err)
	}

	var weight2 float64
	if err = db.QueryRowContext(ctx,
		`SELECT weight_kg FROM exercise_sets
         WHERE workout_exercise_id = ? AND set_number = 2`,
		slotID).Scan(&weight2); err != nil {
		t.Fatalf("query set 2 weight: %v", err)
	}
	if weight2 != 5.0 {
		t.Errorf("set 2 weight = %v, want +5.0 (no checkbox should leave sign positive)", weight2)
	}

	// Submit set 3 with a negative input AND assisted=on → guard prevents double-negation.
	// (Pattern attr forbids minus; this protects against paste/devtools bypass.)
	setForm3 := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[name='signal']").Length() > 0
	}).First()
	if setForm3.Length() == 0 {
		t.Fatalf("expected signal form for set 3")
	}
	setAction3, _ := setForm3.Attr("action")
	if _, err = client.SubmitForm(ctx, doc, setAction3, map[string]string{
		"weight":   "-15",
		"assisted": "on",
		"signal":   "on_target",
		"reps":     "8",
	}); err != nil {
		t.Fatalf("submit assisted set with negative input: %v", err)
	}

	var weight3 float64
	if err = db.QueryRowContext(ctx,
		`SELECT weight_kg FROM exercise_sets
         WHERE workout_exercise_id = ? AND set_number = 3`,
		slotID).Scan(&weight3); err != nil {
		t.Fatalf("query set 3 weight: %v", err)
	}
	if weight3 != -15.0 {
		t.Errorf("set 3 weight = %v, want -15.0 (negative input + assisted=on must not double-negate)", weight3)
	}
}
