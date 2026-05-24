# Template clone removal — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the per-render `(*Template).Clone()` in `pageTemplate` so the parsed `*template.Template` is shared across goroutines, collapsing the bulk of GC pressure and tail-latency spikes observed in the 2026-05-24 staging stresstest.

**Architecture:** Move per-request template state (`nonce`, `mdToHTML`) off the FuncMap and onto the data the templates already receive. With nothing rebound per render, `pageTemplate` can return the cached `*template.Template` directly and the `html/template` escaper runs once per page name instead of once per request.

**Tech Stack:** Go 1.26, `html/template`, `text/template`, `log/slog`, `goldmark` (markdown).

**Spec:** `docs/superpowers/specs/2026-05-24-template-clone-removal-design.md`.

**Working directory:** Create a worktree before starting: `git worktree add .worktrees/template-clone-removal -b template-clone-removal main`. All paths below are relative to that worktree.

**Order matters.** Each task keeps the build green. Steps add the new pathway alongside the old, then flip consumers, then delete the old plumbing. Do not reorder.

---

### Task 1: Add `Nonce` to `BaseTemplateData`

**Files:**
- Modify: `cmd/web/templates.go`

#### Step 1: Add the field and populate it from request context

Modify `cmd/web/templates.go`:

```go
package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"github.com/myrjola/petrapp/internal/contexthelpers"
)

type BaseTemplateData struct {
	Authenticated     bool
	IsAdmin           bool
	InvalidationToken string
	Nonce             template.HTMLAttr
}

func newBaseTemplateData(r *http.Request) BaseTemplateData {
	var token string
	if c, err := r.Cookie("inv_bfcache"); err == nil {
		token = c.Value
	}
	return BaseTemplateData{
		Authenticated:     contexthelpers.IsAuthenticated(r.Context()),
		IsAdmin:           contexthelpers.IsAdmin(r.Context()),
		InvalidationToken: token,
		Nonce: template.HTMLAttr(
			fmt.Sprintf("nonce=%q", contexthelpers.CSPNonce(r.Context())),
		),
	}
}
```

Leave the rest of the file (`findModuleDir`, `resolveAndVerifyTemplatePath`) unchanged.

#### Step 2: Verify the build still compiles

Run: `go build ./...`
Expected: exit 0, no output.

#### Step 3: Run the existing test suite to confirm no regressions

Run: `go test ./cmd/web/... -count=1`
Expected: PASS. Templates still call `{{ nonce }}` (the existing function), so nothing observable changed yet.

#### Step 4: Commit

```sh
git add cmd/web/templates.go
git commit -m "$(cat <<'EOF'
feat(web): add Nonce to BaseTemplateData

Adds a Nonce field to BaseTemplateData populated from the request CSP
nonce. Unused for now — templates still call the {{ nonce }} func. Step
toward removing the per-render template Clone().

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Add `Nonce` to existing component dot structs

**Files:**
- Modify: `cmd/web/components.go`

#### Step 1: Extend each component struct with a `Nonce` field

Modify `cmd/web/components.go`:

```go
package main

import (
	"html/template"

	"github.com/myrjola/petrapp/internal/domain"
)

const (
	BannerVariantError   = "error"
	BannerVariantSuccess = "success"
	BannerVariantInfo    = "info"
)

const (
	inputTypeText   = "text"
	inputTypeNumber = "number"
)

// BannerData is the dot for the `banner` component. Variant is one of
// BannerVariantError, BannerVariantSuccess, or BannerVariantInfo; the
// component renders nothing when Message is empty.
type BannerData struct {
	Variant string
	Message string
	Nonce   template.HTMLAttr
}

// PageHeaderData is the dot for the `page-header` component. Subtitle is
// optional and omitted from the output when empty.
type PageHeaderData struct {
	Title    string
	Subtitle string
	Nonce    template.HTMLAttr
}

// FieldData is the dot for the `field` component.
type FieldData struct {
	Label    string
	Name     string
	Type     string
	Value    string
	Required bool
	Hint     string
	Min      string
	Max      string
	Step     string
	Pattern  string
	Nonce    template.HTMLAttr
}

// ExerciseResultCardData drives the components/exercise-result-card partial,
// shared by the Add and Swap exercise pages.
type ExerciseResultCardData struct {
	Exercise        domain.Exercise
	FormAction      string
	FieldName       string
	ButtonLabel     string
	DescriptionHTML template.HTML
	Nonce           template.HTMLAttr
}
```

`DescriptionHTML` is added here as well, so the markdown pre-render in Task 7 has the field ready.

#### Step 2: Introduce `BackLinkData` and `ExerciseSearchData`

Append to `cmd/web/components.go`:

```go
// BackLinkData is the dot for the `back-link` component.
type BackLinkData struct {
	Href  string
	Nonce template.HTMLAttr
}

