package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/logging"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"golang.org/x/sync/errgroup"
)

// AuthenticatedUser holds a client with valid session
type AuthenticatedUser struct {
	Client *e2etest.Client
	UserID string // or whatever identifier you need
}

func TestAuth(client *e2etest.Client) error {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var err error

	if _, err = client.Register(ctx); err != nil {
		return fmt.Errorf("register user: %w", err)
	}
	if _, err = client.Logout(ctx); err != nil {
		return fmt.Errorf("logout user: %w", err)
	}
	if _, err = client.Login(ctx); err != nil {
		return fmt.Errorf("login user: %w", err)
	}
	return nil
}

// RegisterAndAuthenticateUser creates a new user and logs them in
func RegisterAndAuthenticateUser(ctx context.Context, url, hostname string, userIndex int, logger *slog.Logger) (*AuthenticatedUser, error) {
	// Create a new client for this user (each needs their own session)
	client, err := e2etest.NewClient(url, hostname, url)
	if err != nil {
		return nil, fmt.Errorf("creating client for user %d: %w", userIndex, err)
	}

	// Register the user
	if _, err = client.Register(ctx); err != nil {
		return nil, fmt.Errorf("registering user %d: %w", userIndex, err)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "User registered and authenticated",
		slog.Int("user_index", userIndex))

	return &AuthenticatedUser{
		Client: client,
		UserID: fmt.Sprintf("user_%d", userIndex),
	}, nil
}

// SetupUsers registers and authenticates the specified number of users
func SetupUsers(ctx context.Context, url, hostname string, numUsers int, logger *slog.Logger) ([]*AuthenticatedUser, error) {
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting user registration", slog.Int("num_users", numUsers))

	var (
		users   = make([]*AuthenticatedUser, 0, numUsers)
		usersMu sync.Mutex
		wg      sync.WaitGroup
		errCh   = make(chan error, numUsers)
	)

	// Limit concurrency to avoid overwhelming the server
	semaphore := make(chan struct{}, 10) // Max 10 concurrent registrations

	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(userIndex int) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Create context with timeout for this user
			userCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			user, err := RegisterAndAuthenticateUser(userCtx, url, hostname, userIndex, logger)
			if err != nil {
				errCh <- fmt.Errorf("user %d: %w", userIndex, err)
				return
			}

			usersMu.Lock()
			users = append(users, user)
			usersMu.Unlock()
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errCh)

	// Check for errors
	var errors []error
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		logger.LogAttrs(ctx, slog.LevelError, "Some user registrations failed",
			slog.Int("failed_count", len(errors)),
			slog.Int("successful_count", len(users)))

		// Return first error for now, but you might want to handle this differently
		return users, fmt.Errorf("registration failures: %v", errors[0])
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "All users registered successfully",
		slog.Int("total_users", len(users)))

	return users, nil
}

// WorkoutScenario represents a complete workout flow for stress testing
func WorkoutScenario(ctx context.Context, user *AuthenticatedUser, logger *slog.Logger) error {
	client := user.Client
	today := time.Now().Format("2006-01-02")

	// Set workout preferences (CSRF form)
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		return fmt.Errorf("failed to get preferences: %w", err)
	}

	formData := map[string]string{
		"monday": "true",
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		return fmt.Errorf("failed to submit preferences: %w", err)
	}

	// Start a workout (CSRF form)
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		return fmt.Errorf("failed to start workout: %w", err)
	}

	// Find first exercise
	var exerciseID string
	doc.Find("a.exercise").Each(func(i int, s *goquery.Selection) {
		if i == 0 {
			href, exists := s.Attr("href")
			if exists {
				exerciseID = href[len("/workouts/"+today+"/exercises/"):]
			}
		}
	})

	if exerciseID == "" {
		return fmt.Errorf("no exercise found on workout page")
	}

	// Complete a set (CSRF form)
	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID); err != nil {
		return fmt.Errorf("failed to get exercise page: %w", err)
	}

	// Find set completion form
	form := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[type=submit]:contains('Done!')").Length() > 0
	}).First()

	if form.Length() == 0 {
		return fmt.Errorf("set completion form not found")
	}

	action, exists := form.Attr("action")
	if !exists {
		return fmt.Errorf("form has no action attribute")
	}

	// Submit set with random-ish data to simulate real usage
	setData := map[string]string{
		"weight": fmt.Sprintf("%.1f", 15.0+float64(time.Now().UnixNano()%20)), // 15-35kg range
		"reps":   fmt.Sprintf("%d", 8+time.Now().UnixNano()%8),                // 8-15 reps range
	}

	if doc, err = client.SubmitForm(ctx, doc, action, setData); err != nil {
		return fmt.Errorf("failed to complete set: %w", err)
	}

	logger.LogAttrs(ctx, slog.LevelDebug, "Workout scenario completed",
		slog.String("user_id", user.UserID),
		slog.String("exercise_id", exerciseID))

	return nil
}

