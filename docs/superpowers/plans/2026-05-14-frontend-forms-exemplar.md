# Frontend Forms Exemplar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate the admin-exercises form pages to the frontend foundation as the forms-side reference page — validation rules become a typed domain method, the handler routes failures through the `errors.As` → flash → `banner` flow, and the templates adopt the `field`, `banner`, and `page-header` partials plus the layout primitives.

**Architecture:** Work outward from the data model. A pure `Exercise.Validate()` method in `internal/domain` is the single source of truth for exercise-form validation, returning `domain.ValidationError` on the first failed rule. `Service.UpdateExercise` calls it before persisting; `Service.GenerateExercise` guards an empty name. The handlers parse form input, build a `domain.Exercise`, detect `domain.ValidationError` with `errors.As`, and surface it via `putFlashError` + redirect-to-form; the form's GET handler pops the flash into a `BannerData` field. The templates render the header via `page-header`, server messages via `banner`, single text/number inputs via `field`, and use `.card`/`.stack` for layout.

**Tech Stack:** Go `html/template` (runtime-loaded via `os.DirFS`), `net/http` stdlib router, CSS with `@layer` + `@scope` + open-props-derived tokens, strict CSP (nonces + Trusted Types), `e2etest` package + `goquery` for handler tests.

**Spec:** `docs/superpowers/specs/2026-05-14-frontend-forms-exemplar-design.md`

---

## Background for the implementer

- Templates and `ui/static/` assets load from the filesystem at runtime — editing a template and refreshing the browser is the whole dev loop, no rebuild.
- Every `<style>` and `<script>` tag in a template MUST carry the `{{ nonce }}` attribute or the CSP drops it. Inline `style="..."` attributes are forbidden.
- Components live in `ui/templates/components/*.gohtml` and are parsed alongside every page; call them as `{{ template "name" <dot> }}`. The dot structs (`BannerData`, `PageHeaderData`, `FieldData`) are already defined in `cmd/web/components.go` — do not redefine them.
- Go templates cannot construct structs. Any struct a template needs (a `FieldData`, a `BannerData`) must be built in the Go handler and passed in.
- The `exhaustruct` linter is on for `cmd/web`. Fully-populated struct literals are fine; a partial literal needs a `//nolint:exhaustruct // <reason>` comment on the opening brace line. `PageHeaderData` literals set `Subtitle: ""` explicitly rather than using a nolint (see `handler-workout.go` for the precedent). `FieldData` partial literals use the nolint (see `handler-styleguide.go`).
- Project style: comments end with a period; error types suffixed `Error`; exhaustive enum `switch`; no magic numbers (`mnd` linter — name constants for non-trivial literals); `wrapcheck` wants errors from other packages wrapped with `fmt.Errorf(... %w ...)`.
- Run a single Go test: `go test -v ./internal/domain -run TestName` or `go test -v ./cmd/web -run TestName`. Full suite: `make test`. Linter with autofix: `make lint-fix`. Full gate: `make ci`.
- `e2etest`'s `SubmitForm` resolves each `formFields` key, in order, by: (1) submit-button name/value, (2) an input/textarea/select with `name == key`, (3) a `<label>` whose text *contains* the key, resolved to its bound control. The existing handler test submits keys `"Name"`, `"Category"`, `"Type"`, `"Primary"`, `"Secondary"`, `"Description"`, `"rep_min"`, `"rep_max"` — so the migrated templates must keep `name="rep_min"`/`name="rep_max"` exact, and keep label text that *contains* `Name`/`Category`/`Type`/`Primary`/`Secondary`/`Description`.

---

## File Structure

**Modified:**
- `internal/domain/exercise.go` — add `Exercise.Validate()`.
- `internal/domain/exercise_test.go` — add `Test_Exercise_Validate`.
- `internal/service/exercises.go` — `UpdateExercise` calls `Validate()`.
- `internal/service/exercise_generation.go` — `GenerateExercise` guards empty name.
- `internal/service/exercises_test.go` — add `Test_UpdateExercise_RejectsInvalidExercise` and `Test_GenerateExercise_RejectsEmptyName`.
- `cmd/web/handler-admin-exercises.go` — new template structs (`adminExerciseRow`, `selectOption`, `Header`/`Flash`/`NameField` fields), `errors.As` control flow in both POST handlers, flash-pop in both GET handlers, the `optionalInt` and `buildMuscleGroupOptions` helpers, `parseRepWindow` deleted, `funlen` suppression removed.
- `cmd/web/handler-admin-exercises_test.go` — add the generate-form empty-name subtest.
- `ui/templates/pages/admin-exercises/admin-exercises.gohtml` — migrated.
- `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml` — migrated.

**No doc changes:** `cmd/web/CLAUDE.md` already documents the validation-error flow as a go-forward convention and does not enumerate call sites; `ui/templates/CLAUDE.md` already lists the components. Neither needs updating.

---

## Task 1: Domain — `Exercise.Validate()`

Moves the six validation rules currently inlined in `adminExerciseUpdatePOST` onto a pure, testable domain method. It returns `domain.ValidationError` on the first failed rule. The messages and check order preserve today's handler behaviour.

**Files:**
- Modify: `internal/domain/exercise.go`
- Test: `internal/domain/exercise_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/domain/exercise_test.go`. The file is `package domain_test` and currently imports only `testing` and `github.com/myrjola/petrapp/internal/domain` — add `errors` to the import block.

