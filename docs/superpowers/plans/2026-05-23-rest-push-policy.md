# Rest-push policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the broken session-wide "schedule the next rest push" gate with a per-slot policy that also handles warmup-complete and cancels when the slot is fully done.

**Architecture:** A pure domain function `PlanRestPush` returns a `Schedule | Cancel | NoOp` decision based on the post-mutation `ExerciseSet` slot. The service layer wires it into both `RecordSet` and `MarkWarmupComplete`, layering on I/O (preferences, subscription count, JSON marshaling, scheduler call). The notification package, repository, schema, and service-worker wire format stay unchanged.

**Tech Stack:** Go 1.26.3 (stdlib only for new code), SQLite (no schema changes), existing `internal/notification.Scheduler` for timer mechanics.

**Spec:** [docs/superpowers/specs/2026-05-23-rest-push-policy-design.md](../specs/2026-05-23-rest-push-policy-design.md)

---

## Task 1: Pure domain helper `PlanRestPush`

**Files:**
- Create: `internal/domain/rest_push.go`
- Test: `internal/domain/rest_push_test.go`

The domain function is pure (stdlib-only), uses `RestSecondsFor` already in `progression_scheme.go`, and is exercised entirely through table tests. Subsequent tasks depend on the types declared here.

- [ ] **Step 1: Write the failing test**

Create `internal/domain/rest_push_test.go`:

```go
package domain_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestPlanRestPush(t *testing.T) {
	t.Parallel()

	repMin5, repMax5 := 5, 5      // strength → 180s rest
	repMin12, repMax15 := 12, 15  // hypertrophy → 90s rest
	startSecs := 30               // for the timed-exercise case

	squat := domain.Exercise{ //nolint:exhaustruct
		Name:         "Squat",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin5,
		RepMax:       &repMax5,
	}
	plank := domain.Exercise{ //nolint:exhaustruct
		Name:                   "Plank",
		ExerciseType:           domain.ExerciseTypeTime,
		DefaultStartingSeconds: &startSecs,
	}
	curl := domain.Exercise{ //nolint:exhaustruct
		Name:         "Bicep Curl",
		ExerciseType: domain.ExerciseTypeWeighted,
		RepMin:       &repMin12,
		RepMax:       &repMax15,
	}

	completedAt := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	v := 5
	w := 100.0
	done := time.Date(2026, 5, 23, 9, 59, 0, 0, time.UTC)
	completedSet := domain.Set{ //nolint:exhaustruct
		WeightKg: &w, TargetValue: 5, CompletedValue: &v, CompletedAt: &done,
	}
	incompleteSet := domain.Set{ //nolint:exhaustruct
		WeightKg: &w, TargetValue: 5,
	}

	tests := []struct {
		name     string
		slot     domain.ExerciseSet
		pt       domain.PeriodizationType
		isDeload bool
		want     domain.RestPushDecision
	}{
		{
			name: "empty slot returns Cancel",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: squat, Sets: []domain.Set{},
			},
			pt:   domain.PeriodizationStrength,
			want: domain.RestPushDecision{Action: domain.RestPushActionCancel}, //nolint:exhaustruct
		},
		{
			name: "all sets complete returns Cancel",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: squat,
				Sets: []domain.Set{completedSet, completedSet, completedSet},
			},
			pt:   domain.PeriodizationStrength,
			want: domain.RestPushDecision{Action: domain.RestPushActionCancel}, //nolint:exhaustruct
		},
		{
			name: "no sets completed yet (warmup-just-done) schedules set 1",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: squat,
				Sets: []domain.Set{incompleteSet, incompleteSet, incompleteSet},
			},
			pt: domain.PeriodizationStrength,
			want: domain.RestPushDecision{
				Action: domain.RestPushActionSchedule,
				FireAt: completedAt.Add(180 * time.Second),
				Payload: domain.RestPushPayload{
					Title:         "Rest over",
					Body:          "Time for set 1 of 3 — Squat",
					ExerciseName:  "Squat",
					NextSetNumber: 1,
					SetsTotal:     3,
				},
			},
		},
		{
			name: "mid-exercise schedules next set",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: squat,
				Sets: []domain.Set{completedSet, completedSet, incompleteSet},
			},
			pt: domain.PeriodizationStrength,
			want: domain.RestPushDecision{
				Action: domain.RestPushActionSchedule,
				FireAt: completedAt.Add(180 * time.Second),
				Payload: domain.RestPushPayload{
					Title:         "Rest over",
					Body:          "Time for set 3 of 3 — Squat",
					ExerciseName:  "Squat",
					NextSetNumber: 3,
					SetsTotal:     3,
				},
			},
		},
		{
			name: "time-based exercise (RestSecondsFor returns 0) returns NoOp",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: plank,
				Sets: []domain.Set{incompleteSet, incompleteSet},
			},
			pt:   domain.PeriodizationStrength,
			want: domain.RestPushDecision{Action: domain.RestPushActionNoOp}, //nolint:exhaustruct
		},
		{
			name: "deload session uses deload rest mapping",
			slot: domain.ExerciseSet{ //nolint:exhaustruct
				ID: 1, Exercise: curl,
				Sets: []domain.Set{incompleteSet, incompleteSet},
			},
			pt:       domain.PeriodizationStrength,
			isDeload: true,
			want: domain.RestPushDecision{
				Action: domain.RestPushActionSchedule,
				FireAt: completedAt.Add(90 * time.Second),
				Payload: domain.RestPushPayload{
					Title:         "Rest over",
					Body:          "Time for set 1 of 2 — Bicep Curl",
					ExerciseName:  "Bicep Curl",
					NextSetNumber: 1,
					SetsTotal:     2,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.PlanRestPush(tt.slot, tt.pt, tt.isDeload, completedAt)
			if got != tt.want {
				t.Errorf("PlanRestPush() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/domain -run TestPlanRestPush`
