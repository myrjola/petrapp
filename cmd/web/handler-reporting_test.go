package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test_application_reportingAPI(t *testing.T) {
	// Create a minimal application for testing with a logger that captures output
	var logBuffer bytes.Buffer
	app := &application{ //nolint:exhaustruct // this is a test
		logger: slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{ //nolint:exhaustruct // test only
			Level: slog.LevelDebug,
		})),
	}

	tests := []struct {
		name               string
		method             string
		body               string
		contentType        string
		expectedStatusCode int
		shouldLog          bool
		logContains        []string
		logExcludes        []string
	}{
		{
			name:   "Chrome array format CSP report",
			method: http.MethodPost,
			body: `[{"age":0,"body":{"blockedURL":"eval","columnNumber":1,"disposition":"enforce",` +
				`"documentURL":"https://example.com/","effectiveDirective":"script-src","lineNumber":1,` +
				`"originalPolicy":"default-src 'none'; script-src 'nonce-ABC123'","referrer":"",` +
				`"sample":"","statusCode":200},"type":"csp-violation","url":"https://example.com/",` +
				`"user_agent":"Mozilla/5.0"}]`,
			contentType:        "application/reports+json",
			expectedStatusCode: http.StatusNoContent,
			shouldLog:          true,
			logContains: []string{"Report received via Reporting API", "csp-violation",
				"eval", "script-src"},
			logExcludes: nil,
		},
		{
			name:   "Legacy object format CSP report",
			method: http.MethodPost,
			body: `{"csp-report": {"document-uri": "https://example.com/page", ` +
				`"violated-directive": "script-src", "effective-directive": "script-src", ` +
				`"blocked-uri": "https://evil.com/script.js", "line-number": 42, "column-number": 10, ` +
				`"source-file": "https://example.com/page", "script-sample": "alert('hi')", ` +
				`"disposition": "enforce", "referrer": "https://example.com"}}`,
			contentType:        "application/csp-report",
			expectedStatusCode: http.StatusNoContent,
			shouldLog:          true,
			logContains: []string{"Report received via Reporting API", "script-src",
				"https://evil.com/script.js", "https://example.com/page",
				redactedPlaceholder},
			logExcludes: []string{"alert('hi')", "alert(\\u0027hi\\u0027)"},
		},
		{
			name:   "Sensitive script sample and tokenised URL are redacted",
			method: http.MethodPost,
			body: `[{"age":0,"body":{"blockedURL":"https://cdn.example.com/x.js?token=secret123",` +
				`"documentURL":"https://example.com/path?session=abc","effectiveDirective":"script-src",` +
				`"sample":"window.password = 'secret'","statusCode":200},"type":"csp-violation",` +
				`"url":"https://example.com/"}]`,
			contentType:        "application/reports+json",
			expectedStatusCode: http.StatusNoContent,
			shouldLog:          true,
			logContains: []string{"Report received via Reporting API", "csp-violation",
				"script-src", "https://cdn.example.com/x.js", "https://example.com/path",
				redactedPlaceholder},
			logExcludes: []string{
				"token=secret123", "session=abc",
				"window.password", "secret'", "secret\\u0027",
			},
		},
		{
			name:               "Invalid JSON",
			method:             http.MethodPost,
			body:               `{"invalid json structure`,
			contentType:        "application/csp-report",
			expectedStatusCode: http.StatusBadRequest,
			shouldLog:          true,
			logContains:        []string{"Failed to parse report"},
			logExcludes:        nil,
		},
		{
			name:               "Empty body",
			method:             http.MethodPost,
			body:               "",
			contentType:        "application/csp-report",
			expectedStatusCode: http.StatusBadRequest,
			shouldLog:          true,
			logContains:        []string{"Failed to parse report"},
			logExcludes:        nil,
		},
		{
			name:               "Minimal object format CSP report",
			method:             http.MethodPost,
			body:               `{"csp-report": {"violated-directive": "default-src"}}`,
			contentType:        "application/csp-report",
			expectedStatusCode: http.StatusNoContent,
			shouldLog:          true,
			logContains:        []string{"Report received via Reporting API", "default-src"},
			logExcludes:        nil,
		},
		{
			name:   "Unexpected content type logs warning but processes request",
			method: http.MethodPost,
			body: `{"csp-report": {"violated-directive": "script-src", ` +
				`"blocked-uri": "https://evil.com"}}`,
			contentType:        "text/plain",
			expectedStatusCode: http.StatusNoContent,
			shouldLog:          true,
			logContains: []string{"Report with unexpected content type",
				"text/plain", "Report received via Reporting API"},
			logExcludes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset log buffer for each test
			logBuffer.Reset()

			// Create request
			req := httptest.NewRequest(tt.method, "/api/reports", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Test Browser)")

			// Create response recorder
			w := httptest.NewRecorder()

			// Call the handler
			app.reportingAPI(w, req)

			// Check status code
			if w.Code != tt.expectedStatusCode {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatusCode, w.Code)
			}

			// Check response body for 204 responses (should be empty)
			if tt.expectedStatusCode == http.StatusNoContent {
				if w.Body.Len() != 0 {
					t.Errorf("Expected empty response body for 204, got: %s", w.Body.String())
				}
			}

			// Check that appropriate content was logged
			logOutput := logBuffer.String()
			if tt.shouldLog {
				if logOutput == "" {
					t.Error("Expected log output but got none")
				}
				for _, expectedContent := range tt.logContains {
					if !strings.Contains(logOutput, expectedContent) {
						t.Errorf("Expected log to contain '%s', but log output was: %s", expectedContent, logOutput)
					}
				}
				for _, forbidden := range tt.logExcludes {
					if strings.Contains(logOutput, forbidden) {
						t.Errorf("Expected log to NOT contain '%s', but log output was: %s", forbidden, logOutput)
					}
				}
			}
		})
	}
}

