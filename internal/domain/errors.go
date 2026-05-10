package domain

import "errors"

// ErrNotFound is returned by repositories when a requested record does not
// exist. It is intentionally NOT aliased to sql.ErrNoRows — repositories
// translate the SQL error at their boundary so the domain stays free of
// persistence concerns.
var ErrNotFound = errors.New("not found")

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
