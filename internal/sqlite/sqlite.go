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

//go:embed migrate_user_id_to_integer.sql
var userIdMigration string

type Database struct {
	ReadWrite *sql.DB
	ReadOnly  *sql.DB
	logger    *slog.Logger
}

// NewDatabase connects to a database, migrates the schema, and applies fixtures.
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

	if db, err = connect(ctx, url, logger); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	// Migrate user_id column from BLOB to INTEGER. First, we check the type of the user_id column.
	row := db.ReadWrite.QueryRowContext(ctx, `
		SELECT type 
		FROM PRAGMA_TABLE_INFO('users') 
		WHERE name = 'id';`)
	if row.Err() != nil {
		return nil, fmt.Errorf("check user_id column type: %w", row.Err())
	}
	var userIdColumnType string
	if err := row.Scan(&userIdColumnType); err != nil {
		return nil, fmt.Errorf("scan user_id column type: %w", err)
	}
	if userIdColumnType == "BLOB" {
		now := time.Now()
		if _, err = db.ReadWrite.ExecContext(ctx, userIdMigration); err != nil {
			return nil, fmt.Errorf("user id migration: %w", err)
		}
		duration := time.Since(now)
		logger.LogAttrs(ctx, slog.LevelInfo, "migrating user_id column from BLOB to INTEGER",
			slog.Duration("duration", duration))
	}

	if err = db.migrateTo(ctx, schemaDefinition); err != nil {
		return nil, fmt.Errorf("migrateTo: %w", err)
	}

	// Apply fixtures.
	if _, err = db.ReadWrite.ExecContext(ctx, fixtures); err != nil {
		return nil, fmt.Errorf("apply fixtures: %w", err)
	}

	go db.startDatabaseOptimizer(ctx)

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
	inMemoryConfig := ""
	if isInMemory {
		url = fmt.Sprintf("file:%s", rand.Text())
		inMemoryConfig = "mode=memory&cache=shared"
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
	readConfig := fmt.Sprintf("file:%s?mode=ro&_txlock=deferred&_query_only=true&%s&%s", url, commonConfig, inMemoryConfig)
	readWriteConfig := fmt.Sprintf("file:%s?mode=rwc&_txlock=immediate&%s&%s", url, commonConfig, inMemoryConfig)

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
		ReadWrite: readWriteDB,
		ReadOnly:  readDB,
		logger:    logger,
	}, nil
}

// Close closes the database connections.
func (db *Database) Close() error {
	return errors.Join(db.ReadOnly.Close(), db.ReadWrite.Close())
}
