# Frontend Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish a settled component vocabulary, a forms/validation convention, and template-logic discipline for the no-build, mobile-only Go MPA frontend, with `workout.gohtml` migrated as the worked reference page.

**Architecture:** A guarantee-based split — Go template partials for accessibility/structure-enforcing pieces (`banner`, `page-header`, `field`), CSS classes in `main.css` for pure-paint pieces (`badge`, `card`) and layout primitives (`stack`, `cluster`, `grid-auto`, `center`), inline scoped `<style>` as the documented escape hatch. Derived display values move from templates onto domain methods. A typed `domain.ValidationError` is established as forms infrastructure. The styleguide page becomes the living catalog and the test surface for every component.

**Tech Stack:** Go `html/template` (runtime-loaded via `os.DirFS`), `net/http` stdlib router, CSS with `@layer` + `@scope` + open-props-derived tokens, strict CSP (nonces + Trusted Types), `e2etest` package + `goquery` for handler tests.

**Spec:** `docs/superpowers/specs/2026-05-14-frontend-foundation-design.md`

---

## Background for the implementer

- Templates and `ui/static/` assets load from the filesystem at runtime — editing a template and refreshing the browser is the whole dev loop, no rebuild. `main.css` is fingerprinted only in the Docker build; in dev it is served as-is.
- Every `<style>` and `<script>` tag in a template MUST carry the `{{ nonce }}` attribute or the CSP drops it. Inline `style="..."` attributes are forbidden (CSP `style-src` nonce).
- Components live in `ui/templates/components/*.gohtml`; every file there is parsed alongside every page, so a `{{ define "name" }}` block is callable from any page as `{{ template "name" <dot> }}`. The existing example is `components/back-link.gohtml`.
- Go templates cannot construct structs. A component whose dot is a struct needs that struct prepared in a Go handler. The dot structs for the new partials live in a new file `cmd/web/components.go`.
- The styleguide page (`/dev/styleguide`) is dev-only: `styleguideGET` returns 404 unless `app.devMode` is true. `devMode` is `cfg.FlyAppName == ""`, which is true in tests — so `e2etest` servers can fetch `/dev/styleguide`.
- Run a single Go test: `go test -v ./internal/domain -run TestName` or `go test -v ./cmd/web -run TestName`. Full suite: `make test` (runs `go test --race --shuffle=on ./...`). Linter with autofix: `make lint-fix`.
- Project style: comments end with a period; error types suffixed `Error`, sentinel errors prefixed `Err`; exhaustive enum `switch` (the `exhaustive` linter is on — a `switch` covering every constant, or one with a `default`, satisfies it); no magic numbers (the `mnd` linter).

---

## File Structure

**Created:**
- `cmd/web/components.go` — dot structs for the new partials (`BannerData`, `PageHeaderData`, `FieldData`).
- `cmd/web/handler-styleguide_test.go` — renders `/dev/styleguide` and asserts every component/primitive is present. Grows across Tasks 4–7.
- `ui/templates/components/banner.gohtml` — `banner` partial.
- `ui/templates/components/page-header.gohtml` — `page-header` partial.
- `ui/templates/components/field.gohtml` — `field` partial.
- `internal/domain/errors_test.go` — `ValidationError` unit test.

**Modified:**
- `internal/domain/exercise.go` — add `Category.Label()`.
- `internal/domain/exercise_test.go` — test `Category.Label()`.
- `internal/domain/session.go` — add `Session.WorkoutType()`, `Session.Status()`, `ExerciseSet.CompletionState()` and their enum types.
- `internal/domain/session_test.go` — test the three new methods.
- `internal/domain/errors.go` — add `ValidationError` type.
- `ui/static/main.css` — add `@layer layout` block (4 primitives) and `.badge`/`.card` into `@layer components`.
- `cmd/web/handler-styleguide.go` — add example data for the new components.
- `ui/templates/pages/styleguide/styleguide.gohtml` — add catalog sections.
- `cmd/web/handler-workout.go` — `workoutTemplateData` gains `Header`, `StatusLabel`, `StatusVariant`; `workoutGET` prepares them.
- `ui/templates/pages/workout/workout.gohtml` — migrated to use the new partials, classes, and domain-derived state.
- `ui/templates/CLAUDE.md` — guarantee-based split rule + component inventory.
- `cmd/web/CLAUDE.md` — validation-error flow.

---

## Task 1: Domain — `Category.Label()` and `Session.WorkoutType()`

Moves the workout-type derivation out of `workout.gohtml` (which currently scans exercise categories with `$hasUpper`/`$hasLower` template variables) onto domain methods.

**Files:**
- Modify: `internal/domain/exercise.go`
- Modify: `internal/domain/session.go`
- Test: `internal/domain/exercise_test.go`, `internal/domain/session_test.go`

- [ ] **Step 1: Write the failing test for `Category.Label()`**

Add to `internal/domain/exercise_test.go`:

```go
func TestCategory_Label(t *testing.T) {
	tests := []struct {
		name     string
		category domain.Category
		want     string
	}{
		{"upper", domain.CategoryUpper, "Upper Body"},
		{"lower", domain.CategoryLower, "Lower Body"},
		{"full body", domain.CategoryFullBody, "Full Body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.category.Label(); got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

If `exercise_test.go` does not already import the `domain` package under that path, match the existing import style in the file (external `_test` package importing `github.com/myrjola/petrapp/internal/domain`, or internal package — check the file header and follow it).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./internal/domain -run TestCategory_Label`
Expected: FAIL — `tt.category.Label undefined`.

- [ ] **Step 3: Implement `Category.Label()`**

Add to `internal/domain/exercise.go`, immediately after the `Category.IsValid()` method:

```go
// Label returns the human-readable workout-split name for display.
func (c Category) Label() string {
	switch c {
	case CategoryUpper:
		return "Upper Body"
	case CategoryLower:
		return "Lower Body"
	case CategoryFullBody:
		return "Full Body"
	default:
		return "Full Body"
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -v ./internal/domain -run TestCategory_Label`
Expected: PASS.

- [ ] **Step 5: Write the failing test for `Session.WorkoutType()`**

Add to `internal/domain/session_test.go`. The helper builds a session from a list of categories — adjust the `ExerciseSet`/`Exercise` literals to match the actual struct fields if they differ:

```go
func TestSession_WorkoutType(t *testing.T) {
	sessionWith := func(cats ...domain.Category) domain.Session {
		var sets []domain.ExerciseSet
		for i, c := range cats {
			sets = append(sets, domain.ExerciseSet{
				ID:       i + 1,
				Exercise: domain.Exercise{Category: c},
			})
		}
		return domain.Session{ExerciseSets: sets}
	}
	tests := []struct {
		name string
		sess domain.Session
		want domain.Category
	}{
		{"empty defaults to full body", sessionWith(), domain.CategoryFullBody},
		{"only upper", sessionWith(domain.CategoryUpper, domain.CategoryUpper), domain.CategoryUpper},
		{"only lower", sessionWith(domain.CategoryLower), domain.CategoryLower},
		{"upper and lower is full body", sessionWith(domain.CategoryUpper, domain.CategoryLower), domain.CategoryFullBody},
		{"any full body is full body", sessionWith(domain.CategoryUpper, domain.CategoryFullBody), domain.CategoryFullBody},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sess.WorkoutType(); got != tt.want {
				t.Errorf("WorkoutType() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

If `session_test.go` uses partial struct literals, the `exhaustruct` linter may complain — match how existing tests in that file construct `Session`/`ExerciseSet`/`Exercise` (they likely use a builder or `//nolint:exhaustruct`). Follow the file's existing pattern.

- [ ] **Step 6: Run the test to verify it fails**

Run: `go test -v ./internal/domain -run TestSession_WorkoutType`
Expected: FAIL — `tt.sess.WorkoutType undefined`.

- [ ] **Step 7: Implement `Session.WorkoutType()`**

Add to `internal/domain/session.go`, after the `Session` type's existing methods. Note the **value receiver** — this method is read-only and must be callable from templates on non-addressable values:

```go
// WorkoutType derives the muscle-split category for the session from the
// categories of its exercise slots: full body if any full-body exercise is
// present or both upper and lower are represented, otherwise whichever of
// upper or lower is present. An empty session defaults to full body.
func (s Session) WorkoutType() Category {
	hasUpper, hasLower := false, false
	for i := range s.ExerciseSets {
		switch s.ExerciseSets[i].Exercise.Category {
		case CategoryFullBody:
			return CategoryFullBody
		case CategoryUpper:
			hasUpper = true
		case CategoryLower:
			hasLower = true
		}
	}
	if hasUpper && hasLower {
		return CategoryFullBody
	}
	if hasUpper {
		return CategoryUpper
	}
	if hasLower {
		return CategoryLower
	}
	return CategoryFullBody
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test -v ./internal/domain -run 'TestSession_WorkoutType|TestCategory_Label'`
Expected: PASS.

- [ ] **Step 9: Lint**

Run: `make lint-fix`
Expected: no errors. If `exhaustive` flags the `switch` in `WorkoutType`, confirm all three `Category` constants are listed (they are; the `switch` has no `default` and is exhaustive over the enum).

- [ ] **Step 10: Commit**

```bash
git add internal/domain/exercise.go internal/domain/exercise_test.go internal/domain/session.go internal/domain/session_test.go
git commit -m "Add Category.Label and Session.WorkoutType domain methods"
```

---

## Task 2: Domain — `Session.Status()` and `ExerciseSet.CompletionState()`

Moves the session-status and per-exercise completion derivations out of `workout.gohtml` (currently `$allSetsCompleted`/`$hasSomeCompleted` template variables and a `CompletedAt`/`StartedAt` if-chain) onto domain methods.

**Files:**
- Modify: `internal/domain/session.go`
- Test: `internal/domain/session_test.go`

- [ ] **Step 1: Write the failing test for `Session.Status()`**

Add to `internal/domain/session_test.go`:

```go
func TestSession_Status(t *testing.T) {
	past := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		sess domain.Session
		want domain.SessionStatus
	}{
		{"not started", domain.Session{}, domain.SessionNotStarted},
		{"in progress", domain.Session{StartedAt: past}, domain.SessionInProgress},
		{"completed", domain.Session{StartedAt: past, CompletedAt: later}, domain.SessionCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sess.Status(); got != tt.want {
				t.Errorf("Status() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

Match the file's struct-construction convention (builder or `//nolint:exhaustruct`) as in Task 1.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./internal/domain -run TestSession_Status`
Expected: FAIL — `domain.SessionStatus` / `tt.sess.Status` undefined.

- [ ] **Step 3: Implement `SessionStatus` and `Session.Status()`**

Add to `internal/domain/session.go`, near the top after the `PeriodizationType` block:

```go
// SessionStatus is the lifecycle state of a workout session, for display.
type SessionStatus string

const (
	SessionNotStarted SessionStatus = "not_started"
	SessionInProgress SessionStatus = "in_progress"
	SessionCompleted  SessionStatus = "completed"
)
```

And add the method (value receiver — read-only, template-callable):

```go
// Status reports the session's lifecycle state from its timestamps.
func (s Session) Status() SessionStatus {
	if !s.CompletedAt.IsZero() {
		return SessionCompleted
	}
	if !s.StartedAt.IsZero() {
		return SessionInProgress
	}
	return SessionNotStarted
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -v ./internal/domain -run TestSession_Status`
Expected: PASS.

- [ ] **Step 5: Write the failing test for `ExerciseSet.CompletionState()`**

Add to `internal/domain/session_test.go`:

```go
func TestExerciseSet_CompletionState(t *testing.T) {
	now := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	completed := domain.Set{CompletedAt: &now}
	pending := domain.Set{}
	tests := []struct {
		name string
		sets []domain.Set
		want domain.ExerciseSetState
	}{
		{"no sets is not started", nil, domain.ExerciseSetNotStarted},
		{"all pending is not started", []domain.Set{pending, pending}, domain.ExerciseSetNotStarted},
		{"some completed is started", []domain.Set{completed, pending}, domain.ExerciseSetStarted},
		{"all completed is completed", []domain.Set{completed, completed}, domain.ExerciseSetCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			es := domain.ExerciseSet{Sets: tt.sets}
			if got := es.CompletionState(); got != tt.want {
				t.Errorf("CompletionState() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

`domain.Set` zero value has `CompletedAt` nil — match the file's `exhaustruct` convention if it applies to `Set` literals.

- [ ] **Step 6: Run the test to verify it fails**

Run: `go test -v ./internal/domain -run TestExerciseSet_CompletionState`
Expected: FAIL — `domain.ExerciseSetState` / `es.CompletionState` undefined.

- [ ] **Step 7: Implement `ExerciseSetState` and `ExerciseSet.CompletionState()`**

Add to `internal/domain/session.go`, immediately after the `ExerciseSet` struct definition:

```go
// ExerciseSetState is the completion state of an exercise slot, for display.
type ExerciseSetState string

const (
	ExerciseSetNotStarted ExerciseSetState = "not-started"
	ExerciseSetStarted    ExerciseSetState = "started"
	ExerciseSetCompleted  ExerciseSetState = "completed"
)

