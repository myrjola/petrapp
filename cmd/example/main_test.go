package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func TestRoutes_HealthyOK(t *testing.T) {
	t.Parallel()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	app, cleanup, err := newTestApplication(t, logger)
	if err != nil {
		t.Fatalf("newTestApplication: %v", err)
	}
	t.Cleanup(cleanup)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthy", nil)
	app.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthy = %d, want 200", rec.Code)
	}
}
