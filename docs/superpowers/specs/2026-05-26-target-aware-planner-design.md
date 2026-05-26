# Target-Aware Weekly Planner

**Date:** 2026-05-26
**Status:** Design Review

## Problem

User 24's week of 2026-05-25 (3-day full-body, Tue/Thu/Sat 90 min) showed
two simultaneous failure modes on the home muscle-balance bar:

| Muscle group | Planned weighted load | Target | Status     |
|--------------|----------------------:|-------:|------------|
| Shoulders    | 17.5                  |     10 | **+75 %**  |
| Triceps      | 14.5                  |      8 | **+81 %**  |
| Upper Back   | 16.5                  |     10 | **+65 %**  |
| Chest        |  8.0                  |     10 | **−20 %**  |
| Quads        |  9.0                  |     10 | **−10 %**  |

This is a recurrence of the pattern flagged as out-of-scope in
[`docs/2026-05-12-demote-shoulders-primary-on-presses.md`](../../2026-05-12-demote-shoulders-primary-on-presses.md):
re-classifying individual exercises (primary→secondary) addresses the
symptom for one MG at a time but doesn't fix the underlying planner
blindness. The bar's weighted load also accumulates from secondary
credits that the planner never accounts for at pick time, so muscles
with dense secondary footprints (Shoulders, Triceps, Upper Back) reliably
balloon while muscles with sparse secondary footprints (Chest, Quads) sit
at exactly their primary-set count.

## Root cause

The planner in `internal/domain/planner.go` runs three phases:

1. `determineCategory` for each scheduled day (FullBody/Upper/Lower).
2. `allocateMuscleGroups` — assigns each targeted muscle group to up to
   2 workout days as a "priority list", balancing day load by *count of
   MGs assigned*, not by predicted weighted load.
3. `selectExercisesForDayWithPeriodization` — Phase A iterates through
   the priority MGs trying to satisfy each with a non-conflicting
   exercise; Phase B fills remaining slots with the lowest-id
   non-conflicting exercise the loop can find.

Five compounding causes turn this into the observed imbalance:

1. **Allocation balances day-slots, not weighted load.** A MG is "covered"
   once it has 2 days, regardless of how many primary or secondary sets
   it ends up accumulating.
2. **An exercise picked for one priority credits its other primaries
   free.** Picking Face Pull for the Shoulders priority on Thu also adds
   Upper Back primary that day. Picking Bench for Chest also gives
   Triceps +4 primary.
3. **Phase B blindly fills slots.** On a day where every priority MG is
   already satisfied (e.g. Sat after Pulldown + Push-Up + Squat covered
   all 6 priorities), the lowest-id non-overlapping exercise wins. That's
   how Shoulder Press lands as the 3rd shoulders-primary pick of the
   week.
4. **Secondary load is unbounded.** Each set credits 0.5× to every
   secondary MG. Compound upper-body exercises hit Shoulders/Upper
   Back/Triceps as secondaries on 4–7 different picks; the planner never
   sees this contribution.
5. **Asymmetric secondary footprint.** Chest is secondary on effectively
   no other exercise in the pool; Quads only on Deadlift, Calf Raise,
   and Ab Wheel. So Chest and Quads only accumulate via primary picks,
   while Shoulders/Upper Back/Triceps grow from every direction.

The hand-traced math for the affected week is in the problem table above;
it matches the muscle-balance bar to the decimal, confirming the diagnosis.

## Goal

Replace the priority-MG allocation + two-phase selection with a single
**target-aware greedy selection** that scores each candidate by how much
it pulls the running weighted-load tally toward (or away from) the
configured targets. Result: Shoulders/Triceps/Upper Back stop being
free-fill victims; Chest/Quads get picked when they're underloaded.

## Scope

In scope:

- Rewrite `Plan` and `selectExercisesForDayWithPeriodization` in
  `internal/domain/planner.go`.
- Remove `allocateMuscleGroups`, `scoreExerciseForPriority`, and the
  Phase A/B distinction inside selection.
- Extend `PlanDay`'s signature to receive the current week's weighted
  load so ad-hoc picks share the same target-awareness.
- Update tests in `internal/domain/planner_internal_test.go`,
  `internal/domain/planner_plan_day_internal_test.go`,
  `internal/domain/week_plan_test.go`.
