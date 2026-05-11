package domain

import "time"

// PushSubscription is one device's Web Push subscription. Stored per-user;
// a user may have multiple devices.
type PushSubscription struct {
	ID        int
	UserID    int
	Endpoint  string
	P256dh    string
	Auth      string
	CreatedAt time.Time
}
