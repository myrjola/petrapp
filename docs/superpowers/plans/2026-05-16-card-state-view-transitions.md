# Card-state view transitions on bfcache reload — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the user returns to the workout list after changing an exercise's state on the exerciseset page, the affected card crossfades to its new color and (if newly completed) a strikethrough draws across the exercise name from left to right. All other cards stay still.

**Architecture:** The transition rides on the existing bfcache→reload path (`ui/static/main.js:298-319`). That reload is a cross-document navigation; `@view-transition { navigation: auto }` (already in `ui/static/main.css:580-582`) captures snapshots from both documents. We pair the cards by giving each `view-transition-name: ex-card-{ID}` declaratively (no JS). The strikethrough becomes an `::after` line with its own `view-transition-name: ex-strike-{ID}`, only present on completed cards — the asymmetry between old and new snapshots makes the browser run a "new element" treatment, which we override with a `scaleX(0)→scaleX(1)` draw keyframe.

**Tech Stack:** Go html/template, plain CSS with `@scope` and `<style {{ nonce }}>` colocation, cross-document View Transitions (CSS `@view-transition`), no build step.

**Spec:** [`docs/superpowers/specs/2026-05-16-card-state-view-transitions-design.md`](../specs/2026-05-16-card-state-view-transitions-design.md)

---

## File Structure

**Modified files:**

| File | What changes | Why |
|---|---|---|
| `ui/templates/pages/workout/workout.gohtml` | Rewrite `.exercise.completed .exercise-name` strikethrough into an `::after` pseudo-line; add a dynamic `<style {{ nonce }}>` block before the exercise list emitting per-card `view-transition-name`s and per-completed-card `::view-transition-new(...)` overrides + `strike-draw` keyframe; reduced-motion override | All visual changes live here; both per-card declarations need `.Exercises` data |

**Created files:** none.

**No changes:** Go handlers, services, domain types, `main.js`, `main.css`, or any other template.

---

## Pre-flight verification (Task 0)

This isn't an implementation task — it's a smoke test to confirm the spec's load-bearing assumption (`@view-transition { navigation: auto }` fires on the `navigation.reload()` triggered by the bfcache invalidation handler). Do it before writing any code. If it doesn't fire, stop and re-spec.

**Files:** none modified.

- [ ] **Step 1: Add a temporary `pageswap` / `pagereveal` console log**

Open `ui/templates/pages/workout/workout.gohtml` and locate the existing `<script {{ nonce }}>` block at lines 343–389. Inside it, at the very top (before the existing `URLPattern` declaration), add:

```js
window.addEventListener('pageswap', (e) => {
  console.log('[VT smoke] pageswap', { hasVT: !!e.viewTransition, type: navigation.activation?.navigationType })
})
window.addEventListener('pagereveal', (e) => {
  console.log('[VT smoke] pagereveal', { hasVT: !!e.viewTransition, type: navigation.activation?.navigationType })
})
```

- [ ] **Step 2: Reproduce the bfcache→reload path**

Run `make init` once if you haven't, then `go run ./cmd/web` (or whatever local-run command you use — `make` has no `run` target; just `go run` it). Then in a Chromium-based browser, open the dev server, log in, navigate to a workout for today with an in-progress exercise.

1. Open DevTools console.
2. Click a card → exerciseset page.
3. Mark a set done (POST runs, `inv_bfcache` cookie updates).
4. Click the back link.

Expected console output on the workout list after step 4: `[VT smoke] pagereveal` log with `hasVT: true` and `type: 'reload'`. If `hasVT: false` or no log fires, the spec's assumption is wrong — stop here, do not proceed, and re-spec the approach (likely fallback is manually wrapping the reload in `document.startViewTransition` before invoking it, but that's a same-document transition and would need a different design).

- [ ] **Step 3: Remove the smoke-test logs**

Delete the two `addEventListener` calls you added in Step 1. Do not commit them.

- [ ] **Step 4: Confirm verdict in writing**

In your task report or a scratch note, write one line: "Verified: `pageswap`/`pagereveal` fire on bfcache→reload with `hasVT: true` and `type: 'reload'` on Chromium {version}." If it didn't, the plan changes — escalate.

---