Expected: build fails with `undefined: domain.PlanRestPush`, `undefined: domain.RestPushAction*`, `undefined: domain.RestPushDecision`, `undefined: domain.RestPushPayload`.

- [ ] **Step 3: Write the implementation**

Create `internal/domain/rest_push.go`:

```go
package domain

import (
	"fmt"
	"time"
)

// RestPushAction discriminates the three things the scheduler may be asked
// to do for a slot after a state change.
type RestPushAction int

const (
	// RestPushActionNoOp means the slot transitioned but no push should
	// be touched (e.g. the exercise has no defined rest period).
	RestPushActionNoOp RestPushAction = iota
	// RestPushActionSchedule means a push for the next incomplete set
	// should be scheduled at FireAt with Payload.
	RestPushActionSchedule
	// RestPushActionCancel means any pending push for the slot should be
	// removed (the slot has no incomplete sets left, or the exercise has
	// no rest defined; the latter is handled by NoOp, so Cancel implies
	// "all sets done").
	RestPushActionCancel
)

// RestPushPayload carries the user-visible content of a scheduled rest push.
// The scheduler / sender don't inspect these fields; the service layer
// marshals them into JSON the service worker reads.
type RestPushPayload struct {
	Title         string
	Body          string
	ExerciseName  string
	NextSetNumber int
	SetsTotal     int
}

// RestPushDecision is the value PlanRestPush returns. FireAt and Payload are
// only meaningful when Action == RestPushActionSchedule.
type RestPushDecision struct {
	Action  RestPushAction
	FireAt  time.Time
	Payload RestPushPayload
}

// PlanRestPush inspects the slot after a state change and decides what the
// push scheduler should do. completedAt is the moment the mutation happened
// — used as the rest-clock zero point. The rule is uniform across triggers
// (warmup-complete and set-complete) because both ask the same question:
// "what is the first incomplete set in this slot?"
func PlanRestPush(
	slot ExerciseSet,
	periodization PeriodizationType,
	isDeload bool,
	completedAt time.Time,
) RestPushDecision {
	nextIdx := -1
	for i := range slot.Sets {
		if slot.Sets[i].CompletedAt == nil {
			nextIdx = i
			break
		}
	}
	if nextIdx == -1 {
		// No incomplete sets remain — every set in this slot is done.
		return RestPushDecision{Action: RestPushActionCancel} //nolint:exhaustruct
	}

	restSeconds := RestSecondsFor(slot.Exercise, periodization, isDeload)
	if restSeconds <= 0 {
		return RestPushDecision{Action: RestPushActionNoOp} //nolint:exhaustruct
	}

	nextSetNumber := nextIdx + 1
	setsTotal := len(slot.Sets)
	return RestPushDecision{
		Action: RestPushActionSchedule,
		FireAt: completedAt.Add(time.Duration(restSeconds) * time.Second),
		Payload: RestPushPayload{
			Title:         "Rest over",
			Body:          fmt.Sprintf("Time for set %d of %d — %s", nextSetNumber, setsTotal, slot.Exercise.Name),
			ExerciseName:  slot.Exercise.Name,
			NextSetNumber: nextSetNumber,
			SetsTotal:     setsTotal,
		},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/domain -run TestPlanRestPush`
