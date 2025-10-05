package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// cspViolation handles CSP violation reports.
func (app *application) cspViolation(w http.ResponseWriter, r *http.Request) {
	// Validate content type (should be application/csp-report, application/json, or application/reports+json)
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && contentType != "application/csp-report" &&
		contentType != "application/json" && contentType != "application/reports+json" {
		app.logger.LogAttrs(r.Context(), slog.LevelWarn, "CSP violation report with unexpected content type",
			slog.String("content_type", contentType))
	}

	defer r.Body.Close()

	// Limit request body size to prevent abuse (64KB should be sufficient for CSP reports)
	const maxBodySize = 64 * 1024
	limitedReader := io.LimitReader(r.Body, maxBodySize)

	// Read the request body
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to read CSP violation request body",
			slog.String("error", err.Error()))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Parse the CSP violation report
	var data map[string]any
	err = json.Unmarshal(body, &data)
	if err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse CSP violation report",
			slog.String("error", err.Error()),
			slog.String("body", string(body)))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Log the CSP violation with the payload
	app.logger.LogAttrs(r.Context(), slog.LevelWarn, "CSP violation detected",
		slog.Any("payload", data),
		slog.String("user_agent", r.Header.Get("User-Agent")))

	// Respond with 204 No Content as per CSP specification
	w.WriteHeader(http.StatusNoContent)
}