## Task 1 — Replace strike-through with a pseudo-element line

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml` (the `.exercise.completed .exercise-name` rule at lines 243–247 inside the `@scope (.exercise-list)` block)

The strike line must render at full width on cold loads (so any reload that doesn't trigger a view transition still shows it). It uses a semantic success color with a transparency tint, and a `forced-colors: active` fallback.

- [ ] **Step 1: Make `.exercise-name` a positioning context**

Find the existing `.exercise-name` rule inside `@scope (.exercise-list)` (lines 181–186):

```css
.exercise-name {
    font-weight: var(--font-weight-6);
    font-size: var(--font-size-2);
    color: var(--color-text-primary);
    line-height: 1.2;
}
```

Add `position: relative;` so the future `::after` line can be absolutely positioned against it:

```css
.exercise-name {
    font-weight: var(--font-weight-6);
    font-size: var(--font-size-2);
    color: var(--color-text-primary);
    line-height: 1.2;
    position: relative;
}
```

- [ ] **Step 2: Replace `text-decoration: line-through` with the `::after` line**

Find the `.exercise.completed .exercise-name` rule at lines 243–247:

```css
.exercise.completed .exercise-name {
    color: var(--color-success);
    text-decoration: line-through;
    text-decoration-color: color-mix(in oklab, var(--color-success) 40%, transparent);
}
```

Replace with:

```css
.exercise.completed .exercise-name {
    color: var(--color-success);
}

.exercise.completed .exercise-name::after {
    content: "";
    position: absolute;
    left: 0;
    right: 0;
    top: 50%;
    height: 1.5px;
    background: color-mix(in oklab, var(--color-success) 50%, transparent);
    transform: scaleX(1);
    transform-origin: left center;
    pointer-events: none;
}

@media (forced-colors: active) {
    .exercise.completed .exercise-name::after {
        background: CanvasText;
        forced-color-adjust: none;
    }
}
```

The default `transform: scaleX(1)` keeps the line drawn on cold loads. The custom `transform-origin: left center` is set here as well as in the per-strike view-transition rules so that a transform applied during the transition animates from the same anchor.

- [ ] **Step 3: Run handler tests to confirm nothing assertion-level broke**

```
go test ./cmd/web -run TestWorkout -count=1
```

Expected: PASS. The structural change only affects pseudo-element styling — no `.exercise-name` text content, no class names, no attributes change. If anything fails, read the failure and investigate; do not skip.

- [ ] **Step 4: Visually verify cold render**

Load `/workouts/<date>` for a day with at least one completed exercise (hard reload, `Ctrl+Shift+R`, to bypass any view transition path). The completed card should show a thin success-green line straight across the exercise name, vertically centered with the text. If it looks misaligned, tweak `top: 50%` (some fonts will want `top: 55%` or a small `margin-top`) — but the visual goal is "line through the x-height," same as the prior `text-decoration: line-through` rendered.

- [ ] **Step 5: Commit**

```
git add ui/templates/pages/workout/workout.gohtml
git commit -m "Replace exercise-name strike with an ::after line"
```

---

## Task 2 — Emit per-card `view-transition-name` for every exercise

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml` (new dynamic `<style {{ nonce }}>` block adjacent to the exercise list)

We give every card a unique transition name. The browser will pair old and new snapshots by name across the bfcache→reload navigation. Cards whose state is unchanged produce identical snapshots and visually no-op; cards whose state changed crossfade.

- [ ] **Step 1: Add a dynamic style block before the exercise list**

Find the existing `<style {{ nonce }}>` block ending at line 317 (the closing `}` of `@scope (.exercise-list)`, followed by `</style>`). Immediately *after* that closing `</style>`, before the `{{ range .Exercises }}` loop at line 319, insert a new style block:

```gohtml
<style {{ nonce }}>
    {{ range .Exercises }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] { view-transition-name: ex-card-{{ .ID }}; }
    {{ end }}
</style>
```

Each rule pins one card's transition name to its existing `data-workout-exercise-id` attribute (already set at line 322 — no markup change needed).

- [ ] **Step 2: Verify rendered HTML contains the per-card rules**

Run the dev server, load `/workouts/<date>`, view source. Confirm the new `<style>` block contains one line per exercise, e.g.:

```
.exercise[data-workout-exercise-id="42"] { view-transition-name: ex-card-42; }
.exercise[data-workout-exercise-id="43"] { view-transition-name: ex-card-43; }
```

Also check the Network → Doc response or DevTools Elements panel shows the style block under `<div class="exercise-list">`.

- [ ] **Step 3: Verify the transition captures via DevTools**

1. With the dev server running, complete a set on an exercise (so the back navigation will trigger a reload).
2. Open DevTools → Rendering panel → enable "Show view transition" or set "Emulate the CSS media feature `prefers-reduced-motion`" to "No emulation".
3. Navigate to the exerciseset page, then back.
4. While the transition runs (it's quick — slow throttling helps), open DevTools → Animations panel. You should see entries like `::view-transition-group(ex-card-42)`, `::view-transition-old(ex-card-42)`, `::view-transition-new(ex-card-42)`.

If the cards' geometry didn't change (same layout), the group animation is a no-op; only the old/new crossfade runs. If you see only `::view-transition-group(page)` and no `ex-card-*` entries, the names aren't being captured — recheck the HTML output from Step 2 and confirm the `<style>` block sits *outside* the existing `@scope (.exercise-list)` block (scoped styles can't define `view-transition-name` on elements that aren't direct descendants in the way you'd expect; declaring it at root scope is the safer call).

