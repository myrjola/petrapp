package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

const (
	RestDayMinutes        = 0
	FortyFiveMinutes      = 45
	OneHourMinutes        = 60
	OneAndHalfHourMinutes = 90

	// defaultMesocycleLength is the fallback mesocycle length when input is absent or out of range.
	defaultMesocycleLength = 5
)

type weekdayPreference struct {
	ID      string // lowercase ID for form field name
	Name    string // Display name
	Minutes int    // Selected workout duration in minutes
}

type workoutDurationOption struct {
	Value int    // Minutes value
	Label string // Display label
}

type preferencesTemplateData struct {
	BaseTemplateData

	Header                   PageHeaderData
	Weekdays                 []weekdayPreference
	DurationOptions          []workoutDurationOption
	VAPIDPublicKey           string
	PushSubscriptionCount    int
	RestNotificationsEnabled bool
	DeloadEnabled            bool
	MesocycleLength          int
	MesocycleLengthOptions   []int
	MesocycleAnchor          time.Time
	Flash                    BannerData
}

func getWorkoutDurationOptions() []workoutDurationOption {
	return []workoutDurationOption{
		{Value: RestDayMinutes, Label: "Rest day"},
		{Value: FortyFiveMinutes, Label: "45 minutes"},
		{Value: OneHourMinutes, Label: "1 hour"},
		{Value: OneAndHalfHourMinutes, Label: "1.5 hours"},
	}
}

func preferencesToWeekdays(prefs domain.Preferences) []weekdayPreference {
	return []weekdayPreference{
		{ID: "monday", Name: "Monday", Minutes: prefs.MondayMinutes},
		{ID: "tuesday", Name: "Tuesday", Minutes: prefs.TuesdayMinutes},
		{ID: "wednesday", Name: "Wednesday", Minutes: prefs.WednesdayMinutes},
		{ID: "thursday", Name: "Thursday", Minutes: prefs.ThursdayMinutes},
		{ID: "friday", Name: "Friday", Minutes: prefs.FridayMinutes},
		{ID: "saturday", Name: "Saturday", Minutes: prefs.SaturdayMinutes},
		{ID: "sunday", Name: "Sunday", Minutes: prefs.SundayMinutes},
	}
}

func parseMesocycleLength(value string) int {
	n, err := strconv.Atoi(value)
	if err != nil {
		return defaultMesocycleLength
	}
	if n < 4 || n > 7 {
		return defaultMesocycleLength
	}
	return n
}

func parseMinutes(value string) int {
	minutes, err := strconv.Atoi(value)
	if err != nil {
		return 0 // Default to rest day if parsing fails
	}
	// Validate against allowed values
	switch minutes {
	case RestDayMinutes, FortyFiveMinutes, OneHourMinutes, OneAndHalfHourMinutes:
		return minutes
	default:
		return RestDayMinutes // Default to rest day for invalid values
	}
}

func (app *application) preferencesGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefs, err := app.service.GetUserPreferences(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}
	subCount, err := app.service.CountPushSubscriptions(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("count push subscriptions: %w", err))
		return
	}

	base := newBaseTemplateData(r)
	flash := app.popFlash(ctx)
	data := preferencesTemplateData{
		BaseTemplateData: base,
		Header: PageHeaderData{
			Title:    "Weekly Schedule",
			Subtitle: "Select the days you're planning to go to the gym",
			Nonce:    base.Nonce,
		},
		Weekdays:                 preferencesToWeekdays(prefs),
		DurationOptions:          getWorkoutDurationOptions(),
		VAPIDPublicKey:           app.vapidPublicKey,
		PushSubscriptionCount:    subCount,
		RestNotificationsEnabled: prefs.RestNotificationsEnabled,
		DeloadEnabled:            prefs.DeloadEnabled,
		MesocycleLength:          prefs.MesocycleLength,
		MesocycleLengthOptions:   []int{4, 5, 6, 7},
		MesocycleAnchor:          prefs.MesocycleAnchor,
		Flash: BannerData{
			Variant: flash.Variant,
			Message: flash.Message,
			Nonce:   base.Nonce,
		},
	}

	app.render(w, r, http.StatusOK, "preferences", data)
}

