# Preferences Scroll-Restore and Per-Panel Flash Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every preferences-page form land the user inside the panel they were editing, with a success/error banner inside that panel — so submitting feels like a confirmed in-place edit, not a page reload.

**Architecture:** Server-rendered MPA pattern. Handlers redirect to `/preferences#<panel-id>`; browser snaps the panel into view; flash plumbing carries `{variant, message, anchor}`; the GET handler routes the entry into a per-panel banner slot. `POST /preferences` is split into single-purpose `/preferences/schedule` and `/preferences/deload` routes so each form picks its own redirect target. The stack-navigator shim is unchanged — `navigation.navigate()` honors fragments natively.

**Tech Stack:** Go stdlib `net/http`, `html/template`, `alexedwards/scs/v2` session manager (existing flash backing store), `PuerkitoBio/goquery` for tests, `e2etest` package for end-to-end HTTP-level testing.

**Spec:** `docs/superpowers/specs/2026-05-25-preferences-scroll-and-flash-design.md`

---

## File Structure

**New files:** none. All work modifies existing files.

**Modified files:**
- `cmd/web/helpers.go` — new typed flash plumbing (`flashEntry`, `putFlash`, `putFlashSuccess`, `putFlashErrorWithAnchor`, `popFlash`); remove `popFlashError`; keep `putFlashError` as a thin wrapper.
- `cmd/web/helpers_test.go` — update the two `popFlashError` test sites to use `popFlash().Message`.
- `cmd/web/handler-preferences.go` — split `preferencesPOST` into `preferencesScheduleSavePOST` and `preferencesDeloadSavePOST`; add success flashes to the three single-action POST handlers; route flash entries into `FlashByPanel` in `preferencesGET`.
- `cmd/web/handler-preferences_test.go` — migrate existing tests to new routes; add tests for redirect targets and per-panel flash.
- `cmd/web/handler-admin-exercises.go` — migrate `popFlashError` → `popFlash().Message`.
- `cmd/web/handler-error-ux.go` — migrate `popFlashError` → `popFlash().Message`.
- `cmd/web/handler-schedule.go` — migrate `popFlashError` → `popFlash().Message`.
- `cmd/web/handler-workout.go` — migrate `popFlashError` → `popFlash().Message` (three call sites).
- `cmd/web/routes.go` — drop `POST /preferences`; add `POST /preferences/schedule` and `POST /preferences/deload`.
- `ui/templates/pages/preferences/preferences.gohtml` — split forms by action URL, drop hidden cross-panel fields, insert per-panel banner template calls, add `scroll-margin-top` on `.panel-title`.

---

## Task 1: Add typed flash plumbing

Replace the string-only flash with a `{variant, message, anchor}` entry. Keep `putFlashError`'s signature so non-preferences call sites are unaffected.

**Files:**
- Modify: `cmd/web/helpers.go:115-125`
- Modify: `cmd/web/helpers_test.go` (two sites that call `popFlashError`)

- [ ] **Step 1: Write the failing test for the typed flash round-trip**

Append to `cmd/web/helpers_test.go`. Mirrors the existing `Test_userError_ValidationError_FlashesAndRedirects` setup pattern (uses `newTestSessionManager` + `sessionManager.Load`, not a real server):

```go
func TestPutFlashSuccess_PopFlashRoundtrip(t *testing.T) {
	t.Parallel()

	app := &application{ //nolint:exhaustruct // only sessionManager is touched.
		logger:         slog.New(slog.DiscardHandler),
		sessionManager: newTestSessionManager(t),
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx, err := app.sessionManager.Load(r.Context(), "")
	if err != nil {
		t.Fatalf("session load: %v", err)
	}

	app.putFlashSuccess(ctx, "Saved.", "deload-title")

	entry := app.popFlash(ctx)
	if entry.Variant != BannerVariantSuccess {
		t.Errorf("Variant = %q, want %q", entry.Variant, BannerVariantSuccess)
	}
	if entry.Message != "Saved." {
		t.Errorf("Message = %q, want %q", entry.Message, "Saved.")
	}
	if entry.Anchor != "deload-title" {
		t.Errorf("Anchor = %q, want %q", entry.Anchor, "deload-title")
	}
	if got := app.popFlash(ctx); got != (flashEntry{}) {
		t.Errorf("second pop should be empty, got %+v", got)
	}
}

func TestPutFlashError_LegacySignatureStillWorks(t *testing.T) {
	t.Parallel()

	app := &application{ //nolint:exhaustruct // only sessionManager is touched.
		logger:         slog.New(slog.DiscardHandler),
		sessionManager: newTestSessionManager(t),
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx, err := app.sessionManager.Load(r.Context(), "")
	if err != nil {
		t.Fatalf("session load: %v", err)
	}

	app.putFlashError(ctx, "Boom.")
	entry := app.popFlash(ctx)
	if entry.Variant != BannerVariantError || entry.Message != "Boom." || entry.Anchor != "" {
		t.Errorf("entry = %+v, want {error, Boom., \"\"}", entry)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/web -run TestPutFlashSuccess_PopFlashRoundtrip`
Expected: FAIL — `putFlashSuccess`, `popFlash`, and `flashEntry` are not defined.

- [ ] **Step 3: Implement the typed flash plumbing**

Replace `cmd/web/helpers.go:115-125` with:

