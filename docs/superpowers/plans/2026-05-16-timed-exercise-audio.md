# Timed Exercise Audio Cues Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a JS-only Start button to the active time-based set card that runs a 10-second prep countdown and then a target-seconds exercise countdown, with WebAudio beeps at transitions and a screen wake lock for the duration. No-JS behavior unchanged.

**Architecture:** Pure UI change in one template file (`ui/templates/pages/exerciseset/sets-container.gohtml`). Server emits the runner markup and a Start button inside the existing `{{ else if eq $.ExerciseSet.Exercise.ExerciseType "time_based" }}` branch, both `hidden`. A scoped CSS block and an inline IIFE script (matching the precedent set by the `.rest-chip` block at lines 403–453) own appearance and behavior. One new render test in `cmd/web/handler-exerciseset_test.go` locks the markup contract.

**Tech Stack:** Go html/template, plain JS (`AudioContext`, `navigator.wakeLock`), goquery for tests. CSP nonces on every `<style>` / `<script>` tag.

**Spec:** [`docs/superpowers/specs/2026-05-16-timed-exercise-audio-design.md`](../specs/2026-05-16-timed-exercise-audio-design.md)

---

## File Map

- **Modify** `ui/templates/pages/exerciseset/sets-container.gohtml` (~line 563, inside the `time_based` branch) — adds runner markup, scoped CSS, scoped JS.
- **Modify** `cmd/web/handler-exerciseset_test.go` (append a new top-level test function) — locks the markup contract.

No other files change. No domain, service, handler, or routing edits. No DB migration.

---

## Task 1: Render-test + server-rendered markup (TDD pair)

**Goal:** A failing render test that asserts the timed-runner markup exists on the active time-based set card, then the markup that makes it pass.

**Files:**
- Test: `cmd/web/handler-exerciseset_test.go` (append at end of file)
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml` (inside `{{ else if eq $.ExerciseSet.Exercise.ExerciseType "time_based" }}` branch, ~line 563, immediately before the existing `<form>`)

- [ ] **Step 1: Append the failing render test**

Open `cmd/web/handler-exerciseset_test.go`, append at the bottom of the file:

```go
// Test_application_exerciseSet_time_based_active_timer_markup verifies that
// the active set card for a time-based exercise (Plank) emits the JS hooks
// that the inline timed-runner script needs: a [data-timed-runner] container
// carrying [data-target-seconds] and [hidden], a [data-timed-runner-start]
// button (also [hidden] — JS reveals it), and a [data-cancel] button inside
// the runner. Non-active and non-time-based cards do not match this branch
// of the template, so the negative case is enforced structurally.
func Test_application_exerciseSet_time_based_active_timer_markup(t *testing.T) {
	var (
		ctx = t.Context()
		doc *goquery.Document
		err error
	)

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}

	formData := map[string]string{time.Now().Weekday().String(): "60"}
	if doc, err = client.GetDoc(ctx, "/preferences"); err != nil {
		t.Fatalf("get preferences: %v", err)
	}
	if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
		t.Fatalf("submit preferences: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	if _, err = client.SubmitForm(ctx, doc, "/workouts/"+today+"/start", nil); err != nil {
		t.Fatalf("start workout: %v", err)
	}

	db := server.DB()
	var plankID int
	if err = db.QueryRowContext(ctx,
		`SELECT id FROM exercises WHERE name = 'Plank'`).Scan(&plankID); err != nil {
		t.Fatalf("get Plank id: %v", err)
	}

	var slotID int
	if err = db.QueryRowContext(ctx,
		`INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id,
            warmup_completed_at)
         SELECT user_id, workout_date, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ')
         FROM workout_sessions WHERE workout_date = ?
         RETURNING id`, plankID, today).Scan(&slotID); err != nil {
		t.Fatalf("insert plank slot: %v", err)
	}

	if _, err = db.ExecContext(ctx,
		`INSERT INTO exercise_sets (workout_exercise_id, set_number,
            weight_kg, target_value)
         VALUES (?, 1, 0.0, 30)`, slotID); err != nil {
		t.Fatalf("insert plank set: %v", err)
	}

	slotPath := "/workouts/" + today + "/exercises/" + strconv.Itoa(slotID)
	if doc, err = client.GetDoc(ctx, slotPath); err != nil {
		t.Fatalf("get exercise set page: %v", err)
	}

	activeCard := doc.Find(".exercise-set.active").First()
	if activeCard.Length() == 0 {
		t.Fatalf("expected an active .exercise-set on the Plank page")
	}

	runner := activeCard.Find("[data-timed-runner]")
	if runner.Length() != 1 {
		t.Errorf("active card: [data-timed-runner] count = %d, want 1", runner.Length())
	}
	if target, ok := runner.Attr("data-target-seconds"); !ok || target != "30" {
		t.Errorf("[data-timed-runner] data-target-seconds = %q (present=%v), want %q",
			target, ok, "30")
	}
	if _, hasHidden := runner.Attr("hidden"); !hasHidden {
		t.Errorf("[data-timed-runner] must be server-rendered with [hidden]")
	}

	startBtn := activeCard.Find("[data-timed-runner-start]")
	if startBtn.Length() != 1 {
		t.Errorf("active card: [data-timed-runner-start] count = %d, want 1", startBtn.Length())
	}
	if _, hasHidden := startBtn.Attr("hidden"); !hasHidden {
		t.Errorf("[data-timed-runner-start] must be server-rendered with [hidden]")
	}

	cancelBtn := runner.Find("[data-cancel]")
	if cancelBtn.Length() != 1 {
		t.Errorf("[data-cancel] count inside runner = %d, want 1", cancelBtn.Length())
	}
}
```

The test reuses the exact setup from `Test_application_exerciseSet_time_based_active_oversized_layout` — same imports already in this test file, no new imports required.

- [ ] **Step 2: Run the test and confirm it fails**

Run: `go test -v ./cmd/web -run Test_application_exerciseSet_time_based_active_timer_markup`

Expected: FAIL with messages like `active card: [data-timed-runner] count = 0, want 1` and the same for `[data-timed-runner-start]` and `[data-cancel]`.

- [ ] **Step 3: Add the server-rendered markup**

Open `ui/templates/pages/exerciseset/sets-container.gohtml`. Locate the block (around line 563):

```gotemplate
                    {{ else if eq $.ExerciseSet.Exercise.ExerciseType "time_based" }}
                        <form method="post"
