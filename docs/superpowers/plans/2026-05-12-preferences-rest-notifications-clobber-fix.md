# Preferences Rest-Notifications Clobber Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop `preferencesPOST` from silently overwriting `rest_notifications_enabled` to `false` every time a user saves their weekday schedule.

**Architecture:** Replace the partial struct literal in `preferencesPOST` with a read-modify-write that mirrors `preferencesRestNotificationsTogglePOST`. Delete the `weekdaysToPreferences` helper and the `//nolint:exhaustruct` directive that was hiding the bug. Field-by-field assignment makes the bug unrepresentable.

**Tech Stack:** Go stdlib, project's existing `internal/e2etest` test harness, goquery.

**Spec:** `docs/superpowers/specs/2026-05-12-preferences-rest-notifications-clobber-fix-design.md`

---

## Task 1: Add failing regression test + apply fix

The fix and test ship in a single commit per the design ("one — the fix, the test, and the nolint removal are all the same concern"). Test goes in first so we can demonstrate red→green inside the working tree before committing.

**Files:**
- Modify: `cmd/web/handler-preferences_test.go` — add `Test_application_preferencesPOST_preservesRestNotificationsEnabled` after `Test_application_preferences` (around line 131)
- Modify: `cmd/web/handler-preferences.go` — rewrite `preferencesPOST` (lines 114-137), delete `weekdaysToPreferences` (lines 77-87)

---

- [ ] **Step 1: Add the failing regression test**

Append this test to `cmd/web/handler-preferences_test.go` immediately after `Test_application_preferences` (right before `Test_application_exportUserData`):

```go
// Regression test for the rest-timer hotfix: submitting the weekday form must
// not flip rest_notifications_enabled back to false. The bug was a partial
// domain.Preferences{} literal in preferencesPOST that defaulted the column
// to Go's zero-value and clobbered the persisted true.
func Test_application_preferencesPOST_preservesRestNotificationsEnabled(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// Fresh users default to rest_notifications_enabled = true. Confirm the
	// checkbox is rendered checked so the test's premise is solid.
	doc, err := client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Failed to get preferences: %v", err)
	}
	checkbox := doc.Find("input[name='rest_notifications_enabled']")
	if checkbox.Length() == 0 {
		t.Fatal("Expected rest_notifications_enabled checkbox to be rendered")
	}
	if _, checked := checkbox.Attr("checked"); !checked {
		t.Fatal("Expected rest_notifications_enabled checkbox to start checked (default true)")
	}

	// Submit the weekday-schedule form. Pre-fix this clobbered the column.
	if _, err = client.SubmitForm(ctx, doc, "/preferences", map[string]string{
		"Monday": "60",
	}); err != nil {
		t.Fatalf("Failed to submit weekday form: %v", err)
	}

	// Re-fetch and assert the checkbox is still checked.
	doc, err = client.GetDoc(ctx, "/preferences")
	if err != nil {
		t.Fatalf("Failed to re-get preferences: %v", err)
	}
	checkbox = doc.Find("input[name='rest_notifications_enabled']")
	if checkbox.Length() == 0 {
		t.Fatal("Expected rest_notifications_enabled checkbox to be rendered after submit")
	}
	if _, checked := checkbox.Attr("checked"); !checked {
		t.Error("rest_notifications_enabled was cleared by weekday-form submit; should be preserved")
	}
}
```

- [ ] **Step 2: Run the new test and verify it fails**

Run: `go test -v ./cmd/web -run Test_application_preferencesPOST_preservesRestNotificationsEnabled`

Expected: FAIL on the second `checked` assertion ("rest_notifications_enabled was cleared by weekday-form submit; should be preserved"). The first checkbox-present and first-checked assertions should pass — the failure must come from the post-submit re-fetch, otherwise the test isn't actually exercising the bug.

If the test fails for a different reason (e.g. checkbox not found at all), stop and investigate before proceeding — the template structure may have shifted and the selector needs adjusting.

- [ ] **Step 3: Apply the handler fix**

Open `cmd/web/handler-preferences.go`. Two edits:

**Edit 3a:** Delete the `weekdaysToPreferences` function entirely (lines 77-87 in the current file):

```go
func weekdaysToPreferences(r *http.Request) domain.Preferences {
	return domain.Preferences{ //nolint:exhaustruct // RestNotificationsEnabled handled by separate endpoint.
		MondayMinutes:    parseMinutes(r.Form.Get("monday_minutes")),
		TuesdayMinutes:   parseMinutes(r.Form.Get("tuesday_minutes")),
		WednesdayMinutes: parseMinutes(r.Form.Get("wednesday_minutes")),
		ThursdayMinutes:  parseMinutes(r.Form.Get("thursday_minutes")),
		FridayMinutes:    parseMinutes(r.Form.Get("friday_minutes")),
		SaturdayMinutes:  parseMinutes(r.Form.Get("saturday_minutes")),
		SundayMinutes:    parseMinutes(r.Form.Get("sunday_minutes")),
	}
}
```