```go
const flashKey = "flash"

// flashEntry is the session-backed flash payload. Variant is one of
// BannerVariantError, BannerVariantSuccess, BannerVariantInfo. Anchor is the
// id of the panel that should render the banner; empty Anchor means the page-
// top slot.
type flashEntry struct {
	Variant string
	Message string
	Anchor  string
}

// putFlash stores a typed flash entry in the session for the next page load.
func (app *application) putFlash(ctx context.Context, variant, message, anchor string) {
	app.sessionManager.Put(ctx, flashKey, flashEntry{
		Variant: variant,
		Message: message,
		Anchor:  anchor,
	})
}

// putFlashError is the legacy shim for the page-top error banner. Prefer
// putFlashErrorWithAnchor or putFlashSuccess for new code.
func (app *application) putFlashError(ctx context.Context, message string) {
	app.putFlash(ctx, BannerVariantError, message, "")
}

// putFlashErrorWithAnchor sets an error flash bound to a specific panel id.
func (app *application) putFlashErrorWithAnchor(ctx context.Context, message, anchor string) {
	app.putFlash(ctx, BannerVariantError, message, anchor)
}

// putFlashSuccess sets a success flash bound to a specific panel id.
// Pass an empty anchor for the page-top slot.
func (app *application) putFlashSuccess(ctx context.Context, message, anchor string) {
	app.putFlash(ctx, BannerVariantSuccess, message, anchor)
}

// popFlash retrieves and removes the flash entry from the session. Returns a
// zero-value flashEntry when nothing is stored.
func (app *application) popFlash(ctx context.Context) flashEntry {
	raw := app.sessionManager.Pop(ctx, flashKey)
	if raw == nil {
		return flashEntry{}
	}
	entry, ok := raw.(flashEntry)
	if !ok {
		return flashEntry{}
	}
	return entry
}
```

Delete the old `flashErrorKey` constant, the old `putFlashError`, and `popFlashError`. (We replaced `putFlashError`; the removed `popFlashError` is migrated in Step 5.)

`scs` requires Gob-registering any non-primitive value stored in the session. Add the registration call inside `initializeSessionManager` in `cmd/web/main.go:355-364`:

```go
func initializeSessionManager(dbs *sqlite.Database) *scs.SessionManager {
	gob.Register(flashEntry{})
	sessionManager := scs.New()
	sessionManager.Store = sqlite3store.NewWithCleanupInterval(dbs.ReadWrite, sessionStoreCleanupInterval)
	// ... rest unchanged.
}
```

Add `"encoding/gob"` to `cmd/web/main.go`'s import block. The test helper `newTestSessionManager` also needs the registration; update it in `cmd/web/helpers_test.go:20-25`:

```go
func newTestSessionManager(t *testing.T) *scs.SessionManager {
	t.Helper()
	gob.Register(flashEntry{})
	sm := scs.New()
	sm.Store = memstore.New()
	return sm
}
```

Add `"encoding/gob"` to `cmd/web/helpers_test.go`'s import block.

