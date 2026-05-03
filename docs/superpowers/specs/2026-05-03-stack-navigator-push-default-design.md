# Stack Navigator: Push by Default

## Background

The current stack navigator (designed in `2026-04-25-stack-navigator-redesign-design.md`) uses a single "pop-or-replace" strategy on form submit: walk history backward looking for a URL match, traverse if found, otherwise replace the current entry. This collapses cleanly for same-URL submits and pop-to-ancestor flows, but has one sharp edge — cross-URL form submits whose target is *not* already in history silently overwrite the originating page.

The most visible symptom is "Start Workout" on the home page. The user is at `/`, clicks the form button, and lands at `/workouts/{date}`. Because `/workouts/{date}` is not in the backward stack, pop-or-replace falls through to replace — the home entry is destroyed. The browser back button then exits the app instead of returning to the home page. The same shape applies to any future cross-URL form whose target is brand-new in history: the originating page vanishes.

This redesign flips the default to push and adds an explicit opt-in for the rare "form page is disposable" case.

## Goals

- Cross-URL form submits whose target is new push the new entry on top of the originator (back button returns to the originator).
- Cross-URL form submits whose target is already in the backward stack continue to traverse (no duplicate entries).
- Form pages whose existence should be erased on submit (e.g. `/add-exercise`) get an explicit opt-in for replace semantics.
- Same-URL submits (set updates, warmup completion, validation errors) replace in place with no per-handler ceremony.
- Improve the add-exercise UX: after adding, land the user on the new exercise's detail page (instead of the workout overview) so they can start tracking sets immediately, with `/add-exercise` erased from history.
- Progressive enhancement preserved: no JS / no Navigation API still works via plain 303.

## Non-goals

- No new transport. The existing fetch + custom-header wire protocol is kept.
- No file-upload support through the enhanced submit path (unchanged from prior spec).
- No virtual stack or in-memory page cache (unchanged).
- No view-transition polish beyond what already exists.
- No general "history action" enum. Only push (default) and replace (opt-in) are needed; same-URL is handled implicitly by the client.

## Architecture

Three small pieces. The wire protocol gains one optional response header; the client gains one branch; the server gains one helper.

### Wire protocol

| Direction | Header | Value | Required? | Meaning |
|---|---|---|---|---|
| Request | `X-Requested-With` | `stacknav` | yes (set by JS shim) | This POST is from the JS shim. |
| Response | `X-Location` | URL path | yes (when 200) | Where the client should navigate. |
| Response | `X-Replace-URL` | `true` | optional | Replace the current entry rather than push (form page is disposable). |

