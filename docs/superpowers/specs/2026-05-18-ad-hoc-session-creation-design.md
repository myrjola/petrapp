# Ad-hoc session creation

## Problem

The user cannot start a workout on:

1. A day that is not in their weekly preferences ("extra workout").
2. A day that became a workout day after the schedule was changed mid-week, when at least one other day this week had already been started (so `RegenerateWeeklyPlanIfUnstarted` skips).

The home page already surfaces the right "Start" buttons in both cases — `calculateWorkoutAction` returns `{StartWorkout: true, Label: "Start Extra Workout"}` for unscheduled today, and the normal "Start Workout" / "Start Early" for scheduled days that have no session row. The failure is in the service layer: `StartSession` calls `Sessions.Update(date, …)`, which returns `domain.ErrNotFound` because no row exists, and the handler then renders the "Not in This Week's Plan" page.

## Scope

In scope:

- Make `StartSession` succeed on dates that have no pre-generated session, lazily creating one.
- Keep both "special cases" inside the existing domain model (`domain.Session`, `domain.ExerciseSet`, the planner's existing primitives).
- No UI changes — the buttons already exist.

Out of scope:

- Asking the user to pick a duration / category / muscle focus for the ad-hoc session.
- Auto-creating sessions on GET. Lazy creation only happens on the explicit start action.
- Surfacing a way to "create + view" past missed days that weren't in the schedule (would need a new UI affordance).

## Approach

Add a single-day planning entry point to the domain and a lazy-create branch to `Service.StartSession`. The weekly planner is unchanged.

### Domain: `Planner.PlanDay`

```go
// PlanDay generates one Session for date, suitable for ad-hoc workouts on
// days outside the weekly plan. weekUsedExerciseIDs is the set of exercise
// IDs already used in other sessions this week; the planner avoids
// repeating them when possible.
func (wp *Planner) PlanDay(date time.Time, weekUsedExerciseIDs map[int]bool) (Session, error)
```

Decisions:

| Aspect | Source | Notes |
|--------|--------|-------|
| Category | `determineCategory(date)` | Existing adjacency rule. Isolated day → Full Body. |
| Exercises per session | `exercisesPerSession(prefs, date.Weekday())` if `>0`, else `exercisesMedium` (3) | Medium default only fires for true extra workouts. |
| Periodization | `nextPeriodizationType(firstSessionPeriodizationType(monday), idx)` | `idx` = count of scheduled workout days in prefs falling on or before `date.Weekday()` minus 1, clamped at 0. For an unscheduled date, prefs days strictly before `date.Weekday()` are counted (so `idx = 0` for an isolated extra workout, picking up the week's first periodization type). Matches the alternation the week planner would have used. |
| `IsDeload` | `IsDeloadWeek(monday, prefs.MesocycleAnchor, prefs.MesocycleLength, prefs.DeloadEnabled)` | Identical to weekly path. |
| Exercise selection | `selectExercisesForDayWithPeriodization(category, nil, n, pt, isDeload, weekUsedExerciseIDs)` | Pass `nil` priority muscle groups — no cross-week allocation context. Phase A skipped, Phase B picks non-conflicting exercises. |
| Errors | `"no exercises available for X day"` if pool empty | Matches existing `Plan` behavior. |

### Service: lazy creation in `StartSession`

```go
func (s *Service) StartSession(ctx context.Context, date time.Time) error {
    monday := mondayOf(date)
    existing, err := s.repos.Sessions.List(ctx, monday)
    if err != nil { return fmt.Errorf("list sessions for week of %s: %w", date.Format(time.DateOnly), err) }

    weekCount, hasDate, usedExerciseIDs := summarizeWeek(existing, date, monday)

    if weekCount == 0 {
        if err = s.generateWeeklyPlan(ctx, monday); err != nil {
            return fmt.Errorf("generate weekly plan for %s: %w", date.Format(time.DateOnly), err)
        }
        existing, err = s.repos.Sessions.List(ctx, monday)
        if err != nil { return fmt.Errorf("re-list sessions for week of %s: %w", date.Format(time.DateOnly), err) }
        _, hasDate, usedExerciseIDs = summarizeWeek(existing, date, monday)
    }

    if !hasDate {
        if err = s.createAdHocSession(ctx, date, usedExerciseIDs); err != nil {
            // A concurrent StartSession may have already inserted the row.
            // Treat unique-violation as success and fall through to Update.
            if !errors.Is(err, domain.ErrAlreadyExists) {
                return fmt.Errorf("create ad-hoc session %s: %w", date.Format(time.DateOnly), err)
            }
        }
    }

    err = s.repos.Sessions.Update(ctx, date, func(sess *domain.Session) error {
        return sess.Start(time.Now())
    })
    if errors.Is(err, domain.ErrAlreadyStarted) { return nil }
    if err != nil { return fmt.Errorf("update session %s: %w", date.Format(time.DateOnly), err) }
    return nil
}
```

`summarizeWeek` is a small private helper that walks `existing` once and returns:

- count of sessions whose date falls in the `monday..sunday` window
- whether `date` itself has a session
- the set of exercise IDs already used in any in-week session (for `weekUsedExerciseIDs`)

`createAdHocSession` is the new private method:

```go
func (s *Service) createAdHocSession(ctx context.Context, date time.Time, used map[int]bool) error {
    prefs, err := s.repos.Preferences.Get(ctx)
    if err != nil { return fmt.Errorf("get preferences: %w", err) }
    exercises, err := s.repos.Exercises.List(ctx)
    if err != nil { return fmt.Errorf("get exercises: %w", err) }
    targets, err := s.repos.MuscleTargets.List(ctx)
    if err != nil { return fmt.Errorf("get muscle group targets: %w", err) }

    sess, err := domain.NewPlanner(prefs, exercises, targets).PlanDay(date, used)
    if err != nil { return fmt.Errorf("plan day %s: %w", date.Format(time.DateOnly), err) }

    if sess.IsDeload {
        if err = s.seedDeloadWeights(ctx, &sess); err != nil { return err }
    }
    return s.repos.Sessions.Create(ctx, sess)
}
```

`seedDeloadWeights(ctx, *domain.Session) error` is extracted from the existing inline loop in `generateWeeklyPlan`, which then calls it in a loop over `plannedSessions`. Single source of truth.

### Repository: `Sessions.Create`

New method:

```go
// Create inserts a single session and its exercise slots. Used by ad-hoc
// session creation. The batch path (CreateBatch) is unchanged.
func (r *SessionsRepo) Create(ctx context.Context, sess domain.Session) error
```

Implementation reuses whatever single-session insert helper `CreateBatch` builds on (or factors it out if `CreateBatch` is monolithic).

## Edge cases

- **Past unscheduled day** — no UI path (`calculateWorkoutAction` returns `nil` for past+unscheduled). A direct POST to `/workouts/<past-date>/start` will succeed; this matches today's "Start Late" treatment for past scheduled days and isn't worth blocking.
- **Past newly-scheduled day** — the "Start Late" button navigates rather than POSTs, so the user lands on the workout page which 404s. Lazy creation only fires on the explicit start action; we don't auto-create on GET. Surfacing a "create + view" affordance for this case is out of scope.
- **Future scheduled day, "Start Early"** — works after the fix via the lazy-create branch.
- **Race on double-start** — `Sessions.Create` followed by `Sessions.Update` is not transactional. If two requests race, the second `Create` must fail on the date's UNIQUE constraint. The repository maps that to a new sentinel `domain.ErrAlreadyExists`; the service catches it (see `StartSession` code above) and falls through to `Update`, which is itself idempotent via `ErrAlreadyStarted`.
- **Empty exercise pool for the derived category** — `PlanDay` returns the existing `"no exercises available for X day"` error; service wraps it; handler treats it as a `serverError` (consistent with `generateWeeklyPlan`).

## Testing

**Domain — `internal/domain/planner_plan_day_test.go` (new file):**

- Isolated date (neither neighbor a workout day) → `CategoryFullBody`.
- Date with a workout neighbor → category from adjacency (Upper/Lower).
- Periodization index matches what `Plan(monday)` would assign for the same date.
- `IsDeload` propagates from preferences/anchor.
- `weekUsedExerciseIDs` keeps already-used exercises out of the selection.
- Empty-prefs day defaults to 3 exercises (medium).
- Empty exercise pool for derived category returns the planner's existing error.

**Service — `internal/service/sessions_test.go` (additions):**

- `StartSession` on unscheduled today, empty week → session created, started, sets present.
- `StartSession` on unscheduled today, partial week (Monday already done) → ad-hoc session created and started; existing Monday session untouched.
- `StartSession` on newly-scheduled mid-week day after another day in the week has been started → matches the existing `Test_RegenerateWeeklyPlanIfUnstarted_SkipsRegenerateWhenWorkoutStarted` setup but additionally asserts the new day can be started.
- Double-start race: two concurrent `StartSession` calls on the same unscheduled date — both return nil, exactly one session row exists, `StartedAt` is set.

**Web — `cmd/web/handler-workout_test.go` (additions):**

- End-to-end: home → click "Start Extra Workout" for unscheduled today → land on workout page with exercises.
- End-to-end: change schedule mid-week to add a new day, start an existing day, then start the new day → land on workout page with exercises.

## Non-goals worth restating

- No second planning algorithm. `PlanDay` is the existing planner's primitives composed for a different input shape.
- No transactional Create+Start. The two-step happens behind a single service method and recovers from the only realistic race.
- No GET-side auto-creation. Lazy creation is bound to the explicit start action.
