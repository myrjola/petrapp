# Design: Scientifically-Grounded Muscle-Volume Model

**Date:** 2026-06-07
**Status:** Approved (design); implementation plans to follow per phase.

## Goal

Make the planner's volume model faithful to the resistance-training
dose-response evidence while keeping a one-sentence user story:

> Each muscle has a weekly sweet-spot range of hard sets; we ramp toward it
> over the block, hit every muscle at least twice a week, then deload.

The change is motivated by a review of the program-design literature
(weekly volume is the primary hypertrophy driver with a graded dose-response
to ~20+ sets/muscle/week and no clear plateau; frequency ≥2×/week is the one
near-universal prescription; synergist/indirect work counts as a fractional
set; deltoid heads must be trained separately). The current implementation
gets the foundation right — a volume-aware planner with primary (1.0) /
secondary (0.5) set weighting that already matches the literature's fractional
set count — but has three scientific defects this design corrects:

1. **Volume capped too low.** `MuscleGroupTarget` is documented as a minimum
   but the squared-distance scoring penalizes exceeding it, so the planner
   fills each muscle to ~8–10 sets and then avoids it. The dose-response keeps
   climbing past that.
2. **Incomplete coverage.** Only 9 of 17 muscle groups have targets; calves,
   abs, and (critically) side/rear delts get only incidental volume. A single
   lumped "Shoulders" target is satisfied by pressing (front delt), leaving
   side and rear delts — the posterior work the literature flags — invisible.
3. **No frequency floor.** Nothing rewards spreading a muscle's sets across
   ≥2 days; a muscle's whole week can land on one day.

## Scope

**In scope (one combined design, four implementation phases):**

- **(A)** Muscle-group taxonomy cleanup.
- **(B)** Weekly volume targets as a floor→ceiling range.
- **(C)** Per-muscle frequency floor (≥2×/week).
- **(D)** Weekly volume ramp across the mesocycle.

**Out of scope (confirmed):**

- **Deload trigger changes.** The scheduled mesocycle deload stays exactly as
  it is (`mesocycle.go` `IsDeloadWeek`). The volume ramp simply peaks the week
  before the deload week. Reactive/fatigue-triggered deloads may be a future
  spec; the literature calls the evidence weak either way.
- Splitting Quads (vasti vs rectus femoris) or other intra-muscle regions —
  too granular for the "easy to understand" goal; better handled later via
  exercise-selection guidance than taxonomy.
- Per-user customizable targets — defaults are seeded globally as today;
  individualization is a possible future spec.

## Definitions

- **Targeted group:** has a row in `muscle_group_weekly_targets`; the planner
  actively chases its weekly goal in scoring.
- **Tag-only group:** still mapped on exercises (so it receives primary/
  secondary credit and appears in the volume dashboard) but has no target row,
  so the planner never chases it. New concept introduced by this design.

---

## (A) Taxonomy changes

### Domain (`internal/petra/domain/muscle_group.go`)

- Add constants `MuscleGroupSideDelts = "Side Delts"` and
  `MuscleGroupRearDelts = "Rear Delts"`. Keep `MuscleGroupShoulders`, now
  meaning the front/general delt fed by pressing.
- Remove `MuscleGroupHipFlexors`.
- `RegionFor`: classify **Side Delts → `RegionUpperPush`** and
  **Rear Delts → `RegionUpperPull`** (rear delt is functionally a puller).

### Targeted vs tag-only after this change

- **Targeted:** Chest, Shoulders (front), Side Delts, Rear Delts, Triceps,
  Biceps, Upper Back, Lats, Quads, Hamstrings, Glutes, Calves, Abs,
  Lower Back (maintenance level).
- **Tag-only:** Traps, Forearms, Adductors, Obliques (mapped for credit,
  never chased).
- **Dropped entirely:** Hip Flexors.

### Fixtures remapping (`internal/petra/repository/fixtures.sql`)

- Lateral raises → **Side Delts** primary.
- Rear-delt flyes / face pulls / reverse pec deck → **Rear Delts** primary.
- Rows / pulldowns → add **Rear Delts** secondary.
- Overhead / incline / flat presses → keep **Shoulders** (front) mapping
  as-is.
- Remove all **Hip Flexors** rows from `exercise_muscle_groups`.

---

## (B) Range targets

### Data model

`MuscleGroupTarget` (domain) becomes:

```go
type MuscleGroupTarget struct {
    MuscleGroupName string
    MinSets         int // ≈ MEV (minimum effective volume)
    MaxSets         int // ≈ MRV (maximum recoverable volume)
}
```

(`WeeklySetTarget` is renamed `MinSets`; `MaxSets` is added.)

### Proposed seed defaults

RP-landmark-inspired starting defaults (weekly hard sets). These are starting
points, not per-user tuned:

| Muscle group      | MinSets (MEV) | MaxSets (MRV) | Notes                              |
|-------------------|---------------|---------------|------------------------------------|
| Chest             | 10            | 20            |                                    |
| Upper Back        | 10            | 20            | horizontal pull                    |
| Lats              | 10            | 20            | vertical pull                      |
| Quads             | 10            | 20            |                                    |
| Hamstrings        | 8             | 18            |                                    |
| Glutes            | 8             | 16            |                                    |
| Side Delts        | 8             | 18            | needs direct work; tolerates high  |
| Rear Delts        | 8             | 18            | needs direct work; tolerates high  |
| Triceps           | 8             | 16            |                                    |
| Biceps            | 8             | 16            |                                    |
| Calves            | 8             | 16            | currently untargeted (0)           |
| Abs               | 6             | 16            |                                    |
| Shoulders (front) | 6             | 12            | heavy indirect from all pressing   |
| Lower Back        | 4             | 8             | maintenance; heavy indirect        |

