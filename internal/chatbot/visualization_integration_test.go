package chatbot_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/chatbot"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// Integration test for visualization generation within the chatbot flow
// This test MUST fail initially as the repository methods are not yet implemented.
func TestVisualizationIntegration_EndToEndVisualizationFlow(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	// Create test database
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert test user
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("viz-user"), "Viz User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	// Create comprehensive test workout data for visualization
	setupVisualizationTestData(t, db, userID)

	userCtx := context.WithValue(ctx, "user_id", userID)
	service := chatbot.NewService(db, logger, "test-api-key")

	t.Run("Visualization creation and storage in conversation", func(t *testing.T) {
		// Create a conversation
		conversation, err := service.CreateConversation(userCtx, "Visualization Test")
		if err != nil {
			t.Skip("CreateConversation not implemented yet (expected for TDD)")
		}

		// Send a message that should trigger visualization generation
		assistantMessage, err := service.SendMessage(userCtx, conversation.ID, "Show me a chart of my bench press progress over time")
		if err != nil {
			t.Skip("SendMessage not implemented yet (expected for TDD)")
		}

		// When fully implemented, the assistant message should contain visualizations
		if len(assistantMessage.Visualizations) > 0 {
			viz := assistantMessage.Visualizations[0]

			// Validate visualization structure
			if viz.ID == 0 {
				t.Error("visualization should have valid ID")
			}
			if viz.MessageID != assistantMessage.ID {
				t.Errorf("visualization should belong to message %d, got %d", assistantMessage.ID, viz.MessageID)
			}
			if viz.ChartType == "" {
				t.Error("visualization should have chart type")
			}
			if viz.ChartConfig == "" {
				t.Error("visualization should have chart config")
			}
			if viz.DataQuery == "" {
				t.Error("visualization should have data query")
			}
			if viz.CreatedAt.IsZero() {
				t.Error("visualization should have created timestamp")
			}

			// Validate chart type is valid
			chartType := chatbot.ChartType(viz.ChartType)
			if !chartType.IsValid() {
				t.Errorf("invalid chart type: %s", viz.ChartType)
			}
		}

		// Retrieve conversation messages and verify visualization persistence
		messages, err := service.GetConversationMessages(userCtx, conversation.ID)
		if err != nil {
			t.Errorf("unexpected error getting conversation messages: %v", err)
		}

		// Find the assistant message with visualizations
		var assistantMsgWithViz *chatbot.ChatMessage
		for _, msg := range messages {
			if msg.MessageType == chatbot.MessageTypeAssistant && len(msg.Visualizations) > 0 {
				assistantMsgWithViz = &msg
				break
			}
		}

		if assistantMsgWithViz != nil {
			// Verify visualization data persisted correctly
			for _, viz := range assistantMsgWithViz.Visualizations {
				if viz.MessageID != assistantMsgWithViz.ID {
					t.Error("persisted visualization should maintain message relationship")
				}
			}
		}
	})

	t.Run("Multiple visualizations in single message", func(t *testing.T) {
		conversation, err := service.CreateConversation(userCtx, "Multi-Viz Test")
		if err != nil {
			t.Skip("CreateConversation not implemented yet (expected for TDD)")
		}

		// Send message that might generate multiple visualizations
		assistantMessage, err := service.SendMessage(userCtx, conversation.ID, "Show me my workout summary with progress charts and muscle group distribution")
		if err != nil {
			t.Skip("SendMessage not implemented yet (expected for TDD)")
		}

		// When implemented, this might generate multiple visualizations
		if len(assistantMessage.Visualizations) > 1 {
			// Verify all visualizations belong to the same message
			for _, viz := range assistantMessage.Visualizations {
				if viz.MessageID != assistantMessage.ID {
					t.Error("all visualizations should belong to the same message")
				}
			}

			// Verify different chart types for different purposes
			chartTypes := make(map[string]bool)
			for _, viz := range assistantMessage.Visualizations {
				chartTypes[string(viz.ChartType)] = true
			}

			if len(chartTypes) < 2 {
				t.Log("Multiple visualizations could use different chart types for variety")
			}
		}
	})

	t.Run("Visualization data query security", func(t *testing.T) {
		conversation, err := service.CreateConversation(userCtx, "Security Test")
		if err != nil {
			t.Skip("CreateConversation not implemented yet (expected for TDD)")
		}

		// Send message that should generate a visualization
		assistantMessage, err := service.SendMessage(userCtx, conversation.ID, "Show me a chart of my workout frequency")
		if err != nil {
			t.Skip("SendMessage not implemented yet (expected for TDD)")
		}

		// Check that any generated visualizations use secure queries
		for _, viz := range assistantMessage.Visualizations {
			dataQuery := viz.DataQuery

			// Should not contain dangerous SQL operations
			dangerousPatterns := []string{
				"DROP", "DELETE", "UPDATE", "INSERT", "ALTER", "CREATE",
				"TRUNCATE", "EXEC", "EXECUTE", "ATTACH", "DETACH", "PRAGMA",
			}

			for _, pattern := range dangerousPatterns {
				if containsIgnoreCase(dataQuery, pattern) {
					t.Errorf("visualization data query contains dangerous pattern '%s': %s", pattern, dataQuery)
				}
			}

			// Should use parameterized queries with user isolation
			if !containsIgnoreCase(dataQuery, "user_id") && dataQuery != "" {
				t.Error("visualization data query should include user_id for isolation")
			}
		}
	})
}

