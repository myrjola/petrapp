package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/myrjola/petrapp/internal/petra/domain"
)

const pushBodyMaxBytes = 4096 // tiny JSON body; subscriptions are ~1 KB max.

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

func (app *application) pushSubscribePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, pushBodyMaxBytes)
	var req pushSubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.serverError(w, r, fmt.Errorf("decode subscribe body: %w", err))
		return
	}
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		app.serverError(w, r, errors.New("missing subscription fields"))
		return
	}
	sub := domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt populated by repository.
		Endpoint: req.Endpoint,
		P256dh:   req.Keys.P256dh,
		Auth:     req.Keys.Auth,
	}
	if _, err := app.service.UpsertPushSubscription(r.Context(), sub); err != nil {
		app.serverError(w, r, fmt.Errorf("upsert subscription: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pushVAPIDPublicKeyGET serves the VAPID public key as plain text so the
// service worker can re-subscribe on a pushsubscriptionchange event — when it
// runs there is no page to read the key from. The key is public (it's the
// applicationServerKey handed to every browser at subscribe time), so this
// endpoint needs no authentication.
func (app *application) pushVAPIDPublicKeyGET(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if _, err := io.WriteString(w, app.vapidPublicKey); err != nil {
		app.serverError(w, r, fmt.Errorf("write vapid public key: %w", err))
		return
	}
}

func (app *application) pushUnsubscribePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, pushBodyMaxBytes)
	var req pushUnsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.serverError(w, r, fmt.Errorf("decode unsubscribe body: %w", err))
		return
	}
	if err := app.service.DeletePushSubscription(r.Context(), req.Endpoint); err != nil {
		app.serverError(w, r, fmt.Errorf("delete subscription: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
