package main

import (
	"bytes"
	"github.com/myrjola/petrapp/internal/workout"
	"github.com/yuin/goldmark"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// exerciseInfoTemplateData contains data for the exercise info template.
type exerciseInfoTemplateData struct {
	BaseTemplateData
	Date            time.Time
	Exercise        workout.Exercise
	DescriptionHTML template.HTML
}

// exerciseInfoGET handles GET requests to view exercise information.
func (app *application) exerciseInfoGET(w http.ResponseWriter, r *http.Request) {
	// Parse date from URL path
	dateStr := r.PathValue("date")
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Parse exercise ID from URL path
	exerciseIDStr := r.PathValue("exerciseID")
	exerciseID, err := strconv.Atoi(exerciseIDStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get the exercise
	exercise, err := app.workoutService.GetExercise(r.Context(), exerciseID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Render markdown to HTML
	descriptionHTML := app.renderMarkdownToHTML(r, exercise.DescriptionMarkdown)

	data := exerciseInfoTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Date:             date,
		Exercise:         exercise,
		DescriptionHTML:  descriptionHTML,
	}

	app.render(w, r, http.StatusOK, "exercise-info", data)
}

// renderMarkdownToHTML converts markdown string to HTML.
func (app *application) renderMarkdownToHTML(r *http.Request, markdown string) template.HTML {
	md := goldmark.New()

	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelError, "failed to render markdown",
			slog.Any("error", err))
		return template.HTML("<p>Error rendering markdown content.</p>")
	}

	// Returning as template.HTML tells Go this is safe HTML that doesn't need escaping
	return template.HTML(buf.String()) //nolint:gosec // we trust the markdown renderer
}