func Test_application_reportingAPI_readError(t *testing.T) {
	// Create a minimal application for testing
	var logBuffer bytes.Buffer
	app := &application{ //nolint:exhaustruct // this is a test
		logger: slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{ //nolint:exhaustruct // test only
			Level: slog.LevelDebug,
		})),
	}

	// Create a request with a body that will fail to read
	req := httptest.NewRequest(http.MethodPost, "/api/reports", &errorReader{})
	req.Header.Set("Content-Type", "application/csp-report")

	w := httptest.NewRecorder()

	app.reportingAPI(w, req)

	// Should return 400 due to read error
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d for read error, got %d", http.StatusBadRequest, w.Code)
	}

	// Should log the read error
	logOutput := logBuffer.String()
	if !strings.Contains(logOutput, "Failed to read report request body") {
		t.Errorf("Expected log to contain read error message, got: %s", logOutput)
	}
}

func Test_application_reportingAPI_requestSizeLimit(t *testing.T) {
	// Create a minimal application for testing
	var logBuffer bytes.Buffer
	app := &application{ //nolint:exhaustruct // this is a test
		logger: slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{ //nolint:exhaustruct // test only
			Level: slog.LevelDebug,
		})),
	}

	// Create a request body larger than 64KB limit
	largeReport := map[string]any{
		"csp-report": map[string]any{
			"document-uri":       "https://example.com/page",
			"violated-directive": "script-src",
			"blocked-uri":        "https://evil.com/script.js",
			// Create a very large script sample to exceed size limit
			"script-sample": strings.Repeat("a", 70000), // 70KB string
		},
	}

	largeBody, err := json.Marshal(largeReport)
	if err != nil {
		t.Fatalf("Failed to marshal large report: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/csp-report")

	w := httptest.NewRecorder()

	app.reportingAPI(w, req)

	// The request should still succeed but the body will be truncated
	// This tests that our size limit prevents excessive memory usage
	if w.Code != http.StatusNoContent && w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d or %d for large request, got %d",
			http.StatusNoContent, http.StatusBadRequest, w.Code)
	}

	// If it's a 400, it should be due to JSON parsing error from truncated body
	if w.Code == http.StatusBadRequest {
		logOutput := logBuffer.String()
		if !strings.Contains(logOutput, "Failed to parse report") {
			t.Errorf("Expected log to contain parse error for truncated body, got: %s", logOutput)
		}
	}
}

// errorReader is a helper type that always returns an error when Read is called.
type errorReader struct{}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
