# Domain Context — Ubiquitous Language

The shared vocabulary of the workout bounded context (`internal/petra/domain`).
These are the canonical names domain code, repository, handlers, and templates
should all use. When code and this file disagree, treat it as a bug in one of
them and reconcile.

## Training structure (the long view)

| Term              | Definition                                                                                                | Aliases to avoid              |
| ----------------- | --------------------------------------------------------------------------------------------------------- | ----------------------------- |
| **Mesocycle**     | A fixed-length training block of N weeks that ends in a deload week; repeats on a cadence                  | Cycle, block, program         |
| **Mesocycle length** | The number of weeks in one mesocycle, including the trailing deload week                                | Block size, period            |
| **Mesocycle anchor** | The Monday that marks week 0 of the current mesocycle; all week-in-block math is measured from it       | Start date, epoch             |
| **Week in block** | The 0-based index of a date's week within its mesocycle (`0 … length-1`)                                  | Week number                   |
| **Training week**  | Any week of the mesocycle except the last; weeks where the set count ramps up (block indices `0 … length-2`) | Normal week, work week         |
| **Deload week**   | The last week of a mesocycle: reduced volume (fewer sets, floored so a couple remain) and hypertrophy-rep targets | Rest week, easy week, taper    |
| **Set-count ramp** | The week-over-week increase in per-exercise set count across the training weeks, peaking before the deload | Volume ramp, progression, build-up |
| **Periodization** | The rep-target style assigned **per session** — **Strength** (low reps) or **Hypertrophy** (high reps). Consecutive sessions alternate, and the week's starting style flips each week; independent of mesocycle position | Phase, intensity, style |

## The weekly plan

| Term              | Definition                                                                                                  | Aliases to avoid          |
| ----------------- | ---------------------------------------------------------------------------------------------------------- | ------------------------- |
| **Week plan**     | The seven-day plan for a Monday-anchored week: one session slot per day                                     | Schedule, program week    |
| **Workout day**   | A weekday with a positive planned duration in **Preferences**; gets a populated session                    | Training day, active day  |
| **Rest day**      | A weekday with zero planned minutes; carries an empty session (date only, no slots)                         | Off day, recovery day     |
| **Preferences**   | The user's per-weekday workout-minutes plan plus mesocycle/deload settings                                  | Settings, config          |
| **Planner**       | The domain service that generates a week plan from preferences, the exercise pool, and muscle-group targets | Generator, scheduler      |

## A single workout

| Term              | Definition                                                                                                       | Aliases to avoid            |
| ----------------- | --------------------------------------------------------------------------------------------------------------- | --------------------------- |
| **Session**       | One day's workout: an ordered list of exercise slots, a periodization, and deload flag, with lifecycle timestamps | Workout, day                |
| **Session status**| The lifecycle state derived from timestamps: **Not started → In progress → Completed**                          | State (unqualified)         |
| **Category**      | The muscle-split focus of a session or exercise: **Full Body**, **Upper**, or **Lower**                          | Split, type, focus          |
| **Workout type**  | The category *derived* from a session's actual slots (upper + lower present ⇒ full body)                         | —                           |
| **Exercise slot** | One position in a session: an exercise plus its sets; identity *is* the position (no surrogate ID)               | Entry, item, exercise (bare)|
| **Warmup**        | Per-slot preparation completed before working sets; tracked by a completion timestamp, reset on swap            | Warm-up sets                |
| **Swap**          | Replacing the exercise in a slot with another, keeping the slot's position; ranked by **swap similarity score**  | Substitute, replace         |
| **Difficulty rating** | The post-session 1–5 rating of overall session hardness                                                     | Difficulty, RPE, signal     |

## Exercises and sets

| Term               | Definition                                                                                                     | Aliases to avoid              |
| ------------------ | ------------------------------------------------------------------------------------------------------------- | ----------------------------- |
| **Exercise**       | A movement type (Squat, Bench Press), with a category, exercise type, and primary/secondary muscle groups      | Lift, movement                |
| **Exercise type**  | The load classification: **Weighted**, **Bodyweight**, **Assisted**, or **Time-based**                         | Kind, variant                 |
| **Load model**     | The measurement axis several exercise types share: **Weighted**, **Bodyweight**, **Timed** (drives progression/recording) | Mode                |
| **Set**            | One bout of an exercise: a target value (reps or seconds), optional weight, and — once done — completed value, signal, timestamp. Warmups are tracked on the **exercise slot** (a completion timestamp), not stored as Sets, so every Set counts toward volume | Rep, round |
| **Set count**      | The number of **Sets** prescribed per exercise for a session; driven by the **week in block**, not periodization | Sets (unqualified), volume  |
| **Scheme**         | The per-exercise rep + rest prescription for a session, derived from the rep window and periodization (no set count) | Prescription (unqualified)|
| **Rep window**     | An exercise's `RepMin … RepMax` band; periodization picks the low end (strength) or high end (hypertrophy)     | Rep range, target range       |
| **Set target**     | What the progression recommends for the *next* set: a weight and a target-reps value                           | Recommendation, goal          |
| **Signal**         | The user's perceived effort on a completed set: **Too heavy / On target / Too light**; drives weight progression | RPE, feedback, difficulty   |

## Muscle-group volume

