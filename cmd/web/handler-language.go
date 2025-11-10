package main

import (
	"net/http"
	"time"

	"github.com/myrjola/petrapp/internal/i18n"
)

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
		MaxAge:   365 * 24 * 60 * 60, // 1 year.
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect back to the referrer or home page.
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/"
	}

	// Use 303 See Other to redirect after POST.
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
