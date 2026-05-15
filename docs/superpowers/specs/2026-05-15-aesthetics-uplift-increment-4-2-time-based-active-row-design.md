# Aesthetics Uplift — Increment 4.2: Time-based Active-row Oversized Display — Design

**Status:** drafted 2026-05-15. Approved sections 1–4 in brainstorm; awaiting written-spec review.

## Problem

In Focus mode, when the active set is a **time-based** exercise (e.g. Plank,
held for seconds rather than counted in reps), the oversized-numeral active row
in `ui/templates/pages/exerciseset/sets-container.gohtml` renders with two
defects:

1. **Off-centre layout.** The active grid is `grid-template-columns: 1fr 1fr`,
   and the time-based branch only emits a single `.reps` span. CSS auto-placement
   drops that single span into column 1, leaving column 2 empty. The oversized
   value reads as if it were the left half of a 2-up display that lost its
   right half.
2. **Wrong column-header label.** The `.reps::before { content: "Reps"; }`
   pseudo-element renders "REPS" above the seconds value (e.g. "120s"), which is
   factually wrong — the content is a duration, not a rep count.

The same off-centre layout defect (single `.reps` in a 2-col grid) hits **active
bodyweight** rows via the same template branch — for bodyweight the "REPS"
label is correct, but the layout is still misaligned. This spec fixes both
cases with one shared CSS rule.

The defect was flagged out-of-scope during Increment 4.1's brainstorm
(`docs/superpowers/plans/2026-05-15-aesthetics-uplift-increment-4-1-focus-mode-layout-polish.md`,
"Out of scope" section). This increment closes it.

## Goal

Render single-stat active oversized rows (time-based and bodyweight) as a
single centered column with a correctly-labelled `::before` pseudo-element
header, matching the glanceability and visual weight of the weighted/assisted
two-column treatment.

## Design

### Markup change (one line)

In `ui/templates/pages/exerciseset/sets-container.gohtml`, the active
time-based branch (currently lines 477–478) renames its class and drops the
trailing `s` text suffix:

```diff
- {{ else if and (eq $.ExerciseSet.Exercise.ExerciseType "time_based") $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
-     <span class="reps" aria-label="Recommended target"><span class="value">{{ $.CurrentSetTimedTarget }}</span>s</span>
+ {{ else if and (eq $.ExerciseSet.Exercise.ExerciseType "time_based") $.ExerciseSet.WarmupCompletedAt (eq $.FirstIncompleteIndex $index) }}
+     <span class="time" aria-label="Target time in seconds"><span class="value">{{ $.CurrentSetTimedTarget }}</span></span>
```

Three deltas:

1. `class="reps"` → `class="time"` — the new class names the CSS hook for the
   `Time` pseudo-element label and is semantically right.
2. `aria-label="Recommended target"` → `aria-label="Target time in seconds"` —
   more descriptive for screen readers since the visible "Time" label is now
   decoupled from the numeric unit.
3. Drop the trailing ` s` text node. Symmetric with Increment 4.1's "drop the
   `reps` suffix on the active row" decision: the column header conveys the
   unit; the oversized numeral stands alone.

The active **bodyweight** row at line 480 is **unchanged in markup** — its
`class="reps"` and "Reps" label are correct for that case. The layout fix
reaches it via CSS, not markup.

### CSS changes

Four edits inside the existing `<style {{ nonce }}>` block in
`sets-container.gohtml`, specifically inside the
`&.active { &:not(.completed) .set-info { … } }` rule
(lines 39–79).

```diff
  &:not(.completed) .set-info {
      display: grid;
      grid-template-columns: 1fr 1fr;
      gap: var(--size-4);
      align-items: center;
      justify-items: center;
      padding: var(--size-2) 0;

+     &:not(:has(.weight)) {
+         grid-template-columns: 1fr;
+     }

      @media (max-width: 380px) {
          grid-template-columns: 1fr;
          gap: var(--size-3);
      }

      .weight,
-     .reps {
+     .reps,
+     .time {
          display: flex;
          flex-direction: column;
          align-items: center;
          gap: var(--size-1);
          color: var(--stone-4);
          font-size: var(--font-size-0);
          font-weight: var(--font-weight-6);
          text-transform: uppercase;
          letter-spacing: var(--font-letterspacing-3);
      }

      .weight::before { content: "Weight"; }
      .reps::before { content: "Reps"; }
+     .time::before { content: "Time"; }

      .weight .value,
-     .reps .value {
+     .reps .value,
+     .time .value {
          display: block;
          font-size: var(--font-size-fluid-3);
          font-family: var(--font-mono);
          font-weight: var(--font-weight-7);
          color: var(--stone-0);
          line-height: 1;
          letter-spacing: -0.02em;
          text-transform: none;
      }
  }
```