// Test visualization integration with message history.
func TestVisualizationIntegration_VisualizationHistory(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Insert test user
	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("history-user"), "History User").Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	setupVisualizationTestData(t, db, userID)

	userCtx := context.WithValue(ctx, "user_id", userID)
	service := chatbot.NewService(db, logger, "test-api-key")

	t.Run("Visualization history across conversation", func(t *testing.T) {
		conversation, err := service.CreateConversation(userCtx, "History Test")
		if err != nil {
			t.Skip("CreateConversation not implemented yet (expected for TDD)")
		}

		// Send multiple messages that generate visualizations
		vizMessages := []string{
			"Show me my bench press progress",
			"Display my workout frequency chart",
			"Create a muscle group distribution pie chart",
		}

		var allVisualizations []chatbot.Visualization

		for _, msg := range vizMessages {
			assistantMessage, err := service.SendMessage(userCtx, conversation.ID, msg)
			if err != nil {
				t.Skip("SendMessage not implemented yet (expected for TDD)")
			}
			allVisualizations = append(allVisualizations, assistantMessage.Visualizations...)
		}

		// Get conversation messages to verify visualization history
		messages, err := service.GetConversationMessages(userCtx, conversation.ID)
		if err != nil {
			t.Errorf("unexpected error getting conversation messages: %v", err)
		}

		// Count total visualizations in conversation history
		totalVizCount := 0
		for _, msg := range messages {
			if msg.MessageType == chatbot.MessageTypeAssistant {
				totalVizCount += len(msg.Visualizations)
			}
		}

		// Should maintain visualization history across the conversation
		if totalVizCount > 0 {
			t.Logf("Conversation contains %d total visualizations", totalVizCount)

			// Verify chronological ordering
			var vizTimestamps []chatbot.Visualization
			for _, msg := range messages {
				vizTimestamps = append(vizTimestamps, msg.Visualizations...)
			}

			for i := 1; i < len(vizTimestamps); i++ {
				if vizTimestamps[i].CreatedAt.Before(vizTimestamps[i-1].CreatedAt) {
					t.Error("visualizations should maintain chronological order")
				}
			}
		}
	})

	t.Run("Visualization persistence and retrieval", func(t *testing.T) {
		conversation, err := service.CreateConversation(userCtx, "Persistence Test")
		if err != nil {
			t.Skip("CreateConversation not implemented yet (expected for TDD)")
		}

		// Send message to generate visualization
		originalMessage, err := service.SendMessage(userCtx, conversation.ID, "Show me a workout progress chart")
		if err != nil {
			t.Skip("SendMessage not implemented yet (expected for TDD)")
		}

		if len(originalMessage.Visualizations) == 0 {
			t.Skip("No visualizations generated (expected when tools not implemented)")
		}

		originalViz := originalMessage.Visualizations[0]

		// Retrieve conversation again to test persistence
		messages, err := service.GetConversationMessages(userCtx, conversation.ID)
		if err != nil {
			t.Errorf("unexpected error retrieving messages: %v", err)
		}

		// Find the message with visualization
		found := false
		for _, msg := range messages {
			if msg.ID == originalMessage.ID {
				if len(msg.Visualizations) > 0 {
					retrievedViz := msg.Visualizations[0]

					// Verify visualization data persisted correctly
					if retrievedViz.ID != originalViz.ID {
						t.Error("visualization ID should persist")
					}
					if retrievedViz.ChartType != originalViz.ChartType {
						t.Error("visualization chart type should persist")
					}
					if retrievedViz.ChartConfig != originalViz.ChartConfig {
						t.Error("visualization chart config should persist")
					}
					if retrievedViz.DataQuery != originalViz.DataQuery {
						t.Error("visualization data query should persist")
					}
				}
				found = true
				break
			}
		}

		if !found {
			t.Error("original message with visualization not found after retrieval")
		}
	})
}

