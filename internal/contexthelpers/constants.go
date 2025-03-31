package contexthelpers

type contextKey string

const IsAuthenticatedContextKey = contextKey("isAuthenticated")
const AuthenticatedUserIDContextKey = contextKey("authenticatedUserID")
const CurrentPathContextKey = contextKey("currentPath")
const CsrfTokenContextKey = contextKey("csrfToken")
const CspNonceContextKey = contextKey("cspNonce")
const IsAdminContextKey = contextKey("isAdmin")