(Go's `gob.Register` is idempotent for identical types so calling it from both spots is safe.)

- [ ] **Step 4: Migrate `popFlashError` call sites to `popFlash`**

There are six call sites across handlers (excluding tests):

| File:line | Current | Replacement |
|---|---|---|
| `cmd/web/handler-admin-exercises.go:95` | `Message: app.popFlashError(r.Context()),` | inline-pop pattern (below) |
| `cmd/web/handler-admin-exercises.go:156` | same | same |
| `cmd/web/handler-error-ux.go:32` | `Message: app.popFlashError(r.Context()),` | same |
| `cmd/web/handler-preferences.go:131` | `Message: app.popFlashError(ctx),` | special — handled in Task 2 |
| `cmd/web/handler-schedule.go:37` | `Message: app.popFlashError(ctx),` | same as admin |
| `cmd/web/handler-workout.go:153` | `app.popFlashError(r.Context())` (passed as arg) | local-var pattern |
| `cmd/web/handler-workout.go:177` | same | same |
| `cmd/web/handler-workout.go:185` | same | same |

For each call site **except `handler-preferences.go:131`** (handled in Task 2), replace the `popFlashError(ctx)` call with a local pop and use the entry's fields:

```go
// Before:
Flash: BannerData{
    Variant: BannerVariantError,
    Message: app.popFlashError(ctx),
    Nonce:   base.Nonce,
},

// After:
flash := app.popFlash(ctx)
// ... later in the struct literal:
Flash: BannerData{
    Variant: flash.Variant,
    Message: flash.Message,
    Nonce:   base.Nonce,
},
```

For pages that haven't adopted per-panel slots (admin, schedule, workout, error-ux), the `flash.Anchor` is ignored — the banner falls through to the page-top slot regardless. Acceptable: those pages' handlers don't set anchored flashes, so anchor is always empty in practice.

`handler-workout.go` passes the popped string into `newWorkoutTemplateData(...)` and `newWorkoutNotFoundTemplateData(...)`. Don't change those helper signatures (out of scope) — just pop locally and pass `flash.Message`:

```go
// Before:
data := newWorkoutTemplateData(r, date, session, app.popFlashError(r.Context()))

// After:
flash := app.popFlash(r.Context())
data := newWorkoutTemplateData(r, date, session, flash.Message)
```

- [ ] **Step 5: Migrate the two `helpers_test.go` test sites**

Search for `popFlashError` in `cmd/web/helpers_test.go` (two hits, lines 238 and 274 in the snapshot). Replace each:

```go
// Before:
if got := app.popFlashError(r.Context()); got != "Name must be 1–50 characters." {

// After:
if got := app.popFlash(r.Context()).Message; got != "Name must be 1–50 characters." {
```

- [ ] **Step 6: Run all tests to verify the migration is green**

Run: `go test ./cmd/web/...`
Expected: all tests pass.

Run: `go test -v ./cmd/web -run TestPutFlashSuccess_PopFlashRoundtrip`
Expected: PASS.

- [ ] **Step 7: Run `make lint-fix` and verify it's clean**

Run: `make lint-fix`
Expected: no errors. The `exhaustruct` lint will complain if any new `BannerData{}` or `flashEntry{}` literals are partial — populate every field.

- [ ] **Step 8: Commit**

```bash
git add cmd/web/helpers.go cmd/web/helpers_test.go cmd/web/handler-admin-exercises.go cmd/web/handler-error-ux.go cmd/web/handler-schedule.go cmd/web/handler-workout.go cmd/web/main.go
git commit -m "$(cat <<'EOF'
web: introduce typed flash with variant + anchor

Replaces the string-only flash session payload with a flashEntry struct
carrying variant, message, and anchor. Adds putFlash, putFlashSuccess,
putFlashErrorWithAnchor, and popFlash helpers. Migrates the six existing
popFlashError call sites to popFlash; removes popFlashError. putFlashError
keeps its signature as a thin wrapper for non-anchored error flashes.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Add per-panel banner slot to preferences GET + template

Wire the GET handler to route a popped flash entry into either the page-top slot (empty anchor) or a per-panel slot (non-empty anchor). Add the per-panel banner template calls and the `scroll-margin-top` CSS so fragment snap doesn't crowd the heading.

No new handler behavior yet — the existing validation flash still has no anchor, so this task is observably a no-op. Task 3 starts populating anchors.

**Files:**
- Modify: `cmd/web/handler-preferences.go:99-137` (`preferencesGET` and the `preferencesTemplateData` struct)
- Modify: `ui/templates/pages/preferences/preferences.gohtml` (add three per-panel banner calls and one CSS rule)

- [ ] **Step 1: No test written for this task in isolation**

Populating `FlashByPanel` requires a handler that sets an anchored flash. No handler does that until Task 3 (the schedule validation flash) and Task 4 (the success flashes). Writing a test now would need a test-only seeding hook into the client's session, which is more code than the assertion it would cover. The per-panel rendering is verified end-to-end by Task 3's `TestPreferencesDeloadSave_RedirectsToDeloadAnchorWithSuccessFlash` and the migrated `TestPreferencesPOST_RejectsEmptySchedule` (Task 3 Step 7).

This task remains pure plumbing — committed after Steps 3-6 and verified by the existing preferences test suite (which must still pass: no anchored flash exists yet, so `FlashByPanel` stays empty and the page renders identically).

- [ ] **Step 2: (intentionally blank — no failing-test step for this task)**

- [ ] **Step 3: Add `FlashByPanel` to the template-data struct**

In `cmd/web/handler-preferences.go:37-51`, add the field:

```go
type preferencesTemplateData struct {
	BaseTemplateData

	Header                   PageHeaderData
	Weekdays                 []weekdayPreference
	DurationOptions          []workoutDurationOption
	VAPIDPublicKey           string
	PushSubscriptionCount    int
	RestNotificationsEnabled bool
	DeloadEnabled            bool
	MesocycleLength          int
	MesocycleLengthOptions   []int
	MesocycleAnchor          time.Time
	Flash                    BannerData
	FlashByPanel             map[string]BannerData
}
```

- [ ] **Step 4: Update `preferencesGET` to route the flash entry**

In `cmd/web/handler-preferences.go:99-137`, replace the `Flash:` literal in the `preferencesTemplateData{...}` block. The full updated handler body:

```go
func (app *application) preferencesGET(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	prefs, err := app.service.GetUserPreferences(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}
	subCount, err := app.service.CountPushSubscriptions(ctx)
	if err != nil {
		app.serverError(w, r, fmt.Errorf("count push subscriptions: %w", err))
		return
	}

	base := newBaseTemplateData(r)
	flash := app.popFlash(ctx)
	pageTopFlash := BannerData{Variant: "", Message: "", Nonce: base.Nonce}
	flashByPanel := map[string]BannerData{}
	if flash.Message != "" {
		bd := BannerData{Variant: flash.Variant, Message: flash.Message, Nonce: base.Nonce}
		if flash.Anchor == "" {
			pageTopFlash = bd
		} else {
			flashByPanel[flash.Anchor] = bd
		}
	}

	data := preferencesTemplateData{
		BaseTemplateData: base,
		Header: PageHeaderData{
			Title:    "Weekly Schedule",
			Subtitle: "Select the days you're planning to go to the gym",
			Nonce:    base.Nonce,
		},
		Weekdays:                 preferencesToWeekdays(prefs),
		DurationOptions:          getWorkoutDurationOptions(),
		VAPIDPublicKey:           app.vapidPublicKey,
		PushSubscriptionCount:    subCount,
		RestNotificationsEnabled: prefs.RestNotificationsEnabled,
		DeloadEnabled:            prefs.DeloadEnabled,
		MesocycleLength:          prefs.MesocycleLength,
		MesocycleLengthOptions:   []int{4, 5, 6, 7},
		MesocycleAnchor:          prefs.MesocycleAnchor,
		Flash:                    pageTopFlash,
		FlashByPanel:             flashByPanel,
	}

	app.render(w, r, http.StatusOK, "preferences", data)
}
```

- [ ] **Step 5: Add per-panel banner calls to the template**

In `ui/templates/pages/preferences/preferences.gohtml`, insert a `banner` template call inside each panel that owns a form, immediately after the `<header class="panel-head">…</header>` block.

**Schedule panel** (around line 351-380):

```gohtml
<form method="post" action="/preferences/schedule" class="panel" aria-labelledby="schedule-title">
    <header class="panel-head">
        <span class="panel-eyebrow"><span class="panel-eyebrow-num">01</span> Training week</span>
        <h2 class="panel-title" id="schedule-title">When are you in the gym?</h2>
        <p class="panel-blurb">Pick a session length for each day. Rest days are honored — no plan, no nag.</p>
    </header>

    {{ template "banner" (index $.FlashByPanel "schedule-title") }}

    <ul class="day-list">
    …
```

Note: the form `action` changes from `/preferences` to `/preferences/schedule` in this step (Task 3 wires the route; doing the template change here keeps Task 3 atomic to handler/route work). Until Task 3 lands, submitting the schedule form will 405 — that's acceptable mid-task because Task 2 and Task 3 commit together at the end of Task 3.

Actually — to keep tests green per-task, do NOT change form actions in this step. Insert only the `{{ template "banner" … }}` lines. Form actions move in Task 3.

**Schedule panel** insert:
```gohtml
<header class="panel-head">
    …
</header>

{{ template "banner" (index $.FlashByPanel "schedule-title") }}

<ul class="day-list">
```

**Notifications panel** (around line 382-424). Insert right after the `<header>` block:
```gohtml
<header class="panel-head">
    …
</header>

{{ template "banner" (index $.FlashByPanel "notif-title") }}

<div data-rest-notifications …>
```

**Recovery panel** (around line 585-637). Insert right after the `<header>` block:
```gohtml
<header class="panel-head">
    …
</header>

{{ template "banner" (index $.FlashByPanel "deload-title") }}

<form method="post" action="/preferences" class="stack">
```

- [ ] **Step 6: Add `scroll-margin-top` to panel headings**

Inside the existing `<style {{ $.Nonce }}>` block at the top of the page template, find the `.panel-title { … }` rule (around line 53-61) and append `scroll-margin-top: var(--size-5);`:

```css
.panel-title {
    font-family: 'Iowan Old Style', 'Palatino Linotype', 'Palatino',
                 'Book Antiqua', Georgia, serif;
    font-weight: var(--font-weight-7);
    font-size: var(--font-size-4);
    line-height: 1.15;
    color: var(--color-text-primary);
    letter-spacing: -0.005em;
    scroll-margin-top: var(--size-5);
}
```

- [ ] **Step 7: Run preferences tests to verify nothing broke**

Run: `go test -v ./cmd/web -run Test_application_preferences`
Expected: all existing preferences tests pass. The page renders identically because no handler sets an anchored flash yet — `FlashByPanel` is always empty.

- [ ] **Step 8: Commit**

```bash
git add cmd/web/handler-preferences.go ui/templates/pages/preferences/preferences.gohtml
git commit -m "$(cat <<'EOF'
preferences: add per-panel banner slot and scroll-margin

Adds FlashByPanel to preferencesTemplateData and routes any anchored
flash entry into the matching panel via the banner template. Page-top
slot continues to handle empty-anchor flashes. Adds scroll-margin-top
on .panel-title so fragment snap doesn't crowd the heading.

No observable behavior change yet — no handler sets an anchored flash
until the next commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Split `POST /preferences` into `/preferences/schedule` and `/preferences/deload`

Two new single-purpose handlers, two new routes, template form actions updated, old shared handler and route deleted. Tests for the new routes use TDD; existing tests get migrated in the same task to keep the test suite green.

**Files:**
- Modify: `cmd/web/handler-preferences.go` (delete `preferencesPOST`; add two new handlers)
- Modify: `cmd/web/routes.go:39` (replace one route with two)
- Modify: `ui/templates/pages/preferences/preferences.gohtml` (change two form actions, drop cross-panel hidden inputs)
- Modify: `cmd/web/handler-preferences_test.go` (migrate existing tests, add new ones)

- [ ] **Step 1: Write failing tests for the new routes**

First, add a local helper at the bottom of `cmd/web/handler-preferences_test.go` for raw shim POSTs (the e2etest `Client.SubmitForm` auto-follows redirects and parses HTML, so it can't observe `X-Location`):

```go
// postShimForm makes a raw POST with the stacknav shim header and a manual
// CheckRedirect so the X-Location header on the 200 response is observable.
// Returns the response; caller must close the body.
func postShimForm(
	t *testing.T,
	server *e2etest.Server,
	client *e2etest.Client,
	path string,
	fields neturl.Values,
) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		server.URL()+path, strings.NewReader(fields.Encode()))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "stacknav")

	httpClient := *client.HTTPClient() // shallow copy preserves the cookie jar.
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}
```

Add `neturl "net/url"` to the imports if not already present. The pattern mirrors `handler-workout_test.go:541-598`.

Now the two new tests:

```go
func TestPreferencesScheduleSave_RedirectsHomeWithoutFlash(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	client := server.Client()
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	resp := postShimForm(t, server, client, "/preferences/schedule", neturl.Values{
		"monday_minutes": []string{"60"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Location"); got != "/" {
		t.Errorf("X-Location = %q, want %q", got, "/")
	}
}

func TestPreferencesDeloadSave_RedirectsToDeloadAnchorWithSuccessFlash(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	client := server.Client()
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Seed a valid schedule so prefs is non-empty before saving deload.
	prefsDoc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc /preferences: %v", err)
	}
	if _, err = client.SubmitForm(ctx, prefsDoc,
		"/preferences/schedule", map[string]string{"monday_minutes": "60"}); err != nil {
		t.Fatalf("seed schedule: %v", err)
	}

	resp := postShimForm(t, server, client, "/preferences/deload", neturl.Values{
		"deload_enabled":   []string{"on"},
		"mesocycle_length": []string{"5"},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Location"); got != "/preferences#deload-title" {
		t.Errorf("X-Location = %q, want %q", got, "/preferences#deload-title")
	}

	// Follow with a GET and assert the banner lands inside the deload panel.
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc /preferences: %v", err)
	}
	panel := doc.Find("section[aria-labelledby='deload-title']")
	if panel.Length() == 0 {
		t.Fatal("deload-title panel not found")
	}
	banner := panel.Find(".banner--success")
	if banner.Length() == 0 {
		t.Fatal("success banner not rendered inside deload panel")
	}
	if got := strings.TrimSpace(banner.Text()); got != "Recovery settings saved." {
		t.Errorf("banner text = %q, want %q", got, "Recovery settings saved.")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./cmd/web -run "TestPreferencesScheduleSave|TestPreferencesDeloadSave"`
Expected: FAIL — routes don't exist yet (404 or routing to old handler).

- [ ] **Step 3: Implement `preferencesScheduleSavePOST`**

Replace `cmd/web/handler-preferences.go:139-179` (the existing `preferencesPOST`) with two handlers. First, the schedule handler:

```go
const scheduleAnchor = "schedule-title"

// preferencesScheduleSavePOST persists the weekday-minutes selection. On
// success, the user is redirected to home so they see the regenerated week.
func (app *application) preferencesScheduleSavePOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}

	prefs, err := app.service.GetUserPreferences(r.Context())
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}
	prefs.MondayMinutes = parseMinutes(r.Form.Get("monday_minutes"))
	prefs.TuesdayMinutes = parseMinutes(r.Form.Get("tuesday_minutes"))
	prefs.WednesdayMinutes = parseMinutes(r.Form.Get("wednesday_minutes"))
	prefs.ThursdayMinutes = parseMinutes(r.Form.Get("thursday_minutes"))
	prefs.FridayMinutes = parseMinutes(r.Form.Get("friday_minutes"))
	prefs.SaturdayMinutes = parseMinutes(r.Form.Get("saturday_minutes"))
	prefs.SundayMinutes = parseMinutes(r.Form.Get("sunday_minutes"))

	if prefs.IsEmpty() {
		app.putFlashErrorWithAnchor(r.Context(),
			"Please schedule at least one workout day.", scheduleAnchor)
		redirect(w, r, "/preferences#"+scheduleAnchor)
		return
	}

	if err = app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
		app.logger.LogAttrs(r.Context(), slog.LevelDebug, "preferences details", slog.Any("preferences", prefs))
		return
	}

	if err = app.service.RegenerateWeeklyPlanIfUnstarted(r.Context()); err != nil {
		// Preferences are saved; regeneration failure is not fatal because
		// ResolveWeeklySchedule on the home page will regenerate the plan automatically.
		app.logger.LogAttrs(r.Context(), slog.LevelWarn, "regenerate weekly plan after schedule save",
			slog.Any("error", err))
	}

	redirect(w, r, "/")
}
```

- [ ] **Step 4: Implement `preferencesDeloadSavePOST`**

Append below `preferencesScheduleSavePOST`:

```go
const deloadAnchor = "deload-title"

// preferencesDeloadSavePOST persists the deload-enable toggle and mesocycle
// length. On success, the user lands at the recovery panel with a success
// banner inside it.
func (app *application) preferencesDeloadSavePOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}

	prefs, err := app.service.GetUserPreferences(r.Context())
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get user preferences: %w", err))
		return
	}
	prefs.DeloadEnabled = r.Form.Get("deload_enabled") == "on"
	prefs.MesocycleLength = parseMesocycleLength(r.Form.Get("mesocycle_length"))

	if err = app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
		app.logger.LogAttrs(r.Context(), slog.LevelDebug, "preferences details", slog.Any("preferences", prefs))
		return
	}

	if err = app.service.RegenerateWeeklyPlanIfUnstarted(r.Context()); err != nil {
		app.logger.LogAttrs(r.Context(), slog.LevelWarn, "regenerate weekly plan after deload save",
			slog.Any("error", err))
	}

	app.putFlashSuccess(r.Context(), "Recovery settings saved.", deloadAnchor)
	redirect(w, r, "/preferences#"+deloadAnchor)
}
```

The old `preferencesPOST` function is fully replaced by these two — make sure there is no leftover definition.

- [ ] **Step 5: Update routes**

In `cmd/web/routes.go:39`, replace the single `POST /preferences` line with two new routes:

```go
mux.Handle("POST /preferences/schedule",
    app.mustSessionStack(http.HandlerFunc(app.preferencesScheduleSavePOST)))
mux.Handle("POST /preferences/deload",
    app.mustSessionStack(http.HandlerFunc(app.preferencesDeloadSavePOST)))
```

Delete the `POST /preferences` registration. `GET /preferences` (line 38) stays.

- [ ] **Step 6: Update template form actions and drop cross-panel hidden fields**

In `ui/templates/pages/preferences/preferences.gohtml`:

**Schedule form** (around line 351): change `action="/preferences"` to `action="/preferences/schedule"`. The form's body (weekday `<select>` controls) stays the same.

**Recovery form** (around line 592): change `action="/preferences"` to `action="/preferences/deload"`. Delete the seven hidden weekday inputs inside this form (currently around lines 612-614):

```gohtml
{{ range .Weekdays }}
    <input type="hidden" name="{{ .ID }}_minutes" value="{{ .Minutes }}">
{{ end }}
```

Delete that block entirely. The deload form now carries only `deload_enabled` and `mesocycle_length`. (The schedule form is unchanged structurally — its weekday selects already cover the schedule.)

- [ ] **Step 7: Migrate existing tests that POST to `/preferences`**

Five existing tests POST to `/preferences`. Update each to target the correct new route:

**`Test_application_preferences`** (cmd/web/handler-preferences_test.go:82) — the weekday update submission:
```go
// Before:
if doc, err = client.SubmitForm(ctx, doc, "/preferences", formData); err != nil {

// After:
if doc, err = client.SubmitForm(ctx, doc, "/preferences/schedule", formData); err != nil {
```

**`Test_application_preferencesPOST_preservesRestNotificationsEnabled`** (line 169) — same change:
```go
if _, err = client.SubmitForm(ctx, doc, "/preferences/schedule", map[string]string{
    "Monday": "60",
}); err != nil {
```

**`TestPreferencesPOST_RejectsEmptySchedule`** (line 456) — update to schedule route and update the post-submit URL assertion + banner location:
```go
doc, err = client.SubmitForm(ctx, doc, "/preferences/schedule", formData)
if err != nil {
    t.Fatalf("Failed to submit preferences form: %v", err)
}

// Should land at /preferences (the fragment is dropped by goquery's URL parse;
// the path is what matters for the assertion).
if doc.Url.Path != "/preferences" {
    t.Errorf("Expected to stay on /preferences, got %q", doc.Url.Path)
}

// Error banner now lives inside the schedule panel, not at page top.
schedulePanel := doc.Find("form[aria-labelledby='schedule-title']")
if schedulePanel.Length() == 0 {
    t.Fatal("schedule-title panel not found")
}
banner := schedulePanel.Find(".banner--error")
if banner.Length() == 0 {
    t.Fatal("Expected error banner to be rendered inside schedule panel")
}
if !strings.Contains(banner.Text(), "at least one workout day") {
    t.Errorf("Expected banner to contain 'at least one workout day', got %q", banner.Text())
}
```

**`Test_application_preferencesStartDeloadNow`** (line 543) — the enable-deload step submits the combined form. After the split, this becomes two submissions — one for schedule, one for deload:
```go
// Before:
if _, err = client.SubmitForm(ctx, doc, "/preferences", map[string]string{
    "monday_minutes":   "60",
    "deload_enabled":   "on",
    "mesocycle_length": "5",
}); err != nil {
    t.Fatalf("SubmitForm enable deload: %v", err)
}

// After:
if _, err = client.SubmitForm(ctx, doc, "/preferences/schedule", map[string]string{
    "monday_minutes": "60",
}); err != nil {
    t.Fatalf("SubmitForm save schedule: %v", err)
}
doc, err = client.GetDoc(ctx, "/preferences")
if err != nil {
    t.Fatalf("GetDoc /preferences after schedule save: %v", err)
}
if _, err = client.SubmitForm(ctx, doc, "/preferences/deload", map[string]string{
    "deload_enabled":   "on",
    "mesocycle_length": "5",
}); err != nil {
    t.Fatalf("SubmitForm enable deload: %v", err)
}
```

- [ ] **Step 8: Run tests to verify migration**

Run: `go test -v ./cmd/web -run "Test_application_preferences|TestPreferences"`
Expected: all tests pass, including the two new ones from Step 1.

- [ ] **Step 9: Run full suite + lint**

Run: `make ci` (or at minimum `go test ./... && make lint-fix`)
Expected: green.

- [ ] **Step 10: Commit**

```bash
git add cmd/web/handler-preferences.go cmd/web/handler-preferences_test.go cmd/web/routes.go ui/templates/pages/preferences/preferences.gohtml
git commit -m "$(cat <<'EOF'
preferences: split POST /preferences into /schedule and /deload

Each form on /preferences now POSTs to a single-purpose endpoint that
owns only its fields. /preferences/schedule keeps the empty-schedule
validation and redirects to / on success. /preferences/deload sets a
success flash anchored to the recovery panel and redirects to
/preferences#deload-title so the user lands inside the panel they were
editing.

POST /preferences is removed. Cross-panel hidden inputs in the recovery
form are dropped — the schedule form's selects own the weekday values.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire success flashes for the three single-action POST handlers

Add success flashes and fragment redirects to the notifications toggle, start-deload, and restart-mesocycle handlers.

**Files:**
- Modify: `cmd/web/handler-preferences.go` (three existing handlers)
- Modify: `cmd/web/handler-preferences_test.go` (extend existing tests + add new ones)

- [ ] **Step 1: Write failing tests**

These tests reuse the `postShimForm` helper added in Task 3 Step 1. Add to `cmd/web/handler-preferences_test.go`:

```go
func TestPreferencesRestNotificationsToggle_FlashAndAnchor(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	client := server.Client()
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Toggle OFF (defaults to true). Empty form => rest_notifications_enabled missing => false.
	resp := postShimForm(t, server, client, "/preferences/rest-notifications-toggle", neturl.Values{})
	resp.Body.Close()
	if got := resp.Header.Get("X-Location"); got != "/preferences#notif-title" {
		t.Errorf("X-Location = %q, want %q", got, "/preferences#notif-title")
	}

	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc: %v", err)
	}
	panel := doc.Find("section[aria-labelledby='notif-title']")
	if panel.Length() == 0 {
		t.Fatal("notif-title panel not found")
	}
	banner := panel.Find(".banner--success")
	if banner.Length() == 0 {
		t.Fatal("success banner missing in notifications panel")
	}
	if got := strings.TrimSpace(banner.Text()); got != "Rest pings disabled." {
		t.Errorf("banner text = %q, want %q", got, "Rest pings disabled.")
	}

	// Toggle back ON.
	resp = postShimForm(t, server, client, "/preferences/rest-notifications-toggle", neturl.Values{
		"rest_notifications_enabled": []string{"on"},
	})
	resp.Body.Close()
	doc, err = client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc: %v", err)
	}
	if got := strings.TrimSpace(doc.Find("section[aria-labelledby='notif-title'] .banner--success").Text()); got != "Rest pings enabled." {
		t.Errorf("banner text = %q, want %q", got, "Rest pings enabled.")
	}
}

