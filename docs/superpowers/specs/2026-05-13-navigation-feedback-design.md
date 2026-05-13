# Navigation feedback (slow-transition UX)

Status: draft
Owner: Martin
Created: 2026-05-13

## Goal

Surface that a navigation is in flight when it gets slow, without flickering
during the fast paths the app is tuned for. Two complementary affordances:

1. **Local click confirmation** — the element the user activated (submit
   button or anchor) immediately swaps its text to `Loading…`.
2. **Global slowness indicator** — a thin progress bar fixed at the top of
   the viewport appears only if the navigation has not committed within
   300 ms.

Applies to both intercepted form POSTs (`submitForm` path) and pass-through
GET navigations (link clicks, browser back/forward).

## Behavior

| Time | Form POST | Link-click GET | Back/forward GET |
|---|---|---|---|
| t=0 (click) | `e.sourceElement.innerText = 'Loading…'`; start 300 ms bar timer | Same | No sourceElement → bar timer only |
| t≥300 ms (still pending) | Loading bar fades in | Same | Same |
| nav commits | New document renders; bar/text live only in bfcache snapshot of old document | Same | Same |
| `navigateerror` | `clearLoad()` restores text, cancels timer, hides bar | Same | Same |
| `pageshow` with `event.persisted` | `clearLoad()` (bfcache restoration of an old loading state) | Same | Same |

The 300 ms threshold matches industry guidance (Material/RAIL) for when a
transition stops feeling "instant" and starts needing acknowledgement. The
button-text swap stays instant because it is *click confirmation*, not a
slowness signal — its job is to answer "did my tap register?", and that
answer must be immediate. Staggering the two affordances also prevents the
"everything appeared at once" jump that lands when global chrome animates in
simultaneously with local state changes.

## Files touched

- `ui/templates/base.gohtml` — add the bar element and an SR announcement
  region inside `<body>`.
- `ui/static/main.css` — bar styles + keyframe; reduced-motion and
  forced-colors fallbacks; removal of the dead `form.submitting` button-
  spinner block (lines ~242–262 and the `@keyframes button-spinner` rule)
  superseded by this design.
- `ui/static/main.js` — `activeLoad` module state, `startLoad`/`clearLoad`
  helpers, restructured `navigate` listener to cover both POST and GET
  paths, `navigateerror` listener, augmented `pageshow` listener.

## Components

### Markup

In `base.gohtml`, inside `<body>` as the first children (siblings of the
existing content):

```html
<div id="loading-bar" aria-hidden="true"></div>
<div id="loading-announce" role="status" aria-live="polite" class="sr-only"></div>
```

- `#loading-bar` — visual indicator only, hidden from assistive tech.
- `#loading-announce` — visually hidden, used for SR notification of slow
  navigations. Empty by default; populated with `Loading…` when the 300 ms
  timer fires (so fast paths produce no announcement).

The `.sr-only` class already exists in `main.css`.

### State

Module-level in `main.js`:

```js
let activeLoad = null  // { target, originalText, barTimer }
```

`activeLoad.target` is the element whose text was swapped, or `null` for
back/forward (no source element). `originalText` is preserved so
`clearLoad` can restore on bfcache return or nav error. `barTimer` is the
pending `setTimeout` id.

### Lifecycle helpers

```js
function startLoad(el) {
    clearLoad()  // supersede any prior in-flight feedback

    const target = (el instanceof HTMLElement && el.innerText?.trim()) ? el : null
    let originalText = null
    if (target) {
        originalText = target.innerText
        target.innerText = 'Loading…'
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
        activeLoad.target.innerText = activeLoad.originalText
    }
    document.getElementById('loading-bar').classList.remove('active')
    document.getElementById('loading-announce').textContent = ''
    activeLoad = null
}
```

### Navigation hooks

Restructured `navigate` listener:

```js
navigation.addEventListener('navigate', async (e) => {
    if (e.hashChange || e.downloadRequest) return
    if (new URL(e.destination.url).origin !== location.origin) return

    if (e.formData) {
        if (!e.canIntercept) return
        for (const [, v] of e.formData) {
            if (v instanceof File) return
        }
        startLoad(e.sourceElement)
        e.preventDefault()
        await submitForm(e)
        return
    }

    if (e.userInitiated) startLoad(e.sourceElement)
})

navigation.addEventListener('navigateerror', clearLoad)
```

