# Stack Navigator: Push by Default Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Flip the stack navigator's default form-submit behavior from pop-or-replace to pop-or-push, fix the "Start Workout drops home from history" bug, and introduce an opt-in `X-Replace-URL` response header for the new add-exercise UX (land the user on the just-added exercise's detail page with `/add-exercise` erased from history).

**Architecture:** A small wire-protocol addition (`X-Replace-URL: true` response header), one new Go helper (`redirectReplace`), one client-side branch in `main.js` (replace before pop-search), one service signature change (`AddExercise` returns the new slot ID), updated Playwright coverage, and a CLAUDE.md doc refresh.

**Tech Stack:** Go (stdlib `net/http`, `http.ServeMux`), plain JS using the Navigation API, Playwright for E2E, SQLite (no schema change needed).

**Spec:** `docs/superpowers/specs/2026-05-03-stack-navigator-push-default-design.md`

---

## File Structure

| File | Change |
|---|---|
| `cmd/web/helpers.go` | Add `redirectReplace` helper (alongside existing `redirect`). |
| `cmd/web/helpers_test.go` | Add two tests for `redirectReplace` (stacknav + plain paths). |
| `internal/workout/service.go` | Change `AddExercise` return type from `error` to `(int, error)` — the new slot ID. After successful update, the service fetches the session to find the just-added slot's ID. |
| `internal/workout/service_test.go` | Update existing `AddExercise` callers (`err = svc.AddExercise(...)` → `_, err = svc.AddExercise(...)`); add a new focused test asserting the returned ID matches the new slot. |
| `cmd/web/handler-workout.go` | `workoutAddExercisePOST` captures the returned ID and redirects to the new DETAIL via `redirectReplace`. |
| `ui/static/main.js` | Rename `popOrReplaceTo` → `popOrPushTo`; add a `replace` option that handles same-URL or server-flagged replace before the backward-walk loop; `submitForm` reads `X-Replace-URL`. Header doc comment updated. |
| `cmd/web/playwright_test.go` | Update `addExerciseToWorkout` helper to handle the new redirect target; rewrite Flow 1/2/3/5 comments to reflect new mechanism (assertions stay); add a new Flow 6 (add-exercise replace) at the end of `Test_playwright_stacknav`. |
| `cmd/web/CLAUDE.md` | Rewrite "Redirects and Navigation" section: document both helpers, update client-behavior summary, point at the new spec. |

No SQL schema changes. No new dependencies.

---

## Task 1: Add `redirectReplace` helper

**Files:**
- Modify: `cmd/web/helpers.go`
- Test: `cmd/web/helpers_test.go`

- [ ] **Step 1: Write the failing tests**

Add the two tests below to `cmd/web/helpers_test.go`. Add them after the existing `Test_redirect_PlainRequest_Returns303SeeOther` function (i.e., append at the end of the file).

```go
func Test_redirectReplace_StackNavRequest_SetsXLocationAndXReplaceURL(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)
	r.Header.Set("X-Requested-With", "stacknav")

	redirectReplace(w, r, "/target")

	if got := w.Code; got != http.StatusOK {
		t.Errorf("status = %d, want %d", got, http.StatusOK)
	}
	if got := w.Header().Get("X-Location"); got != "/target" {
		t.Errorf("X-Location = %q, want %q", got, "/target")
	}
	if got := w.Header().Get("X-Replace-URL"); got != "true" {
		t.Errorf("X-Replace-URL = %q, want %q", got, "true")
	}
	if got := w.Header().Get("Location"); got != "" {
		t.Errorf("Location should not be set for stacknav request, got %q", got)
	}
	if got := w.Body.Len(); got != 0 {
		t.Errorf("body length = %d, want 0", got)
	}
}

func Test_redirectReplace_PlainRequest_Returns303SeeOtherWithoutXReplace(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/whatever", nil)

	redirectReplace(w, r, "/target")

	if got := w.Code; got != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", got, http.StatusSeeOther)
	}
	if got := w.Header().Get("Location"); got != "/target" {
		t.Errorf("Location = %q, want %q", got, "/target")
	}
	if got := w.Header().Get("X-Replace-URL"); got != "" {
		t.Errorf("X-Replace-URL should not be set for plain request, got %q", got)
	}
	if got := w.Header().Get("X-Location"); got != "" {
		t.Errorf("X-Location should not be set for plain request, got %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -run "Test_redirectReplace" -v ./cmd/web/ 2>&1 | head -30`

Expected: build failure with `undefined: redirectReplace`.

- [ ] **Step 3: Implement `redirectReplace`**

Append the new helper to `cmd/web/helpers.go`, immediately after the existing `redirect` function (around line 41):

