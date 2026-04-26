package main

import (
	"context"
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

// defaultMaxFormSize is a reasonable maximum size for form data in bytes.
//
// You can use it as follows before calling r.ParseForm(): r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize).
const defaultMaxFormSize = 1024

// largeMaxFormSize is a larger maximum size for form data when there's more content to be expected.
const largeMaxFormSize = 1024 * 10

// redirect detects if the request is originating from a fetch API call or a top-level navigation and points the user
// to the correct URL.
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("X-Requested-With") == "stacknav" {
		w.Header().Set("X-Location", path)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, path, http.StatusSeeOther)
}

const flashErrorKey = "flash_error"

// putFlashError stores a flash error message in the session to be displayed on the next page load.
func (app *application) putFlashError(ctx context.Context, message string) {
	app.sessionManager.Put(ctx, flashErrorKey, message)
}

// popFlashError retrieves and removes the flash error message from the session.
func (app *application) popFlashError(ctx context.Context) string {
	return app.sessionManager.PopString(ctx, flashErrorKey)
}

// parseDateParam parses the "date" path parameter from the request URL.
// Returns the parsed date and true if successful, or zero time and false if parsing fails.
// On failure, sends HTTP 404 response automatically.
func (app *application) parseDateParam(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		app.notFound(w, r)
		return time.Time{}, false
	}
	return date, true
}

// parseWorkoutExerciseIDParam parses the "workoutExerciseID" path parameter from
// the request URL. Returns the parsed ID and true on success, or zero and false
// on failure (sending HTTP 404 automatically).
func (app *application) parseWorkoutExerciseIDParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	idStr := r.PathValue("workoutExerciseID")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		app.notFound(w, r)
		return 0, false
	}
	return id, true
}
