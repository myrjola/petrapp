# Exercise muscle-group cleanup — 2026-05-13

## Context

A data-quality audit of the production exercise library (`fly-sql-readonly`,
2026-05-13) surfaced a handful of muscle-group rows that were biomechanically
wrong and were inflating set counts for muscles that the planner shouldn't be
crediting.

The companion edits to `internal/sqlite/fixtures.sql` cover everything that can
go through the seed (renames via idempotent `UPDATE`s, `is_primary` flips via
`ON CONFLICT … DO UPDATE`, and added stabilizer rows). This script handles the
two things fixtures can't:

1. **Row deletions** — the fixture's `ON CONFLICT(exercise_id,
   muscle_group_name) DO UPDATE` cannot delete rows in prod that aren't in the
   fixture's `VALUES` list. Re-running fixtures.sql is silent about extra
   prod-only rows.
2. **AI-generated exercises** — `Pec Fly` was created by the in-app exercise
   generator (`internal/service/exercise_generation.go`) and is not in the
   fixture at all. Fixing it in prod requires direct SQL.

## What this script changes

| Exercise   | Action                              | Before                            | After                       |
|------------|-------------------------------------|-----------------------------------|-----------------------------|
| Push-Up    | drop `Biceps` primary               | primary {Biceps, Chest, Lats, Triceps}, secondary {Abs, Forearms, Shoulders, Upper Back} | primary {Chest, Triceps}, secondary {Abs, Forearms, Shoulders, Upper Back} |
| Push-Up    | drop `Lats` primary                 | (same row)                        | (same row)                  |
| Bench Press| drop `Upper Back` secondary         | primary {Chest, Triceps}, secondary {Abs, Forearms, Shoulders, Upper Back} | primary {Chest, Triceps}, secondary {Abs, Forearms, Shoulders} |
| Pec Fly    | drop `Biceps` + `Triceps` secondary | primary {Chest}, secondary {Biceps, Shoulders, Triceps} | primary {Chest}, secondary {Shoulders} |

## Reasoning per row

- **Push-Up / Biceps primary** — a push-up has no elbow-flexion load. Biceps
  isn't activated as a mover; at most it's a postural stabilizer in the
  forearm. Marking it primary was double-counting Biceps weekly sets every
  time Push-Up appeared in a workout.
- **Push-Up / Lats primary** — the lats stabilize the trunk during a push-up
  but the movement is shoulder horizontal-adduction and elbow extension. Lats
  isn't a prime mover and shouldn't share the "Lats" weekly-target slot with
  Pulldown / Cable Row, which actually train it.
- **Bench Press / Upper Back secondary** — the upper back retracts the
  scapulae isometrically to set the press; it doesn't perform a working
  contraction. Listing it as secondary credits 0.5× weighted load on every
  bench-press set, distorting Upper Back balance.
- **Pec Fly / Biceps + Triceps secondary** — fly variants fix elbow angle so
  neither the elbow flexors nor extensors take a working load. Only the chest
  (horizontal adduction) and front delts (stabilizing) are engaged.

## Ordering relative to the fixtures commit

This script assumes the matching fixtures commit has already deployed,
because it references the **new** names `Push-Up` and `One-Arm Dumbbell Row`.
The fixture's pre-INSERT renames (`UPDATE exercises SET name = ...`) run on
every boot and are idempotent, so by the time this one-shot runs the names in
prod will already be canonical.

If for some reason this script runs **before** the fixtures deploy, the
`WHERE name = 'Push-Up'` clause will match zero rows (prod still says
`Push-up`), the DELETE will be a no-op, and the data stays untouched. Safe.

## Out of scope

- The library still has gaps the audit flagged (only 2 primary shoulder
  exercises for a target of 10 weekly sets; no barbell overhead press, no
  barbell row, no rear-delt isolation, no hip thrust, no unilateral leg work).
  Those are exercise **additions**, not cleanups, and belong in a separate
  follow-up commit to `fixtures.sql`.
- AI-generated exercises sometimes pick up shoulder/triceps stabilizers as
  primary or secondary. The generation prompt in
  `internal/service/exercise_generation.go` could grow a "only credit a
  muscle when it performs a working contraction; stabilizers don't count"
  rule. Out of scope for this one-shot.

## Run

```
make fly-sql-write SCRIPT=docs/2026-05-13-exercise-cleanup.sql FLY_APP=petra
```

The make target snapshots the live DB to `/data/snapshots/` before applying
the script.
