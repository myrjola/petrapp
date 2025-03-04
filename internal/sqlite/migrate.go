package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"time"
)

// migrateTo ensures that the db schema matches the target schema defined in schema.sql.
//
// We employ a declarative schema migration that:
//
// 1. Deletes deleted tables,
// 2. Creates new tables,
// 3. Migrates changed tables using 12-step schema migration https://www.sqlite.org/lang_altertable.html#otheralter,
// 4. Synchronises triggers and indexes.
//
// Inspired by https://david.rothlis.net/declarative-schema-migration-for-sqlite/
func (db *Database) migrateTo(ctx context.Context, schemaDefinition string) error {
	var err error
	// 12-step schema migration starts here. See https://www.sqlite.org/lang_altertable.html#otheralter.
	start := time.Now()

	closeDatabase, err := db.attachSchemaTargetDatabase(ctx, schemaDefinition)
	if err != nil {
		return fmt.Errorf("attach schema target database: %w", err)
	}
	defer closeDatabase()

	// Step 1: Disable foreign key validation temporarily.
	if _, err = db.ReadWrite.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign key validation: %w", err)
	}
	// Step 12: Re-enable foreign key validation.
	defer func() {
		if _, err = db.ReadWrite.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
			err = fmt.Errorf("re-enable foreign key validation: %w", err)
			db.logger.LogAttrs(ctx, slog.LevelError, "exit to avoid data corruption", slog.Any("error", err))
			err = syscall.Kill(syscall.Getpid(), syscall.SIGINT)
			if err != nil {
				os.Exit(1)
			}
		}
	}()

	// Step 2: Start transaction.
	var tx *sql.Tx
	if tx, err = db.ReadWrite.BeginTx(ctx, nil); err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	defer db.rollback(ctx, tx)()

	// Step 3-7 migrate tables.
	if err = db.migrateTables(ctx, tx); err != nil {
		return fmt.Errorf("migrate tables: %w", err)
	}

	// Step 8: Recreate indexes and triggers associated with table if needed.
	if err = db.migrateSchema(ctx, tx, schemaTypeTrigger); err != nil {
		return fmt.Errorf("migrate triggers: %w", err)
	}
	if err = db.migrateSchema(ctx, tx, schemaTypeIndex); err != nil {
		return fmt.Errorf("migrate indexes: %w", err)
	}

	// Step 9: Recreate views associated with table.
	// Step 10: Check foreign key constraints.
	if _, err = tx.ExecContext(ctx, "PRAGMA foreign_key_check"); err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}

	// Step 11: Commit transaction from step 2.
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Step 12: is in defer above.

	db.logger.LogAttrs(ctx, slog.LevelInfo, "migrated database", slog.Duration("duration", time.Since(start)))

	return nil
}

// attachSchemaTargetDatabase attaches a temporary database initialised with the target schema and returns
// a function to detach the database that must be called after the migration.
func (db *Database) attachSchemaTargetDatabase(ctx context.Context, schemaDefinition string) (func(), error) {
	// Create schema against a temporary database so that we know what has changed.
	var err error
	schemaTargetDataSourceName := fmt.Sprintf("file:%s?mode=memory&cache=shared", rand.Text())
	schemaTargetDatabase, err := sql.Open("sqlite3", schemaTargetDataSourceName)
	if err != nil {
		return nil, fmt.Errorf("open schema target database: %w", err)
	}
	defer func() {
		if err = schemaTargetDatabase.Close(); err != nil {
			err = fmt.Errorf("close schema target database: %w", err)
			db.logger.LogAttrs(ctx, slog.LevelError, "failed to close schema target database",
				slog.Any("error", err))
		}
	}()
	if _, err = schemaTargetDatabase.ExecContext(ctx, schemaDefinition); err != nil {
		return nil, fmt.Errorf("migrate schema target database: %w", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx, "ATTACH DATABASE ? AS schemaTarget",
		schemaTargetDataSourceName); err != nil {
		return nil, fmt.Errorf("attach schema target database: %w", err)
	}
	return func() {
		if _, err = db.ReadWrite.ExecContext(ctx, "DETACH DATABASE schemaTarget"); err != nil {
			db.logger.LogAttrs(ctx, slog.LevelError, "failed to detach schema target database", slog.Any("error", err))
		}
	}, nil
}

// rollback rolls back given transaction.
func (db *Database) rollback(ctx context.Context, tx *sql.Tx) func() {
	return func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			err = fmt.Errorf("rollback transaction: %w", err)
			db.logger.LogAttrs(ctx, slog.LevelError, "failed to rollback transaction", slog.Any("error", err))
		}
	}
}