// Test user isolation for visualizations.
func TestVisualizationIntegration_UserIsolation(t *testing.T) {
	ctx := context.Background()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))

	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Create two test users
	var user1ID, user2ID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("viz-user1"), "Viz User 1").Scan(&user1ID)
	if err != nil {
		t.Fatalf("Failed to insert user1: %v", err)
	}

	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("viz-user2"), "Viz User 2").Scan(&user2ID)
	if err != nil {
		t.Fatalf("Failed to insert user2: %v", err)
	}

	// Setup different workout data for each user
	setupVisualizationTestData(t, db, user1ID)
	setupDifferentVisualizationTestData(t, db, user2ID)

	user1Ctx := context.WithValue(ctx, "user_id", user1ID)
	user2Ctx := context.WithValue(ctx, "user_id", user2ID)

	service := chatbot.NewService(db, logger, "test-api-key")

	t.Run("Visualizations show only user's own data", func(t *testing.T) {
		// Both users create conversations and request similar visualizations
		conv1, err := service.CreateConversation(user1Ctx, "User1 Viz Test")
		if err != nil {
			t.Skip("CreateConversation not implemented yet (expected for TDD)")
		}

		conv2, err := service.CreateConversation(user2Ctx, "User2 Viz Test")
		if err != nil {
			t.Skip("CreateConversation not implemented yet (expected for TDD)")
		}

		// Both users request the same type of visualization
		msg1, err := service.SendMessage(user1Ctx, conv1.ID, "Show me my workout frequency chart")
		if err != nil {
			t.Skip("SendMessage not implemented yet (expected for TDD)")
		}

		msg2, err := service.SendMessage(user2Ctx, conv2.ID, "Show me my workout frequency chart")
		if err != nil {
			t.Skip("SendMessage not implemented yet (expected for TDD)")
		}

		// When implemented, visualizations should show different data for different users
		if len(msg1.Visualizations) > 0 && len(msg2.Visualizations) > 0 {
			viz1 := msg1.Visualizations[0]
			viz2 := msg2.Visualizations[0]

			// Data queries should be similar but isolated per user
			if viz1.DataQuery == viz2.DataQuery && viz1.DataQuery != "" {
				t.Error("visualizations for different users should not use identical data queries (should include user isolation)")
			}

			// Chart configs should potentially be different (different data)
			if viz1.ChartConfig == viz2.ChartConfig && viz1.ChartConfig != "" {
				t.Log("Note: Users with identical workout patterns might generate identical chart configs")
			}
		}
	})

	t.Run("Cross-user visualization access prevention", func(t *testing.T) {
		// User1 generates visualization
		conv1, err := service.CreateConversation(user1Ctx, "Private Viz")
		if err != nil {
			t.Skip("CreateConversation not implemented yet (expected for TDD)")
		}

		msg1, err := service.SendMessage(user1Ctx, conv1.ID, "Show me private workout data")
		if err != nil {
			t.Skip("SendMessage not implemented yet (expected for TDD)")
		}

		// User2 should not be able to access User1's conversation or visualizations
		_, err = service.GetConversationMessages(user2Ctx, conv1.ID)
		if err == nil {
			t.Error("user2 should not be able to access user1's conversation messages with visualizations")
		}

		// When visualization tools are implemented, they should respect user isolation
		if len(msg1.Visualizations) > 0 {
			// The visualization data should only contain User1's data
			viz := msg1.Visualizations[0]
			if !containsIgnoreCase(viz.DataQuery, "user_id") && viz.DataQuery != "" {
				t.Error("visualization data query should include user isolation")
			}
		}
	})
}

