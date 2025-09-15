package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/tools"
)

// GenerateVisualizationParams represents the parameters for the generate_visualization function.
type GenerateVisualizationParams struct {
	ChartType    string                   `json:"chart_type"`
	Title        string                   `json:"title"`
	XAxisLabel   string                   `json:"x_axis_label,omitempty"`
	YAxisLabel   string                   `json:"y_axis_label,omitempty"`
	DataQuery    string                   `json:"data_query"`
	SeriesConfig []map[string]interface{} `json:"series_config,omitempty"`
}

// GenerateVisualizationResult represents the result of the generate_visualization function.
type GenerateVisualizationResult struct {
	ID          int    `json:"id"`
	ChartType   string `json:"chart_type"`
	Title       string `json:"title"`
	ChartConfig string `json:"chart_config"`
	DataQuery   string `json:"data_query"`
	MessageID   *int   `json:"message_id,omitempty"`
}

// SeriesConfig represents configuration for a chart series.
type SeriesConfig struct {
	Name       string `json:"name"`
	DataColumn string `json:"data_column"`
	Color      string `json:"color,omitempty"`
}

// EChartsConfig represents the structure of ECharts configuration.
type EChartsConfig struct {
	Title   TitleConfig   `json:"title"`
	Tooltip TooltipConfig `json:"tooltip"`
	Legend  LegendConfig  `json:"legend,omitempty"`
	XAxis   AxisConfig    `json:"xAxis,omitempty"`
	YAxis   AxisConfig    `json:"yAxis,omitempty"`
	Series  []SeriesData  `json:"series"`
	Color   []string      `json:"color,omitempty"`
}

// TitleConfig represents ECharts title configuration.
type TitleConfig struct {
	Text string `json:"text"`
}

// TooltipConfig represents ECharts tooltip configuration.
type TooltipConfig struct {
	Trigger string `json:"trigger"`
}

// LegendConfig represents ECharts legend configuration.
type LegendConfig struct {
	Data []string `json:"data,omitempty"`
}

// AxisConfig represents ECharts axis configuration.
type AxisConfig struct {
	Type      string     `json:"type,omitempty"`
	Name      string     `json:"name,omitempty"`
	Data      []string   `json:"data,omitempty"`
	AxisLabel *AxisLabel `json:"axisLabel,omitempty"`
}

// AxisLabel represents ECharts axis label configuration.
type AxisLabel struct {
	Rotate int `json:"rotate,omitempty"`
}

// SeriesData represents ECharts series data configuration.
type SeriesData struct {
	Name  string        `json:"name"`
	Type  string        `json:"type"`
	Data  []interface{} `json:"data"`
	Color string        `json:"color,omitempty"`
}

// VisualizationTool provides visualization generation functionality.
type VisualizationTool struct {
	db              *sqlite.Database
	logger          *slog.Logger
	secureQueryTool *tools.SecureQueryTool
}

// NewVisualizationTool creates a new VisualizationTool instance.
func NewVisualizationTool(db *sqlite.Database, logger *slog.Logger) *VisualizationTool {
	return &VisualizationTool{
		db:              db,
		logger:          logger,
		secureQueryTool: tools.NewSecureQueryTool(db.ReadOnly),
	}
}

