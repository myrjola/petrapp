package auth

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// UnknownCredentialError is returned when a credential ID is not found in the database.
// This allows the client to signal the authenticator to remove the credential.
type UnknownCredentialError struct {
	CredentialID []byte
	Err          error
}

func (e *UnknownCredentialError) Error() string {
	return fmt.Sprintf("unknown credential: %x: %v", e.CredentialID, e.Err)
}

func (e *UnknownCredentialError) Unwrap() error {
	return e.Err
}

type WebAuthnHandler struct {
	logger         *slog.Logger
	webAuthn       *webauthn.WebAuthn
	sessionManager *scs.SessionManager
	store          Store

	// InternalErrorHandler, when set, owns the response on any internal
	// failure inside this package (DB lookup errors, etc.). Wired by the
	// caller after construction so this package stays unaware of the
	// stack-navigator wire protocol. When nil, the fallback is a plain
	// 500 via http.Error.
	InternalErrorHandler func(w http.ResponseWriter, r *http.Request, err error)
}

func New(
	addr string,
	fqdn string,
	tlsEnabled bool,
	logger *slog.Logger,
	sessionManager *scs.SessionManager,
	store Store,
) (*WebAuthnHandler, error) {
	var (
		err     error
		timeout = time.Minute * 5
	)
	// Register the session data struct for encoding to the session.
	// gob.Register is idempotent, so calling it per New is safe.
	// See https://github.com/alexedwards/scs?tab=readme-ov-file#working-with-session-data.
	gob.Register(webauthn.SessionData{}) //nolint:exhaustruct // only need to register the struct.

	var rpOrigins []string
	switch {
	case fqdn == "localhost":
		//goland:noinspection HttpUrlsUsage // This is a local server.
		rpOrigins = []string{"http://" + addr}
	case tlsEnabled:
		_, port, _ := net.SplitHostPort(addr)
		if port != "" && port != "443" {
			rpOrigins = []string{"https://" + net.JoinHostPort(fqdn, port)}
		} else {
			rpOrigins = []string{"https://" + fqdn}
		}
	default:
		rpOrigins = []string{"https://" + fqdn}
	}

	var webauthnConfig = &webauthn.Config{
		RPID:          fqdn,
		RPDisplayName: "Petra",
		RPOrigins:     rpOrigins,

		// Top origins are, to my understanding, used for cross-origin Passkeys. We don't need it here.
		RPTopOrigins:                nil,
		RPTopOriginVerificationMode: protocol.TopOriginExplicitVerificationMode,
		RPAllowCrossOrigin:          false,

		Filtering: nil,

		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			AuthenticatorAttachment: "platform",
			RequireResidentKey:      new(true),
			ResidentKey:             protocol.ResidentKeyRequirementRequired,
			UserVerification:        protocol.VerificationDiscouraged,
		},
		Debug:                false,
		EncodeUserIDAsString: false,
		Timeouts: webauthn.TimeoutsConfig{
			Login: webauthn.TimeoutConfig{
				Enforce:    true,
				Timeout:    timeout,
				TimeoutUVD: timeout,
			},
			Registration: webauthn.TimeoutConfig{
				Enforce:    true,
				Timeout:    timeout,
				TimeoutUVD: timeout,
			},
		},
		MDS: nil,
	}

	var webAuthn *webauthn.WebAuthn
	if webAuthn, err = webauthn.New(webauthnConfig); err != nil {
		return nil, fmt.Errorf("new webauthn: %w", err)
	}

	return &WebAuthnHandler{ //nolint:exhaustruct // InternalErrorHandler is wired by the caller after construction.
		logger:         logger,
		webAuthn:       webAuthn,
		sessionManager: sessionManager,
		store:          store,
	}, nil
}

func (h *WebAuthnHandler) BeginRegistration(ctx context.Context) ([]byte, error) {
	var (
		user webauthn.User
		err  error
	)
	if user, err = newRandomUser(); err != nil {
		return nil, fmt.Errorf("new user: %w", err)
	}

	authSelect := protocol.AuthenticatorSelection{
		AuthenticatorAttachment: protocol.Platform,
		RequireResidentKey:      protocol.ResidentKeyNotRequired(),
		ResidentKey:             protocol.ResidentKeyRequirementRequired,
		UserVerification:        protocol.VerificationDiscouraged,
	}

	opts, session, err := h.webAuthn.BeginRegistration(
		user,
		webauthn.WithAuthenticatorSelection(authSelect),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired))
	if err != nil {
		return nil, fmt.Errorf("begin registration: %w", err)
	}

	h.sessionManager.Put(ctx, string(webAuthnSessionKey), *session)
	if err = h.store.upsertUser(ctx, user); err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}

	var out []byte
	if out, err = json.Marshal(opts); err != nil {
		return nil, fmt.Errorf("JSON encode: %w", err)
	}
	return out, nil
}

