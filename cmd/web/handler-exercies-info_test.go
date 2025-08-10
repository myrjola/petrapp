package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_application_exerciseInfo(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(ctx, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// First register to access pages
	t.Run("Setup", func(t *testing.T) {
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
		if doc, err = client.GetDoc(ctx, "/"); err != nil {
			t.Fatalf("Failed to get home page: %v", err)
		}

		// Find and submit the start workout form
		if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
			t.Fatalf("Failed to submit start workout form: %v", err)
		}
	})

	// Test viewing exercise info as a regular user
	t.Run("View exercise info as regular user", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		// First view the workout to find an exercise
		if doc, err = client.GetDoc(ctx, "/workouts/"+today); err != nil {
			t.Fatalf("Failed to get workout page: %v", err)
		}

		// Extract the first exercise ID from the workout page
		var exerciseID string
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

		// View the exercise info page
		if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID+"/info"); err != nil {
			t.Fatalf("Failed to get exercise info page: %v", err)
		}

		// Verify page content
		if doc.Find("h1").Length() == 0 {
			t.Error("Expected exercise name heading")
		}

		// Check for muscle groups
		if doc.Find(".muscle-group").Length() == 0 {
			t.Error("Expected to find muscle groups")
		}

		// Check for description
		if doc.Find("h2:contains(Instructions)").Length() == 0 {
			t.Error("Expected to find exercise description")
		}

		// Regular users should not see the edit button
		if doc.Find(".admin-edit").Length() > 0 {
			t.Error("Regular user should not see admin edit button")
		}
	})

	// Test viewing exercise info as an admin
	t.Run("View exercise info as admin", func(t *testing.T) {
		// Promote user to admin
		_, err = server.DB().Exec("UPDATE users SET is_admin = 1 WHERE TRUE")
		if err != nil {
			t.Fatalf("Failed to promote user to admin: %v", err)
		}

		today := time.Now().Format("2006-01-02")
		// First view the workout to find an exercise
		if doc, err = client.GetDoc(ctx, "/workouts/"+today); err != nil {
			t.Fatalf("Failed to get workout page: %v", err)
		}

		// Extract the first exercise ID from the workout page
		var exerciseID string
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

		// View the exercise info page
		if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID+"/info"); err != nil {
			t.Fatalf("Failed to get exercise info page: %v", err)
		}

		// Admin users should see the edit button
		if doc.Find(".admin-edit").Length() == 0 {
			t.Error("Admin user should see edit button")
		}

		// Check that edit button links to the correct URL
		editHref, exists := doc.Find(".admin-edit").Attr("href")
		if !exists {
			t.Error("Edit button has no href attribute")
		} else if got, want := editHref, "/admin/exercises/"+exerciseID; got != want {
			t.Errorf("Expected edit button href to be %q, got %q", want, got)
		}
	})

	// Test error handling with invalid input
	t.Run("Invalid exercise ID", func(t *testing.T) {
		today := time.Now().Format("2006-01-02")
		var resp *http.Response
		resp, err = client.Get(ctx, "/workouts/"+today+"/exercises/invalid/info")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for invalid exercise ID, got %d", resp.StatusCode)
		}
	})

	t.Run("Invalid date", func(t *testing.T) {
		var resp *http.Response
		resp, err = client.Get(ctx, "/workouts/invalid-date/exercises/1/info")
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 for invalid date, got %d", resp.StatusCode)
		}
	})
}
