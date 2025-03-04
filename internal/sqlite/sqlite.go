package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/myrjola/petrapp/internal/errors"
	"github.com/myrjola/petrapp/internal/random"
	"log/slog"
	"strings"
	"time"

	_ "embed"
	_ "github.com/mattn/go-sqlite3" // Enable sqlite3 driver
)

//go:embed schema.sql
var schemaDefinition string

//go:embed fixtures.sql
var fixtures string

type Database struct {
	ReadWrite *sql.DB
	ReadOnly  *sql.DB
	logger    *slog.Logger
}

// NewDatabase connects to database, migrates the schema, and applies fixtures.
//
// It establishes two database connections, one for read/write operations and one for read-only operations.
// This is a best practice mentioned in https://github.com/mattn/go-sqlite3/issues/1179#issuecomment-1638083995
//
// The url parameter is the path to the SQLite database file or ":memory:" for an in-memory database.
func NewDatabase(ctx context.Context, url string, logger *slog.Logger) (*Database, error) {
	var (
		err error
		db  *Database
	)

	if db, err = connect(url, logger); err != nil {
		return nil, errors.Wrap(err, "connect")
	}

	if err = db.migrateTo(ctx, schemaDefinition); err != nil {
		return nil, errors.Wrap(err, "migrateTo")
	}

	// Apply fixtures.
	if _, err = db.ReadWrite.ExecContext(ctx, fixtures); err != nil {
		return nil, errors.Wrap(err, "apply fixtures")
	}

	go db.startDatabaseOptimizer(ctx)

	return db, nil
}

func connect(url string, logger *slog.Logger) (*Database, error) {
	var (
		err         error
		readWriteDB *sql.DB
		readDB      *sql.DB
	)

	// For in-memory databases, we need shared cache mode so that both databases access the same data.
	//
	// For parallel tests, we need to use a different database file for each test to avoid sharing data.
	// See https://www.sqlite.org/inmemorydb.html.
	isInMemory := strings.Contains(url, ":memory:")
	inMemoryConfig := ""
	if isInMemory {
		var (
			randomID     string
			dbNameLength uint = 20
		)
		if randomID, err = random.Letters(dbNameLength); err != nil {
			return nil, errors.Wrap(err, "generate random ID")
		}
		url = fmt.Sprintf("file:%s", randomID)
		inMemoryConfig = "mode=memory&cache=shared"
	}
	//nolint:godox // temporary todo
	// TODO: Many of these don't work. Would need a connection hook instead.
	//       See https://pkg.go.dev/github.com/mattn/go-sqlite3#hdr-Connection_Hook
	//       and https://github.com/mattn/go-sqlite3/issues/1248#issuecomment-2227586113
	//       also consider adding a logging or duration hook.
	commonConfig := strings.Join([]string{
		// Write-ahead logging enables higher performance and concurrent readers.
		"_journal_mode=wal",
		// Avoids SQLITE_BUSY errors when database is under load.
		"_busy_timeout=5000",
		// Increases performance at the cost of durability https://www.sqlite.org/pragma.html#pragma_synchronous.
		"_synchronous=normal",
		// Enables foreign key constraints.
		"_foreign_keys=on",
		// Performance enhancement by storing temporary tables indices in memory instead of files.
		"_temp_store=memory",
		// Performance enhancement for reducing syscalls by having the pages in memory-mapped I/O.
		"_mmap_size=30000000000",
		// Recommended performance enhancement for long-lived connections.
		// See https://www.sqlite.org/pragma.html#pragma_optimize.
		"_optimize=0x10002",
		// Litestream handles checkpoints.
		// See https://litestream.io/tips/#disable-autocheckpoints-for-high-write-load-servers
		"_wal_autocheckpoint = 0",
	}, "&")

	// The options prefixed with underscore '_' are SQLite pragmas documented at https://www.sqlite.org/pragma.html.
	// The options without leading underscore are SQLite URI parameters documented at https://www.sqlite.org/uri.html.
	readConfig := fmt.Sprintf("file:%s?mode=ro&_txlock=deferred&_query_only=true&%s&%s", url, commonConfig, inMemoryConfig)
	readWriteConfig := fmt.Sprintf("file:%s?mode=rwc&_txlock=immediate&%s&%s", url, commonConfig, inMemoryConfig)

	if readWriteDB, err = sql.Open("sqlite3", readWriteConfig); err != nil {
		return nil, errors.Wrap(err, "open read-write database")
	}

	readWriteDB.SetMaxOpenConns(1)
	readWriteDB.SetMaxIdleConns(1)
	readWriteDB.SetConnMaxLifetime(time.Hour)
	readWriteDB.SetConnMaxIdleTime(time.Hour)

	if readDB, err = sql.Open("sqlite3", readConfig); err != nil {
		return nil, errors.Wrap(err, "open read database")
	}

	maxReadConns := 10
	readDB.SetMaxOpenConns(maxReadConns)
	readDB.SetMaxIdleConns(maxReadConns)
	readDB.SetConnMaxLifetime(time.Hour)
	readDB.SetConnMaxIdleTime(time.Hour)

	return &Database{
		ReadWrite: readWriteDB,
		ReadOnly:  readDB,
		logger:    logger,
	}, nil
}

// Close closes the database connections.
func (db *Database) Close() error {
	return errors.Join(db.ReadOnly.Close(), db.ReadWrite.Close())
}
