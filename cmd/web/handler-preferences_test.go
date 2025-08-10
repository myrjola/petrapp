package main

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_application_preferences(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
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

	// By default, all days should be set to rest day (0 minutes)
	weekdays := map[string]int{
		"monday":    0,
		"tuesday":   0,
		"wednesday": 0,
		"thursday":  0,
		"friday":    0,
		"saturday":  0,
		"sunday":    0,
	}
	verifySelected(t, doc, weekdays)

	// Can update preferences.
	// Submit form with Monday at 60 minutes and Wednesday at 45 minutes
	formData := map[string]string{
		"monday_minutes":    "60",
		"wednesday_minutes": "45",
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

	weekdaysAfterSubmit := map[string]int{
		"monday":    60,
		"tuesday":   0,
		"wednesday": 45,
		"thursday":  0,
		"friday":    0,
		"saturday":  0,
		"sunday":    0,
	}
	verifySelected(t, doc, weekdaysAfterSubmit)

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
	weekdaysAfterPersistence := map[string]int{
		"monday":    60,
		"tuesday":   0,
		"wednesday": 45,
		"thursday":  0,
		"friday":    0,
		"saturday":  0,
		"sunday":    0,
	}
	verifySelected(t, doc, weekdaysAfterPersistence)
}

func verifySelected(t *testing.T, doc *goquery.Document, weekdays map[string]int) {
	t.Helper()
	form, err := e2etest.FindForm(doc, "/preferences")
	if err != nil {
		t.Fatalf("Failed to find form: %v", err)
	}

	for day, expectedMinutes := range weekdays {
		selectName := day + "_minutes"
		selectElement := form.Find("select[name='" + selectName + "']")
		if selectElement.Length() == 0 {
			t.Fatalf("Failed to find select element for %s", selectName)
		}

		selectedOption := selectElement.Find("option[selected]")
		if selectedOption.Length() == 0 {
			// If no option is explicitly selected, find the first option (default)
			selectedOption = selectElement.Find("option").First()
		}

		selectedValue := selectedOption.AttrOr("value", "")
		if selectedValue == "" {
			t.Fatalf("No selected value found for %s", selectName)
		}

		actualMinutes := 0
		if selectedValue != "0" {
			actualMinutes, err = strconv.Atoi(selectedValue)
			if err != nil {
				t.Fatalf("Failed to parse selected value %s for %s: %v", selectedValue, selectName, err)
			}
		}

		if actualMinutes != expectedMinutes {
			t.Errorf("Expected %s to have %d minutes selected, got %d", day, expectedMinutes, actualMinutes)
		}
	}
}
