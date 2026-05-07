# Multi-Axis Progression

## Motivation

Once `01-exercise-attributes.md` ships, the planner knows how many reps to
*prescribe*. This document covers how to *progress* — i.e., what to change
when the user feeds back `SignalTooLight` after hitting target.

Today the answer is fixed: add weight. From
`internal/exerciseprogression/progression.go:118-132`:

```
SignalTooLight  → weight += increment
SignalOnTarget  → hold
SignalTooHeavy  → weight -= max(increment, 10% of weight)
```

That is correct for the bench press and the deadlift. It is wrong for almost
everything else:

- **Calf raise**: jumping from 60 kg → 62.5 kg at 3×15 will fail. The right
  next step is 3×17 at the same load. Reps then load.
- **Ab wheel rollout**: there is no load to add. The right next step is
  *kneeling-partial → kneeling-full → standing-partial → standing-full*.
  Variant then ROM.
- **Back extension**: under high spinal load, more weight is the dangerous
  axis. The right next step is to slow the eccentric (3-1-1 → 4-0-1) before
  adding load. Tempo then reps then load.
- **Pistol squat**: a beginner can't load a pistol; what advances them is
  band-assisted → unassisted-partial-ROM → unassisted-full-ROM. Variant then
  ROM then load.

A single-axis system either advances them dangerously (load on back
extensions) or fails to advance them at all (rollouts that they can't even do
from their knees yet keep showing the same prescription forever).

## What we're adding

A *priority list* per exercise that names the progression axes in the order
the engine should try them. It lives on `exercises` as a single text column:

```sql
progression_axes TEXT NOT NULL DEFAULT 'load,reps'
    -- comma-separated subset of: load, reps, rom, tempo, variant
```

Examples from the seed audit:

| Exercise            | `progression_axes`         |
|---------------------|----------------------------|
| Conventional deadlift | `load`                   |
| Bench press         | `load`                     |
| Standing calf raise | `reps,load,tempo`          |
| Seated calf raise   | `reps,load,tempo`          |
| Back extension      | `tempo,reps,load`          |
| Romanian deadlift   | `load,reps`                |
| Ab wheel rollout    | `variant,rom,reps`         |
| Pistol squat        | `variant,rom,reps,load`    |
| Nordic curl         | `rom,variant,reps`         |
| Pallof press        | `variant,load`             |
| Lateral raise       | `reps,load`                |

The progression engine asks: "is the first axis capped?" If yes, advance the
next axis instead. *Capped* depends on the axis:

- **load** is capped when the user override would jump to a non-loadable
  weight (e.g. dumbbell pairs jumping 10 kg → 12.5 kg).
- **reps** is capped at the top of `RepsMax` from `DeriveRepScheme`.
- **rom** is capped at 100 % (full ROM).
- **tempo** is capped at 4 s eccentric (a programmable max).
- **variant** is capped when there is no harder variant in the chain.

## Key design decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Encoding | One comma-separated text column on `exercises`, parsed at read time | A relational join (axes table) buys nothing; the list is short and read-only at runtime |
| New columns on `exercise_sets` | `rom_pct INTEGER DEFAULT 100`, `tempo_eccentric_s INTEGER DEFAULT 0`, `variant_id INTEGER NULL` | Logging is what makes "did the user cap this axis?" answerable |
| Variants | Sibling table `exercise_variants(exercise_id, name, difficulty_rank)` | Variant chains belong to an exercise; a sibling table keeps `exercises` clean |
| Signal stays | Continue with the existing 3-bucket `Signal` enum | RIR sliders are friction; the bucket is enough to drive the priority list |
| Algorithm shape | Pure function: `NextPrescription(ex, history) Prescription` | No DB, no clock, fully unit-testable — same shape as `DeriveRepScheme` |
| De-load | If `RepsMin` missed for 2 consecutive sessions, step back one variant or –10 % load | Only de-load axis we ship; full mesocycle de-loads are out of scope |
| No DSL | Inline `switch` over the parsed axis list | The Liftosaur DSL is overkill for a solo Go project; this fits in ~150 lines |