- Update the single `PlanDay` caller in
  `internal/service/sessions.go` to compute and pass `weekLoad`.

Out of scope:

- Recalibrating seed targets in `internal/sqlite/fixtures.sql`.
  Decision: ship the planner change first, observe how loads
  redistribute against existing targets, then recalibrate in a
  follow-up. The visualization and target schema stay in weighted-load
  units.
- Changing the visualization in `cmd/web/handler-home.go`.
- Re-classifying any individual exercise's muscle-group fields.
- New per-MG handling for the 8 untargeted MGs (Forearms, Traps, Calves,
  Abs, Lower Back, Hip Flexors, Adductors, Obliques). They contribute 0
  to score; they continue to accumulate incidental load via secondary
  credits on other picks.

---

## Design

### Scoring formula

State carried across a week's picks:

- `load[mg] float64` — running weighted-load tally per muscle group.
  Starts at 0; after each pick, increment by `n_sets × 1.0` for primary
  MGs and `n_sets × 0.5` for secondary MGs.
- `target[mg] int` — from `MuscleGroupTarget.WeeklySetTarget`. MGs not
  in the target table are skipped entirely during scoring.

`n_sets` is supplied by the existing `deriveSchemeForExercise(ex,
periodization, isDeload)`, which already accounts for periodization
(strength uses `repMin`, hypertrophy uses `repMax`) and deload halving.

For each candidate exercise, define:

```
gap_before        = Σ_targeted-mg (target[mg] − load[mg])²
predicted_load[mg] = load[mg] + n_sets × contribution(ex, mg)
                     where contribution = 1.0 (primary), 0.5 (secondary), 0 (none)
gap_after         = Σ_targeted-mg (target[mg] − predicted_load[mg])²
score             = gap_before − gap_after          // positive = pulls closer
```

Properties:

- An exercise that pushes an on-target MG further over → distance grows
  from 0² to overshoot² → score is negative → de-prioritized.
- An exercise hitting multiple under-target MGs → score is large positive
  → preferred.
- An isolation chest pick when chest is far below target → high score;
  the same pick when chest is on target → score ≈ 0.

### Pick loop

For each scheduled day in the week, in Mon→Sun order:

1. Filter the exercise pool to those compatible with the day's category
   (`isCategoryCompatible`) — unchanged from today.
2. Track `selectedPrimaryMGs` for the session (avoids two primary-Chest
   picks on the same day).
3. For each of the `n` slots on that day:
   - For every eligible candidate (not in `weekUsedExerciseIDs`, no
     primary MG already in `selectedPrimaryMGs`), compute its score
     against the *current* `load` tally.
   - Pick the highest-scoring candidate. Tiebreak by lowest exercise ID
     for determinism (matches current behavior).
   - If no eligible candidates remain, stop early — the session ends with
     fewer than `n` exercises (existing graceful degradation).
   - Update `load` and `selectedPrimaryMGs` with the pick's
     contributions; mark the exercise ID in `weekUsedExerciseIDs`.

The score is permitted to be negative; the loop still picks the
least-negative candidate. This is the "all targets already met"
fallback.

### What stays unchanged

- `determineCategory` (FullBody/Upper/Lower from adjacency rules).
- Periodization derivation: `firstSessionPeriodizationType`,
  `nextPeriodizationType`, deload-week detection.
- Per-session exercise-count via `exercisesPerSession(prefs, weekday,
  pt, isDeload)`.
- The week-used dedup constraint and the within-session
  primary-MG-overlap constraint.
- `BuildPlannedSets`, `DeriveScheme`, `buildPlannedExerciseSlot`.

### What is removed

- `allocateMuscleGroups` and its `mgEntry` helper struct.
- `scoreExerciseForPriority`.
- `findBestExerciseInPool` (replaced by an inline scoring loop, since
  the scoring function is small and tied to the new state).
- The `priorityMuscleGroups` parameter on
  `selectExercisesForDayWithPeriodization` and the Phase A/Phase B
  split inside it.
- `selectExercisesForDay` (the convenience wrapper that hard-codes
  PeriodizationStrength + empty priorities — only used by tests; rework
  those tests to call the new signature directly).

### New / changed signatures

