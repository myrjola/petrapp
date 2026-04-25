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

// redirectAfterPOST sends the client to target after a successful POST.
// action is "" (default: replace) or "pop-or-replace" (client traverses to an
// existing matching history entry when present, otherwise replace).
//
// JS-enhanced submits (X-Requested-With: stacknav) get HTTP 200 with X-Location
// and optional X-History-Action headers and an empty body. Non-JS submits get a
// standard 303 See Other redirect.
func (app *application) redirectAfterPOST(w http.ResponseWriter, r *http.Request, target, action string) {
	if r.Header.Get("X-Requested-With") == "stacknav" {
		w.Header().Set("X-Location", target)
		if action != "" {
			w.Header().Set("X-History-Action", action)
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
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

// parseExerciseIDParam parses the "exerciseID" path parameter from the request URL.
// Returns the parsed exercise ID and true if successful, or zero and false if parsing fails.
// On failure, sends HTTP 404 response automatically.
func (app *application) parseExerciseIDParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	exerciseIDStr := r.PathValue("exerciseID")
	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		app.notFound(w, r)
		return 0, false
	}
	return exerciseID, true
}
