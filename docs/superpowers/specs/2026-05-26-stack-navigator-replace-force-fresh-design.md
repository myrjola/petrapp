# Stack Navigator: Force-Fresh Fetch on Replace

## Background

The `2026-05-25-preferences-scroll-and-flash-design.md` spec adopted a
fragment-bearing redirect target as the MPA recipe for "show flash banner +
scroll to panel" after a form submit (e.g. `POST /preferences/deload`
redirects to `/preferences#deload-title`). The spec asserted "Stack-
navigator: no changes", assuming `navigation.navigate('/p#a', {history:
'replace'})` from `/p` would both honor the fragment scroll AND re-render
the page for the new flash.

The fragment scroll part is true. The re-render is not: when target shares
pathname+search with current and only the hash differs (or matches), the
browser resolves it as a same-document operation and skips the GET. The
freshly-set session flash never reaches a renderer. Symptoms:

- Submit button's spinner stays on (the new navigate event has
  `hashChange=true` so the shim's navigate handler returns early without
  calling `startLoad`/`clearLoad`).
- The success banner doesn't appear on the panel.
- The next unrelated GET pops the orphaned flash and renders it on the
  wrong page; `workoutGET` hard-codes `Variant=BannerVariantError`, so a
  success message later surfaces as a red error banner on a workout page.

Two earlier fix commits (`ceea54b`, `024df74`) progressively narrowed the
hole with `history.replaceState + location.reload` — but `location.reload`
triggers scroll-restoration on most browsers, so the fragment in the URL
bar is ignored and the panel never scrolls into view. We patched the
banner but broke the scroll.

## The model–reality mismatch

The TLA+ model in `tlaplus/StackNav.tla` treats `Urls` as a finite opaque
set and `SubmitForm(target, replace)`'s replace branch unconditionally
transitions:

```
docState' = "Loading"
inFlight' = "GetPending"
```

— i.e. every replace fires a real fetch. The `NoStaleSettled` invariant
rests on that. Reality diverges when the browser resolves the navigation as
same-document: `docState` stays `Loaded`, `bakedToken` stays stale, and the
invariant is silently violated. The model doesn't catch it because URL
distinctions below `Urls`-equality (fragments, identical-URL semantics) are
abstracted away.

We can close the gap two ways:

1. Drop fragments from redirect targets entirely. The model stays
   fragment-blind, the implementation stays fragment-blind, no special
   handling needed. URL bar no longer carries `#anchor` — rejected as a
   product constraint.
2. Make every replace-branch firing trigger a real cross-document fetch
   regardless of URL form. This brings reality back to what the model
   already assumes; no model change needed.

This spec adopts option 2.

## Goals

- Every replace-branch firing in `popOrPushTo` triggers a real
  cross-document GET, regardless of whether the target URL is identical to
  current, fragment-only different, or path-different.
- The URL bar carries the canonical redirect target (including any
  fragment) after the new document parses — no permanent cache-bust
  artifact in the URL.
- The fragment in the redirect URL still drives the browser's native
  scroll-to-anchor behavior on the new document.
- TLA+ model verifies the same invariant (`NoStaleSettled`) over the same
  state space, with no model changes.
- The two superseded fix commits (`ceea54b`, `024df74`) are removed in
  favor of one uniform mechanism.

## Non-goals

- No new wire protocol header. `X-Location` and `X-Replace-Url` are kept
  as-is.
- No handler-side awareness of the cache-bust. Handlers keep calling
  `redirect(w, r, "/path#anchor")`. The shim handles everything.
- No fragment-aware refinement of the TLA+ model. The model stays
  URL-opaque; the implementation guarantees the assumption the model
  already encodes.
- No change to push or traverse branches — they cross URL boundaries
  already and the browser fetches.

## Approach

Extend the app's existing "force-fresh against browser default staleness"
vocabulary. Today:

- `inv_bfcache` cookie + `<meta name="invalidation-token">` + pagereveal
  staleness check handles **bfcache** staleness.
- A new `bf_inv` query param, injected by the shim's replace branch,
  handles **same-document-hop** staleness.

Both rotate on every POST, both ride the same `inv_bfcache` token, both
exist so the browser can't optimize past a freshly-rotated server state.

The shim, in its replace branch, rewrites the redirect target as
`<target>?bf_inv=<token>` and uses `location.replace`. The search component
differs from the current URL → cross-document navigation guaranteed → real
GET. The new document carries an inline cleanup script that strips
`?bf_inv=` via `history.replaceState` before first paint, so the URL bar
ends on the canonical target form. Native fragment-scroll fires on the
fresh cross-doc load — `location.reload`'s scroll-restoration trap is
avoided entirely.

## Wire protocol

Unchanged:

| Direction | Header | Value | Meaning |
|---|---|---|---|
| Request | `X-Requested-With` | `stacknav` | POST is from the JS shim |
| Response | `X-Location` | URL path (may include `#fragment`) | Navigation target |
| Response | `X-Replace-Url` | `true` (optional) | Server-flagged replace |

