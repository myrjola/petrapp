# Admin Nav Scaffold Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give admins a discoverable entry into admin tooling from the preferences menu, and let them move between admin sections in one click via a shared horizontal sub-nav.

**Architecture:** A new `admin-nav` partial template, rendered at the top of each admin page, exposes both admin sections as tabs and highlights the active one via `aria-current="page"` + an underline accent. A tiny `/admin` redirect handler keeps the preferences entry stable across future section additions. The preferences page grows a single admin-only panel (gated on `BaseTemplateData.IsAdmin`).

**Tech Stack:** Go stdlib `net/http`, `html/template`, `@scope` scoped CSS, `PuerkitoBio/goquery` for tests, `e2etest` package for end-to-end HTTP-level testing.

**Spec:** `docs/superpowers/specs/2026-05-27-admin-nav-scaffold-design.md`

---

## File Structure

**New files:**
- `cmd/web/handler-admin.go` — `adminGET` (302/303 redirect from `/admin` to `/admin/exercises`).
- `cmd/web/handler-admin_test.go` — auth + redirect tests for `GET /admin`.
- `ui/templates/components/admin-nav.gohtml` — sub-nav partial. Renders a `<nav aria-label="Admin sections">` with two tab links; highlights the active tab via `aria-current="page"` and an `::after` underline.

**Modified files:**
- `cmd/web/components.go` — add `AdminNavData` struct and `adminSectionExercises` / `adminSectionFeatureFlags` constants.
- `cmd/web/routes.go` — register `GET /admin` under `app.mustAdminStack()`.
- `cmd/web/handler-admin-exercises.go` — add `AdminNav AdminNavData` field to `exerciseAdminTemplateData` and `exerciseEditTemplateData`; populate in `adminExercisesGET` and `adminExerciseEditGET`.
- `cmd/web/handler-admin-exercises_test.go` — assert sub-nav is present with "Exercises" tab active on both list and edit pages.
- `cmd/web/handler-admin-feature-flags.go` — add `AdminNav AdminNavData` field to `featureFlagsAdminTemplateData`; populate in `adminFeatureFlagsGET`.
- `cmd/web/handler-admin-feature-flags_test.go` — assert sub-nav is present with "Feature Flags" tab active.
- `cmd/web/handler-preferences_test.go` — assert admin panel and `/admin` link appear for admin users and are absent for non-admin users.
- `cmd/web/handler-styleguide.go` — add `AdminNavExamples` field to `styleguideTemplateData` and a `styleguideAdminNavExamples` factory.
- `cmd/web/handler-styleguide_test.go` — assert the styleguide renders the `admin-nav` component in both states.
- `ui/templates/pages/admin-exercises/admin-exercises.gohtml` — include `{{ template "admin-nav" .AdminNav }}` after `page-header`.
- `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml` — same.
- `ui/templates/pages/admin-feature-flags/admin-feature-flags.gohtml` — same.
- `ui/templates/pages/styleguide/styleguide.gohtml` — add an Admin nav section that renders both example states.
- `ui/templates/pages/preferences/preferences.gohtml` — add admin panel (numbered `05`) between the Your-account panel and the danger-zone, gated by `{{ if $.IsAdmin }}`.

---

## Task 1: Add `AdminNavData` struct and section constants

Adds the dot-struct that the partial (built in Task 2) will consume, plus the section-name constants used by every admin handler. Nothing renders yet — the struct exists so the partial in Task 2 has a type to bind via the `gotype` comment.

**Files:**
- Modify: `cmd/web/components.go`

- [ ] **Step 1: Add the struct and constants**

Append to `cmd/web/components.go` after the existing `ExerciseSearchData` block (last struct in the file):

```go
// AdminNavData is the dot for the `admin-nav` component. Active is the
// section-name string ("exercises" or "feature-flags") used to mark the
// matching tab with aria-current="page".
type AdminNavData struct {
	Active string
	Nonce  template.HTMLAttr
}

// Admin section names accepted by AdminNavData.Active. Keep aligned with
// the values the admin-nav template branches on.
const (
	adminSectionExercises    = "exercises"
	adminSectionFeatureFlags = "feature-flags"
)
```

- [ ] **Step 2: Verify build**