func TestPreferencesStartDeloadNow_FlashAndAnchor(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	client := server.Client()
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Enable deload (requires a non-empty schedule first).
	prefsDoc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc /preferences: %v", err)
	}
	if _, err = client.SubmitForm(ctx, prefsDoc, "/preferences/schedule",
		map[string]string{"monday_minutes": "60"}); err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	prefsDoc, err = client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("re-fetch /preferences: %v", err)
	}
	if _, err = client.SubmitForm(ctx, prefsDoc, "/preferences/deload",
		map[string]string{"deload_enabled": "on", "mesocycle_length": "5"}); err != nil {
		t.Fatalf("enable deload: %v", err)
	}

	resp := postShimForm(t, server, client, "/preferences/mesocycle/start-deload-now", neturl.Values{})
	resp.Body.Close()
	if got := resp.Header.Get("X-Location"); got != "/preferences#deload-title" {
		t.Errorf("X-Location = %q, want %q", got, "/preferences#deload-title")
	}
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc: %v", err)
	}
	banner := doc.Find("section[aria-labelledby='deload-title'] .banner--success")
	if banner.Length() == 0 {
		t.Fatal("success banner missing in deload panel")
	}
	if got := strings.TrimSpace(banner.Text()); got != "Deload started for this week." {
		t.Errorf("banner text = %q, want %q", got, "Deload started for this week.")
	}
}

