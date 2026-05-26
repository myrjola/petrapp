# Stack Navigator Force-Fresh Replace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the `history.replaceState + location.reload` carve-out in `popOrPushTo`'s replace branch with a `bf_inv` cache-bust query param + `location.replace`, paired with an inline cleanup script that strips the param before first paint, so fragment-bearing replace targets fetch fresh AND scroll to the anchor.

**Architecture:** Single uniform rule in the JS shim's replace branch — always cache-bust with the rotated `inv_bfcache` cookie value, always `location.replace`. An inline `<head>` cleanup script normalizes the URL bar before paint. No server changes, no TLA+ model changes; one note added to `tlaplus/README.md` documenting that the cache-bust enforces the model's pre-existing replace-branch fetch assumption.

**Tech Stack:** Go (`cmd/web`), `html/template` (`ui/templates`), vanilla JS (`ui/static/main.js`), Playwright-Go tests (`cmd/web/playwright_test.go`).

**Spec:** `docs/superpowers/specs/2026-05-26-stack-navigator-replace-force-fresh-design.md`.

**Supersedes (in implementation, not history):** commits `ceea54b` (`fix: stacknav fragment-only redirects now refetch the page`) and `024df74` (`fix: force reload on every same-URL submit involving a fragment`). The `history.replaceState + location.reload` carve-out those commits added is removed.

**Execution note:** Land this on `main` via the petrapp `worktreeflow` skill — create `.worktrees/stacknav-force-fresh-replace` from a freshly-fetched main, do the work, gate with `make ci`, then push the branch tip to `origin/main`.

---

## File map

| File | Change |
|---|---|
| `cmd/web/playwright_test.go` | Extend `Test_playwright_preferences_fragment_redirect` with viewport + URL-cleanup assertions; update ExpectResponse regex to allow `?bf_inv=...`. |
| `ui/static/main.js` | Rewrite the replace branch of `popOrPushTo` to use `bf_inv` cache-bust + `location.replace`. Update the header doc-comment to describe the new mechanism. |
| `ui/templates/base.gohtml` | Add an inline nonce'd cleanup script in `<head>` before the main.js `<script>` tag. |
| `tlaplus/README.md` | Add a short "Implementation note: cache-bust on replace" section between "What the model deliberately omits" and "Property of interest". |
| `cmd/web/CLAUDE.md` | Add one paragraph to "Redirects and Navigation" explaining that redirect paths may include `#fragment` and the shim handles same-doc-hop staleness. |

---

## Task 1: Pin the scroll regression with a failing viewport assertion

**Why first:** today, after my prior fix commits, `Test_playwright_preferences_fragment_redirect` passes — but the scroll-to-panel UX is broken because `location.reload()` triggers scroll-restoration, ignoring the fragment in the URL bar. Add a regression-protection assertion that currently fails, then make it pass in Task 2.

**Files:**
- Modify: `cmd/web/playwright_test.go` — extend `Test_playwright_preferences_fragment_redirect` (current body lives roughly at lines 740–860).

- [ ] **Step 1: Add the viewport assertion after the first submit**

  Open `cmd/web/playwright_test.go`. Find the block that runs immediately after the first `saveRecoveryBtn.Click()` and the `banner.WaitFor(...)` call. Just after the existing `aria-busy` check (lines 807–815 before this task) and BEFORE the second-submit ExpectResponse block, insert:

  ```go
  // Scroll-to-panel regression check. The whole point of including
  // #deload-title in the redirect target is to land the user with that
  // panel heading visible. location.reload() — the prior workaround —
  // triggers scroll-restoration on most browsers, ignoring the URL
  // fragment. ToBeInViewport asserts the geometry is correct after a
  // real fragment-scroll on a cross-doc navigation.
  assertions := playwright.NewPlaywrightAssertions()
  if err = assertions.Locator(
      page.Locator("#deload-title"),
  ).ToBeInViewport(); err != nil {
      t.Errorf("deload heading not in viewport after recovery save: %v", err)
  }
  ```

- [ ] **Step 2: Run the test, expect viewport assertion failure**

  Run:
  ```
  go test -count=1 -v -run Test_playwright_preferences_fragment_redirect ./cmd/web/
  ```

  Expected: FAIL with a message about the deload heading not being in the viewport. The other assertions (banner visible, URL has `/preferences` prefix, aria-busy cleared, second submit triggers GET, no flash leak on home) all still pass.

  If the test fails for any other reason (e.g. `playwright.NewPlaywrightAssertions` undefined), import-check the assertion API: it lives in `github.com/playwright-community/playwright-go` and the package alias is already `playwright`. The constructor signature is `NewPlaywrightAssertions(timeout ...float64) PlaywrightAssertions`; `.Locator(loc Locator) LocatorAssertions`; `.ToBeInViewport(opts ...LocatorAssertionsToBeInViewportOptions) error`. Adjust the call only if the API surface drifted in the vendored version.