**Edit 3b:** Rewrite the body of `preferencesPOST` (lines 114-137). The new version reads current prefs, mutates the weekday fields by direct assignment, and saves. Mirrors the read-modify-write pattern in `preferencesRestNotificationsTogglePOST` (lines 209-225).

Replace:

```go
func (app *application) preferencesPOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	prefs := weekdaysToPreferences(r)

	if err := app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
		app.logger.LogAttrs(r.Context(), slog.LevelDebug, "preferences details", slog.Any("preferences", prefs))
		return
	}

	if err := app.service.RegenerateWeeklyPlanIfUnstarted(r.Context()); err != nil {
		// Preferences are already saved; regeneration failure is not fatal because
		// ResolveWeeklySchedule on the home page will regenerate the plan automatically.
		app.logger.LogAttrs(r.Context(), slog.LevelWarn, "regenerate weekly plan after preference save",
			slog.Any("error", err))
	}

	redirect(w, r, "/")
}
```

with:

```go
func (app *application) preferencesPOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
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

	if err = app.service.SaveUserPreferences(r.Context(), prefs); err != nil {
		app.serverError(w, r, fmt.Errorf("save user preferences: %w", err))
		app.logger.LogAttrs(r.Context(), slog.LevelDebug, "preferences details", slog.Any("preferences", prefs))
		return
	}

	if err = app.service.RegenerateWeeklyPlanIfUnstarted(r.Context()); err != nil {
		// Preferences are already saved; regeneration failure is not fatal because
		// ResolveWeeklySchedule on the home page will regenerate the plan automatically.
		app.logger.LogAttrs(r.Context(), slog.LevelWarn, "regenerate weekly plan after preference save",
			slog.Any("error", err))
	}

	redirect(w, r, "/")
}
```

Note: `err` is now declared with `:=` once and reused with `=` in the two subsequent error-returning calls — matches the project convention (CLAUDE.md: "govet shadow is often fixed by reusing the earlier `err` variable").

After both edits, the `weekdaysToPreferences` symbol and the `nolint:exhaustruct` are gone. The only remaining references to weekday-form parsing live inside `preferencesPOST` itself.

- [ ] **Step 4: Run the regression test and verify it now passes**

Run: `go test -v ./cmd/web -run Test_application_preferencesPOST_preservesRestNotificationsEnabled`

Expected: PASS.

- [ ] **Step 5: Run the existing preferences test to confirm no regression**

Run: `go test -v ./cmd/web -run Test_application_preferences$`

Expected: PASS. The existing test (which exercises weekday save + persistence round-trip) must still pass. If it doesn't, the field-assignment order or the form-field names (`monday_minutes`, `tuesday_minutes`, …) drifted from what the form actually posts — re-check against the template.

- [ ] **Step 6: Run `make ci` and verify lint + full test suite pass**

Run: `make ci`

Expected: PASS, including `exhaustruct` running clean without the deleted `nolint` directive. If `exhaustruct` complains about a different site, that's pre-existing and out of scope — note it for follow-up but don't fix here.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-preferences.go cmd/web/handler-preferences_test.go
git -c commit.gpgsign=false commit -m "$(cat <<'EOF'
Fix preferencesPOST clobbering rest_notifications_enabled

Submitting the weekday schedule built a partial domain.Preferences{}
literal that defaulted RestNotificationsEnabled to Go's zero-value
false; SaveUserPreferences then upserted that false over the user's
real value. Affected every authenticated user who touched the weekday
form after subscribing to rest notifications.

Replace the partial-literal helper with a read-modify-write in
preferencesPOST, mirroring preferencesRestNotificationsTogglePOST.
Delete weekdaysToPreferences (its only caller) and remove the
nolint:exhaustruct directive that was hiding the bug -- exhaustruct
was correctly flagging the partial literal as data-losing.

Adds a regression test that fails on the pre-fix code and passes
after.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Expected: commit succeeds, `git status` clean. If the pre-commit hook fails, fix the underlying issue and create a new commit (never `--amend`).

---

## Self-review notes

- Spec coverage: every requirement in the spec is exercised. "Fix" → Steps 3a/3b. "Regression test" → Steps 1/2/4. "Acceptance: make ci passes" → Step 6. "Out of scope" sections are not implemented.
- Placeholder scan: no TBDs, no "add error handling" stubs, no "similar to Task N" references — every code block is the actual code.
- Type consistency: `domain.Preferences` field names match the existing struct; `parseMinutes` signature unchanged; `app.service.GetUserPreferences` already exists (used in the toggle handler at line 215 and in `preferencesGET` at line 91); no new types or methods introduced.
