package chatbot

import (
	"time"
)

// MessageType represents the type of message in a conversation.
type MessageType string

const (
	MessageTypeUser      MessageType = "user"
	MessageTypeAssistant MessageType = "assistant"
)

// ChartType represents the type of visualization chart.
type ChartType string

const (
	ChartTypeLine    ChartType = "line"
	ChartTypeBar     ChartType = "bar"
	ChartTypeScatter ChartType = "scatter"
	ChartTypePie     ChartType = "pie"
	ChartTypeHeatmap ChartType = "heatmap"
)

// Conversation represents an AI trainer conversation session.
type Conversation struct {
	ID             int       `json:"id"`
	UserID         int       `json:"user_id"`
	Title          *string   `json:"title"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	IsActive       bool      `json:"is_active"`
	ContextSummary *string   `json:"context_summary"`
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	ID              int             `json:"id"`
	ConversationID  int             `json:"conversation_id"`
	MessageType     MessageType     `json:"message_type"`
	Content         string          `json:"content"`
	CreatedAt       time.Time       `json:"created_at"`
	TokenCount      *int            `json:"token_count"`
	ErrorMessage    *string         `json:"error_message"`
	QueryExecuted   *string         `json:"query_executed"`
	ExecutionTimeMs *int            `json:"execution_time_ms"`
	Visualizations  []Visualization `json:"visualizations,omitempty"`
}

// Visualization represents a chart/graph generated from workout data.
type Visualization struct {
	ID          int       `json:"id"`
	MessageID   int       `json:"message_id"`
	ChartType   ChartType `json:"chart_type"`
	ChartConfig string    `json:"chart_config"`
	DataQuery   string    `json:"data_query"`
	CreatedAt   time.Time `json:"created_at"`
}

// IsValid validates a MessageType.
func (mt MessageType) IsValid() bool {
	switch mt {
	case MessageTypeUser, MessageTypeAssistant:
		return true
	default:
		return false
	}
}

// IsValid validates a ChartType.
func (ct ChartType) IsValid() bool {
	switch ct {
	case ChartTypeLine, ChartTypeBar, ChartTypeScatter, ChartTypePie, ChartTypeHeatmap:
		return true
	default:
		return false
	}
}

// VisualizationRequest represents a request to generate a chart visualization.
type VisualizationRequest struct {
	ChartType    string                   `json:"chart_type"`
	Title        string                   `json:"title"`
	XAxisLabel   string                   `json:"x_axis_label,omitempty"`
	YAxisLabel   string                   `json:"y_axis_label,omitempty"`
	DataQuery    string                   `json:"data_query"`
	SeriesConfig []map[string]interface{} `json:"series_config,omitempty"`
}

// VisualizationResult represents the result of generating a visualization.
type VisualizationResult struct {
	ChartType   string `json:"chart_type"`
	Title       string `json:"title"`
	ChartConfig string `json:"chart_config"`
	DataQuery   string `json:"data_query"`
}

// DateRange represents a date range for statistical analysis.
type DateRange struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// StatisticsRequest represents a request to calculate statistics.
type StatisticsRequest struct {
	MetricType   string     `json:"metric_type"`
	ExerciseName string     `json:"exercise_name,omitempty"`
	DateRange    *DateRange `json:"date_range,omitempty"`
}

// StatisticsResult represents the result of calculating statistics.
type StatisticsResult struct {
	MetricType   string      `json:"metric_type"`
	ExerciseName string      `json:"exercise_name,omitempty"`
	Value        interface{} `json:"value"`
	Description  string      `json:"description"`
	UserID       int         `json:"user_id,omitempty"`
}

// ExerciseInfoRequest represents a request to get exercise information.
type ExerciseInfoRequest struct {
	ExerciseName   string `json:"exercise_name"`
	IncludeHistory bool   `json:"include_history"`
}

// ExerciseInfoResult represents detailed information about an exercise.
type ExerciseInfoResult struct {
	ExerciseName        string           `json:"exercise_name"`
	Category            string           `json:"category"`
	ExerciseType        string           `json:"exercise_type"`
	Description         string           `json:"description"`
	MuscleGroups        []string         `json:"muscle_groups"`
	PrimaryMuscleGroups []string         `json:"primary_muscle_groups"`
	UserHistory         *ExerciseHistory `json:"user_history,omitempty"`
}

// ExerciseHistory represents user's history with a specific exercise.
type ExerciseHistory struct {
	FirstPerformed *string  `json:"first_performed"`
	LastPerformed  *string  `json:"last_performed"`
	TotalSessions  int      `json:"total_sessions"`
	PersonalRecord *float64 `json:"personal_record"`
	AverageWeight  *float64 `json:"average_weight"`
	TotalVolume    float64  `json:"total_volume"`
}

// WorkoutRecommendationRequest represents a request for workout recommendations.
type WorkoutRecommendationRequest struct {
	WorkoutType        string   `json:"workout_type"`
	DurationMinutes    *int     `json:"duration_minutes,omitempty"`
	EquipmentAvailable []string `json:"equipment_available,omitempty"`
	AvoidMuscleGroups  []string `json:"avoid_muscle_groups,omitempty"`
}

// WorkoutRecommendationResult represents a generated workout recommendation.
type WorkoutRecommendationResult struct {
	WorkoutType       string                `json:"workout_type"`
	EstimatedDuration int                   `json:"estimated_duration"`
	Exercises         []RecommendedExercise `json:"exercises"`
	Notes             []string              `json:"notes,omitempty"`
	WarmupExercises   []RecommendedExercise `json:"warmup_exercises,omitempty"`
	CooldownTips      []string              `json:"cooldown_tips,omitempty"`
}

// RecommendedExercise represents an exercise in a workout recommendation.
type RecommendedExercise struct {
	ExerciseName      string   `json:"exercise_name"`
	Sets              int      `json:"sets"`
	MinReps           int      `json:"min_reps"`
	MaxReps           int      `json:"max_reps"`
	RecommendedWeight *float64 `json:"recommended_weight,omitempty"`
	RestSeconds       int      `json:"rest_seconds"`
	Notes             string   `json:"notes,omitempty"`
	MuscleGroups      []string `json:"muscle_groups"`
}

// WorkoutPatternRequest represents a request for workout pattern analysis.
type WorkoutPatternRequest struct {
	AnalysisType string `json:"analysis_type"`
	LookbackDays int    `json:"lookback_days"`
}

// WorkoutPatternResult represents the result of workout pattern analysis.
type WorkoutPatternResult struct {
	AnalysisType    string                 `json:"analysis_type"`
	LookbackDays    int                    `json:"lookback_days"`
	Summary         string                 `json:"summary"`
	Insights        []string               `json:"insights"`
	Recommendations []string               `json:"recommendations"`
	MetricsData     map[string]interface{} `json:"metrics_data"`
	Score           *float64               `json:"score,omitempty"`
}
