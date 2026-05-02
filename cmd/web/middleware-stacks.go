package main

import "net/http"

// Middleware stacks centralise the per-route composition. Both routes() and
// fileServerHandler() build handlers from these so the layering stays in lock
// step (auth, CSRF, maintenance mode, panic recovery, etc.).

// withoutMaintenanceModeStack is the base of every other stack: tracing,
// security headers, CSRF, common context, and the request timeout.
func (app *application) withoutMaintenanceModeStack(next http.Handler) http.Handler {
	return app.logAndTraceRequest(secureHeaders(app.crossOriginProtection(
		commonContext(app.timeout(next)))))
}

// sharedStack adds maintenance mode and the bfcache-busting cookie on top of
// the base. Used inside session-bearing requests where state may change.
func (app *application) sharedStack(next http.Handler) http.Handler {
	return app.withoutMaintenanceModeStack(app.maintenanceMode(setInvalidationCookieOnPost(next)))
}

// noAuthStack is for endpoints that bypass session and CSRF gating but still
// want headers, tracing, and panic recovery (e.g. /api/reports).
func (app *application) noAuthStack(next http.Handler) http.Handler {
	return app.recoverPanic(app.withoutMaintenanceModeStack(next))
}

// sessionSharedStack layers the session manager and WebAuthn middleware on top
// of sharedStack. Used by sessionStack and noStoreSessionStack.
func (app *application) sessionSharedStack(next http.Handler) http.Handler {
	return app.sessionManager.LoadAndSave(
		app.webAuthnHandler.AuthenticateMiddleware(app.sharedStack(next)))
}

// sessionStack is the canonical authenticated-page stack with private caching.
func (app *application) sessionStack(next http.Handler) http.Handler {
	return app.recoverPanic(noCache(app.sessionSharedStack(next)))
}

// noStoreSessionStack is sessionStack but with no-store caching, for auth
// endpoints whose responses must never be cached.
func (app *application) noStoreSessionStack(next http.Handler) http.Handler {
	return app.recoverPanic(noStore(app.sessionSharedStack(next)))
}

// mustSessionStack requires authentication.
func (app *application) mustSessionStack(next http.Handler) http.Handler {
	return app.sessionStack(app.mustAuthenticate(next))
}

// mustAdminStack requires authentication and admin privileges.
func (app *application) mustAdminStack(next http.Handler) http.Handler {
	return app.mustSessionStack(app.mustAdmin(next))
}

// sessionDeltaStack layers only the session-related middleware (LoadAndSave,
// auth, maintenance mode, noCache, bfcache cookie) without re-running the
// connection-level middleware (timeout, secureHeaders, commonContext, etc.).
// Use it when the request has already been processed by noAuthStack and you
// need a session-aware sub-handler — e.g. the file server's 404 fallback.
// Wrapping the full sessionStack twice would call SetWriteDeadline on a
// writer already wrapped by the outer TimeoutHandler.
func (app *application) sessionDeltaStack(next http.Handler) http.Handler {
	return noCache(app.sessionManager.LoadAndSave(
		app.webAuthnHandler.AuthenticateMiddleware(
			app.maintenanceMode(setInvalidationCookieOnPost(next)))))
}
