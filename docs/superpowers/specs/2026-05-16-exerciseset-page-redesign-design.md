# Exercise Set page redesign

## Problem

The Exercise Set page (`ui/templates/pages/exerciseset/`) is the last large
screen still wearing the pre-redesign template aesthetic. The recent workout
overview move ("serif training brief" — Charter title, mono overline, warm
stone surfaces, semantic warning/success states; commit `e0a6674`) means the
two pages no longer feel like they belong to the same app. The current page
has five concrete issues:

1. **Warmup banner is a giant boxed lecture.** Blue 2px border, a paragraph
   about why warmups matter, and a full-width ember CTA. Recurring noise on
   a recurring workflow — the user knows what a warmup is.
2. **Weight and reps don't align across set states.** Planned rows are a
   flex row with the value rendered as a sub-element; the active card is a
   centered two-column grid; completed rows are flex-wrap. No stable column
   for weight or reps, so numbers don't stack visually down the list.
3. **The `too_light` pill renders the raw `domain.Signal` enum.** Same for
   `too_heavy`. These are programmer strings reaching the user.
4. **The completed-set Edit affordance is a `.btn--ghost .btn--sm` chip.**
   It floats against the soft green completed surface and reads as chrome
   pasted onto the card, not as part of it.
5. **The active set card is a black slab.** It uses `--stone-9` as
   background and mono numbers with negative letter-spacing, in a page
   that's otherwise warm cream + serif. The tonal break breaks the visit.

## Goals

1. Pull the page into the workout overview's visual language: serif title,
   mono overline, warm surfaces, semantic warning/success states.
2. Make weight/reps numbers align vertically down the set list across all
   three states (planned / active / done).
3. Replace raw signal enum text with human-readable copy in both the
   completed-set badge and the active-card signal buttons.
4. Integrate the Edit affordance into the completed-set surface instead of
   bolting on a generic button.
5. Reduce warmup-banner chrome to a single slim row.

## Non-goals

- No changes to the signal submission flow. The form still POSTs
  `signal=too_heavy|on_target|too_light` and the handler is untouched.
- No changes to progression logic, set generation, deload behaviour, or any
  domain or service code.
- No changes to the 10-second prep timer + audio for time-based sets, the
  rest-chip computation, or the post-redirect-get flow.
- No changes to handler data shapes beyond the small additions called out
  below (display strings for signals, set count).
- Not in scope: time-based and bodyweight active cards beyond the same
  tonal/typographic update — their internal structure (`.timed-runner`,
  `.bodyweight-form`) stays as-is. The weighted-set redesign is the source
  of truth and time-based/bodyweight inherit the same warm-card treatment.

## Design

### Header — serif brief

Adopts the workout overview pattern. The exercise name becomes the visual
anchor; everything else collapses into mono overline / quiet links.

```
WED · MAY 16  ·  SET 3 OF 5  ────────────────
Deadlift                                     ← Charter, big
← Back                              Info · Swap
```

Markup shape (replacing `exercise-header.gohtml`):

```gohtml
<header class="exercise-brief">
    <div class="brief-overline">
        <span>{{ .Date.Format "Mon · Jan 2" }}</span>
        <span class="dot">·</span>
        <span>Set {{ .CurrentSetNumber }} of {{ .TotalSetCount }}</span>
    </div>
    <h1 class="exercise-title">{{ .ExerciseSet.Exercise.Name }}</h1>
    <div class="brief-nav">
        {{ template "back-link" (printf "/workouts/%s" (.Date.Format "2006-01-02")) }}
        <div class="brief-actions">
            <a class="brief-action" href=".../info">Info</a>
            <a class="brief-action" href=".../swap">Swap</a>
        </div>
    </div>
</header>
```

Style decisions:

