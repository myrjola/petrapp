package performance

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/chatbot"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// Performance test for 100 concurrent conversations.
// Tests the system's ability to handle multiple concurrent chat sessions
// without blocking or performance degradation.
func TestChatbot_ConcurrentConversations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	logger := testhelpers.NewLogger(t)

	// Create test database
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Create chatbot service
	service := chatbot.NewService(db, logger, "test-api-key")

	// Test parameters
	const (
		numConcurrentConversations = 100
		messagesPerConversation    = 5
		maxTestDuration            = 30 * time.Second
	)

	// Create test users
	userIDs := make([]int, numConcurrentConversations)
	for i := 0; i < numConcurrentConversations; i++ {
		userID, err := testhelpers.InsertUser(db, fmt.Sprintf("user%d@test.com", i))
		if err != nil {
			t.Fatalf("Failed to create user %d: %v", i, err)
		}
		userIDs[i] = userID
	}

	// Channel to collect results
	results := make(chan testResult, numConcurrentConversations)

	// Start timer
	startTime := time.Now()

	// Create context with timeout
	testCtx, cancel := context.WithTimeout(ctx, maxTestDuration)
	defer cancel()

	// Launch concurrent conversations
	var wg sync.WaitGroup
	for i := 0; i < numConcurrentConversations; i++ {
		wg.Add(1)
		go func(userIndex int) {
			defer wg.Done()
			runConversationTest(testCtx, service, userIDs[userIndex], userIndex, messagesPerConversation, results)
		}(i)
	}

	// Wait for all conversations to complete or timeout
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var (
		completedConversations int
		totalMessages          int
		totalErrors            int
		minLatency             = time.Hour
		maxLatency             time.Duration
		totalLatency           time.Duration
	)

	for result := range results {
		completedConversations++
		totalMessages += result.MessagesProcessed
		totalErrors += result.Errors

		if result.AverageLatency > 0 {
			if result.AverageLatency < minLatency {
				minLatency = result.AverageLatency
			}
			if result.AverageLatency > maxLatency {
				maxLatency = result.AverageLatency
			}
			totalLatency += result.AverageLatency
		}
	}

	duration := time.Since(startTime)

	// Performance assertions
	if completedConversations == 0 {
		t.Fatal("No conversations completed successfully")
	}

	// Log performance metrics
	t.Logf("Performance Test Results:")
	t.Logf("  Duration: %v", duration)
	t.Logf("  Completed conversations: %d/%d (%.1f%%)",
		completedConversations, numConcurrentConversations,
		float64(completedConversations)/float64(numConcurrentConversations)*100)
	t.Logf("  Total messages processed: %d", totalMessages)
	t.Logf("  Total errors: %d", totalErrors)
	t.Logf("  Messages/second: %.1f", float64(totalMessages)/duration.Seconds())

	if completedConversations > 0 {
		avgLatency := totalLatency / time.Duration(completedConversations)
		t.Logf("  Average latency: %v", avgLatency)
		t.Logf("  Min latency: %v", minLatency)
		t.Logf("  Max latency: %v", maxLatency)

		// Performance thresholds
		if avgLatency > 5*time.Second {
			t.Errorf("Average latency too high: %v (max: 5s)", avgLatency)
		}
		if maxLatency > 10*time.Second {
			t.Errorf("Max latency too high: %v (max: 10s)", maxLatency)
		}
	}

	// Error rate threshold
	errorRate := float64(totalErrors) / float64(totalMessages) * 100
	if errorRate > 5.0 {
		t.Errorf("Error rate too high: %.1f%% (max: 5%%)", errorRate)
	}

	// Completion rate threshold
	completionRate := float64(completedConversations) / float64(numConcurrentConversations) * 100
	if completionRate < 95.0 {
		t.Errorf("Completion rate too low: %.1f%% (min: 95%%)", completionRate)
	}

	// Test should complete within max duration
	if duration > maxTestDuration {
		t.Errorf("Test took too long: %v (max: %v)", duration, maxTestDuration)
	}
}

type testResult struct {
	ConversationID    int
	MessagesProcessed int
	Errors            int
	AverageLatency    time.Duration
}

func runConversationTest(ctx context.Context, service *chatbot.Service, userID, userIndex, messageCount int, results chan<- testResult) {
	userCtx := context.WithValue(ctx, "user_id", userID)
	result := testResult{
		ConversationID: userIndex,
	}

	// Create conversation
	conversation, err := service.CreateConversation(userCtx, fmt.Sprintf("Performance Test Conversation %d", userIndex))
	if err != nil {
		result.Errors++
		results <- result
		return
	}
	result.ConversationID = conversation.ID

	var totalLatency time.Duration

	// Send messages
	for i := 0; i < messageCount; i++ {
		select {
		case <-ctx.Done():
			results <- result
			return
		default:
		}

		message := fmt.Sprintf("Test message %d from user %d", i+1, userIndex)

		start := time.Now()
		_, err := service.ProcessMessage(userCtx, conversation.ID, message)
		latency := time.Since(start)

		if err != nil {
			result.Errors++
		} else {
			result.MessagesProcessed++
			totalLatency += latency
		}

		// Small delay to simulate realistic usage
		select {
		case <-ctx.Done():
			results <- result
			return
		case <-time.After(100 * time.Millisecond):
		}
	}

	if result.MessagesProcessed > 0 {
		result.AverageLatency = totalLatency / time.Duration(result.MessagesProcessed)
	}

	results <- result
}
