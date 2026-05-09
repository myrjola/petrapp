# Rep-Scheme Derivation From Per-Exercise Windows

## Motivation

Today the planner derives the rep target, set count, and (implicitly) rest from
a single per-session knob, `PeriodizationType`, applied uniformly across every
exercise in `internal/exerciseprogression/progression.go:50-52` and
`internal/weekplanner/weekplanner.go:39,297-301`:

```
Strength    = 5 reps, 3 sets
Hypertrophy = 8 reps, 3 sets
```

This produces wrong prescriptions for entire classes of exercise:

- **Heavy 5-rep back extensions** load the lumbar spine in shear — high injury
  risk for marginal adaptation. McGill's lumbar mechanics work and Hartmann
  et al. (2013) on disc loading both support keeping spinal-flexion-under-load
  movements ≥8 reps.
- **5-rep calf raises** are practically unloadable to a true 5RM and joint-stressful.
  Conventional programming puts them at 10–20 reps; modern hypertrophy research
  (Schoenfeld 2017, 2021) shows the 5–30 rep range is largely interchangeable
  for hypertrophy when proximity to failure is matched, so the high end is
  fine and the low end is wasted.
- **8-rep deadlifts** in a Hypertrophy week keep the spinal-load fatigue cost
  without the SBD-pattern strength reward. Powerlifting tradition reserves
  ≤6 reps for the deadlift for exactly this reason.

The fix is per-exercise rep windows. Each exercise carries `(rep_min, rep_max)`;
the periodization knob biases to one end of that window. A pure derivation
function turns `(rep_min, rep_max, periodization)` into a complete prescription
(reps, sets, rest).

## Decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Window source | Two `INTEGER` columns on `exercises`: `rep_min`, `rep_max` | Catalog is finite (~100 rows). Hand-coded values are cheaper to audit than a derived taxonomy and don't require maintaining a classification system. |
| Periodization–window interaction | Bias-within-window: `Strength → rep_min`, `Hypertrophy → rep_max` | Makes periodization a valid concept on every exercise; preserves the existing 2-week alternation. The window itself encodes "what range is appropriate"; periodization just cycles within. |
| Cycle length | 2 positions (Strength/Hypertrophy alternating, as today) | Endurance was deliberately removed from the planner. The bias-within-window model already gets high-rep work for exercises that need it (calves, back extensions) without a separate global "Endurance" position. |
| Scope | Reps + sets + rest, all derived from rep target | Set count and rest are functions of intensity in conventional programming; deriving them is small extra work and produces an honest prescription. |
| Derivation home | Pure function `DeriveScheme` in `internal/exerciseprogression` | Pure, unit-testable, single source of truth. Extends the existing `TargetReps(t)` helper in the same package. |
| Storage of `RestSeconds` | Per-`PlannedSet`, in-memory only | Matches the existing per-set `TargetValue` storage shape. `PlannedSet`s aren't persisted — they're recomputed each render — so this adds no DB column. |
| Time-based exercises | `rep_min`/`rep_max` NULL; `DeriveScheme` not called | `default_starting_seconds` already drives time-based prescriptions; mirror the existing CHECK pattern. |

## Data model

### Schema

`internal/sqlite/schema.sql`, `exercises` table:

```sql
rep_min INTEGER CHECK (rep_min IS NULL OR (rep_min >= 1 AND rep_min <= 50)),
rep_max INTEGER CHECK (rep_max IS NULL OR (rep_max >= 1 AND rep_max <= 50)),

-- table-level constraints
CHECK (exercise_type = 'time_based' OR (rep_min IS NOT NULL AND rep_max IS NOT NULL)),
CHECK (rep_min IS NULL OR rep_max IS NULL OR rep_min <= rep_max)
```

Numeric bounds (1–50) are guardrails, not policy; real values come from
`fixtures.sql`. Nullability mirrors `default_starting_seconds` (existing pattern
at `schema.sql:90`).

### Go types

`internal/exerciseprogression/scheme.go` (new):

```go
type Scheme struct {
    TargetReps  int
    TargetSets  int
    RestSeconds int
}

// DeriveScheme is pure: same inputs → same output, no DB, no clock.
func DeriveScheme(repMin, repMax int, p PeriodizationType) Scheme
```

`internal/weekplanner/weekplanner.go`, `PlannedSet`:

```go
type PlannedSet struct {
    TargetValue int
    RestSeconds int  // new
}
```

`Exercise` (the `weekplanner` view) gains `RepMin int` and `RepMax int`,
populated from the repository alongside `Category` and `ExerciseType`.

## Derivation rules

`DeriveScheme(repMin, repMax, p)` computes in three steps.

**Reps** — bias to the end of the window:

| Periodization | Target reps |
|---|---|
| `Strength` | `repMin` |
| `Hypertrophy` | `repMax` |

**Sets** — derived from the resulting target reps:

| Target reps | Sets |
|---|---|
| ≤ 5 | 4 |
| 6–10 | 3 |
| ≥ 11 | 3 |

Heavier work warrants more sets to accumulate volume. The high-rep bucket
stays at 3 (rather than 2) so weekly per-muscle volume stays inside Schoenfeld's
evidence range (~10+ sets/muscle/week) under the typical 2-session/muscle/week
cadence.