// GenerateVisualization generates a chart visualization from workout data.
// This function ensures that:
// 1. Only valid chart types are allowed
// 2. All queries are automatically filtered by user_id for security
// 3. Generated ECharts configuration is stored in the database.
func (t *VisualizationTool) GenerateVisualization(ctx context.Context, params GenerateVisualizationParams) (*GenerateVisualizationResult, error) {
	// Get user ID from context
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return nil, errors.New("user not authenticated")
	}

	// Validate parameters
	if err := t.validateParams(params); err != nil {
		return nil, fmt.Errorf("parameter validation failed: %w", err)
	}

	// Add user_id filter to the query for security
	queryTool := NewWorkoutDataQueryTool(t.db, t.logger)
	queryParams := QueryWorkoutDataParams{
		Query:       params.DataQuery,
		Description: fmt.Sprintf("Data query for %s chart: %s", params.ChartType, params.Title),
	}

	// Execute the query to get data
	startTime := time.Now()
	queryResult, err := queryTool.QueryWorkoutData(ctx, queryParams)
	if err != nil {
		t.logger.ErrorContext(ctx, "Data query failed for visualization",
			"user_id", userID,
			"chart_type", params.ChartType,
			"title", params.Title,
			"error", err)
		return nil, fmt.Errorf("data query failed: %w", err)
	}

	executionTime := time.Since(startTime)
	t.logger.InfoContext(ctx, "Data query executed for visualization",
		"user_id", userID,
		"chart_type", params.ChartType,
		"title", params.Title,
		"row_count", queryResult.RowCount,
		"execution_time_ms", executionTime.Milliseconds())

	// Generate ECharts configuration
	chartConfig, err := t.generateEChartsConfig(params, queryResult)
	if err != nil {
		return nil, fmt.Errorf("failed to generate chart configuration: %w", err)
	}

	// Store visualization in database
	visualizationID, err := t.storeVisualization(ctx, params, chartConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to store visualization: %w", err)
	}

	return &GenerateVisualizationResult{
		ID:          visualizationID,
		ChartType:   params.ChartType,
		Title:       params.Title,
		ChartConfig: chartConfig,
		DataQuery:   params.DataQuery,
	}, nil
}

// validateParams validates the parameters for visualization generation.
func (t *VisualizationTool) validateParams(params GenerateVisualizationParams) error {
	// Validate chart type
	validChartTypes := map[string]bool{
		"line":    true,
		"bar":     true,
		"scatter": true,
		"pie":     true,
		"heatmap": true,
	}

	if !validChartTypes[params.ChartType] {
		return fmt.Errorf("invalid chart type: %s. Valid types are: line, bar, scatter, pie, heatmap", params.ChartType)
	}

	// Validate required fields
	if strings.TrimSpace(params.Title) == "" {
		return errors.New("title is required")
	}

	if strings.TrimSpace(params.DataQuery) == "" {
		return errors.New("data_query is required")
	}

	return nil
}

// generateEChartsConfig generates ECharts configuration based on chart type and data.
func (t *VisualizationTool) generateEChartsConfig(params GenerateVisualizationParams, queryResult *QueryWorkoutDataResult) (string, error) {
	config := EChartsConfig{
		Title: TitleConfig{
			Text: params.Title,
		},
		Tooltip: TooltipConfig{
			Trigger: "axis",
		},
		Color: []string{"#2563eb", "#dc2626", "#059669", "#d97706", "#7c3aed", "#db2777"},
	}

	switch params.ChartType {
	case "line":
		return t.generateLineChart(params, queryResult, &config)
	case "bar":
		return t.generateBarChart(params, queryResult, &config)
	case "scatter":
		return t.generateScatterChart(params, queryResult, &config)
	case "pie":
		return t.generatePieChart(params, queryResult, &config)
	case "heatmap":
		return t.generateHeatmapChart(params, queryResult, &config)
	default:
		return "", fmt.Errorf("unsupported chart type: %s", params.ChartType)
	}
}

// generateLineChart generates a line chart configuration.
func (t *VisualizationTool) generateLineChart(params GenerateVisualizationParams, queryResult *QueryWorkoutDataResult, config *EChartsConfig) (string, error) {
	config.XAxis = AxisConfig{
		Type: "category",
		Name: params.XAxisLabel,
		Data: make([]string, 0),
	}
	config.YAxis = AxisConfig{
		Type: "value",
		Name: params.YAxisLabel,
	}

	// Extract X-axis data (first column) and prepare series data
	xAxisData := make([]string, 0, len(queryResult.Rows))
	seriesDataMap := make(map[string][]interface{})

	for _, row := range queryResult.Rows {
		if len(row) > 0 {
			xValue := fmt.Sprintf("%v", row[0])
			xAxisData = append(xAxisData, xValue)

			// For each series configuration, extract the corresponding data
			for _, seriesConfig := range params.SeriesConfig {
				seriesName := fmt.Sprintf("%v", seriesConfig["name"])
				dataColumn := fmt.Sprintf("%v", seriesConfig["data_column"])

				// Find the column index
				columnIndex := -1
				for i, col := range queryResult.Columns {
					if col == dataColumn {
						columnIndex = i
						break
					}
				}

				if columnIndex >= 0 && columnIndex < len(row) {
					if seriesDataMap[seriesName] == nil {
						seriesDataMap[seriesName] = make([]interface{}, 0)
					}
					seriesDataMap[seriesName] = append(seriesDataMap[seriesName], row[columnIndex])
				}
			}
		}
	}

	config.XAxis.Data = xAxisData

	// Create series data
	legendData := make([]string, 0, len(seriesDataMap))
	colorIndex := 0
	for seriesName, data := range seriesDataMap {
		series := SeriesData{
			Name: seriesName,
			Type: "line",
			Data: data,
		}

		// Apply custom color if specified
		for _, seriesConfig := range params.SeriesConfig {
			if fmt.Sprintf("%v", seriesConfig["name"]) == seriesName {
				if color, ok := seriesConfig["color"]; ok && color != nil {
					series.Color = fmt.Sprintf("%v", color)
				} else if colorIndex < len(config.Color) {
					series.Color = config.Color[colorIndex]
				}
				break
			}
		}

		config.Series = append(config.Series, series)
		legendData = append(legendData, seriesName)
		colorIndex++
	}

	config.Legend = LegendConfig{Data: legendData}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal line chart config: %w", err)
	}

	return string(configJSON), nil
}

