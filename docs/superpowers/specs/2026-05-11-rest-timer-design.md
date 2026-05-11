# Rest Timer With Web Push Notifications

## Motivation

`internal/domain/progression_scheme.go` already produces a `RestSeconds` value
per exercise (180s / 150s / 90s by rep bucket) and the planner carries it on
each `PlannedSet`. The value is unused at the UI layer today.

The user typically backgrounds the PWA between sets — phones rest in a
pocket, eyes on the gym. Same-document timer techniques (Ripple-style silent
oscillator + scheduled WebAudio bell, validated in the now-retired
`/dev/rest-timer-poc`) die at the first form submit because petrapp's stack
navigator does a real document navigation per POST. Web Push survives both
backgrounding and navigation and was validated against iOS PWA + Apple's
push service in the also-now-retired `/dev/web-push-poc`.

This spec covers the production rest-timer feature: per-user push
subscriptions, server-scheduled notifications fired `RestSeconds` after each
set completion, an on-page countdown chip for the foreground case, and a
preferences toggle.

## Decisions

| Decision | Choice | Reason |
|---|---|---|
| Delivery channel | Web Push only | The only channel that survives backgrounding; in-app sound only matters on the same document, which petrapp's nav model precludes. |
| On-screen visual | Inline countdown chip on the active set card | Spatial — the rest belongs to the upcoming set. Computed from `last_completed_at + RestSeconds`, no new storage. Existing corner elapsed-clock keeps its role. |
| Opt-in surface | A "Rest notifications" section in `/preferences` only | No nagging on workout pages. Inline guidance when the page isn't a standalone PWA (iOS install gate). |
| Opt-out granularity | Per-user boolean on `workout_preferences`, default-on once subscribed | Silences pushes while keeping the visual countdown. One column. |
| Notification actions | None (no "Skip rest" button) | iOS strips actions, so adding them is Android-only. Defer until evidence the user wants it. |
| VAPID key storage | `PETRAPP_VAPID_PUBLIC` / `PETRAPP_VAPID_PRIVATE` env vars (Fly secrets in prod) | Standard Fly pattern. Explicit, rotatable, no schema. Dev script generates ephemeral keys when unset. |
| Push subscription storage | `push_subscriptions` table, one row per device | Standard shape; multiple devices per user supported. |
| Scheduled-push storage | `scheduled_pushes` table, replaced on next set submit | Persisted so pending pushes survive restarts; dispatcher reloads on startup. |
| Server-side scheduler | In-process `time.AfterFunc` + SQLite persistence + always-on Machine + tired-proxy-style idle monitor | Reliable across the 90-180s window without paying for a fully always-on Machine. Idle monitor exits the process when truly idle AND no pending pushes; Fly stops the Machine; next request restarts it. |
| Cancel-on-edit semantics | Scheduling triggers on first-time completion only | Editing a previously completed set does not reschedule a push; it's not active work. |
| Last-set-in-workout | Skip scheduling | No next event to rest before; the chime would arrive after the user is done. |
| POC retention | Delete both `/dev/rest-timer-poc` and `/dev/web-push-poc` | Production code subsumes their purpose; git history is the archive. |

## Architecture

```
                    ┌─────────────────────────┐
   browser  ──HTTP──▶  cmd/web  (handlers)    │
                    │       │                  │
                    │       ▼                  │
                    │  internal/service        │
                    │       │ RecordSet etc.   │
                    │       ▼                  │
                    │  internal/notification ──┼── HTTPS ──▶  Apple/FCM/Mozilla
                    │   ├ Sender (webpush-go)  │
                    │   ├ Scheduler            │
                    │   └ IdleMonitor          │
                    │       │                  │
                    │       ▼                  │
                    │  internal/repository     │
                    │       │                  │
                    │       ▼                  │
                    │      SQLite              │
                    └─────────────────────────┘
```

### Components

**`internal/notification` (new package).** Owns push delivery logic.
Exports:

