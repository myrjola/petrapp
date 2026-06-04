package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

// stackNavHeaderValue is the X-Requested-With value the JS shim
// (ui/static/main.js) sends on form POSTs it has intercepted. Server-side
// helpers branch on it to negotiate the stack-navigator wire protocol.
const stackNavHeaderValue = "stacknav"

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.LogAttrs(r.Context(), slog.LevelError, "server error", slog.Any("error", err))

	if r.Header.Get("X-Requested-With") == stackNavHeaderValue {
		// Drive the shim's "200 + X-Location ⇒ navigate" path so the user
		// sees the error page instead of a silent reload on the form page.
		// Referer is a UX hint only — sanitised same-origin path becomes a
		// "← Back" link on the error page, cross-origin / missing is fine.
		target := "/error"
		if from := r.Referer(); from != "" {
			if u, parseErr := url.Parse(from); parseErr == nil && u.Host == r.Host {
				target = "/error?from=" + url.QueryEscape(u.Path)
			}
		}
		w.Header().Set("X-Location", target)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Non-shim path: render the error page inline with a 500. This is the
	// path for GET handlers, curl, and no-JS browsers.
	app.render(w, r, http.StatusInternalServerError, "error", errorTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		From:             "",
	})
}

// userError surfaces a failure of an in-flight user action.
//
// Routing:
//   - domain.ValidationError → flash with ve.Message and redirect to safeURL.
//     The safe URL's GET handler pops the flash and renders the banner.
//   - any other error → delegate to serverError. On the shim path that
//     navigates the user to /error (catastrophic-failure UX); on the
//     non-shim path it renders error.gohtml with a 500.
//
// safeURL is only used on the validation branch. It must point at a GET
// handler known to render successfully AND that pops + renders the flash.
// See cmd/web/CLAUDE.md "userError semantics" for the rationale and the
// list of currently-supported safe URLs.
func (app *application) userError(
	w http.ResponseWriter, r *http.Request, err error, safeURL string,
) {
	var ve domain.ValidationError
	if errors.As(err, &ve) {
		app.putFlashError(r.Context(), ve.Message)
		redirect(w, r, safeURL)
		return
	}
	app.serverError(w, r, err)
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
// description cap (internal/repository/schema.sql) plus headroom for other form
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

const flashKey = "flash"

// flashEntry is the session-backed flash payload. Variant is one of
// BannerVariantError, BannerVariantSuccess, BannerVariantInfo. Anchor is the
// id of the panel that should render the banner; empty Anchor means the page-
// top slot.
type flashEntry struct {
	Variant string
	Message string
	Anchor  string
}

// putFlash stores a typed flash entry in the session for the next page load.
func (app *application) putFlash(ctx context.Context, variant, message, anchor string) {
	app.sessionManager.Put(ctx, flashKey, flashEntry{
		Variant: variant,
		Message: message,
		Anchor:  anchor,
	})
}

// putFlashError is the legacy shim for the page-top error banner. Prefer
// putFlashErrorWithAnchor or putFlashSuccess for new code.
func (app *application) putFlashError(ctx context.Context, message string) {
	app.putFlash(ctx, BannerVariantError, message, "")
}

// putFlashErrorWithAnchor sets an error flash bound to a specific panel id.
func (app *application) putFlashErrorWithAnchor(ctx context.Context, message, anchor string) {
	app.putFlash(ctx, BannerVariantError, message, anchor)
}

// putFlashSuccess sets a success flash bound to a specific panel id.
// Pass an empty anchor for the page-top slot.
func (app *application) putFlashSuccess(ctx context.Context, message, anchor string) {
	app.putFlash(ctx, BannerVariantSuccess, message, anchor)
}

// popFlash retrieves and removes the flash entry from the session. Returns a
// zero-value flashEntry when nothing is stored.
func (app *application) popFlash(ctx context.Context) flashEntry {
	raw := app.sessionManager.Pop(ctx, flashKey)
	if raw == nil {
		return flashEntry{}
	}
	entry, ok := raw.(flashEntry)
	if !ok {
		return flashEntry{}
	}
	return entry
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

// parsePositionParam parses the "position" path parameter from the request
// URL. Returns the parsed position and true on success, or zero and false on
// failure (sending HTTP 404 automatically). Negative values are rejected.
func (app *application) parsePositionParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	posStr := r.PathValue("position")
	pos, err := strconv.Atoi(posStr)
	if err != nil || pos < 0 {
		app.notFound(w, r)
		return 0, false
	}
	return pos, true
}
