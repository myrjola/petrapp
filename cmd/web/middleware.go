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
	"time"
)

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate a random nonce for use in CSP and set it in the context so that it can be added to the script tags.
		cspNonce := rand.Text()
		csp := fmt.Sprintf(`default-src 'none';
script-src 'nonce-%s' 'strict-dynamic' 'unsafe-inline' https: http:;
connect-src 'self';
img-src 'self';
style-src 'nonce-%s' 'self' 'unsafe-inline';
frame-ancestors 'self';
form-action 'self';
font-src 'none';
object-src 'none';
manifest-src 'self';
base-uri 'none';`, cspNonce, cspNonce)

		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")

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

func noCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
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
			redirect(w, r, "/")
		}
		next.ServeHTTP(w, r)
	})
}

// mustAdmin asserts that the user is admin.
func (app *application) mustAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAuthenticated := contexthelpers.IsAuthenticated(r.Context())
		isAdmin := contexthelpers.IsAdmin(r.Context())
		if !isAuthenticated || !isAdmin {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
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

// timeout times out the request and cancels the context using http.TimeoutHandler.
// Admins get a longer timeout so that they can call external services.
func (app *application) timeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		timeout := defaultTimeout - (200 * time.Millisecond) //nolint:mnd // writing the response takes time.
		if contexthelpers.IsAdmin(r.Context()) {
			timeout = 29 * time.Second                                   //nolint:mnd // slow external services.
			err := rc.SetWriteDeadline(time.Now().Add(30 * time.Second)) //nolint:mnd // slow external services.
			if err != nil {
				app.serverError(w, r, err)
				return
			}
		}
		http.TimeoutHandler(next, timeout, "timed out").ServeHTTP(w, r)
	})
}
