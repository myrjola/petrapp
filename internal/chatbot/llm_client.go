package chatbot

import (
	"context"
	"log/slog"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// llmClient handles interactions with OpenAI GPT models.
type llmClient struct {
	client openai.Client
	logger *slog.Logger
}

// newLLMClient creates a new OpenAI client for the chatbot.
func newLLMClient(apiKey string, logger *slog.Logger) *llmClient {
	client := openai.NewClient(option.WithAPIKey(apiKey))

	return &llmClient{
		client: client,
		logger: logger,
	}
}

// ChatRequest represents a request to the LLM.
type ChatRequest struct {
	Messages     []openai.ChatCompletionMessageParamUnion
	SystemPrompt *string
}

// ChatResponse represents a response from the LLM.
type ChatResponse struct {
	Content      *string
	FunctionCall *openai.ChatCompletionMessageToolCall
	TokenUsage   openai.CompletionUsage
}

// GenerateResponse sends a chat completion request to OpenAI GPT-4.
func (c *llmClient) GenerateResponse(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// Build the messages array starting with system prompt if provided
	var messages []openai.ChatCompletionMessageParamUnion

	if req.SystemPrompt != nil {
		messages = append(messages, openai.SystemMessage(*req.SystemPrompt))
	}

	messages = append(messages, req.Messages...)

	// Create chat completion request - simplified to match existing usage pattern
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModelGPT4o,
		Messages: messages,
	}

	c.logger.DebugContext(ctx, "sending chat completion request",
		"model", openai.ChatModelGPT4o,
		"message_count", len(messages))

	// Make the API call
	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		c.logger.ErrorContext(ctx, "openai chat completion failed", "error", err)
		return ChatResponse{}, err
	}

	// Extract response data
	response := ChatResponse{
		TokenUsage: completion.Usage,
	}

	if len(completion.Choices) > 0 {
		choice := completion.Choices[0]

		// Handle regular text response
		if choice.Message.Content != "" {
			response.Content = &choice.Message.Content
		}

		// Handle function call response
		if len(choice.Message.ToolCalls) > 0 {
			response.FunctionCall = &choice.Message.ToolCalls[0]
		}
	}

	c.logger.DebugContext(ctx, "received chat completion response",
		"completion_tokens", completion.Usage.CompletionTokens,
		"prompt_tokens", completion.Usage.PromptTokens,
		"total_tokens", completion.Usage.TotalTokens,
		"has_content", response.Content != nil,
		"has_function_call", response.FunctionCall != nil)

	return response, nil
}

// GetSystemPrompt returns the system prompt for the AI trainer.
func (c *llmClient) GetSystemPrompt() string {
	return `You are an AI personal trainer assistant for a fitness tracking app. You have access to the user's complete workout history including exercises, sets, reps, weights, and dates. You can query this data, generate visualizations, and provide personalized recommendations. Always be encouraging and supportive while providing accurate, data-driven insights. When querying data, ensure you're only accessing data for the current user (queries will be automatically filtered by user_id). Never make assumptions about data that isn't explicitly queried - always check the actual data first.

Guidelines:
- Always greet the user warmly and professionally
- When asked about data, query it first before responding
- Provide specific numbers and dates when available
- Offer visualizations when they would be helpful
- Give actionable recommendations based on data
- Explain the reasoning behind recommendations
- Be encouraging about progress and achievements
- Acknowledge plateaus or challenges constructively
- Suggest safe progressions based on current performance
- Remind users about rest and recovery when appropriate`
}
