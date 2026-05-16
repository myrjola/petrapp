# Aesthetics Uplift — Increment 6: Motion & Microinteraction Pass — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the warm Stone palette *feel* alive — add a global button press, a card hover-lift, a rest-chip "Ready" bloom, and tune the existing view-transition shrink — all on a quiet-ease-out motion grammar that honours `prefers-reduced-motion`.

**Architecture:** Increments 1–5 are pure colour work; the existing motion in `ui/static/main.css` (`@keyframes shrink/grow/slide-in/slide-out`, `@view-transition`, the `prefers-reduced-motion` block) is the established pattern this increment extends. Five new motion tokens land in `@layer props`; one global press rule lands on `button, .btn`; one global hover-lift rule lands on `a.card`; the existing `shrink` keyframe softens; the `.rest-chip` scoped block in `sets-container.gohtml` gains a colour transition so the `.ready` swap blooms; and the reduced-motion `@media` block at the bottom of `main.css` is extended to neutralise every new transform and to collapse the view transition. No new fonts, no new JS, no markup change, no test edits — the existing DOM stays bit-identical, so `goquery` / Playwright tests are untouched.

**Tech Stack:** CSS (custom properties, `@layer`, `@scope`, CSS view transitions, `@media (prefers-reduced-motion: reduce)`), Go `html/template`, Go (`cmd/web` handler + `goquery`-based e2e tests for the render gates).

**Spec:** `docs/superpowers/specs/2026-05-14-aesthetics-uplift-design.md` — the original Increment 6 paragraph plus the "Increment 6 — Design notes (2026-05-16)" appendix, which pins the concrete values for everything below.

---

## File Structure

### Modified files

| File | Change |
|---|---|
| `ui/static/main.css` | Add five motion tokens (`--ease-out-quiet`, `--duration-1..4`) to `@layer props`. Add `transition` + `&:active:not([aria-busy="true"]) { transform: scale(0.97); }` to the `button, .btn` rule. Add an `a.card` rule with a `translateY(-1px)` + `--shadow-2` hover and a settle-back active. Soften the `shrink` keyframe from `scale: 0.8; opacity: 0.5` to `scale: 0.96; opacity: 0`, and replace the literal `animation-duration: 0.3s` on `::view-transition-group(page)` with `var(--duration-4)`. Extend the existing `@media (prefers-reduced-motion: reduce)` block: drop the button-press transform, drop the card-lift transform (keep the shadow swap), collapse the view transition to `animation-duration: 0.001ms`. |
| `ui/templates/pages/exerciseset/sets-container.gohtml` | Add a single `transition: background-color … color …` line to the `@scope (.rest-chip) :scope` rule so the swap to `:scope.ready` (`--color-info-bg` → `--color-success-bg`) blooms over 240 ms instead of snapping. |

### Out of scope (untouched)

- The 26 scattered `transition: ... 0.2s ease` sites across `ui/templates/pages/**/*.gohtml`. Refactoring them onto the new motion tokens is pure cleanup, not motion design — separate sweep if ever warranted. Existing inline values continue to work.
- The scoped Focus-mode buttons in `sets-container.gohtml` (`.submit-button`, `.signal-btn`, `.on-target-btn`, `.too-heavy-btn`, `.too-light-btn`) — already have their own tuned motion grammar (the `translateY(-1px)` hover, outline-shadow active). The global button rule does NOT reach them because their scoped rules win on source order, which is the desired behaviour.
- The warmup-complete button in `warmup.gohtml` — same reason. Already has its own `translateY(-1px)` hover.
- The `back-link` partial — already transitions `background-color` and `color` over 0.2s.
- The home `.day-card` — it is a `<div>`, not an `<a>`. The `a.card` rule does not match it. The day-cards are informational tiles with an inner `<a class="btn">` action, not whole-card click targets, and they already have status-tinted backgrounds that read as the visual interest.
- The loading-bar shimmer (`#loading-bar`, `@keyframes loading-bar-slide`) — already exists, already reduced-motion-aware.
- The `button-spinner` keyframe — already exists, already reduced-motion-aware.
- All Go handlers, all templates other than the two listed above, all tests — no DOM or class-name changes are introduced, so no test edits are needed.

---

## Token & rule reference

The single source of truth for every value in this increment. Each value is pinned by the spec appendix's "Design language" and "Microinteraction targets" sections.