- `.exercise-title` uses the same `Charter, "Iowan Old Style", Georgia,
  ui-serif, serif` stack and `clamp(2.25rem, 9vw, 3rem)` sizing as the
  workout overview's `.workout-title`. The `view-transition-name:
  exercise-title-{{ .ExerciseSet.ID }}` is preserved so the existing
  page-to-page slide keeps working.
- `.brief-overline` uses the same `--font-mono`, `--font-size-0`, uppercase
  + `--font-letterspacing-3`, `--clay-4` colour, and trailing
  gradient-to-transparent line as `.workout-overline`.
- `.brief-action` links inherit colour, render as quiet text-buttons with
  a hover underline — no coloured pill backgrounds. The Info/Swap colour
  coding (blue / green) is dropped; both are navigation chrome.
- The elapsed-workout timer is **removed** from the header. It wasn't
  load-bearing (auto-hid at 30 min anyway) and added chrome. The rest
  chip stays, but moves into the active card (see below).

### Warmup — slim row, no lecture

Replaces the boxed banner with a single-row callout sitting in the same
visual slot as a planned set.

Markup shape (in `warmup.gohtml`, incomplete state):

```gohtml
<div class="warmup-row">
    <span class="warmup-label">Warm up</span>
    <span class="warmup-hint">a few easy reps to prime</span>
    <form method="post" action=".../warmup/complete">
        <button type="submit" class="btn btn--quiet btn--sm">Mark done</button>
    </form>
</div>
```

Style:

- Same `--color-surface-elevated` background, `--radius-3`, `--size-3`
  padding, and `var(--shadow-1)` as a planned set row, so it slots in
  visually as "step 0" before the sets.
- `.warmup-label` is a mono uppercase overline (`--font-size-0`,
  `--font-letterspacing-3`), `.warmup-hint` is normal body in
  `--color-text-secondary`, button right-aligned via `margin-left: auto`.
- The description paragraph ("Warming up properly prepares your muscles
  and joints…") is **dropped**. Recurring noise.

Completed state replaces today's bordered green pill with a single mono
line — no border, no background:

```
✓  warmup complete                            (--color-success, mono, --font-size-0)
```

The disabled state on the sets list (warmup not yet done) keeps
`pointer-events: none` but softens the visual: `opacity: 0.6` only, no
grayscale filter.

### Set list — one layout, three skins

A single grid serves all three states. Stable columns mean weight and reps
stack vertically down the page.

Grid: `[index] [weight × reps] [trailing slot]` where the trailing slot
holds either a signal badge + edit link (done), nothing (planned), or is
absent because the active card expands below.

```
Set 1     60.0 kg · 5      ✓                    edit
Set 2     60.0 kg · 6      ↑ too light          edit
─────────────────────────────────────────────
Set 3   (active — see below)
─────────────────────────────────────────────
Set 4     62.5 kg · 5      —
Set 5     62.5 kg · 5      —
```

Markup shape (per row, planned/done):

```gohtml
<div class="set-row {{ .StateClass }}">
    <span class="set-index">Set {{ .Number }}</span>
    <span class="set-figures">
        <span class="set-weight">{{ .WeightStr }}</span>
        <span class="set-times">·</span>
        <span class="set-reps">{{ .RepsStr }}</span>
    </span>
    {{ if .CompletedValue }}
        <span class="set-trailing">
            <span class="set-status">{{ .StatusGlyph }}</span>
            {{ if .SignalLabel }}<span class="set-signal">{{ .SignalLabel }}</span>{{ end }}
            <a class="set-edit" href="?edit={{ .Index }}">edit</a>
        </span>
    {{ end }}