```go
func Test_Exercise_Validate(t *testing.T) {
	intPtr := func(n int) *int { return &n }
	validWeighted := func() domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // test builder sets only the validated fields.
			Name:                "Bench Press",
			Category:            domain.CategoryUpper,
			ExerciseType:        domain.ExerciseTypeWeighted,
			PrimaryMuscleGroups: []string{"Chest"},
			RepMin:              intPtr(5),
			RepMax:              intPtr(10),
		}
	}
	validTimed := func() domain.Exercise {
		return domain.Exercise{ //nolint:exhaustruct // test builder sets only the validated fields.
			Name:                   "Plank",
			Category:               domain.CategoryFullBody,
			ExerciseType:           domain.ExerciseTypeTime,
			PrimaryMuscleGroups:    []string{"Core"},
			DefaultStartingSeconds: intPtr(30),
		}
	}

	cases := []struct {
		name        string
		exercise    domain.Exercise
		wantErr     bool
		wantMessage string
	}{
		{"valid weighted", validWeighted(), false, ""},
		{"valid timed", validTimed(), false, ""},
		{
			"empty name",
			func() domain.Exercise { e := validWeighted(); e.Name = ""; return e }(),
			true, "Name is required.",
		},
		{
			"invalid category",
			func() domain.Exercise { e := validWeighted(); e.Category = domain.Category("bogus"); return e }(),
			true, "Category must be one of full body, upper, or lower.",
		},
		{
			"invalid type",
			func() domain.Exercise { e := validWeighted(); e.ExerciseType = domain.ExerciseType("bogus"); return e }(),
			true, "Exercise type must be weighted, bodyweight, assisted, or time_based.",
		},
		{
			"timed without seconds",
			func() domain.Exercise { e := validTimed(); e.DefaultStartingSeconds = nil; return e }(),
			true, "Default starting seconds must be a positive integer for time-based exercises.",
		},
		{
			"timed with zero seconds",
			func() domain.Exercise { e := validTimed(); e.DefaultStartingSeconds = intPtr(0); return e }(),
			true, "Default starting seconds must be a positive integer for time-based exercises.",
		},
		{
			"no primary muscles",
			func() domain.Exercise { e := validWeighted(); e.PrimaryMuscleGroups = nil; return e }(),
			true, "At least one primary muscle group is required.",
		},
		{
			"missing rep window",
			func() domain.Exercise { e := validWeighted(); e.RepMin = nil; e.RepMax = nil; return e }(),
			true, "Min and max reps must be whole numbers between 1 and 50.",
		},
		{
			"rep window out of range",
			func() domain.Exercise { e := validWeighted(); e.RepMax = intPtr(99); return e }(),
			true, "Min and max reps must be whole numbers between 1 and 50.",
		},
		{
			"rep min greater than max",
			func() domain.Exercise { e := validWeighted(); e.RepMin = intPtr(12); e.RepMax = intPtr(8); return e }(),
			true, "Min reps must be less than or equal to max reps.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.exercise.Validate()
			if !tc.wantErr {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error %q", tc.wantMessage)
			}
			var ve domain.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("Validate() error is not a ValidationError: %v", err)
			}
			if ve.Message != tc.wantMessage {
				t.Errorf("Validate() message = %q, want %q", ve.Message, tc.wantMessage)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./internal/domain -run Test_Exercise_Validate`
Expected: FAIL — `e.Validate undefined`.

- [ ] **Step 3: Implement `Exercise.Validate()`**

Add to `internal/domain/exercise.go`, at the end of the file (after `FormatSetDescription`):

```go
// Validate reports whether the exercise's fields form a persistable record.
// It returns a ValidationError carrying a user-facing message on the first
// rule it fails, and nil when every rule passes. It is the single source of
// truth for exercise-form validation; handlers detect the ValidationError
// with errors.As and surface it via the flash + banner flow. Validate checks
// only that the populated fields are valid and that the fields required for
// the exercise type are present — it does not cross-check that a timed
// exercise lacks a rep window, because handler struct-shaping guarantees it.
func (e Exercise) Validate() error {
	const (
		repBoundMin = 1
		repBoundMax = 50
	)
	if e.Name == "" {
		return ValidationError{Message: "Name is required."}
	}
	if !e.Category.IsValid() {
		return ValidationError{Message: "Category must be one of full body, upper, or lower."}
	}
	if !e.ExerciseType.IsValid() {
		return ValidationError{Message: "Exercise type must be weighted, bodyweight, assisted, or time_based."}
	}
	if e.IsTimed() && (e.DefaultStartingSeconds == nil || *e.DefaultStartingSeconds <= 0) {
		return ValidationError{
			Message: "Default starting seconds must be a positive integer for time-based exercises.",
		}
	}
	if len(e.PrimaryMuscleGroups) == 0 {
		return ValidationError{Message: "At least one primary muscle group is required."}
	}
	if !e.IsTimed() {
		if e.RepMin == nil || e.RepMax == nil ||
			*e.RepMin < repBoundMin || *e.RepMin > repBoundMax ||
			*e.RepMax < repBoundMin || *e.RepMax > repBoundMax {
			return ValidationError{Message: "Min and max reps must be whole numbers between 1 and 50."}
		}
		if *e.RepMin > *e.RepMax {
			return ValidationError{Message: "Min reps must be less than or equal to max reps."}
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -v ./internal/domain -run Test_Exercise_Validate`
Expected: PASS.

