# Error UX Conventions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unify all user-facing error surfacing on the existing `banner` flash flow by adding an `app.userError` helper, migrate the "add exercise on unplanned day" path as a worked example, and add a `#js-flash` client-side surface for network failures.

**Architecture:** Every POST handler ends in exactly one of `redirect` (success), `app.userError(w, r, err, safeURL)` (any user-visible failure), or `app.serverError(w, r, err)` (true full-page failure). `userError` routes by error type: `domain.ValidationError` → `ve.Message`; anything else → log + generic message. Both write the flash and `redirect` to a handler-provided safe URL. The JS shim's `fetch` catch path populates a static `<div id="js-flash" role="alert">` via `textContent` instead of blind `location.reload()` (CSP / Trusted-Types clean).

**Tech Stack:** Go (stdlib `net/http`, `errors`, `log/slog`), `e2etest` + `goquery` for handler tests, server-rendered `gohtml` templates, vanilla JS (Navigation API + `fetch`), `@scope` CSS with CSP nonce, sqlite for service tests.

**Spec:** [docs/superpowers/specs/2026-05-18-error-ux-conventions-design.md](../specs/2026-05-18-error-ux-conventions-design.md)

---

## File Structure

**Modify:**
- `internal/service/exercises.go` — change `AddExercise` to return `domain.ValidationError` on the missing-session branch.
- `internal/service/exercises_test.go` — adjust the existing "Add exercise to non-existent workout" subtest to assert the new error type.
- `cmd/web/helpers.go` — add `userError`.
- `cmd/web/handler-workout.go` — (a) plumb flash through the `workout-not-found` path of `workoutGET`; (b) migrate `workoutAddExercisePOST` to `userError`.
- `ui/templates/pages/workout-not-found/workout-not-found.gohtml` — render the `banner` partial.
- `cmd/web/handler-workout_test.go` — add a regression test for the unplanned-day add-exercise flow.
- `ui/templates/base.gohtml` — add `#js-flash` skeleton plus a co-located nonce'd `<style>`.
- `ui/static/main.js` — update `submitForm` (clear `#js-flash` at the top; populate it in the `catch` instead of reloading).
- `cmd/web/CLAUDE.md` — replace "User-facing validation errors" section with a "User-facing errors" section covering the three terminal calls.
- `ui/templates/CLAUDE.md` — add a one-paragraph note about `#js-flash` under the JavaScript section.

**Create:** none. No new files; the convention reuses the existing `banner` component.

**No `main.js` cache-bust step needed**: per the project root `CLAUDE.md`, `main.js` is md5-fingerprinted at Docker build time and `base.gohtml` references are rewritten automatically.

**Key prior discovery driving Task 3**: only four pages today render `{{ template "banner" .Flash }}` (`workout`, `schedule`, `admin-exercises`, `admin-exercise-edit`). The `workout-not-found` template — which is exactly the page our worked example redirects to — does not. Without Task 3, the flash from `userError` would be silently consumed on the redirect target and the regression test in Task 4 would fail with "no banner found."

---

## Task 1: Domain service returns `ValidationError` for missing session

**Files:**
- Modify: `internal/service/exercises.go:155-159`
- Test: `internal/service/exercises_test.go:356-385` (subtest "Add exercise to non-existent workout")

- [ ] **Step 1: Tighten the failing test to assert the new error type**

Replace the existing subtest body (lines 356–385) so it asserts a `domain.ValidationError` with the documented message, not just "non-nil error":

```go
// Test adding an exercise to a non-existent workout (should return a
// user-facing ValidationError).
t.Run("Add exercise to non-existent workout", func(t *testing.T) {
    // Set a future date for a workout that doesn't exist yet
    futureDate := today.AddDate(0, 0, 7) // 1 week in the future

    // Verify the workout doesn't exist yet
    var existsCheck bool
    var errExists error
    existsCheck, errExists = workoutExistsForDate(ctx, t, svc, futureDate)
    if errExists != nil {
        t.Fatalf("Failed to check if workout exists: %v", errExists)
    }
    if existsCheck {
        t.Fatalf("Workout already exists for future date, can't test error case")
    }

    // Add exercise to the non-existent workout - should fail with a
    // ValidationError carrying a user-facing message.
    _, err = svc.AddExercise(ctx, futureDate, exercise1ID)
    if err == nil {
        t.Fatal("Expected error when adding exercise to non-existent workout, but got nil")
    }
    var ve domain.ValidationError
    if !errors.As(err, &ve) {
        t.Fatalf("Expected ValidationError, got %T: %v", err, err)
    }
    wantMsg := "This day has no planned workout. Schedule one from the home page first."
    if ve.Message != wantMsg {
        t.Errorf("ValidationError.Message = %q, want %q", ve.Message, wantMsg)
    }

    // Verify workout was NOT created
    existsCheck, errExists = workoutExistsForDate(ctx, t, svc, futureDate)
    if errExists != nil {
        t.Fatalf("Failed to check if workout was created: %v", errExists)
    }
    if existsCheck {
        t.Error("Workout was created for future date when it should not have been")
    }
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -run Test_AddExercise/Add_exercise_to_non-existent_workout ./internal/service/`
Expected: FAIL with something like `Expected ValidationError, got *fmt.wrapError: workout session for date ... does not exist`.

