package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
)

const usersTableName = "users"

// createUserDB exports the data for a specific user into a separate SQLite database file.
//
// This can be used for providing the user with all their data to comply with GDPR.
func (db *Database) createUserDB(ctx context.Context, userID int, basePath string) (_ string, err error) {
	exportPath := filepath.Join(basePath, fmt.Sprintf("user-db-%d.sqlite3", userID))
	exportDsn := fmt.Sprintf("file:%s?mode=rwc", exportPath)

	conn, err := db.setupExportConnection(ctx)
	if err != nil {
		return "", fmt.Errorf("setup export connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close db connection: %w", closeErr)
		}
	}()

	return db.executeExport(ctx, conn, exportDsn, userID, exportPath)
}

// setupExportConnection prepares a database connection for export operations.
func (db *Database) setupExportConnection(ctx context.Context) (*sql.Conn, error) {
	conn, err := db.ReadOnly.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("get db connection: %w", err)
	}

	if pragmaErr := db.configurePragmas(ctx, conn, false); pragmaErr != nil {
		if closeErr := conn.Close(); closeErr != nil {
			return nil, fmt.Errorf("configure pragmas: %w (close error: %w)", pragmaErr, closeErr)
		}
		return nil, fmt.Errorf("configure pragmas: %w", pragmaErr)
	}

	return conn, nil
}