**Motion tokens (added to `@layer props :where(html)`):**

| Token | Value | Used by |
|---|---|---|
| `--ease-out-quiet` | `cubic-bezier(0.2, 0, 0, 1)` | Every new transition in this increment |
| `--duration-1` | `80ms` | Button press transform |
| `--duration-2` | `160ms` | Hover colour, hover border |
| `--duration-3` | `240ms` | Card hover-lift, rest-chip bloom |
| `--duration-4` | `320ms` | `::view-transition-group(page)` animation |

**Targets:**

| Target | Rule (the new bits only) | Reduced-motion fallback |
|---|---|---|
| `button, .btn` | `transition: background-color var(--duration-2) var(--ease-out-quiet), transform var(--duration-1) var(--ease-out-quiet);` plus `&:active:not([aria-busy="true"]) { transform: scale(0.97); }` | Drop the `transform` rule (colour still transitions, which is paint not motion) |
| `a.card` | `transition: transform var(--duration-3) var(--ease-out-quiet), box-shadow var(--duration-3) var(--ease-out-quiet), border-color var(--duration-2) var(--ease-out-quiet);` plus `&:hover { transform: translateY(-1px); box-shadow: var(--shadow-2); }` and `&:active { transform: translateY(0); box-shadow: var(--shadow-1); }` | Drop both `transform`s; keep the shadow swap |
| `@keyframes shrink` | `to { scale: 0.96; opacity: 0; }` (was `scale: 0.8; opacity: 0.5`) | Covered by the view-transition reduce rule below — no per-keyframe change |
| `::view-transition-group(page)` | `animation-duration: var(--duration-4);` (was `0.3s`) | `animation-duration: 0.001ms !important;` on `::view-transition-group(page), ::view-transition-old(page), ::view-transition-new(page)` |
| `@scope (.rest-chip) :scope` | Add `transition: background-color var(--duration-3) var(--ease-out-quiet), color var(--duration-3) var(--ease-out-quiet);` | None — colour swap, not motion |

`0.001ms` rather than `0s` for the view-transition collapse so the `animationend` event still fires for any future listener. Defensive but free.

---

## Sequencing rationale

Six tasks. Tokens first (everything else references them); then each microinteraction in its own task with its reduced-motion guard bundled in, so the codebase is never temporarily in a "motion without the a11y guard" state; then the final `make ci` gate.

The order between Tasks 2–5 is independent — they touch disjoint rules — but I sequence them roughly by visual prominence (button press is felt everywhere; card lift is felt on the workout overview; view-transition polish is felt on every navigation; rest-chip bloom is felt only mid-workout). If a task surfaces a Playwright/goquery flake (the stop-condition the briefing flags as the most likely real risk), stopping at the previous task still leaves a coherent, shippable surface.

**Verification shape.** CSS motion changes have no compiler — no unit test catches a wrong easing curve. Each task is verified by (a) a `grep` gate that the touched rule actually landed in the file, (b) a render test against a route that exercises the touched surface (proves the CSS parses and the page still serves 200), and (c) a described visual check (the user can drive a browser; the agent cannot, so report visual checks as "described, not performed"). Task 6 is the full `make ci`.

---

## Task 1: Add the motion tokens to `@layer props`

**Files:**
- Modify: `ui/static/main.css` (`@layer props` block)

The five tokens land at the bottom of the existing `:where(html)` props block, in a new `/* Motion */` group, immediately after the existing `/* Semantic colors */` group. No call sites yet — the next four tasks consume them.

- [ ] **Step 1: Add the motion-token block**

In `ui/static/main.css`, find the end of the `/* Semantic colors */` group (the line `--color-info-bg: #dde8ec;` immediately followed by a `}` closing `:where(html)`, around lines 227–228). Insert a blank line and a new group before that closing brace.

Replace:

```css
        --color-info: #3f5a68;
        --color-info-bg: #dde8ec;
    }
}
```

with:

```css
        --color-info: #3f5a68;
        --color-info-bg: #dde8ec;

        /* Motion */
        --ease-out-quiet: cubic-bezier(0.2, 0, 0, 1);
        --duration-1: 80ms;
        --duration-2: 160ms;
        --duration-3: 240ms;
        --duration-4: 320ms;
    }
}
```

- [ ] **Step 2: Verify the tokens parse**

