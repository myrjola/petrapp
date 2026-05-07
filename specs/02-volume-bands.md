# Per-Muscle Volume Bands (MEV / MAV / MRV)

## Motivation

The week planner currently allocates muscle groups to days using a single
`weekly_sets_target` integer per muscle on `muscle_group_weekly_targets`
(see `internal/weekplanner/weekplanner.go:235`). This conflates three
quantities that the training-science literature treats as distinct:

- **MEV** — *Minimum Effective Volume*. Below this, no growth.
- **MAV** — *Maximum Adaptive Volume*. The sweet spot.
- **MRV** — *Maximum Recoverable Volume*. Above this, fatigue outpaces
  adaptation.

Renaissance Periodization, Schoenfeld's meta-analyses, and Stronger By Science
converge on these landmarks as the most defensible heuristic for setting weekly
sets per muscle. They also vary substantially by muscle: soleus tolerates
~22 sets/wk MRV, spinal erectors ~12. A single integer can't represent that.

The user-visible payoff is two things:

1. The planner stops over- or under-dosing muscle groups regardless of what's
   already on the calendar. Today, if Tuesday and Friday both target lats with
   pull-ups + rows, the planner happily piles ~24 sets onto lats with no signal
   that this is past MRV.
2. A weekly volume bar per muscle (green inside MAV, yellow approaching MRV,
   red over) becomes a free product feature that competitors (RP Hypertrophy)
   sell as their primary differentiator. The data is already there once we
   widen the schema.

## What we're adding

A 3-column widening of the existing target table, plus an awareness pass in
the week planner:

```sql
-- before
weekly_sets_target INTEGER NOT NULL

-- after
weekly_mev_sets INTEGER NOT NULL,
weekly_mav_sets INTEGER NOT NULL,
weekly_mrv_sets INTEGER NOT NULL
```

Defensible defaults to seed with (RP/Schoenfeld-aligned):

| Muscle group       | MEV | MAV | MRV |
|--------------------|-----|-----|-----|
| Quadriceps         | 8   | 14  | 20  |
| Hamstrings         | 6   | 12  | 16  |
| Glutes             | 4   | 10  | 16  |
| Calves (combined)  | 8   | 14  | 22  |
| Pectorals          | 8   | 14  | 22  |
| Lats               | 8   | 14  | 22  |
| Biceps             | 6   | 12  | 20  |
| Triceps            | 6   | 12  | 18  |
| Lateral deltoid    | 8   | 16  | 26  |
| Rear deltoid       | 6   | 14  | 22  |
| Spinal erectors    | 4   | 8   | 12  |
| Abs                | 6   | 14  | 25  |

The week planner's `allocateMuscleGroups` (line 235) consults MEV as the
floor (must hit) and MRV as the ceiling (must not exceed) instead of greedily
matching a single target.

## Key design decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Schema shape | Widen the existing table; don't add a new one | One row per (user_id, muscle_group) is still the right grain |
| Per-user vs. global | Keep per-user; seed defaults at user-create time | Bands are personalised over time as users progress; defaults come from the catalogue |
| Migration | Edit `schema.sql` and add a one-shot `docs/YYYY-MM-DD-volume-bands.sql` to backfill existing rows | Matches the project's declarative migration pattern (see CLAUDE.md) |
| Set-counting | Reuse the existing per-set rows in `exercise_sets`; secondary-muscle sets count fractionally (0.5×) | Follows Schoenfeld's "fractional set" heuristic without needing a new contribution column today |
| Planner contract | MEV is a floor, MRV is a hard ceiling, MAV is a target — the existing greedy allocator is replaced with band-aware selection | Makes the bands actually shape decisions, not just decorate them |
| UI surfacing | Per-muscle bar on the week view, color-coded green/yellow/red | The whole point — invisible bands aren't worth the migration |

## High-level shape

A single new query method on the workout repository:

```go
// WeeklyVolumeByMuscle returns effective sets logged in the trailing 7 days,
// keyed by muscle group, including fractional credit for secondary muscles.
WeeklyVolumeByMuscle(userID int64, asOf time.Time) map[MuscleGroupID]float64
```

Plumbs into `weekplanner` with a single decision rule per slot:

```
for each candidate exercise for this slot:
    projected = current_volume + sets_this_session
    if projected > muscle.MRV → reject
    score adds bonus if current_volume < muscle.MEV (must-hit)
    score subtracts penalty as projected approaches muscle.MRV
pick highest-scoring candidate
```

That's the whole policy. No optimization solver, no rules engine.

## Out of scope

- **Adaptive bands** that learn from user response. Defer — start with seeded
  defaults; revisit when ≥1k users have logged ≥1 mesocycle each.
- **Per-head muscle granularity** (front vs. side vs. rear delts). The seed
  table above already separates lateral and rear delts, which is the most
  important split. Sub-pec heads, sub-tricep heads etc. are too fine-grained
  for the current exercise catalogue.
- **Soft caps and override workflow.** MRV is treated as a hard rejection.
  If users complain, revisit by adding a "force include" affordance later.

## Acceptance

- Generated week for a synthetic user with 7 days/week of preferences keeps
  every muscle inside `[MEV, MRV]` for ≥80 % of muscles, and never breaches MRV.
- Week view renders a per-muscle bar for every tracked muscle with the correct
  color band.

## Brainstorming starter prompt

> I want to brainstorm `specs/02-volume-bands.md`. Two things specifically:
> (1) The seed defaults — are MEV/MAV/MRV right for the muscle groups we
> currently track in `internal/sqlite/fixtures.sql`? Pull the catalogue and
> propose any additions or splits (e.g., should we split calves into
> gastrocnemius vs. soleus to make the soleus 22-set MRV actually addressable?).
> (2) The "fractional sets for secondary muscles" rule — is a flat 0.5×
> credit good enough, or do we need a per-exercise contribution weight on
> `exercise_muscle_groups` to model e.g. romanian deadlifts contributing
> ~0.7× to glutes? Reason in the codebase context: existing schema in
> `internal/sqlite/schema.sql:135-153`, planner in
> `internal/weekplanner/weekplanner.go:235`. Prefer the cheapest model that
> still produces sensible weekly allocations.
