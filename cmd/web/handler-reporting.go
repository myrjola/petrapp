package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// reportingAPI handles reports sent via the Reporting API.
// See: https://developer.mozilla.org/en-US/docs/Web/API/Reporting_API
func (app *application) reportingAPI(w http.ResponseWriter, r *http.Request) {
	// Validate content type (should be application/csp-report, application/json, or application/reports+json)
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && contentType != "application/csp-report" &&
		contentType != "application/json" && contentType != "application/reports+json" {
		app.logger.LogAttrs(r.Context(), slog.LevelWarn, "Report with unexpected content type",
			slog.String("content_type", contentType))
	}

	defer r.Body.Close()

	// Limit request body size to prevent abuse (64KB should be sufficient for reports)
	const maxBodySize = 64 * 1024
	limitedReader := io.LimitReader(r.Body, maxBodySize)

	// Read the request body
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to read report request body",
			slog.String("error", err.Error()))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Parse the report - can be either an array or an object
	var payload any
	var dataArray []map[string]any
	var dataObject map[string]any

	// Try parsing as array first.
	err = json.Unmarshal(body, &dataArray)
	if err == nil {
		payload = dataArray
	} else {
		// Try parsing as object.
		err = json.Unmarshal(body, &dataObject)
		if err != nil {
			app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse report",
				slog.String("error", err.Error()),
				slog.String("body", string(body)))
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
		payload = dataObject
	}

	// Log the report with the payload
	app.logger.LogAttrs(r.Context(), slog.LevelWarn, "Report received via Reporting API",
		slog.Any("payload", payload),
		slog.String("user_agent", r.Header.Get("User-Agent")))

	// Respond with 204 No Content as per Reporting API specification
	w.WriteHeader(http.StatusNoContent)
}
