# Start Deload Early — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Start deload this week" button in Preferences → Recovery that flips the current week's forward-looking, non-completed sessions to deload mode and snaps the mesocycle anchor to next Monday. Extend the existing "Restart cycle next Monday" button to clear the same flips, so the action is fully reversible.

**Architecture:** New aggregate methods `Session.SwitchToDeload` / `Session.ClearDeload` carry the state change. New service method `Service.StartDeloadNow` orchestrates the per-session updates via the existing `repos.Sessions.Update` closure pattern (each closure is its own transaction; the closure re-checks `Status() != SessionCompleted` to make the loop race-free without a global mutex). `Service.RestartMesocycleAnchor` is extended to clear the same set of sessions. A new POST handler and form wire the button into the existing Preferences page.

**Tech Stack:** Go (standard library + `internal/domain`, `internal/service`, `internal/repository`), SQLite via `internal/sqlite`, `html/template` Go templates, e2etest for handler tests, goquery for HTML assertions.

**Reference:** `docs/superpowers/specs/2026-05-24-start-deload-early-design.md`.

---

## File Structure

**Modified files** (one responsibility each):

- `internal/domain/planner.go` — add `StartOfDay` helper next to `MondayOf`.
- `internal/domain/session.go` — add `SwitchToDeload` and `ClearDeload` aggregate methods on `*Session`.
- `internal/domain/session_test.go` — tests for the two new methods.
- `internal/service/sessions.go` — add `StartDeloadNow` orchestrator.
- `internal/service/service.go` — extend `RestartMesocycleAnchor` to also clear current-week deload flips.
- `internal/service/sessions_test.go` — tests for `StartDeloadNow`.
- `internal/service/service_test.go` — tests for the extended `RestartMesocycleAnchor` behaviour.
- `cmd/web/handler-preferences.go` — add `preferencesStartDeloadNowPOST`.
- `cmd/web/handler-preferences_test.go` — handler test exercising the new form via e2etest.
- `cmd/web/routes.go` — register `POST /preferences/mesocycle/start-deload-now`.
- `ui/templates/pages/preferences/preferences.gohtml` — add the new form to the Recovery panel.

No new files. Each modification is scoped to a single responsibility within its file.

---

## Task 1: Add `StartOfDay` helper in domain

**Files:**
- Modify: `internal/domain/planner.go` (add immediately after `MondayOf` near line 504)
- Test: `internal/domain/planner_test.go` (or wherever `MondayOf` is tested — check first)

- [ ] **Step 1: Locate `MondayOf` test (if any)**

Run: `grep -n "TestMondayOf\|Test_MondayOf" internal/domain/*_test.go`

If a test file already covers `MondayOf`, add the `StartOfDay` test there. Otherwise add a new test file `internal/domain/planner_external_test.go` using package `domain_test`.

- [ ] **Step 2: Write the failing test**

Add to the chosen test file:

```go
func Test_StartOfDay_TruncatesToUTCMidnight(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}

	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "UTC noon collapses to UTC midnight",
			in:   time.Date(2026, 5, 24, 12, 30, 0, 0, time.UTC),
			want: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "UTC midnight is fixed point",
			in:   time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Local late-evening uses local calendar date",
			in:   time.Date(2026, 5, 24, 23, 30, 0, 0, loc),
			want: time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := domain.StartOfDay(tc.in)
			if !got.Equal(tc.want) {
				t.Errorf("StartOfDay(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test -v ./internal/domain -run Test_StartOfDay_TruncatesToUTCMidnight`

Expected: FAIL with `undefined: domain.StartOfDay`.

- [ ] **Step 4: Add `StartOfDay` to `internal/domain/planner.go`**

Insert immediately after the `MondayOf` function (after line 504):

```go
// StartOfDay returns the UTC midnight of date's calendar day. Mirrors
// MondayOf's UTC-anchored-but-calendar-date-from-local behaviour so the
// result compares cleanly against session dates loaded from the database
// (which time.Parse always returns in UTC).
func StartOfDay(date time.Time) time.Time {
	y, m, d := date.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -v ./internal/domain -run Test_StartOfDay_TruncatesToUTCMidnight`

Expected: PASS.

- [ ] **Step 6: Run the full domain test suite to confirm no regressions**

Run: `go test ./internal/domain`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/planner.go internal/domain/planner_external_test.go internal/domain/planner_test.go
git commit -m "feat(domain): add StartOfDay UTC-midnight helper