- [ ] **Step 3: Update the service to return ValidationError**

Replace `internal/service/exercises.go:155-159` (the `if _, err = s.repos.Sessions.Get(ctx, date); errors.Is(err, domain.ErrNotFound) { ... }` block) with:

```go
if _, err = s.repos.Sessions.Get(ctx, date); errors.Is(err, domain.ErrNotFound) {
    return 0, domain.ValidationError{
        Message: "This day has no planned workout. Schedule one from the home page first.",
    }
} else if err != nil {
    return 0, fmt.Errorf("check session existence: %w", err)
}
```

The `errors` import is already present in the file (used by `errors.Is`); no import change needed.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race -run Test_AddExercise ./internal/service/`
Expected: PASS for all `Test_AddExercise/*` subtests.

- [ ] **Step 5: Run full service tests + lint to catch regressions**

Run: `go test -race -shuffle=on ./internal/service/...` — Expected: PASS.
Run: `golangci-lint run --fix ./internal/service/...` — Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/service/exercises.go internal/service/exercises_test.go
git commit -m "$(cat <<'EOF'
service: return ValidationError when adding exercise to unplanned day

Replaces the opaque fmt.Errorf wrap on the missing-session branch of
AddExercise with a domain.ValidationError carrying a user-actionable
message. Handlers can now route this through the flash + banner flow
instead of treating it as a 500.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add `app.userError` helper

**Files:**
- Modify: `cmd/web/helpers.go`

No standalone test — coverage comes from the handler test in Task 4. The helper is glue: it dispatches on `ValidationError` and delegates to existing tested primitives (`putFlashError`, `redirect`, `slog`).

- [ ] **Step 1: Add the helper**

Append the following to `cmd/web/helpers.go` (place it directly below `serverError`). Add `"errors"` and `"github.com/myrjola/petrapp/internal/domain"` to the import block at the top of the file.

```go
// userError surfaces a failure of an in-flight user action through the
// flash + banner flow and redirects the client to safeURL. Use it instead
// of serverError when there is a meaningful page to land the user on.
//
// Routing:
//   - domain.ValidationError → flash with ve.Message (safe to display).
//   - any other error → log at ERROR and flash a generic message.
//
// safeURL must point at a GET handler known to render successfully AND
// that pops + renders the flash. Do NOT pass an action endpoint
// (e.g. ".../complete"), and do not default to r.Referer() — it is
// unreliable on direct POSTs.
func (app *application) userError(
    w http.ResponseWriter, r *http.Request, err error, safeURL string,
) {
    var ve domain.ValidationError
    var msg string
    if errors.As(err, &ve) {
        msg = ve.Message
    } else {
        app.logger.LogAttrs(r.Context(), slog.LevelError,
            "user-facing server error", slog.Any("error", err))
        msg = "Couldn't complete that action. Please try again."
    }
    app.putFlashError(r.Context(), msg)
    redirect(w, r, safeURL)
}
```

The imports section at the top of `cmd/web/helpers.go` should become:

```go
import (
    "context"
    "errors"
    "log/slog"
    "net/http"
    "strconv"
    "time"

    "github.com/myrjola/petrapp/internal/domain"
)
```

- [ ] **Step 2: Verify the package builds and lints**

Run: `go build ./cmd/web/...` — Expected: clean build.
Run: `golangci-lint run --fix ./cmd/web/...` — Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add cmd/web/helpers.go
git commit -m "$(cat <<'EOF'
web: add app.userError helper for user-facing action failures

Single helper that routes ValidationError (user message) vs other errors
(logged + generic message), writes the flash, and redirects to a
handler-provided safe URL. Replaces the per-handler errors.As(&ve)
boilerplate going forward and provides a flash-aware path for unexpected
errors that today land users on a generic 500 page.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Plumb flash through `workout-not-found`

The `workout-not-found` page is the redirect target for the unplanned-day worked example. Today its template renders no banner and its handler path does not pop the flash. We extend it to do both so the `userError` flash actually surfaces.

**Files:**
- Modify: `cmd/web/handler-workout.go:61-72` (the `workoutNotFoundTemplateData` struct + `newWorkoutNotFoundTemplateData` constructor) and lines 140-143 + 164-166 (the two `app.render(... "workout-not-found", ...)` call sites in `workoutStartPOST` and `workoutGET`).
- Modify: `ui/templates/pages/workout-not-found/workout-not-found.gohtml`

- [ ] **Step 1: Read the existing struct + constructor**

Run: `sed -n '55,75p' cmd/web/handler-workout.go`
Confirm the struct shape; note the exact field names so the next step's edit is precise.

- [ ] **Step 2: Add a `Flash BannerData` field and constructor parameter**

Modify `workoutNotFoundTemplateData` to include `Flash BannerData` and update `newWorkoutNotFoundTemplateData` to accept a `flashMessage string` parameter (mirroring the pattern in `newWorkoutTemplateData` at line 184). The constructor wraps the message in `BannerData{Variant: "error", Message: flashMessage}` and assigns it to `Flash`. Empty messages render nothing because the `banner` partial guards on `.Message`.

Example shape (read the surrounding code first to match field order and style):

```go
type workoutNotFoundTemplateData struct {
    BaseTemplateData
    Date   time.Time
    Header PageHeaderData
    Flash  BannerData
}

func newWorkoutNotFoundTemplateData(
    r *http.Request, date time.Time, flashMessage string,
) workoutNotFoundTemplateData {
    return workoutNotFoundTemplateData{
        BaseTemplateData: newBaseTemplateData(r),
        Date:             date,
        Header:           /* keep whatever the existing constructor builds */,
        Flash:            BannerData{Variant: "error", Message: flashMessage},
    }
}
```

If the existing constructor doesn't have a `Header` field, leave that out — match what's actually there. The only required additions are the new `Flash` field and the `flashMessage` parameter.

- [ ] **Step 3: Update both `app.render(... "workout-not-found", ...)` call sites to pop the flash**

In `cmd/web/handler-workout.go`, line ~141 (inside `workoutStartPOST`'s not-found branch) and line ~165 (inside `workoutGET`'s not-found branch), change:

```go
data := newWorkoutNotFoundTemplateData(r, date)
```

to:

```go
data := newWorkoutNotFoundTemplateData(r, date, app.popFlashError(r.Context()))
```

- [ ] **Step 4: Render the banner in the template**

Modify `ui/templates/pages/workout-not-found/workout-not-found.gohtml`. Add the banner render directly inside `<main>` and **above** the `{{ template "page-header" .Header }}` line, so the message is the first thing the user sees on the page. The relevant fragment becomes:

```gohtml
    <main class="stack">
        <style {{ nonce }}>
            @scope {
                :scope {
                    margin: var(--size-4);
                    align-items: center;
                    text-align: center;
                    padding: var(--size-6);
                }
                /* ... existing rules unchanged ... */
            }
        </style>

        {{ template "banner" .Flash }}

        {{ template "page-header" .Header }}
