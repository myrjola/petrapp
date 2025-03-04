package main

import (
	"github.com/myrjola/petrapp/internal/errors"
	"log/slog"
	"net/http"
)

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		method = r.Method
		uri    = r.URL.RequestURI()
	)

	app.logger.LogAttrs(r.Context(), slog.LevelError, "server error",
		slog.String("method", method), slog.String("uri", uri), errors.SlogError(err))
	app.render(w, r, http.StatusInternalServerError, "error", nil)
}