// CompletionState reports whether none, some, or all of the slot's sets have
// been completed. A slot with no sets is reported as not started. The string
// values double as CSS state tokens used by the workout page.
func (es ExerciseSet) CompletionState() ExerciseSetState {
	if len(es.Sets) == 0 {
		return ExerciseSetNotStarted
	}
	completed := 0
	for i := range es.Sets {
		if es.Sets[i].CompletedAt != nil {
			completed++
		}
	}
	switch completed {
	case 0:
		return ExerciseSetNotStarted
	case len(es.Sets):
		return ExerciseSetCompleted
	default:
		return ExerciseSetStarted
	}
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test -v ./internal/domain -run 'TestSession_Status|TestExerciseSet_CompletionState'`
Expected: PASS.

- [ ] **Step 9: Lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 10: Commit**

```bash
git add internal/domain/session.go internal/domain/session_test.go
git commit -m "Add Session.Status and ExerciseSet.CompletionState domain methods"
```

---

## Task 3: Domain — `ValidationError` type

Establishes the typed error that domain/service code returns for user-facing validation failures, and that handlers detect with `errors.As` to drive `putFlashError` + redirect-to-form. This task establishes the infrastructure only; wiring specific validation rules onto it happens during later page migrations.

**Files:**
- Modify: `internal/domain/errors.go`
- Test: `internal/domain/errors_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/domain/errors_test.go`:

```go
package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/myrjola/petrapp/internal/domain"
)

