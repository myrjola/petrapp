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
        location.reload()
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
    // body in place. Reload to surface the server state on any unexpected status.
    location.reload()
}

async function popOrPushTo(target, {replace = false} = {}) {
    const targetUrl = new URL(target, location.origin)

    // Replace mode: server-flagged via X-Replace-Url, or same-URL submit
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
            // If the entry was pruned between our read of entries() and the
            // traverse, fall back to a normal navigation rather than no-op.
            navigation.traverseTo(entries[i].key).committed.catch(() => location.assign(link.href))
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