```

Leave the rest of the template unchanged. The `banner` partial renders nothing when `Flash.Message` is empty, so existing visits to the not-found page (without a flash) are unaffected.

- [ ] **Step 5: Build + sanity check**

Run: `go build ./cmd/web/...` — Expected: clean.
Run: `go test -race -run Test_application_workout -shuffle=on ./cmd/web/` — Expected: PASS (no regression in existing workout handler tests).

- [ ] **Step 6: Commit**

```bash
git add cmd/web/handler-workout.go ui/templates/pages/workout-not-found/workout-not-found.gohtml
git commit -m "$(cat <<'EOF'
web: surface flash on the workout-not-found page

Plumbs a Flash BannerData field through workoutNotFoundTemplateData and
pops + renders it on both the workoutStartPOST and workoutGET
not-found branches. Empty messages render nothing, so existing not-found
visits are visually unchanged. Prerequisite for the userError redirect
target in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Migrate `workoutAddExercisePOST` to `userError` (regression test)

**Files:**
- Modify: `cmd/web/handler-workout.go:496-535` (the `workoutAddExercisePOST` body, particularly lines 525-528 where `AddExercise` is called)
- Test: `cmd/web/handler-workout_test.go` (new test function appended to file)

- [ ] **Step 1: Write the failing handler test**

