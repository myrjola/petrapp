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
 *   Response: X-Replace-Url: true         (optional; replace current entry)
 *
 * Without the X-Requested-With header, the server returns a plain 303 See
 * Other and the browser follows. That is the no-JS / no-Navigation-API path.
 *
 * Navigation strategy: pop-or-push (with explicit replace mode)
 * -------------------------------------------------------------
 * Two modes, decided per-response:
 *
 *   1. Replace mode — server set X-Replace-Url: true, OR the target URL
 *      equals the current entry's URL ignoring fragment (same-URL submit).
 *      The current entry is replaced. ALWAYS forces a real cross-document
 *      fetch by appending a bf_inv cache-bust query param (carrying the
 *      rotated inv_bfcache cookie) and calling location.replace. Without
 *      the cache-bust, the Navigation API would resolve identical-URL or
 *      fragment-change navigations as same-document operations that skip
 *      the GET, leaving the freshly-set server state unread. An inline
 *      cleanup script in base.gohtml strips bf_inv before first paint so
 *      the URL bar lands on the canonical fragment-bearing target.
 *      We do not walk the history stack: replace is about erasing the
 *      current entry, not jumping to an existing one.
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

/**
 * Convenience function to get the parent element of the current script tag.
 * Inspired by https://github.com/gnat/surreal.
 * @returns {HTMLElement}
 */
function me() {
    return document.currentScript.parentElement
}

const sameUrl = (a, b) =>
    a.origin === b.origin && a.pathname === b.pathname && a.search === b.search

/**
 * Navigation feedback state.
 *
 * `activeLoad` records the in-flight visual feedback so we can clear the
 * source element's busy state and hide the bar when:
 *   - bfcache restores a page that was mid-navigation (pageshow.persisted)
 *   - the user cancels a navigation (navigateerror)
 *   - a new navigation supersedes the current one
 *
 * For successful navigations the old document is destroyed on commit and
 * no cleanup is needed in that document — clearLoad on bfcache restore
 * handles the snapshot case. The bar and announce region activate
 * synchronously; an earlier 300ms-gated reveal was scrapped because the
 * old document tends to be torn down before the timer fires on this
 * codebase (Speculation Rules prefetch on every link).
 */
let activeLoad = null

function startLoad(el) {
    clearLoad()

    // Only buttons show the inline spinner — anchor labels are part of page
    // content and overlaying a spinner right before the browser tears down
    // the document for a GET navigation is noisy without being reliably
    // visible. We wrap the button's children in a .btn-loading-label span
    // and hide it with visibility:hidden; the span still participates in
    // layout, so the button keeps its exact width and height while the
    // ::after spinner replaces the visible label. Stashing the original
    // child nodes lets clearLoad — including the bfcache pageshow.persisted
    // path — restore the button to its pre-click DOM.
    const target = (el instanceof HTMLButtonElement && el.textContent?.trim()) ? el : null
    let originalChildren = null
    if (target) {
        originalChildren = Array.from(target.childNodes)
        const label = document.createElement('span')
        label.className = 'btn-loading-label'
        label.append(...originalChildren)
        target.appendChild(label)
        target.setAttribute('aria-busy', 'true')
    }

    document.getElementById('loading-bar').classList.add('active')
    document.getElementById('loading-announce').textContent = 'Loading…'

    activeLoad = { target, originalChildren }
}

function clearLoad() {
    if (!activeLoad) return
    if (activeLoad.target && activeLoad.target.isConnected) {
        activeLoad.target.removeAttribute('aria-busy')
        if (activeLoad.originalChildren) {
            activeLoad.target.replaceChildren(...activeLoad.originalChildren)
        }
    }
    document.getElementById('loading-bar').classList.remove('active')
    document.getElementById('loading-announce').textContent = ''
    activeLoad = null
}

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

    navigation.addEventListener('navigateerror', (e) => {
        // Our own form-submit handling calls preventDefault() on the navigate
        // event, which aborts that navigation and fires navigateerror with an
        // AbortError. That is expected — submitForm is about to drive the real
        // navigation, so the feedback must stay up. A superseding navigation
        // likewise aborts the one it replaces, but its startLoad() has already
        // reset the state. Only a genuine failure, where we stay on the current
        // document, should tear the feedback down.
        if (e.error?.name === 'AbortError' || /abort/i.test(e.message || '')) return
        clearLoad()
    })
}

