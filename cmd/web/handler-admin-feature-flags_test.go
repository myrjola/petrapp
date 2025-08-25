package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_application_adminFeatureFlags(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
	)
	server, err := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	// Register a normal user first
	doc, err = client.Register(ctx)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	t.Run("Feature flags admin shows message for non-admin users", func(t *testing.T) {
		// Try to access feature flags admin as non-admin user
		nonAdminDoc, getErr := client.GetDoc(ctx, "/admin/feature-flags")
		if getErr != nil {
			t.Fatalf("Failed to get response: %v", getErr)
		}

		// Should show message about needing admin privileges
		text := nonAdminDoc.Find("p").Text()
		if !strings.Contains(text, "administrator privileges") {
			t.Errorf("Expected message about admin privileges, got: %s", text)
		}

		// Should not show feature flags table
		table := nonAdminDoc.Find("table")
		if table.Length() > 0 {
			t.Error("Non-admin user should not see feature flags table")
		}
	})

	t.Run("Promote to admin and access feature flags", func(t *testing.T) {
		// Promote user to admin
		_, err = server.DB().Exec("UPDATE users SET is_admin = 1 WHERE TRUE")
		if err != nil {
			t.Fatalf("Failed to promote user to admin: %v", err)
		}

		// Access the feature flags admin page
		doc, err = client.GetDoc(ctx, "/admin/feature-flags")
		if err != nil {
			t.Fatalf("Failed to get feature flags admin page: %v", err)
		}

		// Check that the page contains feature flags elements
		title := doc.Find("h1").Text()
		if !strings.Contains(title, "Feature Flags Administration") {
			t.Errorf("Expected feature flags admin title, got: %s", title)
		}

		// Check for table headers
		headers := doc.Find("th")
		expectedHeaders := []string{"Name", "Status", "Actions"}
		for i, expected := range expectedHeaders {
			if i >= headers.Length() {
				t.Errorf("Missing header: %s", expected)
				continue
			}
			actual := headers.Eq(i).Text()
			if actual != expected {
				t.Errorf("Expected header %s, got %s", expected, actual)
			}
		}
	})

	t.Run("Test health endpoint accessibility", func(t *testing.T) {
		// Health endpoint should always be accessible
		resp, getErr := client.Get(ctx, "/api/healthy")
		if getErr != nil {
			t.Fatalf("Failed to access health endpoint: %v", getErr)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Health endpoint should return 200, got %d", resp.StatusCode)
		}
	})
}
