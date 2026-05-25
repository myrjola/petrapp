// Package loadtest provides reusable HTTP scenarios that exercise the petrapp
// app the way a real user would: register, set preferences, start a workout,
// complete sets. The scenarios are imported by cmd/stresstest (which runs them
// against a deployed app) and by an in-process smoke test in cmd/web (which
// runs them against a freshly-started test server every CI run so selector or
// form-field drift fails the test instead of an ops-time perf run).
package loadtest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/myrjola/petrapp/internal/e2etest"
)

const (
	authTimeout        = 10 * time.Second
	baseWeight         = 15.0
	weightRange        = 20
	baseReps           = 8
	repsRange          = 8
	workoutHistoryDays = 7
	// HistoryWeeks is how many weeks of synthetic workout history GenerateWorkoutHistory
	// attempts to backfill. Lazy session creation in StartSession only honors the current
	// week, so older weeks 404 — that is fine for load generation but it does mean the
	// upstream DB stays mostly empty. Address with a fixture DB if richer history matters.
	HistoryWeeks       = 26
	maxWeightVariation = 5
	maxRepsVariation   = 3
)

// AuthenticatedUser holds a client with a valid session.
type AuthenticatedUser struct {
	Client *e2etest.Client
	UserID string
}

// RunAuthFlow exercises the full register → logout → login round-trip against
// a fresh client. Used as a one-shot smoke check before fanning out load.
func RunAuthFlow(ctx context.Context, client *e2etest.Client) error {
	ctx, cancel := context.WithTimeout(ctx, authTimeout)
	defer cancel()

	if _, err := client.Register(ctx); err != nil {
		return fmt.Errorf("register user: %w", err)
	}
	if _, err := client.Logout(ctx); err != nil {
		return fmt.Errorf("logout user: %w", err)
	}
	if _, err := client.Login(ctx); err != nil {
		return fmt.Errorf("login user: %w", err)
	}
	return nil
}

// RegisterAndAuthenticateUser creates a new user with its own cookie jar and
// returns it ready for further scenario calls.
func RegisterAndAuthenticateUser(
	ctx context.Context,
	url, hostname string,
	userIndex int,
	logger *slog.Logger,
) (*AuthenticatedUser, error) {
	client, err := e2etest.NewClient(url, hostname, url)
	if err != nil {
		return nil, fmt.Errorf("creating client for user %d: %w", userIndex, err)
	}

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

// WorkoutScenario runs one full read+write workout flow: set preferences,
// start today's workout, complete warmup, complete one set, fetch progress
// chart. This is the canonical "single user, single iteration" unit of load.
func WorkoutScenario(ctx context.Context, user *AuthenticatedUser, logger *slog.Logger) error {
	client := user.Client
	today := time.Now().Format("2006-01-02")

	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}

	// Label-based match resolves to the underlying name="{weekday}_minutes" select.
	if doc, err = client.SubmitForm(ctx, doc, "/preferences/schedule", map[string]string{
		time.Now().Weekday().String(): "60",
	}); err != nil {
		return fmt.Errorf("submit preferences: %w", err)
	}

	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		return fmt.Errorf("start workout: %w", err)
	}

	exerciseID := firstExerciseID(doc, today)
	if exerciseID == "" {
		return errors.New("no exercise found on workout page")
	}

	if doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID); err != nil {
		return fmt.Errorf("get exercise page: %w", err)
	}

	if doc, err = completeWarmup(ctx, client, doc, today, exerciseID); err != nil {
		return err
	}

	if err = completeOneSet(ctx, client, doc); err != nil {
		return err
	}

	// Progress chart is a common follow-up; fetch it so we exercise that path too.
	chartResp, err := client.Get(ctx, "/workouts/"+today+"/exercises/"+exerciseID+"/progress-chart")
	if err != nil {
		return fmt.Errorf("get progress chart: %w", err)
	}
	if closeErr := chartResp.Body.Close(); closeErr != nil {
		return fmt.Errorf("close progress chart body: %w", closeErr)
	}

	logger.LogAttrs(ctx, slog.LevelDebug, "Workout scenario completed",
		slog.String("user_id", user.UserID),
		slog.String("exercise_id", exerciseID))
	return nil
}

