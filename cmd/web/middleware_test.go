package main

import (
	"context"
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
				handler := app.routes()

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

// workoutServiceForMaintenance defines the minimal interface needed for maintenance mode testing.
type workoutServiceForMaintenance interface {
	IsMaintenanceModeEnabled(ctx context.Context) bool
}

// mockWorkoutService is a mock implementation of the workout service for testing maintenance mode.
type mockWorkoutService struct {
	maintenanceEnabled bool
}

func (m *mockWorkoutService) IsMaintenanceModeEnabled(_ context.Context) bool {
	return m.maintenanceEnabled
}

// testApplication is a minimal application structure for testing.
type testApplication struct {
	logger         *slog.Logger
	workoutService workoutServiceForMaintenance
}

// maintenanceMode implements the maintenance middleware for testing.
func (app *testApplication) maintenanceMode(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Exclude health endpoints from maintenance checks.
		if r.URL.Path == "/api/healthy" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if maintenance mode is enabled (skip if workoutService is nil for tests)
		if app.workoutService != nil && app.workoutService.IsMaintenanceModeEnabled(ctx) {
			// Allow admin access during maintenance.
			isAdmin := contexthelpers.IsAdmin(r.Context())
			if isAdmin {
				next.ServeHTTP(w, r)
				return
			}

			// Add Retry-After header for better HTTP compliance.
			w.Header().Set("Retry-After", "300")

			// Render the maintenance page
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Maintenance Mode"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func Test_application_maintenanceMode(t *testing.T) {
	tests := []struct {
		name                  string
		path                  string
		isAdmin               bool
		maintenanceEnabled    bool
		expectedStatus        int
		expectRetryHeader     bool
		expectMaintenancePage bool
	}{
		{
			name:                  "regular user sees maintenance page when enabled",
			path:                  "/",
			isAdmin:               false,
			maintenanceEnabled:    true,
			expectedStatus:        http.StatusServiceUnavailable,
			expectRetryHeader:     true,
			expectMaintenancePage: true,
		},
		{
			name:                  "admin bypasses maintenance mode",
			path:                  "/",
			isAdmin:               true,
			maintenanceEnabled:    true,
			expectedStatus:        http.StatusOK,
			expectRetryHeader:     false,
			expectMaintenancePage: false,
		},
		{
			name:                  "health endpoint bypasses maintenance mode",
			path:                  "/api/healthy",
			isAdmin:               false,
			maintenanceEnabled:    true,
			expectedStatus:        http.StatusOK,
			expectRetryHeader:     false,
			expectMaintenancePage: false,
		},
		{
			name:                  "normal operation when maintenance disabled",
			path:                  "/",
			isAdmin:               false,
			maintenanceEnabled:    false,
			expectedStatus:        http.StatusOK,
			expectRetryHeader:     false,
			expectMaintenancePage: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock workout service
			mockService := &mockWorkoutService{
				maintenanceEnabled: tt.maintenanceEnabled,
			}

			// Create a test application with our mock service
			app := &testApplication{
				logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
				workoutService: mockService,
			}

			// Create a test handler that simulates successful processing
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("success"))
			})

			// Apply the maintenance middleware
			handler := app.maintenanceMode(testHandler)

			// Create the request
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.isAdmin {
				req = contexthelpers.AuthenticateContext(req, []byte("admin-user-id"), true)
			}

			// Create response recorder
			w := httptest.NewRecorder()

			// Execute the request
			handler.ServeHTTP(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check Retry-After header
			retryAfter := w.Header().Get("Retry-After")
			if tt.expectRetryHeader {
				if retryAfter != "300" {
					t.Errorf("Expected Retry-After header with value '300', got '%s'", retryAfter)
				}
			} else {
				if retryAfter != "" {
					t.Errorf("Expected no Retry-After header, got '%s'", retryAfter)
				}
			}

			// Check for maintenance page content (this would be more comprehensive in a real test)
			body := w.Body.String()
			if tt.expectMaintenancePage {
				if !strings.Contains(body, "Maintenance Mode") {
					t.Errorf("Expected maintenance page content, but it was not found in response body")
				}
			} else {
				if strings.Contains(body, "Maintenance Mode") {
					t.Errorf("Did not expect maintenance page content, but it was found in response body")
				}
			}
		})
	}
}