- [ ] **Step 3: Commit the failing test**

  ```bash
  git add cmd/web/playwright_test.go
  git commit -m "$(cat <<'EOF'
  test: pin scroll-to-fragment regression after recovery save

  Today's main.js fix path uses history.replaceState + location.reload to
  recover the GET-handler refetch after a fragment-bearing redirect. The
  side effect is that location.reload triggers scroll-restoration on most
  browsers, so the #deload-title fragment in the URL bar is ignored and
  the panel never scrolls into view. This assertion pins that regression
  so the bf_inv implementation that's about to replace the workaround
  can't reintroduce it.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## Task 2: Implement bf_inv force-fresh mechanism (JS shim + cleanup script + test updates)

**Why one task:** the JS shim change, the inline cleanup script, and the test's ExpectResponse regex update are tightly coupled. Without the cleanup script the URL bar would carry `?bf_inv=...` permanently — visible regression. Without the regex update the second-submit `ExpectResponse` would time out (response URL is now `/preferences?bf_inv=...`, not `/preferences`). All three land together for a clean green.

**Files:**
- Modify: `ui/static/main.js` — rewrite the replace branch in `popOrPushTo` (current carve-out lives at lines 230–261). Update the header doc-comment.
- Modify: `ui/templates/base.gohtml` — add inline cleanup script in `<head>` before line 36 (the `<script src="/main.js">` tag).
- Modify: `cmd/web/playwright_test.go` — update the `ExpectResponse` regex to permit `?bf_inv=...`; add a URL-cleanup assertion after each submit.

- [ ] **Step 1: Rewrite the `popOrPushTo` replace branch in `ui/static/main.js`**

  In `ui/static/main.js`, replace the entire `if (replace || sameUrl(currentUrl, targetUrl)) { ... }` block (currently lines 238–261, the one with the `history.replaceState(null, '', target); location.reload()` carve-out) with the universal `bf_inv` rule:

  ```js
      // Replace mode: server-flagged via X-Replace-Url, or same-URL submit
      // (auto-detected so backend doesn't have to think about it). We
      // deliberately do not walk back looking for a traverse target —
      // replace is about erasing the current entry, not jumping elsewhere.
      //
      // The replace branch ALWAYS forces a cross-document fetch via a
      // bf_inv query param carrying the rotated inv_bfcache cookie. The
      // Navigation API resolves some same-pathname navigations (identical
      // URL, or same path + a fragment change) as same-document operations
      // that skip the GET — the freshly-rotated server state never reaches
      // the user, and the freshly-set session flash either stays unread or
      // leaks onto the next page. Differentiating the search component
      // forces a real cross-doc fetch; an inline cleanup script in
      // base.gohtml strips bf_inv before first paint so the URL bar
      // carries the canonical target form (fragment included, for native
      // scroll-to-anchor on the new document).
      // See docs/superpowers/specs/2026-05-26-stack-navigator-replace-force-fresh-design.md
      if (replace || sameUrl(currentUrl, targetUrl)) {
          const cookieValue = document.cookie
              .match(/(?:^|;\s*)inv_bfcache=([^;]+)/)?.[1] ?? ''
          const bust = new URL(target, location.origin)
          bust.searchParams.set('bf_inv', cookieValue || String(Date.now()))
          location.replace(bust.toString())
          return
      }
  ```

  Keep the pop-or-push branch below this exactly as it is.

- [ ] **Step 2: Update the `main.js` header doc-comment**

  At the top of `ui/static/main.js`, find the "Navigation strategy: pop-or-push (with explicit replace mode)" section in the doc-comment (around lines 19–40). Replace the "Replace mode" paragraph (around lines 22–27) with:

  ```
   *   1. Replace mode — server set X-Replace-Url: true, OR the target URL
   *      equals the current entry's URL ignoring fragment (same-URL submit).
   *      The current entry is replaced. ALWAYS forces a real cross-document
   *      fetch by appending a bf_inv cache-bust query param (carrying the
   *      rotated inv_bfcache cookie) and calling location.replace. Without
   *      the cache-bust, the Navigation API would resolve identical-URL or
   *      fragment-change navigations as same-document operations that skip
   *      the GET, leaving the freshly-set server state unread. An inline
   *      cleanup script in base.gohtml strips bf_inv before first paint so
   *      the URL bar lands on the canonical fragment-bearing target.
   *      We do not walk the history stack: replace is about erasing the
   *      current entry, not jumping to an existing one.
  ```

- [ ] **Step 3: Add the inline cleanup script to `ui/templates/base.gohtml`**

  In `ui/templates/base.gohtml`, find the line:
  ```
          <script {{ $.Nonce }} src="/main.js"></script>
  ```
  (line 36 before this task). INSERT the following block BEFORE that line, so the cleanup runs while the parser is still in `<head>` and before `main.js` loads:

  ```gohtml
          <script {{ $.Nonce }}>
              // Strip the bf_inv cache-bust query param injected by
              // popOrPushTo's replace branch in main.js. The param exists
              // only to force a cross-document fetch; once parsing has
              // started, the URL bar should carry the canonical target.
              // Runs synchronously during HTML parse, before main.js
              // loads, so the user never sees ?bf_inv=... after first
              // paint.
              // See docs/superpowers/specs/2026-05-26-stack-navigator-replace-force-fresh-design.md
              (() => {
                  const u = new URL(location.href);
                  if (!u.searchParams.has('bf_inv')) return;
                  u.searchParams.delete('bf_inv');
                  history.replaceState(null, '', u.toString());
              })();
          </script>
  ```

- [ ] **Step 4: Update the `ExpectResponse` regex in `cmd/web/playwright_test.go`**

  In `cmd/web/playwright_test.go`, find the regex line in `Test_playwright_preferences_fragment_redirect`:
  ```go
  prefsURLRe := regexp.MustCompile(regexp.QuoteMeta(serverURL+"/preferences") + "$")
  ```
  Replace with:
  ```go
  // After this fix the second-submit GET is /preferences?bf_inv=... ;
  // before it was /preferences . Accept either.
  prefsURLRe := regexp.MustCompile("^" + regexp.QuoteMeta(serverURL+"/preferences") + `(\?|$)`)
  ```

- [ ] **Step 5: Add a URL-cleanup assertion after each submit**

  In the same test function, immediately after the existing URL-prefix check (the block that ends `t.Errorf("URL after recovery save = %q, want prefix %q", ...)`, around lines 801–805 before this task), append:

  ```go
  // URL cleanup: the inline script in base.gohtml must have stripped
  // ?bf_inv=... from the URL bar by the time the banner is visible.
  // The canonical form is /preferences#deload-title.
  if got := page.URL(); strings.Contains(got, "bf_inv") {
      t.Errorf("URL still carries bf_inv after recovery save: %q", got)
  }
  if got := page.URL(); !strings.HasSuffix(got, "#deload-title") {
      t.Errorf("URL after recovery save = %q, want suffix %q", got, "#deload-title")
  }
  ```

  After the second-submit `banner.WaitFor(...)` block (around lines 839–843 before this task), append the same two assertions — repeated literally, not factored out, since the engineer may read tasks out of order and the repetition documents both submit checkpoints explicitly:

  ```go
  if got := page.URL(); strings.Contains(got, "bf_inv") {
      t.Errorf("URL still carries bf_inv after second recovery save: %q", got)
  }
  if got := page.URL(); !strings.HasSuffix(got, "#deload-title") {
      t.Errorf("URL after second recovery save = %q, want suffix %q", got, "#deload-title")
  }
  ```

- [ ] **Step 6: Run the test, expect all assertions to pass**

  Run:
  ```
  go test -count=1 -v -run Test_playwright_preferences_fragment_redirect ./cmd/web/
  ```

  Expected: PASS. The viewport assertion (Task 1) now passes because `location.replace` to a `?bf_inv=...` URL is a cross-document navigation, which honors fragment-scroll. The URL-cleanup assertions pass because the inline cleanup script runs synchronously during HTML parse and strips `bf_inv` before the banner is visible. The second-submit `ExpectResponse` matches because the regex now accepts `?bf_inv=...`.

  If `ExpectResponse` for the second submit times out, double-check the regex anchoring — the matcher checks against the full Response URL string, so a missing `^` will silently let other URLs match before the target.

  If the URL-cleanup assertion fails, verify the cleanup script is in `<head>` BEFORE `main.js`, not inside `<body>`. Running after `<body>` start may leave the URL polluted during early paint.

- [ ] **Step 7: Run the full playwright suite to catch any spillover**

  Run:
  ```
  go test -count=1 -v -run "Test_playwright_stacknav|Test_playwright_bfcache_staleness|Test_playwright_smoketest|Test_playwright_preferences_fragment_redirect" ./cmd/web/
  ```

  Expected: PASS for all four. `Test_playwright_stacknav` exercises Flow 1 (set update) and Flow 5 (validation error), both of which now go through `location.replace + bf_inv` instead of `navigation.navigate`. The URL flash through `?bf_inv=...` is invisible to assertions because the cleanup script runs before any assertion can read `page.URL()`.

- [ ] **Step 8: Commit the implementation**

  ```bash
  git add ui/static/main.js ui/templates/base.gohtml cmd/web/playwright_test.go
  git commit -m "$(cat <<'EOF'
  fix: stack-navigator force-fresh fetch on replace via bf_inv

  Replace the history.replaceState + location.reload carve-out in
  popOrPushTo's replace branch (added in ceea54b and broadened in
  024df74) with a universal cache-bust rule: every replace-branch firing
  rewrites the redirect target with a bf_inv query param carrying the
  rotated inv_bfcache cookie, then calls location.replace. The
  differing search component guarantees a cross-document fetch — the
  Navigation API's same-document optimization that skipped the GET on
  identical-URL or fragment-change navigations no longer applies, and
  native fragment-scroll fires on the fresh cross-doc load. An inline
  cleanup script in base.gohtml strips bf_inv via history.replaceState
  before first paint, so the URL bar carries the canonical target form.

  The TLA+ model assumed every replace-branch firing transitions
  docState → Loading / inFlight → GetPending (a real fetch); the
  cache-bust enforces the assumption that reality was violating. No
  model changes needed.

  Test extensions: viewport assertion proves scroll-to-fragment works
  (previously regressed by location.reload's scroll-restoration); URL-
  cleanup assertions prove the inline cleanup script ran; ExpectResponse
  regex updated to accept /preferences?bf_inv=... .

  See docs/superpowers/specs/2026-05-26-stack-navigator-replace-force-fresh-design.md
  See docs/superpowers/plans/2026-05-26-stack-navigator-replace-force-fresh.md

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## Task 3: Document the model alignment in `tlaplus/README.md`

**Why:** the TLA+ model's replace branch already assumes a real fetch; the cache-bust is the implementation detail that enforces it. Future readers need to know that fragment-bearing redirect targets are safe specifically because of the JS shim's cache-bust — without that context, someone might reasonably add a fragment-bearing redirect, look at the model, and conclude the model verified it.

**Files:**
- Modify: `tlaplus/README.md` — insert a new section between "What the model deliberately omits" (ends around line 101) and "Property of interest" (starts at line 102).

- [ ] **Step 1: Add the implementation-note section**

  In `tlaplus/README.md`, find the line that ends the "What the model deliberately omits" section (the last bullet, around line 101). Just before the next heading `## Property of interest` (line 102), insert:

  ```markdown
  ## Implementation note: cache-bust on replace

  The JS shim's `popOrPushTo` always rewrites every replace-branch target
  with a `bf_inv` query param (carrying the rotated `inv_bfcache` cookie)
  before calling `location.replace`. The model treats `Urls` as a finite
  opaque set and assumes every replace-branch firing transitions
  `docState → Loading / inFlight → GetPending` — i.e. a real cross-
  document fetch. Without the cache-bust, the browser would resolve some
  same-pathname navigations (identical URL, or same path + a fragment
  change) as same-document operations that skip the GET, leaving
  `docState = Loaded` with a stale `bakedToken` — a `NoStaleSettled`
  violation the model does NOT catch because URL distinctions below
  `Urls`-equality (fragments, identical-URL semantics) are abstracted
  away. The cache-bust enforces the model's assumption.

  An inline cleanup script in `ui/templates/base.gohtml` strips
  `?bf_inv=...` via `history.replaceState` before first paint, so the
  URL bar carries the canonical target form (fragment included).

  See `docs/superpowers/specs/2026-05-26-stack-navigator-replace-force-fresh-design.md`.

  ```

- [ ] **Step 2: Commit the docs change**

  ```bash
  git add tlaplus/README.md
  git commit -m "$(cat <<'EOF'
  docs(tlaplus): note bf_inv cache-bust enforces replace-branch fetch

  The model assumes every popOrPushTo replace-branch firing fires a
  real fetch (docState → Loading / inFlight → GetPending). The JS
  shim's bf_inv cache-bust is the implementation detail that makes
  reality match that assumption — without it, the browser would
  resolve some same-pathname navigations as same-document operations,
  silently violating NoStaleSettled in a way the model does not catch.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## Task 4: Update `cmd/web/CLAUDE.md` "Redirects and Navigation"

**Why:** handler authors need to know they can include `#fragment` in their redirect target and the shim handles the same-doc-hop gap. The current "Redirects and Navigation" section doesn't mention fragments at all.

**Files:**
- Modify: `cmd/web/CLAUDE.md` — append a paragraph to the "Using `redirect()` and `redirectReplace()`" subsection under "Redirects and Navigation".

- [ ] **Step 1: Add the fragment paragraph**

  Open `cmd/web/CLAUDE.md`. Find the "Using `redirect()` and `redirectReplace()`" subsection. After the existing paragraph that ends "an additional `X-Replace-Url: true` header (set by `redirectReplace`) flips the strategy from pop-or-push to replace.", BEFORE the `See docs/superpowers/specs/2026-05-03-...` line, insert:

  ```markdown
  Redirect paths may include a `#fragment` to land the user at a specific
  section after a form submit (e.g. `redirect(w, r, "/preferences#deload-title")`).
  The JS shim's `popOrPushTo` replace branch guarantees a real cross-
  document fetch via a `bf_inv` cache-bust query param it injects, then
  strips after parse — so any flash banner the handler sets is rendered
  on the next GET and native scroll-to-anchor fires on the new document.
  Handlers don't think about same-document semantics; emit the redirect
  with the fragment you want and the shim does the rest. See
  `docs/superpowers/specs/2026-05-26-stack-navigator-replace-force-fresh-design.md`.

  ```

- [ ] **Step 2: Commit the docs change**

  ```bash
  git add cmd/web/CLAUDE.md
  git commit -m "$(cat <<'EOF'
  docs(cmd/web): note fragments in redirect paths are shim-safe

  Handlers can include #fragment in the redirect target; popOrPushTo's
  replace branch cache-busts via bf_inv and the inline cleanup script
  strips the param before paint. The fragment drives native scroll-to-
  anchor on the new cross-doc load.

  Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## Task 5: Validate via `make ci` and push to `origin/main`

- [ ] **Step 1: Run the full CI gate**

  From inside the worktree:
  ```
  make ci
  ```

  Expected: PASS — go build, golangci-lint with --fix, full `go test --race --shuffle=on ./...`, and the vulnerability scan. Any lint hits should be fixable in place; do not weaken the lint rules.

  If the `godox` linter flags any "TODO" / "BUG" / "FIXME" tokens in the comments added in Task 2 or Task 3, reword the comment (this happened during prior fixes — the test comment "the bug fails fast" was caught by `godox`).

- [ ] **Step 2: Push the branch tip to origin/main**

  From inside the worktree:
  ```
  git push origin HEAD:main
  ```

  Expected: a fast-forward push containing four commits (Task 1 test pin, Task 2 implementation, Task 3 tlaplus note, Task 4 CLAUDE.md note).

  If the push is rejected as non-fast-forward (someone else pushed in the meantime):
  ```
  git fetch origin
  git rebase origin/main
  make ci
  git push origin HEAD:main
  ```

- [ ] **Step 3: Teardown the worktree per worktreeflow Phase 3**

  Run the four cleanup commands separately, not chained, to avoid the auto-mode permission classifier denying the `pull --ff-only origin main` mid-chain:

  ```
  cd /home/martin/petrapp
  git worktree remove .worktrees/stacknav-force-fresh-replace
  git pull --ff-only origin main
  git branch -d stacknav-force-fresh-replace
  ```

  Expected: clean teardown, outer `main` now at the pushed tip, the feature branch is deleted (safe `-d` works because main contains the branch tip).

---

## Acceptance criteria (cross-check before declaring done)

- [ ] `Test_playwright_preferences_fragment_redirect` passes with the new viewport and URL-cleanup assertions.
- [ ] After submitting any Recovery-panel form on `/preferences` (or from `/preferences#deload-title`), the URL bar reads exactly `/preferences#deload-title` — no `?bf_inv=` artifact — AND the `#deload-title` heading is in the viewport.
- [ ] `Test_playwright_stacknav` (Flow 1 set update, Flow 5 validation error) still passes — those flows now go through `location.replace + bf_inv` too.
- [ ] `Test_playwright_bfcache_staleness` still passes — back-navigation path unaffected.
- [ ] The `history.replaceState + location.reload` block from commits `ceea54b` / `024df74` is gone from `popOrPushTo`'s replace branch.
- [ ] `tlaplus/README.md` and `cmd/web/CLAUDE.md` each gain one section/paragraph documenting the mechanism.
- [ ] `make ci` passes.
- [ ] Branch tip is on `origin/main`; worktree and feature branch are removed.
