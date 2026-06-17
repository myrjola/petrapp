package main

import (
	"io/fs"
	"testing"

	"github.com/jba/templatecheck"
)

// pageViewModels maps every page directory under ui/templates/pages to the
// view-model struct its handler renders via app.render(..., page, data). It is
// the single source of truth for templatecheck coverage.
//
// TestTemplatesTypecheck is fail-closed against it: a page directory with no
// entry here fails the test, so a new page cannot silently skip type-checking
// (see docs/adr/0008-static-template-linting-over-templ.md). Pages rendered with
// newBaseTemplateData(r) map to BaseTemplateData{}.
//
// The zero values are deliberate: templatecheck checks the struct *type*, not
// field values, so a bare T{} is the idiomatic type carrier — hence the
// exhaustruct suppression.
//
//nolint:exhaustruct // bare T{} type carriers; see the note above.
func pageViewModels() map[string]any {
	return map[string]any{
		"admin-exercise-edit": exerciseEditTemplateData{},
		"admin-exercises":     exerciseAdminTemplateData{},
		"admin-feature-flags": featureFlagsAdminTemplateData{},
		"error":               errorTemplateData{},
		"error-ux":            errorUXTemplateData{},
		"exercise-add":        exerciseAddTemplateData{},
		"exercise-info":       exerciseInfoTemplateData{},
		"exercise-swap":       exerciseSwapTemplateData{},
		"exerciseset":         exerciseSetTemplateData{},
		"forbidden":           BaseTemplateData{},
		"home":                homeTemplateData{},
		"maintenance":         BaseTemplateData{},
		"not-found":           BaseTemplateData{},
		"preferences":         preferencesTemplateData{},
		"privacy":             privacyTemplateData{},
		"schedule":            scheduleTemplateData{},
		"styleguide":          styleguideTemplateData{},
		"workout":             workoutTemplateData{},
		"workout-completion":  workoutCompletionTemplateData{},
		"workout-not-found":   workoutNotFoundTemplateData{},
	}
}

// newTemplateTestApp builds a minimal application wired only for parsing
// templates: the embedded template FS plus the asset manifest the "asset"
// template func needs. setupUI(false, ...) uses the //go:embed trees, so no
// PETRAPP_TEMPLATE_PATH resolution is required and the test sees the current
// on-disk files (re-embedded by `go test`).
func newTemplateTestApp(t *testing.T) *application {
	t.Helper()
	templateFS, _, assets, err := setupUI(false, "")
	if err != nil {
		t.Fatalf("setupUI: %v", err)
	}
	//nolint:exhaustruct // parsePageTemplate only reads templateFS + assets.
	return &application{templateFS: templateFS, assets: assets}
}

// TestTemplatesTypecheck parses every page (base + components + page) and runs
// templatecheck against its registered view-model. Parsing is a prerequisite,
// so this doubles as the parse-all test (syntax + undefined-function errors);
// templatecheck additionally catches field/type/arity mismatches and unresolved
// {{template "x"}} targets, statically and across every branch — without
// rendering the page.
func TestTemplatesTypecheck(t *testing.T) {
	t.Parallel()

	app := newTemplateTestApp(t)
	vms := pageViewModels()

	entries, err := fs.ReadDir(app.templateFS, "pages")
	if err != nil {
		t.Fatalf("read pages dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, ok := vms[e.Name()]; !ok {
			t.Errorf("page %q has no entry in pageViewModels(); add its view-model "+
				"so templatecheck covers it (ADR 0008)", e.Name())
		}
	}

	for page, vm := range vms {
		tmpl, parseErr := app.parsePageTemplate(page)
		if parseErr != nil {
			t.Errorf("parse page %q: %v", page, parseErr)
			continue
		}
		base := tmpl.Lookup("base")
		if base == nil {
			t.Errorf("page %q defines no base template", page)
			continue
		}
		if checkErr := templatecheck.CheckHTML(base, vm); checkErr != nil {
			t.Errorf("templatecheck page %q: %v", page, checkErr)
		}
	}
}
