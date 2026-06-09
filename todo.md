# CONTEXT.md follow-ups (temporary tracking file)

Changes the glossary work implies but that live outside CONTEXT.md, plus
deferred in-doc edits. Delete this file once all are done.

## Code ↔ glossary divergence

- [ ] `internal/petra/domain/mesocycle.go` — comments say "volume ramp" / "volume
      increase" (around `baseWeeklySets`/`peakWeeklySets` and `SetsForWeek`).
      CONTEXT.md now canonicalizes this as the **set-count ramp** and reserves
      "volume" for muscle-group set load. Reword the comments to "set-count ramp"
      so code and glossary agree. (Identifiers `baseWeeklySets`/`peakWeeklySets`
      already align — comments only.)

- [ ] `internal/petra/domain/muscle_group.go` — rename `PrimarySetWeight` /
      `SecondarySetWeight` (the 1.0 / 0.5 constants) to `PrimarySetCredit` /
      `SecondarySetCredit` to match the glossary's **set credit**. "Weight"/"load"
      now mean kilograms only. Update their doc comments ("per-set contributions",
      "weighted set load") and the `aggregateMuscleGroupLoad` / `WeeklyPlannedLoad`
      comments that call this "load." (`creditMuscleGroups` already aligns.)
      Ripples to repository/service/handler call sites — grep both constant names.

## Deferred in-doc edits (CONTEXT.md)

- [x] Strip remaining "Phase D" / "Phase C" references — only one existed
      (Relationships bullet), now removed. Swept clean.
- [x] Fix bare "volume ramps up" in the Training week definition → "set count
      ramps up" (the banned set-count-cluster usage).