- `Sender` — wraps `webpush.SendNotification`. Single source of the `Subscriber` claim (a bare email, NOT `mailto:`-prefixed — webpush-go v1.4.0 prepends `mailto:` unconditionally; passing one in produces `mailto:mailto:foo@bar` and Apple returns `BadJwtToken`). Strips `410 Gone` / `404 Not Found` subscriptions from the repository on send failure.
- `Scheduler` — in-process map of `workout_exercise_id → *time.Timer`. Methods: `Schedule(ctx, userID, workoutExerciseID, fireAt, payload)`, `Cancel(ctx, workoutExerciseID)`, `CancelForWorkout(ctx, workoutSessionID)`, `Reload(ctx)`. All public methods are goroutine-safe. `Reload` is called once at process start: for each row, if `fire_at` is still in the future, builds a `time.Timer` for the remaining delta; if `fire_at` is already in the past (server was down longer than the rest window), dispatches immediately on a background goroutine — the user is probably still resting or just starting the next set, and a delayed ping is preferable to a silent miss.
- `IdleMonitor` — separate goroutine started in `main.go`. Tracks last HTTP request timestamp (updated via middleware) and a "pending pushes count" via `Scheduler.PendingCount()`. Every `tickInterval`, if `(now - lastRequest) > idleThreshold` AND `PendingCount() == 0`, sends `SIGTERM` to the process (`syscall.Kill(os.Getpid(), syscall.SIGTERM)`); existing graceful-shutdown handles the rest. Default thresholds: 5 min idle, 10 s tick.

**`internal/repository/pushsubscription.go` and `pushschedule.go` (new files).** Standard repo pattern: `Insert`, `DeleteByEndpoint`, `ListByUser`, `Replace(workoutExerciseID, ...)`, `ListPending(now)`, `DeleteByID`.

**`internal/service` integration.** `RecordSet` gains a notification side-effect: after the domain mutation succeeds (inside the existing repo Update closure or immediately after), if the set was newly completed and is not the last set in the session, call `Scheduler.Schedule(...)`. `CompleteWorkout` and `SwapExercise` call `Scheduler.CancelForWorkout` and `Scheduler.Cancel(workoutExerciseID)` respectively. The "is this the last set" check stays in `internal/domain` as a method on `Session` (e.g. `(*Session).HasIncompleteSetsAfter(workoutExerciseID, setIndex) bool`), per the existing pattern in `internal/domain/CLAUDE.md`.

**`cmd/web/handler-preferences.go` extension.** Add a "Rest notifications" section with three states:

1. Not in standalone PWA → inline guidance ("Add to Home Screen to enable").
2. Standalone, no subscription → "Enable" button → permission + subscribe + POST `/api/push/subscribe`.
3. Standalone, subscribed → status row + "Disable" (drops subscription server-side and locally) + opt-out checkbox.

**`cmd/web/handler-push.go` (new).** Endpoints:

- `POST /api/push/subscribe` — accept subscription JSON, upsert by endpoint, tie to authenticated user.
- `POST /api/push/unsubscribe` — delete subscriptions for current user (optionally filtered by endpoint).

Both `mustSessionStack`-wrapped; CSRF via Go 1.25 CrossOriginProtection (handled by stack).

**`ui/static/sw.js` (new).** Production service worker. Owns:

- `push` event → `self.registration.showNotification(title, options)`. Payload: `{title, body, fire_at_ms, set_index, exercise_name}`. Body includes "arrived ±Δms" only when `app.devMode` (compile-time toggle via fetched config or skipped entirely in prod).
- `notificationclick` → focus existing PWA window if present, else open `/workouts/<today>`.
- No `fetch` handler — we don't need offline caching; let requests go through the network.

Registered from `base.gohtml` unconditionally when `'serviceWorker' in navigator`, so it's installed early. Permission request and subscription only happen on explicit user action in `/preferences`.

**`ui/templates/pages/exerciseset/sets-container.gohtml` extension.** When the active set has a preceding completed set, render a `<div class="rest-chip" data-rest-end-at-ms="...">` above the form. Tiny inline script (already in the file or co-located) computes remaining seconds every 250ms; at 0, the chip swaps class to `.ready` and the text becomes "Ready".

**`fly.toml`.** `auto_stop_machines = "off"`, `auto_start_machines = true` (already true). Application-level idle monitor replaces Fly's autostop.

### Data flow: completing a set

1. User submits form → `POST /workouts/{date}/exercises/{id}/sets/{idx}/update`.
2. Handler parses, calls `service.RecordSet(ctx, ...)`.
3. Service: domain mutation in repo Update closure (atomic).
4. After commit: service derives `RestSeconds` from `DeriveScheme`, checks "is this the last set in the session", checks `workout_preferences.rest_notifications_enabled`, checks `push_subscriptions` is non-empty. If all green, calls `Scheduler.Schedule(userID, workoutExerciseID, completedAt + restSeconds, payload)`.
5. Scheduler: `INSERT INTO scheduled_pushes (...)`, `time.AfterFunc(restSeconds, dispatch)`. If an existing timer for the same `workout_exercise_id` exists, stop it and `DELETE` the row first (replace, not duplicate).
6. Handler redirects to the same exercise page.
7. New page renders with the active set card showing the inline countdown chip computed from `last_completed_at + RestSeconds`.

### Data flow: push firing

