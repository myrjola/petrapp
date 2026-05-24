# StackNav — TLA+ Model of the MPA Stack Navigator

A formal model of the navigation, bfcache, prefetch, and cache-invalidation
behavior implemented in `ui/static/main.js` and its server-side counterparts
(`cmd/web/helpers.go`, `cmd/web/middleware.go`, `cmd/web/templates.go`,
`ui/templates/base.gohtml`, and `ui/templates/pages/workout/workout.gohtml`).

## Why this exists

The stack navigator coordinates four subsystems whose interaction is hard
to reason about by inspection:

1. The browser's **Navigation API** (push / replace / traverseTo).
2. The browser's **bfcache** (a document may be restored from a snapshot
   taken before a state-changing POST happened).
3. The **Speculation Rules** prefetch declared in `base.gohtml`, which
   parks a baked-at-prefetch-time response in a per-URL cache and
   promotes it on the next click.
4. A server-side **cache invalidation token** (`inv_bfcache` cookie set on
   every POST, baked into each page's `<meta name="invalidation-token">`)
   plus a JS `pageshow.persisted` handler that compares the two and forces
   a reload on mismatch.

The interesting question is whether the user can ever observe stale data
in a settled state — i.e. on a fully loaded page with no pending fetch —
after submitting a form.

## Files

- `StackNav.tla` — the model.
- `StackNav.cfg` — baseline (no cookie expiration, no prefetch). Invariant
  holds.
- `StackNav_CookieExpire.cfg` — enables cookie expiration. Invariant
  violated; 6-state trace.
- `StackNav_Prefetch.cfg` — enables Speculation Rules prefetch with no
  mitigation. Invariant violated; 6-state trace.
- `StackNav_PrefetchMitigated.cfg` — enables prefetch AND the proposed
  `EagerPagerevealCheck` mitigation. Invariant holds across 825k states.

## Running

```sh
TLA=/Applications/TLA+\ Toolbox.app/Contents/Eclipse/tla2tools.jar

# Baseline.
java -cp "$TLA" tlc2.TLC -config StackNav.cfg StackNav.tla

# Cookie-expiration counterexample.
java -cp "$TLA" tlc2.TLC -config StackNav_CookieExpire.cfg StackNav.tla

# Prefetch counterexample.
java -cp "$TLA" tlc2.TLC -config StackNav_Prefetch.cfg StackNav.tla

# Prefetch + mitigation: no counterexample.
java -cp "$TLA" tlc2.TLC -config StackNav_PrefetchMitigated.cfg StackNav.tla
```

(A trailing `java.lang.ArithmeticException: Division by zero` after
"Model checking completed. No error has been found." is a cosmetic
TLC printout artifact, not a verification failure.)

## What the model captures

| Real-world piece | TLA+ abstraction |
|---|---|
| Browser history entries | `history` (Seq of `[url, bakedToken]`) |
| Current entry index | `idx` |
| bfcache snapshots | `bfcache` (Seq of `[cached, token]` parallel to `history`) |
| Speculation Rules prefetch cache | `prefetchCache` (function from `Urls` to `[present, token]`) |
| Document lifecycle | `docState` ∈ {`Loading`, `Loaded`, `Bfcached`, `PromotedPrefetch`} |
| Server's `inv_bfcache` value | `serverToken` (rotated on every POST) |
| Browser's `inv_bfcache` cookie | `cookieToken` (NIL or `serverToken`) |
| In-flight fetch | `inFlight` ∈ {`None`, `GetPending`} |
| GET click (prefetch-aware) | `ClickLink` |
| GET response arrival | `GetResponse` |
| Browser back / forward | `GoBack` / `GoForward` |
| Form submit + `popOrPushTo` | `SubmitForm(target, replace)` |
| bfcache pageshow handler | `PageshowFresh` / `PageshowReload` / `PageLocalReplace` |
| 60s cookie MaxAge | `CookieExpire` (gated by `CookieMayExpire`) |
| Browser bfcache eviction | `BfcacheEvict` |
| Browser prefetch eviction | `PrefetchEvict` |
| Speculation Rules prefetch | `Prefetch` (gated by `PrefetchEnabled`) |
| Prefetch-promote settlement (current) | `PromotedSettleNoCheck` (when `~EagerPagerevealCheck`) |
| Pagereveal-time staleness check (mitigation) | `PromotedCheckFresh` / `PromotedCheckReload` / `PromotedPageLocalReplace` |

## What the model deliberately omits

- Visual transitions (`pagereveal`/`pageswap` animation paths).
- Service worker.
- Multi-tab interaction (`inv_bfcache` is shared across tabs).
- File uploads (the JS shim falls back to native navigation).
- The brief window during which a bfcache-restored document is
  interactive before the pageshow handler runs.
- Programmatic `navigation.navigate()` using the prefetch cache: real
  browsers vary; the model conservatively assumes only browser-initiated
  link clicks consume the cache.
- HTTP-cache `Vary: Cookie` + `must-revalidate` automatic invalidation of
  prefetched responses on cookie change. The model treats the prefetch
  cache as never auto-invalidated (worst case) so a real bug isn't
  hidden behind a "browser will probably do the right thing" assumption.

## Property of interest

```
NoStaleSettled ==
    (docState = "Loaded" /\ inFlight = "None")
        => CurrentBaked = serverToken
```

In English: once the user is on a fully loaded page with no pending
navigation, the page they're looking at was rendered with the current
server-side invalidation token.

## Counterexample 1: cookie expiration

The `inv_bfcache` cookie's 60-second `MaxAge` opens a window during which
the staleness check can wrongly conclude "fresh". TLC finds the minimal
trace in 6 steps with `CookieMayExpire = TRUE`:

```
State 1  Init           u1@0 [Loaded, srv=0, cookie=0]
State 2  ClickLink u2   u1@0 bfcached; u2@_ loading
State 3  GetResponse    u2@0 [Loaded]
State 4  SubmitForm u1  POST -> srv=1, cookie=1; popOrPushTo finds u1
                        backward -> traverses; u1 restored from bfcache
                        with bakedToken=0; docState=Bfcached
State 5  CookieExpire   cookie=0
State 6  PageshowFresh  rendered (0) == cookie (0) -> handler concludes
                        "fresh", docState=Loaded.
                        VIOLATION: bakedToken=0 != serverToken=1.
```

The page was rendered before any POST (so its baked token is `""`); a
later POST set the cookie; the cookie expired before the user navigated
back; both `rendered` and `current` are now `""` and the check is a
no-op.

## Counterexample 2: Speculation Rules prefetch

The Speculation Rules block in `base.gohtml` prefetches every `/*`
link at `conservative` eagerness. The prefetched response is baked with
the cookie value *at prefetch time* and parked in a per-URL cache. The
existing `pageshow.persisted` handler in `main.js:327` only fires on
bfcache restore, so a promoted prefetch slips through. TLC finds the
trace in 6 steps with `PrefetchEnabled = TRUE`:

```
State 1  Init               u1@0 [Loaded, srv=0]
State 2  Prefetch(u1)       prefetchCache[u1] = token 0 (no POST yet)
State 3  SubmitForm(u1)     POST same-URL replace -> srv=1, cookie=1;
                            history[1] reset to u1@NIL, docState=Loading
State 4  GetResponse        u1 baked fresh with token 1
State 5  ClickLink(u1)      browser promotes the prefetched response:
                            push history[2]=u1@0, docState=PromotedPrefetch
State 6  PromotedSettleNoCheck  no check today -> docState=Loaded.
                                VIOLATION: bakedToken=0 != serverToken=1.
```

Real-world version: user lands on the home page, taps "Start workout"
which POSTs and redirects forward, then later swipes back and taps a
day card whose URL they had previously touchstart-ed (prefetched before
the POST). The promoted response shows the pre-POST view.

## Counterexample 3: cookie expiration + prefetch

Not configured (would need both `CookieMayExpire = TRUE` and
`PrefetchEnabled = TRUE`). The mitigation that fixes counterexample 2
(`EagerPagerevealCheck`) does **not** fix the cookie-expiration variant
of counterexample 1 — the rendered/cookie comparison stays a no-op when
both sides expire to `""`. Both fixes need to be in place.

## Proposed code changes

### Fix counterexample 2 (prefetch staleness): move the check to `pagereveal`

`main.js:327` currently gates the staleness check on `event.persisted`
(bfcache restore only). The `pagereveal` event fires on every navigation
— fresh load, prefetch promotion, bfcache restore — and runs before the
first paint. Moving the check there subsumes the existing one and closes
the prefetch hole. The workout-page handler already does this; the
default needs to too.

Sketch:

```js
// Replace the existing pageshow listener with a pagereveal one.
window.addEventListener('pagereveal', () => {
    if (document.body.dataset.bfcacheHandler === 'page-local') return
    const meta = document.querySelector('meta[name="invalidation-token"]')
    const rendered = meta ? meta.content : ''
    const m = document.cookie.match(/(?:^|;\s*)inv_bfcache=([^;]+)/)
    const current = m ? m[1] : ''
    if (rendered === current) return
    if ('navigation' in window) navigation.reload()
    else location.reload()
})
```

`clearLoad()` from the existing bfcache handler still needs to run on
bfcache restore — keep a small `pageshow` listener for that:

```js
window.addEventListener('pageshow', (e) => { if (e.persisted) clearLoad() })
```

This is verified by `StackNav_PrefetchMitigated.cfg`
(`EagerPagerevealCheck = TRUE`): 825,386 reachable states, no violation.

### Fix counterexample 1 (cookie expiration): make the freshness signal not depend on cookie presence

The finite `MaxAge` on `inv_bfcache` in `middleware.go` was the root
cause. Three options were considered, in increasing invasiveness:

1. **Bump the MaxAge** to comfortably exceed any plausible bfcache
   retention window. Chrome retains bfcache up to ~10 minutes; Safari
   up to ~30 minutes. An hour-long `MaxAge` closes the typical window
   but a tab left idle past the MaxAge can still observe false-fresh
   if the browser kept the bfcache snapshot alive (mobile background
   tabs woken later, "continue where you left off" restores). The
   tightest form of Option 1 is to drop `MaxAge` entirely — emit a
   *session cookie*. A session cookie outlives any bfcache snapshot
   by construction: bfcache lives in the renderer process and is
   cleared no later than browser restart, while a session cookie is
   cleared no earlier than session end. The model's
   `CookieMayExpire = FALSE` assumption then holds in practice.

2. **Bake an absolute timestamp** into the meta tag (a server-issued
   monotonic counter or unix-millis), and have the JS check
   `rendered_ts < earliest_known_post_ts`, where `earliest_known_post_ts`
   is updated in `localStorage` on every POST response. This survives
   cookie expiration because the trigger is observed POSTs, not the
   server cookie.

3. **Always reload on `pagereveal` if persisted**: drop the cookie check
   on the global handler entirely. Page-local handlers (workout) already
   opt out. The cost is a network round-trip on every back-navigation
   that hits bfcache, even when the page would have been fresh — but
   that round-trip is exactly what the rest of the app does without
   bfcache, and bfcache is fundamentally a "saved render, may be
   stale" optimization. Saves all the cookie machinery.

**Chosen: Option 1 in its session-cookie form.** Smallest patch
(`middleware.go` drops the `MaxAge` line) and pins
`CookieMayExpire = FALSE` in practice. Option 2 remains overkill;
Option 3 (cleanest in the abstract) was rejected because it would
forfeit bfcache's instant back-navigation benefit on every non-opted-
out page.

### Tighten what the model proved is unnecessary

- `redirect: 'manual'` in `submitForm` is fine — the model doesn't see
  any case where transparently following a server-side redirect would
  bypass the check, because the post-POST navigation always goes
  through `popOrPushTo` which always lands in a state where the next
  load (fresh fetch via `GetResponse`) bakes with the latest token.
  No change needed.
- The `data-bfcache-handler="page-local"` opt-out stays. With the
  `pagereveal` change above, the opt-out check moves to the same
  handler and remains a one-line dataset read.
