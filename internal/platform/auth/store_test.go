package auth_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/platform/auth"
	"github.com/myrjola/petrapp/internal/platform/sqlitekit"
	"github.com/myrjola/petrapp/internal/platform/testkit"
)

// Compile-time assertion that *SQLiteStore satisfies the Store interface.
var _ auth.Store = (*auth.SQLiteStore)(nil)

func TestSQLiteStore_SatisfiesStore(t *testing.T) {
	t.Parallel()
	logger := testkit.NewLogger(testkit.NewWriter(t))
	// Migrating auth.SchemaSQL on its own validates the auth tables stand
	// alone, and the returned store gives the compile-time _ = auth.Store
	// assertion above a live value to exercise.
	db, err := sqlitekit.NewDatabase(t.Context(), sqlitekit.Config{
		URL:          ":memory:",
		Schema:       auth.SchemaSQL,
		Fixtures:     "",
		Logger:       logger,
		Premigration: nil,
	})
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := auth.NewSQLiteStore(db)
	if store == nil {
		t.Fatal("NewSQLiteStore returned nil")
	}
}