```go
// redirectReplace works like redirect, but signals to the stack navigator
// that the current history entry should be replaced. Use this for form
// pages whose existence should be erased on submit (e.g. /add-exercise).
// Non-stacknav callers fall through to a plain 303, identical to redirect.
func redirectReplace(w http.ResponseWriter, r *http.Request, path string) {
	if r.Header.Get("X-Requested-With") == "stacknav" {
		w.Header().Set("X-Location", path)
		w.Header().Set("X-Replace-URL", "true")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, path, http.StatusSeeOther)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -run "Test_redirect" -v ./cmd/web/`

Expected: all four tests pass (`Test_redirect_StackNavRequest_Returns200WithXLocation`, `Test_redirect_PlainRequest_Returns303SeeOther`, `Test_redirectReplace_StackNavRequest_SetsXLocationAndXReplaceURL`, `Test_redirectReplace_PlainRequest_Returns303SeeOtherWithoutXReplace`).

- [ ] **Step 5: Commit**

```bash
git add cmd/web/helpers.go cmd/web/helpers_test.go
git commit -m "Add redirectReplace helper for opt-in history replace"
```

---

## Task 2: Change `AddExercise` to return the new slot ID

**Files:**
- Modify: `internal/workout/service.go` (function `AddExercise`, ~line 973)
- Modify: `internal/workout/service_test.go` (existing callers + new test)
- Modify: `cmd/web/handler-workout.go` (existing caller, ~line 377)

The repository assigns `workout_exercise.id` on insert and the service does not see it directly. The cleanest approach is to fetch the session after `Update` returns and locate the slot with matching `ExerciseID` (which the existing `Update` validates as unique within a session, so the lookup is unambiguous).

- [ ] **Step 1: Write the failing assertion**

Augment the existing `Test_AddExercise` subtest at `internal/workout/service_test.go` ("Add exercise to existing workout", around line 579-617). It currently calls `err = svc.AddExercise(ctx, today, exercise2ID)`. We change it to capture the returned slot ID and assert it points at the just-added slot.

Find this block (around line 588-592):

```go
		// Add exercise 2 to the workout
		err = svc.AddExercise(ctx, today, exercise2ID)
		if err != nil {
			t.Fatalf("Failed to add exercise to workout: %v", err)
		}
```

Replace with:

```go
		// Add exercise 2 to the workout. AddExercise returns the
		// workout_exercise.id of the new slot so handlers can redirect
		// straight to the new exercise's detail page.
		var newSlotID int
		newSlotID, err = svc.AddExercise(ctx, today, exercise2ID)
		if err != nil {
			t.Fatalf("Failed to add exercise to workout: %v", err)
		}
		if newSlotID == 0 {
			t.Errorf("expected non-zero new slot ID, got 0")
		}
```

Then, just before the closing brace of this subtest (after the existing `if !exists` block, around line 616), append:

```go
		// Verify the returned slot ID belongs to the slot we just added —
		// same workout_exercise.id, mapped to exercise 2.
		got, errGet := svc.GetSession(ctx, today)
		if errGet != nil {
			t.Fatalf("GetSession after add: %v", errGet)
		}
		var foundSlot bool
		for _, es := range got.ExerciseSets {
			if es.ID == newSlotID {
				if es.Exercise.ID != exercise2ID {
					t.Errorf("slot %d has exercise %d, want %d", newSlotID, es.Exercise.ID, exercise2ID)
				}
				foundSlot = true
				break
			}
		}
		if !foundSlot {
			t.Errorf("returned slot ID %d not present in session", newSlotID)
		}
```

- [ ] **Step 2: Run the augmented test to verify it fails**

Run: `go test -run "Test_AddExercise" -v ./internal/workout/ 2>&1 | head -30`

Expected: build failure with `assignment mismatch: 2 variables but svc.AddExercise returns 1 value`.

- [ ] **Step 3: Update `AddExercise` signature and implementation**

In `internal/workout/service.go`, replace the existing `AddExercise` function (~line 971-1042) with:

```go
// AddExercise adds a new exercise to an existing workout session.
// It will retrieve historical weight data if available. Returns the
// workout_exercise.id assigned to the new slot, so callers can build URLs
// that point at the new exercise's detail page.
func (s *Service) AddExercise(ctx context.Context, date time.Time, exerciseID int) (int, error) {
	// 1. Validate the exercise exists
	exercise, err := s.repo.exercises.Get(ctx, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("get exercise: %w", err)
	}

	// 2. Find historical data for the exercise
	historicalSets, err := s.findHistoricalSets(ctx, date, exerciseID)
	if err != nil {
		return 0, fmt.Errorf("find historical sets: %w", err)
	}

	// 3. Check if the workout session exists
	_, err = s.repo.sessions.Get(ctx, date)
	if errors.Is(err, ErrNotFound) {
		return 0, fmt.Errorf("workout session for date %s does not exist", formatDate(date))
	} else if err != nil {
		return 0, fmt.Errorf("check session existence: %w", err)
	}

	// 4. Update the session to add the new exercise
	err = s.repo.sessions.Update(ctx, date, func(sess *sessionAggregate) (bool, error) {
		// Check if the exercise already exists in the session
		for _, existingExercise := range sess.ExerciseSets {
			if existingExercise.ExerciseID == exerciseID {
				return false, fmt.Errorf("exercise %s already exists in workout for date %s",
					exercise.Name, formatDate(date))
			}
		}

		// Create sets for the exercise
		var newSets []Set
		if historicalSets != nil {
			// Use historical sets if available
			newSets = historicalSets
		} else {
			// Create default sets if no historical data exists
			const defaultSetCount = 3
			newSets = make([]Set, defaultSetCount)
			for i := range newSets {
				newSets[i] = Set{
					WeightKg:      new(float64),
					MinReps:       defaultMinReps,
					MaxReps:       defaultMaxReps,
					CompletedReps: nil,
					CompletedAt:   nil,
					Signal:        nil,
				}
			}
		}

		// Add the new exercise to the session. ID stays 0 so the repository
		// assigns a fresh workout_exercise.id on insert.
		newExerciseSet := exerciseSetAggregate{ //nolint:exhaustruct // ID is auto-assigned by repository.
			ExerciseID:        exerciseID,
			Sets:              newSets,
			WarmupCompletedAt: nil,
		}

		sess.ExerciseSets = append(sess.ExerciseSets, newExerciseSet)
		return true, nil
	})

	if err != nil {
		return 0, fmt.Errorf("update session with new exercise: %w", err)
	}

	// 5. Fetch the session back to learn the slot ID assigned by the
	// repository on insert. The slot is unique by ExerciseID within a
	// session (enforced by the duplicate check above), so locating it is
	// unambiguous.
	updated, err := s.repo.sessions.Get(ctx, date)
	if err != nil {
		return 0, fmt.Errorf("re-fetch session after add: %w", err)
	}
	for _, es := range updated.ExerciseSets {
		if es.ExerciseID == exerciseID {
			return es.ID, nil
		}
	}
	return 0, fmt.Errorf("added exercise %d not found in session %s", exerciseID, formatDate(date))
}
```

- [ ] **Step 4: Update remaining service test callers**

The `AddExercise` call at line 589 was already updated in Step 1 (it now captures `newSlotID`). Three more callers need to be updated to discard the new int return — `internal/workout/service_test.go` lines 622, 645, and 729 (numbers approximate; locate by content):

| Existing line | Change to |
|---|---|
| `err = svc.AddExercise(ctx, today, exercise1ID)` (in subtest "Add duplicate exercise to workout") | `_, err = svc.AddExercise(ctx, today, exercise1ID)` |
| `err = svc.AddExercise(ctx, futureDate, exercise1ID)` (in subtest "Add exercise to non-existent workout") | `_, err = svc.AddExercise(ctx, futureDate, exercise1ID)` |
| `err = svc.AddExercise(ctx, today, exerciseID)` (in `Test_AddExercise_UsesMostRecentHistoricalWeight`) | `_, err = svc.AddExercise(ctx, today, exerciseID)` |

Use `Edit` with enough surrounding context per call site to make each `old_string` unique.

- [ ] **Step 5: Update the handler caller (temporary, will be replaced in Task 3)**

In `cmd/web/handler-workout.go` at line 377, change:
```go
if err = app.workoutService.AddExercise(r.Context(), date, exerciseID); err != nil {
```
to:
```go
if _, err = app.workoutService.AddExercise(r.Context(), date, exerciseID); err != nil {
```

This keeps Task 2's commit compiling cleanly. The handler's redirect target changes in Task 3.

- [ ] **Step 6: Run all workout tests to verify they pass**

Run: `go test -v ./internal/workout/ ./cmd/web/`

Expected: all tests pass, including the new `Test_AddExercise_ReturnsNewSlotID`.

- [ ] **Step 7: Commit**

```bash
git add internal/workout/service.go internal/workout/service_test.go cmd/web/handler-workout.go
git commit -m "AddExercise returns the new slot ID

Lets handler callers redirect straight to the new exercise's detail page
without a second round-trip."
```

---

## Task 3: Redirect to new DETAIL via `redirectReplace`

