# Prefetch staleness fix — design

## Problem

The Speculation Rules block in `ui/templates/base.gohtml:26-35` declares
`prefetch` at `conservative` eagerness for every `/*` link. When the user
touches a link, Chromium issues a `GET` for the target; the response —
baked with whatever `inv_bfcache` cookie value the request carried — is
parked in the browser's prefetch cache. When the user actually clicks,
the prefetched response is promoted into the navigation and the
document is constructed without a network round-trip.

If a POST runs between prefetch and promotion, the promoted document's
`<meta name="invalidation-token">` is older than the current
`inv_bfcache` cookie. The user is then settled on a page whose data
predates the mutation they just made.

Today's staleness check in `ui/static/main.js:327` is gated on
`event.persisted` — true only on bfcache restore. Prefetch promotion
slips through silently.

The bug was surfaced by `tlaplus/StackNav_Prefetch.cfg`, which produces
a 6-state TLC counterexample:

```
State 1  Init               u1@0 [Loaded, srv=0]
State 2  Prefetch(u1)       prefetchCache[u1] = token 0 (no POST yet)
State 3  SubmitForm(u1)     POST same-URL replace -> srv=1, cookie=1
State 4  GetResponse        u1 baked fresh with token 1
State 5  ClickLink(u1)      promoted prefetch: push u1@0,
                            docState=PromotedPrefetch
State 6  PromotedSettleNoCheck  no check today -> docState=Loaded.
                                VIOLATION: bakedToken=0 != serverToken=1
```

The same TLA+ run with `EagerPagerevealCheck = TRUE`
(`StackNav_PrefetchMitigated.cfg`) holds `NoStaleSettled` across 825,386
reachable states. The mitigation is to move the staleness check from
`pageshow.persisted` to `pagereveal`, which fires on every navigation
reveal — fresh load, prefetched-and-promoted load, bfcache restore.

## Goals

- Catch every "Loaded but stale" state caused by Speculation Rules
  prefetch promotion landing on a doc whose meta token predates the
  current `inv_bfcache` cookie.
- Preserve the existing bfcache staleness check (today's working path).
- Preserve `workout.gohtml`'s strike-through animation flow: the
  page-local `pagereveal` handler must still drive a replace navigation
  that keeps the old bfcache snapshot available to the outgoing
  view-transition snapshot. The global handler must not preempt it.
- Verified by `tlaplus/StackNav_PrefetchMitigated.cfg`.

## Non-goals

- Restructuring the cookie/meta/token machinery (option 2/3 from
  `tlaplus/README.md`). The 1h `inv_bfcache` `MaxAge` bumped in `31c19cd`
  closed the cookie-expiration window; the existing comparison-based
  check is good enough.
- Backwards-compatibility for browsers without `pagereveal`. Chrome 126+,
  Firefox 137+, Safari 18.4+ — broadly available in 2026. The same
  browsers that lack `pagereveal` also lack Speculation Rules, so the
  fix doesn't matter to them, and the bfcache staleness window for those
  browsers is already small enough after the cookie MaxAge bump.
- A test for prefetch staleness end-to-end. Playwright can't reliably
  trigger Speculation Rules prefetch; the TLA+ spec is the authority.
- A reload-loop guard. The browser's spec-defined behavior (prefetch
  entry consumed once on promotion; `reload`-mode navigation bypasses
  prefetch cache) makes a loop unreachable. If a browser bug produces
  one in practice, add a `sessionStorage` one-shot then; not pre-emptive.
- Changes to the page-local opt-out mechanism
  (`document.body.dataset.bfcacheHandler === 'page-local'`). Today's
  attribute-based convention stays.

## Architecture

One file, one change.

### `ui/static/main.js`

The current `pageshow` listener at line 327 does two things:

1. Calls `clearLoad()` to reset the button-spinner state captured in the
   bfcache snapshot.
2. Runs the rendered-vs-cookie staleness check and calls
   `navigation.reload()` on mismatch.

The change splits these:

- The `pageshow.persisted` listener is reduced to just the `clearLoad()`
  call. The bfcache snapshot of `aria-busy` / `.btn-loading-label`
  markup is captured by the document being snapshotted; `pageshow` is
  the right hook to reset it on restore.
- A new `pagereveal` listener carries the staleness check. It fires on
  every navigation reveal (fresh, prefetched-and-promoted, bfcache
  restore) and bails early when
  `document.body.dataset.bfcacheHandler === 'page-local'` so that
  `workout.gohtml`'s own `pagereveal` handler stays in charge of its
  flow.

The page-local opt-out attribute is set by `workout.gohtml` from an
inline script in the body, which runs as part of parsing. `pagereveal`
fires after parsing is complete (before first paint) per the HTML spec,
so the global handler always sees the attribute when reading it.

