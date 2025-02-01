package repositories_test

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/myrjola/petrapp/internal/sqlite"
	"log/slog"
	"os"
	"testing"
)

//go:embed testdata/fixtures.sql
var testFixtures string

// newTestDB creates a new in-memory database for testing purposes.
func newTestDB(t *testing.T, logger *slog.Logger) *sqlite.Database {
	var (
		database *sqlite.Database
		err      error
	)
	t.Helper()

	if database, err = sqlite.NewDatabase(context.Background(), ":memory:", logger); err != nil {
		t.Fatal(err)
	}

	// Add test data
	if _, err = database.ReadWrite.Exec(testFixtures); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		defer func() {
			if err = database.ReadWrite.Close(); err != nil {
				t.Fatal(err)
			}
			if err = database.ReadOnly.Close(); err != nil {
				t.Fatal(err)
			}
		}()
	})

	return database
}

// newBenchmarkDB creates database connection pools for benchmarking purposes.
func newBenchmarkDB(b *testing.B, logger *slog.Logger) *sqlite.Database {
	var (
		dbs             *sqlite.Database
		err             error
		benchmarkDBPath = "./benchmark.sqlite"
	)

	if dbs, err = sqlite.NewDatabase(context.Background(), benchmarkDBPath, logger); err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() {
		if err = dbs.ReadWrite.Close(); err != nil {
			b.Fatal(err)
		}
		if err = dbs.ReadOnly.Close(); err != nil {
			b.Fatal(err)
		}
		_ = os.Remove(benchmarkDBPath)
		_ = os.Remove(fmt.Sprintf("%s-shm", benchmarkDBPath))
		_ = os.Remove(fmt.Sprintf("%s-wal", benchmarkDBPath))
	})

	return dbs
}