Append to `cmd/web/handler-workout_test.go`. The test uses `client.HTTPClient()` directly rather than `client.SubmitForm` because the unplanned-date page has no form to discover — the e2etest helper requires a `goquery.Document` containing the form being submitted, and the page that "owns" the action doesn't exist on the unplanned date.

The `e2etest.Client` already wires `Sec-Fetch-Site: same-origin` into its transport, so direct `http.Client.Do` requests pass Go 1.25's `CrossOriginProtection` (the project's CSRF mechanism — see `cmd/web/middleware.go:221-224`). The base URL is exposed via `server.URL()`.

```go
// Test_application_workoutAddExercisePOST_unplanned_day verifies that
// POSTing /workouts/{date}/exercises on a date with no planned session
// surfaces the flash + banner explaining the situation instead of a 500.
// The user lands back on the workout-not-found page with role="alert"
// banner content.
func Test_application_workoutAddExercisePOST_unplanned_day(t *testing.T) {
    ctx := t.Context()

    server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
    if err != nil {
        t.Fatalf("Failed to start server: %v", err)
    }
    client := server.Client()

    if _, err = client.Register(ctx); err != nil {
        t.Fatalf("Register: %v", err)
    }

    // Set preferences so today is a planned day; the date 8 days out lands
    // on a different weekday (an unplanned day) for every possible "today".
    formData := map[string]string{time.Now().Weekday().String(): "60"}
    doc, err := client.GetDoc(ctx, "/preferences")
    if err != nil {
        t.Fatalf("Get preferences: %v", err)
    }
    if _, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {
        t.Fatalf("Submit preferences: %v", err)
    }

    plannedDate := time.Now().Format("2006-01-02")
    unplannedDate := time.Now().AddDate(0, 0, 8).Format("2006-01-02")

    // Start the planned workout so the picker page renders, then grab any
    // exercise_id off the picker — data-driven against fixtures.
    if _, err = client.SubmitForm(ctx, doc, "/workouts/"+plannedDate+"/start", nil); err != nil {
        t.Fatalf("Start planned workout: %v", err)
    }
    pickerDoc, err := client.GetDoc(ctx, "/workouts/"+plannedDate+"/add-exercise")
    if err != nil {
        t.Fatalf("Get add-exercise picker: %v", err)
    }
    exerciseID, ok := pickerDoc.Find(".exercise-option [name='exercise_id']").
        First().Attr("value")
    if !ok || exerciseID == "" {
        t.Fatalf("Could not find an exercise_id in the picker page")
    }

    // POST directly to the unplanned date's exercises endpoint.
    body := url.Values{"exercise_id": {exerciseID}}.Encode()
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        server.URL()+"/workouts/"+unplannedDate+"/exercises",
        strings.NewReader(body))
    if err != nil {
        t.Fatalf("Build POST request: %v", err)
    }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    resp, err := client.HTTPClient().Do(req)
    if err != nil {
        t.Fatalf("POST to unplanned date: %v", err)
    }
    if cerr := resp.Body.Close(); cerr != nil {
        t.Fatalf("Close response body: %v", cerr)
    }
    // Non-stacknav POSTs get a 303 See Other (see helpers.go:redirect).
    if resp.StatusCode != http.StatusSeeOther {
        t.Fatalf("Expected 303 See Other from userError redirect, got %d", resp.StatusCode)
    }
    if loc := resp.Header.Get("Location"); loc != "/workouts/"+unplannedDate {
        t.Errorf("Expected Location: /workouts/%s, got %q", unplannedDate, loc)
    }

    // Follow the redirect and assert the banner is rendered on the
    // workout-not-found page.
    workoutResp, err := client.Get(ctx, "/workouts/"+unplannedDate)
    if err != nil {
        t.Fatalf("Get workout page after redirect: %v", err)
    }
    defer func() { _ = workoutResp.Body.Close() }()
    // workout-not-found renders with HTTP 404; the banner is still in the body.
    workoutDoc, err := goquery.NewDocumentFromReader(workoutResp.Body)
    if err != nil {
        t.Fatalf("Parse workout page: %v", err)
    }
    banner := workoutDoc.Find("[role='alert']").First()
    if banner.Length() == 0 {
        t.Fatalf("Expected role=\"alert\" banner on workout-not-found page; got none")
    }
    wantSubstr := "no planned workout"
    if !strings.Contains(strings.ToLower(banner.Text()), wantSubstr) {
        t.Errorf("Banner text = %q, want substring %q",
            strings.TrimSpace(banner.Text()), wantSubstr)
    }
}
```