How it composes:

- **`&:not(:has(.weight))` is the layout switch.** It fires when the active
  grid contains no `.weight` child — i.e. time-based (single `.time` child)
  and bodyweight (single `.reps` child). Weighted/assisted active rows render
  both `.weight` and `.reps`, so the `:has(.weight)` guard keeps them on the
  two-column grid.
- **`.time` joins the label-block selector** so the new class picks up the
  uppercased letter-spaced label typography that `.weight` / `.reps` already
  share.
- **`.time .value` joins the oversized-numeral selector** so the seconds digits
  render at `--font-size-fluid-3` with mono digits, matching the weighted
  display.
- **`.time::before { content: "Time"; }`** drives the header label visually.
- The mobile `@media (max-width: 380px) { grid-template-columns: 1fr; }` is
  preserved — weighted/assisted rows still drop to a single column on tiny
  screens, unchanged.

### Note on `:has()`

This is **the first use of the `:has()` pseudo-class anywhere in
`ui/templates/` or `ui/static/main.css`.** Browser support is universal across
current Chrome, Edge, Safari, and Firefox; the codebase already targets
modern engines via patterns like `@scope`, `view-transition-name`, native CSS
nesting, and `prefers-reduced-motion`. No fallback is needed. Reviewers should
expect this is a first — not a pre-existing pattern.

If a future increment finds a second site for the trick, consider extracting
either a `.set-info--single` modifier class or documenting a `:has()` recipe
in `ui/templates/CLAUDE.md`. Not in scope for 4.2.

## Test coverage

Add `Test_application_exerciseSet_time_based_active_oversized_layout` to
`cmd/web/handler-exerciseset_test.go`. The defect is currently uncovered:
`Test_application_exerciseSet` picks the first exercise from the generated
workout (always weighted), so no existing test exercises the time-based
active branch.

The new test mirrors the `Test_application_exerciseSet_assisted_storage`
pattern (lines 751–902): register a user, set workout preferences, start a
workout for today, then directly insert a `workout_exercise` row pointing at
the seeded `Plank` exercise (`internal/sqlite/fixtures.sql:382` — the only
seeded time-based exercise) with `warmup_completed_at` set, plus at least
one placeholder `exercise_sets` row.

```go
func Test_application_exerciseSet_time_based_active_oversized_layout(t *testing.T) {
    // ... register + preferences + start workout (same as assisted_storage) ...

    db := server.DB()
    var plankID int
    if err = db.QueryRowContext(ctx,
        `SELECT id FROM exercises WHERE name = 'Plank'`).Scan(&plankID); err != nil {
        t.Fatalf("get Plank id: %v", err)
    }

    var slotID int
    if err = db.QueryRowContext(ctx,
        `INSERT INTO workout_exercise (workout_user_id, workout_date, exercise_id,
            warmup_completed_at)
         SELECT user_id, workout_date, ?, STRFTIME('%Y-%m-%dT%H:%M:%fZ')
         FROM workout_sessions WHERE workout_date = ?
         RETURNING id`, plankID, today).Scan(&slotID); err != nil {
        t.Fatalf("insert plank slot: %v", err)
    }
    if _, err = db.ExecContext(ctx,
        `INSERT INTO exercise_sets (workout_exercise_id, set_number,
            weight_kg, target_value)
         VALUES (?, 1, 0.0, 30)`, slotID); err != nil {
        t.Fatalf("insert plank set: %v", err)
    }

    doc, err := client.GetDoc(ctx, "/workouts/"+today+"/exercises/"+strconv.Itoa(slotID))
    if err != nil {
        t.Fatalf("get exercise set: %v", err)
    }

    activeRow := doc.Find(".exercise-set.active .set-info").First()
    if activeRow.Length() == 0 {
        t.Fatalf("expected an active .set-info row on the Plank page")
    }
    timeCell := activeRow.Find(".time")
    if timeCell.Length() == 0 {
        t.Errorf("expected .time cell on active time-based row, got none")
    }
    if v := strings.TrimSpace(timeCell.Find(".value").Text()); v != "30" {
        t.Errorf("active .time .value = %q, want %q", v, "30")
    }
    // Single-column condition: no .weight cell renders alongside .time.
    if activeRow.Find(".weight").Length() != 0 {
        t.Errorf("active time-based row should not render a .weight cell")
    }
}
```