func TestPreferencesRestartMesocycle_FlashAndAnchor(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}
	client := server.Client()
	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Same enable-deload setup as TestPreferencesStartDeloadNow_FlashAndAnchor.
	prefsDoc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc /preferences: %v", err)
	}
	if _, err = client.SubmitForm(ctx, prefsDoc, "/preferences/schedule",
		map[string]string{"monday_minutes": "60"}); err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	prefsDoc, err = client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("re-fetch /preferences: %v", err)
	}
	if _, err = client.SubmitForm(ctx, prefsDoc, "/preferences/deload",
		map[string]string{"deload_enabled": "on", "mesocycle_length": "5"}); err != nil {
		t.Fatalf("enable deload: %v", err)
	}

	resp := postShimForm(t, server, client, "/preferences/mesocycle/restart", neturl.Values{})
	resp.Body.Close()
	if got := resp.Header.Get("X-Location"); got != "/preferences#deload-title" {
		t.Errorf("X-Location = %q, want %q", got, "/preferences#deload-title")
	}
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("GetDoc: %v", err)
	}
	banner := doc.Find("section[aria-labelledby='deload-title'] .banner--success")
	if banner.Length() == 0 {
		t.Fatal("success banner missing in deload panel")
	}
	if got := strings.TrimSpace(banner.Text()); got != "Cycle will restart next Monday." {
		t.Errorf("banner text = %q, want %q", got, "Cycle will restart next Monday.")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./cmd/web -run "TestPreferencesRestNotificationsToggle_Flash|TestPreferencesStartDeloadNow_Flash|TestPreferencesRestartMesocycle_Flash"`
Expected: FAIL — the handlers don't set flashes or fragments yet.

- [ ] **Step 3: Update `preferencesRestNotificationsTogglePOST`**

In `cmd/web/handler-preferences.go`, find `preferencesRestNotificationsTogglePOST` (current lines ~262-277) and replace its body:

```go
const notifAnchor = "notif-title"

func (app *application) preferencesRestNotificationsTogglePOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}
	prefs, err := app.service.GetUserPreferences(r.Context())
	if err != nil {
		app.serverError(w, r, fmt.Errorf("get preferences: %w", err))
		return
	}
	prefs.RestNotificationsEnabled = r.Form.Get("rest_notifications_enabled") == "on"
	if err = app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save preferences: %w", err))
		return
	}
	msg := "Rest pings disabled."
	if prefs.RestNotificationsEnabled {
		msg = "Rest pings enabled."
	}
	app.putFlashSuccess(r.Context(), msg, notifAnchor)
	redirect(w, r, "/preferences#"+notifAnchor)
}
```

(`notifAnchor` can live next to `scheduleAnchor` and `deloadAnchor` at the top of the file. Group all three together.)

- [ ] **Step 4: Update `preferencesStartDeloadNowPOST`**

Find `preferencesStartDeloadNowPOST` (current lines ~279-289) and update:

```go
func (app *application) preferencesStartDeloadNowPOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}
	if err := app.service.StartDeloadNow(r.Context()); err != nil {
		app.serverError(w, r, fmt.Errorf("start deload now: %w", err))
		return
	}
	app.putFlashSuccess(r.Context(), "Deload started for this week.", deloadAnchor)
	redirect(w, r, "/preferences#"+deloadAnchor)
}
```

- [ ] **Step 5: Update `preferencesRestartMesocyclePOST`**

Find `preferencesRestartMesocyclePOST` (current lines ~251-260) and update:

```go
func (app *application) preferencesRestartMesocyclePOST(w http.ResponseWriter, r *http.Request) {
	if !app.parseForm(w, r, defaultMaxFormSize) {
		return
	}
	if err := app.service.RestartMesocycleAnchor(r.Context()); err != nil {
		app.serverError(w, r, fmt.Errorf("restart mesocycle: %w", err))
		return
	}
	app.putFlashSuccess(r.Context(), "Cycle will restart next Monday.", deloadAnchor)
	redirect(w, r, "/preferences#"+deloadAnchor)
}
```

- [ ] **Step 6: Run the new tests**

Run: `go test -v ./cmd/web -run "TestPreferencesRestNotificationsToggle_Flash|TestPreferencesStartDeloadNow_Flash|TestPreferencesRestartMesocycle_Flash"`
Expected: PASS.

- [ ] **Step 7: Run full suite + lint**

Run: `make ci` (or `go test ./... && make lint-fix`)
Expected: green.

- [ ] **Step 8: Commit**

```bash
git add cmd/web/handler-preferences.go cmd/web/handler-preferences_test.go
git commit -m "$(cat <<'EOF'
preferences: surface success flash on rest-toggle, start-deload, restart