</div>
```

Style decisions:

- `.set-row` is `display: grid; grid-template-columns: minmax(3.5rem, auto)
  1fr auto; align-items: baseline;` so the index column is stable, the
  figures column flexes, and the trailing slot is right-aligned.
- Numbers use the serif stack at `--font-size-2` (planned/done) — same
  family as the title but smaller. The `·` separator is the typographic
  middle dot in `--stone-4`. Tabular figures via `font-variant-numeric:
  tabular-nums` so digits align column-to-column.
- `.set-index` is mono uppercase overline (`--font-size-0`,
  `--font-letterspacing-3`, `--color-text-secondary`).
- Planned state: `--color-surface-elevated` background, `--shadow-1`.
- Done state: `--color-success-bg` background, figures and index tinted
  with `--color-success`. No border-colour change (the wash carries it).
- `.set-status` is a single check glyph (✓) inline with the text rather
  than a 1.5rem circular chip — it's still the same accessible meaning
  but reads as part of the row.
- `.set-signal` is the human label (see "Signal labels" below) styled
  as a small uppercase mono tag in `--color-success` (`--font-size-0`).
  Hidden for `on_target` (the expected case — no badge needed).
- `.set-edit` is a quiet text link in the same success colour, lowercase,
  with `text-decoration: underline; text-underline-offset: 0.2em` on
  hover. No border, no chip, no `.btn--*` class.

### Active card — warm, not black

The active set replaces the dark slab with the workout overview's
"in-progress" treatment: amber-warning wash + a left rule strip + a slight
elevation lift. The rest chip moves into the card header rather than
floating above the list.

```
┌─────────────────────────────────────────┐
│▌ SET 3                       ⏱ 2:28      │
│                                         │
│   62.5 kg   ×   5 reps                  │  ← Charter, hero
│                                         │
│   Weight (kg)         Actual reps       │
│   [  62.5  ]          [    5    ]       │
│                                         │
│   How did 5 reps feel?                  │
│   [ too heavy ]  [ BARELY ]  [ easy ]   │
└─────────────────────────────────────────┘
```

Style decisions:

- Background `--color-warning-bg`, `var(--shadow-2)` elevation,
  `border-radius: var(--radius-3)`, padding `--size-4`.
- A `::before` pseudo-element on the left edge as the amber rule strip,
  exactly mirroring the workout overview's `.exercise.started::before`
  (3px wide, `--color-warning`, vertical rule with right-side radius).
- Header row inside the card: a mono `SET 3` overline on the left, the
  rest chip on the right when active. The rest chip uses the existing
  `.rest-chip` markup but rendered inside the card; the top-of-page rest
  chip is removed.
- Hero figures row: serif numbers at `--font-size-fluid-3` (already used
  in the current card's value spans), tabular figures, separated by `×`
  in `--stone-4`. The current "WEIGHT" / "REPS" uppercase labels are
  dropped; the inputs below carry the labels.
- Input fields keep the `.input-field` shape but switch:
  - Border from `--stone-7` → `--stone-3` (warm light border on warm
    surface — same as the planned-row treatment).
  - Background from `--stone-8` → `--color-surface` (white-ish on the
    warm wash, reads as a proper input).
  - Text from `--stone-0` → `--color-text-primary`.
  - Focus ring stays ember (`--ember` + ember-tinted box-shadow).
- Question text: `<legend>` changes from "Did you reach 3 reps?" to
  "How did 5 reps feel?" — the buttons answer feel, not yes/no.
- Signal buttons relabelled:
  - `too_heavy`: **too heavy** (lowercase, ghost — `transparent` bg,
    `--stone-4` border, `--color-text-secondary` text).
  - `on_target`: **BARELY** (uppercase, ember — kept as-is, the existing
    treatment is already on-brand and is the user-visible "primary CTA").
  - `too_light`: **easy** (lowercase, ghost).
- The buttons keep their existing 3-column grid + single-column reflow
  at 380px and existing `name="signal" value="..."` so the form submit
  is unchanged.

### Time-based and bodyweight active cards

Same warm wash + rule strip + warm inputs as the weighted card. Their
internal structure (`.timed-runner` block for time-based, single
`.bodyweight-form` field for bodyweight) is preserved. The legend reword
applies to time-based: "How did 30s feel?" instead of "Did you reach 30s?".
Time-based gets the same `too heavy / BARELY / easy` button set since the
underlying signal vocabulary is identical (`progression_timed_test.go`
confirms `too_heavy`/`too_light` flow through the timed progression).

### Signal labels — display mapping

Display strings derive from the existing `domain.Signal` enum. Per the
"Display derivations belong on domain types" rule in `internal/domain/
CLAUDE.md`, the mapping lives on the domain type, not in the template:

```go
// internal/domain/set.go
func (s Signal) Label() string {
    switch s {
    case SignalTooHeavy: return "too heavy"
    case SignalTooLight: return "too light"
    case SignalOnTarget: return ""   // expected case — no badge text
    default:            return ""
    }
}