Mirrors MondayOf's UTC-anchored-from-local behaviour for the new
StartDeloadNow flow that compares session dates against today."
```

(Only stage the test file you actually modified.)

---

## Task 2: Add `Session.SwitchToDeload` aggregate method

**Files:**
- Modify: `internal/domain/session.go` (add near the existing aggregate methods, e.g. after `SetDifficulty` near line 137)
- Test: `internal/domain/session_test.go` (add at the end)

- [ ] **Step 1: Write the failing test**

Append to `internal/domain/session_test.go`:

```go
func Test_Session_SwitchToDeload_SetsFlag(t *testing.T) {
	t.Parallel()

	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date: time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
	}

	if err := sess.SwitchToDeload(); err != nil {
		t.Fatalf("SwitchToDeload: %v", err)
	}
	if !sess.IsDeload {
		t.Error("SwitchToDeload did not set IsDeload to true")
	}
}

func Test_Session_SwitchToDeload_Idempotent(t *testing.T) {
	t.Parallel()

	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date:     time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
		IsDeload: true,
	}

	if err := sess.SwitchToDeload(); err != nil {
		t.Fatalf("SwitchToDeload (already deload): %v", err)
	}
	if !sess.IsDeload {
		t.Error("SwitchToDeload cleared IsDeload on already-deload session")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./internal/domain -run Test_Session_SwitchToDeload`

Expected: FAIL with `sess.SwitchToDeload undefined`.

- [ ] **Step 3: Add `SwitchToDeload` to `internal/domain/session.go`**

Insert immediately after the `SetDifficulty` method (after line 137):

```go
// SwitchToDeload marks the session as a deload session going forward.
// Sets recorded prior to this call retain their stored values; the next
// progression recommendation will be derived from GetDeloadStartingWeight
// rather than GetStartingWeight. Idempotent.
func (s *Session) SwitchToDeload() error {
	s.IsDeload = true
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/domain -run Test_Session_SwitchToDeload`

Expected: PASS for both subtests.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/session.go internal/domain/session_test.go
git commit -m "feat(domain): add Session.SwitchToDeload aggregate method"
```

---

## Task 3: Add `Session.ClearDeload` aggregate method

**Files:**
- Modify: `internal/domain/session.go` (add immediately after `SwitchToDeload`)
- Test: `internal/domain/session_test.go` (add at the end)

- [ ] **Step 1: Write the failing tests**

Append to `internal/domain/session_test.go`:

```go
func Test_Session_ClearDeload_ClearsFlag(t *testing.T) {
	t.Parallel()

	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date:     time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
		IsDeload: true,
	}

	if err := sess.ClearDeload(); err != nil {
		t.Fatalf("ClearDeload: %v", err)
	}
	if sess.IsDeload {
		t.Error("ClearDeload did not set IsDeload to false")
	}
}

func Test_Session_ClearDeload_Idempotent(t *testing.T) {
	t.Parallel()

	sess := domain.Session{ //nolint:exhaustruct // Test sessions omit irrelevant fields.
		Date: time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC),
	}

	if err := sess.ClearDeload(); err != nil {
		t.Fatalf("ClearDeload (already clear): %v", err)
	}
	if sess.IsDeload {
		t.Error("ClearDeload toggled IsDeload to true on already-clear session")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./internal/domain -run Test_Session_ClearDeload`

Expected: FAIL with `sess.ClearDeload undefined`.

- [ ] **Step 3: Add `ClearDeload` to `internal/domain/session.go`**

Insert immediately after `SwitchToDeload`:

```go
// ClearDeload marks the session as a non-deload session going forward.
// Counterpart to SwitchToDeload; used by RestartMesocycleAnchor to undo
// an ad-hoc early deload. Idempotent.
func (s *Session) ClearDeload() error {
	s.IsDeload = false
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/domain -run Test_Session_ClearDeload`

Expected: PASS for both subtests.

- [ ] **Step 5: Run the full domain suite**

Run: `go test ./internal/domain`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/session.go internal/domain/session_test.go
git commit -m "feat(domain): add Session.ClearDeload aggregate method"
```

---

## Task 4: Implement `Service.StartDeloadNow`

**Files:**
- Modify: `internal/service/sessions.go` (add new method at the end of the file)
- Test: `internal/service/sessions_test.go` (add new tests at the end)

This task uses `setupTestService` (`internal/service/helpers_test.go:17`) which creates a Mon/Wed/Fri preference baseline. We force deload-enabled prefs with a known anchor in each test so the planner produces consistent sessions.

- [ ] **Step 1: Write the first failing test — flips today + future, leaves past untouched**

Append to `internal/service/sessions_test.go`:

```go
func Test_StartDeloadNow_FlipsTodayAndFutureNonCompletedSessions(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t) // Mon/Wed/Fri 60 min

	// Enable deload so the button is permissible; anchor it to this week's
	// Monday so the planner treats this week as accumulation week 0 (not a
	// natural deload week).
	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}

	// Materialise the week's sessions.
	if _, err = svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	// Sanity: no session should be a natural-cadence deload yet.
	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule (re-list): %v", err)
	}
	for i, s := range sessions {
		if s.IsDeload {
			t.Fatalf("session[%d] (%s) unexpectedly already deload", i, s.Date.Weekday())
		}
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow: %v", err)
	}

	sessions, err = svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after StartDeloadNow: %v", err)
	}

	today := domain.StartOfDay(time.Now())
	for i, s := range sessions {
		if len(s.ExerciseSets) == 0 {
			continue // rest day
		}
		isForwardLooking := !s.Date.Before(today)
		if isForwardLooking && !s.IsDeload {
			t.Errorf("session[%d] (%s, %s) should be deload (today or later, not completed)",
				i, s.Date.Weekday(), s.Date.Format(time.DateOnly))
		}
		if !isForwardLooking && s.IsDeload {
			t.Errorf("session[%d] (%s, %s) should NOT be deload (past)",
				i, s.Date.Weekday(), s.Date.Format(time.DateOnly))
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service -run Test_StartDeloadNow_FlipsTodayAndFutureNonCompletedSessions`

Expected: FAIL with `svc.StartDeloadNow undefined`.

- [ ] **Step 3: Implement `StartDeloadNow` in `internal/service/sessions.go`**

Append to the file:

```go
// StartDeloadNow flips IsDeload to true on every current-week session
// dated today or later that is not already fully completed, then snaps
// the mesocycle anchor to next Monday. Used when the user needs
// recovery off-schedule (e.g. returning from sickness). Undone by
// RestartMesocycleAnchor, which clears the same set of flips.
//
// No mutex: each per-session Update is its own transaction, the closure
// re-checks Status() != SessionCompleted before mutating, and a
// double-press is idempotent.
func (s *Service) StartDeloadNow(ctx context.Context) error {
	monday := domain.MondayOf(time.Now())
	today := domain.StartOfDay(time.Now())

	sessions, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return fmt.Errorf("list sessions for current week: %w", err)
	}

	for _, sess := range sessions {
		if sess.Date.Before(today) {
			continue
		}
		err = s.repos.Sessions.Update(ctx, sess.Date, func(latest *domain.Session) error {
			if latest.Status() == domain.SessionCompleted {
				return nil
			}
			return latest.SwitchToDeload()
		})
		if err != nil {
			return fmt.Errorf("flip deload for %s: %w", sess.Date.Format(time.DateOnly), err)
		}
	}

	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
	if err = s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/service -run Test_StartDeloadNow_FlipsTodayAndFutureNonCompletedSessions`

Expected: PASS.

- [ ] **Step 5: Add anchor-snap test**

Append:

```go
func Test_StartDeloadNow_SnapsAnchorToNextMonday(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	if _, err = svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow: %v", err)
	}

	got, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences after StartDeloadNow: %v", err)
	}
	// The next Monday must be strictly in the future (since the test
	// runs at some instant after this week's Monday).
	if !got.MesocycleAnchor.After(monday) {
		t.Errorf("MesocycleAnchor = %v; want a Monday strictly after %v",
			got.MesocycleAnchor, monday)
	}
	if got.MesocycleAnchor.Weekday() != time.Monday {
		t.Errorf("MesocycleAnchor weekday = %v; want Monday", got.MesocycleAnchor.Weekday())
	}
	// Must be exactly one Monday ahead of monday (the week boundary).
	if !got.MesocycleAnchor.Equal(monday.AddDate(0, 0, 7)) {
		t.Errorf("MesocycleAnchor = %v; want %v (next Monday)",
			got.MesocycleAnchor, monday.AddDate(0, 0, 7))
	}
}
```

- [ ] **Step 6: Run anchor-snap test**

Run: `go test -v ./internal/service -run Test_StartDeloadNow_SnapsAnchorToNextMonday`

Expected: PASS.

- [ ] **Step 7: Add idempotency test**

Append:

```go
func Test_StartDeloadNow_Idempotent(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	if _, err = svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow first call: %v", err)
	}
	first, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after first: %v", err)
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow second call: %v", err)
	}
	second, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after second: %v", err)
	}

	for i := range first {
		if first[i].IsDeload != second[i].IsDeload {
			t.Errorf("session[%d] IsDeload flipped between calls: %v -> %v",
				i, first[i].IsDeload, second[i].IsDeload)
		}
	}
}
```

- [ ] **Step 8: Run idempotency test**

Run: `go test -v ./internal/service -run Test_StartDeloadNow_Idempotent`

Expected: PASS.

- [ ] **Step 9: Run the full service suite**

Run: `go test ./internal/service`

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/service/sessions.go internal/service/sessions_test.go
git commit -m "feat(service): add StartDeloadNow orchestrator

Flips IsDeload on current-week sessions dated today or later that are
not yet fully completed, then snaps the mesocycle anchor to next
Monday. No mutex: each per-session Update is transactional and the
closure re-checks Status() != SessionCompleted before mutating."
```

