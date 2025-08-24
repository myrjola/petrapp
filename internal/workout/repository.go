package workout

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/myrjola/petrapp/internal/sqlite"
)

// repository contains the repositories for the domain-driven design aggregates.
type repository struct {
	prefs        preferencesRepository
	sessions     sessionRepository
	exercises    exerciseRepository
	featureFlags featureFlagRepository
}

// preferencesRepository handles workout preferences.
type preferencesRepository interface {
	Get(ctx context.Context) (Preferences, error)
	Set(ctx context.Context, prefs Preferences) error
}

// exerciseSetAggregate groups all sets for a specific exercise in a workout.
type exerciseSetAggregate struct {
	ExerciseID        int
	Sets              []Set
	WarmupCompletedAt *time.Time // Nullable timestamp when warmup for this exercise was completed
}

// sessionAggregate represents a complete workout session including all exercises and their sets.
type sessionAggregate struct {
	Date             time.Time
	DifficultyRating *int
	StartedAt        time.Time
	CompletedAt      time.Time
	ExerciseSets     []exerciseSetAggregate
}

// sessionRepository handles workout sessions.
type sessionRepository interface {
	List(ctx context.Context, sinceDate time.Time) ([]sessionAggregate, error)
	Get(ctx context.Context, date time.Time) (sessionAggregate, error)
	Create(ctx context.Context, sess sessionAggregate) error
	// Update updates an existing session.
	//
	// The updateFn is called with the existing session and if it returns true, the modified sess is persisted.
	Update(ctx context.Context, date time.Time, updateFn func(sess *sessionAggregate) (bool, error)) error
}

// exerciseRepository handles exercises and sets.
type exerciseRepository interface {
	Get(ctx context.Context, id int) (Exercise, error)
	List(ctx context.Context) ([]Exercise, error)
	Create(ctx context.Context, ex Exercise) (Exercise, error)
	// Update updates an existing exercise.
	//
	// The updateFn is called with the existing exercise and if it returns true, the modified ex is persisted.
	Update(ctx context.Context, exerciseID int, updateFn func(ex *Exercise) (bool, error)) error
	// ListMuscleGroups retrieves all available muscle groups.
	ListMuscleGroups(ctx context.Context) ([]string, error)
}

// featureFlagRepository handles feature flags.
type featureFlagRepository interface {
	Get(ctx context.Context, name string) (FeatureFlag, error)
	Set(ctx context.Context, flag FeatureFlag) error
}

// baseRepository contains common functionality for all repositories.
type baseRepository struct {
	db *sqlite.Database
}

// newBaseRepository creates a new base repository.
func newBaseRepository(db *sqlite.Database) baseRepository {
	return baseRepository{
		db: db,
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

// repositoryFactory creates and initializes repositories.
type repositoryFactory struct {
	db     *sqlite.Database
	logger *slog.Logger
}

// newRepositoryFactory creates a new repository factory.
func newRepositoryFactory(db *sqlite.Database, logger *slog.Logger) *repositoryFactory {
	return &repositoryFactory{
		db:     db,
		logger: logger,
	}
}

// newRepository creates a complete repository with all needed implementations.
func (f *repositoryFactory) newRepository() *repository {
	// Create individual repositories
	exerciseRepo := newSQLiteExerciseRepository(f.db)
	preferencesRepo := newSQLitePreferenceRepository(f.db)
	sessionRepo := newSQLiteSessionRepository(f.db)
	featureFlagRepo := newSQLiteFeatureFlagRepository(f.db)

	// Return a composite repository
	return &repository{
		prefs:        preferencesRepo,
		sessions:     sessionRepo,
		exercises:    exerciseRepo,
		featureFlags: featureFlagRepo,
	}
}

// ErrNotFound is a sentinel error for when a record is not found.
var ErrNotFound = sql.ErrNoRows