The `e.userInitiated` guard on the GET branch skips programmatic
`navigation.navigate()` calls from `popOrPushTo` (which fires its own
navigate event after a successful form submit) — without it we would clobber
the in-progress form-submit feedback state.

Augmented `pageshow` listener (existing handler near line 214):

```js
window.addEventListener('pageshow', (event) => {
    if (event.persisted) {
        clearLoad()  // strip stale loading state from bfcache snapshot
        // ... existing invalidation-token reload check unchanged
    }
})
```

`clearLoad()` is called before the existing invalidation check; if that
check decides to `navigation.reload()`, the reload blows away the DOM
anyway, but calling `clearLoad` first keeps the snapshot tidy for the
non-reload case.

### Styling

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
    #loading-bar.active {
        animation: none;
        background: var(--sky-5);
    }
    #loading-bar { transition: none; }
}

@media (forced-colors: active) {
    #loading-bar.active {
        background: Highlight;
        forced-color-adjust: none;
    }
}
```

`inset-inline: 0` (logical property) covers RTL automatically should the app
ever localize. Bar lives in `<body>` and inherits `view-transition-name:
page`, so it animates out with the old page during the standard page
transition (forward slide / backward grow).

## Accessibility

This section is the contract for assistive-tech behavior.

**Two audiences, two channels.** The instant text-swap on the source
element is a *sighted-user* affordance: changing `innerText` of an already-
focused element is not a reliable SR re-announcement trigger across NVDA,
JAWS, and VoiceOver, so we do not pretend it serves both audiences. SR
feedback comes exclusively through the slow-path live region described
below, deliberately silent on the fast paths to match what the bar does
visually.

**Slow-path announcement (`role="status"` live region).** The hidden
`#loading-announce` div is `aria-live="polite"` with `role="status"`. It is
populated with `Loading…` only when the 300 ms timer fires, mirroring the
visual bar exactly. Fast navigations (< 300 ms) produce no announcement,
avoiding the noise that an always-on live region would generate. Polite
(not assertive) so it does not interrupt other speech; this is a
non-critical status update.

This earns its place specifically because of the MPA lifecycle: during the
fetch window before commit, browser/SR loading announcements are
inconsistent across combinations (some announce "loading", some are
silent, prefetched pages skip the window entirely). The reliable
cross-platform announcement is the new page's title on commit. Without
this live region, an SR user on a slow navigation has no proactive signal
that their activation took effect — they sit in dead air until the new
page loads. The 300 ms gate keeps that signal from firing for the fast
common case.

**No `aria-busy` on the source element.** `aria-busy` exists for regions
whose contents are updating in place (SPA partial swaps), not for elements
that are about to be destroyed by an MPA commit. Setting it for the
duration of the fetch buys at most a mid-announcement state suffix on
inconsistent SR/browser combinations, on an element that disappears within
milliseconds. Not worth the attribute.

**Hidden visual indicator.** `#loading-bar` carries `aria-hidden="true"` so
assistive tech ignores it entirely; all SR information comes through
`#loading-announce`. The bar is decoration.

**Reduced motion.** Under `prefers-reduced-motion: reduce` the bar's
sliding gradient is replaced with a static `var(--sky-5)` block, and the
opacity transition is removed. The bar still appears and disappears (it is
not motion — it is presence/absence of UI), but nothing animates. WCAG
2.3.3 satisfied.

**Forced colors / Windows high contrast.** Under `forced-colors: active`
the bar uses the `Highlight` system color with `forced-color-adjust: none`
so the indicator remains visible against any forced color scheme. WCAG
1.4.1 (do not rely on color alone) is unaffected — the bar is supplementary
to the text + live-region channels.

**Color contrast.** The bar is a UI component; WCAG 1.4.11 requires 3:1
non-text contrast against adjacent colors. `var(--sky-5)` on the default
page background meets this; we will verify in the implementation pass and
swap to a darker sky if it fails on any page background. The bar's slim
height (2 px) is acceptable: 1.4.11 applies to perceivable graphical
information, and the active animation plus the text/live-region channels
provide the same information through other means.

