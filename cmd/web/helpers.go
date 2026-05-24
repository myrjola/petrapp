package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.LogAttrs(r.Context(), slog.LevelError, "server error", slog.Any("error", err))
	app.render(w, r, http.StatusInternalServerError, "error", errorTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		From:             "",
	})
}

// userError surfaces a failure of an in-flight user action through the
// flash + banner flow and redirects the client to safeURL. Use it instead
// of serverError when there is a meaningful page to land the user on.
//
// Routing:
//   - domain.ValidationError → flash with ve.Message (safe to display).
//   - any other error → log at ERROR and flash a generic message.
//
// safeURL must point at a GET handler known to render successfully AND
// that pops + renders the flash. Do NOT pass an action endpoint
// (e.g. ".../complete"), and do not default to r.Referer() — it is
// unreliable on direct POSTs.
func (app *application) userError(
	w http.ResponseWriter, r *http.Request, err error, safeURL string,
) {
	var ve domain.ValidationError
	var msg string
	if errors.As(err, &ve) {
		msg = ve.Message
	} else {
		app.logger.LogAttrs(r.Context(), slog.LevelError,
			"user-facing server error", slog.Any("error", err))
		msg = "Couldn't complete that action. Please try again."
	}
	app.putFlashError(r.Context(), msg)
	redirect(w, r, safeURL)
}

func (app *application) notFound(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusNotFound, "not-found", newBaseTemplateData(r))
}

// defaultMaxFormSize is a reasonable maximum size for form data in bytes.
//
// You can use it as follows before calling r.ParseForm(): r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize).
const defaultMaxFormSize = 1024

// largeMaxFormSize is a larger maximum size for form data when there's more
// content to be expected. Sized to accommodate the schema's 20KB exercise
// description cap (internal/sqlite/schema.sql) plus headroom for other form
// fields and form encoding overhead. Schema check stays as defense-in-depth.
const largeMaxFormSize = 1024 * 32

// redirect detects if the request is originating from a fetch API call or a top-level navigation and points the user
// to the correct URL.
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("X-Requested-With") == "stacknav" {
		w.Header().Set("X-Location", path)
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, path, http.StatusSeeOther) //nolint:gosec // G710: path is handler-chosen, never user input.
}

// redirectReplace works like redirect, but signals to the stack navigator
// that the current history entry should be replaced. Use this for form
// pages whose existence should be erased on submit (e.g. /add-exercise).
// Non-stacknav callers fall through to a plain 303, identical to redirect.
func redirectReplace(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("X-Requested-With") == "stacknav" {
		w.Header().Set("X-Location", path)
		w.Header().Set("X-Replace-Url", "true")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, path, http.StatusSeeOther) //nolint:gosec // G710: path is handler-chosen, never user input.
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

// parseForm caps the request body at maxBytes and parses the form. On failure
// it writes a 500 via serverError and returns false; the caller must return
// immediately when it returns false.
func (app *application) parseForm(w http.ResponseWriter, r *http.Request, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return false
	}
	return true
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
