package main

import (
	"log/slog"
	"net/http"
)

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.LogAttrs(r.Context(), slog.LevelError, "server error", slog.Any("error", err))
	app.render(w, r, http.StatusInternalServerError, "error", nil)
}
