# WeekPlan aggregate with delete-and-reinsert persistence

All workout writes go through a single week-scoped aggregate
(`WeekPlanRepository.Update`), which persists by deleting every session row in
`[monday, monday+6]` inside the transaction (CASCADE clears slots and sets) and
re-inserting from the in-memory `domain.WeekPlan`. We chose this over per-day
aggregates with UPDATE diffing because the week is where the real invariants
live (plan regeneration vs. started sessions, deload flips, the week's
session-goal alternation), and a single `BEGIN IMMEDIATE` transaction closed the
check-then-act races that previously needed a per-user in-process mutex —
correctness comes from the database, not from locks.

## Consequences

- No diffing logic to maintain; the repository is a dumb mirror of the
  aggregate. At Petra's data sizes (a handful of exercises × sets per session)
  the write amplification is negligible.
- A second aggregate over the same tables (e.g. a separate `ExerciseLog`) was
  considered and rejected: no true cross-aggregate invariant exists, and
  `exercise_sets` carries targets and actuals in one row — they belong
  together.
- `SessionRepository` is read-only; reads are unaffected by the write boundary.