// SustainedSetStep performs the per-iteration work of a realistic sustained
// load: open today's workout, complete one set (after warmup if shown), and
// return. It does NOT re-submit /preferences/schedule, does NOT re-POST
// /workouts/{date}/start once the workout exists, and does NOT fetch the
// progress chart on every iteration. Together those keep the request mix
// closer to a real user mid-workout: one write (set completion) plus 1–2
// reads per iteration. Caller is responsible for inter-iteration think time.
//
// On a workout that has no remaining active sets the function returns nil
// without erroring: the user "finished" their workout for the day. Callers
// running in a tight loop should treat that as a signal to either rotate
// users, advance the date, or back off.
func SustainedSetStep(ctx context.Context, user *AuthenticatedUser, logger *slog.Logger) error {
	client := user.Client
	today := time.Now().Format("2006-01-02")

	doc, err := getOrCreateWorkout(ctx, client, today)
	if err != nil {
		return fmt.Errorf("get workout: %w", err)
	}

	exerciseID := firstExerciseID(doc, today)
	if exerciseID == "" {
		return nil
	}

	doc, err = client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID)
	if err != nil {
		return fmt.Errorf("get exercise page: %w", err)
	}

	if doc, err = completeWarmup(ctx, client, doc, today, exerciseID); err != nil {
		return err
	}

	form := activeSetForm(doc)
	if form.Length() == 0 {
		return nil
	}
	if err = completeOneSet(ctx, client, doc); err != nil {
		return err
	}

	logger.LogAttrs(ctx, slog.LevelDebug, "Sustained set completed",
		slog.String("user_id", user.UserID),
		slog.String("exercise_id", exerciseID))
	return nil
}

// GenerateWorkoutHistory backfills weekly workouts for the user as far as
// lazy-create allows. Only the current week succeeds; older weeks 404 (logged
// at DEBUG). Use it to add some load weight, not to seed a realistic DB.
func GenerateWorkoutHistory(ctx context.Context, user *AuthenticatedUser, logger *slog.Logger) error {
	client := user.Client
	now := time.Now()
	startDate := now.AddDate(0, -6, 0)

	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	if _, err = client.SubmitForm(ctx, doc, "/preferences/schedule", map[string]string{
		time.Now().Weekday().String(): "60",
	}); err != nil {
		return fmt.Errorf("submit preferences: %w", err)
	}

	for week := range HistoryWeeks {
		workoutDate := startDate.AddDate(0, 0, week*workoutHistoryDays)
		if workoutDate.After(now) {
			continue
		}
		dateStr := workoutDate.Format("2006-01-02")
		if genErr := generateSingleWorkout(ctx, client, dateStr, logger); genErr != nil {
			logger.LogAttrs(ctx, slog.LevelDebug, "Failed to generate workout (expected for past weeks)",
				slog.String("user_id", user.UserID),
				slog.String("date", dateStr),
				slog.Any("error", genErr))
			continue
		}
	}

	return nil
}

// firstExerciseID returns the first /workouts/{date}/exercises/{id} suffix on the page.
func firstExerciseID(doc *goquery.Document, dateStr string) string {
	var id string
	prefix := "/workouts/" + dateStr + "/exercises/"
	doc.Find("a.exercise").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if href, exists := s.Attr("href"); exists {
			id = href[len(prefix):]
			return false
		}
		return true
	})
	return id
}

// completeWarmup submits the "Mark done" warmup form if present and re-fetches the
// exercise page (warmup POST redirects to the overview, not the set form).
func completeWarmup(
	ctx context.Context, client *e2etest.Client, doc *goquery.Document, today, exerciseID string,
) (*goquery.Document, error) {
	warmupForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[type=submit]:contains('Mark done')").Length() > 0
	}).First()
	if warmupForm.Length() == 0 {
		return doc, nil
	}
	action, exists := warmupForm.Attr("action")
	if !exists {
		return nil, errors.New("warmup form has no action attribute")
	}
	if _, err := client.SubmitForm(ctx, doc, action, nil); err != nil {
		return nil, fmt.Errorf("complete warmup: %w", err)
	}
	refreshed, err := client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+exerciseID)
	if err != nil {
		return nil, fmt.Errorf("re-fetch exercise page after warmup: %w", err)
	}
	return refreshed, nil
}

// completeOneSet submits the active set with random-ish realistic data.
// Normal weeks render signal buttons; deload weeks render a "Done!" button.
func completeOneSet(ctx context.Context, client *e2etest.Client, doc *goquery.Document) error {
	form := activeSetForm(doc)
	if form.Length() == 0 {
		return errors.New("set completion form not found")
	}
	action, exists := form.Attr("action")
	if !exists {
		return errors.New("set form has no action attribute")
	}
	setData := map[string]string{
		"reps":   strconv.FormatInt(baseReps+time.Now().UnixNano()%repsRange, 10),
		"signal": "on_target",
	}
	if form.Find("input[name='weight']").Length() > 0 {
		setData["weight"] = fmt.Sprintf("%.1f", baseWeight+float64(time.Now().UnixNano()%weightRange))
	}
	if _, err := client.SubmitForm(ctx, doc, action, setData); err != nil {
		return fmt.Errorf("complete set: %w", err)
	}
	return nil
}

