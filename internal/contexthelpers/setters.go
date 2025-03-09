package contexthelpers

import (
	"context"
	"net/http"
)

func AuthenticateContext(r *http.Request, userID []byte, isAdmin bool) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, isAuthenticatedContextKey, true)
	ctx = context.WithValue(ctx, authenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, isAdminContextKey, isAdmin)
	return r.WithContext(ctx)
}

func SetCurrentPath(r *http.Request, currentPath string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, currentPathContextKey, currentPath)
	return r.WithContext(ctx)
}

func SetCSRFToken(r *http.Request, csrfToken string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, csrfTokenContextKey, csrfToken)
	return r.WithContext(ctx)
}

func SetCSPNonce(r *http.Request, cspNonce string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, cspNonceContextKey, cspNonce)
	return r.WithContext(ctx)
}

func SetAdminStatus(r *http.Request, isAdmin bool) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, isAdminContextKey, isAdmin)
	return r.WithContext(ctx)
}
