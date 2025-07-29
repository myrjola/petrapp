package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
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

const (
	testTimeout                = 10 * time.Second
	userRegistrationTimeout    = 30 * time.Second
	scenarioTimeout            = 30 * time.Second
	maxConcurrentRegistrations = 10
	maxConcurrentOperations    = 20
	baseWeight                 = 15.0
	weightRange                = 20
	baseReps                   = 8
	repsRange                  = 8
	successRateThreshold       = 95.0
	expectedArgsCount          = 2
	percentageMultiplier       = 100
	workoutHistoryWeeks        = 26 // 6 months of weekly workouts
	daysPerWeek                = 7
	historyTimeout             = 5 * time.Minute
	maxWeightVariation         = 5
	maxRepsVariation           = 3
)

// AuthenticatedUser holds a client with valid session.
type AuthenticatedUser struct {
	Client *e2etest.Client
	UserID string // or whatever identifier you need
}

func TestAuth(client *e2etest.Client) error {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, testTimeout)
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

// RegisterAndAuthenticateUser creates a new user and logs them in.
func RegisterAndAuthenticateUser(
	ctx context.Context,
	url, hostname string,
	userIndex int,
	logger *slog.Logger,
) (*AuthenticatedUser, error) {
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

// SetupUsers registers and authenticates the specified number of users.
func SetupUsers(
	ctx context.Context,
	url, hostname string,
	numUsers int,
	logger *slog.Logger,
) ([]*AuthenticatedUser, error) {
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting user registration", slog.Int("num_users", numUsers))

	var (
		users   = make([]*AuthenticatedUser, 0, numUsers)
		usersMu sync.Mutex
		wg      sync.WaitGroup
		errCh   = make(chan error, numUsers)
		errors  = make([]error, 0, numUsers) // Pre-allocate with capacity
	)

	// Limit concurrency to avoid overwhelming the server
	semaphore := make(chan struct{}, maxConcurrentRegistrations) // Max concurrent registrations

	for i := range numUsers {
		wg.Add(1)
		go func(userIndex int) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Create context with timeout for this user
			userCtx, cancel := context.WithTimeout(ctx, userRegistrationTimeout)
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
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		logger.LogAttrs(ctx, slog.LevelError, "Some user registrations failed",
			slog.Int("failed_count", len(errors)),
			slog.Int("successful_count", len(users)))

		// Return first error for now, but you might want to handle this differently
		return users, fmt.Errorf("registration failures: %w", errors[0])
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "All users registered successfully",
		slog.Int("total_users", len(users)))

	return users, nil
}

// GenerateWorkoutHistory creates 6 months of weekly workout history for a user.
func GenerateWorkoutHistory(ctx context.Context, user *AuthenticatedUser, logger *slog.Logger) error {
	client := user.Client

	// Get current date and calculate start date (6 months ago)
	now := time.Now()
	startDate := now.AddDate(0, -6, 0)

	// Set workout preferences first
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		return fmt.Errorf("failed to get preferences: %w", err)
	}

	formData := map[string]string{
		"monday": "true",
	}
	if _, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		return fmt.Errorf("failed to submit preferences: %w", err)
	}

	// Generate weekly workouts
	for week := range workoutHistoryWeeks {
		workoutDate := startDate.AddDate(0, 0, week*daysPerWeek) // Every Monday
		dateStr := workoutDate.Format("2006-01-02")

		// Skip future dates
		if workoutDate.After(now) {
			continue
		}

		// Start workout for this date
		if genErr := generateSingleWorkout(ctx, client, dateStr, logger); genErr != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to generate workout",
				slog.String("user_id", user.UserID),
				slog.String("date", dateStr),
				slog.Any("error", genErr))
			continue // Continue with next workout instead of failing completely
		}

		logger.LogAttrs(ctx, slog.LevelDebug, "Generated workout",
			slog.String("user_id", user.UserID),
			slog.String("date", dateStr))
	}

	return nil
}

