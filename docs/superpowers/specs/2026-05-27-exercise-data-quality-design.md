# Exercise data-quality cleanup

## Problem

The exercise library has two recurring data-quality issues:

1. **Dead links.** Every exercise generated before this change has a
   `## Resources` section in its `description_markdown`. The AI generator's
   Pass 1 always inserts `https://example.com/...` placeholders; Pass 2
   (web search) tries to replace them but silently leaves placeholders
   when it fails or returns no results. Even when Pass 2 succeeds the
   URLs come from the model's web search with no verification, so 404s
   ship to production.

2. **Redundant rep guidance in descriptions.** The Pass-1 prompt asks
   step 5 of the Instructions list to include "repetition guidance". The
   app already tracks rep targets separately (`exercises.rep_min` /
   `rep_max`) and shows them in the workout UI; the description's
   "Perform 8-12 reps" line is duplicate information that goes stale
   when the user adjusts the rep window for their session.

Both issues are visible on the user-facing exercise-info page
(`ui/templates/pages/exercise-info/exercise-info.gohtml`), which renders
the description's resource links as pills and its Instructions list as
numbered steps.

## Scope

Two deliverables:

- **Generator fixes** so new exercises don't ship with these defects.
- **One-shot backfill** that cleans up existing exercises in production
  (~38 in fixtures, plus AI-generated prod-only rows like `Pec Fly`
  noted in the 2026-05-13 cleanup).

Out of scope: automated detection (link-checking cron, admin quality
report, user "report a problem" flow). Those are tracked as possible
follow-ups but not designed here.

## Generator changes

File: `internal/service/exercise_generation.go`.

### Prompt: drop rep guidance, add stabilizer rule, remove placeholder URLs

`baseExercisePrompt` changes three things:

1. **Drop the rep-guidance step.** Remove the
   `[Optional step 5 with repetition guidance]` line from the
   Instructions structure template. Add an explicit rule below the
   structure block:

   > Do not include rep counts, set counts, weights, or durations in the
   > description. The app tracks these separately and shows them to the
   > user.

2. **Add the stabilizer rule for muscle groups.** Add to the rules block:

   > Only credit a muscle as primary or secondary if it performs a
   > working contraction (concentric or eccentric load). Pure isometric
   > stabilizers (e.g. the lats during a push-up, the upper back during
   > a bench press, the core during an overhead press) do not count.

   The 2026-05-13 cleanup script (`docs/2026-05-13-exercise-cleanup.md`)
   is the precedent — its "Out of scope" section explicitly flagged this
   prompt change as needed.

3. **Remove the `## Resources` block from the Pass-1 structure template.**
   Replace the example block with text instructing the model to emit only
   `## Instructions` and `## Common Mistakes`. Pass 2 appends the
   Resources section iff it finds valid URLs (see below). This removes
   the failure mode where Pass 2 errors entirely and the description
   ships with example.com placeholders.

### Pass 2: validate URLs before injecting them

A new private method on `exerciseGenerator`:

```go
func (eg *exerciseGenerator) validateResourceURLs(
    ctx context.Context,
    resources []domain.Resource,
) []domain.Resource
```

Behaviour:

- Takes the resources returned by the web-search call.
- For each, issues an HTTP HEAD request with a 5s timeout, following up
  to 5 redirects.
- Keeps the resource iff the final response is 2xx or 3xx.
- Drops it on any other status, network error, or timeout.
- Returns the filtered slice. Best-effort, no error return — failures
  during validation are individually logged at `slog.LevelDebug`.

The function uses an `*http.Client` held on `exerciseGenerator` so tests
can inject a fake (see Testing below). The default client is constructed
in `newExerciseGenerator` with the 5s timeout.

`enhanceWithWebSearch` is updated to:

1. Parse resources JSON as today.
2. Call `validateResourceURLs` on the parsed slice.
3. Pass the filtered slice into `updateResourcesInDescription`.