// RunLoadTest performs the actual load testing with authenticated users
func RunLoadTest(ctx context.Context, users []*AuthenticatedUser, logger *slog.Logger) error {
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting load test", slog.Int("num_users", len(users)))

	iterations := 3 // Number of workout scenarios per user
	totalScenarios := len(users) * iterations

	// Counters for success/failure tracking
	var successCount, failureCount int64

	// Create errgroup with context and limit concurrency
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(20) // Max 20 concurrent operations

	// Launch all scenarios
	for _, user := range users {
		for i := 0; i < iterations; i++ {
			// Capture loop variables
			u := user
			iteration := i + 1

			g.Go(func() error {
				// Create context with timeout for this scenario
				scenarioCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()

				if err := WorkoutScenario(scenarioCtx, u, logger); err != nil {
					atomic.AddInt64(&failureCount, 1)
					// Log individual failures but don't stop the entire test
					logger.LogAttrs(scenarioCtx, slog.LevelWarn, "Scenario failed",
						slog.String("user_id", u.UserID),
						slog.Int("iteration", iteration),
						slog.Any("error", err))
					return nil // Don't propagate error to avoid stopping other scenarios
				}

				atomic.AddInt64(&successCount, 1)
				return nil
			})
		}
	}

	// Wait for all scenarios to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("load test failed: %w", err)
	}

	// Report results
	successRate := float64(successCount) / float64(totalScenarios) * 100

	logger.LogAttrs(ctx, slog.LevelInfo, "Load test completed",
		slog.Int("total_scenarios", totalScenarios),
		slog.Int64("successful", successCount),
		slog.Int64("failed", failureCount),
		slog.Float64("success_rate", successRate))

	// Consider test failed if success rate is too low
	if successRate < 95.0 {
		return fmt.Errorf("load test failed: success rate %.1f%% below threshold", successRate)
	}

	return nil
}

func main() {
	logger := testhelpers.NewLogger(os.Stdout)
	ctx := context.Background()

	if len(os.Args) != 2 {
		logger.LogAttrs(ctx, slog.LevelError, "usage: loadtest <hostname>")
		os.Exit(1)
	}

	var (
		hostname = os.Args[1]
		numUsers = 10
		start    = time.Now()
	)

	ctx = logging.WithAttrs(ctx, slog.String("hostname", hostname))

	// First, run the original smoke test to ensure basic functionality
	logger.LogAttrs(ctx, slog.LevelInfo, "Running smoke test first...")
	url := "https://" + hostname
	if strings.Contains(hostname, "localhost") {
		url = "http://" + hostname
		hostname = "localhost"
	}
	client, err := e2etest.NewClient(url, hostname, url)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "error creating client", slog.Any("error", err))
		os.Exit(1)
	}

	if err = client.WaitForReady(ctx, "/api/healthy"); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "server not ready in time", slog.Any("error", err))
		os.Exit(1)
	}

	if err = TestAuth(client); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "smoke test failed", slog.Any("error", err))
		os.Exit(1)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Smoke test passed ✓")

	// Setup users for load testing
	setupStart := time.Now()
	users, err := SetupUsers(ctx, url, hostname, numUsers, logger)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "failed to setup users", slog.Any("error", err))
		os.Exit(1)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "User setup completed",
		slog.Duration("setup_duration", time.Since(setupStart)),
		slog.Int("authenticated_users", len(users)))

	// Run load test
	loadTestStart := time.Now()
	if err = RunLoadTest(ctx, users, logger); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "load test failed", slog.Any("error", err))
		os.Exit(1)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Load test completed successfully 🙌",
		slog.Duration("total_duration", time.Since(start)),
		slog.Duration("load_test_duration", time.Since(loadTestStart)),
		slog.Int("users_tested", len(users)))
}
