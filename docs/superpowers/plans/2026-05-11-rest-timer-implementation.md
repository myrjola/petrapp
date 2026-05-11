# Rest Timer With Web Push Notifications — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Schedule a Web Push notification `RestSeconds` after each completed set so the user gets pinged when their rest is up, even with the PWA backgrounded or the phone locked. Render an inline countdown chip on the active set card for the foreground case. Opt-in lives in `/preferences`.

**Architecture:** A new `internal/notification` package owns `Sender` (wraps webpush-go), `Scheduler` (in-process `time.AfterFunc` map keyed by `workout_exercise_id`, persisted in SQLite so pending pushes survive restarts), and `IdleMonitor` (SIGTERMs the process after N seconds idle with no pending pushes, so Fly's auto-start replaces auto-stop). Two new tables — `push_subscriptions` and `scheduled_pushes` — store device registrations and pending fires. Service-layer `RecordSet` schedules; `CompleteSession` and `SwapExercise` cancel.

**Tech Stack:** Go (stdlib + `internal/domain` + `internal/repository`), `github.com/SherClockHolmes/webpush-go` v1.4.0, SQLite (declarative schema migrator), html/template, vanilla JS service worker.

**Spec:** `docs/superpowers/specs/2026-05-11-rest-timer-design.md`.

---

## File Structure

### New files

| File | Responsibility |
|---|---|
| `internal/notification/sender.go` | `Sender` struct wrapping `webpush.SendNotification`; bare-email Subscriber claim; `ErrSubscriptionGone` sentinel for 410/404. |
| `internal/notification/sender_test.go` | `httptest.Server` impersonates Apple/FCM; verifies JWT `sub` is bare email (no `mailto:mailto:` double-prefix), 410 returns `ErrSubscriptionGone`, 5xx returns wrapped error. |
| `internal/notification/scheduler.go` | `Scheduler` struct; `Schedule`, `Cancel`, `CancelForWorkout`, `Reload`, `PendingCount`. Goroutine-safe in-memory `map[int]*time.Timer`. |
| `internal/notification/scheduler_test.go` | Short-duration timer tests: schedule→fire, schedule→cancel, schedule→replace, reload reconstitutes timers. |
| `internal/notification/idle_monitor.go` | `IdleMonitor` struct; ticks every `tickInterval`; when idle threshold reached AND `PendingCount()==0`, sends `SIGTERM` to self. |
| `internal/notification/idle_monitor_test.go` | Mock clock + pending-count signal; verifies SIGTERM-trigger conditions. |
| `internal/repository/push_subscription.go` | SQLite repo: `Insert`, `DeleteByEndpoint`, `DeleteByID`, `ListByUser`, `CountByUser`. |
| `internal/repository/push_subscription_test.go` | Round-trip persistence test. |
| `internal/repository/scheduled_push.go` | SQLite repo: `Replace`, `Delete`, `DeleteByWorkout`, `ListAll`, `Get`. |
| `internal/repository/scheduled_push_test.go` | Round-trip + ON CONFLICT upsert test. |
| `cmd/web/handler-push.go` | `POST /api/push/subscribe`, `POST /api/push/unsubscribe`. JSON body parsing; ties subscription to authed user. |
| `cmd/web/handler-push_test.go` | E2E subscribe/unsubscribe round-trip via `e2etest`. |
| `ui/static/sw.js` | Service worker: `push` event → `showNotification`; `notificationclick` → focus or open `/workouts/<today>`. |

### Modified files

| File | Change |
|---|---|
| `go.mod` / `go.sum` | Re-add `github.com/SherClockHolmes/webpush-go v1.4.0`. |
| `internal/sqlite/schema.sql` | Add `push_subscriptions` and `scheduled_pushes` tables; add `rest_notifications_enabled` column to `workout_preferences`. |
| `internal/domain/progression_scheme.go` | Add `RestSecondsFor(Exercise, PeriodizationType) int` free function. |
| `internal/domain/preferences.go` | Add `RestNotificationsEnabled bool` field; update zero-value handling. |
| `internal/domain/session.go` | Add `(*Session).HasIncompleteSets() bool` aggregate method. |
| `internal/repository/preferences.go` | SELECT/UPSERT the new column. |
| `internal/repository/repository.go` | Add `PushSubscription` and `ScheduledPush` fields to `Repositories`; wire them in `New`. |
| `internal/service/service.go` | Accept optional `notification.Scheduler` + `Sender`; store on `Service`. Add `NotificationsEnabled(ctx) (bool, error)` and `HasPushSubscription(ctx) (bool, error)`. |
| `internal/service/sets.go` | `RecordSet` schedules a push after successful mutation if conditions all true. |
| `internal/service/sessions.go` | `CompleteSession` cancels pending pushes for the session. |
| `internal/service/exercises.go` | `SwapExercise` cancels the slot's pending push. |
| `internal/service/sets_test.go` | Extend: verify scheduling fires on RecordSet; last-set skips. |
| `internal/service/sessions_test.go` | Extend: verify CompleteSession cancels. |
| `cmd/web/main.go` | Read VAPID env vars; instantiate `notification.Sender` + `Scheduler` + `IdleMonitor`; call `Scheduler.Reload`; start `IdleMonitor` goroutine; pass scheduler to `service.NewService`. |
| `cmd/web/middleware.go` | Add `lastRequestAt` atomic timestamp updated by a small middleware; expose to `IdleMonitor`. |
| `cmd/web/routes.go` | Register `/api/push/subscribe` and `/api/push/unsubscribe`. |
| `cmd/web/handler-preferences.go` | Extend template data with `RestNotificationsEnabled`, `VAPIDPublicKey`, `PushSubscriptionCount`. Preferences POST persists the toggle. |
| `cmd/web/handler-exerciseset.go` | Compute `RestEndAtMs` from `LastCompletedAt + RestSeconds * 1000`; pass to template. |
| `ui/templates/pages/preferences/preferences.gohtml` | Add "Rest notifications" section with three states. |
| `ui/templates/pages/exerciseset/sets-container.gohtml` | Render `.rest-chip` above the active set form when `RestEndAtMs > 0`; inline JS ticks every 250ms. |
| `ui/templates/base.gohtml` | Register `/sw.js` unconditionally when `'serviceWorker' in navigator`. |
| `scripts/dev.sh` | Generate ephemeral VAPID keys if `PETRAPP_VAPID_PUBLIC` unset; export and print. |
| `scripts/dev-tailscale-https.sh` | Same key-generation behaviour. |
| `fly.toml` | `auto_stop_machines = "off"` (was `"stop"`). |

### Untouched

- `internal/sqlite/premigrate.go` — no premigration needed (schema additions are non-destructive). The declarative migrator adds the columns/tables in-place.
- All other test files and templates.

---

## Sequencing rationale

The plan builds bottom-up: dep + schema first, then domain/repo, then the new `notification` package (which has no upstream callers yet), then service integration (which wires the new package into `RecordSet`), then HTTP, then UI. Each task leaves the tree compiling and `make test` green.

The `notification` package can be developed in isolation against `httptest.Server` and short-duration timers — no Fly Machine, no real Apple endpoint. Wiring into `main.go` happens late (Task 13) once all pieces exist.

`fly.toml` change (Task 14) is last because it depends on the `IdleMonitor` actually shipping. Until it does, leaving `auto_stop_machines = "stop"` would silently kill pending pushes.

---

## Tasks

### Task 1: Add webpush-go dep + schema migration

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `internal/sqlite/schema.sql`

- [ ] **Step 1: Add the webpush-go dependency**

Run:

```bash
go get github.com/SherClockHolmes/webpush-go@v1.4.0
```

Expected: `go.mod` gains a `require` line for `github.com/SherClockHolmes/webpush-go v1.4.0`, `go.sum` updated.

- [ ] **Step 2: Add the two new tables and the preferences column to the schema**

Edit `internal/sqlite/schema.sql`. Append the following tables after the existing `feature_flags` block:

```sql
-------------------
-- Push delivery --
-------------------

CREATE TABLE push_subscriptions
(
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint   TEXT    NOT NULL UNIQUE CHECK (LENGTH(endpoint) < 1024),
    p256dh     TEXT    NOT NULL CHECK (LENGTH(p256dh) < 256),
    auth       TEXT    NOT NULL CHECK (LENGTH(auth) < 256),
    created_at TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at)
) STRICT;

CREATE INDEX push_subscriptions_user_id ON push_subscriptions (user_id);

CREATE TABLE scheduled_pushes
(
    id                  INTEGER PRIMARY KEY,
    user_id             INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
    fire_at             TEXT    NOT NULL CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', fire_at) = fire_at),
    payload             TEXT    NOT NULL CHECK (LENGTH(payload) < 2048),
    created_at          TEXT    NOT NULL DEFAULT (STRFTIME('%Y-%m-%dT%H:%M:%fZ'))
        CHECK (STRFTIME('%Y-%m-%dT%H:%M:%fZ', created_at) = created_at)
) STRICT;

CREATE UNIQUE INDEX scheduled_pushes_workout_exercise_id
    ON scheduled_pushes (workout_exercise_id);
CREATE INDEX scheduled_pushes_fire_at ON scheduled_pushes (fire_at);
```

Add the column to `workout_preferences`. Replace the existing block:

```sql
CREATE TABLE workout_preferences
(
    user_id                    INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    monday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (monday_minutes IN (0, 45, 60, 90)),
    tuesday_minutes            INTEGER NOT NULL DEFAULT 0 CHECK (tuesday_minutes IN (0, 45, 60, 90)),
    wednesday_minutes          INTEGER NOT NULL DEFAULT 0 CHECK (wednesday_minutes IN (0, 45, 60, 90)),
    thursday_minutes           INTEGER NOT NULL DEFAULT 0 CHECK (thursday_minutes IN (0, 45, 60, 90)),
    friday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (friday_minutes IN (0, 45, 60, 90)),
    saturday_minutes           INTEGER NOT NULL DEFAULT 0 CHECK (saturday_minutes IN (0, 45, 60, 90)),
    sunday_minutes             INTEGER NOT NULL DEFAULT 0 CHECK (sunday_minutes IN (0, 45, 60, 90)),
    rest_notifications_enabled INTEGER NOT NULL DEFAULT 1 CHECK (rest_notifications_enabled IN (0, 1))
) WITHOUT ROWID, STRICT;
```

- [ ] **Step 3: Verify migrations apply cleanly**

Run:

```bash
rm -f petrapp.sqlite3 && make test
```

Expected: `make test` passes. Schema differences are picked up by the declarative migrator; no premigration is needed because all changes are additive (new tables, new column with a DEFAULT).

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum internal/sqlite/schema.sql
git commit -m "Add webpush-go dep and rest-timer schema"
```

---

### Task 2: Domain helpers — `RestSecondsFor`, `Preferences.RestNotificationsEnabled`, `Session.HasIncompleteSets`

**Files:**
- Modify: `internal/domain/progression_scheme.go`
- Modify: `internal/domain/preferences.go`
- Modify: `internal/domain/session.go`
- Test: `internal/domain/progression_scheme_test.go`
- Test: `internal/domain/session_test.go`

- [ ] **Step 1: Write the failing test for `RestSecondsFor`**

Append to `internal/domain/progression_scheme_test.go`:

```go
func TestRestSecondsFor(t *testing.T) {
	t.Parallel()

	repMin5, repMax5 := 5, 5
	repMin6, repMax10 := 6, 10
	repMin12, repMax15 := 12, 15
	startSecs := 30

	tests := []struct {
		name string
		ex   domain.Exercise
		pt   domain.PeriodizationType
		want int
	}{
		{
			name: "weighted strength 5 reps → 180s",
			ex: domain.Exercise{
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       &repMin5, RepMax: &repMax5,
			},
			pt:   domain.PeriodizationStrength,
			want: 180,
		},
		{
			name: "weighted hypertrophy 10 reps → 150s",
			ex: domain.Exercise{
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       &repMin6, RepMax: &repMax10,
			},
			pt:   domain.PeriodizationHypertrophy,
			want: 150,
		},
		{
			name: "weighted hypertrophy 15 reps → 90s",
			ex: domain.Exercise{
				ExerciseType: domain.ExerciseTypeWeighted,
				RepMin:       &repMin12, RepMax: &repMax15,
			},
			pt:   domain.PeriodizationHypertrophy,
			want: 90,
		},
		{
			name: "time-based exercise → 0 (no scheduling)",
			ex: domain.Exercise{
				ExerciseType:           domain.ExerciseTypeTime,
				DefaultStartingSeconds: &startSecs,
			},
			pt:   domain.PeriodizationStrength,
			want: 0,
		},
		{
			name: "rep-based with nil rep window → 0 (defensive)",
			ex: domain.Exercise{
				ExerciseType: domain.ExerciseTypeWeighted,
			},
			pt:   domain.PeriodizationStrength,
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := domain.RestSecondsFor(tt.ex, tt.pt)
			if got != tt.want {
				t.Errorf("RestSecondsFor() = %d, want %d", got, tt.want)
			}
		})
	}
}
```

Imports already present in the file. Verify the package declaration and existing imports cover `domain` — if the file is `package domain_test`, keep the existing pattern.

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/domain -run TestRestSecondsFor
```

Expected: FAIL — undefined `domain.RestSecondsFor`.

- [ ] **Step 3: Implement `RestSecondsFor`**

Append to `internal/domain/progression_scheme.go`:

```go
// RestSecondsFor returns the inter-set rest in seconds for the given exercise
// under the session's periodization. Returns 0 for time-based exercises and
// for exercises with missing rep windows — service code treats 0 as "no
// rest scheduling".
func RestSecondsFor(ex Exercise, pt PeriodizationType) int {
	if ex.IsTimed() {
		return 0
	}
	if ex.RepMin == nil || ex.RepMax == nil {
		return 0
	}
	return DeriveScheme(*ex.RepMin, *ex.RepMax, pt).RestSeconds
}
```

- [ ] **Step 4: Verify the test passes**

Run:

```bash
go test ./internal/domain -run TestRestSecondsFor
```

Expected: PASS.

- [ ] **Step 5: Add `RestNotificationsEnabled` to `Preferences`**

Edit `internal/domain/preferences.go`. Update the struct:

```go
type Preferences struct {
	MondayMinutes              int
	TuesdayMinutes             int
	WednesdayMinutes           int
	ThursdayMinutes            int
	FridayMinutes              int
	SaturdayMinutes            int
	SundayMinutes              int
	RestNotificationsEnabled   bool
}
```

The zero value is `false`. The repo's `Get` and `Set` will be updated in Task 4 to read/write the column; the column defaults to `1` (true) in SQL, so first-time users see the toggle as on. Domain code does NOT default this to true — the persistence layer owns the default.

