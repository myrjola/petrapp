// Command stresstest drives concurrent workout scenarios against a deployed
// petrapp instance to produce load for performance profiling. Scenario logic
// lives in internal/loadtest so it is also exercised by the in-process smoke
// test in cmd/web.
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

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/loadtest"
	"github.com/myrjola/petrapp/internal/logging"
	"github.com/myrjola/petrapp/internal/testhelpers"
	"golang.org/x/sync/errgroup"
)

const (
	userRegistrationTimeout    = 30 * time.Second
	scenarioTimeout            = 30 * time.Second
	maxConcurrentRegistrations = 10
	maxConcurrentOperations    = 20
	successRateThreshold       = 95.0
	expectedArgsCount          = 2
	percentageMultiplier       = 100
	historyTimeout             = 5 * time.Minute
)

// setupUsers registers and authenticates the specified number of users concurrently.
func setupUsers(
	ctx context.Context,
	url, hostname string,
	numUsers int,
	logger *slog.Logger,
) ([]*loadtest.AuthenticatedUser, error) {
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting user registration", slog.Int("num_users", numUsers))

	var (
		users   = make([]*loadtest.AuthenticatedUser, 0, numUsers)
		usersMu sync.Mutex
		wg      sync.WaitGroup
		errCh   = make(chan error, numUsers)
	)
	semaphore := make(chan struct{}, maxConcurrentRegistrations)

	for i := range numUsers {
		wg.Add(1)
		go func(userIndex int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			userCtx, cancel := context.WithTimeout(ctx, userRegistrationTimeout)
			defer cancel()

			user, err := loadtest.RegisterAndAuthenticateUser(userCtx, url, hostname, userIndex, logger)
			if err != nil {
				errCh <- fmt.Errorf("user %d: %w", userIndex, err)
				return
			}
			usersMu.Lock()
			users = append(users, user)
			usersMu.Unlock()
		}(i)
	}
	wg.Wait()
	close(errCh)

	var firstErr error
	failures := 0
	for err := range errCh {
		failures++
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Some user registrations failed",
			slog.Int("failed_count", failures),
			slog.Int("successful_count", len(users)))
		return users, fmt.Errorf("registration failures: %w", firstErr)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "All users registered successfully",
		slog.Int("total_users", len(users)))
	return users, nil
}

// generateHistoryForUsers backfills synthetic workout history for every user in parallel.
func generateHistoryForUsers(
	ctx context.Context, users []*loadtest.AuthenticatedUser, logger *slog.Logger,
) error {
	var (
		wg    sync.WaitGroup
		errCh = make(chan error, len(users))
	)
	semaphore := make(chan struct{}, maxConcurrentRegistrations)

	for _, user := range users {
		wg.Add(1)
		go func(u *loadtest.AuthenticatedUser) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			historyCtx, cancel := context.WithTimeout(ctx, historyTimeout)
			defer cancel()

			if err := loadtest.GenerateWorkoutHistory(historyCtx, u, logger); err != nil {
				errCh <- fmt.Errorf("user %s: %w", u.UserID, err)
			}
		}(user)
	}
	wg.Wait()
	close(errCh)

	var firstErr error
	failures := 0
	for err := range errCh {
		failures++
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		logger.LogAttrs(ctx, slog.LevelError, "Some workout history generations failed",
			slog.Int("failed_count", failures),
			slog.Int("successful_count", len(users)-failures))
		return fmt.Errorf("workout history generation failures: %w", firstErr)
	}
	return nil
}

// runLoadTest drives each user through one WorkoutScenario concurrently and
// reports a pass/fail based on a 95% success-rate threshold.
func runLoadTest(
	ctx context.Context, users []*loadtest.AuthenticatedUser, logger *slog.Logger,
) error {
	userCount := len(users)
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting load test", slog.Int("num_users", userCount))

	var successCount, failureCount int64
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentOperations)

	for _, user := range users {
		g.Go(func() error {
			u := user
			scenarioCtx, cancel := context.WithTimeout(ctx, scenarioTimeout)
			defer cancel()

			if err := loadtest.WorkoutScenario(scenarioCtx, u, logger); err != nil {
				atomic.AddInt64(&failureCount, 1)
				logger.LogAttrs(scenarioCtx, slog.LevelWarn, "Scenario failed",
					slog.String("user_id", u.UserID),
					slog.Any("error", err))
				return nil
			}
			atomic.AddInt64(&successCount, 1)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("load test failed: %w", err)
	}

	successRate := float64(successCount) / float64(userCount) * percentageMultiplier
	logger.LogAttrs(ctx, slog.LevelInfo, "Load test completed",
		slog.Int64("successful", successCount),
		slog.Int64("failed", failureCount),
		slog.Float64("success_rate", successRate))

	if successRate < successRateThreshold {
		return fmt.Errorf("load test failed: success rate %.1f%% below threshold", successRate)
	}
	return nil
}

func main() {
	logger := testhelpers.NewLogger(os.Stdout)
	ctx := context.Background()

	if len(os.Args) != expectedArgsCount {
		logger.LogAttrs(ctx, slog.LevelError, "usage: stresstest <hostname>")
		os.Exit(1)
	}

	var (
		hostname = os.Args[1]
		numUsers = 10
		start    = time.Now()
	)
	ctx = logging.WithAttrs(ctx, slog.String("hostname", hostname))

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
	if err = loadtest.RunAuthFlow(ctx, client); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "smoke test failed", slog.Any("error", err))
		os.Exit(1)
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "Smoke test passed")

	setupStart := time.Now()
	users, err := setupUsers(ctx, url, hostname, numUsers, logger)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "failed to setup users", slog.Any("error", err))
		os.Exit(1)
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "User setup completed",
		slog.Duration("setup_duration", time.Since(setupStart)),
		slog.Int("authenticated_users", len(users)))

	historyStart := time.Now()
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting workout history generation",
		slog.Int("num_users", len(users)),
		slog.Int("weeks_per_user", loadtest.HistoryWeeks))
	if err = generateHistoryForUsers(ctx, users, logger); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "some workout history generation failed, continuing",
			slog.Any("error", err))
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "Workout history generation completed",
		slog.Duration("history_duration", time.Since(historyStart)),
		slog.Int("users_with_history", len(users)))

	loadTestStart := time.Now()
	if err = runLoadTest(ctx, users, logger); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "load test failed", slog.Any("error", err))
		os.Exit(1)
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "Load test completed successfully",
		slog.Duration("total_duration", time.Since(start)),
		slog.Duration("load_test_duration", time.Since(loadTestStart)),
		slog.Int("users_tested", len(users)))
}