`bf_inv` is a client-only implementation detail. It never appears on the
wire from server to client; the client adds it before `location.replace`
and strips it on parse.

## Client changes

### `ui/static/main.js` — `popOrPushTo` replace branch

Replace the `history.replaceState + location.reload` carve-out from
commits ceea54b and 024df74 with a universal rule:

```js
async function popOrPushTo(target, {replace = false} = {}) {
    const targetUrl = new URL(target, location.origin)
    const currentUrl = new URL(navigation.currentEntry.url)

    if (replace || sameUrl(currentUrl, targetUrl)) {
        // Force a cross-document fetch on every replace. The Navigation
        // API resolves some same-pathname navigations (identical URL, or
        // same path + different fragment) as same-document operations
        // that skip the GET, so a freshly-rotated server state never
        // reaches the user. Carry the canonical inv_bfcache token in a
        // bf_inv query param so the URL's search component differs from
        // current — the browser then commits to a real cross-doc fetch
        // and honors any fragment for native scroll-to-anchor. An inline
        // cleanup script in base.gohtml strips bf_inv before first paint.
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
```

The `cookieValue || Date.now()` fallback guards the (rare) case where the
`inv_bfcache` cookie is missing — uniqueness is the only property `bf_inv`
needs.

### `ui/templates/base.gohtml` — inline cleanup script

Add an inline nonce'd script in `<head>` immediately **before** the
existing `<script {{ $.Nonce }} src="/main.js"></script>` tag (line 36).
That placement gives the cleanup the earliest possible synchronous
execution: it runs while the HTML parser is still in `<head>`, before
`main.js` even starts loading, and well before any body content paints —
so the user never sees `?bf_inv=…` in the URL bar after first paint.

```gohtml
<script {{ $.Nonce }}>
    // Strip the bf_inv cache-bust param injected by popOrPushTo's replace
    // branch in main.js. The param exists only to force a cross-document
    // fetch; once parsing has started, the URL bar should carry the
    // canonical target form. Runs synchronously during HTML parse so the
    // user never sees ?bf_inv=... after first paint.
    (() => {
        const u = new URL(location.href);
        if (!u.searchParams.has('bf_inv')) return;
        u.searchParams.delete('bf_inv');
        history.replaceState(null, '', u.toString());
    })();
</script>
```

## Server changes

None. `redirect()` and `redirectReplace()` keep their current signatures and
all existing call sites are preserved. Handlers that emit
`/preferences#deload-title`, `/preferences#schedule-title`, etc. continue
to do so; the JS shim alone closes the gap.

## TLA+ alignment

No model file changes. Add a short paragraph to `tlaplus/README.md`:

> ### Implementation note: cache-bust on replace
>
> The JS shim's `popOrPushTo` rewrites every replace-branch target with a
> `bf_inv` query param before `location.replace`. The model treats `Urls`
> as a finite opaque set and assumes every replace-branch firing
> transitions `docState → Loading / inFlight → GetPending`. Without the
> cache-bust, the browser would resolve some same-pathname navigations
> (identical URL, or same path + different fragment) as same-document
> operations that skip the GET, leaving `docState = Loaded` with a stale
> `bakedToken` — a `NoStaleSettled` violation the model does NOT catch
> because URL distinctions below `Urls`-equality are abstracted away. The
> cache-bust enforces the model's assumption.

The existing `StackNav_PrefetchMitigated.cfg` continues to verify
`NoStaleSettled` across 825k states with no run-time change.

## Per-flow behavior

| # | Flow | popOrPushTo branch | bf_inv applied? | Back goes to |
|---|---|---|---|---|
| 1 | Set update / warmup complete (DETAIL → same DETAIL) | replace (auto-detect) | yes | Workout overview |
| 2 | Schedule submit success (`/schedule` → `/`) | pop or push | no | Per existing behavior |
| 3 | Schedule validation error (`/schedule` → `/schedule`) | replace (auto-detect) | yes | Parent of `/schedule` |
| 4 | Start workout (`/` → `/workouts/{date}`) | push | no | `/` |
| 5 | Complete workout (`/workouts/{date}` → `/complete`) | push | no | Workout overview |
| 6 | Workout feedback (`/complete` → `/`) | traverse to `/` | no | Exits app |
| 7 | Swap exercise (SWAP → DETAIL via same-slot URL) | traverse | no | Workout overview |
| 8 | Add exercise (`/add-exercise` → new DETAIL via `X-Replace-Url`) | replace (server-flagged) | yes | Workout overview |
| 9 | Add exercise validation (`/add-exercise` → `/add-exercise`) | replace (auto-detect) | yes | Workout overview |
| 10 | **Preferences deload save (`/preferences` or `/preferences#deload-title` → `/preferences#deload-title`)** | **replace (auto-detect)** | **yes** | **per existing behavior** |
| 11 | Preferences notif/start-deload/restart (same pattern as 10) | replace (auto-detect) | yes | per existing behavior |
| 12 | Preferences schedule validation error (`/preferences` → `/preferences#schedule-title`) | replace (auto-detect) | yes | per existing behavior |

