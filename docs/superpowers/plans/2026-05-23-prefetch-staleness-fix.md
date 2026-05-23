# Prefetch staleness fix — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move the bfcache staleness check in `ui/static/main.js` from `pageshow.persisted` to `pagereveal` so Speculation Rules prefetch promotion is covered. Preserve the workout page's strike-through animation flow.

**Architecture:** Single-file change to `main.js` splitting today's combined `pageshow.persisted` listener into (a) a thin `pageshow.persisted` listener that only resets the bfcache-captured button spinner via `clearLoad()`, and (b) a new `pagereveal` listener carrying the rendered-vs-cookie comparison. One Playwright regression test in `cmd/web/playwright_test.go` locks in that some handler triggers the reload after a bfcache restore. Verified by `tlaplus/StackNav_PrefetchMitigated.cfg` (already in repo, 825k states).

**Tech Stack:** Vanilla JS using the Navigation API (`pagereveal`, `navigation.reload`); Playwright with the playwright-go bindings for the regression test.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `ui/static/main.js` | Modify lines 327-354 | Split the combined `pageshow.persisted` block into two focused listeners. |
| `cmd/web/playwright_test.go` | Append new test func | `Test_playwright_bfcache_staleness`: drive bfcache restore on a stale doc and assert that `<meta>` token ends up equal to the `inv_bfcache` cookie. |

No other files. No schema, no service, no migration.

The full design (incl. per-flow table and rationale) is in `docs/superpowers/specs/2026-05-23-prefetch-staleness-fix-design.md`.

---

## Task 1: Replace the pageshow.persisted block in main.js

**Files:**
- Modify: `ui/static/main.js:327-354`

- [ ] **Step 1: Open `ui/static/main.js` and read the current block (lines 327-354) to confirm it matches the diff below**

Run: `awk 'NR>=327 && NR<=354' ui/static/main.js`

Expected output: the existing `window.addEventListener('pageshow', (event) => { if (event.persisted) { ... } })` block. If the line numbers have shifted, locate the block by `grep -n "pageshow" ui/static/main.js`.

- [ ] **Step 2: Replace the block with the split version**

Use the Edit tool to replace exactly this `old_string`:

```js
window.addEventListener('pageshow', (event) => {
    if (event.persisted) {
        // Clear any in-flight navigation feedback that was captured in the
        // bfcache snapshot — the navigation that triggered it has long
        // since resolved (we are reading this snapshot from the cache).
        clearLoad()

        // Pages that handle their own bfcache invalidation opt out here so
        // their custom flow (e.g. replace navigation for cross-doc view
        // transitions) isn't preempted by our global reload.
        if (document.body.dataset.bfcacheHandler === 'page-local') return

        // Reload if the invalidation cookie has changed since this page was rendered.
        // The render-time value is baked into a <meta> tag; a mismatch means a POST
        // ran while we were in bfcache and our state may be stale.
        const meta = document.querySelector('meta[name="invalidation-token"]')
        const rendered = meta ? meta.content : ''
        const m = document.cookie.match(/(?:^|;\s*)inv_bfcache=([^;]+)/)
        const current = m ? m[1] : ''
        if (rendered !== current) {
            if ('navigation' in window) {
                navigation.reload()
            } else {
                location.reload()
            }
        }
    }
})
```

with this `new_string`:

```js
// Clear the bfcache-captured button spinner on restore. The navigation
// that started the spinner has long since resolved; the snapshot just
// has stale aria-busy / .btn-loading-label markup that startLoad would
// otherwise leave dangling.
window.addEventListener('pageshow', (event) => {
    if (event.persisted) clearLoad()
})

// Staleness check runs on every reveal — fresh load, prefetched-and-
// promoted load, bfcache restore. Catches three classes of stale-doc:
//   1. bfcache restore of a page rendered before a since-POSTed mutation
//   2. Speculation Rules prefetch promoted after a POST has moved the
//      cookie past the prefetched response's baked token (base.gohtml
//      prefetches all /* at conservative eagerness)
//   3. (defensive) any other path that promotes a doc whose meta token
//      is older than the current cookie
//
// Pages that drive their own bfcache flow opt out via dataset:
// workout.gohtml sets data-bfcache-handler="page-local" at parse time
// and runs its own pagereveal handler that does a replace navigation to
// keep the bfcache snapshot as the outgoing view-transition snapshot
// (so the per-card strike-through animation can play).
//
// Verified by tlaplus/StackNav_PrefetchMitigated.cfg.
window.addEventListener('pagereveal', () => {
    if (document.body.dataset.bfcacheHandler === 'page-local') return
    const meta = document.querySelector('meta[name="invalidation-token"]')
    const rendered = meta ? meta.content : ''
    const m = document.cookie.match(/(?:^|;\s*)inv_bfcache=([^;]+)/)
    const current = m ? m[1] : ''
    if (rendered === current) return
    navigation.reload()
})
```

