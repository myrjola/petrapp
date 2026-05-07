# Time-Based Exercise Type

## Motivation

Today the planner emits `min_reps`/`max_reps` for every exercise from a single
per-session knob — `PeriodizationType` — yielding 5 reps for strength and 6–10
for hypertrophy. For an isometric hold like plank, currently typed as
`bodyweight` (`internal/sqlite/fixtures.sql:329`), this prescribes
"5 plank reps" or "6–10 plank reps" — meaningless for a hold.

The fix is to introduce a `time_based` exercise type whose sets carry a
duration target instead of a rep count. The same Signal-driven auto-regulation
that bumps weight up/down between weighted sets now bumps duration up/down
between holds.

## What we're adding

- A fourth value, `time_based`, on the `exercises.exercise_type` CHECK enum.
  Implies bodyweight (no `weight_kg`).
- A schema collapse on `exercise_sets`:
  `min_reps`/`max_reps`/`completed_reps` → `target_value`/`completed_value`.
  The unit (reps vs seconds) is derived from the parent exercise's type. This
  removes redundant data — the prior rep range was already a deterministic
  function of `session.periodization_type` and was used purely for display
  (`cmd/web/handler-exerciseset.go:36`); the progression engine in
  `internal/exerciseprogression/progression.go:85` already operates on a
  single integer target.
- A new `default_starting_seconds` column on `exercises`, populated for
  `time_based` rows. Lets the seed encode "plank starts at 30s for new users"
  without code changes.
- A parallel `TimedProgression` engine in `internal/exerciseprogression/`
  mirroring the existing weighted `Progression`, with a second-step ladder
  instead of a kg-step ladder.
- A render-time helper `formatTarget(exercise, session, target)` that produces
  the display string ("5", "6-10", "30s") in one place.
- Plank's fixture row flips to `time_based` with `default_starting_seconds = 30`,
  upserted via the existing `ON CONFLICT (name) DO UPDATE` clause in
  `fixtures.sql`.

## Key design decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Type axis | Add fourth `exercise_type` value `time_based` | Self-contained, simple, easy to migrate. Weighted timed holds are YAGNI. |
| Unit storage | Single `target_value` column; unit from parent `exercise_type` | min/max-reps are display-only; planner+progression use a single integer. Collapsing removes redundant data and gives the future spec 01 per-exercise overrides one column to populate. |
| Set-level unit flag | None | Sets inherit unit from the current exercise. Admin retypings are rare; if needed later we add a flag then. |
| Migration of historical plank rows | Drop them | Pre-migration plank logged "5 reps" — meaningless. Reinterpreting as "5 seconds" misleads; carrying a per-set unit flag forever to handle one historical exercise is overkill. |
| Migration strategy | One-shot premigration: CREATE+INSERT…SELECT+DROP+RENAME | Declarative migrator can't infer that `max_reps` becomes `target_value`. Standard pattern documented in `internal/sqlite/CLAUDE.md`. |
| Progression model | Separate `TimedProgression` type | Two small focused engines beat one engine with a unit flag. Lets the second-step ladder diverge from the kg-step ladder freely. |
| Signal | Reuse existing `Signal` enum | Auto-regulation works the same; the value being adjusted just happens to be seconds. |
| Sets per exercise | 3 (unchanged) | Plank fits the existing 3-sets-per-exercise rhythm. |
| Input format | Plain integer seconds | Plank/dead-hang holds rarely exceed 2 minutes. MM:SS adds parsing for no real gain. |
| Time-based ranges | Single target, no min/max | Auto-regulation does the work between sets. Ranges add UI noise. |
| First-session default | `default_starting_seconds` column on `exercises` | Users don't know what plank target to type on day one; weighted exercises don't have this problem because users know roughly what weight they can lift. |

## Schema

```sql
-- exercises: add fourth exercise_type and default_starting_seconds
exercise_type            TEXT NOT NULL DEFAULT 'weighted'
                         CHECK (exercise_type IN ('weighted', 'bodyweight', 'assisted', 'time_based')),
default_starting_seconds INTEGER CHECK (default_starting_seconds IS NULL OR default_starting_seconds > 0),
-- time_based exercises must have a default; other types may leave it NULL.
CHECK (exercise_type <> 'time_based' OR default_starting_seconds IS NOT NULL),

-- exercise_sets: collapse min/max/reps into target/completed
CREATE TABLE exercise_sets (
    workout_exercise_id INTEGER NOT NULL REFERENCES workout_exercise (id) ON DELETE CASCADE,
    set_number          INTEGER NOT NULL CHECK (set_number > 0),
    weight_kg           REAL,
    target_value        INTEGER NOT NULL CHECK (target_value > 0),
    completed_value     INTEGER CHECK (completed_value IS NULL OR completed_value >= 0),
    completed_at        TEXT CHECK (completed_at IS NULL OR
                                    STRFTIME('%Y-%m-%dT%H:%M:%fZ', completed_at) = completed_at),
    signal              TEXT CHECK (signal IS NULL OR signal IN ('too_heavy', 'on_target', 'too_light')),
    PRIMARY KEY (workout_exercise_id, set_number)
) WITHOUT ROWID, STRICT;
```

