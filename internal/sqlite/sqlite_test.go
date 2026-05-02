package sqlite_test

import (
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// TestNewDatabase_ReadOnlyHandleRejectsWrites locks the in-memory DSN shape:
// the read-only *sql.DB must reject INSERT/UPDATE statements regardless of
// how the URI's mode= parameters are arranged.
func TestNewDatabase_ReadOnlyHandleRejectsWrites(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("NewDatabase: %v", err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.ReadOnly.ExecContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?)",
		[]byte("ro-test"), "RO Test")
	if err == nil {
		t.Fatal("expected read-only handle to reject INSERT, got nil error")
	}
	// The driver returns an "attempt to write a readonly database" error in some
	// configurations and a query_only-style refusal in others; either is fine.
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "readonly") && !strings.Contains(msg, "read only") &&
		!strings.Contains(msg, "query_only") {
		t.Errorf("expected readonly-style error, got %v", err)
	}
}