- [ ] **Step 6: Write the failing test for `Session.HasIncompleteSets`**

Append to `internal/domain/session_test.go`:

```go
func TestSessionHasIncompleteSets(t *testing.T) {
	t.Parallel()

	completedAt := time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC)
	completedVal := 5
	completedSet := domain.Set{
		WeightKg:       nil,
		TargetValue:    5,
		CompletedValue: &completedVal,
		CompletedAt:    &completedAt,
		Signal:         nil,
	}
	incompleteSet := domain.Set{TargetValue: 5} //nolint:exhaustruct

	tests := []struct {
		name string
		sess domain.Session
		want bool
	}{
		{
			name: "empty session — no sets, no incomplete",
			sess: domain.Session{}, //nolint:exhaustruct
			want: false,
		},
		{
			name: "all sets complete",
			sess: domain.Session{ //nolint:exhaustruct
				ExerciseSets: []domain.ExerciseSet{
					{ID: 1, Sets: []domain.Set{completedSet, completedSet}}, //nolint:exhaustruct
				},
			},
			want: false,
		},
		{
			name: "one set incomplete in a later slot",
			sess: domain.Session{ //nolint:exhaustruct
				ExerciseSets: []domain.ExerciseSet{
					{ID: 1, Sets: []domain.Set{completedSet, completedSet}}, //nolint:exhaustruct
					{ID: 2, Sets: []domain.Set{completedSet, incompleteSet}}, //nolint:exhaustruct
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.sess.HasIncompleteSets()
			if got != tt.want {
				t.Errorf("HasIncompleteSets() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 7: Run the test to verify it fails**

Run:

```bash
go test ./internal/domain -run TestSessionHasIncompleteSets
```

Expected: FAIL — undefined method.

- [ ] **Step 8: Implement `HasIncompleteSets`**

Append to `internal/domain/session.go`:

```go
// HasIncompleteSets reports whether any set across any exercise slot in the
// session has not yet been completed. Used by the service layer to decide
// whether a just-completed set is the final set of the workout — if so, no
// rest push should be scheduled.
func (s *Session) HasIncompleteSets() bool {
	for i := range s.ExerciseSets {
		for j := range s.ExerciseSets[i].Sets {
			if s.ExerciseSets[i].Sets[j].CompletedAt == nil {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 9: Verify all tests pass**

Run:

```bash
go test ./internal/domain
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/domain/
git commit -m "Add RestSecondsFor, RestNotificationsEnabled, HasIncompleteSets"
```

---

### Task 3: Update PreferencesRepository to persist `rest_notifications_enabled`

**Files:**
- Modify: `internal/repository/preferences.go`
- Test: `internal/repository/preferences_test.go`

- [ ] **Step 1: Write the failing test**

Edit `internal/repository/preferences_test.go`. Add a test (use the existing setup helpers from the file):

```go
func TestPreferences_RestNotificationsEnabled_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	// Default for first-time users is true.
	prefs, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !prefs.RestNotificationsEnabled {
		t.Errorf("default RestNotificationsEnabled = false, want true")
	}

	// Flip to false and confirm.
	prefs.RestNotificationsEnabled = false
	if err = repos.Preferences.Set(ctx, prefs); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := repos.Preferences.Get(ctx)
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	if got.RestNotificationsEnabled {
		t.Errorf("after Set false, got true")
	}
}
```

If `setupTestRepos` doesn't exist with that exact signature, look at the other tests in `preferences_test.go` for the actual helper name and signature; reuse the same setup.

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/repository -run TestPreferences_RestNotificationsEnabled
```

Expected: FAIL — column missing from SELECT, or default not honored.

- [ ] **Step 3: Update `Get` and `Set` to include the new column**

Edit `internal/repository/preferences.go`. Replace the `Get` SELECT:

```go
err := r.db.ReadOnly.QueryRowContext(ctx, `
    SELECT monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
           friday_minutes, saturday_minutes, sunday_minutes,
           rest_notifications_enabled
    FROM workout_preferences
    WHERE user_id = ?`, userID).Scan(
    &prefs.MondayMinutes, &prefs.TuesdayMinutes, &prefs.WednesdayMinutes, &prefs.ThursdayMinutes,
    &prefs.FridayMinutes, &prefs.SaturdayMinutes, &prefs.SundayMinutes,
    &prefs.RestNotificationsEnabled,
)
```

For the "no row" branch, default `RestNotificationsEnabled` to `true` to match the SQL DEFAULT — first-time users see the toggle on:

```go
if errors.Is(err, sql.ErrNoRows) {
    return domain.Preferences{ //nolint:exhaustruct // weekly minutes zero by design.
        RestNotificationsEnabled: true,
    }, nil
}
```

Replace the `Set` UPSERT:

```go
if _, err := r.db.ReadWrite.ExecContext(ctx, `
    INSERT INTO workout_preferences (
        user_id, monday_minutes, tuesday_minutes, wednesday_minutes, thursday_minutes,
        friday_minutes, saturday_minutes, sunday_minutes, rest_notifications_enabled
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT (user_id) DO UPDATE SET
        monday_minutes = excluded.monday_minutes,
        tuesday_minutes = excluded.tuesday_minutes,
        wednesday_minutes = excluded.wednesday_minutes,
        thursday_minutes = excluded.thursday_minutes,
        friday_minutes = excluded.friday_minutes,
        saturday_minutes = excluded.saturday_minutes,
        sunday_minutes = excluded.sunday_minutes,
        rest_notifications_enabled = excluded.rest_notifications_enabled`,
    userID,
    prefs.MondayMinutes, prefs.TuesdayMinutes, prefs.WednesdayMinutes, prefs.ThursdayMinutes,
    prefs.FridayMinutes, prefs.SaturdayMinutes, prefs.SundayMinutes,
    prefs.RestNotificationsEnabled,
); err != nil {
    return fmt.Errorf("save workout preferences: %w", err)
}
```

- [ ] **Step 4: Verify tests pass**

Run:

```bash
go test ./internal/repository
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/repository/preferences.go internal/repository/preferences_test.go
git commit -m "Persist rest_notifications_enabled in PreferencesRepository"
```

---

### Task 4: `PushSubscriptionRepository`

**Files:**
- Create: `internal/repository/push_subscription.go`
- Test: `internal/repository/push_subscription_test.go`
- Modify: `internal/repository/repository.go`

- [ ] **Step 1: Add the interface**

Edit `internal/repository/repository.go`. Add to the interface block:

```go
// PushSubscriptionRepository persists per-device Web Push subscriptions.
type PushSubscriptionRepository interface {
	Insert(ctx context.Context, sub domain.PushSubscription) (domain.PushSubscription, error)
	DeleteByEndpoint(ctx context.Context, endpoint string) error
	DeleteByID(ctx context.Context, id int) error
	ListByUser(ctx context.Context) ([]domain.PushSubscription, error)
	CountByUser(ctx context.Context) (int, error)
}
```

Add the field to `Repositories`:

```go
type Repositories struct {
	Sessions          SessionRepository
	Exercises         ExerciseRepository
	Preferences       PreferencesRepository
	FeatureFlags      FeatureFlagRepository
	MuscleTargets     MuscleGroupTargetRepository
	PushSubscriptions PushSubscriptionRepository
}
```

And wire it in `New`:

```go
pushSubs := newSQLitePushSubscriptionRepository(db)
return &Repositories{
    Preferences:       prefs,
    MuscleTargets:     muscleTargets,
    FeatureFlags:      featureFlags,
    Exercises:         exercises,
    Sessions:          sessions,
    PushSubscriptions: pushSubs,
}
```

- [ ] **Step 2: Add the domain type**

Create `internal/domain/push_subscription.go`:

```go
package domain

import "time"

// PushSubscription is one device's Web Push subscription. Stored per-user;
// a user may have multiple devices.
type PushSubscription struct {
	ID        int
	UserID    int
	Endpoint  string
	P256dh    string
	Auth      string
	CreatedAt time.Time
}
```

- [ ] **Step 3: Write the failing test**

Create `internal/repository/push_subscription_test.go`:

```go
package repository_test

import (
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestPushSubscriptions_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	sub := domain.PushSubscription{ //nolint:exhaustruct
		Endpoint: "https://web.push.apple.com/foo",
		P256dh:   "BPa-abc",
		Auth:     "auth-xyz",
	}
	got, err := repos.PushSubscriptions.Insert(ctx, sub)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got.ID == 0 {
		t.Errorf("Insert returned ID 0")
	}

	subs, err := repos.PushSubscriptions.ListByUser(ctx)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("ListByUser returned %d, want 1", len(subs))
	}
	if subs[0].Endpoint != sub.Endpoint {
		t.Errorf("endpoint = %q, want %q", subs[0].Endpoint, sub.Endpoint)
	}

	count, err := repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		t.Fatalf("CountByUser: %v", err)
	}
	if count != 1 {
		t.Errorf("CountByUser = %d, want 1", count)
	}

	if err = repos.PushSubscriptions.DeleteByEndpoint(ctx, sub.Endpoint); err != nil {
		t.Fatalf("DeleteByEndpoint: %v", err)
	}

	count, _ = repos.PushSubscriptions.CountByUser(ctx)
	if count != 0 {
		t.Errorf("after delete: count = %d, want 0", count)
	}
}

func TestPushSubscriptions_InsertReplacesByEndpoint(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	sub := domain.PushSubscription{ //nolint:exhaustruct
		Endpoint: "https://fcm.googleapis.com/wp/abc",
		P256dh:   "old",
		Auth:     "old",
	}
	if _, err := repos.PushSubscriptions.Insert(ctx, sub); err != nil {
		t.Fatalf("first Insert: %v", err)
	}
	sub.P256dh = "new"
	sub.Auth = "new"
	if _, err := repos.PushSubscriptions.Insert(ctx, sub); err != nil {
		t.Fatalf("second Insert: %v", err)
	}
	subs, _ := repos.PushSubscriptions.ListByUser(ctx)
	if len(subs) != 1 {
		t.Fatalf("got %d rows, want 1", len(subs))
	}
	if subs[0].P256dh != "new" {
		t.Errorf("P256dh = %q, want updated value", subs[0].P256dh)
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run:

```bash
go test ./internal/repository -run TestPushSubscriptions
```

Expected: FAIL — `newSQLitePushSubscriptionRepository` undefined.

- [ ] **Step 5: Implement the SQLite repository**

Create `internal/repository/push_subscription.go`:

```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqlitePushSubscriptionRepository struct {
	baseRepository
}

func newSQLitePushSubscriptionRepository(db *sqlite.Database) *sqlitePushSubscriptionRepository {
	return &sqlitePushSubscriptionRepository{baseRepository: newBaseRepository(db)}
}

// Insert upserts a push subscription keyed by endpoint. Duplicates are not
// possible (UNIQUE constraint); a second registration with the same endpoint
// rebinds keys to the authenticated user — useful when iOS rotates the auth
// secret on its own.
func (r *sqlitePushSubscriptionRepository) Insert(
	ctx context.Context, sub domain.PushSubscription,
) (domain.PushSubscription, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)

	err := r.db.ReadWrite.QueryRowContext(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET
		    user_id = excluded.user_id,
		    p256dh  = excluded.p256dh,
		    auth    = excluded.auth
		RETURNING id, created_at`,
		userID, sub.Endpoint, sub.P256dh, sub.Auth,
	).Scan(&sub.ID, &sub.CreatedAt)
	if err != nil {
		return domain.PushSubscription{}, fmt.Errorf("insert push subscription: %w", err)
	}
	sub.UserID = userID
	return sub, nil
}

func (r *sqlitePushSubscriptionRepository) DeleteByEndpoint(
	ctx context.Context, endpoint string,
) error {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE user_id = ? AND endpoint = ?`,
		userID, endpoint,
	); err != nil {
		return fmt.Errorf("delete push subscription by endpoint: %w", err)
	}
	return nil
}

func (r *sqlitePushSubscriptionRepository) DeleteByID(ctx context.Context, id int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		`DELETE FROM push_subscriptions WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("delete push subscription by id: %w", err)
	}
	return nil
}

func (r *sqlitePushSubscriptionRepository) ListByUser(ctx context.Context) (_ []domain.PushSubscription, err error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, user_id, endpoint, p256dh, auth, created_at
		FROM push_subscriptions
		WHERE user_id = ?
		ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("query push subscriptions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var subs []domain.PushSubscription
	for rows.Next() {
		var sub domain.PushSubscription
		var createdAt string
		if err = rows.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth, &createdAt); err != nil {
			return nil, fmt.Errorf("scan push subscription: %w", err)
		}
		if sub.CreatedAt, err = parseTimestamp(createdAt); err != nil {
			return nil, fmt.Errorf("parse push subscription created_at: %w", err)
		}
		subs = append(subs, sub)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return subs, nil
}

func (r *sqlitePushSubscriptionRepository) CountByUser(ctx context.Context) (int, error) {
	userID := contexthelpers.AuthenticatedUserID(ctx)
	var count int
	err := r.db.ReadOnly.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM push_subscriptions WHERE user_id = ?`, userID,
	).Scan(&count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("count push subscriptions: %w", err)
	}
	return count, nil
}
```

- [ ] **Step 6: Verify the test passes**

Run:

```bash
go test ./internal/repository -run TestPushSubscriptions
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/push_subscription.go internal/repository/push_subscription.go internal/repository/push_subscription_test.go internal/repository/repository.go
git commit -m "Add PushSubscription domain type and repository"
```

---

### Task 5: `ScheduledPushRepository`

**Files:**
- Create: `internal/repository/scheduled_push.go`
- Test: `internal/repository/scheduled_push_test.go`
- Modify: `internal/repository/repository.go`
- Create: `internal/domain/scheduled_push.go`

- [ ] **Step 1: Add the domain type**

Create `internal/domain/scheduled_push.go`:

```go
package domain

import "time"

// ScheduledPush is a pending Web Push notification persisted so it can be
// replayed after a process restart.
type ScheduledPush struct {
	ID                int
	UserID            int
	WorkoutExerciseID int
	FireAt            time.Time
	Payload           string
	CreatedAt         time.Time
}
```

- [ ] **Step 2: Add the interface**

Edit `internal/repository/repository.go`. Add interface declaration:

```go
// ScheduledPushRepository persists pending push fires so they survive
// process restarts. One row per workout_exercise_id (enforced by UNIQUE
// index).
type ScheduledPushRepository interface {
	Replace(ctx context.Context, push domain.ScheduledPush) (domain.ScheduledPush, error)
	Delete(ctx context.Context, id int) error
	DeleteByWorkoutExercise(ctx context.Context, workoutExerciseID int) error
	DeleteByWorkoutSession(ctx context.Context, userID int, date time.Time) error
	Get(ctx context.Context, workoutExerciseID int) (domain.ScheduledPush, error)
	ListAll(ctx context.Context) ([]domain.ScheduledPush, error)
}
```

Add the field to `Repositories`:

```go
type Repositories struct {
	Sessions          SessionRepository
	Exercises         ExerciseRepository
	Preferences       PreferencesRepository
	FeatureFlags      FeatureFlagRepository
	MuscleTargets     MuscleGroupTargetRepository
	PushSubscriptions PushSubscriptionRepository
	ScheduledPushes   ScheduledPushRepository
}
```

Wire in `New`:

```go
scheduledPushes := newSQLiteScheduledPushRepository(db)
return &Repositories{
    ...
    ScheduledPushes: scheduledPushes,
}
```

- [ ] **Step 3: Write the failing test**

Create `internal/repository/scheduled_push_test.go`:

```go
package repository_test

import (
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
)

func TestScheduledPushes_ReplaceUpsertsByWorkoutExercise(t *testing.T) {
	t.Parallel()
	ctx, repos := setupTestRepos(t)

	userID := contexthelpers.AuthenticatedUserID(ctx)
	weID := seedWorkoutExercise(t, ctx, repos)

	fireAt1 := time.Now().Add(90 * time.Second).UTC().Truncate(time.Millisecond)
	first := domain.ScheduledPush{ //nolint:exhaustruct
		UserID:            userID,
		WorkoutExerciseID: weID,
		FireAt:            fireAt1,
		Payload:           `{"title":"Rest over","body":"Set 1"}`,
	}
	got, err := repos.ScheduledPushes.Replace(ctx, first)
	if err != nil {
		t.Fatalf("first Replace: %v", err)
	}
	if got.ID == 0 {
		t.Error("Replace returned ID 0")
	}

	// Replace with new time + payload — should overwrite, not duplicate.
	fireAt2 := time.Now().Add(150 * time.Second).UTC().Truncate(time.Millisecond)
	second := first
	second.FireAt = fireAt2
	second.Payload = `{"title":"Rest over","body":"Set 2"}`
	got2, err := repos.ScheduledPushes.Replace(ctx, second)
	if err != nil {
		t.Fatalf("second Replace: %v", err)
	}

	all, err := repos.ScheduledPushes.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListAll returned %d rows, want 1", len(all))
	}
	if !all[0].FireAt.Equal(fireAt2) {
		t.Errorf("FireAt = %v, want %v", all[0].FireAt, fireAt2)
	}
	if all[0].Payload != second.Payload {
		t.Errorf("Payload = %q, want %q", all[0].Payload, second.Payload)
	}
	_ = got2
}
```

Add a `seedWorkoutExercise` helper to `internal/repository/helpers_test.go`. Inspect that file first — if a similar helper already exists, reuse it. Otherwise add:

```go
// seedWorkoutExercise inserts a workout_session and workout_exercise row for the
// authenticated user and returns the workout_exercise.id.
func seedWorkoutExercise(t *testing.T, ctx context.Context, repos *repository.Repositories) int {
	t.Helper()
	userID := contexthelpers.AuthenticatedUserID(ctx)
	db := testDB(t) // existing helper — confirm name when implementing
	today := time.Now().Format("2006-01-02")
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date) VALUES (?, ?)
		 ON CONFLICT DO NOTHING`,
		userID, today,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var exerciseID int
	if err := db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Deadlift'`,
	).Scan(&exerciseID); err != nil {
		t.Fatalf("fetch deadlift: %v", err)
	}
	var weID int
	if err := db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		 VALUES (?, ?, ?) RETURNING id`,
		userID, today, exerciseID,
	).Scan(&weID); err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	return weID
}
```

If `testDB(t)` isn't the right accessor in this codebase, read `helpers_test.go` to find how existing tests reach the `*sqlite.Database` and adapt.

- [ ] **Step 4: Run the test to verify it fails**

Run:

```bash
go test ./internal/repository -run TestScheduledPushes
```

Expected: FAIL — constructor undefined.

- [ ] **Step 5: Implement the SQLite repository**

Create `internal/repository/scheduled_push.go`:

```go
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type sqliteScheduledPushRepository struct {
	baseRepository
}

func newSQLiteScheduledPushRepository(db *sqlite.Database) *sqliteScheduledPushRepository {
	return &sqliteScheduledPushRepository{baseRepository: newBaseRepository(db)}
}

// Replace upserts the row for the given workout_exercise_id. The UNIQUE index
// on workout_exercise_id enforces the one-pending-push-per-slot invariant.
func (r *sqliteScheduledPushRepository) Replace(
	ctx context.Context, push domain.ScheduledPush,
) (domain.ScheduledPush, error) {
	var createdAt string
	err := r.db.ReadWrite.QueryRowContext(ctx, `
		INSERT INTO scheduled_pushes (user_id, workout_exercise_id, fire_at, payload)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (workout_exercise_id) DO UPDATE SET
		    user_id = excluded.user_id,
		    fire_at = excluded.fire_at,
		    payload = excluded.payload
		RETURNING id, created_at`,
		push.UserID, push.WorkoutExerciseID, formatTimestamp(push.FireAt), push.Payload,
	).Scan(&push.ID, &createdAt)
	if err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("upsert scheduled push: %w", err)
	}
	if push.CreatedAt, err = parseTimestamp(createdAt); err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("parse created_at: %w", err)
	}
	return push, nil
}

func (r *sqliteScheduledPushRepository) Delete(ctx context.Context, id int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		`DELETE FROM scheduled_pushes WHERE id = ?`, id,
	); err != nil {
		return fmt.Errorf("delete scheduled push: %w", err)
	}
	return nil
}

func (r *sqliteScheduledPushRepository) DeleteByWorkoutExercise(ctx context.Context, workoutExerciseID int) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx,
		`DELETE FROM scheduled_pushes WHERE workout_exercise_id = ?`, workoutExerciseID,
	); err != nil {
		return fmt.Errorf("delete scheduled push by workout_exercise: %w", err)
	}
	return nil
}

func (r *sqliteScheduledPushRepository) DeleteByWorkoutSession(
	ctx context.Context, userID int, date time.Time,
) error {
	if _, err := r.db.ReadWrite.ExecContext(ctx, `
		DELETE FROM scheduled_pushes
		WHERE workout_exercise_id IN (
		    SELECT id FROM workout_exercise
		    WHERE workout_user_id = ? AND workout_date = ?
		)`, userID, formatDate(date),
	); err != nil {
		return fmt.Errorf("delete scheduled pushes by session: %w", err)
	}
	return nil
}

func (r *sqliteScheduledPushRepository) Get(
	ctx context.Context, workoutExerciseID int,
) (domain.ScheduledPush, error) {
	var (
		push      domain.ScheduledPush
		fireAt    string
		createdAt string
	)
	err := r.db.ReadOnly.QueryRowContext(ctx, `
		SELECT id, user_id, workout_exercise_id, fire_at, payload, created_at
		FROM scheduled_pushes
		WHERE workout_exercise_id = ?`, workoutExerciseID,
	).Scan(&push.ID, &push.UserID, &push.WorkoutExerciseID, &fireAt, &push.Payload, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ScheduledPush{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("query scheduled push: %w", err)
	}
	if push.FireAt, err = parseTimestamp(fireAt); err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("parse fire_at: %w", err)
	}
	if push.CreatedAt, err = parseTimestamp(createdAt); err != nil {
		return domain.ScheduledPush{}, fmt.Errorf("parse created_at: %w", err)
	}
	return push, nil
}

func (r *sqliteScheduledPushRepository) ListAll(ctx context.Context) (_ []domain.ScheduledPush, err error) {
	rows, err := r.db.ReadOnly.QueryContext(ctx, `
		SELECT id, user_id, workout_exercise_id, fire_at, payload, created_at
		FROM scheduled_pushes
		ORDER BY fire_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("query scheduled pushes: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	var pushes []domain.ScheduledPush
	for rows.Next() {
		var (
			push      domain.ScheduledPush
			fireAt    string
			createdAt string
		)
		if err = rows.Scan(
			&push.ID, &push.UserID, &push.WorkoutExerciseID, &fireAt, &push.Payload, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan scheduled push: %w", err)
		}
		if push.FireAt, err = parseTimestamp(fireAt); err != nil {
			return nil, fmt.Errorf("parse fire_at: %w", err)
		}
		if push.CreatedAt, err = parseTimestamp(createdAt); err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
		pushes = append(pushes, push)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return pushes, nil
}
```

- [ ] **Step 6: Verify the test passes**

Run:

```bash
go test ./internal/repository -run TestScheduledPushes
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/scheduled_push.go internal/repository/scheduled_push.go internal/repository/scheduled_push_test.go internal/repository/repository.go internal/repository/helpers_test.go
git commit -m "Add ScheduledPush domain type and repository"
```

---

### Task 6: `notification.Sender`

**Files:**
- Create: `internal/notification/sender.go`
- Test: `internal/notification/sender_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/notification/sender_test.go`:

```go
package notification_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/notification"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// mintVAPIDKeys returns a fresh base64url-encoded VAPID keypair using the
// webpush-go test helper.
func mintVAPIDKeys(t *testing.T) (priv, pub string) {
	t.Helper()
	// webpush-go exposes GenerateVAPIDKeys; import alias kept local to test.
	var err error
	priv, pub, err = webpushGenerateKeys()
	if err != nil {
		t.Fatalf("generate vapid keys: %v", err)
	}
	return priv, pub
}

func TestSender_Send_SubjectIsBareEmail(t *testing.T) {
	t.Parallel()
	priv, pub := mintVAPIDKeys(t)

	var capturedAuthHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)

	sender := notification.NewSender(notification.SenderConfig{
		VAPIDSubject:    "vapid@example.com",
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Logger:          testhelpers.NewLogger(testhelpers.NewWriter(t)),
	})

	sub := domain.PushSubscription{ //nolint:exhaustruct
		Endpoint: srv.URL + "/wp/abc",
		P256dh:   testValidP256dh(),
		Auth:     testValidAuth(),
	}
	err := sender.Send(context.Background(), sub, []byte(`{"title":"x"}`))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// The Authorization header contains a JWT under "vapid t=…, k=…".
	// Decode the JWT body and assert the "sub" claim is the bare email,
	// not "mailto:mailto:…".
	subClaim := extractJWTSubClaim(t, capturedAuthHeader)
	if subClaim != "mailto:vapid@example.com" {
		t.Errorf("sub claim = %q, want exactly one mailto: prefix on the bare email", subClaim)
	}
	if strings.Contains(subClaim, "mailto:mailto:") {
		t.Errorf("sub claim has double mailto: prefix: %q", subClaim)
	}
}

func TestSender_Send_410ReturnsErrSubscriptionGone(t *testing.T) {
	t.Parallel()
	priv, pub := mintVAPIDKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	t.Cleanup(srv.Close)

	sender := notification.NewSender(notification.SenderConfig{
		VAPIDSubject:    "vapid@example.com",
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Logger:          testhelpers.NewLogger(testhelpers.NewWriter(t)),
	})
	sub := domain.PushSubscription{ //nolint:exhaustruct
		Endpoint: srv.URL,
		P256dh:   testValidP256dh(),
		Auth:     testValidAuth(),
	}

	err := sender.Send(context.Background(), sub, []byte(`{"title":"x"}`))
	if !errors.Is(err, notification.ErrSubscriptionGone) {
		t.Errorf("Send returned %v, want ErrSubscriptionGone", err)
	}
}

func TestSender_Send_5xxReturnsErrorButNotGone(t *testing.T) {
	t.Parallel()
	priv, pub := mintVAPIDKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	sender := notification.NewSender(notification.SenderConfig{
		VAPIDSubject:    "vapid@example.com",
		VAPIDPublicKey:  pub,
		VAPIDPrivateKey: priv,
		Logger:          testhelpers.NewLogger(testhelpers.NewWriter(t)),
	})
	sub := domain.PushSubscription{ //nolint:exhaustruct
		Endpoint: srv.URL,
		P256dh:   testValidP256dh(),
		Auth:     testValidAuth(),
	}

	err := sender.Send(context.Background(), sub, []byte(`{"title":"x"}`))
	if err == nil {
		t.Fatal("Send returned nil for 5xx, want error")
	}
	if errors.Is(err, notification.ErrSubscriptionGone) {
		t.Errorf("Send returned ErrSubscriptionGone for 5xx; should be a transient error")
	}
}

// testValidP256dh and testValidAuth return base64url-encoded values that
// satisfy webpush-go's input validation. The exact key material does not
// matter — the httptest.Server in these tests does not verify the encryption.
func testValidP256dh() string {
	// 65-byte uncompressed P-256 public point (0x04 || X || Y).
	raw := make([]byte, 65)
	raw[0] = 0x04
	return base64.RawURLEncoding.EncodeToString(raw)
}

func testValidAuth() string {
	return base64.RawURLEncoding.EncodeToString(make([]byte, 16))
}
```

Add a small helper file `internal/notification/test_helpers_test.go` for the JWT extractor and the key-gen indirection (so the test file imports stay tidy):

```go
package notification_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
)

func webpushGenerateKeys() (priv, pub string, err error) {
	return webpush.GenerateVAPIDKeys()
}

// extractJWTSubClaim parses the "Authorization: vapid t=<jwt>, k=<base64>" header
// and returns the JWT's "sub" claim. Fails the test on any parse error.
func extractJWTSubClaim(t *testing.T, authHeader string) string {
	t.Helper()
	const prefix = "vapid t="
	if !strings.HasPrefix(authHeader, prefix) {
		t.Fatalf("auth header %q missing %q prefix", authHeader, prefix)
	}
	rest := strings.TrimPrefix(authHeader, prefix)
	jwt := rest
	if idx := strings.Index(rest, ","); idx >= 0 {
		jwt = strings.TrimSpace(rest[:idx])
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed JWT (%d parts): %q", len(parts), jwt)
	}
	bodyBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT body: %v", err)
	}
	var claims struct {
		Sub string `json:"sub"`
	}
	if err = json.Unmarshal(bodyBytes, &claims); err != nil {
		t.Fatalf("unmarshal JWT body: %v", err)
	}
	return claims.Sub
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/notification
```

Expected: FAIL — package doesn't exist yet (or `NewSender` undefined).

- [ ] **Step 3: Implement the Sender**

Create `internal/notification/sender.go`:

```go
// Package notification owns Web Push delivery: VAPID-signed sends, in-process
// scheduling persisted in SQLite, and the application-level idle monitor that
// allows Fly to scale the Machine to zero between workouts.
package notification

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/myrjola/petrapp/internal/domain"
)

