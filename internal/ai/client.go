package ai

import (
	"context"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"os"
)

type Client struct {
	client *openai.Client
}

func NewClient() Client {
	return Client{
		client: openai.NewClient(os.Getenv("OPENAI_API_KEY")),
	}
}

const MaxTokens = 4096

func (c *Client) SyncCompletion(messages []openai.ChatCompletionMessage) (openai.ChatCompletionResponse, error) {
	completion, err := c.client.CreateChatCompletion(
		context.TODO(),
		openai.ChatCompletionRequest{ //nolint:exhaustruct // this is better for readability
			Model:     openai.GPT3Dot5Turbo1106,
			MaxTokens: MaxTokens,
			Messages:  messages,
		},
	)
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("create chat completion: %w", err)
	}
	return completion, nil
}

func (c *Client) StreamCompletion(messages []openai.ChatCompletionMessage) (*openai.ChatCompletionStream, error) {
	completion, err := c.client.CreateChatCompletionStream(
		context.TODO(),
		openai.ChatCompletionRequest{ //nolint:exhaustruct // this is better for readability
			Model:    openai.GPT3Dot5Turbo,
			Messages: messages,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create chat completion stream: %w", err)
	}
	return completion, nil
}
