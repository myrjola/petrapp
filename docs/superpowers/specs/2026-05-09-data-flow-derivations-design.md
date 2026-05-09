# Data-flow derivations: keep business rules out of handlers

## Background

The 2026-05-09 per-exercise rep-scheme rollout (PR #89) shipped with a UI bug:
`formatTarget` in `cmd/web/handler-exerciseset.go` displayed the hardcoded
string `"6-10"` for any hypertrophy session, while the planner had moved to
per-exercise rep windows driven by `Exercise.RepMin` / `Exercise.RepMax`. A
heavy spinal compound with `RepMin=3, RepMax=6` showed `"6-10"` to the user
even though the planner had targeted 6 reps off the per-exercise window.

Tests passed because the handler test fixture and the planner test fixture
both happened to live with the same hardcoded assumption. Test coverage
reasoned about each layer in isolation; no test pinned the contract between
them.

The shape of the bug — a display rule duplicated in a handler and silently
out of sync with the business rule it was meant to mirror — is the class of
bug this spec is aimed at.

## Goal

Prevent display logic from drifting from business logic by codifying a single
rule, then applying it as a targeted fix to the one live offender.

Out of scope: the broader question of layer sizes (`service.go` is 1,319
lines), aggregate-vs-domain duplication, or any rework of the four-stage data
flow (SQL → repository aggregate → domain model → template data). The flow
itself is sound; this spec only tightens what each layer is allowed to
compute.

## The rule

**Any value that depends on multiple domain attributes, or that encodes a
business rule, is a method on the domain type that owns the rule.** Handlers
may format primitives (`%d`, `%.1fkg`, `time.Format`) and shape data into
per-page template structs. Handlers may not branch on multiple domain
attributes to compute a value.

Test for whether something is a derivation that belongs on the domain: if
changing the rule would force edits in two or more files outside
`internal/workout/`, it is a domain method.

`Exercise.IsTimed()` is the canonical existing example. The rule generalizes
that pattern.

### What this leaves alone

- Templates may branch on enum values (`{{ if eq ExerciseType "weighted" }}`)
  to pick between distinct UI fragments. That is a rendering choice, not a
  derivation.
- Pure formatting (`fmt.Sprintf("%dkg", weight)`) stays in handlers.
- Per-handler template data structs (`exerciseSetTemplateData`,
  `setDisplay`) stay handler-owned.
- The four-stage data flow itself: SQL rows → repository aggregates → domain
  models → template data. Each transition keeps its current home.

## The targeted fix

The hypertrophy rep-window display goes away entirely. RIR (reps-in-reserve)
plus a static target carries the auto-regulation message — there is no UX
need to surface a window at the set-display layer. That collapses the
offending branch instead of replacing it with a domain method.

### Code change

`cmd/web/handler-exerciseset.go:17-31` — `formatTarget`:

```go
// Before
func formatTarget(exercise workout.Exercise, session workout.Session, target int) string {
    if exercise.IsTimed() {
        return fmt.Sprintf("%ds", target)
    }
    // Hypertrophy display preserves the legacy 6-10 rep range UX. TargetValue
    // is the single integer the planner emits (8); progression and storage use
    // that, while the user sees the range.
    if session.PeriodizationType == workout.PeriodizationHypertrophy {
        return "6-10"
    }
    return strconv.Itoa(target)
}

// After
func formatTarget(exercise workout.Exercise, target int) string {
    if exercise.IsTimed() {
        return fmt.Sprintf("%ds", target)
    }
    return strconv.Itoa(target)
}
```

`cmd/web/handler-exerciseset.go:55` — `prepareSetsDisplay` drops its `session`
parameter. The single call site at `handler-exerciseset.go:168` updates to
pass only `exerciseSet.Exercise` and `exerciseSet.Sets`.

The legacy comment about hypertrophy and "the user sees the range" deletes
along with the branch.

### Why no new domain method

An earlier draft proposed `Exercise.RepWindow()` and
`Exercise.TargetDisplayFor(target, periodization)`. With the hypertrophy
branch removed, neither would gain a second caller — the planner already
reads `RepMin` / `RepMax` directly via `DeriveScheme`, and the remaining
two-line `formatTarget` is mechanical formatting on a single predicate
(`IsTimed`) that already exists as a domain method.

Adding the methods anyway would be abstraction without payoff and would
violate the "rule of three" precedent the codebase follows for component
extraction.

### What stays

- `Session.PeriodizationType`, `Exercise.RepMin`, `Exercise.RepMax`, and the
  `weekplanner` / progression consumption of those fields.
- Other format chains in handlers — e.g. `processEntryData` in
  `cmd/web/handler-exercise-info.go:108` switching on `ExerciseType` to
  format chart entries differently per type. Those branches encode distinct
  per-type display shapes (with-weight vs reps vs seconds), not a derivation
  that mirrors a business rule.

## Documentation

Two CLAUDE.md amendments codify the rule for future work.

### `internal/workout/CLAUDE.md` — append under "Common Patterns and Anti-Patterns"

> ### Display derivations belong on domain types
>
> Any value that depends on multiple domain attributes, or that encodes a
> business rule, lives as a method on the domain type that owns the rule
> (`Exercise.IsTimed()` is the canonical example). Handlers may format
> primitives (`%d`, `%.1fkg`, `time.Format`) and shape data into per-page
> template structs, but they may not branch on multiple domain fields to
> compute a value.
>
> **Test:** if changing the rule would force edits in two or more files
> outside `internal/workout/`, it is a domain method. The 2026-05-09
> hypertrophy-window incident traces back to `formatTarget` in `cmd/web/`
> reimplementing a rule that already lived (correctly) in the planner.

### `cmd/web/CLAUDE.md` — extend "Data Transformation Patterns" with one bullet

> - **Don't recompute domain rules.** Handlers may format primitives and
>   shape data, but any value that depends on multiple domain fields must
>   come from a method on the domain type. If you find yourself writing
>   `if exercise.X && session.Y { ... }` in a handler, move it to
>   `internal/workout/`.

## Change set

Three patches:

1. `cmd/web/handler-exerciseset.go` — collapse `formatTarget`, drop the
   `session` parameter from `formatTarget` and `prepareSetsDisplay`, update
   the single `prepareSetsDisplay` call site.
2. `internal/workout/CLAUDE.md` — append the "Display derivations" subsection.
3. `cmd/web/CLAUDE.md` — append the "Don't recompute domain rules" bullet.

No new tests are required. Existing handler tests for the set-display path
already exercise the `IsTimed` and integer-target branches; the deleted
hypertrophy branch was producing the wrong string and any test that pinned
`"6-10"` should update to expect the integer target.

## Risks and trade-offs

- **UX change is not free.** Removing the rep-range display from hypertrophy
  is a real product decision, not just a refactor. Users currently see
  `"6-10"` and will see e.g. `"8"` after the change. RIR is the chosen
  signaling mechanism for headroom; surfacing it in the per-set UI is
  separate work and out of scope here.
- **The rule is enforced by convention, not types.** A future handler can
  still write `if exercise.X && session.Y { ... }` and slip past review.
  This spec accepts that trade-off; introducing view-struct enforcement was
  considered and rejected as too heavy for the observed bug rate.
- **The `service.go` size and `Set.TargetValue` lossiness from the
  brainstorming session remain unaddressed.** Intentional — these are
  separate concerns and were excluded by the chosen scope ("pattern +
  targeted fixes").
