package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strconv"
	"sync"
)

// formatFloat formats a float to remove trailing zeros and unnecessary precision.
// Rounds to one decimal place to avoid floating-point artifacts from weight calculations
// (e.g., multiplying by factors like 0.85 can produce 62.54 instead of 62.5).
func formatFloat(f float64) string {
	rounded := math.Round(f*10) / 10 //nolint:mnd // 10 = one decimal place precision
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

// templateFuncs returns the funcs registered at parse time. Entries are safe to
// share across goroutines; asset closes over the (immutable) asset manifest to
// emit content-hashed URLs for cache busting.
func (app *application) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"asset":       app.assets.URL,
		"formatFloat": formatFloat,
		"sub":         func(a, b int) int { return a - b },
		"backLink": func(href string, nonce template.HTMLAttr) BackLinkData {
			return BackLinkData{Href: href, Nonce: nonce}
		},
		"exerciseSearch": func(query string, nonce template.HTMLAttr) ExerciseSearchData {
			return ExerciseSearchData{Query: query, Nonce: nonce}
		},
	}
}

// templateCache memoizes parsed page templates so each request doesn't
// re-read template files from disk and re-parse them. The cached
// templates are read-only after first execute and safe to share.
type templateCache struct {
	mu sync.RWMutex
	m  map[string]*template.Template
}

func newTemplateCache() *templateCache {
	return &templateCache{mu: sync.RWMutex{}, m: make(map[string]*template.Template)}
}

func (c *templateCache) get(name string) *template.Template {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.m[name]
}

func (c *templateCache) set(name string, t *template.Template) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[name] = t
}

// pageTemplate returns the parsed template for the given page name.
//
// pageName corresponds to the directory inside ui/templates/pages. It
// must include a template named "page".
//
// In dev mode (no FLY_APP_NAME) templates are parsed fresh on every
// call so a template edit is reflected on the next refresh. In
// production the parsed template is cached and reused across requests.
// The cached template is never mutated after the first Execute, so it
// is safe to share across goroutines.
func (app *application) pageTemplate(pageName string) (*template.Template, error) {
	if app.devMode {
		return app.parsePageTemplate(pageName)
	}
	if cached := app.parsedTemplates.get(pageName); cached != nil {
		return cached, nil
	}
	parsed, err := app.parsePageTemplate(pageName)
	if err != nil {
		return nil, err
	}
	app.parsedTemplates.set(pageName, parsed)
	return parsed, nil
}

func (app *application) parsePageTemplate(pageName string) (*template.Template, error) {
	t := template.New(pageName).Funcs(app.templateFuncs())
	t, err := t.ParseFS(app.templateFS,
		"base.gohtml",
		"components/*.gohtml",
		fmt.Sprintf("pages/%s/*.gohtml", pageName),
	)
	if err != nil {
		return nil, fmt.Errorf("new template: %w", err)
	}
	return t, nil
}

// renderBufPool reuses template-output buffers across requests.
//
//nolint:gochecknoglobals // sync.Pool must be package-scoped to share across handler invocations.
var renderBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// maxPooledBufBytes caps the capacity of buffers we return to the
// pool, so a pathologically large render does not pin memory forever.
const maxPooledBufBytes = 1 << 20

// putRenderBuf returns buf to renderBufPool, dropping it if its
// capacity is above maxPooledBufBytes.
func putRenderBuf(buf *bytes.Buffer) {
	if buf.Cap() > maxPooledBufBytes {
		return
	}
	buf.Reset()
	renderBufPool.Put(buf)
}

// renderToBuf executes pageName's template into a buffer drawn from
// renderBufPool. Callers MUST hand the buffer back to putRenderBuf
// after they are done with it.
func (app *application) renderToBuf(_ context.Context, file string, data any) (*bytes.Buffer, error) {
	t, err := app.pageTemplate(file)
	if err != nil {
		return nil, fmt.Errorf("retrieve page template %s: %w", file, err)
	}
	buf, _ := renderBufPool.Get().(*bytes.Buffer)
	if err = t.ExecuteTemplate(buf, "base", data); err != nil {
		putRenderBuf(buf)
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
	putRenderBuf(buf)
}

type privacyTemplateData struct {
	BaseTemplateData

	Header PageHeaderData
}

func (app *application) privacy(w http.ResponseWriter, r *http.Request) {
	base := newBaseTemplateData(r)
	data := privacyTemplateData{
		BaseTemplateData: base,
		Header: PageHeaderData{
			Title:    "Privacy & Security",
			Subtitle: "",
			Nonce:    base.Nonce,
		},
	}

	app.render(w, r, http.StatusOK, "privacy", data)
}
