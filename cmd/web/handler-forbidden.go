package main

import "net/http"

// forbiddenGET renders the 403 page. Reached when middleware bounces the
// user here — currently mustAdmin for authenticated-but-non-admin requests.
// Sits on sessionStack so it works for both authenticated and unauthenticated
// callers (a stale link, a copy-pasted URL).
func (app *application) forbiddenGET(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, http.StatusForbidden, "forbidden", newBaseTemplateData(r))
}
