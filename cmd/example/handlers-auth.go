package main

import "net/http"

func (app *application) beginRegistration(w http.ResponseWriter, r *http.Request) {
	out, err := app.auth.BeginRegistration(r.Context())
	if err != nil {
		app.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (app *application) finishRegistration(w http.ResponseWriter, r *http.Request) {
	if err := app.auth.FinishRegistration(r); err != nil {
		app.serverError(w, err)
	}
}

func (app *application) beginLogin(w http.ResponseWriter, r *http.Request) {
	out, err := app.auth.BeginLogin(r)
	if err != nil {
		app.serverError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out) //#nosec G705 -- structured WebAuthn challenge, not raw user input.
}

func (app *application) finishLogin(w http.ResponseWriter, r *http.Request) {
	if err := app.auth.FinishLogin(r); err != nil {
		app.serverError(w, err)
	}
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	if err := app.auth.Logout(r.Context()); err != nil {
		app.serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleAccount is the one gated page, proving AuthenticateMiddleware composes.
func (app *application) handleAccount(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("account (authenticated)"))
}
