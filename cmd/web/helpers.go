package main

import (
	"log/slog"
	"net/http"
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