The existing `pagereveal` listener at `main.js:254` (view-transition
direction classifier) is untouched. It bails early on
`navigationType === 'reload'` via `e.viewTransition.skipTransition()`,
so the staleness-driven reload doesn't pick up a stray slide animation.

### Resulting code

```js
// Clear the bfcache-captured button spinner on restore. The navigation
// that started the spinner has long since resolved; the snapshot just
// has stale aria-busy / .btn-loading-label markup that startLoad would
// otherwise leave dangling.
window.addEventListener('pageshow', (event) => {
    if (event.persisted) clearLoad()
})

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
```

The `location.reload()` fallback present in today's handler is dropped:
`pagereveal` is part of the Navigation API, so if `pagereveal` fires,
`navigation` is defined.

## Per-flow behavior

| Flow | Today | After fix |
|---|---|---|
| Fresh load (no prefetch, no bfcache) | no check | check runs; tokens match by construction; no-op |
| bfcache restore (non-page-local) | check runs via `pageshow.persisted`; reload on mismatch | check runs via `pagereveal`; reload on mismatch |
| bfcache restore (workout, page-local) | global `pageshow.persisted` returns early; page-local pagereveal drives replace navigation + view transition | global `pagereveal` returns early; page-local pagereveal unchanged |
| Prefetch-promoted load (non-page-local) | **no check** → user settles on stale doc | check runs via `pagereveal`; reload on mismatch |
| Prefetch-promoted load (workout, page-local) | page-local `pagereveal` already runs; if stale, replace navigation + view transition | unchanged |
| Reload triggered by staleness check | n/a (only bfcache path today) | `navigation.reload()`; direction classifier sees `navigationType === 'reload'` and `skipTransition()`s; document is refetched fresh; meta = cookie on arrival; pagereveal handler re-runs, sees match, no-op |

## Testing

### Playwright: bfcache staleness path (new)

Add a `Test_playwright_bfcache_staleness` flow to
`cmd/web/playwright_test.go` that locks in the listener wiring on the
highest-traffic path:

1. Visit `/` (no POSTs yet; meta token empty).
2. Visit `/workouts/{date}`.
3. Trigger a POST that rotates `inv_bfcache` (e.g. record a set or click
   `Start workout`).
4. Navigate back via `page.GoBack()` to `/`.
5. Wait for the pagereveal-driven reload to complete (`page.WaitForURL`
   on `/`).
6. Read meta token and `inv_bfcache` cookie via `page.Evaluate`.
7. Assert: both are non-empty AND equal.

The assertion fails today on `pageshow.persisted` browsers if the
listener were missing; it passes both today and after the fix. Its job
is regression protection — locking in that the listener wiring works at
all.

### Manual: workout strike-through animation (no automation)

Hard to assert programmatically without brittle CSS/transition
inspection. Pre-merge manual check:

1. Open `/workouts/{today}`.
2. Complete a set on one exercise.
3. Navigate back to `/workouts/{today}` (bfcache restore).
4. Observe the strike-through line draws on the just-completed card.

If broken, the symptom is "card just appears already-struck (no
animation)" or "page reloads without animation, then card shows
already-struck." The page-local handler should be unaffected by the
global change, but verify by eye.

### Prefetch staleness (no automation)

`StackNav_Prefetch.cfg` and `StackNav_PrefetchMitigated.cfg` are the
authority. Playwright's `page.hover()` does not reliably trigger
Speculation Rules at `conservative` eagerness; Chromium can decline to
prefetch for any reason. Attempting a flaky test is worse than no test.

## Migration

None. Single-file change, no schema, no shared state, no feature flag.
Old clients (those who loaded the page before the new fingerprinted
`main.js` shipped) continue running the `pageshow.persisted` handler
until they reload; new clients run the `pagereveal` handler. Old and new
handlers don't conflict — different events, identical comparison logic.

## Rollback

Revert `ui/static/main.js`. No coordination needed. The fingerprinted
asset cycles on next deploy.

## References

- `tlaplus/StackNav.tla` — the model.
- `tlaplus/StackNav_Prefetch.cfg` — counterexample (today's code).
- `tlaplus/StackNav_PrefetchMitigated.cfg` — proof (after fix).
- `tlaplus/README.md` — mitigation options + rationale.
- `ui/static/main.js:254` — existing pagereveal direction classifier.
- `ui/static/main.js:327` — pageshow.persisted handler being split.
- `ui/templates/pages/workout/workout.gohtml:444` — page-local
  `pagereveal` handler that drives strike-through view transition.
- `ui/templates/base.gohtml:26` — Speculation Rules block.
- Commit `31c19cd` — preceding fix that bumped `inv_bfcache` MaxAge
  from 60s to 1h, closing the cookie-expiration staleness window.