// ExerciseSearchData is the dot for the `exercise-search` component.
type ExerciseSearchData struct {
	Query string
	Nonce template.HTMLAttr
}
```

#### Step 3: Verify build

Run: `go build ./...`
Expected: exit 0.

#### Step 4: Verify tests still pass

Run: `go test ./cmd/web/... -count=1`
Expected: PASS — no template change yet, no callsite change yet.

#### Step 5: Commit

```sh
git add cmd/web/components.go
git commit -m "$(cat <<'EOF'
feat(web): plumb Nonce through component dots

Adds Nonce template.HTMLAttr to BannerData, PageHeaderData, FieldData,
and ExerciseResultCardData; introduces BackLinkData and
ExerciseSearchData struct dots in place of the previous string dots.
DescriptionHTML field on ExerciseResultCardData reserved for the
upcoming markdown pre-render.

Callsites and templates still use {{ nonce }}; this commit only
extends the structs.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Switch pages and `base.gohtml` to `{{ $.Nonce }}`

**Files:**
- Modify: every file under `ui/templates/pages/` and `ui/templates/base.gohtml` that contains `{{ nonce }}` or `{{nonce}}`.

`$` is the dot passed to the currently executing template — for pages and `base`, that's the page-data struct, which embeds `BaseTemplateData` and therefore has `.Nonce`. Inside `{{ range }}` / `{{ with }}` blocks, `.` is rebound, so `$.Nonce` is the safe form everywhere in a page template.

#### Step 1: Search to confirm the file list

Run:
```sh
grep -rln '{{ *nonce *}}\|{{nonce}}' ui/templates/base.gohtml ui/templates/pages/
```
Expected: prints every page file with a `{{ nonce }}` callsite (33 sites across ~25 files).

#### Step 2: Replace `{{ nonce }}` / `{{nonce}}` with `{{ $.Nonce }}` in every match

For each file printed by the previous grep, replace both spellings with `{{ $.Nonce }}`. Mechanical change. Skip component files (`ui/templates/components/*`) — those land in Task 4.

You may run it as a single sed pass, but verify the diff after — sed cannot tell a Go expression from text and will replace inside heredocs too (none exist in `.gohtml`, but check):

```sh
find ui/templates/pages ui/templates/base.gohtml -name '*.gohtml' -print0 \
  | xargs -0 sed -i \
      -e 's/{{ *nonce *}}/{{ $.Nonce }}/g' \
      -e 's/{{nonce}}/{{ $.Nonce }}/g'
```

Verify nothing under `components/` was touched:
```sh
git diff --stat ui/templates/components/
```
Expected: empty (no changes).

Verify pages no longer contain bare `nonce`:
```sh
grep -rn '{{ *nonce' ui/templates/pages/ ui/templates/base.gohtml
```
Expected: zero matches.

#### Step 3: Run the e2e tests

Run: `go test ./cmd/web/... -count=1`
Expected: PASS. The `{{ nonce }}` function is still registered in `baseTemplateFuncs`, so any callsite the grep missed would surface as a normal render, not a failure — but `$.Nonce` works equally well because `BaseTemplateData.Nonce` is populated.

#### Step 4: Manual smoke

Run: `make run` (in another shell), open `http://localhost:8080/`, view source, confirm `<style nonce="…">` and `<script nonce="…">` tags carry a value. Stop with Ctrl+C.

Skip this step if the dev server is awkward to spin up; the e2e tests render real HTML and already cover nonce presence.

#### Step 5: Commit

