package domain

import (
	"errors"
	"strings"
)

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

// FieldErrors is the multi-field counterpart to ValidationError: a validation
// failure that carries a separate, user-facing message for each form field
// that failed, plus optional form-level messages tied to no single field.
// Validate methods build one, Add every rule they fail, and return it (via
// OrNil) only when non-empty — so a caller gets EVERY problem at once instead
// of the first.
//
// Fields keys MUST equal the HTML form input `name` attributes so the web
// layer can attach each message to its input (and anchor an error summary to
// it) without a translation table. Handlers detect *FieldErrors with errors.As
// and surface it via the form-error + error-summary flow; see cmd/petra
// README "Error Handling". The zero value is ready to use as a builder
// (`var fe FieldErrors; fe.Add(...); return fe.OrNil()`); the error value it
// produces is the *FieldErrors pointer.
//
//nolint:errname // a collection of field errors; the plural name is intentional.
type FieldErrors struct {
	Fields map[string]string // field name (== HTML name attr) -> message
	Form   []string          // messages tied to no single field
}

// Add records msg for field. The first message for a field wins; later Adds
// for the same field are dropped so each input surfaces exactly one message.
func (fe *FieldErrors) Add(field, msg string) {
	if fe.Fields == nil {
		fe.Fields = make(map[string]string)
	}
	if _, exists := fe.Fields[field]; !exists {
		fe.Fields[field] = msg
	}
}

// AddForm records a form-level message that belongs to no single field.
func (fe *FieldErrors) AddForm(msg string) {
	fe.Form = append(fe.Form, msg)
}

// HasErrors reports whether any field-level or form-level message was recorded.
func (fe *FieldErrors) HasErrors() bool {
	return len(fe.Fields) > 0 || len(fe.Form) > 0
}

// Error implements the error interface with a stable, log-friendly join. The
// UI never shows this string verbatim — it reads Fields / Form — so the
// user-facing wording lives in the per-field messages, not here.
func (fe *FieldErrors) Error() string {
	parts := make([]string, 0, len(fe.Fields)+len(fe.Form))
	for name, msg := range fe.Fields {
		parts = append(parts, name+": "+msg)
	}
	parts = append(parts, fe.Form...)
	return strings.Join(parts, "; ")
}

// OrNil returns the *FieldErrors as an error when it carries any message, else
// nil — the idiomatic "return fe.OrNil()" tail that lets a Validate method stay
// flat. Detect the result with errors.As(err, new(*domain.FieldErrors)).
func (fe *FieldErrors) OrNil() error {
	if fe.HasErrors() {
		return fe
	}
	return nil
}