Expected: all subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/rest_push.go internal/domain/rest_push_test.go
git commit -m "domain: add PlanRestPush policy helper"
```

---

## Task 2: Wire `RecordSet` to the new helper, fixing the cancel-on-last-set bug

**Files:**
- Modify: `internal/service/sets.go` (replace `maybeSchedulePush` with `applyRestPushDecision`, update `RecordSet` callsite)
- Test: `internal/service/sets_test.go` (add regression test for the bug)

The current code gates scheduling on `sess.HasIncompleteSets()` — a session-wide predicate. After this task, the gate is per-slot (provided by `PlanRestPush`), the helper handles `Cancel`, and the payload is built from `RestPushPayload`.

- [ ] **Step 1: Write the failing regression test**

Append to `internal/service/sets_test.go` (after the existing `Test_RecordSet_LastSetDoesNotSchedule`):

```go
// Test_RecordSet_LastSetOfSlotWhileOtherSlotsIncomplete_Cancels exercises the
// bug where finishing the last set of one exercise scheduled a "Time for
// set N+1 of N" push because the gate was session-wide instead of per-slot.
// After the fix the policy returns Cancel for the just-finished slot.
func Test_RecordSet_LastSetOfSlotWhileOtherSlotsIncomplete_Cancels(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	// Seed a SECOND exercise slot with one incomplete set so the session
	// still has work after finishing the first slot's only set.
	var otherExerciseID int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Bench Press'`,
	).Scan(&otherExerciseID); err != nil {
		t.Fatalf("get bench press id: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	var otherWEID int
	if err := db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		 VALUES (?, ?, ?) RETURNING id`,
		userID, today, otherExerciseID,
	).Scan(&otherWEID); err != nil {
		t.Fatalf("insert second workout_exercise: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 1, 60.0, 5)`, otherWEID,
	); err != nil {
		t.Fatalf("insert other slot set: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/cancel', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	// Complete the only set in the first slot (weID).
	weight := 100.0
	date := time.Now().UTC().Truncate(24 * time.Hour)
	sig := domain.SignalOnTarget
	if err := svc.RecordSet(ctx, date, weID, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 0 {
		t.Errorf("Schedule calls = %d, want 0 (slot fully complete must not schedule)", len(fake.scheduled))
	}
	if len(fake.cancels) != 1 {
		t.Fatalf("Cancel calls = %d, want 1", len(fake.cancels))
	}
	if fake.cancels[0] != weID {
		t.Errorf("Cancel target = %d, want %d", fake.cancels[0], weID)
	}
}
```

- [ ] **Step 2: Run the new test to verify it fails**

Run: `go test -v ./internal/service -run Test_RecordSet_LastSetOfSlotWhileOtherSlotsIncomplete_Cancels`
Expected: FAIL — current code schedules a push (and never calls Cancel) because the gate is `HasIncompleteSets` session-wide.

- [ ] **Step 3: Replace `maybeSchedulePush` with `applyRestPushDecision` in `internal/service/sets.go`**

Replace the entire `maybeSchedulePush` function (lines ~99–174) with:

```go
// applyRestPushDecision runs the rest-push policy against the post-mutation
// slot and acts on the result. The completion itself is already persisted,
// so failures here just mean the user won't get a notification — never
// propagate.
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
	decision := domain.PlanRestPush(slot, periodization, isDeload, completedAt)
	switch decision.Action {
	case domain.RestPushActionNoOp:
		return
	case domain.RestPushActionCancel:
		if err := s.scheduler.Cancel(ctx, workoutExerciseID); err != nil {
			s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: cancel failed",
				slog.Int("workout_exercise_id", workoutExerciseID),
				slog.Any("error", err))
		}
		return
	case domain.RestPushActionSchedule:
		// fall through
	}

	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: get preferences failed",
			slog.Any("error", err))
		return
	}
	if !prefs.RestNotificationsEnabled {
		return
	}
	subCount, err := s.repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: count subscriptions failed",
			slog.Any("error", err))
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
			slog.Any("error", err))
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
			slog.Any("error", err))
	}
}
```

- [ ] **Step 4: Update `RecordSet` to capture the post-mutation slot and call the new helper**

Replace the body of `RecordSet` (the function starting at `internal/service/sets.go:49`) with:

```go
// RecordSet atomically persists the signal (nil for deload sets), weight
// (nil for time-based sets), completed value (reps or seconds depending on
// exercise type), and timestamp.
func (s *Service) RecordSet(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	signal *domain.Signal,
	weightKg *float64,
	completedValue int,
) error {
	var (
		wasComplete   bool
		postSlot      domain.ExerciseSet
		postSlotOK    bool
		periodization domain.PeriodizationType
		sessionDeload bool
	)
	now := time.Now().UTC()

	err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		if slot, ok := sess.Slot(workoutExerciseID); ok && setIndex >= 0 && setIndex < len(slot.Sets) {
			wasComplete = slot.Sets[setIndex].CompletedAt != nil
		}
		periodization = sess.PeriodizationType
		sessionDeload = sess.IsDeload

		if recErr := sess.RecordSet(workoutExerciseID, setIndex, signal, weightKg, completedValue, now); recErr != nil {
			// Domain sentinels propagate unchanged so callers can errors.Is at the call site;
			// the outer `if err != nil` wraps for diagnostic context.
			return recErr //nolint:wrapcheck // outer fmt.Errorf wraps with date context.
		}
		postSlot, postSlotOK = sess.Slot(workoutExerciseID)
		return nil
	})
	if err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}

	if !wasComplete && postSlotOK {
		userID := contexthelpers.AuthenticatedUserID(ctx)
		s.applyRestPushDecision(ctx, userID, workoutExerciseID, postSlot, periodization, sessionDeload, now)
	}
	return nil
}
```

- [ ] **Step 5: Run the regression test and the existing scheduling tests**

Run: `go test -v ./internal/service -run "Test_RecordSet"`
Expected: all RecordSet tests PASS, including the new `Test_RecordSet_LastSetOfSlotWhileOtherSlotsIncomplete_Cancels`.

- [ ] **Step 6: Run the full service test suite to check for regressions**

Run: `go test ./internal/service`
Expected: PASS (no other tests touched this code path).

- [ ] **Step 7: Commit**

```bash
git add internal/service/sets.go internal/service/sets_test.go
git commit -m "service: route RecordSet through PlanRestPush, cancel on last-set"
```

---

## Task 3: Schedule a push on warmup completion

**Files:**
- Modify: `internal/service/sessions.go` (extend `MarkWarmupComplete`)
- Test: `internal/service/sessions_test.go` (add scheduling tests)

`MarkWarmupComplete` currently just flips `WarmupCompletedAt`. After this task it also calls `applyRestPushDecision` so the user gets a notification for set 1.

- [ ] **Step 1: Write the failing tests**

Append to `internal/service/sessions_test.go`:

```go
func Test_MarkWarmupComplete_SchedulesPushForFirstSet(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/warmup', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 1 {
		t.Fatalf("Schedule calls = %d, want 1", len(fake.scheduled))
	}
	if fake.scheduled[0].WorkoutExerciseID != weID {
		t.Errorf("WorkoutExerciseID = %d, want %d", fake.scheduled[0].WorkoutExerciseID, weID)
	}
	if !strings.Contains(fake.scheduled[0].Payload, `"set_number":1`) {
		t.Errorf("payload = %q, want set_number=1", fake.scheduled[0].Payload)
	}
}