Run: `go build ./cmd/web`
Expected: PASS (compiles, no template wired yet — the new identifiers are unused, which Go tolerates for package-level declarations).

- [ ] **Step 3: Commit**

```bash
git add cmd/web/components.go
git commit -m "feat(web): add AdminNavData and admin section constants"
```

---

## Task 2: Create the `admin-nav` partial and register it in the styleguide

Adds the partial template and a styleguide entry. The styleguide test is the failing test that drives the partial's existence — `admin-nav` must render correctly for the test to pass.

**Files:**
- Create: `ui/templates/components/admin-nav.gohtml`
- Modify: `cmd/web/handler-styleguide.go`
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml`
- Modify: `cmd/web/handler-styleguide_test.go`

- [ ] **Step 1: Write the failing styleguide test**

Append to `cmd/web/handler-styleguide_test.go` before the closing `}` of `Test_application_styleguide`:

```go
	// Admin nav component — both states render on the styleguide.
	if doc.Find("h2:contains('Admin nav')").Length() == 0 {
		t.Error("expected an 'Admin nav' section on the styleguide")
	}
	if doc.Find(".admin-nav").Length() < 2 {
		t.Errorf("expected at least two .admin-nav examples on the styleguide, got %d",
			doc.Find(".admin-nav").Length())
	}
	if doc.Find(`.admin-nav .admin-nav__tab[aria-current="page"][href="/admin/exercises"]`).Length() == 0 {
		t.Error("expected an Exercises-active example with aria-current=page")
	}
	if doc.Find(`.admin-nav .admin-nav__tab[aria-current="page"][href="/admin/feature-flags"]`).Length() == 0 {
		t.Error("expected a Feature-Flags-active example with aria-current=page")
	}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: FAIL — the styleguide page does not have an "Admin nav" section or `.admin-nav` markup yet.

- [ ] **Step 3: Create the partial template**

Create `ui/templates/components/admin-nav.gohtml`:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.AdminNavData*/ -}}
{{ define "admin-nav" }}
<style {{ .Nonce }}>
    @scope (.admin-nav) {
        :scope {
            display: flex;
            flex-wrap: wrap;
            gap: var(--size-2);
            margin-block-end: var(--size-4);
        }

        .admin-nav__tab {
            position: relative;
            display: inline-flex;
            align-items: center;
            min-height: 3rem;
            padding-inline: var(--size-3);
            padding-block: var(--size-2);
            border-radius: var(--radius-2);
            background: var(--color-surface-elevated);
            color: var(--color-text-secondary);
            text-decoration: none;
            font-weight: 500;
        }

        .admin-nav__tab[aria-current="page"] {
            background: var(--color-surface-active);
            color: var(--color-text-primary);
            font-weight: 600;
        }

        .admin-nav__tab[aria-current="page"]::after {
            content: "";
            position: absolute;
            inset-inline: var(--size-3);
            inset-block-end: -2px;
            block-size: 2px;
            background: var(--color-text-primary);
        }

        .admin-nav__tab:focus-visible {
            outline: 2px solid var(--color-text-primary);
            outline-offset: 2px;
        }
    }
</style>
<nav class="admin-nav" aria-label="Admin sections">
    <a class="admin-nav__tab" href="/admin/exercises"
       {{ if eq .Active "exercises" }}aria-current="page"{{ end }}>
        Exercises
    </a>
    <a class="admin-nav__tab" href="/admin/feature-flags"
       {{ if eq .Active "feature-flags" }}aria-current="page"{{ end }}>
        Feature Flags
    </a>
</nav>
{{ end }}
```

- [ ] **Step 4: Wire the partial into the styleguide data**

In `cmd/web/handler-styleguide.go`, add the field to `styleguideTemplateData` (after `FieldExamples`):

```go
	// AdminNavExamples drives the Admin nav section of the styleguide.
	AdminNavExamples []AdminNavData
```

In the same file, populate it inside `styleguideGET` (insert after the existing `FieldExamples:` line):

```go
		AdminNavExamples:  styleguideAdminNavExamples(base.Nonce),
