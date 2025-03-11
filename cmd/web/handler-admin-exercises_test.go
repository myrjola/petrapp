package main

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"io"
	"net/http"
	"testing"
)

func Test_application_adminExercises(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(ctx, io.Discard, testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// First register to access admin pages
	t.Run("Setup", func(t *testing.T) {
		if _, err = client.Register(ctx); err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		// Assert that the user is not an admin expect 401
		var resp *http.Response
		if resp, err = client.Get(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}
		if got, want := resp.StatusCode, http.StatusUnauthorized; got != want {
			t.Fatalf("Got status code %d, want %d", got, want)
		}

		// Promote user to admin
		if _, err = client.GetDoc(ctx, "/api/testing/promote-to-admin"); err != nil {
			t.Fatalf("Failed to promote user to admin: %v", err)
		}
	})

	// Test viewing the exercises admin page
	t.Run("View exercises admin page", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}

		// Verify page content
		if doc.Find("h1").Text() != "Exercise Administration" {
			t.Error("Expected 'Exercise Administration' heading")
		}

		// Check for exercise table
		if doc.Find("table").Length() == 0 {
			t.Error("Expected to find exercise table")
		}

		// Check for the form to add a new exercise
		if doc.Find("form[action='/admin/exercises/create']").Length() == 0 {
			t.Error("Expected to find form to add new exercise")
		}
	})

	// Test creating a new exercise
	t.Run("Create new exercise", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}

		// Find the creation form
		form := doc.Find("form[action='/admin/exercises/create']")
		if form.Length() == 0 {
			t.Fatalf("Exercise creation form not found")
		}

		// Submit the form with test data
		formData := map[string]string{
			"Name":        "Test Squat",
			"Category":    "lower",
			"Primary":     "Quads,Glutes",
			"Secondary":   "Hamstrings,Calves",
			"Description": "A test squat exercise description",
		}

		if doc, err = client.SubmitForm(ctx, doc, "/admin/exercises/create", formData); err != nil {
			t.Fatalf("Failed to submit exercise creation form: %v", err)
		}

		// Verify we're back at the exercises page
		if doc.Find("h1").Text() != "Exercise Administration" {
			t.Error("Expected to be redirected back to 'Exercise Administration' page")
		}

		// Check that our new exercise appears in the table
		if doc.Find("td:contains('Test Squat')").Length() == 0 {
			t.Error("Expected to find the newly created exercise in the table")
		}
	})

	// Test editing an exercise
	t.Run("Edit exercise", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}

		// Find the edit link for our created exercise
		var editURL string
		doc.Find("tr:contains('Test Squat') td a:contains('Edit')").Each(func(_ int, s *goquery.Selection) {
			if href, exists := s.Attr("href"); exists {
				editURL = href
			}
		})

		if editURL == "" {
			t.Fatalf("Edit link for Test Squat not found")
		}

		// Navigate to the edit page
		if doc, err = client.GetDoc(ctx, editURL); err != nil {
			t.Fatalf("Failed to get exercise edit page: %v", err)
		}

		// Verify edit page content
		if doc.Find("h1:contains('Edit Exercise: Test Squat')").Length() == 0 {
			t.Error("Expected to find 'Edit Exercise: Test Squat' heading")
		}

		// Update the exercise
		formData := map[string]string{
			"Name":        "Updated Test Squat",
			"Category":    "lower",
			"Primary":     "Quads,Glutes",
			"Secondary":   "Hamstrings,Calves,Abs,Lower Back",
			"Description": "An updated test squat exercise description",
		}

		if doc, err = client.SubmitForm(ctx, doc, editURL, formData); err != nil {
			t.Fatalf("Failed to submit exercise update form: %v", err)
		}

		// Verify we're back at the exercises page
		if doc.Find("h1").Text() != "Exercise Administration" {
			t.Error("Expected to be redirected back to 'Exercise Administration' page")
		}

		// Check that our exercise was updated
		if doc.Find("td:contains('Updated Test Squat')").Length() == 0 {
			t.Error("Expected to find the updated exercise name in the table")
		}
	})
}