func Test_MarkWarmupComplete_NoSubscriptions_DoesNotSchedule(t *testing.T) {
	ctx, db, _, weID := setupSessionForRecordSet(t)
	fake := &fakeScheduler{} //nolint:exhaustruct
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 0 {
		t.Errorf("Schedule calls = %d, want 0 (no subscriptions)", len(fake.scheduled))
	}
}

func Test_MarkWarmupComplete_AfterFirstSetComplete_SchedulesSet2(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	// Seed a second set so set 2 exists for the schedule target.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 2, 100.0, 5)`, weID,
	); err != nil {
		t.Fatalf("seed second set: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/warmup-after-set1', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	weight := 100.0
	sig := domain.SignalOnTarget
	// Complete set 1 first.
	if err := svc.RecordSet(ctx, date, weID, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}
	// Now click warmup-complete (out-of-order user behavior, but legal).
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 2 {
		t.Fatalf("Schedule calls = %d, want 2 (one for set 1 complete, one for warmup)", len(fake.scheduled))
	}
	if !strings.Contains(fake.scheduled[1].Payload, `"set_number":2`) {
		t.Errorf("second payload = %q, want set_number=2 (warmup plans for first incomplete)",
			fake.scheduled[1].Payload)
	}
}

func Test_MarkWarmupComplete_AllSetsComplete_CancelsAndDoesNotSchedule(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/warmup-done', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	weight := 100.0
	sig := domain.SignalOnTarget
	// Complete the only set, then call warmup-complete on an exhausted slot.
	if err := svc.RecordSet(ctx, date, weID, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}
	preScheduleCount := 0
	preCancelCount := 0
	fake.mu.Lock()
	preScheduleCount = len(fake.scheduled)
	preCancelCount = len(fake.cancels)
	fake.mu.Unlock()

	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != preScheduleCount {
		t.Errorf("Schedule calls after warmup = %d, want %d (no schedule when all done)",
			len(fake.scheduled), preScheduleCount)
	}
	// Warmup on an all-done slot triggers a Cancel from the policy.
	if len(fake.cancels) != preCancelCount+1 {
		t.Errorf("Cancel calls after warmup = %d, want %d", len(fake.cancels), preCancelCount+1)
	}
}

func Test_MarkWarmupComplete_AlreadyDone_DoesNotReschedule(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/warmup-dup', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("first MarkWarmupComplete: %v", err)
	}
	if err := svc.MarkWarmupComplete(ctx, date, weID); err != nil {
		t.Fatalf("second MarkWarmupComplete: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 1 {
		t.Errorf("Schedule calls = %d, want 1 (second call is a no-op)", len(fake.scheduled))
	}
}
```

Make sure `internal/service/sessions_test.go` imports `"strings"` (add it to the existing import block if absent).

- [ ] **Step 2: Run tests to verify the first two fail and the third passes-by-accident**

Run: `go test -v ./internal/service -run "Test_MarkWarmupComplete_"`
Expected:
- `SchedulesPushForFirstSet` FAILs (`Schedule calls = 0, want 1`)
- `AfterFirstSetComplete_SchedulesSet2` FAILs (`Schedule calls = 1, want 2`)
- `AllSetsComplete_CancelsAndDoesNotSchedule` FAILs (`Cancel calls after warmup = 0, want 1`)
- `NoSubscriptions_DoesNotSchedule` PASSes (current code never schedules from warmup)
- `AlreadyDone_DoesNotReschedule` PASSes for the wrong reason (current code never schedules) — locks the behavior once we add scheduling

- [ ] **Step 3: Modify `MarkWarmupComplete` in `internal/service/sessions.go`**

Replace the function (currently at lines 295–307) with:

```go
// MarkWarmupComplete marks the warmup as complete for a specific workout exercise slot.
// Schedules a rest push announcing set 1 when the warmup transitions from
// not-done to done, the user has push enabled, and at least one subscription
// exists. Re-clicking when the warmup is already done is a no-op on the
// push side (the underlying domain mutation still refreshes the timestamp,
// preserving prior behavior).
func (s *Service) MarkWarmupComplete(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
) error {
	var (
		wasComplete   bool
		postSlot      domain.ExerciseSet
		postSlotOK    bool
		periodization domain.PeriodizationType
		sessionDeload bool
	)
	now := time.Now().UTC()

	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		if slot, ok := sess.Slot(workoutExerciseID); ok {
			wasComplete = slot.WarmupCompletedAt != nil
		}
		periodization = sess.PeriodizationType
		sessionDeload = sess.IsDeload

		if mErr := sess.MarkWarmupComplete(workoutExerciseID, now); mErr != nil {
			return mErr //nolint:wrapcheck // outer fmt.Errorf wraps with date context.
		}
		postSlot, postSlotOK = sess.Slot(workoutExerciseID)
		return nil
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}

	if !wasComplete && postSlotOK {
		userID := contexthelpers.AuthenticatedUserID(ctx)
		s.applyRestPushDecision(ctx, userID, workoutExerciseID, postSlot, periodization, sessionDeload, now)
	}
	return nil
}
```

- [ ] **Step 4: Run the warmup tests to verify they pass**

Run: `go test -v ./internal/service -run "Test_MarkWarmupComplete_"`
Expected: all three PASS.

- [ ] **Step 5: Run the full service test suite for regressions**

Run: `go test ./internal/service`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/service/sessions.go internal/service/sessions_test.go
git commit -m "service: schedule rest push on warmup completion"
```

