package main

import (
	"github.com/justinas/alice"
	"net/http"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()
	common := alice.New(app.recoverPanic, app.logRequest, secureHeaders, noSurf, commonContext, timeout)
	session := alice.New(common.Then, app.sessionManager.LoadAndSave, app.webAuthnHandler.AuthenticateMiddleware)
	mustSession := alice.New(session.Then, app.mustAuthenticate)

	fileServer := http.FileServer(http.Dir("./ui/static/"))
	mux.Handle("/", common.Then(cacheForeverHeaders(fileServer)))

	mux.Handle("GET /{$}", session.ThenFunc(app.home))

	mux.Handle("GET /preferences", mustSession.ThenFunc(app.preferences))

	mux.Handle("POST /api/registration/start", session.ThenFunc(app.beginRegistration))
	mux.Handle("POST /api/registration/finish", session.ThenFunc(app.finishRegistration))
	mux.Handle("POST /api/login/start", session.ThenFunc(app.beginLogin))
	mux.Handle("POST /api/login/finish", session.ThenFunc(app.finishLogin))
	mux.Handle("POST /api/logout", session.ThenFunc(app.logout))

	mux.Handle("GET /api/healthy", session.ThenFunc(app.healthy))

	return mux
}
