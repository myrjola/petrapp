# Stack Navigator Redesign

## Background

The app has a small JS layer that makes browser back-button behavior feel app-like on top of a server-rendered Go MPA. The current implementation in `ui/static/main.js` (form-submit interception, smart back-link, `pagereveal` direction) and `cmd/web/helpers.go` (`redirect`) has known bugs:

- `Content-Location` is misused as a redirect-target hint (RFC 9110 §8.7 reserves it for "the URL the response body represents").
- `navigation.activation.from.url` is read unconditionally, crashing on cold loads where `from` is `null`.
- The Navigation API is not feature-detected; the file throws at parse time on browsers without it.
- `e.canIntercept` is not checked before `e.intercept()`.
- URL matching ignores `search`, so paginated/filtered entries collapse incorrectly.
- Non-2xx responses (validation errors) silently navigate to a wrong URL because `Content-Location` is missing.
- Pop-to-existing-entry walks `navigation.entries()` forward instead of backward, picking the oldest match instead of the most recent.
- View-transition direction is inferred from URL depth, which fails for sibling navigation.
- `traverseTo` is followed by an eager `reload()` that flashes a stale page from bfcache.

This spec replaces the stack navigator with a smaller, standards-clean implementation built on the Navigation API and progressive enhancement of POST-Redirect-Get.

## Goals

- Browser back button (and edge-swipe gestures) behave like a native mobile app for the three main flows: same-URL form submit, cross-URL form submit, pop-to-ancestor form submit.
- Single roundtrip for enhanced submits (no redirect-follow penalty).
- Standards-conformant: no misuse of `Content-Location`; clearly-named custom headers; explicit JS detection.
- Progressive enhancement: forms work without JS via plain 303 PRG; the JS shim is a no-op without Navigation API.
- Validation errors round-trip cleanly via flash + redirect-to-form (CSP `require-trusted-types-for 'script'` blocks in-place HTML rendering, so we keep the existing flash pattern).
- Specified behavior verified by Playwright tests that drive the back button.

## Non-goals