The three single-action handlers now set an anchored success flash and
redirect to /preferences#<panel-id> so the user lands inside the panel
with a visible confirmation. Rest-toggle picks its message from the
resulting state ('enabled' / 'disabled'); start-deload and restart-cycle
use the spec's fixed copy.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Manual verification in the browser

The behavior is mostly browser-driven (fragment snap, scroll-margin). Confirm in a real browser before declaring done.

**Files:** none.

- [ ] **Step 1: Start the dev server**

Run: `make init && go run ./cmd/web`
Open: `http://localhost:8080/preferences`

- [ ] **Step 2: Scroll to the Recovery panel and submit each form**

For each of:
- Save recovery settings (after toggling the checkbox and changing cycle length)
- Start deload this week
- Restart cycle next Monday

…verify:
1. Page redirects with a fragment in the URL bar (`/preferences#deload-title`).
2. Recovery panel heading is at the top of the viewport, with `var(--size-5)` of breathing room above it.
3. Green success banner is visible inside the Recovery panel between the heading and the form controls.

- [ ] **Step 3: Submit the Notifications toggle**

Toggle the checkbox; verify URL gains `#notif-title`, panel snaps into view, banner shows "Rest pings enabled." / "Rest pings disabled.".

- [ ] **Step 4: Submit an empty Save week**