```

Append a new helper at the bottom of the file:

```go
// styleguideAdminNavExamples returns the admin-nav examples demoed on
// the styleguide page — one render per active section.
func styleguideAdminNavExamples(nonce template.HTMLAttr) []AdminNavData {
	return []AdminNavData{
		{Active: adminSectionExercises, Nonce: nonce},
		{Active: adminSectionFeatureFlags, Nonce: nonce},
	}
}
```

- [ ] **Step 5: Render the partial in the styleguide template**

In `ui/templates/pages/styleguide/styleguide.gohtml`, add a new section between the existing `Field` section and the `Components` section (after line 423, before the `<section>` that begins with `<h2>Components</h2>`):

```gohtml
        <section>
            <h2>Admin nav</h2>
            <div class="component-sample">
                <div class="stack">
                    {{ range .AdminNavExamples }}
                        {{ template "admin-nav" . }}
                    {{ end }}
                </div>
            </div>
        </section>
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add ui/templates/components/admin-nav.gohtml cmd/web/handler-styleguide.go \
        ui/templates/pages/styleguide/styleguide.gohtml cmd/web/handler-styleguide_test.go
git commit -m "feat(web): add admin-nav partial and styleguide entry"
```

---

## Task 3: Wire `admin-nav` into the admin-exercises pages

Adds the `AdminNav` field to both exercises template-data structs, populates it in both handlers, and includes the partial in both templates. Drives via tests asserting the active tab on the list and edit pages.

**Files:**
- Modify: `cmd/web/handler-admin-exercises.go`
- Modify: `cmd/web/handler-admin-exercises_test.go`
- Modify: `ui/templates/pages/admin-exercises/admin-exercises.gohtml`
- Modify: `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml`

- [ ] **Step 1: Write the failing test**

Two assertions go into the existing `Test_application_adminExercises` function in `cmd/web/handler-admin-exercises_test.go`.

**(a)** Inside the existing `t.Run("View exercises admin page", ...)` subtest (around line 66), append after the form-existence check (around line 84):

```go
		// Admin nav renders with Exercises marked active.
		nav := doc.Find(`nav.admin-nav[aria-label="Admin sections"]`)
		if nav.Length() == 0 {
			t.Fatal("expected admin-nav landmark on /admin/exercises")
		}
		if nav.Find(`a.admin-nav__tab[href="/admin/exercises"][aria-current="page"]`).Length() == 0 {
			t.Error("expected Exercises tab to be marked aria-current=page")
		}
		if nav.Find(`a.admin-nav__tab[href="/admin/feature-flags"][aria-current="page"]`).Length() != 0 {
			t.Error("Feature Flags tab must not be active on the exercises page")
		}
```

**(b)** Inside the existing `t.Run("Create new exercise", ...)` subtest (around line 88), append after the heading assertion (around line 110). At that point in the flow, `doc` holds the edit page that the form submission redirected to:

```go
		// The edit page also renders admin-nav with Exercises active.
		editNav := doc.Find(`nav.admin-nav[aria-label="Admin sections"]`)
		if editNav.Length() == 0 {
			t.Fatal("expected admin-nav landmark on the exercise edit page")
		}
		if editNav.Find(`a.admin-nav__tab[href="/admin/exercises"][aria-current="page"]`).Length() == 0 {
			t.Error("expected Exercises tab to be marked aria-current=page on the edit page")
		}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_adminExercises`
Expected: FAIL — no `nav.admin-nav` rendered on either page.

- [ ] **Step 3: Add `AdminNav` field to the template-data structs**

In `cmd/web/handler-admin-exercises.go`, add the field to `exerciseAdminTemplateData` (declared around line 26):

```go
type exerciseAdminTemplateData struct {
	BaseTemplateData

	Header    PageHeaderData
	Flash     BannerData
	AdminNav  AdminNavData
	Exercises []adminExerciseRow
	NameField FieldData
}
```

And to `exerciseEditTemplateData` (declared around line 50):

```go
type exerciseEditTemplateData struct {
	BaseTemplateData

	Header                 PageHeaderData
	Flash                  BannerData
	AdminNav               AdminNavData
	Exercise               domain.Exercise
	NameField              FieldData
	SecondsField           FieldData
	RepMinField            FieldData
	RepMaxField            FieldData
	CategoryOptions        []selectOption
	TypeOptions            []selectOption
	PrimaryMuscleOptions   []MuscleGroupOption
	SecondaryMuscleOptions []MuscleGroupOption
}
```

- [ ] **Step 4: Populate `AdminNav` in both handlers**

In `adminExercisesGET` (around line 87), insert immediately after the `Flash:` block in the struct literal:

```go
		AdminNav: AdminNavData{
			Active: adminSectionExercises,
			Nonce:  base.Nonce,
		},