// migrateTables ensures table schema is synchronized between databases.
func (db *Database) migrateTables(ctx context.Context, tx *sql.Tx) error {
	// Step 3: Remember schema (also includes trivial creation and deletion of tables).
	var err error

	// Drop deleted tables.
	var deletedTables []string
	if deletedTables, err = db.queryDeletedTables(ctx, tx); err != nil {
		return fmt.Errorf("query deleted tables: %w", err)
	}
	for _, table := range deletedTables {
		db.logger.LogAttrs(ctx, slog.LevelInfo, "dropping table", slog.String("table", table))
		if _, err = tx.ExecContext(ctx, fmt.Sprintf("DROP TABLE %s", table)); err != nil {
			return fmt.Errorf("DROP TABLE %s: %w", table, err)
		}
	}

	// Create new tables.
	var newTableSQLs []string
	if newTableSQLs, err = db.queryNewTableSQLs(ctx, tx); err != nil {
		return fmt.Errorf("query new table SQLs: %w", err)
	}
	for _, newTableSQL := range newTableSQLs {
		db.logger.LogAttrs(ctx, slog.LevelInfo, "creating table", slog.String("query", newTableSQL))
		if _, err = tx.ExecContext(ctx, newTableSQL); err != nil {
			return fmt.Errorf("create table: %w", err)
		}
	}

	// Identify tables with changed schema and continue the 12-step schema migration with them.
	var changedTables []changedSchema
	if changedTables, err = db.queryChangedSchemas(ctx, tx, `SELECT live.name AS changed_table,
       live.sql  AS live_sql,
       target.sql   AS new_sql
FROM sqlite_schema AS live
         JOIN schemaTarget.sqlite_schema AS target ON live.name = target.name AND live.type = target.type
WHERE live.type = 'table'
  AND live.name NOT LIKE 'sqlite_%'
  AND live.name NOT LIKE '_litestream_%'
  -- The table rename operation adds double quotes around the table name, so we remove them for this diff.
  AND REPLACE(live.sql, '"', '') <> REPLACE(target.sql, '"', '')
`); err != nil {
		return fmt.Errorf("query changed tables: %w", err)
	}

	for _, table := range changedTables {
		db.logger.LogAttrs(ctx, slog.LevelInfo, "migrating table",
			slog.String("table", table.name),
			slog.String("live_sql", table.liveSQL),
			slog.String("new_sql", table.newSQL))

		// Step 4: Create tables according to new schema on temporary names.
		tempName := table.name + "_migration_temp"
		tempNameSQL := strings.Replace(table.newSQL, table.name, tempName, 1)
		db.logger.LogAttrs(ctx, slog.LevelInfo, "creating new table to temporary name",
			slog.String("query", tempNameSQL))
		if _, err = tx.ExecContext(ctx, tempNameSQL); err != nil {
			return fmt.Errorf("create new table to temporary name %s: %w", tempNameSQL, err)
		}

		// Step 5: Copy common columns between tables.
		var commonColumns []string
		if commonColumns, err = db.queryCommonColumns(ctx, tx, table.name); err != nil {
			return fmt.Errorf("query common columns: %w", err)
		}
		common := strings.Join(commonColumns, ", ")
		copySQL := fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s;", //nolint: gosec // we trust the query.
			tempName, common, common, table.name)
		db.logger.LogAttrs(ctx, slog.LevelInfo, "copying data", slog.String("query", copySQL))
		if _, err = tx.ExecContext(ctx, copySQL); err != nil {
			return fmt.Errorf("copy data: %w", err)
		}

		// Step 6: Drop the old table.
		dropSQL := fmt.Sprintf("DROP TABLE %s;", table.name)
		db.logger.LogAttrs(ctx, slog.LevelInfo, "dropping old table", slog.String("query", dropSQL))
		if _, err = tx.ExecContext(ctx, dropSQL); err != nil {
			return fmt.Errorf("drop old table: %w", err)
		}

		// Step 7: Rename new table to old table's name.
		renameSQL := fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", tempName, table.name)
		db.logger.LogAttrs(ctx, slog.LevelInfo, "renaming new table", slog.String("query", renameSQL))
		if _, err = tx.ExecContext(ctx, renameSQL); err != nil {
			return fmt.Errorf("rename new table: %w", err)
		}
	}
	return nil
}

