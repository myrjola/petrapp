package main

import "net/http"

// adminGET redirects /admin to the default admin section. Linking from the
// preferences entry to /admin (rather than /admin/exercises directly) keeps
// the entry stable when admin sections come and go.
func (app *application) adminGET(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/admin/exercises")
}
