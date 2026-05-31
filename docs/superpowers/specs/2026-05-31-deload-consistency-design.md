# Deload Week Consistency & Volume

## Problem

Two related issues with how deload weeks prescribe sets:

1. **Inconsistency between planner and swap/add.** Starting a session with 4
   sets per exercise, swapping to another exercise, then swapping back
   yields 2 sets — not 4. The on-screen set count diverges from what the
   swap/add code path produces.
2. **Deload volume is aggressive.** The current rule halves the set count
   (`(n+1)/2`, ceiling), so both 4-set Strength (`reps ≤ 5`) and 3-set
   Hypertrophy (`reps 6–10`) exercises converge to 2 sets in deload. The
   distinction between periodization styles collapses, and 2 sets often
   feels too low.

### Root cause of the inconsistency

The planner and the swap/add code path share the same set-derivation
function (`BuildPlannedSets` → `DeriveScheme`), so they cannot disagree
when run on the same session state. The divergence comes from
`Session.SwitchToDeload` and `Session.ClearDeload`
(`internal/domain/session.go:181`):

```go
func (s *Session) SwitchToDeload() error {
    s.IsDeload = true
    return nil
}
```

These methods flip the flag but leave the existing `Sets` slice
untouched. The flag is also flipped by `WeekPlan.FlipDeloadFromToday`
(called by `Service.StartDeloadNow` and by the preferences "Start Deload
Now" action) and by `WeekPlan.ClearDeloadFromToday` (called by
`Service.RestartMesocycleAnchor`).

After a flip, the session carries `IsDeload=true` and 4 stale sets.
`BuildSetsForAdd`, used by both swap and add, re-derives from the
new flag and returns 2 sets — hence the visible jump from 4 to 2 on the
first swap.

## Goals

- A single source of truth for "what set count does this exercise get in
  this session?" applied by the planner, by swap/add, and by every path
  that mutates the deload flag.
- A less aggressive deload volume that preserves the
  Strength-vs-Hypertrophy distinction.

## Non-goals

- Changing how deload affects rep targets (already forces `repMax` —
  intentional).
- Changing how deload affects rest seconds.
- Changing how deload is *detected* (mesocycle math stays as in
  `internal/domain/mesocycle.go`).
- Re-running the full planner on flip (no exercise re-selection).
- Configurable per-user deload presets.

## Design

### 1. Volume rule: drop one set, floor at 2

`internal/domain/progression_scheme.go` — replace `deloadSets`:

```go
const deloadSetFloor = 2

// deloadSets reduces the normal set count by one, with a floor of 2.
// Deload aims for ~25–33% volume reduction while preserving the
// Strength-vs-Hypertrophy set-count distinction.
func deloadSets(normalSets int) int {
    reduced := normalSets - 1
    if reduced < deloadSetFloor {
        return deloadSetFloor
    }
    return reduced
}
```

Effect on the three rep bands:

| Rep band              | Normal sets | Deload sets (new) | Deload sets (old) |
|-----------------------|-------------|-------------------|-------------------|
| `reps ≤ 5` (Strength) | 4           | 3                 | 2                 |
| `reps 6–10`           | 3           | 2                 | 2                 |
| `reps ≥ 11`           | 3           | 2                 | 2                 |
| Time-based            | 3           | 2                 | 2                 |

The Strength deload now keeps one more working set than the Hypertrophy
deload, mirroring the non-deload distinction.

### 2. Rebuild uncompleted sets on flip

`internal/domain/session.go` — `SwitchToDeload` and `ClearDeload` rebuild
each slot's uncompleted sets after toggling the flag, so the session's
persisted shape matches what `BuildPlannedSets` would have produced for
the new `IsDeload` value.

New private method on `Session`:

```go
// rebuildUncompletedSetsForCurrentPrescription rewrites each slot's
// Sets so that completed sets are preserved verbatim and the
// uncompleted tail matches what BuildPlannedSets would produce for the
// session's current PeriodizationType and IsDeload. Idempotent; called
// after SwitchToDeload / ClearDeload toggles IsDeload.
func (s *Session) rebuildUncompletedSetsForCurrentPrescription() { ... }
```

Per-slot algorithm:

1. Compute `freshSets := BuildPlannedSets(slot.Exercise, s.PeriodizationType, s.IsDeload)`.
   Let `n := len(freshSets)`.
2. Collect the completed entries from `slot.Sets` (`CompletedAt != nil`)
   in their original order into `completed`. Uncompleted entries are
   discarded — they will be replaced by fresh planner-shaped sets.
3. Compute the new length `final := max(len(completed), n)`.
4. Build the new slice of length `final`:
   - Indices `0..len(completed)-1`: the corresponding entry from
     `completed` (unchanged — TargetValue, CompletedValue, WeightKg,
     CompletedAt, Signal all stay).
   - Indices `len(completed)..final-1`: a fresh planner-shaped set — same
     shape `BuildPlannedSets` emits (`TargetValue` from the new
     prescription, all other fields nil). Because `BuildPlannedSets`
     returns sets that are identical to each other, the source index in
     `freshSets` is immaterial; any element works.
5. Assign the result back to `slot.Sets`.

Wire it into the two flag-toggle methods:

