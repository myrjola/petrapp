package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

func Test_PushSubscribe_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	server, err := e2etest.StartServer(t, testkit.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	client := server.Client()
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}

	body := bytes.NewReader([]byte(`{
        "endpoint": "https://web.push.apple.com/test-endpoint",
        "keys": {"p256dh": "BPa-test", "auth": "auth-test"}
    }`))

	resp, err := postJSON(ctx, client, server.URL()+"/api/push/subscribe", body)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("subscribe status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	_ = resp.Body.Close()

	body = bytes.NewReader([]byte(`{"endpoint":"https://web.push.apple.com/test-endpoint"}`))
	resp, err = postJSON(ctx, client, server.URL()+"/api/push/unsubscribe", body)
	if err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("unsubscribe status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	_ = resp.Body.Close()
}

// Test_PushVAPIDPublicKey_Served verifies the service worker can fetch the
// VAPID public key it needs to re-subscribe on pushsubscriptionchange. The
// key is public (it's handed to every browser as applicationServerKey), so the
// endpoint is reachable without authentication.
func Test_PushVAPIDPublicKey_Served(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	server, err := e2etest.StartServer(t, testkit.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	client := server.Client()

	resp, err := client.Get(ctx, "/api/push/vapid-public-key")
	if err != nil {
		t.Fatalf("get vapid public key: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	key, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(bytes.TrimSpace(key)) == 0 {
		t.Error("vapid public key body is empty")
	}
}

// postJSON uses the e2etest Client's underlying http.Client for a one-off
// POST request. Form-encoded form helpers in e2etest don't fit JSON APIs.
func postJSON(ctx context.Context, c *e2etest.Client, url string, body *bytes.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "fetch")
	resp, err := c.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	return resp, nil
}
