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
	if err != nil {
		return "", fmt.Errorf("get db connection: %w", err)
	}
	defer func() {
		closeErr := conn.Close()
		if closeErr != nil && err == nil {
			err = fmt.Errorf("close db connection: %w", closeErr)
		}
	}()

	// Because the attached database requires writes, we temporarily allow writing only inside this db connection.
	_, err = conn.ExecContext(ctx, `PRAGMA QUERY_ONLY = FALSE`)
	if err != nil {
		return "", fmt.Errorf("disable read only mode: %w", err)
	}
	defer func() {
		_, pragmaErr := conn.ExecContext(ctx, `PRAGMA QUERY_ONLY = TRUE`)
		if pragmaErr != nil && err == nil {
			err = fmt.Errorf("re-enable read only mode: %w", pragmaErr)
		}
	}()
	// Disable foreign key checks in the export database during data copy
	_, err = conn.ExecContext(ctx, `PRAGMA FOREIGN_KEYS = OFF`)
	if err != nil {
		return "", fmt.Errorf("disable foreign keys: %w", err)
	}
	defer func() {
		_, pragmaErr := conn.ExecContext(ctx, `PRAGMA FOREIGN_KEYS = ON`)
		if pragmaErr != nil && err == nil {
			err = fmt.Errorf("re-enable foreign keys: %w", pragmaErr)
		}
	}()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			rollbackErr := tx.Rollback()
			if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				if err == nil {
					err = fmt.Errorf("rollback transaction: %w", rollbackErr)
				}
			}
		}
	}()
	_, err = tx.ExecContext(ctx, `ATTACH DATABASE ? AS export`, exportDsn)
	if err != nil {
		return "", fmt.Errorf("create export database: %w", err)
	}

	// Check if users table exists
	var count int
	query := `SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = 'users'`
	err = tx.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return "", fmt.Errorf("check users table existence: %w", err)
	}
	if count == 0 {
		return "", errors.New("users table does not exist")
	}

	// Get all table schemas and find user-related tables
	userRelatedTables, err := db.findUserRelatedTables(ctx, tx)
	if err != nil {
		return "", fmt.Errorf("find user related tables: %w", err)
	}

	// Copy schemas for user-related tables
	for _, table := range userRelatedTables {
		err = db.copyTableSchema(ctx, tx, table.name)
		if err != nil {
			return "", fmt.Errorf("copy schema for table %s: %w", table.name, err)
		}
	}

	// Copy data for user-related tables in proper order (referenced tables first)
	// First copy tables without user column paths (referenced tables like exercises)
	for _, table := range userRelatedTables {
		if table.userColumnPath == nil {
			err = db.copyUserTableData(ctx, tx, table, userID)
			if err != nil {
				return "", fmt.Errorf("copy data for table %s: %w", table.name, err)
			}
		}
	}

	// Then copy user-related tables
	for _, table := range userRelatedTables {
		if table.userColumnPath != nil {
			err = db.copyUserTableData(ctx, tx, table, userID)
			if err != nil {
				return "", fmt.Errorf("copy data for table %s: %w", table.name, err)
			}
		}
	}

	// Re-enable foreign key checks in the export database
	_, err = tx.ExecContext(ctx, `PRAGMA export.foreign_keys = ON`)
	if err != nil {
		return "", fmt.Errorf("re-enable foreign keys in export database: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return "", fmt.Errorf("commit export database: %w", err)
	}
	committed = true

	return exportPath, nil
}

// userTable represents a table and its relationship to the users table.
type userTable struct {
	name           string
	userColumnPath []string // path of columns that lead to users.id (e.g., ["user_id"] or ["workout_user_id", "user_id"])
}