func TestValidationError(t *testing.T) {
	err := error(domain.ValidationError{Message: "name is required"})

	if err.Error() != "name is required" {
		t.Errorf("Error() = %q, want %q", err.Error(), "name is required")
	}

	var ve domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("errors.As failed to match ValidationError")
	}
	if ve.Message != "name is required" {
		t.Errorf("Message = %q, want %q", ve.Message, "name is required")
	}

	wrapped := fmt.Errorf("create exercise: %w", err)
	if !errors.As(wrapped, &ve) {
		t.Fatal("errors.As failed to match wrapped ValidationError")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./internal/domain -run TestValidationError`
Expected: FAIL — `domain.ValidationError` undefined.

- [ ] **Step 3: Implement `ValidationError`**

Add to the end of `internal/domain/errors.go`:

```go
// ValidationError is a domain validation failure carrying a message that is
// safe to surface directly to the end user. Handlers detect it with
// errors.As and surface it via putFlashError + redirect-to-form; see
// cmd/web/CLAUDE.md for the full flow.
type ValidationError struct {
	Message string
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	return e.Message
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -v ./internal/domain -run TestValidationError`
Expected: PASS.

- [ ] **Step 5: Lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/errors.go internal/domain/errors_test.go
git commit -m "Add domain.ValidationError type for user-facing validation failures"
```

---

## Task 4: CSS — layout primitives, `.badge`, `.card`, and styleguide catalog

Adds the four layout primitives to a new `@layer layout` block and the `.badge`/`.card` component classes to the existing `@layer components` block in `main.css`, then catalogs them on the styleguide page. The styleguide render test is written first and drives the work.

**Files:**
- Create: `cmd/web/handler-styleguide_test.go`
- Modify: `ui/static/main.css`
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml`

- [ ] **Step 1: Write the failing styleguide render test**

Create `cmd/web/handler-styleguide_test.go`:

```go
package main

import (
	"testing"

	"github.com/myrjola/petrapp/internal/e2etest"
	"github.com/myrjola/petrapp/internal/testhelpers"
)

func Test_application_styleguide(t *testing.T) {
	ctx := t.Context()

	server, err := e2etest.StartServer(t, testhelpers.NewWriter(t), testLookupEnv, run)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	client := server.Client()

	doc, err := client.GetDoc(ctx, "/dev/styleguide")
	if err != nil {
		t.Fatalf("Failed to get styleguide: %v", err)
	}

	// Layout primitives.
	if doc.Find("h2:contains('Layout primitives')").Length() == 0 {
		t.Error("expected a 'Layout primitives' section")
	}
	for _, cls := range []string{".stack", ".cluster", ".grid-auto", ".center"} {
		if doc.Find(cls).Length() == 0 {
			t.Errorf("expected a %s example on the styleguide", cls)
		}
	}

	// Badge and card.
	if doc.Find(".badge").Length() == 0 {
		t.Error("expected a .badge example on the styleguide")
	}
	if doc.Find(".badge.badge--success").Length() == 0 {
		t.Error("expected a .badge--success example on the styleguide")
	}
	if doc.Find(".card").Length() == 0 {
		t.Error("expected a .card example on the styleguide")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: FAIL — the styleguide page renders but the assertions fail (sections/classes absent). If it fails with a 404 instead, `devMode` is off in the test environment — stop and investigate `testLookupEnv` rather than working around it.

- [ ] **Step 3: Add the `@layer layout` block to `main.css`**

In `ui/static/main.css`, add a new top-level block. Place it after the `@layer props { ... }` block and before `@layer components { ... }` (matches the `@layer reset, props, layout, components;` declaration order):

```css
@layer layout {
    .stack {
        display: flex;
        flex-direction: column;
        gap: var(--size-4);
    }

    .cluster {
        display: flex;
        flex-wrap: wrap;
        gap: var(--size-2);
        align-items: center;
    }

    .grid-auto {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(12rem, 1fr));
        gap: var(--size-3);
    }

    .center {
        max-width: 32rem;
        margin-inline: auto;
    }
}
```

- [ ] **Step 4: Add `.badge` and `.card` to the `@layer components` block**

In `ui/static/main.css`, inside the existing `@layer components { ... }` block, after the `.sr-only` rule and before the closing brace:

```css
    .badge {
        display: inline-flex;
        align-items: center;
        gap: var(--size-1);
        padding: var(--size-1) var(--size-2);
        border-radius: var(--radius-2);
        font-family: var(--font-sans);
        font-size: var(--font-size-0);
        font-weight: var(--font-weight-6);
        text-transform: uppercase;
        letter-spacing: var(--font-letterspacing-2);
        background: var(--gray-2);
        color: var(--gray-9);
    }

    .badge--success {
        background: var(--lime-2);
        color: var(--lime-9);
    }

    .badge--warning {
        background: var(--yellow-2);
        color: var(--yellow-11);
    }

    .badge--neutral {
        background: var(--gray-2);
        color: var(--gray-9);
    }

    .badge--info {
        background: var(--sky-1);
        color: var(--sky-9);
    }

    .card {
        display: block;
        padding: var(--size-3);
        background: var(--color-surface-elevated);
        border: var(--border-size-1) solid var(--color-border);
        border-radius: var(--radius-3);
        box-shadow: var(--shadow-1);
    }
```

- [ ] **Step 5: Add catalog sections to the styleguide page**

In `ui/templates/pages/styleguide/styleguide.gohtml`, add two new `<section>` blocks just before the existing `<section>` that contains `<h2>Components</h2>`. The `.component-sample` class already exists in the page's scoped styles and gives a padded bordered preview box:

```gohtml
        <section>
            <h2>Layout primitives</h2>

            <h3>.stack — vertical flow</h3>
            <div class="component-sample">
                <div class="stack">
                    <div class="card">First</div>
                    <div class="card">Second</div>
                    <div class="card">Third</div>
                </div>
            </div>

            <h3>.cluster — horizontal wrap</h3>
            <div class="component-sample">
                <div class="cluster">
                    <span class="badge">One</span>
                    <span class="badge">Two</span>
                    <span class="badge">Three</span>
                </div>
            </div>

            <h3>.grid-auto — responsive grid</h3>
            <div class="component-sample">
                <div class="grid-auto">
                    <div class="card">A</div>
                    <div class="card">B</div>
                    <div class="card">C</div>
                    <div class="card">D</div>
                </div>
            </div>

            <h3>.center — readable column</h3>
            <div class="component-sample">
                <div class="center">
                    <div class="card">Centred, width-capped content.</div>
                </div>
            </div>
        </section>

        <section>
            <h2>Badges</h2>
            <div class="component-sample">
                <span class="badge badge--success">Completed</span>
                <span class="badge badge--warning">In progress</span>
                <span class="badge badge--neutral">Not started</span>
                <span class="badge badge--info">Info</span>
            </div>
        </section>

        <section>
            <h2>Card</h2>
            <div class="component-sample">
                <div class="card">A card is a surface with padding, border, radius and a subtle shadow. It composes with any children.</div>
            </div>
        </section>
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: PASS.

- [ ] **Step 7: Lint and full test run**

Run: `make lint-fix && make test`
Expected: no lint errors; all tests pass.

- [ ] **Step 8: Commit**

```bash
git add ui/static/main.css ui/templates/pages/styleguide/styleguide.gohtml cmd/web/handler-styleguide_test.go
git commit -m "Add layout primitives, badge and card classes with styleguide catalog"
```

---

## Task 5: `banner` partial

The `banner` component renders server messages (flash errors and inline notices). Renders nothing when its message is empty; uses `role="alert"` for errors and `role="status"` otherwise.

**Files:**
- Create: `cmd/web/components.go`
- Create: `ui/templates/components/banner.gohtml`
- Modify: `cmd/web/handler-styleguide.go`
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml`
- Modify: `cmd/web/handler-styleguide_test.go`

- [ ] **Step 1: Add failing assertions to the styleguide test**

In `cmd/web/handler-styleguide_test.go`, inside `Test_application_styleguide`, append before the closing brace:

```go
	// Banner component.
	if doc.Find("h2:contains('Banner')").Length() == 0 {
		t.Error("expected a 'Banner' section")
	}
	if doc.Find(".banner.banner--error").Length() == 0 {
		t.Error("expected a .banner--error example on the styleguide")
	}
	if doc.Find(".banner.banner--error[role='alert']").Length() == 0 {
		t.Error("expected the error banner to carry role=alert")
	}
	if doc.Find(".banner.banner--success").Length() == 0 {
		t.Error("expected a .banner--success example on the styleguide")
	}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: FAIL — banner section/classes absent.

- [ ] **Step 3: Create the dot-struct file**

Create `cmd/web/components.go`:

```go
package main

// BannerData is the dot for the `banner` component. Variant is one of
// "error", "success", or "info"; the component renders nothing when
// Message is empty.
type BannerData struct {
	Variant string
	Message string
}
```

- [ ] **Step 4: Create the `banner` partial**

Create `ui/templates/components/banner.gohtml`:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.BannerData*/ -}}
{{- /*
    banner: server-message display for flash errors and inline notices.
    Dot: cmd/web.BannerData. Renders nothing when Message is empty.
    role="alert" for the error variant, role="status" otherwise.

    Usage:
        {{ template "banner" .Flash }}
*/ -}}
{{- define "banner" -}}
    {{- if .Message -}}
        <style {{ nonce }}>
            @scope (.banner) {
                :scope {
                    padding: var(--size-2) var(--size-3);
                    border-radius: var(--radius-2);
                    font-weight: var(--font-weight-5);
                }

                :scope.banner--error {
                    background: var(--red-1);
                    color: var(--red-12);
                }

                :scope.banner--success {
                    background: var(--lime-1);
                    color: var(--lime-9);
                }

                :scope.banner--info {
                    background: var(--sky-1);
                    color: var(--sky-9);
                }
            }
        </style>
        <div class="banner banner--{{ .Variant }}"
             {{ if eq .Variant "error" }}role="alert"{{ else }}role="status"{{ end }}>
            {{ .Message }}
        </div>
    {{- end -}}
{{- end }}
```

- [ ] **Step 5: Provide styleguide example data**

In `cmd/web/handler-styleguide.go`, add a field to `styleguideTemplateData`:

```go
	// BannerExamples drives the Banner section of the styleguide.
	BannerExamples []BannerData
```

And in `styleguideGET`, add to the `styleguideTemplateData{...}` literal:

```go
		BannerExamples: []BannerData{
			{Variant: "error", Message: "Something went wrong. Please try again."},
			{Variant: "success", Message: "Your changes have been saved."},
			{Variant: "info", Message: "Heads up — this is informational."},
		},
```

- [ ] **Step 6: Add the styleguide section**

In `ui/templates/pages/styleguide/styleguide.gohtml`, add before the `<h2>Components</h2>` section (next to the sections added in Task 4):

```gohtml
        <section>
            <h2>Banner</h2>
            <div class="component-sample">
                <div class="stack">
                    {{ range .BannerExamples }}
                        {{ template "banner" . }}
                    {{ end }}
                </div>
            </div>
        </section>
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: PASS.

- [ ] **Step 8: Lint and full test run**

Run: `make lint-fix && make test`
Expected: no lint errors; all tests pass.

- [ ] **Step 9: Commit**

```bash
git add cmd/web/components.go ui/templates/components/banner.gohtml cmd/web/handler-styleguide.go ui/templates/pages/styleguide/styleguide.gohtml cmd/web/handler-styleguide_test.go
git commit -m "Add banner component for server-message display"
```

---

## Task 6: `page-header` partial

The `page-header` component emits a page's single `<h1>` with an optional subtitle. Pages render any meta/badges as siblings after it.

**Files:**
- Modify: `cmd/web/components.go`
- Create: `ui/templates/components/page-header.gohtml`
- Modify: `cmd/web/handler-styleguide.go`
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml`
- Modify: `cmd/web/handler-styleguide_test.go`

- [ ] **Step 1: Add failing assertions to the styleguide test**

In `cmd/web/handler-styleguide_test.go`, inside `Test_application_styleguide`, append before the closing brace:

```go
	// Page-header component.
	if doc.Find("h2:contains('Page header')").Length() == 0 {
		t.Error("expected a 'Page header' section")
	}
	if doc.Find(".page-header h1").Length() == 0 {
		t.Error("expected the page-header example to contain an h1")
	}
	if doc.Find(".page-header .page-header-subtitle").Length() == 0 {
		t.Error("expected the page-header example to contain a subtitle")
	}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: FAIL — page-header section/classes absent.

- [ ] **Step 3: Add the dot struct**

Append to `cmd/web/components.go`:

```go
// PageHeaderData is the dot for the `page-header` component. Subtitle is
// optional and omitted from the output when empty.
type PageHeaderData struct {
	Title    string
	Subtitle string
}
```

- [ ] **Step 4: Create the `page-header` partial**

Create `ui/templates/components/page-header.gohtml`:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.PageHeaderData*/ -}}
{{- /*
    page-header: a page's single <h1> block with an optional subtitle.
    Dot: cmd/web.PageHeaderData. Pages render any meta or badges as
    siblings after this component, not inside it.

    Usage:
        {{ template "page-header" .Header }}
*/ -}}
{{- define "page-header" -}}
    <style {{ nonce }}>
        @scope (.page-header) {
            :scope {
                display: flex;
                flex-direction: column;
                gap: var(--size-1);
            }

            h1 {
                font-size: var(--font-size-5);
                font-weight: var(--font-weight-7);
                color: var(--color-text-primary);
            }

            .page-header-subtitle {
                color: var(--color-text-secondary);
                font-size: var(--font-size-2);
            }
        }
    </style>
    <header class="page-header">
        <h1>{{ .Title }}</h1>
        {{- if .Subtitle }}
            <p class="page-header-subtitle">{{ .Subtitle }}</p>
        {{- end }}
    </header>
{{- end }}
```

- [ ] **Step 5: Provide styleguide example data**

In `cmd/web/handler-styleguide.go`, add a field to `styleguideTemplateData`:

```go
	// PageHeaderExample drives the Page header section of the styleguide.
	PageHeaderExample PageHeaderData
```

And in `styleguideGET`, add to the `styleguideTemplateData{...}` literal:

```go
		PageHeaderExample: PageHeaderData{
			Title:    "Page title",
			Subtitle: "An optional subtitle that explains the page.",
		},
```

- [ ] **Step 6: Add the styleguide section**

In `ui/templates/pages/styleguide/styleguide.gohtml`, add before the `<h2>Components</h2>` section:

```gohtml
        <section>
            <h2>Page header</h2>
            <div class="component-sample">
                {{ template "page-header" .PageHeaderExample }}
            </div>
        </section>
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: PASS.

- [ ] **Step 8: Lint and full test run**

Run: `make lint-fix && make test`
Expected: no lint errors; all tests pass.

- [ ] **Step 9: Commit**

```bash
git add cmd/web/components.go ui/templates/components/page-header.gohtml cmd/web/handler-styleguide.go ui/templates/pages/styleguide/styleguide.gohtml cmd/web/handler-styleguide_test.go
git commit -m "Add page-header component"
```

---

## Task 7: `field` partial

The `field` component is a labelled text input that guarantees the `<label for>` ↔ `<input id>` binding and the `aria-describedby` → hint wiring, and passes native-validation attributes through. It covers single `<input>` fields only; `<select>`, `<textarea>`, and checkbox/radio groups stay as inline markup.

**Files:**
- Modify: `cmd/web/components.go`
- Create: `ui/templates/components/field.gohtml`
- Modify: `cmd/web/handler-styleguide.go`
- Modify: `ui/templates/pages/styleguide/styleguide.gohtml`
- Modify: `cmd/web/handler-styleguide_test.go`

- [ ] **Step 1: Add failing assertions to the styleguide test**

In `cmd/web/handler-styleguide_test.go`, inside `Test_application_styleguide`, append before the closing brace:

```go
	// Field component — the label/input binding is the guarantee under test.
	if doc.Find("h2:contains('Field')").Length() == 0 {
		t.Error("expected a 'Field' section")
	}
	fieldInput := doc.Find(".field input").First()
	if fieldInput.Length() == 0 {
		t.Fatal("expected the field example to contain an input")
	}
	inputID, _ := fieldInput.Attr("id")
	if inputID == "" {
		t.Error("expected the field input to have an id")
	}
	if doc.Find(".field label[for='" + inputID + "']").Length() == 0 {
		t.Errorf("expected a label bound to the input id %q", inputID)
	}
	describedBy, hasDescribedBy := fieldInput.Attr("aria-describedby")
	if !hasDescribedBy {
		t.Error("expected the field input to have aria-describedby (example has a hint)")
	}
	if describedBy != "" && doc.Find("#"+describedBy).Length() == 0 {
		t.Errorf("expected an element with id %q for aria-describedby", describedBy)
	}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: FAIL — field section/markup absent.

- [ ] **Step 3: Add the dot struct**

Append to `cmd/web/components.go`:

```go
// FieldData is the dot for the `field` component — a labelled single text
// input. Name is used as both the input's id and its name attribute. Type
// is an HTML input type ("text", "number", "email", ...). Min, Max, Step
// and Pattern are native-validation attributes, passed through verbatim and
// omitted from the output when empty (they are strings so "0" can be set
// explicitly). Hint, when set, is rendered and wired via aria-describedby.
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
}
```

- [ ] **Step 4: Create the `field` partial**

Create `ui/templates/components/field.gohtml`:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.FieldData*/ -}}
{{- /*
    field: a labelled single text input with native-validation passthrough.
    Dot: cmd/web.FieldData. Guarantees the <label for> ↔ <input id> binding
    and the aria-describedby → hint wiring so callers cannot forget them.
    Covers single <input> fields only; <select>, <textarea> and checkbox or
    radio groups stay as inline markup.

    Usage:
        {{ template "field" .NameField }}
*/ -}}
{{- define "field" -}}
    <style {{ nonce }}>
        @scope (.field) {
            :scope {
                display: flex;
                flex-direction: column;
                gap: var(--size-1);
            }

            label {
                font-weight: var(--font-weight-6);
                color: var(--color-text-primary);
            }

            input {
                padding: var(--size-2);
                border: var(--border-size-1) solid var(--color-border);
                border-radius: var(--radius-2);
                background: var(--color-surface-elevated);
            }

            input:focus-visible {
                outline: var(--color-border-focus) solid 2px;
                outline-offset: 1px;
            }

            .field-hint {
                font-size: var(--font-size-0);
                color: var(--color-text-secondary);
            }
        }
    </style>
    <div class="field">
        <label for="{{ .Name }}">{{ .Label }}</label>
        <input id="{{ .Name }}"
               name="{{ .Name }}"
               type="{{ .Type }}"
               value="{{ .Value }}"
               {{- if .Required }} required{{ end }}
               {{- if .Hint }} aria-describedby="{{ .Name }}-hint"{{ end }}
               {{- if .Min }} min="{{ .Min }}"{{ end }}
               {{- if .Max }} max="{{ .Max }}"{{ end }}
               {{- if .Step }} step="{{ .Step }}"{{ end }}
               {{- if .Pattern }} pattern="{{ .Pattern }}"{{ end }}/>
        {{- if .Hint }}
            <span id="{{ .Name }}-hint" class="field-hint">{{ .Hint }}</span>
        {{- end }}
    </div>
{{- end }}
```

- [ ] **Step 5: Provide styleguide example data**

In `cmd/web/handler-styleguide.go`, add a field to `styleguideTemplateData`:

```go
	// FieldExamples drives the Field section of the styleguide.
	FieldExamples []FieldData
```

And in `styleguideGET`, add to the `styleguideTemplateData{...}` literal:

```go
		FieldExamples: []FieldData{
			{
				Label:    "Exercise name",
				Name:     "styleguide-name",
				Type:     "text",
				Required: true,
				Hint:     "Shown to you when picking exercises.",
			},
			{
				Label: "Target reps",
				Name:  "styleguide-reps",
				Type:  "number",
				Value: "8",
				Min:   "1",
				Max:   "30",
				Step:  "1",
			},
		},
```

The first example has a `Hint`, which the test in Step 1 relies on for the `aria-describedby` assertion.

- [ ] **Step 6: Add the styleguide section**

In `ui/templates/pages/styleguide/styleguide.gohtml`, add before the `<h2>Components</h2>` section:

```gohtml
        <section>
            <h2>Field</h2>
            <div class="component-sample">
                <div class="stack">
                    {{ range .FieldExamples }}
                        {{ template "field" . }}
                    {{ end }}
                </div>
            </div>
        </section>
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `go test -v ./cmd/web -run Test_application_styleguide`
Expected: PASS.

- [ ] **Step 8: Lint and full test run**

Run: `make lint-fix && make test`
Expected: no lint errors; all tests pass.

- [ ] **Step 9: Commit**

```bash
git add cmd/web/components.go ui/templates/components/field.gohtml cmd/web/handler-styleguide.go ui/templates/pages/styleguide/styleguide.gohtml cmd/web/handler-styleguide_test.go
git commit -m "Add field component with label binding and native-validation passthrough"
```

---

## Task 8: Migrate `workout.gohtml` (reference page)

Migrate the workout page to the new foundation: derived state comes from the domain methods (Tasks 1–2) via the handler, the header uses `page-header`, the status uses `.badge`, exercise items use `.card`, and layout uses the primitives. Inline scoped `<style>` remains only for genuinely page-specific styling (the `.exercise` state colours, the sticky footer).

**DOM contracts that MUST be preserved** (existing tests depend on them):
- Each exercise link keeps the `exercise` class (`handler-workout_test.go` counts `a.exercise`).
- Each exercise link keeps `data-workout-exercise-id="..."` (the inline view-transition `<script>` and `playwright_test.go` both select on it).
- The complete-workout submit button keeps the exact text `Complete workout` (`playwright_test.go` selects it by accessible name).
- The inline view-transition `<script>` block is preserved verbatim.

**Files:**
- Modify: `cmd/web/handler-workout.go`
- Modify: `ui/templates/pages/workout/workout.gohtml`

- [ ] **Step 1: Extend `workoutTemplateData` and prepare derived data in `workoutGET`**

In `cmd/web/handler-workout.go`, change the `workoutTemplateData` struct to:

```go
type workoutTemplateData struct {
	BaseTemplateData
	Date          time.Time
	Session       domain.Session
	Header        PageHeaderData
	StatusLabel   string
	StatusVariant string
}
```

Then in `workoutGET`, replace the construction of `data` (the `data := workoutTemplateData{...}` block near the end of the function) with:

```go
	var statusLabel, statusVariant string
	switch session.Status() {
	case domain.SessionCompleted:
		statusLabel, statusVariant = "Completed", "success"
	case domain.SessionInProgress:
		statusLabel, statusVariant = "In Progress", "warning"
	case domain.SessionNotStarted:
		statusLabel, statusVariant = "Not Started", "neutral"
	}

	data := workoutTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Date:             date,
		Session:          session,
		Header: PageHeaderData{
			Title: fmt.Sprintf("%s Workout — %s",
				session.WorkoutType().Label(), date.Format("Monday, January 2, 2006")),
		},
		StatusLabel:   statusLabel,
		StatusVariant: statusVariant,
	}

	app.render(w, r, http.StatusOK, "workout", data)
```

The `switch` covers all three `SessionStatus` constants with no `default`, satisfying the `exhaustive` linter. `fmt` is already imported in this file.

- [ ] **Step 2: Run the handler test to verify the handler still compiles and passes**

Run: `go test -v ./cmd/web -run Test_application_addWorkout`
Expected: PASS — the template still references the old `Session` shape, so this confirms the handler change alone is sound before the template rewrite.

- [ ] **Step 3: Rewrite `workout.gohtml`**

Replace the entire contents of `ui/templates/pages/workout/workout.gohtml` with the following. The `<script {{ nonce }}>` block is carried over verbatim from the current file:

```gohtml
{{- /*gotype: github.com/myrjola/petrapp/cmd/web.workoutTemplateData*/ -}}

{{ define "page" }}
    <main class="stack">
        <style {{ nonce }}>
            @scope {
                :scope {
                    margin: var(--size-4);
                }
            }
        </style>

        {{ template "page-header" .Header }}

        <div class="workout-meta cluster">
            <style {{ nonce }}>
                @scope (.workout-meta) {
                    .progress-summary {
                        color: var(--gray-7);
                        font-size: var(--font-size-1);
                    }
                }
            </style>
            <span class="badge badge--{{ .StatusVariant }}">{{ .StatusLabel }}</span>
            <span class="progress-summary">{{ len .Session.ExerciseSets }} exercises</span>
        </div>

        <div class="exercise-list stack">
            <style {{ nonce }}>
                @scope (.exercise-list) {
                    :scope {
                        gap: var(--size-3);
                    }

                    .exercise {
                        display: flex;
                        align-items: center;
                        text-decoration: line-through;
                        color: var(--gray-5);
                    }

                    a.exercise {
                        text-decoration: none;
                        transition: background-color 0.2s;
                    }

                    a.exercise:hover {
                        background: var(--gray-1);
                    }

                    .exercise.active {
                        text-decoration: none;
                        color: var(--gray-9);
                        font-weight: var(--font-weight-6);
                    }

                    .exercise.completed {
                        background-color: var(--lime-2);
                        color: var(--lime-9);
                    }

                    .exercise.started {
                        background-color: var(--yellow-2);
                        color: var(--yellow-11);
                    }

                    .add-exercise {
                        display: flex;
                        justify-content: center;
                        margin-top: var(--size-2);
                    }

                    .add-exercise-button {
                        background-color: var(--lime-2);
                        color: var(--lime-9);
                        border-radius: var(--radius-2);
                        padding: var(--size-2) var(--size-3);
                        font-weight: var(--font-weight-6);
                        text-decoration: none;
                        display: inline-block;
                    }

                    .add-exercise-button:hover {
                        background-color: var(--lime-3);
                    }
                }
            </style>

            {{ range .Session.ExerciseSets }}
                <a href="/workouts/{{ $.Date.Format "2006-01-02" }}/exercises/{{ .ID }}"
                   class="exercise card{{ if not $.Session.CompletedAt }} active{{ end }} {{ .CompletionState }}"
                   data-workout-exercise-id="{{ .ID }}">
                    <span>{{ .Exercise.Name }}</span>
                </a>
            {{ end }}

            <script {{ nonce }}>
              const exercisePagePattern = new URLPattern(`/workouts/:date/exercises/:exerciseId`, window.origin)

              const extractWorkoutExerciseIdFromUrl = (url) => {
                const match = exercisePagePattern.exec(url)
                return match?.pathname.groups.exerciseId
              }

              // When going to an exercise page, set the exercise title view transition name on the relevant exercise.
              window.addEventListener('pageswap', async (e) => {
                if (!e.viewTransition) return
                const entry = e.activation?.entry
                if (!entry) return
                const targetUrl = new URL(entry.url)
                const exercise = extractWorkoutExerciseIdFromUrl(targetUrl)
                if (!exercise) return
                const element = document.querySelector(`a.exercise[data-workout-exercise-id="${exercise}"] span`)
                if (!element) return
                element.style.viewTransitionName = `exercise-title-${exercise}`
                try {
                  await e.viewTransition.finished
                } catch (_) {
                  // Transition may be aborted; ignore.
                }
                element.style.viewTransitionName = ''
              })

              // When going from an exercise page to the workout page, set view transition names on the relevant
              // exercise in the list.
              window.addEventListener('pagereveal', async (e) => {
                if (!e.viewTransition) return
                const from = navigation.activation?.from
                if (!from) return
                const fromUrl = new URL(from.url)
                const workoutExerciseId = extractWorkoutExerciseIdFromUrl(fromUrl)
                if (!workoutExerciseId) return
                const element = document.querySelector(`a.exercise[data-workout-exercise-id="${workoutExerciseId}"] span`)
                if (!element) return
                element.style.viewTransitionName = `exercise-title-${workoutExerciseId}`
                try {
                  await e.viewTransition.finished
                } catch (_) {
                  // Transition may be aborted if a new navigation starts; ignore.
                }
                element.style.viewTransitionName = ''
              })
            </script>

            <div class="add-exercise">
                <a href="/workouts/{{ .Date.Format "2006-01-02" }}/add-exercise" class="add-exercise-button">
                    Add Exercise
                </a>
            </div>
        </div>

        <div class="complete-workout">
            <style {{ nonce }}>
                @scope (.complete-workout) {
                    :scope {
                        padding: var(--size-4);
                        position: sticky;
                        bottom: 0;
                        background: var(--white);
                        box-shadow: 0 -1px 2px 0 rgb(0 0 0 / 0.05);
                    }

                    button {
                        width: 100%;
                        justify-content: center;
                        font-size: var(--font-size-2);
                        padding: var(--size-3);
                    }
                }
            </style>
            <form method="post" action="/workouts/{{ .Date.Format "2006-01-02" }}/complete">
                <button type="submit">Complete workout</button>
            </form>
        </div>
    </main>
{{ end }}
```

Notes on what changed and what did not:
- The `$hasUpper`/`$hasLower`/`$workoutType` block is gone — the title now comes from `.Header` (handler-prepared from `Session.WorkoutType().Label()`).
- The status-badge if-chain is gone — replaced by `.StatusLabel`/`.StatusVariant` and the `.badge` class.
- The per-exercise `$allSetsCompleted`/`$hasSomeCompleted`/`$isStarted` block is gone — the class now uses `{{ .CompletionState }}`, which is the `ExerciseSet.CompletionState()` domain method (returns `not-started`, `started`, or `completed`).
- `{{ if not $.Session.CompletedAt }} active{{ end }}` is **kept verbatim** — it is a single-field display conditional, which is allowed; do not "fix" it as part of this migration.
- `.exercise` keeps its class and `data-workout-exercise-id`; the `card` class is added alongside `.exercise` so the exercise items demonstrate the `.card` surface while the scoped `.exercise` rules keep the state colours and strike-through.
- `<main>`, `.exercise-list`, and `.workout-meta` now use the `stack`/`cluster` layout primitives; their scoped `<style>` blocks shrink to only the page-specific bits.

- [ ] **Step 4: Run the workout handler tests**

Run: `go test -v ./cmd/web -run 'Test_application_addWorkout|Test_application_workoutNotFound'`
Expected: PASS. `a.exercise` is still counted correctly; the `Add Exercise` heading test is unaffected.

- [ ] **Step 5: Run the full test suite**

Run: `make test`
Expected: PASS — including `playwright_test.go`, which selects exercises by `a[data-workout-exercise-id]` and the `Complete workout` button by accessible name, both preserved. If Playwright tests are skipped in this environment (no browser installed), note that and rely on the handler tests; otherwise they must pass.

- [ ] **Step 6: Lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 7: Manually verify in the browser**

Run the dev server (`make dev`), open a workout page with a started session, and confirm: the header renders via `page-header`, the status badge shows the right colour, exercise items render as cards with correct completed/started state, and navigating into and back out of an exercise still runs the view transition.

- [ ] **Step 8: Commit**

```bash
git add cmd/web/handler-workout.go ui/templates/pages/workout/workout.gohtml
git commit -m "Migrate workout page to the frontend foundation"
```

---

## Task 9: Update convention docs

Document the foundation so future page migrations follow it without re-deriving the rules.

**Files:**
- Modify: `ui/templates/CLAUDE.md`
- Modify: `cmd/web/CLAUDE.md`

- [ ] **Step 1: Add the guarantee-based split and component inventory to `ui/templates/CLAUDE.md`**

In `ui/templates/CLAUDE.md`, in the "Shared Components" section, replace the "Current Components" subsection (which currently lists only `back-link`) with a section that documents the full inventory and the delivery rule. Insert this content (keep the existing `back-link` usage example within it):

```markdown
### Component delivery — the guarantee-based split

A component's delivery mechanism is decided by what it must *guarantee*:

- **Go partial** (`components/*.gohtml`, colocated `@scope` `<style>`) — for
  pieces that enforce accessibility or structure: `field`, `banner`,
  `page-header`, `back-link`. The caller passes a small dot and cannot forget
  the a11y wiring. Dot structs live in `cmd/web/components.go`.
- **CSS class** (`main.css @layer components`) — for pure-paint pieces on a
  semantic element: `button`/`.btn`, `.badge`, `.card`. These compose freely
  and have zero per-render cost.
- **Layout primitive class** (`main.css @layer layout`) — `.stack`, `.cluster`,
  `.grid-auto`, `.center`. Reach for these before writing
  `display:flex; gap:…` in a scoped block.
- **Inline scoped `<style>`** — the escape hatch for genuinely page-specific
  composition. Still fine; just not the first reach for layout or for the
  pieces above.

### Current components

**Partials** (call via `{{ template "name" <dot> }}`):

- `back-link` — "← Back" anchor wired to the Navigation API. Dot: an href
  string.
  ```gohtml
  {{ template "back-link" "/" }}
  {{ template "back-link" (printf "/workouts/%s" (.Date.Format "2006-01-02")) }}
  ```
- `banner` — server-message display (flash errors, notices). Dot:
  `cmd/web.BannerData` (`Variant` ∈ `error`/`success`/`info`, `Message`).
  Renders nothing when `Message` is empty.
- `page-header` — a page's single `<h1>` with optional subtitle. Dot:
  `cmd/web.PageHeaderData` (`Title`, `Subtitle`). Render meta/badges as
  siblings after it, not inside it.
- `field` — a labelled single text input; guarantees the `<label for>` ↔
  `<input id>` binding and `aria-describedby` → hint wiring. Dot:
  `cmd/web.FieldData`. Covers `<input>` only — `<select>`, `<textarea>` and
  checkbox/radio groups stay as inline markup.

**Class-components** (`main.css @layer components`): `.btn`/`button`,
`.badge` (+ `--success`/`--warning`/`--neutral`/`--info`), `.card`.

**Layout primitives** (`main.css @layer layout`): `.stack`, `.cluster`,
`.grid-auto`, `.center`.

The `/dev/styleguide` page is the living catalog — add an entry there for any
new component, and assert it in `cmd/web/handler-styleguide_test.go`.
```

- [ ] **Step 2: Add the validation-error flow to `cmd/web/CLAUDE.md`**

In `cmd/web/CLAUDE.md`, in the "Error Handling" section, add a new subsection after "Service Layer Error Handling":

```markdown
### User-facing validation errors

Validation failures that should be shown to the user (not 500s) flow through
`domain.ValidationError`:

1. Domain/service code returns a `domain.ValidationError{Message: "..."}` —
   the `Message` is safe to display verbatim.
2. The handler detects it with `errors.As`, calls
   `app.putFlashError(r.Context(), ve.Message)`, and redirects to the form
   with `redirect(w, r, formPath)`.
3. The form's GET handler pops the flash with `app.popFlashError(...)` and
   passes it to the template as a `BannerData{Variant: "error", Message: ...}`.
4. The form template renders it with `{{ template "banner" .Flash }}`.

This keeps the stack-navigator wire protocol uniform (a `200 + X-Location`
back to the form URL) — see `docs/superpowers/specs/2026-05-14-frontend-foundation-design.md`.
Native HTML validation attributes (`required`, `min`, `pattern`, …) handle the
client-side field UX; the `field` component passes them through. There is no
per-field server-side error channel and no submitted-value preservation.
```

- [ ] **Step 3: Verify the docs render and reference real paths**

Re-read both edited sections. Confirm every referenced path (`cmd/web/components.go`, `cmd/web/handler-styleguide_test.go`, the spec path) exists, and every referenced component name matches a `{{ define }}` block or CSS class actually created in Tasks 4–8.

- [ ] **Step 4: Commit**

```bash
git add ui/templates/CLAUDE.md cmd/web/CLAUDE.md
git commit -m "Document the frontend foundation conventions"
```

---

## Self-Review

**Spec coverage:**
- Guarantee-based split → documented in Task 9; realised across Tasks 4–8.
- CSS structure (`@layer layout` additions, `.badge`/`.card` in `@layer components`) → Task 4.
- Partials `banner`, `page-header`, `field` → Tasks 5, 6, 7.
- Class-components `.badge`, `.card` → Task 4.
- Layout primitives `.stack`, `.cluster`, `.grid-auto`, `.center` → Task 4.
- Forms & validation: native HTML validation via `field` passthrough → Task 7; `domain.ValidationError` infrastructure → Task 3; banner display + flow → Tasks 5 and 9.
- Template-logic discipline: `Session.WorkoutType`/`Category.Label` → Task 1; `Session.Status`/`ExerciseSet.CompletionState` → Task 2; applied in `workout.gohtml` → Task 8.
- Reference page `workout.gohtml` migrated → Task 8.
- Styleguide as living catalog → Tasks 4–7.
- Conventions in `ui/templates/CLAUDE.md` + `cmd/web/CLAUDE.md` → Task 9.
- Testing: domain unit tests (Tasks 1–3), styleguide render test (Tasks 4–7), workout handler + Playwright tests stay green (Task 8). No gaps.

**Type consistency:** `BannerData{Variant, Message}`, `PageHeaderData{Title, Subtitle}`, `FieldData{Label, Name, Type, Value, Required, Hint, Min, Max, Step, Pattern}` are defined once in `cmd/web/components.go` (Tasks 5–7) and consumed with those exact field names in the partials, the styleguide handler, and (for `PageHeaderData`) `workoutTemplateData`. Domain methods `Category.Label() string`, `Session.WorkoutType() Category`, `Session.Status() SessionStatus`, `ExerciseSet.CompletionState() ExerciseSetState` are defined in Tasks 1–2 and consumed in Task 8 with matching signatures. `SessionStatus` constants (`SessionNotStarted`/`SessionInProgress`/`SessionCompleted`) are used in the Task 8 `switch`. All consistent.

**Placeholder scan:** No TBD/TODO; every code step shows complete content; the verbatim view-transition script is reproduced in full in Task 8.
