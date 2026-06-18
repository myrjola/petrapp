# Structured exercise content over rendered markdown

Exercise instructional content was a single `description_markdown` free-text
column rendered to HTML at request time with goldmark. The "free text" was a
fiction: every consumer assumed the *same* rigid three-section shape
(`## Instructions` numbered list, `## Common Mistakes` bullets, `## Resources`
links), and that shape was asserted independently in three places that did not
know about each other — the AI generation prompt, the `updateResourcesInDescription`
line-by-line splicer, and the exercise-info prose CSS, which reverse-engineered
the structure with `:has()` selectors (`ol li` → step cards, `ul li:has(> strong)`
→ mistake cards, `ul:has(> li > a:only-child)` → resource pills). We replaced
the markdown column with structured content on the `Exercise` aggregate —
`Instructions []string`, `CommonMistakes []string`, `Resources []Resource` —
making the one shape explicit instead of triplicated and re-derived.

## Considered options

- **Keep goldmark, keep markdown.** Rejected: the open-ended expressiveness
  (tables, inline emphasis, arbitrary headings) is unused, while the implicit
  three-section contract drifts silently across its three enforcers and the
  rendered HTML must be injected as trusted `template.HTML`.
- **Swap goldmark for a constrained block AST**, keeping one rich-text body.
  Rejected: preserves the general-purpose open-endedness that nothing uses; the
  content is a value object with exactly three known parts, not a document.
- **Normalize into child tables** (`exercise_instructions`, etc.). Rejected:
  the content is a value object owned by the `Exercise` aggregate, never queried
  independently or referenced by foreign key — child tables would model it as an
  entity it is not (cf. ADR-0003/0004's aggregate treatment). It persists as a
  single `content` JSON column on `exercises` instead.

## Consequences

- **Net code deletion.** The `goldmark` dependency, `markdownToHTML`,
  `updateResourcesInDescription`, `containsResourcesHeading`, and the prose
  `:has()` heuristics all go. Templates range over real fields; the AI schema
  emits the structure directly (the separate resource-splice step disappears —
  `validateResourceURLs` now filters an array instead of rewriting text).
- **The trusted-HTML seam is removed.** Structured fields render escaped, so the
  `template.HTML(...) //nolint:gosec` carve-out and its XSS surface are gone.
- **Validation moves into `Exercise.Validate()`.** The DB no longer CHECKs
  content shape; a single JSON column gives up per-row SQL queryability of
  instruction text, which nothing needed.
- **The three-section shape is now load-bearing.** Adding a fourth section
  (e.g. "Variations") becomes a schema change + premigration + fixture rewrite,
  where markdown would have been free typing. This is the accepted price of
  making the structure enforceable in one place.
- **Authoring is line-delimited, progressively enhanced.** The admin form drops
  the markdown textarea for one labeled textarea per section (one item per line;
  resources as `Title | URL`), split server-side with `strings.Split` — a
  deterministic transform, not a parser. The no-JS POST-Redirect-GET baseline
  (ADR-0002) is preserved; admins no longer need markdown literacy.
- **Migration.** Fixtures (the source of truth, re-applied every boot) are
  rewritten to JSON. A one-time `Premigration` parses any DB-only AI-generated
  rows' markdown into `content` before the structural migrator drops the
  `description_markdown` column.