**Files:**
- Modify: `cmd/web/handler-workout.go` (`workoutAddExercisePOST`, ~line 348-384)

- [ ] **Step 1: Update the handler**

Replace the body of `workoutAddExercisePOST` so it captures the new slot ID and redirects to the DETAIL URL via the new helper. In `cmd/web/handler-workout.go`, change the final block of the function from:

```go
	// Add exercise to the workout
	if _, err = app.workoutService.AddExercise(r.Context(), date, exerciseID); err != nil {
		app.serverError(w, r, err)
		return
	}

	// Redirect to the workout page
	redirect(w, r, fmt.Sprintf("/workouts/%s", date.Format("2006-01-02")))
}
```

to:

```go
	// Add exercise to the workout and capture the new slot ID so we can
	// land the user straight on the new exercise's detail page.
	newWorkoutExerciseID, err := app.workoutService.AddExercise(r.Context(), date, exerciseID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Replace /add-exercise with the new exercise's detail page so back
	// goes to the workout overview rather than the picker.
	redirectReplace(w, r, fmt.Sprintf("/workouts/%s/exercises/%d",
		date.Format("2006-01-02"), newWorkoutExerciseID))
}
```

(Note: `err` is already declared earlier in the handler from `r.ParseForm()` and `strconv.Atoi`, so this stays a `=` assignment, not `:=`. Adjust if linter disagrees.)

- [ ] **Step 2: Run handler tests**

Run: `go test -v ./cmd/web/ -run "Test_application_workoutAddExercise"`

