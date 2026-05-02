package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"

	_ "embed"
)

//go:embed schema.sql
var schemaDefinition string

//go:embed fixtures.sql
var fixtures string

type Database struct {
	ReadWrite *sql.DB
	ReadOnly  *sql.DB
	logger    *slog.Logger
	once      sync.Once

	// cancelOptimizer stops the background optimizer goroutine started by
	// NewDatabase. optimizerDone is closed when that goroutine returns so Close
	// can wait for it before tearing down the connections (and, in tests, before
	// the *testing.T logger sink is closed by t.Cleanup).
	cancelOptimizer context.CancelFunc
	optimizerDone   chan struct{}
}

// NewDatabase connects to a database, migrates the schema, and applies fixtures.
//
// It establishes two database connections, one for read/write operations and one for read-only operations.
// This is the best practice mentioned in https://github.com/mattn/go-sqlite3/issues/1179#issuecomment-1638083995
//
// The url parameter is the path to the SQLite database file or ":memory:" for an in-memory database.
func NewDatabase(ctx context.Context, url string, logger *slog.Logger) (*Database, error) {
	var (
		err error
		db  *Database
	)

	if db, err = connect(ctx, url, logger); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err = db.migrateTo(ctx, schemaDefinition); err != nil {
		return nil, fmt.Errorf("migrateTo: %w", err)
	}

	// Apply fixtures.
	if _, err = db.ReadWrite.ExecContext(ctx, fixtures); err != nil {
		return nil, fmt.Errorf("apply fixtures: %w", err)
	}

	optimizerCtx, cancel := context.WithCancel(ctx) //nolint:gosec // cancel stored on db, invoked in Close.
	db.cancelOptimizer = cancel
	db.optimizerDone = make(chan struct{})
	go func() {
		defer close(db.optimizerDone)
		db.startDatabaseOptimizer(optimizerCtx)
	}()

	return db, nil
}

//nolint:gochecknoglobals // once is used to ensure that the SQLite driver is registered only once.
var once sync.Once

const optimizedDriver = "sqlite3optimized"

// registerOptimizedDriver that executes performance-enhancing pragmas on connection.
func registerOptimizedDriver() {
	sql.Register(optimizedDriver,
		&sqlite3.SQLiteDriver{
			Extensions: nil,
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if _, err := conn.Exec(
					// Performance enhancement by storing temporary tables indices in memory instead of files.
					"PRAGMA temp_store = memory;"+
						// Performance enhancement for reducing syscalls by having the pages in memory-mapped I/O.
						"PRAGMA mmap_size = 30000000000;"+
						// Litestream handles checkpoints.
						// See https://litestream.io/tips/#disable-autocheckpoints-for-high-write-load-servers
						"PRAGMA wal_autocheckpoint = 0;", nil); err != nil {
					return fmt.Errorf("exec optimization pragmas: %w", err)
				}
				return nil
			},
		})
}

func connect(ctx context.Context, url string, logger *slog.Logger) (*Database, error) {
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
	if isInMemory {
		url = rand.Text()
	}
	commonConfig := strings.Join([]string{
		// Uses current time.Location for timestamps.
		"_loc=auto",
		// Makes it possible to temporarily violate foreign key constraints during transactions.
		"_defer_foreign_keys=1",
		// Write-ahead logging enables higher performance and concurrent readers.
		"_journal_mode=wal",
		// Avoids SQLITE_BUSY errors when the database is under load.
		"_busy_timeout=5000",
		// Increases performance at the cost of durability https://www.sqlite.org/pragma.html#pragma_synchronous.
		"_synchronous=normal",
		// Enables foreign key constraints.
		"_foreign_keys=on",
	}, "&")

	// The options without leading underscore are SQLite URI parameters documented at https://www.sqlite.org/uri.html.
	// The options prefixed with underscore '_' are documented at
	// https://pkg.go.dev/github.com/mattn/go-sqlite3#SQLiteDriver.Open.
	//
	// Pick exactly one mode= per URL. SQLite's URI parser silently keeps the last
	// duplicate, so mixing mode=ro/rwc with mode=memory would override our intent.
	// For in-memory we rely on cache=shared + _query_only=true to enforce the
	// read-only handle at the connection level.
	var readMode, readWriteMode string
	if isInMemory {
		readMode = "mode=memory&cache=shared"
		readWriteMode = "mode=memory&cache=shared"
	} else {
		readMode = "mode=ro"
		readWriteMode = "mode=rwc"
	}
	readConfig := fmt.Sprintf("file:%s?%s&_txlock=deferred&_query_only=true&%s", url, readMode, commonConfig)
	readWriteConfig := fmt.Sprintf("file:%s?%s&_txlock=immediate&%s", url, readWriteMode, commonConfig)

	once.Do(registerOptimizedDriver)

	if readWriteDB, err = sql.Open(optimizedDriver, readWriteConfig); err != nil {
		return nil, fmt.Errorf("open read-write database: %w", err)
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "opened database", slog.String("sqlDsn", readWriteConfig))

	readWriteDB.SetMaxOpenConns(1)
	readWriteDB.SetMaxIdleConns(1)
	readWriteDB.SetConnMaxLifetime(time.Hour)
	readWriteDB.SetConnMaxIdleTime(time.Hour)

	// Since sql.DB is lazy, we need to ping it to ensure the connection is established and the database is configured.
	err = readWriteDB.PingContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("ping read-write database: %w", err)
	}

	if readDB, err = sql.Open(optimizedDriver, readConfig); err != nil {
		return nil, fmt.Errorf("open read database: %w", err)
	}

	maxReadConns := 10
	readDB.SetMaxOpenConns(maxReadConns)
	readDB.SetMaxIdleConns(maxReadConns)
	readDB.SetConnMaxLifetime(time.Hour)
	readDB.SetConnMaxIdleTime(time.Hour)

	return &Database{
		ReadWrite:       readWriteDB,
		ReadOnly:        readDB,
		logger:          logger,
		once:            sync.Once{},
		cancelOptimizer: nil,
		optimizerDone:   nil,
	}, nil
}

// Close closes the database connections.
//
// If NewDatabase started the background optimizer goroutine, Close cancels it
// and waits for it to return before closing the underlying *sql.DB handles. The
// connect helper does not start that goroutine, so direct callers in package
// tests still close cleanly.
func (db *Database) Close() error {
	var err error
	db.once.Do(func() {
		if db.cancelOptimizer != nil {
			db.cancelOptimizer()
			<-db.optimizerDone
		}
		err = errors.Join(db.ReadOnly.Close(), db.ReadWrite.Close())
	})
	return err
}