## Premigration

`internal/sqlite/premigrate.go`, method `preMigrateExerciseSetTarget`, wired
into `NewDatabase` between `connect` and `migrateTo`.

1. Detect via `pragma_table_info('exercise_sets')` whether `target_value`
   already exists; return early if so. Also early-return if `exercise_sets`
   doesn't exist (fresh DB / in-memory test startup).
2. `PRAGMA foreign_keys = OFF`. Begin transaction.
3. Create `exercise_sets_new` matching the new shape.
4. Translate:
   ```sql
   INSERT INTO exercise_sets_new
       (workout_exercise_id, set_number, weight_kg, target_value,
        completed_value, completed_at, signal)
   SELECT s.workout_exercise_id, s.set_number, s.weight_kg,
          s.max_reps, s.completed_reps, s.completed_at, s.signal
     FROM exercise_sets s
     JOIN workout_exercise wx ON wx.id = s.workout_exercise_id
     JOIN exercises e         ON e.id = wx.exercise_id
    WHERE e.name <> 'Plank';
   ```
5. `DROP TABLE exercise_sets; ALTER TABLE exercise_sets_new RENAME TO exercise_sets;`
6. Commit. The declarative migrator's `migrateTo(schemaDefinition)` then
   sees a database matching `schema.sql` and is a no-op for this table.

Translation rules:

- `target_value := max_reps` (top of the prescribed range; `completed_reps`
  is what actually matters retrospectively).
- `completed_value := completed_reps`.
- `weight_kg`, `completed_at`, `signal` carried 1:1.
- Plank rows dropped.

Test in `migrate_internal_test.go`: define `legacyExerciseSetsSchema`
reproducing the pre-migration shape, seed a hypertrophy bench row
(min=6, max=10, completed=8) and a plank row, run the premigration, assert
post-state, run again to prove idempotence, then call
`migrateTo(schemaDefinition)` to confirm no-op.

After this premigration runs in production, **delete the file, the call site,
the test, and the legacy-schema constant in one commit** — same lifecycle as
PR #75 (workout_exercise stable-id).

## Domain model

```go
// internal/workout/models.go

const ExerciseTypeTime ExerciseType = "time_based"

type Exercise struct {
    // ...existing fields...
    DefaultStartingSeconds *int  // nullable; populated only for time_based
}

func (e Exercise) IsTimed() bool { return e.ExerciseType == ExerciseTypeTime }

type Set struct {
    WeightKg       *float64    // nil for bodyweight and time_based
    TargetValue    int         // reps or seconds; unit from parent exercise
    CompletedValue *int        // same unit as TargetValue
    CompletedAt    *time.Time
    Signal         *Signal
}
```

`Set` deliberately doesn't carry the unit. Callers either have the parent
exercise (handler/render path) or don't need the unit (generic
persistence/aggregation).

## Progression engine

`internal/exerciseprogression/timed_progression.go` (new), parallel to
`progression.go`:

```go
type TimedConfig struct {
    StartingSeconds int  // caller-derived from history; default per-exercise on first session
}

type TimedSetTarget struct { TargetSeconds int }

type TimedSetResult struct {
    ActualSeconds int
    Signal        Signal
}

type TimedProgression struct { /* config, completed slice */ }

func NewTimed(c TimedConfig) *TimedProgression
func NewTimedFromHistory(c TimedConfig, completed []TimedSetResult) *TimedProgression
func (*TimedProgression) CurrentSet() TimedSetTarget
func (*TimedProgression) RecordCompletion(TimedSetResult)
func (*TimedProgression) SetsCompleted() int
```

Auto-regulation rules (mirrors the weighted decrement-factor pattern):

- `SignalTooLight` → `current + incrementSeconds(current)`
- `SignalOnTarget` → `current` unchanged
- `SignalTooHeavy` → `max(5, current - max(incrementSeconds(current), snap5(current * 0.10)))`

Increment ladder:

- `< 60s` → 5s step
- `60–119s` → 10s step
- `≥ 120s` → 15s step

Targets are snapped to multiples of 5 seconds for readability and floored at 5s.

## Service layer

In the existing exercise-execution setup path in `internal/workout/service.go`:

