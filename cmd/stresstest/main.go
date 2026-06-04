// Command stresstest drives concurrent workout scenarios against a deployed
// petrapp instance to produce load for performance profiling. Scenario logic
// lives in internal/loadtest so it is also exercised by the in-process smoke
// test in cmd/petra.
//
// Usage:
//
//	stresstest [--users N] [--duration 2m] <hostname>
//
// With no --duration, each user runs one WorkoutScenario and the run is
// reported pass/fail against a 95% success-rate threshold (legacy mode).
// With --duration > 0, every user loops scenarios for that wall-clock window
// and the run reports per-route latency percentiles + 4xx/5xx counts.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"text/tabwriter"
	"time"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/loadtest"
	"github.com/myrjola/petrapp/internal/platform/obs/logging"
	"github.com/myrjola/petrapp/internal/platform/testkit"
	"golang.org/x/sync/errgroup"
)

const (
	userRegistrationTimeout    = 30 * time.Second
	scenarioTimeout            = 30 * time.Second
	maxConcurrentRegistrations = 10
	maxConcurrentOperations    = 20
	successRateThreshold       = 95.0
	percentageMultiplier       = 100
	historyTimeout             = 5 * time.Minute
	defaultUsers               = 10
	defaultThinkTime           = 2 * time.Second
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

// runLoadTestSingleShot drives each user through one WorkoutScenario concurrently and
// reports a pass/fail based on a 95% success-rate threshold. Legacy mode used when
// --duration is not set.
func runLoadTestSingleShot(
	ctx context.Context, users []*loadtest.AuthenticatedUser, logger *slog.Logger,
) error {
	userCount := len(users)
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting load test (single-shot)", slog.Int("num_users", userCount))

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

// loadResult counts scenario outcomes across a sustained-load run.
type loadResult struct {
	Successes int64
	Failures  int64
}

// runLoadTestSustained drives every user through a realistic per-set loop for
// the full wall-clock window. Each iteration completes one set (see
// loadtest.SustainedSetStep) and sleeps a jittered think time, modelling a
// user resting between sets. Scenario successes/failures are counted but a
// "low success rate" no longer fails the run on its own — the goal of a
// sustained run is to produce load for the latency report, not gate CI.
//
// thinkTime is the mean inter-set delay; the actual sleep jitters uniformly
// between thinkTime/2 and thinkTime*3/2 so users don't synchronise on the
// same cadence. Pass 0 to disable think time (load-amplified mode).
func runLoadTestSustained(
	ctx context.Context,
	users []*loadtest.AuthenticatedUser,
	duration, thinkTime time.Duration,
	logger *slog.Logger,
) loadResult {
	logger.LogAttrs(ctx, slog.LevelInfo, "Starting load test (sustained)",
		slog.Int("num_users", len(users)),
		slog.Duration("duration", duration),
		slog.Duration("think_time", thinkTime))

	loadCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	var result loadResult
	var wg sync.WaitGroup
	for _, user := range users {
		wg.Add(1)
		go func(u *loadtest.AuthenticatedUser) {
			defer wg.Done()
			runSustainedUser(loadCtx, u, thinkTime, &result, logger)
		}(user)
	}
	wg.Wait()
	return result
}

// runSustainedUser loops SustainedSetStep + think time for one virtual user
// until loadCtx is cancelled. Counts success/failure into result atomically.
func runSustainedUser(
	loadCtx context.Context,
	u *loadtest.AuthenticatedUser,
	thinkTime time.Duration,
	result *loadResult,
	logger *slog.Logger,
) {
	for {
		if loadCtx.Err() != nil {
			return
		}
		scenarioCtx, scenarioCancel := context.WithTimeout(loadCtx, scenarioTimeout)
		if err := loadtest.SustainedSetStep(scenarioCtx, u, logger); err != nil {
			atomic.AddInt64(&result.Failures, 1)
			logger.LogAttrs(scenarioCtx, slog.LevelDebug, "Scenario failed",
				slog.String("user_id", u.UserID),
				slog.Any("error", err))
		} else {
			atomic.AddInt64(&result.Successes, 1)
		}
		scenarioCancel()
		if thinkTime <= 0 {
			continue
		}
		select {
		case <-loadCtx.Done():
			return
		case <-time.After(jitterThinkTime(thinkTime)):
		}
	}
}

// jitterThinkTime returns a duration uniformly distributed in [mean/2, mean*3/2).
// Used for load-test pacing; math/rand/v2 is intentional — no crypto strength needed.
func jitterThinkTime(mean time.Duration) time.Duration {
	if mean <= 0 {
		return 0
	}
	return mean/2 + time.Duration(rand.Int64N(int64(mean))) //nolint:gosec // load-test jitter, not security-sensitive.
}

const tabwriterPadding = 2

// printReport writes the per-route latency snapshot to w as a fixed-width table.
func printReport(w *os.File, snap *loadtest.Snapshot, elapsed time.Duration, successes, failures int64) {
	totalRequests := 0
	for _, r := range snap.Routes {
		totalRequests += r.Count
	}
	fmt.Fprintf(w, "\n=== Load report (elapsed %s) ===\n", elapsed.Round(time.Millisecond))
	fmt.Fprintf(w, "Scenarios: %d ok / %d failed\n", successes, failures)
	if elapsed > 0 {
		fmt.Fprintf(w, "Total HTTP requests: %d  (%.1f req/s)\n",
			totalRequests, float64(totalRequests)/elapsed.Seconds())
	}
	tw := tabwriter.NewWriter(w, 0, 0, tabwriterPadding, ' ', 0)
	fmt.Fprintln(tw, "METHOD\tPATH\tN\t4XX\t5XX\tP50\tP95\tP99\tMAX")
	for _, r := range snap.Routes {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\t%s\t%s\t%s\n",
			r.Method, r.Path, r.Count, r.Status4xx, r.Status5xx,
			r.P50.Round(time.Millisecond),
			r.P95.Round(time.Millisecond),
			r.P99.Round(time.Millisecond),
			r.Max.Round(time.Millisecond))
	}
	if flushErr := tw.Flush(); flushErr != nil {
		fmt.Fprintf(os.Stderr, "report flush: %v\n", flushErr)
	}
}

// runSetupPhase runs the smoke test, registers users, and triggers history
// generation. Returns the live users and a fresh recorder scoped to the
// load-test phase (setup traffic is not measured).
func runSetupPhase(
	ctx context.Context, url, hostname string, numUsers int, logger *slog.Logger,
) ([]*loadtest.AuthenticatedUser, *loadtest.Recorder, error) {
	logger.LogAttrs(ctx, slog.LevelInfo, "Running smoke test first...")
	client, err := e2etest.NewClient(url, hostname, url)
	if err != nil {
		return nil, nil, fmt.Errorf("create client: %w", err)
	}
	if err = client.WaitForReady(ctx, "/api/healthy"); err != nil {
		return nil, nil, fmt.Errorf("server not ready: %w", err)
	}
	if err = loadtest.RunAuthFlow(ctx, client); err != nil {
		return nil, nil, fmt.Errorf("smoke test: %w", err)
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "Smoke test passed")

	setupStart := time.Now()
	users, err := setupUsers(ctx, url, hostname, numUsers, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("setup users: %w", err)
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

	// Attach a fresh recorder only after setup, so the report covers the
	// load-test phase alone.
	recorder := loadtest.NewRecorder()
	for _, u := range users {
		httpClient := u.Client.HTTPClient()
		httpClient.Transport = loadtest.NewRecordingTransport(httpClient.Transport, recorder)
	}
	return users, recorder, nil
}

func main() {
	users := flag.Int("users", defaultUsers, "number of concurrent virtual users")
	duration := flag.Duration("duration", 0,
		"sustained-load window (e.g. 2m). 0 = single-shot legacy mode.")
	thinkTime := flag.Duration("think", defaultThinkTime,
		"mean inter-set think time during sustained load (jittered ±50%). "+
			"0 disables think time. Default matches an aggressive but realistic user resting between sets.")
	pprofURL := flag.String("pprof-url", "",
		"base URL where the target exposes pprof (e.g. http://localhost:6060). "+
			"If set, CPU + heap profiles + JSON report are saved to --out during the load run.")
	outDir := flag.String("out", "pprof",
		"directory for pprof captures and JSON report bundle")
	flag.Usage = func() { //nolint:reassign // documented stdlib customization point.
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] <hostname>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	logger := testkit.NewLogger(os.Stdout)
	ctx := context.Background()
	hostname := flag.Arg(0)
	start := time.Now()
	ctx = logging.WithAttrs(ctx, slog.String("hostname", hostname))

	url := "https://" + hostname
	if strings.Contains(hostname, "localhost") {
		url = "http://" + hostname
		hostname = "localhost"
	}

	authedUsers, recorder, err := runSetupPhase(ctx, url, hostname, *users, logger)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "setup phase failed", slog.Any("error", err))
		os.Exit(1)
	}

	loadStart := time.Now()
	pprofDone := startPprofCapture(ctx, *pprofURL, *outDir, *duration, logger)

	var result loadResult
	if *duration > 0 {
		result = runLoadTestSustained(ctx, authedUsers, *duration, *thinkTime, logger)
	} else {
		if err = runLoadTestSingleShot(ctx, authedUsers, logger); err != nil {
			logger.LogAttrs(ctx, slog.LevelError, "load test failed", slog.Any("error", err))
			printReport(os.Stdout, recorder.Snapshot(), time.Since(loadStart), 0, int64(len(authedUsers)))
			<-pprofDone
			os.Exit(1)
		}
		// Per-user attempt count equals user count in single-shot mode.
		result.Successes = int64(len(authedUsers))
	}

	elapsed := time.Since(loadStart)
	snap := recorder.Snapshot()
	printReport(os.Stdout, snap, elapsed, result.Successes, result.Failures)
	<-pprofDone

	if *pprofURL != "" {
		if reportPath, reportErr := loadtest.WriteJSONReport(*outDir, snap); reportErr == nil {
			logger.LogAttrs(ctx, slog.LevelInfo, "report bundle written",
				slog.String("path", reportPath))
		} else {
			logger.LogAttrs(ctx, slog.LevelWarn, "failed to write JSON report",
				slog.Any("error", reportErr))
		}
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "Load test completed successfully",
		slog.Duration("total_duration", time.Since(start)),
		slog.Duration("load_test_duration", elapsed),
		slog.Int("users_tested", len(authedUsers)))
}

