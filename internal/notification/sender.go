// Package notification owns Web Push delivery: VAPID-signed sends, in-process
// scheduling persisted in SQLite, and the application-level idle monitor that
// allows Fly to scale the Machine to zero between workouts.
package notification

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/myrjola/petrapp/internal/domain"
)

// ErrSubscriptionGone signals that a push endpoint returned 404 or 410. The
// caller should drop the subscription from its store; no retry is appropriate.
var ErrSubscriptionGone = errors.New("notification: subscription gone")

// SenderConfig configures a Sender. VAPIDSubject must be a bare email — the
// webpush-go library v1.4.0 prepends "mailto:" unconditionally, so passing
// "mailto:foo@bar" produces "mailto:mailto:foo@bar" and Apple returns
// BadJwtToken. The test pins this invariant.
type SenderConfig struct {
	VAPIDSubject    string
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	Logger          *slog.Logger
	HTTPClient      *http.Client // optional; defaults to http.DefaultClient when nil.
}

// Sender wraps webpush.SendNotification with the project's VAPID keys and the
// 410/404 → ErrSubscriptionGone translation. Goroutine-safe.
type Sender struct {
	cfg SenderConfig
}

// NewSender constructs a Sender. The config is held by value; later mutations
// to the passed-in struct are not reflected.
func NewSender(cfg SenderConfig) *Sender {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	return &Sender{cfg: cfg}
}

// Send delivers payload to one subscription. The payload is expected to be a
// short JSON object the service worker can parse. Returns ErrSubscriptionGone
// for permanent (404/410) failures so the caller can prune the subscription;
// transient errors are wrapped and returned as-is.
func (s *Sender) Send(ctx context.Context, sub domain.PushSubscription, payload []byte) error {
	subscription := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}

	//nolint:exhaustruct // Topic/RecordSize/CryptoKey/VapidExpiration left at zero values on purpose.
	opts := &webpush.Options{
		HTTPClient:      s.cfg.HTTPClient,
		Subscriber:      s.cfg.VAPIDSubject, // BARE EMAIL — webpush-go prepends mailto: itself.
		VAPIDPublicKey:  s.cfg.VAPIDPublicKey,
		VAPIDPrivateKey: s.cfg.VAPIDPrivateKey,
		TTL:             60, //nolint:mnd // 60 seconds — rest pushes are useless after the next set.
		Urgency:         webpush.UrgencyHigh,
	}
	resp, err := webpush.SendNotificationWithContext(ctx, payload, subscription, opts)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		return ErrSubscriptionGone
	}
	if resp.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:mnd // 1 KiB cap on diagnostic body.
		return fmt.Errorf("push delivery failed: status %d, body: %s",
			resp.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}