```sh
git add ui/templates/base.gohtml ui/templates/pages/
git commit -m "$(cat <<'EOF'
refactor(templates): use {{ \$.Nonce }} in pages

Replaces the {{ nonce }} func call with the BaseTemplateData field on
every page template and base.gohtml. \$ reaches the page-data root
even from inside range/with blocks. The {{ nonce }} func still exists
for components, which are migrated next.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Migrate components to `.Nonce` + struct dots

**Files:**
- Modify: `ui/templates/components/back-link.gohtml`
- Modify: `ui/templates/components/banner.gohtml`
- Modify: `ui/templates/components/page-header.gohtml`
- Modify: `ui/templates/components/field.gohtml`
- Modify: `ui/templates/components/exercise-search.gohtml`
- Modify: `ui/templates/components/exercise-result-card.gohtml`

#### Step 1: Update `back-link.gohtml` for struct dot

Replace the first line and the `<a>` body to use `.Href` (was `.`) and `.Nonce` (was `nonce`).

Open `ui/templates/components/back-link.gohtml`. Change:

- Top gotype comment to: `{{- /*gotype: github.com/myrjola/petrapp/cmd/web.BackLinkData*/ -}}`
- `<style {{ nonce }}>` → `<style {{ .Nonce }}>`
- Any `href="{{ . }}"` in the anchor → `href="{{ .Href }}"`

Read the file to confirm exact lines before editing.

#### Step 2: Update `exercise-search.gohtml` for struct dot

Open `ui/templates/components/exercise-search.gohtml`. Change:

- Top gotype comment to reference `cmd/web.ExerciseSearchData`.
- `<style {{ nonce }}>` → `<style {{ .Nonce }}>`
- `value="{{ . }}"` (the search input) → `value="{{ .Query }}"`

#### Step 3: Update the remaining components

For each of `banner.gohtml`, `page-header.gohtml`, `field.gohtml`, `exercise-result-card.gohtml`, replace `{{ nonce }}` with `{{ .Nonce }}` everywhere. The gotype comments already point at structs that now carry `Nonce` — no comment change needed.

`exercise-result-card.gohtml` line 29 also flips `{{ mdToHTML .Exercise.DescriptionMarkdown }}` → `{{ .DescriptionHTML }}` (this is the only template change Task 7 needs in this file; doing it now while we're already editing avoids a second touch).

#### Step 4: Verify zero remaining `{{ nonce }}` anywhere

Run: `grep -rn '{{ *nonce' ui/templates/`
Expected: zero matches.

#### Step 5: Verify the build (cmd/web still compiles)

Run: `go build ./...`
Expected: exit 0. (Tests will fail because no caller populates the new fields yet — that's the next task.)

#### Step 6: Commit

```sh
git add ui/templates/components/
git commit -m "$(cat <<'EOF'
refactor(templates): components read Nonce from their dot

back-link and exercise-search migrate from string dots to BackLinkData
/ ExerciseSearchData. The other four components keep their existing
struct dots and read .Nonce from the added field. exercise-result-card
also flips its markdown call to the new .DescriptionHTML field
introduced in components.go.

Callers and the {{ nonce }} func still exist; the next task wires
them up.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Wire callsites to populate component `Nonce`

**Files:**
- Modify: every handler that builds a `BannerData`, `PageHeaderData`, `FieldData`, or `ExerciseResultCardData`.
- Modify: every page template that invokes `{{ template "back-link" "/…" }}` or `{{ template "exercise-search" .Query }}`.

#### Step 1: Locate every Go callsite that builds a component dot

Run:
```sh
grep -rn 'BannerData{\|PageHeaderData{\|FieldData{\|ExerciseResultCardData{' cmd/web/
```

For each callsite, add `Nonce: base.Nonce` (or equivalent — read what the surrounding handler has as its `BaseTemplateData` instance) to the struct literal.

Pattern (illustrated for one of the existing handlers; apply the same shape everywhere):

Before (in `cmd/web/handler-preferences.go` near line 113):
```go
base := newBaseTemplateData(r)
data := preferencesTemplateData{
	BaseTemplateData: base,
	Header: PageHeaderData{
		Title:    "Preferences",
		Subtitle: "",
	},
	Flash: BannerData{Variant: BannerVariantError, Message: app.popFlashError(ctx)},
	// …
}
```

After:
```go
base := newBaseTemplateData(r)
data := preferencesTemplateData{
	BaseTemplateData: base,
	Header: PageHeaderData{
		Title:    "Preferences",
		Subtitle: "",
		Nonce:    base.Nonce,
	},
	Flash: BannerData{
		Variant: BannerVariantError,
		Message: app.popFlashError(ctx),
		Nonce:   base.Nonce,
	},
	// …
}
```

If the handler currently inlines `BaseTemplateData: newBaseTemplateData(r)` directly inside the struct literal, hoist it to a local `base := newBaseTemplateData(r)` so the nonce can be reused.

Apply the same hoist-and-fill pattern in:

- `cmd/web/handler-preferences.go`
- `cmd/web/handler-exercise-info.go`
- `cmd/web/handler-styleguide.go`
- `cmd/web/handler-admin-feature-flags.go`
- `cmd/web/handler-schedule.go`
- `cmd/web/handler-home.go`
- `cmd/web/handler-workout.go` (multiple sites including the two `ExerciseResultCardData` builders at the swap and add paths around lines 342 and 467 — also populate the two new `DescriptionHTML` fields here, see Task 7 Step 1 note).

The exact line numbers above are from the current `main`; let `grep -n` drive the actual locations.

#### Step 2: Locate template callsites of back-link and exercise-search

Run:
```sh
grep -rn 'template "back-link"\|template "exercise-search"' ui/templates/
```