```go
func (s *Session) SwitchToDeload() error {
    s.IsDeload = true
    s.rebuildUncompletedSetsForCurrentPrescription()
    return nil
}

func (s *Session) ClearDeload() error {
    s.IsDeload = false
    s.rebuildUncompletedSetsForCurrentPrescription()
    return nil
}
```

Because `WeekPlan.FlipDeloadFromToday` and `ClearDeloadFromToday` both
call these `Session` methods (one per scheduled non-completed session
in the affected range), the rebuild propagates without changes to the
service layer or the repository.

### Behaviour examples

| Initial session                      | Flip                | Result                                 |
|--------------------------------------|---------------------|----------------------------------------|
| Strength, 4 untouched sets           | SwitchToDeload      | 3 uncompleted sets, TargetValue=repMax |
| Strength, 2 completed + 2 untouched  | SwitchToDeload (n=3) | 2 completed + 1 uncompleted = 3 sets   |
| Strength, 3 completed + 1 untouched  | SwitchToDeload (n=3) | 3 completed + 0 uncompleted = 3 sets   |
| Strength, 4 completed                | SwitchToDeload (n=3) | 4 completed, no shrink                 |
| Deload, 2 untouched sets             | ClearDeload (n=4)    | 4 uncompleted, TargetValue=repMin      |
| Deload, 1 completed + 1 untouched    | ClearDeload (n=4)    | 1 completed + 3 uncompleted = 4 sets   |

The rebuild never shrinks below `len(completed)`: work the user
actually did is never erased.

### 3. Doc-comment updates

The current comment on `SwitchToDeload` states:

> Sets recorded prior to this call retain their stored values; the next
> progression recommendation will be derived from GetDeloadStartingWeight
> rather than GetStartingWeight.

Update both `SwitchToDeload` and `ClearDeload` doc comments to describe
the new behaviour: completed sets retain their stored values; uncompleted
sets are rebuilt to match the current periodization + deload state.

## Out of scope

- **Mesocycle / deload detection logic** stays as-is
  (`IsDeloadWeek` in `internal/domain/mesocycle.go`).
- **Planner exercise selection** during deload stays as-is — only the
  set-count rule changes, which already flows through the planner via
  `BuildPlannedSets`.
- **Progression weight recommendation** under deload stays as-is — this
  spec changes set *count* and `TargetValue`, not weight.

## Testing

New tests under `internal/domain/`:

- `Test_deloadSets`: explicit table for 4→3, 3→2, 2→2 (floor), 1→2 (floor).
- `Test_DeriveScheme_Deload_PreservesIntent`: Strength repMin → Deload
  repMax (rep target stays at repMax), and set count matches the new
  formula for each rep band.
- `Test_Session_SwitchToDeload_RebuildsUncompletedSets`: 4-set Strength
  session → flip → 3 sets, all uncompleted, TargetValue=repMax.
- `Test_Session_SwitchToDeload_PreservesCompletedSets`: 2 completed +
  2 uncompleted Strength → flip → 2 completed (untouched) +
  1 uncompleted = 3 sets.
- `Test_Session_SwitchToDeload_OverQuota`: 4 completed of 4-set Strength,
  target deload = 3 → no truncation, 4 completed remain.
- `Test_Session_ClearDeload_ExpandsUncompletedSets`: 2-set deload → clear
  → 4 sets (Strength) or 3 sets (Hypertrophy) with the tail uncompleted.
- `Test_Session_ClearDeload_PreservesCompletedSets`: completed deload
  sets untouched after clear.
- `Test_Session_SwitchToDeload_Idempotent`: applying twice yields the same
  Sets slice (within slot-by-slot equality).
- `Test_Session_RebuildOnFlip_TimeBasedExercise`: a time-based exercise
  with 3 untouched sets goes to 2 on SwitchToDeload, and 3 on
  ClearDeload, with `TargetValue` carrying the seconds prescription.

Existing tests to update:

- Any `DeriveScheme` test asserting the old halving formula (e.g.
  `(4+1)/2 == 2`, `(3+1)/2 == 2`).
- Planner tests that assert deload set counts (`internal/domain/planner_test.go`).
- Service-level tests around `StartDeloadNow` / `RestartMesocycleAnchor`
  that may assert specific set counts on affected sessions
  (`internal/service/service_test.go`).
- Handler / e2e tests that snapshot or assert set counts on deload
  sessions.

## Affected files (expected)

- `internal/domain/progression_scheme.go` — new `deloadSets`,
  `deloadSetFloor` constant.
- `internal/domain/session.go` — `SwitchToDeload`, `ClearDeload`, new
  private `rebuildUncompletedSetsForCurrentPrescription`.
- Test files in `internal/domain/`, `internal/service/`, and `cmd/web/`
  as enumerated above.

No schema migration. No template changes (templates render whatever
`session.Slots[*].Sets` contains).

## Risk

- **Behaviour change visible to existing users mid-mesocycle.** A user
  partway through a deload week will see set counts shift on their
  uncompleted sets the next time the page loads. Acceptable — the new
  number is the one the planner would generate fresh and is consistent
  with swap/add.
- **Existing test churn.** A meaningful number of tests assert the old
  halving outcomes; they will need updating in lockstep with the
  domain change.
