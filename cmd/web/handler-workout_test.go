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
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
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

	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/add-exercise", nil); err != nil {
		t.Fatalf("Failed to submit add exercise form: %v", err)
	}

	// Verify we're back on the workout page
	if doc.Find("a.exercise").Length() <= initialExerciseCount {
		t.Errorf("Expected more exercises after adding one, got %d (was %d)",
			doc.Find("a.exercise").Length(), initialExerciseCount)
	}

	// Verify the exercise was added (by checking the count increased)
	newExerciseCount := doc.Find("a.exercise").Length()
	if newExerciseCount != initialExerciseCount+1 {
		t.Errorf("Expected exercise count to increase by 1, got %d (was %d)",
			newExerciseCount, initialExerciseCount)
	}
}

func Test_application_workoutNotFound(t *testing.T) {
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
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
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
	doc.Find(".exercise-option .exercise-name").Each(func(_ int, s *goquery.Selection) {
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
	// add forms.
	noMatch := "zzznotreal"
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/add-exercise?q="+url.QueryEscape(noMatch)); err != nil {
		t.Fatalf("Get add-exercise page with no-match query: %v", err)
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