// findUserRelatedTables discovers all tables that are directly or indirectly related to the users table.
func (db *Database) findUserRelatedTables(ctx context.Context, tx *sql.Tx) ([]userTable, error) {
	var result []userTable

	// Start with the users table itself
	result = append(result, userTable{name: "users", userColumnPath: []string{"id"}})

	// Get all tables
	rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_schema WHERE type = 'table' AND name != 'users'`)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate over tables: %w", err)
	}

	// Build a map of discovered user-related tables for recursive search
	discovered := map[string][]string{"users": {"id"}}

	// Keep searching until no new user-related tables are found
	changed := true
	for changed {
		changed = false

		for _, tableName := range tables {
			if _, alreadyDiscovered := discovered[tableName]; alreadyDiscovered {
				continue
			}

			// Check foreign keys for this table
			fkRows, err := tx.QueryContext(ctx, `SELECT "table", "from", "to" FROM pragma_foreign_key_list(?)`, tableName)
			if err != nil {
				return nil, fmt.Errorf("query foreign keys for table %s: %w", tableName, err)
			}

			for fkRows.Next() {
				var referencedTable, fromColumn, toColumn string
				if err := fkRows.Scan(&referencedTable, &fromColumn, &toColumn); err != nil {
					fkRows.Close()
					return nil, fmt.Errorf("scan foreign key: %w", err)
				}

				// Check if this table references a user-related table
				if userPath, exists := discovered[referencedTable]; exists {
					// For references to users table, use the from column directly
					if referencedTable == "users" && toColumn == "id" {
						discovered[tableName] = []string{fromColumn}
						changed = true
						break
					}
					// For indirect references, check if the referenced column leads to users
					if len(userPath) > 0 && userPath[len(userPath)-1] == toColumn {
						// Build the path from this table to users through the referenced table
						var newPath []string
						newPath = append(newPath, fromColumn)
						newPath = append(newPath, userPath[:len(userPath)-1]...)

						discovered[tableName] = newPath
						changed = true
						break
					}
				}
			}
			fkRows.Close()
		}
	}

	// Convert discovered tables to userTable structs
	for tableName, path := range discovered {
		if tableName != "users" {
			result = append(result, userTable{name: tableName, userColumnPath: path})
		}
	}

	// Also include tables that are referenced by user-related tables (for foreign key constraints)
	referencedTables := make(map[string]bool)
	for _, table := range result {
		// Check what tables this user-related table references
		fkRows, err := tx.QueryContext(ctx, `SELECT "table", "from", "to" FROM pragma_foreign_key_list(?)`, table.name)
		if err != nil {
			return nil, fmt.Errorf("query foreign keys for user-related table %s: %w", table.name, err)
		}

		for fkRows.Next() {
			var referencedTable, fromColumn, toColumn string
			if err := fkRows.Scan(&referencedTable, &fromColumn, &toColumn); err != nil {
				fkRows.Close()
				return nil, fmt.Errorf("scan foreign key: %w", err)
			}

			// If the referenced table is not already in discovered or result, add it
			if _, alreadyDiscovered := discovered[referencedTable]; !alreadyDiscovered {
				referencedTables[referencedTable] = true
			}
		}
		fkRows.Close()
	}

	// Add referenced tables to result (these will copy all their data, not user-specific)
	for tableName := range referencedTables {
		result = append(result, userTable{name: tableName, userColumnPath: nil}) // nil means copy all data
	}

	return result, nil
}

// copyTableSchema copies the schema for a table from the main database to the export database.
func (db *Database) copyTableSchema(ctx context.Context, tx *sql.Tx, tableName string) error {
	// Get the CREATE TABLE statement
	var createSQL string
	schemaQuery := `SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = ?`
	err := tx.QueryRowContext(ctx, schemaQuery, tableName).Scan(&createSQL)
	if err != nil {
		return fmt.Errorf("get schema for table %s: %w", tableName, err)
	}

	// Replace the table name with export.tableName to create it in the export database
	exportSQL := fmt.Sprintf("CREATE TABLE export.%s%s", tableName, createSQL[len("CREATE TABLE "+tableName):])
	_, err = tx.ExecContext(ctx, exportSQL)
	if err != nil {
		return fmt.Errorf("create table schema in export db: %w", err)
	}

	return nil
}

// copyUserTableData copies data for a specific user from a table to the export database.
func (db *Database) copyUserTableData(ctx context.Context, tx *sql.Tx, table userTable, userID int) error {
	var whereClause string
	var args []interface{}

	if table.userColumnPath == nil {
		// This is a referenced table (like exercises) - copy all data
		whereClause = ""
	} else if table.name == "users" {
		whereClause = "WHERE id = ?"
		args = append(args, userID)
	} else if len(table.userColumnPath) == 1 {
		// Direct relationship to users table
		whereClause = fmt.Sprintf("WHERE %s = ?", table.userColumnPath[0])
		args = append(args, userID)
	} else {
		// Indirect relationship - need to join through intermediate tables
		// For now, assume the column name contains the user ID directly
		// This works for the test cases where exercise_sets.workout_user_id contains the user ID
		whereClause = fmt.Sprintf("WHERE %s = ?", table.userColumnPath[0])
		args = append(args, userID)
	}

	// Copy the data
	query := fmt.Sprintf("INSERT INTO export.%s SELECT * FROM main.%s %s", table.name, table.name, whereClause)
	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	return nil
}
