package main

import (
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
