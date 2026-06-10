# Demote Shoulders from primary on chest presses/flys — 2026-05-12

## Symptom

Home page muscle-balance section showed Shoulders red ("over") while
Biceps, Quads, Hamstrings were orange ("under") for user 24's week of
2026-05-11. The shoulders bar had been creeping up week-over-week:

| Monday     | Weighted load | Status     |
|------------|---------------|------------|
| 2026-04-06 |  9.0          | under      |
| 2026-04-13 | 10.5          | on-target  |
| 2026-04-20 | 10.5          | on-target  |
| 2026-04-27 | 13.5          | on-target  |
| 2026-05-04 | 15.0          | on-target  |
| 2026-05-11 | 16.5          | **over**   |

## Root cause

The week's primary-only shoulder sets were exactly **10**, matching the
seeded target. The "over" came entirely from **13 secondary sets**
piling on 6.5 of weighted load on top.

Shoulders was getting double-counted because three chest-dominant
exercises misclassified it as a PRIMARY muscle when it's at most a
synergist or stabilizer:

- `Bench Press` (id=2): `Chest + Shoulders + Triceps` all primary.
- `Incline Dumbbell Bench Press` (id=22): `Chest + Shoulders` primary.
- `Pec Fly` (id=30): `Chest + Shoulders` primary. Pec Fly is a chest
  isolation movement; shoulders is purely stabilizing.

Effect on the planner (`internal/domain/planner.go`):

- `Bench Press` being primary-Shoulders meant picking it for the
  "Chest" priority on day 1 also "consumed" the Shoulders allocation
  slot. The planner then allocated a second shoulder-primary exercise
  (`Lateral Raise`) on another day for the second shoulder slot. Net:
  10 primary shoulder sets per week.
- On top of that, seven other upper-body exercises (Pulldown, Tricep
  Pushdown, Ab Wheel Rollout, Dumbbell Bench Press, …) credit
  Shoulders as secondary, adding 6.5 weighted load with no upper bound.

## Fix (this script)

Move Shoulders from primary to secondary on the three pressing/fly
exercises listed above. Expected impact for the 2026-05-11 week:

- Shoulders primary sets: 10 → 3 (just Lateral Raise on Tue).
- Shoulders weighted load: 16.5 → ~13.0 (within 1.5× target → on-target).
- Chest, Triceps unchanged on Bench Press (Chest still primary;
  Triceps stays primary because triceps is a genuine prime mover).

The seed in `internal/sqlite/fixtures.sql` is updated for `Bench Press`
in the same commit. The fixture re-applies on every boot via
`ON CONFLICT DO UPDATE`, so the next deploy carries this change for
fresh databases. `Pec Fly` and `Incline Dumbbell Bench Press` are
AI-generated rows that don't appear in the seed — this script is the
only path to fix them in prod.

## Out of scope (consider if shoulder over-credit recurs)

- Triceps on `Bench Press` stays primary. Triceps is the largest
  synergist on the press and is conventionally treated as primary in
  hypertrophy programs. If Triceps starts showing over after this
  fix, revisit.
- The visualization in `cmd/web/handler-home.go:378` (`muscleStatus`)
  still compares weighted-load (primary + 0.5× secondary) against a
  target seeded as a primary-set count. If muscles with many
  synergist contributions keep flagging "over" even after individual
  misclassifications are corrected, change the comparison to
  primary-only sets, or recalibrate the seed targets in
  `internal/sqlite/fixtures.sql:463`.
- AI-generated exercises seem to over-claim shoulders as primary
  (Pec Fly, Incline DBP both made this mistake). The generation
  prompt in `internal/service/exercise_generation.go` may need a
  rule along the lines of "only mark a muscle as primary if it's a
  prime mover for the movement, not a stabilizer or synergist."

## Run

```
make fly-sql-write SCRIPT=docs/2026-05-12-demote-shoulders-primary-on-presses.sql FLY_APP=petra
```

The make target snapshots the live DB to `/data/snapshots/` before
applying the script.
