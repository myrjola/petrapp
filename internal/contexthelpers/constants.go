package contexthelpers

type contextKey string

const isAuthenticatedContextKey = contextKey("isAuthenticated")
const authenticatedUserIDContextKey = contextKey("authenticatedUserID")
const currentPathContextKey = contextKey("currentPath")
const csrfTokenContextKey = contextKey("csrfToken")
const cspNonceContextKey = contextKey("cspNonce")
const isAdminContextKey = contextKey("isAdmin")
