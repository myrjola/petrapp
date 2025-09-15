package models

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/chatbot"
)

func TestConversation_Validation(t *testing.T) {
	tests := []struct {
		name         string
		conversation chatbot.Conversation
		expectValid  bool
		expectedErr  string
	}{
		{
			name: "valid conversation",
			conversation: chatbot.Conversation{
				ID:        1,
				UserID:    123,
				Title:     stringPtr("Workout Planning"),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				IsActive:  true,
			},
			expectValid: true,
		},
		{
			name: "valid conversation without title",
			conversation: chatbot.Conversation{
				ID:        2,
				UserID:    123,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				IsActive:  true,
			},
			expectValid: true,
		},
		{
			name: "missing user ID",
			conversation: chatbot.Conversation{
				ID:        3,
				Title:     stringPtr("Test Conversation"),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				IsActive:  true,
			},
			expectValid: false,
			expectedErr: "user ID is required",
		},
		{
			name: "title too long",
			conversation: chatbot.Conversation{
				ID:        4,
				UserID:    123,
				Title:     stringPtr(strings.Repeat("a", 201)), // Exceeds 200 char limit
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				IsActive:  true,
			},
			expectValid: false,
			expectedErr: "title exceeds maximum length",
		},
		{
			name: "created after updated",
			conversation: chatbot.Conversation{
				ID:        5,
				UserID:    123,
				Title:     stringPtr("Time Test"),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now().Add(-time.Hour), // Updated before created
				IsActive:  true,
			},
			expectValid: false,
			expectedErr: "updated_at cannot be before created_at",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConversation(&tt.conversation)

			if tt.expectValid {
				if err != nil {
					t.Errorf("Expected conversation to be valid, but got error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected conversation to be invalid, but got no error")
				} else if tt.expectedErr != "" && !contains(err.Error(), tt.expectedErr) {
					t.Errorf("Expected error to contain '%s', but got: %v", tt.expectedErr, err)
				}
			}
		})
	}
}

func TestConversation_StateTransitions(t *testing.T) {
	now := time.Now()
	conversation := &chatbot.Conversation{
		ID:        1,
		UserID:    123,
		Title:     stringPtr("Test Conversation"),
		CreatedAt: now,
		UpdatedAt: now,
		IsActive:  true,
	}

	// Test activation
	t.Run("activate conversation", func(t *testing.T) {
		conversation.IsActive = false
		ActivateConversation(conversation)

		if !conversation.IsActive {
			t.Error("Expected conversation to be active")
		}
		if !conversation.UpdatedAt.After(now) {
			t.Error("Expected updated_at to be updated")
		}
	})

	// Test deactivation
	t.Run("deactivate conversation", func(t *testing.T) {
		updateTime := conversation.UpdatedAt
		time.Sleep(time.Millisecond) // Ensure time difference

		DeactivateConversation(conversation)

		if conversation.IsActive {
			t.Error("Expected conversation to be inactive")
		}
		if !conversation.UpdatedAt.After(updateTime) {
			t.Error("Expected updated_at to be updated on deactivation")
		}
	})

	// Test archive
	t.Run("archive conversation", func(t *testing.T) {
		updateTime := conversation.UpdatedAt
		time.Sleep(time.Millisecond)

		ArchiveConversation(conversation)

		if conversation.IsActive {
			t.Error("Expected conversation to be inactive after archiving")
		}
		if !conversation.UpdatedAt.After(updateTime) {
			t.Error("Expected updated_at to be updated on archiving")
		}
	})
}

func TestConversation_UpdateActivity(t *testing.T) {
	now := time.Now()
	conversation := &chatbot.Conversation{
		ID:        1,
		UserID:    123,
		Title:     stringPtr("Activity Test"),
		CreatedAt: now,
		UpdatedAt: now,
		IsActive:  true,
	}

	originalUpdatedAt := conversation.UpdatedAt
	time.Sleep(time.Millisecond)

	UpdateConversationActivity(conversation)

	if !conversation.UpdatedAt.After(originalUpdatedAt) {
		t.Error("Expected updated_at to be updated")
	}

	if !conversation.IsActive {
		t.Error("Expected conversation to remain active")
	}
}

