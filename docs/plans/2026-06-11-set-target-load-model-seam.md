# Plan: Resolve the load-model seam below the handlers

**Date:** 2026-06-11
**Status:** Proposed (from the 2026-06-11 domain-layer architecture review).
Delete this file in the change that ships it.

## Problem

`Progression` and `TimedProgression` are justified parallel implementations —
weighted and timed exercises progress by different rules. But the seam
between them resolves one layer too high: `buildProgressionTargets`
(`cmd/petra/handler-exerciseset.go`) switches on `exercise.LoadModel()`,
picks `service.BuildProgression` or `service.BuildTimedProgression`, and
threads two parallel values to the view — a `domain.SetTarget` and a bare
`int` of seconds, one always zero-valued. Load-model knowledge leaks into
the web layer.

## Direction

1. **Inventory consumers.** Find every reader of `SetTarget` and the timed
   target seconds (handler, templates, any service callers) to learn what
   shape the view actually needs.
2. **Design one set-target value covering both load models** (weight+reps or
   seconds). Either extend the domain's set target or introduce a service-level
   type — decide in session. `CONTEXT.md` defines **Set target** as "a weight
   and a target-reps value"; if the domain type is extended to timed
   exercises, reconcile that entry in the same change.
3. **One service entry point** ("next set target for this exercise slot")
   that owns the `LoadModel` switch. `LoadBodyweight`/`LoadUnknown` keep
   their current no-progression behaviour.
4. **Update the handler and templates** to consume the single value; the
   `LoadModel` switch leaves `cmd/petra`.

## Abort criterion

The review flagged this as "worth exploring", not "strong": if the unified
type turns into an awkward union that blurs the weight-vs-seconds split
(CONTEXT.md keeps kilograms and seconds strictly apart), the seam genuinely
belongs at two methods — stop, and record the reason as an ADR so the next
architecture review doesn't re-suggest it.

## Constraints

- No UI copy changes; UI register labels stay as they are.
- `make ci` green before push.