- Native swipe-back gestures inside iOS standalone PWAs (platform doesn't allow this).
- A virtual stack or in-memory page cache. The URL is the source of truth.
- File uploads through the enhanced submit path. Forms with `File` entries fall through to native submission.
- Cross-document View Transitions polish beyond direction classification (no per-element morphs, no scroll coordination beyond the browser default).

## Architecture

Three pieces, each with a single responsibility:

### Server (`cmd/web/helpers.go`)

A new helper, `redirectAfterPOST`, replaces uses of the existing `redirect`:

```go
// redirectAfterPOST sends the client to target after a successful POST.
// action is "" (default: replace) or "pop-or-replace" to traverse to an
// existing matching history entry when present.
//
// JS-enhanced submits (X-Requested-With: stacknav) get HTTP 200 with
// X-Location and optional X-History-Action headers, and an empty body.
// Non-JS submits get a standard 303 See Other.
func (app *application) redirectAfterPOST(w http.ResponseWriter, r *http.Request, target, action string) {
    if r.Header.Get("X-Requested-With") == "stacknav" {
        w.Header().Set("X-Location", target)
        if action != "" {
            w.Header().Set("X-History-Action", action)
        }
        w.WriteHeader(http.StatusOK)
        return
    }
    http.Redirect(w, r, target, http.StatusSeeOther)
}
```

The existing `redirect` is removed. Validation-error paths in handlers continue to use `putFlashError` + `redirectAfterPOST` back to the form URL — this fits the same wire protocol, and the GET handler pops the flash on re-render. (We cannot render the error response in place because the app's CSP includes `require-trusted-types-for 'script'`, which blocks `document.write` and `innerHTML` of HTML strings.)

### Client (`ui/static/main.js`)

Gated on `'navigation' in window`. Without Navigation API, the file does nothing and forms submit natively (303 redirect path).

Top-level pieces:

1. `navigate` listener that intercepts form submits.
2. `submitForm` handler: fetches the POST, dispatches by status.
3. `replaceTo` / `popOrReplaceTo`: history-action implementations.
4. `pageshow` listener: bfcache invalidation marker consumer.
5. `pagereveal` listener: view-transition direction classifier.
6. `click` delegator for `a[data-back-button]`: smart pop-to-ancestor links.
7. Existing `.submitting` class and button-disable on `submit` (kept).

### Wire protocol

| Direction | Header | Value | Meaning |
|---|---|---|---|
| Request | `X-Requested-With` | `stacknav` | This POST is from the JS shim. Server returns 200 instead of 303. |
| Response | `X-Location` | URL path | Where the client should navigate. |
| Response | `X-History-Action` | `replace` (default), `pop-or-replace` | History semantics. |

Response status codes used by the contract:

- **200**: success or validation error, navigate per `X-Location` and `X-History-Action`. Body empty.
  - Success → `X-Location` points to the next page.
  - Validation error → server sets a flash message and `X-Location` points back to the form URL; the form's GET handler pops the flash on re-render.
- **Anything else (5xx, network error, non-200)**: client falls back to `location.reload()` to surface state.

Non-JS submits work unchanged: server returns 303, browser follows.

The CSP (`require-trusted-types-for 'script'`) prevents the client from rendering arbitrary HTML in place via `document.write` or `innerHTML`. This is why validation flows use a re-redirect rather than a direct re-render.

## Per-flow behavior

These are the assertions verified by the Playwright spec.

### Flow 1 — Same-URL replace (set update)

- Setup: `[HOME, DETAIL]`, at DETAIL.
- Action: submit set update form. Server: `redirectAfterPOST(w, r, detailURL, "")`.
- Result: history `[HOME, DETAIL]` (replaced), at DETAIL. Browser back → HOME.

### Flow 2 — Cross-URL replace (swap exercise)

- Setup: `[HOME, DETAIL_42, SWAP]`, at SWAP.
- Action: submit swap form, server picks exercise 99. `redirectAfterPOST(w, r, "/workouts/.../exercises/99", "")`.
- Result: history `[HOME, DETAIL_42, DETAIL_99]` (SWAP replaced), at DETAIL_99. Back → DETAIL_42, back again → HOME.

The stale DETAIL_42 in history is acceptable per Q2 of the brainstorm. A future task assigning stable per-position exercise IDs will fix the 404-on-back risk.

### Flow 3 — Pop-or-replace (schedule submit)

- Setup: `[HOME, SCHEDULE]` (HOME redirected to SCHEDULE for empty prefs), at SCHEDULE.
- Action: submit valid schedule. Server: `redirectAfterPOST(w, r, "/", "pop-or-replace")`.
- Client: walks entries backward, finds HOME entry, sets `sessionStorage["stacknav:invalidate:/"] = "1"`, calls `traverseTo(homeKey)`. Browser may restore HOME from bfcache; the destination's `pageshow` handler sees the marker and `event.persisted === true`, reloads.
- Result: history `[HOME, SCHEDULE]` (cursor moved to HOME). Back from HOME goes outside the app — not back to SCHEDULE in the next forward step from elsewhere.

### Flow 4 — Hierarchical back link (`data-back-button`)

- Setup: `[HOME, DETAIL, SWAP]`, at SWAP.
- Action: click in-page "Back to exercise" link.
- Client: walks entries backward, finds DETAIL by full-URL match, calls `traverseTo`. No fetch, no `X-*` headers — pure history operation.
- Result: history `[HOME, DETAIL, SWAP]` (cursor at DETAIL). Browser back → HOME. Browser forward → SWAP.

If no matching entry exists (e.g. cold deep-link to SWAP), the click handler does not call `preventDefault` and the link navigates normally — a regular push. This makes `data-back-button` a hierarchical "Up" link with no degraded fallback.

### Flow 5 — Validation error

- Setup: at SCHEDULE.
- Action: submit empty form.
- Server: `putFlashError("Please schedule at least one workout day.")` then `redirectAfterPOST(w, r, "/schedule", "")` — same wire-protocol shape as a successful same-URL replace.
- Client: 200 + `X-Location: /schedule` → `replaceTo("/schedule")` → fresh GET of `/schedule`, which pops the flash and renders the alert.
- Result: SCHEDULE re-rendered at the same URL with the alert visible. URL unchanged. History `[…, /schedule]` (the original entry, replaced).

## Client implementation details

### Form-submit interception

```js
if ('navigation' in window) {
  navigation.addEventListener('navigate', (e) => {
    if (!e.formData) return;
    if (!e.canIntercept || e.hashChange || e.downloadRequest) return;
    if (new URL(e.destination.url).origin !== location.origin) return;
    for (const [, v] of e.formData) {
      if (v instanceof File) return; // multipart not supported by shim
    }
    e.intercept({ handler: () => submitForm(e) });
  });
}
```

`submitForm`:

```js
async function submitForm(e) {
  const body = new URLSearchParams(e.formData);
  let res;
  try {
    res = await fetch(e.destination.url, {
      method: 'POST',
      body,
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-Requested-With': 'stacknav',
      },
      redirect: 'manual',
    });
  } catch (_) {
    location.reload();
    return;
  }

  if (res.status === 200) {
    const target = res.headers.get('X-Location');
    const action = res.headers.get('X-History-Action');
    if (!target) { location.reload(); return; }
    if (action === 'pop-or-replace') popOrReplaceTo(target);
    else replaceTo(target);
    return;
  }

  // Anything else (5xx, network shape, missing headers): reload the
  // current page to surface state. We can't render the response body
  // in place because the CSP blocks document.write/innerHTML of HTML
  // strings. Validation errors don't reach this branch — they use
  // flash + redirect-to-form and arrive as a normal 200 + X-Location.
  location.reload();
}
```

### History action implementations

```js
function replaceTo(target) {
  navigation.navigate(target, { history: 'replace' });
}

function popOrReplaceTo(target) {
  const targetUrl = new URL(target, location.origin);
  const entries = navigation.entries();
  for (let i = navigation.currentEntry.index - 1; i >= 0; i--) {
    if (sameUrl(new URL(entries[i].url), targetUrl)) {
      sessionStorage.setItem(invalidateKey(targetUrl), '1');
      navigation.traverseTo(entries[i].key);
      return;
    }
  }
  replaceTo(target);
}

const sameUrl = (a, b) =>
  a.origin === b.origin && a.pathname === b.pathname && a.search === b.search;

const invalidateKey = (url) =>
  'stacknav:invalidate:' + url.pathname + url.search;
```

### bfcache invalidation marker

```js
window.addEventListener('pageshow', (e) => {
  const key = invalidateKey(new URL(location.href));
  const marker = sessionStorage.getItem(key);
  if (marker) sessionStorage.removeItem(key);
  if (e.persisted && marker) location.reload();
});
```

The marker is consumed regardless of `e.persisted` to avoid stale entries surviving across sessions. Reload only fires when the page actually came from bfcache.

### View-transition direction

```js
window.addEventListener('pagereveal', (e) => {
  if (!e.viewTransition) return;
  if (!('navigation' in window)) return;
  const act = navigation.activation;
  if (!act) return;
  if (act.navigationType === 'replace' || act.navigationType === 'reload') {
    e.viewTransition.skipTransition();
    return;
  }
  let dir = 'forward';
  if (act.navigationType === 'traverse' && act.from && act.entry) {
    dir = act.entry.index < act.from.index ? 'backward' : 'forward';
  }
  e.viewTransition.types.add(dir);
});
```

Fixes the null-`from` crash. Index comparison replaces URL-depth heuristic.

### Smart back link

```js
document.addEventListener('click', (e) => {
  if (!('navigation' in window)) return;
  const link = e.target.closest('a[data-back-button]');
  if (!link) return;
  const target = new URL(link.href);
  const entries = navigation.entries();
  for (let i = navigation.currentEntry.index - 1; i >= 0; i--) {
    if (sameUrl(new URL(entries[i].url), target)) {
      e.preventDefault();
      navigation.traverseTo(entries[i].key);
      return;
    }
  }
});
```

Event delegation so dynamically-rendered links work. No fallback handler — without a match, the link's natural href takes over.

### Submitting state (unchanged)

```js
document.addEventListener('submit', (e) => {
  e.target.classList.add('submitting');
  e.target.querySelector('button[type=submit]')?.setAttribute('disabled', '');
});

window.addEventListener('pageshow', (event) => {
  if (!event.persisted) return;
  document.querySelectorAll('form.submitting').forEach((form) => {
    form.classList.remove('submitting');
    form.querySelector('button[type=submit]')?.removeAttribute('disabled');
  });
});
```

The two `pageshow` handlers (this one and the bfcache invalidator) can be merged or kept separate — implementation detail.

## Server-side changes

### Handlers to update

Every POST handler that currently calls `redirect(w, r, target)` is updated to `app.redirectAfterPOST(w, r, target, action)`:

| Handler | File | Action |
|---|---|---|
| `schedulePOST` | `handler-schedule.go` | `pop-or-replace` (target `/`) |
| `workoutStartPOST` | `handler-workout.go` | `replace` (target `/workouts/{date}`) |
| `workoutCompletePOST` | `handler-workout.go` | `replace` |
| `workoutFeedbackPOST` | `handler-workout.go` | `replace` |
| `workoutAddExercisePOST` | `handler-workout.go` | `replace` |
| `workoutSwapExercisePOST` | `handler-workout.go` | `replace` (cross-URL ok per Flow 2) |
| `exerciseSetUpdatePOST` | `handler-exerciseset.go` | `replace` (same-URL) |
| `exerciseSetWarmupCompletePOST` | `handler-exerciseset.go` | `replace` |
| `preferencesPOST` | `handler-preferences.go` | `replace` |
| `deleteUserPOST` | `handler-preferences.go` | `replace` (target `/`) |
| Admin POSTs | `handler-admin-*.go` | `replace` |

The full audit happens during implementation. Handlers that re-render forms on validation error must use HTTP 422 (some currently use 200 + re-render — these need updating).

### Removed code

- `redirect` function in `helpers.go` is removed (replaced by `redirectAfterPOST`).
- `Sec-Fetch-Dest: empty` branch is gone.
- `Content-Location` is no longer set anywhere by the app.

## Testing

A new Playwright test, `Test_playwright_stacknav` (or extension of the existing smoke test), drives the five flows above. The test:

- Registers a user.
- Submits a valid schedule (Flow 3 + bfcache invalidation observable via post-reload content).
- Navigates into a workout day.
- Submits a set update (Flow 1) and asserts back goes to workout overview, not the same exercise page.
- Visits exercise swap, submits a swap (Flow 2) and asserts back goes to the previous exercise, then to workout overview.
- Visits exercise swap again, clicks the `data-back-button` (Flow 4) and asserts traversal (not push).
- Submits the empty schedule form to assert Flow 5 (URL unchanged, alert present).

Existing assertions continue to pass; the test extension adds back-button clicks and URL assertions.

Browser console errors fail the test (existing pattern).

### Unit-test surface

`redirectAfterPOST` gets a small unit test in `cmd/web/helpers_test.go`:

- With `X-Requested-With: stacknav`: response is 200, has `X-Location` and (when set) `X-History-Action`, body empty.
- Without the header: response is 303, has `Location` header.

## Browser support

- Navigation API support is required for the JS layer to do anything. Without it, forms submit natively (303 redirect path) and the app works as a plain MPA. As of Jan 2026, Navigation API is Baseline Newly Available (Safari 26.2+, Firefox 147+, Chrome 102+).
- `pagereveal` is Chromium-only as of early 2026. On Safari/Firefox the listener is a no-op; navigation works without view transitions.
- `bfcache` is universal; `event.persisted` is reliable.

## Open questions

- Whether to keep two separate `pageshow` handlers (submitting-state reset and bfcache invalidator) or merge them. Style call during implementation.

## References

- Internal: prior research write-up at `~/Downloads/compass_artifact_wf-013df750-5d4e-47cb-a1a2-ea81ecb77eca_text_markdown.md` (PWA navigation guide).
- MDN: [Navigation API](https://developer.mozilla.org/en-US/docs/Web/API/Navigation_API), [NavigateEvent](https://developer.mozilla.org/en-US/docs/Web/API/NavigateEvent), [bfcache](https://web.dev/articles/bfcache).
- RFC 9110 §8.7 (Content-Location semantics).