- [ ] **Step 5: Lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/exercise.go internal/domain/exercise_test.go
git commit -m "Add Exercise.Validate domain method"
```

---

## Task 2: Service — wire validation into `UpdateExercise` and `GenerateExercise`

`Service.UpdateExercise` validates before persisting, so the invariant is enforced for every caller. `Service.GenerateExercise` guards the empty-name case (today an empty name reaches `createMinimalExercise` and persists a nameless exercise; the handler is the only thing that rejects it, with a raw 500).

**Files:**
- Modify: `internal/service/exercises.go`
- Modify: `internal/service/exercise_generation.go`
- Test: `internal/service/exercises_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/service/exercises_test.go`. The file is `package service_test` and imports `context`, `testing`, `time`, plus the project packages — add `errors` to the import block.

```go
func Test_UpdateExercise_RejectsInvalidExercise(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	svc := service.NewService(db, logger, "")

	invalid := domain.Exercise{ //nolint:exhaustruct // intentionally invalid: empty name.
		ID:           1,
		Category:     domain.CategoryUpper,
		ExerciseType: domain.ExerciseTypeWeighted,
	}
	err = svc.UpdateExercise(ctx, invalid)
	if err == nil {
		t.Fatal("UpdateExercise() = nil, want ValidationError")
	}
	var ve domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("UpdateExercise() error is not a ValidationError: %v", err)
	}
	if ve.Message != "Name is required." {
		t.Errorf("message = %q, want %q", ve.Message, "Name is required.")
	}
}