func TestConversation_GenerateTitle(t *testing.T) {
	tests := []struct {
		name           string
		firstMessage   string
		expectedPrefix string
		maxLength      int
	}{
		{
			name:           "short message",
			firstMessage:   "Hello, how are you?",
			expectedPrefix: "Hello, how are you?",
			maxLength:      50,
		},
		{
			name:           "long message gets truncated",
			firstMessage:   "This is a very long message that should be truncated because it exceeds the maximum title length that we want to have for conversations in the system",
			expectedPrefix: "This is a very long message that should be",
			maxLength:      50,
		},
		{
			name:           "empty message",
			firstMessage:   "",
			expectedPrefix: "New Conversation",
			maxLength:      50,
		},
		{
			name:           "whitespace only",
			firstMessage:   "   \n\t  ",
			expectedPrefix: "New Conversation",
			maxLength:      50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title := GenerateConversationTitle(tt.firstMessage, tt.maxLength)

			if len(title) > tt.maxLength {
				t.Errorf("Generated title too long: %d chars (max: %d)", len(title), tt.maxLength)
			}

			if tt.firstMessage == "" || isWhitespace(tt.firstMessage) {
				if title != "New Conversation" {
					t.Errorf("Expected default title for empty message, got: %s", title)
				}
			} else {
				if !startsWith(title, tt.expectedPrefix) {
					t.Errorf("Expected title to start with '%s', got: %s", tt.expectedPrefix, title)
				}
			}
		})
	}
}

func TestConversation_IsStale(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		updatedAt   time.Time
		threshold   time.Duration
		expectStale bool
	}{
		{
			name:        "recent conversation",
			updatedAt:   now.Add(-10 * time.Minute),
			threshold:   24 * time.Hour,
			expectStale: false,
		},
		{
			name:        "stale conversation",
			updatedAt:   now.Add(-25 * time.Hour),
			threshold:   24 * time.Hour,
			expectStale: true,
		},
		{
			name:        "exactly at threshold",
			updatedAt:   now.Add(-24 * time.Hour),
			threshold:   24 * time.Hour,
			expectStale: true, // Stale if exactly at threshold (>= threshold)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conversation := &chatbot.Conversation{
				UpdatedAt: tt.updatedAt,
			}

			isStale := IsConversationStale(conversation, tt.threshold)
			if isStale != tt.expectStale {
				t.Errorf("IsConversationStale() = %v, want %v", isStale, tt.expectStale)
			}
		})
	}
}

// Helper functions that would be implemented in the actual models package

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		len(s) > len(substr) && findInString(s, substr)
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func isWhitespace(s string) bool {
	for _, char := range s {
		if char != ' ' && char != '\t' && char != '\n' && char != '\r' {
			return false
		}
	}
	return true
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// Helper function to create string pointers.
func stringPtr(s string) *string {
	return &s
}

// Stub implementations for validation and state management functions
// These would normally be implemented in the actual models package

func ValidateConversation(conv *chatbot.Conversation) error {
	if conv.UserID == 0 {
		return errors.New("user ID is required")
	}
	if conv.Title != nil && len(*conv.Title) > 200 {
		return errors.New("title exceeds maximum length")
	}
	if conv.UpdatedAt.Before(conv.CreatedAt) {
		return errors.New("updated_at cannot be before created_at")
	}
	return nil
}

func ActivateConversation(conv *chatbot.Conversation) {
	conv.IsActive = true
	conv.UpdatedAt = time.Now()
}

func DeactivateConversation(conv *chatbot.Conversation) {
	conv.IsActive = false
	conv.UpdatedAt = time.Now()
}

func ArchiveConversation(conv *chatbot.Conversation) {
	conv.IsActive = false
	conv.UpdatedAt = time.Now()
}

func UpdateConversationActivity(conv *chatbot.Conversation) {
	conv.UpdatedAt = time.Now()
}

func GenerateConversationTitle(firstMessage string, maxLength int) string {
	if strings.TrimSpace(firstMessage) == "" {
		return "New Conversation"
	}

	if len(firstMessage) <= maxLength {
		return firstMessage
	}

	// Truncate and try to break at word boundary
	truncated := firstMessage[:maxLength]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > 0 {
		return truncated[:lastSpace]
	}

	return truncated
}

func IsConversationStale(conv *chatbot.Conversation, threshold time.Duration) bool {
	return time.Since(conv.UpdatedAt) >= threshold
}
