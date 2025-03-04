package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/errors"
	"html/template"
	"log/slog"
	"net/http"
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
	}).ParseFS(app.templateFS, "base.gohtml", fmt.Sprintf("pages/%s/*.gohtml", pageName)); err != nil {
		return nil, errors.Wrap(err, "new template")
	}
	return t, nil
}

func (app *application) renderToBuf(ctx context.Context, file string, data any) (*bytes.Buffer, error) {
	var (
		err error
		t   *template.Template
	)

	if t, err = app.pageTemplate(file); err != nil {
		return nil, errors.Wrap(err, "retrieve page template", slog.String("page", file))
	}

	buf := new(bytes.Buffer)
	nonce := fmt.Sprintf("nonce=\"%s\"", contexthelpers.CSPNonce(ctx))
	csrf := fmt.Sprintf("<input type=\"hidden\" name=\"csrf_token\" value=\"%s\"/>", contexthelpers.CSRFToken(ctx))
	t.Funcs(template.FuncMap{
		"nonce": func() template.HTMLAttr {
			return template.HTMLAttr(nonce) //nolint:gosec // we trust the nonce since it's not provided by user.
		},
		"csrf": func() template.HTML {
			return template.HTML(csrf) //nolint:gosec // we trust the csrf since it's not provided by user.
		},
	})
	if err = t.ExecuteTemplate(buf, "base", data); err != nil {
		return nil, errors.Wrap(err, "execute template", slog.String("page", file))
	}

	return buf, nil
}

func (app *application) render(w http.ResponseWriter, r *http.Request, status int, file string, data any) {
	var (
		buf *bytes.Buffer
		err error
	)

	if buf, err = app.renderToBuf(r.Context(), file, data); err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(status)

	_, _ = buf.WriteTo(w)
}