The assertions cover:

- `.time` cell exists (markup change applied).
- Its `.value` contains the raw integer (`"30"`, not `"30s"`).
- No `.weight` cell — this is what triggers the `:has(.weight)`-driven
  single-column layout.

The aria-label change is left unchecked because asserting visible behaviour
(presence of `.time` and absence of `.weight`) is sufficient to lock in the
layout switch; the aria-label is testable but adds a brittle assertion for
limited value.

## What's out of scope

- **Completed time-based row redundancy.** The completed-row markup at line
  475 renders `<span class="reps" aria-label="Completed value">{{ CompletedStr }} {{ Unit }}</span>`,
  which for a time-based exercise concatenates as `"30s seconds"` —
  `FormatSetValue` emits `"30s"` and `Unit` is `"seconds"`. A real but
  pre-existing redundancy, not in scope for the active-row defect this
  increment targets. Worth flagging in `simplify` / a future increment.
- **Upcoming time-based row redundancy.** The same `"30s seconds"` issue at
  line 480 in the upcoming-row default-else branch. Same disposition.
- **`min:sec` formatting.** The brainstorm considered `"2:00"` over `"120"`
  for long holds; the team chose raw integer seconds for symmetry with the
  bare-numeral `.reps` treatment on weighted rows. Reconsider if the seed
  data ever includes holds in the multi-minute range.
- **Handler/domain changes.** No new data shape; the handler already exposes
  `ExerciseType` and `CurrentSetTimedTarget`. Domain `Exercise.IsTimed()`,
  `Exercise.SetValueUnit()`, `Exercise.FormatSetValue()` are unused by this
  increment but remain available for future cleanups.
- **A11y wider review.** The aria-label tweak is the only a11y-visible
  change; nothing else about screen-reader behaviour is in scope.
- **Other Focus-mode polish.** Time-based form (`<form class="set-form timed-form">`
  lines 555–591), bodyweight form (lines 592–613), warmup, the workout
  timer, the exercise header — all untouched.

## Verification

Same shape as Increments 1–4:

1. **Cross-file palette grep gate** (must produce no output):
   ```bash
   grep -rnE 'var\(--(sky|lime|yellow|red|gray)-|var\(--white\)|var\(--black\)' \
     ui/templates/ ui/static/main.css
   ```
2. **Full exerciseset test suite** including the new test:
   ```bash
   go test ./cmd/web/ -run 'ExerciseSet|exerciseSet|computeSetActive' -v -count=1
   ```
   Expected: ten tests pass (the nine existing + the new
   `Test_application_exerciseSet_time_based_active_oversized_layout`).
3. **`make ci`** green: `init` + `build` + `lint-fix` (0 issues) + `test` + `sec`.
4. **Visual QA on `make dev`** — not performed in this environment; document
   that verification rests on the render test + grep gate + CI, not on a
   manual browser pass. Pre-merge, run the dev server and inspect a workout
   that includes Plank to confirm the single-centered-column treatment.

## Files touched

| File | Change |
|---|---|
| `ui/templates/pages/exerciseset/sets-container.gohtml` | One template branch (lines 477–478): class, aria-label, suffix drop. CSS additions inside the existing scoped `<style>` block: extend three selectors, add one `:has()` rule, add one pseudo-element rule. |
| `cmd/web/handler-exerciseset_test.go` | Add `Test_application_exerciseSet_time_based_active_oversized_layout` mirroring the assisted-storage test setup. |

No other files change. No Go handler or domain change.

## Relationship to prior increments

- Builds on Increment 4 (the dark `--stone-9` active panel, oversized numerals,
  `--ember` CTA, the `.weight` / `.reps` pseudo-element label pattern).
- Picks up the out-of-scope flag from Increment 4.1's "Out of scope" section
  (third bullet) verbatim.
- Compatible with Increment 5 (Stone palette rollout complete on `main` as of
  commit `7266b48`).
- No interaction with Increment 6 territory (if any).
