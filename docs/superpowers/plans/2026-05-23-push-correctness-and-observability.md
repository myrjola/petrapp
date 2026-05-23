# Push Correctness & Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three highest-risk gaps in the recently-landed rest-push flow: a non-atomic delete loop, missing `user_id` on rest-push failure log lines in the service, and missing `user_id` on dispatch/repo-delete log lines in the scheduler.

**Architecture:** Pure additive changes. One new repository method (`DeleteAllByUser`) replaces a service-side loop with a single `DELETE`. The two log-observability changes thread the already-available `userID` through `LogAttrs` calls — no new fields, no new types, no behavior changes beyond what's logged.

**Tech Stack:** Go 1.24 (stdlib only), SQLite (no schema changes), existing `slog` logger, existing `internal/repository` + `internal/notification` packages.

**Scope notes (derived from a code-smell audit on 2026-05-23):**
- Finding #1 ("fire-and-forget after commit"): the silent-failure design is *intentional* (see `internal/service/sets.go:93-96` — "failures here just mean the user won't get a notification — never propagate"). The actionable smell is observability of those silent failures, not the architecture. Scope of this plan: harmonize log attribute keys so failures are filterable.
- Finding #2 ("scheduler context divorced from request"): inheriting the request context into the scheduler would be a bug — fire happens minutes/hours later. The actionable smell is that `fire()` logs only `workout_exercise_id`, not `user_id`. Scope of this plan: add `user_id` to the two log sites.
- Finding #5 ("non-transactional delete loop"): real, fixable by a single `DELETE WHERE user_id = ?`.

---

## Task 1: Atomic `DeleteAllByUser` for push subscriptions

**Files:**
- Modify: `internal/repository/repository.go` (interface)
- Modify: `internal/repository/push_subscription.go` (implementation)
- Modify: `internal/service/push.go` (use the new method)
- Test (create): `internal/repository/push_subscription_test.go` (add a test in the existing file)
- Test (modify): `internal/service/feature_flags_test.go` — no, see Step 6 — we add a fresh test file `internal/service/push_test.go` if absent, otherwise extend it. (Step 6 checks.)

The current `service.DeletePushSubscription` with empty endpoint calls `ListByUser` then loops `DeleteByID(ctx, sub.ID)`. A mid-loop error leaves some rows deleted, some not. Replace with one `DELETE FROM push_subscriptions WHERE user_id = ?`.

- [ ] **Step 1: Write the failing repository test**

Append to `internal/repository/push_subscription_test.go`:

```go
func TestPushSubscriptions_DeleteAllByUser(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	endpoints := []string{
		"https://web.push.apple.com/a",
		"https://web.push.apple.com/b",
		"https://fcm.googleapis.com/wp/c",
	}
	for _, ep := range endpoints {
		sub := domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt populated by Insert.
			Endpoint: ep,
			P256dh:   "p",
			Auth:     "a",
		}
		if _, err := repos.PushSubscriptions.Insert(ctx, sub); err != nil {
			t.Fatalf("Insert %s: %v", ep, err)
		}
	}
	count, err := repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		t.Fatalf("CountByUser before: %v", err)
	}
	if count != len(endpoints) {
		t.Fatalf("CountByUser before = %d, want %d", count, len(endpoints))
	}

	if err = repos.PushSubscriptions.DeleteAllByUser(ctx); err != nil {
		t.Fatalf("DeleteAllByUser: %v", err)
	}

	count, err = repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		t.Fatalf("CountByUser after: %v", err)
	}
	if count != 0 {
		t.Errorf("after DeleteAllByUser: CountByUser = %d, want 0", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/repository -run TestPushSubscriptions_DeleteAllByUser`

Expected: build fails with `repos.PushSubscriptions.DeleteAllByUser undefined`.

- [ ] **Step 3: Extend the repository interface**

In `internal/repository/repository.go`, modify the `PushSubscriptionRepository` interface (currently lines 118–124) to add the new method:

```go
// PushSubscriptionRepository persists per-device Web Push subscriptions.
type PushSubscriptionRepository interface {
	Insert(ctx context.Context, sub domain.PushSubscription) (domain.PushSubscription, error)
	DeleteByEndpoint(ctx context.Context, endpoint string) error
	DeleteByID(ctx context.Context, id int) error
	// DeleteAllByUser removes every subscription for the authenticated user in
	// a single statement. Used by the service layer when the caller asks to
	// delete all of their own devices (e.g. logout).
	DeleteAllByUser(ctx context.Context) error
	ListByUser(ctx context.Context) ([]domain.PushSubscription, error)
	CountByUser(ctx context.Context) (int, error)
}
```