Add the following imports to `cmd/web/handler-workout_test.go` if not already present (the existing search test already uses `net/url`, `strings`, `time`, `goquery`, `e2etest`, `testhelpers` — verify the import block):

- `"net/http"` — for `http.NewRequestWithContext`, `http.MethodPost`, `http.StatusSeeOther`.
- `"net/url"` — for `url.Values{}.Encode()` (likely already present as `neturl` or `url`).
- `"strings"` — already present.
- `"github.com/PuerkitoBio/goquery"` — for `goquery.NewDocumentFromReader` (already present).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -race -run Test_application_workoutAddExercisePOST_unplanned_day -v ./cmd/web/`
Expected: FAIL. The current handler calls `serverError` on the wrapped error → 500 response (not 303), so the status-code assertion fires first. Capture which.

- [ ] **Step 3: Migrate the handler**

In `cmd/web/handler-workout.go`, replace lines 525-529 (the `if err != nil { app.serverError(w, r, err); return }` block immediately after `AddExercise`) with:

```go
newWorkoutExerciseID, err := app.service.AddExercise(r.Context(), date, exerciseID)
if err != nil {
    workoutURL := fmt.Sprintf("/workouts/%s", date.Format("2006-01-02"))
    app.userError(w, r, err, workoutURL)
    return
}
```

No other changes in the handler. The `errors` and `domain` imports remain.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -race -run Test_application_workoutAddExercisePOST_unplanned_day -v ./cmd/web/`
Expected: PASS.

- [ ] **Step 5: Run the full handler-workout test file**

Run: `go test -race -shuffle=on ./cmd/web/ -run Test_application_workout`
Expected: PASS for all `workout*` tests; the existing search test still passes.

- [ ] **Step 6: Lint**

Run: `golangci-lint run --fix ./cmd/web/...`
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-workout.go cmd/web/handler-workout_test.go
git commit -m "$(cat <<'EOF'
web: route add-exercise unplanned-day failure through userError

Replaces the serverError fallthrough in workoutAddExercisePOST with the
new userError helper, redirecting back to the workout-not-found page
where the flash + banner explains "This day has no planned workout."
Adds a handler test that POSTs to an unplanned date and asserts the
303 redirect plus the role=alert banner on the resulting page.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Add `#js-flash` skeleton to `base.gohtml`

**Files:**
- Modify: `ui/templates/base.gohtml:43-47` (the `<body>` opening through `{{ template "page" . }}`)

- [ ] **Step 1: Add the skeleton with co-located style**

Replace lines 43-47 of `ui/templates/base.gohtml` with:

```gohtml
    <body>
    <div id="loading-bar" aria-hidden="true"></div>
    <div id="loading-announce" role="status" aria-live="polite" class="sr-only"></div>
    <style {{ nonce }}>
        @scope (#js-flash) {
            :scope {
                margin: var(--size-3);
                padding: var(--size-2) var(--size-3);
                border-radius: var(--radius-2);
                font-weight: var(--font-weight-5);
                background: var(--color-error-bg);
                color: var(--color-error);
            }
            :scope[hidden] {
                display: none;
            }
        }
    </style>
    <div id="js-flash" role="alert" hidden></div>
    {{ template "page" . }}
    </body>
```

Notes:
- `role="alert"` is implicit `aria-live="assertive"`. Modifying `textContent` and toggling `hidden=false` triggers an announcement.
- The `[hidden]` selector inside `@scope` is defensive — the HTML `hidden` attribute already implies `display: none`, but stating it inside the scope guards against future scope-internal selectors that would otherwise override it.
- Styles intentionally mirror the existing `.banner` / `.banner--error` rules in `ui/templates/components/banner.gohtml` so the visual surface is consistent without coupling to that component.

- [ ] **Step 2: Boot the dev server and eyeball the skeleton**

Run: `go run ./cmd/web/` in a background terminal (or `make run` if defined). Note the listening port from the log output.
Open: `http://localhost:<port>/`.
Devtools → Elements: confirm `<div id="js-flash" role="alert" hidden></div>` exists immediately after `loading-announce`.
Devtools → Console:
```
const f = document.getElementById('js-flash');
f.textContent = 'Test'; f.hidden = false;
```
Expect a red-on-light-red banner at the top of the page.
Then:
```
f.hidden = true;
```
Expect it to vanish.