// activeSetForm finds the active set form. Prefers .set-form (the canonical
// class), falls back to the deload-only "Done!" button match.
func activeSetForm(doc *goquery.Document) *goquery.Selection {
	form := doc.Find("form.set-form").First()
	if form.Length() > 0 {
		return form
	}
	return doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[type=submit]:contains('Done!')").Length() > 0
	}).First()
}

// generateSingleWorkout creates (or fetches) the workout for dateStr and
// completes every exercise on it. Used by GenerateWorkoutHistory.
func generateSingleWorkout(
	ctx context.Context, client *e2etest.Client, dateStr string, logger *slog.Logger,
) error {
	doc, err := getOrCreateWorkout(ctx, client, dateStr)
	if err != nil {
		return err
	}

	prefix := "/workouts/" + dateStr + "/exercises/"
	var exerciseIDs []string
	doc.Find("a.exercise").Each(func(_ int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			exerciseIDs = append(exerciseIDs, href[len(prefix):])
		}
	})
	if len(exerciseIDs) == 0 {
		return errors.New("no exercises found on workout page")
	}

	var failures int
	for _, exerciseID := range exerciseIDs {
		if err = completeExerciseSets(ctx, client, dateStr, exerciseID); err != nil {
			failures++
			logger.LogAttrs(ctx, slog.LevelWarn, "Failed to complete exercise sets",
				slog.String("date", dateStr),
				slog.String("exercise_id", exerciseID),
				slog.Any("error", err))
		}
	}
	if failures == len(exerciseIDs) {
		return fmt.Errorf("all %d exercises failed to complete on %s", failures, dateStr)
	}
	return nil
}

// getOrCreateWorkout returns the workout page for dateStr, falling back to lazy-create
// via POST /workouts/{date}/start on 404. Older-than-current-week dates fail here.
func getOrCreateWorkout(
	ctx context.Context, client *e2etest.Client, dateStr string,
) (*goquery.Document, error) {
	resp, err := client.Get(ctx, "/workouts/"+dateStr)
	if err != nil {
		return nil, fmt.Errorf("get workout page for %s: %w", dateStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		workoutDoc, parseErr := goquery.NewDocumentFromReader(resp.Body)
		if parseErr != nil {
			return nil, fmt.Errorf("parse workout page for %s: %w", dateStr, parseErr)
		}
		return workoutDoc, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		homeDoc, getErr := client.GetDoc(ctx, "/")
		if getErr != nil {
			return nil, fmt.Errorf("get home page for CSRF token: %w", getErr)
		}
		startURL := "/workouts/" + dateStr + "/start"
		createdDoc, submitErr := client.SubmitForm(ctx, homeDoc, startURL, nil)
		if submitErr != nil {
			return nil, fmt.Errorf("lazy-create workout for %s: %w", dateStr, submitErr)
		}
		return createdDoc, nil
	}

	return nil, fmt.Errorf("unexpected status code %d for workout page %s", resp.StatusCode, dateStr)
}

// completeExerciseSets loops over every remaining active set on the exercise
// page and submits it. Stops when no active set form is present.
func completeExerciseSets(
	ctx context.Context, client *e2etest.Client, dateStr, exerciseID string,
) error {
	url := "/workouts/" + dateStr + "/exercises/" + exerciseID
	doc, err := client.GetDoc(ctx, url)
	if err != nil {
		return fmt.Errorf("get exercise page: %w", err)
	}

	warmupForm := doc.Find("form").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("button[type=submit]:contains('Mark done')").Length() > 0
	}).First()
	if warmupForm.Length() > 0 {
		if action, exists := warmupForm.Attr("action"); exists {
			if _, err = client.SubmitForm(ctx, doc, action, nil); err != nil {
				return fmt.Errorf("complete warmup: %w", err)
			}
		}
	}

	for {
		doc, err = client.GetDoc(ctx, url)
		if err != nil {
			return fmt.Errorf("refresh exercise page: %w", err)
		}
		form := activeSetForm(doc)
		if form.Length() == 0 {
			return nil // no more active sets
		}
		action, exists := form.Attr("action")
		if !exists {
			return errors.New("set form has no action attribute")
		}

		baseWeightForHistory := baseWeight + float64(time.Now().UnixNano()%maxWeightVariation)
		baseRepsForHistory := baseReps + int(time.Now().UnixNano()%maxRepsVariation)
		setData := map[string]string{
			"reps":   strconv.Itoa(baseRepsForHistory),
			"signal": "on_target",
		}
		if form.Find("input[name='weight']").Length() > 0 {
			setData["weight"] = fmt.Sprintf("%.1f", baseWeightForHistory)
		}
		if _, err = client.SubmitForm(ctx, doc, action, setData); err != nil {
			return fmt.Errorf("complete set: %w", err)
		}
	}
}