- [ ] **Step 4: Implement `DeleteAllByUser` on the SQLite repo**

In `internal/repository/push_subscription.go`, add this method below `DeleteByID` (after the existing `func (r *sqlitePushSubscriptionRepository) DeleteByID(...)` block):

```go
// DeleteAllByUser removes every subscription belonging to the authenticated
// user. One statement so callers don't have to wrap the loop in a tx.
func (r *sqlitePushSubscriptionRepository) DeleteAllByUser(ctx context.Context) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE user_id = ?`,
		userID,
	); err != nil {
		return fmt.Errorf("delete all push subscriptions: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run repo test to verify it passes**

Run: `go test -v ./internal/repository -run TestPushSubscriptions_DeleteAllByUser`

Expected: PASS.

- [ ] **Step 6: Write a failing service-level test for the empty-endpoint branch**

Check whether `internal/service/push_test.go` exists:

```bash
ls internal/service/push_test.go
```

If it does NOT exist, create it with this content. If it does, append the new test function only.

```go
package service_test

import (
	"context"
	"testing"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/service"
	"github.com/myrjola/petrapp/internal/sqlite"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_DeletePushSubscription_EmptyEndpoint_RemovesAll(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("del-all-user"), "Del All User",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	svc := service.NewService(db, logger, "")

	for _, ep := range []string{"https://x/a", "https://x/b", "https://x/c"} {
		if _, err = svc.UpsertPushSubscription(ctx, domain.PushSubscription{ //nolint:exhaustruct // ID/UserID/CreatedAt assigned by repo.
			Endpoint: ep, P256dh: "p", Auth: "a",
		}); err != nil {
			t.Fatalf("UpsertPushSubscription %s: %v", ep, err)
		}
	}

	if err = svc.DeletePushSubscription(ctx, ""); err != nil {
		t.Fatalf("DeletePushSubscription(empty): %v", err)
	}

	count, err := svc.CountPushSubscriptions(ctx)
	if err != nil {
		t.Fatalf("CountPushSubscriptions: %v", err)
	}
	if count != 0 {
		t.Errorf("after delete-all: count = %d, want 0", count)
	}
}
```

- [ ] **Step 7: Run the new service test — it should pass already**

Run: `go test -v ./internal/service -run Test_DeletePushSubscription_EmptyEndpoint_RemovesAll`

Expected: PASS (the loop already achieves the same outcome on the happy path). The point of this test is to lock in the contract before we change the implementation in Step 8 — refactoring without a test would be unsafe.

- [ ] **Step 8: Replace the loop in `service.DeletePushSubscription`**

In `internal/service/push.go`, replace lines 25–42 (the entire `DeletePushSubscription` function) with:

```go
// DeletePushSubscription removes the authenticated user's subscription
// identified by endpoint. Empty endpoint deletes all subscriptions for the
// user in a single atomic statement.
func (s *Service) DeletePushSubscription(ctx context.Context, endpoint string) error {
	if endpoint == "" {
		if err := s.repos.PushSubscriptions.DeleteAllByUser(ctx); err != nil {
			return fmt.Errorf("delete all push subscriptions: %w", err)
		}
		return nil
	}
	if err := s.repos.PushSubscriptions.DeleteByEndpoint(ctx, endpoint); err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	return nil
}
```

- [ ] **Step 9: Run all push tests to confirm green**

Run: `go test ./internal/repository -run TestPushSubscriptions && go test ./internal/service -run DeletePushSubscription`

Expected: PASS on both packages.

- [ ] **Step 10: Run the full test suite**

Run: `make test`

Expected: PASS (no regressions in handler tests, service tests, etc. that might have depended on the loop's behavior).

- [ ] **Step 11: Run lint-fix**

Run: `make lint-fix`

Expected: no diff, no errors.

- [ ] **Step 12: Commit**

```bash
git add internal/repository/repository.go internal/repository/push_subscription.go \
        internal/repository/push_subscription_test.go \
        internal/service/push.go internal/service/push_test.go
git commit -m "repository: add atomic DeleteAllByUser for push subscriptions

Replace the service-side ListByUser+loop DeleteByID with a single
DELETE WHERE user_id = ?. The loop could leave partial state on error
(rows 1-2 deleted, row 3+ retained) since it ran outside any txn.
"
```

---

## Task 2: Add `user_id` + `workout_exercise_id` to rest-push log lines

**Files:**
- Modify: `internal/service/sets.go` (the `applyRestPushDecision` function, lines 97–173)
- Test (modify): `internal/service/sets_test.go` (add one new test + a tiny extension to the existing `fakeScheduler`)

`applyRestPushDecision` has five `s.logger.LogAttrs(..., slog.LevelWarn, ...)` sites at lines 114, 125, 134, 158, 170. Only the first (cancel-failed) includes `workout_exercise_id`. None include `user_id`. Triaging a "user reports no push" issue requires both keys on every line.

- [ ] **Step 1: Write the failing test (lock the log contract)**

Append to `internal/service/sets_test.go`. First, we need a fake scheduler that errors on demand. Add this struct definition near the existing `fakeScheduler` (immediately below it):

```go
// erroringScheduler returns the configured error from Schedule and behaves
// like a normal fake for Cancel/CancelForWorkout. Used to exercise the
// rest-push failure-logging path.
type erroringScheduler struct {
	scheduleErr error
}

func (e *erroringScheduler) Schedule(_ context.Context, _ domain.ScheduledPush) error {
	return e.scheduleErr
}
func (e *erroringScheduler) Cancel(_ context.Context, _ int) error { return nil }
func (e *erroringScheduler) CancelForWorkout(_ context.Context, _ int, _ time.Time) error {
	return nil
}
```

Next, add the test itself (also in `sets_test.go`). It uses a `bytes.Buffer`-backed slog handler to capture log output — this is the only way to verify attribute keys actually made it onto the log line.

```go
func Test_RecordSet_FailedSchedule_LogsUserAndExerciseID(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	// Seed an incomplete second set so the just-completed one isn't the last,
	// which means PlanRestPush returns Schedule (not Cancel).
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 2, 100.0, 5)`, weID,
	); err != nil {
		t.Fatalf("seed second set: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/fail', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{ //nolint:exhaustruct // AddSource/ReplaceAttr zero.
		Level: slog.LevelDebug,
	}))
	failer := &erroringScheduler{scheduleErr: errors.New("boom: push backend down")}
	svc := service.NewService(db, logger, "").WithScheduler(failer)

	weight := 100.0
	sig := domain.SignalOnTarget
	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.RecordSet(ctx, date, weID, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	out := logBuf.String()
	wantUser := fmt.Sprintf("user_id=%d", userID)
	wantWE := fmt.Sprintf("workout_exercise_id=%d", weID)
	if !strings.Contains(out, wantUser) {
		t.Errorf("log missing %q; got:\n%s", wantUser, out)
	}
	if !strings.Contains(out, wantWE) {
		t.Errorf("log missing %q; got:\n%s", wantWE, out)
	}
	if !strings.Contains(out, "rest push: schedule failed") {
		t.Errorf("log missing expected message; got:\n%s", out)
	}
}
```

You also need to add imports to `sets_test.go` if they aren't already there: `"bytes"`, `"errors"`, `"fmt"`, `"log/slog"`, `"strings"`. Check the existing import block (currently lines 3–14) and add only what's missing.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./internal/service -run Test_RecordSet_FailedSchedule_LogsUserAndExerciseID`

Expected: FAIL — log line is missing `user_id=` (it currently contains `workout_exercise_id=` only on the cancel path, not the schedule path; on the schedule path it has neither).

- [ ] **Step 3: Add the attributes to all five rest-push failure log sites**

In `internal/service/sets.go`, replace the `applyRestPushDecision` function (lines 97–173) so every `LogAttrs(..., slog.LevelWarn, ...)` call carries both `user_id` and `workout_exercise_id`. The replacement keeps behavior identical and only changes log attributes.

```go
// applyRestPushDecision runs the rest-push policy against the post-mutation
// slot and acts on the result. The completion itself is already persisted,
// so failures here just mean the user won't get a notification — never
// propagate. Every log line carries user_id + workout_exercise_id so
// triage can filter by either.
func (s *Service) applyRestPushDecision(
	ctx context.Context,
	userID, workoutExerciseID int,
	slot domain.ExerciseSet,
	periodization domain.PeriodizationType,
	isDeload bool,
	completedAt time.Time,
) {
	if s.scheduler == nil {
		return
	}
	logAttrs := []slog.Attr{
		slog.Int("user_id", userID),
		slog.Int("workout_exercise_id", workoutExerciseID),
	}

	decision := domain.PlanRestPush(slot, periodization, isDeload, completedAt)
	switch decision.Action {
	case domain.RestPushActionNoOp:
		return
	case domain.RestPushActionCancel:
		if err := s.scheduler.Cancel(ctx, workoutExerciseID); err != nil {
			s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: cancel failed",
				append(logAttrs, slog.Any("error", err))...)
		}
		return
	case domain.RestPushActionSchedule:
		// fall through
	}

	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: get preferences failed",
			append(logAttrs, slog.Any("error", err))...)
		return
	}
	if !prefs.RestNotificationsEnabled {
		return
	}
	subCount, err := s.repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: count subscriptions failed",
			append(logAttrs, slog.Any("error", err))...)
		return
	}
	if subCount == 0 {
		return
	}

	payloadBytes, err := json.Marshal(struct {
		Title        string `json:"title"`
		Body         string `json:"body"`
		ExerciseName string `json:"exercise_name"`
		SetNumber    int    `json:"set_number"`
		SetsTotal    int    `json:"sets_total"`
		FireAtMS     int64  `json:"fire_at_ms"`
	}{
		Title:        decision.Payload.Title,
		Body:         decision.Payload.Body,
		ExerciseName: decision.Payload.ExerciseName,
		SetNumber:    decision.Payload.NextSetNumber,
		SetsTotal:    decision.Payload.SetsTotal,
		FireAtMS:     decision.FireAt.UnixMilli(),
	})
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: marshal payload",
			append(logAttrs, slog.Any("error", err))...)
		return
	}

	push := domain.ScheduledPush{ //nolint:exhaustruct // ID and CreatedAt assigned by the repository at insert time.
		UserID:            userID,
		WorkoutExerciseID: workoutExerciseID,
		FireAt:            decision.FireAt,
		Payload:           string(payloadBytes),
	}
	if err = s.scheduler.Schedule(ctx, push); err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: schedule failed",
			append(logAttrs, slog.Any("error", err))...)
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -v ./internal/service -run Test_RecordSet_FailedSchedule_LogsUserAndExerciseID`

Expected: PASS.

- [ ] **Step 5: Run all set-related tests to confirm nothing broke**

Run: `go test ./internal/service -run RecordSet`

Expected: PASS on every existing test.

- [ ] **Step 6: Run lint-fix**

Run: `make lint-fix`

Expected: clean. (If `gocritic` flags `append(logAttrs, ...)` for shared-backing-array reuse, switch the helper to `slog.Group("ctx", logAttrs...)` or rebuild the slice per call — but on first try the `append` pattern is fine because each call site is a leaf with no further appends to `logAttrs`.)

- [ ] **Step 7: Commit**

```bash
git add internal/service/sets.go internal/service/sets_test.go
git commit -m "service: stamp user_id+workout_exercise_id on rest-push log lines

The five failure paths in applyRestPushDecision logged inconsistently
(only the cancel branch carried workout_exercise_id; none carried
user_id). Triage of \"user reports no push\" was unnecessarily hard.
All five paths now carry both keys.
"
```

---

## Task 3: Add `user_id` to scheduler `fire()` log lines

**Files:**
- Modify: `internal/notification/scheduler.go` (the `fire` method, lines 155–181)
- Test (modify): `internal/notification/scheduler_test.go` (add one new test + a small extension to `fakeDispatch`)

`Scheduler.fire()` has two `LogAttrs(..., slog.LevelWarn, ...)` calls (line 172, 177). Both carry `workout_exercise_id`; neither carries `user_id`. `push.UserID` is already available inside the method.

- [ ] **Step 1: Write the failing test**

Append to `internal/notification/scheduler_test.go`:

```go
func TestScheduler_DispatchFailure_LogsUserID(t *testing.T) {
	t.Parallel()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{ //nolint:exhaustruct // AddSource/ReplaceAttr zero.
		Level: slog.LevelDebug,
	}))
	repo := newInMemoryScheduledPushRepo()
	dispatchErr := errors.New("push gateway down")
	dispatched := make(chan struct{})
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo: repo,
		Dispatch: func(_ context.Context, _ domain.ScheduledPush) error {
			close(dispatched)
			return dispatchErr
		},
		Logger: logger,
		Now:    time.Now,
	})

	push := domain.ScheduledPush{ //nolint:exhaustruct // ID/CreatedAt assigned by the repo.
		UserID:            7,
		WorkoutExerciseID: 314,
		FireAt:            time.Now().Add(30 * time.Millisecond),
		Payload:           `{}`,
	}
	if err := scheduler.Schedule(context.Background(), push); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	select {
	case <-dispatched:
	case <-time.After(time.Second):
		t.Fatalf("dispatch never fired")
	}
	// Give the scheduler a moment to log after Dispatch returns.
	time.Sleep(50 * time.Millisecond)

	out := logBuf.String()
	if !strings.Contains(out, "user_id=7") {
		t.Errorf("log missing user_id=7; got:\n%s", out)
	}
	if !strings.Contains(out, "workout_exercise_id=314") {
		t.Errorf("log missing workout_exercise_id=314; got:\n%s", out)
	}
	if !strings.Contains(out, "push dispatch failed") {
		t.Errorf("log missing dispatch failure message; got:\n%s", out)
	}
}
```

Add the following imports to `internal/notification/scheduler_test.go` if not already present: `"bytes"`, `"log/slog"`, `"strings"`. Check the existing import block (currently lines 3–14) and add only what's missing.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./internal/notification -run TestScheduler_DispatchFailure_LogsUserID`

Expected: FAIL — `log missing user_id=7`.

- [ ] **Step 3: Add `user_id` to the two log sites in `fire`**

In `internal/notification/scheduler.go`, replace the `fire` method (lines 155–181) with:

```go
func (s *Scheduler) fire(selfBox **time.Timer, push domain.ScheduledPush) {
	s.mu.Lock()
	// Identity check: only clear the map entry if it still points at *this* timer. A concurrent
	// Schedule may have installed a replacement between our timer firing and us acquiring the
	// lock; in that case the replacement is the rightful map owner and must not be evicted.
	self := *selfBox
	if current, ok := s.timers[push.WorkoutExerciseID]; ok && current == self {
		delete(s.timers, push.WorkoutExerciseID)
	}
	s.mu.Unlock()

	// 30s is generous enough for a single Web Push round-trip plus row delete; tighter than
	// the 60s push TTL so we don't outlive the message we'd dispatch.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) //nolint:mnd // see above.
	defer cancel()

	if err := s.cfg.Dispatch(ctx, push); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "push dispatch failed",
			slog.Int("user_id", push.UserID),
			slog.Int("workout_exercise_id", push.WorkoutExerciseID),
			slog.Any("error", err))
	}
	if err := s.cfg.Repo.Delete(ctx, push.ID); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "delete scheduled push row after fire",
			slog.Int("user_id", push.UserID),
			slog.Int("workout_exercise_id", push.WorkoutExerciseID),
			slog.Int("id", push.ID),
			slog.Any("error", err))
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -v ./internal/notification -run TestScheduler_DispatchFailure_LogsUserID`

Expected: PASS.

- [ ] **Step 5: Run all scheduler tests to confirm nothing broke**

Run: `go test ./internal/notification`

Expected: PASS on every existing test.

- [ ] **Step 6: Run lint-fix**

Run: `make lint-fix`

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add internal/notification/scheduler.go internal/notification/scheduler_test.go
git commit -m "notification: stamp user_id on scheduler fire log lines

push dispatch failed / delete scheduled push row after fire both
carried workout_exercise_id but not user_id, even though push.UserID
is in scope. Triage of dispatch failures by user is now a one-line
filter.
"
```

---

## Final verification

- [ ] **Step 1: Run the full validation pipeline**

Run: `make ci`

Expected: PASS on init, build, lint-fix (no diff), test, and sec.

- [ ] **Step 2: Confirm no leftover untracked files**

Run: `git status`

Expected: working tree clean.

---

## Out of scope (intentionally deferred to other plans)

- All other 12 findings from the 2026-05-23 audit (repository hygiene, domain hygiene, handler/template refactors). See sibling plans `2026-05-23-repository-and-domain-hygiene.md` and `2026-05-23-handler-and-template-refactor.md`.
