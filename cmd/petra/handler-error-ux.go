package main

import (
	"fmt"
	"io"
	"net/http"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

const devErrorUXPath = "/dev/error-ux"

// devErrorUXPanelAnchor is the id of the demo's anchored panel. An anchored
// flash redirects to devErrorUXPath+"#"+devErrorUXPanelAnchor so the browser
// scrolls the panel into view (mirrors the preferences panel-flash pattern).
const devErrorUXPanelAnchor = "demo-panel"

type errorUXTemplateData struct {
	BaseTemplateData

	Flash          BannerData // page-top flash (anchor "")
	PanelFlash     BannerData // flash anchored to the demo panel
	BannerVariants []BannerData
}

// devErrorUXGET renders the live catalog of the four error-surfacing classes
// documented in cmd/petra/README.md under "Error Handling".
// Wired in routes.go only when app.devMode is true; returns 404 otherwise.
func (app *application) devErrorUXGET(w http.ResponseWriter, r *http.Request) {
	if !app.devMode {
		http.NotFound(w, r)
		return
	}

	base := newBaseTemplateData(r)
	popped := app.popFlash(r.Context())
	// Route the popped flash either to the page top or the anchored panel,
	// matching the anchor the trigger handler stored it under.
	var pageTopFlash, panelFlash BannerData
	if popped.Message != "" {
		bd := BannerData{
			Variant: popped.Variant,
			Message: popped.Message,
			Live:    true,
			Nonce:   base.Nonce,
		}
		if popped.Anchor == devErrorUXPanelAnchor {
			panelFlash = bd
		} else {
			pageTopFlash = bd
		}
	}
	data := errorUXTemplateData{
		BaseTemplateData: base,
		Flash:            pageTopFlash,
		PanelFlash:       panelFlash,
		BannerVariants: []BannerData{
			{
				Variant: BannerVariantError,
				Message: "Something went wrong. Please try again.",
				Live:    false, // static reference — must not steal focus
				Nonce:   base.Nonce,
			},
			{
				Variant: BannerVariantSuccess,
				Message: "Your changes have been saved.",
				Live:    false,
				Nonce:   base.Nonce,
			},
			{
				Variant: BannerVariantInfo,
				Message: "Heads up — this is informational.",
				Live:    false,
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

	// Confirmation flashes (success / info) and the panel-anchored error do
	// not represent a failed service call, so they flash + redirect directly
	// rather than routing through userError (which is for service errors only).
	switch r.PathValue("kind") {
	case "success":
		app.putFlashSuccess(r.Context(), "Your changes have been saved.", "")
		redirect(w, r, devErrorUXPath)
		return
	case "info":
		app.putFlash(r.Context(), BannerVariantInfo, "Heads up — this is informational.", "")
		redirect(w, r, devErrorUXPath)
		return
	case "anchored":
		app.putFlashErrorWithAnchor(r.Context(),
			"This panel needs your attention before you can continue.", devErrorUXPanelAnchor)
		redirect(w, r, devErrorUXPath+"#"+devErrorUXPanelAnchor)
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