// ErrSubscriptionGone signals that a push endpoint returned 404 or 410. The
// caller should drop the subscription from its store; no retry is appropriate.
var ErrSubscriptionGone = errors.New("notification: subscription gone")

// SenderConfig configures a Sender. VAPIDSubject must be a bare email — the
// webpush-go library v1.4.0 prepends "mailto:" unconditionally, so passing
// "mailto:foo@bar" produces "mailto:mailto:foo@bar" and Apple returns
// BadJwtToken. The test pins this invariant.
type SenderConfig struct {
	VAPIDSubject    string
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	Logger          *slog.Logger
	HTTPClient      *http.Client // optional; defaults to http.DefaultClient when nil.
}

// Sender wraps webpush.SendNotification with the project's VAPID keys and the
// 410/404 → ErrSubscriptionGone translation. Goroutine-safe.
type Sender struct {
	cfg SenderConfig
}

// NewSender constructs a Sender. The config is held by value; later mutations
// to the passed-in struct are not reflected.
func NewSender(cfg SenderConfig) *Sender {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	return &Sender{cfg: cfg}
}

// Send delivers payload to one subscription. The payload is expected to be a
// short JSON object the service worker can parse. Returns ErrSubscriptionGone
// for permanent (404/410) failures so the caller can prune the subscription;
// transient errors are wrapped and returned as-is.
func (s *Sender) Send(ctx context.Context, sub domain.PushSubscription, payload []byte) error {
	subscription := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}

	resp, err := webpush.SendNotificationWithContext(ctx, payload, subscription, &webpush.Options{
		HTTPClient:      s.cfg.HTTPClient,
		Subscriber:      s.cfg.VAPIDSubject, // BARE EMAIL — webpush-go prepends mailto: itself.
		VAPIDPublicKey:  s.cfg.VAPIDPublicKey,
		VAPIDPrivateKey: s.cfg.VAPIDPrivateKey,
		TTL:             60, //nolint:mnd // 60 seconds — rest pushes are useless after the next set.
		Topic:           "",
		Urgency:         webpush.UrgencyHigh,
	})
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer func() {
		// Drain body so the underlying connection is reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
		return ErrSubscriptionGone
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024)) //nolint:mnd // 1 KiB cap.
		return fmt.Errorf("push delivery failed: status %d, body: %s",
			resp.StatusCode, bytes.TrimSpace(body))
	}
	return nil
}
```

- [ ] **Step 4: Verify the test passes**

Run:

```bash
go test ./internal/notification
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notification/sender.go internal/notification/sender_test.go internal/notification/test_helpers_test.go
git commit -m "Add notification.Sender with bare-email VAPID subject"
```

---

### Task 7: `notification.Scheduler`

**Files:**
- Create: `internal/notification/scheduler.go`
- Test: `internal/notification/scheduler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/notification/scheduler_test.go`:

```go
package notification_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/notification"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