For every `{{ template "back-link" "/some-path" }}` callsite, change to:
```gohtml
{{ template "back-link" (backLink "/some-path" $.Nonce) }}
```

The `backLink` constructor is a new template func — add it inline in handlers.go alongside the other stateless funcs (see next step).

For every `{{ template "exercise-search" .Query }}`:
```gohtml
{{ template "exercise-search" (exerciseSearch .Query $.Nonce) }}
```

#### Step 3: Register `backLink` and `exerciseSearch` constructors

Modify the FuncMap definition in `cmd/web/handlers.go` (the function will be renamed in Task 8 — for now it is still called `baseTemplateFuncs`). Add:

```go
"backLink": func(href string, nonce template.HTMLAttr) BackLinkData {
	return BackLinkData{Href: href, Nonce: nonce}
},
"exerciseSearch": func(query string, nonce template.HTMLAttr) ExerciseSearchData {
	return ExerciseSearchData{Query: query, Nonce: nonce}
},
```

`baseTemplateFuncs` lives in `cmd/web/handlers.go` around line 30. These two helpers are stateless and registered at parse time, so they survive the Funcs() removal in Task 8.

#### Step 4: Verify the build and tests

Run: `go build ./...`
Expected: exit 0.

Run: `go test ./cmd/web/... -count=1`
Expected: PASS. Every component now has its nonce populated; rendering produces the same `nonce="…"` substring as before.

#### Step 5: Commit

