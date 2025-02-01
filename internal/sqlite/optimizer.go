package sqlite

import (
	"context"
	"github.com/myrjola/petrapp/internal/errors"
	"log/slog"
	"time"
)

// startDatabaseOptimizer runs optimize once per hour. See https://www.sqlite.org/pragma.html#pragma_optimize.
func (db *Database) startDatabaseOptimizer(ctx context.Context) {
	for {
		start := time.Now()
		if _, err := db.ReadWrite.ExecContext(ctx, "PRAGMA optimize;"); err != nil {
			err = errors.Wrap(err, "optimize database")
			db.logger.LogAttrs(ctx, slog.LevelError, "failed to optimize database", errors.SlogError(err))
		} else {
			db.logger.LogAttrs(ctx, slog.LevelInfo, "optimized database",
				slog.Duration("duration", time.Since(start)))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
			continue
		}
	}
}
