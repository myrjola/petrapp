package sqlite

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// startDatabaseOptimizer runs optimize once per hour. See https://www.sqlite.org/pragma.html#pragma_optimize.
//
// Logging is skipped after ctx is cancelled. Otherwise a fast-finishing test can race
// the goroutine: t.Context() is cancelled just before t.Cleanup runs, ExecContext returns
// a cancellation error, and the goroutine writes to the logger after the testwriter sink
// has been torn down.
func (db *Database) startDatabaseOptimizer(ctx context.Context) {
	// Recommended performance enhancement for long-lived connections.
	if _, err := db.ReadWrite.ExecContext(ctx, "PRAGMA optimize = 0x10002;"); err != nil && ctx.Err() == nil {
		err = fmt.Errorf("init optimize database: %w", err)
		db.logger.LogAttrs(ctx, slog.LevelError, "failed to optimize database", slog.Any("error", err))
	}
	for {
		if ctx.Err() != nil {
			return
		}
		start := time.Now()
		_, err := db.ReadWrite.ExecContext(ctx, "PRAGMA optimize;")
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			err = fmt.Errorf("optimize database: %w", err)
			db.logger.LogAttrs(ctx, slog.LevelError, "failed to optimize database", slog.Any("error", err))
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
