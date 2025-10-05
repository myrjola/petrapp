package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// CSPViolationReport represents the structure of a CSP violation report.
type CSPViolationReport struct {
	CSPReport struct {
		DocumentURI        string `json:"document-uri"`
		Referrer           string `json:"referrer"`
		ViolatedDirective  string `json:"violated-directive"`
		EffectiveDirective string `json:"effective-directive"`
		OriginalPolicy     string `json:"original-policy"`
		Disposition        string `json:"disposition"`
		BlockedURI         string `json:"blocked-uri"`
		LineNumber         int    `json:"line-number"`
		ColumnNumber       int    `json:"column-number"`
		SourceFile         string `json:"source-file"`
		StatusCode         int    `json:"status-code"`
		ScriptSample       string `json:"script-sample"`
	} `json:"csp-report"`
}

// cspViolation handles CSP violation reports.
func (app *application) cspViolation(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate content type (should be application/csp-report or application/json)
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && contentType != "application/csp-report" && contentType != "application/json" {
		app.logger.LogAttrs(r.Context(), slog.LevelWarn, "CSP violation report with unexpected content type",
			slog.String("content_type", contentType))
	}

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
	defer r.Body.Close()

	// Parse the CSP violation report
	var report CSPViolationReport
	err = json.Unmarshal(body, &report)
	if err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse CSP violation report",
			slog.String("error", err.Error()),
			slog.String("body", string(body)))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Log the CSP violation with relevant details
	app.logger.LogAttrs(r.Context(), slog.LevelWarn, "CSP violation detected",
		slog.String("document_uri", report.CSPReport.DocumentURI),
		slog.String("violated_directive", report.CSPReport.ViolatedDirective),
		slog.String("effective_directive", report.CSPReport.EffectiveDirective),
		slog.String("blocked_uri", report.CSPReport.BlockedURI),
		slog.String("source_file", report.CSPReport.SourceFile),
		slog.Int("line_number", report.CSPReport.LineNumber),
		slog.Int("column_number", report.CSPReport.ColumnNumber),
		slog.String("script_sample", report.CSPReport.ScriptSample),
		slog.String("disposition", report.CSPReport.Disposition),
		slog.String("user_agent", r.Header.Get("User-Agent")),
		slog.String("referrer", report.CSPReport.Referrer))

	// Respond with 204 No Content as per CSP specification
	w.WriteHeader(http.StatusNoContent)
}