Run: `grep -nE '\-\-(ease-out-quiet|duration-[1-4]):' ui/static/main.css`
Expected: five lines of output, one per token, all under `@layer props`.

- [ ] **Step 3: Render check via the styleguide**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v -count=1`
Expected: PASS. The styleguide page serves `main.css` and parses every layer; if the new tokens break the layer, the page returns 500 and this test catches it.

- [ ] **Step 4: Commit**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Add the motion token group to main.css @layer props

Introduces --ease-out-quiet plus --duration-1..4 (80/160/240/320ms) as
the motion vocabulary for the rest of Increment 6. No call sites yet;
each subsequent task adds one.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Global button press

**Files:**
- Modify: `ui/static/main.css` (`@layer components` `button, .btn` rule; bottom `@media (prefers-reduced-motion: reduce)` block)

Adds the transition + `:active` scale to every `button` and `.btn` in the app. Honoured by reduced-motion via a single new rule in the existing media block.

- [ ] **Step 1: Add the transition declaration and the `:active` press rule to `button, .btn`**

In `ui/static/main.css`, in the `@layer components` block, find the `button, .btn` rule (around lines ~258–317). Replace this exact span:

```css
    button, .btn {
        position: relative;
        display: inline-flex;
        border-radius: var(--radius-3);
        background-color: var(--clay-4);
        color: var(--color-surface-elevated);
        font-family: var(--font-sans);
        font-size: var(--font-size-1);
        font-weight: var(--font-weight-7);
        letter-spacing: var(--font-letterspacing-3);
        --vertical-padding: 2.5em;
        padding: var(--size-2) var(--vertical-padding);
        border: none;
        white-space: nowrap;
        text-decoration: none;

        &:hover {
            cursor: pointer;
            background-color: var(--clay-5);
        }

        &:focus-visible {
            outline: var(--clay-3) solid 2px;
        }
```

with (one new `transition:` line in the opening declarations; one new `&:active:not([aria-busy="true"])` rule inserted between `&:focus-visible` and the existing `&[aria-busy="true"]` rule that follows it):

```css
    button, .btn {
        position: relative;
        display: inline-flex;
        border-radius: var(--radius-3);
        background-color: var(--clay-4);
        color: var(--color-surface-elevated);
        font-family: var(--font-sans);
        font-size: var(--font-size-1);
        font-weight: var(--font-weight-7);
        letter-spacing: var(--font-letterspacing-3);
        --vertical-padding: 2.5em;
        padding: var(--size-2) var(--vertical-padding);
        border: none;
        white-space: nowrap;
        text-decoration: none;
        transition: background-color var(--duration-2) var(--ease-out-quiet), transform var(--duration-1) var(--ease-out-quiet);

        &:hover {
            cursor: pointer;
            background-color: var(--clay-5);
        }

        &:focus-visible {
            outline: var(--clay-3) solid 2px;
        }

        &:active:not([aria-busy="true"]) {
            transform: scale(0.97);
        }
```

Nothing further down in the `button, .btn { ... }` block changes — the `&[aria-busy="true"]` rule and its descendants stay exactly as they are.

- [ ] **Step 2: Extend the reduced-motion block to drop the button-press transform**

In `ui/static/main.css`, find the existing `@media (prefers-reduced-motion: reduce) { ... }` block (around lines 495–507). It currently contains the loading-bar and button-spinner overrides:

```css
@media (prefers-reduced-motion: reduce) {
    #loading-bar {
        transition: none;
    }
    #loading-bar.active {
        animation: none;
        background: var(--clay-3);
    }
    button[aria-busy="true"]::after,
    .btn[aria-busy="true"]::after {
        animation: none;
    }
}
```

Replace it with (additions at the bottom, before the closing brace):

```css
@media (prefers-reduced-motion: reduce) {
    #loading-bar {
        transition: none;
    }
    #loading-bar.active {
        animation: none;
        background: var(--clay-3);
    }
    button[aria-busy="true"]::after,
    .btn[aria-busy="true"]::after {
        animation: none;
    }
    button, .btn {
        transition: background-color var(--duration-2) var(--ease-out-quiet);
    }
    button:active:not([aria-busy="true"]),
    .btn:active:not([aria-busy="true"]) {
        transform: none;
    }
}
```

The two rules together: keep the colour transition (colour is paint, not motion), but suppress the `transform` declaration and any active-press scaling.

- [ ] **Step 3: Verify the rules parse**

Run: `grep -nE 'transition: background-color var\(--duration-2\) var\(--ease-out-quiet\), transform var\(--duration-1\)|scale\(0\.97\)|transform: none;' ui/static/main.css`
Expected: three matching lines — the new `transition` declaration, the `scale(0.97)` active rule, and the `transform: none` reduced-motion override.

- [ ] **Step 4: Render check via the styleguide (covers the global `.btn`)**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v -count=1`
Expected: PASS. The styleguide renders every button variant; if the rule breaks the layer parsing, the route 500s and this catches it.