**Rest** — also derived from target reps, single integer per bucket:

| Target reps | Rest seconds |
|---|---|
| ≤ 5 | 180 |
| 6–10 | 150 |
| ≥ 11 | 90 |

180s for strength is supported by Schoenfeld 2016 and ATP-PCr recovery
kinetics. 150s for the middle bucket matches the same paper's finding that
trained lifters get slightly better hypertrophy from longer rests than the
60–90s "hypertrophy" tradition.

## Planner integration

`internal/weekplanner/weekplanner.go`:

- Delete `setsForPeriodization` (line 297) and the `setsPerExercise = 3`
  constant (line 39).
- Add a named `timeBasedSets = 3` so the time-based branch keeps its current
  3-set behavior explicitly.
- In `selectExercisesForDayWithPeriodization` (~line 420): for rep-based
  exercises, call `exerciseprogression.DeriveScheme(ex.RepMin, ex.RepMax, pt)`
  and build the `[]PlannedSet` slice with length `scheme.TargetSets`, each
  populated with `{TargetValue: scheme.TargetReps, RestSeconds: scheme.RestSeconds}`.
  For time-based, keep the existing `default_starting_seconds` branch and use
  `timeBasedSets`.

The repository layer (`internal/workout/repository-exercises.go`) selects
`rep_min`, `rep_max` alongside the existing exercise columns and surfaces
them on the domain `Exercise` model.

## Migration and seed

The declarative migrator handles the column adds because they're nullable —
no premigration needed.

`fixtures.sql` is updated in the same commit to set `rep_min`/`rep_max` for
every non-time-based exercise. Seed defaults by exercise family:

| Family | rep_min | rep_max | Examples |
|---|---|---|---|
| Heavy spinal-load compounds | 3 | 6 | Deadlift (conv/sumo), back squat |
| Compounds, non-spinal-heavy | 5 | 10 | Bench, OHP, front squat, pull-up |
| Lumbar-stress accessories | 8 | 20 | Back extension, good morning, RDL |
| Isolation, large muscle | 8 | 12 | Bicep curl, tricep extension, lateral raise |
| Isolation, small/slow muscle | 10 | 20 | Calf raise (standing/seated), rear delt fly, wrist curl |
| Stability/anti-extension | 8 | 15 | Pallof press, rep-counted ab work |

Production already holds `exercises` rows beyond what `fixtures.sql` defines
(manually backfilled via `docs/`). Fixtures use `INSERT … ON CONFLICT DO UPDATE`
so any matching production row gets backfilled on next boot. Before merging,
use the `fly-ops` skill to snapshot prod, identify orphans (exercises in prod
but not in the fixture), and either add them to the fixture or write a one-shot
`docs/2026-05-09-backfill-rep-windows.sql` to cover them. Rows that remain
NULL after fixture/script application would fail the table-level CHECK on
next boot — verify zero orphans before merging the schema change.

## Testing

- **`internal/exerciseprogression/scheme_internal_test.go`** (new): table-driven
  for `DeriveScheme`. ~30 rows covering each (window, periodization)
  combination at the boundaries (3-6, 5-10, 8-12, 8-20, 10-20) and the bucket
  edges (5/6, 10/11) for sets and rest.
- **`internal/weekplanner/weekplanner_internal_test.go`** (update): tests that
  hard-code `setsPerExercise == 3` (lines 287, 559) and
  `TargetValue == repsStrength` (line 295) re-derive their expectations from
  the test exercise's window.
- **`internal/sqlite/migrate_internal_test.go`** (extend): round-trip from a
  pre-rep-min schema, exercising the existing migration coverage pattern.

## Acceptance

- `make test` passes with all updated and new tests.
- Manual eyeball of 10 generated weeks shows: deadlift weeks at 3 or 6 reps
  (never 8), calf raise weeks at 10 or 20 reps (never 5), back-extension weeks
  at 8 or 20 reps (never 5), bench weeks at 5 or 10 reps.
- Production fly-ops snapshot taken before merge; zero `exercises` rows left
  with NULL `rep_min`/`rep_max` after fixtures and any backfill SQL apply.

## Out of scope

- **RIR/RPE-driven prescription.** Modern hypertrophy research (Helms,
  Israetel, Schoenfeld) centers on proximity-to-failure rather than absolute
  rep targets. The existing `Signal` enum is a coarse 1-bucket RIR proxy used
  for set-to-set autoregulation; lifting it into prescription is a much
  larger redesign and a separate spec.
- **Per-exercise sets override.** Sets are derived from rep target only. If
  an exercise needs unusual set counts, that's a future attribute.
- **Volume bands per muscle group.** Was its own removed spec
  (`02-volume-bands.md`). Independent of this work.
- **Rest UI.** This spec adds `RestSeconds` to the in-memory model but does
  not specify how it surfaces in templates. UI work is a follow-up.
- **Bodyweight/multi-axis progression.** Pistol-squat regressions, tempo as
  a progression axis, etc. (was `03-multi-axis-progression.md`). Independent.
- **Endurance periodization.** Deliberately omitted from the planner cycle;
  the bias-within-window model gets high-rep work for the exercises that
  warrant it without a separate global position.