---

## Task 5: Cross-layer test — `BuildProgression` returns deload weight after `StartDeloadNow`

This is the critical end-to-end behavioural assertion called out in the spec ("displayed weight switches to the deload recommendation automatically on the next page render"). It belongs in the service layer because that is where `BuildProgression` lives.

**Files:**
- Test: `internal/service/sessions_test.go` (append)

- [ ] **Step 1: Inspect an existing `BuildProgression` test to copy its weight-history setup**

Run: `grep -n "func Test_BuildProgression\|GetDeloadStartingWeight" internal/service/progression_test.go | head`

Open `internal/service/progression_test.go` around `Test_BuildProgression_CurrentSetUsesDeriveScheme` (which sets up hypertrophy history and asserts `GetDeloadStartingWeight`). Mirror that setup pattern (history-seeding helpers, exercise creation) for the new test.

- [ ] **Step 2: Write the failing test**

Append to `internal/service/sessions_test.go` (or to `progression_test.go` if the helpers used in step 1 are file-local — pick the file where the helpers are defined):

```go
func Test_StartDeloadNow_BuildProgressionReturnsDeloadWeight(t *testing.T) {
	t.Parallel()

	// 1) Set up a service with hypertrophy history that GetDeloadStartingWeight
	//    can read. Copy the exact arrangement from Test_BuildProgression_*
	//    in progression_test.go: register exercise, insert prior completed
	//    sets via a hypertrophy session before the current week.
	//
	//    The setup is non-trivial; replicate it verbatim from the existing
	//    test rather than improvising. Specifically:
	//      - Use setupTestService.
	//      - Insert an exercise.
	//      - Seed a prior hypertrophy session with completed sets at a known
	//        working weight (e.g. 80 kg).
	//      - Schedule the current week so a session exists today/tomorrow
	//        for that same exercise.

	// 2) Capture the non-deload BuildProgression result for a forward-looking
	//    session today/tomorrow (call svc.BuildProgression(ctx, date, exID)).
	//    Record the recommended WeightKg.

	// 3) Call svc.StartDeloadNow(ctx).

	// 4) Call svc.BuildProgression for the same date+exercise again.
	//    Assert the new recommended WeightKg equals
	//    svc.GetDeloadStartingWeight(ctx, exID, date) and is strictly less
	//    than the pre-flip value.

	t.Skip("replace this skeleton with the copy-from-progression_test setup once located")
}
```

