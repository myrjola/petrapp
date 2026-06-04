package domain

import "errors"

// ErrNotFound is returned by repositories when a requested record does not
// exist. It is intentionally NOT aliased to sql.ErrNoRows — repositories
// translate the SQL error at their boundary so the domain stays free of
// persistence concerns.
var ErrNotFound = errors.New("not found")

// ErrAlreadyExists is returned by repositories when an insert would violate
// a uniqueness constraint (e.g. inserting a workout_sessions row for a date
// the user already has). Callers use errors.Is to fall through to the
// "already there" code path (idempotent retry, lazy-create race recovery).
var ErrAlreadyExists = errors.New("already exists")

// Aggregate-method sentinels. Each is returned by a Session method when an
// invariant is violated; callers use errors.Is to branch.
var (
	ErrAlreadyStarted           = errors.New("session already started")
	ErrNotStarted               = errors.New("session not started")
	ErrSlotNotFound             = errors.New("workout exercise slot not found")
	ErrSetIndexOutOfBounds      = errors.New("set index out of bounds")
	ErrExerciseAlreadyInSession = errors.New("exercise already in session")
	ErrInvalidDifficultyRating  = errors.New("difficulty rating must be 1-5")
)

// ValidationError is a domain validation failure carrying a message that is
// safe to surface directly to the end user. Handlers detect it with
// errors.As and surface it via putFlashError + redirect-to-form; see
// cmd/petra/CLAUDE.md for the full flow.
type ValidationError struct {
	Message string
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	return e.Message
}
