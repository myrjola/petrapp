package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
)

// createUserDB exports the data for a specific user into a separate SQLite database file.
//
// This can be used for providing the user with all their data to comply with GDPR.
func (db *Database) createUserDB(ctx context.Context, userID int, basePath string) (dbPath string, err error) {
	exportPath := filepath.Join(basePath, fmt.Sprintf("user-db-%d.sqlite3", userID))
	// TODO: ponder on arguments to use such as foreign_keys=on etc.
	exportDsn := fmt.Sprintf("file:%s?mode=rwc", exportPath)

	var conn *sql.Conn
	conn, err = db.ReadOnly.Conn(ctx)
	defer func() {
		err = conn.Close()
		if err != nil {
			err = fmt.Errorf("close db connection: %v", err)
		}
	}()

	// Because the attached database requires writes, we temporarily allow writing only inside this db connection.
	_, err = conn.ExecContext(ctx, `PRAGMA QUERY_ONLY = FALSE`)
	if err != nil {
		return "", fmt.Errorf("disable read only mode: %v", err)
	}
	defer func() {
		_, err = conn.ExecContext(ctx, `PRAGMA QUERY_ONLY = TRUE`)
		if err != nil {
			err = fmt.Errorf("re-enable read only mode: %v", err)
		}
	}()

	tx, _ := conn.BeginTx(ctx, nil)
	defer func(tx *sql.Tx) {
		err = tx.Rollback()
		if err != nil && !errors.Is(err, sql.ErrTxDone) {
			err = fmt.Errorf("rollback transaction: %v", err)
		} else {
			err = nil
		}
	}(tx)
	_, err = tx.ExecContext(ctx, `ATTACH DATABASE ? AS export`, exportDsn)
	if err != nil {
		return "", fmt.Errorf("create export database: %v", err)
	}

	// TODO: Create tables and copy the data over for specific user ID.

	err = tx.Commit()
	if err != nil {
		return "", fmt.Errorf("commit export database: %v", err)
	}

	return exportPath, nil
}
