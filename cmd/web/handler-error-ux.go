package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

const devErrorUXPath = "/dev/error-ux"

type errorUXTemplateData struct {
	BaseTemplateData

	Flash          BannerData
	BannerVariants []BannerData
}

// devErrorUXGET renders the live catalog of the four error-surfacing classes
// documented in docs/superpowers/specs/2026-05-18-error-ux-conventions-design.md.
// Wired in routes.go only when app.devMode is true; returns 404 otherwise.
func (app *application) devErrorUXGET(w http.ResponseWriter, r *http.Request) {
	if !app.devMode {
		http.NotFound(w, r)
		return
	}

	base := newBaseTemplateData(r)
	popped := app.popFlash(r.Context())
	flash := BannerData{
		Variant: popped.Variant,
		Message: popped.Message,
		Nonce:   base.Nonce,
	}
	data := errorUXTemplateData{
		BaseTemplateData: base,
		Flash:            flash,
		BannerVariants: []BannerData{
			{
				Variant: BannerVariantError,
				Message: "Something went wrong. Please try again.",
				Nonce:   base.Nonce,
			},
			{
				Variant: BannerVariantSuccess,
				Message: "Your changes have been saved.",
				Nonce:   base.Nonce,
			},
			{
				Variant: BannerVariantInfo,
				Message: "Heads up — this is informational.",
				Nonce:   base.Nonce,
			},
		},
	}
	app.render(w, r, http.StatusOK, "error-ux", data)
}

// devErrorUXTriggerPOST dispatches on the {kind} path parameter and routes
// the resulting error through app.userError so the live banner appears on
// the next GET of /dev/error-ux. Unknown kinds return 404 — there is no
// graceful UX path for an unrecognised demo trigger.
func (app *application) devErrorUXTriggerPOST(w http.ResponseWriter, r *http.Request) {
	if !app.devMode {
		http.NotFound(w, r)
		return
	}

	var err error
	switch r.PathValue("kind") {
	case "validation":
		err = domain.ValidationError{Message: "Name must be 1–50 characters."}
	case "business":
		err = domain.ValidationError{
			Message: "This day has no planned workout. Schedule one from the home page first.",
		}
	case "system":
		err = fmt.Errorf("simulated system fault: %w", io.ErrUnexpectedEOF)
	default:
		http.NotFound(w, r)
		return
	}
	app.userError(w, r, err, devErrorUXPath)
}

// devErrorUXServerErrorGET exists to demonstrate the rare class E path:
// app.serverError renders the full-page 500 directly because no safe URL
// exists. Hit via a regular anchor on /dev/error-ux.
func (app *application) devErrorUXServerErrorGET(w http.ResponseWriter, r *http.Request) {
	if !app.devMode {
		http.NotFound(w, r)
		return
	}
	app.serverError(w, r, fmt.Errorf("simulated full-page server error: %w", io.ErrUnexpectedEOF))
}
