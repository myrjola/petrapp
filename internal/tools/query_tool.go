package tools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/xwb1989/sqlparser"
)

// SecureQueryTool executes SQL queries safely with security constraints.
type SecureQueryTool struct {
	db               *sql.DB
	maxExecutionTime time.Duration
	maxRowsReturned  int
	logger           *slog.Logger
}

// QueryResult represents the result of a query execution.
type QueryResult struct {
	Columns  []string        `json:"columns"`
	Rows     [][]interface{} `json:"rows"`
	RowCount int             `json:"row_count"`
}

// NewSecureQueryTool creates a new SecureQueryTool instance.
func NewSecureQueryTool(db *sql.DB, logger *slog.Logger) *SecureQueryTool {
	const defaultTimeout = 5 * time.Second
	const defaultMaxRows = 1000

	return &SecureQueryTool{
		db:               db,
		maxExecutionTime: defaultTimeout,
		maxRowsReturned:  defaultMaxRows,
		logger:           logger,
	}
}

// WithTimeout configures the maximum execution time for queries.
func (t *SecureQueryTool) WithTimeout(timeout time.Duration) *SecureQueryTool {
	t.maxExecutionTime = timeout
	return t
}

// WithMaxRows configures the maximum number of rows to return.
func (t *SecureQueryTool) WithMaxRows(maxRows int) *SecureQueryTool {
	t.maxRowsReturned = maxRows
	return t
}

// ExecuteQuery executes a SQL query with security constraints.
func (t *SecureQueryTool) ExecuteQuery(ctx context.Context, query string) (*QueryResult, error) {
	// Validate SQL query
	if err := t.ValidateSQL(query); err != nil {
		return nil, t.SanitizeError(err)
	}

	// Apply row limit
	limitedQuery := t.EnforceRowLimit(query)

	// Set timeout and execute query
	ctx, cancel := context.WithTimeout(ctx, t.maxExecutionTime)
	defer cancel()

	return t.executeQueryInTransaction(ctx, limitedQuery)
}

// executeQueryInTransaction executes the query within a read-only transaction.
func (t *SecureQueryTool) executeQueryInTransaction(ctx context.Context, query string) (*QueryResult, error) {
	tx, err := t.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelDefault})
	if err != nil {
		return nil, t.SanitizeError(fmt.Errorf("failed to begin transaction: %w", err))
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			t.logger.ErrorContext(ctx, "failed to rollback transaction", "error", rollbackErr)
		}
	}()

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, t.SanitizeError(fmt.Errorf("query execution failed: %w", err))
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			t.logger.ErrorContext(ctx, "failed to close rows", "error", closeErr)
		}
	}()

	return t.collectQueryResults(rows)
}

// collectQueryResults processes rows and returns structured results.
func (t *SecureQueryTool) collectQueryResults(rows *sql.Rows) (*QueryResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, t.SanitizeError(fmt.Errorf("failed to get columns: %w", err))
	}

	var resultRows [][]interface{}
	rowCount := 0

	for rows.Next() && rowCount < t.maxRowsReturned {
		row, scanErr := t.scanRow(rows, len(columns))
		if scanErr != nil {
			return nil, scanErr
		}
		resultRows = append(resultRows, row)
		rowCount++
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, t.SanitizeError(fmt.Errorf("row iteration error: %w", rowsErr))
	}

	return &QueryResult{
		Columns:  columns,
		Rows:     resultRows,
		RowCount: rowCount,
	}, nil
}

// scanRow scans a single row and converts values for JSON compatibility.
func (t *SecureQueryTool) scanRow(rows *sql.Rows, columnCount int) ([]interface{}, error) {
	values := make([]interface{}, columnCount)
	valuePtrs := make([]interface{}, columnCount)
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if scanErr := rows.Scan(valuePtrs...); scanErr != nil {
		return nil, t.SanitizeError(fmt.Errorf("failed to scan row: %w", scanErr))
	}

	// Convert byte slices to strings for JSON compatibility
	for i, val := range values {
		if b, ok := val.([]byte); ok {
			values[i] = string(b)
		}
	}

	return values, nil
}

