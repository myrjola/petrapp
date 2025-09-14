package chatbot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/myrjola/petrapp/internal/chatbot/tools"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// Service handles the business logic for AI chatbot conversations.
type Service struct {
	repo   *repository
	db     *sqlite.Database
	logger *slog.Logger
	llm    *llmClient
}

// NewService creates a new chatbot service.
func NewService(db *sqlite.Database, logger *slog.Logger, openaiAPIKey string) *Service {
	factory := newRepositoryFactory(db, logger)
	return &Service{
		repo:   factory.newRepository(),
		db:     db,
		logger: logger,
		llm:    newLLMClient(openaiAPIKey, logger),
	}
}

// CreateConversation creates a new conversation for the user.
func (s *Service) CreateConversation(ctx context.Context, title string) (Conversation, error) {
	conv := Conversation{
		Title:    &title,
		IsActive: true,
	}

	createdConv, err := s.repo.conversations.Create(ctx, conv)
	if err != nil {
		return Conversation{}, fmt.Errorf("create conversation: %w", err)
	}

	return createdConv, nil
}

// GetConversation retrieves a conversation by ID.
func (s *Service) GetConversation(ctx context.Context, id int) (Conversation, error) {
	conv, err := s.repo.conversations.Get(ctx, id)
	if err != nil {
		return Conversation{}, fmt.Errorf("get conversation: %w", err)
	}

	return conv, nil
}

// GetUserConversations retrieves all conversations for the current user.
func (s *Service) GetUserConversations(ctx context.Context) ([]Conversation, error) {
	conversations, err := s.repo.conversations.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("get user conversations: %w", err)
	}

	return conversations, nil
}

// GetConversationMessages retrieves all messages for a conversation.
func (s *Service) GetConversationMessages(ctx context.Context, conversationID int) ([]ChatMessage, error) {
	messages, err := s.repo.messages.ListByConversation(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("get conversation messages: %w", err)
	}

	return messages, nil
}

// SendMessage processes a user message and generates an AI response.
func (s *Service) SendMessage(ctx context.Context, conversationID int, content string) (ChatMessage, error) {
	// Create user message
	userMessage := ChatMessage{
		ConversationID: conversationID,
		MessageType:    MessageTypeUser,
		Content:        content,
	}

	_, err := s.repo.messages.Create(ctx, userMessage)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("save user message: %w", err)
	}

	// Update conversation activity
	err = s.repo.conversations.UpdateActivity(ctx, conversationID)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to update conversation activity", "conversation_id", conversationID, "error", err)
	}

	// Generate AI response (placeholder for now)
	assistantMessage := ChatMessage{
		ConversationID: conversationID,
		MessageType:    MessageTypeAssistant,
		Content:        "This is a placeholder response. AI integration will be implemented in the LLM client.",
	}

	savedAssistantMessage, err := s.repo.messages.Create(ctx, assistantMessage)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("save assistant message: %w", err)
	}

	return savedAssistantMessage, nil
}

// GetGenerateVisualizationTool returns the visualization tool for generating charts.
func (s *Service) GetGenerateVisualizationTool() *tools.VisualizationTool {
	return tools.NewVisualizationTool(s.db, s.logger)
}

// GetCalculateStatisticsTool returns the statistics tool for calculating workout metrics.
func (s *Service) GetCalculateStatisticsTool() *StatisticsToolWrapper {
	tool := tools.NewStatisticsTool(s.db, s.logger)
	return &StatisticsToolWrapper{tool: tool}
}

// GetExerciseInfoTool returns the exercise info tool for retrieving exercise details.
func (s *Service) GetExerciseInfoTool() *ExerciseInfoToolWrapper {
	tool := tools.NewExerciseInfoTool(s.db, s.logger)
	return &ExerciseInfoToolWrapper{tool: tool}
}

