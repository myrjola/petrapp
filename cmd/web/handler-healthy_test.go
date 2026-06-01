package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// newHealthTestApp builds a minimal application whose only wired dependencies
// are the logger and a service backed by a fresh in-memory database. Returns
// the app and the underlying database so the test can close it to simulate an
// unreachable database.
func newHealthTestApp(t *testing.T) (*application, *sqlite.Database) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	db, err := sqlite.NewDatabase(t.Context(), ":memory:", logger)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	svc := service.NewService(db, logger, "")
	app := &application{logger: logger, service: svc} //nolint:exhaustruct // health handler only needs these.
	return app, db
}

func Test_application_healthy_ok(t *testing.T) {
	t.Parallel()

	app, db := newHealthTestApp(t)
	t.Cleanup(func() { _ = db.Close() })

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/healthy", nil)
	rec := httptest.NewRecorder()
	app.healthy(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body %q: %v", rec.Body.String(), err)
	}
	if body.Status != "ok" {
		t.Errorf("status field: got %q, want %q", body.Status, "ok")
	}
}

func Test_application_healthy_databaseDown(t *testing.T) {
	t.Parallel()

	app, db := newHealthTestApp(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/healthy", nil)
	rec := httptest.NewRecorder()
	app.healthy(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