async function submitForm(e) {
    // Clear any leftover client-only flash at the start of every submit;
    // a fresh action supersedes the prior failure state.
    const flash = document.getElementById('js-flash')
    if (flash) {
        flash.hidden = true
        flash.textContent = ''
    }

    const body = new URLSearchParams(e.formData)
    let res
    try {
        res = await fetch(e.destination.url, {
            method: 'POST',
            body,
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded',
                'X-Requested-With': 'stacknav',
            },
            // Surface server-side redirects (e.g., a future auth bounce that
            // doesn't go through redirect()) as opaqueredirect responses
            // rather than transparently following them — fall through to the
            // unexpected-status branch below and reload to surface state.
            redirect: 'manual',
        })
    } catch (_) {
        // Client-side failure (offline, DNS, CORS). The server never saw the
        // request, so there is no flash to read on reload — and reload itself
        // may fail when offline. Surface inline via the pre-existing
        // role="alert" skeleton; textContent is CSP / Trusted-Types safe.
        if (flash) {
            flash.textContent = 'Connection lost. Check your network and try again.'
            flash.hidden = false
        }
        clearLoad()
        return
    }

    if (res.status === 200) {
        const target = res.headers.get('X-Location')
        if (!target) {
            location.reload()
            return
        }
        const replace = res.headers.get('X-Replace-Url') === 'true'
        await popOrPushTo(target, {replace})
        return
    }

    // CSP blocks document.write/innerHTML, so we can't render the response
    // body in place. Reload to surface the server state on any unexpected
    // status — the server's flash (set via app.userError) carries the message.
    location.reload()
}

