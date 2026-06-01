package main

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// healthCheckTimeout bounds the readiness probe so a wedged database connection
// can't hang the health endpoint (and, in turn, Fly's machine health checks).
const healthCheckTimeout = 2 * time.Second

// healthy is a readiness probe: it confirms the database is reachable before
// reporting OK. A failed probe returns 503 so orchestration treats the instance
// as not-ready instead of routing traffic to a process that can't serve queries.
func (app *application) healthy(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")
	if err := app.service.HealthCheck(ctx); err != nil {
		app.logger.LogAttrs(ctx, slog.LevelError, "health check failed", slog.Any("error", err))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unavailable"}`))
		return
	}
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// testTimeout sleeps for the sleep_ms query-parameter duration so the timeout
// middleware can be exercised in tests. Returns 404 outside devMode.
func (app *application) testTimeout(w http.ResponseWriter, r *http.Request) {
	if !app.devMode {
		http.NotFound(w, r)
		return
	}

	sleepMsStr := r.URL.Query().Get("sleep_ms")
	if sleepMsStr == "" {
		sleepMsStr = "0"
	}

	sleepMs, err := strconv.Atoi(sleepMsStr)
	if err != nil {
		http.Error(w, "Invalid sleep_ms parameter", http.StatusBadRequest)
		return
	}

	if sleepMs > 0 {
		time.Sleep(time.Duration(sleepMs) * time.Millisecond)
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"completed","slept_ms":` + strconv.Itoa(sleepMs) + `}`))
}