// fakeDispatch implements notification.Dispatcher (or the matching
// callback signature — adapt to the final API). It records every fire.
type fakeDispatch struct {
	mu     sync.Mutex
	fires  []int // workoutExerciseIDs in fire order
	done   chan struct{}
	target int
}

func newFakeDispatch(target int) *fakeDispatch {
	return &fakeDispatch{done: make(chan struct{}), target: target}
}

func (f *fakeDispatch) Dispatch(_ context.Context, push domain.ScheduledPush) error {
	f.mu.Lock()
	f.fires = append(f.fires, push.WorkoutExerciseID)
	if len(f.fires) == f.target {
		close(f.done)
	}
	f.mu.Unlock()
	return nil
}

func (f *fakeDispatch) fired() []int {
	f.mu.Lock()
	out := append([]int(nil), f.fires...)
	f.mu.Unlock()
	return out
}

func TestScheduler_ScheduleFires(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(1)
	repo := newInMemoryScheduledPushRepo()
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:       repo,
		Dispatch:   fd.Dispatch,
		Logger:     testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now:        time.Now,
	})

	push := domain.ScheduledPush{ //nolint:exhaustruct
		UserID:            1,
		WorkoutExerciseID: 42,
		FireAt:            time.Now().Add(50 * time.Millisecond),
		Payload:           `{}`,
	}
	if err := scheduler.Schedule(context.Background(), push); err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if scheduler.PendingCount() != 1 {
		t.Fatalf("PendingCount = %d, want 1", scheduler.PendingCount())
	}

	select {
	case <-fd.done:
	case <-time.After(time.Second):
		t.Fatalf("timer never fired")
	}
	if got := fd.fired(); len(got) != 1 || got[0] != 42 {
		t.Errorf("fires = %v, want [42]", got)
	}
	if scheduler.PendingCount() != 0 {
		t.Errorf("after fire: PendingCount = %d, want 0", scheduler.PendingCount())
	}
}

func TestScheduler_CancelStopsTimer(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(0)
	repo := newInMemoryScheduledPushRepo()
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     repo,
		Dispatch: fd.Dispatch,
		Logger:   testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now:      time.Now,
	})

	push := domain.ScheduledPush{ //nolint:exhaustruct
		UserID: 1, WorkoutExerciseID: 99,
		FireAt: time.Now().Add(100 * time.Millisecond),
	}
	_ = scheduler.Schedule(context.Background(), push)
	if err := scheduler.Cancel(context.Background(), 99); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if got := fd.fired(); len(got) != 0 {
		t.Errorf("got fires after Cancel: %v", got)
	}
	if scheduler.PendingCount() != 0 {
		t.Errorf("PendingCount = %d, want 0", scheduler.PendingCount())
	}
}

func TestScheduler_ReplaceOnlyFiresLatest(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(1)
	repo := newInMemoryScheduledPushRepo()
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     repo,
		Dispatch: fd.Dispatch,
		Logger:   testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now:      time.Now,
	})

	// Schedule far-future, then immediately reschedule near-future.
	farFuture := domain.ScheduledPush{ //nolint:exhaustruct
		UserID: 1, WorkoutExerciseID: 7,
		FireAt:  time.Now().Add(5 * time.Second),
		Payload: `"original"`,
	}
	nearFuture := domain.ScheduledPush{ //nolint:exhaustruct
		UserID: 1, WorkoutExerciseID: 7,
		FireAt:  time.Now().Add(50 * time.Millisecond),
		Payload: `"replacement"`,
	}
	_ = scheduler.Schedule(context.Background(), farFuture)
	_ = scheduler.Schedule(context.Background(), nearFuture)

	select {
	case <-fd.done:
	case <-time.After(time.Second):
		t.Fatalf("replacement never fired within window")
	}
	if got := fd.fired(); len(got) != 1 {
		t.Errorf("got %d fires, want exactly 1 (replacement only): %v", len(got), got)
	}
}

func TestScheduler_ReloadReconstitutesFutureTimers(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(1)
	repo := newInMemoryScheduledPushRepo()
	// Pre-seed the repo with a future fire.
	_, _ = repo.Replace(context.Background(), domain.ScheduledPush{ //nolint:exhaustruct
		UserID: 1, WorkoutExerciseID: 11,
		FireAt:  time.Now().Add(50 * time.Millisecond),
		Payload: "{}",
	})
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     repo,
		Dispatch: fd.Dispatch,
		Logger:   testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now:      time.Now,
	})

	if err := scheduler.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if scheduler.PendingCount() != 1 {
		t.Fatalf("PendingCount after Reload = %d, want 1", scheduler.PendingCount())
	}
	select {
	case <-fd.done:
	case <-time.After(time.Second):
		t.Fatalf("reloaded timer never fired")
	}
}

func TestScheduler_ReloadFiresPastDueImmediately(t *testing.T) {
	t.Parallel()
	fd := newFakeDispatch(1)
	repo := newInMemoryScheduledPushRepo()
	_, _ = repo.Replace(context.Background(), domain.ScheduledPush{ //nolint:exhaustruct
		UserID: 1, WorkoutExerciseID: 22,
		FireAt:  time.Now().Add(-time.Minute),
		Payload: "{}",
	})
	var fakeNow atomic.Value
	fakeNow.Store(time.Now())
	scheduler := notification.NewScheduler(notification.SchedulerConfig{
		Repo:     repo,
		Dispatch: fd.Dispatch,
		Logger:   testhelpers.NewLogger(testhelpers.NewWriter(t)),
		Now:      func() time.Time { return fakeNow.Load().(time.Time) },
	})

	if err := scheduler.Reload(context.Background()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	select {
	case <-fd.done:
	case <-time.After(time.Second):
		t.Fatalf("past-due fire never dispatched on Reload")
	}
}

// newInMemoryScheduledPushRepo returns an in-memory implementation of the
// repository.ScheduledPushRepository interface for the scheduler tests.
func newInMemoryScheduledPushRepo() *inMemScheduledPushRepo {
	return &inMemScheduledPushRepo{rows: map[int]domain.ScheduledPush{}}
}

type inMemScheduledPushRepo struct {
	mu     sync.Mutex
	rows   map[int]domain.ScheduledPush // keyed by workout_exercise_id
	nextID int
}

func (m *inMemScheduledPushRepo) Replace(_ context.Context, push domain.ScheduledPush) (domain.ScheduledPush, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	push.ID = m.nextID
	push.CreatedAt = time.Now()
	m.rows[push.WorkoutExerciseID] = push
	return push, nil
}

func (m *inMemScheduledPushRepo) Delete(_ context.Context, id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.rows {
		if v.ID == id {
			delete(m.rows, k)
			return nil
		}
	}
	return nil
}

func (m *inMemScheduledPushRepo) DeleteByWorkoutExercise(_ context.Context, weID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rows, weID)
	return nil
}

func (m *inMemScheduledPushRepo) DeleteByWorkoutSession(_ context.Context, _ int, _ time.Time) error {
	return errors.New("not implemented in test")
}

func (m *inMemScheduledPushRepo) Get(_ context.Context, weID int) (domain.ScheduledPush, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.rows[weID]
	if !ok {
		return domain.ScheduledPush{}, domain.ErrNotFound
	}
	return v, nil
}

func (m *inMemScheduledPushRepo) ListAll(_ context.Context) ([]domain.ScheduledPush, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.ScheduledPush, 0, len(m.rows))
	for _, v := range m.rows {
		out = append(out, v)
	}
	return out, nil
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/notification -run TestScheduler
```

Expected: FAIL — `notification.NewScheduler` undefined.

- [ ] **Step 3: Implement the Scheduler**

Create `internal/notification/scheduler.go`:

```go
package notification

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/myrjola/petrapp/internal/domain"
)

// DispatchFunc is called when a scheduled push fires. Implementations should
// load subscriptions, send the payload, and prune 410'd subscriptions. The
// scheduler invokes Dispatch from a fresh goroutine; implementations are
// responsible for their own context propagation.
type DispatchFunc func(ctx context.Context, push domain.ScheduledPush) error

// ScheduledPushRepo is the subset of repository.ScheduledPushRepository the
// scheduler needs. Declared locally so the package doesn't import the whole
// repository package — keeps notification at a lower layer in the dep graph.
type ScheduledPushRepo interface {
	Replace(ctx context.Context, push domain.ScheduledPush) (domain.ScheduledPush, error)
	Delete(ctx context.Context, id int) error
	DeleteByWorkoutExercise(ctx context.Context, workoutExerciseID int) error
	DeleteByWorkoutSession(ctx context.Context, userID int, date time.Time) error
	ListAll(ctx context.Context) ([]domain.ScheduledPush, error)
}

// SchedulerConfig configures a Scheduler.
type SchedulerConfig struct {
	Repo     ScheduledPushRepo
	Dispatch DispatchFunc
	Logger   *slog.Logger
	Now      func() time.Time // injectable for tests; defaults to time.Now when nil.
}

// Scheduler holds an in-process map of workout_exercise_id → *time.Timer,
// persisted to SQLite so pending pushes survive restarts. Goroutine-safe.
type Scheduler struct {
	cfg    SchedulerConfig
	mu     sync.Mutex
	timers map[int]*time.Timer // keyed by workout_exercise_id
}

// NewScheduler constructs a Scheduler. Call Reload once at process start.
func NewScheduler(cfg SchedulerConfig) *Scheduler {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Scheduler{
		cfg:    cfg,
		timers: map[int]*time.Timer{},
	}
}

// Schedule persists the push and starts an in-process timer. If a timer for
// the same workout_exercise_id already exists, it is stopped and replaced
// (the repo's UNIQUE index handles the row-level replace).
func (s *Scheduler) Schedule(ctx context.Context, push domain.ScheduledPush) error {
	stored, err := s.cfg.Repo.Replace(ctx, push)
	if err != nil {
		return fmt.Errorf("persist scheduled push: %w", err)
	}
	s.startTimer(stored)
	return nil
}

// Cancel stops the in-process timer for the given workout_exercise_id and
// deletes its row from the repo. No-op if neither exists.
func (s *Scheduler) Cancel(ctx context.Context, workoutExerciseID int) error {
	s.mu.Lock()
	if t, ok := s.timers[workoutExerciseID]; ok {
		t.Stop()
		delete(s.timers, workoutExerciseID)
	}
	s.mu.Unlock()
	if err := s.cfg.Repo.DeleteByWorkoutExercise(ctx, workoutExerciseID); err != nil {
		return fmt.Errorf("delete scheduled push row: %w", err)
	}
	return nil
}

// CancelForWorkout stops every in-process timer whose row belongs to the
// given workout session and deletes all matching rows. Used by
// CompleteSession.
func (s *Scheduler) CancelForWorkout(ctx context.Context, userID int, date time.Time) error {
	// Find which workout_exercise_ids belong to this session by reading the
	// repo's view of pending rows — the repo's DeleteByWorkoutSession also
	// joins on workout_exercise. We pre-list so we can stop in-process timers.
	all, err := s.cfg.Repo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list scheduled pushes for cancel: %w", err)
	}
	s.mu.Lock()
	for _, p := range all {
		if p.UserID != userID {
			continue
		}
		// We don't have the date on the push row; the repo's
		// DeleteByWorkoutSession will filter authoritatively. Stop every
		// timer for this user — false positives just re-Schedule next set.
		// To keep this precise without an extra query, the repo could
		// return joined rows; left as-is for simplicity.
		if t, ok := s.timers[p.WorkoutExerciseID]; ok {
			t.Stop()
			delete(s.timers, p.WorkoutExerciseID)
		}
	}
	s.mu.Unlock()
	if err = s.cfg.Repo.DeleteByWorkoutSession(ctx, userID, date); err != nil {
		return fmt.Errorf("delete scheduled pushes for workout: %w", err)
	}
	return nil
}