| Term                | Definition                                                                                                      | Aliases to avoid            |
| ------------------- | -------------------------------------------------------------------------------------------------------------- | --------------------------- |
| **Muscle group**    | A canonical trained-muscle identifier (Chest, Lats, Quads…); an exercise has primary and secondary ones         | Muscle, body part           |
| **Muscle-group region** | A coarse anatomical grouping for UI layout: Upper Push / Upper Pull / Legs / Core / Other                  | Section, area               |
| **Muscle-group target** | A muscle group's weekly **hard-set** range (a hard set = a performed Set near failure): **MinSets** (≈ MEV, the floor the planner drives toward) and **MaxSets** (≈ MRV, the ceiling that penalizes excess). Authored in whole sets but compared against accumulated **volume** — a secondary set counts as a **fractional set** (½) toward it | Goal, quota |
| **Fractional set**  | A performed Set's contribution to one muscle group's weekly **volume**: a **primary** muscle gets a full set, a **secondary** a fractional (½) set. The term is the training literature's (Renaissance Periodization) | Set credit, set load, set weight, score |
| **Muscle-group volume** | A muscle group's weekly training stimulus for one muscle group, summed in **fractional sets** (planned vs completed) | Load, training load (it is sets, not kg) |

## Weight progression

| Term                | Definition                                                                                                       | Aliases to avoid           |
| ------------------- | --------------------------------------------------------------------------------------------------------------- | -------------------------- |
| **Progression**     | The set-to-set engine that recommends each set's weight from the prior set's signal (autoregulation)             | Engine, algorithm          |
| **Starting weight** | The seed weight for a session's first set, derived from history (user-overridable)                               | Initial weight             |
| **Increment**       | The load step added on a too-light signal: a small step in the dumbbell range, a larger one for plate-loaded weights | Step, bump                 |
| **Snap**            | Rounding a weight to the nearest realisable load (finer in the dumbbell range, coarser above)                    | Round (unqualified)        |
| **Deload seed weight** | The reduced, definitely-loadable first-set weight for a deload week                                          | —                          |

## Relationships

- A **Mesocycle** is `length` weeks long; its first `length-1` are **training weeks** and the last is the **deload week**.
- A **Week plan** holds seven **session** slots; each **workout day** has a populated session, each **rest day** an empty one.
- A **Session** carries one **periodization** and one deload flag, and holds ordered **exercise slots**.
- An **Exercise slot** binds one **exercise** to its **sets**; the slot's identity is its position.
- An **Exercise** has one **exercise type**, which maps to one **load model**, and one or more **muscle groups** (primary/secondary).
- A **Set** contributes to every muscle group its exercise touches — a full set to each primary, a **fractional set** (½) to each secondary — summed into **muscle-group volume**.
- **Set count** comes from the **week in block** (the **set-count ramp**); **reps + rest** come from **periodization** via the **scheme** — the two are independent prescriptions.
- The unit of **muscle-group volume** is the **fractional set**; a **muscle-group target** is authored in whole sets but compared against accumulated volume (a secondary set counts ½). "Load" means kilograms only — never muscle-group volume.

## Example dialogue

> **Dev:** "When the planner builds Wednesday's **session**, where does the **set count** come from — the **periodization**?"

> **Domain expert:** "No, those are decoupled. **Periodization** only decides reps and rest through the **scheme** — strength takes the low end of the **rep window**, hypertrophy the high end. The **set count** comes from the **week in block**: it ramps from its base up to its peak across the **training weeks**."

> **Dev:** "And on the **deload week**?"

> **Domain expert:** "The deload drops the set count — floored so a couple of sets remain — and forces hypertrophy reps regardless of the session's periodization. It's still training, just lower **volume**."

> **Dev:** "If I log a set as **too heavy**, that **signal** feeds the **progression**, right — not the **difficulty rating**?"

> **Domain expert:** "Right. **Signal** is per-set perceived effort and drives the next set's weight via autoregulation. The **difficulty rating** is the single 1–5 you give the whole **session** afterward — it doesn't touch the weight math."

## Flagged ambiguities

- **"Volume"** — bare **volume** means training stimulus per muscle, summed in **fractional sets**, i.e. **muscle-group volume**. The week-over-week growth in per-exercise **set count** is the **set-count ramp** — never "volume ramp." Keep the word "volume" out of the set-count cluster.
- **"Weight" and "load"** always mean kilograms lifted (`WeightKg`). A set's contribution to a muscle group's weekly volume (a full set for a primary, a **fractional set** for a secondary) is *never* a "weight" or a "load" — those are kilograms; volume is sets. ("Credit" was a retired in-house name for the fractional set.)
- **"Set"** is both the entity (one bout, with reps/weight/signal) and a count. When you mean the number per exercise, say **set count**; when you mean a muscle group's weekly MinSets/MaxSets, say **muscle-group target** (its unit is the **hard set**). "Working set" is fine in prose for a performed Set, but it is not a separate counted thing.
- **"Target"** appears three ways: **muscle-group target** (weekly hard-set range, MEV/MRV), **set target** (next-set weight + reps recommendation), and **target reps / target value** (the per-set rep-or-seconds goal — computed in a scheme/recommendation, stored as `TargetValue` once on a Set). Qualify which.
- **"Category" vs "Workout type"**: **Category** is the stored focus of an exercise or session; **workout type** is the category *derived* from a session's actual slots. They can differ — keep "workout type" for the derived value only.
- **"Difficulty" vs "Signal"**: **Signal** is per-set perceived effort feeding weight progression; **Difficulty rating** is the post-session 1–5. They are distinct inputs to distinct mechanisms — never use "difficulty" for the per-set signal.
- **Three "week" rhythms** are independent and easily conflated. (1) **Periodization** alternates session-to-session, and its weekly *starting* style flips week-to-week. (2) The **set-count ramp** climbs with the **week in block**. (3) The **deload week** is the last week of the **mesocycle**. Periodization parity is keyed to the calendar (weeks since epoch); the ramp and deload are keyed to the **mesocycle anchor**. One "week" is not interchangeable with another.
