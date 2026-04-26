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

const invalidateKey = (url) =>
  'stacknav:invalidate:' + url.pathname + url.search

// Intercept form submits via the submit event (not navigate+intercept) so that
// we can call replaceTo() without the outer navigate event committing a push
// entry first.  Using navigate+intercept causes a spurious push commit before
// the handler's replaceTo fires, resulting in duplicate history entries.
document.addEventListener('submit', (e) => {
  const form = e.target
  if (!(form instanceof HTMLFormElement)) return
  if (!('navigation' in window)) return
  if (form.method.toLowerCase() !== 'post') return
  const formData = new FormData(form)
  for (const [, v] of formData) {
    if (v instanceof File) return // multipart not supported by shim
  }
  const action = new URL(form.action)
  if (action.origin !== location.origin) return
  e.preventDefault()
  submitFormElement(form, action, new URLSearchParams(formData))
})

async function submitFormElement(form, action, body) {
  form.classList.add('submitting')
  const submitButton = form.querySelector('button[type=submit]')
  if (submitButton) submitButton.disabled = true

  let res
  try {
    res = await fetch(action.href, {
      method: 'POST',
      body,
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-Requested-With': 'stacknav',
      },
      redirect: 'manual',
    })
  } catch (_) {
    location.reload()
    return
  }

  if (res.status === 200) {
    const target = res.headers.get('X-Location')
    const histAction = res.headers.get('X-History-Action')
    if (!target) {
      location.reload()
      return
    }
    if (histAction === 'pop-or-replace') popOrReplaceTo(target)
    else replaceTo(target)
    return
  }

  // Anything else (5xx, missing headers, unexpected status): reload
  // to surface state. We can't render the response body in place
  // because the CSP (require-trusted-types-for 'script') blocks
  // document.write and innerHTML of HTML strings. Validation errors
  // don't reach here — they use flash + redirect-to-form and arrive
  // as a normal 200 + X-Location response.
  location.reload()
}

function replaceTo(target) {
  // For same-document URL updates, navigation.navigate() with history:'replace'
  // can cause competing navigations in Chrome. Use location.replace() to trigger
  // a clean navigation that properly completes before subsequent history traversals.
  const targetUrl = new URL(target, location.origin)
  if (sameUrl(targetUrl, new URL(location.href))) {
    // Same URL: use location.reload() to refresh without pushing to history.
    // history.replaceState ensures the entry stays as-is before reload.
    history.replaceState(history.state, '', target)
    location.reload()
  } else {
    navigation.navigate(target, { history: 'replace' })
  }
}

function popOrReplaceTo(target) {
  const targetUrl = new URL(target, location.origin)
  const entries = navigation.entries()
  for (let i = navigation.currentEntry.index - 1; i >= 0; i--) {
    if (sameUrl(new URL(entries[i].url), targetUrl)) {
      sessionStorage.setItem(invalidateKey(targetUrl), '1')
      navigation.traverseTo(entries[i].key)
      return
    }
  }
  replaceTo(target)
}


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

// Reset submit state and process bfcache invalidation marker on pageshow.
window.addEventListener('pageshow', (event) => {
  // Reset submitting forms after bfcache restore.
  if (event.persisted) {
    document.querySelectorAll('form.submitting').forEach((form) => {
      form.classList.remove('submitting')
      const submitButton = form.querySelector('button[type=submit]')
      if (submitButton) submitButton.disabled = false
    })
  }

  // Bfcache invalidation marker: reload if the entry was marked stale.
  const key = invalidateKey(new URL(location.href))
  const marker = sessionStorage.getItem(key)
  if (marker) sessionStorage.removeItem(key)
  if (event.persisted && marker) location.reload()
})