**Why a skeleton:** the history-seeding shape is verbose and lives in `progression_test.go`. Rather than guess at it, mirror the existing test's setup verbatim. Remove the `t.Skip` once filled in.

- [ ] **Step 3: Fill in the skeleton using the same setup as `Test_BuildProgression_CurrentSetUsesDeriveScheme`**

Open `internal/service/progression_test.go`, locate that test, copy its arrangement (exercise creation, prior-session seeding) into the new test, then perform the four numbered steps above. Remove the `t.Skip`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -v ./internal/service -run Test_StartDeloadNow_BuildProgressionReturnsDeloadWeight`

Expected: PASS. (If it fails before any code change in `sessions.go`, the test is mis-arranged — the implementation from Task 4 is already in place.)

- [ ] **Step 5: Commit**

```bash
git add internal/service/sessions_test.go internal/service/progression_test.go
git commit -m "test(service): assert BuildProgression flips to deload weight after StartDeloadNow"
```

(Stage only files you actually modified.)

---

## Task 6: Extend `RestartMesocycleAnchor` to clear current-week deload flips

**Files:**
- Modify: `internal/service/service.go` (replace existing `RestartMesocycleAnchor` near lines 108-120)
- Test: `internal/service/service_test.go` (append)

- [ ] **Step 1: Write the failing test — full undo round-trip**

Append to `internal/service/service_test.go`:

```go
func Test_RestartMesocycleAnchor_ClearsCurrentWeekDeloadAfterStartDeloadNow(t *testing.T) {
	t.Parallel()

	ctx, svc := setupTestService(t)

	prefs, err := svc.GetUserPreferences(ctx)
	if err != nil {
		t.Fatalf("GetUserPreferences: %v", err)
	}
	monday := domain.MondayOf(time.Now())
	prefs.DeloadEnabled = true
	prefs.MesocycleLength = 5
	prefs.MesocycleAnchor = monday
	if err = svc.SaveUserPreferences(ctx, prefs); err != nil {
		t.Fatalf("SaveUserPreferences: %v", err)
	}
	if _, err = svc.ResolveWeeklySchedule(ctx); err != nil {
		t.Fatalf("ResolveWeeklySchedule: %v", err)
	}

	if err = svc.StartDeloadNow(ctx); err != nil {
		t.Fatalf("StartDeloadNow: %v", err)
	}
	if err = svc.RestartMesocycleAnchor(ctx); err != nil {
		t.Fatalf("RestartMesocycleAnchor: %v", err)
	}

	sessions, err := svc.ResolveWeeklySchedule(ctx)
	if err != nil {
		t.Fatalf("ResolveWeeklySchedule after restart: %v", err)
	}

	today := domain.StartOfDay(time.Now())
	for i, s := range sessions {
		if len(s.ExerciseSets) == 0 {
			continue
		}
		if !s.Date.Before(today) && s.IsDeload {
			t.Errorf("session[%d] (%s) should be cleared after restart, still IsDeload",
				i, s.Date.Weekday())
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service -run Test_RestartMesocycleAnchor_ClearsCurrentWeekDeloadAfterStartDeloadNow`

Expected: FAIL — the existing `RestartMesocycleAnchor` only snaps the anchor; current-week sessions remain `IsDeload == true`.

- [ ] **Step 3: Replace `RestartMesocycleAnchor` in `internal/service/service.go`**

Locate the current implementation at `internal/service/service.go:108-120` and replace it with:

```go
// RestartMesocycleAnchor snaps the mesocycle anchor to the next Monday,
// effectively restarting the deload cycle from that date. Additionally
// clears IsDeload on every current-week session dated today or later
// that is not already fully completed, so an accidental StartDeloadNow
// press can be fully undone with one click.
func (s *Service) RestartMesocycleAnchor(ctx context.Context) error {
	monday := domain.MondayOf(time.Now())
	today := domain.StartOfDay(time.Now())

	sessions, err := s.repos.Sessions.List(ctx, monday)
	if err != nil {
		return fmt.Errorf("list sessions for current week: %w", err)
	}
	for _, sess := range sessions {
		if sess.Date.Before(today) {
			continue
		}
		err = s.repos.Sessions.Update(ctx, sess.Date, func(latest *domain.Session) error {
			if latest.Status() == domain.SessionCompleted {
				return nil
			}
			return latest.ClearDeload()
		})
		if err != nil {
			return fmt.Errorf("clear deload for %s: %w", sess.Date.Format(time.DateOnly), err)
		}
	}

	prefs, err := s.repos.Preferences.Get(ctx)
	if err != nil {
		return fmt.Errorf("get preferences: %w", err)
	}
	prefs.MesocycleAnchor = nextMonday(time.Now().UTC())
	if err = s.repos.Preferences.Set(ctx, prefs); err != nil {
		return fmt.Errorf("save preferences: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run the new test to verify it passes**

Run: `go test -v ./internal/service -run Test_RestartMesocycleAnchor_ClearsCurrentWeekDeloadAfterStartDeloadNow`

Expected: PASS.

- [ ] **Step 5: Run the full service suite to catch regressions in existing `RestartMesocycleAnchor` tests**

Run: `go test ./internal/service`

Expected: PASS. If a pre-existing test asserted that `RestartMesocycleAnchor` does *not* touch session rows, update its expectation in the same commit and document the change in the commit message.

- [ ] **Step 6: Commit**

```bash
git add internal/service/service.go internal/service/service_test.go
git commit -m "feat(service): RestartMesocycleAnchor also clears current-week deload flips

Makes the existing Restart button the canonical undo for the new
StartDeloadNow flow. Clears IsDeload on forward-looking,
non-completed current-week sessions; behaviour for already-completed
sessions and past dates is unchanged."
```

---

## Task 7: Register the new route

**Files:**
- Modify: `cmd/web/routes.go` (insert next to the existing `/preferences/mesocycle/restart` registration near line 44)

- [ ] **Step 1: Add the route registration**

In `cmd/web/routes.go`, immediately after the existing line:

```go
mux.Handle("POST /preferences/mesocycle/restart",
    app.mustSessionStack(http.HandlerFunc(app.preferencesRestartMesocyclePOST)))
```

add:

```go
mux.Handle("POST /preferences/mesocycle/start-deload-now",
    app.mustSessionStack(http.HandlerFunc(app.preferencesStartDeloadNowPOST)))
```

- [ ] **Step 2: Confirm the file does not yet compile (handler is missing)**

Run: `go build ./cmd/web`

Expected: FAIL with `app.preferencesStartDeloadNowPOST undefined`. This validates the route is wired to the correct symbol; the next task supplies the handler.

- [ ] **Step 3: Do not commit yet**

The route alone won't compile. Hold the change uncommitted until Task 8 lands the handler; they commit together.

---

## Task 8: Implement the handler and form, ship together with the route

**Files:**
- Modify: `cmd/web/handler-preferences.go` (append new handler at the end)
- Modify: `ui/templates/pages/preferences/preferences.gohtml` (add new form inside Recovery panel, between cycle-length form and restart-cycle form)
- Test: `cmd/web/handler-preferences_test.go` (append a new top-level test function)

- [ ] **Step 1: Add the handler in `cmd/web/handler-preferences.go`**

Append to the file:

```go
func (app *application) preferencesStartDeloadNowPOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}
	if err := app.service.StartDeloadNow(r.Context()); err != nil {
		app.serverError(w, r, fmt.Errorf("start deload now: %w", err))
		return
	}
	redirect(w, r, "/preferences")
}
```

- [ ] **Step 2: Add the form in `ui/templates/pages/preferences/preferences.gohtml`**

Find the Recovery panel section that begins with `<section class="panel" aria-labelledby="deload-title">`. Inside, locate the existing restart form:

```gohtml
<form method="post" action="/preferences/mesocycle/restart" class="panel-actions">
    <button type="submit" class="btn btn--ghost btn--block" {{ if not .DeloadEnabled }}disabled{{ end }}>
        Restart cycle next Monday
    </button>
</form>
```

Insert immediately *before* it:

```gohtml
<form method="post" action="/preferences/mesocycle/start-deload-now" class="panel-actions">
    <button type="submit" class="btn btn--ghost btn--block"
            {{ if not .DeloadEnabled }}disabled{{ end }}>
        Start deload this week
    </button>
</form>
```

So the final order inside the panel is: deload-enable toggle → cycle-length select → anchor note → **Start deload this week** → Restart cycle next Monday.

- [ ] **Step 3: Build to confirm everything compiles together**

Run: `go build ./...`

Expected: PASS.

- [ ] **Step 4: Write a handler test in `cmd/web/handler-preferences_test.go`**

Append a new top-level test function. The test must (a) register a user, (b) enable deload in preferences via `SubmitForm` on `/preferences`, (c) submit the new form, (d) assert the redirect lands back on `/preferences` and the page renders, (e) assert the button is rendered enabled when deload is on and disabled when deload is off.

```go
func Test_application_preferencesStartDeloadNow(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 1) Visit /preferences with deload OFF: button must be disabled.
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc /preferences: %v", err)
	}
	startBtn := doc.Find("button:contains('Start deload this week')")
	if startBtn.Length() == 0 {
		t.Fatal("Start deload button not found on /preferences")
	}
	if _, ok := startBtn.Attr("disabled"); !ok {
		t.Error("Start deload button should be disabled when DeloadEnabled is false")
	}

	// 2) Enable deload + at least one workout day so the prefs form validates.
	doc, err = client.SubmitForm(ctx, doc, "/preferences", map[string]string{
		"monday_minutes":   "60",
		"deload_enabled":   "on",
		"mesocycle_length": "5",
	})
	if err != nil {
		t.Fatalf("SubmitForm enable deload: %v", err)
	}

	// 3) Re-fetch /preferences and confirm the button is now enabled.
	doc, err = client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc /preferences (after enable): %v", err)
	}
	startBtn = doc.Find("button:contains('Start deload this week')")
	if startBtn.Length() == 0 {
		t.Fatal("Start deload button missing after enabling deload")
	}
	if _, ok := startBtn.Attr("disabled"); ok {
		t.Error("Start deload button should be enabled when DeloadEnabled is true")
	}

	// 4) Submit the form; expect a redirect back to /preferences that renders.
	doc, err = client.SubmitForm(ctx, doc, "/preferences/mesocycle/start-deload-now", nil)
	if err != nil {
		t.Fatalf("SubmitForm start-deload-now: %v", err)
	}
	if got := doc.Find("h2#deload-title").Text(); got != "Deload cycles" {
		t.Errorf("expected Recovery panel heading after redirect, got %q", got)
	}
}
```

- [ ] **Step 5: Run the new handler test**

Run: `go test -v ./cmd/web -run Test_application_preferencesStartDeloadNow`

Expected: PASS.

- [ ] **Step 6: Run the full web suite**

Run: `go test ./cmd/web`

Expected: PASS. If `Test_application_preferences` (the broad pre-existing test) trips on the new button, investigate whether its DOM assertion accidentally matched the new copy and adjust the selector to be more specific.

- [ ] **Step 7: Lint**

Run: `make lint-fix`

Expected: PASS.

- [ ] **Step 8: Commit handler + route + template + test together**

```bash
git add cmd/web/routes.go cmd/web/handler-preferences.go cmd/web/handler-preferences_test.go ui/templates/pages/preferences/preferences.gohtml
git commit -m "feat(web): add Start deload this week button to Preferences

Posts to /preferences/mesocycle/start-deload-now; gated by
DeloadEnabled. The handler delegates to Service.StartDeloadNow.
Undo path is the existing 'Restart cycle next Monday' button,
which now also clears the deload flips."
```

---

## Task 9: Full CI gate

**Files:** none modified.

- [ ] **Step 1: Run the full validation pipeline**

Run: `make ci`

Expected: PASS (init + build + lint-fix + test + sec).

- [ ] **Step 2: If `make ci` modifies any files via lint-fix, commit them**

Run: `git status` to inspect. If the lint-fix step rewrote anything:

```bash
git add -p   # review hunks
git commit -m "chore: apply lint-fix after early-deload changes"
```

- [ ] **Step 3: Manually exercise the flow (optional but recommended)**

Start the app, register, enable deload, generate the week (visit `/`), then press "Start deload this week" in `/preferences`. Open a workout for today/tomorrow and confirm the displayed recommended weight is lower than before (the deload weight from `GetDeloadStartingWeight`). Press "Restart cycle next Monday" and confirm the recommended weight reverts to the normal progression weight.

---

## Self-Review Notes

- **Spec coverage:** every requirement in `docs/superpowers/specs/2026-05-24-start-deload-early-design.md` is addressed.
  - StartOfDay helper → Task 1.
  - SwitchToDeload / ClearDeload aggregate methods → Tasks 2, 3.
  - StartDeloadNow orchestrator (no mutex, status re-check inside closure) → Task 4.
  - BuildProgression returns deload weight after flip (the key end-to-end assertion) → Task 5.
  - RestartMesocycleAnchor extended as canonical undo → Task 6.
  - Route registration with `mustSessionStack` → Task 7.
  - Handler, form (gated by `DeloadEnabled`, no confirm), e2etest → Task 8.
  - Lint + full CI gate → Task 9.
- **Placeholder scan:** Task 5 ships an explicit skeleton because the history-seeding setup is verbose and lives elsewhere in the codebase; step 3 of that task directs the implementer to mirror an existing test verbatim rather than improvise. All other tasks contain complete code.
- **Type consistency:** `SwitchToDeload`, `ClearDeload`, `StartDeloadNow`, `StartOfDay` are used with identical names and signatures in every task that references them. `preferencesStartDeloadNowPOST` matches the handler/route registration. `MondayOf` and `nextMonday` are used as they exist in the codebase.