Kill the dev server.

- [ ] **Step 3: Run the styleguide test to confirm template parsing is still clean**

Run: `go test -race -run Test_application_styleguide -shuffle=on ./cmd/web/`
Expected: PASS (the styleguide page exercises the same template parse stack, so a template-parsing regression surfaces here).

Run also: `go test -race -shuffle=on ./cmd/web/...` for a fuller sweep — Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add ui/templates/base.gohtml
git commit -m "$(cat <<'EOF'
ui: add #js-flash skeleton for client-only error surfacing

Adds a hidden role="alert" element above the page content with
co-located scoped styles mirroring the .banner--error pairing. The
JS shim populates this via textContent on fetch failures (CSP / Trusted-
Types clean) so users see "Connection lost" instead of a silent reload.
Visible behavior is unchanged until the JS change in the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Wire `main.js` to populate `#js-flash` on network failure

**Files:**
- Modify: `ui/static/main.js:175-211` (the `submitForm` function)

- [ ] **Step 1: Replace the `submitForm` body**

Replace the entire `async function submitForm(e) { ... }` body (currently lines 175-211) with:

```js
async function submitForm(e) {
    // Clear any leftover client-only flash at the start of every submit;
    // a fresh action supersedes the prior failure state.
    const flash = document.getElementById('js-flash')
    if (flash) {
        flash.hidden = true
        flash.textContent = ''
    }

    const body = new URLSearchParams(e.formData)
    let res
    try {
        res = await fetch(e.destination.url, {
            method: 'POST',
            body,
            headers: {
                'Content-Type': 'application/x-www-form-urlencoded',
                'X-Requested-With': 'stacknav',
            },
            // Surface server-side redirects (e.g., a future auth bounce that
            // doesn't go through redirect()) as opaqueredirect responses
            // rather than transparently following them — fall through to the
            // unexpected-status branch below and reload to surface state.
            redirect: 'manual',
        })
    } catch (_) {
        // Client-side failure (offline, DNS, CORS). The server never saw the
        // request, so there is no flash to read on reload — and reload itself
        // may fail when offline. Surface inline via the pre-existing
        // role="alert" skeleton; textContent is CSP / Trusted-Types safe.
        if (flash) {
            flash.textContent = 'Connection lost. Check your network and try again.'
            flash.hidden = false
        }
        clearLoad()
        return
    }

    if (res.status === 200) {
        const target = res.headers.get('X-Location')
        if (!target) {
            location.reload()
            return
        }
        const replace = res.headers.get('X-Replace-Url') === 'true'
        await popOrPushTo(target, {replace})
        return
    }

    // CSP blocks document.write/innerHTML, so we can't render the response
    // body in place. Reload to surface the server state on any unexpected
    // status — the server's flash (set via app.userError) carries the message.
    location.reload()
}
```

Two semantic changes vs the prior body:
1. Clear `#js-flash` at the top of every submit so a subsequent action doesn't leave the previous error visible.
2. The `catch` branch populates `#js-flash` instead of reloading. Successful (200) and non-200 server responses still behave identically to today.

- [ ] **Step 2: Manually verify the network-failure path**

Run: `go run ./cmd/web/` (background). Note the port.
Open the app, log in, navigate to any form (e.g. preferences).
Devtools → Network → check "Offline".
Submit the form.
Expected: `#js-flash` appears at the top with the "Connection lost" message; the page does **not** reload; the form's inputs are preserved.

Uncheck "Offline", submit again.
Expected: `#js-flash` disappears at the start of the submit; the form processes normally and navigates as expected.

Restore network, kill the dev server.

- [ ] **Step 3: Run the full test suite to ensure no regressions**

Run: `make test` — Expected: PASS.
Run: `golangci-lint run --fix` — Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add ui/static/main.js
git commit -m "$(cat <<'EOF'
ui: surface fetch failures via #js-flash instead of blind reload

submitForm clears the #js-flash skeleton at the start of every submit,
and populates it via textContent on fetch throw (offline / DNS / CORS).
The non-2xx branch still reloads so the server-set flash from
app.userError surfaces through the banner component. The user keeps
their scroll position and form inputs on connection loss instead of
landing on the browser's offline page.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Update `cmd/web/CLAUDE.md` — three terminal calls

