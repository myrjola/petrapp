# Natural-key exercise slots: position is identity

Exercise slots have no surrogate ID. A slot's identity is the composite
`(workout_user_id, workout_date, position)`, where `position` is its 0-based
index in `Session.Slots`; children (`exercise_sets`, `scheduled_pushes`) key on
the same composite, and URLs use `{position}`. The previous surrogate
`workout_exercise.id` fought the delete-and-reinsert persistence (ADR 0003):
preserving rowids across the wipe while inserting new slots forced a fragile
three-pass reinsert to dodge UNIQUE collisions. The root cause was a model
mismatch — a surrogate key claims slots are independently-identified entities,
but they are aggregate-internal parts of the WeekPlan. Natural keys made the
reinsert a trivial single pass.

## Consequences

- Slot URLs are not stable across plan regeneration or swaps. Deliberately
  accepted: bookmarked deep links into a workout may 404 after the week is
  rewritten.
- Slot lookup in the domain is slice indexing with a bounds check; there is no
  find-by-ID helper to maintain.
