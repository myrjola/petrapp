package main

import (
	"net/http"
)

func (app *application) beginRegistration(w http.ResponseWriter, r *http.Request) {
	var (
		err error
		out []byte
	)
	if out, err = app.webAuthnHandler.BeginRegistration(r.Context()); err != nil {
		app.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(out); err != nil {
		app.serverError(w, r, err)
		return
	}
}

func (app *application) finishRegistration(w http.ResponseWriter, r *http.Request) {
	if err := app.webAuthnHandler.FinishRegistration(r); err != nil {
		app.serverError(w, r, err)
		return
	}
}

func (app *application) beginLogin(w http.ResponseWriter, r *http.Request) {
	out, err := app.webAuthnHandler.BeginLogin(w, r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(out)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
}

func (app *application) finishLogin(w http.ResponseWriter, r *http.Request) {
	if err := app.webAuthnHandler.FinishLogin(r); err != nil {
		app.serverError(w, r, err)
		return
	}
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	if err := app.webAuthnHandler.Logout(r.Context()); err != nil {
		app.serverError(w, r, err)
		return
	}
	redirect(w, r, "/")
}
