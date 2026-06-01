package sqlite

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
	db, err := NewDatabase(ctx, ":memory:", logger)
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
