package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
)

// redactedPlaceholder is substituted for inline-script samples and other
// potentially sensitive scalar fields before logging.
const redactedPlaceholder = "[redacted]"

// scriptSampleFields lists payload keys whose values are inline script
// fragments and must be replaced wholesale before logging.
var scriptSampleFields = map[string]struct{}{ //nolint:gochecknoglobals // immutable lookup table
	"script-sample": {},
	"sample":        {},
}

// urlFields lists payload keys whose values are URLs whose query strings
// may carry credentials or tokens. We keep scheme/host/path so the log
// stays useful for debugging.
var urlFields = map[string]struct{}{ //nolint:gochecknoglobals // immutable lookup table
	"source-file":  {},
	"document-uri": {},
	"blocked-uri":  {},
	"referrer":     {},
	"documentURL":  {},
	"blockedURL":   {},
	"referrerURL":  {},
}

// redactReport returns a copy of the parsed report with potentially sensitive
// fields scrubbed. The original payload is left untouched so the request-
// scoped processing can still use it.
func redactReport(payload any) any {
	switch v := payload.(type) {
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i, item := range v {
			out[i] = redactReportObject(item)
		}
		return out
	case map[string]any:
		return redactReportObject(v)
	default:
		return payload
	}
}

func redactReportObject(obj map[string]any) map[string]any {
	out := make(map[string]any, len(obj))
	for k, val := range obj {
		out[k] = redactReportValue(k, val)
	}
	return out
}

func redactReportValue(key string, val any) any {
	if _, ok := scriptSampleFields[key]; ok {
		if s, isString := val.(string); isString && s != "" {
			return redactedPlaceholder
		}
	}
	if _, ok := urlFields[key]; ok {
		if s, isString := val.(string); isString {
			return stripURLQuery(s)
		}
	}
	switch nested := val.(type) {
	case map[string]any:
		return redactReportObject(nested)
	case []any:
		out := make([]any, len(nested))
		for i, item := range nested {
			out[i] = redactReportValue(key, item)
		}
		return out
	default:
		return val
	}
}

// stripURLQuery returns the URL with its query string and fragment removed.
// Non-URL values pass through unchanged.
func stripURLQuery(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.RawQuery == "" && u.Fragment == "" {
		return raw
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// reportingAPI handles reports sent via the Reporting API.
// See: https://developer.mozilla.org/en-US/docs/Web/API/Reporting_API
func (app *application) reportingAPI(w http.ResponseWriter, r *http.Request) {
	// Validate content type (should be application/csp-report, application/json, or application/reports+json)
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && contentType != "application/csp-report" &&
		contentType != "application/json" && contentType != "application/reports+json" {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Report with unexpected content type",
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

	// Log a redacted copy so script samples and URL query strings (which can
	// carry tokens) don't leak into application logs.
	app.logger.LogAttrs(r.Context(), slog.LevelWarn, "Report received via Reporting API",
		slog.Any("payload", redactReport(payload)),
		slog.String("user_agent", r.Header.Get("User-Agent")))

	// Respond with 204 No Content as per Reporting API specification
	w.WriteHeader(http.StatusNoContent)
}
