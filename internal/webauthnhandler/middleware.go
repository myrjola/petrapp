package webauthnhandler

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/logging"
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

		role, err := h.getUserRole(ctx, userID)
		switch {
		case errors.Is(err, sql.ErrNoRows): // Do not authenticate if user does not exist.
		case err != nil:
			h.logger.LogAttrs(r.Context(), slog.LevelError, "unable to fetch user", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		default:
			r = contexthelpers.AuthenticateContext(r, userID, role == roleAdmin)
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
