package main

import (
	"crypto/rand"
	"fmt"
	"github.com/justinas/nosurf"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/logging"
	"log/slog"
	"net/http"
	"runtime/debug"
)

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate a random nonce for use in CSP and set it in the context so that it can be added to the script tags.
		cspNonce := rand.Text()
		csp := fmt.Sprintf(`default-src 'none';
script-src 'nonce-%s' 'strict-dynamic' https: http:;
connect-src 'self';
img-src 'self';
style-src 'nonce-%s' 'self';
frame-ancestors 'self';
form-action 'self';
object-src 'none';
manifest-src 'self';
base-uri 'none';`, cspNonce, cspNonce)

		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")

		r = contexthelpers.SetCSPNonce(r, cspNonce)

		next.ServeHTTP(w, r)
	})
}

func cacheForeverHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		next.ServeHTTP(w, r)
	})
}

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			proto  = r.Proto
			method = r.Method
			uri    = r.URL.RequestURI()
		)

		ctx := r.Context()
		requestID := rand.Text()
		ctx = logging.WithAttrs(
			ctx,
			slog.Any("request_id", requestID),
			slog.String("proto", proto),
			slog.String("method", method),
			slog.String("uri", uri),
		)
		r = r.WithContext(ctx)

		app.logger.LogAttrs(ctx, slog.LevelDebug, "received request")

		next.ServeHTTP(w, r)
	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if excp := recover(); excp != nil {
				err := fmt.Errorf("panic: %v\n%s", excp, string(debug.Stack()))
				app.serverError(w, r, err)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// mustAuthenticate redirects the user to the home page if they are not authenticated.
func (app *application) mustAuthenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAuthenticated := contexthelpers.IsAuthenticated(r.Context())
		if !isAuthenticated {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
		next.ServeHTTP(w, r)
	})
}

func commonContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = contexthelpers.SetCurrentPath(r, r.URL.Path)
		r = contexthelpers.SetCSRFToken(r, nosurf.Token(r))
		next.ServeHTTP(w, r)
	})
}

// noSurf implements CSRF protection using https://github.com/justinas/nosurf
func (app *application) noSurf(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)
	csrfHandler.SetFailureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.logger.LogAttrs(r.Context(), slog.LevelWarn, "csrf token validation failed")
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
	}))
	csrfHandler.SetBaseCookie(http.Cookie{
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	return csrfHandler
}
