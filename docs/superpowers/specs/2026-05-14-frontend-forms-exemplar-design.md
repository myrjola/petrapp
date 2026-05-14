# Frontend Forms Exemplar Design — admin-exercises

**Date:** 2026-05-14
**Status:** Approved — ready for implementation planning

## Goal

Migrate the admin-exercises form pages to the frontend foundation (merged at
commit `1fd9d9c`) as the **forms-side reference page**, the way
`workout.gohtml` is the display-side reference. The migration concretises the
"Forms & validation" section of
`docs/superpowers/specs/2026-05-14-frontend-foundation-design.md`: validation
rules move into the domain layer as a typed, testable method; the handler
detects them with `errors.As` and routes them through the flash → redirect →
`banner` flow; the templates adopt the `field`, `banner`, and `page-header`
partials plus the layout primitives.

### In scope

- `cmd/web/handler-admin-exercises.go` — all four handlers
  (`adminExercisesGET`, `adminExerciseEditGET`, `adminExerciseUpdatePOST`,
  `adminExerciseGeneratePOST`) and the `parseRepWindow` free function.
- `ui/templates/pages/admin-exercises/admin-exercises.gohtml`
- `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml`
- `internal/domain/exercise.go` — a new `Exercise.Validate()` method.
- `internal/service/exercises.go` and
  `internal/service/exercise_generation.go` — wire validation into the
  orchestration entry points.

### Out of scope

- Per-field server-side validation errors and submitted-value preservation
  (the foundation spec already rules these out — one banner channel).
- Any change to the stack-navigator wire protocol or the flash mechanism.
- The `field` partial gaining `<select>`/`<textarea>` support.
- Styleguide changes — `banner`, `field`, and `page-header` are already
  cataloged.
- The `data-back-button` link's mechanism — left verbatim.
- Visual/aesthetic polish — deferred to the frontend-design plugin.

## Constraints (unchanged)

- No-build frontend; templates load from the filesystem at runtime.
- Strict CSP — every `<style>`/`<script>` carries `{{ nonce }}`; no inline
  `style="..."` attributes.
- Comments end with a period; error types suffixed `Error`; exhaustive enum
  `switch`; no magic numbers (`mnd` linter).
- `make ci` green before finishing.

## Behaviour change (deliberate, single)

The "Generate New Exercise" form's empty-name case currently calls
`app.serverError(w, r, nil)` — a 500. It becomes a `domain.ValidationError`
surfaced as a flash banner on `/admin/exercises`, consistent with every other
validation failure. This is a real bug fix, not just a refactor. Everything
else is behaviour-preserving.

## Domain layer — `Exercise.Validate()`

A new pure method `func (e Exercise) Validate() error` on
`internal/domain/exercise.go`. It returns `domain.ValidationError{Message: ...}`
on the **first** failure (matching today's first-failure → single-banner
behaviour) and `nil` when the exercise is valid. Value receiver — read-only.

Rules, checked in this order (the order and the messages preserve today's
handler behaviour; the handler test asserts the substrings `"Name is
required"` and `"less than or equal to"`):

1. `Name == ""` → `"Name is required."`
2. `!Category.IsValid()` →
   `"Category must be one of full body, upper, or lower."`
3. `!ExerciseType.IsValid()` →
   `"Exercise type must be weighted, bodyweight, assisted, or time_based."`
4. `IsTimed()` and `DefaultStartingSeconds == nil || *DefaultStartingSeconds <= 0`
   → `"Default starting seconds must be a positive integer for time-based exercises."`
5. `len(PrimaryMuscleGroups) == 0` →
   `"At least one primary muscle group is required."`
6. `!IsTimed()`: `RepMin` and `RepMax` must be non-nil, each in `[1, 50]`, and
   `*RepMin <= *RepMax`. Out-of-range or missing →
   `"Min and max reps must be whole numbers between 1 and 50."`;
   `*RepMin > *RepMax` → `"Min reps must be less than or equal to max reps."`

