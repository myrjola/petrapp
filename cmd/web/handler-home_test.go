package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func testLookupEnv(key string) (string, bool) {
	switch key {
	case "PETRAPP_SQLITE_URL":
		return ":memory:", true
	case "PETRAPP_ADDR":
		return "localhost:0", true
	case "PETRAPP_TRACES_DIRECTORY":
		return "", true // Use default (empty string means use module root)
	default:
		return "", false
	}
}

func Test_application_home(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
	)
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	t.Run("Initial state", func(t *testing.T) {
		doc, err = client.GetDoc(ctx, "/")
		if err != nil {
			t.Fatalf("Failed to get document: %v", err)
		}

		checkButtonPresence(t, doc, "Sign in", 1)
		checkButtonPresence(t, doc, "Register", 1)
	})

	t.Run("After registration", func(t *testing.T) {
		doc, err = client.Register(ctx)
		if err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		checkButtonPresence(t, doc, "Sign in", 0)
		checkButtonPresence(t, doc, "Register", 0)
	})

	t.Run("After logout", func(t *testing.T) {
		doc, err = client.Logout(ctx)
		if err != nil {
			t.Fatalf("Failed to logout: %v", err)
		}

		checkButtonPresence(t, doc, "Sign in", 1)
		checkButtonPresence(t, doc, "Register", 1)
	})

	t.Run("After login", func(t *testing.T) {
		doc, err = client.Login(ctx)
		if err != nil {
			t.Fatalf("Failed to login: %v", err)
		}

		checkButtonPresence(t, doc, "Sign in", 0)
		checkButtonPresence(t, doc, "Register", 0)
	})
}

func checkButtonPresence(t *testing.T, doc *goquery.Document, buttonText string, expectedCount int) {
	t.Helper()
	count := doc.Find("button:contains('" + buttonText + "')").Length()
	if count != expectedCount {
		t.Errorf("Expected %d '%s' button(s), but found %d", expectedCount, buttonText, count)
	}
}

func Test_crossOriginProtection(t *testing.T) {
	ctx := t.Context()
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Create a malicious client that simulates cross-origin requests
	maliciousClient, err := e2etest.NewClientWithSecFetchSite(
		server.URL(),
		"localhost",
		server.URL(),
		"cross-site", // This simulates a malicious cross-origin request
	)
	if err != nil {
		t.Fatalf("Failed to create malicious client: %v", err)
	}

	// Get the home page to find a form that should be protected
	doc, err := maliciousClient.GetDoc(ctx, "/")
	if err != nil {
		t.Fatalf("Failed to get home page: %v", err)
	}

	// Try to submit the registration form with cross-origin headers
	// This should be blocked by the CSRF protection
	_, err = maliciousClient.SubmitForm(ctx, doc, "/api/registration/start", nil)
	if err == nil {
		t.Error("Expected cross-origin form submission to be blocked, but it succeeded")
	}

	// The error should indicate that the request was blocked (likely a 403 or similar status)
	if !containsStatusError(err, 403) && !containsStatusError(err, 400) {
		t.Errorf("Expected status error 403 or 400 for blocked request, got: %v", err)
	}
}

// containsStatusError checks if the error contains a specific HTTP status code.
func containsStatusError(err error, statusCode int) bool {
	return err != nil &&
		(err.Error() == fmt.Sprintf("unexpected status code: %d", statusCode) ||
			strings.Contains(err.Error(), fmt.Sprintf("status code: %d", statusCode)))
}

// submitLanguageChange is a helper that directly submits to /set-language via SubmitForm.
// It uses a dummy doc with just the form, since SubmitForm needs a doc parameter.
func submitLanguageChange(t *testing.T, client *e2etest.Client, lang string) (*goquery.Document, error) {
	t.Helper()
	// Get the current page which has the form.
	doc, err := client.GetDoc(t.Context(), "/")
	if err != nil {
		return nil, fmt.Errorf("get current page: %w", err)
	}

	// Extract hidden fields from the current page.
	hiddenFields := make(map[string]string)
	doc.Find("form[action='/set-language'] input[type='hidden']").Each(func(_ int, s *goquery.Selection) {
		var exists bool
		var name, value string
		if name, exists = s.Attr("name"); exists {
			if value, exists = s.Attr("value"); exists {
				hiddenFields[name] = value
			}
		}
	})

	// Now we need to submit without using labels. The simplest approach is to just
	// prepare the form data manually and use the internal HTTP client.
	// Since we can't access internal methods, let's use a workaround: we'll modify
	// the document HTML to have an English label that we can use.

	// Actually, simpler approach: just get a fresh page (which will be in English by default for new tests)
	// and use that to submit. Or, add form data directly.

	// Let's extract the select element directly by name and set its value.
	selectElem := doc.Find("select[name='language']")
	if selectElem.Length() == 0 {
		return nil, errors.New("language select not found")
	}

	// Use SubmitForm with empty formFields map and manually prepare form data.
	// Actually, we can't do this easily. Let me try a different approach:
	// Since SubmitForm looks for labels, and labels change, we'll extract
	// the current label text and use that.

	label := doc.Find("form[action='/set-language'] label").First()
	if label.Length() == 0 {
		return nil, errors.New("language label not found")
	}
	labelText := strings.TrimSpace(label.Text())

	// Now submit with the current label text.
	formFields := map[string]string{
		labelText: lang,
	}

	return client.SubmitForm(t.Context(), doc, "/set-language", formFields)
}

