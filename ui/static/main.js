/**
 * Convenience function to get the parent element of the current script tag.
 * Inspired by https://github.com/gnat/surreal.
 * @returns {HTMLElement}
 */
function me() {
  return document.currentScript.parentElement
}

/**
 * View transition handler for sliding animations for sliding left when we are going deeper in URL hierarchy and
 * sliding right when we are going shallower.
 */
window.addEventListener('pagereveal', async (e) => {
  if (!e.viewTransition) {
    return
  }

  const fromUrl = navigation.activation.from.url
  const entryUrl = navigation.activation.entry.url
  const depthDifference = fromUrl.split('/').length - entryUrl.split('/').length
  if (depthDifference === 0) {
    e.viewTransition.skipTransition()
  }
  e.viewTransition.types.add(depthDifference > 0 ? 'backward' : 'forward')
})

navigation.addEventListener('navigate', (e) => {
  // Stack Navigation implementation using Navigation API.
  //
  // When a form is submitted, we want the browser's back button to behave sensibly. An example of what we want to
  // avoid is when we submit a form and return to the same URL as the form, we don't want the back button to be a
  // no-op.
  //
  // Check how the backend is setting Content-Location HTTP header to understand better how this plumbs together
  // with the backend.
  if (e.formData) {
    e.intercept({
      async handler() {
        try {
          // Submit the form using fetch so that we can process the Content-Location header.
          const body = new URLSearchParams(e.formData).toString()
          const result = await fetch(e.destination.url, {
            headers: {
              "Content-Type": "application/x-www-form-urlencoded"
            },
            method: "POST",
            body,
          })
          const baseUrl = window.location.origin
          const location = result.headers.get("Content-Location")
          const locationUrl = new URL(location, baseUrl)

          // If there's an entry in the navigation history that matches the location, we want to replace it.
          for (entry of navigation.entries()) {
            const entryUrl = new URL(entry.url)
            if (entryUrl.pathname === locationUrl.pathname) {
              await navigation.traverseTo(entry.key).committed
              // This is a bit hacky to reload the page, but it's because the bfcache might show a stale page.
              navigation.reload()
              return
            }
          }
          await navigation.navigate(location, {history: "replace"})
        } catch (err) {
          // TODO: We need error feedback to the user.
          console.error("Failed to submit form:", err)
        }
      }
    })
  }
})

function findMatchingHistoryEntry(targetUrl) {
  const entries = navigation.entries()
  const currentIndex = navigation.currentEntry.index
  const targetURL = new URL(targetUrl)

  // Search backwards through history
  for (let i = currentIndex - 1; i >= 0; i--) {
    const entry = entries[i]
    const entryURL = new URL(entry.url)

    if (urlsMatch(entryURL, targetURL)) {
      return entry
    }
  }

  return null
}

function urlsMatch(url1, url2) {
  return url1.href === url2.href ||
    (url1.pathname === url2.pathname && url1.origin === url2.origin)
}

// Create a smart back button using the Navigation API to make it pop the navigation stack.
document.addEventListener("DOMContentLoaded", function () {
  // Find all anchors marked for back button enhancement.
  const backLinks = document.querySelectorAll('a[data-back-button]')

  backLinks.forEach(link => {
    const destinationUrl = link.href

    // Only enhance it if Navigation API is supported
    if (typeof navigation !== 'undefined') {
      link.addEventListener('click', async (e) => {
        const matchingEntry = findMatchingHistoryEntry(destinationUrl)

        if (matchingEntry) {
          e.preventDefault()
          await navigation.traverseTo(matchingEntry.key)
        }
      })
    }
  })
})

// Form submission detector
document.addEventListener('submit', function (e) {
  const form = e.target
  form.classList.add('submitting')
  const submitButton = form.querySelector("button[type=submit]")
  if (submitButton) {
    submitButton.disabled = true
  }
})

// Reset form states when the page is loaded from the browser cache (back button).
window.addEventListener('pageshow', function (event) {
  // pageshow event fires when the page is loaded, including from cache.
  // event.persisted is true when the page is loaded from the back/forward cache.
  if (event.persisted) {
    // This was a back/forward navigation from the cache.
    document.querySelectorAll('form.submitting').forEach(form => {
      form.classList.remove('submitting')
      const submitButton = form.querySelector("button[type=submit]")
      if (submitButton) {
        submitButton.disabled = false
      }
    })
  }
})
