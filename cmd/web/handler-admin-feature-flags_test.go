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
	t.Cleanup(server.Shutdown)

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

func Test_application_maintenanceMode_integration(t *testing.T) {
	var (
		ctx = t.Context()
	)
	server, err := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	t.Cleanup(server.Shutdown)

	client := server.Client()

	// Register and promote user to admin
	_, err = client.Register(ctx)
	if err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	_, err = server.DB().Exec("UPDATE users SET is_admin = 1 WHERE TRUE")
	if err != nil {
		t.Fatalf("Failed to promote user to admin: %v", err)
	}

	// Enable maintenance mode
	_, err = server.DB().Exec("INSERT OR REPLACE INTO feature_flags (name, enabled) VALUES ('maintenance_mode', 1)")
	if err != nil {
		t.Fatalf("Failed to enable maintenance mode: %v", err)
	}

	t.Run("Regular user sees maintenance page when enabled", func(t *testing.T) {
		// Create a new client without admin privileges
		nonAdminServer, serverErr := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
		if serverErr != nil {
			t.Fatalf("Failed to start non-admin server: %v", serverErr)
		}
		t.Cleanup(nonAdminServer.Shutdown)
		nonAdminClient := nonAdminServer.Client()

		// Register a regular user
		_, registerErr := nonAdminClient.Register(ctx)
		if registerErr != nil {
			t.Fatalf("Failed to register non-admin user: %v", registerErr)
		}

		// Enable maintenance mode in the non-admin server DB
		_, execErr := nonAdminServer.DB().Exec(
			"INSERT OR REPLACE INTO feature_flags (name, enabled) VALUES ('maintenance_mode', 1)")
		if execErr != nil {
			t.Fatalf("Failed to enable maintenance mode: %v", execErr)
		}

		// Try to access a regular page
		resp, getErr := nonAdminClient.Get(ctx, "/")
		if getErr != nil {
			t.Fatalf("Failed to get response: %v", getErr)
		}

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
		}

		// Check Retry-After header
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "300" {
			t.Errorf("Expected Retry-After header with value '300', got '%s'", retryAfter)
		}
	})

	t.Run("Admin bypasses maintenance mode", func(t *testing.T) {
		// Access home page as admin - should work during maintenance
		resp, getErr := client.Get(ctx, "/")
		if getErr != nil {
			t.Fatalf("Failed to get response: %v", getErr)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Admin should bypass maintenance mode, got status %d", resp.StatusCode)
		}

		// Should not have Retry-After header for admin
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			t.Errorf("Admin should not get Retry-After header, got '%s'", retryAfter)
		}
	})

	t.Run("Health endpoint bypasses maintenance mode", func(t *testing.T) {
		// Health endpoint should always be accessible
		resp, getErr := client.Get(ctx, "/api/healthy")
		if getErr != nil {
			t.Fatalf("Failed to access health endpoint: %v", getErr)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Health endpoint should bypass maintenance mode, got status %d", resp.StatusCode)
		}

		// Should not have Retry-After header
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			t.Errorf("Health endpoint should not get Retry-After header, got '%s'", retryAfter)
		}
	})

	t.Run("Feature flags admin bypasses maintenance mode", func(t *testing.T) {
		// Feature flags admin should be accessible during maintenance
		resp, getErr := client.Get(ctx, "/admin/feature-flags")
		if getErr != nil {
			t.Fatalf("Failed to access feature flags admin: %v", getErr)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Feature flags admin should bypass maintenance mode, got status %d", resp.StatusCode)
		}

		// Should not have Retry-After header
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			t.Errorf("Feature flags admin should not get Retry-After header, got '%s'", retryAfter)
		}
	})

	t.Run("Authentication APIs bypass maintenance mode", func(t *testing.T) {
		// Create a new client without admin privileges to test auth API access
		nonAdminServer, serverErr := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
		if serverErr != nil {
			t.Fatalf("Failed to start non-admin server: %v", serverErr)
		}
		t.Cleanup(nonAdminServer.Shutdown)
		nonAdminClient := nonAdminServer.Client()

		// Enable maintenance mode in the non-admin server DB
		_, execErr := nonAdminServer.DB().Exec(
			"INSERT OR REPLACE INTO feature_flags (name, enabled) VALUES ('maintenance_mode', 1)")
		if execErr != nil {
			t.Fatalf("Failed to enable maintenance mode: %v", execErr)
		}

		// Test that we can access login start endpoint during maintenance
		// Using GET which should return method not allowed, but not service unavailable
		resp, getErr := nonAdminClient.Get(ctx, "/api/login/start")
		if getErr != nil {
			t.Fatalf("Failed to access login start: %v", getErr)
		}

		// Should not be blocked by maintenance mode (might return 405 due to GET vs POST, but not 503)
		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Error("Login start API should bypass maintenance mode")
		}

		// Should not have Retry-After header
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			t.Errorf("Login start API should not get Retry-After header, got '%s'", retryAfter)
		}

		// Test login finish endpoint similarly
		resp, getErr = nonAdminClient.Get(ctx, "/api/login/finish")
		if getErr != nil {
			t.Fatalf("Failed to access login finish: %v", getErr)
		}

		// Should not be blocked by maintenance mode
		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Error("Login finish API should bypass maintenance mode")
		}

		// Should not have Retry-After header
		retryAfter = resp.Header.Get("Retry-After")
		if retryAfter != "" {
			t.Errorf("Login finish API should not get Retry-After header, got '%s'", retryAfter)
		}
	})

	t.Run("Static files bypass maintenance mode", func(t *testing.T) {
		// Create a new client without admin privileges to test static file access
		nonAdminServer, serverErr := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
		if serverErr != nil {
			t.Fatalf("Failed to start non-admin server: %v", serverErr)
		}
		t.Cleanup(nonAdminServer.Shutdown)
		nonAdminClient := nonAdminServer.Client()

		// Enable maintenance mode in the non-admin server DB
		_, execErr := nonAdminServer.DB().Exec(
			"INSERT OR REPLACE INTO feature_flags (name, enabled) VALUES ('maintenance_mode', 1)")
		if execErr != nil {
			t.Fatalf("Failed to enable maintenance mode: %v", execErr)
		}

		// Test access to CSS file (use existing file)
		resp, getErr := nonAdminClient.Get(ctx, "/main.css")
		if getErr != nil {
			t.Fatalf("Failed to access static file: %v", getErr)
		}

		// Should not be blocked by maintenance mode (might return 404 if file doesn't exist, but not 503)
		if resp.StatusCode == http.StatusServiceUnavailable {
			t.Error("Static files should bypass maintenance mode")
		}

		// Should not have Retry-After header
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			t.Errorf("Static files should not get Retry-After header, got '%s'", retryAfter)
		}
	})

	t.Run("Normal operation when maintenance disabled", func(t *testing.T) {
		// Disable maintenance mode
		_, err = server.DB().Exec("INSERT OR REPLACE INTO feature_flags (name, enabled) VALUES ('maintenance_mode', 0)")
		if err != nil {
			t.Fatalf("Failed to disable maintenance mode: %v", err)
		}

		// Create a new client without admin privileges
		nonAdminServer, serverErr := e2etest.StartServer(t.Context(), testhelpers.NewWriter(t), testLookupEnv, run)
		if serverErr != nil {
			t.Fatalf("Failed to start non-admin server: %v", serverErr)
		}
		t.Cleanup(nonAdminServer.Shutdown)
		nonAdminClient := nonAdminServer.Client()

		// Register a regular user
		_, registerErr := nonAdminClient.Register(ctx)
		if registerErr != nil {
			t.Fatalf("Failed to register non-admin user: %v", registerErr)
		}

		// Disable maintenance mode in the non-admin server DB
		_, execErr := nonAdminServer.DB().Exec(
			"INSERT OR REPLACE INTO feature_flags (name, enabled) VALUES ('maintenance_mode', 0)")
		if execErr != nil {
			t.Fatalf("Failed to disable maintenance mode: %v", execErr)
		}

		// Try to access a regular page
		resp, getErr := nonAdminClient.Get(ctx, "/")
		if getErr != nil {
			t.Fatalf("Failed to get response: %v", getErr)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Should work normally when maintenance disabled, got status %d", resp.StatusCode)
		}

		// Should not have Retry-After header
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			t.Errorf("Should not get Retry-After header when maintenance disabled, got '%s'", retryAfter)
		}
	})
}