---

## Task 4: Guard tests for non-trigger paths

**Files:**
- Test: `internal/service/sets_test.go` (add invariant tests)

These tests document and lock in the rule that mutations which don't transition a slot toward completion never touch the scheduler. They should pass against the post-Task-3 code without further implementation changes.

- [ ] **Step 1: Add the invariant tests**

Append to `internal/service/sets_test.go`:

```go
// Test_UpdateCompletedValue_DoesNotTouchScheduler locks in that editing the
// recorded value on an already-complete set is bookkeeping, not progress —
// no Schedule and no Cancel calls should reach the scheduler.
func Test_UpdateCompletedValue_DoesNotTouchScheduler(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/edit', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	weight := 100.0
	sig := domain.SignalOnTarget
	if err := svc.RecordSet(ctx, date, weID, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet (seed completion): %v", err)
	}

	fake.mu.Lock()
	scheduledBefore := len(fake.scheduled)
	cancelsBefore := len(fake.cancels)
	fake.mu.Unlock()

	if err := svc.UpdateCompletedValue(ctx, date, weID, 0, 6); err != nil {
		t.Fatalf("UpdateCompletedValue: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != scheduledBefore {
		t.Errorf("Schedule calls after edit = %d, want %d", len(fake.scheduled), scheduledBefore)
	}
	if len(fake.cancels) != cancelsBefore {
		t.Errorf("Cancel calls after edit = %d, want %d", len(fake.cancels), cancelsBefore)
	}
}

// Test_UpdateSetWeight_DoesNotTouchScheduler locks in the same invariant
// for weight edits.
func Test_UpdateSetWeight_DoesNotTouchScheduler(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/weight-edit', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.UpdateSetWeight(ctx, date, weID, 0, 105.0); err != nil {
		t.Fatalf("UpdateSetWeight: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 0 {
		t.Errorf("Schedule calls = %d, want 0 (weight edit is not a trigger)", len(fake.scheduled))
	}
	if len(fake.cancels) != 0 {
		t.Errorf("Cancel calls = %d, want 0 (weight edit is not a trigger)", len(fake.cancels))
	}
}

// Test_RecordSet_RerecordCompletedSet_DoesNotReschedule locks in the
// !wasComplete guard: re-recording an already-complete set produces no
// new scheduler interactions.
func Test_RecordSet_RerecordCompletedSet_DoesNotReschedule(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	// Seed a second set so the first completion schedules a push.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 2, 100.0, 5)`, weID,
	); err != nil {
		t.Fatalf("seed second set: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/rerecord', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{} //nolint:exhaustruct
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	date := time.Now().UTC().Truncate(24 * time.Hour)
	weight := 100.0
	sig := domain.SignalOnTarget
	if err := svc.RecordSet(ctx, date, weID, 0, &sig, &weight, 5); err != nil {
		t.Fatalf("RecordSet (first): %v", err)
	}

	fake.mu.Lock()
	if len(fake.scheduled) != 1 {
		fake.mu.Unlock()
		t.Fatalf("after first RecordSet, scheduled = %d, want 1", len(fake.scheduled))
	}
	fake.mu.Unlock()

	// Re-record the same set with a different value. wasComplete is true now,
	// so the policy must not be re-invoked.
	if err := svc.RecordSet(ctx, date, weID, 0, &sig, &weight, 6); err != nil {
		t.Fatalf("RecordSet (re-record): %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 1 {
		t.Errorf("Schedule calls after re-record = %d, want 1", len(fake.scheduled))
	}
	if len(fake.cancels) != 0 {
		t.Errorf("Cancel calls after re-record = %d, want 0", len(fake.cancels))
	}
}
```

- [ ] **Step 2: Run the new tests**

Run: `go test -v ./internal/service -run "Test_UpdateCompletedValue_DoesNotTouchScheduler|Test_UpdateSetWeight_DoesNotTouchScheduler|Test_RecordSet_RerecordCompletedSet_DoesNotReschedule"`
Expected: all PASS — these are invariant guards, not behavior changes.

- [ ] **Step 3: Commit**

```bash
git add internal/service/sets_test.go
git commit -m "service: guard tests for non-trigger push paths"
```

---

## Task 5: Full validation

**Files:** none modified

- [ ] **Step 1: Run lint with auto-fix**

Run: `make lint-fix`
Expected: clean exit. If lint flags anything in the new code (e.g. an `exhaustruct` complaint), follow its suggestion or add a targeted `//nolint:exhaustruct` directive consistent with neighboring code.

- [ ] **Step 2: Run the full test suite with race detector and shuffle**

Run: `make test`
Expected: PASS across all packages.

- [ ] **Step 3: Smoke-run the binary**

Run: `make ci`
Expected: build + lint-fix + test + sec all clean.

- [ ] **Step 4: If lint-fix or sec made any changes, commit them**

```bash
git status
# if there are changes:
git add -u
git commit -m "chore: lint-fix after rest-push policy changes"
```

If `git status` is clean, skip this step.

---

## Notes for the implementer

- **Do not change `internal/notification/`.** The scheduler's `Schedule` / `Cancel` / `CancelForWorkout` contract is unchanged; the new policy lives in `internal/domain/` and is consumed by `internal/service/`.
- **The wire JSON format is preserved exactly.** `applyRestPushDecision` marshals an inline struct with `json:"set_number"`, `json:"sets_total"`, `json:"fire_at_ms"` — same field names the service worker (`ui/static/sw.js`) reads. If you find yourself renaming any JSON tag, stop: that's a wire-format change requiring a service-worker update.
- **`maybeSchedulePush` is fully replaced, not renamed.** The new helper is `applyRestPushDecision` with a different signature (it takes a `slot` instead of an `exercise + completedSetNumber + setsTotal`). All call sites move to the new helper in Task 2.
- **There is one new call site (warmup) and one existing call site (RecordSet).** Nothing else in `internal/service/` should call `applyRestPushDecision` — `UpdateCompletedValue`, `UpdateSetWeight`, `SwapExerciseInSlot`, `CompleteSession` deliberately don't.
- **The cancel-vs-fire race in the scheduler is out of scope.** See spec § "Edge cases (accepted, not fixed)" §1.