func (s Signal) Glyph() string {
    switch s {
    case SignalTooHeavy: return "↓"
    case SignalTooLight: return "↑"
    case SignalOnTarget: return ""
    default:            return ""
    }
}
```

Template renders `{{ .Glyph }} {{ .Label }}` only when `.Label` is
non-empty, so `on_target` sets don't get a badge. Existing handler tests
that assert against the badge value (none today — the only `signal_badge`
reference is the CSS class) are unaffected; new tests cover the mapping.

### Handler additions

Two small additions to `exerciseSetTemplateData` in
`cmd/web/handler-exerciseset.go`:

- `CurrentSetNumber int` — `FirstIncompleteIndex + 1`, for the brief
  overline.
- `TotalSetCount int` — `len(ExerciseSet.Sets)`, also for the overline.

Both are pure read-only derivations off existing data. No domain change is
needed for these — they're trivial enough to compute in the handler. If
the brief overline starts to grow, they could move to a small `Brief`
struct on `domain.ExerciseSet`, but YAGNI for now.

`setDisplay` gains:

- `SignalLabel string` — `set.Signal.Label()` if `Signal != nil`, else "".
- `SignalGlyph string` — `set.Signal.Glyph()` if `Signal != nil`, else "".

So the template no longer reaches into `$set.Signal` directly.

### Files touched

| File | Change |
| --- | --- |
| `internal/domain/set.go` | Add `Signal.Label()` and `Signal.Glyph()`. |
| `internal/domain/set_test.go` | Test the new mapping for all three values + nil. |
| `cmd/web/handler-exerciseset.go` | Add `CurrentSetNumber`, `TotalSetCount` to template data; add `SignalLabel`, `SignalGlyph` to `setDisplay`. |
| `ui/templates/pages/exerciseset/exercise-header.gohtml` | Rewrite as serif brief; drop elapsed-timer markup/script. |
| `ui/templates/pages/exerciseset/exerciseset.gohtml` | Drop the `initializeTimer()` script (no more elapsed timer). Keep the autofocus-on-reps-input script. |
| `ui/templates/pages/exerciseset/warmup.gohtml` | Slim row + single button; success state as one-line mono confirmation. |
| `ui/templates/pages/exerciseset/sets-container.gohtml` | Rewrite the per-row markup + scoped CSS to the new grid; rewrite the active-card block to warm-wash + amber strip; relocate `.rest-chip` into the active card; relabel signal buttons; reword `<legend>`. |

No service, sqlite, or routing changes.

## Testing

- `make test` — existing `handler-exerciseset_test.go` selectors must keep
  passing. The form `action`, `name="signal"` values, and the `name`/`id`
  attributes on weight/reps inputs are preserved. Button text changes
  (`Could do more` → `easy`, `No` → `too heavy`) will need test updates
  if any assertion matches on that copy — grep before editing.
- New `internal/domain/set_test.go` assertions for `Label()`/`Glyph()`.
- Manual on the deployed Playwright e2e suite (`cmd/web/playwright_test.go`)
  — there's a workout-flow test that submits sets; verify it still passes
  end-to-end after relabelling.
- Manual visual check on mobile viewport (350px, 380px, 414px): set list
  numbers align in a column; active card readable on warm wash; warmup
  row sits cleanly above the first set; rest chip inside the card doesn't
  collide with the SET overline.
- `make lint-fix` to keep golangci-lint clean.

## Open questions

None.
