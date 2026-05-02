package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"runtime/trace"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/logging"
)

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func newStatusResponseWriter(w http.ResponseWriter) *statusResponseWriter {
	return &statusResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		headerWritten:  false,
	}
}

func (mw *statusResponseWriter) WriteHeader(statusCode int) {
	mw.ResponseWriter.WriteHeader(statusCode)

	if !mw.headerWritten {
		mw.statusCode = statusCode
		mw.headerWritten = true
	}
}

func (mw *statusResponseWriter) Write(b []byte) (int, error) {
	mw.headerWritten = true
	written, err := mw.ResponseWriter.Write(b)
	if err != nil {
		return written, fmt.Errorf("write response: %w", err)
	}
	return written, nil
}

func (mw *statusResponseWriter) Unwrap() http.ResponseWriter {
	return mw.ResponseWriter
}

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
base-uri 'none';
require-trusted-types-for 'script';
report-uri /api/reports; report-to reports;`, cspNonce, cspNonce)

		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Reporting-Endpoints", `reports="/api/reports"`)
		w.Header().Set("Referrer-Policy", "origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		// Prevent unload handlers from disabling bfcache.
		w.Header().Set("Permissions-Policy", "unload=()")

		r = contexthelpers.SetCSPNonce(r, cspNonce)

		next.ServeHTTP(w, r)
	})
}

// cacheForever for static assets ensures they are cached indefinitely.
func cacheForever(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

		next.ServeHTTP(w, r)
	})
}

// noCache ensures authenticated requests force revalidation.
func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")
		w.Header().Set("Vary", "Cookie, Accept")
		next.ServeHTTP(w, r)
	})
}

// noStore is used for authentication routes to prevent caching sensitive data anywhere.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, max-age=0, must-revalidate")
		w.Header().Set("Vary", "Cookie, Accept")
		next.ServeHTTP(w, r)
	})
}

func (app *application) logAndTraceRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			proto  = r.Proto
			method = r.Method
			uri    = r.URL.RequestURI()
			path   = r.URL.Path
		)

		ctx := r.Context()
		traceID := rand.Text()
		ctx = logging.WithAttrs(
			ctx,
			slog.Any("trace_id", traceID),
			slog.String("proto", proto),
			slog.String("method", method),
			slog.String("uri", uri),
		)
		r = r.WithContext(ctx)

		start := time.Now()
		app.logger.LogAttrs(ctx, slog.LevelDebug, "received request")

		// Wrap the response writer to capture status code
		sw := newStatusResponseWriter(w)

		if !trace.IsEnabled() {
			next.ServeHTTP(sw, r)
		} else {
			taskName := fmt.Sprintf("HTTP %s %s", r.Method, path)
			traceCtx, task := trace.NewTask(ctx, taskName)

			// Add trace attributes for better context
			trace.Log(traceCtx, "request", fmt.Sprintf("method=%s path=%s proto=%s", method, path, proto))
			trace.Log(traceCtx, "trace_id", traceID)

			defer func() {
				trace.Log(traceCtx, "response", fmt.Sprintf("status=%d duration=%v", sw.statusCode, time.Since(start)))
				task.End()
			}()

			r = r.WithContext(traceCtx)
			next.ServeHTTP(sw, r)
		}

		// Log request completion
		level := slog.LevelInfo
		if sw.statusCode >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		app.logger.LogAttrs(r.Context(), level, "request completed",
			slog.Int("status_code", sw.statusCode), slog.Duration("duration", time.Since(start)))

		// If we have a flight recorder, capture a trace if the request timed out.
		flightRecorderCtx := context.WithoutCancel(ctx)
		if sw.statusCode == http.StatusServiceUnavailable && app.flightRecorder != nil {
			go app.flightRecorder.CaptureTimeoutTrace(flightRecorderCtx)
		}
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
			return
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
		next.ServeHTTP(w, r)
	})
}

// crossOriginProtection implements CSRF protection using Go 1.25's CrossOriginProtection.
func (app *application) crossOriginProtection(next http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	return protection.Handler(next)
}

// Per-request timeout budget. The handler timeout is shorter than the write
// deadline so TimeoutHandler can format the 503 body and flush before the
// connection-level deadline fires.
const (
	regularWriteMargin = 200 * time.Millisecond
	adminWriteDeadline = 30 * time.Second // admins call slow external services.
	adminWriteMargin   = 1 * time.Second
)

// timeout times out the request and cancels the context using http.TimeoutHandler.
// Admins get a longer timeout so that they can call external services. Both
// branches set a write deadline on the underlying connection so the per-request
// contract holds even if the handler stalls inside Write().
func (app *application) timeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		writeDeadline := defaultTimeout
		handlerTimeout := defaultTimeout - regularWriteMargin
		if contexthelpers.IsAdmin(r.Context()) {
			writeDeadline = adminWriteDeadline
			handlerTimeout = adminWriteDeadline - adminWriteMargin
		}
		if err := rc.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
			app.serverError(w, r, err)
			return
		}
		http.TimeoutHandler(next, handlerTimeout, "timed out").ServeHTTP(w, r)
	})
}

// maintenanceMode checks if maintenance mode is enabled and serves a maintenance page if so.
func (app *application) maintenanceMode(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Exclude health endpoints, admin authentication paths, and static files from maintenance checks.
		path := r.URL.Path
		if path == "/api/healthy" ||
			path == "/admin/feature-flags" ||
			path == "/api/login/start" ||
			path == "/api/login/finish" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if maintenance mode is enabled (skip if workoutService is nil for tests)
		if app.workoutService != nil && app.workoutService.IsMaintenanceModeEnabled(ctx) {
			// Allow admin access during maintenance.
			isAdmin := contexthelpers.IsAdmin(r.Context())
			if isAdmin {
				next.ServeHTTP(w, r)
				return
			}

			// Add Retry-After header for better HTTP compliance.
			w.Header().Set("Retry-After", "300")

			// Render the maintenance page
			data := newBaseTemplateData(r)
			app.render(w, r, http.StatusServiceUnavailable, "maintenance", data)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// setInvalidationCookieOnPost busts bfcache by setting a cookie.
func setInvalidationCookieOnPost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			http.SetCookie(w, &http.Cookie{
				Name:     "inv_bfcache",
				Value:    rand.Text(),
				Path:     "/",
				MaxAge:   60, //nolint:mnd // Lives long enough for a back-button after a POST to detect staleness.
				SameSite: http.SameSiteLaxMode,
				Secure:   true,
				HttpOnly: false, // Read client-side in the pageshow handler to compare against the rendered snapshot.
			})
		}
		next.ServeHTTP(w, r)
	})
}
