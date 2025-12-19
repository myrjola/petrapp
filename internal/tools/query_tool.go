package tools

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// SecureQueryTool executes SQL queries safely with security constraints.
type SecureQueryTool struct {
	db               *sql.DB
	maxExecutionTime time.Duration
	maxRowsReturned  int
}

// QueryResult represents the result of a query execution.
type QueryResult struct {
	Columns  []string        `json:"columns"`
	Rows     [][]interface{} `json:"rows"`
	RowCount int             `json:"row_count"`
}

// NewSecureQueryTool creates a new SecureQueryTool instance.
func NewSecureQueryTool(db *sql.DB) *SecureQueryTool {
	const defaultTimeout = 5 * time.Second
	const defaultMaxRows = 1000

	return &SecureQueryTool{
		db:               db,
		maxExecutionTime: defaultTimeout,
		maxRowsReturned:  defaultMaxRows,
	}
}

// WithTimeout configures the maximum execution time for queries.
func (sqt *SecureQueryTool) WithTimeout(timeout time.Duration) *SecureQueryTool {
	sqt.maxExecutionTime = timeout
	return sqt
}

// WithMaxRows configures the maximum number of rows to return.
func (sqt *SecureQueryTool) WithMaxRows(maxRows int) *SecureQueryTool {
	sqt.maxRowsReturned = maxRows
	return sqt
}

// ExecuteQuery executes a SQL query with security constraints.
func (sqt *SecureQueryTool) ExecuteQuery(ctx context.Context, query string) (*QueryResult, error) {
	// Basic validation for dangerous operations
	if err := sqt.validateDangerousOperations(query); err != nil {
		return nil, err
	}

	// Set timeout and execute query
	ctx, cancel := context.WithTimeout(ctx, sqt.maxExecutionTime)
	defer cancel()

	return sqt.executeQueryWithPragma(ctx, query)
}

// executeQueryWithPragma executes the query using a transaction with query_only pragma.
func (sqt *SecureQueryTool) executeQueryWithPragma(ctx context.Context, query string) (_ *QueryResult, err error) {
	// We use a dedicated connection to ensure the PRAGMA settings are applied and reverted correctly.
	var (
		conn *sql.Conn
	)
	conn, err = sqt.db.Conn(ctx)
	defer func() {
		// Close the sqlite connection so that the pragmas are reset when a new connection is created.
		if rawErr := conn.Raw(func(_ any) error {
			return driver.ErrBadConn // According to the sql.Conn.Raw docs, this prevents reusing the Conn.
		}); rawErr != nil && !errors.Is(rawErr, driver.ErrBadConn) {
			err = fmt.Errorf("close raw db connection: %w", errors.Join(rawErr, err))
			return
		}
		if closeErr := conn.Close(); closeErr != nil && !errors.Is(closeErr, sql.ErrConnDone) {
			err = fmt.Errorf("close db connection: %w", errors.Join(closeErr, err))
		}
	}()

	// Set read-only pragma to prevent writing to the database
	if _, pragmaErr := conn.ExecContext(ctx, `PRAGMA QUERY_ONLY = TRUE`); pragmaErr != nil {
		return nil, fmt.Errorf("failed to enable read-only mode: %w", pragmaErr)
	}

	// Begin transaction
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{
		ReadOnly:  true,
		Isolation: sql.LevelDefault,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = fmt.Errorf("rollback transaction: %w", errors.Join(rollbackErr, err))
		}
	}()

	// Execute query
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = fmt.Errorf("close rows: %w", errors.Join(closeErr, err))
		}
	}()

	var result *QueryResult
	result, err = sqt.collectQueryResults(ctx, rows)
	if err != nil {
		return nil, fmt.Errorf("collect query results: %w", err)
	}

	return result, nil
}

// collectQueryResults processes rows and returns structured results.
func (sqt *SecureQueryTool) collectQueryResults(_ context.Context, rows *sql.Rows) (*QueryResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var resultRows [][]interface{}
	rowCount := 0

	for rows.Next() {
		// Check if we've reached the row limit
		if rowCount >= sqt.maxRowsReturned {
			// Stop processing more rows to enforce the limit
			break
		}

		row, scanErr := sqt.scanRow(rows, len(columns))
		if scanErr != nil {
			return nil, scanErr
		}
		resultRows = append(resultRows, row)
		rowCount++
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("row iteration error: %w", rowsErr)
	}

	return &QueryResult{
		Columns:  columns,
		Rows:     resultRows,
		RowCount: rowCount,
	}, nil
}

// scanRow scans a single row and converts values for JSON compatibility.
func (sqt *SecureQueryTool) scanRow(rows *sql.Rows, columnCount int) ([]interface{}, error) {
	values := make([]interface{}, columnCount)
	valuePtrs := make([]interface{}, columnCount)
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if scanErr := rows.Scan(valuePtrs...); scanErr != nil {
		return nil, fmt.Errorf("failed to scan row: %w", scanErr)
	}

	// Convert byte slices to strings for JSON compatibility
	for i, val := range values {
		if b, ok := val.([]byte); ok {
			values[i] = string(b)
		}
	}

	return values, nil
}

// validateDangerousOperations checks for specific dangerous operations that bypass pragma protection.
func (sqt *SecureQueryTool) validateDangerousOperations(query string) error {
	// Remove comments and normalize whitespace
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return errors.New("empty query")
	}

	// Check for dangerous operations that could bypass pragma restrictions
	dangerousPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bATTACH\s+DATABASE\b`),
		regexp.MustCompile(`(?i)\bPRAGMA\b`),
	}

	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(cleanQuery) {
			return errors.New("query contains restricted operations")
		}
	}

	return nil
}
