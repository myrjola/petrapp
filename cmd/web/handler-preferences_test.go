package main

import (
	"context"
	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"net/url"
	"os"
	"testing"
)

//nolint:gocognit // This test is inherently complex due to the nature of the tested function.
func Test_application_preferences(t *testing.T) {
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

	checkboxIsChecked := func(doc *goquery.Document, id string) bool {
		t.Helper()
		return doc.Find("input#" + id + "[type='checkbox']").Is("[checked]")
	}

	t.Run("Requires authentication", func(t *testing.T) {
		resp, err := client.Get(ctx, "/preferences")
		if err != nil {
			t.Fatalf("Failed to get preferences: %v", err)
		}
		if got, want := resp.Request.URL.Path, "/"; got != want {
			t.Errorf("Expected redirect to %q, got %q", want, got)
		}
	})

	t.Run("Shows default preferences", func(t *testing.T) {
		// First register to get authenticated
		doc, err = client.Register(ctx)
		if err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		// Then navigate to preferences
		doc, err = client.GetDoc(ctx, "/preferences")
		if err != nil {
			t.Fatalf("Failed to get preferences: %v", err)
		}

		// Check that we're on the preferences page
		if doc.Find("h1").Text() != "Weekly Schedule" {
			t.Error("Expected to be on preferences page")
		}

		// By default, all days should be unchecked
		weekdays := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
		for _, day := range weekdays {
			if checkboxIsChecked(doc, day) {
				t.Errorf("Expected %s to be unchecked by default", day)
			}
		}
	})

	t.Run("Can update preferences", func(t *testing.T) {
		// Submit form with Monday and Wednesday checked
		formData := url.Values{
			"monday":    {"true"},
			"wednesday": {"true"},
		}
		doc, err = client.SubmitForm(ctx, doc, "/preferences", formData)
		if err != nil {
			t.Fatalf("Failed to submit form: %v", err)
		}

		// After submission, we should still be on preferences page
		if doc.Find("h1").Text() != "Weekly Schedule" {
			t.Error("Expected to stay on preferences page after submission")
		}

		// Verify Monday and Wednesday are checked, others unchecked
		weekdays := map[string]bool{
			"monday":    true,
			"tuesday":   false,
			"wednesday": true,
			"thursday":  false,
			"friday":    false,
			"saturday":  false,
			"sunday":    false,
		}

		for day, shouldBeChecked := range weekdays {
			isChecked := checkboxIsChecked(doc, day)
			if isChecked != shouldBeChecked {
				t.Errorf("Expected %s checked status to be %v, got %v", day, shouldBeChecked, isChecked)
			}
		}
	})

	t.Run("Preferences persist after logout and login", func(t *testing.T) {
		// First logout
		doc, err = client.Logout(ctx)
		if err != nil {
			t.Fatalf("Failed to logout: %v", err)
		}

		// Then login again
		doc, err = client.Login(ctx)
		if err != nil {
			t.Fatalf("Failed to login: %v", err)
		}

		// Navigate to preferences
		doc, err = client.GetDoc(ctx, "/preferences")
		if err != nil {
			t.Fatalf("Failed to get preferences: %v", err)
		}

		// Verify preferences were persisted
		weekdays := map[string]bool{
			"monday":    true,
			"tuesday":   false,
			"wednesday": true,
			"thursday":  false,
			"friday":    false,
			"saturday":  false,
			"sunday":    false,
		}

		for day, shouldBeChecked := range weekdays {
			isChecked := checkboxIsChecked(doc, day)
			if isChecked != shouldBeChecked {
				t.Errorf("Expected %s checked status to be %v, got %v", day, shouldBeChecked, isChecked)
			}
		}
	})
}
