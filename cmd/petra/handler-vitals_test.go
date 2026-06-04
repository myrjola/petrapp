package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

//nolint:paralleltest // subtests share a single logBuffer (Reset between cases).
func Test_application_vitalsPOST(t *testing.T) {
	var logBuffer bytes.Buffer
	app := &application{ //nolint:exhaustruct // test only.
		logger: slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{ //nolint:exhaustruct // test only.
			Level: slog.LevelDebug,
		})),
	}

	tests := []struct {
		name               string
		body               string
		expectedStatusCode int
		logContains        []string
	}{
		{
			name: "Single metric",
			body: `{"path":"/workouts/2026-05-26","navigationType":"navigate",` +
				`"metrics":[{"name":"LCP","value":1234.5,"rating":"good","target":"img.hero"}]}`,
			expectedStatusCode: http.StatusNoContent,
			logContains: []string{
				"web vital", "metric=LCP", "value=1234.5", "rating=good",
				"target=img.hero", "path=/workouts/2026-05-26", "nav_type=navigate",
			},
		},
		{
			name: "Batched metrics",
			body: `{"path":"/","navigationType":"navigate","metrics":[` +
				`{"name":"LCP","value":1800,"rating":"good","target":""},` +
				`{"name":"INP","value":250,"rating":"needs-improvement","target":"button.signal-btn"},` +
				`{"name":"FCP","value":900,"rating":"good","target":""},` +
				`{"name":"TTFB","value":120,"rating":"good","target":""}]}`,
			expectedStatusCode: http.StatusNoContent,
			logContains: []string{
				"metric=LCP", "metric=INP", "metric=FCP", "metric=TTFB",
				"target=button.signal-btn", "rating=needs-improvement",
			},
		},
		{
			name:               "Empty metrics array",
			body:               `{"path":"/","navigationType":"reload","metrics":[]}`,
			expectedStatusCode: http.StatusNoContent,
			logContains:        nil,
		},
		{
			name:               "Invalid JSON",
			body:               `{"path":"/", invalid`,
			expectedStatusCode: http.StatusBadRequest,
			logContains:        []string{"Failed to parse vitals report"},
		},
		{
			name:               "Empty body",
			body:               "",
			expectedStatusCode: http.StatusBadRequest,
			logContains:        []string{"Failed to parse vitals report"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logBuffer.Reset()

			req := httptest.NewRequest(http.MethodPost, "/api/vitals", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "Mozilla/5.0 (Test Browser)")

			w := httptest.NewRecorder()
			app.vitalsPOST(w, req)

			if w.Code != tt.expectedStatusCode {
				t.Errorf("Expected status %d, got %d", tt.expectedStatusCode, w.Code)
			}

			logOutput := logBuffer.String()
			for _, expected := range tt.logContains {
				if !strings.Contains(logOutput, expected) {
					t.Errorf("Expected log to contain %q, got: %s", expected, logOutput)
				}
			}
		})
	}
}