```sh
git add cmd/web/ ui/templates/
git commit -m "$(cat <<'EOF'
refactor(web): handlers and templates populate component Nonce

Every BannerData/PageHeaderData/FieldData/ExerciseResultCardData
construction now carries Nonce; back-link and exercise-search invocations
go through new backLink / exerciseSearch template constructors that
build the struct dots from $.Nonce.

The {{ nonce }} func is now unused in templates but still registered
in baseTemplateFuncs — removed in the next task.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 6: Rewrite the `pageTemplate` cache test (TDD red phase)

**Files:**
- Modify: `cmd/web/handlers_test.go`

#### Step 1: Replace the clone assertion with a same-instance assertion

In `cmd/web/handlers_test.go`, replace `Test_pageTemplate_cachesAndReturnsCloneInProdMode` (lines 11–53) with:

```go
// Test_pageTemplate_cachesAndReturnsSameInstanceInProdMode verifies that
// successive calls in production mode return the same cached
// *template.Template pointer (no per-render clone) and that concurrent
// access does not race.
func Test_pageTemplate_cachesAndReturnsSameInstanceInProdMode(t *testing.T) {
	t.Parallel()

	templatePath, err := filepath.Abs(filepath.Join("..", "..", "ui", "templates"))
	if err != nil {
		t.Fatalf("resolve template path: %v", err)
	}
	app := &application{ //nolint:exhaustruct // only the fields touched by pageTemplate matter here.
		templateFS:      os.DirFS(templatePath),
		parsedTemplates: newTemplateCache(),
		devMode:         false,
	}

	first, err := app.pageTemplate("home")
	if err != nil {
		t.Fatalf("first pageTemplate: %v", err)
	}
	second, err := app.pageTemplate("home")
	if err != nil {
		t.Fatalf("second pageTemplate: %v", err)
	}
	if first != second {
		t.Errorf("expected pageTemplate to return the same cached pointer; got distinct instances")
	}
	if cached := app.parsedTemplates.get("home"); cached != first {
		t.Errorf("expected cache to retain the parsed template for 'home'")
	}

	// Concurrent access must not race.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, gerr := app.pageTemplate("home"); gerr != nil {
				t.Errorf("concurrent pageTemplate: %v", gerr)
			}
		})
	}
	wg.Wait()
}
```

`Test_pageTemplate_skipsCacheInDevMode` (lines 57–75) is unchanged.

#### Step 2: Run the test to see it fail

Run: `go test ./cmd/web/ -run Test_pageTemplate_cachesAndReturnsSameInstanceInProdMode -count=1 -race`
Expected: FAIL. The current `pageTemplate` returns `cached.Clone()`, so `first != second`.

#### Step 3: Do NOT commit yet — implementation lands in Task 8 and they ship together

This task only stages the failing test. Task 8 implements `pageTemplate` correctly and the test goes green.

---

### Task 7: Pre-render markdown in handlers

**Files:**
- Modify: `cmd/web/handler-exercise-info.go`
- Modify: `cmd/web/handler-workout.go`

The `exercise-result-card.gohtml` template change (`{{ mdToHTML … }}` → `{{ .DescriptionHTML }}`) already landed in Task 4; this task adds the data-side counterpart so the field is non-empty.

#### Step 1: Replace `renderMarkdownToHTML` with a package-level helper

In `cmd/web/handler-exercise-info.go`, delete the method and add a package-level function (top of the file, after imports):

```go
// markdownToHTML renders Markdown to template.HTML. On error it logs
// and returns a fallback paragraph; goldmark.Convert into a bytes.Buffer
// does not error in practice, but the defensive branch matches prior
// behaviour.
func markdownToHTML(ctx context.Context, logger *slog.Logger, md string) template.HTML {
	gm := goldmark.New()
	var buf bytes.Buffer
	if err := gm.Convert([]byte(md), &buf); err != nil {
		logger.LogAttrs(ctx, slog.LevelError, "failed to render markdown",
			slog.Any("error", err))
		return "<p>Error rendering markdown content.</p>"
	}
	return template.HTML(buf.String()) //nolint:gosec // markdown renderer output is trusted.
}
```

Remove `func (app *application) renderMarkdownToHTML(...)`. Imports already include `bytes`, `context`, `html/template`, `log/slog`, and `github.com/yuin/goldmark`.

#### Step 2: Add `DescriptionHTML` to the exercise-info data and populate it

In `cmd/web/handler-exercise-info.go`, change `exerciseInfoTemplateData`:

```go
type exerciseInfoTemplateData struct {
	BaseTemplateData

	Date              time.Time
	Header            PageHeaderData
	WorkoutExerciseID int
	Exercise          domain.Exercise
	IsAdmin           bool
	ProgressPoints    []ExerciseProgressDataPoint
	DescriptionHTML   template.HTML
}
```

In `exerciseInfoGET`, populate it when constructing the data:

```go
base := newBaseTemplateData(r)
data := exerciseInfoTemplateData{
	BaseTemplateData: base,
	Date:             date,
	Header: PageHeaderData{
		Title:    exercise.Name,
		Subtitle: "",
		Nonce:    base.Nonce,
	},
	WorkoutExerciseID: workoutExerciseID,
	Exercise:          exercise,
	IsAdmin:           isAdmin,
	ProgressPoints:    progressData,
	DescriptionHTML:   markdownToHTML(r.Context(), app.logger, exercise.DescriptionMarkdown),
}
```

#### Step 3: Update the exercise-info template

Open `ui/templates/pages/exercise-info/exercise-info.gohtml`. Line 365 (currently `{{ mdToHTML .Exercise.DescriptionMarkdown }}`) becomes:

```gohtml
{{ .DescriptionHTML }}
```

#### Step 4: Populate `DescriptionHTML` and `Nonce` in the two `ExerciseResultCardData` builders

In `cmd/web/handler-workout.go`, at the swap-candidates loop (currently around line 342):

```go
cards := make([]ExerciseResultCardData, 0, len(candidates))
for _, ex := range candidates {
	cards = append(cards, ExerciseResultCardData{
		Exercise:        ex,
		FormAction:      fmt.Sprintf("/workouts/%s/exercises/%d/swap", dateStr, workoutExerciseID),
		FieldName:       "new_exercise_id",
		ButtonLabel:     "Swap to this exercise",
		DescriptionHTML: markdownToHTML(r.Context(), app.logger, ex.DescriptionMarkdown),
		Nonce:           base.Nonce,
	})
}
```

Hoist `base := newBaseTemplateData(r)` ahead of the loop if it isn't already a local.

Apply the same shape at the add-exercise loop (currently around line 467).

#### Step 5: Run the tests

Run: `go test ./cmd/web/... -count=1`
Expected: PASS. Existing handler e2e tests render the affected pages and would catch any structural regression in the markdown output.

#### Step 6: Confirm no remaining `mdToHTML` references

Run: `grep -rn 'mdToHTML\|renderMarkdownToHTML' cmd/web ui/templates`
Expected: zero matches.

#### Step 7: Commit

```sh
git add cmd/web/handler-exercise-info.go cmd/web/handler-workout.go ui/templates/pages/exercise-info/exercise-info.gohtml
git commit -m "$(cat <<'EOF'
refactor(web): pre-render markdown in handlers

Replaces the per-request mdToHTML template func with a package-level
markdownToHTML helper called from handlers. exerciseInfoTemplateData
and ExerciseResultCardData now carry a DescriptionHTML
template.HTML field; the templates dereference it directly.

The mdToHTML func registration is now dead; removed in the next task.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 8: Remove the per-render Clone() and Funcs() rebind (TDD green phase)

**Files:**
- Modify: `cmd/web/handlers.go`

This is the change the rest of the work was setting up. Combined with the test from Task 6.