// Reload rebuilds the in-process timer map from persistent state. Past-due
// rows fire immediately on a fresh goroutine; future rows get a timer for
// the remaining delta. Call once at process start.
func (s *Scheduler) Reload(ctx context.Context) error {
	pending, err := s.cfg.Repo.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("list pending pushes: %w", err)
	}
	for _, p := range pending {
		s.startTimer(p)
	}
	return nil
}

// PendingCount returns the number of in-process timers. Used by IdleMonitor
// to gate process exit on a clean (no pushes pending) state.
func (s *Scheduler) PendingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.timers)
}

func (s *Scheduler) startTimer(push domain.ScheduledPush) {
	delay := push.FireAt.Sub(s.cfg.Now())
	if delay < 0 {
		delay = 0
	}
	weID := push.WorkoutExerciseID
	timer := time.AfterFunc(delay, func() {
		s.fire(push)
	})
	s.mu.Lock()
	if existing, ok := s.timers[weID]; ok {
		existing.Stop()
	}
	s.timers[weID] = timer
	s.mu.Unlock()
}

func (s *Scheduler) fire(push domain.ScheduledPush) {
	s.mu.Lock()
	delete(s.timers, push.WorkoutExerciseID)
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) //nolint:mnd
	defer cancel()

	if err := s.cfg.Dispatch(ctx, push); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "push dispatch failed",
			slog.Int("workout_exercise_id", push.WorkoutExerciseID),
			slog.Any("error", err))
	}
	if err := s.cfg.Repo.Delete(ctx, push.ID); err != nil {
		s.cfg.Logger.LogAttrs(ctx, slog.LevelWarn, "delete scheduled push row after fire",
			slog.Int("id", push.ID),
			slog.Any("error", err))
	}
}
```

- [ ] **Step 4: Verify the tests pass**

Run:

```bash
go test ./internal/notification -run TestScheduler -race
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notification/scheduler.go internal/notification/scheduler_test.go
git commit -m "Add notification.Scheduler with persistent timer reload"
```

---

### Task 8: `notification.IdleMonitor`

**Files:**
- Create: `internal/notification/idle_monitor.go`
- Test: `internal/notification/idle_monitor_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/notification/idle_monitor_test.go`:

```go
package notification_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/myrjola/petrapp/internal/notification"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func TestIdleMonitor_TriggersWhenIdleAndNoPending(t *testing.T) {
	t.Parallel()

	var fakeNow atomic.Int64
	fakeNow.Store(time.Now().UnixNano())
	var lastRequest atomic.Int64
	lastRequest.Store(time.Now().UnixNano())
	var pending atomic.Int32
	var triggered atomic.Bool

	mon := notification.NewIdleMonitor(notification.IdleMonitorConfig{
		IdleThreshold: 50 * time.Millisecond,
		TickInterval:  10 * time.Millisecond,
		Now:           func() time.Time { return time.Unix(0, fakeNow.Load()) },
		LastRequestAt: func() time.Time { return time.Unix(0, lastRequest.Load()) },
		PendingCount:  func() int { return int(pending.Load()) },
		Trigger:       func() { triggered.Store(true) },
		Logger:        testhelpers.NewLogger(testhelpers.NewWriter(t)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go mon.Run(ctx)

	// Advance simulated time past the idle threshold while keeping pending=0.
	fakeNow.Store(time.Now().Add(time.Second).UnixNano())

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !triggered.Load() {
		time.Sleep(10 * time.Millisecond)
	}
	if !triggered.Load() {
		t.Fatal("idle monitor never triggered")
	}
}

func TestIdleMonitor_BlockedByPending(t *testing.T) {
	t.Parallel()

	var fakeNow atomic.Int64
	fakeNow.Store(time.Now().UnixNano())
	var lastRequest atomic.Int64
	lastRequest.Store(time.Now().UnixNano())
	var pending atomic.Int32
	pending.Store(1)
	var triggered atomic.Bool

	mon := notification.NewIdleMonitor(notification.IdleMonitorConfig{
		IdleThreshold: 20 * time.Millisecond,
		TickInterval:  5 * time.Millisecond,
		Now:           func() time.Time { return time.Unix(0, fakeNow.Load()) },
		LastRequestAt: func() time.Time { return time.Unix(0, lastRequest.Load()) },
		PendingCount:  func() int { return int(pending.Load()) },
		Trigger:       func() { triggered.Store(true) },
		Logger:        testhelpers.NewLogger(testhelpers.NewWriter(t)),
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go mon.Run(ctx)

	fakeNow.Store(time.Now().Add(time.Second).UnixNano())
	time.Sleep(100 * time.Millisecond)
	if triggered.Load() {
		t.Error("idle monitor triggered despite pending > 0")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/notification -run TestIdleMonitor
```

Expected: FAIL — `notification.NewIdleMonitor` undefined.

- [ ] **Step 3: Implement the IdleMonitor**

Create `internal/notification/idle_monitor.go`:

```go
package notification

import (
	"context"
	"log/slog"
	"time"
)

// IdleMonitorConfig configures an IdleMonitor.
type IdleMonitorConfig struct {
	IdleThreshold time.Duration
	TickInterval  time.Duration
	Now           func() time.Time
	LastRequestAt func() time.Time
	PendingCount  func() int
	// Trigger fires when idle + no pending. In production this sends
	// SIGTERM to the process; tests substitute a flag-set.
	Trigger func()
	Logger  *slog.Logger
}

// IdleMonitor watches request inactivity and a pending-push count, and fires
// Trigger when both conditions are met. Used by main.go to scale the Fly
// Machine to zero between workouts.
type IdleMonitor struct {
	cfg IdleMonitorConfig
}

// NewIdleMonitor constructs an IdleMonitor.
func NewIdleMonitor(cfg IdleMonitorConfig) *IdleMonitor {
	return &IdleMonitor{cfg: cfg}
}

// Run blocks until ctx is cancelled. Fires Trigger at most once per Run.
func (m *IdleMonitor) Run(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.shouldTrigger() {
				m.cfg.Logger.LogAttrs(ctx, slog.LevelInfo, "idle monitor triggering shutdown")
				m.cfg.Trigger()
				return
			}
		}
	}
}

func (m *IdleMonitor) shouldTrigger() bool {
	idleFor := m.cfg.Now().Sub(m.cfg.LastRequestAt())
	if idleFor < m.cfg.IdleThreshold {
		return false
	}
	if m.cfg.PendingCount() > 0 {
		return false
	}
	return true
}
```

- [ ] **Step 4: Verify the tests pass**

Run:

```bash
go test ./internal/notification -run TestIdleMonitor -race
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/notification/idle_monitor.go internal/notification/idle_monitor_test.go
git commit -m "Add notification.IdleMonitor for Fly auto-stop replacement"
```

---

### Task 9: Service integration — schedule on `RecordSet`, cancel on `CompleteSession` and `SwapExercise`

**Files:**
- Modify: `internal/service/service.go`
- Modify: `internal/service/sets.go`
- Modify: `internal/service/sessions.go`
- Modify: `internal/service/exercises.go`
- Test: `internal/service/sets_test.go`
- Test: `internal/service/sessions_test.go`

- [ ] **Step 1: Add a Scheduler dependency to `Service`**

Edit `internal/service/service.go`:

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/notification"
	"github.com/myrjola/petrapp/internal/repository"
	"github.com/myrjola/petrapp/internal/sqlite"
)

type Service struct {
	repos        *repository.Repositories
	db           *sqlite.Database
	logger       *slog.Logger
	openaiAPIKey string
	scheduler    PushScheduler // nil-safe; methods no-op when nil.
}

// PushScheduler is the subset of notification.Scheduler the service depends on.
// Declared as an interface so tests can substitute a fake or pass nil.
type PushScheduler interface {
	Schedule(ctx context.Context, push domain.ScheduledPush) error
	Cancel(ctx context.Context, workoutExerciseID int) error
	CancelForWorkout(ctx context.Context, userID int, date time.Time) error
}

// NewService creates a workout service.
func NewService(
	db *sqlite.Database,
	logger *slog.Logger,
	openaiAPIKey string,
) *Service {
	return &Service{
		repos:        repository.New(db, logger),
		db:           db,
		logger:       logger,
		openaiAPIKey: openaiAPIKey,
		scheduler:    nil,
	}
}

// WithScheduler returns a copy of the service wired to a push scheduler.
// Called from main.go after the notification package is initialised. Tests
// that need scheduling behaviour call this with a fake; tests that don't
// leave it nil.
func (s *Service) WithScheduler(scheduler PushScheduler) *Service {
	cp := *s
	cp.scheduler = scheduler
	return &cp
}
```

Add the `time` import. Note: this adds a method `WithScheduler` rather than threading the scheduler through `NewService` so existing tests don't need updating.

- [ ] **Step 2: Extend `RecordSet` to schedule a push on first-time completion**

Edit `internal/service/sets.go`. Replace the `RecordSet` method:

```go
func (s *Service) RecordSet(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	setIndex int,
	signal domain.Signal,
	weightKg *float64,
	completedValue int,
) error {
	var (
		wasComplete  bool
		exercise     domain.Exercise
		periodization domain.PeriodizationType
		setNumber    int
		setsTotal    int
		hasMoreAfter bool
	)
	now := time.Now().UTC()

	err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		for i := range sess.ExerciseSets {
			if sess.ExerciseSets[i].ID != workoutExerciseID {
				continue
			}
			if setIndex < 0 || setIndex >= len(sess.ExerciseSets[i].Sets) {
				break
			}
			wasComplete = sess.ExerciseSets[i].Sets[setIndex].CompletedAt != nil
			exercise = sess.ExerciseSets[i].Exercise
			setNumber = setIndex + 1
			setsTotal = len(sess.ExerciseSets[i].Sets)
			break
		}
		periodization = sess.PeriodizationType

		if err := sess.RecordSet(workoutExerciseID, setIndex, signal, weightKg, completedValue, now); err != nil {
			return err
		}
		hasMoreAfter = sess.HasIncompleteSets()
		return nil
	})
	if err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}

	if !wasComplete && hasMoreAfter {
		s.maybeSchedulePush(ctx, date, workoutExerciseID, exercise, periodization, setNumber, setsTotal, now)
	}
	return nil
}

// maybeSchedulePush schedules a rest-over push if every precondition holds:
// the user has push enabled, has at least one subscription, and the exercise's
// derivation yields a positive RestSeconds. Failures are logged at Debug; the
// completion itself is already persisted.
func (s *Service) maybeSchedulePush(
	ctx context.Context,
	date time.Time,
	workoutExerciseID int,
	exercise domain.Exercise,
	periodization domain.PeriodizationType,
	setNumber, setsTotal int,
	completedAt time.Time,
) {
	if s.scheduler == nil {
		return
	}
	restSeconds := domain.RestSecondsFor(exercise, periodization)
	if restSeconds <= 0 {
		return
	}
	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelDebug, "rest push: get preferences failed",
			slog.Any("error", err))
		return
	}
	if !prefs.RestNotificationsEnabled {
		return
	}
	subCount, err := s.repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelDebug, "rest push: count subscriptions failed",
			slog.Any("error", err))
		return
	}
	if subCount == 0 {
		return
	}
	userID := contexthelpers.AuthenticatedUserID(ctx)
	fireAt := completedAt.Add(time.Duration(restSeconds) * time.Second)

	payloadBytes, err := json.Marshal(struct {
		Title        string `json:"title"`
		Body         string `json:"body"`
		ExerciseName string `json:"exercise_name"`
		SetNumber    int    `json:"set_number"`
		SetsTotal    int    `json:"sets_total"`
		FireAtMS     int64  `json:"fire_at_ms"`
	}{
		Title:        "Rest over",
		Body:         fmt.Sprintf("Time for set %d of %d — %s", setNumber+1, setsTotal, exercise.Name),
		ExerciseName: exercise.Name,
		SetNumber:    setNumber,
		SetsTotal:    setsTotal,
		FireAtMS:     fireAt.UnixMilli(),
	})
	if err != nil {
		s.logger.LogAttrs(ctx, slog.LevelDebug, "rest push: marshal payload",
			slog.Any("error", err))
		return
	}

	push := domain.ScheduledPush{ //nolint:exhaustruct
		UserID:            userID,
		WorkoutExerciseID: workoutExerciseID,
		FireAt:            fireAt,
		Payload:           string(payloadBytes),
	}
	if err = s.scheduler.Schedule(ctx, push); err != nil {
		s.logger.LogAttrs(ctx, slog.LevelWarn, "rest push: schedule failed",
			slog.Any("error", err))
	}
	_ = date // reserved for future per-date logging
}
```

Add the imports: `encoding/json`, `log/slog`, and `github.com/myrjola/petrapp/internal/contexthelpers`.

- [ ] **Step 3: Cancel on `CompleteSession`**

Edit `internal/service/sessions.go`. Replace `CompleteSession`:

```go
func (s *Service) CompleteSession(ctx context.Context, date time.Time) error {
	if err := s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
		return sess.Complete(time.Now())
	}); err != nil {
		return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
	}
	if s.scheduler != nil {
		userID := contexthelpers.AuthenticatedUserID(ctx)
		if err := s.scheduler.CancelForWorkout(ctx, userID, date); err != nil {
			s.logger.LogAttrs(ctx, slog.LevelWarn, "cancel pending pushes on workout complete",
				slog.Any("error", err))
		}
	}
	return nil
}
```

Add the `contexthelpers` and `log/slog` imports if missing.

- [ ] **Step 4: Cancel on `SwapExercise`**

Edit `internal/service/exercises.go`. After the `Update` call in `SwapExercise` returns successfully:

```go
err = s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
    newSets := domain.BuildSetsForAdd(newExercise, sess.PeriodizationType, historicalSets)
    return sess.SwapExerciseInSlot(workoutExerciseID, newExercise, newSets)
})
if err != nil {
    return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err)
}

if s.scheduler != nil {
    if err = s.scheduler.Cancel(ctx, workoutExerciseID); err != nil {
        s.logger.LogAttrs(ctx, slog.LevelWarn, "cancel pending push on swap",
            slog.Int("workout_exercise_id", workoutExerciseID),
            slog.Any("error", err))
    }
}
return nil
```

Add `log/slog` import if missing.

- [ ] **Step 5: Write integration test for scheduling**

Edit `internal/service/sets_test.go`. Add:

```go
// fakeScheduler captures Schedule/Cancel calls in test.
type fakeScheduler struct {
	mu       sync.Mutex
	scheduled []domain.ScheduledPush
	cancels  []int
	workout  []struct {
		userID int
		date   time.Time
	}
}

func (f *fakeScheduler) Schedule(_ context.Context, push domain.ScheduledPush) error {
	f.mu.Lock()
	f.scheduled = append(f.scheduled, push)
	f.mu.Unlock()
	return nil
}

func (f *fakeScheduler) Cancel(_ context.Context, weID int) error {
	f.mu.Lock()
	f.cancels = append(f.cancels, weID)
	f.mu.Unlock()
	return nil
}

func (f *fakeScheduler) CancelForWorkout(_ context.Context, userID int, date time.Time) error {
	f.mu.Lock()
	f.workout = append(f.workout, struct {
		userID int
		date   time.Time
	}{userID, date})
	f.mu.Unlock()
	return nil
}

func Test_RecordSet_SchedulesRestPush(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	// Seed a second incomplete set so the just-completed one isn't the last.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 2, 100.0, 5)`, weID,
	); err != nil {
		t.Fatalf("seed second set: %v", err)
	}
	// Seed a push subscription so the precondition holds.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/1', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	// Make sure rest_notifications_enabled defaults to true: explicitly set the row.
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_preferences (user_id, rest_notifications_enabled) VALUES (?, 1)
		 ON CONFLICT(user_id) DO UPDATE SET rest_notifications_enabled = 1`, userID,
	); err != nil {
		t.Fatalf("seed preferences: %v", err)
	}

	fake := &fakeScheduler{}
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	weight := 100.0
	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.RecordSet(ctx, date, weID, 0, domain.SignalOnTarget, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 1 {
		t.Fatalf("Schedule calls = %d, want 1", len(fake.scheduled))
	}
	if fake.scheduled[0].WorkoutExerciseID != weID {
		t.Errorf("WorkoutExerciseID = %d, want %d", fake.scheduled[0].WorkoutExerciseID, weID)
	}
}

func Test_RecordSet_LastSetDoesNotSchedule(t *testing.T) {
	ctx, db, userID, weID := setupSessionForRecordSet(t)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		 VALUES (?, 'https://example.test/wp/last', 'p', 'a')`, userID,
	); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	fake := &fakeScheduler{}
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	weight := 100.0
	date := time.Now().UTC().Truncate(24 * time.Hour)
	if err := svc.RecordSet(ctx, date, weID, 0, domain.SignalOnTarget, &weight, 5); err != nil {
		t.Fatalf("RecordSet: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.scheduled) != 0 {
		t.Errorf("Schedule calls = %d, want 0 (last set should not schedule)", len(fake.scheduled))
	}
}

// setupSessionForRecordSet builds a workout session with one weighted exercise
// and one planned set, returning everything the scheduling tests need.
func setupSessionForRecordSet(t *testing.T) (context.Context, *sqlite.Database, int, int) {
	t.Helper()
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var userID int
	err = db.ReadWrite.QueryRowContext(ctx,
		"INSERT INTO users (webauthn_user_id, display_name) VALUES (?, ?) RETURNING id",
		[]byte("rs-user"), "RS User").Scan(&userID)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, userID)
	ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

	var exerciseID int
	if err = db.ReadOnly.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Deadlift'`,
	).Scan(&exerciseID); err != nil {
		t.Fatalf("get exercise id: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO workout_sessions (user_id, workout_date, started_at)
		 VALUES (?, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ'))`,
		userID, today,
	); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	var weID int
	if err = db.ReadWrite.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id)
		 VALUES (?, ?, ?) RETURNING id`,
		userID, today, exerciseID,
	).Scan(&weID); err != nil {
		t.Fatalf("insert workout_exercise: %v", err)
	}
	if _, err = db.ReadWrite.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number, weight_kg, target_value)
		 VALUES (?, 1, 100.0, 5)`, weID,
	); err != nil {
		t.Fatalf("insert set: %v", err)
	}
	return ctx, db, userID, weID
}
```

Imports for the new test code: add `sync`, `context`, and `github.com/myrjola/petrapp/internal/contexthelpers` if not already present. The existing `Test_RecordSetCompletion` should be refactored to call `setupSessionForRecordSet` instead of duplicating the preamble — the body shrinks to the post-setup assertions.

- [ ] **Step 6: Run the new tests**

Run:

```bash
go test ./internal/service -run "Test_RecordSet_SchedulesRestPush|Test_RecordSet_LastSetDoesNotSchedule" -race
```

Expected: PASS.

- [ ] **Step 7: Add CompleteSession-cancels test**

Append to `internal/service/sessions_test.go`:

```go
func Test_CompleteSession_CancelsPendingPushes(t *testing.T) {
	ctx, db, _, _ := setupSessionForRecordSet(t)
	fake := &fakeScheduler{}
	svc := service.NewService(db, testhelpers.NewLogger(testhelpers.NewWriter(t)), "").
		WithScheduler(fake)

	// CompleteSession requires StartedAt; seed it.
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if _, err := db.ReadWrite.ExecContext(ctx,
		`UPDATE workout_sessions SET started_at = STRFTIME('%Y-%m-%dT%H:%M:%fZ') WHERE workout_date = ?`,
		today.Format("2006-01-02"),
	); err != nil {
		t.Fatalf("seed started_at: %v", err)
	}

	if err := svc.CompleteSession(ctx, today); err != nil {
		t.Fatalf("CompleteSession: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.workout) != 1 {
		t.Errorf("CancelForWorkout calls = %d, want 1", len(fake.workout))
	}
}
```

- [ ] **Step 8: Run the cancel test**

Run:

```bash
go test ./internal/service -run "Test_CompleteSession_CancelsPendingPushes" -race
```

Expected: PASS.

- [ ] **Step 9: Run full service test suite**

Run:

```bash
go test ./internal/service -race
```

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/service/
git commit -m "Wire scheduler into RecordSet, CompleteSession, SwapExercise"
```

---

### Task 10: HTTP handler `/api/push/subscribe` and `/api/push/unsubscribe`

**Files:**
- Create: `cmd/web/handler-push.go`
- Test: `cmd/web/handler-push_test.go`
- Modify: `cmd/web/routes.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/web/handler-push_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_PushSubscribe_RoundTrip(t *testing.T) {
	ctx := t.Context()
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	client := server.Client()
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}

	body := strings.NewReader(`{
        "endpoint": "https://web.push.apple.com/test-endpoint",
        "keys": {"p256dh": "BPa-test", "auth": "auth-test"}
    }`)

	resp, err := postJSON(ctx, client, server.URL+"/api/push/subscribe", body)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("subscribe status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	// Unsubscribe.
	body = strings.NewReader(`{"endpoint":"https://web.push.apple.com/test-endpoint"}`)
	resp, err = postJSON(ctx, client, server.URL+"/api/push/unsubscribe", body)
	if err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("unsubscribe status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

// postJSON is a small helper using the e2etest Client's underlying http.Client.
// Implementer: if a helper like client.PostJSON already exists in e2etest,
// use that and delete this one. Inspect internal/e2etest/client.go before
// implementing.
func postJSON(ctx context.Context, c *e2etest.Client, url string, body *strings.Reader) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	req.Header.Set("Content-Type", "application/json")
	return c.HTTPClient().Do(req)
}
```

If `e2etest.Client` doesn't expose `HTTPClient()`, expose it: edit `internal/e2etest/client.go` to add a `func (c *Client) HTTPClient() *http.Client { return c.httpClient }`-style accessor — confirm the field name from the source first.

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./cmd/web -run Test_PushSubscribe
```

Expected: FAIL — endpoints don't exist.

- [ ] **Step 3: Implement the handler**

Create `cmd/web/handler-push.go`:

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/myrjola/petrapp/internal/domain"
)

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

func (app *application) pushSubscribePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096) //nolint:mnd // tiny JSON body.
	var req pushSubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.serverError(w, r, fmt.Errorf("decode subscribe body: %w", err))
		return
	}
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		app.serverError(w, r, errors.New("missing subscription fields"))
		return
	}
	sub := domain.PushSubscription{ //nolint:exhaustruct
		Endpoint: req.Endpoint,
		P256dh:   req.Keys.P256dh,
		Auth:     req.Keys.Auth,
	}
	if _, err := app.service.UpsertPushSubscription(r.Context(), sub); err != nil {
		app.serverError(w, r, fmt.Errorf("upsert subscription: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) pushUnsubscribePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096) //nolint:mnd
	var req pushUnsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.serverError(w, r, fmt.Errorf("decode unsubscribe body: %w", err))
		return
	}
	if err := app.service.DeletePushSubscription(r.Context(), req.Endpoint); err != nil {
		app.serverError(w, r, fmt.Errorf("delete subscription: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Add the service methods**

Create `internal/service/push.go`:

```go
package service

import (
	"context"
	"fmt"

	"github.com/myrjola/petrapp/internal/domain"
)

// UpsertPushSubscription inserts or updates the authenticated user's push
// subscription identified by endpoint.
func (s *Service) UpsertPushSubscription(
	ctx context.Context, sub domain.PushSubscription,
) (domain.PushSubscription, error) {
	stored, err := s.repos.PushSubscriptions.Insert(ctx, sub)
	if err != nil {
		return domain.PushSubscription{}, fmt.Errorf("insert push subscription: %w", err)
	}
	return stored, nil
}

// DeletePushSubscription removes the authenticated user's subscription
// identified by endpoint. Empty endpoint deletes all subscriptions for the
// user.
func (s *Service) DeletePushSubscription(ctx context.Context, endpoint string) error {
	if endpoint == "" {
		subs, err := s.repos.PushSubscriptions.ListByUser(ctx)
		if err != nil {
			return fmt.Errorf("list push subscriptions: %w", err)
		}
		for _, sub := range subs {
			if err = s.repos.PushSubscriptions.DeleteByID(ctx, sub.ID); err != nil {
				return fmt.Errorf("delete push subscription: %w", err)
			}
		}
		return nil
	}
	if err := s.repos.PushSubscriptions.DeleteByEndpoint(ctx, endpoint); err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	return nil
}

// CountPushSubscriptions returns the number of subscribed devices for the
// authenticated user.
func (s *Service) CountPushSubscriptions(ctx context.Context) (int, error) {
	n, err := s.repos.PushSubscriptions.CountByUser(ctx)
	if err != nil {
		return 0, fmt.Errorf("count push subscriptions: %w", err)
	}
	return n, nil
}
```

- [ ] **Step 5: Register the routes**

Edit `cmd/web/routes.go`. Add after the existing `/api/` routes:

```go
mux.Handle("POST /api/push/subscribe",
    app.mustSessionStack(http.HandlerFunc(app.pushSubscribePOST)))
mux.Handle("POST /api/push/unsubscribe",
    app.mustSessionStack(http.HandlerFunc(app.pushUnsubscribePOST)))
```

- [ ] **Step 6: Verify tests pass**

Run:

```bash
go test ./cmd/web -run Test_PushSubscribe
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-push.go cmd/web/handler-push_test.go cmd/web/routes.go internal/service/push.go internal/e2etest/
git commit -m "Add /api/push/subscribe and /api/push/unsubscribe endpoints"
```

---

### Task 11: Service worker (`ui/static/sw.js`)

**Files:**
- Create: `ui/static/sw.js`

- [ ] **Step 1: Create the service worker**

Create `ui/static/sw.js`:

```js
// Service worker for Petra. Owns Web Push delivery only — no offline caching.

self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()));

