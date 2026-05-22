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
