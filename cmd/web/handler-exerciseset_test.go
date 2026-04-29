package main

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
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

	// Set workout preferences (enable today's weekday).
	formData := map[string]string{
		time.Now().Weekday().String(): "60",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Failed to submit form: %v", err)
	}

	// Start a workout for today
	today := time.Now().Format("2006-01-02")
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

	formData := map[string]string{time.Now().Weekday().String(): "60"}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Submit preferences: %v", err)
	}

	today := time.Now().Format("2006-01-02")
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

	// Set workout preferences (enable today's weekday).
	formData := map[string]string{
		time.Now().Weekday().String(): "60",
	}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Failed to submit form: %v", err)
	}

	// Start a workout for today
	today := time.Now().Format("2006-01-02")
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
