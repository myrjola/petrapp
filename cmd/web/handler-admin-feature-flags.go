package main

import (
	"log/slog"
	"net/http"

	"github.com/myrjola/petrapp/internal/workout"
)

// featureFlagsAdminTemplateData contains data for the feature flags admin template.
type featureFlagsAdminTemplateData struct {
	BaseTemplateData
	FeatureFlags []workout.FeatureFlag
}

// adminFeatureFlagsGET handles GET requests to the feature flags admin page.
func (app *application) adminFeatureFlagsGET(w http.ResponseWriter, r *http.Request) {
	// Get all feature flags from the workout service
	flags, err := app.workoutService.ListFeatureFlags(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := featureFlagsAdminTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		FeatureFlags:     flags,
	}

	app.render(w, r, http.StatusOK, "admin-feature-flags", data)
}

// adminFeatureFlagTogglePOST handles POST requests to toggle a feature flag.
func (app *application) adminFeatureFlagTogglePOST(w http.ResponseWriter, r *http.Request) {
	// Get feature flag name from URL
	name := r.PathValue("name")
	if name == "" {
		http.NotFound(w, r)
		return
	}

	// Get current flag state
	flag, err := app.workoutService.GetFeatureFlag(r.Context(), name)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Toggle the flag
	flag.Enabled = !flag.Enabled

	// Update the flag
	if err = app.workoutService.SetFeatureFlag(r.Context(), flag); err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "toggled feature flag",
		slog.String("name", name),
		slog.Bool("enabled", flag.Enabled))

	// Redirect back to feature flags list
	redirect(w, r, "/admin/feature-flags")
}
