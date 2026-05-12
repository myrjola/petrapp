# Deload Periodization — Design

**Date:** 2026-05-12
**Status:** Draft for review
**Type:** Feature design

## Motivation

PetrApp already runs weekly Daily Undulating Periodization (DUP) — sessions
alternate between `PeriodizationStrength` (low reps, more sets, full rest)
and `PeriodizationHypertrophy` (high reps, fewer sets, shorter rest). Per-set
autoregulation via `SignalTooLight` / `SignalOnTarget` / `SignalTooHeavy`
moves weight up or down between sets.

Missing from this picture is **planned recovery**. With no deload week,
cumulative fatigue accrues unchecked: the well-supported finding across
hypertrophy and strength literature is that periodic volume + intensity
reduction restores performance, reduces injury risk, and improves long-run
progression versus continuous overload.

This design introduces a fixed-cadence deload week — a planned "lighter
hypertrophy" week — into the existing weekly planner. It is additive: no
existing behaviour changes when the feature is disabled, and the design
leaves clean seams for future autoregulated triggers and volume-landmark
progression.

## Academic anchors

- **Bompa, *Periodization* (6th ed.)** — classical block periodization with
  fixed accumulation:deload ratios (3:1 / 4:1).
- **Helms et al., *The Muscle and Strength Pyramid* (2018)** — practical
  deload prescriptions: ~50% volume reduction, ~10% intensity reduction,
  keep movement patterns familiar.
- **Israetel et al. (Renaissance Periodization)** — volume-landmark
  framework (MEV → MAV → MRV → deload at ~50% MEV). The within-block ramp
  is *out of scope* here; the deload-week prescription that closes each
  block is what we adopt.
- **Schoenfeld (2016)** — longer rest periods improve hypertrophy outcomes
  in trained lifters; relevant because we do **not** shorten rest on
  deload weeks.
- **Pelzer et al. (2017)**, **Zourdos et al. (2016)** — programmed deloads
  improve adaptation versus continuous training even when fatigue markers
  do not yet indicate overreaching.

## Approach summary

A **fixed-cadence mesocycle** with both volume and intensity reduction on
the deload week, defaulting to 5-week blocks (4 accumulation + 1 deload)
and user-configurable to 4 / 5 / 6 / 7 weeks. Mesocycle position is
**derived statelessly** from a stored anchor date plus the cadence, in
the same shape as the existing strength/hypertrophy alternation.

The deload week is implemented as a **forced-hypertrophy week with cut
volume, cut starting weight, and no set-level autoregulation**.
Periodization on a deload session is invariantly `Hypertrophy`; the
planner overrides the normal alternation for that week and resumes
alternation in week 1 of the next block. Signal buttons (too-heavy /
on-target / too-light) are **hidden** in the UI on deload sessions, all
sets target the same pre-stamped weight, and deload session rows are
**excluded from every future starting-weight lookup**, so a deload
week never anchors subsequent progression.

## Mesocycle model

### Stateless derivation

Mirroring the existing pattern in `planner.go:90`
(`firstSessionPeriodizationType` deriving from `weeksSinceEpoch`), week
position within the block is a pure function of the date, the user's
mesocycle anchor, and the user's cadence:

```
weeksSinceAnchor(date, anchorMonday) → int  // floor((date - anchor)/7 days)
WeekInBlock(date, anchor, length)    → int  // weeksSinceAnchor % length, in 0..length-1
IsDeloadWeek(date, anchor, length)   → bool // WeekInBlock == length - 1
```

Properties:

- **No counter to drift.** Replanning a week or skipping a week never
  desyncs the schedule.
- **Replannable.** The Monday weekly-planner call computes deload-ness
  fresh each run.
- **Resettable.** A "restart cycle" action just sets the anchor to the
  next Monday — no history rewrite needed.

### Configuration

Three new fields on `Preferences`:

| Field | Type | Default | Meaning |
|---|---|---|---|
| `DeloadEnabled` | `bool` | `false` | Feature toggle. When false, planner behaves exactly as today. |
| `MesocycleLength` | `int` | `5` | Total weeks in a block (4–7). The last week is the deload. |
| `MesocycleAnchor` | `time.Time` | Monday of feature opt-in | A Monday defining week 0 of the user's first block. |

