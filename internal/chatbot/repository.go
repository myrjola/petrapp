package chatbot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/sqlite"
)

// ErrNotFound is returned when a requested entity is not found.
var ErrNotFound = errors.New("not found")

// repository contains the repositories for the chatbot domain aggregates.
type repository struct {
	conversations  conversationRepository
	messages       messageRepository
	visualizations visualizationRepository
}

// conversationRepository handles conversation persistence.
type conversationRepository interface {
	Create(ctx context.Context, conv Conversation) (Conversation, error)
	Get(ctx context.Context, id int) (Conversation, error)
	List(ctx context.Context) ([]Conversation, error)
	UpdateActivity(ctx context.Context, id int) error
	Update(ctx context.Context, id int, updateFn func(*Conversation) (bool, error)) error
}

// messageRepository handles chat message persistence.
type messageRepository interface {
	Create(ctx context.Context, msg ChatMessage) (ChatMessage, error)
	Get(ctx context.Context, id int) (ChatMessage, error)
	ListByConversation(ctx context.Context, conversationID int) ([]ChatMessage, error)
	Update(ctx context.Context, id int, updateFn func(*ChatMessage) (bool, error)) error
}

// visualizationRepository handles message visualization persistence.
type visualizationRepository interface {
	Create(ctx context.Context, viz Visualization) (Visualization, error)
	Get(ctx context.Context, id int) (Visualization, error)
	ListByMessage(ctx context.Context, messageID int) ([]Visualization, error)
}

// repositoryFactory creates repository instances.
type repositoryFactory struct {
	db     *sqlite.Database
	logger *slog.Logger
}

// newRepositoryFactory creates a new repository factory.
func newRepositoryFactory(db *sqlite.Database, logger *slog.Logger) *repositoryFactory {
	return &repositoryFactory{
		db:     db,
		logger: logger,
	}
}

// newRepository creates a new repository aggregate.
func (f *repositoryFactory) newRepository() *repository {
	return &repository{
		conversations:  newConversationRepository(f.db, f.logger),
		messages:       newMessageRepository(f.db, f.logger),
		visualizations: newVisualizationRepository(f.db, f.logger),
	}
}

// Placeholder implementations - these will be implemented in Phase 3.3
type conversationRepositoryImpl struct {
	db     *sqlite.Database
	logger *slog.Logger
}

type messageRepositoryImpl struct {
	db     *sqlite.Database
	logger *slog.Logger
}

type visualizationRepositoryImpl struct {
	db     *sqlite.Database
	logger *slog.Logger
}

func newConversationRepository(db *sqlite.Database, logger *slog.Logger) conversationRepository {
	return &conversationRepositoryImpl{db: db, logger: logger}
}

func newMessageRepository(db *sqlite.Database, logger *slog.Logger) messageRepository {
	return &messageRepositoryImpl{db: db, logger: logger}
}

func newVisualizationRepository(db *sqlite.Database, logger *slog.Logger) visualizationRepository {
	return &visualizationRepositoryImpl{db: db, logger: logger}
}

// Create creates a new conversation.
func (r *conversationRepositoryImpl) Create(ctx context.Context, conv Conversation) (Conversation, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return Conversation{}, errors.New("user not authenticated")
	}

	query := `
		INSERT INTO conversations (user_id, title, is_active)
		VALUES (?, ?, ?)
		RETURNING id, user_id, title, created_at, updated_at, is_active, context_summary
	`

	var result Conversation
	err := r.db.ReadWrite.QueryRowContext(ctx, query, userID, conv.Title, conv.IsActive).Scan(
		&result.ID,
		&result.UserID,
		&result.Title,
		&result.CreatedAt,
		&result.UpdatedAt,
		&result.IsActive,
		&result.ContextSummary,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to create conversation", "error", err)
		return Conversation{}, fmt.Errorf("create conversation: %w", err)
	}

	return result, nil
}