async function popOrPushTo(target, {replace = false} = {}) {
    const targetUrl = new URL(target, location.origin)
    const currentUrl = new URL(navigation.currentEntry.url)

    // Replace mode: server-flagged via X-Replace-Url, or same-URL submit
    // (auto-detected so backend doesn't have to think about it). We
    // deliberately do not walk back looking for a traverse target —
    // replace is about erasing the current entry, not jumping elsewhere.
    //
    // The replace branch ALWAYS forces a cross-document fetch via a
    // bf_inv query param carrying the rotated inv_bfcache cookie. The
    // Navigation API resolves some same-pathname navigations (identical
    // URL, or same path + a fragment change) as same-document operations
    // that skip the GET — the freshly-rotated server state never reaches
    // the user, and the freshly-set session flash either stays unread or
    // leaks onto the next page. Differentiating the search component
    // forces a real cross-doc fetch; an inline cleanup script in
    // base.gohtml strips bf_inv before first paint so the URL bar
    // carries the canonical target form (fragment included, for native
    // scroll-to-anchor on the new document).
    // See docs/superpowers/specs/2026-05-26-stack-navigator-replace-force-fresh-design.md
    if (replace || sameUrl(currentUrl, targetUrl)) {
        const cookieValue = document.cookie
            .match(/(?:^|;\s*)inv_bfcache=([^;]+)/)?.[1] ?? ''
        const bust = new URL(target, location.origin)
        bust.searchParams.set('bf_inv', cookieValue || String(Date.now()))
        location.replace(bust.toString())
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

window.addEventListener('pagereveal', (e) => {
    if (!e.viewTransition) return
    if (!('navigation' in window)) return
    const act = navigation.activation
    if (!act) return
    if (act.navigationType === 'reload') {
        // Reload-triggered cross-doc VT: never animate (would replay the page
        // slide for "same page, just fresher data" — disorienting). Replace
        // navigations are allowed through so pages that opt into a replace-
        // driven bfcache flow (see workout.gohtml) can animate state changes.
        e.viewTransition.skipTransition()
        return
    }
    // Replace navigations stay on the same logical position (no new history
    // entry), so they don't get a slide direction. Pages that want a custom
    // transition on a replace opt in via their own pagereveal handler (e.g.,
    // workout.gohtml's 'state-refresh' type).
    let dir
    if (act.navigationType === 'traverse' && act.from && act.entry) {
        dir = act.entry.index < act.from.index ? 'backward' : 'forward'
    } else if (act.navigationType === 'push') {
        dir = 'forward'
    }
    if (dir) {
        e.viewTransition.types.add(dir)
    }
})

document.addEventListener('click', (e) => {
    if (!('navigation' in window)) return
    const link = e.target.closest('a[data-back-button]')
    if (!link) return
    const target = new URL(link.href)
    const entries = navigation.entries()
    for (let i = navigation.currentEntry.index - 1; i >= 0; i--) {
        if (sameUrl(new URL(entries[i].url), target)) {
            e.preventDefault()
            // No .committed.catch fallback. The previous fallback —
            // .catch(() => location.assign(link.href)) — was intended to
            // handle the TOCTOU case where the entry was pruned between
            // our read of entries() and the traverse. It also fired on
            // ANY committed rejection, including the cross-bfcache one
            // that produced the navbug: a traverseTo issued from a doc
            // that subsequently goes into bfcache leaves .committed
            // pending until the doc is restored; the first navigation
            // initiated from the restored doc (e.g. our pagereveal
            // staleness reload) supersedes it, and the .catch then fires
            // location.assign with the closure-captured back-target URL,
            // pushing a spurious history entry. Dropping the fallback
            // means the rare entry-pruned case becomes a no-op click
            // (user can click again) rather than history corruption.
            navigation.traverseTo(entries[i].key)
            return
        }
    }
})

/**
 * Service-worker registration helper.
 *
 * The CSP `require-trusted-types-for 'script'` makes the browser reject raw
 * strings passed to script-loading sinks like `navigator.serviceWorker.register`.
 * We funnel registration through one TrustedTypePolicy that whitelists the
 * single URL we actually load, so callers don't have to know about Trusted
 * Types. Add a new allowed URL to this policy rather than creating a second
 * policy with the same name — the CSP has no `'allow-duplicates'`, so a
 * second `createPolicy('sw-loader', ...)` would throw.
 *
 * @returns {Promise<ServiceWorkerRegistration>}
 */
const swUrl = (() => {
    if (!window.trustedTypes || !window.trustedTypes.createPolicy) return '/sw.js'
    const policy = window.trustedTypes.createPolicy('sw-loader', {
        createScriptURL: (input) => {
            if (input === '/sw.js') return input
            throw new TypeError(`sw-loader: disallowed URL ${input}`)
        },
    })
    return policy.createScriptURL('/sw.js')
})()

function registerServiceWorker() {
    return navigator.serviceWorker.register(swUrl)
}

// Clear the bfcache-captured button spinner on restore. The navigation
// that started the spinner has long since resolved; the snapshot just
// has stale aria-busy / .btn-loading-label markup that startLoad would
// otherwise leave dangling.
window.addEventListener('pageshow', (event) => {
    if (event.persisted) clearLoad()
})

// Web Vitals reporter — observes LCP, INP, FCP, TTFB and beacons one
// batched JSON payload on pagehide / visibility:hidden. INP picks the
// worst event duration seen so far rather than the web-vitals 98th-
// percentile interaction — slightly conservative, fine for regression
// monitoring. CLS is omitted; its session-window math doesn't earn its
// bytes at this scale. `target` carries a best-effort element selector
// for LCP / INP so log lines can be triaged.
;(() => {
    if (!('PerformanceObserver' in window) || !navigator.sendBeacon) return

    const thresholds = {
        LCP: [2500, 4000], INP: [200, 500],
        FCP: [1800, 3000], TTFB: [800, 1800],
    }
    const rate = (name, value) => {
        const t = thresholds[name]
        if (!t) return ''
        return value <= t[0] ? 'good' : value <= t[1] ? 'needs-improvement' : 'poor'
    }

    const describeNode = (el) => {
        if (!el || !el.tagName) return ''
        let s = el.tagName.toLowerCase()
        if (el.id) {
            s += '#' + el.id
        } else if (typeof el.className === 'string') {
            const cls = el.className.trim().split(/\s+/).slice(0, 2).join('.')
            if (cls) s += '.' + cls
        }
        return s.slice(0, 80)
    }

    const reports = new Map()
    const record = (name, value, target) => {
        reports.set(name, {name, value, rating: rate(name, value), target: target || ''})
    }

    const observe = (type, options, handler) => {
        try {
            new PerformanceObserver(handler).observe({type, buffered: true, ...options})
        } catch (_) {
            // Browser doesn't support this entry type — skip.
        }
    }

    observe('largest-contentful-paint', {}, (list) => {
        const entries = list.getEntries()
        const last = entries[entries.length - 1]
        record('LCP', last.renderTime || last.loadTime, describeNode(last.element))
    })

    observe('paint', {}, (list) => {
        for (const entry of list.getEntries()) {
            if (entry.name === 'first-contentful-paint') record('FCP', entry.startTime)
        }
    })

    let worstInp = 0
    observe('event', {durationThreshold: 16}, (list) => {
        for (const entry of list.getEntries()) {
            if (entry.interactionId && entry.duration > worstInp) {
                worstInp = entry.duration
                record('INP', worstInp, describeNode(entry.target))
            }
        }
    })

    const nav = performance.getEntriesByType('navigation')[0]
    if (nav) record('TTFB', nav.responseStart)

    const flush = () => {
        if (reports.size === 0) return
        const payload = JSON.stringify({
            path: location.pathname,
            navigationType: nav?.type || '',
            metrics: Array.from(reports.values()),
        })
        navigator.sendBeacon('/api/vitals', new Blob([payload], {type: 'application/json'}))
        reports.clear()
    }

    addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') flush()
    })
    addEventListener('pagehide', flush)
})()

