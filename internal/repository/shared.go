package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/sqlite"
)

const (
	timestampFormat = "2006-01-02T15:04:05.000Z"
	dateFormat      = time.DateOnly
)

// queryer is satisfied by both *sql.DB and *sql.Tx, so read helpers can run
// either standalone or inside an open transaction.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// muscleGroups holds the primary/secondary muscle-group names for one exercise.
type muscleGroups struct {
	primary   []string
	secondary []string
}

// fetchMuscleGroupsByExerciseID loads primary/secondary muscle groups for every
// given exercise ID in a single query, keyed by exercise ID. Returns an empty
// map when ids is empty. Shared by the exercise and session repositories so
// neither issues a per-exercise follow-up query.
func fetchMuscleGroupsByExerciseID(
	ctx context.Context,
	q queryer,
	ids []int,
) (_ map[int]muscleGroups, err error) {
	if len(ids) == 0 {
		return map[int]muscleGroups{}, nil
	}

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1] // trim trailing comma
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	query := `
		SELECT emg.exercise_id, mg.name, emg.is_primary
		FROM exercise_muscle_groups emg
		JOIN muscle_groups mg ON emg.muscle_group_name = mg.name
		WHERE emg.exercise_id IN (` + placeholders + `)`

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query muscle groups: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close muscle group rows: %w", closeErr))
		}
	}()

	byExercise := make(map[int]muscleGroups, len(ids))
	for rows.Next() {
		var (
			exerciseID int
			name       string
			isPrimary  bool
		)
		if err = rows.Scan(&exerciseID, &name, &isPrimary); err != nil {
			return nil, fmt.Errorf("scan muscle group row: %w", err)
		}
		g := byExercise[exerciseID]
		if isPrimary {
			g.primary = append(g.primary, name)
		} else {
			g.secondary = append(g.secondary, name)
		}
		byExercise[exerciseID] = g
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate muscle group rows: %w", err)
	}
	return byExercise, nil
}

// baseRepository contains common functionality for all SQLite repositories.
type baseRepository struct {
	db *sqlite.Database
}

func newBaseRepository(db *sqlite.Database) baseRepository {
	return baseRepository{db: db}
}

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

// formatDate formats a time.Time to the canonical YYYY-MM-DD string.
func formatDate(date time.Time) string {
	return date.Format(dateFormat)
}

// formatTimestamp formats a time.Time to the canonical UTC ISO-8601 string.
func formatTimestamp(t time.Time) string {
	return t.UTC().Format(timestampFormat)
}
