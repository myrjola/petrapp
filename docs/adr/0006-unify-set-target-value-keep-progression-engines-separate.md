# Unify the set-target value, keep the progression engines separate

The progression seam resolved one layer too high: `cmd/petra` switched on
`LoadModel` to pick `BuildProgression` vs `BuildTimedProgression` and threaded
two parallel values (a `SetTarget` and a bare seconds `int`, one always zero)
to the view. We pushed the switch down into one service entry point,
`Service.NextSetTarget`, returning a single `domain.SetTarget{WeightKg,
TargetValue}` — `TargetValue` being the rep-or-seconds axis the `Set` entity
already models, so kilograms and seconds never share a field (per CONTEXT.md's
strict kg/sec separation).

We deliberately did **not** merge `Progression` and `TimedProgression`
themselves. Their *return value* and *service entry point* unify; their *rules*
(weight autoregulation by signal vs. duration stepping by threshold) stay as
two engines because they genuinely differ. Merging the value without merging
the engines is the intended end state, not an unfinished step — so a future
architecture review should not re-suggest collapsing the engines.
