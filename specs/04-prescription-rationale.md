# Per-Prescription Rationale

## Motivation

Once the prescription engine has attributes, volume bands, and a multi-axis
progression policy, the user gets a noticeably smarter plan — but they have
no idea *why* it changed. The state of the art in commercial planners is
remarkably poor on this front:

- **RP Hypertrophy** explains *what* it did (added a set; you're in a de-load)
  but not *why* in this specific user's data.
- **Fitbod** shows a recovery heat-map but doesn't tie exercise selection
  back to it.
- **Hevy Trainer** auto-progresses silently — App Store reviews repeatedly
  cite "I don't know why my weight didn't go up this week".
- **Liftosaur** is transparent only because the program is plain text the
  user can read; the system itself doesn't explain.

This is a free product win for petrapp: every prescription change is already
the output of a deterministic rule. If we capture *which rule fired and on
what input*, we get a one-line user-facing rationale at zero algorithmic
cost. It is the cheapest differentiation available, and it is what makes the
planner feel competent rather than magical.

## What we're adding

A single nullable `TEXT` column on the row that already exists for prescribed
sets, plus a small per-axis writer in the progression engine.

```sql
-- exercise_sets, prescribed columns are already on the row
rationale TEXT  -- nullable, < 256 chars
```

Examples of rationales the engine emits:

| Situation                                              | Rationale string                                                         |
|--------------------------------------------------------|---------------------------------------------------------------------------|
| First time seeing this exercise                        | `Starting weight from history average; reps from hypertrophy default.`    |
| `SignalTooLight` advanced load                         | `+2.5 kg — last session beat the rep target.`                             |
| `SignalTooLight` capped on load, advanced reps         | `+2 reps — load already at next dumbbell jump; reps cheaper than skipping a size.` |
| Spinal-load clamp prevented a 3-rep prescription       | `Reps held at 8 — back extension carries high spinal load; staying ≥6 reps.` |
| Slow-twitch shift on calf raise                        | `Reps shifted to 15–20 — soleus is slow-twitch dominant.`                 |
| MRV ceiling rejected a candidate                       | `Skipping cable row — lats already at 22/22 sets this week (MRV).`        |
| Variant advancement                                    | `Advanced to standing partial — completed 3 sessions of kneeling full at top reps.` |
| De-load on consecutive misses                          | `–10 % load — failed rep target two sessions in a row.`                   |

Three properties matter:

1. **Single line, ≤ 256 chars.** Anything longer is unread. The rendered UI
   surface is one row of light grey text under the prescribed weight × reps.
2. **Built from the same inputs the rule used.** Don't write a separate
   "explanation generator" that re-derives why; that's where commercial
   apps go wrong (the explanation drifts from reality). The rule writes its
   own rationale.
3. **Stored, not recomputed.** History queries should be able to surface
   "rationales for the last 4 weeks" without re-running the engine.

## Key design decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Storage | Single nullable `TEXT` column on `exercise_sets` | The prescription/log fusion in this codebase means the row is already there; no new table |
| Length cap | < 256 chars enforced in CHECK | Forces sentences, not paragraphs; matches the rest of `schema.sql` |
| Authorship | The rule that fires writes the rationale; `DeriveRepScheme` and `NextPrescription` return `(Prescription, string)` | One source of truth — the rule and its explanation can't drift |
| Composition | When multiple rules apply to one prescription, join with `; ` and cap to the first 256 chars | Keeps the contract simple; first rules listed are most important |
| Localization | English-only initially | The whole app is English-only; revisit when i18n lands generally |
| UI | Render under the prescribed line, dim text, no icon | Doesn't compete for attention with the actual targets |
| History | Surface rationales in the per-exercise history view alongside set logs | Lets users see a story of "why this got harder over time" |

## High-level shape

The signature change is small but consistent:

```go
// before
DeriveRepScheme(ex, p) RepScheme
NextPrescription(ex, scheme, history) Prescription

// after
DeriveRepScheme(ex, p) (RepScheme, []string)
NextPrescription(ex, scheme, history) (Prescription, []string)
```

Each rule that mutates the output appends one line. The handler joins the
slice with `; ` and stores the result. If the slice is empty, the column
stays NULL.

The volume-band layer in `02-volume-bands.md` does the same when it rejects a
candidate during selection, except those rationales attach to the
*selection decision* (which exercise was chosen) rather than the
prescription. To keep the schema minimal, store those on a sibling row in
`workout_sessions` (a single per-session `selection_rationale TEXT`) — they're
session-scoped, not set-scoped.

## Out of scope

- **End-of-mesocycle reports.** "Started at MEV=10, ended at 14 sets, weight
  went up 5 kg" is real product gold but belongs in a separate spec.
- **Causal explanation graphs.** The literature on explainable
  recommendation (Zhang & Chen, 2018) covers richer formats. Template-based
  rationales as above are sufficient for petrapp's audience and order of
  magnitude cheaper to ship.
- **User-overridable rationales.** If a user disagrees with the planner, the
  fix is to override the prescription (already supported), not to argue with
  the rationale.

## Acceptance

- Every prescription generated by `DeriveRepScheme` and `NextPrescription`
  with a non-default behavior carries a non-empty rationale string.
- Manual eyeball: pick 10 random recent sets in the per-exercise history
  view and confirm each one's rationale reads as a complete English sentence
  matching the user's data.
- The rationale never references state the user can't see (no internal
  variable names, no IDs).

## Brainstorming starter prompt

> I want to brainstorm `specs/04-prescription-rationale.md`. Two open
> questions: (1) Voice and length — pull the example rationales table and
> tell me which read as patronising, which read as useful, and how they'd
> sound after seeing the same one for the tenth time. Should rationales
> de-duplicate across consecutive identical sessions? (2) Where in the UI
> the rationale should live — under the prescription line on the
> exerciseset page, or only revealed on tap to keep the primary view clean.
> Codebase context: prescription rendering happens in
> `ui/templates/pages/exerciseset/sets-container.gohtml` and
> `cmd/web/handler-exerciseset.go`. Push back hard if a rationale is
> ever going to be more useful as silence.
