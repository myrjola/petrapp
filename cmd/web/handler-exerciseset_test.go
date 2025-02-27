package main

import (
	"context"
	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"net/http"
	"os"
	"testing"
)

//nolint:gocognit // this is a test function
func Test_application_exerciseSet(t *testing.T) {
	var (
		ctx = context.Background()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(context.Background(), os.Stdout, testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// First register to get authenticated
	if doc, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Test 1: Accessing exercise set page without authentication should redirect to home
	t.Run("Requires authentication", func(t *testing.T) {
		resp, err := http.Get(server.URL() + "/workouts/2025-02-27/exercises/1")
		if err != nil {
			t.Fatalf("Failed to get exercise set page: %v", err)
		}
		if got, want := resp.Request.URL.Path, "/"; got != want {
			t.Errorf("Expected redirect to %q, got %q", want, got)
		}
	})

	// Test 2: Access exercise set page with authentication
	t.Run("Shows exercise set with forms", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/workouts/2025-02-27/exercises/1"); err != nil {
			t.Fatalf("Failed to get exercise set page: %v", err)
		}

		// Check that we're on the exercise set page
		title := doc.Find("h1").Text()
		if title != "Squat" {
			t.Errorf("Expected exercise title 'Squat', got %q", title)
		}

		// Check that the form for the first set exists
		form, err := e2etest.FindForm(doc, "/workouts/2025-02-27/exercises/1/sets/0/done")
		if err != nil {
			t.Fatalf("Failed to find form for first set: %v", err)
		}

		// Check that the weight input exists and has correct value
		weightInput := form.Find("input[name=weight]")
		weightValue, exists := weightInput.Attr("value")
		if !exists || weightValue != "20" {
			t.Errorf("Expected weight input with value '20', got %q", weightValue)
		}

		// Check that the reps input exists
		repsInput := form.Find("input[name=reps]")
		if repsInput.Length() == 0 {
			t.Errorf("Reps input not found")
		}
	})

	// Test 3: Complete a set
	t.Run("Can complete a set", func(t *testing.T) {
		// Submit form with reps value
		formData := map[string]string{
			"weight": "20",
			"reps":   "10",
		}

		if doc, err = client.GetDoc(ctx, "/workouts/2025-02-27/exercises/1"); err != nil {
			t.Fatalf("Failed to get exercise set page: %v", err)
		}

		// Submit the form to mark the set as complete
		if doc, err = client.SubmitForm(ctx, doc, "/workouts/2025-02-27/exercises/1/sets/0/done", formData); err != nil {
			t.Fatalf("Failed to submit form: %v", err)
		}

		// Check if we're redirected back to the exercise set page
		if doc.Url.Path != "/workouts/2025-02-27/exercises/1" {
			t.Errorf("Expected to be on exercise set page, got %q", doc.Url.Path)
		}

		// Check that the completed set shows the correct reps
		completedReps := doc.Find(".set.completed .reps").Text()
		if completedReps != "10 reps" {
			t.Errorf("Expected completed set to show '10 reps', got %q", completedReps)
		}

		// Check that the edit button exists
		editButton := doc.Find(".edit-button")
		if editButton.Length() == 0 {
			t.Errorf("Edit button not found for completed set")
		}
	})

	// Test 4: Edit a completed set
	t.Run("Can edit a completed set", func(t *testing.T) {
		// Navigate to the edit page
		if doc, err = client.GetDoc(ctx, "/workouts/2025-02-27/exercises/1?edit=0"); err != nil {
			t.Fatalf("Failed to get edit page: %v", err)
		}

		// Find the update form
		form, err := e2etest.FindForm(doc, "/workouts/2025-02-27/exercises/1/sets/0/update")
		if err != nil {
			t.Fatalf("Failed to find update form: %v", err)
		}

		// Check that the reps input is enabled and has the current value
		repsInput := form.Find("input[name=reps]")
		disabled, exists := repsInput.Attr("disabled")
		if exists && disabled != "" {
			t.Errorf("Expected reps input to be enabled, but it's disabled")
		}

		repsValue, exists := repsInput.Attr("value")
		if !exists || repsValue != "10" {
			t.Errorf("Expected reps input to have value '10', got %q", repsValue)
		}

		// Submit form with new reps value
		formData := map[string]string{
			"weight": "20",
			"reps":   "12",
		}

		// Submit the form to update the set
		if doc, err = client.SubmitForm(ctx, doc, "/workouts/2025-02-27/exercises/1/sets/0/update", formData); err != nil {
			t.Fatalf("Failed to submit update form: %v", err)
		}

		// Check if we're redirected back to the exercise set page (with no query parameters)
		if doc.Url.Path != "/workouts/2025-02-27/exercises/1" || doc.Url.RawQuery != "" {
			t.Errorf("Expected to be on clean exercise set page, got URL %q", doc.Url.String())
		}

		// Check that the completed set shows the updated reps
		completedReps := doc.Find(".set.completed .reps").Text()
		if completedReps != "12 reps" {
			t.Errorf("Expected completed set to show '12 reps', got %q", completedReps)
		}
	})
}