// ValidateSQL validates that the query is safe to execute.
func (t *SecureQueryTool) ValidateSQL(query string) error {
	// Remove comments and normalize whitespace
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return errors.New("empty query")
	}

	// Parse the SQL statement
	stmt, err := sqlparser.Parse(cleanQuery)
	if err != nil {
		return fmt.Errorf("invalid SQL syntax: %w", err)
	}

	// Only allow SELECT statements
	switch stmt.(type) {
	case *sqlparser.Select:
		// This is allowed
	case *sqlparser.Union:
		// UNION statements with SELECT are allowed
	default:
		return errors.New("only SELECT queries are allowed")
	}

	// Check for dangerous operations using case-insensitive regex
	dangerousPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bATTACH\s+DATABASE\b`),
		regexp.MustCompile(`(?i)\bPRAGMA\b`),
		regexp.MustCompile(`(?i)\bLOAD_EXTENSION\b`),
		regexp.MustCompile(`(?i)\bCREATE\s+TEMP\s+TABLE\b`),
		regexp.MustCompile(`(?i)\bCREATE\s+TEMPORARY\s+TABLE\b`),
		regexp.MustCompile(`(?i)\bCREATE\s+VIEW\b`),
		regexp.MustCompile(`(?i)\bCREATE\s+TRIGGER\b`),
		regexp.MustCompile(`(?i)\bCREATE\s+INDEX\b`),
	}

	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(cleanQuery) {
			return errors.New("query contains restricted operations")
		}
	}

	return nil
}

// EnforceRowLimit adds or modifies LIMIT clause to ensure row limits are enforced.
func (t *SecureQueryTool) EnforceRowLimit(query string) string {
	// Simple approach: if query doesn't contain LIMIT, add it
	// If it contains LIMIT with a higher value, replace it with our max
	cleanQuery := strings.TrimSpace(query)

	limitRegex := regexp.MustCompile(`(?i)\bLIMIT\s+(\d+)`)
	matches := limitRegex.FindStringSubmatch(cleanQuery)

	if len(matches) > 0 {
		// Query has LIMIT, replace if higher than our max
		return limitRegex.ReplaceAllString(cleanQuery, fmt.Sprintf("LIMIT %d", t.maxRowsReturned))
	}

	// No LIMIT clause found, add one
	return fmt.Sprintf("%s LIMIT %d", cleanQuery, t.maxRowsReturned)
}

// SanitizeError removes sensitive information from error messages.
func (t *SecureQueryTool) SanitizeError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Remove file paths and internal details
	sanitizedMsg := regexp.MustCompile(`/[^\s]*`).ReplaceAllString(errMsg, "[path]")
	sanitizedMsg = regexp.MustCompile(`line \d+`).ReplaceAllString(sanitizedMsg, "line [number]")
	sanitizedMsg = regexp.MustCompile(`column \d+`).ReplaceAllString(sanitizedMsg, "column [number]")

	// Remove specific SQLite error codes that might leak information
	_ = regexp.MustCompile(`SQLITE_[A-Z_]+`).ReplaceAllString(sanitizedMsg, "database error")

	// Keep the message generic but helpful
	switch {
	case strings.Contains(strings.ToLower(errMsg), "syntax"):
		return errors.New("SQL syntax error")
	case strings.Contains(strings.ToLower(errMsg), "timeout"):
		return errors.New("query execution timeout")
	case strings.Contains(strings.ToLower(errMsg), "no such"):
		return errors.New("referenced table or column not found")
	case strings.Contains(strings.ToLower(errMsg), "constraint"):
		return errors.New("constraint violation")
	default:
		return errors.New("query execution failed")
	}
}

// ConfigureSecureDB applies security settings to a database connection.
func ConfigureSecureDB(ctx context.Context, db *sql.DB) error {
	// Apply security-focused PRAGMA settings
	securityPragmas := []string{
		"PRAGMA trusted_schema = OFF",
		"PRAGMA ignore_check_constraints = OFF",
		"PRAGMA max_page_count = 10000",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA secure_delete = ON",
		"PRAGMA cell_size_check = ON",
	}

	for _, pragma := range securityPragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("failed to set security pragma %q: %w", pragma, err)
		}
	}

	return nil
}