self.addEventListener('push', (event) => {
    let payload = {};
    try {
        if (event.data) {
            payload = event.data.json();
        }
    } catch (_) {
        // ignore malformed payloads; fall back to defaults below
    }
    const title = payload.title || 'Rest over';
    const body = payload.body || 'Time for your next set.';
    const tag = payload.exercise_name ? `rest-${payload.exercise_name}` : 'rest';

    event.waitUntil(self.registration.showNotification(title, {
        body,
        tag,
        renotify: true,
        icon: '/apple-touch-icon.png',
        badge: '/logo.svg',
        data: payload,
    }));
});

self.addEventListener('notificationclick', (event) => {
    event.notification.close();
    const targetPath = '/';
    event.waitUntil((async () => {
        const clientList = await self.clients.matchAll({type: 'window', includeUncontrolled: true});
        for (const client of clientList) {
            if ('focus' in client) {
                await client.focus();
                return;
            }
        }
        if (self.clients.openWindow) {
            await self.clients.openWindow(targetPath);
        }
    })());
});
```

- [ ] **Step 2: Commit**

```bash
git add ui/static/sw.js
git commit -m "Add production service worker for rest-timer push"
```

---

### Task 12: Preferences extension (handler + template)

**Files:**
- Modify: `cmd/web/handler-preferences.go`
- Modify: `ui/templates/pages/preferences/preferences.gohtml`

- [ ] **Step 1: Extend the handler with VAPID key + subscription status**

Edit `cmd/web/handler-preferences.go`. Replace `preferencesTemplateData`:

```go
type preferencesTemplateData struct {
	BaseTemplateData
	Weekdays                 []weekdayPreference
	DurationOptions          []workoutDurationOption
	VAPIDPublicKey           string
	PushSubscriptionCount    int
	RestNotificationsEnabled bool
}
```

Replace `preferencesGET`:

```go
func (app *application) preferencesGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefs, err := app.service.GetUserPreferences(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}
	subCount, err := app.service.CountPushSubscriptions(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("count push subscriptions: %w", err))
		return
	}

	data := preferencesTemplateData{
		BaseTemplateData:         newBaseTemplateData(r),
		Weekdays:                 preferencesToWeekdays(prefs),
		DurationOptions:          getWorkoutDurationOptions(),
		VAPIDPublicKey:           app.vapidPublicKey,
		PushSubscriptionCount:    subCount,
		RestNotificationsEnabled: prefs.RestNotificationsEnabled,
	}

	app.render(w, r, http.StatusOK, "preferences", data)
}
```

`app.vapidPublicKey` is wired in Task 14. For now the field doesn't exist yet — that's expected, the test for Task 12 just verifies the form/template rendering; we tolerate the empty string.

Extend `preferencesPOST` to read the toggle. Replace the relevant block:

```go
prefs := weekdaysToPreferences(r)
prefs.RestNotificationsEnabled = r.Form.Get("rest_notifications_enabled") == "on"
```

Update `weekdaysToPreferences` to set `RestNotificationsEnabled` from the form there if you prefer; but inline assignment is fine and keeps the helper focused on the schedule grid.

- [ ] **Step 2: Add a placeholder field on `application` so the code compiles**

Edit `cmd/web/main.go`. Add to the `application` struct:

```go
type application struct {
	logger          *slog.Logger
	webAuthnHandler *webauthnhandler.WebAuthnHandler
	sessionManager  *scs.SessionManager
	templateFS      fs.FS
	service         *service.Service
	flightRecorder  *flightrecorder.Service
	devMode         bool
	vapidPublicKey  string
}
```

Initialise the new field as `""` in the `app := application{...}` literal — Task 14 wires the real value.

- [ ] **Step 3: Extend the template**

Edit `ui/templates/pages/preferences/preferences.gohtml`. Insert this section between `</form>` (close of schedule form, ~line 283) and `<div class="data-export-section">`:

```gohtml
<div class="rest-notifications-section">
    <style {{ nonce }}>
        @scope (.rest-notifications-section) {
            :scope {
                background: var(--color-surface-elevated);
                border: var(--border-size-1) solid var(--color-border);
                border-radius: var(--radius-3);
                padding: var(--size-6);
                margin-bottom: var(--size-6);
                box-shadow: var(--shadow-2);
            }

            h2 {
                font-size: var(--font-size-3);
                font-weight: var(--font-weight-6);
                color: var(--color-text-primary);
                margin-bottom: var(--size-3);
            }

            .status {
                font-size: var(--font-size-1);
                color: var(--color-text-secondary);
                margin-bottom: var(--size-3);
            }

            .install-hint {
                font-size: var(--font-size-1);
                color: var(--color-text-secondary);
            }

            .toggle-row {
                display: flex;
                align-items: center;
                gap: var(--size-2);
                margin-top: var(--size-3);
                font-size: var(--font-size-1);
            }

            .push-button {
                padding: var(--size-2) var(--size-4);
                border-radius: var(--radius-2);
                border: var(--border-size-1) solid var(--color-border);
                background: var(--color-surface);
                font-weight: var(--font-weight-5);
                cursor: pointer;
            }

            .push-button.danger {
                color: var(--red-8);
                border-color: var(--red-3);
            }

            .error {
                color: var(--red-8);
                font-size: var(--font-size-1);
                margin-top: var(--size-2);
            }

            .hidden {
                display: none;
            }
        }
    </style>

    <h2>Rest notifications</h2>

    <div data-rest-notifications data-vapid-key="{{ .VAPIDPublicKey }}" data-sub-count="{{ .PushSubscriptionCount }}">
        <p class="status not-standalone hidden">
            On iOS, Add to Home Screen first, then open the installed app and try again.
        </p>
        <div class="standalone hidden">
            <p class="status not-subscribed hidden">Not enabled on this device.</p>
            <p class="status subscribed hidden">
                Enabled — you'll be pinged when each rest is up.
            </p>
            <button type="button" class="push-button enable-btn hidden">Enable rest notifications</button>
            <button type="button" class="push-button danger disable-btn hidden">Disable on this device</button>
            <p class="error hidden"></p>
            <form method="post" action="/preferences/rest-notifications-toggle" class="toggle-row subscribed hidden">
                <label>
                    <input type="checkbox" name="rest_notifications_enabled"
                           {{ if .RestNotificationsEnabled }}checked{{ end }}
                           onchange="this.form.submit()">
                    Send notifications when rest is over
                </label>
            </form>
        </div>
    </div>

    <script {{ nonce }}>
        (() => {
            const root = document.querySelector('[data-rest-notifications]');
            if (!root) return;
            const isStandalone = window.matchMedia('(display-mode: standalone)').matches ||
                                 window.navigator.standalone === true;
            const supported = 'Notification' in window && 'serviceWorker' in navigator && 'PushManager' in window;
            if (!supported) {
                root.style.display = 'none';
                return;
            }
            const notStandaloneEl = root.querySelector('.not-standalone');
            const standaloneEl = root.querySelector('.standalone');
            if (!isStandalone) {
                notStandaloneEl.classList.remove('hidden');
                return;
            }
            standaloneEl.classList.remove('hidden');

            const notSubEl = root.querySelector('.not-subscribed');
            const subEl = root.querySelector('.status.subscribed');
            const enableBtn = root.querySelector('.enable-btn');
            const disableBtn = root.querySelector('.disable-btn');
            const toggleForm = root.querySelector('.toggle-row.subscribed');
            const errorEl = root.querySelector('.error');
            const vapidKey = root.dataset.vapidKey;

            const showError = (msg) => {
                errorEl.textContent = msg;
                errorEl.classList.remove('hidden');
            };
            const clearError = () => {
                errorEl.textContent = '';
                errorEl.classList.add('hidden');
            };

            const refreshUI = async () => {
                const reg = await navigator.serviceWorker.ready;
                const sub = await reg.pushManager.getSubscription();
                if (sub) {
                    notSubEl.classList.add('hidden');
                    subEl.classList.remove('hidden');
                    enableBtn.classList.add('hidden');
                    disableBtn.classList.remove('hidden');
                    toggleForm.classList.remove('hidden');
                } else {
                    notSubEl.classList.remove('hidden');
                    subEl.classList.add('hidden');
                    enableBtn.classList.remove('hidden');
                    disableBtn.classList.add('hidden');
                    toggleForm.classList.add('hidden');
                }
            };

            const b64ToUint8 = (b64) => {
                const padding = '='.repeat((4 - b64.length % 4) % 4);
                const base64 = (b64 + padding).replace(/-/g, '+').replace(/_/g, '/');
                const raw = atob(base64);
                return Uint8Array.from([...raw].map(c => c.charCodeAt(0)));
            };

            enableBtn.addEventListener('click', async () => {
                clearError();
                try {
                    const perm = await Notification.requestPermission();
                    if (perm !== 'granted') {
                        showError('Notification permission was denied. Re-enable in your device settings if you change your mind.');
                        return;
                    }
                    const reg = await navigator.serviceWorker.ready;
                    const sub = await reg.pushManager.subscribe({
                        userVisibleOnly: true,
                        applicationServerKey: b64ToUint8(vapidKey),
                    });
                    const resp = await fetch('/api/push/subscribe', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json', 'X-Requested-With': 'fetch'},
                        body: JSON.stringify(sub),
                    });
                    if (!resp.ok) throw new Error('subscribe HTTP ' + resp.status);
                    await refreshUI();
                } catch (e) {
                    showError(e.name + ': ' + e.message);
                }
            });

            disableBtn.addEventListener('click', async () => {
                clearError();
                try {
                    const reg = await navigator.serviceWorker.ready;
                    const sub = await reg.pushManager.getSubscription();
                    if (sub) {
                        await fetch('/api/push/unsubscribe', {
                            method: 'POST',
                            headers: {'Content-Type': 'application/json', 'X-Requested-With': 'fetch'},
                            body: JSON.stringify({endpoint: sub.endpoint}),
                        });
                        await sub.unsubscribe();
                    }
                    await refreshUI();
                } catch (e) {
                    showError(e.name + ': ' + e.message);
                }
            });

            navigator.serviceWorker.ready.then(refreshUI);
        })();
    </script>
