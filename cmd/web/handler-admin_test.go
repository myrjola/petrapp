package main

import (
	"net/http"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

//nolint:paralleltest // subtests sequentially promote the same user to admin.
func Test_application_adminGET(t *testing.T) {
	var (
		ctx = t.Context()
	)
	server, err := e2etest.StartServer(t, testkit.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// httpClient is shared by both subtests so the second subtest reuses the
	// admin-promoted session.
	httpClient := *client.HTTPClient() // shallow copy preserves jar + transport.
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	t.Run("Non-admin is bounced to /forbidden", func(t *testing.T) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+"/admin", nil)
		if reqErr != nil {
			t.Fatalf("Build /admin request: %v", reqErr)
		}
		resp, getErr := httpClient.Do(req)
		if getErr != nil {
			t.Fatalf("GET /admin: %v", getErr)
		}
		if cerr := resp.Body.Close(); cerr != nil {
			t.Fatalf("Close response body: %v", cerr)
		}

		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("Expected 303, got %d", resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "/forbidden" {
			t.Errorf("Expected Location: /forbidden, got %q", loc)
		}
	})

	t.Run("Admin gets redirected to /admin/exercises", func(t *testing.T) {
		if _, err = server.DB().Exec("UPDATE users SET is_admin = 1 WHERE TRUE"); err != nil {
			t.Fatalf("Promote to admin: %v", err)
		}

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+"/admin", nil)
		if reqErr != nil {
			t.Fatalf("Build /admin request: %v", reqErr)
		}
		resp, getErr := httpClient.Do(req)
		if getErr != nil {
			t.Fatalf("GET /admin: %v", getErr)
		}
		if cerr := resp.Body.Close(); cerr != nil {
			t.Fatalf("Close response body: %v", cerr)
		}

		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("Expected 303, got %d", resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "/admin/exercises" {
			t.Errorf("Expected Location: /admin/exercises, got %q", loc)
		}
	})
}