- [ ] **Step 5: Visual check (described — cannot drive a browser)**

Intended result on `make dev` → `/dev/styleguide`: hovering any button fades into its clay-5 hover colour over ~160ms (felt as "soft", not "snap"); pressing-and-holding any button briefly compresses to 97% (felt as a polite "click acknowledged"); releasing returns to rest. The loading-state busy spinner still shows when `aria-busy="true"`; pressing a busy button does *not* compress (the `:not([aria-busy="true"])` guard). Under OS-level reduced motion, the colour fades remain but the press scale disappears entirely. Report as "described, not visually verified".

- [ ] **Step 6: Commit**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Add the global button press scale to button, .btn

Every primary CTA in the app now fades its hover colour over 160ms and
compresses to scale(0.97) under :active, gated by
:not([aria-busy="true"]) so loading buttons don't fight their spinner.
Reduced-motion drops the transform but keeps the colour fade.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `a.card` hover-lift

**Files:**
- Modify: `ui/static/main.css` (`@layer components`, after the existing `.card` rule; bottom `@media (prefers-reduced-motion: reduce)` block)

Adds a 1px hover-lift and shadow swap to *anchor* cards — the workout exercise-list row is the major call site. The plain `.card` (used by informational tiles like the home day-card div) is unaffected.

- [ ] **Step 1: Add the `a.card` rule immediately after the existing `.card` rule**

In `ui/static/main.css`, find the existing `.card` rule (around lines 368–375):

```css
    .card {
        display: block;
        padding: var(--size-3);
        background: var(--color-surface-elevated);
        border: var(--border-size-1) solid var(--color-border);
        border-radius: var(--radius-3);
        box-shadow: var(--shadow-1);
    }
}
```

Replace it with (the existing rule unchanged, plus the new `a.card` rule, plus the closing brace of `@layer components`):

```css
    .card {
        display: block;
        padding: var(--size-3);
        background: var(--color-surface-elevated);
        border: var(--border-size-1) solid var(--color-border);
        border-radius: var(--radius-3);
        box-shadow: var(--shadow-1);
    }

    a.card {
        transition:
            transform var(--duration-3) var(--ease-out-quiet),
            box-shadow var(--duration-3) var(--ease-out-quiet),
            border-color var(--duration-2) var(--ease-out-quiet);

        &:hover {
            transform: translateY(-1px);
            box-shadow: var(--shadow-2);
        }

        &:active {
            transform: translateY(0);
            box-shadow: var(--shadow-1);
        }
    }
}
```

- [ ] **Step 2: Extend the reduced-motion block to drop the card-lift transform**

In `ui/static/main.css`, in the same `@media (prefers-reduced-motion: reduce) { ... }` block at the bottom, add this rule just before the closing brace (after the button-press rules from Task 2):

Find the current end of the block (after Task 2):

```css
    button:active:not([aria-busy="true"]),
    .btn:active:not([aria-busy="true"]) {
        transform: none;
    }
}
```

Replace with:

```css
    button:active:not([aria-busy="true"]),
    .btn:active:not([aria-busy="true"]) {
        transform: none;
    }
    a.card:hover,
    a.card:active {
        transform: none;
    }
}
```

The shadow swap and border-colour transition still fire — the only thing dropped is the geometry change.

- [ ] **Step 3: Verify the rule parses**

Run: `grep -nE 'a\.card \{|translateY\(-1px\)|a\.card:hover,|a\.card:active' ui/static/main.css`
Expected: four matching lines — the rule selector, the hover transform, and the two reduced-motion overrides.

