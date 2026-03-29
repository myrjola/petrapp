package main

import (
	"net/http"
	"strings"

	"github.com/myrjola/petrapp/internal/i18n"
)

const (
	secondsPerMinute = 60
	minutesPerHour   = 60
	hoursPerDay      = 24
	daysPerYear      = 365
)

// isRelativePath checks if a path is a relative path without scheme or host and doesn't allow ambiguous slashes.
func isRelativePath(path string) bool {
	// Reject paths that contain a scheme (e.g., http://, https://, //).
	if strings.Contains(path, "://") || strings.HasPrefix(path, "//") {
		return false
	}
	// Accept paths that start with /, but not if the second character is / or \.
	if strings.HasPrefix(path, "/") {
		if len(path) == 1 || (path[1] != '/' && path[1] != '\\') {
			return true
		}
	}
	return false
}

// setLanguagePOST handles the POST request to set the user's language preference.
func (app *application) setLanguagePOST(w http.ResponseWriter, r *http.Request) {
	lang := r.FormValue("language")

	// Validate the language.
	if !i18n.IsSupported(i18n.Language(lang)) {
		http.Error(w, "Invalid language", http.StatusBadRequest)
		return
	}

	// Set the language cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     "language",
		Value:    lang,
		Path:     "/",
		MaxAge:   daysPerYear * hoursPerDay * minutesPerHour * secondsPerMinute, // 1 year.
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect back to the referrer or home page.
	// Only allow relative paths to prevent open redirects.
	referer := r.Header.Get("Referer")
	if referer == "" || !isRelativePath(referer) {
		referer = "/"
	}

	// Use 303 See Other to redirect after POST.
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
