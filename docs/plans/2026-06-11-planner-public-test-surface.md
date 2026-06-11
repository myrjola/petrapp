# Plan: Give the Planner a public test surface

**Date:** 2026-06-11
**Status:** Proposed (from the 2026-06-11 domain-layer architecture review).
Delete this file in the change that ships it.

## Problem

The Planner's interface is deep — `Plan`/`PlanDay` hide 542 lines — but not
one test exercises it from outside the package. `planner_internal_test.go`
(1,619 lines) and `planner_plan_day_internal_test.go` (255 lines) pin private
helpers (`determineCategory`, `firstSessionGoal`,
`selectExercisesForDayWithGoal`, `pickBestExerciseIdx`, `scoreCandidate`);
there is no `planner_test.go`. Refactoring the implementation breaks tests
even when behaviour through `Plan` is unchanged, so the interface's depth
buys no leverage.

## Direction

1. **Classify the internal tests.** Each is either a behavioural contract of
   `Plan`/`PlanDay` (deload forces hypertrophy and reduces sets, no exercise
   repeats across the week, session diversity, category rotation, session-goal
   alternation) or an algorithmic detail of candidate scoring.
2. **Rewrite the behavioural ones** in `package domain_test` against
   `Plan`/`PlanDay` (`planner_test.go`), driving them through `Preferences`,
   exercise pools, and muscle-group targets only.
3. **Carve the scoring cluster into its own module** — `scoreCandidate`,
   `segmentReward`, `overlapLength`, `weekVolume`, `goalForWeek` form a
   coherent concept: score one candidate exercise against the day's context
   (session goal, deload, week volume, muscle-group targets, accumulated
   volume). Give it a small exported interface and move the scoring tests
   there. The module needs a name; add the chosen term to `CONTEXT.md`
   when it crystallises (candidate: "selection score", next to the existing
   "swap similarity score").
4. **Delete internal tests** made redundant by 2–3. Target: internal test
   files shrink to near zero.

## Constraints

- Pure refactor: `Plan` output for a fixed input must not change. Consider a
  golden snapshot of `Plan` for one rich fixture before starting.
- The deterministic tie-break (`pickBestExerciseIdx` falls back to lowest ID
  on equal scores) is load-bearing — keep a test for it at whichever
  interface it lands behind.
- `make ci` between every step; ship when green.