```

Insert immediately before the `<form ...>` line:

```gotemplate
                        <div class="timed-runner" data-timed-runner
                             data-target-seconds="{{ $.CurrentSetTimedTarget }}"
                             aria-live="polite"
                             hidden>
                            <div class="timed-runner-display" data-phase="idle">
                                <span class="timed-runner-label" data-label>Get ready</span>
                                <span class="timed-runner-time" data-time>0:10</span>
                            </div>
                            <button type="button" class="timed-runner-cancel" data-cancel>Cancel</button>
                        </div>
                        <button type="button" class="submit-button timed-runner-start"
                                data-timed-runner-start hidden>Start timer</button>
```

Indent matches the surrounding `<form>` block (24 spaces of leading indent). Order: runner container, then Start button. Both `hidden` so non-JS users see nothing.

The Start button piggy-backs the existing `.submit-button` class (defined at line 349) so it inherits the project's primary-CTA styling.

- [ ] **Step 4: Run the test and confirm it passes**

Run: `go test -v ./cmd/web -run Test_application_exerciseSet_time_based_active_timer_markup`

Expected: PASS.

- [ ] **Step 5: Run the broader handler test set to confirm no regression**

Run: `go test ./cmd/web -run Test_application_exerciseSet`

Expected: PASS (all matching tests).

- [ ] **Step 6: Commit**

```bash
git add cmd/web/handler-exerciseset_test.go ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "Add timed-runner markup hooks to active time-based set card"
```

---

## Task 2: Scoped CSS for the runner

**Goal:** Style the runner container — visually weighty countdown, clear phase distinction, secondary Cancel button.

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml` (insert a new `<style {{ nonce }}>` block immediately after the markup added in Task 1, mirroring the `.rest-chip` precedent at lines 405–428)

- [ ] **Step 1: Insert the scoped style block**

Open `ui/templates/pages/exerciseset/sets-container.gohtml`. Immediately after the closing `</button>` of `data-timed-runner-start` (the last line you added in Task 1), insert:

```gotemplate
                        <style {{ nonce }}>
                            @scope (.timed-runner) {
                                :scope {
                                    display: flex;
                                    flex-direction: column;
                                    align-items: center;
                                    gap: var(--size-3);
                                    padding: var(--size-4);
                                    margin-bottom: var(--size-3);
                                    background: var(--color-info-bg);
                                    color: var(--color-info);
                                    border-radius: var(--radius-3);
                                }
                                .timed-runner-display {
                                    display: flex;
                                    flex-direction: column;
                                    align-items: center;
                                    gap: var(--size-1);
                                }
                                .timed-runner-label {
                                    font-size: var(--font-size-0);
                                    text-transform: uppercase;
                                    letter-spacing: var(--font-letterspacing-3);
                                    font-weight: var(--font-weight-7);
                                }
                                .timed-runner-time {
                                    font-family: var(--font-mono);
                                    font-size: var(--font-size-7);
                                    font-weight: var(--font-weight-8);
                                    line-height: 1;
                                    font-variant-numeric: tabular-nums;
                                }
                                .timed-runner-display[data-phase="exercise"] {
                                    color: var(--color-success);
                                }
                                .timed-runner-cancel {
                                    padding: var(--size-1) var(--size-3);
                                    background: transparent;
                                    color: inherit;
                                    border: var(--border-size-1) solid currentColor;
                                    border-radius: var(--radius-2);
                                    font-size: var(--font-size-0);
                                    text-transform: uppercase;
                                    letter-spacing: var(--font-letterspacing-3);
                                    cursor: pointer;
                                }
                            }
                        </style>
```

Token rationale: `--size-*`, `--radius-*`, `--font-size-*`, `--font-weight-*`, `--font-letterspacing-3`, `--color-info`, `--color-info-bg`, `--color-success`, `--font-mono` are all defined in `ui/static/main.css` (verified at lines 56–82, 96, 121–130, 220–221, 274). The pattern of `@scope (.foo)` blocks inside `<style {{ nonce }}>` is the established convention in this file.

- [ ] **Step 2: Run the previous task's test to confirm no regression**

Run: `go test -v ./cmd/web -run Test_application_exerciseSet_time_based_active_timer_markup`

