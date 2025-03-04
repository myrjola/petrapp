package main

import (
	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"net/http"
	"os"
	"testing"
)

func Test_application_preferences(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t.Context(), os.Stdout, testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// Requires authentication
	var resp *http.Response
	resp, err = client.Get(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	if got, want := resp.Request.URL.Path, "/"; got != want {
		t.Errorf("Expected redirect to %q, got %q", want, got)
	}

	// Shows default preferences
	// First register to get authenticated
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Then navigate to preferences
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	// Check that we're on the preferences page
	if doc.Find("h1").Text() != "Weekly Schedule" {
		t.Error("Expected to be on preferences page")
	}

	// By default, all days should be unchecked
	weekdays := map[string]bool{
		"Monday":    false,
		"Tuesday":   false,
		"Wednesday": false,
		"Thursday":  false,
		"Friday":    false,
		"Saturday":  false,
		"Sunday":    false,
	}
	verifyChecked(t, doc, weekdays)

	// Can update preferences.
	// Submit form with Monday and Wednesday checked
	formData := map[string]string{
		"Monday":    "true",
		"Wednesday": "true",
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("Failed to submit form: %v", err)
	}

	// After submission, we go to the home page.
	if doc.Url.Path != "/" {
		t.Errorf("Expected to be on home page, got %q", doc.Url.Path)
	}

	// Navigate to preferences again
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	weekdays = map[string]bool{
		"Monday":    true,
		"Tuesday":   false,
		"Wednesday": true,
		"Thursday":  false,
		"Friday":    false,
		"Saturday":  false,
		"Sunday":    false,
	}
	verifyChecked(t, doc, weekdays)

	// First logout
	if _, err = client.Logout(ctx); err != nil {
		t.Fatalf("Failed to logout: %v", err)
	}

	// Then login again
	if _, err = client.Login(ctx); err != nil {
		t.Fatalf("Failed to login: %v", err)
	}

	// Navigate to preferences
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}

	// Verify preferences were persisted
	weekdays = map[string]bool{
		"Monday":    true,
		"Tuesday":   false,
		"Wednesday": true,
		"Thursday":  false,
		"Friday":    false,
		"Saturday":  false,
		"Sunday":    false,
	}
	verifyChecked(t, doc, weekdays)
}

func verifyChecked(t *testing.T, doc *goquery.Document, weekdays map[string]bool) {
	t.Helper()
	var (
		form *goquery.Selection
		err  error
	)
	for day, shouldBeChecked := range weekdays {
		if form, err = e2etest.FindForm(doc, "/preferences"); err != nil {
			t.Fatalf("Failed to find form: %v", err)
		}
		var input *goquery.Selection
		if input, err = e2etest.FindInputForLabel(form, day); err != nil {
			t.Fatalf("Failed to find input for label %s: %v", day, err)
		}
		if got, want := input.Is("[checked]"), shouldBeChecked; got != want {
			t.Errorf("Expected %s checked status to be %v, got %v", day, shouldBeChecked, got)
		}
	}
}
