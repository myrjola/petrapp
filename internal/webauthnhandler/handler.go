package webauthnhandler

import (
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"github.com/alexedwards/scs/v2"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/ptr"
	"github.com/myrjola/petrapp/internal/sqlite"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type WebAuthnHandler struct {
	logger         *slog.Logger
	webAuthn       *webauthn.WebAuthn
	sessionManager *scs.SessionManager
	database       *sqlite.Database
}

func New(
	addr string,
	fqdn string,
	logger *slog.Logger,
	sessionManager *scs.SessionManager,
	dbs *sqlite.Database,
) (*WebAuthnHandler, error) {
	var (
		err     error
		timeout = time.Minute * 5
		init    sync.Once
	)
	// Register the session data struct for encoding to the session.
	// See https://github.com/alexedwards/scs?tab=readme-ov-file#working-with-session-data.
	init.Do(func() {
		gob.Register(webauthn.SessionData{}) //nolint:exhaustruct // only need to register the struct.
	})

	rpOrigins := []string{fmt.Sprintf("https://%s", fqdn)}
	if fqdn == "localhost" {
		//goland:noinspection HttpUrlsUsage // This is a local server.
		rpOrigins = []string{fmt.Sprintf("http://%s", addr)}
	}

	var webauthnConfig = &webauthn.Config{
		RPID:          fqdn,
		RPDisplayName: "Petrapp",
		RPOrigins:     rpOrigins,

		// Top origins are to my understanding used for cross-origin Passkeys. We don't need it here.
		RPTopOrigins:                nil,
		RPTopOriginVerificationMode: protocol.TopOriginIgnoreVerificationMode,

		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			AuthenticatorAttachment: "platform",
			RequireResidentKey:      ptr.Ref(true),
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
		return nil, errors.Wrap(err, "new webauthn")
	}

	return &WebAuthnHandler{
		logger:         logger,
		webAuthn:       webAuthn,
		sessionManager: sessionManager,
		database:       dbs,
	}, nil
}

func (h *WebAuthnHandler) BeginRegistration(ctx context.Context) ([]byte, error) {
	var (
		user webauthn.User
		err  error
	)
	if user, err = newRandomUser(); err != nil {
		return nil, errors.Wrap(err, "new user")
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
		return nil, errors.Wrap(err, "begin registration")
	}

	h.sessionManager.Put(ctx, string(webAuthnSessionKey), *session)
	if err = h.upsertUser(ctx, user); err != nil {
		return nil, errors.Wrap(err, "upsert user")
	}

	var out []byte
	if out, err = json.Marshal(opts); err != nil {
		return nil, errors.Wrap(err, "JSON encode")
	}
	return out, nil
}

func (h *WebAuthnHandler) parseWebAuthnSession(ctx context.Context) (webauthn.SessionData, error) {
	var (
		session webauthn.SessionData
		ok      bool
		err     error
	)
	ses := h.sessionManager.Get(ctx, string(webAuthnSessionKey))
	if session, ok = ses.(webauthn.SessionData); !ok {
		err = errors.New("could not parse webauthn.SessionData", slog.Any("data", ses))
	}
	return session, err
}

func (h *WebAuthnHandler) FinishRegistration(r *http.Request) error {
	var (
		err     error
		session webauthn.SessionData
		ctx     = r.Context()
	)

	if session, err = h.parseWebAuthnSession(ctx); err != nil {
		return errors.Wrap(err, "parse webauthn session")
	}

	var user webauthn.User
	if user, err = h.getUser(ctx, session.UserID); err != nil {
		return errors.Wrap(err, "get user")
	}

	var credential *webauthn.Credential
	if credential, err = h.webAuthn.FinishRegistration(user, session, r); err != nil {
		return errors.Wrap(err, "finish webauthn registration")
	}

	if err = h.upsertCredential(ctx, user.WebAuthnID(), credential); err != nil {
		return errors.Wrap(err, "upsert webauthn credential")
	}

	// Log in the newly registered user
	if err = h.sessionManager.RenewToken(r.Context()); err != nil {
		return errors.Wrap(err, "renew session token")
	}
	h.sessionManager.Put(r.Context(), string(userIDSessionKey), user.WebAuthnID())

	return nil
}

func (h *WebAuthnHandler) BeginLogin(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	options, session, err := h.webAuthn.BeginDiscoverableLogin()
	if err != nil {
		return nil, errors.Wrap(err, "begin discoverable webauthn login")
	}

	h.sessionManager.Put(r.Context(), string(webAuthnSessionKey), *session)

	w.Header().Set("Content-Type", "application/json")
	var out []byte
	if out, err = json.Marshal(options); err != nil {
		return nil, errors.Wrap(err, "json marshal webauthn options")
	}
	return out, nil
}

func (h *WebAuthnHandler) findUserHandler(ctx context.Context) webauthn.DiscoverableUserHandler {
	return func(_, userID []byte) (webauthn.User, error) {
		return h.getUser(ctx, userID)
	}
}

func (h *WebAuthnHandler) FinishLogin(r *http.Request) error {
	var (
		session webauthn.SessionData
		err     error
		user    webauthn.User
		ctx     = r.Context()
	)
	if session, err = h.parseWebAuthnSession(ctx); err != nil {
		return errors.Wrap(err, "parse webauthn session")
	}

	parsedResponse, err := protocol.ParseCredentialRequestResponse(r)
	if err != nil {
		return errors.Wrap(err, "parse credential request response")
	}
	user, credential, err := h.webAuthn.ValidatePasskeyLogin(h.findUserHandler(ctx), session, parsedResponse)
	if err != nil {
		return errors.Wrap(err, "validate Passkey login")
	}

	if err = h.upsertCredential(ctx, user.WebAuthnID(), credential); err != nil {
		return errors.Wrap(err, "upsert webauthn credential")
	}

	// Set userID in session
	if err = h.sessionManager.RenewToken(r.Context()); err != nil {
		return errors.Wrap(err, "renew session token")
	}
	h.sessionManager.Put(r.Context(), string(userIDSessionKey), user.WebAuthnID())

	return nil
}

func (h *WebAuthnHandler) Logout(ctx context.Context) error {
	if err := h.sessionManager.RenewToken(ctx); err != nil {
		return errors.Wrap(err, "renew session token")
	}
	h.sessionManager.Remove(ctx, string(userIDSessionKey))
	return nil
}
