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
(*      by newBaseTemplateData in cmd/web/templates.go).                  *)
(*                                                                         *)
(*   2. bfcache -- a parallel sequence; bfcache[i] holds the bakedToken   *)
(*      of a cached document for entry i, or NIL if not cached.           *)
(*                                                                         *)
(*   3. serverToken / cookieToken -- the canonical server-side cookie     *)
(*      value (rotated on every POST by setInvalidationCookieOnPost in    *)
(*      cmd/web/middleware.go) and the browser's view of it.              *)
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
    Urls,             \* Set of URLs the app exposes.
    PageLocalUrls,    \* URLs whose page bypasses the global bfcache reload
                      \* and runs its own pagereveal-based replace
                      \* (today: /workouts/{date}).
    InitialUrl,       \* The first URL the user lands on.
    MaxHistory,       \* Bound on history length, for model checking.
    MaxPosts,         \* Bound on number of POSTs.
    CookieMayExpire   \* TRUE iff the inv_bfcache cookie may expire.
                      \* Models the 60s MaxAge in middleware.go.

ASSUME PageLocalUrls \subseteq Urls
ASSUME InitialUrl \in Urls
ASSUME MaxHistory \in Nat /\ MaxHistory >= 1
ASSUME MaxPosts \in Nat
ASSUME CookieMayExpire \in BOOLEAN

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

DocStates == {"Loading", "Loaded", "Bfcached"}

InFlightStates == {"None", "GetPending"}

VARIABLES
    history,        \* Seq(Entry).
    idx,            \* Current entry index (1-based).
    docState,       \* Element of DocStates.
    bfcache,        \* Seq(BfcacheSlot) parallel to history.
    serverToken,    \* Latest server token (incremented on POST).
    cookieToken,    \* Browser's cookie value (NIL or some token).
    inFlight        \* Element of InFlightStates.

vars == <<history, idx, docState, bfcache, serverToken, cookieToken,
          inFlight>>

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
    /\ serverToken \in Tokens
    /\ cookieToken \in Tokens
    /\ inFlight \in InFlightStates

Init ==
    /\ history = <<[url |-> InitialUrl, bakedToken |-> NIL]>>
    /\ idx = 1
    /\ docState = "Loaded"
    /\ bfcache = <<NoCache>>
    /\ serverToken = NIL
    /\ cookieToken = NIL
    /\ inFlight = "None"

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
    /\ LET h2 == Append(Truncate(history, idx),
                        [url |-> targetUrl, bakedToken |-> NIL])
           b2 == Append(Truncate(StashBfcache(bfcache), idx), NoCache)
       IN  /\ history' = h2
           /\ bfcache' = b2
           /\ idx' = Len(h2)
           /\ docState' = "Loading"
           /\ inFlight' = "GetPending"
    /\ UNCHANGED <<serverToken, cookieToken>>

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
    /\ UNCHANGED <<idx, bfcache, serverToken, cookieToken>>

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
    /\ UNCHANGED <<serverToken, cookieToken>>

GoBack    == Traverse(idx - 1)
GoForward == Traverse(idx + 1)

(***************************************************************************)
(* Form submission. Server processes the POST atomically:                 *)
(*   - serverToken is rotated (middleware.setInvalidationCookieOnPost)    *)
(*   - the new value lands in the browser's inv_bfcache cookie            *)
(*   - the response carries X-Location (and optionally X-Replace-Url)     *)
(*                                                                         *)
(* The client then runs popOrPushTo(target, {replace}) (main.js:230).     *)
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

(***************************************************************************)
(* On bfcache restore, the global pageshow.persisted handler runs         *)
(* (main.js:327). If rendered token equals current cookie value, do       *)
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
    /\ UNCHANGED <<history, idx, bfcache, serverToken, cookieToken,
                   inFlight>>

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
    /\ UNCHANGED <<history, idx, bfcache, serverToken, cookieToken>>

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
    /\ UNCHANGED <<idx, serverToken, cookieToken>>

(***************************************************************************)
(* Cookie expiration. The inv_bfcache cookie has MaxAge 60s in production *)
(* (cmd/web/middleware.go:313). When CookieMayExpire is TRUE this action  *)
(* is enabled to expose the staleness window that opens once the cookie  *)
(* is gone.                                                                *)
(***************************************************************************)
CookieExpire ==
    /\ CookieMayExpire
    /\ cookieToken /= NIL
    /\ cookieToken' = NIL
    /\ UNCHANGED <<history, idx, docState, bfcache, serverToken, inFlight>>

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
    /\ UNCHANGED <<history, idx, docState, serverToken, cookieToken,
                   inFlight>>

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

\* Fairness on the pageshow / reload / response actions: once they are
\* enabled they must eventually fire. Without this, TLC could leave the
\* document stuck in "Bfcached" or "Loading" forever and trivially refute
\* every liveness claim.
Spec ==
    Init /\ [][Next]_vars
         /\ WF_vars(GetResponse)
         /\ WF_vars(PageshowFresh)
         /\ WF_vars(PageshowReload)
         /\ WF_vars(PageLocalReplace)

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