Expected: PASS (the existing search-filter test does not submit the form, so it's unaffected).

- [ ] **Step 3: Commit**

```bash
git add cmd/web/handler-workout.go
git commit -m "workoutAddExercisePOST: redirect to new exercise's DETAIL

Lands the user straight on the just-added exercise so they can start
tracking sets immediately, with /add-exercise replaced rather than
pushed onto history."
```

---

## Task 4: Update client-side `main.js`

**Files:**
- Modify: `ui/static/main.js`

- [ ] **Step 1: Replace the top doc-comment**

Replace lines 1-56 of `ui/static/main.js` (the doc-comment block) with:

```js
/**
 * Stack Navigator
 * ===============
 *
 * Mission: make this server-rendered MPA feel native by intercepting form
 * POSTs, replaying them as fetch, and steering the History API so the back
 * button mirrors the user's mental model. The URL is the source of truth —
 * there is no virtual stack or in-memory page cache.
 *
 * Wire protocol
 * -------------
 *   Request:  X-Requested-With: stacknav  (set by the JS shim's fetch)
 *   Response: 200 + X-Location: <url>     (where to navigate; body empty)
 *   Response: X-Replace-URL: true         (optional; replace current entry)
 *
 * Without the X-Requested-With header, the server returns a plain 303 See
 * Other and the browser follows. That is the no-JS / no-Navigation-API path.
 *
 * Navigation strategy: pop-or-push (with explicit replace mode)
 * -------------------------------------------------------------
 * Two modes, decided per-response:
 *
 *   1. Replace mode — server set X-Replace-URL: true, OR the target URL
 *      equals the current entry's URL (same-URL submit). The current entry
 *      is replaced. We do not walk the history stack: replace is about
 *      erasing the current entry, not jumping to an existing one.
 *
 *   2. Pop-or-push mode — default. Walk back through history looking for
 *      an entry whose URL matches the target; traverse to it if found,
 *      otherwise push a new entry on top. Pop collapses cross-URL submits
 *      whose target is already in the backward stack (e.g. schedule → home,
 *      swap → original DETAIL via same slot URL); push handles cross-URL
 *      submits whose target is brand-new (e.g. start workout → workout day).
 *
 * Validation errors use putFlashError + redirect-to-form on the server, so
 * they arrive as 200 + X-Location pointing back at the form URL — same-URL
 * auto-replace handles them. The CSP (require-trusted-types-for 'script')
 * blocks any in-place HTML render, so we keep the wire shape uniform across
 * success and failure.
 *
 * Hierarchical backlink (data-back-button)
 * -----------------------------------------
 * Click delegation finds the closest <a data-back-button>; if a matching
 * URL exists earlier in the stack we traverse to it instead of pushing.
 * Without a match the link's natural href takes over — it becomes a regular
 * "up" navigation.
 *
 * Why preventDefault instead of intercept()
 * -----------------------------------------
 * iOS Safari does not yet fire precommit handlers (WebKit bug 293952), so
 * the validation we want to do before touching history is unreliable through
 * e.intercept(). preventDefault() + an awaited fetch is consistent across
 * browsers. Revisit when the WebKit bug closes — intercept() also gives us
 * e.signal for cancellation and centralized error handling.
 *
 * Progressive enhancement
 * -----------------------
 * Everything is gated on 'navigation' in window. Without the Navigation API
 * forms submit natively (303 redirect path) and the app works as a plain MPA.
 */
```

- [ ] **Step 2: Update `submitForm` to read `X-Replace-URL`**

In `ui/static/main.js`, find the success branch in `submitForm` (currently around line 109-117):

```js
    if (res.status === 200) {
        const target = res.headers.get('X-Location')
        if (!target) {
            location.reload()
            return
        }
        await popOrReplaceTo(target)
        return
    }
```

Replace with:

```js
    if (res.status === 200) {
        const target = res.headers.get('X-Location')
        if (!target) {
            location.reload()
            return
        }
        const replace = res.headers.get('X-Replace-URL') === 'true'
        await popOrPushTo(target, {replace})
        return
    }
```

- [ ] **Step 3: Replace `popOrReplaceTo` with `popOrPushTo`**

Replace the entire `popOrReplaceTo` function (currently around line 124-134) with:

```js
async function popOrPushTo(target, {replace = false} = {}) {
    const targetUrl = new URL(target, location.origin)

    // Replace mode: server-flagged via X-Replace-URL, or same-URL submit
    // (auto-detected so backend doesn't have to think about it). We
    // deliberately do not walk back looking for a traverse target —
    // replace is about erasing the current entry, not jumping elsewhere.
    if (replace || sameUrl(new URL(navigation.currentEntry.url), targetUrl)) {
        navigation.navigate(target, {history: 'replace'})
        return
    }

    // Genuine cross-URL navigation. Traverse to a backward match if
    // present, otherwise push a new entry.
    const entries = navigation.entries()
    for (let i = navigation.currentEntry.index - 1; i >= 0; i--) {
        if (sameUrl(new URL(entries[i].url), targetUrl)) {
            await navigation.traverseTo(entries[i].key).committed
            return
        }
    }
    navigation.navigate(target, {history: 'push'})
}
```

- [ ] **Step 4: Sanity-check there are no remaining references to `popOrReplaceTo`**

Run: `grep -n "popOrReplaceTo" /Users/personal/air/petrapp/ui/static/main.js`

Expected: no output (no remaining references).

- [ ] **Step 5: Run the smoke test to verify the JS changes don't break the golden path**

Run: `go test -v ./cmd/web/ -run "Test_playwright_smoketest"`

Expected: PASS. The smoke test exercises the main flows (start workout, exercise sets, complete workout, feedback) and would catch regressions in the navigation logic.

- [ ] **Step 6: Commit**

```bash
git add ui/static/main.js
git commit -m "Stack navigator: pop-or-push by default, opt-in replace

Server-set X-Replace-URL: true and same-URL submits both replace the
current history entry. Cross-URL submits push when the target is new and
traverse when the target is already in the backward stack. Fixes the
'Start Workout drops home from history' bug."
```

---

## Task 5: Update Playwright stacknav test

**Files:**
- Modify: `cmd/web/playwright_test.go` (`Test_playwright_stacknav` and `addExerciseToWorkout` helper)

- [ ] **Step 1: Update `addExerciseToWorkout` helper**

In `cmd/web/playwright_test.go`, replace the entire `addExerciseToWorkout` function (currently at the bottom of the file, ~lines 605-628) with:

```go
// addExerciseToWorkout navigates from the workout page to the add-exercise page,
// adds the first available exercise, and returns the page to workoutURL.
// Used when the weekly planner exhausts its exercise pool and creates an empty workout.
//
// Note: post add-exercise UX, the POST replaces /add-exercise with the new
// exercise's DETAIL page rather than the workout overview. This helper
// follows up with a Goto(workoutURL) so callers see the overview state they
// expect.
func addExerciseToWorkout(t *testing.T, page playwright.Page, workoutURL string) {
	t.Helper()
	var err error
	if err = page.Locator("a.add-exercise-button").Click(); err != nil {
		t.Fatalf("click Add Exercise link: %v", err)
	}
	if err = page.WaitForURL(func(u string) bool { return strings.Contains(u, "/add-exercise") }); err != nil {
		t.Fatalf("wait for add-exercise page: %v", err)
	}
	addBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Add this exercise"}).First()
	if err = addBtn.WaitFor(); err != nil {
		t.Fatalf("wait for Add this exercise button: %v", err)
	}
	if err = addBtn.Click(); err != nil {
		t.Fatalf("click Add this exercise: %v", err)
	}
	// The POST replaces /add-exercise with the new exercise's DETAIL page.
	if err = page.WaitForURL(func(u string) bool {
		return strings.Contains(u, "/exercises/")
	}); err != nil {
		t.Fatalf("wait for new exercise DETAIL after add: %v", err)
	}
	// Return the page to the workout overview for the caller's downstream
	// assertions (the helper's contract is "exercise added, page back at overview").
	if _, err = page.Goto(workoutURL); err != nil {
		t.Fatalf("goto workout overview after add: %v", err)
	}
}
```

- [ ] **Step 2: Update Flow 1 comment (set update)**

Find the Flow 1 block (search for `=== Flow 1: same-URL replace (set update) ===` in `cmd/web/playwright_test.go`). Update the comment header and the surrounding inline notes to reflect that same-URL replace is now driven by client auto-detect rather than the old "fall-through to replace" behavior. Concretely, update the comment block immediately above the Flow 1 setup. Find:

```go
	// === Flow 1: same-URL replace (set update) ===
	// All test exercises are weighted. The page shows a warmup banner first —
	// complete it before the set form appears. Warmup uses replaceTo(same-URL),
	// so history does not grow.
```

and change to:

```go
	// === Flow 1: same-URL replace (set update) ===
	// All test exercises are weighted. The page shows a warmup banner first —
	// complete it before the set form appears. The set/warmup POSTs redirect
	// back to the same DETAIL URL, and the client auto-detects same-URL and
	// replaces in place — so history does not grow on either submit.
```

Then find the comment block that says:

```go
		// After the set update, replaceTo(same-URL) fires history.replaceState +
		// location.reload().  Wait for the reload-triggered page navigation to fully
		// commit by checking that the first set now shows as completed.  This is more
		// reliable than WaitForURL (URL unchanged) or WaitForLoadState (already-load
		// resolves immediately).
```

and change to:

```go
		// After the set update, navigation.navigate(target, {history: 'replace'})
		// fires (target equals current URL → client auto-replaces). Wait for the
		// resulting reload-triggered navigation to fully commit by checking that
		// the first set now shows as completed. This is more reliable than
		// WaitForURL (URL unchanged) or WaitForLoadState (already-load resolves
		// immediately).
```

- [ ] **Step 3: Update Flow 3 comments (schedule submit)**

Find the Flow 3 setup block (search for `=== Flow 3 setup` in `cmd/web/playwright_test.go`). Update the inline comments that describe the navigation behavior. Find:

```go
	// === Flow 3 setup: fill all weekdays, then submit to get to /. ===
	// Because the server has no preferences yet, navigating to / redirects to
	// /schedule — meaning / never lands in the navigation stack.  To get / into
	// history we first submit the schedule once (landing at /) and then revisit
	// /schedule directly.  The second submit is the actual Flow 3 exercise.
```

and change to:

```go
	// === Flow 3 setup: fill all weekdays, then submit to get to /. ===
	// Because the server has no preferences yet, navigating to / redirects to
	// /schedule — meaning / never lands in the navigation stack from a normal
	// click. To exercise the "pop" branch in Flow 3 below, we first submit the
	// schedule once (landing at /) and then revisit /schedule directly. The
	// second submit is the actual Flow 3 exercise.
```

Find:

```go
	// First submit: prefs saved, server responds with pop-or-replace → /.
	// No / in history yet, so popOrReplaceTo falls back to replaceTo(/).
	// History after: [..., /]  (the /schedule entry is replaced).
```

and change to:

```go
	// First submit: prefs saved, server responds with pop-or-push → /.
	// No / in history yet, so popOrPushTo pushes / on top of /schedule.
	// History after: [..., /schedule, /].
```

Find:

```go
	// === Flow 3: pop-or-replace (schedule submit) ===
	// Re-fill the form, then submit. Now / IS in history so popOrReplaceTo
	// traverses to it instead of pushing — /schedule is removed from history.
```

and change to:

```go
	// === Flow 3: pop-or-push, traverse branch (schedule submit) ===
	// Re-fill the form, then submit. Now / IS in history (we navigated back to
	// /schedule via Goto, which pushed another /schedule on top), so popOrPushTo
	// traverses to / instead of pushing a duplicate.
```

Find:

```go
	// Verify pop-or-replace traversed (rather than pushed): /schedule must be
	// in the FORWARD stack (not backward), which we prove by GoForward returning
	// to /schedule.  A push-based implementation would instead have /schedule in
	// the backward stack and GoForward would go somewhere else.
```

and change to:

```go
	// Verify pop-or-push traversed (rather than pushed): /schedule must be in
	// the FORWARD stack (not backward), which we prove by GoForward returning
	// to /schedule. A naive push would have placed a fresh / on top of the
	// existing /schedule, leaving the forward stack empty.
```

- [ ] **Step 4: Update Flow 2 comment (swap)**

Find the Flow 2 block (search for `=== Flow 2: cross-URL replace (swap exercise) ===`). The header is misleading post-redesign: swap traverses to the original DETAIL via the same-slot URL. Change:

```go
	// === Flow 2: cross-URL replace (swap exercise) ===
```

to:

```go
	// === Flow 2: cross-URL submit, target present (swap exercise) ===
	// Swap redirects to the same workoutExerciseID slot, so the URL matches
	// the original DETAIL the user came from. popOrPushTo finds it in the
	// backward stack and traverses to it.
```

- [ ] **Step 5: Update Flow 5 comment (validation error)**

Find the Flow 5 block (search for `=== Flow 5: Validation error`). Change:

```go
	// === Flow 5: Validation error (empty schedule submit). Run before filling
	// the form so we see the error path first. ===
```

to:

```go
	// === Flow 5: Validation error (empty schedule submit). Run before filling
	// the form so we see the error path first. The flash + redirect-to-form
	// pattern lands at the same URL the form was submitted from, which the
	// client auto-detects as same-URL and replaces in place. ===
```

- [ ] **Step 6: Add Flow 6 — add-exercise replace**

At the very end of `Test_playwright_stacknav` (just before the closing brace of the function), append:

```go
	// === Flow 6: add-exercise replace ===
	// Click the add-exercise link from the workout overview, pick an exercise,
	// and assert that the POST replaces /add-exercise with the new exercise's
	// DETAIL page (back goes to the workout overview, not the picker).
	if _, err = page.Goto(workoutURL); err != nil {
		t.Fatalf("Flow 6: goto workoutURL: %v", err)
	}
	addExerciseLink := page.Locator("a.add-exercise-button")
	if err = addExerciseLink.WaitFor(); err != nil {
		t.Fatalf("Flow 6: wait for add-exercise link: %v", err)
	}
	if err = addExerciseLink.Click(); err != nil {
		t.Fatalf("Flow 6: click add-exercise link: %v", err)
	}
	if err = page.WaitForURL(func(u string) bool { return strings.Contains(u, "/add-exercise") }); err != nil {
		t.Fatalf("Flow 6: wait for /add-exercise: %v", err)
	}

	// Capture the name of the first available exercise to verify we land on its DETAIL.
	firstAvailableName, err := page.Locator(".exercise-option .exercise-name").First().InnerText()
	if err != nil {
		t.Fatalf("Flow 6: read first available exercise name: %v", err)
	}

	addThisBtn := page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Add this exercise"}).First()
	if err = addThisBtn.Click(); err != nil {
		t.Fatalf("Flow 6: click Add this exercise: %v", err)
	}

	// Should land on the new exercise's DETAIL page.
	flow6DetailPattern := regexp.MustCompile(fmt.Sprintf(
		`/workouts/%s/exercises/\d+$`, regexp.QuoteMeta(strings.TrimPrefix(workoutHref, "/workouts/"))))
	if err = page.WaitForURL(flow6DetailPattern); err != nil {
		t.Fatalf("Flow 6: wait for new DETAIL URL: %v", err)
	}

	// Verify it's the exercise we picked.
	gotHeading, err := page.Locator("h1").First().InnerText()
	if err != nil {
		t.Fatalf("Flow 6: read DETAIL heading: %v", err)
	}
	if !strings.Contains(strings.TrimSpace(gotHeading), strings.TrimSpace(firstAvailableName)) {
		t.Errorf("Flow 6: DETAIL heading = %q, want to contain %q", gotHeading, firstAvailableName)
	}

	// Back should land on the workout overview, NOT /add-exercise (proves replace).
	if _, err = page.GoBack(); err != nil {
		t.Fatalf("Flow 6: GoBack: %v", err)
	}
	if got, want := page.URL(), workoutURL; got != want {
		t.Errorf("Flow 6: back from new DETAIL = %q, want %q (add-exercise should be replaced)", got, want)
	}

	// Forward should NOT return to /add-exercise — that entry was destroyed.
	if _, err = page.GoForward(); err != nil {
		t.Fatalf("Flow 6: GoForward: %v", err)
	}
	if got := page.URL(); strings.Contains(got, "/add-exercise") {
		t.Errorf("Flow 6: forward URL = %q contains /add-exercise — it should have been replaced", got)
	}
```

(The `regexp` and `fmt` and `strings` imports are already in scope in this file from earlier flows.)

- [ ] **Step 7: Run the stacknav test**

Run: `go test -v ./cmd/web/ -run "Test_playwright_stacknav"`

Expected: PASS, including new Flow 6 assertions.

- [ ] **Step 8: Run the smoke test for regressions**

Run: `go test -v ./cmd/web/ -run "Test_playwright_smoketest"`

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add cmd/web/playwright_test.go
git commit -m "Update Playwright stacknav test for push-by-default

- addExerciseToWorkout helper handles new redirect target
- Flow 1/2/3/5 comments updated to reflect new mechanism
- New Flow 6 verifies add-exercise replace behavior"
```

---

## Task 6: Update `cmd/web/CLAUDE.md`

**Files:**
- Modify: `cmd/web/CLAUDE.md` ("Redirects and Navigation" section)

- [ ] **Step 1: Replace the section**

Find the existing "Redirects and Navigation" section in `cmd/web/CLAUDE.md` (currently has `## Redirects and Navigation` heading, then `### Using redirect() Helper` subheading with four bullet points pointing at the old spec). Replace the entire section with:

```markdown
## Redirects and Navigation

### Using redirect() and redirectReplace()

Two helpers cover all redirect needs. Both negotiate the stack-navigator wire protocol when the request carries `X-Requested-With: stacknav` (set by the JS shim's POST fetch), and fall through to a plain 303 See Other otherwise. Non-POST callers transparently use the 303 path because they don't carry the header.

- **`redirect(w, r, "/path")`** — default. Use for almost all redirects: POST results, GET-handler bounces, auth gates, validation re-renders via flash + redirect-to-form. The client behavior is "pop-or-push": traverse to the URL if it's already in the backward history stack, otherwise push a new entry. Same-URL submits (target equals the current URL — set updates, warmup completion, validation errors that re-render the form) are auto-detected by the client and become a replace; the helper itself stays simple.
- **`redirectReplace(w, r, "/path")`** — opt-in. Use when the originating page should be erased from history on submit. Today's only call site is `workoutAddExercisePOST`, which redirects to the new exercise's detail page and replaces `/add-exercise`. Reach for this when the form page only exists to submit (a picker, an editor that disappears on save) and you don't want it left behind in the back-button stack.

The client treats every 200 response with `X-Location` as a navigation; an additional `X-Replace-URL: true` header (set by `redirectReplace`) flips the strategy from pop-or-push to replace.

See `docs/superpowers/specs/2026-05-03-stack-navigator-push-default-design.md` for the wire protocol, per-flow behavior, and rationale.
```

- [ ] **Step 2: Verify the section reads cleanly in the rendered file**

Run: `grep -A 30 "## Redirects and Navigation" /Users/personal/air/petrapp/cmd/web/CLAUDE.md`

Expected: prints the new section. Spot-check that the old `### Using redirect() Helper` subheading is gone.

- [ ] **Step 3: Commit**

```bash
git add cmd/web/CLAUDE.md
git commit -m "Update web layer CLAUDE.md for new redirect helpers"
```

---

## Task 7: Final verification

- [ ] **Step 1: Run the full CI check**

Run: `make ci`

Expected: build passes, lint passes, all tests pass, security check passes.

If lint flags any issues (e.g., `nolint` formatting, unused vars from the signature change), fix them inline and re-run. If a test fails, debug rather than mass-skipping — the new behavior should produce stable results.

- [ ] **Step 2: Manual verification of the bug fix**

If a dev server is convenient, run `make run` (or whatever the project's dev command is — check the Makefile), navigate to `/`, click "Start Workout", and press the browser back button. You should land back on `/` (the home page). Before this change, back exited the app.

Also exercise the add-exercise flow: from the workout overview, click "Add Exercise" → pick an exercise → confirm you land on its detail page → press back → confirm you're at the workout overview, not the picker.

- [ ] **Step 3: Final commit if any fixes needed**

If `make ci` produced any fix-up commits, those are already done above. Otherwise no commit is needed for Step 3.

---

## Self-review notes

- All 7 tasks land on a single coherent feature; no task introduces speculative abstractions.
- Tests precede implementation in Tasks 1 and 2 (TDD); Tasks 3–6 are wiring + docs and do not introduce new units that warrant their own unit tests beyond the integration coverage in Task 5.
- The `AddExercise` re-fetch in Task 2 is intentional: the simplest correct implementation. A future optimization could surface the new ID through the repository's `Update` callback, but YAGNI for now.
- No SQL schema change needed — `workout_exercise.id` already exists and the repository assigns it on insert.
- Browser-support assumptions (Navigation API, iOS Safari precommit absence) are unchanged from the prior spec.