`Validate()` checks only that the *populated* fields are valid and that the
fields *required for the exercise type* are present. It does not assert that a
timed exercise lacks a rep window (or vice versa) — the handler's struct
shaping (below) guarantees that, so a cross-check would be dead code.

The existing `parseRepWindow` free function — which both parsed strings and
authored validation messages — is **deleted**. Parsing moves to a small pure
helper in the handler (below); message authoring moves into `Validate()`.

## Service layer

- `Service.UpdateExercise` calls `ex.Validate()` before persisting:
  `if err := ex.Validate(); err != nil { return fmt.Errorf("validate exercise: %w", err) }`.
  This enforces the invariant for every caller, per
  `internal/service/CLAUDE.md` ("call the rule from a one-line service
  method"). The `fmt.Errorf` wrap satisfies `wrapcheck`; `errors.As` traverses
  it.
- `Service.GenerateExercise` gains a one-line guard at the top:
  `if name == "" { return domain.Exercise{}, domain.ValidationError{Message: "Exercise name is required."} }`.
  The service constructing a `ValidationError` directly is allowed — the
  foundation spec says "domain/service returns typed errors".

## Handler layer — `handler-admin-exercises.go`

### `adminExerciseUpdatePOST`

1. Parse the exercise ID from the path; `notFound` on failure.
2. `r.ParseForm()` with `largeMaxFormSize` (unchanged).
3. Extract raw form values; parse the optional integer fields with a new pure
   helper `optionalInt(raw string) *int` (returns `nil` for an empty or
   unparseable string — native HTML validation is the client-side guard, the
   domain method is the backstop).
4. Build the `domain.Exercise`. The handler reads the type-appropriate fields:
   for `ExerciseTypeTime` it parses `default_starting_seconds` and leaves the
   rep window `nil`; otherwise it parses `rep_min`/`rep_max` and leaves seconds
   `nil`. This mirrors today's behaviour exactly and is acceptable handler
   shaping (the *rule* — "timed exercises carry seconds, others carry a rep
   window" — lives in `Validate()`; the handler only decides which form fields
   to read).
5. `err := app.service.UpdateExercise(r.Context(), exercise)`. Then:
   ```go
   var ve domain.ValidationError
   if errors.As(err, &ve) {
       app.putFlashError(r.Context(), ve.Message)
       redirect(w, r, editPath)
       return
   }
   if err != nil {
       app.serverError(w, r, err)
       return
   }
   ```
6. Log and `redirect(w, r, "/admin/exercises")` on success.

The `//nolint:funlen` annotation comes off — the six inline validation blocks
collapse to one `errors.As` block. If the function is still over the 100-line
limit after the rewrite, that is a signal the rewrite is incomplete; do not
re-add the suppression without cause.

### `adminExerciseGeneratePOST`

Same `errors.As` pattern. On a `domain.ValidationError` from
`GenerateExercise`: `putFlashError` + `redirect(w, r, "/admin/exercises")`. On
any other error: `serverError`. On success: `redirect` to
`/admin/exercises/{id}` (unchanged).

### `adminExercisesGET` and `adminExerciseEditGET`

Both pop the flash into a `Flash BannerData` template field:
`Flash: BannerData{Variant: "error", Message: app.popFlashError(r.Context())}`.
The `banner` partial renders nothing when `Message` is empty, so the no-error
case is handled for free.

## Template data structures

### `exerciseAdminTemplateData`

```go
type adminExerciseRow struct {
    ID               int
    Name             string
    CategoryLabel    string // domain Category.Label()
    PrimaryMuscles   string // strings.Join(..., ", ")
    SecondaryMuscles string // strings.Join(..., ", ")
}

type exerciseAdminTemplateData struct {
    BaseTemplateData
    Header    PageHeaderData
    Flash     BannerData
    Exercises []adminExerciseRow
}
```

`adminExercisesGET` builds the rows, joining muscle-group slices with
`strings.Join(_, ", ")` and labelling the category with `Category.Label()`.
This removes the in-template `{{ range $index, $element := ... }}{{ if $index
}},{{ end }}` comma-join hack — the known template-logic offense on this page.
`MuscleGroups []string` is dropped from the struct (the list page never used
it; only the edit page builds muscle options).

### `exerciseEditTemplateData`

```go
type selectOption struct {
    Value    string
    Label    string
    Selected bool
}

type exerciseEditTemplateData struct {
    BaseTemplateData
    Header                      PageHeaderData
    Flash                       BannerData
    Exercise                    domain.Exercise
    CategoryOptions             []selectOption
    TypeOptions                 []selectOption
    PrimaryMuscleOptions        []MuscleGroupOption
    SecondaryMuscleOptions      []MuscleGroupOption
    DefaultStartingSecondsValue string
    RepMinValue                 string
    RepMaxValue                 string
}
```

The `ValidationError string` field is **replaced** by `Flash BannerData`.
`CategoryOptions`/`TypeOptions` are prepared in `adminExerciseEditGET` — mirroring
the existing `MuscleGroupOption` pattern — so the template no longer carries
inline `{{ if eq .Exercise.Category "full_body" }}selected{{ end }}` literal
comparisons. `Header.Title` is `"Edit Exercise: " + exercise.Name`, built in
the handler.

## Templates

### Both pages

- `<main class="stack">`.
- `<h1>...</h1>` → `{{ template "page-header" .Header }}`.
- `{{ template "banner" .Flash }}` near the top of `<main>`, after the header.

### `admin-exercises.gohtml`

- The exercise table stays a `<table>` — a data table is neither a `card` nor a
  `field` candidate. Its rows now read `adminExerciseRow` fields
  (`.CategoryLabel`, `.PrimaryMuscles`, `.SecondaryMuscles`) instead of
  iterating slices.
- The "Generate New Exercise" `<section>` becomes a `.card`. Its name input →
  `{{ template "field" .NameField }}` with
  `FieldData{Label: "Exercise Name", Name: "name", Type: "text", Required: true,
  Hint: "e.g., Bench Press, Deadlift, Squat"}` (the old `placeholder` text
  becomes the accessible hint). `NameField` is a handler-prepared field on
  `exerciseAdminTemplateData` — added alongside the fields above.

### `admin-exercise-edit.gohtml`

- The `<form>` sits inside a `.card` with `.stack` for vertical rhythm.
- `name`, `default_starting_seconds`, `rep_min`, `rep_max` inputs →
  `{{ template "field" ... }}`. The seconds and rep inputs stay inside their
  existing `<div id="default-starting-seconds-field">` /
  `<div id="rep-window-field">` wrappers so the toggle `<script>` keeps
  working — the script targets those wrapper ids and the input ids, all of
  which the `field` partial preserves (the input `id` equals its `Name`).
- The wrapper `hidden` attribute uses the `.Exercise.IsTimed` domain method:
  `<div id="default-starting-seconds-field"{{ if not .Exercise.IsTimed }} hidden{{ end }}>`
  and `<div id="rep-window-field"{{ if .Exercise.IsTimed }} hidden{{ end }}>`.
- Per-field `FieldData`:
  - name: `{Label: "Name", Name: "name", Type: "text", Value: exercise.Name, Required: true}`
  - seconds: `{Label: "Default Starting Seconds", Name: "default_starting_seconds",
    Type: "number", Value: .DefaultStartingSecondsValue, Min: "1",
    Required: exercise.IsTimed(), Hint: "Number of seconds to hold on the first set for new users."}`
  - rep_min / rep_max: `{Type: "number", Min: "1", Max: "50",
    Required: !exercise.IsTimed(), ...}` with the existing `<small>` rep-range
    note kept as a shared sibling inside the wrapper (it describes the pair, not
    a single input).
- `category`/`exercise_type`/`primary_muscles`/`secondary_muscles` `<select>`s
  stay as inline markup (the `field` partial covers `<input>` only, per
  `ui/templates/CLAUDE.md`). The `category`/`exercise_type` selects now `range`
  over `.CategoryOptions`/`.TypeOptions`.
- The `description` `<textarea>` stays as inline markup.
- The toggle `<script {{ nonce }}>` and the
  `<a href="/admin/exercises" data-back-button>` link are carried over
  verbatim.

## Preserved DOM contracts

The existing `handler-admin-exercises_test.go` and any `playwright_test.go`
selectors depend on these — all preserved:

- `h1` text: `"Exercise Administration"` and `"Edit Exercise: {name}"`
  (`page-header` emits exactly one `<h1>` with `Title` as its text).
- `table` present on the list page; `form[action='/admin/exercises/generate']`;
  `form[action='/admin/exercises/{id}']`.
- All input/select/textarea `name` attributes: `name`, `category`,
  `exercise_type`, `default_starting_seconds`, `rep_min`, `rep_max`,
  `primary_muscles`, `secondary_muscles`, `description`.
- `[role=alert]` for validation errors — the `banner` partial's `error` variant
  emits `role="alert"`.
- The toggle-script target ids: `default-starting-seconds-field`,
  `default_starting_seconds`, `rep-window-field`, `rep_min`, `rep_max`,
  `exercise_type`.
- `td:contains(...)`, `tr:contains(...) td a:contains('Edit')` row selectors —
  the table keeps `<td>` cells per column and the `Edit` link.

Before editing the templates, the implementer reads `internal/e2etest`'s
`SubmitForm` to confirm how it resolves the test's `formData` keys (`"Name"`,
`"Category"`, `"Type"`, `"Primary"`, `"Secondary"`, `"Description"`) to form
controls, so label/`name` changes don't silently break form submission.

