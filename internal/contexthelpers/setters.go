package contexthelpers

import (
	"context"
	"net/http"

	"github.com/myrjola/petrapp/internal/i18n"
)

func AuthenticateContext(r *http.Request, userID int, isAdmin bool) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, IsAuthenticatedContextKey, true)
	ctx = context.WithValue(ctx, AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, IsAdminContextKey, isAdmin)
	return r.WithContext(ctx)
}

func SetCurrentPath(r *http.Request, currentPath string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, CurrentPathContextKey, currentPath)
	return r.WithContext(ctx)
}

func SetCSPNonce(r *http.Request, cspNonce string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, CspNonceContextKey, cspNonce)
	return r.WithContext(ctx)
}

func SetLanguage(r *http.Request, language i18n.Language) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, LanguageContextKey, language)
	return r.WithContext(ctx)
}
