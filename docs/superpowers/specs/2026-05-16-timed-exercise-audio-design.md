# Timed Exercise Audio Cues

## Motivation

For `time_based` exercises (e.g. planks), the active set today shows a target
in seconds and a "Seconds held" form, but nothing actually times the hold. The
user has to watch a separate clock or count in their head — eyes off the
exercise, hands off the bar. Interval-trainer apps solve this with audible
"get ready / go / stop" cues plus an on-screen countdown.

This spec adds a JS-only progressive enhancement to the active time-based set
card: a Start button that runs a 10-second prep countdown, then the
target-seconds exercise countdown, with WebAudio beeps at the transitions and
a screen wake lock for the duration. No-JS users see today's form unchanged.

## Decisions

| Decision | Choice | Reason |
|---|---|---|
| Scope | Active `time_based` set only | Weighted/assisted sets aren't time-bounded; rest-chip already covers the between-set case. |
| Layering | UI-only (`ui/templates/pages/exerciseset/sets-container.gohtml`) | No domain/service/handler change; target seconds already in the rendered context (`CurrentSetTimedTarget`). |
| Audio source | Synthesized via WebAudio oscillator | No asset to ship, no Dockerfile fingerprinting, no cache invalidation, works offline. ~30 lines of JS. |
| End-of-timer behavior | Play end beep, restore the existing manual form | User still chooses No / Barely / Could do more. Matches today's flow; preserves agency if they bailed early or held longer. |
| Controls during countdown | Cancel only | A bobbled Start should be recoverable; Pause is meaningless mid-hold; Skip-to-end is YAGNI. |
| Settings | None — fixed 10s prep, no mute toggle | Device volume / silent switch handles muting; smallest surface area; can add prefs later if real demand emerges. |
| Wake lock | `navigator.wakeLock.request('screen')` for the duration | Phone propped against a wall during a plank shouldn't sleep mid-hold. Released on end, Cancel, or page hide. |
| No-JS fallback | Today's form, unchanged | Server emits no Start button — JS injects it; no new server-rendered elements depend on JS. |

## Architecture

All changes land in one template file:
`ui/templates/pages/exerciseset/sets-container.gohtml`, inside the existing
`{{ else if eq $.ExerciseSet.Exercise.ExerciseType "time_based" }}` branch
(around line 562 today). The pattern mirrors the existing `.rest-chip` block
at lines 403–453: a `<style {{ nonce }}>` for scoped CSS and a
`<script {{ nonce }}>` IIFE for behavior, both adjacent to the markup they
own.

### Components inside that branch

**Markup (server-rendered, all initially `hidden`):**

```html
<div class="timed-runner" data-timed-runner data-target-seconds="{{ .CurrentSetTimedTarget }}" hidden>
    <div class="timed-runner-display" data-phase="idle" aria-live="polite">
        <span class="timed-runner-label" data-label>Get ready</span>
        <span class="timed-runner-time" data-time>0:10</span>
    </div>
    <button type="button" class="timed-runner-cancel" data-cancel>Cancel</button>
</div>
<button type="button" class="timed-runner-start" data-timed-runner-start hidden>Start timer</button>
```

The Start button sits above the existing `<form class="set-form timed-form">`.
The `[hidden]` attribute is removed by JS on load (progressive enhancement
gate) and toggled between Start ↔ runner across phases.

**Scoped CSS (`@scope (.timed-runner)` + a sibling rule for the Start
button).** Big monospace countdown — visual weight comparable to the existing
`<span class="time">` target display. Phase-driven styling via the
`[data-phase="prep"|"exercise"]` attribute (e.g. prep in info color, exercise
in success color, switching at the "go" beep).

**JS module (single IIFE):**

```
state = { phase, deadline, intervalId, audioCtx, wakeLock }

onLoad:
    if (!document.querySelector('[data-timed-runner]')) return
    reveal Start button (remove [hidden])
    if ('wakeLock' in navigator) // capability already exists, log debug
    bind Start click → beginPrep
    bind Cancel click → cancel
    bind document visibilitychange → re-acquire wake lock if still running

beginPrep:
    ensureAudioContext() // first user gesture; resume() if suspended
    acquireWakeLock()
    hide form + Start
    show runner with data-phase="prep", deadline = now + 10s
    startTick()

onTick (every 100ms):
    remaining = deadline - now
    update [data-time] text
    if phase=prep:
        play 800Hz/120ms beep at t=3, t=2, t=1 (one chirp per integer second crossing)
        if remaining ≤ 0: beginExercise()
    if phase=exercise:
        if remaining ≤ 0: finishExercise()

beginExercise:
    play 1200Hz/250ms "go" beep
    set data-phase="exercise", deadline = now + targetSeconds*1000
    swap label text from "Get ready" to "Hold"

finishExercise:
    play 440Hz/600ms end beep
    teardown()

cancel:
    teardown() // no audio

teardown:
    clearInterval, release wake lock, reset phase to "idle"
    hide runner, show form, show Start button (one tap to re-run after either
    cancel or natural finish)
```