// startPprofCapture spawns goroutines that fetch CPU + heap profiles from the
// target's pprof endpoint, if pprofURL is non-empty. Returns a channel that
// closes once both captures have finished, so main can wait on completion
// before exiting. The CPU profile sample window matches duration (clamped to
// at least 30s so a single-shot run still gives pprof something to chew on).
func startPprofCapture(
	ctx context.Context, pprofURL, outDir string, duration time.Duration, logger *slog.Logger,
) <-chan struct{} {
	done := make(chan struct{})
	if pprofURL == "" {
		close(done)
		return done
	}
	cpuWindow := max(duration, minPprofWindow)
	go func() {
		defer close(done)
		logger.LogAttrs(ctx, slog.LevelInfo, "capturing CPU profile",
			slog.Duration("seconds", cpuWindow),
			slog.String("pprof_url", pprofURL))
		if path, err := loadtest.CapturePprofProfile(ctx, pprofURL, outDir, cpuWindow); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "cpu profile capture failed",
				slog.Any("error", err))
		} else {
			logger.LogAttrs(ctx, slog.LevelInfo, "cpu profile saved", slog.String("path", path))
		}
		if path, err := loadtest.CapturePprofHeap(ctx, pprofURL, outDir); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "heap profile capture failed",
				slog.Any("error", err))
		} else {
			logger.LogAttrs(ctx, slog.LevelInfo, "heap profile saved", slog.String("path", path))
		}
	}()
	return done
}

const minPprofWindow = 30 * time.Second
