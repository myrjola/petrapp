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

func AuthenticatedUserID(ctx context.Context) int {
	userID, ok := ctx.Value(AuthenticatedUserIDContextKey).(int)
	if !ok {
		return 0
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
