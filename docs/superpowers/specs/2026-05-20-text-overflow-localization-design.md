# Text Overflow & Localization-Readiness — Design

**Date:** 2026-05-20
**Status:** Pending spec review
**Worktree:** `worktree-text-overflow-guidelines`

## Problem

On the home page "This Week" view, a long day name combined with a long
action label bleeds past the card edge and overlaps the day name (observed:
"Wednesday" + "START EXTRA WORKOUT →"). The app is not yet localized, so every
layout has only ever been validated against compact English strings. German,
Finnish, and Russian translations routinely run 30–50% longer — a future
localization effort would turn this single visible bug into a systemic one.

## Root cause

`ui/templates/pages/home/day-cards.gohtml` lays each card out with
`grid-template-columns: 1fr auto`. A bare `1fr` is `minmax(auto, 1fr)`: its
minimum is the *min-content* width of its contents, so column 1 will not
shrink below the word "Wednesday". Column 2 (`auto`) likewise demands its
content width. When `col1-min + col2-min + column-gap` exceeds the card width,
the grid overflows its container and `justify-self: end` pushes the action
past the right edge into the day name.

This is a general CSS trap: flex and grid children default to `min-width: auto`
and refuse to shrink below their longest word. The repo currently has almost no
defenses against it — one `min-width: 0` and one `overflow-wrap` across the
whole `ui/` tree.

## Goals

1. Document a concise, enforceable set of text-overflow and
   localization-readiness practices in the UI guidelines.
2. Fix the day-card overflow, so the corrected component is the worked example
   the documentation references.

## Non-goals

- Introducing an i18n framework or a centralized string catalogue (the app is
  not being localized right now).
- Auditing and fixing every other layout in the app.
- Building pseudo-localization tooling or styleguide long-string variants.
  Long-string review is documented as a practice; no tooling is built.

## Part 1 — Documentation

Add a new section to `ui/templates/CLAUDE.md`, titled
**"Text overflow & localization-readiness"**, placed immediately after the
"Accessibility" section — overflow resilience sits naturally next to a11y, and
both are standing cross-cutting requirements. Follow the file's house style:
rule-first, terse, each rule carrying its *why*, ending with a pointer to the
fixed `day-cards.gohtml` as the worked example.

The section has four rule groups:

**A. Let flex/grid children shrink.** Flex and grid items default to
`min-width: auto` (= min-content) and will not shrink below their longest word.
Use `minmax(0, 1fr)` rather than bare `1fr` for tracks that should yield; use
`min-width: 0` on text-bearing flex children. Apply `justify-self: end` /
`margin-inline-start: auto` only after the element is allowed to shrink —
otherwise it pushes itself off the edge.

**B. Every text node gets exactly one explicit overflow strategy:**

- *Wrap* (default, preferred) — no fixed height; the parent row uses
  `flex-wrap: wrap` if it holds competing items.
- *Break* unbreakable tokens (long compounds, URLs) — `overflow-wrap: anywhere`
  (note: `anywhere` shrinks min-content and so also fixes grid/flex track
  sizing; `break-word` does not), optionally with `hyphens: auto`.
- *Truncate* — `text-overflow: ellipsis` or `-webkit-line-clamp`, only for
  secondary, repeating text whose full value is reachable elsewhere (a `title`
  attribute, an `aria-label`, or a detail view). Never silently truncate a
  primary label.

**C. Don't make two flexible elements compete for one row.** Either let the row
wrap, or let the lower-priority element move to its own line. No fixed widths or
heights on text containers — `min-*` plus padding is fine, fixed `width` /
`height` is not. Buttons sized to fit an English label break in other
languages.

**D. Design for string growth.** Budget roughly +40% length for non-English
translations. Validate new components at 320px width with a deliberately long
label, not just the English string. Keep source labels short, but a short label
is never a substitute for a layout that survives a long one.

Target length: each rule two to three lines; whole section ~35–45 lines.

## Part 2 — Fix `day-cards.gohtml`

Restructure the day name and action so they can never collide:

- Wrap `.day-name` and `.day-action` in a single full-width container
  (`grid-column: 1 / -1`) that is a wrapping flex row:
  `display: flex; flex-wrap: wrap; align-items: baseline;
  justify-content: space-between; gap: var(--size-2) var(--size-3)`
  (row gap when wrapped, column gap inline).
- When the name and action fit on one line they sit name-left / action-right,
  exactly as today. When the combined width exceeds the card, the action wraps
  to its own line, left-aligned. The day name is never truncated.
- Drop `justify-self: end` from `.day-action`: flex `space-between` handles the
  inline case, and the wrapped case is naturally left-aligned.
- Add `min-width: 0` defensively so no descendant can bleed past the card edge.
- This requires a small markup change (introduce the wrapper element) plus the
  matching scoped-CSS edits in the same template.

The fix must preserve every status variant: `today` (clay CTA button),
`in_progress`, `completed` (struck-through day name), `past-incomplete`,
`upcoming`, and `unscheduled` (dashed hairline). The `data-ribbon` `::before`
ribbon and the per-day progress-width `<style>` block are unaffected.

## Testing

- `make test` — the home-page handler tests run through e2etest/goquery and
  assert on text and structure, not on layout. They should be unaffected
  provided button and link text stays stable. Run to confirm no regression.
- `make lint-fix` before committing.
- Visual check: render the home page at 320px width and confirm every status
  variant — including "Wednesday" + "Start Extra Workout" — wraps cleanly with
  no bleed past the card edge.

## Files touched

- `ui/templates/CLAUDE.md` — new "Text overflow & localization-readiness"
  section (Part 1).
- `ui/templates/pages/home/day-cards.gohtml` — markup and scoped CSS (Part 2).