If the filtered slice is empty, the description's Resources section is
not added. The current `updateResourcesInDescription` already guards the
append-when-missing branch with `len(resources) > 0`; no change needed
there. The replace-existing branch is no longer reachable from new
generations (Pass 1 doesn't emit the heading anymore) but is kept in
place because Pass 2 might be re-invoked against an exercise that
already has resources — outside the scope of this change but worth not
breaking.

### Logging

- Each dropped URL: `slog.LevelDebug`, with the URL and the reason
  (HTTP status or error string).
- Zero URLs survived after validation:
  `slog.LevelInfo` ("dropped all resource URLs", exercise name).
- Whole Pass 2 errored: unchanged, `slog.LevelWarn` as today.

## Backfill tool

New one-shot Go program: `cmd/exercise-content-fixup/`.

### CLI

```
exercise-content-fixup -db <path-to-snapshot.sqlite3> -out <path-to-output.sql>
```

The tool is read-only against the input database. It emits a SQL one-shot
file plus a human-readable diff log to stdout.

### Workflow

1. Take a prod snapshot. See the `fly-ops` skill for the canonical
   read-only snapshot flow against the Fly Machine.
2. Run the tool locally against the snapshot.
3. Review the diff log and the generated SQL by eye.
4. Update `internal/sqlite/fixtures.sql` by hand to match the same
   cleanups (the seed file's exercise descriptions are the dev/test
   equivalent of the prod rows being cleaned).
5. Ship the SQL via the established
   `make fly-sql-write SCRIPT=docs/<DATE>-exercise-content-cleanup.sql`
   pattern.
6. Commit a `docs/<DATE>-exercise-content-cleanup.md` post-mortem
   alongside the SQL, matching the 2026-05-13 cleanup's format.

### Per-row transforms

For each `exercises.description_markdown`:

**(a) Strip dead resource links.**

- Parse the `## Resources` section: find lines matching
  `^\s*-\s*\[.*\]\((http[s]?://[^)]+)\)`.
- For each URL, HTTP HEAD with 5s timeout, 5-redirect cap. Treat 2xx/3xx
  final response as alive.
- Drop list items whose URL is dead, errors, or matches the placeholder
  prefix `https://example.com/` (those are guaranteed dead and we don't
  need to hit the network to confirm).
- If zero list items survive in the section, drop the `## Resources`
  heading and the (now empty) list.

**(b) Strip rep / set / duration list items from Instructions.**

In the `## Instructions` numbered list, drop any list item whose text
matches one of:

- `\b\d+\s*(?:-\s*\d+)?\s*(?:reps?|repetitions|sets)\b`
  (e.g. "Perform 8-12 reps", "Complete 3 sets")
- `\bhold\s+for\s+\d+\s*(?:seconds?|s)\b`
  (e.g. "Hold for 30 seconds")
- `\bperform\s+\d+\s*(?:to\s+\d+)?\s*(?:reps?|repetitions|times)\b`
- `\brepetition\s+guidance\b` (catches the literal prompt template
  leaking through)

Negative tests (must NOT match): "Step 3", "Take 2 deep breaths",
"3-second tempo on the descent" without rep/set context.

The remaining list items are not renumbered in the markdown source —
markdown renderers auto-number `1.` `1.` `1.` correctly. Leave the
original numerals in place to keep the diff minimal.

### Implementation layout

```
cmd/exercise-content-fixup/
├── main.go             — flag parsing, DB open, row loop, SQL emission
├── transforms.go       — pure string transforms (testable)
├── transforms_test.go  — table-driven tests for each regex case
└── linkcheck.go        — HEAD-check helper (~20 lines, duplicated from
                          internal/service rather than shared)
```

The transforms are pure functions:

```go
func StripDeadResourceLinks(desc string, aliveURLs map[string]bool) string
func StripRepGuidanceLines(desc string) string
```

HEAD-checking is done in `main.go`'s row loop; the result map is passed
into `StripDeadResourceLinks`. This keeps transforms unit-testable
without a network.

The link-check code (~20 lines) is intentionally duplicated rather than
extracted to a shared package, because:

- `cmd/exercise-content-fixup` is a one-shot tool that should not pull
  dependencies into `internal/service`'s reverse-dependency graph.
- The runtime generator's `validateResourceURLs` is intrinsic to the
  generator's behaviour and belongs on `exerciseGenerator`.

If the duplication grows or a third caller appears, revisit.

### Tool lifecycle

The tool stays in the repo long-term as a maintained one-shot. Future
link-rot sweeps rerun it against fresh snapshots, generating a new
dated SQL file. The post-mortem markdown captures *what* this specific
run changed; the tool itself is the durable engine.

## Testing

### Generator (internal tests, no live API)

In `internal/service/exercise_generation_internal_test.go`:

- `Test_validateResourceURLs`: spin up `httptest.Server` with multiple
  handlers (200, 301→200, 404, 500, body-but-no-status — i.e. timeout
  via `time.Sleep` longer than the test client timeout). Assert which
  URLs survive.
- `Test_updateResourcesInDescription_emptyResources`: pass an empty
  slice into the existing helper, assert the output markdown has no
  `## Resources` heading. (May require a small fix to the helper if
  the replace branch currently emits the heading even when the slice
  is empty — verify during implementation.)
- `Test_baseExercisePrompt`: snapshot-assert the prompt string contains:
  - The stabilizer-rule sentence (substring match).
  - The "do not include rep counts" sentence.
  - No `## Resources` heading.
  - No `example.com` substring.
  - No "repetition guidance" substring.

The existing live OpenAI test stays gated on `OPENAI_API_KEY` and
`testing.Short()`. Add one assertion: the live-generated description
must not contain a rep mention. Use a small regex inline in the test
(the backfill tool's full pattern set lives under `cmd/` and isn't
importable from `internal/service`). Catches future prompt-drift.

### Backfill transforms (unit tests)

In `cmd/exercise-content-fixup/transforms_test.go`:

- `TestStripDeadResourceLinks`: table-driven with several markdown
  fixtures (one with all-dead links, one with mixed, one with no
  Resources section, one where Resources is the last heading). Pass
  in an `aliveURLs` map and assert the output.
- `TestStripRepGuidanceLines`: table-driven covering all positive
  patterns above plus the negative cases ("Step 3", "Take 2 deep
  breaths").

No end-to-end test of the tool itself — transforms cover the logic,
the shipped SQL is reviewed by hand.

## Migration / rollout

1. Land generator changes + tests on `main`. Deploy. New exercises
   from this point on are clean.
2. Take a prod snapshot.
3. Run `exercise-content-fixup` locally against the snapshot, review
   diff log.
4. Update `internal/sqlite/fixtures.sql` by hand to match.
5. Ship the SQL one-shot via `make fly-sql-write`, alongside the
   `docs/<DATE>-exercise-content-cleanup.md` post-mortem.

Order matters: deploy generator changes first so the prod rows aren't
overwritten by stale generations between the snapshot and the SQL apply.

## Out of scope (possible follow-ups)

- **Periodic link checker.** A cron / scheduled job that re-validates
  resource URLs on existing exercises and surfaces decay in an admin
  report or log metric. Not designed here.
- **Admin quality dashboard.** A page listing exercises with no
  resources, empty descriptions, or suspicious muscle-group counts.
- **User "report a problem" affordance.** A button on the exercise-info
  page so users can flag bad data; surfaced to admin.
- **Stricter muscle-group caps.** Cap primary at 2 and secondary at 3
  in the prompt. Considered and dropped — the stabilizer rule alone
  should fix the over-tagging the 2026-05-13 cleanup surfaced, and
  hard caps risk dropping legitimate primary movers on compound lifts.