Flows 10–12 are the ones this spec exists to fix. Flows 1, 3, 8, 9 are
"replace branch" flows that already worked through `navigation.navigate`;
they now go through `location.replace + bf_inv` instead. Observable
behavior is unchanged from the user's perspective — both replace the
current entry, both fetch — and the universal rule makes the model
alignment uniform.

## Testing

### Extend `Test_playwright_preferences_fragment_redirect`

The test already exercises a first and second submit with the leak-prevention
check on home. Update it to:

- Accept either `/preferences` or `/preferences?bf_inv=...` as the expected
  GET in the `ExpectResponse` matcher (regex `^.*/preferences(\?|$)`).
- After each submit, assert `page.URL()` ends in `/preferences#deload-title`
  with no residual `?bf_inv=` segment — verifying the cleanup script ran.
- After the first submit, assert the deload heading is in the viewport.
  `playwright-go` exposes this via the assertions API:

  ```go
  assertions := playwright.NewPlaywrightAssertions()
  if err := assertions.Locator(page.Locator("#deload-title")).
      ToBeInViewport(); err != nil {
      t.Errorf("deload heading not in viewport after recovery save: %v", err)
  }
  ```

  The viewport assertion is the regression-protection for the scroll bug
  the prior fixes introduced.

### Other playwright tests

- `Test_playwright_stacknav` — Flow 1 set-update and Flow 5 validation
  observably unchanged (both already in the replace branch). Re-run to
  confirm the URL-bar flash through `?bf_inv=` doesn't leak into any
  existing URL assertions.
- `Test_playwright_bfcache_staleness` — back-navigation path; cache-bust
  fires only on form-submit replace. Should be unaffected. Re-run to
  confirm.

### TLA+

No new model runs needed. The existing configs continue to verify
`NoStaleSettled`.

## Migration & rollout

One coherent change. The JS shim edit, the `base.gohtml` cleanup script,
the `tlaplus/README.md` note, the `cmd/web/CLAUDE.md` doc update, and the
test extension all land in a single commit. The previous two fix commits
(`ceea54b`, `024df74`) are superseded by this commit — the diff makes the
`history.replaceState + location.reload` carve-out go away. Mention this in
the commit message.

No staged rollout. Older clients on the prior `main.js` would still ship
the (broken-on-fragment-replace) flow until their next page load — but
that's the flow we've shipped for the past two weeks. No new regression.

## Documentation

`cmd/web/CLAUDE.md` "Redirects and Navigation" section gains one paragraph:

> Redirect paths may include a `#fragment` to land the user at a specific
> section after a form submit. The JS shim's `popOrPushTo` replace branch
> guarantees a real cross-document fetch (via a `bf_inv` cache-bust query
> param it injects, then strips after parse) so any flash banner the
> handler sets is rendered on the next GET. Handlers don't think about
> same-document semantics — just emit the redirect with the fragment they
> want, and the shim does the rest. See `docs/superpowers/specs/
> 2026-05-26-stack-navigator-replace-force-fresh-design.md`.

`ui/static/main.js` header doc-comment gains one paragraph on the
cache-bust under "Navigation strategy".

## Acceptance criteria

- Submitting any Recovery-panel form on `/preferences` (or repeated
  submits from `/preferences#deload-title`) renders the success banner
  inside the recovery panel and scrolls `#deload-title` into the viewport.
- The URL bar after submit reads `/preferences#deload-title` exactly — no
  `?bf_inv=` artifact.
- Navigating away from `/preferences` after a submit leaves the
  destination page banner-free (no flash leak).
- The two prior fix commits' `history.replaceState + location.reload`
  carve-out in `popOrPushTo` is removed.
- `Test_playwright_preferences_fragment_redirect` passes with the
  extended assertions.
- `make ci` passes.

## References

- Prior specs:
  - `docs/superpowers/specs/2026-04-25-stack-navigator-redesign-design.md`
  - `docs/superpowers/specs/2026-05-03-stack-navigator-push-default-design.md`
  - `docs/superpowers/specs/2026-05-25-preferences-scroll-and-flash-design.md`
  (its "Stack-navigator: no changes" claim is superseded by this spec.)
- TLA+ model: `tlaplus/StackNav.tla`, `tlaplus/README.md`.
- Prior fix commits: `ceea54b`, `024df74` (both superseded by the
  implementation of this spec).
- MDN: [Navigation API](https://developer.mozilla.org/en-US/docs/Web/API/Navigation_API),
  [history.scrollRestoration](https://developer.mozilla.org/en-US/docs/Web/API/History/scrollRestoration).