func Test_GenerateExercise_RejectsEmptyName(t *testing.T) {
	ctx := t.Context()
	logger := testhelpers.NewLogger(testhelpers.NewWriter(t))
	db, err := sqlite.NewDatabase(ctx, ":memory:", logger)
	if err != nil {
		t.Fatalf("create test database: %v", err)
	}
	defer func() { _ = db.Close() }()

	svc := service.NewService(db, logger, "")

	_, err = svc.GenerateExercise(ctx, "")
	if err == nil {
		t.Fatal("GenerateExercise(\"\") = nil, want ValidationError")
	}
	var ve domain.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("GenerateExercise(\"\") error is not a ValidationError: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -v ./internal/service -run 'Test_UpdateExercise_RejectsInvalidExercise|Test_GenerateExercise_RejectsEmptyName'`
Expected: FAIL — `UpdateExercise` currently persists without validating (the `errors.As` assertion fails), and `GenerateExercise("")` currently succeeds.

- [ ] **Step 3: Implement the `UpdateExercise` validation call**

In `internal/service/exercises.go`, replace the `UpdateExercise` method body so it validates first:

```go
// UpdateExercise validates an exercise and updates the existing record.
func (s *Service) UpdateExercise(ctx context.Context, ex domain.Exercise) error {
	if err := ex.Validate(); err != nil {
		return fmt.Errorf("validate exercise: %w", err)
	}
	if err := s.repos.Exercises.Update(ctx, ex.ID, func(oldEx *domain.Exercise) error {
		*oldEx = ex
		return nil
	}); err != nil {
		return fmt.Errorf("update exercise: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Implement the `GenerateExercise` empty-name guard**

In `internal/service/exercise_generation.go`, add the guard as the first statement of `GenerateExercise`:

```go
func (s *Service) GenerateExercise(ctx context.Context, name string) (domain.Exercise, error) {
	if name == "" {
		return domain.Exercise{}, domain.ValidationError{Message: "Exercise name is required."}
	}
	exercise := s.generateExerciseContent(ctx, name)

	persisted, err := s.repos.Exercises.Create(ctx, exercise)
	if err != nil {
		return domain.Exercise{}, fmt.Errorf("create exercise: %w", err)
	}

	return persisted, nil
}
```

If `make lint-fix` in Step 6 flags `wrapcheck` on the direct `domain.ValidationError{...}` return, wrap it: `return domain.Exercise{}, fmt.Errorf("generate exercise: %w", domain.ValidationError{Message: "Exercise name is required."})` — `errors.As` still traverses the wrap.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test -v ./internal/service -run 'Test_UpdateExercise_RejectsInvalidExercise|Test_GenerateExercise_RejectsEmptyName|Test_UpdateExercise_PreservesExerciseSets'`
Expected: PASS for all three. `Test_UpdateExercise_PreservesExerciseSets` updates a valid weighted exercise (name set, category lower, primary muscles set, rep window 5–10), so the new `Validate()` call passes and the test stays green.

- [ ] **Step 6: Lint**

Run: `make lint-fix`
Expected: no errors (apply the `wrapcheck` fallback from Step 4 if needed).

- [ ] **Step 7: Commit**

```bash
git add internal/service/exercises.go internal/service/exercise_generation.go internal/service/exercises_test.go
git commit -m "Wire exercise validation into the service layer"
```

---

## Task 3: Migrate the admin-exercises list page (handler + template)

Migrates `adminExercisesGET` and `adminExerciseGeneratePOST` plus `admin-exercises.gohtml`. The list rows get their derived display values (category label, comma-joined muscle lists) from the handler; the generate form's empty-name failure now flows through `errors.As` → flash → `banner` instead of a 500. The handler and template change together because the template references the new struct fields.

**Files:**
- Modify: `cmd/web/handler-admin-exercises.go`
- Modify: `ui/templates/pages/admin-exercises/admin-exercises.gohtml`
- Test: `cmd/web/handler-admin-exercises_test.go`

- [ ] **Step 1: Add the failing handler subtest**

In `cmd/web/handler-admin-exercises_test.go`, inside `Test_application_adminExercises`, append this subtest as the last `t.Run(...)` block before the function's closing brace. `strings` is already imported.

```go
	// The generate form's empty-name case must surface as a flash banner on the
	// admin page, not a 500. Today the handler returns serverError(w, r, nil).
	t.Run("Generate with empty name shows validation error", func(t *testing.T) {
		if doc, err = client.GetDoc(ctx, "/admin/exercises"); err != nil {
			t.Fatalf("Failed to get admin exercises page: %v", err)
		}
		formData := map[string]string{"Name": ""}
		if doc, err = client.SubmitForm(ctx, doc, "/admin/exercises/generate", formData); err != nil {
			t.Fatalf("Failed to submit generate form with empty name: %v", err)
		}
		if doc.Find("h1").Text() != "Exercise Administration" {
			t.Error("Expected to land back on the exercise admin page")
		}
		if doc.Find("[role=alert]").Length() == 0 {
			t.Error("Expected a validation alert after submitting an empty exercise name")
		}
		if !strings.Contains(doc.Find("[role=alert]").Text(), "Exercise name is required") {
			t.Errorf("Expected 'Exercise name is required' in alert, got: %s",
				doc.Find("[role=alert]").Text())
		}
	})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -v ./cmd/web -run Test_application_adminExercises`
Expected: FAIL — the new subtest fails because `SubmitForm` gets a 500 (`adminExerciseGeneratePOST` currently calls `serverError(w, r, nil)` for an empty name).

- [ ] **Step 3: Update the imports, structs, and helpers in `handler-admin-exercises.go`**

In `cmd/web/handler-admin-exercises.go`, change the import block to add `errors` and `strings`:

```go
import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/myrjola/petrapp/internal/domain"
)
```

Replace the `exerciseAdminTemplateData` struct definition with the new shape and add the `adminExerciseRow` and `selectOption` types (put `adminExerciseRow` directly above `exerciseAdminTemplateData`, and `selectOption` directly above `exerciseEditTemplateData`):

```go
// adminExerciseRow is one row of the exercise admin table, with display
// values (category label, comma-joined muscle lists) prepared by the handler.
type adminExerciseRow struct {
	ID               int
	Name             string
	CategoryLabel    string
	PrimaryMuscles   string
	SecondaryMuscles string
}

// exerciseAdminTemplateData contains data for the exercise admin template.
type exerciseAdminTemplateData struct {
	BaseTemplateData
	Header    PageHeaderData
	Flash     BannerData
	Exercises []adminExerciseRow
	NameField FieldData
}
```

```go
// selectOption is one <option> of a single-select dropdown, with its
// selected state resolved by the handler.
type selectOption struct {
	Value    string
	Label    string
	Selected bool
}
```

Add these two helpers at the end of the file (after `adminExerciseGeneratePOST`; `parseRepWindow` is being removed in Step 4):

```go
// optionalInt parses a form field into an *int, returning nil for an empty or
// unparseable value. Native HTML validation guards the client; Exercise.Validate
// is the backstop for malformed input that reaches the server anyway.
func optionalInt(raw string) *int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &n
}

// buildMuscleGroupOptions pairs every muscle group with whether it appears in
// the selected list, for rendering a <select multiple>.
func buildMuscleGroupOptions(groups, selected []string) []MuscleGroupOption {
	options := make([]MuscleGroupOption, len(groups))
	for i, group := range groups {
		options[i] = MuscleGroupOption{
			Name:     group,
			Selected: slices.Contains(selected, group),
		}
	}
	return options
}
```

- [ ] **Step 4: Rewrite `adminExercisesGET` and `adminExerciseGeneratePOST`, and delete `parseRepWindow`**

In `cmd/web/handler-admin-exercises.go`, replace the whole `adminExercisesGET` function with:

```go
// adminExercisesGET handles GET requests to the exercise admin page.
func (app *application) adminExercisesGET(w http.ResponseWriter, r *http.Request) {
	exercises, err := app.service.List(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	rows := make([]adminExerciseRow, 0, len(exercises))
	for _, ex := range exercises {
		rows = append(rows, adminExerciseRow{
			ID:               ex.ID,
			Name:             ex.Name,
			CategoryLabel:    ex.Category.Label(),
			PrimaryMuscles:   strings.Join(ex.PrimaryMuscleGroups, ", "),
			SecondaryMuscles: strings.Join(ex.SecondaryMuscleGroups, ", "),
		})
	}

	data := exerciseAdminTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Header:           PageHeaderData{Title: "Exercise Administration", Subtitle: ""},
		Flash:            BannerData{Variant: "error", Message: app.popFlashError(r.Context())},
		Exercises:        rows,
		NameField: FieldData{ //nolint:exhaustruct // labelled text input; native-validation attrs unused here.
			Label:    "Exercise Name",
			Name:     "name",
			Type:     "text",
			Required: true,
			Hint:     "e.g., Bench Press, Deadlift, Squat",
		},
	}

	app.render(w, r, http.StatusOK, "admin-exercises", data)
}
```

Replace the whole `adminExerciseGeneratePOST` function with:

```go
// adminExerciseGeneratePOST handles POST requests to generate a new exercise.
func (app *application) adminExerciseGeneratePOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, defaultMaxFormSize)
	if err := r.ParseForm(); err != nil {
		app.serverError(w, r, err)
		return
	}

	name := r.PostForm.Get("name")
	exercise, err := app.service.GenerateExercise(r.Context(), name)
	if err != nil {
		var ve domain.ValidationError
		if errors.As(err, &ve) {
			app.putFlashError(r.Context(), ve.Message)
			redirect(w, r, "/admin/exercises")
			return
		}
		app.serverError(w, r, err)
		return
	}

	redirect(w, r, fmt.Sprintf("/admin/exercises/%d", exercise.ID))
}
```

**Do not delete `parseRepWindow` in this task.** `adminExerciseUpdatePOST` still calls it and is not rewritten until Task 4 — leaving it in place keeps `cmd/web` compiling so Task 3 commits independently with a green build. Task 4 deletes it once `adminExerciseUpdatePOST` no longer references it.

Note: `adminExercisesGET` no longer calls `app.service.ListMuscleGroups` — the list page never rendered the muscle-group list. `adminExerciseEditGET` still calls it; do not remove it there.

- [ ] **Step 5: Rewrite `admin-exercises.gohtml`**

Replace the entire contents of `ui/templates/pages/admin-exercises/admin-exercises.gohtml` with:

```gohtml
{{- /* gotype: github.com/myrjola/petrapp/cmd/web.exerciseAdminTemplateData */ -}}
{{ define "page" }}
    <main class="stack">
        {{ template "page-header" .Header }}
        {{ template "banner" .Flash }}

        <section class="stack">
            <h2>Exercises</h2>
            <table>
                <thead>
                <tr>
                    <th>ID</th>
                    <th>Name</th>
                    <th>Category</th>
                    <th>Primary Muscles</th>
                    <th>Secondary Muscles</th>
                    <th>Actions</th>
                </tr>
                </thead>
                <tbody>
                {{ range .Exercises }}
                    <tr>
                        <td>{{ .ID }}</td>
                        <td>{{ .Name }}</td>
                        <td>{{ .CategoryLabel }}</td>
                        <td>{{ .PrimaryMuscles }}</td>
                        <td>{{ .SecondaryMuscles }}</td>
                        <td>
                            <a href="/admin/exercises/{{ .ID }}">Edit</a>
                        </td>
                    </tr>
                {{ end }}
                </tbody>
            </table>
        </section>

        <section class="card stack">
            <h2>Generate New Exercise</h2>
            <form method="post" action="/admin/exercises/generate" class="stack">
                {{ template "field" .NameField }}
                <button type="submit">Generate Exercise</button>
            </form>
        </section>
    </main>
{{ end }}
```

- [ ] **Step 6: Build to confirm the package still compiles**

Run: `go build ./cmd/web/`
Expected: PASS — `adminExercisesGET`/`adminExerciseGeneratePOST` compile against the new structs, and `parseRepWindow` is still present (it is deleted in Task 4), so `adminExerciseUpdatePOST` still compiles. The `admin-exercises.gohtml` template is not type-checked at build time. Do not run the handler tests yet — they exercise `adminExerciseEditGET`/`adminExerciseUpdatePOST`, which still use the old structs but are unchanged, so they pass; the rep-window subtests also still pass against the old `adminExerciseUpdatePOST`. If you want, run `go test ./cmd/web -run Test_application_adminExercises` here: the original subtests pass, the new "Generate with empty name" subtest passes (the generate path is fully migrated), confirming Task 3 is sound.

- [ ] **Step 7: Lint**

Run: `make lint-fix`
Expected: no errors.

- [ ] **Step 8: Commit**

```bash
git add cmd/web/handler-admin-exercises.go ui/templates/pages/admin-exercises/admin-exercises.gohtml cmd/web/handler-admin-exercises_test.go
git commit -m "Migrate admin-exercises list page to the frontend foundation"
```

---

## Task 4: Migrate the admin-exercise-edit page (handler + template)

Migrates `adminExerciseEditGET` and `adminExerciseUpdatePOST` plus `admin-exercise-edit.gohtml`. The GET handler prepares the header, the flash banner, the `field` dots for the four `<input>`s, and the category/type `<select>` options; the POST handler parses the form, builds a `domain.Exercise`, and routes a `domain.ValidationError` through `errors.As` → flash → redirect. The existing handler subtests ("Edit exercise", "Empty name shows validation error", "Edit exercise with 15KB description", "Invalid rep window shows validation error") are the regression net — they must stay green because the messages and behaviour are preserved.

**Files:**
- Modify: `cmd/web/handler-admin-exercises.go`
- Modify: `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml`

- [ ] **Step 1: Confirm the baseline — existing edit-page subtests pass**

Run: `go test -v ./cmd/web -run Test_application_adminExercises`
Expected: PASS — all subtests, including the four edit-page subtests and the new generate subtest from Task 3. This establishes the regression baseline before the refactor.

- [ ] **Step 2: Rewrite the `exerciseEditTemplateData` struct**

In `cmd/web/handler-admin-exercises.go`, replace the `exerciseEditTemplateData` struct definition with:

```go
// exerciseEditTemplateData contains data for the exercise edit template.
type exerciseEditTemplateData struct {
	BaseTemplateData
	Header                 PageHeaderData
	Flash                  BannerData
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

The `ValidationError`, `DefaultStartingSecondsValue`, `RepMinValue`, and `RepMaxValue` fields are removed — their values now live inside the `Flash` banner and the `FieldData` dots.

- [ ] **Step 3: Rewrite `adminExerciseEditGET`**

In `cmd/web/handler-admin-exercises.go`, replace the whole `adminExerciseEditGET` function with:

```go
// adminExerciseEditGET handles GET requests to the exercise edit page.
func (app *application) adminExerciseEditGET(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		app.notFound(w, r)
		return
	}

	exercise, err := app.service.GetExercise(r.Context(), id)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	muscleGroups, err := app.service.ListMuscleGroups(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	defaultSecondsValue := ""
	if exercise.DefaultStartingSeconds != nil {
		defaultSecondsValue = strconv.Itoa(*exercise.DefaultStartingSeconds)
	}
	repMinValue := ""
	if exercise.RepMin != nil {
		repMinValue = strconv.Itoa(*exercise.RepMin)
	}
	repMaxValue := ""
	if exercise.RepMax != nil {
		repMaxValue = strconv.Itoa(*exercise.RepMax)
	}

	categoryOptions := []selectOption{
		{
			Value:    string(domain.CategoryFullBody),
			Label:    domain.CategoryFullBody.Label(),
			Selected: exercise.Category == domain.CategoryFullBody,
		},
		{
			Value:    string(domain.CategoryUpper),
			Label:    domain.CategoryUpper.Label(),
			Selected: exercise.Category == domain.CategoryUpper,
		},
		{
			Value:    string(domain.CategoryLower),
			Label:    domain.CategoryLower.Label(),
			Selected: exercise.Category == domain.CategoryLower,
		},
	}
	typeOptions := []selectOption{
		{
			Value:    string(domain.ExerciseTypeWeighted),
			Label:    "Weighted",
			Selected: exercise.ExerciseType == domain.ExerciseTypeWeighted,
		},
		{
			Value:    string(domain.ExerciseTypeBodyweight),
			Label:    "Bodyweight",
			Selected: exercise.ExerciseType == domain.ExerciseTypeBodyweight,
		},
		{
			Value:    string(domain.ExerciseTypeAssisted),
			Label:    "Assisted",
			Selected: exercise.ExerciseType == domain.ExerciseTypeAssisted,
		},
		{
			Value:    string(domain.ExerciseTypeTime),
			Label:    "Time-based",
			Selected: exercise.ExerciseType == domain.ExerciseTypeTime,
		},
	}

	data := exerciseEditTemplateData{
		BaseTemplateData: newBaseTemplateData(r),
		Header:           PageHeaderData{Title: fmt.Sprintf("Edit Exercise: %s", exercise.Name), Subtitle: ""},
		Flash:            BannerData{Variant: "error", Message: app.popFlashError(r.Context())},
		Exercise:         exercise,
		NameField: FieldData{ //nolint:exhaustruct // labelled text input; native-validation attrs unused here.
			Label:    "Name",
			Name:     "name",
			Type:     "text",
			Value:    exercise.Name,
			Required: true,
		},
		SecondsField: FieldData{ //nolint:exhaustruct // labelled number input; Max/Step/Pattern unused here.
			Label:    "Default Starting Seconds",
			Name:     "default_starting_seconds",
			Type:     "number",
			Value:    defaultSecondsValue,
			Required: exercise.IsTimed(),
			Hint:     "Number of seconds to hold on the first set for new users.",
			Min:      "1",
		},
		RepMinField: FieldData{ //nolint:exhaustruct // labelled number input; Hint/Step/Pattern unused here.
			Label:    "Min Reps",
			Name:     "rep_min",
			Type:     "number",
			Value:    repMinValue,
			Required: !exercise.IsTimed(),
			Min:      "1",
			Max:      "50",
		},
		RepMaxField: FieldData{ //nolint:exhaustruct // labelled number input; Hint/Step/Pattern unused here.
			Label:    "Max Reps",
			Name:     "rep_max",
			Type:     "number",
			Value:    repMaxValue,
			Required: !exercise.IsTimed(),
			Min:      "1",
			Max:      "50",
		},
		CategoryOptions:        categoryOptions,
		TypeOptions:            typeOptions,
		PrimaryMuscleOptions:   buildMuscleGroupOptions(muscleGroups, exercise.PrimaryMuscleGroups),
		SecondaryMuscleOptions: buildMuscleGroupOptions(muscleGroups, exercise.SecondaryMuscleGroups),
	}

	app.render(w, r, http.StatusOK, "admin-exercise-edit", data)
}
```

- [ ] **Step 4: Rewrite `adminExerciseUpdatePOST` and delete `parseRepWindow`**

In `cmd/web/handler-admin-exercises.go`, replace the whole `adminExerciseUpdatePOST` function (including its `//nolint:funlen` comment line) with:

```go
// adminExerciseUpdatePOST handles POST requests to update an exercise.
func (app *application) adminExerciseUpdatePOST(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		app.notFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, largeMaxFormSize)
	if err = r.ParseForm(); err != nil {
		app.serverError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	// Time-based exercises carry a starting-seconds value; every other type
	// carries a rep window. The handler reads the type-appropriate fields;
	// Exercise.Validate enforces that the populated fields are valid.
	exerciseType := domain.ExerciseType(r.PostForm.Get("exercise_type"))
	var defaultStartingSeconds, repMin, repMax *int
	if exerciseType == domain.ExerciseTypeTime {
		defaultStartingSeconds = optionalInt(r.PostForm.Get("default_starting_seconds"))
	} else {
		repMin = optionalInt(r.PostForm.Get("rep_min"))
		repMax = optionalInt(r.PostForm.Get("rep_max"))
	}

	exercise := domain.Exercise{
		ID:                     id,
		Name:                   r.PostForm.Get("name"),
		Category:               domain.Category(r.PostForm.Get("category")),
		ExerciseType:           exerciseType,
		DescriptionMarkdown:    r.PostForm.Get("description"),
		PrimaryMuscleGroups:    r.PostForm["primary_muscles"],
		SecondaryMuscleGroups:  r.PostForm["secondary_muscles"],
		DefaultStartingSeconds: defaultStartingSeconds,
		RepMin:                 repMin,
		RepMax:                 repMax,
	}

	editPath := fmt.Sprintf("/admin/exercises/%d", id)
	if err = app.service.UpdateExercise(r.Context(), exercise); err != nil {
		var ve domain.ValidationError
		if errors.As(err, &ve) {
			app.putFlashError(r.Context(), ve.Message)
			redirect(w, r, editPath)
			return
		}
		app.serverError(w, r, err)
		return
	}

	app.logger.LogAttrs(r.Context(), slog.LevelInfo, "updated exercise",
		slog.Int("id", id),
		slog.String("name", exercise.Name))

	redirect(w, r, "/admin/exercises")
}
```

Then delete the entire `parseRepWindow` function — it is no longer referenced (`optionalInt` does the parsing, `Exercise.Validate()` does the validation).

- [ ] **Step 5: Rewrite `admin-exercise-edit.gohtml`**

Replace the entire contents of `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml` with the following. The `<script {{ nonce }}>` block is carried over verbatim from the current file; the `data-back-button` link is unchanged.

```gohtml
{{- /* gotype: github.com/myrjola/petrapp/cmd/web.exerciseEditTemplateData */ -}}
{{ define "page" }}
    <main class="stack">
        {{ template "page-header" .Header }}
        {{ template "banner" .Flash }}

        <form method="post" action="/admin/exercises/{{ .Exercise.ID }}" class="card stack">
            {{ template "field" .NameField }}

            <div>
                <label for="category">Category:</label>
                <select id="category" name="category" required>
                    {{ range .CategoryOptions }}
                        <option value="{{ .Value }}"{{ if .Selected }} selected{{ end }}>{{ .Label }}</option>
                    {{ end }}
                </select>
            </div>

            <div>
                <label for="exercise_type">Exercise Type:</label>
                <select id="exercise_type" name="exercise_type" required>
                    {{ range .TypeOptions }}
                        <option value="{{ .Value }}"{{ if .Selected }} selected{{ end }}>{{ .Label }}</option>
                    {{ end }}
                </select>
            </div>

            <div id="default-starting-seconds-field"{{ if not .Exercise.IsTimed }} hidden{{ end }}>
                {{ template "field" .SecondsField }}
            </div>

            <div id="rep-window-field"{{ if .Exercise.IsTimed }} hidden{{ end }}>
                {{ template "field" .RepMinField }}
                {{ template "field" .RepMaxField }}
                <small>Target rep range per set (1–50).</small>
            </div>

            <script {{ nonce }}>
                (function () {
                    const typeSelect = document.getElementById('exercise_type');
                    const secondsField = document.getElementById('default-starting-seconds-field');
                    const secondsInput = document.getElementById('default_starting_seconds');
                    const repField = document.getElementById('rep-window-field');
                    const repMinInput = document.getElementById('rep_min');
                    const repMaxInput = document.getElementById('rep_max');

                    function update() {
                        const isTimed = typeSelect.value === 'time_based';
                        secondsField.hidden = !isTimed;
                        secondsInput.required = isTimed;
                        repField.hidden = isTimed;
                        repMinInput.required = !isTimed;
                        repMaxInput.required = !isTimed;
                    }

                    typeSelect.addEventListener('change', update);
                    update();
                })();
            </script>

            <div>
                <label for="primary_muscles">Primary Muscle Groups:</label>
                <select id="primary_muscles" name="primary_muscles" multiple required>
                    {{ range .PrimaryMuscleOptions }}
                        <option value="{{ .Name }}" {{ if .Selected }}selected{{ end }}>{{ .Name }}</option>
                    {{ end }}
                </select>
                <small>Hold Ctrl/Cmd to select multiple</small>
            </div>

            <div>
                <label for="secondary_muscles">Secondary Muscle Groups:</label>
                <select id="secondary_muscles" name="secondary_muscles" multiple>
                    {{ range .SecondaryMuscleOptions }}
                        <option value="{{ .Name }}" {{ if .Selected }}selected{{ end }}>{{ .Name }}</option>
                    {{ end }}
                </select>
                <small>Hold Ctrl/Cmd to select multiple</small>
            </div>

            <div>
                <label for="description">Description (Markdown):</label>
                <textarea id="description" name="description" rows="10">{{ .Exercise.DescriptionMarkdown }}</textarea>
            </div>

            <button type="submit">Update Exercise</button>
        </form>

        <a href="/admin/exercises" data-back-button>Back to Exercise List</a>
    </main>
{{ end }}
```

Notes on what changed and what did not:
- `<h1>` → `{{ template "page-header" .Header }}`; the ad-hoc `<p role="alert">` → `{{ template "banner" .Flash }}`.
- The `name`, `default_starting_seconds`, `rep_min`, `rep_max` `<input>`s are rendered by the `field` partial — the partial sets `id` equal to `Name`, so the toggle `<script>`'s `getElementById` targets (`default_starting_seconds`, `rep_min`, `rep_max`) and the wrapper-div ids (`default-starting-seconds-field`, `rep-window-field`) all still resolve.
- The `category`/`exercise_type` `<select>`s now `range` over handler-prepared `[]selectOption`, removing the inline `{{ if eq ... }}` literal comparisons. The `primary_muscles`/`secondary_muscles` `<select>`s and the `description` `<textarea>` stay as inline markup (the `field` partial covers `<input>` only).
- The wrapper-div `hidden` toggles use the `.Exercise.IsTimed` domain method instead of `{{ ne .Exercise.ExerciseType "time_based" }}`.
- `.card stack` on the `<form>` gives it a surface and vertical rhythm; `<main class="stack">` spaces the header, banner, form, and back-link.
- The shared `<small>` rep-range note is kept — it describes the rep_min/rep_max pair, not a single input, so it does not fit a single `field` Hint.

- [ ] **Step 6: Run the admin handler tests**

Run: `go test -v ./cmd/web -run Test_application_adminExercises`
Expected: PASS — all subtests. The four edit-page subtests stay green because: the `banner` error variant emits `role="alert"`; `Exercise.Validate()`'s messages contain `"Name is required"` and `"less than or equal to"`; the field `name` attributes and the form `action` are unchanged; `SubmitForm` resolves `"Name"`/`"Category"`/`"Type"`/`"Primary"`/`"Secondary"`/`"Description"` against label text that still contains those substrings, and `"rep_min"`/`"rep_max"` against the unchanged input `name` attributes.

- [ ] **Step 7: Run the full cmd/web test package**

Run: `go test ./cmd/web/...`
Expected: PASS — confirms no other handler test (e.g. `handler-exercise-info_test.go`) regressed on the admin DOM changes.

- [ ] **Step 8: Lint**

Run: `make lint-fix`
Expected: no errors. `adminExerciseUpdatePOST` is now well under the 100-line `funlen` limit (the six inline validation blocks collapsed into one `errors.As` block), so the removed `//nolint:funlen` is not needed. `adminExerciseEditGET` is under the limit because `buildMuscleGroupOptions` collapses the duplicated muscle-option loops. If `funlen` still flags `adminExerciseEditGET`, extract the `categoryOptions`/`typeOptions` construction into a small unexported helper rather than re-adding a suppression.

- [ ] **Step 9: Commit**

```bash
git add cmd/web/handler-admin-exercises.go ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml
git commit -m "Migrate admin-exercise-edit page to the frontend foundation"
```

---

## Task 5: Full verification

Runs the complete CI gate and a manual browser check before the branch is finished.

**Files:** none (verification only).

- [ ] **Step 1: Run the full CI gate**

Run: `make ci`
Expected: PASS — build, lint (0 issues), full test suite (`go test --race --shuffle=on ./...`), and `govulncheck`. If anything fails for a reason unrelated to this change, stop and report rather than working around it.

- [ ] **Step 2: Manually verify both pages in the browser**

Run the dev server (`make dev`), log in, promote yourself to admin (or use an admin account), and visit `/admin/exercises`:
- The header renders via `page-header`; the exercise table shows human-readable category labels ("Upper Body", not "upper") and comma-joined muscle lists.
- Submit the "Generate New Exercise" form with an empty name (bypass the `required` attribute via devtools or curl) — confirm you land back on `/admin/exercises` with a red error banner reading "Exercise name is required.", not a 500 page.
- Open an exercise's edit page: the header renders via `page-header`; the name/seconds/rep inputs render as `field` components inside a `.card`; switching the Exercise Type select to/from "Time-based" still toggles the seconds field and the rep-window field via the inline script.
- Submit the edit form with an empty name — confirm the red error banner reads "Name is required." and you stay on the edit page.

- [ ] **Step 3: Report status**

Report that all tasks are complete and `make ci` is green, then hand off to `superpowers:finishing-a-development-branch`.

---

## Self-Review

**Spec coverage:**
- Domain `Exercise.Validate()` with the six rules, first-failure, preserved messages → Task 1.
- `parseRepWindow` deleted, replaced by `optionalInt` (parse) + `Validate()` (rules) → Tasks 3 (helper added) and 4 (deletion).
- `Service.UpdateExercise` calls `Validate()`; `Service.GenerateExercise` guards empty name → Task 2.
- `adminExerciseUpdatePOST` / `adminExerciseGeneratePOST` `errors.As` → flash → redirect control flow → Tasks 4 and 3.
- `adminExercisesGET` / `adminExerciseEditGET` pop the flash into a `Flash BannerData` field → Tasks 3 and 4.
- Template data: `adminExerciseRow` (category label + joined muscles), `selectOption` (category/type options), `Header`/`Flash`/`NameField`/`SecondsField`/`RepMinField`/`RepMaxField` → Tasks 3 and 4; `ValidationError`/`DefaultStartingSecondsValue`/`RepMinValue`/`RepMaxValue` removed, `MuscleGroups` dropped from the list struct → Tasks 4 and 3.
- Templates adopt `page-header`, `banner`, `field`, `.card`, `.stack`; selects/textarea stay inline; toggle script + back-link verbatim → Tasks 3 and 5 (`admin-exercises.gohtml`), Task 4 step 5 (`admin-exercise-edit.gohtml`).
- Preserved DOM contracts (h1 text, table, form actions, input `name`s, `[role=alert]`, toggle-script ids) → asserted by the Task 3/4 handler tests; `SubmitForm` label-matching documented in Background.
- Deliberate behaviour change (generate empty-name 500 → banner) → Task 3 new subtest.
- Tests: domain `Test_Exercise_Validate` → Task 1; service `Test_UpdateExercise_RejectsInvalidExercise` + `Test_GenerateExercise_RejectsEmptyName` → Task 2; handler generate-empty-name subtest + existing subtests stay green → Tasks 3, 4; `make ci` → Task 5.
- No doc changes — `cmd/web/CLAUDE.md` already documents the flow as a go-forward convention; `ui/templates/CLAUDE.md` already lists the components. Noted in File Structure.

**Placeholder scan:** No TBD/TODO; every code step shows complete content; the verbatim toggle script is reproduced in full in Task 4 Step 5.

**Type consistency:** `Exercise.Validate() error` (Task 1) is called in `Service.UpdateExercise` (Task 2) and exercised through `adminExerciseUpdatePOST` (Task 4). `adminExerciseRow{ID, Name, CategoryLabel, PrimaryMuscles, SecondaryMuscles}`, `selectOption{Value, Label, Selected}`, `exerciseAdminTemplateData{Header, Flash, Exercises, NameField}`, and `exerciseEditTemplateData{Header, Flash, Exercise, NameField, SecondsField, RepMinField, RepMaxField, CategoryOptions, TypeOptions, PrimaryMuscleOptions, SecondaryMuscleOptions}` are defined once in Tasks 3–4 and consumed with those exact field names in the matching templates. `optionalInt(string) *int` and `buildMuscleGroupOptions([]string, []string) []MuscleGroupOption` are defined in Task 3 and `buildMuscleGroupOptions` is consumed in Task 4. `BannerData{Variant, Message}`, `PageHeaderData{Title, Subtitle}`, `FieldData{Label, Name, Type, Value, Required, Hint, Min, Max, Step, Pattern}` are the pre-existing `cmd/web/components.go` structs, used with their existing field names.

**Note on task independence:** Task 3 intentionally leaves `parseRepWindow` in place so `cmd/web` keeps compiling after Task 3's commit; Task 4 deletes it once `adminExerciseUpdatePOST` no longer calls it. Both tasks commit independently and leave the build green.
