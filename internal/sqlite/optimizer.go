package sqlite

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// startDatabaseOptimizer runs optimize once per hour. See https://www.sqlite.org/pragma.html#pragma_optimize.
func (db *Database) startDatabaseOptimizer(ctx context.Context) {
	// Recommended performance enhancement for long-lived connections.
	if _, err := db.ReadWrite.ExecContext(ctx, "PRAGMA optimize = 0x10002;"); err != nil {
		err = fmt.Errorf("init optimize database: %w", err)
		db.logger.LogAttrs(ctx, slog.LevelError, "failed to optimize database", slog.Any("error", err))
	}
	for {
		start := time.Now()
		if _, err := db.ReadWrite.ExecContext(ctx, "PRAGMA optimize;"); err != nil {
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
