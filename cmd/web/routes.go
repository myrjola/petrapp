package main

import (
	"fmt"
	"net/http"
)

func (app *application) routes() (*http.ServeMux, error) {
	mux := http.NewServeMux()

	mux.Handle("GET /workouts/{date}", app.mustSessionStack(http.HandlerFunc(app.workoutGET)))
	mux.Handle("POST /workouts/{date}/start", app.mustSessionStack(http.HandlerFunc(app.workoutStartPOST)))
	mux.Handle("POST /workouts/{date}/complete", app.mustSessionStack(http.HandlerFunc(app.workoutCompletePOST)))
	mux.Handle("GET /workouts/{date}/complete", app.mustSessionStack(http.HandlerFunc(app.workoutCompletionGET)))

	mux.Handle("GET /workouts/{date}/exercises/{workoutExerciseID}",
		app.mustSessionStack(http.HandlerFunc(app.exerciseSetGET)))
	mux.Handle("POST /workouts/{date}/exercises/{workoutExerciseID}/sets/{setIndex}/update",
		app.mustSessionStack(http.HandlerFunc(app.exerciseSetUpdatePOST)))
	mux.Handle("POST /workouts/{date}/exercises/{workoutExerciseID}/warmup/complete",
		app.mustSessionStack(http.HandlerFunc(app.exerciseSetWarmupCompletePOST)))
	mux.Handle("GET /workouts/{date}/exercises/{workoutExerciseID}/info",
		app.mustSessionStack(http.HandlerFunc(app.exerciseInfoGET)))
	mux.Handle("GET /workouts/{date}/exercises/{workoutExerciseID}/swap",
		app.mustSessionStack(http.HandlerFunc(app.workoutSwapExerciseGET)))
	mux.Handle("POST /workouts/{date}/exercises/{workoutExerciseID}/swap",
		app.mustSessionStack(http.HandlerFunc(app.workoutSwapExercisePOST)))
	mux.Handle("GET /workouts/{date}/add-exercise",
		app.mustSessionStack(http.HandlerFunc(app.workoutAddExerciseGET)))
	mux.Handle("POST /workouts/{date}/add-exercise",
		app.mustSessionStack(http.HandlerFunc(app.workoutAddExercisePOST)))
	mux.Handle("POST /workouts/{date}/feedback/{difficulty}",
		app.mustSessionStack(http.HandlerFunc(app.workoutFeedbackPOST)))

	mux.Handle("GET /schedule", app.mustSessionStack(http.HandlerFunc(app.scheduleGET)))
	mux.Handle("POST /schedule", app.mustSessionStack(http.HandlerFunc(app.schedulePOST)))

	mux.Handle("GET /preferences", app.mustSessionStack(http.HandlerFunc(app.preferencesGET)))
	mux.Handle("POST /preferences", app.mustSessionStack(http.HandlerFunc(app.preferencesPOST)))
	mux.Handle("GET /preferences/export-data", app.mustSessionStack(http.HandlerFunc(app.exportUserDataGET)))
	mux.Handle("POST /preferences/delete-user", app.mustSessionStack(http.HandlerFunc(app.deleteUserPOST)))

	mux.Handle("POST /api/registration/start", app.noStoreSessionStack(http.HandlerFunc(app.beginRegistration)))
	mux.Handle("POST /api/registration/finish", app.noStoreSessionStack(http.HandlerFunc(app.finishRegistration)))
	mux.Handle("POST /api/login/start", app.noStoreSessionStack(http.HandlerFunc(app.beginLogin)))
	mux.Handle("POST /api/login/finish", app.noStoreSessionStack(http.HandlerFunc(app.finishLogin)))
	mux.Handle("POST /api/logout", app.noStoreSessionStack(http.HandlerFunc(app.logout)))

	mux.Handle("GET /api/healthy", app.sessionStack(http.HandlerFunc(app.healthy)))
	mux.Handle("POST /api/reports", app.noAuthStack(http.HandlerFunc(app.reportingAPI)))
	mux.Handle("GET /api/test/timeout", app.noAuthStack(http.HandlerFunc(app.testTimeout)))

	mux.Handle("GET /admin/exercises", app.mustAdminStack(http.HandlerFunc(app.adminExercisesGET)))
	mux.Handle("GET /admin/exercises/{id}", app.mustAdminStack(http.HandlerFunc(app.adminExerciseEditGET)))
	mux.Handle("POST /admin/exercises/{id}", app.mustAdminStack(http.HandlerFunc(app.adminExerciseUpdatePOST)))
	mux.Handle("POST /admin/exercises/generate", app.mustAdminStack(http.HandlerFunc(app.adminExerciseGeneratePOST)))

	mux.Handle("GET /admin/feature-flags", app.sessionStack(http.HandlerFunc(app.adminFeatureFlagsGET)))
	mux.Handle("POST /admin/feature-flags/{name}/toggle",
		app.mustAdminStack(http.HandlerFunc(app.adminFeatureFlagTogglePOST)))

	// Privacy page
	mux.Handle("GET /privacy", app.sessionStack(http.HandlerFunc(app.privacy)))

	// Developer-only design-token reference. Gated inside the handler on app.devMode
	// so prod returns 404; route is registered unconditionally to keep startup simple.
	mux.Handle("GET /dev/styleguide", app.sessionStack(http.HandlerFunc(app.styleguideGET)))

	// Home route (most specific)
	mux.Handle("GET /{$}", app.sessionStack(http.HandlerFunc(app.home)))

	// File server with custom 404 handling
	fileServerHandler, err := app.fileServerHandler()
	if err != nil {
		return nil, fmt.Errorf("fileServerHandler: %w", err)
	}
	mux.Handle("/", fileServerHandler)

	return mux, nil
}