## Testing (TDD throughout)

- **`internal/domain/exercise_test.go`** — `TestExercise_Validate`, a
  table-driven test: a valid weighted exercise and a valid timed exercise both
  return `nil`; each of the six rules has a failing case asserting the exact
  message. Match the file's existing struct-construction convention
  (`exhaustruct`).
- **`internal/service/exercises_test.go`** — `UpdateExercise` with an invalid
  exercise returns an error that `errors.As` matches to
  `domain.ValidationError` (proves the rule is enforced at the service boundary
  and survives the `fmt.Errorf` wrap). A companion case in the
  exercise-generation tests covers `GenerateExercise("")` →
  `domain.ValidationError`.
- **`cmd/web/handler-admin-exercises_test.go`** — every existing subtest stays
  green (the `[role=alert]` assertions are satisfied by the `banner` partial).
  One new subtest: submitting the generate form with an empty name lands back
  on `/admin/exercises` with a `[role=alert]` banner, **not** a 500.
- `make ci` green before the branch is finished.

## Deliverables

1. `internal/domain/exercise.go` — `Exercise.Validate()`; `exercise_test.go` —
   `TestExercise_Validate`.
2. `internal/service/exercises.go` — `UpdateExercise` calls `Validate()`;
   `internal/service/exercise_generation.go` — `GenerateExercise` empty-name
   guard; service tests for both.
3. `cmd/web/handler-admin-exercises.go` — `errors.As` control flow in both POST
   handlers, flash-pop in both GET handlers, `optionalInt` helper,
   `parseRepWindow` deleted, `funlen` suppression removed, new template structs
   (`adminExerciseRow`, `selectOption`, `Header`/`Flash`/`NameField` fields).
4. `ui/templates/pages/admin-exercises/admin-exercises.gohtml` and
   `ui/templates/pages/admin-exercise-edit/admin-exercise-edit.gohtml` —
   migrated to `page-header`, `banner`, `field`, `.card`, and `.stack`.
5. `cmd/web/handler-admin-exercises_test.go` — new generate-form validation
   subtest; existing subtests confirmed green.
