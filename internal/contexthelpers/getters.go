package contexthelpers

import (
	"context"
)

func IsAuthenticated(ctx context.Context) bool {
	isAuthenticated, ok := ctx.Value(IsAuthenticatedContextKey).(bool)
	if !ok {
		return false
	}

	return isAuthenticated
}

func AuthenticatedUserID(ctx context.Context) []byte {
	userID, ok := ctx.Value(AuthenticatedUserIDContextKey).([]byte)
	if !ok {
		return nil
	}

	return userID
}

func CurrentPath(ctx context.Context) string {
	currentPath, ok := ctx.Value(CurrentPathContextKey).(string)
	if !ok {
		return ""
	}

	return currentPath
}

func CSRFToken(ctx context.Context) string {
	csrfToken, ok := ctx.Value(CsrfTokenContextKey).(string)
	if !ok {
		return ""
	}

	return csrfToken
}

func CSPNonce(ctx context.Context) string {
	cspNonce, ok := ctx.Value(CspNonceContextKey).(string)
	if !ok {
		return ""
	}

	return cspNonce
}

func IsAdmin(ctx context.Context) bool {
	isAdmin, ok := ctx.Value(IsAdminContextKey).(bool)
	if !ok {
		return false
	}
	return isAdmin
}