`ensureAudioContext` is lazy: constructs `new (window.AudioContext ||
window.webkitAudioContext)()` on first Start click, then `resume()` on every
subsequent Start (handles browsers that re-suspend on tab switch).

`playBeep(freq, ms)` creates an `OscillatorNode` + `GainNode` per call with a
~10ms attack/release envelope, schedules `stop()` at `now + ms/1000`.

Capability checks are silent fall-throughs:
- No `AudioContext` → countdown still runs, no sound.
- No `navigator.wakeLock` → countdown still runs, screen may sleep.

### Tick precision

`setInterval(100ms)` is good enough for human-perceptible timing on a 10–60s
hold; we don't need `requestAnimationFrame`. The `.rest-chip` script uses
250ms and is visibly fine.

For the prep chirps, the tick handler tracks `lastWholeSecond` and fires a
chirp when it crosses 3 → 2, 2 → 1, 1 → 0. This avoids missed or double
chirps if a tick straddles a second boundary.

### Wake Lock lifecycle

1. Request inside the Start click handler (same gesture that unlocks audio).
   Browsers require an active user gesture; piggy-backing on Start is the
   correct pattern.
2. Stash the returned `WakeLockSentinel` on module state.
3. Release on `finishExercise`, `cancel`, and `beforeunload`. Browsers also
   auto-release when the document becomes hidden; our `visibilitychange`
   handler re-requests on `visible` if a countdown is still running. Skip the
   re-request if no countdown is active.
4. Wrap each call in `try/catch` and log to `console.debug` only — wake lock
   failures must never block the timer.

## Testing

- **`cmd/web/handler-exerciseset_test.go` extension:** for an active
  time-based set, assert the rendered HTML contains `[data-timed-runner]`,
  `[data-target-seconds="<n>"]`, and `[data-timed-runner-start]`. For a
  non-active or completed set, assert these are absent. For a non-time-based
  active set (weighted/assisted), assert they are absent.
- **No JS unit tests.** Same convention as the existing `.rest-chip`
  countdown; behavior is covered by manual browser check.
- **Manual browser sanity check (recorded in commit message, not automated):**
  1. Time-based set: Start → prep counts 10→0 with three chirps and a "go"
     beep → exercise counts down → end beep → form re-appears.
  2. Cancel during prep → form returns immediately, no audio.
  3. Cancel during exercise → form returns immediately, no audio.
  4. Lock phone during exercise → screen stays on (wake lock); on iOS Safari
     this is verifiable in DevTools / by observation.
  5. No-JS (disable JS in DevTools) → no Start button, original form
     unchanged.

`make ci` passes; only the one Go render-test assertion is new.

## Acceptance

- An authenticated user with a time-based exercise (e.g. plank) in their
  current workout sees a `Start timer` button above the form when JS is
  enabled.
- Tapping Start hides the form, shows a 10-second prep countdown with three
  ascending chirps at 3/2/1 and a higher "go" beep at 0.
- A target-seconds exercise countdown follows, ending with a lower end beep,
  after which the original form re-appears for the user to submit a signal.
- A Cancel button is available during the prep and exercise phases and
  restores the form silently. After a natural finish or a cancel, the Start
  button is visible again so the timer can be re-run with one tap.
- The screen does not sleep while a countdown is active (on browsers with
  Wake Lock API support).
- With JS disabled, the page renders exactly as it does today.
- `make ci` passes.

## Out of scope

- **Audio cues for non-time-based exercises** (weighted/assisted "between
  reps" or "tempo" beeps). Not requested; would require domain modeling of
  cadence.
- **Per-user mute toggle or custom prep duration.** No UI in `/preferences`,
  no DB column. Add when there's evidence users want it.
- **Pre-recorded sound files.** Synthesizer covers the need with zero
  asset-pipeline cost.
- **Auto-submit on timer end.** User still taps a signal button; preserves
  the existing too-heavy / on-target / too-light feedback loop.
- **Background timer / push notification for the timer itself.** The rest
  timer ([[2026-05-11-rest-timer-design]]) already handles between-set
  background notifications; the exercise timer is foreground-only by nature.
- **Persisting timer state across page reloads.** A reload during a hold
  resets state; that's acceptable for a 10–60s window and avoids server-side
  state.