// GetQueryWorkoutDataTool returns the workout data query tool.
func (s *Service) GetQueryWorkoutDataTool() *tools.WorkoutDataQueryTool {
	return tools.NewWorkoutDataQueryTool(s.db, s.logger)
}

// StatisticsToolWrapper wraps the statistics tool to provide type conversion.
type StatisticsToolWrapper struct {
	tool *tools.StatisticsTool
}

// CalculateStatistics calculates statistical metrics with type conversion.
func (w *StatisticsToolWrapper) CalculateStatistics(ctx context.Context, request StatisticsRequest) (*StatisticsResult, error) {
	// Convert from chatbot types to tool types
	params := tools.StatisticsParams{
		MetricType:   request.MetricType,
		ExerciseName: request.ExerciseName,
	}

	if request.DateRange != nil {
		params.DateRange = &tools.DateRange{
			StartDate: request.DateRange.StartDate,
			EndDate:   request.DateRange.EndDate,
		}
	}

	// Call the tool
	result, err := w.tool.CalculateStatistics(ctx, params)
	if err != nil {
		return nil, err
	}

	// Convert back to chatbot types
	return &StatisticsResult{
		MetricType:   result.MetricType,
		ExerciseName: result.ExerciseName,
		Value:        result.Value,
		Description:  result.Description,
		UserID:       result.UserID,
	}, nil
}

// ToOpenAIFunction delegates to the wrapped tool.
func (w *StatisticsToolWrapper) ToOpenAIFunction() map[string]interface{} {
	return w.tool.ToOpenAIFunction()
}

// ExecuteFunction delegates to the wrapped tool.
func (w *StatisticsToolWrapper) ExecuteFunction(ctx context.Context, functionName string, argumentsJSON string) (string, error) {
	return w.tool.ExecuteFunction(ctx, functionName, argumentsJSON)
}

// ExerciseInfoToolWrapper wraps the exercise info tool to provide type conversion.
type ExerciseInfoToolWrapper struct {
	tool *tools.ExerciseInfoTool
}

// GetExerciseInfo retrieves exercise information with type conversion.
func (w *ExerciseInfoToolWrapper) GetExerciseInfo(ctx context.Context, request ExerciseInfoRequest) (*ExerciseInfoResult, error) {
	// Convert from chatbot types to tool types
	params := tools.ExerciseInfoParams{
		ExerciseName:   request.ExerciseName,
		IncludeHistory: request.IncludeHistory,
	}

	// Call the tool
	result, err := w.tool.GetExerciseInfo(ctx, params)
	if err != nil {
		return nil, err
	}

	// Convert back to chatbot types
	chatbotResult := &ExerciseInfoResult{
		ExerciseName:        result.ExerciseName,
		Category:            result.Category,
		ExerciseType:        result.ExerciseType,
		Description:         result.Description,
		MuscleGroups:        result.MuscleGroups,
		PrimaryMuscleGroups: result.PrimaryMuscleGroups,
	}

	// Convert user history if present
	if result.UserHistory != nil {
		chatbotResult.UserHistory = &ExerciseHistory{
			FirstPerformed: result.UserHistory.FirstPerformed,
			LastPerformed:  result.UserHistory.LastPerformed,
			TotalSessions:  result.UserHistory.TotalSessions,
			PersonalRecord: result.UserHistory.PersonalRecord,
			AverageWeight:  result.UserHistory.AverageWeight,
			TotalVolume:    result.UserHistory.TotalVolume,
		}
	}

	return chatbotResult, nil
}

// ToOpenAIFunction delegates to the wrapped tool.
func (w *ExerciseInfoToolWrapper) ToOpenAIFunction() map[string]interface{} {
	return w.tool.ToOpenAIFunction()
}

// ExecuteFunction delegates to the wrapped tool.
func (w *ExerciseInfoToolWrapper) ExecuteFunction(ctx context.Context, functionName string, argumentsJSON string) (string, error) {
	return w.tool.ExecuteFunction(ctx, functionName, argumentsJSON)
}