func (r *conversationRepositoryImpl) Get(ctx context.Context, id int) (Conversation, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return Conversation{}, errors.New("user not authenticated")
	}

	query := `
		SELECT id, user_id, title, created_at, updated_at, is_active, context_summary
		FROM conversations
		WHERE id = ? AND user_id = ?
	`

	var result Conversation
	err := r.db.ReadOnly.QueryRowContext(ctx, query, id, userID).Scan(
		&result.ID,
		&result.UserID,
		&result.Title,
		&result.CreatedAt,
		&result.UpdatedAt,
		&result.IsActive,
		&result.ContextSummary,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Conversation{}, ErrNotFound
		}
		r.logger.ErrorContext(ctx, "failed to get conversation", "id", id, "error", err)
		return Conversation{}, fmt.Errorf("get conversation: %w", err)
	}

	return result, nil
}

func (r *conversationRepositoryImpl) List(ctx context.Context) ([]Conversation, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return nil, errors.New("user not authenticated")
	}

	query := `
		SELECT id, user_id, title, created_at, updated_at, is_active, context_summary
		FROM conversations
		WHERE user_id = ?
		ORDER BY updated_at DESC
	`

	rows, err := r.db.ReadOnly.QueryContext(ctx, query, userID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to list conversations", "error", err)
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var conversations []Conversation
	for rows.Next() {
		var conv Conversation
		err := rows.Scan(
			&conv.ID,
			&conv.UserID,
			&conv.Title,
			&conv.CreatedAt,
			&conv.UpdatedAt,
			&conv.IsActive,
			&conv.ContextSummary,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to scan conversation", "error", err)
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, conv)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "failed to iterate conversations", "error", err)
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}

	return conversations, nil
}

func (r *conversationRepositoryImpl) UpdateActivity(ctx context.Context, id int) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return errors.New("user not authenticated")
	}

	query := `
		UPDATE conversations
		SET updated_at = STRFTIME('%Y-%m-%dT%H:%M:%fZ')
		WHERE id = ? AND user_id = ?
	`

	result, err := r.db.ReadWrite.ExecContext(ctx, query, id, userID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to update conversation activity", "id", id, "error", err)
		return fmt.Errorf("update conversation activity: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *conversationRepositoryImpl) Update(ctx context.Context, id int, updateFn func(*Conversation) (bool, error)) error {
	// Get current conversation
	conv, err := r.Get(ctx, id)
	if err != nil {
		return err
	}

	// Apply update function
	shouldUpdate, err := updateFn(&conv)
	if err != nil {
		return err
	}

	if !shouldUpdate {
		return nil
	}

	query := `
		UPDATE conversations
		SET title = ?, is_active = ?, context_summary = ?
		WHERE id = ? AND user_id = ?
	`

	result, err := r.db.ReadWrite.ExecContext(ctx, query, conv.Title, conv.IsActive, conv.ContextSummary, id, conv.UserID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to update conversation", "id", id, "error", err)
		return fmt.Errorf("update conversation: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *messageRepositoryImpl) Create(ctx context.Context, msg ChatMessage) (ChatMessage, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return ChatMessage{}, errors.New("user not authenticated")
	}

	query := `
		INSERT INTO chat_messages (conversation_id, message_type, content, token_count, error_message, query_executed, execution_time_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING id, conversation_id, message_type, content, created_at, token_count, error_message, query_executed, execution_time_ms
	`

	var result ChatMessage
	err := r.db.ReadWrite.QueryRowContext(ctx, query, msg.ConversationID, msg.MessageType, msg.Content, msg.TokenCount, msg.ErrorMessage, msg.QueryExecuted, msg.ExecutionTimeMs).Scan(
		&result.ID,
		&result.ConversationID,
		&result.MessageType,
		&result.Content,
		&result.CreatedAt,
		&result.TokenCount,
		&result.ErrorMessage,
		&result.QueryExecuted,
		&result.ExecutionTimeMs,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to create message", "error", err)
		return ChatMessage{}, fmt.Errorf("create message: %w", err)
	}

	return result, nil
}

func (r *messageRepositoryImpl) Get(ctx context.Context, id int) (ChatMessage, error) {
	// Implementation not needed for current functionality
	return ChatMessage{}, errors.New("not implemented")
}

func (r *messageRepositoryImpl) ListByConversation(ctx context.Context, conversationID int) ([]ChatMessage, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if userID == 0 {
		return nil, errors.New("user not authenticated")
	}

	// First verify the conversation belongs to the user
	convQuery := `SELECT id FROM conversations WHERE id = ? AND user_id = ?`
	var convID int
	err := r.db.ReadOnly.QueryRowContext(ctx, convQuery, conversationID, userID).Scan(&convID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("verify conversation ownership: %w", err)
	}

	query := `
		SELECT id, conversation_id, message_type, content, created_at, token_count, error_message, query_executed, execution_time_ms
		FROM chat_messages
		WHERE conversation_id = ?
		ORDER BY created_at ASC
	`

	rows, err := r.db.ReadOnly.QueryContext(ctx, query, conversationID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to list messages", "conversation_id", conversationID, "error", err)
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		err := rows.Scan(
			&msg.ID,
			&msg.ConversationID,
			&msg.MessageType,
			&msg.Content,
			&msg.CreatedAt,
			&msg.TokenCount,
			&msg.ErrorMessage,
			&msg.QueryExecuted,
			&msg.ExecutionTimeMs,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to scan message", "error", err)
			return nil, fmt.Errorf("scan message: %w", err)
		}

		// TODO: Load visualizations for this message when needed
		msg.Visualizations = nil

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "failed to iterate messages", "error", err)
		return nil, fmt.Errorf("iterate messages: %w", err)
	}

	return messages, nil
}

func (r *messageRepositoryImpl) Update(ctx context.Context, id int, updateFn func(*ChatMessage) (bool, error)) error {
	// Implementation not needed for current functionality
	return errors.New("not implemented")
}

func (r *visualizationRepositoryImpl) Create(ctx context.Context, viz Visualization) (Visualization, error) {
	query := `
		INSERT INTO message_visualizations (message_id, chart_type, chart_config, data_query)
		VALUES (?, ?, ?, ?)
		RETURNING id, message_id, chart_type, chart_config, data_query, created_at
	`

	var result Visualization
	err := r.db.ReadWrite.QueryRowContext(ctx, query, viz.MessageID, viz.ChartType, viz.ChartConfig, viz.DataQuery).Scan(
		&result.ID,
		&result.MessageID,
		&result.ChartType,
		&result.ChartConfig,
		&result.DataQuery,
		&result.CreatedAt,
	)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to create visualization", "error", err)
		return Visualization{}, fmt.Errorf("create visualization: %w", err)
	}

	return result, nil
}