// getOrCreateWorkout attempts to get a workout, and if it doesn't exist (404), creates it first.
func getOrCreateWorkout(ctx context.Context, client *e2etest.Client, dateStr string) (*goquery.Document, error) {
	// Try to get the workout page first
	resp, err := client.Get(ctx, "/workouts/"+dateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get workout page for %s: %w", dateStr, err)
	}
	defer resp.Body.Close()

	// If workout exists, parse and return the document
	if resp.StatusCode == http.StatusOK {
		workoutDoc, parseErr := goquery.NewDocumentFromReader(resp.Body)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse workout page for %s: %w", dateStr, parseErr)
		}
		return workoutDoc, nil
	}

	// If it's a 404, we need to create the workout first
	if resp.StatusCode == http.StatusNotFound {
		// Parse the 404 page to get the create workout form
		notFoundDoc, parseErr := goquery.NewDocumentFromReader(resp.Body)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse workout not found page for %s: %w", dateStr, parseErr)
		}

		// Find and submit the create workout form
		form := notFoundDoc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
			return s.Find("button:contains('Create Workout')").Length() > 0
		}).First()

		if form.Length() == 0 {
			return nil, fmt.Errorf("could not find create workout form on 404 page for %s", dateStr)
		}

		action, exists := form.Attr("action")
		if !exists {
			return nil, fmt.Errorf("create workout form has no action attribute for %s", dateStr)
		}

		// Submit the form to create the workout
		createdDoc, submitErr := client.SubmitForm(ctx, notFoundDoc, action, nil)
		if submitErr != nil {
			return nil, fmt.Errorf("failed to create workout for %s: %w", dateStr, submitErr)
		}

		return createdDoc, nil
	}

	// Any other status code is an error
	return nil, fmt.Errorf("unexpected status code %d when accessing workout page for %s", resp.StatusCode, dateStr)
}

// generateSingleWorkout creates and completes a single workout for the given date.
func generateSingleWorkout(ctx context.Context, client *e2etest.Client, dateStr string, logger *slog.Logger) error {
	// Get or create the workout
	doc, err := getOrCreateWorkout(ctx, client, dateStr)
	if err != nil {
		return err
	}

	// Find all exercises on this workout
	var exerciseIDs []string
	doc.Find("a.exercise").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			// Extract exercise ID from URL like /workouts/2024-01-01/exercises/123
			exerciseID := href[len("/workouts/"+dateStr+"/exercises/"):]
			exerciseIDs = append(exerciseIDs, exerciseID)
		}
	})

	if len(exerciseIDs) == 0 {
		return errors.New("no exercises found on workout page")
	}

	// Complete sets for each exercise
	for _, exerciseID := range exerciseIDs {
		if completeErr := completeExerciseSets(ctx, client, dateStr, exerciseID); completeErr != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to complete exercise sets",
				slog.String("date", dateStr),
				slog.String("exercise_id", exerciseID),
				slog.Any("error", completeErr))
			continue // Continue with next exercise
		}
	}

	return nil
}

// completeExerciseSets completes all sets for a given exercise.
func completeExerciseSets(ctx context.Context, client *e2etest.Client, dateStr, exerciseID string) error {
	// Get the exercise page
	doc, err := client.GetDoc(ctx, "/workouts/"+dateStr+"/exercises/"+exerciseID)
	if err != nil {
		return fmt.Errorf("failed to get exercise page: %w", err)
	}

	// Complete warmup if present
	warmupForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[type=submit]:contains('Warmup Done!')").Length() > 0
	}).First()

	if warmupForm.Length() > 0 {
		warmupAction, exists := warmupForm.Attr("action")
		if exists {
			if _, err = client.SubmitForm(ctx, doc, warmupAction, nil); err != nil {
				return fmt.Errorf("failed to complete warmup: %w", err)
			}
		}
	}

	// Complete all sets
	for {
		// Refresh the page to get current state
		doc, err = client.GetDoc(ctx, "/workouts/"+dateStr+"/exercises/"+exerciseID)
		if err != nil {
			return fmt.Errorf("failed to refresh exercise page: %w", err)
		}

		// Find the next incomplete set
		setForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
			return s.Find("button[type=submit]:contains('Done!')").Length() > 0
		}).First()

		if setForm.Length() == 0 {
			break // No more sets to complete
		}

		action, exists := setForm.Attr("action")
		if !exists {
			return errors.New("set form has no action attribute")
		}

		// Generate realistic workout data with some progression over time
		baseWeightForHistory := baseWeight + float64(time.Now().UnixNano()%maxWeightVariation) // Small variation
		baseRepsForHistory := baseReps + int(time.Now().UnixNano()%maxRepsVariation)           // 8-10 reps typically

		setData := map[string]string{
			"reps": strconv.Itoa(baseRepsForHistory),
		}

		// Only add weight if there's a weight input field (not a bodyweight exercise)
		if setForm.Find("input[name='weight']").Length() > 0 {
			setData["weight"] = fmt.Sprintf("%.1f", baseWeightForHistory)
		}

		if _, err = client.SubmitForm(ctx, doc, action, setData); err != nil {
			return fmt.Errorf("failed to complete set: %w", err)
		}
	}

	return nil
}