When `DeloadEnabled` flips from false to true, the preferences-update
service path snaps `MesocycleAnchor` to the upcoming Monday (i.e.
"today if Monday, else next Monday"). This guarantees the user's
first cycle starts with an accumulation week, never with an immediate
deload. The anchor write happens in the same transaction as the
toggle so the two cannot diverge.

When the user changes `MesocycleLength` mid-block, the anchor is
**not** rewritten. The week-index calculation simply applies the new
modulus from the next planning run. (Edge cases — e.g. shortening
cadence mid-block resulting in "today is suddenly a deload" — are
acceptable; the worst case is one extra easy week.)

The `MesocycleAnchor` is exposed on the preferences page as a
"Restart cycle from next Monday" button that re-snaps the anchor.

## Domain shape

### Session

Add one field to `Session`:

```go
type Session struct {
    // ... existing fields
    IsDeload bool
}
```

**Invariant:** when `IsDeload == true`, `PeriodizationType` must equal
`PeriodizationHypertrophy`. Enforced at the planner; the aggregate
itself does not validate this on every method call (the planner is the
sole writer of these two fields together).

### Scheme derivation

`DeriveScheme` gains an `isDeload bool` parameter:

```go
func DeriveScheme(repMin, repMax int, p PeriodizationType, isDeload bool) Scheme
```

When `isDeload == true`, `DeriveScheme` defensively forces the
hypertrophy mapping regardless of the incoming `p` value:
`TargetReps = repMax`, hypertrophy rest. The planner also passes
`p = PeriodizationHypertrophy` (so the two sources of truth agree),
but `DeriveScheme` does not rely on the caller. This concentrates
the invariant in one place. Set count halves; rest stays the same.

```
Normal sets → Deload sets
  4 (low)   →  2
  3 (mid)   →  2  (ceil(1.5))
  3 (high)  →  2
```

General rule: `deloadSets = max(1, int(math.Ceil(float64(normalSets) * 0.5)))`.
Rest seconds unchanged from the hypertrophy/normal mapping — deload is
recovery, not metabolic conditioning.

`RestSecondsFor` (`progression_scheme.go:70`) gains the same `isDeload`
threading. For deload, rest equals hypertrophy rest (since periodization
is forced to hypertrophy).

### Progression

`Progression.Config` gains an `IsDeload bool` field. When set:

1. **Starting weight derivation is the caller's responsibility** (as
   today), but the contract becomes: on deload, the caller passes
   90% of the prior hypertrophy-zone working weight for this exercise,
   snapped via the existing `snapWeight` rule. If no hypertrophy
   history exists yet, fall back to 80% of any recent working weight
   for the exercise (rough adjustment from low-rep to high-rep load).
2. **`CurrentSet()` returns the starting weight for every set.**
   No set-to-set adjustment. Signals are not collected on deload
   (see UX section), so `RecordCompletion` will receive a `nil`
   signal; `adjustedWeight` is skipped entirely on the deload code
   path. This removes the need for any "what does
   SignalTooLight mean on deload?" branching — the question never
   arises because the signal isn't there.

Concretely, `CurrentSet()` on a deload `Progression` ignores
`p.completed` and always returns `{WeightKg: config.StartingWeight,
TargetReps: ...}`. The reps target still comes from `DeriveScheme`
(hypertrophy, halved sets).

The set's `Signal` column ends up NULL — already permitted by the
`exercise_sets.signal` schema constraint.

This concentrates the deload knowledge in two well-bounded places:
`DeriveScheme` (volume) and `Progression.CurrentSet()` (intensity —
constant weight, no autoregulation).

### Starting-weight lookup — deload sessions are excluded everywhere

Deload sets must not contribute to future starting-weight derivations.
This is enforced at the repository: `GetLatestStartingWeightBefore`
(`internal/repository/sessions.go:493`) gains an `AND
workout_sessions.is_deload = 0` filter in its query — applied
unconditionally, regardless of whether the *upcoming* session is a
deload or a normal session. Rationale: deload weights are
intentionally suppressed and shouldn't anchor any progression — the
user lifted lighter on purpose, not because that's their current
working weight.

