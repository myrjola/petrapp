package main

import (
	"strconv"
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

	// Set workout preferences
	formData := map[string]string{
		"Monday": "60",
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

	// Find the set completion form (which contains a button with type="submit" and text "Done!")
	form := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[type=submit]:contains('Done!')").Length() > 0
	}).First()

	if form.Length() == 0 {
		t.Fatalf("Set completion form not found")
	}

	// Get the form action
	action, exists := form.Attr("action")
	if !exists {
		t.Fatalf("Form has no action attribute")
	}

	// Submit the form with weight and reps
	formData = map[string]string{
		"weight": "20,5", // Test comma for decimal point
		"reps":   "10",   // Using 10 reps as a valid number
	}

	if doc, err = client.SubmitForm(ctx, doc, action, formData); err != nil {
		t.Fatalf("Failed to submit set completion form: %v", err)
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

	// Find the edit form (which contains a button with type="submit" and text "Done!")
	form = doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[type=submit]:contains('Done!')").Length() > 0
	}).First()

	if form.Length() == 0 {
		t.Fatalf("Edit form not found")
	}

	action, exists = form.Attr("action")
	if !exists {
		t.Fatalf("Edit form has no action attribute")
	}

	// Get the current weight value
	var weight string
	weight, exists = form.Find("input[name='weight']").Attr("value")
	if !exists {
		t.Fatalf("Edit form has no weight input")
	}

	// Convert weight to float and increase it
	weightFloat, _ := strconv.ParseFloat(weight, 64)
	newWeight := weightFloat + 2.5 // Increase by 2.5 kg

	// Update the completed set with new weight and reps
	formData = map[string]string{
		"weight": strconv.FormatFloat(newWeight, 'f', 1, 64),
		"reps":   "12", // Increase reps
	}

	if doc, err = client.SubmitForm(ctx, doc, action, formData); err != nil {
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