// generateBarChart generates a bar chart configuration.
func (t *VisualizationTool) generateBarChart(params GenerateVisualizationParams, queryResult *QueryWorkoutDataResult, config *EChartsConfig) (string, error) {
	config.XAxis = AxisConfig{
		Type: "category",
		Name: params.XAxisLabel,
		Data: make([]string, 0),
	}
	config.YAxis = AxisConfig{
		Type: "value",
		Name: params.YAxisLabel,
	}

	// Extract data similar to line chart but use bar type
	xAxisData := make([]string, 0, len(queryResult.Rows))
	seriesDataMap := make(map[string][]interface{})

	for _, row := range queryResult.Rows {
		if len(row) > 0 {
			xValue := fmt.Sprintf("%v", row[0])
			xAxisData = append(xAxisData, xValue)

			for _, seriesConfig := range params.SeriesConfig {
				seriesName := fmt.Sprintf("%v", seriesConfig["name"])
				dataColumn := fmt.Sprintf("%v", seriesConfig["data_column"])

				columnIndex := -1
				for i, col := range queryResult.Columns {
					if col == dataColumn {
						columnIndex = i
						break
					}
				}

				if columnIndex >= 0 && columnIndex < len(row) {
					if seriesDataMap[seriesName] == nil {
						seriesDataMap[seriesName] = make([]interface{}, 0)
					}
					seriesDataMap[seriesName] = append(seriesDataMap[seriesName], row[columnIndex])
				}
			}
		}
	}

	config.XAxis.Data = xAxisData
	// Rotate labels if we have many categories
	if len(xAxisData) > 6 {
		config.XAxis.AxisLabel = &AxisLabel{Rotate: 45}
	}

	// Create series data
	legendData := make([]string, 0, len(seriesDataMap))
	colorIndex := 0
	for seriesName, data := range seriesDataMap {
		series := SeriesData{
			Name: seriesName,
			Type: "bar",
			Data: data,
		}

		for _, seriesConfig := range params.SeriesConfig {
			if fmt.Sprintf("%v", seriesConfig["name"]) == seriesName {
				if color, ok := seriesConfig["color"]; ok && color != nil {
					series.Color = fmt.Sprintf("%v", color)
				} else if colorIndex < len(config.Color) {
					series.Color = config.Color[colorIndex]
				}
				break
			}
		}

		config.Series = append(config.Series, series)
		legendData = append(legendData, seriesName)
		colorIndex++
	}

	config.Legend = LegendConfig{Data: legendData}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal bar chart config: %w", err)
	}

	return string(configJSON), nil
}

