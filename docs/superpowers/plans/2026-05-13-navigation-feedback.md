# Navigation Feedback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add instant button/link text confirmation and a 300 ms-delayed loading bar to make slow MPA navigations feel responsive, for both intercepted form POSTs and pass-through GETs.

**Architecture:** Persistent `#loading-bar` div + visually-hidden `#loading-announce` live region live in `base.gohtml`. JS module-level state (`activeLoad`) tracks the in-flight target and timer; `startLoad`/`clearLoad` helpers manage lifecycle. The Navigation API `navigate` event listener is restructured to cover both POST and GET paths; `navigateerror` and `pageshow.persisted` clear stale state.

**Tech Stack:** Go html/template (`base.gohtml`), vanilla CSS in `main.css`, vanilla JS in `main.js` using the Navigation API. No build step; templates and static assets are served from disk in dev (`os.DirFS`).

**Why no automated tests:** The Go template-tests don't drive client JS, and the project has no Playwright/Cypress harness. Per the spec, verification is manual at implementation time. The final task walks each scenario; do not declare the plan done without running through them.

**Reference spec:** `docs/superpowers/specs/2026-05-13-navigation-feedback-design.md`

---

## File Structure

| File | Change | Responsibility |
|---|---|---|
| `ui/templates/base.gohtml` | Modify | Adds `#loading-bar` (visual) and `#loading-announce` (SR live region) as the first children of `<body>`. |
| `ui/static/main.css` | Modify | Adds bar styles (base, active animation, reduced-motion fallback, forced-colors fallback). Removes the dead `form.submitting` spinner block. |
| `ui/static/main.js` | Modify | Adds `activeLoad` module state, `startLoad`/`clearLoad` helpers, restructures the `navigate` listener for both POST and GET, adds a `navigateerror` listener, and augments the existing `pageshow` listener to call `clearLoad`. |

No new files. No Go code touched. The string `Loading…` uses the horizontal ellipsis character (U+2026) consistently.

A small deviation from the spec's literal wording: the spec writes `el.innerText = 'Loading…'`, but the rest of the codebase consistently uses `textContent` (see `errorEl.textContent = msg` in `preferences.gohtml`, `timeEl.textContent = …` in `sets-container.gohtml`). The behaviors are equivalent for our targets (visible button/link text), and `textContent` is the project convention. The plan below uses `textContent`. This is a wording fix, not a design change.

---

### Task 1: Add loading bar and SR announce region to base.gohtml

**Files:**
- Modify: `ui/templates/base.gohtml:43-45`

- [ ] **Step 1: Edit `base.gohtml` to insert the bar + announce div as the first children of `<body>`.**

The current `<body>` block is:

```gohtml
    <body>
    {{ template "page" . }}
    </body>
```

Change it to:

```gohtml
    <body>
    <div id="loading-bar" aria-hidden="true"></div>
    <div id="loading-announce" role="status" aria-live="polite" class="sr-only"></div>
    {{ template "page" . }}
    </body>
```

The `.sr-only` class is already defined in `main.css` (visually hidden, retained for screen readers).

- [ ] **Step 2: Smoke check the dev server.**

Run: `make run` (or whatever starts the dev server locally; see `Makefile`).
Open any page. The page should look identical — bar is opacity 0 by default, announce div is `sr-only` (visually hidden). No JS error in DevTools console.

- [ ] **Step 3: Commit.**

```bash
git add ui/templates/base.gohtml
git commit -m "$(cat <<'EOF'
Navigation feedback: add loading bar and SR announce region to base layout

#loading-bar is the visual indicator (aria-hidden, hidden by default).
#loading-announce is a polite live region for the 300ms slow-path SR
announcement. Both live as first children of <body>. Styles and JS
toggling land in follow-up commits.
EOF
)"
```

---

### Task 2: Add loading bar CSS

**Files:**
- Modify: `ui/static/main.css` (append rules near the existing `@view-transition` and keyframe block around line 330–367)

- [ ] **Step 1: Append the bar styles to `main.css`.**

Add the following rules at the end of the file (after the existing `@keyframes button-spinner` block on line 369–373):