1. `time.AfterFunc` callback runs in scheduler goroutine.
2. Re-check `rest_notifications_enabled` (user may have flipped it off mid-rest).
3. Fetch all `push_subscriptions` rows for the user.
4. For each: `Sender.Send(payload, sub)`. On 410/404: delete that row.
5. `DELETE FROM scheduled_pushes WHERE id = ?`.
6. If `PendingCount()` is now 0 and the idle threshold has been reached, `IdleMonitor` may proceed with shutdown.

### Data flow: completing a workout

1. `service.CompleteWorkout(ctx, date)` runs domain mutation.
2. After commit: `Scheduler.CancelForWorkout(workoutSessionID)` — stops every `*time.Timer` for any `workout_exercise_id` belonging to this workout and `DELETE`s the rows.

### Data flow: swapping an exercise

1. `service.SwapExercise(ctx, ...)` runs domain mutation.
2. If the swap deletes and recreates the `workout_exercise` row, the FK `ON DELETE CASCADE` removes pending `scheduled_pushes` rows automatically; the scheduler's in-memory timer leaks until it fires (then no-ops on missing row).
3. If the swap updates in place, the service explicitly calls `Scheduler.Cancel(workoutExerciseID)`.

Both paths are correct; the explicit cancel is cheap insurance.

## Data model

```sql
CREATE TABLE push_subscriptions (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   TEXT    NOT NULL UNIQUE,
    p256dh     TEXT    NOT NULL,
    auth       TEXT    NOT NULL,
    created_at TEXT    NOT NULL
);

CREATE INDEX push_subscriptions_user_id ON push_subscriptions(user_id);

CREATE TABLE scheduled_pushes (
    id                   INTEGER PRIMARY KEY,
    user_id              INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workout_exercise_id  INTEGER NOT NULL REFERENCES workout_exercise(id) ON DELETE CASCADE,
    fire_at              TEXT    NOT NULL,
    payload              TEXT    NOT NULL,
    created_at           TEXT    NOT NULL
);

CREATE UNIQUE INDEX scheduled_pushes_workout_exercise_id
    ON scheduled_pushes(workout_exercise_id);
CREATE INDEX scheduled_pushes_fire_at ON scheduled_pushes(fire_at);

ALTER TABLE workout_preferences
    ADD COLUMN rest_notifications_enabled INTEGER NOT NULL DEFAULT 1;
```

The unique index on `workout_exercise_id` enforces the "one pending push per slot" invariant at the DB layer; the `Replace` repo method uses `INSERT ... ON CONFLICT(workout_exercise_id) DO UPDATE`.

## Configuration

| Env var | Purpose |
|---|---|
| `PETRAPP_VAPID_PUBLIC` | Base64url-encoded public key. Sent to client during subscribe. |
| `PETRAPP_VAPID_PRIVATE` | Base64url-encoded private key. Used to sign JWTs. |
| `PETRAPP_VAPID_SUBJECT` | Email address for the VAPID `sub` claim. Defaults to `vapid@example.com` (placeholder; real push services treat it as non-deliverable, so prod must override via Fly secret). **Bare email, no `mailto:` prefix.** |
| `PETRAPP_NOTIFICATION_IDLE_TIMEOUT` | Idle-monitor threshold, default `5m`. |

Dev script (`scripts/dev.sh`, `scripts/dev-tailscale-https.sh`) generates a fresh keypair via `webpush.GenerateVAPIDKeys` if `PETRAPP_VAPID_PUBLIC` is unset, exports both, and prints the public key so subscribed dev devices can be re-aligned across restarts.

## Onboarding UX (`/preferences`)

A new section, placed after the existing preferences groups:

```
Rest notifications
  Status: [not subscribed | subscribed (this device) | subscribed (this and N other device(s))]
  [Enable rest notifications] / [Disable on this device]
  [✓] Send notifications when rest is over
```

Detection rules:

- If not in standalone PWA (`!window.matchMedia('(display-mode: standalone)').matches && !window.navigator.standalone`): replace the Enable button with inline guidance: "On iOS, Add to Home Screen first, then open the installed app and try again." (No button rendered.)
- If `'Notification' in window` is false: section is hidden entirely.
- If subscribed: the opt-out checkbox is visible and reflects `rest_notifications_enabled`.

The subscribe flow follows the POC: `Notification.requestPermission` → `pushManager.subscribe({userVisibleOnly: true, applicationServerKey})` → `POST /api/push/subscribe` with the JSON. Errors surface as inline text in the section.

## Visual countdown

A new component on the active set card. Server-rendered markup:

```html
<div class="rest-chip" data-rest-end-at-ms="1715432000000" aria-live="polite">
    Rest 2:30
</div>
```