### Scoring (`scoreCandidate`)

Replace the symmetric squared-distance-to-a-point metric with a **piecewise**
reward measured against *this week's* goal (see D for the goal):

- **below `goal(mg, week)`** — reward per added set (pulls the muscle toward
  the goal); steepest segment.
- **at/above goal but below `MaxSets`** — smaller positive reward
  (dose-response keeps paying, with diminishing returns).
- **above `MaxSets`** — penalty (protects against runaway volume and spreads
  sets to muscles with remaining headroom).

The metric remains a per-set delta summed over **targeted** muscle groups so
the planner's existing "pick the exercise that most improves balance" loop is
unchanged in shape — only the per-muscle reward curve changes. Tag-only groups
contribute no reward (no target row).

---

## (C) Frequency floor

Soft, not hard. The planner already builds the week day-by-day with a running
per-muscle load map (`Plan`). Add a running **days-touched-per-muscle** map.
In `scoreCandidate`, add a bonus when a targeted muscle is **both** below 2
days touched this week **and** below its weekly goal. Effect: volume naturally
spreads across ≥2 days without a hard constraint that could starve a slot. No
new data model; the bonus is tuned so it nudges distribution without
overriding the volume-balance signal.

---

## (D) Weekly volume ramp

### The fork (resolved): decouple intensity from volume

Session length (user minutes) caps total weekly sets, so moving the scoring
goal up alone cannot deliver more volume — something must supply more sets.

**Chosen approach: decouple the two axes.**

- **Periodization** (`PeriodizationStrength` / `PeriodizationHypertrophy`)
  governs **reps, rest, and load emphasis** — as today, minus set count.
- **Mesocycle week** governs **set count per exercise** (the volume ramp):
  set count rises as the block progresses (e.g. +1 set per exercise across the
  block, capped), then drops on the deload week.

This matches how RP-style mesocycles actually add volume and cleanly separates
"intensity style" from "how hard this week is." It requires refactoring
`DeriveScheme` / `BuildPlannedSets` (`progression_scheme.go`,
`planning_sets.go`) so set count is a function of the mesocycle week index
rather than the periodization type.

(Rejected alternative — ramp by adding whole exercises with fixed set count:
less refactor, but session time bounds slot count hard, making the ramp narrow
and lumpy since one exercise ≈ 3–4 sets at once.)

### Per-week goal

A new helper in `mesocycle.go`:

```go
// MesocycleWeekIndex returns the 0-based training-week index within the
// current mesocycle for date, given the deload anchor and mesocycle length.
func MesocycleWeekIndex(date, anchor time.Time, length int) int
```

The planner derives `goal(mg, week) = lerp(MinSets, MaxSets, progress)` where
`progress` ramps from 0 in week 1 to 1 in the last **training** week (the
deload week is excluded from the ramp). The deload week targets well below
`MinSets` via the existing deload set reduction.

---

## Storage & migration

- `muscle_group_weekly_targets`: rename `weekly_sets_target` → `min_sets`,
  add `max_sets`. One-shot migration committed under
  `docs/2026-06-07-muscle-volume-targets.sql` per the repo convention (also
  picked up by the deploy migration path).
- `muscle_groups`: insert `Side Delts`, `Rear Delts`; remove `Hip Flexors`
  and cascade-remove its `exercise_muscle_groups` rows.
- Re-seed delt mappings and the `min/max` target rows for all targeted groups
  (table above) in `fixtures.sql` and the migration.

## Testing strategy

- **`scoreCandidate`:** below-goal reward > above-goal reward > over-`MaxSets`
  penalty; an over-`MaxSets` pick loses to a fresh-muscle pick. Tag-only groups
  contribute nothing.
- **`MesocycleWeekIndex` + `goal()`:** lerp across a full block including the
  deload week (ramp excludes deload, peaks the prior week).
- **Frequency:** in a representative 3-day full-body week, each major targeted
  muscle's volume lands on ≥2 days.
- **Set-count-as-function-of-week:** set count rises across the block and drops
  on deload; periodization no longer drives set count.
- **Volume aggregation:** unaffected by the `MinSets` rename; Hip Flexors
  removal orphans no data; Side/Rear Delts appear in `WeeklyMuscleGroupVolume`.

## Phasing (implementation plans, one per phase)

1. **(A) Taxonomy** — domain constants, `RegionFor`, fixtures remapping,
   migration for `muscle_groups`. Self-contained; volume model unchanged.
2. **(B) Range targets** — `MuscleGroupTarget` shape, target migration,
   piecewise `scoreCandidate` (using a static goal = `MinSets` initially).
3. **(D) Volume ramp** — `MesocycleWeekIndex`, decoupled set count, ramping
   `goal()`. Depends on B.
4. **(C) Frequency floor** — days-touched tracking + scoring bonus. Depends on
   B (and reads naturally after the ramp distributes more volume).

This ordering lets each phase ship and be verified independently while sharing
the data-model groundwork laid in A and B.

### Plan status

- **Phase A (Taxonomy):** implemented and shipped (its execution plan is
  deleted per the plan lifecycle — see git history).
- **Phases B, D, C:** plans intentionally deferred. Write them only after the
  preceding phase is implemented and merged, so each is grounded in the
  then-current code.
