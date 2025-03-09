package contexthelpers

import (
	"context"
)

func IsAuthenticated(ctx context.Context) bool {
	isAuthenticated, ok := ctx.Value(isAuthenticatedContextKey).(bool)
	if !ok {
		return false
	}

	return isAuthenticated
}

func AuthenticatedUserID(ctx context.Context) []byte {
	userID, ok := ctx.Value(authenticatedUserIDContextKey).([]byte)
	if !ok {
		return nil
	}

	return userID
}

func CurrentPath(ctx context.Context) string {
	currentPath, ok := ctx.Value(currentPathContextKey).(string)
	if !ok {
		return ""
	}

	return currentPath
}

func CSRFToken(ctx context.Context) string {
	csrfToken, ok := ctx.Value(csrfTokenContextKey).(string)
	if !ok {
		return ""
	}

	return csrfToken
}

func CSPNonce(ctx context.Context) string {
	cspNonce, ok := ctx.Value(cspNonceContextKey).(string)
	if !ok {
		return ""
	}

	return cspNonce
}

func IsAdmin(ctx context.Context) bool {
	isAdmin, ok := ctx.Value(isAdminContextKey).(bool)
	if !ok {
		return false
	}
	return isAdmin
}
