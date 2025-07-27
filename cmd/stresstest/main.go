package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/logging"
	"github.com/myrjola/petrapp/internal/testhelpers"
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

// RunLoadTest performs the actual load testing with authenticated users
func RunLoadTest(ctx context.Context, users []*AuthenticatedUser, logger *slog.Logger) error {
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting load test", slog.Int("num_users", len(users)))

	// TODO: Implement your stress testing logic here
	// For now, just verify all users are still authenticated
	var wg sync.WaitGroup
	errCh := make(chan error, len(users))

	for i, user := range users {
		wg.Add(1)
		go func(userIndex int, u *AuthenticatedUser) {
			defer wg.Done()

			// Example: Make a request that requires authentication
			// You'll need to implement this based on your app's endpoints
			// For now, just log that we have an authenticated user
			logger.LogAttrs(ctx, slog.LevelInfo, "User ready for load test",
				slog.String("user_id", u.UserID))

			// TODO: Add your actual load testing requests here
			// e.g., u.Client.SomeAuthenticatedRequest(ctx)

		}(i, user)
	}

	wg.Wait()
	close(errCh)

	// Check for errors
	for err := range errCh {
		if err != nil {
			return fmt.Errorf("load test error: %w", err)
		}
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
		numUsers = 100
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