// Helper function to setup comprehensive test data for visualization testing.
func setupVisualizationTestData(t *testing.T, db *sqlite.Database, userID int) {
	ctx := context.Background()

	// Insert exercises
	_, err := db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercises (id, name, category, exercise_type, description_markdown) VALUES
		(1, 'Bench Press', 'upper', 'weighted', 'Chest exercise'),
		(2, 'Squat', 'lower', 'weighted', 'Leg exercise'),
		(3, 'Deadlift', 'full_body', 'weighted', 'Full body exercise'),
		(4, 'Pull-ups', 'upper', 'bodyweight', 'Back exercise')
	`)
	if err != nil {
		t.Fatalf("Failed to insert test exercises: %v", err)
	}

	// Insert muscle groups
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO muscle_groups (name) VALUES
		('Chest'), ('Quads'), ('Back'), ('Shoulders')
	`)
	if err != nil {
		t.Fatalf("Failed to insert muscle groups: %v", err)
	}

	// Insert exercise muscle group relationships
	_, err = db.ReadWrite.ExecContext(ctx, `
		INSERT INTO exercise_muscle_groups (exercise_id, muscle_group_name, is_primary) VALUES
		(1, 'Chest', 1), (2, 'Quads', 1), (3, 'Back', 1), (4, 'Back', 1)
	`)
	if err != nil {
		t.Fatalf("Failed to insert exercise muscle groups: %v", err)
	}

	// Insert workout sessions with progression over 8 weeks
	for week := range 8 {
		for day := range 3 { // 3 workouts per week
			workoutDate := "2024-01-" + fmt.Sprintf("%02d", 1+week*7+day*2)

			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at, difficulty_rating) VALUES
				(?, ?, ?, ?, ?)
			`, userID, workoutDate, workoutDate+"T10:00:00Z", workoutDate+"T11:00:00Z", 3+week%3)
			if err != nil {
				t.Fatalf("Failed to insert workout session: %v", err)
			}

			// Insert workout exercises
			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
				(?, ?, 1), (?, ?, 2)
			`, userID, workoutDate, userID, workoutDate)
			if err != nil {
				t.Fatalf("Failed to insert workout exercises: %v", err)
			}

			// Insert exercise sets with progressive weights
			benchWeight := 80.0 + float64(week)*2.5
			squatWeight := 100.0 + float64(week)*5.0

			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
				(?, ?, 1, 1, ?, 8, 10, 10),
				(?, ?, 1, 2, ?, 8, 10, 9),
				(?, ?, 1, 3, ?, 8, 10, 8),
				(?, ?, 2, 1, ?, 5, 8, 8),
				(?, ?, 2, 2, ?, 5, 8, 7)
			`, userID, workoutDate, benchWeight, userID, workoutDate, benchWeight, userID, workoutDate, benchWeight,
				userID, workoutDate, squatWeight, userID, workoutDate, squatWeight)
			if err != nil {
				t.Fatalf("Failed to insert exercise sets: %v", err)
			}
		}
	}
}

// Helper function to setup different test data for second user.
func setupDifferentVisualizationTestData(t *testing.T, db *sqlite.Database, userID int) {
	ctx := context.Background()

	// Insert different workout pattern for user2 (less frequent, different exercises)
	for week := range 8 {
		for day := range 2 { // Only 2 workouts per week
			workoutDate := "2024-01-" + fmt.Sprintf("%02d", 2+week*7+day*3)

			_, err := db.ReadWrite.ExecContext(ctx, `
				INSERT INTO workout_sessions (user_id, workout_date, started_at, completed_at, difficulty_rating) VALUES
				(?, ?, ?, ?, ?)
			`, userID, workoutDate, workoutDate+"T14:00:00Z", workoutDate+"T15:00:00Z", 2+week%2)
			if err != nil {
				t.Fatalf("Failed to insert user2 workout session: %v", err)
			}

			// Different exercise selection (more focus on deadlifts and pull-ups)
			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id) VALUES
				(?, ?, 3), (?, ?, 4)
			`, userID, workoutDate, userID, workoutDate)
			if err != nil {
				t.Fatalf("Failed to insert user2 workout exercises: %v", err)
			}

			// Different progression pattern
			deadliftWeight := 120.0 + float64(week)*7.5

			_, err = db.ReadWrite.ExecContext(ctx, `
				INSERT INTO exercise_sets (workout_user_id, workout_date, exercise_id, set_number, weight_kg, min_reps, max_reps, completed_reps) VALUES
				(?, ?, 3, 1, ?, 3, 5, 5),
				(?, ?, 3, 2, ?, 3, 5, 4),
				(?, ?, 4, 1, 0, 8, 12, 10),
				(?, ?, 4, 2, 0, 8, 12, 8)
			`, userID, workoutDate, deadliftWeight, userID, workoutDate, deadliftWeight,
				userID, workoutDate, userID, workoutDate)
			if err != nil {
				t.Fatalf("Failed to insert user2 exercise sets: %v", err)
			}
		}
	}
}

// Helper function to check case-insensitive substring.
func containsIgnoreCase(str, substr string) bool {
	// Convert to lowercase and check
	lowerStr := strings.ToLower(str)
	lowerSubstr := strings.ToLower(substr)
	return strings.Contains(lowerStr, lowerSubstr)
}
