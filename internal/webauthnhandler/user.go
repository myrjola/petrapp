package webauthnhandler

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

type user struct {
	id             int64
	webauthnUserID []byte
	displayName    string
	credentials    []webauthn.Credential
}

const webauthnIDSize = 64

// newRandomUser initialises a new user with random ID and anonymous display name.
func newRandomUser() (user, error) {
	webAuthnUserID := make([]byte, webauthnIDSize)
	if _, err := rand.Read(webAuthnUserID); err != nil {
		return user{}, fmt.Errorf("generate user webauthnUserID: %w", err)
	}

	return user{
		id:             0, // This is populated by the database.
		displayName:    fmt.Sprintf("Anonymous user created at %s", time.Now().Format(time.RFC3339)),
		webauthnUserID: webAuthnUserID,
		credentials:    []webauthn.Credential{},
	}, nil
}

// WebAuthnID provides the user handle of the user account. A user handle is an opaque byte sequence with a maximum
// size of 64 bytes and is not meant to be displayed to the user.
//
// To ensure secure operation, authentication and authorization decisions MUST be made on the basis of this id
// member, not the displayName nor name members. See Section 6.1 of [RFC8266].
//
// It's recommended this value is completely random and uses the entire 64 bytes.
//
// Specification: §5.4.3. User Account Parameters for Credential Generation
// (https://w3c.github.io/webauthn/#dom-publickeycredentialuserentity-id)
func (u user) WebAuthnID() []byte {
	return u.webauthnUserID
}

// WebAuthnName provides the name attribute of the user account during registration and is a human-palatable name for
// the user account, intended only for display. For example, "Alex Müller" or "田中倫". The Relying Party SHOULD let the
// user choose this, and SHOULD NOT restrict the choice more than necessary.
//
// Specification: §5.4.3. User Account Parameters for Credential Generation
// (https://w3c.github.io/webauthn/#dictdef-publickeycredentialuserentity)
func (u user) WebAuthnName() string {
	return u.displayName
}

// WebAuthnDisplayName provides the name attribute of the user account during registration and is a human-palatable
// name for the user account, intended only for display. For example, "Alex Müller" or "田中倫". The Relying Party
// SHOULD let the user choose this, and SHOULD NOT restrict the choice more than necessary.
//
// Specification: §5.4.3. User Account Parameters for Credential Generation
// (https://www.w3.org/TR/webauthn/#dom-publickeycredentialuserentity-displayname)
func (u user) WebAuthnDisplayName() string {
	return u.displayName
}

// WebAuthnCredentials provides the list of [webauthn.Credential] owned by the user.
func (u user) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}
