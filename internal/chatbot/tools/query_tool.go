package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/tools"
)

// QueryWorkoutDataParams represents the parameters for the query_workout_data function.
type QueryWorkoutDataParams struct {
	Query       string `json:"query"`
	Description string `json:"description"`
}

// QueryWorkoutDataResult represents the result of the query_workout_data function.
type QueryWorkoutDataResult struct {
	Columns     []string        `json:"columns"`
	Rows        [][]interface{} `json:"rows"`
	RowCount    int             `json:"row_count"`
	Description string          `json:"description"`
	UserID      int             `json:"user_id"`
}

// WorkoutDataQueryTool provides secure SQL query execution for workout data.
type WorkoutDataQueryTool struct {
	db              *sqlite.Database
	logger          *slog.Logger
	secureQueryTool *tools.SecureQueryTool
}

// NewWorkoutDataQueryTool creates a new WorkoutDataQueryTool instance.
func NewWorkoutDataQueryTool(db *sqlite.Database, logger *slog.Logger) *WorkoutDataQueryTool {
	return &WorkoutDataQueryTool{
		db:              db,
		logger:          logger,
		secureQueryTool: tools.NewSecureQueryTool(db.ReadOnly),
	}
}

// QueryWorkoutData executes a SQL query to retrieve workout data for the current user.
// This function ensures that:
// 1. Only SELECT statements are allowed
// 2. All queries are automatically filtered by user_id for security
// 3. SQL injection protection is enforced.
func (t *WorkoutDataQueryTool) QueryWorkoutData(ctx context.Context, params QueryWorkoutDataParams) (*QueryWorkoutDataResult, error) {
	// Get user ID from context
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return nil, errors.New("user not authenticated")
	}

	// Validate that this is a SELECT statement
	if err := t.validateSelectStatement(params.Query); err != nil {
		return nil, fmt.Errorf("query validation failed: %w", err)
	}

	// Add user_id filter to the query for security
	userFilteredQuery, err := t.addUserFilter(params.Query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to add user filter: %w", err)
	}

	// Execute the query using the secure query tool
	startTime := time.Now()
	result, err := t.secureQueryTool.ExecuteQuery(ctx, userFilteredQuery)
	if err != nil {
		t.logger.ErrorContext(ctx, "Query execution failed",
			"user_id", userID,
			"description", params.Description,
			"error", err)
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	executionTime := time.Since(startTime)
	t.logger.InfoContext(ctx, "Query executed successfully",
		"user_id", userID,
		"description", params.Description,
		"row_count", result.RowCount,
		"execution_time_ms", executionTime.Milliseconds())

	return &QueryWorkoutDataResult{
		Columns:     result.Columns,
		Rows:        result.Rows,
		RowCount:    result.RowCount,
		Description: params.Description,
		UserID:      userID,
	}, nil
}