</div>
```

- [ ] **Step 4: Add the toggle-only POST route**

Edit `cmd/web/handler-preferences.go`. Add:

```go
func (app *application) preferencesRestNotificationsTogglePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}
	prefs, err := app.service.GetUserPreferences(r.Context())
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get preferences: %w", err))
		return
	}
	prefs.RestNotificationsEnabled = r.Form.Get("rest_notifications_enabled") == "on"
	if err = app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save preferences: %w", err))
		return
	}
	redirect(w, r, "/preferences")
}
```

Register in `routes.go`:

```go
mux.Handle("POST /preferences/rest-notifications-toggle",
    app.mustSessionStack(http.HandlerFunc(app.preferencesRestNotificationsTogglePOST)))
```

- [ ] **Step 5: Verify compile and existing preferences test still passes**

Run:

```bash
go test ./cmd/web -run TestPreferences
go test ./cmd/web -run Test_PushSubscribe
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/handler-preferences.go cmd/web/main.go cmd/web/routes.go ui/templates/pages/preferences/preferences.gohtml
git commit -m "Add rest-notifications section to /preferences"
```

---

### Task 13: Rest chip on active set card

**Files:**
- Modify: `cmd/web/handler-exerciseset.go`
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml`
- Test: `cmd/web/handler-exerciseset_test.go`

- [ ] **Step 1: Compute `RestEndAtMs` in the handler**

Edit `cmd/web/handler-exerciseset.go`. Add to `exerciseSetTemplateData`:

```go
type exerciseSetTemplateData struct {
	BaseTemplateData
	Date                  time.Time
	ExerciseSet           domain.ExerciseSet
	SetsDisplay           []setDisplay
	FirstIncompleteIndex  int
	EditingIndex          int
	IsEditing             bool
	LastCompletedAt       *time.Time
	CurrentSetTarget      domain.SetTarget
	CurrentSetTimedTarget int
	AbsCurrentWeight      float64
	RestEndAtMs           int64 // 0 when no rest chip should be shown.
}
```

In `exerciseSetGET`, compute it before constructing `data`:

```go
var restEndAtMs int64
lastCompletedAt := getLastCompletedAt(exerciseSet.Sets)
if lastCompletedAt != nil {
    restSeconds := domain.RestSecondsFor(exerciseSet.Exercise, session.PeriodizationType)
    if restSeconds > 0 {
        restEnd := lastCompletedAt.Add(time.Duration(restSeconds) * time.Second)
        if restEnd.After(time.Now()) {
            restEndAtMs = restEnd.UnixMilli()
        }
    }
}
```

Set `RestEndAtMs: restEndAtMs` in the `data` literal.

- [ ] **Step 2: Render the chip in the template**

Edit `ui/templates/pages/exerciseset/sets-container.gohtml`. Add at the top of the outer `<div class="sets-container...">`, after the `<style>` block and before the `{{ range $index, $setDisplay := .SetsDisplay }}` loop:

```gohtml
{{ if gt .RestEndAtMs 0 }}
<div class="rest-chip" data-rest-end-at-ms="{{ .RestEndAtMs }}" aria-live="polite">
    <style {{ nonce }}>
        @scope (.rest-chip) {
            :scope {
                display: inline-flex;
                align-items: center;
                gap: var(--size-2);
                padding: var(--size-2) var(--size-3);
                border-radius: var(--radius-round);
                background: var(--color-info-bg);
                color: var(--color-info);
                font-weight: var(--font-weight-6);
                font-size: var(--font-size-1);
                align-self: flex-start;
            }
            :scope.ready {
                background: var(--color-success-bg);
                color: var(--color-success);
            }
        }
    </style>
    <span class="label">Rest</span>
    <span class="time" data-rest-time>—:—</span>
</div>
<script {{ nonce }}>
    (() => {
        const chips = document.querySelectorAll('.rest-chip[data-rest-end-at-ms]');
        if (chips.length === 0) return;
        const tick = () => {
            const now = Date.now();
            chips.forEach((chip) => {
                const endAt = parseInt(chip.dataset.restEndAtMs, 10);
                const remaining = Math.max(0, endAt - now);
                const timeEl = chip.querySelector('[data-rest-time]');
                if (remaining === 0) {
                    chip.classList.add('ready');
                    if (timeEl) timeEl.textContent = 'Ready';
                } else {
                    const totalSec = Math.ceil(remaining / 1000);
                    const m = Math.floor(totalSec / 60);
                    const s = totalSec % 60;
                    if (timeEl) timeEl.textContent = m + ':' + String(s).padStart(2, '0');
                }
            });
        };
        tick();
        setInterval(tick, 250);
    })();
</script>
{{ end }}
```

- [ ] **Step 3: Write the failing test**

Append to `cmd/web/handler-exerciseset_test.go`:

```go
func Test_ExerciseSet_RestChipAfterCompletedSet(t *testing.T) {
	ctx := t.Context()
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	client := server.Client()
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Set today as a workout day.
	formData := map[string]string{
		time.Now().Weekday().String(): "60",
	}
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("save preferences: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if doc, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("start workout: %v", err)
	}

	// Find a weighted exercise and post a set completion.
	var weightedExerciseURL string
	doc.Find("a.exercise").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		href, _ := sel.Attr("href")
		weightedExerciseURL = href
		return false
	})
	if weightedExerciseURL == "" {
		t.Fatal("no exercise found")
	}
	if doc, err = client.GetDoc(ctx, weightedExerciseURL); err != nil {
		t.Fatalf("get exercise: %v", err)
	}
	// Mark warmup complete to enable the form.
	warmupAction := weightedExerciseURL + "/warmup/complete"
	if doc, err = client.SubmitForm(ctx, doc, warmupAction, nil); err != nil {
		t.Fatalf("warmup complete: %v", err)
	}

	// Submit set 0 with on_target signal.
	setAction := weightedExerciseURL + "/sets/0/update"
	if doc, err = client.SubmitForm(ctx, doc, setAction, map[string]string{
		"weight": "100",
		"reps":   "5",
		"signal": "on_target",
	}); err != nil {
		t.Fatalf("submit set: %v", err)
	}

	chip := doc.Find(".rest-chip[data-rest-end-at-ms]")
	if chip.Length() == 0 {
		t.Fatal("expected .rest-chip on the page after completing a set")
	}
	endAtStr, _ := chip.Attr("data-rest-end-at-ms")
	endAtMs, err := strconv.ParseInt(endAtStr, 10, 64)
	if err != nil {
		t.Fatalf("parse data-rest-end-at-ms: %v", err)
	}
	if endAtMs < time.Now().UnixMilli() {
		t.Errorf("data-rest-end-at-ms = %d is in the past", endAtMs)
	}
}
```

Add imports `strconv` and re-use any existing `goquery` import already in the file.

- [ ] **Step 4: Run the test**

Run:

```bash
go test ./cmd/web -run Test_ExerciseSet_RestChipAfterCompletedSet
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/web/handler-exerciseset.go cmd/web/handler-exerciseset_test.go ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "Render rest-timer countdown chip on active set card"
```

---

### Task 14: Wire notification package into `main.go`, base template, dev scripts

**Files:**
- Modify: `cmd/web/main.go`
- Modify: `cmd/web/middleware.go`
- Modify: `ui/templates/base.gohtml`
- Modify: `scripts/dev.sh`
- Modify: `scripts/dev-tailscale-https.sh`

- [ ] **Step 1: Extend `config` with VAPID fields**

Edit `cmd/web/main.go`. Extend `config`:

```go
type config struct {
	// ... existing fields ...
	VAPIDPublic           string        `env:"PETRAPP_VAPID_PUBLIC" envDefault:""`
	VAPIDPrivate          string        `env:"PETRAPP_VAPID_PRIVATE" envDefault:""`
	VAPIDSubject          string        `env:"PETRAPP_VAPID_SUBJECT" envDefault:"vapid@example.com"`
	IdleTimeout           time.Duration `env:"PETRAPP_NOTIFICATION_IDLE_TIMEOUT" envDefault:"5m"`
}
```

Confirm `envstruct` supports `time.Duration`. If not, accept a `string` and parse it with `time.ParseDuration` inline; or fall back to `int` seconds with a 300 default.

- [ ] **Step 2: Validate VAPID env in prod, generate in dev**

In `run`, after `envstruct.Populate`:

