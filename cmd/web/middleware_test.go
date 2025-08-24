package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
)

type timeoutResponseWriter struct {
	httptest.ResponseRecorder
}

func newTimeoutResponseWriter() *timeoutResponseWriter {
	return &timeoutResponseWriter{
		ResponseRecorder: *httptest.NewRecorder(),
	}
}

func (w *timeoutResponseWriter) Unwrap() http.ResponseWriter {
	return &w.ResponseRecorder // for Flush
}

func (w *timeoutResponseWriter) SetReadDeadline(_ time.Time) error {
	// No-op for testing
	return nil
}

func (w *timeoutResponseWriter) SetWriteDeadline(_ time.Time) error {
	// No-op for testing
	return nil
}

func Test_application_timeout_completes_within_timeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create a minimal application for testing
		app := &application{
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		handler := app.routes()

		req := httptest.NewRequest("GET", "/api/test/timeout?sleep_ms=500", nil)
		w := newTimeoutResponseWriter()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})
}

func Test_application_timeout_times_out_for_regular_user(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create a minimal application for testing
		app := &application{
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		handler := app.routes()

		req := httptest.NewRequest("GET", "/api/test/timeout?sleep_ms=3000", nil)
		w := newTimeoutResponseWriter()

		handler.ServeHTTP(w, req)

		time.Sleep(3000 * time.Millisecond) // Ensure the handler has time to process

		// TimeoutHandler returns 503 Service Unavailable with "timed out" message
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status 503 on timeout, got %d", w.Code)
		}

		if !strings.Contains(w.Body.String(), "timed out") {
			t.Errorf("Expected timeout message in response body, got: %s", w.Body.String())
		}
	})
}

func Test_application_timeout_admin_gets_longer_timeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create a minimal application for testing
		app := &application{
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		}
		handler := app.routes()

		// Create request with admin context
		req := httptest.NewRequest("GET", "/api/test/timeout?sleep_ms=28000", nil)
		req = contexthelpers.AuthenticateContext(req, []byte("admin-user-id"), true)
		w := newTimeoutResponseWriter()

		// Test a request that would time out for regular users but not admins.
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected admin request to succeed, got status %d", w.Code)
		}
	})
}
