package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"

	"github.com/myrjola/petrapp/internal/contexthelpers"
)

// pageTemplate returns a template for the given page name.
//
// pageName corresponds to directory inside ui/templates/pages folder. It has to include a template named "page".
func (app *application) pageTemplate(pageName string) (*template.Template, error) {
	var err error
	// We need to initialize the FuncMap before parsing the files. These will be overridden in the render function.
	var t *template.Template
	if t, err = template.New(pageName).Funcs(template.FuncMap{
		"nonce": func() string {
			panic("not implemented")
		},
		"csrf": func() string {
			panic("not implemented")
		},
		"mdToHTML": func() string {
			panic("not implemented")
		},
	}).ParseFS(app.templateFS, "base.gohtml", fmt.Sprintf("pages/%s/*.gohtml", pageName)); err != nil {
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
	nonce := fmt.Sprintf("nonce=\"%s\"", contexthelpers.CSPNonce(ctx))
	t.Funcs(template.FuncMap{
		"nonce": func() template.HTMLAttr {
			return template.HTMLAttr(nonce) //nolint:gosec // we trust the nonce since it's not provided by user.
		},
		"mdToHTML": func(markdown string) template.HTML {
			return app.renderMarkdownToHTML(ctx, markdown)
		},
	})
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
