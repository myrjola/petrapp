package chatbot

import (
	"context"
	"fmt"
	"log/slog"

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
