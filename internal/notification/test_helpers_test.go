package notification_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func webpushGenerateKeys() (string, string, error) {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	return priv, pub, err
}

// testValidP256dh returns a base64url-encoded uncompressed P-256 public point
// that passes webpush-go's elliptic.Unmarshal curve check. The corresponding
// private key is discarded — the tests do not decrypt the push payload.
func testValidP256dh(t *testing.T) string {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate P256 key: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes())
}

// extractJWTSubClaim parses the "Authorization: vapid t=<jwt>, k=<base64>" header
// and returns the JWT's "sub" claim. Fails the test on any parse error.
func extractJWTSubClaim(t *testing.T, authHeader string) string {
	t.Helper()
	const prefix = "vapid t="
	if !strings.HasPrefix(authHeader, prefix) {
		t.Fatalf("auth header %q missing %q prefix", authHeader, prefix)
	}
	rest := strings.TrimPrefix(authHeader, prefix)
	jwt := rest
	if before, _, found := strings.Cut(rest, ","); found {
		jwt = strings.TrimSpace(before)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed JWT (%d parts): %q", len(parts), jwt)
	}
	bodyBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT body: %v", err)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err = json.Unmarshal(bodyBytes, &claims); err != nil {
		t.Fatalf("unmarshal JWT body: %v", err)
	}
	return claims.Sub
}
