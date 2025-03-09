package workout

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/myrjola/petrapp/internal/sqlite"
	"log/slog"
	"time"
)

// Repository defines the complete repository interface.
type Repository struct {
	prefs     PreferencesRepository
	sessions  SessionRepository
	exercises ExerciseRepository
}

// PreferencesRepository handles workout preferences.
type PreferencesRepository interface {
	Get(ctx context.Context) (Preferences, error)
	Set(ctx context.Context, prefs Preferences) error
}

// SessionRepository handles workout sessions.
type SessionRepository interface {
	List(ctx context.Context, sinceDate time.Time) ([]Session, error)
	Get(ctx context.Context, date time.Time) (Session, error)
	Create(ctx context.Context, sess Session) error
	// Update updates an existing session.
	//
	// The updateFn is called with the existing session and if it returns true, the modified sess is persisted.
	Update(ctx context.Context, date time.Time, updateFn func(sess *Session) (bool, error)) error
}

// ExerciseRepository handles exercises and sets.
type ExerciseRepository interface {
	Get(ctx context.Context, id int) (Exercise, error)
	List(ctx context.Context) ([]Exercise, error)
}

// baseRepository contains common functionality for all repositories.
type baseRepository struct {
	db     *sqlite.Database
	logger *slog.Logger
}

// newBaseRepository creates a new base repository.
func newBaseRepository(db *sqlite.Database, logger *slog.Logger) baseRepository {
	return baseRepository{
		db:     db,
		logger: logger,
	}
}

// Constants for date and time formats.
const (
	timestampFormat = "2006-01-02T15:04:05.000Z"
	dateFormat      = time.DateOnly
)

// parseTimestamp parses a timestamp from a nullable database string.
func parseTimestamp(timestampStr sql.NullString) (time.Time, error) {
	if !timestampStr.Valid {
		return time.Time{}, nil
	}

	parsedTime, err := time.Parse(timestampFormat, timestampStr.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp format: %w", err)
	}
	return parsedTime, nil
}

// formatDate formats a time.Time to a date string.
func formatDate(date time.Time) string {
	return date.Format(dateFormat)
}

// formatTimestamp formats a time.Time to a timestamp string.
func formatTimestamp(t time.Time) string {
	return t.UTC().Format(timestampFormat)
}

// RepositoryFactory creates and initializes repositories.
type RepositoryFactory struct {
	db     *sqlite.Database
	logger *slog.Logger
}

// NewRepositoryFactory creates a new repository factory.
func NewRepositoryFactory(db *sqlite.Database, logger *slog.Logger) *RepositoryFactory {
	return &RepositoryFactory{
		db:     db,
		logger: logger,
	}
}

// NewRepository creates a complete repository with all needed implementations.
func (f *RepositoryFactory) NewRepository() *Repository {
	// Create individual repositories
	exerciseRepo := newSQLiteExerciseRepository(f.db, f.logger)
	preferencesRepo := newSQLitePreferencesRepository(f.db, f.logger)
	sessionRepo := newSQLiteSessionRepository(f.db, f.logger, exerciseRepo)

	// Return a composite repository
	return &Repository{
		prefs:     preferencesRepo,
		sessions:  sessionRepo,
		exercises: exerciseRepo,
	}
}

// ErrNotFound is a sentinel error for when a record is not found.
var ErrNotFound = sql.ErrNoRows
