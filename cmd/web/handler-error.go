package main

import (
	"net/http"
	"strings"
)

// errorTemplateData feeds error.gohtml. From is a sanitised same-origin path
// the user came from, or "" when none was provided / it failed sanitisation.
type errorTemplateData struct {
	BaseTemplateData

	From string
}

// sanitiseFromPath returns the input if it is a same-origin path safe to use
// as an anchor href, or "" otherwise. Defence-in-depth against open-redirect
// vectors via the ?from= query parameter:
//
//   - must start with "/" (rejects relative paths and absolute URLs)
//   - must not start with "//" (rejects protocol-relative URLs that the
//     browser would resolve to a different origin)
//   - must not contain "//" anywhere (rejects "/path//evil.example.com")
//
// The query string is preserved verbatim — the user's original location may
// have legitimately carried one.
func sanitiseFromPath(s string) string {
	if s == "" || !strings.HasPrefix(s, "/") || strings.Contains(s, "//") {
		return ""
	}
	return s
}

func (app *application) errorGET(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusOK, "error", errorTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		From:             sanitiseFromPath(r.URL.Query().Get("from")),
	})
}