```go
// Plan stays public; its internal shape simplifies.
func (wp *Planner) Plan(startingDate time.Time) (WeekPlan, error)

// PlanDay gains weekLoad so ad-hoc picks share target-awareness.
func (wp *Planner) PlanDay(
    date time.Time,
    weekUsedExerciseIDs map[int]bool,
    weekLoad map[string]float64,
) (Session, error)

// selectExercisesForDayWithPeriodization loses priorityMuscleGroups,
// gains the running load tally and a targets lookup; mutates load.
func (wp *Planner) selectExercisesForDayWithPeriodization(
    category Category,
    n int,
    pt PeriodizationType,
    isDeload bool,
    weekUsedExercises map[int]bool,
    load map[string]float64,
) []ExerciseSlot

// New pure helper used by the loop and by tests.
func scoreCandidate(
    ex Exercise,
    pt PeriodizationType,
    isDeload bool,
    load map[string]float64,
    targets map[string]int,
) float64
```

`weekLoad` for `PlanDay` is built by the caller in
`internal/service/sessions.go` from the persisted week's existing
sessions via the existing `aggregateMuscleGroupLoad` machinery (a small
exported wrapper or a slight broadening of `WeeklyMuscleGroupVolume`).

### Edge cases

1. **Empty targets table** — every score is 0; the tiebreaker (lowest
   exercise ID) picks. Preserves deterministic behavior for pure-domain
   tests that don't seed targets.
2. **All targeted MGs already at or over target** — every candidate
   scores ≤ 0; pick the least-negative. No crash, no skipped slot.
3. **Deload weeks** — `deriveSchemeForExercise` already returns halved
   sets; predicted load uses those halved counts. Targets are *not*
   halved; deload is intentionally under-target on the bar.
4. **Untargeted MGs** — contribute 0 to score. They still accumulate
   incidental weighted load via secondary credits on targeted-MG
   exercises.
5. **Pool exhaustion mid-session** — same graceful degradation as today:
   the session ends with fewer than `n` slots, no error.
6. **Single workout day in prefs** — produces one session with the best
   available picks against the targets; no special-casing needed.

### Test plan

1. **Replace, don't update, the obsolete tests.** Tests for
   `allocateMuscleGroups`, `findBestExerciseInPool`,
   `scoreExerciseForPriority`, and the Phase-A/Phase-B invariants are
   deleted along with the functions they cover.
2. **New unit tests for `scoreCandidate`**: positive when the candidate
   pulls under-target MGs up, negative when it pushes an on-target MG
   over, zero when no targeted MG is touched, deterministic.
3. **Updated `Plan` integration tests**: where a test asserted a specific
   pick, restate it as a property — e.g. "given target Chest=10 and one
   chest-primary exercise in the pool, that exercise is in the plan."
4. **Prod-scenario regression test**: seed the 38 prod exercises with
   their current muscle-group classifications, the 9 prod targets, and
   user 24's Tue/Thu/Sat 90-min preferences. Assert qualitative invariants:
   - every targeted MG falls within `[0.7 × target, 1.4 × target]`
     weighted load,
   - no MG exceeds `1.5 × target`,
   - Chest planned ≥ 8 weighted (prevents the original undershoot).
5. **Determinism test**: same `Plan` input twice → byte-equal output;
   guards against map iteration leaking into scoring.
6. **`PlanDay` parity**: a test where `weekLoad` is pre-loaded with a
   near-over-target Shoulders value and the ad-hoc pick avoids
   shoulder-primary exercises.
7. **Edge cases**: empty `Targets`, all-MGs-over synthetic pool,
   single-workout-day prefs, deload week.

### Migration

No schema change. No data backfill. The new planner takes effect on
the next plan generation per user. Existing persisted sessions are
untouched.

### Rollout

A single PR replacing the planner internals. CI runs the full test
suite. On merge, staging then prod auto-deploy. Smoke check on user 24's
next scheduled plan: muscle-balance bar should show Shoulders/Triceps/
Upper Back back inside their target band and Chest/Quads at or above
target.

## Open questions

None blocking. Target recalibration is intentionally a follow-up: after
one or two weeks of the new planner, if any MG still trends outside the
target band, adjust the seed targets in `internal/sqlite/fixtures.sql`
(and a one-shot prod script under `docs/`) rather than re-tuning the
algorithm.
