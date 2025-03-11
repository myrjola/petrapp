package main

import (
	"net/http"
)

func (app *application) routes() *http.ServeMux {
	mux := http.NewServeMux()

	// Define middleware chain functions
	common := func(next http.Handler) http.Handler {
		return app.recoverPanic(app.logRequest(secureHeaders(app.noSurf(commonContext(timeout(next))))))
	}

	session := func(next http.Handler) http.Handler {
		return common(app.sessionManager.LoadAndSave(app.webAuthnHandler.AuthenticateMiddleware(next)))
	}

	mustSession := func(next http.Handler) http.Handler {
		return session(app.mustAuthenticate(next))
	}

	mustAdmin := func(next http.Handler) http.Handler {
		return mustSession(app.mustAdmin(next))
	}

	// File server
	fileServer := http.FileServer(http.Dir("./ui/static/"))
	mux.Handle("/", common(cacheForeverHeaders(fileServer)))

	// Routes
	mux.Handle("GET /{$}", session(http.HandlerFunc(app.home)))
	mux.Handle("GET /workouts/{date}", mustSession(http.HandlerFunc(app.workoutGET)))
	mux.Handle("POST /workouts/{date}/start", mustSession(http.HandlerFunc(app.workoutStartPOST)))
	mux.Handle("POST /workouts/{date}/complete", mustSession(http.HandlerFunc(app.workoutCompletePOST)))
	mux.Handle("GET /workouts/{date}/complete", mustSession(http.HandlerFunc(app.workoutCompletionGET)))
	mux.Handle("GET /workouts/{date}/exercises/{exerciseID}", mustSession(http.HandlerFunc(app.exerciseSetGET)))
	mux.Handle("POST /workouts/{date}/exercises/{exerciseID}/sets/{setIndex}/update",
		mustSession(http.HandlerFunc(app.exerciseSetUpdatePOST)))
	mux.Handle("POST /workouts/{date}/feedback/{difficulty}", mustSession(http.HandlerFunc(app.workoutFeedbackPOST)))

	mux.Handle("GET /preferences", mustSession(http.HandlerFunc(app.preferencesGET)))
	mux.Handle("POST /preferences", mustSession(http.HandlerFunc(app.preferencesPOST)))

	mux.Handle("POST /api/registration/start", session(http.HandlerFunc(app.beginRegistration)))
	mux.Handle("POST /api/registration/finish", session(http.HandlerFunc(app.finishRegistration)))
	mux.Handle("POST /api/login/start", session(http.HandlerFunc(app.beginLogin)))
	mux.Handle("POST /api/login/finish", session(http.HandlerFunc(app.finishLogin)))
	mux.Handle("POST /api/logout", session(http.HandlerFunc(app.logout)))
	mux.Handle("GET /api/healthy", session(http.HandlerFunc(app.healthy)))

	mux.Handle("GET /admin/exercises", mustAdmin(http.HandlerFunc(app.adminExercisesGET)))
	mux.Handle("GET /admin/exercises/{id}", mustAdmin(http.HandlerFunc(app.adminExerciseEditGET)))
	mux.Handle("POST /admin/exercises/{id}", mustAdmin(http.HandlerFunc(app.adminExerciseUpdatePOST)))
	mux.Handle("POST /admin/exercises/create", mustAdmin(http.HandlerFunc(app.adminExerciseCreatePOST)))

	return mux
}