Set every day to "Rest day" and submit; verify:
1. URL gains `#schedule-title`.
2. Schedule panel is at the top of viewport.
3. Red error banner reads "Please schedule at least one workout day." inside the schedule panel (not at page top).

- [ ] **Step 5: Submit a valid Save week**

Verify redirect lands at `/` (home page), no fragment, no orphan flash on the next `/preferences` visit (the home page does not render `FlashByPanel` and the schedule save does not set a flash, so the next preferences visit should be flash-free).

- [ ] **Step 6: Browser-back from `/` to `/preferences`**

After a successful schedule save lands on home, click browser back. Confirm the preferences page restores at the scroll position the user had before submit (browser-native scroll restoration). No banner appears (the flash was never set for the schedule path).

- [ ] **Step 7: Commit any incidental fixes**

If Step 2-6 surface bugs (off-by-one CSS, wrong banner color, etc.), fix and commit separately. If everything works, no commit needed for this task.

---

## Self-Review (run before declaring complete)

**Spec coverage:**

- [x] Per-form redirect targets table → Tasks 3 and 4 set every redirect.
- [x] Split `POST /preferences` → Task 3.
- [x] CSS `scroll-margin-top` → Task 2 Step 6.
- [x] Flash plumbing extension (`flashEntry`, helpers, key rename) → Task 1.
- [x] Per-panel banner rendering (template data field, GET routing, template calls) → Task 2.
- [x] Handler wiring with per-form messages → Tasks 3 and 4.
- [x] Stack-navigator: no changes → confirmed; no shim work in any task.
- [x] Acceptance criteria — every bullet maps to a test in Tasks 3-4 or a manual check in Task 5.
- [x] Testing section — `e2etest` patterns, `X-Location` assertion via shim header, panel-scoped banner selector.
- [x] Out-of-scope items — none re-introduced.

