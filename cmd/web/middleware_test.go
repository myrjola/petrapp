package main

import (
	"fmt"
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

// SetWriteDeadline is needed to not get "feature not implemented" error.
func (w *timeoutResponseWriter) SetWriteDeadline(_ time.Time) error {
	// No-op for testing
	return nil
}

func Test_application_timeout(t *testing.T) {
	tests := []struct {
		name     string
		sleepMS  int
		isAdmin  bool
		timesOut bool
	}{
		{
			name:     "completes within timeout",
			sleepMS:  500,
			isAdmin:  false,
			timesOut: false,
		},
		{
			name:     "times out for regular user",
			sleepMS:  3000,
			isAdmin:  false,
			timesOut: true,
		},
		{
			name:     "admin gets longer timeout",
			sleepMS:  28000,
			isAdmin:  true,
			timesOut: false,
		},
		{
			name:     "admin times out",
			sleepMS:  31000,
			isAdmin:  true,
			timesOut: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Create a minimal application for testing
				app := &application{ //nolint:exhaustruct // this is a test
					logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				}
				handler, err := app.routes()
				if err != nil {
					t.Fatalf("Failed to set up routes: %v", err)
				}

				url := fmt.Sprintf("/api/test/timeout?sleep_ms=%d", tt.sleepMS)
				req := httptest.NewRequest(http.MethodGet, url, nil)
				if tt.isAdmin {
					req = contexthelpers.AuthenticateContext(req, []byte("admin-user-id"), true)
				}
				w := newTimeoutResponseWriter()

				handler.ServeHTTP(w, req)

				time.Sleep(time.Duration(tt.sleepMS) * time.Millisecond)

				if tt.timesOut {
					// TimeoutHandler returns 503 Service Unavailable with "timed out" message
					if w.Code != http.StatusServiceUnavailable {
						t.Errorf("Expected status 503 on timeout, got %d", w.Code)
					}

					if !strings.Contains(w.Body.String(), "timed out") {
						t.Errorf("Expected timeout message in response body, got: %s", w.Body.String())
					}
				} else if w.Code != http.StatusOK {
					t.Errorf("Expected status 200, got %d", w.Code)
				}
			})
		})
	}
}
