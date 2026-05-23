# StackNav — TLA+ Model of the MPA Stack Navigator

A formal model of the navigation, bfcache, and cache-invalidation behavior
implemented in `ui/static/main.js` and its server-side counterparts
(`cmd/web/helpers.go`, `cmd/web/middleware.go`, `cmd/web/templates.go`,
and `ui/templates/pages/workout/workout.gohtml`).

## Why this exists

The stack navigator coordinates three subsystems whose interaction is hard
to reason about by inspection:

1. The browser's **Navigation API** (push / replace / traverseTo).
2. The browser's **bfcache** (a document may be restored from a snapshot
   taken before a state-changing POST happened).
3. A server-side **cache invalidation token** (`inv_bfcache` cookie set on
   every POST, baked into each page's `<meta name="invalidation-token">`)
   plus a JS `pageshow.persisted` handler that compares the two and forces
   a reload on mismatch.

The interesting question is whether the user can ever observe stale data
in a settled state — i.e. on a fully loaded page with no pending fetch —
after submitting a form.

## Files

- `StackNav.tla` — the model.
- `StackNav.cfg` — runs the model with cookie expiration **disabled**.
  `NoStaleSettled` is expected to hold.
- `StackNav_CookieExpire.cfg` — runs the model with the production-realistic
  60-second cookie expiration enabled. `NoStaleSettled` is expected to be
  **violated**; the counterexample is the staleness window that opens
  once the `inv_bfcache` cookie has expired.

## Running

With the TLA+ toolbox (`tlc2.TLC`) on the classpath:

```sh
# Baseline: cookie always present. Invariant should hold.
java -cp /path/to/tla2tools.jar tlc2.TLC -config StackNav.cfg StackNav.tla

# Cookie expiration enabled. Expect a NoStaleSettled violation trace.
java -cp /path/to/tla2tools.jar tlc2.TLC \
    -config StackNav_CookieExpire.cfg StackNav.tla
```

## What the model captures

| Real-world piece | TLA+ abstraction |
|---|---|
| Browser history entries | `history` (Seq of `[url, bakedToken]`) |
| Current entry index | `idx` |
| bfcache snapshots | `bfcache` (Seq of token-or-NIL parallel to `history`) |
| Document lifecycle | `docState` ∈ {`Loading`, `Loaded`, `Bfcached`} |
| Server's `inv_bfcache` value | `serverToken` (rotated on every POST) |
| Browser's `inv_bfcache` cookie | `cookieToken` (NIL or `serverToken`) |
| In-flight fetch | `inFlight` ∈ {`None`, `GetPending`} |
| GET click | `ClickLink` |
| GET response arrival | `GetResponse` |
| Browser back / forward | `GoBack` / `GoForward` |
| Form submit + `popOrPushTo` | `SubmitForm(target, replace)` — three sub-branches: replace / traverse-to-backward-match / push |
| `pageshow.persisted` handler decision | `PageshowFresh` / `PageshowReload` / `PageLocalReplace` |
| 60s cookie MaxAge | `CookieExpire` (gated by `CookieMayExpire`) |
| Browser bfcache eviction | `BfcacheEvict` |

## What the model deliberately omits

- View transitions (`pagereveal`, `pageswap`) — they are visual polish
  and don't affect cache invalidation correctness.
- Service worker.
- Multi-tab interaction (the `inv_bfcache` cookie is shared across tabs;
  a single-tab model is the simplest one that can refute or confirm the
  invariant).
- Speculation Rules prefetch.
- File uploads (the JS shim falls back to native navigation; the model
  conservatively assumes the enhanced path).
- The brief window during which a bfcache-restored document is
  interactive before the pageshow handler runs (the model treats
  `SubmitForm` etc. as guarded by `docState = "Loaded"`).

## Property of interest

```
NoStaleSettled ==
    (docState = "Loaded" /\ inFlight = "None")
        => CurrentBaked = serverToken
```

In English: once the user is on a fully loaded page with no pending
navigation, the page they're looking at was rendered with the current
server-side invalidation token. There is no "Loaded but stale" state.

## Known counterexample: cookie expiration

The `inv_bfcache` cookie's 60-second `MaxAge` opens a window during which
the staleness check can wrongly conclude "fresh". TLC finds the minimal
trace in 6 steps with `CookieMayExpire = TRUE`:

```
State 1  Init           u1@0 [Loaded, srv=0, cookie=0]
State 2  ClickLink u2   u1@0 bfcached; u2@_ loading
State 3  GetResponse    u2@0 [Loaded]
State 4  SubmitForm u1  POST -> srv=1, cookie=1; client popOrPushTo
                        finds u1 backward -> traverses; u1 restored from
                        bfcache with bakedToken=0; docState=Bfcached
State 5  CookieExpire   cookie=0
State 6  PageshowFresh  rendered (0) == cookie (0) -> handler concludes
                        "fresh", docState=Loaded.
                        VIOLATION: bakedToken=0 != serverToken=1.
```

The realistic scenario this abstracts: user submits a form, lands back on
a page that was bfcached *before* the POST, and the cookie has since
expired (the 60s `MaxAge` is short enough that this happens on any
sufficiently long flow with intermediate idle time). The
`pageshow.persisted` handler reads both `rendered` and `current` as the
empty string and skips the reload, leaving the user looking at a snapshot
that predates the mutation they just made.

Mitigations to evaluate (not implemented):

- Bump `MaxAge` to a value comfortably longer than any plausible bfcache
  retention window (Chrome caps bfcache at ~10 minutes; Safari ~30 minutes).
- Bake an absolute "rendered-at" timestamp into the meta and reload if it
  pre-dates the latest POST-set cookie, even when the cookie is absent.
- Skip the cookie check entirely: trigger a reload on every
  `pageshow.persisted` unless the page opts out (workout.gohtml-style
  page-local handler is already opt-in via `data-bfcache-handler`).