Expected: PASS (CSS doesn't affect markup assertions).

- [ ] **Step 3: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "Style the timed-runner countdown card"
```

---

## Task 3: JS module — Start, prep, exercise, end, cancel, wake lock

**Goal:** The behavioral payload — Start unlocks audio + acquires wake lock + hides form + counts down 10s with chirps, transitions to target-seconds countdown with go-beep, ends with end-beep and restores form. Cancel returns silently.

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml` (insert a new `<script {{ nonce }}>` block immediately after the `<style>` block from Task 2)

- [ ] **Step 1: Insert the inline JS module**

Open `ui/templates/pages/exerciseset/sets-container.gohtml`. Immediately after the closing `</style>` from Task 2, insert:

```gotemplate
                        <script {{ nonce }}>
                            (() => {
                                const root = document.querySelector('[data-timed-runner]');
                                const startBtn = document.querySelector('[data-timed-runner-start]');
                                if (!root || !startBtn) return;

                                const targetSeconds = parseInt(root.dataset.targetSeconds, 10);
                                if (!Number.isFinite(targetSeconds) || targetSeconds <= 0) return;

                                const form = document.querySelector('.set-form.timed-form');
                                if (!form) return;

                                const display = root.querySelector('.timed-runner-display');
                                const labelEl = root.querySelector('[data-label]');
                                const timeEl = root.querySelector('[data-time]');
                                const cancelBtn = root.querySelector('[data-cancel]');

                                const PREP_SECONDS = 10;
                                let intervalId = null;
                                let phase = 'idle';
                                let deadline = 0;
                                let lastWhole = -1;
                                let audioCtx = null;
                                let wakeLock = null;

                                // Progressive-enhancement gate: server emits hidden, JS reveals Start.
                                startBtn.hidden = false;

                                function fmt(seconds) {
                                    const s = Math.max(0, Math.ceil(seconds));
                                    const m = Math.floor(s / 60);
                                    const r = s % 60;
                                    return m + ':' + String(r).padStart(2, '0');
                                }

                                function ensureAudio() {
                                    const Ctor = window.AudioContext || window.webkitAudioContext;
                                    if (!Ctor) return null;
                                    if (!audioCtx) audioCtx = new Ctor();
                                    if (audioCtx.state === 'suspended') audioCtx.resume();
                                    return audioCtx;
                                }

                                function playBeep(freq, ms) {
                                    const ctx = ensureAudio();
                                    if (!ctx) return;
                                    const t0 = ctx.currentTime;
                                    const t1 = t0 + ms / 1000;
                                    const osc = ctx.createOscillator();
                                    const gain = ctx.createGain();
                                    osc.type = 'sine';
                                    osc.frequency.value = freq;
                                    gain.gain.setValueAtTime(0, t0);
                                    gain.gain.linearRampToValueAtTime(0.3, t0 + 0.01);
                                    gain.gain.setValueAtTime(0.3, Math.max(t0 + 0.01, t1 - 0.01));
                                    gain.gain.linearRampToValueAtTime(0, t1);
                                    osc.connect(gain).connect(ctx.destination);
                                    osc.start(t0);
                                    osc.stop(t1);
                                }

                                async function acquireWakeLock() {
                                    if (!('wakeLock' in navigator)) return;
                                    try {
                                        wakeLock = await navigator.wakeLock.request('screen');
                                    } catch (err) {
                                        console.debug('wake lock request failed', err);
                                    }
                                }

                                function releaseWakeLock() {
                                    if (!wakeLock) return;
                                    const sentinel = wakeLock;
                                    wakeLock = null;
                                    sentinel.release().catch((err) => console.debug('wake lock release failed', err));
                                }

                                function tick() {
                                    const remainingMs = deadline - Date.now();
                                    const remainingSec = remainingMs / 1000;
                                    timeEl.textContent = fmt(remainingSec);
                                    if (phase === 'prep') {
                                        const whole = Math.ceil(remainingSec);
                                        if (whole !== lastWhole && whole >= 1 && whole <= 3) {
                                            playBeep(800, 120);
                                        }
                                        lastWhole = whole;
                                        if (remainingMs <= 0) beginExercise();
                                    } else if (phase === 'exercise') {
                                        if (remainingMs <= 0) finishExercise();
                                    }
                                }

                                function beginPrep() {
                                    ensureAudio();
                                    acquireWakeLock();
                                    form.hidden = true;
                                    startBtn.hidden = true;
                                    root.hidden = false;
                                    labelEl.textContent = 'Get ready';
                                    display.dataset.phase = 'prep';
                                    phase = 'prep';
                                    deadline = Date.now() + PREP_SECONDS * 1000;
                                    lastWhole = -1;
                                    timeEl.textContent = fmt(PREP_SECONDS);
                                    intervalId = setInterval(tick, 100);
                                }

                                function beginExercise() {
                                    playBeep(1200, 250);
                                    labelEl.textContent = 'Hold';
                                    display.dataset.phase = 'exercise';
                                    phase = 'exercise';
                                    deadline = Date.now() + targetSeconds * 1000;
                                    timeEl.textContent = fmt(targetSeconds);
                                }

                                function finishExercise() {
                                    playBeep(440, 600);
                                    teardown();
                                }

                                function teardown() {
                                    if (intervalId !== null) {
                                        clearInterval(intervalId);
                                        intervalId = null;
                                    }
                                    phase = 'idle';
                                    releaseWakeLock();
                                    root.hidden = true;
                                    display.dataset.phase = 'idle';
                                    form.hidden = false;
                                    startBtn.hidden = false;
                                }

                                startBtn.addEventListener('click', beginPrep);
                                cancelBtn.addEventListener('click', teardown);

                                document.addEventListener('visibilitychange', () => {
                                    if (document.visibilityState === 'visible'
                                            && phase !== 'idle' && !wakeLock) {
                                        acquireWakeLock();
                                    }
                                });

                                window.addEventListener('beforeunload', releaseWakeLock);
                            })();
                        </script>
```

Key invariants:
- All capability checks (`AudioContext`, `navigator.wakeLock`) fall through silently so the countdown still works without sound or wake lock.
- `playBeep` uses an attack/release envelope to avoid click artifacts. The `Math.max(t0 + 0.01, t1 - 0.01)` guard prevents the release schedule from going negative on very short beeps.
- `releaseWakeLock` clears the field before awaiting `release()` so a concurrent `acquire` can re-set it cleanly.
- `tick` runs at 100 ms — fine-grained enough for 1-second chirps without missing crossings.

- [ ] **Step 2: Run the markup test to confirm no regression**

Run: `go test -v ./cmd/web -run Test_application_exerciseSet_time_based_active_timer_markup`

Expected: PASS.

- [ ] **Step 3: Run the full web test suite**

Run: `go test ./cmd/web/...`

Expected: PASS (no test depends on the absence of these elements; the new markup is gated to an active time-based card, which only the dedicated tests instantiate).

- [ ] **Step 4: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "Drive timed-runner: Start, prep, exercise, end, cancel, wake lock"
```

---

## Task 4: Lint, full CI, and manual browser verification

**Goal:** Confirm the change passes the project's standard validation and walk through the user flow once in a real browser.

- [ ] **Step 1: Run lint-fix per project convention**

Run: `make lint-fix`

Expected: clean output, no diff (or autofixes applied — if so, inspect with `git diff` and re-commit before continuing).

- [ ] **Step 2: Run the full test suite**

Run: `make test`

Expected: PASS.

- [ ] **Step 3: Start the dev server**

Run: `make run &` (or whatever the project's dev script is; check `scripts/dev.sh` first if unsure)

Then open `http://localhost:<port>` in a browser, register/log in, set today's workout duration so Plank is scheduled (or add Plank to today's session via the admin UI), navigate to the active Plank set page.

- [ ] **Step 4: Manual sanity check**

Walk through:

1. **Start happy path:** Click `Start timer`. The form hides, runner appears with `Get ready 0:10` and counts to `0:00`. Three short ascending chirps at the 3/2/1 marks, one higher/longer "go" beep at 0. Runner switches to `Hold 0:30` (matching the target) and counts down. End beep at 0. Form re-appears. Start button visible again.
2. **Cancel during prep:** Click Start, then Cancel before prep ends. Runner disappears immediately, no audio plays, form is back.
3. **Cancel during exercise:** Click Start, wait for prep to finish (with the go-beep), then Cancel mid-hold. Runner disappears, no end beep, form is back.
4. **Wake lock:** Click Start. Open DevTools → Application → Service Workers / Background Services or run `await navigator.wakeLock.request('screen')` in the console once — the screen-wake-lock entry should be active during the countdown and gone after. (On a real phone: prop it up, lock screen behavior should be suppressed during a hold; baseline behavior without this change is that the screen dims after the OS timeout.)
5. **No-JS path:** Disable JavaScript in DevTools, reload. The Start button must not appear; the form renders exactly as it did before this change.
6. **Re-run:** After a natural finish, click Start again — second run works identically.

- [ ] **Step 5: Stop the dev server**

Foreground the `make run` job and `Ctrl+C`, or `kill %1`.

- [ ] **Step 6: If manual check exposed any tweak, commit it**

If the manual check led to a small fix (e.g. typo, color tweak, label text), commit it as a follow-up:

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "Polish timed-runner: <what you fixed>"
```

If no fix was needed, no commit; the feature is complete.

---

## Self-Review

Spec coverage:
- "Scope: Active time_based set only" → Task 1 markup is gated on the existing template branch; render test asserts it.
- "Audio source: Synthesized via WebAudio" → Task 3 `playBeep` with `OscillatorNode` + `GainNode`.
- "End-of-timer behavior: Play end beep, restore form" → Task 3 `finishExercise` → `playBeep(440, 600)` → `teardown()` which un-hides the form.
- "Controls during countdown: Cancel only" → Task 1 markup has only `[data-cancel]`; Task 3 wires it.
- "Settings: None" → no DB/handler/preferences changes; nothing in the plan touches those layers.
- "Wake lock: request on Start, release on end/cancel/hide" → Task 3 `acquireWakeLock`, `releaseWakeLock`, `visibilitychange` re-acquire, `beforeunload` release.
- "No-JS fallback: today's form unchanged" → Task 1 markup is `hidden`; Task 3 reveals only when JS runs; Task 4 step 4 manually verifies.
- "Testing: one render test for markup contract, manual browser sanity" → Task 1 step 1 (test), Task 4 step 4 (manual).

Type/name consistency: `data-timed-runner`, `data-timed-runner-start`, `data-target-seconds`, `data-label`, `data-time`, `data-cancel`, `data-phase`, `.timed-runner`, `.timed-runner-display`, `.timed-runner-label`, `.timed-runner-time`, `.timed-runner-cancel`, `.timed-runner-start` — used identically across markup (Task 1), CSS (Task 2), JS (Task 3), and test (Task 1 step 1). `PREP_SECONDS = 10` matches the spec's "fixed 10s". `playBeep` frequencies (800/1200/440) and durations (120/250/600 ms) match the spec's audio section verbatim.

Placeholder scan: no TBDs, no "add appropriate error handling", no "similar to Task N", no missing code blocks. Manual-verification steps are concrete (specific URLs, specific button clicks, specific expected sounds).
