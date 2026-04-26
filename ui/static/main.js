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
 *   Request:  X-Requested-With: stacknav (set by the JS shim's fetch)
 *   Response: 200 + X-Location: <url> (where to navigate; body empty)
 *
 * Without the X-Requested-With header, the server returns a plain 303 See
 * Other and the browser follows. That is the no-JS / no-Navigation-API path.
 *
 * Navigation strategy: pop-or-replace
 * -----------------------------------
 * One strategy covers every flow: walk back through history looking for an
 * entry whose URL matches the target; traverse to it if found, otherwise
 * replace the current entry. This collapses correctly across all real cases:
 *
 *   1. Same-URL submit (e.g., set update on DETAIL → DETAIL): no older
 *      DETAIL entry behind us, so we fall through to replace. Back goes to
 *      the parent rather than the form-submit page.
 *   2. Cross-URL submit, target present (e.g. schedule → home): traverse
 *      back to the existing home entry instead of pushing a duplicate.
 *   3. Cross-URL submit, target absent: fall through to replace. The
 *      form page leaves no trace in history.
 *
 * Validation errors use putFlashError + redirect-to-form on the server, so
 * they arrive as a plain 200 + X-Location pointing back at the form. The
 * CSP (require-trusted-types-for 'script') blocks any in-place HTML render,
 * so we keep the wire shape uniform across success and failure.
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
        await popOrReplaceTo(target)
        return
    }

    // CSP blocks document.write/innerHTML, so we can't render the response
    // body in place. Reload to surface the server state on any unexpected status.
    location.reload()
}

async function popOrReplaceTo(target) {
    const targetUrl = new URL(target, location.origin)
    const entries = navigation.entries()
    for (let i = navigation.currentEntry.index - 1; i >= 0; i--) {
        if (sameUrl(new URL(entries[i].url), targetUrl)) {
            await navigation.traverseTo(entries[i].key).committed
            return
        }
    }
    navigation.navigate(target, {history: 'replace'})
}

window.addEventListener('pagereveal', (e) => {
    if (!e.viewTransition) return
    if (!('navigation' in window)) return
    const act = navigation.activation
    if (!act) return
    if (act.navigationType === 'replace' || act.navigationType === 'reload') {
        e.viewTransition.skipTransition()
        return
    }
    let dir = 'forward'
    if (act.navigationType === 'traverse' && act.from && act.entry) {
        dir = act.entry.index < act.from.index ? 'backward' : 'forward'
    }
    e.viewTransition.types.add(dir)
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
            navigation.traverseTo(entries[i].key)
            return
        }
    }
})

// Form submission UI state.
document.addEventListener('submit', (e) => {
    const form = e.target
    if (!(form instanceof HTMLFormElement)) return
    form.classList.add('submitting')
    const submitButton = form.querySelector('button[type=submit]')
    if (submitButton) submitButton.disabled = true
})

// Reset submit state after bfcache restore.
window.addEventListener('pageshow', (event) => {
    if (event.persisted) {
        document.querySelectorAll('form.submitting').forEach((form) => {
            form.classList.remove('submitting')
            const submitButton = form.querySelector('button[type=submit]')
            if (submitButton) submitButton.disabled = false
        })
    }
})