```css
#loading-bar {
    position: fixed;
    inset-inline: 0;
    top: 0;
    height: 2px;
    z-index: 1000;
    opacity: 0;
    pointer-events: none;
    transition: opacity 120ms ease-out;
    background: linear-gradient(90deg, transparent, var(--sky-5), transparent);
    background-size: 40% 100%;
    background-repeat: no-repeat;
    background-position: -40% 0;
}

#loading-bar.active {
    opacity: 1;
    animation: loading-bar-slide 1.1s linear infinite;
}

@keyframes loading-bar-slide {
    to { background-position: 140% 0; }
}

@media (prefers-reduced-motion: reduce) {
    #loading-bar {
        transition: none;
    }
    #loading-bar.active {
        animation: none;
        background: var(--sky-5);
    }
}

@media (forced-colors: active) {
    #loading-bar.active {
        background: Highlight;
        forced-color-adjust: none;
    }
}
```

Notes for the engineer:
- `inset-inline: 0` is the logical equivalent of `left: 0; right: 0;` and survives a future RTL switch.
- The bar lives in `<body>` and inherits `view-transition-name: page` from the existing body rule, so it animates out with the page on navigation — no separate transition handling needed.
- `var(--sky-5)` is already defined (`main.css:155`).
- `--sky-5` on a typical page background satisfies WCAG 1.4.11 (3:1 non-text contrast). If a particular page background fails it, swap to `var(--sky-7)` later — not blocking now.

- [ ] **Step 2: Manually verify the bar appears when toggled via DevTools.**

Run the dev server, open any page in a browser. In DevTools console:

```js
document.getElementById('loading-bar').classList.add('active')
```

A thin sky-blue gradient pulse should slide across the top of the viewport. Then:

```js
document.getElementById('loading-bar').classList.remove('active')
```

The bar should fade out over 120 ms.

Also verify reduced motion: in DevTools → Rendering → Emulate CSS media feature `prefers-reduced-motion: reduce`. Re-add the `.active` class. The bar should appear as a solid `--sky-5` strip with no slide animation.

- [ ] **Step 3: Commit.**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Navigation feedback: style #loading-bar with reduced-motion/forced-colors fallbacks

2px fixed-top sky-5 gradient pulse, animates on .active. Honors
prefers-reduced-motion (static block, no animation) and forced-colors
(Highlight system color).
EOF
)"
```

---

### Task 3: Remove the dead `form.submitting` spinner CSS

**Files:**
- Modify: `ui/static/main.css:242-262, 369-373`

The `form.submitting &[type=submit]` block and its `@keyframes button-spinner` are unreferenced — no JS adds the `.submitting` class. This new design supersedes them; clean them out so future readers don't think they're load-bearing.

- [ ] **Step 1: Remove the spinner block inside the `button, .btn` rule.**

Delete lines 242–262 (the block beginning with the comment `/* Add a spinner to a button when submitting */` and ending with the closing brace of the `form.submitting &[type=submit]` selector).

The surrounding `button, .btn` selector and its sibling `&:focus-visible` rule remain.

- [ ] **Step 2: Remove the `button-spinner` keyframe.**

Delete lines 369–373 (the entire `@keyframes button-spinner { ... }` block).

- [ ] **Step 3: Verify no other CSS references `button-spinner` or `form.submitting`.**

Run: `grep -n "button-spinner\|form\.submitting\|submitting" ui/static/main.css`
Expected: no matches.

Also verify nothing in JS adds the class:

Run: `grep -rn "submitting" ui/static/ ui/templates/`
Expected: no matches.

- [ ] **Step 4: Smoke check.**

Reload the dev server. Open a form page (e.g., the home page). Layout should be unchanged from before — buttons render normally.

- [ ] **Step 5: Commit.**

```bash
git add ui/static/main.css
git commit -m "$(cat <<'EOF'
Navigation feedback: drop unused form.submitting spinner CSS

Nothing toggles the .submitting class; the new #loading-bar replaces
this affordance. Removes the button-spinner keyframe and the
form.submitting &[type=submit] block.
EOF
)"
```

---

### Task 4: Add JS module-level state and lifecycle helpers

**Files:**
- Modify: `ui/static/main.js:71-73`

- [ ] **Step 1: Insert state and helpers after the `sameUrl` constant.**

Find this section near the top of `main.js`:

```js
const sameUrl = (a, b) =>
    a.origin === b.origin && a.pathname === b.pathname && a.search === b.search