The same filter applies to the **new** "most recent hypertrophy-zone
working weight" query used to seed a deload week's starting weight:

> Find the most recent `workout_sessions` row for this user where
> `periodization_type = 'hypertrophy'` AND `is_deload = 0`, walk its
> `exercise_sets` for this exercise, and return the maximum
> `weight_kg` recorded across those sets. (Maximum, not median: it
> reflects the actual working weight the user got to, ignoring any
> mid-session decreases triggered by `SignalTooHeavy`.)

Fallback policy when no hypertrophy history exists:

1. Most recent non-deload working weight for the exercise, × 0.80.
2. If no history at all: the exercise's default starting weight × 0.80.

To keep this clean, both queries — the existing
`GetLatestStartingWeightBefore` and the new hypertrophy-specific
lookup — go through one repository method that takes the desired
periodization filter, with `is_deload = 0` baked in.

## Planner behaviour

Single point of change in the planner: when generating a session for
a date that falls on a deload week (per `IsDeloadWeek`), force
`PeriodizationType = PeriodizationHypertrophy` and `IsDeload = true`,
then pass both through `BuildPlannedSets` / `DeriveScheme`. Exercise
selection is **unchanged** — same movements, same muscle-group
allocation, same priority logic. Only the per-exercise prescription
(set count, starting weight) differs.

