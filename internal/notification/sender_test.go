package notification_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/notification"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

// mintVAPIDKeys returns a fresh base64url-encoded VAPID keypair.
func mintVAPIDKeys(t *testing.T) (string, string) {
	t.Helper()
	priv, pub, err := webpushGenerateKeys()
	if err != nil {
		t.Fatalf("generate vapid keys: %v", err)
	}
	return priv, pub
}

func TestSender_Send_SubjectIsBareEmail(t *testing.T) {
	t.Parallel()
	priv, pub := mintVAPIDKeys(t)

	var capturedAuthHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)

	sender := notification.NewSender(notification.SenderConfig{ //nolint:exhaustruct // HTTPClient defaulted.
		VAPIDSubject:    "vapid@example.com",
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Logger:          testkit.NewLogger(testkit.NewWriter(t)),
	})

	sub := domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt unused by the sender.
		Endpoint: srv.URL + "/wp/abc",
		P256dh:   testValidP256dh(t),
		Auth:     testValidAuth(),
	}
	err := sender.Send(context.Background(), sub, []byte(`{"title":"x"}`))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	subClaim := extractJWTSubClaim(t, capturedAuthHeader)
	if subClaim != "mailto:vapid@example.com" {
		t.Errorf("sub claim = %q, want exactly one mailto: prefix on the bare email", subClaim)
	}
	if strings.Contains(subClaim, "mailto:mailto:") {
		t.Errorf("sub claim has double mailto: prefix: %q", subClaim)
	}
}

func TestSender_Send_410ReturnsErrSubscriptionGone(t *testing.T) {
	t.Parallel()
	priv, pub := mintVAPIDKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	t.Cleanup(srv.Close)

	sender := notification.NewSender(notification.SenderConfig{ //nolint:exhaustruct // HTTPClient defaulted.
		VAPIDSubject:    "vapid@example.com",
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Logger:          testkit.NewLogger(testkit.NewWriter(t)),
	})
	sub := domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt unused by the sender.
		Endpoint: srv.URL,
		P256dh:   testValidP256dh(t),
		Auth:     testValidAuth(),
	}

	err := sender.Send(context.Background(), sub, []byte(`{"title":"x"}`))
	if !errors.Is(err, notification.ErrSubscriptionGone) {
		t.Errorf("Send returned %v, want ErrSubscriptionGone", err)
	}
}

func TestSender_Send_5xxReturnsErrorButNotGone(t *testing.T) {
	t.Parallel()
	priv, pub := mintVAPIDKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	sender := notification.NewSender(notification.SenderConfig{ //nolint:exhaustruct // HTTPClient defaulted.
		VAPIDSubject:    "vapid@example.com",
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Logger:          testkit.NewLogger(testkit.NewWriter(t)),
	})
	sub := domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt unused by the sender.
		Endpoint: srv.URL,
		P256dh:   testValidP256dh(t),
		Auth:     testValidAuth(),
	}

	err := sender.Send(context.Background(), sub, []byte(`{"title":"x"}`))
	if err == nil {
		t.Fatal("Send returned nil for 5xx, want error")
	}
	if errors.Is(err, notification.ErrSubscriptionGone) {
		t.Errorf("Send returned ErrSubscriptionGone for 5xx; should be a transient error")
	}
}

// testValidAuth returns a base64url-encoded 16-byte auth secret. The exact
// bytes do not matter — the httptest.Server in these tests does not verify
// the encryption.
func testValidAuth() string {
	return base64.RawURLEncoding.EncodeToString(make([]byte, 16))
}
