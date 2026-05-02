package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strconv"

	"github.com/myrjola/petrapp/internal/contexthelpers"
)

// formatFloat formats a float to remove trailing zeros and unnecessary precision.
// Rounds to one decimal place to avoid floating-point artifacts from weight calculations
// (e.g., multiplying by factors like 0.85 can produce 62.54 instead of 62.5).
func formatFloat(f float64) string {
	rounded := math.Round(f*10) / 10 //nolint:mnd // 10 = one decimal place precision
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

// baseTemplateFuncs returns the base template.FuncMap with safe zero-value
// implementations for context-dependent functions. Real implementations are
// bound per-request in contextTemplateFuncs and override these defaults via
// (*template.Template).Funcs. The signatures must match the real ones so
// templates parsed with the base set can also be executed with it (a panic
// here would surface as a 500 in any code path that forgets to rebind).
func (app *application) baseTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"nonce":       func() template.HTMLAttr { return "" },
		"mdToHTML":    func(_ string) template.HTML { return "" },
		"formatFloat": formatFloat,
	}
}

// contextTemplateFuncs returns template.FuncMap with context-dependent function implementations.
func (app *application) contextTemplateFuncs(ctx context.Context) template.FuncMap {
	nonce := fmt.Sprintf("nonce=\"%s\"", contexthelpers.CSPNonce(ctx))
	return template.FuncMap{
		"nonce": func() template.HTMLAttr {
			return template.HTMLAttr(nonce) //nolint:gosec // we trust the nonce since it's not provided by user.
		},
		"mdToHTML": func(markdown string) template.HTML {
			return app.renderMarkdownToHTML(ctx, markdown)
		},
		"formatFloat": formatFloat,
	}
}

// pageTemplate returns a template for the given page name.
//
// pageName corresponds to directory inside ui/templates/pages folder. It has to include a template named "page".
// Shared component templates from ui/templates/components are also parsed and available to every page.
func (app *application) pageTemplate(pageName string) (*template.Template, error) {
	var err error
	// We need to initialize the FuncMap before parsing the files. These will be overridden in the render function.
	var t *template.Template
	t = template.New(pageName).Funcs(app.baseTemplateFuncs())
	if t, err = t.ParseFS(app.templateFS,
		"base.gohtml",
		"components/*.gohtml",
		fmt.Sprintf("pages/%s/*.gohtml", pageName),
	); err != nil {
		return nil, fmt.Errorf("new template: %w", err)
	}
	return t, nil
}

func (app *application) renderToBuf(ctx context.Context, file string, data any) (*bytes.Buffer, error) {
	var (
		err error
		t   *template.Template
	)

	if t, err = app.pageTemplate(file); err != nil {
		return nil, fmt.Errorf("retrieve page template %s: %w", file, err)
	}

	buf := new(bytes.Buffer)
	t.Funcs(app.contextTemplateFuncs(ctx))
	if err = t.ExecuteTemplate(buf, "base", data); err != nil {
		return nil, fmt.Errorf("execute template %s: %w", file, err)
	}

	return buf, nil
}

/*
 * render renders the template residing in the /ui/templates/pages/{pageName} folder from the repository root and writes
 * it to the response writer.
 */
func (app *application) render(w http.ResponseWriter, r *http.Request, status int, pageName string, data any) {
	var (
		buf *bytes.Buffer
		err error
	)

	if buf, err = app.renderToBuf(r.Context(), pageName, data); err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(status)

	_, _ = buf.WriteTo(w)
}

type privacyTemplateData struct {
	BaseTemplateData
}

func (app *application) privacy(w http.ResponseWriter, r *http.Request) {
	data := privacyTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
	}

	app.render(w, r, http.StatusOK, "privacy", data)
}