Status codes are unchanged: `200` with `X-Location` for both successes and validation errors (validation continues to use flash + redirect-to-form; the form's GET handler pops the flash on re-render); anything else triggers `location.reload()`.

Non-stacknav callers (no `X-Requested-With` header — e.g. JS disabled or Navigation API unavailable) get plain `303 See Other` and the browser follows. This is the no-JS PRG fallback path.

### Client (`ui/static/main.js`)

The dispatch function is renamed `popOrPushTo` (from `popOrReplaceTo`) and gains an optional `replace` parameter. The replace branches sit *before* the backward-walk loop — they are not fallbacks; they describe a different mode entirely:

```js
async function popOrPushTo(target, {replace = false} = {}) {
    const targetUrl = new URL(target, location.origin)

    // Replace mode (server-flagged or same-URL submit): stay in place. We
    // deliberately do not walk back looking for a traverse target — replace
    // is about erasing the current entry, not jumping to an existing one.
    if (replace || sameUrl(new URL(navigation.currentEntry.url), targetUrl)) {
        navigation.navigate(target, {history: 'replace'})
        return
    }

    // Genuine cross-URL navigation. Traverse to a backward match if present,
    // otherwise push.
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

The caller in `submitForm` reads the optional response header:

```js
if (res.status === 200) {
    const target = res.headers.get('X-Location')
    if (!target) { location.reload(); return }
    const replace = res.headers.get('X-Replace-URL') === 'true'
    await popOrPushTo(target, {replace})
    return
}
```

Same-URL detection lives entirely on the client. The server never has to think about whether a redirect target equals the request URL.

The header doc-comment at the top of `main.js` is rewritten to describe the new strategy and the wire protocol's optional `X-Replace-URL` header.

### Server (`cmd/web/helpers.go`)

The existing `redirect` helper keeps its current shape and call sites:

```go
func redirect(w http.ResponseWriter, r *http.Request, path string) {
    if r.Header.Get("X-Requested-With") == "stacknav" {
        w.Header().Set("X-Location", path)
        w.WriteHeader(http.StatusOK)
        return
    }
    http.Redirect(w, r, path, http.StatusSeeOther)
}
```

A new helper covers the opt-in replace case:

```go
// redirectReplace is like redirect, but signals to the stack navigator that
// the current history entry should be replaced. Use this for form pages
// whose existence should be erased on submit (e.g. /add-exercise).
// Non-stacknav callers fall through to a plain 303, identical to redirect.
func redirectReplace(w http.ResponseWriter, r *http.Request, path string) {
    if r.Header.Get("X-Requested-With") == "stacknav" {
        w.Header().Set("X-Location", path)
        w.Header().Set("X-Replace-URL", "true")
        w.WriteHeader(http.StatusOK)
        return
    }
    http.Redirect(w, r, path, http.StatusSeeOther)
}
```

Same-URL submits do NOT use `redirectReplace` — the client handles that automatically. `redirectReplace` is only for the genuinely opt-in case where the form page is disposable.

## Per-flow behavior

These are the assertions verified by the Playwright spec.

| # | Flow | Mechanism | Back goes to |
|---|---|---|---|
| 1 | Set update / warmup complete (DETAIL → DETAIL) | Client same-URL auto-replace | Workout overview |
| 2 | Schedule submit success (/schedule → /) | Traverse to / if in history, else push | Parent of /schedule (or /schedule itself if cold) |
| 3 | Schedule validation error (/schedule → /schedule) | Client same-URL auto-replace | Parent of /schedule |
| 4 | **Start workout (/ → /workouts/{date})** | **Push (target absent)** | **/ ✓ (bug fix)** |
| 5 | Complete workout (WORKOUT → /complete) | Push | WORKOUT |
| 6 | Workout feedback (/complete → /) | Traverse to / | Exits app |
| 7 | Swap exercise (SWAP → DETAIL via same-slot URL) | Traverse to DETAIL | Workout overview |
| 8 | **Add exercise (/add-exercise → new DETAIL)** | **`redirectReplace` → replace** | **Workout overview** |
| 9 | Add exercise validation (/add-exercise → /add-exercise) | Client same-URL auto-replace | Workout overview |
| 10 | Hierarchical back link (`a[data-back-button]`) | Client-only traverse | Per existing spec |

Flow 4 is the bug fix. Flow 8 is the new UX.

## Server-side handler changes

| Handler | Change |
|---|---|
| `workoutAddExercisePOST` (`cmd/web/handler-workout.go`) | Redirect target changes from `/workouts/{date}` to `/workouts/{date}/exercises/{newWorkoutExerciseID}`, called via `redirectReplace`. |
| `workout.Service.AddExercise` (`internal/workout/service.go`) | Return signature changes from `error` to `(int, error)`. The int is the newly assigned `workoutExerciseID` (slot ID). |

All other handlers are unchanged. `redirect` keeps every existing call site.

### Add-exercise UX rationale

Today, after adding an exercise the user lands on the workout overview and has to click into the new exercise to start tracking sets. Pushing them straight onto the new DETAIL with `/add-exercise` erased from history is the expected mobile-app gesture: tap "Add this exercise" → the new exercise is the next thing you see, back returns to the workout overview as if you never visited the picker.

## Testing

`Test_playwright_stacknav` in `cmd/web/playwright_test.go` is updated; one new flow is added; the `addExerciseToWorkout` helper changes to match the new add-exercise redirect target.

### Updated flows

- Flow 1 (set update) and Flow 5 (validation error) — observable assertions unchanged; comments are updated to describe the new same-URL auto-detect mechanism instead of "pop-or-replace fallthrough".
- Flow 2 (swap) — observable behavior unchanged. Same-slot URL traverses to original DETAIL.
- Flow 3 (schedule submit) — observable assertions unchanged given the existing two-stage setup that pushes / into history before the test submit. Comments updated to describe pop-or-PUSH instead of pop-or-REPLACE.
- Flow 4 (data-back-button) — untouched.

### New flow: Flow 6 — Add-exercise replace

Setup:
- Navigate into a workout day's overview (`/workouts/{date}`).
- Click the add-exercise link → land at `/workouts/{date}/add-exercise`.

Action:
- Click "Add this exercise" on the first available exercise. Capture its name beforehand.

Assertions:
- URL matches `/workouts/{date}/exercises/\d+$`.
- The DETAIL page shows the just-added exercise (verify by name match against the captured value).
- `page.GoBack()` lands on the workout overview URL (not `/add-exercise`) — proves replace, not push.
- `page.GoForward()` does not return to `/add-exercise` (the entry was destroyed by replace).

### Helper update

`addExerciseToWorkout` currently waits for the workout overview after submit. With the new flow it must:
1. Wait for a DETAIL URL (`/workouts/{date}/exercises/\d+`).
2. `page.Goto(workoutURL)` to return to the overview for the caller's downstream assertions.

## Documentation

`cmd/web/CLAUDE.md` "Redirects and Navigation" section is rewritten:

- Document both `redirect` and `redirectReplace` helpers, with guidance on when each applies.
- Update the client-behavior summary: traverse if backward match, replace if same-URL or `X-Replace-URL: true`, otherwise push.
- Update the spec link to point at this document.

The prior spec (`2026-04-25-stack-navigator-redesign-design.md`) stays in place as historical context. Its "Per-flow behavior" and client-implementation sections are superseded by this document.

## Browser support

Unchanged from the prior spec. Navigation API support is required for the JS layer to do anything; without it, forms submit natively via the 303 fallback. iOS Safari still uses `e.preventDefault()` + awaited fetch instead of `e.intercept()` (WebKit bug 293952). `pagereveal` view-transition direction classification is Chromium-only and remains a progressive nice-to-have.

## Migration

Single coherent change. The protocol header, client function rename, server helper, handler update, service signature change, test updates, and CLAUDE.md edits all land in one commit/PR. No staged rollout needed; the new header is purely additive (older clients ignore it because they would not be running this updated `main.js`).

## References

- Prior spec: `docs/superpowers/specs/2026-04-25-stack-navigator-redesign-design.md`.
- HTMX precedent for similar headers: `HX-Push-Url` / `HX-Replace-Url`. We deliberately use a simpler boolean-only `X-Replace-URL` since the navigation target is already conveyed by `X-Location`.
- WebKit bug [293952](https://bugs.webkit.org/show_bug.cgi?id=293952) — iOS Safari precommit handler support; reason we use `preventDefault()` instead of `e.intercept()`.
- MDN: [Navigation API](https://developer.mozilla.org/en-US/docs/Web/API/Navigation_API).