func (r *visualizationRepositoryImpl) Get(ctx context.Context, id int) (Visualization, error) {
	// Implementation not needed for current functionality
	return Visualization{}, errors.New("not implemented")
}

func (r *visualizationRepositoryImpl) ListByMessage(ctx context.Context, messageID int) ([]Visualization, error) {
	query := `
		SELECT id, message_id, chart_type, chart_config, data_query, created_at
		FROM message_visualizations
		WHERE message_id = ?
		ORDER BY created_at ASC
	`

	rows, err := r.db.ReadOnly.QueryContext(ctx, query, messageID)
	if err != nil {
		r.logger.ErrorContext(ctx, "failed to list visualizations", "message_id", messageID, "error", err)
		return nil, fmt.Errorf("list visualizations: %w", err)
	}
	defer rows.Close()

	var visualizations []Visualization
	for rows.Next() {
		var viz Visualization
		err := rows.Scan(
			&viz.ID,
			&viz.MessageID,
			&viz.ChartType,
			&viz.ChartConfig,
			&viz.DataQuery,
			&viz.CreatedAt,
		)
		if err != nil {
			r.logger.ErrorContext(ctx, "failed to scan visualization", "error", err)
			return nil, fmt.Errorf("scan visualization: %w", err)
		}
		visualizations = append(visualizations, viz)
	}

	if err := rows.Err(); err != nil {
		r.logger.ErrorContext(ctx, "failed to iterate visualizations", "error", err)
		return nil, fmt.Errorf("iterate visualizations: %w", err)
	}

	return visualizations, nil
}
