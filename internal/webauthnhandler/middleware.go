package webauthnhandler

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/logging"
	"log/slog"
	"net/http"
)

func (h *WebAuthnHandler) AuthenticateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		userID := h.sessionManager.GetBytes(r.Context(), string(userIDSessionKey))

		// User has not yet authenticated.
		if userID == nil {
			next.ServeHTTP(w, r)
			return
		}

		// If user exists, set context values.
		exists, err := h.userExists(ctx, userID)
		if err != nil {
			h.logger.LogAttrs(r.Context(), slog.LevelError, "server error",
				slog.String("method", r.Method), slog.String("uri", r.URL.RequestURI()), slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		if exists {
			r = contexthelpers.AuthenticateContext(r, userID)
		}

		// Add session information to logging context.
		token := h.sessionManager.Token(ctx)
		// Hash token with sha256 to avoid leaking it in logs.
		tokenHash := sha256.Sum256([]byte(token))
		ctx = logging.WithAttrs(r.Context(),
			slog.String("session_hash", hex.EncodeToString(tokenHash[:])),
			slog.String("user_id", hex.EncodeToString(userID)),
		)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}