```go
if cfg.VAPIDPublic == "" || cfg.VAPIDPrivate == "" {
    if cfg.FlyAppName != "" {
        return errors.New("PETRAPP_VAPID_PUBLIC and PETRAPP_VAPID_PRIVATE must be set in production")
    }
    priv, pub, genErr := webpush.GenerateVAPIDKeys()
    if genErr != nil {
        return fmt.Errorf("generate dev vapid keys: %w", genErr)
    }
    cfg.VAPIDPrivate, cfg.VAPIDPublic = priv, pub
    logger.LogAttrs(ctx, slog.LevelWarn, "generated ephemeral VAPID keys for dev",
        slog.String("public", pub))
}
```

Add the imports `webpush "github.com/SherClockHolmes/webpush-go"` and `errors`.

- [ ] **Step 3: Wire Sender + Scheduler + IdleMonitor + service**

Replace the `app := application{...}` block with the wiring sequence:

```go
sender := notification.NewSender(notification.SenderConfig{ //nolint:exhaustruct
    VAPIDSubject:    cfg.VAPIDSubject,
    VAPIDPublicKey:  cfg.VAPIDPublic,
    VAPIDPrivateKey: cfg.VAPIDPrivate,
    Logger:          logger,
})

baseService := service.NewService(db, logger, cfg.OpenAIAPIKey)

scheduler := notification.NewScheduler(notification.SchedulerConfig{
    Repo:     baseService.ScheduledPushRepo(),
    Dispatch: makeDispatchFunc(logger, baseService, sender),
    Logger:   logger,
    Now:      time.Now,
})
if err = scheduler.Reload(ctx); err != nil {
    return fmt.Errorf("reload scheduled pushes: %w", err)
}

svc := baseService.WithScheduler(scheduler)

var lastRequestAt atomic.Int64
lastRequestAt.Store(time.Now().UnixNano())

idleMonitor := notification.NewIdleMonitor(notification.IdleMonitorConfig{
    IdleThreshold: cfg.IdleTimeout,
    TickInterval:  10 * time.Second, //nolint:mnd
    Now:           time.Now,
    LastRequestAt: func() time.Time { return time.Unix(0, lastRequestAt.Load()) },
    PendingCount:  scheduler.PendingCount,
    Trigger: func() {
        _ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
    },
    Logger: logger,
})
go idleMonitor.Run(ctx)

app := application{
    logger:          logger,
    webAuthnHandler: webAuthnHandler,
    sessionManager:  sessionManager,
    templateFS:      os.DirFS(htmlTemplatePath),
    service:         svc,
    flightRecorder:  flightRecorderService,
    devMode:         cfg.FlyAppName == "",
    vapidPublicKey:  cfg.VAPIDPublic,
    lastRequestAt:   &lastRequestAt,
}
```

Add the `application` field:

```go
lastRequestAt *atomic.Int64
```

Add imports: `sync/atomic`, `syscall`, plus `"github.com/myrjola/petrapp/internal/notification"`.

- [ ] **Step 4: Expose `ScheduledPushRepo` from `Service`**

Edit `internal/service/service.go`. Add the accessor:

```go
// ScheduledPushRepo returns the persistent scheduled-push repo so the
// notification.Scheduler can be wired without re-instantiating the
// repositories. Only intended for process startup in main.go.
func (s *Service) ScheduledPushRepo() repository.ScheduledPushRepository {
	return s.repos.ScheduledPushes
}
```

- [ ] **Step 5: Add `makeDispatchFunc`**

Append to `internal/service/push.go`:

```go
package service
```

No — `makeDispatchFunc` belongs in the main package since it wires Sender + repos together with a context that re-establishes the user identity. Create `cmd/web/dispatch.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/myrjola/petrapp/internal/contexthelpers"
	"github.com/myrjola/petrapp/internal/domain"
	"github.com/myrjola/petrapp/internal/notification"
	"github.com/myrjola/petrapp/internal/service"
)

// makeDispatchFunc returns the dispatcher closure passed to notification.Scheduler.
// It loads the user's subscriptions, sends the payload to each, and prunes any
// 410'd subscriptions. The closure runs outside an HTTP context, so it
// re-establishes the authenticated-user-id via contexthelpers.
func makeDispatchFunc(
	logger *slog.Logger,
	svc *service.Service,
	sender *notification.Sender,
) notification.DispatchFunc {
	return func(ctx context.Context, push domain.ScheduledPush) error {
		ctx = context.WithValue(ctx, contexthelpers.AuthenticatedUserIDContextKey, push.UserID)
		ctx = context.WithValue(ctx, contexthelpers.IsAuthenticatedContextKey, true)

		// Recheck the user's opt-in — they may have flipped it off mid-rest.
		prefs, err := svc.GetUserPreferences(ctx)
		if err != nil {
			return err
		}
		if !prefs.RestNotificationsEnabled {
			return nil
		}

		subs, err := svc.ListPushSubscriptions(ctx)
		if err != nil {
			return err
		}
		if len(subs) == 0 {
			return nil
		}

		// Validate JSON shape — payload was generated by maybeSchedulePush.
		var probe map[string]any
		if err = json.Unmarshal([]byte(push.Payload), &probe); err != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "skip dispatch: bad payload",
				slog.Int("push_id", push.ID))
			return nil
		}

		for _, sub := range subs {
			if sendErr := sender.Send(ctx, sub, []byte(push.Payload)); sendErr != nil {
				if errors.Is(sendErr, notification.ErrSubscriptionGone) {
					if delErr := svc.DeletePushSubscriptionByID(ctx, sub.ID); delErr != nil {
						logger.LogAttrs(ctx, slog.LevelWarn, "prune gone subscription",
							slog.Int("sub_id", sub.ID),
							slog.Any("error", delErr))
					}
					continue
				}
				logger.LogAttrs(ctx, slog.LevelWarn, "push send",
					slog.Int("sub_id", sub.ID),
					slog.Any("error", sendErr))
			}
		}
		return nil
	}
}
```

Add the two helper methods to `internal/service/push.go`:

```go
func (s *Service) ListPushSubscriptions(ctx context.Context) ([]domain.PushSubscription, error) {
	subs, err := s.repos.PushSubscriptions.ListByUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	return subs, nil
}

func (s *Service) DeletePushSubscriptionByID(ctx context.Context, id int) error {
	if err := s.repos.PushSubscriptions.DeleteByID(ctx, id); err != nil {
		return fmt.Errorf("delete push subscription by id: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Add the `lastRequestAt`-stamping middleware**

Edit `cmd/web/middleware.go`. Add at the bottom:

```go
// stampLastRequest updates the application's lastRequestAt atomic on every
// request. Used by notification.IdleMonitor to gate process exit.
func (app *application) stampLastRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.lastRequestAt.Store(time.Now().UnixNano())
		next.ServeHTTP(w, r)
	})
}
```

Wire it into `withoutMaintenanceModeStack` in `cmd/web/middleware-stacks.go`:

```go
func (app *application) withoutMaintenanceModeStack(next http.Handler) http.Handler {
	return app.stampLastRequest(app.logAndTraceRequest(secureHeaders(app.crossOriginProtection(
		commonContext(app.timeout(next))))))
}
```

- [ ] **Step 7: Register the service worker in base.gohtml**

Edit `ui/templates/base.gohtml`. Replace `<script {{nonce}} src="/main.js"></script>` with:

```gohtml
<script {{nonce}} src="/main.js"></script>
<script {{nonce}}>
    if ('serviceWorker' in navigator) {
        navigator.serviceWorker.register('/sw.js').catch(() => { /* ignore — push is opt-in */ });
    }
</script>
```

- [ ] **Step 8: Update dev scripts**

Edit `scripts/dev.sh`. Add right after `set -euo pipefail`:

```bash
if [[ -z "${PETRAPP_VAPID_PUBLIC:-}" ]]; then
    # Generate ephemeral keys via the binary's own helper. The Go process
    # will also detect-and-generate at startup, so this is purely so the
    # operator can see the public key (e.g. to align previously-subscribed
    # devices).
    :
fi
```

Or simpler — leave key generation entirely to the binary (it already logs the public key at startup). No script change needed beyond a comment to that effect:

```bash
# VAPID keys: PETRAPP_VAPID_PUBLIC / PETRAPP_VAPID_PRIVATE.
# Unset → binary generates ephemeral pair on startup and logs the public key.
```

Same comment in `scripts/dev-tailscale-https.sh`.

- [ ] **Step 9: Build and run dev**

Run:

```bash
make build && ./bin/petrapp 2>&1 | head -20
```

Expected: startup log includes `"generated ephemeral VAPID keys for dev"` with the public key. Press Ctrl-C.

- [ ] **Step 10: Verify the test suite**

Run:

```bash
make test
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add cmd/web/main.go cmd/web/middleware.go cmd/web/middleware-stacks.go cmd/web/dispatch.go internal/service/service.go internal/service/push.go ui/templates/base.gohtml scripts/dev.sh scripts/dev-tailscale-https.sh
git commit -m "Wire notification.Scheduler, Sender, IdleMonitor into main"
```

---

### Task 15: Flip `auto_stop_machines = "off"` in `fly.toml`

**Files:**
- Modify: `fly.toml`

- [ ] **Step 1: Update fly.toml**

Edit `fly.toml`. Replace:

```toml
auto_stop_machines = "stop"
```

with:

```toml
auto_stop_machines = "off"
```

The IdleMonitor now handles the shutdown side. Fly's `auto_start_machines = true` keeps starting the Machine on incoming requests.

- [ ] **Step 2: Commit**

```bash
git add fly.toml
git commit -m "Hand off auto-stop to in-app IdleMonitor"
```

---

### Task 16: Playwright e2e — rest chip appears, ticks, transitions to Ready

**Files:**
- Modify: `cmd/web/playwright_test.go`

- [ ] **Step 1: Inspect the existing Playwright test structure**

Read `cmd/web/playwright_test.go` end-to-end. Identify:

- The test function naming pattern.
- How a browser page is created and navigated.
- How a workout is set up (preferences → start → exercise → warmup → complete set).
- Which assertion style is used (`t.Errorf`, `playwright.PlaywrightAssertions`, etc.).

The e2etest coverage in Task 13 already proves the chip renders. This Playwright test verifies the JS countdown ticks in a real browser.

- [ ] **Step 2: Add the test by adapting the closest existing pattern**

Pick the playwright test that most closely matches "user completes a set then verifies what's on the next page" — use it as a template. After the simulated set-completion click that lands on the exercise page, add these assertions:

```go
chip := page.Locator(".rest-chip[data-rest-end-at-ms]")
if err := expect.Locator(chip).ToBeVisible(); err != nil {
    t.Fatalf("rest chip not visible: %v", err)
}
timeText, err := chip.Locator("[data-rest-time]").TextContent()
if err != nil {
    t.Fatalf("read time text: %v", err)
}
if matched, _ := regexp.MatchString(`^\d:\d{2}$`, timeText); !matched {
    t.Errorf("rest chip text %q does not match M:SS", timeText)
}

// Wait long enough for the 250ms interval to update the text.
time.Sleep(700 * time.Millisecond)
timeText2, _ := chip.Locator("[data-rest-time]").TextContent()
if timeText2 == timeText {
    t.Errorf("rest chip text did not change in 700ms: still %q", timeText2)
}
```

Use whichever locator/assertion helper (`expect`, raw `Locator`) the surrounding tests use. If `expect` isn't imported anywhere, drop the `ToBeVisible()` call and use `chip.Count()` / explicit waits in the file's existing style.

- [ ] **Step 3: Run the test**

Run:

```bash
go test ./cmd/web -run TestPlaywright_RestChip -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add cmd/web/playwright_test.go
git commit -m "Add Playwright test for rest-chip countdown"
```

---

### Task 17: Final verification

- [ ] **Step 1: Run the full CI suite**

Run:

```bash
make ci
```

Expected: PASS (build + lint + test + sec).

- [ ] **Step 2: Manual smoke test on the dev server**

Run:

```bash
make dev-tailnet
```

Then on an iPhone with the PWA installed:

1. Open `/preferences`. Verify the "Rest notifications" section appears, with "Not enabled on this device." status and an "Enable rest notifications" button.
2. Tap "Enable" — confirm the iOS permission prompt appears. Approve.
3. Status updates to "Enabled — you'll be pinged when each rest is up." Toggle "Send notifications when rest is over" persists across reload.
4. Navigate to today's workout, mark warmup complete, complete the first set of an exercise (rep-based, e.g. Deadlift).
5. Confirm a `.rest-chip` appears above the next set's form, ticking down from a number close to 3:00 (180s, strength week) or 2:30 (150s, hypertrophy week).
6. Lock the phone. After ~RestSeconds, the notification arrives.
7. Tap the notification — it focuses or reopens the PWA on the workout page.
8. Verify "Disable on this device" removes the subscription and the section reverts to the not-subscribed state.

- [ ] **Step 3: Acceptance checklist**

Walk through the spec's Acceptance section and confirm each bullet is met. If something fails, file a follow-up rather than letting the plan claim done.

---

## Spec coverage check

| Spec section | Tasks |
|---|---|
| `internal/notification` package — Sender | Task 6 |
| `internal/notification` package — Scheduler | Task 7 |
| `internal/notification` package — IdleMonitor | Task 8 |
| `internal/repository/pushsubscription.go` and `pushschedule.go` | Tasks 4, 5 |
| `internal/service` integration (RecordSet, CompleteWorkout, SwapExercise) | Task 9 |
| `cmd/web/handler-preferences.go` extension | Task 12 |
| `cmd/web/handler-push.go` (new) | Task 10 |
| `ui/static/sw.js` (new) | Task 11 |
| `ui/templates/pages/exerciseset/sets-container.gohtml` extension | Task 13 |
| `fly.toml` auto_stop_machines change | Task 15 |
| Data model — push_subscriptions, scheduled_pushes, rest_notifications_enabled | Task 1 |
| Configuration — VAPID env vars + dev key gen | Task 14 |
| Onboarding UX detection rules | Task 12 |
| Visual countdown chip rules | Task 13 |
| Error handling (VAPID missing in prod, permission denied, 410, 5xx, timeout) | Tasks 6, 12, 14 |
| Testing — sender_test, scheduler_test, idle_monitor_test, service tests, handler-push_test, handler-exerciseset_test, playwright | Tasks 6, 7, 8, 9, 10, 13, 16 |
| Cleanup — POC removal | Not applicable; POCs already removed in commit `2b2476d`. |
| `webpush-go` re-added with bare-email Subscriber pin | Tasks 1, 6 |

All spec sections trace to at least one task. The POC cleanup section is already satisfied by the pre-existing `go mod tidy` commit on `main`.