// Rest-chip tick. Drives every [data-rest-end-at-ms] chip on the page —
// the exerciseset active card and per-row chips on the workout overview
// — so the same logic powers both contexts. Each chip carries its own
// "chimed" flag so multiple in-progress slots (power sets) each chime
// once when their rest elapses. A deadline already in the past renders
// directly as "Ready" with no chime; the push notification covered that
// window while the user was away.
//
// main.js is loaded synchronously in <head>, so wait for DOM ready
// before querying for chip elements — they live in <body>.
function initRestChips() {
    const chips = document.querySelectorAll('[data-rest-end-at-ms]')
    if (chips.length === 0) return

    let audioCtx = null
    const unlockAudio = () => {
        const Ctor = window.AudioContext || window.webkitAudioContext
        if (!Ctor) return
        if (!audioCtx) audioCtx = new Ctor()
        if (audioCtx.state === 'suspended') audioCtx.resume()
    }
    document.addEventListener('pointerdown', unlockAudio, {once: true})
    document.addEventListener('keydown', unlockAudio, {once: true})
    unlockAudio()

    const playChime = () => {
        if (!audioCtx || audioCtx.state !== 'running') return
        const start = audioCtx.currentTime
        ;[880, 1320].forEach((freq, i) => {
            const t0 = start + i * 0.18
            const t1 = t0 + 0.16
            const osc = audioCtx.createOscillator()
            const gain = audioCtx.createGain()
            osc.type = 'sine'
            osc.frequency.value = freq
            gain.gain.setValueAtTime(0, t0)
            gain.gain.linearRampToValueAtTime(0.3, t0 + 0.01)
            gain.gain.setValueAtTime(0.3, t1 - 0.01)
            gain.gain.linearRampToValueAtTime(0, t1)
            osc.connect(gain).connect(audioCtx.destination)
            osc.start(t0)
            osc.stop(t1)
        })
    }

    const states = Array.from(chips).map((chip) => ({
        chip,
        timeEl: chip.querySelector('[data-rest-time]'),
        endAt: parseInt(chip.dataset.restEndAtMs, 10),
        chimed: false,
    }))

    // Screen Wake Lock — hold the screen on while a rest is counting down so the
    // in-page countdown stays visible and the chime can fire. The platform
    // auto-releases the lock whenever the page is hidden, so we re-acquire on
    // visibilitychange while a rest is still active. Degrades silently where
    // unsupported (pre-iOS 18.4 installed PWAs, older browsers); the countdown
    // and the server push both still work without it.
    let wakeLock = null
    let wakeLockBusy = false
    const restActive = () => states.some((st) => st.endAt - Date.now() > 0)
    const acquireWakeLock = async () => {
        if (!('wakeLock' in navigator) || wakeLock || wakeLockBusy) return
        if (document.visibilityState !== 'visible') return
        wakeLockBusy = true
        try {
            wakeLock = await navigator.wakeLock.request('screen')
            wakeLock.addEventListener('release', () => { wakeLock = null }, {once: true})
        } catch (_) {
            // Rejected (not visible, blocked by policy, unsupported): leave it.
            wakeLock = null
        } finally {
            wakeLockBusy = false
        }
    }
    const releaseWakeLock = () => {
        if (!wakeLock) return
        const held = wakeLock
        wakeLock = null
        held.release().catch(() => {})
    }
    document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible' && restActive()) acquireWakeLock()
    })

    let intervalId = null
    const tick = () => {
        const now = Date.now()
        let allReady = true
        for (const st of states) {
            const remaining = Math.max(0, st.endAt - now)
            if (remaining === 0) {
                st.chip.classList.add('ready')
                if (st.timeEl) st.timeEl.textContent = 'Ready'
                if (!st.chimed) {
                    st.chimed = true
                    if (document.visibilityState === 'visible' && now - st.endAt <= 1000) {
                        playChime()
                    }
                }
            } else {
                allReady = false
                const totalSec = Math.ceil(remaining / 1000)
                const m = Math.floor(totalSec / 60)
                const s = totalSec % 60
                if (st.timeEl) st.timeEl.textContent = m + ':' + String(s).padStart(2, '0')
            }
        }
        if (allReady) {
            releaseWakeLock()
            if (intervalId !== null) {
                clearInterval(intervalId)
                intervalId = null
            }
        } else {
            acquireWakeLock()
        }
    }
    tick()
    intervalId = setInterval(tick, 250)
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initRestChips, {once: true})
} else {
    initRestChips()
}

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