```

In `adminExerciseEditGET` (around line 149), insert immediately after the `Flash:` block:

```go
		AdminNav: AdminNavData{
			Active: adminSectionExercises,
			Nonce:  base.Nonce,
		},
```

- [ ] **Step 5: Include the partial in both templates**

In `ui/templates/pages/admin-exercises/admin-exercises.gohtml`, insert the partial call between `page-header` and `banner`:

```gohtml
{{ define "page" }}
    <main class="stack">
        {{ template "page-header" .Header }}
        {{ template "admin-nav" .AdminNav }}
        {{ template "banner" .Flash }}
```

In `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml`, insert the same call in the same place:

```gohtml
{{ define "page" }}
    <main class="stack">
        {{ template "page-header" .Header }}
        {{ template "admin-nav" .AdminNav }}
        {{ template "banner" .Flash }}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test -v ./cmd/web -run Test_application_adminExercises`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-admin-exercises.go cmd/web/handler-admin-exercises_test.go \
        ui/templates/pages/admin-exercises/admin-exercises.gohtml \
        ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml
git commit -m "feat(web): render admin-nav on exercise admin and edit pages"
```

---

## Task 4: Wire `admin-nav` into the feature-flags page

Same shape as Task 3, applied to the feature-flags handler and template.

**Files:**
- Modify: `cmd/web/handler-admin-feature-flags.go`
- Modify: `cmd/web/handler-admin-feature-flags_test.go`
- Modify: `ui/templates/pages/admin-feature-flags/admin-feature-flags.gohtml`

- [ ] **Step 1: Write the failing test**

In `cmd/web/handler-admin-feature-flags_test.go`, inside the existing `t.Run("Promote to admin and access feature flags", ...)` block, append after the table-header assertions (around line 91):

```go
		// Admin nav renders with Feature Flags marked active.
		nav := doc.Find(`nav.admin-nav[aria-label="Admin sections"]`)
		if nav.Length() == 0 {
			t.Fatal("expected admin-nav landmark on /admin/feature-flags")
		}
		if nav.Find(`a.admin-nav__tab[href="/admin/feature-flags"][aria-current="page"]`).Length() == 0 {
			t.Error("expected Feature Flags tab to be marked aria-current=page")
		}
		if nav.Find(`a.admin-nav__tab[href="/admin/exercises"][aria-current="page"]`).Length() != 0 {
			t.Error("Exercises tab must not be active on the feature-flags page")
		}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_adminFeatureFlags`
Expected: FAIL — no `nav.admin-nav` rendered.

- [ ] **Step 3: Add `AdminNav` field to the template-data struct**

In `cmd/web/handler-admin-feature-flags.go`, update `featureFlagsAdminTemplateData` (around line 11):

```go
type featureFlagsAdminTemplateData struct {
	BaseTemplateData

	Header       PageHeaderData
	AdminNav     AdminNavData
	FeatureFlags []domain.FeatureFlag
}
```

- [ ] **Step 4: Populate `AdminNav` in the handler**

In `adminFeatureFlagsGET` (around line 28), insert into the struct literal after the `Header:` block:

```go
		AdminNav: AdminNavData{
			Active: adminSectionFeatureFlags,
			Nonce:  base.Nonce,
		},
```

- [ ] **Step 5: Include the partial in the template**

In `ui/templates/pages/admin-feature-flags/admin-feature-flags.gohtml`, insert the partial call right after `page-header`:

```gohtml
{{ define "page" }}
    <main class="stack">
        {{ template "page-header" .Header }}
        {{ template "admin-nav" .AdminNav }}
        <section>
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test -v ./cmd/web -run Test_application_adminFeatureFlags`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/web/handler-admin-feature-flags.go cmd/web/handler-admin-feature-flags_test.go \
        ui/templates/pages/admin-feature-flags/admin-feature-flags.gohtml