- [ ] **Step 3: Run lint to confirm no formatting issues**

Run: `make lint-fix`

Expected: `0 issues.`

- [ ] **Step 4: Run the existing test suite to confirm nothing broke**

Run: `make test`

Expected: all packages pass. `cmd/web` includes the existing `Test_playwright_stacknav` and `Test_playwright_smoketest`; both touch `pageshow`-driven flows and would surface any regression in the listener split.

- [ ] **Step 5: Commit**

```bash
git add ui/static/main.js
git commit -m "$(cat <<'EOF'
fix: move bfcache staleness check from pageshow.persisted to pagereveal

The pageshow.persisted handler only fired on bfcache restore, so
Speculation Rules prefetch promotion landed on stale docs without
triggering the rendered-vs-cookie check. pagereveal fires on every
navigation reveal (fresh, prefetched, bfcache) so one place catches
all three. workout.gohtml's page-local opt-out (and its strike-through
view transition) is unchanged. Verified by
tlaplus/StackNav_PrefetchMitigated.cfg.
EOF
)"
```

---

## Task 2: Add the Playwright bfcache regression test

**Files:**
- Modify: `cmd/web/playwright_test.go` (append a new test func at end-of-file, before the helper funcs `dumpNavDiagnostics` / `addExerciseToWorkout`)

- [ ] **Step 1: Locate the insertion point**

Run: `grep -n "^func dumpNavDiagnostics" cmd/web/playwright_test.go`

Expected output: a line number for `func dumpNavDiagnostics`. Insert the new test func directly above that line so it sits next to the other top-level test funcs and before the helpers.

- [ ] **Step 2: Insert the new test func**

Add this block immediately before `func dumpNavDiagnostics`:

```go
// Test_playwright_bfcache_staleness drives a flow where the home page (/)
// is rendered before any POST (bakedToken == ""), then a POST rotates the
// inv_bfcache cookie, then the user navigates back. The browser restores /
// from bfcache (its rendered meta token is still ""), our pagereveal
// handler detects the mismatch against the now-rotated cookie, and triggers
// navigation.reload(). After the reload, meta should equal the cookie.
//
// This is regression protection for the listener wiring: if both
// pageshow.persisted and pagereveal staleness paths regress simultaneously
// (e.g. someone removes the listener), the assertion below fails. It does
// not exercise the Speculation Rules prefetch path — that's the TLA+
// spec's job (tlaplus/StackNav_Prefetch.cfg /
// tlaplus/StackNav_PrefetchMitigated.cfg).
func Test_playwright_bfcache_staleness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow playwright bfcache staleness test")
	}

	page, serverURL := setupPlaywrightPage(t)
	var err error

	// Register and schedule today + tomorrow so today has a workout
	// (matches the smoketest setup pattern around the midnight boundary).
	if err = page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Begin training"}).Click(); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/schedule", serverURL)); err != nil {
		t.Fatalf("expect /schedule after registration: %v", err)
	}
	testStart := time.Now()
	todayWeekday := testStart.Weekday().String()
	nextWeekday := testStart.AddDate(0, 0, 1).Weekday().String()
	for _, day := range []string{todayWeekday, nextWeekday} {
		if _, err = page.GetByLabel(day).SelectOption(playwright.SelectOptionValues{
			Labels: &[]string{"1 hour"},
		}); err != nil {
			t.Fatalf("select %s duration: %v", day, err)
		}
	}
	if err = page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Start Tracking"}).Click(); err != nil {
		t.Fatalf("submit schedule: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect / after schedule submit: %v", err)
	}

	// At this point / is loaded with meta == current inv_bfcache cookie
	// (the schedule POST already rotated it). Drive a second POST so the
	// /-as-it-currently-sits will be bfcached BEFORE the rotation we care
	// about. Click "Start Workout": POSTs /workouts/{today}/start and
	// pushes /workouts/{today} on top of /.
	if err = page.GetByRole("button",
		playwright.PageGetByRoleOptions{Name: "Start Workout"}).Click(); err != nil {
		t.Fatalf("click Start Workout: %v", err)
	}
	workoutURLPattern := regexp.MustCompile(fmt.Sprintf(
		`^%s/workouts/\d{4}-\d{2}-\d{2}$`, regexp.QuoteMeta(serverURL)))
	if err = page.WaitForURL(workoutURLPattern); err != nil {
		t.Fatalf("expect /workouts/{today} after Start Workout: %v", err)
	}

	// Snapshot the cookie that the Start Workout POST set. Used as the
	// expected value to assert against after the back-nav-triggered reload.
	cookieAfterPost, err := page.Evaluate(
		`document.cookie.match(/(?:^|;\s*)inv_bfcache=([^;]+)/)?.[1] ?? ''`)
	if err != nil {
		t.Fatalf("read cookie after Start Workout: %v", err)
	}
	wantToken, _ := cookieAfterPost.(string)
	if wantToken == "" {
		t.Fatalf("inv_bfcache cookie not set after Start Workout POST")
	}

	// Navigate back to /. The browser restores it from bfcache with the
	// pre-POST meta token (which was rotated when the schedule POST set the
	// cookie, but the / snapshot in bfcache was captured BEFORE the Start
	// Workout POST rotated it again). Mismatch -> pagereveal handler
	// triggers navigation.reload(). After reload, meta == cookie.
	if _, err = page.GoBack(); err != nil {
		t.Fatalf("GoBack to /: %v", err)
	}
	if err = page.WaitForURL(fmt.Sprintf("%s/", serverURL)); err != nil {
		t.Fatalf("expect / after GoBack: %v", err)
	}

	// Poll until the page's meta token equals the post-Start-Workout cookie.
	// The bfcache snapshot has the OLD token; after the staleness-triggered
	// reload, the freshly-fetched / will have the new token. If no listener
	// fires the reload, this times out.
	_, err = page.WaitForFunction(`
		(want) => {
			const meta = document.querySelector('meta[name=invalidation-token]')?.content ?? '';
			return meta === want;
		}
	`, wantToken, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(5000),
	})
	if err != nil {
		// Failure path: dump current state to narrow the post-mortem.
		state, _ := page.Evaluate(`() => ({
			meta: document.querySelector('meta[name=invalidation-token]')?.content ?? null,
			cookie: document.cookie.match(/(?:^|;\s*)inv_bfcache=([^;]+)/)?.[1] ?? null,
			url: location.href,
		})`)
		t.Fatalf("post-bfcache reload did not converge meta to cookie within 5s: %v; state=%+v; want=%q",
			err, state, wantToken)
	}
}
```

