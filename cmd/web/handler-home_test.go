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

	t.Run("Muscle balance section renders after login", func(t *testing.T) {
		// Schedule preferences so a weekly plan gets generated; without this the
		// home handler redirects to /schedule and never renders the section.
		doc, err = client.GetDoc(ctx, "/preferences")
		if err != nil {
			t.Fatalf("get preferences page: %v", err)
		}
		if doc, err = client.SubmitForm(ctx, doc, "/preferences", map[string]string{
			"Monday":    "60",
			"Wednesday": "60",
			"Friday":    "60",
		}); err != nil {
			t.Fatalf("submit preferences: %v", err)
		}

		doc, err = client.GetDoc(ctx, "/")
		if err != nil {
			t.Fatalf("get home: %v", err)
		}

		section := doc.Find("section.muscle-balance")
		if section.Length() != 1 {
			t.Fatalf("want exactly one .muscle-balance section, got %d", section.Length())
		}

		// Section must render after the weekly schedule, not before, so the visual
		// reading order is "this week's days, then how the load is distributed".
		main := doc.Find("main").First()
		schedule := main.Find(".weekly-schedule").First()
		if schedule.Length() == 0 {
			t.Fatal("weekly-schedule must exist on home page")
		}
		schedulePos := -1
		balancePos := -1
		main.Children().Each(func(i int, s *goquery.Selection) {
			if s.HasClass("weekly-schedule") {
				schedulePos = i
			}
			if s.HasClass("muscle-balance") {
				balancePos = i
			}
		})
		if schedulePos < 0 || balancePos < 0 || balancePos < schedulePos {
			t.Errorf("want muscle-balance after weekly-schedule, got positions schedule=%d balance=%d",
				schedulePos, balancePos)
		}

		// One row per known muscle group (17 from fixtures.sql).
		const wantMuscleGroups = 17
		if rows := section.Find(".row").Length(); rows != wantMuscleGroups {
			t.Errorf("want %d muscle group rows, got %d", wantMuscleGroups, rows)
		}

		// Targeted groups must show their target text; untargeted ones must not.
		chestRow := section.Find(`.row[data-slug="chest"]`).First()
		if chestRow.Length() == 0 {
			t.Fatal("chest row missing")
		}
		if !strings.Contains(chestRow.Text(), "target 10") {
			t.Errorf("chest row must show seeded target of 10, got: %q", chestRow.Text())
		}
		calvesRow := section.Find(`.row[data-slug="calves"]`).First()
		if calvesRow.Length() == 0 {
			t.Fatal("calves row missing")
		}
		if calvesRow.Find(".target-mark").Length() != 0 {
			t.Error("calves has no seeded target; must not render a target mark")
		}
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