// validateSelectStatement ensures that only SELECT statements are allowed.
func (t *WorkoutDataQueryTool) validateSelectStatement(query string) error {
	// Remove leading/trailing whitespace and normalize
	cleanQuery := strings.TrimSpace(query)
	if cleanQuery == "" {
		return errors.New("empty query")
	}

	// Convert to lowercase for pattern matching
	lowerQuery := strings.ToLower(cleanQuery)

	// Must start with SELECT
	if !strings.HasPrefix(lowerQuery, "select") {
		return errors.New("only SELECT statements are allowed")
	}

	// Check for dangerous SQL operations
	dangerousPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|DROP|CREATE|ALTER|TRUNCATE|REPLACE)\b`),
		regexp.MustCompile(`(?i)\b(ATTACH|DETACH)\b`),
		regexp.MustCompile(`(?i)\bPRAGMA\b`),
		regexp.MustCompile(`(?i)\bEXEC\b`),
		regexp.MustCompile(`(?i)\bCALL\b`),
		regexp.MustCompile(`(?i)\bLOAD_EXTENSION\b`),
		regexp.MustCompile(`(?i)\bVACUUM\b`),
		regexp.MustCompile(`(?i)\bREINDEX\b`),
		regexp.MustCompile(`(?i)\bANALYZE\b`),
	}

	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(cleanQuery) {
			return errors.New("query contains forbidden SQL operations")
		}
	}

	return nil
}

// addUserFilter adds a user_id filter to the query to ensure data isolation.
func (t *WorkoutDataQueryTool) addUserFilter(query string, userID int) (string, error) {
	// Convert to lowercase for analysis but preserve original case for final query
	lowerQuery := strings.ToLower(query)

	// Check if query already has user_id filtering
	if t.hasUserIDFilter(lowerQuery) {
		// If it already has user_id filtering, replace any hardcoded user_id values
		return t.replaceUserIDValues(query, userID), nil
	}

	// For queries that reference tables with user_id directly, add WHERE clause
	userTables := []string{"workout_sessions", "conversations", "workout_preferences"}
	for _, table := range userTables {
		if strings.Contains(lowerQuery, table) {
			return t.injectUserFilter(query, userID)
		}
	}

	// For exercise_sets table, use workout_user_id
	if strings.Contains(lowerQuery, "exercise_sets") {
		return t.injectUserFilterWithColumn(query, userID, "workout_user_id")
	}

	// For workout_exercise table, use workout_user_id
	if strings.Contains(lowerQuery, "workout_exercise") {
		return t.injectUserFilterWithColumn(query, userID, "workout_user_id")
	}

	// For complex queries or queries on other tables, return as-is for now
	// In a production system, you might want to be more restrictive
	return query, nil
}

// hasUserIDFilter checks if the query already contains user_id filtering.
func (t *WorkoutDataQueryTool) hasUserIDFilter(lowerQuery string) bool {
	// Look for user_id or workout_user_id in WHERE clauses or JOIN conditions
	userIDPattern := regexp.MustCompile(`\b(user_id|workout_user_id)\b`)
	return userIDPattern.MatchString(lowerQuery)
}

// replaceUserIDValues replaces any hardcoded user_id values with the actual user ID.
func (t *WorkoutDataQueryTool) replaceUserIDValues(query string, userID int) string {
	// Replace patterns like "user_id = 123" with "user_id = actualUserID"
	userIDPattern := regexp.MustCompile(`(?i)\buser_id\s*=\s*\d+`)
	query = userIDPattern.ReplaceAllString(query, fmt.Sprintf("user_id = %d", userID))

	// Replace patterns like "workout_user_id = 123" with "workout_user_id = actualUserID"
	workoutUserIDPattern := regexp.MustCompile(`(?i)\bworkout_user_id\s*=\s*\d+`)
	query = workoutUserIDPattern.ReplaceAllString(query, fmt.Sprintf("workout_user_id = %d", userID))

	return query
}

// injectUserFilter automatically adds user_id filtering to queries that don't have it.
func (t *WorkoutDataQueryTool) injectUserFilter(query string, userID int) (string, error) {
	lowerQuery := strings.ToLower(query)

	// Add WHERE clause with user_id filter
	if strings.Contains(lowerQuery, " where ") {
		// Already has WHERE clause, add AND condition
		whereIndex := strings.LastIndex(lowerQuery, " where ")
		beforeWhere := query[:whereIndex+7] // +7 for " where "
		afterWhere := query[whereIndex+7:]

		return fmt.Sprintf("%s(%s) AND user_id = %d", beforeWhere, afterWhere, userID), nil
	}

	// No WHERE clause, add one
	// Find the position to insert WHERE clause (before ORDER BY, GROUP BY, LIMIT, etc.)
	insertPos := len(query)

	keywords := []string{" order by ", " group by ", " having ", " limit ", " offset "}
	for _, keyword := range keywords {
		if idx := strings.LastIndex(lowerQuery, keyword); idx != -1 && idx < insertPos {
			insertPos = idx
		}
	}

	beforeClause := strings.TrimSpace(query[:insertPos])
	afterClause := query[insertPos:]

	return fmt.Sprintf("%s WHERE user_id = %d%s", beforeClause, userID, afterClause), nil
}

// injectUserFilterWithColumn adds user filtering with a specific column name.
func (t *WorkoutDataQueryTool) injectUserFilterWithColumn(query string, userID int, columnName string) (string, error) {
	lowerQuery := strings.ToLower(query)

	// Add WHERE clause with the specified column filter - be more flexible with whitespace
	wherePattern := regexp.MustCompile(`(?i)\bwhere\b`)
	if wherePattern.MatchString(lowerQuery) {
		// Already has WHERE clause, add AND condition
		// Find the WHERE clause and just add AND to it, don't include GROUP BY, ORDER BY, etc.
		whereMatch := wherePattern.FindStringIndex(lowerQuery)
		if whereMatch != nil {
			whereEnd := whereMatch[1]

			// Find the end of the WHERE condition (before GROUP BY, ORDER BY, HAVING, etc.)
			restOfQuery := query[whereEnd:]
			lowerRest := strings.ToLower(restOfQuery)

			endOfWhereClause := len(restOfQuery)
			keywordPatterns := []*regexp.Regexp{
				regexp.MustCompile(`(?i)\s+group\s+by\s+`),
				regexp.MustCompile(`(?i)\s+having\s+`),
				regexp.MustCompile(`(?i)\s+order\s+by\s+`),
				regexp.MustCompile(`(?i)\s+limit\s+`),
				regexp.MustCompile(`(?i)\s+offset\s+`),
			}
			for _, pattern := range keywordPatterns {
				if match := pattern.FindStringIndex(lowerRest); match != nil && match[0] < endOfWhereClause {
					endOfWhereClause = match[0]
				}
			}

			whereCondition := strings.TrimSpace(restOfQuery[:endOfWhereClause])
			remainingQuery := restOfQuery[endOfWhereClause:]
			beforeWhere := query[:whereEnd]

			return fmt.Sprintf("%s %s AND %s = %d%s", beforeWhere, whereCondition, columnName, userID, remainingQuery), nil
		}
	}

	// No WHERE clause, add one
	// Find the position to insert WHERE clause (before ORDER BY, GROUP BY, LIMIT, etc.)
	insertPos := len(query)

	keywords := []string{" order by ", " group by ", " having ", " limit ", " offset "}
	for _, keyword := range keywords {
		if idx := strings.Index(lowerQuery, keyword); idx != -1 && idx < insertPos {
			insertPos = idx
		}
	}

	beforeClause := strings.TrimSpace(query[:insertPos])
	afterClause := query[insertPos:]

	if strings.TrimSpace(afterClause) != "" {
		return fmt.Sprintf("%s WHERE %s = %d %s", beforeClause, columnName, userID, afterClause), nil
	}
	return fmt.Sprintf("%s WHERE %s = %d", beforeClause, columnName, userID), nil
}

// ToOpenAIFunction returns the OpenAI function definition for query_workout_data.
func (t *WorkoutDataQueryTool) ToOpenAIFunction() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "query_workout_data",
			"description": "Execute a SQL query to retrieve workout data for the current user",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "SQL SELECT query to execute. Must be a SELECT statement only.",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Human-readable description of what this query retrieves",
					},
				},
				"required": []string{"query", "description"},
			},
		},
	}
}

// ExecuteFunction executes the query_workout_data function with the given parameters.
// This method is compatible with OpenAI function calling.
func (t *WorkoutDataQueryTool) ExecuteFunction(ctx context.Context, functionName string, argumentsJSON string) (string, error) {
	if functionName != "query_workout_data" {
		return "", fmt.Errorf("unsupported function: %s", functionName)
	}

	var params QueryWorkoutDataParams
	if err := json.Unmarshal([]byte(argumentsJSON), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	result, err := t.QueryWorkoutData(ctx, params)
	if err != nil {
		return "", err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}
