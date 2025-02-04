package main

import (
	"net/http"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	// Define middleware chain functions
	common := func(next http.Handler) http.Handler {
		return app.recoverPanic(app.logRequest(secureHeaders(noSurf(commonContext(timeout(next))))))
	}

	session := func(next http.Handler) http.Handler {
		return common(app.sessionManager.LoadAndSave(app.webAuthnHandler.AuthenticateMiddleware(next)))
	}

	mustSession := func(next http.Handler) http.Handler {
		return session(app.mustAuthenticate(next))
	}

	// File server
	fileServer := http.FileServer(http.Dir("./ui/static/"))
	mux.Handle("/", common(cacheForeverHeaders(fileServer)))

	// Routes
	mux.Handle("GET /{$}", session(http.HandlerFunc(app.home)))

	mux.Handle("GET /preferences", mustSession(http.HandlerFunc(app.preferencesGET)))
	mux.Handle("POST /preferences", mustSession(http.HandlerFunc(app.preferencesPOST)))

	mux.Handle("POST /api/registration/start", session(http.HandlerFunc(app.beginRegistration)))
	mux.Handle("POST /api/registration/finish", session(http.HandlerFunc(app.finishRegistration)))
	mux.Handle("POST /api/login/start", session(http.HandlerFunc(app.beginLogin)))
	mux.Handle("POST /api/login/finish", session(http.HandlerFunc(app.finishLogin)))
	mux.Handle("POST /api/logout", session(http.HandlerFunc(app.logout)))
	mux.Handle("GET /api/healthy", session(http.HandlerFunc(app.healthy)))

	return mux
}
