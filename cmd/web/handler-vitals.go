package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// vitalsReport is the JSON payload posted by the client web-vitals reporter
// on pagehide. One report carries every metric observed for a page lifetime.
type vitalsReport struct {
	Path           string         `json:"path"`
	NavigationType string         `json:"navigationType"`
	Metrics        []vitalsMetric `json:"metrics"`
}

type vitalsMetric struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Rating string  `json:"rating"`
	Target string  `json:"target"`
}

// vitalsPOST receives a batched web-vitals report and emits one INFO log line
// per metric. The endpoint is unauthenticated to allow sendBeacon, which
// cannot carry custom headers.
func (app *application) vitalsPOST(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	const maxBodySize = 4 * 1024
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to read vitals body",
			slog.String("error", err.Error()))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	var report vitalsReport
	if err = json.Unmarshal(body, &report); err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "Failed to parse vitals report",
			slog.String("error", err.Error()))
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	userAgent := r.Header.Get("User-Agent")
	for _, m := range report.Metrics {
		app.logger.LogAttrs(r.Context(), slog.LevelInfo, "web vital",
			slog.String("metric", m.Name),
			slog.Float64("value", m.Value),
			slog.String("rating", m.Rating),
			slog.String("target", m.Target),
			slog.String("path", report.Path),
			slog.String("nav_type", report.NavigationType),
			slog.String("user_agent", userAgent))
	}

	w.WriteHeader(http.StatusNoContent)
}