- [ ] **Step 4: Visually verify the color crossfade**

With DevTools' Rendering → "Slow View Transitions" (or temporarily bump the page transition duration), complete an in-progress exercise's last set so it goes from `started` (amber) to `completed` (green). Navigate back. You should see the amber card smoothly bleed into the green card. Other cards stay still. No flicker on unchanged cards.

If unchanged cards flicker (unexpected), the fix is gating `view-transition-name` to only state-changed cards. That requires diffing prev/next state, which we'd need to coordinate via `sessionStorage` or a `pageswap` handler. Don't do that prophylactically — only if the smoke test shows the flicker.

- [ ] **Step 5: Commit**

```
git add ui/templates/pages/workout/workout.gohtml
git commit -m "Name exercise cards for cross-doc view transitions"
```

---

## Task 3 — Add per-strike `view-transition-name` and the draw animation

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml` (extend the dynamic `<style {{ nonce }}>` block from Task 2)

The strike line gets a per-card name on completed cards only. Since the old snapshot (pre-reload, card was not yet completed) has no `::after` line and the new snapshot has one, the browser runs the default "new element appears" treatment — which we override with a `scaleX` draw.

- [ ] **Step 1: Extend the dynamic style block with per-strike names and overrides**

Extend the block you added in Task 2. Replace it with:

```gohtml
<style {{ nonce }}>
    {{ range .Exercises }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] { view-transition-name: ex-card-{{ .ID }}; }
    {{ if eq .State "completed" }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] .exercise-name::after { view-transition-name: ex-strike-{{ .ID }}; }
    ::view-transition-new(ex-strike-{{ .ID }}) {
        animation: strike-draw 320ms cubic-bezier(.2, .7, .3, 1) both;
        transform-origin: left center;
    }
    {{ end }}
    {{ end }}

    @keyframes strike-draw {
        from { transform: scaleX(0); }
        to   { transform: scaleX(1); }
    }
</style>
```

`.State` is `domain.ExerciseSetState` (a named string type). Go's template `eq` compares string-kind values regardless of the named type, so `eq .State "completed"` works the same as `eq $.ExerciseSet.Exercise.ExerciseType "assisted"` already does in `ui/templates/pages/exerciseset/sets-container.gohtml:476`.

- [ ] **Step 2: Verify the strike's name shows up only on completed cards**

Reload `/workouts/<date>` and view source. For each completed card, confirm two new rules per card:

```
.exercise[data-workout-exercise-id="42"] .exercise-name::after { view-transition-name: ex-strike-42; }
::view-transition-new(ex-strike-42) { animation: strike-draw 320ms ...; transform-origin: left center; }
```

Non-completed cards should have *only* the `ex-card-{ID}` rule, not the strike rules. Confirm by counting: rule blocks should equal `N + 2 * (number of completed cards)`.

- [ ] **Step 3: Reproduce the bfcache→reload path and watch the strike draw**

Steps to repro:

1. Open a workout with an in-progress exercise (sets started but not all done).
2. Open the exerciseset page, log the *last* remaining set so the exercise flips to `completed`.
3. Click back.

Expected: the just-completed card crossfades cream→green (per Task 2), and *during the same transition* a thin green line draws across the exercise name from left to right over ~320ms. Other completed-from-before cards do not redraw (because their `::after` exists in both snapshots — identical-snapshot crossfade is a no-op).

If the strike just fades in instead of drawing, the `::view-transition-new(ex-strike-{ID})` rule isn't winning over the browser default. Check the rendered HTML for typos, then check the DevTools Animations panel — the animation name should read `strike-draw`. If it reads `-ua-view-transition-fade-out`, the rule isn't applying; verify the rule emission is happening before `</style>` close.

- [ ] **Step 4: Commit**

```
git add ui/templates/pages/workout/workout.gohtml
git commit -m "Draw the exercise-name strike during the bfcache reload transition"
```

---

## Task 4 — Reduced-motion override for the strike draw

**Files:**
- Modify: `ui/templates/pages/workout/workout.gohtml` (extend the dynamic `<style {{ nonce }}>` block from Task 3)

`main.css:670-674` already collapses the page transition to 1ms under `prefers-reduced-motion: reduce`. The `strike-draw` keyframe needs the same treatment so the line snaps instead of drawing.

- [ ] **Step 1: Add the `@media (prefers-reduced-motion: reduce)` block**

Inside the same dynamic `<style {{ nonce }}>` block (after the `@keyframes strike-draw` declaration), add:

```gohtml
@media (prefers-reduced-motion: reduce) {
    {{ range .Exercises }}{{ if eq .State "completed" }}
    ::view-transition-new(ex-strike-{{ .ID }}) { animation-duration: 0.001ms; }
    {{ end }}{{ end }}
}
```

The full block now reads:

```gohtml
<style {{ nonce }}>
    {{ range .Exercises }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] { view-transition-name: ex-card-{{ .ID }}; }
    {{ if eq .State "completed" }}
    .exercise[data-workout-exercise-id="{{ .ID }}"] .exercise-name::after { view-transition-name: ex-strike-{{ .ID }}; }
    ::view-transition-new(ex-strike-{{ .ID }}) {
        animation: strike-draw 320ms cubic-bezier(.2, .7, .3, 1) both;
        transform-origin: left center;
    }
    {{ end }}
    {{ end }}

    @keyframes strike-draw {
        from { transform: scaleX(0); }
        to   { transform: scaleX(1); }
    }

    @media (prefers-reduced-motion: reduce) {
        {{ range .Exercises }}{{ if eq .State "completed" }}
        ::view-transition-new(ex-strike-{{ .ID }}) { animation-duration: 0.001ms; }
        {{ end }}{{ end }}
    }