func Test_languageSwitching(t *testing.T) {
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

	// Test that we can change language and see translated content.
	t.Run("Switch to Finnish", func(t *testing.T) {
		// Get the home page.
		doc, err = client.GetDoc(ctx, "/")
		if err != nil {
			t.Fatalf("Failed to get document: %v", err)
		}

		// Verify default English content.
		if doc.Find("h1:contains('Petra')").Length() != 1 {
			t.Error("Expected to find English title 'Petra'")
		}
		if doc.Find("p:contains('Personal trainer in your pocket.')").Length() != 1 {
			t.Error("Expected to find English tagline")
		}

		// Find the language form.
		form := doc.Find("form[action='/set-language']")
		if form.Length() == 0 {
			t.Fatal("Language form not found")
		}

		// Submit the form with Finnish language.
		doc, err = submitLanguageChange(t, client, "fi")
		if err != nil {
			t.Fatalf("Failed to submit language form: %v", err)
		}

		// Verify Finnish content.
		if doc.Find("p:contains('Henkilökohtainen valmentaja taskussasi.')").Length() != 1 {
			t.Error("Expected to find Finnish tagline")
		}
		if doc.Find("button:contains('Kirjaudu')").Length() != 1 {
			t.Error("Expected to find Finnish 'Kirjaudu' button")
		}

		// Verify the correct language is selected in the dropdown.
		selected := doc.Find("select[name='language'] option[selected]")
		if selected.Length() != 1 {
			t.Error("Expected exactly one selected option")
		}
		if selectedValue, _ := selected.Attr("value"); selectedValue != "fi" {
			t.Errorf("Expected selected language to be 'fi', got '%s'", selectedValue)
		}
	})

	t.Run("Switch to Swedish", func(t *testing.T) {
		// Submit the form with Swedish language.
		doc, err = submitLanguageChange(t, client, "sv")
		if err != nil {
			t.Fatalf("Failed to submit language form: %v", err)
		}

		// Verify Swedish content.
		if doc.Find("p:contains('Personlig tränare i fickan.')").Length() != 1 {
			t.Error("Expected to find Swedish tagline")
		}
		if doc.Find("button:contains('Logga in')").Length() != 1 {
			t.Error("Expected to find Swedish 'Logga in' button")
		}

		// Verify the correct language is selected in the dropdown.
		selected := doc.Find("select[name='language'] option[selected]")
		if selected.Length() != 1 {
			t.Error("Expected exactly one selected option")
		}
		if selectedValue, _ := selected.Attr("value"); selectedValue != "sv" {
			t.Errorf("Expected selected language to be 'sv', got '%s'", selectedValue)
		}
	})

	t.Run("Switch back to English", func(t *testing.T) {
		// Submit the form with English language.
		doc, err = submitLanguageChange(t, client, "en")
		if err != nil {
			t.Fatalf("Failed to submit language form: %v", err)
		}

		// Verify English content.
		if doc.Find("p:contains('Personal trainer in your pocket.')").Length() != 1 {
			t.Error("Expected to find English tagline")
		}
		if doc.Find("button:contains('Sign in')").Length() != 1 {
			t.Error("Expected to find English 'Sign in' button")
		}

		// Verify the correct language is selected in the dropdown.
		selected := doc.Find("select[name='language'] option[selected]")
		if selected.Length() != 1 {
			t.Error("Expected exactly one selected option")
		}
		if selectedValue, _ := selected.Attr("value"); selectedValue != "en" {
			t.Errorf("Expected selected language to be 'en', got '%s'", selectedValue)
		}
	})

	t.Run("Invalid language", func(t *testing.T) {
		// Try to submit with an invalid language.
		_, err = submitLanguageChange(t, client, "invalid")
		if err == nil {
			t.Error("Expected error when submitting invalid language, but got none")
		}

		// Should get a 400 Bad Request.
		if !containsStatusError(err, 400) {
			t.Errorf("Expected status error 400 for invalid language, got: %v", err)
		}
	})

	t.Run("Language persists across requests", func(t *testing.T) {
		// Set to Finnish.
		_, err = submitLanguageChange(t, client, "fi")
		if err != nil {
			t.Fatalf("Failed to submit language form: %v", err)
		}

		// Make a new request and verify language is still Finnish.
		doc, err = client.GetDoc(ctx, "/")
		if err != nil {
			t.Fatalf("Failed to get document: %v", err)
		}

		if doc.Find("p:contains('Henkilökohtainen valmentaja taskussasi.')").Length() != 1 {
			t.Error("Expected language preference to persist across requests")
		}

		selected := doc.Find("select[name='language'] option[selected]")
		if selectedValue, _ := selected.Attr("value"); selectedValue != "fi" {
			t.Errorf("Expected selected language to be 'fi' after page reload, got '%s'", selectedValue)
		}
	})
}