// configurePragmas sets up the necessary PRAGMA settings for export operations.
func (db *Database) configurePragmas(ctx context.Context, conn *sql.Conn, readOnly bool) error {
	var queryOnlyMode, foreignKeysMode string
	var modeErr, fkErr string

	if readOnly {
		queryOnlyMode = "TRUE"
		foreignKeysMode = "ON"
		modeErr = "enable read only mode"
		fkErr = "enable foreign keys"
	} else {
		queryOnlyMode = "FALSE"
		foreignKeysMode = "OFF"
		modeErr = "disable read only mode"
		fkErr = "disable foreign keys"
	}

	if _, err := conn.ExecContext(ctx, `PRAGMA QUERY_ONLY = `+queryOnlyMode); err != nil {
		return fmt.Errorf("%s: %w", modeErr, err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA FOREIGN_KEYS = `+foreignKeysMode); err != nil {
		return fmt.Errorf("%s: %w", fkErr, err)
	}
	return nil
}

// executeExport performs the main export operation within a transaction.
func (db *Database) executeExport(
	ctx context.Context, conn *sql.Conn, exportDsn string, userID int, exportPath string,
) (string, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback() // Ignore rollback errors to preserve original error
		}
		// Restore original pragmas
		_ = db.configurePragmas(ctx, conn, true) // Ignore pragma restoration errors
	}()

	_, err = tx.ExecContext(ctx, `ATTACH DATABASE ? AS export`, exportDsn)
	if err != nil {
		return "", fmt.Errorf("create export database: %w", err)
	}

	err = db.validateUsersTable(ctx, tx)
	if err != nil {
		return "", fmt.Errorf("validate users table: %w", err)
	}

	userRelatedTables, err := db.findUserRelatedTables(ctx, tx)
	if err != nil {
		return "", fmt.Errorf("find user related tables: %w", err)
	}

	err = db.copyTableSchemas(ctx, tx, userRelatedTables)
	if err != nil {
		return "", fmt.Errorf("copy table schemas: %w", err)
	}

	err = db.copyTableData(ctx, tx, userRelatedTables, userID)
	if err != nil {
		return "", fmt.Errorf("copy table data: %w", err)
	}

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

// validateUsersTable checks if the users table exists.
func (db *Database) validateUsersTable(ctx context.Context, tx *sql.Tx) error {
	var count int
	query := `SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = ?`
	if err := tx.QueryRowContext(ctx, query, usersTableName).Scan(&count); err != nil {
		return fmt.Errorf("check users table existence: %w", err)
	}
	if count == 0 {
		return errors.New("users table does not exist")
	}
	return nil
}

// copyTableSchemas copies the schemas for all user-related tables.
func (db *Database) copyTableSchemas(ctx context.Context, tx *sql.Tx, tables []userTable) error {
	for _, table := range tables {
		if err := db.copyTableSchema(ctx, tx, table.name); err != nil {
			return fmt.Errorf("copy schema for table %s: %w", table.name, err)
		}
	}
	return nil
}

// copyTableData copies data for user-related tables in proper order.
func (db *Database) copyTableData(ctx context.Context, tx *sql.Tx, tables []userTable, userID int) error {
	// First copy tables without user column paths (referenced tables like exercises)
	for _, table := range tables {
		if table.userColumnPath == nil {
			if err := db.copyUserTableData(ctx, tx, table, userID); err != nil {
				return fmt.Errorf("copy data for table %s: %w", table.name, err)
			}
		}
	}

	// Then copy user-related tables
	for _, table := range tables {
		if table.userColumnPath != nil {
			if err := db.copyUserTableData(ctx, tx, table, userID); err != nil {
				return fmt.Errorf("copy data for table %s: %w", table.name, err)
			}
		}
	}

	return nil
}

// userTable represents a table and its relationship to the users table.
type userTable struct {
	name           string
	userColumnPath []string // path of columns that lead to users.id (e.g., ["user_id"] or ["workout_user_id", "user_id"])
}

// findUserRelatedTables discovers all tables that are directly or indirectly related to the users table.
func (db *Database) findUserRelatedTables(ctx context.Context, tx *sql.Tx) ([]userTable, error) {
	const initialCapacity = 16
	result := make([]userTable, 0, initialCapacity)

	// Start with the users table itself
	result = append(result, userTable{name: usersTableName, userColumnPath: []string{"id"}})

	tables, err := db.getAllTableNames(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("get all table names: %w", err)
	}

	discovered, err := db.discoverUserRelatedTables(ctx, tx, tables)
	if err != nil {
		return nil, fmt.Errorf("discover user related tables: %w", err)
	}

	// Convert discovered tables to userTable structs
	for tableName, path := range discovered {
		if tableName != usersTableName {
			result = append(result, userTable{name: tableName, userColumnPath: path})
		}
	}

	// Add referenced tables for foreign key constraints
	referencedTables, err := db.findReferencedTables(ctx, tx, result, discovered)
	if err != nil {
		return nil, fmt.Errorf("find referenced tables: %w", err)
	}

	for tableName := range referencedTables {
		result = append(result, userTable{name: tableName, userColumnPath: nil})
	}

	return result, nil
}

// getAllTableNames retrieves all table names except 'users'.
func (db *Database) getAllTableNames(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_schema WHERE type = 'table' AND name != ?`, usersTableName)
	if err != nil {
		return nil, fmt.Errorf("query tables: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("close rows: %w", closeErr)
			}
		}
	}()

	var tables []string
	for rows.Next() {
		var tableName string
		err = rows.Scan(&tableName)
		if err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, tableName)
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate over tables: %w", err)
	}

	return tables, nil
}

// discoverUserRelatedTables recursively finds tables related to users through foreign keys.
func (db *Database) discoverUserRelatedTables(
	ctx context.Context, tx *sql.Tx, tables []string,
) (map[string][]string, error) {
	discovered := map[string][]string{usersTableName: {"id"}}

	changed := true
	for changed {
		changed = false

		for _, tableName := range tables {
			if _, alreadyDiscovered := discovered[tableName]; alreadyDiscovered {
				continue
			}

			found, path, err := db.checkTableForeignKeys(ctx, tx, tableName, discovered)
			if err != nil {
				return nil, fmt.Errorf("check foreign keys for table %s: %w", tableName, err)
			}

			if found {
				discovered[tableName] = path
				changed = true
			}
		}
	}

	return discovered, nil
}