git commit -m "feat(web): render admin-nav on feature-flags admin page"
```

---

## Task 5: Add `/admin` landing redirect

A two-line handler that 303-redirects to `/admin/exercises`. Sits under the same `app.mustAdminStack()` middleware as the other admin routes, so non-admins land on `/forbidden`.

**Files:**
- Create: `cmd/web/handler-admin.go`
- Create: `cmd/web/handler-admin_test.go`
- Modify: `cmd/web/routes.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/web/handler-admin_test.go`:

```go
package main

import (
	"net/http"
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

//nolint:paralleltest // subtests sequentially promote the same user to admin.
func Test_application_adminGET(t *testing.T) {
	var (
		ctx = t.Context()
	)
	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	// httpClient is shared by both subtests so the second subtest reuses the
	// admin-promoted session.
	httpClient := *client.HTTPClient() // shallow copy preserves jar + transport.
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	t.Run("Non-admin is bounced to /forbidden", func(t *testing.T) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+"/admin", nil)
		if reqErr != nil {
			t.Fatalf("Build /admin request: %v", reqErr)
		}
		resp, getErr := httpClient.Do(req)
		if getErr != nil {
			t.Fatalf("GET /admin: %v", getErr)
		}
		if cerr := resp.Body.Close(); cerr != nil {
			t.Fatalf("Close response body: %v", cerr)
		}

		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("Expected 303, got %d", resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "/forbidden" {
			t.Errorf("Expected Location: /forbidden, got %q", loc)
		}
	})

	t.Run("Admin gets redirected to /admin/exercises", func(t *testing.T) {
		if _, err = server.DB().Exec("UPDATE users SET is_admin = 1 WHERE TRUE"); err != nil {
			t.Fatalf("Promote to admin: %v", err)
		}

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+"/admin", nil)
		if reqErr != nil {
			t.Fatalf("Build /admin request: %v", reqErr)
		}
		resp, getErr := httpClient.Do(req)
		if getErr != nil {
			t.Fatalf("GET /admin: %v", getErr)
		}
		if cerr := resp.Body.Close(); cerr != nil {
			t.Fatalf("Close response body: %v", cerr)
		}

		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("Expected 303, got %d", resp.StatusCode)
		}
		if loc := resp.Header.Get("Location"); loc != "/admin/exercises" {
			t.Errorf("Expected Location: /admin/exercises, got %q", loc)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_adminGET`
Expected: FAIL — `GET /admin` returns 404 (no route registered).

- [ ] **Step 3: Create the handler**

Create `cmd/web/handler-admin.go`:

```go
package main

import "net/http"

// adminGET redirects /admin to the default admin section. Linking from the
// preferences entry to /admin (rather than /admin/exercises directly) keeps
// the entry stable when admin sections come and go.
func (app *application) adminGET(w http.ResponseWriter, r *http.Request) {
	redirect(w, r, "/admin/exercises")
}
```

- [ ] **Step 4: Register the route**

In `cmd/web/routes.go`, add a new line in the admin block (after the four `/admin/exercises*` lines, before `mux.Handle("GET /admin/feature-flags", ...)`):

```go
	mux.Handle("GET /admin", app.mustAdminStack(http.HandlerFunc(app.adminGET)))
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test -v ./cmd/web -run Test_application_adminGET`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/web/handler-admin.go cmd/web/handler-admin_test.go cmd/web/routes.go
git commit -m "feat(web): add /admin landing redirect to /admin/exercises"
```

---

## Task 6: Add admin entry to the preferences page

Adds an admin-only panel (numbered `05`) to `/preferences`, placed between the Your-account panel and the danger-zone section. Gated by `{{ if $.IsAdmin }}`.

**Files:**
- Modify: `cmd/web/handler-preferences_test.go`
- Modify: `ui/templates/pages/preferences/preferences.gohtml`

- [ ] **Step 1: Write the failing test**

Append to `cmd/web/handler-preferences_test.go` (after the existing `Test_application_preferences` function):

```go
//nolint:paralleltest // subtest promotes the registered user to admin.
func Test_application_preferences_adminEntry(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	if _, err = client.Register(ctx); err != nil {
		t.Fatalf("Failed to register: %v", err)
	}

	t.Run("Non-admin does not see the admin entry", func(t *testing.T) {
		doc, getErr := client.GetDoc(ctx, "/preferences")
		if getErr != nil {
			t.Fatalf("Failed to get preferences: %v", getErr)
		}
		if doc.Find(`a[href="/admin"]`).Length() != 0 {
			t.Error("non-admin user should not see a link to /admin on preferences")
		}
		if doc.Find(`section[aria-labelledby="admin-title"]`).Length() != 0 {
			t.Error("non-admin user should not see the admin panel on preferences")
		}
	})

	t.Run("Admin sees the admin entry", func(t *testing.T) {
		if _, err = server.DB().Exec("UPDATE users SET is_admin = 1 WHERE TRUE"); err != nil {
			t.Fatalf("Promote to admin: %v", err)
		}
		doc, getErr := client.GetDoc(ctx, "/preferences")
		if getErr != nil {
			t.Fatalf("Failed to get preferences: %v", getErr)
		}
		panel := doc.Find(`section.panel[aria-labelledby="admin-title"]`)
		if panel.Length() == 0 {
			t.Fatal("admin user should see an admin panel on preferences")
		}
		if panel.Find(`a[href="/admin"]`).Length() == 0 {
			t.Error("admin panel should contain a link to /admin")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_preferences_adminEntry`
Expected: FAIL — the admin panel does not exist yet (both subtests would actually pass at this point because the "non-admin doesn't see it" assertion is vacuously satisfied; the admin subtest is what fails — `panel.Length() == 0`).

- [ ] **Step 3: Add the admin panel to the preferences template**

In `ui/templates/pages/preferences/preferences.gohtml`, insert the new panel between the existing `04 Your account` panel and the `danger-zone` section. The existing structure is:

```gohtml
<section class="panel" aria-labelledby="account-title">
    ...04 Your account content...
</section>

<section class="danger-zone" aria-labelledby="danger-title">
    ...
</section>
```

Insert between them:

```gohtml
{{ if $.IsAdmin }}
    <section class="panel" aria-labelledby="admin-title">
        <header class="panel-head">
            <span class="panel-eyebrow"><span class="panel-eyebrow-num">05</span> Admin</span>
            <h2 class="panel-title" id="admin-title">Admin tools</h2>
            <p class="panel-blurb">Manage exercises and feature flags.</p>
        </header>
        <div class="panel-actions">
            <a href="/admin" class="btn btn--ghost btn--block">Open admin</a>
        </div>
    </section>
{{ end }}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./cmd/web -run Test_application_preferences_adminEntry`
Expected: PASS — both subtests.

- [ ] **Step 5: Commit**

```bash
git add cmd/web/handler-preferences_test.go ui/templates/pages/preferences/preferences.gohtml
git commit -m "feat(web): expose admin entry in preferences for admin users"
```

---

## Task 7: Full `make ci` verification

Lint + the rest of the test suite must pass before this is shippable. This is a single-step task because there is no code to write — just the verification gate.

- [ ] **Step 1: Run the full CI suite**

Run: `make ci`
Expected: PASS — lint clean, full test suite green.

If lint flags any issue, fix it inline (typically `funlen`, `gofmt`, or an unused-import) and re-run.

- [ ] **Step 2: Manual smoke (optional but recommended)**

Run: `make run` (or whatever the project's dev-server command is — check the Makefile if unsure)
Then in a browser:
- Visit `/preferences` as a non-admin user → no admin panel.
- Promote the user to admin (`sqlite3 ./var/petra.db 'UPDATE users SET is_admin = 1'`) and refresh → admin panel `05` appears with an "Open admin" button.
- Click "Open admin" → lands on `/admin/exercises` with the Exercises tab active.
- Click "Feature Flags" in the sub-nav → lands on `/admin/feature-flags` with the Feature Flags tab active.
- Click "Exercises" → goes back, Exercises tab active.
- Open the edit page for any exercise → sub-nav still shows Exercises active.
- Resize browser to 320 px wide → tabs wrap, no horizontal scroll.

If anything looks off (touch target too small, tabs overflow, active state visually weak), revisit Task 2's scoped CSS before declaring done.

- [ ] **Step 3: No commit required**

`make ci` produces no artifacts to commit. If a lint-fix landed in Step 1, commit it with `git commit -m "lint: fix golangci-lint findings"` before declaring done.