## High-level shape

```go
// internal/exerciseprogression/progression.go (extended)

type Axis string

const (
    AxisLoad    Axis = "load"
    AxisReps    Axis = "reps"
    AxisROM     Axis = "rom"
    AxisTempo   Axis = "tempo"
    AxisVariant Axis = "variant"
)

type Prescription struct {
    WeightKg       float64
    Reps           int
    ROMPct         int
    TempoEccS      int
    VariantID      int64
}

// NextPrescription replaces adjustedWeight as the engine entry point.
func NextPrescription(ex Exercise, scheme RepScheme, history []SetResult) Prescription
```

Pseudocode:

```
last := history[len-1]
switch last.Signal:
case OnTarget   → hold all axes, return last as-is
case TooHeavy   → de-load the *current* axis (back off load, or back off ROM, or step
                  back a variant); rest unchanged
case TooLight   → for each axis in ex.progression_axes:
                      if axis is not capped, advance it and return
                  → all axes capped: hold (the user has graduated this exercise)
```

The existing `Signal`-driven code path becomes one branch (the
`progression_axes == "load"` case) of this function.

## Concrete progression examples

The contract this delivers for the seed audit:

- **Calf raise** (`reps,load,tempo`): hits 3×15 → next session 3×18; hits 3×20
  → +5 kg, drop reps to 12; eventually cap reps at 25 → switch to slowing the
  eccentric.
- **Deadlift** (`load`): hits 3×5 → +2.5 kg next session. Reps stay at 5. The
  SBD strength profile is preserved.
- **Back extension** (`tempo,reps,load`): hits 3×12 with 1-1-1 → next session
  3×12 at 3-1-1; then 3×15 at 3-1-1; then add 5 kg, reset to 3×12 with 1-1-1.
- **Ab wheel rollout** (`variant,rom,reps`): three sessions of 3×8 kneeling-
  partial → advance variant to kneeling-full, restart at 3×6.
- **Nordic curl** (`rom,variant,reps`): hits 3×5 at 60 % ROM → next session
  3×5 at 75 % ROM; eventually 100 % ROM → advance variant to weighted Nordic.

## Out of scope

- **Per-set rationale tracking.** Captured in `04-prescription-rationale.md`.
- **Mesocycle-level de-loads** (RP-style 4-week accumulation + 1 de-load week).
  This doc only covers within-progression regression on consecutive misses.
- **RIR/RPE picker.** Defer; see `01-exercise-attributes.md`.
- **Equipment profiles for variants.** Variants are seeded with their own
  equipment requirements; the planner respects the existing `exercise_type`
  flag without learning about gym profiles yet.
- **Within-session adjustments** (Liftosaur's `update` blocks). Out of scope —
  current `recordSetCompletionWithWeight` flow is enough.

## Acceptance

- A property test asserts: progression never *skips* a variant level upward;
  on `TooHeavy` after two failures it backs off exactly one step on the active
  axis, never silently advances another.
- A 4-week synthetic mesocycle test for each example above produces the
  prescriptions listed under "Concrete progression examples".

## Brainstorming starter prompt

> I want to brainstorm `specs/03-multi-axis-progression.md`. Three things:
> (1) The five axes — is `load,reps,rom,tempo,variant` the complete set, or
> am I missing one (e.g., set count, rest interval)? (2) The variant model —
> seeding `exercise_variants` for ab wheel, pistol squat, Nordic curl, and
> pull-up. Walk me through the chain you'd seed for each, including the
> `prerequisites` for advancing (e.g., "min 3 sessions at the prior variant
> hitting `RepsMax`"). (3) The de-load policy on `TooHeavy` — should we step
> back the *active* axis only, or always step back load first regardless of
> the priority list, since load is the most reversible failure mode? Codebase
> context: `internal/exerciseprogression/progression.go:118-132` for the
> existing single-axis logic, `internal/sqlite/schema.sql:121-133` for
> `exercise_sets`. Keep the design inline — no DSL, no rules engine.