// queryDeletedTables returns a list of tables that are present in the live schema but not in the target schema.
func (db *Database) queryDeletedTables(ctx context.Context, tx *sql.Tx) ([]string, error) {
	var (
		deletedTables []string
		err           error
	)
	if deletedTables, err = db.queryStringSlice(ctx, tx, `SELECT live.name AS deleted_table
FROM sqlite_schema AS live
         LEFT JOIN schemaTarget.sqlite_schema AS target ON live.name = target.name AND live.type = target.type
WHERE live.type = 'table'
  AND target.type IS NULL
  AND live.name NOT LIKE 'sqlite_%'
  AND live.name NOT LIKE '_litestream_%'`); err != nil {
		return nil, fmt.Errorf("query string slice: %w", err)
	}
	return deletedTables, nil
}

// queryNewTableSQLs returns a list of SQL statements to create new tables that are present in the target schema but not
// in the live schema.
func (db *Database) queryNewTableSQLs(ctx context.Context, tx *sql.Tx) ([]string, error) {
	var (
		newTableSQLs []string
		err          error
	)
	if newTableSQLs, err = db.queryStringSlice(ctx, tx, `SELECT target.sql AS sql
FROM sqlite_schema AS live RIGHT JOIN schemaTarget.sqlite_schema AS target
ON live.name=target.name AND live.type=target.type
WHERE target.type = 'table'
  AND live.type IS NULL
  AND target.name NOT LIKE 'sqlite_%'
  AND target.name NOT LIKE '_litestream_%'`); err != nil {
		return nil, fmt.Errorf("query string slice: %w", err)
	}
	return newTableSQLs, nil
}

func (db *Database) queryCommonColumns(ctx context.Context, tx *sql.Tx, table string) ([]string, error) {
	var (
		commonColumns []string
		err           error
	)
	// We wrap the column names in with double quotes to handle column names that are SQLite keywords.
	if commonColumns, err = db.queryStringSlice(ctx, tx, `SELECT '"' || target.name || '"'
FROM PRAGMA_TABLE_INFO(:table_name) AS live
JOIN PRAGMA_TABLE_INFO(:table_name, 'schemaTarget') AS target ON target.name = live.name`,
		sql.Named("table_name", table)); err != nil {
		return nil, fmt.Errorf("query string slice: %w", err)
	}
	return commonColumns, nil
}

// queryStringSlice returns a slice of strings from a query and its args.
//
// It is used to query a single column from a table.
func (db *Database) queryStringSlice(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	var (
		results []string
		rows    *sql.Rows
		err     error
	)
	if rows, err = tx.QueryContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() {
		if err = rows.Close(); err != nil {
			err = fmt.Errorf("close rows: %w", err)
			db.logger.Error("could not close rows", slog.Any("error", err))
		}
	}()
	for rows.Next() {
		var result string
		if err = rows.Scan(&result); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, result)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return results, nil
}

type changedSchema struct {
	name    string
	liveSQL string
	newSQL  string
}

