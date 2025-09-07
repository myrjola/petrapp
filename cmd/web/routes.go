package main

import (
	"fmt"
	"net/http"
)

func (app *application) routes() (*http.ServeMux, error) {
	mux := http.NewServeMux()

	var (
		withoutMaintenanceMode = func(next http.Handler) http.Handler {
			return app.logAndTraceRequest(secureHeaders(app.crossOriginProtection(
				commonContext(app.timeout(next)))))
		}
		shared = func(next http.Handler) http.Handler {
			return withoutMaintenanceMode(app.maintenanceMode(next))
		}
		noAuth = func(next http.Handler) http.Handler {
			return app.recoverPanic(withoutMaintenanceMode(next))
		}
		session = func(next http.Handler) http.Handler {
			return app.recoverPanic(noCache(app.sessionManager.LoadAndSave(
				app.webAuthnHandler.AuthenticateMiddleware(shared(next)))))
		}
		mustSession = func(next http.Handler) http.Handler {
			return session(app.mustAuthenticate(next))
		}
		mustAdmin = func(next http.Handler) http.Handler {
			return mustSession(app.mustAdmin(next))
		}
	)

	mux.Handle("GET /workouts/{date}", mustSession(http.HandlerFunc(app.workoutGET)))
	mux.Handle("POST /workouts/{date}/start", mustSession(http.HandlerFunc(app.workoutStartPOST)))
	mux.Handle("POST /workouts/{date}/complete", mustSession(http.HandlerFunc(app.workoutCompletePOST)))
	mux.Handle("GET /workouts/{date}/complete", mustSession(http.HandlerFunc(app.workoutCompletionGET)))

	mux.Handle("GET /workouts/{date}/exercises/{exerciseID}", mustSession(http.HandlerFunc(app.exerciseSetGET)))
	mux.Handle("POST /workouts/{date}/exercises/{exerciseID}/sets/{setIndex}/update",
		mustSession(http.HandlerFunc(app.exerciseSetUpdatePOST)))
	mux.Handle("POST /workouts/{date}/exercises/{exerciseID}/warmup/complete",
		mustSession(http.HandlerFunc(app.exerciseSetWarmupCompletePOST)))
	mux.Handle("GET /workouts/{date}/exercises/{exerciseID}/info", mustSession(http.HandlerFunc(app.exerciseInfoGET)))
	mux.Handle("GET /workouts/{date}/exercises/{exerciseID}/progress-chart",
		mustSession(http.HandlerFunc(app.exerciseProgressChart)))
	mux.Handle("GET /workouts/{date}/exercises/{exerciseID}/swap",
		mustSession(http.HandlerFunc(app.workoutSwapExerciseGET)))
	mux.Handle("POST /workouts/{date}/exercises/{exerciseID}/swap",
		mustSession(http.HandlerFunc(app.workoutSwapExercisePOST)))
	mux.Handle("GET /workouts/{date}/add-exercise",
		mustSession(http.HandlerFunc(app.workoutAddExerciseGET)))
	mux.Handle("POST /workouts/{date}/add-exercise",
		mustSession(http.HandlerFunc(app.workoutAddExercisePOST)))
	mux.Handle("POST /workouts/{date}/feedback/{difficulty}", mustSession(http.HandlerFunc(app.workoutFeedbackPOST)))

	mux.Handle("GET /preferences", mustSession(http.HandlerFunc(app.preferencesGET)))
	mux.Handle("POST /preferences", mustSession(http.HandlerFunc(app.preferencesPOST)))
	mux.Handle("GET /preferences/export-data", mustSession(http.HandlerFunc(app.exportUserDataGET)))
	mux.Handle("POST /preferences/delete-user", mustSession(http.HandlerFunc(app.deleteUserPOST)))

	mux.Handle("POST /api/registration/start", session(http.HandlerFunc(app.beginRegistration)))
	mux.Handle("POST /api/registration/finish", session(http.HandlerFunc(app.finishRegistration)))
	mux.Handle("POST /api/login/start", session(http.HandlerFunc(app.beginLogin)))
	mux.Handle("POST /api/login/finish", session(http.HandlerFunc(app.finishLogin)))
	mux.Handle("POST /api/logout", session(http.HandlerFunc(app.logout)))
	mux.Handle("GET /api/healthy", session(http.HandlerFunc(app.healthy)))
	mux.Handle("GET /api/test/timeout", noAuth(http.HandlerFunc(app.testTimeout)))

	mux.Handle("GET /admin/exercises", mustAdmin(http.HandlerFunc(app.adminExercisesGET)))
	mux.Handle("GET /admin/exercises/{id}", mustAdmin(http.HandlerFunc(app.adminExerciseEditGET)))
	mux.Handle("POST /admin/exercises/{id}", mustAdmin(http.HandlerFunc(app.adminExerciseUpdatePOST)))
	mux.Handle("POST /admin/exercises/generate", mustAdmin(http.HandlerFunc(app.adminExerciseGeneratePOST)))

	mux.Handle("GET /admin/feature-flags", session(http.HandlerFunc(app.adminFeatureFlagsGET)))
	mux.Handle("POST /admin/feature-flags/{name}/toggle", mustAdmin(http.HandlerFunc(app.adminFeatureFlagTogglePOST)))

	// Privacy page
	mux.Handle("GET /privacy", session(http.HandlerFunc(app.privacy)))

	// Home route (most specific)
	mux.Handle("GET /{$}", session(http.HandlerFunc(app.home)))

	// File server with custom 404 handling
	fileServerHandler, err := app.fileServerHandler()
	if err != nil {
		return nil, fmt.Errorf("fileServerHandler: %w", err)
	}
	mux.Handle("/", fileServerHandler)

	return mux, nil
}