The handler computes `rest_end_at_ms` from `last_completed_set.completed_at + scheme.RestSeconds * 1000`. The template only renders the chip when:

- The active set is not the first set of the exercise, OR the warmup transition just happened
- `last_completed_at + RestSeconds > now()`

CSS lives in `sets-container.gohtml` alongside the existing `.exercise-set` styles. Inline `<script {{ nonce }}>` (≤ 30 lines) finds all `.rest-chip[data-rest-end-at-ms]`, ticks every 250 ms via `requestAnimationFrame`-backed `setInterval`, and switches to `.ready` class at 0.

Survives page reloads automatically because the chip is purely derived from server state.

## Error handling

- **VAPID env vars missing in production:** app fails fast on startup with a clear message. In dev, generates ephemeral keys and warns.
- **Permission denied:** subscribe button on `/preferences` surfaces the error inline ("Notification permission was denied. Re-enable in your device settings if you change your mind.").
- **`pushManager.subscribe` throws:** caught, logged, surfaced to the user with the exception name.
- **Push delivery 410/404:** subscription row deleted, no retry.
- **Push delivery 5xx:** logged with response body, no retry (next set will schedule a new push).
- **Push delivery times out (`webpush.SendNotification` returns transport error):** logged, no retry. Acceptable — the user's already in or out of the rest window.
- **`scheduled_pushes` row exists but corresponding `workout_exercise` is gone:** caught by the `time.AfterFunc` callback; logged at DEBUG and treated as a successful cancel.

## Testing

- **`internal/notification/sender_test.go`:** uses `httptest.Server` to impersonate Apple/FCM; verifies the JWT is built correctly (decode and inspect `aud`/`sub`/`exp`), `Subscriber` doesn't double-`mailto:`, 410 deletes the subscription, 5xx doesn't.
- **`internal/notification/scheduler_test.go`:** in-memory clock; verify `Schedule → Cancel`, `Schedule → fire`, `Schedule → Replace → only second one fires`, `Reload` reconstitutes timers.
- **`internal/notification/idle_monitor_test.go`:** mock pending-count + last-request signals; verify shutdown trigger conditions.
- **`internal/service/sessions_test.go` extension:** `RecordSet` schedules; `CompleteWorkout` cancels; last-set-in-session does not schedule.
- **`cmd/web/handler-push_test.go` (new):** subscribe/unsubscribe round-trip via e2etest.
- **`cmd/web/handler-exerciseset_test.go` extension:** verify `data-rest-end-at-ms` attribute present and matches scheme.
- **`cmd/web/playwright_test.go` extension:** countdown chip appears after a set submit, ticks down, transitions to "Ready". Push tests are out of scope for Playwright (browser-only).

`make ci` passes with the new tests.

## Cleanup

Remove in the same series of commits:

- `cmd/web/handler-rest-timer-poc.go`
- `cmd/web/handler-web-push-poc.go`
- `ui/templates/pages/rest-timer-poc/`
- `ui/templates/pages/web-push-poc/`
- `ui/static/sw-web-push-poc.js`
- The three `/dev/rest-timer-poc*` and three `/dev/web-push-poc*` routes in `cmd/web/routes.go`
- The two POC links in `ui/templates/pages/home/home.gohtml` dev banner (leaves `/dev/styleguide`)

`go mod tidy` is enough to retain `webpush-go` since the production code uses it.

## Acceptance

- A user can opt in via `/preferences` from inside the installed PWA on iPhone and Android (Chrome + Firefox).
- After a set is completed during a workout, a notification arrives within `RestSeconds + 2s` whether the PWA is foregrounded, backgrounded, or the phone is locked.
- The on-page countdown chip appears immediately on the active set card after submit and ticks down accurately.
- Submitting the next set early cancels the pending push (no late chime).
- Completing the workout cancels all pending pushes.
- The Fly Machine stops within `idle_threshold + tick_interval` after the last request, provided no pending pushes are queued.
- `make ci` passes.

## Out of scope

- **"Skip rest" notification action.** Android-only; deferred until requested.
- **Per-exercise / per-user rest override.** Spec keeps `DeriveScheme` as the single source of `RestSeconds`. Customisation is a future feature.
- **Cross-device rest sync.** Each device fires its own notification based on subscription; no consolidation.
- **Web Push for non-rest events** (workout reminders, weekly summaries). The plumbing supports it; the wiring is out of scope.
- **Reactivating dropped subscriptions.** If a subscription is 410'd, the user must re-subscribe via `/preferences` manually.
- **A Notification Bell / unread list in-app.** The notification *is* the surface; no in-app inbox.