// queryChangedSchemas returns a list of entities that have different schema in the live schema and the target schema.
func (db *Database) queryChangedSchemas(
	ctx context.Context,
	tx *sql.Tx,
	query string,
	args ...any,
) ([]changedSchema, error) {
	var (
		changedSchemas []changedSchema
		rows           *sql.Rows
		err            error
	)
	if rows, err = tx.QueryContext(ctx, query, args...); err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer func() {
		if err = rows.Close(); err != nil {
			err = fmt.Errorf("close rows: %w", err)
			db.logger.Error("could not close rows", slog.Any("error", err))
		}
	}()
	for rows.Next() {
		var result changedSchema
		if err = rows.Scan(&result.name, &result.liveSQL, &result.newSQL); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		changedSchemas = append(changedSchemas, result)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return changedSchemas, nil
}

type schemaType string

const (
	schemaTypeTrigger schemaType = "trigger"
	schemaTypeIndex   schemaType = "index"
)

// migrateSchema ensures all entities of typ are synchronized between databases.
func (db *Database) migrateSchema(ctx context.Context, tx *sql.Tx, typ schemaType) error {
	var (
		err     error
		deleted []string
		logger  = db.logger.With(slog.String("schemaType", string(typ)))
	)

	if deleted, err = db.queryStringSlice(ctx, tx, `SELECT live.name AS deleted
FROM sqlite_schema AS live
         LEFT JOIN schemaTarget.sqlite_schema AS target ON live.name = target.name AND live.type = target.type
WHERE live.type = ?
  AND target.type IS NULL
  AND live.name NOT LIKE 'sqlite_%'`, typ); err != nil {
		return fmt.Errorf("query deleted %s: %w", string(typ), err)
	}
	for _, name := range deleted {
		dropQuery := fmt.Sprintf("DROP %s %s;", strings.ToUpper(string(typ)), name)
		logger.LogAttrs(ctx, slog.LevelInfo, "dropping", slog.String("name", name), slog.String("query", dropQuery))
		if _, err = tx.ExecContext(ctx, dropQuery, name); err != nil {
			return fmt.Errorf("drop schema type %s %s: %w", string(typ), name, err)
		}
	}

	var created []string
	if created, err = db.queryStringSlice(ctx, tx, `SELECT target.sql AS new_index_sql
FROM sqlite_schema AS live
         RIGHT JOIN schemaTarget.sqlite_schema AS target ON live.name = target.name AND live.type = target.type
WHERE target.type = ?
  AND live.type IS NULL
  AND target.name NOT LIKE 'sqlite_%'`, typ); err != nil {
		return fmt.Errorf("query created %s: %w", string(typ), err)
	}
	for _, newSQL := range created {
		logger.LogAttrs(ctx, slog.LevelInfo, "creating", slog.String("query", newSQL))
		if _, err = tx.ExecContext(ctx, newSQL); err != nil {
			return fmt.Errorf("create changed: %w", err)
		}
	}

	var changedList []changedSchema
	if changedList, err = db.queryChangedSchemas(ctx, tx, `SELECT live.name  AS changed_trigger,
       live.sql   AS live_sql,
       target.sql AS new_sql
FROM sqlite_schema AS live
         JOIN schemaTarget.sqlite_schema AS target ON live.name = target.name AND live.type = target.type
WHERE live.type = ?
  AND live.name NOT LIKE 'sqlite_%'
  AND live.sql <> target.sql`, typ); err != nil {
		return fmt.Errorf("query changed %s: %w", string(typ), err)
	}

	for _, changed := range changedList {
		logger.LogAttrs(ctx, slog.LevelInfo, "migrating",
			slog.String("changed", changed.name),
			slog.String("live_sql", changed.liveSQL),
			slog.String("new_sql", changed.newSQL))

		dropSQL := fmt.Sprintf("DROP %s %s;", strings.ToUpper(string(typ)), changed.name)
		logger.LogAttrs(ctx, slog.LevelInfo, "dropping old changed",
			slog.String("name", changed.name), slog.String("query", dropSQL))
		if _, err = tx.ExecContext(ctx, dropSQL); err != nil {
			return fmt.Errorf("drop old changed %s %s: %w", string(typ), changed.name, err)
		}
		logger.LogAttrs(ctx, slog.LevelInfo, "creating new changed", slog.String("query", changed.newSQL))
		if _, err = tx.ExecContext(ctx, changed.newSQL); err != nil {
			return fmt.Errorf("create new changed %s %s: %w", string(typ), changed.name, err)
		}
	}
	return nil
}
