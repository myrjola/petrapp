------------------------------- MODULE StackNav -------------------------------
(***************************************************************************)
(* High-level model of the stack-navigator + bfcache invalidation flow     *)
(* implemented in ui/static/main.js.                                       *)
(*                                                                         *)
(* The model captures four pieces of state and the events that mutate     *)
(* them:                                                                   *)
(*                                                                         *)
(*   1. Browser history -- a sequence of entries [url, bakedToken].       *)
(*      bakedToken is the value of the inv_bfcache cookie at the moment   *)
(*      the page was rendered (baked into <meta name="invalidation-token">*)
(*      by newBaseTemplateData in cmd/petra/templates.go).                *)
(*                                                                         *)
(*   2. bfcache -- a parallel sequence; bfcache[i] holds the bakedToken   *)
(*      of a cached document for entry i, or NIL if not cached.           *)
(*                                                                         *)
(*   3. serverToken / cookieToken -- the canonical server-side cookie     *)
(*      value (rotated on every POST by setInvalidationCookieOnPost in    *)
(*      cmd/petra/middleware.go) and the browser's view of it.            *)
(*                                                                         *)
(*   4. Document state -- one of "Loading", "Loaded", "Bfcached" --       *)
(*      tracks what the user sees right now.                              *)
(*                                                                         *)
(* The invariant of interest, NoStaleSettled, asserts that whenever the   *)
(* user is in a "settled" state (document fully loaded, no in-flight     *)
(* navigation), the displayed document's bakedToken equals the current    *)
(* serverToken -- i.e. the user cannot end up looking at stale data       *)
(* after a form submission.                                                *)
(***************************************************************************)

EXTENDS Naturals, Sequences, FiniteSets

CONSTANTS
    Urls,                 \* Set of URLs the app exposes.
    PageLocalUrls,        \* URLs whose page bypasses the global bfcache
                          \* reload and runs its own pagereveal-based
                          \* replace (today: /workouts/{date}).
    InitialUrl,           \* The first URL the user lands on.
    MaxHistory,           \* Bound on history length, for model checking.
    MaxPosts,             \* Bound on number of POSTs.
    CookieMayExpire,      \* TRUE iff the inv_bfcache cookie may expire.
                          \* Models the 60s MaxAge in middleware.go.
    PrefetchEnabled,      \* TRUE iff the browser-prefetch action is
                          \* enabled. Models the Speculation Rules block
                          \* in cmd/petra/ui/templates/base.gohtml that
                          \* prefetches every /* link at moderate eagerness.
    EagerPagerevealCheck, \* TRUE iff the staleness check runs on every
                          \* page reveal (not just pageshow.persisted).
                          \* This mitigation SHIPPED: the global handler in
                          \* main.js:633 checks on every pagereveal. The
                          \* FALSE setting models the pre-fix main.js that
                          \* only checked on bfcache restore, so prefetch-
                          \* promoted loads slipped through.
    EnableForwardMisroute, \* TRUE iff PagerevealMisrouteOnForward is
                           \* enabled. Models a HYPOTHESIS that was
                           \* explored and DISPROVEN by the production
                           \* diagnostic run: that calling
                           \* navigation.reload() synchronously inside a
                           \* pagereveal handler -- while a forward
                           \* traverse is still committing -- causes
                           \* Chromium to push a new history entry at
                           \* history[idx-1].url instead of reloading at
                           \* idx. The diagnostic [navbug] log showed
                           \* the reload's destination is the correct
                           \* exercise URL, not workout. Action retained
                           \* in the model as a documented dead-end --
                           \* all production cfgs gate this FALSE; the
                           \* StackNav_ForwardMisrouteHypothesis.cfg
                           \* exercises it for didactic comparison.
    EnableBackCatchBug     \* TRUE iff BackCatchFiresOnReveal is enabled.
                           \* Models the actual production navbug: an
                           \* in-page back-button click handler in
                           \* main.js registers
                           \* `navigation.traverseTo(...).committed
                           \*    .catch(() => location.assign(link.href))`.
                           \* The .committed promise is left pending
                           \* indefinitely when the issuing doc goes to
                           \* bfcache. Forward-restoring the issuing doc
                           \* and initiating ANY new navigation from it
                           \* (e.g. the pagereveal staleness reload)
                           \* aborts the pending committed -> .catch
                           \* fires -> location.assign pushes a new
                           \* entry at the closure-captured back-link
                           \* URL. Confirmed by the [navbug] diagnostic
                           \* log; fixed in main.js commit 6a4c239 by
                           \* dropping the .catch entirely.

ASSUME PageLocalUrls \subseteq Urls
ASSUME InitialUrl \in Urls
ASSUME MaxHistory \in Nat /\ MaxHistory >= 1
ASSUME MaxPosts \in Nat
ASSUME CookieMayExpire \in BOOLEAN
ASSUME PrefetchEnabled \in BOOLEAN
ASSUME EagerPagerevealCheck \in BOOLEAN
ASSUME EnableForwardMisroute \in BOOLEAN
ASSUME EnableBackCatchBug \in BOOLEAN

\* NIL is the sentinel for "no token". In JS the corresponding values are
\* the empty string -- both the cookie (when absent) and the meta content
\* (when rendered without a cookie) read as "". `'' === ''` so they match,
\* which models the "no POST yet" pre-cookie state where the check is a
\* no-op.
NIL == 0

Tokens == 0..MaxPosts

Entry == [url: Urls, bakedToken: Tokens]

\* bfcache slot. `cached` distinguishes "no document snapshot" from
\* "snapshot with token NIL" -- conflating the two would silently model
\* a page rendered before any POST as never bfcached, which would hide
\* the cookie-expiration staleness bug entirely.
BfcacheSlot == [cached: BOOLEAN, token: Tokens]

NoCache == [cached |-> FALSE, token |-> NIL]
Cache(tok) == [cached |-> TRUE, token |-> tok]

\* Prefetch slot, one per URL. The Speculation Rules block in
\* base.gohtml may issue a prefetch GET for any /* URL, and the response
\* (with its baked invalidation-token meta) sits in a per-URL cache
\* until either evicted or promoted to a real navigation.
PrefetchSlot == [present: BOOLEAN, token: Tokens]

NoPrefetch == [present |-> FALSE, token |-> NIL]
Prefetched(tok) == [present |-> TRUE, token |-> tok]

\* Captured in-page back-button click closure (main.js,
\* navigation.traverseTo(...).committed.catch(() => location.assign(link.href))).
\* `pending` flags whether a catch is registered and awaiting rejection.
\* `srcIdx` is the history idx of the doc that registered the closure --
\* the catch can only fire when we are back in that doc (because the
\* JS execution context lives in that doc). `captureUrl` is the
\* link.href captured by the closure body (the back-link target at
\* click time). 0 / InitialUrl are sentinels used only when ~pending.
PendingCatchSlot == [pending: BOOLEAN, srcIdx: 0..MaxHistory, captureUrl: Urls]

NoPendingCatch == [pending |-> FALSE, srcIdx |-> 0, captureUrl |-> InitialUrl]

DocStates == {"Loading", "Loaded", "Bfcached", "PromotedPrefetch"}

InFlightStates == {"None", "GetPending"}

VARIABLES
    history,        \* Seq(Entry).
    idx,            \* Current entry index (1-based).
    docState,       \* Element of DocStates.
    bfcache,        \* Seq(BfcacheSlot) parallel to history.
    prefetchCache,  \* Function from Urls to PrefetchSlot.
    serverToken,    \* Latest server token (incremented on POST).
    cookieToken,    \* Browser's cookie value (NIL or some token).
    inFlight,       \* Element of InFlightStates.
    lastForwardTargetIdx,
                    \* Aux. The idx that the most recent GoForward
                    \* landed on. Reset to 0 on any new user action
                    \* (ClickLink, GoBack, SubmitForm) that isn't a
                    \* forward traverse. Preserved across system
                    \* resolution actions (GetResponse, Pageshow*,
                    \* PromotedCheck*, Evict, etc.) so the
                    \* ForwardLandsAtTarget invariant can compare the
                    \* eventual settled idx against where GoForward
                    \* claimed it would land.
    pendingCatch
                    \* Captures the in-page back-button click handler's
                    \* `navigation.traverseTo(key).committed.catch(...)`
                    \* registration in main.js (pre-fix). Set by
                    \* BackButtonClick with the click's source idx and
                    \* the link.href captured by the closure body.
                    \* Persists across bfcache because the source doc's
                    \* execution context is merely suspended (.committed
                    \* not resolved). Consumed by BackCatchFiresOnReveal
                    \* when the doc is restored AND a new navigation is
                    \* initiated from it (modeled by the same
                    \* enablement conditions as PageshowReload).

vars == <<history, idx, docState, bfcache, prefetchCache, serverToken,
          cookieToken, inFlight, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Helpers                                                                 *)
(***************************************************************************)

CurrentEntry == history[idx]
CurrentUrl == CurrentEntry.url
CurrentBaked == CurrentEntry.bakedToken

\* Stash the currently displayed document into bfcache before leaving it.
\* The bfcache slot for idx now holds the bakedToken that the displayed
\* document was rendered with.
StashBfcache(b) == [b EXCEPT ![idx] = Cache(CurrentBaked)]

\* Truncate a sequence at length n. Used when a new push wipes the forward
\* stack.
Truncate(s, n) == SubSeq(s, 1, n)

\* Find the most recent (closest to fromIdx) backward index whose URL
\* matches `url`. Returns 0 if no match. Mirrors the JS popOrPushTo loop
\* in ui/static/main.js which walks from currentEntry.index - 1 down to 0
\* and returns on the first hit.
FindBackwardMatch(h, fromIdx, url) ==
    IF \E i \in 1..(fromIdx - 1) : h[i].url = url
    THEN CHOOSE i \in 1..(fromIdx - 1) :
            /\ h[i].url = url
            /\ \A j \in (i+1)..(fromIdx - 1) : h[j].url /= url
    ELSE 0

(***************************************************************************)
(* Type invariant and initial state                                        *)
(***************************************************************************)

TypeOk ==
    /\ history \in Seq(Entry)
    /\ Len(history) >= 1
    /\ Len(history) <= MaxHistory
    /\ idx \in 1..Len(history)
    /\ docState \in DocStates
    /\ bfcache \in Seq(BfcacheSlot)
    /\ Len(bfcache) = Len(history)
    /\ prefetchCache \in [Urls -> PrefetchSlot]
    /\ serverToken \in Tokens
    /\ cookieToken \in Tokens
    /\ inFlight \in InFlightStates
    /\ lastForwardTargetIdx \in 0..MaxHistory
    /\ pendingCatch \in PendingCatchSlot

Init ==
    /\ history = <<[url |-> InitialUrl, bakedToken |-> NIL]>>
    /\ idx = 1
    /\ docState = "Loaded"
    /\ bfcache = <<NoCache>>
    /\ prefetchCache = [u \in Urls |-> NoPrefetch]
    /\ serverToken = NIL
    /\ cookieToken = NIL
    /\ inFlight = "None"
    /\ lastForwardTargetIdx = 0
    /\ pendingCatch = NoPendingCatch

(***************************************************************************)
(* User-initiated GET navigation (link click)                              *)
(*                                                                         *)
(* Source: <a> click handled natively by the browser, plus the speculation*)
(* rules in base.gohtml. Pushes a new entry on top of the current entry; *)
(* the forward stack is wiped per the HTML history spec.                  *)
(***************************************************************************)
ClickLink(targetUrl) ==
    /\ docState = "Loaded"
    /\ inFlight = "None"
    /\ idx < MaxHistory  \* respect history bound
    /\ LET hit == prefetchCache[targetUrl].present
           newEntry ==
               IF hit
               THEN [url        |-> targetUrl,
                     bakedToken |-> prefetchCache[targetUrl].token]
               ELSE [url |-> targetUrl, bakedToken |-> NIL]
           h2 == Append(Truncate(history, idx), newEntry)
           b2 == Append(Truncate(StashBfcache(bfcache), idx), NoCache)
       IN  /\ history' = h2
           /\ bfcache' = b2
           /\ idx' = Len(h2)
           /\ IF hit
              THEN \* Prefetched response is promoted into the current
                   \* navigation: the document is constructed immediately,
                   \* no network round trip. The meta token is whatever
                   \* it was at prefetch time, which may be stale.
                /\ docState' = "PromotedPrefetch"
                /\ inFlight' = "None"
                /\ prefetchCache' = [prefetchCache EXCEPT
                                       ![targetUrl] = NoPrefetch]
              ELSE \* No prefetch: fresh fetch.
                /\ docState' = "Loading"
                /\ inFlight' = "GetPending"
                /\ prefetchCache' = prefetchCache
    /\ lastForwardTargetIdx' = 0
    /\ UNCHANGED <<serverToken, cookieToken, pendingCatch>>

(***************************************************************************)
(* The pending GET response arrives. The new document is baked with the   *)
(* current server token (newBaseTemplateData reads the inv_bfcache cookie *)
(* off the request).                                                       *)
(*                                                                         *)
(* Models both the GET that follows ClickLink/Back/Forward/Push and the   *)
(* GET that follows a reload triggered by the pageshow handler.           *)
(***************************************************************************)
GetResponse ==
    /\ inFlight = "GetPending"
    /\ docState = "Loading"
    /\ history' = [history EXCEPT
                     ![idx] = [url        |-> history[idx].url,
                               bakedToken |-> serverToken]]
    /\ docState' = "Loaded"
    /\ inFlight' = "None"
    /\ UNCHANGED <<idx, bfcache, prefetchCache, serverToken, cookieToken,
                   lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Browser back / forward button. Per the HTML spec, the previous         *)
(* document is restored from bfcache if present; otherwise it is fetched  *)
(* fresh.                                                                  *)
(***************************************************************************)
Traverse(newIdx) ==
    /\ docState = "Loaded"
    /\ inFlight = "None"
    /\ newIdx \in 1..Len(history)
    /\ newIdx /= idx
    /\ bfcache' = StashBfcache(bfcache)
    /\ idx' = newIdx
    /\ IF bfcache[newIdx].cached
       THEN \* bfcache hit: restore cached document with its original
            \* bakedToken. The document is now visible; the pageshow
            \* handler will run via one of the Pageshow* actions below.
            /\ history' = [history EXCEPT
                             ![newIdx] = [url        |-> history[newIdx].url,
                                          bakedToken |-> bfcache[newIdx].token]]
            /\ docState' = "Bfcached"
            /\ inFlight' = "None"
       ELSE \* No bfcache: fresh fetch.
            /\ history' = [history EXCEPT
                             ![newIdx] = [url        |-> history[newIdx].url,
                                          bakedToken |-> NIL]]
            /\ docState' = "Loading"
            /\ inFlight' = "GetPending"
    \* Record forward-traverse target so ForwardLandsAtTarget can
    \* assert the eventual settled idx equals what GoForward landed on.
    \* Backward traversal resets the tracker (we only care about the
    \* forward-then-bfcache-then-misroute pattern from the bug).
    /\ lastForwardTargetIdx' = IF newIdx > idx THEN newIdx ELSE 0
    \* Traverse models the browser back/forward button -- no JS click
    \* handler runs, so no pending catch is registered or consumed here.
    \* The in-page back-link click that DOES register a catch is
    \* modeled by BackButtonClick below.
    /\ UNCHANGED <<prefetchCache, serverToken, cookieToken, pendingCatch>>

GoBack    == Traverse(idx - 1)
GoForward == Traverse(idx + 1)

(***************************************************************************)
(* Form submission. Server processes the POST atomically:                 *)
(*   - serverToken is rotated (middleware.setInvalidationCookieOnPost)    *)
(*   - the new value lands in the browser's inv_bfcache cookie            *)
(*   - the response carries X-Location (and optionally X-Replace-Url)     *)
(*                                                                         *)
(* The client then runs popOrPushTo(target, {replace}) (main.js:238).     *)
(* Three branches:                                                         *)
(*   A. Replace mode: explicit X-Replace-Url OR target == currentUrl      *)
(*      (the client's auto-detect for same-URL submits).                  *)
(*   B. Traverse: target equals a backward entry's URL.                   *)
(*   C. Push: target is brand new in history.                             *)
(***************************************************************************)
SubmitForm(targetUrl, isReplace) ==
    /\ docState = "Loaded"
    /\ inFlight = "None"
    /\ serverToken < MaxPosts
    /\ LET newToken == serverToken + 1
           replaceMode == isReplace \/ (CurrentUrl = targetUrl)
           backMatch == FindBackwardMatch(history, idx, targetUrl)
       IN
        /\ serverToken' = newToken
        /\ cookieToken' = newToken
        /\ IF replaceMode
           THEN \* A. Replace current entry. The old document and its
                \* bfcache slot are erased -- the entry is destroyed and
                \* re-created at the same index.
              /\ history' = [history EXCEPT
                              ![idx] = [url        |-> targetUrl,
                                        bakedToken |-> NIL]]
              /\ bfcache' = [bfcache EXCEPT ![idx] = NoCache]
              /\ idx' = idx
              /\ docState' = "Loading"
              /\ inFlight' = "GetPending"
           ELSE IF backMatch /= 0
           THEN \* B. Traverse to the matching backward entry.
              /\ idx' = backMatch
              /\ bfcache' = StashBfcache(bfcache)
              /\ IF bfcache[backMatch].cached
                 THEN /\ history' = [history EXCEPT
                                       ![backMatch] = [url        |-> history[backMatch].url,
                                                       bakedToken |-> bfcache[backMatch].token]]
                      /\ docState' = "Bfcached"
                      /\ inFlight' = "None"
                 ELSE /\ history' = [history EXCEPT
                                       ![backMatch] = [url        |-> history[backMatch].url,
                                                       bakedToken |-> NIL]]
                      /\ docState' = "Loading"
                      /\ inFlight' = "GetPending"
           ELSE \* C. Push a brand new entry on top of the current one,
                \* wiping the forward stack.
              /\ idx < MaxHistory
              /\ LET h2 == Append(Truncate(history, idx),
                                  [url |-> targetUrl, bakedToken |-> NIL])
                     b2 == Append(Truncate(StashBfcache(bfcache), idx),
                                  NoCache)
                 IN  /\ history' = h2
                     /\ bfcache' = b2
                     /\ idx' = Len(h2)
                     /\ docState' = "Loading"
                     /\ inFlight' = "GetPending"
    \* Programmatic navigation.navigate() in popOrPushTo does not
    \* consume the speculation-rules prefetch cache: that cache is for
    \* browser-initiated link navigations. The post-cookie-change Vary
    \* on Cookie would, in any case, invalidate the prefetch for the
    \* target URL. We conservatively leave the cache untouched here.
    /\ UNCHANGED <<prefetchCache, pendingCatch>>
    /\ lastForwardTargetIdx' = 0

(***************************************************************************)
(* On bfcache restore, the global staleness handler runs (main.js:633,    *)
(* on pagereveal). If rendered token equals current cookie value, do      *)
(* nothing; the document is presumed fresh.                                *)
(*                                                                         *)
(* NOTE: in the JS code, the comparison is between two strings -- both    *)
(* default to "" when missing. We model that as NIL = 0, so NIL = NIL    *)
(* matches.                                                                *)
(***************************************************************************)
PageshowFresh ==
    /\ docState = "Bfcached"
    /\ CurrentBaked = cookieToken
    /\ docState' = "Loaded"
    /\ UNCHANGED <<history, idx, bfcache, prefetchCache, serverToken,
                   cookieToken, inFlight, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Tokens differ and the page has the default (global) bfcache handler.   *)
(* The handler calls navigation.reload() -- modeled as transitioning to   *)
(* a "Loading" state that GetResponse will resolve to a fresh document.   *)
(***************************************************************************)
PageshowReload ==
    /\ docState = "Bfcached"
    /\ CurrentBaked /= cookieToken
    /\ CurrentUrl \notin PageLocalUrls
    /\ docState' = "Loading"
    /\ inFlight' = "GetPending"
    /\ UNCHANGED <<history, idx, bfcache, prefetchCache, serverToken,
                   cookieToken, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Page-local handler (workout.gohtml: dataset.bfcacheHandler = 'page-     *)
(* local'). On staleness, the page does a replace navigation to           *)
(* `pathname?_inv=1` to bust bfcache, then strips the query on arrival    *)
(* via history.replaceState. We model the end-state: the entry's URL is  *)
(* its original URL, the document is fresh.                                *)
(***************************************************************************)
PageLocalReplace ==
    /\ docState = "Bfcached"
    /\ CurrentBaked /= cookieToken
    /\ CurrentUrl \in PageLocalUrls
    /\ history' = [history EXCEPT
                     ![idx] = [url        |-> CurrentUrl,
                               bakedToken |-> NIL]]
    /\ bfcache' = [bfcache EXCEPT ![idx] = NoCache]
    /\ docState' = "Loading"
    /\ inFlight' = "GetPending"
    /\ UNCHANGED <<idx, prefetchCache, serverToken, cookieToken,
                   lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Cookie expiration. The inv_bfcache cookie USED to carry a 60s MaxAge,  *)
(* which opened the staleness window this action models. The fix shipped  *)
(* a session cookie (no MaxAge / no Expires; cmd/petra/middleware.go:329),*)
(* so in production CookieMayExpire is FALSE and this action never fires. *)
(* It stays enabled only in StackNav_CookieExpire.cfg to keep the         *)
(* counterexample reproducible.                                           *)
(***************************************************************************)
CookieExpire ==
    /\ CookieMayExpire
    /\ cookieToken /= NIL
    /\ cookieToken' = NIL
    /\ UNCHANGED <<history, idx, docState, bfcache, prefetchCache,
                   serverToken, inFlight, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Browser may evict a non-current bfcache entry at any time. Per the     *)
(* HTML spec this is implementation-defined; we model it as fully         *)
(* non-deterministic.                                                      *)
(***************************************************************************)
BfcacheEvict ==
    /\ \E i \in 1..Len(bfcache) :
         /\ i /= idx
         /\ bfcache[i].cached
         /\ bfcache' = [bfcache EXCEPT ![i] = NoCache]
    /\ UNCHANGED <<history, idx, docState, prefetchCache, serverToken,
                   cookieToken, inFlight, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Speculation Rules prefetch. The browser fires a GET for `url` at       *)
(* `eagerness: moderate` (hover/pointerdown). The response,               *)
(* baked with whatever inv_bfcache cookie the request carried, is parked  *)
(* in the prefetch cache.                                                  *)
(***************************************************************************)
Prefetch(url) ==
    /\ PrefetchEnabled
    /\ ~prefetchCache[url].present
    /\ prefetchCache' = [prefetchCache EXCEPT
                           ![url] = Prefetched(serverToken)]
    /\ UNCHANGED <<history, idx, docState, bfcache, serverToken,
                   cookieToken, inFlight, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Browser may evict a prefetched response (memory pressure, age, the     *)
(* Vary: Cookie + must-revalidate combination invalidating it after a    *)
(* POST changes the cookie). Modeled as non-deterministic.                *)
(***************************************************************************)
PrefetchEvict(url) ==
    /\ prefetchCache[url].present
    /\ prefetchCache' = [prefetchCache EXCEPT ![url] = NoPrefetch]
    /\ UNCHANGED <<history, idx, docState, bfcache, serverToken,
                   cookieToken, inFlight, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* After a prefetched response is promoted, the page is fully loaded but *)
(* its meta token may not match the current cookie. Today's main.js gates*)
(* the staleness check on pageshow.persisted (true only for bfcache      *)
(* restores), so the prefetch case is silently accepted.                  *)
(*                                                                         *)
(* PromotedSettleNoCheck models that buggy path: the document is         *)
(* declared "Loaded" without checking the meta token.                     *)
(***************************************************************************)
PromotedSettleNoCheck ==
    /\ docState = "PromotedPrefetch"
    /\ ~EagerPagerevealCheck
    /\ CurrentUrl \notin PageLocalUrls
    /\ docState' = "Loaded"
    /\ UNCHANGED <<history, idx, bfcache, prefetchCache, serverToken,
                   cookieToken, inFlight, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* With EagerPagerevealCheck (shipped mitigation: move the check from    *)
(* `pageshow.persisted` to `pagereveal`, which fires on every navigation),*)
(* the staleness comparison runs after every load. If meta == cookie the *)
(* page is presumed fresh and settles.                                    *)
(***************************************************************************)
PromotedCheckFresh ==
    /\ docState = "PromotedPrefetch"
    /\ EagerPagerevealCheck \/ CurrentUrl \in PageLocalUrls
    /\ CurrentBaked = cookieToken
    /\ docState' = "Loaded"
    /\ UNCHANGED <<history, idx, bfcache, prefetchCache, serverToken,
                   cookieToken, inFlight, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Mismatch on a non-page-local URL with the mitigation enabled: reload  *)
(* exactly as the bfcache handler would.                                  *)
(***************************************************************************)
PromotedCheckReload ==
    /\ docState = "PromotedPrefetch"
    /\ EagerPagerevealCheck
    /\ CurrentBaked /= cookieToken
    /\ CurrentUrl \notin PageLocalUrls
    /\ docState' = "Loading"
    /\ inFlight' = "GetPending"
    /\ UNCHANGED <<history, idx, bfcache, prefetchCache, serverToken,
                   cookieToken, lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Page-local handler (workout.gohtml) already fires on every pagereveal,*)
(* so the prefetch-promoted case is caught regardless of mitigation flag.*)
(***************************************************************************)
PromotedPageLocalReplace ==
    /\ docState = "PromotedPrefetch"
    /\ CurrentBaked /= cookieToken
    /\ CurrentUrl \in PageLocalUrls
    /\ history' = [history EXCEPT
                     ![idx] = [url        |-> CurrentUrl,
                               bakedToken |-> NIL]]
    /\ bfcache' = [bfcache EXCEPT ![idx] = NoCache]
    /\ docState' = "Loading"
    /\ inFlight' = "GetPending"
    /\ UNCHANGED <<idx, prefetchCache, serverToken, cookieToken,
                   lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Bug hypothesis (gated by EnableForwardMisroute):                        *)
(*                                                                         *)
(* On a forward traverse to a bfcached, stale page, the new global         *)
(* pagereveal handler in main.js:633 calls navigation.reload()             *)
(* synchronously inside the pagereveal event -- i.e. while the traverse    *)
(* is still committing. Chromium (per the workout.gohtml inline comment    *)
(* at line 451-453) is known to "reclassify" same-URL navigations fired   *)
(* from a deferred context. We hypothesize that in this race, the reload  *)
(* is resolved against the about-to-be-discarded source-of-forward URL    *)
(* (history[idx-1].url) rather than the just-landed-on idx URL, AND it    *)
(* materializes as a PUSH of a new entry rather than a reload of the      *)
(* current entry. The user observation: history grows from               *)
(*    [workout, exercise]                                                  *)
(* to                                                                      *)
(*    [workout, exercise, workout]                                         *)
(* after a forward press lands on a stale bfcached exercise page. We     *)
(* model exactly that transition here.                                     *)
(*                                                                         *)
(* The action competes with PageshowReload (the "well-behaved" path); TLC *)
(* will explore both branches. If misroute is reachable and the           *)
(* ForwardLandsAtTarget invariant fails, we've reproduced the bug in the *)
(* model.                                                                  *)
(***************************************************************************)
(***************************************************************************)
(* In-page back-button click (a click on an `<a data-back-button>`)       *)
(* under the PRE-FIX main.js code.                                         *)
(*                                                                         *)
(* Pre-fix main.js intercepted the click, walked history backward for a    *)
(* matching URL, and issued                                                *)
(*   navigation.traverseTo(key).committed.catch(() =>                     *)
(*     location.assign(link.href))                                        *)
(*                                                                         *)
(* The traverse itself behaves like a backward Traverse(idx - 1). The      *)
(* extra effect is the registration of a .committed.catch closure         *)
(* capturing link.href -- this is recorded in pendingCatch. The closure   *)
(* lives in the source doc's execution context, so it can only fire when  *)
(* we are back in that doc.                                                *)
(*                                                                         *)
(* GATED on EnableBackCatchBug so existing cfgs (which model the post-fix *)
(* or .catch-irrelevant world) get the same state-space coverage as       *)
(* before this commit. In the FIX (EnableBackCatchBug = FALSE), an        *)
(* in-page back-button click is behaviorally identical to a browser back  *)
(* button (no .catch registered), so GoBack already covers it.            *)
(*                                                                         *)
(* The model assumes the back-link target is the most-recent backward     *)
(* entry (idx - 1). In production main.js it could be any earlier match;  *)
(* the bug shape is invariant under that detail.                           *)
(***************************************************************************)
BackButtonClick ==
    /\ EnableBackCatchBug
    /\ docState = "Loaded"
    /\ inFlight = "None"
    /\ idx > 1
    /\ LET capturedUrl == history[idx - 1].url
       IN
        /\ bfcache' = StashBfcache(bfcache)
        /\ idx' = idx - 1
        /\ IF bfcache[idx - 1].cached
           THEN /\ history' = [history EXCEPT
                                ![idx - 1] = [url        |-> history[idx - 1].url,
                                              bakedToken |-> bfcache[idx - 1].token]]
                /\ docState' = "Bfcached"
                /\ inFlight' = "None"
           ELSE /\ history' = [history EXCEPT
                                ![idx - 1] = [url        |-> history[idx - 1].url,
                                              bakedToken |-> NIL]]
                /\ docState' = "Loading"
                /\ inFlight' = "GetPending"
        /\ pendingCatch' = [pending     |-> TRUE,
                            srcIdx      |-> idx,       \* the doc that clicked
                            captureUrl  |-> capturedUrl]
    /\ lastForwardTargetIdx' = 0
    /\ UNCHANGED <<prefetchCache, serverToken, cookieToken>>

(***************************************************************************)
(* THE PRODUCTION NAVBUG (gated by EnableBackCatchBug).                   *)
(*                                                                         *)
(* On a stale-bfcache restore of the same doc that previously issued an   *)
(* in-page back-button click, the new pagereveal handler in main.js:633   *)
(* fires navigation.reload(). The reload supersedes the long-pending     *)
(* .committed promise from the back-button click closure; the promise    *)
(* rejects with AbortError; the .catch fires location.assign(link.href). *)
(* The reload itself is in turn superseded by location.assign, so the    *)
(* end state is a NEW history entry pushed at the captured URL.          *)
(*                                                                         *)
(* Enablement intentionally mirrors PageshowReload (the "well-behaved"   *)
(* path); TLC explores both branches. The competing actions are mutually *)
(* exclusive per pagereveal event in production, but TLC may interleave  *)
(* them across reachable states -- the invariant `ForwardLandsAtTarget`  *)
(* catches the violating one regardless.                                  *)
(*                                                                         *)
(* Confirmed from the production [navbug] diagnostic log: the .catch     *)
(* fires with currentUrl=exercise, href=workout, AbortError. Fixed in    *)
(* main.js commit 6a4c239 by dropping the .catch (model: set            *)
(* EnableBackCatchBug = FALSE).                                          *)
(***************************************************************************)
BackCatchFiresOnReveal ==
    /\ EnableBackCatchBug
    /\ docState = "Bfcached"
    /\ CurrentBaked /= cookieToken
    /\ CurrentUrl \notin PageLocalUrls
    /\ pendingCatch.pending
    /\ pendingCatch.srcIdx = idx
    /\ idx < MaxHistory
    /\ LET captured == pendingCatch.captureUrl
           newEntry == [url |-> captured, bakedToken |-> NIL]
           h2 == Append(Truncate(history, idx), newEntry)
           b2 == Append(Truncate(StashBfcache(bfcache), idx), NoCache)
       IN  /\ history' = h2
           /\ bfcache' = b2
           /\ idx' = Len(h2)
           /\ docState' = "Loading"
           /\ inFlight' = "GetPending"
    \* Consume the pending catch -- the closure has fired once.
    /\ pendingCatch' = NoPendingCatch
    \* lastForwardTargetIdx intentionally NOT cleared: the violation is
    \* that we forwarded onto srcIdx (which set lastForwardTargetIdx =
    \* srcIdx == idx) and then ended up on srcIdx + 1, idx /=
    \* lastForwardTargetIdx, ForwardLandsAtTarget fires.
    /\ UNCHANGED <<prefetchCache, serverToken, cookieToken,
                   lastForwardTargetIdx>>

PagerevealMisrouteOnForward ==
    /\ EnableForwardMisroute
    /\ docState = "Bfcached"
    /\ lastForwardTargetIdx = idx
    /\ idx > 1
    /\ idx < MaxHistory
    /\ CurrentBaked /= cookieToken
    /\ CurrentUrl \notin PageLocalUrls
    /\ LET targetUrl == history[idx - 1].url
           newEntry == [url |-> targetUrl, bakedToken |-> NIL]
           h2 == Append(Truncate(history, idx), newEntry)
           b2 == Append(Truncate(StashBfcache(bfcache), idx), NoCache)
       IN  /\ history' = h2
           /\ bfcache' = b2
           /\ idx' = Len(h2)
           /\ docState' = "Loading"
           /\ inFlight' = "GetPending"
    \* lastForwardTargetIdx is intentionally NOT cleared here: the
    \* invariant `idx = lastForwardTargetIdx` then fires immediately
    \* after the misroute fires (idx /= original target), pinning the
    \* TLC counterexample to the misroute step rather than a later
    \* settling step.
    /\ UNCHANGED <<prefetchCache, serverToken, cookieToken,
                   lastForwardTargetIdx, pendingCatch>>

(***************************************************************************)
(* Next-state relation                                                     *)
(***************************************************************************)

Next ==
    \/ \E u \in Urls : ClickLink(u)
    \/ GetResponse
    \/ GoBack
    \/ GoForward
    \/ \E t \in Urls, r \in BOOLEAN : SubmitForm(t, r)
    \/ PageshowFresh
    \/ PageshowReload
    \/ PageLocalReplace
    \/ CookieExpire
    \/ BfcacheEvict
    \/ \E u \in Urls : Prefetch(u)
    \/ \E u \in Urls : PrefetchEvict(u)
    \/ PromotedSettleNoCheck
    \/ PromotedCheckFresh
    \/ PromotedCheckReload
    \/ PromotedPageLocalReplace
    \/ PagerevealMisrouteOnForward
    \/ BackButtonClick
    \/ BackCatchFiresOnReveal

\* Fairness on the pageshow / reload / response / settle actions: once
\* they are enabled they must eventually fire. Without this, TLC could
\* leave the document stuck in a transient state forever and trivially
\* refute every liveness claim.
Spec ==
    Init /\ [][Next]_vars
         /\ WF_vars(GetResponse)
         /\ WF_vars(PageshowFresh)
         /\ WF_vars(PageshowReload)
         /\ WF_vars(PageLocalReplace)
         /\ WF_vars(PromotedSettleNoCheck)
         /\ WF_vars(PromotedCheckFresh)
         /\ WF_vars(PromotedCheckReload)
         /\ WF_vars(PromotedPageLocalReplace)

(***************************************************************************)
(* Properties                                                              *)
(***************************************************************************)

\* Safety: in any settled state, the displayed document is fresh.
\* "Settled" = document fully loaded, no in-flight fetch. Transient
\* "Bfcached" and "Loading" states are explicitly excluded -- they are
\* the windows during which the stack navigator is working to restore
\* freshness.
NoStaleSettled ==
    (docState = "Loaded" /\ inFlight = "None")
        => CurrentBaked = serverToken

\* Safety: a forward traverse must settle the user on the entry it
\* landed on, not somewhere else. GoForward sets lastForwardTargetIdx
\* to its destination idx. ClickLink / GoBack / SubmitForm reset it to
\* 0 (the user has taken a new action that supersedes the forward
\* expectation). Resolution actions (GetResponse, Pageshow*, Promoted*)
\* preserve it. PagerevealMisrouteOnForward also preserves it -- so
\* immediately after the bug action fires, idx has shifted while the
\* expected target still points at the old idx, and this invariant
\* fires, pinning the TLC counterexample to the misroute step.
ForwardLandsAtTarget ==
    lastForwardTargetIdx = 0 \/ idx = lastForwardTargetIdx

\* Auxiliary structural invariant.

\* Auxiliary structural invariant.
HistoryAligned ==
    /\ Len(history) = Len(bfcache)
    /\ idx \in 1..Len(history)

\* Liveness: any transient state ("Loading" or "Bfcached") eventually
\* leads to a settled, fresh state. Stronger than NoStaleSettled because
\* it rules out getting stuck mid-navigation, but weaker than "always
\* eventually settled" -- the user is free to start a new navigation
\* before settlement happens; we only require that IF the document is
\* not settled, THEN it eventually does reach a settled fresh state on
\* some future step. Requires WF_vars on the internal resolution actions.
TransientResolves ==
    [](docState \in {"Loading", "Bfcached"}
        ~> (docState = "Loaded" /\ inFlight = "None"
            /\ CurrentBaked = serverToken))

\* State constraint to bound TLC's search.
StateConstraint == Len(history) <= MaxHistory

=============================================================================