// GenerateWorkoutHistoryForUsers generates workout history for all users concurrently.
func GenerateWorkoutHistoryForUsers(ctx context.Context, users []*AuthenticatedUser, logger *slog.Logger) error {
	var (
		wg     sync.WaitGroup
		errCh  = make(chan error, len(users))
		errors = make([]error, 0, len(users))
	)

	// Limit concurrency to avoid overwhelming the server
	semaphore := make(chan struct{}, maxConcurrentRegistrations)

	for _, user := range users {
		wg.Add(1)
		go func(u *AuthenticatedUser) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Create context with timeout for workout history generation
			historyCtx, cancel := context.WithTimeout(ctx, historyTimeout) // Generous timeout for history
			defer cancel()

			if err := GenerateWorkoutHistory(historyCtx, u, logger); err != nil {
				errCh <- fmt.Errorf("user %s: %w", u.UserID, err)
				return
			}

			logger.LogAttrs(historyCtx, slog.LevelDebug, "Generated workout history",
				slog.String("user_id", u.UserID))
		}(user)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errCh)

	// Check for errors
	for err := range errCh {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		logger.LogAttrs(ctx, slog.LevelError, "Some workout history generations failed",
			slog.Int("failed_count", len(errors)),
			slog.Int("successful_count", len(users)-len(errors)))

		// Return first error, but continue with load test as some users have history
		return fmt.Errorf("workout history generation failures: %w", errors[0])
	}

	return nil
}

// WorkoutScenario represents a complete workout flow for stress testing.
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
		return errors.New("no exercise found on workout page")
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
		return errors.New("set completion form not found")
	}

	action, exists := form.Attr("action")
	if !exists {
		return errors.New("form has no action attribute")
	}

	// Submit set with random-ish data to simulate real usage
	setData := map[string]string{
		"reps": strconv.FormatInt(baseReps+time.Now().UnixNano()%repsRange, 10), // 8-15 reps range
	}

	// Only add weight if there's a weight input field (not a bodyweight exercise)
	if form.Find("input[name='weight']").Length() > 0 {
		setData["weight"] = fmt.Sprintf("%.1f", baseWeight+float64(time.Now().UnixNano()%weightRange)) // 15-35kg range
	}

	if _, err = client.SubmitForm(ctx, doc, action, setData); err != nil {
		return fmt.Errorf("failed to complete set: %w", err)
	}

	// Fetch progress chart which is a common operation after completing a set.
	chartResp, err := client.Get(ctx, "/workouts/"+today+"/exercises/"+exerciseID+"/progress-chart")
	if err != nil {
		return fmt.Errorf("failed to get progress chart: %w", err)
	}
	chartResp.Body.Close()

	logger.LogAttrs(ctx, slog.LevelDebug, "Workout scenario completed",
		slog.String("user_id", user.UserID),
		slog.String("exercise_id", exerciseID))

	return nil
}

// RunLoadTest performs the actual load testing with authenticated users.
func RunLoadTest(ctx context.Context, users []*AuthenticatedUser, logger *slog.Logger) error {
	userCount := len(users)
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting load test", slog.Int("num_users", userCount))

	// Counters for success/failure tracking
	var successCount, failureCount int64

	// Create errgroup with context and limit concurrency
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentOperations) // Max concurrent operations

	// Launch all scenarios
	for _, user := range users {
		g.Go(func() error {
			// Capture loop variables
			u := user
			// Create context with timeout for this scenario
			scenarioCtx, cancel := context.WithTimeout(ctx, scenarioTimeout)
			defer cancel()

			if err := WorkoutScenario(scenarioCtx, u, logger); err != nil {
				atomic.AddInt64(&failureCount, 1)
				// Log individual failures but don't stop the entire test
				logger.LogAttrs(scenarioCtx, slog.LevelWarn, "Scenario failed",
					slog.String("user_id", u.UserID),
					slog.Any("error", err))
				return nil // Don't propagate error to avoid stopping other scenarios
			}

			atomic.AddInt64(&successCount, 1)
			return nil
		})
	}

	// Wait for all scenarios to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("load test failed: %w", err)
	}

	// Report results
	successRate := float64(successCount) / float64(userCount) * percentageMultiplier

	logger.LogAttrs(ctx, slog.LevelInfo, "Load test completed",
		slog.Int64("successful", successCount),
		slog.Int64("failed", failureCount),
		slog.Float64("success_rate", successRate))

	// Consider test failed if success rate is too low
	if successRate < successRateThreshold {
		return fmt.Errorf("load test failed: success rate %.1f%% below threshold", successRate)
	}

	return nil
}

func main() {
	logger := testhelpers.NewLogger(os.Stdout)
	ctx := context.Background()

	if len(os.Args) != expectedArgsCount {
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

	// Generate workout history for all users
	historyStart := time.Now()
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting workout history generation",
		slog.Int("num_users", len(users)),
		slog.Int("weeks_per_user", workoutHistoryWeeks))

	if err = GenerateWorkoutHistoryForUsers(ctx, users, logger); err != nil {
		logger.LogAttrs(ctx, slog.LevelWarn, "some workout history generation failed, continuing with load test",
			slog.Any("error", err))
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Workout history generation completed",
		slog.Duration("history_duration", time.Since(historyStart)),
		slog.Int("users_with_history", len(users)))

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