**Placeholder scan:** none found. Every step has either exact code or an exact command.

**Type consistency:**
- `flashEntry{Variant, Message, Anchor string}` — used consistently across Task 1, 2, 3, 4.
- `BannerData{Variant, Message string, Nonce template.HTMLAttr}` — unchanged from current code.
- Handler names: `preferencesScheduleSavePOST`, `preferencesDeloadSavePOST` — consistent across Task 3 and 4.
- Anchor constants: `scheduleAnchor`, `deloadAnchor`, `notifAnchor` — defined in Task 3 (schedule, deload) and Task 4 (notif).

**Verified against the codebase before writing:**
- `e2etest.Client` does not expose a "raw POST with X-Location" helper. Task 3 Step 1 adds a local `postShimForm` helper in `cmd/web/handler-preferences_test.go` modeled on `handler-workout_test.go:541-598`. Task 4 reuses it.
- `newTestSessionManager` in `cmd/web/helpers_test.go:20-25` is the right scaffolding for Task 1's flash round-trip tests — no real server needed.
- `gob.Register(flashEntry{})` goes inside `initializeSessionManager` in `cmd/web/main.go:355-364`, plus inside `newTestSessionManager` so tests using the in-memory store can round-trip too. `gob.Register` is idempotent for identical types, so double-registration is safe.

---

**Plan complete and saved to `docs/superpowers/plans/2026-05-25-preferences-scroll-and-flash.md`. Two execution options:**

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