</style>
```

- [ ] **Step 2: Verify with `prefers-reduced-motion: reduce` enabled**

DevTools → Rendering panel → "Emulate CSS media feature `prefers-reduced-motion`" → "reduce". Repeat the bfcache→reload repro from Task 3 Step 3. The card colors should snap (existing page-level reduced-motion override handles this), and the strike line should appear without any visible draw — instantly at full width. Compare against the same flow with reduced motion off (the strike should draw smoothly).

- [ ] **Step 3: Commit**

```
git add ui/templates/pages/workout/workout.gohtml
git commit -m "Honor prefers-reduced-motion for the strike draw"
```

---

## Task 5 — Run the spec's full test plan and lint

**Files:** none modified (verification only). If issues turn up, file follow-up tasks rather than fixing in-line — each fix should be its own commit with its own justification.

- [ ] **Step 1: Manual test matrix (from the spec)**

Run through each case in [`spec § Test plan`](../specs/2026-05-16-card-state-view-transitions-design.md). Expected outcomes:

1. *Complete an exercise, navigate back.* Card crossfades amber → green, strike draws.
2. *Re-open an already-completed exercise, back out.* No reload (cookie unchanged) → instant return, no animation.
3. *Start (but don't complete) sets on a different card, back out.* That card crossfades cream → amber. No strike (still not completed).
4. *Repeat (1) with `prefers-reduced-motion: reduce`.* Strike snaps in; colors snap.
5. *Cold-load `/workouts/<date>` with mixed states.* Strike lines render at full width on completed cards. No animation on first paint.

For each, note pass/fail. Any failure is a spec-violation regression — investigate before claiming done.

- [ ] **Step 2: Run `make lint-fix` and `make test`**

```
make lint-fix && make test
```

Expected: clean lint output (no diff), all tests pass. The template change has no Go signature impact, so a clean test pass plus clean lint is sufficient.

- [ ] **Step 3: Final commit (only if Step 2 produced changes)**

If `make lint-fix` modified anything, commit it:

```
git add -A
git commit -m "Lint-fix"
```

Otherwise no commit.

- [ ] **Step 4: Mark plan complete**

In your subagent/inline-execution status, report: plan complete, N commits, links to commits, brief one-line summary of verified outcomes from Step 1.

---

## Notes for the implementer

- **Don't break the existing title morph.** The `pageswap`/`pagereveal` script in `ui/templates/pages/workout/workout.gohtml:343-389` is unrelated to this change — leave it alone. It handles a *different* transition (workout list ↔ exerciseset page, title element). Per-card transitions added here ride on a different navigation (reload, same URL).
- **Don't add `view-transition-name` to elements via inline `style=` attributes.** CSP nonce mode disables inline styles entirely (see `ui/templates/CLAUDE.md`). Always emit via a nonce'd `<style>` block.
- **Don't mass-rename `data-workout-exercise-id`.** It's already used by the title-morph script (line 359, 379). The pattern is `data-workout-exercise-id="{{ .ID }}"`; reuse, don't replace.
- **Test on a real device too if possible.** The bfcache path behaves slightly differently on iOS Safari and mobile Chromium than on desktop Chromium. The graceful fallback for any browser that doesn't run the cross-doc transition is "hard reload, no animation" — same as today.
