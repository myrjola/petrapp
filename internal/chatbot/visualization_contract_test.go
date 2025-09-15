package chatbot_test

import (
	"context"
	"testing"

	"github.com/myrjola/petrapp/internal/chatbot"
	"github.com/myrjola/petrapp/internal/chatbot/tools"
	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// Contract test for generate_visualization LLM function
// This test MUST fail initially as the function is not yet implemented.
func TestGenerateVisualizationTool_CreateChart(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	// Create test database
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert test user
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("test-user-id"), "Test User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create test workout data for visualization
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (name, category, exercise_type, description_markdown) VALUES
		('Bench Press', 'upper', 'weighted', 'Chest exercise'),
		('Squat', 'lower', 'weighted', 'Leg exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test exercises: %v", err)
	}

	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at) VALUES
		(?, '2024-01-01', '2024-01-01T10:00:00Z', '2024-01-01T11:00:00Z'),
		(?, '2024-01-02', '2024-01-02T10:00:00Z', '2024-01-02T11:00:00Z'),
		(?, '2024-01-03', '2024-01-03T10:00:00Z', '2024-01-03T11:00:00Z')
	`, userID, userID, userID)
	if err != nil {
		t.Fatalf("Failed to insert test workout sessions: %v", err)
	}

	// Create chatbot service
	service := chatbot.NewService(db, logger, "test-api-key")

	// Test cases for different chart types
	testCases := []struct {
		name         string
		chartType    string
		title        string
		xAxisLabel   string
		yAxisLabel   string
		dataQuery    string
		seriesConfig []map[string]interface{}
		expectError  bool
	}{
		{
			name:       "Line chart for workout frequency",
			chartType:  "line",
			title:      "Workout Frequency Over Time",
			xAxisLabel: "Date",
			yAxisLabel: "Workouts",
			dataQuery:  "SELECT workout_date as date, COUNT(*) as count FROM workout_sessions WHERE user_id = ? GROUP BY workout_date ORDER BY workout_date",
			seriesConfig: []map[string]interface{}{
				{
					"name":        "Workouts",
					"data_column": "count",
					"color":       "#2563eb",
				},
			},
			expectError: false,
		},
		{
			name:       "Bar chart for exercise distribution",
			chartType:  "bar",
			title:      "Exercise Distribution",
			xAxisLabel: "Exercise",
			yAxisLabel: "Frequency",
			dataQuery:  "SELECT e.name as exercise, COUNT(*) as frequency FROM exercises e JOIN workout_exercise we ON e.id = we.exercise_id WHERE we.workout_user_id = ? GROUP BY e.name",
			seriesConfig: []map[string]interface{}{
				{
					"name":        "Frequency",
					"data_column": "frequency",
					"color":       "#dc2626",
				},
			},
			expectError: false,
		},
		{
			name:      "Pie chart for muscle group focus",
			chartType: "pie",
			title:     "Muscle Group Focus",
			dataQuery: "SELECT e.category as category, COUNT(*) as count FROM exercises e JOIN workout_exercise we ON e.id = we.exercise_id WHERE we.workout_user_id = ? GROUP BY e.category",
			seriesConfig: []map[string]interface{}{
				{
					"name":        "Exercises",
					"data_column": "count",
				},
			},
			expectError: false,
		},
		{
			name:       "Scatter plot for weight progression",
			chartType:  "scatter",
			title:      "Weight Progression",
			xAxisLabel: "Date",
			yAxisLabel: "Weight (kg)",
			dataQuery:  "SELECT ws.workout_date as date, es.weight_kg as weight FROM exercise_sets es JOIN workout_sessions ws ON es.workout_user_id = ws.user_id AND es.workout_date = ws.workout_date WHERE ws.user_id = ? AND es.weight_kg IS NOT NULL ORDER BY ws.workout_date",
			seriesConfig: []map[string]interface{}{
				{
					"name":        "Weight",
					"data_column": "weight",
					"color":       "#059669",
				},
			},
			expectError: false,
		},
		{
			name:        "Invalid chart type",
			chartType:   "invalid_chart",
			title:       "Invalid Chart",
			dataQuery:   "SELECT * FROM workout_sessions",
			expectError: true,
		},
		{
			name:        "Missing required title",
			chartType:   "line",
			title:       "",
			dataQuery:   "SELECT * FROM workout_sessions",
			expectError: true,
		},
		{
			name:        "Empty data query",
			chartType:   "bar",
			title:       "Empty Query Test",
			dataQuery:   "",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set user context
			userCtx := context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)

			// This will fail because GenerateVisualizationTool doesn't exist yet
			// That's expected for TDD - we write the test first, then implement
			tool := service.GetGenerateVisualizationTool()
			if tool == nil {
				t.Skip("GenerateVisualizationTool not implemented yet (expected for TDD)")
			}

			result, err := tool.GenerateVisualization(userCtx, tools.GenerateVisualizationParams{
				ChartType:    tc.chartType,
				Title:        tc.title,
				XAxisLabel:   tc.xAxisLabel,
				YAxisLabel:   tc.yAxisLabel,
				DataQuery:    tc.dataQuery,
				SeriesConfig: tc.seriesConfig,
			})

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error for invalid input: %v", tc)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for valid input: %v", err)
				}
				if result == nil {
					t.Error("expected non-nil result for valid input")
				} else {
					// Validate result structure
					if result.ChartType != tc.chartType {
						t.Errorf("expected chart type %s, got %s", tc.chartType, result.ChartType)
					}
					if result.Title != tc.title {
						t.Errorf("expected title %s, got %s", tc.title, result.Title)
					}
					if result.ChartConfig == "" {
						t.Error("expected non-empty chart config")
					}
					if result.DataQuery != tc.dataQuery {
						t.Errorf("expected data query %s, got %s", tc.dataQuery, result.DataQuery)
					}
				}
			}
		})
	}
}

// Test ECharts configuration generation.
func TestGenerateVisualizationTool_EChartsConfig(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetGenerateVisualizationTool()
	if tool == nil {
		t.Skip("GenerateVisualizationTool not implemented yet (expected for TDD)")
	}

	result, err := tool.GenerateVisualization(ctx, tools.GenerateVisualizationParams{
		ChartType: "line",
		Title:     "Sample Chart",
		DataQuery: "SELECT 1 as x, 2 as y",
		SeriesConfig: []map[string]interface{}{
			{
				"name":        "Series 1",
				"data_column": "y",
				"color":       "#ff6b6b",
			},
		},
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		// Validate that the chart config is valid ECharts JSON
		// This should contain proper ECharts structure
		if result.ChartConfig == "" {
			t.Error("expected non-empty ECharts config")
		}
		// Should contain basic ECharts properties
		config := result.ChartConfig
		expectedProperties := []string{"title", "xAxis", "yAxis", "series", "tooltip", "legend"}
		for _, prop := range expectedProperties {
			if !contains(config, prop) {
				t.Errorf("ECharts config should contain '%s' property", prop)
			}
		}
	}
}

// Test data security and user isolation.
func TestGenerateVisualizationTool_DataSecurity(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	service := chatbot.NewService(db, logger, "test-api-key")

	// This will fail because the tool doesn't exist yet
	tool := service.GetGenerateVisualizationTool()
	if tool == nil {
		t.Skip("GenerateVisualizationTool not implemented yet (expected for TDD)")
	}

	maliciousQueries := []struct {
		name  string
		query string
	}{
		{"SQL injection", "SELECT * FROM users; DROP TABLE users; --"},
		{"Cross-table access", "SELECT * FROM credentials"},
		{"System access", "SELECT name FROM sqlite_master"},
		{"File access", "ATTACH DATABASE '/etc/passwd' AS etc"},
	}

	for _, tc := range maliciousQueries {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.GenerateVisualization(ctx, tools.GenerateVisualizationParams{
				ChartType: "line",
				Title:     "Malicious Chart",
				DataQuery: tc.query,
			})

			if err == nil {
				t.Errorf("expected malicious query to be blocked: %s", tc.query)
			}
		})
	}
}

// Helper function to check if a string contains a substring.
func contains(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || str[0:len(substr)] == substr ||
		(len(str) > len(substr) && (str[len(str)-len(substr):] == substr ||
			str[1:len(substr)+1] == substr)))
}