- [ ] **Step 4: Render check via the workout overview (covers `a.exercise.card`)**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v -count=1`
Expected: PASS. The styleguide includes plain `.card` examples (lines 317–363 of `styleguide.gohtml`); the workout-overview tests aren't strictly needed for parse-check, but the styleguide is sufficient.

- [ ] **Step 5: Visual check (described — cannot drive a browser)**

Intended result on `make dev`: navigate to `/workouts/<today>` mid-workout. Hovering an exercise row in the list lifts it 1px and elevates its shadow from subtle (`--shadow-1`) to card (`--shadow-2`) over ~240ms (felt as "I'm responsive", not "I'm jumping at you"). Pressing settles the card back to its resting shadow. Plain `<div class="card">` surfaces (admin-exercises form sections, styleguide card samples) are unchanged — no hover effect, no shadow change, because they are not anchors. Home day-cards (also `<div>`) are unaffected. Under reduced motion: the shadow still elevates on hover, but the card doesn't move. Report as "described, not visually verified".

- [ ] **Step 6: Commit**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Add the a.card 1px hover-lift to interactive card surfaces

Anchor cards (workout exercise list, future anchor-wrapped cards) now
rise 1px on hover with shadow lifting from --shadow-1 to --shadow-2,
settling back on active. Plain .card divs are unchanged. Reduced-motion
drops the transform but keeps the shadow swap.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: View-transition polish

**Files:**
- Modify: `ui/static/main.css` (`@keyframes shrink` and `::view-transition-group(page)`; bottom `@media (prefers-reduced-motion: reduce)` block)

Two literal-value tweaks plus the reduced-motion collapse. Softens the page-exit shrink and aligns the duration with the new token system.

- [ ] **Step 1: Soften the `shrink` keyframe**

In `ui/static/main.css`, find the existing `@keyframes shrink` (around lines 378–383):

```css
@keyframes shrink {
    to {
        scale: 0.8;
        opacity: 0.5;
    }
}
```

Replace with:

```css
@keyframes shrink {
    to {
        scale: 0.96;
        opacity: 0;
    }
}
```

- [ ] **Step 2: Swap the literal `0.3s` for the duration token**

In the same file, find the `::view-transition-group(page)` rule (around lines 440–442):

```css
::view-transition-group(page) {
    animation-duration: 0.3s;
}
```

Replace with:

```css
::view-transition-group(page) {
    animation-duration: var(--duration-4);
}
```

- [ ] **Step 3: Extend the reduced-motion block to collapse the view transition**

In `ui/static/main.css`, in the same `@media (prefers-reduced-motion: reduce) { ... }` block, add the view-transition override just before the closing brace (after the card-lift rule from Task 3).

Find the current end of the block (after Task 3):

```css
    a.card:hover,
    a.card:active {
        transform: none;
    }
}
```

Replace with:

```css
    a.card:hover,
    a.card:active {
        transform: none;
    }
    ::view-transition-group(page),
    ::view-transition-old(page),
    ::view-transition-new(page) {
        animation-duration: 0.001ms !important;
    }
}
```

`!important` is required because the `animation-duration: var(--duration-4)` on `::view-transition-group(page)` has equal specificity; without `!important`, source order wins (and the override would have to live below the rule it overrides, outside the `@media` block). With `!important` inside the media query, the override wins under reduced motion only — the cleanest expression of the intent.

- [ ] **Step 4: Verify the changes**

Run: `grep -nE 'scale: 0\.96|animation-duration: var\(--duration-4\)|animation-duration: 0\.001ms' ui/static/main.css`
Expected: three matching lines — the softened keyframe, the tokenised duration, and the reduced-motion collapse.

Then: `grep -nE 'scale: 0\.8;|opacity: 0\.5;|animation-duration: 0\.3s' ui/static/main.css`
Expected: no output (the old values are gone).

- [ ] **Step 5: Render check via the styleguide**

Run: `go test ./cmd/web/ -run Test_application_styleguide -v -count=1`
Expected: PASS. View-transition rules live in `main.css`; a parse error caused by a missing brace would 500 the styleguide.

- [ ] **Step 6: Visual check (described — cannot drive a browser)**

Intended result on `make dev`: navigate from `/` to `/workouts/<today>`, then tap Back. The forward navigation should *feel* a touch cleaner — the old page recedes a fraction (`scale: 0.96`) and cleanly fades out (`opacity: 0`) instead of half-fading at 0.5 opacity behind the new page. The 320ms duration is virtually indistinguishable from the previous 300ms but now matches `--duration-4`. Under OS-level reduced motion, the page swap is instant (`0.001ms`) — no animation, but the snapshot/swap mechanism still runs so navigation works normally. Report as "described, not visually verified".

- [ ] **Step 7: Commit**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Soften the view-transition shrink and tokenise its duration

The page-exit shrink now scales to 0.96 and fades to opacity 0 instead
of scaling to 0.8 and hovering at 0.5 — the old page recedes cleanly
rather than ghosting behind the new one. Duration moves from 0.3s onto
var(--duration-4) (320ms). Reduced-motion collapses the transition to
0.001ms so the snapshot mechanism still fires for any future listener
but no animation plays.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Rest-chip "Ready" bloom

**Files:**
- Modify: `ui/templates/pages/exerciseset/sets-container.gohtml` (the `@scope (.rest-chip) :scope` block)

The single template edit in this increment. Adds a 240ms colour transition so the class swap from default to `.ready` blooms instead of snapping.

- [ ] **Step 1: Add the `transition` line to the rest-chip scoped block**

In `ui/templates/pages/exerciseset/sets-container.gohtml`, find the `@scope (.rest-chip)` block (around lines 406–423):

```css
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
```

Replace with (the new `transition` line appended to `:scope { ... }`, before its closing brace):

```css
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
                        transition: background-color var(--duration-3) var(--ease-out-quiet), color var(--duration-3) var(--ease-out-quiet);
                    }
                    :scope.ready {
                        background: var(--color-success-bg);
                        color: var(--color-success);
                    }
                }
