package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/sqlite"
)

const (
	timestampFormat = "2006-01-02T15:04:05.000Z"
	dateFormat      = time.DateOnly
)

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
