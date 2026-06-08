package sqlitekit

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestDatabase_HealthCheck(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	db, err := NewDatabase(ctx, Config{
		URL:          ":memory:",
		Schema:       "CREATE TABLE t (id INTEGER PRIMARY KEY);",
		Fixtures:     "",
		Logger:       logger,
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}

	if err = db.HealthCheck(ctx); err != nil {
		t.Errorf("HealthCheck on open database: got %v, want nil", err)
	}

	if err = db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err = db.HealthCheck(ctx); err == nil {
		t.Error("HealthCheck on closed database: got nil, want error")
	}
}
