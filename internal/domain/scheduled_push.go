package domain

import "time"

// ScheduledPush is a pending Web Push notification persisted so it can be
// replayed after a process restart.
type ScheduledPush struct {
	ID                int
	UserID            int
	WorkoutExerciseID int
	FireAt            time.Time
	Payload           string
	CreatedAt         time.Time
}