**Files:**
- Modify: `cmd/web/CLAUDE.md:117-156` (the "Error Handling" and "User-facing validation errors" sections)

- [ ] **Step 1: Read the current section bounds**

Run: `grep -n "^##\|^### " cmd/web/CLAUDE.md`
Confirm `## Error Handling` is followed by `### Error Response Patterns`, `### Service Layer Error Handling`, `### User-facing validation errors`, then `## Redirects and Navigation`. The new section replaces everything from `## Error Handling` through the end of `### User-facing validation errors`, leaving `## Redirects and Navigation` intact.

- [ ] **Step 2: Rewrite the section**

Replace lines 117-156 of `cmd/web/CLAUDE.md` (everything from `## Error Handling` through the end of the `### User-facing validation errors` block, just before `## Redirects and Navigation`) with:

```markdown
## Error Handling

### Three terminal calls

Every POST handler ends in exactly one of:

| Call | When | Effect |
|---|---|---|
| `redirect(w, r, url)` | Success | 200 + `X-Location` (stacknav) or 303 (plain); client navigates |
| `app.userError(w, r, err, safeURL)` | Any user-visible failure on an in-flight action | Routes by error type, calls `putFlashError`, then `redirect(w, r, safeURL)` |
| `app.serverError(w, r, err)` | True full-page failure (template render, broken session, no safe URL exists) | Logs and renders `error.gohtml` 500. Rare. |

`userError` is the single helper for *both* `domain.ValidationError` and unexpected
system errors on inline actions. It dispatches on the error type:

- `errors.As(err, &domain.ValidationError{})` → flash with `ve.Message` verbatim.
- Otherwise → log the underlying error at ERROR and flash a generic message.

Then it writes the flash and redirects to `safeURL`. The form's GET handler
pops the flash with `app.popFlashError(...)` and renders the `banner`
component as today — see the worked example in `workoutGET` /
`workoutAddExercisePOST`.

#### `safeURL` is mandatory, must pop + render the flash

The call site must pass a URL that is known to render successfully AND whose
handler pops + renders the flash banner. Today that means: `/`, `/workouts/{date}`
(both success and not-found branches), `/schedule`, `/admin/exercises`,
`/admin/exercises/{id}`. If you need a new target, plumb a `Flash BannerData`
field through its template data struct, render `{{ template "banner" .Flash }}`
in the template, and pop with `app.popFlashError(r.Context())` in the handler.

**Do not** default `safeURL` to `r.Referer()` (unreliable on direct POSTs,
easily forged) or to the request URL (wrong for action endpoints like
`POST /workouts/.../complete`, which would 404 on a GET). Pointing `safeURL`
at an action endpoint or another broken handler will produce a redirect loop.

#### Existing handlers may still use inline `errors.As(&ve)` boilerplate

> Go-forward convention for new and migrating handlers. Existing handlers
> predate `userError` and may still use the inline `errors.As(&ve) {
> putFlashError(ve.Message); redirect(formURL) }` pattern — that's fine,
> functionally equivalent, and they migrate opportunistically when next
> touched. Don't expect every form handler to call `userError` today.

#### Other patterns

- `http.NotFound(w, r)` (or `app.notFound(w, r)`) for 404s. Path-param
  parsers like `parseDateParam` call `notFound` for you on parse failure.
- `app.render(w, r, http.StatusNotFound, "workout-not-found", data)` for
  domain-level "no such resource" pages that want richer copy than the
  generic 404. These pages also pop + render the flash so `userError`
  redirects to them surface the message.

### Service Layer Error Handling

- Check for specific business errors using `errors.Is(err, domain.ErrNotFound)`.
- For user-actionable business failures, return a `domain.ValidationError`
  from the service / domain layer so `userError` can surface the message
  verbatim. Example: `internal/service/exercises.go:AddExercise` returns
  `domain.ValidationError{Message: "This day has no planned workout..."}`
  on the missing-session branch.
- For system faults the user cannot fix, return the wrapped underlying
  error and let `userError` log it and show the generic message.
- Let the service layer handle business validation; handlers handle HTTP
  concerns.

### Client-side error surface (`#js-flash`)

