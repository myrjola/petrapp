# exerciseprogression Integration Design

**Date:** 2026-04-19
**Scope:** Wire `internal/exerciseprogression` into the workout package and exercise set UI, implementing RIR-based auto-regulation with daily undulating periodization (DUP).

## Context

`internal/exerciseprogression` manages set-to-set weight progression for a single weighted exercise execution. This spec covers how it integrates with `internal/workout` (service layer) and `cmd/web` (HTTP handlers and templates).

The integration replaces the current static weight+rep-range model for weighted exercises with a signal-driven model: the user indicates proximity to failure after each set, and the package adjusts the next set's weight accordingly.

## Periodization Model

Two-type daily undulating periodization (DUP): sessions alternate between **Strength** (5 reps) and **Hypertrophy** (8 reps). Research (Rhea 2002, Miranda 2011) shows this heavy/moderate alternation is optimal for simultaneous strength and hypertrophy gains for the general population.

- **Endurance** (15 reps) is out of scope; deload weeks are a separate future feature.
- The type is assigned per session at generation time by counting the user's total prior sessions with `completed_at IS NOT NULL` mod 2: even = Strength, odd = Hypertrophy.
- The assigned type is stable — stored in the DB so it does not shift if session count changes between requests.

## Database Changes

### `workout_sessions` — add column

```sql
periodization_type TEXT NOT NULL DEFAULT 'strength'
    CHECK(periodization_type IN ('strength', 'hypertrophy'))
```

Assigned at `GenerateWorkout` / `StartSession` time. Follows existing text-enum convention (`'full_body'|'upper'|'lower'`).

### `exercise_sets` — add column

```sql
signal TEXT CHECK(signal IN ('too_heavy', 'on_target', 'too_light'))
```

Nullable until the user completes the set. Maps to `exerciseprogression.Signal` constants. The existing `completed_reps`, `weight_kg`, and `completed_at` columns are unchanged.

No new tables. `min_reps`/`max_reps` columns remain for bodyweight exercises; for weighted exercises they are superseded by the session's `periodization_type`.

## Workout Package Changes (`internal/workout/`)

### New domain types

- `PeriodizationType` — mirrors `exerciseprogression.PeriodizationType`, values `PeriodizationStrength` and `PeriodizationHypertrophy`. Keeps `exerciseprogression` out of the repository and DB serialization layers.
- `Signal` — mirrors `exerciseprogression.Signal`, values `SignalTooHeavy`, `SignalOnTarget`, `SignalTooLight`. Same rationale.
- `Session.PeriodizationType PeriodizationType` — new field on the `Session` domain model.
- `Set.Signal *Signal` — nullable; nil until the set is completed.

Handlers and templates import `exerciseprogression` directly for `Progression` and `SetTarget` — the mirroring is for the domain model and DB layer only, not for full package isolation.

### Service method changes

**`GenerateWorkout()` / `StartSession()`** — before creating the session aggregate, count the user's total completed sessions, take mod 2, assign `periodization_type`.

**New: `GetStartingWeight(ctx, exerciseID) (float64, error)`** — loads the most recent completed session for that exercise, returns `weight_kg` from its first set. Returns `0.0` if no history exists (user adjusts from zero on first encounter).

**New: `RecordSetCompletion(ctx, date, exerciseID, setIndex int, signal Signal, weightKg float64, reps int) error`** — atomically persists `signal`, `weight_kg`, `completed_reps`, and `completed_at` for one set. Replaces the current separate `UpdateSetWeight` + `UpdateCompletedReps` calls.

**New: `BuildProgression(ctx, date, exerciseID int) (*exerciseprogression.Progression, error)`** — derives `exerciseprogression.Config` from the session's `periodization_type` and `GetStartingWeight`, then calls `exerciseprogression.NewFromHistory` with the completed sets for that exercise. Returns a ready `Progression`; the handler calls `progression.CurrentSet()` to obtain the recommendation.

## HTTP Handler Changes (`cmd/web/`)

### `exerciseSetGET`

- Calls `app.workoutService.BuildProgression(ctx, date, exerciseID)`.
- Calls `progression.CurrentSet()` → `SetTarget{WeightKg, TargetReps}`.
- Adds `CurrentSetTarget exerciseprogression.SetTarget` to `exerciseSetTemplateData`.
- Drops rep-range string formatting for weighted exercises — target reps come from `CurrentSetTarget.TargetReps`.

### `exerciseSetUpdatePOST`

- Parses `signal` from form (`too_heavy`, `on_target`, `too_light`).
- Parses `weight_kg` from form (editable input, pre-filled from recommendation).
- Parses `actual_reps`: present only when `signal == too_heavy`; for `on_target`/`too_light`, derived from `target_reps` hidden field.
- Calls `app.workoutService.RecordSetCompletion(ctx, date, exerciseID, setIndex, signal, weightKg, reps)`.
- Redirects back to the exercise page as today.

No new routes. The two-step TooHeavy flow is client-side only (JS reveals the reps input when "No" is selected; all data is submitted in one POST).

## UI Changes (`ui/templates/`)

### `exerciseset.gohtml` — active set

The active set area changes from a weight+reps form to a signal-first interaction:

```
┌─────────────────────────────────────┐
│  Set 2  ·  Hypertrophy              │
│                                     │
│  Weight: [45.0 kg            ▼]    │
│                                     │
│  Did you reach 8 reps?              │
│  [No]  [Barely]  [Could do more]    │
│                                     │
│  (revealed only when No is clicked) │
│  Actual reps: [___]   [Submit]      │
└─────────────────────────────────────┘
```

- **Weight input**: pre-filled from `CurrentSetTarget.WeightKg`, editable.
- **Signal buttons**: "No" (TooHeavy), "Barely" (OnTarget), "Could do more" (TooLight). "No" reveals a hidden reps input via a small JS snippet. "Barely" and "Could do more" submit immediately with `reps` set to `TargetReps` via a hidden field.
- **Completed sets**: continue to show lime background + checkmark, now displaying signal alongside weight/reps (e.g., "45 kg × 8 · Barely").
- **Session label**: periodization type shown as a pill/badge near the set header (e.g., "Hypertrophy").

No changes to `workout.gohtml`.

## Data Flow Summary

```
exerciseSetGET
  → workoutService.GetSession()          — loads session (incl. periodization_type, sets with signal)
  → workoutService.BuildProgression()    — derives Config, calls NewFromHistory
  → progression.CurrentSet()            — returns SetTarget{WeightKg, TargetReps}
  → render template with CurrentSetTarget

exerciseSetUpdatePOST
  → parse signal, weightKg, reps from form
  → workoutService.RecordSetCompletion() — persists signal + weight + reps atomically
  → redirect → exerciseSetGET
```

## Scope Constraints

- **Weighted exercises only.** Bodyweight exercises are unaffected; their sets continue to use `min_reps`/`max_reps`.
- **No cross-session history access in `exerciseprogression`.** The workout service derives starting weight and passes it via `Config`.
- **No deload weeks.** Separate future feature.
- **No Endurance periodization type.** Can be added later as a third rotation step.