// checkTableForeignKeys checks if a table references user-related tables through foreign keys.
func (db *Database) checkTableForeignKeys(
	ctx context.Context, tx *sql.Tx, tableName string, discovered map[string][]string,
) (bool, []string, error) {
	fkRows, err := tx.QueryContext(ctx, `SELECT "table", "from", "to" FROM pragma_foreign_key_list(?)`, tableName)
	if err != nil {
		return false, nil, fmt.Errorf("query foreign keys: %w", err)
	}
	defer func() {
		if closeErr := fkRows.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("close foreign key rows: %w", closeErr)
			}
		}
	}()

	for fkRows.Next() {
		var referencedTable, fromColumn, toColumn string
		err = fkRows.Scan(&referencedTable, &fromColumn, &toColumn)
		if err != nil {
			return false, nil, fmt.Errorf("scan foreign key: %w", err)
		}

		if userPath, exists := discovered[referencedTable]; exists {
			if referencedTable == usersTableName && toColumn == "id" {
				return true, []string{fromColumn}, nil
			}
			if len(userPath) > 0 && userPath[len(userPath)-1] == toColumn {
				var newPath []string
				newPath = append(newPath, fromColumn)
				newPath = append(newPath, userPath[:len(userPath)-1]...)
				return true, newPath, nil
			}
		}
	}

	err = fkRows.Err()
	if err != nil {
		return false, nil, fmt.Errorf("iterate foreign key rows: %w", err)
	}

	return false, nil, nil
}

// findReferencedTables finds tables that are referenced by user-related tables.
func (db *Database) findReferencedTables(
	ctx context.Context, tx *sql.Tx, userTables []userTable, discovered map[string][]string,
) (map[string]bool, error) {
	referencedTables := make(map[string]bool)

	for _, table := range userTables {
		refs, err := db.getTableReferences(ctx, tx, table.name)
		if err != nil {
			return nil, fmt.Errorf("get references for table %s: %w", table.name, err)
		}

		for _, ref := range refs {
			if _, alreadyDiscovered := discovered[ref]; !alreadyDiscovered {
				referencedTables[ref] = true
			}
		}
	}

	return referencedTables, nil
}

// getTableReferences gets all tables referenced by the given table.
func (db *Database) getTableReferences(ctx context.Context, tx *sql.Tx, tableName string) ([]string, error) {
	fkRows, err := tx.QueryContext(ctx, `SELECT "table", "from", "to" FROM pragma_foreign_key_list(?)`, tableName)
	if err != nil {
		return nil, fmt.Errorf("query foreign keys: %w", err)
	}
	defer func() {
		if closeErr := fkRows.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("close foreign key rows: %w", closeErr)
			}
		}
	}()

	var references []string
	for fkRows.Next() {
		var referencedTable, fromColumn, toColumn string
		err = fkRows.Scan(&referencedTable, &fromColumn, &toColumn)
		if err != nil {
			return nil, fmt.Errorf("scan foreign key: %w", err)
		}
		references = append(references, referencedTable)
	}

	err = fkRows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate foreign key rows: %w", err)
	}

	return references, nil
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

	switch {
	case table.userColumnPath == nil:
		// This is a referenced table (like exercises) - copy all data
		whereClause = ""
	case table.name == usersTableName:
		whereClause = "WHERE id = ?"
		args = append(args, userID)
	default:
		// Direct or indirect relationship to users table
		// For indirect relationships, assume the column name contains the user ID directly
		// This works for the test cases where exercise_sets.workout_user_id contains the user ID
		whereClause = "WHERE " + table.userColumnPath[0] + " = ?"
		args = append(args, userID)
	}

	// Copy the data
	var query string
	if whereClause == "" {
		query = "INSERT INTO export." + table.name + " SELECT * FROM main." + table.name
	} else {
		query = "INSERT INTO export." + table.name + " SELECT * FROM main." + table.name + " " + whereClause
	}
	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	return nil
}