#### Step 1: Delete `contextTemplateFuncs`, simplify `baseTemplateFuncs`

In `cmd/web/handlers.go`, replace the func-map helpers. Drop `contextTemplateFuncs` entirely. Rename `baseTemplateFuncs` to `templateFuncs`; it keeps only the stateless funcs plus the two constructors added in Task 5:

```go
// templateFuncs returns the funcs registered at parse time. All entries
// are stateless and safe to share across goroutines.
func (app *application) templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatFloat":    formatFloat,
		"sub":            func(a, b int) int { return a - b },
		"backLink":       func(href string, nonce template.HTMLAttr) BackLinkData { return BackLinkData{Href: href, Nonce: nonce} },
		"exerciseSearch": func(query string, nonce template.HTMLAttr) ExerciseSearchData { return ExerciseSearchData{Query: query, Nonce: nonce} },
	}
}
```

Receiver `app` is kept for symmetry / future extension but is unused; the linter may flag it. If `funlen`/`unused-receiver` complains, drop the receiver and call it as a package-level `templateFuncs()`.

#### Step 2: Update `parsePageTemplate` to call `templateFuncs`

```go
func (app *application) parsePageTemplate(pageName string) (*template.Template, error) {
	t := template.New(pageName).Funcs(app.templateFuncs())
	t, err := t.ParseFS(app.templateFS,
		"base.gohtml",
		"components/*.gohtml",
		fmt.Sprintf("pages/%s/*.gohtml", pageName),
	)
	if err != nil {
		return nil, fmt.Errorf("new template: %w", err)
	}
	return t, nil
}
```

#### Step 3: Drop the Clone() from `pageTemplate`

Replace the body of `pageTemplate`:

```go
// pageTemplate returns the parsed template for the given page name.
//
// pageName corresponds to the directory inside ui/templates/pages. It
// must include a template named "page".
//
// In dev mode (no FLY_APP_NAME) templates are parsed fresh on every
// call so a template edit is reflected on the next refresh. In
// production the parsed template is cached and reused across requests.
// The cached template is never mutated after the first Execute, so it
// is safe to share across goroutines.
func (app *application) pageTemplate(pageName string) (*template.Template, error) {
	if app.devMode {
		return app.parsePageTemplate(pageName)
	}
	if cached := app.parsedTemplates.get(pageName); cached != nil {
		return cached, nil
	}
	parsed, err := app.parsePageTemplate(pageName)
	if err != nil {
		return nil, err
	}
	app.parsedTemplates.set(pageName, parsed)
	return parsed, nil
}
```

#### Step 4: Drop the per-request `Funcs(...)` call from `renderToBuf`

```go
func (app *application) renderToBuf(ctx context.Context, file string, data any) (*bytes.Buffer, error) {
	t, err := app.pageTemplate(file)
	if err != nil {
		return nil, fmt.Errorf("retrieve page template %s: %w", file, err)
	}
	buf := new(bytes.Buffer)
	if err = t.ExecuteTemplate(buf, "base", data); err != nil {
		return nil, fmt.Errorf("execute template %s: %w", file, err)
	}
	return buf, nil
}
```

The `ctx` parameter is now unused; keep it on the signature — callers pass it and rewriting the signature is out of scope. The linter may flag it; if so, add `//nolint:revive` or use the underscore convention used elsewhere in the file.

Actually — check whether `ctx` is used inside the function body for anything else first. If not, and if removing it requires touching every caller, leave it in and add `_ = ctx` only if the linter complains. The standard project lint set tolerates unused parameters on exported-shape methods.

#### Step 5: Update the docstring on `templateCache`

The struct comment (`cmd/web/handlers.go:54-56`) calls itself "cloned per render"; rewrite:

```go
// templateCache memoizes parsed page templates so each request doesn't
// re-read template files from disk and re-parse them. The cached
// templates are read-only after first execute and safe to share.
type templateCache struct {
	mu sync.RWMutex
	m  map[string]*template.Template
}
```

#### Step 6: Run the rewritten cache test — it should now pass

Run: `go test ./cmd/web/ -run Test_pageTemplate_cachesAndReturnsSameInstanceInProdMode -count=1 -race`
Expected: PASS.

#### Step 7: Run the full cmd/web suite under -race

Run: `go test ./cmd/web/... -count=1 -race`
Expected: PASS. The e2e tests render every page; any missed nonce/markdown migration surfaces here.

#### Step 8: Confirm `contextTemplateFuncs` is gone