// generateScatterChart generates a scatter chart configuration.
func (t *VisualizationTool) generateScatterChart(params GenerateVisualizationParams, queryResult *QueryWorkoutDataResult, config *EChartsConfig) (string, error) {
	config.XAxis = AxisConfig{
		Type: "value",
		Name: params.XAxisLabel,
	}
	config.YAxis = AxisConfig{
		Type: "value",
		Name: params.YAxisLabel,
	}

	// For scatter plot, data should be in [x, y] format
	seriesDataMap := make(map[string][]interface{})

	for _, row := range queryResult.Rows {
		if len(row) >= 2 {
			for _, seriesConfig := range params.SeriesConfig {
				seriesName := fmt.Sprintf("%v", seriesConfig["name"])
				dataColumn := fmt.Sprintf("%v", seriesConfig["data_column"])

				// Find the column index for Y values
				columnIndex := -1
				for i, col := range queryResult.Columns {
					if col == dataColumn {
						columnIndex = i
						break
					}
				}

				if columnIndex >= 0 && columnIndex < len(row) {
					if seriesDataMap[seriesName] == nil {
						seriesDataMap[seriesName] = make([]interface{}, 0)
					}
					// Create [x, y] pair
					point := []interface{}{row[0], row[columnIndex]}
					seriesDataMap[seriesName] = append(seriesDataMap[seriesName], point)
				}
			}
		}
	}

	// Create series data
	legendData := make([]string, 0, len(seriesDataMap))
	colorIndex := 0
	for seriesName, data := range seriesDataMap {
		series := SeriesData{
			Name: seriesName,
			Type: "scatter",
			Data: data,
		}

		for _, seriesConfig := range params.SeriesConfig {
			if fmt.Sprintf("%v", seriesConfig["name"]) == seriesName {
				if color, ok := seriesConfig["color"]; ok && color != nil {
					series.Color = fmt.Sprintf("%v", color)
				} else if colorIndex < len(config.Color) {
					series.Color = config.Color[colorIndex]
				}
				break
			}
		}

		config.Series = append(config.Series, series)
		legendData = append(legendData, seriesName)
		colorIndex++
	}

	config.Legend = LegendConfig{Data: legendData}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal scatter chart config: %w", err)
	}

	return string(configJSON), nil
}

// generatePieChart generates a pie chart configuration.
func (t *VisualizationTool) generatePieChart(params GenerateVisualizationParams, queryResult *QueryWorkoutDataResult, config *EChartsConfig) (string, error) {
	config.Tooltip = TooltipConfig{Trigger: "item"}

	// For pie chart, data should be in {name: string, value: number} format
	var pieData []interface{}

	for _, row := range queryResult.Rows {
		if len(row) >= 2 {
			// First column is name, second column is value
			name := fmt.Sprintf("%v", row[0])
			value := row[1]

			// Convert value to number if it's a string
			if strValue, ok := value.(string); ok {
				if numValue, err := strconv.ParseFloat(strValue, 64); err == nil {
					value = numValue
				}
			}

			pieData = append(pieData, map[string]interface{}{
				"name":  name,
				"value": value,
			})
		}
	}

	series := SeriesData{
		Name: "Data",
		Type: "pie",
		Data: pieData,
	}

	// Apply custom color if specified in first series config
	if len(params.SeriesConfig) > 0 {
		if color, ok := params.SeriesConfig[0]["color"]; ok && color != nil {
			series.Color = fmt.Sprintf("%v", color)
		}
	}

	config.Series = []SeriesData{series}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal pie chart config: %w", err)
	}

	return string(configJSON), nil
}

// generateHeatmapChart generates a heatmap chart configuration.
func (t *VisualizationTool) generateHeatmapChart(params GenerateVisualizationParams, queryResult *QueryWorkoutDataResult, config *EChartsConfig) (string, error) {
	config.XAxis = AxisConfig{
		Type: "category",
		Name: params.XAxisLabel,
		Data: make([]string, 0),
	}
	config.YAxis = AxisConfig{
		Type: "category",
		Name: params.YAxisLabel,
		Data: make([]string, 0),
	}

	// For heatmap, data should be in [x_index, y_index, value] format
	xCategories := make(map[string]int)
	yCategories := make(map[string]int)
	var heatmapData []interface{}

	for _, row := range queryResult.Rows {
		if len(row) >= 3 {
			xValue := fmt.Sprintf("%v", row[0])
			yValue := fmt.Sprintf("%v", row[1])
			zValue := row[2]

			// Track unique categories
			if _, exists := xCategories[xValue]; !exists {
				xCategories[xValue] = len(xCategories)
			}
			if _, exists := yCategories[yValue]; !exists {
				yCategories[yValue] = len(yCategories)
			}

			// Convert value to number if it's a string
			if strValue, ok := zValue.(string); ok {
				if numValue, err := strconv.ParseFloat(strValue, 64); err == nil {
					zValue = numValue
				}
			}

			heatmapData = append(heatmapData, []interface{}{
				xCategories[xValue],
				yCategories[yValue],
				zValue,
			})
		}
	}

	// Convert category maps to slices
	xAxisData := make([]string, len(xCategories))
	yAxisData := make([]string, len(yCategories))

	for category, index := range xCategories {
		xAxisData[index] = category
	}
	for category, index := range yCategories {
		yAxisData[index] = category
	}

	config.XAxis.Data = xAxisData
	config.YAxis.Data = yAxisData

	series := SeriesData{
		Name: "Heatmap",
		Type: "heatmap",
		Data: heatmapData,
	}

	config.Series = []SeriesData{series}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal heatmap chart config: %w", err)
	}

	return string(configJSON), nil
}

