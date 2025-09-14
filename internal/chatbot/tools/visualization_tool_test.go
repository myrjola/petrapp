package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func TestVisualizationTool_GenerateVisualization_LineChart(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	// Create test database
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Insert test user
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create test workout data for visualization (only if not exists)
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT OR IGNORE INTO exercises (name, category, exercise_type, description_markdown) VALUES
		('Bench Press', 'upper', 'weighted', 'Chest exercise'),
		('Squat', 'lower', 'weighted', 'Leg exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at) VALUES
		(?, '2024-01-01', '2024-01-01T10:00:00.000Z', '2024-01-01T11:00:00.000Z'),
		(?, '2024-01-02', '2024-01-02T10:00:00.000Z', '2024-01-02T11:00:00.000Z'),
		(?, '2024-01-03', '2024-01-03T10:00:00.000Z', '2024-01-03T11:00:00.000Z')
	`, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert test workout sessions: %v", err)
	}

	// Create visualization tool
	tool := NewVisualizationTool(db, logger)

	// Set user context
	userCtx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)

	// Test line chart
	result, err := tool.GenerateVisualization(userCtx, GenerateVisualizationParams{
		ChartType:  "line",
		Title:      "Workout Frequency Over Time",
		XAxisLabel: "Date",
		YAxisLabel: "Workouts",
		DataQuery:  "SELECT workout_date as date, COUNT(*) as count FROM workout_sessions GROUP BY workout_date ORDER BY workout_date",
		SeriesConfig: []map[string]interface{}{
			{
				"name":        "Workouts",
				"data_column": "count",
				"color":       "#2563eb",
			},
		},
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Validate result structure
	if result.ChartType != "line" {
		t.Errorf("Expected chart type 'line', got %s", result.ChartType)
	}
	if result.Title != "Workout Frequency Over Time" {
		t.Errorf("Expected title 'Workout Frequency Over Time', got %s", result.Title)
	}
	if result.ChartConfig == "" {
		t.Error("Expected non-empty chart config")
	}

	// Validate that the chart config is valid JSON
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(result.ChartConfig), &config); err != nil {
		t.Errorf("Chart config is not valid JSON: %v", err)
	}

	// Check for basic ECharts properties
	expectedProperties := []string{"title", "xAxis", "yAxis", "series", "tooltip"}
	for _, prop := range expectedProperties {
		if _, exists := config[prop]; !exists {
			t.Errorf("ECharts config should contain '%s' property", prop)
		}
	}

	// Verify data is stored in database
	var storedID int
	err = db.ReadOnly.QueryRowContext(ctx, "SELECT id FROM message_visualizations WHERE id = ?", result.ID).Scan(&storedID)
	if err != nil {
		t.Errorf("Failed to find stored visualization in database: %v", err)
	}
	if storedID != result.ID {
		t.Errorf("Expected stored ID %d, got %d", result.ID, storedID)
	}
}

func TestVisualizationTool_GenerateVisualization_BarChart(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Insert test user
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create visualization tool and test bar chart
	tool := NewVisualizationTool(db, logger)
	userCtx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)

	result, err := tool.GenerateVisualization(userCtx, GenerateVisualizationParams{
		ChartType:  "bar",
		Title:      "Test Bar Chart",
		XAxisLabel: "Category",
		YAxisLabel: "Value",
		DataQuery:  "SELECT 'A' as category, 10 as value UNION SELECT 'B' as category, 20 as value UNION SELECT 'C' as category, 15 as value",
		SeriesConfig: []map[string]interface{}{
			{
				"name":        "Values",
				"data_column": "value",
			},
		},
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.ChartType != "bar" {
		t.Errorf("Expected chart type 'bar', got %s", result.ChartType)
	}

	// Validate JSON structure
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(result.ChartConfig), &config); err != nil {
		t.Errorf("Chart config is not valid JSON: %v", err)
	}
}

func TestVisualizationTool_GenerateVisualization_PieChart(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Insert test user
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create visualization tool and test pie chart
	tool := NewVisualizationTool(db, logger)
	userCtx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)

	result, err := tool.GenerateVisualization(userCtx, GenerateVisualizationParams{
		ChartType: "pie",
		Title:     "Test Pie Chart",
		DataQuery: "SELECT 'Upper Body' as category, 60 as count UNION SELECT 'Lower Body' as category, 40 as count",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.ChartType != "pie" {
		t.Errorf("Expected chart type 'pie', got %s", result.ChartType)
	}

	// For pie chart, tooltip should be 'item'
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(result.ChartConfig), &config); err != nil {
		t.Errorf("Chart config is not valid JSON: %v", err)
	}

	tooltip, exists := config["tooltip"].(map[string]interface{})
	if !exists {
		t.Error("Expected tooltip configuration")
	} else if tooltip["trigger"] != "item" {
		t.Errorf("Expected pie chart tooltip trigger to be 'item', got %v", tooltip["trigger"])
	}
}

func TestVisualizationTool_GenerateVisualization_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Insert test user
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	tool := NewVisualizationTool(db, logger)
	userCtx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)

	testCases := []struct {
		name   string
		params GenerateVisualizationParams
	}{
		{
			name: "Invalid chart type",
			params: GenerateVisualizationParams{
				ChartType: "invalid_chart",
				Title:     "Test",
				DataQuery: "SELECT 1 as x, 2 as y",
			},
		},
		{
			name: "Missing title",
			params: GenerateVisualizationParams{
				ChartType: "line",
				Title:     "",
				DataQuery: "SELECT 1 as x, 2 as y",
			},
		},
		{
			name: "Missing data query",
			params: GenerateVisualizationParams{
				ChartType: "line",
				Title:     "Test",
				DataQuery: "",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.GenerateVisualization(userCtx, tc.params)
			if err == nil {
				t.Errorf("Expected error for invalid input: %v", tc.params)
			}
		})
	}
}

func TestVisualizationTool_GenerateVisualization_UnauthenticatedUser(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	tool := NewVisualizationTool(db, logger)

	// Don't set user context - should fail
	_, err = tool.GenerateVisualization(ctx, GenerateVisualizationParams{
		ChartType: "line",
		Title:     "Test Chart",
		DataQuery: "SELECT 1 as x, 2 as y",
	})

	if err == nil {
		t.Error("Expected authentication error for unauthenticated user")
	}
}

func TestVisualizationTool_ToOpenAIFunction(t *testing.T) {
	db, err := sqlite.NewDatabase(context.Background(), ":memory:", testhelpers.NewLogger(testhelpers.NewWriter(t)))
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	tool := NewVisualizationTool(db, testhelpers.NewLogger(testhelpers.NewWriter(t)))
	function := tool.ToOpenAIFunction()

	if function["type"] != "function" {
		t.Errorf("Expected function type 'function', got %v", function["type"])
	}

	functionDef, ok := function["function"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected function definition to be a map")
	}

	if functionDef["name"] != "generate_visualization" {
		t.Errorf("Expected function name 'generate_visualization', got %v", functionDef["name"])
	}

	// Verify required parameters
	params, ok := functionDef["parameters"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected parameters to be a map")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("Expected required to be a string slice")
	}

	expectedRequired := []string{"chart_type", "title", "data_query"}
	if len(required) != len(expectedRequired) {
		t.Errorf("Expected %d required parameters, got %d", len(expectedRequired), len(required))
	}

	for _, exp := range expectedRequired {
		found := false
		for _, req := range required {
			if req == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected required parameter '%s' not found", exp)
		}
	}
}

func TestVisualizationTool_ExecuteFunction(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Insert test user
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	tool := NewVisualizationTool(db, logger)
	userCtx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)

	// Test valid function call
	argumentsJSON := `{
		"chart_type": "line",
		"title": "Test Chart",
		"data_query": "SELECT 'Day 1' as date, 1 as count UNION SELECT 'Day 2' as date, 2 as count",
		"series_config": [{"name": "Count", "data_column": "count"}]
	}`

	resultJSON, err := tool.ExecuteFunction(userCtx, "generate_visualization", argumentsJSON)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify result is valid JSON
	var result GenerateVisualizationResult
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		t.Errorf("Result is not valid JSON: %v", err)
	}

	if result.ChartType != "line" {
		t.Errorf("Expected chart type 'line', got %s", result.ChartType)
	}

	// Test unsupported function
	_, err = tool.ExecuteFunction(userCtx, "unsupported_function", argumentsJSON)
	if err == nil {
		t.Error("Expected error for unsupported function")
	}

	// Test invalid arguments
	_, err = tool.ExecuteFunction(userCtx, "generate_visualization", "invalid json")
	if err == nil {
		t.Error("Expected error for invalid JSON arguments")
	}
}