func (app *application) preferencesPOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}

	prefs, err := app.service.GetUserPreferences(r.Context())
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}
	prefs.DeloadEnabled = r.Form.Get("deload_enabled") == "on"
	prefs.MesocycleLength = parseMesocycleLength(r.Form.Get("mesocycle_length"))
	prefs.MondayMinutes = parseMinutes(r.Form.Get("monday_minutes"))
	prefs.TuesdayMinutes = parseMinutes(r.Form.Get("tuesday_minutes"))
	prefs.WednesdayMinutes = parseMinutes(r.Form.Get("wednesday_minutes"))
	prefs.ThursdayMinutes = parseMinutes(r.Form.Get("thursday_minutes"))
	prefs.FridayMinutes = parseMinutes(r.Form.Get("friday_minutes"))
	prefs.SaturdayMinutes = parseMinutes(r.Form.Get("saturday_minutes"))
	prefs.SundayMinutes = parseMinutes(r.Form.Get("sunday_minutes"))

	if prefs.IsEmpty() {
		app.putFlashError(r.Context(), "Please schedule at least one workout day.")
		redirect(w, r, "/preferences")
		return
	}

	if err = app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
		app.logger.LogAttrs(r.Context(), slog.LevelDebug, "preferences details", slog.Any("preferences", prefs))
		return
	}

	if err = app.service.RegenerateWeeklyPlanIfUnstarted(r.Context()); err != nil {
		// Preferences are already saved; regeneration failure is not fatal because
		// ResolveWeeklySchedule on the home page will regenerate the plan automatically.
		app.logger.LogAttrs(r.Context(), slog.LevelWarn, "regenerate weekly plan after preference save",
			slog.Any("error", err))
	}

	redirect(w, r, "/")
}

func (app *application) deleteUserPOST(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Delete the user and all their data
	if err := app.webAuthnHandler.DeleteUser(ctx); err != nil {
		app.serverError(w, r, fmt.Errorf("delete user: %w", err))
		return
	}

	// Log the user out by clearing the session and redirect home.
	clearSiteData(w)
	if err := app.webAuthnHandler.Logout(ctx); err != nil {
		app.serverError(w, r, fmt.Errorf("logout after user deletion: %w", err))
		return
	}

	redirect(w, r, "/")
}

// clearSiteData to ensure you can't navigate backwards to sensitive content after logging out.
func clearSiteData(w http.ResponseWriter) {
	w.Header().Set("Clear-Site-Data",
		"\"cache\", \"cookies\", \"storage\", \"executionContexts\", \"prefetchCache\", \"prerenderCache\"")
}

func (app *application) exportUserDataGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Create the user database export
	exportPath, err := app.service.ExportUserData(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("export user data: %w", err))
		return
	}

	// Clean up the temporary file when done
	defer func() {
		if removeErr := os.Remove(exportPath); removeErr != nil {
			app.logger.LogAttrs(ctx, slog.LevelWarn, "failed to remove temporary export file",
				slog.String("path", exportPath), slog.Any("error", removeErr))
		}
	}()

	// Open the file for reading
	file, err := os.Open(exportPath)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("open export file: %w", err))
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			app.logger.LogAttrs(ctx, slog.LevelWarn, "failed to close export file",
				slog.String("path", exportPath), slog.Any("error", closeErr))
		}
	}()

	// Set headers for file download
	filename := filepath.Base(exportPath)
	w.Header().Set("Content-Type", "application/x-sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Stream the file to the client
	_, err = io.Copy(w, file)
	if err != nil {
		app.logger.LogAttrs(ctx, slog.LevelError, "failed to stream export file to client",
			slog.String("path", exportPath), slog.Any("error", err))
		return
	}
}

func (app *application) preferencesRestartMesocyclePOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}
	if err := app.service.RestartMesocycleAnchor(r.Context()); err != nil {
		app.serverError(w, r, fmt.Errorf("restart mesocycle: %w", err))
		return
	}
	redirect(w, r, "/preferences")
}

func (app *application) preferencesRestNotificationsTogglePOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}
	prefs, err := app.service.GetUserPreferences(r.Context())
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get preferences: %w", err))
		return
	}
	prefs.RestNotificationsEnabled = r.Form.Get("rest_notifications_enabled") == "on"
	if err = app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save preferences: %w", err))
		return
	}
	redirect(w, r, "/preferences")
}

func (app *application) preferencesStartDeloadNowPOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}
	if err := app.service.StartDeloadNow(r.Context()); err != nil {
		app.serverError(w, r, fmt.Errorf("start deload now: %w", err))
		return
	}
	redirect(w, r, "/preferences")
}