if ('navigation' in window) {
```

Insert between them so the result reads:

```js
const sameUrl = (a, b) =>
    a.origin === b.origin && a.pathname === b.pathname && a.search === b.search

/**
 * Navigation feedback state.
 *
 * `activeLoad` records the in-flight visual feedback so we can restore the
 * source element's text and hide the bar when:
 *   - bfcache restores a page that was mid-navigation (pageshow.persisted)
 *   - the user cancels a navigation (navigateerror)
 *   - a new navigation supersedes the current one
 *
 * For successful navigations the old document is destroyed on commit and
 * no cleanup is needed in that document — clearLoad on bfcache restore
 * handles the snapshot case.
 */
let activeLoad = null

function startLoad(el) {
    clearLoad()

    const target = (el instanceof HTMLElement && el.textContent?.trim()) ? el : null
    let originalText = null
    if (target) {
        originalText = target.textContent
        target.textContent = 'Loading…'
    }

    const barTimer = setTimeout(() => {
        document.getElementById('loading-bar').classList.add('active')
        document.getElementById('loading-announce').textContent = 'Loading…'
    }, 300)

    activeLoad = { target, originalText, barTimer }
}

function clearLoad() {
    if (!activeLoad) return
    clearTimeout(activeLoad.barTimer)
    if (activeLoad.target && activeLoad.target.isConnected) {
        activeLoad.target.textContent = activeLoad.originalText
    }
    document.getElementById('loading-bar').classList.remove('active')
    document.getElementById('loading-announce').textContent = ''
    activeLoad = null
}

if ('navigation' in window) {
```

Notes:
- The helpers are defined at module scope (outside the `if ('navigation' in window)` gate) because `clearLoad` is also called from the `pageshow` listener which is unconditional.
- `textContent` instead of `innerText` matches the codebase convention (CSP-safe and inert per CLAUDE.md template guidance).
- `el.textContent?.trim()` filters out non-elements and elements with whitespace-only text.
- The `isConnected` check guards against the case where the element has been removed from the DOM between startLoad and clearLoad (defensive; not currently a code path we hit, but cheap).
- `getElementById` calls assume `#loading-bar` and `#loading-announce` exist in `base.gohtml` (Task 1).

- [ ] **Step 2: Sanity check via DevTools console.**

Reload the dev server. Open any page. In DevTools console:

```js
startLoad(document.querySelector('button, a[href]'))
```

Within 300 ms, the picked button/link should swap to `Loading…`. After 300 ms the bar fades in and the announce region should contain `Loading…` (inspect with `document.getElementById('loading-announce').textContent`).

Then:

```js
clearLoad()
```

The text reverts, bar fades out, announce region empties.

- [ ] **Step 3: Commit.**

```bash
git add ui/static/main.js
git commit -m "$(cat <<'EOF'
Navigation feedback: add activeLoad state + startLoad/clearLoad helpers

Module-level state tracking for the in-flight feedback (source element,
original text, bar timer). startLoad swaps text instantly and schedules
the bar for 300ms. clearLoad reverses both and resets the live region.
Not wired into navigation events yet.
EOF
)"
```

---

### Task 5: Restructure the `navigate` listener for both POST and GET, add `navigateerror`

**Files:**
- Modify: `ui/static/main.js:74-89` (inside the `if ('navigation' in window)` block)

- [ ] **Step 1: Replace the existing `navigate` listener.**

The current listener is:

```js
if ('navigation' in window) {
    navigation.addEventListener('navigate', async (e) => {
        if (!e.formData) return
        if (!e.canIntercept || e.hashChange || e.downloadRequest) return
        if (new URL(e.destination.url).origin !== location.origin) return
        for (const [, v] of e.formData) {
            if (v instanceof File) return
        }
        // TODO: when precommitHandler works in iOS, it might be an even better
        //       way to handle this since we can pass e.signal and also reject inside the handler
        //       to have centralised error handling.
        //       https://bugs.webkit.org/show_bug.cgi?id=293952
        e.preventDefault()
        await submitForm(e)
    })
}
```

Replace with:

```js
if ('navigation' in window) {
    navigation.addEventListener('navigate', async (e) => {
        if (e.hashChange || e.downloadRequest) return
        if (new URL(e.destination.url).origin !== location.origin) return

        if (e.formData) {
            if (!e.canIntercept) return
            for (const [, v] of e.formData) {
                if (v instanceof File) return
            }
            // TODO: when precommitHandler works in iOS, it might be an even better
            //       way to handle this since we can pass e.signal and also reject inside the handler
            //       to have centralised error handling.
            //       https://bugs.webkit.org/show_bug.cgi?id=293952
            startLoad(e.sourceElement)
            e.preventDefault()
            await submitForm(e)
            return
        }

        // GET navigations (link clicks, back/forward). We do not intercept;
        // the browser handles the fetch. e.userInitiated filters out the
        // programmatic navigation.navigate() calls that popOrPushTo makes
        // after a successful form submit — without this guard those would
        // overwrite the in-flight form-submit feedback state.
        if (e.userInitiated) startLoad(e.sourceElement)
    })

    navigation.addEventListener('navigateerror', clearLoad)
}
```

Key changes:
- The `if (!e.formData) return` early-out is gone; the listener now handles both branches.
- The shared guards (hashChange, downloadRequest, cross-origin) run once at the top.
- The form-POST branch is unchanged except for the added `startLoad(e.sourceElement)` immediately before `preventDefault()`.
- The GET branch adds `startLoad(e.sourceElement)` for user-initiated navigations only.
- `navigateerror` triggers `clearLoad` to recover from cancelled navigations (browser Stop, supersession).

- [ ] **Step 2: Verify in browser.**

Reload the dev server. Open a page with a form (e.g., the home page). DevTools → Network → throttle to "Slow 3G".

Submit the form. The submit button's text should change to `Loading…` immediately. The loading bar should fade in after ~300 ms. After the new page loads, the old document is gone; the new page renders normally.

Click any link on a page. Same behavior: link text becomes `Loading…`, bar appears after 300 ms.

Click browser Back. Same behavior; if it's a bfcached page, the prior page state is restored — see Task 6 for the cleanup hook.

- [ ] **Step 3: Verify form file uploads still bypass intercept.**

If any form has `type="file"` in the codebase, submitting it should fall through to the native browser submit (no intercept, no JS feedback). Run:

```bash
grep -rn 'type="file"' ui/templates/
```

If no matches, this code path is unreachable today, but the guard remains for future-proofing.

- [ ] **Step 4: Commit.**

```bash
git add ui/static/main.js
git commit -m "$(cat <<'EOF'
Navigation feedback: wire startLoad/clearLoad into navigate + navigateerror

Restructure the navigate listener to handle both intercepted form POSTs
and pass-through GETs. Form path keeps existing intercept + submitForm
flow with startLoad called before preventDefault. GET path calls
startLoad only when e.userInitiated, which filters out the programmatic
navigation.navigate calls from popOrPushTo. navigateerror clears the
feedback when a navigation is cancelled.
EOF
)"
```

---

### Task 6: Augment the `pageshow` listener to call `clearLoad` on bfcache restore

**Files:**
- Modify: `ui/static/main.js:214-231`

- [ ] **Step 1: Add the `clearLoad()` call inside the `event.persisted` branch.**

The current listener is:

```js
window.addEventListener('pageshow', (event) => {
    if (event.persisted) {
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

Change it to:

```js
window.addEventListener('pageshow', (event) => {
    if (event.persisted) {
        // Clear any in-flight navigation feedback that was captured in the
        // bfcache snapshot — the navigation that triggered it has long
        // since resolved (we are reading this snapshot from the cache).
        clearLoad()

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

`clearLoad` runs before the invalidation check. If the invalidation reload fires, the DOM is torn down anyway so the order doesn't matter; if it doesn't fire, `clearLoad` has already restored the source element text and hidden the bar.

- [ ] **Step 2: Verify the bfcache path.**

Reload the dev server. DevTools → Network → throttle to "Slow 3G".

1. On the home page, click a link.
2. Before the new page commits, observe the link reads `Loading…` and the bar is visible (after 300 ms).
3. After the new page renders, click the browser Back button.
4. The previous page should restore from bfcache with the link text back to its original value, and no loading bar visible.

If you cannot reliably reproduce a bfcache restore on your machine, an alternative way to verify is from DevTools: Application → Back/forward cache → Test back/forward cache. The page should be eligible and produce a `persisted: true` event.

- [ ] **Step 3: Commit.**

```bash
git add ui/static/main.js
git commit -m "$(cat <<'EOF'
Navigation feedback: clear stale state on bfcache restore

When a page is restored from bfcache its snapshot may include the
Loading… text swap and the active bar from when the user navigated
away. clearLoad runs at the top of the persisted branch of pageshow
to restore the snapshot before the user observes it.
EOF
)"
```

---

### Task 7: Full manual verification + lint/test

**Files:** none

This task verifies the spec's scenarios end-to-end. Each scenario must pass before declaring the plan complete.

- [ ] **Step 1: Run the Go test suite to confirm no regressions.**

Run: `make test`
Expected: all tests pass. Our changes don't touch Go, but Go template tests load `base.gohtml` and could theoretically fail if the markup is malformed.

- [ ] **Step 2: Run the linter.**

Run: `make lint-fix`
Expected: no Go lint complaints. (golangci-lint doesn't lint JS/CSS in this project.)

- [ ] **Step 3: Slow-path verification (throttled network).**

Setup: dev server running. DevTools → Network → Slow 3G throttling.

- [ ] Submit a form: the submit button reads `Loading…` immediately and the bar fades in after ~300 ms.
- [ ] Click a link: the link text reads `Loading…` immediately and the bar fades in after ~300 ms.
- [ ] Browser back: the bar still appears for slow back-traverse to non-bfcached pages.

- [ ] **Step 4: Fast-path verification (no throttling).**

- [ ] Submit a form locally: bar should not appear (fetch resolves in well under 300 ms). The submit button briefly shows `Loading…` for one or two frames, then the new page commits.
- [ ] Click a prefetched link: should not show the bar.

- [ ] **Step 5: Validation-error path (same-URL replace).**

- [ ] Trigger a form submit that produces a validation error and replaces the same URL (e.g., a malformed input on any form). The Loading… text and bar should appear and clear cleanly on the replaced page.

- [ ] **Step 6: bfcache path.**

- [ ] Submit a form / click a link → arrive at new page → click browser Back. The prior page must restore with the original button/link text and no bar.

- [ ] **Step 7: Cancel path.**

- [ ] Throttle to Slow 3G, click a link, then immediately click the browser Stop button (or click another link). The first link's text must restore to its original value and the bar must hide.

- [ ] **Step 8: Reduced motion.**

- [ ] DevTools → Rendering → Emulate `prefers-reduced-motion: reduce`. Slow-path repeat: bar should appear as a static `--sky-5` strip with no slide animation, opacity transition disabled.

- [ ] **Step 9: Forced colors.**

- [ ] DevTools → Rendering → Emulate `forced-colors: active`. Slow-path repeat: bar should appear using the `Highlight` system color.

- [ ] **Step 10: Screen reader.**

- [ ] With VoiceOver (Cmd+F5 on macOS) or NVDA active, submit a slow form. Confirm `Loading…` is announced once via the live region around the 300 ms mark.
- [ ] Repeat on a fast path: confirm no announcement.

- [ ] **Step 11: Programmatic-navigation guard (the userInitiated check).**

- [ ] Submit a form. The post-submit `navigation.navigate()` call inside `popOrPushTo` fires a second `navigate` event. Verify the button text from the form submit is **not** clobbered by the GET branch — `e.userInitiated` is false for that programmatic navigation, so `startLoad` is not called a second time. (If it were, the form's submit button reference inside `activeLoad` would be overwritten and the bfcache cleanup wouldn't find it.)
- [ ] A clear way to confirm: throttle Slow 3G, submit, observe the button text stays `Loading…` throughout the fetch + navigate flow.

- [ ] **Step 12: If anything above fails, do not mark the plan complete. Diagnose, fix, and re-verify.**

---

## Out of scope (do not implement)

- Loading-bar behavior for cross-origin or download navigations (skipped by existing guards).
- An `aria-busy` attribute on the source element (see the design spec for the rationale).
- Disabling the source element during load (would steal focus).
- Queueing or cancellation of concurrent submits beyond the `clearLoad()` at the top of `startLoad`.
- Internationalization of the `Loading…` string (app is `lang="en"` only today).
- Automated tests for this layer (no harness in the project; spec mandates manual verification).