```go
if exercise.IsTimed() {
    starting := s.GetStartingSeconds(ctx, exerciseID, date)  // last completed_value, fallback to default_starting_seconds
    progression := exerciseprogression.NewTimed(TimedConfig{StartingSeconds: starting})
    // ...
} else {
    // existing weighted/assisted path via BuildProgression / GetStartingWeight
}
```

`GetStartingSeconds` is a new service method analogous to `GetStartingWeight`
(`service.go:557`): it pulls the latest successful set for this exercise — the
last set in DESC order by `(workout_date, set_number)` from sessions strictly
before the current date, where `completed_value IS NOT NULL` and
`signal IN ('on_target', 'too_light')` — and returns its `completed_value`.
Falls back to `exercise.DefaultStartingSeconds` when no successful history
exists. No periodization-based conversion is needed because periodization
does not apply to time-based exercises.

`UpdateCompletedReps` becomes `UpdateCompletedValue` — the handler passes a
unit-agnostic integer.

## Planner

`internal/weekplanner/weekplanner.go` emits a single `target_value` per
`PlannedSet` instead of `MinReps`/`MaxReps`:

- Rep-based: `TargetReps(periodization)` (5 or 8).
- Time-based: caller-supplied starting seconds (from service-layer history
  derivation).

`setsForPeriodization` collapses from returning `(minR, maxR int)` to
returning a single integer. Display range derivation moves out of storage and
into the render-time helper.

## UI

`cmd/web/handler-exerciseset.go`:

- `formatRepRange(min, max int) string` becomes
  `formatTarget(exercise Exercise, session Session, target int) string`:
  - `time_based` → `"30s"`
  - rep-based + strength → `"5"`
  - rep-based + hypertrophy → `"6-10"`
- `setDisplay` exposes pre-formatted `TargetStr`, `CompletedStr`, plus a
  `Unit` string ("reps" or "seconds") for input labels.
- Form field renames: `completed_reps` → `completed_value`. Same numeric input.

Templates (`ui/templates/pages/exerciseset/sets-container.gohtml`,
`pages/workout/workout.gohtml`) read `setDisplay`'s pre-formatted strings —
no per-template unit branching.

`cmd/web/handler-exercise-info.go`'s `processEntryData` already switches on
`ExerciseType` to format chart entries (e.g., `"8x100kg"` for weighted,
`"8 reps"` for bodyweight). Add a `case ExerciseTypeTime` branch that
formats `"30s"` from `set.CompletedValue`.

Admin form (`cmd/web/handler-admin-exercises.go`) adds `time_based` to the
type dropdown and a `default_starting_seconds` field shown only for that type.

## Out of scope

- Weighted timed holds (weighted dead hang, weighted plank). `time_based`
  implies no `weight_kg`.
- MM:SS input format. Plain integer seconds.
- Time-based ranges (hold 30–45s). Single target.
- Catalog expansion (dead hang, L-sit, wall sit, farmer's carry). The schema
  supports them without further changes; populate as needed.
- Cross-session conversion logic for time-based — `conversion.go` is
  rep-domain.
- The bigger attribute-driven prescription in
  `specs/01-exercise-attributes.md`. This work doesn't depend on it;
  per-exercise time targets eventually subsume into that system's
  `default_*` overrides.

## Acceptance

- Admin form roundtrips a `time_based` exercise; plank fixture lands with
  `default_starting_seconds = 30`.
- First-time plank session renders "Target: 30s" with a numeric input;
  submitting 35 with `Signal=too_light` causes the next set to render
  "Target: 35s".
- Cross-session: a plank session whose final set was 40s with
  `Signal=on_target` opens the next plank session at 40s.
- Existing rep-based exercises (bench, deadlift, …) render unchanged: "5"
  for strength sessions, "6-10" for hypertrophy.
- Premigration test: legacy `exercise_sets` schema with hypertrophy bench
  (min=6, max=10, completed=8) and a plank row migrates to
  `target_value=10, completed_value=8` for bench, plank row dropped.
  Idempotent on second invocation. `migrateTo(schemaDefinition)` is a no-op
  afterward.
- `TimedProgression` table-driven test covers ≥10 (current, signal) → next
  pairs across the three increment buckets, plus the 5s floor and the
  5s-snap rounding.
- After premigration runs in production, file/test/legacy-schema constant
  deleted in a single follow-up commit.

## Brainstorming starter prompt

> I want to brainstorm `specs/05-time-based-exercises.md`. Walk me through
> the schema collapse (single `target_value` instead of min/max) — does it
> lose anything that the rep-range UX gives users today? Sanity-check the
> `TimedProgression` increment ladder against a real plank session
> (start 30s, two too-lights, one on-target, one too-heavy). Then identify
> edge cases the premigration must handle: sessions in progress at deploy
> time, exercises whose type was changed mid-history, and any constraint
> interaction with `workout_exercise.warmup_completed_at`.
