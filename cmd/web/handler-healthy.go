package main

import (
	"net/http"
	"strconv"
	"time"
)

// healthy responds with a JSON object indicating that the server is healthy.
func (app *application) healthy(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// testTimeout is a handler for testing timeout functionality.
// It accepts a query parameter sleep_ms to control how long it sleeps.
func (app *application) testTimeout(w http.ResponseWriter, r *http.Request) {
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
	_, _ = w.Write([]byte(`{"status":"completed","slept_ms":` + sleepMsStr + `}`))
}
