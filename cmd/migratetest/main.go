package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func main() {
	if err := run(os.Stdout); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func run(w io.Writer) error {
	logger := testhelpers.NewLogger(w)
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second) //nolint:mnd // 5 seconds
	defer cancel()

	start := time.Now()

	sqliteURL, ok := os.LookupEnv("PETRAPP_SQLITE_URL")
	if !ok {
		logger.LogAttrs(ctx, slog.LevelError, "PETRAPP_SQLITE_URL not set")
		return errors.New("PETRAPP_SQLITE_URL not set")
	}

	db, err := sqlite.NewDatabase(ctx, sqliteURL, logger)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "error creating database",
			slog.String("url", sqliteURL), slog.Any("error", err))
		return fmt.Errorf("create database: %w", err)
	}
	defer func() {
		if err = db.Close(); err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "failed to close database", slog.Any("error", err))
		}
	}()

	// Fetch the number of users from the database and print it out as a simple smoke test.
	row := db.ReadWrite.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`)
	var count int
	if err = row.Scan(&count); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "error fetching user count", slog.Any("error", err))
		return fmt.Errorf("fetch user count: %w", err)
	}
	if count == 0 {
		logger.LogAttrs(ctx, slog.LevelError, "no users found, something is likely wrong")
		return errors.New("no users found")
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "user count", slog.Int("count", count))

	logger.LogAttrs(ctx, slog.LevelInfo, "Migration test successful 🙌", slog.Duration("duration", time.Since(start)))
	return nil
}