```

- [ ] **Step 2: Verify the rule landed**

Run: `grep -n 'transition: background-color var(--duration-3) var(--ease-out-quiet), color' ui/templates/pages/exerciseset/sets-container.gohtml`
Expected: one matching line, inside the `.rest-chip` scoped block.

- [ ] **Step 3: Render check via the exercise-set page**

Run: `go test ./cmd/web/ -run Test_application_exerciseSet -v -count=1`
Expected: PASS. This exercises the rendering of `sets-container.gohtml` — if the new CSS line broke the scoped block, the route would 500.

- [ ] **Step 4: Visual check (described — cannot drive a browser)**

Intended result on `make dev`: start a set with a rest period (e.g., trigger an in-progress workout, complete a set). The rest chip appears with the calm `--color-info-bg` blue-grey background and `--color-info` text, counting down. When the timer hits 0, the chip swaps class (`.ready` is added by the inline JS) and *blooms* over ~240ms from blue-grey/blue → sage/sage with the "Ready" text. The bloom rewards the wait — the moment the user is watching for. Under reduced motion there is no special guard because colour-only transitions are paint, not motion. Report as "described, not visually verified".

- [ ] **Step 5: Commit**

```bash
git add ui/templates/pages/exerciseset/sets-container.gohtml
git commit -m "$(cat <<'EOF'
Bloom the rest-chip from blue-grey to sage on .ready

The rest timer chip's class-swap from default to .ready (info → success)
now transitions its background-color and color over 240ms instead of
snapping, so the moment the user is waiting for arrives as a brief
"ready" cue rather than a flicker.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Final full-suite gate

**Files:**
- None — verification only.

- [ ] **Step 1: Run the full CI suite**

Run: `make ci`
Expected: green. This runs `init + build + lint-fix + test + sec` as documented in `CLAUDE.md`. If it fails, the failure pinpoints the regression (lint issue from a stray formatting tic, a test that grew a flake from a CSS change — though no DOM hooks change in this increment, so the test failures should be zero).

- [ ] **Step 2: Report**

Confirm the final commit list (5 commits, one per implementation task plus the spec appendix from before the plan):

Run: `git log --oneline main..HEAD`
Expected: 6 lines — the spec appendix commit and the five implementation commits, in order:

```
<sha> Bloom the rest-chip from blue-grey to sage on .ready
<sha> Soften the view-transition shrink and tokenise its duration
<sha> Add the a.card 1px hover-lift to interactive card surfaces
<sha> Add the global button press scale to button, .btn
<sha> Add the motion token group to main.css @layer props
<sha> Spec: Increment 6 — motion design notes appendix
```

If `make ci` is green and the log is as expected, hand off to `superpowers:finishing-a-development-branch` to merge `feature/inc-6-motion` into local `main`.