- [ ] **Step 3: Run lint to confirm no formatting issues**

Run: `make lint-fix`

Expected: `0 issues.`

- [ ] **Step 4: Run the new test alone**

Run: `go test -v ./cmd/web -run Test_playwright_bfcache_staleness`

Expected: `PASS` within ~15 seconds. If it fails with a `WaitForFunction` timeout, the post-mortem state dump prints `meta`, `cookie`, and `url` — first thing to check is whether the cookie is set and whether the URL is actually `/`. If it fails on registration or schedule submit, the surrounding flow is broken (not the staleness path).

- [ ] **Step 5: Run the full test suite to confirm nothing else broke**

Run: `make test`

Expected: all packages pass, including the existing `Test_playwright_smoketest` and `Test_playwright_stacknav`.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/playwright_test.go
git commit -m "$(cat <<'EOF'
test: lock in bfcache staleness reload via Playwright

Drives a back-navigation that hits bfcache on a doc rendered before
the most recent POST, then asserts the <meta> invalidation-token
converges to the current cookie value within 5s. If both
pageshow.persisted and pagereveal staleness paths regress
simultaneously this fails. Regression protection for the listener
wiring change in the previous commit; prefetch staleness itself is
covered by tlaplus/StackNav_Prefetch.cfg.
EOF
)"
```

---

## Task 3: Manual verification of the workout strike-through animation

No automation — see the spec's testing section for why. This task documents the manual check that gates merging.

- [ ] **Step 1: Start the dev server**

Run: `make dev`

Expected: server listens on a local port; terminal shows the URL (typically `http://localhost:4000`).

- [ ] **Step 2: Open the app in Chrome and log in / register**

Navigate to the URL printed in step 1. Register (or log in if a test user already exists). Configure today's schedule if not already set.

- [ ] **Step 3: Open today's workout and complete one set on the first exercise**

On `/`, click "Start Workout". On `/workouts/{today}`, click the first exercise. Mark warmup done, then complete at least one set on that exercise so the exercise gains a "completed set" state.

- [ ] **Step 4: Navigate back to the workout overview**

Click the back link (or use the browser back button) to return to `/workouts/{today}`.

- [ ] **Step 5: Observe the strike-through animation**

Expected: the just-completed exercise's name should animate a strike-through line drawing left-to-right over ~320ms. If the line appears already drawn with no animation (or the whole page reloads to a static state), the workout page-local handler has been disturbed and the fix needs investigation.

- [ ] **Step 6: Negative check — confirm the global handler still bails on `page-local`**

In Chrome dev tools, while on `/workouts/{today}`, run in the console:

```js
document.body.dataset.bfcacheHandler
```

Expected: `'page-local'`. If it returns `undefined`, the workout page's inline script that sets the attribute is broken (unrelated to this fix, but the same negative consequence — global handler would trample the page-local flow).

- [ ] **Step 7: Stop the dev server**

Ctrl-C the terminal running `make dev`.

No commit for this task — verification only.

---

## Done criteria

- `make ci` passes.
- `Test_playwright_bfcache_staleness` passes.
- Manual workout strike-through verification (Task 3) confirmed visually.
- Both commits pushed.
