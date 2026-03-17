package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/myrjola/petrapp/internal/webauthnhandler"
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
	out, err := app.webAuthnHandler.BeginLogin(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(out) //#nosec G705 -- out is a structured WebAuthn challenge, not a reflection of raw user input.
	if err != nil {
		app.serverError(w, r, err)
		return
	}
}

func (app *application) finishLogin(w http.ResponseWriter, r *http.Request) {
	if err := app.webAuthnHandler.FinishLogin(r); err != nil {
		// Check if the error is due to an unknown credential.
		var unknownCredErr *webauthnhandler.UnknownCredentialError
		if errors.As(err, &unknownCredErr) {
			// Return JSON error response with credential ID for client to signal removal.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			response := map[string]string{
				"error":        "unknown_credential",
				"credentialId": base64.RawURLEncoding.EncodeToString(unknownCredErr.CredentialID),
			}
			if err = json.NewEncoder(w).Encode(response); err != nil {
				app.serverError(w, r, err)
			}
			return
		}
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
