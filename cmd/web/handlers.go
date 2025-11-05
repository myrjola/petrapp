package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/myrjola/petrapp/internal/contexthelpers"
)

// formatFloat formats a float to remove trailing zeros and unnecessary precision.
// This handles the floating point rounding errors like 60.900000000000006.
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// baseTemplateFuncs returns the base template.FuncMap with placeholder implementations.
// Context-dependent functions (nonce, csrf, mdToHTML) must be overridden with actual implementations.
func (app *application) baseTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"nonce": func() string {
			panic("not implemented")
		},
		"csrf": func() string {
			panic("not implemented")
		},
		"mdToHTML": func() string {
			panic("not implemented")
		},
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
func (app *application) pageTemplate(pageName string) (*template.Template, error) {
	var err error
	// We need to initialize the FuncMap before parsing the files. These will be overridden in the render function.
	var t *template.Template
	t = template.New(pageName).Funcs(app.baseTemplateFuncs())
	if t, err = t.ParseFS(app.templateFS, "base.gohtml", fmt.Sprintf("pages/%s/*.gohtml", pageName)); err != nil {
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