The intra-week strength/hypertrophy alternation
(`nextPeriodizationType` in `planner.go:336`) is skipped on deload
weeks: every session in a deload week is hypertrophy. The alternation
resumes from a fresh start in week 1 of the next block (i.e., week 1
defaults to whichever periodization
`firstSessionPeriodizationType(week_1_monday)` picks — the existing
function still works because it's date-derived).

When `DeloadEnabled = false`, the planner runs exactly as today —
the deload computation is gated on the preference.

## Schema

Additive migration (no premigration needed):

```sql
ALTER TABLE workout_preferences ADD COLUMN deload_enabled INTEGER NOT NULL DEFAULT 0
    CHECK (deload_enabled IN (0, 1));
ALTER TABLE workout_preferences ADD COLUMN mesocycle_length INTEGER NOT NULL DEFAULT 5
    CHECK (mesocycle_length BETWEEN 4 AND 7);
ALTER TABLE workout_preferences ADD COLUMN mesocycle_anchor TEXT
    CHECK (mesocycle_anchor IS NULL
        OR STRFTIME('%Y-%m-%d', mesocycle_anchor) = mesocycle_anchor);

ALTER TABLE workout_sessions ADD COLUMN is_deload INTEGER NOT NULL DEFAULT 0
    CHECK (is_deload IN (0, 1));
```

`mesocycle_anchor` is nullable: NULL means "not yet anchored," which
the planner treats identically to `deload_enabled = 0`.

The existing declarative migrator (`internal/sqlite/migrate.go`)
handles `ALTER TABLE ADD COLUMN` by table rebuild, which is safe here
because all new columns have defaults.

## UX surfaces

Three small touchpoints:

1. **Preferences page** — new "Deload" section:
   - Toggle: *Enable planned deloads*.
   - Dropdown: *Block length* — 4 / 5 / 6 / 7 weeks (labelled "3+1
     / 4+1 / 5+1 / 6+1"). Disabled when toggle is off.
   - Button: *Restart cycle from next Monday* — re-anchors. Disabled
     when toggle is off.
2. **Plan / week view** — small chip on the week header showing
   `Week 3 of 5` or `Week 5 of 5 · Deload`. Hidden when feature is off.
3. **Session page on a deload day** — three changes:
   - Banner at the top of the session: *"Deload week — lighter loads,
     same weight every set. Just hit your reps and rest. These sets
     don't influence future progression."*
   - **Signal buttons hidden** on every set's completion form. The
     completion control on a deload set is a single "Done" action
     that records the actual reps and timestamp, without prompting
     for too-heavy / on-target / too-light. The `Signal` column ends
     up NULL in the database (already permitted by schema).
   - Per-set weight display shows the deload weight consistently
     across all sets in the exercise — there is no "next set might
     be heavier/lighter" hint because the planner pre-stamps the
     same weight on every set.

Rationale for hiding signals (vs. capturing them as a softer
disabled-but-visible state):

- **Conceptual coherence with the user instruction:** deload sets are
  declared not to influence progression. Hiding the signals enforces
  this in the UI, not just in the back-end query filters.
- **Removes cognitive load on a recovery week** — the point of the
  deload is to stop grading effort.
- **Trivially reversible** if a future autoregulation feature wants
  signal data on deload weeks: the column is already nullable, the
  hide is just a template branch on `Session.IsDeload`.

The handler / form posting from a deload set therefore omits the
`signal` field; the service layer's `RecordSet` path is reached with
a `nil` signal (the underlying `Session.RecordSet` aggregate method
signature is updated to accept `*Signal` rather than `Signal` — a
small, safe refactor since storage is already nullable).

## Testing

- **Domain unit tests** (`internal/domain/`):
  - `WeekInBlock` / `IsDeloadWeek` — table-driven across cadences
    (4, 5, 6, 7) and anchor dates including DST boundaries.
  - `DeriveScheme` with `isDeload=true` — every rep band, both
    nominal periodization types (the function defensively forces
    hypertrophy + halved sets regardless of the incoming `p`).
  - `Progression.CurrentSet()` with `IsDeload=true` — verify the
    starting weight is returned for every set, and `RecordCompletion`
    with `nil` signal does not panic and does not move the weight.
  - `Session.RecordSet` with `*Signal == nil` — verify the set is
    recorded with `Signal == nil` in storage.
- **Planner integration test**:
  - Plan a week that lands on a deload week — every session has
    `IsDeload=true`, `PeriodizationType=Hypertrophy`, halved sets,
    and starting weight at 90% of prior hypertrophy weight.
  - Plan a week that lands on week N-2 — feature off / normal output.
- **Repository tests**:
  - Round-trip the new preferences and sessions columns; existing
    tests should pass with default zero-valued columns.
  - `GetLatestStartingWeightBefore` excludes rows where `is_deload =
    1`. Seed two prior sessions for the same exercise — one normal,
    one deload (lighter weight) — and verify the lookup returns the
    normal weight even when the deload session is more recent.
- **Service-layer starting-weight test** for the hypertrophy-zone
  lookup with both fallback branches.
- **Handler test** for the preferences form: persisting the new
  fields, validating cadence bounds.
- **Template / handler test** for the session page on a deload day:
  signal buttons are absent; the "Done" form submits without a
  `signal` field; the banner is rendered.

## Out of scope (intentional follow-ups)

- **Volume landmark ramping (MEV → MAV → MRV) within the
  accumulation phase.** This would require tracking weekly set
  volume per muscle group and progressing across the block. The
  fixed-cadence model and the `IsDeload` field don't preclude it;
  it would slot into the planner's `BuildPlannedSets` path.
- **Autoregulated trigger / override** — advancing or postponing
  the deload based on `DifficultyRating` trend, `SignalTooHeavy`
  rate, or weight stalls. The orthogonal `IsDeload` field and
  per-week stateless derivation make this an additive change.
- **Peaking / realization weeks** at the end of a strength-biased
  block. Not aligned with the app's "consistent training" framing.
- **Per-exercise deload exemption** (e.g. "don't cut volume on
  rotator-cuff work"). Possible later via an `Exercise.DeloadPolicy`
  enum; not part of this change.

## Open questions / known asymmetries

- **Cycle anchor on signup vs. opt-in.** Anchored to the Monday of
  feature opt-in (not signup), so existing users opting in always
  get a fresh week-1 next Monday. A retroactive anchor would risk
  putting an existing user into an immediate deload, which would
  feel arbitrary.
- **Mid-block cadence change.** Documented above as "applies from
  next planning run, no anchor rewrite." Worst case: one bonus easy
  week. Acceptable.
- **What if the planner runs on a non-Monday?** The existing
  `Plan` already requires `startingDate.Weekday() == Monday` —
  the deload calculation reuses this contract.
