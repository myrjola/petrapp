package main

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.LogAttrs(r.Context(), slog.LevelError, "server error", slog.Any("error", err))
	app.render(w, r, http.StatusInternalServerError, "error", nil)
}

func (app *application) notFound(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusNotFound, "not-found", newBaseTemplateData(r))
}

// redirect detects if the request is originating from a fetch API call or a top-level navigation and points the user
// to the correct URL.
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("Sec-Fetch-Dest") == "empty" {
		w.Header().Set("Content-Location", path)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, path, http.StatusSeeOther)
}

// parseDateParam parses the "date" path parameter from the request URL.
// Returns the parsed date and true if successful, or zero time and false if parsing fails.
// On failure, sends HTTP 404 response automatically.
func (app *application) parseDateParam(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return time.Time{}, false
	}
	return date, true
}

// parseExerciseIDParam parses the "exerciseID" path parameter from the request URL.
// Returns the parsed exercise ID and true if successful, or zero and false if parsing fails.
// On failure, sends HTTP 404 response automatically.
func (app *application) parseExerciseIDParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	exerciseIDStr := r.PathValue("exerciseID")
	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return exerciseID, true
}
