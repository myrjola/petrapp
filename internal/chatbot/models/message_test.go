package models

import (
	"strings"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/chatbot"
)

func TestChatMessage_Validation(t *testing.T) {
	tests := []struct {
		name        string
		message     chatbot.ChatMessage
		expectValid bool
		expectedErr string
	}{
		{
			name: "valid user message",
			message: chatbot.ChatMessage{
				ID:             1,
				ConversationID: 1,
				Role:           "user",
				Content:        "Hello, how are you?",
				CreatedAt:      time.Now(),
			},
			expectValid: true,
		},
		{
			name: "valid assistant message",
			message: chatbot.ChatMessage{
				ID:             2,
				ConversationID: 1,
				Role:           "assistant",
				Content:        "I'm doing well, thank you! How can I help you with your workout today?",
				CreatedAt:      time.Now(),
				TokenCount:     &[]int{45}[0],
			},
			expectValid: true,
		},
		{
			name: "empty content",
			message: chatbot.ChatMessage{
				ID:             3,
				ConversationID: 1,
				Role:           "user",
				Content:        "",
				CreatedAt:      time.Now(),
			},
			expectValid: false,
			expectedErr: "content cannot be empty",
		},
		{
			name: "content too long",
			message: chatbot.ChatMessage{
				ID:             4,
				ConversationID: 1,
				Role:           "user",
				Content:        strings.Repeat("a", 10001), // Exceeds 10000 char limit
				CreatedAt:      time.Now(),
			},
			expectValid: false,
			expectedErr: "content exceeds maximum length",
		},
		{
			name: "invalid role",
			message: chatbot.ChatMessage{
				ID:             5,
				ConversationID: 1,
				Role:           "system", // Not allowed in this context
				Content:        "System message",
				CreatedAt:      time.Now(),
			},
			expectValid: false,
			expectedErr: "role must be 'user' or 'assistant'",
		},
		{
			name: "missing conversation ID",
			message: chatbot.ChatMessage{
				ID:        6,
				Role:      "user",
				Content:   "Hello",
				CreatedAt: time.Now(),
			},
			expectValid: false,
			expectedErr: "conversation ID is required",
		},
		{
			name: "negative token count",
			message: chatbot.ChatMessage{
				ID:             7,
				ConversationID: 1,
				Role:           "assistant",
				Content:        "Response with invalid token count",
				CreatedAt:      time.Now(),
				TokenCount:     &[]int{-5}[0],
			},
			expectValid: false,
			expectedErr: "token count cannot be negative",
		},
		{
			name: "message with whitespace only",
			message: chatbot.ChatMessage{
				ID:             8,
				ConversationID: 1,
				Role:           "user",
				Content:        "   \n\t  ",
				CreatedAt:      time.Now(),
			},
			expectValid: false,
			expectedErr: "content cannot be empty or whitespace only",
		},
		{
			name: "valid message with visualization data",
			message: chatbot.ChatMessage{
				ID:                 9,
				ConversationID:     1,
				Role:               "assistant",
				Content:            "Here's your workout progress chart:",
				CreatedAt:          time.Now(),
				VisualizationData:  &[]string{`{"type":"line","data":[1,2,3]}`}[0],
				VisualizationTitle: &[]string{"Workout Progress"}[0],
			},
			expectValid: true,
		},
		{
			name: "invalid visualization data JSON",
			message: chatbot.ChatMessage{
				ID:                 10,
				ConversationID:     1,
				Role:               "assistant",
				Content:            "Here's your chart:",
				CreatedAt:          time.Now(),
				VisualizationData:  &[]string{`{"invalid json"`}[0],
				VisualizationTitle: &[]string{"Chart"}[0],
			},
			expectValid: false,
			expectedErr: "visualization data must be valid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMessage(&tt.message)

			if tt.expectValid {
				if err != nil {
					t.Errorf("Expected message to be valid, but got error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected message to be invalid, but got no error")
				} else if !strings.Contains(err.Error(), tt.expectedErr) {
					t.Errorf("Expected error to contain '%s', but got: %v", tt.expectedErr, err)
				}
			}
		})
	}
}

func TestChatMessage_SanitizeContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal content",
			input:    "Hello, how are you?",
			expected: "Hello, how are you?",
		},
		{
			name:     "content with extra whitespace",
			input:    "  Hello   world  \n\n  ",
			expected: "Hello   world",
		},
		{
			name:     "content with tabs and newlines",
			input:    "Line 1\n\nLine 2\t\tTab content",
			expected: "Line 1\n\nLine 2\t\tTab content",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \n\t  ",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeContent(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeContent(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestChatMessage_EstimateTokens(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		expectRange [2]int // [min, max] expected token count
	}{
		{
			name:        "short message",
			content:     "Hello",
			expectRange: [2]int{1, 3},
		},
		{
			name:        "medium message",
			content:     "Can you help me create a workout plan for building muscle?",
			expectRange: [2]int{10, 20},
		},
		{
			name:        "long message",
			content:     strings.Repeat("This is a longer message that should result in more tokens. ", 10),
			expectRange: [2]int{80, 200},
		},
		{
			name:        "empty message",
			content:     "",
			expectRange: [2]int{0, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := EstimateTokens(tt.content)

			if tokens < tt.expectRange[0] || tokens > tt.expectRange[1] {
				t.Errorf("EstimateTokens(%q) = %d, want range [%d, %d]",
					tt.content, tokens, tt.expectRange[0], tt.expectRange[1])
			}
		})
	}
}

func TestMessageRole_IsValid(t *testing.T) {
	validRoles := []string{"user", "assistant"}
	invalidRoles := []string{"system", "function", "", "invalid", "USER", "ASSISTANT"}

	for _, role := range validRoles {
		t.Run("valid_"+role, func(t *testing.T) {
			if !IsValidRole(role) {
				t.Errorf("Expected role '%s' to be valid", role)
			}
		})
	}

	for _, role := range invalidRoles {
		t.Run("invalid_"+role, func(t *testing.T) {
			if IsValidRole(role) {
				t.Errorf("Expected role '%s' to be invalid", role)
			}
		})
	}
}
