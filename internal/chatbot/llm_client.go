package chatbot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// llmClient handles interactions with OpenAI GPT models.
type llmClient struct {
	client openai.Client
	logger *slog.Logger
	tools  []Tool
}

// Tool interface for function calling tools.
type Tool interface {
	ToOpenAIFunction() map[string]interface{}
	ExecuteFunction(ctx context.Context, functionName string, arguments string) (string, error)
}

// newLLMClient creates a new OpenAI client for the chatbot.
func newLLMClient(apiKey string, logger *slog.Logger) *llmClient {
	client := openai.NewClient(option.WithAPIKey(apiKey))

	return &llmClient{
		client: client,
		logger: logger,
		tools:  make([]Tool, 0),
	}
}

// RegisterTool adds a tool to the LLM client for function calling.
func (c *llmClient) RegisterTool(tool Tool) {
	c.tools = append(c.tools, tool)
}

// ChatRequest represents a request to the LLM.
type ChatRequest struct {
	Messages     []openai.ChatCompletionMessageParamUnion
	SystemPrompt *string
}

// ChatResponse represents a response from the LLM.
type ChatResponse struct {
	Content       *string
	FunctionCalls []openai.ChatCompletionMessageToolCall
	TokenUsage    openai.CompletionUsage
}

// GenerateResponse sends a chat completion request to OpenAI GPT-4.
func (c *llmClient) GenerateResponse(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	// Build the messages array starting with system prompt if provided
	var messages []openai.ChatCompletionMessageParamUnion

	if req.SystemPrompt != nil {
		messages = append(messages, openai.SystemMessage(*req.SystemPrompt))
	}

	messages = append(messages, req.Messages...)

	// Create chat completion request - function calling will be added later when OpenAI API is stabilized
	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModelGPT4o,
		Messages: messages,
	}

	// TODO: Add function calling support when OpenAI Go client is properly configured
	// For now, use text-based responses and parse function calls manually

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

		// TODO: Handle function call responses when OpenAI client supports it
		// if len(choice.Message.ToolCalls) > 0 {
		//     response.FunctionCalls = choice.Message.ToolCalls
		// }
	}

	c.logger.DebugContext(ctx, "received chat completion response",
		"completion_tokens", completion.Usage.CompletionTokens,
		"prompt_tokens", completion.Usage.PromptTokens,
		"total_tokens", completion.Usage.TotalTokens,
		"has_content", response.Content != nil)

	return response, nil
}

// ExecuteFunctionCall executes a function call from the LLM.
func (c *llmClient) ExecuteFunctionCall(ctx context.Context, functionCall openai.ChatCompletionMessageToolCall) (string, error) {
	functionName := functionCall.Function.Name
	arguments := functionCall.Function.Arguments

	c.logger.DebugContext(ctx, "executing function call",
		"function", functionName,
		"arguments", arguments)

	// Find the tool that handles this function
	for _, tool := range c.tools {
		result, err := tool.ExecuteFunction(ctx, functionName, arguments)
		if err != nil {
			// If error is not "unsupported function", return it
			if err.Error() != fmt.Sprintf("unsupported function: %s", functionName) {
				c.logger.ErrorContext(ctx, "function execution failed",
					"function", functionName,
					"error", err)
				return "", fmt.Errorf("function execution failed: %w", err)
			}
			// Otherwise continue to next tool
			continue
		}

		c.logger.DebugContext(ctx, "function executed successfully",
			"function", functionName,
			"result_length", len(result))

		return result, nil
	}

	err := fmt.Errorf("unknown function: %s", functionName)
	c.logger.ErrorContext(ctx, "unknown function called", "function", functionName)
	return "", err
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
