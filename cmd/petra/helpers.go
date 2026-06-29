package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
// Routing (first match wins):
//   - domain.FieldErrors → stash the per-field messages AND the submitted form
//     values (r.PostForm) for the next GET of safeURL, then redirect. The safe
//     URL's GET handler pops them, re-renders each input with its own error and
//     the user's submitted value, and shows an error summary.
//   - domain.ValidationError → flash with ve.Message and redirect to safeURL.
//     The safe URL's GET handler pops the flash and renders the banner.
//   - any other error → delegate to serverError. On the shim path that
//     navigates the user to /error (catastrophic-failure UX); on the
//     non-shim path it renders error.gohtml with a 500.
//
// safeURL is only used on the two validation branches. It must point at a GET
// handler known to render successfully AND that pops + renders the flash /
// form-error payload. See cmd/petra/CLAUDE.md "userError semantics" for the
// rationale and the list of currently-supported safe URLs.
func (app *application) userError(
	w http.ResponseWriter, r *http.Request, err error, safeURL string,
) {
	var fe *domain.FieldErrors
	if errors.As(err, &fe) {
		app.putFormError(r.Context(), formErrorPayload{
			Fields:      fe.Fields,
			FormMessage: strings.Join(fe.Form, " "),
			Values:      r.PostForm, // populated by parseForm before the service call
		})
		redirect(w, r, safeURL)
		return
	}
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

	http.Redirect(w, r, path, http.StatusSeeOther)
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

	http.Redirect(w, r, path, http.StatusSeeOther)
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

const formErrorKey = "formError"

// formErrorPayload carries a multi-field validation failure across the
// Post-Redirect-Get bounce: the per-field messages, any form-level message,
// and the values the user submitted — so the re-rendered form shows their
// edits, not the database state. Session-backed and gob-registered (see
// main.go); url.Values is gob-encodable.
type formErrorPayload struct {
	Fields      map[string]string
	FormMessage string
	Values      url.Values
}

// putFormError stashes a form-error payload for the next GET of the form page.
func (app *application) putFormError(ctx context.Context, p formErrorPayload) {
	app.sessionManager.Put(ctx, formErrorKey, p)
}

// popFormError retrieves and removes the form-error payload, returning a
// zero-value payload (has() == false) when nothing is stored.
func (app *application) popFormError(ctx context.Context) formErrorPayload {
	raw := app.sessionManager.Pop(ctx, formErrorKey)
	p, ok := raw.(formErrorPayload)
	if !ok {
		return formErrorPayload{}
	}
	return p
}

// has reports whether a payload is present — i.e. this GET is a validation
// bounce. When false the form handler renders pure database state.
func (p formErrorPayload) has() bool { return p.Values != nil }

// value returns the submitted value for a single-value field when a payload is
// present (so the user sees their edit), else the database-derived fallback.
func (p formErrorPayload) value(name, fallback string) string {
	if p.Values == nil {
		return fallback
	}
	if vs, ok := p.Values[name]; ok {
		return strings.Join(vs, "")
	}
	return fallback
}

// multi returns the submitted values for a multi-value field (e.g. a
// <select multiple>) when a payload is present, else the fallback.
func (p formErrorPayload) multi(name string, fallback []string) []string {
	if p.Values == nil {
		return fallback
	}
	if vs, ok := p.Values[name]; ok {
		return vs
	}
	return fallback
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