func (h *WebAuthnHandler) FinishRegistration(r *http.Request) error {
	var (
		err     error
		session webauthn.SessionData
		ctx     = r.Context()
	)

	if session, err = h.parseWebAuthnSession(ctx); err != nil {
		return fmt.Errorf("parse webauthn session: %w", err)
	}

	var user webauthn.User
	if user, err = h.store.getUser(ctx, session.UserID); err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	var credential *webauthn.Credential
	if credential, err = h.webAuthn.FinishRegistration(user, session, r); err != nil {
		return fmt.Errorf("finish webauthn registration: %w", err)
	}

	if err = h.store.upsertCredential(ctx, user.WebAuthnID(), credential); err != nil {
		return fmt.Errorf("upsert webauthn credential: %w", err)
	}

	// Log in the newly registered user
	if err = h.sessionManager.RenewToken(r.Context()); err != nil {
		return fmt.Errorf("renew session token: %w", err)
	}
	h.sessionManager.Put(r.Context(), string(userIDSessionKey), user.WebAuthnID())

	return nil
}

func (h *WebAuthnHandler) BeginLogin(r *http.Request) ([]byte, error) {
	options, session, err := h.webAuthn.BeginDiscoverableLogin()
	if err != nil {
		return nil, fmt.Errorf("begin discoverable webauthn login: %w", err)
	}

	h.sessionManager.Put(r.Context(), string(webAuthnSessionKey), *session)

	var out []byte
	if out, err = json.Marshal(options); err != nil {
		return nil, fmt.Errorf("json marshal webauthn options: %w", err)
	}
	return out, nil
}

func (h *WebAuthnHandler) FinishLogin(r *http.Request) error {
	var (
		session webauthn.SessionData
		err     error
		ctx     = r.Context()
	)
	if session, err = h.parseWebAuthnSession(ctx); err != nil {
		return fmt.Errorf("parse webauthn session: %w", err)
	}

	parsedResponse, err := protocol.ParseCredentialRequestResponse(r)
	if err != nil {
		return fmt.Errorf("parse credential request response: %w", err)
	}

	// Extract credential ID before validation for error reporting.
	credentialID := parsedResponse.RawID

	usr, credential, err := h.webAuthn.ValidatePasskeyLogin(h.findUserHandler(ctx), session, parsedResponse)
	if err != nil {
		// Check if the error is due to an unknown credential or the user not existing.
		_, isUnknownCredentialErr := errors.AsType[*protocol.ErrorUnknownCredential](err)
		if isUnknownCredentialErr || errors.Is(err, ErrUserNotFound) {
			return &UnknownCredentialError{
				CredentialID: credentialID,
				Err:          err,
			}
		}
		return fmt.Errorf("validate Passkey login: %w", err)
	}

	if err = h.store.upsertCredential(ctx, usr.WebAuthnID(), credential); err != nil {
		return fmt.Errorf("upsert webauthn credential: %w", err)
	}

	// Set userID in session
	if err = h.sessionManager.RenewToken(r.Context()); err != nil {
		return fmt.Errorf("renew session token: %w", err)
	}
	h.sessionManager.Put(r.Context(), string(userIDSessionKey), usr.WebAuthnID())

	return nil
}

func (h *WebAuthnHandler) Logout(ctx context.Context) error {
	if err := h.sessionManager.RenewToken(ctx); err != nil {
		return fmt.Errorf("renew session token: %w", err)
	}
	h.sessionManager.Remove(ctx, string(userIDSessionKey))
	return nil
}

func (h *WebAuthnHandler) DeleteUser(ctx context.Context) error {
	userID := h.sessionManager.Get(ctx, string(userIDSessionKey))
	if userID == nil {
		return errors.New("no authenticated user in session")
	}

	userIDBytes, ok := userID.([]byte)
	if !ok {
		return errors.New("invalid user ID type in session")
	}

	if err := h.store.deleteUser(ctx, userIDBytes); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return nil
}

func (h *WebAuthnHandler) parseWebAuthnSession(ctx context.Context) (webauthn.SessionData, error) {
	var (
		session webauthn.SessionData
		ok      bool
		err     error
	)
	ses := h.sessionManager.Get(ctx, string(webAuthnSessionKey))
	if session, ok = ses.(webauthn.SessionData); !ok {
		err = fmt.Errorf("could not parse webauthn.SessionData (data: %v)", ses)
	}
	return session, err
}

func (h *WebAuthnHandler) findUserHandler(ctx context.Context) webauthn.DiscoverableUserHandler {
	return func(_, userID []byte) (webauthn.User, error) {
		return h.store.getUser(ctx, userID)
	}
}

// internalError surfaces a non-recoverable server-side failure. If the
// caller wired InternalErrorHandler (typically app.serverError), it owns
// the response — including stack-navigator-aware navigation to /error.
// Otherwise we fall back to a plain 500.
func (h *WebAuthnHandler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	if h.InternalErrorHandler != nil {
		h.InternalErrorHandler(w, r, err)
		return
	}
	h.logger.LogAttrs(r.Context(), slog.LevelError, "internal error", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
