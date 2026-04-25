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

if ('navigation' in window) {
  navigation.addEventListener('navigate', (e) => {
    if (!e.formData) return
    if (!e.canIntercept || e.hashChange || e.downloadRequest) return
    if (new URL(e.destination.url).origin !== location.origin) return
    for (const [, v] of e.formData) {
      if (v instanceof File) return
    }
    e.intercept({ handler: () => submitForm(e) })
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
      redirect: 'manual',
    })
  } catch (_) {
    location.reload()
    return
  }

  if (res.status === 200) {
    const target = res.headers.get('X-Location')
    const action = res.headers.get('X-History-Action')
    if (!target) {
      location.reload()
      return
    }
    if (action === 'pop-or-replace') popOrReplaceTo(target)
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
  navigation.navigate(target, { history: 'replace' })
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