`base.gohtml` renders a hidden `<div id="js-flash" role="alert" hidden>` for
the JS shim to populate via `textContent` when `fetch` throws (offline / DNS
/ CORS). It is **only** for client-only failures — any server response (2xx
or not) still drives navigation or reload through the wire protocol, so the
flash-on-reload path stays canonical for server-originating messages.
```

- [ ] **Step 3: Sanity-check section boundaries are intact**

Run: `grep -n "## Redirects and Navigation" cmd/web/CLAUDE.md`
Expected: exactly one match.

Run: `grep -n "^##\|^### " cmd/web/CLAUDE.md` and visually confirm a clean heading hierarchy.

- [ ] **Step 4: Commit**

```bash
git add cmd/web/CLAUDE.md
git commit -m "$(cat <<'EOF'
docs(web): document three terminal calls and userError convention

Replaces the User-facing validation errors section with a unified Error
Handling section covering redirect / userError / serverError, the
ValidationError-vs-system routing inside userError, the safeURL
discipline (must point at a flash-rendering page), and a pointer to
#js-flash as the client-only surface. Existing inline errors.As
boilerplate stays valid as a go-forward convention.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Update `ui/templates/CLAUDE.md` — `#js-flash` note

**Files:**
- Modify: `ui/templates/CLAUDE.md` — append a one-paragraph note inside the existing "JavaScript & CSP (Trusted Types)" section.

- [ ] **Step 1: Add the note**

Find the paragraph in `ui/templates/CLAUDE.md` that ends with `Inline scripts must follow the same DOM-construction and script-URL rules — there's no relaxation for being inline.` (search for `relaxation for being inline`). Add the following new subsection directly after that paragraph, and before the next `## ` heading:

```markdown
### Client-only error surface (`#js-flash`)

`base.gohtml` renders a hidden `<div id="js-flash" role="alert" hidden>` above
the page content. The Stack Navigator shim populates it via `textContent` on
`fetch` failure (offline / DNS / CORS) — the standard "live region whose text
changes" pattern, announced by screen readers without focus moves. Use
`textContent` only (CSP / Trusted Types blocks `innerHTML`). The skeleton is
*not* for server-originating errors: those flow through `app.userError` →
flash + redirect → server-rendered `banner` component on the next GET. Keep
the client surface as a true last resort.
```

- [ ] **Step 2: Sanity-check heading hierarchy**

Run: `grep -n "^##\|^### " ui/templates/CLAUDE.md`
Expected: a clean heading hierarchy with the new `### Client-only error surface` heading inside the JavaScript section, not promoted accidentally.

- [ ] **Step 3: Commit**

```bash
git add ui/templates/CLAUDE.md
git commit -m "$(cat <<'EOF'
docs(templates): document #js-flash skeleton convention

Adds a note under the JavaScript & CSP section explaining that
#js-flash is the client-only error surface populated via textContent on
fetch failure, and that server-originating errors continue to flow
through the flash + banner reload path. Clarifies the role boundary so
future contributors don't reach for #js-flash for server messages.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Full validation sweep

**Files:** none (validation only).

- [ ] **Step 1: Run the full CI target**

Run: `make ci`
Expected: PASS. This runs init + build + lint-fix + test + sec — the project's full validation gate.

If anything fails, do **not** mark the task complete. Diagnose and fix; if the fix is in a file touched by an earlier task, add a fixup commit (do not rewrite history).

- [ ] **Step 2: Manual a11y smoke (documented for future runs)**

If a screen reader is available (NVDA on Windows, VoiceOver on macOS — `Cmd+F5` to toggle):

1. Trigger the unplanned-day add-exercise path manually (browser-based POST or any UI that lands there). Expect the screen reader to announce the banner text on the redirected `workout-not-found` page.
2. With devtools "Offline" toggled, submit any form. Expect the screen reader to announce the `#js-flash` "Connection lost..." text.

Document any issues found as new issues / tickets; do not fix in this PR unless trivial.

- [ ] **Step 3: No commit**

This task is verification only; no files change.

---

## Out of scope (do not include in this plan)

- Migrating other handlers from `errors.As(&ve)` boilerplate to `userError`. They keep working and migrate opportunistically when next touched.
- A JS-layer automated test for the network-failure path. The change is small and covered by the manual smoke step in Task 6 / Task 9.
- A toast component, per-field server-side error channel, or any changes to `error.gohtml`.
- Promoting the `banner` rendering from per-page `{{ template "banner" .Flash }}` calls into `base.gohtml`. Worth considering later but not required for this convention.
- Adding flash support to additional non-flash-aware pages (e.g. the home page). Only the redirect targets that handlers actually pass to `userError` need the plumbing; this plan only covers `workout-not-found` because it's the worked example's target.
