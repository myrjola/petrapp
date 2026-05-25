package domain

import "time"

// ScheduledPush is a pending Web Push notification persisted so it can be
// replayed after a process restart. The slot it belongs to is identified by
// the composite (UserID, WorkoutDate, Position).
type ScheduledPush struct {
	ID          int
	UserID      int
	WorkoutDate time.Time
	Position    int
	FireAt      time.Time
	Payload     string
	CreatedAt   time.Time
}