Run: `grep -rn 'contextTemplateFuncs\|baseTemplateFuncs' cmd/web/`
Expected: zero matches. (If `baseTemplateFuncs` still appears, it's an orphan rename — clean up.)

#### Step 9: Commit (both Task 6's test and Task 8's implementation)

```sh
git add cmd/web/handlers.go cmd/web/handlers_test.go
git commit -m "$(cat <<'EOF'
perf(web): stop cloning templates per render

Removes the (*Template).Clone() and per-request Funcs() rebind from
pageTemplate / renderToBuf. With Nonce on data and markdown
pre-rendered in handlers, no per-request template state remains; the
cached *Template is safe to share. html/template's lazy escaper now
walks each page tree once per process lifetime instead of once per
request.

Rewrites the cache test to assert same-instance pointer and
race-free concurrent access.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 9: Add a render benchmark

**Files:**
- Create: `cmd/web/render_bench_test.go`

#### Step 1: Write the benchmark

Create `cmd/web/render_bench_test.go`:

```go
package main

import (
	"context"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/myrjola/petrapp/internal/testhelpers"
)

// BenchmarkRenderHome measures the cost of rendering the cached home
// template into an io.Discard writer with a representative
// homeTemplateData payload. Used as a regression guard for the
// template clone-removal work.
func BenchmarkRenderHome(b *testing.B) {
	templatePath, err := filepath.Abs(filepath.Join("..", "..", "ui", "templates"))
	if err != nil {
		b.Fatalf("resolve template path: %v", err)
	}
	app := &application{ //nolint:exhaustruct // only the fields touched by pageTemplate matter here.
		templateFS:      os.DirFS(templatePath),
		parsedTemplates: newTemplateCache(),
		logger:          testhelpers.NewLogger(io.Discard),
		devMode:         false,
	}
	// Prime the cache.
	if _, err = app.pageTemplate("home"); err != nil {
		b.Fatalf("prime pageTemplate: %v", err)
	}
	data := homeTemplateData{ //nolint:exhaustruct // benchmark uses the zero-valued unauthenticated path.
		BaseTemplateData: BaseTemplateData{
			Nonce: template.HTMLAttr(`nonce="benchmark"`),
		},
	}
	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		buf, rerr := app.renderToBuf(ctx, "home", data)
		if rerr != nil {
			b.Fatalf("render: %v", rerr)
		}
		buf.Reset()
	}
}
```

`homeTemplateData` is at `cmd/web/handler-home.go:49`. If the template requires non-zero fields to render without erroring (check `pages/home/*.gohtml`), populate the minimum needed — or swap the target to a simpler page (`forbidden`, `not-found`). The benchmark exists to track regression in the render pipeline, not to be representative.

#### Step 2: Run the benchmark

Run: `go test ./cmd/web/ -bench=BenchmarkRenderHome -benchmem -run=^$ -count=3`
Expected: a `ns/op` and `B/op` figure. Record both in the commit message for posterity. Allocations should be small (no template-tree copies); the value will depend on the host.

#### Step 3: Commit

```sh
git add cmd/web/render_bench_test.go
git commit -m "$(cat <<'EOF'
test(web): add BenchmarkRenderHome regression guard

Locks in the post-clone-removal render cost. Run with:

  go test ./cmd/web/ -bench=BenchmarkRenderHome -benchmem -run=^\$

Recorded baseline at <ns/op>, <B/op>, <allocs/op>.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

Edit the recorded numbers into the commit message before running `git commit`.

---

### Task 10: Update CLAUDE.md docs

**Files:**
- Modify: `cmd/web/CLAUDE.md`
- Modify: `ui/templates/CLAUDE.md`

#### Step 1: `cmd/web/CLAUDE.md` — BaseTemplateData section

Find the paragraph beginning "The CSP nonce and CSRF plumbing do **not** travel on this struct" (around line 45). Replace with:

> `BaseTemplateData` (defined in `templates.go`) exposes four fields that every page template can read:
>
> - `Authenticated bool` — set from `contexthelpers.IsAuthenticated(r.Context())`
> - `IsAdmin bool` — set from `contexthelpers.IsAdmin(r.Context())`
> - `InvalidationToken string` — set from the `inv_bfcache` cookie
> - `Nonce template.HTMLAttr` — the CSP nonce, pre-formatted as `nonce="<value>"`. Reference as `{{ $.Nonce }}` in page templates and `{{ .Nonce }}` in components (whose dot structs each carry their own `Nonce` field).
>
> CSRF protection is handled in middleware and does not appear on this struct.

#### Step 2: `ui/templates/CLAUDE.md` — Available Template Functions section

Find the "Available Template Functions" / "Security Functions" block (around line 170). Replace:

> ### Security helpers (data-driven)
>
> CSP nonces are passed through data, not via a template function:
>
> - In page templates and `base.gohtml`, reference `{{ $.Nonce }}` — `$` reaches the page-data root even from inside `{{ range }}` / `{{ with }}` blocks.
> - In component partials, the component's dot struct carries a `Nonce template.HTMLAttr` field; reference it as `{{ .Nonce }}`. Callers populate it from their surrounding `BaseTemplateData.Nonce`.
>
> Markdown rendering happens in the handler — pre-render to `template.HTML` and pass it on the data struct. There is no `mdToHTML` template function.

Update every code example in the file that uses `{{ nonce }}` to use `{{ .Nonce }}` (lines around 163, 187, 191, 254 per the current file). The general rule is: examples shown inside a component context use `.Nonce`; examples shown inside a page context use `$.Nonce`.

#### Step 3: Update the `pageTemplate` docstring tracking note (already done in Task 8)

No-op step; verify the docstring touched in Task 8 Step 5 is the current text.

#### Step 4: Commit

```sh
git add cmd/web/CLAUDE.md ui/templates/CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: BaseTemplateData carries Nonce; markdown is handler-rendered

Updates the CLAUDE.md guidance to reflect the new data-driven
convention: {{ \$.Nonce }} in pages, {{ .Nonce }} in components,
template.HTML pre-rendered in handlers for markdown.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 11: Final verification gauntlet

#### Step 1: Run the full lint + test pipeline

Run: `make ci`
Expected: PASS through build, lint, test, govulncheck.

#### Step 2: Run the cached suite under -race once more

Run: `go test ./... -race -count=1`
Expected: PASS.

#### Step 3: Confirm no remnants of the old plumbing

Run:
```sh
grep -rn '{{ *nonce\b\|mdToHTML\|contextTemplateFuncs\|baseTemplateFuncs\|renderMarkdownToHTML\|\.Clone()' cmd/web ui/templates
```
Expected: zero matches.

#### Step 4: (Optional) local render smoke

Run `make run`, browse `/`, `/preferences`, `/workouts/<today>`, and `/workouts/<today>/exercises/<id>/info`. Confirm:
- Page sources show `nonce="…"` on every `<style>` and `<script>`.
- The exercise-info page renders its markdown description.
- Browser devtools shows no CSP violations.

Stop with Ctrl+C.

#### Step 5: Push branch and open PR

```sh
git push -u origin template-clone-removal
gh pr create --title "perf(web): stop cloning templates per render" --body "$(cat <<'EOF'
## Summary
- Plumb CSP nonce through `BaseTemplateData` and component dot structs so templates dereference `{{ .Nonce }}` / `{{ $.Nonce }}` instead of calling a per-request func.
- Pre-render exercise-description markdown in handlers; templates dereference `{{ .DescriptionHTML }}`.
- Drop the per-render `(*Template).Clone()` + per-request `Funcs()` rebind. The cached `*template.Template` is now reused across goroutines, so `html/template`'s lazy escaper walks each page tree once per process instead of once per request.

## Why
2026-05-24 staging stresstest (`docs/superpowers/specs/2026-05-24-template-clone-removal-design.md`): p99 on `/`, `/preferences`, `POST /preferences` hit 700–900 ms under 20-user load with 1.6 GB of `parse.Tree.Copy` allocations over 2 m. Heap profile attributed the spikes to the clone.

## Test plan
- [x] `make ci`
- [x] `go test ./... -race -count=1`
- [x] `go test ./cmd/web/ -bench=BenchmarkRenderHome -benchmem -run=^\$`
- [ ] After staging deploy: re-run `bin/stresstest --users 20 --duration 2m --pprof-url http://localhost:6060 --out pprof petra-staging.fly.dev` and compare p99 against the baseline in the design doc.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-review notes

- **Spec coverage**: every spec section maps to a task — BaseTemplateData (T1), components (T2–T5), pageTemplate (T6+T8), markdown (T7), test rewrite (T6), benchmark (T9), docs (T10), verification (T11).
- **Placeholder scan**: no TBDs, every step shows the actual code. Line numbers are noted as "around line X" because the spec was written against `main` and the worktree may drift — let `grep -n` drive exact locations.
- **Type consistency**: `BackLinkData`/`ExerciseSearchData` are introduced in T2, consumed in T4, populated via constructors in T5. `markdownToHTML` signature `(context.Context, *slog.Logger, string) template.HTML` is consistent across T7 callsites. `Nonce template.HTMLAttr` is the same type on every struct it appears on.
- **Order safety**: every task leaves the build green. The `Funcs()` registration of `nonce`/`mdToHTML` survives until T8; the templates stop using them earlier (T3, T4, T7) but the func map still satisfies parse-time references during the transition. T6's test goes red; T8's implementation makes it green; they ship in one commit.