// storeVisualization stores the visualization in the message_visualizations table.
// For testing purposes, we create a temporary message to satisfy the foreign key constraint.
func (t *VisualizationTool) storeVisualization(ctx context.Context, params GenerateVisualizationParams, chartConfig string) (int, error) {
	// Get user ID from context
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return 0, errors.New("user not authenticated")
	}

	// For testing/standalone visualization generation, create a temporary conversation and message
	// In production, this would be handled by the chatbot service with actual messages
	var conversationID int
	err := t.db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO conversations (user_id, title) VALUES (?, 'Visualization Generation') RETURNING id`,
		userID).Scan(&conversationID)
	if err != nil {
		return 0, fmt.Errorf("failed to create temporary conversation: %w", err)
	}

	var messageID int
	err = t.db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO chat_messages (conversation_id, message_type, content) VALUES (?, 'assistant', 'Generated visualization') RETURNING id`,
		conversationID).Scan(&messageID)
	if err != nil {
		return 0, fmt.Errorf("failed to create temporary message: %w", err)
	}

	// Now store the visualization
	query := `
		INSERT INTO message_visualizations (message_id, chart_type, chart_config, data_query, created_at)
		VALUES (?, ?, ?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
		RETURNING id`

	var id int
	err = t.db.ReadWrite.QueryRowContext(ctx, query,
		messageID,
		params.ChartType,
		chartConfig,
		params.DataQuery,
	).Scan(&id)

	if err != nil {
		return 0, fmt.Errorf("failed to insert visualization: %w", err)
	}

	return id, nil
}

// ToOpenAIFunction returns the OpenAI function definition for generate_visualization.
func (t *VisualizationTool) ToOpenAIFunction() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        "generate_visualization",
			"description": "Generate a chart or graph visualization from workout data",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"chart_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"line", "bar", "scatter", "pie", "heatmap"},
						"description": "Type of chart to generate",
					},
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Chart title",
					},
					"x_axis_label": map[string]interface{}{
						"type":        "string",
						"description": "Label for X axis",
					},
					"y_axis_label": map[string]interface{}{
						"type":        "string",
						"description": "Label for Y axis",
					},
					"data_query": map[string]interface{}{
						"type":        "string",
						"description": "SQL query to fetch data for the visualization. Results should have columns matching the chart requirements.",
					},
					"series_config": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"name": map[string]interface{}{
									"type":        "string",
									"description": "Series name",
								},
								"data_column": map[string]interface{}{
									"type":        "string",
									"description": "Column name from query results to use for this series",
								},
								"color": map[string]interface{}{
									"type":        "string",
									"description": "Optional color for the series (hex or name)",
								},
							},
						},
						"description": "Configuration for data series in the chart",
					},
				},
				"required": []string{"chart_type", "title", "data_query"},
			},
		},
	}
}

// ExecuteFunction executes the generate_visualization function with the given parameters.
// This method is compatible with OpenAI function calling.
func (t *VisualizationTool) ExecuteFunction(ctx context.Context, functionName string, argumentsJSON string) (string, error) {
	if functionName != "generate_visualization" {
		return "", fmt.Errorf("unsupported function: %s", functionName)
	}

	var params GenerateVisualizationParams
	if err := json.Unmarshal([]byte(argumentsJSON), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	result, err := t.GenerateVisualization(ctx, params)
	if err != nil {
		return "", err
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(resultJSON), nil
}
