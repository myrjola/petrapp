# Attribute-Driven Exercise Classification

## Motivation

Today the planner derives rep targets from a single per-session knob —
`PeriodizationType` — applied uniformly across every exercise in
`internal/exerciseprogression/progression.go:50-52`:

```
Strength    = 5 reps
Hypertrophy = 8 reps
Endurance   = 15 reps
```

This produces wrong prescriptions for entire classes of exercise:

- **Heavy 5-rep back extensions** load the lumbar spine in shear — high injury
  risk for marginal adaptation. Established practice (NSCA, Renaissance
  Periodization, Stronger By Science) reserves <6-rep work for big compound
  patterns where the loaded-spine motor pattern is itself the goal.
- **5-rep calf raises** under-stimulate the soleus, which is ~70–90 % slow-twitch
  and grows from 15–25 reps with longer time-under-tension.
- **5-rep ab wheel rollouts** are nonsense — the rollout is anti-extension
  isometric stability work; the progression that matters is lever length
  (kneeling → standing) and ROM, not load.
- **8-rep deadlifts** in a hypertrophy mesocycle keep the spinal-load fatigue
  cost without the SBD-pattern strength reward.

The fix is **not** more periodization types. It is to give each exercise enough
attributes to *derive* a sensible rep range, then drive the progression engine
from those attributes instead of from the periodization constant alone.

## What we're adding

A small, fixed vocabulary of attributes on `exercises`, set once per row in
`fixtures.sql` and read by a pure derivation function:

| Attribute            | Values                                                              | Drives                                              |
|----------------------|---------------------------------------------------------------------|-----------------------------------------------------|
| `loading_type`       | compound / isolation / isometric_stability / plyometric / carry     | Whether reps or duration; whether load is the axis  |
| `spinal_load`        | none / low / moderate / high / very_high                            | Floor on rep range; clamps strength prescriptions   |
| `skill_demand`       | low / moderate / high                                               | RIR conservatism for novices                        |
| `rom_sensitivity`    | low / moderate / high                                               | Whether tempo is a meaningful progression axis      |
| `tempo_sensitivity`  | low / moderate / high                                               | Whether eccentric tempo should be prescribed        |
| `default_rep_min/max`| nullable integers                                                   | Per-exercise override of the periodization default  |

All are `TEXT CHECK (... IN (...))` enums, idiomatic in this STRICT-mode SQLite
codebase (`internal/sqlite/schema.sql:81-88` already uses this pattern for
`category` and `exercise_type`).

## Key design decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Storage | New columns on `exercises` (no sibling table) | All attributes are 1:1 with the exercise; a join buys nothing |
| Defaults | Sensible per-attribute defaults via `DEFAULT` clauses | Existing rows in `fixtures.sql` upsert cleanly without a values audit blocking the migration |
| Derivation | Pure Go function `DeriveRepScheme(Exercise, PeriodizationType) RepScheme` | Unit-testable without DB; one place to change the prescription model |
| Override path | `default_rep_min/max` win over the periodization default | Lets the seed encode "calves are always 15+ regardless of mesocycle" |
| Periodization stays | Keep the existing `PeriodizationType` enum | It's still the macro lever — strength/hypertrophy now *modulates* rather than *dictates* |
| No new feedback | Continue using the existing `Signal` enum | Attribute-driven prescription is independent of how the user reports effort |

## High-level shape

```go
// internal/exerciseprogression/repscheme.go (new)

type RepScheme struct {
    RepsMin     int
    RepsMax     int
    RestSeconds int
}

// DeriveRepScheme is pure: same inputs → same output, no DB, no clock.
func DeriveRepScheme(ex Exercise, p PeriodizationType) RepScheme
```

Derivation rules apply in this order, last write wins:

1. Start from the periodization default (5 / 8 / 15).
2. If `default_rep_min/max` are set on the exercise, override.
3. If `spinal_load` ≥ `high` and `p == Strength`, clamp `RepsMin` to ≥ 5.
4. If `loading_type == isometric_stability`, force the range to 6–12 with a
   strict-form flag.
5. Rest follows the *final* range, not the periodization: ≤8 reps → 2.5–4 min,
   9–15 → 90–120 s, 16+ → 60–90 s.

The progression engine in `progression.go` stops calling `TargetReps(t)` and
instead asks `DeriveRepScheme` for the range. The `Signal` autoregulation
loop is unchanged.

## Seed audit

Before merging, audit the ~20 highest-error-rate exercises in `fixtures.sql`.
The shortlist (rows that the goal-keyed system reliably gets wrong):

- Back extension, good morning, RDL, conventional deadlift, back squat, front squat
- Standing calf raise, seated calf raise
- Ab wheel rollout, plank, Pallof press, hanging leg raise, cable crunch
- Pistol squat, Nordic curl, Bulgarian split squat
- Bench press, pull-up, lateral raise, face pull, wrist curl, farmer's carry

Rest of the catalogue can take attribute defaults and be revisited later.

## Out of scope

- Per-muscle fiber profile (covered in `02-volume-bands.md`).
- Multi-axis progression — the rep range coming out of `DeriveRepScheme` is
  still single-axis (load) for now (covered in `03-multi-axis-progression.md`).
- A rules engine. The five rules above are short enough that an inline
  switch in `DeriveRepScheme` is the readable choice. Defer the engine until
  the rule count exceeds ~10.

## Acceptance

- A table-driven test in `repscheme_test.go` covers ≥30 (exercise,
  periodization) pairs with hand-picked expected ranges, including: deadlift in
  Hypertrophy stays ≤6, calf raise in Strength stays ≥12, ab rollout in any
  periodization is 6–12.
- Manual eyeball of 10 generated weeks shows none of the previously-wrong
  prescriptions surfacing.

## Brainstorming starter prompt

> I want to brainstorm `specs/01-exercise-attributes.md`. Walk me through the
> attribute set: are these the right six attributes, are the enum values right,
> and is there any attribute I'm missing that would let the seed encode the
> ~20 high-error-rate exercises faithfully? Then sanity-check the
> `DeriveRepScheme` rule order and clamp logic against the example exercises
> in the seed audit. Finally, identify the tradeoffs of putting all six
> attributes directly on `exercises` versus a sibling table or JSON blob.
> The codebase is Go + STRICT-mode SQLite with a single `schema.sql`; idiomatic
> patterns are visible in `internal/sqlite/schema.sql` and
> `internal/exerciseprogression/progression.go`.