**Focus retention.** Swapping `innerText` on a focused button keeps focus
on the same element node — no focus loss. We deliberately do **not** set
the `disabled` attribute, which would move focus away and break the
"button now reads Loading…" confirmation for sighted users.

**Keyboard equivalence.** All entry points (mouse click, Enter, Space)
flow through the same `navigate` event, so keyboard users see identical
feedback to mouse/touch users.

**Touch targets.** The bar uses `pointer-events: none` and sits in a 2 px
strip at the top of the viewport, so it cannot intercept taps on whatever
the user is interacting with.

**Internationalization.** The string `Loading…` is hardcoded. The app is
currently `lang="en"` (`base.gohtml` line 4); if the app is later
localized, route the string through a template-rendered JS constant or a
data attribute on `#loading-announce`. Not solved in this change.

## Failure modes

- **Network failure / unexpected status.** Existing code calls
  `location.reload()`. A hard reload tears down the DOM, so the bar and
  text swap are wiped cleanly. No manual cleanup needed.
- **User-cancelled navigation (Stop button, supersession).** The
  Navigation API fires `navigateerror` in the surviving document. Our
  listener calls `clearLoad`, restoring text and hiding the bar.
- **Rapid double-submit.** `startLoad` calls `clearLoad` first, so the
  prior `activeLoad` state is restored before the new one is recorded.
  Without this, the first button's `Loading…` text could become permanent
  if the user submitted a second form before the first one resolved.
- **Source element missing or non-element.** `startLoad` checks for
  `HTMLElement` with non-empty `innerText`. If absent (e.g.,
  browser-initiated traverse with no source), the text swap is skipped
  but the bar still shows after 300 ms.
- **Loading bar element absent.** Should not happen — the bar lives in
  `base.gohtml` and renders on every page. If a page bypasses the base
  template (none currently do), `getElementById('loading-bar')` returns
  null and the `.classList` access throws. Acceptable; the spec expects
  base.gohtml everywhere.

## Non-goals (YAGNI)

- **Loading bar for cross-origin / download navigations.** Skipped via
  existing guards (different origin, `downloadRequest`). Cross-origin
  takes the user away from our app; download keeps them on the page.
- **Persisting the bar across view transitions.** No own
  `view-transition-name`; the bar animates out with the page group. The
  fetch resolves before navigation, so there is no in-flight state that
  needs to outlive the transition.
- **Replacing `<progress>` semantics.** A plain div with an `aria-live`
  sibling is more controllable and quieter for SR users than indeterminate
  `<progress>`, which announces on every appearance.
- **Disabling the source element.** Would steal focus and break the
  `Loading…` confirmation.
- **Queueing / cancellation of concurrent submits.** `clearLoad` at the
  start of `startLoad` is sufficient; we trust the Navigation API to
  serialize.
- **i18n of the `Loading…` string.** App is single-language today.
  Flagged in the Accessibility section.

## Verification plan

No automated UI tests for this path (Go template tests do not exercise
client JS). Manual verification at implementation time:

- Slow path: throttle the network to 3G in DevTools, submit a form,
  confirm button reads `Loading…` at t=0 and bar fades in at ~300 ms.
- Fast path: submit on local-loopback, confirm bar never appears, text
  swap is brief but visible (one or two frames is fine).
- Validation-error path: trigger a same-URL replace submit, confirm
  feedback clears cleanly on the replaced page.
- bfcache path: submit, navigate to another page, hit Back; the prior
  form page must restore with original button text and no bar.
- Cancel path: start a slow nav, press the browser Stop button (or trigger
  another nav), confirm `clearLoad` restores text.
- Reduced motion: enable `prefers-reduced-motion`; confirm bar appears as
  a static block, no animation.
- Forced colors: enable Windows High Contrast / forced-colors emulation in
  DevTools; confirm bar uses `Highlight` color.
- Screen reader: with VoiceOver/NVDA active, submit a slow form; confirm
  `Loading…` is announced once via the live region. Then submit a fast
  form and confirm there is no spurious announcement.

## Open questions

None.
